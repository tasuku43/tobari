# ADR 0025: Add closed Codex and Claude OAuth credential plans

- Status: Accepted
- Date: 2026-08-10
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O,
  harness, public boundary, and release
- Extends: [ADR 0020: Add a host credential companion for renewable brokered credentials](0020-broker-reviewed-credential-plans.md)
- Revised by: ADR 0027 places both plans and component APIs inside exact V1 and
  withholds official image authority until reviewed V1 indexes exist

## Context

The maintainer's default local Context acceptance recipe contains Codex 0.146.0
and Claude Code 2.1.220, but a Workspace cannot use either tool's account OAuth
without receiving provider credentials or bypassing Auth Broker. That combined
recipe and its version-command observation are machine-local acceptance
evidence, not a shipped default or public combined image; the repository keeps
the two agent runtime recipes as separate local/CI build-only artifacts. Static
OpenAI and Anthropic API-key manifests authenticate different products and do
not close the native account-login outcome.

Neither ambient host credential stores nor a user-controlled Context image are
trusted acquisition authorities. General OAuth configuration, arbitrary
helpers, and raw auth-file copying would also weaken the closed post-policy
credential boundary.

The pinned clients expose different safe acquisition contracts. Codex has a
renewable ChatGPT session in an isolated file credential store. Claude's
portable documented export is `setup-token`: an inference-only OAuth token
with a one-year requested lifetime and no refresh token. Claude's full login
store is private and platform-dependent.

Codex 0.146.0 can consume a handle without parsing it through the external-host
`chatgptAuthTokens` auth-file shape. Upstream labels that interface unstable
and internal. It is therefore usable only as an explicitly version-pinned
compatibility shim, not as an upstream stability claim.

## Decision

Add two fixed built-in pairings selected through the existing command:

```text
tobari auth login --provider openai
tobari auth login --provider anthropic
```

The `openai` schema-2 plan uses helper `codex-chatgpt-oauth` and credential kind
`openai_codex_oauth_session`. Its trusted-host driver accepts only Codex
0.146.0, runs fixed `login --device-auth` argv with only
`cli_auth_credentials_store="file"` and
`check_for_update_on_startup=false` configuration overrides in an owner-only
temporary home, validates the executable before and after, strictly parses
only the reviewed managed `auth.json`, deletes the temporary home, and commits
canonical encrypted OAuth state. It neither reads nor modifies ambient Codex
state.

The canonical driver state is schema 1 and contains only the absolute
executable path, its SHA-256 digest, exact version `0.146.0`, `auth_mode` equal
to `chatgpt`, a null `OPENAI_API_KEY`, the ID/access/refresh tokens, one account
ID, and an RFC3339Nano `last_refresh`. The namespaced ID-token account claim is
mandatory and must equal the stored account ID; FedRAMP state is rejected. The
executable digest is the credential driver revision, so an executable change
cannot silently reuse captured state.

Workspace projection is this one complete `.codex/auth.json` compatibility
shim, where `${HANDLE}` is the project-bound Tobari handle rather than an
OpenAI token:

```json
{"auth_mode":"chatgptAuthTokens","OPENAI_API_KEY":null,"tokens":{"id_token":"e30.e30.x","access_token":"${HANDLE}","refresh_token":"","account_id":null},"last_refresh":"1970-01-01T00:00:00Z"}
```

Codex must be exactly 0.146.0. Automated contracts prove exact projection
bytes, Gateway handle recognition and bearer/account injection, and version or
schema drift rejection. A recorded isolated network-disabled observation also
confirmed login-status recognition and verbatim handle placement, but that
client compatibility observation remains a manual release replay rather than
a redistributable automated artifact claim. A future Codex version is
unsupported until this shim is re-reviewed or replaced by an
upstream-supported external credential surface.

The exact request binding is HTTPS `chatgpt.com:443`, source and destination
`Authorization: Bearer`, with `Authorization`, `ChatGPT-Account-ID`, and
`X-OpenAI-FedRAMP` treated as credential-sensitive headers. The first slice is
limited to the normal ChatGPT authority and rejects FedRAMP state. Gateway
removes caller-supplied sensitive headers, performs non-secret introspection,
asks OPA about the ordinary HTTP effect, and only after allow asks Broker to
resolve the same revision. Broker returns one access token and the validated
non-secret account routing value; Gateway injects both for one upstream
attempt.

Broker uses an access token only while more than five minutes remain. Otherwise
it takes the per-record single-flight lock, persists an encrypted no-replay
barrier, and performs one proxy-free, no-redirect JSON POST to the
Codex-0.146-reviewed endpoint `https://auth.openai.com/oauth/token`. The
canonical body crosses only stdin to a fixed isolated Python worker. Broker
starts the deadline before spawn, supplies no ambient environment, and kills
and reaps the worker at the 30-second wall-clock bound; the worker's own socket
bound is 29 seconds. The compiled request uses the fixed public Codex client ID
`app_EMoamEEZ73f0CkXaXp7hrann` and refresh-token grant.
Broker strictly validates the response, preserves omitted token fields,
revalidates account identity, atomically persists the replacement state while
preserving the grant revision, and only then returns a credential. An uncertain
outcome leaves the barrier and requires explicit OpenAI re-login or logout.
This is a closed implementation derived from the pinned Apache-2.0 official
client source, not manifest-selected OAuth behavior.

