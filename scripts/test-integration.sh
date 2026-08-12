#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "$0")/.."

binary=${TOBARI_INTEGRATION_BINARY:-$PWD/bin/tobari}
custom_base_image=${TOBARI_INTEGRATION_CUSTOM_BASE:-ghcr.io/tasuku43/tobari/runtime:latest}
mock_name=tobari-mock-upstream
auth_mock_name=tobari-auth-mock-upstream
auth_network=tobari-auth-integration
custom_image="tobari-integration-custom-$$"
gateway_base_image="tobari-gateway-integration-base-$$"
test_keychain_service=
test_root=
work_root=
work_nested_root=
other_root=
work_id=
other_id=
restricted_id=
work_container=
other_container=
restricted_container=
runtime_image=
official_runtime_image=
created_dev_runtime_tag=false
owns_shared_fixture=false
host_docker_config=${DOCKER_CONFIG:-$HOME/.docker}
host_docker_context=${DOCKER_CONTEXT:-$(docker context show)}

fail() {
  echo "integration: $*" >&2
  exit 1
}

report_unexpected_failure() {
  local status=$?
  if [[ $- == *e* ]]; then
    echo "integration: unexpected command failure near lines ${BASH_LINENO[*]}" >&2
  fi
  return "$status"
}

trap report_unexpected_failure ERR

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

network_contains_container() {
  local network=$1
  local container=$2
  local container_id
  local member_ids
  container_id=$(docker inspect --format '{{.Id}}' "$container")
  member_ids=$(docker network inspect --format '{{range $id, $_ := .Containers}}{{println $id}}{{end}}' "$network")
  grep -Fx "$container_id" <<<"$member_ids" >/dev/null
}

wait_network_membership() {
  local network=$1
  local container=$2
  local _
  for _ in $(seq 1 60); do
    if network_contains_container "$network" "$container"; then
      return 0
    fi
    sleep 0.5
  done
  return 1
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
      XDG_CONFIG_HOME="$test_root/config" \
      XDG_STATE_HOME="$test_root/state" \
      XDG_DATA_HOME="$test_root/data" \
      NO_COLOR=1 \
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
  local context=${2:-}
  python3 -c \
    'import json,sys
root,context=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["tobari"]
           if item["root"] == root and (not context or item["context"] == context)))' \
    "$root" "$context"
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

run_restricted_project() {
  docker exec "$restricted_container" "$@"
}

create_nested_tobari_at() {
  local root=$1
  shift
  local output
  if output=$({ printf 'n'; sleep 1; printf 'exit\n'; } | run_tobari_pty_at "$root" "$@" 2>&1); then
    return 0
  fi
  printf '%s\n' "$output" >&2
  return 1
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

graphql_candidate_id_for_effect() {
	local project_id=$1
	local host=$2
	local operation_type=$3
	local root_field=$4
  python3 -c \
    'import json,sys
project_id,host,operation_type,root_field=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["policy_candidates"]
           if item["project_id"] == project_id and item["host"] == host
           and item["protocol"] == "graphql"
           and item["graphql_operation_type"] == operation_type
           and item["graphql_root_field"] == root_field))' \
    "$project_id" "$host" "$operation_type" "$root_field"
}

cleanup() {
  if [[ $owns_shared_fixture == true ]]; then
    docker rm -f "$mock_name" >/dev/null 2>&1 || true
    docker rm -f "$auth_mock_name" >/dev/null 2>&1 || true
    docker network rm "$auth_network" >/dev/null 2>&1 || true
    if [[ -n ${test_root:-} && -x $binary && -n ${work_root:-} ]]; then
      run_tobari_at "$work_root" delete --force >/dev/null 2>&1 || true
			run_tobari_at "$work_root" delete --context restricted --force >/dev/null 2>&1 || true
      if [[ -n $other_root ]]; then
        run_tobari_at "$other_root" delete --context restricted --force >/dev/null 2>&1 || true
      fi
      run_tobari cluster down --purge >/dev/null 2>&1 || true
    fi
    # A failed startup may leave an interrupted reconciliation journal that
    # prevents the public cleanup path from completing. The preflight above
    # requires these exact shared names to be absent before ownership is set,
    # so any survivors here were created by this integration run.
    for container in \
      tobari-auth-broker tobari-gateway tobari-opa \
      "$work_container" "$other_container" "$restricted_container"; do
      [[ -n $container ]] || continue
      docker rm -f "$container" >/dev/null 2>&1 || true
    done
    docker network rm "$auth_network" >/dev/null 2>&1 || true
    docker network rm tobari-control tobari-egress >/dev/null 2>&1 || true
    docker volume rm tobari-gateway-ca tobari-public-ca tobari-policy-bundle >/dev/null 2>&1 || true
    docker image rm -f "$custom_image" >/dev/null 2>&1 || true
    docker image rm -f "$gateway_base_image" >/dev/null 2>&1 || true
    if [[ -n ${runtime_image:-} ]]; then
      docker image rm -f "$runtime_image" >/dev/null 2>&1 || true
    fi
    if [[ -n ${official_runtime_image:-} ]]; then
      docker image rm -f "$official_runtime_image" >/dev/null 2>&1 || true
    fi
    if [[ $created_dev_runtime_tag == true ]]; then
      docker image rm tobari-runtime:dev >/dev/null 2>&1 || true
    fi
  fi
  if [[ -n $test_keychain_service ]]; then
    /usr/bin/security delete-generic-password \
      -a tobari -s "$test_keychain_service" >/dev/null 2>&1 || true
  fi
}

finish() {
  local status=$?
  trap - EXIT
  if ((status != 0)); then
    if [[ -n ${test_root:-} && -x $binary ]]; then
      echo "integration diagnostics: cluster status" >&2
      run_tobari cluster status --format json >&2 || true
    fi
    for container in tobari-auth-broker tobari-gateway tobari-opa "$mock_name" "$auth_mock_name" "$work_container" "$other_container" "$restricted_container"; do
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
if [[ $(uname -s) == Darwin ]]; then
  test_keychain_service="io.tobari.integration.$$"
  export TOBARI_TEST_KEYCHAIN_SERVICE=$test_keychain_service
fi
for name in tobari-auth-broker tobari-gateway tobari-opa "$mock_name" "$auth_mock_name"; do
  if docker inspect "$name" >/dev/null 2>&1; then
    fail "container $name already exists; stop the active Tobari cluster before integration tests"
  fi
done
for volume in tobari-gateway-ca tobari-public-ca tobari-policy-bundle; do
  if docker volume inspect "$volume" >/dev/null 2>&1; then
    fail "volume $volume already exists; use a clean Docker Engine for integration tests"
  fi
done
if docker network inspect "$auth_network" >/dev/null 2>&1; then
  fail "network $auth_network already exists; remove the stale integration fixture"
fi
owns_shared_fixture=true

test_root=$(mktemp -d "$PWD/.tobari-integration.XXXXXX")
mkdir -p \
  "$test_root/user/workspace" \
  "$test_root/config/tobari/auth/providers" \
  "$test_root/state" \
  "$test_root/tls"
chmod 0700 "$test_root/config/tobari/auth" "$test_root/config/tobari/auth/providers" "$test_root/tls"
chmod 0700 "$test_root/state"

config_directory=$test_root/config/tobari
tool_auth_value=tobari-tool-auth-canary
synthetic_default_secret=synthetic-real-default-canary
synthetic_restricted_secret=synthetic-real-restricted-canary
synthetic_provider=synthetic-ci
cat >"$config_directory/auth/providers/$synthetic_provider.json" <<'JSON'
{
  "schema_version": 1,
  "id": "synthetic-ci",
  "display_name": "Synthetic CI Provider",
  "acquisition": {"mode": "stdin_import"},
  "credential": {"kind": "primary_secret"},
  "workspace_projections": [
    {"kind": "env", "name": "SYNTHETIC_TOKEN", "template": "${HANDLE}"}
  ],
  "header_bindings": [
    {
      "target": {"scheme": "https", "host": "api.synthetic.example", "port": 443},
      "source": {"header": "x-synthetic-auth", "formats": ["raw"]},
      "destination": {
        "header": "authorization",
        "format": "bearer",
        "secret_field": "primary_secret"
      },
      "secret_headers": ["authorization", "x-synthetic-auth"]
    }
  ]
}
JSON
chmod 0600 "$config_directory/auth/providers/$synthetic_provider.json"
synthetic_noop_provider=synthetic-noop
cat >"$config_directory/auth/providers/$synthetic_noop_provider.json" <<'JSON'
{
  "schema_version": 1,
  "id": "synthetic-noop",
  "display_name": "Synthetic No-op Provider",
  "acquisition": {"mode": "stdin_import"},
  "credential": {"kind": "primary_secret"},
  "workspace_projections": [
    {"kind": "env", "name": "SYNTHETIC_NOOP_TOKEN", "template": "${HANDLE}"}
  ],
  "header_bindings": [
    {
      "target": {"scheme": "https", "host": "noop.synthetic.example", "port": 443},
      "source": {"header": "x-synthetic-noop-auth", "formats": ["raw"]},
      "destination": {
        "header": "authorization",
        "format": "bearer",
        "secret_field": "primary_secret"
      },
      "secret_headers": ["authorization", "x-synthetic-noop-auth"]
    }
  ]
}
JSON
chmod 0600 "$config_directory/auth/providers/$synthetic_noop_provider.json"

if [[ -n ${TOBARI_INTEGRATION_BINARY:-} ]]; then
  [[ -x $binary ]] || fail "TOBARI_INTEGRATION_BINARY is not executable: $binary"
else
  mitmproxy_image=$(awk -F= '$1 == "MITMPROXY_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
  debian_image=$(awk -F= '$1 == "DEBIAN_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
  docker_arch=$(docker info --format '{{.Architecture}}')
  case $docker_arch in
    amd64 | x86_64) auth_target_arch=amd64 ;;
    arm64 | aarch64) auth_target_arch=arm64 ;;
    *) fail "unsupported Docker architecture for Auth Broker integration: $docker_arch" ;;
  esac
  docker build --tag "$gateway_base_image" --file gateway/Dockerfile \
    --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" gateway >/dev/null
  docker run --rm --user "$(id -u):$(id -g)" \
    -v "$test_root/tls:/tls" \
    --entrypoint sh "$mitmproxy_image" -eu -c '
      openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 2 \
        -subj /CN=api.synthetic.example \
        -addext subjectAltName=DNS:api.synthetic.example,DNS:mock-upstream \
        -addext basicConstraints=critical,CA:TRUE \
        -addext keyUsage=critical,digitalSignature,keyEncipherment,keyCertSign \
        -addext extendedKeyUsage=serverAuth \
        -keyout /tls/synthetic-server.key \
        -out /tls/synthetic-ca.crt >/dev/null 2>&1
      chmod 0600 /tls/synthetic-server.key
      chmod 0644 /tls/synthetic-ca.crt
    '
  docker build --tag tobari-gateway:dev \
    --file test/integration/gateway-auth.Dockerfile \
    --build-arg "TOBARI_GATEWAY_BASE=$gateway_base_image" \
    "$test_root/tls" >/dev/null
  docker build --tag tobari-auth-broker:dev --file authbroker/Dockerfile \
    --build-arg "DEBIAN_IMAGE=$debian_image" \
    --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" \
    --build-arg "TARGETARCH=$auth_target_arch" \
    authbroker >/dev/null
  go build -tags=tobari_dev -buildvcs=false -trimpath -o "$binary" ./cmd/tobari
