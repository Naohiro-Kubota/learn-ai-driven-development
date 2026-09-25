package main

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"time"

	"github.com/Naohiro-Kubota/learn-ai-driven-development/internal/config"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
	tracenoop "go.opentelemetry.io/otel/trace/noop"
)

type telemetryRuntime struct {
	meter    metric.Meter
	tracer   trace.Tracer
	shutdown func(context.Context) error
}

func newTelemetry(ctx context.Context, cfg config.Config, logger *slog.Logger) (telemetryRuntime, error) {
	if cfg.OTLPEndpoint == "" {
		return telemetryRuntime{
			meter:    metricnoop.NewMeterProvider().Meter("httpapi"),
			tracer:   tracenoop.NewTracerProvider().Tracer("httpapi"),
			shutdown: func(context.Context) error { return nil },
		}, nil
	}
	endpoint, err := url.Parse(cfg.OTLPEndpoint)
	if err != nil || endpoint.Host == "" {
		return telemetryRuntime{}, errors.New("invalid telemetry endpoint")
	}
	timeout := cfg.OTLPTimeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	metricOptions := []otlpmetrichttp.Option{otlpmetrichttp.WithEndpoint(endpoint.Host), otlpmetrichttp.WithTimeout(timeout), otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false})}
	traceOptions := []otlptracehttp.Option{otlptracehttp.WithEndpoint(endpoint.Host), otlptracehttp.WithTimeout(timeout), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false})}
	if endpoint.Scheme == "http" {
		metricOptions = append(metricOptions, otlpmetrichttp.WithInsecure())
		traceOptions = append(traceOptions, otlptracehttp.WithInsecure())
	}
	metricExporter, err := otlpmetrichttp.New(ctx, metricOptions...)
	if err != nil {
		return telemetryRuntime{}, errors.New("initialize metric exporter")
	}
	traceExporter, err := otlptracehttp.New(ctx, traceOptions...)
	if err != nil {
		_ = metricExporter.Shutdown(ctx)
		return telemetryRuntime{}, errors.New("initialize trace exporter")
	}
	res := resource.NewWithAttributes("", attribute.String("service.name", "approval-flow-api"))
	meterProvider := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(safeMetricExporter{Exporter: metricExporter, logger: logger}, sdkmetric.WithInterval(15*time.Second))),
	)
	traceProvider := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRatio))),
		sdktrace.WithBatcher(safeTraceExporter{SpanExporter: traceExporter, logger: logger}),
	)
	return telemetryRuntime{
		meter:  meterProvider.Meter("httpapi"),
		tracer: traceProvider.Tracer("httpapi"),
		shutdown: func(ctx context.Context) error {
			flushErr := errors.Join(traceProvider.ForceFlush(ctx), meterProvider.ForceFlush(ctx))
			shutdownErr := errors.Join(traceProvider.Shutdown(ctx), meterProvider.Shutdown(ctx))
			return errors.Join(flushErr, shutdownErr)
		},
	}, nil
}

type safeMetricExporter struct {
	sdkmetric.Exporter
	logger *slog.Logger
}

func (e safeMetricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) error {
	err := e.Exporter.Export(ctx, data)
	if err != nil {
		e.logger.Warn("telemetry export failed", "signal", "metrics")
		return errors.New("metrics export failed")
	}
	return nil
}

type safeTraceExporter struct {
	sdktrace.SpanExporter
	logger *slog.Logger
}

func (e safeTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	err := e.SpanExporter.ExportSpans(ctx, spans)
	if err != nil {
		e.logger.Warn("telemetry export failed", "signal", "traces")
		return errors.New("traces export failed")
	}
	return nil
}
