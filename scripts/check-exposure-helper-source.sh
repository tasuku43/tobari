#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

destination=internal/infra/runtimeassets/_helper-source
temporary=$(mktemp -d)
cleanup() { rm -rf -- "$temporary"; }
trap cleanup EXIT

./scripts/exposure-helper-source-files.py > "$temporary/expected"
(
  cd "$destination"
  find . -type f -print | sed 's#^\./##' | LC_ALL=C sort
) > "$temporary/actual"
diff -u "$temporary/expected" "$temporary/actual"
while IFS= read -r relative; do
  source=$relative
  case "$relative" in
    tobari-go.mod) source=go.mod ;;
    tobari-go.sum) source=go.sum ;;
  esac
  cmp -- "$source" "$destination/$relative"
done < "$temporary/expected"

echo "exposure helper source snapshot: OK"
