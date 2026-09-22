# Go API実装計画

> **エージェント実行者向け:** この計画は、`superpowers:subagent-driven-development`（推奨）または`superpowers:executing-plans`を使い、タスク単位で実行すること。進捗はチェックボックス（`- [ ]`）で管理する。

**目的:** Accepted Decisionで選定済みの依存関係だけを用い、最初のVertical Sliceにおける認証済みGo HTTP APIのOpenAPI 3.1契約を実装する。

**アーキテクチャ:** Goの`net/http` adapterは処理をアプリケーションサービスへ委譲し、サービスがrepository interfaceを通じてワークフローと認可規則を適用する。PostgreSQL repositoryは、Request・Approval・Audit Event・OIDC認証transaction・不透明なsessionを原子的に処理する。OIDCおよびsession middlewareがhandler実行前に認証済みActorを確立する。handlerの責務はDTOのdecode、unsafe methodのCSRF防御、型付きapplication errorからOpenAPI error modelへの変換だけとする。

**技術スタック:** Go 1.27.1、`net/http`、`database/sql`、`httptest`、標準`crypto` package、pgx stdlib v5.11.0を用いるPostgreSQL、golang-migrate v4.20.1、`github.com/coreos/go-oidc/v3` v3.21.0、`golang.org/x/oauth2` v0.37.0、Biome v2.5.14、`gofmt`、`go vet`、`api/openapi.yaml`のOpenAPI 3.1契約。

**仕様:** `docs/superpowers/specs/2026-09-20-api-contract-design.md`

## 共通制約

- `api/openapi.yaml`に定義したoperationとschemaだけを実装する。Reject、Cancel、通知、ワークフロー管理、frontend codeは追加しない。
- Go 1.27.1とADR-004/ADR-010で選定した正確なmodule versionを使用する。runtimeのORM、router、session、JWT、test-container依存を追加しない。Task 8の正確な`yaml`開発依存だけを契約parserの例外とする。
- ADR-002に従い、Go 1.22以上の`ServeMux`のmethod-aware patternと`Request.PathValue`を使用する。
- domainの認可、状態遷移、version比較、監査記録をHTTP handlerへ置かない。
- browserにはCSPRNGで生成した不透明なcookie値だけを保存し、PostgreSQLにはcookie値のhashを保存する。OIDC token、PKCE verifier、state、nonce、session cookie、CSRF tokenをlogへ出力しない。
- 全unsafe `/api/v1` operationでsessionに束縛した`X-CSRF-Token`と許可された`Origin`を必須とする。`SameSite`をCSRF防御として十分とは扱わない。
- SubmitとApproveでは、一つのPostgreSQL transactionでRequest version更新とAudit Event追記を行う。古いversionには`409 version_conflict`を返す。
- PostgreSQL integration testは事前に用意した`TEST_DATABASE_URL`を使用する。transaction/concurrency coverageでSQLiteやin-memory DBへ代替しない。
- ADR-012に従い、TypeScript/TSX/JavaScript/JSONはBiome v2.5.14でformat/lintし、Go sourceは`gofmt`と`go vet ./...`で検査する。`tsc -b`、format/lint、`go vet`、testは相互の代替と扱わない。
- Formatterはローカルでのみファイルを書き換えてよい。CIのformat/lint検査はworking treeを書き換えず、未整形または診断がある場合に失敗しなければならない。
- Biomeは正確な直接`devDependency`として、`package.json`、`pnpm-workspace.yaml`、`pnpm-lock.yaml`を同一変更でレビューして追加する。ADR-007で許可されていないbuild/install scriptを要求する場合は、この計画を停止して人間のレビューを求める。
- 既存の未commit toolchain・Decision document変更は、人間のownerが明示的に含めない限り、この計画のcommit対象にしない。

## レビュー重点項目

- 空白だけのTitleはtrim後に拒否する。Descriptionは空を許容し、Audit snapshotで空であることを確認できなければならない。
- 同一`expectedVersion`を持つ同時SubmitまたはApproveは、必ず1件だけ成功し、他方は`409 version_conflict`となる。Audit Eventを重複記録してはならない。
- Requesterは他RequesterのDraftを更新・Submitできず、未割当Approverは既知のPending RequestをApproveできない。いずれの変更操作も`403 forbidden`を返す。
- OIDC callbackの再送、state・nonce・transaction cookie・PKCE verifierの不一致はtransactionを消費または拒否し、sessionを発行してはならない。
- 有効なsessionでも、CSRF tokenの欠落・不一致、または許可されないOriginではCreate、Update、Submit、Approve、logoutできず、`403 csrf_validation_failed`を返す。

