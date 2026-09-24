package httpapi

import (
	"bytes"
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

func decodeJSONBody(w http.ResponseWriter, request *http.Request, dst any, required ...string) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, request.Body, 64<<10))
	var object map[string]json.RawMessage
	if err := decodeStrictObject(decoder, &object); err != nil {
		return err
	}
	for _, field := range required {
		if _, ok := object[field]; !ok {
			return invalidField(field, "required")
		}
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return err
	}
	valueDecoder := json.NewDecoder(bytes.NewReader(encoded))
	valueDecoder.DisallowUnknownFields()
	if err := valueDecoder.Decode(dst); err != nil {
		var typeErr *json.UnmarshalTypeError
		if errors.As(err, &typeErr) && typeErr.Field != "" {
			return invalidField(typeErr.Field, "invalid")
		}
		return err
	}
	return nil
}

func invalidField(field, code string) error {
	return &domain.ValidationError{Fields: []domain.FieldViolation{{Field: field, Code: code}}}
}

func invalidInputAPIError(err error) APIError {
	if errors.Is(err, domain.ErrInvalidRequest) {
		return requestAPIError(err)
	}
	return APIError{Status: http.StatusBadRequest, Code: "invalid_request"}
}

func decodeStrictObject(decoder *json.Decoder, object *map[string]json.RawMessage) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok || delim != '{' {
		return errors.New("JSON body must be an object")
	}
	values := make(map[string]json.RawMessage)
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return err
		}
		key, ok := keyToken.(string)
		if !ok {
			return errors.New("invalid JSON property")
		}
		if _, exists := values[key]; exists {
			return errors.New("duplicate JSON property")
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return err
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return invalidField(key, "invalid")
		}
		values[key] = raw
	}
	end, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := end.(json.Delim); !ok || delim != '}' {
		return errors.New("JSON body is incomplete")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON documents")
		}
		return err
	}
	*object = values
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
	if err := decodeJSONBody(w, request, &input, "title"); err != nil {
		WriteError(w, invalidInputAPIError(err))
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
	if err := decodeJSONBody(w, request, &input, "title", "description", "expectedVersion"); err != nil {
		WriteError(w, invalidInputAPIError(err))
		return
	}
	if input.ExpectedVersion < 1 {
		WriteError(w, invalidInputAPIError(invalidField("expectedVersion", "invalid")))
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
	if err := decodeJSONBody(w, request, &input, "expectedVersion"); err != nil {
		WriteError(w, invalidInputAPIError(err))
		return
	}
	if input.ExpectedVersion < 1 {
		WriteError(w, invalidInputAPIError(invalidField("expectedVersion", "invalid")))
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
