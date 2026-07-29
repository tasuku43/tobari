#!/usr/bin/env bash
# Validate release scripts and public binary distribution contracts without publishing.
set -euo pipefail
cd "$(dirname "$0")/.."
export GO111MODULE=on
export GOENV=off
export GOEXPERIMENT=
export GOFIPS140=off
export GOFLAGS=
export GOTOOLCHAIN=local
export GOWORK=off

bash -n \
  scripts/check.sh \
  scripts/package-release.sh \
  scripts/release-archive-entries.sh \
  scripts/render-formula.sh \
  scripts/audit-formula.sh \
  scripts/test-audit-formula.sh \
  scripts/test-check-environment.sh \
  scripts/test-release-archive-entries.sh \
  scripts/testdata/fake-go-gate-environment.sh \
  scripts/testdata/fake-brew.sh
if ! command -v shellcheck >/dev/null 2>&1; then
  echo "release gate requires ShellCheck for every repository shell script" >&2
  exit 1
fi
shellcheck_version=$(shellcheck --version | awk '$1 == "version:" { print $2 }')
if [[ ! $shellcheck_version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "could not determine a semantic ShellCheck version" >&2
  exit 1
fi
if ! awk -v current="$shellcheck_version" -v floor=0.9.0 'BEGIN {
  split(current, have, "."); split(floor, need, ".")
  for (i = 1; i <= 3; i++) {
    if ((have[i] + 0) > (need[i] + 0)) exit 0
    if ((have[i] + 0) < (need[i] + 0)) exit 1
  }
  exit 0
}'; then
  echo "release gate requires ShellCheck >= 0.9.0; running $shellcheck_version" >&2
  exit 1
fi
git ls-files -co --exclude-standard -z -- '*.sh' |
  while IFS= read -r -d '' script; do
    [[ -f $script ]] && printf '%s\0' "$script"
  done |
  xargs -0 shellcheck
go test ./tools/archivepack ./tools/internal/releaseversion ./tools/releaseversion
required_go=go$(awk '$1 == "go" { print $2 }' go.mod)
actual_go=$(go env GOVERSION)
if [[ $actual_go != "$required_go" ]]; then
  echo "release gate requires $required_go from go.mod; running $actual_go" >&2
  exit 1
fi
go mod verify >/dev/null
local_module_replacements=$(go list -m -f '{{if .Replace}}{{if not .Replace.Version}}{{.Path}} => {{.Replace.Dir}}{{end}}{{end}}' all)
if [[ -n $local_module_replacements ]]; then
  echo "release gate rejects local filesystem module replacements:" >&2
  printf '%s\n' "$local_module_replacements" >&2
  exit 1
fi
binary=$(go run ./tools/projectmeta --field binary_name)
module=$(go run ./tools/projectmeta --field go_module)
formula_class=$(go run ./tools/projectmeta --field formula_class)
template=Formula/${binary}.rb.template
test -f "$template"
archive_supporting_files=(LICENSE)
if [[ -e THIRD_PARTY_NOTICES || -L THIRD_PARTY_NOTICES ]]; then
  archive_supporting_files+=(THIRD_PARTY_NOTICES)
fi

for required in \
  '@@FORMULA_CLASS@@' '@@DESCRIPTION@@' '@@LICENSE_SPDX@@' '@@REPOSITORY_URL@@' '@@VERSION@@' \
  '@@MACOS_ARM64_URL@@' '@@MACOS_AMD64_URL@@' \
  '@@MACOS_ARM64_SHA256@@' '@@MACOS_AMD64_SHA256@@' '@@BINARY_NAME@@'; do
  grep -qF "$required" "$template" || {
    echo "Formula template is missing $required" >&2
    exit 1
  }
done

if grep -qF -- '--clobber' .github/workflows/release.yml; then
  echo "release workflow must never overwrite existing release assets" >&2
  exit 1
fi
grep -qF 'already exists; refusing to replace immutable release assets' .github/workflows/release.yml || {
  echo "release workflow does not fail closed when the tag already has a release" >&2
  exit 1
}
grep -A4 -F 'ref: main' .github/workflows/release.yml | grep -qF 'persist-credentials: false' || {
  echo "Formula checkout persists workflow credentials" >&2
  exit 1
}

for forbidden in 'git describe' '{{.VERSION}}' '{{.COMMIT}}'; do
  if grep -qF "$forbidden" Taskfile.yml; then
    echo "local build must not interpolate repository-controlled version metadata: $forbidden" >&2
    exit 1
  fi
done
grep -qF 'go build -buildvcs=false -trimpath -o bin/' Taskfile.yml || {
  echo "local build must use fixed dev metadata without implicit VCS stamping" >&2
  exit 1
}
for required in \
  'export GO111MODULE=on' 'export GOENV=off' 'export GOEXPERIMENT=' 'export GOFIPS140=off' \
  'export GOFLAGS=' 'export GOTOOLCHAIN=local' 'export GOWORK=off'; do
  for go_boundary in scripts/check.sh scripts/package-release.sh; do
    if ! grep -qFx "$required" "$go_boundary"; then
      echo "$go_boundary does not neutralize ambient Go configuration: $required" >&2
      exit 1
    fi
  done
done
scripts/test-check-environment.sh >/dev/null
scripts/test-release-archive-entries.sh >/dev/null

for forbidden in 'HOMEBREW_GITHUB_API_TOKEN' 'api.github.com/repos/' 'Authorization: Bearer'; do
  if grep -R -F "$forbidden" Formula scripts/render-formula.sh .github/workflows/release.yml >/dev/null 2>&1; then
    echo "public release path contains private-asset behavior: $forbidden" >&2
    exit 1
  fi
done

for required in \
  './scripts/check.sh full' './scripts/check.sh security' './scripts/check.sh release' \
  './scripts/check.sh public' './scripts/package-release.sh' 'checksums.txt' \
  'gh release create' 'Formula/' 'scripts/render-formula.sh'; do
  grep -qF "$required" .github/workflows/release.yml || {
    echo "release workflow is missing: $required" >&2
    exit 1
  }
done

formula_job=$(awk '
  /^  formula:/ { in_formula=1 }
  in_formula && !/^  formula:/ && /^  [A-Za-z0-9_-]+:/ { exit }
  in_formula { print }
' .github/workflows/release.yml)
build_job=$(awk '
  /^  build:/ { in_build=1 }
  in_build && !/^  build:/ && /^  [A-Za-z0-9_-]+:/ { exit }
  in_build { print }
' .github/workflows/release.yml)
release_revision_ref="ref: \${{ needs.preflight.outputs.revision }}"
formula_temp_ref="\${RUNNER_TEMP}/formula"
printf '%s\n' "$build_job" | grep -A4 -F "$release_revision_ref" | grep -qF 'persist-credentials: false' || {
  echo "matrix build checkout is not fixed to the credential-free preflight revision" >&2
  exit 1
}
for required in \
  "$release_revision_ref" \
  './scripts/render-formula.sh' 'ruby -c' './scripts/audit-formula.sh' \
  "$formula_temp_ref" 'ref: main' 'Stage audited Formula on main'; do
  if ! printf '%s\n' "$formula_job" | grep -qF "$required"; then
    echo "Formula job is missing its host-specific check: $required" >&2
    exit 1
  fi
done
printf '%s\n' "$formula_job" | grep -A4 -F "$release_revision_ref" | grep -qF 'persist-credentials: false' || {
  echo "exact release source checkout persists workflow credentials" >&2
  exit 1
}
if printf '%s\n' "$formula_job" | grep -qF './scripts/check.sh release'; then
  echo "Formula job must not repeat the Linux preflight release profile" >&2
  exit 1
fi
release_checkout_line=$(printf '%s\n' "$formula_job" | grep -n -m1 -F "$release_revision_ref" | cut -d: -f1)
render_line=$(printf '%s\n' "$formula_job" | grep -n -m1 -F './scripts/render-formula.sh' | cut -d: -f1)
audit_line=$(printf '%s\n' "$formula_job" | grep -n -m1 -F './scripts/audit-formula.sh' | cut -d: -f1)
main_checkout_line=$(printf '%s\n' "$formula_job" | grep -n -m1 -F 'ref: main' | cut -d: -f1)
stage_line=$(printf '%s\n' "$formula_job" | grep -n -m1 -F 'Stage audited Formula on main' | cut -d: -f1)
if ((release_checkout_line >= render_line || render_line >= audit_line || audit_line >= main_checkout_line || main_checkout_line >= stage_line)); then
  echo "Formula must be rendered and audited at the release revision before its output is staged on main" >&2
  exit 1
fi

if scripts/package-release.sh bad-tag 0000000000000000000000000000000000000000 linux amd64 dist >/dev/null 2>&1; then
  echo "package-release accepted an invalid tag" >&2
  exit 1
fi
ambient_status=0
ambient_output=$(env \
  GO111MODULE=off \
  GOENV=/definitely/missing/go.env \
  GOEXPERIMENT=definitely-invalid \
  GOFIPS140=definitely-invalid \
  GOFLAGS=-definitely-invalid \
  GOTOOLCHAIN=definitely-invalid \
  GOWORK=/definitely/missing/go.work \
  scripts/package-release.sh \
    v0.0.0 0000000000000000000000000000000000000000 plan9 amd64 dist 2>&1) || ambient_status=$?
if [[ $ambient_status -ne 2 || $ambient_output != *"unsupported release target: plan9/amd64"* ]]; then
  echo "package-release did not neutralize malicious ambient Go configuration" >&2
  printf '%s\n' "$ambient_output" >&2
  exit 1
fi
if go run ./tools/releaseversion v1.2.3-01 >/dev/null 2>&1; then
  echo "releaseversion accepted a numeric prerelease identifier with a leading zero" >&2
  exit 1
fi
if go run ./tools/releaseversion v1.2.3+different-build >/dev/null 2>&1; then
  echo "releaseversion accepted build metadata excluded by immutable-release policy" >&2
  exit 1
fi
if scripts/render-formula.sh v1.2.3-rc.1 https://github.com/tasuku43/agentic-cli-foundry /dev/null >/dev/null 2>&1; then
  echo "render-formula accepted a prerelease tag" >&2
  exit 1
fi

# Build one primary complete matrix for archive, checksum, and Formula checks.
# A second independent matrix below proves that identical inputs reproduce the
# exact archive bytes instead of merely reproducing their names and contents.
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum -- "$1" | awk '{ print $1 }'
    return
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 -- "$1" | awk '{ print $1 }'
    return
  fi
  echo "release gate requires sha256sum or shasum" >&2
  return 1
}

release_root=$(mktemp -d "${TMPDIR:-/tmp}/${binary}-release-check.XXXXXXXX")
cleanup() { rm -rf -- "$release_root"; }
trap cleanup EXIT
release_input_roots=(
  go.mod
  LICENSE
  .harness/project.json
  .github/workflows/release.yml
  Formula
  Taskfile.yml
  scripts
  cmd
  internal
  tools
)
if [[ -e THIRD_PARTY_NOTICES || -L THIRD_PARTY_NOTICES ]]; then
  release_input_roots+=(THIRD_PARTY_NOTICES)
fi
for optional_input in go.sum vendor; do
  if [[ -e $optional_input ]]; then
    release_input_roots+=("$optional_input")
  fi
done
release_input_fingerprint() {
  local diagnostic=${1:-}
  local manifest path_list unsafe_path_list path digest
  manifest=$(mktemp "$release_root/package-inputs.XXXXXXXX")
  path_list=$(mktemp "$release_root/package-input-paths.XXXXXXXX")
  unsafe_path_list=$(mktemp "$release_root/package-input-unsafe-paths.XXXXXXXX")
  : >"$manifest"
  find "${release_input_roots[@]}" ! -type d ! -type f -print0 >"$unsafe_path_list"
  if [[ -s $unsafe_path_list ]]; then
    echo "release inputs must contain only regular files and directories" >&2
    return 1
  fi
  find "${release_input_roots[@]}" -type f -print0 | LC_ALL=C sort -z >"$path_list"
  if [[ ! -s $path_list ]]; then
    echo "release input set is empty" >&2
    return 1
  fi
  if [[ -n $diagnostic ]]; then
    : >"$diagnostic"
  fi
  while IFS= read -r -d '' path; do
    digest=$(sha256_of "$path")
    printf '%s\0%s\0' "$path" "$digest" >>"$manifest"
    if [[ -n $diagnostic ]]; then
      printf '%s  %q\n' "$digest" "$path" >>"$diagnostic"
    fi
  done <"$path_list"
  digest=$(sha256_of "$manifest")
  rm -f -- "$manifest" "$path_list" "$unsafe_path_list"
  printf '%s' "$digest"
}

report_release_input_drift() {
  local phase=$1
  local before=$2
  local after=$3
  echo "release inputs changed $phase:" >&2
  diff -u "$before" "$after" >&2 || true
}

dist=$release_root/dist
primary_go_cache=$release_root/go-cache-primary
reproduction_go_cache=$release_root/go-cache-reproduction
mkdir -p "$dist" "$primary_go_cache" "$reproduction_go_cache"
release_tag=v0.0.0
release_revision=0000000000000000000000000000000000000000
targets=(
  linux/amd64/tar.gz
  linux/arm64/tar.gz
  darwin/amd64/tar.gz
  darwin/arm64/tar.gz
  windows/amd64/zip
)
expected_assets=$release_root/expected-assets.txt
: >"$expected_assets"
primary_inputs_before=$release_root/primary-inputs-before.txt
primary_inputs_after=$release_root/primary-inputs-after.txt
inputs_before_primary=$(release_input_fingerprint "$primary_inputs_before")
go mod verify >/dev/null

for target in "${targets[@]}"; do
  goos=${target%%/*}
  remainder=${target#*/}
  goarch=${remainder%%/*}
  extension=${target##*/}
  asset=${binary}_${release_tag}_${goos}_${goarch}.${extension}
  executable=$binary
  if [[ $goos == windows ]]; then
    executable=${binary}.exe
  fi

  env GOCACHE="$primary_go_cache" scripts/package-release.sh \
    "$release_tag" "$release_revision" "$goos" "$goarch" "$dist" >/dev/null
  archive=$dist/$asset
  test -s "$archive"
  printf '%s\n' "$asset" >>"$expected_assets"

  expected_members=$(printf '%s\n' "${archive_supporting_files[@]}" "$executable")
  if [[ $extension == zip ]]; then
    members=$(unzip -Z1 "$archive")
  else
    members=$(tar -tzf "$archive")
  fi
  if [[ $members != "$expected_members" ]]; then
    echo "archive $asset contains unexpected entries: $members" >&2
    exit 1
  fi

  extract_dir=$release_root/extract-${goos}-${goarch}
  mkdir -p "$extract_dir"
  if [[ $extension == zip ]]; then
    unzip -q "$archive" -d "$extract_dir"
  else
    tar -xzf "$archive" -C "$extract_dir"
  fi
  expected_file_count=$((${#archive_supporting_files[@]} + 1))
  if [[ $(find "$extract_dir" -type f | wc -l | tr -d ' ') -ne $expected_file_count || ! -f $extract_dir/$executable ]]; then
    echo "archive $asset did not extract to its exact executable and supporting-file set" >&2
    exit 1
  fi
  for supporting_file in "${archive_supporting_files[@]}"; do
    if [[ ! -f $extract_dir/$supporting_file ]] || ! cmp -s "$supporting_file" "$extract_dir/$supporting_file"; then
      echo "archive $asset does not contain the reviewed $supporting_file bytes" >&2
      exit 1
    fi
  done
  metadata=$(go version -m "$extract_dir/$executable")
  for required_metadata in "$module" "GOOS=$goos" "GOARCH=$goarch"; do
    if ! printf '%s\n' "$metadata" | grep -Fq "$required_metadata"; then
      echo "archive $asset is missing build metadata: $required_metadata" >&2
      exit 1
    fi
  done
done
if ! go mod verify >/dev/null; then
  echo "module inputs changed or failed verification during the primary archive pass" >&2
  exit 1
fi
inputs_after_primary=$(release_input_fingerprint "$primary_inputs_after")
if [[ $inputs_before_primary != "$inputs_after_primary" ]]; then
  report_release_input_drift "during the primary archive pass; reproducibility comparison was not attempted" "$primary_inputs_before" "$primary_inputs_after"
  exit 1
fi

LC_ALL=C sort -o "$expected_assets" "$expected_assets"
actual_assets=$release_root/actual-assets.txt
find "$dist" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort >"$actual_assets"
if ! cmp -s "$expected_assets" "$actual_assets"; then
  echo "release archive set does not match the supported five-target matrix" >&2
  exit 1
fi

repro_dist=$release_root/repro-dist
mkdir -p "$repro_dist"
reproduction_inputs_before=$release_root/reproduction-inputs-before.txt
reproduction_inputs_after=$release_root/reproduction-inputs-after.txt
inputs_before_reproduction=$(release_input_fingerprint "$reproduction_inputs_before")
if [[ $inputs_after_primary != "$inputs_before_reproduction" ]]; then
  report_release_input_drift "before the reproduction archive pass" "$primary_inputs_after" "$reproduction_inputs_before"
  exit 1
fi
go mod verify >/dev/null
for target in "${targets[@]}"; do
  goos=${target%%/*}
  remainder=${target#*/}
  goarch=${remainder%%/*}
  extension=${target##*/}
  asset=${binary}_${release_tag}_${goos}_${goarch}.${extension}

  env GOCACHE="$reproduction_go_cache" scripts/package-release.sh \
    "$release_tag" "$release_revision" "$goos" "$goarch" "$repro_dist" >/dev/null
done
if ! go mod verify >/dev/null; then
  echo "module inputs changed or failed verification during the reproduction archive pass" >&2
  exit 1
fi
inputs_after_reproduction=$(release_input_fingerprint "$reproduction_inputs_after")
if [[ $inputs_before_reproduction != "$inputs_after_reproduction" ]]; then
  report_release_input_drift "during the reproduction archive pass; digest comparison is invalid" "$reproduction_inputs_before" "$reproduction_inputs_after"
  exit 1
fi
for target in "${targets[@]}"; do
  goos=${target%%/*}
  remainder=${target#*/}
  goarch=${remainder%%/*}
  extension=${target##*/}
  asset=${binary}_${release_tag}_${goos}_${goarch}.${extension}

  primary_digest=$(sha256_of "$dist/$asset")
  reproduced_digest=$(sha256_of "$repro_dist/$asset")
  if [[ $primary_digest != "$reproduced_digest" ]]; then
    echo "release archive is not byte-for-byte reproducible: $asset" >&2
    exit 1
  fi
done
repro_assets=$release_root/repro-assets.txt
find "$repro_dist" -maxdepth 1 -type f -exec basename {} \; | LC_ALL=C sort >"$repro_assets"
if ! cmp -s "$expected_assets" "$repro_assets"; then
  echo "reproduced archive set does not match the supported five-target matrix" >&2
  exit 1
fi

# The package command is create-only. This negative check reaches the collision
# guard before another build, so the two verified matrices above remain the
# only builds performed by this profile.
first_asset=$dist/${binary}_${release_tag}_linux_amd64.tar.gz
first_digest_before=$(sha256_of "$first_asset")
if scripts/package-release.sh "$release_tag" "$release_revision" linux amd64 "$dist" >/dev/null 2>&1; then
  echo "package-release overwrote an existing archive" >&2
  exit 1
fi
first_digest_after=$(sha256_of "$first_asset")
if [[ $first_digest_before != "$first_digest_after" ]]; then
  echo "package-release changed an existing archive on collision" >&2
  exit 1
fi

checksums=$dist/checksums.txt
: >"$checksums"
while IFS= read -r asset; do
  printf '%s  %s\n' "$(sha256_of "$dist/$asset")" "$asset" >>"$checksums"
done <"$expected_assets"
if [[ $(wc -l <"$checksums" | tr -d ' ') -ne 5 ]]; then
  echo "checksums.txt does not contain exactly five archives" >&2
  exit 1
fi
checksum_assets=$release_root/checksum-assets.txt
awk '{ print $2 }' "$checksums" | LC_ALL=C sort >"$checksum_assets"
if ! cmp -s "$expected_assets" "$checksum_assets"; then
  echo "checksums.txt does not correspond to the complete archive set" >&2
  exit 1
fi
while read -r digest asset extra; do
  if [[ -n ${extra:-} ]] || ! printf '%s' "$digest" | grep -Eq '^[0-9a-f]{64}$'; then
    echo "invalid checksum record for $asset" >&2
    exit 1
  fi
  if [[ $digest != "$(sha256_of "$dist/$asset")" ]]; then
    echo "checksum mismatch for $asset" >&2
    exit 1
  fi
done <"$checksums"

formula=$release_root/${binary}.rb
repository_url=https://github.com/tasuku43/agentic-cli-foundry
scripts/render-formula.sh "$release_tag" "$repository_url" "$checksums" "$formula" >/dev/null
test -s "$formula"
arm64_asset=${binary}_${release_tag}_darwin_arm64.tar.gz
amd64_asset=${binary}_${release_tag}_darwin_amd64.tar.gz
arm64_sha=$(awk -v asset="$arm64_asset" '$2 == asset { print $1 }' "$checksums")
amd64_sha=$(awk -v asset="$amd64_asset" '$2 == asset { print $1 }' "$checksums")
for expected_formula in \
  "class $formula_class < Formula" \
  "version \"${release_tag#v}\"" \
  "$repository_url/releases/download/$release_tag/$arm64_asset" \
  "$repository_url/releases/download/$release_tag/$amd64_asset" \
  "sha256 \"$arm64_sha\"" \
  "sha256 \"$amd64_sha\""; do
  if ! grep -Fq "$expected_formula" "$formula"; then
    echo "rendered Formula is missing: $expected_formula" >&2
    exit 1
  fi
done
if grep -qE '@@[A-Z0-9_]+@@' "$formula"; then
  echo "positive Formula render retained a placeholder" >&2
  exit 1
fi
if ! command -v ruby >/dev/null 2>&1; then
  echo "release gate requires Ruby for Formula syntax validation; install Ruby or use the documented CI release gate" >&2
  exit 1
fi
ruby -c "$formula" >/dev/null

scripts/test-audit-formula.sh >/dev/null

release_checks_inputs_after=$release_root/release-checks-inputs-after.txt
inputs_after_release_checks=$(release_input_fingerprint "$release_checks_inputs_after")
if [[ $inputs_after_reproduction != "$inputs_after_release_checks" ]]; then
  report_release_input_drift "during checksum or Formula validation" "$reproduction_inputs_after" "$release_checks_inputs_after"
  exit 1
fi

echo "lint-release: OK"
