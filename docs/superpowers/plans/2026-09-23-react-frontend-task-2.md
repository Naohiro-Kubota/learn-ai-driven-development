# React Frontend Task 2 実装計画

**Status: Completed (2026-09-23)。** 実装・検証・レビューの結果は [完了記録](../../development/react-frontend-task2-completion-2026-09-23.md) を参照。成果物は [PR #23](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/23) に含めた。実装時は Git 保護フックと隔離コピーでの作業により、下記の Task ごとの commit を行わず、Task 2 全体を `194e615` の単一 commit にまとめた。RED 確認は Task 2.1 で `pnpm test -- src/config.test.ts`、Task 2.2/2.3 で `pnpm exec vitest run ...` を実行した。

> **エージェント実行者向け:** 実装時は `superpowers:subagent-driven-development` または `superpowers:executing-plans` を使い、チェックボックス単位で進める。`AGENTS.md` 7.1 に従い、implementer に実装とテスト、reviewer に差分レビューを依頼する。

**Goal:** OpenAPI と一致する型付き API client と、session を起点に表示を切り替える最小の React application shell を作る。

**Architecture:** `src/config.ts` が API origin を検証し、`src/api/client.ts` が全 HTTP 通信、JSON/204/エラーの扱いを集約する。`src/app.tsx` は初回 session 取得と Sign in/認証済み/再試行可能な失敗の表示だけを担当する。Request 操作画面と Organization 選択画面は元計画の Task 3 で作る。

**Tech Stack:** Node.js 26.9.0、pnpm 12.5.1、React/React DOM 19.3.0、TypeScript 7.0.2、Vite 8.3.0、Vitest 5.0.1、Testing Library 16.3.3、jsdom 30.1.0、Biome 2.5.14。既存 lockfile の依存だけを使用する。

**Spec:** `docs/superpowers/plans/2026-09-23-react-frontend-implementation.md` の Task 2、`docs/superpowers/specs/2026-09-20-api-contract-design.md`、`docs/superpowers/specs/2026-09-21-multi-organization-oidc-identity-design.md`、`api/openapi.yaml`。

## 共通制約と着手条件

- 対応要求は FR-003/004/005/007/011/012 と NFR-001/002/003/004。依拠する Accepted Decision は ADR-001/003/006/007/011/012/013/015、PDR-001/002。
- Task 1 の `APP_FRONTEND_ORIGIN`、単一 origin CORS、固定 redirect は現在実装済み。Task 2 はその API と `api/openapi.yaml` を消費する。
- `VITE_API_ORIGIN` は絶対 HTTP(S) origin とし、userinfo、path、query、fragment を拒否する。production は HTTPS、loopback 開発のみ HTTP を許す。値を受け取る関数で環境依存をテストできるようにする。
- `fetch` は常に `credentials: "include"`。unsafe operation に現在の CSRF token を `X-CSRF-Token` で送り、JSON body があるときだけ `Content-Type: application/json` を付ける。GET には token と `Content-Type` を付けない。
- `ErrorResponse.code` だけを UI の分岐に使い、`message` を解析しない。401 と CSRF/409 の自動 mutation 再送はしない。
- cookie 値、OIDC token、CSRF token、role、Member ID を browser storage、URL、log に書かない。session と token は React memory state に置く。
- API DTO は OpenAPI の公開 field だけを写す。code generation、router、global state/UI component library、新たな依存を導入しない。
- Task 2 の scope は transport と shell。organization selection の 302 は `fetch` 内ではページ遷移を起こさないため、client は `redirect: "manual"` の `opaqueredirect` を成功として扱い、Task 3 の UI が固定 Frontend path `/` へ移動する。ブラウザでの cookie 成立は Task 4 の E2E で検証する（下記「継続前の確認」）。

## Review Focus

- 未設定、相対 URL、credential/path/query/fragment 付き API origin を拒否し、開発用 loopback HTTP だけ許す（Task 2.1）。
- GET/POST/PATCH の URL、body、header、cookie 設定を操作ごとに固定する（Task 2.2）。
- JSON 以外の成功/失敗、空 body、ネットワーク例外は parse 例外や偽の認証状態にせず、再試行可能な transport failure として扱う（Task 2.2/2.3）。
- `204` を JSON parse せず、`fieldErrors` を落とさず `ApiError` に保持する（Task 2.2）。
- session bootstrap の 401 は Sign in、他の失敗は Retry に分け、再取得時に古い結果が新しい結果を上書きしない（Task 2.3）。

## ファイル構成

| ファイル | 責務 |
| --- | --- |
| `index.html`, `src/main.tsx` | Vite の HTML/React entry |
| `vite.config.ts`, `src/test/setup.ts`, `tsconfig.json` | Vitest jsdom/Testing Library、React JSX/DOM の型チェック |
| `src/config.ts`, `src/config.test.ts` | `VITE_API_ORIGIN` の検証と正規化 |
| `src/api/types.ts` | OpenAPI と一致する DTO、入力、error code |
| `src/api/client.ts`, `src/api/client.test.ts` | URL、credential、CSRF、JSON、error と全 API method |
| `src/app.tsx`, `src/app.test.tsx` | session bootstrap と最小 shell |

### Task 2.1: Vite entry、テスト環境、API origin 設定

**Files:** Create `index.html`, `vite.config.ts`, `src/main.tsx`, `src/config.ts`, `src/config.test.ts`, `src/test/setup.ts`; modify `tsconfig.json`.

**Interfaces:** `export function parseApiOrigin(value: string | undefined): URL` を提供する。成功時は path が `/` で query/fragment が空の URL。失敗時は設定名を示す Error。`main.tsx` はこの段階では loading 表示を render し、Task 2.3 で client と App を接続する。

- [x] **Step 1: 設定の失敗するテストを書く。** `src/config.test.ts` で `https://api.example.test` と `http://127.0.0.1:8080` を受け入れ、`undefined`, `""`, `"/api"`, `"https://u@api.example.test"`, `"https://api.example.test/path"`, `"https://api.example.test/?x=1"`, `"https://api.example.test/#x"`, `"http://api.example.test"` を拒否する。

```ts
import { describe, expect, it } from "vitest";
import { parseApiOrigin } from "./config";

describe("parseApiOrigin", () => {
  it("accepts a production origin", () => {
    expect(parseApiOrigin("https://api.example.test").origin).toBe("https://api.example.test");
  });
  it.each([undefined, "", "/api", "https://u@api.example.test", "https://api.example.test/path", "https://api.example.test/?x=1", "https://api.example.test/#x", "http://api.example.test"])("rejects %s", (value) => {
    expect(() => parseApiOrigin(value)).toThrow(/VITE_API_ORIGIN/);
  });
});
```

- [x] **Step 2: `pnpm test -- src/config.test.ts` を実行し、module 未実装で FAIL を確認する。**
- [x] **Step 3: 設定と entry を実装する。** `new URL(value)` の前に空値と前後空白を拒否し、protocol は `https:`、または hostname が `localhost`/`127.0.0.1`/`[::1]` のときの `http:` だけ受け入れる。`username`、`password`、`pathname !== "/"`、`search`、`hash` を拒否する。`index.html` は `#root` を持ち `src/main.tsx` を module として読み込む。`main.tsx` はこの段階で `<main>Loading…</main>` だけを render する。`vite.config.ts` は `vitest/config` の `defineConfig` と React plugin を使い、`test.environment: "jsdom"` と `test.setupFiles: ["./src/test/setup.ts"]` を設定する。`setup.ts` は `@testing-library/jest-dom/vitest` を import する。`tsconfig.json` は `DOM`/`DOM.Iterable`、`jsx: "react-jsx"`、`types: ["vite/client", "vitest/globals"]` と必要な source/config file を含める。既存の `strict`, `moduleResolution: "bundler"`, `noEmit` を維持する。
- [x] **Step 4: `pnpm exec vitest run src/config.test.ts` と `pnpm run typecheck` を実行し PASS を確認する。** 後続 Task 2.2/2.3 の未作成 import を entry に先行追加しない。
- [x] **Step 5: この task のファイルを Task 2 全体の commit に含める。** `index.html`、`vite.config.ts`、`tsconfig.json`、`src/main.tsx`、`src/config.ts`、`src/config.test.ts`、`src/test/setup.ts` を `194e615` に含めた。

### Task 2.2: OpenAPI DTO と credentialed API client

**Files:** Create `src/api/types.ts`, `src/api/client.ts`, `src/api/client.test.ts`.

**Interfaces:**

```ts
export class ApiError extends Error {
  constructor(readonly status: number, readonly body: ErrorResponse) { super(body.message); }
}
export type ApiClient = ReturnType<typeof createApiClient>;
export function createApiClient(apiOrigin: URL, fetchFn: typeof fetch = fetch): {
  getSession(): Promise<Session>;
  getOrganizationSelection(): Promise<OrganizationSelection>;
  selectOrganization(memberId: string, csrfToken: string): Promise<void>;
  createRequest(input: CreateRequestInput, csrfToken: string): Promise<Request>;
  updateRequest(id: string, input: UpdateDraftRequestInput, csrfToken: string): Promise<Request>;
  submitRequest(id: string, expectedVersion: number, csrfToken: string): Promise<Request>;
  approveRequest(id: string, expectedVersion: number, csrfToken: string): Promise<Request>;
  getRequest(id: string): Promise<Request>;
  listPending(): Promise<Request[]>;
  listAuditEvents(id: string): Promise<AuditEvent[]>;
  logout(csrfToken: string): Promise<void>;
};
```

- [x] **Step 1: DTO と client の失敗するテストを書く。** `types.ts` は `Session`, `Actor`, `OrganizationSelection`, `CreateRequestInput`, `UpdateDraftRequestInput`, `Request`, `Approval`, `AuditEvent`, `ErrorResponse`, `FieldError` を `api/openapi.yaml` の required/optional/nullability に従って定義する。特に `Request.approval: Approval | null`、`AuditEvent.requestContent: {title: string; description: string} | null`、`ErrorResponse.fieldErrors?: FieldError[]`、`PendingRequestList.requests`、`AuditEventList.events` を守る。`client.test.ts` は全 11 method の HTTP method/path と成功 DTO、path segment の `encodeURIComponent(id)`、GET の header なし、mutation の JSON/CSRF/credentials、204、400 fieldErrors、401、403、409、非 JSON response、fetch rejection を表駆動で確認する。Organization 選択は `redirect: "manual"` と `opaqueredirect` 成功、通常の JSON error 応答を確認する。

```ts
it("sends the current CSRF token on create", async () => {
  const fetchFn = vi.fn().mockResolvedValue(new Response(JSON.stringify(draft), { status: 201, headers: { "Content-Type": "application/json" } }));
  const client = createApiClient(new URL("https://api.example.test"), fetchFn);
  await client.createRequest({ title: "Laptop", description: "" }, "csrf-current");
  expect(fetchFn).toHaveBeenCalledWith("https://api.example.test/api/v1/requests", expect.objectContaining({
    method: "POST", credentials: "include", headers: { "Content-Type": "application/json", "X-CSRF-Token": "csrf-current" },
    body: JSON.stringify({ title: "Laptop", description: "" }),
  }));
});
```

- [x] **Step 2: `pnpm exec vitest run src/api/client.test.ts` を実行し、DTO/client 未実装で FAIL を確認する。**
- [x] **Step 3: `types.ts` と client を実装する。** origin の `.origin` と定数 path から URL を作る。`id` は path segment として encode し、slash、`?`、`#` を path 構造へ混ぜない。private `request<T>` は全操作で `credentials: "include"`、unsafe operation で `X-CSRF-Token`、JSON body があるときだけ `Content-Type: application/json` を設定する。成功の 204 は body を読まず `void`、他の成功は JSON object を返す。`selectOrganization` だけ `redirect: "manual"` を指定し、`opaqueredirect` を成功として返す。HTTP error は JSON の `code`/`message` を検査して `ApiError(status, body)` を投げ、JSON でない response や malformed body は別の `TransportError`（固定の安全な文言）を投げる。fetch rejection はそのまま transport failure とし、mutation を自動再送しない。`listPending` は `.requests`、`listAuditEvents` は `.events` を返す。login は client method にせず `window.location.assign(new URL("/auth/oidc/login", apiOrigin))` で開始する。
- [x] **Step 4: `pnpm exec vitest run src/api/client.test.ts`、`pnpm run typecheck`、`pnpm run format:check`、`pnpm run lint` を実行し PASS を確認する。**
- [x] **Step 5: この task のファイルを Task 2 全体の commit に含める。** `src/api` を `194e615` に含めた。

### Task 2.3: Session bootstrap と最小 application shell

**Files:** Create `src/app.tsx`, `src/app.test.tsx`; modify `src/main.tsx`.

**Interfaces:** `export function App({ client, login }: { client: ApiClient; login: () => void }): ReactElement`。`main.tsx` は client と、固定 API login URL へ navigation する `login` callback を渡す。Task 3 は `App` の認証済み branch に画面を追加する。

- [x] **Step 1: shell の失敗するテストを書く。** `getSession` が未解決なら loading、成功なら認証済み shell、`ApiError(401, {code:"authentication_required", ...})` なら Sign in button、他の `ApiError`/network failure なら Retry buttonを確認する。Sign in は固定 `/auth/oidc/login` へ移り return URL を付けない。Retry は `getSession` を再実行する。遅い古い取得結果が新しい結果や unmount 後の state を上書きしないことを確認する。

```tsx
it("shows sign in after authentication_required", async () => {
  const client = { getSession: vi.fn().mockRejectedValue(new ApiError(401, { code: "authentication_required", message: "Authentication is required." })) } as unknown as ApiClient;
  const login = vi.fn();
  render(<App client={client} login={login} />);
  await userEvent.click(await screen.findByRole("button", { name: "Sign in" }));
  expect(login).toHaveBeenCalledOnce();
});
```

- [x] **Step 2: `pnpm exec vitest run src/app.test.tsx` を実行し、App 未実装で FAIL を確認する。**
- [x] **Step 3: 最小 shell を実装する。** `useEffect` で初回 `getSession` を起動し、cleanup で古い promise の反映を無効にする。state は `loading | authenticated(session) | unauthenticated | error` の判別可能 union にする。`ApiError.body.code === "authentication_required"` のときだけ unauthenticated、それ以外は generic error。session/CSRF はこの state 以外へ永続化・出力しない。authenticated branch には Task 3 の workspace を差し込める領域を置き、未実装の業務操作を表示しない。`main.tsx` で `createRoot(...).render(<App ... />)` を接続する。
- [x] **Step 4: `pnpm exec vitest run src/config.test.ts src/api/client.test.ts src/app.test.tsx`、`pnpm test`、`pnpm run typecheck`、`pnpm run format:check`、`pnpm run lint`、`pnpm run build`、`git diff --check` を実行し PASS を確認する。** `dist/` は commit しない。
- [x] **Step 5: この task のファイルを Task 2 全体の commit に含める。** `src/app.tsx`、`src/app.test.tsx`、`src/main.tsx` を `194e615` に含めた。

## 継続前の確認

- `POST /auth/oidc/organization-selection` は成功時 302 を返すが、cross-origin `fetch` の 302 は top-level browser navigation にならない。Task 2 client は `redirect: "manual"` の `opaqueredirect` を成功として返し、Task 3 UI は固定 Frontend path `/` へ明示的に移動する。この response から status/Location は読めないため、成功 cookie が保存されることと遷移後の session を Task 4 の実ブラウザ E2E で検証する。失敗した場合はコードだけで回避せず、API 契約と ADR-015 の変更要否を人間へ提示する。
- `getSession()` は読むたびに CSRF token を rotate する。Task 3 の UI は同時 bootstrap を避け、最新 token のみ使い、403 後も mutation を自動再送しない。

## 計画の自己レビュー

- **仕様カバレッジ:** Task 2.1 が entry/config、2.2 が全 DTO と全 endpoint/HTTP error、2.3 が初回 session と shell。元計画 Task 2 の要求を全て対応付けた。
- **境界:** UI workflow、organization selection 画面、Request/Audit の再取得、E2E は元計画 Task 3/4。新しい本番依存・重要 Decision は追加しない。
- **型とテスト:** `ApiClient` は Task 2.2 が定義し、Task 2.3 が注入を受ける。Review Focus の入力クラスは各 Task のテスト項目へ対応付けた。
