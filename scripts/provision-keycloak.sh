#!/usr/bin/env bash
set -Eeuo pipefail

repo_root="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)"
compose=(docker compose -f "$repo_root/compose.local.yaml")
realm="approval-flow-dev"
server="http://127.0.0.1:8080"
host_issuer="http://127.0.0.1:8081/realms/approval-flow-dev"
kcadm_config="/tmp/kcadm-config-$$"

required_env() {
	local name="$1"
	if [[ -z "${!name:-}" ]]; then
		echo "$name must be supplied externally" >&2
		exit 1
	fi
}

required_env KEYCLOAK_ADMIN_USERNAME
required_env KEYCLOAK_ADMIN_PASSWORD
required_env TEST_USER_PASSWORD

kcadm() {
	"${compose[@]}" exec -T keycloak /opt/keycloak/bin/kcadm.sh --config "$kcadm_config" "$@"
}

readiness_max_attempts=10
readiness_retry_delay_seconds="${KEYCLOAK_READINESS_RETRY_DELAY_SECONDS:-2}"

authenticate_admin() {
	local attempt
	for ((attempt = 1; attempt <= readiness_max_attempts; attempt++)); do
		if printf '%s\n' "$KEYCLOAK_ADMIN_PASSWORD" | kcadm config credentials \
			--server "$server" --realm master --user "$KEYCLOAK_ADMIN_USERNAME" \
			>/dev/null 2>&1; then
			return 0
		fi
		if (( attempt < readiness_max_attempts )); then
			sleep "$readiness_retry_delay_seconds"
		fi
	done
	echo "Keycloak admin authentication did not become ready after $readiness_max_attempts attempts" >&2
	return 1
}

cleanup() {
	"${compose[@]}" exec -T keycloak rm -f "$kcadm_config" >/dev/null 2>&1 || true
}
trap cleanup EXIT

authenticate_admin

ensure_user() {
	local username="$1"
	local email="$2"
	local user_id
	user_id="$(kcadm get users -r "$realm" -q "username=$username" --fields id --format csv --noquotes 2>/dev/null | tail -n 1 | tr -d '\r')"
	if [[ -z "$user_id" ]]; then
		kcadm create users -r "$realm" -s "username=$username" -s enabled=true -s "email=$email" >/dev/null
	else
		kcadm update "users/$user_id" -r "$realm" -s enabled=true -s "email=$email" >/dev/null
	fi
	printf '%s\n' "$TEST_USER_PASSWORD" | kcadm set-password -r "$realm" \
		--username "$username" >/dev/null
}

ensure_user requester requester@approval-flow.invalid
ensure_user approver approver@approval-flow.invalid
ensure_user admin admin@approval-flow.invalid

printf 'OIDC_ISSUER=%s\n' "$host_issuer"
printf 'OIDC_CLIENT_ID=approval-flow\n'
printf 'OIDC_REDIRECT_URI=http://127.0.0.1:8080/auth/oidc/callback\n'
