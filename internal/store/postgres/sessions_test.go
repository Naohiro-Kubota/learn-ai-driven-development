package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

func TestSessionPrincipalIsReadFromMemberRoles(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'subject-1'); INSERT INTO member_roles (member_id, role) VALUES ('member-1', 'requester'), ('member-1', 'approver')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	r := NewRepository(db)
	if err := r.CreateSession(context.Background(), auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "csrf", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	session, err := r.Authenticate(context.Background(), "session-cookie", now)
	if err != nil {
		t.Fatal(err)
	}
	if session.ID != "session-1" || session.Principal.MemberID != "member-1" {
		t.Fatalf("session = %#v", session)
	}
	if len(session.Principal.Roles) != 2 || session.Principal.Roles[0] != domain.RoleApprover || session.Principal.Roles[1] != domain.RoleRequester {
		t.Fatalf("roles = %#v", session.Principal.Roles)
	}
}

func TestIssueCSRFTokenReplacesStoredHash(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'subject-1')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	r := NewRepository(db)
	if err := r.CreateSession(context.Background(), auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "initial-csrf", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}); err != nil {
		t.Fatal(err)
	}

	first, err := r.IssueCSRFToken(context.Background(), "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := r.IssueCSRFToken(context.Background(), "session-1", now)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("CSRF token was reused")
	}
	assertSessionCSRFHash(t, db, "session-1", sha256.Sum256([]byte(second)))
}

func TestAuthenticateRejectsRevokedAndExpiredSessions(t *testing.T) {
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		session auth.SessionInput
		revoke  bool
	}{
		{name: "revoked", session: testSessionInput(now), revoke: true},
		{name: "idle expired", session: sessionInputWithExpiry(now.Add(-time.Second), now.Add(time.Hour))},
		{name: "absolute expired", session: sessionInputWithExpiry(now.Add(time.Hour), now.Add(-time.Second))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openWorkflowTestDatabase(t)
			seedSessionDatabase(t, db)
			r := NewRepository(db)
			if err := r.CreateSession(context.Background(), tc.session); err != nil {
				t.Fatal(err)
			}
			if tc.revoke {
				if err := r.Revoke(context.Background(), tc.session.Cookie, now); err != nil {
					t.Fatal(err)
				}
			}
			_, err := r.Authenticate(context.Background(), tc.session.Cookie, now)
			if !errors.Is(err, auth.ErrNotFound) {
				t.Fatalf("err = %v, want not found", err)
			}
		})
	}
}

func TestIssueCSRFTokenRejectsInactiveSessionsWithoutChangingStoredHash(t *testing.T) {
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name        string
		session     *auth.SessionInput
		revoke      bool
		wantRecords int
	}{
		{name: "missing", wantRecords: 0},
		{name: "revoked", session: pointerTo(testSessionInput(now)), revoke: true, wantRecords: 1},
		{name: "idle expired", session: pointerTo(sessionInputWithExpiry(now.Add(-time.Second), now.Add(time.Hour))), wantRecords: 1},
		{name: "absolute expired", session: pointerTo(sessionInputWithExpiry(now.Add(time.Hour), now.Add(-time.Second))), wantRecords: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openWorkflowTestDatabase(t)
			seedSessionDatabase(t, db)
			r := NewRepository(db)
			if tc.session != nil {
				if err := r.CreateSession(context.Background(), *tc.session); err != nil {
					t.Fatal(err)
				}
				if tc.revoke {
					if err := r.Revoke(context.Background(), tc.session.Cookie, now); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, err := r.IssueCSRFToken(context.Background(), "session-1", now)
			if !errors.Is(err, auth.ErrNotFound) {
				t.Fatalf("err = %v, want not found", err)
			}
			assertAppSessionCount(t, db, tc.wantRecords)
			if tc.session != nil {
				assertSessionCSRFHash(t, db, tc.session.ID, sha256.Sum256([]byte(tc.session.CSRFToken)))
			}
		})
	}
}

