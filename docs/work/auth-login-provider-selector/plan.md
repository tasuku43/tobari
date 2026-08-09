# Work Plan: Interactive auth login provider selection

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Replace the existing positional `provider` input with optional `--provider` in the catalog. When it
is absent, the CLI verifies terminal input/error streams, reads the selected
Context's typed auth status, filters the installed collection to the closed
reviewed login-provider set, and uses the existing dependency-free terminal
menu machinery to obtain one explicit provider ID. The chosen ID then enters
the unchanged `authcmd.Service.Login` mutation path.

## Alternatives considered

### Add a separate provider-discovery command

This would add a machine workflow and another public command even though
`auth status` already owns exhaustive provider discovery. The requested human
flow can remain one guided command.

### Let infrastructure prompt internally

That would mix presentation with provider acquisition and make terminal UX an
infrastructure policy. CLI-owned selection preserves the four-layer boundary.

## Design

### Public contract

`auth login [--provider PROVIDER] [--method ...]` remains `RoleAct`, effect `write`, with
the same fixed `auth-credentials` target, output, prerequisites, faults, and
capability ID. `--provider` becomes an optional flag whose omission
means terminal selection. `--method` requires an explicitly supplied provider,
so AWS method selection is deterministic before interaction. No opaque
reference is produced or consumed.

### Layer changes

- Domain: no credential or result-schema changes.
- Application: expose terminal capability and the closed reviewed-login-provider predicate needed by CLI orchestration.
- Infrastructure: no adapter changes.
- CLI and catalog: optional input declaration, omitted-provider orchestration, and a selector backed by the existing terminal menu.

### Data and control flow

Catalog parsing preserves omitted versus supplied provider. For omission, CLI
checks TTY capability, calls the read-only auth status use case, derives the
eligible provider options, presents them on stderr, validates the returned
choice against the snapshot, and calls the existing login mutation with that
exact provider and the snapshot-returned Context name. Explicit provider input
skips the status read and selector.

### Error and cancellation behavior

Redirected omission returns `auth_login_tty_required` before status or login
calls. Empty eligible collections return `provider_login_unsupported` before
mutation. Cancellation maps to retryable `operation_canceled`. `--method`
without provider is rejected by the shared parser as `invalid_arguments`.
Provider-native failures and post-action outcomes remain unchanged.

### Security and public boundary

The change reads only secret-free provider IDs already exposed by `auth status`.
No new credential surface, executable selection, destination, dependency,
handle authority, or state mutation is introduced.

## Implementation slices

1. Catalog and CLI interaction tests.
2. Optional-input and selector implementation.
3. Durable product/authentication/readiness documentation.
4. Focused verification and `task check`.

## Verification

- Unit and contract tests: auth catalog/parser/help and auth CLI selection tests.
- Negative side-effect tests: redirected omission, cancellation, empty eligible provider collection, and method-without-provider.
- Opaque-reference and complete-pagination tests: not applicable; fixed target and complete scalar output remain unchanged.
- Structured output, hostile-output, and recovery tests: existing auth result tests plus selector safe-text rendering tests.
- Agent-readiness scenario and discovery-round-trip count: explicit provider remains one scoped-help plus execution; human omission remains one execution with one internal status read and zero external processing.
- Human-handoff scorecard for setup/authentication candidates: omitted provider removes one prior provider-ID discovery/typing step while preserving explicit choice and provider-native login ceremony.
- Manual observation: exact human and agent help plus a PTY selector unit path.
- Required profiles: focused Go tests and `task check`.
- Generated-diff or artifact checks: `task check` covers generated site/catalog drift.

## Rollout and rollback

Explicit provider selection moves from the pre-v1 positional form to
`--provider`; the former positional form is rejected instead of becoming a
hidden alias. Omission changes from parser rejection to terminal interaction.
Rolling back code restores the prior argument grammar without state migration
because no schema or persisted state changes.

## Documentation promotion

Update the product command/input contract, authentication command syntax, and
agent-readiness authentication journey. No thesis, architecture, or security
invariant requires revision.
