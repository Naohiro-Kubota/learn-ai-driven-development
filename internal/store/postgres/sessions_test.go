package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
)

func TestMembersForIdentityReturnsMembershipsAcrossOrganizations(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE organization_selection_transaction_members, organization_selection_transactions, member_oidc_identities, oidc_identities, app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'), ('org-2', 'Two'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'legacy-1'), ('member-2', 'org-2', 'legacy-2'); INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject-1'); INSERT INTO member_oidc_identities (identity_id, member_id) VALUES ('identity-1', 'member-1'), ('identity-1', 'member-2')`); err != nil {
		t.Fatal(err)
	}
	got, err := NewRepository(db).MembersForIdentity(context.Background(), "https://issuer.example", "subject-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "member-1" || got[1] != "member-2" {
		t.Fatalf("members = %#v", got)
	}
}

func TestConsumeOrganizationSelectionRejectsCandidateOutsideSnapshot(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE organization_selection_transaction_members, organization_selection_transactions, member_oidc_identities, oidc_identities, app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'), ('org-2', 'Two'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'legacy-1'), ('member-2', 'org-2', 'legacy-2'); INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject-1')`); err != nil {
		t.Fatal(err)
	}
	r := NewRepository(db)
	now := time.Now().UTC()
	if err := r.CreateOrganizationSelection(context.Background(), auth.OrganizationSelectionInput{ID: "selection-1", Cookie: "selection-cookie", CSRFToken: "selection-csrf", IdentityID: "identity-1", MemberIDs: []string{"member-1"}, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	_, err := r.ConsumeOrganizationSelection(context.Background(), auth.ConsumeOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "selection-csrf", MemberID: "member-2", Now: now, Session: auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "session-csrf", MemberID: "member-2", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	_, err = r.ConsumeOrganizationSelection(context.Background(), auth.ConsumeOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "selection-csrf", MemberID: "member-1", Now: now, Session: auth.SessionInput{ID: "session-2", Cookie: "session-cookie-2", CSRFToken: "session-csrf-2", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}})
	if !errors.Is(err, auth.ErrConsumed) {
		t.Fatalf("replay err = %v, want consumed", err)
	}
}

func TestSessionAuthenticationHonorsRevocationAndExpiry(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'legacy-1')`); err != nil {
		t.Fatal(err)
	}
	r, now := NewRepository(db), time.Now().UTC()
	if err := r.CreateSession(context.Background(), auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "csrf", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AuthenticateSession(context.Background(), "session-cookie", now); err != nil {
		t.Fatal(err)
	}
	if err := r.RevokeSession(context.Background(), "session-cookie", now); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AuthenticateSession(context.Background(), "session-cookie", now); !errors.Is(err, auth.ErrNotFound) {
		t.Fatalf("revoked err = %v", err)
	}
}

func TestConsumeAuthTransactionConsumesReplay(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE oidc_auth_transactions RESTART IDENTITY`); err != nil {
		t.Fatal(err)
	}
	r, now := NewRepository(db), time.Now().UTC()
	if err := r.CreateAuthTransaction(context.Background(), auth.AuthTransaction{ID: "auth-1", Cookie: "auth-cookie", State: "state", Nonce: "nonce", EncryptedVerifier: []byte("encrypted"), Issuer: "https://issuer.example", ClientID: "client", RedirectURI: "https://app.example/callback", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	tx, err := r.ConsumeAuthTransaction(context.Background(), "auth-cookie", "state", now)
	if err != nil {
		t.Fatal(err)
	}
	if tx.Nonce != "nonce" || string(tx.EncryptedVerifier) != "encrypted" {
		t.Fatalf("transaction = %#v", tx)
	}
	_, err = r.ConsumeAuthTransaction(context.Background(), "auth-cookie", "state", now)
	if !errors.Is(err, auth.ErrConsumed) {
		t.Fatalf("replay err = %v", err)
	}
}
