package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
)

func TestValidateCSRFTokenUsesCurrentSessionHash(t *testing.T) {
	db := openWorkflowTestDatabase(t)
	seedSessionDatabase(t, db)
	repository := NewRepository(db)
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	session := testSessionInput(now)
	ctx := context.Background()
	if err := repository.CreateSession(ctx, session); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.Authenticate(ctx, session.Cookie, now); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateCSRFToken(ctx, session.Cookie, session.CSRFToken, now); err != nil {
		t.Fatal(err)
	}
	token, err := repository.IssueCSRFToken(ctx, session.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateCSRFToken(ctx, session.Cookie, session.CSRFToken, now); !errors.Is(err, auth.ErrCSRFValidation) {
		t.Fatalf("old token error = %v, want CSRF validation failure", err)
	}
	if err := repository.ValidateCSRFToken(ctx, session.Cookie, token, now); err != nil {
		t.Fatalf("current token error = %v", err)
	}
}

func TestValidateCSRFTokenRequiresActiveCookieBoundSession(t *testing.T) {
	now := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name   string
		cookie string
		token  string
		now    time.Time
		revoke bool
		want   error
	}{
		{"wrong cookie", "other-cookie", "initial-csrf", now, false, auth.ErrNotFound},
		{"empty cookie", "", "initial-csrf", now, false, auth.ErrNotFound},
		{"wrong token", "session-cookie", "wrong-token", now, false, auth.ErrCSRFValidation},
		{"empty token", "session-cookie", "", now, false, auth.ErrCSRFValidation},
		{"revoked", "session-cookie", "initial-csrf", now, true, auth.ErrNotFound},
		{"idle expiry boundary", "session-cookie", "initial-csrf", now.Add(time.Hour), false, auth.ErrNotFound},
		{"absolute expiry boundary", "session-cookie", "initial-csrf", now.Add(2 * time.Hour), false, auth.ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db := openWorkflowTestDatabase(t)
			seedSessionDatabase(t, db)
			repository := NewRepository(db)
			session := testSessionInput(now)
			if tc.name == "absolute expiry boundary" {
				session.IdleExpiresAt = now.Add(3 * time.Hour)
			}
			ctx := context.Background()
			if err := repository.CreateSession(ctx, session); err != nil {
				t.Fatal(err)
			}
			if tc.revoke {
				if err := repository.Revoke(ctx, session.Cookie, now); err != nil {
					t.Fatal(err)
				}
			}
			if err := repository.ValidateCSRFToken(ctx, tc.cookie, tc.token, tc.now); !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
		})
	}
}
