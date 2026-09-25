package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

const requestIDHeader = "X-Request-ID"

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// NewObservabilityMiddleware instruments the outer HTTP boundary. Neither
// request contents nor user-controlled paths become telemetry attributes.
func NewObservabilityMiddleware(logger *slog.Logger, meter metric.Meter, tracer trace.Tracer) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	if meter == nil {
		meter = metricnoop.NewMeterProvider().Meter("httpapi")
	}
	if tracer == nil {
		tracer = tracenoop.NewTracerProvider().Tracer("httpapi")
	}
	requests, _ := meter.Int64Counter("http.server.request.count")
	duration, _ := meter.Float64Histogram("http.server.request.duration", metric.WithUnit("s"))
	failures, _ := meter.Int64Counter("http.server.request.failure.count")
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			method := boundedMethod(r.Method)
			id := r.Header.Get(requestIDHeader)
			if !safeRequestID.MatchString(id) || len(r.Header.Values(requestIDHeader)) != 1 {
				var bytes [16]byte
				if _, err := rand.Read(bytes[:]); err != nil {
					panic("cannot generate request ID")
				}
				id = hex.EncodeToString(bytes[:])
			}
			w.Header().Set(requestIDHeader, id)
			started := time.Now()
			ctx, span := tracer.Start(r.Context(), "HTTP "+method, trace.WithSpanKind(trace.SpanKindServer))
			defer span.End()
			writer := &observedResponseWriter{ResponseWriter: w}
			defer func() {
				panicked := recover()
				committedBeforePanic := writer.status != 0
				if panicked != nil && writer.status == 0 {
					WriteError(writer, APIError{Status: http.StatusInternalServerError, Code: "internal_error"})
				}
				status := writer.status
				if status == 0 {
					status = http.StatusOK
				}
				route := routePattern(next, r)
				if route == "" {
					route = "unmatched"
				}
				failure := failureClass(status, panicked != nil)
				attrs := metric.WithAttributes(attribute.String("http.route", route), attribute.String("http.request.method", method), attribute.Int("http.response.status_code", status), attribute.String("failure.class", failure))
				requests.Add(ctx, 1, attrs)
				duration.Record(ctx, time.Since(started).Seconds(), attrs)
				if failure != "none" {
					failures.Add(ctx, 1, attrs)
					span.SetStatus(codes.Error, failure)
				}
				span.SetName("HTTP " + route)
				span.SetAttributes(attribute.String("http.route", route), attribute.String("http.request.method", method), attribute.Int("http.response.status_code", status), attribute.String("failure.class", failure))
				fields := []any{"request_id", id, "method", method, "route", route, "status", status, "duration_ms", float64(time.Since(started).Microseconds()) / 1000, "failure_class", failure}
				if span.SpanContext().IsSampled() {
					fields = append(fields, "trace_id", span.SpanContext().TraceID().String())
				}
				logger.Info("http request completed", fields...)
				if panicked != nil && committedBeforePanic {
					panic(http.ErrAbortHandler)
				}
			}()
			next.ServeHTTP(writer, r.WithContext(ctx))
		})
	}
}

func boundedMethod(method string) string {
	switch method {
	case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS":
		return method
	default:
		return "OTHER"
	}
}

func routePattern(handler http.Handler, request *http.Request) string {
	if routed, ok := handler.(routedHandler); ok {
		return routed.RoutePattern(request)
	}
	return ""
}

func failureClass(status int, panicked bool) string {
	if panicked {
		return "panic"
	}
	switch status {
	case http.StatusUnauthorized:
		return "authentication"
	case http.StatusForbidden:
		return "authorization"
	case http.StatusConflict:
		return "conflict"
	}
	if status >= 500 {
		return "server"
	}
	if status >= 400 {
		return "client"
	}
	return "none"
}

type observedResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *observedResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *observedResponseWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(p)
}

func (w *observedResponseWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
