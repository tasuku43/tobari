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
  echo "usage: $0 <fast|full|security|release|public>" >&2
  exit 2
}

preflight_commands() {
  local selected_profile=$1
  local -a required_commands=(go gofmt git)
  local -a missing_commands=()
  local command_name
  if [[ $selected_profile == release ]]; then
    required_commands+=(shellcheck tar unzip ruby)
  fi
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
  go run ./tools/archlint
  go run ./tools/contractlint
  go test ./...
}

run_security() {
  go mod verify
  go run ./tools/repoguard --scope security
  go run github.com/securego/gosec/v2/cmd/gosec@v2.27.1 -quiet ./...
  go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...
}

run_release() {
  ./scripts/lint-release.sh
  go run github.com/rhysd/actionlint/cmd/actionlint@v1.7.7
}

run_public() {
  go run ./tools/repoguard --scope public
  go run ./tools/contractlint
}

run_full() {
  run_fast
  go vet ./...
  go test -race ./...
  go mod tidy -diff
  git diff --check
}

case "$profile" in
  fast|full|security|release|public) ;;
  *) usage ;;
esac

preflight "$profile"

case "$profile" in
  fast) run_fast ;;
  full) run_full ;;
  security) run_security ;;
  release) run_release ;;
  public) run_public ;;
esac
