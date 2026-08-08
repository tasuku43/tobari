# Authentication handling

Tobari supports three explicit authentication paths without inheriting host
authentication state:

- tool-native passthrough remains the universal default for a tool that logs in
  inside its own Workspace home;
- the Auth Broker stores one host-acquired credential per Context/provider and
  gives each eligible Workspace only a project-bound opaque handle; and
- the retained `managed` Gateway adapter supports the earlier static
  profile/file compatibility contract.

There is no implicit fallback between these paths. Fallback is selected only
when the request contains no Tobari broker-handle marker in any inspected URL
or header position. A valid handle uses the broker route. Any Tobari-looking
marker, including a malformed, misplaced, ambiguous, or binding-mismatched
value, fails closed as `credential_handle_invalid`; it is never forwarded and
never falls back to the passthrough or managed adapter.

## Public Auth Broker commands

The public Catalog owns four commands:

```text
tobari auth login <provider> [--context NAME] [--format text|json]
secret-source-command | tobari auth import <provider> [--context NAME] [--format text|json]
tobari auth status [--context NAME] [--format text|json]
tobari auth logout <provider> [--context NAME] [--format text|json]
```

`--context` selects an existing Context; omission uses the current/default
Context without changing it. Login, import, and logout are command-bound
fixed-target writes to the installation's Context-scoped credential catalog.
Status is read-only and returns the complete installed provider collection for
one Context. Every JSON result uses envelope `auth` and schema version 1.
Outputs contain stable Context identity, provider state, a secret-free account
label when available, an opaque credential revision, root-key storage backend,
broker state, and explicit Workspace activation guidance. They never contain a
root key, vault bytes, raw credential, or Workspace handle.

Login is currently supported only for the built-in `github` provider and
requires an interactive terminal. The trusted host executes the pinned GitHub
CLI inside the Auth Broker container with an ephemeral
`GH_CONFIG_DIR=/run/tobari-auth/login`. It runs the ordinary GitHub.com web
login with GitHub CLI prompting and container browser launch disabled. When the
fixed device page appears in the helper stream, the host CLI opens exactly
`https://github.com/login/device` through the platform opener. If that is
unavailable, the same visible URL is the manual next action and login continues.
The helper requests no Git protocol, performs no Git credential setup, verifies
the active account through bounded JSON output, captures the token internally,
and destroys the temporary CLI state. Its expected plaintext fallback exists
only in the private tmpfs and is withheld from public output because it is not
persistent credential storage. The real account label may appear in the
secret-free result. The credential itself never crosses CLI stdout/stderr,
argv, environment, or a Workspace.

Import is available only to installed providers whose manifest declares
`stdin_import`. Replace `secret-source-command` with a trusted password-manager
or equivalent no-echo source. Interactive terminal stdin is rejected before a
byte is read. For non-terminal stdin, Tobari reads one non-empty opaque
credential of at most 32 KiB only after public Context/provider argument,
intent, and mutation validation. Infrastructure then validates the selected
existing Context, installed provider and acquisition mode, and broker readiness
before sending the credential to the broker. The command never accepts the
credential as an argument, flag, or Tobari environment value, and an imported
credential has no account label.

Logout removes the local Context/provider credential and atomically revokes all
of its issued handles. It does not contact the provider or promise remote token
revocation. Login, import, replacement, and logout cannot rewrite the
environment of an already-running process, so their result always directs the
user to leave and re-enter affected Workspaces.

Credential ownership and eligibility are exact: one credential belongs to one
Context/provider, and every project permanently bound to that Context is
eligible for its own distinct handle. No handle is pushed into a running
session. A project receives the current handle only on its next matching
Workspace entry/reconciliation. After replacement or logout, an already-running
session may retain an old projected handle, but that handle is revoked and
fails as `credential_handle_invalid`. On the next matching entry, replacement
projects the new revision; logout removes the declared environment projection
by recreation and removes only unchanged Tobari-owned complete files.

## Shared locked broker and encrypted Context vaults

One installation-local `tobari-auth-broker` service is shared by every Context.
It runs as a fixed non-root user, has no TCP listener, never joins a Workspace
network, and exposes two owner-only Unix sockets:

- `/run/tobari-auth/control/broker.sock` for bounded host control operations;
- `/run/tobari-auth/runtime/broker.sock` for Gateway introspection and
  post-authorization resolution.

