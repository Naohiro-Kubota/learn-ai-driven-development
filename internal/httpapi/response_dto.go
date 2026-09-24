package httpapi

import (
	"errors"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

// Request bodies intentionally contain no identity or organization fields.
type createRequestInput struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type updateDraftRequestInput struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

type expectedVersionInput struct {
	ExpectedVersion int64 `json:"expectedVersion"`
}

type requestDTO struct {
	ID                string       `json:"id"`
	Title             string       `json:"title"`
	Description       string       `json:"description"`
	Status            string       `json:"status"`
	Version           int64        `json:"version"`
	RequesterMemberID string       `json:"requesterMemberId"`
	Approval          *approvalDTO `json:"approval"`
	CreatedAt         time.Time    `json:"createdAt"`
	UpdatedAt         time.Time    `json:"updatedAt"`
}

type approvalDTO struct {
	ID               string     `json:"id"`
	AssigneeMemberID string     `json:"assigneeMemberId"`
	Status           string     `json:"status"`
	ApprovedAt       *time.Time `json:"approvedAt"`
}

type pendingRequestListDTO struct {
	Requests []requestDTO `json:"requests"`
}

type auditEventListDTO struct {
	Events []auditEventDTO `json:"events"`
}

type auditEventDTO struct {
	ID                       string             `json:"id"`
	Type                     string             `json:"type"`
	OccurredAt               time.Time          `json:"occurredAt"`
	ActorMemberID            string             `json:"actorMemberId"`
	RequestContent           *requestContentDTO `json:"requestContent"`
	ApprovalAssigneeMemberID *string            `json:"approvalAssigneeMemberId"`
	ApprovalID               *string            `json:"approvalId"`
}

type requestContentDTO struct {
	Title       string `json:"title"`
	Description string `json:"description"`
}

type fieldErrorDTO struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func toRequestDTO(request domain.Request, approval *domain.Approval) requestDTO {
	var approvalValue *approvalDTO
	if approval != nil {
		approvalValue = &approvalDTO{ID: approval.ID, AssigneeMemberID: approval.AssigneeMemberID, Status: string(approval.Status), ApprovedAt: approval.ApprovedAt}
	}
	return requestDTO{ID: request.ID, Title: request.Title, Description: request.Description, Status: string(request.Status), Version: request.Version, RequesterMemberID: request.RequesterMemberID, Approval: approvalValue, CreatedAt: request.CreatedAt, UpdatedAt: request.UpdatedAt}
}

func toPendingRequestListDTO(requests []domain.Request) pendingRequestListDTO {
	items := make([]requestDTO, 0, len(requests))
	for _, request := range requests {
		items = append(items, toRequestDTO(request, nil))
	}
	return pendingRequestListDTO{Requests: items}
}

func toAuditEventDTO(event domain.AuditEvent) auditEventDTO {
	dto := auditEventDTO{ID: event.ID, Type: event.Type, OccurredAt: event.OccurredAt, ActorMemberID: event.ActorMemberID}
	if event.ContentSnapshot != nil {
		dto.RequestContent = &requestContentDTO{Title: event.ContentSnapshot.Title, Description: event.ContentSnapshot.Description}
	}
	if event.ApprovalMetadata != nil {
		assignee, approvalID := event.ApprovalMetadata.AssigneeMemberID, event.ApprovalMetadata.ApprovalID
		dto.ApprovalAssigneeMemberID, dto.ApprovalID = &assignee, &approvalID
	}
	return dto
}

func toAuditEventListDTO(events []domain.AuditEvent) auditEventListDTO {
	items := make([]auditEventDTO, 0, len(events))
	for _, event := range events {
		items = append(items, toAuditEventDTO(event))
	}
	return auditEventListDTO{Events: items}
}

func requestAPIError(err error) APIError {
	switch {
	case errors.Is(err, domain.ErrInvalidRequest):
		apiErr := APIError{Status: 400, Code: "invalid_request"}
		var validation *domain.ValidationError
		if errors.As(err, &validation) {
			for _, violation := range validation.Fields {
				apiErr.FieldErrors = append(apiErr.FieldErrors, fieldErrorDTO{Field: violation.Field, Code: violation.Code, Message: fieldErrorMessage(violation)})
			}
		}
		return apiErr
	case errors.Is(err, domain.ErrForbidden):
		return APIError{Status: 403, Code: "forbidden"}
	case errors.Is(err, domain.ErrNotFound):
		return APIError{Status: 404, Code: "request_not_found"}
	case errors.Is(err, domain.ErrVersionConflict):
		return APIError{Status: 409, Code: "version_conflict"}
	case errors.Is(err, domain.ErrInvalidState):
		return APIError{Status: 409, Code: "invalid_state"}
	case errors.Is(err, domain.ErrApprovalRoutingUnavailable):
		return APIError{Status: 409, Code: "approval_routing_unavailable"}
	default:
		return APIError{Status: 500, Code: "internal_error"}
	}
}

func fieldErrorMessage(violation domain.FieldViolation) string {
	switch violation {
	case domain.FieldViolation{Field: "title", Code: "required"}:
		return "Title must not be blank."
	case domain.FieldViolation{Field: "title", Code: "too_long"}:
		return "Title must be at most 120 characters."
	case domain.FieldViolation{Field: "description", Code: "too_long"}:
		return "Description must be at most 2000 characters."
	case domain.FieldViolation{Field: "description", Code: "required"}:
		return "Description is required."
	case domain.FieldViolation{Field: "expectedVersion", Code: "required"}:
		return "Expected version is required."
	default:
		return "Invalid field value."
	}
}
