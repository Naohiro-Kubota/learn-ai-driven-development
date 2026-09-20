# ADR-010: OIDC Authorization Code FlowとID Token検証のGoライブラリ

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-001, FR-002, FR-003, FR-005, FR-007, FR-013, NFR-001, NFR-003, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

ADR-005は、OIDC Authorization Code Flow with PKCEとサーバー側RBACを採用し、ブラウザにはアプリケーション発行のsession cookieだけを保持させる。ADR-009はOIDC Identity ProviderとしてKeycloakを採用し、Discovery documentとJWKSを用いたissuer、audience、署名、有効期限、nonce、PKCE verifier、`state`の検証を要求している。しかし、GoアプリケーションでAuthorization Code交換とID Token検証を行うライブラリは未選定である。

この選定は認証境界の正しさ、Keycloak以外の標準OIDC Providerへの移行可能性、JWKS取得・鍵ローテーション時の振る舞い、テスト容易性に影響する。sessionの永続化方式、OIDC callbackの一時状態をどこへ保存するか、CSRF防御の詳細、logoutの詳細、Keycloak realmのprovisioningは本ADRの対象外とする。

## Decision drivers

- OIDC Discovery、JWKSに基づくID Token署名検証、issuer、audience、有効期限、nonce検証を標準に従って実装できること
- Authorization Code Flowの`state`、PKCE S256 challenge、code verifierによる交換を明示的に実装できること
- Keycloak固有のrole/groupや管理APIへアプリケーションの認可を結合しないこと
- JWKSの鍵ローテーションを各リクエストごとに独自実装せず、安全に扱えること
- `httptest`とテスト用OIDC Providerで検証しやすく、Go 1.27.1と互換であること
- 初回Sliceに不要なOIDC Provider実装、管理API、セッション管理を持ち込まないこと

## Options considered

### Option A: `github.com/coreos/go-oidc/v3`と`golang.org/x/oauth2`

`go-oidc`をOIDC Discovery、JWKS取得、ID Token検証とclaims復号に使用し、`x/oauth2`をAuthorization URL生成とAuthorization Code交換に使用する。

**利点**

- OIDC clientとして必要な範囲に責務を絞り、Keycloak固有APIへ依存しない
- `Provider.VerifierContext`はDiscoveryで得たJWKSを利用し、長寿命のKeySetとして鍵を再利用・未知のkey ID時に再取得できる
- `x/oauth2`はPKCEの`S256ChallengeOption`、`VerifierOption`、ランダムverifier生成APIを提供する
- 標準的な`context.Context`、`http.Client`、`httptest`と組み合わせられる

**欠点**

- `state`、nonce、PKCE verifierの生成・短期保存・callback照合、アプリケーションsession、CSRF防御はアプリケーションが明示的に実装する必要がある
- 2つのmoduleのversion、脆弱性、間接依存を保守する必要がある

### Option B: `github.com/zitadel/oidc/v3`

OIDC clientとprovider実装を含む包括的なOIDC libraryを使用する。

**利点**

- OIDC clientの実装例と広いプロトコル機能を一つのecosystemから利用できる
- OIDC Provider実装を必要とする将来の用途にも拡張余地がある

**欠点**

- 本アプリケーションはKeycloakをProviderとして採用済みであり、Provider実装を含む広い機能は初回Sliceに不要である
- 初回Sliceに対してAPI面積、依存関係、チームが理解すべき規約が大きい
- `coreos/go-oidc`と`x/oauth2`よりも、client実装の責務境界を狭く保ちにくい

### Option C: Go標準ライブラリと手書きのOIDC/JWT処理

HTTP、JSON、`crypto`を組み合わせ、Discovery、JWKSキャッシュ、JWT検証、Authorization Code交換をアプリケーションで実装する。

**利点**

- 追加の本番依存関係を導入しない
- プロトコル上の全ての処理をアプリケーションで明示できる

**欠点**

