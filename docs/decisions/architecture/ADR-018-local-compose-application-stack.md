# ADR-018: ローカルアプリケーションスタックのDocker Compose起動方式

- Status: Accepted
- Date: 2026-09-25
- Owners: Human project owner
- Related requirements: NFR-001, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

NFR-006は、新しい開発者が文書化されたコマンドでアプリケーションと必要サービスをローカル起動できることを求める。現在の`compose.local.yaml`はKeycloakだけを起動する。PostgreSQL、migration、Go API、Viteは別々に準備・起動する必要がある。依頼された到達点は、Frontend、Backend、必要なサービスを`docker compose`でローカル起動できることである。

既存のAccepted DecisionはPostgreSQL（ADR-004）、Keycloak（ADR-009）、PostgreSQL上のsession（ADR-011）、FrontendとAPIの別originかつ同一siteの配信（ADR-015）を定める。現行の開発設定では、Go APIは非Secure cookieを使う際に`APP_LISTEN_ADDR`のloopback待受けを要求し、OIDC issuerはブラウザから見える`http://127.0.0.1:8081/realms/approval-flow-dev`である。通常のCompose bridge networkでは、container内の`127.0.0.1`はhostのloopbackでも別serviceでもない。現行の`migrate-local`もDB URLのloopback hostを要求する。このため、serviceをComposeへ追加するだけでは正常なログインとmigrationを含む起動にならない。

このRecordはローカル開発用スタックの接続方式、初期化順序、公開境界を決める。production deployment方式、Keycloak production mode、CI/E2Eの隔離スタックは対象外とする。

## Decision drivers

- `docker compose`によるFrontend、API、PostgreSQL、Keycloakの再現可能な起動
- ADR-009/011/015のissuer、cookie、Origin、認可境界の維持
- 公開portをhostのloopbackに限定し、credentialや鍵をrepositoryへ保存しないこと
- migrationをAPI起動と区別し、DB準備完了後に明示的に一度実行すること
- macOSのDocker DesktopとLinuxでの再現性、既存E2E/DB test stackとの分離
- 追加runtime dependencyと運用手順の最小化

## Options considered

### Option A: Compose bridge networkと開発専用の接続設定

Frontend、API、PostgreSQL、Keycloakを通常のCompose networkで動かす。host公開portは`127.0.0.1`へbindする。APIはcontainer内で接続できるDB hostを使用し、OIDCの公開issuerを維持しながら、APIからのDiscovery/token/JWKS通信だけを内部接続先へ送る開発専用設定を設ける。開発用の非Secure cookie設定は、明示的なcontainer modeに限ってAPIのcontainer内全interface待受けを許可し、Frontend originは引き続きloopbackに限定する。migrationはAPIとは別の一度きりのCompose jobとして、開発用DBにだけ実行する。Keycloakのrealm/user provisioningとアプリDBへのMember/role/identity mappingも起動手順に含める。

**利点**

- macOS/Linuxの標準的なCompose networkで動作し、host networkingの設定に依存しない
- ブラウザに見えるURLを既存realm、CORS、cookie設定と一致させられる
- 開発DBをE2E/DB testの使い捨てDBから分離できる

**欠点**

- APIの開発専用network設定とmigrationのcontainer実行方法を実装・検証する必要がある
- OIDC通信先の変更がissuer検証を緩めないこと、公開portがloopback限定であることのテストが必要
- 初回起動時のuserとアプリ認可データのprovisioning手順が必要

### Option B: Compose host networking

Frontend、API、PostgreSQL、Keycloakをhost network上で起動する。既存の`127.0.0.1`設定を維持し、migrationを別jobで実行する。

**利点**

- APIのloopback待受け、OIDC issuer、migration URLの既存制約を変更せずに済む
- 開発環境内の通信経路が単純

**欠点**

- macOS Docker Desktopではhost networkingの利用可否と設定に依存し、新規開発者の一コマンド起動を保証しにくい
- Composeのport公開設定とnetwork隔離を利用できず、host側のport競合・公開境界を個別に管理する必要がある

### Option C: Composeは必要サービスのみ、FrontendとBackendはhostで起動

既存のKeycloak ComposeへPostgreSQLを追加し、migration、API、Viteはhost processとしてまとめて起動する補助scriptを用意する。

**利点**

- Go APIとOIDCのnetwork設定変更を避けられる
- host側の既存開発コマンドを再利用できる

**欠点**

- FrontendとBackendを`docker compose`で起動するという依頼を満たさない
- hostのNode.js、pnpm、Go toolchain導入を起動の前提に残す

## Decision

**Option Aを採用する。** `compose.local.yaml`をFrontend、Backend、PostgreSQL、Keycloak、migration jobを含むローカル開発専用スタックに拡張する。開発用container設定はproduction設定と明確に分離し、APIのissuer検証、CORS/CSRF、session cookieの既存要件を維持する。秘密情報は環境から与え、値を含む`.env`や固定の初期passwordをcommitしない。E2E/DB test用Compose fileとvolumeには触れない。

開発専用network設定の有効条件、migration jobの実行方式、provisioningの再実行時の振る舞いを明示し、テストで検証する。

## Rationale

Option Aだけが、Frontend/BackendをComposeで起動する依頼と、macOS/Linuxの一般的なCompose環境での再現性を両立する。追加の接続設定はローカル専用に閉じ、OIDC tokenのissuer検証とブラウザに見えるoriginを変えない。Option Bは環境依存が強く、Option Cは要求の起動範囲を満たさない。

## Consequences

### Positive

- 文書化したCompose commandでアプリケーションと必要サービスを起動できる
- 開発スタックとE2E/DB testスタックの資源を分離できる

### Negative / trade-offs

- 開発専用のnetwork設定、container build、migration/provisioningの保守が必要
- 初回のimage buildとKeycloak起動には時間がかかる

## Validation

- `docker compose -f compose.local.yaml config`で設定を検証する
- 新規volumeでmigration、Keycloak provisioning、Frontend/API起動、OIDC loginと認可済み操作を確認する
- hostに公開されるportがすべてloopback boundであることを確認する
- issuer不一致、未許可Origin、無効なcookie設定が拒否される既存テストを維持し、新しい開発専用設定を追加テストする
- `pnpm run check`、Go test/vet、適用対象のE2Eを実行する

## Revisit conditions

- Docker Desktopのnetwork機能またはKeycloakのissuer仕様が変わり、開発専用接続設定が不要または不適切になった場合
- productionにもCompose deploymentを採用する場合（別Decisionで公開・TLS・secret・永続化・backupを決める）
