# ADR 0019: Add a shared locked Auth Broker for Context credentials

- Status: Accepted
- Date: 2026-08-08
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O, harness, public boundary, and release
- Supersedes: [ADR 0007: Exclude provider adapters from the MVP](0007-exclude-provider-adapters-from-the-mvp.md)
- Superseded by: [ADR 0020: Add reviewed broker credential plans and post-policy AWS signing](0020-broker-reviewed-credential-plans.md)

## Context

Tool-native authentication inside a Workspace remains the universal path, and
the retained static managed adapter remains useful. Neither path closes the
case where a user wants to acquire one host-owned credential once for a Context
without copying the real credential into every Workspace. A Workspace-readable
host token would violate the credential boundary, while resolving a token
before policy would let credential access precede network authorization.

The first repeated provider case is GitHub CLI access to GitHub.com. The
product needs a narrow capability that preserves generic HTTP policy, keeps
provider secrets outside Workspaces and OPA, binds Workspace use to the trusted
Context/project principal, and does not create a general provider API or OAuth
framework.

## Decision drivers

- Acquire a credential through a trusted-host interaction without argv,
  environment, or Workspace secret exposure.
- Keep OPA as the only authority for the outgoing HTTP effect.
- Give every eligible Workspace a revocable project-bound handle rather than a
  reusable Context secret.
- Preserve tool-native passthrough and the retained managed adapter.
- Make provider recognition finite, declarative, strict, and testable.
- Keep root-key, encrypted-vault, and credential-bearing code inside
  infrastructure.

## Considered options

### Keep only tool-native and static managed authentication

This has the smallest trusted surface but leaves users to repeat login per
Workspace or manually provision managed files. It does not close the desired
Context-wide host acquisition outcome.

### Copy a Context credential or host CLI state into each Workspace

This is operationally simple, but every untrusted process could read the real
credential and reuse it outside the bound request. It is rejected.

### Add a shared locked broker with project-bound handles

One trusted daemon owns encrypted Context vaults and issues random opaque
handles bound to Context, project, provider, credential revision, and exact
HTTP header binding. Gateway introspects the handle before policy and resolves
the secret once only after allow. This option is selected.

## Decision

Tobari adds exactly one shared, non-root Auth Broker to the installation-local
cluster. The daemon has no TCP listener and starts locked. It serves schema-1
bounded newline-delimited JSON over separate owner-only control and runtime
Unix sockets. Host commands reach the control boundary through fixed
in-container operations; only Gateway mounts the runtime socket. Neither OPA
nor any Workspace can address either socket.

The installation root key is 32 random bytes. On macOS it is stored as the
`io.tobari.auth-root.v1` generic password for account `tobari` in Keychain. On
Linux it is stored in the owner-only XDG state file
`auth/keys/root.key`. Cluster reconciliation transfers the key through stdin,
never argv or environment, and the broker retains it only in memory. A missing
key while an encrypted vault exists fails closed instead of generating a
replacement.

Each stable Context owns one schema-1 AES-256-GCM vault at
`auth/contexts/<context-id>/vault.enc`. A random 12-byte nonce and authenticated
data bind the schema and Context ID. The broker stores raw handles only inside
authenticated ciphertext; its live lookup index contains only handle hashes.
An ordinary restart returns the broker to locked state. Unlock rehydrates the
same valid handles for the same credential revision and bindings. Credential
replacement and logout revoke every associated handle atomically, so an
already-running Workspace must be re-entered after any auth mutation.
Credential ownership is Context-wide: every permanently bound project is
eligible, but each distinct project handle is issued or reconciled only on its
next matching Workspace entry. Logout revokes old handles immediately; next
entry removes the environment projection and only unchanged Tobari-owned
complete files.

Provider manifests use one strict schema-1 data contract. They declare
acquisition mode, one opaque primary secret, bounded Workspace environment or
complete-file templates, and exact HTTPS target/header transformations. They
contain no secret, shell, executable path, arbitrary route, method, or policy
semantics. The built-in `github` provider is the only helper-backed provider:
it uses the pinned GitHub CLI for an interactive trusted-host login, projects
`GH_TOKEN=<handle>` and `GH_HOST=github.com`, and recognizes GitHub API
`Authorization` bearer or token syntax for `https://api.github.com:443`.
Owner-controlled providers below XDG config may use protected non-terminal stdin import
only and cannot replace a built-in provider. Terminal stdin is refused before a
byte is read. Non-terminal bytes are read after public Context/provider
argument, intent, and mutation validation; infrastructure validates the
selected existing Context, installed provider/acquisition mode, and broker
readiness before broker send. Overlapping exact
scheme/host/port/source-header/source-format recognition rejects the complete
collection as `ambiguous_provider_http_binding`.

