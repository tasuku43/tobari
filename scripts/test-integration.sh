#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

binary=$PWD/bin/tobari
mock_name=tobari-mock-upstream
custom_image="tobari-integration-custom-$$"
test_root=
work_root=
work_nested_root=
other_root=
work_id=
other_id=
work_container=
other_container=
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
    TOBARI_CREDENTIAL_ADAPTER=passthrough \
    XDG_CONFIG_HOME="$test_root/config" \
    XDG_STATE_HOME="$test_root/state" \
    XDG_DATA_HOME="$test_root/data" \
    "$binary" "$@"
}

run_tobari_at() {
  local root=$1
  shift
  (cd "$root" && run_tobari "$@")
}

run_tobari_pty_at() {
  local root=$1
  shift
  (
    cd "$root"
    if [[ $(uname -s) == Darwin ]]; then
      env \
        HOME="$test_root/user" \
        DOCKER_CONFIG="$host_docker_config" \
        TOBARI_CREDENTIAL_ADAPTER=passthrough \
        XDG_CONFIG_HOME="$test_root/config" \
        XDG_STATE_HOME="$test_root/state" \
        XDG_DATA_HOME="$test_root/data" \
        script -q /dev/null "$binary" "$@"
    else
      local command
      printf -v command '%q ' env HOME="$test_root/user" DOCKER_CONFIG="$host_docker_config" \
        TOBARI_CREDENTIAL_ADAPTER=passthrough \
        XDG_CONFIG_HOME="$test_root/config" XDG_STATE_HOME="$test_root/state" \
        XDG_DATA_HOME="$test_root/data" "$binary" "$@"
      script -q -c "$command" /dev/null
    fi
  )
}

enter_tobari_at() {
  local root=$1
  shift
  local output
  if output=$(printf 'exit\n' | run_tobari_pty_at "$root" "$@" 2>&1); then
    return 0
  fi
  printf '%s\n' "$output" >&2
  return 1
}

enter_ancestor_tobari_at() {
  local root=$1
  shift
  local output
  if output=$({ printf '\r'; sleep 1; printf 'exit\n'; } | run_tobari_pty_at "$root" "$@" 2>&1); then
    return 0
  fi
  printf '%s\n' "$output" >&2
  return 1
}

container_for_id() {
  local id=$1
  python3 -c 'import sys; print("tobari-" + sys.argv[1][:13].replace("-", "") + "-work")' "$id"
}

network_for_id() {
  local id=$1
  python3 -c 'import sys; print("tobari-" + sys.argv[1][:13].replace("-", "") + "-net")' "$id"
}

id_for_root() {
  local root=$1
  python3 -c \
    'import json,sys; root=sys.argv[1]; print(next(item["id"] for item in json.load(sys.stdin)["tobari"] if item["root"] == root))' \
    "$root"
}

run_project() {
  docker exec "$work_container" "$@"
}

run_project_shell() {
  docker exec -i "$work_container" /bin/bash
}

run_other_project() {
  docker exec "$other_container" "$@"
}

assert_resource_bounds() {
  local container=$1
  [[ $(docker inspect --format '{{.HostConfig.NanoCpus}}' "$container") == 2000000000 ]] ||
    fail "$container does not have the fixed CPU limit"
  [[ $(docker inspect --format '{{.HostConfig.Memory}}' "$container") == 4294967296 ]] ||
    fail "$container does not have the fixed memory limit"
  [[ $(docker inspect --format '{{.HostConfig.MemorySwap}}' "$container") == 4294967296 ]] ||
    fail "$container does not have the fixed total memory limit"
  [[ $(docker inspect --format '{{.HostConfig.PidsLimit}}' "$container") == 512 ]] ||
    fail "$container does not have the fixed PID limit"
  [[ $(docker inspect --format '{{.HostConfig.LogConfig.Type}}' "$container") == json-file ]] ||
    fail "$container does not use the bounded JSON log driver"
  [[ $(docker inspect --format '{{index .HostConfig.LogConfig.Config "max-size"}}' "$container") == 10m ]] ||
    fail "$container does not have the fixed log-size limit"
  [[ $(docker inspect --format '{{index .HostConfig.LogConfig.Config "max-file"}}' "$container") == 3 ]] ||
    fail "$container does not have the fixed log-file count"
}

