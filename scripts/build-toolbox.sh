#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# shellcheck disable=SC1091
source images/toolbox/versions.env

tag=${TOBARI_TOOLBOX_TAG:-tobari-toolbox:local}
for version_name in GH_VERSION AWS_CLI_VERSION KUBECTL_VERSION TWG_VERSION CWK_VERSION PUP_VERSION; do
  if [[ ! ${!version_name} =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "invalid ${version_name} in images/toolbox/versions.env" >&2
    exit 1
  fi
done
for checksum_name in CWK_AMD64_SHA256 CWK_ARM64_SHA256 PUP_AMD64_SHA256 PUP_ARM64_SHA256; do
  if [[ ! ${!checksum_name} =~ ^[0-9a-f]{64}$ ]]; then
    echo "invalid ${checksum_name} in images/toolbox/versions.env" >&2
    exit 1
  fi
done

docker build \
  --pull=true \
  --tag "$tag" \
  --file images/toolbox/Dockerfile \
  --build-arg "GH_VERSION=$GH_VERSION" \
  --build-arg "AWS_CLI_VERSION=$AWS_CLI_VERSION" \
  --build-arg "KUBECTL_VERSION=$KUBECTL_VERSION" \
  --build-arg "TWG_VERSION=$TWG_VERSION" \
  --build-arg "CWK_VERSION=$CWK_VERSION" \
  --build-arg "CWK_AMD64_SHA256=$CWK_AMD64_SHA256" \
  --build-arg "CWK_ARM64_SHA256=$CWK_ARM64_SHA256" \
  --build-arg "PUP_VERSION=$PUP_VERSION" \
  --build-arg "PUP_AMD64_SHA256=$PUP_AMD64_SHA256" \
  --build-arg "PUP_ARM64_SHA256=$PUP_ARM64_SHA256" \
  .

runtime_api=$(docker image inspect --format '{{index .Config.Labels "io.tobari.runtime-api"}}' "$tag")
runtime_lifetime=$(docker image inspect --format '{{index .Config.Labels "io.tobari.runtime-lifetime-command"}}' "$tag")
image_user=$(docker image inspect --format '{{.Config.User}}' "$tag")
entrypoint=$(docker image inspect --format '{{json .Config.Entrypoint}}' "$tag")
[[ $runtime_api == 1 ]] || {
  echo "toolbox image does not preserve Tobari runtime API 1" >&2
  exit 1
}
[[ $runtime_lifetime == 'sleep infinity' ]] || {
  echo "toolbox image does not preserve Tobari lifetime command" >&2
  exit 1
}
[[ $image_user == tobari ]] || {
  echo "toolbox image does not preserve the tobari user" >&2
  exit 1
}
[[ $entrypoint == '["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]' ]] || {
  echo "toolbox image does not preserve the Tobari entrypoint" >&2
  exit 1
}

docker run --rm --entrypoint /bin/bash \
  --env "CWK_VERSION=$CWK_VERSION" \
  --env "PUP_VERSION=$PUP_VERSION" \
  "$tag" -ceu '
  git --version
  gh --version | head -n 1
  aws --version
  kubectl version --client=true
  twg --version
  cwk version | grep -F "$CWK_VERSION"
  pup --version | grep -F "$PUP_VERSION"
  test -r /usr/share/doc/tobari-toolbox/THIRD_PARTY_NOTICES.md
  test -r /usr/share/licenses/tobari-toolbox/cwk/LICENSE
  test -r /usr/share/licenses/tobari-toolbox/cwk/THIRD_PARTY_NOTICES
  test -r /usr/share/licenses/tobari-toolbox/pup/LICENSE
  test -r /usr/share/licenses/tobari-toolbox/pup/LICENSE-3rdparty.csv
  curl --version | head -n 1
  jq --version
  ssh -V
'

printf 'toolbox image ready: %s\n' "$tag"
