#!/usr/bin/env bash

# Sourced by the final first-use scenario. The parent owns isolated XDG roots,
# Docker resources, PTY entry, and cleanup.
# shellcheck disable=SC2034,SC2154

rewrite_template_source_access() {
	local source_path=$1
	local expected=$2
	local replacement=$3
	python3 - "$source_path" "$expected" "$replacement" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
expected, replacement = sys.argv[2:]
source = path.read_text(encoding="utf-8")
old = f"source_access: {expected}"
new = f"source_access: {replacement}"
if source.count(old) != 1:
    raise SystemExit(f"expected one {old!r} in Template source")
path.write_text(source.replace(old, new, 1), encoding="utf-8")
PY
}

rewrite_template_session_default() {
	local source_path=$1
	python3 - "$source_path" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
source = path.read_text(encoding="utf-8")
old = "shell_environment: []"
new = "shell_environment:\n    - source: literal\n      value: '1'\n      variable: NO_COLOR"
if source.count(old) != 1:
    raise SystemExit("expected one empty shell environment in Template source")
path.write_text(source.replace(old, new, 1), encoding="utf-8")
PY
}

clear_template_session_default() {
	local source_path=$1
	python3 - "$source_path" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
source = path.read_text(encoding="utf-8")
old = "shell_environment:\n    - source: literal\n      value: '1'\n      variable: NO_COLOR"
new = "shell_environment: []"
if source.count(old) != 1:
    raise SystemExit("expected one transition-harness shell setting in Template source")
path.write_text(source.replace(old, new, 1), encoding="utf-8")
PY
}

read_template_runtime_source() {
	local source_path=$1
	python3 - "$source_path" <<'PY'
from pathlib import Path
import sys

lines = Path(sys.argv[1]).read_text(encoding="utf-8").splitlines()
matches = [index for index, line in enumerate(lines) if line == "  runtime:"]
if len(matches) != 1:
    raise SystemExit("Template source has no unique entry-default Runtime")
index = matches[0]
if lines[index + 1].strip().split(":", 1)[0] != "id" or lines[index + 2].strip().split(":", 1)[0] != "revision":
    raise SystemExit("Template source Runtime shape changed")
print(lines[index + 1].split(":", 1)[1].strip(), lines[index + 2].split(":", 1)[1].strip(), sep="\t")
PY
}

rewrite_template_runtime_source() {
	local source_path=$1
	local expected_id=$2
	local expected_revision=$3
	local replacement_id=$4
	local replacement_revision=$5
	python3 - "$source_path" "$expected_id" "$expected_revision" "$replacement_id" "$replacement_revision" <<'PY'
from pathlib import Path
import sys

path = Path(sys.argv[1])
expected_id, expected_revision, replacement_id, replacement_revision = sys.argv[2:]
lines = path.read_text(encoding="utf-8").splitlines()
matches = [index for index, line in enumerate(lines) if line == "  runtime:"]
if len(matches) != 1:
    raise SystemExit("Template source has no unique entry-default Runtime")
index = matches[0]
if lines[index + 1] != f"    id: {expected_id}" or lines[index + 2] != f"    revision: {expected_revision}":
    raise SystemExit("Template source Runtime changed before deterministic rewrite")
lines[index + 1] = f"    id: {replacement_id}"
lines[index + 2] = f"    revision: {replacement_revision}"
path.write_text("\n".join(lines) + "\n", encoding="utf-8")
PY
}

apply_template_change() {
	local template_ref=$1
	local plan plan_ref
	plan=$(run_tobari template plan --id "$template_ref" --format json)
	plan_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template_change_plan"]["plan_ref"])' <<<"$plan")
	run_tobari template apply --plan "$plan_ref" --format json
}

