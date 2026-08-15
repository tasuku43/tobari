# ADR 0040: Use the selected Context runtime for pup login

- Status: Accepted
- Date: 2026-08-16
- Deciders: Tobari maintainers
- Scope: Datadog authentication acquisition, browser callback, runtime tooling, and optional-toolbox retirement
- Revises: The Datadog acquisition and optional-toolbox portions of ADR 0020, ADR 0021, and ADR 0031
- Related: ADR 0038 (isolated native Claude acquisition)
- Superseded by: None

## Context

Datadog acquisition originally resolved a trusted-root pup from host PATH. An
initial isolation attempt instead required `tobari-toolbox:local`, an optional
separately built image containing an exactly pinned pup. Both choices separate
the credential writer from the Context runtime the user selected: the host
installation can drift independently, while the optional toolbox is an obscure
second runtime users must discover and maintain.

An exact pup version allowlist also does not establish native-state
compatibility. The properties Tobari needs are narrower and observable: a
bounded semantic version, stable executable-byte identity, the fixed native
login argv, the expected callback behavior, strict authenticated status, and a
strict bounded native-state capture that canonicalizes to Tobari-owned state.

pup supports callback port 8000 and a full callback-URL stdin fallback when its
browser opener fails. Docker port publishing cannot reach a
container-loopback-only listener, while host networking would broaden the
acquisition boundary.

## Decision

`tobari auth login --provider datadog` resolves the requested Context by stable
ID and uses that Context's selected compatible runtime image. It has no host
pup, public-base, or optional-image fallback. If the Context cannot be
resolved, its image fails runtime API validation, or `/usr/local/bin/pup` cannot
satisfy the reviewed contract, login fails with a stable secret-free
`datadog_cli_unavailable` diagnostic stage and directs the user to repair and
build the selected Context runtime.

Before a credential-bearing container exists, Tobari binds the selected image's
immutable ID and starts a no-network, read-only, resource-bounded preflight
container from that exact ID. It accepts bounded semantic pup version syntax
without a compiled version allowlist and computes SHA-256 over the executable
bytes streamed through Docker Engine. Checked preflight cleanup is mandatory.
The login container then uses the same immutable image ID.

The fresh random-name login container has no mounts, volumes, project state,
persistent home, Broker socket, or Docker socket. It runs non-root with all
capabilities dropped, `no-new-privileges`, and fixed CPU, memory, PID, output,
and time bounds. The fixed invocation uses US1, file storage, callback port
8000, and a fixed failing container browser command. Tobari validates pup's
exact HTTPS authorization URL and opens it with the trusted host browser.

Tobari binds a temporary host listener only at `127.0.0.1:8000`. It accepts one
bounded `GET /oauth/callback` with an allowlisted, single-valued query shape,
reconstructs the fixed full loopback URL in memory, and writes it only to the
already-running pup process through Docker exec stdin. The authorization code,
state, and callback URL never enter argv, environment, logs, faults, or durable
state. pup remains responsible for state comparison, PKCE exchange, and OAuth
error interpretation. The browser receives only a fixed secret-free page after
capture succeeds or fails.

After native login, the same login container and executable run `pup auth
status`; Tobari requires an authenticated US1 file-backed result. It copies
exactly the private client, default-token, and session files, validates their
bounded schemas, and canonicalizes only the existing Tobari-owned `PupState`
with the observed executable digest. Successful fixed invocation plus strict
capture is the compatibility contract. Checked listener and container cleanup
are commit preconditions.

Auth Broker refresh and Workspace projection do not change. Broker retains the
real access token, refresh token, and DCR client state, refreshes the same
record after policy allow, and the Workspace continues to receive only
`DD_ACCESS_TOKEN=${HANDLE}` with `DD_SITE=datadoghq.com`. Native pup credential
files never enter the Workspace.

The optional `images/toolbox` artifact, `task toolbox:build`, its scripts,
checks, notices, and active documentation claims are retired. A locally cached
image is external unmanaged state and is not automatically deleted. Existing
normalized Datadog Broker records remain valid.

## Consequences

- Native acquisition follows the same selected-Context ownership model as
  Claude without requiring Claude's exact-version policy.
- Datadog login no longer requires or trusts host pup or a second Tobari image.
- A custom runtime may advance pup versions when the fixed invocation and
  strict status/native-state capture still conform.
- The callback preserves the native browser redirect without host networking,
  published Docker ports, a generic callback service, or manual copying.
- Native file fields remain an acquisition input, but schema drift fails closed
  at capture/state diagnostic stages before replacing the prior credential.

## Mechanical enforcement

- Context/image tests fix stable Context selection, runtime compatibility,
  immutable image use, semantic version bounds, executable digest capture, and
  absence of host/base fallback.
- Container tests fix argv/environment, resource bounds, no mounts, strict
  status and three-file capture, redaction, cleanup, and Broker commit shape.
- Relay tests fix address/path/query bounds, one-callback semantics, stdin-only
  transfer, fixed browser responses, and absence of callback values in output.
- Browser tests accept only the exact Datadog host, path, redirect URI, PKCE
  method, bounded opaque fields, and dynamically observed sorted scope syntax.
- Repository checks reject reintroduction of the retired toolbox task and
  artifact through active build/check surfaces.

## Compatibility and migration

This pre-public V1 change intentionally has no compatibility mode. Existing
encrypted Datadog records remain valid because the canonical Broker record and
refresh contract are unchanged. A new login requires a structurally compatible
pup at `/usr/local/bin/pup` in the selected Context runtime.
