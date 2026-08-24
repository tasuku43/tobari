# ADR 0079: Model Workspace Manifests and applied Workspaces

- Status: Accepted
- Date: 2026-08-23
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, CLI, architecture, security, state, migration,
  harness, and public boundary
- Supersedes: ADR 0013, ADR 0029, ADR 0071, and ADR 0078 where their Context
  vocabulary or flat lifecycle conflicts
- Revises: ADR 0070
- Related: ADR 0018, ADR 0062, ADR 0067, and ADR 0069
- Superseded by: None
- Proposed successor: ADR 0084; ADR 0079 remains the current public and state
  contract until that proposal is implemented, verified, and accepted

## Context

Current main stores one flat `ContextManifest` containing an immutable
source/network Boundary, mutable Runtime binding, later-session shell and Git
defaults, and new-Workspace-only bootstrap defaults. A Workspace stores partial
runtime and bootstrap evidence but not the complete desired Manifest revision,
last successful entry application, or latest failed/unknown reconciliation.
Public and internal boundaries also call the same logical instance Workspace,
ProjectInstance, project, and instance.

`context use` already changes only the installation default used by later
invocations that omit an explicit selector. It does not activate a Workspace or
retarget an existing binding, but `active` and `current Context` vocabulary
obscure that behavior.

The product owner selected Workspace Manifest as the public noun for the
reusable type declaration and Workspace as its root-bound applied instance.
The governing evidence and full alternatives are in
`docs/work/domain-model-v1/`.

## Decision drivers

- Desired, last-successful applied, and currently observed facts must remain
  distinct across failures and attachments.
- Workspace entry and `cluster up` are the only mutation-bearing
  reconciliation boundaries; Tobari has no resident controller.
- One user-facing reusable declaration must not be mechanically split when its
  fields share owner and reuse scope but differ only in activation lifetime.
- Runtime source and immutable successful revisions have an independent
  owner/lifetime and remain reusable across Workspace Manifests.
- Stable IDs and semantic digests authorize; names, generations, ordinals,
  roots, images, and containers do not.
- Pre-public V1 should remove Context/project aliases rather than freeze them.

## Considered options

### Preserve Context and improve presentation

This minimizes migration but leaves complete desired/applied/failure identity
implicit and retains competing Workspace identities.

### Split Boundary and Environment into public resources

This maximizes composability but adds selectors, deletion rules, and reference
edges without evidence of independent V1 owners or lifetimes.

### Use immutable definitions and applied instances

This adds typed revision and receipt state while keeping a bounded public
surface. Public naming candidates included Work Mode, Type, Workspace
Definition, Blueprint, and Manifest.

### Fold Runtime into the reusable Workspace declaration

This removes one noun but destroys build-without-rollout, revision reuse, and
the independent Runtime source/history lifecycle.

## Decision

Public V1 has three durable resources in this area: **Workspace Manifest**,
**Runtime**, and **Workspace**. Project root and Runtime revision remain typed
subordinate concepts.

A Workspace Manifest is a stable named, CLI-managed, trusted-host declaration
of a Workspace type. Each accepted semantic mutation publishes one complete
immutable `WorkspaceManifestRevision` with a semantic digest and monotonic
correlation generation. Content authority is always the pair
`(WorkspaceManifestID, semantic digest)`. Generation is correlation and history
position only: it never authorizes content, and a generation difference alone
does not prove a semantic difference. A semantic no-op publishes nothing and
does not increment generation. An A→B→A sequence records a new generation for
the later A while retaining the same A digest. Storage may therefore key a
retained receipt by `generation + digest`, but actions and validation must not
promote that receipt key to revision authority. Every revision under one
WorkspaceManifestID retains the same immutable Boundary fingerprint. Boundary
change creates another Manifest.

A Workspace permanently binds one WorkspaceManifestID and one canonical
ProjectRoot. It records create-once defaults, the last successful entry
application, and at most one bounded latest failed/unknown reconciliation.
Desired Manifest revision, AppliedEntry, and observed runtime facts remain
separate.

