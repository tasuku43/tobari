# ADR 0028: Store Context policy source by exact domain generation

- Status: Accepted
- Date: 2026-08-12
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, policy learning, runtime, and harness
- Supersedes: The single-file Context source-promotion mechanism in ADR 0024
- Superseded by: None

## Context

One Context previously stored boundary, credential-binding, baseline-Deny,
learned-Allow, and learned exact-Deny data in one `policy/data.json`. That shape
hid the primary authorization boundary: the exact destination host. It also
made future trusted-host review and exact-host compaction unnecessarily broad.

The replacement source must remain directly editable at stable host paths,
keep each host's method authority isolated, preserve deny precedence, and
support routine CLI mutation without allowing a crash, concurrent process, or
lockless reader to accept a mixed multi-file state. No compatibility or
migration contract exists before first publication.

## Decision

Each Context stores source at
`policy/domains/<canonical-host>/allow.json` and `deny.json`. `allow.json` owns
the host's authorities, HTTP methods, GraphQL endpoints, credential profile
host bindings, and learned Allows. `deny.json` owns baseline Denies and learned
exact Denies. Provider identity may remain metadata but never selects physical
placement.

Both documents use strict schema 1 with explicit arrays. The directory name
and every authority, endpoint, credential binding, and rule host must match the
same canonical lower-case DNS host. Unknown fields, duplicate JSON keys or rule
IDs, missing fields, incomplete pairs, extra entries, symlinks, unsafe modes,
uppercase or trailing-dot forms, IP literals, and wildcards fail closed.
Methods are composed into only that domain's authority records. Deny retains
precedence over every allow form.

Routine mutation treats the complete `domains/` tree as one source generation.
Under an in-process mutex and installation-wide file lock, Tobari:

1. rereads and compares the expected source snapshot;
2. writes, validates, and fsyncs a complete private sibling generation;
3. records a strict durable journal containing original and candidate digests;
4. renames the live generation to a validated recovery name;
5. renames the complete candidate generation into the stable `domains/` path;
6. builds, tests, and activates one immutable content-addressed aggregate;
7. durably records the candidate aggregate revision; and
8. removes the recovery generation and journal only after confirmed state.

A lockless reader sees the old complete generation, a missing generation or
live journal that fails closed, or the new complete generation. It never sees
one new domain file paired with one old file. Recovery accepts the candidate
only when its exact aggregate revision is already durable; otherwise it
restores the digest-matched original. A direct external edit that makes either
generation differ from the journal is ambiguous and remains fail closed rather
than being overwritten.

Direct trusted-host editing keeps the stable paths. One existing file should be
saved by atomic rename. A manually added host should be prepared as a complete
private directory and renamed into `domains/`. Any incomplete or changing
snapshot is rejected before projection. CLI mutation preserves every unchanged
source file byte-for-byte.

Context source never contains `data.json`. Composition may generate one
`data.json` inside a private preflight or immutable content-addressed OPA
projection; that file is execution input and is not user-edited source.

## Alternatives considered

- **Sequential atomic file renames.** Rejected because another process can
  observe old/new `allow.json` and `deny.json` combinations.
- **Per-domain directory rename.** Rejected as the transaction boundary because
  one reviewed mutation may change several exact domains and expose a mixed
  Context generation.
- **Immutable revision directories plus a `current` pointer.** This gives a
  simple activation pointer but makes the human-editable stable paths indirect
  and turns revision retention/garbage collection into source semantics.
- **Write-ahead patch journal and in-place recovery.** Rejected because readers
  would need to replay or understand partial file operations, expanding the
  trusted source parser and compatibility surface.
- **Complete generation rename with a recovery journal.** Selected because it
  retains conventional editable paths, gives readers a small fail-closed
  condition, and aligns source recovery with immutable projection activation.

## Consequences

- A domain always appears with both strict documents during CLI creation.
- Cross-process mutation is serialized; direct edits race by rejection, not
  merge or overwrite.
- Directory rename has a brief no-live-generation interval, which is explicitly
  fail closed rather than an availability guarantee.
- Wildcard policy and automatic/manual compaction formats require a future
  schema and review decision; schema 1 is exact-host only.
- Development Contexts using the retired flat source must be removed and
  recreated. There is no reader, fallback, migration, or compatibility layer.

## Mechanical enforcement

- Strict parser tests cover unknown/duplicate/missing fields, host mismatch,
  canonicalization, exact-only hosts, rule identity, unsafe files, modes, and
  extra entries.
- Rego tests prove per-domain method isolation and deny precedence.
- Transaction tests cover new-domain pairs, unchanged bytes, rollback,
  durable-revision recovery, external editing, same-process serialization, and
  separate-runtime lock contention.
- Guided and Advanced preflight tests enforce their exact source layouts.
- Projection tests require deterministic composed bytes and immutable atomic
  activation.

## Validation

- `task check`
- `task security`
- `task policy:test`
- `task integration:test`
- `task public:check`
