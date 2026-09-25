package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
)

const allowedOrigin = "https://app.example"

var testNow = time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)

type fakeAuthenticator struct {
	begin    func(context.Context) (auth.LoginStart, error)
	complete func(context.Context, auth.CallbackInput) (auth.LoginResult, error)
}

func (f fakeAuthenticator) BeginLogin(ctx context.Context) (auth.LoginStart, error) {
	return f.begin(ctx)
}
func (f fakeAuthenticator) CompleteLogin(ctx context.Context, input auth.CallbackInput) (auth.LoginResult, error) {
	return f.complete(ctx, input)
}

type fakeSelectionStore struct {
	read     func(context.Context, string, time.Time) (auth.OrganizationSelection, error)
	complete func(context.Context, auth.CompleteOrganizationSelectionInput) (auth.SessionInput, error)
}

func (f fakeSelectionStore) ReadAndIssueCSRFToken(ctx context.Context, cookie string, now time.Time) (auth.OrganizationSelection, error) {
	return f.read(ctx, cookie, now)
}
func (f fakeSelectionStore) Complete(ctx context.Context, input auth.CompleteOrganizationSelectionInput) (auth.SessionInput, error) {
	return f.complete(ctx, input)
}

func testDependencies() Dependencies {
	return Dependencies{Config: config.Config{CookieSecure: true, FrontendOrigin: allowedOrigin, AuthTransactionTTL: 5 * time.Minute}, Now: func() time.Time { return testNow }}
}

func TestLoginRedirectAndTransactionCookie(t *testing.T) {
	for _, secure := range []bool{true, false} {
		t.Run(fmt.Sprint(secure), func(t *testing.T) {
			d := testDependencies()
			d.Config.CookieSecure = secure
			d.Authenticator = fakeAuthenticator{begin: func(context.Context) (auth.LoginStart, error) {
				return auth.LoginStart{AuthorizationURL: "https://issuer.example/authorize?state=server-state", TransactionCookie: "transaction-secret"}, nil
			}}
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/oidc/login?returnUrl=https://evil.example", nil))
			assertRedirect(t, rr, "https://issuer.example/authorize?state=server-state")
			name := "__Host-approval_flow_auth_transaction"
			if !secure {
				name = "approval_flow_auth_transaction"
			}
			cookie := requireCookie(t, rr, name)
			assertCookiePolicy(t, cookie, secure)
			if cookie.Value != "transaction-secret" || cookie.MaxAge != 300 || !cookie.Expires.Equal(testNow.Add(5*time.Minute)) {
				t.Fatalf("transaction lifetime/value incorrect")
			}
			assertNoSecrets(t, rr, "transaction-secret")
		})
	}
}

func TestLoginUnexpectedErrorIsSanitized(t *testing.T) {
	d := testDependencies()
	d.Authenticator = fakeAuthenticator{begin: func(context.Context) (auth.LoginStart, error) {
		return auth.LoginStart{}, errors.New("private-verifier")
	}}
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil))
	assertError(t, rr, 500, "internal_error")
	assertNoSecrets(t, rr, "private-verifier")
	if len(rr.Result().Cookies()) != 0 {
		t.Fatal("failure issued cookie")
	}
}

