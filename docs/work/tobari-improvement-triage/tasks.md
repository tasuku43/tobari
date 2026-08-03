# Work Tasks: Triage Tobari product and maintenance backlog

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, harness, public-repository, release, and agent-readiness sections. Evidence: reviewed `docs/00` through `docs/06` and `docs/09` before creating this packet.
- [x] Observe the reported policy-review behavior and compare it with the JSON/read-only and TTY contracts. Evidence: user-provided transcript plus `internal/cli/tobari.go`, `internal/cli/policy_review_selector.go`, CLI tests, and `scripts/test-integration.sh` inspection.
- [x] Record verified facts, unknowns, current dirty-worktree boundaries, and the issue register in `context.md`.
- [x] Record the auth-broker branch disposition without changing the branch. Evidence: local `codex/auth-broker` exists and is not an ancestor of `main`.
- [x] Confirm the coordinator outcome and non-goals in `goal.md`.

## Decide

- [x] Compare one coordinator packet plus bounded child packets with one large implementation packet.
- [x] Classify the current topics as reported, deferred, discovery, planned, or continuous maintenance.
- [x] Confirm the proposed execution order and owner roles with the maintainer before opening child implementation packets. Evidence: maintainer approved proceeding with the policy-review reproduction.
- [x] Create the policy-review TTY child packet with a deterministic PTY reproduction and exact acceptance tests. Evidence: [policy-review-tty](../policy-review-tty/goal.md) records the child goal, context, plan, and tasks.
- [x] Create the agent-integration discovery packet, including skill-first versus plugin/MCP alternatives and the runtime-image workflow boundary; require a replayable workflow E2E. Evidence: [agent-integration-discovery](../agent-integration-discovery/goal.md); scoped commit `8e1adfa`.
- [x] Record the auth-broker deferral as a coordinator register decision with a machine-checkable main/branch boundary E2E. Evidence: `context.md` records the deferred row, `codex/auth-broker` is not an ancestor of `codex/first-wave`, and the local auth-broker packet remains deliberately outside current-wave commits and integration.
- [ ] Create the README/architecture/publication packet after the policy and catalog inputs are stable; require clean-checkout docs and journey replay E2E.
- [x] Create the CLI catalog audit packet and copy `capability-retirement.md` if removal candidates are found; require command-surface E2E for every retirement candidate. Evidence: [cli-catalog-audit](../cli-catalog-audit/goal.md); scoped commit `a007059`.
- [x] Create the runtime Bash shell packet and assign its image/TTY E2E. Evidence: [runtime-bash-shell](../runtime-bash-shell/goal.md); scoped commit `912c602`.
- [x] Define the packet-audit evidence table and the disposition of every non-empty active/historical packet; require packet lifecycle E2E before cleanup. Evidence: [work-packet-retirement](../work-packet-retirement/goal.md); scoped commit `92d742c`.
- [ ] Record any durable decision in an ADR or governing document before closing a child packet.

## Implement

- [ ] Do not implement child issue code in this coordinator packet; use successor packets for implementation.
- [x] Remove only confirmed stale empty work-packet directories after recording their disposition. Evidence: removed the six empty paths associated with `agent-auth-broker`, `auth-profile-catalog`, `context-bundle`, `home-relative-workspace`, `local-tool-auth`, and `retire-devcontainer-support`; no non-empty packet or unrelated integration artifact was removed.
- [x] Preserve all unrelated user changes and the detached auth-broker branch. Evidence: after the coordinator commit, the current `codex/first-wave` status contains only the untracked auth-broker packet; `codex/auth-broker` is not merged.

## Verify

- [x] Focused coordinator documentation checks pass. Evidence: clean detached worktree at `a2015c2` passed `task check`, `task security`, and `task public:check`; the first run's only failure was the now-removed link to the intentionally uncommitted auth packet.
- [x] `task check` passes. Evidence: clean-checkout `task check` passed at `a2015c2` after the coordinator link correction.
- [x] `task public:check` passes when this public work packet is checked. Evidence: clean-checkout `task public:check` returned `repoguard (public): OK` and `contractlint: OK` at `a2015c2`.
- [ ] Each child packet records its required security, release, runtime, and integration profiles. Evidence:
- [ ] The relevant agent-readiness scenario and discovery-round-trip budget are recorded for each child capability. Evidence:
- [ ] Every child packet records a successful end-to-end user-journey replay, or an explicit unresolved environment blocker; no analysis-only closure is accepted. Evidence:
- [x] Every completed current-wave child reports an intentional scoped commit after rerunning its E2E and required gates. Evidence: `7d096bb`, `8e1adfa`, `912c602`, `a007059`, and `92d742c`; unresolved environment blockers remain explicitly recorded in the affected child packets.
- [x] The parked auth-broker packet remains detached, unmerged, and unscheduled until explicit maintainer resumption. Evidence: `codex/auth-broker` is not an ancestor of `codex/first-wave`, and the packet remains unstaged.
- [ ] The routine-success external-processing count remains zero for supported outcomes, or a deliberately raw utility documents its narrower promise. Evidence:
- [x] Generated diff and repository status are understood without absorbing unrelated changes. Evidence: the first-wave branch contains only scoped child commits plus this coordinator commit; the remaining uncommitted path is limited to `docs/work/auth-broker-deferral/`.

## Hand off

- [ ] All seven issue-register entries have a successor, explicit deferral, or completed evidence.
- [ ] Acceptance criteria have evidence.
- [ ] Goal status is changed to `Complete` only after all child handoffs and packet cleanup are complete.
- [ ] Durable decisions were promoted out of the coordinator packet.
- [ ] Temporary diagnostics and sensitive artifacts were removed.
- [ ] The temporary coordinator packet is removed after the register has been handed off and the cleanup audit is complete.
- [ ] Handoff summary explains the policy-review outcome, deferred auth-broker condition, plugin/runtime decision, documentation/CLI follow-ups, packet dispositions, checks, and risks.
