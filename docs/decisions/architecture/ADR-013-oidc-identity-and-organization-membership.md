# ADR-013: OIDC Identity と複数 Organization Member の対応

- Status: Proposed
- Date: 2026-09-21
- Owners: Human project owner
- Related requirements: FR-001, FR-002, FR-003, FR-005, FR-007, FR-012, FR-013, NFR-001, NFR-002, NFR-003, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

ADR-009、ADR-010、ADR-011は、検証済みのOIDC `iss` と `sub` の組をアプリケーションのMemberへ対応付け、Memberをsessionおよび認可のActorとすることを求める。

初期schemaの`members`は`oidc_subject`を持ち、`(organization_id, oidc_subject)`だけを一意にしている。この形ではissuerを表現できず、同じOIDC identityが複数Organizationに所属した場合にcallbackから一意のMemberを選べない。プロダクト要件として、同一identityの複数Organization所属をサポートする。

また、ADR-011が認証transactionに要求する`created_at`列は、既存の`oidc_auth_transactions`に存在しない。

## Decision drivers

- 検証済みの`(iss, sub)`を一意のidentityとして扱うこと
- 1 identityを複数OrganizationのMemberへ安全に対応付けること
- sessionのActorを常に選択済みMemberに限定し、identityだけのsessionを認可に使用しないこと
- クライアント指定のMember ID、Organization ID、OIDC role/groupを認可の根拠にしないこと
- PostgreSQL、`database/sql`、手書きSQL、既存のOIDC依存だけで検証可能にすること

## Options considered

### Option A: `members`にissuerを追加する

Memberへissuerとsubjectを直接保存する。

**利点**
- schema変更が小さい。

**欠点**
- 同一identityを複数Memberへ対応付けられない。
- identityとOrganization membershipを同一の概念として固定する。

### Option B: identityだけに紐づく未選択sessionを発行する

callback後にidentity sessionを発行し、後続requestでMemberを選択する。

**利点**
- callback直後にOrganizationを決めなくてよい。

**欠点**
- 未選択sessionを認可境界から確実に排除する追加規則が必要になる。
- ADR-011および既存`app_sessions.member_id`の、sessionがActor Memberを表す責務と一致しない。

### Option C: identityとMemberを分離し、選択済みMemberにだけsessionを発行する

OIDC identityを別tableに保存し、対応表を介して複数Memberへ関連付ける。callbackで複数Memberが得られた場合は、短命・単回使用の選択transactionを経てMemberを選択し、その後に通常sessionを発行する。

**利点**
- identity、Organization membership、Role、認可Actorを分離できる。
- sessionを常に一意のMemberへ束縛できる。
- 将来、1 Memberへ複数のOIDC identityを対応付ける拡張も対応表で表現できる。

**欠点**
- schema、repository、OIDC callback、Organization選択endpointに追加実装と統合testが必要になる。
- login完了前にOrganization選択画面またはendpointが必要になる。

## Decision

**Option Cを採用することを提案する。**

`oidc_identities`はCSPRNG生成の公開されないID、`issuer`、`subject`、`created_at`を保持し、`(issuer, subject)`を一意にする。`member_oidc_identities`は`identity_id`と`member_id`を外部キーで保持する対応表とし、同一identityを複数OrganizationのMemberへ関連付けられるようにする。

検証済みidentityに対応するMemberが1件なら、callbackはそのMemberへ束縛したapplication sessionを新規発行する。複数件なら、identity、候補Member、短いexpiry、消費時刻をサーバー側に保存する不透明なOrganization選択transactionを発行する。選択endpointはtransactionに含まれる候補だけを受け入れ、成功時にtransactionを消費して選択済みMemberへ束縛したsessionを新規発行する。identityだけへ束縛したsession、または未検証のクライアント入力からのActor決定は作らない。

既存の`members.oidc_subject`は既存migrationとの互換のため当面保持するが、OIDC loginの対応付けには使用しない。identity対応は明示的なprovisioningで作成する。既存Memberをログイン可能にする必要がある環境では、検証済みのissuerを指定した運用手順でidentityおよび対応表を作成してから、この認証経路を有効化する。

新しいmigrationで、上記2table、Organization選択transaction table、および`oidc_auth_transactions.created_at timestamptz NOT NULL DEFAULT now()`を追加する。既存migration `000001`〜`000003`は変更しない。

## Rationale

identityをMemberから分離すれば、OIDCの主体とOrganizationごとのRBACを別々の正本として保てる。選択transactionを用いることで、複数所属のidentityに対しても、認可済みAPI requestに渡るsessionは常に1つのMemberを指す。これはADR-005のサーバー側Actor決定とADR-011のserver-side sessionの責務を維持する。

## Consequences

### Positive

- `(iss, sub)`を正確に一意のOIDC identityとして保存できる。
- 同じidentityが複数Organizationで異なるRoleを持てる。
- application session、Audit Actor、Request認可は従来どおりMember IDを用いられる。
- issuer変更や複数OIDC providerへの将来拡張を、Member schemaへ再び埋め込まずに扱える。

### Negative / trade-offs

- Task 5のfile限定範囲と「migrationを追加・変更しない」という既存hand-offは成立しなくなるため、承認後にTask 5の計画とhand-offを更新する必要がある。
- Organization選択のHTTP/UIはTask 6へ追加する必要がある。
- identity provisioningと既存Memberの移行を運用手順として明文化する必要がある。

## Validation

- 同一`(issuer, subject)`を複数OrganizationのMemberへ対応付けられることを実PostgreSQLで確認する。
- 1件の対応ではcallbackがそのMemberのsessionを発行することを確認する。
- 複数対応では選択transactionなし、期限切れ、使用済み、候補外Memberの選択を拒否することを確認する。
- 選択成功後だけ、選択されたMemberのsessionが発行され、他OrganizationのMemberとして認可できないことを確認する。
- issuerまたはsubjectが異なるidentityを対応表経由で解決できないことを確認する。
- identity、transaction cookie、session cookie、CSRF token、PKCE verifier、OIDC tokenがcookieの意味内容、URL、平文DB列、JSON、logへ出ないことを確認する。

## Revisit conditions

- 組織切替を同一browser session内で継続的に扱う要求が追加された場合
- SCIM、JIT provisioning、外部IdP federation、複数issuer同時利用を導入する場合
- 1 Memberと複数identityのリンク、identityの無効化・統合・監査が必要になった場合
