# API Contract Design

## Goal

最初のVertical Sliceについて、ブラウザUI、OIDC認証境界、Go HTTP APIが共有するレビュー可能なOpenAPI 3.1契約を定義する。対象はDraft作成・更新、Submit、割当済みPending一覧、Approve、Audit History、ログイン、現在session、logoutである。

Reject、Cancel、複数Approval Step、通知、検索・絞り込み、ワークフロー設定は、この契約の対象外とする。RejectとCancelは対応するProduct Decisionが未承認のため、endpointを先行して追加しない。

## Decision basis

- ADR-002: Go 1.22以降の`net/http`と`ServeMux`、JSON over HTTP、HTTP statusと機械可読なエラーコード
- ADR-003: OpenAPI 3.1をHTTP API契約の正本とし、内部DB表現を公開しない
- ADR-005、ADR-009、ADR-010、ADR-011、ADR-013: OIDC Authorization Code Flow with PKCE、Keycloak、Memberに束縛したserver-side session、複数Organization所属時の選択、CSRF防御
- ADR-008: resource APIは`/api/v1` URL prefixを使用する
- PDR-001: Title/Description、Draftだけの更新、Submit後の不変性
- PDR-002: Organization既定Approverへの単一Approval Step割当と自己承認禁止

## Boundary and routes

OpenAPIのserver URLは`/`とする。OIDC browser handoffはresource APIではないため、`/auth/oidc/login`と`/auth/oidc/callback`に置く。一方、sessionと業務操作は`/api/v1`に置く。

| Route | Purpose |
| --- | --- |
| `GET /auth/oidc/login` | OIDC認証transactionを開始し、Providerへ302 redirectする |
| `GET /auth/oidc/callback` | callbackを検証する。対応するMemberが1件ならsessionを発行してFrontendの`/`へ、複数件なら選択transactionだけを発行して`/organization-selection`へ302 redirectする |
| `GET /auth/oidc/organization-selection` | 選択transactionの候補Memberと新しいCSRF tokenを返す |
| `POST /auth/oidc/organization-selection` | 候補Memberの選択を検証し、選択済みMemberのsessionを発行してFrontendの`/`へ302 redirectする |
| `GET /api/v1/session` | 現在のActorとunsafe request用CSRF tokenを返す |
| `POST /api/v1/session/logout` | sessionを失効してcookieを削除する |
| `POST /api/v1/requests` | Draftを作成する |
| `GET` / `PATCH /api/v1/requests/{requestId}` | Requestを取得し、Draftを更新する |
| `POST /api/v1/requests/{requestId}/submit` | 既定Approverへ単一Stepを割り当て、Pendingへ遷移する |
| `GET /api/v1/requests/pending` | 認証済みApproverに割り当てられたPending一覧を返す |
| `POST /api/v1/requests/{requestId}/approvals` | 割当済みApproverがApproveする |
| `GET /api/v1/requests/{requestId}/audit-events` | Requestの監査履歴を返す |

## Representation and concurrency

`Request`、`Approval`、`AuditEvent`は内部tableを表現しないDTOである。IDは不透明な文字列、日時はRFC 3339 `date-time`、`version`は1以上の整数とする。URLのpath segmentとなる`requestId`は空文字列および正確に`.`または`..`の値を認めない。後二者はブラウザやrouterのpath正規化で別のendpointを指し得るためであり、その他の不透明なIDは許容する。

全ての状態変更とDraft更新はbody内の`expectedVersion`を必須とする。現在versionまたは状態と合わない場合は`409`と`version_conflict`または`invalid_state`を返す。ネットワーク障害で結果不明になったクライアントは再送せず、`GET /api/v1/requests/{requestId}`で最新状態を取得する。初回Sliceでは`Idempotency-Key`を使用しない。

## Authentication, authorization, and CSRF

`/api/v1`はsession cookieで認証する。OpenAPIではproductionの`__Host-approval_flow_session` cookieを`sessionCookie`として表現する。unsafe methodには、`GET /api/v1/session`で取得したsession-bound CSRF tokenを`X-CSRF-Token` headerで送る。実装は許可originも照合する。

複数所属のcallbackではapplication sessionを発行しない。選択画面は短命な選択transactionのcookieを使って候補と選択用CSRF tokenを取得する。選択POSTは許可origin、CSRF token、transaction内の候補Memberを検証し、成功時に選択済みMemberへ束縛したsessionを発行する。候補が0件または認証検証に失敗した場合もsessionを発行しない。

OIDC token、PKCE verifier、role、Member IDはcookieやbrowser storageに置かない。認可とAudit EventのActorは、server-side sessionから得たMemberとアプリケーションDBのrole・割当で決定する。

読み取りでは、閲覧を許可しないRequestを`404 request_not_found`として扱う。変更操作では、認証済みだがRequester/割当Approverの条件を満たさない場合を`403 forbidden`として扱う。

## Error model

全てのJSONエラーは`ErrorResponse`を使う。`code`はUIが分岐に使用する安定した機械可読値であり、`message`の解析を禁止する。入力不正は`fieldErrors`で対象fieldを示す。

| Status | Code | Meaning |
| --- | --- | --- |
| 400 | `invalid_request` | JSON形式、field、またはcallback入力が不正 |
| 400 | `invalid_auth_transaction` | OIDC transactionが不一致、期限切れ、または使用済み |
| 401 | `authentication_required` | 有効なsessionがない、期限切れ、または失効済み |
| 403 | `forbidden` | 認証済みActorに当該変更操作の権限がない |
| 403 | `csrf_validation_failed` | CSRF tokenまたはorigin検証に失敗 |
| 404 | `request_not_found` | Requestがない、または読取りを許可しない |
| 409 | `version_conflict` | `expectedVersion`が現在versionと異なる |
| 409 | `invalid_state` | 現在のRequest stateでは操作できない |
| 409 | `approval_routing_unavailable` | 既定Approver不在・role不一致・自己割当のためSubmitできない |

## Validation scope

OpenAPI構文・参照を検証し、`httptest`統合テストは各operationの成功・400・401・403・404・409とJSON schemaを検証する。ブラウザE2EはRequesterのDraft作成・Submit、ApproverのApprove、RequesterのApproved/Audit History確認を通す。詳細な実装順序は、この仕様のレビュー承認後に作成するimplementation planで定義する。
