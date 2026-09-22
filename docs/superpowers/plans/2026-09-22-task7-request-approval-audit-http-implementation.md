# Task 7: Request、Approval、Audit HTTP 実装計画

> **エージェント実行者向け:** 必ず `superpowers:subagent-driven-development`（推奨）または `superpowers:executing-plans` を使用し、タスク単位で実行すること。進捗はチェックボックスで管理する。

**目的:** 承認済みの全`/api/v1/requests` operationをOpenAPI JSON契約どおりに公開し、組織選択と認可にはsession由来のActorだけを使用する。

**アーキテクチャ:** `internal/httpapi`はDTO decode、context内の認証済み`requests.Actor`取得、application service呼出しを担う。workflow、可視性、並行性はapplication serviceに委譲する。serviceとPostgreSQL repositoryはserver側組織ID、Approval詳細、Audit Event ID/metadataを提供し、SQL rowやHTTP入力の組織／Actorを公開・信頼しない。

**技術スタック:** Go 1.27.1、既存の`net/http`/`ServeMux`、`encoding/json`、`httptest`、`database/sql`、PostgreSQL。dependencyは追加しない。

**仕様:** `docs/superpowers/specs/2026-09-20-api-contract-design.md`、`api/openapi.yaml`、`docs/superpowers/plans/2026-09-20-go-api-implementation.md`のTask 7。

## 現状の確認結果

- Task 6は完了済み。`internal/httpapi`にはsession/CSRF middleware、`ActorFromContext`、error writer、composition rootがある。
- `CreateRequestInput`に`organizationId`はなく、現serviceは作成時に組織IDを要する。選択済みMemberの組織をserver側で取得し、`auth.Principal`から`requests.Actor`へ伝播する必要がある。
- OpenAPIが必須とするApproval ID、Audit Event ID、approval metadataを、現domain/repositoryは完全には返していない。DBには必要な`approvals.id`、`audit_events.id`、`approval_metadata`がある。

## 共通制約

- `api/openapi.yaml`に定義済みのCreate、Get、Update Draft、Submit、List Pending、Approve、List Audit Eventsだけを実装する。Reject、Cancel、通知、workflow管理、frontend、ORM、router、dependencyは追加しない。
- ADR-003に従いOpenAPI 3.1を契約の正本とし、ADR-002に従いmethod-aware `ServeMux`と`Request.PathValue("requestId")`を使用する。
- 全request operationにsession middleware、Create/PATCH/Submit/ApproveにはOrigin/CSRF middlewareをhandler前に適用する。
- HTTP header/body/path/query/OIDC claimからActor、role、組織を選んではならない。Createの組織は認証済みMemberのDB membershipから導出する。
- 認可、state transition、version比較、approval assignment、audit作成は`internal/application/requests`に残す。
- PDR-001（trim後Title 1–120、任意Description 2,000以下、Requester自身のDraftのみ編集）とPDR-002（default Approverへの割当、self-assignment禁止）を維持する。
- mutationには正の`expectedVersion`を要求する。古いversionは`409 version_conflict`、無効stateは`409 invalid_state`、routing failureは`409 approval_routing_unavailable`とする。`Idempotency-Key`は追加しない。
- 可視でないRequestは`404 request_not_found`、forbidden mutationと非ApproverのPending listは`403 forbidden`とする。
- PostgreSQL testは隔離した`TEST_DATABASE_URL`を使用し、Go commandには`GOCACHE=/private/tmp/learn-ai-go-cache`を付ける。

## レビュー重点項目

- 偽装したbody/header/queryで別組織のRequestを作れない。
- Draftは`approval:null`、Pending/ApprovedはApproval ID・assignee・status・適切な`approvedAt`を返す。
- AuditはID、`(occurred_at, id)`昇順、空Descriptionを含むsnapshot、Submit/Approve metadataを保持する。
- 不正JSON、unknown field、trailing JSON、非正expectedVersion、content validationは`400 invalid_request`でありservice mutationを呼ばない。
- CSRF/Originの失敗はmutable handler/serviceより前に拒否され、認証済みGETには両headerを要求しない。

---

## ファイル構成

- `internal/auth/session.go`／`session_test.go`、`internal/store/postgres/sessions.go`／`sessions_test.go` — `auth.Principal.OrganizationID`をserver側Memberから取得・検証する。
- `internal/domain/request.go`、`internal/application/requests/repository.go`／`service.go`／`service_test.go`、`internal/store/postgres/requests.go`／`requests_test.go` — Approval/AuditのID・metadataと認可済みapproval read seamを提供する。
- `internal/httpapi/errors.go` — request error codeと任意`fieldErrors`を内部detailなしで出力する。
- `internal/httpapi/response_dto.go`／`response_dto_test.go` — Request、Approval、PendingRequestList、AuditEventListの公開DTO変換。
- `internal/httpapi/request_handlers.go`／`request_handlers_test.go` — strict JSON decodeと全7 HTTP operation。
- `internal/httpapi/router.go`、`cmd/api/main.go` — request serviceのdependency injectionとroute登録。

