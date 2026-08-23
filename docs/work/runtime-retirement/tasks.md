# Work Tasks: Close the managed Runtime lifecycle

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

This checklist is for future implementation. Completed packet-research items
record current evidence; no implementation, production test, durable document,
public contract, generated file, or release file was changed in this session.

## Implementation entry gate

- [x] Complete the read-only WP01 + WP02 audit against integrated ADR 0079,
      `docs/00` through `docs/04`, and final Workspace Manifest/Runtime
      interfaces. Evidence: HEAD `52a53bc`; model/copy commit `07535a9`;
      migration/security commit `428812f`.
- [ ] Wait for and inspect `WP08_IMPLEMENTATION_COMPLETE`; consume its single
      recursive Catalog output/reference traversal and final gates without
      adding a WP03-local walker. Evidence:
- [ ] Re-read the Product Owner-fixed WP03 packet and integrated governing docs,
      then record that implementation may start in the fixed order WP01 + WP02
      audit -> WP08 -> WP03. Evidence:

## Understand and approve

- [x] Fetch `origin/main`, verify local/current main identity, and protect the
      initial clean worktree. Evidence: `HEAD` and `origin/main` were
      `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42` on 2026-08-23; later concurrent
      untracked packets were not edited.
- [x] Read `AGENTS.md`, theses, product, architecture, security, harness, and
      required authentication/external-API/readiness documents in numeric order.
- [x] Inspect README, current-source Runtime agent help, Catalog, domain,
      application, infrastructure, tests, accepted Runtime ADRs, current
      lifecycle lock, and recent relevant commits.
- [x] Verify no current-main public Runtime delete, revision delete, prune,
      restore, or gc command exists.
- [x] Check official Docker image removal/prune/builder-prune/disk/storage facts
      and record exact sources/date in `context.md`.
- [x] Separate verified facts, evaluation, inferences, and unknowns in
      `context.md`.
- [x] Compare whole delete, revision delete, prune dry-run flag, separate
      discover/apply, generic gc, public source/image separation, restore, and
      image archive alternatives.
- [x] Record owner, scope, lifetime, mutability, authority/presentation identity,
      concept count, trust boundary, compatibility, migration, and packet
      conflicts.
- [x] Consume the accepted ADR 0079 model: Workspace Manifest /
      Manifest, `manifest`, `--manifest`, `workspace_manifest_id`, `workspace_id`,
      exact Runtime ID + semantic revision digest, and separate desired/last
      successful applied/observed/last bounded failure state. Evidence: ADR
      0079 and integrated `docs/00` through `docs/04` at HEAD `52a53bc`.
- [x] Product Owner fixed WP03 implementation order as WP01 + WP02 completion
      audit -> WP08 -> WP03; WP03 does not start parallel production work.
      Evidence: Product Owner packet-metadata/design-fix instruction.
- [x] Record the durable upstream retention and migration inputs without
      deciding them in WP03: complete current/retained Manifest protection
      inventory, exact read-only Docker evidence, and cluster-stopped/no-
      attachment migration precondition. Evidence: ADR 0079; `docs/02` through
      `docs/04`; commits `07535a9` and `428812f`.
- [x] Product Owner fixed this packet's head-image prune rule, exact-or-fail
      restore promise, pre-public JSON reset, `runtime.lifecycle` capability,
      bounded idempotency receipts, non-forced shared-image preservation, and no
      V1 usage receipt; these choices are not reopened during implementation.
      Evidence: Product Owner accepted the reviewed WP03 design.
- [ ] Promote the Product Owner-fixed Runtime lifecycle decision into an ADR
      revising ADR 0067 before mechanism implementation. Evidence:

## Contract and negative enforcement

- [ ] Add failing domain and Catalog tests for `runtime`, `runtime-revision`, and
      `runtime-prune-plan` opaque reference construction, validation, producer
      reachability, exact byte round trip, wrong-kind input, and closed cycles.
- [ ] Extend Catalog reference derivation/runtime validation/scoped help to
      canonical nested object/array field paths; add generic positive and
      sibling-negative tests before Runtime outputs depend on it.
- [ ] Change Runtime reads to discover roles and declare complete reference-
      producing outputs without making Docker IDs or paths reference kinds.
- [ ] Keep Runtime create as the fixed catalog-target create, emit only the
      confirmed child Runtime reference, and change build to a required
      reference-bound Runtime write with one/no/yes/no impact.
