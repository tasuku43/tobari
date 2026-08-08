#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

binary=${TOBARI_INTEGRATION_BINARY:-$PWD/bin/tobari}
custom_base_image=${TOBARI_INTEGRATION_CUSTOM_BASE:-ghcr.io/tasuku43/tobari/runtime:latest}
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
runtime_image=
official_runtime_image=
host_docker_config=${DOCKER_CONFIG:-$HOME/.docker}
host_docker_context=${DOCKER_CONTEXT:-$(docker context show)}

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
    DOCKER_CONTEXT="$host_docker_context" \
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
    env \
      HOME="$test_root/user" \
      DOCKER_CONFIG="$host_docker_config" \
      DOCKER_CONTEXT="$host_docker_context" \
      TOBARI_CREDENTIAL_ADAPTER=passthrough \
      XDG_CONFIG_HOME="$test_root/config" \
      XDG_STATE_HOME="$test_root/state" \
      XDG_DATA_HOME="$test_root/data" \
      TERM=xterm-256color \
      python3 -c '
import errno
import fcntl
import json
import os
import pty
import select
import signal
import struct
import sys
import termios
import time

argv = sys.argv[1:]
try:
    scheduled_events = json.loads(os.environ.pop("TOBARI_TEST_PTY_EVENTS", "[]"))
    timeout_seconds = float(os.environ.pop("TOBARI_TEST_PTY_TIMEOUT_SECONDS", "60"))
except (TypeError, ValueError, json.JSONDecodeError) as error:
    print(f"invalid integration PTY configuration: {error}", file=sys.stderr)
    raise SystemExit(2)
if not isinstance(scheduled_events, list) or timeout_seconds <= 0:
    print("invalid integration PTY configuration", file=sys.stderr)
    raise SystemExit(2)

pid, master = pty.fork()
if pid == 0:
    os.execvpe(argv[0], argv, os.environ)

fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
os.set_blocking(master, False)
started = time.monotonic()
event_index = 0
next_event = started
if scheduled_events:
    next_event += float(scheduled_events[0].get("after_ms", 0)) / 1000
stdin_open = not scheduled_events
status = None
master_closed = False
while status is None:
    elapsed = time.monotonic() - started
    if elapsed >= timeout_seconds:
        print(
            f"integration PTY timed out after {timeout_seconds:.1f}s; "
            f"events_sent={event_index}/{len(scheduled_events)}",
            file=sys.stderr,
        )
        try:
            os.kill(pid, signal.SIGTERM)
        except ProcessLookupError:
            pass
        try:
            _, status = os.waitpid(pid, 0)
        except ChildProcessError:
            status = 1
        raise SystemExit(124)

    readable = [] if master_closed else [master]
    if stdin_open:
        readable.append(0)
    wait_seconds = 0.1
    if event_index < len(scheduled_events):
        wait_seconds = min(wait_seconds, max(0, next_event - time.monotonic()))
    ready, _, _ = select.select(readable, [], [], wait_seconds)
    if 0 in ready:
        data = os.read(0, 4096)
        if data:
            os.write(master, data)
        else:
            stdin_open = False
            try:
                os.write(master, b"\x04")
            except OSError as error:
                if error.errno not in (errno.EIO, errno.EPIPE):
                    raise
    if master in ready:
        try:
            data = os.read(master, 4096)
        except OSError as error:
            if error.errno == errno.EIO:
                data = b""
                master_closed = True
            else:
                raise
        if data:
            os.write(1, data)
    now = time.monotonic()
    while event_index < len(scheduled_events) and now >= next_event:
        event = scheduled_events[event_index]
        data = str(event.get("data", "")).encode("utf-8")
        if not master_closed:
            try:
                os.write(master, data)
            except OSError as error:
                if error.errno not in (errno.EIO, errno.EPIPE):
                    raise
                master_closed = True
        event_index += 1
        if event_index < len(scheduled_events):
            next_event += float(scheduled_events[event_index].get("after_ms", 0)) / 1000
    waited, status = os.waitpid(pid, os.WNOHANG)
    if waited == 0:
        status = None

