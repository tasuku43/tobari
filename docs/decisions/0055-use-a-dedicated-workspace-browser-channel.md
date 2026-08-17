# ADR 0055: Use a dedicated Workspace browser channel

- Status: Accepted
- Date: 2026-08-17
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, runtime, and harness
- Revises: ADR 0046, ADR 0048, ADR 0050, and ADR 0053
- Related: ADR 0051 and ADR 0052
- Revised by: ADR 0057
- Superseded by: None

## Context

ADR 0053 inferred browser intent from provider presentation. It copied attached
terminal output through a bounded observer, recognized provider-specific lines
or terminal frames, and used a GitHub marker to represent native confirmation.
A live TWG 1.2.5 device login reached its confirmation and polling state but did
not reliably open the browser. Terminal rendering, buffering, and provider copy
made the presentation parser a compatibility dependency even though browser
opening already has an executable boundary.

Linux system-call interception was considered. Browser launch is normally an
`execve` of a browser adapter rather than a dedicated kernel operation. Seccomp
user notification would therefore have to mediate broad process execution,
copy pointer-based arguments safely, reproduce exec success and failure
semantics, and add a trusted VM/runtime supervisor. That is disproportionate
authority for one narrow host effect.

## Decision

Each attached Workspace receives one unpredictable Unix socket path in its
`/run` tmpfs and three attachment-local environment values: `BROWSER` and
`GH_BROWSER` name one Tobari opener executable, and
`TOBARI_BROWSER_SOCKET` names that socket. The same binary-embedded, read-only
opener is mounted over `xdg-open`. It accepts exactly one target argument,
sends one bounded schema-v1 JSON request to the socket, accepts only one exact
boolean schema-v1 response, emits no target or protocol data, and otherwise
returns the native opener failure status.

The Unix listener belongs to a fixed Python agent started by a separate,
non-TTY Docker exec. That agent has no URL or browser policy. It transports one
framed request at a time over its stdin/stdout control stream and removes the
socket when the attachment closes. The attached shell keeps its original
stdin, stdout, stderr, and Docker PTY directly; Tobari no longer observes or
rewrites terminal output.

The trusted host remains the only authority. It rejects unknown schema fields,
duplicate keys, malformed or oversized frames, replays beyond the attachment
budget, unowned container selection, and every URL outside the existing closed
Claude Code, Codex, GitHub CLI, TWG, and pup authentication union. One fresh
compile-time driver registry is the only Workspace-union membership source.
Each entry supplies one stable driver ID, one exact semantic target parser, and
whether its parser must return a non-privileged callback port. Dispatch evaluates
the complete small registry and rejects zero matches, multiple matches, duplicate
IDs, missing parsers, and callback-mode inconsistencies rather than using entry
order to resolve authority.
Callback-bearing Codex, GitHub, and pup requests bind their exact validated non-privileged host
loopback port before browser open and relay one opaque callback through the
selected Workspace. Claude remote callback and GitHub/TWG device requests open
without a listener.

Device-code timing now belongs entirely to the native CLI. GitHub CLI calls
`GH_BROWSER` after its native Enter transition. TWG calls its selected browser
adapter after its native confirmation. Because Tobari reacts to the executable
request rather than displayed text, the visible copy window remains available
without provider-specific terminal parsing or input interception.

The opener is a binary-owned runtime asset, not a Context image customization.
Its content participates in the runtime asset version and desired Workspace
container spec. Activating a new Tobari binary through normal cluster
reconciliation replaces a stale Workspace container and mounts the new asset;
no Context snapshot recreation or custom Context image rebuild is required.

## Consequences

- New compatible CLIs that honor `BROWSER`, `GH_BROWSER`, or `xdg-open` need no
  provider presentation parser.
- Provider URL semantics remain explicit and fail closed; this is not a generic
  host opener.
- A newly reviewed client adds one semantic parser and one compile-time registry
  entry; it does not add another transport, opener, relay, or allowlist chain.
- Localhost callback relay remains necessary for reviewed callback flows.
- A custom API-1 runtime without Python cannot run the control agent and retains
  the CLI's manual fallback; it receives no broader host authority.
- The GitHub marker, output observer, PTY copy/resize relay, and TWG staged URL
  state are removed.

## Mechanical enforcement

- Runtime materialization tests bind executable mode and asset versioning.
- Workspace spec and Docker-create tests bind both read-only opener mounts.
- Attachment tests bind the three transient environment values and direct
  Docker stream ownership.
- Protocol tests execute the real opener and agent, bind exact framing and
  cleanup, reject duplicate keys, unknown fields, malformed versions, oversized
  targets, neighboring URLs, ownership mismatch, browser failure, replay, and
  callback-port failure.
- Existing semantic URL and opaque callback tests retain the closed provider
  union and selected-Workspace relay invariants.
- Registry tests fix the complete driver IDs and callback modes, require fresh
  compiled values, cover every entry, and reject malformed or overlapping
  definitions.

## Compatibility and migration

No public command, flag, policy rule, credential format, or Gateway protocol
changes. `tobari cluster up` materializes the binary-owned asset revision and
normal Workspace reconciliation replaces only stale Workspace containers.
Existing native credentials persist in the Workspace home bind. Context
snapshots and custom Context images remain unchanged.

## Security and public-boundary impact

The Workspace receives a transient request capability only while attached. It
does not receive a host socket, Docker socket, browser executable, callback
parser, or arbitrary process supervisor. Authentication URLs and callback bytes
remain transient and absent from policy, audit, logs, durable state, and public
fixtures. Repository tests use only synthetic provider-shaped values.

## Revision by ADR 0057

ADR 0057 adds pinned custom-runtime pup authorization to the closed provider
union. Its strict US1 DCR authorization shape relays one callback on one of the
four callback ports compiled into pup 1.10.7; this does not create a generic
Datadog opener or callback route.