### Task 1: server-derived request read modelを補正する

**ファイル:** 上記auth/session、domain、application/requests、PostgreSQL request関連ファイル。

**Interface:**

```go
type Principal struct {
	MemberID, OrganizationID string
	Roles []domain.Role
}
type Actor struct {
	MemberID, OrganizationID string
	Roles []domain.Role
}
type Approval struct {
	ID, RequestID, AssigneeMemberID string
	Status ApprovalStatus
	ApprovedAt *time.Time
}
type ApprovalMetadata struct { AssigneeMemberID, ApprovalID string }
type AuditEvent struct {
	ID, RequestID, ActorMemberID, Type string
	OccurredAt time.Time
	ContentSnapshot *ContentSnapshot
	ApprovalMetadata *ApprovalMetadata
}
func (s *Service) GetApproval(context.Context, Actor, string) (*domain.Approval, error)
```

- [x] **Step 1: 失敗するunit／integration testを書く**

session principalが`OrganizationID`を返すことをtestする。`GetApproval`はRequester/assigned Approverにpending Approvalを返し、visible Draftにはnil、別Requesterには`domain.ErrNotFound`を返すことをtestする。PostgreSQLでcreate→submit→approve後にApproval ID、Audit Event ID、Submit/Approve metadata、nil metadata、空Description snapshot、`(occurred_at,id)`順をassertする。

- [x] **Step 2: focused testが失敗することを確認する**

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/auth -run 'Test.*Principal' -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests -run 'Test(GetApproval|CreateDraft)' -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/store/postgres -run 'Test(SessionPrincipal|Audit|Approval)' -count=1
```

期待結果: 組織ID、ID/metadata、`GetApproval`未実装のためFAIL。

- [x] **Step 3: 最小実装を追加する**

principal SQLで`members.organization_id`を取得し、roleは引き続き`member_roles`のみから得る。`RequireSession`は組織IDをActorへコピーする。Approval IDをopaqueで保持し、Content Snapshot/approval metadataをpointerでSQL NULLに対応させる。Create/Update/Submit/Approveはnon-nil snapshotを保持する。Submitでは一度だけ生成したApproval IDをapproval insertとaudit metadataへ使用する。Approveではtransaction内で既存Approval IDを読みmetadataへ保存する。`ListAuditEvents`はIDとnullable snapshot/metadataをdecodeし、認可済み`Service.GetApproval`は`Service.Get`のvisibilityを先に適用する。

- [x] **Step 4: focused testが成功することを確認する**

Step 2の3 commandを再実行する。期待結果: 隔離済みDBでPASS。

- [x] **Step 5: commitする**

```bash
git add internal/auth internal/domain/request.go internal/application/requests internal/store/postgres
git commit -m "fix: expose server-derived request read model"
```

### Task 2: strict request inputとOpenAPI response mappingを追加する

**ファイル:** 作成`internal/httpapi/response_dto.go`、`internal/httpapi/response_dto_test.go`、変更`internal/httpapi/errors.go`。

**Interface:** private request bodyは`createRequestInput`、`updateDraftRequestInput`、`expectedVersionInput`のみとする。DTOはRequest/Approval/AuditEventのOpenAPI fieldだけを公開し、`APIError`に任意`FieldErrors []fieldErrorDTO`を追加する。

- [x] **Step 1: 失敗するDTO/error testを書く**

Draftの`approval:null`、Pending/ApprovedのApproval field、RFC 3339日時、nil sliceから`[]`、Auditのnullability/opaque ID、`WriteError`の`fieldErrors`とsecret-free responseをtestする。

- [x] **Step 2: focused testが失敗することを確認する**

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(RequestDTO|ApprovalDTO|AuditEventDTO|WriteError)' -count=1
```

期待結果: DTO/error field未実装のためFAIL。

- [x] **Step 3: DTO/error mappingを実装する**

DTOはprivateにし、`Content-Type: application/json`と`Cache-Control: no-store`で出力する。sliceはempty arrayを返す。`requestAPIError`で`ErrInvalidRequest`→400、`ErrForbidden`→403、`ErrNotFound`→404、`ErrVersionConflict`／`ErrInvalidState`／`ErrApprovalRoutingUnavailable`→各409 codeを一元化する。body fieldを特定できる場合だけ`fieldErrors`を使い、malformed JSON等ではfield名を推測しない。wrapped Go errorは公開しない。

- [x] **Step 4: focused testが成功することを確認する**

Step 2を再実行する。期待結果: PASS。

