# ADR-015: React Frontendの配信トポロジー

- Status: Accepted
- Date: 2026-09-23
- Owners: Human project owner
- Related requirements: FR-012, NFR-001, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

ADR-001により、初回Vertical SliceのブラウザUIはReactとViteで実装する。ADR-005およびADR-011により、認証済みAPIはHttpOnlyなserver-side session cookie、session-bound CSRF token、許可された単一Originを使う。現在のGo APIはOIDC callback後に固定ローカルUI pathの`/`または`/organization-selection`へredirectし、`APP_ALLOWED_ORIGIN`と一致するOriginだけをunsafe requestで受け入れる。

Reactの開発サーバーをAPIとは別originでそのまま起動すると、callback後の`/`はAPI originへ戻り、React UIへ到達しない。また、cookieを伴うbrowser API requestにはCORS response policy、credential mode、origin設定を別途定義する必要がある。反対に、productionでReact buildをGo binaryが直接配信するか、同一originのreverse proxy/static hostが配信するかも未決定である。

このDecisionは、FrontendのURL、OIDC redirect、Cookie/CSRF境界、開発手順、production packagingを横断して決める。React component構造、client-side routing、UI component library、デプロイ先の選定は対象外とする。

## Decision drivers

- OIDC callback、Organization選択、session cookie、unsafe API requestを同一のbrowser originで一貫して成立させること
- ADR-011のCookie属性とOrigin/CSRF検証を弱めずに、ローカル開発を再現可能にすること
- 初回SliceにCORS、credentialed cross-origin request、return URL処理を不用意に導入しないこと
- React/Viteの開発体験を維持しつつ、productionの配信責務を明確にすること
- 新しい本番runtime dependencyを最小化すること

## Options considered

### Option A: Go APIがReactのbuild成果物を同一originで配信する

Viteは開発時だけReactを配信する。productionでは`dist/`をGo API processが静的配信し、既存のAPI/OIDC route以外の`/`と`/organization-selection`はSPA entry pointへfallbackする。local developmentは、Vite dev serverが`/api`と`/auth`をGo APIへproxyし、browserから見えるoriginをViteの単一originにする。Go APIの`APP_ALLOWED_ORIGIN`はVite originと一致させる。

**利点**

- productionのUI、OIDC callback、session cookie、APIを同一originにでき、credentialed CORSを追加しない。
- callbackの固定path `/`、`/organization-selection`をそのままReact entry pointで扱える。
- 追加のproduction web server/runtime dependencyを導入しない。

**欠点**

- Go binaryまたは同一release artifactにFrontend build成果物を含めるbuild/package手順が必要になる。
- API routerは静的asset配信とSPA fallbackを、安全にAPI/OIDC routeより後段へ追加する必要がある。

### Option B: 同一originのreverse proxy/static hostがReactとGo APIを配信する

Vite buildはstatic hostが`/`とSPA fallbackを配信し、reverse proxyが`/api`と`/auth`をGo APIへ転送する。local developmentも同等のproxyを用意する。browserはproxy originだけを利用する。

**利点**

- API processからstatic file配信を分離でき、cache policyを専用web serverで最適化できる。
- UIとAPIを同一originに保てる。

**欠点**

- reverse proxy/static hostの製品、設定、local/production運用を選定・保守する必要がある。
- 初回Sliceには未選定の配信基盤と追加の運用面が入る。

### Option C: ReactとGo APIを別originで配信し、credentialed CORSを追加する

Vite/static hostとGo APIを別originに置く。Go APIは厳格なCORS response headerを返し、Frontendは`fetch`でcredentialsを明示的に送信する。OIDC callbackはFrontend originへredirectできるよう、API設定またはintermediate handoffを追加する。

**利点**

- FrontendとAPIを独立して配信・スケールできる。
- static hosting/CDNを容易に利用できる。

**欠点**

- Cookie、SameSite、CORS、OIDC callback、CSRFの組合せが初回Sliceに追加され、セキュリティ検証範囲が広がる。
- CORS policyとredirect/handoffの新たなAPI・運用設計が必要になる。

## Decision

**Option Cを採用する。** ReactをFrontend origin、Go APIをAPI originとして別々に配信する。初回SliceはADR-011の`SameSite=Lax` cookieを維持するため、両originを同一site（同一schemeかつ同一registrable domain）に限定する。例としてproductionでは`https://app.example.test`と`https://api.example.test`、local developmentでは`http://127.0.0.1:5173`と`http://127.0.0.1:8080`を使用できる。cross-site originの組合せは対象外であり、`SameSite=None`を必要とするため別ADRなしには導入しない。

Go APIには厳格なcredentialed CORSを追加する。許可するoriginは設定済みの単一Frontend originだけとし、APIは許可originへのresponseにだけ`Access-Control-Allow-Origin`をその完全な値で返す。`Access-Control-Allow-Credentials: true`、`Vary: Origin`、必要最小限のmethod（`GET`、`POST`、`PATCH`、`OPTIONS`）とheader（`Content-Type`、`X-CSRF-Token`）を返す。wildcard origin、wildcard header、複数origin、reflective allowlist、CookieまたはAuthorizationをpreflight許可の代替にする実装は採用しない。

`APP_FRONTEND_ORIGIN`を、CORS allowlistとOIDC callback／Organization selection完了後の固定redirect destinationの正本とする。値はabsolute HTTP(S) originだけを受け入れ、path、query、fragment、userinfoを含めない。callbackとselection成功時のredirectは、クライアント入力を反映せず、`APP_FRONTEND_ORIGIN + "/"`または`APP_FRONTEND_ORIGIN + "/organization-selection"`だけへ送る。`APP_ALLOWED_ORIGIN`は互換性のため同一値で受け入れず、`APP_FRONTEND_ORIGIN`へ名称統一する。

## Consequences

### Positive

- FrontendとAPIを独立して配信・スケールできる。
- ReactはAPI originと異なるstatic hosting/CDNへ配置できる。
- Reactのclient-side routerを導入せず、初回Sliceは`location.pathname`で`/`と`/organization-selection`を分岐できる。

### Negative / trade-offs

- CORS preflight、credential送信、callback redirect、Frontend/API origin設定をHTTP統合・E2Eで検証する必要がある。
- cross-site deploymentはSameSite cookie要件とCSRF threat modelを変更するため、初回Sliceに含めない。
- static host/CDNの具体的な製品、TLS termination、cache policy、deployment automationは未決定のまま残る。

## Validation

- allowed Frontend originから、login開始、OIDC callback、Organization選択、session、unsafe API requestがCookieとCSRF tokenを保ったまま動作することをPlaywright E2Eで確認する。
- preflightと実responseが正確なallow-origin、credentials、methods、headers、`Vary: Origin`を返し、未知originにはCORS headerを返さないことを`httptest`で確認する。
- `APP_FRONTEND_ORIGIN`以外のOriginはunsafe API requestで`403 csrf_validation_failed`となり、CORS response headerによって回避できないことを確認する。
- callbackとselection成功時のredirectが設定済みFrontend originの固定pathだけを使い、query/body入力を反映しないことを確認する。
- React build、Go test、TypeScript component test、Playwright E2Eを文書化した固定version commandで実行できることを確認する。

## Revisit conditions

- cross-site origin、複数許可Frontend origin、mobile/embedded client、外部API clientが必要になった場合。
- static host/CDN、TLS termination、cache、observability、deployment automationの具体的な基盤選定が必要になった場合。
- client-side routingが`/`と`/organization-selection`以外の共有可能なURLを必要とした場合。
