#!/usr/bin/env bash
# Focused live permission-resume assertions sourced by scripts/test-integration.sh.
# shellcheck disable=SC2154 # Integration owner state is declared by the sourcing scenario.

verify_live_permission_observation_snapshot() {
  local expected_allow=$1 policy_input query output _
  # Gateway policy input schema 2 retains workspace_manifest_id as a non-authoritative ContextID wire field.
  policy_input='{"schema_version":2,"principal":{"cluster":"default","context_id":"'"$default_context_id"'","project_id":"'"$work_id"'"},"request":{"authority":{"scheme":"https","host":"'"$mock_host"'","port":8080},"method":"GET","path":{"raw":"/permission-resume","segments":["permission-resume"]},"query":{},"headers":{}},"authorization":{"broker_provider":null}}'
  query='[result | observation := http.send({"method":"post","url":"http://127.0.0.1:8181/v1/data/tobari/http/permission_wait_observation","headers":{"content-type":"application/json"},"body":{"input":'"$policy_input"'}}); observation.status_code == 200; object.get(observation.body, "result", null) != null; result := observation.body.result][0]'
  for _ in $(seq 1 600); do
    output=$(docker exec tobari-opa /opa eval --fail --format raw "$query")
    if EXPECTED_ALLOW=$expected_allow OPA_OBSERVATION="$output" python3 2>/dev/null <<'PY'
import json
import os
import re

document = json.loads(os.environ["OPA_OBSERVATION"])
if (not isinstance(document, dict)
        or re.fullmatch(r"[0-9a-f]{64}", document.get("revision", "")) is None):
    raise SystemExit(f"live OPA observation omitted its exact revision: {document!r}")
decision = document.get("decision")
expected = os.environ["EXPECTED_ALLOW"] == "true"
if not isinstance(decision, dict) or decision.get("allow") is not expected:
    raise SystemExit(f"live OPA observation did not bind its decision snapshot: {document!r}")
PY
    then
      return
    fi
    sleep 0.1
  done
  printf 'integration diagnostics: last live OPA observation: %s\n' "$output" >&2
  fail "live OPA observation did not reach the expected atomic decision snapshot"
}

