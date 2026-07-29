#!/usr/bin/env bash
# Test double used by test-check-environment.sh for a mixed Go installation.
set -euo pipefail

if [[ $(basename "$0") == compile ]]; then
  echo "compile version go1.26.3"
  exit 0
fi

case ${1:-} in
  version)
    echo "go version go1.26.5 darwin/arm64"
    ;;
  env)
    case ${2:-} in
      GOVERSION) echo "go1.26.5" ;;
      GOROOT) echo "${FAKE_GO_ROOT:?}" ;;
      GOTOOLDIR) echo "${FAKE_GO_ROOT:?}/pkg/tool/darwin_arm64" ;;
      *) exit 64 ;;
    esac
    ;;
  *)
    exit 64
    ;;
esac
