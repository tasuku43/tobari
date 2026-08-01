#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

codex_version=$(go run ./tools/runtimecheck --print-codex-version)
tag=${TOBARI_CODEX_TAG:-tobari-codex-runtime:local}
base_image=${TOBARI_CODEX_BASE_IMAGE:-$(go run ./tools/runtimecheck --print-codex-parent)}

./scripts/check-runtime-codex.sh

docker build \
  --pull=false \
  --tag "$tag" \
  --file runtimes/codex/Dockerfile \
  --build-arg "BASE_IMAGE=$base_image" \
  --build-arg "CODEX_VERSION=$codex_version" \
  .

runtime_api=$(docker inspect --format '{{index .Config.Labels "io.tobari.runtime-api"}}' "$tag")
runtime_lifetime=$(docker inspect --format '{{index .Config.Labels "io.tobari.runtime-lifetime-command"}}' "$tag")
image_user=$(docker inspect --format '{{.Config.User}}' "$tag")
entrypoint=$(docker inspect --format '{{json .Config.Entrypoint}}' "$tag")
command=$(docker inspect --format '{{json .Config.Cmd}}' "$tag")

test "$runtime_api" = 1
test "$runtime_lifetime" = 'sleep infinity'
test "$image_user" = tobari
test "$entrypoint" = '["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]'
test "$command" = '["sleep","infinity"]'

docker run --rm --entrypoint /bin/bash "$tag" -ceu '
  test "${HOME}" = /var/lib/tobari
  test "${CODEX_HOME}" = /var/lib/tobari/.codex
  test "$(command -v codex)" = /usr/local/bin/codex
  test "$(command -v codex-code-mode-host)" = /usr/local/bin/codex-code-mode-host
  test "$(command -v rg)" = /usr/local/bin/rg
  test -x /usr/local/bin/codex
  test ! -e /var/lib/tobari/.codex/packages/standalone/releases
  codex --version
  codex --help >/dev/null
  git --version
  gh --version | head -n 1
  aws --version
'

docker run --rm --mount type=tmpfs,dst=/var/lib/tobari --entrypoint /bin/bash "$tag" -ceu '
  test "$(command -v codex)" = /usr/local/bin/codex
  test "$(command -v codex-code-mode-host)" = /usr/local/bin/codex-code-mode-host
  test "$(command -v rg)" = /usr/local/bin/rg
  codex --version
'

printf 'Codex runtime image ready: %s\n' "$tag"
