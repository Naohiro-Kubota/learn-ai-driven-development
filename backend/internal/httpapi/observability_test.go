package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestObservabilityRecordsBoundedRequestSignals(t *testing.T) {
	var logs bytes.Buffer
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	defer meterProvider.Shutdown(context.Background())
	spans := tracetest.NewSpanRecorder()
	traceProvider := trace.NewTracerProvider(trace.WithSpanProcessor(spans), trace.WithSampler(trace.AlwaysSample()))
	defer traceProvider.Shutdown(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/requests/{requestId}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	})
	handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), meterProvider.Meter("test"), traceProvider.Tracer("test"))(routedHandler{Handler: mux, mux: mux, patterns: map[string]struct{}{"GET /api/v1/requests/{requestId}": {}}})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/requests/private-request-123", strings.NewReader("secret-body"))
	req.Header.Set("X-Request-ID", "safe-123")
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret-cookie")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusConflict || w.Header().Get("X-Request-ID") != "safe-123" {
		t.Fatalf("status=%d request_id=%q", w.Code, w.Header().Get("X-Request-ID"))
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["route"] != "GET /api/v1/requests/{requestId}" || entry["status"] != float64(409) || entry["failure_class"] != "conflict" || entry["request_id"] != "safe-123" {
		t.Fatalf("unexpected completion log: %v", entry)
	}
	if entry["trace_id"] == "" || entry["duration_ms"] == nil {
		t.Fatalf("missing trace or duration: %v", entry)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	if len(data.ScopeMetrics) == 0 || len(spans.Ended()) != 1 {
		t.Fatalf("metrics=%v spans=%d", data.ScopeMetrics, len(spans.Ended()))
	}
	metricNames := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			metricNames[m.Name] = true
			switch m.Name {
			case "http.server.request.count", "http.server.request.failure.count":
				sum, ok := m.Data.(metricdata.Sum[int64])
				if !ok || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
					t.Fatalf("%s data=%v, want one request", m.Name, m.Data)
				}
				assertRequestMetricAttributes(t, sum.DataPoints[0].Attributes)
			case "http.server.request.duration":
				histogram, ok := m.Data.(metricdata.Histogram[float64])
				if !ok || len(histogram.DataPoints) != 1 || histogram.DataPoints[0].Count != 1 || histogram.DataPoints[0].Sum <= 0 {
					t.Fatalf("duration data=%v, want positive observation", m.Data)
				}
				assertRequestMetricAttributes(t, histogram.DataPoints[0].Attributes)
			}
		}
	}
	for _, name := range []string{"http.server.request.count", "http.server.request.duration", "http.server.request.failure.count"} {
		if !metricNames[name] {
			t.Fatalf("missing metric %q: %v", name, metricNames)
		}
	}
	if spans.Ended()[0].SpanContext().TraceID().String() != entry["trace_id"] {
		t.Fatalf("log trace ID does not match exported span")
	}
	for _, sensitive := range []string{"private-request-123", "secret-token", "secret-cookie", "secret-body"} {
		if strings.Contains(logs.String(), sensitive) || strings.Contains(fmt.Sprint(data), sensitive) || strings.Contains(fmt.Sprint(spans.Ended()[0].Attributes()), sensitive) {
			t.Fatalf("sensitive value %q leaked", sensitive)
		}
	}
}

func assertRequestMetricAttributes(t *testing.T, attrs attribute.Set) {
	t.Helper()
	for _, tc := range []struct{ key, want string }{
		{"http.route", "GET /api/v1/requests/{requestId}"},
		{"http.request.method", "GET"},
		{"failure.class", "conflict"},
	} {
		got, ok := attrs.Value(attribute.Key(tc.key))
		if !ok || got.AsString() != tc.want {
			t.Fatalf("attribute %q=%q, want %q", tc.key, got.AsString(), tc.want)
		}
	}
	status, ok := attrs.Value(attribute.Key("http.response.status_code"))
	if !ok || status.AsInt64() != 409 {
		t.Fatalf("status attribute=%d, want 409", status.AsInt64())
	}
	if attrs.Len() != 4 {
		t.Fatalf("unexpected high-cardinality attributes: %v", attrs)
	}
}

func TestObservabilityDoesNotUseRedirectTargetAsRouteLabel(t *testing.T) {
	var logs bytes.Buffer
	handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), nil, nil)(NewRouter(requestTestDependencies()))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/requests/private-one/../private-two", nil))
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["route"] != "GET /api/v1/requests/{requestId}" || strings.Contains(logs.String(), "private-one") || strings.Contains(logs.String(), "private-two") {
		t.Fatalf("redirect target leaked to route: status=%d entry=%v", w.Code, entry)
	}
}

