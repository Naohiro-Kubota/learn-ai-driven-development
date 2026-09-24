# React Frontend実装計画

> **エージェント実行者向け:** `superpowers:subagent-driven-development`（推奨）または`superpowers:executing-plans`を使い、タスク単位で実行すること。進捗は`- [ ]`で管理する。

**目的:** 同一site上の別originで配信されるReact UIから、OIDCログイン、Organization選択、Draft作成・Submit、Pending承認、Approved/Audit確認を安全に完了できるようにする。

**アーキテクチャ:** Go APIは設定済みの単一Frontend originだけをcredentialed CORSで許可し、OIDC完了時はそのoriginの固定UI pathへredirectする。Reactは`VITE_API_ORIGIN`を唯一のAPI base URLとし、すべての`fetch`に`credentials: "include"`を指定する。sessionとCSRF tokenはReact stateだけに保持し、型付きAPI clientを経由して画面へ渡す。

**技術スタック:** Go 1.27.1の`net/http`、React 19.3.0、React DOM 19.3.0、TypeScript 7.0.2、Vite 8.3.0、Vitest 5.0.1、Testing Library 16.3.3、jsdom 30.1.0、Playwright 1.63.0、Biome 2.5.14。追加dependencyはない。

**仕様:** `docs/superpowers/specs/2026-09-20-api-contract-design.md`、`docs/superpowers/specs/2026-09-21-multi-organization-oidc-identity-design.md`、`api/openapi.yaml`、`docs/decisions/architecture/ADR-015-frontend-delivery-topology.md`

## 共通制約

- ADR-015 Option Cに従い、Frontend/API originは異なってよいが、初回Sliceでは同一scheme・同一registrable domainの同一siteに限定する。cross-site cookie、`SameSite=None`、複数Frontend originは実装しない。
- APIは`APP_FRONTEND_ORIGIN`と完全一致する1 originだけをCORS許可する。`Access-Control-Allow-Origin: *`、origin reflection、複数originは使わない。許可methodは`GET`、`POST`、`PATCH`、`OPTIONS`、request headerは`Content-Type`と`X-CSRF-Token`だけとする。
- `APP_FRONTEND_ORIGIN`はabsolute HTTP(S) originだけを受け入れ、path/query/fragment/userinfoを拒否する。OIDC callbackとOrganization選択成功時のredirect先はこの設定値と固定pathだけから作り、return URLを受け入れない。
- ADR-011を維持し、cookieは`HttpOnly`、opaque、productionでは`Secure`かつ`SameSite=Lax`とする。OIDC token、cookie値、CSRF token、role、Member IDをlocalStorage/sessionStorage、URL query、logへ保存しない。
- unsafe operationには最新CSRF tokenを`X-CSRF-Token`で送る。`409 version_conflict`ではmutationを自動再送せず、RequestとAuditを再取得して人が次を選ぶ。
- OpenAPI 3.1を契約の正本とする。Reject、Cancel、検索・一覧、router/state-library/UI component library、新しいproduct behaviorを追加しない。

## レビュー重点項目

- 非許可Origin/preflightにCORS credential headerを返さず、handler、session、mutationを実行しない（Task 1）。
- callback/selection redirectは固定Frontend pathだけを使い、query/bodyの攻撃者入力を反映しない（Task 1）。
- 401、403 CSRF、409 conflict、fieldErrorsではmessageを解析せず、機械可読な`code`だけでUIを分岐する（Task 2、3）。
- token rotation後の古いCSRF tokenを自動再送せず、session再取得後も明示的な再操作を求める（Task 2、3）。
- Request IDを失わず、RequesterとApproverの別browser contextで同じRequestのApproved/Auditを確認できる（Task 3、4）。

## ファイル構成

- `internal/config/config.go` / `config_test.go` — Frontend origin validation。
- `internal/httpapi/cors.go` / `cors_test.go` — 単一originのcredentialed CORSとpreflight。
- `internal/httpapi/router.go`、`auth_handlers.go`、handler test、`cmd/api/main_test.go`、`api/openapi.yaml` — CORSと固定redirectの契約・wiring。
- `index.html`、`vite.config.ts`、`src/main.tsx`、`src/app.tsx`、`src/styles.css` — Vite entry、navigation、画面layout。
- `src/config.ts` / test — API origin validation。
- `src/api/types.ts`、`client.ts`、test — OpenAPI DTO、credentialed fetch、typed error、CSRF header。
- `src/components/*.tsx` とtest — sign-in、Organization選択、Request form/detail、pending list、audit/error。
- `playwright.config.ts`、`e2e/approval-flow.spec.ts`、`scripts/e2e-stack.mjs`、`compose.e2e.yaml`、`.env.example` — isolated E2E harness。

