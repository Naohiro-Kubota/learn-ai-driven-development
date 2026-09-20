# PDR-001: 初回Vertical SliceにおけるRequest内容と編集可能範囲

- Status: Accepted
- Date: 2026-09-20
- Owners: Human project owner
- Related requirements: FR-003, FR-007, FR-009, NFR-002, NFR-003
- Supersedes: none
- Superseded by: none

## Context

FR-003はMemberがRequestをDraftとして作成し、Submitできることを求めるが、初回SliceでRequestにどの内容を含めるか、いつ編集を許可しなくなるかは未決定である。Request内容は、Requester、Approver、Audit Historyが同じ申請を理解できるための最小情報であり、将来の申請種別ごとのフォームとは分離して決める必要がある。

このDecisionは初回Sliceの汎用Requestに限る。添付ファイル、申請種別、カテゴリ、動的フォーム、機密区分、外部システム参照は対象外とする。

## Options considered

### Option A: 必須のTitleだけを持つRequest

- Behavior: Requesterは空白でないTitleだけを入力してDraftを作成・Submitする。Draftの間だけTitleを編集できる。
- Benefits: 入力画面、データモデル、バリデーションが最小になる。
- Risks / edge cases: Titleだけでは申請の目的や判断材料を十分に伝えられず、Approverが確認のために別チャネルへ戻る可能性がある。

### Option B: 必須Titleと任意Descriptionを持つRequest

- Behavior: Requesterは必須Titleと任意Descriptionを入力する。Titleは前後空白を除いた1〜120文字、Descriptionは0〜2,000文字とする。両方ともDraftの間だけ編集でき、Submit後は変更できない。
- Benefits: 最小のフォームで、短い識別子と判断に必要な補足を分けて記録できる。Submit時点の内容をAudit Historyと結び付けられる。
- Risks / edge cases: Descriptionが任意のため、内容が不足するRequestを完全には防げない。長さと空白のバリデーションを実装・テストする必要がある。

### Option C: 必須カテゴリと構造化フォームを持つRequest

- Behavior: Requesterは申請種別を選択し、種別ごとに定義された必須項目を入力する。
- Benefits: 承認判断に必要な情報を申請種別ごとに揃えられ、将来のワークフロー分岐にもつなげられる。
- Risks / edge cases: 申請種別、フォーム定義、管理UI、バリデーションポリシーという未決定のプロダクト領域を初回Sliceへ持ち込む。

## Decision

**必須Titleと任意Descriptionを持つ汎用Requestを採用することを推奨する。** Titleは前後空白を除いて1〜120文字、Descriptionは0〜2,000文字とする。Requesterは自分のDraftだけを作成・編集できる。Submit成功後、TitleとDescriptionは初回Sliceでは不変とし、変更要求は新しいRequestとして扱う。

Audit Historyには、Draft作成時のActorと時刻、Title/DescriptionのDraft更新時のActorと時刻、Submit時のActor・時刻およびSubmit時点の内容を記録する。Descriptionが空の場合も、空であることをSubmit時点の内容として記録する。

## Rationale

TitleだけではApproverが申請の意図を判断する情報として弱く、構造化フォームは初回Sliceの目的を超える。Titleと任意Descriptionなら、作成・Submit・Approve・Audit Historyの一連の価値を、申請種別管理を先取りせずに実証できる。Submit後に内容を不変にすることで、承認と監査がどの内容に対して行われたかを明確にする。

## Consequences

- Requesterは短い識別子と補足説明を記録でき、Approverは同じ内容を確認して判断できる。
- Draft更新とSubmit時点の内容をAudit Historyに記録するため、監査記録の量が増える。
- Submit後の訂正は既存Requestの更新ではなく新規Requestの作成となるため、誤字の修正にも再申請が必要になる。
- 将来、申請種別や添付ファイルを追加する場合は、本PDRをSupersedeするか、別PDRで後方互換な拡張を決める必要がある。

## Acceptance examples

1. Requester Aが`"  VPN access  "`とDescriptionを入力してDraftを保存すると、表示・保存されるTitleは`"VPN access"`である。
2. Requester Aが空白だけのTitleでDraft作成またはSubmitを試みると、Requestは作成・Submitされず、Titleの入力エラーが表示される。
3. Requester AがDescriptionなしでTitle `"Laptop purchase"`のDraftをSubmitすると、RequestはPendingへ進める。Audit HistoryにはSubmit時点でDescriptionが空だったことが記録される。
4. Requester AがSubmit済みRequestのTitleまたはDescriptionを変更しようとすると拒否される。Requester Aは修正内容で新しいDraftを作成できる。
5. Requester BはRequester AのDraftを閲覧または編集できない。

## Revisit conditions

- Approverが任意Descriptionだけでは判断できない事例が継続的に発生した場合
- 複数の申請種別に固有の必須項目、添付ファイル、機密区分、または外部参照が必要になった場合
- 承認済みRequestを訂正・再承認する業務要件が発生した場合
