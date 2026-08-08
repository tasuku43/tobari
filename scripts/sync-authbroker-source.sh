#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source_dir=authbroker
snapshot_dir=internal/infra/runtimeassets/assets/authbroker

rm -rf -- "$snapshot_dir"
mkdir -p "$snapshot_dir"
cp -R -- "$source_dir/." "$snapshot_dir/"
find "$snapshot_dir" -type d -name __pycache__ -prune -exec rm -rf -- {} +
find "$snapshot_dir" -type f -name '*.pyc' -delete
chmod 0755 "$snapshot_dir/entrypoint.sh" "$snapshot_dir/control-entrypoint.sh"

./scripts/check-authbroker-source.sh

