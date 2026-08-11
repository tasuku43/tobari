#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source_dir=gateway
snapshot_dir=internal/infra/runtimeassets/assets/gateway
dockerfile=$source_dir/Dockerfile

test "$(grep -c 'io\.tobari\.gateway-api=\"1\"' "$dockerfile")" -eq 1
test "$(grep -c 'io\.tobari\.gateway-role=\"enforcement\"' "$dockerfile")" -eq 1
test "$(grep -c 'IPROUTE2_VERSION=6\.15\.0-1' "$dockerfile")" -eq 1
test "$(grep -c 'NFTABLES_VERSION=1\.1\.3-1' "$dockerfile")" -eq 1
test "$(grep -c -- '--mode transparent@15001' "$source_dir/entrypoint.sh")" -eq 1
test "$(grep -c -- '--mode regular' "$source_dir/entrypoint.sh" || true)" -eq 0
test "$(grep -c 'dport 8080' "$source_dir/network-guard.sh" || true)" -eq 0

if ! diff -ru "$source_dir" "$snapshot_dir"; then
  echo "Gateway source snapshot is stale; run ./scripts/sync-gateway-source.sh" >&2
  exit 1
fi

echo "gateway source snapshot: OK"