Manifest fields activate by typed contract: Boundary is invariant, cluster
projection applies only at `cluster up`, entry fields apply only at explicit
Workspace entry, session defaults resolve for each new child session, and
creation defaults snapshot only for a new Workspace home. Reads never
reconcile.

The installation separately owns one optional `DefaultManifestSelection`
containing exact WorkspaceManifestID. `manifest default set --name NAME` is its
only ordinary writer. `--manifest NAME` is an invocation-local explicit
selector; omission uses the default. Neither operation retargets an existing
Workspace or mutates Manifest revision, AppliedEntry, Docker, principal, or
cluster state. Root entry, status, and delete select the exact
`(ProjectRoot, WorkspaceManifestID)` pair after this resolution.

`Manifest` does not expose an arbitrary project-owned YAML/JSON contract,
repository discovery, generic import, or `apply -f`. Such a surface requires a
separate trust-boundary decision.

### One-time copy initialization

`manifest create --copy-from NAME --name NAME` initializes one complete draft
from the source Manifest's exact immutable current revision. Review binds the
source WorkspaceManifestID, semantic digest, and canonical body. Publication
revalidates all three under the Manifest lifecycle lock, then issues a fresh
WorkspaceManifestID at generation 1. Source name reuse, revision advancement,
or body drift fails before publication. The result has no persisted or emitted
parent, provenance, lineage, or continuing relationship.

`runtime create --copy-source-from standard|NAME --name NAME` instead copies a
Runtime's current editable source, not an immutable Runtime revision. It issues
a fresh RuntimeID with empty history. Manifest and Runtime copy remain separate
domain/application operations because their source authority and mutation
semantics differ. Neither copy reads or copies Workspace identity or home,
ProjectRoot, authentication, learned permissions, Attachment, AppliedEntry,
failure, observation, or default selection, and neither reconciles Docker.
The former `--base` spelling has no alias.

### Reconciliation and Runtime protection seams

Only explicit Workspace entry and `cluster up` reconcile. Entry resolves one
exact desired Manifest digest and RuntimeID+semantic Runtime revision, observes
the bound Workspace, blocks unsafe attached adoption before Docker mutation,
and writes AppliedEntry only after runtime, guards, endpoints, health, and
principal state are verified. Failure preserves the prior AppliedEntry and
records one bounded attempted digest, phase, change-state, and recovery fact.
`status`, `list`, `show`, `doctor`, and completion only observe.

Runtime lifecycle consumers receive typed read ports over current and retained
Manifest revisions, last-successful AppliedEntry, pending adoption, and
observed Workspace/container references. Those are authoritative protection
graph inputs; `AppliedEntry.reconciled_at` is not Runtime `last_used` evidence.
Runtime delete/prune/restore and revised build selection remain separate work.

## Consequences

### Positive

- Routine status can show Current and Next entry without inferring authority
  from an image, generation, label, or timestamp.
- Failed or unknown adoption preserves the last successful AppliedEntry and
  has an exact read-only recovery barrier.
- Attached Workspaces remain unchanged until a safe explicit entry.
- Default selection says what it does and no longer implies active runtime
  state.
- Runtime retains independent source, build, and reuse semantics.

### Negative

- Context command, schema, persistence, principal, policy, completion, help,
  and fixture vocabulary changes before V1.
- Workspace state gains bounded desired/applied/failure correlation.
- An exact migration and old/new binary cutover are required for development
  state worth preserving.
- Manifest naming creates a continuing obligation not to imply an undeclared
  file-import boundary.

### Risks and mitigations

- A premature AppliedEntry could authorize unsafe replay. Record it only after
  owned mutation, verification, principal publication, and durable success.
- Old and new principal schemas could be interpreted concurrently. Require
  cluster stop/no attachments and one atomic migration cutover.
- A changed default could be mistaken for Workspace retargeting. Output and
  tests state that existing Workspaces remain unchanged and preserve the exact
  explicit/default request dimension.
- Slice revisions could become presentation-only inference. Domain types and
  fixtures derive every status from desired, applied, and observed facts.

## Mechanical enforcement

- Domain types validate WorkspaceManifestID, immutable revisions, Boundary
  fingerprint, slice digests, DefaultManifestSelection, Workspace uniqueness,
  AppliedEntry, and bounded attempt state.