func TestObservabilityBoundsConnectSlashRedirectWithWildcard(t *testing.T) {
	var logs bytes.Buffer
	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	spans := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(spans), trace.WithSampler(trace.AlwaysSample()))
	defer tp.Shutdown(context.Background())
	mux := http.NewServeMux()
	mux.HandleFunc("CONNECT /tree/{id}/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), mp.Meter("test"), tp.Tracer("test"))(routedHandler{Handler: mux, mux: mux, patterns: map[string]struct{}{"CONNECT /tree/{id}/": {}}})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("CONNECT", "/tree/private-id", nil))
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["route"] != "unmatched" || strings.Contains(logs.String(), "private-id") {
		t.Fatalf("redirect target leaked to route: status=%d entry=%v", w.Code, entry)
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(fmt.Sprint(data), "private-id") || strings.Contains(fmt.Sprint(spans.Ended()[0].Attributes()), "private-id") {
		t.Fatal("redirect target leaked to metric or trace")
	}
}

func TestObservabilityBoundsUnrecognizedMethodAndRoute(t *testing.T) {
	var logs bytes.Buffer
	handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), nil, nil)(http.NewServeMux())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("SECRET_CUSTOM_METHOD", "/private-id", nil))
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["route"] != "unmatched" || entry["method"] != "OTHER" || strings.Contains(logs.String(), "private-id") || strings.Contains(logs.String(), "SECRET_CUSTOM_METHOD") {
		t.Fatalf("unbounded values in log: %v", entry)
	}
}

func TestObservabilityRejectsUnsafeCorrelationIDAndClassifiesFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		class  string
	}{
		{"unauthenticated", 401, "authentication"},
		{"forbidden", 403, "authorization"},
		{"server error", 500, "server"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			mux := http.NewServeMux()
			mux.HandleFunc("GET /test", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status) })
			handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), nil, nil)(mux)
			req := httptest.NewRequest(http.MethodGet, "/test?body=secret-body", nil)
			req.Header.Set("X-Request-ID", "bad\nsecret-injected")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			var entry map[string]any
			if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
				t.Fatal(err)
			}
			id, _ := entry["request_id"].(string)
			if id == "" || id == "bad\nsecret-injected" || w.Header().Get("X-Request-ID") != id || entry["failure_class"] != tc.class {
				t.Fatalf("entry=%v response id=%q", entry, w.Header().Get("X-Request-ID"))
			}
			if strings.Contains(logs.String(), "secret-body") || strings.Contains(logs.String(), "secret-injected") {
				t.Fatalf("sensitive data leaked: %s", logs.String())
			}
		})
	}
}

func TestObservabilityRealRouterAuthenticationFailureKeepsSecretsOutOfSignals(t *testing.T) {
	var logs bytes.Buffer
	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	spans := tracetest.NewSpanRecorder()
	tp := trace.NewTracerProvider(trace.WithSpanProcessor(spans), trace.WithSampler(trace.AlwaysSample()))
	defer tp.Shutdown(context.Background())
	handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), mp.Meter("test"), tp.Tracer("test"))(NewRouter(requestTestDependencies()))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session?state=secret-state", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	req.Header.Set("Cookie", "session=secret-cookie")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized || !strings.Contains(logs.String(), `"route":"GET /api/v1/session"`) || !strings.Contains(logs.String(), `"failure_class":"authentication"`) {
		t.Fatalf("status=%d log=%s", w.Code, logs.String())
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-state", "secret-token", "secret-cookie"} {
		if strings.Contains(logs.String(), secret) || strings.Contains(fmt.Sprint(data), secret) || strings.Contains(fmt.Sprint(spans.Ended()[0].Attributes()), secret) {
			t.Fatalf("secret %q leaked into telemetry", secret)
		}
	}
}

func TestObservabilityRecoversPanicWithoutLoggingValue(t *testing.T) {
	var logs bytes.Buffer
	handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), nil, nil)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret-panic-value")
	}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/secret-path", nil))
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if w.Code != 500 || entry["status"] != float64(500) || entry["failure_class"] != "panic" || strings.Contains(logs.String(), "secret-panic-value") || strings.Contains(logs.String(), "secret-path") || strings.Contains(w.Body.String(), "secret-panic-value") {
		t.Fatalf("panic status=%d log=%v body=%q", w.Code, entry, w.Body.String())
	}
}

func TestObservabilityAbortsCommittedResponseAfterPanic(t *testing.T) {
	var logs bytes.Buffer
	reader := metric.NewManualReader()
	mp := metric.NewMeterProvider(metric.WithReader(reader))
	defer mp.Shutdown(context.Background())
	handler := NewObservabilityMiddleware(slog.New(slog.NewJSONHandler(&logs, nil)), mp.Meter("test"), nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("partial"))
		panic("secret-panic-value")
	}))
	var panicked any
	func() {
		defer func() { panicked = recover() }()
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	}()
	if panicked != http.ErrAbortHandler || strings.Contains(logs.String(), "secret-panic-value") || !strings.Contains(logs.String(), `"failure_class":"panic"`) || !strings.Contains(logs.String(), `"status":200`) {
		t.Fatalf("panic=%v log=%s", panicked, logs.String())
	}
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatal(err)
	}
	foundFailure := false
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != "http.server.request.failure.count" {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || len(sum.DataPoints) != 1 || sum.DataPoints[0].Value != 1 {
				t.Fatalf("panic failure count=%v", m.Data)
			}
			attrs := sum.DataPoints[0].Attributes
			status, hasStatus := attrs.Value(attribute.Key("http.response.status_code"))
			class, hasClass := attrs.Value(attribute.Key("failure.class"))
			foundFailure = hasStatus && status.AsInt64() == 200 && hasClass && class.AsString() == "panic"
		}
	}
	if !foundFailure {
		t.Fatalf("panic metric status/class missing: %v", data)
	}
}
