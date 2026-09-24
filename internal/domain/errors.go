package domain

import "errors"

var (
	ErrForbidden                  = errors.New("forbidden")
	ErrNotFound                   = errors.New("not found")
	ErrVersionConflict            = errors.New("version conflict")
	ErrInvalidState               = errors.New("invalid state")
	ErrApprovalRoutingUnavailable = errors.New("approval routing unavailable")
	ErrInvalidRequest             = errors.New("invalid request")
)

// FieldViolation identifies a request field that failed validation.
type FieldViolation struct {
	Field string
	Code  string
}

// ValidationError preserves invalid-request identity and field details.
type ValidationError struct {
	Fields []FieldViolation
}

func (e *ValidationError) Error() string { return ErrInvalidRequest.Error() }
func (e *ValidationError) Unwrap() error { return ErrInvalidRequest }