- JWT algorithm制約、鍵選択とローテーション、issuer/audience/nonce/expiry検証を安全に実装・レビューする負担が大きい
- 仕様準拠とセキュリティ修正の保守責務を不必要にアプリケーションへ持ち込む
- 初回Sliceの目的に対してテスト量と実装リスクが大きい

## Decision

**`github.com/coreos/go-oidc/v3` v3.21.0と`golang.org/x/oauth2` v0.37.0を採用することを推奨する。** 承認後に両方をGo moduleの直接依存として正確なversionで固定し、`go.sum`を含めてレビューする。

アプリケーションはOIDC Discoveryで得たProviderを初期化し、issuerとclient IDを設定したverifierでID Tokenを検証する。JWKS KeySetはプロセス内で再利用し、リクエストごとに新規作成しない。Authorization開始時には暗号学的にランダムな`state`、nonce、PKCE verifierを生成し、Authorization URLには`state`、`oidc.Nonce`、`oauth2.S256ChallengeOption`を渡す。callbackでは、戻された`state`を照合してから、`oauth2.VerifierOption`を用いてcodeを交換し、ID Tokenの署名、issuer、audience、有効期限、nonceを検証する。

検証済みの`iss`と`sub`だけをMember対応付けの入力として使用する。Keycloak realm role/group、クライアントから送られたActor ID、未検証のJWT claimsを、Request認可またはAudit EventのActor決定に使用してはならない。

## Rationale

`coreos/go-oidc`はOIDC DiscoveryとID Token verifierに特化し、`x/oauth2`はAuthorization Code FlowとPKCEに必要なAPIを提供する。この組み合わせは、ADR-005/009が要求する標準OIDC境界を満たしながら、Keycloak管理API、Provider実装、JWT処理の自前実装を持ち込まない。ライブラリは認証トランザクションやアプリケーションsessionを隠蔽しないため、cookieの属性、CSRF防御、認可、監査Actorの決定をアプリケーション側でレビュー可能な責務として保てる。

## Consequences

### Positive

- ID Tokenの署名、issuer、audience、有効期限、nonceをOIDC向けverifierで一貫して検証できる
- Keycloakから他の準拠OIDC Providerへ移行しても、アプリケーションの認可モデルを維持できる
- JWKSの再利用と未知のkey IDへの再取得をライブラリに委ねられる
- OIDCプロトコル境界とOrganization/Role/Requestの業務認可を分離できる

### Negative / trade-offs

- 2つのGo moduleとその間接依存を、更新・脆弱性対応の対象として保守する必要がある
- `state`、nonce、PKCE verifier、session、CSRF、logoutの安全なライフサイクルは自動化されない
- OIDC callback一時状態とアプリケーションsessionの保存方式は、実装開始前に別途設計・承認が必要である

## Validation

- テスト用OIDC Providerに対し、DiscoveryからAuthorization Code Flow with PKCEを完了し、検証済みID Tokenから`iss`と`sub`を取得できることを確認する
- 不正なissuer、audience、署名、期限、nonce、`state`、PKCE verifier、またはcallback errorを拒否する統合テストを作成する
- JWKSのkey rotation後、新しいkey IDのID Tokenを検証できることをテスト用ProviderまたはHTTP test serverで確認する
- Keycloak role/groupまたはクライアント入力だけでは、アプリケーションDBが許可しないRequest操作を実行できないことを確認する
- `go mod verify`で依存チェックサムを検証し、HTTP認証フローのテストではtokenやverifierをログへ出力しないことを確認する

## Revisit conditions

- Provider主導logout、back-channel logout、PAR、JAR、DPoP、FAPIなど、`go-oidc`と`x/oauth2`の明示的な実装を超えるOIDC要件が追加された場合
- OIDC callback状態またはsessionの保存方式により、ライブラリ統合方式を根本から見直す必要が生じた場合
- Keycloak以外のProviderとの相互運用テストで、標準準拠の問題を再現・解決できない場合
- セキュリティ修正、Go toolchain互換性、または依存更新が継続的な保守コストとして過大になった場合
