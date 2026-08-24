# Work Tasks: WP11 — Separate Workspace Template, Context, and Policy Memory

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

This packet records the accepted final-only cutover and its executable closure
evidence. Production implementation was authorized by the product-owner and
root decisions recorded in `context.md`.

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
- [x] Decide the disposition of existing WP01+02 development state. Evidence:
      the later product-owner clean-break decision supersedes the earlier exact-
      predecessor plan; it is unsupported state to reset and recreate, not a
      migration target.
- [x] Assign an implementation owner and target only after the above ordering is
      explicit. Evidence: the root task owns the atomic final-only cutover;
      shared production files were frozen and edited by one owner per batch.

## Decide

- [x] Replace the open-choice list with one owner-decision target covering
      vocabulary, identity, uniqueness, defaults, deletion, authentication,
      activation, CLI/references/schemas, and the pre-release clean break.
- [x] Supersede the earlier predecessor ID-mapping decision. Final
      WorkspaceTemplateID, ContextID, and WorkspaceID are created only by final
      tasks; no predecessor identity is converted or adopted.
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

- [x] Approve the Workspace Template / Context / Policy Memory / Workspace
      aggregate split. Evidence: ADR 0084 and the promoted governing contracts.
- [x] Finalize public and internal vocabulary, retiring public `Manifest` and
      retaining `Policy Memory` as Context-owned authority.
- [x] Decide Context identity and uniqueness: one opaque ContextID per canonical
      ProjectRoot/TemplateID pair, with no mutable Context name selector.
- [x] Decide default Template versus per-Project Context selection semantics.
      Evidence: one installation default Template plus an exact canonical-root
      Context, initialized atomically by the bare first-use task.
- [x] Fix Template follow-current, AppliedEntry, pending adoption, failure,
      retry/recovery, policy activation, and read-only observation contracts.
- [x] Fix Template, Context, Policy Memory, Workspace, home, and authentication
      deletion/preservation behavior.
- [x] Fix public commands, roles, effects, reference kinds, producer/consumer
      graph, output/schema versions, faults, confirmations, and next actions.
- [x] Fix pre-release compatibility and first-public-release timing. Evidence:
      final-only clean break, bounded legacy-presence rejection, explicit
      reset/recreation, no migration/rollback selection, and no post-release
      precedent.
- [x] Revise or supersede ADR 0079 and obtain product/security/architecture
      approval. Evidence: accepted ADR 0084, superseded ADRs 0070/0079, and the
      propagated theses, product, architecture, and security contracts.

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
      rejects predecessor authority rather than rebinding it.
- [x] WP05 Host Loopback: retain attachment-local authority and bind its final
      principal to Context/Workspace without changing hostname, retirement,
      route/grant lifetime, or lock order. Evidence: reread `0bbd9deb` and ADR
      0083 before paused mechanism work.
- [x] WP06 status home handoff: publish independent desired/current and active
      Template-policy, Policy-Memory, and AppliedEntry axes without reconciling
      from reads. The broader WP06 presentation packet remains future work.
- [x] WP07 permission resume/handoff: move candidate/rule ownership and exact
      references to Context Policy Memory while preserving Workspace observation
      correlation and same-session retry semantics. Evidence: reread final
      `77c5607` and ADR 0081; helper/transport/wait lifecycle remains unchanged.
- [x] WP08 Catalog/domain/output conformance handoff: rebuild reference
      producers, consumers, JSON fields, and negative vocabulary guards from the
      final Catalog. Evidence: the exhaustive Batch D evaluator.
- [x] WP09 service exposure handoff: retain attachment-local service authority;
      no WP11 authority owns it. The broader WP09 packet remains future work.
- [x] WP10 first-use handoff: atomically initialize the default Template/Context
      pair and enter through the explicit Workspace reconciliation contract.
      The broader WP10 progress presentation remains future work.

## Implement

- [x] Add domain, Catalog, output, clean-break, and security contract tests.
- [x] Add a Catalog-construction test proving the proposed final Catalog
      validates and its exact derived producer/consumer sets contain no
      RoleUtility reference input or output. Evidence:
      `TestADR0084WholeCatalogReferenceGraphIsExact` in Batch D.
- [x] Implement dormant pure Template revision/history with one complete typed
      static body, body-derived Boundary/slice/overall digests, exact copy and
      entry derivation, immutable Boundary, Context uniqueness, Policy Memory
      revision, Workspace binding/AppliedEntry, independent activation-receipt,
      and kind-specific opaque-reference invariants without connecting an
      ordinary reader or writer.
