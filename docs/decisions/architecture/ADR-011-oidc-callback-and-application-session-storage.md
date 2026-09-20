# ADR-011: OIDC Callback一時状態とアプリケーションSessionの保存方式

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-001, FR-002, FR-003, FR-005, FR-007, FR-013, NFR-001, NFR-003, NFR-004, NFR-006
- Supersedes: none
- Superseded by: none

## Context

ADR-005は、OIDC Authorization Code Flow with PKCEを使用し、ブラウザにはアプリケーション発行の`Secure`、`HttpOnly`、`SameSite`属性付きsession cookieだけを保持させる。ADR-009は、callbackでissuer、audience、署名、有効期限、nonce、PKCE verifier、`state`を検証することを要求する。ADR-010は、OIDC Discovery、ID Token検証、Authorization Code交換のGoライブラリを選定した。

Authorization開始からcallbackまでには、単回使用の`state`、nonce、PKCE verifierを同じユーザーエージェントへ安全に束縛しなければならない。ログイン後には、検証済みのMemberを複数API requestに結び付け、logout、失効、期限切れをサーバー側で判定できなければならない。ブラウザにOIDC token、Member ID、role、PKCE verifier、または認可判断の根拠を保存してはならない。

このADRは、OIDC callback一時状態とログイン済みsessionの保存・cookie伝達・失効・CSRF防御を決める。Identity Providerのrealm設定、Member provisioning、session期限の具体的な時間値、refresh token、logoutのProvider連携、key rotation、複数リージョンのsession共有は対象外とする。

## Decision drivers

- `state`、nonce、PKCE verifierをtransactionごとに生成し、開始したユーザーエージェントと安全に束縛して単回使用できること
- 検証済みの`iss`と`sub`から対応付けたMemberだけを、サーバー側で各API requestのActorとして決定できること
- sessionの失効、logout、期限切れをサーバー側で判定できること
- OIDC token、role、Member ID、PKCE verifierをブラウザやURLへ露出しないこと
- 既に採用済みのPostgreSQL、`database/sql`、手書きSQL、Go標準ライブラリだけで、明示的にテストできること
- Keycloak固有のsession、role/group、管理APIに依存しないこと

## Options considered

### Option A: PostgreSQLに保存する不透明なserver-side sessionと認証transaction

ログイン開始ごとに認証transactionをPostgreSQLへ保存し、callback成功後に別のapplication sessionをPostgreSQLへ保存する。ブラウザには意味を持たないランダムなcookie値だけを置き、DBにはその値のhashを保存する。

**利点**

- sessionと認証transactionをサーバー側で単回使用、失効、期限切れとして判定できる
- アプリケーション再起動後もsessionとcallback transactionを継続でき、複数プロセスへ拡張する際も共有できる
- cookieにMember、role、OIDC token、PKCE verifierを含めず、認可の正本をアプリケーションDBに保てる
- PostgreSQL以外の新しい本番サービス・libraryを導入しない

**欠点**

- session・認証transaction用のmigration、index、削除ジョブまたは期限付き照会を実装・運用する必要がある
- requestごとにsession照会が発生する
- 認証transactionのPKCE verifierをDBに安全に保護する責務がある

### Option B: 署名または暗号化した自己完結cookie

sessionのMember識別子・期限・認証transactionの値を署名または暗号化し、ブラウザcookieに保存する。サーバーはcookieを検証してActorを復元する。

**利点**

- requestごとのDB session照会を避けられる
- session専用テーブルを作らずに開始できる

**欠点**

- logout、侵害時の即時失効、単回使用transactionを一貫して扱いにくい
- cookieに認可に関係する状態または一時secretを保持するため、鍵管理、暗号化、rotation、サイズ制限を追加で設計する必要がある
- tokenやroleをcookieへ入れないというADR-005/009の境界を誤って拡張しやすい

### Option C: プロセス内メモリのsession store

アプリケーションprocess内のmap等にsessionと認証transactionを保存し、cookieにはランダムIDだけを保存する。

**利点**

- 初期実装が小さく、DB schemaを増やさない
- 単一プロセスでは照会が高速である

**欠点**

- process再起動でログイン中のsessionと認証途中のtransactionが失われる
- 複数process、並列E2E、将来の水平拡張で共有できない
- cleanup、失効、テスト隔離を独自に実装する必要があり、初回Sliceの既存PostgreSQL採用と整合しない

## Decision

**PostgreSQLに不透明なserver-side application sessionとOIDC認証transactionを保存することを推奨する。** sessionとtransactionのcookieには、CSPRNGで生成した少なくとも128 bitの意味を持たない値だけを保存する。DBにはcookieのhashを保存し、受信値をhashして検索する。未発行のcookie値は有効なsessionとして受け入れない。

