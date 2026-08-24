# Work Context: Make first Workspace entry legible and recoverable

## Rebaseline

- Verified independent worktree:
  `/Users/tasuku/work/github.com/tasuku43/tobari-wp10`.
- Verified branch: `codex/wp10-first-use-progress-recovery`.
- Verified starting HEAD:
  `c421588bb5a2938dee35be629bd81ab7d76b4308`.
- Verified clean status before edits.
- Verified WP03 `922fa792`, WP04 `cc5d14b`, WP07 `77c5607`, WP11
  `6ccdb424`, WP05 `dd1af498`, WP09 `e6341cf0`, and WP06 HEAD are ancestors.
- This packet's predecessor ADR 0079/Manifest design is superseded by accepted
  ADR 0084. ADR 0085 owns status schema 3 and zero-mutation observation.

## Verified current behavior

- Catalog root is `tobari [-- <command>...]`; there is no root Template or
  Context selector. Nondefault entry is `context enter --id <context-ref>`.
- `status` is RoleDiscover/EffectRead, schema 3, with one typed primary Next,
  ordered Attention, no overall status, and Docker budgets 0/6/12.
- `InitializeFinalDefaultPair` atomically publishes a fresh `default` Template,
  selects it as installation default, and creates the CWD Context with empty
  Policy Memory. It deliberately creates no Workspace or active receipts.
- Final Context entry is the sole Workspace reconciliation boundary. It
  journals an exact plan, preserves last-successful AppliedEntry, blocks live
  adoption, and uses a bounded settlement context independent from the child
  and cleanup contexts.
- Canonical final `cluster up` owns Template-policy and Policy-Memory activation
  plus shared Gateway/OPA reconciliation. Reads do not activate it.
- Current root calls initialization and Context entry directly; it does not
  compose canonical final `cluster up` between them.
- Current production composition does not bind the predecessor `tobaricmd`
  service. Final cluster and final authority services are separate.
- Existing cluster progress is predecessor-only, TTY-only, redraws at 10 Hz,
  has three presentation phases, and is not connected to final root/cluster.
- Existing direct argv parser preserves values after `--`; malformed forms do
  not reach the root handler.
- The final child owner already separates child run and bounded cleanup.
  Cleanup issues are secondary and the child exit code is retained.
- Standard native auth is Workspace-home owned. Release Catalog contains no
  research auth/serve commands.

## Reproduction evidence

Built a temporary standard binary from the exact baseline and used fresh XDG
roots. Fresh `status --format=json` returned schema 3, `authority_state=empty`,
`default_template_state=absent`, `next.path=tobari`, and created no XDG entry.

A redirected fresh root invocation then returned an undeclared-fault contract
error after creating:

- the final authority envelope;
- Template `default` generation 1 and default selection;
- one Context with empty Policy Memory;
- the lifecycle lock.

The subsequent status reported Context selected, Workspace absent, inactive
Template/Memory axes, and cluster/runtime unknown. This is verified P1 product
impact: a noninteractive attempt crosses durable mutation before the required
first-use review and cannot reach a safe Workspace.

## Authority inventory

| Owner | Scope | Lifetime | Mutability | Authority used by WP10 |
|---|---|---|---|---|
| installation | Workspace Template | explicit delete | complete immutable revisions within fixed Boundary | WorkspaceTemplateID + semantic digest |
| installation | default Template selection | until explicit replacement | exact reference-bound write | DefaultTemplateSelection |
| Project + Template | Context | until Context delete | immutable binding in V1 | ContextID |
| Context | Policy Memory | Context lifetime | complete immutable remembered-decision revisions | ContextID + semantic digest |
| Context | Workspace | replaceable until Workspace delete | last-successful entry plus bounded observation | WorkspaceID + ContextID |
| installation | Runtime | explicit retirement | managed source plus immutable successful revisions | RuntimeID + revision digest |
| invocation | first-entry progress | process lifetime only | monotonic presentation state | no durable authority |
| child path | terminal and process status | child lifetime | exact stream/status ownership | child process result |

## Trust and side-effect boundaries

- Project files, child output, Docker/BuildKit text, provider hints, image
  metadata, and terminal content are untrusted input. None may select a stage,
  result, retry, authority, or recovery edge.
- Progress accepts only typed Tobari-owned events and projects bounded fixed
  text. It stores no event history and grants no authority.
- Root may compose only existing controlled boundaries: generic readiness
  observation, final default-pair initialization, final cluster reconcile,
  final Context entry, and final session handoff.
- A stopped Docker provider is external state. Tobari neither starts nor
  identifies Colima, Docker Desktop, Podman, or another provider as authority.
- Runtime absence/mismatch is observed, never repaired by root. `review
  runtimes` remains the exact human discovery seam for restore/build choices.
- Authentication bytes remain opaque to Tobari standard first use.

## Compatibility and schemas

- Pre-public clean break: no Manifest alias, migration, or dual reader.
- status remains schema 3 and is not extended by progress.
- Existing Template/Context/Workspace/cluster/Runtime schemas and reference
  kinds remain owned by their upstream slices.
- Human stderr gains invocation-scoped progress and causal retained-fact
  summaries. No public flag, preference, event schema, percent, or ETA is added.
- The direct-command grammar and child exit contract remain compatible.

## Dependencies and collision boundaries

- WP03 owns Runtime discover/build/restore/delete/prune and opaque references.
- WP04 owns release/research program identity; release first use excludes
  research auth/serve.
- WP05 owns Host Loopback capability and migration.
- WP07 owns attachment-local permission wait, not progress or retry.
- WP09 owns Service request/review/open/stop and cleanup evidence.
- WP06 owns StatusHomeSnapshot, PrimaryNext, Attention, typed non-command
  conditions, schema 3, zero mutation, and call budgets.
- WP10 may add only root journey orchestration, progress/cancellation
  presentation, generic Catalog recovery validation, durable first-use text,
  and integration evidence.

## Implementation-time unknowns

- [ ] Measure the exact point at which final session ownership is established
      so the final progress line precedes all child bytes without delaying
      handoff.
- [ ] Measure cold/warm standard Runtime and Gateway preparation on the
      supported explicit Docker/Colima context; record evidence only, never ETA.
- [ ] Confirm whether the current final Runtime material fault taxonomy exposes
      enough typed cause for `review runtimes` without widening WP03.
- [ ] Confirm the isolated Docker context available on this host and its exact
      cleanup path before any runtime integration run.

## Glossary

- **Template:** reusable static Workspace design and policy Boundary.
- **Context:** durable relationship between one Project root and one Template.
- **Policy Memory:** remembered Allow/exact Deny authority owned by a Context.
- **AppliedEntry:** last confirmed Workspace entry receipt; not live state.
- **checkpoint:** one exact mutation or handoff fact proven by a progress stage.
- **settlement:** bounded classification after a mutation may already have
  crossed its effect boundary.
- **handoff:** the point after which child terminal streams and status are
  authoritative.
