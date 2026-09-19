# Codexへ最初に渡すプロンプト例

## Prompt 1 — リポジトリ理解

リポジトリ内の指示とプロダクト文書を読んでください。
本番向けコードは書かないでください。

以下を要約してください。

1. プロダクトの目的
2. 固定された制約
3. 意図的に未決定としている事項
4. Human / Codex の責任分界
5. Human Approvalのために停止しなければならない条件

また、リポジトリ内の指示に矛盾または曖昧さがあれば指摘してください。

## Prompt 2 — 基盤Decision

`tasks/TASK-001-foundation-decisions.md` を実行してください。

本番向けアプリケーションコードは実装しないでください。
Proposed ADRのみ作成し、人間の承認待ちで停止してください。

## Prompt 3 — Human Approval後

必要なADRを私が明示的にAcceptした後、`tasks/TASK-002-first-vertical-slice.md` を実行してください。

実装前に、今回依拠するAccepted Decisionを一覧化し、残っているApproval Gateがあれば示してください。
