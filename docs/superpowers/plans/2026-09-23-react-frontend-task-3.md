# React Frontend Task 3 Implementation Plan

**Status: Completed (2026-09-24).** 実装・検証・タスク別レビュー・全体レビューの結果は [完了記録](../../development/react-frontend-task3-completion-2026-09-24.md) を参照。

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` or `superpowers:executing-plans` to execute this plan task by task. Track progress with `- [ ]`. The repository's `AGENTS.md` §7.1 requires an implementer for code and tests, followed by a reviewer; resolve valid findings and request a second review before completion.

**Goal:** Deliver the first browser UI slice: organization choice, Draft creation and editing, Submit, assigned Pending approval, and Request/Audit viewing, with explicit recovery from expired sessions, stale CSRF tokens, and version conflicts.

**Architecture:** Keep `App` as the owner of the current `Session`, selected opaque Request ID, and top-level notice. Render `/organization-selection` or `/` from `location.pathname` without a routing library. Presentational components receive the Task 2 `ApiClient`; mutations use the current in-memory CSRF token and server Request version. A conflict refreshes Request and Audit once and leaves the next mutation to the user.

**Tech Stack:** React/React DOM 19.3.0, TypeScript 7.0.2, Vite 8.3.0, Vitest 5.0.1, Testing Library 16.3.3, jsdom 30.1.0, Biome 2.5.14; existing `pnpm` lockfile only.

**Spec:** `docs/superpowers/plans/2026-09-23-react-frontend-implementation.md` Task 3; `docs/superpowers/specs/2026-09-20-api-contract-design.md`; `docs/superpowers/specs/2026-09-21-multi-organization-oidc-identity-design.md`; `api/openapi.yaml`.

## Global Constraints

- This plan covers FR-003/004/005/007/011/012 and NFR-001/002/003/004. It follows Accepted ADR-001/003/005/006/007/011/013/015 and PDR-001/002. No new important Decision or dependency is needed for the scoped UI.
- The first slice uses distinct Frontend/API origins within one site. Preserve the Task 2 `ApiClient`, `credentials: "include"`, server-side authorization, and `X-CSRF-Token` on unsafe requests.
- Keep OIDC tokens, cookie values, CSRF tokens, Member IDs, and roles out of storage, URL, and logs. The only URL state added here is `?requestId=<opaque Request ID>`; render API text as React text, never HTML.
- Title trims to 1–120 characters; Description is optional and at most 2,000 characters. Only the Requester's own Draft is editable. Submit never asks the user to choose an Approver. Only the assigned Approver may approve Pending.
- `GET /api/v1/session` and `GET /auth/oidc/organization-selection` each rotate their respective CSRF token. Do not run either GET on every render. A recognized organization selection attempt consumes its transaction even on failure; failed selection goes back to Sign in, not automatic retry.
- Branch UI behavior on `ApiError.body.code`, not `message`. Never automatically replay a mutation after `csrf_validation_failed`, `version_conflict`, `invalid_state`, or uncertain network failure. Keep `requestId` when a Request read fails so the user can retry.
- Do not add Reject, Cancel, search, request listing, client router, state library, UI library, generated API client, or a new product rule. Task 4 owns browser E2E and the isolated OIDC stack.

## Current Baseline

Task 1 is merged and Task 2 is complete on `develop` as of 2026-09-23. `src/api/client.ts` already exposes `getSession`, `getOrganizationSelection`, `selectOrganization`, Request mutations and reads, `listPending`, `listAuditEvents`, and `logout`. `src/app.tsx` currently renders only loading, Sign in, Retry, and an empty authenticated workspace. The working tree was clean at planning time. Preserve the Task 2 session request generation guard against stale responses.

Task 2's `selectOrganization` treats a cross-origin `opaqueredirect` from its manual fetch as success. The UI must then navigate to the fixed Frontend `/`. Whether the browser retains the new cookie through that handoff remains a Task 4 real-browser E2E gate; do not claim it from jsdom tests.

## Review Focus

- A repeated or late session/Request read must not replace newer state or rotate the CSRF token behind a pending user action (Tasks 3.1, 3.2).
- An empty, `.` or `..` URL Request ID must never reach `getRequest`; a 404 or network failure must preserve a usable retry path (Tasks 3.1, 3.2).
- A failed or consumed organization selection must never issue another POST with the same selection token, and must offer a fresh Sign in (Task 3.1).
- Form field errors, blank Title, overlong fields, and empty Description must be handled without discarding user edits or changing the audited content (Task 3.2).
- A 409 refresh and a CSRF refresh must never send the original mutation again; a user must explicitly act after seeing the refreshed state (Task 3.3).

---

## File Map and Interfaces

| File | Responsibility |
| --- | --- |
| `src/app.tsx`, `src/app.test.tsx` | Path dispatch, session/token ownership, Request ID URL synchronization, logout and authentication recovery |
| `src/components/sign-in.tsx` | Fixed Sign in action and brief authentication prompt |
| `src/components/organization-selection.tsx`, `.test.tsx` | One candidate read, single-use selection POST, recovery to fresh login |
| `src/components/request-workspace.tsx`, `.test.tsx` | Request/detail/audit loading, mutations, conflict and CSRF recovery, pending refresh |
| `src/components/request-form.tsx` | Controlled Title/Description input and field errors for create or Draft update |
| `src/components/request-detail.tsx` | Status/content, Draft actions or assigned Pending Approve action |
| `src/components/pending-list.tsx` | Current Approver's assigned Pending items and selection by ID |
| `src/components/audit-history.tsx` | Ordered event list, actor/time/content snapshots including empty Description |
| `src/components/error-notice.tsx` | Accessible, code-driven recoverable status and action |
| `src/styles.css`, `src/main.tsx` | Minimal responsive layout and stylesheet import |

Use these component boundaries. `App` supplies `session`, `requestId`, `onRequestIdChange`, `onSessionChange`, `onAuthenticationRequired`, and `onNotice` to `RequestWorkspace`; the callbacks change in-memory state only except for `onRequestIdChange`, which updates `?requestId=` with `history.pushState`. `App` renders the workspace notice using `ErrorNotice`, while Organization selection can keep its own local failure notice. `RequestWorkspace` never reads browser storage. `OrganizationSelection` receives `client`, `login`, and `onSelected`; `onSelected` navigates to the fixed `/`. The implementation may use a discriminated union for local loading/error/ready state, but it must not add a global state abstraction.

### Task 3.1: Path dispatch, Organization choice, and session ownership

**Files:** Modify `src/app.tsx`, `src/app.test.tsx`; create `src/components/sign-in.tsx`, `src/components/organization-selection.tsx`, `src/components/organization-selection.test.tsx`, `src/components/error-notice.tsx`.

**Interfaces:**

```ts
type OrganizationSelectionProps = {
  client: ApiClient;
  login: () => void;
  onSelected: () => void; // fixed frontend "/" navigation
};
type WorkspaceProps = {
  client: ApiClient;
  session: Session;
  requestId: string | null;
  onRequestIdChange: (id: string | null) => void;
  onSessionChange: (session: Session) => void;
  onAuthenticationRequired: () => void;
  onNotice: (notice: { code: ErrorCode | "transport_failure"; text: string } | null) => void;
};
```

- [x] **Step 1: Write failing route/session/selection tests.** In `app.test.tsx`, set `history.replaceState` to `/organization-selection` and assert `getOrganizationSelection` is called while `getSession` is not. At `/`, assert `getSession` is called once, an authenticated Requester sees the workspace, and a 401 returns Sign in. Keep the existing stale-response/StrictMode tests green. In `organization-selection.test.tsx`, test candidate names, pending-button disablement, success callback, `invalid_auth_transaction`/`csrf_validation_failed` failure returning Sign in, and no second POST after failure. Example assertion:

  ```tsx
  const selectOrganization = vi.fn().mockResolvedValue(undefined);
  render(<OrganizationSelection client={clientWith({ getOrganizationSelection, selectOrganization })} login={login} onSelected={onSelected} />);
  await user.click(await screen.findByRole("button", { name: "North Office" }));
  expect(selectOrganization).toHaveBeenCalledWith("member-north", "current-selection-csrf");
  expect(onSelected).toHaveBeenCalledOnce();
  ```

- [x] **Step 2: Confirm RED.** Run `pnpm test -- src/app.test.tsx src/components/organization-selection.test.tsx`. Expect route/selection tests to fail because these components and branches do not exist; retain the exact failure output.
- [x] **Step 3: Implement route and session state.** Make `App` dispatch the two fixed paths and retain its existing authenticated shell until Task 3.2 mounts `RequestWorkspace`. Read `requestId` through `new URLSearchParams(window.location.search).get("requestId")`, reject empty/`.`/`..`, and listen for `popstate`; do not put session data in history. On `onRequestIdChange`, build a URL from `window.location.href`, set/delete only `requestId`, and call `history.pushState`. Reuse the existing `getSession` generation guard. Render `OrganizationSelection` on its path without session bootstrap. Its effect calls `getOrganizationSelection` once per mount, stores only the current candidate response in state, and guards against a late result after unmount. On selection success, invoke `onSelected` (from `App`, `window.location.assign("/")`); on a failed recognized attempt, discard its token and show Sign in. Keep candidate IDs out of text except as button values.
- [x] **Step 4: Confirm GREEN and review scope.** Run `pnpm test -- src/app.test.tsx src/components/organization-selection.test.tsx` and `pnpm run typecheck`. Inspect `git diff` for URL or storage exposure. The `getSession` rotation and selection rotation must remain separate.
- [x] **Step 5: Reviewer gate.** Have implementer complete the code/tests and reviewer inspect this task's diff. Fix valid findings and obtain reviewer recheck before the next task.

### Task 3.2: Draft form, Request detail, and Audit history

**Files:** Create `src/components/request-workspace.tsx`, `src/components/request-workspace.test.tsx`, `src/components/request-form.tsx`, `src/components/request-detail.tsx`, `src/components/audit-history.tsx`; modify `src/app.tsx` only to mount `RequestWorkspace` using the interface above.

**Interfaces:** `RequestWorkspace(props: WorkspaceProps): ReactElement` owns the selected Request and Audit load state. `RequestForm` takes `title`, `description`, `fieldErrors`, input callbacks, submit label and pending state; it never calls the API. `RequestDetail` takes the current `Request`, `Session.actor`, and explicit Update/Submit/Approve callbacks. `AuditHistory` takes `AuditEvent[]` and has no mutation callback.

- [x] **Step 1: Write failing component tests.** Use a fake `ApiClient` with typed Request/Audit fixtures. Assert `?requestId=a%2Fb` loads `a/b`, `?requestId=.` and `?requestId=..` never call `getRequest`, and `popstate` selects the prior ID. Assert a Requester can create a Draft with trimmed Title and empty Description, the returned ID calls `onRequestIdChange`, and the same ID loads detail plus audit. A Draft owned by the actor offers Update and Submit with its current `version`; Pending/Approved does not offer editing. An Approver assigned in `approval.assigneeMemberId` sees Approve on Pending; a different Approver does not. Assert blank/overlong Title and overlong Description are rejected, form `fieldErrors` are rendered by `field` rather than parsed from `message`, entered values survive a 400, and empty Description is visible as an intentional empty value in Audit. Example:

  ```tsx
  await user.type(screen.getByLabelText("Title"), "  VPN access  ");
  await user.click(screen.getByRole("button", { name: "Create Draft" }));
  expect(client.createRequest).toHaveBeenCalledWith(
    { title: "VPN access", description: "" }, session.csrfToken,
  );
  expect(onRequestIdChange).toHaveBeenCalledWith("request-1");
  ```

- [x] **Step 2: Confirm RED.** Run `pnpm test -- src/components/request-workspace.test.tsx`; expect missing components/actions to fail.
- [x] **Step 3: Implement form and read paths.** Render a create form for a Requester even when a Request is selected, so a submitted Request can be followed by a new Draft. Validate Title/Description against PDR-001 before calling the API and show server `fieldErrors` as the final authority. For a selected ID, load `getRequest(id)` and `listAuditEvents(id)` together; guard stale responses when ID changes and provide a Retry action without clearing the ID. Show plain text for title, description, and audit snapshots; show organization name on the selection screen where its DTO supplies it. Keep Audit order returned by the API, with actor ID and `occurredAt` displayed. Apply returned Request after successful create/update and refresh Audit once. Use `expectedVersion: request.version` on Update; do not provide an Approver picker on Submit.
- [x] **Step 4: Confirm GREEN.** Run `pnpm test -- src/components/request-workspace.test.tsx src/app.test.tsx`, `pnpm run typecheck`, `pnpm run format:check`, and `pnpm run lint`. Review conditions for button visibility against server authorization; hiding a button is only presentation, not a substitute for the API check.
- [x] **Step 5: Reviewer gate.** Have implementer complete the code/tests and reviewer inspect this task's diff. Fix valid findings and obtain reviewer recheck before the next task.

### Task 3.3: Pending queue, Approve, logout, and explicit recovery

**Files:** Create `src/components/pending-list.tsx`, `src/styles.css`; modify `src/components/request-workspace.tsx`, `src/components/request-workspace.test.tsx`, `src/app.tsx`, `src/app.test.tsx`, `src/main.tsx`, and only the presentational components needed to show notices.

**Interfaces:** `PendingList` takes `Request[]`, `onSelect(id: string)`, loading and retry inputs. `RequestWorkspace` calls `onSessionChange` only after an explicit `getSession` refresh and `onAuthenticationRequired` on `authentication_required`. `App` clears its session and Request data on logout/401 and renders Sign in.

- [x] **Step 1: Write failing recovery tests.** Assert Approver role alone loads `listPending`, its item selects the opaque ID, and an empty queue is visible. Assert Submit/Approve send the selected Request's current version and refresh Audit (and pending queue for Approver) on success. For `version_conflict` and `invalid_state`, assert `getRequest` and `listAuditEvents` each run once, notice requests a human decision, and mutation call count stays one. For `csrf_validation_failed`, assert one `getSession` refresh, no replay, and only the next explicit click uses the rotated token. For `authentication_required` during read/mutation/logout, assert session and token disappear from App state and Sign in appears. For unknown network result, assert no automatic retry and a read refresh path. Example:

  ```tsx
  await user.click(await screen.findByRole("button", { name: "Submit" }));
  expect(client.submitRequest).toHaveBeenCalledTimes(1);
  expect(client.getRequest).toHaveBeenCalledWith("request-1");
  expect(client.listAuditEvents).toHaveBeenCalledWith("request-1");
  expect(screen.getByRole("status")).toHaveTextContent(/changed|refresh/i);
  ```

- [x] **Step 2: Confirm RED.** Run `pnpm test -- src/components/request-workspace.test.tsx src/app.test.tsx`; expect the new Pending/recovery/logout assertions to fail.
- [x] **Step 3: Implement mutation and recovery paths.** Add a single in-flight mutation guard so double click cannot submit twice. On a successful mutation, replace the shown Request with the response and refresh Audit; refresh Pending for an Approver. On 409, read the current Request and Audit once, update displayed version/state, and require another click. On CSRF failure, call `getSession` once, pass the new `Session` to App, and require another click; on a failed refresh, show Retry or Sign in according to error code. On 401, invoke `onAuthenticationRequired` and clear Request state. `approval_routing_unavailable`, `forbidden`, `request_not_found`, `invalid_request`, and transport errors get an accessible notice with an appropriate read or retry action, but no hidden mutation replay. Logout uses the current token, waits for `204`, then clears session; errors follow the same 401/CSRF/transport rules. Import `src/styles.css` in `src/main.tsx`; style form labels, status notices, focus states and narrow screens without a new library.
- [x] **Step 4: Run the complete Task 3 checks.** Run `pnpm test`, `pnpm run typecheck`, `pnpm run format:check`, `pnpm run lint`, `pnpm run build`, and `git diff --check`; all should pass. Do not treat jsdom as proof that organization-selection `opaqueredirect` persists the cookie; Task 4 Playwright must verify this with two real browser contexts.
- [x] **Step 5: Document and review.** Update `docs/product/requirements.md` only if the implemented behavior changes its present wording; do not rewrite Accepted Decisions. Add a Task 3 completion record under `docs/development/` containing changed files, FR/NFR IDs, Accepted Decisions, verification commands/results, and the Task 4 browser E2E gap. Have implementer and reviewer perform the required final diff review and recheck any valid fixes. Commit Task 3 on a task branch after checks and review.

## Self-Review

- **Coverage:** Every Task 3 parent-plan state is assigned above: selection/sign-in (3.1), Draft/detail/audit (3.2), Pending/Approve and 401/403/409 recovery (3.3). Task 4 retains live OIDC/browser evidence.
- **Decision gate:** React/Vite, local state, fixed paths, API contract, cookie/CSRF, assigned Approver, and content rules are already Accepted. The plan makes no new product or architecture selection.
- **Interfaces:** `ApiClient` methods and DTO fields named here exist in Task 2; `WorkspaceProps` and the App callbacks are defined once above. The selected `requestId` is a Request identifier, not an Actor/Member identifier.
- **Review Focus:** The five conditions above have an owning test step. No test requires a real Keycloak or database for Task 3.

## Completion

Tasks 3.1–3.3 used implementer and reviewer gates. The whole-branch review and re-review ended with **Ready to merge: Yes**. Browser E2E remains Task 4. The approved plan's Request-detail organization-name wording was narrowed to the current OpenAPI DTO: `OrganizationSelectionCandidate` provides the name on the selection screen, while `Request` and `Session` do not provide it for detail display. No API contract change was introduced.
