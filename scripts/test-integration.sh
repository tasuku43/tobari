#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

binary=$PWD/bin/tobari
mock_name=tobari-mock-upstream
custom_image="tobari-integration-custom-$$"
test_root=
work_id=
policy_id=
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

wait_network_connection() {
  local source_container=$1
  local target_host=$2
  local port=$3
  local _
  for _ in $(seq 1 60); do
    if docker exec "$source_container" python3 -c \
      "import socket; socket.create_connection(('$target_host', $port), 1).close()" \
      >/dev/null 2>&1; then
      return 0
    fi
    sleep 0.5
  done
  fail "$source_container could not reach $target_host:$port"
}

run_tobari() {
  env \
    HOME="$test_root/user" \
    DOCKER_CONFIG="$host_docker_config" \
    XDG_CONFIG_HOME="$test_root/config" \
    XDG_STATE_HOME="$test_root/state" \
    "$binary" "$@"
}

id_for_name() {
  local name=$1
  python3 -c \
    'import json,sys; name=sys.argv[1]; print(next(item["id"] for item in json.load(sys.stdin)["tobari"] if item["name"] == name))' \
    "$name"
}

candidate_id_for_effect() {
  local host=$1
  local method=$2
  local path=$3
  python3 -c \
    'import json,sys
host,method,path=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["policy_candidates"]
           if item["host"] == host and item["method"] == method and item["path"] == path))' \
    "$host" "$method" "$path"
}

compaction_id_for_prefix() {
  local host=$1
  local method=$2
  local prefix=$3
  python3 -c \
    'import json,sys
host,method,prefix=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["policy_compactions"]
           if item["host"] == host and item["method"] == method and item["path_prefix"] == prefix))' \
    "$host" "$method" "$prefix"
}

cleanup() {
  docker rm -f "$mock_name" >/dev/null 2>&1 || true
  if [[ -n ${test_root:-} && -x $binary ]]; then
    if [[ -n ${work_id:-} ]]; then
      run_tobari detach --id "$work_id" --purge >/dev/null 2>&1 || true
    fi
    if [[ -n ${policy_id:-} ]]; then
      run_tobari detach --id "$policy_id" --purge >/dev/null 2>&1 || true
    fi
    run_tobari cluster down --purge >/dev/null 2>&1 || true
  fi
  docker image rm -f "$custom_image" >/dev/null 2>&1 || true
}

