# 複数Organization対応OIDC Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 同一の検証済みOIDC identityを複数OrganizationのMemberに対応付け、選択済みMemberだけへ不透明なapplication sessionを発行する。

**Architecture:** OIDC identityをMemberから分離し、PostgreSQLの対応表でidentityと複数Memberを関連付ける。callbackは単一候補にはsessionを発行し、複数候補には短命かつ単回使用のOrganization選択transactionを発行する。選択endpointがtransactionに固定された候補とCSRF tokenを原子的に検証・消費してから、Member-bound sessionを発行する。

**Tech Stack:** Go 1.27.1、`net/http`、`database/sql`、PostgreSQL、golang-migrate v4.20.1、pgx stdlib v5.11.0、`github.com/coreos/go-oidc/v3` v3.21.0、`golang.org/x/oauth2` v0.37.0、標準`crypto`、`httptest`、標準`testing`。

**Spec:** `docs/superpowers/specs/2026-09-21-multi-organization-oidc-identity-design.md`

## Global Constraints

- ADR-004に従い、PostgreSQL、`database/sql`、手書きSQL、golang-migrateだけを使い、ORM、query builder、session/JWT/test-container依存を追加しない。
- ADR-005、ADR-009、ADR-010、ADR-011、ADR-013に従い、OIDC token、role/group、未検証のクライアントActor入力を認可の根拠にしない。
- `iss`と`sub`の組だけでOIDC identityを照合し、Request認可とAudit Actorは選択済みMemberからサーバー側で決定する。
- browser cookieにはCSPRNG生成の不透明値だけを保存し、cookie/state/CSRF tokenはSHA-256 hashだけを保存する。PKCE verifierだけは`AuthTransactionKey`でAES-GCM暗号化して保存する。
- callbackまたはOrganization選択transactionは成功・失敗を問わず再利用不能にし、既存sessionを昇格・再利用しない。
- production cookieは`__Host-approval_flow_session`、`Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/`、Domainなしとする。loopback developmentだけ異なる非Secure名を許可する。
- `internal/application/requests.Repository`、Request workflow、migration `000001`〜`000003`を変更しない。

## Review Focus

- 同じsubjectでもissuerが異なるidentityは別identityとして扱い、sessionを発行しない。Task 2で実PostgreSQL testを追加する。
- 対応表が変更されても、選択transaction作成時点の候補外Memberを選べない。Task 2で候補snapshot testを追加する。
- 複数候補のcallbackがapplication sessionを先に発行しない。Task 3でOIDC test doubleを使って確認する。
- 選択POSTのCSRF token不一致、cookie不一致、expiry、replayはsessionを発行せずtransactionを再利用不能にする。Task 2とTask 4で確認する。
- identity subject、token、verifier、cookie、CSRF tokenがresponse、log、平文DB列へ出ない。Task 3とTask 4で検査する。

---

## File Structure

- `migrations/000004_oidc_identity_memberships.{up,down}.sql` — identity、対応表、選択transaction候補snapshot、auth transactionの`created_at`。
- `internal/store/postgres/sessions.{go,test.go}` — identity lookup、OIDC transaction、session、選択transactionを実PostgreSQLで処理する。
- `internal/auth/{oidc,session}.{go,test.go}` — OIDC discovery、callback検証、opaque secrets、cookie policy、Member候補分岐。
- `internal/httpapi/{router,auth_handlers,auth_handlers_test}.go` — login/callbackとOrganization候補取得・選択endpoint。
- `api/openapi.yaml` — Organization候補取得と選択のbrowser/API契約。
- `internal/store/postgres/{migrations_test,seed_test}.go`、Task 5 handoff、既存実装計画 — migration・fixture・トレーサビリティ。

### Task 1: identity と選択transaction schemaを追加する

**Files:**
- Create: `migrations/000004_oidc_identity_memberships.up.sql`
- Create: `migrations/000004_oidc_identity_memberships.down.sql`
- Modify: `internal/store/postgres/migrations_test.go`

**Interfaces:**
- Consumes: `members(id)`、`oidc_auth_transactions`、migration `000001`〜`000003`。
- Produces: identity、対応表、選択transaction、候補snapshot、および`oidc_auth_transactions.created_at`。

- [ ] **Step 1: 失敗するmigration integration testを書く**

