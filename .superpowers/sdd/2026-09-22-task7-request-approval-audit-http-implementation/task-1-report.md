# Task 1 implementation report

## Changed files

- `internal/auth/session.go`: add server-derived `Principal.OrganizationID`; PostgreSQL session authentication joins `members` to populate it.
- `internal/domain/request.go`: expose opaque Approval/Audit IDs, nullable content snapshots, and approval metadata.
- `internal/application/requests/repository.go`: change approval read seam to `*domain.Approval`.
- `internal/application/requests/service.go`: add `Actor.OrganizationID`, add visibility-first `Service.GetApproval`, and preserve non-nil audit snapshots.
- `internal/store/postgres/requests.go`: decode Approval/Audit IDs and nullable JSON, persist Submit/Approve metadata using the same Approval ID, and order audit history by `(occurred_at, id)`.
- Tests/helpers in `internal/application/requests/service_test.go`, `internal/store/postgres/requests_test.go`, `internal/store/postgres/seed_test.go`, and `internal/store/postgres/sessions_test.go` cover the new read model.

## TDD evidence

RED was observed before production changes:

```text
go test ./internal/application/requests -run 'Test(GetApproval|CreateDraft)' -count=1
service.GetApproval undefined

go test ./internal/store/postgres -run 'Test(SessionPrincipal|Audit|Approval)' -count=1
session.Principal.OrganizationID undefined
```

GREEN:

```text
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/auth ./internal/application/requests -count=1
ok (both packages)

GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests -run 'Test(GetApproval|CreateDraft)' -count=1
ok

GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./... -run '^$'
ok (all packages compile)

GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./internal/auth ./internal/application/requests ./internal/store/postgres
ok
```

The PostgreSQL focused tests were executed but the environment did not provide `TEST_DATABASE_URL`, so they failed before database setup with `TEST_DATABASE_URL is required`:

```text
go test ./internal/store/postgres -run 'Test(SessionPrincipal|ApprovalAndAudit)' -count=1
FAIL (missing TEST_DATABASE_URL)
```

`git diff --check` passed.

## Concerns / unresolved

- PostgreSQL behavior (ID/metadata/nullability/order) still needs the repository's isolated database test run with `TEST_DATABASE_URL` configured.
- No new dependency or decision record was introduced; implementation follows the existing PostgreSQL/`database/sql` boundary and the Task 7 plan.

## Commit

Filled after commit: `d4ec3789236344a27fdb857a2fb38343858a09ce`

## Review fix round 1

### Changes

- `internal/httpapi/session_handlers.go` now copies the authenticated Principal's server-derived `OrganizationID` into `requests.Actor`; the HTTP test asserts this remains authoritative despite forged request values.
- `internal/application/requests/service.go` rejects a missing or mismatched actor organization with `domain.ErrNotFound` before requester, approver, or admin visibility checks. This constrains Admin `Get`, `GetApproval`, and `ListAuditEvents` to the same organization.
- Focused tests were added/updated in `internal/httpapi/session_handlers_test.go` and `internal/application/requests/service_test.go`.

### TDD evidence

RED before production changes:

```text
go test ./internal/httpapi -run 'TestRequireSessionUsesOnlyServerActorAndPropagatesContext' -count=1
FAIL: actor OrganizationID was empty

go test ./internal/application/requests -run 'TestAdminCannotReadAnotherOrganizationRequestApprovalOrAudit' -count=1
FAIL: Get/GetApproval/ListAuditEvents returned nil error for cross-organization admin
```

GREEN after the minimal fixes:

```text
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'TestRequireSessionUsesOnlyServerActorAndPropagatesContext' -count=1
ok

GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests -run 'Test(GetApproval|AdminCannotReadAnotherOrganizationRequestApprovalOrAudit|CreateDraft)' -count=1
ok

GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/auth ./internal/application/requests ./internal/httpapi -count=1
ok

GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./internal/auth ./internal/application/requests ./internal/httpapi
ok

git diff --check
ok
```

Review-fix commit: `66c89dae8c6721c90a0f76d056d336de704336ff`
