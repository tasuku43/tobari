# Security Model

This document is the durable Tobari security contract. The detailed threat
catalog and operational limits are in [Threat Model](THREAT_MODEL.md).

## Objective

Tobari makes bounded autonomous work practical: untrusted Tobari processes may
freely execute and may modify their explicitly mounted root, but they cannot
access other host files, another Tobari, Docker control, real host-managed
credentials, OPA administration, or direct Internet egress through the
supported configuration. Tool-owned credentials may exist inside one Tobari's
exact home by explicit user action. A brokered Workspace receives only an
opaque project-bound handle; its Context secret remains in an authenticated
encrypted vault and is resolved by trusted infrastructure after OPA allow.
Every supported HTTP/HTTPS request is normalized, authorized by OPA, and
enforced by the shared Gateway before forwarding.

Adoption is part of this security objective. A boundary that is too difficult
to create or customize will be bypassed by running the agent on the host. The
safe default therefore remains opt-in and deny-by-default, while the user
journey should make isolation, useful denial evidence, and exact permission
growth easier than manually operating Docker, OPA, or host policy files.

## Trust boundaries

Trusted components are the host OS and user, Docker Engine or its Linux VM,
Tobari CLI, resident host credential companion, reviewed host credential
drivers, Gateway, OPA, Rego policy, Auth Broker, host root-key provider,
owner-controlled XDG provider manifests, encrypted Context vaults, and reserved
host credential storage used by the managed adapter. Every Workspace and
process in it, coding agents, project files, Workspace home, copied opaque
handles, generated code, downloaded packages, request data, upstream responses,
and user/provider text displayed by CLIs are untrusted.

The reviewed GitHub, AWS, and pup host drivers are trusted, purpose-limited CLI side
effects. They select a canonical host executable and bind its SHA-256 identity,
construct only fixed argv, use sanitized environments and private temporary
homes, and accept no Workspace, repository, manifest, or request-selected
executable or argument. GitHub recognizes only the fixed
`https://github.com/login/device` browser target. AWS runs only the explicitly
selected fixed IAM Identity Center device flow or fixed console-based
cross-device login, plus typed credential export. Console browser opening
requires the selected commercial region, exact HTTPS authority/path, fixed
OAuth values, bounded UUID state and PKCE challenge, and same-region redirect;
neither AWS flow starts a callback listener or reads ambient AWS state.
Datadog pup runs only fixed US1 OAuth argv with a sanitized environment and
private file backend. pup owns the generated consent URL and loopback callback;
Tobari accepts only the fixed port allowlist and strict default-session files.

The host Git identity reader is a separate trusted, purpose-limited CLI side
effect. It accepts one canonical Workspace root and can request only global
`user.name` and `user.email` through an absolute Git executable outside that
root. The child process receives only validated host `HOME` and optional
`XDG_CONFIG_HOME` paths outside the root plus fixed locale and Git controls;
`PATH`, loader variables, shell-startup variables, and every other ambient
entry are absent. Repository/worktree configuration, caller-selected keys, raw
config files, source paths, and raw diagnostics cannot cross this boundary.

```text
host root A (rw) ---> Tobari A --+
                                  +--explicit proxy--> Gateway --OPA--> upstream
host root B (rw) ---> Tobari B --+                       |
                no cross-route                           +-- post-allow runtime socket
                                                                  |
host CLI --fixed control exec/stdin--> locked Auth Broker --encrypted Context vaults
host CLI --reviewed fixed GitHub/AWS/pup login drivers--> provider HTTPS
host companion --encrypted reverse docker exec--> Broker-private bridge socket
      |-- reviewed fixed AWS refresh driver --> provider HTTPS
Auth Broker --exact proxy-free Datadog token refresh--> api.datadoghq.com:443
host CLI --two fixed global Git reads--> validated identity scalars --> Workspace fallback
```

Each Tobari joins only its dedicated internal network. OPA joins only an
internal control network. Gateway has separate interfaces for every Tobari
proxy network, control, and egress. Auth Broker joins control and egress but
has no TCP listener and never joins a Tobari network. Only Gateway mounts its
runtime Unix socket; host control uses a separate socket through fixed
in-container operations. The companion opens no listener and mounts no host
socket or provider home. It holds one fixed reverse exec stream to an
image-owned byte-pump and an unmounted Broker-private socket.

## Assets

- Host files outside the selected root.
- Docker Engine and socket.
- Tool-owned authentication state and broker handles inside a Tobari home or
  environment, plus real brokered credentials in encrypted Context vaults,
  the installation root key, purpose-derived companion sessions, opaque AWS
  CLI cache state, encrypted pup OAuth state, request-local AWS role leases and
  Datadog access tokens, and reserved managed-adapter
  material.
- OPA policy, decision API, and Gateway management surface.
- Denial of direct Internet connectivity.
- Integrity of request normalization, policy decisions, and audit records.
- Integrity and privacy of Context-owned non-secret identity configuration;
  email is personal data even though it grants no authentication authority.

Files under a read-write root are explicitly not protected from its Tobari.
Processes can change or delete that entire mounted root. Same-root and
parent/child-root Tobari intentionally observe each other's overlapping host
file changes, even across Contexts; Tobari does not provide filesystem integrity
isolation for those files. Docker or kernel
compromise, VM/container escape, allowed-destination exfiltration, the
installation-wide baseline policy, non-HTTP protocols, covert channels,
same-Tobari process interference, and malware detection are outside the MVP
guarantee. Mutable learned permissions are bound to the host-issued project
principal described below. A tool credential is intentionally available to all
processes in the same Tobari and may be sent to any policy-allowed destination.
A broker handle is also readable by every same-Workspace process, but it is
usable only with its exact trusted Context/project/provider/revision/HTTP
binding and an OPA allow. That does not protect against a malicious process in
the same Workspace exercising an already allowed brokered capability.

The Linux owner-only XDG root-key file does not protect against compromise of
the host user or complete XDG state tree. It separates the key from encrypted
vault files, keeps the primary secret out of Workspaces, and provides a future
migration point. macOS Keychain provides stronger at-rest separation, but a
compromised trusted host, Tobari CLI, Docker Engine, Gateway, Auth Broker, or
root-key provider can still exercise or recover the credential.

## Resource and process boundary

