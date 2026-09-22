package postgres

import (
	"context"
	"errors"
	"sync"
	"testing"

	apprequests "github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

func TestCreateAndUpdateDraftPersistAuditHistory(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()

	created, err := repository.CreateDraft(ctx, createDraftCommand(newDraft(seed), seed.requesterID))
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Status != domain.RequestStatusDraft || created.Version != 1 {
		t.Fatalf("CreateDraft() = %#v", created)
	}
	created.Title = "Updated travel request"
	updated, err := repository.UpdateDraft(ctx, apprequests.UpdateDraftCommand{Request: created, ExpectedVersion: 1, AuditEvent: newAuditEvent(created, seed.requesterID, "request_updated")})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "Updated travel request" || updated.Version != 2 {
		t.Fatalf("UpdateDraft() = %#v", updated)
	}
	assertAuditEventCount(t, db, created.ID, 2)
	events, err := repository.ListAuditEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	eventsByType := make(map[string]domain.AuditEvent, len(events))
	for _, event := range events {
		eventsByType[event.Type] = event
	}
	if len(events) != 2 || eventsByType["request_created"].ContentSnapshot == nil || eventsByType["request_updated"].ContentSnapshot == nil || eventsByType["request_created"].ContentSnapshot.Title != "Travel request" || eventsByType["request_updated"].ContentSnapshot.Title != "Updated travel request" {
		t.Fatalf("ListAuditEvents() = %#v", events)
	}
}

func TestApprovalAndAuditReadModelPreservesOpaqueIDsAndMetadata(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()
	created, err := repository.CreateDraft(ctx, createDraftCommand(newDraft(seed), seed.requesterID))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Submit(ctx, submitCommand(created, seed))
	if err != nil {
		t.Fatal(err)
	}
	approval, err := repository.GetApproval(ctx, pending.ID)
	if err != nil || approval == nil || approval.ID == "" {
		t.Fatalf("GetApproval() = %#v, error = %v", approval, err)
	}
	if _, err := repository.Approve(ctx, approveCommand(pending, seed)); err != nil {
		t.Fatal(err)
	}
	events, err := repository.ListAuditEvents(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 {
		t.Fatalf("ListAuditEvents() count = %d, want 3", len(events))
	}
	for i, event := range events {
		if event.ID == "" || event.ContentSnapshot == nil {
			t.Fatalf("event[%d] = %#v, want opaque ID and snapshot", i, event)
		}
		if i > 0 && (events[i-1].OccurredAt.After(event.OccurredAt) || (events[i-1].OccurredAt.Equal(event.OccurredAt) && events[i-1].ID >= event.ID)) {
			t.Fatalf("events are not ordered by (occurred_at,id): %#v", events)
		}
		if event.ContentSnapshot.Description != "" {
			t.Fatalf("event[%d] description = %q, want empty snapshot", i, event.ContentSnapshot.Description)
		}
	}
	eventsByType := make(map[string]domain.AuditEvent, len(events))
	for _, event := range events {
		if _, exists := eventsByType[event.Type]; exists {
			t.Fatalf("duplicate audit event type %q", event.Type)
		}
		eventsByType[event.Type] = event
	}
	if event := eventsByType["request_created"]; event.ApprovalMetadata != nil {
		t.Fatalf("create metadata = %#v, want nil", event.ApprovalMetadata)
	}
	for _, eventType := range []string{"request_submitted", "request_approved"} {
		event, exists := eventsByType[eventType]
		if !exists || event.ApprovalMetadata == nil || event.ApprovalMetadata.ApprovalID != approval.ID || event.ApprovalMetadata.AssigneeMemberID != seed.approverID {
			t.Fatalf("%s metadata = %#v", eventType, event.ApprovalMetadata)
		}
	}
}

func TestDefaultApproverReturnsConfiguredMemberAndRole(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)

	approver, err := repository.DefaultApprover(context.Background(), seed.organizationID)
	if err != nil {
		t.Fatal(err)
	}
	if approver.MemberID != seed.approverID || !approver.HasApproverRole {
		t.Fatalf("DefaultApprover() = %#v", approver)
	}
	if _, err := db.Exec(`DELETE FROM member_roles WHERE member_id = $1 AND role = 'approver'`, seed.approverID); err != nil {
		t.Fatal(err)
	}
	approver, err = repository.DefaultApprover(context.Background(), seed.organizationID)
	if err != nil {
		t.Fatal(err)
	}
	if approver.MemberID != seed.approverID || approver.HasApproverRole {
		t.Fatalf("DefaultApprover() after role removal = %#v", approver)
	}
	if _, err := db.Exec(`UPDATE organizations SET default_approver_member_id = NULL WHERE id = $1`, seed.organizationID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.DefaultApprover(context.Background(), seed.organizationID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("DefaultApprover() without configured member error = %v, want not found", err)
	}
}

func TestSubmitRejectsWrongIDVersionOrStateWithoutAuditEvent(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()
	created, err := repository.CreateDraft(ctx, createDraftCommand(newDraft(seed), seed.requesterID))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name    string
		command apprequests.SubmitCommand
		want    error
	}{
		{"unknown ID", apprequests.SubmitCommand{RequestID: "missing", RequesterMemberID: seed.requesterID, AssigneeMemberID: seed.approverID, ExpectedVersion: 1, AuditEvent: newAuditEvent(created, seed.requesterID, "request_submitted")}, domain.ErrNotFound},
		{"stale version", apprequests.SubmitCommand{RequestID: created.ID, RequesterMemberID: seed.requesterID, AssigneeMemberID: seed.approverID, ExpectedVersion: 2, AuditEvent: newAuditEvent(created, seed.requesterID, "request_submitted")}, domain.ErrVersionConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := repository.Submit(ctx, test.command)
			if !errors.Is(err, test.want) {
				t.Fatalf("Submit() error = %v, want %v", err, test.want)
			}
			assertAuditEventCount(t, db, created.ID, 1)
		})
	}
	pending, err := repository.Submit(ctx, submitCommand(created, seed))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Submit(ctx, submitCommand(pending, seed)); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("Submit() from pending error = %v, want invalid state", err)
	}
	assertAuditEventCount(t, db, created.ID, 2)
}