On Workspace reconciliation, each configured Context provider receives a
distinct random `tobari-h1_...` handle bound to the stable Context and project
IDs, provider, credential revision, and normalized header bindings. Tobari may
place that handle only in the manifest-declared environment or complete-file
projection. Tobari-owned projected files are owner-only, atomically written,
tracked by digest, and never overwrite or silently replace an unowned or
modified file.

Gateway recognizes a handle only when exactly one provider binding matches the
normalized HTTPS authority and source-header syntax. It removes the handle from
the request before broker or OPA calls, performs non-secret introspection, and
sends body-free OPA input schema 5 with the trusted principal and non-secret
`authorization.broker_provider`. A denial ends the flow without secret
resolution. After allow, Gateway requires no managed profile selection,
resolves the same revision exactly once, replaces only the declared header, and
performs the existing single upstream attempt. Copied, malformed, stale,
revoked, ambiguous, or binding-mismatched handles return
`credential_handle_invalid`; a locked, unavailable, or invalid broker returns
`credential_broker_unavailable`. Both fail closed and expose no secret.
Fallback is selected only when no Tobari broker-handle marker exists in any
inspected URL/header position. A malformed, misplaced, ambiguous, or
binding-mismatched Tobari-looking marker never falls back and is never
forwarded. Denial audit emits neither query nor headers, retains only the path
component, and replaces the whole path with `/[redacted-auth-handle]` when it
contains a marker. Structural URL/header handle rejections are non-learnable.

This decision extends but does not replace
[ADR 0009](0009-defer-gateway-managed-credential-injection.md): tool-native
passthrough remains the default fallback for requests containing no Tobari
broker-handle marker anywhere inspected, and the static managed adapter remains
available. OPA continues
to authorize generic Context/project/scheme/host/port/method/path effects; the
broker never interprets GitHub operations or grants network permission.

## Consequences

### Positive

- One Context credential can serve multiple Workspaces without sharing its real
  value with any Workspace.
- Rotation, logout, restart locking, and project-specific handles have explicit
  deterministic behavior.
- The GitHub happy path is catalog-owned through `auth login`, `auth status`,
  and `auth logout`; owner-controlled providers have a bounded `auth import`
  path.
- Provider recognition and secret replacement stay separate from OPA policy
  semantics.

### Negative

- The shared trusted runtime gains a third service, a host root-key dependency,
  encrypted state, Unix-socket protocols, and another published image.
- Existing Workspace processes cannot receive changed environment or file
  projections; users must leave and re-enter them after login, import, or
  logout.
- The built-in GitHub flow requires an interactive terminal and live GitHub
  network access. Automated tests cannot prove provider availability or account
  authorization.
- Linux root-key storage protects against accidental cross-boundary exposure,
  not compromise of the host user account.
- Shared cluster teardown intentionally preserves encrypted Context vaults and
  the installation root key. `--purge` adds only shared CA-volume deletion;
  explicit `auth logout` remains the credential-deletion/revocation action.

### Risks and mitigations

- A forged handle could be replayed. The broker checks the full Context,
  project, provider, revision, target, and source binding on introspection and
  resolution; Gateway derives Context/project authority from its local
  interface.
- A compromised Gateway could request a secret. Gateway is already trusted to
  forward or inject credentials; the runtime socket remains unavailable to
  Workspaces and OPA, and the broker returns a value only for an exact valid
  binding.
- Root-key loss could make state unrecoverable. Tobari reports the missing-key
  condition and never silently replaces the key while a vault exists; backup
  and recovery remain an operator responsibility.
- A provider manifest could create ambiguous or unsafe projection. Strict size,
  ownership, key, header, path, template, collision, and acquisition-mode
  validation rejects the entire collection before activation.

## Mechanical enforcement

- Domain parsers and normalization tests cover exact schema-1 manifests,
  collision rejection, safe paths/templates, and deterministic bindings.
- Root-key tests cover Keychain command shape, Linux owner/mode/symlink rules,
  atomic creation, and missing-key-with-vault failure.
- Auth Broker tests cover strict 64 KiB frames, 32 KiB secrets, locked startup,
  AES-GCM vault integrity, hash-only live indexes, durable handles, Context and
  project separation, rotation, logout, and restart/unlock behavior.
- CLI catalog and application tests cover all four auth commands, stdin-only
  import with terminal refusal before reading,
  public-validation-before-read and runtime-prerequisite-before-broker-send
  ordering, complete secret-free schema-1 output, effects, mutation facts, full
  failure inventory including `ambiguous_provider_http_binding`, all
  non-retryable mutation-unknown reconciliation paths, and exact Context-wide
  next-entry/logout guidance.