assert_component_log_bounds() {
  local container=$1
  [[ $(docker inspect --format '{{.HostConfig.LogConfig.Type}}' "$container") == json-file ]] ||
    fail "$container does not use the bounded JSON log driver"
  [[ $(docker inspect --format '{{index .HostConfig.LogConfig.Config "max-size"}}' "$container") == 10m ]] ||
    fail "$container does not have the fixed log-size limit"
  [[ $(docker inspect --format '{{index .HostConfig.LogConfig.Config "max-file"}}' "$container") == 3 ]] ||
    fail "$container does not have the fixed log-file count"
}

assert_component_resource_bounds() {
  local container=$1
  local cpus=$2
  local memory=$3
  local pids=$4
  [[ $(docker inspect --format '{{.HostConfig.NanoCpus}}' "$container") == "$cpus" ]] ||
    fail "$container does not have the fixed CPU limit"
  [[ $(docker inspect --format '{{.HostConfig.Memory}}' "$container") == "$memory" ]] ||
    fail "$container does not have the fixed memory limit"
  [[ $(docker inspect --format '{{.HostConfig.MemorySwap}}' "$container") == "$memory" ]] ||
    fail "$container does not have the fixed total memory limit"
  [[ $(docker inspect --format '{{.HostConfig.PidsLimit}}' "$container") == "$pids" ]] ||
    fail "$container does not have the fixed PID limit"
}

candidate_id_for_effect() {
	local project_id=$1
	local host=$2
	local method=$3
	local path=$4
  python3 -c \
    'import json,sys
project_id,host,method,path=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["policy_candidates"]
           if item["project_id"] == project_id and item["host"] == host and item["method"] == method and item["path"] == path))' \
    "$project_id" "$host" "$method" "$path"
}

compaction_id_for_prefix() {
	local project_id=$1
	local host=$2
	local method=$3
	local prefix=$4
  python3 -c \
    'import json,sys
project_id,host,method,prefix=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["policy_compactions"]
           if item["project_id"] == project_id and item["host"] == host and item["method"] == method and item["path_prefix"] == prefix))' \
    "$project_id" "$host" "$method" "$prefix"
}

cleanup() {
  docker rm -f "$mock_name" >/dev/null 2>&1 || true
  if [[ -n ${test_root:-} && -x $binary && -n ${work_root:-} ]]; then
    run_tobari_at "$work_root" delete --force >/dev/null 2>&1 || true
    if [[ -n $other_root ]]; then
      run_tobari_at "$other_root" delete --force >/dev/null 2>&1 || true
    fi
    run_tobari cluster down --purge >/dev/null 2>&1 || true
  fi
  docker image rm -f "$custom_image" >/dev/null 2>&1 || true
}

