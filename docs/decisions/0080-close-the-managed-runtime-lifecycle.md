# ADR 0080: Close the managed Runtime lifecycle

- Status: Accepted
- Date: 2026-08-23
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, Runtime, Workspace Manifest,
  Workspace, catalog, state, Docker, harness, and public boundary
- Revises: ADR 0067
- Related: ADR 0069, ADR 0070, ADR 0071, ADR 0077, ADR 0078, ADR 0079
- Superseded by: None

## Context

ADR 0067 made a managed Runtime installation-wide and revisioned but deferred
garbage collection. Tobari can create editable Runtime source, append immutable
successful revisions, retain exact source snapshots, and build local Docker
execution material. It has no supported way to retire an abandoned Runtime,
review and reclaim unused execution material, or reconstruct execution
material that disappeared outside Tobari.

Manual Docker cleanup cannot close that lifecycle safely. A Runtime revision
may be required by a current or retained immutable Workspace Manifest revision,
a Workspace's last successful applied entry, a pending desired adoption, or an
observed Workspace container. Docker tags, image IDs, container IDs, names,
ordinals, paths, and timestamps do not carry those product relationships.

## Decision

Tobari closes the managed Runtime lifecycle through three separate outcomes:

1. `runtime delete --id <runtime-ref> --confirm=delete` retires one complete
   managed Runtime.
2. `runtime prune dry-run` produces one exact `runtime-prune-plan` reference;
   `runtime prune apply --plan <runtime-prune-plan-ref> --confirm=prune`
   consumes it unchanged and removes only still-unused local image material.
3. `runtime restore --id <runtime-revision-ref>` rebuilds one missing revision
   from its retained immutable snapshot and publishes availability only when
   the inspected content digest equals the recorded successful evidence.

The public reference kinds are `runtime`, `runtime-revision`, and the
ephemeral `runtime-prune-plan`. Runtime authority is stable Runtime ID plus
semantic source digest. Human `name@ordinal`, `head`, Docker tags, image IDs,
container IDs, filesystem paths, byte counts, and timestamps are presentation
or diagnostic evidence only.

Immutable revision history, ordinals, semantic digests, and source snapshots
remain contiguous and immutable until whole-Runtime retirement. There is no
individual revision-delete command. Prune changes only local image
availability; it never deletes Runtime authority, editable source, snapshots,
Workspace Manifests, Workspaces, homes, IDs, applied receipts, pending state,
project roots, credentials, or shared cluster state. The built-in standard
Runtime is never delete- or prune-eligible.

Every retirement decision consumes a complete fail-closed protection
inventory. The inventory distinguishes current Manifest binding, retained
Manifest revision binding, Workspace applied entry, pending desired adoption,
and current observed Workspace use. Malformed, incomplete, racing, or
migration-unverified evidence blocks mutation. Manifest retention remains
owned by ADR 0079; Runtime cleanup cannot shorten it.

`runtime prune dry-run` is a zero-write read. Apply and delete persist an
owner-only schema-validated journal before their first destructive step,
revalidate the complete target and protection snapshot, and resume only the
same exact target. A changed plan fails before mutation. Terminal receipts keep
an old Runtime reference idempotent after same-name recreation with a fresh ID.
An exact successful restore durably supersedes the latest matching prune
availability receipt before restore cleanup can erase its journal. The old
result receipt remains replayable, while the supersession generation changes
the next opaque prune plan and prevents a later external disappearance from
being misreported as a Tobari prune.

Docker operations are bounded to exact validated Tobari evidence. Tobari never
invokes daemon-global image, builder, or system prune, never forces removal,
and never infers ownership from a prefix, tag, or label alone. Foreign tags,
containers, and shared content are preserved and reported. BuildKit cache is
outside this lifecycle.

