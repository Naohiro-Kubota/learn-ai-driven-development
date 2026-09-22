package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

func (r *router) requestActor(w http.ResponseWriter, request *http.Request) (requests.Actor, bool) {
	actor, ok := ActorFromContext(request.Context())
	if !ok || actor.MemberID == "" || actor.OrganizationID == "" {
		WriteError(w, APIError{Status: http.StatusUnauthorized, Code: "authentication_required"})
		return requests.Actor{}, false
	}
	return actor, true
}

func decodeJSONBody(w http.ResponseWriter, request *http.Request, dst any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 64<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON documents")
		}
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (r *router) approvalDTO(w http.ResponseWriter, request *http.Request, actor requests.Actor, result domain.Request) (requestDTO, bool) {
	approval, err := r.dependencies.RequestService.GetApproval(request.Context(), actor, result.ID)
	if err != nil {
		WriteError(w, requestAPIError(err))
		return requestDTO{}, false
	}
	return toRequestDTO(result, approval), true
}

func (r *router) createRequest(w http.ResponseWriter, request *http.Request) {
	actor, ok := r.requestActor(w, request)
	if !ok {
		return
	}
	var input createRequestInput
	if err := decodeJSONBody(w, request, &input); err != nil {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_request"})
		return
	}
	result, err := r.dependencies.RequestService.CreateDraft(request.Context(), actor, actor.OrganizationID, input.Title, input.Description)
	if err != nil {
		WriteError(w, requestAPIError(err))
		return
	}
	dto, ok := r.approvalDTO(w, request, actor, result)
	if !ok {
		return
	}
	w.Header().Set("Location", "/api/v1/requests/"+result.ID)
	writeJSON(w, http.StatusCreated, dto)
}

func (r *router) getRequest(w http.ResponseWriter, request *http.Request) {
	actor, ok := r.requestActor(w, request)
	if !ok {
		return
	}
	result, err := r.dependencies.RequestService.Get(request.Context(), actor, request.PathValue("requestId"))
	if err != nil {
		WriteError(w, requestAPIError(err))
		return
	}
	dto, ok := r.approvalDTO(w, request, actor, result)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (r *router) updateRequest(w http.ResponseWriter, request *http.Request) {
	actor, ok := r.requestActor(w, request)
	if !ok {
		return
	}
	var input updateDraftRequestInput
	if err := decodeJSONBody(w, request, &input); err != nil || input.ExpectedVersion < 1 {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_request"})
		return
	}
	result, err := r.dependencies.RequestService.UpdateDraft(request.Context(), actor, request.PathValue("requestId"), input.ExpectedVersion, input.Title, input.Description)
	if err != nil {
		WriteError(w, requestAPIError(err))
		return
	}
	dto, ok := r.approvalDTO(w, request, actor, result)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (r *router) submitRequest(w http.ResponseWriter, request *http.Request) {
	r.mutateRequest(w, request, r.dependencies.RequestService.Submit)
}
func (r *router) approveRequest(w http.ResponseWriter, request *http.Request) {
	r.mutateRequest(w, request, r.dependencies.RequestService.Approve)
}

func (r *router) mutateRequest(w http.ResponseWriter, request *http.Request, operation func(context.Context, requests.Actor, string, int64) (domain.Request, error)) {
	actor, ok := r.requestActor(w, request)
	if !ok {
		return
	}
	var input expectedVersionInput
	if err := decodeJSONBody(w, request, &input); err != nil || input.ExpectedVersion < 1 {
		WriteError(w, APIError{Status: http.StatusBadRequest, Code: "invalid_request"})
		return
	}
	result, err := operation(request.Context(), actor, request.PathValue("requestId"), input.ExpectedVersion)
	if err != nil {
		WriteError(w, requestAPIError(err))
		return
	}
	dto, ok := r.approvalDTO(w, request, actor, result)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, dto)
}

func (r *router) listPending(w http.ResponseWriter, request *http.Request) {
	actor, ok := r.requestActor(w, request)
	if !ok {
		return
	}
	results, err := r.dependencies.RequestService.ListPending(request.Context(), actor)
	if err != nil {
		WriteError(w, requestAPIError(err))
		return
	}
	items := make([]requestDTO, 0, len(results))
	for _, result := range results {
		dto, ok := r.approvalDTO(w, request, actor, result)
		if !ok {
			return
		}
		items = append(items, dto)
	}
	writeJSON(w, http.StatusOK, pendingRequestListDTO{Requests: items})
}

func (r *router) listAuditEvents(w http.ResponseWriter, request *http.Request) {
	actor, ok := r.requestActor(w, request)
	if !ok {
		return
	}
	results, err := r.dependencies.RequestService.ListAuditEvents(request.Context(), actor, request.PathValue("requestId"))
	if err != nil {
		WriteError(w, requestAPIError(err))
		return
	}
	writeJSON(w, http.StatusOK, toAuditEventListDTO(results))
}