- Gateway tests cover exact handle recognition, removal before OPA, schema-5
  non-secret input, no resolve on deny, exactly one same-revision resolve on
  allow, marker-absence-only fallback, header replacement, query/header-free
  audit, whole-path marker redaction, non-learnable structural rejection, and
  secret-free failure output.
- Runtime tests cover the three-service Compose topology, socket mount
  separation, provider projection, locked-to-ready startup, project handle
  projection, file ownership, down/purge vault-and-root-key preservation,
  cleanup, and fixed resource/log bounds.
- Doctor tests prove provider/root-key/vault/broker/project-binding diagnostics
  are read-only, warnings alone remain healthy, failures return
  `diagnostic_failed`, and no key, service, or authentication state is repaired.
- Canonical-source drift and image checks cover the Auth Broker snapshot,
  pinned GitHub CLI archive checksums, license notice, labels, entrypoint,
  non-root user, and Linux amd64/arm64 build.

## Compatibility and migration

The public Catalog adds `auth login`, `auth import`, `auth status`, and `auth
logout`; all use JSON envelope `auth` with schema version 1. Auth mutations are
fixed-target writes against the installation's Context-scoped credential
catalog. `auth status` is a complete exhaustive provider collection for one
Context. Provider manifests, broker control/runtime protocols, encrypted
vaults, provider projection, Workspace auth-file registry, handle prefix, image
labels, and OPA input schema 5 are explicit pre-v1 compatibility boundaries.
Cluster status JSON advances to schema 3 and Context report JSON to schema 5
to project explicit broker/provider state. Current Context Rego source targets
input schema 4; aggregate generation translates legacy source 3 and current
source 4 to Gateway runtime schema 5.
Public backend values are `macos_keychain|xdg_file`, with cluster diagnostic
`unavailable`; `linux_xdg_file` is infrastructure/doctor prose rather than a
public JSON enum. The canonical schema/path/backend inventory is in
[Authentication handling](../07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

Existing Workspaces and tool-native or managed authentication remain valid.
No legacy secret is inferred into a vault. Enabling, replacing, or removing a
broker credential affects existing Workspaces only after their next root entry;
logical Workspace identity, root, and home are preserved. Rolling back to a
binary without broker support requires first removing brokered credentials and
reconciling Workspaces so stale handles are not mistaken for useful tokens.

## Security and public-boundary impact

The canonical public source is `authbroker/`; the CLI embeds a byte-checked
snapshot under `internal/infra/runtimeassets/assets/authbroker/`. The image
includes GitHub CLI 2.96.0, verified against checked-in Linux amd64/arm64
SHA-256 values, with its MIT license and third-party notice. Pull-request image
validation has no package-write permission; only the main-push job may publish
`latest`, `main`, and `sha-<commit>` development identities to
`ghcr.io/<owner>/tobari/auth-broker`. After publication, routine startup uses the reviewed
immutable multi-architecture digest recorded in embedded version metadata;
moving tags are not runtime authority. No credential, live account fixture,
live login output, vault, root key, or runtime-issued handle may enter source,
logs, test artifacts, or published images. Deterministic synthetic handle
canaries remain permitted only in tests.

The initial `AUTH_BROKER_IMAGE=unpublished` bootstrap state was replaced only
after the main-push workflow published the public multi-architecture manifest
and a reviewer independently verified and pinned its immutable digest. Current
public and release validation reject that marker, a moving tag, or a malformed
official reference. The synthetic provider-v1 fixture is pinned
in `.harness/schemas.json`; no live provider response is vendored.

## Validation

- `task check`
- `task security`
- `task public:check`
- `task gateway:test`
- `task authbroker:test`
- `task integration:test` (required reproducible synthetic Auth Broker proof)
- `task runtime:test`
- `task release:check`
- Manual, trusted-host `tobari auth login github` validation against a test
  GitHub account, followed by `auth status`, no-print checks that `GH_TOKEN` has
  the `tobari-h1_` shape and `gh auth token --hostname github.com` equals that
  exact projected handle, an OPA-allowed `gh api user` from a re-entered
  Workspace, `auth logout`, and proof that the old handle fails.

The manual GitHub check records only pass/fail, the secret-free account label,
and reviewed command outcomes outside the repository. It does not duplicate
synthetic broker manipulation. Automated tests use synthetic credentials and
make no live provider call; `task integration:test` is their required
agent-readiness evidence.

## Reconsideration signals

- A second helper-backed provider cannot fit the strict declarative acquisition
  and header-binding contract.
- A provider requires refresh, signing, multi-account selection, or more than
  one primary secret.
- Operational evidence shows restart locking, re-entry, or image publication
  makes the safe path less usable than tool-native login.
- A stronger host root-key backend or per-provider process isolation is needed
  on supported platforms.
