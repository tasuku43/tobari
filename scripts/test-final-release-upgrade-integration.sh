#!/usr/bin/env bash
set -Eeuo pipefail
cd "$(dirname "$0")/.."

test_root=$(mktemp -d "$PWD/.tobari-final-release-upgrade.XXXXXX")
cleanup() {
	local status=$?
	trap - EXIT
	rm -rf -- "$test_root"
	exit "$status"
}
trap cleanup EXIT

predecessor_binary=$test_root/tobari-predecessor
./scripts/build-release-predecessor.sh "$predecessor_binary"

TOBARI_FIRST_USE_PREDECESSOR_BINARY=$predecessor_binary \
	./scripts/test-final-first-use-integration.sh
