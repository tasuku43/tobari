# ADR 0039: Make the default Context agent-ready

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, policy, runtime images, harness, release, and public boundary
- Revises: ADR 0025, ADR 0029, ADR 0038, and the default-preset consequence of Thesis 3
- Superseded by: Optional-surface exclusion revised by ADR 0047; Codex client pin revised by ADR 0042 and runtime publication revised by ADR 0043

## Context

Tobari exists to put an execution boundary around coding agents. Its previous
default, `builtin/reviewed-exact`, required the developer to review every
network effect, including the coding client's own bootstrap, model catalog,
account status, inference, and telemetry requests.

Live validation exposed the product failure. Claude Code 2.1.220 held a valid
brokered account session, yet its interactive status reported an expired login,
fallback API mode, and fallback model while bootstrap, OAuth usage, and message
requests were denied. Codex 0.146.0 produced the same class of candidates for
models, responses, account usage, and telemetry. The user had to reverse-engineer
the agent's internal protocol before the agent could perform useful work.

Gateway cannot authenticate a process name. It sees a Context/project principal
and a normalized HTTP effect. Granting authority to a command named `claude` or
`codex` would therefore be both inaccurate and bypassable. Conversely, allowing
an entire vendor host would also authorize plugins, MCP registries, remote
connectors, file transfer, self-update, and future provider features without a
separate decision.

The canonical runtime already had integrity-pinned build-only recipes for
Claude Code 2.1.220 and Codex 0.146.0. Their redistribution and image-layer
license reviews remain pending.

## Decision

New Context creation defaults to `builtin/agent-ready`. The preset is a strict,
normalized, immutable schema-V1 snapshot using the existing baseline-grant
mechanism. It grants only the exact HTTPS authority, method, and path effects
reviewed for the pinned Claude Code 2.1.220 and Codex 0.146.0 clients:

- model/inference requests;
- bootstrap and model catalog;
- account profile, settings, policy limits, usage, and rate-limit state; and
- the clients' fixed first-party analytics/telemetry endpoints.

The grant is Context-wide HTTP authority. Every process in that Context can use
the same exact effects; Tobari neither claims nor infers executable identity.
A learned or baseline exact Deny remains terminal over a baseline grant.

The baseline excludes plugin catalogs and execution, MCP discovery and
registries, remote connectors, file upload/download, release and changelog
downloads, evaluation/experiment routes, and self-update. Those and all
third-party destinations continue through terminal guardrails or ordinary exact
deny/review/allow learning. The stricter `builtin/reviewed-exact`,
`builtin/get-only-reviewed`, and `builtin/offline` presets remain explicit
Context-creation choices.

The canonical `runtimes/base` source includes both pinned clients outside the
mutable Workspace home. Claude self-update is disabled, Codex uses its pinned
standalone package, and both version commands are part of the runtime contract.
The client pins and the agent-ready effect catalog are one compatibility unit:
updating either client requires reviewing and updating the other side in the
same change.

The base workflow builds and verifies the combined image but has no
registry-write permission, login, or push step. The canonical runtime declares
`NOASSERTION` for the combined layer. ADR 0043 makes the base permanently
local-build-only; Tobari publishes the pinned recipe rather than that image.

## Consequences

### Positive

- A new default Context can start either supported coding client without an
  inbox full of client-internal control-plane requests.
- Project-originated third-party egress remains deny/review by default.
- The policy boundary stays generic and effect-based; no process-name security
  model is introduced.
- Runtime and network compatibility move together under fixed pins and exact
  tests.

### Negative

- Any process in an agent-ready Context can call the exact baseline endpoints.
- The fixed client versions and routes may drift together, requiring a Tobari
  update rather than silent widening.
- The combined base is substantially larger.
- First use must build the combined base on the user's Docker host.

## Mechanical enforcement

- Domain tests fix the built-in catalog, non-empty core grants, stable
  normalization, and negative vocabulary for optional surfaces.
- Aggregate-policy tests keep exact Deny ahead of baseline grant and keep every
  unmatched effect in the ordinary Context evaluator.
- Runtime checks bind base Dockerfile arguments to the exact Claude and Codex
  artifact locks, require both executables outside Workspace home, require
  self-update disablement, and smoke both versions.
- Runtime and release checks reject a base workflow or protected release path
  containing registry-write permission, Docker login, or `--push`.
- Product, architecture, security, release, harness, and readiness documents
  distinguish the agent-ready default from the three stricter presets and from
  process identity.

## Compatibility and migration

Policy preset snapshots remain immutable. Existing Contexts retain their exact
stored origin and revision; Tobari does not retrofit baseline authority into
them. This pre-public V1 change adds no compatibility reader or migration.
Developers who want the new envelope create a new Context (or delete and
recreate disposable local state) and re-enter its Workspace.

## Security and public-boundary impact

This decision intentionally widens the default Context's initial HTTP authority
to a finite audited set. It does not widen destinations by suffix, wildcard,
prefix, provider brand, process name, query, header, or body. Brokered primary
credentials remain post-policy and project-bound. Repository fixtures contain
no live credentials or authenticated provider responses, and public image
publication remains mechanically disabled while redistribution is unresolved.
