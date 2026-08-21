# Security Model

This document is the durable Tobari security contract. The detailed threat
catalog and operational limits are in [Threat Model](THREAT_MODEL.md).

## Objective

Tobari makes bounded autonomous work practical: untrusted Tobari processes may
freely execute and may modify their explicitly mounted root, but they cannot
access other host files, another Workspace, Docker control, real host-managed
credentials, OPA administration, or direct Internet egress through the
supported configuration. In standard, tool-owned credentials exist inside one
Tobari's exact home only by explicit user action and are forwarded only after
OPA allow. Pinned Claude Code, Codex, GitHub CLI, AWS CLI, custom-runtime TWG,
and custom-runtime pup native login may also use a
session-scoped host-browser and loopback callback bridge. The host selects one
strict reviewed provider authorization contract and transports one callback to
the selected Workspace without inspecting or persisting its bytes. The
experimental Broker profile instead gives a Workspace an opaque
project-bound handle and retains its Context secret in an encrypted vault.
Every supported HTTP/HTTPS request is normalized, authorized by OPA, and
enforced by the shared Gateway before forwarding.

Every interactive attachment exposes the constant physical-host loopback HTTP
capability, but that ambient existence grants no authority. Gateway derives an
active Attachment Epoch from host-owned registry state and OPA must allow the
exact Host Loopback effect including URL port.
The resulting Attachment Grant applies to every process in the Workspace and
ends with the owning attachment. It provides no Docker, raw TCP, private-LAN,
arbitrary socket, executable, or persistent policy authority.

The host relay listens on a random TCP port for transport portability, but it
is not an unauthenticated host-service exposure. Each connection must present
the registry-bound 256-bit attachment token before the relay opens its one
policy-granted `127.0.0.1:PORT` target. Only Gateway receives the owner-only registry;
Workspace capability data, OPA input, audit, and review output never receive
the relay port or token. Missing, malformed, stale, or unauthenticated relay
traffic reaches no target bytes.

Adoption is part of this security objective. A boundary that is too difficult
to create or customize will be bypassed by running the agent on the host. The
safe default therefore remains opt-in and deny-by-default, while the user
journey should make isolation, useful denial evidence, and exact permission
growth easier than manually operating Docker, OPA, or host policy files.

## Trust boundaries

Trusted components are the host OS and user, Docker Engine or its Linux VM,
Tobari CLI, Gateway, OPA, and Rego policy. The experimental profile additionally
trusts its reviewed provider drivers, Auth Broker, root-key provider, owner
manifests, and encrypted Context vaults. Every Workspace and
process in it, coding agents, project files, Workspace home, copied opaque
handles, generated code, downloaded packages, request data, upstream responses,
and user/provider text displayed by CLIs are untrusted.

The host also owns each stable Context's creation-time Boundary: immutable
source-access choice, policy mode, normalized Context-owned policy snapshot and
revision, terminal destination/method ceilings, and enabled/disabled
native-readiness choice. They remain secret-free authority metadata in
separate owner-only state; project files, runtime images, Workspaces, and
policy source files cannot rewrite that Boundary. Enabled readiness
independently selects the trusted binary's finite native-readiness overlay,
still bounded by the terminal Context policy ceilings. A binary update can change that overlay for
existing Contexts, but runtime data cannot select or define it.

The same stable Context deliberately has mutable non-Boundary components behind
separate fixed-target host mutations. One exact Runtime binding changes only
through `context runtime set` and affects bound Workspaces on next entry while
preserving their identity and home. Shell and Git session defaults are resolved
for later entry or child-session creation without writing the Workspace home.
Typed bootstrap recipes affect only future Workspace creation. None of these
paths changes source/network authority, retargets an existing Workspace, or
changes standard Workspace-owned authentication.

Only in the experimental profile, the reviewed GitHub, AWS, and Codex host-driver implementations and isolated pup and Claude
Context-runtime drivers are trusted, purpose-limited CLI side effects. Host
drivers select canonical executables outside the project, reject group/world-
writable candidates, bind SHA-256 identity, construct only fixed argv, use
sanitized private state, and accept no
Workspace, repository, manifest, project `PATH`, or request-selected
executable or argument. GitHub uses only its fixed device page and attempts one
host-browser open at most once. Codex uses its native browser login: the
verified child alone owns the loopback listener, dynamic authorization URL,
PKCE state, callback, and exchange; Tobari never receives or opens them. AWS uses only
the reviewed Identity Center or commercial-console flow; Codex uses its
contract-checked native flow. Pup runs from the selected Context image with no
mounts or persistent home, binds an immutable image and executable digest,
accepts semantic version syntax without an exact version allowlist, and is
fixed to Datadog US1 through strict login/status/state capture. Claude Code 2.1.220
runs in a fresh selected-Context-image container with no mounts, project,
persistent home, Broker socket, or Docker socket; Tobari hashes its copied
executable bytes, extracts only the required token, refresh, expiry, scope, and
bounded non-secret subscription/rate-limit values, and discards every other
provider-owned optional credential field before
creating a strict Tobari-owned renewable record. Every driver revalidates executable identity and
performs checked cleanup. Managed profiles and manifest-selected helpers remain
absent.

These controls trust the operator-installed conventional host CLI to implement
its provider login. Path, mode, and digest bind one invocation and detect
in-process replacement; they do not prove publisher provenance or sandbox all
upstream filesystem/network behavior. Tobari's compiled driver contracts bound
argv, environment, captured state, cleanup, and credential commit.

The host Git identity reader is a separate trusted, purpose-limited CLI side
effect. It accepts one canonical Workspace root and can request only global
`user.name` and `user.email` through an absolute Git executable outside that
root. The child process receives only validated host `HOME` and optional
`XDG_CONFIG_HOME` paths outside the root plus fixed locale and Git controls;
`PATH`, loader variables, shell-startup variables, and every other ambient
entry are absent. Repository/worktree configuration, caller-selected keys, raw
config files, source paths, and raw diagnostics cannot cross this boundary.

The following extended topology is experimental; standard omits every Broker,
driver, companion, and vault edge:

```text
host root A (Context-selected ro/rw) ---> Tobari A --+
                                  +--guarded HTTP(S)--> Gateway --OPA--> upstream
host root B (Context-selected ro/rw) ---> Tobari B --+                       |
                no cross-route                           +-- post-allow runtime socket
                                                                  |
host CLI --fixed control exec/stdin--> locked Auth Broker --encrypted Context vaults
host CLI --reviewed fixed credential drivers--> provider acquisition
host CLI --private resident AWS companion--> Auth Broker bridge
host CLI --two fixed global Git reads--> validated identity scalars --> Workspace fallback
```

Each Workspace joins only its dedicated internal network. OPA joins only an
internal control network. Gateway has separate interfaces for every Workspace
proxy network, control, and egress. Auth Broker joins control and egress, has
no TCP listener, uses egress only for compiled Datadog/OpenAI/Anthropic refresh, and never
joins a Workspace network. Only Gateway mounts its runtime Unix socket; host
control uses a separate socket through fixed in-container operations. Provider
acquisition runs from the trusted host rather than through Broker egress.

Each Workspace namespace has a verified output guard and default route through
its exact Gateway project endpoint. The Gateway namespace redirects project
TCP and DNS to local non-root listeners while IPv4/IPv6 forwarding remains
disabled and its forward path drops. A fixed one-shot helper receives only
`CAP_NET_ADMIN` and one verified target network namespace, installs the exact
owned state, verifies it, and exits before entry. It has no mounts, secrets,
Docker socket, host network, or caller-selected executable. No host-global
packet filter is changed.

## Assets

- Host files outside the selected root.
- Docker Engine and socket.
- Workspace-owned authentication state and broker handles inside a Workspace home
  or environment, plus brokered static/renewable credential records in
  encrypted Context vaults and the installation root key.
- OPA policy, decision API, and Gateway management surface.
- Denial of direct Internet connectivity.
- Integrity of request normalization, policy decisions, and audit records.
- Integrity and privacy of Context-owned non-secret identity configuration;
  email is personal data even though it grants no authentication authority.
- Physical-host loopback ports reviewed for one active attachment, including
  protection from unreviewed ports, stale attachment
  grants, cross-Workspace reuse, and later port reassignment.

Files under a read-write source bind are explicitly not protected from its
Tobari. Processes can change or delete that entire mounted root. A read-only
source bind denies create/change/delete and Git metadata writes through that
bind while preserving reads and writable Workspace home/tmpfs. It is a live
bind, not a snapshot: host writes and same-root read-write Context changes may
be observed, and no writable alias of the source is allowed. Same-root and
parent/child-root Tobari intentionally observe each other's overlapping host
file changes, even across Contexts; Tobari does not provide filesystem integrity
isolation for those files. Docker or kernel
compromise, VM/container escape, allowed-destination exfiltration, the
installation-wide baseline policy, semantic authorization of non-HTTP
protocols, covert channels,
same-Tobari process interference, and malware detection are outside the MVP
guarantee. Mutable learned permissions are bound to the host-issued project
principal described below. A tool credential is intentionally available to all
processes in the same Workspace and may be sent to any policy-allowed destination.
A broker handle is also readable by every same-Workspace process, but it is
usable only with its exact trusted Context/project/provider/revision/HTTP
binding and an OPA allow. That does not protect against a malicious process in
the same Workspace exercising an already allowed brokered capability.

