#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source_dir=runtimes/base
snapshot_dir=internal/infra/runtimeassets/assets/tobari

cp -- "$source_dir/Dockerfile" "$snapshot_dir/Dockerfile"
cp -- "$source_dir/entrypoint.sh" "$snapshot_dir/entrypoint.sh"
cp -- "$source_dir/gh" "$snapshot_dir/gh"
cp -- "$source_dir/aws-cli-public-key.asc" "$snapshot_dir/aws-cli-public-key.asc"
chmod 0755 "$snapshot_dir/entrypoint.sh" "$snapshot_dir/gh"

./scripts/check-runtime-base.sh
