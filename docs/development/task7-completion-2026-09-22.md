# Task 7 completion evidence

実施日: 2026-09-22

## 変更ファイル

- `internal/auth/session.go`
- `internal/domain/request.go`
- `internal/application/requests/repository.go`
- `internal/application/requests/service.go`
- `internal/application/requests/service_test.go`
- `internal/store/postgres/requests.go`
- `internal/store/postgres/requests_test.go`
- `internal/store/postgres/seed_test.go`
- `internal/store/postgres/sessions.go`
- `internal/store/postgres/sessions_test.go`
- `internal/httpapi/errors.go`
- `internal/httpapi/response_dto.go`
- `internal/httpapi/response_dto_test.go`
- `internal/httpapi/request_handlers.go`
- `internal/httpapi/request_handlers_test.go`
- `internal/httpapi/router.go`
- `internal/httpapi/auth_handlers.go`
- `internal/httpapi/session_handlers.go`
- `internal/httpapi/session_handlers_test.go`
- `cmd/api/main.go`

Request/Approval/Audit の server-derived read model、OpenAPI DTO/error mapping、7 operation の HTTP handler/route wiring、PostgreSQL persistence とテストを実装した。監査イベントテストは `(occurred_at,id)` の取得順検証を維持し、metadata の内容検証をイベント種別で行うことで opaque ID のランダム性に依存しないようにした。

## Decision traceability

実装は ADR-002、ADR-003、ADR-005、ADR-011、ADR-013、および PDR-001、PDR-002 に依拠する。今回、新しい Product Decision / ADR は不要だった。browser E2E は実施していない。

## 検証結果

- `source /Users/nao/.nvm/nvm.sh; nvm use 26.9.0; GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db`: PASS。リポジトリの `compose.test.yaml` による一時 PostgreSQL と `TEST_DATABASE_URL` を使用し、終了後に破棄。
- `TEST_DATABASE_URL=postgres://test_user:test_password@127.0.0.1:55432/approval_flow_test?sslmode=disable GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests ./internal/store/postgres ./internal/httpapi ./cmd/api -count=1`: PASS。隔離DBを使用。
- `TEST_DATABASE_URL=... GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestApprovalAndAuditReadModelPreservesOpaqueIDsAndMetadata -count=10`: PASS。
- `pnpm install --frozen-lockfile`: PASS。
- `pnpm run format:check`: PASS。
- `pnpm run lint`: PASS。
- `pnpm run typecheck`: PASS。
- `pnpm run check:gofmt`: PASS。
- `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...`: PASS。
- `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go mod verify`: PASS。
- `git diff --check`: PASS。

未解決事項: なし。テスト用PostgreSQL container/network/volume は検証後に破棄済み。
