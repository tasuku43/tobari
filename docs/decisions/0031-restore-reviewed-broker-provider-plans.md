# ADR 0031: Restore closed reviewed broker provider plans

- Status: Accepted
- Date: 2026-08-12
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O, state, harness, public boundary, and release
- Supersedes: The provider-removal and static-only authentication decisions in ADR 0030
- Revises: ADR 0019, ADR 0020, Datadog ADR 0021, ADR 0023, ADR 0025, and ADR 0027
- Revised by: ADR 0032 separates host Codex contract detection from the pinned
  Workspace Codex projection; ADR 0038 replaces Anthropic setup-token acquisition
  with isolated Context-runtime native login and a renewable session
- Superseded by: None

## Context

ADR 0030 narrowed the unpublished first V1 to GitHub plus owner static import
to reduce trusted code before publication. That deletion also removed the
reviewed outcomes required by the intended development environment: AWS IAM
Identity Center and console sessions, Datadog OAuth, Codex ChatGPT OAuth,
Claude setup-token acquisition, and the Chatwork static plan.

The project still has no public release or compatibility obligation. The
narrowing evidence therefore does not require retaining an incomplete provider
set when the maintainer explicitly requires the reviewed workflows. The later
exact-policy, guarded-network, source-principal, policy-preset, GraphQL, and
schema-V1 corrections remain stronger governing constraints and must survive
the restoration.

## Decision

First public V1 restores the closed compiled provider union:

- GitHub static credential through the reviewed GitHub CLI device flow;
- AWS IAM Identity Center and explicit AWS console login through reviewed host
  AWS CLI drivers, a bounded encrypted companion channel, post-policy refresh,
  and request-local SigV4 signing;
- Datadog US1 OAuth through the reviewed host pup acquisition flow and one
  fixed post-policy token refresh endpoint;
- OpenAI ChatGPT OAuth through the contract-checked reviewed host Codex driver,
  a handle-only `.codex/auth.json` shim reviewed against the separately pinned
  Codex 0.146.0 Workspace runtime, one fixed post-policy refresh endpoint, and
  exact account routing;
- Anthropic setup-token acquisition for exactly Claude Code 2.1.220 with only
  `CLAUDE_CODE_OAUTH_TOKEN=<handle>` projected; and
- Chatwork static protected-stdin import with exact header replacement.

The provider union is closed in compiled infrastructure. Owner manifests remain
strict non-secret, non-executable static-primary-secret plans and cannot select
a helper, driver, refresh endpoint, signer, supplemental header, companion
operation, shell, argv, environment, or provider business operation.

Auth Broker again owns typed dynamic records, encrypted refresh state, grant
revisions, project handles, durable no-replay barriers, and correlated outcome
settlement. Provider-native executables remain on the trusted host, not in the
Broker image or Workspace authority. The resident companion uses only the
reviewed bounded reverse Docker-exec protocol and exposes no host listener,
generic RPC, shell, or manifest-selected command.

Gateway continues to derive Context/project identity from the host principal
registry, remove and introspect a recognized handle before OPA, resolve or
refresh only after allow, and make at most one upstream attempt. AWS signing
and OpenAI supplemental account routing are fixed provider application plans;
OPA still authorizes generic exact HTTP/GraphQL effects and never provider
business operations. Invalid or ambiguous Tobari-looking handles never fall
back to passthrough.

All restored Tobari-owned schemas and component APIs are expressed directly as
exact V1. No old-state migration, compatibility reader, retired alias, or
fallback is restored. Development state from the static-only snapshot must be
recreated when its shape is incompatible.

## Consequences

### Positive

- The intended Codex, Claude, AWS, Datadog, GitHub, and Chatwork workflows are
  available without placing reusable host credentials in a Workspace.
- Login and refresh remain provider-specific closed contracts while Gateway
  policy stays generic.
- The current bounded provider selector becomes useful without turning owner
  manifests into executable extensions.

### Negative

- Trusted host drivers, the companion protocol, dynamic vault records, fixed
  refresh clients, AWS canonical signing, and additional release checks return
  to the maintained security surface.
- Codex and Claude integrations remain exact-version compatibility contracts
  and require manual release replay when their upstream clients change.
- Dynamic-provider use requires the trusted host process/companion to remain
  available, and an outcome-unknown refresh may require explicit re-login or
  logout after status reconciliation.

## Mechanical enforcement

- Catalog and selector tests admit only installed compiled login plans and
  validate explicit AWS method selection before mutation.
- Provider parsers reject every owner attempt to select dynamic kinds, drivers,
  helpers, refresh, signing, or supplemental headers.
- Host-driver tests fix executable identity and version where required, argv,
  environment, browser targets, bounded output/state, cleanup, cancellation,
  and secret-free failures.
- Companion tests fix handshake binding, direction-separated encryption,
  sequence/replay/gap rejection, bounds, cancellation, drain, disconnect, and
  zero generic execution authority.
- Broker tests fix encrypted dynamic state, grant/revision binding, per-record
  single flight, durable barriers, correlated CAS settlement, rotation,
  revocation, and secret-free results.
- Gateway tests prove handle removal and introspection before OPA, zero
  resolution/refresh/signing on deny, exactly one post-allow application,
  current source-principal and GraphQL behavior, audit redaction, and no invalid-
  marker fallback.
- Exact Codex/Claude projection tests and manual pinned-client release replays
  detect upstream compatibility drift.
- Canonical/snapshot equality, image-content/dependency checks, synthetic
  integration, `task check`, and `task security` remain completion gates.

## Compatibility and migration

No public version used the static-only snapshot. Readers accept only the new
exact V1 restored shape. Operators delete and recreate incompatible development
Context vault, Workspace projection, and cluster state rather than relying on
implicit conversion or secret-bearing migration.

## Security and public-boundary impact

The restored code introduces fixed external endpoints and trusted host
executables already reviewed in ADRs 0020, 0021, 0023, and 0025. Automated
fixtures use synthetic state, fake CLIs, and local intercepted transports.
Real credentials, device codes, account identifiers, provider responses,
handles, vaults, and authenticated transcripts remain prohibited repository
evidence. Image and dependency license/integrity review is required before
publication; no image or release publication is authorized by this ADR.

## Reconsideration signals

Revisit a plan when a pinned client or endpoint changes, a provider supplies a
stable external credential surface, refresh/signing semantics can no longer be
represented without broader authority, or operational evidence shows that a
plan's maintenance cost exceeds its user outcome. Do not route around the
closed registry with a generic executable or SDK plugin.
