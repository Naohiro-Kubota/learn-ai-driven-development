package requests

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

func requester(id string) Actor {
	return Actor{MemberID: id, OrganizationID: "organization", Roles: []domain.Role{domain.RoleRequester}}
}
func approver(id string) Actor {
	return Actor{MemberID: id, OrganizationID: "organization", Roles: []domain.Role{domain.RoleApprover}}
}

func TestCreateDraftNormalizesTitleAndAllowsEmptyDescription(t *testing.T) {
	repo := newFakeRepository()
	request, err := NewService(repo).CreateDraft(context.Background(), requester("requester"), "organization", "  VPN access  ", "")
	if err != nil {
		t.Fatalf("CreateDraft() error = %v", err)
	}
	if request.Title != "VPN access" || request.Description != "" {
		t.Errorf("request = %#v, want normalized title and empty description", request)
	}
	assertAuditSnapshot(t, repo.auditEvents, "request_created", "requester", "VPN access", "")
}

func TestCreateDraftRejectsInvalidTitle(t *testing.T) {
	for _, testCase := range []struct{ name, title string }{
		{name: "whitespace only", title: " \t\n "},
		{name: "over 120 characters", title: strings.Repeat("x", 121)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewService(newFakeRepository()).CreateDraft(context.Background(), requester("requester"), "organization", testCase.title, "")
			if !errors.Is(err, domain.ErrInvalidRequest) {
				t.Fatalf("CreateDraft() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestCreateDraftReportsInvalidContentFields(t *testing.T) {
	for _, tc := range []struct {
		name, title, description string
		want                     []domain.FieldViolation
	}{
		{"blank title", " \t ", "", []domain.FieldViolation{{Field: "title", Code: "required"}}},
		{"long title", strings.Repeat("x", 121), "", []domain.FieldViolation{{Field: "title", Code: "too_long"}}},
		{"long description", "Title", strings.Repeat("x", 2001), []domain.FieldViolation{{Field: "description", Code: "too_long"}}},
		{"both fields", " ", strings.Repeat("x", 2001), []domain.FieldViolation{{Field: "title", Code: "required"}, {Field: "description", Code: "too_long"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(newFakeRepository()).CreateDraft(context.Background(), requester("requester"), "organization", tc.title, tc.description)
			if !errors.Is(err, domain.ErrInvalidRequest) {
				t.Fatalf("error = %v, want ErrInvalidRequest", err)
			}
			var validation *domain.ValidationError
			if !errors.As(err, &validation) || !reflect.DeepEqual(validation.Fields, tc.want) {
				t.Fatalf("validation = %#v, want %#v", validation, tc.want)
			}
		})
	}
}

func TestUpdateDraftAllowsOnlyTheRequesterAndDraftState(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		actor   Actor
		request domain.Request
		wantErr error
	}{
		{name: "requester updates draft", actor: requester("requester"), request: draftRequest()},
		{name: "another requester is forbidden", actor: requester("other"), request: draftRequest(), wantErr: domain.ErrForbidden},
		{name: "pending request is immutable", actor: requester("requester"), request: pendingRequest(), wantErr: domain.ErrInvalidState},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newFakeRepository(testCase.request)
			updated, err := NewService(repo).UpdateDraft(context.Background(), testCase.actor, "request-1", 1, "Updated", "")
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("UpdateDraft() error = %v, want %v", err, testCase.wantErr)
			}
			if testCase.wantErr == nil && updated.Title != "Updated" {
				t.Errorf("Title = %q, want Updated", updated.Title)
			}
			if testCase.wantErr == nil {
				assertAuditSnapshot(t, repo.auditEvents, "request_updated", "requester", "Updated", "")
			} else if got := repo.AuditEventCount(); got != 0 {
				t.Errorf("AuditEventCount() = %d, want 0", got)
			}
		})
	}
}

func TestSubmitAppliesRoutingAuthorizationVersionAndAudit(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		actor    Actor
		approver string
		version  int64
		wantErr  error
	}{
		{name: "requester submits to default approver", actor: requester("requester"), approver: "approver", version: 1},
		{name: "default approver unavailable", actor: requester("requester"), version: 1, wantErr: domain.ErrApprovalRoutingUnavailable},
		{name: "another requester is forbidden", actor: requester("other"), approver: "approver", version: 1, wantErr: domain.ErrForbidden},
		{name: "self approval routing is unavailable", actor: requester("requester"), approver: "requester", version: 1, wantErr: domain.ErrApprovalRoutingUnavailable},
		{name: "stale version is rejected without audit", actor: requester("requester"), approver: "approver", version: 0, wantErr: domain.ErrVersionConflict},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newFakeRepository(draftRequest())
			repo.defaultApproverID = testCase.approver
			submitted, err := NewService(repo).Submit(context.Background(), testCase.actor, "request-1", testCase.version)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Submit() error = %v, want %v", err, testCase.wantErr)
			}
			if testCase.wantErr != nil {
				if got := repo.AuditEventCount(); got != 0 {
					t.Errorf("AuditEventCount() = %d, want 0", got)
				}
				return
			}
			if submitted.Status != domain.RequestStatusPending || submitted.Version != 2 {
				t.Errorf("submitted = %#v, want pending version 2", submitted)
			}
			if repo.approval.AssigneeMemberID != "approver" {
				t.Errorf("approval assignee = %q, want approver", repo.approval.AssigneeMemberID)
			}
			assertAuditSnapshot(t, repo.auditEvents, "request_submitted", "requester", "VPN access", "")
		})
	}
}

