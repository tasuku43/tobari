# Authentication handling

This document defines Tobari's reviewed authentication boundary. ADR 0031
supersedes ADR 0030 only for provider removal and static-only authentication;
managed profiles remain retired.

## Two explicit ownership models

Workspace-owned authentication is the universal default. A tool may run its
own login inside one Workspace and persist its files below that Workspace's
writable home. Tobari does not inherit host credentials, mount a host CLI home,
or claim that tool-owned credentials stay outside the Workspace. Network
authority remains separately controlled by Gateway and OPA.

The optional brokered route keeps one typed static or reviewed renewable
credential record in an encrypted Context vault. A Workspace receives only a
random opaque handle bound to the stable Context, project, provider,
credential revision, and exact HTTPS header/signing plan. Gateway performs
exactly one plan-owned static resolution, token selection/refresh, or bounded
AWS SigV4 result only after OPA allows the ordinary HTTP effect.

Brokered acquisition or import never grants network permission and never
creates a policy rule.

## Supported surface

The closed reviewed built-in set is GitHub, AWS, Datadog, OpenAI/Codex, and
Anthropic/Claude for `auth login`, plus Chatwork through protected stdin
`auth import`. Omitted `--provider` on an interactive trusted-host terminal
opens a bounded selector over installed reviewed login drivers. Explicit
selection remains deterministic:

```sh
tobari auth login --provider github
tobari auth login --provider aws --method identity-center
tobari auth login --provider aws --method console
tobari auth login --provider datadog
tobari auth login --provider openai
tobari auth login --provider anthropic
trusted-secret-source | tobari auth import chatwork
```

Only AWS accepts `--method`; omission selects `identity-center`. GitHub:

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

AWS uses only fixed IAM Identity Center or commercial-console acquisition
flows through a canonical AWS CLI. The encrypted record retains bounded opaque
driver state. After allow, a private authenticated resident companion performs
one compiled AWS credential export and Broker signs the already-authorized,
fully bounded request locally.

Datadog uses fixed `pup --no-agent auth login --site datadoghq.com` acquisition
on the trusted host. Broker selects a valid US1 token or performs one strict
same-record refresh after allow. OpenAI records a stable host Codex version
without allowlisting it, requires the exact compiled
`openai_codex_chatgpt_oauth` device-auth/state contract in an isolated home,
and Broker selects or refreshes the same-account token after allow while returning only its validated
account ID as the supplemental header. Anthropic requires exactly Claude Code
2.1.220 and captures one setup token through a private PTY; its Broker plan is
static and never refreshes.

`auth import PROVIDER` remains the owner extension path. It accepts one bounded
non-empty static primary secret only from protected non-terminal stdin.
Terminal stdin is rejected before reading. Public Context/provider input,
intent, and mutation validation happen before the read; the selected existing
Context, installed static provider, and ready Broker are validated before the
secret is sent.

`auth status` is read-only. `auth logout PROVIDER` removes one local
Context/provider record and revokes all associated handles without claiming
remote provider revocation. Login and import replace the prior record and
rotate every associated handle. Running Workspaces are not rewritten; they
must be re-entered for a new or removed projection.

Managed Gateway profiles, manifest-selected helpers, arbitrary executable
adapters, provider-defined routes, multiple accounts, and compatibility
readers remain unsupported. The dynamic records, refresh, signing,
supplemental header, companion, compiled provider drivers, provider selector, and
AWS method selector exist only in the closed reviewed built-in union above.

## Static provider manifests

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

## Canonical schemas, paths, and backend identifiers

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

## Post-policy request sequence

Gateway enforces this exact order:

1. Derive stable Context/project authority from the kernel-observed source
   endpoint and owner-only principal registry.
2. Reject every malformed, misplaced, ambiguous, copied, stale, revoked, or
   binding-mismatched Tobari-looking handle marker.
3. Remove one recognized handle and ask Broker for non-secret introspection of
   the full Context/project/provider/revision/target/header binding.
4. Redact client authentication and control headers from OPA input and send the
   ordinary normalized HTTP effect to OPA.
5. On deny, stop with zero static resolution, refresh, companion call, signing,
   external DNS, or upstream call.
6. On a static allow, resolve the same revision exactly once and replace only
   the declared destination header.
7. On Datadog/OpenAI allow, select or refresh the same record once and apply
   only the reviewed bearer/supplemental-header result.
8. On AWS allow, retain the complete request within 8 MiB, obtain one
   same-revision companion export, sign locally, and apply only those headers.
9. Make one upstream attempt. Gateway and Broker never replay the application
   request.

Passthrough applies only when no Tobari-looking marker exists. An invalid
marker never falls back or reaches upstream. Gateway never searches request
bodies for handles and never retries.

Opaque handles persist only inside authenticated vault ciphertext; the live
lookup uses SHA-256 hashes. A Workspace can copy its own handle as arbitrary
payload, but that creates no Broker authority. The real primary secret never
enters Workspace state, OPA input, audit, denial evidence, CLI output, or logs.

## Failure and recovery

Locked, unavailable, timed-out, or invalid Broker state fails as
`credential_broker_unavailable` without forwarding. A malformed, stale,
revoked, ambiguous, or mismatched handle fails as
`credential_handle_invalid` without fallback. Auth mutation cancellation or
an unclassified result preserves the standard non-retryable reconciliation
contract: run `auth status` before attempting another mutation. Successful
login/import/logout output is finalized before late cancellation can imply
that replay is safe.

If an AWS companion operation or Datadog refresh may have been dispatched but
its result is unknown, Gateway returns non-retryable
`credential_refresh_outcome_unknown` and makes no application upstream
attempt. Reconcile with trusted-host `auth status`; a durable task barrier
requires explicit login or logout before retry.

## Verification and release evidence

Automated evidence uses synthetic credentials, fake GitHub CLI output, local
servers, fixed clocks, secret canaries, and temporary owner-only state. It
proves locked startup, root-key/vault integrity, project-specific handles,
deny-before-resolution, exact same-revision static replacement, bounded
refresh/signing/companion behavior, durable unknown-outcome barriers, rotation,
revocation, logout, no invalid-handle fallback, source/snapshot equality, and
absence of managed-profile or manifest-selected executable paths.

Before publication, reviewers replay each reviewed host acquisition against a
disposable account without recording a token, code, handle, vault, account
identifier, or authenticated transcript. The GitHub slice includes:

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
the old handle fails. Publication requires `task check`, `task security`,
`task public:check`, and the release gate; none permits storing live secrets as
repository evidence.