Runtime specs prohibit privileged mode, host networking, the Docker socket,
SSH agent mounts, host home mounts, and added Linux capabilities. Each
project work container also receives fixed CPU, total memory-plus-swap,
PID-count, and container-log bounds through the infrastructure-owned Docker
specification;
the desired resource contract is part of the spec hash, so drift or an older
unbounded container is recreated before reuse. These bounds do not provide a
quota for the explicitly mounted project root or shape network bandwidth.
The shared Gateway, OPA, and Auth Broker services use fixed JSON-file log rotation of 10 MiB
per file and three files, plus fixed CPU, memory-plus-swap, and PID ceilings.
Those ceilings bound shared-service exhaustion but do not provide per-project
fairness inside one shared Gateway, OPA, or Auth Broker.
Tobari uses a non-root work user mapped to the invoking UID/GID where Docker
supports it.
Only the selected root and that Tobari's exact XDG-owned home directory are
mounted writable. The project-root resolver rejects filesystem root, the user's
home and its ancestors, and paths overlapping XDG configuration, state, or
shared-profile management directories, Docker sockets, and Docker management
paths. For a project below the host home, the selected root may be nested below
the container home path `/var/lib/tobari`, but the host home itself is never
mounted. A bind mount masks image-layer contents at its destination without
deleting them; runtime compatibility therefore keeps executable and package
assets outside the mutable home. Policy source repositories remain allowed; the active host policy,
principal registry, and reserved managed credential files are separate trusted
assets and are never selected as project roots. Publishing or applying a source
to active policy is an explicit trusted-host operation; entering that source
repository never changes policy.

A custom Tobari image is untrusted input and receives no additional authority.
The CWD-owned runtime accepts only a locally available image that asserts
runtime API `1` and preserves the built-in image user, entrypoint, and
`io.tobari.runtime-lifetime-command=sleep infinity` capability needed for the
fixed Workspace lifetime command, then independently fixes the numeric runtime
user, read-only root, capabilities, security options,
mounts, network, proxy environment, and health check. Compatibility metadata is
not a signature or provenance claim. Tobari does not grant the image's `CMD`
lifecycle authority: the infrastructure supplies the long-lived command and
runs user commands through child exec sessions. Missing or incompatible image
metadata is rejected before project home, network, or container mutation. Users
remain responsible for image contents and should prefer immutable digest
references.

Executable build identity is public metadata, not runtime authority. It is
limited to the CLI version, a full lowercase source commit or `unknown`, the
compiled resolver channel, and integer required/selected component APIs. It
contains no branch, dirty diff, absolute path, username, environment value,
registry credential, or unreviewed digest. Repository-only recovery text is
gated by the compiled development resolver metadata rather than CWD inspection.

The current Context's runtime recipe is a trusted-host build input. Explicit
`cluster up` may obtain the published official runtime base for an
uncustomized Context; `runtime build` may obtain the declared base image only
because the user explicitly requested a host build. Docker receives the
owner-only Context `runtime/` directory as its complete build context; policy
files, provider manifests, credential metadata, encrypted vaults, root keys,
secret files, the host home, Docker sockets, and Workspace mounts are outside
it. The generated image must pass the same
compatibility inspection before its reference is promoted into the Context.
Editing the recipe or a failed build cannot replace the last selected image.
After promotion succeeds, only Workspaces permanently bound to that Context
observe the selected image through their next trusted root-entry reconciliation.
Docker/BuildKit build output is untrusted diagnostic text even though the
Dockerfile is an owner-only host input. The explicit build forwards both Docker
streams through visible projection, preserving line structure and concrete
errors while making backslashes, terminal controls/formats, and Unicode line
separators distinguishable. It is not copied into the stable structured fault,
Context manifest, or audit state.
For the exact official `ghcr.io/tasuku43/tobari/runtime:latest` first base,
the explicit build also requests a refresh of the moving base. Explicit local
or custom bases do not receive that registry-pull request; this keeps local
development bases usable without weakening the build-context boundary.

The official base runtime and derived agent images do not change this boundary.
The base main channel is published by a protected main-branch workflow;
derived agent variants are separate later slices in the same runtime package.
A published Claude or Codex image is a convenience rootfs and tool bundle, not
a source of host mounts, host credentials, capabilities, network routes, or lifecycle
policy, and registry provenance does not replace local runtime compatibility
validation.

The current Claude and Codex image slices are build-only and contain no
credentials or agent configuration. Their workflows verify the versioned
release packages against the checked-in per-architecture checksums and have no
package-write permission; public publication remains gated on third-party
redistribution and license review.

The public base retains its pre-change GitHub CLI and AWS CLI artifact
inventory and associated integrity/license checks. `kubectl`, `cwk`, `pup`,
and TWG are downloaded only during an explicit trusted-host local toolbox build
with checked identities and local license inventory; that build creates no
public redistribution claim. Both images contain no credentials, copy no host
CLI configuration file, inherit the same untrusted runtime treatment, and
receive no Docker socket or direct egress. A Context narrow projection is
generated separately from its fixed validated non-secret scalars and never
changes image contents.

Each Tobari's bound Context runtime image selector is strict, bounded, and stored in
the owner-only Context manifest. The legacy XDG `config.json` image default is
used only to seed the default Context during compatibility initialization.
Project metadata cannot select or alter the runtime image, so untrusted project
files cannot introduce a second image or execution-boundary authority. The
stored project image is diagnostic last-success state rather than an image
authority.

Project runtime readiness is an explicit healthcheck boundary. Enter waits for
healthy rather than treating a running or healthcheck-less container as ready;
unhealthy, exited, and timeout outcomes remain distinct diagnostics. Desired
runtime drift is detected from the bound Context image and an
ownership-scoped spec hash, and recreates only the work container, preserving
the logical project home and root record. Missing or incompatible newly
selected images fail before replacing an existing work container or updating
project state.

Gateway image code is root-owned and read-only in the image, while the service
starts directly as the invoking numeric non-root identity supplied by Compose.
Routine startup pulls only the reviewed immutable Gateway digest recorded in
the embedded asset metadata and rejects missing labels, a root default user,
the wrong entrypoint, or a Docker Engine platform mismatch before cluster
resources are created. Contributor source testing uses the separate
`task build:dev` path and a `tobari_dev` binary that selects local image tags;
the public `cluster up` command never builds Gateway source. The image does not bake in a host UID/GID. The private CA named volume is
mounted only into Gateway and its initialization directory is writable by that
service; the public CA named volume is written by Gateway and mounted
read-only into each Tobari. Gateway opens no root entrypoint and receives no
added capability. Host credential files remain owner-only read-only binds, and
Docker Desktop-specific behavior is outside the current macOS release contract.

