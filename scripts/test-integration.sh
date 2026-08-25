#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "$0")/.."
source test/integration/workspace_service_exposure.sh
source test/integration/gateway_fixture.sh
source test/integration/runtime_image_cleanup.sh
source test/integration/permission_resume.sh
binary=${TOBARI_INTEGRATION_BINARY:-}
binary_digest=
custom_base_image=${TOBARI_INTEGRATION_CUSTOM_BASE:-tobari-runtime:dev}
host_loopback_only=${TOBARI_INTEGRATION_HOST_LOOPBACK_ONLY:-false}
mock_name=tobari-mock-upstream
auth_mock_name=tobari-auth-mock-upstream
auth_network=tobari-auth-integration
runtime_name=integration
gateway_base_image="tobari-gateway-integration-base-$$"
experimental_gateway_base_image="tobari-gateway-integration-experimental-base-$$"
gateway_dev_tag='' gateway_previous_image_id='' gateway_fixture_image_id=''
gateway_fixture_image="tobari-gateway-integration-tls-$$"
gateway_fixture_tag_installed=false
test_keychain_service=
test_root=
work_root=
other_root=
work_id=
other_id=
restricted_id=
work_ref=
other_ref=
restricted_ref=
default_context_ref=
restricted_context_ref=
other_context_ref=
default_context_id=
restricted_context_id=
other_context_id=
work_container=
other_container=
restricted_container=
runtime_image=
runtime_image_id=
runtime_id=
runtime_source_digest=
official_runtime_image=
created_dev_runtime_tag=false
owns_shared_fixture=false
current_phase=
phase_started=$SECONDS
host_service_server_pid=
host_service_attachment_pid=
host_docker_config=${DOCKER_CONFIG:-$HOME/.docker}
host_docker_context=${TOBARI_INTEGRATION_DOCKER_CONTEXT:-${DOCKER_CONTEXT:-}}
docker() {
  [[ -n ${host_docker_context:-} && $host_docker_context != default ]] || { echo "integration: explicit non-default Docker context is required" >&2; return 1; }
  command docker --context "$host_docker_context" "$@"
}
begin_phase() {
  local next_phase=$1
  if [[ -n $current_phase ]]; then
    echo "integration phase: $current_phase OK ($((SECONDS - phase_started))s)" >&2
  fi
  current_phase=$next_phase
  phase_started=$SECONDS
  echo "integration phase: $current_phase START" >&2
}

complete_phase() {
  if [[ -n $current_phase ]]; then
    echo "integration phase: $current_phase OK ($((SECONDS - phase_started))s)" >&2
  fi
  current_phase=
}
fail() {
  echo "integration: phase=${current_phase:-setup}: $*" >&2
  exit 1
}

