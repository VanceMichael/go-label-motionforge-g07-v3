package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/annotation"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/dataset"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/facility"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/storage"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/stream"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/training"
)

type Dependencies struct {
	Database    *storage.Database
	Auth        *auth.Service
	Facilities  *facility.Service
	Captures    *capture.Service
	Streams     *stream.Service
	Annotations *annotation.Service
	Datasets    *dataset.Service
	Training    *training.Service
	Logger      *slog.Logger
}

type API struct {
	database    *storage.Database
	auth        *auth.Service
	facilities  *facility.Service
	captures    *capture.Service
	streams     *stream.Service
	annotations *annotation.Service
	datasets    *dataset.Service
	training    *training.Service
	logger      *slog.Logger
}

func New(deps Dependencies) (*API, error) {
	if deps.Database == nil || deps.Auth == nil || deps.Facilities == nil || deps.Captures == nil || deps.Streams == nil || deps.Annotations == nil || deps.Datasets == nil || deps.Training == nil {
		return nil, errors.New("all API dependencies are required")
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	return &API{database: deps.Database, auth: deps.Auth, facilities: deps.Facilities, captures: deps.Captures, streams: deps.Streams, annotations: deps.Annotations, datasets: deps.Datasets, training: deps.Training, logger: deps.Logger}, nil
}

func (a *API) Handler() http.Handler {
	public := http.NewServeMux()
	public.HandleFunc("GET /healthz", a.health)
	public.HandleFunc("GET /readyz", a.ready)
	public.HandleFunc("POST /api/v1/bootstrap", a.bootstrap)
	public.HandleFunc("POST /api/v1/auth/login", a.login)

	protected := http.NewServeMux()
	protected.HandleFunc("POST /api/v1/auth/logout", a.logout)
	protected.HandleFunc("POST /api/v1/users", a.createUser)
	protected.HandleFunc("POST /api/v1/facilities", a.createFacility)
	protected.HandleFunc("POST /api/v1/rigs", a.createRig)
	protected.HandleFunc("POST /api/v1/scenarios", a.createScenario)
	protected.HandleFunc("POST /api/v1/captures", a.planCapture)
	protected.HandleFunc("GET /api/v1/captures/{capture_id}", a.getCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/ready", a.readyCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/start", a.startCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/submit", a.submitCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/validate", a.validateCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/reject", a.rejectCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/cancel", a.cancelCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/reopen", a.reopenCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/manifests", a.openManifest)
	protected.HandleFunc("GET /api/v1/captures/{capture_id}/manifests", a.listManifests)
	protected.HandleFunc("POST /api/v1/manifests/{manifest_id}/segments", a.appendSegments)
	protected.HandleFunc("POST /api/v1/manifests/{manifest_id}/seal", a.sealManifest)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/align", a.alignCapture)
	protected.HandleFunc("POST /api/v1/captures/{capture_id}/annotation-batches", a.createAnnotationBatch)
	protected.HandleFunc("GET /api/v1/annotation-batches/{batch_id}", a.getAnnotationBatch)
	protected.HandleFunc("POST /api/v1/annotation-batches/{batch_id}/claim", a.claimAnnotationBatch)
	protected.HandleFunc("POST /api/v1/annotation-batches/{batch_id}/renew", a.renewAnnotationBatch)
	protected.HandleFunc("POST /api/v1/annotation-batches/{batch_id}/items/{item_id}", a.annotateItem)
	protected.HandleFunc("POST /api/v1/annotation-batches/{batch_id}/submit", a.submitAnnotationBatch)
	protected.HandleFunc("POST /api/v1/annotation-batches/{batch_id}/review", a.reviewAnnotationBatch)
	protected.HandleFunc("POST /api/v1/datasets", a.createDataset)
	protected.HandleFunc("GET /api/v1/datasets", a.listDatasets)
	protected.HandleFunc("GET /api/v1/datasets/{dataset_id}", a.getDataset)
	protected.HandleFunc("POST /api/v1/datasets/{dataset_id}/captures", a.addDatasetCaptures)
	protected.HandleFunc("POST /api/v1/datasets/{dataset_id}/freeze", a.freezeDataset)
	protected.HandleFunc("POST /api/v1/datasets/{dataset_id}/review", a.reviewDataset)
	protected.HandleFunc("POST /api/v1/datasets/{dataset_id}/publish", a.publishDataset)
	protected.HandleFunc("POST /api/v1/releases/{release_id}/revoke", a.revokeRelease)
	protected.HandleFunc("POST /api/v1/releases/{release_id}/training-jobs", a.enqueueTraining)
	protected.HandleFunc("POST /api/v1/workers/training/claim", a.claimTraining)
	protected.HandleFunc("GET /api/v1/training-jobs/{job_id}", a.getTraining)
	protected.HandleFunc("POST /api/v1/training-jobs/{job_id}/renew", a.renewTraining)
	protected.HandleFunc("POST /api/v1/training-jobs/{job_id}/checkpoint", a.checkpointTraining)
	protected.HandleFunc("POST /api/v1/training-jobs/{job_id}/complete", a.completeTraining)
	protected.HandleFunc("POST /api/v1/training-jobs/{job_id}/fail", a.failTraining)

	root := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if isPublic(request.Method, request.URL.Path) {
			public.ServeHTTP(writer, request)
			return
		}
		a.authenticate(protected).ServeHTTP(writer, request)
	})
	return withRequestID(recoverPanic(a.logger, requestLog(a.logger, root)))
}

func isPublic(method, path string) bool {
	return method == http.MethodGet && (path == "/healthz" || path == "/readyz") ||
		method == http.MethodPost && (path == "/api/v1/bootstrap" || path == "/api/v1/auth/login")
}

func (a *API) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "alive"})
}

func (a *API) ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if err := a.database.Ping(ctx); err != nil {
		writeError(a.logger, writer, request, domain.Wrap(domain.ErrUnavailable, "http.ready", "", "", "database is unavailable", err))
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, target any) error {
	request.Body = http.MaxBytesReader(writer, request.Body, 1<<20)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.Validation("http.decode", "invalid JSON body: "+err.Error())
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.Validation("http.decode", "request body must contain one JSON object")
	}
	return nil
}

func parseIntQuery(request *http.Request, key string, fallback int) (int, error) {
	raw := request.URL.Query().Get(key)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.Validation("http.query", key+" must be an integer")
	}
	return value, nil
}

func parseDatasetStatuses(raw string) ([]domain.DatasetStatus, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	values := strings.Split(raw, ",")
	statuses := make([]domain.DatasetStatus, 0, len(values))
	for _, value := range values {
		status := domain.DatasetStatus(strings.TrimSpace(value))
		switch status {
		case domain.DatasetStatusDraft, domain.DatasetStatusFrozen, domain.DatasetStatusApproved,
			domain.DatasetStatusPublished, domain.DatasetStatusRevoked, domain.DatasetStatusArchived:
			statuses = append(statuses, status)
		default:
			return nil, domain.Validation("http.query", "status contains an unsupported value")
		}
	}
	return statuses, nil
}
