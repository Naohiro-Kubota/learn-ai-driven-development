# Go API Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the OpenAPI 3.1 contract for the initial Vertical Slice's authenticated Go HTTP API without adding unapproved application dependencies.

**Architecture:** A Go `net/http` adapter delegates to application services, which enforce workflow and authorization rules through repository interfaces. PostgreSQL repositories atomically apply request changes, approval changes, audit events, OIDC authentication transactions, and opaque sessions. OIDC and session middleware establish the authenticated Actor before handlers run; handlers only decode DTOs, require CSRF protection for unsafe methods, and translate typed application errors into the OpenAPI error model.

**Tech Stack:** Go 1.27.1; `net/http`, `database/sql`, `httptest`, and standard `crypto` packages; PostgreSQL with pgx stdlib v5.11.0; golang-migrate v4.20.1; `github.com/coreos/go-oidc/v3` v3.21.0; `golang.org/x/oauth2` v0.37.0; OpenAPI 3.1 contract at `api/openapi.yaml`.

**Spec:** `docs/superpowers/specs/2026-09-20-api-contract-design.md`

## Global Constraints

- Implement exactly the operations and schemas in `api/openapi.yaml`; do not add Reject, Cancel, notifications, workflow management, or frontend code.
- Use Go 1.27.1 and exact module versions selected by ADR-004 and ADR-010; do not add runtime ORM, router, session, JWT, or test-container dependencies. Task 8's exact `yaml` development dependency is the sole contract-parser exception.
- Use Go 1.22+ `ServeMux` method-aware patterns and `Request.PathValue`, as required by ADR-002.
- Keep domain authorization, state transition, version comparison, and audit recording out of HTTP handlers.
- Store only opaque CSPRNG cookie values in browsers; hash cookie values in PostgreSQL. Do not log OIDC tokens, PKCE verifiers, state, nonce, session cookies, or CSRF tokens.
- Every unsafe `/api/v1` operation requires the session-bound `X-CSRF-Token` and an allowed `Origin`; do not treat `SameSite` as sufficient CSRF protection.
- Submit and Approve must update the Request version and append the Audit Event in one PostgreSQL transaction; stale versions must return `409 version_conflict`.
- PostgreSQL integration tests use a pre-provisioned `TEST_DATABASE_URL`; tests must never substitute SQLite or an in-memory DB for transaction/concurrency coverage.
- Existing uncommitted toolchain and decision-document changes are not part of this plan's commits unless they are intentionally included by the human owner.

## Review Focus

- A title containing only whitespace must be rejected after trimming, while a description may be empty and must remain visible in the audit snapshot.
- Two concurrent Submit or Approve calls carrying the same `expectedVersion` must yield exactly one success and one `409 version_conflict`; no duplicate Audit Event may be written.
- A Requester must not update or submit another Requester's Draft, and an unassigned Approver must not approve a known Pending Request; both mutations return `403 forbidden`.
- An OIDC callback replay, mismatched state, nonce, transaction cookie, or PKCE verifier must consume or reject the transaction and never issue a session.
- A valid session with a missing/wrong CSRF token or disallowed Origin must not create, update, Submit, Approve, or logout; it returns `403 csrf_validation_failed`.

---

### Task 1: Add OIDC modules and validated runtime configuration

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Interfaces:**
- Produces: `config.Load(lookup func(string) string) (config.Config, error)`.
- Produces: `config.Config` containing database URL, listener address, allowed origin, OIDC issuer/client ID/redirect URI, and cookie/session secret settings.
- Consumed by: `cmd/api/main.go`, OIDC client construction, session middleware, PostgreSQL setup.

- [ ] **Step 1: Write failing configuration tests**

```go
func TestLoadRejectsProductionWithoutSecureCookie(t *testing.T) {
	_, err := Load(func(key string) string {
		return map[string]string{
			"APP_ENV": "production", "APP_COOKIE_SECURE": "false",
		}[key]
	})
	if err == nil { t.Fatal("expected validation error") }
}

func TestLoadAllowsInsecureCookieOnlyForLoopbackDevelopment(t *testing.T) {
	values := map[string]string{
		"APP_ENV": "development", "APP_COOKIE_SECURE": "false",
		"APP_LISTEN_ADDR": "127.0.0.1:8080", "APP_ALLOWED_ORIGIN": "http://127.0.0.1:5173",
		"DATABASE_URL": "postgres://test", "OIDC_ISSUER": "http://127.0.0.1:8081/realms/dev",
		"OIDC_CLIENT_ID": "approval-flow", "OIDC_REDIRECT_URI": "http://127.0.0.1:8080/auth/oidc/callback",
		"AUTH_TRANSACTION_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"SESSION_IDLE_TTL": "15m", "SESSION_ABSOLUTE_TTL": "8h", "AUTH_TRANSACTION_TTL": "5m",
	}
	cfg, err := Load(func(key string) string { return values[key] })
	if err != nil { t.Fatalf("Load() error = %v", err) }
	if cfg.CookieSecure { t.Fatal("CookieSecure = true") }
}
```