---

### Task 0: Formatter/Linterの品質ゲートを追加する

**ファイル:**
- 作成: `biome.json`
- 作成: `scripts/check-gofmt.mjs`
- 作成: `scripts/check-gofmt.test.mjs`
- 変更: `package.json`
- 変更: `pnpm-workspace.yaml`
- 変更: `pnpm-lock.yaml`
- 変更: `docs/development/toolchain.md`

**Interface:**
- 提供: `pnpm run format`（ローカルでBiomeの書式を修正）、`pnpm run format:check`（非破壊の書式検査）、`pnpm run lint`（Biome lint）、`pnpm run check:gofmt`（tracked Go sourceの非破壊`gofmt`検査）。
- 提供: `scripts/check-gofmt.mjs`からimport可能な`findUnformattedFiles(files, runGofmt)`。
- 利用元: 以降の全Task、Task 8の完全verification suite、CI。

- [x] **Step 1: 失敗する`gofmt`検査testを書く**

`node:test`で、整形済みのGo sourceには空配列を返し、未整形のGo sourceにはそのfile pathを返すことをtestする。testは実際の`gofmt -l`を呼ばず、`runGofmt` test doubleを渡してexit statusと標準出力の処理を固定する。

```js
import test from "node:test";
import assert from "node:assert/strict";
import { findUnformattedFiles } from "./check-gofmt.mjs";

test("reports paths printed by gofmt -l", () => {
  const result = findUnformattedFiles(["internal/example.go"], () => "internal/example.go\n");
  assert.deepEqual(result, ["internal/example.go"]);
});
```

- [x] **Step 2: focused testを実行して失敗を確認する**

Run: `node --test scripts/check-gofmt.test.mjs`

期待結果: `scripts/check-gofmt.mjs`と`findUnformattedFiles`が存在しないためFAIL。

- [x] **Step 3: Biome設定、Go検査script、package scriptを実装する**

`@biomejs/biome` **2.5.14**を正確な`devDependency`として追加する。`biome.json`ではformatterとrecommended linterを有効にし、`node_modules`、`dist`、`coverage`、`playwright-report`、`test-results`を検査対象外にする。import整理やプロダクト固有ruleを暗黙に有効化しない。

`scripts/check-gofmt.mjs`は、`git ls-files -z -- '*.go'`でtracked Go sourceだけを取得し、空の対象集合では成功する。対象がある場合は`gofmt -l`を実行し、出力pathを返す`findUnformattedFiles`を介して、1件以上ならpathを標準errorへ出力してexit 1、空ならexit 0とする。formatter自体は実行しない。

`package.json`へ以下のscriptを追加する。

```json
{
  "format": "biome format --write .",
  "format:check": "biome format .",
  "lint": "biome lint .",
  "check:gofmt": "node scripts/check-gofmt.mjs",
  "check": "pnpm run format:check && pnpm run lint && pnpm run typecheck && pnpm run check:gofmt"
}
```

`pnpm install --lockfile-only --ignore-scripts`でlockfileを更新する前に、ADR-007の`minimumReleaseAge`、`strictDepBuilds`、空の`allowBuilds`を確認する。Biomeまたはtransitive dependencyがbuild/install scriptの許可を要求した場合は、`allowBuilds`を編集せずに作業を止めて人間へ報告する。`docs/development/toolchain.md`にはBiome 2.5.14とADR-012を記録し、OIDC Go libraryの記載をAccepted ADR-010と矛盾しない内容へ更新する。

- [x] **Step 4: Formatter/Linterのfocused verificationを実行する**

Run:

```bash
node --test scripts/check-gofmt.test.mjs
pnpm install --frozen-lockfile
pnpm run format:check
pnpm run lint
pnpm run check:gofmt
GOTOOLCHAIN=go1.27.1 go vet ./...
```

