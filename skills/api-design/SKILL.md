# API設計Knowledge

Frontend / Backend間の契約を提案・変更する場合に使用します。

- Product use case と Domain language から設計を開始する
- Authorization と Error semantics を明示する
- 状態変更操作に対するConcurrency behaviorを定義する
- 理由なく内部Persistence representationをPublic Contractへ漏らさない
- Idempotency と retry behavior を検討する
- Compatibility / Versioningを意識的なDecisionとして扱う
- REST、RPC、GraphQL、Generated Client、Schema-first Toolingなど、プロジェクト全体へ影響する契約方式を選択する場合は実装前にADRを作成する