while True:
    try:
        data = os.read(master, 4096)
    except OSError as error:
        if error.errno in (errno.EAGAIN, errno.EWOULDBLOCK, errno.EIO):
            break
        raise
    if not data:
        break
    os.write(1, data)

if os.WIFEXITED(status):
    raise SystemExit(os.WEXITSTATUS(status))
raise SystemExit(128 + os.WTERMSIG(status))
' "$binary" "$@"
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

assert_base_bash_contract() {
  local image=$1
  local output
  output=$(docker run --rm --entrypoint /bin/bash "$image" -lc '
    test -x /bin/bash
    test "$(getent passwd tobari | cut -d: -f7)" = /bin/bash
    test "$(id -un)" = tobari
    printf "base-bash-ok shell=%s user=%s\\n" "$BASH" "$(id -un)"
  ')
  assert_contains "$output" "base-bash-ok" "base runtime Bash contract"
  [[ $(docker image inspect --format '{{json .Config.Cmd}}' "$image") == '["sleep","infinity"]' ]] ||
    fail "$image changed the infrastructure-owned lifetime command"
  [[ $(docker image inspect --format '{{json .Config.Entrypoint}}' "$image") == '["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]' ]] ||
    fail "$image changed the fixed Tobari entrypoint"
}

enter_bash_tobari_at() {
  local root=$1
  shift
  local output
  if output=$({
    sleep 1
    printf '%s\n' "printf \"tobari-shell:%s\\n\" \"\$BASH\""
    printf 'if test -t 0 && test -t 1 && test -t 2; then printf "tobari-tty:yes\\n"; else printf "tobari-tty:no\\n"; fi\n'
    printf 'exit\n'
  } | run_tobari_pty_at "$root" "$@" 2>&1); then
    :
  else
    printf '%s\n' "$output" >&2
    return 1
  fi
  assert_contains "$output" "tobari-shell:" "interactive Bash entry"
  assert_contains "$output" "tobari-tty:yes" "interactive Bash TTY"
}

run_other_project() {
  docker exec "$other_container" "$@"
}

start_cluster() {
  run_tobari cluster up
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
  if [[ -n ${runtime_image:-} ]]; then
    docker image rm -f "$runtime_image" >/dev/null 2>&1 || true
  fi
  if [[ -n ${official_runtime_image:-} ]]; then
    docker image rm -f "$official_runtime_image" >/dev/null 2>&1 || true
  fi
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
mkdir -p "$test_root/user/workspace" "$test_root/config/tobari" "$test_root/state"

config_directory=$test_root/config/tobari
policy_directory=$config_directory/contexts/default/policy
tool_auth_value=tobari-tool-auth-canary

if [[ -n ${TOBARI_INTEGRATION_BINARY:-} ]]; then
  [[ -x $binary ]] || fail "TOBARI_INTEGRATION_BINARY is not executable: $binary"
else
  go build -buildvcs=false -trimpath -o "$binary" ./cmd/tobari
fi
work_root=$test_root/user/workspace
work_nested_root=$work_root/root
other_root=$test_root/user/other-workspace
mkdir -p "$work_root" "$work_nested_root" "$other_root"
printf 'host-home-canary\n' >"$test_root/user/host-home-canary"
docker build --tag "$custom_image" \
  --file test/integration/custom-image.Dockerfile \
  --build-arg "TOBARI_RUNTIME_BASE=$custom_base_image" . >/dev/null
assert_base_bash_contract "$custom_base_image"
printf '{"version":"v1","default_image":"%s"}\n' "$custom_image" \
  >"$config_directory/config.json"
chmod 0600 "$config_directory/config.json"
unconfigured_context_use=$(run_tobari context use --name default --format json)
assert_contains "$unconfigured_context_use" '"cluster":"not_configured"' "unconfigured Context selection"
start_cluster >/dev/null
assert_component_log_bounds tobari-opa
assert_component_log_bounds tobari-gateway
assert_component_resource_bounds tobari-opa 1000000000 536870912 128
assert_component_resource_bounds tobari-gateway 2000000000 1073741824 256
run_tobari context create --name project-tools --image "$custom_image" --format json >/dev/null
running_context_use=$(run_tobari context use --name project-tools --format json)
assert_contains "$running_context_use" '"cluster":"reconciled"' "running Context switch"
opa_context_mounts=$(docker inspect --format '{{range .Mounts}}{{println .Source "=>" .Destination}}{{end}}' tobari-opa)
if [[ $opa_context_mounts != *"$config_directory/contexts/project-tools/policy => /policy"* ]]; then
  printf 'selected OPA policy mounts:\n%s\n' "$opa_context_mounts" >&2
  fail "selected OPA policy mount did not point to project-tools"
fi
gateway_context_mounts=$(docker inspect --format '{{range .Mounts}}{{println .Source "=>" .Destination}}{{end}}' tobari-gateway)
if [[ $gateway_context_mounts != *"$config_directory/contexts/project-tools/credentials.json => /run/tobari/config/credentials.json"* ]]; then
  printf 'selected Gateway mounts:\n%s\n' "$gateway_context_mounts" >&2
  fail "selected Gateway credential config mount did not point to project-tools"
fi
running_context_use_pty=$(run_tobari_pty_at "$test_root/user" context use --name default)
assert_contains "$running_context_use_pty" "Cluster: reconciled" "PTY running Context switch"
assert_contains "$running_context_use_pty" "Next: run \`tobari\` from a project directory." "PTY Context switch continuation"
docker stop tobari-gateway tobari-opa >/dev/null
stopped_context_use=$(run_tobari context use --name project-tools --format json)
assert_contains "$stopped_context_use" '"cluster":"not_running"' "stopped Context selection"
[[ $(docker inspect --format '{{.State.Running}}' tobari-gateway) == false ]] || fail "Context selection started the stopped Gateway"
start_cluster >/dev/null
default_context_use=$(run_tobari context use --name default --format json)
assert_contains "$default_context_use" '"cluster":"reconciled"' "running Context switch back"
default_context=$(run_tobari context show --format json)
assert_contains "$default_context" '"active":true' "Context selection after explicit recovery"
container_work_root="/var/lib/tobari/${work_root#"$test_root/user/"}"
container_nested_root="/var/lib/tobari/${work_nested_root#"$test_root/user/"}"
enter_tobari_at "$work_root"
enter_ancestor_tobari_at "$work_nested_root"
enter_tobari_at "$other_root"

status_from_nested=$(run_tobari_at "$work_nested_root" status --format json)
assert_contains "$status_from_nested" '"exists":true' "nested status"
assert_contains "$status_from_nested" "\"root\":\"$work_root\"" "nested status root"
nested_pwd=$({ printf '\r'; sleep 1; printf 'pwd\nexit\n'; } | run_tobari_pty_at "$work_nested_root" 2>&1)
assert_contains "$nested_pwd" "$container_nested_root" "nested host CWD mapping"
list_json=$(run_tobari_at "$work_root" list --format json)
work_id=$(id_for_root "$work_root" <<<"$list_json")
other_id=$(id_for_root "$other_root" <<<"$list_json")
work_container=$(container_for_id "$work_id")
work_network=$(network_for_id "$work_id")
other_container=$(container_for_id "$other_id")
enter_bash_tobari_at "$work_root"
[[ $(docker inspect --format '{{.State.Running}}' "$work_container") == true ]] ||
  fail "Workspace stopped after the interactive Bash child exited"
[[ $(docker inspect --format '{{json .Config.Cmd}}' "$work_container") == '["sleep","infinity"]' ]] ||
  fail "Workspace lifetime command was not sleep infinity after Bash exit"
[[ $work_id =~ ^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]] ||
  fail "list did not return the project's stable ID"
[[ $work_id != "$other_id" ]] || fail "CWD projects received the same stable ID"

python3 - "$config_directory/principal-registry/principals.json" "$work_id" "$other_id" <<'PY'
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
[[ $(run_project printenv HOME) == /var/lib/tobari ]] || fail "project HOME is not /var/lib/tobari"
[[ $(run_project sh -c 'command -v gh') == /usr/local/bin/gh ]] || fail "GitHub CLI disappeared behind the project mount"
[[ $(run_project sh -c 'command -v aws') == /usr/local/bin/aws ]] || fail "AWS CLI disappeared behind the project mount"
if run_project test -e /var/lib/tobari/host-home-canary; then
  fail "Tobari mounted the host home wholesale"
fi

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
if ! docker exec "$work_container" test -e "$container_work_root/.claude/project.md"; then
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

start_cluster >/dev/null
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

if run_project test -e "$container_work_root/credentials"; then
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
start_cluster >/dev/null
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
[[ $body_status == 200 ]] || fail "allowed nonempty-body request returned $body_status instead of 200"
docker logs "$mock_name" 2>&1 | grep -F '"/body"' >/dev/null ||
  fail "allowed nonempty-body request did not reach mock upstream"

upload_output="$test_root/stream-upload.out"
docker exec "$work_container" python3 -c '
import http.client
import time

connection = http.client.HTTPConnection("gateway", 8080, timeout=10)
connection.putrequest("POST", "http://mock-upstream:8080/stream-upload", skip_host=True)
connection.putheader("Host", "mock-upstream:8080")
connection.putheader("Transfer-Encoding", "chunked")
connection.endheaders()
connection.send(b"5\r\nfirst\r\n")
time.sleep(5)
connection.send(b"6\r\nsecond\r\n0\r\n\r\n")
response = connection.getresponse()
print(response.status)
response.read()
' >"$upload_output" &
upload_pid=$!
first_chunk_seen=false
for _ in $(seq 1 40); do
  if docker logs "$mock_name" 2>&1 | grep -F '"event":"first_request_chunk"' >/dev/null; then
    first_chunk_seen=true
    break
  fi
  sleep 0.1
done
[[ $first_chunk_seen == true ]] || fail "allowed chunked request was buffered before upstream forwarding"
wait "$upload_pid"
[[ $(<"$upload_output") == 200 ]] || fail "chunked request did not complete successfully"

stream_prefix=$(run_project curl -NsS --max-time 1 \
  http://mock-upstream:8080/stream-response || true)
assert_contains "$stream_prefix" 'data: first' "streaming response prefix"
if [[ $stream_prefix == *'data: second'* ]]; then
  fail "streaming response completed before the upstream delay"
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
deny_body=$(run_project curl -sS -X POST http://mock-upstream:8080/denied)
assert_contains "$deny_body" '"error":"policy_denied"' "agent denial response"
assert_contains "$deny_body" '"event":"permission_review_unavailable"' "baseline denial response"
assert_contains "$deny_body" '"available":false' "baseline denial response"
assert_contains "$deny_body" '"command":null' "baseline denial response"
assert_contains "$deny_body" '"automatic_retry":false' "agent denial response"
assert_contains "$deny_body" '"retry_after_review":false' "baseline denial response"
assert_contains "$deny_body" '"path":"/denied"' "agent denial response"
if [[ $deny_body == *"$tool_auth_value"* || $deny_body == *'Bearer '* || $deny_body == *'"key"'* ]]; then
  fail "agent denial response contains a credential or request secret"
fi
if docker logs "$mock_name" 2>&1 | grep -F '"/denied"' >/dev/null; then
  fail "denied request reached mock upstream"
fi

review_allow_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-allow)
[[ $review_allow_status == 403 ]] || fail "review allow candidate returned $review_allow_status instead of 403"
review_deny_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-deny)
[[ $review_deny_status == 403 ]] || fail "review deny candidate returned $review_deny_status instead of 403"
review_body_first_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'content-type: application/json' --data '{"value":"first"}' \
  http://mock-upstream:8080/review-body)
[[ $review_body_first_status == 403 ]] ||
  fail "first body-bearing review candidate returned $review_body_first_status instead of 403"
review_body_second_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'content-type: application/json' --data '{"value":"second"}' \
  http://mock-upstream:8080/review-body)
[[ $review_body_second_status == 403 ]] ||
  fail "second body-bearing review candidate returned $review_body_second_status instead of 403"
review_patch_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PATCH -H 'content-type: application/json' --data '{"value":"patch"}' \
  http://mock-upstream:8080/review-patch)
[[ $review_patch_status == 403 ]] ||
  fail "body-bearing PATCH candidate returned $review_patch_status instead of 403"
review_allow_body=$(run_project curl -sS -X PUT http://mock-upstream:8080/review-allow)
assert_contains "$review_allow_body" '"event":"permission_review_available"' "learnable denial response"
assert_contains "$review_allow_body" '"command":"tobari policy review"' "learnable denial response"
assert_contains "$review_allow_body" '"automatic_retry":false' "learnable denial response"
assert_contains "$review_allow_body" '"retry_after_review":true' "learnable denial response"
review_deny_body=$(run_project curl -sS -X PUT http://mock-upstream:8080/review-deny)
assert_contains "$review_deny_body" '"event":"permission_review_available"' "learnable denial response"
assert_contains "$review_deny_body" '"command":"tobari policy review"' "learnable denial response"
if [[ $review_allow_body == *"$tool_auth_value"* || $review_allow_body == *'Bearer '* || \
  $review_deny_body == *"$tool_auth_value"* || $review_deny_body == *'Bearer '* ]]; then
  fail "learnable denial response contains a credential value"
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
assert_contains "$denials_json" '"review_command":"tobari policy review"' "focused denial recovery"
if [[ $denials_json == *"$tool_auth_value"* || $denials_json == *'Bearer '* ]]; then
  fail "focused denial evidence contains a credential value"
fi

candidates_json=$(run_tobari policy candidates --tail 500 --format json)
if python3 -c 'import json,sys; sys.exit(0 if not any(item["path"] == "/denied" for item in json.load(sys.stdin)["policy_candidates"]) else 1)' <<<"$candidates_json"; then
  :
else
  fail "baseline explicit deny remained in the actionable policy queue"
fi
allow_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream PUT /review-allow <<<"$candidates_json")
deny_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream PUT /review-deny <<<"$candidates_json")
body_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream PUT /review-body <<<"$candidates_json")
patch_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream PATCH /review-patch <<<"$candidates_json")
[[ $allow_candidate_id == pcy_* && $deny_candidate_id == pcy_* && \
  $body_candidate_id == pcy_* && $patch_candidate_id == pcy_* ]] ||
  fail "policy candidates did not emit opaque candidate IDs"
