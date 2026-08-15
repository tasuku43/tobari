# ADR 0032: Detect the Codex host authentication contract

- Status: Accepted
- Date: 2026-08-12
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O,
  harness, agent readiness, and release
- Revises: The exact trusted-host Codex product-version requirement in ADR
  0025 and ADR 0031
- Revised by: ADR 0036 replaces explicit device authentication with Codex's
  native trusted-host browser login while retaining this contract detection
- Superseded by: None

## Context

The OpenAI plan coupled two different compatibility surfaces to Codex 0.146.0:
the trusted-host CLI used once to acquire a ChatGPT OAuth session and the
Workspace CLI that consumes Tobari's handle-only `.codex/auth.json` shim. A
normal host package-manager update to Codex 0.147.0 therefore prevented login
before the fixed acquisition command or its strict state postconditions could
be evaluated.

Exact product-version output was only a compatibility proxy. It was not
executable provenance because another binary can print the same version. The
existing canonical-path, non-writable-file, before/after SHA-256, private-home,
fixed-argv, strict-state, and checked-cleanup controls define the actual host
execution boundary.

This does not make an operator-installed Codex binary behaviorally sandboxed or
prove its publisher provenance. Tobari trusts the selected conventional host
installation to perform its own login, including provider network calls. The
compiled contract bounds Tobari's invocation, captured state, and commit; it
does not claim to observe every filesystem or network side effect performed
inside the upstream CLI. Closing that larger boundary would require a reviewed
artifact allowlist and/or an OS process/network sandbox.

## Decision

The trusted-host Codex driver accepts a stable bounded `codex-cli X.Y.Z`
identity observation without comparing it to an allowlisted product version.
The observed version is retained as canonical audit metadata. Compatibility is
decided by the compiled host-login contract:

- one canonical conventional non-project executable;
- non-group/world-writable identity and one before/after SHA-256 witness;
- fixed `login --device-auth` argv with file credential storage and startup
  update checks disabled;
- a sanitized private temporary `HOME` and `CODEX_HOME`;
- bounded visible output, timeout, cancellation, and checked cleanup; and
- exact canonical `auth.json` keys, managed ChatGPT mode, null API key,
  printable bounded tokens, RFC3339Nano refresh time, matching namespaced
  account claim, and non-FedRAMP authority.

The existing exact `openai_codex_chatgpt_oauth` driver ID is the compiled
contract revision. The canonical state shape remains unchanged: its existing
`version` field records the observed host product version beside executable
path and digest. Broker accepts only that exact canonical state and driver
revision. It
continues to own the fixed client ID, token endpoint, same-account refresh,
barriers, and post-policy selection. A future state shape, client identity,
endpoint, account authority, or refresh behavior requires a new reviewed
compiled contract revision; a product version alone creates no new authority.

The Workspace consumer contract is independent. The handle-only auth-file
projection remains byte-exact and is validated against the separately pinned
Codex runtime recipe. A custom Workspace binary is not made a host acquisition
authority and receives no compatibility claim merely because a host login
succeeded.

No owner manifest, flag, environment variable, or configuration file can
select a driver contract, executable, argv, parser, OAuth endpoint, client ID,
or Workspace projection. The provider-specific driver remains compiled; this
decision does not add a generic capability-probe or executor framework.

## Consequences

- Routine stable host Codex upgrades no longer require a Tobari product-version
  bump when the actual login/state contract is unchanged.
- Acquisition, Broker refresh, and Workspace consumption have independently
  reviewable compatibility boundaries; schema or behavior drift still fails
  closed before credential commit.
- A release can preserve captured state while changing an undocumented refresh
  identity. Acquisition may then succeed and later refresh fails closed until
  a new contract is reviewed.
- Existing canonical OpenAI records remain readable because the state keys and
  schema are unchanged; their observed 0.146.0 version remains valid metadata.
- Rollback is asymmetric: a later-version record remains removable by the old
  Broker because inner state is opaque to vault/logout, but old resolve may
  reject it after status reported ready. Recovery is to restore the updated
  Broker or logout and re-login with the rolled-back acquisition contract;
  observed versions are never rewritten to manufacture compatibility.
- Current Codex access-token login is not a replacement for the Workspace shim:
  its token-shape classification and account-metadata hydration do not accept a
  Tobari handle as an opaque external bearer. Replacement waits for an upstream
  external credential-provider surface whose exact bearer, account-routing,
  and refresh contract can be reviewed; it is not inferred from host
  acquisition compatibility.

## Mechanical enforcement

- Host-driver tests accept multiple stable semantic product versions under one
  contract and reject malformed identity output, changed executables, unsafe
  state, schema/claim drift, cancellation, bounds, and cleanup failure.
- Go and Broker parsers require the exact driver ID, canonical observed
  version, executable digest binding, auth keys, claims, account, and refresh
  state. Broker tests retain the fixed refresh request and same-account result.
- Catalog tests remove host downgrade instructions while Workspace projection,
  Gateway replacement, runtime pin, manual release replay, canonical/snapshot
  equality, `task check`, and `task security` remain independent gates.
- A newly accepted host Codex release is reviewed against its official refresh
  client identity and replayed through one near-expiry refresh before release;
  captured login state alone does not prove refresh compatibility.
