#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source_dir=gateway
snapshot_dir=internal/infra/runtimeassets/assets/gateway
dockerfile=$source_dir/Dockerfile

test "$(grep -c 'io\.tobari\.gateway-api=\"4\"' "$dockerfile")" -eq 1
test "$(grep -c 'io\.tobari\.gateway-role=\"enforcement\"' "$dockerfile")" -eq 1

if ! diff -ru "$source_dir" "$snapshot_dir"; then
  echo "Gateway source snapshot is stale; run ./scripts/sync-gateway-source.sh" >&2
  exit 1
fi

echo "gateway source snapshot: OK"