python3 -c '
import json
import sys

project_id, candidate_id = sys.argv[1:]
candidate = next(
    item for item in json.load(sys.stdin)["policy_candidates"]
    if item["id"] == candidate_id and item["project_id"] == project_id
)
assert candidate["observation_count"] == 2, candidate
assert "body" not in candidate, candidate
' "$work_id" "$body_candidate_id" <<<"$candidates_json" ||
  fail "body variants did not aggregate into one body-free policy candidate"
assert_contains "$candidates_json" \
	"\"allow_command\":\"tobari policy allow --id $allow_candidate_id\"" \
	"policy candidate exact action"
assert_contains "$candidates_json" \
	"\"deny_command\":\"tobari policy deny --id $deny_candidate_id\"" \
	"policy candidate exact rejection"
tail_output=$(run_tobari policy tail --tail 500)
assert_contains "$tail_output" \
	"allow_command=tobari policy allow --id $allow_candidate_id" \
	"human policy tail"
review_output=$(run_tobari policy review --tail 500)
assert_contains "$review_output" \
	"Allow exact    tobari policy allow --id $allow_candidate_id" \
	"human policy review"
assert_contains "$review_output" \
	"Deny exact     tobari policy deny --id $deny_candidate_id" \
	"human policy review"
review_json=$(run_tobari policy review --tail 500 --format json)
assert_contains "$review_json" \
	"\"allow_command\":\"tobari policy allow --id $allow_candidate_id\"" \
	"machine policy review"
