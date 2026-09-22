# Task 6 HTTP Authentication and Session Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Expose the accepted OIDC login, organization-selection, current-session, and logout behavior through the OpenAPI-governed Go HTTP API, while enforcing server-side session, Origin, and CSRF checks.

**Architecture:** `internal/httpapi` owns `net/http` routing, HTTP-only cookie transport, request-context actor propagation, JSON DTOs, and OpenAPI error responses. `internal/auth` owns opaque value generation and session/selection use cases; its PostgreSQL adapter resolves roles from the application database and atomically rotates CSRF-token hashes. Handlers never derive an Actor from a request header, query string, OIDC claim, or selection input.

**Tech Stack:** Go 1.27.1; `net/http`, `httptest`, `encoding/json`, `crypto/subtle`, and `crypto/rand` from the standard library; existing `database/sql` PostgreSQL adapter; existing `github.com/coreos/go-oidc/v3` v3.21.0 and `golang.org/x/oauth2` v0.37.0. No production dependency is added.

**Spec:** `docs/superpowers/specs/2026-09-20-api-contract-design.md`, `docs/superpowers/specs/2026-09-21-multi-organization-oidc-identity-design.md`, and `docs/superpowers/plans/2026-09-20-go-api-implementation.md` Task 6.

## Current-state findings