- [x] Preserve the closed Advanced executable-source boundary as exactly one
      bounded `tobari.rego`/`tobari_test.rego` pair; reject missing, renamed,
      duplicate, extra, incomplete, or oversized sources.
- [x] Historical evidence only: the dormant pure exact-predecessor
      migration/rollback model and bounded Docker mapping were implemented and
      reviewed before the clean-break decision. They are no longer WP11
      acceptance authority and no final task may select them.
- [x] Implement dormant separate Template, Context, Workspace, and Policy Memory
      application use cases with task-owned smallest ports, exact unchanged ref
      consumption/re-emission, coherent domain-owned receipts, one mutation
      invoker boundary, and no Catalog or infrastructure wiring.
- [x] Bind direct Allow/Deny/Reset results to exact changed authority: complete
      candidate Context/observing-Workspace/effect evidence, expected decision,
      exact previous revision, and plus-one/minus-one full rule-set
      reconstruction; final candidates require complete final authority without
      a predecessor read.
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
- [x] Historical evidence only: a dormant journaled migration engine and
      internal rollback seam were implemented and reviewed before the clean-
      break decision. They remain unreachable and are not selected by current or
      final readers, Catalog, or public migration composition.
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
- [x] Wire one final-only reader composition and bounded legacy-presence guard
      with the atomic Catalog cutover. A genuinely fresh installation is exact
      empty authority; any legacy/unsafe/ambiguous presence is zero-mutation
      reset guidance. Inject final protection/principal/policy/auth authority
      without changing WP03/04/07 mechanisms or selecting migration.
- [x] Implement one dormant lifecycle-owned final settlement coordinator for
      first entry, existing Workspace AppliedEntry/creation-authority changes,
      Workspace retirement/re-entry, Context deletion, direct Policy
      Allow/Deny/Reset, and cluster current/current candidates. Evidence: one
      durable effect class chooses OPA-only only when Gateway bytes, principals,
      topology, selected component images/profile/env/mount closure all match;
      otherwise one exact journal keeps OPA deny-all through global zero-owner
      fences, candidate principal CAS, bounded healthy Gateway+OPA replacement,
      candidate OPA, global/per-axis receipt confirmation, and envelope
      publication. Journaled environment and selected image identities survive
      process ambient drift; cluster and settlement journals exclude each other;
      interruption resumes the same action without repeated replacement.
- [x] Route direct Gateway-changing Policy Memory Allow/Deny/Reset through the
      same final coordinator, while byte-identical Gateway content retains the
      bounded OPA-only path. Workspace and Context deletion now retire their
      complete active principal/policy authority in the initiating durable
      parent decision; fully inactive Contexts are omitted without adoption,
      partial active axes fail closed, and Context deletion can produce an exact
      empty active projection. No current composition exposes these adapters.
- [x] Implement fixed-target `policy apply-reviewed` as one complete reviewed-set
      settlement and `EffectCreate` whose fixed decision-set scope produces only
      resulting active policy-rule child references: advance every reviewed
      target Policy Memory together, preserve
      all non-target memories and all active Template-policy axes, and publish
      one global Gateway/OPA/principal receipt. It must not sequence one
      settlement per Context or use cluster current/current selection. Add
      normal nonempty application, same-set terminal replay with zero repeated
      external effect and the original changed result, multi-Context
      GraphQL+HTTP, interruption, and zero-partial-adoption fixtures before
      public cutover. Empty reviewed sets are invalid rather than no-op. The
      dormant concrete boundary now validates exact previous→set→next authority
      before any external effect and binds the set digest in its private active
      receipt. Review items use one strict canonical ReviewItemID order, so
      reversed enumeration resumes the same durable set rather than inventing
      another action identity; public/current composition remains intentionally
      absent.
- [x] Implement the dormant final-Context research authentication seam without
      selecting it from the current Catalog/composition. Login/import/status/
      logout consume one unchanged Context ref; mutation results retain exact
      Context-parent/task/provider/method/runtime decision authority, while
      status is a zero-mutation exhaustive Context-scoped Broker inventory and
      re-emits only `context_ref`. Login/import/logout and Context deletion
      serialize through one installation lifecycle authority. A durable
      secret-free no-envelope decision distinguishes pre-effect interruption
      from a confirmed Broker consequence, logout converges idempotently, and
      a later explicit identical login/import remains a new rotation. Context
      deletion uses the bounded exhaustive final inventory, including removed
      providers, and never treats locked/incomplete/unsafe state as absence.
      Container-backed acquisition resolves an exact immutable Runtime image
      from stable final Runtime authority; standard providers do not inspect a
      Runtime image. Predecessor credentials are never read, adopted, or
      rebound, release remains unavailable, and the five research paths remain
      unchanged and still unwired in this concern.
      The strict read-only status path uses the existing non-creating lifecycle
      observation plus exact two-pass final-envelope and Broker-inventory
      equality, so a fresh read creates no state directory or lock and drift is
      observation failure. Active recovery retains the complete normalized
      reviewed Provider body; a same-ID owner-manifest change cannot substitute
      another acquisition/projection plan after the durable decision.