- [ ] **Step 2: Run the focused test to verify failure**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/config -run TestLoad -count=1`

Expected: FAIL because the package and `Load` do not exist.

- [ ] **Step 3: Add exact OIDC dependencies and configuration validation**

Add direct requirements for `github.com/coreos/go-oidc/v3 v3.21.0` and `golang.org/x/oauth2 v0.37.0`. Implement `Config` with required non-empty configuration values and positive duration parsing. Require a 32-byte base64-decoded auth-transaction encryption key. Permit `APP_COOKIE_SECURE=false` only when `APP_ENV=development` and both application and allowed-origin hosts are loopback; otherwise fail at startup.

```go
type Config struct {
	DatabaseURL, ListenAddress, AllowedOrigin string
	OIDCIssuer, OIDCClientID, OIDCRedirectURI string
	CookieSecure bool
	AuthTransactionKey [32]byte
	SessionIdleTTL, SessionAbsoluteTTL, AuthTransactionTTL time.Duration
}
```

- [ ] **Step 4: Run focused tests and module integrity checks**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/config -count=1 && GOTOOLCHAIN=go1.27.1 go mod verify`

Expected: PASS.

- [ ] **Step 5: Commit the task files**

```bash
git add go.mod go.sum internal/config/config.go internal/config/config_test.go
git commit -m "feat: add validated API runtime configuration"
```

### Task 2: Create versioned PostgreSQL schema for workflow, audit, sessions, and OIDC transactions

**Files:**
- Create: `migrations/000001_initial_workflow.up.sql`
- Create: `migrations/000001_initial_workflow.down.sql`
- Create: `migrations/000002_auth_sessions.up.sql`
- Create: `migrations/000002_auth_sessions.down.sql`
- Create: `internal/store/postgres/migrations_test.go`

**Interfaces:**
- Produces: tables for Organization, Member, MemberRole, Request, Approval, AuditEvent, `app_sessions`, and `oidc_auth_transactions`.
- Produces: unique and foreign-key constraints needed by later repository methods.
- Consumed by: all PostgreSQL repositories and integration tests.

- [ ] **Step 1: Write a failing migration integration test**

Create a fresh schema/database from `TEST_DATABASE_URL`, apply every `up` migration with golang-migrate, and assert that required tables and constraints exist. Add a second test that applies `down` migrations in reverse and verifies the schema is empty.

```go
func TestMigrationsCreateWorkflowAndAuthTables(t *testing.T) {
	db := openFreshTestDatabase(t)
	applyUpMigrations(t, db)
	for _, table := range []string{"requests", "approvals", "audit_events", "app_sessions", "oidc_auth_transactions"} {
		assertTableExists(t, db, table)
	}
}
```

- [ ] **Step 2: Run the migration test to verify failure**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestMigrationsCreateWorkflowAndAuthTables -count=1` (with an isolated `TEST_DATABASE_URL` exported)

Expected: FAIL because no migration files exist.

- [ ] **Step 3: Implement the migrations**

Use opaque text IDs in the public-facing entities while keeping internal persistence fields private to repositories. Add:

- `requests.version BIGINT NOT NULL CHECK (version >= 1)` and a status constraint limited to `draft`, `pending`, `approved`.
- one `approvals` row per initial Request with an assignee, `pending`/`approved` status, and unique Request ID.
- append-only `audit_events` with event type, actor Member ID, occurrence time, nullable content snapshot/approval metadata, and no application update/delete path.
- `app_sessions` with a unique cookie hash, Member ID, CSRF token hash, created/last-used/idle-expiry/absolute-expiry/revoked timestamps.
- `oidc_auth_transactions` with unique cookie and state hashes, nonce, encrypted verifier, issuer/client/redirect values, expiry, and consumed timestamp.

Index Pending approvals by assignee and Request status; index active session and transaction lookup keys. Down migrations must remove tables in dependency-safe reverse order.

- [ ] **Step 4: Run migration up/down tests**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestMigrations -count=1` (with an isolated `TEST_DATABASE_URL` exported)