期待結果: すべてPASS。`pnpm install`はlockfileを書き換えず、各検査はworking treeを書き換えない。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add biome.json scripts/check-gofmt.mjs scripts/check-gofmt.test.mjs package.json pnpm-workspace.yaml pnpm-lock.yaml docs/development/toolchain.md
git commit -m "build: add TypeScript and Go quality checks"
```

### Task 1: OIDC moduleと検証済みruntime configurationを追加する

**ファイル:**
- 変更: `go.mod`
- 変更: `go.sum`
- 作成: `internal/config/config.go`
- 作成: `internal/config/config_test.go`

**Interface:**
- 提供: `config.Load(lookup func(string) string) (config.Config, error)`。
- 提供: database URL、listener address、許可origin、OIDC issuer/client ID/redirect URI、cookie/session secret設定を含む`config.Config`。
- 利用元: `cmd/api/main.go`、OIDC client構築、session middleware、PostgreSQL設定。

- [x] **Step 1: 失敗するconfiguration testを書く**

```go
func TestLoadRejectsProductionWithoutSecureCookie(t *testing.T) {
	_, err := Load(func(key string) string {
		return map[string]string{
			"APP_ENV": "production", "APP_COOKIE_SECURE": "false",
		}[key]
	})
	if err == nil { t.Fatal("expected validation error") }
}

func TestLoadAllowsInsecureCookieOnlyForLoopbackDevelopment(t *testing.T) {
	values := map[string]string{
		"APP_ENV": "development", "APP_COOKIE_SECURE": "false",
		"APP_LISTEN_ADDR": "127.0.0.1:8080", "APP_ALLOWED_ORIGIN": "http://127.0.0.1:5173",
		"DATABASE_URL": "postgres://test", "OIDC_ISSUER": "http://127.0.0.1:8081/realms/dev",
		"OIDC_CLIENT_ID": "approval-flow", "OIDC_REDIRECT_URI": "http://127.0.0.1:8080/auth/oidc/callback",
		"AUTH_TRANSACTION_KEY": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
		"SESSION_IDLE_TTL": "15m", "SESSION_ABSOLUTE_TTL": "8h", "AUTH_TRANSACTION_TTL": "5m",
	}
	cfg, err := Load(func(key string) string { return values[key] })
	if err != nil { t.Fatalf("Load() error = %v", err) }
	if cfg.CookieSecure { t.Fatal("CookieSecure = true") }
}
```

- [x] **Step 2: focused testを実行して失敗を確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/config -run TestLoad -count=1`

期待結果: packageと`Load`が存在しないためFAIL。

- [x] **Step 3: 正確なOIDC依存とconfiguration validationを追加する**

`github.com/coreos/go-oidc/v3 v3.21.0`と`golang.org/x/oauth2 v0.37.0`を直接requireへ追加する。必須の非空configuration値と正のdurationを検証する`Config`を実装する。base64 decode後32 byteとなるauth transaction暗号化鍵を必須にする。`APP_COOKIE_SECURE=false`は、`APP_ENV=development`かつapplication/allowed-originのhostがともにloopbackの場合だけ許可し、それ以外は起動時に失敗させる。

```go
type Config struct {
	DatabaseURL, ListenAddress, AllowedOrigin string
	OIDCIssuer, OIDCClientID, OIDCRedirectURI string
	CookieSecure bool
	AuthTransactionKey [32]byte
	SessionIdleTTL, SessionAbsoluteTTL, AuthTransactionTTL time.Duration
}
```

- [x] **Step 4: focused testとmodule integrity checkを実行する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/config -count=1 && GOTOOLCHAIN=go1.27.1 go mod verify`

期待結果: PASS。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add go.mod go.sum internal/config/config.go internal/config/config_test.go
git commit -m "feat: add validated API runtime configuration"
```

### Task 2: workflow、audit、session、OIDC transaction用のversion管理PostgreSQL schemaを作成する

**ファイル:**
- 作成: `migrations/000001_initial_workflow.up.sql`
- 作成: `migrations/000001_initial_workflow.down.sql`
- 作成: `migrations/000002_auth_sessions.up.sql`
- 作成: `migrations/000002_auth_sessions.down.sql`
- 作成: `internal/store/postgres/migrations_test.go`

**Interface:**
- 提供: Organization、Member、MemberRole、Request、Approval、AuditEvent、`app_sessions`、`oidc_auth_transactions`のtable。
- 提供: 後続repository methodに必要なunique constraintとforeign key constraint。
- 利用元: 全PostgreSQL repositoryとintegration test。

