# ADR-008: HTTP APIのバージョニング伝達方式

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-003, FR-005, FR-007, FR-011, FR-012, FR-013, NFR-002, NFR-004
- Supersedes: none
- Superseded by: none

## Context

ADR-003はOpenAPI 3.1をHTTP API契約の正本として提案している。互換性のないAPI変更を将来導入する場合、サーバーがどのAPI versionを処理すべきかをクライアントが伝える方式を決める必要がある。初回Sliceでは単一のブラウザクライアントだけを提供するが、version方式はURL、キャッシュ、ルーティング、可観測性、OpenAPI文書の構成に横断的に影響する。

このADRはmajor API versionの伝達方式を決める。APIの廃止期限、個別フィールドの追加・非推奨、外部利用者向け公開ポリシーは、外部クライアントが必要になった時点で別途決定する。

## Decision drivers

- ブラウザ、開発者、運用者が要求するAPI major versionを容易に識別できること
- `ServeMux`のルーティング、ログ、proxy、キャッシュで特別なcontent negotiation設定を要しないこと
- OpenAPI文書、HTTP統合テスト、ブラウザクライアントが同じversionを明示できること
- 初回Sliceを複雑にせず、将来の非互換変更に明確な移行経路を残すこと

## Options considered

### Option A: URL path prefix

`/api/v1/requests`のように、API major versionをURL pathに含める。

**利点**
- URL、ログ、router、proxy設定だけでversionを可視化できる
- ブラウザ、curl、OpenAPI server URLで追加headerなしに利用できる
- CDNやHTTP cacheのkeyにversionが自然に含まれる

**欠点**
- URLにversionが現れ、非互換変更時は新しいroute群を維持する必要がある
- resource URLの安定性よりversionの明示を優先することになる

### Option B: `Accept` headerのmedia type versioning

`Accept: application/vnd.approval-flow.v1+json`のように、media type parameterまたはvendor media typeでversionを指定する。

**利点**
- resource URLをversion非依存に保てる
- HTTP content negotiationの概念に沿って表現できる

**欠点**
- ブラウザ、ログ、proxy、cacheで`Accept`と`Vary`を正しく扱う必要がある
- 単一ブラウザクライアントの初回Sliceには可視性と運用負荷の釣り合いが悪い

### Option C: custom request header

`API-Version: 1`のような独自request headerでversionを指定する。

**利点**
- URLを変更せずにversionを送れる
- header値は単純である

**欠点**
- HTTP標準のcontent negotiationと異なり、proxy、cache、CORS、ログへ独自対応が必要になる
- リクエストを単独で見てもversionを把握しにくく、OpenAPIやブラウザの利用規約が増える

## Decision

**URL path prefixでmajor versionを伝達し、初回Sliceは`/api/v1`を使用することを推奨する。** `v1`内では、既存の必須field・型・HTTP意味論を壊さない限り、後方互換なfieldやoperationの追加を許容する。後方互換でない変更は、新しい`/api/v{N}` route群と対応するOpenAPI文書を追加してから導入する。

## Rationale

URL prefixは、初回Sliceの単一ブラウザクライアントにとって最も明示的である。Go 1.22以降の`ServeMux` pattern、アクセスログ、ブラウザ開発ツール、OpenAPI server URLで特別なheader処理なしにversionが読める。header方式はURLを保てるが、初回Sliceに必要のない`Vary`、CORS、proxy、ログの横断設定を増やす。

## Consequences

### Positive

- API versionがURL、ルーティング、ログ、テスト、OpenAPI文書に一貫して現れる
- ブラウザクライアントは追加headerなしに対象versionを選べる
- 非互換変更を既存クライアントと並行して段階導入できる

### Negative / trade-offs

- 非互換versionを長期に併存させる場合、route・テスト・ドキュメントの保守対象が増える
- URLのversionを変更することは、クライアント側の明示的な移行を必要とする

## Validation

- OpenAPI文書のserver URLと全HTTP統合テストが`/api/v1`を使用することを確認する
- `v1`に後方互換なfieldを追加しても既存クライアントが動作することを契約テストで確認する
- 将来`v2`を追加する場合、同一Requestに対する`v1`と`v2`のroute・OpenAPI文書が混同されないことを確認する

## Revisit conditions

- 外部API利用者がURL安定性またはHTTP content negotiationを組織標準として要求する場合
- CDN、proxy、gatewayがURL prefix方式を許容せず、header方式の実測上の利点が運用負荷を上回る場合
- version単位ではなく、profileやcapability negotiationが必要になる場合