finish() {
  local status=$?
  trap - EXIT
  if ((status != 0)); then
    for container in tobari-gateway tobari-opa "$mock_name" "$work_container" "$other_container"; do
      [[ -n $container ]] || continue
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
for name in tobari-gateway tobari-opa "$mock_name"; do
  if docker inspect "$name" >/dev/null 2>&1; then
    fail "container $name already exists; stop the active Tobari cluster before integration tests"
  fi
done

test_root=$(mktemp -d "$PWD/.tobari-integration.XXXXXX")
mkdir -p "$test_root/user" "$test_root/config/tobari" "$test_root/state" "$test_root/workspace"

config_directory=$test_root/config/tobari
tool_auth_value=tobari-tool-auth-canary

go build -buildvcs=false -trimpath -o "$binary" ./cmd/tobari
run_tobari cluster up >/dev/null
assert_component_log_bounds tobari-opa
assert_component_log_bounds tobari-gateway
assert_component_resource_bounds tobari-opa 1000000000 536870912 128
assert_component_resource_bounds tobari-gateway 2000000000 1073741824 256
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
work_root=$test_root/workspace
work_nested_root=$work_root/root
other_root=$test_root/other-workspace
mkdir -p "$work_nested_root"
mkdir -p "$other_root"
enter_tobari_at "$work_root"
enter_ancestor_tobari_at "$work_nested_root"
enter_tobari_at "$other_root"

status_from_nested=$(run_tobari_at "$work_nested_root" status --format json)
assert_contains "$status_from_nested" '"exists":true' "nested status"
assert_contains "$status_from_nested" "\"root\":\"$work_root\"" "nested status root"
nested_pwd=$({ printf '\r'; sleep 1; printf 'pwd\nexit\n'; } | run_tobari_pty_at "$work_nested_root" 2>&1)
assert_contains "$nested_pwd" "/workspace${work_root}/root" "nested host CWD mapping"
list_json=$(run_tobari_at "$work_root" list --format json)
work_id=$(id_for_root "$work_root" <<<"$list_json")
other_id=$(id_for_root "$other_root" <<<"$list_json")
work_container=$(container_for_id "$work_id")
work_network=$(network_for_id "$work_id")
other_container=$(container_for_id "$other_id")
[[ $work_id =~ ^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]] ||
  fail "list did not return the project's stable ID"
[[ $work_id != "$other_id" ]] || fail "CWD projects received the same stable ID"

python3 - "$config_directory/principals.json" "$work_id" "$other_id" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    document = json.load(handle)
bindings = document.get("bindings", [])
if document.get("schema_version") != 1 or len(bindings) != 2:
    raise SystemExit(f"unexpected project principal registry: {document!r}")
ids = {item["project_id"] for item in bindings}
if ids != set(sys.argv[2:]):
    raise SystemExit(f"registry project IDs {ids!r} do not match CWD projects")
addresses = [item["gateway_ip"] for item in bindings]
if len(addresses) != len(set(addresses)):
    raise SystemExit("project principal registry reused one Gateway address")
PY

work_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["status"]["home"])' <<<"$status_from_nested")
other_status=$(run_tobari_at "$other_root" status --format json)
other_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["status"]["home"])' <<<"$other_status")
[[ $work_home != "$other_home" ]] || fail "CWD projects share a home directory"
assert_resource_bounds "$work_container"
assert_resource_bounds "$other_container"

profile_directory=$test_root/data/tobari/profiles/default
profile_skill=$profile_directory/claude/skills/shared.md
profile_settings=$profile_directory/common/settings.json
container_before_profile_change=$(docker inspect --format '{{.Id}}' "$work_container")
printf 'shared skill\n' >"$profile_skill"
printf '{"shared":true,"theme":"dark"}\n' >"$profile_settings"
mkdir -p "$work_root/.claude"
printf '{"theme":"light","local":true}\n' >"$work_home/.claude/settings.json"
printf 'project-local\n' >"$work_root/.claude/project.md"
enter_tobari_at "$work_root"
container_after_profile_change=$(docker inspect --format '{{.Id}}' "$work_container")
[[ $container_before_profile_change != "$container_after_profile_change" ]] ||
  fail "profile revision drift did not recreate only the project container"
merged_settings=$(python3 -c 'import json,sys; print(json.dumps(json.load(open(sys.argv[1]))))' "$work_home/.claude/settings.json")
assert_contains "$merged_settings" '"shared": true' "merged shared settings"
assert_contains "$merged_settings" '"theme": "light"' "local settings override"
assert_contains "$merged_settings" '"local": true' "per-Tobari settings"
assert_contains "$(run_project cat /var/lib/tobari/.claude/skills/shared.md)" "shared skill" "shared profile skill"
assert_contains "$(docker exec "$other_container" cat /var/lib/tobari/.claude/skills/shared.md)" "shared skill" "shared profile reuse"
if docker exec "$work_container" sh -c 'printf forbidden > /var/lib/tobari/.claude/skills/shared.md' >/dev/null 2>&1; then
  fail "Tobari modified the read-only shared profile"
fi
if ! docker exec "$work_container" test -e "/workspace${work_root}/.claude/project.md"; then
  fail "Tobari did not expose project-local .claude content"
fi
run_project sh -c 'printf private > /var/lib/tobari/.claude/memory.txt'
if docker exec "$other_container" test -e /var/lib/tobari/.claude/memory.txt; then
  fail "Tobari home state leaked to another project"
fi
run_project sh -c "printf '%s\\n' '$tool_auth_value' > /var/lib/tobari/tool-auth-state"
if docker exec "$other_container" test -e /var/lib/tobari/tool-auth-state; then
  fail "tool authentication state leaked to another project"
