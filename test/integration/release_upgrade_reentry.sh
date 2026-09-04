#!/usr/bin/env bash

# This file is sourced by scripts/test-final-first-use-integration.sh. It owns
# the predecessor-to-candidate transition while the parent owns isolated XDG
# roots, Docker resources, PTY entry, and exact cleanup.
# shellcheck disable=SC2034,SC2154 # Sourced functions intentionally read and update parent-owned journey state.

prepare_release_upgrade_reentry() {
	[[ -n ${predecessor_binary:-} ]] || return 0

	local context_list=$1
	local workspace_list=$2
	local denial_status candidates candidate_identity

	upgrade_expected_context_id=$(python3 -c '
import json,sys
items=json.load(sys.stdin)["contexts"]["items"]
assert len(items) == 1
print(items[0]["context_id"])
' <<<"$context_list")
	upgrade_expected_workspace_id=$(python3 -c '
import json,sys
items=json.load(sys.stdin)["workspaces"]["items"]
assert len(items) == 1
print(items[0]["workspace_id"])
' <<<"$workspace_list")
	local managed_home_name=home
	upgrade_settings_path="$test_root/state/tobari/workspace-authority-runtime/contexts/$upgrade_expected_context_id/$managed_home_name/.claude/settings.json"
	[[ -f $upgrade_settings_path ]] || {
		echo "release upgrade integration: predecessor did not create Context-owned Claude settings" >&2
		return 1
	}
	cat >"$upgrade_settings_path" <<'JSON'
{
  "permissions": {"defaultMode": "bypassPermissions"},
  "releaseUpgradeHarness": "byte-preservation-canary",
  "nested": {"array": [false, 0, ""]}
}
JSON
	chmod 0600 "$upgrade_settings_path"
	cp "$upgrade_settings_path" "$test_root/expected-claude-settings.json"

	denial_status=$(run_tobari_at "$test_root/user/project" -- /bin/bash -lc \
		'curl -sS -o /dev/null -w "%{http_code}" --connect-timeout 5 --max-time 15 https://upgrade-harness.example/reentry')
	[[ $denial_status == 403 ]] || {
		echo "release upgrade integration: predecessor request returned $denial_status instead of deterministic policy denial" >&2
		return 1
	}

	candidate_identity=$($candidate_binary version --format json)
	python3 -c '
import json,sys
identity=json.load(sys.stdin)["build_identity"]
assert identity["capability_surface"] == "release"
assert identity["resolver_channel"] == "embedded"
assert identity["compatible"] is True
' <<<"$candidate_identity"
	binary=$candidate_binary

	candidates=$(run_tobari policy candidates --format json)
	upgrade_candidate_ref=$(python3 -c '
import json,sys
items=json.load(sys.stdin)["policy_candidates"]
matching=[item for item in items if item["effect"]["host"] == "upgrade-harness.example" and item["effect"]["path"] == "/reentry"]
assert len(matching) == 1
print(matching[0]["id"])
' <<<"$candidates")
	run_tobari_at "$test_root/user/project" doctor --format json >"$test_root/pre-allow-doctor.json"

	run_tobari policy allow --id "$upgrade_candidate_ref" --format json >/dev/null
	python3 -c '
import json,sys
items=json.load(sys.stdin)["policy_rules"]
assert any(item["body"]["host"] == "upgrade-harness.example" for item in items)
' <<<"$(run_tobari policy rules --format json)"
	upgrade_prepared=true
}

verify_release_upgrade_after_reentry() {
	[[ ${upgrade_prepared:-false} == true ]] || return 0

	local contexts workspaces status template_ref context_create delete_help
	local old_workspace_ref=$workspace_ref
	local old_workspace_id=$upgrade_expected_workspace_id

	# Keep this assertion after the first post-Allow bare entry. A predecessor
	# with both historical defects must fail on the broken entry receipt rather
	# than letting the stale doctor wording hide that deeper continuity failure.
	python3 -c '
import json,sys
checks={item["check"]:item for item in json.load(sys.stdin)["report"]}
detail=checks["policy_data"]["detail"]
assert checks["policy_data"]["status"] == "pass"
assert "policy candidates" in detail
assert "0 pending candidates" not in detail
' <"$test_root/pre-allow-doctor.json"

	contexts=$(run_tobari context list --format json)
	workspaces=$(run_tobari workspace list --format json)
	python3 -c '
import json,sys
contexts=json.loads(sys.argv[1])["contexts"]["items"]
workspaces=json.load(sys.stdin)["workspaces"]["items"]
assert len(contexts) == 1 and contexts[0]["context_id"] == sys.argv[2]
assert len(workspaces) == 1 and workspaces[0]["workspace_id"] == sys.argv[3]
' "$contexts" "$upgrade_expected_context_id" "$upgrade_expected_workspace_id" <<<"$workspaces"
	cmp "$test_root/expected-claude-settings.json" "$upgrade_settings_path"

	template_ref=$(python3 -c '
import json,sys
items=json.load(sys.stdin)["templates"]["items"]
assert len(items) == 1
print(items[0]["template_ref"])
' <<<"$(run_tobari template list --format json)")
	mkdir -p "$test_root/user/unrelated"
	context_create=$(run_tobari_at "$test_root/user/unrelated" context create --template "$template_ref" --format json)
	upgrade_context_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["context"]["context_ref"])' <<<"$context_create")
	run_tobari context delete --id "$upgrade_context_ref" --confirm=delete --format json >/dev/null
	upgrade_context_ref=

	run_bare_tobari_pty_at reentry "$test_root/user/project" >"$test_root/post-context-delete-reentry.log" 2>&1 || {
		cat "$test_root/post-context-delete-reentry.log" >&2
		echo "release upgrade integration: unrelated Context deletion broke bare re-entry" >&2
		return 1
	}
	cmp "$test_root/expected-claude-settings.json" "$upgrade_settings_path"
	status=$(run_tobari_at "$test_root/user/project" status --format json)
	python3 -c '
import json,sys
value=json.load(sys.stdin)["status"]
assert value["workspace"]["presence"] == "present"
assert value["workspace"]["entry_state"] == "current"
assert value["cluster"]["runtime"] == "running"
' <<<"$status"

	delete_help=$(run_tobari help workspace delete --format agent)
	python3 -c '
import json,sys
commands=json.load(sys.stdin)["commands"]
assert len(commands) == 1
assert commands[0]["contract"]["outcome"] == "Retire one exact Workspace and owned runtime resources while preserving Context Policy Memory and the complete Context-owned managed Home"
' <<<"$delete_help"
	run_tobari workspace delete --id "$old_workspace_ref" --confirm=delete --force --format json >/dev/null
	cmp "$test_root/expected-claude-settings.json" "$upgrade_settings_path"
	run_bare_tobari_pty_at reentry "$test_root/user/project" >"$test_root/post-workspace-recreate-entry.log" 2>&1 || {
		cat "$test_root/post-workspace-recreate-entry.log" >&2
		echo "release upgrade integration: Workspace recreation failed after upgrade" >&2
		return 1
	}
	workspaces=$(run_tobari workspace list --format json)
	workspace_ref=$(python3 -c '
import json,sys
items=json.load(sys.stdin)["workspaces"]["items"]
assert len(items) == 1 and items[0]["workspace_id"] != sys.argv[1]
print(items[0]["workspace_ref"])
' "$old_workspace_id" <<<"$workspaces")
	cmp "$test_root/expected-claude-settings.json" "$upgrade_settings_path"

	echo "release upgrade integration: predecessor state, policy mutation, unrelated Context deletion, repeated re-entry, and Context Home preservation OK"
}
