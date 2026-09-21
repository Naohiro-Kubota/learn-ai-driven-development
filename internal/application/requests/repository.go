package requests

import (
	"context"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

type Repository interface {
	CreateDraft(context.Context, CreateDraftCommand) (domain.Request, error)
	Get(context.Context, string) (domain.Request, error)
	GetApproval(context.Context, string) (domain.Approval, error)
	UpdateDraft(context.Context, UpdateDraftCommand) (domain.Request, error)
	DefaultApprover(context.Context, string) (domain.DefaultApprover, error)
	Submit(context.Context, SubmitCommand) (domain.Request, error)
	Approve(context.Context, ApproveCommand) (domain.Request, error)
	ListPending(context.Context, string) ([]domain.Request, error)
	ListAuditEvents(context.Context, string) ([]domain.AuditEvent, error)
}

type CreateDraftCommand struct {
	Request    domain.Request
	AuditEvent domain.AuditEvent
}
type UpdateDraftCommand struct {
	Request         domain.Request
	ExpectedVersion int64
	AuditEvent      domain.AuditEvent
}
type SubmitCommand struct {
	RequestID, RequesterMemberID, AssigneeMemberID string
	ExpectedVersion                                int64
	AuditEvent                                     domain.AuditEvent
}
type ApproveCommand struct {
	RequestID, AssigneeMemberID string
	ExpectedVersion             int64
	AuditEvent                  domain.AuditEvent
}