assert_contains "$review_json" \
	"\"deny_command\":\"tobari policy deny --id $deny_candidate_id\"" \
	"machine policy review"

allow_output=$(run_tobari policy allow --id "$allow_candidate_id")
assert_contains "$allow_output" "policy: $policy_directory" "exact policy approval"
assert_contains "$allow_output" 'match: exact' "exact policy approval"
assert_contains "$allow_output" 'path: /review-allow' "exact policy approval"
assert_contains "$allow_output" 'applied: true' "exact policy approval"

body_allow_output=$(run_tobari policy allow --id "$body_candidate_id")
assert_contains "$body_allow_output" 'path: /review-body' "body-independent policy approval"
assert_contains "$body_allow_output" 'applied: true' "body-independent policy approval"
patch_deny_output=$(run_tobari policy deny --id "$patch_candidate_id")
assert_contains "$patch_deny_output" 'path: /review-patch' "body-bearing PATCH policy review"
assert_contains "$patch_deny_output" 'applied: true' "body-bearing PATCH policy review"
body_applied_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'content-type: application/json' --data '{"value":"third"}' \
  http://mock-upstream:8080/review-body)
[[ $body_applied_status == 200 ]] ||
  fail "body-independent learned policy did not allow a new body value"

applied_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-allow)
[[ $applied_status == 200 ]] || fail "exact learned policy was not active after policy allow"
other_learned_status=$(run_other_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-allow)
[[ $other_learned_status == 403 ]] ||
  fail "learned policy crossed the project boundary with status $other_learned_status"
