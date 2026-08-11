# Work Context: Keep declared reads observational on first use

## Verified behavior and implementation

- The public catalog has 16 `EffectRead` paths. A dynamic catalog test executes
  every path against its own fresh XDG tree, rejects parse-only coverage,
  compares the complete owned tree before/after, and records zero Docker
  mutation argv.
- Context observation now returns `persisted`, `synthetic_default`, or
  `legacy_unmigrated`. Synthetic state carries no ID, stores, or mutation
  authority; explicitly naming a missing default returns not found.
- Context list schema is 4, Context report schema is 8, auth result/status
  schema is 3, and Workspace status schema is 4.
- Safe schema-1/2 legacy Context reads do not migrate. The first authorized
  mutation revalidates and migrates atomically. Schema-3/4 Contexts with valid
  IDs project persisted authority in memory without rewriting their document.
- Unsafe and corrupt stored state fails closed and is not normalized into a
  synthetic default.
- Fresh and ordinary lifecycle reads create no project lock. Observation uses
  an existing lock when present; bounded cleanup of a pre-existing validated
  journal may create the recovery lock solely to serialize that cleanup and
  never creates the journal itself.
- Read-only XDG and 24-reader concurrency fixtures are deterministic and make
  no durable changes.

## Verification evidence

- `go test ./internal/...`: PASS.
- Focused domain/application/infrastructure/CLI tests: PASS.
- `task security`: PASS (`repoguard (security): OK`, no vulnerabilities,
  public documentation source verification passed).
- `task check`: hygiene and architecture lint passed, then stopped because the
  English and Japanese architecture-site JSON-schema tables still declare
  Context list 3, Context report 7, auth 2, and Workspace status 3 instead of
  4, 8, 3, and 4.

## Verification-deferred constraint

The user explicitly prohibited changes below `docs/architecture-site`. The
repository gate correctly requires those public tables to match the catalog,
and the gate must not be weakened. Completion is therefore deferred until an
authorized change synchronizes both architecture-site tables and reruns
`task check`. No Pages file was changed by this packet.

During the deferred-packet handoff, `presentation-evidence.md` was accidentally
deleted while removing the temporary packet. Its captured head was restored
verbatim; the final three unchecked security bullets were reconstructed from
the repository packet template and sibling packets. This restoration makes no
new evidence claim, and every presentation item remains Pending/unchecked.

## Security and public-boundary notes

- Tests use private temporary trees and synthetic identities only.
- No credential material is read or created by first-use status.
- The change removes side effects and grants no new read scope.
