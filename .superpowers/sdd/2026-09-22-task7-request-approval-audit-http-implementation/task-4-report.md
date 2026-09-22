# Task 4 verification report

実施日: 2026-09-22

## 結果

Task 4 の完了条件を達成した。初回検証で判明したテストの順序仮定をテスト側だけ修正し、completion evidence を作成した。本番コードの変更はない。

## 検証コマンド

| Command | Result |
| --- | --- |
| `source /Users/nao/.nvm/nvm.sh; nvm use 26.9.0; GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db` | PASS。`scripts/test-postgres.mjs` が `compose.test.yaml` の一時 PostgreSQL (`127.0.0.1:55432/approval_flow_test`) を起動し、テスト後に container/network/volume を破棄。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests ./internal/store/postgres ./internal/httpapi ./cmd/api -count=1` | FAIL。`TEST_DATABASE_URL` 未設定のため、postgres 統合テストが `TEST_DATABASE_URL is required` で失敗。production DB には接続していない。 |
| `docker compose -f compose.test.yaml up -d --wait`; `TEST_DATABASE_URL=postgres://test_user:test_password@127.0.0.1:55432/approval_flow_test?sslmode=disable GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests ./internal/store/postgres ./internal/httpapi ./cmd/api -count=1` | 初回はFAIL。`TestApprovalAndAuditReadModelPreservesOpaqueIDsAndMetadata` の作成順仮定が原因。テスト修正後は全4 package PASS。DB は終了時に `docker compose ... down -v` で破棄。 |
| `TEST_DATABASE_URL=... GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestApprovalAndAuditReadModelPreservesOpaqueIDsAndMetadata -count=10` | 初回はFAIL（10回中4回）。テスト修正後はPASS。metadataをイベント種別で検証し、`(occurred_at,id)` の順序検証は維持。 |
| `pnpm install --frozen-lockfile` | PASS。 |
| `pnpm run format:check` | PASS。 |
| `pnpm run lint` | PASS。 |
| `pnpm run typecheck` | PASS。 |
| `pnpm run check:gofmt` | PASS。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...` | PASS。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go mod verify` | PASS。 |
| `git diff --check` | PASS。 |

## 失敗の詳細と解決

`internal/store/postgres/requests_test.go:87` が `events[0..2]` の作成順を暗黙に仮定していた。全イベントが同じ `occurred_at` で、実装はopaqueなランダムIDをtie-breakerにするため、テスト側をイベント種別のmapで検証するよう修正した。未解決事項はない。

## Decision traceability

検証対象は ADR-002/003/005/011/013 および PDR-001/002 に従う前提で確認した。今回、新しい Product Decision / ADR は作成していない。browser E2E は実施していない。

## Commit

関連コミット: `3a4d7d37612959f4d1f2a7e2e44363432cb3d67c`（completion evidence とテスト修正）。この報告書の更新コミットSHAはコミット後に追記する。
