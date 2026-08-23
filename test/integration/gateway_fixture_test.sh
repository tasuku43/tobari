#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/../.."
source test/integration/gateway_fixture.sh

fixture_test_root=$(mktemp -d "${TMPDIR:-/tmp}/tobari-gateway-fixture.XXXXXX")
trap 'rm -rf "$fixture_test_root"' EXIT

gateway_dev_tag=tobari-gateway-experimental:dev-test
gateway_fixture_image=tobari-gateway-integration-tls-test
gateway_previous_image_id=
gateway_fixture_image_id=
gateway_fixture_tag_installed=false
exact_state=$fixture_test_root/exact
fixture_state=$fixture_test_root/fixture
calls=$fixture_test_root/calls

docker() {
  printf '%s\n' "$*" >>"$calls"
  case "$*" in
    "image inspect --format {{.Id}} $gateway_dev_tag")
      [[ -f $exact_state ]] || return 1
      cat "$exact_state"
      ;;
    "image inspect --format {{.Id}} $gateway_fixture_image")
      [[ -f $fixture_state ]] || return 1
      cat "$fixture_state"
      ;;
    "image tag sha256:fixture $gateway_dev_tag")
      printf '%s\n' sha256:fixture >"$exact_state"
      ;;
    "image tag sha256:previous $gateway_dev_tag")
      printf '%s\n' sha256:previous >"$exact_state"
      ;;
    "image rm $gateway_dev_tag")
      rm -f "$exact_state"
      ;;
    "image rm $gateway_fixture_image")
      rm -f "$fixture_state"
      ;;
    *)
      echo "unexpected fake Docker call: $*" >&2
      return 1
      ;;
  esac
}

reset_fixture() {
  : >"$calls"
  printf '%s\n' sha256:fixture >"$fixture_state"
  gateway_previous_image_id=
  gateway_fixture_image_id=
  gateway_fixture_tag_installed=false
}

# A failure after publication runs the same EXIT cleanup as the full harness
# and restores the exact predecessor identity rather than merely a tag name.
reset_fixture
printf '%s\n' sha256:previous >"$exact_state"
(
  trap 'gateway_fixture_restore_tag' EXIT
  gateway_fixture_snapshot_tag
  gateway_fixture_publish_tag
  false
) 2>/dev/null || true
[[ $(cat "$exact_state") == sha256:previous ]]
[[ ! -e $fixture_state ]]

# The signal path also reaches restoration. A short-lived child reports the
# actual subshell PID because macOS Bash does not expose BASHPID.
reset_fixture
printf '%s\n' sha256:previous >"$exact_state"
set +e
(
  trap 'status=$?; gateway_fixture_restore_tag; exit "$status"' EXIT
  trap 'exit 130' INT
  gateway_fixture_snapshot_tag
  gateway_fixture_publish_tag
  subshell_pid=$(sh -c 'printf "%s\n" "$PPID"')
  kill -INT "$subshell_pid"
)
interrupt_status=$?
set -e
[[ $interrupt_status == 130 ]]
[[ $(cat "$exact_state") == sha256:previous ]]
[[ ! -e $fixture_state ]]

# When no contributor tag existed, cleanup removes only the run-owned exact
# tag and temporary fixture tag.
reset_fixture
rm -f "$exact_state"
gateway_fixture_snapshot_tag
gateway_fixture_publish_tag
gateway_fixture_restore_tag
[[ ! -e $exact_state ]]
[[ ! -e $fixture_state ]]

# A concurrent replacement is not overwritten with either remembered image.
reset_fixture
printf '%s\n' sha256:previous >"$exact_state"
gateway_fixture_snapshot_tag
gateway_fixture_publish_tag
printf '%s\n' sha256:concurrent >"$exact_state"
if gateway_fixture_restore_tag 2>/dev/null; then
  echo "Gateway fixture cleanup accepted concurrent tag drift" >&2
  exit 1
fi
[[ $(cat "$exact_state") == sha256:concurrent ]]

echo "integration Gateway fixture ownership: OK"