Expected: PASS.

- [ ] **Step 5: Commit the task files**

```bash
git add migrations internal/store/postgres/migrations_test.go
git commit -m "feat: add workflow and auth database migrations"
```

### Task 3: Implement domain workflow and audit application services with unit tests

**Files:**
- Create: `internal/domain/request.go`
- Create: `internal/domain/errors.go`
- Create: `internal/application/requests/service.go`
- Create: `internal/application/requests/service_test.go`
- Create: `internal/application/requests/repository.go`

**Interfaces:**
- Consumes: `Actor{MemberID string, Roles []Role}`, `ExpectedVersion int64`, and repository interfaces.
- Produces: `CreateDraft`, `UpdateDraft`, `Submit`, `Approve`, `Get`, `ListPending`, and `ListAuditEvents` methods plus typed errors (`ErrForbidden`, `ErrNotFound`, `ErrVersionConflict`, `ErrInvalidState`, `ErrApprovalRoutingUnavailable`).
- Consumed by: PostgreSQL adapter and HTTP handlers.

- [ ] **Step 1: Write failing table-driven service tests**

Cover Title trimming, empty Description, Draft-only update, self-submission to the default Approver, missing default Approver, wrong Requester, wrong Approver, stale versions, and audit snapshots.

```go
func TestSubmitRejectsStaleVersionWithoutAuditEvent(t *testing.T) {
	repo := newFakeRepository(draftRequest(7))
	_, err := service.Submit(ctx, requester, requestID, 6)
	if !errors.Is(err, ErrVersionConflict) { t.Fatalf("got %v", err) }
	if got := repo.AuditEventCount(); got != 0 { t.Fatalf("events = %d", got) }
}
```

- [ ] **Step 2: Run unit tests to verify failure**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests -count=1`

Expected: FAIL because the service and typed errors do not exist.

- [ ] **Step 3: Implement pure application rules**

Keep HTTP, SQL, and OIDC types out of the package. Normalize Title with `strings.TrimSpace`, reject an empty normalized Title, and enforce PDR-001 bounds. Enforce PDR-002 at Submit, store the assigned Approver in the Approval returned by the repository, and create one audit event per successful mutation.

```go
type Repository interface {
	CreateDraft(context.Context, CreateDraftCommand) (domain.Request, error)
	MutateDraft(context.Context, DraftMutationCommand) (domain.Request, error)
	Submit(context.Context, SubmitCommand) (domain.Request, error)
	Approve(context.Context, ApproveCommand) (domain.Request, error)
}
```

- [ ] **Step 4: Run unit tests**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/domain ./internal/application/requests -count=1`

Expected: PASS.

- [ ] **Step 5: Commit the task files**

```bash
git add internal/domain internal/application/requests
git commit -m "feat: add request workflow application service"
```

### Task 4: Implement PostgreSQL repositories and transactional concurrency tests

**Files:**
- Create: `internal/store/postgres/requests.go`
- Create: `internal/store/postgres/requests_test.go`
- Create: `internal/store/postgres/seed_test.go`

**Interfaces:**
- Implements: `internal/application/requests.Repository`.
- Consumes: `*sql.DB` and the migrations from Task 2.
- Produces: transactional Draft/Submit/Approve persistence and DTO-ready domain values.

- [ ] **Step 1: Write failing PostgreSQL integration tests**

Seed one Organization, Requester, Approver, Admin, and default Approver. Test that the conditional update predicate includes both `id`, expected `version`, and expected current state. Start two goroutines with the same expected version and assert only one can Submit or Approve.

```go
func TestApproveIsAtomicWithAuditEvent(t *testing.T) {
	request := seedPendingRequest(t, db, approverID, 3)
	_, err := repo.Approve(ctx, ApproveCommand{RequestID: request.ID, ExpectedVersion: 3, Actor: approver})
	requireNoError(t, err)
	assertRequestVersion(t, db, request.ID, 4)
	assertAuditEventTypes(t, db, request.ID, "request_submitted", "request_approved")
}
```