- [x] **Step 1: 失敗するmigration integration testを書く**

`TEST_DATABASE_URL`から新しいschema/databaseを作成し、golang-migrateで全`up` migrationを適用して、必要なtableとconstraintの存在を確認する。逆順に`down` migrationを適用してschemaが空になることを確認するtestも追加する。

```go
func TestMigrationsCreateWorkflowAndAuthTables(t *testing.T) {
	db := openFreshTestDatabase(t)
	applyUpMigrations(t, db)
	for _, table := range []string{"requests", "approvals", "audit_events", "app_sessions", "oidc_auth_transactions"} {
		assertTableExists(t, db, table)
	}
}
```

- [x] **Step 2: migration testを実行して失敗を確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestMigrationsCreateWorkflowAndAuthTables -count=1` (with an isolated `TEST_DATABASE_URL` exported)

期待結果: migration fileが存在しないためFAIL。

- [x] **Step 3: migrationを実装する**

公開entityには不透明なtext IDを使用し、内部persistence fieldはrepositoryに閉じ込める。次を追加する。

- `requests.version BIGINT NOT NULL CHECK (version >= 1)`と、`draft`、`pending`、`approved`に限定するstatus constraint。
- assignee、`pending`/`approved` status、unique Request IDを持つ、初回Requestあたり1行の`approvals`。
- event type、Actor Member ID、発生時刻、nullableなcontent snapshot/approval metadataを持ち、applicationから更新・削除できないappend-only `audit_events`。
- unique cookie hash、Member ID、CSRF token hash、作成・最終利用・idle-expiry・absolute-expiry・失効時刻を持つ`app_sessions`。
- unique cookie/state hash、nonce、暗号化verifier、issuer/client/redirect値、expiry、消費時刻を持つ`oidc_auth_transactions`。

Pending approvalをassigneeとRequest statusでindexし、active sessionとtransaction lookup keyもindexする。down migrationは依存関係を壊さない逆順でtableを削除する。

- [x] **Step 4: migrationのup/down testを実行する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestMigrations -count=1` (with an isolated `TEST_DATABASE_URL` exported)

期待結果: PASS。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add migrations internal/store/postgres/migrations_test.go
git commit -m "feat: add workflow and auth database migrations"
```

### Task 3: domain workflowとaudit application serviceをunit testとともに実装する

**ファイル:**
- 作成: `internal/domain/request.go`
- 作成: `internal/domain/errors.go`
- 作成: `internal/application/requests/service.go`
- 作成: `internal/application/requests/service_test.go`
- 作成: `internal/application/requests/repository.go`

**Interface:**
- 利用: `Actor{MemberID string, Roles []Role}`、`ExpectedVersion int64`、repository interface。
- 提供: `CreateDraft`、`UpdateDraft`、`Submit`、`Approve`、`Get`、`ListPending`、`ListAuditEvents` methodと、型付きerror（`ErrForbidden`、`ErrNotFound`、`ErrVersionConflict`、`ErrInvalidState`、`ErrApprovalRoutingUnavailable`）。
- 利用元: PostgreSQL adapterとHTTP handler。

- [x] **Step 1: 失敗するtable-driven service testを書く**

Title trim、空Description、Draft限定更新、既定Approverへの自己Submit、既定Approver不在、誤ったRequester、誤ったApprover、古いversion、audit snapshotを対象にする。

```go
func TestSubmitRejectsStaleVersionWithoutAuditEvent(t *testing.T) {
	repo := newFakeRepository(draftRequest(7))
	_, err := service.Submit(ctx, requester, requestID, 6)
	if !errors.Is(err, ErrVersionConflict) { t.Fatalf("got %v", err) }
	if got := repo.AuditEventCount(); got != 0 { t.Fatalf("events = %d", got) }
}
```

- [x] **Step 2: unit testを実行して失敗を確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests -count=1`

期待結果: serviceと型付きerrorが存在しないためFAIL。

- [x] **Step 3: 純粋なapplication ruleを実装する**

HTTP、SQL、OIDC typeをこのpackageへ持ち込まない。`strings.TrimSpace`でTitleをnormalizeし、normalize後の空Titleを拒否してPDR-001の長さ制約を適用する。SubmitでPDR-002を適用し、repositoryが返すApprovalに割当Approverを保存する。成功した変更操作ごとに1件のaudit eventを作成する。

