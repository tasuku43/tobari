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

A second live replay exposed two more compatibility effects. After device-token
exchange, GitHub CLI 2.96.0 sends `query UserCurrent { viewer { login } }` to
`POST https://api.github.com/graphql`. An existing Context with no declared
GraphQL endpoint correctly treated that request as ordinary HTTP and therefore
offered a route-wide exact permission, which is too broad for routine login.
The replay also proved that opening the device URL from the observed interactive
prompt does not complete the client interaction: GitHub CLI still waits for
Enter and then invokes its own Workspace browser adapter, producing missing-
opener guidance even though the host browser already succeeded.

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
- `POST https://github.com:443/login/oauth/access_token`; and
- GraphQL `query` root `viewer` at the declared exact endpoint `POST
  https://api.github.com:443/graphql`.

The GraphQL grant is not an HTTP route grant: mutation, any sibling root, a
mixed-root document, another operation type, ordinary HTTP at the same route,
and a neighboring endpoint remain unmatched. It grants no general GitHub API,
Git transport, repository, release, download, upload, self-update, host-wide,
or path-prefix authority. The grants are Context-wide effects available to
every process in the Context, and exact Deny remains terminal.

Strict policy-preset schema V1 adds an explicit `graphql_baseline_grants`
collection analogous to `mcp_baseline_grants`. Each item binds one declared
exact GraphQL endpoint, operation type, and root field. A grant without its
matching endpoint, an incomplete identity, a duplicate, or a value outside the
preset ceiling fails validation. OPA requires every root in one request to
match independently before the baseline permits the request.

The canonical runtime exposes a compatibility wrapper at `/usr/local/bin/gh`
and retains the pinned real executable outside `PATH`. Only exact default argv
`gh auth login` is adapted. The wrapper runs the fixed GitHub.com HTTPS web/device
login with GitHub CLI prompting and its Workspace browser disabled; this makes
the client display its provider-owned one-time code and fixed device URL, then
begin polling without Enter or `xdg-open`. After successful acquisition it runs
GitHub CLI's fixed Git credential setup for GitHub.com. Every other argv executes
the real pinned binary unchanged.

The standard attached-session observer accepts the exact
`https://github.com/login/device` target from either the legacy complete bounded
no-newline prompt or the wrapper's exact non-interactive URL line. It first
verifies the selected owned Workspace, opens the target once through the strict
host browser adapter, and creates no listener. It does not read, retain, relay,
validate, suppress, or render the one-time code. Partial, oversized, duplicate,
ambiguous, neighboring, queried, or non-GitHub targets open nothing.

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
  one `gh auth login` without a permission-inbox detour, Enter, or Workspace
  browser executable.
- Future pinned tools have one explicit review unit for authentication
  bootstrap without adding a provider plugin system.
- The Gateway remains generic and effect-based; no process-name claim is added.
- Device and loopback flows retain different, mechanically checked host powers.

### Negative

- Every process in an agent-ready Context can call the two exact GitHub
  authentication endpoints and the exact GraphQL `query` / `viewer` effect.
- Exact default `gh auth login` is a pinned compatibility workflow rather than
  GitHub CLI's interactive choice sequence; explicit non-default argv remains
  native and outside this ready-state promise.
- GitHub CLI prompt or endpoint drift requires a reviewed Tobari update and
  otherwise falls back visibly to the client's native failure guidance.
- The built-in preset revision changes. Existing immutable Context snapshots do
  not receive `gh_ready` automatically.

## Mechanical enforcement

- Domain tests fix the readiness bundle IDs, client versions, exact GitHub
  methods/authority/paths, GraphQL operation/root, strict-preset zero grants,
  and absence of neighboring GitHub baseline authority.
- Runtime checks bind `gh_ready` to the canonical GitHub CLI 2.96.0 artifact
  lock alongside the existing Claude and Codex pins.
- Fragmented interactive-prompt and non-interactive-line tests preserve every
  child byte, open the exact device URL once, and prove zero callback-listener
  calls.
- Runtime checks fix the wrapper's exact interception predicate, real executable
  path, login/setup argv, non-interactive browser environment, pass-through,
  source/snapshot equality, and absence of network or shell-evaluation authority.
- OPA tests require all GraphQL roots to match, preserve GraphQL exact-Deny
  precedence, and reject mutation, sibling, mixed-root, empty-root, ordinary
  HTTP, and neighboring-route canaries.
- Hostile prompt tests reject neighboring paths, query additions, other hosts,
  incomplete trailers, duplicate URLs, replay, ambiguity, and size overflow.
- Existing loopback fallback, Codex PTY/callback, ownership, opener-failure,
  listener-cleanup, exact-Deny, and Gateway secret-redaction tests remain gates.
- The agent-readiness scenario uses synthetic effects and records no one-time
  code, token, account identity, authenticated response, or raw transcript.

## Compatibility and migration

No public command, flag, credential schema, Gateway protocol, or Workspace state
format changes. Strict preset schema V1 gains the required explicit
`graphql_baseline_grants` collection; custom sources must include it, including
an empty array when unused. Existing Contexts retain their stored preset
revision and do not gain the declared endpoint or semantic grant. A user first
resets any route-wide learned `POST /graphql` permission, then creates a new
Context or deletes and recreates disposable pre-public local Context state,
re-enters its Workspace, and performs native login there.

## Security and public-boundary impact

This decision widens the default baseline by two exact unauthenticated GitHub
authentication effects and one authenticated semantic GraphQL query/root. It
widens the runtime compatibility and observer input shapes but not the one exact
host-open-only URL. It does not authorize a GitHub host, route, mutation,
repository operation, callback, or executable identity. Repository fixtures use
synthetic prompts and effects and never contain a live device code, token,
state, callback, account identifier, or authenticated transcript.
