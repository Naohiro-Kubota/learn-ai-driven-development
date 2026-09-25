package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/auth"
)

func TestCORSExposesRequestIDOnlyToAllowedOrigin(t *testing.T) {
	for _, tc := range []struct {
		origin string
		want   string
	}{
		{"https://app.example.test", "X-Request-ID"},
		{"https://other.example.test", ""},
	} {
		handler := NewObservabilityMiddleware(nil, nil, nil)(NewCORS("https://app.example.test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		})))
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if got := w.Header().Get("Access-Control-Expose-Headers"); got != tc.want {
			t.Fatalf("origin=%q exposed=%q want=%q", tc.origin, got, tc.want)
		}
	}
}

func TestRouterCORSAllowsConfiguredOriginAndPreflightWithoutCallingMux(t *testing.T) {
	d := testDependencies()
	cases := []struct {
		name       string
		method     string
		path       string
		preflight  bool
		wantStatus int
	}{
		{name: "simple", method: http.MethodGet, path: "/not-a-route", wantStatus: http.StatusNotFound},
		{name: "preflight", method: http.MethodOptions, path: "/api/v1/requests", preflight: true, wantStatus: http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, nil)
			r.Header.Set("Origin", allowedOrigin)
			if tc.preflight {
				r.Header.Set("Access-Control-Request-Method", http.MethodPost)
				r.Header.Set("Access-Control-Request-Headers", "Content-Type, X-CSRF-Token")
			}
			rr := httptest.NewRecorder()
			NewRouter(d).ServeHTTP(rr, r)
			if rr.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rr.Code, tc.wantStatus)
			}
			if rr.Header().Get("Access-Control-Allow-Origin") != allowedOrigin || rr.Header().Get("Access-Control-Allow-Credentials") != "true" || rr.Header().Get("Vary") != "Origin" {
				t.Fatalf("CORS response headers = %v", rr.Header())
			}
			if tc.preflight {
				if rr.Header().Get("Access-Control-Allow-Methods") != "GET, POST, PATCH, OPTIONS" || rr.Header().Get("Access-Control-Allow-Headers") != "Content-Type, X-CSRF-Token" {
					t.Fatalf("preflight response headers = %v", rr.Header())
				}
			}
		})
	}
}

func TestRouterCORSRejectsInvalidRequestsBeforeCallingMux(t *testing.T) {
	cases := []struct {
		name    string
		method  string
		headers http.Header
	}{
		{"disallowed origin", http.MethodGet, http.Header{"Origin": {"http://evil.example.test"}}},
		{"duplicate origin", http.MethodGet, http.Header{"Origin": {allowedOrigin, allowedOrigin}}},
		{"duplicate preflight method", http.MethodOptions, http.Header{"Origin": {allowedOrigin}, "Access-Control-Request-Method": {http.MethodPost, http.MethodDelete}}},
		{"duplicate preflight headers", http.MethodOptions, http.Header{"Origin": {allowedOrigin}, "Access-Control-Request-Method": {http.MethodPost}, "Access-Control-Request-Headers": {"Content-Type", "Authorization"}}},
		{"malformed method list", http.MethodOptions, http.Header{"Origin": {allowedOrigin}, "Access-Control-Request-Method": {"POST, DELETE"}}},
		{"malformed headers list", http.MethodOptions, http.Header{"Origin": {allowedOrigin}, "Access-Control-Request-Method": {http.MethodPost}, "Access-Control-Request-Headers": {"Content-Type,"}}},
		{"unrecognized method", http.MethodOptions, http.Header{"Origin": {allowedOrigin}, "Access-Control-Request-Method": {http.MethodDelete}}},
		{"unrecognized header", http.MethodOptions, http.Header{"Origin": {allowedOrigin}, "Access-Control-Request-Method": {http.MethodPost}, "Access-Control-Request-Headers": {"Authorization"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			h := NewCORS(allowedOrigin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
			r := httptest.NewRequest(tc.method, "/api/v1/requests", nil)
			r.Header = tc.headers
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, r)
			if rr.Code != http.StatusForbidden || called {
				t.Fatalf("status = %d, called = %v", rr.Code, called)
			}
			for _, header := range []string{"Access-Control-Allow-Origin", "Access-Control-Allow-Credentials", "Access-Control-Allow-Methods", "Access-Control-Allow-Headers"} {
				if rr.Header().Get(header) != "" {
					t.Fatalf("rejected request exposes %s: %v", header, rr.Header())
				}
			}
		})
	}
}

func TestRouterCORSErrorsRemainReadableToAllowedOrigin(t *testing.T) {
	cases := []struct {
		name         string
		request      *http.Request
		dependencies Dependencies
		wantStatus   int
		wantCode     string
	}{
		{name: "missing session", request: httptest.NewRequest(http.MethodGet, "/api/v1/session", nil), dependencies: testDependencies(), wantStatus: http.StatusUnauthorized, wantCode: "authentication_required"},
		{name: "missing CSRF", request: sessionRequest(http.MethodPost, "/api/v1/requests", true), dependencies: func() Dependencies {
			d := testDependencies()
			d.SessionStore = fakeSessionStore{authenticate: func(_ context.Context, _ string, _ time.Time) (auth.AuthenticatedSession, error) {
				return authenticatedSession("token"), nil
			}}
			return d
		}(), wantStatus: http.StatusForbidden, wantCode: "csrf_validation_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.request.Header.Set("Origin", allowedOrigin)
			rr := httptest.NewRecorder()
			NewRouter(tc.dependencies).ServeHTTP(rr, tc.request)
			assertError(t, rr, tc.wantStatus, tc.wantCode)
			if rr.Header().Get("Access-Control-Allow-Origin") != allowedOrigin || rr.Header().Get("Access-Control-Allow-Credentials") != "true" || rr.Header().Get("Vary") != "Origin" {
				t.Fatalf("CORS response headers = %v", rr.Header())
			}
		})
	}
}

func TestRouterCORSPreservesExistingVaryValue(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Vary", "Accept-Encoding")
		NewCORS(allowedOrigin, next).ServeHTTP(w, r)
	})
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", allowedOrigin)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)

	if rr.Header().Get("Vary") != "Accept-Encoding, Origin" {
		t.Fatalf("Vary = %q, want %q", rr.Header().Get("Vary"), "Accept-Encoding, Origin")
	}
}