fi
work_root=$test_root/user/workspace
work_nested_root=$work_root/root
other_root=$work_root/child-workspace
mkdir -p "$work_root" "$work_nested_root" "$other_root"
printf 'host-home-canary\n' >"$test_root/user/host-home-canary"
docker build --tag "$custom_image" \
  --file test/integration/custom-image.Dockerfile \
  --build-arg "TOBARI_RUNTIME_BASE=$custom_base_image" . >/dev/null
assert_base_bash_contract "$custom_base_image"
if ! docker image inspect tobari-runtime:dev >/dev/null 2>&1; then
  docker tag "$custom_base_image" tobari-runtime:dev
  created_dev_runtime_tag=true
fi
default_context_create=$(run_tobari context create --name default --image "$custom_image" \
  --source-access read-write --policy-preset builtin/reviewed-exact --format json)
assert_contains "$default_context_create" '"source_access":"read-write"' "default Context source access"
assert_contains "$default_context_create" '"policy_preset_origin":"builtin/reviewed-exact"' "default Context policy preset"
preset_catalog=$(run_tobari policy preset list --format json)
PRESET_CATALOG_DOCUMENT="$preset_catalog" python3 <<'PY'
import json
import os

items = json.loads(os.environ["PRESET_CATALOG_DOCUMENT"])["policy_presets"]["items"]
builtins = {item["origin"]: item for item in items if item["origin"].startswith("builtin/")}
expected = {
    "builtin/offline",
    "builtin/reviewed-exact",
    "builtin/get-only-reviewed",
}
if set(builtins) != expected:
    raise SystemExit(f"unexpected built-in preset catalog: {builtins!r}")
if any(item["immediate_grant_count"] != 0 for item in builtins.values()):
    raise SystemExit(f"built-in preset granted authority immediately: {builtins!r}")
PY
custom_preset_init=$(run_tobari policy preset init --name snapshot --format json)
custom_preset_path=$(python3 -c \
  'import json,sys; print(json.load(sys.stdin)["policy_presets"]["source_path"])' \
  <<<"$custom_preset_init")
custom_preset_revision=$(python3 -c \
  'import json,sys; print(json.load(sys.stdin)["policy_presets"]["revision"])' \
  <<<"$custom_preset_init")
custom_context_create=$(run_tobari context create --name custom-snapshot --image "$custom_image" \
  --source-access read-write --policy-preset custom/snapshot --format json)
assert_contains "$custom_context_create" '"policy_preset_origin":"custom/snapshot"' \
  "custom preset Context origin"
assert_contains "$custom_context_create" "\"policy_preset_revision\":\"$custom_preset_revision\"" \
  "custom preset Context revision"
python3 - "$custom_preset_path" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as source:
    document = json.load(source)
document["guardrail"] = "reviewed_exact"
with open(path, "w", encoding="utf-8") as destination:
    json.dump(document, destination, separators=(",", ":"))
    destination.write("\n")
PY
custom_preset_validate=$(run_tobari policy preset validate --name custom/snapshot --format json)
updated_custom_revision=$(python3 -c \
  'import json,sys; print(json.load(sys.stdin)["policy_presets"]["revision"])' \
  <<<"$custom_preset_validate")
[[ $updated_custom_revision != "$custom_preset_revision" ]] ||
  fail "custom preset source edit did not change its revision"
custom_context_show=$(run_tobari context show --name custom-snapshot --format json)
assert_contains "$custom_context_show" "\"policy_preset_revision\":\"$custom_preset_revision\"" \
  "immutable custom preset snapshot"
unconfigured_context_use=$(run_tobari context use --name default --format json)
assert_contains "$unconfigured_context_use" '"cluster":"already_ready"' "unconfigured current Context selection"
start_cluster >/dev/null
assert_component_log_bounds tobari-opa
assert_component_log_bounds tobari-gateway
assert_component_log_bounds tobari-auth-broker
assert_component_resource_bounds tobari-opa 1000000000 536870912 128
assert_component_resource_bounds tobari-gateway 2000000000 1073741824 256
assert_component_resource_bounds tobari-auth-broker 1000000000 536870912 128
[[ $(docker ps -a --filter name='^/tobari-gateway$' --format '{{.Names}}' | wc -l | tr -d ' ') == 1 ]] || fail "cluster did not create exactly one Gateway"
[[ $(docker ps -a --filter name='^/tobari-opa$' --format '{{.Names}}' | wc -l | tr -d ' ') == 1 ]] || fail "cluster did not create exactly one OPA"
[[ $(docker ps -a --filter name='^/tobari-auth-broker$' --format '{{.Names}}' | wc -l | tr -d ' ') == 1 ]] || fail "cluster did not create exactly one Auth Broker"
[[ $(docker inspect --format '{{index .Config.Labels "io.tobari.gateway-api"}}' tobari-gateway) == 1 ]] ||
  fail "Gateway did not expose image API 1"
[[ $(docker inspect --format '{{index .Config.Labels "io.tobari.auth-broker-api"}}' tobari-auth-broker) == 1 ]] ||
  fail "Auth Broker did not expose image API 1"
[[ $(docker inspect --format '{{.HostConfig.ReadonlyRootfs}}' tobari-auth-broker) == true ]] ||
  fail "Auth Broker root filesystem is writable"
[[ $(docker inspect --format '{{join .HostConfig.CapDrop ","}}' tobari-auth-broker) == ALL ]] ||
  fail "Auth Broker did not drop every capability"
for provider_cli in gh aws pup codex claude; do
  if docker exec tobari-auth-broker python3 -c \
    'import shutil,sys; raise SystemExit(0 if shutil.which(sys.argv[1]) else 1)' \
    "$provider_cli" >/dev/null 2>&1; then
    fail "Auth Broker image unexpectedly contains provider CLI $provider_cli"
  fi
done
wait_network_membership tobari-control tobari-auth-broker ||
  fail "Auth Broker is not attached to the shared control network"
if network_contains_container tobari-egress tobari-auth-broker; then
  fail "static-only Auth Broker retained provider egress"
fi
docker network disconnect -f tobari-control tobari-auth-broker >/dev/null
start_cluster >/dev/null
wait_network_membership tobari-control tobari-auth-broker ||
  fail "Auth Broker lost the shared control network during egress reconciliation"
if network_contains_container tobari-egress tobari-auth-broker; then
  fail "Auth Broker joined provider egress during control reconciliation"
fi
created_context=$(run_tobari context create --name restricted --image "$custom_image" \
  --source-access read-only --policy-preset builtin/reviewed-exact --format json)
assert_contains "$created_context" '"cluster":"requires_reconcile"' "running Context creation"
assert_contains "$created_context" '"source_access":"read-only"' "restricted Context source access"
assert_contains "$created_context" '"policy_preset_origin":"builtin/reviewed-exact"' "restricted Context policy preset"
start_cluster >/dev/null
gateway_before_use=$(docker inspect --format '{{.Id}}' tobari-gateway)
opa_before_use=$(docker inspect --format '{{.Id}}' tobari-opa)
running_context_use=$(run_tobari context use --name restricted --format json)
assert_contains "$running_context_use" '"cluster":"default_updated"' "running current Context change"
[[ $(docker inspect --format '{{.Id}}' tobari-gateway) == "$gateway_before_use" ]] || fail "context use recreated Gateway"
[[ $(docker inspect --format '{{.Id}}' tobari-opa) == "$opa_before_use" ]] || fail "context use recreated OPA"
opa_policy_mount=$(docker inspect --format \
  '{{range .Mounts}}{{if eq .Destination "/bundle"}}{{.Type}}|{{.Name}}|{{.Destination}}|{{.RW}}{{end}}{{end}}' \
  tobari-opa)