fi

run_tobari cluster up >/dev/null
enter_tobari_at "$work_root" &
first_enter_pid=$!
enter_tobari_at "$work_root" &
second_enter_pid=$!
wait "$first_enter_pid"
wait "$second_enter_pid"
owned_containers=$(docker ps -a --filter label=io.tobari.owner=default --format '{{.Names}}' | wc -l | tr -d ' ')
[[ $owned_containers == 4 ]] || fail "idempotent reconciliation left $owned_containers owned containers"

docker rm -f "$work_container" >/dev/null
enter_tobari_at "$work_root"
docker network disconnect -f "$work_network" "$work_container" >/dev/null
docker network disconnect -f "$work_network" tobari-gateway >/dev/null
docker network rm "$work_network" >/dev/null
enter_tobari_at "$work_root"
status_after_reconcile=$(run_tobari_at "$work_root" status --format json)
assert_contains "$status_after_reconcile" '"runtime":"ready"' "runtime recovery"
assert_resource_bounds "$work_container"
assert_contains "$(run_project cat /var/lib/tobari/tool-auth-state)" "$tool_auth_value" \
  "tool authentication persistence"

if run_project test -e "/workspace${work_root}/credentials"; then
  fail "Tobari unexpectedly contains the host credential directory"
fi
if run_project getent hosts "$other_container" >/dev/null 2>&1; then
  fail "one CWD-owned Tobari can resolve another Tobari across dedicated networks"
fi

tobari_image=$(docker inspect --format '{{.Config.Image}}' "$work_container")
[[ $tobari_image == "$custom_image" ]] ||
  fail "custom Tobari image selector was not preserved"
custom_image_cmd=$(docker image inspect --format '{{json .Config.Cmd}}' "$custom_image")
[[ $custom_image_cmd == '["sh","-c","exit 23"]' ]] ||
  fail "custom image fixture does not have a terminating default command: $custom_image_cmd"
work_image_cmd=$(docker inspect --format '{{json .Config.Cmd}}' "$work_container")
[[ $work_image_cmd == '["sleep","infinity"]' ]] ||
  fail "Tobari did not override the custom image command: $work_image_cmd"
work_uid=$(docker exec "$work_container" sh -c "awk '/^Uid:/{print \$2}' /proc/1/status")
[[ $work_uid == "$(id -u)" ]] ||
  fail "custom-image Tobari runs as uid $work_uid instead of the host uid"
