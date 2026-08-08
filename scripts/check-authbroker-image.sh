#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

dockerfile=authbroker/Dockerfile
checksums=authbroker/gh-checksums.txt

test "$(grep -c '^ARG DEBIAN_IMAGE$' "$dockerfile")" -eq 1
test "$(grep -c '^ARG MITMPROXY_IMAGE$' "$dockerfile")" -eq 1
test "$(grep -c '^ARG GH_VERSION=2\.96\.0$' "$dockerfile")" -eq 1
test "$(grep -c '^USER 1000:1000$' "$dockerfile")" -eq 1
test "$(grep -c '^ENTRYPOINT \[\"/opt/tobari/entrypoint.sh\"\]$' "$dockerfile")" -eq 1
test "$(grep -c 'io\.tobari\.auth-broker-api=\"1\"' "$dockerfile")" -eq 1
test "$(grep -c 'io\.tobari\.auth-broker-role=\"credential-resolution\"' "$dockerfile")" -eq 1
test "$(grep -c '^EXPOSE' "$dockerfile")" -eq 0
test "$(grep -Ec '^[0-9a-f]{64}  gh_2\.96\.0_linux_(amd64|arm64)\.tar\.gz$' "$checksums")" -eq 2
grep -q '/run/tobari-auth/runtime' "$dockerfile"
grep -q '/run/tobari-auth/control' "$dockerfile"
grep -q 'sha256sum --check --strict' "$dockerfile"

echo "auth broker image contract: OK"