child_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-allow/child)
[[ $child_status == 403 ]] || fail "exact learned policy broadened to a child path"

policy_rules_json=$(run_tobari policy rules --format json)
assert_contains "$policy_rules_json" '"policy_rules":' "current policy decision inventory"
allow_rule_id=$(python3 -c \
  'import json,sys
print(next(item["id"] for item in json.load(sys.stdin)["policy_rules"]
           if item["decision"] == "allow" and item["path"] == "/review-allow"))' \
  <<<"$policy_rules_json")
[[ $allow_rule_id == plr_* ]] || fail "policy rules did not emit the learned Allow ID"
assert_contains "$policy_rules_json" \
  "\"reset_command\":\"tobari policy reset --id $allow_rule_id\"" \
  "policy decision reset action"

reset_allow_output=$(run_tobari policy reset --id "$allow_rule_id")
assert_contains "$reset_allow_output" "target_id: $allow_rule_id" "learned Allow reset"
assert_contains "$reset_allow_output" 'decision: allow' "learned Allow reset"
assert_contains "$reset_allow_output" 'applied: true' "learned Allow reset"
reset_allow_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-allow)
[[ $reset_allow_status == 403 ]] || fail "reset Allow did not return the request to default deny"
review_after_allow_reset=$(run_tobari policy review --tail 1000 --format json)
assert_contains "$review_after_allow_reset" "\"id\":\"$allow_candidate_id\"" \
  "reset Allow re-review queue"
