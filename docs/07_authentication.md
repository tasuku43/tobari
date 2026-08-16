# Authentication handling

This document defines Tobari's reviewed authentication boundary. ADR 0044
supersedes the Broker-first standard profile. The Broker implementation remains
an experimental development capability; managed profiles remain retired.

## Standard native Workspace authentication

The standard and release profile has no provider binding, normalized provider
projection, credential handle, vault, root key, companion, Auth Broker service,
or `auth` command. Claude Code, Codex, and other tools run their own supported
login inside a persistent Workspace home. Host credentials and host CLI homes
are never inherited or copied.

A native credential is readable by every process in that Workspace. Gateway
removes authentication and control headers from OPA input and audit evidence,
asks OPA about the ordinary exact HTTP effect, and forwards the original
credential only after allow. Tobari does not identify the process that created
the credential and does not claim per-tool secret isolation inside one
Workspace.

Login creates no policy rule. `builtin/agent-ready` grants only a finite exact
set of client-bootstrap and core effects, independently from account scope or
credential state. It includes these observed native-login effects for the
pinned clients:

| Client | Method | Exact authority and path |
|---|---|---|
| Claude Code | `GET` | `https://platform.claude.com/v1/oauth/hello` |
| Claude Code | `POST` | `https://platform.claude.com/v1/oauth/token` |
| Claude Code | `GET` | `https://api.anthropic.com/api/oauth/claude_cli/roles` |
| Claude Code | `GET` | `https://api.anthropic.com/api/oauth/profile` |
| Codex | `POST` | `https://auth.openai.com/oauth/token` |
| Codex device auth | `POST` | `https://auth.openai.com/api/accounts/deviceauth/usercode` |
| Codex device auth | `POST` | `https://auth.openai.com/api/accounts/deviceauth/token` |
| GitHub CLI device auth | `POST` | `https://github.com/login/device/code` |
| GitHub CLI device auth | `POST` | `https://github.com/login/oauth/access_token` |
| GitHub CLI current-user identity | GraphQL `query` / `viewer` | `https://api.github.com/graphql` |

The compile-time review sources for these exact rules are named
`claude_ready`, `codex_ready`, and `gh_ready` and are coupled to Claude Code
2.1.220, Codex 0.147.0, and GitHub CLI 2.96.0 respectively. Normalization
expands the bundles into ordinary exact rules; bundle and executable names
never enter policy and custom presets cannot select them.

Browser navigation occurs on the host and is not a Workspace egress grant.
Every request outside the immutable baseline remains denied or reviewable under
the selected preset. The policy applies to every process in the Context, never
to an executable name.

For Codex 0.147.0 browser login, the attached `tobari` host process recognizes
one semantically valid strict OpenAI authorization URL from either a bounded
direct line or one complete bounded ANSI synchronized-update frame in a copy
of the unchanged child stream. It accepts only the two exact reviewed direct
CLI and interactive-TUI originators and does not depend on surrounding native
prose, cursor positions, or terminal width. Docker's stdout remains a real raw PTY and its size follows the caller
terminal; Tobari observes the relayed master copy, so ordinary Codex terminal
negotiation, raw input, rendering, and resize remain native. It derives a
non-privileged port from the validated localhost redirect,
binds host `127.0.0.1` only for that login, opens the validated URL, and relays
one opaque browser callback through a fixed Docker exec program plus that port
to Codex's same-port listener on the selected Workspace loopback. Codex owns
state, PKCE, callback parsing, exchange, and credential storage. Tobari neither
logs nor persists the authorization URL or callback bytes. Invalid, duplicate,
ambiguous, or oversized output; another host/path/client/scope; an external or
privileged callback; a port collision; or session end cannot create a lasting
listener or generic host opener. Scope order and bounded state length are not
authority. `codex login --device-auth` is unchanged.

