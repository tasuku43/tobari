#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
fixture_root=$(mktemp -d)
cleanup() { rm -rf -- "$fixture_root"; }
trap cleanup EXIT

mkdir -p "$fixture_root/decisions"
printf '# ADR 0001: First\n' >"$fixture_root/decisions/0001-first.md"
./scripts/check-decision-records.sh "$fixture_root/decisions" >/dev/null

printf '# ADR 0001: Duplicate\n' >"$fixture_root/decisions/0001-duplicate.md"
if ./scripts/check-decision-records.sh "$fixture_root/decisions" >"$fixture_root/output" 2>&1; then
  echo "duplicate ADR number unexpectedly passed" >&2
  exit 1
fi
grep -Fq 'duplicate ADR number 0001' "$fixture_root/output"
rm -f -- "$fixture_root/decisions/0001-duplicate.md"

printf '# ADR 0003: Wrong heading\n' >"$fixture_root/decisions/0002-wrong-heading.md"
if ./scripts/check-decision-records.sh "$fixture_root/decisions" >"$fixture_root/output" 2>&1; then
  echo "mismatched ADR heading unexpectedly passed" >&2
  exit 1
fi
grep -Fq 'ADR heading does not match filename 0002' "$fixture_root/output"

./scripts/check-decision-records.sh
echo "decision record identity tests: OK"