allow_output=$(run_tobari policy allow --id "$allow_candidate_id")
assert_contains "$allow_output" 'applied: true' "re-allow after reset"
reallowed_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-allow)
[[ $reallowed_status == 200 ]] || fail "re-review could not restore the exact Allow"

deny_output=$(run_tobari policy deny --id "$deny_candidate_id")
assert_contains "$deny_output" "policy: $policy_directory" "exact policy rejection"
assert_contains "$deny_output" 'path: /review-deny' "exact policy rejection"
assert_contains "$deny_output" 'applied: true' "exact policy rejection"
rejected_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-deny)
[[ $rejected_status == 403 ]] || fail "exact policy rejection changed the denied request to $rejected_status"
review_after_deny=$(run_tobari policy review --tail 1000 --format json)
if [[ $review_after_deny == *"$deny_candidate_id"* ]]; then
  fail "denied candidate remained in the review queue"
fi

policy_rules_json=$(run_tobari policy rules --format json)
deny_rule_id=$(python3 -c \
  'import json,sys
print(next(item["id"] for item in json.load(sys.stdin)["policy_rules"]
           if item["decision"] == "deny" and item["path"] == "/review-deny"))' \
  <<<"$policy_rules_json")
[[ $deny_rule_id == pdr_* ]] || fail "policy rules did not emit the learned Deny ID"
reset_deny_output=$(run_tobari policy reset --id "$deny_rule_id")
assert_contains "$reset_deny_output" "target_id: $deny_rule_id" "learned Deny reset"
assert_contains "$reset_deny_output" 'decision: deny' "learned Deny reset"
reset_deny_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-deny)
[[ $reset_deny_status == 403 ]] || fail "reset Deny weakened default denial"
review_after_deny_reset=$(run_tobari policy review --tail 1000 --format json)
assert_contains "$review_after_deny_reset" "\"id\":\"$deny_candidate_id\"" \
  "reset Deny re-review queue"
