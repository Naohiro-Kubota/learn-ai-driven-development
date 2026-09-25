#!/usr/bin/env bash
set -Eeuo pipefail

script="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)/scripts/provision-keycloak.sh"

if grep -Fq 'set -Eeuo pipefail' "$script"; then
  :
else
  echo "provision script must enable strict mode" >&2
  exit 1
fi
grep -Fq 'kcadm_config="/tmp/kcadm-config-$$"' "$script"
grep -Fq '"${compose[@]}" exec -T keycloak /opt/keycloak/bin/kcadm.sh "$@" --config "$kcadm_config"' "$script"
grep -Fq 'printf '\''%s\n'\'' "$KEYCLOAK_ADMIN_PASSWORD" | kcadm_secret config credentials' "$script"
grep -Fq 'printf '\''%s\n'\'' "$TEST_USER_PASSWORD" | kcadm_secret set-password' "$script"
grep -Fq 'trap cleanup EXIT' "$script"
grep -Fq 'rm -f "$kcadm_config"' "$script"
grep -Fq 'http://127.0.0.1:8081/realms/approval-flow-dev' "$script"
if grep -Eq -- '--(password|new-password)([=:[:space:]]|$)' "$script"; then
  echo "provision script must not pass passwords as arguments" >&2
  exit 1
fi

fake_root="$(mktemp -d)"
trap 'rm -rf "$fake_root"' EXIT
fake_log="$fake_root/log"
mkdir -p "$fake_log"
cat >"$fake_root/docker" <<'EOF'
#!/usr/bin/env bash
set -Eeuo pipefail

: "${FAKE_DOCKER_LOG:?}"
index_file="$FAKE_DOCKER_LOG/index"
if [[ -f "$index_file" ]]; then
  index="$(<"$index_file")"
else
  index=0
fi
index=$((index + 1))
printf '%s\n' "$index" >"$index_file"
printf '%s\n' "$@" >"$FAKE_DOCKER_LOG/$index.args"
cat >"$FAKE_DOCKER_LOG/$index.stdin"

  case " $* " in
  *' config credentials '*)
    credentials_attempts_file="$FAKE_DOCKER_LOG/credentials-attempts"
    if [[ -f "$credentials_attempts_file" ]]; then
      credentials_attempts="$(<"$credentials_attempts_file")"
    else
      credentials_attempts=0
    fi
    credentials_attempts=$((credentials_attempts + 1))
    printf '%s\n' "$credentials_attempts" >"$credentials_attempts_file"
    if (( credentials_attempts <= ${FAKE_DOCKER_TRANSIENT_CREDENTIAL_FAILURES:-0} )); then
      echo "deliberate transient fake docker failure" >&2
      exit 42
    fi
    if [[ "${FAKE_DOCKER_FAIL_AFTER_CREDENTIALS:-false}" == true ]]; then
      echo "deliberate fake docker failure after credentials" >&2
      exit 42
    fi
    ;;
  *' get users '*)
    case " $* " in
      *' username=requester '*)
        if [[ "${FAKE_DOCKER_BAD_SUBJECT:-false}" == true ]]; then
          printf '%s\n' 'invalid-subject'
        else
          printf '%s\n' '11111111-1111-4111-8111-111111111111'
        fi
        ;;
      *' username=approver '*) printf '%s\n' '22222222-2222-4222-8222-222222222222' ;;
      *' username=admin '*) printf '%s\n' '33333333-3333-4333-8333-333333333333' ;;
    esac
    ;;
  *' create users '*) ;;
  *' update users/'*) ;;
  *' set-password '*) ;;
  *' postgres psql '*) ;;
  *' rm -f '*) ;;
  *)
    echo "unexpected fake docker invocation" >&2
    exit 1
    ;;
esac
EOF
chmod +x "$fake_root/docker"

admin_password='admin-password-for-test-only'
user_password='user-password-for-test-only'
if ! PATH="$fake_root:$PATH" \
FAKE_DOCKER_LOG="$fake_log" \
KEYCLOAK_ADMIN_USERNAME='admin' \
KEYCLOAK_ADMIN_PASSWORD="$admin_password" \
TEST_USER_PASSWORD="$user_password" \
LOCAL_DB_PASSWORD='test-db-password' \
AUTH_TRANSACTION_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' \
KEYCLOAK_READINESS_RETRY_DELAY_SECONDS=0 \
FAKE_DOCKER_TRANSIENT_CREDENTIAL_FAILURES=2 \
bash "$script" >"$fake_root/output" 2>"$fake_root/error"; then
  cat "$fake_root/error" >&2
  exit 1
