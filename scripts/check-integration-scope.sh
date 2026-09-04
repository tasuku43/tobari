#!/usr/bin/env bash
# The guard searches literal shell source, so its single-quoted patterns must
# retain unexpanded variable markers.
# shellcheck disable=SC2016
set -euo pipefail
cd "$(dirname "$0")/.."

scenario=scripts/test-integration.sh
first_use_scenario=scripts/test-final-first-use-integration.sh
upgrade_scenario=scripts/test-final-release-upgrade-integration.sh
upgrade_helper=test/integration/release_upgrade_reentry.sh
predecessor_builder=scripts/build-release-predecessor.sh
workspace_service_helper=test/integration/workspace_service_exposure.sh
gateway_fixture_helper=test/integration/gateway_fixture.sh
runtime_image_cleanup_helper=test/integration/runtime_image_cleanup.sh
permission_resume_helper=test/integration/permission_resume.sh

first_use_line_count=$(wc -l <"$first_use_scenario" | tr -d ' ')
if ((first_use_line_count > 350)); then
	echo "integration scope: first-use scenario grew to $first_use_line_count lines (limit 350)" >&2
	exit 1
fi
for claim in \
  '[[ ! -e $test_root/config && ! -e $test_root/state && ! -e $test_root/data ]]' \
  'go build -buildvcs=false -trimpath' \
  'run_bare_tobari_pty_at fresh "$test_root/user/project"' \
  'items[0]["runtime_id"] == "builtin/standard"' \
  'run_bare_tobari_pty_at reentry "$test_root/user/project"' \
  'run_bare_tobari_pty_at ancestor-use "$test_root/user/project/child"' \
  'run_bare_tobari_pty_at nested-create "$test_root/user/project/child"' \
  'E2E_CWD=/var/lib/tobari/project/child' \
  'printf '\''E2E_CWD=%s\\n'\''' \
  'Tobari image $image must be absent' \
  'Context source root was not created' \
  'docker volume inspect --format' \
  'tobari-policy-bundle'; do
  if ! grep -F "$claim" "$first_use_scenario" >/dev/null; then
    echo "integration scope: missing real first-use canary: $claim" >&2
    exit 1
  fi
done

upgrade_scenario_line_count=$(wc -l <"$upgrade_scenario" | tr -d ' ')
upgrade_helper_line_count=$(wc -l <"$upgrade_helper" | tr -d ' ')
predecessor_builder_line_count=$(wc -l <"$predecessor_builder" | tr -d ' ')
if ((upgrade_scenario_line_count > 30 || upgrade_helper_line_count > 170 || predecessor_builder_line_count > 110)); then
	echo "integration scope: release upgrade scenario exceeded its reviewed size budget" >&2
	exit 1
fi
for claim in \
	'TOBARI_FIRST_USE_PREDECESSOR_BINARY' \
	'prepare_release_upgrade_reentry "$context_list" "$workspace_list"' \
	'verify_release_upgrade_after_reentry'; do
	grep -F "$claim" "$first_use_scenario" >/dev/null || {
		echo "integration scope: first-use scenario lost upgrade seam: $claim" >&2
		exit 1
	}
done
for claim in \
	'git archive --format=tar "$predecessor_tag"' \
	'./scripts/package-release.sh' \
	'"resolver_channel": "embedded"' \
	'"capability_surface": "release"'; do
	grep -F "$claim" "$predecessor_builder" >/dev/null || {
		echo "integration scope: predecessor build lost release identity evidence: $claim" >&2
		exit 1
	}
done
for claim in \
	'https://upgrade-harness.example/reentry' \
	'run_tobari policy allow --id "$upgrade_candidate_ref"' \
	'"0 pending candidates" not in detail' \
	'post-context-delete-reentry.log' \
	'run_tobari help workspace delete --format agent' \
	'run_tobari workspace delete --id "$old_workspace_ref"' \
	'cmp "$test_root/expected-claude-settings.json" "$upgrade_settings_path"'; do
	grep -F "$claim" "$upgrade_helper" >/dev/null || {
		echo "integration scope: upgrade journey lost runtime assertion: $claim" >&2
		exit 1
	}
done

