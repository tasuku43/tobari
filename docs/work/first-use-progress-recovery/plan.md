# Work Plan: Make first Workspace entry legible and recoverable

- Status: Accepted / Fixed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Keep root as a finite CLI composition over current application-owned
boundaries. Add no aggregate authority and no recovery state machine. Root
observes final default-pair state, requires a human review only for exact empty
authority, performs generic Docker readiness before the first mutation, then
executes the existing initialization, final cluster, and Context-entry tasks in
dependency order. Progress observes those calls but cannot affect them.

```text
review (fresh only; no authority)
  -> check requirements          read-only preflight
  -> resolve Context             default Template + Context publication
  -> prepare protection          canonical final cluster up receipt
  -> prepare Workspace           Workspace AppliedEntry
  -> enter Workspace             child handoff
  -> child owns terminal/status
```

An existing initialized/no-default installation is not repaired by inference.
Root stops with `template list`, after which the user supplies the exact
Template reference to `template default set --id` and retries. Nondefault work
continues through exact `context list/show` then `context enter --id`.

## Progress contract

| Stage | Routine label | Exact checkpoint |
|---|---|---|
| `check_requirements` | Check requirements | closed read-only Docker CLI/selected Engine/context/Compose preflight |
| `resolve_context` | Save setup (fresh) / Use Context (existing) | exact final default Template/Context receipt; no active policy or Workspace implication |
| `prepare_protection` | Prepare protection | canonical final cluster receipt and independent current Template-policy/Policy-Memory activations |
| `prepare_workspace` | Prepare Workspace | exact last-successful Workspace AppliedEntry |
| `enter_workspace` | Enter Workspace | successful child owner handoff only |

Execution state is closed to `pending`, `running`, `succeeded`, `skipped`,
`blocked`, `failed`, and `unknown`. Review is not a stage. Desired, active,
applied, and observed facts remain separate domain facts, never stage status.

The first implementation is stable line-oriented stderr. A presentation timer
suppresses a stage that finishes within 250 ms. At one second it may add
monotonic elapsed time. At ten seconds it reveals one stage-owned bounded
substage/wait reason. Redirected stderr may repeat a bounded heartbeat no more
than every 30 seconds. TTY refresh is capped at 4 Hz and is optional; output is
meaningful without ANSI, color, cursor control, percent, ETA, or raw logs.

## Fresh review and Customize

Exact empty final authority shows one non-authoritative recommended Template
draft with Start Workspace, Customize, and Cancel. The draft has no ID,
revision, Context, default selection, or Workspace.

- Start uses the reviewed built-in standard Template body.
- Customize edits only values already representable by the final Template body
  and Runtime catalog, then returns to a complete review. It is creation, not
  Template copy and creates no lineage.
- Existing reusable designs are managed separately through `template show`,
  `template copy --from`, `template default set --id`, and `context create
  --template`; root does not turn those reference-bound tasks into name-based
  shortcuts.
- Cancel/EOF/render/terminal failure creates no state or lock and performs no
  Docker call.

## Error and recovery matrix

| Condition | Confirmed retained facts | Root action | One primary recovery |
|---|---|---|---|
| noninteractive fresh invocation | none | fail before readiness/state/Docker | rerun `tobari` in an interactive terminal |
| Docker CLI/Engine/context/Compose unavailable | none beyond prior authority | zero mutation after preflight | `doctor`; start the selected engine externally, then `tobari` |
| initialized authority without default Template | catalog retained; default absent | no inference or mutation | `template list` |
| default-pair publication confirmed, later step fails | Template/default/Context receipt retained | never roll back | `status` |
| cluster absent or stopped, known safe | desired authority retained | canonical `cluster up` is idempotent | `cluster up` or root retry only when typed outcome permits |
| cluster journal partial/unknown | prior and possible current receipts retained | no opposite write or blind retry | `cluster status`; its typed outcome must terminate causally |
| Runtime revision ready but material missing/mismatched/pruned | authority/history retained | zero implicit repair | `review runtimes` |
| Workspace entry canceled before decision | prior AppliedEntry retained | no mutation retry claim | `status` or exact `context show` as declared |
| Workspace entry decision partial/unknown | decision journal and prior AppliedEntry retained | exact same-Context recovery only | `status`, then its typed command/condition |
| desired entry differs while attached | running authority retained | launch no child and mutate no Docker | typed `wait_for_detach` condition |
| entry confirmed but attachment fails | AppliedEntry retained | do not replay reconciliation implicitly | `status` |
| child running or exited | child streams/status authoritative | cleanup is secondary | no automatic retry; report one host-owned cleanup diagnostic if needed |
| output encoding/write fails after confirmed mutation | confirmed receipt retained | never recommend same encoding-failing command | `version` for build identity, then Catalog-owned read-only recovery |

## Cancellation phases

```text
operation context
  -> bounded mutation settlement/classification context
  -> child context after handoff
  -> bounded cleanup context after child
```

Before handoff, the first SIGINT cancels only the current canonical operation.
Its owner settles/classifies through its existing bounded context, preserves
confirmed success, prints retained facts and one recovery, and root exits 130.
A late cancel cannot convert success into replay permission. After handoff,
signals and all streams belong to the child path; the exact child status,
including signal-derived status, wins over cleanup faults.

