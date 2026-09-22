# Task 4 verification report

実施日: 2026-09-22

## 結果

Task 4 の完了条件は未達。database-backed の対象パッケージ実行で既存テストが失敗したため、`docs/development/task7-completion-2026-09-22.md` は作成していない。実装ファイルの変更も行っていない。

## 検証コマンド

| Command | Result |
| --- | --- |
| `source /Users/nao/.nvm/nvm.sh; nvm use 26.9.0; GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db` | PASS。`scripts/test-postgres.mjs` が `compose.test.yaml` の一時 PostgreSQL (`127.0.0.1:55432/approval_flow_test`) を起動し、テスト後に container/network/volume を破棄。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests ./internal/store/postgres ./internal/httpapi ./cmd/api -count=1` | FAIL。`TEST_DATABASE_URL` 未設定のため、postgres 統合テストが `TEST_DATABASE_URL is required` で失敗。production DB には接続していない。 |
| `docker compose -f compose.test.yaml up -d --wait`; `TEST_DATABASE_URL=postgres://test_user:test_password@127.0.0.1:55432/approval_flow_test?sslmode=disable GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests ./internal/store/postgres ./internal/httpapi ./cmd/api -count=1` | FAIL。`internal/store/postgres` の `TestApprovalAndAuditReadModelPreservesOpaqueIDsAndMetadata` が失敗。DB は終了時に `docker compose ... down -v` で破棄。 |
| `TEST_DATABASE_URL=... GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestApprovalAndAuditReadModelPreservesOpaqueIDsAndMetadata -count=10` | FAIL（10回中4回）。同一 `occurred_at` のイベントを `ORDER BY occurred_at, id` で取得するため、ランダム opaque ID の順序により作成イベントが `events[0]` にならない。 |
| `pnpm install --frozen-lockfile` | PASS。 |
| `pnpm run format:check` | PASS。 |
| `pnpm run lint` | PASS。 |
| `pnpm run typecheck` | PASS。 |
| `pnpm run check:gofmt` | PASS。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...` | PASS。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go mod verify` | PASS。 |
| `git diff --check` | PASS。 |

## 失敗の詳細と未解決事項

`internal/store/postgres/requests_test.go:87` は `events[0]` が create、`events[1]` が submit、`events[2]` が approve であることを暗黙に仮定している。一方、テストヘルパーは全イベントに同じ `occurred_at` を設定し、実装は opaque なランダム ID を同値時の tie-breaker にしている。このため、同テストは非決定的に失敗する。Task 7 の実装またはテスト側で、イベント種別に依存しない検証、または作成順を表現する時刻・順序キーの扱いを決定する必要がある。

## Decision traceability

検証対象は ADR-002/003/005/011/013 および PDR-001/002 に従う前提で確認した。今回、新しい Product Decision / ADR は作成していない。browser E2E は実施していない。

## Commit

この報告書を最初に追加したコミット: `8526727fcff295bde4cbf69cdf8fcd7af5f148d0`。この行を含む最終コミット SHA は完了報告で示す。
