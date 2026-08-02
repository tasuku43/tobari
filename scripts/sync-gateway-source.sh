#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source_dir=gateway
snapshot_dir=internal/infra/runtimeassets/assets/gateway

rm -rf -- "$snapshot_dir"
mkdir -p "$snapshot_dir"
cp -R -- "$source_dir/." "$snapshot_dir/"
chmod 0755 "$snapshot_dir/entrypoint.sh"

./scripts/check-gateway-source.sh
