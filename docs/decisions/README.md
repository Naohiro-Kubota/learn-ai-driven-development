# Decision Record

## 目的

仕様書は「現在期待されている振る舞い」を記述します。
Decision Recordは「なぜその重要な選択をしたのか」「どの代替案を検討したのか」「いつ再検討すべきか」を保存します。

## 種類

### ADR — Architecture Decision Record
重要な技術構造、プラットフォーム、依存関係、横断的なEngineering Decisionに使用します。

保存先: `docs/decisions/architecture/`

### PDR — Product Decision Record
重要なプロダクトの振る舞い、ポリシー、ワークフロー、業務ルールの決定に使用します。

保存先: `docs/decisions/product/`

## ステータス遷移

`Proposed -> Accepted | Rejected`

Accepted Decisionが後から置き換えられた場合、古いDecisionを `Superseded` として新しいRecordへリンクします。

## 承認ルール

Codexは Proposed 状態のRecordを作成・更新できます。

人間が特定のDecisionについて明示的に委譲しない限り、`Accepted` または `Rejected` への変更は人間だけが行います。

## 何をDecision Recordにすべきか

以下のいずれかを満たす場合、Record作成を検討してください。

- 後から戻すコストが高い
- 複数の妥当な選択肢が存在する
- 複数モジュール/チームに影響する
- 再利用されるプロジェクト共通ルールを作る
- セキュリティ、データ整合性、運用性、ユーザー挙動を大きく変える
- 将来の保守者が「なぜこうなっているのか」と疑問を持ちそう

局所的で自明な実装詳細にはRecordを作成しないでください。
