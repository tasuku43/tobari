# Work Context: Create a Runtime source from an explicit Base

This file records verified facts and unresolved questions. It does not make a
desired design current reality.

## Current behavior

- `runtime create --name NAME` is one fixed-target `EffectCreate` over the
  installation Runtime catalog. `--name` is currently required and creation
  never prompts (`internal/cli/runtime_catalog.go`,
  `internal/cli/runtime_library.go`).
- Application owns the canonical create intent and invokes the narrow
  `CreateRuntime(context.Context, string)` infrastructure port. It validates a
  confirmed new Runtime with the requested name
  (`internal/app/runtimecmd/service.go`).
- Infrastructure creates a new Runtime ID, empty revision history, owner-only
  `source/` and `revisions/` directories, and a single `0600` Dockerfile from
  `managedRuntimeTemplate`. The template starts from the resolver-selected
  standard Runtime image. It does not build or select anything
  (`internal/infra/dockerruntime/runtime_library.go`).
- Managed editable source accepts at most 1,024 regular files, 256 directories,
  32 MiB per file, and 64 MiB total. It rejects links, special files,
  non-canonical children, and group/other permission bits. The build snapshot
  algorithm detects path, identity, size, and mode drift while streaming with a
  fixed buffer and retains original owner permission bits in its semantic
  digest.
- After copying source into an immutable revision snapshot, the current build
  path changes every snapshot regular file to `0400`. Runtime revision records
  retain the semantic digest and snapshot path, but not a per-file original
  permission inventory. Therefore a `name@ordinal` snapshot cannot faithfully
  reconstruct whether a source file was originally executable. This is the
  verified reason immutable revision snapshots are not Base candidates
  (`freezeRuntimeSnapshot` in
  `internal/infra/dockerruntime/runtime_library.go`).
- `runtime list` already returns `standard` first and managed Runtime summaries
  sorted by name without creating state. Completion already distinguishes all
  Runtime names, managed-only names, and ready revision references
  (`internal/app/completioncmd/service.go`).

## Relevant structure

- Entry point: `cmd/tobari` through Catalog-derived `runtime create` dispatch.
- Domain rule: `RuntimeManifest`, `RuntimeReport`, Runtime catalog fixed target,
  stable Runtime identity, and empty-versus-immutable revision history.
- Application use case: `internal/app/runtimecmd.Service.Create` and its
  consumer-owned Runtime port.
- Infrastructure boundary: `CreateRuntime`, current managed `source/` trees,
  `snapshotRuntimeSource`, and owner-only atomic Runtime store operations in
  `internal/infra/dockerruntime/runtime_library.go`.
- CLI catalog or presentation: `runtimeCreateSpec`, `runRuntimeCreate`,
  Catalog-derived completion, and terminal selection primitives already used
  by Runtime Build Review.
- Existing tests and harness checks: Runtime domain/application/store tests,
  Catalog and completion contracts, source bound/mode/drift tests, Runtime
  presentation tests, contractlint, agent-readiness, and the shared Runtime
  claim row in `docs/04_harness.md`.

## Constraints

- Base denotes source, not a built Runtime revision. `standard` denotes the
  built-in starter source generated from Tobari's current resolver-selected
  base; a managed name denotes that Runtime's current editable `source/` tree.
- The source copy must reuse or extract the same bounded traversal and drift
  checks as `runtime build`; a second weaker recursive copy is unacceptable.
- The target must be staged privately and published atomically with a fresh ID
  and empty history. Failed validation, drift, cancellation, or collision must
  leave neither a partial Runtime directory nor an authoritative manifest.
- File bytes and owner permission bits, including owner execute, are semantic
  source facts. Directory ownership and safe traversal must remain owner-only.
- A Base selector is ordinary validated text and completion metadata. The
  action remains a command-bound fixed-target create with explicit empty
  `target_inputs`; `--base` is not an opaque reference, `parent_input`, or
  `target_id_input`.
- No source bytes, private binary names, absolute Base paths, or raw private
  filesystem errors should enter stable output or structured faults.
