# Local API and Compose development stack

The local stack follows Accepted [ADR-018](../decisions/architecture/ADR-018-local-compose-application-stack.md). It starts the React Frontend, Go Backend, PostgreSQL 17.11, and Keycloak 26.7.4 with Docker Compose. All published ports bind to host `127.0.0.1`. Use only development credentials and data.

## Start

Run from the repository root. Docker with Compose is required; host Go, Node.js, and pnpm are not needed to start the application containers. On the first start, create local credentials and keep them in a private password manager. On later starts with the preserved volumes, reuse the same `LOCAL_DB_PASSWORD` and Keycloak bootstrap admin credentials. Enter passwords at the prompts so they do not enter shell history:

```sh
export KEYCLOAK_ADMIN_USERNAME=local_admin
printf 'Keycloak admin password: '; read -r -s KEYCLOAK_ADMIN_PASSWORD; printf '\n'
export KEYCLOAK_ADMIN_PASSWORD
printf 'Development database password (URL-safe): '; read -r -s LOCAL_DB_PASSWORD; printf '\n'
export LOCAL_DB_PASSWORD
printf 'Local test user password: '; read -r -s TEST_USER_PASSWORD; printf '\n'
export TEST_USER_PASSWORD
export AUTH_TRANSACTION_KEY="$(openssl rand -base64 32 | tr -d '\n')"
docker compose -f compose.local.yaml up --build -d
bash scripts/provision-keycloak.sh
```

Use a URL-safe value for `LOCAL_DB_PASSWORD`, such as hex generated with `openssl rand -hex 24`; Compose passes it into the PostgreSQL URL. PostgreSQL and Keycloak initialize their stored credentials only when their volumes are first created. Changing these values in the shell does not rotate credentials in existing volumes. To start with newly generated credentials, explicitly reset the local volumes as described below. `AUTH_TRANSACTION_KEY` must decode to exactly 32 bytes. Supply all four secret values externally; do not commit a populated `.env` file or reuse production credentials. The compose startup waits for the database, applies the checked-in migrations in a separate one-shot job, and starts the API after required services are ready. The provisioning command creates or updates the development Keycloak users and application authorization data. It is safe to rerun after a restart. Keycloak realm import by itself does not grant application roles.

Open [http://127.0.0.1:5173](http://127.0.0.1:5173). The API is at `http://127.0.0.1:8080`, and the public OIDC issuer is `http://127.0.0.1:8081/realms/approval-flow-dev`. The Frontend and API are different origins on the same loopback site, as required by ADR-015. The API's internal Keycloak dial address is only for container-to-container traffic; discovery and ID token issuer validation retain the public issuer.

Check service state and the unauthenticated API response:

```sh
docker compose -f compose.local.yaml ps
curl -i http://127.0.0.1:8080/api/v1/session
```

An unauthenticated `GET /api/v1/session` should return `401`. To inspect a startup failure, run `docker compose -f compose.local.yaml logs --tail=100` and inspect the migration, Keycloak, and Backend services. Do not paste logs containing local credentials into a shared issue.

## Stop or reset

```sh
docker compose -f compose.local.yaml down
```

This stops only the local Compose project and preserves its development PostgreSQL and Keycloak volumes. Reuse the original database password and Keycloak admin credentials when starting it again. To discard **all local stack database data** deliberately, use `docker compose -f compose.local.yaml down -v`; run it only after confirming you want that reset. Close the shell or unset the exported secrets after use.

The isolated `compose.test.yaml` database and `compose.e2e.yaml` browser stack use different Compose projects and data. Do not run them alongside this stack when they need the same loopback ports. Never point the local stack at a test or production database.

## API contract and verification

The implemented API paths are listed in [OpenAPI](../../api/openapi.yaml). The local browser flow uses `/auth/oidc/login` and `/auth/oidc/callback`, the session endpoints under `/api/v1/session`, and the request and approval endpoints under `/api/v1/requests`. Authenticated writes require the server-side session, permitted Frontend Origin, and CSRF token.

For repository checks, install the pinned host toolchain described in [toolchain.md](toolchain.md) and run:

```sh
pnpm install --frozen-lockfile
pnpm run check
pnpm test
pnpm run verify:openapi
(cd backend && GOTOOLCHAIN=go1.27.1 go test ./internal/config ./internal/auth ./internal/httpapi ./cmd/api ./cmd/migrate-local -count=1)
(cd backend && GOTOOLCHAIN=go1.27.1 go vet ./...)
```

The PostgreSQL integration harness uses its own disposable database and tears it down after the run:

```sh
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
```

Run the browser E2E separately with `pnpm run test:e2e`, after stopping the local stack. Its runner creates and removes its own PostgreSQL and Keycloak resources. On local macOS, Codex requests the Chromium launch permission on the first E2E run as required by `AGENTS.md`.