finish() {
  local status=$?
  trap - EXIT
  if ((status != 0)); then
    for container in tobari-work tobari-policy tobari-gateway tobari-opa "$mock_name"; do
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
command -v python3 >/dev/null || fail "python3 is required"
docker version >/dev/null 2>&1 || fail "Docker Engine is unavailable"
for name in tobari-work tobari-policy tobari-gateway tobari-opa "$mock_name"; do
  if docker inspect "$name" >/dev/null 2>&1; then
    fail "container $name already exists; stop the active Tobari cluster before integration tests"
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
run_tobari cluster up >/dev/null
docker build --tag "$custom_image" \
  --file test/integration/custom-image.Dockerfile . >/dev/null
mkdir -p "$test_root/workspace/.devcontainer"
cat >"$test_root/workspace/.devcontainer/devcontainer.json" <<JSON
{
  // Tobari consumes only this literal compatible image.
  "name": "integration",
  "image": "$custom_image",
  "customizations": {},
}
JSON
printf '{"version":"v1","default_image":"%s"}\n' "$custom_image" \
  >"$config_directory/config.json"
chmod 0600 "$config_directory/config.json"
run_tobari attach --name work --root "$test_root/workspace" \
  --devcontainer .devcontainer/devcontainer.json >/dev/null
run_tobari attach --name policy --root "$config_directory/policy" >/dev/null

list_json=$(run_tobari list --format json)
work_id=$(id_for_name work <<<"$list_json")
policy_id=$(id_for_name policy <<<"$list_json")
[[ $work_id == tbr_* && $policy_id == tbr_* && $work_id != "$policy_id" ]] ||
  fail "list did not return two distinct opaque IDs"

run_tobari cluster up >/dev/null
run_tobari attach --name work --root "$test_root/workspace" \
  --devcontainer .devcontainer/devcontainer.json >/dev/null
owned_containers=$(docker ps -a --filter label=io.tobari.owner=default --format '{{.Names}}' | wc -l | tr -d ' ')
[[ $owned_containers == 4 ]] || fail "idempotent reconciliation left $owned_containers owned containers"

run_tobari exec --id "$policy_id" -- test -f /workspace/tobari.rego
if run_tobari exec --id "$policy_id" -- test -e /workspace/credentials; then
  fail "policy Tobari unexpectedly contains the sibling credential directory"
fi
if run_tobari exec --id "$work_id" -- getent hosts tobari-policy >/dev/null 2>&1; then
  fail "one Tobari can resolve another Tobari across dedicated networks"
fi

tobari_image=$(docker inspect --format '{{.Config.Image}}' tobari-work)
[[ $tobari_image == "$custom_image" ]] ||
  fail "custom Tobari image selector was not preserved"
[[ $(docker inspect --format '{{.Config.Image}}' tobari-policy) == "$custom_image" ]] ||
  fail "XDG default Tobari image selector was not applied"
work_uid=$(docker exec tobari-work sh -c "awk '/^Uid:/{print \$2}' /proc/1/status")
[[ $work_uid == "$(id -u)" ]] ||
  fail "custom-image Tobari runs as uid $work_uid instead of the host uid"
[[ $(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' tobari-work) == true ]] ||
  fail "custom-image Tobari root filesystem is writable"
[[ $(docker inspect --format '{{join .HostConfig.CapDrop ","}}' tobari-work) == ALL ]] ||
  fail "custom-image Tobari did not drop every capability"
docker run -d \
  --name "$mock_name" \
  --network tobari-egress \
  --network-alias mock-upstream \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --entrypoint python3 \
  -v "$PWD/test/integration/mock_upstream.py:/mock_upstream.py:ro" \
  "$tobari_image" -u /mock_upstream.py >/dev/null
wait_listening "$mock_name" 8080
wait_network_connection tobari-gateway mock-upstream 8080

plain_response=$(run_tobari exec --id "$work_id" -- curl -fsS http://mock-upstream:8080/allowed)
assert_contains "$plain_response" '"authorization_present":false' "allowed HTTP response"

gateway_uid=$(docker exec tobari-gateway sh -c "awk '/^Uid:/{print \$2}' /proc/1/status")
gateway_gid=$(docker exec tobari-gateway sh -c "awk '/^Gid:/{print \$2}' /proc/1/status")
[[ $gateway_uid == "$(id -u)" ]] || fail "Gateway runs as uid $gateway_uid instead of the host uid"
[[ $gateway_gid == "$(id -g)" ]] || fail "Gateway runs as gid $gateway_gid instead of the host gid"

policy_mount_rw=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/policy"}}{{.RW}}{{end}}{{end}}' tobari-opa)
[[ $policy_mount_rw == false ]] || fail "OPA policy bind is writable"

expected_digest=$(printf 'Bearer %s' "$secret_value" | shasum -a 256 | awk '{print $1}')
credential_response=$(run_tobari exec --id "$work_id" -- curl -fsS \
  -H 'X-Tobari-Credential-Profile: integration' \
  http://mock-upstream:8080/credential)
assert_contains "$credential_response" '"authorization_present":true' "credential response"
assert_contains "$credential_response" "\"authorization_sha256\":\"$expected_digest\"" "credential digest"

deny_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/denied)
[[ $deny_status == 403 ]] || fail "denied method/path returned $deny_status instead of 403"
if docker logs "$mock_name" 2>&1 | grep -F '"/denied"' >/dev/null; then
  fail "denied request reached mock upstream"
fi

gateway_logs=$(run_tobari cluster logs --component gateway --tail 500)
assert_contains "$gateway_logs" '"decision":"deny"' "Gateway denial audit"
assert_contains "$gateway_logs" '"host":"mock-upstream"' "Gateway denial audit"
assert_contains "$gateway_logs" '"method":"POST"' "Gateway denial audit"
assert_contains "$gateway_logs" '"path":"/denied"' "Gateway denial audit"
if [[ $gateway_logs == *"$secret_value"* || $gateway_logs == *'Bearer '* ]]; then
  fail "Gateway logs contain a credential value"
fi

denials_json=$(run_tobari cluster denials --tail 500 --format json)
assert_contains "$denials_json" '"policy":' "focused denial evidence"
assert_contains "$denials_json" '"host":"mock-upstream"' "focused denial evidence"
assert_contains "$denials_json" '"method":"POST"' "focused denial evidence"
assert_contains "$denials_json" '"path":"/denied"' "focused denial evidence"
assert_contains "$denials_json" '"learnable":true' "focused denial evidence"
assert_contains "$denials_json" '"apply_command":"tobari policy apply"' "focused denial recovery"
if [[ $denials_json == *"$secret_value"* || $denials_json == *'Bearer '* ]]; then
  fail "focused denial evidence contains a credential value"
fi

candidates_json=$(run_tobari policy candidates --tail 500 --format json)
deny_candidate_id=$(candidate_id_for_effect mock-upstream POST /denied <<<"$candidates_json")
[[ $deny_candidate_id == pcy_* ]] || fail "policy candidates did not emit an opaque candidate ID"
assert_contains "$candidates_json" \
  "\"allow_command\":\"tobari policy allow --id $deny_candidate_id\"" \
  "policy candidate exact action"
tail_output=$(run_tobari policy tail --tail 500)
assert_contains "$tail_output" \
  "allow_command=tobari policy allow --id $deny_candidate_id" \
  "human policy tail"

allow_output=$(run_tobari policy allow --id "$deny_candidate_id")
assert_contains "$allow_output" "policy: $config_directory/policy" "exact policy approval"
assert_contains "$allow_output" 'match: exact' "exact policy approval"
assert_contains "$allow_output" 'path: /denied' "exact policy approval"
assert_contains "$allow_output" 'applied: true' "exact policy approval"

applied_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/denied)
[[ $applied_status == 200 ]] || fail "exact learned policy was not active after policy allow"
child_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/denied/child)
[[ $child_status == 403 ]] || fail "exact learned policy broadened to a child path"

for item_path in one two three; do
  item_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "http://mock-upstream:8080/api/v1/items/$item_path")
  [[ $item_status == 403 ]] || fail "compaction source $item_path was not initially denied"
done

for item_path in one two three; do
  candidates_json=$(run_tobari policy candidates --tail 1000 --format json)
  item_candidate_id=$(candidate_id_for_effect \
    mock-upstream POST "/api/v1/items/$item_path" <<<"$candidates_json")
  item_allow_output=$(run_tobari policy allow --id "$item_candidate_id")
  assert_contains "$item_allow_output" "path: /api/v1/items/$item_path" \
    "exact compaction source approval"
done

for item_path in one two three; do
  item_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "http://mock-upstream:8080/api/v1/items/$item_path")
  [[ $item_status == 200 ]] || fail "exact source rule $item_path was not active"
done

compactions_json=$(run_tobari policy compactions --format json)
compaction_id=$(compaction_id_for_prefix \
  mock-upstream POST /api/v1/items/ <<<"$compactions_json")
[[ $compaction_id == pcx_* ]] || fail "policy compactions did not emit an opaque compaction ID"
assert_contains "$compactions_json" '"source_rule_count":3' "compaction evidence"
assert_contains "$compactions_json" \
  '"outside_canary":"/api/v1/items-outside-tobari-canary"' \
  "compaction boundary"

compact_output=$(run_tobari policy compact --id "$compaction_id")
assert_contains "$compact_output" 'match: prefix' "policy compaction"
assert_contains "$compact_output" 'path: /api/v1/items/' "policy compaction"
assert_contains "$compact_output" 'source_rule_count: 3' "policy compaction"
assert_contains "$compact_output" 'applied: true' "policy compaction"

compacted_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/api/v1/items/four)
[[ $compacted_status == 200 ]] || fail "compacted prefix did not allow a sibling path"
outside_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/api/v1/items-outside-tobari-canary)
[[ $outside_status == 403 ]] || fail "compacted prefix crossed its tested directory boundary"