func TestConcurrentSubmitAllowsOneTransitionAndOneAuditEvent(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()
	created, err := repository.CreateDraft(ctx, createDraftCommand(newDraft(seed), seed.requesterID))
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for range 2 {
		go func() {
			start.Wait()
			_, err := repository.Submit(ctx, submitCommand(created, seed))
			results <- err
		}()
	}
	start.Done()
	assertOneSuccessOneConflict(t, results)
	assertAuditEventCount(t, db, created.ID, 2)
}

func TestSubmitAndApproveRollBackWhenAuditEventCannotPersist(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()

	t.Run("submit", func(t *testing.T) {
		created, err := repository.CreateDraft(ctx, createDraftCommand(newDraft(seed), seed.requesterID))
		if err != nil {
			t.Fatal(err)
		}
		command := submitCommand(created, seed)
		command.AuditEvent.ActorMemberID = "missing-member"
		if _, err := repository.Submit(ctx, command); err == nil {
			t.Fatal("Submit() unexpectedly succeeded")
		}
		request, err := repository.Get(ctx, created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if request.Status != domain.RequestStatusDraft || request.Version != 1 {
			t.Fatalf("request after failed Submit() = %#v", request)
		}
		if _, err := repository.GetApproval(ctx, created.ID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("GetApproval() after failed Submit() error = %v, want not found", err)
		}
		assertAuditEventCount(t, db, created.ID, 1)
	})

	t.Run("approve", func(t *testing.T) {
		pending := seedPendingRequest(t, ctx, repository, seed)
		command := approveCommand(pending, seed)
		command.AuditEvent.ActorMemberID = "missing-member"
		if _, err := repository.Approve(ctx, command); err == nil {
			t.Fatal("Approve() unexpectedly succeeded")
		}
		request, err := repository.Get(ctx, pending.ID)
		if err != nil {
			t.Fatal(err)
		}
		if request.Status != domain.RequestStatusPending || request.Version != pending.Version {
			t.Fatalf("request after failed Approve() = %#v", request)
		}
		approval, err := repository.GetApproval(ctx, pending.ID)
		if err != nil {
			t.Fatal(err)
		}
		if approval.Status != domain.ApprovalStatusPending {
			t.Fatalf("approval after failed Approve() = %#v", approval)
		}
		assertAuditEventCount(t, db, pending.ID, 2)
	})
}

func TestApproveIsAtomicWithAuditEventAndRejectsUnassignedApprover(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()
	pending := seedPendingRequest(t, ctx, repository, seed)

	_, err := repository.Approve(ctx, apprequests.ApproveCommand{RequestID: pending.ID, AssigneeMemberID: seed.adminID, ExpectedVersion: pending.Version, AuditEvent: newAuditEvent(pending, seed.adminID, "request_approved")})
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("unassigned Approve() error = %v, want forbidden", err)
	}
	assertAuditEventCount(t, db, pending.ID, 2)
	approved, err := repository.Approve(ctx, approveCommand(pending, seed))
	if err != nil {
		t.Fatal(err)
	}
	if approved.Status != domain.RequestStatusApproved || approved.Version != pending.Version+1 {
		t.Fatalf("Approve() = %#v", approved)
	}
	assertAuditEventCount(t, db, pending.ID, 3)
}

