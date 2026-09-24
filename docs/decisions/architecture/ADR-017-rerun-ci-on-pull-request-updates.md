# ADR-017: Pull request更新時のCI再実行

- Status: Accepted
- Date: 2026-09-24
- Owners: Human project owner
- Related requirements: NFR-001, NFR-002, NFR-003, NFR-004, FR-011
- Supersedes: ADR-016
- Superseded by: none

## Context

ADR-016は、GitHub ActionsのCIをpull requestの`opened`イベントだけで実行すると決めた。そのため、PR作成後にcommitを追加しても、その変更に対するCI結果が付かない。プロジェクトオーナーは、追加commitとPR再オープン時にもCIを実行するOption Bを採用した。CIで失敗する変更をbase branchへマージしないことが目的である。

CI基盤、品質ゲート、toolchain、権限、使い捨てテスト環境に関するADR-016の判断は維持し、起動イベントだけを見直す。

## Decision drivers

- PRの最新commitと再オープンしたPRをマージ前に検証できること
- PRを伴わないbranch pushではCIを起動しないこと
- 同一変更による重複実行と運用コストを抑えること
- 既存の品質ゲートを弱めないこと

## Options considered

### Option A: `opened`と`synchronize`で実行

PR作成時に加え、PRのhead branchが更新されたときにCIを実行する。`reopened`と単独の`push`は対象にしない。

**利点**
- 追加commitごとに最新のPR内容を検証できる
- PRを伴わないpushでは実行しない
- 既存のworkflowと検証内容を変えずに済む

**欠点**
- 追加commitのたびにDB/E2Eを含む全jobの実行時間と利用枠を消費する
- PR再オープンだけでは再実行されない

### Option B: `opened`、`synchronize`、`reopened`で実行

Option Aに加え、PRを再オープンしたときもCIを実行する。

**利点**
- 再オープン時にも検証結果を更新できる

**欠点**
- コード変更を伴わない再オープンでも全jobを実行する

### Option C: PRイベントに加えてbranchへの`push`で実行

PRイベントとbranch pushの双方でCIを実行する。

**利点**
- PR作成前の変更も検証できる

**欠点**
- PRがあるbranchへのpushで重複実行し得る
- PRを伴わないbranchでも実行し、PR中心の運用から外れる

## Decision

**Option Bを採用する。** `.github/workflows/ci.yaml`の起動条件を`pull_request`の`opened`、`synchronize`、`reopened`にする。`push`は追加しない。PR作成後の追加commitと再オープンに対して、frontend、Go、PostgreSQL統合、browser E2Eの全jobを再実行する。

ADR-016のGitHub Actions hosted runner、固定toolchain、frozen install、OpenAPI/format/lint/typecheck/vet/単体・DB統合・E2Eの品質ゲート、使い捨てcredentialと環境の後片付け、actionのcommit SHA固定、最小権限、非破壊の検査、品質ゲートを迂回しない運用を引き継ぐ。CI失敗時のマージ禁止には、base branchの保護設定で4つのjobを必須status checkに指定する必要がある。GitHub上の管理権限を要するため、workflow導入後に別途設定・確認する。

## Rationale

GitHubの`pull_request`における`synchronize`はPRのhead branch更新に、`reopened`は閉じたPRの再オープンに対応する。両方を含めることでマージ前にCI結果を更新し、PRを伴わないpushにはCIを広げずに済む。

## Consequences

### Positive

- PR作成後の追加commitと再オープンにもCI結果が付く
- 最新commitに必須status checkを要求するbranch保護と整合する

### Negative / trade-offs

- 追加commitと再オープンのたびに4つのjobが再実行される
- 必須status checkを設定するまでは、CI失敗だけでマージを禁止できない

## Validation

- workflowの起動条件が`pull_request: { types: [opened, synchronize, reopened] }`であり、`push`を含まないことを自動テストで確認する
- PR作成、追加commit、再オープンのそれぞれで全jobが起動することをGitHub上で確認する
- base branchの必須status checkに4つのjobを指定し、失敗時にマージできないことをGitHub上で確認する
- 既存の品質ゲート、権限、固定versionが変わらないことを確認する

## Revisit conditions

- PR再オープン時の再実行が不要になった場合
- 追加commitごとの実行時間や利用枠が運用上の問題になった場合
- PR作成前のbranch pushにもCIが必要になった場合
