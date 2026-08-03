# Work Tasks: Audit and safely retire completed work packets

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read `AGENTS.md`, `docs/00_theses.md` through `docs/06_release.md`, `docs/README.md`, and all applicable work-packet templates. Evidence: the documents and `goal.md`, `context.md`, `plan.md`, `tasks.md`, `capability-retirement.md`, and `presentation-evidence.md` templates were read before and during the audit.
- [x] Enumerate every non-empty non-template packet and empty direct child directory under `docs/work`. Evidence: the final scan found `agent-integration-discovery`, `auth-broker-deferral`, `cli-catalog-audit`, `gateway-official-image`, `official-image-distribution`, `policy-review-tty`, `quickstart-runtime-docs`, `runtime-bash-shell`, `tobari-improvement-triage`, and this packet; it found zero empty non-template direct child directories.
- [x] Read every existing packet's goal, context, plan, and tasks files before finalization. Evidence: all packet sets were read; `cli-catalog-audit/e2e-transcript.md` was also read because its packet references it. Only the four user-authorized packet directories are updated in this finalization slice.
- [x] Record the retention, completion, E2E, durable-conclusion, successor, dirty-work, and deletion rules in `context.md`. Evidence: `context.md` defines the predicates, inventory columns, dirty-work preservation rule, and no-candidate disposition.

## Decide

- [x] Define a fail-closed mechanical classifier for complete, incomplete, superseded, and deferred states. Evidence: the classifier counts goal/task open checkboxes, reads status/retention/successor, requires the packet E2E marker for complete, and reads the deferred register separately.
- [x] Apply the classifier to the actual repository and include complete, incomplete, and deferred cases. Evidence: the final output is recorded in `context.md`: catalog/lifecycle and agent-integration evidence packets are complete, active packets are incomplete, the Quick Start packet is preserved as concurrent evidence, and the accepted auth packet remains explicitly deferred.
- [x] Decide deletion eligibility packet-by-packet and list every candidate or explicitly prove that the candidate set is empty. Evidence: the final candidate set is empty; evidence-retention packets, accepted/deferred auth evidence, active/incomplete packets, and the concurrent Quick Start packet are all preserved.
- [x] Confirm that the coordinator and all non-authorized packets are outside this packet's write scope. Evidence: `docs/work/tobari-improvement-triage`, auth-broker, and every other packet remain untouched; policy-review, runtime Bash, catalog, and lifecycle are the four explicitly authorized directories.

## Implement

- [x] Create only `docs/work/work-packet-retirement/{goal,context,plan,tasks}.md`. Evidence: the packet contains exactly these four files; no production file or other packet was changed.
- [x] Do not edit or delete `docs/work/tobari-improvement-triage/*` or any out-of-scope packet. Evidence: the coordinator and all non-authorized packet paths remain present and unchanged; only the four authorized directories are edited.
- [x] Preserve the deferred auth-broker branch/register disposition and do not create a false completion packet. Evidence: the coordinator register remains unchanged; `codex/auth-broker` exists and is not an ancestor of `HEAD`.

## Verify

- [x] Rerun the lifecycle classifier after writing this packet and record its output. Evidence: final classifier output is recorded in `context.md` with `complete`, `incomplete`, and `deferred` cases.
- [x] Run the detached-branch E2E boundary check for `codex/auth-broker` and record the result. Evidence: `git show-ref --verify refs/heads/codex/auth-broker` returned success and `git merge-base --is-ancestor codex/auth-broker HEAD` returned `1`.
- [x] Run the Markdown/reference check and confirm no broken packet or successor links. Evidence: current-main `task public:check` passes with `repoguard (public): OK` and `contractlint: OK`; `README.md` is excluded from the scoped staging set.
- [x] Run `task check` successfully. Evidence: current-main `task check` passes with hygiene, architecture, catalog, runtime, vet, race, tidy, and Go tests green.
- [x] Run `task public:check` successfully. Evidence: current-main `task public:check` passes with public repoguard and contractlint green.
- [x] Run `git diff --check` and confirm the final diff is limited to the four authorized packet directories. Evidence: final diff check passes; the unrelated `README.md` modification remains unstaged and preserved.
- [x] Stage exactly the 16 `goal/context/plan/tasks` files in the four authorized packet directories, verify the cached path set, and create one commit. Evidence: the final handoff reports the exact staged set and resulting commit SHA; no `README.md` path is staged.

## Hand off

- [x] Mark this packet `Complete` only after every acceptance and task condition has evidence. Evidence: lifecycle E2E, current-main gates, exact staging, and the final scoped commit are reported in the handoff.
- [x] Return the changed paths, the empty cleanup-candidate set, lifecycle classifications, and E2E result to the coordinator/maintainer. Evidence: `context.md` contains the four authorized paths, preserved packet holds, final classifier output, and E2E result; the handoff is reported in the response.
- [x] Keep this packet as `evidence` retention until its review/delete trigger is satisfied. Evidence: `goal.md` names the coordinator-acceptance and superseding-audit trigger; this audit packet remains as evidence in the commit.
