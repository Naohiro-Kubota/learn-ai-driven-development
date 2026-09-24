# Toolchain and dependency baseline

Status: Accepted ADR configuration

この文書は、Accepted ADR-001、ADR-002、ADR-004、ADR-006、ADR-007、ADR-009、ADR-010、ADR-012、ADR-015、ADR-017に基づく初回Sliceの再現可能なtoolchain基準です。versionは2026-09-21時点の最新安定版として固定します。更新は依存関係ポリシーとADR-007のレビュー手順に従って行います。

## Toolchain

| Component | Fixed version | Pin location |
| --- | --- | --- |
| Go | 1.27.1 | `.go-version`, `go.mod` `go` directive |
| Node.js | 26.9.0 | `.node-version`, `package.json` `engines` |
| pnpm | 12.5.1 | `package.json` `packageManager` / `engines` |
| TypeScript | 7.0.2 | `package.json`, `pnpm-lock.yaml` |
| Biome | 2.5.14 | `package.json`, `pnpm-lock.yaml`, `biome.json` |
| Keycloak | 26.7.4 | `quay.io/keycloak/keycloak:26.7.4@sha256:82a77884f3af238beab1e7afd63b5f530e1b5c0590bd7aa60b40a40463e29b2c` |

Node.js 26.9.0は現時点の最新stable Current releaseである。LTSを優先する運用要件が生じた場合は、Node.js 24系への変更を依存関係更新として評価する。

## Selected libraries

| Area | Library | Fixed version | Authority |
| --- | --- | --- | --- |
| UI | React / React DOM | 19.3.0 | ADR-001 |
| Build | Vite / React plugin | 8.3.0 / 6.1.1 | ADR-001 |
| UI testing | Vitest / Testing Library / jsdom | 5.0.1 / 16.3.3 / 7.0.1 / 30.1.0 | ADR-006 |
| Browser E2E | Playwright | 1.63.0 | ADR-006 |
| TypeScript format/lint | Biome | 2.5.14 | ADR-012 |
| OpenAPI contract validation only | yaml | 2.9.1 | Task 8.2 |
| PostgreSQL | pgx stdlib driver | 5.11.0 | ADR-004 |
| Migration | golang-migrate | 4.20.1 | ADR-004 |
| OIDC provider | Keycloak | 26.7.4 | ADR-009 |
| OIDC client/token verification | coreos/go-oidc / x/oauth2 | 3.21.0 / 0.37.0 | ADR-010 |

Go sourceはADR-012に従い`gofmt`でformatし、`go vet ./...`で静的解析する。TypeScript/TSX/JavaScript/JSONはBiomeでformat/lintする。CIではいずれの検査もファイルを書き換えない。

## Installation policy

- Node.js、pnpm、Goは固定versionを使用する。既存のローカルversionが異なる場合は、固定versionを導入してから実行する。
- CIは`pnpm install --frozen-lockfile`のみを使用する。`pnpm-lock.yaml`の更新は、`package.json`と`pnpm-workspace.yaml`を含む同一レビューで行う。
- pnpmの`minimumReleaseAge`、`blockExoticSubdeps`、`strictStorePkgContentCheck`、`strictDepBuilds`、`allowBuilds`は`pnpm-workspace.yaml`を正本とする。
- Go moduleは`go.mod`と`go.sum`をコミットする。依存更新はGo module versionと間接依存の差分をレビューする。
- Keycloak imageはtagだけで運用せず、provisioning時に対応するcontainer digestを記録する。development modeはローカル/E2E限定である。
- PostgreSQL migration統合テストは`pnpm run test:db`を使用する。これは実行ごとにDBパスワードを生成し、`compose.test.yaml`で一時的なPostgreSQL 17.11 containerを起動して`TEST_DATABASE_URL`を注入した後、containerとvolumeを破棄する。

## CI

`.github/workflows/ci.yaml` は ADR-017 に従い、pull request の `opened`、`synchronize`（追加commitなどによる更新）、`reopened` イベントで起動する。pull requestのないbranch pushでは起動しない。各jobはUbuntu 24.04の別runnerで実行されるため、PostgreSQL統合テストと固定portを使うbrowser E2Eは競合しない。

- Frontend and contract: frozen install、OpenAPI、非破壊format/lint、型チェック、Vitest、Node script tests、E2E runner unit test、Keycloak provisioning shell test、buildを個別stepで検査する。
- Go static and unit: gofmt、vet、DBを使わないGo test、module verifyを個別stepで検査する。
- PostgreSQL integration: `pnpm run test:db`。使い捨てcredentialを生成し、test runnerとworkflowの終了処理でcontainer/volumeを削除する。
- Browser E2E: frozen install後にChromiumとLinux依存を導入し、`pnpm run test:e2e`を実行する。E2E runnerが使い捨てcredentialを生成して自分のDocker projectと一時ファイルを削除する。

GitHub Actionsの`permissions`は`contents: read`のみで、外部actionを完全なcommit SHAで固定する。format、lint、OpenAPI、frozen install後にtracked fileが変わっていないことも検査する。CI失敗時にbase branchへのmergeを禁止するには、GitHub上でPRの最新commitに必須status checkを要求するbranch保護を別途設定する。

## Frontend Task 3 の起動と検証

Frontend は ADR-015 に従い API と異なる origin で起動する。loopback 開発例では API 側の `APP_FRONTEND_ORIGIN=http://127.0.0.1:5173` と、Frontend 側の `VITE_API_ORIGIN=http://127.0.0.1:8080` を対応させる。`VITE_API_ORIGIN` は絶対 HTTP(S) origin とし、path、query、fragment、userinfo は付けない。HTTP は loopback 開発のみ許す。

```bash
pnpm install --frozen-lockfile
VITE_API_ORIGIN=http://127.0.0.1:8080 pnpm run dev --host 127.0.0.1
pnpm test
pnpm run typecheck
pnpm run format:check
pnpm run lint
pnpm run build
```

画面は session bootstrap、Sign in、Organization 選択、Draft 作成・編集、Submit、割当済み Pending 一覧、Approve、Request と Audit の確認、logout と明示的なエラー回復を提供する。手動起動の API・Keycloak・DB の準備は [`frontend-local-development.md`](frontend-local-development.md) と [`local-api.md`](local-api.md) を参照する。

Browser E2E の entrypoint は `pnpm run test:e2e`。これは `compose.e2e.yaml` の専用 PostgreSQL/Keycloak と Go API、Vite、Playwright を起動し、その実行が作成した資源だけを終了時に削除する。`pnpm run test:e2e:runner` は runner の unit test。固定 loopback port `8080`、`8081`、`5173`、`55432` を使用するので、`pnpm run test:db` や手動開発 stack と同時に実行しない。Playwright Chromium を初回に `pnpm exec playwright install chromium` で導入する。

ローカル macOS で Codex が Browser E2E を実行するときは、最初から `exec_command` の `sandbox_permissions: "require_escalated"` と `prefix_rule: ["pnpm", "run", "test:e2e"]` を指定する。承認・自動審査は引き続き適用される。sandbox 内の既知の Chromium 権限エラーを E2E の失敗として記録せず、権限付き実行の実結果、または承認が得られなかった事実を記録する。開発者が端末で実行するコマンドは変わらない。詳細は [`frontend-local-development.md`](frontend-local-development.md) を参照する。
