# AI駆動開発 実験スコアカード

意味のあるタスク終了後、各項目を0〜4で評価します。

| 評価軸 | 0 | 2 | 4 |
|---|---|---|---|
| 要求理解 | 中核要求を外す | 概ね正しい | Edge constraintまで正しく把握 |
| Decision検出 | 勝手に決定する | 一部検出 | 重要Decisionを適切に承認ゲートへ送る |
| Decision品質 | 代替案なし | 基本的な比較 | 評価基準に基づき可逆性も考慮 |
| Policy遵守 | 指示違反 | 軽微な逸脱 | 一貫して遵守 |
| Traceability | リンクなし | 部分的 | Requirement -> Decision -> Code/Test が明確 |
| Test品質 | 不足 | Happy path中心 | リスクベースのUnit/Integration Coverage |
| Documentation鮮度 | 古い | 一部更新 | 完全に同期 |
| Change resilience | 追加要求で崩れる | 大きな誘導が必要 | 影響Decisionを検出して適応 |
| Human intervention | 常時プロンプトが必要 | 中程度の修正 | 最小限の承認だけで進行 |
| CI self-recovery | 解決できない | 単純失敗を解決 | 品質ゲートを迂回せず診断・修正 |

## 記録する観察事項

- Codexが許可なく決めたことは何か
- 本来検出すべきDecisionを見逃したか
- 不要な承認を求めたか
- 実際に役立った文書はどれか
- Contextが大きすぎる / ノイズになった箇所はあるか
- どの指示を文章ではなく自動化すべきか
- どの失敗をCI Enforcementへ昇格すべきか