```go
type Repository interface {
	CreateDraft(context.Context, CreateDraftCommand) (domain.Request, error)
	MutateDraft(context.Context, DraftMutationCommand) (domain.Request, error)
	Submit(context.Context, SubmitCommand) (domain.Request, error)
	Approve(context.Context, ApproveCommand) (domain.Request, error)
}
```

- [x] **Step 4: unit testを実行する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/domain ./internal/application/requests -count=1`

期待結果: PASS。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add internal/domain internal/application/requests
git commit -m "feat: add request workflow application service"
```

### Task 4: PostgreSQL repositoryとtransactional concurrency testを実装する

**ファイル:**
- 作成: `migrations/000003_organization_default_approver.up.sql`
- 作成: `migrations/000003_organization_default_approver.down.sql`
- 変更: `internal/store/postgres/migrations_test.go`
- 作成: `internal/store/postgres/requests.go`
- 作成: `internal/store/postgres/requests_test.go`
- 作成: `internal/store/postgres/seed_test.go`

**Interface:**
- 実装: `internal/application/requests.Repository`。
- 利用: `*sql.DB`とTask 2のmigration。
- 提供: transactionを使うDraft/Submit/Approve persistenceとDTO変換可能なdomain value。

PDR-002でAcceptedとなっているOrganization既定Approverを永続化するため、最初に`000003` migrationで`organizations.default_approver_member_id`を追加する。この参照先は同一OrganizationのMemberに限定し、既定Approverの未設定はapplication serviceが既存どおり`ErrApprovalRoutingUnavailable`へ変換する。詳細は`docs/superpowers/plans/2026-09-21-default-approver-schema-repair.md`を参照する。

- [x] **Step 1: 失敗するPostgreSQL integration testを書く**

Organization、Requester、Approver、Admin、既定Approverを各1件seedする。条件付き更新predicateが`id`、期待`version`、期待current stateの全てを含むことをtestする。同じ期待versionで2つのgoroutineを起動し、SubmitまたはApproveが1件だけ成功することを確認する。

```go
func TestApproveIsAtomicWithAuditEvent(t *testing.T) {
	request := seedPendingRequest(t, db, approverID, 3)
	_, err := repo.Approve(ctx, ApproveCommand{RequestID: request.ID, ExpectedVersion: 3, Actor: approver})
	requireNoError(t, err)
	assertRequestVersion(t, db, request.ID, 4)
	assertAuditEventTypes(t, db, request.ID, "request_submitted", "request_approved")
}
```

- [x] **Step 2: integration testを実行して失敗を確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run 'Test(Submit|Approve)' -count=1` (with an isolated `TEST_DATABASE_URL` exported)

期待結果: repository実装が存在しないためFAIL。

- [x] **Step 3: 明示的なSQL repositoryを実装する**

SubmitとApproveでは`BEGIN`/`COMMIT`を使用する。各transactionで`WHERE id = $1 AND version = $2 AND status = $3`により`requests`を更新し、`RowsAffected`を確認してから対応するApproval/Audit Eventを書き込み、commitする。更新行数が0の場合はeventを書き込まず、型付きconflict/state errorを返す。table rowを直接公開せず、domain DTOへmapする。

- [x] **Step 4: 全PostgreSQL repository testを実行する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -count=1` (with an isolated `TEST_DATABASE_URL` exported)

期待結果: 同時変更のcaseを含めてPASS。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add internal/store/postgres
git commit -m "feat: persist workflow transitions atomically"
```

### Task 5: OIDC transactionと不透明なserver-side sessionを実装する

**ファイル:**
- 作成: `internal/auth/oidc.go`
- 作成: `internal/auth/oidc_test.go`
- 作成: `internal/auth/session.go`
- 作成: `internal/auth/session_test.go`
- 作成: `internal/store/postgres/sessions.go`
- 作成: `internal/store/postgres/sessions_test.go`

**Interface:**
- 提供: `Authenticator.BeginLogin`、`Authenticator.CompleteLogin`、`SessionStore.Create`、`SessionStore.Authenticate`、`SessionStore.Revoke`。
- 利用: OIDC issuer/client/redirect configuration、`oidc.Provider`、`oauth2.Config`、暗号化鍵、PostgreSQL auth table。
- 利用元: HTTP login/callback handlerとauthentication middleware。

- [x] **Step 1: 失敗するauth/session testを書く**

`httptest.Server`をOIDC discovery/JWKS/token endpointのtest doubleにする。state不一致、nonce不一致、無効issuer/audience/signature、誤ったverifierによるcode交換、callback replay、暗号化verifier保存、logout、期限切れsession、CSRF token検証をtestする。

```go
func TestCompleteLoginConsumesTransactionBeforeIssuingSession(t *testing.T) {
	tx := createAuthTransaction(t)
	_, err := auth.CompleteLogin(ctx, tx.Cookie, tx.State, "authorization-code")
	requireNoError(t, err)
	_, err = auth.CompleteLogin(ctx, tx.Cookie, tx.State, "authorization-code")
	if !errors.Is(err, ErrInvalidAuthTransaction) { t.Fatalf("got %v", err) }
}
```

- [x] **Step 2: focused auth testを実行して失敗を確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/auth -count=1`