Auth Broker image code is likewise root-owned and read-only, with a fixed
non-root default user, entrypoint, API/role labels, dropped capabilities,
read-only root, and no host UID baked into the image. Routine startup uses only
the reviewed immutable multi-architecture digest; the explicit contributor dev
image path cannot replace that authority in a normal binary. The writable Context-vault
mount, Gateway-visible runtime-socket mount, private control/companion tmpfs,
and provider projection are distinct paths. The image contains no provider CLI,
credential, provider configuration, handle, root key, or vault.

All resources carry `io.tobari.owner=default`; per-Tobari resources also carry
the exact stable Tobari ID and a resource role. Destructive lifecycle code
resolves the nearest canonical CWD root, selects exact stored names, and
confirms owner, ID, and role labels before removal. `delete` removes that
instance's persistent XDG home and records after the session-attachment guard;
`--force` explicitly overrides an attached-session warning.
Shared cluster removal is rejected while any Tobari record remains.
Both `cluster down` and `cluster down --purge` preserve encrypted Context vaults
and the installation root key. Purge removes only the shared CA volumes and
the rebuildable active policy-bundle volume in
addition to transient cluster resources; it is not credential deletion or
revocation.

No Context directory is mounted wholesale into OPA or a Workspace. OPA sees
only one read-only content-addressed projection generated from every Context
source. Generation and policy mutation are serialized; each source and the
whole candidate are tested before atomic publication to an exact owner-labeled
Docker-managed bundle volume. A fixed networkless pinned OPA invocation writes
only a revision-named candidate; a fixed networkless pinned publisher renames
that candidate to the watched path in the same volume. The running owner-labeled OPA must report the expected revision
before success. A failed activation restores the source and prior known-good
bundle; a reducing or mixed change first confirms a deny-all transition
revision. Partial state is never mounted, and OPA receives no authority to
rewrite source or projection.

## HTTP authorization boundary

Gateway constructs normalized OPA input in mitmproxy. Ordinary HTTP remains
body-free at the request-header hook. A trusted Context-declared exact GraphQL
endpoint instead requires one positive-length body no larger than 1 MiB and
defers policy until Gateway has derived only the selected query/mutation type
and sorted canonical root fields. The input includes the host-issued Context/project principal, a structured request
authority, method, path and path segments, multi-valued query, redacted headers,
and an authorization object containing an adapter-dependent requested
credential profile plus a non-secret broker provider ID when a handle has been
successfully introspected. Both stable IDs are derived from the local Gateway
interface address and an owner-only host registry. Caller headers, environment,
URLs, session metadata, profile names, and supplied Context/project IDs are not
authorization inputs. Missing, unknown, stale, ambiguous, or mismatched Context
bindings deny before OPA and upstream I/O.
Guided Contexts supply no executable Rego: aggregate generation uses the
current Tobari-owned shared evaluator and tests with each Context's policy
data. Only Advanced Contexts own Rego source; schema 4 is current, schema 3 is
the sole compatibility input, both are rewritten to the runtime schema-5
document, and other input shapes fail before policy activation.

Secret header values, handles, credential revisions, queries, headers, and
request/response body content are absent from denial audit. GraphQL source,
operation name, variables, aliases, fragment names, directives, nested
selections, arguments, extensions, literals, and body hashes are also absent;
only an exact operation type/root-field coordinate may be retained. Audit retains the
path component, but replaces the whole path with
`/[redacted-auth-handle]` when it contains a Tobari handle marker. Structural
URL/header handle rejections are non-learnable and cannot enter policy
candidate discovery. Gateway enables ordinary request streaming only after
principal, credential binding or non-secret handle introspection, OPA decision,
and credential application succeed. An allowed AWS SigV4 plan instead retains
the complete request within the same 8 MiB cap, obtains one post-policy
signature for those exact bytes, and only then forwards. It
enables response streaming only for a flow that carries authorized-upstream
audit state. A local denial therefore cannot become an upstream request.
Mitmproxy retains an 8 MiB `body_size_limit`; a request or response with a known
`Content-Length` above that value is rejected before the ordinary addon header
hook. Allowed unknown-length ordinary bodies stream without a total-byte limit,
avoiding full-body memory retention while preserving the advertised-size cap;
unknown-length or specialized streaming AWS signing forms are rejected.
Declared GraphQL endpoints reject unknown, ambiguous, transfer-encoded,
compressed, non-JSON, non-UTF-8, or over-1-MiB request forms before OPA
learning, credential resolution, and upstream I/O.

OPA timeout, connection failure, non-2xx status, malformed JSON, missing
fields, unknown decision values, and Gateway exceptions all deny. Plain HTTP
to non-local destinations is denied by the initialized policy. The initialized
policy also requires an explicit port for each supported scheme; learned rules
retain the observed Context/project/host/port/method/path and optional GraphQL
coordinate and cannot be used on another Context, project, port, or scheme.
Ordinary body presence and content are not authorization or learning dimensions;
an exact learned rule covers every body value at its exact
Context/project/host/port/method/path. Immediately before an upstream connection,
Gateway resolves the
hostname, rejects non-global addresses for dotted hostnames, and pins the
connection to the selected resolved address. Single-label private service
names remain an explicit policy-controlled local exception for the Docker
integration shape.

A learnable policy denial returns a fixed, secret-free navigation object that
names the trusted host and `tobari policy review`, keeps the current Workspace
running, and requires a separate trusted-host terminal. A non-learnable denial
offers only `tobari cluster denials` as a read-only diagnostic and no review
command. The object is advisory data
for the child process, not an authorization token or an instruction to retry;
it contains no candidate ID, query, body, header, credential, policy path, or
dynamic command argument. Non-learnable denials advertise no review command.
The host-owned retained denial queue remains the source of truth, and only the
reference-bound host action can change policy. Interactive `policy review`
instead permits one command-bound fixed-target Apply over a bounded typed set:
each entry must originate on its exact detail screen, retain its opaque ID
unchanged, belong to one Context, and pass fresh snapshot validation. The
one-Context bound preserves one atomic policy-source promotion even if the host
process is interrupted. Display position cannot create authority, staging
writes nothing, cancellation discards the set, wildcard creation is impossible,
and redirected or machine-readable review is read-only. Refresh retains staged
authority only for an identical candidate ID; stale and same-label replacement
IDs remain undecided. Confirmed Apply returns an authoritative active revision
and exact typed receipt, after which the original running Workspace may issue a
new request. Neither Workspace nor OPA is recreated by this activation.
Session-close summaries use the same untrusted request projection
and are best-effort host stderr output.