`TestMigrationsCreateWorkflowAndAuthTables`へtable存在、`created_at`非NULL、`UNIQUE (issuer, subject)`、同一identityの異Organization Member対応、候補tableの重複拒否を追加する。

```go
_, err := db.Exec(`INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-1', 'https://issuer.example', 'subject-1')`)
if err != nil { t.Fatal(err) }
_, err = db.Exec(`INSERT INTO oidc_identities (id, issuer, subject) VALUES ('identity-2', 'https://issuer.example', 'subject-1')`)
if err == nil { t.Fatal("duplicate issuer/subject was accepted") }
```

- [ ] **Step 2: migration testが失敗することを確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestMigrationsCreateWorkflowAndAuthTables -count=1`  
Expected: FAIL。identity tableまたは`created_at`が存在しない。

- [ ] **Step 3: migrationを最小実装する**

```sql
CREATE TABLE oidc_identities (
  id text PRIMARY KEY, issuer text NOT NULL, subject text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE (issuer, subject)
);
CREATE TABLE member_oidc_identities (
  identity_id text NOT NULL REFERENCES oidc_identities(id),
  member_id text NOT NULL REFERENCES members(id),
  PRIMARY KEY (identity_id, member_id)
);
CREATE TABLE organization_selection_transactions (
  id text PRIMARY KEY, cookie_hash bytea NOT NULL UNIQUE,
  csrf_token_hash bytea NOT NULL,
  identity_id text NOT NULL REFERENCES oidc_identities(id),
  expires_at timestamptz NOT NULL, consumed_at timestamptz
);
CREATE TABLE organization_selection_transaction_members (
  transaction_id text NOT NULL REFERENCES organization_selection_transactions(id),
  member_id text NOT NULL REFERENCES members(id),
  PRIMARY KEY (transaction_id, member_id)
);
ALTER TABLE oidc_auth_transactions ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
```

down migrationは候補table、選択transaction、対応表、identity table、`created_at`を逆順に削除する。

- [ ] **Step 4: migration testを成功確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run TestMigrationsCreateWorkflowAndAuthTables -count=1`  
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add migrations/000004_oidc_identity_memberships.up.sql migrations/000004_oidc_identity_memberships.down.sql internal/store/postgres/migrations_test.go
git commit -m "feat: add OIDC identity membership schema"
```

### Task 2: PostgreSQL identity、session、選択transaction repositoryを実装する

**Files:**
- Create: `internal/auth/session.go`
- Create: `internal/auth/session_test.go`
- Create: `internal/store/postgres/sessions.go`
- Create: `internal/store/postgres/sessions_test.go`
- Modify: `internal/store/postgres/seed_test.go`

**Interfaces:**
- Consumes: Task 1 schema。
- Produces: `internal/auth`の`Identity`、`AuthTransaction`、`Session`、`OrganizationSelection`、`ErrNotFound`、`ErrExpired`、`ErrConsumed`と、`CreateAuthTransaction`、`ConsumeAuthTransaction`、`MembersForIdentity`、`CreateSession`、`AuthenticateSession`、`RevokeSession`、`CreateOrganizationSelection`、`ConsumeOrganizationSelection`。

- [ ] **Step 1: 実PostgreSQLの失敗するrepository testを書く**

まず`session_test.go`にopaque値のSHA-256 hash、AES-GCM暗号化と復号、production/development cookie policyを検証する失敗testを書く。続けて同一identityを`org-1/member-1`と`org-2/member-2`へ対応付けるfixtureを作る。cookie/CSRF hashだけの保存、issuer違いの候補0件、候補snapshot、候補外Member・CSRF不一致・replay・expiryの拒否、成功時に1件だけsessionが作られることをtable-driven integration testで検証する。

```go
_, err := repository.ConsumeOrganizationSelection(ctx, auth.ConsumeOrganizationSelectionInput{
  Cookie: cookie, CSRFToken: csrf, MemberID: "member-not-a-candidate", Now: now,
})
if !errors.Is(err, auth.ErrForbidden) { t.Fatalf("err = %v, want ErrForbidden", err) }
```

