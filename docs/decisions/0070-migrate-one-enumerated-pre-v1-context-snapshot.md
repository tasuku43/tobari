# ADR 0070: Migrate one enumerated pre-V1 Context snapshot explicitly

- Status: Superseded
- Date: 2026-08-20
- Deciders: Tobari maintainers
- Scope: Product, CLI, architecture, security, state, Runtime, Context, harness,
  and public boundary
- Revises: ADR 0027, ADR 0066, and ADR 0067
- Related: ADR 0069
- Revised by: ADR 0084 retires the capability before first publication
- Superseded by: ADR 0084, which restores the pre-release clean break and
  removes this predecessor from public migration scope

## Context

ADR 0027 correctly removed transparent compatibility readers and generic
migration machinery before the first public release. Later pre-public changes
made Context policy directly owned and made Runtime revisions reusable across
Contexts. A real development installation now retains useful Context IDs,
Workspace homes, learned exact rules, and one compatible custom Runtime, but
ordinary readers can report only that the current Context is invalid.

Deleting the installation is mechanically simple but needlessly couples a
non-secret authority-shape change to deletion of Workspace-owned tool state.
Manually editing the manifest, policy revision, and Runtime binding is unsafe
and bypasses the product's validation boundaries.

## Decision

Keep every current V1 reader strict. Add one explicit fixed-target mutation,
`migrate apply`, which alone may decode one enumerated unpublished
predecessor: schema-V1 Context manifests with `policy_preset_origin`,
`policy_preset_revision`, an exact owner-only `policy/preset.json`, and an
optional Context-owned Dockerfile Runtime recipe produced by the superseded
contracts.

The command validates the complete Context collection before its first Context
write. It accepts only the built-in agent-ready predecessor, checks the exact
snapshot digest and strict JSON shape, removes the retired guardrail and
historical native-readiness copies, and normalizes one current Context-owned
policy snapshot. Current native readiness remains an explicit manifest fact
and the installed trusted binary supplies its reviewed overlay.

A predecessor Context using the enumerated development standard selector
`tobari-runtime:dev` receives the exact current built-in standard Runtime
binding. A Context-owned Dockerfile is promoted to a
deterministically named managed Runtime through the existing bounded source
snapshot, Docker build, image compatibility, and immutable revision boundary.
An existing exact promoted Runtime is reused; a same-name mismatch fails.

Before committing Contexts, the command creates an owner-only content-addressed
backup of every authority file it will replace. It writes the normalized policy
atomically before atomically replacing that Context's manifest, then removes
the legacy preset from the active policy directory. The private backup retains
the exact preset; the Context-owned recipe may remain as ignored local evidence,
never as fallback authority. A current Context with that exact residual preset
is a cleanup-only restart point, and other current Contexts are no-ops, so
interruption between Contexts is restartable. Context IDs, active selection, project/instance bindings,
Workspace homes, learned domain rules, credential stores, and running
containers are not rewritten.

The migration report is secret-free and complete. `doctor` recognizes this
exact predecessor without decoding it as current authority and returns
`migrate apply` as the recovery. Unknown fields, versions, preset origins,
unsafe paths, mismatched revisions, custom preset shapes, source drift,
Runtime conflicts, and incompatible images fail closed.

This is not a general compatibility framework. A later migration requires a
new versioned decision and a new enumerated source contract.

## Consequences

- Developers retain useful local Workspace state without weakening current
  readers or adding implicit authority reinterpretation.
- The migration capability has a deliberately finite lifetime and source
  schema, but remains public while the supported predecessor may exist.
- A custom Runtime promotion may perform the same bounded Docker build and
  reviewed downloads declared by its Dockerfile; no new network adapter is
  introduced.
- Backups consume local owner-only storage and require an explicit later
  cleanup capability if automatic retention is ever desired.

## Mechanical enforcement

- Catalog validation fixes the command's exact `migrate apply` path, effect,
  role, fixed target, complete output, many-target access-changing/destructive
  mutation impact, and faults.
- Strict fixtures cover current, predecessor, mixed/restarted, custom Runtime,
  malformed, duplicate-key, unsafe-mode, symlink, digest-drift, conflict,
  cancellation, backup-failure, and write-failure cases.
- Tests prove Context IDs, active selection, root/instance records, Workspace
  home bytes, learned rules, and running Docker resources are not mutation
  targets.
- Repository checks continue rejecting migration logic outside this named
  capability and historical decision text.

## Compatibility and migration

This record is the complete compatibility decision. The command accepts only
the enumerated predecessor and current V1 state. It does not accept an unknown
schema, infer missing identity, migrate credentials, or become a reader
fallback. After successful migration it returns a valid no-change result.

## Security and public-boundary impact

The trusted input surface grows by one bounded, owner-only, non-secret legacy
shape. The side-effect surface remains local filesystem mutation plus the
existing managed Runtime build boundary. No provider credential, Workspace
home content, external API interpretation, arbitrary executable selector,
remote source, or additional network authority is introduced.