## Recovery graph enforcement

Extend the existing Catalog traversal rather than add a status model. The
machine check covers declared root/status/fault edges and verifies:

- the target Catalog path or namespace exists;
- any inputs are typed and come from the owning result, never interpolated argv;
- a read-only classifier has finite causal terminal outcomes;
- there are no self-loops, closed reference cycles, action rediscovery loops,
  unchecked opaque references, or mutation retries after unknown outcome;
- typed non-command conditions remain WP06 values, not executable strings;
- release recovery never reaches research-only auth/serve.

## Public and schema impact

- No new command, selector, flag, reference kind, public resource, or JSON
  schema is introduced.
- Root and `context enter` retain exact positional-only direct argv.
- Human stderr receives progress and retained-fact recovery. Child streams stay
  exact after handoff.
- status remains schema 3 and unchanged in ownership/call budget.
- Durable README and docs/00-04/09 replace remaining first-use Manifest text
  with Template/Context authority and document direct shell/Claude/Codex/gh
  login entry.

## Security invariants

- Review and argument validation precede state, lock, Docker, or child effects.
- External text cannot decide progress, result, or retry.
- Root never executes provider startup commands or Runtime lifecycle actions.
- Host credentials are neither inspected nor inherited. Native clients log in
  only inside the policy-applied Workspace home.
- No child starts under ambiguous attached/pending authority.
- Each nested mutation uses its existing Intent, TargetRef, Impact, journal,
  mutation-complete output, and lock owner.

## Alternatives rejected

### One atomic root transaction

Rejected because Template/Context publication, cluster activation, Workspace
AppliedEntry, and child handoff have different owners, recovery evidence, and
rollback semantics. One checkmark would overclaim later boundaries.

### A new setup/progress resource or daemon

Rejected because invocation progress has no independent owner or lifetime and
would duplicate desired/active/applied/observed state.

### Root-side Runtime or provider repair

Rejected because WP03 and the external provider own those outcomes. Implicit
repair would cross trust and reference boundaries.

### Status-driven automatic convergence

Rejected because ADR 0085 fixes status as zero mutation and Context entry plus
cluster up are the explicit reconciliation boundaries.

## Implementation order

1. Rebaseline packet and add the seven-row evaluator fixtures.
2. Add final first-entry progress domain values and deterministic renderer
   tests, including hostile text and timing bounds.
3. Add a final generic readiness use case without reconnecting predecessor
   root/cluster composition.
4. Split root orchestration at existing default-pair, final cluster, Context
   entry, and child-handoff checkpoints; preserve each nested mutation owner.
5. Thread typed Workspace preparation/handoff progress through the existing
   Context entry port without changing its authority/journal.
6. Close pre-handoff SIGINT 130 and post-handoff exact child/cleanup behavior.
7. Add Catalog-wide causal recovery validation and finite fault fixtures.
8. Promote README/docs/00-04/09 and architecture-site contracts.
9. Run focused tests, full profiles, explicit disposable-context Docker/Colima
   evidence, exact cleanup, and agent-readiness replay.
10. Delete this packet and commit the final durable handoff.

## Highest-risk areas

- Root orchestration can accidentally nest locks or collapse distinct mutation
  outcomes. Each service must retain its current lifecycle owner and intent.
- Cancellation at effect/publication boundaries can mislabel confirmed state.
- Progress emitted after child handoff can corrupt exact streams.
- Generic recovery checks can accidentally treat a Catalog path as executable
  argv or duplicate WP06 PrimaryNext.
- Runtime integration can touch the default Docker context unless every command
  is explicitly scoped and the disposable endpoint is independently verified.

## Verification

- Same semantic fixtures across TTY, redirected, narrow, NO_COLOR, root,
  explicit Context entry, failure, cancellation, status, and readiness docs.
- Boundary cancellation before/after default-pair publication, cluster receipt,
  AppliedEntry, handoff, child exit, and cleanup.
- Direct Bash/Claude/Codex/`gh auth login` exact argv/status/stream tests.
- Fresh empty, initialized/no-default, desired-only, cluster partial/unknown,
  Workspace absent/pending/attached/unknown, Runtime unavailable/unrestorable,
  stopped engine, binary upgrade, and output-failure fixtures, without adding
  evaluator rows.
- `task check`, `task security`, `task public:check`, `task release:check`.
- Explicit disposable Docker/Colima context only; record cold/warm ranges as
  evidence, never public ETA; exact resource/context cleanup.

## Rollout and rollback

Pre-public direct cutover. There is no persisted progress state or schema
migration. Root may be rolled back as code only if doing so does not re-expose
Manifest vocabulary or a noninteractive fresh mutation path. Existing final
Template, Context, Policy Memory, Workspace, Runtime, and status authority is
preserved.

## Documentation promotion

Promote the finite journey, five checkpoint semantics, cancellation/handoff
boundary, provider-neutral recovery, native-auth guidance, recovery-graph
invariant, and explicit-context integration requirement into docs/00-04/09,
README, and the generated public site before deleting this packet.
