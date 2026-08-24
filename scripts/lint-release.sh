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
  scripts/build-dev-images.sh \
  scripts/check.sh \
  scripts/test-capability-surfaces.sh \
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
go test ./tools/archivepack ./tools/internal/releaseversion ./tools/releaseversion ./tools/releaseartifacts
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
if grep -qE '^  push:' .github/workflows/release.yml; then
  echo "release workflow must not publish implicitly from a pushed tag" >&2
  exit 1
fi
for component_workflow in .github/workflows/gateway-image.yml .github/workflows/authbroker-image.yml; do
  if grep -qE '^  push:' "$component_workflow"; then
    echo "$component_workflow must not publish implicitly from a main-branch push" >&2
    exit 1
  fi
  for required in 'workflow_dispatch:' 'revision:' 'type=cacheonly'; do
    if ! grep -qF -- "$required" "$component_workflow"; then
      echo "$component_workflow is missing protected component evidence: $required" >&2
      exit 1
    fi
  done
  for forbidden in 'packages: write' 'docker login ghcr.io' '--push'; do
    if grep -qF -- "$forbidden" "$component_workflow"; then
      echo "$component_workflow retains a GHCR publication path: $forbidden" >&2
      exit 1
    fi
  done
done
# Auth Broker remains validation-only. The standard Release workflow must not
# reference its image workflow, Dockerfile, repository, evidence, or lock API.
for forbidden in 'authbroker-image' 'authbroker/Dockerfile' 'tobari/auth-broker' 'auth-broker.component.json' 'auth-broker-api'; do
  if grep -qF -- "$forbidden" .github/workflows/release.yml; then
    echo "release workflow retains experimental Auth Broker publication input: $forbidden" >&2
    exit 1
  fi
done
# The Workspace helpers are built into the local engine-native base image. Host
# release archives retain exactly one executable and must never build or add the
# helpers as additional release members.
for release_boundary in scripts/package-release.sh scripts/release-archive-entries.sh .github/workflows/release.yml; do
  if grep -qE 'tobari-(expose|permission)' "$release_boundary"; then
    echo "$release_boundary packages the engine-native Workspace helper as a host release artifact" >&2
    exit 1
  fi
done
validation_workflow=.github/workflows/runtime-base.yml
for forbidden in 'packages: write' 'docker login ghcr.io' '--push'; do
  if grep -qF -- "$forbidden" "$validation_workflow"; then
    echo "$validation_workflow retains a GHCR publication path: $forbidden" >&2
    exit 1
  fi
done
if grep -R -F 'packages: write' .github/workflows >/dev/null 2>&1; then
	echo "no workflow may receive package-write permission" >&2
	exit 1
fi
for forbidden in \
  'ghcr.io/tasuku43/tobari/' 'docker login ghcr.io' '--push' \
  'component-lock' 'build-release-components' 'tools/componentlock' \
  'runtimecheck --require-publishable' 'Publish three components'; do
  if grep -qF -- "$forbidden" .github/workflows/release.yml scripts/package-release.sh; then
		echo "release publication still includes retired OCI authority: $forbidden" >&2
		exit 1
	fi
done

for forbidden in 'git describe' '{{.VERSION}}' '{{.COMMIT}}'; do
  if grep -qF "$forbidden" Taskfile.yml; then
    echo "local build must not interpolate repository-controlled version metadata: $forbidden" >&2
    exit 1
  fi
done
grep -qF 'go build -tags=tobari_dev -buildvcs=false -trimpath -o bin/' Taskfile.yml || {
	echo "local build must use fixed dev version metadata without implicit VCS stamping" >&2
	exit 1
}
grep -qF "go build -tags='tobari_dev tobari_research'" Taskfile.yml || {
	echo "build:dev must compile the research capability surface explicitly" >&2
	exit 1
}
# The Taskfile must contain this literal shell expansion.
# shellcheck disable=SC2016
if [[ $(grep -cF 'git rev-parse --verify HEAD' Taskfile.yml) -ne 2 ]] ||
	[[ $(grep -cF -- '-X main.commit=$(git rev-parse --verify HEAD)' Taskfile.yml) -ne 2 ]]; then
	echo "standard and development repository builds must embed only the exact source commit" >&2
	exit 1
