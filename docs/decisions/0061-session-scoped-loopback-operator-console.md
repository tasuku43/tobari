# ADR 0061: Serve one session-scoped loopback operator console

- Status: Accepted
- Date: 2026-08-18

## Context

The trusted-host permission workflow is complete in the terminal, but users
also need one browser surface that makes shared-cluster health, local
Workspaces, pending permission evidence, and current learned decisions visible.
The most important browser task is the same staged Permission Inbox Apply that
already exists in `review permissions`; a second policy mutation mechanism would
weaken the product and security contracts.

A browser surface introduces a new local HTTP trust boundary. Binding only to
loopback is insufficient by itself because cross-origin requests and DNS
rebinding can still target local services, and putting an authority token in a
query string would expose it to ordinary URL handling.

## Decision

The Operator Console is compiled only into the unsupported experimental
capability profile produced by `task build:dev`; standard development and
release catalogs omit `serve`. In that profile, `tobari serve` starts one
foreground, process-owned operator console on an
OS-selected IPv4 `127.0.0.1` port. The command accepts no bind address, fixed
port, remote-access, daemon, or persisted-session option. Cancellation closes
the listener and invalidates the session.

The console generates one 256-bit random bearer token per invocation. The host
browser receives it only in a URL fragment. The embedded application moves it
to tab-scoped session storage, removes the fragment from the visible URL, and
sends it in an authorization header for API calls. No cookie is used. The
server validates the exact listener Host, loopback peer, bearer token, request
method, path, content type, and for writes the exact Origin. It sets a closed
Content Security Policy, serves no external assets, disables caching, bounds
headers and request bodies, and applies finite server and use-case deadlines.

The browser reads one typed task-owned snapshot composed from existing cluster
status, Workspace list, policy review, and learned-decision application use
cases. The console does not parse CLI text, Docker output, labels, or display
order. Browser staging is inert. Final Apply submits a bounded
`PolicyReviewDecisionSet` containing unchanged opaque review-item IDs to the
existing internal `policy apply-reviewed` fixed-target action. That action
re-reads evidence and rules, validates one Context-bound set, tests the complete
all-Context candidate, activates one revision, and returns the authoritative
receipt. The browser never retries the denied request.

The command attempts one purpose-limited open of its exact generated loopback
URL. Failure leaves the server running and prints the exact manual URL. The
MVP UI uses the selected Operator Console visual language with dark and light
modes; presentation never becomes authority.

## Consequences

- The browser is a trusted-host convenience surface, not a remote control
  plane or a second policy engine.
- Standard and release binaries carry no Operator Console command or composition
  wiring while the interface is evaluated.
- Browser refresh can repeat reads but cannot replay a confirmed mutation.
- A stolen live session token can act with the user's console authority until
  the foreground command exits, so it is never logged, persisted, placed in a
  query string, or shared with external assets.
- Only a ready local cluster can enter the mutation-capable MVP. Diagnosis and
  remote administration remain separate work.
- Future remote access, daemonization, selectable listeners, or additional
  mutations require a new product and security decision.

## Verification

Domain and application tests bind snapshot identity and every nested scope.
HTTP tests require exact loopback listener/peer/Host/Origin/token behavior,
closed routes and methods, strict JSON, bounded bodies, no cookies, no-store
responses, CSP, and cancellation cleanup. Browser-asset tests prove fragment
removal, session-scoped bearer use, explicit staging, final confirmation,
receipt rendering, no automatic retry, and both dark and light modes. Catalog
and CLI tests fix discovery, startup output, manual fallback, cancellation, and
reuse of the internal reviewed-set action. `task check` remains completion
authority.
