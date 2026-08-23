#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/../docs/architecture-site"
profile=${1:-}

install_dependencies() {
  npm ci
}

run_static() {
  install_dependencies
  npm run test:static
}

run_browser() {
  if [[ ${CI:-} == true && $(uname -s) == Linux ]]; then
    npm exec -- playwright install --with-deps chromium
  else
    npm exec -- playwright install chromium
  fi
  npm run test:browser
}

case "$profile" in
  fast)
    run_static
    ;;
  full)
    run_static
    run_browser
    ;;
  browser)
    run_browser
    ;;
  security)
    npm run check:source
    npm run test:release-surface-guard
    npm run check:release-surface
    ;;
  public)
    install_dependencies
    npm run generate:check
    npm run check:source
    npm run test:release-surface-guard
    npm run check:release-surface
    npm run build:pages
    npm run check:dist
    npm run check:release-surface:dist
    ;;
  *)
    echo "usage: $0 <fast|full|browser|security|public>" >&2
    exit 2
    ;;
esac
