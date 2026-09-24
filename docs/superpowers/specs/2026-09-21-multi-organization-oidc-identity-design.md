# 複数Organization対応OIDC Identity設計

更新日: 2026-09-24
Status: Accepted design — ADR-013承認済み、Task 5・6実装済み、Frontendとbrowser E2E検証済み
Related decision: `docs/decisions/architecture/ADR-013-oidc-identity-and-organization-membership.md`

「実装境界と順序」は設計時点の作業計画を記録したものです。現在の実装・検証結果は[Task 6完了記録](../../development/task6-completion-2026-09-22.md)と[Frontend Task 4完了記録](../../development/frontend-completion-2026-09-24.md)を参照してください。

## 目的と成功条件

同一の検証済みOIDC identity（`iss`と`sub`の組）が複数OrganizationのMemberへ所属できるようにする。ログイン後のapplication sessionは必ず1つの選択済みMemberに束縛し、OrganizationごとのRoleと既存Request認可を維持する。

成功条件は、複数所属identityが候補Organizationから安全にMemberを選択でき、候補外のMemberを指定してもsessionやActorを取得できないこととする。

## データモデル

新規migrationで以下を追加する。

| Table | 役割 | 主な制約 |
| --- | --- | --- |
| `oidc_identities` | 検証済みOIDC identity | `UNIQUE (issuer, subject)` |
| `member_oidc_identities` | identityとMemberの多対多対応 | 両列の外部キー、重複対応を禁止 |
| `organization_selection_transactions` | 複数所属時の短命・単回使用選択状態 | 不透明cookieのhash、identity、expiry、consumed marker |

`app_sessions.member_id`は変更しない。sessionを発行するのは、対応先が1件の場合またはOrganization選択成功後だけとする。

`oidc_auth_transactions`には`created_at timestamptz NOT NULL DEFAULT now()`を追加し、ADR-011との不整合を解消する。`members.oidc_subject`は既存migrationとの互換のため残すが、login時の照合には使用しない。

## 認証・選択フロー

1. `BeginLogin`は従来どおりOIDC transactionを作成する。
2. `CompleteLogin`はtransactionを消費し、issuer、audience、署名、有効期限、nonce、state、PKCE verifierを検証する。
3. 検証後の`iss`/`sub`で`oidc_identities`を検索し、関連するMemberを得る。
4. 候補が0件なら認証を失敗させる。1件なら新しいMember-bound sessionを発行する。複数件ならOrganization選択transactionだけを発行する。
5. 選択endpointは対応表に存在する候補Memberだけを受け入れる。成功時に選択transactionを消費し、新しいMember-bound sessionを発行する。

候補Member IDやOrganization IDはクライアントから届く未信頼入力であり、transactionの候補集合に対してサーバー側で照合する。OIDC role/group、クライアントActor指定、identityだけのsessionを認可に用いない。

## 失敗時の振る舞い

- callbackで識別できたOIDC transactionは、検証または対応付けの失敗時も消費する。
- identityが未provisioned、候補が0件、選択transactionが期限切れ・使用済み、候補外Memberの指定はsessionを発行しない。
- Organization選択transactionも一度の成功・失敗試行で再利用不能にする。
- token、PKCE verifier、cookie生値、CSRF token、identity subjectはJSON responseまたはlogへ含めない。

## 実装境界と順序（設計時点の計画）

1. 承認後、ADR-013をAcceptedにし、Task 5 hand-offと実装計画を更新する。
2. schema migrationとPostgreSQL repositoryを追加し、identity対応、選択transaction、`created_at`を実PostgreSQLでテストする。
3. `internal/auth`にOIDC callback分岐と選択transactionの境界を実装し、OIDC test doubleで検証する。
4. Task 6にOrganization選択endpointと画面遷移を追加する。既存session endpointおよびmiddlewareはMember-bound sessionだけを受け入れる。
5. 既存Memberを使う環境では、OIDC issuerを明示したidentity provisioningを行い、対応表を作成してからログインを有効化する。

## 検証

- 単一所属と複数所属の両方で、正しいMemberだけがsessionのActorになること。
- issuer、subject、state、nonce、PKCE verifier、選択cookie、候補外Member、replay、期限切れの拒否。
- `oidc_auth_transactions.created_at`を含むmigration up/down。
- logout、idle/absolute expiry、CSRF tokenの既存session要件を維持すること。
- `go vet ./...`、PostgreSQL integration test、`gofmt`、Biome format/lint、`git diff --check`。

## 非対象

- JIT provisioning、SCIM、identity管理UI、複数issuer同時利用、identity統合・削除
- Organization切替を既存session内で行う機能
- refresh token、Provider連携logout、Key rotation、複数region