func TestSubmitMapsMissingDefaultApproverToRoutingUnavailable(t *testing.T) {
	repo := newFakeRepository(draftRequest())
	repo.defaultApproverErr = domain.ErrNotFound
	_, err := NewService(repo).Submit(context.Background(), requester("requester"), "request-1", 1)
	if !errors.Is(err, domain.ErrApprovalRoutingUnavailable) {
		t.Fatalf("Submit() error = %v, want ErrApprovalRoutingUnavailable", err)
	}
	if got := repo.AuditEventCount(); got != 0 {
		t.Errorf("AuditEventCount() = %d, want 0", got)
	}
}

func TestSubmitRejectsDefaultApproverWithoutApproverRole(t *testing.T) {
	repo := newFakeRepository(draftRequest())
	repo.defaultApproverID = "approver"
	repo.defaultApproverEligible = false
	_, err := NewService(repo).Submit(context.Background(), requester("requester"), "request-1", 1)
	if !errors.Is(err, domain.ErrApprovalRoutingUnavailable) {
		t.Fatalf("Submit() error = %v, want ErrApprovalRoutingUnavailable", err)
	}
	if got := repo.AuditEventCount(); got != 0 {
		t.Errorf("AuditEventCount() = %d, want 0", got)
	}
}

func TestApproveAllowsOnlyAssignedApproverAndWritesAuditSnapshot(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		actor   Actor
		version int64
		wantErr error
	}{
		{name: "assigned approver approves", actor: approver("approver"), version: 2},
		{name: "unassigned approver is forbidden", actor: approver("other"), version: 2, wantErr: domain.ErrForbidden},
		{name: "stale version is rejected without audit", actor: approver("approver"), version: 1, wantErr: domain.ErrVersionConflict},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newFakeRepository(pendingRequest())
			approved, err := NewService(repo).Approve(context.Background(), testCase.actor, "request-1", testCase.version)
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Approve() error = %v, want %v", err, testCase.wantErr)
			}
			if testCase.wantErr != nil {
				if got := repo.AuditEventCount(); got != 0 {
					t.Errorf("AuditEventCount() = %d, want 0", got)
				}
				return
			}
			if approved.Status != domain.RequestStatusApproved || approved.Version != 3 {
				t.Errorf("approved = %#v, want approved version 3", approved)
			}
			assertAuditSnapshot(t, repo.auditEvents, "request_approved", "approver", "VPN access", "")
		})
	}
}

