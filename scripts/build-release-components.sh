#!/usr/bin/env bash
# Build the published Gateway and create its source-bound lock.
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ $# -ne 3 ]]; then
  echo "usage: $0 <revision> <publish:true|false> <output-directory>" >&2
  exit 2
fi
revision=$1
publish=$2
output_directory=$3
if [[ ! $revision =~ ^[0-9a-f]{40}$ ]] || [[ $publish != true && $publish != false ]]; then
  echo "invalid release component request" >&2
  exit 2
fi
test "$(git rev-parse HEAD)" = "$revision"
mkdir -p "$output_directory"
output_directory=$(cd "$output_directory" && pwd)
if [[ -e $output_directory/component-lock.json ]]; then
  echo "component lock already exists; refusing to overwrite it" >&2
  exit 1
fi

mitmproxy_image=$(awk -F= '$1 == "MITMPROXY_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
gateway_api=$(sed -n 's/.*io\.tobari\.gateway-api="\([1-9][0-9]*\)".*/\1/p' gateway/Dockerfile)
[[ $mitmproxy_image == *@sha256:* ]]
[[ $gateway_api =~ ^[1-9][0-9]*$ ]]

builder="tobari-release-${GITHUB_RUN_ID:-local}-$$"
docker buildx create --name "$builder" --driver docker-container --use >/dev/null
cleanup() { docker buildx rm "$builder" >/dev/null 2>&1 || true; }
trap cleanup EXIT
docker buildx inspect --bootstrap >/dev/null

build_component() {
  local name=$1 image=$2 dockerfile=$3 context=$4 evidence=$5
  shift 5
  local metadata=$output_directory/$name.metadata.json
  local -a output_args
  if [[ $publish == true ]]; then
    if docker buildx imagetools inspect "$image:sha-$revision" >/dev/null 2>&1; then
      echo "immutable $name tag already exists; refusing to overwrite it" >&2
      return 1
    fi
    output_args=(--tag "$image:sha-$revision" --push)
  else
    output_args=(--output "type=oci,dest=$output_directory/$name.oci.tar")
  fi
  docker buildx build \
    --platform linux/amd64,linux/arm64 \
    --file "$dockerfile" \
    --label "org.opencontainers.image.revision=$revision" \
    --label "org.opencontainers.image.source=https://github.com/tasuku43/tobari" \
    --sbom=true --provenance=mode=max \
    --metadata-file "$metadata" \
    "${output_args[@]}" "$@" "$context"
  local digest
  digest=$(jq -er '.["containerimage.digest"]' "$metadata")
  [[ $digest =~ ^sha256:[0-9a-f]{64}$ ]]
  if [[ $publish == true ]]; then
    local remote_digest
    remote_digest=$(docker buildx imagetools inspect "$image:sha-$revision" --format '{{json .}}' | jq -er '.manifest.digest')
    test "$remote_digest" = "$digest"
  else
    rm -f -- "$output_directory/$name.oci.tar"
  fi
  jq -n --arg image "$image" --arg digest "$digest" --arg revision "$revision" \
    '{schema_version:1,image:$image,digest:$digest,revision:$revision,platforms:["linux/amd64","linux/arm64"]}' >"$evidence"
}

build_component gateway ghcr.io/tasuku43/tobari/gateway gateway/Dockerfile gateway \
  "$output_directory/gateway.component.json" --build-arg "MITMPROXY_IMAGE=$mitmproxy_image"

go run ./tools/componentlock create "$revision" "$gateway_api" "$output_directory/gateway.component.json" \
  "$output_directory/component-lock.json"
go run ./tools/componentlock verify "$output_directory/component-lock.json" "$revision"
