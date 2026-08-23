package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/VanceMichael/go-base-motionforge-g07/internal/auth"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/capture"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/domain"
	"github.com/VanceMichael/go-base-motionforge-g07/internal/stream"
)

func (a *API) planCapture(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input capture.PlanInput
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	input.RequestID = requestIDFromContext(request.Context())
	var result capture.PlanResult
	if err == nil {
		result, err = a.captures.Plan(request.Context(), principal, input)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	status := http.StatusCreated
	if result.Replay {
		status = http.StatusOK
	}
	writeJSON(writer, status, result)
}

func (a *API) getCapture(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var value domain.CaptureSession
	if err == nil {
		value, err = a.captures.Get(request.Context(), principal, request.PathValue("capture_id"))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) readyCapture(writer http.ResponseWriter, request *http.Request) {
	a.captureTransition(writer, request, a.captures.MarkReady)
}

func (a *API) startCapture(writer http.ResponseWriter, request *http.Request) {
	a.captureTransition(writer, request, a.captures.Start)
}

func (a *API) submitCapture(writer http.ResponseWriter, request *http.Request) {
	a.captureTransition(writer, request, a.captures.Submit)
}

func (a *API) validateCapture(writer http.ResponseWriter, request *http.Request) {
	a.captureTransition(writer, request, a.captures.Validate)
}

func (a *API) rejectCapture(writer http.ResponseWriter, request *http.Request) {
	a.captureTransition(writer, request, a.captures.Reject)
}

func (a *API) cancelCapture(writer http.ResponseWriter, request *http.Request) {
	a.captureTransition(writer, request, a.captures.Cancel)
}

func (a *API) reopenCapture(writer http.ResponseWriter, request *http.Request) {
	a.captureTransition(writer, request, a.captures.Reopen)
}

func (a *API) captureTransition(writer http.ResponseWriter, request *http.Request, operation func(context.Context, auth.Principal, string, string) (domain.CaptureSession, error)) {
	principal, err := principalFromContext(request.Context())
	var value domain.CaptureSession
	if err == nil {
		value, err = operation(request.Context(), principal, request.PathValue("capture_id"), requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) openManifest(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Kind domain.StreamKind `json:"kind"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var value domain.StreamManifest
	if err == nil {
		value, err = a.streams.OpenManifest(request.Context(), principal, request.PathValue("capture_id"), input.Kind, requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, value)
}

func (a *API) listManifests(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var values []domain.StreamManifest
	if err == nil {
		values, err = a.streams.List(request.Context(), principal, request.PathValue("capture_id"))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": values})
}

func (a *API) appendSegments(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		Segments []stream.SegmentInput `json:"segments"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var result stream.AppendResult
	if err == nil {
		result, err = a.streams.Append(request.Context(), principal, request.PathValue("manifest_id"), requestIDFromContext(request.Context()), input.Segments)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, result)
}

func (a *API) sealManifest(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var value domain.StreamManifest
	if err == nil {
		value, err = a.streams.Seal(request.Context(), principal, request.PathValue("manifest_id"), requestIDFromContext(request.Context()))
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, value)
}

func (a *API) alignCapture(writer http.ResponseWriter, request *http.Request) {
	principal, err := principalFromContext(request.Context())
	var input struct {
		ToleranceMillis int64 `json:"tolerance_millis"`
	}
	if err == nil {
		err = decodeJSON(writer, request, &input)
	}
	var values []domain.StreamManifest
	if err == nil {
		values, err = a.streams.AlignCapture(request.Context(), principal, request.PathValue("capture_id"), requestIDFromContext(request.Context()), time.Duration(input.ToleranceMillis)*time.Millisecond)
	}
	if err != nil {
		writeError(a.logger, writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"items": values})
}