fi
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

for required in \
  'require_embedded_gateway_release_contract' \
  'runtimeassets.ComponentVersion("gateway")' 'BuildIfMissing: true'; do
  grep -qF "$required" scripts/check.sh || {
		echo "release gate is missing embedded Gateway compatibility authority: $required" >&2
		exit 1
  }
done

for forbidden in 'HOMEBREW_GITHUB_API_TOKEN' 'api.github.com/repos/' 'Authorization: Bearer'; do
  if grep -R -F "$forbidden" Formula scripts/render-formula.sh .github/workflows/release.yml >/dev/null 2>&1; then
    echo "public release path contains private-asset behavior: $forbidden" >&2
    exit 1
  fi
done

for required in \
  'workflow_dispatch:' 'operation:' 'prepared_run_id:' 'actions: read' \
  "if: inputs.operation == 'prepare'" "if: inputs.operation == 'publish'" \
  'environment: release-publication' './scripts/package-release.sh' \
  './tools/releaseartifacts create' './tools/releaseartifacts verify' './tools/releaseartifacts verify-final' \
  'checksums.txt' 'sbom.spdx.json' 'provenance.intoto.jsonl' \
  'gh release create' 'scripts/render-formula.sh'; do
  grep -qF "$required" .github/workflows/release.yml || {
    echo "release workflow is missing: $required" >&2
    exit 1
  }
done
if grep -qF './scripts/check.sh' .github/workflows/release.yml; then
  echo "release preparation must reuse exact-revision CI evidence instead of repeating source profiles" >&2
  exit 1
fi
for required in \
  './scripts/check.sh full' './scripts/check.sh security' './scripts/check.sh release' \
  './scripts/check.sh public' './scripts/check.sh runtime'; do
  grep -qF "$required" .github/workflows/ci.yml || {
    echo "parallel CI is missing required release evidence: $required" >&2
    exit 1
  }