- Task 5 is complete on `develop` in `5d20868` (PR #7); `dadcbd0` records that completion. The working tree was clean when this plan was written.
- `api/openapi.yaml` currently defines login, callback, current-session, and logout, but does not define the Organization-selection operations required by Accepted ADR-013. This plan corrects that contract before registering those routes.
- `app_sessions` and `organization_selection_transactions` store only CSRF-token hashes. Consequently, an authenticated `GET` must generate a replacement raw token, store only its SHA-256 hash, and return the raw value once; it cannot recover a prior raw token from the database.
- `cmd/api` and `internal/httpapi` do not exist. `internal/store/postgres.AuthenticateSession` currently returns a Member ID but not roles, so the Task 6 store boundary must resolve application roles server-side before constructing `requests.Actor`.

## Global Constraints

- Use only the HTTP operations and schemas in `api/openapi.yaml`; add the two Organization-selection operations to that OpenAPI source before implementing them, because Accepted ADR-013 requires the flow and Accepted ADR-003 makes OpenAPI the contract source of truth.
- Use Go 1.27.1, `net/http` method-aware `ServeMux` patterns, and `Request.PathValue` where a path parameter is needed, as required by ADR-002. Do not add a router, session, JWT, ORM, query-builder, or test-container dependency.
- A browser cookie stores only a CSPRNG-generated opaque value. PostgreSQL stores SHA-256 hashes of session, transaction, and CSRF cookie/token values; raw token values, PKCE verifier, OIDC token, identity subject, and cookie values must not enter JSON responses except the explicitly returned CSRF token, URLs, or logs.
- A session always represents exactly one selected Member. Roles are read from `member_roles` in the application database; OIDC roles/groups, a caller-provided actor, and an unselected identity are never authorization inputs.
- For every unsafe `/api/v1` operation, require the exact configured `Origin` and a valid `X-CSRF-Token` bound to that session. Cookie `SameSite=Lax` is not a substitute. The Organization-selection POST follows the same Origin and synchronizer-token validation.
- Cookie policy remains ADR-011 compliant: production uses `__Host-approval_flow_session`, `Secure`, `HttpOnly`, `SameSite=Lax`, `Path=/`, and no Domain; loopback development alone uses the existing distinct non-Secure name.
- Callback, selection, and logout redirects use fixed local UI paths only. Do not accept a return URL from request query parameters or body.
- Preserve Task 5 workflow behavior and migrations `000001` through `000004`; this task adds no schema migration and does not change `internal/application/requests.Repository`.
- Use an isolated `TEST_DATABASE_URL` for PostgreSQL tests. In this environment prefix Go checks with `GOCACHE=/private/tmp/learn-ai-go-cache` because the default Go build cache is not writable.

## Contract to add before implementation

Add these operations and schemas to `api/openapi.yaml`:

- `GET /auth/oidc/organization-selection`: requires an HttpOnly selection cookie and returns `200 OrganizationSelection`, containing `candidates` and a newly issued `csrfToken`; returns `400 invalid_auth_transaction` for missing, expired, or consumed selection state.
- `POST /auth/oidc/organization-selection`: accepts `{ "memberId": "..." }`, requires the selection cookie and `X-CSRF-Token`, sets an application-session cookie and redirects with `302` after one valid selection; returns `400 invalid_auth_transaction`, `403 forbidden` for a candidate outside the snapshot, and `403 csrf_validation_failed` for an invalid Origin or selection CSRF token.
- `OrganizationSelectionCandidate` exposes only `memberId`, `organizationId`, and `organizationName`. The schema deliberately does not add an unmodeled person display name, identity subject, OIDC claim, or role.
- `OrganizationSelection` contains `candidates` and `csrfToken`; `SelectOrganizationInput` contains one non-empty `memberId`. Add a distinct `organizationSelectionCookie` security scheme while retaining `sessionCookie` for `/api/v1/session` operations.

## Review Focus

- A valid selection cookie with a candidate Member outside its server-side snapshot must return `403 forbidden`, consume the selection transaction, and create no application session (Task 2).
- A second callback or selection POST after any first attempt must return `400 invalid_auth_transaction` and must not set a session cookie (Task 2).
- A session/selection CSRF token returned by a prior GET must become unusable after a subsequent successful token issue; only its hash is persisted (Task 1 and Task 3).
- A valid session cookie without the configured Origin, with a malformed Origin, or with a missing/mismatched `X-CSRF-Token` must return `403 csrf_validation_failed` before logout or other unsafe API work runs (Task 3).
- Session authentication must get roles only from `member_roles`; adding an OIDC claim, a request header, or a JSON `roles` field cannot grant an application role (Task 1 and Task 3).

---

## File Structure

- `api/openapi.yaml` — source-of-truth definitions for Organization selection and its security/error behavior.
- `internal/auth/session.go` — repository-independent session principal, selection candidate, opaque-value generation, and narrow auth-store interfaces.
- `internal/auth/session_test.go` — unit tests for raw token issue/rotation orchestration and cookie policy without a database.
- `internal/store/postgres/sessions.go` — SQL-backed principal lookup, role resolution, CSRF hash rotation, selection-candidate retrieval, and atomic selection completion.
- `internal/store/postgres/sessions_test.go` — PostgreSQL integration coverage for token hashes, role lookup, selection snapshot, expiry, consumption, and no-session-on-failure behavior.
- `internal/httpapi/errors.go` — private typed HTTP errors and the sole OpenAPI `ErrorResponse` writer.
- `internal/httpapi/router.go` — dependency container, `ServeMux` registration, context helpers, session middleware, and Origin/CSRF middleware.
- `internal/httpapi/auth_handlers.go` — OIDC login, callback, and Organization-selection handlers.
- `internal/httpapi/session_handlers.go` — current-session and logout handlers.
- `internal/httpapi/auth_handlers_test.go` — handler contract tests for redirects, cookies, selection, and secret-free responses.
- `internal/httpapi/session_handlers_test.go` — handler contract tests for current session, authentication, Origin/CSRF rejection, and logout.
- `cmd/api/main.go` — composition root: validated configuration, PostgreSQL connection, authenticator, request service, HTTP handler, and graceful server lifecycle.

### Task 1: Complete the auth-store boundary needed by HTTP handlers

**Files:**
- Modify: `internal/auth/session.go`
- Modify: `internal/auth/session_test.go`
- Modify: `internal/store/postgres/sessions.go`
- Modify: `internal/store/postgres/sessions_test.go`

**Interfaces:**
- Consumes: existing `app_sessions`, `member_roles`, `members`, and Organization-selection tables from migrations `000001`–`000004`.
- Produces: `auth.Principal{MemberID string, Roles []domain.Role}`, `auth.AuthenticatedSession{ID string, Principal Principal, CSRFTokenHash []byte}`, `auth.OrganizationSelection{Candidates []OrganizationSelectionCandidate, CSRFToken string}`, `auth.SessionStore`, and `auth.SelectionStore` methods used by `internal/httpapi`.
- Exact methods:

```go
type SessionStore interface {
	Authenticate(context.Context, string, time.Time) (AuthenticatedSession, error)
	IssueCSRFToken(context.Context, string, time.Time) (string, error)
	Revoke(context.Context, string, time.Time) error
}

type SelectionStore interface {
	ReadAndIssueCSRFToken(context.Context, string, time.Time) (OrganizationSelection, error)
	Complete(context.Context, CompleteOrganizationSelectionInput) (SessionInput, error)
}

type CompleteOrganizationSelectionInput struct {
	Cookie, CSRFToken, MemberID string
	Now                         time.Time
}
```

- [ ] **Step 1: Write failing unit and PostgreSQL integration tests**

Add a unit test proving raw CSRF tokens are freshly generated and not retained in a value returned from an earlier call. Add PostgreSQL tests that seed a session with requester and approver roles, authenticate it as a `Principal`, verify no roles come from caller input, and assert that only the SHA-256 hash of an issued CSRF token is stored. Seed two Organizations for one identity and verify `ReadAndIssueCSRFToken` returns the snapshot `{memberId, organizationId, organizationName}` candidates and a raw token, while the database retains only its hash.

```go
func TestIssueCSRFTokenReplacesStoredHash(t *testing.T) {
	first, err := store.IssueCSRFToken(ctx, "session-1", now)
	if err != nil { t.Fatal(err) }
	second, err := store.IssueCSRFToken(ctx, "session-1", now)
	if err != nil { t.Fatal(err) }
	if first == second { t.Fatal("CSRF token was reused") }
	assertSessionCSRFHash(t, db, "session-1", sha256.Sum256([]byte(second)))
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/auth -run 'Test(IssueCSRF|SessionCookie)' -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run 'Test(SessionPrincipal|IssueCSRF|OrganizationSelection)' -count=1
```

Expected: FAIL because `Principal`, token-issue methods, and selection-candidate retrieval do not yet exist.

- [ ] **Step 3: Implement the minimal boundary and SQL**

Generate every newly issued CSRF token with the existing CSPRNG helper, hash it with SHA-256, and atomically replace the matching active record's `csrf_token_hash`. `Authenticate` must look up an unrevoked, unexpired session by cookie hash once, then query only `member_roles` for that selected Member and return a private session ID plus the stored hash—never a raw cookie or token. `ReadAndIssueCSRFToken` must reject missing, expired, or consumed transactions; query candidates through `organization_selection_transaction_members → members → organizations`; rotate the selection CSRF hash; and return only the three contract fields plus the new raw token.

`Complete` must generate the application-session cookie, CSRF token, and ID inside `internal/auth`, then call one PostgreSQL transaction that consumes the selection record before checking token hash and candidate membership. Any recognized invalid selection attempt is consumed; no failed path inserts `app_sessions`.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run:

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/auth -run 'Test(IssueCSRF|SessionCookie)' -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run 'Test(SessionPrincipal|IssueCSRF|OrganizationSelection)' -count=1
```

Expected: PASS; the integration tests require an isolated `TEST_DATABASE_URL`.

- [ ] **Step 5: Commit this task**

```bash
git add internal/auth/session.go internal/auth/session_test.go internal/store/postgres/sessions.go internal/store/postgres/sessions_test.go
git commit -m "feat: prepare server-side session boundary for HTTP"
```

### Task 2: Add the Organization-selection API contract and OIDC handlers

**Files:**
- Modify: `api/openapi.yaml`
- Create: `internal/httpapi/errors.go`
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/auth_handlers.go`
- Create: `internal/httpapi/auth_handlers_test.go`

**Interfaces:**
- Consumes: `auth.Authenticator.BeginLogin(context.Context) (auth.LoginStart, error)`, `auth.Authenticator.CompleteLogin(context.Context, auth.CallbackInput) (auth.LoginResult, error)`, Task 1 `auth.SelectionStore`, and `config.Config`.
- Produces: `NewRouter(Dependencies) http.Handler`; `GET /auth/oidc/login`, `GET /auth/oidc/callback`, `GET /auth/oidc/organization-selection`, `POST /auth/oidc/organization-selection`; and `WriteError(http.ResponseWriter, APIError)`.

- [ ] **Step 1: Write failing OpenAPI and handler contract tests**

Add OpenAPI assertions for the two new operations, their cookie security schemes, schemas, and the exact `400`/`403` error codes. With fake authenticator and store implementations, test login's authorization redirect and short-lived transaction cookie; a single-member callback's session cookie and fixed UI redirect; a multi-member callback's selection cookie and selection redirect without session cookie; selection GET's candidates plus CSRF token; and selection POST's `302` and application-session cookie.

```go
func TestOrganizationSelectionRejectsCandidateOutsideSnapshot(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/auth/oidc/organization-selection", strings.NewReader(`{"memberId":"member-outside"}`))
	r.AddCookie(selectionCookie(t))
	r.Header.Set("Origin", allowedOrigin)
	r.Header.Set("X-CSRF-Token", selectionCSRF(t))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden { t.Fatalf("status = %d", rr.Code) }
	assertErrorCode(t, rr, "forbidden")
}
```

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Login|Callback|OrganizationSelection|WriteError)' -count=1
```

Expected: FAIL because the HTTP package, OpenAPI operations, and handlers do not exist.

- [ ] **Step 3: Implement contract, error mapping, routes, and handlers**

Extend `api/openapi.yaml` exactly as specified in **Contract to add before implementation**. Register method-aware `ServeMux` routes. Map `auth.ErrNotFound`, `auth.ErrExpired`, and `auth.ErrConsumed` to `400 invalid_auth_transaction`; map `auth.ErrForbidden` to `403 forbidden`; map malformed JSON to `400 invalid_request`; and map unexpected errors to `500 internal_error` without exposing wrapped details.

Set the transaction/selection cookie as `HttpOnly`, `SameSite=Lax`, `Path=/`, and `Secure` when `cfg.CookieSecure` is true. On callback, always clear the transaction cookie after a recognized callback attempt. On selection completion, clear the selection cookie and set a session cookie only after `SelectionStore.Complete` succeeds. Both selection GET and POST use a fixed local UI location; neither reflects a user-supplied redirect destination. Encode JSON with `Content-Type: application/json` and never serialize `LoginResult`, raw OIDC response values, verifier, identity subject, or cookies.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run:

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Login|Callback|OrganizationSelection|WriteError)' -count=1
```

