#!/usr/bin/env bash
# Prove Formula audit cleanup never removes a pre-existing user tap.
set -euo pipefail
cd "$(dirname "$0")/.."

test_root=$(mktemp -d "${TMPDIR:-/tmp}/tobari-audit-test.XXXXXXXX")
cleanup() { rm -rf -- "$test_root"; }
trap cleanup EXIT

fake_brew=$PWD/scripts/testdata/fake-brew.sh
formula=$test_root/tobari.rb
printf '%s\n' 'class Tobari < Formula' 'end' >"$formula"
preexisting_tap=tobari-ci/audit

run_case() {
  expected=$1
  audit_failure=$2
  case_root=$test_root/$expected
  mkdir -p "$case_root"
  log=$case_root/brew.log
  : >"$log"

  set +e
  FAKE_BREW_LOG=$log \
    FAKE_BREW_ROOT=$case_root \
    FAKE_BREW_EXISTING_TAP=$preexisting_tap \
    FAKE_BREW_AUDIT_FAIL=$audit_failure \
    BREW_COMMAND=$fake_brew \
    AUDIT_FORMULA_BINARY=tobari \
    scripts/audit-formula.sh "$formula" >/dev/null 2>&1
  status=$?
  set -e

  if [[ $expected == success && $status -ne 0 ]]; then
    echo "audit-formula success fixture failed with status $status" >&2
    exit 1
  fi
  if [[ $expected == failure && $status -eq 0 ]]; then
    echo "audit-formula failure fixture unexpectedly succeeded" >&2
    exit 1
  fi
  if grep -Fxq "untap $preexisting_tap" "$log"; then
    echo "audit-formula removed a pre-existing user tap" >&2
    exit 1
  fi
  created_tap=$(awk '$1 == "tap-new" && $2 == "--no-git" { print $3 }' "$log")
  if [[ -z $created_tap || $created_tap == "$preexisting_tap" ]]; then
    echo "audit-formula did not create a unique owned tap" >&2
    exit 1
  fi
  if [[ $(grep -Fc "untap $created_tap" "$log") -ne 1 ]]; then
    echo "audit-formula did not clean up exactly its owned tap" >&2
    exit 1
  fi
}

run_case success false
run_case failure true

echo "test-audit-formula: OK"
