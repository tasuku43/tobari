#!/usr/bin/env bash
# Render a public Homebrew Formula from stable macOS release checksums.
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ $# -lt 3 || $# -gt 4 ]]; then
  echo "usage: $0 <stable-tag> <repository-url> <checksums-file> [output-file]" >&2
  exit 2
fi

tag=$1
repository_url=${2%.git}
checksums_file=$3
go run ./tools/releaseversion --stable "$tag" >/dev/null
if [[ ! $repository_url =~ ^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]]; then
  echo "repository URL must be https://github.com/owner/repository" >&2
  exit 2
fi
if [[ ! -f $checksums_file ]]; then
  echo "checksums file not found: $checksums_file" >&2
  exit 2
fi

binary=$(go run ./tools/projectmeta --field binary_name)
formula_class=$(go run ./tools/projectmeta --field formula_class)
description=$(go run ./tools/projectmeta --field description)
license_spdx=$(go run ./tools/projectmeta --field license_spdx)
version=${tag#v}
arm64_asset=${binary}_${tag}_darwin_arm64.tar.gz
amd64_asset=${binary}_${tag}_darwin_amd64.tar.gz

sha_for() {
  local asset=$1 checksum
  checksum=$(awk -v asset="$asset" '$2 == asset { print $1; found=1 } END { if (!found) exit 3 }' "$checksums_file") || {
    echo "checksum not found for $asset" >&2
    return 2
  }
  if [[ ! $checksum =~ ^[0-9a-f]{64}$ ]]; then
    echo "invalid checksum for $asset" >&2
    return 2
  fi
  printf '%s' "$checksum"
}

escape_sed() { printf '%s' "$1" | sed 's/[&|]/\\&/g'; }

arm64_sha=$(sha_for "$arm64_asset")
amd64_sha=$(sha_for "$amd64_asset")
arm64_url=${repository_url}/releases/download/${tag}/${arm64_asset}
amd64_url=${repository_url}/releases/download/${tag}/${amd64_asset}
template=Formula/${binary}.rb.template
output=${4:-Formula/${binary}.rb}
if [[ ! -f $template ]]; then
  echo "Formula template not found: $template" >&2
  exit 2
fi
output_dir=$(dirname "$output")
if [[ ! -d $output_dir ]]; then
  echo "Formula output directory not found: $output_dir" >&2
  exit 2
fi
temporary=$(mktemp "$output_dir/.${binary}.rb.XXXXXXXX")
cleanup() { rm -f -- "$temporary"; }
trap cleanup EXIT

sed \
  -e "s|@@FORMULA_CLASS@@|$(escape_sed "$formula_class")|g" \
  -e "s|@@DESCRIPTION@@|$(escape_sed "$description")|g" \
  -e "s|@@LICENSE_SPDX@@|$(escape_sed "$license_spdx")|g" \
  -e "s|@@REPOSITORY_URL@@|$(escape_sed "$repository_url")|g" \
  -e "s|@@VERSION@@|$(escape_sed "$version")|g" \
  -e "s|@@MACOS_ARM64_URL@@|$(escape_sed "$arm64_url")|g" \
  -e "s|@@MACOS_AMD64_URL@@|$(escape_sed "$amd64_url")|g" \
  -e "s|@@MACOS_ARM64_SHA256@@|$arm64_sha|g" \
  -e "s|@@MACOS_AMD64_SHA256@@|$amd64_sha|g" \
  -e "s|@@BINARY_NAME@@|$(escape_sed "$binary")|g" \
  "$template" > "$temporary"

if grep -qE '@@[A-Z0-9_]+@@' "$temporary"; then
  echo "rendered Formula still contains a placeholder" >&2
  exit 1
fi
mv "$temporary" "$output"
echo "updated $output for $tag"