report_unexpected_failure() {
  local status=$?
  if [[ $- == *e* ]]; then
    echo "integration: phase=${current_phase:-setup} elapsed=$((SECONDS - phase_started))s: unexpected command failure near lines ${BASH_LINENO[*]}" >&2
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
  assert_integration_binary
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
  assert_integration_binary
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

assert_integration_binary() {
  local current_digest
  current_digest=$(shasum -a 256 "$binary" | awk '{print $1}')
  [[ -n $binary_digest && $current_digest == "$binary_digest" ]] ||
    fail "integration binary changed while the scenario was running"
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

workspace_field_for_context() {
  local field=$1
  local context_id=$2
  python3 -c \
    'import json,sys
field,context_id=sys.argv[1:]
print(next(item[field] for item in json.load(sys.stdin)["workspaces"]["items"]
           if item["context_id"] == context_id))' \
    "$field" "$context_id"
}

run_project() {
  docker exec "$work_container" "$@"
}

wait_project_broker_policy_denial() {
  local url=$1
  local consecutive=0
  local status
  local _
  for _ in $(seq 1 60); do
    # Expansion is intentionally deferred to the isolated `sh -c` process.
    # shellcheck disable=SC2016
    status=$(run_project sh -c \
      'curl -sS -o /dev/null -w "%{http_code}" -H "X-Synthetic-Auth: $SYNTHETIC_TOKEN" "$1"' \
      sh "$url" 2>/dev/null || true)
    if [[ $status == 403 ]]; then
      consecutive=$((consecutive + 1))
      if [[ $consecutive == 3 ]]; then
        return 0
      fi
    else
      consecutive=0
    fi
    sleep 0.2
  done
  fail "policy did not settle on brokered default denial for $url"
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
  local output
  if output=$(TOBARI_INTEGRATION_FAULT_DIAGNOSTICS=true run_tobari cluster up 2>&1); then
    printf '%s\n' "$output"
    return 0
  fi
  printf '%s\n' "$output" >&2
  if [[ $host_loopback_only != true ]] && docker inspect tobari-auth-broker >/dev/null 2>&1; then
    echo "integration diagnostics: Auth Broker companion status" >&2
    docker exec tobari-auth-broker python3 -m authbroker.control companion_status 2>&1 |
      sed -E 's/"epoch_id":"[^"]*"/"epoch_id":"[redacted]"/g' >&2 || true
    echo "integration diagnostics: Auth Broker processes" >&2
    docker top tobari-auth-broker -eo pid,args >&2 || true
  fi
  return 1
}

run_final_host_loopback_evaluator() {
  begin_phase final-host-loopback-evaluator
  work_root=$test_root/user/workspace
  mkdir -p "$work_root"

  run_tobari template create --name default --source-access read-write --format json >/dev/null
  local template_list template_ref context_create context_ref context_id
  template_list=$(run_tobari template list --format json)
  template_ref=$(python3 -c 'import json,sys; print(next(item["template_ref"] for item in json.load(sys.stdin)["templates"]["items"] if item["name"] == sys.argv[1]))' default <<<"$template_list")
  context_create=$(run_tobari_at "$work_root" context create --template "$template_ref" --format json)
  context_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_ref"])' <<<"$context_create")
  context_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_id"])' <<<"$context_create")
  start_cluster >/dev/null

  local host_service_port_file host_service_request_log host_service_port
  host_service_port_file=$test_root/host-service.port
  host_service_request_log=$test_root/host-service.requests.jsonl
  python3 - "$host_service_port_file" "$host_service_request_log" <<'PY' &
import http.server
import json
import pathlib
import socketserver
import sys

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        with open(sys.argv[2], "a", encoding="utf-8") as handle:
            handle.write(json.dumps({"host": self.headers.get("Host"), "path": self.path}) + "\n")
        body = b"host-service-ok\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format, *_args):
        pass

with socketserver.TCPServer(("127.0.0.1", 0), Handler) as server:
    pathlib.Path(sys.argv[1]).write_text(str(server.server_address[1]), encoding="ascii")
    server.serve_forever()
PY
  host_service_server_pid=$!
  for _ in $(seq 1 60); do
    [[ -s $host_service_port_file ]] && break
    sleep 0.1
  done
  [[ -s $host_service_port_file ]] || fail "physical-host HTTP fixture did not publish its port"
  host_service_port=$(<"$host_service_port_file")

  local gateway_ca_before gateway_cert_files_before
  local host_loopback_hostname=host.tobari.internal
  local retired_host_loopback_hostname=host.tobari.test
  local sibling_host_loopback_hostname=sibling.tobari.internal
  gateway_ca_before=$(docker exec tobari-gateway sha256sum /var/lib/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem | awk '{print $1}')
  gateway_cert_files_before=$(docker exec tobari-gateway sh -c 'find /var/lib/mitmproxy/.mitmproxy -type f -print | sort')

  # Expansion is intentionally deferred to the attached Workspace shell.
  # shellcheck disable=SC2016
  run_tobari_at "$work_root" context enter --id "$context_ref" -- /bin/bash -lc \
    'port=$1; host=$2; retired=$3; { printf "%s\n" "$TOBARI_CAPABILITIES_JSON"; curl -sS -o /dev/null -w "%{http_code}" "http://${host}:${port}/health"; printf "\n"; curl -sS -o /dev/null -w "%{http_code}" "http://${retired}:${port}/health"; printf "\n"; curl -ksS --connect-timeout 5 "https://${host}:${port}/health" >/dev/null 2>&1; printf "%s\n" "$?"; curl -ksS --connect-timeout 5 "https://${retired}:${port}/health" >/dev/null 2>&1; printf "%s\n" "$?"; python3 -c "import socket,sys; print(socket.gethostbyname(sys.argv[1])); print(socket.gethostbyname(sys.argv[2]))" "$host" "$retired"; } > /var/lib/tobari/host-probe.tmp; mv /var/lib/tobari/host-probe.tmp /var/lib/tobari/host-probe; while [[ ! -e /var/lib/tobari/host-probe.done ]]; do sleep 0.1; done' \
    bash "$host_service_port" "$host_loopback_hostname" "$retired_host_loopback_hostname" \
    >"$test_root/host-attachment.out" 2>&1 &
  host_service_attachment_pid=$!

  local workspace_list workspace_ref workspace_id host_probe
  for _ in $(seq 1 900); do
    work_container=$(docker ps --filter label=io.tobari.owner=default --filter label=io.tobari.role=work --format '{{.Names}}' | sed -n '1p')
    if [[ -n $work_container ]]; then
      workspace_id=$(docker inspect --format '{{index .Config.Labels "io.tobari.id"}}' "$work_container")
      [[ -n $workspace_id ]] && break
    fi
    sleep 0.1
  done
  [[ -n $workspace_id ]] || fail "final Context entry did not materialize its exact Workspace container"
  work_id=$workspace_id
  for _ in $(seq 1 900); do
    if run_project test -s /var/lib/tobari/host-probe >/dev/null 2>&1; then
      break
    fi
    sleep 0.1
  done
  run_project test -s /var/lib/tobari/host-probe >/dev/null 2>&1 || fail "final attachment did not publish Host Loopback probe"
  host_probe=$(run_project cat /var/lib/tobari/host-probe)

  HOST_CAPABILITIES=$(sed -n '1p' <<<"$host_probe") python3 - <<'PY'
import json
import os
document = json.loads(os.environ["HOST_CAPABILITIES"])
host_http = document.get("host_http", {})
if document.get("schema_version") != 1 or host_http != {
    "url_template": "http://host.tobari.internal:{port}",
    "minimum_port": 1024,
    "maximum_port": 65535,
    "lifetime": "attachment",
    "audience": "workspace",
    "access": "policy_review_required",
}:
    raise SystemExit(f"invalid public Host Loopback capability: {document!r}")
PY
  [[ $(sed -n '2p' <<<"$host_probe") == 403 ]] || fail "current unreviewed Host Loopback was not denied"
  [[ $(sed -n '3p' <<<"$host_probe") == 410 ]] || fail "retired Host Loopback was not terminally denied"
  [[ $(sed -n '4p' <<<"$host_probe") != 0 && $(sed -n '5p' <<<"$host_probe") != 0 ]] || fail "Host Loopback TLS did not close"
  [[ $(sed -n '6p' <<<"$host_probe") == 198.18.0.10 && $(sed -n '7p' <<<"$host_probe") == 198.18.0.10 ]] || fail "Host Loopback bypassed synthetic DNS"
  [[ ! -s $host_service_request_log ]] || fail "denied Host Loopback traffic reached physical loopback"
  [[ $(docker exec tobari-gateway sha256sum /var/lib/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem | awk '{print $1}') == "$gateway_ca_before" ]] || fail "Host Loopback TLS rotated the Gateway CA"
  [[ $(docker exec tobari-gateway sh -c 'find /var/lib/mitmproxy/.mitmproxy -type f -print | sort') == "$gateway_cert_files_before" ]] || fail "Host Loopback TLS changed the persistent certificate store"

  local candidates candidate_ref
  for _ in $(seq 1 60); do
    candidates=$(run_tobari policy candidates --format json)
    candidate_ref=$(python3 -c '
import json,sys
items=json.load(sys.stdin)["policy_candidates"]
port=int(sys.argv[1])
print(next((item["id"] for item in items if item["effect"]["host"] == "host.tobari.internal" and item["effect"]["port"] == port and item["effect"]["path"] == "/health"), ""))
' "$host_service_port" <<<"$candidates")
    [[ -n $candidate_ref ]] && break
    sleep 0.2
  done
  [[ -n $candidate_ref ]] || fail "Host Loopback denial did not create an exact final candidate"
  run_tobari policy allow --id "$candidate_ref" >/dev/null

  python3 - "$config_directory/host-loopback/routes.json" "$config_directory/host-loopback/grants.json" "$context_id" "$workspace_id" "$host_service_port" <<'PY'
import hashlib
import json
import sys
routes_doc=json.load(open(sys.argv[1], encoding="utf-8"))
grants_doc=json.load(open(sys.argv[2], encoding="utf-8"))
if routes_doc.get("schema_version") != 2 or grants_doc.get("schema_version") != 2:
    raise SystemExit("private Host Loopback registries are not schema V2")
route=next(item for item in routes_doc["routes"] if item["project_id"] == sys.argv[4])
material="\0".join(("tobari-host-loopback-route-v2",route["attachment_epoch_id"],route["context_id"],route["project_id"],route["hostname"]))
if route["context_id"] != sys.argv[3] or route["hostname"] != "host.tobari.internal" or route["id"] != "hlr_"+hashlib.sha256(material.encode()).hexdigest()[:32]:
    raise SystemExit(f"route identity drift: {route!r}")
grant=next(item for item in grants_doc["grants"] if item["project_id"] == sys.argv[4] and item["target_port"] == int(sys.argv[5]))
if grant["context_id"] != sys.argv[3] or grant["host"] != route["hostname"] or grant["attachment_epoch_id"] != route["attachment_epoch_id"] or grant["lifetime"] != "attachment":
    raise SystemExit(f"grant identity drift: {grant!r}")
PY

  assert_contains "$(run_project curl -fsS "http://$host_loopback_hostname:$host_service_port/health")" "host-service-ok" "curl Host Loopback"
  assert_contains "$(run_project python3 -c 'import sys,urllib.request; print(urllib.request.urlopen(sys.argv[1],timeout=5).read().decode())' "http://$host_loopback_hostname:$host_service_port/health")" "host-service-ok" "Python Host Loopback"
  [[ $(run_project curl -sS -o /dev/null -w '%{http_code}' "http://$sibling_host_loopback_hostname:$host_service_port/health") == 403 ]] || fail "sibling .internal borrowed Host Loopback authority"
  python3 - "$host_service_request_log" "$host_service_port" <<'PY'
import json
import sys
entries=[json.loads(line) for line in open(sys.argv[1], encoding="utf-8")]
allowed={"host.tobari.internal",f"host.tobari.internal:{sys.argv[2]}"}
if len(entries) != 2 or any(item != {"host": item["host"], "path": "/health"} or item["host"] not in allowed for item in entries):
    raise SystemExit(f"Host header changed or terminal traffic reached the fixture: {entries!r}")
PY
  if grep -F 'host.tobari.internal' "$test_root/state/tobari/workspace-authority/authority.json" >/dev/null; then
    fail "Host Loopback attachment grant entered Context Policy Memory"
  fi

  run_project touch /var/lib/tobari/host-probe.done
  wait "$host_service_attachment_pid"
  host_service_attachment_pid=
  workspace_list=$(run_tobari workspace list --format json)
  workspace_ref=$(python3 -c 'import json,sys; d=json.load(sys.stdin); print(next(item["workspace_ref"] for item in d["workspaces"]["items"] if item["workspace_id"] == sys.argv[1]))' "$workspace_id" <<<"$workspace_list")
  [[ $(run_project curl -sS -o /dev/null -w '%{http_code}' "http://$host_loopback_hostname:$host_service_port/health") == 403 ]] || fail "Host Loopback authority survived attachment teardown"
  python3 - "$config_directory/host-loopback/routes.json" "$config_directory/host-loopback/grants.json" <<'PY'
import json
import sys
routes=json.load(open(sys.argv[1], encoding="utf-8"))
grants=json.load(open(sys.argv[2], encoding="utf-8"))
if routes.get("schema_version") != 2 or grants.get("schema_version") != 2 or routes["routes"] or grants["grants"]:
    raise SystemExit(f"attachment authority survived teardown: {routes!r} {grants!r}")
PY

  kill "$host_service_server_pid" >/dev/null 2>&1 || true
  wait "$host_service_server_pid" >/dev/null 2>&1 || true
  host_service_server_pid=
  run_tobari workspace delete --id "$workspace_ref" --confirm=delete --force >/dev/null
  work_container=
  run_tobari_at "$work_root" context delete --id "$context_ref" --confirm=delete >/dev/null
  run_tobari cluster down >/dev/null
  complete_phase
  echo "Host Loopback isolated integration: OK"
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
	local workspace_id=$1
	local scheme=$2
	local host=$3
	local port=$4
	local method=$5
	local path=$6
  python3 -c \
    'import json,sys
workspace_id,scheme,host,port,method,path=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["policy_candidates"]
           if item.get("workspace_id") == workspace_id and item.get("scheme") == scheme
           and item.get("host") == host and item.get("port") == int(port)
           and item.get("method") == method and item.get("path") == path))' \
    "$workspace_id" "$scheme" "$host" "$port" "$method" "$path"
}

allow_exact_effect() {
  local project_id=$1
  local host=$2
  local method=$3
  local path=$4
  local candidates
  local candidate_id
  candidates=$(run_tobari policy candidates --tail 1000 --format json)
  candidate_id=$(candidate_id_for_effect "$project_id" https "$host" 8080 "$method" "$path" <<<"$candidates")
  run_tobari policy allow --id "$candidate_id" >/dev/null
}

deny_exact_effect() {
  local project_id=$1
  local host=$2
  local method=$3
  local path=$4
  local candidates
  local candidate_id
  local output
  candidates=$(run_tobari policy candidates --tail 1000 --format json)
  candidate_id=$(candidate_id_for_effect "$project_id" https "$host" 8080 "$method" "$path" <<<"$candidates")
  output=$(run_tobari policy deny --id "$candidate_id")
  assert_contains "$output" "Permission denied" "explicit integration policy denial"
}

graphql_candidate_id_for_effect() {
	local workspace_id=$1
	local scheme=$2
	local host=$3
	local port=$4
	local operation_type=$5
	local root_field=$6
  python3 -c \
    'import json,sys
workspace_id,scheme,host,port,operation_type,root_field=sys.argv[1:]
print(next(item["id"] for item in json.load(sys.stdin)["policy_candidates"]
           if item.get("workspace_id") == workspace_id and item.get("scheme") == scheme
           and item.get("host") == host and item.get("port") == int(port)
           and item.get("protocol") == "graphql"
           and item.get("graphql_operation_type") == operation_type
           and item.get("graphql_root_field") == root_field))' \
    "$workspace_id" "$scheme" "$host" "$port" "$operation_type" "$root_field"
}

cleanup() {
	local gateway_fixture_restore_status=0
	if [[ -n $host_service_attachment_pid ]]; then
		kill "$host_service_attachment_pid" >/dev/null 2>&1 || true
		wait "$host_service_attachment_pid" >/dev/null 2>&1 || true
	fi
	if [[ -n $host_service_server_pid ]]; then
		kill "$host_service_server_pid" >/dev/null 2>&1 || true
		wait "$host_service_server_pid" >/dev/null 2>&1 || true
	fi
  if [[ $owns_shared_fixture == true ]]; then
    docker rm -f "$mock_name" >/dev/null 2>&1 || true
    docker rm -f "$auth_mock_name" >/dev/null 2>&1 || true
    docker network rm "$auth_network" >/dev/null 2>&1 || true
    if [[ -n ${test_root:-} && -x $binary && -n ${work_root:-} ]]; then
      # First ask the public lifecycle to settle an interruption journal and
      # its partial shared runtime before deleting logical Workspaces.
      if [[ -f $test_root/state/tobari/cluster-reconcile.json ]]; then
        run_tobari cluster up >/dev/null 2>&1 || true
      fi
      if [[ -n ${work_ref:-} ]]; then
        run_tobari workspace delete --id "$work_ref" --confirm=delete --force >/dev/null 2>&1 || true
      fi
      if [[ -n ${restricted_ref:-} ]]; then
        run_tobari workspace delete --id "$restricted_ref" --confirm=delete --force >/dev/null 2>&1 || true
      fi
      if [[ -n ${other_ref:-} ]]; then
        run_tobari workspace delete --id "$other_ref" --confirm=delete --force >/dev/null 2>&1 || true
      fi
      if [[ -n ${default_context_ref:-} ]]; then
        run_tobari context delete --id "$default_context_ref" --confirm=delete >/dev/null 2>&1 || true
      fi
      if [[ -n ${restricted_context_ref:-} ]]; then
        run_tobari context delete --id "$restricted_context_ref" --confirm=delete >/dev/null 2>&1 || true
      fi
      if [[ -n ${other_context_ref:-} ]]; then
        run_tobari context delete --id "$other_context_ref" --confirm=delete >/dev/null 2>&1 || true
      fi
      run_tobari cluster down --purge >/dev/null 2>&1 || true
    fi
    # Keep an exact-name Docker fallback for failures in the lifecycle code
    # under test or abrupt harness termination. The preflight above requires
    # these exact shared names to be absent before ownership is set, so any
    # survivors here were created by this integration run.
    for container in \
      tobari-auth-broker tobari-gateway tobari-opa \
      "$work_container" "$other_container" "$restricted_container"; do
      [[ -n $container ]] || continue
      docker rm -f "$container" >/dev/null 2>&1 || true
    done
    docker network rm "$auth_network" >/dev/null 2>&1 || true
    docker network rm tobari-control tobari-egress >/dev/null 2>&1 || true
    docker volume rm tobari-gateway-ca tobari-public-ca tobari-policy-bundle >/dev/null 2>&1 || true
    gateway_fixture_restore_tag || gateway_fixture_restore_status=1
    docker image rm -f "$experimental_gateway_base_image" >/dev/null 2>&1 || true
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
  return "$gateway_fixture_restore_status"
}

finish() {
  local status=$?
  trap - EXIT
  if ((status != 0)); then
    if [[ -n ${test_root:-} && -x $binary && -n ${binary_digest:-} ]]; then
      echo "integration diagnostics: cluster status" >&2
      run_tobari cluster status --format json >&2 || true
      echo "integration diagnostics: doctor" >&2
      run_tobari doctor --format json >&2 || true
      echo "integration diagnostics: final lifecycle journal phases" >&2
      find "$test_root/state" -type f \( -name bootstrap.json -o -name settlement.json -o -name activation.json -o -name cluster-reconcile.json \) -exec grep -H -Eo '"(phase|effect_class|operation)":"[^"]*"' {} + >&2 || true
    fi
    for container in tobari-auth-broker tobari-gateway tobari-opa "$mock_name" "$auth_mock_name" "$work_container" "$other_container" "$restricted_container"; do
      [[ -n $container ]] || continue
      if docker inspect "$container" >/dev/null 2>&1; then
        echo "integration diagnostics: $container" >&2
        docker inspect --format '{{json .State}}' "$container" >&2 || true
        docker logs --tail 200 "$container" >&2 || true
      fi
    done
    if docker volume inspect tobari-policy-bundle >/dev/null 2>&1; then
      local diagnostic_opa_image
      diagnostic_opa_image=$(awk -F= '$1 == "OPA_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
      echo "integration diagnostics: policy bundle aggregate revision" >&2
      docker run --rm --network none --read-only \
        --mount type=volume,src=tobari-policy-bundle,dst=/bundle,readonly \
        "$diagnostic_opa_image" eval --bundle /bundle/bundle.tar.gz --format raw \
        data.tobari.aggregate_revision >&2 || true
      echo "integration diagnostics: policy bundle namespaces" >&2
      docker run --rm --network none --read-only \
        --mount type=volume,src=tobari-policy-bundle,dst=/bundle,readonly \
        "$diagnostic_opa_image" inspect /bundle/bundle.tar.gz >&2 || true
      echo "integration diagnostics: policy bundle top-level data keys" >&2
      docker run --rm --network none --read-only \
        --mount type=volume,src=tobari-policy-bundle,dst=/bundle,readonly \
        "$diagnostic_opa_image" eval --bundle /bundle/bundle.tar.gz --format raw \
        'object.keys(data)' >&2 || true
      local diagnostic_policy_file diagnostic_policy_directory
      diagnostic_policy_file=$(find "$test_root/state" -path '*/policy/data.json' -print -quit)
      if [[ -n $diagnostic_policy_file ]]; then
        diagnostic_policy_directory=$(dirname "$diagnostic_policy_file")
        echo "integration diagnostics: host aggregate policy keys and modes" >&2
        python3 - "$diagnostic_policy_file" <<'PY' >&2 || true
import json
import os
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as source:
    document = json.load(source)
print(sorted(document), oct(os.stat(path).st_mode & 0o777), oct(os.stat(os.path.dirname(path)).st_mode & 0o777))
PY
        echo "integration diagnostics: bind-mounted aggregate policy keys" >&2
        docker run --rm --network none --read-only \
          --mount "type=bind,src=$diagnostic_policy_directory,dst=/policy,readonly" \
          "$diagnostic_opa_image" eval --data /policy/data.json --format raw \
          'object.keys(data)' >&2 || true
      fi
    fi
  fi
  if ! cleanup; then
    echo "integration: failed to restore the pre-existing Gateway resolver tag" >&2
    if ((status == 0)); then
      status=1
    fi
  fi
  if [[ -n ${test_root:-} ]]; then
    rm -rf "$test_root"
  fi
  exit "$status"
}
trap finish EXIT

begin_phase preflight
command -v docker >/dev/null || fail "docker is required"
command -v python3 >/dev/null || fail "python3 is required"
[[ -n $host_docker_context && $host_docker_context != default ]] ||
  fail "TOBARI_INTEGRATION_DOCKER_CONTEXT must name an explicit non-default Docker context"
docker context inspect "$host_docker_context" >/dev/null 2>&1 ||
  fail "Docker context $host_docker_context is unavailable"
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

begin_phase build-fixtures
test_root=$(mktemp -d "$PWD/.tobari-integration.XXXXXX")
if [[ -z $binary ]]; then
  binary=$test_root/tobari-integration
fi
mkdir -p \
  "$test_root/user/workspace" \
  "$test_root/state" \
  "$test_root/tls"
chmod 0700 "$test_root/tls"
chmod 0700 "$test_root/state"

config_directory=$test_root/config/tobari
tool_auth_value=tobari-tool-auth-canary
synthetic_default_secret=synthetic-real-default-canary
synthetic_restricted_secret=synthetic-real-restricted-canary
synthetic_provider=synthetic-ci
mitmproxy_image=$(awk -F= '$1 == "MITMPROXY_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
gateway_dev_tag="tobari-gateway-experimental:dev-$(go run ./tools/runtimeassetid gateway)"
if [[ $host_loopback_only == true ]]; then
  gateway_dev_tag="tobari-gateway:dev-$(go run ./tools/runtimeassetid gateway)"
fi
auth_broker_dev_tag="tobari-auth-broker:dev-$(go run ./tools/runtimeassetid authbroker)"
gateway_fixture_snapshot_tag
# The certificate belongs to this temporary integration run, not to binary or
# image build ownership. Both the self-built and explicit-binary paths use it
# for their bounded synthetic TLS upstreams.
docker run --rm --user "$(id -u):$(id -g)" \
  -v "$test_root/tls:/tls" \
  --entrypoint sh "$mitmproxy_image" -eu -c '
    openssl req -x509 -newkey rsa:2048 -nodes -sha256 -days 2 \
      -subj /CN=api.synthetic.example \
      -addext subjectAltName=DNS:api.synthetic.example,DNS:mock-upstream,DNS:graphql.tobari.dev \
      -addext basicConstraints=critical,CA:TRUE \
      -addext keyUsage=critical,digitalSignature,keyEncipherment,keyCertSign \
      -addext extendedKeyUsage=serverAuth \
      -keyout /tls/synthetic-server.key \
      -out /tls/synthetic-ca.crt >/dev/null 2>&1
    chmod 0600 /tls/synthetic-server.key
    chmod 0644 /tls/synthetic-ca.crt
  '
if [[ -n ${TOBARI_INTEGRATION_BINARY:-} ]]; then
  [[ -x $binary ]] || fail "TOBARI_INTEGRATION_BINARY is not executable: $binary"
  [[ -n $gateway_previous_image_id ]] || fail "explicit integration binary requires the matching experimental development Gateway image"
  [[ $(docker image inspect --format '{{index .Config.Labels "io.tobari.gateway-api"}}' "$gateway_previous_image_id") == 1 ]] || fail "explicit integration Gateway image has an incompatible API"
  [[ $(docker image inspect --format '{{index .Config.Labels "io.tobari.gateway-role"}}' "$gateway_previous_image_id") == enforcement ]] || fail "explicit integration Gateway image has an incompatible role"
  [[ -z $(docker image inspect --format '{{index .Config.Labels "io.tobari.integration-fixture"}}' "$gateway_previous_image_id") ]] || fail "explicit integration Gateway image is a stale TLS fixture rather than a source image"
  gateway_wrapper_base=$gateway_dev_tag
else
  debian_image=$(awk -F= '$1 == "DEBIAN_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
  docker_arch=$(docker info --format '{{.Architecture}}')
  case $docker_arch in
    amd64 | x86_64) auth_target_arch=amd64 ;;
    arm64 | aarch64) auth_target_arch=arm64 ;;
    *) fail "unsupported Docker architecture for Auth Broker integration: $docker_arch" ;;
  esac
  docker build --tag "$gateway_base_image" --file gateway/Dockerfile \
    --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" gateway >/dev/null
  if [[ $host_loopback_only == true ]]; then
    gateway_wrapper_base=$gateway_base_image
    go build -tags=tobari_dev -buildvcs=false -trimpath -o "$binary" ./cmd/tobari
  else
    docker build --tag "$experimental_gateway_base_image" \
      --file gateway/Dockerfile.experimental \
      --build-arg "TOBARI_GATEWAY_BASE=$gateway_base_image" gateway >/dev/null
    gateway_wrapper_base=$experimental_gateway_base_image
    docker build --tag "$auth_broker_dev_tag" --file authbroker/Dockerfile \
      --build-arg "DEBIAN_IMAGE=$debian_image" \
      --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" \
      --build-arg "TARGETARCH=$auth_target_arch" \
      authbroker >/dev/null
    go build -tags='tobari_dev tobari_research' -buildvcs=false -trimpath -o "$binary" ./cmd/tobari
  fi
fi
docker build --tag "$gateway_fixture_image" --file test/integration/gateway-auth.Dockerfile \
  --build-arg "TOBARI_GATEWAY_BASE=$gateway_wrapper_base" \
  "$test_root/tls" >/dev/null
gateway_fixture_publish_tag || fail "failed to publish the run-local Gateway TLS fixture"
expected_gateway_ca_digest=$(shasum -a 256 "$test_root/tls/synthetic-ca.crt" | awk '{print $1}')
actual_gateway_ca_digest=$(docker run --rm --entrypoint sha256sum "$gateway_dev_tag" /usr/local/share/ca-certificates/tobari-integration.crt | awk '{print $1}')
[[ $actual_gateway_ca_digest == "$expected_gateway_ca_digest" ]] ||
  fail "Gateway TLS fixture did not embed the run-local CA"
docker run --rm --entrypoint sh "$gateway_dev_tag" -eu -c 'certifi_bundle=$(python3 -c "import certifi; print(certifi.where())")
  openssl verify -CAfile "$certifi_bundle" /usr/local/share/ca-certificates/tobari-integration.crt >/dev/null
' || fail "Gateway TLS fixture does not trust the run-local CA"
if [[ $host_loopback_only == true ]]; then
  go version -m "$binary" | grep -F $'build\t-tags=tobari_dev' >/dev/null ||
    fail "Host Loopback evaluator binary does not use the standard development surface"
else
  go version -m "$binary" | grep -F $'build\t-tags=tobari_dev,tobari_research' >/dev/null ||
    fail "integration binary does not use the research capability surface"
fi
binary_digest=$(shasum -a 256 "$binary" | awk '{print $1}')
if [[ $host_loopback_only == true ]]; then
  run_final_host_loopback_evaluator
  exit 0
fi
work_root=$test_root/user/workspace
other_root=$test_root/user/other-workspace
mkdir -p "$work_root" "$other_root"
printf 'host-home-canary\n' >"$test_root/user/host-home-canary"
if [[ $host_loopback_only != true && $custom_base_image == tobari-runtime:dev ]] && ! docker image inspect "$custom_base_image" >/dev/null 2>&1; then
  base_image=$(go run ./tools/runtimecheck --print-base-image)
  go_builder_image=$(awk -F= '$1 == "GO_BUILDER_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
  exposure_helper_source=$(go run ./tools/runtimeassetid exposure-helper)
  docker build --tag "$custom_base_image" \
    --file runtimes/base/Dockerfile \
    --build-arg "DEBIAN_IMAGE=$base_image" \
    --build-arg "GO_BUILDER_IMAGE=$go_builder_image" \
    --build-arg "TOBARI_EXPOSURE_HELPER_SOURCE=$exposure_helper_source" \
    --build-arg "TOBARI_UID=$(id -u)" \
    --build-arg "TOBARI_GID=$(id -g)" \
    --build-context helper-source=internal/infra/runtimeassets/_helper-source \
    runtimes/base >/dev/null
  created_dev_runtime_tag=true
fi
if [[ $host_loopback_only != true ]]; then
  assert_base_bash_contract "$custom_base_image"
  if ! docker image inspect tobari-runtime:dev >/dev/null 2>&1; then
    docker tag "$custom_base_image" tobari-runtime:dev
    created_dev_runtime_tag=true
  fi
fi
begin_phase templates-and-cluster
if [[ $host_loopback_only != true ]]; then
  runtime_create=$(run_tobari runtime create --name "$runtime_name" --format json)
  runtime_source_path=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["source_path"])' <<<"$runtime_create")
  runtime_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["runtime_ref"])' <<<"$runtime_create")
  python3 - "$runtime_source_path/Dockerfile" "$custom_base_image" test/integration/custom-image.Dockerfile <<'PY'