func TestCallbackSessionAndSelection(t *testing.T) {
	for _, multi := range []bool{false, true} {
		t.Run(fmt.Sprint(multi), func(t *testing.T) {
			d := testDependencies()
			d.Authenticator = fakeAuthenticator{complete: func(ctx context.Context, in auth.CallbackInput) (auth.LoginResult, error) {
				if in != (auth.CallbackInput{TransactionCookie: "transaction-secret", State: "state-secret", Code: "code-secret"}) {
					t.Fatal("callback input not forwarded")
				}
				if multi {
					return auth.LoginResult{Selection: &auth.OrganizationSelectionInput{Cookie: "selection-secret", CSRFToken: "csrf-secret", IdentityID: "identity-subject-secret", ExpiresAt: testNow.Add(5 * time.Minute)}}, nil
				}
				return auth.LoginResult{Session: &auth.SessionInput{Cookie: "session-secret", CSRFToken: "csrf-secret", MemberID: "member-1"}}, nil
			}}
			rr := serveCallback(d, "code=code-secret&state=state-secret&returnUrl=https://evil.example")
			assertClearedCookie(t, rr, "__Host-approval_flow_auth_transaction")
			if multi {
				assertRedirect(t, rr, allowedOrigin+"/organization-selection")
				cookie := requireCookie(t, rr, "__Host-approval_flow_organization_selection")
				assertCookiePolicy(t, cookie, true)
				if cookie.Value != "selection-secret" || cookie.MaxAge != 300 || !cookie.Expires.Equal(testNow.Add(5*time.Minute)) {
					t.Fatal("selection cookie incorrect")
				}
				assertNoSessionCookie(t, rr)
			} else {
				assertRedirect(t, rr, allowedOrigin+"/")
				cookie := requireCookie(t, rr, "__Host-approval_flow_session")
				assertCookiePolicy(t, cookie, true)
				if cookie.Value != "session-secret" {
					t.Fatal("session cookie incorrect")
				}
			}
			assertNoSecrets(t, rr, "transaction-secret", "state-secret", "code-secret", "selection-secret", "session-secret", "csrf-secret", "identity-subject-secret")
		})
	}
}

func TestCallbackRejectsInvalidAndReplayedAttempts(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"missing", auth.ErrNotFound, 400, "invalid_auth_transaction"},
		{"expired", auth.ErrExpired, 400, "invalid_auth_transaction"},
		{"consumed", auth.ErrConsumed, 400, "invalid_auth_transaction"},
		{"state mismatch", auth.ErrForbidden, 400, "invalid_auth_transaction"},
		{"invalid ID token", auth.ErrInvalidAuthentication, 400, "invalid_auth_transaction"},
		{"unexpected", errors.New("raw-id-token-secret"), 500, "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDependencies()
			d.Authenticator = fakeAuthenticator{complete: func(context.Context, auth.CallbackInput) (auth.LoginResult, error) {
				return auth.LoginResult{}, fmt.Errorf("details: %w", tc.err)
			}}
			rr := serveCallback(d, "state=state-secret&code=code-secret")
			assertError(t, rr, tc.status, tc.code)
			assertClearedCookie(t, rr, "__Host-approval_flow_auth_transaction")
			assertNoSessionCookie(t, rr)
			assertNoSecrets(t, rr, "raw-id-token-secret", "details", "state-secret", "code-secret")
		})
	}
	for _, query := range []string{"error=access_denied&error_description=provider-secret&state=state-secret", "state=state-secret", "code=code-secret", "code=a&code=b&state=state-secret", "code=code-secret&state=a&state=b"} {
		d := testDependencies()
		called := false
		d.Authenticator = fakeAuthenticator{complete: func(_ context.Context, in auth.CallbackInput) (auth.LoginResult, error) {
			called = true
			if in.Code != "" {
				t.Fatal("malformed callback may not exchange a code")
			}
			return auth.LoginResult{}, errors.New("private-code-error")
		}}
		rr := serveCallback(d, query)
		if !called {
			t.Fatal("recognized callback did not consume transaction")
		}
		assertError(t, rr, 400, "invalid_auth_transaction")
		assertClearedCookie(t, rr, "__Host-approval_flow_auth_transaction")
		assertNoSessionCookie(t, rr)
		assertNoSecrets(t, rr, "provider-secret", "private-code-error")
	}
}

func TestCallbackRequiresCookie(t *testing.T) {
	rr := httptest.NewRecorder()
	NewRouter(testDependencies()).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=s&code=c", nil))
	assertError(t, rr, 400, "invalid_auth_transaction")
	assertNoSessionCookie(t, rr)
}

func TestCallbackReplayNeverIssuesAnotherSession(t *testing.T) {
	d := testDependencies()
	consumed := false
	d.Authenticator = fakeAuthenticator{complete: func(context.Context, auth.CallbackInput) (auth.LoginResult, error) {
		if consumed {
			return auth.LoginResult{}, auth.ErrConsumed
		}
		consumed = true
		return auth.LoginResult{Session: &auth.SessionInput{Cookie: "session-secret"}}, nil
	}}
	assertRedirect(t, serveCallback(d, "code=c&state=s"), allowedOrigin+"/")
	rr := serveCallback(d, "code=c&state=s")
	assertError(t, rr, 400, "invalid_auth_transaction")
	assertNoSessionCookie(t, rr)
}

