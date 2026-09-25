package main

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/httpapi"
)

func TestTelemetryFlushesOTLPOnShutdown(t *testing.T) {
	var mu sync.Mutex
	counts := map[string]int{}
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer collector.Close()
	cfg := config.Config{OTLPEndpoint: collector.URL, OTLPTimeout: time.Second, TraceSampleRatio: 1}
	telemetry, err := newTelemetry(context.Background(), cfg, slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil)))
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewObservabilityMiddleware(nil, telemetry.meter, telemetry.tracer)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(409) }))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/private-id", nil))
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := telemetry.shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if counts["/v1/traces"] == 0 || counts["/v1/metrics"] == 0 {
		t.Fatalf("OTLP requests = %v", counts)
	}
}

func TestUnavailableCollectorDoesNotChangeHTTPResponse(t *testing.T) {
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("secret-from-collector"))
	}))
	defer collector.Close()
	var logs bytes.Buffer
	cfg := config.Config{OTLPEndpoint: collector.URL, OTLPTimeout: 100 * time.Millisecond, TraceSampleRatio: 1}
	telemetry, err := newTelemetry(context.Background(), cfg, slog.New(slog.NewJSONHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewObservabilityMiddleware(nil, telemetry.meter, telemetry.tracer)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusConflict) }))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/private-id", nil))
	if w.Code != http.StatusConflict {
		t.Fatalf("status=%d", w.Code)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	shutdownErr := telemetry.shutdown(ctx)
	if shutdownErr != nil && strings.Contains(shutdownErr.Error(), "secret-from-collector") {
		t.Fatalf("export error exposed response body: %v", shutdownErr)
	}
	if !bytes.Contains(logs.Bytes(), []byte("telemetry export failed")) {
		t.Fatalf("missing operator signal: %s", logs.String())
	}
	if bytes.Contains(logs.Bytes(), []byte("secret-from-collector")) {
		t.Fatalf("export warning exposed response body: %s", logs.String())
	}
}
