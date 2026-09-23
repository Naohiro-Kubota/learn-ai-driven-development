# React Frontend Task 2 完了記録

Date: 2026-09-23
Scope: `docs/superpowers/plans/2026-09-23-react-frontend-task-2.md`

## 実装と境界

Vite/React entry、`VITE_API_ORIGIN` の検証、OpenAPI に沿う TypeScript DTO と credentialed API client、session を初回取得する application shell を追加した。すべての API `fetch` は cookie を送信し、unsafe operation は現在の CSRF token を送る。HTTP error は機械可読な `code` を保持し、204 は logout だけで成功として扱う。Sign in は固定の API `/auth/oidc/login` に移動し、return URL や session 情報を付けない。

`getSession` の必須フィールドと一覧 envelope を検証し、不正な成功 JSON は `TransportError` にする。Request ID の `.` と `..` は path 正規化で別 endpoint を指すため送信前に拒否し、OpenAPI と API 設計文書の `requestId` 制約を同期した。この修正は既存の ID 発行形式や業務規則を変更しない。

## 変更ファイル

- Frontend entry/config/test: `index.html`、`vite.config.ts`、`tsconfig.json`、`src/main.tsx`、`src/config.ts`、`src/config.test.ts`、`src/test/setup.ts`
- API boundary/test: `src/api/types.ts`、`src/api/client.ts`、`src/api/client.test.ts`
- Session shell/test: `src/app.tsx`、`src/app.test.tsx`
- 契約と文書: `api/openapi.yaml`、`docs/superpowers/specs/2026-09-20-api-contract-design.md`、`docs/development/toolchain.md`、`docs/superpowers/plans/2026-09-23-react-frontend-task-2.md`、本記録

新しい本番・開発依存関係は追加していない。`package.json` と `pnpm-lock.yaml` は変更していない。

## 要求と Decision の追跡

- FR-012、NFR-004: React/Vite の最小 shell と独立した API boundary。
- FR-003/004/005/007/011: Draft、Submit、Approve、Pending、Audit 向けの型付き transport と expectedVersion/CSRF の送信。業務画面は後続 Task 3 の範囲。
- NFR-001: credentialed CORS 前提の通信、CSRF header、固定 login URL、秘密値の React memory state 保持。
- NFR-002/003: 本記録、OpenAPI 契約同期、Vitest/Testing Library と既存 Go テストでの検証。
- Accepted ADR-001/003/006/007/011/012/013/015 および PDR-001/002 に依拠する。新たな重要な Product/Architecture Decision は不要だった。

## 検証結果

Frontend の全検証は Task 2 feature branch の実 Git checkout で実行した。Go の検証は同じ変更内容を持つ書き込み可能な隔離コピーで実行した。

PR 用の `codex/task2-react-transport-shell-pr` ブランチへ同じ 17 ファイルを移し、全ファイルの SHA-1 一致を確認した。この checkout では npm registry への DNS 接続ができず、`pnpm install --frozen-lockfile` の再実行は完了しなかった。既に同じ lockfile で導入・検証済みの依存を隔離作業ツリーからコピーし、`./node_modules/.bin/vitest run`（73/73）、`./node_modules/.bin/tsc -b --pretty false`、`./node_modules/.bin/biome format .`、`./node_modules/.bin/biome lint .`、`./node_modules/.bin/vite build`、`node scripts/verify-openapi.mjs` を再実行してすべて PASS した。

| Command | Result |
| --- | --- |
| `pnpm install --frozen-lockfile` | PASS、既存 lockfile のまま |
| `pnpm test` | PASS、73/73、実 Git checkout で実行 |
| `pnpm run typecheck` | PASS |
| `pnpm run format:check` | PASS |
| `pnpm run lint` | PASS |
| `pnpm run build` | PASS |
| `pnpm run verify:openapi` | PASS |
| `git diff --check` | PASS |
| `node --test scripts/verify-openapi.test.mjs` | PASS、11 passed、1 skipped |
| `GOCACHE=/private/tmp/learn-ai-task2-go-cache GOMODCACHE=/private/tmp/learn-ai-task2-go-modcache GOTOOLCHAIN=go1.27.1 go test ./cmd/api ./cmd/migrate-local ./internal/application/requests ./internal/auth ./internal/config ./internal/domain ./internal/httpapi -count=1` | PASS |
| `GOCACHE=/private/tmp/learn-ai-task2-go-cache GOMODCACHE=/private/tmp/learn-ai-task2-go-modcache GOTOOLCHAIN=go1.27.1 go vet ./...` | PASS |
| `GOCACHE=/private/tmp/learn-ai-task2-go-cache GOMODCACHE=/private/tmp/learn-ai-task2-go-modcache pnpm run test:db` | PASS、一時 PostgreSQL container/volume を破棄済み |

Task 2.1/2.2/2.3 は implementer の実装後に reviewer が個別に確認し、指摘の修正と再レビューを終えた。最終の全体レビューでは API 契約外の成功応答を受け入れる問題が見つかり、修正と再レビューを終えた。最終レビューの SPEC/QUALITY はいずれも PASS。GitHub PR は別途確認する。

## 残る検証と後続作業

- `skipLibCheck: true` は既存の第三者 `.d.ts` と不足する Node 型定義の衝突を避けるために用いた。アプリの source と `vite.config.ts` は型チェック対象だが、宣言ファイル自体の検査範囲は限定される。
- Organization 選択の `opaqueredirect` 後の cookie 保存と Frontend `/` への遷移は、後続 Task 3/4 の UI と実ブラウザ E2E で確認する。Task 2 は選択 API の transport までを提供する。
- Request 操作画面、Organization 選択画面、二者 E2E は元の React Frontend 実装計画の Task 3/4 に残る。
