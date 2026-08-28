#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

evidence_source=internal/cli/batch_d_public_composition_test.go
temporary_root=$(mktemp -d)
trap 'rm -rf -- "$temporary_root"' EXIT

coverage_profile=$temporary_root/cli.cover
coverage_functions=$temporary_root/functions.txt
handlers=$temporary_root/handlers.txt

go test -coverprofile="$coverage_profile" ./internal/cli >/dev/null
go tool cover -func="$coverage_profile" >"$coverage_functions"

awk '
  /public-production-handler-evidence:start/ { inside = 1; next }
  /public-production-handler-evidence:end/ { inside = 0 }
  inside { print }
' "$evidence_source" |
  sed -n 's/.*: *"\(run[A-Za-z0-9]*\)".*/\1/p' |
  sort -u >"$handlers"

[[ -s $handlers ]] || {
  echo "public composition: exact handler evidence set is empty" >&2
  exit 1
}

failed=false
while IFS= read -r handler; do
  percent=$(awk -v handler="$handler" '$2 == handler { print $3; exit }' "$coverage_functions")
  if [[ -z $percent || $percent == 0.0% ]]; then
    echo "public composition: $handler is public but no focused CLI test executes it" >&2
    failed=true
  fi
done <"$handlers"

if [[ $failed == true ]]; then
  echo "public composition: add a behavior test that reaches the exact production handler; global coverage is not a substitute" >&2
  exit 1
fi

echo "public composition: every exact release/helper handler has focused execution evidence"
