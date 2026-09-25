# Frontend local development and browser E2E

Run commands from the repository root. The supported versions are Node.js `26.9.0`, pnpm `12.5.1`, Go `1.27.1`, PostgreSQL `17.11`, and Keycloak `26.7.4` with the pinned image digest in [toolchain.md](toolchain.md). Docker with Compose and a Chromium browser installed by Playwright are required for the browser test. For repository tests, install the locked packages and browser once:

```sh
pnpm install --frozen-lockfile
pnpm exec playwright install chromium
```

## Local Compose development

Follow [Local API](local-api.md) to supply local credentials, run `docker compose -f compose.local.yaml up --build -d`, and provision the Keycloak users and application memberships with `bash scripts/provision-keycloak.sh`. The Compose stack starts Vite, the Go API, PostgreSQL, and Keycloak; the migration job completes before the API starts. Open `http://127.0.0.1:5173` after provisioning. Stop it with `docker compose -f compose.local.yaml down` to preserve the development database volume.

The Frontend origin is `http://127.0.0.1:5173`, the API origin is `http://127.0.0.1:8080`, and the Keycloak issuer is `http://127.0.0.1:8081/realms/approval-flow-dev`. These URLs match the realm callback, cookie, and CORS configuration in ADR-015. The test user's Keycloak identity must be mapped to an application Member and role before login succeeds; the provisioning command performs this mapping. No production credentials or data belong in this stack.

## Isolated browser E2E

Run the complete browser flow with one command:

```sh
pnpm run test:e2e
```

When Codex runs this command on local macOS, it must request the needed execution permission on the first attempt: call `exec_command` with `sandbox_permissions: "require_escalated"` and `prefix_rule: ["pnpm", "run", "test:e2e"]`. Approval and automatic review still apply. A sandboxed Chromium launch can fail with `MachPortRendezvousServer ... Permission denied`; do not count that expected permission failure as the browser E2E result. Report the outcome of the permission granted run, or report that approval was denied. The command above remains unchanged for a developer's own terminal.

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
(cd backend && GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...)
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
```

`test:db` creates and removes its own `compose.test.yaml` PostgreSQL service on port `55432`; run it **sequentially** with `test:e2e`. Bare `go test ./...` needs `TEST_DATABASE_URL` for the PostgreSQL package; use `test:db` for that integration suite.