func TestApproveRejectsNonPendingApprovalWithoutAuditEvent(t *testing.T) {
	repo := newFakeRepository(pendingRequest())
	repo.approval.Status = domain.ApprovalStatusApproved
	_, err := NewService(repo).Approve(context.Background(), approver("approver"), "request-1", 2)
	if !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("Approve() error = %v, want ErrInvalidState", err)
	}
	if got := repo.AuditEventCount(); got != 0 {
		t.Errorf("AuditEventCount() = %d, want 0", got)
	}
}

func TestApproveWithApprovalReturnsTransitionDataAndPreservesErrors(t *testing.T) {
	for _, tc := range []struct {
		name        string
		request     domain.Request
		actor       Actor
		approvalErr error
		wantErr     error
	}{
		{"assigned pending", pendingRequest(), approver("approver"), nil, nil},
		{"unassigned pending", pendingRequest(), approver("other"), nil, domain.ErrForbidden},
		{"approved request", func() domain.Request { r := pendingRequest(); r.Status = domain.RequestStatusApproved; return r }(), approver("approver"), nil, domain.ErrInvalidState},
		{"requester draft", draftRequest(), requester("requester"), nil, domain.ErrInvalidState},
		{"approval read failure", pendingRequest(), approver("approver"), errors.New("database unavailable"), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeRepository(tc.request)
			repo.approvalErr = tc.approvalErr
			request, approval, err := NewService(repo).ApproveWithApproval(context.Background(), tc.actor, "request-1", 2)
			if tc.approvalErr != nil {
				if err != tc.approvalErr {
					t.Fatalf("error = %v, want database error", err)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if err != nil {
				if repo.AuditEventCount() != 0 {
					t.Fatal("failed approval wrote audit event")
				}
				return
			}
			if request.Status != domain.RequestStatusApproved || approval == nil || approval.ID != "approval-1" || approval.Status != domain.ApprovalStatusApproved || approval.ApprovedAt == nil || !approval.ApprovedAt.Equal(request.UpdatedAt) {
				t.Fatalf("ApproveWithApproval() = %#v, %#v", request, approval)
			}
		})
	}
}

func TestGetAndListAuditEventsHideUnauthorizedRequests(t *testing.T) {
	repo := newFakeRepository(pendingRequest())
	repo.auditEvents = []domain.AuditEvent{{RequestID: "request-1", ActorMemberID: "requester", Type: "request_submitted"}}
	service := NewService(repo)

	for _, testCase := range []struct {
		name    string
		actor   Actor
		wantErr error
	}{
		{name: "requester can read", actor: requester("requester")},
		{name: "assigned approver can read", actor: approver("approver")},
		{name: "assigned member without approver role receives not found", actor: Actor{MemberID: "approver"}, wantErr: domain.ErrNotFound},
		{name: "other member receives not found", actor: requester("other"), wantErr: domain.ErrNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			request, err := service.Get(context.Background(), testCase.actor, "request-1")
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("Get() error = %v, want %v", err, testCase.wantErr)
			}
			if testCase.wantErr == nil && request.ID != "request-1" {
				t.Errorf("Get() request ID = %q, want request-1", request.ID)
			}
			events, err := service.ListAuditEvents(context.Background(), testCase.actor, "request-1")
			if !errors.Is(err, testCase.wantErr) {
				t.Fatalf("ListAuditEvents() error = %v, want %v", err, testCase.wantErr)
			}
			if testCase.wantErr == nil && len(events) != 1 {
				t.Errorf("ListAuditEvents() count = %d, want 1", len(events))
			}
		})
	}
}

