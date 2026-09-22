package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/application/requests"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/domain"
)

type fakeSessionStore struct {
	authenticate func(context.Context, string, time.Time) (auth.AuthenticatedSession, error)
	issue        func(context.Context, string, time.Time) (string, error)
	revoke       func(context.Context, string, time.Time) error
}

func (f fakeSessionStore) Authenticate(ctx context.Context, cookie string, now time.Time) (auth.AuthenticatedSession, error) {
	return f.authenticate(ctx, cookie, now)
}
func (f fakeSessionStore) IssueCSRFToken(ctx context.Context, id string, now time.Time) (string, error) {
	return f.issue(ctx, id, now)
}
func (f fakeSessionStore) Revoke(ctx context.Context, cookie string, now time.Time) error {
	return f.revoke(ctx, cookie, now)
}

func authenticatedSession(token string) auth.AuthenticatedSession {
	hash := sha256.Sum256([]byte(token))
	return auth.AuthenticatedSession{ID: "private-session-id", Principal: auth.Principal{MemberID: "selected-member", Roles: []domain.Role{domain.RoleRequester}}, CSRFTokenHash: hash[:]}
}

func sessionRequest(method, path string, secure bool) *http.Request {
	r := httptest.NewRequest(method, path, nil)
	r.AddCookie(auth.SessionCookie("current-cookie-secret", secure))
	return r
}

func TestSessionAndLogoutRequireAuthentication(t *testing.T) {
	for _, endpoint := range []struct{ method, path string }{{http.MethodGet, "/api/v1/session"}, {http.MethodPost, "/api/v1/session/logout"}} {
		for _, tc := range []struct {
			name, cookie string
			err          error
		}{
			{"missing", "", nil},
			{"empty", "__Host-approval_flow_session=", nil},
			{"wrong cookie name", "approval_flow_session=wrong-cookie", nil},
			{"unknown", "__Host-approval_flow_session=unknown", auth.ErrNotFound},
			{"expired", "__Host-approval_flow_session=expired", auth.ErrExpired},
			{"revoked", "__Host-approval_flow_session=revoked", fmt.Errorf("revoked: %w", auth.ErrNotFound)},
		} {
			t.Run(endpoint.method+"/"+tc.name, func(t *testing.T) {
				d := testDependencies()
				calls := 0
				d.SessionStore = fakeSessionStore{authenticate: func(context.Context, string, time.Time) (auth.AuthenticatedSession, error) {
					calls++
					return auth.AuthenticatedSession{}, tc.err
				}}
				r := httptest.NewRequest(endpoint.method, endpoint.path, nil)
				r.Header.Set("Cookie", tc.cookie)
				rr := httptest.NewRecorder()
				NewRouter(d).ServeHTTP(rr, r)
				assertError(t, rr, http.StatusUnauthorized, "authentication_required")
				wantCalls := 0
				if tc.err != nil {
					wantCalls = 1
				}
				if calls != wantCalls {
					t.Fatalf("Authenticate calls = %d, want %d", calls, wantCalls)
				}
			})
		}
	}
}

func TestRequireSessionUsesOnlyServerActorAndPropagatesContext(t *testing.T) {
	if _, ok := ActorFromContext(context.Background()); ok {
		t.Fatal("missing session returned actor")
	}
	for _, secure := range []bool{true, false} {
		t.Run(fmt.Sprint(secure), func(t *testing.T) {
			d := testDependencies()
			d.Config.CookieSecure = secure
			calls := 0
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			d.SessionStore = fakeSessionStore{authenticate: func(gotCtx context.Context, cookie string, now time.Time) (auth.AuthenticatedSession, error) {
				calls++
				if gotCtx != ctx || cookie != "current-cookie-secret" || now != testNow {
					t.Fatal("authentication lost context, configured cookie, or clock")
				}
				return authenticatedSession("current-token"), nil
			}}
			r := sessionRequest(http.MethodPost, "/?memberId=forged&roles=admin", secure).WithContext(ctx)
			r.Header.Set("X-Member-ID", "forged")
			r.Header.Set("X-Roles", "admin")
			r.AddCookie(&http.Cookie{Name: "memberId", Value: "forged"})
			r.Body = http.NoBody
			rr := httptest.NewRecorder()
			h := (&router{dependencies: d}).RequireSession(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				actor, ok := ActorFromContext(r.Context())
				if !ok || !reflect.DeepEqual(actor, requests.Actor{MemberID: "selected-member", Roles: []domain.Role{domain.RoleRequester}}) {
					t.Fatalf("actor = %+v, present = %v", actor, ok)
				}
				w.WriteHeader(http.StatusNoContent)
			}))
			h.ServeHTTP(rr, r)
			if rr.Code != http.StatusNoContent || calls != 1 {
				t.Fatalf("status = %d, authentication calls = %d", rr.Code, calls)
			}
		})
	}
}

