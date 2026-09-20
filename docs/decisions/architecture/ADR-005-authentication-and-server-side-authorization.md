# ADR-005: 最初のVertical Sliceの認証とサーバー側認可

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-001, FR-002, FR-003, FR-005, FR-007, FR-012, FR-013, NFR-001, NFR-003, NFR-004
- Supersedes: none
- Superseded by: none

## Context

初回Sliceでは、RequesterとApproverを区別し、誰がDraftを作成・Submit・ApproveしたかをAudit Historyに記録する必要がある。NFR-001はサーバー側認可を必須としている。一方、詳細な権限ルール、認証プロバイダ、組織横断の委任・代理承認は未決定である。

このADRは、ブラウザから認証済みActorを得る方式と、初回Sliceに必要な最小RBACを決める。パスワード管理、SCIM、MFA、組織向けSSOのプロバイダ選定、代理承認のポリシーは対象外とする。

## Decision drivers

- ブラウザベースのWebアプリケーションで、認証済みMemberを信頼できる形で特定できること
- RequesterとApproverの操作権限をサーバー側で必ず判定できること
- 監査記録に、検証済みのActorと操作時刻を結び付けられること
- 認証プロバイダを早期に固定せず、資格情報をアプリケーションで保管しないこと
- 開発・テストで複数Actorの権限を再現できること

## Options considered

### Option A: OIDC Authorization Code Flow with PKCEとアプリケーションRBAC

ブラウザはOIDC準拠のIdentity Providerへ認証を委譲する。バックエンドは発行者、署名、対象者、有効期限を検証してsubjectをMemberへ対応付け、Organization内のroleに基づき認可する。

**利点**
- パスワードや多要素認証をアプリケーションで扱わない
- 標準プロトコルにより、将来のIdentity Provider変更の余地を残せる
- 検証済みのsubjectをAudit EventのActorとして記録できる

**欠点**
- ローカル開発・E2EテストでOIDC準拠のテスト用Identity Providerまたは同等のテストダブルが必要になる
- トークン検証、セッション、redirect URI、cookie保護を正しく実装・運用する必要がある

### Option B: アプリケーション内ユーザー名/パスワード認証とセッション

アプリケーションが認証情報を保存し、パスワード検証後にセッションcookieを発行する。

**利点**
- 外部Identity Providerなしに初回Sliceを実行できる
- ユーザーとroleのテストデータを直接制御できる

**欠点**
- 資格情報の保管、リセット、MFA、侵害対応をアプリケーションが担う
- 初回Sliceの目的に対してセキュリティ・運用上の責務が大きい

### Option C: リクエストheaderでActorを指定する開発用なりすまし

ブラウザまたはテストが任意のheaderでMember IDとroleを送信し、バックエンドがそれを信頼する。

**利点**
- 複数Actorのローカル検証を迅速に開始できる
- 認証基盤の準備を後回しにできる

**欠点**
- クライアントがActorを偽装でき、NFR-001を満たさない
- 誤って本番相当環境に残った場合の影響が重大である

## Decision

**OIDC Authorization Code Flow with PKCEで認証し、アプリケーションがOrganization内の最小RBACをサーバー側で評価することを推奨する。** 特定のIdentity ProviderはこのADRでは選定しない。バックエンドは、設定されたOIDC issuerのdiscovery metadataとJWKSを用いてID tokenまたはaccess tokenを検証し、検証済みsubjectをMemberへ対応付ける。ブラウザにはアプリケーションが発行する`Secure`、`HttpOnly`、`SameSite`属性付きセッションcookieのみを保持させ、OIDC tokenをJavaScriptから公開しない。

初回Sliceの認可規則は以下に限定する。

- Requesterは自分のDraftを作成し、自分が作成したDraftをSubmitできる
- Approverは自分に割り当てられたPending RequestだけをApproveできる
- Requesterは自分のRequestとそのAudit Historyを閲覧できる
- Approverは自分に割り当てられたPending RequestとそのAudit Historyを閲覧できる
- Adminはテスト・運用支援のために組織内Requestを閲覧できるが、初回Sliceでワークフロー設定機能は実装しない

すべての状態変更は、クライアント入力ではなく認証済みActorから監査Actorを決定する。ローカル開発と自動テストには、本番経路と分離されたOIDC準拠のテスト用Identity Providerまたは検証可能なテストダブルを使用する。任意headerを信頼するなりすまし機構は採用しない。

## Rationale

OIDCは認証の難しい責務を標準的なIdentity Providerへ委譲し、アプリケーションはプロダクト固有のOrganization/Role/Request権限に集中できる。セッションcookieを採用すれば、ブラウザJavaScriptにbearer tokenを持たせずにAPI呼び出しを行える。最小RBACをサーバー側に置くことで、UIの表示制御を回避されてもApprove等の操作を防止でき、監査Actorの信頼性を保てる。

## Consequences

### Positive

- アプリケーションはユーザーパスワードを保管しない
- 認可と監査Actorの決定をサーバー側に一元化できる
- 特定ベンダーに固定せず、OIDC互換のIdentity Providerを選べる

### Negative / trade-offs

- OIDC連携、セッション管理、CSRF対策、logoutの実装・テストが必要になる
- 初回Sliceでも開発・テスト用Identity Providerの準備が必要になる
- roleとApproval Stepの対応は初回Sliceの最小規則に限定され、将来の詳細ポリシーには拡張が必要になる

## Validation

- 認証なしのAPI呼び出しが401となり、他MemberのRequest操作や未割当RequestのApproveが403となることを検証する
- RequesterとApproverの複数Actorで、許可・拒否される全操作をHTTP統合テストとブラウザE2Eテストで確認する
- Audit Eventがクライアント指定値ではなく、検証済みsessionのMemberをActorとして記録することを確認する
- session cookieの`Secure`、`HttpOnly`、`SameSite`属性と、状態変更に対するCSRF防御を検証する

## Revisit conditions

- SAML、社内IdP、SCIM、MFA、またはプロビジョニング要件が追加された場合
- 代理承認、職務分掌、属性ベース認可、複数Organization横断権限が必要になった場合
- セッションcookie方式がモバイル/外部クライアント要件を満たさなくなった場合