func TestAssignedApproverCannotReadApprovedRequestOrAudit(t *testing.T) {
	repo := newFakeRepository(pendingRequest())
	request := repo.requests["request-1"]
	request.Status = domain.RequestStatusApproved
	repo.requests[request.ID] = request
	repo.approval.Status = domain.ApprovalStatusApproved
	service := NewService(repo)
	for _, operation := range []string{"Get", "GetApproval", "ListAuditEvents"} {
		t.Run(operation, func(t *testing.T) {
			var err error
			switch operation {
			case "Get":
				_, err = service.Get(context.Background(), approver("approver"), request.ID)
			case "GetApproval":
				_, err = service.GetApproval(context.Background(), approver("approver"), request.ID)
			case "ListAuditEvents":
				_, err = service.ListAuditEvents(context.Background(), approver("approver"), request.ID)
			}
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("%s() error = %v, want ErrNotFound", operation, err)
			}
		})
	}
	if _, err := service.Get(context.Background(), requester("requester"), request.ID); err != nil {
		t.Fatalf("requester Get() error = %v", err)
	}
}

func TestGetHidesRequestWhenApprovalIsMissing(t *testing.T) {
	repo := newFakeRepository(pendingRequest())
	repo.nilApproval = true
	_, err := NewService(repo).Get(context.Background(), approver("approver"), "request-1")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("Get() error = %v, want ErrNotFound", err)
	}
}

type approveDuringReadRepository struct {
	*fakeRepository
	onApprovalRead bool
	onAuditRead    bool
}

func (r *approveDuringReadRepository) markApproved() {
	request := r.requests["request-1"]
	request.Status = domain.RequestStatusApproved
	r.requests[request.ID] = request
	r.approval.Status = domain.ApprovalStatusApproved
}

func (r *approveDuringReadRepository) GetApproval(ctx context.Context, id string) (*domain.Approval, error) {
	approval, err := r.fakeRepository.GetApproval(ctx, id)
	if r.onApprovalRead {
		r.markApproved()
	}
	return approval, err
}

func (r *approveDuringReadRepository) ListAuditEvents(ctx context.Context, id string) ([]domain.AuditEvent, error) {
	events, err := r.fakeRepository.ListAuditEvents(ctx, id)
	if r.onAuditRead {
		r.markApproved()
	}
	return events, err
}

func TestApproverReadDoesNotReturnDataAfterConcurrentApproval(t *testing.T) {
	for _, operation := range []string{"GetApproval", "ListAuditEvents"} {
		t.Run(operation, func(t *testing.T) {
			repo := &approveDuringReadRepository{fakeRepository: newFakeRepository(pendingRequest()), onApprovalRead: operation == "GetApproval", onAuditRead: operation == "ListAuditEvents"}
			service := NewService(repo)
			var err error
			if operation == "GetApproval" {
				_, err = service.GetApproval(context.Background(), approver("approver"), "request-1")
			} else {
				_, err = service.ListAuditEvents(context.Background(), approver("approver"), "request-1")
			}
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("%s() error = %v, want ErrNotFound", operation, err)
			}
		})
	}
}

func TestGetApprovalUsesRequestVisibility(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		actor   Actor
		wantID  string
		wantErr error
	}{
		{name: "requester", actor: requester("requester"), wantID: "approval-1"},
		{name: "assigned approver", actor: approver("approver"), wantID: "approval-1"},
		{name: "visible draft has no approval", actor: requester("requester")},
		{name: "other requester", actor: requester("other"), wantErr: domain.ErrNotFound},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo := newFakeRepository(pendingRequest())
			service := NewService(repo)
			requestID := "request-1"
			if testCase.name == "visible draft has no approval" {
				repo.requests[requestID] = draftRequest()
			}
			approval, err := service.GetApproval(context.Background(), testCase.actor, requestID)
			if testCase.name == "visible draft has no approval" {
				if err != nil || approval != nil {
					t.Fatalf("GetApproval() = %#v, error = %v, want nil approval", approval, err)
				}
				return
			}
			if testCase.wantErr != nil {
				if !errors.Is(err, testCase.wantErr) {
					t.Fatalf("GetApproval() error = %v, want %v", err, testCase.wantErr)
				}
				return
			}
			if err != nil || approval == nil || approval.ID != testCase.wantID || approval.Status != domain.ApprovalStatusPending {
				t.Fatalf("GetApproval() = %#v, error = %v", approval, err)
			}
		})
	}
}