from pathlib import Path
import json
import sys

destination, base, source_path = map(Path, sys.argv[1:])
source = source_path.read_text(encoding="utf-8")
lines = []
for line in source.splitlines():
    if line.startswith("ARG TOBARI_RUNTIME_BASE="):
        lines.append(f"FROM {base}")
    elif line != "FROM ${TOBARI_RUNTIME_BASE}":
        lines.append(line)
destination.write_text("\n".join(lines) + "\n", encoding="utf-8")
destination.chmod(0o600)
PY
  runtime_build=$(run_tobari runtime build --id "$runtime_ref" --format json)
  python3 -c \
    'import json,sys; revision=json.load(sys.stdin)["runtime"]["runtime"]["revisions"][-1]; assert revision["availability"]["state"] == "available"; assert revision["source_digest"]; assert revision["revision_ref"]; assert all(key not in revision for key in ("image", "image_digest", "snapshot_path", "revision"))' \
    <<<"$runtime_build"
  runtime_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["id"])' <<<"$runtime_build")
  runtime_revision_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["revisions"][-1]["revision_ref"])' <<<"$runtime_build")
  runtime_source_digest=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["revisions"][-1]["source_digest"])' <<<"$runtime_build")
  capture_runtime_image_for_cleanup "$runtime_id" "$runtime_source_digest"
  if [[ ${TOBARI_INTEGRATION_FAIL_AFTER_RUNTIME_CAPTURE:-false} == true ]]; then
    fail "injected failure after managed Runtime cleanup authority capture"
  fi
