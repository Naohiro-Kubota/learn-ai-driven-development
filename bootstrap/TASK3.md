承認済みの「Go API実装計画」の Task 3 を進めてください。

まず、GitHub 操作にはブラウザを使わず GitHub CLI（gh）のみを使用してください。
開始時に以下を確認してください。

- `gh auth status` で GitHub CLI の認証状態を確認する
- `git fetch origin` を実行する
- `origin/develop` に Task 0〜2 の変更が取り込まれているか確認する
- 現在の作業ツリーに他者または未処理の変更がないか `git status --short` で確認する

Task 0〜2 の変更が `develop` に未マージの場合は、Task 3 の実装を開始しないでください。
`feature/TASK-001` から `develop` 向けの PR が未作成なら、GitHub CLI で作成してください。PR のレビュー・承認・マージを待ち、マージ済みであることを確認してから停止してください。

Task 0〜2 が `origin/develop` にマージ済みなら、`origin/develop` を起点として `codex/task-3-workflow-service` ブランチを作成し、隔離された worktree で作業してください。

必ず次を読んでから実装してください。

- `AGENTS.md`
- `docs/superpowers/plans/2026-09-20-go-api-implementation.md` の Task 3
- `docs/superpowers/specs/2026-09-20-api-contract-design.md`
- Accepted 状態の PDR-001、PDR-002
- Accepted 状態の ADR-002、ADR-004、ADR-005、ADR-006、ADR-012
- `skills/go-development/SKILL.md`
- 必要な superpowers skill instructions

Task 3 の範囲だけを実装してください。

- `internal/domain/request.go`
- `internal/domain/errors.go`
- `internal/application/requests/repository.go`
- `internal/application/requests/service.go`
- `internal/application/requests/service_test.go`

実装対象は、純粋な domain/application rule に限定します。

- `CreateDraft`、`UpdateDraft`、`Submit`、`Approve`、`Get`、`ListPending`、`ListAuditEvents`
- 型付きエラー: `ErrForbidden`、`ErrNotFound`、`ErrVersionConflict`、`ErrInvalidState`、`ErrApprovalRoutingUnavailable`
- Title は trim 後に空文字を拒否し、PDR-001 の長さ制約を適用する
- Description の空文字は許可する
- Draft 以外は更新不可とする
- Requester は自分の Draft だけを更新・Submit できる
- Approver は自分に割り当てられた Pending Request だけを Approve できる
- Submit / Approve は期待 version を検証する
- 成功した状態変更ごとに Audit Event をちょうど 1 件作成する
- 古い version や失敗操作では Audit Event を追加しない
- HTTP、SQL、OIDC の型や実装を domain/application package へ持ち込まない
- 新しい依存関係を追加しない
- PostgreSQL adapter、HTTP handler、OIDC/session、frontend は実装しない

TDD で進めてください。最初に table-driven の失敗する service test を追加し、失敗を確認してから最小実装を行ってください。

最低限、以下を含むテストを追加してください。

- Title の trim と空白だけの Title の拒否
- 空 Description
- Draft 限定更新
- 正しい Requester による Submit
- 既定 Approver 不在時の Submit 拒否
- 別 Requester による更新・Submit の拒否
- 未割当 Approver による Approve の拒否
- 古い expectedVersion の拒否と Audit Event 非作成
- Submit / Approve 成功時の Audit snapshot

実装後、少なくとも次を実行してください。

- `GOTOOLCHAIN=go1.27.1 go test ./internal/domain ./internal/application/requests -count=1`
- `GOTOOLCHAIN=go1.27.1 go vet ./...`
- `pnpm run check:gofmt`
- `pnpm run format:check`
- `pnpm run lint`
- `git diff --check`

進捗は `.superpowers/sdd/.../progress.md` に日本語で記録してください。

完了後は変更を 1 コミットにまとめてください。

`feat: add request workflow application service`

続けて GitHub CLI で `develop` 宛ての PR を作成してください。PR 本文には、変更内容、依拠した PDR/ADR、実行した検証結果、未解決リスクを日本語で記載してください。

PR を作成したら、レビュー・承認待ちで停止してください。PR のレビューコメントを受けた場合は、そのコメントだけを根拠に必要な修正を行い、同じ PR へ push してください。