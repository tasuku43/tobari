#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source_dir=runtimes/base
snapshot_dir=internal/infra/runtimeassets/assets/tobari

cp -- "$source_dir/Dockerfile" "$snapshot_dir/Dockerfile"
cp -- "$source_dir/entrypoint.sh" "$snapshot_dir/entrypoint.sh"
chmod 0755 "$snapshot_dir/entrypoint.sh"

./scripts/check-runtime-base.sh
