# ADR-002: 最初のVertical SliceにおけるGo HTTP APIの提供方式

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-003, FR-005, FR-007, FR-011, FR-013, NFR-001, NFR-003, NFR-004
- Supersedes: none
- Superseded by: none

## Context

ブラウザUIは、Draft作成、Submit、Pending申請の取得、Approve、申請詳細とAudit Historyの取得をバックエンドへ要求する。バックエンドの実装言語はGoに固定されているが、HTTP/APIの提供方式およびルーターは未選定である。

このSliceでは外部公開API、ストリーミング、双方向通信、RPC固有の性能要件はない。認可、エラー、状態変更時の競合検出をHTTP境界で明示し、ドメインルールをHTTP実装へ結合させないことが必要である。具体的なAPI契約の記述方式はADR-003で扱う。

JetBrainsの`go-modern-guidelines`は、Go 1.22以降の`ServeMux`について、HTTP methodを含むpatternとnamed path wildcard、`Request.PathValue`の使用を推奨している。本リポジトリにはまだ`go.mod`がないため、現時点ではGo toolchainの最小バージョンは未記録である。このADRの推奨はGo 1.22以上を前提とし、実装を始める前にその前提をtoolchain設定へ明記する。

## Decision drivers

- Go標準機能で明示的なルーティング、認証コンテキスト、エラー変換を実装できること
- 成功、認証失敗、権限不足、入力不正、対象不在、競合をクライアントがstatusとエラーコードから区別し、適切に画面表示できること
- 依存関係、運用負荷、フレームワーク固有の暗黙挙動を最小化できること
- ドメインサービスをHTTPハンドラーから分離してテストできること

## Options considered

### Option A: Go標準ライブラリの`net/http`と`ServeMux`

Go標準ライブラリのHTTPサーバー、ルーティング、JSONエンコードを使用し、ハンドラーはアプリケーションサービスへ処理を委譲する。

**利点**
- HTTP提供層の本番依存関係を追加しない
- Goの標準的な`http.Handler`、`context.Context`、`httptest`を利用できる
- ミドルウェア、認可、エラー応答の挙動を明示的に記述できる

**欠点**
- パラメータ抽出、ルート組み立て、入力検証の補助は自前で少量実装する必要がある
- 高度なルーティング機能は最小限である

### Option B: `chi`によるHTTPルーター

`chi`を導入し、ルーティングとミドルウェアの組み立てを補助する。

**利点**
- ルートパラメータやミドルウェアの記述を簡潔にできる
- 標準の`http.Handler`互換で移行性が比較的高い

**欠点**
- 初回Sliceで必須ではない本番依存関係を導入する
- 標準ライブラリだけで十分な範囲にもフレームワーク固有APIが入る

### Option C: GinなどのフルスタックHTTPフレームワーク

ルーティング、バインディング、検証、ミドルウェアをまとめて提供するフレームワークを導入する。

**利点**
- 一部の定型的なハンドラー実装を短縮できる
- エコシステムの補助機能を使える

**欠点**
- フレームワークの独自コンテキストと暗黙挙動に結合しやすい
- 初回Sliceの要件に対して依存関係と学習・保守コストが大きい

## Decision

**Go 1.22以上の標準ライブラリ`net/http`と`ServeMux`で、JSON over HTTP APIを提供することを推奨する。** `ServeMux`のmethod-aware patternと`Request.PathValue`を使用する。ハンドラーは、認証済みActorをリクエストコンテキストから取得し、入力をAPI契約のDTOへ復号して、ドメイン/アプリケーションサービスを呼び出す。認可、状態遷移、監査記録の判断はハンドラーではなくサーバー側のアプリケーション層で行う。

状態変更においては、認証失敗を401、権限不足を403、対象不在を404、入力不正を400、現在状態と両立しない操作または楽観的並行性競合を409として表現することを推奨する。失敗応答は契約で定義された機械可読なコードを含める。

## HTTP意味論

ここでいうHTTP意味論とは、URL・HTTP method・status・header・JSONエラーコードに、操作結果を一貫して割り当てることである。画面は文言の解析ではなく、この機械可読な結果に基づいて次の動作を選べる。詳細なschemaはADR-003のOpenAPI契約に記載する。

| 操作または結果 | HTTP表現 | UIが取るべき動作 |
| --- | --- | --- |
| Draftを新規作成 | `POST`が`201 Created`、`Location` headerと作成済みRequestを返す | 作成済みDraftを表示する |
| SubmitまたはApproveが成功 | 対象の状態と新しい`version`を`200 OK`で返す | 返却された状態・Audit Historyを表示する |
| 必須項目欠落などの入力不正 | `400 Bad Request`と`invalid_request`、field別エラー | 当該フォーム項目の修正を促す |
| 未認証 | `401 Unauthorized`と`authentication_required` | 認証開始画面へ遷移する |
| 認証済みだが権限不足 | `403 Forbidden`と`forbidden` | 操作を実行せず、権限不足を示す |
| 存在しない、または閲覧を許可しないRequest | `404 Not Found`と`request_not_found` | 対象が利用できないことを示す。権限の有無を漏らさない |
| 古い`version`でのSubmit/Approve、または現在状態で許可されない操作 | `409 Conflict`と`version_conflict`または`invalid_state` | 最新状態を再取得し、再操作が必要であることを示す |

状態変更は、クライアントが確認したRequest versionを送る。ネットワーク障害で結果が不明な場合、UIは同じ状態変更を自動再送せず、まずRequestを再取得して結果を確定する。`Idempotency-Key`による再送保証は初回Sliceの範囲では採用せず、必要になった時点で別ADRとして検討する。

## Rationale

このSliceは少数のリソースと状態変更操作だけを必要とするため、Go 1.22以降の`ServeMux`のmethod-aware patternとpath parameterで不足しない。これは`go-modern-guidelines`のHTTP指針とも一致する。標準ライブラリを採用すれば、追加フレームワークを正当化せずに、HTTP境界の認証、認可、エラー、競合の振る舞いを明文化できる。ドメインサービスをHTTPハンドラーから分離すれば、後にルーターを変更しても業務ルールと監査要件への影響を局所化できる。

## Consequences

### Positive

- HTTP層の本番依存関係を増やさない
- ハンドラーの統合テストを`httptest`で実施できる
- APIの認証・認可・競合エラーを一貫して扱える

### Negative / trade-offs

- JSON復号、入力検証、エラー変換、ルートパラメータ処理の規約をアプリケーション側で定義する必要がある
- ルート数や横断ミドルウェアが増えた場合、標準`ServeMux`だけでは記述性が下がる可能性がある

## Validation

- `httptest`によるHTTP統合テストで、認証なし、権限不足、入力不正、対象不在、競合、および成功応答のHTTP statusとエラーコードを検証する
- 同一Requestへの競合するSubmitまたはApproveが、決定論的に一方だけ成功し、失敗側が409になることを検証する
- ハンドラーを介さないドメインテストで、状態遷移と監査記録作成を検証する

## Revisit conditions

- APIのルート・ミドルウェアが増え、標準ライブラリによる実装が一貫性または保守性を損なう場合
- Go 1.22以上を使用できない実行環境が必要になった場合
- ストリーミング、双方向通信、またはRPCが要件になった場合
- 性能・可観測性要件の計測結果により、HTTP提供層の変更が合理的になった場合