- [ ] **Step 2: Run integration tests to verify failure**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run 'Test(Submit|Approve)' -count=1` (with an isolated `TEST_DATABASE_URL` exported)

Expected: FAIL because no repository implementation exists.

- [ ] **Step 3: Implement explicit SQL repositories**

Use `BEGIN`/`COMMIT` for Submit and Approve. In each transaction, update `requests` with `WHERE id = $1 AND version = $2 AND status = $3`, check `RowsAffected`, write the corresponding Approval/Audit Event, then commit. Return a typed conflict/state error without writing an event when the update affects zero rows. Do not expose table rows directly; map them to domain DTOs.

- [ ] **Step 4: Run all PostgreSQL repository tests**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -count=1` (with an isolated `TEST_DATABASE_URL` exported)

Expected: PASS, including concurrent mutation cases.

- [ ] **Step 5: Commit the task files**

```bash
git add internal/store/postgres
git commit -m "feat: persist workflow transitions atomically"
```

### Task 5: Implement OIDC transactions and opaque server-side sessions

**Files:**
- Create: `internal/auth/oidc.go`
- Create: `internal/auth/oidc_test.go`
- Create: `internal/auth/session.go`
- Create: `internal/auth/session_test.go`
- Create: `internal/store/postgres/sessions.go`
- Create: `internal/store/postgres/sessions_test.go`

**Interfaces:**
- Produces: `Authenticator.BeginLogin`, `Authenticator.CompleteLogin`, `SessionStore.Create`, `SessionStore.Authenticate`, and `SessionStore.Revoke`.
- Consumes: OIDC issuer/client/redirect configuration, `oidc.Provider`, `oauth2.Config`, encryption key, and PostgreSQL auth tables.
- Consumed by: HTTP login/callback handlers and authentication middleware.

- [ ] **Step 1: Write failing auth and session tests**

Use `httptest.Server` as an OIDC discovery/JWKS/token-endpoint test double. Test state mismatch, nonce mismatch, invalid issuer/audience/signature, code exchange with the wrong verifier, callback replay, encrypted verifier storage, logout, expired session, and CSRF token verification.

```go
func TestCompleteLoginConsumesTransactionBeforeIssuingSession(t *testing.T) {
	tx := createAuthTransaction(t)
	_, err := auth.CompleteLogin(ctx, tx.Cookie, tx.State, "authorization-code")
	requireNoError(t, err)
	_, err = auth.CompleteLogin(ctx, tx.Cookie, tx.State, "authorization-code")
	if !errors.Is(err, ErrInvalidAuthTransaction) { t.Fatalf("got %v", err) }
}
```

- [ ] **Step 2: Run focused auth tests to verify failure**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/auth -count=1`

Expected: FAIL because authentication and session packages do not exist.

- [ ] **Step 3: Implement the OIDC and session boundary**

Use `oidc.NewProvider`, one long-lived `Provider.VerifierContext`, `oauth2.GenerateVerifier`, `oauth2.S256ChallengeOption`, `oidc.Nonce`, and `oauth2.VerifierOption`. Generate state, nonce, verifier, transaction cookie, session cookie, and CSRF token with `crypto/rand`. Encrypt only the stored PKCE verifier using AES-GCM with the configured 32-byte key; hash cookie and CSRF tokens with SHA-256 before storage. Implement `SessionStore.IssueCSRFToken` to atomically replace the stored CSRF-token hash and return the raw token only to `GET /api/v1/session`; never persist or log the raw token. Match `iss` and `sub` only after token verification and map them to a Member through an explicit repository method.

Issue production `__Host-approval_flow_session` cookies with `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, and no Domain. Use a distinct development-only cookie name when the configuration permits loopback HTTP. Delete/expire the transaction record on every callback outcome after it has been identified.

- [ ] **Step 4: Run authentication and session tests**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/auth ./internal/store/postgres -run 'Test(CompleteLogin|Session|Csrf)' -count=1` (with an isolated `TEST_DATABASE_URL` exported)

Expected: PASS.

- [ ] **Step 5: Commit the task files**

```bash
git add internal/auth internal/store/postgres/sessions.go internal/store/postgres/sessions_test.go
git commit -m "feat: add OIDC login and opaque sessions"
```

### Task 6: Add HTTP middleware, errors, OIDC endpoints, and session endpoints

**Files:**
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/auth_handlers.go`
- Create: `internal/httpapi/session_handlers.go`
- Create: `internal/httpapi/errors.go`
- Create: `internal/httpapi/auth_handlers_test.go`
- Create: `internal/httpapi/session_handlers_test.go`
- Create: `cmd/api/main.go`