### Task 1: Cross-origin API transportと固定Frontend redirectを追加する

**完了（2026-09-23）:** [PR #21](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/21)を`develop`へマージ済み。以下は実装前に書かれた当初計画で、主要な設定・CORS・redirectはTask 1着手時点ですでに存在していた。実際のred/green、差分、検証、レビューは[Task 1補完計画](2026-09-23-react-frontend-task1-completion.md)と[完了記録](../../development/react-frontend-task1-completion-2026-09-23.md)を正本とする。当初計画の未チェック項目はこの履歴を保つため変更しない。

**ファイル:**
- 作成: `internal/httpapi/cors.go`、`internal/httpapi/cors_test.go`
- 変更: `internal/config/config.go`、`internal/config/config_test.go`、`internal/httpapi/router.go`、`internal/httpapi/auth_handlers.go`、`internal/httpapi/auth_handlers_test.go`、`cmd/api/main_test.go`、`api/openapi.yaml`、`docs/development/toolchain.md`

**Interface:** `config.Config.FrontendOrigin string`、`NewCORS(allowedOrigin string, next http.Handler) http.Handler`、`frontendLocation(frontendOrigin, path string) string`を提供する。`NewRouter`はCORS wrapperを返し、許可された`OPTIONS`はroute dispatch前に`204`を返す。

- [ ] **Step 1: 失敗するconfiguration、CORS、redirect contract testを書く**

```go
func TestLoadRejectsFrontendOriginWithPath(t *testing.T) {
	_, err := Load(lookupWith("APP_FRONTEND_ORIGIN", "https://app.example.test/path"))
	if err == nil { t.Fatal("expected origin validation error") }
}

func TestCORSPreflightAllowsOnlyConfiguredOrigin(t *testing.T) {
	h := NewCORS("https://app.example.test", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("handler must not run for preflight")
	}))
	r := httptest.NewRequest(http.MethodOptions, "/api/v1/requests", nil)
	r.Header.Set("Origin", "https://app.example.test")
	r.Header.Set("Access-Control-Request-Method", "POST")
	r.Header.Set("Access-Control-Request-Headers", "content-type, x-csrf-token")
	rr := httptest.NewRecorder(); h.ServeHTTP(rr, r)
	if rr.Code != http.StatusNoContent { t.Fatalf("status = %d", rr.Code) }
}
```

非許可origin/method/header、malformedまたは複数`Origin` headerにはCORS headerを返さないこと、許可originの`401`/`403`にはCORS headerを返すこと、callback/selection成功時は攻撃者queryがあっても固定Frontend pathへredirectすることをtestする。

- [ ] **Step 2: focused testを実行して失敗を確認する**

Run:

```bash
GOTOOLCHAIN=go1.27.1 go test ./internal/config ./internal/httpapi ./cmd/api -run 'Test(Load.*Frontend|CORS|Callback|OrganizationSelection)' -count=1
```

期待結果: `APP_FRONTEND_ORIGIN`、CORS middleware、absolute redirectが未実装のためFAIL。

- [ ] **Step 3: 最小の厳格なtransport boundaryを実装する**

`APP_ALLOWED_ORIGIN`を`APP_FRONTEND_ORIGIN`へ置換し、`net/url`でHTTP(S)、host、空または`/`だけのpathを検証する。query、fragment、userinfo、whitespaceを拒否し、loopback development制約はFrontend originとAPI listen addressで検査する。

CORSは標準libraryだけのwrapperにする。既存`Vary`を残して`Origin`を加え、許可originだけにexact allow-origin/credentialsを返す。列挙済みmethod/headerだけのpreflightを処理し、OPTIONSでは認証、CSRF発行、body parse、wrapped handler呼出しをしない。mux外側に置き、許可originからerror responseも読めるようにする。redirectはvalidated `FrontendOrigin`とconstant pathだけから構築し、OpenAPIの`302 Location`説明も更新する。

- [ ] **Step 4: focused testとAPI regression checkを実行する**

```bash
GOTOOLCHAIN=go1.27.1 go test ./internal/config ./internal/httpapi ./cmd/api -count=1
GOTOOLCHAIN=go1.27.1 go test ./... -count=1
```

期待結果: PASS。

- [ ] **Step 5: このタスクをcommitする**

```bash
git add api/openapi.yaml internal/config internal/httpapi/cors.go internal/httpapi/cors_test.go internal/httpapi/router.go internal/httpapi/auth_handlers.go internal/httpapi/auth_handlers_test.go cmd/api/main_test.go docs/development/toolchain.md
git commit -m "feat: allow the configured frontend origin"
```

### Task 2: 型付きReact transport layerとapplication shellを作る

**ファイル:**
- 作成: `index.html`、`vite.config.ts`、`src/main.tsx`、`src/config.ts`、`src/config.test.ts`、`src/api/types.ts`、`src/api/client.ts`、`src/api/client.test.ts`、`src/app.tsx`、`src/app.test.tsx`、`src/test/setup.ts`
- 変更: `tsconfig.json`

**Interface:**

```ts
export class ApiError extends Error { constructor(readonly status: number, readonly body: ErrorResponse) }
export type ApiClient = ReturnType<typeof createApiClient>;
export function createApiClient(apiOrigin: URL, fetchFn?: typeof fetch): {
  getSession(): Promise<Session>; getOrganizationSelection(): Promise<OrganizationSelection>;
  selectOrganization(memberId: string, csrfToken: string): Promise<void>;
  createRequest(input: CreateRequestInput, csrfToken: string): Promise<Request>;
  updateRequest(id: string, input: UpdateDraftRequestInput, csrfToken: string): Promise<Request>;
  submitRequest(id: string, expectedVersion: number, csrfToken: string): Promise<Request>;
  approveRequest(id: string, expectedVersion: number, csrfToken: string): Promise<Request>;
  getRequest(id: string): Promise<Request>; listPending(): Promise<Request[]>;
  listAuditEvents(id: string): Promise<AuditEvent[]>; logout(csrfToken: string): Promise<void>;
};
```

- [ ] **Step 1: 失敗するconfiguration/client/shell testを書く**

```ts
it("sends credentials and CSRF only for an unsafe request", async () => {
  const fetchFn = vi.fn().mockResolvedValue(jsonResponse(201, request));
  const client = createApiClient(new URL("https://api.example.test"), fetchFn);
  await client.createRequest({ title: "Laptop", description: "" }, "csrf-current");
  expect(fetchFn).toHaveBeenCalledWith(
    "https://api.example.test/api/v1/requests",
    expect.objectContaining({ credentials: "include", headers: expect.objectContaining({ "X-CSRF-Token": "csrf-current" }) }),
  );
});
```

不正/未設定`VITE_API_ORIGIN`、non-JSON network response、`fieldErrors`、`204` logout、CSRFなしのGET、最新tokenだけを持つPOST/PATCH、return URLなしのlogin navigation、`authentication_required`時のSign in表示をtestする。

- [ ] **Step 2: focused testを実行して失敗を確認する**

Run: `pnpm test -- src/config.test.ts src/api/client.test.ts src/app.test.tsx`

期待結果: Vite entry、transport type/client、React shellが未実装のためFAIL。

- [ ] **Step 3: shellとAPI boundaryを実装する**

router/state/UI library/generated clientなしにVite React entryとtest environmentを追加する。`config.ts`はabsolute HTTP(S)でcredentials/query/fragmentなしの`VITE_API_ORIGIN`だけを受け入れ、trailing slashを正規化する。`types.ts`はOpenAPI public fieldだけをmirrorする。

`client.ts`はvalidated API originからURLを作り、全requestへ`credentials: "include"`を指定する。unsafe JSON requestだけに`Content-Type`と`X-CSRF-Token`を付与し、`ErrorResponse.code`だけを分岐に用いる。`App`は最初に`getSession`を呼び、`authentication_required`をSign in state、成功をauthenticated state、その他をretry可能なgeneric errorとする。raw CSRFはReact stateだけに置く。

- [ ] **Step 4: focused testとstatic checkを実行する**

```bash
pnpm test -- src/config.test.ts src/api/client.test.ts src/app.test.tsx
pnpm run typecheck
pnpm run format:check
pnpm run lint
```

期待結果: PASS。package/lockfile変更なし。

- [ ] **Step 5: このタスクをcommitする**

```bash
git add index.html vite.config.ts tsconfig.json src/main.tsx src/config.ts src/config.test.ts src/api src/app.tsx src/app.test.tsx src/test/setup.ts
git commit -m "feat: add typed React API client"
```

### Task 3: Vertical Slice画面とrecovery behaviorを実装する

**完了（2026-09-24）:** 実装とタスク別・全体レビューを完了した。実際の差分、RED/GREEN、検証結果、残るTask 4 browser E2Eは[Task 3詳細計画](2026-09-23-react-frontend-task-3.md)と[完了記録](../../development/react-frontend-task3-completion-2026-09-24.md)を正本とする。以下の当初計画の未チェック項目は履歴として残す。

**ファイル:**
- 作成: `src/components/sign-in.tsx`、`organization-selection.tsx`、`request-workspace.tsx`、`request-form.tsx`、`request-detail.tsx`、`pending-list.tsx`、`audit-history.tsx`、`error-notice.tsx`、`request-workspace.test.tsx`、`organization-selection.test.tsx`、`src/styles.css`
- 変更: `src/app.tsx`

**Interface:** `App`は`{ session, requestId, notice }`を管理し、`requestId`は`?requestId=<opaque id>`だけから得る。`RequestWorkspace`はCreate/Update/Submit/Approve後またはconflict後にRequestとAuditを再取得する。`OrganizationSelection`はcandidateと一時CSRF tokenだけを使い、成功後はserver redirectに従う。

- [ ] **Step 1: 全user-visible stateの失敗するcomponent testを書く**

```tsx
it("refetches instead of retrying a stale Submit", async () => {
  const client = clientWith({ submitRequest: rejectApi("version_conflict"), getRequest: resolve(newerPending), listAuditEvents: resolve(events) });
  render(<RequestWorkspace client={client} session={session} requestId="request-1" onRequestIdChange={vi.fn()} />);
  await userEvent.click(await screen.findByRole("button", { name: "Submit" }));
  expect(client.getRequest).toHaveBeenCalledWith("request-1");
  expect(client.submitRequest).toHaveBeenCalledTimes(1);
});
```

空白Titleの`fieldErrors`、Draftだけの編集/Submit、Pending/Approvedのread-only、assigned ApproverだけのApprove、SubmitでApproverを選ばないこと、Approver roleだけのPending一覧、空Descriptionを含むAudit、CSRF failure後の明示的retry、401/logout後のSign inをtestする。

- [ ] **Step 2: focused testを実行して失敗を確認する**

Run: `pnpm test -- src/components/request-workspace.test.tsx src/components/organization-selection.test.tsx`

期待結果: screen componentとmutation recoveryが未実装のためFAIL。

- [ ] **Step 3: 最小UI behaviorを実装する**

`/organization-selection`ではcandidateを一度取得し、organization nameをbuttonとして表示して、そのCSRF tokenを一度だけselectionに使う。`/`ではsessionをbootstrapし、未認証ならSign in、認証済みならdraft formとApprover role向けPending listを表示する。

create成功時はURLを`?requestId=<encodeURIComponent(request.id)>`へ更新してdetail/auditを読む。Draft Update、Submit、Approve成功時はresponseを表示しAuditを更新する。mutationにはcurrent `request.version`を渡す。`version_conflict`/`invalid_state`は一度だけre-fetch、`csrf_validation_failed`はsession refresh後に明示的retry、`authentication_required`はsession/tokenをclearしてSign inへ戻す。server contentはtextとして表示する。

- [ ] **Step 4: UI suiteとbuildを実行する**

```bash
pnpm test -- src/app.test.tsx src/components/request-workspace.test.tsx src/components/organization-selection.test.tsx
pnpm run build
```

期待結果: PASS。`dist/`はcommitしない。

- [ ] **Step 5: このタスクをcommitする**

```bash
git add src/app.tsx src/styles.css src/components
git commit -m "feat: add request approval workflow screens"
```

### Task 4: 再現可能なbrowser E2Eとdeveloper evidenceを追加する

**完了（2026-09-24）:** 実装、レビュー、実ブラウザ E2E と開発者向け文書を完了した。実際の差分、RED/GREEN、検証結果、環境制約は [Task 4 詳細計画](2026-09-24-react-frontend-task-4.md) と [完了記録](../../development/frontend-completion-2026-09-24.md) を正本とする。以下の当初計画の未チェック項目と 2026-09-23 の予定ファイル名は履歴として残す。正規の認証通信には cookie や CSRF header が載るため、「network fixture に raw cookie 等がない」という当初表現は、保存 artifact・URL・storage・console への漏出を防ぐ検証として具体化した。

**ファイル:**
- 作成: `playwright.config.ts`、`e2e/approval-flow.spec.ts`、`compose.e2e.yaml`、`scripts/e2e-stack.mjs`、`.env.example`、`docs/development/frontend-local-development.md`、`docs/development/frontend-completion-2026-09-23.md`
- 変更: `package.json`、`docs/development/toolchain.md`

**Interface:** `pnpm run test:e2e`はisolated PostgreSQL、Keycloak 26.7.4、Go API、Viteを起動し、Requester/Approver mappingとdefault Approverをprovisionして、finallyで破棄する。Playwrightは別browser contextを使い、test headerやserver-side authentication bypassを使わない。

- [ ] **Step 1: 失敗する二者E2E specを書く**

```ts
test("requester submits, assigned approver approves, requester reads audit", async ({ browser }) => {
  const requester = await browser.newContext();
  const approver = await browser.newContext();
  const requestId = await createAndSubmit(requester);
  await approveAssignedRequest(approver, requestId);
  await requesterPage(requester, requestId).getByText("approved").waitFor();
});
```

複数membership loginが`/organization-selection`へ到達し、候補Memberだけを選べることを追加する。redirect後のbrowser addressがFrontend originであること、console/network fixtureにraw cookie、OIDC token、CSRFがないことをassertする。

- [ ] **Step 2: E2E commandを実行して失敗を確認する**

Run: `pnpm run test:e2e`

期待結果: isolated stack、provisioned identity、Playwright specが未実装のためFAIL。

- [ ] **Step 3: isolated stack、E2E、運用文書を実装する**

`compose.e2e.yaml`はpinned Keycloak image/digestとPostgreSQLを使い、Frontend/APIをloopback same-siteだがdistinct originとして設定する。`e2e-stack.mjs`はrepository外のtemporary runtime configを作り、`APP_FRONTEND_ORIGIN`、OIDC redirect URI、database URLを明示して起動する。bounded health check、failure時artifact保存、`finally`でのresource cleanupを実装し、production test-auth routeやActor header信頼を追加しない。

local環境変数、`pnpm run dev`、Go API command、OIDC callback URL、`VITE_API_ORIGIN`と`APP_FRONTEND_ORIGIN`の違い、same-site制限を文書化する。全check成功後だけcompletion evidenceに変更file、FR-003/004/005/007/011/012、ADR-001/003/005/006/007/011/013/015、PDR-001/002、未決のdeployment hostを記録する。

- [ ] **Step 4: 全verificationを実行する**

```bash
pnpm install --frozen-lockfile
pnpm run format:check
pnpm run lint
pnpm run typecheck
pnpm test
pnpm run test:e2e
pnpm run check:gofmt
GOTOOLCHAIN=go1.27.1 go test ./... -count=1
GOTOOLCHAIN=go1.27.1 go vet ./...
git diff --check
```

期待結果: PASS。E2Eはdisposable stateを使う。

- [ ] **Step 5: このタスクをcommitする**

```bash
git add playwright.config.ts e2e compose.e2e.yaml scripts/e2e-stack.mjs .env.example package.json docs/development/toolchain.md docs/development/frontend-local-development.md docs/development/frontend-completion-2026-09-23.md
git commit -m "test: verify the cross-origin approval flow"
```

## 計画の自己レビュー

- **仕様カバレッジ:** Task 1はADR-015のCORS/fixed redirect契約、Task 2は認証済みOpenAPI transport、Task 3はlogin、organization selection、Draft、Submit、Pending、Approve、Approved、Audit、validation、CSRF、conflict recovery、Task 4は二者browser flowとtraceabilityを扱う。
- **Decision確認:** framework、router、state、UI、runtime、test dependencyは追加しない。Task 1のAPI contract変更はAccepted ADR-015により必要であり、OpenAPIを先に更新する。
- **型の整合性:** `ApiClient`をcomponent-facing API boundaryとし、全mutationは`csrfToken`、Request mutationはcurrent `expectedVersion`を受ける。`requestId`はFrontend URL queryだけで保持する。
- **placeholder確認:** 未指定のimplementation、dependency、endpoint、retry rule、test commandはない。static hosting/CDN vendorはADR-015によりscope外である。
