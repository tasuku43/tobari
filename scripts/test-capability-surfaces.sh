#!/usr/bin/env bash
# Compile the admitted capability-surface/resolver tuples and reject retired
# or incomplete tuples at the build boundary.
set -euo pipefail
cd "$(dirname "$0")/.."
export GO111MODULE=on
export GOENV=off
export GOEXPERIMENT=
export GOFIPS140=off
export GOFLAGS=
export GOTOOLCHAIN=local
export GOWORK=off

packages=(
  ./internal/domain/capabilitysurface
  ./internal/domain/buildidentity
  ./internal/cli
  ./internal/infra/dockerruntime
)

catalog_paths() {
  local tags=$1
  local output marker
  if [[ -n $tags ]]; then
    output=$(go test -tags="$tags" -run '^TestCatalogSurfaceSetIsExact$' -v ./internal/cli 2>&1)
  else
    output=$(go test -run '^TestCatalogSurfaceSetIsExact$' -v ./internal/cli 2>&1)
  fi
  marker=$(printf '%s\n' "$output" | sed -n 's/.*CATALOG_SURFACE_PATHS=//p' | tail -n 1)
  if [[ -z $marker ]]; then
    echo "catalog surface proof did not emit its path set for tags: ${tags:-default}" >&2
    printf '%s\n' "$output" >&2
    return 1
  fi
  printf '%s\n' "$marker" | tr '|' '\n' | sort
}

release_catalog_paths=$(mktemp)
release_dev_catalog_paths=$(mktemp)
research_catalog_paths=$(mktemp)
trap 'rm -f "$release_catalog_paths" "$release_dev_catalog_paths" "$research_catalog_paths"' EXIT
catalog_paths "" >"$release_catalog_paths"
catalog_paths "tobari_dev" >"$release_dev_catalog_paths"
catalog_paths "tobari_dev tobari_research" >"$research_catalog_paths"
if ! cmp -s "$release_catalog_paths" "$release_dev_catalog_paths"; then
  echo "release surface Catalog differs between embedded and development resolver tuples" >&2
  diff -u "$release_catalog_paths" "$release_dev_catalog_paths" >&2 || true
  exit 1
fi
common_research_paths=$(mktemp)
research_only_paths=$(mktemp)
trap 'rm -f "$release_catalog_paths" "$release_dev_catalog_paths" "$research_catalog_paths" "$common_research_paths" "$research_only_paths"' EXIT
comm -12 "$release_catalog_paths" "$research_catalog_paths" >"$common_research_paths"
comm -23 "$research_catalog_paths" "$release_catalog_paths" >"$research_only_paths"
expected_research_only=$'auth import\nauth login\nauth logout\nauth status\nserve\n'
if [[ $(cat "$research_only_paths")$'\n' != "$expected_research_only" ]]; then
  echo "research surface Catalog delta is not exactly auth×4 plus serve" >&2
  diff -u <(printf '%s' "$expected_research_only") "$research_only_paths" >&2 || true
  exit 1
fi
if ! cmp -s "$release_catalog_paths" "$common_research_paths"; then
  echo "research surface common Catalog differs from release surface common Catalog" >&2
  diff -u "$release_catalog_paths" "$common_research_paths" >&2 || true
  exit 1
fi
rm -f "$common_research_paths" "$research_only_paths"

go test "${packages[@]}"
go test -tags=tobari_dev "${packages[@]}"
go test -tags='tobari_dev tobari_research' "${packages[@]}"

assert_compile_failure() {
  local label=$1
  local tags=$2
  local diagnostic=$3
  local output
  local status=0
  output=$(mktemp)
  trap 'rm -f "$output"' RETURN
  if go test -tags="$tags" ./internal/domain/capabilitysurface >"$output" 2>&1; then
    echo "$label unexpectedly compiled with tags: $tags" >&2
    cat "$output" >&2
    return 1
  else
    status=$?
  fi
  if [[ $status -eq 0 ]] || ! grep -qF "$diagnostic" "$output"; then
    echo "$label failed without its task-owned diagnostic: $diagnostic" >&2
    cat "$output" >&2
    return 1
  fi
}

assert_compile_failure \
  "research surface without development resolver" \
  "tobari_research" \
  "undefined: tobari_research_requires_development_resolver"
assert_compile_failure \
  "retired tobari_experimental build tag" \
  "tobari_dev tobari_experimental" \
  "undefined: tobari_experimental_build_tag_is_retired"
assert_compile_failure \
  "retired tobari_experimental build tag with research" \
  "tobari_research tobari_experimental" \
  "undefined: tobari_experimental_build_tag_is_retired"

echo "capability surface tuple gate passed"