期待結果: authentication/session packageが存在しないためFAIL。

- [x] **Step 3: OIDCとsessionの境界を実装する**

`oidc.NewProvider`、1個の長寿命`Provider.VerifierContext`、`oauth2.GenerateVerifier`、`oauth2.S256ChallengeOption`、`oidc.Nonce`、`oauth2.VerifierOption`を使用する。`crypto/rand`でstate、nonce、verifier、transaction cookie、session cookie、CSRF tokenを生成する。保存するPKCE verifierだけを設定済み32 byte鍵のAES-GCMで暗号化し、cookie/CSRF tokenは保存前にSHA-256でhashする。`SessionStore.IssueCSRFToken`で保存済みCSRF token hashを原子的に置換し、生tokenは`GET /api/v1/session`にだけ返す。生tokenを永続化・log出力してはならない。token検証後にのみ`iss`と`sub`を照合し、明示的repository methodを通じてMemberに対応付ける。

productionでは、`Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/`、Domainなしの`__Host-approval_flow_session` cookieを発行する。configurationがloopback HTTPを許可する場合だけ、開発用の別cookie名を使用する。識別できたtransaction recordは、callbackの成功・失敗にかかわらず削除または失効させる。

- [x] **Step 4: authentication/session testを実行する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/auth ./internal/store/postgres -run 'Test(CompleteLogin|Session|Csrf)' -count=1` (with an isolated `TEST_DATABASE_URL` exported)

期待結果: PASS。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add internal/auth internal/store/postgres/sessions.go internal/store/postgres/sessions_test.go
git commit -m "feat: add OIDC login and opaque sessions"
```

### Task 6: HTTP middleware、error、OIDC endpoint、session endpointを追加する

**ファイル:**
- 作成: `internal/httpapi/router.go`
- 作成: `internal/httpapi/auth_handlers.go`
- 作成: `internal/httpapi/session_handlers.go`
- 作成: `internal/httpapi/errors.go`
- 作成: `internal/httpapi/auth_handlers_test.go`
- 作成: `internal/httpapi/session_handlers_test.go`
- 作成: `cmd/api/main.go`

**Interface:**
- 利用: `auth.Authenticator`、`auth.SessionStore`、`config.Config`、Request application service。
- 提供: `GET /auth/oidc/login`、`GET /auth/oidc/callback`、`GET /auth/oidc/organization-selection`、`POST /auth/oidc/organization-selection`、`GET /api/v1/session`、`POST /api/v1/session/logout`を持つ`http.Handler`。複数Organization候補のcallbackは選択transaction cookieを発行し、選択成功後だけMember-bound sessionを発行する。
- 提供: 型付きerrorをOpenAPIの`ErrorResponse`へmapする`WriteError(http.ResponseWriter, APIError)`。

- [x] **Step 1: 失敗するhandler testを書く**

正確なroute/methodの動作、login/callbackの302 LocationとSet-Cookie、欠落/期限切れsessionの401、CSRF tokenまたはOriginの欠落/不一致時の403 `csrf_validation_failed`、logout時の204とcookie削除、JSON/logにtoken/verifierがないことを確認する。

```go
func TestLogoutRejectsMissingCSRFToken(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/session/logout", nil)
	r.AddCookie(validSessionCookie(t))
	r.Header.Set("Origin", allowedOrigin)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusForbidden { t.Fatalf("status = %d", rr.Code) }
	assertErrorCode(t, rr, "csrf_validation_failed")
}
```

