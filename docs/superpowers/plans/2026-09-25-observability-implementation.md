# Observability Implementation Plan

> **For agentic workers:** Implement against Accepted ADR-019, then obtain an independent reviewer verdict before PR creation.

**Goal:** Deliver the application-side logs, metrics, and traces specified by ADR-019, with tests and local operating guidance.

**Architecture:** Keep HTTP instrumentation outside domain and persistence logic. Emit bounded JSON request logs through `log/slog`; use OpenTelemetry Go for HTTP metrics and traces and configurable OTLP export. Telemetry failure must not change the business response or Audit Event.

**Tech Stack:** Go `net/http`, `log/slog`, OpenTelemetry Go, existing Go tests and project quality commands.

**Spec:** `docs/decisions/architecture/ADR-019-observability-signal-architecture.md` (Accepted)

## Global constraints

- Preserve ADR-004 Audit Event semantics and ADR-005/011 authentication secrecy.
- Do not add identity, raw path, request body, header values, or error strings to metrics or spans.
- Keep exporter endpoint and credentials in environment configuration. Do not select a monitoring service or retention period.
- No frontend RUM or background processor is currently in scope.

## Task 1: HTTP observability and tests

**Files:** `backend/internal/httpapi/` instrumentation and tests; `backend/internal/config/` configuration and tests; `backend/cmd/api/` wiring and tests; `backend/go.mod`, `backend/go.sum`.

- [x] Add failing tests for one completion log per success and 4xx/5xx response, bounded route labels, safe correlation ID generation/validation, and no sensitive data in logs, metric attributes, or span attributes.
- [x] Add failing tests for request count, duration, failure reporting, trace correlation, exporter failure isolation, and clean shutdown flush.
- [x] Implement `slog` JSON completion logs and OTel HTTP metrics/traces at the outer HTTP boundary. Ensure panic paths are classified and rethrown for existing `net/http` recovery behavior or recovered safely without corrupting the response.
- [x] Add opt-in OTLP exporter configuration, with validated endpoint/transport and bounded export timeout. A disabled exporter must leave local metrics/traces testable and avoid external network use.
- [x] Run focused Go tests, `go test ./...`, `go vet ./...`, `gofmt`, and verify the tests observe representative 401/403/409/5xx cases.

## Task 2: Documentation and integration verification

**Files:** `docs/development/local-api.md`, relevant architecture/developer docs, and tests or scripts only where required to verify the implementation.

- [x] Document local log fields, opt-in export settings, correlation behavior, shutdown behavior, and diagnostic procedure without exposing credentials.
- [x] Link NFR-005 and ADR-019, explain the separation from Audit Events and the intentionally undecided monitoring backend/alerts/retention.
- [x] Run `pnpm run check`, applicable Go tests and integration tests, `git diff --check`, and confirm no previously failing quality gate was weakened.
- [x] Review the full diff against every Decision and Validation item in ADR-019 before independent review.