fi
template_create_args=(template create --name default --source-access read-write --format json)
if [[ $host_loopback_only != true ]]; then
  template_create_args+=(--graphql-endpoint https://graphql.tobari.dev:8080/graphql)
fi
run_tobari "${template_create_args[@]}" >/dev/null
template_list=$(run_tobari template list --format json)
default_template_ref=$(python3 -c 'import json,sys; print(next(item["template_ref"] for item in json.load(sys.stdin)["templates"]["items"] if item["name"] == "default"))' <<<"$template_list")
run_tobari template default set --id "$default_template_ref" --format json >/dev/null
if [[ $host_loopback_only != true ]]; then
  run_tobari template create --name restricted --source-access read-only --format json >/dev/null
  template_list=$(run_tobari template list --format json)
  restricted_template_ref=$(python3 -c 'import json,sys; print(next(item["template_ref"] for item in json.load(sys.stdin)["templates"]["items"] if item["name"] == "restricted"))' <<<"$template_list")
  run_tobari template runtime set --id "$default_template_ref" --runtime "$runtime_revision_ref" --format json >/dev/null
  run_tobari template runtime set --id "$restricted_template_ref" --runtime "$runtime_revision_ref" --format json >/dev/null
  default_context_create=$(run_tobari_at "$work_root" context create --template "$default_template_ref" --format json)
  restricted_context_create=$(run_tobari_at "$work_root" context create --template "$restricted_template_ref" --format json)
  other_context_create=$(run_tobari_at "$other_root" context create --template "$restricted_template_ref" --format json)
  default_context_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_ref"])' <<<"$default_context_create")
  restricted_context_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_ref"])' <<<"$restricted_context_create")
  other_context_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_ref"])' <<<"$other_context_create")
  default_context_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_id"])' <<<"$default_context_create")
  restricted_context_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_id"])' <<<"$restricted_context_create")
  other_context_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_id"])' <<<"$other_context_create")
fi
# Install research provider fixtures only after first final-authority publication.
if [[ $host_loopback_only != true ]]; then
  mkdir -p "$config_directory/auth/providers"
  chmod 0700 "$config_directory/auth" "$config_directory/auth/providers"
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
fi
  for legacy in "$config_directory/contexts" "$test_root/state/roots" "$test_root/state/instances" "$test_root/state/auth/projects" "$test_root/state/state.json" "$test_root/state/projects.json" "$test_root/state/cluster-reconcile.json" "$config_directory/migrations" "$test_root/state/migrations"; do
    [[ -e $legacy || -L $legacy ]] && echo "integration diagnostics: legacy=${legacy#$test_root/}" >&2
  done
start_cluster >/dev/null

# These assertions intentionally inspect the assembled runtime rather than
# rechecking catalog, renderer, or domain semantics covered by fast tests.
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
wait_network_membership tobari-egress tobari-auth-broker ||
  fail "Auth Broker is not attached to bounded refresh egress"
opa_policy_mount=$(docker inspect --format \
  '{{range .Mounts}}{{if eq .Destination "/bundle"}}{{.Type}}|{{.Name}}|{{.Destination}}|{{.RW}}{{end}}{{end}}' \
  tobari-opa)
[[ $opa_policy_mount == 'volume|tobari-policy-bundle|/bundle|false' ]] ||
  fail "OPA policy bundle mount is not the owned read-only volume: $opa_policy_mount"
gateway_context_mounts=$(docker inspect --format '{{range .Mounts}}{{println .Source "=>" .Destination}}{{end}}' tobari-gateway)
if [[ $gateway_context_mounts == *"/credentials.json =>"* || $gateway_context_mounts == *"/run/tobari/credentials"* ]]; then
  fail "Gateway retained the retired managed credential projection"
fi
assert_contains "$gateway_context_mounts" "/run/tobari/auth" \
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

begin_phase credentials-and-workspaces
printf '%s' "$synthetic_default_secret" | \
  run_tobari auth import "$synthetic_provider" --context "$default_context_ref" --format json >/dev/null
printf '%s' "$synthetic_restricted_secret" | \
  run_tobari auth import "$synthetic_provider" --context "$restricted_context_ref" --format json >/dev/null
printf '%s' "$synthetic_restricted_secret" | \
  run_tobari auth import "$synthetic_provider" --context "$other_context_ref" --format json >/dev/null
container_work_root="/var/lib/tobari/${work_root#"$test_root/user/"}"
enter_tobari_at "$work_root" context enter --id "$default_context_ref"
enter_tobari_at "$work_root" context enter --id "$restricted_context_ref"
enter_tobari_at "$other_root" context enter --id "$other_context_ref"
list_json=$(run_tobari workspace list --format json)
work_ref=$(workspace_field_for_context workspace_ref "$default_context_id" <<<"$list_json")
restricted_ref=$(workspace_field_for_context workspace_ref "$restricted_context_id" <<<"$list_json")
other_ref=$(workspace_field_for_context workspace_ref "$other_context_id" <<<"$list_json")
work_id=$(workspace_field_for_context workspace_id "$default_context_id" <<<"$list_json")
restricted_id=$(workspace_field_for_context workspace_id "$restricted_context_id" <<<"$list_json")
other_id=$(workspace_field_for_context workspace_id "$other_context_id" <<<"$list_json")
work_container=$(container_for_id "$work_id")
restricted_container=$(container_for_id "$restricted_id")
work_network=$(network_for_id "$work_id")
restricted_network=$(network_for_id "$restricted_id")
other_network=$(network_for_id "$other_id")
other_container=$(container_for_id "$other_id")
[[ $(docker inspect --format '{{.State.Running}}' "$work_container") == true ]] ||
  fail "Workspace is not running after entry detached"
[[ $(docker inspect --format '{{json .Config.Cmd}}' "$work_container") == '["sleep","infinity"]' ]] ||
  fail "Workspace lifetime command was not sleep infinity after Bash exit"
assert_workspace_service_helper_mount "$work_container"

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
    raise SystemExit(f"Context bindings are incomplete: {bindings!r}")
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

work_status=$(run_tobari workspace status --id "$work_ref" --format json)
work_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["workspace"]["workspace_home"])' <<<"$work_status")
restricted_status=$(run_tobari workspace status --id "$restricted_ref" --format json)
restricted_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["workspace"]["workspace_home"])' <<<"$restricted_status")
other_status=$(run_tobari workspace status --id "$other_ref" --format json)
other_home=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["workspace"]["workspace_home"])' <<<"$other_status")
[[ $work_home != "$restricted_home" && $work_home != "$other_home" && $restricted_home != "$other_home" ]] || fail "Context-bound Workspaces share a home directory"
printf 'shared-project-files\n' >"$work_root/context-sharing-canary"
assert_contains "$(run_restricted_project cat "$container_work_root/context-sharing-canary")" "shared-project-files" "same-root cross-Context project file sharing"

