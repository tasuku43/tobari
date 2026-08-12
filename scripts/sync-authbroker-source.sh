#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source scripts/lib/component-runtime-source.sh

sync_component_runtime_source authbroker
snapshot_dir=internal/infra/runtimeassets/assets/authbroker
chmod 0755 "$snapshot_dir/entrypoint.sh" "$snapshot_dir/control-entrypoint.sh"

./scripts/check-authbroker-source.sh
