#!/usr/bin/env bash
# The guard intentionally matches literal shell and Docker source patterns.
# shellcheck disable=SC2016
set -euo pipefail
cd "$(dirname "$0")/.."

fail() {
  echo "runtime image identity: $*" >&2
  exit 1
}

dev_alias="tobari-runtime:"
dev_alias+="dev"
base_alias="tobari-runtime:"
base_alias+="base"

# These paths are executable/current authorities. Historical ADRs and the
# temporary packet may retain predecessor evidence, but neither may define a
# current selector or build dependency.
if git grep -nF "$dev_alias" -- internal scripts test tools Taskfile.yml >/dev/null 2>&1; then
  git grep -nF "$dev_alias" -- internal scripts test tools Taskfile.yml >&2
  fail "mutable standard Runtime alias remains in active code or build/test inputs"
fi
if git grep -nE "$base_alias([^[:alnum:]_-]|$)" -- internal scripts test tools Taskfile.yml >/dev/null 2>&1; then
	git grep -nE "$base_alias([^[:alnum:]_-]|$)" -- internal scripts test tools Taskfile.yml >&2
  fail "unversioned standard Runtime alias remains in active code or build/test inputs"
fi

grep -qF 'go run ./tools/runtimeassetid standard-runtime-image' scripts/build-dev-images.sh ||
  fail "development build does not consume canonical standard Runtime derivation"
grep -qF 'runtimeassets.StandardRuntimeImage()' internal/infra/dockerruntime/image_resolver_dev.go ||
  fail "development resolver does not consume canonical standard Runtime derivation"
grep -qF 'runtimeassets.StandardRuntimeImage()' internal/infra/dockerruntime/image_resolver_official.go ||
  fail "embedded resolver does not consume canonical standard Runtime derivation"
if git grep -nE 'localBaseRuntimeImage|base_source' -- scripts/package-release.sh >/dev/null 2>&1; then
  fail "release packaging still injects a Runtime image authority"
fi

grep -qF '"default_selector": "builtin"' docs/architecture-site/src/generated/component-versions.json ||
  fail "generated Runtime contract does not publish builtin as the stable selector"
grep -qF 'ARG TOBARI_RUNTIME_BASE' test/integration/custom-image.Dockerfile ||
  fail "custom Runtime fixture does not require explicit base material"
grep -qF 'FROM ${TOBARI_RUNTIME_BASE}' test/integration/custom-image.Dockerfile ||
  fail "custom Runtime fixture does not use explicit base material"
if grep -qF 'ARG TOBARI_RUNTIME_BASE=' test/integration/custom-image.Dockerfile; then
  fail "custom Runtime fixture provides a Docker default for the builtin selector"
fi

grep -qF 'custom_base_image == "$standard_runtime_image"' scripts/test-integration.sh ||
  fail "integration does not restrict local build to the canonical standard image"
grep -qF 'custom_base_image != "$standard_runtime_image"' scripts/test-integration.sh ||
  fail "integration does not reject missing explicit custom images"
grep -qF 'must name an existing compatible custom image' scripts/test-integration.sh ||
  fail "integration custom-image independence failure is not explicit"

echo "runtime image identity: source-addressed standard Runtime contract OK"
