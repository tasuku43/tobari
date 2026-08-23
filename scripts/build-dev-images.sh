#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

runtime_tag=tobari-runtime:dev
research=false
if [[ ${1:-} == --research ]]; then
  research=true
elif [[ $# -ne 0 ]]; then
  echo "usage: $0 [--research]" >&2
  exit 2
fi
gateway_version=$(go run ./tools/runtimeassetid gateway)
gateway_tag="tobari-gateway:dev-${gateway_version}"
base_image=$(go run ./tools/runtimecheck --print-base-image)
mitmproxy_image=$(awk -F= '$1 == "MITMPROXY_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
debian_image=$(awk -F= '$1 == "DEBIAN_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
go_builder_image=$(awk -F= '$1 == "GO_BUILDER_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
exposure_helper_source=$(go run ./tools/runtimeassetid exposure-helper)
test -n "$base_image"
test -n "$mitmproxy_image"
test -n "$debian_image"

docker build \
  --tag "$runtime_tag" \
  --file runtimes/base/Dockerfile \
  --build-arg "DEBIAN_IMAGE=$base_image" \
  --build-arg "GO_BUILDER_IMAGE=$go_builder_image" \
  --build-arg "TOBARI_EXPOSURE_HELPER_SOURCE=$exposure_helper_source" \
  --build-arg "TOBARI_UID=$(id -u)" \
  --build-arg "TOBARI_GID=$(id -g)" \
  --build-context helper-source=internal/infra/runtimeassets/_helper-source \
  runtimes/base

if ! docker image inspect "$gateway_tag" >/dev/null 2>&1; then
  docker build \
    --tag "$gateway_tag" \
    --file gateway/Dockerfile \
    --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" \
    gateway
fi

if [[ $research == true ]]; then
  auth_broker_version=$(go run ./tools/runtimeassetid authbroker)
  auth_broker_tag="tobari-auth-broker:dev-${auth_broker_version}"
  experimental_gateway_tag="tobari-gateway-experimental:dev-${gateway_version}"
  if ! docker image inspect "$auth_broker_tag" >/dev/null 2>&1; then
    docker build \
      --tag "$auth_broker_tag" \
      --file authbroker/Dockerfile \
      --build-arg "MITMPROXY_IMAGE=$mitmproxy_image" \
      --build-arg "DEBIAN_IMAGE=$debian_image" \
      authbroker
  fi
  docker build \
    --tag "$experimental_gateway_tag" \
    --file gateway/Dockerfile.experimental \
    --build-arg "TOBARI_GATEWAY_BASE=$gateway_tag" \
    gateway
  printf 'Built research development images: %s %s %s %s\n' "$runtime_tag" "$gateway_tag" "$experimental_gateway_tag" "$auth_broker_tag"
else
  printf 'Built release-surface development images: %s %s\n' "$runtime_tag" "$gateway_tag"
fi