fi

if grep -Fq "$admin_password" "$fake_root/output" "$fake_root/error" "$fake_log"/*.args; then
  echo "administrator password leaked to output or arguments" >&2
  exit 1
fi
if grep -Fq "$user_password" "$fake_root/output" "$fake_root/error" "$fake_log"/*.args; then
  echo "user password leaked to output or arguments" >&2
  exit 1
fi
grep -Fq 'config' "$fake_log/1.args"
grep -Fq 'credentials' "$fake_log/1.args"
cmp -s <(printf '%s\n' "$admin_password") "$fake_log/1.stdin"
[[ "$(<"$fake_log/credentials-attempts")" -eq 3 ]]

set_password_count=0
config_path=''
for args_file in "$fake_log"/*.args; do
  grep -Fq -- '--config' "$args_file" || continue
  next_is_config=false
  while IFS= read -r argument; do
    if [[ "$next_is_config" == true ]]; then
      if [[ -z "$config_path" ]]; then
        config_path="$argument"
      else
        [[ "$argument" == "$config_path" ]]
      fi
      next_is_config=false
    elif [[ "$argument" == '--config' ]]; then
      next_is_config=true
    fi
  done <"$args_file"
  if grep -Eq -- '--(password|new-password)(=|$|[[:space:]])' "$args_file"; then
    echo "password flag found in fake docker arguments" >&2
    exit 1
  fi
  if grep -Fq 'set-password' "$args_file"; then
    set_password_count=$((set_password_count + 1))
    stdin_file="${args_file%.args}.stdin"
    cmp -s <(printf '%s\n' "$user_password") "$stdin_file"
  elif grep -Fq 'config' "$args_file" && grep -Fq 'credentials' "$args_file"; then
    :
  else
    stdin_file="${args_file%.args}.stdin"
    if grep -Fq "$admin_password" "$stdin_file" || grep -Fq "$user_password" "$stdin_file"; then
      echo "password captured on stdin for an unexpected invocation" >&2
      exit 1
    fi
  fi
done
[[ "$config_path" == /tmp/kcadm-config-* ]]
[[ "$set_password_count" -eq 3 ]]

psql_count=0
for args_file in "$fake_log"/*.args; do
  if grep -Fq 'psql' "$args_file"; then
    psql_count=$((psql_count + 1))
    sql_file="${args_file%.args}.stdin"
    grep -Fq 'ON CONFLICT' "$sql_file"
    grep -Fq 'default_approver_member_id' "$sql_file"
  fi
done
[[ "$psql_count" -eq 1 ]]

PATH="$fake_root:$PATH" FAKE_DOCKER_LOG="$fake_log" \
  KEYCLOAK_ADMIN_USERNAME='admin' KEYCLOAK_ADMIN_PASSWORD="$admin_password" \
  TEST_USER_PASSWORD="$user_password" LOCAL_DB_PASSWORD='test-db-password' \
  AUTH_TRANSACTION_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' \
  KEYCLOAK_READINESS_RETRY_DELAY_SECONDS=0 bash "$script" >"$fake_root/repeat-output" 2>"$fake_root/repeat-error"
psql_count=0
for args_file in "$fake_log"/*.args; do
  if grep -Fq 'psql' "$args_file"; then psql_count=$((psql_count + 1)); fi
done
[[ "$psql_count" -eq 2 ]]

bad_log="$fake_root/bad-log"
mkdir -p "$bad_log"
if PATH="$fake_root:$PATH" FAKE_DOCKER_LOG="$bad_log" FAKE_DOCKER_BAD_SUBJECT=true \
  KEYCLOAK_ADMIN_USERNAME='admin' KEYCLOAK_ADMIN_PASSWORD="$admin_password" \
  TEST_USER_PASSWORD="$user_password" LOCAL_DB_PASSWORD='test-db-password' \
  AUTH_TRANSACTION_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' \
  KEYCLOAK_READINESS_RETRY_DELAY_SECONDS=0 bash "$script" >"$fake_root/bad-output" 2>"$fake_root/bad-error"; then
  echo "provisioning accepted invalid subject" >&2
  exit 1
fi
grep -Fq 'Invalid Keycloak subject' "$fake_root/bad-error"
for args_file in "$bad_log"/*.args; do
  if grep -Fq 'psql' "$args_file"; then echo "database was seeded for invalid subject" >&2; exit 1; fi
done

cleanup_args=''
for args_file in "$fake_log"/*.args; do
  if grep -Fq 'rm' "$args_file" && grep -Fq -- '-f' "$args_file"; then
    cleanup_args="$args_file"
    break
  fi
done
[[ -n "$cleanup_args" ]]
grep -Fxq "$config_path" "$cleanup_args"

failed_log="$fake_root/failed-log"
mkdir -p "$failed_log"
failed_stdout="$fake_root/failed-stdout"
failed_stderr="$fake_root/failed-stderr"
failed_argv="$fake_root/failed-argv"
if PATH="$fake_root:$PATH" \
  FAKE_DOCKER_LOG="$failed_log" \
  FAKE_DOCKER_FAIL_AFTER_CREDENTIALS=true \
  KEYCLOAK_READINESS_RETRY_DELAY_SECONDS=0 \
  KEYCLOAK_ADMIN_USERNAME='admin' \
  KEYCLOAK_ADMIN_PASSWORD="$admin_password" \
  TEST_USER_PASSWORD="$user_password" \
  LOCAL_DB_PASSWORD='test-db-password' \
  AUTH_TRANSACTION_KEY='AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=' \
  bash "$script" >"$failed_stdout" 2>"$failed_stderr"; then
  echo "provisioning unexpectedly succeeded after fake docker failure" >&2
  exit 1
fi

: >"$failed_argv"
for args_file in "$failed_log"/*.args; do
  cat "$args_file" >>"$failed_argv"
done
for output_file in "$failed_stdout" "$failed_stderr" "$failed_argv"; do
  if grep -Fq "$admin_password" "$output_file"; then
    echo "administrator password leaked to failure stdout, stderr, or arguments" >&2
    exit 1
  fi
  if grep -Fq "$user_password" "$output_file"; then
    echo "user password leaked to failure stdout, stderr, or arguments" >&2
    exit 1
  fi
done

failed_config_path=''
failed_cleanup_args=''
failed_credentials_count=0
for args_file in "$failed_log"/*.args; do
  stdin_file="${args_file%.args}.stdin"
  if grep -Fq -- '--config' "$args_file"; then
    next_is_config=false
    while IFS= read -r argument; do
      if [[ "$next_is_config" == true ]]; then
        if [[ -z "$failed_config_path" ]]; then
          failed_config_path="$argument"
        else
          [[ "$argument" == "$failed_config_path" ]]
        fi
        next_is_config=false
      elif [[ "$argument" == '--config' ]]; then
        next_is_config=true
      fi
    done <"$args_file"
  fi
  if grep -Fq 'config' "$args_file" && grep -Fq 'credentials' "$args_file"; then
    failed_credentials_count=$((failed_credentials_count + 1))
    cmp -s <(printf '%s\n' "$admin_password") "$stdin_file"
  elif grep -Fq "$admin_password" "$stdin_file" || grep -Fq "$user_password" "$stdin_file"; then
    echo "password captured on stdin for an unexpected failure invocation" >&2
    exit 1
  fi
  if grep -Fq 'rm' "$args_file" && grep -Fq -- '-f' "$args_file"; then
    failed_cleanup_args="$args_file"
  fi
done
[[ "$failed_credentials_count" -eq 10 ]]
[[ "$(<"$failed_log/credentials-attempts")" -eq 10 ]]
[[ "$failed_config_path" == /tmp/kcadm-config-* ]]
[[ -n "$failed_cleanup_args" ]]
grep -Fxq "$failed_config_path" "$failed_cleanup_args"

realm="$(cd -- "$(dirname -- "$script")/.." && pwd)/keycloak/realms/approval-flow-dev-realm.json"
jq -e '
  .clients | length == 1 and
  .[0].directAccessGrantsEnabled == false and
  .[0].attributes["pkce.code.challenge.method"] == "S256" and
  .[0].redirectUris == ["http://127.0.0.1:8080/auth/oidc/callback"] and
  .[0].webOrigins == ["http://127.0.0.1:5173"]
' "$realm" >/dev/null
