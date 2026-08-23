#!/usr/bin/env bash
# Focused live permission-resume assertions sourced by scripts/test-integration.sh.
# shellcheck disable=SC2154 # Integration owner state is declared by the sourcing scenario.

verify_permission_resume_handoff() {
  local permission_denial permission_wait_result permission_retry _
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
