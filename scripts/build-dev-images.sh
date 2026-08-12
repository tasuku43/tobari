#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

runtime_tag=tobari-runtime:dev
gateway_version=$(go run ./tools/runtimeassetid gateway)
auth_broker_version=$(go run ./tools/runtimeassetid authbroker)
gateway_tag="tobari-gateway:dev-${gateway_version}"
auth_broker_tag="tobari-auth-broker:dev-${auth_broker_version}"
base_image=$(go run ./tools/runtimecheck --print-base-image)
mitmproxy_image=$(awk -F= '$1 == "MITMPROXY_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
debian_image=$(awk -F= '$1 == "DEBIAN_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
test -n "$base_image"
test -n "$mitmproxy_image"
test -n "$debian_image"

docker build \
  --tag "$runtime_tag" \
  --file runtimes/base/Dockerfile \
  --build-arg "DEBIAN_IMAGE=$base_image" \
  --build-arg "TOBARI_UID=$(id -u)" \
  --build-arg "TOBARI_GID=$(id -g)" \
  runtimes/base

if ! docker image inspect "$auth_broker_tag" >/dev/null 2>&1; then
  docker build \
    --tag "$auth_broker_tag" \
    --file authbroker/Dockerfile \
    --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" \
    --build-arg "DEBIAN_IMAGE=$debian_image" \
    authbroker
fi

if ! docker image inspect "$gateway_tag" >/dev/null 2>&1; then
  docker build \
    --tag "$gateway_tag" \
    --file gateway/Dockerfile \
    --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" \
    gateway
fi

printf 'Built development images: %s %s %s\n' "$runtime_tag" "$gateway_tag" "$auth_broker_tag"
