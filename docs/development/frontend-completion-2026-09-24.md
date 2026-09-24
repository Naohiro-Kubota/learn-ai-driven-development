# React Frontend Task 4 完了記録（2026-09-24）

## 対象と根拠

実 OIDC と別 browser context による Requester の Draft/Submit、Approver の Approve、Requester の Approved/Audit 確認、および複数 Organization の選択を、専用 PostgreSQL・Keycloak・Go API・Vite・Playwright の stack で検証した。実装対象は `compose.e2e.yaml`、`scripts/e2e-stack.mjs` とその test、`scripts/e2e-seed.mjs` とその test、`playwright.config.ts`、`e2e/approval-flow.spec.ts`、`package.json`。開発手順は `.env.example`、`frontend-local-development.md`、`toolchain.md` に記録した。

対応要求は FR-003/004/005/007/011/012 と NFR-001/002/003/006。依拠した Accepted Decision は ADR-001/003/005/006/007/009/011/013/015、PDR-001/002。API の正本は `api/openapi.yaml`。新しい本番依存関係や重要な Product/Architecture Decision は追加していない。

## 実測した検証

以下は Task 4 文書担当が repository root で実行した結果である。

| Command | Result |
| --- | --- |
| `pnpm install --frozen-lockfile` | PASS。lockfile の解決変更なし。 |
| `pnpm run format:check` | PASS（Task 3 修正前）。修正中の一時的な失敗は下記参照。最終差分でも再確認。 |
| `pnpm run lint` | PASS。 |
| `pnpm run typecheck` | PASS。 |
| `pnpm test` | PASS、6 files / 136 tests。 |
| `pnpm run test:e2e:runner` | PASS、15 tests。 |
| `pnpm run build` | PASS、Vite build 完了。 |
| `pnpm run check:gofmt` | PASS。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...` | PASS。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./cmd/... ./internal/application/... ./internal/auth/... ./internal/config/... ./internal/httpapi/... -count=1` | PASS。 |
| `GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db` | PASS。専用 PostgreSQL の起動、store test、container/network の削除を確認。 |
| `git diff --check` | PASS。 |

`GOTOOLCHAIN=go1.27.1 go test ./... -count=1` は、macOS の標準 Go cache が sandbox から書けず失敗した。`GOCACHE=/private/tmp/learn-ai-go-cache` を指定した再実行では、DB 統合 test が `TEST_DATABASE_URL is required` で失敗した。上表の非 DB package test と、DB URL を専用 Compose で注入する `test:db` の両方で各範囲を検証した。

文書担当の sandbox 内 `pnpm run test:e2e` は `Playwright command failed`、exit 1 となった。runner は秘密を含む browser 出力を表示しないため、この出力だけから原因を特定していない。この実行の固定 port は解放された。

Task 3 実装担当はブラウザ実行が許可された環境で `pnpm run test:e2e` を修正前に連続 2 回 PASS と報告した。さらに URL query の検証を追加した commit `040689f` の後、同コマンドを再実行し、exit 0 と `Playwright completed` を報告した。終了後、`approval-flow-e2e-*` の container/volume と一時 Playwright 出力 directory が残っていないことも確認した。この browser run は文書担当自身の実行結果ではなく、Task 3 担当から受け取った検証結果である。

## Browser が検証する境界

`e2e/approval-flow.spec.ts` は Requester と Approver を別の `browser.newContext()` に置き、`/api/v1/session` の Member ID が異なることを確認する。同じ Request ID を Draft 作成後から承認・再取得まで追跡し、最終 `Status: approved` と `request_approved` Audit を確認する。複数所属の利用者は提示された Organization A/B の候補から選び、選択前は未認証、選択後は選んだ Member の session で Frontend origin `/` に戻ることを確認する。Frontend URL、storage に認証 token や CSRF 値を置かず、trace/HAR/video と raw network dump は保存しない。

## 残る範囲

Static host/CDN、TLS termination、cache、deployment automation の選定は ADR-015 に従い未決のまま。cross-site 配信も ADR-015 の対象外であり、導入時に別の Decision が必要になる。E2E は固定 loopback port と Docker/Chromium 起動権限を要するため、共有環境では port 利用状況を確認して順番に実行する。