- [ ] **Step 2: repository testが失敗することを確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run 'Test(Identity|Session|OrganizationSelection)' -count=1`  
Expected: FAIL。session repositoryまたはauth値型が未実装。

- [ ] **Step 3: repositoryとfixtureを実装する**

入力cookieとCSRF tokenは`sha256.Sum256`して照合する。`ConsumeOrganizationSelection`は単一transactionでactive recordを`FOR UPDATE`し、hash、expiry、CSRF hash、候補tableを照合して`consumed_at`を更新し、その後に`app_sessions`へinsertする。識別済みselectionは拒否時もconsumeし、rollback時にsessionを残さない。

```go
type ConsumeOrganizationSelectionInput struct {
  Cookie, CSRFToken, MemberID string
  Now                         time.Time
  Session                     SessionInput
}
func (r *Repository) ConsumeOrganizationSelection(ctx context.Context, input ConsumeOrganizationSelectionInput) (Session, error)
```

最初に`session.go`へrepository非依存の値型、sentinel error、SHA-256/AES-GCM helper、cookie policyを実装する。次にrepositoryでその型を使用する。`AuthenticateSession`はcookie hash、revocation、idle/absolute expiryを検査し、成功時だけ`last_used_at`を更新する。

- [ ] **Step 4: repository testを成功確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run 'Test(Identity|Session|OrganizationSelection)' -count=1`  
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/auth/session.go internal/auth/session_test.go internal/store/postgres/sessions.go internal/store/postgres/sessions_test.go internal/store/postgres/seed_test.go
git commit -m "feat: persist OIDC identities and selection transactions"
```

### Task 3: OIDC callbackとsession境界を実装する

**Files:**
- Create: `internal/auth/oidc.go`
- Create: `internal/auth/oidc_test.go`

**Interfaces:**
- Consumes: `config.Config`、Task 2のsession値型とrepository、`oidc.Provider`、`oauth2.Config`。
- Produces: `Authenticator.BeginLogin(context.Context) (LoginStart, error)`、`Authenticator.CompleteLogin(context.Context, CallbackInput) (LoginResult, error)`、`SessionStore.Authenticate`、`SessionStore.Revoke`、`SessionStore.CompleteOrganizationSelection`。

- [ ] **Step 1: 失敗するauth testを書く**

`httptest.Server`でdiscovery、JWKS、token endpointを持つOIDC test doubleを作る。state、nonce、issuer、audience、署名、有効期限、PKCE verifier不一致とcallback replayを拒否し、単一候補はsession、複数候補はselectionだけを返すことを検証する。

```go
result, err := authenticator.CompleteLogin(ctx, auth.CallbackInput{TransactionCookie: txCookie, State: state, Code: code})
if err != nil { t.Fatal(err) }
if result.Session != nil || result.Selection == nil {
  t.Fatalf("multiple memberships result = %#v, want selection without session", result)
}
```

- [ ] **Step 2: auth testが失敗することを確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/auth -count=1`  
Expected: FAIL。packageまたはAuthenticatorが未実装。

- [ ] **Step 3: OIDC、cookie、暗号化境界を最小実装する**

`oidc.NewProvider`を初期化時に一度だけ呼び、同じProviderの`VerifierContext`を再利用する。`crypto/rand`、`oauth2.GenerateVerifier`、`oauth2.S256ChallengeOption`、`oidc.Nonce`、`oauth2.VerifierOption`を使う。PKCE verifierはAES-GCM暗号化、cookie/state/CSRF tokenはhashだけをrepositoryへ渡す。

```go
type LoginResult struct {
  Session   *IssuedSession
  Selection *IssuedOrganizationSelection
}
func (a *Authenticator) CompleteLogin(ctx context.Context, input CallbackInput) (LoginResult, error)
func (s *SessionStore) CompleteOrganizationSelection(ctx context.Context, input OrganizationSelectionInput) (IssuedSession, error)
```

認証transactionを最初にconsumeし、ID Token検証後だけ`iss`/ `sub`でMember候補を解決する。raw secretをerror、JSON、logへ含めない。

- [ ] **Step 4: auth testを成功確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/auth -count=1`  
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add internal/auth/oidc.go internal/auth/oidc_test.go
git commit -m "feat: add OIDC login and member-bound sessions"
```

### Task 4: Organization選択のHTTP契約とhandlerを追加する

**Files:**
- Modify: `api/openapi.yaml`
- Create: `internal/httpapi/router.go`
- Create: `internal/httpapi/auth_handlers.go`
- Create: `internal/httpapi/auth_handlers_test.go`

