#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >>"$TOBARI_TEST_DOCKER_LOG"
case "$*" in
  "--context fake-integration-context context inspect fake-integration-context")
    ;;
  "--context fake-integration-context version")
    ;;
  "--context fake-integration-context inspect tobari-auth-broker")
    ;;
  *)
    exit 1
    ;;
esac
