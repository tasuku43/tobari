#!/usr/bin/env bash
# Focused live permission-resume assertions sourced by scripts/test-integration.sh.
# shellcheck disable=SC2154 # Integration owner state is declared by the sourcing scenario.

verify_permission_observer_opa_expression_shape() {
  local opa_image revision decision query output
  opa_image=$(awk -F= '$1 == "OPA_IMAGE" { print $2 }' internal/infra/runtimeassets/assets/versions.env)
  revision=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
  for decision in \
    '{"allow":true,"reason":"allowed by Context policy","status_code":403,"learnable":false}' \
    '{"allow":false,"reason":"denied by exact policy","status_code":403,"learnable":false}'; do
    query='[result | observation := {"status_code":200,"body":{"result":{"revision":"'"$revision"'","decision":'"$decision"'}}}; observation.status_code == 200; object.get(observation.body, "result", null) != null; result := observation.body.result][0]'
    output=$(docker run --rm "$opa_image" eval --fail --format raw "$query")
    OPA_OBSERVATION="$output" python3 - "$revision" <<'PY'
import json
import os
import sys

document = json.loads(os.environ["OPA_OBSERVATION"])
if not isinstance(document, dict) or document.get("revision") != sys.argv[1]:
    raise SystemExit(f"OPA observer query did not return one exact revision-bound object: {document!r}")
decision = document.get("decision")
if not isinstance(decision, dict) or not isinstance(decision.get("allow"), bool):
    raise SystemExit(f"OPA observer query did not retain its decision object: {document!r}")
PY
  done
  query='[result | observation := {"status_code":503,"body":{}}; observation.status_code == 200; result := observation.body.result][0]'
  if docker run --rm "$opa_image" eval --fail --format raw "$query" >/dev/null 2>&1; then
    fail "undefined OPA observer query did not fail closed"
  fi
}

verify_live_permission_observation_snapshot() {
  local expected_allow=$1 policy_input query output _
  policy_input='{"schema_version":1,"principal":{"cluster":"default","context_id":"'"$default_manifest_id"'","project_id":"'"$work_id"'"},"request":{"authority":{"scheme":"https","host":"mock-upstream","port":8080},"method":"GET","path":{"raw":"/permission-resume","segments":["permission-resume"]},"query":{},"headers":{}},"authorization":{"broker_provider":null}}'
  query='[result | observation := http.send({"method":"post","url":"http://127.0.0.1:8181/v1/data/tobari/http/permission_wait_observation","headers":{"content-type":"application/json"},"body":{"input":'"$policy_input"'}}); observation.status_code == 200; object.get(observation.body, "result", null) != null; result := observation.body.result][0]'
  for _ in $(seq 1 120); do
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
  fail "live OPA observation did not reach the expected atomic decision snapshot"
}

verify_permission_resume_handoff() {
  local permission_denial permission_wait_result permission_retry _
  verify_permission_observer_opa_expression_shape
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
  PERMISSION_DENIAL="$permission_denial" python3 - "$default_manifest_id" "$work_id" <<'PY'
import json
import os
import re
import sys

document = json.loads(os.environ["PERMISSION_DENIAL"])
tobari = document["tobari"]
if tobari.get("schema_version") != 2:
    raise SystemExit(f"permission denial schema is not 2: {tobari!r}")
if tobari.get("workspace_manifest_id") != sys.argv[1] or tobari.get("workspace_id") != sys.argv[2]:
    raise SystemExit(f"permission denial identity drifted: {tobari!r}")
if "context_id" in tobari or "project_id" in tobari:
    raise SystemExit(f"permission denial exposed frozen aliases: {tobari!r}")
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
  allow_exact_effect "$work_id" mock-upstream GET /permission-resume
  verify_live_permission_observation_snapshot true
  for _ in $(seq 1 120); do
    if run_project test -s /var/lib/tobari/permission-retry.json >/dev/null 2>&1; then
      break
    fi
    sleep 0.1
  done
  permission_wait_result=$(run_project cat /var/lib/tobari/permission-wait.out)
  [[ $permission_wait_result == Allow ]] || {
    run_project cat /var/lib/tobari/permission-wait.err >&2 || true
    fail "permission helper returned $permission_wait_result instead of Allow"
  }
  permission_retry=$(run_project cat /var/lib/tobari/permission-retry.json)
  assert_contains "$permission_retry" '"path":"/permission-resume"' \
    "fresh independently authorized permission-resume retry"
}
