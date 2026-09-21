# Toolchain and dependency baseline

Status: Accepted ADR configuration

この文書は、Accepted ADR-001、ADR-002、ADR-004、ADR-006、ADR-007、ADR-009、ADR-010、ADR-012に基づく初回Sliceの再現可能なtoolchain基準です。versionは2026-09-21時点の最新安定版として固定します。更新は依存関係ポリシーとADR-007のレビュー手順に従って行います。

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
