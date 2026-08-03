# Work Context: Audit and safely retire completed work packets

This file records the repository-state audit and its evidence. The first scan
is retained as historical evidence; the current refresh below supersedes its
packet-state table after the integration and PTY blockers were resolved.

## Main-history reconciliation

- The audit baseline is the clean `main` line at `2810f48`; this refresh
  records the packet tree at that baseline before its own scoped evidence
  commit.
- The first-wave merge is `966dd08841a7ccd88212dd9c8683562c99e17aa9`.
- The four child commits audited here are present in that history:
  policy-review `7d096bb5749e3ad8afd6d85c88af301f5dda113f`, runtime Bash
  `912c602b4e80d055775e557e6e509b6beff26928`, catalog audit
  `a007059863ec3c5f80441c9ac86bbc643ca1d5ac`, and this lifecycle audit
  `92d742c3397e7aea8a24ccc23fbfef41e33d7134`.
- The deferred auth packet is committed separately in `93e5cacfdff3a34c4392cd24b8e17bcd92daefe0`, with its handoff correction in `ed37f805a4e2876f93c6ad86fb70beb40b6fc073`; the auth implementation remains on `codex/auth-broker` and is not merged.
- The packet-finalization refresh is `831c7e73eb2b40fdaae2191096462df46a9b8539`; the second-wave architecture and Quick Start commits are `001c4a7741b013174257cca3514d836ccb86649f` and `d98e086e1ca305bc82e08d7a9f7b2892aff076d2`.
- The runtime preflight, public first-use handoff, PTY evidence boundary, and
  packet-state reconciliation are `6094f08`, `a4e6334`, `b8e0d50`, and
  `2810f48` respectively.
- Current `main` has no rebase, alternate-index, or read-only Git metadata
  blocker. Remaining packet holds are E2E/product/lifecycle decisions, not
  staging failures.

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

The historical scan covered fifteen non-empty packets and reported no empty
direct child directory. The standalone `auth-broker-deferral` packet is now
`Accepted` and explicitly deferred, with all of its governance evidence
committed on `main`; it remains preserved under its own temporary-retention
trigger and is not a product-completion claim. The catalog audit, architecture
publication, Quick Start/runtime documentation, runtime lifecycle, and this
lifecycle audit are complete or evidence-backed. The new-user journey, policy
review, runtime Bash, PTY evidence, the two image publication packets, and the
coordinator remain active because raw/blind replay, integration, publication,
or handoff conditions are unresolved. The deferred product state is checked
separately from the standalone packet through the coordinator register and
detached-branch ancestry check.

## Inventory and disposition

The table is the audit's required evidence surface. “Tasks” is the count of
unchecked task checkboxes at the recorded scan; “check” lists recorded gates,
not a claim that every gate is sufficient for packet completion.

