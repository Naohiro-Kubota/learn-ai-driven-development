# ADR-003: FrontendとBackend間のHTTP API契約方式

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-003, FR-005, FR-007, FR-011, FR-012, FR-013, NFR-001, NFR-002, NFR-003, NFR-004
- Supersedes: none
- Superseded by: none

## Context

ADR-002で、ブラウザUIとGoバックエンド間はJSON over HTTPと提案している。両者を別々に実装すると、リクエスト/レスポンス、認可失敗、状態競合、監査履歴の表現がずれるリスクがある。初回Sliceでも、API契約をレビュー・テスト可能な形で共有する方式が必要である。

契約は内部のデータベース表現を公開してはならない。初回Sliceでは、Draft作成、Submit、Pending一覧/詳細、Approve、Request詳細、Audit Historyを対象とする。状態変更は既存のRequest versionを明示して送信し、再試行時は同じ状態変更を安全に扱えることを検討する必要がある。

## Decision drivers

- FrontendとBackendが同じリクエスト、レスポンス、エラー意味論をレビューできること
- 認可、入力不正、状態遷移不能、並行性競合を契約に含められること
- 実装言語に依存せず、将来のクライアント・テストから利用できること
- 初回Sliceでコード生成や専用ゲートウェイを必須にせず、契約の更新コストを抑えること

## Options considered

### Option A: OpenAPI 3.1を正本とするschema-first契約

OpenAPI文書にHTTP操作、JSON schema、認証スキーム、成功・失敗応答を定義し、FrontendとBackendが同じ文書を参照する。コード生成の採否は別途評価する。

**利点**
- HTTP契約を実装前にレビューでき、変更を追跡できる
- エラー応答、認可、競合を操作ごとに明示できる
- 人手実装、契約テスト、将来のコード生成のいずれにも利用できる

**欠点**
- OpenAPI文書を実装と同期する規律と検証が必要になる
- 複雑な業務状態をschemaだけで完全に説明しようとすると読みにくくなる

### Option B: MarkdownのAPI仕様と手書きTypeScript/Go DTO

人間向けMarkdownでAPIを記述し、FrontendとBackendの型を別々に手書きする。

**利点**
- 追加ツールなしで始められる
- 仕様文を自由に記述できる

**欠点**
- 型とHTTP意味論の機械検証が難しく、二重管理になる
- API変更時に両実装との乖離を発見しにくい

### Option C: RPCまたはGraphQLのIDLを正本とする

Protocol Buffers/gRPC-Web、Connect、またはGraphQL schemaを契約として使用する。

**利点**
- 型付きクライアントやコード生成を早期に利用できる
- 操作中心またはクエリ中心のAPIを表現できる

**欠点**
- ブラウザ、Go、プロキシ、コード生成の基盤Decisionが追加で必要になる
- 初回Sliceの単純なHTTPリソース操作に対して導入・運用コストが大きい

## Decision

**OpenAPI 3.1文書をHTTP API契約の正本とすることを推奨する。** 契約はresource-orientedなJSON over HTTP操作を定義し、少なくとも次を含める。

- Draft Requestの作成、Requestの取得、Pending Requestの一覧取得、Submit、Approve、Audit Historyの取得
- 認証の要求、操作ごとの認可失敗、入力不正、対象不在、状態競合の応答
- 内部DBモデルから分離されたRequest、Approval、Audit EventのDTO
- 状態変更で使用するRequest versionと、409競合時にクライアントが最新状態を再取得する規約
- 破壊的変更を避けるための互換性方針。API versionの伝達方式と互換性のない変更時の運用はADR-008で決定する

初回SliceではOpenAPIからのサーバースタブまたはクライアントの自動生成は採用しない。生成導入は、手書きDTOの同期コストが実測で問題になった場合に別Decisionとして評価する。

## Rationale

OpenAPIは、ブラウザUIとGoサーバーが異なる言語で共有する契約を、ベンダー固有の実行基盤なしに明示できる。状態変更と監査が重要な本プロダクトでは、成功形だけでなく認可・競合・再取得の意味論を契約に含めることが必要である。コード生成を先送りすれば、契約の価値を得ながら初期ツールチェーンを最小にできる。

## Consequences

### Positive

- APIを実装前に人間がレビューでき、NFR-002のトレーサビリティを支援する
- FrontendとBackendの契約テストの基準を共有できる
- 将来、生成クライアントや外部利用者が必要になっても移行の出発点がある

### Negative / trade-offs

- OpenAPI文書と実装の同期をCIまたはテストで確認する必要がある
- 初回SliceでもDTO、エラーコード、versionの意図的な設計が必要になる
- API versioning方式がADR-008で決まるまで、version表現を実装できない

## Validation

- OpenAPI文書の構文と参照を検証する
- HTTP統合テストが、文書化した成功・失敗status、JSON形式、エラーコードに一致することを確認する
- ブラウザE2Eテストで、409競合が発生した場合に最新状態を再取得してユーザーに説明できることを確認する

## Revisit conditions

- 外部クライアント、モバイルクライアント、または多数のAPI利用者が加わり、コード生成の費用対効果が正になる場合
- APIがストリーミング、subscription、複合クエリを必要とし、REST/HTTP表現が明確さを失う場合
- ADR-008で決まるversioning方式または互換性・廃止方針が、組織標準と両立しなくなった場合