**Interfaces:**
- Consumes: `auth.Authenticator`, `auth.SessionStore`, `config.Config`, and request application service.
- Produces: `http.Handler` with `GET /auth/oidc/login`, `GET /auth/oidc/callback`, `GET /api/v1/session`, and `POST /api/v1/session/logout`.
- Produces: `WriteError(http.ResponseWriter, APIError)` mapping typed errors to OpenAPI `ErrorResponse`.

- [ ] **Step 1: Write failing handler tests**

Assert exact route/method behavior, 302 Location and Set-Cookie on login/callback, 401 for missing/expired session, 403 `csrf_validation_failed` for missing/wrong CSRF token or Origin, 204 and cookie clearing on logout, and no token/verifier in JSON or logs.

```go
func TestLogoutRejectsMissingCSRFToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/session/logout", nil)
	r.AddCookie(validSessionCookie(t))
	r.Header.Set("Origin", allowedOrigin)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden { t.Fatalf("status = %d", rr.Code) }
	assertErrorCode(t, rr, "csrf_validation_failed")
}
```

- [ ] **Step 2: Run HTTP auth/session tests to verify failure**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Login|Callback|Session|Logout)' -count=1`

Expected: FAIL because router and handlers do not exist.

- [ ] **Step 3: Implement router and handlers**

Register method-aware `ServeMux` patterns. Authentication middleware loads the opaque session once and stores only `application.Actor` in `request.Context`. The CSRF middleware runs only for unsafe `/api/v1` methods and requires exact allowed Origin plus the session-bound header token. Handlers must use the approved error codes and must redirect only to configured local UI paths, never a query-supplied return URL.

```go
mux.Handle("GET /auth/oidc/login", beginLoginHandler)
mux.Handle("GET /auth/oidc/callback", completeLoginHandler)
mux.Handle("GET /api/v1/session", requireSession(currentSessionHandler))
mux.Handle("POST /api/v1/session/logout", requireCSRF(requireSession(logoutHandler)))
```

- [ ] **Step 4: Run HTTP auth/session tests**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Login|Callback|Session|Logout)' -count=1` (with an isolated `TEST_DATABASE_URL` exported)

Expected: PASS.

- [ ] **Step 5: Commit the task files**

```bash
git add cmd/api/main.go internal/httpapi
git commit -m "feat: expose OIDC and session HTTP endpoints"
```

### Task 7: Implement Request, Approval, and Audit HTTP operations

**Files:**
- Create: `internal/httpapi/request_handlers.go`
- Create: `internal/httpapi/request_handlers_test.go`
- Create: `internal/httpapi/response_dto.go`
- Create: `internal/httpapi/response_dto_test.go`
- Modify: `internal/httpapi/router.go`

**Interfaces:**
- Consumes: authenticated Actor from request context and `application/requests.Service`.
- Produces: all `/api/v1/requests` operations and OpenAPI-shaped JSON DTOs.
- Produces: `201` plus Location for Draft creation, `200` for mutation success, `404 request_not_found` for invisible reads, and OpenAPI error responses for all failure modes.

- [ ] **Step 1: Write failing HTTP contract tests**

Write tests against `httptest` for every OpenAPI operation. Include valid Requester/Approver sessions, JSON decode errors, field errors, forbidden mutations, invisible reads, stale versions, invalid state, approval routing failure, and the Audit Event snapshot fields.

```go
func TestSubmitStaleVersionReturns409AndDoesNotCreateExtraAuditEvent(t *testing.T) {
	r := authenticatedJSONRequest(t, requester, http.MethodPost,
		"/api/v1/requests/req-1/submit", `{"expectedVersion":1}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusConflict { t.Fatalf("status = %d", rr.Code) }
	assertErrorCode(t, rr, "version_conflict")
	assertAuditCount(t, "req-1", 1)
}
```

- [ ] **Step 2: Run request handler tests to verify failure**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Create|Get|Update|Submit|Pending|Approve|Audit)' -count=1`

Expected: FAIL because request routes and DTO mapping do not exist.

- [ ] **Step 3: Implement decoders, DTO mapping, and handlers**

Use `json.Decoder` with `DisallowUnknownFields` and reject trailing data. Decode `expectedVersion` from every unsafe Request operation. Use `r.PathValue("requestId")`; do not concatenate unvalidated values into SQL. Map typed application errors centrally: invalid input to 400, missing session to 401, forbidden mutation to 403, invisible/not-found read to 404, and version/state/routing conflicts to 409. Return the current Request and Approval DTO after a successful mutation, and sorted audit events for Audit History.

