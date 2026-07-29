#!/usr/bin/env bash
# Audit a rendered Formula in a uniquely owned temporary Homebrew tap.
set -euo pipefail
cd "$(dirname "$0")/.."

if [[ $# -gt 1 ]]; then
  echo "usage: $0 [formula-file]" >&2
  exit 2
fi

binary=${AUDIT_FORMULA_BINARY:-$(go run ./tools/projectmeta --field binary_name)}
formula=${1:-Formula/${binary}.rb}
brew_command=${BREW_COMMAND:-brew}
if [[ ! $binary =~ ^[a-z][a-z0-9-]*$ ]]; then
  echo "binary name is unsafe for an isolated tap: $binary" >&2
  exit 2
fi
if [[ ! -f $formula ]]; then
  echo "Formula not found: $formula" >&2
  exit 2
fi
if ! command -v "$brew_command" >/dev/null 2>&1; then
  echo "Homebrew command not found: $brew_command" >&2
  exit 2
fi

tap_marker=$(mktemp -d "${TMPDIR:-/tmp}/${binary}-homebrew-tap.XXXXXXXX")
suffix=${tap_marker##*.}
suffix=$(printf '%s' "$suffix" | tr '[:upper:]' '[:lower:]' | tr -cd 'a-z0-9')
if [[ -z $suffix ]]; then
  echo "could not derive a safe unique tap name" >&2
  exit 1
fi
tap=${binary}-ci-${suffix}/audit
tap_created=false

cleanup() {
  status=$?
  trap - EXIT
  if [[ $tap_created == true ]]; then
    if ! "$brew_command" untap "$tap" >/dev/null 2>&1; then
      echo "failed to remove owned temporary tap: $tap" >&2
      status=1
    fi
  fi
  if ! rm -rf -- "$tap_marker"; then
    echo "failed to remove temporary tap marker: $tap_marker" >&2
    status=1
  fi
  exit "$status"
}
trap cleanup EXIT

installed_taps=$("$brew_command" tap)
if printf '%s\n' "$installed_taps" | grep -Fxq "$tap"; then
  echo "generated temporary tap already exists; refusing to modify it: $tap" >&2
  exit 1
fi

"$brew_command" tap-new --no-git "$tap" >/dev/null
tap_created=true
tap_dir=$("$brew_command" --repository "$tap")
if [[ ! -d $tap_dir/Formula ]]; then
  echo "temporary tap has no Formula directory: $tap_dir" >&2
  exit 1
fi
cp "$formula" "$tap_dir/Formula/${binary}.rb"
"$brew_command" audit --strict "$tap/$binary"
