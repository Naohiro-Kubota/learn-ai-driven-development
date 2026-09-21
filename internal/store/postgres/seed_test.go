package postgres

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"

	apprequests "github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

type workflowSeed struct {
	organizationID string
	requesterID    string
	approverID     string
	adminID        string
}

func openWorkflowTestDatabase(t *testing.T) *sql.DB {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Fatal("TEST_DATABASE_URL is required")
	}
	db, err := sql.Open("pgx", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Ping(); err != nil {
		t.Fatal(err)
	}
	m, err := migrate.New("file://../../../migrations", url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = m.Close()
	})
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		t.Fatal(err)
	}
	return db
}

func seedWorkflow(t *testing.T, db *sql.DB) workflowSeed {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE audit_events, approvals, requests, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	seed := workflowSeed{organizationID: "org-1", requesterID: "requester-1", approverID: "approver-1", adminID: "admin-1"}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ($1, $2)`, seed.organizationID, "Test organization"); err != nil {
		t.Fatal(err)
	}
	for _, member := range []struct {
		id, subject string
		role        domain.Role
	}{
		{seed.requesterID, "requester-subject", domain.RoleRequester},
		{seed.approverID, "approver-subject", domain.RoleApprover},
		{seed.adminID, "admin-subject", domain.RoleAdmin},
	} {
		if _, err := db.Exec(`INSERT INTO members (id, organization_id, oidc_subject) VALUES ($1, $2, $3)`, member.id, seed.organizationID, member.subject); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO member_roles (member_id, role) VALUES ($1, $2)`, member.id, member.role); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec(`UPDATE organizations SET default_approver_member_id = $1 WHERE id = $2`, seed.approverID, seed.organizationID); err != nil {
		t.Fatal(err)
	}
	return seed
}

func newDraft(seed workflowSeed) domain.Request {
	now := time.Date(2026, time.September, 21, 12, 0, 0, 0, time.UTC)
	return domain.Request{OrganizationID: seed.organizationID, RequesterMemberID: seed.requesterID, Title: "Travel request", Description: "", Status: domain.RequestStatusDraft, Version: 1, CreatedAt: now, UpdatedAt: now}
}

func newAuditEvent(request domain.Request, actorID, eventType string) domain.AuditEvent {
	return domain.AuditEvent{RequestID: request.ID, ActorMemberID: actorID, Type: eventType, OccurredAt: request.UpdatedAt, ContentSnapshot: domain.ContentSnapshot{Title: request.Title, Description: request.Description}}
}

func createDraftCommand(request domain.Request, actorID string) apprequests.CreateDraftCommand {
	return apprequests.CreateDraftCommand{Request: request, AuditEvent: newAuditEvent(request, actorID, "request_created")}
}

func submitCommand(request domain.Request, seed workflowSeed) apprequests.SubmitCommand {
	return apprequests.SubmitCommand{RequestID: request.ID, RequesterMemberID: seed.requesterID, AssigneeMemberID: seed.approverID, ExpectedVersion: request.Version, AuditEvent: newAuditEvent(request, seed.requesterID, "request_submitted")}
}

func approveCommand(request domain.Request, seed workflowSeed) apprequests.ApproveCommand {
	return apprequests.ApproveCommand{RequestID: request.ID, AssigneeMemberID: seed.approverID, ExpectedVersion: request.Version, AuditEvent: newAuditEvent(request, seed.approverID, "request_approved")}
}

func seedPendingRequest(t *testing.T, ctx context.Context, repository *Repository, seed workflowSeed) domain.Request {
	t.Helper()
	created, err := repository.CreateDraft(ctx, createDraftCommand(newDraft(seed), seed.requesterID))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := repository.Submit(ctx, submitCommand(created, seed))
	if err != nil {
		t.Fatal(err)
	}
	return pending
}

func assertAuditEventCount(t *testing.T, db *sql.DB, requestID string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT count(*) FROM audit_events WHERE request_id = $1`, requestID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("audit event count = %d, want %d", got, want)
	}
}