The Linux owner-only XDG root-key file does not protect against compromise of
the host user or complete XDG state tree. It separates the key from encrypted
vault files and keeps the primary secret out of Workspaces. macOS Keychain
provides stronger at-rest separation, but a
compromised trusted host, Tobari CLI, Docker Engine, Gateway, Auth Broker, or
root-key provider can still exercise or recover the credential.

## Resource and process boundary

Runtime specs prohibit privileged mode, host networking, the Docker socket,
SSH agent mounts, host home mounts, and added Linux capabilities in every
resident process. The fixed one-shot network guard is the sole exception: it
receives root plus `CAP_NET_ADMIN` only while sharing one already-verified
container network namespace, with all other capabilities dropped. Each
project work container also receives fixed CPU, total memory-plus-swap,
PID-count, and container-log bounds through the infrastructure-owned Docker
specification;
the desired resource contract is part of the spec hash, so drift or an older
unbounded container is recreated before reuse. These bounds do not provide a
quota for the explicitly mounted project root or shape network bandwidth.
The shared Gateway and OPA services use fixed JSON-file log rotation of 10 MiB
per file and three files, plus fixed CPU, memory-plus-swap, and PID ceilings.
The experimental Auth Broker follows the same bounds.
Those ceilings bound shared-service exhaustion but do not provide per-project
fairness inside one shared Gateway, OPA, or Auth Broker.
Tobari uses a non-root work user mapped to the invoking UID/GID where Docker
supports it.
The attached native-login bridge changes none of those runtime settings. After
bounded provider-specific semantic URL recognition it first verifies one
label-owned selected Workspace. A callback-bearing target then binds a host
`127.0.0.1` listener at the validated non-privileged redirect port, accepts one
connection, relays through a fixed Docker exec program plus that port, and
closes with that login or session. Claude Code's exact remote-callback target,
GitHub CLI's exact device target, and TWG CLI's strict device-verification
target instead perform one host browser open after their client-owned
confirmation and create no listener. The closed union accepts only Claude Code
2.1.220's fixed client, redirect, PKCE shape, and complete reviewed scope set;
Codex's reviewed authorization semantics; GitHub CLI 2.96.0's exact device URL or fixed
GitHub.com web-application client, standard HTTPS-login scope ceiling, exact
state shape, and `127.0.0.1/callback` redirect; AWS CLI 2.36.11's exact
commercial-region authorization-code shape, default scope, bounded DCR/state/PKCE
fields, and dynamic `127.0.0.1/oauth/callback` port; TWG CLI 1.2.5's strict
Atlassian activation URL; or pup 1.10.7's exact US1 authorization route,
seven mandatory query fields, an optional single UUID-shaped `dd_oid`
organization hint, bounded DCR/PKCE fields, sorted default-scope subset, and one of its four exact
`127.0.0.1/oauth/callback` ports. A dedicated schema-v1 control stream carries the
request while Docker directly owns the attached terminal;
the channel adds no filesystem, network, Docker, browser, or credential authority,
and preserves Docker's native terminal detection, raw-input cleanup, and resize
path.
Only the selected root and that Workspace's exact XDG-owned home directory are
mounted from the host; the root uses immutable Context-selected read-only or
read-write access while the home stays writable. The project-root resolver rejects filesystem root, the user's
home and its ancestors, and paths overlapping XDG configuration, state, or
shared-profile management directories, Docker sockets, and Docker management
paths. For a project below the host home, the selected root may be nested below
the container home path `/var/lib/tobari`, but the host home itself is never
mounted. A bind mount masks image-layer contents at its destination without
deleting them; runtime compatibility therefore keeps executable and package
assets outside the mutable home. Policy source repositories remain allowed; the active host policy,
principal registry, and encrypted broker vaults are separate trusted
assets and are never selected as project roots. Publishing or applying a source
to active policy is an explicit trusted-host operation; entering that source
repository never changes policy.

A custom Tobari image is untrusted input and receives no additional authority.
The CWD-owned runtime accepts only a locally available image that asserts
runtime API `1` and preserves the built-in image user, entrypoint, and
`io.tobari.runtime-lifetime-command=sleep infinity` capability needed for the
fixed Workspace lifetime command, then independently fixes the numeric runtime
user, read-only root, capabilities, security options,
mounts, guarded network, and health check. Compatibility metadata is
not a signature or provenance claim. Tobari does not grant the image's `CMD`
lifecycle authority: the infrastructure supplies the long-lived command and
runs Bash or exact caller argv through foreground child exec sessions. Direct
argv follows Docker's `--` without shell evaluation and cannot replace the
fixed PID 1 lifetime process. Missing or incompatible image
metadata is rejected before project home, network, or container mutation. Users
remain responsible for image contents and should prefer immutable digest
references.

Executable build identity is public metadata, not runtime authority. It is
limited to the CLI version, a full lowercase source commit or `unknown`, the
compiled resolver channel, and integer required/selected component APIs. It
contains no branch, dirty diff, absolute path, username, environment value,
registry credential, or unreviewed digest. Repository-only recovery text is
gated by the compiled development resolver metadata rather than CWD inspection.

Managed Runtime source is a trusted-host build input. A release resolver
ensures its pinned agent-ready base from embedded source under a source-derived
local tag; contributor development uses the local combined base. `runtime
build` may obtain that declared base only because the user explicitly requested
a host build. Docker receives one immutable Runtime revision snapshot as its
complete build context; Context policy, provider manifests, credential
metadata, encrypted vaults, root keys, secret files, the host home, Docker
sockets, and Workspace mounts are outside it. The generated image must pass the
same compatibility inspection before history append. Editing source or a
failed build cannot replace a Context binding. After explicit Context
selection succeeds, only Workspaces permanently bound to that Context
observe the selected image through their next trusted root-entry reconciliation.
Docker/BuildKit build output is untrusted diagnostic text even though the
complete Runtime source tree is owner-only host input. The explicit build forwards both Docker
streams through visible projection, preserving line structure and concrete
errors while making backslashes, terminal controls/formats, and Unicode line
separators distinguishable. It is not copied into the stable structured fault,
Context manifest, or audit state.
Attached Workspace stdout is also untrusted child text. Only the interactive
Unix terminal path may pass one bounded, complete JSON object/array or
conservative YAML mapping/sequence through the infrastructure syntax-color
projection. The projection preserves the visible child bytes and whitespace,
adds only fixed Tobari-owned SGR wrappers, rejects actual terminal controls,
and passes ambiguous, incomplete, invalid, oversized, tagged, anchored, or
aliased YAML through unchanged. Escaped control text inside a value is not a
terminal control. The relay keeps Docker's child PTY and forwards input and
resize; it never interprets output as a command, policy, URL, credential, or
recovery action. Stderr, redirected output, machine output, and `NO_COLOR`
remain ANSI-free.
The built-in and explicit local bases receive no registry-pull request.
Explicit remote custom bases remain Docker-owned inputs to the requested build;
this keeps the local base boundary independent from a Tobari registry artifact.

The base runtime does not change this boundary. Bundled Claude or Codex is a
convenience rootfs and tool bundle, not
a source of host mounts, host credentials, capabilities, network routes, or lifecycle
policy, and registry provenance does not replace local runtime compatibility
validation.

The canonical base includes integrity-pinned Claude Code 2.1.220 and Codex
0.147.0 and contains no credentials or agent configuration. Its ordinary Workspace
binaries remain untrusted; only the separate mount-free Claude login container
may treat exact Claude as a provider-only acquisition authority. Their workflows verify
the versioned release packages against the checked-in per-architecture
checksums. The combined base declares `NOASSERTION`, is permanently
local-build-only, and its workflow has no package-write permission, registry
login, or push path.

The base retains its GitHub CLI and AWS CLI artifact inventory and associated
integrity/license checks. `kubectl`, `cwk`, `pup`, and TWG are not base-runtime
artifacts. Custom Context images receive no implicit publication authority or
trust; the selected image used for pup acquisition contains no mounted host or
Workspace configuration and receives no Docker socket. A Context narrow projection is
generated separately from its fixed validated non-secret scalars and never
changes image contents.

Each Workspace's bound Context runtime image selector is strict, bounded, and stored in
the owner-only Context manifest. A new Context selects the built-in image until
its manifest explicitly selects another image.
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
Routine startup builds or reuses the reviewed embedded Gateway source under a
content-derived local tag and rejects missing labels, a root default user, the
wrong entrypoint, or a Docker Engine platform mismatch before cluster
resources are created. Contributor source testing uses `task build` and a
development resolver with its own embedded-source-hash local image tags. The
image does not bake in a host UID/GID. The private CA named volume is
mounted only into Gateway and its initialization directory is writable by that
service; the public CA named volume is written by Gateway and mounted
read-only into each Workspace. Gateway opens no root entrypoint and receives no
added capability. Host credential files remain owner-only read-only binds, and
Docker Desktop-specific behavior is outside the current macOS release contract.

