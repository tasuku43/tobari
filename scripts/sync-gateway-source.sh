#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source scripts/lib/component-runtime-source.sh

sync_component_runtime_source gateway
snapshot_dir=internal/infra/runtimeassets/assets/gateway
chmod 0755 "$snapshot_dir/entrypoint.sh"

./scripts/check-gateway-source.sh