## Credentials

The default `passthrough` adapter uses tool-owned authentication state created
below the selected Tobari's `HOME=/var/lib/tobari`. A Context does not contain
or copy this state. It redacts client
authentication and cookie values from OPA input and audit, preserves them until
policy allow, then forwards them upstream. It strips proxy and Tobari control
headers and never reads managed credential files.

The retained `managed` adapter uses static bearer or fixed-header secrets
supplied through Context-owned owner-only host files. A generated Gateway-only
projection keys profiles and secret subdirectories by stable Context ID, so the
same display profile name may exist in multiple Contexts. Configuration contains
a profile type, exact allowed hosts, explicit project IDs, and a Context-scoped
container secret path; it never contains the secret value. The host-owned
`principal-registry/principals.json` schema 2 registry binds each Context/project
pair to one exact Gateway network and local address. Its dedicated directory is mounted
read-only into Gateway; credential configuration and secret directories are
not included in that mount.

In passthrough mode, `Authorization`, `X-API-Key`, cookies, and other client
authentication are forwarded only after allow; `Proxy-Authorization`, the
profile selector, and Tobari session control headers are removed. In managed
mode, Gateway removes Tobari-provided `Authorization`, `Proxy-Authorization`,
`X-API-Key`, and configured managed-secret headers. Cookie and Set-Cookie
values may remain part of the authorized application flow but are excluded
from OPA input and Tobari audit logs. A managed header is added only after an
allow decision names a configured profile whose Context, project, and host bindings
exactly match the established principal and normalized host. Gateway checks
the Context/project binding before OPA and repeats it before reading the secret. The
value is never returned to Tobari, OPA, CLI output, errors, or audit logs.

The Auth Broker route stores one typed credential record per Context/provider
in `auth/contexts/<context-id>/vault.enc`. The schema-1 AES-256-GCM envelope
contains a schema-2 encrypted payload, uses a random 12-byte nonce, and binds
the envelope schema plus stable Context ID as authenticated data. Strict valid
schema-1 static payloads are migrated on read. Opaque AWS host-driver state and
strict Datadog OAuth-session state exist only in the schema-2 encrypted
payload. AWS cache state remains opaque to the host lifecycle and is
materialized only by the reviewed host driver in a private bounded temporary
home. Identity Center and console login use distinct strict state schemas and
driver IDs; companion refresh rejects a mismatch before provider execution.
Datadog state is interpreted only by the fixed Broker-owned US1 plan; it is
never materialized into a Workspace or provider CLI inside Broker.
All parent directories, files, ownership, modes, and symlink status are checked
before use; updates use a durable atomic replace. The broker starts locked and
retains the 32-byte installation root key only in memory. macOS stores that key
in Keychain service `io.tobari.auth-root.v1`, account `tobari`; Linux stores it
as owner-only XDG state `auth/keys/root.key`. A missing key alongside a vault is
never replaced automatically.
Public auth results name the Linux backend `xdg_file`, while macOS uses
`macos_keychain`; cluster status may additionally report `unavailable`.
Cluster status schema 5 separately reports nullable unconfigured resources and
always-present secret-free
`credential_companion_state=ready|prepared|absent|unavailable`; this is
process/channel readiness,
not provider credential state.
`linux_xdg_file` is an infrastructure/doctor diagnostic label, not a public
auth or cluster JSON value. The complete compatibility table is in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

The daemon exposes strict 64 KiB schema-1 NDJSON over separate control and
runtime Unix sockets. Key and import bytes enter through bounded stdin after a
declared length; they never use argv or environment. A third private socket is
reserved for the authenticated companion reverse channel and is not mounted
outside the Broker container. The live handle index is
SHA-256-only. Raw handles persist only inside authenticated ciphertext and bind
the Context, project, provider, credential revision, and the complete normalized
header/signing binding union. Login,
replacement, and logout atomically revoke all old handles for that
Context/provider.

`cluster up` creates a fresh companion epoch and derives a purpose-separated
session key from the installation root key; only the derived key enters the
companion through inherited stdin. The root key, derived key, and channel file
descriptors never enter a provider child process. A fresh challenge derives
direction-specific AES-GCM keys. Exact sequence numbers reject replay and gaps;
strict framing rejects unknown message kinds, duplicate sessions, oversized
payloads, invalid tags, and expired deadlines. Any failure closes the channel
without logging frame content. The bridge parses, logs, and persists nothing.
The companion is trusted host infrastructure, but its authority is limited to
the compiled reviewed AWS refresh operation and its fixed process contract;
interactive login is a direct context-bound host operation.

Gateway recognizes exactly one handle position from the owner-only normalized
provider projection. It rejects URL, cookie, header-name, unsupported-value,
and ambiguous header occurrences, removes the declared placeholder fields,
and performs non-secret introspection. OPA sees only the provider ID. For a
static provider, denial makes zero resolve calls and allow permits exactly one
same-revision resolve and one declared header replacement. For AWS, denial
makes zero companion, refresh, role-credential, or signing calls; allow
captures at most one complete 8 MiB request, hashes it, obtains one
same-revision signature, and applies only broker-returned SigV4 headers before
one upstream attempt. Broker takes a per-record single-flight lock, releases
its global state lock during one bounded companion export, and bounds a
same-record waiter to one second before a known pre-execution failure. Such a
wait expiry creates no barrier and performs no companion call. Broker rejects stale
results after replacement/logout, atomically persists refreshed opaque state,
and signs locally. Before host execution it persists a task-digest barrier in
the encrypted AWS record; only the exact correlated successful CAS clears that
barrier while committing new state. An unobserved outcome therefore survives a
Broker restart and performs no second host refresh until AWS re-login or logout.
For Datadog, denial makes zero token-selection or refresh calls. After allow,
Broker selects an access token only outside the five-minute refresh window or
performs one same-record refresh at the exact proxy-free, no-redirect US1 token
endpoint. It atomically commits the same-revision state before returning one
request-local bearer value. A task-digest barrier prevents automatic replay
when the refresh outcome is unknown.
A
copied, malformed, stale, revoked, ambiguous,
or mismatched handle returns secret-free HTTP 403
`credential_handle_invalid`; a locked, unavailable, timed-out, or invalid
broker returns HTTP 503 `credential_broker_unavailable`. Neither failure falls
back to forwarding the handle. An explicit or post-dispatch transport-unknown
AWS companion operation or Datadog refresh returns HTTP 409
`credential_refresh_outcome_unknown`, which is outside automatic retry and
likewise never forwards the request.
Because this code also covers loss of a successful Broker response, recovery
consults `auth status` after the request settles: `broker_state=ready` with
the affected AWS or Datadog provider `configured` permits an explicit user
retry because Gateway made no application upstream attempt, while
`not_configured` identifies the durable barrier and requires re-login or
logout. More
precisely, configured passthrough or
managed fallback is selected only when no Tobari broker-handle marker occurs in
any inspected URL/header position. Every Tobari-looking marker that is
malformed, misplaced, ambiguous, or binding-mismatched fails as
`credential_handle_invalid`; it is never forwarded and never falls back.

