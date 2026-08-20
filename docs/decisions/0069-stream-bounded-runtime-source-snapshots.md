# ADR 0069: Stream bounded Runtime source snapshots

- Status: Accepted
- Date: 2026-08-20
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, runtime, CLI, harness, and public boundary
- Revises: ADR 0067
- Related: None
- Revised by: None
- Superseded by: None

## Context

ADR 0067 made one managed Runtime source directory the complete Docker build
context and required bounded regular files and directories. The first
implementation accepted 2 MiB per file and 16 MiB total and retained every
file in memory before hashing and writing a snapshot.

A normal custom Runtime may need a private release binary acquired through an
authenticated host CLI and then copied into the image. A synthetic equivalent
of the observed 10 MB input exceeds the first per-file limit even though the
complete context remains small. Adding a binary-only channel would create a
second acquisition, identity, permission, snapshot, and trust contract.

The adapter already detects the size and owner-only violation before Docker,
but the application converts its unstructured error to `runtime_build_failed`.
The CLI therefore omits the offending path, actual value, limit, and required
permission correction.

## Decision

Keep one uniform regular-file source contract. A managed Runtime source accepts
at most 1,024 regular files and 256 directories, 32 MiB per regular file, and
64 MiB across all regular files. The source root and every child have no group
or other permission bits. Links, special files, escaping paths, and a missing
root `Dockerfile` remain invalid. Owner execute bits remain valid so copied
scripts and binaries may preserve `0700` rather than being forced to `0600`.

Snapshot creation inventories bounded metadata, then streams each safely opened
file into one private temporary snapshot while feeding the exact copied bytes
to the semantic digest. It does not retain a file or the complete source as a
`[]byte`. Canonical path, mode, size, and byte ordering remain unchanged, and
the source path and opened file are revalidated around the copy. Docker still
receives only the completed immutable snapshot.

Every source-contract rejection before Docker is the stable non-retryable
`runtime_source_invalid` fault. Its reviewed public message identifies a
bounded, quoted relative source path and the concrete invalid fact: observed
and allowed sizes/counts, observed and required permission class, entry kind,
missing Dockerfile, or concurrent source change. It never exposes an absolute
host path or arbitrary private cause. `runtime_build_failed` remains for
unclassified snapshot/storage and Docker build failures.

Exact human and agent help state the complete numeric and owner-only boundary.

## Consequences

- Authenticated host-acquired binaries use the same Docker context, digest,
  snapshot, history, and review path as scripts and configuration.
- The 64 MiB source ceiling bounds disk input and work, but no longer implies a
  matching whole-source heap allocation; copying uses one fixed buffer.
- Increasing a future source limit does not automatically increase peak heap,
  but remains a public resource-contract decision with tests and documentation.
- Users can correct ordinary copy-mode and size failures without reading Tobari
  source or rerunning Docker directly.
- Source remains trusted-host build input, not a credential transport. Users
  remain responsible for the licensing and secrecy of files they place there.

## Mechanical enforcement

- Infrastructure constants and boundary tests fix all four numeric limits.
- A synthetic 10 MiB owner-executable regular file reaches the fake Docker
  runner and its complete bytes exist in the immutable snapshot.
- Per-file, total, count, entry-kind, and file/directory owner-only tests reject
  before Docker and append no history in proportion to each boundary; semantic
  no-op tests bind the streamed digest to the stored snapshot revision.
- Application tests preserve a valid source fault while stripping its private
  cause; CLI text and JSON tests expose the same code and reviewed message.
- Catalog/help tests require every numeric limit, the group/other permission
  rule, and `runtime_source_invalid` in exact command help.
- The full repository gate owns source, snapshot, history, catalog, help, and
  public documentation agreement.

## Compatibility and migration

Existing managed source, manifests, semantic digests, revision snapshots, and
Context bindings remain compatible. Inputs between the old and new size limits
become valid. No stored schema changes and no migration runs.

## Security and public-boundary impact

The maximum trusted-host build input increases to 64 MiB, bounded independently
by file and count limits. Streaming avoids turning that ceiling into retained
heap. No source is mounted into a Workspace, no credential or remote fetch
channel is added, and no host path, source byte, Docker output, or private cause
enters the stable fault. Synthetic fixtures contain no private asset.
