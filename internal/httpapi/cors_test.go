package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

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

func TestRouterCORSRejectsDisallowedOriginBeforeCallingMux(t *testing.T) {
	called := false
	d := testDependencies()
	h := NewCORS(d.Config.FrontendOrigin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Origin", "http://evil.example.test")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden || called {
		t.Fatalf("status = %d, called = %v", rr.Code, called)
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "" || rr.Header().Get("Access-Control-Allow-Credentials") != "" {
		t.Fatalf("disallowed response exposes CORS credentials: %v", rr.Header())
	}
}

func TestRouterCORSRejectsInvalidPreflightBeforeCallingMux(t *testing.T) {
	called := false
	h := NewCORS(allowedOrigin, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	r := httptest.NewRequest(http.MethodOptions, "/", nil)
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set("Access-Control-Request-Method", http.MethodDelete)
	r.Header.Set("Access-Control-Request-Headers", "Authorization")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden || called {
		t.Fatalf("status = %d, called = %v", rr.Code, called)
	}
	for _, header := range []string{
		"Access-Control-Allow-Origin",
		"Access-Control-Allow-Credentials",
		"Access-Control-Allow-Methods",
		"Access-Control-Allow-Headers",
		"Vary",
	} {
		if rr.Header().Get(header) != "" {
			t.Fatalf("rejected preflight exposes %s: %v", header, rr.Header())
		}
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
