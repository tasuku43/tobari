#!/usr/bin/env bash
set -euo pipefail

printf '%s\n' "$*" >>"$TOBARI_TEST_DOCKER_LOG"
case "${1:-} ${2:-}" in
  "context show")
    printf '%s\n' fake-integration-context
    ;;
  "version ")
    ;;
  "inspect tobari-auth-broker")
    ;;
  *)
    exit 1
    ;;
esac
