# React Frontend Task 4 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 実際の OIDC と別 browser context を使い、Requester の Submit から Approver の Approve、Requester の Audit 確認までを再現可能なブラウザ E2E で証明する。

**Architecture:** `test:e2e` は専用の PostgreSQL と Keycloak を Compose で起動し、migration、Keycloak 利用者とアプリ DB の membership を provision してから Go API と Vite を子プロセスとして起動する。Playwright は loopback の同一 site・別 origin 上で UI を操作し、runner は終了理由にかかわらず自分が作成した資源だけを停止する。

**Tech Stack:** Node.js 26.9.0、pnpm 12.5.1、Go 1.27.1、PostgreSQL 17.11、Keycloak 26.7.4（既存 digest）、Playwright 1.63.0、React 19.3.0。新しい package は追加しない。

**Spec:** [親計画](2026-09-23-react-frontend-implementation.md) Task 4、[API 契約](../../superpowers/specs/2026-09-20-api-contract-design.md)、[複数 Organization 設計](../../superpowers/specs/2026-09-21-multi-organization-oidc-identity-design.md)、`api/openapi.yaml`。

## Global Constraints

- Accepted ADR-001/003/005/006/007/009/011/013/015 と PDR-001/002 に従う。FR-003/004/005/007/011/012、NFR-001/002/003/006 を追跡する。
- Frontend は `http://127.0.0.1:5173`、API は `http://127.0.0.1:8080`、Keycloak issuer は `http://127.0.0.1:8081/realms/approval-flow-dev`。別 origin・同一 site を固定し、realm の callback `http://127.0.0.1:8080/auth/oidc/callback` と一致させる。
- 開発用 HTTP cookie 例外は loopback の `APP_ENV=development` のみ。認証・認可は実 OIDC とアプリ DB の mapping を使い、test-only API route、Actor header、偽 session を導入しない。
- test credential と `AUTH_TRANSACTION_KEY` は実行ごとに生成し、repository、process arguments、console、Playwright artifact に保存しない。E2E データは専用 Compose project/volume のみへ書く。
- `compose.local.yaml`、`compose.test.yaml`、既存の開発 DB/Keycloak を操作しない。固定 port が使用中なら、既存サービスを停止せず明確に失敗する。
- Playwright は最小のブラウザ統合を担当する。状態遷移、認可、競合の網羅は既存 Go/Vitest に残す。static host/CDN、TLS termination、cache、deployment automation は ADR-015 の未決範囲。

## Review Focus

- port 8080/8081/5173/55432 が使用中の場合、既存 process を停止せず E2E 起動前に失敗する（Task 1 の runner test）。
- Keycloak 利用者の `sub` と DB の `oidc_identities` が違う場合、ログイン成功に見せかけず provisioning で失敗する（Task 2 の seed 検証）。
- Requester と Approver が同じ browser context を共有した場合、独立した session の証拠にならない（Task 3 の context identity assertion）。
- Organization 選択後の redirect が API origin に戻る場合、Frontend 上の session が使えない（Task 3 の URL・session assertion）。
- 失敗時に secret を含む trace/HAR/log が保存される場合、テスト証拠から秘密が漏れる（Task 1 と 3 の artifact test）。

## ファイル構成

- `compose.e2e.yaml`: E2E 専用 PostgreSQL と digest 固定 Keycloak。loopback port、healthcheck、使い捨て volume。
- `scripts/e2e-stack.mjs`: 実行ごとに一意な Compose project 名を生成し、起動順、bounded readiness、環境変数、子プロセス、SIGINT/SIGTERM、cleanup、Playwright 実行を管理。
- `scripts/e2e-stack.test.mjs`: spawn/health/cleanup/port 占有の失敗を fake process で検証する runner test。
- `scripts/e2e-seed.mjs` と `.test.mjs`: Keycloak `sub` を取得・検証し、migration 後のアプリ DB に Organization、Member、Role、identity mapping、既定 Approver を作る。SQL 値は固定 ID と検証済み UUID だけを使う。
- `playwright.config.ts`: Frontend URL、単一 worker、retry なし、secret を含む trace/HAR/video を保存しない reporter 設定。
- `e2e/approval-flow.spec.ts`: 実 login、二者フロー、複数 membership 選択。Page helper は spec 内に閉じる。
- `package.json`: 既存 `test:e2e` を stack runner に接続し、runner unit test を `test:e2e:runner` として追加する。lockfile は変更しない。
- `.env.example`、`docs/development/frontend-local-development.md`、`docs/development/toolchain.md`: ローカル起動と E2E 前提・コマンド。
- `docs/development/frontend-completion-2026-09-24.md`: **すべての検証成功後**に実測結果と追跡情報を記録する。親計画の 2026-09-23 ファイル名指定は実際の完了日に合わせる。