func TestApproveRejectsWrongIDVersionOrStateWithoutAuditEvent(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()
	pending := seedPendingRequest(t, ctx, repository, seed)

	for _, test := range []struct {
		name    string
		command apprequests.ApproveCommand
		want    error
	}{
		{"unknown ID", apprequests.ApproveCommand{RequestID: "missing", AssigneeMemberID: seed.approverID, ExpectedVersion: pending.Version, AuditEvent: newAuditEvent(pending, seed.approverID, "request_approved")}, domain.ErrNotFound},
		{"stale version", apprequests.ApproveCommand{RequestID: pending.ID, AssigneeMemberID: seed.approverID, ExpectedVersion: pending.Version + 1, AuditEvent: newAuditEvent(pending, seed.approverID, "request_approved")}, domain.ErrVersionConflict},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := repository.Approve(ctx, test.command)
			if !errors.Is(err, test.want) {
				t.Fatalf("Approve() error = %v, want %v", err, test.want)
			}
			assertAuditEventCount(t, db, pending.ID, 2)
		})
	}
	approved, err := repository.Approve(ctx, approveCommand(pending, seed))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Approve(ctx, approveCommand(approved, seed)); !errors.Is(err, domain.ErrInvalidState) {
		t.Fatalf("Approve() from approved error = %v, want invalid state", err)
	}
	assertAuditEventCount(t, db, pending.ID, 3)
}

func TestConcurrentApproveAllowsOneTransitionAndOneAuditEvent(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	ctx := context.Background()
	pending := seedPendingRequest(t, ctx, repository, seed)
	results := make(chan error, 2)
	var start sync.WaitGroup
	start.Add(1)
	for range 2 {
		go func() {
			start.Wait()
			_, err := repository.Approve(ctx, approveCommand(pending, seed))
			results <- err
		}()
	}
	start.Done()
	assertOneSuccessOneConflict(t, results)
	assertAuditEventCount(t, db, pending.ID, 3)
}

func TestListPendingReturnsOnlyAssignedPendingRequests(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seed := seedWorkflow(t, db)
	repository := NewRepository(db)
	pending := seedPendingRequest(t, context.Background(), repository, seed)

	requests, err := repository.ListPending(context.Background(), seed.approverID)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0].ID != pending.ID {
		t.Fatalf("ListPending() = %#v", requests)
	}
	requests, err = repository.ListPending(context.Background(), seed.adminID)
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 0 {
		t.Fatalf("unassigned ListPending() = %#v", requests)
	}
}

func assertOneSuccessOneConflict(t *testing.T, results <-chan error) {
	t.Helper()
	var successes, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, domain.ErrVersionConflict) {
			conflicts++
		} else {
			t.Fatalf("operation error = %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("successes = %d, conflicts = %d", successes, conflicts)
	}
}
