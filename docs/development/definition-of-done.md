# Definition of Done

変更は、該当する以下の条件をすべて満たすまでDoneではありません。

## Behavior
- 要求された振る舞いがRequirementsおよびAccepted Product Decisionと一致している
- Edge caseおよび認可への影響が検討されている

## Architecture
- 実装がAccepted ADRと一致している
- 未承認の重要なArchitecture Decisionを新たに導入していない
- 新しい依存関係に明示的な理由がある

## Verification
- 適切な自動テストが存在する
- Formatter / Linter が成功する
- 設定済みであれば型チェック / Static Analysisが成功する
- 該当するUnit / Integration / E2E Testが成功する

## Documentation
- Requirementsが現在の振る舞いと一致している
- 関連するDecision Recordがリンクまたは更新されている
- Architecture / API / Developer Documentationが同期されている

## Traceability
完了報告には以下を含める。

- 対応したRequirement ID
- 依拠したDecision Record
- 実行した検証コマンド
- 既知の制約・Follow-up
