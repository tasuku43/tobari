#!/usr/bin/env bash
set -euo pipefail

: "${FAKE_BREW_LOG:?}"
: "${FAKE_BREW_ROOT:?}"
: "${FAKE_BREW_EXISTING_TAP:?}"

command_name=${1:-}
shift || true
printf '%s' "$command_name" >>"$FAKE_BREW_LOG"
for argument in "$@"; do
  printf ' %s' "$argument" >>"$FAKE_BREW_LOG"
done
printf '\n' >>"$FAKE_BREW_LOG"

tap_path() {
  printf '%s/%s' "$FAKE_BREW_ROOT" "${1//\//__}"
}

case "$command_name" in
  tap)
    printf '%s\n' "$FAKE_BREW_EXISTING_TAP"
    ;;
  tap-new)
    [[ ${1:-} == --no-git ]]
    tap=${2:?}
    path=$(tap_path "$tap")
    mkdir -p "$path/Formula"
    printf '%s' "$tap" >"$FAKE_BREW_ROOT/created-tap"
    ;;
  --repository)
    tap=${1:?}
    [[ -f $FAKE_BREW_ROOT/created-tap ]]
    [[ $(<"$FAKE_BREW_ROOT/created-tap") == "$tap" ]]
    tap_path "$tap"
    ;;
  audit)
    if [[ ${FAKE_BREW_AUDIT_FAIL:-false} == true ]]; then
      exit 9
    fi
    ;;
  untap)
    tap=${1:?}
    if [[ $tap == "$FAKE_BREW_EXISTING_TAP" ]]; then
      echo "attempted to remove an existing user tap" >&2
      exit 97
    fi
    [[ -f $FAKE_BREW_ROOT/created-tap ]]
    [[ $(<"$FAKE_BREW_ROOT/created-tap") == "$tap" ]]
    rm -rf -- "$(tap_path "$tap")"
    ;;
  *)
    echo "unexpected fake brew command: $command_name" >&2
    exit 98
    ;;
esac