| Packet or registered item | Goal/status/retention | Tasks | Check evidence | E2E evidence | Durable conclusion | Successor link | Lifecycle disposition |
|---|---|---|---|---|---|---|---|
| `docs/work/cli-catalog-audit/` | Public CLI catalog audit; `Complete`; `evidence` | 0 open goal/task items | Current-main `task check` and `task public:check` pass; scoped commit `a007059` is merged by `966dd08` | Catalog E2E covers the clean binary, 25/25 help scopes, representative argv/recovery paths, and legacy negative paths; the positive shared-cluster flow remains held after its integration blocker | No public contract change; retirement/read-effect findings are explicit follow-ups | `None` | `complete`; retain evidence until the coordinator promotes the follow-ups |
| `docs/work/agent-integration-discovery/` | Agent skill/plugin/MCP surface decision; `Complete`; `evidence` | 0 open goal/task items | `task check`, public, and integration evidence recorded | Real Colima integration reached `integration: OK`, including policy recovery, runtime build, retry, canaries, and cleanup | No durable contract change; existing theses/contracts govern; evidence is intentionally retained until a follow-up skill proof | `None` | `complete` but not a temporary cleanup candidate; retain evidence until its review trigger |
| `docs/work/architecture-publication/` | Architecture presentation; `Complete`; `evidence` | 0 open goal/task items | Current-main `task check`, `task security`, and `task public:check` pass; `task release:check` stops at the existing ShellCheck findings in `scripts/test-integration.sh:238`; scoped commit `001c4a7` is on `main` | Local HTML/CSS/link E2E, 16 links, required terms, no external assets, and loopback HTTP 200/byte match passed; Pages activation was not performed | No durable contract change; static publication workflow and local proof are retained until replacement or publication review | `None` | `complete`; retain evidence until its review trigger and keep owner-side Pages activation explicit |
| `docs/work/auth-broker-deferral/` | Detached auth-broker governance boundary; `Accepted`/explicitly deferred; `temporary` | 0 open goal/task items | Clean main archive `task check`, public, security, and build evidence recorded; docs commits `93e5cac` and `ed37f80` are on current `main` | Governance E2E passes: refs/merge-base, branch-only `cat-file`, main Catalog/help negative path, and clean main snapshot | No durable document changed because current contracts already govern; future restart gate and new reviewed packet are explicit | `None; any restart must create a new reviewed packet` | `deferred`; preserve branch and packet under its review/delete trigger |
| `docs/work/gateway-official-image/` | Trusted Gateway image distribution; `Active`; `temporary` | 4 open: package visibility and handoff remain unchecked | `task check`, security, public, and release evidence recorded; Gateway/OPA focused gates recorded | Official-digest and embedded-source runtime integration reported `OK` on 2026-08-03; publication visibility is still unverified | ADR 0017 is listed; packet handoff says durable promotion is still pending | `None` | `incomplete`; preserve unchanged because public visibility and handoff remain open |
| `docs/work/official-image-distribution/` | Official runtime/agent image family; `Active`; `temporary` | 7 open goal criteria and 18 open task items at audit baseline | `task check`, public, release, and local image gates recorded; security is explicitly blocked by four pre-existing gosec findings | Local base/Claude/Codex image builds and contract checks pass; published digest, attestation/SBOM, license, and consumer verification remain open | ADR 0012 is listed, but the packet explicitly requires release/publication conclusions to be promoted | `None` | `incomplete`; preserve unchanged because publication, rights, support, and handoff remain open |
| `docs/work/policy-review-tty/` | Visible interactive policy review; `Active`; `temporary` | 2 open goal criteria and 5 open task items | Focused CLI/application, Gateway, OPA, and clean repository profiles pass; full review integration remains blocked before policy review | Durable fake-runtime PTY E2E exercises allow/deny/cancel/read-only paths; supported runtime review journey remains unproven | No ADR; supported-runtime/readiness handoff remains open | `None` | `incomplete`; preserve because the runtime blocker remains unresolved |
| `docs/work/runtime-bash-shell/` | Base-runtime Bash entry contract; `Active`; `temporary` | Broader integration/acceptance hold remains | Current-main runtime, implementation, security, and public gates pass; scoped commit `912c602` is merged | Canonical/embedded Bash and dedicated interactive Workspace E2E pass; broader integration remains at policy review and is unresolved | Existing runtime contract is confirmed; broader integration handoff remains open | `None` | `incomplete`; preserve because the runtime integration blocker remains unresolved |
| `docs/work/new-user-value-e2e/` | Four new-user value journeys; `Active`; `evidence` | 7 open goal criteria and 4 open task items | Current-main `task check` and `task public:check` pass; raw-PTY helper contract is now available but official reports predate it | Four official real-PTY journeys have redacted functional feedback; raw digests/checkpoints remain an explicit rerun condition | Findings are routed to policy, runtime, Quick Start, CLI audit, and PTY evidence packets | `comparison.md` and the listed successor packets | `incomplete`; preserve because the evidence packet still owns the blind/raw gap |
| `docs/work/new-user-quickstart-handoff/` | First-use host/Workspace handoff; `Complete`; `temporary` | 0 open goal/task items | Current-main `task check`, security, and public gates pass; scoped commit `a4e6334` is on `main` | Fresh 120x40 PTY reaches the documented value signal; aggregate runtime replay stops at the external policy-review handoff | README/help now names executable recovery, runtime preflight, host ownership, and cleanup | `None` | `complete` but retain until its temporary review trigger and shared PTY blocker are resolved |
| `docs/work/runtime-lifecycle-reconcile/` | Runtime readiness/reuse decision; `Complete`; `temporary` | 0 open goal/task items | Focused Go/runtime, `task check`, security, and public gates pass; scoped commit `6094f08` is on `main` | Fresh 120x40 PTY proves missing-image preflight, custom-image entry/re-entry, Bash identity, and cleanup; `task runtime:test` stops at the external policy-review handoff | New Workspace registration is preflighted; existing Workspace images are not silently refreshed | `None` | `complete` but retain until its temporary review trigger and specialized runtime blocker are resolved |
| `docs/work/pty-evidence-harness/` | Parent-owned raw PTY evidence boundary; `Active`; `temporary` | 2 open goal/task items | `task check`, security, and public gates pass; scoped commit `b8e0d50` is on `main` | Real-PTY smoke contract proves metadata, delayed input, ANSI-preserving redaction, external raw output, digest, and checkpoints; blind Tobari artifact rerun is pending | Harness/readiness docs define the external bundle and child-blind boundary | `new-user-value-e2e` feedback intake | `incomplete`; preserve until one blind product run consumes the artifact |
| `docs/work/quickstart-runtime-docs/` | Second-wave Quick Start/runtime documentation; `Complete`; `evidence` | 0 open goal/task items | Current-main `task check`, `task security`, and `task public:check` pass; scoped commit `d98e086` is on `main` | Focused help/policy/Gateway/runtime E2E and the bounded Context build/entry replay passed; aggregate integration/runtime stopped at the existing PTY review helper with exit 130 | No durable contract change; the runnable host/agent journey and explicit runtime-build boundary are retained as evidence | `None` | `complete`; retain evidence until a repeatable public replay replaces it |
| `docs/work/tobari-improvement-triage/` | Backlog coordinator; `Active`; `temporary` | 0 open goal criteria and 12 open task items | Coordinator register now records runtime/Quick Start completion, PTY evidence activity, auth deferral, and current commits; repository gates pass | Its child-routing E2E remains open while policy, image publication, release, external Pages, and blind PTY follow-ups remain | No ADR; it remains the user-owned coordinator and requires the remaining child handoffs | `None` | `incomplete`; preserve because it is the user-owned coordinator |
| `auth-broker-deferral` (registered product item) | Explicitly `Deferred`, detached from `main`; the standalone governance packet remains `Active` | Not applicable to the register row | Coordinator context records the branch and resumption condition | Machine register check plus `git merge-base --is-ancestor codex/auth-broker HEAD` negative result; this is a repository-boundary E2E, not a feature-completion claim | Deferral condition is recorded in the coordinator; no auth capability is promoted into `main` | No standalone successor; resume only after the stated product/auth-boundary condition | `deferred`; preserve branch and do not create a false completed product capability |
| [`work-packet-retirement`](goal.md) | Lifecycle audit evidence; `Complete`; `evidence` | 0 open goal/task items | Current-main `task check` and `task public:check` pass; scoped audit commit `92d742c` is merged | Final classifier runs against the current packet tree and produces complete/incomplete/deferred results; branch-boundary E2E is negative as required | Lifecycle classification is evidence, not a product/architecture decision; no ADR required | `None`; later deletion is governed by this packet's trigger | `complete`; retain as evidence until the review/delete trigger is satisfied |

