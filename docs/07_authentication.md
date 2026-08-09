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
tobari auth login <provider> [--method identity-center|console] [--context NAME] [--format text|json]
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

Login requires an interactive terminal and supports two reviewed built-ins:

The provider executable must resolve through `PATH` from `/bin`, `/usr/bin`,
`/usr/local`, `/opt/homebrew`, `/opt/local`, `/nix/store`, or `/snap`.
Project, temporary, relative, and home-local
executables are rejected. Symlinks are resolved, the final regular executable
must not be group- or world-writable, and its digest is bound before provider
state is accepted.

- `github` executes a reviewed fixed driver around the trusted host's GitHub CLI
  with an ephemeral private configuration directory. It binds the canonical
  executable identity, opens only `https://github.com/login/device`, requests
  no Git protocol, captures one API token, and confirms destruction of the
  temporary state before accepting the result.
- `aws` requires one explicit method, with omission preserving
  `identity-center`. Identity Center asks for a classic commercial
  `https://<label>.awsapps.com/start` URL, reviewed commercial SSO region,
  12-digit account ID, and role name, then runs the fixed device-code login.
  `console` asks for one commercial region, requires trusted-host AWS CLI 2.32
  or newer, and runs fixed `aws login --remote`; AWS CLI reads the returned
  authorization code from terminal stdin. Both methods use a private temporary
  home, bind the executable digest, and capture only their distinct bounded
  opaque cache state. China, GovCloud, ISO, sovereign partitions, newer portal
  URL forms, same-device callback login, and ambient profiles are excluded.
  Cleanup failure rejects the result. State enters the encrypted Context vault;
  no provider home or temporary credential is copied into a Workspace.

For GitHub and Identity Center, opener failure leaves the same validated URL
and code as the manual action. Console mode is deliberately cross-device:
Tobari opens only the first authorization URL whose HTTPS authority, path,
fixed OAuth values, UUID state, PKCE challenge, redirect, and selected
commercial region all match the reviewed contract. It always leaves AWS CLI's
same sign-in URL and authorization-code prompt in the terminal, and opener
failure adds a manual-action message. Provider output is bounded
and projects backslashes, controls/formats, invalid UTF-8, and Unicode line
separators visibly before writing to the terminal. Only a bounded account label
may appear in the secret-free result. Credential and SSO state never cross CLI
stdout/stderr, argv, environment, or a Workspace.
The direct host driver returns its bounded result only to trusted CLI
infrastructure, which commits it through the fixed Broker control operation and
length-bound stdin. Login is not a companion message and does not expose a host
service.

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

After unlock, `cluster up` verifies the exact Broker container and starts one
resident trusted-host credential companion through the current Tobari
executable's private same-binary mode. It opens no listener and holds fixed
`docker exec -i` argv to an image-owned bridge that byte-pumps to
`/run/tobari-auth/companion/bridge.sock`, an unmounted Broker-private socket.
The host derives one fresh purpose-separated epoch key from the installation
root key; only the derived key crosses inherited companion stdin. A challenge
derives direction-specific AES-GCM keys, and exact sequence numbers, frame
bounds, deadlines, and a closed message set reject replay, gaps, ambiguity, and
arbitrary execution. Gateway, OPA, Workspaces, and provider children receive no
session key or channel descriptor. The only provider operation on this channel
is post-policy AWS credential export; interactive GitHub/AWS login runs directly
through context-bound host drivers.

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

