#!/usr/bin/env bash
# Focused live boundary assertions sourced by scripts/test-integration.sh.

assert_workspace_service_helper_mount() {
  local container=$1 helper_mount helper_source_path
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
}

verify_workspace_service_exposure() {
  local requests request_ref allow_result url exposure_ref helper_output list_output response _
  for _ in $(seq 1 60); do
    requests=$(run_tobari service requests)
    request_ref=$(awk '$1 == "Request" && $2 ~ /^srq_/ {print $2; exit}' <<<"$requests")
    [[ -n $request_ref ]] && break
    sleep 0.2
  done
  [[ -n $request_ref ]] || fail "Workspace service request did not reach separate-host discovery"
  assert_contains "$requests" "Service 127.0.0.1:32123" "exact Workspace service request"
  allow_result=$(run_tobari service allow --id "$request_ref")
  url=$(awk '$1 == "Host" && $2 == "URL" {print $3; exit}' <<<"$allow_result")
  exposure_ref=$(awk '$1 == "Exposure" && $2 ~ /^exp_/ {print $2; exit}' <<<"$allow_result")
  [[ $url == http://127.0.0.1:* && -n $exposure_ref ]] ||
    fail "service Allow once did not return exact numeric loopback URL and opaque exposure reference"
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
  list_output=$(run_project cat /var/lib/tobari/service-exposure-list.out)
  assert_contains "$list_output" "$exposure_ref" "current-attachment exposure list"
  if curl --fail --silent --show-error "$url/" >/dev/null 2>&1; then
    fail "stopped Workspace service exposure remained reachable"
  fi
}
