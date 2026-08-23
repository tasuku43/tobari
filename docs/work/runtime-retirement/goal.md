# Work Goal: Close the managed Runtime lifecycle

- Status: Accepted
- Decision state: Fixed by Product Owner; the accepted WP03 contract is not
  reopened by implementation planning
- Implementation readiness: Planned; implementation not started
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md` through `docs/04_harness.md`,
  `docs/07_authentication.md` through `docs/09_agent_readiness_validation.md`,
  and the ADR required by this packet
- Review/delete trigger: Delete after the approved lifecycle is promoted,
  implemented, verified through every required gate, and the change completes
- Successor: None
- Owner: Tobari product, domain, Runtime, Workspace Manifest, Workspace, and
  security maintainers
- Target: Before the first public V1 contract is frozen
- Related ADRs: ADR 0067, ADR 0069, ADR 0070, ADR 0071, ADR 0077, ADR 0078,
  and [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
- Upstream durable contract: ADR 0079 plus integrated `docs/00_theses.md`
  through `docs/04_harness.md`; commits `07535a9` and `428812f` are the
  model/copy and migration-security implementation evidence
- Related work: [status home](../status-home/goal.md),
  [first-use progress and recovery](../first-use-progress-recovery/goal.md),
  [first public release core](../first-public-release-core/goal.md), and
  [first public release artifacts](../first-public-release-artifacts/goal.md)
- Fixed implementation order: WP01 + WP02 completion audit -> WP08 -> WP03
- Completion notification: the future implementation owner reports
  `WP03_IMPLEMENTATION_COMPLETE` or `WP03_IMPLEMENTATION_BLOCKED` to the root
  orchestration thread with final interfaces, gates, HEAD/status, packet
  retention, and WP04 readiness

## Outcome

A user can deliberately retire an unused managed Runtime, reclaim only safely
unused Runtime-owned Docker image material after reviewing an exact plan, and
restore a missing revision image when exact reconstruction remains possible.
Tobari preserves every referenced immutable Runtime revision, every retained
immutable Workspace Manifest revision binding, every Workspace's last
successful applied entry and pending desired adoption, every Workspace and
Workspace home, and the built-in standard Runtime. All destructive work has
stable opaque target identity,
explicit confirmation, a complete machine result, and idempotent recovery from
interruption without asking the user to identify a Docker image or container.

The intended V1 command family is:

```text
runtime list|show|history                         # discover stable references and lifecycle state
runtime delete --id <runtime-ref> --confirm=delete
runtime prune dry-run                            # discover one exact prune-plan reference
runtime prune apply --plan <prune-plan-ref> --confirm=prune
runtime restore --id <runtime-revision-ref>
```

`runtime delete` closes the lifecycle of the complete managed Runtime. Prune
removes only reconstructible, currently unused local Docker execution material;
it does not delete Runtime authority, editable source, immutable source
snapshots, or revision history. `runtime restore` recreates execution material
from one retained immutable snapshot and accepts it only if the inspected image
content digest exactly matches the recorded successful revision evidence.

## User outcome and lifecycle boundary

- **Owner:** one Tobari installation owns managed Runtime authority, private
  source/snapshots, exact Tobari image tags, lifecycle journals, and receipts.
  Docker owns the content store and shared layers; Tobari never claims exclusive
  ownership merely from a tag or label.
- **Scope:** a Runtime is installation-wide and reusable across Workspace
  Manifests. A Runtime revision is a subordinate immutable semantic
  value, not an independently mutable resource. A prune plan is an ephemeral
  review token, not durable domain authority.
- **Lifetime:** editable source, immutable revisions, and snapshots exist until
  the complete managed Runtime is retired. Docker image material is a
  replaceable availability facet and may be pruned only while unreferenced and
  unused. A retirement receipt outlives deletion long enough to make the exact
  action idempotent.
- **Mutability:** source is editable while the Runtime is active; revision
  identity, ordinal, snapshot, creation time, and recorded image digest are
  immutable; image availability and diagnostic usage/storage observations may
  change; the standard Runtime is not deletable.
- **Authority identity:** stable Runtime ID and semantic source digest remain
  authority. Human `name@ordinal`, `head`, and name are presentation and
  selection metadata. Docker tags, Docker `.Id`, container IDs, filesystem
  paths, byte counts, and last-used timestamps are never target authority.

## Why now

ADR 0067 explicitly deferred garbage collection after making managed Runtimes
installation-wide and revisioned. Current main creates editable source and
append-only immutable revisions but has no public Runtime delete, revision
retirement, prune, restore, or interrupted-cleanup workflow. A zero-revision
Runtime can therefore never be removed through Tobari, every successful build
retains an image and snapshot indefinitely, and predecessor Context deletion
cannot lead to a supported storage-reclamation outcome. In current main that
predecessor operation is named `context delete`; the target V1 vocabulary is
exclusively Workspace Manifest / `manifest` / `workspace_manifest_id`. This is
an open V1 lifecycle rather than an optional optimization.

Current build failure semantics also allow BuildKit cache and a successfully
loaded but rejected candidate image to remain. Current no-change detection uses
semantic source history without first proving the recorded image still exists.
The first public contract should close those states before provisional Docker
names and ad hoc manual cleanup become compatibility expectations.

## Non-goals

- Implementing production code, tests, durable documentation, generated data,
  migrations, release artifacts, or public CLI changes in this packet-authoring
  session.
- Deleting an individual immutable Runtime revision or renumbering revision
  ordinals.
- Automatically deleting an empty Runtime, an unreferenced revision, a failed
  build artifact, a Workspace Manifest, a Project root, a Workspace, a
  Workspace home, the installation root, or shared cluster state.
- Exposing a public generic `gc`, `docker image rm`, builder-cache prune, source
  delete, image delete, raw Docker selector, Docker `.Id`, container ID, or
  arbitrary filesystem deletion utility.
- Reclaiming Docker BuildKit cache. It is daemon-wide and may be shared with
  non-Tobari builds; V1 reports it as excluded rather than claiming ownership.
- Guaranteeing that an externally deleted image can always be reproduced from
  an unpinned or unavailable external build input. Restore is exact-or-fail and
  never rewrites historical evidence.
- Changing Runtime copy semantics, adding lineage, changing Workspace Manifest
  deletion, or deciding how many previous immutable Workspace Manifest revision
  bodies the durable Manifest history contract retains.
- Adding a resident daemon, automatic timer, background collector, remote
  registry, OCI publication path, new credential flow, or new network
  destination.

## Acceptance criteria

- [ ] Current-main negative catalog evidence remains explicit: no public
      `runtime delete`, `runtime revision delete`, `runtime prune`, `runtime
      restore`, or `runtime gc` exists before this implementation.
- [ ] `runtime list`, `runtime show`, and `runtime history` produce stable,
      kind-specific opaque `runtime` and `runtime-revision` references, preserve
      them byte-for-byte, and keep human `name@ordinal` and head status
      presentation-only. No routine input or output uses a Docker tag, image ID,
      container ID, snapshot path, or decoded provider notation as authority.
- [ ] `runtime create` remains a command-bound `tool_local` fixed-target create
      against the Runtime catalog and may produce only the confirmed child
      `runtime` reference. Existing `runtime build` becomes `RoleAct`,
      `EffectWrite`, reference-bound to one required `runtime` target rather
      than declaring the catalog singleton while selecting a Runtime by name.
- [ ] `runtime delete` is `RoleAct`, `EffectWrite`, reference-bound to one
      required `runtime` target through `target_id_input`, and declares impact
      cardinality one, notification no, access change yes, destructive yes. It
      requires `--confirm=delete`; the standard Runtime fails before mutation.
- [ ] Complete Runtime retirement removes only the exact managed Runtime's
      editable source, immutable source snapshots, manifest, exact Tobari-owned
      image tags/material that Docker proves removable, and its completed build
      artifacts. It does not delete or rewrite any Workspace Manifest, Project
      root, Workspace, `workspace_id`, Workspace home, last successful applied
      receipt, pending-adoption state, last bounded failure, credential state,
      root index, shared service, or unrelated/shared Docker resource.
- [ ] Runtime deletion fails before its first destructive step if any current
      Workspace Manifest binding, any immutable Manifest revision body retained
      by the ADR 0079/durable retention inventory, any Workspace pending desired adoption, any
      Workspace `last_successful_entry`, or any Tobari-owned Workspace container
      observation requires one of its revisions. The preview distinguishes
      these blocker classes. Unknown, malformed, racing, migration-unverified,
      or incomplete protection evidence fails closed.
- [ ] A zero-revision managed Runtime is deletable with the same exact target,
      confirmation, journaling, and no-cascade rules. The built-in standard
      Runtime is never deletable or prune-eligible.
- [ ] Public `runtime revision delete` is absent. Revision history, ordinals,
      semantic digests, and source snapshots remain immutable and contiguous
      until whole-Runtime retirement; no tombstone or ordinal reuse is added.
- [ ] `runtime prune dry-run` is `RoleDiscover`, `EffectRead`, performs no state
      creation or cleanup, completely reports candidate and protected image
      material, and produces one deterministic opaque `runtime-prune-plan`
      reference. `runtime prune apply` is a distinct `RoleAct`, `EffectWrite`
      command, consumes exactly that required reference as `target_id_input`,
      declares many/no/yes/yes impact, and requires `--confirm=prune`.
- [ ] Prune removes only exact managed Runtime image material that is absent
      from every current Workspace Manifest binding, every retained immutable
      Manifest revision body, every Workspace pending desired adoption and
      `last_successful_entry`, and every owned Workspace container observation
      at apply time. Source, snapshots, revision authority, and head metadata remain.
      A head revision receives no authority exemption: it is protected by use,
      not by display position, and becomes explicitly unavailable if pruned.
- [ ] Workspace Manifest deletion never cascades into Runtime mutation. A
      revision can appear as newly unreferenced only in a later explicit
      dry-run after the authoritative Manifest-retention inventory no longer
      retains its binding and no Workspace applied/pending/observed protection
      remains. Neither reads nor Manifest mutations perform cleanup.
- [ ] `runtime restore` is `RoleAct`, `EffectWrite`, reference-bound to one
      required `runtime-revision` target, and declares one/no/yes/no impact. It
      rebuilds only the retained immutable snapshot, validates the normal
      Runtime compatibility contract, compares the inspected image content
      digest with the immutable recorded digest, and publishes availability
      only on exact equality. Missing or changed external inputs produce a
      safe, non-destructive `runtime_revision_unrestorable` result.
- [ ] A repeated semantic build is `no_change` only after the recorded image is
      observed compatible with the exact recorded content digest. A missing or
      mismatched image returns a stable restore recovery rather than claiming
      success or appending a replacement revision.
- [ ] Successful build staging and every post-build failure artifact have exact
      private Runtime ownership and a journaled disposition. Prune can select a
      confirmed abandoned candidate image but never globally prunes BuildKit
      cache or infers ownership from a name prefix alone.
- [ ] Delete and prune persist an owner-only, schema-validated journal before
      the first destructive Docker or filesystem step. Repeating the exact
      target after cancellation, process death, output failure, or partial work
      resumes or returns the confirmed receipt without broadening the target.
      Reads reconcile but do not repair; unclassified outcomes are
      non-retryable until a read-only recovery command has run.
- [ ] Mutation success crosses the mutation-complete output boundary before
      later cancellation can imply replay permission. Structured errors expose
      exact phase, strongest proved `change_state`, retryability, and catalog
      path recovery. No partial mutation is reported as unchanged or complete.
- [ ] Human output leads with Runtime/revision display identity, protection or
      candidate reason, exact action, and confirmation consequence. JSON schema
      1 has task-owned complete results with explicit absent/unknown states,
      candidate dispositions, logical bytes, non-additive image-byte evidence,
      last-used certainty, preserved-shared outcomes, and zero claims about
      deletion of Workspace Manifests, Workspaces, homes, IDs, applied receipts,
      or Project roots. Protection objects use `workspace_manifest_id` and
      `workspace_id`; no `context`, `context_id`, `project_id`, or `instance_id`
      alias is emitted.
- [ ] Source and snapshot logical disk usage is measured exactly within bounded
      owner-only trees. Docker image size is diagnostic and explicitly
      non-additive; unknown unique/shared/reclaimable bytes remain unknown.
      `last_successful_entry.reconciled_at` may support a clearly named
      last-successfully-applied diagnostic, but is not silently relabeled as
      last-used. Last-used distinguishes exact observed evidence from unknown;
      no historical use is inferred during migration.
- [ ] Runtime name reuse after confirmed deletion creates a fresh stable ID and
      new opaque reference. The old target reference continues to resolve only
      to its idempotent deletion receipt and can never target the new Runtime.
- [ ] Catalog, reference-flow, typed argv, mutation-contract, complete-delivery,
      hostile-output, negative-inference, cancellation, interruption, Docker
      race, filesystem race, and public-boundary tests mechanically enforce the
      lifecycle. Routine supported outcomes require zero external processing
      and at most one exact-command help retrieval plus one local discovery.
- [ ] `task check`, `task security`, `task public:check`, and
      `task release:check` pass on the integrated implementation, including the
      required supported-platform Docker observations and generated-diff
      checks. No gate is weakened.

## Governing documents

- Thesis: `docs/00_theses.md` installation-wide reusable Runtime, exact Runtime
  binding, one controlled side-effect boundary, catalog authority, and
  semantics-before-presentation theses.
- Product contract: `docs/01_product_contract.md` Runtime customization,
  predecessor Context deletion, mutation, output, causal recovery, and Docker
  image rules.
- Architecture: `docs/02_architecture.md` four layers, Runtime/predecessor
  Context/Workspace ports, lifecycle lock, Docker runner boundary, and strict
  state ownership.
- Security: `docs/03_security_model.md` owner-only state, Docker trust boundary,
  no force/global cleanup, external text, and secret-free results.
- Harness: `docs/04_harness.md` catalog-derived contracts, mutation result
  classification, zero-write reads, Runtime fixtures, generated ledgers, and
  completion gates.
- Authentication and external API: `docs/07_authentication.md` through
  `docs/09_agent_readiness_validation.md` remain unchanged in authority; their
  Workspace-owned state and recovery scenarios are protected negative targets.
- Existing decisions: ADRs 0067, 0069, 0070, 0071, 0077, and 0078.

## Completion definition

The future change is complete only after the Runtime lifecycle decision is
promoted to an ADR and all affected theses/product/architecture/security/
harness contracts agree; implementation and migration slices pass the required
gates; supported-platform interruption and Docker-sharing evidence is recorded;
all acceptance criteria have evidence; and this temporary packet is removed.
The implementation owner then commits and notifies root with
`WP03_IMPLEMENTATION_COMPLETE`, or reports `WP03_IMPLEMENTATION_BLOCKED` when
an integration or gate fact prevents completion. The notification includes the
final public/internal interfaces, every gate result, exact HEAD and explained
status, packet retention/removal, and explicit WP04 start readiness. This
Accepted/fixed packet remains Planned and is not implementation completion.