func TestSessionReturnsActorAndRotatesCSRFToken(t *testing.T) {
	d := testDependencies()
	session := authenticatedSession("old-token")
	issued := 0
	revoked := false
	d.SessionStore = fakeSessionStore{
		authenticate: func(context.Context, string, time.Time) (auth.AuthenticatedSession, error) { return session, nil },
		issue: func(ctx context.Context, id string, now time.Time) (string, error) {
			if id != "private-session-id" || now != testNow {
				t.Fatal("token rotation did not use authenticated session ID and clock")
			}
			issued++
			token := fmt.Sprintf("fresh-token-%d", issued)
			session = authenticatedSession(token)
			return token, nil
		},
		revoke: func(context.Context, string, time.Time) error { revoked = true; return nil },
	}
	h := NewRouter(d)
	for i := 1; i <= 2; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, sessionRequest(http.MethodGet, "/api/v1/session?memberId=forged", true))
		if rr.Code != http.StatusOK || rr.Header().Get("Content-Type") != "application/json" || rr.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status = %d, headers = %v", rr.Code, rr.Header())
		}
		want := fmt.Sprintf(`{"actor":{"memberId":"selected-member","roles":["requester"]},"csrfToken":"fresh-token-%d"}`, i)
		if strings.TrimSpace(rr.Body.String()) != want {
			t.Fatalf("session = %s, want %s", rr.Body.String(), want)
		}
		assertNoSecrets(t, rr, "current-cookie-secret", "private-session-id", "old-token")
	}
	for _, token := range []string{"old-token", "fresh-token-1", "fresh-token-2"} {
		r := sessionRequest(http.MethodPost, "/api/v1/session/logout", true)
		r.Header.Set("Origin", allowedOrigin)
		r.Header.Set("X-CSRF-Token", token)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, r)
		if token != "fresh-token-2" {
			assertError(t, rr, http.StatusForbidden, "csrf_validation_failed")
			if revoked {
				t.Fatal("stale token revoked session")
			}
		} else if rr.Code != http.StatusNoContent || !revoked {
			t.Fatalf("current token logout status = %d, revoked = %v", rr.Code, revoked)
		}
	}
}

func TestSessionEmptyRolesUsesJSONArray(t *testing.T) {
	d := testDependencies()
	d.SessionStore = fakeSessionStore{
		authenticate: func(context.Context, string, time.Time) (auth.AuthenticatedSession, error) {
			session := authenticatedSession("token")
			session.Principal.Roles = nil
			return session, nil
		},
		issue: func(context.Context, string, time.Time) (string, error) { return "fresh", nil },
	}
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, sessionRequest(http.MethodGet, "/api/v1/session", true))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if string(body["actor"]) != `{"memberId":"selected-member","roles":[]}` {
		t.Fatalf("actor = %s", body["actor"])
	}
}

func TestLogoutRejectsInvalidCSRFBeforeRevocation(t *testing.T) {
	for _, tc := range []struct {
		name            string
		origins, tokens []string
	}{
		{"missing origin", nil, []string{"current-token"}},
		{"scheme", []string{"http://app.example"}, []string{"current-token"}},
		{"host", []string{"https://other.example"}, []string{"current-token"}},
		{"port", []string{"https://app.example:443"}, []string{"current-token"}},
		{"multiple origins", []string{allowedOrigin, allowedOrigin}, []string{"current-token"}},
		{"origin list", []string{allowedOrigin + " https://other.example"}, []string{"current-token"}},
		{"null origin", []string{"null"}, []string{"current-token"}},
		{"missing token", []string{allowedOrigin}, nil},
		{"mismatch", []string{allowedOrigin}, []string{"other-token"}},
		{"multiple tokens", []string{allowedOrigin}, []string{"current-token", "current-token"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDependencies()
			d.SessionStore = fakeSessionStore{
				authenticate: func(context.Context, string, time.Time) (auth.AuthenticatedSession, error) {
					return authenticatedSession("current-token"), nil
				},
				revoke: func(context.Context, string, time.Time) error { t.Fatal("CSRF rejection invoked revoke"); return nil },
			}
			r := sessionRequest(http.MethodPost, "/api/v1/session/logout", true)
			r.Header["Origin"] = tc.origins
			r.Header["X-Csrf-Token"] = tc.tokens
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			assertError(t, rr, http.StatusForbidden, "csrf_validation_failed")
			if len(rr.Result().Cookies()) != 0 {
				t.Fatal("rejected logout cleared cookie")
			}
		})
	}
}

