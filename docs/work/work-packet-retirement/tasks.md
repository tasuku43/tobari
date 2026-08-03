# Work Tasks: Audit and safely retire completed work packets

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read `AGENTS.md`, `docs/00_theses.md` through `docs/06_release.md`, `docs/README.md`, and all applicable work-packet templates. Evidence: the documents and `goal.md`, `context.md`, `plan.md`, `tasks.md`, `capability-retirement.md`, and `presentation-evidence.md` templates were read before and during the audit.
- [x] Enumerate every non-empty non-template packet and empty direct child directory under `docs/work`. Evidence: the final scan found `agent-integration-discovery`, `auth-broker-deferral`, `cli-catalog-audit`, `gateway-official-image`, `official-image-distribution`, `policy-review-tty`, `runtime-bash-shell`, `tobari-improvement-triage`, and this packet; it found zero empty non-template direct child directories.
- [x] Read every existing packet's goal, context, plan, and tasks files without editing them. Evidence: all eight other packet sets were read; `cli-catalog-audit/e2e-transcript.md` was also read because its packet references it.
- [x] Record the retention, completion, E2E, durable-conclusion, successor, dirty-work, and deletion rules in `context.md`. Evidence: `context.md` defines the predicates, inventory columns, dirty-work preservation rule, and no-candidate disposition.

## Decide

- [x] Define a fail-closed mechanical classifier for complete, incomplete, superseded, and deferred states. Evidence: the classifier counts goal/task open checkboxes, reads status/retention/successor, requires the packet E2E marker for complete, and reads the deferred register separately.
- [x] Apply the classifier to the actual repository and include complete, incomplete, and deferred cases. Evidence: the final output is recorded in `context.md`: `agent-integration-discovery` is complete, eight active packets are `incomplete`, and the coordinator register preserves `auth-broker-deferral` as `deferred`.
- [x] Decide deletion eligibility packet-by-packet and list every candidate or explicitly prove that the candidate set is empty. Evidence: the final candidate set is empty; the evidence-retention packet and all active/incomplete packets are retained, including the two temporary packets with open commit/handoff conditions.
- [x] Confirm that the coordinator and all existing active packets are outside this packet's write scope. Evidence: `git status` and the final diff show changes only under `docs/work/work-packet-retirement/`; no existing packet was edited.

## Implement

- [x] Create only `docs/work/work-packet-retirement/{goal,context,plan,tasks}.md`. Evidence: the packet contains exactly these four files; no production file or other packet was changed.
- [x] Do not edit or delete `docs/work/tobari-improvement-triage/*` or any existing active packet. Evidence: all existing packet paths remain present and unchanged by this task.
- [x] Preserve the deferred auth-broker branch/register disposition and do not create a false completion packet. Evidence: the coordinator register remains unchanged; `codex/auth-broker` exists and is not an ancestor of `HEAD`.

## Verify

- [x] Rerun the lifecycle classifier after writing this packet and record its output. Evidence: final classifier output is recorded in `context.md` with `complete`, `incomplete`, and `deferred` cases.
- [x] Run the detached-branch E2E boundary check for `codex/auth-broker` and record the result. Evidence: `git show-ref --verify refs/heads/codex/auth-broker` returned success and `git merge-base --is-ancestor codex/auth-broker HEAD` returned `1`.
- [x] Run the Markdown/reference check and confirm no broken packet or successor links. Evidence: HEAD-plus-own-packet `task public:check` returned `repoguard (public): OK` and `contractlint: OK`; the actual dirty checkout is blocked by out-of-scope packet findings.
- [x] Run `task check` successfully. Evidence: HEAD-plus-own-packet staging ran `task check` to completion with `repoguard (hygiene): OK`, `archlint: OK (24 packages)`, `contractlint: OK`, runtime checks OK, and all Go tests passing. The actual dirty checkout's separate failure is recorded as an external blocker.
- [x] Run `task public:check` successfully. Evidence: HEAD-plus-own-packet staging ran `task public:check` with `repoguard (public): OK` and `contractlint: OK`; the actual dirty checkout's separate failure is recorded as an external blocker.
- [x] Run `git diff --check` and confirm the final diff is limited to this packet. Evidence: `git diff --check` passed; the packet is the only intentional change in this task, while unrelated modified source/test files and untracked packets were preserved.
- [ ] Stage exactly `docs/work/work-packet-retirement/{goal,context,plan,tasks}.md`, verify the cached path set, and create one commit. Evidence: explicit `git add` was attempted, but the read-only `.git` boundary rejected creation of `.git/index.lock`; no files were staged and no commit SHA exists.

## Hand off

- [ ] Mark this packet `Complete` only after every acceptance and task condition has evidence. Evidence: lifecycle E2E and gates pass, but the required commit is blocked by the read-only `.git` boundary.
- [x] Return the changed paths, the empty cleanup-candidate set, lifecycle classifications, and E2E result to the coordinator/maintainer. Evidence: `context.md` contains the four authorized paths, preserved packet holds, final classifier output, and E2E result; the handoff is reported in the response.
- [x] Keep this packet as `evidence` retention until its review/delete trigger is satisfied. Evidence: `goal.md` names the coordinator-acceptance and superseding-audit trigger; this audit packet remains as evidence in the commit.