[[ $opa_policy_mount == 'volume|tobari-policy-bundle|/bundle|false' ]] ||
  fail "OPA policy bundle mount is not the owned read-only volume: $opa_policy_mount"
gateway_context_mounts=$(docker inspect --format '{{range .Mounts}}{{println .Source "=>" .Destination}}{{end}}' tobari-gateway)
if [[ $gateway_context_mounts == *"/credentials.json =>"* || $gateway_context_mounts == *"/run/tobari/credentials"* ]]; then
  fail "Gateway retained the retired managed credential projection"
fi
assert_contains "$gateway_context_mounts" "/run/tobari/auth/providers.json" \
  "Gateway provider projection mount"
assert_contains "$gateway_context_mounts" "/run/tobari-auth/runtime" \
  "Gateway Auth Broker runtime socket mount"
gateway_environment=$(docker inspect --format '{{json .Config.Env}}' tobari-gateway)
assert_contains "$gateway_environment" \
  'TOBARI_AUTH_PROVIDER_PROJECTION=/run/tobari/auth/providers.json' \
  "Gateway provider projection environment"
gateway_projection_mode=$(docker exec tobari-gateway stat -c '%a:%u' /run/tobari/auth/providers.json)
[[ $gateway_projection_mode == "600:$(id -u)" ]] ||
  fail "Gateway provider projection ownership/mode is $gateway_projection_mode instead of 600:$(id -u)"
running_context_use_pty=$(run_tobari_pty_at "$test_root/user" context use --name default)
assert_contains "$running_context_use_pty" "Cluster: default_updated" "PTY current Context change"
docker stop tobari-gateway tobari-opa >/dev/null
stopped_context_use=$(run_tobari context use --name restricted --format json)
assert_contains "$stopped_context_use" '"cluster":"default_updated"' "stopped current Context change"
[[ $(docker inspect --format '{{.State.Running}}' tobari-gateway) == false ]] || fail "Context selection started the stopped Gateway"
start_cluster >/dev/null
default_context_use=$(run_tobari context use --name default --format json)
assert_contains "$default_context_use" '"cluster":"default_updated"' "running current Context change back"
default_context=$(run_tobari context show --format json)
assert_contains "$default_context" '"active":true' "Context selection after explicit recovery"
default_context_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["id"])' <<<"$default_context")

default_auth_import=$(printf '%s' "$synthetic_default_secret" | \
  run_tobari auth import "$synthetic_provider" --context default --format json)
restricted_auth_import=$(printf '%s' "$synthetic_restricted_secret" | \
  run_tobari auth import "$synthetic_provider" --context restricted --format json)
assert_contains "$default_auth_import" '"provider":"synthetic-ci"' \
  "default Context synthetic auth import"
assert_contains "$default_auth_import" '"configured":true' \
  "default Context synthetic auth import"
assert_contains "$restricted_auth_import" '"configured":true' \
  "restricted Context synthetic auth import"
if [[ $default_auth_import == *"$synthetic_default_secret"* || \
  $restricted_auth_import == *"$synthetic_restricted_secret"* ]]; then
  fail "auth import output exposed a synthetic real credential"
fi
default_auth_status=$(run_tobari auth status --context default --format json)
restricted_auth_status=$(run_tobari auth status --context restricted --format json)
assert_contains "$default_auth_status" '"provider":"synthetic-ci","state":"configured"' \
  "default Context auth status"
assert_contains "$restricted_auth_status" '"provider":"synthetic-ci","state":"configured"' \
  "restricted Context auth status"
container_work_root="/var/lib/tobari/${work_root#"$test_root/user/"}"
container_nested_root="/var/lib/tobari/${work_nested_root#"$test_root/user/"}"
enter_tobari_at "$work_root"
enter_tobari_at "$work_root" --context restricted
enter_ancestor_tobari_at "$work_nested_root"
create_nested_tobari_at "$other_root" --context restricted

status_from_nested=$(run_tobari_at "$work_nested_root" status --format json)
assert_contains "$status_from_nested" '"exists":true' "nested status"
assert_contains "$status_from_nested" "\"root\":\"$work_root\"" "nested status root"
nested_pwd=$({ printf '\r'; sleep 1; printf 'pwd\nexit\n'; } | run_tobari_pty_at "$work_nested_root" 2>&1)
assert_contains "$nested_pwd" "$container_nested_root" "nested host CWD mapping"
list_json=$(run_tobari_at "$work_root" list --format json)
work_id=$(id_for_root "$work_root" default <<<"$list_json")
restricted_id=$(id_for_root "$work_root" restricted <<<"$list_json")
other_id=$(id_for_root "$other_root" restricted <<<"$list_json")
work_container=$(container_for_id "$work_id")
restricted_container=$(container_for_id "$restricted_id")
work_network=$(network_for_id "$work_id")
restricted_network=$(network_for_id "$restricted_id")
other_network=$(network_for_id "$other_id")
other_container=$(container_for_id "$other_id")
enter_bash_tobari_at "$work_root"
[[ $(docker inspect --format '{{.State.Running}}' "$work_container") == true ]] ||
  fail "Workspace stopped after the interactive Bash child exited"
[[ $(docker inspect --format '{{json .Config.Cmd}}' "$work_container") == '["sleep","infinity"]' ]] ||
  fail "Workspace lifetime command was not sleep infinity after Bash exit"
[[ $work_id =~ ^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$ ]] ||
  fail "list did not return the project's stable ID"
[[ $work_id != "$restricted_id" && $work_id != "$other_id" && $restricted_id != "$other_id" ]] || fail "Context-bound Tobari received duplicate stable IDs"

# Re-entry reconciles the exact project-bound projection. Status must derive
# current/Ready from authoritative registry and Broker binding facts rather
# than from Context-wide provider configuration alone.
enter_tobari_at "$work_root"
ready_auth_status_before_noop=$(run_tobari auth status --context default --format json)
READY_AUTH_STATUS_DOCUMENT="$ready_auth_status_before_noop" python3 - "$work_id" <<'PY'
import json
import os
import sys

project_id = sys.argv[1]
document = json.loads(os.environ["READY_AUTH_STATUS_DOCUMENT"])
auth = document["auth"]
activation = auth["workspace_activation"]
if document.get("schema_version") != 1 or activation.get("coverage") != "exhaustive" or activation.get("state") != "ready":
    raise SystemExit(f"auth status did not report schema-1 exhaustive Ready: {document!r}")
workspace = next((item for item in activation.get("workspaces", []) if item.get("project_id") == project_id), None)
if workspace is None or workspace.get("state") != "ready" or workspace.get("next_action") is not None:
    raise SystemExit(f"auth status did not report the re-entered Workspace Ready: {workspace!r}")
providers = {item.get("provider"): item.get("state") for item in workspace.get("providers", [])}
if providers.get("synthetic-ci") != "current":
    raise SystemExit(f"auth status did not report the synthetic projection current: {providers!r}")
PY

# An installed but never-configured synthetic provider is the deterministic
# no-op target. The receipt cannot claim a mutation or Workspace action, and
# read-only status is byte-for-byte identical before and after.
synthetic_noop_logout=$(run_tobari auth logout "$synthetic_noop_provider" --context default --format json)
SYNTHETIC_NOOP_LOGOUT_DOCUMENT="$synthetic_noop_logout" python3 <<'PY'
import json
import os

document = json.loads(os.environ["SYNTHETIC_NOOP_LOGOUT_DOCUMENT"])
auth = document["auth"]
activation = auth["workspace_activation"]
if document.get("schema_version") != 1 or auth.get("provider") != "synthetic-noop" or auth.get("change") != "no_change":
    raise SystemExit(f"absent-provider logout did not report schema-1 no_change: {document!r}")
if activation.get("state") != "not_applicable" or activation.get("coverage") != "not_applicable" or activation.get("workspaces") != []:
    raise SystemExit(f"no-op logout invented Workspace activation: {activation!r}")
PY
ready_auth_status_after_noop=$(run_tobari auth status --context default --format json)
[[ $ready_auth_status_after_noop == "$ready_auth_status_before_noop" ]] ||
  fail "no-op logout changed the before/after auth status document"

python3 - "$config_directory/principal-registry/principals.json" "$work_id" "$restricted_id" "$other_id" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    document = json.load(handle)
bindings = document.get("bindings", [])
if document.get("schema_version") != 1 or len(bindings) != 3:
    raise SystemExit(f"unexpected project principal registry: {document!r}")
ids = {item["project_id"] for item in bindings}
if ids != set(sys.argv[2:]):
    raise SystemExit(f"registry project IDs {ids!r} do not match CWD projects")
addresses = [item["gateway_ip"] for item in bindings]
if len(addresses) != len(set(addresses)):
    raise SystemExit("project principal registry reused one Gateway address")
workspace_addresses = [item["workspace_ip"] for item in bindings]
if len(workspace_addresses) != len(set(workspace_addresses)):
    raise SystemExit("project principal registry reused one Workspace address")
if set(addresses) & set(workspace_addresses):
    raise SystemExit("project principal registry overlapped Workspace and Gateway endpoints")
if {item["context"] for item in bindings} != {"default", "restricted"}:
    raise SystemExit(f"registry Context bindings are incomplete: {bindings!r}")
PY

