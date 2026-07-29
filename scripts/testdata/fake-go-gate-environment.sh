#!/usr/bin/env bash
# Test double used by test-check-environment.sh. It must be installed as `go`.
set -euo pipefail

[[ ${GO111MODULE:-} == on ]]
[[ ${GOENV:-} == off ]]
[[ ${GOEXPERIMENT+x} == x && -z $GOEXPERIMENT ]]
[[ ${GOFIPS140:-} == off ]]
[[ ${GOFLAGS+x} == x && -z $GOFLAGS ]]
[[ ${GOTOOLCHAIN:-} == local ]]
[[ ${GOWORK:-} == off ]]

echo "canonical gate reached Go with a sanitized environment" >&2
exit 73
