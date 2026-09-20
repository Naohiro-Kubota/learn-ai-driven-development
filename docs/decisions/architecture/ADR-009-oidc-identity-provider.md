# ADR-009: 初回Vertical SliceのOIDC Identity Provider

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-001, FR-002, FR-003, FR-005, FR-007, FR-012, FR-013, NFR-001, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

ADR-005は、OIDC Authorization Code Flow with PKCEで認証し、アプリケーション側でOrganization内のRBACを評価することをAcceptedとしている。しかし、具体的なOIDC Identity Providerは未選定である。初回Sliceでは、複数Member（Requester、Approver、Admin）を認証してブラウザE2Eテストを再現可能に実行し、新しい開発者が必要なサービスをローカル起動できる必要がある。

Identity Providerは認証だけを担い、Organization、Memberのアプリケーション上の対応付け、Role、Requestへの認可、Audit EventのActorはアプリケーションDBを正本とする。Identity Providerのrealm/group/roleをアプリケーション認可の正本にはしない。パスワードポリシー、MFA、SCIM、外部IdP federation、組織SSO、productionの高可用性構成は初回Sliceの対象外とする。

## Decision drivers

- OIDC Discovery、JWKS、Authorization Code Flow with PKCEを標準的に提供できること
- ローカル開発とブラウザE2Eで、決定論的なテストユーザー・client・redirect URIを再現できること
- アプリケーションの認可モデルをIdentity Provider固有のrole/group設計へ結合させないこと
- 外部SaaSアカウントやネットワーク接続がなくても開発を継続できること
- セキュリティ更新、バックアップ、TLS、運用監視の責務と限界を明示できること

## Options considered

### Option A: 自己運用Keycloak

KeycloakをOIDC providerとして運用し、ローカル開発・E2E用に固定versionのcontainer imageとrealm importでclientおよびテストユーザーを再現する。初期運用環境では、開発用`start-dev`ではなくHTTPS、永続DB、明示的なhostnameを備えたproduction modeを使用する。

**利点**
- OIDC、OIDC client管理、realmのimport/export、管理コンソールを一つの自己管理可能なサービスで提供できる
- ローカル開発・E2Eは外部SaaSアカウントに依存せず、realm設定をバージョン管理できる
- OIDC標準境界により、将来別providerへ移行してもアプリケーションの認可モデルを維持できる

**欠点**
- Keycloakのcontainer image、DB、TLS、backup、脆弱性更新、管理者credentialを運用する責務が生じる
- JVMベースのサービスであり、初回Sliceにも追加のメモリ・起動時間が必要になる
- realm管理者権限を厳格に分離しないと、認証基盤への影響が大きい

### Option B: Managed Auth0

Auth0 tenantにRegular Web Applicationを登録し、Universal LoginとAuthorization Code Flowを使用する。

**利点**
- Identity Providerの可用性、更新、login UI、MFAなどをSaaSへ委譲できる
- 標準OIDCに基づき、アプリケーションはissuer/JWKSを使った検証を行える

**欠点**
- 開発・E2Eの再現に外部アカウント、tenant設定、ネットワーク接続が必要になる
- 料金、利用上限、tenant設定、ベンダー運用の変更に影響される
- 初回Sliceの実験環境に、外部サービスのcredentialと運用管理を持ち込む

### Option C: ZITADEL

ZITADEL Cloudまたは自己運用ZITADELをOIDC providerとして使用する。

**利点**
- OIDC、SAML、MFA、組織管理などを提供し、将来のIdentity機能拡張に対応できる
- Cloud利用と自己運用の選択肢がある

**欠点**
- 自己運用はproxy、PostgreSQL、login UIなど複数コンポーネントを必要とし、初回Sliceには運用構成が重い
- Cloud利用時はAuth0と同様に外部サービスへの依存が生じる
- 初回Sliceに不要な組織・IAM機能の設定判断が増える

## Decision

**Keycloakを初回SliceのOIDC Identity Providerとして採用することを推奨する。** OIDC issuer、client ID、redirect URI、logout redirect URI、テストユーザーの定義は、開発環境用realm設定としてリポジトリで再現可能に管理する。テストユーザーのcredentialはローカルまたはCIのsecret入力からprovisioningし、本番credential、client secret、管理者credentialをリポジトリへ保存しない。Keycloak container imageはtagではなく正確なversionとdigestで固定する。

ローカル開発とE2Eでは、loopback interfaceだけに公開したKeycloak development modeを使用してよい。realm importは開発・テスト環境でのみ使用し、Admin credentialをリポジトリへ保存しない。初期運用環境ではdevelopment modeを使用せず、HTTPS、明示的hostname、永続的なKeycloak専用PostgreSQL database、backup、管理者credentialのsecret管理を必須とする。

アプリケーションはOIDC Discovery documentとJWKSを用い、issuer、audience、署名、有効期限、nonce、PKCE verifier、`state`を検証する。認証成功後、検証済みの`iss`と`sub`の組をアプリケーションのMemberへ対応付け、アプリケーションDB内のOrganization/Roleで認可する。Keycloak realm role/groupをRequest操作の認可判断に使用しない。

## Rationale

Keycloakは、ADR-005で決めた標準OIDC境界を満たしながら、ローカル開発とE2Eの外部サービス依存をなくせる。realmのimport/exportと管理コンソールにより、初回Sliceに必要なテストユーザーとclient設定を再現できる。Keycloakのdevelopment modeには安全でない既定値があるためローカル用途に限定し、初期運用ではproduction modeと永続DBを必須とする。Identity Providerからアプリケーションの認可を分離するため、将来のprovider変更もOrganization/Role/Requestの業務ルールを変更せずに実施できる。

## Consequences

### Positive

- OIDC認証をローカル、E2E、初期運用環境で同じ標準プロトコルにより検証できる
- テスト用realmとユーザーを再現可能に管理できる
- アプリケーション固有のRoleとRequest認可をKeycloak固有の設定から分離できる

### Negative / trade-offs

- Keycloakと専用DBの起動・保守・backup・脆弱性対応が必要になる
- Keycloak image、realm設定、client secret、管理者credentialのversion・secret管理を整備する必要がある
- OIDC clientおよびID token検証に使用するGoライブラリは、このADRでは選定していない。実装前に依存関係ポリシーに従って別途評価する必要がある

## Validation

- 新しい開発者が文書化されたコマンドでKeycloak、realm、テストユーザーを起動・importできることを確認する
- Requester、Approver、Adminの各テストユーザーがAuthorization Code Flow with PKCEを完了し、アプリケーションsessionを取得できることをブラウザE2Eで確認する
- 不正なissuer、audience、署名、期限、nonce、`state`、PKCE verifierを持つcallback/tokenが拒否されることを統合テストで確認する
- Keycloak上のrole/groupを変更しても、アプリケーションDBで許可されないRequest操作は許可されないことを確認する
- 初期運用環境がdevelopment modeを使わず、HTTPS、永続DB、secret管理、backupを備えることを運用チェックで確認する

## Revisit conditions

- 組織の既存IdP、SAML、SCIM、MFA、外部IdP federation、または組織SSOの要求が追加された場合
- Keycloakの運用負荷、resource消費、更新対応が初回Slice以降の運用能力を上回る場合
- Managed Identity Providerのセキュリティ・可用性・費用条件が、自己運用より明確に優位になった場合
- アプリケーション認可を属性ベースまたはポリシーエンジンへ移行する必要がある場合
