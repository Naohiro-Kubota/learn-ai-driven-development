# PDR-002: 初回Vertical Sliceにおける単一Approval Stepの割当

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-001, FR-002, FR-004, FR-005, FR-007, FR-010, FR-011, NFR-001, NFR-002, NFR-003
- Supersedes: none
- Superseded by: none

## Context

FR-004はSubmit済みRequestが1つ以上のApproval Stepを通過することを求めるが、初回Sliceで誰がそのStepのApproverになるかは未決定である。ADR-005は、Approverが自分に割り当てられたPending RequestだけをApproveできることをAcceptedとしている。この割当規則がなければ、RequestをSubmitしても誰が承認責任を持つかを説明できない。

このDecisionは初回Sliceの単一Organization、単一Approval Stepに限る。複数Step、並列承認、定足数、条件分岐、代理承認、承認ワークフローを編集するUIは対象外とする。FR-005のReject詳細とFR-006のCancel詳細も別PDRで扱う。

## Options considered

### Option A: RequesterがSubmit時にApproverを選択する

- Behavior: RequesterはOrganization内のApprover roleを持つMemberから1人を選んでSubmitする。
- Benefits: 管理設定なしで複数ApproverへRequestを振り分けられる。
- Risks / edge cases: Requesterが都合のよいApproverを選べる。自分自身を選ぶことの禁止、候補の表示範囲、組織ポリシーを追加で決める必要がある。

### Option B: Organizationの既定Approverへ単一Stepを自動割当する

- Behavior: Organizationごとに、Adminがprovisioning時に1人の既定Approverを設定する。RequesterはApproverを選択しない。Submit時に既定Approverを1件のApproval Stepへ固定し、そのMemberだけがApproveできる。
- Benefits: 承認責任が明確であり、Requesterによる恣意的な宛先選択を防げる。単一StepのEnd-to-Endフローを最小の設定で実証できる。
- Risks / edge cases: 既定Approverが不在・利用不能の場合にSubmitできない。初回SliceにはAdminによる設定変更UIがないため、provisioning手順が必要になる。

### Option C: Organization内の全Approverが承認可能な共有キュー

- Behavior: SubmitしたRequestを全ApproverのPending一覧へ表示し、最初にApproveしたMemberの判断で完了する。
- Benefits: 既定Approver設定が不要で、対応可能なApproverが任意に処理できる。
- Risks / edge cases: 誰が担当すべきか不明確で、同時Approveや責任の所在を扱う追加規則が必要になる。Approval Stepが特定のApproverに割り当てられるというADR-005の前提とも整合しない。

## Decision

**Organizationの既定Approverへ自動割当する単一Approval Stepを採用することを推奨する。** 初回SliceのOrganizationには、Approver roleを持つ1人の既定Approverをprovisioning時に設定する。RequesterはDraft作成時・Submit時ともにApproverを選択できない。

Submitは、既定Approverが存在し、Approver roleを持ち、かつRequester本人と異なる場合だけ成功する。成功時、システムはそのMember IDを単一のApproval Stepへ記録し、RequestをPendingへ遷移させる。以後、既定Approver設定が変わっても既存Pending Requestの割当先は変更しない。割当先のApproverだけがApproveでき、Approve成功時にRequestはApprovedへ遷移する。同じMemberがRequester roleとApprover roleの両方を持つこと自体は許可するが、自分が作成したRequestを自分へ割り当てることは許可しない。

初回Sliceでは既定Approverの変更を行うUIを提供しない。provisioning後に既定Approverを変更する必要がある場合は、運用手順で新規Requestにのみ反映する。既存Pending Requestの再割当は将来のワークフロー管理Decisionで扱う。

## Rationale

既定Approverへの自動割当は、誰が承認責任を持つかを明確にし、Requesterによる宛先選択や共有キューの競合を避けられる。Submit時にMember IDをApproval Stepへ固定すれば、設定変更後も監査対象となる承認責任を説明できる。複雑なワークフロー管理を後回しにしながら、FR-004の最低1StepとFR-005の権限あるApproverによる判断を満たす。

## Consequences

- Requesterは宛先を選べないため、公平性と監査可能性が上がる一方、柔軟な振り分けはできない。
- Organizationのprovisioning手順には、既定Approverの設定とRequesterとの分離の確認が必要になる。
- 既定Approverが不在またはApprover roleを失った場合、新しいRequestをSubmitできない。既存Pending Requestの再割当は初回Sliceではできない。
- Approval Stepには割当Member IDを保存し、Pending一覧はそのMemberに割り当てられたRequestだけを表示する必要がある。
- Admin UIでのワークフロー管理、複数Step、Reject、Cancelの振る舞いは未決定のまま残る。

## Acceptance examples

1. Organization Oの既定ApproverがMember Bで、Requester AがRequestをSubmitすると、1件のApproval StepがMember Bに割り当てられ、RequestはPendingになる。
2. Member CもApprover roleを持つが、Member Bに割り当てられたPending RequestをApproveしようとすると拒否される。
3. Requester A自身がOrganization Oの既定Approverである場合、Requester AのSubmitは失敗し、RequestはDraftのままである。Member AがRequester roleとApprover roleの両方を持つこと自体は妨げない。
4. Organization Oに既定Approverがいない、または既定ApproverがApprover roleを持たない場合、Requester AのSubmitは失敗し、RequestはDraftのままである。
5. Member Bに割り当てられたRequestがPendingの後、Organization Oの既定ApproverをMember Cへ変更しても、既存RequestはMember BだけがApproveできる。以後にSubmitしたRequestはMember Cへ割り当てられる。

## Revisit conditions

- 複数のApprover、金額・申請種別による条件分岐、並列承認、定足数、または代理承認が必要になった場合
- 既定Approver不在によりRequestの滞留またはSubmit失敗が運用上の問題になった場合
- RequesterによるApprover選択、共有キュー、またはAdmin UIでのワークフロー設定が必要になった場合
