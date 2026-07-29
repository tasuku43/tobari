#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
source scripts/release-archive-entries.sh

fixture=$(mktemp -d "${TMPDIR:-/tmp}/release-archive-entries.XXXXXXXX")
cleanup() { rm -rf -- "$fixture"; }
trap cleanup EXIT

mkdir -p "$fixture/without-notice" "$fixture/with-notice"
for directory in "$fixture/without-notice" "$fixture/with-notice"; do
  printf 'license\n' >"$directory/LICENSE"
  printf 'binary\n' >"$directory/example-tool"
done
printf 'notices\n' >"$fixture/with-notice/THIRD_PARTY_NOTICES"

cd "$fixture/without-notice"
release_archive_entries "$fixture/without-notice/example-tool" example-tool
if [[ ${#archive_entries[@]} -ne 6 || ${#archive_supporting_files[@]} -ne 1 ||
      ${archive_supporting_files[0]} != LICENSE ]]; then
  echo "release entry helper did not select executable plus LICENSE" >&2
  exit 1
fi

cd "$fixture/with-notice"
release_archive_entries "$fixture/with-notice/example-tool" example-tool
if [[ ${#archive_entries[@]} -ne 9 || ${#archive_supporting_files[@]} -ne 2 ||
      ${archive_supporting_files[0]} != LICENSE || ${archive_supporting_files[1]} != THIRD_PARTY_NOTICES ]]; then
  echo "release entry helper did not include the reviewed notice" >&2
  exit 1
fi