- [ ] Add read-only `review runtimes` to preserve human build selection and
      explicit confirmation; prove direct build does not accept a name, omit its
      target, or rediscover inside the action.
- [ ] Register the proposed delete, prune dry-run, prune apply, and restore specs
      with exact paths, typed inputs, roles, effects, delivery/coverage,
      prerequisites, faults, recovery, and machine invocations.
- [ ] Enforce delete mutation contract: required `runtime` `--id`,
      `target_id_input`, no parent/fixed target, impact one/no/yes/yes, and exact
      `--confirm=delete`.
- [ ] Enforce prune apply mutation contract: required `runtime-prune-plan`
      `--plan`, `target_id_input`, no parent/fixed target, impact
      many/no/yes/yes, and exact `--confirm=prune`.
- [ ] Enforce restore mutation contract: required `runtime-revision` `--id`,
      `target_id_input`, no parent/fixed target, impact one/no/yes/no.
- [ ] Add negative catalog/parser/help/generated canaries proving absence of
      `runtime revision delete`, `runtime gc`, `runtime prune --dry-run`, raw
      Docker selectors, force, recursive path inputs, public source/image delete,
      and hidden/deprecated aliases.
- [ ] Implement the fixed `runtime.lifecycle` capability ID and update its ledger
      without splitting the supported outcome from discovery, action,
      interpretation, or recovery.
- [ ] Add task-owned schema-1 output contracts and fail-closed runtime validators
      for list/show/history availability, delete, prune plan/apply, and restore.

## Domain and application

- [ ] Add immutable lifecycle domain vocabulary for references, availability,
      protection reasons, usage certainty, storage certainty, candidates,
      dispositions, journal phases, receipts, and task identities.
- [ ] Prove revision ordinals/digests/snapshots remain immutable and contiguous;
      no individual delete/tombstone/renumber path exists.
- [ ] Implement one pure protection/candidate planner shared by prune dry-run and
      apply, with deterministic ordering and exact plan-reference derivation.
- [ ] Emit distinct closed blocker reasons for current Manifest binding,
      retained immutable Manifest revision, Workspace pending adoption,
      Workspace `last_successful_entry`, observed Workspace runtime/container,
      migration-unverified state, and unknown observation; retain all applicable
      reasons rather than collapsing them.
- [ ] Add complete Workspace Manifest retention, Workspace
      `last_successful_entry`, and pending desired-adoption query ports owned by
      Runtime lifecycle use cases; reject partial/unknown/migration-unverified
      collections before effects.
- [ ] Add exact bounded Docker-use observation ports without exposing a runner,
      image ID, tag parser, or container selector above infrastructure.
- [ ] Add Runtime delete use case with stable target resolution, standard
      protection, confirmation, lifecycle-lock revalidation, journal-before-
      effect, complete result validation, and idempotent terminal receipt.
- [ ] Add zero-write prune dry-run use case and prove empty-scoped results retain
      task/scope identity and produce a typed plan reference.
- [ ] Add prune apply use case with exact plan equality, stale-before-I/O,
      per-item journal progress, preserved-shared disposition, and exact resume.
- [ ] Add restore use case with retained snapshot, normal compatibility,
      recorded digest equality, no history mutation, exact no-change, and
      `unrestorable` outcome.
- [ ] Refine build no-change and partial-failure semantics around availability,
      exact candidate ownership, journal phases, and mutation-complete output.
- [ ] Preserve structured partial/confirmed faults before cancellation and map
      unclassified outcomes to non-retryable read-only recovery.

## State, locking, and migration

- [ ] Promote one lock order across lifecycle, Runtime, Workspace Manifest,
      Workspace desired/applied state, and Docker calls; add inversion/deadlock/
      race tests before destructive code.
- [ ] Implement strict owner-only schema-1 build journals with exact Runtime and
      semantic revision identity, staging artifact, monotonic phases, bounded
      data, atomic writes, and restart reconciliation.
- [ ] Implement strict owner-only schema-1 retirement journals/receipts outside
      the retiring directory, including exact plan/target, per-item phase,
      terminal receipt, and safe same-name fresh-ID behavior.
- [ ] Implement exact validated quarantine and removal for one Runtime directory;
      reject symlink, special file, path escape, ownership/mode drift, identity
      drift, and unexpected children before mutation.
- [x] Consume the integrated ADR 0079 Workspace Manifest/Workspace state and
      migration: predecessor UUID preservation, exact-evidence-only
      `last_successful_entry`, unverified/pending fallback, and no dual public
      vocabulary. Evidence: commits `07535a9` and `428812f`.