OPA mounts neither socket. Only Gateway mounts the runtime socket. Public CLI
commands do not expose either protocol; infrastructure invokes fixed control
operations inside the broker container. Both protocols use strict schema-1
newline-delimited JSON frames of at most 64 KiB. Root-key and import payloads
follow their JSON frame as length-bound raw stdin bytes.

The broker starts locked after every creation or restart. `cluster up` reads or
creates one 32-byte installation root key and sends it through stdin to unlock
the broker. On macOS, the key is a generic Keychain password with service
`io.tobari.auth-root.v1` and account `tobari`. On Linux it is the exact
owner-only XDG state file:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/keys/root.key
```

Linux XDG storage protects the container/Workspace boundary but does not claim
security against compromise of the host user. On either platform, a missing
root key while an encrypted vault exists is a recovery fault; Tobari never
silently generates a replacement that would orphan or disguise existing
state.

Each stable Context owns one file:

```text
${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/contexts/<context-id>/vault.enc
```

Vault schema 1 uses AES-256-GCM, a random 12-byte nonce, and authenticated data
that binds the schema and stable Context ID. The file is written through an
owner/mode/symlink-checked atomic fsync-and-rename boundary. A provider has at
most one active opaque `primary_secret` in one Context. Credential and handle
records have random secret-free revisions. Raw handles are durable only inside
the encrypted vault; the live broker index stores their SHA-256 hashes.

For the same Context, project, provider, credential revision, and normalized
bindings, ordinary Workspace reconciliation and broker restart/unlock retain
the same handle. Credential replacement or logout deletes every handle for that
Context/provider. A copied handle remains unusable from another project or
Context even before policy evaluation.

`cluster down` and `cluster down --purge` preserve encrypted Context vaults and
the installation root key. Purge additionally removes only the shared CA
volumes; it is not an authentication reset. A later `cluster up` reuses and
unlocks the preserved state. Use the exact `auth logout <provider>` command to
remove one Context/provider credential and revoke its handles.

## Canonical schemas, paths, and backend identifiers

The following identifiers are compatibility contracts. Paths use the effective
XDG roots; macOS Keychain storage has no filesystem path.

| Surface or state | Schema/version | Canonical path, endpoint, or value |
|---|---|---|
| Public `auth` JSON result | envelope `auth`, schema `1` | `storage_backend` is exactly `macos_keychain` or `xdg_file` |
| Public cluster status | schema `3` | `root_key_backend` is `macos_keychain`, `xdg_file`, or diagnostic `unavailable` |
| Public Context report | schema `4` | Secret-free broker/provider state; no vault path/content, root key, primary secret, or handle |
| Provider manifest | `tobari.auth-provider.v1`; `schema_version: 1` | `${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/*.json` |
| Normalized provider projection | schema `1` | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/projection/providers.json` |
| Encrypted Context vault | schema `1` | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/contexts/<context-id>/vault.enc` |
| Linux installation root key | raw 32 bytes | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/keys/root.key` |
| macOS installation root key | raw 32-byte generic password | Keychain service `io.tobari.auth-root.v1`, account `tobari` |
| Project authentication registry | schema `1` | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/projects/<project-id>.json` |
| Broker control protocol | schema `1` | `/run/tobari-auth/control/broker.sock` |
| Broker runtime protocol | schema `1` | `/run/tobari-auth/runtime/broker.sock` |
| Workspace handle | version `1` | prefix `tobari-h1_` |

`linux_xdg_file` is an infrastructure/doctor diagnostic label only. It is not
a permitted public `auth.storage_backend` or successful cluster
`root_key_backend` value; the public Linux value is `xdg_file`.

## Provider manifest boundary

Built-in and owner-controlled providers share one strict schema-1 parser. A
manifest declares:

- a stable provider ID and secret-free display name;
- `builtin_helper` or `stdin_import` acquisition;
- exactly one opaque `primary_secret`;
- bounded Workspace environment or complete-file templates containing a
  `${HANDLE}` placeholder; and
- exact HTTPS target, source header/syntax, destination header/transformation,
  and secret-header redaction names.

The public schema identifier is `tobari.auth-provider.v1`; its on-disk
discriminator is `schema_version: 1`. This synthetic user-provider manifest
shows every v1 responsibility and the exact field spelling:

```json
{
  "schema_version": 1,
  "id": "example-token",
  "display_name": "Example API",
  "acquisition": {"mode": "stdin_import"},
  "credential": {"kind": "primary_secret"},
  "workspace_projections": [
    {
      "kind": "env",
      "name": "EXAMPLE_TOKEN",
      "template": "${HANDLE}"
    },
    {
      "kind": "complete_file",
      "path": ".config/example/auth.toml",
      "template": "provider = \"${PROVIDER_ID}\"\ntoken = \"${HANDLE}\"\n"
    }
  ],
  "header_bindings": [
    {
      "target": {
        "scheme": "https",
        "host": "api.example.com",
        "port": 443
      },
      "source": {
        "header": "x-api-key",
        "formats": ["raw"]
      },
      "destination": {
        "header": "x-api-key",
        "format": "raw",
        "secret_field": "primary_secret"
      },
      "secret_headers": ["x-api-key"]
    }
  ]
}
```

Unknown and duplicate fields are errors. An unsupported `schema_version`
fails the whole provider collection before activation; Tobari does not guess,
partially load, or automatically rewrite a manifest. A future schema requires
an explicit migration and compatibility decision.

A manifest cannot contain a secret, command, shell fragment, executable path,
HTTP method/path policy, arbitrary route, refresh behavior, or provider
operation semantics. Normalization rejects ambiguous header recognition,
provider/display-name collisions, environment/file collisions, unsafe relative
home paths, unsafe headers, unsupported substitutions, duplicate keys, unknown
fields, documents larger than 64 KiB, and incomplete bindings.

Executable manifest commands are rejected because acquisition code runs inside
the trusted broker: accepting repository-selected shell or user code would turn
data extension into code loading. Request-wide replacement is rejected because
the same bytes can occur in URLs, bodies, cookies, and unrelated headers; only
an exact declared header position has unambiguous credential semantics and a
finite redaction contract.

The built-in provider is `github`. Its reviewed contract is:

- acquisition helper `github-gh`;
- Workspace environment `GH_TOKEN=<project handle>` and
  `GH_HOST=github.com`;
- target `https://api.github.com:443`; and
- `Authorization` source syntax `Bearer` or `token`, replaced after allow while
  preserving the recognized scheme.

This contract supports `gh api`, `gh issue`, `gh pr`, and other GitHub API
operations. It does not authenticate `git clone`, `fetch`, or `push`.
Repository `.git/config` and optional global Git identity/configuration belong
to the Workspace and its persistent home. Auth Broker never owns either. A
future authenticated Git-over-HTTPS slice would require a separately reviewed
exact credential-helper and HTTP-binding design; the current handle contract
must not be repurposed implicitly.

Owner-controlled manifests are regular, non-symlink, owner-only JSON files
below:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/*.json
```

They cannot override a built-in ID and may use only `stdin_import`. Adding a
manifest declares credential placement and header transformation; it does not
add policy permission. `auth status` exposes the installed provider ID and its
Context configuration state without exposing the manifest's internal handle
projection or any secret.

## Workspace handle projection

On root entry, Tobari reconciles every configured provider in the Workspace's
permanent Context binding. The broker issues one handle bound to the stable
Context ID, project ID, provider ID, credential revision, and normalized HTTP
bindings. Tobari renders only the provider-declared environment and complete
files into that Workspace. A handle has the versioned shape
`tobari-h1_<base64url>` and carries 32 bytes of randomness; it is opaque and
must never be decoded into authority.

The handle is random rather than a token hash so equal or reused upstream
tokens cannot be correlated across Contexts/projects, token text never defines
the capability identity, and rotation/revocation can invalidate one independent
record without depending on an assumed provider token format. The encrypted
vault stores the raw handle; the broker's live lookup index stores only its
SHA-256 digest.

Complete-file projections are written only below the Workspace home, with mode
`0600`, through a no-symlink atomic boundary. An owner-only schema-1 registry
records Tobari-owned file paths and digests. Tobari refuses to overwrite an
unowned existing path or a projection changed outside reconciliation. Logout
or removal of a provider projection deletes only an unchanged Tobari-owned
file. Environment projections are fixed on container creation; a changed
credential revision therefore recreates only the work container at the next
root entry while preserving the logical Workspace, root, and home.

The handle is not the real credential, but every process in the same Workspace
can read and attempt to use it. Its usable authority remains the conjunction of
the trusted Context/project principal, exact provider binding, current
credential revision, and OPA allow.

## Gateway and OPA ordering

For a request that contains a Tobari-looking handle, Gateway follows this
order:

1. Derive stable Context/project identity from the local Gateway interface and
   owner-only principal registry.
2. Reject a Tobari handle marker in the URL, cookie, header name, unsupported
   header value, or ambiguous header syntax. Otherwise strictly match exactly
   one provider target, source header, and source syntax, then remove the
   placeholder header from the request.
3. Ask the broker to introspect the full Context/project/provider/target/header
   binding. Introspection returns only the revision and normalized non-secret
   metadata.
4. Send body-free OPA input schema 5. The authorization object contains the
   null managed profile and the non-secret broker provider ID.
5. Until one exact learned rule covers the ordinary L7 effect, return a
   learnable deny for host review even when a broader static host/method rule
   would allow an unauthenticated or fallback-adapter request.
6. On deny, stop without calling `resolve` or reaching upstream.
7. On allow, require OPA's managed `credential_profile` selection to remain
   null, resolve the same handle and revision exactly once, replace only the
   declared destination header, and perform the existing single upstream
   attempt.

The resolved secret is request-scoped and is absent from OPA input, audit,
denial bodies, errors, logs, CLI output, and Workspace mounts. The broker does
not interpret method or path and cannot grant network authority. OPA remains
the sole decision point for the normalized Context/project/scheme/host/port/
method/path effect.

Request bodies remain opaque and are never searched for a handle or used for
credential selection or replacement. A Workspace already knows its own handle
and may deliberately place those bytes in an otherwise policy-allowed request
body, just as it may send any other Workspace-readable data. Tobari's guarantee
is that such a body value grants no credential authority; preventing deliberate
payload exfiltration is outside this header-only v1 boundary.

A request with no Tobari broker-handle marker in any inspected URL or header
position continues through the selected fallback adapter:

- `passthrough` is the default. It redacts authentication and cookie values
  from OPA/audit and forwards original client authentication only after allow.
- `managed` validates one static profile against the trusted Context, project,
  and normalized host, then reads and injects its Gateway-only secret after
  allow. Existing `credentials.json` and `credentials/` inputs remain reserved
  for this compatibility path.

## Failure and recovery

The public catalog declares the following complete authentication recovery
inventory, including the shared read/mutation boundary faults used by these
commands.

| Faults | Meaning and recovery |
|---|---|
| `invalid_arguments` | Correct the exact command arguments through scoped help. |
| `invalid_context_name`, `context_not_found` | Choose one existing Context from `context list`; authentication never creates or guesses a Context. |
| `provider_login_unsupported`, `provider_import_unsupported` | Use only the acquisition mode declared by the installed provider; built-in login currently supports `github` only. |
| `auth_login_tty_required` | Run GitHub login with interactive stdin and stderr. |
| `github_login_cancelled`, `github_login_failed` | The helper did not commit a replacement; preserve the previous credential, correct or retry the interactive login, then inspect `auth status`. |
| `invalid_credential_input` | Import was empty, oversized, unreadable, or attached to terminal stdin. A terminal is refused before reading; pipe or redirect a trusted no-echo source. |
| `auth_status_failed`, `invalid_auth_result` | The read result was not trustworthy. Run `cluster up`, then repeat `auth status`; do not infer credential absence. |
| `auth_broker_unavailable`, `auth_broker_request_failed`, `auth_broker_locked`, `invalid_auth_broker_metadata` | Run `cluster up` to reconcile the shared broker, sockets, projection, and unlock state before another broker operation. |
| `root_key_unavailable`, `root_key_unsafe`, `keychain_denied` | Use `doctor` to inspect the fixed host backend and permissions, then retry cluster reconciliation. Never copy a key into argv or environment. |
| `root_key_missing_with_vault` | Restore the original key or explicitly remove unrecoverable local authentication state; Tobari will not replace it. |
| `auth_vault_invalid`, `auth_vault_version_unsupported` | Use `doctor` and repair or explicitly remove the exact local state without printing vault contents. |
| `invalid_provider`, `invalid_provider_manifest`, `provider_not_installed` | Correct the provider selector or owner-controlled manifest collection, run `cluster up`, then inspect `auth status`. |
| `ambiguous_provider_http_binding` | Remove overlapping exact scheme/host/port/source-header/source-format recognition from the provider collection. Run `doctor`, then `cluster up`; Tobari never partially activates the ambiguous projection. |
| `provider_credential_not_configured` | Inspect `auth status`, then choose declared `auth login` or protected non-terminal `auth import`. |
| `invalid_mutation_contract`, `missing_mutation_action`, `missing_mutation_policy`, `mutation_rejected` | No safe auth action was performed. Repair the declared/runtime boundary or exact ownership condition, then reconcile with `auth status`; do not route around the guard. |
| `auth_mutation_outcome_unknown`, `unclassified_mutation_outcome`, `mutation_output_write_failed` | The action may have committed, or confirmed completion could not be delivered. These faults are non-retryable: do not replay login, import, or logout. Run `auth status` for the selected Context and reconcile before another auth mutation. |
| `operation_canceled` | Retry only when this structured pre-action cancellation is returned; post-action uncertainty uses one of the non-retryable mutation faults above. |
| `output_encoding_failed`, `output_write_failed` | Auth state was not safely delivered. For a read, repair the output and retry; after a mutation, the catalog uses non-retryable `mutation_output_write_failed` instead. |
| `missing_runtime` | Run `doctor` and repair CLI runtime composition before another auth command. |
| `credential_handle_invalid` (HTTP 403) | Leave and re-enter the Workspace for the current project-bound handle. A copied, malformed, stale, revoked, misplaced, ambiguous, or binding-mismatched handle is never forwarded and never falls back. |
| `credential_broker_unavailable` (HTTP 503) | Leave the Workspace, run `cluster up` on the host, and retry only after the broker is ready. |

The public result's `workspace_activation.state` is
`workspace_reentry_required` after every successful login, import, or logout.
`auth status` reports `locked`, `ready`, or an unavailable command fault and
uses explicit `configured`, `not_configured`, or `unavailable` provider state;
absence is never inferred from a locked vault.

`doctor` is read-only. It reports its full diagnostic set and returns
`diagnostic_failed` when any check fails; warnings alone remain healthy. Its
authentication checks validate provider manifests, owner/mode/symlink safety,
encrypted-vault presence and integrity, the root-key backend without creating a
key, observed broker lock/readiness, and—when the broker is ready—secret-free
Context/provider and project-binding consistency. It does not unlock or start
the broker, reconcile the cluster, create or replace a root key, repair a
manifest, mutate a vault, credential, handle, or project registry, or reveal a
vault path/content, root key, primary secret, or handle.

## Deliberate limits

The supported slice has one built-in GitHub.com account per Context and
owner-controlled single-secret import providers. It does not implement token
refresh, provider logout/revocation, multiple accounts per Context, GitHub App
tokens, Git credential helpers, arbitrary OAuth, provider-selected or general
browser bridges, AWS SigV4,
request signing, dynamic short-lived credentials, provider SDK operations, or
provider-specific policy semantics. A tool may still implement its own native
flow inside its Workspace home, and advanced static users may retain the
managed adapter.

## Verification

- Domain, application, CLI, root-key, provider, broker, Gateway, runtime, and
  integration tests use synthetic secrets and make no provider call.
- Auth Broker source/snapshot and image checks verify exact bytes, pinned
  GitHub CLI checksums and license, non-root labels/entrypoint, and Linux
  amd64/arm64 construction.
- The official Auth Broker is a reviewed Linux amd64/arm64 OCI index selected
  by immutable manifest digest. Contributors use `task build:dev` and
  `tobari-auth-broker:dev` for explicit source validation; a development image
  or moving tag cannot become normal runtime authority.
- A release candidate requires a manual trusted-host GitHub check: login to a
  test account, confirm the host opens the fixed device page without a Git
  credential prompt, Broker browser error, persistent-plaintext warning, or
  Git configuration, confirm only secret-free status, re-enter a Workspace, verify
  without printing either value that `GH_TOKEN` has the `tobari-h1_` shape and
  `gh auth token --hostname github.com` returns that exact handle, perform one
  OPA-allowed `gh api` request, logout, and prove the prior handle fails.
  `task integration:test` supplies the required reproducible synthetic Auth
  Broker proof. Credential values, device codes, vaults, handles, and raw
  authenticated transcripts are never committed as evidence.
