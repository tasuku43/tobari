#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

claude_version=$(go run ./tools/runtimecheck --print-claude-version)
tag=${TOBARI_CLAUDE_TAG:-tobari-claude-runtime:local}
base_image=${TOBARI_CLAUDE_BASE_IMAGE:-$(go run ./tools/runtimecheck --print-claude-parent)}

./scripts/check-runtime-claude.sh

docker build \
  --pull=false \
  --tag "$tag" \
  --file runtimes/claude/Dockerfile \
  --build-arg "BASE_IMAGE=$base_image" \
  --build-arg "CLAUDE_CODE_VERSION=$claude_version" \
  .

runtime_api=$(docker image inspect --format '{{index .Config.Labels "io.tobari.runtime-api"}}' "$tag")
runtime_lifetime=$(docker image inspect --format '{{index .Config.Labels "io.tobari.runtime-lifetime-command"}}' "$tag")
image_user=$(docker image inspect --format '{{.Config.User}}' "$tag")
entrypoint=$(docker image inspect --format '{{json .Config.Entrypoint}}' "$tag")
command=$(docker image inspect --format '{{json .Config.Cmd}}' "$tag")
[[ $runtime_api == 1 ]] || {
  echo "Claude image does not preserve Tobari runtime API 1" >&2
  exit 1
}
[[ $runtime_lifetime == 'sleep infinity' ]] || {
  echo "Claude image does not preserve Tobari lifetime command" >&2
  exit 1
}
[[ $image_user == tobari ]] || {
  echo "Claude image does not preserve the tobari user" >&2
  exit 1
}
[[ $entrypoint == '["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]' ]] || {
  echo "Claude image does not preserve the Tobari entrypoint" >&2
  exit 1
}
[[ $command == '["sleep","infinity"]' ]] || {
  echo "Claude image does not preserve the Tobari lifetime command" >&2
  exit 1
}

docker run --rm --entrypoint /bin/bash "$tag" -ceu '
  test "${HOME}" = /var/lib/tobari
  test "${DISABLE_AUTOUPDATER}" = 1
  test "$(command -v claude)" = /usr/local/bin/claude
  test -x /usr/local/bin/claude
  test ! -e /var/lib/tobari/.local/bin/claude
  claude --version
  git --version
  gh --version | head -n 1
  aws --version
'

docker run --rm --mount type=tmpfs,dst=/var/lib/tobari --entrypoint /bin/bash "$tag" -ceu '
  test "$(command -v claude)" = /usr/local/bin/claude
  claude --version
'

printf 'Claude runtime image ready: %s\n' "$tag"