Runtime discovery produces stable references. Runtime create remains a
command-bound fixed-target create and produces the confirmed child Runtime
reference. Runtime build becomes a reference-bound write through required
`--id <runtime-ref>`. The read-only `review runtimes` flow preserves human
selection; direct mutations do not rediscover a name.

Successful builds use journaled private staging identity. A semantic no-change
is reported only after exact compatible image availability and recorded digest
are observed. Missing or mismatched material selects restore recovery rather
than appending or rewriting history.

Disk evidence stays explicit. Source and snapshot logical bytes are exact only
inside validated owner-only trees. Docker virtual size is non-additive;
unique, shared, and reclaimable bytes remain unknown unless a bounded
authoritative observation proves them. `AppliedEntry.reconciled_at` is
last-successfully-applied evidence, not last-used. V1 adds no usage receipt, so
last-used remains unknown unless directly observed for the current bounded
operation.

## Consequences

- Managed Runtime creation has deliberate retirement and exact recovery before
  the first public V1 contract is frozen.
- Runtime revision remains a subordinate immutable value rather than a fourth
  durable public resource.
- A prune plan adds one ephemeral review reference and prevents effect or role
  from changing behind a `--dry-run` boolean.
- A referenced image may be externally absent without changing Manifest,
  Workspace, or Runtime authority; restore is exact-or-fail.
- Runtime name reuse never reuses stable identity or action authority.
- Unknown protection, ownership, Docker use, migration, or journal state
  blocks destructive work rather than falling back to broad cleanup.

## Mechanical enforcement

- Domain constructors and validators enforce the three reference kinds,
  immutable lifecycle values, complete protection inventory, deterministic
  prune plans, result identity, and terminal receipt invariants.
- Application services resolve references by exact derivation-and-comparison,
  interpret protection completely, and order review, lock, revalidation,
  journal, effect, receipt, and mutation-complete output.
- Infrastructure validates owner-only paths, retained Manifest and Workspace
  evidence, bounded current Docker observations, exact image ownership and
  digest, non-forced removal, monotonic journals, atomic receipts, and
  interruption recovery.
- The Catalog derives nested produced references through its one shared
  bounded `OutputField` traversal. Typed inputs are the consumed-reference
  authority; Runtime adds no walker.
- Contract, hostile-output, cancellation, interruption, race, image-sharing,
  zero-write, migration, generated-ledger, public-boundary, and agent-readiness
  tests enforce the lifecycle.

## Compatibility and migration

No public V1 Runtime-retirement contract has shipped. The provisional
`runtime build --name` action is replaced without an alias by reference-bound
`runtime build --id`. Provisional Runtime JSON may be replaced atomically by
the semantic schema-1 lifecycle projection without retaining Docker selectors,
snapshot paths, or old vocabulary aliases.

That projection names the semantic revision identity `source_digest`, retains
opaque `revision_ref` only for eligible managed revisions, and represents
availability, storage, last-used certainty, and snapshot state separately.
`ready` remains successful-history readiness even when head availability is
`missing`, `mismatched`, `unknown`, or `pruned`; built-in `standard` has null
storage. The public schema has no `revision` alias, Docker `image` or
`image_digest`, or private `snapshot_path`.

ADR 0079's pre-public UUID preservation and Workspace Manifest revision
retention remain unchanged. Migration may synthesize an applied entry only
from sufficient state plus bounded read-only Docker evidence; otherwise
Runtime cleanup remains migration-unverified and fails closed. Migration and
Manifest deletion perform no Runtime cleanup.

## Security and public-boundary impact

The trust boundary gains owner-only lifecycle journals and receipts plus
bounded Docker inspect/build/tag/remove operations. It gains no network
destination, credential flow, daemon, timer, registry, generic garbage
collector, raw Docker selector, arbitrary filesystem target, or public source
or image resource. Raw Docker output remains untrusted infrastructure data.
Routine human and machine output contains semantic Runtime, protection,
availability, disposition, and uncertainty facts but no Docker or container ID
authority and no Workspace-owned secret or home content.