deny_output=$(run_tobari policy deny --id "$deny_candidate_id")
assert_contains "$deny_output" 'applied: true' "re-deny after reset"

reject_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/rejected)
[[ $reject_status == 403 ]] || fail "rejection candidate request returned $reject_status instead of 403"
reject_candidates_json=$(run_tobari policy review --tail 1000 --format json)
reject_candidate_id=$(python3 -c \
  'import json,sys
print(next(item["id"] for item in json.load(sys.stdin)["policy_review"]
           if item["project_id"] == sys.argv[1] and item["host"] == "mock-upstream" and item["method"] == "PUT" and item["path"] == "/rejected"))' \
  "$work_id" <<<"$reject_candidates_json")
[[ $reject_candidate_id == pcy_* ]] || fail "policy review JSON did not emit the rejection candidate"
assert_contains "$reject_candidates_json" \
  "\"deny_command\":\"tobari policy deny --id $reject_candidate_id\"" \
  "policy review JSON rejection action"
deny_output=$(run_tobari policy deny --id "$reject_candidate_id")
assert_contains "$deny_output" 'applied: true' "exact policy rejection"
rejected_after_deny=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/rejected)
[[ $rejected_after_deny == 403 ]] || fail "exact policy rejection changed the denied request to $rejected_after_deny"
remaining_review=$(run_tobari policy review --tail 1000 --format json)
if [[ $remaining_review == *"$reject_candidate_id"* ]]; then
  fail "denied candidate remained in the review queue"
fi

pending_before_interactive=$(run_tobari policy review --tail 1000 --format json)
while IFS= read -r pending_id; do
  [[ -n $pending_id ]] || continue
  run_tobari policy deny --id "$pending_id" >/dev/null
done < <(python3 -c '
import json
import sys

for item in json.load(sys.stdin)["policy_review"]:
    print(item["id"])
' <<<"$pending_before_interactive")

interactive_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review-interactive)
[[ $interactive_status == 403 ]] || fail "interactive review candidate returned $interactive_status instead of 403"
if ! interactive_output=$(TOBARI_TEST_PTY_TIMEOUT_SECONDS=15 \
  TOBARI_TEST_PTY_EVENTS='[{"after_ms":5000,"data":"1"},{"after_ms":750,"data":"d"},{"after_ms":750,"data":"y"},{"after_ms":750,"data":"q"}]' \
  run_tobari_pty_at "$work_root" policy review --tail 1000 2>&1); then
  printf '%s\n' "$interactive_output" >&2
  fail "interactive policy review PTY session failed"
fi
assert_contains "$interactive_output" 'Permission denied' "interactive policy review"
interactive_review=$(run_tobari policy review --tail 1000 --format json)
if python3 -c 'import json,sys; sys.exit(0 if any(item["path"] == "/review-interactive" for item in json.load(sys.stdin)["policy_review"]) else 1)' <<<"$interactive_review"; then
  fail "interactive deny did not remove the candidate from the review queue"
fi

for item_path in one two three; do
  item_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
    -X PUT "http://mock-upstream:8080/review/items/$item_path")
  [[ $item_status == 403 ]] || fail "compaction source $item_path was not initially denied"
done

