# React Frontend Task 3 完了記録

Date: 2026-09-24
Scope: `docs/superpowers/plans/2026-09-23-react-frontend-task-3.md`

## 実装した振る舞い

- `/organization-selection` では選択候補と一時 CSRF token を取得し、候補 Member の選択を一度だけ送信する。成功後は固定の Frontend `/` へ移動する。失敗後は使用済みの選択を再送せず、Sign in から新しい認証を開始する。
- `/` では session に応じて Sign in、Requester の Draft 作成・編集、Request 詳細と Audit、Approver の割当済み Pending 一覧と Approve を表示する。選択中の不透明な Request ID は `?requestId=` だけに保存し、Back/Forward に追従する。
- Title は前後空白を除いて 1〜120 文字、Description は任意で 2,000 文字以下とする。編集は Requester 自身の Draft だけに表示し、Submit 時に Approver を選択しない。Audit には空 Description、Actor、時刻、承認先・承認 ID を含め、API の順序で表示する。
- 409 は Request と Audit を再取得し、CSRF 失敗は session を再取得する。どちらも mutation を自動再送せず、次の操作は人が行う。401 は session と表示中の Request を消す。結果不明の通信失敗では mutation の再送を促さず、明示的な状態確認を案内する。
- 二重操作、同じ Request の連続更新、Request 選択変更、利用者変更、遅延した logout 応答について、古い応答が新しい表示や session を上書きしないようにした。

## 変更ファイル

- App と entry: `src/app.tsx`、`src/app.test.tsx`、`src/main.tsx`、`src/styles.css`
- UI とテスト: `src/components/sign-in.tsx`、`organization-selection.tsx`、`organization-selection.test.tsx`、`request-workspace.tsx`、`request-workspace.test.tsx`、`request-form.tsx`、`request-detail.tsx`、`pending-list.tsx`、`audit-history.tsx`、`error-notice.tsx`
- 計画と開発文書: `docs/superpowers/plans/2026-09-23-react-frontend-task-3.md`、`docs/superpowers/plans/2026-09-23-react-frontend-implementation.md`、`docs/development/toolchain.md`、本記録

`package.json`、`pnpm-lock.yaml`、Go API、OpenAPI は変更していない。新しい依存関係や重要な Product/Architecture Decision は追加していない。`docs/product/requirements.md` は既存の FR-003/004/005/007/011/012 の記述と一致するため変更していない。

## Decision と要求の追跡

| 要求 | 実装と確認 |
| --- | --- |
| FR-003、PDR-001 | Title/Description の Draft 作成・編集と Submit。Title trim、長さ、Draft の編集制限を component test で確認。 |
| FR-004/005、PDR-002 | 既定 Approver への Submit と割当済み Pending の Approve。画面の表示条件と version 送信を確認。サーバー認可は既存 Go API が担当。 |
| FR-007 | Request の Audit 履歴、内容 snapshot、Actor/時刻、承認情報の表示を確認。 |
| FR-011 | `expectedVersion`、409 後の再取得、mutation の自動再送禁止と古い非同期応答の排除を確認。 |
| FR-012、NFR-003/004 | React/TypeScript の局所 state と Testing Library の component test。router、state/UI library を追加しない。 |
| NFR-001/002 | session/CSRF をメモリに保ち、401・CSRF・Organization 選択を code で扱う。Decision とこの記録へ追跡可能。 |

依拠した Accepted Decision は ADR-001/003/005/006/007/011/013/015 と PDR-001/002。Task 2 の `ApiClient` と `api/openapi.yaml` を契約の正本として用いた。

## 検証とレビュー

Task 3.1、3.2、3.3 は、それぞれ implementer が RED/GREEN を確認して実装し、reviewer の指摘を修正して再レビューを受けた。Task 3.3 の最後の logout 競合も再レビューで Approved になった。全体レビューでは Unicode 文字数、CSRF 更新の競合、作成済み Draft の導線、Request/Audit 読込の失敗処理を修正し、再レビューは **Ready to merge: Yes** で終了した。

| Command | Result |
| --- | --- |
| `pnpm test` | PASS、6 files、136 tests |
| `pnpm run typecheck` | PASS |
| `pnpm run format:check` | PASS |
| `pnpm run lint` | PASS |
| `pnpm run build` | PASS |
| `git diff --check` | PASS |

## 残る制約と次の作業

- Organization 名は選択候補には表示するが、Request/Session DTO に Organization 名がないため Request 詳細からは取得できない。契約を拡張せず、既存の OpenAPI に従った。
- Organization 選択の cross-origin `opaqueredirect` 後に session cookie が実ブラウザで維持されるか、Requester と Approver の二者フローが成立するかは Task 4 の Playwright E2E で検証する。Task 3 の jsdom test はこの証拠にならない。
- Static hosting/CDN 製品、TLS termination、cache、deployment automation は ADR-015 に従い未決定のまま残る。
