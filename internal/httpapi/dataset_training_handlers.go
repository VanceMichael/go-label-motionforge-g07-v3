package httpapi

import (
	"net/http"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/dataset"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/training"
)

func (a *API) createDataset(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Name string `json:"name"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.DatasetDraft
	if err == nil {
		value, err = a.datasets.Create(request.Context(), principal, input.Name, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, value)
}

func (a *API) listDatasets(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	limit, limitErr := parseIntQuery(request, "limit", 50)
	offset, offsetErr := parseIntQuery(request, "offset", 0)
	statuses, statusErr := parseDatasetStatuses(request.URL.Query().Get("status"))
	if err == nil {
		err = limitErr
	}
	if err == nil {
		err = offsetErr
	}
	if err == nil {
		err = statusErr
	}
	var values []domain.DatasetDraft
	var total int
	if err == nil {
		values, total, err = a.datasets.List(request.Context(), principal, dataset.ListFilter{Statuses: statuses, Search: request.URL.Query().Get("search"), Limit: limit, Offset: offset})
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": values, "total": total, "limit": limit, "offset": offset})
}

func (a *API) getDataset(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var draft domain.DatasetDraft
	var items []domain.DatasetItem
	if err == nil {
		draft, items, err = a.datasets.Get(request.Context(), principal, request.PathValue("dataset_id"))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"dataset": draft, "items": items})
}

func (a *API) addDatasetCaptures(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		CaptureIDs []string `json:"capture_ids"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var draft domain.DatasetDraft
	var items []domain.DatasetItem
	if err == nil {
		draft, items, err = a.datasets.AddCaptures(request.Context(), principal, request.PathValue("dataset_id"), requestIDFromContext(request.Context()), input.CaptureIDs)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"dataset": draft, "items": items})
}

func (a *API) freezeDataset(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var value domain.DatasetDraft
	if err == nil {
		value, err = a.datasets.Freeze(request.Context(), principal, request.PathValue("dataset_id"), requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) reviewDataset(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Approve bool   `json:"approve"`
		Reason  string `json:"reason"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.DatasetDraft
	if err == nil {
		value, err = a.datasets.Review(request.Context(), principal, request.PathValue("dataset_id"), requestIDFromContext(request.Context()), input.Approve, input.Reason)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) publishDataset(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var value domain.DatasetRelease
	if err == nil {
		value, err = a.datasets.Publish(request.Context(), principal, request.PathValue("dataset_id"), requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, value)
}

func (a *API) revokeRelease(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Reason string `json:"reason"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.DatasetRelease
	if err == nil {
		value, err = a.datasets.Revoke(request.Context(), principal, request.PathValue("release_id"), requestIDFromContext(request.Context()), input.Reason)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) enqueueTraining(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var value domain.TrainingJob
	if err == nil {
		value, err = a.training.Enqueue(request.Context(), principal, request.PathValue("release_id"), requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, value)
}

func (a *API) claimTraining(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var result training.ClaimResult
	if err == nil {
		result, err = a.training.Claim(request.Context(), principal)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (a *API) getTraining(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var value domain.TrainingJob
	if err == nil {
		value, err = a.training.Get(request.Context(), principal, request.PathValue("job_id"))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) renewTraining(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		LeaseToken string `json:"lease_token"`
		Version    int64  `json:"version"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.TrainingJob
	if err == nil {
		value, err = a.training.Renew(request.Context(), principal, request.PathValue("job_id"), input.LeaseToken, input.Version)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) checkpointTraining(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		LeaseToken string `json:"lease_token"`
		Checkpoint string `json:"checkpoint"`
		Version    int64  `json:"version"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.TrainingJob
	if err == nil {
		value, err = a.training.Checkpoint(request.Context(), principal, request.PathValue("job_id"), input.LeaseToken, input.Checkpoint, input.Version)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) completeTraining(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		LeaseToken string `json:"lease_token"`
		OutputURI  string `json:"output_uri"`
		Version    int64  `json:"version"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.TrainingJob
	if err == nil {
		value, err = a.training.Complete(request.Context(), principal, request.PathValue("job_id"), input.LeaseToken, input.OutputURI, requestIDFromContext(request.Context()), input.Version)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) failTraining(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		LeaseToken string `json:"lease_token"`
		Message    string `json:"message"`
		Version    int64  `json:"version"`
		Permanent  bool   `json:"permanent"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.TrainingJob
	if err == nil {
		value, err = a.training.Fail(request.Context(), principal, request.PathValue("job_id"), input.LeaseToken, input.Message, requestIDFromContext(request.Context()), input.Version, input.Permanent)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}
