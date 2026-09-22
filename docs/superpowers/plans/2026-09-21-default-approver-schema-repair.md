# 既定 Approver スキーマ補完 実装計画

> **エージェント実行者向け:** この計画は、`superpowers:subagent-driven-development`（推奨）または `superpowers:executing-plans` を使い、タスク単位で実行すること。各手順はチェックボックス（`- [ ]`）で管理する。

**目的:** Task 4 の PostgreSQL workflow repository を完了する前に、Accepted PDR-002 が定める Organization の既定 Approver 割当を永続化可能にする。

**アーキテクチャ:** Organization から既定 Approver となる Member への参照を記録する forward-only PostgreSQL migration を追加し、既存の実 PostgreSQL migration suite で検証する。Task 4 の repository は設定済みの正確な Member を取得し、別途 `approver` role を検証する。role を持つ任意の Member を選択してはならない。

**技術スタック:** PostgreSQL 17、`database/sql`、手書き SQL、golang-migrate、Go 標準 `testing`、既存の Compose 起動 PostgreSQL integration runner。

**仕様:** `docs/decisions/product/PDR-002-initial-approval-routing.md`、`docs/decisions/architecture/ADR-004-relational-persistence-data-access-and-migrations.md`、`docs/development/handoff-2026-09-21-task4.md`。

## 共通制約

- PDR-002 は、Organization ごとに設定済みの単一既定 Approver を要求する。`approver` role を持つ任意 Member は代替にならない。
- ADR-004 に従い、PostgreSQL、`database/sql`、手書き SQL、バージョン管理した `golang-migrate` migration、実 PostgreSQL integration test を使用する。
- migration は forward-only とし、既存の Accepted ルールを永続化する以外の依存関係、ORM、query builder、workflow 振る舞いを追加しない。
- Task 4 の Submit transaction は、Request 状態を条件付きで更新した後に限り Audit Event をちょうど1件追記する。
- `pnpm run test:db` をDBテストの実行入口とする。`TEST_DATABASE_URL` なしの通常の `go test ./...` は integration test の代替にしない。

## レビュー重点項目

- 設定済み Member が `approver` role を持たない場合、`DefaultApprover` は別 Member を選ばず `HasApproverRole=false` を返す。
- 別 Organization の Member を既定 Approver に設定しようとすると、データベースの外部キー制約で拒否される。
- Organization から参照されている既定 Approver Member は削除できない。
- Organization の既定 Approver を変更しても、既存 Pending Approval の assignee は保持される。
- provisioning 前の Organization は表現可能であり、新規 Submit は既存の routing-unavailable 経路で失敗する。

### Task 1: Organization の既定 Approver を永続化し検証する

**ファイル:**

- 作成: `migrations/000003_organization_default_approver.up.sql`
- 作成: `migrations/000003_organization_default_approver.down.sql`
- 変更: `internal/store/postgres/migrations_test.go`
- 変更: `docs/development/handoff-2026-09-21-task4.md`
- 変更: `docs/superpowers/plans/2026-09-20-go-api-implementation.md`

**Interface:**

- 提供: nullable な `organizations.default_approver_member_id text REFERENCES members(id)`。
- 利用元: Task 4 の `internal/store/postgres.Repository.DefaultApprover(context.Context, organizationID)`。

- [x] **Step 1: 失敗する migration integration test を書く**

全 migration を適用し、Organization `org-default`、それに属する Member `approver-default`、別 Organization に属する `other-org-member` を作成するテストを追加する。`org-default.default_approver_member_id = 'approver-default'` は成功し、`other-org-member` の設定は失敗することを確認する。既存の `TEST_DATABASE_URL` と実 PostgreSQL 接続を使用する。

```go
func TestDefaultApproverBelongsToOrganization(t *testing.T) {
	db := openMigratedDatabase(t)
	seedOrganizationAndMember(t, db, "org-default", "approver-default")
	seedOrganizationAndMember(t, db, "org-other", "other-org-member")
	if _, err := db.Exec(`UPDATE organizations SET default_approver_member_id = $1 WHERE id = $2`, "approver-default", "org-default"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE organizations SET default_approver_member_id = $1 WHERE id = $2`, "other-org-member", "org-default"); err == nil {
		t.Fatal("cross-organization default approver was accepted")
	}
}
```

- [x] **Step 2: migration test を実行し RED を確認する**

Run: `pnpm run test:db`

期待結果: `default_approver_member_id` が存在しないため FAIL。

- [x] **Step 3: 最小の forward-only migration を追加する**

`organizations.default_approver_member_id` を nullable な列として追加し、参照先 Member が同一 Organization に属することを強制する。`members` の `(id, organization_id)` に composite unique key を追加し、`(default_approver_member_id, id)` からその key への composite foreign key を設定する。列を non-null にしてはならない。provisioning 未完了は許可され、Task 3 は設定未完了を `ErrApprovalRoutingUnavailable` に変換済みである。

```sql
ALTER TABLE members ADD CONSTRAINT members_id_organization_id_key UNIQUE (id, organization_id);
ALTER TABLE organizations ADD COLUMN default_approver_member_id text;
ALTER TABLE organizations
  ADD CONSTRAINT organizations_default_approver_same_organization_fkey
  FOREIGN KEY (default_approver_member_id, id)
  REFERENCES members (id, organization_id);