- [x] Consume the durable cluster-stopped/no-attachment migration precondition,
      backup/recovery, and explicit post-migration `cluster up`; migration and
      reads perform no Runtime cleanup. Evidence: ADR 0079 and `docs/03`/`04`.
- [x] Consume the integrated authoritative current/retained Manifest protection
      inventory without shortening or independently redefining retention.
      Evidence: ADR 0079 Runtime-protection seam and commit `07535a9`.
- [ ] Prove existing Runtime manifest schema 1 remains valid without rewrite,
      tombstone, availability bit, usage field, prune field, or copy lineage.
- [ ] Prove list/show/history/prune dry-run on a fresh XDG root and existing V1
      root create no directories, locks, journals, timestamps, or Docker state.
- [ ] Define and test receipt retention bounds sufficient for exact retry and
      recovery; do not add a generic cleanup engine as an implicit prerequisite.

## Infrastructure

- [ ] Add bounded private source/snapshot logical-byte observation with exact
      absent/empty/unsafe results and no file-content exposure.
- [ ] Add bounded per-exact-image availability and virtual-size observation;
      keep unique/shared/reclaimable bytes unknown unless supported-platform
      evidence proves an owner-bounded authoritative value.
- [ ] Present `last_successful_entry.reconciled_at` only as
      last-successfully-applied. Keep `last_used` unknown because WP03 adds no
      V1 usage receipt; do not infer use from applied time, migration, passive
      container activity, or filesystem metadata.
- [ ] Add trusted CLI-supplied and post-build-verified Runtime owner/revision
      labels that untrusted Dockerfile content cannot override.
- [ ] Add exact staged image promotion and cleanup for build/restore; no scan by
      prefix and no ownership inference from tag or label alone.
- [ ] Add non-forced exact Tobari tag removal with foreign-tag preservation,
      stopped/running owned/foreign container protection, shared-content result,
      and no daemon-global prune.
- [ ] Explicitly exclude `docker image prune`, `docker builder prune`, `docker
      system prune`, BuildKit cache, unrelated containers/images, and external
      image stores from every argv and result.
- [ ] Add restore rebuild from immutable snapshot and enforce inspected content
      digest equality before normal tag publication.
- [ ] Test mutable/missing base and external downloads: mismatch never replaces
      immutable evidence or silently creates a new revision.
- [ ] Test process death/cancellation at every build, restore, prune, quarantine,
      filesystem removal, Docker removal, manifest/receipt, and output boundary.

## CLI, human, and JSON presentation

- [ ] Add copyable stable Runtime/revision/plan references to human and JSON
      discovery without asking humans to interpret their bytes.
- [ ] Lead human output with `name@ordinal`, lifecycle state, protection/candidate
      reason, source/snapshot disposition, last-used certainty, and exact
      confirmation consequence; keep Docker ID/tag/path out of routine output.
- [ ] Replace provisional public Docker selector/digest/snapshot-path fields with
      schema-1 semantic availability/storage/snapshot state in the same
      pre-public change; add no aliases or dual projection.
- [ ] Add complete machine results for deleted/already-deleted, empty/applied/
      already-applied prune, restored/already-available, per-item disposition,
      shared preservation, logical bytes, and explicit unknown reclaimed bytes.
- [ ] Use only `workspace_manifest_id`, optional `manifest_revision`, and
      `workspace_id` in protection/machine results; add negative schema canaries
      for `context`, `context_id`, `project_id`, and `instance_id` aliases.
- [ ] Add typed semantic fixtures and answer keys for zero revision, head
      candidate, current/retained Manifest protection, pending adoption, old
      last-successful Workspace protection, stopped/running container, shared
      tag, missing image, failed candidate,
      interrupted action, same-name reuse, unknown last-used, and empty plan.
- [ ] Add negative-inference canaries proving labels, names, ordinals, head,
      proximity, output order, Docker tags/IDs, paths, sizes, and timestamps do
      not create authority, protection, availability, or certainty.
- [ ] Add hostile terminal/JSON projection tests and prove opaque refs bypass
      visible escaping while every external text field is safely projected.
- [ ] Prove mutation output failure after commit reports confirmed change and
      read-only recovery without replay permission.

## Integration and concurrency

- [ ] Test multiple Workspace Manifests selecting the same revision
      and different revisions of one Runtime.
- [ ] Test desired Runtime changes while an existing Workspace
      `last_successful_entry` and stopped/running work container still use the
      old revision; preview reports pending adoption separately and cleanup does
      not reconcile or recreate an attached Workspace.