[[ $(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' "$work_container") == true ]] ||
  fail "custom-image Tobari root filesystem is writable"
[[ $(docker inspect --format '{{join .HostConfig.CapDrop ","}}' "$work_container") == ALL ]] ||
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
# Docker Desktop file binds can retain the pre-reconcile inode after the
# host-owned principal registry is atomically replaced. Recreate only the
# trusted Gateway so it observes the complete current registry before traffic.
docker rm -f tobari-gateway >/dev/null
run_tobari cluster up >/dev/null
wait_healthy tobari-gateway

plain_response=$(run_project curl -fsS http://mock-upstream:8080/allowed)
assert_contains "$plain_response" '"authorization_present":false' "allowed HTTP response"

wrong_port_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  http://mock-upstream:8081/wrong-port)
[[ $wrong_port_status == 403 ]] || fail "non-policy HTTP port returned $wrong_port_status instead of 403"
if docker logs "$mock_name" 2>&1 | grep -F '"/wrong-port"' >/dev/null; then
  fail "wrong-port request reached mock upstream"
fi

body_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST -H 'content-type: application/json' --data '{"value":true}' \
  http://mock-upstream:8080/body)
[[ $body_status == 403 ]] || fail "nonempty-body request returned $body_status instead of 403"
if docker logs "$mock_name" 2>&1 | grep -F '"/body"' >/dev/null; then
  fail "nonempty-body request reached mock upstream"
fi

oversized_body_status=$(dd if=/dev/zero bs=1048576 count=9 2>/dev/null | \
  docker exec -i "$work_container" curl -sS -o /dev/null -w '%{http_code}' \
    --max-time 15 -X POST -H 'content-type: application/octet-stream' \
    --data-binary @- http://mock-upstream:8080/oversized-body || true)
[[ $oversized_body_status == 413 ]] ||
  fail "oversized request returned $oversized_body_status instead of 413"
if docker logs "$mock_name" 2>&1 | grep -F '"/oversized-body"' >/dev/null; then
  fail "oversized request reached mock upstream"
fi

gateway_uid=$(docker exec tobari-gateway sh -c "awk '/^Uid:/{print \$2}' /proc/1/status")
gateway_gid=$(docker exec tobari-gateway sh -c "awk '/^Gid:/{print \$2}' /proc/1/status")
[[ $gateway_uid == "$(id -u)" ]] || fail "Gateway runs as uid $gateway_uid instead of the host uid"
[[ $gateway_gid == "$(id -g)" ]] || fail "Gateway runs as gid $gateway_gid instead of the host gid"

policy_mount_rw=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/policy"}}{{.RW}}{{end}}{{end}}' tobari-opa)
[[ $policy_mount_rw == false ]] || fail "OPA policy bind is writable"

expected_digest=$(printf 'Bearer %s' "$tool_auth_value" | shasum -a 256 | awk '{print $1}')
auth_response=$(run_project curl -fsS \
  -H "Authorization: Bearer $tool_auth_value" \
  http://mock-upstream:8080/credential)
assert_contains "$auth_response" '"authorization_present":true' "tool auth response"
assert_contains "$auth_response" "\"authorization_sha256\":\"$expected_digest\"" "tool auth digest"

deny_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
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
if [[ $gateway_logs == *"$tool_auth_value"* || $gateway_logs == *'Bearer '* ]]; then
  fail "Gateway logs contain a credential value"
fi

denials_json=$(run_tobari cluster denials --tail 500 --format json)
assert_contains "$denials_json" '"policy":' "focused denial evidence"
assert_contains "$denials_json" '"host":"mock-upstream"' "focused denial evidence"
assert_contains "$denials_json" "\"project_id\":\"$work_id\"" "focused denial evidence"
assert_contains "$denials_json" '"method":"POST"' "focused denial evidence"
assert_contains "$denials_json" '"path":"/denied"' "focused denial evidence"
assert_contains "$denials_json" '"learnable":true' "focused denial evidence"
assert_contains "$denials_json" '"apply_command":"tobari policy apply"' "focused denial recovery"
if [[ $denials_json == *"$tool_auth_value"* || $denials_json == *'Bearer '* ]]; then
  fail "focused denial evidence contains a credential value"
fi

candidates_json=$(run_tobari policy candidates --tail 500 --format json)
deny_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream POST /denied <<<"$candidates_json")
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

applied_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/denied)
[[ $applied_status == 200 ]] || fail "exact learned policy was not active after policy allow"
other_learned_status=$(run_other_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/denied)
[[ $other_learned_status == 403 ]] ||
  fail "learned policy crossed the project boundary with status $other_learned_status"
child_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/denied/child)
[[ $child_status == 403 ]] || fail "exact learned policy broadened to a child path"

for item_path in one two three; do
  item_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "http://mock-upstream:8080/api/v1/items/$item_path")
  [[ $item_status == 403 ]] || fail "compaction source $item_path was not initially denied"
done

for item_path in one two three; do
  candidates_json=$(run_tobari policy candidates --tail 1000 --format json)
  item_candidate_id=$(candidate_id_for_effect \
    "$work_id" mock-upstream POST "/api/v1/items/$item_path" <<<"$candidates_json")
  item_allow_output=$(run_tobari policy allow --id "$item_candidate_id")
  assert_contains "$item_allow_output" "path: /api/v1/items/$item_path" \
    "exact compaction source approval"
done

for item_path in one two three; do
  item_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
    -X POST "http://mock-upstream:8080/api/v1/items/$item_path")
  [[ $item_status == 200 ]] || fail "exact source rule $item_path was not active"
done

compactions_json=$(run_tobari policy compactions --format json)
compaction_id=$(compaction_id_for_prefix \
  "$work_id" mock-upstream POST /api/v1/items/ <<<"$compactions_json")
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

compacted_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/api/v1/items/four)
[[ $compacted_status == 200 ]] || fail "compacted prefix did not allow a sibling path"
outside_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST http://mock-upstream:8080/api/v1/items-outside-tobari-canary)
[[ $outside_status == 403 ]] || fail "compacted prefix crossed its tested directory boundary"

