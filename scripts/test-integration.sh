#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

binary=$PWD/bin/tobari
mock_name=tobari-mock-upstream
test_root=
host_docker_config=${DOCKER_CONFIG:-$HOME/.docker}

fail() {
  echo "integration: $*" >&2
  exit 1
}

assert_contains() {
  local value=$1
  local expected=$2
  local context=$3
  [[ $value == *"$expected"* ]] || fail "$context did not contain the expected value"
}

wait_healthy() {
  local container=$1
  local _
  for _ in $(seq 1 60); do
    if [[ $(docker inspect --format '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' "$container" 2>/dev/null || true) == healthy ]]; then
      return 0
    fi
    sleep 0.5
  done
  fail "$container did not become healthy"
}

wait_listening() {
  local container=$1
  local port=$2
  local _
  for _ in $(seq 1 60); do
    if docker exec "$container" python3 -c \
      "import socket; socket.create_connection(('127.0.0.1', $port), 1).close()" \
      >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  fail "$container did not listen on port $port"
}

run_tobari() {
  env \
    HOME="$test_root/user" \
    DOCKER_CONFIG="$host_docker_config" \
    XDG_CONFIG_HOME="$test_root/config" \
    XDG_STATE_HOME="$test_root/state" \
    "$binary" "$@"
}

cleanup() {
  docker rm -f "$mock_name" >/dev/null 2>&1 || true
  if [[ -n ${test_root:-} && -x $binary ]]; then
    run_tobari down --purge >/dev/null 2>&1 || true
  fi
}

finish() {
  local status=$?
  trap - EXIT
  if ((status != 0)); then
    for container in tobari-gateway tobari-opa "$mock_name"; do
      if docker inspect "$container" >/dev/null 2>&1; then
        echo "integration diagnostics: $container" >&2
        docker inspect --format '{{json .State}}' "$container" >&2 || true
        docker logs --tail 200 "$container" >&2 || true
      fi
    done
  fi
  cleanup
  if [[ -n ${test_root:-} ]]; then
    rm -rf "$test_root"
  fi
  exit "$status"
}
trap finish EXIT

command -v docker >/dev/null || fail "docker is required"
docker version >/dev/null 2>&1 || fail "Docker Engine is unavailable"
for name in tobari-realm tobari-gateway tobari-opa "$mock_name"; do
  if docker inspect "$name" >/dev/null 2>&1; then
    fail "container $name already exists; stop the active Tobari Realm before integration tests"
  fi
done

test_root=$(mktemp -d "$PWD/.tobari-integration.XXXXXX")
mkdir -p "$test_root/user" "$test_root/config/tobari/credentials" "$test_root/state" "$test_root/workspace"

config_directory=$test_root/config/tobari
secret_value=tobari-integration-secret-canary
secret_file=$config_directory/credentials/integration
printf '%s\n' "$secret_value" >"$secret_file"
chmod 0600 "$secret_file"
credential_config=$config_directory/credentials.json
cat >"$credential_config" <<'JSON'
{
  "version": "v1",
  "profiles": {
    "integration": {
      "type": "bearer",
      "hosts": ["mock-upstream"],
      "secret_file": "/run/tobari/credentials/integration"
    }
  }
}
JSON
chmod 0600 "$credential_config"

go build -buildvcs=false -trimpath -o "$binary" ./cmd/tobari
run_tobari up --root "$test_root/workspace" >/dev/null

realm_image=$(docker inspect --format '{{.Config.Image}}' tobari-realm)
docker run -d \
  --name "$mock_name" \
  --network tobari-egress \
  --network-alias mock-upstream \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --entrypoint python3 \
  -v "$PWD/test/integration/mock_upstream.py:/mock_upstream.py:ro" \
  "$realm_image" -u /mock_upstream.py >/dev/null
wait_listening "$mock_name" 8080

