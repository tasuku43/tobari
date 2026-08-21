#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

go run ./tools/runtimecheck
./scripts/check-exposure-helper-source.sh