The request body is never a credential source or replacement surface. Tobari
does not search arbitrary body bytes. Ordinary bodies stream; the reviewed AWS
plan alone hashes an already-authorized bounded body for exact SigV4 output and
does not expose the bytes to policy, audit, logs, or broker state.
Because a Workspace can read its own handle, a malicious Workspace can include
those bytes as ordinary payload when the surrounding L7 effect is allowed;
this grants no broker authority but is outside Tobari's payload-exfiltration
guarantee. The real primary secret remains unavailable to that Workspace.

Provider manifests contain no secrets or executable paths. Valid schema-1
static providers normalize into projection schema 2. Provider schema 2 accepts
bounded handle templates, exact HTTPS/header transformations, and enumerated
built-in-only credential plans; it rejects target/projection collisions and
prohibits user manifests from overriding built-ins or selecting a helper,
refresher, signer, executable, argv, or environment. Auth Broker contains no
provider CLI. The built-in GitHub, AWS, and Datadog drivers execute on the trusted host,
resolve one canonical executable identity, use fixed argv and sanitized
environments, and delete private temporary homes on every outcome. GitHub asks
for no Git protocol and recognizes only its fixed device URL. AWS performs one
fixed Identity Center device login, one fixed console-based remote login, or
typed credential export. Broker encrypts the opaque
AWS cache between calls; temporary role credentials exist only while Broker
signs one authorized request. The fixed Datadog plan encrypts pup OAuth state
and refreshes it only after allow against the exact US1 token endpoint with no
ambient proxy or redirect. User providers use protected non-terminal
stdin import only. A terminal stream is refused before reading. Non-terminal
bytes are read after public Context/provider argument, intent, and mutation
validation; infrastructure validates the selected existing Context, installed
provider/acquisition mode, and broker readiness before sending the credential
to the broker. Provider collections with overlapping exact
scheme/host/port/source-header/source-format recognition fail completely as
`ambiguous_provider_http_binding`; no partial projection becomes active.

Arbitrary OAuth orchestration or request signing, multiple provider accounts
per Context, provider SDK operation inference, remote logout/revocation, Git
credential helpers, GitHub App tokens, SigV4a, presigning, AWS streaming
signatures, and process-level identity are not implemented. Refreshable AWS CLI
sessions acquired through IAM Identity Center or AWS console login plus
standard bounded SigV4, and the fixed Datadog US1 OAuth-session refresh plan,
are the only reviewed dynamic plans;
general TWG login/refresh remains unsupported. The
optional `session` value remains caller
metadata, not authentication. Gateway performs all selected credential
handling inside the trusted infrastructure boundary. A missing, malformed,
ambiguous, or stale principal registry entry denies before broker resolution,
OPA authorization, and upstream I/O.

## Mutation policy

An `EffectRead` has no first-use initialization authority. Missing XDG state is
reported as absence or a display-only synthetic default without a Context ID;
it cannot authorize a Workspace, credential, policy, key, vault, or Docker
operation. Legacy reads do not migrate, and corrupt or unsafe stored input
fails closed. The only read-side filesystem mutation is bounded cleanup of a
pre-existing validated mutation journal; that exceptional path may create the
project recovery lock only to serialize cleanup, but a read never creates the
journal itself. Fresh and ordinary reads create no lock. First durable
initialization and migration remain behind a fully validated create/write
intent.

`runtime init` is a host-only create of one owner-only recipe directory.
`runtime build` is a host-only write against the current Context runtime target;
its Docker build context is fixed to that recipe directory, and its image
promotion happens only after compatibility and digest checks. Neither command
mounts the Context directory into a Workspace or accepts a secret/image-name
override. A build failure is therefore a safe retry point: the old selected
image and its Context authority remain unchanged. Tobari does not delete an
older selected image, a failed candidate tag, or BuildKit cache as failure
cleanup; the failure summary distinguishes unchanged, uncertain, and already
promoted selection state.

Shared lifecycle mutations target one catalog-declared `tool_local` cluster.
The root command uses the catalog-declared current-directory fixed target to
read and present containing Workspace candidates, then creates or reconciles
one logical Tobari only after an explicit choice and a fresh locked check. An
exact current-root record is reused directly; a nested root requires the
explicit create-here choice. The command never creates shared resources.
Each canonical root and stable Context pair is unique: repeated or concurrent
explicit creation is serialized by the state lock and only one logical record
for that pair can be committed. Same-root records in different Contexts and
overlapping parent/child roots remain allowed.
Entering a session and returning from the child shell are not lifecycle-ending
mutations. `exit` only detaches the session; `delete` resolves the same target
and is the only routine lifecycle-ending mutation. Ordinary delete observes
active exec sessions and rejects when one is attached; `--force` is the
explicit override.
Neither operation accepts an ID or arbitrary root selector. Root entry,
status, and delete accept a Context name only as a host-side selector resolved
to stable identity before CWD, Workspace-state, or Docker observation. Prefix
and command-local spellings normalize through the same catalog input; empty,
duplicate, unknown, invalid, or stale bindings fail before downstream
lifecycle I/O. A force-delete preview carries the resolved Context ID back into
the mutation boundary, which rejects a changed authority instead of
rediscovering or falling back to the current Context.
All mutations use complete intent and impact declarations before Docker
execution. Ordinary runtime reconciliation needs no human approval;
ordinary deletion requires no attached session, while `--force` overrides that
guard. Runtime image reconciliation validates the bound Context image before
mutating Docker resources and preserves the selected XDG home; deletion affects
only the selected XDG home, tool-owned authentication state, and exact owned
runtime resources; it never removes the mounted project root. Shared
CA purge remains separate
and only follows an empty instance repository. It removes the shared CA
volumes, not encrypted Context vaults or the installation root key.