The OPA builder never receives an owner-only host policy bind. Tobari reads the
tested `0700`/`0600` aggregate as its invoking host identity, writes one
owner-only temporary tar archive, and streams it through container stdin into
the owned bundle volume. The pinned networkless builder and atomic publisher
operate only on that volume, and staged source is removed before publication.

Auth Broker image code is likewise root-owned and read-only, with a fixed
non-root default user, entrypoint, API/role labels, dropped capabilities,
read-only root, and no host UID baked into the image. Routine startup uses only
the reviewed immutable multi-architecture digest; the explicit contributor dev
image path cannot replace that authority in a normal binary. The writable Context-vault
mount, Gateway-visible runtime-socket mount, private control tmpfs,
and provider projection are distinct paths. The image contains no provider CLI,
credential, provider configuration, handle, root key, or vault.

The canonical Gateway source declares API V1. Its label makes guarded
transparent routing and schema-1 source-principal binding fail closed against a
non-V1 component. The strict Gateway-only lock binds its reviewed service index
to the exact CLI source revision without storing generated digests in source.
The experimental Auth Broker separately declares API V1 but has no release
lock entry. The runtime base is bound by embedded recipe bytes and a
source-derived local tag instead.

All resources carry `io.tobari.owner=default`; per-Workspace resources also carry
the exact stable Workspace ID and a resource role. Destructive lifecycle code
resolves the nearest canonical CWD root, selects exact stored names, and
confirms owner, ID, and role labels before removal. `delete` removes that
instance's persistent XDG home and records after the session-attachment guard;
`--force` explicitly overrides an attached-session warning.
Shared cluster removal is rejected while any Workspace record remains.
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

