#!/usr/bin/env bash
# This is the only implementation of repository quality gates. Task, agent
# Task, optional local automation, CI, and release workflows call a named profile here.
set -euo pipefail
cd "$(dirname "$0")/.."
export GO111MODULE=on
export GOENV=off
export GOEXPERIMENT=
export GOFIPS140=off
export GOFLAGS=
export GOTOOLCHAIN=local
export GOWORK=off

profile=${1:-}

usage() {
  echo "usage: $0 <fast|full|security|release|public|policy|gateway|authbroker|integration|runtime>" >&2
  exit 2
}

preflight_commands() {
  local selected_profile=$1
  local -a required_commands=(go gofmt git)
  local -a missing_commands=()
  local command_name
  if [[ $selected_profile == fast || $selected_profile == full ]]; then
    required_commands+=(python3 node npm)
  fi
  if [[ $selected_profile == gateway || $selected_profile == runtime ]]; then
    required_commands+=(python3)
  fi
  if [[ $selected_profile == security || $selected_profile == public ]]; then
    required_commands+=(node npm)
  fi
  if [[ $selected_profile == release ]]; then
    required_commands+=(shellcheck tar unzip ruby)
  fi
  case "$selected_profile" in
    policy|gateway|authbroker|integration|runtime) required_commands+=(docker) ;;
  esac
  for command_name in "${required_commands[@]}"; do
    if ! command -v "$command_name" >/dev/null 2>&1; then
      missing_commands+=("$command_name")
    fi
  done
  if [[ $selected_profile == release ]]; then
    if ! command -v sha256sum >/dev/null 2>&1 && ! command -v shasum >/dev/null 2>&1; then
      missing_commands+=("sha256sum-or-shasum")
    fi
  fi
  if ((${#missing_commands[@]} != 0)); then
    echo "check preflight: missing required local tools for $selected_profile: ${missing_commands[*]}" >&2
    echo "Install the listed tools, or use the documented CI gate, before rerunning ./scripts/check.sh $selected_profile." >&2
    return 1
  fi
}

preflight_node_toolchain() {
  local required_node required_npm reported_node reported_npm
  required_node=$(tr -d '[:space:]' <.node-version)
  required_npm=$(sed -n 's/.*"packageManager": "npm@\([^"]*\)".*/\1/p' docs/architecture-site/package.json)
  reported_node=$(node --version)
  reported_npm=$(npm --version)
  if [[ $reported_node == "v$required_node" && $reported_npm == "$required_npm" ]]; then
    return 0
  fi
  cat >&2 <<EOF
check preflight: Node.js toolchain mismatch
  required (.node-version): v$required_node
  actual: $reported_node
  required npm (site packageManager): $required_npm
  actual: $reported_npm
Select the repository-pinned Node.js toolchain and npm version, then rerun ./scripts/check.sh $profile.
EOF
  return 1
}

preflight_go_toolchain() {
  local required_go go_binary version_output reported_version env_version go_root go_tool_dir
  local compiler_output compiler_version
  required_go=go$(awk '$1 == "go" { print $2; found=1; exit } END { if (!found) exit 1 }' go.mod) || {
    echo "check preflight: go.mod does not declare a required Go version" >&2
    return 1
  }
  go_binary=$(command -v go)
  version_output=$(go version 2>&1) || {
    local status=$?
    echo "check preflight: unable to query the local Go binary at $go_binary" >&2
    printf '%s\n' "$version_output" >&2
    return "$status"
  }
  reported_version=$(printf '%s\n' "$version_output" | awk 'NR == 1 { print $3 }')
  env_version=$(go env GOVERSION 2>&1) || {
    echo "check preflight: unable to query GOVERSION from $go_binary" >&2
    printf '%s\n' "$env_version" >&2
    return 1
  }
  go_root=$(go env GOROOT 2>&1) || {
    echo "check preflight: unable to query GOROOT from $go_binary" >&2
    printf '%s\n' "$go_root" >&2
    return 1
  }
  go_tool_dir=$(go env GOTOOLDIR 2>&1) || {
    echo "check preflight: unable to query GOTOOLDIR from $go_binary" >&2
    printf '%s\n' "$go_tool_dir" >&2
    return 1
  }
  compiler_output=unavailable
  compiler_version=unavailable
  if [[ -x $go_tool_dir/compile ]]; then
    compiler_output=$("$go_tool_dir/compile" -V=full 2>&1) || compiler_output=unavailable
    compiler_version=$(printf '%s\n' "$compiler_output" | awk 'NR == 1 { print $3 }')
  fi

  local tool_dir_matches_root=false
  if [[ $go_tool_dir == "$go_root"/* ]]; then
    tool_dir_matches_root=true
  fi
  if [[ $reported_version == "$required_go" && $env_version == "$required_go" &&
    $compiler_version == "$required_go" && $tool_dir_matches_root == true ]]; then
    return 0
  fi

  cat >&2 <<EOF
check preflight: Go toolchain mismatch
  required (go.mod): $required_go
  binary: $go_binary
  go version: $version_output
  go env GOVERSION: $env_version
  GOROOT: $go_root
  GOTOOLDIR: $go_tool_dir
  compiler: $compiler_output
The gate sets GOTOOLCHAIN=local. Install Go ${required_go#go}, put that installation's bin directory first on PATH, and clear a stale GOROOT if it names another installation. If using mise, select go@${required_go#go} for this repository or shell. Then rerun ./scripts/check.sh $profile.
EOF
  return 1
}

preflight() {
  local selected_profile=$1
  local failed=0
  local go_status=0
  preflight_commands "$selected_profile" || failed=1
  if command -v go >/dev/null 2>&1; then
    preflight_go_toolchain || go_status=$?
    if ((go_status != 0)); then
      if ((go_status != 1)); then
        return "$go_status"
      fi
      failed=1
    fi
  fi
  if [[ $selected_profile == fast || $selected_profile == full ||
    $selected_profile == security || $selected_profile == public ]]; then
    if command -v node >/dev/null 2>&1 && command -v npm >/dev/null 2>&1; then
      preflight_node_toolchain || failed=1
    fi
  fi
  return "$failed"
}

run_fast() {
  local unformatted
  unformatted=$(gofmt -l .)
  if [[ -n "$unformatted" ]]; then
    echo "gofmt is required for:" >&2
    echo "$unformatted" >&2
    return 1
  fi
  go run ./tools/repoguard --scope hygiene
  ./scripts/test-decision-records.sh
  go run ./tools/archlint
  go run ./tools/contractlint
  python3 scripts/protocol_classifier_admission.py
  python3 scripts/test-protocol-classifier-admission.py
  python3 scripts/test-pty-evidence.py
  ./scripts/test-integration-preflight-cleanup.sh
  ./scripts/check-integration-scope.sh
  ./scripts/test-site-workflow-ownership.sh
  ./scripts/check-runtime-base.sh
  ./scripts/check-gateway-source.sh
  ./scripts/check-authbroker-source.sh
  ./scripts/check-authbroker-image.sh
  ./scripts/test-capability-surfaces.sh
  ./scripts/check-site.sh fast
  go test ./...
	go test -tags='tobari_dev tobari_research' ./internal/app/authcmd ./internal/cli ./internal/domain/authbroker ./internal/domain/buildidentity ./internal/domain/capabilitysurface ./internal/infra/authproviders ./internal/infra/dockerruntime
}

run_security() {
  go mod verify
  go run ./tools/repoguard --scope security
  go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 \
    -exclude-dir=internal/infra/runtimeassets/_helper-source -quiet ./...
  go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
  ./scripts/check-site.sh security
}

run_release() {
	require_embedded_gateway_release_contract
	./scripts/lint-release.sh
  go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
}

run_public() {
  go run ./tools/repoguard --scope public
  go run ./tools/contractlint
	require_embedded_gateway_release_contract
  ./scripts/check-site.sh public
}

load_runtime_versions() {
  # shellcheck disable=SC1091
  source internal/infra/runtimeassets/assets/versions.env
	[[ -n ${OPA_IMAGE:-} && -n ${MITMPROXY_IMAGE:-} && -n ${DEBIAN_IMAGE:-} ]] || {
    echo "runtime image references are incomplete" >&2
    return 1
  }
}

require_embedded_gateway_release_contract() {
	load_runtime_versions
	if grep -Eq '^(GATEWAY|AUTH_BROKER)_IMAGE(_API)?=' internal/infra/runtimeassets/assets/versions.env; then
		echo "Tobari-owned release outputs must not be committed to versions.env" >&2
		return 1
	fi
	go test ./internal/infra/runtimeassets ./internal/infra/dockerruntime
	if grep -R -F 'component-lock.json' .github/workflows/release.yml scripts/package-release.sh tools/releaseartifacts >/dev/null 2>&1; then
		echo "release path still depends on a published component lock" >&2
		return 1
	fi
	grep -qF 'runtimeassets.ComponentVersion("gateway")' internal/infra/dockerruntime/image_resolver_official.go
	grep -qF 'BuildIfMissing: true' internal/infra/dockerruntime/image_resolver_official.go
}

run_policy() {
  load_runtime_versions
  docker version >/dev/null
  docker run --rm \
    -v "$PWD/internal/infra/runtimeassets/assets/opa/policy:/policy:ro" \
    "$OPA_IMAGE" fmt --fail /policy/tobari.rego /policy/tobari_test.rego >/dev/null
  docker run --rm \
    -v "$PWD/internal/infra/runtimeassets/assets/opa/policy:/policy:ro" \
    "$OPA_IMAGE" test /policy -v
}

run_gateway() {
  python3 scripts/protocol_classifier_admission.py
  python3 scripts/test-protocol-classifier-admission.py
  load_runtime_versions
  docker version >/dev/null
  local gateway_test_image
  gateway_test_image=$(docker build --quiet --build-arg "MITMPROXY_IMAGE=$MITMPROXY_IMAGE" gateway)
  docker run --rm \
    --entrypoint python \
    -e PYTHONPATH=/work/addon \
    -e TOBARI_GATEWAY_CONFIG=/work/config.example.json \
    -v "$PWD/gateway:/work:ro" \
    -w /work \
    "$gateway_test_image" \
    -m unittest -v \
      test_tobari_gateway.py \
      test_tobari_gateway_aws.py \
      test_aws_request.py \
      test_graphql_request.py \
      test_git_request.py \
      test_kubernetes_request.py \
      test_oci_request.py \
      test_mcp_request.py \
      test_synthetic_dns.py
}

run_authbroker() {
  load_runtime_versions
  ./scripts/check-authbroker-source.sh
  ./scripts/check-authbroker-image.sh
  docker version >/dev/null
  docker run --rm --read-only --network none \
    --tmpfs /tmp:rw,noexec,nosuid,size=32m \
    -v "$PWD:/workspace:ro" \
    -w /workspace \
    "$MITMPROXY_IMAGE" \
    python3 -m unittest discover -s authbroker/tests -v
}

run_integration() {
  local integration_context=${TOBARI_INTEGRATION_DOCKER_CONTEXT:-${DOCKER_CONTEXT:-}}
  [[ -n $integration_context && $integration_context != default ]] || {
    echo "check integration: TOBARI_INTEGRATION_DOCKER_CONTEXT must name an explicit non-default Docker context" >&2
    return 1
  }
  docker --context "$integration_context" version >/dev/null
  export DOCKER_CONTEXT="$integration_context"
  export TOBARI_INTEGRATION_DOCKER_CONTEXT="$integration_context"
  ./scripts/test-integration.sh
}

run_runtime() {
  run_policy
  run_gateway
  run_authbroker
  run_integration
}

run_full() {
  run_fast
  ./scripts/check-site.sh browser
  go vet ./...
  go test -race ./...
  go mod tidy -diff
  git diff --check
}

case "$profile" in
  fast|full|security|release|public|policy|gateway|authbroker|integration|runtime) ;;
  *) usage ;;
esac

preflight "$profile"

case "$profile" in
  fast) run_fast ;;
  full) run_full ;;
  security) run_security ;;
  release) run_release ;;
  public) run_public ;;
  policy) run_policy ;;
  gateway) run_gateway ;;
  authbroker) run_authbroker ;;
  integration) run_integration ;;
  runtime) run_runtime ;;
esac