## Empty directories

The final direct-child scan found no empty directory under `docs/work`. The
non-empty `_template` directory is intentionally excluded from packet
classification. No empty directory was treated as a packet or as completion
evidence.

## Deletion candidates

No packet met every temporary-retention deletion predicate in the current
scan. `agent-integration-discovery`, `cli-catalog-audit`,
`architecture-publication`, `quickstart-runtime-docs`, and this packet are
`Retention: evidence`; the accepted/deferred auth packet remains under its
temporary trigger. The complete runtime/Quick Start packets remain temporary
because their review triggers still depend on the shared PTY blocker and
coordinator handoff. New-user, policy, runtime Bash, PTY evidence, image
publication, and coordinator packets retain open conditions. Therefore no
packet was deleted.

## Commit gate

This refresh stages only the lifecycle audit metadata and evidence files:

```text
docs/work/work-packet-retirement/goal.md
docs/work/work-packet-retirement/context.md
docs/work/work-packet-retirement/tasks.md
```

The lifecycle handoff verifies this exact three-path set after staging and
creates one scoped commit. The coordinator edits are intentionally left for a
separate subsequent commit.

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

## Historical E2E and gate evidence

Audit E2E verdict: PASS

The lifecycle procedure was rerun against the current packet tree after the
second-wave commits. The deferred product state remains in the standalone auth
packet's explicit execution state, and no packet is deleted merely because a
focused E2E passed.

```text
docs/work/agent-integration-discovery files=4 status=Complete retention=evidence goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
docs/work/architecture-publication files=5 status=Complete retention=evidence goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
docs/work/auth-broker-deferral files=4 status=Accepted retention=temporary goal_open=0 task_open=0 e2e_marker=1 successor=None; any restart must create a new reviewed packet. => incomplete
docs/work/cli-catalog-audit files=5 status=Complete retention=evidence goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
docs/work/gateway-official-image files=4 status=Active retention=temporary goal_open=0 task_open=4 e2e_marker=0 successor=None => incomplete
docs/work/new-user-quickstart-handoff files=4 status=Complete retention=temporary goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
docs/work/new-user-value-e2e files=24 status=Active retention=evidence goal_open=7 task_open=4 e2e_marker=1 successor=See [comparison.md](comparison.md); successor packets are candidates only and are not opened by this evidence slice. => incomplete
docs/work/official-image-distribution files=4 status=Active retention=temporary goal_open=7 task_open=18 e2e_marker=0 successor=None => incomplete
docs/work/policy-review-tty files=5 status=Active retention=temporary goal_open=2 task_open=5 e2e_marker=1 successor=None => incomplete
docs/work/pty-evidence-harness files=4 status=Active retention=temporary goal_open=2 task_open=2 e2e_marker=1 successor=None => incomplete
docs/work/quickstart-runtime-docs files=5 status=Complete retention=evidence goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
docs/work/runtime-bash-shell files=5 status=Active retention=temporary goal_open=0 task_open=3 e2e_marker=1 successor=None => incomplete
docs/work/runtime-lifecycle-reconcile files=5 status=Complete retention=temporary goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
docs/work/tobari-improvement-triage files=4 status=Active retention=temporary goal_open=0 task_open=12 e2e_marker=1 successor=None => incomplete
docs/work/work-packet-retirement files=4 status=Complete retention=evidence goal_open=0 task_open=0 e2e_marker=1 successor=None => complete
deferred-register: standalone auth packet is `Accepted` with explicit deferred execution state; `codex/auth-broker` remains outside `main`
empty-directories: 0
```

