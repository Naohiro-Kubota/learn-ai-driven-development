# ADR-004: 最初のVertical Sliceの永続化、DBアクセス、およびスキーママイグレーション

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-001, FR-003, FR-004, FR-005, FR-007, FR-011, NFR-003, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

最初のVertical Sliceは、Organization、MemberとRole、Request、Approval、Audit Eventを永続化する必要がある。特に、Submit/Approveの状態遷移、監査記録の追記、同一Requestへの競合操作は、同一の整合性境界で扱う必要がある。ローカル開発では、初期スキーマと変更履歴を再現可能に適用できなければならない。

Persistence、DBアクセス、Migrationは相互に強く依存する。別々のライブラリや規約を先に採用すると初回Sliceの実装境界が不整合になるため、本ADRでは3点を一体として提案する。通知キュー、検索エンジン、キャッシュ、分析基盤は初回Sliceには必要ないため対象外とする。

## Decision drivers

- Request状態遷移とAudit Event追記を原子的に永続化できること
- 楽観的並行性または同等の仕組みで競合操作を決定論的に扱えること
- SQL、トランザクション、スキーマ変更をレビュー・テストできること
- Goアプリケーションの本番依存関係と隠れたORM挙動を最小化できること
- 新規開発者が文書化したコマンドでローカルDBを再現できること

## Options considered

### Option A: PostgreSQL、`database/sql`、手書きSQL、`golang-migrate`

PostgreSQLを主データストアにし、Go標準の`database/sql`を接続抽象として使用する。クエリは手書きSQLとしてリポジトリに置き、バージョン管理されたSQL migrationを`golang-migrate`で適用する。

**利点**
- トランザクション、制約、行ロック、条件付き更新を使って監査と競合安全性を実現できる
- SQLとスキーマ変更をレビュー可能な資産として残せる
- Go標準ライブラリのDB APIを中心にし、ORM依存を避けられる

**欠点**
- PostgreSQLドライバーとmigration CLI/APIという依存関係が必要になる
- SQLの記述・走査・テストを明示的に保守する必要がある
- ローカル開発にPostgreSQLサービスが必要になる

### Option B: SQLite、`database/sql`、手書きSQL、SQL migration

SQLiteをファイルベースのデータストアとして使用し、Go標準DB APIと手書きSQLでアクセスする。

**利点**
- ローカルセットアップを単純にできる
- SQLとトランザクションを用いる設計を維持できる

**欠点**
- 本番候補のマルチユーザー並行性・運用特性を初回Sliceで検証しにくい
- PostgreSQLとのSQL方言・ロック挙動の差が将来の移行コストになる

### Option C: PostgreSQLとORM/クエリ生成ツール

PostgreSQLを使用し、ORMまたはSQLからのコード生成ツールでDBアクセスを実装する。Migrationは同ツールまたは別ツールに委ねる。

**利点**
- 一部のCRUDや型変換の定型コードを削減できる
- 選択したツールによりクエリの型チェックを得られる

**欠点**
- DBアクセス方式、生成物、migrationの責務をツール固有の規約に委ねる
- トランザクション、更新件数、監査追記の意味を追跡しにくくなる場合がある
- 初回Sliceに対して評価・運用する依存関係が増える

## Decision

**PostgreSQL、Go標準ライブラリの`database/sql`、手書きSQL、`golang-migrate`によるバージョン管理SQL migrationを採用することを推奨する。** PostgreSQL用Goドライバーは`pgx`の`stdlib`互換ドライバーを使用することを推奨する。

Requestテーブルには単調増加する`version`を持たせる。SubmitおよびApproveは、期待versionを条件にした更新とAudit Eventの追記を単一トランザクションで実行する。条件付き更新が0行の場合は競合または状態不整合として扱い、API層へ409として変換する。監査記録はアプリケーション操作で更新・削除しない追記専用テーブルとする。Migrationは順序付きSQLファイルをリポジトリに保存し、アプリケーション起動時には自動実行しない。

## Rationale

PostgreSQLは、Requestの状態遷移とAudit Eventを同一トランザクションで確定し、競合時に期待versionを判定するための明示的な整合性機構を提供する。`database/sql`と手書きSQLは、重要な更新条件・更新件数・トランザクション境界をレビュー可能にし、初回SliceにORMを導入する必要をなくす。`golang-migrate`はスキーマの適用履歴を再現可能にするが、アプリケーション起動時の暗黙実行を避けることで運用上の権限と失敗を明確に分離できる。

## Consequences

### Positive

- 状態遷移、監査記録、並行性制御をRDBMSトランザクションで一貫して扱える
- SQL、migration、データ整合性制約をコードレビューとテストの対象にできる
- DBアクセスの本番依存はドライバーだけに留め、クエリ層のロックインを抑えられる

### Negative / trade-offs

- PostgreSQLのローカル起動、migration適用、テスト用DB準備が必要になる
- 手書きSQLの引数・結果マッピングを保守する必要がある
- `golang-migrate`とPostgreSQLドライバーのバージョン、脆弱性、運用手順を管理する必要がある
- Audit Eventを追記専用にするため、訂正は既存記録の更新ではなく新たなイベントとして表現する必要がある

## Validation

- 空のPostgreSQLに全migrationを適用し、必要なテーブル、制約、indexが作成されることを確認する
- transaction内でSubmit/ApproveとAudit Event追記がともに成功するか、ともにロールバックされることを統合テストで確認する
- 同一versionに対する並行Submit/Approveで一方のみが更新でき、もう一方が競合として検出されることを確認する
- migrationを新規ローカルDBに文書化されたコマンドで適用できることを確認する

## Revisit conditions

- 実測で手書きSQLの重複・マッピングが保守性を損ない、クエリ生成の導入便益が依存コストを上回る場合
- 要求された可用性、データ量、または運用環境がPostgreSQLの単一データベース構成では満たせない場合
- マルチテナント分離、リージョン配置、データ保持ポリシーがデータモデルまたはmigration運用を根本的に変更する場合