work_gateway_ip=$(python3 - "$config_directory/principal-registry/principals.json" "$work_id" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
print(next(item["gateway_ip"] for item in bindings if item["project_id"] == sys.argv[2]))
PY
)
work_workspace_ip=$(python3 - "$config_directory/principal-registry/principals.json" "$work_id" <<'PY'
import json
import sys
with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
print(next(item["workspace_ip"] for item in bindings if item["project_id"] == sys.argv[2]))
PY
)
[[ $(docker inspect --format "{{(index .NetworkSettings.Networks \"$work_network\").IPAddress}}" "$work_container") == "$work_workspace_ip" ]] ||
  fail "principal registry Workspace endpoint does not match Docker"
[[ $(docker inspect --format "{{(index .NetworkSettings.Networks \"$work_network\").IPAddress}}" tobari-gateway) == "$work_gateway_ip" ]] ||
  fail "principal registry Gateway endpoint does not match Docker"
[[ $(docker inspect --format '{{json .HostConfig.Dns}}' "$work_container") == "[\"$work_gateway_ip\"]" ]] ||
  fail "Workspace DNS is not fixed to its Gateway endpoint"
[[ $(docker inspect --format '{{index .HostConfig.Sysctls "net.ipv4.ip_forward"}}' tobari-gateway) == 0 ]] ||
  fail "Gateway IPv4 forwarding sysctl is not disabled"
[[ $(docker inspect --format '{{index .HostConfig.Sysctls "net.ipv6.conf.all.forwarding"}}' tobari-gateway) == 0 ]] ||
  fail "Gateway IPv6 forwarding sysctl is not disabled"
gateway_image_id=$(docker inspect --format '{{.Image}}' tobari-gateway)
gateway_guard_rules=$(docker run --rm --network container:tobari-gateway --user 0:0 --read-only \
  --cap-drop ALL --cap-add NET_ADMIN --security-opt no-new-privileges:true \
  --entrypoint nft "$gateway_image_id" list table inet tobari_guard_v1)
assert_contains "$gateway_guard_rules" "hook forward" "Gateway forward guard"
assert_contains "$gateway_guard_rules" "policy drop" "Gateway forward guard"
assert_contains "$gateway_guard_rules" "redirect to :15001" "Gateway transparent redirect"
[[ $gateway_guard_rules != *"dport 8080"* ]] || fail "Gateway guard retained explicit proxy port"
workspace_guard_rules=$(docker run --rm --network "container:$work_container" --user 0:0 --read-only \
  --cap-drop ALL --cap-add NET_ADMIN --security-opt no-new-privileges:true \
  --entrypoint nft "$gateway_image_id" list table inet tobari_guard_v1)
assert_contains "$workspace_guard_rules" "hook output" "Workspace output guard"
assert_contains "$workspace_guard_rules" "policy drop" "Workspace output guard"
assert_contains "$workspace_guard_rules" "reject with icmpv6 admin-prohibited" "Workspace IPv6 guard"
[[ $workspace_guard_rules != *"dport 8080"* ]] || fail "Workspace guard retained explicit proxy port"
workspace_default_route=$(docker run --rm --network "container:$work_container" --read-only \
  --cap-drop ALL --security-opt no-new-privileges:true --entrypoint ip "$gateway_image_id" -4 route show default)
assert_contains "$workspace_default_route" "default via $work_gateway_ip" "Workspace guarded default route"

work_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["status"]["home"])' <<<"$status_from_nested")
restricted_status=$(run_tobari_at "$work_root" status --context restricted --format json)
restricted_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["status"]["home"])' <<<"$restricted_status")
other_status=$(run_tobari_at "$other_root" status --context restricted --format json)
other_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["status"]["home"])' <<<"$other_status")
[[ $work_home != "$restricted_home" && $work_home != "$other_home" && $restricted_home != "$other_home" ]] || fail "Context-bound Tobari share a home directory"
printf 'shared-project-files\n' >"$work_root/context-sharing-canary"
assert_contains "$(run_restricted_project cat "$container_work_root/context-sharing-canary")" "shared-project-files" "same-root cross-Context project file sharing"

# Source access applies only to the selected live source bind. Both Contexts
# observe the same host tree, while Workspace home and tmpfs remain writable.
[[ $(docker inspect --format \
  "{{range .Mounts}}{{if eq .Destination \"$container_work_root\"}}{{.RW}}{{end}}{{end}}" \
  "$work_container") == true ]] || fail "read-write Context source bind is not writable"
[[ $(docker inspect --format \
  "{{range .Mounts}}{{if eq .Destination \"$container_work_root\"}}{{.RW}}{{end}}{{end}}" \
  "$restricted_container") == false ]] || fail "read-only Context source bind is writable"
if docker inspect --format '{{range .Mounts}}{{println .Destination .RW}}{{end}}' "$restricted_container" | \
  awk -v root="$container_work_root" '$1 == root && $2 == "true" {found=1} END {exit found ? 0 : 1}'; then
  fail "read-only Context exposes a writable alias for the selected source"
fi
assert_contains "$(run_restricted_project cat "$container_work_root/context-sharing-canary")" \
  "shared-project-files" "read-only Context source read"
for mutation in \
  "printf changed > '$container_work_root/context-sharing-canary'" \
  "printf created > '$container_work_root/read-only-create'" \
  "rm '$container_work_root/context-sharing-canary'" \
  "mv '$container_work_root/context-sharing-canary' '$container_work_root/read-only-rename'" \
  "chmod 0600 '$container_work_root/context-sharing-canary'" \
  "git -C '$container_work_root' init"; do
  if run_restricted_project sh -c "$mutation" >/dev/null 2>&1; then
    fail "read-only Context allowed source mutation: $mutation"
  fi
done
run_project sh -c "printf observed > '$container_work_root/read-write-observation'"
assert_contains "$(run_restricted_project cat "$container_work_root/read-write-observation")" \
  "observed" "read-only Context observation of read-write Context change"
printf 'host-observed\n' >"$work_root/host-observation"
assert_contains "$(run_restricted_project cat "$container_work_root/host-observation")" \
  "host-observed" "read-only Context observation of host change"
run_restricted_project sh -c 'printf home-write > /var/lib/tobari/source-access-home'
run_restricted_project sh -c 'printf tmp-write > /tmp/source-access-tmp'
assert_contains "$(run_restricted_project cat /var/lib/tobari/source-access-home)" \
  "home-write" "read-only Context writable home"
assert_contains "$(run_restricted_project cat /tmp/source-access-tmp)" \
  "tmp-write" "read-only Context writable tmpfs"

assert_resource_bounds "$work_container"
assert_resource_bounds "$restricted_container"
assert_resource_bounds "$other_container"
[[ $(run_project printenv HOME) == /var/lib/tobari ]] || fail "project HOME is not /var/lib/tobari"
for prohibited_proxy_variable in HTTP_PROXY HTTPS_PROXY http_proxy https_proxy NO_PROXY no_proxy; do
  if run_project printenv "$prohibited_proxy_variable" >/dev/null 2>&1; then
    fail "Workspace received prohibited proxy variable $prohibited_proxy_variable"
  fi
done
[[ $(run_project sh -c 'command -v gh') == /usr/local/bin/gh ]] || fail "GitHub CLI disappeared behind the project mount"
[[ $(run_project sh -c 'command -v aws') == /usr/local/bin/aws ]] || fail "AWS CLI disappeared behind the project mount"
work_auth_handle=$(run_project printenv SYNTHETIC_TOKEN)
restricted_auth_handle=$(run_restricted_project printenv SYNTHETIC_TOKEN)
other_auth_handle=$(run_other_project printenv SYNTHETIC_TOKEN)
for projected_handle in "$work_auth_handle" "$restricted_auth_handle" "$other_auth_handle"; do
  [[ $projected_handle =~ ^tobari-h1_[A-Za-z0-9_-]{43}$ ]] ||
    fail "Workspace did not receive one versioned opaque authentication handle"
done
[[ $work_auth_handle != "$restricted_auth_handle" ]] ||
  fail "same-root Workspaces in different Contexts received the same handle"
[[ $restricted_auth_handle != "$other_auth_handle" ]] ||
  fail "different projects in one Context received the same handle"
[[ $(docker inspect --format '{{len .NetworkSettings.Networks}}' tobari-auth-broker) == 1 ]] ||
  fail "static-only Auth Broker joined a network outside shared control"
for project_network in "$work_network" "$restricted_network" "$other_network"; do
  if network_contains_container "$project_network" tobari-auth-broker; then
    fail "Auth Broker joined Workspace network $project_network"
  fi
done
for credential_canary in \
  "$synthetic_default_secret" "$synthetic_restricted_secret"; do
  if docker inspect "$work_container" "$restricted_container" "$other_container" | \
    grep -F "$credential_canary" >/dev/null; then
    fail "Docker inspection exposed a synthetic real credential"
  fi
  for project_container in "$work_container" "$restricted_container" "$other_container"; do
    if docker exec "$project_container" sh -c \
      'env; find /var/lib/tobari -maxdepth 5 -type f -exec grep -a -H . {} + 2>/dev/null; ps -ef' | \
      grep -F "$credential_canary" >/dev/null; then
      fail "Workspace environment, home, or process state exposed a synthetic real credential"
    fi
  done
done

workspace_gh_token=$(docker exec \
  -e GH_TOKEN="$work_auth_handle" -e GH_HOST=github.com \
  "$work_container" gh auth token)
[[ $workspace_gh_token == "$work_auth_handle" ]] ||
  fail "gh auth token did not return only the opaque Workspace handle"
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
printf '{"theme":"light","local":true}\n' >"$work_home/.claude/settings.local.json"
chmod 0600 "$work_home/.claude/settings.local.json"
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
if docker exec "$restricted_container" test -e /var/lib/tobari/tool-auth-state; then
  fail "tool authentication state leaked across Contexts on the same root"