- [ ] **Step 4: Run all HTTP API tests**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -count=1` (with an isolated `TEST_DATABASE_URL` exported)

Expected: PASS.

- [ ] **Step 5: Commit the task files**

```bash
git add internal/httpapi/request_handlers.go internal/httpapi/request_handlers_test.go internal/httpapi/response_dto.go internal/httpapi/response_dto_test.go internal/httpapi/router.go
git commit -m "feat: expose request approval API"
```

### Task 8: Verify the published contract, application startup, and developer documentation

**Files:**
- Modify: `api/openapi.yaml`
- Modify: `docs/development/toolchain.md`
- Create: `docs/development/local-api.md`
- Create: `scripts/verify-openapi.mjs`
- Create: `scripts/verify-openapi.test.mjs`
- Modify: `package.json`
- Modify: `pnpm-lock.yaml`

**Interfaces:**
- Consumes: `api/openapi.yaml` and the existing pinned Node/pnpm toolchain.
- Produces: `pnpm run verify:openapi`, which parses OpenAPI YAML and checks local component references before CI or implementation tests run.
- Produces: reproducible instructions for PostgreSQL migration, Keycloak startup, required runtime secret inputs, and API test commands.

- [ ] **Step 1: Write a failing contract-validation test**

Add a Node test that loads the contract, asserts `openapi` starts with `3.1.`, checks all `$ref` values resolve under `components`, and checks that the accepted operations/security requirements remain present.

```js
test("all local OpenAPI references resolve", () => {
  const document = loadOpenAPI("api/openapi.yaml");
  for (const reference of references(document)) {
    assert.ok(resolveLocalReference(document, reference), reference);
  }
});
```

- [ ] **Step 2: Run the test to verify failure**

Run: `pnpm run verify:openapi`

Expected: FAIL because the parser and script do not exist.

- [ ] **Step 3: Add the smallest direct development dependency and verification script**

Add `yaml` v2.9.1 as an exact direct `devDependency`, update `pnpm-lock.yaml` through the ADR-007 review process, and use only it plus Node standard modules in `scripts/verify-openapi.mjs`. Record in `docs/development/toolchain.md` that this is a contract-validation-only development dependency, not an application runtime dependency. Add `verify:openapi` to `package.json`.

- [ ] **Step 4: Document reproducible local API operation**

In `docs/development/local-api.md`, document the exact order: provision PostgreSQL, apply migrations explicitly, start loopback-only Keycloak development mode with externally supplied test credentials, set required OIDC/session encryption configuration, launch `cmd/api`, run Go unit/integration tests, and run `pnpm run verify:openapi`. Do not place credentials or encryption keys in the repository.

- [ ] **Step 5: Run the complete API verification suite**

Run:

```bash
pnpm install --frozen-lockfile
pnpm run verify:openapi
GOTOOLCHAIN=go1.27.1 go test ./internal/domain ./internal/application/requests ./internal/auth ./internal/httpapi
GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres ./internal/httpapi -count=1
GOTOOLCHAIN=go1.27.1 go mod verify
git diff --check
```

Expected: PASS. The integration commands require an isolated PostgreSQL database URL; no test may run against a production database.

- [ ] **Step 6: Commit the task files**

```bash
git add api/openapi.yaml docs/development/toolchain.md docs/development/local-api.md scripts package.json pnpm-lock.yaml
git commit -m "test: verify API contract and local operation"
```

## Plan Self-Review

- **Spec coverage:** Tasks 1 and 5 cover OIDC/PKCE/session/CSRF; Tasks 2 and 4 cover PostgreSQL, migration, audit, and concurrency; Task 3 covers PDR workflow rules; Tasks 6 and 7 cover every OpenAPI operation and error class; Task 8 covers contract validation and reproducible operation.
- **Placeholder scan:** No task contains unfinished-work markers; all named files, interfaces, commands, and expected outcomes are explicit.
- **Type consistency:** `application.Actor` is the authenticated identity passed from `internal/auth` through HTTP middleware to request services; `expectedVersion` is `int64` throughout; OpenAPI operation names map directly to named handler/service methods.
- **Review focus:** Task 3 tests normalized Title and audit content; Task 4 tests race-safe mutations; Task 5 tests callback replay; Task 6 tests CSRF/origin; Task 7 tests authorization/error mapping.
