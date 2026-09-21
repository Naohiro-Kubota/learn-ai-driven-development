# Task 4 完了記録: PostgreSQL workflow repository

更新日: 2026-09-21  
対象PR: [#4](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/4)  
マージcommit: `bf7bf3827edb2c207aeac529d44f51886beb731f`  
実装計画: `docs/superpowers/plans/2026-09-20-go-api-implementation.md` の Task 4

## 結果

Task 4 はレビュー承認後、2026-09-21 に `develop` へマージされた。PostgreSQL の手書き SQL adapter は `internal/application/requests.Repository` を実装し、Draft 作成・更新、既定 Approver の取得、Submit、Approve、Pending 一覧、Audit History の取得を提供する。

PDR-002 が要求する Organization ごとの既定 Approver に対し、既存 schema に永続化先がない矛盾を実装前に検出した。人間の承認後、同一 Organization の Member だけを参照できる `organizations.default_approver_member_id` を migration `000003` で追加した。role の有無は Member 参照とは独立して確認するため、role を持つ任意の Member を既定 Approver として選択することはない。

## 変更内容

- `migrations/000003_organization_default_approver.*.sql`
  - nullable な既定 Approver 参照を追加。
  - composite foreign key により、別 Organization の Member を設定できないようにした。
- `internal/store/postgres/requests.go`
  - `database/sql` と手書き SQL で application repository interface を実装。
  - 公開 Request ID、Approval ID、Audit Event ID はCSPRNGで生成する不透明な値とした。
  - Submit/Approve は、`id`、期待 `version`、期待 `status` を含む条件付き更新、Approval の書込み、Audit Event の追記を単一 transaction で処理する。
  - 更新件数が0の場合、not found、version conflict、invalid stateを区別する。
- `internal/store/postgres/requests_test.go` と `seed_test.go`
  - 実 PostgreSQL での条件付き更新、同時 Submit/Approve、監査追記失敗時の rollback、未割当 Approver の拒否、Pending 一覧、Audit Historyを検証。

## トレーサビリティ

- Requirements: FR-001、FR-003、FR-004、FR-005、FR-007、FR-011、NFR-001、NFR-003、NFR-004、NFR-006
- Architecture Decision: ADR-004、ADR-005、ADR-006、ADR-012
- Product Decision: PDR-001、PDR-002

## 検証記録

次のコマンドを成功確認した。

```bash
pnpm run test:db
GOTOOLCHAIN=go1.27.1 go vet ./...
pnpm run check:gofmt
pnpm run format:check
pnpm run lint
git diff --check
```

`pnpm test` は Task 4 差分外の既存設定問題により失敗する。Vitest が Node 標準 `node:test` の `scripts/check-gofmt.test.mjs` を Vitest suite として収集し、`No test suite found` を返すためである。`node --test scripts/check-gofmt.test.mjs` は成功する。この問題は Task 4 の scope 外として execution ledger に記録し、PR #4 のレビュー・マージ時にも明記した。

## 独立レビュー

独立レビューで指摘された以下の Important 項目は、マージ前に対応した。

- Submit/Approve の current version における invalid-state coverage
- Approve の unknown ID と stale version coverage
- 失敗した Submit が Approval/Audit Event を残さないこと
- test fixture が作る migration instance の resource cleanup

Critical 指摘はなかった。

## 次の作業

次は Task 5「OIDC transaction と不透明な server-side session」を実施する。詳細は `docs/development/handoff-2026-09-21-task5.md` を参照する。
