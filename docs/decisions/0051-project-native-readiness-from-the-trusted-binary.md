# ADR 0051: Project native readiness from the trusted binary

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, policy, runtime, authentication, and harness
- Revises: ADR 0039, ADR 0048, and ADR 0050
- Revised by: ADR 0058
- Superseded by: None

## Context

Native readiness bundles were compile-time definitions, but Context creation
expanded their exact rules into the immutable `preset.json` snapshot. Adding
`twg_ready` therefore required users to recreate an otherwise unchanged
Context. The implementation location and the user-visible lifecycle did not
match: a trusted Tobari/client compatibility update behaved like an owner
capability-envelope change.

Simply appending the binary's current rules during policy projection is unsafe.
An endpoint removed by a later client version would survive indefinitely in an
older snapshot. Rewriting snapshots in place would instead violate their
integrity and recovery contract.

## Decision

Native-client readiness is a trusted binary compatibility overlay selected by
the immutable preset origin `builtin/agent-ready`. It is not part of the
Context-owned preset snapshot. New agent-ready Contexts persist only the core
guardrail, non-readiness baseline, templates, and MCP boundary. Strict built-in
and custom presets receive no overlay.

At aggregate policy generation Tobari:

1. validates the Context manifest and exact snapshot bytes/revision;
2. for `builtin/agent-ready` only, removes every native-readiness exact rule
   and semantic endpoint recorded in the binary's complete compatibility
   history;
3. adds the current compile-time `claude_ready`, `codex_ready`, `gh_ready`, and
   `twg_ready` expansions;
4. normalizes and validates the effective preset; and
5. projects that effective result into the Tobari-owned evaluator.

The history is append-only review metadata. When a current bundle changes, its
former version remains in history so old snapshotted rules can be removed. The
current set is selected only by compile-time version constants. Runtime data,
project files, images, executable names, and custom presets cannot define,
select, or extend a bundle.

The stored `policy_preset_revision` continues to identify the immutable
Context-owned snapshot, not the complete effective agent-ready baseline. The
trusted binary identity and reviewed bundle constants identify the overlay.
Exact baseline and learned Deny precedence remains terminal over the merged
result.

## Consequences

- Updating Tobari updates native readiness for existing agent-ready Contexts;
  no Context or Workspace recreation is required.
- A trusted binary update may add, remove, or change Context-wide native-login
  effects for every agent-ready Context. Bundle review is therefore part of the
  binary/release security boundary.
- Existing legacy snapshots remain byte-identical. Their historical readiness
  rules are replaced during projection rather than accumulated.
- Source preset edits still affect only future Context creation. Strict/custom
  origins and the immutable guardrail remain snapshot-owned.
- `policy preset show builtin/agent-ready` describes the current effective
  built-in source, while a Context report's preset revision remains its stored
  snapshot integrity revision.

## Mechanical enforcement

- Domain tests prove new snapshots exclude all historical readiness rules,
  current projection equals the effective built-in, legacy rules are replaced,
  exact Deny survives, and strict/custom origins are unchanged.
- Infrastructure tests create a core-only snapshot and a legacy pre-TWG
  snapshot, generate aggregate policy, prove current TWG effects are present,
  and prove neither snapshot is rewritten.
- The append-only history and current version constants are compiled together;
  bundle tests fix IDs, versions, exact effects, semantic endpoints, and
  negative provider neighbors.
- OPA tests retain exact-Deny precedence over every projected baseline grant.
- Full, security, and public gates cover snapshot integrity, aggregate
  generation, runtime compatibility, and documentation.

## Compatibility and migration

No public command, manifest schema, preset schema, Gateway protocol, or stored
file migration is required. Old and new snapshot revisions remain valid for
their exact bytes. The next ordinary aggregate regeneration (`cluster up`,
policy activation, Context lifecycle reconciliation, or equivalent existing
path) applies the installed binary's readiness overlay.

## Security and public-boundary impact

The trusted-binary boundary now includes the current native-readiness overlay
for every agent-ready Context. This is a deliberate dynamic compatibility
surface, but not a runtime extension surface: destinations remain finite exact
compile-time rules, exact Deny remains terminal, and strict/custom presets,
provider business operations, acquisition, downloads, file transfer, and
self-update gain no authority.

## Revision by ADR 0058

ADR 0058 makes readiness history an append-only contract revision independent
from the pinned client version and moves definitions into one dedicated
compile-time catalog. Binary projection and Context migration behavior remain
unchanged.