The vault keeps a schema-1 AES-256-GCM envelope with a random 12-byte nonce and
authenticated data that binds the envelope schema and stable Context ID. Its
encrypted schema-2 payload stores either a static primary secret or opaque
schema-v1 AWS host-driver state. Strict valid schema-1 static payloads are
migrated on read. The file
is written through an owner/mode/symlink-checked atomic fsync-and-rename
boundary. A provider has at most one active typed credential in one Context.
Credential and handle
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
| Public cluster status | schema `4` | `root_key_backend` is `macos_keychain`, `xdg_file`, or diagnostic `unavailable`; always-present `credential_companion_state` is secret-free `ready`, `prepared`, `absent`, or `unavailable` |
| Public Context report | schema `6` | Complete shell and Git identity policy plus secret-free broker/provider state; no vault path/content, root key, primary secret, or handle |
| Owner provider manifest | `tobari.auth-provider.v1`; `schema_version: 1` | `${XDG_CONFIG_HOME:-$HOME/.config}/tobari/auth/providers/*.json` |
| Reviewed built-in provider manifest | `schema_version: 1|2` | Embedded; schema 2 is reserved for typed built-in plans |
| Normalized provider projection | schema `2` (schema `1` remains readable) | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/projection/providers.json` |
| Encrypted Context vault | envelope schema `1`; payload schema `2` with strict static schema-1 migration | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/contexts/<context-id>/vault.enc` |
| Linux installation root key | raw 32 bytes | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/keys/root.key` |
| macOS installation root key | raw 32-byte generic password | Keychain service `io.tobari.auth-root.v1`, account `tobari` |
| Project authentication registry | schema `1` | `${XDG_STATE_HOME:-$HOME/.local/state}/tobari/auth/projects/<project-id>.json` |
| Broker control protocol | schema `1` | `/run/tobari-auth/control/broker.sock` |
| Broker runtime protocol | schema `1` | `/run/tobari-auth/runtime/broker.sock` |
| Broker companion protocol | private epoch/frame schema `1` | `/run/tobari-auth/companion/bridge.sock`; unmounted private tmpfs |
| Workspace handle | version `1` | prefix `tobari-h1_` |

For `credential_companion_state`, `absent` means no prepared epoch or active
session, `prepared` means Broker accepted an epoch and awaits the authenticated
channel, `ready` means the channel is active, and `unavailable` means the
Broker control observation failed. None of these values describes a provider
credential or grants permission.

`linux_xdg_file` is an infrastructure/doctor diagnostic label only. It is not
a permitted public `auth.storage_backend` or successful cluster
`root_key_backend` value; the public Linux value is `xdg_file`.

## Provider manifest boundary

Owner-controlled providers use the strict schema-1 static parser. Reviewed
built-ins may remain schema 1 or use schema 2 for one typed credential/signing
plan. A static manifest declares:

- a stable provider ID and secret-free display name;
- `builtin_helper` or `stdin_import` acquisition;
- exactly one opaque `primary_secret`;
- bounded Workspace environment or complete-file templates containing a
  `${HANDLE}` placeholder; and
- exact HTTPS target, source header/syntax, destination header/transformation,
  and secret-header redaction names.

The owner schema identifier is `tobari.auth-provider.v1`; its on-disk
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
fails the whole provider collection before activation; Tobari does not guess
or partially load a manifest. Owner files cannot select schema 2, a helper, a
refresh flow, or a signer. New behavioral plans require a reviewed built-in,
an accepted compatibility/security decision, and executable negative tests.

Reviewed built-in driver names are data only after the binary selects one
closed compiled implementation. A manifest, Context, repository, request, or
companion frame cannot supply an executable path, argv, environment name,
shell, or driver module. The host driver resolves and hashes one executable
from the closed conventional installation roots above, uses fixed argv and a
sanitized environment, and reconstructs only a private bounded temporary home.
Auth Broker contains no provider CLI.

A manifest cannot contain a secret, command, shell fragment, executable path,
HTTP method/path policy, arbitrary route, refresh behavior, or provider
operation semantics. Normalization rejects ambiguous header recognition,
provider/display-name collisions, environment/file collisions, unsafe relative
home paths, unsafe headers, unsupported substitutions, duplicate keys, unknown
fields, documents larger than 64 KiB, and incomplete bindings.

Executable manifest commands are rejected because acquisition code executes in
the trusted host driver boundary: accepting repository-selected shell or user
code would turn data extension into code loading. Request-wide replacement is rejected because
the same bytes can occur in URLs, bodies, cookies, and unrelated headers; only
an exact declared header position has unambiguous credential semantics and a
finite redaction contract.

The built-in static providers are:

- `github`: reviewed host driver `github-gh`, `GH_TOKEN=<handle>`,
  `GH_HOST=github.com`, and exact bearer/token replacement at
  `https://api.github.com:443`;
- `chatwork`: protected-stdin import, `CWK_API_TOKEN=<handle>`, and exact raw
  `X-ChatWorkToken` replacement at `https://api.chatwork.com:443`; and
- `datadog`: protected-stdin import, `DD_ACCESS_TOKEN=<handle>`, fixed
  `DD_SITE=datadoghq.com`, and exact bearer replacement at
  `https://api.datadoghq.com:443`.

The built-in schema-2 `aws` provider retains compatibility helper ID `aws-sso`
for its closed refreshable AWS CLI session plan. Public method selection maps to
strict `aws_cli_sso` or `aws_cli_console_login` state; manifests cannot select
either driver. The provider projects the same
handle into `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, and
`AWS_SESSION_TOKEN`, disables EC2 metadata lookup, and declares the fixed
`aws_sigv4` plan. The handle is deliberately accepted only as AWS CLI's
placeholder input; it is removed before policy and is never an AWS credential.
After allow, Broker sends one bounded `refresh_lease` operation through the
authenticated companion. The fixed AWS host driver materializes encrypted
opaque state in a private temporary home, runs
`aws configure export-credentials --profile tobari --format process`, and
returns one typed temporary role tuple plus updated opaque state. Broker
rechecks the credential record/revision, atomically persists state, and signs
the bounded request. The provider CLI never enters the Broker image.

The GitHub contract supports `gh api`, `gh issue`, `gh pr`, and other GitHub
API operations. It does not authenticate `git clone`, `fetch`, or `push`.
Repository `.git/config` and Workspace-authored global Git configuration belong
to the Workspace and its persistent home. A separate `config git` boundary may
install only a lower-precedence Context fallback for `user.name` and
`user.email`; it transfers no helper, token, SSH, signing, HTTP, or other Git
configuration and grants no transport authority. Auth Broker never owns that
identity policy, and Git identity never implies a broker provider account.

Two owner-manifest examples cover bounded integrations without creating broad
built-ins:

- `examples/auth-providers/kubernetes-bearer` renders one complete CA-verified
  kubeconfig for one exact public DNS API server and replaces one bearer handle;
- `examples/auth-providers/twg-delegated-oauth` replaces a caller-supplied
  delegated OAuth access-token handle only at `https://api.atlassian.com:443`.

Their READMEs are part of the contract. Kubernetes exec/OIDC/client-certificate
flows and private or multi-cluster endpoints are excluded. The TWG example has
no login or refresh and makes no claim for calls that leave the exact authority;
TWG itself stays in the local toolbox pending redistribution permission.

The optional locally built Context toolbox supplies `kubectl`, `cwk`, `pup`,
and TWG while inheriting the published base's existing GitHub CLI and AWS CLI.
This capability does not add those four tools to the public base or any
provider CLI to Auth Broker. Binary placement and broker authority are separate:
Chatwork, Datadog, one bounded Kubernetes bearer configuration, and the bounded
delegated TWG authority use project-bound static handles with no automatic-
refresh claim; general TWG remains unsupported.

The exact normalized GitHub fields are:

- acquisition host driver `github-gh`;
- Workspace environment `GH_TOKEN=<project handle>` and
  `GH_HOST=github.com`;
- target `https://api.github.com:443`; and
- `Authorization` source syntax `Bearer` or `token`, replaced after allow while
  preserving the recognized scheme.

A future authenticated Git-over-HTTPS slice would require a separately reviewed
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
   one static header binding or the built-in AWS authorization/token
   placeholders, then remove every placeholder credential field.
3. Ask the broker to introspect the full Context/project/provider binding.
   Static introspection binds the exact target/header transformation; AWS
   introspection binds the complete fixed signing plan and concrete HTTPS
   authority. Introspection returns only the revision and normalized
   non-secret metadata.
4. For ordinary HTTP, send body-free OPA input schema 5. At an exact
   trusted-host-declared GraphQL endpoint, first buffer and parse the bounded
   request, then send only operation type and sorted canonical root fields;
   document text and variables never enter OPA. The authorization object
   contains the null managed profile and the non-secret broker provider ID.
5. Until one exact learned rule covers the ordinary L7 effect, or every
   GraphQL root coordinate, return a
   learnable deny for host review even when a broader static host/method rule
   would allow an unauthenticated or fallback-adapter request.
