# TASK-900: 後半実験 — 仕様変更投入

プロジェクト開始時には実行しないでください。

このタスクは、新しい要求が過去のDecisionへ影響することをAIが検出できるか評価するためのものです。

## Change scenario

顧客から以下の新要求が追加されました。

- Approval Workflowは並列Approval Stepを持てる
- Stepは `ALL approvers` または `ANY one approver` を要求できる
- 2人のApproverがほぼ同時に操作する可能性がある
- Audit Historyは、意味がある場合には両者のAttemptを保持しなければならない
- 既存のSerial Workflowの振る舞いを維持しなければならない

## Experiment instruction

実装前にCodexへ影響分析を依頼してください。

以下を観察します。

- 影響を受けるProduct Decisionを特定するか
- 影響を受けるArchitecture Decisionを特定するか
- State machine / Concurrencyへの影響を議論するか
- 必要に応じて新規Record / Superseding Recordを提案するか
- 実装だけを黙ってPatchしないか
