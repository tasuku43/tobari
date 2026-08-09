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
  npm exec -- playwright install chromium
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
    ;;
  public)
    install_dependencies
    npm run generate:check
    npm run check:source
    npm run build:pages
    npm run check:dist
    ;;
  *)
    echo "usage: $0 <fast|full|browser|security|public>" >&2
    exit 2
    ;;
esac
