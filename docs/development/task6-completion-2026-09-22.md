# Task 6 完了記録

更新日: 2026-09-22  
対象計画: `docs/superpowers/plans/2026-09-22-task6-http-auth-session-implementation.md`  
承認: 人間のproject ownerがTask 6の実施内容を承認

## 実施内容

- OIDC login/callback、Organization選択、current session、logoutのHTTP endpointをOpenAPI契約に沿って提供した。
- 選択済みMemberとapplication databaseのroleからActorを構築し、unsafe operationに正確なOriginとsession-bound CSRF tokenを要求した。
- CSRF token rotationとunsafe requestが競合した場合にも、現在のserver-side session hashをlock下で検証するようにした。
- `cmd/api`にPostgreSQL、OIDC authenticator、request service、HTTP routerのcomposition rootとgraceful shutdownを追加した。

## Traceability

- ADR-002: Go標準`net/http`と`ServeMux`
- ADR-003: OpenAPI 3.1をHTTP契約の正本とする
- ADR-005: OIDC認証とサーバー側認可
- ADR-011: PostgreSQL上のopaque session、CSRF、OIDC transaction
- ADR-013: OIDC identityと複数Organization Memberの選択

## 検証記録

project用permission profile更新後、Docker APIとIPv4/IPv6 loopback listenerの利用可能性を確認した。Go build cacheはsandbox外の既定cacheが書込み不可のため、`GOCACHE=/private/tmp/learn-ai-go-cache`を指定した。

```bash
source /Users/nao/.nvm/nvm.sh
nvm use 26.9.0
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./cmd/api ./internal/auth ./internal/httpapi -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...
pnpm run check
git diff --check
```

すべて成功した。`pnpm run test:db`はtest専用PostgreSQL containerを作成し、`internal/store/postgres`のintegration testを成功後にcontainer、network、volumeごと破棄した。

## 未解決事項

Task 6の範囲に未解決のProduct/Architecture Decisionはない。identity provisioning workflow、Request/Approval/Audit HTTP operationは後続Taskの範囲である。