func TestCallbackInvalidResultDoesNotIssueCookie(t *testing.T) {
	for _, result := range []auth.LoginResult{{}, {Session: &auth.SessionInput{Cookie: "session-secret"}, Selection: &auth.OrganizationSelectionInput{Cookie: "selection-secret"}}} {
		d := testDependencies()
		d.Authenticator = fakeAuthenticator{complete: func(context.Context, auth.CallbackInput) (auth.LoginResult, error) { return result, nil }}
		rr := serveCallback(d, "code=c&state=s")
		assertError(t, rr, 500, "internal_error")
		assertNoSessionCookie(t, rr)
		if len(rr.Result().Cookies()) != 1 {
			t.Fatal("invalid result issued flow cookie")
		}
	}
}

func TestOrganizationSelectionReturnsOnlyContractFields(t *testing.T) {
	d := testDependencies()
	d.SelectionStore = fakeSelectionStore{read: func(_ context.Context, cookie string, now time.Time) (auth.OrganizationSelection, error) {
		if cookie != "selection-secret" || !now.Equal(testNow) {
			t.Fatal("incorrect selection lookup")
		}
		return auth.OrganizationSelection{Candidates: []auth.OrganizationSelectionCandidate{{MemberID: "member-1", OrganizationID: "org-1", OrganizationName: "One"}}, CSRFToken: "new-csrf-token"}, nil
	}}
	r := selectionRequest(http.MethodGet, "", true)
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, r)
	if rr.Code != 200 || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status/content type: %d %s", rr.Code, rr.Header().Get("Content-Type"))
	}
	var got any
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"candidates": []any{map[string]any{"memberId": "member-1", "organizationId": "org-1", "organizationName": "One"}}, "csrfToken": "new-csrf-token"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected contract response: %v", got)
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("CSRF response can be cached")
	}
	assertNoSecrets(t, rr, "selection-secret")
}

func TestOrganizationSelectionCompleteAndReplay(t *testing.T) {
	d := testDependencies()
	consumed := false
	d.SelectionStore = fakeSelectionStore{complete: func(_ context.Context, in auth.CompleteOrganizationSelectionInput) (auth.SessionInput, error) {
		if in.Cookie != "selection-secret" || in.CSRFToken != "selection-csrf" || in.MemberID != "member-1" || !in.Now.Equal(testNow) {
			t.Fatal("incorrect completion input")
		}
		if consumed {
			return auth.SessionInput{}, auth.ErrConsumed
		}
		consumed = true
		return auth.SessionInput{Cookie: "new-session-secret"}, nil
	}}
	h := NewRouter(d)
	for attempt := 0; attempt < 2; attempt++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, selectionRequest(http.MethodPost, `{"memberId":"member-1"}`, true))
		if attempt == 0 {
			assertRedirect(t, rr, allowedOrigin+"/")
			assertClearedCookie(t, rr, "__Host-approval_flow_organization_selection")
			cookie := requireCookie(t, rr, "__Host-approval_flow_session")
			assertCookiePolicy(t, cookie, true)
			if cookie.Value != "new-session-secret" {
				t.Fatal("session cookie incorrect")
			}
		} else {
			assertError(t, rr, 400, "invalid_auth_transaction")
			assertNoSessionCookie(t, rr)
		}
		assertNoSecrets(t, rr, "new-session-secret", "selection-secret", "selection-csrf")
	}
}