fi

start_cluster >/dev/null
enter_tobari_at "$work_root" &
first_enter_pid=$!
enter_tobari_at "$work_root" &
second_enter_pid=$!
wait "$first_enter_pid"
wait "$second_enter_pid"
owned_containers=$(docker ps -a --filter label=io.tobari.owner=default --format '{{.Names}}' | wc -l | tr -d ' ')
[[ $owned_containers == 6 ]] || fail "idempotent reconciliation left $owned_containers owned containers"

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
other_workspace_ip=$(docker inspect --format "{{(index .NetworkSettings.Networks \"$other_network\").IPAddress}}" "$other_container")
peer_resolution=$(run_project getent ahostsv4 "$other_container")
assert_contains "$peer_resolution" "198.18.0.10" "cross-project synthetic DNS isolation"
if [[ $peer_resolution == *"$other_workspace_ip"* ]]; then
  fail "one CWD-owned Tobari resolved another Tobari's real endpoint across dedicated networks"
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
  --user "$(id -u):$(id -g)" \
  --network tobari-egress \
  --network-alias mock-upstream \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --entrypoint python3 \
  -e TOBARI_MOCK_TLS_CERT=/tls/synthetic-ca.crt \
  -e TOBARI_MOCK_TLS_KEY=/tls/synthetic-server.key \
  -v "$PWD/test/integration/mock_upstream.py:/mock_upstream.py:ro" \
  -v "$test_root/tls:/tls:ro" \
  "$tobari_image" -u /mock_upstream.py >/dev/null
wait_listening "$mock_name" 8080
wait_network_connection tobari-gateway mock-upstream 8080
start_cluster >/dev/null
wait_healthy tobari-gateway
docker network create --internal --subnet 11.254.43.0/24 "$auth_network" >/dev/null
docker network connect "$auth_network" tobari-gateway
docker run -d \
  --name "$auth_mock_name" \
  --user "$(id -u):$(id -g)" \
  --network "$auth_network" \
  --network-alias api.synthetic.example \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  --entrypoint python3 \
  -e TOBARI_MOCK_PORT=443 \
  -e TOBARI_MOCK_TLS_CERT=/tls/synthetic-ca.crt \
  -e TOBARI_MOCK_TLS_KEY=/tls/synthetic-server.key \
  -v "$PWD/test/integration/mock_upstream.py:/mock_upstream.py:ro" \
  -v "$test_root/tls:/tls:ro" \
  "$tobari_image" -u /mock_upstream.py >/dev/null
wait_listening "$auth_mock_name" 443
wait_network_connection tobari-gateway api.synthetic.example 443

# Refresh the reconciliation-owned projections after the container/network
# recovery above. Stable credential revisions preserve each project handle.
work_auth_handle=$(run_project printenv SYNTHETIC_TOKEN)
restricted_auth_handle=$(run_restricted_project printenv SYNTHETIC_TOKEN)
other_auth_handle=$(run_other_project printenv SYNTHETIC_TOKEN)

default_vault="$test_root/state/tobari/auth/contexts/$default_context_id/vault.enc"
python3 - "$default_vault" "$(id -u)" <<'PY'
import os
import stat
import sys

info = os.lstat(sys.argv[1])
if not stat.S_ISREG(info.st_mode) or stat.S_ISLNK(info.st_mode):
    raise SystemExit("default Context vault is not a regular non-symlink file")
if info.st_uid != int(sys.argv[2]) or stat.S_IMODE(info.st_mode) != 0o600:
    raise SystemExit(
        f"default Context vault ownership/mode is {info.st_uid}:{stat.S_IMODE(info.st_mode):03o}"
    )
PY

# Introspection is deliberately non-secret. Making the encrypted vault
# unreadable proves that a policy denial completes without the resolve step,
# which would need to reopen and decrypt this file.
chmod 000 "$default_vault"
set +e
# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
default_broker_denial=$(run_project sh -c \
  'curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: $SYNTHETIC_TOKEN" https://api.synthetic.example/brokered-default')
default_broker_curl_status=$?
set -e
chmod 0600 "$default_vault"
[[ $default_broker_curl_status == 0 ]] ||
  fail "brokered denial request failed before receiving an HTTP decision"
