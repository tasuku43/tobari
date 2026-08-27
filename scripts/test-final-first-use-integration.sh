#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "$0")/.."

host_docker_config=${DOCKER_CONFIG:-$HOME/.docker}
host_docker_context=${TOBARI_INTEGRATION_DOCKER_CONTEXT:-${DOCKER_CONTEXT:-}}
test_root=
binary=${TOBARI_FIRST_USE_INTEGRATION_BINARY:-}
owns_shared_resources=false
template_ref=
context_ref=
workspace_ref=

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

cleanup() {
  local status=$?
  trap - EXIT
  if [[ $owns_shared_resources == true ]]; then
    if [[ -n ${test_root:-} && -x ${binary:-} && -n ${workspace_ref:-} ]]; then
      run_tobari workspace delete --id "$workspace_ref" --confirm=delete --force >/dev/null 2>&1 || true
    fi
    if [[ -n ${test_root:-} && -x ${binary:-} && -n ${context_ref:-} ]]; then
      run_tobari context delete --id "$context_ref" --confirm=delete >/dev/null 2>&1 || true
    fi
    if [[ -n ${test_root:-} && -x ${binary:-} && -f $test_root/state/tobari/cluster-reconcile.json ]]; then
      run_tobari cluster up >/dev/null 2>&1 || true
    fi
    if [[ -n ${test_root:-} && -x ${binary:-} ]]; then
      run_tobari cluster down --purge >/dev/null 2>&1 || true
    fi
    docker rm -f tobari-gateway tobari-opa >/dev/null 2>&1 || true
    docker network rm tobari-control tobari-egress >/dev/null 2>&1 || true
    docker volume rm tobari-gateway-ca tobari-public-ca tobari-policy-bundle >/dev/null 2>&1 || true
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

# Keep bind-mounted fixtures under the checkout: remote Linux Docker engines
# such as Colima do not necessarily share the host's platform TMPDIR.
test_root=$(mktemp -d "$PWD/.tobari-final-first-use.XXXXXX")
mkdir -p "$test_root/user/project" "$test_root/config" "$test_root/state" "$test_root/data"
[[ ! -e $test_root/config/tobari && ! -e $test_root/state/tobari ]] || {
  echo "first-use integration: Tobari XDG roots must not exist before the first command" >&2
  exit 1
}
if [[ -z $binary ]]; then
  binary=$test_root/tobari
  DOCKER_CONTEXT="$host_docker_context" ./scripts/build-dev-images.sh
  go build -tags=tobari_dev -buildvcs=false -trimpath -o "$binary" \
    -ldflags "-X main.commit=$(git rev-parse --verify HEAD)" ./cmd/tobari
fi
[[ -x $binary ]] || { echo "first-use integration: binary is unavailable" >&2; exit 1; }

template_create=$(run_tobari template create --name default --source-access read-write --format=json)
template_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template"]["template_ref"])' <<<"$template_create")
template_plan=$(run_tobari template plan --id "$template_ref" --format=json)
template_plan_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template_change_plan"]["plan_ref"])' <<<"$template_plan")
template_apply=$(run_tobari template apply --plan "$template_plan_ref" --format=json)
python3 -c 'import json,sys; value=json.load(sys.stdin)["template"]; assert value["runtime_id"] == "builtin/standard"; assert value["changed"] is True' <<<"$template_apply"
run_tobari template default set --id "$template_ref" --format=json >/dev/null

context_create=$(run_tobari_at "$test_root/user/project" context create --template "$template_ref" --format=json)
context_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_ref"])' <<<"$context_create")
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

context_plan=$(run_tobari context plan --id "$context_ref" --format=json)
context_plan_ref=$(python3 -c 'import json,sys; value=json.load(sys.stdin)["context_activation_plan"]; assert value["runtime_id"] == "builtin/standard"; print(value["plan_ref"])' <<<"$context_plan")
run_tobari context apply --plan "$context_plan_ref" --format=json >/dev/null

cluster_up=$(run_tobari cluster up --format=json)
python3 -c 'import json,sys; value=json.load(sys.stdin)["cluster_up"]; assert value["applied"] is True; assert len(value["contexts"]) == 1' <<<"$cluster_up"
for resource in tobari-gateway tobari-opa; do docker container inspect "$resource" >/dev/null; done
for resource in tobari-control tobari-egress; do docker network inspect "$resource" >/dev/null; done
for resource in tobari-gateway-ca tobari-public-ca tobari-policy-bundle; do
  [[ $(docker volume inspect --format '{{index .Labels "io.tobari.owner"}}' "$resource") == default ]]
done

entry=$(run_tobari_at "$test_root/user/project" context enter --id "$context_ref" --format=json -- /bin/true 2>&1)
workspace_ref=$(python3 -c 'import json,sys; value=json.load(sys.stdin)["entry"]; assert value["exit_code"] == 0; print(value["workspace_ref"])' <<<"$entry")
workspaces=$(run_tobari workspace list --format=json)
python3 -c 'import json,sys; ref=sys.argv[1]; items=json.load(sys.stdin)["workspaces"]["items"]; assert [item["workspace_ref"] for item in items] == [ref]' "$workspace_ref" <<<"$workspaces"
status=$(run_tobari_at "$test_root/user/project" status --format=json)
python3 -c 'import json,sys; value=json.load(sys.stdin)["status"]; assert value["authority_state"] == "initialized"; assert value["workspace"]["presence"] == "present"; assert value["cluster"]["runtime"] == "running"' <<<"$status"

echo "first-use integration: empty XDG to builtin/standard Workspace entry OK"
