# Work Tasks: WP11 — Separate Workspace Template, Context, and Policy Memory

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

This packet records a design direction for parent sequencing. No production
implementation is authorized by creating it.

## Understand

- [x] Read governing theses, product, architecture, security, harness, and ADR
      0079 sections relevant to Manifest, Workspace, learned policy, migration,
      and explicit reconciliation.
- [x] Inspect the current Manifest revision, Workspace/AppliedEntry, policy-rule,
      application-port, infrastructure-store, Catalog, and harness seams.
- [x] Record verified facts separately from proposed design in `context.md`.
- [x] Record the repeated static-definition/dynamic-policy ownership confusion as
      thesis evidence.
- [x] Confirm that the immediate outcome is a schedulable design packet, not an
      implementation or public-contract change.

## Parent sequencing decision

- [ ] Decide whether WP11 must precede the remaining pre-public V1 work, can be
      interleaved at a defined seam, or is deferred until after the first public
      release with compatibility obligations.
- [ ] Name the downstream packet owners that must pause, rebase, or consume a
      stable WP11 interface.
- [ ] Confirm whether the existing WP01+02 implementation remains an intentional
      intermediate state or should be treated as the migration predecessor for
      WP11.
- [ ] Assign an implementation owner and target only after the above ordering is
      explicit.

## Decide

- [ ] Approve the Workspace Template / Context / Policy Memory / Workspace
      aggregate split.
- [ ] Finalize public and internal vocabulary, including the remaining role of
      `Manifest` and the final name for Policy Memory.
- [ ] Decide Context authority identity, uniqueness, naming, and whether same
      Project + same Template can have multiple Contexts.
- [ ] Decide default Template versus per-Project Context selection semantics.
- [ ] Fix Template follow-current, AppliedEntry, pending adoption, failure,
      retry/recovery, policy activation, and read-only observation contracts.
- [ ] Fix Template, Context, Policy Memory, Workspace, home, and authentication
      deletion/preservation behavior.
- [ ] Fix public commands, roles, effects, reference kinds, producer/consumer
      graph, output/schema versions, faults, confirmations, and next actions.
- [ ] Fix exact migration/rollback/compatibility and first-public-release timing.
- [ ] Revise or supersede ADR 0079 and obtain product/security/architecture
      approval.

## Cross-packet audit

- [ ] WP03 Runtime retirement: update protection reads for Template current and
      retained revisions, Context desired binding, Workspace AppliedEntry, and
      observed runtime without treating Policy Memory time as `last_used`.
- [ ] WP04 build profile: confirm standard Workspace-native auth and research
      Broker state do not move into Template or Context implicitly.
- [ ] WP05 Host Loopback: retain attachment-local authority and bind any
      presentation identity to Context/Workspace without changing its lifetime.
- [ ] WP06 status home: redesign routine Current/Next around Context, separate
      Template and Policy Memory revision axes, and keep reads non-reconciling.
- [ ] WP07 permission resume/handoff: move candidate/rule ownership and exact
      references to Context Policy Memory while preserving Workspace observation
      correlation and same-session retry semantics.
- [ ] WP08 Catalog/domain/output conformance: rebuild reference producers,
      consumers, JSON fields, and negative vocabulary guards from the final
      Catalog.
- [ ] WP09 service exposure UX: retain attachment-local service authority rather
      than placing it in Template or Policy Memory.
- [ ] WP10 first-use progress/recovery: split Template selection/copy from Context
      creation and explicit Workspace entry without adding discovery burden.

## Implement

- [ ] Add failing domain, Catalog, output, migration, and security contract tests.
- [ ] Implement Template revision and Context/Policy Memory invariants.
- [ ] Implement separate application use cases and smallest owned ports.
- [ ] Implement owner-only stores, atomic migration/rollback, principal/policy
      projection, and reconciliation adapters.
- [ ] Perform one public Catalog hard cutover with no accidental aliases.
- [ ] Update human output, JSON schemas, completion, help, examples, site,
      embedded/generated snapshots, and agent-readiness fixtures.
- [ ] Promote durable conclusions to theses, product, architecture, security,
      harness, ADR, and relevant Skill.

## Verify

- [ ] Focused tests pass. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes when the release surface is affected. Evidence:
- [ ] Isolated runtime/integration behavior passes without changing a user-owned
      cluster. Evidence:
- [ ] Agent-readiness covers Template creation/copy, Context creation/selection,
      Workspace entry/recreation, and retained policy learning with zero
      undeclared external processing. Evidence:
- [ ] Generated diff and repository status are understood. Evidence:

## Hand off

- [x] Send this Draft packet to the control task for ordering against current V1
      work.
- [ ] Acceptance criteria have evidence.
- [ ] Goal status is changed to `Complete` only after all goal and task
      checkboxes are complete; `Superseded` names a canonical successor goal.
- [ ] Durable decisions were promoted out of the work packet.
- [ ] Temporary diagnostics and sensitive artifacts were removed.
- [ ] Follow-up work is explicit and does not block this goal.
- [ ] Final handoff explains outcome, why, checks, migration, and risks.
