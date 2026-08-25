#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

scenario=scripts/test-integration.sh
workspace_service_helper=test/integration/workspace_service_exposure.sh
gateway_fixture_helper=test/integration/gateway_fixture.sh
runtime_image_cleanup_helper=test/integration/runtime_image_cleanup.sh
permission_resume_helper=test/integration/permission_resume.sh

expected_phases=$'preflight\nbuild-fixtures\nmanifests-and-cluster\ncredentials-and-workspaces\ngateway-broker-and-transport\nlive-policy-activation\nattachment-scoped-host-loopback\nruntime-failure-boundaries\nlifecycle'
actual_phases=$(awk '/^begin_phase / { print $2 }' "$scenario")
if [[ $actual_phases != "$expected_phases" ]]; then
  echo "integration scope: phase ownership changed" >&2
  diff -u <(printf '%s\n' "$expected_phases") <(printf '%s\n' "$actual_phases") >&2 || true
  exit 1
fi

line_count=$(wc -l <"$scenario" | tr -d ' ')
if ((line_count > 1980)); then
  echo "integration scope: scenario grew to $line_count lines (limit 1980)" >&2
  exit 1
fi
runtime_image_cleanup_line_count=$(wc -l <"$runtime_image_cleanup_helper" | tr -d ' ')
if ((runtime_image_cleanup_line_count > 50)); then
  echo "integration scope: Runtime cleanup helper grew to $runtime_image_cleanup_line_count lines (limit 50)" >&2
  exit 1
fi
helper_line_count=$(wc -l <"$workspace_service_helper" | tr -d ' ')
if ((helper_line_count > 100)); then
  echo "integration scope: Workspace service phase helper grew to $helper_line_count lines (limit 100)" >&2
  exit 1
fi
gateway_fixture_helper_line_count=$(wc -l <"$gateway_fixture_helper" | tr -d ' ')
if ((gateway_fixture_helper_line_count > 100)); then
  echo "integration scope: Gateway fixture helper grew to $gateway_fixture_helper_line_count lines (limit 100)" >&2
  exit 1
fi
permission_resume_helper_line_count=$(wc -l <"$permission_resume_helper" | tr -d ' ')
if ((permission_resume_helper_line_count > 90)); then
  echo "integration scope: Permission resume helper grew to $permission_resume_helper_line_count lines (limit 90)" >&2
  exit 1
fi
./test/integration/gateway_fixture_test.sh
for claim in \
  'label=io.tobari.owner=default' \
  'label=io.tobari.component=runtime-revision' \
  "label=io.tobari.runtime-id=\$expected_runtime_id" \
  "label=io.tobari.runtime-revision=\$expected_source_digest" \
  "runtime_image=\"\$repository:\$tag\"" \
  "runtime_image_id=\$image_id"; do
  if ! grep -F "$claim" "$runtime_image_cleanup_helper" >/dev/null; then
    echo "integration scope: missing exact managed Runtime cleanup evidence: $claim" >&2
    exit 1
  fi
done
# The claim intentionally matches the literal shell variable in the sourced helper.
# shellcheck disable=SC2016
for claim in \
  'run_tobari review services --format=json' \
  'run_tobari service allow --id "$request_ref" --format=json' \
  'exact generated origin and opaque exposure reference' \
  'exact-authority Workspace HTTP relay' \
  'current-attachment exposure status' \
  'stopped Workspace service exposure remained reachable'; do
  if ! grep -F "$claim" "$workspace_service_helper" >/dev/null; then
    echo "integration scope: missing Workspace service boundary canary: $claim" >&2
    exit 1
  fi
done
# The claim intentionally matches the literal shell variable in the sourced helper.
# shellcheck disable=SC2016
for claim in \
  'schema_version") != 2' \
  'tobari-permission wait --id pwt_' \
  'allow_exact_effect "$work_id" mock-upstream GET /permission-resume' \
  'fresh independently authorized permission-resume retry'; do
  if ! grep -F "$claim" "$permission_resume_helper" >/dev/null; then
    echo "integration scope: missing permission resume boundary canary: $claim" >&2
    exit 1
  fi
done

cli_reference_count=$(grep -Ehoc 'run_tobari(_at|_pty_at)?' "$scenario" "$workspace_service_helper" "$permission_resume_helper" | awk '{sum += $1} END {print sum}')
if ((cli_reference_count > 55)); then
  echo "integration scope: scenario grew to $cli_reference_count CLI references (limit 55)" >&2
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

# The Host Loopback slice must use the final Context identity and a bound
# Context reference for entry. Frozen private context wire keys are checked
# separately and do not authorize a predecessor public alias.
# shellcheck disable=SC2016
for claim in \
  'context create --template "$template_ref"' \
  'context enter --id "$context_ref"'; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: missing final Context Host Loopback canary: $claim" >&2
    exit 1
  fi
