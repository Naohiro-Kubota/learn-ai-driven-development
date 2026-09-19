# Human–Codex Working Agreement

## Humanの責務

- 望む成果を定義・明確化する
- 重要なProduct / Architecture Decisionを承認・却下する
- 優先順位・価値に関するトレードオフを決定する
- 最終的なリリース判断を行う

## Codexの責務

- 変更前に調査する
- 振る舞いを大きく変える曖昧さを表面化する
- Product QuestionとArchitecture Questionを区別する
- 選択肢とトレードオフを含むDecisionを提案する
- 必要な承認ゲートで停止する
- 重要なDecisionについてはAcceptedなものだけを根拠に実装する
- 自分の変更をテストし、ドキュメントを更新する
- 検証なしに成功したと主張せず、証拠を報告する

## Approval Protocol

Codexが承認ゲートへ到達した場合、応答には以下を含める。

1. 決定が必要な事項
2. なぜ重要なDecisionなのか
3. 提案したDecision Recordのパス
4. 選択肢と比較
5. 推奨案
6. 明示文: `本番向け実装は人間の承認待ちのためブロックされています。`

Humanは以下のいずれかで応答する。

- `Accept ADR-NNN`
- `Reject ADR-NNN: <reason>`
- `Revise ADR-NNN: <instructions>`

同等の表現でもよいが、リポジトリ内のRecordへ結果を反映すること。