- [ ] Test Workspace Manifest deletion leaves Runtime untouched; only a later
      dry-run after authoritative retained-revision expiry and removal of every
      applied/pending/observed edge reports a newly unreferenced revision.
- [ ] Test concurrent Runtime build/restore/delete/prune, Manifest accepted
      mutation/delete, Workspace create/entry/delete, and status/read operations
      under the accepted lock order.
- [ ] Byte-compare synthetic Project/root indexes, Workspace homes, native and
      brokered authentication state, learned policy, Workspace Manifest state,
      shared cluster state, and unrelated Docker resources before/after every
      destructive scenario.
- [ ] Prove Runtime delete/prune never deletes or rewrites a Workspace's
      `workspace_id`, home, `last_successful_entry`, pending-adoption state, or
      last bounded failure, even when its runtime container is absent.
- [ ] Test zero-revision Runtime deletion, standard Runtime protection, invalid
      catalog state, unsafe snapshot state, missing Docker, foreign container
      use, shared tags/layers, and partial external deletion.
- [ ] Test an externally removed referenced image: reads remain truthful,
      Workspace Manifest authority remains unchanged, Workspace entry gives exact
      recovery, restore is exact-or-fail, and no history is rewritten.

## Documentation, compatibility, and generated contracts

- [ ] Promote the accepted lifecycle to a new ADR revising ADR 0067 and align it
      with accepted ADR 0079 and integrated `docs/00` through `docs/04`.
- [ ] Update theses, product contract, architecture, security model, threat
      model, harness, authentication protected-target notes, and agent-readiness
      scenarios in numeric/governing order.
- [ ] Preserve ADR 0079's durable `runtime create --copy-source-from` contract;
      do not add lineage or a parent deletion guard.
- [ ] Coordinate desired/applied/last-used presentation with `status-home` and
      first-entry recovery with `first-use-progress-recovery`.
- [ ] Consume the completed WP08 nested output validation contract in
      [Architecture](../../02_architecture.md) and [Harness](../../04_harness.md)
      rather than adding a second schema checker.
- [ ] Update README, exact human/agent help, capability/schema/claim ledgers,
      architecture-site data, examples, fixtures, release locks, and generated
      files only after the integrated public contract is final.
- [ ] Prove no public compatibility alias, hidden old JSON field, generic
      migration reader, raw Docker fallback, or revision-delete dormant path
      remains.
- [ ] Prove unpublished `runtime build --name` and action-local omitted-name
      Review are absent; document `runtime create`/list/show-produced references
      and the separate `review runtimes` flow.
- [ ] Delete this temporary packet only after durable conclusions and mechanical
      enforcement are complete.

## Verify

- [ ] Focused domain/application/CLI/infrastructure tests pass. Evidence:
- [ ] Race and interruption tests pass. Evidence:
- [ ] Supported Docker Desktop/Linux Engine observations pass with versions and
      image-store modes recorded. Evidence:
- [ ] Agent-readiness scenario completes with at most one exact-command help
      retrieval plus one local discovery and zero external processing. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes. Evidence:
- [ ] Generated diff contains only expected public/catalog/schema/site/release
      consequences and repository status preserves unrelated changes. Evidence:

## Hand off

- [ ] Every acceptance criterion has direct evidence and every unknown is
      resolved or promoted as an explicit reviewed non-goal.
- [ ] Goal becomes `Complete` only when implementation and all gates are truly
      complete; this Planned packet is not completion evidence.
- [ ] Temporary diagnostics, Docker test assets, journals, synthetic homes, and
      sensitive or host-specific paths are removed.
- [ ] Durable decisions are promoted and this temporary packet is removed in
      the same completion handoff.
- [ ] Handoff explains outcome, public concepts, hidden/retired surfaces,
      schema/migration, trust-boundary change, cross-packet dependencies, checks,
      and residual risk.
- [ ] Commit the completed WP03 implementation and record exact final HEAD plus
      clean or fully explained `git status`. Evidence:
- [ ] Notify root orchestration thread
      `01a02c51-885b-7b80-a66f-05850f48ba4d` with exactly
      `WP03_IMPLEMENTATION_COMPLETE`, or `WP03_IMPLEMENTATION_BLOCKED` when an
      integration/gate fact prevents completion. Include final public/internal
      interfaces, all gate results, HEAD/status, packet retention/removal, and
      explicit WP04 readiness. Evidence:
