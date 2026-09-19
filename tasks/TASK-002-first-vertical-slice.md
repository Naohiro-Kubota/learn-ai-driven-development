# TASK-002: 最初のVertical Sliceを実装する

Status: TASK-001で必要と判断された基盤ADRがAcceptedになるまでBLOCKED。

## Goal

以下を実証する最小のEnd-to-End Sliceを実装してください。

- Draft申請作成
- Request Submit
- Pending approval表示
- Request Approve
- Approved result表示
- 上記操作のAudit History表示

## Rules

- 最初に関連するAccepted ADR / PDRをすべて読む
- 要求から未解決の重要なProduct Behaviorが見つかった場合、PDRを提案し、そのBehaviorについては承認ゲートで停止する
- Accepted Decisionで扱われていない基盤Dependencyを追加しない
- Accepted Testing Strategyに沿ったテストを追加する
- ドキュメントを更新する
- TraceabilityとValidation evidenceを報告する
