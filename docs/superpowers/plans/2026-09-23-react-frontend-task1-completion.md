# React Frontend Task 1 Completion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to implement this plan task by task. Track progress with `- [ ]` checkboxes. The repository's `AGENTS.md` requires an implementer and a reviewer for implementation work.

**Goal:** Complete and verify the single-origin CORS and fixed frontend redirect boundary required by Task 1 of the React frontend plan.

**Architecture:** `APP_FRONTEND_ORIGIN` is the one validated source for both the CORS allowlist and the two fixed post-login UI destinations. The Go HTTP router wraps its mux in a standard-library CORS handler; session and CSRF checks remain in their existing handlers.

**Tech Stack:** Go 1.27.1 `net/http`; OpenAPI 3.1.1; no new dependency.

**Spec:** `docs/superpowers/plans/2026-09-23-react-frontend-implementation.md` (Task 1), `docs/decisions/architecture/ADR-015-frontend-delivery-topology.md`, `api/openapi.yaml`.

**Status:** Completed by [PR #21](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/21), merged into `develop` on 2026-09-23. Verification and review evidence: [`docs/development/react-frontend-task1-completion-2026-09-23.md`](../../development/react-frontend-task1-completion-2026-09-23.md).

## Global Constraints

- Accepted ADR-015 selects distinct frontend and API origins within the same site for the first slice. Cross-site cookies and multiple allowed frontend origins are outside this task.
- Only the exact configured `APP_FRONTEND_ORIGIN` receives `Access-Control-Allow-Origin` and `Access-Control-Allow-Credentials: true`.
- Preflight permits `GET`, `POST`, and `PATCH`, plus request headers `Content-Type` and `X-CSRF-Token`. It returns `204` without entering the mux, authentication, session, or mutation handlers.
- `APP_FRONTEND_ORIGIN` must be an absolute HTTP(S) origin with no path, query, fragment, userinfo, or surrounding whitespace. `APP_ALLOWED_ORIGIN` is not an alias.
- Callback and successful organization selection redirect only to `APP_FRONTEND_ORIGIN + "/"` or `APP_FRONTEND_ORIGIN + "/organization-selection"`; client-supplied return URLs do not affect `Location`.
- Keep ADR-011 session cookie and CSRF rules, and use the Go standard library without a new dependency.

## Current Baseline (2026-09-23)

The working tree is clean, and the focused packages pass with `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/config ./internal/httpapi ./cmd/api -count=1`. `FrontendOrigin` loading and validation, `NewCORS`, router wrapping, absolute redirects, and their main tests already exist. Execute this as a completion plan: retain the passing behavior and change only the gaps below. The earlier Task 1 plan's expected initial failure is no longer applicable.

## Review Focus

- Duplicate or malformed `Access-Control-Request-Method` and `Access-Control-Request-Headers` values must not let a preflight pass.
- A rejected Origin or preflight must never call the wrapped handler or emit credentialed CORS response headers.
- Allowed-origin `401 authentication_required` and `403 csrf_validation_failed` responses must remain readable to the browser through exact CORS headers.
- A preflight to an otherwise valid API route must not read a session, rotate a CSRF token, or perform a mutation.
- Both callback outcomes and organization selection success must use a fixed absolute frontend `Location` even when attacker-controlled query parameters are supplied.

---

### Task 1.1: Finish the CORS transport boundary

**Files:**
- Create: `internal/httpapi/cors.go` (move existing CORS-only functions from `router.go`)
- Modify: `internal/httpapi/router.go`
- Modify: `internal/httpapi/cors_test.go`

**Interfaces:** Preserve `NewCORS(allowedOrigin string, next http.Handler) http.Handler` and `NewRouter(dependencies Dependencies) http.Handler`. Keep `validOrigin` and `singleHeader` available to the existing CSRF handlers; their location may remain in `router.go`.

- [x] **Step 1: Add failing boundary tests.** Add table cases to `cors_test.go` for duplicated `Origin`, duplicated request-method values, duplicated request-header values, an unrecognized method, and an unrecognized header. For every rejected case, assert status `403`, no `Access-Control-Allow-*` headers, and `called == false` on a counting wrapped handler. Add tests that call `NewRouter` with the allowed Origin and assert exact CORS headers on `GET /api/v1/session` returning `401` and `POST /api/v1/requests` returning `403` for missing/invalid CSRF. Preserve the existing Vary-value test.

  Example test construction for a duplicated preflight header:

  ```go
  r := httptest.NewRequest(http.MethodOptions, "/api/v1/requests", nil)
  r.Header.Set("Origin", allowedOrigin)
  r.Header.Add("Access-Control-Request-Method", http.MethodPost)
  r.Header.Add("Access-Control-Request-Method", http.MethodDelete)
  rr := httptest.NewRecorder()
  NewCORS(allowedOrigin, countingHandler).ServeHTTP(rr, r)
  // Assert 403, no CORS credential headers, countingHandler call count 0.
  ```

- [x] **Step 2: Run the focused red test.** Run `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'TestRouterCORS' -count=1`. The duplicated preflight values should fail against the current `Header.Get` implementation; if another asserted case unexpectedly passes, record the actual output before editing production code.

- [x] **Step 3: Make the smallest implementation change.** Move `allowedCORSMethods`, `allowedCORSHeaders`, `NewCORS`, `appendVaryToken`, and `allowedCORSPreflight` to `cors.go`. In `allowedCORSPreflight`, require exactly one nonempty `Access-Control-Request-Method` value and at most one `Access-Control-Request-Headers` value before parsing the existing allowlist. Do not broaden the accepted methods or headers. Keep the CORS wrapper outside the mux.

- [x] **Step 4: Run the focused green test.** Re-run the Step 2 command and expect all `TestRouterCORS*` cases to pass. Inspect the resulting diff to ensure only the CORS functions moved and the single-value checks changed behavior.

### Task 1.2: Synchronize the redirect contract and verify the boundary

**Files:**
- Modify: `api/openapi.yaml`
- Modify only if an assertion fails: `internal/httpapi/auth_handlers.go`, `internal/httpapi/auth_handlers_test.go`, `internal/config/config.go`, `internal/config/config_test.go`, `cmd/api/main_test.go`
- Modify: `docs/development/toolchain.md` only if a command or version recorded there is wrong; otherwise leave it unchanged.

**Interfaces:** Preserve `config.Config.FrontendOrigin string` and the existing fixed-path `(*router).frontendLocation(path string) string`. No client-supplied redirect destination is added.

- [x] **Step 1: Add or confirm redirect and configuration assertions.** Confirm tests cover rejection of path/query/fragment/userinfo/whitespace, absence of the old alias, loopback-only insecure development, callback session redirect to `https://app.example/`, callback selection redirect to `https://app.example/organization-selection`, and successful selection redirect to `https://app.example/` with `returnUrl` in the request. Add only a missing assertion, with an exact expected `Location`.

- [x] **Step 2: Correct the OpenAPI callback description.** In `/auth/oidc/callback` change the stale phrase “redirects to `/`” to “redirects to the configured frontend origin plus `/`”. Keep the `302` response and organization-selection path consistent with the existing implementation and ADR-015. Do not add a return-URL parameter or change the accepted Decision text.

- [x] **Step 3: Validate the contract and Go code.** Run the commands below from the repository root. `GOCACHE` uses a writable temporary directory because the default macOS Go cache is not writable in this workspace sandbox.

  ```sh
  pnpm run verify:openapi
  node --test scripts/verify-openapi.test.mjs
  pnpm run check:gofmt
  GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/config ./internal/httpapi ./cmd/api -count=1
  GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./... -count=1
  GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...
  git diff --check
  ```

- [x] **Step 4: Review and report.** Have the required reviewer inspect the final diff and the Review Focus cases; resolve valid findings and have the reviewer recheck. Report changed files, FR-012/NFR-001/NFR-004/NFR-006, ADR-011/ADR-015, each verification result, and the remaining Task 4 browser E2E requirement. Commit this completion change on a writable task branch or worktree, not the protected `develop` branch.

## Self-Review

- **Coverage:** The plan tests the current CORS and redirect implementation and corrects the one identified OpenAPI wording mismatch. The React client and browser E2E remain with Tasks 2–4 of the parent plan.
- **Decision gate:** ADR-015 already accepts this transport and redirect approach. This plan introduces no new important product or architecture decision and no dependency.
- **Type consistency:** `FrontendOrigin`, `NewCORS`, `NewRouter`, and `frontendLocation` retain their existing signatures.