func TestOrganizationSelectionErrors(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		for _, tc := range []struct {
			err    error
			status int
			code   string
		}{
			{auth.ErrNotFound, 400, "invalid_auth_transaction"}, {auth.ErrExpired, 400, "invalid_auth_transaction"}, {auth.ErrConsumed, 400, "invalid_auth_transaction"},
			{auth.ErrForbidden, 403, "forbidden"}, {auth.ErrCSRFValidation, 403, "csrf_validation_failed"}, {errors.New("raw-verifier-secret"), 500, "internal_error"},
		} {
			t.Run(method+tc.code+tc.err.Error(), func(t *testing.T) {
				d := testDependencies()
				d.SelectionStore = fakeSelectionStore{
					read: func(context.Context, string, time.Time) (auth.OrganizationSelection, error) {
						return auth.OrganizationSelection{}, fmt.Errorf("private: %w", tc.err)
					},
					complete: func(context.Context, auth.CompleteOrganizationSelectionInput) (auth.SessionInput, error) {
						return auth.SessionInput{}, fmt.Errorf("private: %w", tc.err)
					},
				}
				rr := httptest.NewRecorder()
				NewRouter(d).ServeHTTP(rr, selectionRequest(method, `{"memberId":"member-outside"}`, true))
				assertError(t, rr, tc.status, tc.code)
				assertNoSessionCookie(t, rr)
				assertNoSecrets(t, rr, "raw-verifier-secret", "private")
			})
		}
		rr := httptest.NewRecorder()
		NewRouter(testDependencies()).ServeHTTP(rr, selectionRequest(method, `{"memberId":"member-1"}`, false))
		assertError(t, rr, 400, "invalid_auth_transaction")
	}
}

func TestOrganizationSelectionRejectsCandidateOutsideSnapshot(t *testing.T) {
	d := testDependencies()
	consumed := false
	d.SelectionStore = fakeSelectionStore{complete: func(_ context.Context, input auth.CompleteOrganizationSelectionInput) (auth.SessionInput, error) {
		if input.Cookie != "selection-secret" || input.CSRFToken != "selection-csrf" {
			t.Fatal("wrong selection credentials")
		}
		if consumed {
			return auth.SessionInput{}, auth.ErrConsumed
		}
		consumed = true
		if input.MemberID != "member-1" {
			return auth.SessionInput{}, auth.ErrForbidden
		}
		return auth.SessionInput{Cookie: "session-secret"}, nil
	}}
	h := NewRouter(d)
	for i, member := range []string{"member-outside", "member-1"} {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, selectionRequest(http.MethodPost, `{"memberId":"`+member+`"}`, true))
		if i == 0 {
			assertError(t, rr, 403, "forbidden")
		} else {
			assertError(t, rr, 400, "invalid_auth_transaction")
		}
		assertNoSessionCookie(t, rr)
	}
}

func TestOrganizationSelectionDuplicateCookieIsRejected(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		r := selectionRequest(method, `{"memberId":"member-1"}`, true)
		r.AddCookie(&http.Cookie{Name: "__Host-approval_flow_organization_selection", Value: "another-secret"})
		rr := httptest.NewRecorder()
		NewRouter(testDependencies()).ServeHTTP(rr, r)
		assertError(t, rr, 400, "invalid_auth_transaction")
		assertNoSessionCookie(t, rr)
	}
}

func TestOrganizationSelectionRejectsInvalidOrigin(t *testing.T) {
	for _, origins := range [][]string{nil, {""}, {"null"}, {"http://app.example"}, {"https://app.example:443"}, {"https://evil.example"}, {allowedOrigin, allowedOrigin}, {allowedOrigin + " https://evil.example"}} {
		r := selectionRequest(http.MethodPost, `{"memberId":"member-1"}`, true)
		r.Header.Del("Origin")
		for _, origin := range origins {
			r.Header.Add("Origin", origin)
		}
		rr := httptest.NewRecorder()
		// No store: rejection must occur before a completion can issue a session.
		NewRouter(testDependencies()).ServeHTTP(rr, r)
		assertError(t, rr, 403, "csrf_validation_failed")
		assertNoSessionCookie(t, rr)
	}
}

func TestOrganizationSelectionMissingAndDuplicateCSRFConsumeAttempt(t *testing.T) {
	for _, tokens := range [][]string{nil, {""}, {"selection-csrf", "selection-csrf"}} {
		d := testDependencies()
		called := false
		d.SelectionStore = fakeSelectionStore{complete: func(_ context.Context, in auth.CompleteOrganizationSelectionInput) (auth.SessionInput, error) {
			called = true
			if in.CSRFToken != "" {
				t.Fatal("ambiguous token forwarded")
			}
			return auth.SessionInput{}, auth.ErrCSRFValidation
		}}
		r := selectionRequest(http.MethodPost, `{"memberId":"member-1"}`, true)
		r.Header.Del("X-CSRF-Token")
		for _, token := range tokens {
			r.Header.Add("X-CSRF-Token", token)
		}
		rr := httptest.NewRecorder()
		NewRouter(d).ServeHTTP(rr, r)
		if !called {
			t.Fatal("recognized invalid token attempt not consumed")
		}
		assertError(t, rr, 403, "csrf_validation_failed")
		assertNoSessionCookie(t, rr)
	}
}

func TestOrganizationSelectionRejectsMalformedJSON(t *testing.T) {
	for _, body := range []string{``, `{`, `null`, `[]`, `{}`, `{"memberId":""}`, `{"memberId":null}`, `{"memberId":1}`, `{"MemberID":"member-1"}`, `{"memberId":"member-1","memberId":"member-2"}`, `{"memberId":"member-1","roles":["admin"]}`, `{"memberId":"member-1"} {}`, `{"memberId":"` + strings.Repeat("x", 65536) + `"}`} {
		rr := httptest.NewRecorder()
		NewRouter(testDependencies()).ServeHTTP(rr, selectionRequest(http.MethodPost, body, true))
		assertError(t, rr, 400, "invalid_request")
		assertNoSessionCookie(t, rr)
	}
}

func TestOrganizationSelectionRejectsDuplicateMemberID(t *testing.T) {
	rr := httptest.NewRecorder()
	NewRouter(testDependencies()).ServeHTTP(rr, selectionRequest(http.MethodPost, `{"memberId":"member-1","memberId":"member-2"}`, true))
	assertError(t, rr, http.StatusBadRequest, "invalid_request")
	assertNoSessionCookie(t, rr)
}

func TestWriteErrorSanitizesUnexpectedDetails(t *testing.T) {
	var logs bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(previous) })
	rr := httptest.NewRecorder()
	WriteError(rr, APIError{Status: 500, Code: "internal_error"})
	assertError(t, rr, 500, "internal_error")
	d := testDependencies()
	d.Authenticator = fakeAuthenticator{complete: func(context.Context, auth.CallbackInput) (auth.LoginResult, error) {
		return auth.LoginResult{}, errors.New("raw-token verifier-secret identity-subject-secret transaction-secret")
	}}
	rr = serveCallback(d, "code=code-secret&state=state-secret")
	assertError(t, rr, 500, "internal_error")
	assertNoSecrets(t, rr, "raw-token", "verifier-secret", "identity-subject-secret", "transaction-secret", "code-secret", "state-secret")
	if logs.Len() != 0 {
		t.Fatal("error writer logged data")
	}
}

func TestOrganizationSelectionOpenAPIContract(t *testing.T) {
	data, err := os.ReadFile("../../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	doc := string(data)
	path := yamlBlock(t, doc, "  /auth/oidc/organization-selection:\n", "  /api/")
	get := yamlBlock(t, path, "    get:\n", "    post:\n")
	post := yamlBlock(t, path, "    post:\n", "\x00")
	for _, text := range []string{"organizationSelectionCookie: []", "'200':", "#/components/schemas/OrganizationSelection", "'400':", "#/components/responses/InvalidAuthTransaction", "'403':", "#/components/responses/ForbiddenOrCsrfValidationFailed"} {
		if !strings.Contains(get, text) {
			t.Errorf("GET contract missing %s", text)
		}
	}
	for _, text := range []string{"organizationSelectionCookie: []", "csrfToken: []", "#/components/schemas/SelectOrganizationInput", "'302':", "'400':", "invalid_auth_transaction", "invalid_request", "'403':", "#/components/responses/ForbiddenOrCsrfValidationFailed"} {
		if !strings.Contains(post, text) {
			t.Errorf("POST contract missing %s", text)
		}
	}
	for _, tc := range []struct {
		start, end string
		required   []string
	}{
		{"    organizationSelectionCookie:\n", "    csrfToken:\n", []string{"type: apiKey", "in: cookie", "name: __Host-approval_flow_organization_selection"}},
		{"    OrganizationSelectionCandidate:\n", "    OrganizationSelection:\n", []string{"additionalProperties: false", "required: [memberId, organizationId, organizationName]"}},
		{"    OrganizationSelection:\n", "    SelectOrganizationInput:\n", []string{"required: [candidates, csrfToken]", "#/components/schemas/OrganizationSelectionCandidate"}},
		{"    SelectOrganizationInput:\n", "    Session:\n", []string{"additionalProperties: false", "required: [memberId]", "minLength: 1"}},
		{"    InvalidAuthTransaction:\n", "    AuthenticationRequired:\n", []string{"code: invalid_auth_transaction"}},
		{"    ForbiddenOrCsrfValidationFailed:\n", "    RequestNotFound:\n", []string{"code: forbidden", "code: csrf_validation_failed"}},
	} {
		block := yamlBlock(t, doc, tc.start, tc.end)
		for _, text := range tc.required {
			if !strings.Contains(block, text) {
				t.Errorf("%s missing %s", tc.start, text)
			}
		}
	}
}

func yamlBlock(t *testing.T, doc, start, end string) string {
	t.Helper()
	_, block, ok := strings.Cut(doc, start)
	if !ok {
		t.Fatalf("missing OpenAPI block %s", start)
	}
	block, _, _ = strings.Cut(block, end)
	return block
}

func selectionRequest(method, body string, cookie bool) *http.Request {
	r := httptest.NewRequest(method, "/auth/oidc/organization-selection?returnUrl=https://evil.example", strings.NewReader(body))
	if cookie {
		r.AddCookie(&http.Cookie{Name: "__Host-approval_flow_organization_selection", Value: "selection-secret"})
	}
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set("X-CSRF-Token", "selection-csrf")
	r.Header.Set("Content-Type", "application/json")
	return r
}

func serveCallback(d Dependencies, query string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?"+query, nil)
	r.AddCookie(&http.Cookie{Name: "__Host-approval_flow_auth_transaction", Value: "transaction-secret"})
	rr := httptest.NewRecorder()
	NewRouter(d).ServeHTTP(rr, r)
	return rr
}

func requireCookie(t *testing.T, rr *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range rr.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("cookie %s missing", name)
	return nil
}

func assertCookiePolicy(t *testing.T, cookie *http.Cookie, secure bool) {
	t.Helper()
	if cookie.Secure != secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/" || cookie.Domain != "" {
		t.Fatal("cookie policy violation")
	}
}

func assertClearedCookie(t *testing.T, rr *httptest.ResponseRecorder, name string) {
	t.Helper()
	cookie := requireCookie(t, rr, name)
	assertCookiePolicy(t, cookie, true)
	if cookie.Value != "" || cookie.MaxAge != -1 || !cookie.Expires.Before(testNow) {
		t.Fatal("cookie not cleared")
	}
}

func assertNoSessionCookie(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	for _, cookie := range rr.Result().Cookies() {
		if strings.HasSuffix(cookie.Name, "approval_flow_session") {
			t.Fatal("failed/unselected login issued session cookie")
		}
	}
}

func assertRedirect(t *testing.T, rr *httptest.ResponseRecorder, location string) {
	t.Helper()
	if rr.Code != 302 || rr.Header().Get("Location") != location {
		t.Fatalf("redirect = %d %q", rr.Code, rr.Header().Get("Location"))
	}
}

func assertError(t *testing.T, rr *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rr.Code != status || rr.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status/content type = %d %q, want %d application/json", rr.Code, rr.Header().Get("Content-Type"), status)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["code"] != code || body["message"] == "" || len(body) != 2 {
		t.Fatalf("error body = %v, want %s", body, code)
	}
}

func assertNoSecrets(t *testing.T, rr *httptest.ResponseRecorder, secrets ...string) {
	t.Helper()
	for _, secret := range secrets {
		if strings.Contains(rr.Body.String(), secret) || strings.Contains(rr.Header().Get("Location"), secret) {
			t.Fatal("secret appeared in response body/location")
		}
	}
}
