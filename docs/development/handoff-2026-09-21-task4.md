# 引き継ぎ資料: Task 4 PostgreSQL workflow repository

更新日: 2026-09-21

対象PR: [#3](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/3)
実装計画: `docs/superpowers/plans/2026-09-20-go-api-implementation.md`

## 開始ゲート

Task 4 を開始する前に、PR #3 が `develop` にマージ済みであることを確認する。Task 3 の application service 契約を前提にするため、未マージの feature branch を起点に Task 4 を実装しない。

開始時には、以下を確認する。

- `gh auth status`
- `git fetch origin`
- Task 3 の commit `f1b92cb` と gofmt 修正 `d80a659` が `origin/develop` に含まれること
- `git status --short` が空であること

上記を満たした後、`origin/develop` から隔離 worktree と `codex/task-4-postgres-repository` branch を作成する。

## 現在地

| Task | 状態 | 主な成果 | Commit |
| --- | --- | --- | --- |
| Task 0 | 完了・承認済み | Biome、`gofmt`検査、`go vet`方針 | `0781852` |
| Task 1 | 完了・承認済み | runtime configuration、OIDC Go dependency固定 | `4425c94` |
| Task 2 | 完了・承認済み | PostgreSQL Compose環境、workflow/auth migration、統合テスト | `86ece7a` |
| Task 3 | PR #3・承認済み、マージ待ち | domain workflow と application service | `f1b92cb` |
| gofmt 修正 | PR #3 に追加済み | migration integration test の標準書式化 | `d80a659` |
| Task 4 | 未着手 | PostgreSQL repository と transactional concurrency test | — |

## 承認済みの主要Decision

- Go API は標準 `net/http`/`ServeMux` を使用し、業務規則を HTTP handler へ置かない（ADR-002）。
- PostgreSQL、`database/sql`、手書き SQL、`golang-migrate`を使用する（ADR-004）。
- 認可はサーバー側で評価し、Requester は自分の Draft、Approver は自分に割り当てられた Pending Request だけを操作する（ADR-005）。
- domain/application は Go 標準 `testing`、永続化は実 PostgreSQL integration test で検証する（ADR-006）。
- Go の formatter/static analysis は `gofmt` と `go vet` を使用する（ADR-012）。
- Title/Description、Draft 限定編集、Audit History は PDR-001、既定 Approver への単一 step 割当と自己承認禁止は PDR-002 に従う。

## Task 3 が提供する契約

`internal/application/requests.Repository` は Task 4 の PostgreSQL adapter が実装する interface である。HTTP、SQL、OIDC の型を application package へ追加しない。

- `CreateDraft` と `UpdateDraft` は Request と Audit Event を受け取り、Request を返す。
- `DefaultApprover` は `domain.DefaultApprover` を返す。`MemberID` と `HasApproverRole` の両方を正しく設定する。既定 Approver が存在しない場合は `domain.ErrNotFound`、Approver role を持たない場合は `HasApproverRole=false` を返す。application service がいずれも `ErrApprovalRoutingUnavailable` として扱う。
- `Submit` と `Approve` は `ExpectedVersion`、状態変更の必要情報、Audit Event を command として受け取る。
- `Get`、`GetApproval`、`ListPending`、`ListAuditEvents` は domain DTO を返す。

Task 3 で使用する Audit Event 名は、OpenAPI の enum と一致させた `request_created`、`request_updated`、`request_submitted`、`request_approved` である。

## Task 4 の実装範囲

作成するファイルは次に限定する。

- `internal/store/postgres/requests.go`
- `internal/store/postgres/requests_test.go`
- `internal/store/postgres/seed_test.go`

次の作業は Task 4 の範囲外である。

- HTTP handler、OIDC/session、frontend
- migration の追加・変更
- ORM、query builder、test container などの新規 dependency
- workflow の再割当、Reject、Cancel、複数 Approval Step

## 永続化と整合性の要件

`migrations/000001_initial_workflow.up.sql` の `requests`、`approvals`、`audit_events` を使用する。

- Submit と Approve は各々 1 個の PostgreSQL transaction で、Request の条件付き更新、Approval の作成または更新、Audit Event の追記を完了させる。
- Request 更新の predicate には必ず `id`、`version`、期待する `status` を含める。`RowsAffected()==0` の場合、最新 Request を確認して `ErrVersionConflict` と `ErrInvalidState` を区別する。
- 条件付き更新が失敗した場合、Approval/Audit Event を書き込まない。成功する Submit/Approve ごとに Audit Event はちょうど 1 件である。
- Submit は Organization の既定 Approver を lookup し、member role が `approver` であるか確認する。Requester 自身への routing は application service が拒否するが、repository も不正な command を永続化しない。
- Pending 一覧は assignee と pending Approval を基準にし、他 Member の Request を返さない。
- table row や `database/sql` の型を domain/application へ公開せず、adapter 内で domain DTO に変換する。

## TDD と integration test

最初に `requests_test.go` と `seed_test.go` を追加し、repository 未実装による失敗を確認する。テストは実 PostgreSQL を使用し、SQLite や in-memory 代替を使わない。

最低限、以下を test する。

- Draft 作成・更新、Submit、Approve の正常系
- Request ID、期待 version、期待 status のいずれかが異なる条件付き更新の失敗
- 同じ `expectedVersion` の Submit または Approve を goroutine 2 本で実行したとき、1件だけ成功し、もう1件は `ErrVersionConflict`、Audit Event は重複しないこと
- Submit/Approve の Audit Event が同じ transaction 内で追記されること
- 未割当 Approver による操作が永続化されないこと
- `ListPending` が割当済み Pending Request だけを返すこと

`pnpm run test:db` は `compose.test.yaml` で PostgreSQL を起動し、`TEST_DATABASE_URL` を注入して `internal/store/postgres` package 全体を実行した後、container、network、volume を破棄する。通常の `go test ./...` は `TEST_DATABASE_URL` がないと `internal/store/postgres` integration test で失敗するため、環境なしの品質確認に用いない。

## 検証コマンド

固定 toolchain を使う。

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

Task 4 の完了前に、`.superpowers/sdd/2026-09-20-go-api-implementation/progress.md` へ日本語で判断と実行記録を追記する。この directory は git 管理対象外である。

## 未解決リスク・次のDecision

- Task 4 は SQL transaction と concurrent update の実装であり、実 DB integration test が transaction 境界を直接証明する必要がある。
- PostgreSQL adapter の ID 生成方式は公開 ID が不透明であるという migration/contract の前提を満たす必要がある。既存の Accepted Decision と矛盾する新方式・依存を導入しない。
- OIDC transaction、session、CSRF は Task 5 の範囲であり、Task 4 に先取りしない。
