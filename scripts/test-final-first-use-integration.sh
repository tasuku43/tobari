#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "$0")/.."

host_docker_config=${DOCKER_CONFIG:-$HOME/.docker}
host_docker_context=${TOBARI_INTEGRATION_DOCKER_CONTEXT:-${DOCKER_CONTEXT:-}}
test_root=
binary=
candidate_binary=${TOBARI_FIRST_USE_INTEGRATION_BINARY:-}
predecessor_binary=${TOBARI_FIRST_USE_PREDECESSOR_BINARY:-}
owns_shared_resources=false
context_ref=
workspace_ref=
nested_workspace_ref=
upgrade_context_ref=
upgrade_prepared=false
transition_runtime_image_id=
standard_runtime_image=$(go run ./tools/runtimeassetid standard-runtime-image)
gateway_image="tobari-gateway:base-$(go run ./tools/runtimeassetid gateway)"

# shellcheck disable=SC1091
source test/integration/release_upgrade_reentry.sh
# shellcheck disable=SC1091
source test/integration/workspace_entry_transition_matrix.sh

docker() {
  [[ -n ${host_docker_context:-} && $host_docker_context != default ]] || {
    echo "first-use integration: explicit non-default Docker context is required" >&2
    return 1
  }
  command docker --context "$host_docker_context" "$@"
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

run_bare_tobari_pty_at() {
  local mode=$1
  local root=$2
	(
		cd "$root"
		# shellcheck disable=SC2016 # $PWD must expand inside the Workspace shell, not on the host.
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
import os
import pty
import select
import struct
import sys
import termios
import time

mode = sys.argv[1]
argv = sys.argv[2:]
pid, master = pty.fork()
if pid == 0:
    os.execvpe(argv[0], argv, os.environ)
fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", 40, 120, 0, 0))
os.set_blocking(master, False)
deadline = time.monotonic() + 1200
review = bytearray()
started = False
selected = False
entered = False
status = None
while status is None:
    if time.monotonic() >= deadline:
        os.kill(pid, 15)
        raise SystemExit("first-use PTY timed out")
    ready, _, _ = select.select([master], [], [], 0.1)
    if master in ready:
        try:
            data = os.read(master, 4096)
        except OSError as error:
            if error.errno == errno.EIO:
                data = b""
            else:
                raise
        if data:
            os.write(1, data)
            review.extend(data)
            if mode == "fresh" and not started and b"Create and enter Workspace" in review:
                os.write(master, b"\r")
                started = True
            if mode in ("ancestor-use", "nested-create") and not selected and b"Create a new Workspace here" in review:
                os.write(master, b"\r" if mode == "ancestor-use" else b"n")
                selected = True
            if b"E2E_CWD=" in review:
                entered = True
            if len(review) > 131072:
                del review[:-65536]
        elif mode == "fresh" and not started:
            raise SystemExit("first-use review closed before Start")
    waited, value = os.waitpid(pid, os.WNOHANG)
    if waited != 0:
        status = value
if mode == "fresh" and not started:
    raise SystemExit("first-use review was not observed")
if mode in ("ancestor-use", "nested-create") and not selected:
    raise SystemExit("ancestor selector was not observed")
if not entered:
    raise SystemExit("bare Tobari did not run the deterministic Workspace child")
if os.WIFEXITED(status):
    raise SystemExit(os.WEXITSTATUS(status))
raise SystemExit(128 + os.WTERMSIG(status))
' "$mode" "$binary" -- /bin/bash -lc "printf 'E2E_CWD=%s\\n' \"\$PWD\""
	)
}

cleanup() {
	local status=$?
	local cleanup_failed=false
	local cleanup_log=
	cleanup_step() {
		local label=$1
		shift
		cleanup_log=$test_root/cleanup-${label}.log
		if "$@" >"$cleanup_log" 2>&1; then return; fi
		if grep -F 'final_authority_mutation_interrupted' "$cleanup_log" >/dev/null; then
			mv -- "$cleanup_log" "${cleanup_log%.log}-interrupted.log"
			if "$@" >"$cleanup_log" 2>&1; then echo "first-use integration: cleanup step $label recovered exact interrupted mutation"; return; fi
		fi
		echo "first-use integration: cleanup step $label failed" >&2
		[[ ! -f ${cleanup_log%.log}-interrupted.log ]] || sed -n '1,240p' "${cleanup_log%.log}-interrupted.log" >&2
		sed -n '1,240p' "$cleanup_log" >&2
		cleanup_failed=true
	}
	trap - EXIT
	if [[ $status != 0 && ${TOBARI_INTEGRATION_PRESERVE_FAILURE:-false} == true ]]; then
		echo "first-use integration: preserved failed state at $test_root" >&2
		exit "$status"
	fi
	if [[ $owns_shared_resources == true ]]; then
		if [[ -n ${test_root:-} && -x ${binary:-} && -n ${nested_workspace_ref:-} ]]; then cleanup_step nested-workspace run_tobari workspace delete --id "$nested_workspace_ref" --confirm=delete --force; fi
		if [[ -n ${test_root:-} && -x ${binary:-} && -n ${upgrade_context_ref:-} ]]; then cleanup_step upgrade-context run_tobari context delete --id "$upgrade_context_ref" --confirm=delete; fi
		if [[ -n ${test_root:-} && -x ${binary:-} && -n ${workspace_ref:-} ]]; then cleanup_step workspace run_tobari workspace delete --id "$workspace_ref" --confirm=delete --force; fi
		if [[ -n ${test_root:-} && -x ${binary:-} && -n ${context_ref:-} ]]; then cleanup_step context run_tobari context delete --id "$context_ref" --confirm=delete; fi
		if [[ -n ${test_root:-} && -x ${binary:-} && -f $test_root/state/tobari/cluster-reconcile.json ]]; then cleanup_step cluster-up run_tobari cluster up; fi
		if [[ -n ${test_root:-} && -x ${binary:-} ]]; then cleanup_step cluster-down run_tobari cluster down --purge; fi
		if [[ -n ${transition_runtime_image_id:-} ]]; then cleanup_step transition-runtime-image docker image rm "$transition_runtime_image_id"; fi
	fi
	if [[ $status == 0 && $cleanup_failed == true ]]; then
		status=1
	fi
	if [[ $status != 0 && ${TOBARI_INTEGRATION_PRESERVE_FAILURE:-false} == true ]]; then
		echo "first-use integration: preserved cleanup failure at $test_root" >&2
		exit "$status"
	fi
	if [[ -n ${test_root:-} && $test_root == "$PWD"/.tobari-final-first-use.* ]]; then
		rm -rf -- "$test_root"
	fi
	exit "$status"
}
trap cleanup EXIT

[[ -n $host_docker_context && $host_docker_context != default ]] || {
  echo "first-use integration: TOBARI_INTEGRATION_DOCKER_CONTEXT must name an explicit non-default Docker context" >&2
  exit 1
}
docker context inspect "$host_docker_context" >/dev/null
docker version >/dev/null

if [[ -n $(docker ps -a --filter label=io.tobari.owner=default --format '{{.Names}}') ||
      -n $(docker network ls --filter label=io.tobari.owner=default --format '{{.Name}}') ||
      -n $(docker volume ls --filter label=io.tobari.owner=default --format '{{.Name}}') ]]; then
  echo "first-use integration: the explicit Docker context must contain no Tobari-owned resources" >&2
  exit 1
fi

for container in tobari-gateway tobari-opa; do
  if docker container inspect "$container" >/dev/null 2>&1; then
    echo "first-use integration: container $container must be absent" >&2
    exit 1
  fi
done
for network in tobari-control tobari-egress; do
  if docker network inspect "$network" >/dev/null 2>&1; then
    echo "first-use integration: network $network must be absent" >&2
    exit 1
  fi
done
for volume in tobari-gateway-ca tobari-public-ca tobari-policy-bundle; do
  if docker volume inspect "$volume" >/dev/null 2>&1; then
    echo "first-use integration: volume $volume must be absent" >&2
    exit 1
  fi
done
owns_shared_resources=true

if [[ -z $predecessor_binary ]]; then
	for image in "$standard_runtime_image" "$gateway_image"; do
		if docker image inspect "$image" >/dev/null 2>&1; then
			echo "first-use integration: Tobari image $image must be absent; use an isolated cold Docker context" >&2
			exit 1
		fi
	done
fi

# Keep bind-mounted fixtures under the checkout: remote Linux Docker engines
# such as Colima do not necessarily share the host's platform TMPDIR.
test_root=$(mktemp -d "$PWD/.tobari-final-first-use.XXXXXX")
mkdir -p "$test_root/user/project"
[[ ! -e $test_root/config && ! -e $test_root/state && ! -e $test_root/data ]] || {
  echo "first-use integration: XDG parent directories must not exist before the first command" >&2
  exit 1
}
if [[ -z $candidate_binary ]]; then
	candidate_binary=$test_root/tobari-candidate
	go build -buildvcs=false -trimpath -o "$candidate_binary" \
		-ldflags "-X main.commit=$(git rev-parse --verify HEAD)" ./cmd/tobari
fi
[[ -x $candidate_binary ]] || { echo "first-use integration: candidate binary is unavailable" >&2; exit 1; }
binary=$candidate_binary
if [[ -n $predecessor_binary ]]; then
	[[ -x $predecessor_binary ]] || { echo "first-use integration: predecessor binary is unavailable" >&2; exit 1; }
	binary=$predecessor_binary
fi

first_entry_log=$test_root/first-entry.log
if ! run_bare_tobari_pty_at fresh "$test_root/user/project" >"$first_entry_log" 2>&1; then
  sed -n '1,240p' "$first_entry_log" >&2
  echo "first-use integration: bare cold entry failed" >&2
  exit 1
fi
grep -F 'Create and enter Workspace' "$first_entry_log" >/dev/null || {
  echo "first-use integration: recommended review was not rendered" >&2
  exit 1
}
grep -F 'Prepare Workspace' "$first_entry_log" >/dev/null || {
  echo "first-use integration: Workspace preparation checkpoint was not rendered" >&2
  exit 1
}
if [[ -z $predecessor_binary ]]; then
	docker image inspect "$standard_runtime_image" "$gateway_image" >/dev/null
fi

template_list=$(run_tobari template list --format=json)
python3 -c 'import json,sys; items=json.load(sys.stdin)["templates"]["items"]; assert len(items) == 1; assert items[0]["runtime_id"] == "builtin/standard"' <<<"$template_list"
context_list=$(run_tobari context list --format=json)
context_ref=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["contexts"]["items"]; assert len(items) == 1; print(items[0]["context_ref"])' <<<"$context_list")
workspace_list=$(run_tobari workspace list --format=json)
workspace_ref=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["workspaces"]["items"]; assert len(items) == 1; print(items[0]["workspace_ref"])' <<<"$workspace_list")
prepare_release_upgrade_reentry "$context_list" "$workspace_list"
[[ -d $test_root/config/tobari/contexts ]] || {
  echo "first-use integration: Context source root was not created" >&2
  exit 1
}
post_create=$(run_tobari_at "$test_root/user/project" status --format=json)
post_create+=$(run_tobari context list --format=json)
post_create+=$(run_tobari_at "$test_root/user/project" doctor --format=json)
[[ $post_create != *legacy_state_present* && $post_create != *undeclared_fault_contract* ]] || {
  echo "first-use integration: Context draft poisoned final authority" >&2
  exit 1
}

for resource in tobari-gateway tobari-opa; do docker container inspect "$resource" >/dev/null; done
for resource in tobari-control tobari-egress; do docker network inspect "$resource" >/dev/null; done
for resource in tobari-gateway-ca tobari-public-ca tobari-policy-bundle; do
  [[ $(docker volume inspect --format '{{index .Labels "io.tobari.owner"}}' "$resource") == default ]]
done

status=$(run_tobari_at "$test_root/user/project" status --format=json)
python3 -c 'import json,sys; value=json.load(sys.stdin)["status"]; assert value["authority_state"] == "initialized"; assert value["workspace"]["presence"] == "present"; assert value["cluster"]["runtime"] == "running"' <<<"$status"

reentry_log=$test_root/reentry.log
if ! run_bare_tobari_pty_at reentry "$test_root/user/project" >"$reentry_log" 2>&1; then
	cat "$reentry_log" >&2
	echo "first-use integration: bare Workspace re-entry failed" >&2
	exit 1
fi
if grep -F 'cleanup needs attention' "$reentry_log" >/dev/null; then
	cat "$reentry_log" >&2
	echo "first-use integration: successful re-entry reported cleanup drift" >&2
	exit 1
fi
reentered=$(run_tobari_at "$test_root/user/project" status --format=json)
python3 -c 'import json,sys; value=json.load(sys.stdin)["status"]; assert value["workspace"]["presence"] == "present"; assert value["workspace"]["entry_state"] == "current"' <<<"$reentered"
python3 -c 'import json,sys; assert len(json.load(sys.stdin)["templates"]["items"]) == 1' <<<"$(run_tobari template list --format=json)"
python3 -c 'import json,sys; assert len(json.load(sys.stdin)["contexts"]["items"]) == 1' <<<"$(run_tobari context list --format=json)"
python3 -c 'import json,sys; assert len(json.load(sys.stdin)["workspaces"]["items"]) == 1' <<<"$(run_tobari workspace list --format=json)"
verify_release_upgrade_after_reentry

verify_workspace_entry_transition_matrix

mkdir -p "$test_root/user/project/child"
ancestor_log=$test_root/ancestor-entry.log
run_bare_tobari_pty_at ancestor-use "$test_root/user/project/child" >"$ancestor_log" 2>&1 || {
	cat "$ancestor_log" >&2; echo "first-use integration: descendant ancestor selection failed" >&2; exit 1;
}
grep -F 'Create a new Workspace here' "$ancestor_log" >/dev/null
grep -F 'Using existing Workspace' "$ancestor_log" >/dev/null
grep -F 'E2E_CWD=/var/lib/tobari/project/child' "$ancestor_log" >/dev/null
python3 -c 'import json,sys; assert len(json.load(sys.stdin)["contexts"]["items"]) == 1' <<<"$(run_tobari context list --format=json)"
python3 -c 'import json,sys; assert len(json.load(sys.stdin)["workspaces"]["items"]) == 1' <<<"$(run_tobari workspace list --format=json)"

parent_workspace_id=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["workspaces"]["items"]; assert len(items) == 1; print(items[0]["workspace_id"])' <<<"$(run_tobari workspace list --format=json)")
parent_container=$(docker ps -aq --no-trunc \
  --filter "label=io.tobari.owner=default" \
  --filter "label=io.tobari.role=work" \
  --filter "label=io.tobari.id=$parent_workspace_id")
[[ $parent_container =~ ^[0-9a-f]{64}$ ]] || {
  echo "first-use integration: exact parent Workspace container was not uniquely observable" >&2
  exit 1
}
docker stop "$parent_container" >/dev/null
[[ $(docker inspect --format '{{json .State.Running}}' "$parent_container") == false ]] || {
  echo "first-use integration: parent Workspace remained live before nested creation" >&2
  exit 1
}

nested_log=$test_root/nested-entry.log
run_bare_tobari_pty_at nested-create "$test_root/user/project/child" >"$nested_log" 2>&1 || {
	cat "$nested_log" >&2; echo "first-use integration: explicit nested-root creation failed" >&2; exit 1;
}
grep -F 'Creating a new Workspace here' "$nested_log" >/dev/null
grep -F 'E2E_CWD=/var/lib/tobari/project/child' "$nested_log" >/dev/null
context_list=$(run_tobari context list --format=json)
workspace_list=$(run_tobari workspace list --format=json)
nested_workspace_ref=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["workspaces"]["items"]; assert len(items) == 2; print(next(x["workspace_ref"] for x in items if x["project_root"].endswith("/project/child")))' <<<"$workspace_list")
nested_context_id=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["workspaces"]["items"]; assert len(items) == 2; print(next(x["context_id"] for x in items if x["project_root"].endswith("/project/child")))' <<<"$workspace_list")
python3 -c 'import json,sys; target=sys.argv[1]; items=json.load(sys.stdin)["contexts"]["items"]; assert len(items) == 1; assert items[0]["context_id"] == target; assert items[0]["workspace_count"] == 2' "$nested_context_id" <<<"$context_list"

echo "first-use integration: cold entry, re-entry, descendant selection, and stopped-ancestor sibling creation in one Context OK"