default_broker_denial_status=${default_broker_denial##*$'\n'}
default_broker_denial_body=${default_broker_denial%$'\n'*}
[[ $default_broker_denial_status == 403 ]] ||
  fail "brokered request with an unreadable vault returned $default_broker_denial_status instead of policy denial"
assert_contains "$default_broker_denial_body" '"error":"policy_denied"' \
  "brokered deny-before-resolution response"
if docker logs "$auth_mock_name" 2>&1 | grep -F '"/brokered-default"' >/dev/null; then
  fail "policy-denied brokered request reached the synthetic upstream"
fi

broker_candidates=$(run_tobari policy candidates --tail 1000 --format json)
default_broker_candidate_id=$(candidate_id_for_effect \
  "$work_id" api.synthetic.example GET /brokered-default <<<"$broker_candidates")
default_broker_allow=$(run_tobari policy allow --id "$default_broker_candidate_id")
assert_contains "$default_broker_allow" 'Policy rule updated' \
  "default Context brokered policy approval"
assert_contains "$default_broker_allow" 'api.synthetic.example:443 GET /brokered-default' \
  "default Context brokered policy approval target"
# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
default_broker_response=$(run_project sh -c \
  'curl -fsS -H "X-Synthetic-Auth: $SYNTHETIC_TOKEN" https://api.synthetic.example/brokered-default')
default_broker_digest=$(printf 'Bearer %s' "$synthetic_default_secret" | shasum -a 256 | awk '{print $1}')
assert_contains "$default_broker_response" '"authorization_present":true' \
  "default Context brokered upstream response"
assert_contains "$default_broker_response" \
  "\"authorization_sha256\":\"$default_broker_digest\"" \
  "default Context brokered credential digest"
assert_contains "$default_broker_response" '"placeholder_present":false' \
  "default Context placeholder removal"

# A handle in body bytes is ordinary Workspace-controlled payload, never a
# broker selector. Keep the vault unreadable so an accidental post-policy
# resolve fails, while the already allowed effect must still stream upstream
# without either the placeholder or the real credential header.
chmod 000 "$default_vault"
set +e
body_only_handle_result=$(printf '%s' "$work_auth_handle" | \
  docker exec -i "$work_container" curl -sS -w $'\n%{http_code}' \
    -X GET -H 'content-type: application/octet-stream' --data-binary @- \
    https://api.synthetic.example/brokered-default)
body_only_handle_curl_status=$?
set -e
chmod 0600 "$default_vault"
[[ $body_only_handle_curl_status == 0 ]] ||
  fail "body-only handle request failed before receiving an upstream response"
body_only_handle_status=${body_only_handle_result##*$'\n'}
body_only_handle_response=${body_only_handle_result%$'\n'*}
[[ $body_only_handle_status == 200 ]] ||
  fail "body-only handle request returned $body_only_handle_status instead of 200"
assert_contains "$body_only_handle_response" '"authorization_present":false' \
  "body-only handle upstream response"
assert_contains "$body_only_handle_response" '"placeholder_present":false' \
  "body-only handle upstream response"
assert_contains "$body_only_handle_response" '"method":"GET"' \
  "body-only handle upstream method"
assert_contains "$body_only_handle_response" '"path":"/brokered-default"' \
  "body-only handle upstream path"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
restricted_broker_denial=$(run_restricted_project sh -c \
  'curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: $SYNTHETIC_TOKEN" https://api.synthetic.example/brokered-restricted')
restricted_broker_denial_status=${restricted_broker_denial##*$'\n'}
restricted_broker_denial_body=${restricted_broker_denial%$'\n'*}
[[ $restricted_broker_denial_status == 403 ]] ||
  fail "restricted Context brokered request returned $restricted_broker_denial_status instead of policy denial"
assert_contains "$restricted_broker_denial_body" '"error":"policy_denied"' \
  "restricted Context brokered denial"
if docker logs "$auth_mock_name" 2>&1 | grep -F '"/brokered-restricted"' >/dev/null; then
  fail "restricted policy-denied brokered request reached the synthetic upstream"
fi

broker_candidates=$(run_tobari policy candidates --tail 1000 --format json)
restricted_broker_candidate_id=$(candidate_id_for_effect \
  "$restricted_id" api.synthetic.example GET /brokered-restricted <<<"$broker_candidates")
restricted_broker_allow=$(run_tobari policy allow --id "$restricted_broker_candidate_id")
assert_contains "$restricted_broker_allow" 'Policy rule updated' \
  "restricted Context brokered policy approval"
assert_contains "$restricted_broker_allow" 'api.synthetic.example:443 GET /brokered-restricted' \
  "restricted Context brokered policy approval target"
# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
restricted_broker_response=$(run_restricted_project sh -c \
  'curl -fsS -H "X-Synthetic-Auth: $SYNTHETIC_TOKEN" https://api.synthetic.example/brokered-restricted')
restricted_broker_digest=$(printf 'Bearer %s' "$synthetic_restricted_secret" | shasum -a 256 | awk '{print $1}')
assert_contains "$restricted_broker_response" \
  "\"authorization_sha256\":\"$restricted_broker_digest\"" \
  "restricted Context brokered credential digest"
assert_contains "$restricted_broker_response" '"placeholder_present":false' \
  "restricted Context placeholder removal"
if [[ $restricted_broker_response == *"$default_broker_digest"* ]]; then
  fail "one shared Auth Broker crossed Context credential authority"
fi

copied_context_result=$(printf '%s\n' "$work_auth_handle" | \
  docker exec -i "$restricted_container" sh -c \
    'IFS= read -r copied; curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: $copied" https://api.synthetic.example/copied-context')
copied_context_status=${copied_context_result##*$'\n'}
[[ $copied_context_status == 403 ]] ||
  fail "handle copied across Contexts returned $copied_context_status instead of 403"
assert_contains "${copied_context_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "copied cross-Context handle rejection"

copied_project_result=$(printf '%s\n' "$restricted_auth_handle" | \
  docker exec -i "$other_container" sh -c \
    'IFS= read -r copied; curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: $copied" https://api.synthetic.example/copied-project')
copied_project_status=${copied_project_result##*$'\n'}
[[ $copied_project_status == 403 ]] ||
  fail "handle copied across projects returned $copied_project_status instead of 403"
assert_contains "${copied_project_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "copied cross-project handle rejection"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
wrong_header_result=$(run_project sh -c \
  'curl -sS -w "\n%{http_code}" -H "X-Wrong-Auth: $SYNTHETIC_TOKEN" https://api.synthetic.example/wrong-header')
wrong_header_status=${wrong_header_result##*$'\n'}
[[ $wrong_header_status == 403 ]] ||
  fail "handle in an unsupported header returned $wrong_header_status instead of 403"
assert_contains "${wrong_header_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "unsupported handle header rejection"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
wrong_format_result=$(run_project sh -c \
  'curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: Bearer $SYNTHETIC_TOKEN" https://api.synthetic.example/wrong-format')
wrong_format_status=${wrong_format_result##*$'\n'}
[[ $wrong_format_status == 403 ]] ||
  fail "handle in an unsupported format returned $wrong_format_status instead of 403"
assert_contains "${wrong_format_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "unsupported handle format rejection"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
embedded_header_result=$(run_project sh -c \
  'curl -sS -w "\n%{http_code}" -H "X-Wrong-Auth: prefix=$SYNTHETIC_TOKEN" https://api.synthetic.example/embedded-header')
embedded_header_status=${embedded_header_result##*$'\n'}
[[ $embedded_header_status == 403 ]] ||
  fail "handle embedded in an unsupported header returned $embedded_header_status instead of 403"
assert_contains "${embedded_header_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "embedded handle header rejection"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
cookie_handle_result=$(run_project sh -c \
  'curl -sS -w "\n%{http_code}" -H "Cookie: auth=$SYNTHETIC_TOKEN" https://api.synthetic.example/cookie-handle')
cookie_handle_status=${cookie_handle_result##*$'\n'}
[[ $cookie_handle_status == 403 ]] ||
  fail "handle in a cookie returned $cookie_handle_status instead of 403"
assert_contains "${cookie_handle_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "cookie handle rejection"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
query_handle_result=$(run_project sh -c \
  'curl -sS -w "\n%{http_code}" "https://api.synthetic.example/query-handle?auth=$SYNTHETIC_TOKEN"')
query_handle_status=${query_handle_result##*$'\n'}
[[ $query_handle_status == 403 ]] ||
  fail "handle in a query returned $query_handle_status instead of 403"
assert_contains "${query_handle_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "query handle rejection"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
path_handle_result=$(run_project sh -c \
  'curl -sS -w "\n%{http_code}" "https://api.synthetic.example/path-handle/$SYNTHETIC_TOKEN"')
path_handle_status=${path_handle_result##*$'\n'}
[[ $path_handle_status == 403 ]] ||
  fail "handle in a request path returned $path_handle_status instead of 403"
assert_contains "${path_handle_result%$'\n'*}" '"error":"credential_handle_invalid"' \
  "request-path handle rejection"

for rejected_path in \
  copied-context copied-project wrong-header wrong-format embedded-header \
  cookie-handle query-handle path-handle; do
  if docker logs "$auth_mock_name" 2>&1 | grep -F "\"/$rejected_path\"" >/dev/null; then
    fail "invalid broker handle request /$rejected_path reached the synthetic upstream"
  fi
done

wrong_port_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  https://mock-upstream:8081/wrong-port)
[[ $wrong_port_status == 403 ]] || fail "non-policy HTTP port returned $wrong_port_status instead of 403"
if docker logs "$mock_name" 2>&1 | grep -F '"/wrong-port"' >/dev/null; then
  fail "wrong-port request reached mock upstream"
fi

body_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST -H 'content-type: application/json' --data '{"value":true}' \
  https://mock-upstream:8080/body)
[[ $body_status == 200 ]] || fail "allowed nonempty-body request returned $body_status instead of 200"
docker logs "$mock_name" 2>&1 | grep -F '"/body"' >/dev/null ||
  fail "allowed nonempty-body request did not reach mock upstream"

upload_output="$test_root/stream-upload.out"
docker exec "$work_container" python3 -c '
import http.client
import time

connection = http.client.HTTPConnection("mock-upstream", 8080, timeout=10)
connection.putrequest("POST", "/stream-upload", skip_host=True)
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
  https://mock-upstream:8080/stream-response || true)
assert_contains "$stream_prefix" 'data: first' "streaming response prefix"
if [[ $stream_prefix == *'data: second'* ]]; then
  fail "streaming response completed before the upstream delay"
fi

oversized_body_status=$(dd if=/dev/zero bs=1048576 count=9 2>/dev/null | \
  docker exec -i "$work_container" curl -sS -o /dev/null -w '%{http_code}' \
    --max-time 15 -X POST -H 'content-type: application/octet-stream' \
    --data-binary @- https://mock-upstream:8080/oversized-body || true)
[[ $oversized_body_status == 413 ]] ||
  fail "oversized request returned $oversized_body_status instead of 413"
if docker logs "$mock_name" 2>&1 | grep -F '"/oversized-body"' >/dev/null; then
  fail "oversized request reached mock upstream"
fi

gateway_uid=$(docker exec tobari-gateway sh -c "awk '/^Uid:/{print \$2}' /proc/1/status")
gateway_gid=$(docker exec tobari-gateway sh -c "awk '/^Gid:/{print \$2}' /proc/1/status")
[[ $gateway_uid == "$(id -u)" ]] || fail "Gateway runs as uid $gateway_uid instead of the host uid"
[[ $gateway_gid == "$(id -g)" ]] || fail "Gateway runs as gid $gateway_gid instead of the host gid"

policy_bundle_mount=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/bundle"}}{{println .Name .RW}}{{end}}{{end}}' tobari-opa)
[[ $policy_bundle_mount == 'tobari-policy-bundle false' ]] ||
  fail "OPA does not mount the exact read-only policy bundle volume: $policy_bundle_mount"
[[ $(docker volume inspect --format '{{index .Labels "io.tobari.owner"}}' tobari-policy-bundle) == default ]] ||
  fail "policy bundle volume is missing its Tobari owner label"

expected_digest=$(printf 'Bearer %s' "$tool_auth_value" | shasum -a 256 | awk '{print $1}')
auth_response=$(run_project curl -fsS \
  -H "Authorization: Bearer $tool_auth_value" \
  https://mock-upstream:8080/credential)
assert_contains "$auth_response" '"authorization_present":true' "tool auth response"
assert_contains "$auth_response" "\"authorization_sha256\":\"$expected_digest\"" "tool auth digest"

deny_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST https://mock-upstream:8080/denied)
[[ $deny_status == 403 ]] || fail "denied method/path returned $deny_status instead of 403"
deny_body=$(run_project curl -sS -X POST https://mock-upstream:8080/denied)
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

graphql_canary=graphql-variable-canary
# The GraphQL `$value` references stay literal while only the quoted canary expands.
# shellcheck disable=SC2016
graphql_body='{"query":"mutation Change($value: String!) { closeIssue(input: {value: $value}) updateIssue(input: {value: $value}) }","variables":{"value":"'"$graphql_canary"'"}}'
graphql_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -H 'content-type: application/json' --data-binary "$graphql_body" \
  https://mock-upstream:8080/graphql)
[[ $graphql_status == 403 ]] ||
  fail "declared GraphQL roots returned $graphql_status instead of a learnable denial"
if docker logs "$mock_name" 2>&1 | grep -F '"/graphql"' >/dev/null; then
  fail "denied GraphQL request reached mock upstream"
fi
graphql_candidates=$(run_tobari policy candidates --tail 1000 --format json)
close_issue_candidate_id=$(graphql_candidate_id_for_effect \
  "$work_id" mock-upstream mutation closeIssue <<<"$graphql_candidates")
update_issue_candidate_id=$(graphql_candidate_id_for_effect \
  "$work_id" mock-upstream mutation updateIssue <<<"$graphql_candidates")
[[ $close_issue_candidate_id == pcy_* && $update_issue_candidate_id == pcy_* && \
  $close_issue_candidate_id != "$update_issue_candidate_id" ]] ||
  fail "GraphQL roots did not produce independent opaque candidates"
opa_before_graphql_policy=$(docker inspect --format '{{.Id}}' tobari-opa)
run_tobari policy allow --id "$close_issue_candidate_id" >/dev/null
graphql_partial_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -H 'content-type: application/json' --data-binary "$graphql_body" \
  https://mock-upstream:8080/graphql)
[[ $graphql_partial_status == 403 ]] ||
  fail "one GraphQL root approval authorized an unapproved sibling root"
run_tobari policy allow --id "$update_issue_candidate_id" >/dev/null
[[ $(docker inspect --format '{{.Id}}' tobari-opa) == "$opa_before_graphql_policy" ]] ||
  fail "routine GraphQL policy activation recreated OPA"
graphql_response=$(run_project curl -fsS \
  -H 'content-type: application/json' --data-binary "$graphql_body" \
  https://mock-upstream:8080/graphql)
graphql_body_sha=$(printf '%s' "$graphql_body" | shasum -a 256 | awk '{print $1}')
assert_contains "$graphql_response" '"path":"/graphql"' "GraphQL upstream response"
assert_contains "$graphql_response" "\"body_sha256\":\"$graphql_body_sha\"" \
  "unchanged GraphQL request body"
graphql_gateway_logs=$(run_tobari cluster logs --component gateway --tail 1000)
if [[ $graphql_gateway_logs == *"$graphql_canary"* || $graphql_gateway_logs == *'mutation Change'* ]]; then
  fail "GraphQL source or variables entered Gateway audit"
fi

review_allow_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow)
[[ $review_allow_status == 403 ]] || fail "review allow candidate returned $review_allow_status instead of 403"
restricted_review_allow_status=$(run_restricted_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow)
[[ $restricted_review_allow_status == 403 ]] || fail "restricted review candidate returned $restricted_review_allow_status instead of 403"
review_deny_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-deny)
[[ $review_deny_status == 403 ]] || fail "review deny candidate returned $review_deny_status instead of 403"
review_body_first_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'content-type: application/json' --data '{"value":"first"}' \
  https://mock-upstream:8080/review-body)
[[ $review_body_first_status == 403 ]] ||
  fail "first body-bearing review candidate returned $review_body_first_status instead of 403"
review_body_second_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'content-type: application/json' --data '{"value":"second"}' \
  https://mock-upstream:8080/review-body)
[[ $review_body_second_status == 403 ]] ||
  fail "second body-bearing review candidate returned $review_body_second_status instead of 403"
review_patch_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PATCH -H 'content-type: application/json' --data '{"value":"patch"}' \
  https://mock-upstream:8080/review-patch)
[[ $review_patch_status == 403 ]] ||
  fail "body-bearing PATCH candidate returned $review_patch_status instead of 403"
review_allow_body=$(run_project curl -sS -X PUT https://mock-upstream:8080/review-allow)
assert_contains "$review_allow_body" '"event":"permission_review_available"' "learnable denial response"
assert_contains "$review_allow_body" '"command":"tobari policy review"' "learnable denial response"
assert_contains "$review_allow_body" '"automatic_retry":false' "learnable denial response"
assert_contains "$review_allow_body" '"retry_after_review":true' "learnable denial response"
review_deny_body=$(run_project curl -sS -X PUT https://mock-upstream:8080/review-deny)
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
restricted_allow_candidate_id=$(candidate_id_for_effect "$restricted_id" mock-upstream PUT /review-allow <<<"$candidates_json")
deny_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream PUT /review-deny <<<"$candidates_json")
body_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream PUT /review-body <<<"$candidates_json")
patch_candidate_id=$(candidate_id_for_effect "$work_id" mock-upstream PATCH /review-patch <<<"$candidates_json")
[[ $allow_candidate_id == pcy_* && $restricted_allow_candidate_id == pcy_* && "$allow_candidate_id" != "$restricted_allow_candidate_id" && $deny_candidate_id == pcy_* && \
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
review_output=$(run_tobari policy review --tail 500)
assert_contains "$review_output" "restricted" "cross-Context permission Inbox"
assert_contains "$review_output" "$work_root" "same-root permission Inbox"
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

opa_before_exact_policy=$(docker inspect --format '{{.Id}}' tobari-opa)
allow_output=$(run_tobari policy allow --id "$allow_candidate_id")
assert_contains "$allow_output" "Policy rule updated" "exact policy approval"
assert_contains "$allow_output" 'mock-upstream:8080 PUT /review-allow' "exact policy approval target"

body_allow_output=$(run_tobari policy allow --id "$body_candidate_id")
assert_contains "$body_allow_output" 'Policy rule updated' "body-independent policy approval"
assert_contains "$body_allow_output" 'mock-upstream:8080 PUT /review-body' "body-independent policy approval target"
patch_deny_output=$(run_tobari policy deny --id "$patch_candidate_id")
assert_contains "$patch_deny_output" 'Permission denied' "body-bearing PATCH policy review"
assert_contains "$patch_deny_output" 'mock-upstream:8080 PATCH /review-patch' "body-bearing PATCH policy review target"
[[ $(docker inspect --format '{{.Id}}' tobari-opa) == "$opa_before_exact_policy" ]] ||
  fail "routine exact policy mutations recreated OPA"
body_applied_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'content-type: application/json' --data '{"value":"third"}' \
  https://mock-upstream:8080/review-body)
[[ $body_applied_status == 200 ]] ||
  fail "body-independent learned policy did not allow a new body value"

applied_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow)
[[ $applied_status == 200 ]] || fail "exact learned policy was not active after policy allow"
restricted_after_allow=$(run_restricted_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow)
[[ $restricted_after_allow == 403 ]] || fail "default Context learned Allow crossed into restricted with status $restricted_after_allow"
restricted_deny_output=$(run_tobari policy deny --id "$restricted_allow_candidate_id")
assert_contains "$restricted_deny_output" "Permission denied" "restricted exact Deny"
assert_contains "$restricted_deny_output" "mock-upstream:8080 PUT /review-allow" "restricted exact Deny target"
restricted_rules=$(run_tobari policy rules --format json)
restricted_deny_rule_id=$(python3 -c '
import json,sys
print(next(item["id"] for item in json.load(sys.stdin)["policy_rules"]
           if item["decision"] == "deny" and item["context"] == "restricted" and item["path"] == "/review-allow"))
' <<<"$restricted_rules")
run_tobari policy reset --id "$restricted_deny_rule_id" >/dev/null
[[ $(run_project curl -sS -o /dev/null -w '%{http_code}' -X PUT https://mock-upstream:8080/review-allow) == 200 ]] ||
  fail "restricted rule reset changed the default Context Allow"
[[ $(run_restricted_project curl -sS -o /dev/null -w '%{http_code}' -X PUT https://mock-upstream:8080/review-allow) == 403 ]] ||
  fail "restricted rule reset weakened the restricted Context default deny"
other_learned_status=$(run_other_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow)
[[ $other_learned_status == 403 ]] ||
  fail "learned policy crossed the project boundary with status $other_learned_status"
child_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow/child)
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
assert_contains "$reset_allow_output" 'Policy decision reset' "learned Allow reset"
assert_contains "$reset_allow_output" "$allow_rule_id" "learned Allow reset target"
assert_contains "$reset_allow_output" 'Removed' "learned Allow reset decision"
assert_contains "$reset_allow_output" 'allow' "learned Allow reset decision"
assert_contains "$reset_allow_output" 'Default deny' "learned Allow reset outcome"
assert_contains "$reset_allow_output" 'yes' "learned Allow reset outcome"
reset_allow_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow)
[[ $reset_allow_status == 403 ]] || fail "reset Allow did not return the request to default deny"
review_after_allow_reset=$(run_tobari policy review --tail 1000 --format json)
assert_contains "$review_after_allow_reset" "\"id\":\"$allow_candidate_id\"" \
  "reset Allow re-review queue"
allow_output=$(run_tobari policy allow --id "$allow_candidate_id")
assert_contains "$allow_output" 'Policy rule updated' "re-allow after reset"
reallowed_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-allow)
[[ $reallowed_status == 200 ]] || fail "re-review could not restore the exact Allow"

deny_output=$(run_tobari policy deny --id "$deny_candidate_id")
assert_contains "$deny_output" 'Permission denied' "exact policy rejection"
assert_contains "$deny_output" 'mock-upstream:8080 PUT /review-deny' "exact policy rejection target"
assert_contains "$deny_output" 'Applied' "exact policy rejection outcome"
assert_contains "$deny_output" 'yes' "exact policy rejection outcome"
rejected_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-deny)
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
assert_contains "$reset_deny_output" 'Policy decision reset' "learned Deny reset"
assert_contains "$reset_deny_output" "$deny_rule_id" "learned Deny reset target"
assert_contains "$reset_deny_output" 'Removed' "learned Deny reset decision"
assert_contains "$reset_deny_output" 'deny' "learned Deny reset decision"
assert_contains "$reset_deny_output" 'Default deny' "learned Deny reset outcome"
assert_contains "$reset_deny_output" 'yes' "learned Deny reset outcome"
reset_deny_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-deny)
[[ $reset_deny_status == 403 ]] || fail "reset Deny weakened default denial"
review_after_deny_reset=$(run_tobari policy review --tail 1000 --format json)
assert_contains "$review_after_deny_reset" "\"id\":\"$deny_candidate_id\"" \
  "reset Deny re-review queue"
deny_output=$(run_tobari policy deny --id "$deny_candidate_id")
assert_contains "$deny_output" 'Permission denied' "re-deny after reset"

reject_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/rejected)
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
assert_contains "$deny_output" 'Permission denied' "exact policy rejection"
assert_contains "$deny_output" 'mock-upstream:8080 PUT /rejected' "exact policy rejection target"
rejected_after_deny=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/rejected)
[[ $rejected_after_deny == 403 ]] || fail "exact policy rejection changed the denied request to $rejected_after_deny"
remaining_review=$(run_tobari policy review --tail 1000 --format json)
if [[ $remaining_review == *"$reject_candidate_id"* ]]; then
  fail "denied candidate remained in the review queue"
fi

interactive_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-interactive)
[[ $interactive_status == 403 ]] || fail "interactive review candidate returned $interactive_status instead of 403"
interactive_index=
for _ in $(seq 1 30); do
  interactive_review=$(run_tobari policy review --tail 1000 --format json)
  interactive_index=$(python3 -c '
import json
import sys

items = json.load(sys.stdin)["policy_review"]
print(next((index for index, item in enumerate(items, 1) if item["path"] == "/review-interactive"), ""))
' <<<"$interactive_review")
  [[ -n $interactive_index ]] && break
  sleep 0.2
done
[[ -n $interactive_index ]] || fail "interactive review candidate did not reach the review queue"
interactive_candidate_id=$(python3 -c '
import json
import sys

items = json.load(sys.stdin)["policy_review"]
print(next(item["id"] for item in items if item["path"] == "/review-interactive"))
' <<<"$interactive_review")
interactive_workspace_before=$(docker inspect --format '{{.Id}}' "$work_container")
interactive_opa_before=$(docker inspect --format '{{.Id}}' tobari-opa)
interactive_events=$(python3 -c '
import json
import sys

index = sys.argv[1]
selection = "\x1b[B" * (int(index) - 1) + "\r"
print(json.dumps([
    {"after_ms": 5000, "data": selection},
    {"after_ms": 750, "data": "a"},
    {"after_ms": 750, "data": "p"},
    {"after_ms": 750, "data": "y"},
]))
' "$interactive_index")
if ! interactive_output=$(TOBARI_TEST_PTY_TIMEOUT_SECONDS=15 \
  TOBARI_TEST_PTY_EVENTS="$interactive_events" \
  run_tobari_pty_at "$work_root" policy review --tail 1000 2>&1); then
  printf '%s\n' "$interactive_output" >&2
  fail "interactive policy review PTY session failed"
fi
if [[ $interactive_output != *'Reviewed permissions applied'* || \
  $interactive_output != *'1 (1 Allow, 0 Deny)'* ]]; then
  printf '%s\n' "$interactive_output" >&2
  fail "interactive policy review did not contain the expected value"
fi
assert_contains "$interactive_output" "Context" "interactive permission Context detail and confirmation"
assert_contains "$interactive_output" "Tobari" "interactive permission Tobari detail and confirmation"
assert_contains "$interactive_output" "/review-interactive" "interactive permission request detail and confirmation"
assert_contains "$interactive_output" "$default_context_id" "interactive permission exact Context receipt"
assert_contains "$interactive_output" "$work_id" "interactive permission exact project receipt"
assert_contains "$interactive_output" "$interactive_candidate_id" "interactive permission exact candidate receipt"
interactive_cluster_status=$(run_tobari cluster status --format json)
interactive_revision=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["cluster"]["policy_revision"])' \
  <<<"$interactive_cluster_status")
[[ $interactive_revision =~ ^[0-9a-f]{64}$ ]] || fail "interactive review did not activate one typed policy revision"
assert_contains "$interactive_output" "$interactive_revision" "interactive permission active revision receipt"
[[ $(docker inspect --format '{{.Id}}' "$work_container") == "$interactive_workspace_before" ]] ||
  fail "interactive policy review replaced the running Workspace"
[[ $(docker inspect --format '{{.Id}}' tobari-opa) == "$interactive_opa_before" ]] ||
  fail "interactive policy review replaced OPA"
interactive_retry_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review-interactive)
[[ $interactive_retry_status == 200 ]] ||
  fail "same running Workspace could not retry the reviewed request: $interactive_retry_status"
interactive_review=$(run_tobari policy review --tail 1000 --format json)
if python3 -c 'import json,sys; sys.exit(0 if any(item["path"] == "/review-interactive" for item in json.load(sys.stdin)["policy_review"]) else 1)' <<<"$interactive_review"; then
  fail "interactive Allow did not remove the candidate from the review queue"
fi

for item_path in one two three; do
  item_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
    -X PUT "https://mock-upstream:8080/review/items/$item_path")
  [[ $item_status == 403 ]] || fail "exact-rule source $item_path was not initially denied"
done

for item_path in one two three; do
  candidates_json=$(run_tobari policy candidates --tail 1000 --format json)
  item_candidate_id=$(candidate_id_for_effect \
    "$work_id" mock-upstream PUT "/review/items/$item_path" <<<"$candidates_json")
  item_allow_output=$(run_tobari policy allow --id "$item_candidate_id")
  assert_contains "$item_allow_output" "Policy rule updated" \
    "exact source approval"
  assert_contains "$item_allow_output" "mock-upstream:8080 PUT /review/items/$item_path" \
    "exact source approval"
done

for item_path in one two three; do
  item_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
    -X PUT "https://mock-upstream:8080/review/items/$item_path")
  [[ $item_status == 200 ]] || fail "exact source rule $item_path was not active"
done

unseen_sibling_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review/items/four)
[[ $unseen_sibling_status == 403 ]] || fail "exact rules widened to an unseen sibling path"
outside_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT https://mock-upstream:8080/review/items-outside-tobari-canary)
[[ $outside_status == 403 ]] || fail "exact rules widened outside their directory boundary"
if run_tobari help policy compactions --format agent >/dev/null 2>&1 ||
  run_tobari help policy compact --format agent >/dev/null 2>&1; then
  fail "retired policy compaction command remained invocable"
fi

final_default_auth_status=$(run_tobari auth status --context default --format json)
final_restricted_auth_status=$(run_tobari auth status --context restricted --format json)
final_context_status=$(run_tobari context show --format json)
final_cluster_status=$(run_tobari cluster status --format json)
CLUSTER_STATUS_DOCUMENT="$final_cluster_status" python3 <<'PY'
import json
import os

document = json.loads(os.environ["CLUSTER_STATUS_DOCUMENT"])
if document.get("schema_version") != 1 or "proxy" in document.get("cluster", {}):
    raise SystemExit(f"cluster status retained explicit proxy contract: {document!r}")
PY
assert_contains "$final_cluster_status" '"auth_broker_state":"ready"' \
  "shared Auth Broker readiness status"
if [[ $final_cluster_status == *"credential_companion_state"* ]]; then
  fail "cluster status retained the retired credential companion state"
fi
final_doctor_status=$(run_tobari doctor --root "$work_root" --format json)
final_gateway_logs=$(run_tobari cluster logs --component gateway --tail 1000)
final_broker_logs=$(run_tobari cluster logs --component auth-broker --tail 1000)
final_opa_logs=$(run_tobari cluster logs --component opa --tail 1000)
final_policy_diagnostics=$(run_tobari policy candidates --tail 1000 --format json)
final_mock_logs=$(docker logs "$mock_name" 2>&1)
final_auth_mock_logs=$(docker logs "$auth_mock_name" 2>&1)
diagnostic_surface=$(printf '%s\n' \
	"$default_auth_import" "$restricted_auth_import" \
	"$ready_auth_status_before_noop" "$synthetic_noop_logout" "$ready_auth_status_after_noop" \
	"$final_default_auth_status" "$final_restricted_auth_status" \
  "$final_context_status" "$final_cluster_status" "$final_doctor_status" \
  "$final_gateway_logs" "$final_broker_logs" "$final_opa_logs" \
  "$final_policy_diagnostics" "$final_mock_logs" "$final_auth_mock_logs")
for authentication_canary in \
  "$synthetic_default_secret" "$synthetic_restricted_secret" \
  "$work_auth_handle" "$restricted_auth_handle" "$other_auth_handle"; do
  if [[ $diagnostic_surface == *"$authentication_canary"* ]]; then
    fail "CLI, Gateway, Broker, OPA, policy, or upstream diagnostics exposed authentication material"
  fi
  if grep -R -a -F -- "$authentication_canary" "$test_root/state/tobari" \
    >/dev/null 2>&1; then
    fail "host machine state stored authentication material outside the encrypted vault contract"
  fi
done

shell_output=$(printf 'printf shell-ok\\nexit\\n' | run_project_shell)
assert_contains "$shell_output" "shell-ok" "interactive shell"

transparent_dns=$(run_project getent ahostsv4 transparent.invalid)
assert_contains "$transparent_dns" "198.18.0.10" "synthetic non-recursive DNS"
if run_project curl --noproxy '*' --max-time 3 -fsS \
  http://opa:8181/health >/dev/null 2>&1; then
  fail "Tobari reached the OPA control API"
fi

docker stop tobari-opa >/dev/null
opa_down_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  https://mock-upstream:8080/opa-down)
[[ $opa_down_status == 503 ]] || fail "OPA outage returned $opa_down_status instead of 503"
docker start tobari-opa >/dev/null
wait_healthy tobari-opa

docker stop tobari-gateway >/dev/null
if run_project curl --max-time 3 -fsS \
  https://mock-upstream:8080/gateway-down >/dev/null 2>&1; then
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
set +e
run_tobari cluster down >/dev/null 2>&1
down_with_projects_status=$?
set -e
[[ $down_with_projects_status != 0 ]] || fail "cluster down succeeded while Context-bound Tobari remained"
[[ $(docker inspect --format '{{.State.Running}}' tobari-gateway) == true ]] || fail "refused cluster down stopped the shared Gateway"
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
python3 - "$config_directory/principal-registry/principals.json" "$restricted_id" "$other_id" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
if {item["project_id"] for item in bindings} != set(sys.argv[2:]):
    raise SystemExit(f"deleted project principal was not removed: {bindings!r}")
PY
run_tobari_at "$work_root" delete --context restricted --force >/dev/null
restricted_id=
restricted_container=
run_tobari_at "$other_root" delete --context restricted --force >/dev/null
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
