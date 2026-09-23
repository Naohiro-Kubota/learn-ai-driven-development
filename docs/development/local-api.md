# Local API

This document describes a safe local run of the Go API. It is for loopback-only
development and test databases. Do not use a production database, production
OIDC issuer, or real credentials in a shell history, this repository, or a
shared environment.

## 1. Fixed toolchain and install

Run from the repository root. Use these exact versions:

- Node.js `26.9.0` (`.node-version`)
- pnpm `12.5.1` (`package.json`)
- Go `1.27.1` (`.go-version` and `go.mod`)
- PostgreSQL `17.11` for the repository's test container
- Keycloak `26.7.4` with the pinned image digest in
  [`docs/development/toolchain.md`](toolchain.md)

Confirm the active versions before continuing:

```sh
node --version
pnpm --version
go version
```

Install exactly from the lockfile. Do not run an unfrozen install or modify the
lockfile as part of a local API run:

```sh
pnpm install --frozen-lockfile
```

## 2. Verify the isolated test database

The repository-provided database script is the supported integration-test
entrypoint:

```sh
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
```

`scripts/test-postgres.mjs` starts the test-only PostgreSQL 17.11 container on
loopback `127.0.0.1:55432`, injects the isolated
`TEST_DATABASE_URL` into the Go PostgreSQL tests, and runs the tests with
`GOTOOLCHAIN=go1.27.1`. The tests apply the checked-in migrations from
`migrations/` in version order. The script always runs `docker compose -f
compose.test.yaml down -v` afterward, including after a failure.

This test database is not the API's development database. Do not point
`DATABASE_URL` at it while the script is running or reuse it after the script
has cleaned it up.

## 3. Prepare separate databases and migrations

Create or select a separate local development database and a separate local
test database. Both must be reachable only from the local machine. Never copy
production data into either database.

The API does not run migrations at startup. Apply the checked-in migrations to
the separate development database with the repository's explicit local CLI:

```sh
export DATABASE_URL='<separate local development database URL>'
GOTOOLCHAIN=go1.27.1 go run ./cmd/migrate-local up
```

`up` is the default when the command is omitted. The CLI also accepts
`-database-url '<local URL>'`, and supports `down` and `version`; it never
prints the URL. Do not use it with a production database. Do not edit the
schema manually or run migrations by starting `cmd/api`.

For automated migration and PostgreSQL verification, use only:

```sh
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
```

That command is intentionally destructive to its temporary test database and
does not migrate the separate development database.

## 4. Start and provision loopback-only Keycloak

The checked-in compose file uses Keycloak `26.7.4` with the exact digest in
[`docs/development/toolchain.md`](toolchain.md), `start-dev`, the imported
`keycloak/realms/approval-flow-dev-realm.json`, and only
`127.0.0.1:8081`. It contains no database service and is for local development
only. Supply all credentials externally and keep them out of shell history and
the repository:

```sh
export KEYCLOAK_ADMIN_USERNAME='<local bootstrap admin username>'
read -r -s KEYCLOAK_ADMIN_PASSWORD
export KEYCLOAK_ADMIN_PASSWORD
read -r -s TEST_USER_PASSWORD
export TEST_USER_PASSWORD
docker compose -f compose.local.yaml up -d
bash scripts/provision-keycloak.sh
```

After Keycloak starts, the provisioning script waits for the first admin
authentication to succeed, retrying a bounded number of times before it
provisions users. It logs in with `kcadm`, creates or updates the development
`requester`, `approver`, and `admin` users, and prints only the issuer and
public client values. It passes passwords through stdin where supported. Do
not expose Keycloak on `0.0.0.0` or a network interface shared with other
users. Stop it with `docker compose -f compose.local.yaml down` when finished.

Keycloak users and client configuration do not provision application
authorization. Before testing a login, explicitly provision the application
database: create the local Organization and Members, add the required
Requester/Approver/Admin roles, map each Keycloak `iss` + `sub` to its Member
in `oidc_identities`/`member_oidc_identities`, and set the same-Organization
default Approver where required. This DB mapping is a prerequisite for an
authenticated API session; Keycloak realm roles are not an authorization
source. Keep this data in a separate local database and do not commit identity
or password values.