`context use` is also a trusted-host fixed-target write. It may select only an
existing validated Context and atomically changes only the current/default
marker. It does not touch Docker, the aggregate projection, an existing Tobari,
or any enforcement authority. `context create` likewise does not start Docker;
an explicit `cluster up` validates and activates the new all-Context candidate.
`config shell` and `config git` are trusted-host fixed-target writes to
owner-only Context configuration. A terminal staged editor may complete a
wholly omitted setting group, but it shows typed complete current state and
performs no write before Apply. Shell validates every distinct staged row
before one atomic manifest replacement. Apply is bound to the Context name returned by that read, even if
another process changes the current/default marker during review. Explicit-
empty Context input, partial direct input, and redirected, JSON, canceled, or
invalid editor attempts make zero mutation calls. Direct and interactive modes
reach the same application policy and atomic store boundary.

Shell accepts only `PS1`, `TERM`, `COLORTERM`, and `NO_COLOR`, never enumerates
the host environment, and resolves `inherit` only when a future child shell is
entered. `PATH`, `HOME`, `BASH_ENV`, `ENV`, `PROMPT_COMMAND`, credential names,
host startup files, and host home mounts remain outside the boundary. Values
are bounded, valid UTF-8 without NUL, and Bash-quoted when assigned by the
CLI-owned prompt hook. Prompt contents may still exercise Bash prompt
expansion inside the untrusted Workspace; they gain no host authority or
Gateway permission.

Git owns only an atomic `user.name`/`user.email` fallback. New and migrated
Contexts project none. Inherit uses no shell and gives Git an exact environment
allowlist: validated `HOME`, optional `XDG_CONFIG_HOME`, fixed `LC_ALL`, and
three fixed `GIT_*` controls. It drops ambient `PATH`, loader controls, shell
startup controls, credentials, and all other entries, rejects configuration
directories and a Git executable inside the project root, and performs two
fixed-key `--global --includes` queries with finite timeout and bounded output.
A hostile repository-local include cannot become a trusted host read. Only a
complete, bounded, valid UTF-8, control-free pair is quoted into a private
atomic system config whose existing and replacement sizes are bounded before
read or write and whose directory is mounted read-only. `/etc/gitconfig` is
included first, while Workspace-global and repository/worktree scopes override
the fallback. Host paths/directives and Git credentials, helpers, HTTP headers,
SSH commands, signing keys/programs, hooks, aliases, URL rewrites, filters,
proxies, and arbitrary keys are never serialized. Identity does not imply Git
authentication, signing, provider account authority, or network permission.
`auth login`, `auth import`, and `auth logout` are trusted-host fixed-target
writes against the installation credential catalog. They resolve one existing
explicit or current Context and one installed provider before acquisition or
vault I/O. Login accepts that provider directly or requires one terminal-only
choice from the typed installed reviewed-login-provider collection; omission
binds the later login to the Context returned by that snapshot, while omission
with redirected streams fails before the collection read or mutation. An AWS
method flag requires explicit `--provider`. Login still uses only the reviewed
built-in helper; import reads one
bounded secret from non-terminal stdin only under the ordering above. One
credential belongs to one Context/provider, and every permanently bound project
is eligible for a distinct handle only on its next matching Workspace entry.
Replacement and logout atomically revoke prior handles. No auth mutation
rewrites a running Workspace process, calls policy, grants a network permission,
or makes logout a claim of remote provider revocation. Confirmed output
therefore requires Workspace re-entry. Logout's next entry omits the environment
projection and deletes only unchanged Tobari-owned complete files. The
non-retryable `auth_mutation_outcome_unknown`,
`unclassified_mutation_outcome`, and `mutation_output_write_failed` faults all
require `auth status` reconciliation before another auth mutation; none permits
replay.

`doctor` is a read-only recovery observation. It reports the full diagnostic
set through a fixed dependency graph: only checks whose direct prerequisites
passed cross an infrastructure boundary, while independent checks continue and
unready checks are typed as blocked without duplicated recovery. It fails with
`diagnostic_failed` when any check fails; warnings alone are healthy.
Policy checks perform bounded owner/symlink/source-structure validation on the
host and never use `docker run` or create an OPA test resource. Authentication
checks validate provider manifests, vault path safety,
the fixed root-key backend without creating a key, broker state, vault
integrity, and project binding consistency when the broker is ready. Doctor
does not start/reconcile/unlock services, create or replace a key, repair a
manifest, or mutate vault, credential, handle, or project-auth state.
`policy allow`, `policy deny`, `policy reset`, and `policy compact` are
access-changing writes bound to opaque references. Discovery never mutates. An
allow reference identifies one retained validated denial that OPA marked
exact-rule learnable; a
deny reference identifies one retained validated denial and binds its exact
Context/project/host/port/method/path plus optional GraphQL coordinate; a compaction reference identifies one current
exact source-rule set; a reset reference identifies one current CLI-owned
learned Allow or exact Deny and removes it, returning the effect to default
deny. Scheme, cluster, and credential-binding failures never become permission
candidates. Ordinary body presence and content neither disqualify nor split a
candidate. Declared GraphQL documents split only by selected operation type and
canonical root field; every other document and payload detail is excluded.
Host-authored baseline denies are terminal, excluded from both the actionable
queue and the reversible decision inventory, while their bounded audit records
remain visible. CLI-owned exact Deny rules remain terminal in enforcement but
are visible in `policy rules` and can be explicitly reset; reset never makes
the request allowed.
The mutation rejects stale or ambiguous references, unsafe policy files,
malformed learned data, failed preflight tests, and unrecognized compaction
shapes before the atomic policy write.

Learned rules never broaden a project, host, port, method, or GraphQL root
coordinate beyond the
explicitly approved evidence. Baseline and exact deny rules remain terminal; an
exact deny wins over a learned allow for the same Context/project/host/port/method/path.
Prefix compaction accepts ordinary HTTP rules only and requires three exact
sources, keeps host, port, and method fixed, requires a multi-segment directory
boundary, rejects percent
encoding, backslashes, empty segments, and dot segments, retains positive
examples, and tests its matcher against an adjacent outside-prefix canary.
Host wildcards, method wildcards, user-supplied pattern text, and automatic
compaction are unsupported.

## External calls

Gateway applies finite OPA and upstream timeouts and performs one upstream
attempt. It does not retry requests because arbitrary HTTP methods and bodies
may be unsafe or non-idempotent. Redirect handling remains in the proxy flow and
each resulting request is independently authorized.