func TestAdminCannotReadAnotherOrganizationRequestApprovalOrAudit(t *testing.T) {
	repo := newFakeRepository(pendingRequest())
	service := NewService(repo)
	actor := Actor{MemberID: "admin", OrganizationID: "other-organization", Roles: []domain.Role{domain.RoleAdmin}}
	for _, name := range []string{"Get", "GetApproval", "ListAuditEvents"} {
		t.Run(name, func(t *testing.T) {
			var err error
			switch name {
			case "Get":
				_, err = service.Get(context.Background(), actor, "request-1")
			case "GetApproval":
				_, err = service.GetApproval(context.Background(), actor, "request-1")
			case "ListAuditEvents":
				_, err = service.ListAuditEvents(context.Background(), actor, "request-1")
			}
			if !errors.Is(err, domain.ErrNotFound) {
				t.Fatalf("%s() error = %v, want ErrNotFound", name, err)
			}
		})
	}
}

func TestListPendingReturnsOnlyAssignedApproverRequests(t *testing.T) {
	repo := newFakeRepository(pendingRequest())
	service := NewService(repo)

	requests, err := service.ListPending(context.Background(), approver("approver"))
	if err != nil {
		t.Fatalf("ListPending() error = %v", err)
	}
	if len(requests) != 1 || requests[0].ID != "request-1" {
		t.Errorf("ListPending() = %#v, want request-1", requests)
	}
	_, err = service.ListPending(context.Background(), requester("requester"))
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("ListPending() error = %v, want ErrForbidden", err)
	}
}

func draftRequest() domain.Request {
	return domain.Request{ID: "request-1", OrganizationID: "organization", RequesterMemberID: "requester", Title: "VPN access", Status: domain.RequestStatusDraft, Version: 1}
}
func pendingRequest() domain.Request {
	request := draftRequest()
	request.Status = domain.RequestStatusPending
	request.Version = 2
	return request
}
func assertAuditSnapshot(t *testing.T, events []domain.AuditEvent, eventType, actorID, title, description string) {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("audit event count = %d, want 1", len(events))
	}
	event := events[0]
	if event.Type != eventType || event.ActorMemberID != actorID || event.ContentSnapshot == nil || event.ContentSnapshot.Title != title || event.ContentSnapshot.Description != description {
		t.Errorf("audit event = %#v, want type %q actor %q snapshot (%q, %q)", event, eventType, actorID, title, description)
	}
}

type fakeRepository struct {
	nilApproval             bool
	approvalErr             error
	requests                map[string]domain.Request
	approval                domain.Approval
	defaultApproverID       string
	defaultApproverErr      error
	defaultApproverEligible bool
	auditEvents             []domain.AuditEvent
}

func newFakeRepository(initial ...domain.Request) *fakeRepository {
	repo := &fakeRepository{requests: make(map[string]domain.Request), defaultApproverEligible: true}
	for _, request := range initial {
		repo.requests[request.ID] = request
		if request.Status == domain.RequestStatusPending {
			repo.approval = domain.Approval{ID: "approval-1", RequestID: request.ID, AssigneeMemberID: "approver", Status: domain.ApprovalStatusPending}
		}
	}
	return repo
}
func (r *fakeRepository) AuditEventCount() int { return len(r.auditEvents) }

