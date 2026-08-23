package httpapi

import (
	"net/http"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/annotation"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
)

func (a *API) createAnnotationBatch(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var batch domain.AnnotationBatch
	var items []domain.AnnotationItem
	if err == nil {
		batch, items, err = a.annotations.Create(request.Context(), principal, request.PathValue("capture_id"), requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, map[string]any{"batch": batch, "items": items})
}

func (a *API) getAnnotationBatch(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var batch domain.AnnotationBatch
	var items []domain.AnnotationItem
	if err == nil {
		batch, items, err = a.annotations.Get(request.Context(), principal, request.PathValue("batch_id"))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"batch": batch, "items": items})
}

func (a *API) claimAnnotationBatch(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var result annotation.ClaimResult
	if err == nil {
		result, err = a.annotations.Claim(request.Context(), principal, request.PathValue("batch_id"), requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (a *API) renewAnnotationBatch(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		LeaseToken string `json:"lease_token"`
		Version    int64  `json:"version"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var batch domain.AnnotationBatch
	if err == nil {
		batch, err = a.annotations.Renew(request.Context(), principal, request.PathValue("batch_id"), input.LeaseToken, requestIDFromContext(request.Context()), input.Version)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, batch)
}

func (a *API) annotateItem(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		LeaseToken string `json:"lease_token"`
		Label      string `json:"label"`
		Payload    string `json:"payload"`
		Version    int64  `json:"version"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var item domain.AnnotationItem
	if err == nil {
		item, err = a.annotations.Annotate(request.Context(), principal, request.PathValue("batch_id"), input.LeaseToken, request.PathValue("item_id"), input.Label, input.Payload, requestIDFromContext(request.Context()), input.Version)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, item)
}

func (a *API) submitAnnotationBatch(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		LeaseToken string `json:"lease_token"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var batch domain.AnnotationBatch
	if err == nil {
		batch, err = a.annotations.Submit(request.Context(), principal, request.PathValue("batch_id"), input.LeaseToken, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, batch)
}

func (a *API) reviewAnnotationBatch(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Accept bool   `json:"accept"`
		Reason string `json:"reason"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var batch domain.AnnotationBatch
	if err == nil {
		batch, err = a.annotations.Review(request.Context(), principal, request.PathValue("batch_id"), requestIDFromContext(request.Context()), input.Accept, input.Reason)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, batch)
}