for item_path in one two three; do
  candidates_json=$(run_tobari policy candidates --tail 1000 --format json)
  item_candidate_id=$(candidate_id_for_effect \
    "$work_id" mock-upstream PUT "/review/items/$item_path" <<<"$candidates_json")
  item_allow_output=$(run_tobari policy allow --id "$item_candidate_id")
  assert_contains "$item_allow_output" "path: /review/items/$item_path" \
    "exact compaction source approval"
done

for item_path in one two three; do
  item_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
    -X PUT "http://mock-upstream:8080/review/items/$item_path")
  [[ $item_status == 200 ]] || fail "exact source rule $item_path was not active"
done

compactions_json=$(run_tobari policy compactions --format json)
compaction_id=$(compaction_id_for_prefix \
  "$work_id" mock-upstream PUT /review/items/ <<<"$compactions_json")
[[ $compaction_id == pcx_* ]] || fail "policy compactions did not emit an opaque compaction ID"
assert_contains "$compactions_json" '"source_rule_count":3' "compaction evidence"
assert_contains "$compactions_json" \
  '"outside_canary":"/review/items-outside-tobari-canary"' \
  "compaction boundary"

compact_output=$(run_tobari policy compact --id "$compaction_id")
assert_contains "$compact_output" 'match: prefix' "policy compaction"
assert_contains "$compact_output" 'path: /review/items/' "policy compaction"
assert_contains "$compact_output" 'source_rule_count: 3' "policy compaction"
assert_contains "$compact_output" 'applied: true' "policy compaction"

compacted_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review/items/four)
[[ $compacted_status == 200 ]] || fail "compacted prefix did not allow a sibling path"
outside_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT http://mock-upstream:8080/review/items-outside-tobari-canary)
[[ $outside_status == 403 ]] || fail "compacted prefix crossed its tested directory boundary"

policy_help=$(run_tobari help policy)
if [[ $policy_help == *"policy apply"* ]]; then
  fail "retired policy apply command remains in public policy help"
fi

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
python3 - "$config_directory/principal-registry/principals.json" "$other_id" <<'PY'
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
python3 - "$config_directory/principal-registry/principals.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
if bindings:
    raise SystemExit(f"project principal registry was not cleared: {bindings!r}")
PY
runtime_init_json=$(run_tobari runtime init --format json)
runtime_dockerfile=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["runtime"]["dockerfile"])' <<<"$runtime_init_json")
official_runtime_build_json=$(run_tobari runtime build --format json)
official_runtime_image=$(python3 -c 'import json,sys; d=json.load(sys.stdin)["context"]; assert d["runtime"]["status"] == "ready"; print(d["image"])' <<<"$official_runtime_build_json")
[[ $official_runtime_image == tobari-context-default:* ]] || fail "official runtime build selected an unexpected image: $official_runtime_image"
official_runtime_context=$(run_tobari context show --format json)
assert_contains "$official_runtime_context" "\"image\":\"$official_runtime_image\"" "Official Context runtime promotion"
assert_contains "$official_runtime_context" '"status":"ready"' "Official Context runtime status"
runtime_template_base=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["runtime"]["base_reference"])' <<<"$runtime_init_json")
python3 - "$runtime_dockerfile" "$runtime_template_base" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
base = sys.argv[2]
text = path.read_text(encoding="utf-8")
expected = f"FROM {base}"
if text.count(expected) != 1:
    raise SystemExit(f"runtime template did not contain one base reference: {expected}")
path.write_text(text + "\n# integration custom runtime rebuild\n", encoding="utf-8")
PY
runtime_build_json=$(run_tobari runtime build --format json)
runtime_image=$(python3 -c 'import json,sys; d=json.load(sys.stdin)["context"]; assert d["runtime"]["status"] == "ready"; print(d["image"])' <<<"$runtime_build_json")
[[ $runtime_image == tobari-context-default:* ]] || fail "runtime build selected an unexpected image: $runtime_image"
runtime_context=$(run_tobari context show --format json)
assert_contains "$runtime_context" "\"image\":\"$runtime_image\"" "Context runtime promotion"
assert_contains "$runtime_context" '"status":"ready"' "Context runtime status"

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