The detached-branch E2E additionally returned a successful branch ref lookup
and `git merge-base --is-ancestor codex/auth-broker main` exit `1`, proving the
experiment is not an ancestor of the supported checkout. On the historical
audit baseline, the release profile stopped at the pre-existing ShellCheck
findings and the integration aggregate stopped at the interactive policy-review
helper; those blockers are resolved in the current main line and are retained
above only as historical evidence.

## Current refresh: 2026-08-04

The current main line includes the bounded integration-input fix (`c957401`),
the completed policy/runtime packet handoff (`ea95421`), the PTY single-event
contract assertion (`7f1b9ac`), the PTY/new-user evidence closure (`5568e61`),
and the accepted runtime-image boundary (`6a5846`). The current focused and
repository gates are green: `task check`, `task security`,
`task public:check`, `task release:check`, and `task integration:test` all
pass. The separate GHCR visibility check for the trusted Gateway remains an
external owner action and is classified below rather than inferred from a
successful CI publication.

The final current tree has these non-template packets:

| Packet | Current state | E2E/decision evidence | Disposition |
|---|---|---|---|
| `agent-integration-discovery` | `Complete`, evidence | Skill-first workflow discovery and integration proof | Retain evidence |
| `architecture-publication` | `Complete`, evidence | Deterministic local HTML/link/render proof; Pages activation remains owner-side | Retain evidence |
| `auth-broker-deferral` | `Accepted`, explicitly deferred | Branch/main reachability and negative Catalog E2E; implementation remains detached | Retain until deferral review trigger |
| `cli-catalog-audit` | `Complete`, evidence | Catalog-derived inventory and no-removal disposition | Retain evidence |
| `gateway-official-image` | `Accepted`, external owner action pending | Official multi-architecture digest and runtime integration pass; anonymous GHCR visibility returned `401` and package-scope auth refresh timed out | Retain until GHCR owner action or explicit deferral |
| `new-user-value-e2e` | `Complete`, evidence | Four functional blind journeys plus parent PTY artifact boundary; child artifact absence is explicit | Retain evidence until findings are promoted |
| `official-image-distribution` | `Accepted`, explicit deferral | Base publication accepted; Claude/Codex build-only; public release policy deferred | Retain evidence until successor decision |
| `quickstart-runtime-docs` | `Complete`, evidence | Runnable public Quick Start/runtime evidence | Retain evidence |
| `work-packet-retirement` | `Complete`, evidence | This current classifier and cleanup record | Retain until superseded |

The following temporary packets met their deletion predicates after their
conclusions were promoted into numbered docs, README/help, tests, or the
current evidence packet, and were removed in the accompanying cleanup commit:

- `policy-review-tty`: policy/PTY behavior is covered by the current CLI,
  integration, `docs/04_harness.md`, and `docs/09_agent_readiness_validation.md`.
- `runtime-bash-shell`: Bash entry and Workspace lifetime are covered by the
  runtime image contract, integration tests, and `docs/02_architecture.md`.
- `runtime-lifecycle-reconcile`: runtime preflight and new-Workspace-only
  image selection are covered by the application tests and numbered contracts.
- `new-user-quickstart-handoff`: host/Workspace recovery and cleanup wording
  are now in README/help and the public contract.
- `pty-evidence-harness`: the capture helper, contract test, and durable
  capture boundary are in `scripts/` and `docs/04_harness.md`; the external
  artifact remains outside Git.
- `tobari-improvement-triage`: the final register and dispositions are
  preserved by this audit; it is not retained as a second permanent roadmap.

No production, branch, release, or external package state was deleted. The
detached `codex/auth-broker` ref remains intact. The only deleted repository
paths are the six temporary packet directories listed above; their commits
remain recoverable in Git history.
