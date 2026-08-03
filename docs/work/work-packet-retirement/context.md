# Work Context: Audit and safely retire completed work packets

This file records the repository-state audit and its evidence. It does not
change the status of another packet or turn a partial implementation into a
completion claim.

## Governing lifecycle rules read

- `AGENTS.md` requires a bounded packet with `goal.md`, `context.md`,
  `plan.md`, and `tasks.md` for non-trivial work. Temporary packets are removed
  only after acceptance/E2E evidence, durable-conclusion promotion, and a
  clean handoff; evidence retention is a narrow exception.
- `docs/00_theses.md` through `docs/04_harness.md`, plus `docs/05_public_repository.md`
  and `docs/06_release.md`, were read before the audit. The relevant invariant
  is that claims are executable and `task check` is the completion gate, with
  public/release gates added when the boundary requires them.
- There is no `docs/work/README.md` in this checkout. The repository's work
  packet documentation map is `docs/README.md`; its work-packet section points
  to `docs/work/_template/goal.md`. The goal, context, plan, tasks,
  `capability-retirement.md`, and `presentation-evidence.md` templates were
  read.
- `tools/repoguard` treats only `Complete` and `Superseded` as terminal
  historical statuses. A complete packet must have all acceptance and task
  checkboxes checked. A superseded packet must name one existing canonical
  successor path. This audit applies the stricter temporary-retention rule on
  top of that syntax check.

## Audit scope and method

At audit time, a packet is a non-template direct child of `docs/work` that
contains at least one regular file. `_template` is guidance, not a work
packet. Empty direct child directories are inventoried separately because an
empty directory is not completion evidence and is not tracked by Git.

The lifecycle procedure has these mechanical predicates:

1. Read `goal.md` metadata and count unchecked acceptance criteria.
2. Count unchecked task checkboxes in `tasks.md`.
3. Require explicit E2E evidence of the packet's actual user, maintainer, or
   repository-boundary journey; a unit/build-only result is insufficient.
4. Require a durable conclusion or an explicit successor/deferred condition.
5. Classify a packet as `complete` only when its status/checklists/E2E,
   conclusion, references, and clean-handoff conditions all hold. Classify an
   active packet with any open condition as `incomplete`. Classify a deferred
   registered item from its explicit state and resumption condition even when
   it has no standalone packet.
6. Mark a temporary packet as a deletion candidate only when acceptance and
   E2E are proven, conclusions are promoted, no tasks/references remain, no
   dirty user work is entangled, and the exact target is listed. Otherwise
   preserve it and report why.

The command used for the first repository-state classification was:

```sh
for d in $(find docs/work -mindepth 1 -maxdepth 1 -type d ! -name _template -print | sort); do
  files=$(find "$d" -type f -not -path '*/_template/*' | wc -l | tr -d ' ')
  packet_state=$(awk -F': ' '/^- Status:/ {print $2; exit}' "$d/goal.md")
  retention=$(awk -F': ' '/^- Retention:/ {print $2; exit}' "$d/goal.md")
  successor=$(awk -F': ' '/^- Successor:/ {print $2; exit}' "$d/goal.md")
  goal_open=$(awk 'BEGIN{in_accept=0; n=0} /^## Acceptance criteria/{in_accept=1; next} /^## /{if(in_accept) exit} in_accept && /^- \[ \]/{n++} END{print n}' "$d/goal.md")
  [ -f "$d/goal.md" ] || continue
  task_open=$(rg -c '^[-*+] \[ \]' "$d/tasks.md" 2>/dev/null || true)
  task_open=${task_open:-0}
  if rg -i -q 'e2e|end-to-end|integration: OK|integration.*pass|governance-boundary.*pass' "$d" --glob '*.md'; then e2e_marker=1; else e2e_marker=0; fi
  case "$packet_state" in
    Complete*) terminal=1 ;;
    Superseded*) terminal=2 ;;
    Active*|Draft*|Accepted*) terminal=0 ;;
    *) terminal=3 ;;
  esac
  if [ "$terminal" = 1 ] && [ "$goal_open" = 0 ] && [ "$task_open" = 0 ] && [ "$e2e_marker" = 1 ]; then classification=complete
  elif [ "$terminal" = 2 ]; then classification=superseded
  elif [ "$terminal" = 0 ]; then classification=incomplete
  else classification=review
  fi
  printf '%s files=%s status=%s retention=%s goal_open=%s task_open=%s e2e_marker=%s successor=%s => %s\n' "$d" "$files" "$packet_state" "$retention" "$goal_open" "$task_open" "$e2e_marker" "$successor" "$classification"
done
printf 'deferred-register: '
rg -n 'auth-broker-deferral.*Deferred' docs/work/tobari-improvement-triage/context.md | head -1 || true
printf 'empty-directories: '
find docs/work -mindepth 1 -maxdepth 1 -type d -empty ! -name _template -print | wc -l | tr -d ' '
```