Expected: PASS. Assertions prove invalid/replayed selection cannot issue a session cookie and response/log fixtures contain none of the secret inputs.

- [ ] **Step 5: Commit this task**

```bash
git add api/openapi.yaml internal/httpapi/errors.go internal/httpapi/router.go internal/httpapi/auth_handlers.go internal/httpapi/auth_handlers_test.go
git commit -m "feat: expose OIDC login and organization selection"
```

### Task 3: Add authenticated-session middleware and session endpoints

**Files:**
- Modify: `internal/httpapi/router.go`
- Create: `internal/httpapi/session_handlers.go`
- Create: `internal/httpapi/session_handlers_test.go`

**Interfaces:**
- Consumes: Task 1 `auth.SessionStore`, `requests.Actor`, `config.Config.AllowedOrigin`, `auth.SessionCookie`, and Task 2 `WriteError`.
- Produces: `GET /api/v1/session`, `POST /api/v1/session/logout`, `ActorFromContext(context.Context) (requests.Actor, bool)`, `RequireSession(http.Handler) http.Handler`, and `RequireCSRF(http.Handler) http.Handler`.

- [ ] **Step 1: Write failing middleware and session-handler tests**

Use store fakes to test that a missing/expired/revoked cookie returns `401 authentication_required`; `GET /api/v1/session` returns exactly the selected Member ID, database roles, and a freshly issued CSRF token; and logout requires a valid session, exact Origin, and current token. Include missing Origin, different scheme/host/port, multiple Origin values, missing token, stale token after rotation, and mismatched token cases. Confirm the revoke operation is not called on any CSRF rejection and a valid logout returns `204` plus an expired session cookie using the configured cookie name.

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