For GitHub CLI 2.96.0, the attached host process recognizes the complete
bounded no-newline browser prompt from both native GitHub.com login paths and
the exact non-interactive fixed-device URL line emitted by Tobari's pinned
compatibility path. The
preferred device flow accepts only the exact
`https://github.com/login/device` target, verifies the selected owned
Workspace, opens it once on the host, and creates no callback listener. Tobari
does not read, retain, relay, validate, or render the one-time code. The prompt
or line supplies only a complete stream boundary.

For exact default argv `gh auth login`, `/usr/local/bin/gh` is a canonical
runtime compatibility wrapper around the pinned real executable. It selects
GitHub.com HTTPS web/device login with GitHub CLI prompting and its Workspace
browser disabled. GitHub CLI therefore displays its one-time code and fixed
device URL, begins polling without Enter, persists its own credential, and then
performs its fixed Git credential setup. Other argv, including explicit
non-default login options, executes the real pinned client unchanged.

When device bootstrap is unavailable, GitHub CLI's web-application fallback
instead requires the independent strict
`https://github.com/login/oauth/authorize` contract:
fixed client `178c6fc778ccc68e1d6a`, required `repo`, `read:org`, and `gist`
scopes, optional `workflow`, exact 20-character lowercase hexadecimal state,
and an HTTP `127.0.0.1:<non-privileged-port>/callback` redirect. Unknown query
keys, caller-added scopes, SSH-key-upload scope, and GitHub Enterprise hosts
open nothing. Tobari opens the validated URL and relays one opaque callback to
the same port in the exact selected Workspace. GitHub CLI still owns state
validation where applicable, code exchange, credential persistence, and result
presentation. `gh_ready` grants only the two exact device bootstrap/exchange
POSTs and the exact GraphQL `query` / `viewer` current-user lookup above. Its
declared GraphQL endpoint does not grant ordinary HTTP, mutation, sibling roots,
or mixed-root documents. Authorization-browser GETs occur on the host, while
every other GitHub API, Git transport, repository, download, upload, release,
self-update, and unmatched effect continues through ordinary Context policy.

Claude native login is a release regression because the previous standard
projection allowed token exchange and then rejected Claude's authenticated
`GET /api/oauth/profile` as `broker_auth_required`. That made
`subscriptionType` and `rateLimitTier` null and caused `/status` to report
`Claude API account`. Standard conformance now proves the profile request and
the other login effects take the ordinary passthrough route, that their bearer
value is absent from OPA input, and that it is preserved for upstream only
after allow. Codex's browser callback and device-code variants have the same
post-policy passthrough proof.

`context show` reports `authentication.mode` as `native_workspace`. The agent
CLI itself owns credential status, refresh, logout, account metadata, and error
presentation. Deleting the Workspace removes its native credential state;
switching or recreating a Workspace requires native login there.

## Experimental Broker profile

`task build:dev` compiles the legacy reviewed Broker capability and its `auth`
commands behind `tobari_experimental`. It uses a three-service Compose override
and an experimental Gateway layer. No environment variable or runtime flag can
activate it in a standard binary, and it is not supported or published.

The experimental profile accepts:

```sh
tobari-dev auth login --provider aws --method identity-center
tobari-dev auth login --provider aws --method console
```

Only AWS accepts `--method`; omission selects `identity-center`. The remaining
sections describe this experimental profile only. GitHub:

All reviewed helper lookups inspect a finite PATH-ordered candidate set. A
temporary integration shim may shadow a conventional installation for ordinary
shell use, but Tobari never executes that shim during credential acquisition;
it selects only the first later candidate whose canonical executable passes
the existing trusted-root and mode checks.

- resolves one canonical GitHub CLI executable outside the project;
- checks that it is not group/world-writable and hashes it before the flow;
- uses a sanitized environment and private temporary `GH_CONFIG_DIR`;
- runs fixed API-authentication-only argv and requests no Git protocol;
- opens only `https://github.com/login/device`, retaining manual fallback;
- captures one bounded token without printing it;
- rechecks the executable digest and performs checked private-home cleanup
  before committing the token.