apply_output=$(run_tobari policy apply)
assert_contains "$apply_output" "policy: $config_directory/policy" "policy activation"
assert_contains "$apply_output" 'applied: true' "policy activation"

https_status=$(run_project curl -fsS -o /dev/null -w '%{http_code}' https://example.com/)
[[ $https_status == 200 ]] || fail "intercepted HTTPS returned $https_status instead of 200"

shell_output=$(printf 'printf shell-ok\\nexit\\n' | run_project_shell)
assert_contains "$shell_output" "shell-ok" "interactive shell"

if run_project env -u HTTP_PROXY -u HTTPS_PROXY -u http_proxy -u https_proxy \
  curl --noproxy '*' --max-time 3 -fsS https://example.com/ >/dev/null 2>&1; then
  fail "Tobari reached the Internet without Gateway"
fi
if run_project curl --noproxy '*' --max-time 3 -fsS \
  http://opa:8181/health >/dev/null 2>&1; then
  fail "Tobari reached the OPA control API"
fi

docker stop tobari-opa >/dev/null
opa_down_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  http://mock-upstream:8080/opa-down)
[[ $opa_down_status == 503 ]] || fail "OPA outage returned $opa_down_status instead of 503"
docker start tobari-opa >/dev/null
wait_healthy tobari-opa

docker stop tobari-gateway >/dev/null
if run_project curl --max-time 3 -fsS \
  http://mock-upstream:8080/gateway-down >/dev/null 2>&1; then
  fail "request succeeded while Gateway was stopped"
fi
docker start tobari-gateway >/dev/null
wait_healthy tobari-gateway

if run_project test -e /var/run/docker.sock; then
  fail "Tobari contains the Docker socket"
fi
if run_project test -e /run/tobari/credentials/integration; then
  fail "Tobari contains the Gateway credential file"
fi
if run_project env | grep -E 'TOBARI_CREDENTIAL|AUTHORIZATION|API_KEY' >/dev/null; then
  fail "Tobari environment exposes credential metadata"
fi
mounts=$(docker inspect --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' "$work_container")
if grep -E '^/(run/tobari/credentials|var/run/docker.sock)$' <<<"$mounts" >/dev/null; then
  fail "Tobari has a forbidden mount"
fi

set +e
run_project sh -c 'exit 37'
exec_status=$?
set -e
[[ $exec_status == 37 ]] || fail "exec returned $exec_status instead of child status 37"
status_after_exec=$(run_tobari_at "$work_root" status --format json)
assert_contains "$status_after_exec" '"runtime":"ready"' "runtime remains ready after child exit"

run_project sh -c 'sleep 1' &
first_pid=$!
run_project sh -c 'sleep 1' &
second_pid=$!
wait "$first_pid"
wait "$second_pid"

docker rm -f "$mock_name" >/dev/null
status_before_delete=$(run_tobari_at "$work_root" status --format json)
assert_contains "$status_before_delete" '"exists":true' "status before delete"
docker rm -f "$work_container" >/dev/null
docker network disconnect -f "$work_network" tobari-gateway >/dev/null 2>&1 || true
docker network rm "$work_network" >/dev/null 2>&1 || true
run_tobari_at "$work_root" delete --force >/dev/null
work_id=
work_container=
[[ ! -e "$work_home/tool-auth-state" ]] || fail "delete did not remove tool authentication state"
status_after_delete=$(run_tobari_at "$work_root" status --format json)
assert_contains "$status_after_delete" '"exists":false' "status after delete"
[[ -f "$profile_skill" && -f "$profile_settings" ]] || fail "delete removed the shared agent profile"
python3 - "$config_directory/principals.json" "$other_id" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
if [item["project_id"] for item in bindings] != [sys.argv[2]]:
    raise SystemExit(f"deleted project principal was not removed: {bindings!r}")
PY
run_tobari_at "$other_root" delete --force >/dev/null
other_id=
other_container=
python3 - "$config_directory/principals.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
if bindings:
    raise SystemExit(f"project principal registry was not cleared: {bindings!r}")
PY
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