- [ ] **Step 2: Run the focused tests to verify they fail**

Run:

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Session|Logout|RequireSession|RequireCSRF)' -count=1
```

Expected: FAIL because authenticated middleware and session handlers do not exist.

- [ ] **Step 3: Implement session context, Origin/CSRF protection, and handlers**

`RequireSession` selects the configured session-cookie name, calls `SessionStore.Authenticate` once, and puts `requests.Actor{MemberID, Roles}` plus a package-private authenticated-session value in the request context. Only `ActorFromContext` is exported to application-facing handlers; it never trusts actor information from HTTP input. `RequireCSRF` first requires the exact single `Origin` equal to `cfg.AllowedOrigin`, then compares `sha256.Sum256` of the raw `X-CSRF-Token` with the context-private stored hash using `crypto/subtle.ConstantTimeCompare`; it returns `403 csrf_validation_failed` before invoking its wrapped handler.

The current-session handler calls `IssueCSRFToken` and returns the OpenAPI `Session` JSON DTO. Logout revokes only the authenticated current cookie, writes `204 No Content`, and sends the matching session cookie with `MaxAge: -1` and an expiry in the past. Register the two `/api/v1/session` routes with method-aware patterns and wrap only the unsafe logout route with Origin/CSRF protection.

- [ ] **Step 4: Run the focused tests to verify they pass**

Run:

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Session|Logout|RequireSession|RequireCSRF)' -count=1
```

