# Reactフロントエンド Task 1 完了記録

- 日付: 2026-09-23
- 状態: 完了。[PR #21](https://github.com/Naohiro-Kubota/learn-ai-driven-development/pull/21)を`develop`へマージ済み（merge commit `4a811ae212d027b3c282d330f58213b5432c6efc`）。
- 対象: [React Frontend実装計画のTask 1](../superpowers/plans/2026-09-23-react-frontend-implementation.md)と[Task 1補完計画](../superpowers/plans/2026-09-23-react-frontend-task1-completion.md)。

## 実施内容

Task 1着手時点で、`APP_FRONTEND_ORIGIN`の検証、単一Originのcredentialed CORS、OIDC callbackとOrganization選択後の固定Frontend redirectは実装済みだった。PR #21では、重複したpreflightヘッダーの拒否を追加し、CORSを`cors.go`へ分離した。拒否されたrequestがhandlerへ届かないこと、許可Originから401/403応答を読めること、設定値と固定redirectの境界をテストで確認した。OpenAPIのcallback説明も実装と一致させた。

PR #21の変更ファイルは`api/openapi.yaml`、`internal/config/config_test.go`、`internal/httpapi/cors.go`、`internal/httpapi/cors_test.go`、`internal/httpapi/router.go`、`docs/superpowers/plans/2026-09-23-react-frontend-task1-completion.md`の6件。新しい本番依存関係はない。

## 要求・Decisionとの対応

- FR-012: 別OriginのReact UIが利用するAPI transportと固定Frontend redirectの境界。
- NFR-001: 単一Originへの厳格なCORS許可、重複preflight拒否、既存のsession・CSRF検証の維持。
- NFR-004: CORS処理を独立したファイルへまとめ、既存の標準ライブラリ実装を維持。
- NFR-006: loopback開発時のFrontend originとcookie設定の検証。
- ADR-011: `SameSite=Lax`、server-side session、CSRFの境界を維持。
- ADR-015: 同一site内の別Origin、`APP_FRONTEND_ORIGIN`の単一正本、固定Frontend redirectに従う。

新たな重要なProduct DecisionまたはArchitecture Decisionは必要なかった。

## 検証とレビュー

| 検証 | 結果 |
| --- | --- |
| 重複preflightのred/green | 修正前は不正な重複ヘッダーを`204`で受け入れるテスト失敗を確認。修正後は同テスト成功。 |
| `pnpm install --frozen-lockfile` | PASS |
| `pnpm run verify:openapi` | PASS |
| `node --test scripts/verify-openapi.test.mjs` | PASS（11件成功、1件スキップ、失敗0件） |
| `pnpm run check:gofmt` | PASS |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 TEST_DATABASE_URL=<一時ローカルDB> go test ./... -count=1` | PASS（全Go package。一時PostgreSQLを起動し、終了後にcontainer・networkを削除） |
| `GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...` | PASS |
| `git diff --check` | PASS |

implementerによる実装後、Task 1.1とTask 1.2をそれぞれreviewerが確認し、最終の横断レビューも有効な指摘なしで完了した。

## 残る範囲

この記録はTask 1の完了を示す。ReactのAPI client・画面は親計画のTask 2・3、申請者と承認者を使うブラウザE2EはTask 4で確認する。cross-site配置、複数Frontend origin、配信基盤の具体的な選定が必要になった場合はADR-015の再検討条件に従ってDecisionを起こす。
