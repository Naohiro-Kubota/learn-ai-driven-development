package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
)

// APIError carries public error identity, never an underlying error message.
type APIError struct {
	Status      int
	Code        string
	FieldErrors []fieldErrorDTO
}

var errorMessages = map[string]string{
	"invalid_request":              "Request validation failed.",
	"invalid_auth_transaction":     "The sign-in request is no longer valid.",
	"authentication_required":      "Authentication is required.",
	"forbidden":                    "You are not permitted to perform this operation.",
	"request_not_found":            "The requested resource was not found.",
	"version_conflict":             "The request version is stale.",
	"invalid_state":                "The request is not in a valid state for this operation.",
	"approval_routing_unavailable": "No eligible approver is available.",
	"csrf_validation_failed":       "CSRF validation failed.",
	"internal_error":               "An unexpected error occurred.",
}

// WriteError is the common writer for the OpenAPI ErrorResponse envelope.
func WriteError(w http.ResponseWriter, apiErr APIError) {
	message, ok := errorMessages[apiErr.Code]
	if !ok {
		apiErr = APIError{Status: http.StatusInternalServerError, Code: "internal_error"}
		message = errorMessages[apiErr.Code]
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(apiErr.Status)
	_ = json.NewEncoder(w).Encode(struct {
		Code        string          `json:"code"`
		Message     string          `json:"message"`
		FieldErrors []fieldErrorDTO `json:"fieldErrors,omitempty"`
	}{apiErr.Code, message, apiErr.FieldErrors})
}

func authAPIError(err error) APIError {
	switch {
	case errors.Is(err, auth.ErrNotFound), errors.Is(err, auth.ErrExpired), errors.Is(err, auth.ErrConsumed), errors.Is(err, auth.ErrInvalidAuthentication):
		return APIError{Status: http.StatusBadRequest, Code: "invalid_auth_transaction"}
	case errors.Is(err, auth.ErrCSRFValidation):
		return APIError{Status: http.StatusForbidden, Code: "csrf_validation_failed"}
	case errors.Is(err, auth.ErrForbidden):
		return APIError{Status: http.StatusForbidden, Code: "forbidden"}
	default:
		return APIError{Status: http.StatusInternalServerError, Code: "internal_error"}
	}
}