## 5. Configure and start the API

Export the following values in the process environment. Use a separate local
development database URL for `DATABASE_URL`; do not record its password here.
All URLs below are examples of loopback addresses and must match the local
Keycloak realm/client configuration.

```sh
export APP_ENV=development
export APP_LISTEN_ADDR=127.0.0.1:8080
export APP_FRONTEND_ORIGIN=http://127.0.0.1:5173
export APP_COOKIE_SECURE=false
export DATABASE_URL='<separate local development database URL>'
export OIDC_ISSUER='http://127.0.0.1:8081/realms/approval-flow-dev'
export OIDC_CLIENT_ID='<local-client-id>'
export OIDC_REDIRECT_URI='http://127.0.0.1:8080/auth/oidc/callback'
export AUTH_TRANSACTION_KEY='<base64 encoding of exactly 32 random bytes>'
export SESSION_IDLE_TTL='15m'
export SESSION_ABSOLUTE_TTL='8h'
export AUTH_TRANSACTION_TTL='5m'
```

`AUTH_TRANSACTION_KEY` must decode as standard base64 to exactly 32 bytes. Use
a newly generated local-only value supplied by the environment; never use a
sample value or a production key. Every `SESSION_*` and
`AUTH_TRANSACTION_TTL` value must be a positive Go duration. The local
configuration deliberately permits insecure cookies only when `APP_ENV` is
`development`, the API listens on loopback, and `APP_FRONTEND_ORIGIN` is a
loopback URL. Use `APP_COOKIE_SECURE=true` when testing through HTTPS.

After the separate development database has been migrated and Keycloak is
ready, start the API:

```sh
GOTOOLCHAIN=go1.27.1 go run ./cmd/api
```

The API listens on `127.0.0.1:8080`. Every implemented method/path in the
authoritative [`api/openapi.yaml`](../../api/openapi.yaml) contract is:

- `GET /auth/oidc/login`
- `GET /auth/oidc/callback`
- `GET /auth/oidc/organization-selection`
- `POST /auth/oidc/organization-selection`
- `GET /api/v1/session`
- `POST /api/v1/session/logout`
- `POST /api/v1/requests`
- `GET /api/v1/requests/pending`
- `GET /api/v1/requests/{requestId}`
- `PATCH /api/v1/requests/{requestId}`
- `POST /api/v1/requests/{requestId}/submit`
- `POST /api/v1/requests/{requestId}/approvals`
- `GET /api/v1/requests/{requestId}/audit-events`

## 6. Verification

Run these checks from another terminal while keeping the API terminal
available. OpenAPI validation and the following Go packages do not require a
live PostgreSQL database or Keycloak:

```sh
pnpm run verify:openapi
node --test scripts/verify-openapi.test.mjs
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go test ./internal/config ./internal/auth ./internal/httpapi ./cmd/api ./cmd/migrate-local -count=1
GOCACHE=/private/tmp/learn-ai-go-cache GOTOOLCHAIN=go1.27.1 go vet ./...
```

Run the PostgreSQL integration suite separately with its isolated harness:

```sh
GOCACHE=/private/tmp/learn-ai-go-cache pnpm run test:db
```

The integration command supplies `TEST_DATABASE_URL` itself and applies and
removes migrations in the temporary test database. Do not set
`TEST_DATABASE_URL` to the development or production database.

## 7. Cleanup

Stop the API with `Ctrl-C`. Stop and remove only the local Keycloak container
and its local-only data using the same compose/container command that started
it. Do not remove unrelated containers or volumes.

The test harness cleans its own PostgreSQL container and volume. If it was
interrupted before its `finally` cleanup ran, inspect the exact compose project
first, then run:

```sh
docker compose -f compose.test.yaml down -v
```

Unset the exported API variables or close the terminal. Keep local credentials
and generated keys outside the repository, and verify that no secret-bearing
files were created before sharing changes.