The complete authority inventory, including Scope, Lifetime, Owner, and
Precedence, is canonical in
[Architecture: HTTP authority scope, lifetime, owner, and precedence](02_architecture.md#http-authority-scope-lifetime-owner-and-precedence).
Security enforcement preserves two closed branches.

For ordinary external traffic, Gateway first establishes a valid
principal/schema/Context. OPA then applies the terminal destination and method
Boundary; one exact-Deny tier containing trusted baseline Deny and remembered
exact Deny with no order between its members; the Context-policy positive tier;
and only while unresolved either Guided remembered Allow or Advanced Rego.
Anything still unresolved fails closed or becomes eligible for exact review.
No lower tier can override a terminal decision.

Host Loopback does not enter that order. Gateway must first resolve an active
principal-owned route and Attachment Epoch. OPA then applies exact Attachment
Deny, exact Attachment Allow, and exact attachment review in that order.
Ordinary Context ceilings, baseline/native authority, remembered Allow/exact
Deny, and Advanced Rego are inapplicable. Attachment decisions are likewise
inapplicable to ordinary traffic. Gateway opens the bridge only after Allow;
the relay revalidates the active principal, epoch, target port, and Allow before
physical-host I/O. Closing the owning attachment closes the relay before route
and grant removal, making stale or mismatched attachment material inert.

Gateway constructs normalized OPA input in mitmproxy. Ordinary HTTP remains
body-free at the request-header hook. A trusted Context-declared exact GraphQL
endpoint instead accepts one positive length no larger than 1 MiB, or an absent
length without transfer/content encoding under the fixed 8 MiB transport cap.
It rejects a complete body over 1 MiB and defers policy until Gateway has
derived only the selected query/mutation type and sorted canonical root fields.
The input includes the host-issued Context/project principal, a structured request
authority, method, path and path segments, multi-valued query, redacted headers,
and an authorization object containing only a non-secret broker provider ID
when a handle has been successfully introspected. Both stable IDs are derived from the local Gateway
interface address and an owner-only host registry. Caller headers, environment,
URLs, session metadata, profile names, and supplied Context/project IDs are not
authorization inputs. Missing, unknown, stale, ambiguous, or mismatched Context
bindings deny before OPA and upstream I/O.
Guided Contexts supply no executable Rego: aggregate generation uses the
current Tobari-owned shared evaluator and tests with each Context's policy
data. Only Advanced Contexts own Rego source; source and runtime documents use
exact schema 1, and every other input shape fails before policy activation.

Secret header values, handles, credential revisions, queries, headers, and
request/response body content are absent from denial audit. GraphQL source,
operation name, variables, aliases, fragment names, directives, nested
selections, arguments, extensions, literals, and body hashes are also absent;
only an exact operation type/root-field coordinate may be retained. Audit retains the
path component, but replaces the whole path with
`/[redacted-auth-handle]` when it contains a Tobari handle marker. Structural
URL/header handle rejections are non-learnable and cannot enter policy
candidate discovery. Gateway enables ordinary request streaming only after
principal, static credential binding or non-secret handle introspection, OPA
decision, and credential application succeed. It
enables response streaming only for a flow that carries authorized-upstream
audit state. A local denial therefore cannot become an upstream request.
Mitmproxy retains an 8 MiB `body_size_limit`; a request or response with a known
`Content-Length` above that value is rejected before the ordinary addon header
hook. Allowed unknown-length ordinary bodies stream without a total-byte limit,
avoiding full-body memory retention while preserving the advertised-size cap;
unknown-length ordinary bodies remain streaming.
Declared GraphQL endpoints reject ambiguous length, transfer-encoded,
compressed, non-JSON, non-UTF-8, or over-1-MiB request forms before OPA
learning, credential resolution, and upstream I/O. An absent length is accepted
only without transfer/content encoding; the fixed 8 MiB transport cap bounds
receipt before the independent 1 MiB complete-body check.

Signed AWS Query and AWS JSON RPC on exact commercial `*.amazonaws.com:443`
authorities are a bounded structural exception. Gateway accepts only `POST /`
without URL query, one complete positive `Content-Length` of at most 8 MiB, a
strict non-streaming payload, and an exact SigV4 scope. Query RPC retains only
wire protocol `query`, signing service, and one valid `Action`; JSON RPC retains
only `json`, signing service, and one signed `X-Amz-Target`. It retains no access
key, region, other form parameter, JSON body field, body, or body hash in OPA,
audit, evidence, or CLI output. Claimed unsigned, ambiguous, changed, streaming,
or unsupported AWS RPC fails closed before learning and upstream I/O. These
tokens are exact request identity only: Gateway and OPA do not load AWS service
models or infer IAM action, resource, read, create, write, idempotence, or retry
safety.

OPA timeout, connection failure, non-2xx status, malformed JSON, missing
fields, unknown decision values, and Gateway exceptions all deny. Plain HTTP
to non-local destinations is denied by the initialized policy. The initialized
policy also requires an explicit port for each supported scheme; learned rules
retain the observed Context/project/scheme/host/port/method/path and optional
GraphQL or AWS coordinate and cannot be used on another Context, project, port, or
scheme. Query and headers may be available to Advanced Rego as additional deny
constraints but never become guided candidate/rule identity.
Ordinary body presence and content are not authorization or learning dimensions;
an exact learned rule covers every body value at its exact
Context/project/scheme/host/port/method/path. Immediately before an upstream connection,
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
an exact entry may originate from the typed raw list or detail screen, while a
template entry must originate from its scope-bearing detail screen; every entry
retains its opaque ID unchanged, belongs to one Context, and passes fresh
snapshot validation. The
one-Context bound preserves one atomic policy-source promotion even if the host
process is interrupted. The authoritative source is one strict
`policy/domains/<canonical-host>/allow.json` and `deny.json` pair per exact
host, never a Context `data.json`. Directory names and every embedded
authority, endpoint, credential binding, and rule host must match; unknown
fields, duplicate keys or rule IDs, non-canonical hosts, wildcards, IP
literals, incomplete pairs, extra files, symlinks, and unsafe permissions fail
closed. Per-domain method data is projected only into that domain's authority,
and explicit deny retains precedence. Display position cannot create authority, staging
writes nothing, cancellation discards the set, wildcard creation is impossible,
and redirected or machine-readable review is read-only. `--watch` is accepted
only with human text and raw terminal ownership; it repeats the bounded read,
never grants or retries, retains the last valid snapshot on failure, and uses
bounded backoff. The alternate-screen presentation remains open across watch
reads but closes on stop, error, and before confirmed mutation output; raw input
mode restoration remains paired at every selector boundary. Refresh retains staged
authority only for an identical candidate ID; stale and same-label replacement
IDs remain undecided. Confirmed Apply returns an authoritative active revision
and exact typed receipt, after which the original running Workspace may issue a
new request. Neither Workspace nor OPA is recreated by this activation.
The watch attention cue is not a desktop/OS integration or policy signal. Its
OSC 9 payload is one compile-time fixed printable-ASCII message framed by fixed
control bytes, and BEL has no payload. The notifier never receives or
interpolates Context names, project roots, hosts, paths, reasons, counts, or
other external evidence, so hostile denial text cannot inject terminal control
data. `auto` selects OSC 9 only for a reviewed terminal identity: exact iTerm2
identity or the conjunction of protected non-empty cmux workspace and surface
identities. Every other identity falls back to BEL. Those environment values
are tested only for presence and never enter the
payload. Tobari never changes tmux or SSH passthrough. Cue failure is
non-authoritative and leaves watch active.
The experimental-only Operator Console is a session-scoped trusted-host presentation,
not a network control plane. `tobari serve` binds TCP4 only to
`127.0.0.1:0`, accepts no remote/fixed-port option, and exits with its owning
foreground process. A cryptographically random 256-bit bearer exists only in
process memory and the initial URL fragment; script moves it to tab-scoped
session storage and removes the fragment before API use. The server sets no
cookie, serves no external asset, emits a restrictive CSP and `no-store`, and
rejects requests unless the peer is IPv4 loopback, Host is the exact selected
listener authority, method/path/content type are allowlisted, and Authorization
contains the exact bearer. Mutation requests additionally require the exact
loopback Origin, a bounded strict JSON body, and one complete decision set.
Browser staging is inert. Apply reuses the fixed-target reviewed mutation,
freshly validates every opaque candidate, has a bounded deadline, never retries
automatically, and returns only the authoritative typed receipt. Opener failure
does not widen binding or authority; the exact URL remains available for manual
host-browser navigation.
Session-close summaries use the same untrusted request projection
and are best-effort host stderr output.

Before any baseline, learned, or Advanced allow, the Tobari-owned evaluator
applies the immutable destination ceiling and resolves one complete method
decision from an exact override or the Context policy default. Method `deny` is
terminal; `allow` enters the Context-policy baseline path; `exact_review` grants nothing
by itself. Terminal denial emits no
permission candidate and causes zero external DNS, Auth Broker resolution, or
upstream calls. The Context policy ceiling cannot be replaced by Context Rego,
learned state, provider metadata, or a Workspace-supplied value. The fixed
agent-ready baseline is not selectable and Context creation can choose a
deny-only or GET-only method policy directly;
enabled native readiness grants the reviewed Claude Code 2.1.220 and Codex 0.147.0
native model/account/bootstrap, first-party capability-discovery, bounded
evaluation, and telemetry effects. Dynamic evaluation matches only one safe
direct-child identifier. At exact trusted MCP endpoints, Gateway accepts one
bounded unencoded JSON-RPC object and exposes only method plus the exact
`tools/call` tool name to OPA and audit; arguments, resource URIs, responses,
and credentials remain absent. Bootstrap/enumeration methods have baseline
authority, while actions remain exact semantic review candidates. These are
Context-wide effects, not executable identity. Exact Deny remains terminal;
downloads, file transfer, acquisition, self-update, malformed/batched MCP, and
unmatched effects receive no baseline authority. Compile-time
`claude_ready`, `codex_ready`, `gh_ready`, `twg_ready`, and `pup_ready` bundles add only the exact native
authentication effects of the pinned clients, including GitHub CLI 2.96.0's
two exact GitHub.com device bootstrap/exchange POSTs and exact GraphQL `query`
/ `viewer` current-user lookup. The latter is bound to a declared endpoint and
cannot authorize ordinary HTTP, mutation, a sibling root, or a mixed-root
document. `twg_ready` adds only exact `POST` device-code, token, and revoke
effects at `auth.atlassian.com`; exact `POST /accessible-products` and GraphQL
`query` / `me` at `api.atlassian.com`; and exact `GET
/cli/manifest.json` at `teamwork-graph.atlassian.com`. The semantic grant is at declared exact endpoint
`POST https://api.atlassian.com/graphql`. That semantic grant cannot authorize
ordinary HTTP, mutation, a sibling root, or a mixed-root document. Atlassian
REST, beta manifest, installer, artifact, update execution, and download remain
unmatched. `pup_ready` adds only exact
US1 `POST /api/v2/oauth2/register` and `POST /oauth2/v1/token` effects at
`api.datadoghq.com`; product APIs, telemetry, revoke, alternate sites, and
neighboring OAuth routes remain unmatched. Bundle
and executable names never enter policy.
New Context snapshots omit these readiness rules. Aggregate generation removes
every form retained in the binary's append-only compatibility history from a
legacy snapshot, then, when enabled, adds only the current binary set.
Destination ceilings, method Deny, and exact Deny remain terminal. An omitted
readiness value resolves to the explicit default without rewriting the
immutable manifest.
One reviewed dedicated compile-time family catalog owns each bundle's pinned
client version, independent current contract revision, and append-only removal
history. Observed candidates cannot extend it. The active aggregate revision must equal the
revision recomputed from current Context sources and that catalog; status marks
a mismatch invalid and Workspace entry performs no downstream reconciliation
until explicit `cluster up` activates the complete candidate.
GET is not classified as safe or read-only, and exact Deny remains terminal
over method Allow.

## Credentials

The standard `passthrough` adapter is the only credential route. Standard has
no normalized provider projection or declared provider binding. It uses
tool-owned authentication state created below the selected Workspace's
`HOME=/var/lib/tobari`; a Context does not contain or copy this state. It redacts client
authentication and cookie values from OPA input and audit, preserves them until
policy allow, then forwards them upstream. It strips proxy and Tobari control
headers and never reads broker vaults. The host-owned
`principal-registry/principals.json` schema 1 registry binds each
Context/project pair to one exact owned project network, Workspace source
endpoint, and Gateway endpoint. Gateway derives the transparent-ingress
principal from the kernel-observed source endpoint; duplicate, missing, stale,
and ambiguous bindings fail before OPA. Its dedicated directory is mounted
read-only into Gateway; broker state is not included in that mount.

In standard passthrough, `Authorization`, `X-API-Key`, cookies, and other client
authentication are forwarded only after allow; `Proxy-Authorization` and
Tobari session control headers are removed. Cookie and Set-Cookie values may
remain part of the authorized application flow but are excluded from OPA input
and Tobari audit logs. There is no managed profile or secret-file fallback.
The native-login bridge is transport, not a credential adapter: it cannot read
a Workspace credential file, and callback bytes or device codes never enter
OPA, Gateway, audit, denial evidence, or durable Tobari state. Its exact host
browser effects grant no Workspace HTTP authority. Separately, the immutable
agent-ready readiness bundles grant only their expanded exact HTTP effects.

The experimental development profile's Auth Broker stores one static primary-secret record per Context/provider in
`auth/contexts/<context-id>/vault.enc`. The schema-1 AES-256-GCM envelope
contains a schema-1 payload, uses a random 12-byte nonce, and binds schema plus
stable Context ID as authenticated data. All parent directories, files,
ownership, modes, and symlink status are checked before use; updates use a
durable atomic replace. The broker starts locked and retains the 32-byte
installation root key only in memory. macOS stores that key in Keychain service
`io.tobari.auth-root.v1`, account `tobari`; Linux stores it as owner-only XDG
state `auth/keys/root.key`. A missing key alongside a vault is never replaced
automatically. Public auth results name `macos_keychain` or `xdg_file`;
`linux_xdg_file` remains an infrastructure/doctor-only label.

The daemon exposes strict 64 KiB schema-1 NDJSON over separate control and
runtime Unix sockets. Key and import/login bytes enter through bounded stdin
after a declared length; they never use argv or environment. The live handle
index is SHA-256-only. Raw handles persist only inside authenticated ciphertext
and bind Context, project, provider, credential revision, exact HTTPS target,
source header, and source format. Login, replacement, and logout atomically
revoke every old handle for that Context/provider.

Only in the experimental profile, Gateway recognizes exactly one handle position from the owner-only normalized
provider projection. It rejects URL, cookie, header-name, unsupported-value,
and ambiguous occurrences, removes the placeholder, and performs non-secret
introspection before OPA. Denial makes zero resolve calls. Allow permits exactly
one same-revision resolve, one declared header replacement, and one upstream
attempt. At the same declared header or AWS signing binding, a real Workspace
credential is removed and rejected before OPA as non-learnable HTTP 403
`broker_auth_required`. A copied, malformed, stale, revoked, ambiguous, or
mismatched handle returns secret-free HTTP 403 `credential_handle_invalid`; a
locked, unavailable, timed-out, or invalid broker returns HTTP 503
`credential_broker_unavailable`. Neither failure forwards the handle or falls
back to passthrough. Compatibility passthrough is selected only when no
declared binding and no Tobari-looking marker exists in any inspected position.

The request body is never a credential source or replacement surface. Tobari
does not search arbitrary body bytes. Because a Workspace can read its own
handle, a malicious Workspace can include those bytes as ordinary payload when
the surrounding L7 effect is allowed; this grants no broker authority but is
outside Tobari's payload-exfiltration guarantee. The handle is not the primary
secret, but it remains a scoped bearer capability that should not be logged or
published. The real brokered primary secret remains unavailable to that
Workspace.

Provider manifests contain no secrets or executable paths. Exact schema-1
owner static providers and the closed reviewed built-ins normalize into
projection schema 1. The schema accepts
bounded handle templates and exact HTTPS/header transformations; it rejects
target/projection collisions and prohibits user manifests from overriding the
any built-in or selecting a helper, dynamic record, refresh, signer,
supplemental header, executable, argv, environment, policy, or business
operation. Reviewed built-in drivers execute on the trusted host under their
fixed contracts, except Anthropic's exact client runs in the isolated selected-
Context-image login container. Owner providers use protected non-terminal stdin import only.
A terminal stream is refused
before reading. Provider collections with overlapping exact
scheme/host/port/source-header/source-format recognition fail completely as
`ambiguous_provider_http_binding`; no partial projection becomes active.

Datadog, OpenAI, Anthropic, Chatwork, and the experimental AWS capability are
implemented only through the closed reviewed plan union. Standard projection
cannot select AWS. Dynamic records, Datadog/OpenAI/Anthropic refresh, AWS
signing/companion, OpenAI supplemental-header ownership, and exact-version
Codex/Claude drivers cannot be selected or extended by owner manifests.
The renewable-session implementation is an immutable compiled registry, not a
plugin surface: provider adapters can parse and refresh only their bounded
state and return one typed result. Broker core alone can validate handle scope,
persist or clear the durable execution barrier, serialize one record, compare
the pre-call snapshot, write the Vault, rotate revisions, or revoke handles.
Registry closure, exact membership, and the absence of Broker-state authority
from adapters are executable test claims. AWS companion/signing and trusted-host
acquisition stay outside this adapter contract because they cross different
trust and request-interpretation boundaries. AWS signing therefore has its own
one-member compiled mechanics registry: it may parse and sign one bounded
request and validate one correlated companion result, but it cannot call the
companion, acquire a record lock, persist or clear a barrier, read or write a
Vault, compare mutable state, or commit refreshed state.
Persisted record validation is a separate immutable compiled union. Its
contracts can validate and construct only bounded record data; they cannot
open, decrypt, encrypt, replace, or locate a Vault. `VaultStore` authenticates
the Context-bound envelope before record validation on load and validates the
complete record union before encryption on save. Unknown kinds, provider-kind
mismatches, and binding-shape mismatches fail closed. Static records remain
provider-generic only because owner-defined schema-v1 static providers are an
intentional non-executable extension boundary.
Control login plans are likewise an immutable compiled union rather than an
acquisition plugin. They may select only one fixed request shape, credential
kind, driver allowlist, and reviewed record constructor. They receive no
Broker/Vault authority and cannot run a helper, read an executable path, or
perform provider I/O. The Broker independently validates the exact control
request and raw length, validates renewable state where applicable, and alone
serializes the resulting record mutation and handle revocation.
Gateway reviewed credential profiles are a separate immutable compiled union
over non-secret projection and response metadata. They cannot observe or mutate
an HTTP request, call OPA or the Broker, handle a secret, or select runtime
code. Owner manifests cannot select these profiles: only an exact compiled
provider/credential/helper combination can satisfy one. Gateway core retains
all pre-policy stripping, fail-closed recognition, post-allow call ordering,
revision matching, and final credential application.
The cross-language reviewed-provider capability fixture is a test oracle, not
configuration. Strict Go and Python readers reject unknown fields, membership
drift, unregistered capability names, and mismatched boundary projections, but
the fixture is absent from both runtime images. A provider addition must still
change and review the relevant compiled registry, manifest, protocol/record
shape, and capability implementation; changing the fixture alone grants no
execution, refresh, signing, supplemental-header, Vault, or Gateway authority.
Managed profiles, arbitrary OAuth orchestration, provider SDK inference,
multiple provider accounts, and remote revocation remain unsupported. A missing, malformed,
ambiguous, or stale principal registry entry denies before broker resolution,
OPA authorization, and upstream I/O.

## Mutation policy

An `EffectRead` has no first-use initialization authority. Missing XDG state is
reported as absence or a display-only synthetic default without a Context ID;
it cannot authorize a Workspace, credential, policy, key, vault, or Docker
operation. Unsupported-version, corrupt, or unsafe stored input
fails closed. The only read-side filesystem mutation is bounded cleanup of a
pre-existing validated mutation journal; that exceptional path may create the
project recovery lock only to serialize cleanup, but a read never creates the
journal itself. Fresh and ordinary reads create no lock. First durable
initialization remains behind a fully validated create/write intent.

`migrate apply` is the only predecessor-state authority. It is a fixed-target
`EffectWrite` over the installation-local Context/Runtime collection and uses
strict JSON decoding, duplicate-key rejection, bounded owner-only regular-file
checks, exact digests, fixed legacy identities, source-drift revalidation, and
atomic replacement. Unknown fields or modes fail before Context writes. Its
backup contains only the non-secret authority files it replaces or removes and remains
owner-only. Workspace homes, project and instance records, learned domain
rules, credential stores, running containers, and Docker networks are outside
the mutation boundary. A legacy custom Dockerfile crosses only the existing
bounded managed Runtime build boundary; it grants no additional execution or
network authority.

`runtime create` is a host-only create in the installation Runtime catalog.
`runtime build` is a host-only catalog write. It rejects symlinks, special
files, escaping paths, group/other permission bits, more than 1,024 regular
files or 256 directories, a regular file over 32 MiB, and a total over 64 MiB,
then builds only a private immutable snapshot of the complete source tree. The
adapter streams source bytes into the snapshot and semantic hash with a fixed
buffer, so the 64 MiB input ceiling is not also a whole-source heap allocation.
Validation faults expose a bounded quoted relative path and reviewed
actual/limit or owner-only facts, never an absolute host path, source bytes, or
private cause.
Compatibility and image-digest checks complete before the successful revision
is appended. Neither command mounts Runtime source into a Workspace, accepts a
secret or arbitrary image-name override, or changes a Context. A build failure
is therefore a safe retry point: history and every Context binding remain
unchanged. `context runtime set` separately revalidates an existing ready exact
revision before replacing one Context binding.

When their primary selector is omitted, terminal Runtime Review performs only
typed Runtime/Context reads before explicit Build or Apply. Build candidates
come only from managed Runtime summaries; binding candidates come only from the
built-in standard revision or validated successful history. An omitted current
Context is rebound to the exact persisted name shown during Review, preventing
a concurrent current-marker change from retargeting Apply. Non-interactive,
machine-readable, canceled, unchanged, invalid, or failed Review paths make
zero Runtime-build and Context-binding mutation calls. Fully specified direct
mode reaches the same application invoker without Review.

Shared lifecycle mutations target one catalog-declared `tool_local` cluster.
The root command uses the catalog-declared current-directory fixed target to
read and present containing Workspace candidates, then creates or reconciles
one Workspace only after an explicit choice and a fresh locked check. An
exact current-root record is reused directly; a nested root requires the
explicit create-here choice. During interactive entry, the CLI may precede
that CWD mutation with the separately cataloged cluster create when observation
is not ready; first use may also compose Context-catalog create and optional
Context-runtime create actions. Each action enters
its own application invoker with its own validated intent, target, impact, and
mutation-complete output; root cannot combine or weaken them. Non-interactive
root invocation and Context-wizard cancellation perform none of those setup
mutations. A later-stage failure preserves an already confirmed earlier
mutation and reports reconciliation rather than claiming the whole sequence is
safe to replay.
Project runtime reconciliation does not remove authority merely to prove that
an already matching Workspace is reusable. It first performs a read-only exact
ownership, endpoint, specification, source-access, connectivity, and health
check. Only that complete match may retain the old binding while both guards
are revalidated and the current binding is atomically refreshed. A mismatch
closes the principal before Docker repair, and cancellation continues to flow
through the one caller context; interrupted drift remains fail closed for
explicit `cluster up` and never retries Workspace entry.
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
marker. It does not touch Docker, the aggregate projection, an existing Workspace,
or any enforcement authority. `context create` likewise does not start Docker.
Its complete or partially prefilled wizard is allowed only with text
success/error output and terminal stdin/stderr; supplied values remain bound
while it validates every omitted stage before one mutation. Redirected and JSON
creation require the complete direct input group and never prompt. Cancellation,
partial machine input, and terminal failure perform no mutation. A composed method Deny
filters only positive baseline entries that would otherwise exceed the new
method ceiling; it cannot widen the independent destination ceiling or remove
exact Deny precedence. An explicit `cluster up` validates and activates the new
all-Context candidate.

`context delete` is a lifecycle-locked trusted-host destructive write against
the fixed Context catalog. It rejects the foundational default, current
Context, and any Context ID referenced by durable Workspace state before
removal. It removes only the exact validated owner-only Context directory and
Context-ID authentication directory. It has no force path, never chooses a new
current Context, and never removes project roots, Workspace homes, or shared
runtime images. Existing shared state requires explicit aggregate
reconciliation.
`config shell` and `config git` are trusted-host fixed-target writes to
owner-only Context configuration. A terminal staged editor may complete a
wholly omitted setting group, but it shows typed complete current state and
performs no write before Apply. Shell validates every distinct staged row
before one atomic manifest replacement. Apply is bound to the Context name returned by that read, even if
another process changes the current/default marker during review. Explicit-
empty Context input, partial direct input, and redirected, JSON, canceled, or
invalid editor attempts make zero mutation calls. Direct and interactive modes
reach the same application policy and atomic store boundary.

`config bootstrap aws` is a distinct fixed-target write whose authority ends at
one secret-free future-Workspace recipe. Infrastructure reads only exact host
`~/.aws/config` through a bounded regular-file parser. The selected profile and
referenced `sso-session` reject duplicate and unknown keys; credentials, SSO
token caches, helpers, executables, includes, alternate paths, and arbitrary
dotfiles are never read. A terminal preview exposes only semantic field names
and revisions, and Apply rejects source drift before atomic manifest write.
Projection creates only private `.aws/config` in a fresh Workspace home before
publication. No refresh or reconciliation path may write an existing
Workspace home.

The interactive Context wizard's Workspace-bootstrap step does not inspect host
AWS or Kubernetes configuration merely by being shown or continued. Only its
explicit Configure from host choice or later Workspace-bootstrap editing
invokes a read-only candidate port. Infrastructure reads the same fixed bounded
file once, resolves
each profile through its referenced SSO session with the strict preparation
resolver, and returns no credential or cache material. Individually invalid
profiles are non-authoritative unavailable data; malformed, duplicate, or
unsafe whole-file state fails closed with no partial candidates. Final Create
revalidates the selected semantic bundle before mutation and never silently
drops a reviewed bootstrap selection.

The dependent `kubernetes_eks` adapter applies the same boundary to fixed host
`~/.kube/config`. Kubeconfig is untrusted executable and credential-bearing
input, not a file-copy source. The parser resolves one explicit context and
accepts only an inline certificate-authority bundle, one canonical commercial
EKS HTTPS origin, optional namespace, and exact AWS CLI `eks get-token` argv
whose sole `AWS_PROFILE` equals the Context AWS profile. Tokens, passwords,
client keys/certificates, auth providers, proxy or insecure-TLS settings, file
references, role arguments, aliases/anchors, unknown fields, other exec commands
or environment, alternate paths, symlinks, and unsafe or oversized files fail
before mutation. Projection re-encodes canonical private JSON kubeconfig; it
does not execute or copy the host entry. This grants no Kubernetes endpoint or
AWS API effect, and Workspace-owned AWS SSO remains the only credential source.

Wizard EKS discovery uses the same parser and resolver, is scoped to the exact
reviewed AWS semantic revision, and performs no network access or execution.
Only available typed candidates can be selected; display labels, ordering, and
adjacency cannot establish profile compatibility.

Shell accepts only `PS1`, `TERM`, `COLORTERM`, and `NO_COLOR`, never enumerates
the host environment, and resolves `inherit` only when a future child shell is
entered. `PATH`, `HOME`, `BASH_ENV`, `ENV`, `PROMPT_COMMAND`, credential names,
host startup files, and host home mounts remain outside the boundary. Values
are bounded, valid UTF-8 without NUL, and Bash-quoted when assigned by the
CLI-owned prompt hook. Prompt contents may still exercise Bash prompt
expansion inside the untrusted Workspace; they gain no host authority or
Gateway permission.

Git owns only an atomic `user.name`/`user.email` fallback. New
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
vault I/O. Standard login accepts only the installed reviewed GitHub, Datadog,
OpenAI, or Anthropic driver union. Anthropic alone uses a fresh mount-free
container from the selected compatible Context image; interactive omission
opens only its bounded selector. The experimental compile-time profile
additionally activates AWS and its `identity-center|console` methods. Import reads one
bounded secret from non-terminal stdin only under the ordering above. One
credential belongs to one Context/provider, and every permanently bound project
is eligible for a distinct handle only on its next matching Workspace entry.
Replacement and logout atomically revoke prior handles. No auth mutation
rewrites a running Workspace process, calls policy, grants a network permission,
or makes logout a claim of remote provider revocation. Confirmed output never
assumes Workspace re-entry from provider configuration. Only an exhaustive,
exact Context/project registry and Broker binding comparison may mark a
Workspace `ready` or attach its validated root plus Context-bound re-entry argv
to `missing` or `stale`. Unreadable enumeration, registry, or binding state
remains `unavailable` or `unresolved` and carries no action. A `no_change`
logout claims no removal, revocation, or projection change. Logout's next entry
after a confirmed change omits the environment
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
`policy allow`, `policy deny`, and `policy reset` are
access-changing writes bound to opaque references. Discovery never mutates. An
allow reference identifies one retained validated denial that OPA marked
exact-rule learnable; a
deny reference identifies one retained validated denial and binds its exact
Context/project/scheme/host/port/method/path plus optional GraphQL or AWS coordinate;
a reset reference identifies one current CLI-owned
learned Allow or exact Deny and removes it, returning the effect to default
deny. Scheme, cluster, and credential-binding failures never become permission
candidates. Ordinary body presence and content neither disqualify nor split a
candidate. Declared GraphQL documents split only by selected operation type and
canonical root field. Classified AWS RPC splits only by wire protocol, signing
service, and exact wire operation; every other document and payload detail is excluded.
Host-authored baseline denies are terminal, excluded from both the actionable
queue and the reversible decision inventory, while their bounded audit records
remain visible. CLI-owned exact Deny rules remain terminal in enforcement but
are visible in `policy rules` and can be explicitly reset; reset never makes
the request allowed.
The mutation rejects stale or ambiguous references, unsafe policy files,
malformed learned data, and failed preflight tests before the atomic policy
write.

Routine source writes hold both an in-process mutex and a cross-process file
lock. Tobari writes and validates a complete sibling `domains/` generation,
fsyncs it, records a digest-bound durable journal, and promotes it by directory
rename while retaining the prior complete generation for recovery. Readers
reject any live journal, missing generation, incomplete pair, changed snapshot,
or ambiguous recovery layout. Recovery accepts the candidate only when the
matching immutable aggregate revision is already durable; otherwise it restores
the verified original. A concurrent direct host edit is never overwritten by
guess. The generated content-addressed OPA projection may contain one internal
`data.json`, but Workspaces cannot edit either source or projection.

Learned rules never broaden a Context, project, scheme, host, port, method, or
GraphQL/AWS coordinate. A reviewed HTTP path template may broaden only one explicit
non-empty raw `{id}` segment after two distinct compatible examples while
preserving every literal segment. Baseline and exact
deny rules remain terminal; an exact deny wins over a learned allow for the
same Context/project/scheme/host/port/method/path. Prefix learned rules,
compaction proposals, wildcards, regexes, multiple placeholders, and automatic
observation-derived authority do not exist. Percent-encoded segments,
backslashes, empty/dot segments, ambiguous inference, and GraphQL templates
fail closed.

## External calls

Gateway applies finite OPA and upstream timeouts and performs one upstream
attempt. It does not retry requests because arbitrary HTTP methods and bodies
may be unsafe or non-idempotent. Redirect handling remains in the proxy flow and
each resulting request is independently authorized.

Gateway applies a finite broker-socket timeout and performs at most one
introspection plus one post-allow static resolution, token selection/refresh,
or AWS signing action. It never retries or performs a secret-bearing plan
action on deny. The built-in GitHub host driver runs one fixed `gh auth login`,
followed by one bounded active-account status capture and one
token capture in a private configuration directory. It recognizes the one
fixed device URL; opener failure leaves manual navigation and is not a provider
mutation failure. The driver performs no Git configuration, pagination, or
automatic retry; user cancellation or any failed capture preserves the
previous Context credential. AWS/pup/Codex/Claude drivers and the fixed
Datadog/OpenAI/Anthropic post-policy transports follow their separately bounded contracts
and never provide a general provider-call surface.
The Codex visible-output boundary recognizes only the exact reviewed reset,
muted, and accent SGR sequences and regenerates them from Tobari-owned styles
for an interactive terminal. `NO_COLOR` strips that closed vocabulary, unknown
controls remain visibly escaped, and neither presentation path changes URL
recognition. A dynamic OpenAI authorization URL is never a Tobari browser
target; the verified Codex child owns its open attempt and fallback guidance.
That sentence applies to the experimental host-acquisition driver. In the
separate standard attached Workspace, ADRs 0046, 0048, 0050, and 0055 permit the strict
Claude Code, Codex, GitHub CLI, AWS CLI, TWG, and pup native-login URL union only. GitHub CLI's
device prompt remains visible; GitHub CLI invokes the attachment-scoped opener
only after native Enter, while its callback-bearing path invokes it immediately.
Its exact validated device URL may then open once with no listener, or its strict callback URL
may open once with one opaque relay. Exact default `gh auth login`
uses fixed GitHub.com HTTPS argv with prompting and the attachment-owned opener,
so native Enter remains required; every other argv
retains native input.
Callback or provider failure remains the child's login result. GitHub
Enterprise hosts, SSH-key-upload scope, and caller-added scopes cause zero
browser/listener/relay effects.
TWG accepts only a dedicated opener request carrying a strict
`auth.atlassian.com/oauth/activate` URL with one bounded unreserved
`user_code`; it opens once after TWG's native confirmation and never creates a listener. Tobari does not
intercept Enter, `c`, or other child input and performs no clipboard action.
Pup accepts only the reviewed US1 DCR/PKCE authorization shape and binds one of
ports 8000, 8080, 8888, or 9000 before browser open; it relays one opaque
callback only to the selected Workspace. Caller-added scopes, alternate sites,
and changed callback targets open and relay nothing.
AWS SSO accepts only the pinned commercial-region authorization-code shape and
default scope, binds its URL-selected non-privileged port before browser open,
and relays one opaque callback to the selected Workspace. Other partitions,
scopes, callback targets, and neighboring OIDC routes open and relay nothing;
`--use-device-code` remains AWS CLI-owned recovery.
For exact Claude 2.1.220, the separate Context-runtime boundary consumes only
the reviewed opening, OSC 8 link, browser-result, and paste-prompt events. It
opens the exact validated HTTPS URL once, hides it after successful host open,
retains it on opener failure, emits the no-newline prompt immediately, and
relies on the exact Claude prompt's non-echoing terminal mode rather than
hiding the next child line. Tobari never emits entered input. The one-shot
container bypasses the Workspace CA-waiting entrypoint with fixed
`/usr/bin/tini -- /usr/bin/sleep infinity`; otherwise the deliberately absent
CA mount would terminate acquisition after ten seconds. Its raw-TTY projection emits explicit CRLF for
fixed and control-safe pass-through lines, preventing cursor-column state from
changing the meaning or readability of later instructions. Exact Claude cursor
hide/show controls are consumed and closed with one Tobari-owned cursor-show;
unknown controls remain visibly projected. Fixed pre-prompt guidance explains
that authorization may take a moment after Enter, and fixed progress appears
before credential capture without reflecting the pasted value. Tobari does not
wrap, inspect, or pump Claude login stdin; the original terminal reader reaches
Docker unchanged. Authorization-request and granted OAuth scopes are validated
as bounded, duplicate-free RFC OAuth scope tokens, but their provider-owned
names are not compiled into Tobari. The grant must be a subset of the observed
request and is then normalized; response ordering grants no authority. Refresh
and Workspace projection preserve that same set, and refresh scope drift fails
closed. Capture also preserves only the bounded non-secret native
subscription-type and rate-limit-tier labels; their values are not compiled
into Tobari and every other optional native field is discarded. Workspace
receives those labels plus a fixed public `dummy-value` refresh sentinel so
Claude 2.1.220 does not misclassify the handle projection as expired. The
sentinel is neither a secret nor a recognized handle, and the real refresh
token remains Broker-only. Workspace reconciliation also merges only
`hasCompletedOnboarding: true` into a private regular `.claude.json`, preserves
all unrelated values, and removes only that registered field with the
Anthropic projection. Malformed, duplicate-key, oversized, symlinked, or
non-private targets fail closed, and owner manifests cannot request mutable
JSON merging. Timeout retains
precedence when checked cleanup also fails; setup, authorization, output,
native-state capture, and cleanup-only failures use distinct fixed faults and
never disclose child output or credential content. Native-state failures expose
only one compiled diagnostic-stage name and never retain a raw provider cause.
The shared host-CLI resolver inspects at most 256 distinct absolute PATH
candidates. It never executes a rejected shim, accepts no relative or empty
current-directory entry, and returns only the first candidate whose canonical
executable already satisfies the reviewed trusted-root and mode boundary.

Inherited Git identity reconciliation performs at most two host Git calls, one
per exact key, with one attempt and a finite timeout each. It performs no
network call or retry. Exit status 1 with no value means absent; an absent or
incomplete pair produces no fallback, while execution, timeout, framing, size,
encoding, or control-character failures preserve the previous projection and
fail before Docker mutation. Published failures contain neither raw stderr nor
identity values.

## Logging

Trusted-host login availability failures expose only a fixed diagnostic stage
identifier through the existing CLI fault. They do not log the selected
executable path or digest, temporary home, raw process error/stdout/stderr,
device code, auth state, or credential material.

Audit JSON includes timestamp, request ID, cluster, stable Context ID and name,
the schema-V1 `project_id` carrying Workspace ID, safe project root, host, port,
method, redacted path, an optional
GraphQL operation type and one root field or AWS wire protocol, service, and operation, decision,
reason, optional broker provider ID, upstream status, and duration. It
emits no query or headers. Ordinarily the redacted path is the URL path
component; if that component contains a Tobari handle marker, the whole value is
`/[redacted-auth-handle]`. Provider identity is non-secret metadata; secret values,
Git identity values, and raw bodies are excluded. URL/header handle-structure failures remain
non-learnable and cannot become policy candidates.
For brokered requests, the provider ID may be retained as non-secret adapter
metadata, while the handle, credential revision, vault data, and resolved
primary secret are excluded. Provider metadata never grants permission and
does not inherit a broad static host/method allow; the brokered request remains
a learnable denial until an exact Context/project-bound L7 rule exists.
CLI `cluster logs` reads only a bounded component-log window and does not add
unredacted diagnostics. `cluster denials` projects only validated deny records
and preserves only non-secret broker provider metadata. A denial-shaped record
that lacks a valid trusted Context/project scope or otherwise violates the
typed audit contract is skipped and counted; its raw contents and parse cause
are not reflected, and it cannot become a policy candidate. Read-only policy
candidate commands aggregate exact Context/project/scheme/host/port/method/path and
optional GraphQL- or AWS-coordinate proposals from
that evidence, treating reason, status, request identity, timestamps, and
broker-provider display evidence as non-identity fields. The latest evidence
and bounded observation count do not grant authority. `policy review`
presents the same exact machine queue plus domain-typed inert `{id}` proposals
after two distinct compatible HTTP paths. Its TTY detail screen stages an
explicit template-Allow, observed-exact Allow, or pending-exact Deny, retains
every unchanged opaque review-item reference, and applies the reviewed set once
after final confirmation and fresh proposal reconstruction. Staging
is inactive authority; `policy rules`
projects every current CLI-owned learned Allow and exact Deny, and its TTY flow
delegates one unchanged opaque reference to `policy reset`. All redirected and
machine-readable paths remain read-only. Observation alone never changes
authority; every allow, deny, or reset is an explicit
reference-bound mutation.

Shell completion is also observational. The generated zsh adapter performs no
startup write and invokes only the public bounded `completion candidates`
read. Static candidates come from validated public catalog literals. Dynamic
candidates contain only validated Context names, Runtime names, and ready
Runtime review selectors; they exclude credentials, opaque action references,
store paths, manifest contents, Docker state, and network results. Candidate
collections have fixed word, count, and byte bounds. Empty values, duplicates,
invalid UTF-8, tabs, line breaks, NUL, and Unicode line separators fail before
the two-column TSV boundary. The adapter discards candidate-command stderr and
treats a failed read as no candidates, never as shell source or authority.

## Enforcement

| Claim | Enforcement |
|---|---|
| Shell completion cannot become startup mutation, command-registry drift, or structural injection | Public read-only completion commands, catalog-owned typed sources, narrow validated Context/Runtime read port, fresh-XDG zero-write canary, bounded request/output tests, hostile TSV candidate rejection, and a static adapter with no embedded registry |
| Operator Console cannot become remote control or alternate policy authority | TCP4 `127.0.0.1:0` binding, exact peer/Host/Origin/bearer/method/path/content-type checks, fragment-to-sessionStorage bootstrap, no cookies/external assets, CSP/no-store headers, bounded strict bodies/timeouts, cancellation shutdown, inert staging, and one canonical reviewed-apply delegation with zero automatic retries |
| No direct Workspace egress | Per-Workspace internal topology, forwarding-off sysctls, forward-drop and namespace-guard inspection, Gateway raw-protocol/UDP/QUIC call-count tests, and Docker integration canaries for raw TCP, control-API reachability, and Gateway/OPA outage paths |
| Transparent denial performs no pre-policy external I/O | Non-recursive synthetic DNS tests plus Gateway DNS/resolver/upstream call-count canaries for denied, malformed, raw TCP, non-HTTP TLS, UDP, and QUIC traffic |
| A network guard cannot expand into a persistent privileged service | Exact fixed helper argv, verified namespace ownership, read-only/no-mount/no-secret/no-Docker-socket assertions, sole `NET_ADMIN` capability, and guard-before-entry ordering tests |
| A Workspace cannot access OPA or peers | Separate internal networks and integration test |
| OPA outage denies | Gateway unit and integration tests |
| Host-managed secrets stay outside Tobari; tool-owned state stays in its home | Mount-spec tests and integration canaries |
| Brokered primary secrets stay outside every Workspace and OPA | Socket/mount topology tests, encrypted-vault tests, Gateway canaries, and integration log/output scans |
| Broker handles cannot cross Context, project, provider, revision, or HTTP binding | Broker introspect/resolve negative tests, principal-derived Gateway calls, rotation/logout tests, and cross-Workspace integration |
| Policy denial cannot perform a brokered secret action | Gateway call-count and ordering tests proving zero resolve/refresh/companion/signing calls before or on deny and exactly one reviewed action after allow |
| The broker restarts locked and cannot silently replace a missing root key | Restart/unlock tests, Keychain/XDG provider tests, and missing-key-with-vault rejection |
| Provider manifests cannot become executable or ambiguous authority | Strict schema/collision/path/header tests, owner-only XDG loading, and built-in override rejection |
| Provider login cannot turn visible text into arbitrary browser execution | Conventional non-project executable selection, identity/digest recheck, fixed argv/environment, bounded browser/PTY projection, checked cleanup, cancellation, and provider-specific negative tests |
| Native Workspace login cannot become generic host ingress or browser authority | One fresh compile-time registry with exact driver-ID/callback-mode coverage and malformed/ambiguous-definition canaries; a shared closed query-field schema with invalid-definition, requiredness, singleton-cardinality, unknown-field, and validator-dispatch tests; closed Claude Code/Codex/GitHub CLI/AWS CLI/TWG/pup semantic URL-schema tests with exact mandatory fields, individually reviewed optional selectors, exact OAuth clients or bounded DCR IDs, reviewed scope ceilings, callback shapes, and state bounds; AWS commercial-region/default-scope/partition canaries; complete pup 1.10.7 scope-ceiling, optional UUID-shaped `dd_oid`, and four-port canaries; one binary-owned read-only opener; exact `BROWSER`/`GH_BROWSER`/`xdg-open` projection; dedicated schema-v1 Unix-socket and non-TTY Docker exec protocol tests; duplicate-key, unknown-field, malformed-version, oversized-target, replay-budget, neighboring-target, and ownership canaries; zero-listener device/remote-callback opens; dynamic non-privileged host-loopback-only one-shot callback relay; opaque callback canaries; port-collision failure; direct Docker terminal ownership; and session cleanup |
| Unsupported credential mechanisms cannot remain dormant | Catalog/state/dependency/image-content tests reject managed profiles, owner-selected dynamic plans, arbitrary helpers, compatibility readers, and provider CLIs inside Broker |
| Agent-ready tools retain reviewed identity without Tobari redistribution | Base-runtime locks/checks for GitHub CLI, AWS CLI, Claude Code, and Codex; version smokes outside Workspace home; local missing-image build tests; workflow and release canaries reject every base registry write/login/push path |
| Secret headers, queries, handle-bearing paths, and bodies stay out of logs | Gateway redacted-path/header-absence tests, non-learnable structural-rejection tests, and log scans |
| Declared provider bindings are Broker-required | Direct bearer/raw and SigV4 canaries return `broker_auth_required` before fallback/Broker/OPA/DNS/upstream; valid handles retain policy-before-action ordering; an undeclared binding retains compatibility passthrough |
| Broker fallback cannot accept a Tobari-looking handle | Undeclared-binding and marker-absence fallback tests plus malformed, misplaced, ambiguous, and binding-mismatch fail-closed canaries |
| Cluster cleanup preserves authentication authority until explicit logout | Down/purge tests proving vault and root-key preservation plus exact logout/revocation tests |
| Doctor observes but never repairs authentication state | Fixed-DAG dependency fixtures, recording-runner Docker-argv allowlists, fresh/unsupported-tree content snapshots, and filesystem canaries for provider, root-key, vault, broker, and project-binding diagnostics |
| Only owned Docker resources are removed | Label validation and fake-runner tests |
| Attached sessions are not removed accidentally | Exact work-container Exec ID observation, guard-before-delete tests, and explicit force-override tests |
| Each root and XDG home are its Tobari's only host write scopes | Mount-spec and path-containment tests |
| Ambiguous CWD selection cannot mutate before a valid choice | Typed candidate snapshot, locked stale-choice revalidation, and zero-call cancellation tests |
| One Workspace cannot consume unbounded CPU, memory, PIDs, or container logs | Fixed create-argv and spec-hash tests plus runtime HostConfig assertions |
| A custom image cannot expand its runtime specification | Compatibility inspection, fixed create-argv tests, and integration test |
| Selected Context pup cannot become an ambient host helper | Runtime API and immutable-image checks, bounded semantic version observation, Docker-streamed executable digest, fixed argv/status/state capture, no mounts, and no host/base fallback |
| Project metadata cannot become a second runtime boundary | Context-only image resolution, ignored-project-metadata regression test, and fixed runtime adapter |
| OPA cannot rewrite Context policy | Read-only mount-spec test |
| Tested host policy activates across Docker hosts | Docker-managed watched-bundle and exact-revision adapter tests plus a live stable-OPA allow-and-retry integration canary |
| CWD lifecycle actions use exact Workspace identity | Canonical-root, state, and label-validation tests |
| One canonical root/Context pair has one Workspace | Pair-derived root-index hash naming, locked exact-pair checks, domain duplicate-index validation, same-root/different-Context tests, and concurrent explicit-creation tests |
| Session exit cannot delete a Workspace | Child exit-status tests, host-stderr summary tests, and logical-state preservation after entry |
| Gateway cannot accept caller-selected Context or project authority | Owner-only atomic schema-1 registry, exact Workspace-source and Gateway-endpoint binding, duplicate/stale rejection, forged-header/SNI/authority and unknown-principal denial, source-bind/IP_FREEBIND canaries, and multi-Context integration |
| Static broker credentials cannot cross Context/project principals | Encrypted Context vaults, explicit project-bound handles, pre-OPA introspection, same-revision post-allow replacement, cross-Context tests, and integration |
| Unknown effects fail closed | Domain and catalog validation |
| Denials support safe policy learning | Typed Context/project/scheme/host/port/method/path denial validation, fixed navigation-response schema, host-only session summary, secret canaries, and integration projection |
| Learned permissions stay explicit and Context/project-bound | Context-scoped opaque-reference round trips, exact effect domain tests, Rego cross-Context/project canaries, preflight-before-aggregate activation tests, and Docker integration |
| One bad Context cannot replace known-good policy | Strict host-paired source validation, mutex plus cross-process locking, digest-bound source journal recovery, serialized content-addressed aggregate generation, reserved namespace validation, whole-candidate OPA tests, atomic publish, rollback tests, and integration |
| Context changes cannot mutate existing Workspace authority | Permanent instance binding, current-marker-only domain/application tests, and Context-local recording-runner restart reconciliation |
| Source access is exact and not a snapshot claim | Runtime spec/hash and Docker inspect tests, read-only mutation/Git-metadata failures, writable home/tmpfs canaries, no writable alias, and same-root host/read-write observation tests |
| Context policy ceilings cannot be bypassed | Default-plus-override method-decision tests plus destination/method terminal zero-candidate/DNS/Broker/upstream canaries and exact-Deny precedence above broad Allow, baseline, learned, and Advanced policy |
| Overlapping roots are not misrepresented as isolated | Product contract, Context-selected direct mounts, same-root/parent-child integration canaries, and absence of overlay/root-lock paths |
| Gateway does not retain allowed streaming bodies | Header-hook ordering unit tests plus incremental chunked-request and SSE-response integration canaries |
| Declared oversized bodies retain the transport bound | Fixed mitmproxy body-size asset test, over-limit `Content-Length` integration request, incremental unknown-length transport-cap evidence, and complete-body semantic-cap tests |
| GraphQL identity cannot collapse into one HTTP route grant | Trusted exact-endpoint projection, bounded parser fixtures, OPA all-roots matching, HTTP-rule non-matching canaries, GraphQL-aware opaque-reference round trips, and zero-upstream integration tests |
| AWS RPC identity cannot collapse into one `POST /` grant or semantic guess | Exact commercial authority and SigV4 structural checks, bounded Query/JSON fixtures, unsigned/ambiguous/streaming/query-mixed fail-closed canaries, OPA exact-coordinate matching, HTTP-rule non-matching, opaque-reference round trips, and absence of service-model/IAM/read-write classification |

## Supply chain and publication

Container bases distributed by Tobari and CI actions are pinned to immutable
versions or digests recorded in source. User-selected local images are neither
distributed nor trusted by Tobari; their selector is persisted as user
configuration. Third-party licenses are reviewed. Tests use synthetic
credentials, fake fixed-driver output, fixed clocks, and `example.com`
identities only. Live reviewed-provider acquisition, logout, and stale-handle
rejection may be replayed as experimental compatibility observations, but they
are not standard release checks. Tokens, codes, handles, and authenticated
transcripts are never repository fixtures. Publication still requires
`task security` and `task public:check`; neither replaces a human history and
confidentiality review. The canonical Gateway source is the public `gateway/`
tree; its embedded Docker build-input snapshot is checked for exact membership
and bytes against the current source, while
the locally built image is inspected against the exact embedded source identity
that built it. The experimental Auth Broker source is the public `authbroker/` tree; its
embedded Docker build-input snapshot is checked for exact membership and bytes,
and provider-CLI
absence and closed-plan protocol behavior are checked in validation. Image
workflows have no package-write permission. Gateway and the runtime base are
built locally from embedded pinned source and validated before use. Source
contains no owned image-output fallback, and release packaging has no registry
or component-lock authority. The checked combined Claude Code 2.1.220 and Codex 0.147.0 base
establishes integrity and local build identity only; Tobari never publishes
that combined image.