- [x] **Step 2: HTTP auth/session testを実行して失敗を確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Login|Callback|Session|Logout)' -count=1`

期待結果: routerとhandlerが存在しないためFAIL。

- [x] **Step 3: routerとhandlerを実装する**

method-aware `ServeMux` patternを登録する。authentication middlewareは不透明sessionを1回だけloadし、`application.Actor`だけを`request.Context`へ保存する。CSRF middlewareはunsafe `/api/v1` methodだけで実行し、正確な許可Originとsessionに束縛したheader tokenを必須とする。handlerは承認済みerror codeを使用し、queryで受け取ったreturn URLへredirectせず、設定済みlocal UI pathだけへredirectする。

```go
mux.Handle("GET /auth/oidc/login", beginLoginHandler)
mux.Handle("GET /auth/oidc/callback", completeLoginHandler)
mux.Handle("GET /api/v1/session", requireSession(currentSessionHandler))
mux.Handle("POST /api/v1/session/logout", requireCSRF(requireSession(logoutHandler)))
```

- [x] **Step 4: HTTP auth/session testを実行する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Login|Callback|Session|Logout)' -count=1` (with an isolated `TEST_DATABASE_URL` exported)

期待結果: PASS。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add cmd/api/main.go internal/httpapi
git commit -m "feat: expose OIDC and session HTTP endpoints"
```

### Task 7: Request、Approval、AuditのHTTP operationを実装する

**ファイル:**
- 作成: `internal/httpapi/request_handlers.go`
- 作成: `internal/httpapi/request_handlers_test.go`
- 作成: `internal/httpapi/response_dto.go`
- 作成: `internal/httpapi/response_dto_test.go`
- 変更: `internal/httpapi/router.go`

**Interface:**
- 利用: request contextから得た認証済みActorと`application/requests.Service`。
- 提供: 全`/api/v1/requests` operationとOpenAPI形式のJSON DTO。
- 提供: Draft作成時の`201`とLocation、変更成功時の`200`、閲覧不可readの`404 request_not_found`、全failure modeのOpenAPI error response。

- [x] **Step 1: 失敗するHTTP contract testを書く**

全OpenAPI operationに対して`httptest`を使うtestを書く。有効なRequester/Approver session、JSON decode error、field error、禁止mutation、閲覧不可read、古いversion、無効state、approval routing失敗、Audit Event snapshot fieldを含める。

```go
func TestSubmitStaleVersionReturns409AndDoesNotCreateExtraAuditEvent(t *testing.T) {
	r := authenticatedJSONRequest(t, requester, http.MethodPost,
		"/api/v1/requests/req-1/submit", `{"expectedVersion":1}`)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusConflict { t.Fatalf("status = %d", rr.Code) }
	assertErrorCode(t, rr, "version_conflict")
	assertAuditCount(t, "req-1", 1)
}
```

- [x] **Step 2: Request handler testを実行して失敗を確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Create|Get|Update|Submit|Pending|Approve|Audit)' -count=1`

期待結果: Request routeとDTO mappingが存在しないためFAIL。

- [x] **Step 3: decoder、DTO mapping、handlerを実装する**

`DisallowUnknownFields`を有効化した`json.Decoder`を使用し、trailing dataを拒否する。全unsafe Request operationから`expectedVersion`をdecodeする。`r.PathValue("requestId")`を使用し、未検証値をSQLへ連結しない。型付きapplication errorは一元的にmapする。無効inputは400、session欠落は401、禁止mutationは403、閲覧不可/not-found readは404、version/state/routing conflictは409とする。成功mutation後は現在のRequest/Approval DTOを返し、Audit Historyには並び替えたaudit eventを返す。