- [x] Perform one public Catalog hard cutover with no accidental aliases.
- [x] Remove the ADR 0070 `migrate apply` predecessor capability and prove no
      WP11 migration/rollback engine, preflight, cutoff selector, quarantine, or
      predecessor decoder is reachable from public composition.
- [x] Prove predecessor files cannot influence final Runtime protection, policy,
      principal/session, or authentication; legacy-only and legacy-plus-final
      presence return typed reset guidance with zero mutation.
- [x] Prove clean installation -> exact final-empty -> first Template, Context,
      Workspace entry, and final recovery without a migration prerequisite.
- [x] Update human output, JSON schemas, completion, help, examples, site,
      embedded/generated snapshots, and agent-readiness fixtures.
- [x] Promote durable conclusions to theses, product, architecture, security,
      harness, and ADR 0084. No separate Skill contract required a semantic
      change; the repository capability and schema ledgers are synchronized.

## Atomic-cutover closure ledger

This ledger is the closed implementation backlog. A later observation extends
one representative fixture unless it proves a supported P0/P1 transition that
cannot fit any listed root.

| Batch | Root | Frozen invariant and representative proof | Status |
| --- | --- | --- | --- |
| A | B1+B5 default pair, first entry, Context axes | One final snapshot exposes desired Template revision/policy digest, independently nullable active Template-policy digest, current and independently nullable active Policy-Memory revisions, and independently nullable Workspace AppliedEntry. Fresh default Template+Context publish in one envelope without name selection. Entry settles inactive/stale current axes in its existing parent protocol. Cover fresh, exact no-op, A-active/B-desired, selection drift, decision/effect/envelope/terminal interruption, and cancellation after confirmed effect in domain, Store/runtime, app, and public schema tests. One evaluator: `sh docs/work/workspace-template-context-policy-memory/check_batch_a.sh`. | Complete: evaluator passes standard and race across domain, Store, application, and CLI; final review found no remaining production divergence in this frozen bundle. |
| B | B2 Template mutations | Every shell, Git, AWS, EKS, and Runtime change is one typed delta derived from current authority under the lifecycle lock. Unrelated fields and retained revisions survive concurrency; only Runtime change binds the distinct Runtime-revision parent. Cover normal, same-field last success, different-field serialization, stale Runtime authority, cancellation, and exact outputs. One evaluator: `sh docs/work/workspace-template-context-policy-memory/check_batch_b.sh`. | Complete: evaluator passes standard and race across domain, final Store, fixed host-source adapters, application, and CLI; bounded review found no remaining Batch B matrix cell. |
| C | B3 reviewed Permission Inbox | One coherent final snapshot owns pending candidates, exact source rules, and immutable reviewed set. TTY and research serve submit the same unchanged set once to the accepted global ApplyReviewed protocol. Cover refresh invalidation, multi-Context selection, stale snapshot, every durable recovery branch, and result delivery. One evaluator: `sh docs/work/workspace-template-context-policy-memory/check_batch_c.sh`. | Complete: evaluator passes standard and race across domain, final Store, application, CLI, and the research operator-console HTTP boundary. Exact/template multi-Context sets, refresh invalidation, stale zero-effect, one unchanged Apply, public schema/ref projection, same-set recovery, and terminal delivery are green; bounded frozen review found zero remaining B3 matrix cells. |
| D | B4+B6+C1 public composition | Registered root/status/cluster/policy/auth paths use only final services. One whole-Catalog test fixes exact reference producers/consumers and zero RoleUtility edges; repository negatives reject Manifest commands, flags, keys, predecessor readers, and public WP11 migration. Release/research path sets and schemas are exact. | Complete: `check_batch_d.sh` passes standard, race, research, and research-race across CLI, final Store, and runtime. Root/status, cluster lifecycle/log/denial, policy, and research-auth paths reach final task-owned services; bounded cluster reads use final Store selection plus before/after final component receipts. Exact Catalog refs, schemas, RoleUtility zero edges, legacy flag/path/key and migration absence, closed legacy guard, and the exact research +5 path delta are green. One frozen read-only review found zero remaining D matrix cells. |

