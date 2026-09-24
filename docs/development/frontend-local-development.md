# Frontend local development and browser E2E

Run commands from the repository root. The supported versions are Node.js `26.9.0`, pnpm `12.5.1`, Go `1.27.1`, PostgreSQL `17.11`, and Keycloak `26.7.4` with the pinned image digest in [toolchain.md](toolchain.md). Docker with Compose and a Chromium browser installed by Playwright are required for the browser test. Install the locked packages and browser once:

```sh
pnpm install --frozen-lockfile
pnpm exec playwright install chromium
```

## Manual development

Follow [Local API](local-api.md) to create a **separate development database**, apply migrations, provision the local Keycloak realm and users, and map their verified `(iss, sub)` identities to application Members and roles. A Keycloak user alone has no application authorization. The app DB mapping and same-Organization default Approver are required for the Requester → Approver flow. Do not use the temporary test database as the development database.

The example settings are listed in [`.env.example`](../../.env.example). That file is a checklist, not a source of credentials or an automatically loaded Go configuration. Supply `DATABASE_URL`, `OIDC_CLIENT_ID`, and a fresh standard-base64 `AUTH_TRANSACTION_KEY` encoding exactly 32 random bytes in the API process environment. Keep all secrets out of the repository and shell history. Start the migrated API as described in [Local API](local-api.md):

```sh
GOTOOLCHAIN=go1.27.1 go run ./cmd/api
```

In another terminal, start Vite:

```sh
VITE_API_ORIGIN=http://127.0.0.1:8080 pnpm run dev --host 127.0.0.1
```

Open `http://127.0.0.1:5173`. The API is `http://127.0.0.1:8080`; Keycloak issuer is `http://127.0.0.1:8081/realms/approval-flow-dev`. These are separate origins on the same loopback site. `APP_FRONTEND_ORIGIN` must be the Vite origin, and `VITE_API_ORIGIN` must be the API origin. Keycloak's OIDC callback is `http://127.0.0.1:8080/auth/oidc/callback`; successful login and Organization selection return to the Frontend origin. The development-only HTTP cookie setting requires `APP_ENV=development` and loopback addresses. Stop Vite and the API with `Ctrl-C`, then stop only the Keycloak stack started for manual development using `docker compose -f compose.local.yaml down`.

## Isolated browser E2E

Run the complete browser flow with one command:

```sh
pnpm run test:e2e
```

The runner checks that loopback ports `8080`, `8081`, `5173`, and `55432` are free before starting. It creates a uniquely named `approval-flow-e2e-*` Compose project with its own PostgreSQL volume and Keycloak, runs migrations, provisions fresh Keycloak `requester`, `approver`, and `multi` users and matching app DB memberships, starts Go API and Vite, then runs Playwright. The Requester and Approver use separate browser contexts; the `multi` user selects one of two Organization memberships. The runner generates credentials and an auth transaction key per run, removes its own containers, volume, child processes, Go cache, and Playwright output directory even after a test failure. It does not touch `compose.local.yaml`, `compose.test.yaml`, or their data. Do not point it at a development or production database.

If a fixed port is occupied, the runner fails before Compose startup. Stop or move the process you own before retrying; do not remove an unknown service. If Docker or browser startup fails, check that the Docker daemon is available, Chromium is installed, and local execution is permitted to start Docker, bind loopback ports, and launch a browser. The runner reports only validated diagnostic fields from Playwright; it intentionally suppresses raw request headers, cookies, response bodies, traces, HAR, video, and screenshots. Diagnose failures with the failing test name and phase; avoid copying secrets into logs or reports. If interrupted outside normal cleanup, inspect the exact `approval-flow-e2e-*` project named by the runner before removing only that project's resources.

The following checks cover other layers separately:

```sh
pnpm run format:check
pnpm run lint
pnpm run typecheck
pnpm test
pnpm run test:e2e:runner
pnpm run build
pnpm run check:gofmt
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
```

`test:db` creates and removes its own `compose.test.yaml` PostgreSQL service on port `55432`; run it **sequentially** with `test:e2e`. Bare `go test ./...` needs `TEST_DATABASE_URL` for the PostgreSQL package; use `test:db` for that integration suite.