**Interfaces:**
- Consumes: Task 3の`LoginResult`と`SessionStore.CompleteOrganizationSelection`、`config.Config.AllowedOrigin`。
- Produces: `GET /auth/oidc/organization-selection`、`POST /auth/oidc/organization-selection`、既存login/callback route。

- [ ] **Step 1: 失敗するHTTP contract testとOpenAPI assertionsを書く**

GETはselection cookieで候補Organization/Member表示名とselection専用CSRF tokenを返す。POSTは`memberId` JSONと`X-CSRF-Token`を受け、成功時だけsession cookieをSet-CookieしてUIへ302 redirectする。missing/invalid/replayed selectionは400 `invalid_auth_transaction`、候補外Memberは403 `forbidden`、tokenまたはOrigin不一致は403 `csrf_validation_failed`とする。

```go
r := httptest.NewRequest(http.MethodPost, "/auth/oidc/organization-selection", strings.NewReader(`{"memberId":"member-2"}`))
r.AddCookie(selectionCookie)
r.Header.Set("X-CSRF-Token", selectionCSRFToken)
r.Header.Set("Origin", allowedOrigin)
rr := httptest.NewRecorder()
handler.ServeHTTP(rr, r)
if rr.Code != http.StatusFound { t.Fatalf("status = %d, want 302", rr.Code) }
```

- [ ] **Step 2: HTTP testが失敗することを確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Callback|OrganizationSelection)' -count=1`  
Expected: FAIL。routerとhandlerが未実装。

- [ ] **Step 3: contract、router、handlerを最小実装する**

GETはsessionを発行せず候補とraw selection CSRF tokenだけを返す。POSTは正確なOrigin、header token、cookieを検証してからselectionをconsumeする。callbackは単一候補ならsession redirect、複数候補なら選択画面へredirectする。session発行まで`application.Actor`をcontextへ置かない。

```go
mux.Handle("GET /auth/oidc/organization-selection", organizationSelectionHandler)
mux.Handle("POST /auth/oidc/organization-selection", completeOrganizationSelectionHandler)
```

- [ ] **Step 4: HTTP testを成功確認する**

Run: `GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Callback|OrganizationSelection)' -count=1`  
Expected: PASS。

- [ ] **Step 5: Commit**

```bash
git add api/openapi.yaml internal/httpapi/router.go internal/httpapi/auth_handlers.go internal/httpapi/auth_handlers_test.go
git commit -m "feat: add organization selection after OIDC login"
```

### Task 5: 文書同期と完全検証

**Files:**
- Modify: `docs/development/handoff-2026-09-21-task5.md`
- Modify: `docs/superpowers/plans/2026-09-20-go-api-implementation.md`

**Interfaces:**
- Consumes: Tasks 1〜4。
- Produces: ADR-013と実装境界に整合したhandoff、計画、検証記録。

- [ ] **Step 1: documentation差分を書く**

「migrationを追加・変更しない」「callbackが必ずsessionを発行する」という旧記述を、ADR-013、identity provisioning、複数候補の選択transaction、Task 6の新endpointに更新する。Task 8で作成予定の`docs/development/local-api.md`は先取り作成しない。

- [ ] **Step 2: documentation diffを確認する**

Run: `git diff -- docs/development/handoff-2026-09-21-task5.md docs/superpowers/plans/2026-09-20-go-api-implementation.md`  
Expected: ADR-013に整合し、既存migrationを編集しない記録だけが含まれる。

- [ ] **Step 3: 完全検証を実行する**

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

Expected: PASS。既知のVitestとNode標準testの収集競合による`pnpm test`失敗は、scope外リスクとして記録する。

- [ ] **Step 4: execution ledgerを更新する**

`.superpowers/sdd/2026-09-20-go-api-implementation/progress.md`へ、ADR-013、identity provisioning前提、実行した検証と結果、既知の`pnpm test`問題を日本語で追記する。このgit管理外directoryはcommitしない。

- [ ] **Step 5: Commit**

```bash
git add docs/development/handoff-2026-09-21-task5.md docs/superpowers/plans/2026-09-20-go-api-implementation.md
git commit -m "docs: align OIDC implementation plan with identity memberships"
```
