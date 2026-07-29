#!/usr/bin/env bash
# Define the exact reviewed member set shared by packaging and focused tests.

release_archive_entries() {
  if [[ $# -ne 2 ]]; then
    echo "release_archive_entries requires <built-executable-path> <archive-executable-name>" >&2
    return 2
  fi
  local built_executable=$1
  local archive_executable=$2

  archive_entries=(
    "$built_executable" "$archive_executable" 0755
    LICENSE LICENSE 0644
  )
  archive_supporting_files=(LICENSE)
  if [[ -e THIRD_PARTY_NOTICES || -L THIRD_PARTY_NOTICES ]]; then
    archive_entries+=(THIRD_PARTY_NOTICES THIRD_PARTY_NOTICES 0644)
    archive_supporting_files+=(THIRD_PARTY_NOTICES)
  fi
}
