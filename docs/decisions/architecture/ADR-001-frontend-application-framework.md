# ADR-001: 最初のVertical Sliceのフロントエンドアプリケーションフレームワーク

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-003, FR-005, FR-007, FR-009, FR-012, NFR-003, NFR-004
- Supersedes: none
- Superseded by: none

## Context

最初のVertical Sliceでは、申請者がDraftを作成・Submitし、承認者がPending申請を確認してApproveし、申請者がApproved状態およびAudit Historyを確認するブラウザUIが必要である。フロントエンド実装言語はTypeScriptに固定されているが、アプリケーションフレームワークは未選定である。

この選定は、画面の状態更新、フォーム、一覧、API呼び出しの境界、およびテスト方法に影響する。一方で、初回Sliceではクライアント側ルーティング、グローバル状態管理、UIコンポーネントライブラリは必須ではないため、本ADRでは選定しない。React/Viteの依存関係を取得・固定するパッケージマネージャーとJavaScriptサプライチェーン対策はADR-007で扱う。

## Decision drivers

- TypeScriptでフォーム、申請状態、監査履歴を明示的かつテスト可能に扱えること
- 初回Sliceのために追加する本番依存関係と学習コストを抑えられること
- HTTP API契約とUI表示を疎結合に保てること
- 将来の画面追加を妨げず、標準的な開発・テストツールで保守できること

## Options considered

### Option A: フレームワークなしのTypeScriptとWeb Platform API

DOM操作、画面状態、HTTP呼び出しをTypeScriptモジュールとして実装する。

**利点**
- 本番依存関係を最小化できる
- ブラウザ標準APIの理解だけで開始できる

**欠点**
- 複数の画面状態と非同期更新を扱う規約を独自に設計する必要がある
- UIの分割とコンポーネント単位テストの一貫性を保ちにくい

### Option B: ReactとVite

ReactをUIコンポーネントと画面ローカル状態の基盤に、ViteをTypeScript開発・ビルド基盤に用いる。

**利点**
- フォーム、一覧、状態遷移後の再描画を宣言的に実装できる
- React向けの成熟したTypeScript・テストエコシステムを利用できる
- ルーターや状態管理ライブラリを最初から導入せずに開始できる

**欠点**
- ReactとViteという依存関係・ツールチェーンを導入する
- JSX、コンポーネント境界、再描画モデルの学習が必要になる

### Option C: VueとVite

VueをUIコンポーネントと画面状態の基盤に、Viteを開発・ビルド基盤に用いる。

**利点**
- 単一ファイルコンポーネントでUI、状態、スタイルを近接配置できる
- TypeScriptとViteを使用できる

**欠点**
- Reactと同等に追加依存関係とフレームワーク固有の規約を導入する
- 本リポジトリにはVueを優先する既存のDecisionや資産がない

## Decision

**ReactとViteを採用することを推奨する。** Reactは画面コンポーネントと画面ローカル状態に限定して使用し、Viteは開発サーバーと本番ビルドに使用する。初回SliceではReact Router、グローバル状態管理、UIコンポーネントライブラリを導入しない。

## Rationale

初回Sliceには、作成フォーム、申請・監査履歴の表示、Submit/Approve後の状態反映という複数の相互に関連するUI状態がある。Reactはこれらを明示的なコンポーネント入力と状態として表現し、UIだけをバックエンドから分離してテストしやすい。フレームワークなしの実装より独自規約を減らしつつ、追加ライブラリを最小に留められる。Vueも実現可能だが、リポジトリ内の既存資産による優位性がないため、より広く利用されるReactの採用を推奨する。

## Consequences

### Positive

- 初回SliceのUIをコンポーネント単位で分割し、状態更新をテストできる
- API呼び出し層をコンポーネントから分離する設計を取りやすい
- 将来、画面数が増えた場合にルーターなどを追加できる

### Negative / trade-offs

- ReactとViteのアップデート、脆弱性対応、ビルド設定を保守する必要がある
- 初期実装者はReactの状態管理と副作用の境界を守る必要がある
- React固有のJSX/コンポーネント表現がフロントエンド実装に入る

## Validation

- TypeScriptの型チェックと本番ビルドが成功することを確認する
- Draft作成、Submit、Pending表示、Approve、Approved表示、Audit History表示をブラウザE2Eテストで確認する
- UIコンポーネントの表示・操作を、採用するTesting Strategyに従って単体またはコンポーネントテストで確認する

## Revisit conditions

- 画面間ナビゲーションやURL共有が初回Sliceの範囲に加わり、ルーティングDecisionが必要になった場合
- 複数画面で共有するクライアント状態が増え、Reactのローカル状態だけでは一貫性を維持できなくなった場合
- 組織の標準フロントエンド基盤または既存の設計システムとの統合が要求された場合
