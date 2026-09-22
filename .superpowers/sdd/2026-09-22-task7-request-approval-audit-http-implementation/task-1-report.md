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

Filled after commit: `8e95a87f227d4fd33e12c94d278c3819b4d91a20`