# Source access applies only to the selected live source bind. Both Templates
# observe the same host tree, while Workspace home and tmpfs remain writable.
[[ $(docker inspect --format \
  "{{range .Mounts}}{{if eq .Destination \"$container_work_root\"}}{{.RW}}{{end}}{{end}}" \
  "$work_container") == true ]] || fail "read-write Template source bind is not writable"
[[ $(docker inspect --format \
  "{{range .Mounts}}{{if eq .Destination \"$container_work_root\"}}{{.RW}}{{end}}{{end}}" \
  "$restricted_container") == false ]] || fail "read-only Template source bind is writable"
if docker inspect --format '{{range .Mounts}}{{println .Destination .RW}}{{end}}' "$restricted_container" | \
  awk -v root="$container_work_root" '$1 == root && $2 == "true" {found=1} END {exit found ? 0 : 1}'; then
  fail "read-only Template exposes a writable alias for the selected source"
fi
assert_contains "$(run_restricted_project cat "$container_work_root/context-sharing-canary")" \
  "shared-project-files" "read-only Template source read"
for mutation in \
  "printf changed > '$container_work_root/context-sharing-canary'" \
  "printf created > '$container_work_root/read-only-create'" \
  "rm '$container_work_root/context-sharing-canary'" \
  "mv '$container_work_root/context-sharing-canary' '$container_work_root/read-only-rename'" \
  "chmod 0600 '$container_work_root/context-sharing-canary'" \
  "git -C '$container_work_root' init"; do
  if run_restricted_project sh -c "$mutation" >/dev/null 2>&1; then
    fail "read-only Template allowed source mutation: $mutation"
  fi
done
run_project sh -c "printf observed > '$container_work_root/read-write-observation'"
assert_contains "$(run_restricted_project cat "$container_work_root/read-write-observation")" \
  "observed" "read-only Template observation of read-write Template change"
printf 'host-observed\n' >"$work_root/host-observation"
assert_contains "$(run_restricted_project cat "$container_work_root/host-observation")" \
  "host-observed" "read-only Template observation of host change"
run_restricted_project sh -c 'printf home-write > /var/lib/tobari/source-access-home'
run_restricted_project sh -c 'printf tmp-write > /tmp/source-access-tmp'
assert_contains "$(run_restricted_project cat /var/lib/tobari/source-access-home)" \
  "home-write" "read-only Template writable home"
assert_contains "$(run_restricted_project cat /tmp/source-access-tmp)" \
  "tmp-write" "read-only Template writable tmpfs"

assert_resource_bounds "$work_container"
assert_resource_bounds "$restricted_container"
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
[[ $(docker inspect --format '{{len .NetworkSettings.Networks}}' tobari-auth-broker) == 2 ]] ||
  fail "Auth Broker did not retain exactly control and bounded refresh egress networks"
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

if run_project test -e /var/lib/tobari/host-home-canary; then
  fail "Tobari mounted the host home wholesale"
fi
run_project sh -c "printf '%s\\n' '$tool_auth_value' > /var/lib/tobari/tool-auth-state"
if docker exec "$other_container" test -e /var/lib/tobari/tool-auth-state; then
  fail "tool authentication state leaked to another project"
fi
if docker exec "$restricted_container" test -e /var/lib/tobari/tool-auth-state; then
  fail "tool authentication state leaked across Contexts on the same root"
fi

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
[[ $tobari_image == "$runtime_image" ]] || fail "Workspace did not retain the captured managed Runtime selector"
[[ $(docker inspect --format '{{.Image}}' "$work_container") == "$runtime_image_id" ]] ||
  fail "Workspace did not retain the captured managed Runtime image identity"