done
ci_job() {
  local job_name=$1
  awk -v heading="  ${job_name}:" '
    $0 == heading { in_job=1 }
    in_job && $0 != heading && /^  [A-Za-z0-9_-]+:/ { exit }
    in_job { print }
  ' .github/workflows/ci.yml
}
for binding in \
  'full:./scripts/check.sh full' \
  'security:./scripts/check.sh security' \
  'public:./scripts/check.sh public' \
  'runtime:./scripts/check.sh runtime' \
  'release:./scripts/check.sh release'; do
  job_name=${binding%%:*}
  profile_call=${binding#*:}
  job=$(ci_job "$job_name")
  if [[ -z $job ]] || ! printf '%s\n' "$job" | grep -qF "$profile_call"; then
    echo "CI must own $profile_call in its independent $job_name job" >&2
    exit 1
  fi
done

assemble_job=$(awk '
  /^  assemble:/ { in_assemble=1 }
  in_assemble && !/^  assemble:/ && /^  [A-Za-z0-9_-]+:/ { exit }
  in_assemble { print }
' .github/workflows/release.yml)
ci_evidence_job=$(awk '
  /^  ci-evidence:/ { in_evidence=1 }
  in_evidence && !/^  ci-evidence:/ && /^  [A-Za-z0-9_-]+:/ { exit }
  in_evidence { print }
' .github/workflows/release.yml)
homebrew_job=$(awk '
  /^  homebrew-tap:/ { in_homebrew=1 }
  in_homebrew && !/^  homebrew-tap:/ && /^  [A-Za-z0-9_-]+:/ { exit }
  in_homebrew { print }
' .github/workflows/release.yml)
publish_job=$(awk '
  /^  publish:/ { in_publish=1 }
  in_publish && !/^  publish:/ && /^  [A-Za-z0-9_-]+:/ { exit }
  in_publish { print }
' .github/workflows/release.yml)
build_job=$(awk '
  /^  build:/ { in_build=1 }
  in_build && !/^  build:/ && /^  [A-Za-z0-9_-]+:/ { exit }
  in_build { print }
' .github/workflows/release.yml)
release_revision_ref="ref: \${{ needs.identity.outputs.revision }}"
printf '%s\n' "$build_job" | grep -A4 -F "$release_revision_ref" | grep -qF 'persist-credentials: false' || {
  echo "matrix build checkout is not fixed to the credential-free release revision" >&2
  exit 1
}
# These are literal workflow expressions and jq programs.
# shellcheck disable=SC2016
for required in \
  "if: inputs.operation == 'prepare'" 'actions/workflows/ci.yml/runs' \
  '-f branch=main' '-f event=push' '-f head_sha="${RELEASE_REVISION}"' \
  '.path == ".github/workflows/ci.yml"' '.head_branch == "main"' \
  '.head_sha == $revision' '.status == "completed"' '.conclusion == "success"'; do
  if ! printf '%s\n' "$ci_evidence_job" | grep -qF -- "$required"; then
    echo "release CI-evidence job is missing: $required" >&2
    exit 1
  fi
done
for required in \
  "$release_revision_ref" \
  "if: inputs.operation == 'prepare'" \
  "runs-on: \${{ needs.identity.outputs.stable == 'true' && 'macos-15' || 'ubuntu-latest' }}" \
  './tools/releaseartifacts create' './tools/releaseartifacts verify' './tools/releaseartifacts verify-final' \
  './scripts/render-formula.sh' 'ruby -c' './scripts/audit-formula.sh' \
  'Upload complete prepared release asset set' 'retention-days: 7'; do
  if ! printf '%s\n' "$assemble_job" | grep -qF "$required"; then
    echo "release assembly job is missing its host-specific check: $required" >&2
    exit 1
  fi
done
printf '%s\n' "$assemble_job" | grep -A4 -F "$release_revision_ref" | grep -qF 'persist-credentials: false' || {
  echo "exact release source checkout persists workflow credentials" >&2
  exit 1
}
if printf '%s\n' "$assemble_job" | grep -qF './scripts/check.sh release'; then
  echo "release assembly job must not repeat the CI-owned release profile" >&2
  exit 1
fi
release_checkout_line=$(printf '%s\n' "$assemble_job" | grep -n -m1 -F "$release_revision_ref" | cut -d: -f1)
metadata_line=$(printf '%s\n' "$assemble_job" | grep -n -m1 -F './tools/releaseartifacts create' | cut -d: -f1)
verify_line=$(printf '%s\n' "$assemble_job" | grep -n -m1 -F './tools/releaseartifacts verify' | cut -d: -f1)
render_line=$(printf '%s\n' "$assemble_job" | grep -n -m1 -F './scripts/render-formula.sh' | cut -d: -f1)
audit_line=$(printf '%s\n' "$assemble_job" | grep -n -m1 -F './scripts/audit-formula.sh' | cut -d: -f1)
final_verify_line=$(printf '%s\n' "$assemble_job" | grep -n -m1 -F './tools/releaseartifacts verify-final' | cut -d: -f1)
upload_line=$(printf '%s\n' "$assemble_job" | grep -n -m1 -F 'Upload complete prepared release asset set' | cut -d: -f1)
if ((release_checkout_line >= metadata_line || metadata_line >= verify_line || verify_line >= render_line || render_line >= audit_line || audit_line >= final_verify_line || final_verify_line >= upload_line)); then
  echo "release metadata and Formula must be generated and verified at the release revision before upload" >&2
  exit 1
fi

# These are literal workflow expressions and shell expansions.
# shellcheck disable=SC2016
for required in \
  "if: inputs.operation == 'publish'" 'environment: release-publication' \
  'actions: read' 'contents: write' '.path == ".github/workflows/release.yml"' \
  '.head_branch == "main"' '.head_sha == $revision' '.conclusion == "success"' \
  'Assemble verified release assets' 'release-assets-${RELEASE_TAG}' \
  'github-token: ${{ github.token }}' 'repository: ${{ github.repository }}' \
  'run-id: ${{ inputs.prepared_run_id }}' 'digest-mismatch: error' \
  '/actions/runs/${PREPARED_RUN_ID}/attempts/${PREPARED_RUN_ATTEMPT}' \
  './tools/releaseartifacts verify-final' 'gh release create'; do
  if ! printf '%s\n' "$publish_job" | grep -qF -- "$required"; then
    echo "prepared release promotion job is missing: $required" >&2
    exit 1
  fi
done
for forbidden in './scripts/package-release.sh' './tools/releaseartifacts create'; do
  if printf '%s\n' "$publish_job" | grep -qF "$forbidden"; then
    echo "publication must promote prepared bytes without rebuilding: $forbidden" >&2
    exit 1
  fi
done
prepared_validation_line=$(printf '%s\n' "$publish_job" | grep -n -m1 -F 'Validate prepared run and exact artifact' | cut -d: -f1)
prepared_download_line=$(printf '%s\n' "$publish_job" | grep -n -m1 -F 'Download complete prepared release asset set' | cut -d: -f1)
prepared_verify_line=$(printf '%s\n' "$publish_job" | grep -n -m1 -F 'Reverify exact prepared subjects and published tag' | cut -d: -f1)
publish_line=$(printf '%s\n' "$publish_job" | grep -n -m1 -F 'Create GitHub Release without replacement' | cut -d: -f1)
if ((prepared_validation_line >= prepared_download_line || prepared_download_line >= prepared_verify_line || prepared_verify_line >= publish_line)); then
  echo "prepared run validation, download, verification, and publication are out of order" >&2
  exit 1
fi

# These are literal workflow expressions and shell expansions.
# shellcheck disable=SC2016
for required in \
  "if: inputs.operation == 'publish' && needs.identity.outputs.stable == 'true'" \
  'needs: [identity, publish]' 'environment: release-publication' \
  'actions/create-github-app-token@' 'app-id: ${{ secrets.HOMEBREW_APP_ID }}' \
  'private-key:' '${{ secrets.HOMEBREW_APP_KEY }}' 'owner: tasuku43' \
  'repositories: homebrew-tap' 'gh release download' '--pattern tobari.rb' \
  'repository: tasuku43/homebrew-tap' 'Formula/tobari.rb' \
  'git switch -c "${branch}"' 'gh pr create' '--repo tasuku43/homebrew-tap'; do
  if ! printf '%s\n' "$homebrew_job" | grep -qF -- "$required"; then
    echo "stable Homebrew publication job is missing: $required" >&2
    exit 1
  fi
done
if printf '%s\n' "$homebrew_job" | grep -qF 'git push origin main'; then
  echo "stable Homebrew publication must use the tap pull-request boundary" >&2
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
if [[ $(go run ./tools/releaseversion v0.1.0-dev.1) != $'version=0.1.0-dev.1\nstable=false' ]]; then
	echo "releaseversion does not classify the first development prerelease" >&2
	exit 1
fi
grep -qF 'args+=(--prerelease)' .github/workflows/release.yml || {
	echo "release workflow does not publish non-stable tags as GitHub prereleases" >&2
	exit 1
}
if scripts/render-formula.sh v1.2.3-rc.1 https://github.com/tasuku43/tobari /dev/null >/dev/null 2>&1; then
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
  if printf '%s\n' "$metadata" | grep -Eq 'tobari_research|tobari-research'; then
    echo "archive $asset contains research-surface build metadata" >&2
    exit 1
  fi
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
if grep -Eq 'tobari[-_]research' "$expected_assets" "$actual_assets"; then
  echo "release archive inventory contains a research binary name" >&2
  exit 1
fi

# A hostile caller must not widen a protected release through ambient GOFLAGS.
hostile_dist=$release_root/hostile-dist
mkdir -p "$hostile_dist"
env GOFLAGS=-tags=tobari_research GOCACHE="$release_root/go-cache-hostile" scripts/package-release.sh \
  "$release_tag" "$release_revision" linux amd64 "$hostile_dist" >/dev/null
hostile_archive=$hostile_dist/${binary}_${release_tag}_linux_amd64.tar.gz
hostile_extract=$release_root/hostile-extract
mkdir -p "$hostile_extract"
tar -xzf "$hostile_archive" -C "$hostile_extract"
hostile_metadata=$(go version -m "$hostile_extract/$binary")
if printf '%s\n' "$hostile_metadata" | grep -Eq 'tobari_research|tobari-research'; then
  echo "hostile GOFLAGS selected the research surface in a release archive" >&2
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

synthetic_builder=https://example.com/tobari/release-dry-run/v1
synthetic_invocation=https://example.com/tobari/release-dry-run/invocations/1
env GOPROXY=off GOSUMDB=off go run ./tools/releaseartifacts create \
  "$release_tag" "$release_revision" "$synthetic_builder" "$synthetic_invocation" "$dist"
env GOPROXY=off GOSUMDB=off go run ./tools/releaseartifacts verify \
  "$release_tag" "$release_revision" "$synthetic_builder" "$synthetic_invocation" "$dist"
env GOPROXY=off GOSUMDB=off go run ./tools/releaseartifacts create \
  "$release_tag" "$release_revision" "$synthetic_builder" "$synthetic_invocation" "$repro_dist"
env GOPROXY=off GOSUMDB=off go run ./tools/releaseartifacts verify \
  "$release_tag" "$release_revision" "$synthetic_builder" "$synthetic_invocation" "$repro_dist"
for metadata in checksums.txt sbom.spdx.json provenance.intoto.jsonl; do
  if ! cmp -s "$dist/$metadata" "$repro_dist/$metadata"; then
    echo "release metadata is not byte-for-byte deterministic: $metadata" >&2
    exit 1
  fi
done

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

metadata_digests_before=$release_root/metadata-digests-before.txt
metadata_digests_after=$release_root/metadata-digests-after.txt
for metadata in checksums.txt provenance.intoto.jsonl sbom.spdx.json; do
  printf '%s  %s\n' "$(sha256_of "$dist/$metadata")" "$metadata" >>"$metadata_digests_before"
done
if env GOPROXY=off GOSUMDB=off go run ./tools/releaseartifacts create \
  "$release_tag" "$release_revision" "$synthetic_builder" "$synthetic_invocation" "$dist" >/dev/null 2>&1; then
  echo "releaseartifacts overwrote existing metadata" >&2
  exit 1
fi
for metadata in checksums.txt provenance.intoto.jsonl sbom.spdx.json; do
  printf '%s  %s\n' "$(sha256_of "$dist/$metadata")" "$metadata" >>"$metadata_digests_after"
done
if ! cmp -s "$metadata_digests_before" "$metadata_digests_after"; then
  echo "releaseartifacts changed existing metadata on collision" >&2
  exit 1
fi

checksums=$dist/checksums.txt
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

formula=$dist/${binary}.rb
repository_url=https://github.com/tasuku43/tobari
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
env GOPROXY=off GOSUMDB=off go run ./tools/releaseartifacts verify-final \
  "$release_tag" "$release_revision" "$synthetic_builder" "$synthetic_invocation" "$dist"

scripts/test-audit-formula.sh >/dev/null

release_checks_inputs_after=$release_root/release-checks-inputs-after.txt
inputs_after_release_checks=$(release_input_fingerprint "$release_checks_inputs_after")
if [[ $inputs_after_reproduction != "$inputs_after_release_checks" ]]; then
  report_release_input_drift "during checksum or Formula validation" "$reproduction_inputs_after" "$release_checks_inputs_after"
  exit 1
fi

echo "lint-release: OK"
