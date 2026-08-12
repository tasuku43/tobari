# ADR 0024: Hot-activate reviewed policy decision sets

- Status: Accepted
- Date: 2026-08-09
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, policy learning, runtime, and harness
- Supersedes: The per-decision OPA recreation mechanism in ADR 0018 after the
  activation transport is accepted
- Superseded by: ADR 0028 for Context policy-source promotion only

## Context

Interactive `policy review` currently closes one exact decision at a time.
Every choice tests policy, rebuilds the complete all-Context projection,
recreates OPA, waits for health, and then rereads the queue. This preserves a
strong activation boundary but makes several ordinary exact permissions pay
the same container lifecycle latency repeatedly.

OPA supports validated bundle replacement while serving. A failed bundle does
not replace the prior active bundle. The product can therefore preserve its
all-or-nothing policy semantics without making container identity the unit of
activation, provided Tobari confirms the exact active revision and fences stale
authority during reductions.

The public mutation contract also currently binds each policy action to one
opaque candidate. Arbitrary multi-reference acts remain unsafe. A human review
session, however, can bind a complete typed snapshot, retain each opaque ID
unchanged, collect explicit per-candidate choices, and apply the resulting set
as one mutation of the command-owned installation policy decision set. The
first slice bounds one Apply to one Context so durable source promotion remains
one transaction. ADR 0028 replaces this ADR's former single-file promotion
mechanism with a journaled complete domain-generation transaction.

## Decision

Routine policy activation uses a watched complete bundle in one exact
owner-labeled Docker-managed volume. A fixed, networkless, capability-dropped
builder invocation of the pinned OPA image writes a revision-named candidate
archive. A second fixed, networkless, capability-dropped invocation of the
already-pinned Debian image atomically renames that archive to the watched path
inside the same volume. The shared OPA mounts the volume read-only and runs
with `--watch --bundle`. This transport creates the file event inside the
Docker host and does not rely on host bind-mount notification. No host port,
Workspace network, egress path, credential mount, or additional resident
service is introduced.

A bounded macOS/Colima experiment with the pinned OPA 1.17.0 image loaded
revision A, replaced the volume bundle with revision B, and observed exact
revision B from the same OPA container. Linux CI retains independent integration
coverage before the mechanism is complete.

The terminal `policy review` workflow may stage several exact Allow or Deny
choices. Staging is not authority and writes no policy source. Apply is one
command-bound fixed-target mutation against the installation decision set. It
revalidates the complete snapshot and every unchanged opaque candidate ID,
rejects a set spanning multiple Contexts,
tests the affected Context sources and complete all-Context candidate, promotes
the source update under the projection lock, and activates one complete bundle.
Cancellation discards the set and performs no mutation.

Redirected and machine-readable `policy review` remain read-only. Existing
`policy candidates`, `policy allow --id`, and `policy deny --id` retain the
single-reference discover/act workflow.

Every aggregate bundle carries a host-derived revision in protected aggregate
data. A mutation is confirmed only after the running OPA reports that exact
revision. An increasing change may transition directly from the prior narrower
authority. A reducing or mixed change first installs a complete deny-all
transition revision. Until either the candidate or the previous known-good
revision is confirmed, the cluster remains fenced. A failed candidate never
falls through to partially loaded or stale broader authority.

## Consequences

- Several explicit human decisions pay one full aggregate validation and
  activation cost.
- Routine exact mutation preserves the running OPA container identity.
- OPA health is necessary but no longer sufficient evidence of mutation
  completion; exact active revision is mandatory.
- A reduction may briefly deny unrelated traffic, matching the existing
  fail-closed activation behavior without restarting OPA.
- The batch mutation target is the one installation decision set. Candidate
  IDs are validated typed contents of the human snapshot, not independent
  public target inputs or positional display authority.
- One Apply may contain several projects and effects but exactly one Context.
  The operator applies or discards before switching Context; this keeps the
  durable source change within one domain-generation transaction.
- Machine and automation workflows remain one-reference actions.
- A process interruption cannot expose a partially promoted reviewed set:
  ADR 0028's journaled complete-generation promotion fails closed or recovers,
  and runtime authority changes only through a complete bundle revision.
  Mutation output remains subject to the existing uncertain-outcome
  reconciliation contract after interruption.

## Mechanical enforcement

- Catalog tests keep machine actions single-reference-bound and declare the
  human Apply workflow as one complete fixed target.
- Selector tests prove every staged decision originated on its exact detail
  screen and that list positions cannot create authority.
- Application tests prove fresh snapshot resolution, one activation, zero-call
  discard, and bounded set cardinality.
- Runtime tests reject OPA recreation in routine policy mutation, require exact
  revision confirmation, and fix fence/candidate/rollback ordering.
- Integration tests keep the OPA container ID stable, retain the old revision
  after invalid publication, deny traffic during a reducing transition, and
  reject cross-Context staging before a source write.

## Validation

- `task check`
- `task security`
- `task policy:test`
- `task gateway:test`
- `task integration:test`
