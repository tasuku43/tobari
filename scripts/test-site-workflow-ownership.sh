#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

require_literal() {
  local path=$1
  local expected=$2
  if ! grep -Fq -- "$expected" "$path"; then
    echo "$path is missing required workflow ownership: $expected" >&2
    exit 1
  fi
}

reject_literal() {
  local path=$1
  local forbidden=$2
  if grep -Fq -- "$forbidden" "$path"; then
    echo "$path duplicates canonical workflow behavior: $forbidden" >&2
    exit 1
  fi
}

for workflow in \
  .github/workflows/ci.yml \
  .github/workflows/security.yml \
  .github/workflows/architecture-pages.yml \
  .github/workflows/release.yml; do
  require_literal "$workflow" "uses: ./.github/actions/setup-repository-node"
done

reject_literal .github/workflows/ci.yml "npm ci"
reject_literal .github/workflows/ci.yml "playwright install"
reject_literal .github/workflows/architecture-pages.yml "pull_request:"
reject_literal .github/workflows/architecture-pages.yml "npm run test:static"
reject_literal .github/workflows/architecture-pages.yml "npm run test:browser"
require_literal .github/workflows/architecture-pages.yml "run: ./scripts/check-site.sh full"
require_literal scripts/check-site.sh "playwright install --with-deps chromium"

echo "site workflow ownership: OK"
