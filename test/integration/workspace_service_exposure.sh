#!/usr/bin/env bash
# Focused live boundary assertions sourced by scripts/test-integration.sh.

assert_workspace_service_helper_mount() {
  local container=$1 helper_mount helper_source_path permission_mount permission_source_path
  helper_mount=$(docker inspect --format \
    '{{range .Mounts}}{{if eq .Destination "/usr/local/bin/tobari-expose"}}{{.Type}} {{.RW}} {{.Source}}{{end}}{{end}}' \
    "$container")
  [[ $helper_mount == bind\ false\ * ]] ||
    fail "Workspace service helper is not an exact read-only bind mount: $helper_mount"
  helper_source_path=${helper_mount#bind false }
  [[ -f $helper_source_path && ! -L $helper_source_path && $(stat -c '%a' "$helper_source_path" 2>/dev/null || stat -f '%Lp' "$helper_source_path") == 700 ]] ||
    fail "Workspace service helper owner-state source is not a mode-0700 regular file"
  run_project tobari-expose help >/dev/null ||
    fail "engine-native Workspace service helper did not execute in the selected custom Runtime"
  if run_project sh -c 'printf tamper > /usr/local/bin/tobari-expose' >/dev/null 2>&1; then
    fail "Workspace could write the read-only service helper mount"
  fi
  permission_mount=$(docker inspect --format \
    '{{range .Mounts}}{{if eq .Destination "/usr/local/bin/tobari-permission"}}{{.Type}} {{.RW}} {{.Source}}{{end}}{{end}}' \
    "$container")
  [[ $permission_mount == bind\ false\ * ]] ||
    fail "Workspace permission helper is not an exact read-only bind mount: $permission_mount"
  permission_source_path=${permission_mount#bind false }
  [[ -f $permission_source_path && ! -L $permission_source_path && $(stat -c '%a' "$permission_source_path" 2>/dev/null || stat -f '%Lp' "$permission_source_path") == 700 ]] ||
    fail "Workspace permission helper owner-state source is not a mode-0700 regular file"
  run_project tobari-permission help >/dev/null ||
    fail "engine-native Workspace permission helper did not execute in the selected custom Runtime"
  if run_project sh -c 'printf tamper > /usr/local/bin/tobari-permission' >/dev/null 2>&1; then
    fail "Workspace could write the read-only permission helper mount"
  fi
}

verify_workspace_service_exposure() {
  local review request_ref allow_result url exposure_ref helper_output helper_status response _
  for _ in $(seq 1 60); do
    review=$(run_tobari review services --format=json)
    request_ref=$(python3 -c 'import json,sys; rows=json.load(sys.stdin)["service_review"]["requests"]; print(rows[0]["id"] if rows else "")' <<<"$review")
    [[ -n $request_ref ]] && break
    sleep 0.2
  done
  [[ -n $request_ref ]] || fail "Workspace service request did not reach separate-host discovery"
  python3 -c 'import json,sys; row=json.load(sys.stdin)["service_review"]["requests"][0]; assert row["target_port"] == 32123 and row["state"] == "pending"' <<<"$review" ||
    fail "Service review did not preserve the exact pending Workspace target"
  allow_result=$(run_tobari service allow --id "$request_ref" --format=json)
  url=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["exposure"]["url"])' <<<"$allow_result")
  exposure_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["exposure"]["id"])' <<<"$allow_result")
  python3 - "$url" "$exposure_ref" <<'PY' || fail "Allow once did not return the exact generated origin and opaque exposure reference"
import re
import sys
from urllib.parse import urlsplit

parsed = urlsplit(sys.argv[1])
suffix = "." + "localhost"
assert parsed.scheme == "http" and parsed.path == "/" and not parsed.query and not parsed.fragment
assert parsed.hostname is not None and parsed.hostname.endswith(suffix)
assert re.fullmatch(r"svc-[0-9a-f]{32}", parsed.hostname[:-len(suffix)])
assert parsed.port is not None and 1024 <= parsed.port <= 65535
assert re.fullmatch(r"exp_[0-9a-f]{32}", sys.argv[2])
PY
  for _ in $(seq 1 60); do
    run_project test -e /var/lib/tobari/service-exposure-ready >/dev/null 2>&1 && break
    sleep 0.1
  done
  run_project test -e /var/lib/tobari/service-exposure-ready >/dev/null 2>&1 ||
    fail "Workspace helper did not receive the confirmed service exposure"
  response=$(curl --fail --silent --show-error "$url/")
  assert_contains "$response" "Directory listing" "exact-authority Workspace HTTP relay"
  helper_output=$(run_project cat /var/lib/tobari/service-exposure.out)
  assert_contains "$helper_output" "$url" "helper confirmed host URL"
  assert_contains "$helper_output" "$exposure_ref" "helper confirmed opaque exposure reference"
  run_project touch /var/lib/tobari/service-stop
  for _ in $(seq 1 60); do
    run_project test -e /var/lib/tobari/service-stopped >/dev/null 2>&1 && break
    sleep 0.1
  done
  run_project test -e /var/lib/tobari/service-stopped >/dev/null 2>&1 ||
    fail "opaque Workspace service stop did not complete"
  helper_status=$(run_project cat /var/lib/tobari/service-exposure-status.out)
  assert_contains "$helper_status" "$exposure_ref" "current-attachment exposure status"
  if curl --fail --silent --show-error "$url/" >/dev/null 2>&1; then
    fail "stopped Workspace service exposure remained reachable"
  fi
}
