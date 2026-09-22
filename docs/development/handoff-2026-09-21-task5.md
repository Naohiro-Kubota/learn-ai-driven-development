# 引き継ぎ資料: Task 5 OIDC transaction と不透明 session

更新日: 2026-09-21  
前提PR: [#4](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/4)（merged）  
実装計画: `docs/superpowers/plans/2026-09-20-go-api-implementation.md`

## 開始ゲート

Task 5 を開始する前に、PR #4 が `develop` にマージ済みであることを確認する。Task 5 は Task 4 の PostgreSQL repository と migration を前提にするため、未マージの feature branch を起点に実装しない。

開始時には、以下を確認する。

- `gh auth status`
- `git fetch origin`
- PR #4 の merge commit `bf7bf3827edb2c207aeac529d44f51886beb731f` が `origin/develop` に含まれること
- `origin/develop` に `0f60f5a`（既定Approver migration）と `bf7bf38`（workflow repository）が存在すること
- `git status --short` が空であること

上記を満たした後、`origin/develop` から隔離 worktree と `codex/task-5-oidc-sessions` branch を作成する。

## 現在地

| Task | 状態 | 主な成果 | develop上のcommit |
| --- | --- | --- | --- |
| Task 0 | 完了・承認済み | Biome、`gofmt`検査、`go vet`方針 | `fc83455` |
| Task 1 | 完了・承認済み | runtime configuration、OIDC dependency固定 | `416a803` |
| Task 2 | 完了・承認済み | PostgreSQL schema、Compose、migration test | `e2bd22b` |
| Task 3 | 完了・承認済み | domain workflow と application service | `6a45c1c` |
| Task 4 | 完了・承認済み | workflow repository、schema補完、並行性 test | `0f60f5a`、`bf7bf38` |
| Task 5 | 完了・承認済み | OIDC identityとMember対応表、単回使用transaction、opaque session、Organization選択候補snapshot | [PR #7](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/7)、`5d20868` |

## 承認済みの主要 Decision

- PostgreSQL、`database/sql`、手書き SQL、`golang-migrate`を使用する（ADR-004）。新しいORM、query builder、test containerは追加しない。
- 認可はアプリケーションDBに基づきサーバー側で行う。OIDC claim、Keycloak role/group、クライアント指定 Actor を認可の正本にしない（ADR-005、ADR-009、ADR-010）。
- domain/application は標準 `testing`、永続化は実 PostgreSQL integration testで検証する（ADR-006）。
- KeycloakをOIDC Providerとし、`github.com/coreos/go-oidc/v3` v3.21.0と`golang.org/x/oauth2` v0.37.0を使う（ADR-009、ADR-010）。新規依存を追加しない。
- PostgreSQL保存の不透明な application session と単回使用 OIDC transaction を使う。browserには意味を持つ値やOIDC tokenを保存しない（ADR-011）。
- Goのformat/static analysisは`gofmt`と`go vet`を使う（ADR-012）。

## Task 4 が提供する境界

`internal/store/postgres.NewRepository(*sql.DB)` は `internal/application/requests.Repository` を実装済みである。Task 5 はこの interface を変更してはならない。

- Request/Approval/Audit Eventの公開IDは不透明なCSPRNG生成値である。
- Submit/Approveは条件付き更新、Approval変更、Audit追記を一つの PostgreSQL transaction として処理する。
- Organizationの既定Approverは`organizations.default_approver_member_id`に保存され、同一OrganizationのMemberへ制約される。
- `DefaultApprover`は設定済みMember IDと`approver` roleを独立して確認する。既定Approver未設定は`domain.ErrNotFound`、role欠落は`HasApproverRole=false`である。

Task 5 のsession/OIDC repositoryは、既存Request repositoryと同じ `internal/store/postgres` packageへ追加するが、Request workflowの振る舞いやmigration `000001`〜`000003`を変更してはならない。

## 既存の設定と schema

`internal/config.Config` は既に次を検証して提供する。

- `DatabaseURL`、`ListenAddress`、`AllowedOrigin`
- `OIDCIssuer`、`OIDCClientID`、`OIDCRedirectURI`
- `CookieSecure`
- 32 byte の `AuthTransactionKey`
- `SessionIdleTTL`、`SessionAbsoluteTTL`、`AuthTransactionTTL`

`migrations/000002_auth_sessions.up.sql` は既に次を作成済みである。

- `app_sessions`: cookie hash、Member ID、CSRF token hash、created/last-used、idle/absolute expiry、revocation
- `oidc_auth_transactions`: cookie hash、state hash、nonce、暗号化PKCE verifier、issuer/client/redirect、expiry、consumed marker

ADR-013のAccepted後、Task 5はmigration `000004`でOIDC identity、Member対応表、Organization選択transaction、および`oidc_auth_transactions.created_at`を追加する。migration `000001`〜`000003`は変更しない。既存Memberは、検証済みissuerを指定した明示的provisioningで`oidc_identities`と`member_oidc_identities`へ対応付けてからloginを有効化する。

## Task 5 の実装範囲

Task 5で作成・変更するファイルは次に限定する。

- `internal/auth/oidc.go`
- `internal/auth/oidc_test.go`
- `internal/auth/session.go`
- `internal/auth/session_test.go`
- `internal/store/postgres/sessions.go`
- `internal/store/postgres/sessions_test.go`
- `migrations/000004_oidc_identity_memberships.up.sql`
- `migrations/000004_oidc_identity_memberships.down.sql`

範囲外:

- HTTP router/handler、CSRF/origin middleware、frontend、Keycloak Compose/realm provision
- Request workflow、migration、OpenAPI契約の変更
- refresh token、Provider連携logout、複数region、key rotation、Admin workflow管理

複数Member候補のOrganization選択HTTP endpointと画面遷移はTask 6で追加する。Task 5は選択transactionの生成・候補snapshot・単回消費をauth/repository境界で実装する。

## 実装・セキュリティ要件

- OIDC discoveryとID Token検証には長寿命の`oidc.Provider`/verifierを使う。callbackではissuer、audience、署名、有効期限、nonce、`state`、PKCE verifierを検証する。
- authorization開始ごとに、CSPRNGでstate、nonce、PKCE verifier、transaction cookie、session cookie、CSRF tokenを生成する。
- PKCE verifierだけを`AuthTransactionKey`によるAES-GCMで暗号化して保存する。cookie値、state、CSRF tokenはSHA-256 hashだけをDBへ保存する。
- transaction cookieとsession cookieには不透明値だけを保存する。OIDC token、Member ID、role、PKCE verifier、CSRF tokenをcookie、URL、DBの平文、ログへ出力してはならない。
- callbackで特定できたtransactionは、成功・失敗を問わず再利用不能にする。検証済み`iss`/`sub`をMemberへ対応付けてから、新しいsessionを発行する。既存sessionを昇格・再利用しない。
- session lookupではhash、revocation、idle expiry、absolute expiryを確認する。logoutはsessionをrevokeする。
- production cookieは`__Host-approval_flow_session`、`Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/`、Domainなしとする。loopback developmentのみ異なるcookie名と非Secureを許可する。

## TDD と integration test

最初に `internal/auth` と `sessions_test.go` の test を追加し、未実装による失敗を確認する。OIDC protocolの検証は`httptest.Server`によるDiscovery/JWKS/token endpointのtest doubleと実 PostgreSQLを組み合わせる。SQLite、in-memory session store、任意headerによるActorなりすましは使わない。

最低限、以下を検証する。

- state、nonce、issuer、audience、署名、有効期限、PKCE verifierの不一致を拒否すること
- callback replayがtransactionを再利用できずsessionを発行しないこと
- verifierが暗号化され、cookie/state/CSRF tokenがhashだけで保存されること
- sessionのlogout、revocation、idle expiry、absolute expiry、CSRF token検証
- 生のtoken、verifier、cookie、CSRF tokenがJSONまたはlogへ出ないこと

## 検証コマンド

固定toolchainを使用する。

```bash
source /Users/nao/.nvm/nvm.sh
nvm use 26.9.0
pnpm install --frozen-lockfile
pnpm run test:db
GOTOOLCHAIN=go1.27.1 go vet ./...
pnpm run check:gofmt
pnpm run format:check
pnpm run lint
git diff --check
```

Task 5 の完了前に、`.superpowers/sdd/2026-09-20-go-api-implementation/progress.md` へ日本語で判断と実行記録を追記する。このdirectoryはgit管理対象外である。

## 後続Taskの前提・次のDecision

- loginを有効化する前に、検証済みissuerを指定したprovisioningで`oidc_identities`と`member_oidc_identities`を作成する。管理用provisioning workflowは後続Taskで扱う。
- `pnpm test` のNode標準`node:test`とVitestの収集競合は解消済みである。`scripts/check-gofmt.test.mjs`はVitestへ移行済みであり、2026-09-22に`pnpm test`が1 file・2 testsの成功を確認した。
- Task 6では、`GET /auth/oidc/login`、`GET /auth/oidc/callback`、Organization選択のGET/POST、`GET /api/v1/session`、`POST /api/v1/session/logout`を実装し、HTTP cookie発行、server-side session由来のActor、Origin/CSRF防御、および設定済みPostgreSQL/OIDCへのcomposition rootを追加した。`cmd/api`と`internal/httpapi`のテスト、および`go vet`、`gofmt`、Biome、`git diff --check`は成功した。Task 6を検証完了とするには、Dockerとloopback listenerを利用できる環境で`pnpm run test:db`、`go test ./cmd/api ./internal/auth ./internal/httpapi -count=1`、隔離した`TEST_DATABASE_URL`を指定した`go test ./internal/store/postgres -count=1`がすべて成功する必要がある。この実行環境ではDocker socketへのアクセスと`httptest.Server`のloopback bindが拒否されたため、これらの完了ゲートは未達であり、テストの代替・弱体化は行っていない。