6. On deny, stop without calling the companion or `resolve`, refreshing AWS,
   deriving role credentials, signing, or reaching upstream.
7. On a static allow, require OPA's managed `credential_profile` selection to
   remain null, resolve the same handle/revision exactly once, replace only the
   declared destination header, and make one upstream attempt. On an AWS allow,
   retain the complete request only within the 8 MiB cap, hash it, send the
   body-free normalized signing request with the same revision. Broker validates
   encrypted driver state, runs at most one post-policy companion export under
   a per-record single-flight lock, releases its global state mutex over host
   I/O, rechecks the record/revision, rejects stale results, persists refreshed
   opaque state, signs locally, and returns final headers. Gateway applies only
   those headers and makes one upstream attempt.

The resolved secret is request-scoped and is absent from OPA input, audit,
denial bodies, errors, logs, CLI output, and Workspace mounts. The broker does
not interpret method or path and cannot grant network authority. OPA remains
the sole decision point for the normalized Context/project/scheme/host/port/
method/path effect and, at a declared GraphQL endpoint, its operation type/root
coordinate.

Request bodies are never searched for a handle or used for credential
selection. Ordinary bodies stream after allow. AWS buffers one complete
bounded body after header-time allow so the broker can sign its SHA-256 digest.
A declared GraphQL endpoint instead buffers at most 1 MiB before policy and
derives only operation type and root fields; document text and variables never
enter the broker, OPA, audit, or Tobari logs. A Workspace already knows its own handle and may
deliberately place those bytes in an otherwise policy-allowed request body.
Such a body value grants no credential authority; preventing deliberate
payload exfiltration remains outside the boundary.

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
| `provider_login_unsupported`, `provider_import_unsupported` | Use only the acquisition mode declared by the installed provider; built-in login supports `github` and `aws`. |
| `auth_login_tty_required` | Run built-in provider login with interactive stdin and stderr. |
| `github_cli_unavailable` | Install the reviewed GitHub CLI on the trusted host so Tobari can resolve one absolute executable, then retry login. |
| `github_login_cancelled`, `github_login_failed` | The trusted-host driver did not commit a replacement; verify the host GitHub CLI, then correct or retry the interactive login and inspect `auth status`. |
| `aws_cli_unavailable` | Install the reviewed AWS CLI on the trusted host so Tobari can resolve one absolute non-group/world-writable executable, then retry login. If the executable changed after login, repeat `auth login aws` with the intended method to bind fresh state to its new identity. |
| `auth_login_method_not_applicable` | Remove `--method` for non-AWS providers. |
| `aws_console_login_unsupported` | Install AWS CLI 2.32 or newer on the trusted host, then retry `auth login aws --method console`. |
| `aws_console_login_cancelled`, `aws_console_login_timeout` | No AWS replacement was committed. Start a new console login and complete the remote authorization-code flow deliberately. |
| `aws_console_config_invalid`, `aws_console_login_failed` | Correct the commercial AWS region, verify AWS CLI 2.32 or newer and provider connectivity, then retry console login; the previous credential remains unchanged. |
| `aws_sso_login_cancelled`, `aws_sso_login_timeout` | No AWS replacement was committed. Correct or complete the trusted-host device flow, then retry deliberately. |
| `aws_sso_config_invalid`, `aws_sso_login_failed` | Correct the bounded IAM Identity Center configuration, verify the host AWS CLI, or recover provider connectivity; the previous credential remains unchanged. |
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
| `credential_broker_unavailable` (HTTP 503) | This is a known pre-execution availability class. A same-record AWS refresh waiter stops after one second without creating a barrier or calling the companion; retry after the current request settles. For cluster/companion absence, leave the Workspace, run `cluster up`, and retry only after readiness. |
| `credential_refresh_outcome_unknown` (HTTP 409) | Do not let the AWS CLI or SDK automatically replay the request. After the original request settles, run `auth status`. If `broker_state=ready` and AWS is `configured`, Gateway made no upstream attempt and the user may explicitly retry the task. If AWS is `not_configured`, the encrypted record is durably barred across Broker restart: run `auth login aws` or logout, then leave and re-enter the Workspace before retrying. Reconcile `locked` or `unavailable` status first. |
| `broker_signing_request_invalid` (HTTP 403) | The AWS request used an unsupported or ambiguous signing form. Use a standard bounded SigV4 header request to a reviewed AWS HTTPS authority; do not retry as presigned, SigV4a, streaming/event, custom-endpoint, or over-limit traffic. |

