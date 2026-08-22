# ADR 0074: Expose one reviewed Workspace service

- Status: Accepted
- Date: 2026-08-22
- Deciders: Tobari maintainers
- Scope: Product, catalog, attachment architecture, HTTP relay, security, and
  harness
- Revises: Catalog fixed-target create reference production
- Related: ADR 0049, ADR 0055, ADR 0073
- Revised by: None
- Superseded by: None

## Context

Development servers normally listen inside a Workspace while their browser runs
on the trusted host. Docker publishing would bind this access to container
lifetime and daemon configuration. Installing the host CLI in the Workspace or
reusing the browser channel as a generic RPC bus would give untrusted processes
an interface broader than this one task. ADR 0073 also rules out drawing trusted
review over the child's terminal.

Compatibility observations used exact host-facing authority
`127.0.0.1:<random-port>` while targeting Workspace
`127.0.0.1:<service-port>`. Pinned Vite 8.2.2, Next.js 16.3.2 with React 19,
Storybook 10.5.10, and Jupyter Server 2.20.0 with ipykernel 7.3 accepted that
authority without Host, Origin, redirect, cookie, content, or WebSocket
rewriting. A random `.localhost` name was rejected by Jupyter's default remote
access check, so numeric loopback is the compatible narrow contract.

The helper creates an opaque pending child resource in one fixed
current-attachment scope. The existing catalog rule allowed fixed-target acts
to produce no references at all, which made an opaque create result impossible
without inventing a second registry or reconstructing identity from a port.

## Decision

An interactive attachment receives one Tobari-owned, engine-native
`tobari-expose` helper, mounted read-only. A dedicated Linux main hardcodes the
helper Program; `argv[0]` never selects host authority. The canonical base
cross-compiles it for Docker `TARGETARCH` with a pinned Go builder and the exact
checked source/module closure. The host extracts it from the verified
source-derived base through a bounded temporary container, validates its
source/API/digest identity and Linux ELF engine architecture, and retains one
owner-only copy for read-only mounting into standard and custom Runtimes.
`tobari-expose <port>` requests review of one exact
non-privileged Workspace-loopback port. It neither chooses nor opens host
access. A separate host terminal runs `tobari review services` and immediately
applies one reference-bound `Allow once` or `Deny`. Bare `tobari review` is
only the Catalog-derived namespace listing; it owns no selector or authority.
The existing `tobari review permissions [--watch]` and its staged Apply remain
unchanged.

Allow once makes the live attachment owner bind one random IPv4-loopback host
port. The URL and every accepted HTTP/1.1 request use exact authority
`127.0.0.1:<host-port>`. After bounded validation, bytes relay to exact
Workspace `127.0.0.1:<service-port>`. WebSocket Upgrade switches to a bounded
bidirectional stream only after a valid `101` response. Tobari does not rewrite
Host, Origin, redirects, cookies, application headers, or content. TLS
termination, HTTP/2, raw TCP, UDP, LAN binding, automatic discovery, health
polling, requested host ports, and automatic browser opening are excluded.

The attachment owner—not the helper, reviewer, cluster, Context, or Workspace
container—owns pending requests, host listeners, streams, and cleanup. The
helper may list only its current attachment and stops an exposure only by the
opaque `exp_...` reference returned by create or list. Attachment exit closes
listeners and active streams and removes pending state and rendezvous records.

Host review discovers live owners through owner-only atomic ephemeral records
and unpredictable Unix rendezvous sockets. Both sides validate peer process,
user, nonce, and attachment identity. Review always requests a fresh snapshot
and never acquires route or listener lifetime. Stale, malformed, mismatched,
symlink, or forged records fail closed and are removed without following them.
Workspace output, control messages, and browser requests cannot invoke or drive
review.

One global Program-aware Catalog validates commands and opaque-reference flow
across `tobari` and `tobari-expose`, while routing and help expose only the
selected program. A fixed-target `EffectCreate` may now produce confirmed opaque
child-resource references only when it consumes no reference, the produced kind
differs from its fixed target kind, `target_inputs` is explicitly empty, no
parent or target ID is bound, and `TargetKind` equals the fixed scope kind.
Fixed-target reads and writes remain reference-free. Mutation Impact, canonical
invoker, and confirmed-result requirements are unchanged.

## Consequences

- Routine browser development takes one Workspace request, one visibly
  trusted-host approval, and one exact loopback URL without Docker publishing.
- Approval is temporary access, not Context policy, a remembered decision, or
  Host Loopback authority in the opposite direction.
- The data plane accepts bounded HTTP metadata but treats application bytes as
  untrusted opaque data. No application payload enters logs, policy evidence,
  review copy, or diagnostics.
- A validated request whose Workspace target is unavailable receives one fixed
  secret-free 502 response. No probe creates application-visible traffic.
- Pending requests are bounded at 64, active exposures at 16, connections per
  attachment at 64, headers/messages at 32 KiB, and setup/shutdown phases have
  finite deadlines. Stream backpressure is kernel- and pipe-bounded; shutdown
  closes both directions.
- `tobari -- tobari-expose 3000` is valid but not a useful persistent workflow:
  the direct child exit ends the attachment and therefore its exposure.

## Mechanical enforcement

- Catalog tests prove global cross-program reference closure, program-filtered
  routing/help, the fixed-create child-reference rule, and unchanged fixed
  read/write and mutation bindings.
- Domain/application tests validate ports, request/exposure references, scope,
  state, action ordering, and invalid-input zero-side-effect behavior.
- Infrastructure tests cover owner-only rendezvous, Darwin/Linux peer identity,
  stale/forged cleanup, concurrent attachments, exact authority before
  Workspace connection, ambiguous and smuggling headers, keepalive revalidation,
  fixed 502, WebSocket Upgrade, 4 MiB full-duplex backpressure, cancellation,
  foreign references, and owner teardown.
- Asset tests require source/snapshot equality, the dedicated hardcoded helper
  Program, spoofed-`argv[0]` denial, pinned-builder Linux amd64/arm64
  construction, source/API/digest/regular-file/safe-mode/Linux-ELF/engine-arch
  extraction, cleanup and stale replacement, owner-only executable host mode,
  and read-only `/usr/local/bin/tobari-expose` mount for standard and custom
  Runtimes. Clean-environment integration closes request, discovery, Allow,
  exact-authority relay, exhaustive list, opaque stop, and attachment teardown.
- Catalog, capability-ledger, agent-readiness, public-boundary, security, and
  generated-site gates keep the public grammar and excluded transports aligned.

## Reconsideration trigger

Broader protocols, another bind address, persistent approval, automatic
discovery, application-specific rewriting, or a generic attachment RPC require
a separate product and trust-boundary decision with compatibility, resource,
cleanup, and hostile-input evidence.
