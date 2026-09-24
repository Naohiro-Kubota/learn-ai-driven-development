# Toolchain and dependency baseline

Status: Accepted ADR configuration

この文書は、Accepted ADR-001、ADR-002、ADR-004、ADR-006、ADR-007、ADR-009、ADR-010、ADR-012、ADR-015に基づく初回Sliceの再現可能なtoolchain基準です。versionは2026-09-21時点の最新安定版として固定します。更新は依存関係ポリシーとADR-007のレビュー手順に従って行います。

## Toolchain

| Component | Fixed version | Pin location |
| --- | --- | --- |
| Go | 1.27.1 | `.go-version`, `go.mod` `toolchain` |
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
- PostgreSQL migration統合テストは`pnpm run test:db`を使用する。これは`compose.test.yaml`で一時的なPostgreSQL 17.11 containerを起動し、`TEST_DATABASE_URL`を注入してから、終了時にcontainerとvolumeを破棄する。

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
