# Local Compose Stack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Start the local Frontend, Backend, PostgreSQL, and Keycloak with Docker Compose and make the login flow usable after documented local provisioning.

**Architecture:** Keep the browser origins fixed at `127.0.0.1:5173`, `:8080`, and `:8081` while containers communicate on the Compose bridge. The Go OIDC client retains the public issuer and uses a development-only dial override for Keycloak; API and migration configuration admits the Compose service addresses only in explicit development mode. A migration job completes before API startup. Provisioning maps real Keycloak subjects to local app memberships.

**Tech Stack:** Go 1.27.1, TypeScript/Node 26.9.0, pnpm 12.5.1, PostgreSQL 17.11, Keycloak 26.7.4, Docker Compose.

**Spec:** `docs/decisions/architecture/ADR-018-local-compose-application-stack.md` (Accepted)

## Global Constraints

- Maintain ADR-004, ADR-009, ADR-011, ADR-015 and NFR-001/NFR-006.
- Bind every published port to host `127.0.0.1`; no secret values in tracked files.
- Keep `compose.e2e.yaml`, `compose.test.yaml`, and their volumes isolated.
- Keep issuer validation, CSRF, and Origin checks unchanged.

## Review Focus

- Backend container starts only when DB and Keycloak are ready; a failed migration prevents API startup.
- OIDC discovery, token exchange, and JWKS reach Keycloak while issuer remains the public loopback URL.
- Production settings cannot activate development-only all-interface binding or OIDC dial override.
- Restarting provisioning preserves local application data and current member mappings.
- A developer can stop the stack without removing the development database unless explicitly choosing `down -v`.

---

### Task 1: Compose-aware API and migration configuration

**Files:** `backend/internal/config/config.go`, `backend/internal/config/config_test.go`, `backend/internal/auth/oidc.go`, `backend/internal/auth/oidc_test.go`, `backend/cmd/migrate-local/main.go`, `backend/cmd/migrate-local/main_test.go`.

**Interfaces:** Add one explicit local Compose mode and a validated OIDC internal dial target; preserve `OIDC_ISSUER` as the expected issuer and public authorization endpoint.

- [x] Write failing Go tests for allowed Compose development settings, rejected production settings, and OIDC dial override retaining the expected issuer.
- [x] Run the focused tests and confirm they fail for missing behavior.
- [x] Implement the minimum config and transport changes; use the existing Go standard library and current OIDC library.
- [x] Run focused Go tests, `go vet ./...`, and `gofmt` check.

### Task 2: Container build and Compose startup

**Files:** `compose.local.yaml`, `backend/Dockerfile.local`, `frontend/Dockerfile.local`, `.dockerignore` (or equivalent root build files), and a Compose configuration test.

**Interfaces:** `docker compose -f compose.local.yaml up --build -d` starts PostgreSQL, Keycloak, one-shot migration, Backend, and Frontend. Credentials and the 32-byte auth key come from the caller environment.

- [x] Write a configuration test asserting service names, loopback-bound published ports, migration dependency, and separation from test/E2E volumes.
- [x] Run that test and confirm it fails before changes.
- [x] Add pinned-toolchain build definitions and Compose services with health/dependency conditions.
- [x] Run the configuration test and `docker compose -f compose.local.yaml config` with disposable test environment values.

### Task 3: Idempotent local identity and membership provisioning

**Files:** `scripts/provision-keycloak.sh`, a focused local seed script and tests, `docs/development/local-api.md`, `docs/development/frontend-local-development.md`, `README.md`, `.env.example`.

**Interfaces:** A documented provisioning command reads externally supplied credentials, obtains verified Keycloak subjects, and upserts local Organization, Member, Role, identity, and default approver data without deleting unrelated rows.

- [x] Write failing tests for repeat provisioning, invalid or missing subjects, and no secret output.
- [x] Run those tests and confirm failures for missing behavior.
- [x] Implement the provisioning script and document setup, startup, verification, and shutdown commands.
- [x] Run provisioning tests and a fresh-volume browser login smoke test.

### Task 4: Final validation and review

- [x] Run `pnpm run check`, `pnpm test`, Go tests and vet, Compose configuration, and browser E2E where applicable.
- [x] Request an independent reviewer; fix valid findings and re-review.
- [x] Confirm the changed files and test results against ADR-018 and report residual limits.