The final scan covers nine non-empty packets and reports no empty direct child
directory. The shared worktree changed while the audit was running: the
standalone `auth-broker-deferral` packet is present but still `Active` because
its commit/handoff task is unchecked, and `cli-catalog-audit` is still `Active`
with its child-packet commit task unchecked. Neither is treated as complete or
eligible for cleanup. The deferred product state is checked separately from
the standalone packet through the coordinator register and the detached-branch
ancestry check.

## Inventory and disposition

The table is the audit's required evidence surface. “Tasks” is the count of
unchecked task checkboxes at the recorded scan; “check” lists recorded gates,
not a claim that every gate is sufficient for packet completion.

| Packet or registered item | Goal/status/retention | Tasks | Check evidence | E2E evidence | Durable conclusion | Successor link | Lifecycle disposition |
|---|---|---|---|---|---|---|---|
| `docs/work/cli-catalog-audit/` | Public CLI catalog audit; `Active (content complete; commit blocked by repository state)`; `temporary` | 0 open goal criteria and 1 open task item | `task check` and `task public:check` are recorded as passed in its packet | Catalog E2E covers the clean binary, 25/25 help scopes, representative argv/recovery paths, and legacy negative paths; the positive shared-cluster flow is held after `cluster_start_failed` | ADRs 0012/0015 are listed; retirement/read-effect conclusions remain packet-local and are not promoted | `None` | `incomplete`; preserve unchanged because the status and child-packet commit/handoff remain open |
| `docs/work/agent-integration-discovery/` | Agent skill/plugin/MCP surface decision; `Complete`; `evidence` | 0 open goal/task items | `task check`, public, and integration evidence recorded | Real Colima integration reached `integration: OK`, including policy recovery, runtime build, retry, canaries, and cleanup | No durable contract change; existing theses/contracts govern; evidence is intentionally retained until a follow-up skill proof | `None` | `complete` but not a temporary cleanup candidate; retain evidence until its review trigger |
| `docs/work/auth-broker-deferral/` | Detached auth-broker governance boundary; `Active`; `temporary` | 1 open goal criterion and 3 open task items | Exact `main` archive `task check`, public, and build evidence recorded; security baseline finding is disclosed | Governance E2E passes: refs/merge-base, branch-only `cat-file`, main Catalog/help negative path, and clean main snapshot | No durable document changed because current contracts already govern; future restart gate and new reviewed packet are explicit | `None; any restart must create a new reviewed packet` | `incomplete`; preserve unchanged because the packet commit/handoff is still open |
| `docs/work/gateway-official-image/` | Trusted Gateway image distribution; `Active`; `temporary` | 4 open: package visibility and handoff remain unchecked | `task check`, security, public, and release evidence recorded; Gateway/OPA focused gates recorded | Official-digest and embedded-source runtime integration reported `OK` on 2026-08-03; publication visibility is still unverified | ADR 0017 is listed; packet handoff says durable promotion is still pending | `None` | `incomplete`; preserve unchanged because public visibility and handoff remain open |
| `docs/work/official-image-distribution/` | Official runtime/agent image family; `Active`; `temporary` | 7 open goal criteria and 18 open task items at audit baseline | `task check`, public, release, and local image gates recorded; security is explicitly blocked by four pre-existing gosec findings | Local base/Claude/Codex image builds and contract checks pass; published digest, attestation/SBOM, license, and consumer verification remain open | ADR 0012 is listed, but the packet explicitly requires release/publication conclusions to be promoted | `None` | `incomplete`; preserve unchanged because publication, rights, support, and handoff remain open |
| `docs/work/policy-review-tty/` | Visible interactive policy review; `Active`; `temporary` | 8 open goal criteria and 4 open task items at final rescan | Focused CLI/application, Gateway, and OPA checks pass; full review integration stops at `cluster_start_failed` before policy review | Durable fake-runtime PTY E2E now exercises allow/deny/cancel/read-only paths; supported runtime review journey remains unproven | No ADR; packet still has open public interaction and readiness decisions | `None` | `incomplete`; preserve unchanged because supported runtime E2E and handoff remain open |
| `docs/work/runtime-bash-shell/` | Base-runtime Bash entry contract; `Active`; `temporary` | 6 open goal criteria and 19 open task items | No final gate evidence; runtime and Docker verification remain open | Source observations exist, but base-image and interactive reusable-Workspace E2E are not proven | Thesis/runtime contract is cited; no durable conclusion is promoted | `None` | `incomplete`; preserve unchanged because runtime E2E, gates, and commit remain open |
| `docs/work/tobari-improvement-triage/` | Backlog coordinator; `Active`; `temporary` | 8 open goal criteria and 21 open task items at final scan | Coordinator task/public evidence is open; child packets are not all handed off | Its child-routing E2E is incomplete while policy, catalog, docs, runtime, and lifecycle work remain open | No ADR; it is explicitly a temporary coordinator and requires child handoffs | `None` | `incomplete`; preserve unchanged because it is the user-owned coordinator |
| `auth-broker-deferral` (registered product item) | Explicitly `Deferred`, detached from `main`; the standalone governance packet remains `Active` | Not applicable to the register row | Coordinator context records the branch and resumption condition | Machine register check plus `git merge-base --is-ancestor codex/auth-broker HEAD` negative result; this is a repository-boundary E2E, not a feature-completion claim | Deferral condition is recorded in the coordinator; no auth capability is promoted into `main` | No standalone successor; resume only after the stated product/auth-boundary condition | `deferred`; preserve branch and do not create a false completed product capability |
| [`work-packet-retirement`](goal.md) | Lifecycle audit evidence; `Active`; `evidence` | 1 open goal criterion and 2 open commit/handoff tasks | HEAD-plus-own-packet `task check` and `task public:check` pass; actual dirty checkout is blocked by out-of-scope packet findings | Final classifier runs against the current packet tree and produces complete/incomplete/deferred results; branch-boundary E2E is negative as required | Lifecycle classification is evidence, not a product/architecture decision; no ADR required | `None`; later deletion is governed by this packet's trigger | `incomplete`; preserve as evidence because the required commit is blocked |