plain_response=$(run_tobari exec -- curl -fsS http://mock-upstream:8080/allowed)
assert_contains "$plain_response" '"authorization_present":false' "allowed HTTP response"

gateway_uid=$(docker exec tobari-gateway sh -c "awk '/^Uid:/{print \$2}' /proc/1/status")
gateway_gid=$(docker exec tobari-gateway sh -c "awk '/^Gid:/{print \$2}' /proc/1/status")
[[ $gateway_uid == "$(id -u)" ]] || fail "Gateway runs as uid $gateway_uid instead of the host uid"
[[ $gateway_gid == "$(id -g)" ]] || fail "Gateway runs as gid $gateway_gid instead of the host gid"

expected_digest=$(printf 'Bearer %s' "$secret_value" | shasum -a 256 | awk '{print $1}')
credential_response=$(run_tobari exec -- curl -fsS \
  -H 'X-Tobari-Credential-Profile: integration' \
  http://mock-upstream:8080/credential)
assert_contains "$credential_response" '"authorization_present":true' "credential response"
assert_contains "$credential_response" "\"authorization_sha256\":\"$expected_digest\"" "credential digest"

deny_status=$(run_tobari exec -- curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/denied)
[[ $deny_status == 403 ]] || fail "denied method/path returned $deny_status instead of 403"
if docker logs "$mock_name" 2>&1 | grep -F '"/denied"' >/dev/null; then
  fail "denied request reached mock upstream"
fi

gateway_logs=$(run_tobari logs --component gateway --tail 500)
assert_contains "$gateway_logs" '"decision":"deny"' "Gateway denial audit"
assert_contains "$gateway_logs" '"host":"mock-upstream"' "Gateway denial audit"
assert_contains "$gateway_logs" '"method":"POST"' "Gateway denial audit"
assert_contains "$gateway_logs" '"path":"/denied"' "Gateway denial audit"
if [[ $gateway_logs == *"$secret_value"* ]]; then
  fail "Gateway logs contain the credential secret"
fi
if [[ $gateway_logs == *'Bearer '* ]]; then
  fail "Gateway logs contain a bearer value"
fi

other_host_status=$(run_tobari exec -- curl -sS -o /dev/null -w '%{http_code}' \
  -H 'X-Tobari-Credential-Profile: integration' https://example.com/)
[[ $other_host_status == 403 ]] || fail "cross-host credential request returned $other_host_status instead of 403"

https_status=$(run_tobari exec -- curl -fsS -o /dev/null -w '%{http_code}' https://example.com/)
[[ $https_status == 200 ]] || fail "intercepted HTTPS returned $https_status instead of 200"

shell_output=$(printf 'printf shell-ok\\nexit\\n' | run_tobari shell)
assert_contains "$shell_output" "shell-ok" "interactive shell"

if run_tobari exec -- env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy \
  curl --noproxy '*' --max-time 3 -fsS https://example.com/ >/dev/null 2>&1; then
  fail "Realm reached the Internet without Gateway"
fi
if run_tobari exec -- curl --noproxy '*' --max-time 3 -fsS \
  http://opa:8181/health >/dev/null 2>&1; then
  fail "Realm reached the OPA control API"
fi

docker stop tobari-opa >/dev/null
opa_down_status=$(run_tobari exec -- curl -sS -o /dev/null -w '%{http_code}' \
  http://mock-upstream:8080/opa-down)
[[ $opa_down_status == 503 ]] || fail "OPA outage returned $opa_down_status instead of 503"
docker start tobari-opa >/dev/null
wait_healthy tobari-opa

docker stop tobari-gateway >/dev/null
if run_tobari exec -- curl --max-time 3 -fsS http://mock-upstream:8080/gateway-down >/dev/null 2>&1; then
  fail "request succeeded while Gateway was stopped"
fi
docker start tobari-gateway >/dev/null
wait_healthy tobari-gateway

if run_tobari exec -- test -e /var/run/docker.sock; then
  fail "Realm contains the Docker socket"
fi
if run_tobari exec -- test -e /run/tobari/credentials/integration; then
  fail "Realm contains the Gateway credential file"
fi
if run_tobari exec -- env | grep -E 'TOBARI_CREDENTIAL|AUTHORIZATION|API_KEY' >/dev/null; then
  fail "Realm environment exposes credential metadata"
fi
mounts=$(docker inspect --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' tobari-realm)
if grep -E '^/(run/tobari/credentials|var/run/docker.sock)$' <<<"$mounts" >/dev/null; then
  fail "Realm has a forbidden mount"
fi

set +e
run_tobari exec -- sh -c 'exit 37'
exec_status=$?
set -e
[[ $exec_status == 37 ]] || fail "exec returned $exec_status instead of child status 37"

run_tobari exec -- sh -c 'sleep 1' &
first_pid=$!
run_tobari exec -- sh -c 'sleep 1' &
second_pid=$!
wait "$first_pid"
wait "$second_pid"

run_tobari up --root "$test_root/workspace" >/dev/null
owned_containers=$(docker ps -a --filter label=io.tobari.owner=default --format '{{.Names}}' | wc -l | tr -d ' ')
[[ $owned_containers == 3 ]] || fail "idempotent up left $owned_containers owned containers"

docker rm -f "$mock_name" >/dev/null
run_tobari down --purge >/dev/null
run_tobari down >/dev/null

if docker ps -a --filter label=io.tobari.owner=default --format '{{.Names}}' | grep . >/dev/null; then
  fail "owned containers remain after down"
fi
if docker network ls --filter label=io.tobari.owner=default --format '{{.Name}}' | grep . >/dev/null; then
  fail "owned networks remain after down"
fi
if docker volume ls --filter label=io.tobari.owner=default --format '{{.Name}}' | grep . >/dev/null; then
  fail "owned volumes remain after purge"
fi

echo "integration: OK"
