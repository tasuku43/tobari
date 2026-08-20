# ADR 0067: Share versioned Runtimes across Contexts

- Status: Accepted
- Date: 2026-08-19
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, runtime, Context, onboarding, harness, and public boundary
- Revises: ADR 0013 and ADR 0016
- Related: ADR 0066
- Revised by: ADR 0069 fixes the first public source bounds, streaming snapshot,
  and actionable validation-fault contract
- Superseded by: None

## Context

A Context currently owns one optional Dockerfile recipe and the image promoted
from it. Equivalent environments must therefore be recreated per Context, and
the first-Context journey asks about customization after the Context already
exists. The recipe model also names only a Dockerfile even though real Docker
builds commonly require scripts, package manifests, and configuration files.

Runtime customization and Context isolation have different lifecycles. A user
wants to build an environment once, inspect its history, and deliberately pin
several Contexts to the same successful revision. Editing or building that
environment must not silently change any Context.

## Decision

Runtime is an installation-wide reusable object. A managed Runtime owns a
stable generated ID, unique human name, owner-only mutable source directory,
and ordered immutable successful revisions. Its source is a complete Docker
build context rooted at `source/`; the first version accepts bounded regular
files and directories only and rejects symlinks and special files.

Each build first validates and snapshots the complete source tree. The semantic
SHA-256 digest of canonical paths and bytes is the revision identity. Docker
builds only from that immutable snapshot. A revision is appended atomically
after the image succeeds, passes the Tobari compatibility contract, and yields
an inspected image digest. Failure leaves Runtime history and every Context
unchanged. A digest already in history is a successful no-op rather than a new
ordinal.

Humans see revisions as `<name>@<ordinal>`. Persistent authority is the stable
Runtime ID plus semantic revision digest; the ordinal is presentation and
selection syntax, not independent identity. History is append-only in this
slice. Image selectors and inspected image digests are build evidence and
execution material, not the identity of a Runtime revision.

A Context stores one exact Runtime binding. Context creation defaults to the
built-in immutable standard Runtime and may select only a ready managed
revision. Building a Runtime never changes a Context. Changing or rolling back
a Context is one explicit Context mutation that replaces its binding with a
validated ready revision. Existing Workspaces preserve their home and adopt
the new selection only on their next entry through ordinary reconciliation.

The first-use Context wizard remains four stages: Name, Filesystem, Network,
and Review. Runtime is always a Review row and defaults to standard. Ready
custom revisions can be selected by editing that row. Fresh root onboarding
does not present a post-create Runtime chooser and does not initialize Runtime
source implicitly. A user who needs customization first runs `runtime create`,
edits the shown managed source directory, and runs `runtime build`, then selects
that ready revision during Context creation or a later Context Runtime change.

Fully specified `runtime build --name` and `context runtime set --runtime`
remain deterministic direct actions. On interactive text streams, omission of
the primary selector opens a CLI-owned Review. Build Review selects one managed
Runtime and confirms that no Context changes. Context Runtime Review starts
from the exact current binding, may change an omitted Context selector, offers
the built-in standard revision and every successful managed revision, and
shows that existing Workspaces adopt the choice only on next entry. Only final
Build or Apply mutates; unavailable streams, cancellation, unchanged selection,
and Review failure perform no mutation.

This is a pre-public V1 public-contract replacement. `runtime init` has no
public compatibility alias or implicit migration. The internal reader may
continue to tolerate unpublished Context-owned recipe fixtures during the
transition, but every new public Context stores an exact Runtime binding and no
public command creates or mutates the legacy recipe.

## Consequences

- Environment reuse becomes explicit while Context execution remains pinned
  and reviewable.
- Runtime source can contain Dockerfile-adjacent files without expanding the
  Workspace mount boundary.
- The normal first-use journey is shorter and customization is prepared before
  it can be selected.
- A new build consumes storage but creates no rollout. Garbage collection is a
  later explicit capability.
- Existing unpublished Context manifests are never silently reinterpreted as
  shared Runtimes; users recreate them before relying on the new public model.

## Mechanical enforcement

- Domain validation binds each managed revision to one stable Runtime ID,
  ordinal, semantic digest, image selector, and inspected image digest.
- The Runtime store enforces owner-only ancestors and files, unique names and
  IDs, canonical relative paths, bounded file counts/sizes, no symlinks or
  special files, immutable snapshot directories, and atomic manifests.
- Build tests prove snapshot-before-Docker ordering, fixed BuildKit argv,
  compatibility inspection, no-op semantic rebuilds, failed-build history
  preservation, and no Context writes.
- Context tests prove standard defaulting, ready-revision-only selection,
  exact binding persistence, explicit rollback, and next-entry image adoption
  with home preservation.
- Catalog and onboarding tests prove `runtime init` and the post-create chooser
  are absent, Runtime is visible in Review, and non-interactive inputs remain
  deterministic.
- Harness tests keep routine supported outcomes free of source inspection,
  Docker-tag parsing, or undeclared external processing.

## Compatibility and migration

No public V1 release has shipped the Context-owned recipe capability. The old
shape receives no public mutation or migration workflow and is never promoted
into the new Runtime catalog. Users recreate disposable Contexts and explicitly
create managed Runtimes; temporary internal read tolerance exists only to keep
pre-public fixtures observable during the transition.

## Security and public-boundary impact

The trusted host build boundary moves from a Context directory to one
installation-owned Runtime directory. Runtime source and snapshots are never
mounted into a Workspace. The adapter rejects links, special files, escaping
paths, unsafe ownership/modes, and bounded-resource violations before Docker
I/O. No secret, arbitrary image selector, project path, remote source, or
runtime-selected executable boundary is added.