```

down migration では、外部キー、列、補助 unique constraint の順に依存関係を保って削除する。

- [x] **Step 4: DB test を実行し GREEN を確認する**

Run: `pnpm run test:db`

期待結果: PASS。テスト終了後に Compose の service、network、volume が削除される。

- [x] **Step 5: Task 4 文書を同期して commit する**

元の計画と handoff の Task 4 ファイル一覧・対象範囲に、PDR-002 の永続化修復として migration `000003` を追加する。`.superpowers/sdd/2026-09-20-go-api-implementation/progress.md` へ、検出した schema 矛盾、人間による承認、実 PostgreSQL test のコマンドと結果を日本語で追記する。

```bash
git add migrations/000003_organization_default_approver.up.sql migrations/000003_organization_default_approver.down.sql internal/store/postgres/migrations_test.go docs/superpowers/plans/2026-09-20-go-api-implementation.md docs/development/handoff-2026-09-21-task4.md
git commit -m "fix: persist organization default approver"
```

### Task 2: 修復済み schema を前提に Task 4 を完了する

**ファイル:**

- 作成: `internal/store/postgres/requests.go`
- 作成: `internal/store/postgres/requests_test.go`
- 作成: `internal/store/postgres/seed_test.go`

**Interface:**

- 利用: `organizations.default_approver_member_id`、`member_roles`、`internal/application/requests.Repository`。
- 提供: `NewRepository(*sql.DB) *Repository`。SQL row を `internal/store/postgres` 外へ公開せず、`requests.Repository` の全 method を実装する。

- [x] **Step 1: 失敗する repository integration test を書く**

既定 Approver に Approver Member を設定した Organization、Requester、未割当 Admin を seed する。Draft 作成・更新、誤った ID/version/state の条件付き Submit/Approve error、同時 Submit/Approve の1成功1競合、条件失敗後に Audit Event がないこと、未割当 Approve の拒否と非永続化、assignee 限定の `ListPending` を test する。

- [x] **Step 2: focused test を実行し RED を確認する**

Run: `pnpm run test:db`

期待結果: `NewRepository` と PostgreSQL adapter が存在しないため FAIL。

- [x] **Step 3: 明示的な SQL repository を実装する**

既存 application interface を実装する。`DefaultApprover` は設定済み Organization 参照を読み、設定済み ID と独立した `approver` role 判定を返す。Submit/Approve はそれぞれ1 transaction で、`id`、`version`、期待する `status` により `requests` を条件更新し、`RowsAffected` を確認する。条件成功後に限り Approval と Audit Event を書き込む。not-found/version-conflict/invalid-state を区別する。

- [x] **Step 4: 完全な検証 suite を実行する**

Run:

```bash
source /Users/nao/.nvm/nvm.sh
nvm use 26.9.0
pnpm install --frozen-lockfile
pnpm run test:db
GOTOOLCHAIN=go1.27.1 go vet ./...
pnpm run check:gofmt
pnpm run format:check
pnpm run lint
git diff --check
```

期待結果: すべて exit 0。

- [x] **Step 5: Task 4 を commit し execution ledger を更新する**

```bash
git add internal/store/postgres/requests.go internal/store/postgres/requests_test.go internal/store/postgres/seed_test.go
git commit -m "feat: persist workflow transitions atomically"
```

`.superpowers/sdd/2026-09-20-go-api-implementation/progress.md` へ Task 4 のコマンドと結果を日本語で追記する。

## 自己レビュー

- PDR-002 の「設定済み、単一、Organization 内」の既定 Approver は、Task 1 のデータ制約と Task 2 の lookup で満たす。
- ADR-004 の手書き migration、SQL repository、transaction 境界、実 PostgreSQL test の責務は、2つの Task で満たす。
- 新規 dependency、API 契約、workflow 状態遷移、role model は導入しない。
- レビュー重点項目は、Task 1 の外部キー test と Task 2 の repository test に割り当てる。
