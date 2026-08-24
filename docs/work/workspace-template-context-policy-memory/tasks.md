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
- [x] Bind direct Allow/Deny/Reset results to exact changed authority: complete
      candidate Context/observing-Workspace/effect evidence, expected decision,
      exact previous revision, and plus-one/minus-one full rule-set
      reconstruction; carry actionable candidates through migration without a
      predecessor read.
- [x] Validate exhaustive Context and Workspace collections as aggregate
      authority: unique ProjectRoot+TemplateID Context pairs, unique optional
      Workspace IDs, at most one Workspace per Context, exact equality for a
      repeated TemplateID, and unique installation Template names.
- [x] Implement one bounded owner-only final-authority envelope and
      zero-mutation coherent Template/Context/Workspace observations. Reads
      validate the complete normalized authority, never consult predecessor
      Manifest state, and return empty/not-found without creating a root or
      lock.
- [x] Implement the dormant owner-only final-envelope mutation adapter for
      Template/default, Context, Workspace retirement, and direct Policy Memory
      operations. Serialize through the installation lifecycle authority,
      preserve semantic no-op generations, classify publication by exact
      read-back, durably bind external effects before execution, exclude
      different mutations during recovery, and retain one bounded terminal
      receipt for zero-repeat same-ref result replay.
- [x] Implement the dormant journaled migration engine and internal rollback
      seam: exact owner-only preflight facts, one final-envelope publication,
      atomic cutoff selection, same-filesystem predecessor quarantine,
      research disposition, byte-untouched standard homes, idempotent committed
      apply, terminal rollback, crash recovery, and kernel-released exclusion.
      No current reader or invocable migration route selects it yet.
- [x] Implement dormant explicit Context-entry reconciliation. Consume one
      unchanged Context ref; derive desired authority from the final Template;
      require independent current Template-policy and Policy-Memory receipts;
      bind WorkspaceID-derived home and exact 64-hex container observation;
      publish AppliedEntry only after bounded confirmation; preserve prior
      last-successful state and one durable same-ref recovery decision across
      interruption; and invoke the session-owner port under the existing
      lifecycle authority while running the returned owner only after lock
      release.
- [x] Implement the concrete final-identity bridge from the dormant
      `WorkspaceSessionAuthority` port to WP07's canonical interactive
      attachment/session registry, with exact Context/Workspace principal
      projection and live-session tests. Evidence: a host-only sibling bridge
      keeps dockerruntime independent of the final store; complete final entry
      authority validates Template/Runtime/spec/container before beginning;
      frozen `context_id`/`project_id`/`context` values carry final
      Context/Workspace/presentation; Begin borrows the same epoch/nonce/wait
      owner; Run double-observes exact principal+session around liveness before
      route/channel/child effects; and persistent snapshot identity classifies
      missing/empty registries as absent while malformed, stale, or owner-loss
      evidence fails closed. Current composition/Catalog remain unchanged.
- [x] Implement the dormant final-authority policy projection and hot Policy
      Memory activation adapter. Evidence: one complete collection selects
      active Template slices independently from the target current Policy
      Memory; materialization binds exact healthy Gateway/container/network/IP
      evidence, retained Workspace creation defaults, current Workspace IDs,
      rendered OPA/Gateway artifacts, and one global publication receipt. A
      journal is durable before OPA effects, large receipts use a bounded
      task-owned codec, same-content confirmation survives unrelated collection
      changes, and Docker/Gateway/artifact drift fails before mutation. Cluster
      current/current projection is prepared as a dormant candidate only.
- [ ] Wire the exact predecessor adapter and migration selection only with the
      atomic final-reader cutover; implement principal/policy projection and
      reconciliation adapters without changing WP03/04/07 mechanisms.
- [ ] Close the first-entry principal-publication seam in the later atomic
      entry/runtime projection concern: a newly reconciled Workspace must
      publish its exact final principal under the same lifecycle decision before
      `BeginFinalWorkspaceSession`, without making `cluster up` an undocumented
      prerequisite. The same settlement must cover an existing Workspace whose
      AppliedEntry/creation authority changes and Workspace retirement that
      removes the last principal. The dormant policy adapter does not implement
      or claim entry/deletion principal, Gateway-mount, OPA, or receipt
      settlement, and no current composition exposes it.
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
- [x] Dormant journal engine and owner-store focused tests pass. Evidence:
      `go test ./internal/infra/workspaceauthoritymigration` and `go test -race
      ./internal/domain/... ./internal/app/...
      ./internal/infra/workspaceauthoritystore
      ./internal/infra/workspaceauthoritymigration` with the pinned Go 1.26.6
      toolchain; `task check:fast` passes with pinned Go 1.26.6 and Node
      24.18.0 after the default shell's older toolchains were rejected by the
      expected preflight.
- [x] Dormant final projection/hot-activation focused tests pass. Evidence:
      complete domain/app/infra package tests; focused standard and race tests
      cover independent hot/cluster axes, no-op confirmation, stale/missing
      authority, exact principal/Gateway topology and health, artifact/receipt
      mismatch, post-effect journal recovery, the task-owned >128 KiB codec,
      and over-ceiling rejection before OPA; helper source snapshot and
      `task check:fast` pass with pinned Go 1.26.6/Node 24.18.0.
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