Expected: PASS; tests prove that only a server-derived Actor reaches the context and that CSRF rejection prevents revocation.

- [ ] **Step 5: Commit this task**

```bash
git add internal/httpapi/router.go internal/httpapi/session_handlers.go internal/httpapi/session_handlers_test.go
git commit -m "feat: protect and expose application sessions"
```

### Task 4: Compose the runnable API and perform complete verification

**Files:**
- Create: `cmd/api/main.go`
- Modify: `docs/development/handoff-2026-09-21-task5.md`

**Interfaces:**
- Consumes: `config.Load(os.Getenv)`, `postgres.NewRepository(*sql.DB)`, `auth.NewOIDCAuthenticator`, `requests.NewService`, and `httpapi.NewRouter`.
- Produces: an API process listening on `config.Config.ListenAddress` and an updated handoff stating that Task 6 is implemented only after the verification below passes.

- [ ] **Step 1: Write a failing composition test or startup seam**

Extract a `newHandler(ctx context.Context, cfg config.Config, db *sql.DB) (http.Handler, error)` function in `cmd/api/main.go` and add a focused test that supplies a test database and a fake OIDC-construction seam. It must assert configuration/database/OIDC initialization errors are returned rather than starting a partially configured server.

```go
func TestNewHandlerReturnsOIDCInitializationError(t *testing.T) {
	_, err := newHandler(context.Background(), validConfig(t), testDB(t))
	if err == nil { t.Fatal("expected OIDC initialization error") }
}
```

- [ ] **Step 2: Run the focused test to verify it fails**

Run:

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./cmd/api -run TestNewHandler -count=1
```

Expected: FAIL because `cmd/api` and `newHandler` do not exist.

- [ ] **Step 3: Implement the composition root and synchronize the handoff**

Use `database/sql` with the already-selected pgx stdlib driver, call `config.Load(os.Getenv)`, construct one PostgreSQL repository, one request service, one long-lived OIDC authenticator, and one HTTP router. Start `http.Server` at the validated configured address; on `SIGINT`/`SIGTERM`, use a bounded shutdown context and close the database. Do not log configuration secrets, request cookies, CSRF tokens, authorization codes, or ID tokens.

Update the Task 5 handoff only to replace the statement that HTTP cookie issuance/CSRF middleware is pending with the Task 6 route and validation evidence. Retain the identity-provisioning prerequisite and do not add Keycloak credentials to the repository.

- [ ] **Step 4: Run focused and complete verification**

Run:

```bash
source /Users/nao/.nvm/nvm.sh
nvm use 26.9.0
pnpm install --frozen-lockfile
pnpm run test:db
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./cmd/api ./internal/auth ./internal/httpapi -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...
pnpm run check:gofmt
pnpm run format:check
pnpm run lint
git diff --check
```

Expected: PASS. `pnpm run test:db` and the PostgreSQL package test use an isolated `TEST_DATABASE_URL`; none may target a production database. If the known Node test-runner collection issue recurs, record its exact command/output and resolve it without weakening the suite.

- [ ] **Step 5: Commit this task**

```bash
git add cmd/api/main.go docs/development/handoff-2026-09-21-task5.md
git commit -m "feat: run the authenticated approval API"
```

## Plan Self-Review

- **Spec coverage:** Task 1 supplies the missing server-side role and one-way CSRF-token boundaries. Task 2 synchronizes the missing ADR-013 Organization-selection contract and implements the OIDC browser handoff. Task 3 covers the OpenAPI current-session/logout operations, 401/403 semantics, and unsafe-method CSRF/Origin defense. Task 4 provides the configured executable and all project quality checks.
- **Contract consistency:** Organization-selection is added to OpenAPI before route implementation. `memberId` is treated as untrusted input and checked against a persisted candidate snapshot; organization name is display data only. Existing session DTOs retain their documented `actor` and `csrfToken` fields.
- **Security coverage:** Tests cover callback/selection replay, candidate escape, transaction/session expiry, secret-free output, session-derived roles, raw-token rotation, absent/mismatched Origin, stale/missing CSRF token, and cookie deletion.
- **Dependency and Decision check:** The plan adds no dependency and relies on Accepted ADR-002, ADR-003, ADR-005, ADR-011, ADR-012, ADR-013, and the existing product requirements. No new ADR/PDR is needed because the contract correction implements ADR-013's already accepted selection endpoint rather than choosing a new architecture or product behavior.
- **Placeholder scan:** No deferred implementation marker or unspecified test/verification command was found.