## Empty directories

The final direct-child scan found no empty directory under `docs/work`. The
non-empty `_template` directory is intentionally excluded from packet
classification. No empty directory was treated as a packet or as completion
evidence.

## Deletion candidates

No packet met every temporary-retention deletion predicate in the final scan.
`agent-integration-discovery` and this packet are `Retention: evidence`.
`auth-broker-deferral` and `cli-catalog-audit` have E2E evidence but remain
active with an unchecked commit/handoff condition. `runtime-bash-shell` has
not completed its runtime E2E, and all other temporary
packets have open acceptance, integration, publication, or handoff work.
Therefore no packet was deleted. The only paths authorized for the final
commit are this packet's four files; all other packet and dirty-work paths are
preserved and reported.

## Commit gate

The explicit final staging attempt used only these paths:

```text
docs/work/work-packet-retirement/goal.md
docs/work/work-packet-retirement/context.md
docs/work/work-packet-retirement/plan.md
docs/work/work-packet-retirement/tasks.md
```

It failed before staging because the environment could not create
`.git/index.lock` (`Operation not permitted`). The cached path set is empty and
there is no commit SHA. No alternate path, force flag, or unrelated file was
used; this packet remains `Active` as required by the completion rule.

## Security and public-boundary notes

- The audit reads packet text and Git metadata only; it performs no external
  mutation, network call, credential access, branch change, or packet deletion.
