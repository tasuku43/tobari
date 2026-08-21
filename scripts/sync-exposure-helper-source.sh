#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

old_destination=internal/infra/runtimeassets/helper-source
destination=internal/infra/runtimeassets/_helper-source
rm -rf -- "$old_destination"
rm -rf -- "$destination"
mkdir -p "$destination"
while IFS= read -r relative; do
  mkdir -p "$destination/$(dirname "$relative")"
  source=$relative
  case "$relative" in
    tobari-go.mod) source=go.mod ;;
    tobari-go.sum) source=go.sum ;;
  esac
  cp -- "$source" "$destination/$relative"
done < <(./scripts/exposure-helper-source-files.py)

./scripts/check-exposure-helper-source.sh