---

### Task 1: 専用 stack runner と確実な cleanup

**Files:** Create `compose.e2e.yaml`, `scripts/e2e-stack.mjs`, `scripts/e2e-stack.test.mjs`; modify `package.json`.

**Interfaces:** `pnpm run test:e2e` → `node scripts/e2e-stack.mjs`。runner は固定 local URL、`DATABASE_URL`、`OIDC_ISSUER`、`APP_FRONTEND_ORIGIN`、`VITE_API_ORIGIN` を子プロセスに渡し、Playwright 終了 code を返す。`runStack(deps)` は injected spawn/fetch/port check を使い unit test できるよう export する。

- [ ] **Step 1: cleanup と port 保護の失敗する test を書く。** `node:test` で fake `spawn` を注入し、port 占有時に Compose `up` が呼ばれないこと、Playwright が非 0 終了しても API/Vite に SIGTERM と Compose `down --volumes --remove-orphans` が呼ばれること、秘密文字列を含む env がログへ出ないことを検証する。例:

```js
test("occupied port leaves existing services alone", async () => {
  const commands = [];
  await assert.rejects(runStack(fakeDeps({ occupiedPort: 8081, commands })), /8081/);
  assert.deepEqual(commands, []);
});
test("failed browser run cleans only its project", async () => {
  const calls = [];
  await assert.rejects(runStack(fakeDeps({ playwrightExit: 1, calls })), /Playwright/);
  assert.match(calls.at(-1)[3], /^approval-flow-e2e-[a-f0-9]+$/);
  assert.deepEqual(calls.at(-1).slice(4), ["-f", "compose.e2e.yaml", "down", "--volumes", "--remove-orphans"]);
});
```

- [ ] **Step 2: RED を確認する。** `node --test scripts/e2e-stack.test.mjs` → `runStack` が存在せず FAIL。
- [ ] **Step 3: 最小実装を行う。** `compose.e2e.yaml` には `postgres:17.11-bookworm` と既存 `compose.local.yaml` と同じ Keycloak digest を記載し、DB `127.0.0.1:55432`、Keycloak `127.0.0.1:8081` のみを公開する。runner は `net.createServer()` で 4 port の空きを先に確認し、`approval-flow-e2e-<random hex>` の project 名で `docker compose ... up -d --wait`、`go run ./cmd/migrate-local up`、Task 2 の seed、`go run ./cmd/api`、`pnpm exec vite --host 127.0.0.1 --port 5173 --strictPort`、`pnpm exec playwright test` の順に実行する。Keycloak discovery、API の `/api/v1/session`（未認証 401 が到達証拠）、Vite root を deadline 付きで待ち、子プロセスの早期終了も検知する。実行ごとに `crypto.randomBytes` で test password と 32 byte key を生成し、stdout/stderr は秘密を含む可能性のある応答本文を記録しない。`finally` で起動済み子だけを停止し、その実行の Compose project だけを `down -v` する。cleanup 失敗は成功扱いにしない。
- [ ] **Step 4: GREEN を確認する。** `node --test scripts/e2e-stack.test.mjs` と `pnpm run format:check` → PASS。`package.json` は `test:e2e` を runner へ接続し、`test:e2e:runner` を追加するだけ。
- [ ] **Step 5: commit。** `git add compose.e2e.yaml scripts/e2e-stack.mjs scripts/e2e-stack.test.mjs package.json && git commit -m "test: add isolated browser test stack"`。

### Task 2: 実 OIDC identity と app membership の provision

**Files:** Create `scripts/e2e-seed.mjs`, `scripts/e2e-seed.test.mjs`; modify `scripts/e2e-stack.mjs`.