- [x] **Step 4: 全HTTP API testを実行する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -count=1` (with an isolated `TEST_DATABASE_URL` exported)

期待結果: PASS。

- [x] **Step 5: このタスクのファイルをcommitする**

```bash
git add internal/httpapi/request_handlers.go internal/httpapi/request_handlers_test.go internal/httpapi/response_dto.go internal/httpapi/response_dto_test.go internal/httpapi/router.go
git commit -m "feat: expose request approval API"
```

### Task 8: 公開契約、application起動、開発者documentを検証する

**ファイル:**
- 変更: `api/openapi.yaml`
- 変更: `docs/development/toolchain.md`
- 作成: `docs/development/local-api.md`
- 作成: `scripts/verify-openapi.mjs`
- 作成: `scripts/verify-openapi.test.mjs`
- 変更: `package.json`
- 変更: `pnpm-lock.yaml`

**Interface:**
- 利用: `api/openapi.yaml`と既存の固定済みNode/pnpm toolchain。
- 提供: CIまたはimplementation testの前にOpenAPI YAMLをparseし、local component referenceを確認する`pnpm run verify:openapi`。
- 提供: PostgreSQL migration、Keycloak起動、必須runtime secret入力、API test commandの再現可能な手順。

- [ ] **Step 1: 失敗するcontract validation testを書く**

契約をloadし、`openapi`が`3.1.`で始まること、全`$ref`値が`components`配下でresolveすること、承認済みoperation/security requirementが残っていることを確認するNode testを追加する。

```js
test("all local OpenAPI references resolve", () => {
  const document = loadOpenAPI("api/openapi.yaml");
  for (const reference of references(document)) {
    assert.ok(resolveLocalReference(document, reference), reference);
  }
});
```

- [ ] **Step 2: testを実行して失敗を確認する**

Run: `pnpm run verify:openapi`

期待結果: parserとscriptが存在しないためFAIL。

- [ ] **Step 3: 最小の直接開発依存とverification scriptを追加する**

`yaml` v2.9.1を正確な直接`devDependency`として追加し、ADR-007のreview手順に従って`pnpm-lock.yaml`を更新する。`scripts/verify-openapi.mjs`ではこれとNode標準moduleだけを使用する。これはapplication runtime dependencyではなくcontract validation専用のdevelopment dependencyであることを`docs/development/toolchain.md`に記録する。`package.json`へ`verify:openapi`を追加する。

- [ ] **Step 4: 再現可能なlocal API運用手順をdocument化する**

`docs/development/local-api.md`に、PostgreSQLの準備、明示的なmigration適用、外部から与えたtest credentialを使うloopback限定Keycloak development modeの起動、必須OIDC/session暗号化configurationの設定、`cmd/api`の起動、Go unit/integration test、`pnpm run verify:openapi`の実行という正確な順序を記載する。credentialや暗号化鍵をrepositoryへ置かない。

- [ ] **Step 5: 完全なAPI verification suiteを実行する**

Run:

```bash
pnpm install --frozen-lockfile
pnpm run format:check
pnpm run lint
pnpm run typecheck
pnpm run check:gofmt
pnpm run verify:openapi
GOTOOLCHAIN=go1.27.1 go vet ./...
GOTOOLCHAIN=go1.27.1 go test ./internal/domain ./internal/application/requests ./internal/auth ./internal/httpapi
GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres ./internal/httpapi -count=1
GOTOOLCHAIN=go1.27.1 go mod verify
git diff --check
```

期待結果: PASS。integration commandには分離されたPostgreSQL database URLが必要であり、production databaseに対してtestを実行してはならない。

- [ ] **Step 6: このタスクのファイルをcommitする**

```bash
git add api/openapi.yaml docs/development/toolchain.md docs/development/local-api.md scripts package.json pnpm-lock.yaml
git commit -m "test: verify API contract and local operation"
```

## 計画の自己レビュー

- **仕様coverage:** Task 0はADR-012のFormatter/Linter、非破壊CI検査、ADR-007に従うBiome導入を扱う。Task 1/5はOIDC・PKCE・session・CSRFを扱う。Task 2/4はPostgreSQL、migration、audit、concurrencyを扱う。Task 3はPDRのworkflow ruleを扱う。Task 6/7は全OpenAPI operationとerror classを扱う。Task 8はcontract validation、品質ゲート、再現可能な運用を扱う。
- **未完了表現の走査:** 全taskでファイル、interface、command、期待結果を明示している。
- **型の整合性:** `application.Actor`は`internal/auth`からHTTP middlewareを経てRequest serviceへ渡る認証済みidentityである。`expectedVersion`は全体で`int64`とする。OpenAPI operation名は対応するhandler/service methodへ直接mapする。
- **レビュー重点項目:** Task 0は未整形Go sourceとCI非破壊性、Task 3はnormalize済みTitleとaudit content、Task 4はrace-safe mutation、Task 5はcallback replay、Task 6はCSRF/origin、Task 7はauthorization/error mappingをtestする。
