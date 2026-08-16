# ADR 0048: Make pinned native tool authentication ready

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, policy, runtime, and harness
- Revises: ADR 0039, ADR 0044, and ADR 0046
- Related: ADR 0047
- Superseded by: None

## Context

Standard authentication belongs to each client inside its persistent Workspace,
but a native login still composes two independently governed effect classes:
exact HTTP requests leaving the Workspace and purpose-limited browser actions
performed by the attached trusted host process. Treating either class as an
ordinary project permission makes first use depend on provider transport
knowledge rather than the supported native workflow.

A live GitHub CLI 2.96.0 replay exposed this gap. While `POST
https://github.com/login/device/code` was denied, GitHub CLI selected its
loopback web-application fallback. Tobari opened the strict authorization URL
and relayed its callback, but the later `POST
https://github.com/login/oauth/access_token` was denied. After those exact
effects were allowed, GitHub CLI selected its preferred device flow, displayed
a one-time code, and requested the exact
`https://github.com/login/device` browser target. The attached-session observer
did not recognize that host-open-only shape, so GitHub CLI attempted a missing
Workspace-local `xdg-open` and the native workflow still failed.

The same decision recurs for every pinned native tool: its reviewed setup must
name the finite exact effects required for routine readiness without creating
provider-wide authority, process identity, or a runtime extension surface.

## Decision

The built-in agent-ready source composes named compile-time native tool
authentication readiness bundles. The initial closed set is `claude_ready`,
`codex_ready`, and `gh_ready`, each coupled to the corresponding client version
pinned by the canonical runtime. A bundle is review provenance, not runtime
authority: normalization expands it into the existing exact policy rules, and
neither its name nor an executable name enters the Context snapshot or OPA.
Custom presets cannot select, extend, or define readiness bundles.

`gh_ready` for GitHub CLI 2.96.0 grants exactly:

- `POST https://github.com:443/login/device/code`; and
- `POST https://github.com:443/login/oauth/access_token`.

It grants no GitHub API, Git transport, repository, release, download, upload,
self-update, host-wide, or path-prefix authority. The grants are Context-wide
HTTP effects available to every process in the Context, and exact Deny remains
terminal.

The standard attached-session observer also accepts the exact
`https://github.com/login/device` target only from GitHub CLI's complete bounded
no-newline browser prompt. It first verifies the exact selected owned Workspace,
opens the target once through the existing strict host browser adapter, creates
no listener, and leaves the child's bytes and Enter input unchanged. It does
not read, retain, relay, validate, or render the one-time code. Partial,
oversized, duplicate, ambiguous, neighboring, queried, or non-GitHub targets
open nothing.

GitHub CLI's strict loopback web-application fallback remains a distinct branch
of the same observer. That branch continues to validate the fixed OAuth client,
scope ceiling, state shape, callback host/path/port, bind one host-loopback
listener, and relay one opaque callback. Codex retains its separately reviewed
callback branch. A device target never creates callback authority.

Adding or updating a pinned tool requires reviewing its artifact identity,
exact Workspace authentication effects, exact host interactions, negative
neighbors, and routine native-login fixture as one compatibility change.
Unobserved or unmatched effects remain denied or reviewable.

## Consequences

### Positive

- A fresh default Context can complete pinned GitHub CLI device login through
  its native prompt without a permission-inbox detour or Workspace browser.
- Future pinned tools have one explicit review unit for authentication
  bootstrap without adding a provider plugin system.
- The Gateway remains generic and effect-based; no process-name claim is added.
- Device and loopback flows retain different, mechanically checked host powers.

### Negative

- Every process in an agent-ready Context can call the two exact GitHub
  authentication endpoints.
- GitHub CLI prompt or endpoint drift requires a reviewed Tobari update and
  otherwise falls back visibly to the client's native failure guidance.
- The built-in preset revision changes. Existing immutable Context snapshots do
  not receive `gh_ready` automatically.

## Mechanical enforcement

- Domain tests fix the readiness bundle IDs, client versions, exact GitHub
  methods/authority/paths, strict-preset zero grants, and absence of neighboring
  GitHub baseline authority.
- Runtime checks bind `gh_ready` to the canonical GitHub CLI 2.96.0 artifact
  lock alongside the existing Claude and Codex pins.
- Fragmented prompt tests preserve every child byte, open the exact device URL
  once, and prove zero callback-listener calls.
- Hostile prompt tests reject neighboring paths, query additions, other hosts,
  incomplete trailers, duplicate URLs, replay, ambiguity, and size overflow.
- Existing loopback fallback, Codex PTY/callback, ownership, opener-failure,
  listener-cleanup, exact-Deny, and Gateway secret-redaction tests remain gates.
- The agent-readiness scenario uses synthetic effects and records no one-time
  code, token, account identity, authenticated response, or raw transcript.

## Compatibility and migration

No public command, flag, custom preset schema, credential schema, Gateway
protocol, or Workspace state format changes. Existing Contexts retain their
stored preset revision. A user who wants `gh_ready` creates a new Context or
deletes and recreates disposable pre-public local Context state, then re-enters
its Workspace and performs native login there.

## Security and public-boundary impact

This decision widens the default baseline by two exact unauthenticated GitHub
authentication effects and widens the attached host observer by one exact
host-open-only URL. It does not authorize a GitHub host, prefix, API, callback,
or executable. Repository fixtures use synthetic prompts and effects and never
contain a live device code, token, state, callback, account identifier, or
authenticated transcript.