The `anthropic` built-in remains a schema-1 `primary_secret` header plan but
uses reviewed helper `claude-setup-token`. Its trusted-host driver accepts only
Claude Code 2.1.220 and runs exactly `claude setup-token` in an owner-only
temporary home with a sanitized environment. A bounded PTY parser holds every
provider byte until the complete terminal frame is validated, then emits only
fixed recognized non-secret instructions and captures exactly one printable
token from the fixed success frame. The token is never written to visible
output. A raw, erased/redrawn, or chunk-split token echo, ambiguous framing,
unknown controls, multiple candidates, executable change, cancellation,
timeout, or cleanup failure prevents commit.

Claude projects only `CLAUDE_CODE_OAUTH_TOKEN=${HANDLE}`. It binds
exactly HTTPS `api.anthropic.com:443`, source and destination
`Authorization: Bearer`. It does not project `ANTHROPIC_API_KEY` or
`ANTHROPIC_AUTH_TOKEN`. Broker resolves the stored token only after OPA allow;
there is no refresh operation. The supported client outcome is first-party
inference and local MCP. Remote Control and claude.ai connectors are outside
this plan. Re-login rotates the token before expiry or after a provider 401.

Both helpers require canonical non-group/world-writable host executables from
the existing conventional non-project trusted installation roots. The host
must therefore provide `codex --version` as exactly `codex-cli 0.146.0` and
`claude --version` as exactly `2.1.220 (Claude Code)` before the corresponding
login can start. Codex persists the observed executable digest as the dynamic
credential driver revision; Claude rechecks the same digest before returning
the captured static token but does not persist executable metadata in that
static vault record. A Workspace binary, `PATH` entry from the project,
ambient provider home, or a newer client is not a fallback acquisition
authority.

`auth logout` retains its existing local-only contract: it atomically removes
the Context/provider record and handles without contacting OpenAI or Anthropic
or claiming remote revocation.

Gateway image API advances from 3 to 4 and Auth Broker image API advances from
2 to 3 because older images cannot safely interpret the new dynamic credential
and supplemental-header response. The normalized provider schema stays version
2: it is already the closed reviewed built-in union, and unknown credential
kinds fail closed. Owner schema 1 and existing GitHub, AWS, Datadog, and static
records remain readable. These are canonical-source and local-development API
versions; the already published immutable Gateway API-3 and Auth Broker API-2
digests remain historical facts and do not acquire this capability.

## Consequences

- Workspaces receive no OpenAI or Anthropic token, refresh token, identity
  token, authorization code, or provider credential file.
- Auth Broker gains one fixed OpenAI refresh destination. It still contains no
  Codex or Claude executable and no provider home.
- Claude setup tokens are long-lived but not renewable. Their displayed
  one-year lifetime is a client-requested estimate, not a provider-issued
  server expiry claim.
- Codex compatibility is intentionally pinned to 0.146.0 and inherits the risk
  of an upstream-internal input shape. Version drift fails closed.
- Multiple accounts, arbitrary scopes/endpoints, custom ChatGPT authorities,
  remote revocation, and generic OAuth remain unsupported.
- Runtime and image validation for this capability remains contributor-local.
  The Codex and Claude runtime recipes do not publish agent tags until each
  artifact's license and redistribution review is complete, and the API-4/API-3
  Gateway/Broker images are not routine authority until reviewed immutable
  multi-architecture digests replace the historical pins.

## Mechanical enforcement

- Domain and Gateway tests accept only the two exact provider/helper/
  projection/binding plans and reject owner attempts, alternate authorities,
  headers, formats, token destinations, or client versions.
- Host-driver tests fix executable identity, version, argv, environment,
  bounded output/state, private modes, strict parsing, cleanup, cancellation,
  and redaction without real provider calls.
- Broker tests prove state canonicalization, account continuity, no refresh for
  a valid token, exact refresh request, same-revision atomic replacement,
  durable outcome-unknown behavior, static Claude resolution, and local-only
  logout.
- Gateway tests prove all handle and account-routing material is removed before
  OPA, denial performs zero resolution, and only an allowed request receives
  the reviewed OpenAI supplemental header or Claude bearer.
- Exact projection and Gateway tests prove the handle-only client shape and
  post-policy replacement. The recorded isolated pinned-client observation is
  replayed manually for release to confirm verbatim authorization placement
  and absence of Workspace-side Codex refresh.
- Canonical Auth Broker and Gateway Python remain byte-equal to embedded
  runtime snapshots. Automated tests use only synthetic tokens, JWT claims,
  fixed clocks, fake CLIs, and intercepted local request fixtures. Real OAuth,
  the documented OpenAI near-expiry refresh, Claude rotation, logout, and
  stale-handle rejection remain manual release checks and never provide
  repository fixtures. Natural provider expiry and provider-generated 401
  recovery are not claimed as completed release evidence without a separate
  executable manual procedure.

## Validation

- `task check`
- `task security`
- `task public:check` before public publication
- `task release:check` after redistribution review and reviewed image pins
- Manual login for both providers with disposable accounts, Workspace
  re-entry, policy denial, allowed inference, OpenAI refresh, Claude rotation,
  logout, and stale-handle rejection.