- [x] **Step 5: commitする**

```bash
git add internal/httpapi/response_dto.go internal/httpapi/response_dto_test.go internal/httpapi/errors.go
git commit -m "feat: map request API responses to OpenAPI"
```

### Task 3: 全Request、Approval、Audit operationを登録してtestする

**ファイル:** 作成`internal/httpapi/request_handlers.go`、`request_handlers_test.go`、変更`internal/httpapi/router.go`、`cmd/api/main.go`。

**Interface:** `httpapi.Dependencies`へ`RequestService` interfaceを追加する。CreateDraft、UpdateDraft、Submit、Approve、Get、GetApproval、ListPending、ListAuditEventsの既存service methodだけを定義し、handlerはrepositoryへ直接アクセスしない。

- [x] **Step 1: 失敗するHTTP contract testを書く**

`RequestService` fakeと既存session fakeで、成功Create（201/Location）、Get、PATCH、Submit、Pending、Approve、Auditをtestする。ActorはHTTP headerでinjectせず、context由来の`MemberID`、`Roles`、`OrganizationID`がserviceへ渡ることをassertする。各mutationで401、CSRF/Origin 403かつservice未呼出し、JSON/expectedVersionの400、403/404/409 mappingをtestする。stale Submitが409となりAudit eventを追加しないstateful fake testも追加する。GETはOrigin/CSRFなしで動作することを確認する。

- [x] **Step 2: focused testが失敗することを確認する**

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -run 'Test(Create|GetRequest|Update|Submit|Pending|Approve|Audit)' -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./cmd/api -run TestNewHandler -count=1
```

期待結果: handler、route、dependency wiring未実装のためFAIL。

- [x] **Step 3: handler／route／wiringを実装する**

`json.Decoder.DisallowUnknownFields()`を使うprivate `decodeJSONBody`で、bodyなし、empty body、second document、unknown field、trailing bytesを拒否する。`ExpectedVersion >= 1`をdispatch前に検証する。Actorがcontextになければ401でfail closedする。`request.PathValue("requestId")`と対応service methodだけを使い、errorは`requestAPIError`、responseはTask 2 DTOへ渡す。Create/Update/Submit/Approve成功時には認可済み`GetApproval`でresponseを完成させる。`Location`は返却opaque IDからだけ作る。7 routeを`RequireSession`、mutationには内側`RequireCSRF`で登録し、`cmd/api`で構築済みserviceを一度だけDependencyへ渡す。

- [x] **Step 4: HTTP testを実行する**

```bash
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/httpapi -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./cmd/api -count=1
```

期待結果: PASS。

- [x] **Step 5: commitする**

```bash
git add internal/httpapi/request_handlers.go internal/httpapi/request_handlers_test.go internal/httpapi/router.go cmd/api/main.go
git commit -m "feat: expose request approval API"
```

### Task 4: Task 7全体を検証してcompletion evidenceを残す

- [x] **Step 1: database-backed testを実行する**

```bash
source /Users/nao/.nvm/nvm.sh
nvm use 26.9.0
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/application/requests ./internal/store/postgres ./internal/httpapi ./cmd/api -count=1
```

期待結果: PASS。test database以外を対象にしない。

- [x] **Step 2: quality gateを実行する**

```bash
pnpm install --frozen-lockfile
pnpm run format:check
pnpm run lint
pnpm run typecheck
pnpm run check:gofmt
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go mod verify
git diff --check
```

期待結果: PASS。各commandはworking treeを書き換えない。

- [x] **Step 3: completion evidenceを記録する**

全check成功後に`docs/development/task7-completion-2026-09-22.md`を作成する。変更file、ADR-002/003/005/011/013、PDR-001/002、検証command/result、新Decisionなしを記録する。browser E2Eを実施したとは記録しない。

- [x] **Step 4: evidenceをcommitする**

```bash
git add docs/development/task7-completion-2026-09-22.md
git commit -m "docs: record Task 7 completion"
```

## 計画の自己レビュー

- **仕様coverage:** Task 1が不足するserver-side source、Task 2がpublic DTO/error fidelity、Task 3が7 OpenAPI operation、Task 4がpersistence/HTTP/quality/traceabilityを扱う。
- **Decision確認:** 新PDR/ADRは不要。ADR-002/003/005/011/013とPDR-001/002を実装する。見つかった欠落は既存実装／計画の不完全さである。
- **型の整合性:** `auth.Principal.OrganizationID`が`requests.Actor.OrganizationID`の唯一のsource。`GetApproval`のpointerはvisible DraftにApprovalがないことを表す。DTOはorganization、cookie、CSRF、OIDC、repository、SQL rowを公開しない。
- **placeholder確認:** 未決の実装、指定されていないerror処理、関連付かないtest stepは残っていない。