Gateway applies a finite broker-socket timeout and performs at most one
introspection plus one post-allow static resolution, AWS signing operation, or
Datadog token selection/refresh operation.
It never retries, calls the companion, resolves, refreshes, obtains role
credentials, or signs on deny. The built-in GitHub host driver runs one fixed
`gh auth login`, followed by one bounded active-account status capture and one
token capture in a private configuration directory. It recognizes the one
fixed device URL; opener failure leaves manual navigation and is not a provider
mutation failure. The driver performs no Git configuration, pagination, or
automatic retry; user cancellation or any failed capture preserves the
previous Context credential.

The AWS Identity Center host driver collects its four non-secret profile fields through
canonical terminal line input whose finite readiness polling observes command
cancellation and the login deadline without an abandoned reader. It verifies
canonical mode before prompting and again after readiness; a noncanonical
VMIN/VTIME mode fails closed before reading. The host resolves the actual
terminal device from the inherited input, opens an identity-checked private
nonblocking description, and closes it before provider execution. Readiness
flushes therefore return to bounded polling without changing flags shared with
the parent shell. Cancellation or timeout while a prompt is waiting starts
neither the AWS CLI nor a Broker mutation. It then runs one fixed device-code
`aws sso login` against a private
rendered profile. A post-policy companion operation runs at most one
fixed `aws configure export-credentials --format process` with finite connect,
read, process, output, and state bounds. AWS CLI may refresh its renewable SSO
token as part of that export; an expired overall session fails with re-login
recovery. Broker makes no provider HTTP call itself, accepts only a typed
temporary role tuple plus updated opaque cache, rechecks record/revision and
driver identity, and creates one local signature. Neither driver inherits an
ambient provider home, credential, proxy, loader, or browser selector, and
automated tests make no live provider call.

The AWS console method instead collects one validated commercial region,
checks that the resolved host AWS CLI is at least 2.32, pre-renders one empty
private profile, sets a private `AWS_LOGIN_CACHE_DIRECTORY`, and runs fixed
`aws login --remote`. AWS CLI reads the returned authorization code directly
from terminal stdin. Tobari accepts only the resulting profile's validated
`login_session` ARN/account match and one bounded canonical JSON login cache.
The companion later materializes that exact state and uses the same fixed
credential export; AWS CLI owns refresh while its refresh token remains valid.

Inherited Git identity reconciliation performs at most two host Git calls, one
per exact key, with one attempt and a finite timeout each. It performs no
network call or retry. Exit status 1 with no value means absent; an absent or
incomplete pair produces no fallback, while execution, timeout, framing, size,
encoding, or control-character failures preserve the previous projection and
fail before Docker mutation. Published failures contain neither raw stderr nor
identity values.

## Logging

Audit JSON includes timestamp, request ID, cluster, stable Context ID and name,
project ID, safe project root, host, port, method, redacted path, an optional
GraphQL operation type and one root field, decision,
reason, selected credential profile name, upstream status, and duration. It
emits no query or headers. Ordinarily the redacted path is the URL path
component; if that component contains a Tobari handle marker, the whole value is
`/[redacted-auth-handle]`. A profile name is non-secret metadata; secret values,
Git identity values, and raw bodies are excluded. URL/header handle-structure failures remain
non-learnable and cannot become policy candidates.
For brokered requests, the provider ID may be retained as non-secret adapter
metadata, while the handle, credential revision, vault data, and resolved
primary secret are excluded. Provider metadata never grants permission and
does not inherit a broad static host/method allow; the brokered request remains
a learnable denial until an exact Context/project-bound L7 rule exists.
CLI `cluster logs` reads only a bounded component-log window and does not add
unredacted diagnostics. `cluster denials` projects only validated deny records
and preserves only non-secret credential-profile names. Read-only policy
candidate commands aggregate exact Context/project/host/port/method/path and
optional GraphQL-coordinate proposals from
that evidence, treating reason, status, request identity, timestamps, and
credential-profile display evidence as non-identity fields. The latest evidence
and bounded observation count do not grant authority. `policy review`
presents the same queue, stages explicit Allow-exact or Deny-exact choices only
from each candidate's TTY detail screen, retains every unchanged opaque
reference, and applies the reviewed set once after final confirmation. Staging
is inactive authority; `policy rules`
projects every current CLI-owned learned Allow and exact Deny, and its TTY flow
delegates one unchanged opaque reference to `policy reset`. All redirected and
machine-readable paths remain read-only. Observation alone never changes
authority; every allow, deny, reset, or compaction is an explicit
reference-bound mutation.

## Enforcement