Only the primary implementation owner edits shared production state-machine or
composition files within an active batch. Parallel work is limited to fixtures
and read-only audits. Each batch freezes only after focused standard and race
gates pass for its complete vertical proof bundle.

## Verify

- [x] Pure domain focused tests pass. Evidence: `go test
      ./internal/domain/tobari`, `go test -race ./internal/domain/tobari`,
      `go test ./internal/domain/...`, and `task check:fast` with the pinned
      Go 1.26.6/Node 24.18.0 toolchains.
- [x] Dormant application contract tests pass. Evidence: `go test -race
      ./internal/app/workspaceauthoritycmd ./internal/domain/tobari`, `go test
      ./internal/app/... ./internal/domain/...`, and `task check:fast` with the
      pinned toolchains.
- [x] Historical dormant journal-engine and owner-store focused tests pass; the
      engine evidence predates the clean-break decision and is not a current
      cutover acceptance requirement. Evidence:
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
- [x] Dormant final Gateway settlement focused tests pass. Evidence: complete
      standard and race runs of `internal/domain/tobari`,
      `internal/infra/workspaceauthoritystore`, and
      `internal/infra/dockerruntime` cover exact OPA-only/full classification,
      selected image/profile/env/mount/topology closure, bounded component
      readiness, cluster-journal exclusion, canonical session expiry and both
      zero-owner fences, principal-before-component deny fencing, every
      post-effect resume boundary, first entry, Workspace and Context deletion,
      inactive Context omission/partial-axis rejection, and terminal same-action
      replay. Canonical/helper sources are byte-identical and `task check:fast`
      passes with pinned Go 1.26.6/Node 24.18.0.
- [x] Dormant final-Context research authentication focused tests pass.
      Evidence: complete standard and focused race runs of
      `internal/domain/authbroker`, `internal/app/authcmd`,
      `internal/infra/workspaceauthoritystore`, and
      `internal/infra/dockerruntime` cover exact Context A/B isolation,
      exhaustive multi-provider status and deletion absence, non-creating
      two-pass status observation, stable complete Provider recovery,
      immutable Runtime image execution, no-envelope interruption/terminal
      recovery, consecutive credential rotation, release absence, and
      predecessor non-adoption. `task authbroker:test` passes all
      123 tests; canonical/helper sources are byte-identical.
- [x] All four closure evaluators pass on one snapshot. Evidence: Batch A and B
      standard/race, Batch C standard/race/research/research-race, and Batch D
      standard/race/research/research-race.
- [x] `task check:fast` passes. Evidence: pinned Go 1.26.6, Node 24.18.0,
      npm 11.16.0; final CLI/runtime release and research suites, site generation,
      site typecheck/build, helper snapshots, contract/architecture guards, and
      repository tests all pass.
- [x] `task check` passes on the final cutover snapshot. Evidence: pinned Go
      1.26.6, Node 24.18.0, and npm 11.16.0 full profile, including release and
      research surfaces, site static/browser checks, all packages, and race.
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes when the release surface is affected. Evidence:
- [ ] Isolated runtime/integration behavior passes without changing a user-owned
      cluster. Evidence:
- [ ] Agent-readiness covers Template creation/copy, Context creation/selection,
      Workspace entry/recreation, and retained policy learning with zero
      undeclared external processing. Evidence:
- [x] Generated diff and repository status are understood. Evidence: changes
      are limited to the accepted harness/docs/domain/app/infra/CLI/sitegen
      cutover graph and its helper snapshot; `git diff --check` and exposure
      helper equality pass. The commit-fixed site source snapshot and temporary
      packet retirement are intentionally the next mechanical concern.

## Hand off

- [x] Send this Draft packet to the control task for ordering against current V1
      work.
- [ ] Acceptance criteria have evidence.
- [ ] Goal status is changed to `Complete` only after all goal and task
      checkboxes are complete; `Superseded` names a canonical successor goal.
- [ ] Durable decisions were promoted out of the work packet.
- [ ] Temporary diagnostics and sensitive artifacts were removed.
- [ ] Follow-up work is explicit and does not block this goal.
- [ ] Final handoff explains outcome, why, checks, pre-release clean-break
      disposition, and risks.