func TestLogoutRevokesCurrentSessionAndClearsConfiguredCookie(t *testing.T) {
	for _, secure := range []bool{true, false} {
		t.Run(fmt.Sprint(secure), func(t *testing.T) {
			d := testDependencies()
			d.Config.CookieSecure = secure
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls := 0
			d.SessionStore = fakeSessionStore{
				authenticate: func(context.Context, string, time.Time) (auth.AuthenticatedSession, error) {
					return authenticatedSession("current-token"), nil
				},
				revoke: func(gotCtx context.Context, cookie string, now time.Time) error {
					calls++
					if gotCtx.Err() != context.Canceled || cookie != "current-cookie-secret" || now != testNow {
						t.Fatal("revocation lost context, authenticated cookie, or clock")
					}
					return nil
				},
			}
			cancel()
			r := sessionRequest(http.MethodPost, "/api/v1/session/logout?cookie=forged", secure).WithContext(ctx)
			r.Header.Set("Origin", allowedOrigin)
			r.Header.Set("X-CSRF-Token", "current-token")
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			if rr.Code != http.StatusNoContent || rr.Body.Len() != 0 || calls != 1 {
				t.Fatalf("status = %d, body = %s, calls = %d", rr.Code, rr.Body.String(), calls)
			}
			name := "__Host-approval_flow_session"
			if !secure {
				name = "approval_flow_session"
			}
			cookie := requireCookie(t, rr, name)
			assertCookiePolicy(t, cookie, secure)
			if cookie.Value != "" || cookie.MaxAge != -1 || !cookie.Expires.Before(testNow) {
				t.Fatal("session cookie not cleared")
			}
		})
	}
}

func TestSessionStoreErrorsAreSanitized(t *testing.T) {
	for _, operation := range []string{"authenticate", "issue", "revoke"} {
		for _, stale := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stale=%v", operation, stale), func(t *testing.T) {
				d := testDependencies()
				err := errors.New("private-database-secret")
				status, code := http.StatusInternalServerError, "internal_error"
				if stale {
					err = fmt.Errorf("private-database-secret: %w", auth.ErrNotFound)
					status, code = http.StatusUnauthorized, "authentication_required"
				}
				d.SessionStore = fakeSessionStore{
					authenticate: func(context.Context, string, time.Time) (auth.AuthenticatedSession, error) {
						if operation == "authenticate" {
							return auth.AuthenticatedSession{}, err
						}
						return authenticatedSession("token"), nil
					},
					issue:  func(context.Context, string, time.Time) (string, error) { return "", err },
					revoke: func(context.Context, string, time.Time) error { return err },
				}
				r := sessionRequest(http.MethodGet, "/api/v1/session", true)
				if operation == "revoke" {
					r = sessionRequest(http.MethodPost, "/api/v1/session/logout", true)
					r.Header.Set("Origin", allowedOrigin)
					r.Header.Set("X-CSRF-Token", "token")
				}
				rr := httptest.NewRecorder()
				NewRouter(d).ServeHTTP(rr, r)
				assertError(t, rr, status, code)
				assertNoSecrets(t, rr, "private-database-secret", "current-cookie-secret")
				if len(rr.Result().Cookies()) != 0 {
					t.Fatal("failed operation changed cookies")
				}
			})
		}
	}
}

func TestRequireCSRFFailsClosedWithoutSession(t *testing.T) {
	router := &router{dependencies: testDependencies()}
	h := router.RequireCSRF(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("missing session invoked handler") }))
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set("X-CSRF-Token", "token")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	assertError(t, rr, http.StatusForbidden, "csrf_validation_failed")
}
