#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source_dir=authbroker
snapshot_dir=internal/infra/runtimeassets/assets/authbroker

if ! diff -ru --exclude=__pycache__ --exclude='*.pyc' "$source_dir" "$snapshot_dir"; then
  echo "Auth Broker source snapshot is stale; run ./scripts/sync-authbroker-source.sh" >&2
  exit 1
fi

echo "auth broker source snapshot: OK"