認証開始時、サーバーはCSPRNGにより十分なエントロピーを持つ`state`、nonce、PKCE verifier、認証transaction cookie値をtransactionごとに新規生成する。

`oidc_auth_transactions`は、transaction cookie hash、`state` hash、nonce、暗号化したPKCE verifier、issuer、client ID、redirect URI、作成時刻、短い有効期限、使用済み時刻を保存する。callbackではtransaction cookie、`state`、期限、未使用状態を照合し、成功・失敗を問わずtransactionを再利用不能にする。PKCE verifierの暗号化鍵は実行環境のsecretとして渡し、リポジトリ、ログ、cookie、URLへ保存しない。

callbackでID Tokenを検証し、`iss`と`sub`をMemberに対応付けた後、既存sessionを昇格・再利用せず新しいapplication sessionを発行する。`app_sessions`は、session cookie hash、Member ID、作成時刻、最終利用時刻、idle/absolute expiry、失効時刻を保存する。各認証済みAPI requestは、このsessionを照会し、期限切れまたは失効済みsessionを401として拒否する。logoutはsessionを失効し、cookieを削除する。

productionでsession cookieは`__Host-approval_flow_session`とし、`Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/`を設定し、`Domain`属性を設定しない。OIDC redirectがcross-site top-level GETであるため、認証transaction cookieも`SameSite=Lax`を用いる。`Secure`を外すdevelopment例外はloopback interface上のローカル開発・E2Eだけに限定し、production cookieと異なる名前を使う。cookieだけをCSRF防御の根拠にせず、認証済みのunsafe API methodにはserver-side sessionに束縛したsynchronizer CSRF tokenと許可origin検証を必須とする。

## Rationale

PostgreSQLは既にADR-004で採用済みであり、sessionと認証transactionを一貫して扱うための新たな基盤依存を増やさない。不透明なcookieとserver-side recordを分離すれば、ブラウザには秘密の意味を持つ情報を渡さず、sessionの失効、logout、認証transactionの単回使用をサーバー側で確実に判定できる。OAuth security BCPが求めるtransaction固有のPKCE/nonceとユーザーエージェントへの安全な束縛を、transaction cookieとDB recordの照合で表現できる。

`SameSite=Lax`はOIDC redirectを受けるために必要だが、CSRFの完全な代替ではない。そのため、sessionに束縛したsynchronizer tokenと許可origin検証を併用し、状態変更を明示的に守る。

## Consequences

### Positive

- OIDC callbackの`state`、nonce、PKCE verifierをtransaction単位・単回使用で検証できる
- sessionの失効、logout、期限を即時にサーバー側へ反映できる
- OIDC token、role、Member ID、PKCE verifierをブラウザのJavaScript、cookie内容、URLへ公開しない
- session・transactionの動作を実PostgreSQLを用いた統合テストで検証できる

### Negative / trade-offs

- session照会とtransaction照会のためのDBアクセス、migration、期限切れrecordのcleanupが必要になる
- PKCE verifier暗号化鍵のruntime secret管理と、将来のrotation方針が必要になる
- `SameSite=Lax`を使うため、unsafe requestごとのCSRF tokenとorigin検証を実装・テストする必要がある
- 開発用HTTP cookieの例外設定がproductionへ混入しないよう、起動時検証とE2Eで確認する必要がある

## Validation

- 同じcallbackを再送しても、認証transactionが一度しか使用できず、二度目は`400 Bad Request`と`invalid_auth_transaction`で拒否されることを確認する
- transaction cookie、`state`、nonce、PKCE verifier、issuer、client ID、redirect URIのいずれかが不一致、期限切れ、または使用済みならcallbackが失敗することを確認する
- session cookieにMember ID、role、OIDC token、PKCE verifierが含まれず、DBにはcookieのhashだけが保存されることを確認する
- logoutまたはsession expiry後のAPI requestが401となることを確認する
- production設定で`__Host-` cookieに`Secure`、`HttpOnly`、`SameSite=Lax`、`Path=/`が設定され、`Domain`がないことを確認する。development例外がloopback以外では起動時に拒否されることを確認する
- CSRF tokenまたは許可originがないunsafe requestを拒否し、正しいsession・CSRF token・originを持つRequest作成、Submit、Approveだけが成功することを確認する

## Revisit conditions

- 複数リージョン、非常に高いrequest量、またはsession DB照会の実測がPostgreSQL session storeを満たせなくした場合
- refresh token、長期ログイン、provider連携logout、device binding、passkeyなどがsession modelを拡張する場合
- 組織のsession基盤、key management service、またはsecret rotation要件が導入された場合
- browser、embedded client、外部API clientの要件によりcookie sessionとCSRF tokenの前提が成立しなくなった場合