expected_phases=$'preflight\nbuild-fixtures\ntemplates-and-cluster\ncredentials-and-workspaces\ngateway-broker-and-transport\nlive-policy-activation\nattachment-scoped-host-loopback\nruntime-failure-boundaries\nlifecycle'
actual_phases=$(awk '/^begin_phase / { print $2 }' "$scenario")
if [[ $actual_phases != "$expected_phases" ]]; then
  echo "integration scope: phase ownership changed" >&2
  diff -u <(printf '%s\n' "$expected_phases") <(printf '%s\n' "$actual_phases") >&2 || true
  exit 1
fi

line_count=$(wc -l <"$scenario" | tr -d ' ')
if ((line_count > 2140)); then
  echo "integration scope: scenario grew to $line_count lines (limit 2140)" >&2
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
if ((permission_resume_helper_line_count > 151)); then
  echo "integration scope: Permission resume helper grew to $permission_resume_helper_line_count lines (limit 151)" >&2
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
  'policy_input='\''{"schema_version":2' \
  'schema_version") != 3' \
  'tobari-permission wait --id pwt_' \
  'allow_exact_effect "$work_id" "$mock_host" GET /permission-resume' \
  'permission-network-restored' \
  'fresh independently authorized permission-resume retry'; do
  if ! grep -F "$claim" "$permission_resume_helper" >/dev/null; then
    echo "integration scope: missing permission resume boundary canary: $claim" >&2
    exit 1
  fi
done

cli_reference_count=$(grep -Ehoc 'run_tobari(_at|_pty_at)?' "$scenario" "$workspace_service_helper" "$permission_resume_helper" | awk '{sum += $1} END {print sum}')
if ((cli_reference_count > 80)); then
  echo "integration scope: scenario grew to $cli_reference_count CLI references (limit 80)" >&2
  exit 1
fi
if grep -E 'review permissions([^[:alnum:]]|$).*--tail' "$scenario" "$permission_resume_helper" >&2; then
  echo "integration scope: complete Permission Inbox E2E still uses removed --tail input" >&2
  exit 1
fi

# These command families are owned by fast domain/application/CLI tests. The
# Docker scenario may use only the minimal create/import/discover/act calls
# needed to assemble and exercise real runtime boundaries.
if grep -En \
  'policy preset (list|validate)|policy (rules|reset)|auth (status|login)|runtime init|help policy' \
  "$scenario" >&2; then
  echo "integration scope: semantic or presentation matrix returned to the Docker scenario" >&2
  exit 1
fi

# The Host Loopback slice must use the final Context identity, select it without
# CWD, and enter through bare root. Frozen private context wire keys are checked
# separately and do not authorize a predecessor public alias.
# shellcheck disable=SC2016
for claim in \
  'context create --template "$template_ref"' \
  'context use --id "$context_ref"'; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: missing final Context Host Loopback canary: $claim" >&2
    exit 1
  fi
done
if grep -F 'contexts/default/policy/context.json' "$scenario" >&2; then
  echo "integration scope: post-publication policy drift bypassed the fixture publication seam" >&2
  exit 1
fi
# These source patterns intentionally retain literal shell variables; expanding
# them while the guard runs would stop matching the harness source itself.
# shellcheck disable=SC2016
provider_fixture_line=$(grep -nF 'cat >"$config_directory/auth/providers/$synthetic_provider.json"' "$scenario" | cut -d: -f1)
final_publication_line=$(grep -nF 'default_template_ref=$(apply_template_ref "$default_template_ref")' "$scenario" | cut -d: -f1)
if [[ -z $provider_fixture_line || -z $final_publication_line || $provider_fixture_line -le $final_publication_line ]]; then
  echo "integration scope: research provider fixture was installed before first final-authority publication" >&2
  exit 1
fi
for claim in \
  'template create --name default --source-access read-write' \
  'template_create_args+=(--graphql-endpoint https://graphql.tobari.dev:8080/graphql)' \
  'template create --name restricted --source-access read-only' \
	  'rewrite_template_runtime_source "$default_template_source" "$runtime_id" "$runtime_source_digest"' \
	  'default_template_ref=$(apply_template_ref "$default_template_ref")' \
	  'apply_context_ref "$default_context_ref"' \
  'context create --template "$default_template_ref"' \
  'context use --id "$default_context_ref"' \
  'workspace list --format json' \
  'workspace delete --id "$work_ref" --confirm=delete --force' \
  'volume|tobari-auth-runtime|/run/tobari-auth/runtime|false' \
  'restore_gateway_auth_fixture_network' \
  'wait_network_connection tobari-gateway "$mock_host" 8080' \
  'auth import "$synthetic_provider" --context "$default_context_ref"' \
  '["runtime"]["runtime"]["runtime_ref"]' \
  'revision["availability"]["state"] == "available"' \
  'revision["source_digest"]' \
  "capture_runtime_image_for_cleanup \"\$runtime_id\" \"\$runtime_source_digest\"" \
  'TOBARI_INTEGRATION_FAIL_AFTER_RUNTIME_CAPTURE' \
  "run_tobari runtime build --id \"\$runtime_ref\""; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: missing final Workspace Template/Context canary: $claim" >&2
    exit 1
  fi
done
for claim in \
  'line == "ARG TOBARI_RUNTIME_BASE"' \
  'line != "FROM ${TOBARI_RUNTIME_BASE}"'; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: custom Runtime fixture does not require explicit resolved base material: $claim" >&2
    exit 1
  fi
done
for retired in '--manifest' 'manifest create' 'workspace_manifest' 'default_manifest' 'integrationfixture manifest-policy' 'run_tobari_at "$work_root" delete'; do
  if grep -F -- "$retired" "$scenario" >&2; then
    echo "integration scope: retired predecessor harness token remains active: $retired" >&2
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

for claim in \
  'host_docker_context=${TOBARI_INTEGRATION_DOCKER_CONTEXT:-${DOCKER_CONTEXT:-}}' \
  'command docker --context "$host_docker_context"' \
  'explicit non-default Docker context is required' \
  'docker context inspect "$host_docker_context"'; do
  if ! grep -F "$claim" "$scenario" >/dev/null; then
    echo "integration scope: missing explicit isolated Docker-context guard: $claim" >&2
    exit 1
  fi
done
if grep -F 'docker context show' "$scenario" >&2; then
  echo "integration scope: scenario queried the ambient/default Docker context" >&2
  exit 1
fi

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
if ! grep -F './scripts/check.sh runtime-release-components' .github/workflows/ci.yml >/dev/null ||
	! grep -F './scripts/check.sh first-use' .github/workflows/ci.yml >/dev/null ||
	! grep -F './scripts/check.sh upgrade' .github/workflows/ci.yml >/dev/null; then
	echo "integration scope: CI does not run release component, cold first-use, and previous-release upgrade profiles" >&2
	exit 1
fi
runtime_release_components_body=$(awk '
  /^run_runtime_release_components\(\)/ { in_runtime_release_components=1 }
  in_runtime_release_components { print }
  in_runtime_release_components && /^}/ { exit }
' scripts/check.sh)
runtime_release_body=$(awk '
  /^run_runtime_release\(\)/ { in_runtime_release=1 }
  in_runtime_release { print }
  in_runtime_release && /^}/ { exit }
' scripts/check.sh)
if ! grep -F 'run_policy' <<<"$runtime_release_components_body" >/dev/null ||
  ! grep -F 'run_gateway' <<<"$runtime_release_components_body" >/dev/null ||
  ! grep -F 'activate_integration_docker_context' <<<"$runtime_release_body" >/dev/null ||
  ! grep -F 'run_runtime_release_components' <<<"$runtime_release_body" >/dev/null ||
  ! grep -F 'run_first_use' <<<"$runtime_release_body" >/dev/null; then
  echo "integration scope: release runtime profile lost isolated-context, policy, Gateway, or cold first-use coverage" >&2
  exit 1
fi
runtime_release_context_line=$(grep -nF 'activate_integration_docker_context' <<<"$runtime_release_body" | head -n 1 | cut -d: -f1)
runtime_release_components_line=$(grep -nF 'run_runtime_release_components' <<<"$runtime_release_body" | head -n 1 | cut -d: -f1)
if [[ -z $runtime_release_context_line || -z $runtime_release_components_line ]] ||
  ((runtime_release_context_line >= runtime_release_components_line)); then
  echo "integration scope: release components run before the isolated Docker context is active" >&2
  exit 1
fi
if grep -Eq 'run_authbroker|run_integration' <<<"$runtime_release_components_body$runtime_release_body"; then
  echo "integration scope: release runtime profile re-enabled deferred research tests" >&2
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

echo "integration scope: OK ($line_count lines, $first_use_line_count first-use lines, $cli_reference_count CLI references)"
