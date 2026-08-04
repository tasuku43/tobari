#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

runtime_tag=tobari-runtime:dev
gateway_tag=tobari-gateway:dev
base_image=$(go run ./tools/runtimecheck --print-base-image)
mitmproxy_image=$(awk -F= '$1 == "MITMPROXY_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
test -n "$base_image"
test -n "$mitmproxy_image"

docker build \
  --tag "$runtime_tag" \
  --file runtimes/base/Dockerfile \
  --build-arg "DEBIAN_IMAGE=$base_image" \
  --build-arg "TOBARI_UID=$(id -u)" \
  --build-arg "TOBARI_GID=$(id -g)" \
  runtimes/base

docker build \
  --tag "$gateway_tag" \
  --file gateway/Dockerfile \
  --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" \
  gateway

printf 'Built development images: %s %s\n' "$runtime_tag" "$gateway_tag"
