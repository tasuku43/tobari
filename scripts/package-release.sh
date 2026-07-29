#!/usr/bin/env bash
# Build one byte-for-byte reproducible release archive for a pure-Go target.
set -euo pipefail
cd "$(dirname "$0")/.."
export GO111MODULE=on
export GOENV=off
export GOEXPERIMENT=
export GOFIPS140=off
export GOFLAGS=
export GOTOOLCHAIN=local
export GOWORK=off
source scripts/release-archive-entries.sh

if [[ $# -ne 5 ]]; then
  echo "usage: $0 <tag> <revision> <goos> <goarch> <output-dir>" >&2
  exit 2
fi

tag=$1
revision=$2
goos=$3
goarch=$4
output_dir=$5

go run ./tools/releaseversion "$tag" >/dev/null
if [[ ! $revision =~ ^[0-9a-f]{40}$ ]]; then
  echo "revision must be a full lowercase Git commit SHA" >&2
  exit 2
fi
case "$goos/$goarch" in
  linux/amd64|linux/arm64|darwin/amd64|darwin/arm64|windows/amd64) ;;
  *) echo "unsupported release target: $goos/$goarch" >&2; exit 2 ;;
esac

binary=$(go run ./tools/projectmeta --field binary_name)
module=$(go run ./tools/projectmeta --field go_module)
version=${tag#v}
executable=$binary
extension=tar.gz
if [[ $goos == windows ]]; then
  executable=${binary}.exe
  extension=zip
fi
archive_format=$extension
archive=${binary}_${tag}_${goos}_${goarch}.${extension}
mkdir -p "$output_dir"
output_dir=$(cd "$output_dir" && pwd)
archive_path=$output_dir/$archive
if [[ -e $archive_path || -L $archive_path ]]; then
  echo "release archive already exists; refusing to overwrite it: $archive_path" >&2
  exit 1
fi
work_dir=$(mktemp -d "$output_dir/.${binary}-package.XXXXXXXX")
cleanup() { rm -rf -- "$work_dir"; }
trap cleanup EXIT

target_environment=(CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch")
case "$goarch" in
  amd64) target_environment+=(GOAMD64=v1) ;;
  arm64) target_environment+=(GOARM64=v8.0) ;;
esac
env "${target_environment[@]}" go build -buildvcs=false -trimpath \
  -ldflags "-s -w -X main.version=${version} -X main.commit=${revision}" \
  -o "$work_dir/$executable" "./cmd/$binary"

go version -m "$work_dir/$executable" | grep -F "$module" >/dev/null

host_os=$(go env GOHOSTOS)
host_arch=$(go env GOHOSTARCH)
if [[ $goos == "$host_os" && $goarch == "$host_arch" ]]; then
  actual=$("$work_dir/$executable" version)
  expected="$binary $version ($revision)"
  if [[ $actual != "$expected" ]]; then
    echo "version output = $actual, want $expected" >&2
    exit 1
  fi
fi

release_archive_entries "$work_dir/$executable" "$executable"
expected_members=("${archive_supporting_files[@]}")
expected_members+=("$executable")

go run ./tools/archivepack \
  "$archive_format" \
  "$work_dir/$archive" \
  "${archive_entries[@]}"
go run ./tools/archivepack verify \
  "$archive_format" \
  "$work_dir/$archive" \
  "${archive_entries[@]}"
expected_member_list=$(printf '%s\n' "${expected_members[@]}")
if [[ $goos == windows ]]; then
  members=$(unzip -Z1 "$work_dir/$archive")
else
  members=$(tar -tzf "$work_dir/$archive")
fi
if [[ $members != "$expected_member_list" ]]; then
  echo "release archive contains unexpected entries: $members" >&2
  exit 1
fi
if ! ln "$work_dir/$archive" "$archive_path"; then
  echo "release archive appeared during build or cannot be created without overwrite: $archive_path" >&2
  exit 1
fi

echo "created $archive_path"