done
if grep -F 'contexts/default/policy/context.json' "$scenario" >&2; then
  echo "integration scope: post-publication policy drift bypassed the fixture publication seam" >&2
  exit 1
fi
provider_fixture_line=$(grep -nF 'cat >"$config_directory/auth/providers/$synthetic_provider.json"' "$scenario" | cut -d: -f1)
final_publication_line=$(grep -nF 'default_manifest_create=$(run_tobari manifest create' "$scenario" | cut -d: -f1)
if [[ -z $provider_fixture_line || -z $final_publication_line || $provider_fixture_line -le $final_publication_line ]]; then
  echo "integration scope: research provider fixture was installed before first final-authority publication" >&2
  exit 1
fi
for claim in \
  'item["workspace_manifest"]' \
  '["workspace_manifest"]["workspace_manifest_id"]' \
  '["runtime"]["runtime"]["runtime_ref"]' \
  'revision["availability"]["state"] == "available"' \
  'revision["source_digest"]' \
  "capture_runtime_image_for_cleanup \"\$runtime_id\" \"\$runtime_source_digest\"" \
  'TOBARI_INTEGRATION_FAIL_AFTER_RUNTIME_CAPTURE' \
  "run_tobari runtime build --id \"\$runtime_ref\"" \
  'go run ./tools/integrationfixture manifest-policy'; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: missing Workspace Manifest public JSON canary: $claim" >&2
    exit 1
  fi
done
capture_line=$(grep -nF "capture_runtime_image_for_cleanup \"\$runtime_id\" \"\$runtime_source_digest\"" "$scenario" | cut -d: -f1)
failure_line=$(grep -nF 'TOBARI_INTEGRATION_FAIL_AFTER_RUNTIME_CAPTURE' "$scenario" | tail -1 | cut -d: -f1)
container_line=$(grep -nF "work_container=\$(container_for_id \"\$work_id\")" "$scenario" | cut -d: -f1)
cleanup_line=$(grep -nF "docker image rm -f \"\$runtime_image\"" "$scenario" | cut -d: -f1)
if [[ -z $capture_line || -z $failure_line || -z $container_line || -z $cleanup_line ||
  $capture_line -ge $failure_line || $failure_line -ge $container_line ]]; then
  echo "integration scope: managed Runtime cleanup authority is not captured before the injectable pre-container failure" >&2
  exit 1
fi
if grep -F 'run_tobari runtime build --name' "$scenario" >&2; then
  echo "integration scope: retired Runtime build target returned" >&2
  exit 1
fi
if grep -E 'revisions.*\["(image|image_digest|snapshot_path|revision)"\]' "$scenario" >&2; then
  echo "integration scope: provisional Runtime infrastructure projection returned" >&2
  exit 1
fi
for claim in 'item["project_id"]' 'item["context"]'; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: frozen Gateway wire canary is missing: $claim" >&2
    exit 1
  fi
done

required_runtime_claims=(
  'network container:tobari-gateway'
  'cap-add NET_ADMIN'
  'ReadonlyRootfs'
  'handle copied across Manifests returned'
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

# The documented explicit-binary path skips source-image and binary builds, but
# it still owns a fresh temporary TLS wrapper for this run. Keep certificate
# generation before build selection and wrapper publication after it.
binary_branch_line=$(grep -nF "if [[ -n \${TOBARI_INTEGRATION_BINARY:-} ]]; then" "$scenario" | cut -d: -f1)
tls_fixture_line=$(grep -nF 'openssl req -x509 -newkey' "$scenario" | cut -d: -f1)
gateway_wrapper_line=$(grep -nF "docker build --tag \"\$gateway_fixture_image\"" "$scenario" | cut -d: -f1)
if [[ -z $binary_branch_line || -z $tls_fixture_line || -z $gateway_wrapper_line ]] ||
  ((tls_fixture_line >= binary_branch_line || gateway_wrapper_line <= binary_branch_line)); then
  echo "integration scope: run-local TLS fixture is not owned by both binary paths" >&2
  exit 1
fi
for claim in \
  "-v \"\$test_root/tls:/tls\"" \
  '-out /tls/synthetic-ca.crt' \
  'gateway_fixture_snapshot_tag' \
  'gateway_fixture_publish_tag' \
  'gateway_fixture_restore_tag' \
  'explicit integration Gateway image is a stale TLS fixture' \
  'Gateway TLS fixture did not embed the run-local CA' \
  'Gateway TLS fixture does not trust the run-local CA' \
  'TOBARI_MOCK_TLS_CERT=/tls/synthetic-ca.crt' \
  'TOBARI_MOCK_TLS_KEY=/tls/synthetic-server.key'; do
  if ! grep -F -- "$claim" "$scenario" >/dev/null; then
    echo "integration scope: missing run-local TLS fixture claim: $claim" >&2
    exit 1
  fi
done

echo "integration scope: OK ($line_count lines, $cli_reference_count CLI references)"