func TestOrganizationSelectionReadAndIssueCSRFTokenReturnsSnapshot(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE organization_selection_transaction_members, organization_selection_transactions, member_oidc_identities, oidc_identities, app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'), ('org-2', 'Two'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'subject-1'), ('member-2', 'org-2', 'subject-2'); INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject'); INSERT INTO member_oidc_identities (identity_id, member_id) VALUES ('identity-1', 'member-1'), ('identity-1', 'member-2')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	r := NewRepository(db)
	if err := r.CreateOrganizationSelection(context.Background(), auth.OrganizationSelectionInput{ID: "selection-1", Cookie: "selection-cookie", CSRFToken: "initial-csrf", IdentityID: "identity-1", MemberIDs: []string{"member-1", "member-2"}, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}

	selection, err := r.ReadAndIssueCSRFToken(context.Background(), "selection-cookie", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(selection.Candidates) != 2 || selection.Candidates[0] != (auth.OrganizationSelectionCandidate{MemberID: "member-1", OrganizationID: "org-1", OrganizationName: "One"}) || selection.Candidates[1] != (auth.OrganizationSelectionCandidate{MemberID: "member-2", OrganizationID: "org-2", OrganizationName: "Two"}) {
		t.Fatalf("candidates = %#v", selection.Candidates)
	}
	if selection.CSRFToken == "" {
		t.Fatal("selection CSRF token is empty")
	}
	assertSelectionCSRFHash(t, db, "selection-1", sha256.Sum256([]byte(selection.CSRFToken)))
}

func TestOrganizationSelectionCompletionCreatesSession(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE organization_selection_transaction_members, organization_selection_transactions, member_oidc_identities, oidc_identities, app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'subject-1'); INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject'); INSERT INTO member_oidc_identities (identity_id, member_id) VALUES ('identity-1', 'member-1')`); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	r := NewRepository(db)
	if err := r.CreateOrganizationSelection(context.Background(), auth.OrganizationSelectionInput{ID: "selection-1", Cookie: "selection-cookie", CSRFToken: "selection-csrf", IdentityID: "identity-1", MemberIDs: []string{"member-1"}, ExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	sessionInput := auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "session-csrf", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}

	if err := r.CompleteOrganizationSelection(context.Background(), auth.CompleteOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "selection-csrf", MemberID: "member-1", Now: now}, sessionInput); err != nil {
		t.Fatal(err)
	}
	assertSessionCSRFHash(t, db, "session-1", sha256.Sum256([]byte("session-csrf")))
	var consumedAt time.Time
	if err := db.QueryRow(`SELECT consumed_at FROM organization_selection_transactions WHERE id = 'selection-1'`).Scan(&consumedAt); err != nil {
		t.Fatal(err)
	}
}

func TestOrganizationSelectionReadRejectsMissingExpiredAndConsumed(t *testing.T) {
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name    string
		seed    func(*testing.T, *Repository, time.Time)
		wantErr error
	}{
		{name: "missing", wantErr: auth.ErrNotFound},
		{
			name: "expired",
			seed: func(t *testing.T, r *Repository, now time.Time) {
				seedSelection(t, r, "selection-cookie", "selection-csrf", now.Add(-time.Second))
			},
			wantErr: auth.ErrExpired,
		},
		{
			name: "consumed",
			seed: func(t *testing.T, r *Repository, now time.Time) {
				seedSelection(t, r, "selection-cookie", "selection-csrf", now.Add(time.Hour))
				if _, err := r.db.Exec(`UPDATE organization_selection_transactions SET consumed_at = $1 WHERE id = 'selection-1'`, now); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: auth.ErrConsumed,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openWorkflowTestDatabase(t)
			seedSelectionDatabase(t, db)
			r := NewRepository(db)
			if tc.seed != nil {
				tc.seed(t, r, now)
			}
			_, err := r.ReadAndIssueCSRFToken(context.Background(), "selection-cookie", now)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestOrganizationSelectionCompletionConsumesIncorrectCSRFWithoutSession(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seedSelectionDatabase(t, db)
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	r := NewRepository(db)
	seedSelection(t, r, "selection-cookie", "correct-csrf", now.Add(time.Hour))
	session := auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "session-csrf", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(2 * time.Hour)}

	err := r.CompleteOrganizationSelection(context.Background(), auth.CompleteOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "incorrect-csrf", MemberID: "member-1", Now: now}, session)
	if !errors.Is(err, auth.ErrCSRFValidation) || errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("err = %v, want CSRF validation failure distinct from forbidden", err)
	}
	assertSelectionConsumed(t, db, "selection-1")
	assertAppSessionCount(t, db, 0)
	err = r.CompleteOrganizationSelection(context.Background(), auth.CompleteOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "correct-csrf", MemberID: "member-1", Now: now}, session)
	if !errors.Is(err, auth.ErrConsumed) {
		t.Fatalf("replay err = %v, want consumed", err)
	}
	assertAppSessionCount(t, db, 0)
}

func seedSelectionDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE organization_selection_transaction_members, organization_selection_transactions, member_oidc_identities, oidc_identities, app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'subject-1'); INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject'); INSERT INTO member_oidc_identities (identity_id, member_id) VALUES ('identity-1', 'member-1')`); err != nil {
		t.Fatal(err)
	}
}

func seedSelection(t *testing.T, r *Repository, cookie, csrfToken string, expiresAt time.Time) {
	t.Helper()
	if err := r.CreateOrganizationSelection(context.Background(), auth.OrganizationSelectionInput{ID: "selection-1", Cookie: cookie, CSRFToken: csrfToken, IdentityID: "identity-1", MemberIDs: []string{"member-1"}, ExpiresAt: expiresAt}); err != nil {
		t.Fatal(err)
	}
}

func assertSelectionConsumed(t *testing.T, db *sql.DB, selectionID string) {
	t.Helper()
	var consumedAt time.Time
	if err := db.QueryRow(`SELECT consumed_at FROM organization_selection_transactions WHERE id = $1`, selectionID).Scan(&consumedAt); err != nil {
		t.Fatal(err)
	}
}

func assertAppSessionCount(t *testing.T, db *sql.DB, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(`SELECT count(*) FROM app_sessions`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("app sessions = %d, want %d", got, want)
	}
}

func assertSessionCSRFHash(t *testing.T, db interface{ QueryRow(string, ...any) *sql.Row }, sessionID string, want [sha256.Size]byte) {
	t.Helper()
	var got []byte
	if err := db.QueryRow(`SELECT csrf_token_hash FROM app_sessions WHERE id = $1`, sessionID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want[:]) {
		t.Fatalf("CSRF hash = %x, want %x", got, want)
	}
}

func assertSelectionCSRFHash(t *testing.T, db interface{ QueryRow(string, ...any) *sql.Row }, selectionID string, want [sha256.Size]byte) {
	t.Helper()
	var got []byte
	if err := db.QueryRow(`SELECT csrf_token_hash FROM organization_selection_transactions WHERE id = $1`, selectionID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want[:]) {
		t.Fatalf("CSRF hash = %x, want %x", got, want)
	}
}

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

func TestOrganizationSelectionCompletionRejectsCandidateOutsideSnapshot(t *testing.T) {
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
	err := r.CompleteOrganizationSelection(context.Background(), auth.CompleteOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "selection-csrf", MemberID: "member-2", Now: now}, auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "session-csrf", MemberID: "member-2", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	assertAppSessionCount(t, db, 0)
	err = r.CompleteOrganizationSelection(context.Background(), auth.CompleteOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "selection-csrf", MemberID: "member-1", Now: now}, auth.SessionInput{ID: "session-2", Cookie: "session-cookie-2", CSRFToken: "session-csrf-2", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)})
	if !errors.Is(err, auth.ErrConsumed) {
		t.Fatalf("replay err = %v, want consumed", err)
	}
}

func seedSessionDatabase(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`TRUNCATE app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'subject-1')`); err != nil {
		t.Fatal(err)
	}
}

func testSessionInput(now time.Time) auth.SessionInput {
	return sessionInputWithExpiry(now.Add(time.Hour), now.Add(2*time.Hour))
}

func sessionInputWithExpiry(idleExpiresAt, absoluteExpiresAt time.Time) auth.SessionInput {
	return auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "initial-csrf", MemberID: "member-1", CreatedAt: time.Date(2026, time.September, 22, 8, 0, 0, 0, time.UTC), IdleExpiresAt: idleExpiresAt, AbsoluteExpiresAt: absoluteExpiresAt}
}

func pointerTo[T any](value T) *T {
	return &value
}