custom_image_cmd=$(docker image inspect --format '{{json .Config.Cmd}}' "$runtime_image")
[[ $custom_image_cmd == '["sh","-c","exit 23"]' ]] ||
  fail "managed Runtime fixture does not have a terminating default command: $custom_image_cmd"
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
begin_phase gateway-broker-and-transport
docker network create --internal --subnet 11.254.43.0/24 "$auth_network" >/dev/null
docker network connect "$auth_network" tobari-gateway
docker network connect --alias graphql.tobari.dev "$auth_network" "$mock_name"
wait_network_connection tobari-gateway graphql.tobari.dev 8080
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
wait_project_broker_policy_denial https://api.synthetic.example/brokered-default

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
[[ $default_broker_denial_status == 403 ]] ||
  fail "brokered request with an unreadable vault returned $default_broker_denial_status instead of policy denial"
if docker logs "$auth_mock_name" 2>&1 | grep -F '"/brokered-default"' >/dev/null; then
  fail "policy-denied brokered request reached the synthetic upstream"
fi

broker_candidates=$(run_tobari policy candidates --tail 1000 --format json)
default_broker_candidate_id=$(candidate_id_for_effect \
  "$work_id" https api.synthetic.example 443 GET /brokered-default <<<"$broker_candidates")
opa_before_policy_activation=$(docker inspect --format '{{.Id}}' tobari-opa)
run_tobari policy allow --id "$default_broker_candidate_id" >/dev/null
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
[[ $(docker inspect --format '{{.Id}}' tobari-opa) == "$opa_before_policy_activation" ]] ||
  fail "live policy activation recreated OPA"

# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
restricted_broker_denial=$(run_restricted_project sh -c \
  'curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: $SYNTHETIC_TOKEN" https://api.synthetic.example/brokered-restricted')