- All committed examples use repository-relative paths and synthetic/public
  identifiers. No shell history, private URL, credential, or local absolute
  path is copied into the packet.
- Reference validation uses the repository's public guard and its regular-file
  and Markdown-link checks. A broken successor or packet link fails the gate.
- A task gate failure is evidence against completion, not a reason to weaken or
  bypass the gate.

## Glossary

- **Complete:** a packet whose own acceptance, tasks, E2E, durable conclusion,
  references, and handoff conditions are all proven.
- **Incomplete:** an active packet with any open acceptance, task, gate, E2E,
  conclusion, or handoff condition.
- **Deferred:** a deliberately paused registered item with an explicit
  resumption condition; it is not a completed implementation.
- **Evidence retention:** a narrow packet lifetime used when the audit record
  itself is useful and cannot be replaced by a durable product contract.

## Final E2E and gate evidence

Audit E2E verdict: PASS

The lifecycle procedure was rerun against the final packet tree after the
concurrent packet changes were observed. The final classifier output is
recorded below; the deferred product state remains in the coordinator register
and the standalone auth-broker packet is classified independently as
incomplete.

```text
docs/work/agent-integration-discovery files=4 status=Complete retention=evidence goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
docs/work/auth-broker-deferral files=4 status=Active retention=temporary goal_open=1 task_open=3 e2e_marker=1 successor=None; any restart must create a new reviewed packet. => incomplete
docs/work/cli-catalog-audit files=5 status=Active (content complete; commit blocked by repository state) retention=temporary goal_open=0 task_open=1 e2e_marker=1 successor=None => incomplete
docs/work/gateway-official-image files=4 status=Active retention=temporary goal_open=0 task_open=4 e2e_marker=0 successor=None => incomplete
docs/work/official-image-distribution files=4 status=Active retention=temporary goal_open=7 task_open=18 e2e_marker=0 successor=None => incomplete
docs/work/policy-review-tty files=5 status=Active retention=temporary goal_open=8 task_open=4 e2e_marker=1 successor=None => incomplete
docs/work/runtime-bash-shell files=4 status=Active retention=temporary goal_open=6 task_open=19 e2e_marker=1 successor=None => incomplete
docs/work/tobari-improvement-triage files=4 status=Active retention=temporary goal_open=8 task_open=21 e2e_marker=1 successor=None => incomplete
docs/work/work-packet-retirement files=4 status=Active retention=evidence goal_open=1 task_open=2 e2e_marker=1 successor=None => incomplete
deferred-register: 31:| `auth-broker-deferral` | Deferred, detached from `main` | 2 only when resumed | Security/product owner | Tobari's core value proposition and an explicit auth boundary | Keep the branch isolated; record the resumption condition; do not revive provider-specific auth as a convenience shortcut |
empty-directories: 0
```

The detached-branch E2E for the deferred item additionally returned a
successful branch ref lookup
and `git merge-base --is-ancestor codex/auth-broker HEAD` exit `1`, proving the
experiment is not an ancestor of the supported checkout. The reference/public
check passed in an isolated HEAD-plus-own-packet staging (`repoguard (public):
OK`, `contractlint: OK`), and the full gate passed the same staging with
hygiene, architecture, contract, runtime, vet, race, tidy, and Go tests green.
The actual dirty-checkout `task check` and `task public:check` both fail before
the repository checks because the out-of-scope `auth-broker-deferral` packet
contains a machine-specific home path and `cli-catalog-audit` uses a non-schema
status suffix. Those packets were not edited. The unrelated generated
status suffix. Those packets were not edited. The unrelated generated
artifacts, modified source/test files, and packet files remain untouched. The required Git stage/commit
attempt is separately recorded above and remains the only open completion
condition for this packet.