Exact GitHub CLI product-version equality is not a security boundary. The
fixed observed command contract and executable identity are.

Experimental AWS uses only fixed IAM Identity Center or commercial-console acquisition
flows through a canonical AWS CLI. The encrypted record retains bounded opaque
driver state. After allow, a private authenticated resident companion performs
one compiled AWS credential export and Broker signs the already-authorized,
fully bounded request locally.

Datadog uses fixed `pup --no-agent auth login --site datadoghq.com` acquisition
in a fresh mount-free container from the selected Context image. Tobari binds
the immutable image and executable digest, accepts bounded semantic version
syntax without an exact version allowlist, bridges one validated localhost
callback only to pup stdin, and requires strict native status and state capture.
Broker selects a valid US1 token or performs one strict same-record refresh
after allow. OpenAI records a stable host Codex version
without allowlisting it, requires the exact compiled
`openai_codex_chatgpt_oauth` native-browser/state contract in an isolated home.
The verified Codex child owns its loopback listener, dynamic authorization URL,
browser request, PKCE state, callback, and exchange. Tobari binds no listener,
never parses or opens that URL, and preserves Codex's bounded manual fallback
guidance. Its bounded visible stream maps only the reviewed
Codex reset, muted, and accent SGR sequences to Tobari-owned terminal styles;
`NO_COLOR` emits the same guidance without styling and unknown controls remain
visibly projected. Broker selects or refreshes the same-account token after
allow while returning only its validated
account ID as the supplemental header. Anthropic requires exactly Claude Code
2.1.220 at `/usr/local/bin/claude` in the selected Context image. A fresh
mount-free login container runs native account login, Tobari opens only the
validated authorization URL, extracts only access token, refresh token, expiry,
the dynamically granted scope set, subscription type, and rate-limit tier from
Linux state, validates bounded OAuth
scope-token syntax, requires that grant to be a subset of the observed request,
normalizes provider ordering, structurally bounds the two non-secret
entitlement labels without compiling provider values, and discards every other
provider-owned optional field before checked cleanup. Broker stores its own strict record and
selects or refreshes its bearer with the fixed reviewed client and persisted
scope set after allow. Tobari compiles no Claude scope-name catalog.

Workspace projection reproduces the scope and entitlement labels for the exact
pinned client, uses the project-bound handle as `accessToken`, and uses the
fixed public `dummy-value` as a local refresh-presence sentinel. That sentinel
is not accepted as a Broker handle and cannot refresh the provider session;
the actual renewable token never leaves Broker.

The same reviewed Workspace projection merges only
`hasCompletedOnboarding: true` into Claude's private top-level state. This
closes the interactive first-run screen after `claude auth status` is already
authenticated without copying account metadata or making the mutable file a
credential source.

`auth import PROVIDER` remains the owner extension path. It accepts one bounded
non-empty static primary secret only from protected non-terminal stdin.
Terminal stdin is rejected before reading. Public Context/provider input,
intent, and mutation validation happen before the read; the selected existing
Context, installed static provider, and ready Broker are validated before the
secret is sent.

Experimental `auth status` is read-only and reports `declared_bindings: broker_required`
and `undeclared_bindings: workspace_owned_compatibility` beside Broker,
provider, and Workspace activation state. `context show` reports the same
routing contract in its authentication section and points to `auth status` for
per-Workspace re-entry guidance. `auth logout PROVIDER` removes one local
Context/provider record and revokes all associated handles without claiming
remote provider revocation. Login and import replace the prior record and
rotate every associated handle. Running Workspaces are not rewritten; they
must be re-entered for a new or removed projection.

Managed Gateway profiles, manifest-selected helpers, arbitrary executable
adapters, provider-defined routes, multiple accounts, and compatibility
readers remain unsupported. The dynamic records, refresh, signing,
supplemental header, companion, compiled provider drivers, provider selector, and
AWS method selector exist only in the compiled implementation union above;
the experimental projection cannot select capabilities outside that compiled
union.

## Experimental runtime credential classes