restricted_broker_denial_status=${restricted_broker_denial##*$'\n'}
[[ $restricted_broker_denial_status == 403 ]] ||
  fail "restricted Context brokered request returned $restricted_broker_denial_status instead of policy denial"
if docker logs "$auth_mock_name" 2>&1 | grep -F '"/brokered-restricted"' >/dev/null; then
  fail "restricted policy-denied brokered request reached the synthetic upstream"
fi

broker_candidates=$(run_tobari policy candidates --tail 1000 --format json)
restricted_broker_candidate_id=$(candidate_id_for_effect \
  "$restricted_id" https api.synthetic.example 443 GET /brokered-restricted <<<"$broker_candidates")
run_tobari policy allow --id "$restricted_broker_candidate_id" >/dev/null
# SYNTHETIC_TOKEN expands inside the Workspace.
# shellcheck disable=SC2016
restricted_broker_response=$(run_restricted_project sh -c \
  'curl -fsS -H "X-Synthetic-Auth: $SYNTHETIC_TOKEN" https://api.synthetic.example/brokered-restricted')
restricted_broker_digest=$(printf 'Bearer %s' "$synthetic_restricted_secret" | shasum -a 256 | awk '{print $1}')
assert_contains "$restricted_broker_response" \
  "\"authorization_sha256\":\"$restricted_broker_digest\"" \
  "restricted Context brokered credential digest"
if [[ $restricted_broker_response == *"$default_broker_digest"* ]]; then
  fail "one shared Auth Broker crossed Context credential authority"
fi

copied_context_result=$(printf '%s\n' "$work_auth_handle" | \
  docker exec -i "$restricted_container" sh -c \
    'IFS= read -r copied; curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: $copied" https://api.synthetic.example/copied-context')
copied_context_status=${copied_context_result##*$'\n'}
[[ $copied_context_status == 403 ]] ||
  fail "handle copied across Contexts returned $copied_context_status instead of 403"

copied_project_result=$(printf '%s\n' "$restricted_auth_handle" | \
  docker exec -i "$other_container" sh -c \
    'IFS= read -r copied; curl -sS -w "\n%{http_code}" -H "X-Synthetic-Auth: $copied" https://api.synthetic.example/copied-project')
copied_project_status=${copied_project_result##*$'\n'}
[[ $copied_project_status == 403 ]] ||
  fail "handle copied across projects returned $copied_project_status instead of 403"
for rejected_path in copied-context copied-project; do
  if docker logs "$auth_mock_name" 2>&1 | grep -F "\"/$rejected_path\"" >/dev/null; then
    fail "cross-boundary broker handle request /$rejected_path reached the synthetic upstream"
  fi
done

wrong_port_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  http://mock-upstream:8081/wrong-port)
[[ $wrong_port_status == 403 ]] || fail "plain-HTTP guardrail canary returned $wrong_port_status instead of 403"
if docker logs "$mock_name" 2>&1 | grep -F '"/wrong-port"' >/dev/null; then
  fail "wrong-port request reached mock upstream"
fi

upload_denial=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST https://mock-upstream:8080/stream-upload)
[[ $upload_denial == 403 ]] || fail "unreviewed chunked target returned $upload_denial instead of 403"
allow_exact_effect "$work_id" mock-upstream POST /stream-upload
upload_output="$test_root/stream-upload.out"
docker exec "$work_container" python3 -c '
import http.client
import os
import ssl
import time

connection = http.client.HTTPSConnection(
    "mock-upstream", 8080, timeout=10,
    context=ssl.create_default_context(cafile=os.environ["SSL_CERT_FILE"]),
)
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

stream_denial=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  https://mock-upstream:8080/stream-response)
[[ $stream_denial == 403 ]] || fail "unreviewed streaming target returned $stream_denial instead of 403"
allow_exact_effect "$work_id" mock-upstream GET /stream-response
stream_prefix=$(run_project curl -NsS --max-time 1 \
  https://mock-upstream:8080/stream-response || true)
assert_contains "$stream_prefix" 'data: first' "streaming response prefix"
if [[ $stream_prefix == *'data: second'* ]]; then
  fail "streaming response completed before the upstream delay"
fi

oversized_target_denial=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -X POST https://mock-upstream:8080/oversized-body)
[[ $oversized_target_denial == 403 ]] ||
  fail "unreviewed oversized-body target returned $oversized_target_denial instead of 403"
allow_exact_effect "$work_id" mock-upstream POST /oversized-body
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

begin_phase live-policy-activation
policy_bundle_mount=$(docker inspect --format \
  '{{range .Mounts}}{{if eq .Destination "/bundle"}}{{println .Name .RW}}{{end}}{{end}}' \
  tobari-opa)
[[ $policy_bundle_mount == 'tobari-policy-bundle false' ]] ||
  fail "OPA does not mount the exact read-only policy bundle volume: $policy_bundle_mount"
[[ $(docker volume inspect --format '{{index .Labels "io.tobari.owner"}}' tobari-policy-bundle) == default ]] ||
  fail "policy bundle volume is missing its Tobari owner label"

# One passthrough credential canary proves that live denial, opaque-reference
# activation, watched-bundle publication, and post-policy forwarding compose.
expected_digest=$(printf 'Bearer %s' "$tool_auth_value" | shasum -a 256 | awk '{print $1}')
auth_denial=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer $tool_auth_value" \
  https://mock-upstream:8080/credential)
[[ $auth_denial == 403 ]] || fail "unreviewed passthrough request returned $auth_denial instead of 403"
opa_before_passthrough_activation=$(docker inspect --format '{{.Id}}' tobari-opa)
allow_exact_effect "$work_id" mock-upstream GET /credential
auth_response=$(run_project curl -fsS \
  -H "Authorization: Bearer $tool_auth_value" \
  https://mock-upstream:8080/credential)
assert_contains "$auth_response" "\"authorization_sha256\":\"$expected_digest\"" \
  "post-policy passthrough credential digest"
[[ $(docker inspect --format '{{.Id}}' tobari-opa) == "$opa_before_passthrough_activation" ]] ||
  fail "watched policy activation recreated OPA"

# The parser and policy matrix live in fast Gateway/Rego/domain tests. This
# sole runtime canary proves that a declared GraphQL denial crosses the real
# transparent transport and performs zero upstream I/O.
graphql_projection=$(docker exec tobari-gateway cat /run/tobari/config/gateway.json)
python3 -c '
import json
import sys
context_id = sys.argv[1]
document = json.load(sys.stdin)
endpoint = {"scheme": "https", "host": "graphql.tobari.dev", "port": 8080, "path": "/graphql"}
if endpoint not in document["contexts"][context_id]["graphql_endpoints"]:
    raise SystemExit("declared GraphQL endpoint is absent from the live Gateway projection")
' "$default_context_id" <<<"$graphql_projection"
graphql_body='{"query":"mutation Change { closeIssue updateIssue }","variables":{"value":"runtime-canary"}}'
graphql_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  -H 'content-type: application/json' --data-binary "$graphql_body" \
  https://graphql.tobari.dev:8080/graphql)
[[ $graphql_status == 403 ]] ||
  fail "declared GraphQL denial returned $graphql_status instead of 403"
if docker logs "$mock_name" 2>&1 | grep -F '"/graphql"' >/dev/null; then
  fail "denied GraphQL request reached mock upstream"
fi

begin_phase attachment-scoped-host-loopback
host_service_port_file=$test_root/host-service.port
host_service_request_log=$test_root/host-service.requests.jsonl
python3 - "$host_service_port_file" "$host_service_request_log" <<'PY' &
import http.server
import json
import pathlib
import socketserver
import sys

class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        with open(sys.argv[2], "a", encoding="utf-8") as handle:
            handle.write(json.dumps({"host": self.headers.get("Host"), "path": self.path}) + "\n")
        body = b"host-service-ok\n"
        self.send_response(200)
        self.send_header("Content-Type", "text/plain")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def log_message(self, _format, *_args):
        pass

with socketserver.TCPServer(("127.0.0.1", 0), Handler) as server:
    pathlib.Path(sys.argv[1]).write_text(str(server.server_address[1]), encoding="ascii")
    server.serve_forever()
PY
host_service_server_pid=$!
for _ in $(seq 1 60); do
  [[ -s $host_service_port_file ]] && break
  sleep 0.1
done
[[ -s $host_service_port_file ]] || fail "physical-host HTTP fixture did not publish its port"
host_service_port=$(<"$host_service_port_file")
host_loopback_hostname=host.tobari.internal
retired_host_loopback_hostname=host.tobari.test
gateway_ca_before=$(docker exec tobari-gateway sha256sum /var/lib/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem | awk '{print $1}')
gateway_cert_files_before=$(docker exec tobari-gateway sh -c 'find /var/lib/mitmproxy/.mitmproxy -type f -print | sort')

# Expansion is intentionally deferred to the attached Workspace shell.
# shellcheck disable=SC2016
host_attachment_events=$(python3 -c '
import json
import sys
print(json.dumps([
    {"after_ms": 500, "data": """curl -sS https://mock-upstream:8080/permission-resume > /var/lib/tobari/permission-denial.json; python3 -c '\''import json,re,sys; document=json.load(open(sys.argv[1], encoding=\"utf-8\")); command=document[\"tobari\"][\"resume\"][\"command\"]; prefix=\"tobari-permission wait --id \"; wait_id=command[len(prefix):] if command.startswith(prefix) else \"\"; match=re.fullmatch(r\"pwt_[0-9a-f]{32}\", wait_id); sys.exit(2) if match is None else None; print(wait_id)'\'' /var/lib/tobari/permission-denial.json > /var/lib/tobari/permission-wait-id; { tobari-permission wait --id \"$(cat /var/lib/tobari/permission-wait-id)\" > /var/lib/tobari/permission-wait.out 2> /var/lib/tobari/permission-wait.err && if grep -qx Allow /var/lib/tobari/permission-wait.out; then curl -fsS https://mock-upstream:8080/permission-resume > /var/lib/tobari/permission-retry.json; fi; } &\n"""},
    {"after_ms": 3000, "data": """host=""" + sys.argv[2] + """; retired=""" + sys.argv[3] + """; { printf "%s\\n" "$TOBARI_CAPABILITIES_JSON"; curl -sS -o /dev/null -w "%{http_code}" http://${host}:""" + sys.argv[1] + """/health; printf "\\n"; curl -sS -o /dev/null -w "%{http_code}" http://${retired}:""" + sys.argv[1] + """/health; printf "\\n"; curl -ksS --connect-timeout 5 https://${host}:""" + sys.argv[1] + """/health >/dev/null 2>&1; printf "%s\\n" "$?"; curl -ksS --connect-timeout 5 https://${retired}:""" + sys.argv[1] + """/health >/dev/null 2>&1; printf "%s\\n" "$?"; python3 -c \"import socket,sys; print(socket.gethostbyname(sys.argv[1])); print(socket.gethostbyname(sys.argv[2]))\" "$host" "$retired"; } > /var/lib/tobari/host-probe.tmp && mv /var/lib/tobari/host-probe.tmp /var/lib/tobari/host-probe\n"""},
  {"after_ms": 1500, "data": "python3 -m http.server 32123 --bind 127.0.0.1 >/var/lib/tobari/service-server.log 2>&1 & tobari-expose 32123 > /var/lib/tobari/service-exposure.out 2>/var/lib/tobari/service-exposure.err; printf ready > /var/lib/tobari/service-exposure-ready; tobari-expose status > /var/lib/tobari/service-exposure-status.out; while [ ! -e /var/lib/tobari/service-stop ]; do sleep 0.1; done; exposure_ref=$(python3 -c '\''import json; print(json.load(open(\"/var/lib/tobari/service-exposure.out\"))[\"exposure\"][\"id\"])'\''); tobari-expose stop \"$exposure_ref\" > /var/lib/tobari/service-stop.out; printf stopped > /var/lib/tobari/service-stopped\n"},
    {"after_ms": 25000, "data": "exit\n"},
]))
' "$host_service_port" "$host_loopback_hostname" "$retired_host_loopback_hostname")
TOBARI_TEST_PTY_TIMEOUT_SECONDS=40 \
  TOBARI_TEST_PTY_EVENTS="$host_attachment_events" \
  run_tobari_pty_at "$work_root" \
  >"$test_root/host-attachment.out" 2>&1 &
host_service_attachment_pid=$!
verify_permission_resume_handoff
for _ in $(seq 1 300); do
  if run_project test -s /var/lib/tobari/host-probe >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$host_service_attachment_pid" >/dev/null 2>&1; then
    wait "$host_service_attachment_pid" || true
    host_service_attachment_pid=
    cat "$test_root/host-attachment.out" >&2
    fail "Host Loopback attachment exited before its capability probe completed"
  fi
  sleep 0.1
done
if ! run_project test -s /var/lib/tobari/host-probe >/dev/null 2>&1; then
  cat "$test_root/host-attachment.out" >&2
  fail "Host Loopback attachment did not complete its capability probe"
fi
host_probe=$(run_project cat /var/lib/tobari/host-probe)
host_capabilities=$(sed -n '1p' <<<"$host_probe")
HOST_CAPABILITIES="$host_capabilities" python3 <<'PY'
import json
import os

document = json.loads(os.environ["HOST_CAPABILITIES"])
host_http = document.get("host_http", {})
if document.get("schema_version") != 1 or host_http.get("url_template") != "http://host.tobari.internal:{port}" or host_http.get("lifetime") != "attachment":
    raise SystemExit(f"active attachment did not project the Host Loopback route: {document!r}")
if "relay" in os.environ["HOST_CAPABILITIES"] or "token" in os.environ["HOST_CAPABILITIES"]:
    raise SystemExit("Host Loopback capability projection disclosed relay authority")
PY
host_denial_status=$(sed -n '2p' <<<"$host_probe")
[[ $host_denial_status == 403 ]] || fail "unreviewed Host Loopback returned $host_denial_status instead of 403"
retired_host_status=$(sed -n '3p' <<<"$host_probe")
[[ $retired_host_status == 410 ]] || fail "retired Host Loopback returned $retired_host_status instead of terminal 410"
current_tls_status=$(sed -n '4p' <<<"$host_probe")
retired_tls_status=$(sed -n '5p' <<<"$host_probe")
[[ $current_tls_status != 0 && $retired_tls_status != 0 ]] ||
  fail "Host Loopback TLS did not close terminally: current=$current_tls_status retired=$retired_tls_status"
[[ $(sed -n '6p' <<<"$host_probe") == 198.18.0.10 ]] || fail "current Host Loopback did not use synthetic DNS"
[[ $(sed -n '7p' <<<"$host_probe") == 198.18.0.10 ]] || fail "retired Host Loopback did not use synthetic DNS before terminal classification"
[[ ! -s $host_service_request_log ]] || fail "denied or TLS Host Loopback traffic reached physical-host loopback"
gateway_ca_after_terminal=$(docker exec tobari-gateway sha256sum /var/lib/mitmproxy/.mitmproxy/mitmproxy-ca-cert.pem | awk '{print $1}')
gateway_cert_files_after_terminal=$(docker exec tobari-gateway sh -c 'find /var/lib/mitmproxy/.mitmproxy -type f -print | sort')
[[ $gateway_ca_after_terminal == "$gateway_ca_before" ]] || fail "terminal Host Loopback TLS rotated the Gateway CA"
[[ $gateway_cert_files_after_terminal == "$gateway_cert_files_before" ]] || fail "terminal Host Loopback TLS changed the persistent certificate store"

host_review=
host_review_index=
for _ in $(seq 1 30); do
  host_review=$(run_tobari review permissions --tail 1000 --format json)
  host_review_index=$(python3 -c '
import json
import sys
items = json.load(sys.stdin)["policy_review"]
print(next((index for index, item in enumerate(items, 1)
            if item["host"] == "host.tobari.internal" and item["port"] == int(sys.argv[1]) and item["path"] == "/health"), ""))
' "$host_service_port" <<<"$host_review")
  [[ -n $host_review_index ]] && break
  sleep 0.2
done
[[ -n $host_review_index ]] || fail "Host Loopback denial did not reach review permissions"

host_review_events=$(python3 -c '
import json
import sys
selection = "\x1b[B" * (int(sys.argv[1]) - 1) + "\r"
print(json.dumps([
    {"after_ms": 5000, "data": selection},
    {"after_ms": 750, "data": "a"},
    {"after_ms": 750, "data": "p"},
    {"after_ms": 750, "data": "y"},
]))
' "$host_review_index")
if ! host_review_output=$(TOBARI_TEST_PTY_TIMEOUT_SECONDS=15 \
  TOBARI_TEST_PTY_EVENTS="$host_review_events" \
  run_tobari_pty_at "$work_root" review permissions --tail 1000 2>&1); then
  printf '%s\n' "$host_review_output" >&2
  fail "interactive Host Loopback review permissions failed"
fi
python3 - "$config_directory/host-loopback/routes.json" "$work_id" "$host_service_port" <<'PY'
import hashlib
import json
import socket
import sys

document = json.load(open(sys.argv[1], encoding="utf-8"))
if document.get("schema_version") != 2:
    raise SystemExit(f"Host Loopback route registry is not schema V2: {document!r}")
routes = document["routes"]
route = next(item for item in routes if item["project_id"] == sys.argv[2])
material = "\0".join(("tobari-host-loopback-route-v2", route["attachment_epoch_id"], route["context_id"], route["project_id"], route["hostname"]))
expected_id = "hlr_" + hashlib.sha256(material.encode()).hexdigest()[:32]
if route["hostname"] != "host.tobari.internal" or route["id"] != expected_id:
    raise SystemExit(f"Host Loopback route identity is not bound to the exact authority: {route!r}")
target_port = int(sys.argv[3])
with socket.create_connection(("127.0.0.1", route["relay_port"]), timeout=3) as relay:
    relay.sendall(b"C" + route["relay_token"].encode("ascii") + target_port.to_bytes(2, "big"))
    if relay.recv(2) != b"OK":
        raise SystemExit("reviewed Host Loopback relay rejected its trusted-host integration probe")
    relay.sendall(b"GET /health HTTP/1.1\r\nHost: host.tobari.internal\r\nConnection: close\r\n\r\n")
    response = bytearray()
    while chunk := relay.recv(65536):
        response.extend(chunk)
if b"host-service-ok" not in response:
    raise SystemExit("reviewed Host Loopback relay did not forward its trusted-host integration probe")
PY
python3 - "$config_directory/host-loopback/routes.json" "$work_id" <<'PY' |
import json
import sys

routes = json.load(open(sys.argv[1], encoding="utf-8"))["routes"]
route = next(item for item in routes if item["project_id"] == sys.argv[2])
json.dump({"relay_port": route["relay_port"], "relay_token": route["relay_token"]}, sys.stdout)
PY
  docker exec -i tobari-gateway python3 -c '
import json
import socket
import sys

route = json.load(sys.stdin)
try:
    with socket.create_connection(("host.docker.internal", route["relay_port"]), timeout=3) as relay:
        relay.sendall(b"P" + route["relay_token"].encode("ascii"))
        if relay.recv(2) != b"OK":
            raise OSError("relay rejected probe")
except OSError:
    raise SystemExit("Gateway could not authenticate to the active Host Loopback relay")
'
host_retry=$(run_project curl -fsS "http://$host_loopback_hostname:$host_service_port/health")
assert_contains "$host_retry" "host-service-ok" "reviewed physical-host HTTP response"
host_python_retry=$(run_project python3 -c 'import sys,urllib.request; print(urllib.request.urlopen(sys.argv[1], timeout=5).read().decode())' \
  "http://$host_loopback_hostname:$host_service_port/health")
assert_contains "$host_python_retry" "host-service-ok" "reviewed Python physical-host HTTP response"
python3 - "$host_service_request_log" "$host_service_port" <<'PY'
import json
import sys

entries = [json.loads(line) for line in open(sys.argv[1], encoding="utf-8")]
allowed_hosts = {"host.tobari.internal", f"host.tobari.internal:{sys.argv[2]}"}
if len(entries) < 3 or any(item.get("host") not in allowed_hosts or item.get("path") != "/health" for item in entries):
    raise SystemExit(f"Host Loopback did not preserve the exact public authority: {entries!r}")
PY
python3 - "$config_directory/host-loopback/grants.json" "$work_id" "$host_service_port" <<'PY'
import json
import sys

document = json.load(open(sys.argv[1], encoding="utf-8"))
if document.get("schema_version") != 2:
    raise SystemExit(f"Host Loopback grant registry is not schema V2: {document!r}")
grant = next(item for item in document["grants"] if item["project_id"] == sys.argv[2] and item["target_port"] == int(sys.argv[3]))
if grant["host"] != "host.tobari.internal" or grant["lifetime"] != "attachment":
    raise SystemExit(f"Host Loopback grant widened its exact attachment authority: {grant!r}")
PY

verify_workspace_service_exposure

wait "$host_service_attachment_pid"
host_service_attachment_pid=
post_detach_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  "http://$host_loopback_hostname:$host_service_port/health")
[[ $post_detach_status == 403 ]] || fail "detached Host Loopback returned $post_detach_status instead of 403"
python3 - "$config_directory/host-loopback/routes.json" "$config_directory/host-loopback/grants.json" <<'PY'
import json
import sys
services = json.load(open(sys.argv[1], encoding="utf-8"))["routes"]
grants = json.load(open(sys.argv[2], encoding="utf-8"))["grants"]
if services or grants:
    raise SystemExit(f"Host Loopback authority survived detach: routes={services!r} grants={grants!r}")
PY
kill "$host_service_server_pid" >/dev/null 2>&1 || true
wait "$host_service_server_pid" >/dev/null 2>&1 || true
host_service_server_pid=

if [[ $host_loopback_only == true ]]; then
  complete_phase
  echo "Host Loopback isolated integration: OK"
  exit 0
fi

begin_phase runtime-failure-boundaries
diagnostic_surface=$(printf '%s\n' \
  "$(docker logs tobari-gateway 2>&1)" \
  "$(docker logs tobari-auth-broker 2>&1)" \
  "$(docker logs tobari-opa 2>&1)" \
  "$(docker logs "$mock_name" 2>&1)" \
  "$(docker logs "$auth_mock_name" 2>&1)")
for authentication_canary in \
  "$synthetic_default_secret" "$synthetic_restricted_secret" \
  "$work_auth_handle" "$restricted_auth_handle" "$other_auth_handle"; do
  if [[ $diagnostic_surface == *"$authentication_canary"* ]]; then
    fail "runtime logs exposed authentication material"
  fi
  if grep -R -a -F --exclude=vault.enc -- "$authentication_canary" \
    "$test_root/state/tobari" >/dev/null 2>&1; then
    fail "host state stored authentication material outside an encrypted vault"
  fi
done

if run_project curl --noproxy '*' --max-time 3 -fsS \
  http://opa:8181/health >/dev/null 2>&1; then
  fail "Workspace reached the OPA control API"
fi
docker stop tobari-opa >/dev/null
opa_down_status=$(run_project curl -sS -o /dev/null -w '%{http_code}' \
  https://mock-upstream:8080/opa-down)
[[ $opa_down_status == 503 ]] || fail "OPA outage returned $opa_down_status instead of 503"
docker start tobari-opa >/dev/null
wait_healthy tobari-opa

docker stop tobari-gateway >/dev/null
if run_project python3 -c \
  'import socket; socket.create_connection(("1.1.1.1", 443), 2)' \
  >/dev/null 2>&1; then
  fail "Workspace opened a direct raw Internet connection while Gateway was stopped"
fi
if run_project curl --max-time 3 -fsS \
  https://mock-upstream:8080/gateway-down >/dev/null 2>&1; then
  fail "request succeeded while Gateway was stopped"
fi
docker start tobari-gateway >/dev/null
wait_healthy tobari-gateway

if run_project test -e /var/run/docker.sock; then
  fail "Workspace contains the Docker socket"
fi
mounts=$(docker inspect --format '{{range .Mounts}}{{.Destination}}{{"\n"}}{{end}}' "$work_container")
if grep -E '^/(run/tobari/credentials|var/run/docker.sock)$' <<<"$mounts" >/dev/null; then
  fail "Workspace has a forbidden credential or Docker mount"
fi

begin_phase lifecycle
docker rm -f "$mock_name" >/dev/null
docker rm -f "$auth_mock_name" >/dev/null
set +e
run_tobari cluster down >/dev/null 2>&1
down_with_projects_status=$?
set -e
[[ $down_with_projects_status != 0 ]] || fail "cluster down succeeded while Context-bound Workspaces remained"
[[ $(docker inspect --format '{{.State.Running}}' tobari-gateway) == true ]] || fail "refused cluster down stopped the shared Gateway"
run_tobari workspace delete --id "$work_ref" --confirm=delete --force >/dev/null
work_id=
work_ref=
work_container=
[[ ! -e "$work_home/tool-auth-state" ]] || fail "delete did not remove tool authentication state"
python3 - "$config_directory/principal-registry/principals.json" "$restricted_id" "$other_id" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
if {item["project_id"] for item in bindings} != set(sys.argv[2:]):
    raise SystemExit(f"deleted project principal was not removed: {bindings!r}")
PY
run_tobari workspace delete --id "$restricted_ref" --confirm=delete --force >/dev/null
restricted_id=
restricted_ref=
restricted_container=
run_tobari workspace delete --id "$other_ref" --confirm=delete --force >/dev/null
other_id=
other_ref=
other_container=
python3 - "$config_directory/principal-registry/principals.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    bindings = json.load(handle)["bindings"]
if bindings:
    raise SystemExit(f"project principal registry was not cleared: {bindings!r}")
PY

run_tobari auth logout "$synthetic_provider" --context "$default_context_ref" --format json >/dev/null
run_tobari auth logout "$synthetic_provider" --context "$restricted_context_ref" --format json >/dev/null
run_tobari auth logout "$synthetic_provider" --context "$other_context_ref" --format json >/dev/null
run_tobari context delete --id "$default_context_ref" --confirm=delete >/dev/null
run_tobari context delete --id "$restricted_context_ref" --confirm=delete >/dev/null
run_tobari context delete --id "$other_context_ref" --confirm=delete >/dev/null
default_context_ref=
restricted_context_ref=
other_context_ref=
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

complete_phase
echo "integration: OK"