verify_workspace_entry_transition_matrix() {
	local templates template_ref source_path applied child_log
	local original_runtime_id original_runtime_revision runtime_create runtime_ref runtime_build runtime_id runtime_revision
	local context_id managed_home settings_path expected_settings old_workspace_id new_workspace_list
	templates=$(run_tobari template list --format json)
	template_ref=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["templates"]["items"]; assert len(items) == 1; print(items[0]["template_ref"])' <<<"$templates")
	source_path=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template"]["source_path"])' <<<"$(run_tobari template show --format json)")

	rewrite_template_session_default "$source_path"
	echo "workspace entry transition matrix: apply session-only change" >&2
	applied=$(apply_template_change "$template_ref")
	template_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template"]["template_ref"])' <<<"$applied")

	mkdir -p "$test_root/user/project/child"
	child_log=$test_root/template-session-change-child-reentry.log
	if ! run_bare_tobari_pty_at ancestor-use "$test_root/user/project/child" >"$child_log" 2>&1; then
		cat "$child_log" >&2
		run_tobari_at "$test_root/user/project/child" status --format json >&2 || true
		run_tobari doctor --format json >&2 || true
		if run_tobari workspace delete --id "$workspace_ref" --confirm=delete --force --format json >&2; then
			workspace_ref=
			echo "workspace entry transition matrix: re-entry failed but public deletion succeeded" >&2
		else
			echo "workspace entry transition matrix: re-entry and public deletion both failed" >&2
		fi
		return 1
	fi
	grep -F 'Using existing Workspace' "$child_log" >/dev/null
	grep -F 'E2E_CWD=/var/lib/tobari/project/child' "$child_log" >/dev/null

	rewrite_template_source_access "$source_path" read-write read-only
	echo "workspace entry transition matrix: apply entry-slice-only change" >&2
	applied=$(apply_template_change "$template_ref")
	template_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template"]["template_ref"])' <<<"$applied")
	child_log=$test_root/template-entry-change-child-reentry.log
	run_bare_tobari_pty_at ancestor-use "$test_root/user/project/child" >"$child_log" 2>&1 || {
		cat "$child_log" >&2
		echo "workspace entry transition matrix: entry-slice change broke child re-entry" >&2
		return 1
	}
	grep -F 'E2E_CWD=/var/lib/tobari/project/child' "$child_log" >/dev/null

	IFS=$'\t' read -r original_runtime_id original_runtime_revision < <(read_template_runtime_source "$source_path")
	runtime_create=$(run_tobari runtime create --name transition-matrix --format json)
	runtime_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["runtime_ref"])' <<<"$runtime_create")
	echo "workspace entry transition matrix: build unselected managed Runtime" >&2
	runtime_build=$(run_tobari runtime build --id "$runtime_ref" --format json)
	runtime_id=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["id"])' <<<"$runtime_build")
	runtime_revision=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["runtime"]["runtime"]["revisions"][-1]["source_digest"])' <<<"$runtime_build")
	transition_runtime_image_id=$(docker image ls --quiet \
		--filter "label=io.tobari.owner=default" \
		--filter "label=io.tobari.component=runtime-revision" \
		--filter "label=io.tobari.runtime-id=$runtime_id")
	[[ $transition_runtime_image_id =~ ^sha256:[0-9a-f]{64}$ || $transition_runtime_image_id =~ ^[0-9a-f]{12,64}$ ]] || {
		echo "workspace entry transition matrix: managed Runtime image is not unique" >&2
		return 1
	}

	child_log=$test_root/runtime-catalog-change-child-reentry.log
	run_bare_tobari_pty_at ancestor-use "$test_root/user/project/child" >"$child_log" 2>&1 || {
		cat "$child_log" >&2
		echo "workspace entry transition matrix: unselected Runtime build broke child re-entry" >&2
		return 1
	}
	grep -F 'E2E_CWD=/var/lib/tobari/project/child' "$child_log" >/dev/null

	rewrite_template_runtime_source "$source_path" "$original_runtime_id" "$original_runtime_revision" "$runtime_id" "$runtime_revision"
	echo "workspace entry transition matrix: apply selected Runtime change" >&2
	applied=$(apply_template_change "$template_ref")
	template_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template"]["template_ref"])' <<<"$applied")
	child_log=$test_root/selected-runtime-change-child-reentry.log
	run_bare_tobari_pty_at ancestor-use "$test_root/user/project/child" >"$child_log" 2>&1 || {
		cat "$child_log" >&2
		echo "workspace entry transition matrix: selected Runtime change broke child re-entry" >&2
		return 1
	}
	grep -F 'E2E_CWD=/var/lib/tobari/project/child' "$child_log" >/dev/null

	rewrite_template_runtime_source "$source_path" "$runtime_id" "$runtime_revision" "$original_runtime_id" "$original_runtime_revision"
	clear_template_session_default "$source_path"
	rewrite_template_source_access "$source_path" read-only read-write
	echo "workspace entry transition matrix: apply combined Runtime/Template revert" >&2
	applied=$(apply_template_change "$template_ref")
	template_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template"]["template_ref"])' <<<"$applied")
	child_log=$test_root/combined-revert-child-reentry.log
	run_bare_tobari_pty_at ancestor-use "$test_root/user/project/child" >"$child_log" 2>&1 || {
		cat "$child_log" >&2
		echo "workspace entry transition matrix: combined Runtime/Template revert broke child re-entry" >&2
		return 1
	}
	grep -F 'Using existing Workspace' "$child_log" >/dev/null
	grep -F 'E2E_CWD=/var/lib/tobari/project/child' "$child_log" >/dev/null

	# Leave the prior AppliedEntry intentionally stale, then prove exact public
	# retirement is independent from entry currency and preserves Context Home.
	rewrite_template_source_access "$source_path" read-write read-only
	echo "workspace entry transition matrix: apply stale-delete prerequisite" >&2
	applied=$(apply_template_change "$template_ref")
	template_ref=$(python3 -c 'import json,sys; print(json.load(sys.stdin)["template"]["template_ref"])' <<<"$applied")
	context_id=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["contexts"]["items"]; assert len(items) == 1; print(items[0]["context_id"])' <<<"$(run_tobari context list --format json)")
	managed_home=$test_root/state/tobari/workspace-authority-runtime/contexts/$context_id/home
	settings_path=$managed_home/.claude/settings.json
	expected_settings=$test_root/transition-expected-settings.json
	cp "$settings_path" "$expected_settings"
	old_workspace_id=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["workspaces"]["items"]; assert len(items) == 1; print(items[0]["workspace_id"])' <<<"$(run_tobari workspace list --format json)")
	echo "workspace entry transition matrix: delete stale exact Workspace" >&2
	run_tobari workspace delete --id "$workspace_ref" --confirm=delete --force --format json >/dev/null
	workspace_ref=
	cmp "$expected_settings" "$settings_path"
	run_bare_tobari_pty_at reentry "$test_root/user/project" >"$test_root/post-stale-delete-recreate.log" 2>&1 || {
		cat "$test_root/post-stale-delete-recreate.log" >&2
		echo "workspace entry transition matrix: exact recovery after stale deletion failed" >&2
		return 1
	}
	new_workspace_list=$(run_tobari workspace list --format json)
	workspace_ref=$(python3 -c 'import json,sys; items=json.load(sys.stdin)["workspaces"]["items"]; assert len(items) == 1 and items[0]["workspace_id"] != sys.argv[1]; print(items[0]["workspace_ref"])' "$old_workspace_id" <<<"$new_workspace_list")
	cmp "$expected_settings" "$settings_path"
	echo "workspace entry transition matrix: Runtime catalog, session, entry slice, selected Runtime, combined revert, stale delete, and Context Home continuity OK"
}
