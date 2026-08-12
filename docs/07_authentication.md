# Authentication handling

This document defines Tobari's first-public-V1 authentication boundary. The
security and product authority is ADR 0030 together with the project theses.

## Two explicit ownership models

Workspace-owned authentication is the universal default. A tool may run its
own login inside one Workspace and persist its files below that Workspace's
writable home. Tobari does not inherit host credentials, mount a host CLI home,
or claim that tool-owned credentials stay outside the Workspace. Network
authority remains separately controlled by Gateway and OPA.

The optional brokered route keeps one static primary secret in an encrypted
Context vault. A Workspace receives only a random opaque handle bound to the
stable Context, project, provider, credential revision, exact HTTPS target,
source header, and source format. Gateway resolves that secret exactly once
only after OPA allows the ordinary HTTP effect.

Brokered acquisition or import never grants network permission and never
creates a policy rule.

## Supported V1 surface

The sole reviewed built-in pairing is GitHub.com with GitHub CLI (`gh`).
`auth login` requires exactly `--provider github`; provider omission and every
other provider fail before acquisition. The driver:

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

First public V1 has no brokered AWS, Datadog, OpenAI, Anthropic, or Chatwork
built-in; managed Gateway profile; dynamic credential record; refresh; task
barrier; signer; supplemental header; resident companion; private companion
protocol; exact-client-version driver; provider menu; or `--method` selector.
Their tools may still use Workspace-owned login. Reintroducing any retired path
requires a new thesis, trust-boundary, catalog, state, dependency, and release
decision.

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

`linux_xdg_file` is an infrastructure/doctor label, not a public JSON enum.
The Context manifest contains no vault path, root key, primary secret, or
handle. Stable Context ID selects the separate encrypted vault.

Vaults use AES-256-GCM with a random 12-byte nonce and associated data binding
schema plus stable Context ID. Root keys stay in host storage and broker
memory. A missing key alongside a vault is never replaced automatically. The
Broker starts locked after every restart and is unlocked through fixed control
stdin only after exact container identity and control readiness are verified.

The Broker is non-root, has no TCP listener, joins no Workspace network, and
contains no provider CLI. Gateway alone mounts the runtime socket read-only.
Host commands use fixed bounded control operations. Control and runtime frames
are strict 64 KiB schema-1 NDJSON; key and secret payload bytes follow their
declared length and never use argv or environment.

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
5. On deny, stop with zero Broker resolution, external DNS, or upstream call.
6. On allow, resolve the same revision exactly once, replace only the declared
   destination header, and make one upstream attempt.

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

## Verification and release evidence

Automated evidence uses synthetic credentials, fake GitHub CLI output, local
servers, fixed clocks, secret canaries, and temporary owner-only state. It
proves locked startup, root-key/vault integrity, project-specific handles,
deny-before-resolution, exact same-revision replacement, rotation, revocation,
logout, no invalid-handle fallback, source/snapshot equality, and absence of
managed/dynamic/refresh/signing/companion/exact-version code and dependencies.

Before publication, one reviewer performs a live disposable GitHub acquisition
without recording a token, code, handle, vault, or authenticated transcript:

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