**Interfaces:** `provisionE2E({ exec, issuer, password })` は Keycloak 管理 CLI に `requester`、`approver`、`multi` を作成し、返された UUID `sub` を使って PostgreSQL に mapping を作る。`multi` は Org A と Org B の二つの Requester Member に紐付ける。Org A の既定 Approver は `approver` の Member。

- [ ] **Step 1: 失敗する seed test を書く。** Keycloak が空 ID、重複 ID、UUID 以外、または必須利用者不足を返したら SQL 実行前に失敗すること、正常時は二つの Organization、3 identity、4 Member、Requester/Approver role、Org A の default Approver、`multi` の候補 2 件を検証する。例:

```js
test("rejects untrusted Keycloak subject before SQL", async () => {
  const sql = [];
  await assert.rejects(provisionE2E(fakeExec({ requester: "'bad'", sql })), /subject/);
  assert.equal(sql.length, 0);
});
```

- [ ] **Step 2: RED を確認する。** `node --test scripts/e2e-seed.test.mjs` → `provisionE2E` が存在せず FAIL。
- [ ] **Step 3: 最小実装を行う。** 既存 `scripts/provision-keycloak.sh` と同じ `kcadm` 操作を E2E 専用 Compose project に適用する。管理 credential と test password は stdin/env で渡し、値を CLI 引数やログに置かない。Keycloak の `users` 検索結果を `username` 完全一致かつ 1 件に限定し、`id` を UUID として検証する。`psql -v ON_ERROR_STOP=1` への stdin に transaction 付き SQL を渡し、固定 test ID と検証済み UUID のみを埋め込む。`oidc_identities(issuer, subject)`、`member_oidc_identities`、`member_roles`、`organizations.default_approver_member_id` を明示的に設定し、最後に件数を照合する。Keycloak/DB の認可正本を混同しない。
- [ ] **Step 4: GREEN を確認する。** `node --test scripts/e2e-seed.test.mjs scripts/e2e-stack.test.mjs` → PASS。既存 `scripts/provision-keycloak.sh` の通常開発用引数や `compose.local.yaml` は変更しない。
- [ ] **Step 5: commit。** `git add scripts/e2e-seed.mjs scripts/e2e-seed.test.mjs scripts/e2e-stack.mjs && git commit -m "test: provision isolated OIDC memberships"`。

### Task 3: 実ブラウザで二者フローと Organization 選択を証明

**Files:** Create `playwright.config.ts`, `e2e/approval-flow.spec.ts`.

**Interfaces:** `playwright.config.ts` は `baseURL=http://127.0.0.1:5173`、`workers=1`、`retries=0`。credential は runner から環境変数で渡し、spec は二つの `browser.newContext()` を個別に作成・破棄する。

- [ ] **Step 1: 失敗する spec を書く。** UI の role/label を使い、Requester login → Draft 作成 → Request ID の URL 保持 → Submit → 別 context の Approver login → Pending から同じ ID の Approve → Requester の `Refresh request` → `Status: approved` と `request_approved` Audit を確認する。続けて `multi` login → `/organization-selection` → Org A/B の候補だけを表示 → Org A 選択 → Frontend `/` と Org A Member の session を確認する。骨子:

```ts
test("two users complete approval with one request id", async ({ browser }) => {
  const requester = await browser.newContext();
  const approver = await browser.newContext();
  try {
    const requestId = await createAndSubmit(requester);
    await approveFromPending(approver, requestId);
    const page = await requester.newPage();
    await page.goto(`/?requestId=${encodeURIComponent(requestId)}`);
    await page.getByRole("button", { name: "Refresh request" }).click();
    await expect(page.getByText("Status: approved")).toBeVisible();
    await expect(page.getByText("request_approved")).toBeVisible();
  } finally { await requester.close(); await approver.close(); }
});
```

