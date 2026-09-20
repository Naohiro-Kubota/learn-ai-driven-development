# ADR-006: 最初のVertical Sliceのテスト戦略

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-003, FR-005, FR-007, FR-011, FR-012, FR-013, NFR-001, NFR-003, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

最初のVertical Sliceは、Requestの状態遷移、認可、Audit History、HTTP API、ブラウザUIをまたぐ。NFR-003は、業務ルールと状態遷移を完全なE2E環境なしにテスト可能とすることを要求する。同時に、ユーザーが実際にDraft作成からApproved状態・監査履歴の確認まで完了できることを検証する必要がある。

このADRはテストの責務分担と採用するテスト基盤を決める。CI基盤、カバレッジ閾値、視覚回帰、負荷試験、セキュリティスキャンは初回Sliceに必須ではないため選定しない。

## Decision drivers

- 状態遷移、認可、監査記録、競合を高速で決定論的にテストできること
- HTTP契約のエラー・認可意味論を実際のハンドラー経由で検証できること
- ブラウザで最重要の利用者フローを少数のE2Eテストで確認できること
- TypeScriptとGoそれぞれで保守されている標準的なテスト手段を使用できること
- E2Eテストだけに依存せず、失敗箇所を局所化できること

## Options considered

### Option A: 層別テスト（Go標準testing、Vitest/Testing Library、Playwright）

Goのドメイン・アプリケーション層を`testing`で、HTTP層を`httptest`と実PostgreSQLで、ReactコンポーネントをVitestとTesting Libraryで検証する。さらにPlaywrightで主要な一連のブラウザフローをE2E検証する。

**利点**
- NFR-003に沿って業務ルールをE2Eから分離して検証できる
- HTTP境界と実データ永続化の統合不具合を検出できる
- 利用者フローがブラウザで成立することも確認できる

**欠点**
- Go、Node、ブラウザ自動化、PostgreSQLのテスト環境を保守する必要がある
- テスト種別ごとに責務を重複させない規律が必要になる

### Option B: Playwright中心のブラウザE2Eテスト

主要な振る舞いをすべてブラウザ自動化で確認し、少量の補助テストだけを置く。

**利点**
- 利用者視点で最初に検証できる
- テスト構成を一見単純にできる

**欠点**
- 状態遷移、競合、認可失敗の原因を局所化しにくく、実行が遅く不安定になりやすい
- NFR-003の「完全なE2E環境を必要としない」要件に適合しない

### Option C: Go unit/integration testとTypeScript component testのみ

ドメイン、HTTP、UIコンポーネントを分離してテストし、ブラウザ自動化を実施しない。

**利点**
- 実行が速く、CI環境を単純にできる
- 各層の失敗を局所化しやすい

**欠点**
- 実際の認証、cookie、画面遷移、API接続を通した利用者フローを検証できない
- 初回SliceのEnd-to-End実証として不十分である

## Decision

**層別テストを採用することを推奨する。** 具体的には、Go標準の`testing`をドメイン・アプリケーション層に、`httptest`と実PostgreSQLをHTTP/永続化統合テストに、VitestとTesting LibraryをReactコンポーネントテストに、Playwrightを主要利用者フローのE2Eテストに使用する。

テスト責務は次のとおりとする。

- ドメイン/アプリケーションテスト: Draft→Pending→Approvedの許可状態遷移、禁止遷移、監査イベント、期待versionによる競合を実DB・HTTPなしで検証する
- HTTP/永続化統合テスト: 認証・認可、入力不正、404、409、JSON契約、トランザクションとmigrationを検証する
- UIコンポーネントテスト: フォーム入力、API結果の表示、エラーと競合メッセージを検証する
- E2Eテスト: RequesterがDraftを作成・Submitし、ApproverがApproveし、RequesterがApproved状態とAudit Historyを確認する最小フローを検証する

各バグは、再現に必要な最も低い層に回帰テストを追加する。E2Eテストだけで業務ルールを網羅しない。

## Rationale

このプロダクトのリスクは、UIが表示されることだけではなく、認可されないApprove、競合した状態遷移、監査記録の欠落である。Go標準テストとHTTP統合テストはこれらを高速かつ決定論的に検証し、Playwrightは実際のブラウザ利用者フローを確認する。ReactのUIテストを加えることで、APIが正しくてもフォームやエラー表示が壊れる問題を局所化できる。

## Consequences

### Positive

- 業務ルールと状態遷移を高速なテストで守り、NFR-003を満たせる
- API契約、認可、DBトランザクション、UI、ブラウザ統合をそれぞれ適切な層で検証できる
- E2E失敗時に低い層のテストから原因を絞り込める

### Negative / trade-offs

- Vitest、Testing Library、Playwrightという開発依存関係とブラウザ実行環境を導入する
- 実PostgreSQLを使う統合テストのセットアップとデータ隔離を整備する必要がある
- 同じ要件を複数層で無差別に重複テストしないよう、責務をレビューする必要がある

## Validation

- CIまたは同等の再現可能なローカルコマンドで、Go unit、Go HTTP/DB integration、TypeScript component、Playwright E2Eの各スイートを個別に実行できることを確認する
- 意図的に禁止状態遷移、403、409、Audit Event欠落、UI表示不整合を導入した場合に、それぞれ対応する層のテストが失敗することを確認する
- E2Eが最小フローを検証しつつ、状態遷移全ケースの唯一のテストになっていないことをレビューする

## Revisit conditions

- ブラウザ互換性、アクセシビリティ、視覚回帰、性能、セキュリティ要件により別のテスト種別が必要になった場合
- テスト実行時間または不安定性がフィードバック速度を損ない、並列化・環境分離・テスト選別が必要になった場合
- API契約の生成または外部利用開始により、consumer-driven contract testの導入が合理的になった場合
