#!/usr/bin/env bash
# Prove that the canonical gate neutralizes hostile ambient Go configuration
# before its first Go-backed public check.
set -euo pipefail
cd "$(dirname "$0")/.."

fixture_root=$(mktemp -d "${TMPDIR:-/tmp}/agentic-cli-foundry-go-environment.XXXXXXXX")
mismatch_root=
cleanup() {
  rm -rf -- "$fixture_root"
  if [[ -n $mismatch_root ]]; then
    rm -rf -- "$mismatch_root"
  fi
}
trap cleanup EXIT
cp scripts/testdata/fake-go-gate-environment.sh "$fixture_root/go"
chmod 0700 "$fixture_root/go"

status=0
output=$(env \
  PATH="$fixture_root:$PATH" \
  GO111MODULE=off \
  GOENV=/definitely/missing/go.env \
  GOEXPERIMENT=definitely-invalid \
  GOFIPS140=definitely-invalid \
  GOFLAGS=-run=NoTests \
  GOTOOLCHAIN=definitely-invalid \
  GOWORK=/definitely/missing/go.work \
  scripts/check.sh public 2>&1) || status=$?

if [[ $status -ne 73 || $output != *"canonical gate reached Go with a sanitized environment"* ]]; then
  echo "canonical gate did not sanitize ambient Go configuration before its first Go check" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi

echo "test-check-environment: OK"

mismatch_root=$(mktemp -d "${TMPDIR:-/tmp}/agentic-cli-foundry-go-mismatch.XXXXXXXX")
cp scripts/testdata/fake-go-toolchain-mismatch.sh "$mismatch_root/go"
mkdir -p "$mismatch_root/fake-goroot/pkg/tool/darwin_arm64"
cp scripts/testdata/fake-go-toolchain-mismatch.sh "$mismatch_root/fake-goroot/pkg/tool/darwin_arm64/compile"
chmod 0700 "$mismatch_root/go" "$mismatch_root/fake-goroot/pkg/tool/darwin_arm64/compile"

status=0
output=$(env \
  PATH="$mismatch_root:$PATH" \
  FAKE_GO_ROOT="$mismatch_root/fake-goroot" \
  scripts/check.sh public 2>&1) || status=$?

if [[ $status -ne 1 || $(grep -cF "check preflight: Go toolchain mismatch" <<<"$output") -ne 1 ]]; then
  echo "canonical gate did not collapse a mixed Go installation to one diagnostic" >&2
  printf '%s\n' "$output" >&2
  exit 1
fi
for expected in \
  "required (go.mod): go1.26.5" \
  "binary: $mismatch_root/go" \
  "go version: go version go1.26.5 darwin/arm64" \
  "go env GOVERSION: go1.26.5" \
  "GOROOT: $mismatch_root/fake-goroot" \
  "GOTOOLDIR: $mismatch_root/fake-goroot/pkg/tool/darwin_arm64" \
  "compiler: compile version go1.26.3" \
  "GOTOOLCHAIN=local" \
  "select go@1.26.5"; do
  if [[ $output != *"$expected"* ]]; then
    echo "mixed Go diagnostic is missing: $expected" >&2
    printf '%s\n' "$output" >&2
    exit 1
  fi
done

echo "test-check-toolchain-mismatch: OK"
