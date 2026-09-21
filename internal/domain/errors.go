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
