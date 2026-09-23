# Task 8 completion — OpenAPI verification

Date: 2026-09-23
Worktree: `/Users/nao/.codex/worktrees/task8-openapi-verification/learn-ai-driven-development`

## Status

Task 8.4 verification is complete with one environment limitation: the exact
frozen pnpm install could not write the existing worktree `node_modules`
metadata. All remaining requested checks passed using the already-installed
dependencies and writable Go/TypeScript cache paths.

Final OpenAPI review correction: every implementation route now documents the
matching 403 CORS/CSRF and 500 internal-error responses. The contract test
asserts these response references for every affected operation.

## Changed files in the Task 8 scope

- OpenAPI contract and verification: `api/openapi.yaml`,
  `scripts/verify-openapi.mjs`, `scripts/verify-openapi.test.mjs`.
- Package/tooling: `package.json`, `pnpm-lock.yaml`,
  `docs/development/toolchain.md`.
- Local operation: `cmd/migrate-local/`, `compose.local.yaml`,
  `scripts/provision-keycloak.sh`, `scripts/provision-keycloak.test.sh`,
  `keycloak/realms/approval-flow-dev-realm.json`,
  `docs/development/local-api.md`.
- API/config/CORS alignment: `internal/config/`, `internal/httpapi/`,
  `cmd/api/main_test.go` and related tests.
- This completion record and the SDD ledger.

## Requirement traceability

- FR-001, FR-002, FR-003, FR-004, FR-005, FR-007, FR-011, FR-013: API routes,
  OIDC/session boundaries, request lifecycle, approvals, audit events,
  PostgreSQL migrations, and concurrency-safe integration coverage.
- FR-012 and NFR-006: reproducible local Node/pnpm/Go/PostgreSQL/Keycloak
  operation, bounded Keycloak readiness retry, and OpenAPI contract
  verification.
- NFR-001: loopback-only local services, exact-origin credentialed CORS,
  PKCE, secret confinement, and no committed credentials.
- NFR-003 and NFR-004: formatter/lint/type/static checks, layered tests, and
  explicit verification records.
- NFR-002: this record plus the SDD ledger preserve decision and command
  traceability.

NFR-005 is outside Task 8 scope and remains a follow-up. Task 8 introduced no
observability stack, so it does not claim coverage for metrics, logs, traces,
or alerting requirements.

## Decision traceability

- ADR-002 / ADR-003 / ADR-006 / ADR-007 / ADR-012: Go HTTP API, OpenAPI
  contract, layered verification, pnpm supply-chain controls, and Biome/
  gofmt/go vet checks.
- ADR-004: PostgreSQL, checked-in ordered SQL migrations, explicit migration
  execution, and isolated PostgreSQL integration. `cmd/migrate-local` rejects
  non-loopback URLs and unsupported query options.
- ADR-009: Keycloak 26.7.4 pinned by digest, loopback-only binding, imported
  development realm, S256 PKCE, and externally supplied credentials.
- ADR-015: `APP_FRONTEND_ORIGIN` is the sole origin source; CORS allows one
  exact origin with credentials, `Vary: Origin`, and the documented minimal
  methods/headers. The implementation does not accept the legacy
  `APP_ALLOWED_ORIGIN` alias.

## Audit results

- OpenAPI operationId ↔ router: all 13 contract operations have matching Go
  routes and handlers; no missing or duplicate operation was found.
- Config environment ↔ documentation: required API variables in
  `internal/config/config.go` are documented in `local-api.md`; the document
  also explicitly marks test/Keycloak credentials as external inputs.
- Migration/Keycloak artifacts ↔ ADR-004/009: ordered migrations, explicit
  CLI use, isolated test compose, pinned Keycloak image/realm, loopback port,
  PKCE, and no application authorization based on Keycloak roles match the
  accepted decisions.
- Origin/CORS ↔ ADR-015: implementation and tests use only
  `APP_FRONTEND_ORIGIN`, exact origin matching, credentialed responses, safe
  preflight methods/headers, and rejection of unknown origins.
- Secrets: no live credential, client secret, private key, production URL, or
  transaction key was found. Test fixtures contain only disposable placeholder
  strings and the provisioner test verifies that supplied secrets stay out of
  argv/output.

## Verification commands and results

| Command | Result |
|---|---|
| `pnpm install --frozen-lockfile` | BLOCKED: worktree `node_modules/.pnpm-workspace-state-v1.json` is not writable (`ERR_PNPM_WORKSPACE_STATE_WRITE_IO`). A `/private/tmp` store/virtual-store retry also failed at importer symlink creation under the protected worktree. |
| `pnpm run format:check` | PASS — 10 files, no fixes |
| `pnpm run lint` | PASS — 10 files, no fixes |
| `pnpm run typecheck` | BLOCKED by denied default `tsconfig.tsbuildinfo` path |
| `pnpm exec tsc --project tsconfig.json --pretty false --tsBuildInfoFile /private/tmp/learn-ai-task8.tsbuildinfo` | PASS |
| `pnpm run check:gofmt` | PASS |
| `pnpm run verify:openapi` | PASS — contract valid |
| `node --test scripts/verify-openapi.test.mjs` | PASS — 11 passed, 1 skipped, 0 failed |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOMODCACHE=/private/tmp/learn-ai-go-modcache GOTOOLCHAIN=go1.27.1 go vet ./...` | PASS |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOMODCACHE=/private/tmp/learn-ai-go-modcache GOTOOLCHAIN=go1.27.1 go test ./cmd/api ./cmd/migrate-local ./internal/application/requests ./internal/auth ./internal/config ./internal/domain ./internal/httpapi -count=1` | PASS |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOMODCACHE=/private/tmp/learn-ai-go-modcache GOTOOLCHAIN=go1.27.1 go test ./cmd/migrate-local -count=1` | PASS |
| `GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db` | PASS — isolated PostgreSQL container/network created, tests passed, cleanup completed |
| `GOTOOLCHAIN=go1.27.1 GOMODCACHE=/private/tmp/learn-ai-go-modcache go mod verify` | PASS — all modules verified |
| `bash -n scripts/provision-keycloak.sh scripts/provision-keycloak.test.sh` | PASS |
| `bash scripts/provision-keycloak.test.sh` | PASS |
| `jq -e . keycloak/realms/approval-flow-dev-realm.json` | PASS |
| `docker compose -f compose.local.yaml config --quiet` with non-secret review inputs | PASS |
| `docker compose -f compose.test.yaml config --quiet` | PASS |
| `git diff --check` | PASS |

## Live-Keycloak limitation

No live Keycloak container smoke test was run in this verification pass. The
compose file was rendered successfully, the realm JSON parsed, and the
provisioner behavior test used a hermetic fake transport to verify stdin-only
secret handling and cleanup. A live run against the pinned image remains an
operational follow-up when the image is available and disposable local
credentials can be supplied.

## Follow-ups and unresolved concerns

1. Re-run `pnpm install --frozen-lockfile` in CI or a checkout whose
   `node_modules` and pnpm workspace metadata are writable; do not weaken the
   frozen-install gate.
2. Run the live Keycloak smoke path with disposable credentials and verify the
   authorization-code/PKCE callback end to end.
3. Clean up the historical `APP_ALLOWED_ORIGIN` wording in the Context and
   Option A sections of accepted ADR-015. Its Decision section already states
   `APP_FRONTEND_ORIGIN` as the sole canonical name, and implementation/tests
   follow that Decision; changing the accepted record should preserve history
   according to the project decision policy.