The public result's `workspace_activation.state` is
`workspace_reentry_required` after every successful login, import, or logout.
`auth status` reports `locked`, `ready`, or `unavailable` without turning an
absent cluster or Broker into a command fault, and
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

The supported slice has one built-in GitHub.com account, one configured AWS IAM
Identity Center role, the exact Chatwork/Datadog static bindings, and
owner-controlled single-secret import providers per Context. Host AWS CLI
credential export plus standard Broker SigV4 is the sole dynamic plan. It excludes multiple
accounts or roles per provider/Context, remote provider revocation, GitHub App
tokens, Git credential helpers, arbitrary OAuth, provider-selected helpers,
SigV4a, query presigning, streaming/aws-chunked/EventStream signatures,
normalization-sensitive object paths, custom/private endpoints, bodies over
8 MiB, provider SDK operations, and provider-specific policy semantics.

The Kubernetes example is limited to one public globally routed DNS API server,
one CA-verified complete kubeconfig, and one static bearer token. The TWG
delegated-token example has no login or refresh and covers only requests that
remain on exact `api.atlassian.com:443`; it is not a complete TWG authority
contract. General TWG login, automatic refresh, and its other authorities are
unsupported. A tool may still implement its native flow inside its Workspace home,
and advanced static users may retain the managed adapter.

## Verification

- Domain, application, CLI, root-key, provider, broker, Gateway, runtime, and
  integration tests use synthetic secrets and make no provider call.
- Auth Broker source/snapshot and image checks verify exact bytes,
  provider-CLI absence, bridge/protocol behavior, non-root labels/entrypoint,
  and Linux amd64/arm64 construction. Host-driver tests independently verify
  canonical executable identity and fixed GitHub/AWS CLI contracts.
- The official Auth Broker is a reviewed Linux amd64/arm64 OCI index selected
  by immutable manifest digest. Contributors use `task build:dev` and
  `tobari-auth-broker:dev` for explicit source validation; a development image
  or moving tag cannot become normal runtime authority.
- A release candidate requires manual trusted-host GitHub and AWS checks.
  For GitHub: login to a
  test account, confirm the host driver opens the fixed device page without a
  Git credential prompt or Git configuration, confirm only secret-free status,
  re-enter a Workspace, verify
  without printing either value that `GH_TOKEN` has the `tobari-h1_` shape and
  `gh auth token --hostname github.com` returns that exact handle, perform one
  OPA-allowed `gh api` request, logout, and prove the prior handle fails.
  For AWS: validate both `--method identity-center` and `--method console`
  against disposable test identities. For each, re-enter, confirm without
  printing that the three AWS credential variables equal one handle, run one
  OPA-allowed bounded `aws sts get-caller-identity --region <region>`, repeat
  after the temporary lease expires to exercise automatic post-policy refresh,
  logout, and prove the old handle fails. Console validation additionally
  proves AWS CLI 2.32-or-newer preflight and fixed remote flow.
  `task integration:test` supplies the required reproducible synthetic Auth
  Broker proof. Credential values, SSO state, role credentials, signed headers,
  device codes, vaults, handles, and raw authenticated transcripts are never
  committed as evidence. The reviewed Gateway API-3 index built from source
  revision `328196221c5be2861b67ec51339d0184b04c6b31` and Auth Broker API-2 index
  built from source revision `a3fedb66ad5a72c19d6721f3f8da49852882ced8` are
  anonymously retrievable for Linux amd64/arm64 and pinned as Gateway
  `sha256:44a84576266617c78eae433ea53d60e199226dc7bc275b2aaa6c728875c91878`
  and Auth Broker
  `sha256:a2df8169fd1b28ab67d42c83c5181714ce5373ab74fe9931e84ab4542dc97fb1`;
  their inspected configurations use the required API versions, reviewed
  roles/entrypoints, and non-root `1000:1000` users. Live trusted-host provider
  scenarios remain a separate pre-tag release check.