- Catalog contracts expose `manifest`, `--manifest`, and `manifest default set`
  with no Context alias or `manifest use` path.
- Negative tests prove default set, explicit selection, desired mutation, and
  all reads perform only their declared effects.
- Migration fixtures accept only enumerated current-main state and reject
  dangling, duplicate, name-only, unsafe, or inconsistent default markers.
- Principal/security tests bind WorkspaceManifestID and WorkspaceID and reject
  stale Context/project fallback.
- Presentation-independent fixtures cover absent, current, pending,
  attached-blocked, failed, unknown, drifted, unavailable, and incomplete
  states.
- Repository and public guards reject retired vocabulary and undeclared
  manifest-file authority outside historical evidence.

## Compatibility and migration

Final V1 exposes no Context aliases or dual readers. Extend `migrate apply`
with one exact current-main predecessor after the existing ADR 0070 source.
Retain Context UUID bytes as WorkspaceManifestID, ProjectInstance UUID bytes as
WorkspaceID, Runtime history, Workspace homes, learned rules, and bootstrap
receipt. Convert the active Context marker into exact
DefaultManifestSelection.

Create generation 1 Manifest revisions and synthesize AppliedEntry only when
stored state plus read-only owned-Docker observation prove the exact desired
spec. Otherwise retain an explicit unverified/pending state. Migration never
reads standard Workspace-native credentials and preserves Workspace homes byte
for byte. It requires reviewed cluster-stop/no-attachment preconditions, an
owner-only content-addressed backup, atomic publication, and an explicit
`cluster up` before Workspace entry. Retained-revision collision recovery may
reuse only an exact same-path receipt with the same WorkspaceManifestID and
canonical body; another ID, another body, or a partial artifact fails closed.
Ordinary readers accept only final V1.

The predecessor experimental Broker is not Manifest desired/applied state and
is not rebound by preserved UUID bytes. Migration enumerates its complete
filesystem-side replay-capable authority: ciphertext plus every provider,
binding, handle, lookup, projection, Runtime/project registry, and related
configuration record. Unknown, mixed, partial, corrupt, unsafe-mode,
symlinked, or drifted sets fail before mutation. Under one owner-only journal,
the exact set moves atomically to a private quarantine unreachable by ordinary
old or new readers; output is secret- and path-free and reports at most
`research_auth_disposition: reauthentication_required`. Automatic decrypt,
import, rebind, standard fallback, and cleanup are forbidden.

On macOS the unchanged Keychain root-key item is recovery material, not
authorizing state once all filesystem ciphertext and bindings are unreachable;
migration does not read, write, rename, or delete it. On Linux the filesystem
root key moves with the quarantined set. Rollback restores the byte-identical
predecessor set and refuses to merge with fresh canonical auth state. Crash
ordering exposes either the complete predecessor authority before the central
state move or zero resolvable predecessor authority afterwards, never a
partially resolvable set.

## Security and public-boundary impact

Workspace Manifest revisions, DefaultManifestSelection, AppliedEntry, and
bounded reconciliation attempts are trusted host-owned state. Workspace
contents, project files, image contents, Docker observations, and external text
remain untrusted until exact validation. Manifest names, default labels,
generations, ordinals, roots, image selectors, tags, and container IDs never
select policy authority.

The decision adds no credential source, external destination, provider API,
dependency, or resident process. Standard credentials remain Workspace-home
owned; authentication build profile remains compile-time/system-owned; Host
Loopback remains attachment-owned.

## Validation

- `task check`
- `task security`
- `task public:check`
- relevant Linux and macOS/Colima reconciliation and exact migration profiles
- the agent-readiness journey for default selection, explicit override,
  Current/Next entry, attachment blocking, failure observation, and retry

## Reconsideration signals

- Repeated independent owners or reuse lifetimes for Boundary and Environment
  justify public resource separation.
- A reviewed need for repository-authored declarative configuration justifies
  a separate Manifest file/trust-boundary ADR.
- Public V1 publication retires this unpublished migration path through an
  explicit capability-retirement decision.
