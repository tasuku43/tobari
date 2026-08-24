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

- [x] Decide whether WP11 must precede the remaining pre-public V1 work, can be
      interleaved at a defined seam, or is deferred until after the first public
      release with compatibility obligations. Evidence: control authorized an
      immediate pre-public hard cutover before further WP05 mechanism and before
      WP09/WP06/WP10.
- [x] Name the downstream packet owners that must pause, rebase, or consume a
      stable WP11 interface. Evidence: WP05/WP09/WP06/WP10 are paused; completed
      WP03/WP04/WP07 are stable capability inputs with affected seams only.
- [x] Confirm whether the existing WP01+02 implementation remains an intentional
      intermediate state or should be treated as the migration predecessor for
      WP11. Evidence: exact predecessor is `0bbd9deb`.
- [ ] Assign an implementation owner and target only after the above ordering is
      explicit.

## Decide

- [x] Replace the open-choice list with one owner-decision target covering
      vocabulary, identity, uniqueness, defaults, deletion, authentication,
      activation, CLI/references/schemas, and migration/rollback.
- [x] Record the recommended ID mapping: predecessor WorkspaceManifestID ->
      WorkspaceTemplateID, predecessor WorkspaceID -> WorkspaceID, and one fresh
      journaled ContextID per exact Project/Template pair.
- [x] Record the recommended V1 uniqueness and selection: at most one Context
      per canonical Project/Template pair, no Context name/default/use, one
      installation default Template, bare root/status default-pair selection,
      and reference-bound explicit nondefault entry.
- [x] Record separate Template current/Context desired/Workspace applied,
      Template cluster projection, and Policy Memory current/active axes.
- [x] Record exact schema-family version transitions and the public/internal
      reference graph without aliases or dual readers.
- [x] Close mutable-name action selection: make Template show discovery, split
      direct create from exact-revision `template copy`, reserve bare root/status
      for the revalidated default pair, and add Context/Workspace reference-bound
      entry/status/delete paths.
- [x] Restore the immutable Boundary fingerprint per TemplateID and record
      separate Context-scoped Template-policy and Policy-Memory cluster receipts
      plus the independent Workspace AppliedEntry.
- [x] Close the research auth chain: keep WP04's five-path delta, require one
      unchanged Context ref for login/import/status/logout, preserve release
      absence, and make logout the exact Context-delete prerequisite.
- [x] Record exact `policy-candidate` and `policy-rule` producer/consumer graphs
      and list the Workspace plus research-auth reference producers and
      consumers.
- [x] Limit schema advancement to changed keys or semantics and retain unchanged
      helper, Runtime, Host Loopback capability, version, error, help, and build
      versions.
- [x] Make every reference-producing or reference-consuming read RoleDiscover:
      Template show, Context show, bare status, Workspace status, and research
      auth status; each exact-input read re-emits the unchanged reference.

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

- [x] WP03 Runtime retirement: update protection reads for Template current and
      retained revisions, Context desired binding, Workspace AppliedEntry, and
      observed runtime without treating Policy Memory time as `last_used`.
      Evidence: reread final `922fa792` and ADR 0080; only protection-port
      identity inputs change.
- [x] WP04 build profile: confirm standard Workspace-native auth remains
      Workspace-owned, research Broker state remains outside Template desired,
      and no build-surface mechanism changes. Evidence: reread final `cc5d14b`
      and ADR 0082; the packet recommends Context-owned fresh research login and
      quarantine rather than migration rebind.
- [x] WP05 Host Loopback: retain attachment-local authority and bind its final
      principal to Context/Workspace without changing hostname, retirement,
      route/grant lifetime, or lock order. Evidence: reread `0bbd9deb` and ADR
      0083 before paused mechanism work.
- [ ] WP06 status home: redesign routine Current/Next around Context, separate
      Template and Policy Memory revision axes, and keep reads non-reconciling.
- [x] WP07 permission resume/handoff: move candidate/rule ownership and exact
      references to Context Policy Memory while preserving Workspace observation
      correlation and same-session retry semantics. Evidence: reread final
      `77c5607` and ADR 0081; helper/transport/wait lifecycle remains unchanged.
- [ ] WP08 Catalog/domain/output conformance: rebuild reference producers,
      consumers, JSON fields, and negative vocabulary guards from the final
      Catalog.
- [ ] WP09 service exposure UX: retain attachment-local service authority rather
      than placing it in Template or Policy Memory.
- [ ] WP10 first-use progress/recovery: split Template selection/copy from Context
      creation and explicit Workspace entry without adding discovery burden.

## Implement

- [ ] Add failing domain, Catalog, output, migration, and security contract tests.
- [ ] Add a Catalog-construction test proving the proposed final Catalog
      validates and its exact derived producer/consumer sets contain no
      RoleUtility reference input or output.
- [x] Implement dormant pure Template revision/history with one complete typed
      static body, body-derived Boundary/slice/overall digests, exact copy and
      entry derivation, immutable Boundary, Context uniqueness, Policy Memory
      revision, Workspace binding/AppliedEntry, independent activation-receipt,
      and kind-specific opaque-reference invariants without connecting an
      ordinary reader or writer.
- [x] Preserve the closed Advanced executable-source boundary as exactly one
      bounded `tobari.rego`/`tobari_test.rego` pair; reject missing, renamed,
      duplicate, extra, incomplete, or oversized sources and migrate only the
      exact pair.
- [x] Implement the pure exact-predecessor migration input/plan/output and
      rollback-eligibility model with preserved Template/Workspace bytes, fresh
      journaled Context IDs, exact predecessor-body transformation, retained
      revision/policy/default/candidate mapping, research quarantine disposition,
      and no I/O.
- [x] Require bounded exact-owned Docker evidence before retaining a predecessor
      AppliedEntry; map missing, mismatched, and unknown material to explicit
      unverified state.
- [x] Implement dormant separate Template, Context, Workspace, and Policy Memory
      application use cases with task-owned smallest ports, exact unchanged ref
      consumption/re-emission, coherent domain-owned receipts, one mutation
      invoker boundary, and no Catalog or infrastructure wiring.
- [ ] Implement owner-only stores, atomic migration/rollback, principal/policy
      projection, and reconciliation adapters.
- [ ] Perform one public Catalog hard cutover with no accidental aliases.
- [ ] Update human output, JSON schemas, completion, help, examples, site,
      embedded/generated snapshots, and agent-readiness fixtures.
- [ ] Promote durable conclusions to theses, product, architecture, security,
      harness, ADR, and relevant Skill.

## Verify

- [x] Pure domain focused tests pass. Evidence: `go test
      ./internal/domain/tobari`, `go test -race ./internal/domain/tobari`,
      `go test ./internal/domain/...`, and `task check:fast` with the pinned
      Go 1.26.6/Node 24.18.0 toolchains.
- [x] Dormant application contract tests pass. Evidence: `go test -race
      ./internal/app/workspaceauthoritycmd ./internal/domain/tobari`, `go test
      ./internal/app/... ./internal/domain/...`, and `task check:fast` with the
      pinned toolchains.
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