- No new dependency, network destination, Docker action, external process, or
  release artifact is needed for source creation.

## External facts

None. The capability is installation-local and relies only on repository-owned
Runtime source contracts.

## Unknowns

- [ ] Decide whether interactive Base selection should be one standalone
      selector followed immediately by create, or a combined Review screen;
      preserve the accepted standard-first choice and zero-mutation cancel in
      either form.
- [ ] Decide the stable fault code for a Base source that disappears or changes
      during copy: reuse the existing `runtime_not_found` /
      `runtime_source_invalid` contract where precise, or add one narrowly
      classified Base-drift fault.
- [ ] Audit whether the existing source snapshot traversal can be refactored
      into one validated inventory/copy primitive without changing build
      digest or freeze behavior.
- [ ] Confirm the atomic staging directory and rename sequence when Base and
      target are sibling Runtime entries under the same store lock.
- [ ] Confirm whether `InputCompletionRuntimeName` is the exact completion kind
      for `--base`; it currently includes `standard` plus managed names and no
      ordinal revision references.

## Thesis evidence

- Repeated design decision or point of agent confusion: Context and Runtime
  creation both need a higher-level Base decision before individual editing,
  but Runtime Base must be defined in terms of editable source rather than an
  immutable selected artifact.
- User outcome or friction observed in the minimal slice: a nearby Runtime
  source must currently be reconstructed or copied outside the CLI before it
  can enter Tobari's managed build boundary.
- Code workaround or exception being considered: copying a revision snapshot
  looked deterministic, but `0400` freezing destroys the original executable
  mode needed to reconstruct editable source faithfully.
- Current thesis that resolves it, or proposed thesis revision: Runtime source
  is the editable installation-owned definition; build creates an immutable
  revision; Context selection remains a later explicit operation. Base belongs
  only to source creation.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: public grammar/help/completion, source-copy port and infrastructure
  invariant, selector presentation evidence, shared Runtime harness claims,
  docs 00–04, and agent-readiness. No new trust boundary is expected.

## Reproduction or observation

```sh
sed -n '363,548p' internal/infra/dockerruntime/runtime_library.go
go test ./internal/infra/dockerruntime -run 'Test.*Runtime.*(Source|Snapshot|Create)'
```

Observed in source on 2026-08-22: source mode is included in the semantic
digest and used while the temporary snapshot is copied; after the copy,
`freezeRuntimeSnapshot` changes each regular snapshot file to `0400` and does
not persist the former mode separately. The current managed `source/` tree is
therefore the only faithful managed Base.

## Security and public-boundary notes

- Assets and side effects involved: owner-only installation Runtime source and
  creation of one new owner-only standalone Runtime catalog entry.
- Credentials or confidential data involved: source may contain host-acquired
  private binaries by existing contract; bytes remain owner-local and are
  neither rendered nor copied outside the new Runtime source.
- New dependencies, destinations, files, processes, or generated content: one
  new managed source tree only; no dependency, network, Docker, or process.
- External schema provenance, publication rights, and drift evidence: not
  applicable; copying does not publish or redistribute source.
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: complete scalar creation result; no
  pagination; target name makes repeat create non-idempotent; pre-action and
  mid-copy cancellation leave no target; confirmed mutation output rules stay
  unchanged.
- Publication and licensing concerns: public CLI/help/docs only. The source
  may contain user-owned material, but the operation remains a local copy and
  no fixture may contain real private binaries or third-party payloads.

## Glossary

- **Base:** the source initializer selected by Runtime name: `standard` for the
  built-in starter source, or one managed Runtime's current editable source.
- **Editable source:** the owner-only managed `source/` build context whose
  bytes and owner permission bits can still be changed by the trusted host.
- **Revision snapshot:** a build-time immutable copy identified by
  `name@ordinal`; explicitly not a Base because its files are frozen to `0400`.
- **Lineage:** a persisted relationship through which later Base changes affect
  the target; explicitly unsupported.