func TestConsumeOrganizationSelectionRejectsDifferentIssuedMember(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE organization_selection_transaction_members, organization_selection_transactions, member_oidc_identities, oidc_identities, app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'), ('org-2', 'Two'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'legacy-1'), ('member-2', 'org-2', 'legacy-2'); INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject-1')`); err != nil {
		t.Fatal(err)
	}
	r, now := NewRepository(db), time.Now().UTC()
	if err := r.CreateOrganizationSelection(context.Background(), auth.OrganizationSelectionInput{ID: "selection-1", Cookie: "selection-cookie", CSRFToken: "selection-csrf", IdentityID: "identity-1", MemberIDs: []string{"member-1"}, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	_, err := r.ConsumeOrganizationSelection(context.Background(), auth.ConsumeOrganizationSelectionInput{Cookie: "selection-cookie", CSRFToken: "selection-csrf", MemberID: "member-1", Now: now, Session: auth.SessionInput{ID: "session-1", Cookie: "session-cookie", CSRFToken: "session-csrf", MemberID: "member-2", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}})
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("err = %v, want forbidden", err)
	}
	var sessions int
	if err := db.QueryRow(`SELECT count(*) FROM app_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatalf("sessions = %d, want 0", sessions)
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

func TestConsumeAuthTransactionConsumesStateMismatch(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE oidc_auth_transactions RESTART IDENTITY`); err != nil {
		t.Fatal(err)
	}
	r, now := NewRepository(db), time.Now().UTC()
	if err := r.CreateAuthTransaction(context.Background(), auth.AuthTransaction{ID: "auth-record", Cookie: "auth-cookie", State: "correct-state", Nonce: "nonce", EncryptedVerifier: []byte("encrypted"), Issuer: "https://issuer.example", ClientID: "client", RedirectURI: "https://app.example/callback", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	_, err := r.ConsumeAuthTransaction(context.Background(), "auth-cookie", "incorrect-state", now)
	if !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("mismatch err = %v, want forbidden", err)
	}
	_, err = r.ConsumeAuthTransaction(context.Background(), "auth-cookie", "correct-state", now)
	if !errors.Is(err, auth.ErrConsumed) {
		t.Fatalf("replay err = %v, want consumed", err)
	}
}

func TestRepositoryNeverStoresRawBrowserSecretsInIDColumns(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	if _, err := db.Exec(`TRUNCATE organization_selection_transaction_members, organization_selection_transactions, oidc_auth_transactions, oidc_identities, app_sessions, member_roles, members, organizations RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO organizations (id, name) VALUES ('org-1', 'One'); INSERT INTO members (id, organization_id, oidc_subject) VALUES ('member-1', 'org-1', 'legacy-1'); INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject-1')`); err != nil {
		t.Fatal(err)
	}
	r, now := NewRepository(db), time.Now().UTC()
	if err := r.CreateAuthTransaction(context.Background(), auth.AuthTransaction{ID: "auth-record", Cookie: "auth-cookie", State: "auth-state", Nonce: "nonce", EncryptedVerifier: []byte("encrypted"), Issuer: "https://issuer.example", ClientID: "client", RedirectURI: "https://app.example/callback", ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateOrganizationSelection(context.Background(), auth.OrganizationSelectionInput{ID: "selection-record", Cookie: "selection-cookie", CSRFToken: "selection-csrf", IdentityID: "identity-1", MemberIDs: []string{"member-1"}, ExpiresAt: now.Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if err := r.CreateSession(context.Background(), auth.SessionInput{ID: "session-record", Cookie: "session-cookie", CSRFToken: "session-csrf", MemberID: "member-1", CreatedAt: now, IdleExpiresAt: now.Add(time.Hour), AbsoluteExpiresAt: now.Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{`SELECT id FROM oidc_auth_transactions`, `SELECT id FROM organization_selection_transactions`, `SELECT id FROM app_sessions`} {
		var id string
		if err := db.QueryRow(query).Scan(&id); err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"auth-cookie", "auth-state", "selection-cookie", "selection-csrf", "session-cookie", "session-csrf"} {
			if id == secret {
				t.Fatalf("id %q stores raw secret", id)
			}
		}
	}
}
