# Task 8: 公開契約とローカルAPI運用の検証計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans. Track steps with checkboxes.

**Goal:** OpenAPI 3.1契約を自動検証し、Go APIのローカル運用を再現可能な文書として提供する。

**Architecture:** `api/openapi.yaml`を正本とし、Node標準moduleと直接開発依存`yaml`だけで構文、local `$ref`、必須operation/securityを検証する。runtime dependency、code generation、外部credentialは追加しない。

**Tech Stack:** Node.js 26.9.0、pnpm 12.5.1、`yaml` 2.9.1、`node:test`、Go 1.27.1、PostgreSQL 17.11、Keycloak 26.7.4。

**Spec:** `docs/superpowers/plans/2026-09-20-go-api-implementation.md` Task 8、`api/openapi.yaml`、ADR-002/003/006/007/012。

## Global Constraints

- `yaml`は正確な`2.9.1`の直接`devDependency`。`pnpm-workspace.yaml`のinstall policyと`allowBuilds: {}`は変更しない。
- validatorはNode標準moduleと`yaml`だけを使い、外部reference、fragmentなしreference、不正JSON Pointerを拒否する。
- credential、秘密鍵、production database URLを文書やtestへ書かない。integration testは既存の隔離PostgreSQL手順を使う。
- OpenAPI operationとsecurity scheme以外のAPI機能を追加しない。

## Review Focus

- 欠落local `$ref`、escaped JSON Pointer、OpenAPI version driftを検出する。
- 必須operationIdとsecurity schemeの削除を検出する。
- `yaml`依存とlockfile/install policyを一致させる。
- local API文書のenvironment名、route、test commandを実装と一致させ、secretを含めない。

---

### Task 8.1: OpenAPI検証scriptとテスト

**Files:**
- Create: `scripts/verify-openapi.mjs`
- Create: `scripts/verify-openapi.test.mjs`

**Interfaces:** `loadOpenAPI(filePath)`, `references(value)`, `resolveLocalReference(document, reference)`, `validateOpenAPI(document)`をexportする。CLIの既定pathは`api/openapi.yaml`。

- [ ] **Step 1: 失敗するtestを書く**

`node:test`で、repository YAMLの`3.1.` version、全local `$ref`の解決、escaped pointer `#/components/schemas/a~1b~0c`、missing referenceの失敗、required operationの不足を検証する。required operationIdは`beginOidcLogin`, `completeOidcLogin`, `getOrganizationSelection`, `selectOrganization`, `getCurrentSession`, `logout`, `createRequest`, `listPendingRequests`, `getRequest`, `updateDraftRequest`, `submitRequest`, `approveRequest`, `listRequestAuditEvents`。required schemesは`sessionCookie`, `organizationSelectionCookie`, `csrfToken`。

- [ ] **Step 2: testを失敗させる**

Run: `node --test scripts/verify-openapi.test.mjs`

Expected: script未作成でFAIL。

- [ ] **Step 3: 最小実装**

`yaml.parse`でloadし、object/arrayを再帰走査して`$ref`を収集する。`resolveLocalReference`は`#/`だけを受け付け、JSON Pointerの`~1`/`~0`を復号してobject keyまたはarray indexを辿る。`validateOpenAPI`はversion、paths/componentsのobject性、全local reference、required operationIdの重複・欠落、required security schemeを検証する。CLIはsuccess時に`OpenAPI contract valid: ...`、failure時にstderrとexit 1を返す。

- [ ] **Step 4: focused testを通す**

Run: `node --test scripts/verify-openapi.test.mjs`

Expected: PASS。

### Task 8.2: 依存とpackage script

**Files:** `package.json`, `pnpm-lock.yaml`, `docs/development/toolchain.md`

- [ ] **Step 1:** `devDependencies`へ`"yaml": "2.9.1"`、scriptsへ`"verify:openapi": "node scripts/verify-openapi.mjs"`を追加する。
- [ ] **Step 2:** `pnpm install --lockfile-only --ignore-scripts`を実行する。build script許可や`allowBuilds`変更が必要なら停止する。
- [ ] **Step 3:** `pnpm install --frozen-lockfile && pnpm run verify:openapi && node --test scripts/verify-openapi.test.mjs`を実行する。

### Task 8.3: local API document

**Files:** Create `docs/development/local-api.md`

- [ ] **Step 1:** 固定version、`pnpm install --frozen-lockfile`、test PostgreSQL/`TEST_DATABASE_URL`、明示migration、loopback限定Keycloak 26.7.4、`AUTH_TRANSACTION_KEY`の32-byte条件、`SESSION_*`/`OIDC_*`設定、`go run ./cmd/api`、verification commands、cleanupを正確な順序で記録する。実credentialや秘密値は記載しない。
- [ ] **Step 2:** `pnpm run verify:openapi`とDB不要のGo testsで文書のコマンドを確認する。

### Task 8.4: 完全verificationと完了記録

- [ ] **Step 1:** `pnpm install --frozen-lockfile`, `pnpm run format:check`, `pnpm run lint`, `pnpm run typecheck`, `pnpm run check:gofmt`, `pnpm run verify:openapi`, Node testを実行する。
- [ ] **Step 2:** `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...`、全unit test、`pnpm run test:db`、`go mod verify`、`git diff --check`を実行する。
- [ ] **Step 3:** OpenAPI operationIdとrouter、error response、config environment、DB scriptを突合し、範囲外変更がないことを確認する。
- [ ] **Step 4:** `docs/development/task8-completion-2026-09-23.md`へ変更、Decision/Requirement、verification結果、riskを記録する。