All declared bindings are Broker-required, but their post-policy behavior is
not interchangeable:

| Class | Providers | Broker-held state | Post-allow use | Expiry and recovery |
|---|---|---|---|---|
| Static replacement | GitHub, Chatwork, owner static providers | Primary secret | Replace one exact header once | Broker cannot refresh; replace with `auth login`/`auth import` when invalid |
| Renewable session | Datadog, OpenAI, Anthropic | OAuth session state | Select a valid bearer value or refresh at one fixed endpoint, persist the new state, then apply it | Broker refreshes when possible; an invalid grant or durable unknown outcome requires trusted-host reconciliation and usually re-login |
| Request signing | AWS (experimental profile) | Reviewed login/session state | Obtain bounded temporary credentials and sign the exact already-authorized request | Broker/companion renew temporary state; unknown dispatch outcome is not replayed automatically |

These classes describe runtime use, not acquisition. `builtin_helper` and
`stdin_import` describe how state enters the Broker; they do not say whether
that state is static, renewable, or request-signing.

## Experimental static provider manifests

Owner manifests are strict owner-only schema-V1 non-secret, non-executable
local data. A manifest may declare:

- one provider ID that does not replace a built-in;
- protected-stdin static-primary-secret import;
- bounded handle projection;
- one or more exact HTTPS target, source-header, source-format, and destination-
  header transformations.

Unknown fields and versions fail closed. Wildcards, IP/private destinations,
shell, helpers, executable paths, argv, environment selection, OAuth, refresh,
signing, supplemental headers, arbitrary methods/routes, policy, include,
inheritance, remote fetch, provider business operations, and multiple accounts
are unsupported. Overlapping recognition coordinates reject the complete
provider projection as `ambiguous_provider_http_binding`; partial authority is
never activated.

## Experimental canonical schemas, paths, and backend identifiers

All Tobari-owned authentication schemas and component APIs are exactly V1.
Readers reject every other version without migration or fallback.

| State or protocol | Version | Canonical location or value |
|---|---:|---|
| Owner provider manifest | schema 1 | `${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/<provider>.json` |
| Normalized provider projection | schema 1 | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/projection/providers.json` |
| Encrypted Context vault | envelope 1, payload 1 | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/contexts/<context-id>/vault.enc` |
| Workspace auth registry | schema 1 | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/projects` |
| Linux root key | 32 bytes | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/keys/root.key` |
| macOS root key | 32 bytes | Keychain service `io.tobari.auth-root.v1`, account `tobari` |
| Public root-key backend | enum | `macos_keychain`, `xdg_file`, or observation-only `unavailable` |
| Broker control socket | NDJSON schema 1 | `/run/tobari-auth/control/broker.sock` |
| Broker runtime socket | NDJSON schema 1 | `/run/tobari-auth/runtime/broker.sock` |
| Private companion socket | framed schema 1 | `/run/tobari-auth/companion/bridge.sock` |

`linux_xdg_file` is an infrastructure/doctor label, not a public JSON enum.
The Context manifest contains no vault path, root key, primary secret, or
handle. Stable Context ID selects the separate encrypted vault.

Vaults use AES-256-GCM with a random 12-byte nonce and associated data binding
schema plus stable Context ID. Root keys stay in host storage and broker
memory. A missing key alongside a vault is never replaced automatically. The
Broker starts locked after every restart and is unlocked through fixed control
stdin only after exact container identity and control readiness are verified.

The Broker is non-root, has no TCP listener, joins no Workspace network, and
contains no provider CLI. It has bounded egress only for its compiled Datadog
and OpenAI refresh transports. Gateway alone mounts the runtime socket
read-only. The AWS companion is a private authenticated reverse session, not a
host listener or Workspace mount. Control and runtime frames are strict 64 KiB
schema-1 NDJSON; key and credential payload bytes follow their declared length
and never use argv or environment.

## Experimental post-policy request sequence

Gateway enforces this exact order:

1. Derive stable Context/project authority from the kernel-observed source
   endpoint and owner-only principal registry.
2. Match the normalized request against the host-owned declared header and
   signing bindings. At a declared binding, reject a real Workspace credential
   as non-learnable `broker_auth_required` before OPA or external I/O.
3. Reject every malformed, misplaced, ambiguous, copied, stale, revoked, or
   binding-mismatched Tobari-looking handle marker.
4. Remove one recognized handle and ask Broker for non-secret introspection of
   the full Context/project/provider/revision/target/header binding.
5. Only when no declared binding and no Tobari-looking marker matches, select
   Workspace-owned compatibility passthrough.
6. Redact client authentication and control headers from OPA input and send the
   ordinary normalized HTTP effect to OPA.
7. On deny, stop with zero static resolution, refresh, companion call, signing,
   external DNS, or upstream call.
8. On a static allow, resolve the same revision exactly once and replace only
   the declared destination header.
9. On Datadog/OpenAI/Anthropic allow, select or refresh the same record once and apply
   only the reviewed bearer/supplemental-header result.
10. On AWS allow, retain the complete request within 8 MiB, obtain one
   same-revision companion export, sign locally, and apply only those headers.
11. Make one upstream attempt. Gateway and Broker never replay the application
   request.

Compatibility passthrough applies only when no declared binding and no
Tobari-looking marker exists. An invalid marker never falls back or reaches
upstream. Gateway never searches request bodies for handles and never retries.

Opaque handles persist only inside authenticated vault ciphertext; the live
lookup uses SHA-256 hashes. A handle is not the primary secret, but it is a
scoped bearer capability and should not be published or logged. Copying it to a
different Workspace or binding does not create Broker authority. For declared
bindings, the real primary secret never enters Workspace state, OPA input,
audit, denial evidence, CLI output, or logs.

## Experimental failure and recovery

Locked, unavailable, timed-out, or invalid Broker state fails as
`credential_broker_unavailable` without forwarding. A malformed, stale,
revoked, ambiguous, or mismatched handle fails as
`credential_handle_invalid` without fallback. Auth mutation cancellation or
an unclassified result preserves the experimental non-retryable reconciliation
contract: run `auth status` before attempting another mutation. Successful
login/import/logout output is finalized before late cancellation can imply
that replay is safe.

A real credential at a declared binding fails as `broker_auth_required`.
Running policy review cannot recover it. Inspect `auth status`, configure the
provider with its reviewed `auth login` or protected `auth import` route, and
re-enter an affected Workspace when its handle projection is missing or stale.

If an AWS companion operation or Datadog refresh may have been dispatched but
its result is unknown, Gateway returns non-retryable
`credential_refresh_outcome_unknown` and makes no application upstream
attempt. Reconcile with trusted-host `auth status`; a durable task barrier
requires explicit login or logout before retry.

## Experimental verification evidence

Automated evidence uses synthetic credentials, fake GitHub CLI output, local
servers, fixed clocks, secret canaries, and temporary owner-only state. It
proves locked startup, root-key/vault integrity, project-specific handles,
deny-before-resolution, exact same-revision static replacement, bounded
refresh/signing/companion behavior, durable unknown-outcome barriers, rotation,
revocation, logout, no invalid-handle fallback, source/snapshot equality, and
absence of managed-profile or manifest-selected executable paths.

Experimental manual validation may replay each reviewed host acquisition
against a disposable account without recording a token, code, handle, vault,
account identifier, or authenticated transcript. The GitHub slice includes:

```sh
tobari auth login --provider github --context default
tobari auth status --context default --format json
# Re-enter the Context-bound Workspace.
case "${GH_TOKEN-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$(gh auth token --hostname github.com)" = "$GH_TOKEN"
gh api user --jq .login >/dev/null
tobari auth logout github --context default --format json
```

The reviewer records pass/fail and secret-free observations only, then proves
the old handle fails. This evidence never authorizes or accompanies standard
publication; no gate permits storing live secrets as repository evidence.
