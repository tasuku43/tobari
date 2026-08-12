#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

source scripts/lib/component-runtime-source.sh

check_component_runtime_source authbroker

echo "auth broker runtime-input snapshot: OK"