- [ ] **Step 2: RED を確認する。** `pnpm run test:e2e` → spec/helper 未完成により FAIL。runner はこの段階でも専用 volume と子プロセスを片付ける。
- [ ] **Step 3: helper と assertion を仕上げる。** Keycloak UI は `username`/`password` label で操作し、UI の `Sign in`、`Create Draft`、`Submit`、`Approve`、`Refresh request` を使う。Request ID は `new URL(page.url()).searchParams.get("requestId")` から取得し、API response の body や test bypass から取らない。両 context の `/api/v1/session` が異なる `actor.memberId` を返すこと、redirect URL が Frontend origin であること、Org 選択は表示された候補 ID だけを送信することを確認する。コンソール error と page error は収集して redacted な期待外 error のみ失敗にする。`localStorage`/`sessionStorage` と URL に OIDC token、raw cookie、CSRF token がないことを確認し、trace/HAR/video と raw network header は保存しない。
- [ ] **Step 4: GREEN を確認する。** `pnpm run test:e2e` をクリーンな環境で 2 回実行し、各回 PASS と Compose volume 消滅を確認する。失敗が出た場合は原因を調べ、最も低い層へ回帰 test を追加する。
- [ ] **Step 5: commit。** `git add playwright.config.ts e2e/approval-flow.spec.ts && git commit -m "test: cover browser approval and organization selection"`。

### Task 4: 開発手順と完了 evidence

**Files:** Create `.env.example`, `docs/development/frontend-local-development.md`, `docs/development/frontend-completion-2026-09-24.md`; modify `docs/development/toolchain.md`.

**Interfaces:** 新しい開発者は文書の固定 version、`pnpm install --frozen-lockfile`、`pnpm run test:e2e` で専用 E2E を再現できる。手動開発用 `pnpm run dev`、API 起動、callback と二つの origin の説明は既存 `local-api.md` と一致させる。

- [ ] **Step 1: 文書の検証項目を先に固定する。** `.env.example` は値のない variable 名と loopback URL 例だけを記し、secret の例を置かない。`frontend-local-development.md` に前提 version、Docker、port、通常開発 DB と E2E 専用 DB の違い、Keycloak user と DB membership の対応、API/Vite 起動、cleanup、失敗時の診断を書く。`toolchain.md` に新 entrypoint と Compose 専用性を追加する。
- [ ] **Step 2: 完了前のチェックを実行する。** `pnpm install --frozen-lockfile`; `pnpm run format:check`; `pnpm run lint`; `pnpm run typecheck`; `pnpm test`; `pnpm run test:e2e:runner`; `pnpm run test:e2e`; `pnpm run build`; `pnpm run check:gofmt`; `GOTOOLCHAIN=go1.27.1 go test ./... -count=1`; `GOTOOLCHAIN=go1.27.1 go vet ./...`; `git diff --check`。各 code と結果を記録する。実 DB 統合も必要なら `pnpm run test:db` を専用 `compose.test.yaml` で実行する。
- [ ] **Step 3: 実測だけで完了記録を書く。** 変更ファイル、対応 FR/NFR、Accepted ADR/PDR、各検証コマンドの結果、実 browser の origin/session/audit 証拠、既知の制限、未決の deployment host を書く。失敗が残る間は「完了」と記さない。
- [ ] **Step 4: doc check を実行する。** `pnpm run format:check && pnpm run lint && git diff --check` → PASS。secret らしい値、未完の記述、環境固有の実 ID が文書や artifact に混じっていないことも確認する。
- [ ] **Step 5: commit。** `git add .env.example docs/development/frontend-local-development.md docs/development/frontend-completion-2026-09-24.md docs/development/toolchain.md && git commit -m "docs: record frontend browser verification"`。

## 自己レビューと仕様上の注意

- Task 1 は資源分離と cleanup、Task 2 は実 identity mapping、Task 3 は二者・複数所属の browser 証拠、Task 4 は再現手順と Decision traceability を担当する。新しい重要 Decision は不要。
- 親計画の「network fixture に raw cookie、OIDC token、CSRF がないことを assert」は、正規の認証通信では cookie/CSRF が request header に載り、OIDC code/token も provider 通信に現れるため、文字どおりには検証できない。本計画では **URL、storage、console、保存 artifact へ漏らさない**ことを検証し、raw network dump 自体を保存しない。親計画のこの表現は実装時に誤解がないよう別途修正する。
- 固定 port は既存 realm と local 設定に合わせるための制約。共有開発環境で衝突した場合は明示的に失敗し、既存環境を破棄しない。
