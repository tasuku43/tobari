#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

scenario=scripts/test-integration.sh

expected_phases=$'preflight\nbuild-fixtures\ncontexts-and-cluster\ncredentials-and-workspaces\ngateway-broker-and-transport\nlive-policy-activation\nattachment-scoped-host-loopback\nruntime-failure-boundaries\nlifecycle'
actual_phases=$(awk '/^begin_phase / { print $2 }' "$scenario")
if [[ $actual_phases != "$expected_phases" ]]; then
  echo "integration scope: phase ownership changed" >&2
  diff -u <(printf '%s\n' "$expected_phases") <(printf '%s\n' "$actual_phases") >&2 || true
  exit 1
fi

line_count=$(wc -l <"$scenario" | tr -d ' ')
if ((line_count > 1700)); then
  echo "integration scope: scenario grew to $line_count lines (limit 1700)" >&2
  exit 1
fi

cli_reference_count=$(grep -Eoc 'run_tobari(_at|_pty_at)?' "$scenario")
if ((cli_reference_count > 45)); then
  echo "integration scope: scenario grew to $cli_reference_count CLI references (limit 45)" >&2
  exit 1
fi

# These command families are owned by fast domain/application/CLI tests. The
# Docker scenario may use only the minimal create/import/discover/act calls
# needed to assemble and exercise real runtime boundaries.
if grep -En \
  'policy preset (list|validate)|policy (rules|reset)|auth (status|login|logout)|runtime init|help policy' \
  "$scenario" >&2; then
  echo "integration scope: semantic or presentation matrix returned to the Docker scenario" >&2
  exit 1
fi

required_runtime_claims=(
  'network container:tobari-gateway'
  'cap-add NET_ADMIN'
  'ReadonlyRootfs'
  'handle copied across Contexts returned'
  'first_request_chunk'
  'oversized request'
  'denied GraphQL request reached mock upstream'
  'host.docker.internal'
  'Workspace opened a direct raw Internet connection'
  'OPA outage returned'
  'cluster down --purge'
)
for claim in "${required_runtime_claims[@]}"; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: missing runtime-only canary marker: $claim" >&2
    exit 1
  fi
done

if grep -F './scripts/check.sh integration' .github/workflows/ci.yml >/dev/null; then
  echo "integration scope: CI invokes integration separately even though runtime already includes it" >&2
  exit 1
fi
if ! awk '
  /^run_runtime\(\)/ { in_runtime=1 }
  in_runtime && /run_integration/ { found=1 }
  in_runtime && /^}/ { exit(found ? 0 : 1) }
  END { if (!in_runtime) exit 1 }
' scripts/check.sh; then
  echo "integration scope: runtime no longer includes the integration boundary" >&2
  exit 1
fi

echo "integration scope: OK ($line_count lines, $cli_reference_count CLI references)"
