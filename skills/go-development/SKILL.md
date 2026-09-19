# Go開発Knowledge

Goコードを変更する場合に使用してください。

## 目的

Project Decisionを守りながら、モダンで読みやすく、テストしやすいGoコードを作成します。

## Guidance

- 標準ライブラリで十分な場合は優先する
- Packageは凝集度を高くし、境界を明示する
- Blocking、I/O、Request Lifetimeの意味を持つ処理には `context.Context` を適切に伝播する
- 呼び出し側でError Identityが必要な場合は、それを保ちながら有用な文脈を付与する
- Accepted ADRが明示的に選択していない限り、Core Business RuleをFramework固有コードへ強く結合しない
- Concurrency ownership と cancellation を明示する
- Coverageと可読性が向上する場合はTable-driven testを活用する
- 完了前にRepositoryで定義されたformatter、static analysis、testを実行する

## 外部参照

プロジェクトではJetBrainsの `go-modern-guidelines` をKnowledge Source候補として評価してよい。
外部Guidanceを利用する場合は、内容が現時点で有効か確認し、Accepted Project Decisionと競合しないことを確認する。