verify_permission_resume_handoff() {
  local attachment_registry permission_denial permission_wait_error permission_wait_result permission_retry _
  attachment_registry=$config_directory/interactive-attachments/sessions.json
  # Entry publishes the session only after final-authority reconciliation has
  # completed. On a cold or policy-heavy integration cluster that can include
  # rebuilding the protected services, so observe the canonical registry
  # instead of treating an elapsed startup interval as readiness.
  for _ in $(seq 1 600); do
    if python3 -c 'import json,sys; raise SystemExit(not json.load(open(sys.argv[1], encoding="utf-8"))["sessions"])' "$attachment_registry" 2>/dev/null; then
      break
    fi
    sleep 0.1
  done
  python3 -c 'import json,sys; raise SystemExit(not json.load(open(sys.argv[1], encoding="utf-8"))["sessions"])' "$attachment_registry" 2>/dev/null ||
    { cat "$test_root/host-attachment.out" >&2; fail "permission-resume attachment did not publish its live session"; }
  ensure_gateway_fixture_network
  verify_live_permission_observation_snapshot false
  for _ in $(seq 1 120); do
    if run_project test -s /var/lib/tobari/permission-denial.json >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "$host_service_attachment_pid" >/dev/null 2>&1; then
      wait "$host_service_attachment_pid" || true
      host_service_attachment_pid=
      cat "$test_root/host-attachment.out" >&2
      fail "permission-resume attachment exited before publishing its denial"
    fi
    sleep 0.1
  done
  run_project test -s /var/lib/tobari/permission-denial.json >/dev/null 2>&1 ||
    fail "eligible ordinary denial did not publish a permission-resume handoff"
  permission_denial=$(run_project cat /var/lib/tobari/permission-denial.json)
  PERMISSION_DENIAL="$permission_denial" python3 - "$default_context_id" "$work_id" <<'PY'
import json
import os
import re
import sys
document = json.loads(os.environ["PERMISSION_DENIAL"])
tobari = document["tobari"]
if tobari.get("schema_version") != 3:
    raise SystemExit(f"permission denial schema is not 3: {tobari!r}")
if tobari.get("workspace_manifest_id") != sys.argv[1] or tobari.get("workspace_id") != sys.argv[2]:
    raise SystemExit(f"permission denial identity drifted: {tobari!r}")
if "context_id" in tobari or "project_id" in tobari:
    raise SystemExit(f"permission denial exposed frozen aliases: {tobari!r}")
if tobari.get("event") != "permission_review_available" or tobari.get("run_on") != "host":
    raise SystemExit(f"permission denial host-review navigation drifted: {tobari!r}")
review = tobari.get("review", {})
if review != {
    "available": True,
    "command": "tobari review permissions",
    "automatic_retry": False,
    "retry_after_review": True,
}:
    raise SystemExit(f"permission denial review contract is invalid: {review!r}")
request = tobari.get("request", {})
if request != {
    "scheme": "https",
    "host": "mock-upstream.synthetic.example",
    "port": 8080,
    "method": "GET",
    "path": "/permission-resume",
}:
    raise SystemExit(f"permission denial request projection drifted: {request!r}")
resume = tobari.get("resume", {})
command = resume.get("command", "")
if (resume.get("available") is not True or resume.get("run_on") != "workspace"
        or resume.get("automatic_retry") is not False
        or resume.get("result_values") != ["allow", "deny", "expired"]
        or re.fullmatch(r"tobari-permission wait --id pwt_[0-9a-f]{32}", command) is None):
    raise SystemExit(f"permission denial resume contract is invalid: {resume!r}")
PY
  if docker logs "$mock_name" 2>&1 | grep -F '"path":"/permission-resume"' >/dev/null; then
    fail "denied permission-resume request reached the upstream"
  fi
  # A live attachment deliberately fences physical Gateway replacement. Drop
  # the unmanaged synthetic-upstream network after the denial is recorded so
  # this authority-only change proves the supported OPA-only settlement path.
  docker network disconnect "$auth_network" tobari-gateway
  if network_contains_container "$auth_network" tobari-gateway; then
    fail "Gateway retained the synthetic upstream network before live Policy Allow"
  fi
  allow_exact_effect "$work_id" "$mock_host" GET /permission-resume
  verify_live_permission_observation_snapshot true
  for _ in $(seq 1 600); do
    if run_project test -s /var/lib/tobari/permission-wait.out >/dev/null 2>&1 ||
      run_project test -s /var/lib/tobari/permission-wait.err >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "$host_service_attachment_pid" >/dev/null 2>&1; then
      wait "$host_service_attachment_pid" || true
      host_service_attachment_pid=
      cat "$test_root/host-attachment.out" >&2
      fail "permission-resume attachment exited before returning its wait result"
    fi
    sleep 0.1
  done
  permission_wait_error=$(run_project cat /var/lib/tobari/permission-wait.err)
  permission_wait_result=$(run_project cat /var/lib/tobari/permission-wait.out)
  if [[ -n $permission_wait_error || $permission_wait_result != Allow ]]; then
    printf 'integration diagnostics: permission helper stderr: %s\n' "$permission_wait_error" >&2
    fail "permission helper returned ${permission_wait_result:-<empty>} instead of Allow"
  fi
  ensure_gateway_fixture_network
  run_project touch /var/lib/tobari/permission-network-restored
  for _ in $(seq 1 120); do
    if run_project test -s /var/lib/tobari/permission-retry.json >/dev/null 2>&1; then
      break
    fi
    sleep 0.1
  done
  run_project test -s /var/lib/tobari/permission-retry.json >/dev/null 2>&1 ||
    fail "permission helper did not publish its deliberate fresh retry"
  permission_retry=$(run_project cat /var/lib/tobari/permission-retry.json)
  assert_contains "$permission_retry" '"path":"/permission-resume"' \
    "fresh independently authorized permission-resume retry"
}