| Claim | Enforcement |
|---|---|
| No direct Tobari egress | Per-Tobari internal topology and Docker integration test |
| Tobari cannot access OPA or peers | Separate internal networks and integration test |
| OPA outage denies | Gateway unit and integration tests |
| Host-managed secrets stay outside Tobari; tool-owned state stays in its home | Mount-spec tests and integration canaries |
| Brokered primary secrets stay outside every Workspace and OPA | Socket/mount topology tests, encrypted-vault tests, Gateway canaries, and integration log/output scans |
| Broker handles cannot cross Context, project, provider, revision, or HTTP binding | Broker introspect/resolve negative tests, principal-derived Gateway calls, rotation/logout tests, and cross-Workspace integration |
| Policy denial cannot resolve a brokered secret | Gateway call-count and ordering tests proving zero resolve calls before or on deny and exactly one after allow |
| The broker restarts locked and cannot silently replace a missing root key | Restart/unlock tests, Keychain/XDG provider tests, and missing-key-with-vault rejection |
| Provider manifests cannot become executable or ambiguous authority | Strict schema/collision/path/header tests, owner-only XDG loading, and built-in override rejection |
| Provider login cannot turn visible text into arbitrary browser execution or Broker Git authority | Conventional non-project installation-root selection, canonical executable identity, fixed argv/environment, control-safe visible projection, exact fixed-URL and region-bound AWS console URL recognition, duplicate/cross-region/hostile rejection, manual fallback, checked private-home cleanup, cancellation, and no-Git-protocol tests |
| AWS CLI session state and temporary credentials cannot enter a Workspace | Encrypted SSO/console opaque-driver-state tests, private-home bounds, driver/state mismatch rejection, companion refresh/revision tests, project-binding checks, and secret-free output/log canaries |
| AWS denial cannot trigger a companion call, refresh, role acquisition, or signing | Gateway two-stage call-order tests and Broker same-revision signing checks |
| Companion transport cannot become a host service or arbitrary executor | Same-binary bootstrap, exact container/exec argv, no-listener/no-socket-mount assertions, authenticated replay/gap/size tests, closed driver registry, and child environment/FD canaries |
| Published tools retain reviewed identity and redistribution evidence | Base-runtime baseline checks for GitHub CLI and AWS CLI; optional local toolbox locks for kubectl, cwk, pup, and local-only TWG |
| Secret headers, queries, handle-bearing paths, and bodies stay out of logs | Gateway redacted-path/header-absence tests, non-learnable structural-rejection tests, and log scans |
| Broker fallback cannot accept a Tobari-looking handle | Marker-absence fallback tests plus malformed, misplaced, ambiguous, and binding-mismatch fail-closed canaries |
| Cluster cleanup preserves authentication authority until explicit logout | Down/purge tests proving vault and root-key preservation plus exact logout/revocation tests |
| Doctor observes but never repairs authentication state | Fixed-DAG dependency fixtures, recording-runner Docker-argv allowlists, legacy/fresh-tree content snapshots, and filesystem canaries for provider, root-key, vault, broker, and project-binding diagnostics |
| Only owned Docker resources are removed | Label validation and fake-runner tests |
| Attached sessions are not removed accidentally | Exact work-container Exec ID observation, guard-before-delete tests, and explicit force-override tests |
| Each root and XDG home are its Tobari's only host write scopes | Mount-spec and path-containment tests |
| Ambiguous CWD selection cannot mutate before a valid choice | Typed candidate snapshot, locked stale-choice revalidation, and zero-call cancellation tests |
| One Tobari cannot consume unbounded CPU, memory, PIDs, or container logs | Fixed create-argv and spec-hash tests plus runtime HostConfig assertions |
| A custom image cannot expand its runtime specification | Compatibility inspection, fixed create-argv tests, and integration test |
| Optional toolbox artifacts retain reviewed identity | Pinned versions, vendor checksums, explicit local build validation, and no public TWG publication claim |
| Project metadata cannot become a second runtime boundary | Context-only image resolution, ignored-project-metadata regression test, and fixed runtime adapter |
| OPA cannot rewrite Context policy | Read-only mount-spec test |
| Tested host policy activates across Docker hosts | Docker-managed watched-bundle test, exact revision assertion, stable OPA identity, and Linux integration scenario |
| CWD lifecycle actions use exact Tobari identity | Canonical-root, state, and label-validation tests |
| One canonical root/Context pair has one Workspace | Pair-derived root-index hash naming, locked exact-pair checks, domain duplicate-index validation, same-root/different-Context tests, and concurrent explicit-creation tests |
| Session exit cannot delete a Workspace | Child exit-status tests, host-stderr summary tests, and logical-state preservation after entry |
| Gateway cannot accept caller-selected Context or project authority | Owner-only atomic schema-2 registry, local-interface derivation, forged-header/malformed/unknown/stale denial tests, and multi-Context integration |
| Managed credentials cannot cross Context/project principals | Context-scoped projections and secret paths, explicit project bindings, pre-OPA Gateway rejection, repeated injection check, same-name cross-Context tests, and integration |
| Unknown effects fail closed | Domain and catalog validation |
| Denials support safe policy learning | Typed Context/project/host/port/method/path denial validation, fixed navigation-response schema, host-only session summary, secret canaries, and integration projection |
| Learned permissions stay explicit and Context/project-bound | Context-scoped opaque-reference round trips, exact effect domain tests, Rego cross-Context/project canaries, preflight-before-aggregate activation tests, and Docker integration |
| One bad Context cannot replace known-good policy | Serialized content-addressed aggregate generation, reserved namespace validation, whole-candidate OPA tests, atomic publish, rollback tests, and integration |
| Context changes cannot mutate existing Tobari authority | Permanent instance binding, current-marker-only tests, Context-local runtime reconciliation, and restart integration |
| Overlapping roots are not misrepresented as isolated | Product contract, direct read-write mounts, same-root/parent-child integration canaries, and absence of overlay/root-lock paths |
| Compaction preserves declared boundaries | Three-source same-host/port/method grouping invariant, retained positive examples, outside-prefix canary, stale-reference rejection, and OPA tests |
| Gateway does not retain allowed streaming bodies | Header-hook ordering unit tests plus incremental chunked-request and SSE-response integration canaries |
| Declared oversized bodies retain the transport bound | Fixed mitmproxy body-size asset test and over-limit `Content-Length` integration request |
| GraphQL identity cannot collapse into one HTTP route grant | Trusted exact-endpoint projection, bounded parser fixtures, OPA all-roots matching, HTTP-rule non-matching canaries, GraphQL-aware opaque-reference round trips, and zero-upstream integration tests |

## Supply chain and publication

Container bases distributed by Tobari and CI actions are pinned to immutable
versions or digests recorded in source. User-selected local images are neither
distributed nor trusted by Tobari; their selector is persisted as user
configuration. Third-party licenses are reviewed. Tests use synthetic
credentials and `example.com` identities only. Publication still requires
`task security` and `task public:check`; neither replaces a human history and
confidentiality review. The canonical Gateway source is the public `gateway/`
tree; its embedded snapshot is byte-checked against the current source, while
each published image is inspected against the exact source revision that built
it. The canonical Auth Broker source is the public `authbroker/` tree; its
embedded snapshot is byte-checked against current source, and provider-CLI
absence, bridge/protocol behavior, and multi-architecture metadata are checked
for each image's recorded build revision. Pull-request image
jobs have no package-write permission. GHCR
moving tags are development conveniences, not a trusted runtime identity;
routine Gateway and Auth Broker consumption use reviewed immutable digests
recorded in `versions.env`. A marker or moving tag is not an accepted image
reference. The reviewed Gateway API-3 index built from source revision
`328196221c5be2861b67ec51339d0184b04c6b31` and Auth Broker API-2 index built
from source revision `a3fedb66ad5a72c19d6721f3f8da49852882ced8` are anonymously
retrievable for Linux amd64/arm64; their platform metadata fixes the API/role
labels, `1000:1000` user, and reviewed entrypoint. The pinned Gateway digest is
`sha256:44a84576266617c78eae433ea53d60e199226dc7bc275b2aaa6c728875c91878`
and the Auth Broker digest is
`sha256:a2df8169fd1b28ab67d42c83c5181714ce5373ab74fe9931e84ab4542dc97fb1`.
Those selected images implement the earlier AWS Identity Center path. Current
canonical source and tests also contain AWS console login and the Datadog
request path, but those changes postdate the selected image revisions. Source
and selected runtime identity are therefore separate facts until reviewed
immutable pins advance; source/snapshot equality does not silently replace a
running or newly reconciled standard cluster image.
