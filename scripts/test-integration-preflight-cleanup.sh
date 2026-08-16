#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

fixture_root=$(mktemp -d "${TMPDIR:-/tmp}/tobari-integration-preflight.XXXXXXXX")
cleanup() { rm -rf -- "$fixture_root"; }
trap cleanup EXIT

mkdir -p "$fixture_root/bin"
ln -s "$PWD/scripts/testdata/fake-docker-integration-preflight.sh" "$fixture_root/bin/docker"
docker_log="$fixture_root/docker.log"
output="$fixture_root/output.log"
touch "$docker_log"

if ! grep -F -- '--file gateway/Dockerfile.experimental' scripts/test-integration.sh >/dev/null ||
  ! grep -F -- "--build-arg \"TOBARI_GATEWAY_BASE=\$experimental_gateway_base_image\"" scripts/test-integration.sh >/dev/null; then
  echo "integration fixture does not layer Broker modules into its experimental Gateway image" >&2
  exit 1
fi

set +e
PATH="$fixture_root/bin:$PATH" \
  TOBARI_TEST_DOCKER_LOG="$docker_log" \
  bash scripts/test-integration.sh >"$output" 2>&1
status=$?
set -e

if ((status == 0)); then
  echo "integration preflight fixture unexpectedly succeeded" >&2
  exit 1
fi
if ! grep -F "container tobari-auth-broker already exists" "$output" >/dev/null; then
  echo "integration preflight fixture did not reject the existing shared container" >&2
  cat "$output" >&2
  exit 1
fi
if ! grep -F "integration phase: preflight START" "$output" >/dev/null ||
  ! grep -F "integration: phase=preflight:" "$output" >/dev/null; then
  echo "integration preflight fixture did not report its named failure phase" >&2
  cat "$output" >&2
  exit 1
fi
if grep -E '^(rm|network rm|volume rm|image rm|run|exec)( |$)' "$docker_log" >/dev/null; then
  echo "integration preflight rejection attempted a Docker mutation" >&2
  cat "$docker_log" >&2
  exit 1
fi

echo "integration preflight cleanup ownership: OK"
