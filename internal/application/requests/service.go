package requests

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

const (
	maxTitleLength       = 120
	maxDescriptionLength = 2000
)

type Actor struct {
	MemberID       string
	OrganizationID string
	Roles          []domain.Role
}
type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: time.Now}
}

func (s *Service) CreateDraft(ctx context.Context, actor Actor, organizationID, title, description string) (domain.Request, error) {
	title, err := validateContent(title, description)
	if err != nil {
		return domain.Request{}, err
	}
	if !actor.hasRole(domain.RoleRequester) {
		return domain.Request{}, domain.ErrForbidden
	}
	now := s.now()
	request := domain.Request{OrganizationID: organizationID, RequesterMemberID: actor.MemberID, Title: title, Description: description, Status: domain.RequestStatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
	return s.repository.CreateDraft(ctx, CreateDraftCommand{Request: request, AuditEvent: auditEvent(request, actor.MemberID, "request_created", now)})
}

func (s *Service) UpdateDraft(ctx context.Context, actor Actor, requestID string, expectedVersion int64, title, description string) (domain.Request, error) {
	request, err := s.repository.Get(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if request.RequesterMemberID != actor.MemberID || !actor.hasRole(domain.RoleRequester) {
		return domain.Request{}, domain.ErrForbidden
	}
	if request.Status != domain.RequestStatusDraft {
		return domain.Request{}, domain.ErrInvalidState
	}
	if request.Version != expectedVersion {
		return domain.Request{}, domain.ErrVersionConflict
	}
	title, err = validateContent(title, description)
	if err != nil {
		return domain.Request{}, err
	}
	request.Title, request.Description, request.UpdatedAt = title, description, s.now()
	return s.repository.UpdateDraft(ctx, UpdateDraftCommand{Request: request, ExpectedVersion: expectedVersion, AuditEvent: auditEvent(request, actor.MemberID, "request_updated", request.UpdatedAt)})
}

func (s *Service) Submit(ctx context.Context, actor Actor, requestID string, expectedVersion int64) (domain.Request, error) {
	request, err := s.repository.Get(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if request.RequesterMemberID != actor.MemberID || !actor.hasRole(domain.RoleRequester) {
		return domain.Request{}, domain.ErrForbidden
	}
	if request.Status != domain.RequestStatusDraft {
		return domain.Request{}, domain.ErrInvalidState
	}
	if request.Version != expectedVersion {
		return domain.Request{}, domain.ErrVersionConflict
	}
	defaultApprover, err := s.repository.DefaultApprover(ctx, request.OrganizationID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return domain.Request{}, domain.ErrApprovalRoutingUnavailable
		}
		return domain.Request{}, err
	}
	if defaultApprover.MemberID == "" || !defaultApprover.HasApproverRole || defaultApprover.MemberID == actor.MemberID {
		return domain.Request{}, domain.ErrApprovalRoutingUnavailable
	}
	return s.repository.Submit(ctx, SubmitCommand{RequestID: requestID, RequesterMemberID: actor.MemberID, AssigneeMemberID: defaultApprover.MemberID, ExpectedVersion: expectedVersion, AuditEvent: auditEvent(request, actor.MemberID, "request_submitted", s.now())})
}

func (s *Service) Approve(ctx context.Context, actor Actor, requestID string, expectedVersion int64) (domain.Request, error) {
	request, err := s.repository.Get(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if request.Status != domain.RequestStatusPending {
		return domain.Request{}, domain.ErrInvalidState
	}
	if request.Version != expectedVersion {
		return domain.Request{}, domain.ErrVersionConflict
	}
	approval, err := s.repository.GetApproval(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if approval.Status != domain.ApprovalStatusPending {
		return domain.Request{}, domain.ErrInvalidState
	}
	if approval.AssigneeMemberID != actor.MemberID || !actor.hasRole(domain.RoleApprover) {
		return domain.Request{}, domain.ErrForbidden
	}
	return s.repository.Approve(ctx, ApproveCommand{RequestID: requestID, AssigneeMemberID: actor.MemberID, ExpectedVersion: expectedVersion, AuditEvent: auditEvent(request, actor.MemberID, "request_approved", s.now())})
}

func (s *Service) Get(ctx context.Context, actor Actor, requestID string) (domain.Request, error) {
	request, err := s.repository.Get(ctx, requestID)
	if err != nil {
		return domain.Request{}, err
	}
	if actor.OrganizationID == "" || actor.OrganizationID != request.OrganizationID {
		return domain.Request{}, domain.ErrNotFound
	}
	if request.RequesterMemberID == actor.MemberID || actor.hasRole(domain.RoleAdmin) {
		return request, nil
	}
	approval, err := s.repository.GetApproval(ctx, requestID)
	if err == nil && approval.AssigneeMemberID == actor.MemberID && actor.hasRole(domain.RoleApprover) {
		return request, nil
	}
	return domain.Request{}, domain.ErrNotFound
}

func (s *Service) GetApproval(ctx context.Context, actor Actor, requestID string) (*domain.Approval, error) {
	if _, err := s.Get(ctx, actor, requestID); err != nil {
		return nil, err
	}
	approval, err := s.repository.GetApproval(ctx, requestID)
	if errors.Is(err, domain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return approval, nil
}

func (s *Service) ListPending(ctx context.Context, actor Actor) ([]domain.Request, error) {
	if !actor.hasRole(domain.RoleApprover) {
		return nil, domain.ErrForbidden
	}
	return s.repository.ListPending(ctx, actor.MemberID)
}

func (s *Service) ListAuditEvents(ctx context.Context, actor Actor, requestID string) ([]domain.AuditEvent, error) {
	if _, err := s.Get(ctx, actor, requestID); err != nil {
		return nil, err
	}
	return s.repository.ListAuditEvents(ctx, requestID)
}

func (a Actor) hasRole(want domain.Role) bool {
	for _, role := range a.Roles {
		if role == want {
			return true
		}
	}
	return false
}
func validateContent(title, description string) (string, error) {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > maxTitleLength || len([]rune(description)) > maxDescriptionLength {
		return "", domain.ErrInvalidRequest
	}
	return title, nil
}
func auditEvent(request domain.Request, actorID, eventType string, occurredAt time.Time) domain.AuditEvent {
	return domain.AuditEvent{RequestID: request.ID, ActorMemberID: actorID, Type: eventType, OccurredAt: occurredAt, ContentSnapshot: &domain.ContentSnapshot{Title: request.Title, Description: request.Description}}
}