func (r *fakeRepository) CreateDraft(_ context.Context, command CreateDraftCommand) (domain.Request, error) {
	command.Request.ID = "request-1"
	command.AuditEvent.RequestID = command.Request.ID
	r.requests[command.Request.ID] = command.Request
	r.auditEvents = append(r.auditEvents, command.AuditEvent)
	return command.Request, nil
}
func (r *fakeRepository) Get(_ context.Context, requestID string) (domain.Request, error) {
	request, ok := r.requests[requestID]
	if !ok {
		return domain.Request{}, domain.ErrNotFound
	}
	return request, nil
}
func (r *fakeRepository) GetApproval(_ context.Context, requestID string) (*domain.Approval, error) {
	if r.approvalErr != nil {
		return nil, r.approvalErr
	}
	if r.nilApproval {
		return nil, nil
	}
	if request, ok := r.requests[requestID]; !ok || request.Status == domain.RequestStatusDraft {
		return nil, domain.ErrNotFound
	}
	if r.approval.RequestID != requestID {
		return nil, domain.ErrNotFound
	}
	approval := r.approval
	return &approval, nil
}
func (r *fakeRepository) UpdateDraft(_ context.Context, command UpdateDraftCommand) (domain.Request, error) {
	request, ok := r.requests[command.Request.ID]
	if !ok {
		return domain.Request{}, domain.ErrNotFound
	}
	if request.Version != command.ExpectedVersion {
		return domain.Request{}, domain.ErrVersionConflict
	}
	if request.Status != domain.RequestStatusDraft {
		return domain.Request{}, domain.ErrInvalidState
	}
	command.Request.Version++
	r.requests[command.Request.ID] = command.Request
	r.auditEvents = append(r.auditEvents, command.AuditEvent)
	return command.Request, nil
}
func (r *fakeRepository) DefaultApprover(_ context.Context, _ string) (domain.DefaultApprover, error) {
	return domain.DefaultApprover{MemberID: r.defaultApproverID, HasApproverRole: r.defaultApproverEligible}, r.defaultApproverErr
}
func (r *fakeRepository) Submit(_ context.Context, command SubmitCommand) (domain.Request, error) {
	request, ok := r.requests[command.RequestID]
	if !ok {
		return domain.Request{}, domain.ErrNotFound
	}
	if request.Version != command.ExpectedVersion {
		return domain.Request{}, domain.ErrVersionConflict
	}
	if request.Status != domain.RequestStatusDraft {
		return domain.Request{}, domain.ErrInvalidState
	}
	request.Status = domain.RequestStatusPending
	request.Version++
	r.requests[request.ID] = request
	r.approval = domain.Approval{RequestID: request.ID, AssigneeMemberID: command.AssigneeMemberID, Status: domain.ApprovalStatusPending}
	r.auditEvents = append(r.auditEvents, command.AuditEvent)
	return request, nil
}
func (r *fakeRepository) Approve(_ context.Context, command ApproveCommand) (domain.Request, error) {
	request, ok := r.requests[command.RequestID]
	if !ok {
		return domain.Request{}, domain.ErrNotFound
	}
	if request.Version != command.ExpectedVersion {
		return domain.Request{}, domain.ErrVersionConflict
	}
	if request.Status != domain.RequestStatusPending {
		return domain.Request{}, domain.ErrInvalidState
	}
	request.Status = domain.RequestStatusApproved
	request.Version++
	r.requests[request.ID] = request
	r.approval.Status = domain.ApprovalStatusApproved
	r.auditEvents = append(r.auditEvents, command.AuditEvent)
	return request, nil
}
func (r *fakeRepository) ListPending(_ context.Context, assigneeID string) ([]domain.Request, error) {
	if r.approval.AssigneeMemberID != assigneeID || r.approval.Status != domain.ApprovalStatusPending {
		return nil, nil
	}
	return []domain.Request{r.requests[r.approval.RequestID]}, nil
}
func (r *fakeRepository) ListAuditEvents(_ context.Context, _ string) ([]domain.AuditEvent, error) {
	return r.auditEvents, nil
}
