# ADR-016: CI実行基盤と品質ゲートの適用方法

- Status: Superseded
- Date: 2026-09-24
- Owners: Human project owner
- Related requirements: NFR-001, NFR-002, NFR-003, NFR-004, FR-011
- Supersedes: none
- Superseded by: ADR-017

## Context

ADR-006は層別テスト、ADR-007は固定したNode.js・pnpmとfrozen install、ADR-012は非破壊のformat/lint検査と独立した`go vet`を要求する。`docs/superpowers/plans/2026-09-20-go-api-implementation.md`もCIからの利用を予定している。一方、リポジトリにはCI workflowがなく、現状はローカルコマンドを手動実行するのみである。ADR-006はCI基盤を選定対象外としており、実行基盤と必須ゲートを新たに決める必要がある。

Gitの`origin`はGitHubである。検査対象にはNode.js 26.9.0、pnpm 12.5.1、Go 1.27.1、実PostgreSQLの統合テスト、およびKeycloak・Chromiumを用いるE2Eがある。CIによる品質ゲートは複数領域の共通運用と外部実行環境を定めるため、承認前にworkflowを導入しない。

## Decision drivers

- pull request作成時に、失敗をレビュー前に検出できること
- ADR-006/007/012の既存コマンドを、品質基準を弱めず再現できること
- format/lint検査でworking treeやlockfileを書き換えないこと
- 実DBとブラウザテストの隔離、秘密情報の非公開、実行権限を管理できること
- 運用負担、実行時間、将来の移行コストを抑えること

## Options considered

### Option A: GitHub Actionsのhosted runner

GitHubでpull requestが作成されたときだけworkflowを実行する（`pull_request`の`opened`イベント）。`synchronize`・`reopened`イベントおよびbranchへのpushでは実行しない。固定toolchainを明示し、既存のfrozen install、OpenAPI検証、format/lint、型チェック、Go vet、単体・DB統合・E2Eテストを独立した失敗として表示する。E2Eには実行時生成の使い捨てcredentialを使い、Docker資源を終了時に削除する。

**利点**
- 現在のGitHubリポジトリとレビュー画面へ直接結果を返せる
- runnerの保守をプロジェクトで引き受けずに済む
- 既存のローカル検証コマンドを再利用できる

**欠点**
- GitHub Actionsの実行時間・runner availability・利用枠に依存する
- actionとrunner imageの更新を監視し、第三者actionの参照を固定する必要がある
- Docker/Chromium E2Eの起動時間と失敗時の診断コストがある

### Option B: 自己管理runnerまたは別のCIサービス

同じコマンド群を専用runnerで実行し、GitHubへ結果を連携する。

**利点**
- runner image、Docker、cache、実行資源を細かく制御できる
- 組織の既存CI基盤へ統合しやすい場合がある

**欠点**
- runner更新、隔離、秘密情報、障害対応を継続的に運用する必要がある
- 現時点で既存の組織CI基盤は確認されておらず、初期導入コストが大きい

### Option C: ローカル検証コマンドのみを維持

`pnpm run check`、Go検査、DB/E2Eテストを開発者とレビュー担当者が手動実行する。

**利点**
- 外部CIの設定と実行費用が不要
- 現行コマンドをそのまま使用できる

**欠点**
- レビュー前の実行を保証できず、未実行や環境差を検出しにくい
- 計画で求めるCIへの品質ゲート接続を満たさない

## Decision

**Option Aを採用する。** GitHub Actionsのhosted runnerをCI基盤とし、pull request作成時の`opened`イベントでのみ実行する。少なくとも次を別々に実行して失敗を表示する。

1. 固定Node.js/pnpmによる`pnpm install --frozen-lockfile`、`pnpm run verify:openapi`、`pnpm run format:check`、`pnpm run lint`、`pnpm run typecheck`、`pnpm test`、`pnpm run build`
2. 固定Goによる`pnpm run check:gofmt`、`go vet ./...`、DB不要のGoテスト、`go mod verify`
3. 隔離したPostgreSQLでの`pnpm run test:db`
4. 隔離したPostgreSQL・Keycloak・Chromiumでの`pnpm run test:e2e`

テスト用credentialは実行時に生成し、repositoryへ保存しない。外部actionは変更不能なcommit SHAで参照し、権限はcontents readなど必要最小限にする。format/lint、frozen install、OpenAPI検証はworking treeを書き換えない。workflow内で検査を迂回するskipや`continue-on-error`を設けない。必須status checkのbranch保護設定はGitHub上の管理権限を要するため、workflow導入後に別途確認する。その際、PR作成後のcommitにはCI結果が付かないことを考慮する。

## Rationale

既存のGitHubレビュー経路に結果を直接結び付けられ、自己管理runnerを先行運用せずに計画の品質ゲートを自動化できる。検査コマンドと期待結果は既存のAccepted ADRとリポジトリにあるため、新しいテスト基盤や本番依存関係を導入する必要がない。

## Consequences

### Positive

- レビュー時に検証結果を追跡でき、未実行の見落としを減らせる
- ローカルとCIで同じ固定toolchain・コマンドを使用できる
- DB/E2Eの隔離と後片付けをworkflowで明示できる

### Negative / trade-offs

- runner/actionの更新と固定SHAの管理が必要になる
- DB/E2Eは追加の実行時間と失敗診断を要する
- PR作成後の追加commitは自動検証されない。最新commitに必須status checkを要求すると、再実行手段がない場合にmergeできなくなる
- branch保護を設定するまでは、workflow失敗だけでmergeを禁止できない

## Validation

- pull request作成時に全検査が個別に成功・失敗として表示され、`synchronize`・`reopened`・branch pushでは起動しないことを確認する
- 意図的な未整形ファイル、lint違反、型エラー、Go vet違反、失敗するテストが各ゲートを失敗させることを確認する
- CI終了後、検査対象ファイルとlockfileに変更がないことを確認する
- DB/E2Eが隔離された一時環境で実行され、失敗時も生成資源を削除することを確認する
- workflowとactionの権限、credentialの生成・破棄、ログへの秘密情報混入がないことをレビューする

## Revisit conditions

- GitHub以外へリポジトリを移す場合
- hosted runnerの実行時間・利用枠・機能が必要な検証を満たさない場合
- 組織の標準CI基盤またはより厳格なrunner隔離要件が導入された場合