apply_output=$(run_tobari policy apply)
assert_contains "$apply_output" "policy: $config_directory/policy" "policy activation"
assert_contains "$apply_output" 'applied: true' "policy activation"

other_host_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
  -H 'X-Tobari-Credential-Profile: integration' https://example.com/)
[[ $other_host_status == 403 ]] || fail "cross-host credential request returned $other_host_status instead of 403"
unlearnable_denials=$(run_tobari cluster denials --tail 1000 --format json)
assert_contains "$unlearnable_denials" '"learnable":false' "orthogonal denial evidence"
post_credential_candidates=$(run_tobari policy candidates --tail 1000 --format json)
python3 -c \
  'import json,sys
items=json.load(sys.stdin)["policy_candidates"]
if any(item["host"] == "example.com" and item["method"] == "GET" and item["path"] == "/" for item in items):
    raise SystemExit("credential-binding denial became an ineffective policy candidate")' \
  <<<"$post_credential_candidates"

https_status=$(run_tobari exec --id "$work_id" -- curl -fsS -o /dev/null -w '%{http_code}' https://example.com/)
[[ $https_status == 200 ]] || fail "intercepted HTTPS returned $https_status instead of 200"

shell_output=$(printf 'printf shell-ok\\nexit\\n' | run_tobari shell --id "$work_id")
assert_contains "$shell_output" "shell-ok" "interactive shell"

if run_tobari exec --id "$work_id" -- env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy \
  curl --noproxy '*' --max-time 3 -fsS https://example.com/ >/dev/null 2>&1; then
  fail "Tobari reached the Internet without Gateway"
fi
if run_tobari exec --id "$work_id" -- curl --noproxy '*' --max-time 3 -fsS \
  http://opa:8181/health >/dev/null 2>&1; then
  fail "Tobari reached the OPA control API"
fi

docker stop tobari-opa >/dev/null
opa_down_status=$(run_tobari exec --id "$work_id" -- curl -sS -o /dev/null -w '%{http_code}' \
  http://mock-upstream:8080/opa-down)
[[ $opa_down_status == 503 ]] || fail "OPA outage returned $opa_down_status instead of 503"
docker start tobari-opa >/dev/null
wait_healthy tobari-opa

docker stop tobari-gateway >/dev/null
if run_tobari exec --id "$work_id" -- curl --max-time 3 -fsS \
  http://mock-upstream:8080/gateway-down >/dev/null 2>&1; then
  fail "request succeeded while Gateway was stopped"
fi
docker start tobari-gateway >/dev/null
wait_healthy tobari-gateway

if run_tobari exec --id "$work_id" -- test -e /var/run/docker.sock; then
  fail "Tobari contains the Docker socket"
fi
if run_tobari exec --id "$work_id" -- test -e /run/tobari/credentials/integration; then
  fail "Tobari contains the Gateway credential file"
fi
if run_tobari exec --id "$work_id" -- env | grep -E 'TOBARI_CREDENTIAL|AUTHORIZATION|API_KEY' >/dev/null; then
  fail "Tobari environment exposes credential metadata"
fi
mounts=$(docker inspect --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' tobari-work)
if grep -E '^/(run/tobari/credentials|var/run/docker.sock)$' <<<"$mounts" >/dev/null; then
  fail "Tobari has a forbidden mount"
fi

set +e
run_tobari exec --id "$work_id" -- sh -c 'exit 37'
exec_status=$?
set -e
[[ $exec_status == 37 ]] || fail "exec returned $exec_status instead of child status 37"

run_tobari exec --id "$work_id" -- sh -c 'sleep 1' &
first_pid=$!
run_tobari exec --id "$work_id" -- sh -c 'sleep 1' &
second_pid=$!
wait "$first_pid"
wait "$second_pid"

if run_tobari cluster down >/dev/null 2>&1; then
  fail "cluster down succeeded while Tobari remained attached"
fi

docker rm -f "$mock_name" >/dev/null
run_tobari detach --id "$work_id" --purge >/dev/null
work_id=
run_tobari detach --id "$policy_id" --purge >/dev/null
policy_id=
run_tobari cluster down --purge >/dev/null
run_tobari cluster down >/dev/null

if docker ps -a --filter label=io.tobari.owner=default --format '{{.Names}}' | grep . >/dev/null; then
  fail "owned containers remain after cleanup"
fi
if docker network ls --filter label=io.tobari.owner=default --format '{{.Name}}' | grep . >/dev/null; then
  fail "owned networks remain after cleanup"
fi
if docker volume ls --filter label=io.tobari.owner=default --format '{{.Name}}' | grep . >/dev/null; then
  fail "owned volumes remain after purge"
fi

echo "integration: OK"
