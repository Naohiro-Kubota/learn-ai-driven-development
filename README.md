# AI-Driven Development Lab

Codex を主エージェントとして、AI駆動開発のプロセスそのものを検証するための実験用リポジトリです。

## 実験の目的

「AIがコードを書けるか」ではなく、以下を検証します。

- 要求・制約・既存DecisionをAIが正しく参照できるか
- 重要な意思決定が必要な場面をAIが検出できるか
- ADR / Product Decision Record を提案し、人間の承認を待てるか
- 承認済みDecisionと整合する実装・テスト・ドキュメント更新を行えるか
- 後から仕様変更を投入した際に、既存Decisionとの矛盾や再検討条件を検出できるか
- CIを品質ゲートとして利用し、自律的に修正できるか

## 固定する技術制約

- Frontend language: TypeScript
- Backend language: Go
- Primary coding agent: Codex

フレームワーク、主要ライブラリ、データストア、API方式、認証方式、非同期処理方式などは初期状態では決定しません。
重要な選定は ADR として提案し、人間が承認してから採用します。

## 人間とAIの責任分担

### Human

- 要求の提示
- 重要な仕様判断
- ADR / Decision Record の承認・却下
- リリース可否などの最終判断

### Codex

- 既存コード・文書の調査
- 設計案・選択肢の提示
- ADR / Decision Record の草案作成
- 承認済みDecisionに基づく実装
- テスト
- CIエラー修正
- 関連ドキュメント更新
- 変更結果と未解決事項の報告

## 最初の進め方

1. `AGENTS.md` を読む
2. `docs/product/` を読む
3. `docs/decisions/README.md` を読む
4. `tasks/TASK-001-foundation-decisions.md` を Codex に依頼する
5. Codex が作成した ADR 候補を人間がレビューする
6. Accepted になったDecisionだけを実装する
