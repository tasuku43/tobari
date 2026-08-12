# Tobari Threat Model

## Scope

This model covers the current local product: one installation-local cluster
with a shared Gateway, OPA, and locked Auth Broker; multiple CWD-selected
Workspaces permanently bound to a canonical project root and one immutable
Context capability envelope; one selected direct read-only or read-write root
and persistent writable home per Workspace; and the supported
Docker topology for mediated HTTP and HTTPS. It covers the default tool-native
passthrough credential route, the Auth Broker's project-bound opaque-handle
route, and the bounded static `managed` Gateway adapter.

The model covers strict schema-1 static provider transformations and the
closed reviewed built-in plans. AWS uses fixed trusted-host CLI login selected only
from conventional non-project installation roots, control-safe bounded login
output, checked private-home cleanup, post-policy host CLI credential export
through a resident private companion, and Broker-owned bounded SigV4. Datadog
uses fixed trusted-host pup OAuth acquisition and Broker-owned post-policy
selection or exact US1 refresh. OpenAI uses exact Codex 0.146.0 device login,
encrypted ChatGPT OAuth state, exact post-policy account routing, and one fixed
refresh endpoint. Anthropic uses exact Claude Code 2.1.220 setup-token
acquisition and post-policy static resolution without refresh. It does not
extend the product boundary to
arbitrary provider programs, manifest-selected refresh/signing, general TWG
authentication, multiple accounts per Context, recursive DNS, or semantic
authorization/forwarding of non-HTTP protocols.

## Trust classification

Trusted:

- the host user and OS, Docker Engine or its Linux VM, container runtime, and
  kernel;
- Tobari CLI, embedded runtime assets, host lifecycle code, resident credential
  companion, reviewed host GitHub/AWS/pup/Codex/Claude drivers, owner-only XDG state, and the
  platform-defined host root-key backend;
- Gateway and its private CA state, OPA and the activated Rego projection, and
  the owner-only principal registry;
- Auth Broker, the installation root key, encrypted Context vaults, normalized
  provider projection, and owner-controlled provider manifests;
- host credential storage used by the static `managed` adapter; and
- explicit trusted-host choices of project root, Context, policy decision,
  credential acquisition, and credential removal.

Untrusted:

- every Workspace, its selected project root and persistent home, and every
  process that runs inside it;
- coding agents, shells, generated or downloaded code, custom runtime images,
  packages, and tool-owned authentication state;
- request and response data, upstream services, DNS answers, provider text,
  repository contents, and diagnostics derived from them; and
- caller-supplied session, project, Context, profile, and credential claims,
  including any copied opaque broker handle.

An opaque broker handle is intentionally non-secret, but it is still untrusted
input rather than authority by itself. Its use requires a trusted principal,
an exact live broker binding, and an OPA allow.

## Assets and boundaries

| Asset | Threat | Boundary | Enforcement |
|---|---|---|---|
| Host files outside a selected root | Workspace reads or modifies them | Docker mounts and root selection | The only host paths that may be writable are the Context-selected direct root bind and that Workspace's exact home; read-only source removes write authority from the former, and unsafe roots or management paths are rejected |
| Context envelope authority | Project content, image, Workspace, or changed source preset rewrites source/network authority | Owner-only immutable manifest and preset snapshot | Source access plus normalized preset origin/revision are fixed at Context creation, digest-bound, reported explicitly, and never re-read from a mutable source preset |
| Another Workspace's network authority | A process crosses isolation spaces or reuses its policy | Docker networks, namespace guards, and host principal | One dedicated non-overlapping internal network and one owned endpoint per Workspace; Context/project identity is derived from the kernel-observed source endpoint and complete owner-only Workspace/Gateway binding; duplicate/stale endpoints and source-spoof canaries fail closed |
| Overlapping project files | Separate Workspaces are mistaken for filesystem-isolated copies | Direct host bind mounts | Same-root and parent/child-root Workspaces may coexist, but overlapping paths expose the same host file effects; no overlay, root lock, or file-integrity isolation is claimed |
| Docker Engine | Workspace controls host containers | Unix socket and process boundary | Docker socket and host process interfaces are not mounted |
| Direct Internet access | Agent bypasses policy or ignores proxy variables | Docker topology and guarded routing | A Workspace joins only its dedicated internal network; its default route terminates at Gateway-local listeners, Gateway forwarding stays disabled with a forward drop, and Gateway is the only component on both that network and egress |
| Network-guard authority | A helper persists, mutates an unrelated namespace, or opens forwarding | Fixed host-side Docker invocation and exact owned rules | One-shot helper shares only one verified container network namespace, has no mounts/secrets/socket/host network, drops every capability except `NET_ADMIN`, installs one exact owned revision, verifies it, and exits before user entry |
| Pre-policy DNS | A Workspace leaks data in queries or causes destination lookup before allow | Non-recursive synthetic DNS and Gateway call ordering | Bounded A questions receive one synthetic non-public answer without forwarding or retained full-name logs; unsupported DNS is refused; real resolution begins only after one normalized HTTP authority is allowed |
| OPA policy and decision API | Workspace reads, changes, or bypasses policy | Control network and read-only projection | OPA joins only control; Workspaces do not; OPA receives a validated read-only aggregate generated from host-owned Context sources |
| Auth Broker control and runtime APIs | Workspace acquires or resolves a primary secret directly | Separate Unix sockets and mounts | Broker exposes no TCP listener and joins no Workspace network; host control uses a private control socket and only Gateway mounts the runtime socket |
| Host credential companion | Workspace or network input turns refresh into host execution, replays a session, or reaches a listener | Private same-binary process plus authenticated reverse exec | No listener or host socket mount; fixed verified Broker container/exec argv; root-key-derived epoch, direction keys, strict sequence/frame/deadline schemas, and only the compiled reviewed AWS refresh operation |
| Broker-owned Datadog/OpenAI refresh | Workspace input redirects OAuth refresh, uses ambient proxy state, controls OpenAI account routing, or triggers refresh before allow | Fixed schema-1 plans and post-policy Broker boundary | Exact Datadog US1 or OpenAI token endpoint, no redirect or ambient proxy, bounded strict exchange, same-revision per-record single-flight, account continuity for OpenAI, encrypted durable task barriers, and no refresh on OPA denial |
| Installation root key and encrypted Context vaults | Workspace obtains primary credentials or corrupts credential authority | Host root-key backend, authenticated encryption, and owner-only state | Broker starts locked; the 32-byte key enters through bounded stdin, stays in broker memory, and is never mounted into a Workspace; schema-1 vaults use AES-256-GCM with Context-bound associated data and checked atomic writes |
| Project-bound broker capability | A copied, stale, or malformed handle resolves a real credential | Gateway recognition, principal registry, and Broker binding | Handle must match Context, project, provider, credential revision, exact HTTPS target, source syntax, destination transformation, and redaction binding; invalid markers fail closed without fallback |
| Tool-owned authentication | Another Workspace reads or reuses it | Per-Workspace home and network | Tool state remains in that Workspace's exact home; all processes in the same Workspace may read it |
| Static managed secrets | Workspace reads or injects them | Gateway-only files and binding checks | Context-scoped owner-only files mount read-only only into Gateway; Context, project, host, and OPA-selected profile are checked before post-allow reading and injection |
| Authorization integrity | Request, DNS lookup, or credential reaches upstream before authorization | Gateway request-header hook, synthetic DNS, and lazy upstream connection | Gateway establishes the source-bound principal, normalizes one transparent authority, prepares credential metadata, asks OPA once, applies credentials only after allow, then resolves/pins, creates a separate upstream connection, and enables body streaming |
| Authority binding | An exact decision is replayed for a different effect | OPA decision and Gateway connection boundary | Learned rules bind Context, project, scheme, exact 1-65535 port, host, method, and raw path; query, headers, and body are not learned identity; arbitrary valid TCP ports do not collapse schemes or ports; Gateway classifies and pins resolved addresses |
| Preset guardrail bypass | Baseline, learned, Advanced, or provider policy exceeds the selected ceiling | Tobari-owned system evaluator | Immutable terminal guardrail runs before every allow path and finishes with zero candidate, external DNS, Broker resolution, or upstream call |
| GraphQL multiplexing | One coarse `POST /graphql` allow authorizes unrelated roots | Trusted endpoint declaration, bounded parser, and system OPA evaluator | Declared endpoints never fall back to HTTP rules; Gateway sends only query/mutation plus sorted canonical roots, every root needs an exact rule, and Advanced Context input cannot bypass the system GraphQL evaluator |
| HTTPS confidentiality in transit | TLS inspection is described as plaintext egress or silently bypassed | Transparent ingress and two TLS connections | Transparent clients retain authority in SNI/HTTP headers. Gateway terminates Workspace-side TLS with the Tobari CA, authorizes decrypted HTTP attributes, then creates a separate verified TLS connection to upstream |
| Body payload exfiltration | An allowed route carries different or sensitive bytes | Deliberate route-level policy boundary | Ordinary body content is not policy or candidate identity; one exact HTTP allow covers every body value at that Context/project/host/port/method/path. Declared GraphQL narrows only by operation type/root, not arguments or variables |
| Gateway buffering | One Workspace exhausts shared memory with a large body | mitmproxy transport boundary | Ordinary allowed bodies stream; a known `Content-Length` above 8 MiB retains the transport rejection, while unknown-length ordinary bodies have no total-byte quota. AWS signing buffers one already-authorized body within that cap; declared GraphQL requires a positive length no greater than 1 MiB before buffering |
| Audit confidentiality | Recognized credentials, handles, query, headers, or bodies leak into retained denial evidence | Gateway projection and logger | Audit omits query, headers, request and response bodies, broker primary secrets, handles, and revisions; recognized credential values are redacted before OPA and audit, and handle-bearing paths are replaced; an ordinary path remains potentially sensitive application text |
| Shared runtime capacity | One Workspace exhausts work-container or shared-service resources | Docker resource contract | Work containers and shared Gateway, OPA, and Auth Broker have fixed CPU, memory-plus-swap, PID, and rotated-log ceilings; there is no per-project fairness guarantee |
| Docker lifecycle state | Cleanup removes unrelated or credential state | Exact names, labels, and separate host state | Lifecycle code verifies installation owner, opaque ID, and role; cluster purge removes transient cluster resources and CA volumes, not encrypted vaults or the installation root key |

OPA is the policy decision point: it evaluates normalized ordinary HTTP or a
declared GraphQL root effect. Gateway is the enforcement point: it derives the principal, prepares
body-free ordinary input or bounded GraphQL-derived coordinates, refuses invalid or unavailable decisions, applies any
post-allow credential transformation, connects upstream, and streams the
request. Auth Broker neither interprets method/path policy nor grants network
permission; it validates and resolves an exact credential binding only when
Gateway follows the broker protocol. For the reviewed AWS plan it owns
encrypted opaque driver state, record/revision authority, and signing, and may
request one temporary host-CLI role lease only after OPA allow. For Datadog and
OpenAI it owns the fixed post-allow token-selection/refresh plans; Anthropic is
one same-revision static post-allow resolution.

The body-free OPA schema includes the host-issued Context/project principal,
scheme, host, port, method, raw path and path segments, multi-valued query,
redacted headers, and non-secret adapter metadata such as a successfully
introspected broker provider ID. It excludes request and response bodies,
broker handles and revisions, resolved primary secrets, and recognized client
credential values. At a declared GraphQL endpoint the schema additionally
carries only operation type and canonical root names; document text, operation
name, arguments, variables, directives, aliases, fragments, and nested fields
remain excluded. Those excluded values cannot become decision dimensions.

## Credible abuse cases

### Proxy bypass

A Workspace process opens an ordinary HTTP or HTTPS socket directly. Its
dedicated Docker network, synthetic DNS, guarded route, and output rules admit
that traffic only to the project's transparent Gateway listener. Tobari does
not project proxy variables or expose an explicit-proxy listener. If Gateway
is unavailable, there is no alternate egress path.

### Cross-Workspace access and overlapping roots

A process tries to reach another CWD-owned Workspace. The two containers have
different internal networks; only Gateway joins both, and no peer route is
given to either container. Network, principal, policy, home, and credential
scope therefore remain distinct.

That separation does not make project files private when roots overlap. A
canonical root may have separate Workspaces in different Contexts, and
parent/child roots may coexist. Because each is a direct read-write host bind,
changes under the overlap are visible to both. Tobari does not provide a
filesystem snapshot, overlay, integrity monitor, or mutual-exclusion lock.

### Direct policy or broker access

A Workspace process tries to resolve OPA or Auth Broker. OPA is absent from the
Workspace network. Auth Broker has no TCP listener, never joins a Workspace
network, and neither broker Unix socket is mounted into a Workspace. Gateway
exposes only the proxy port on each Workspace interface; its management and
credential-runtime surfaces remain on trusted paths. The companion opens no
host or container listener. Its Broker-private socket is unmounted, and the
fixed reverse exec stream belongs only to the trusted host process.

### Policy editing and activation

A process may intentionally modify its selected project root, including a
repository that happens to contain policy source. Entering or editing that
repository does not activate policy. The active principal registry, Context
policy sources, and aggregate projection are separate owner-only host state.

Trusted-host policy mutations use opaque references, validate the selected
Context source and the complete all-Context candidate, test it privately, and
publish a revisioned complete bundle to the owner-labeled Docker-managed policy
volume. The running OPA must report the exact expected revision before success.
An authority-reducing change first activates a deny-all transition bundle.
Unsafe, malformed, stale, ambiguous, or failed candidates do not become active.
A failed action retains or restores the prior source and known projection;
traffic still fails closed if no valid OPA decision is available. Interactive
review may stage several exact decisions for one Context only, preserving one
atomic source-file promotion before the all-Context aggregate activation.

### Forged authorization

A Workspace supplies authorization, cookies, API keys, a requested managed
profile, or another credential-like header. Gateway does not treat those bytes
as project authority. It removes proxy/Tobari control headers and excludes
recognized or configured secret header values from OPA input and audit.

In default `passthrough`, tool-owned client authentication is forwarded only
after allow. In static `managed`, Gateway strips managed credential positions
and may add only an OPA-selected profile whose trusted Context, project, and
host bindings match; it reads the Gateway-only secret only after allow.
Ordinary application values can still be sent on an allowed route, and Tobari
does not promise to classify every user-chosen application header as secret.

### Project identity and credential scope

A process writes another project or Context ID into a header, environment
variable, URL, SNI, session value, or profile name. Gateway ignores those
claims for caller authority. It observes the Workspace source endpoint on the
accepted connection and resolves that exact address through the host-owned
principal registry. The schema-1 registry binds one Context/project pair to
one dedicated owned network, exact Workspace endpoint, and exact Gateway
endpoint. The runtime publishes it only after both endpoints and the Workspace
guard are verified.

An unknown, duplicate, malformed, stale, or mismatched binding returns
`project_principal_unavailable` before OPA, broker resolution, or upstream I/O.
Learned denials, opaque candidate references, exact rules, broker handles, and
managed profiles retain the stable principal dimensions. An approval for one
Workspace therefore does not match another even when host, port, method, and
path are identical. The initialized baseline remains an explicit
installation-wide policy, not a per-project allowlist.

### Policy outage or ambiguity

OPA times out, stops, returns a non-success status, malformed JSON, missing
fields, or an unknown decision. Gateway returns local HTTP 503
`policy_unavailable`, does not connect upstream, and does not infer permission
from an earlier observation. There is one bounded OPA request and no automatic
application retry.

A valid nonmatching effect returns local HTTP 403 `policy_denied`. A learnable
denial may be retained as bounded host evidence that omits query, headers,
bodies, recognized credential values, broker handles, and primary secrets. The
ordinary path remains part of that evidence and can itself contain
application-sensitive text. The response only names the trusted review
location; it is not an authorization token and does not trigger a retry. A
baseline deny is terminal and never enters review. A CLI-owned exact deny is
also terminal but may be explicitly reset; reset removes that learned decision
and returns the effect to baseline/default deny rather than allowing it.

### Policy learning authority

A Workspace attempts to choose a displayed candidate, infer an ID from list
order, or turn several exact observations into a wildcard. Read-only discovery
returns opaque references. Only a trusted-host reference-bound action can allow,
deny, reset, or compact. Allow and deny bind the exact Context, project, host,
port, method, and raw path. Query, headers, body presence, and body content do
not split the candidate identity.

Compaction is an explicit separate action. It requires at least three safe
same-host/port/method exact sources, retains positive examples, checks an
outside-prefix canary, and never creates host or method wildcards. Observation,
display position, and elapsed time never grow permission automatically.

### Ordinary route authority and GraphQL root authority

For ordinary HTTP, Tobari deliberately does not claim that policy inspected,
understood, or bound the body. Gateway authorizes the principal, authority, method, and raw path from
request-header state before enabling upstream body streaming. An exact approval
therefore permits any body at that route, including a later value different
from the denied observation. Review and audit do not retain body content, so a
reviewer must assess the whole route-level grant rather than infer a payload
restriction.

At an exact trusted-host-declared GraphQL endpoint, the coarse HTTP identity is
ineligible. Gateway parses one bounded executable POST and conservatively
collects every reachable canonical root regardless of aliases, directives, or
variable values. Each root needs an exact query/mutation rule. Arguments,
variables, nested selections, and resolver meaning remain outside policy, so a
reviewer grants the whole root capability rather than one argument instance.

The reviewed AWS signing plan is the only buffering exception. After the exact
effect is allowed, Gateway retains one complete request within the same 8 MiB
cap, sends only its digest and normalized signing fields to Broker, and forwards
the body once after a valid signature. Unknown-length, streaming, aws-chunked,
EventStream, presigned, SigV4a, and ambiguous forms fail closed.

### HTTP, HTTPS, and certificate pinning

Clients keep their ordinary HTTP/HTTPS URLs, receive a synthetic non-public DNS
answer, and have their TCP connection redirected to Gateway's transparent listener.
Gateway requires one unambiguous HTTP authority, including consistent SNI for
TLS, and never recursively resolves that pre-policy DNS name. Gateway uses the
Tobari CA trusted by the Workspace to terminate TLS connection A, so it can
normalize and authorize the decrypted HTTP authority, method, path, query, and
redacted headers. The body remains outside policy identity.

Only after allow does Gateway replace any synthetic destination with that same
validated authority, resolve and pin a permitted address, and create the
upstream connection. HTTPS uses a separate, certificate-verified TLS
connection B from Gateway to the destination; Tobari is not sending plaintext
HTTP over the final Internet hop. Trusting the Tobari CA enables inspection but
does not grant an OPA allow. A certificate-pinned client rejects Gateway's
certificate and fails; Tobari does not silently bypass inspection or create a
direct route.

### Broker handle replay or downgrade

A process copies a `tobari-h1_...` value to another Workspace, keeps one after
replacement or logout, places it in an unsupported position, or changes its
declared header syntax. Gateway first establishes the trusted Context/project
principal, recognizes the handle only through the owner-controlled provider
projection, removes the placeholder header, and asks Broker for non-secret
introspection of the complete binding. OPA sees the provider ID, not the
handle, revision, or primary secret.

Context, project, provider, revision, exact HTTPS target, source header and
syntax, destination transformation, and redaction binding must all match. A
copied, malformed, stale, revoked, misplaced, ambiguous, or mismatched marker
returns `credential_handle_invalid` and never reaches upstream. Any
Tobari-looking marker fails closed; Gateway selects passthrough or managed
fallback only when no marker appears in any inspected URL/header position.

### Credential acquisition versus network authority

`auth login` and `auth import` acquire or replace one credential for one
Context/provider. Login is limited to reviewed fixed host
GitHub/AWS/pup/Codex/Claude drivers;
owner-controlled providers use protected non-terminal stdin import. Strict
schema-1 manifests are non-executable declarations of handle projection and
exact HTTPS/header transformation: they cannot name a shell command or add
method/path policy. Neither acquisition nor manifest installation calls policy,
adds a learned rule, or grants network permission. A matching Workspace
receives only a random project-bound handle on its next entry or reconciliation;
the primary secret stays encrypted in the Context vault.

Each host driver resolves one canonical executable and binds its digest, uses
fixed argv and a sanitized environment, and creates a private bounded temporary
home. GitHub captures one API token and configures no Git transport. AWS packs
opaque CLI cache state; request region remains separate Context/tool
configuration. Datadog captures one strict default-US1 pup DCR client, token,
and session state. Codex captures one strict non-FedRAMP ChatGPT OAuth session;
Claude captures one inference setup token without forwarding it to visible
output. Auth Broker contains no provider CLI or provider home. A
manifest, repository, Workspace, request, or provider response cannot select a
driver, executable path, argv, environment key, or shell.

For a brokered request, Gateway performs non-secret introspection before OPA.
On deny it makes zero secret-resolution calls and never connects upstream. On
static allow it resolves the same handle and revision once, replaces only the
declared destination header, then makes the single upstream attempt. On AWS
allow it hashes the bounded body and asks Broker for the same-revision
signature; Broker may call the companion once for a typed temporary lease,
recheck and persist the revision, and sign locally. Auth Broker cannot
override OPA, and possession of a valid handle alone is insufficient.
Non-secret provider metadata does not inherit a broader static host/method
allow: a brokered effect remains denied until an exact Context/project-bound
L7 rule covers it.

On Datadog allow, Broker resolves the same revision and selects an access token
only while it remains outside the five-minute pup refresh window. Otherwise it
persists a task barrier and makes one fixed, bounded, proxy-free,
redirect-rejecting refresh request to the US1 token endpoint. A strict response
replaces encrypted state atomically; an unknown outcome remains barred until
explicit Datadog re-login or logout.

On OpenAI allow, Broker selects the same-revision access token and fixed
account-routing value, refreshing only inside the reviewed time window with
one bounded proxy-free, redirect-rejecting request to the exact token endpoint.
It preserves account continuity and atomically commits state before returning.
An unknown result remains barred until explicit OpenAI re-login or logout. On
Anthropic allow, Broker resolves the same revision once for the exact API
authority; no refresh or supplemental header exists.

### Companion replay, crash, or stale refresh

An attacker replays a frame, skips a sequence, replaces the resident process,
disconnects after provider execution, or rotates/logs out a credential while
refresh is in flight. `cluster up` prepares one fresh root-key-derived epoch;
the handshake derives separate directions, and invalid tags, replay, gaps,
duplicates, oversized frames, unknown messages, or a second session close the
channel. Every complete encrypted-frame write has a two-second deadline; a
partial, timed-out, or failed write closes the whole session before any later
sequence number can be used. Close and invalidation interrupt blocked writes.
The bridge parses, logs, and persists nothing.

AWS, Datadog, and OpenAI refresh are single-flight per credential record with a
one-second queue wait. Queue expiry is known pre-execution and creates no
barrier or provider call. Broker atomically stores the task digest in the
encrypted record before AWS host execution or Datadog/OpenAI HTTPS refresh and clears
it only with the same correlated successful state commit.
An unknown outcome therefore remains barred across Broker restart. Broker does
not hold its installation-wide state mutex over host/provider I/O and rechecks record,
revision, driver identity, and request/task digest before committing returned
opaque state or using the temporary lease. Replacement or logout therefore
wins. A companion disconnect or Datadog/OpenAI transport ambiguity after dispatch is
outcome-unknown, maps to non-retryable
HTTP 409, and is not blindly replayed. After the request settles, `auth status`
distinguishes `broker_state=ready` with the affected provider `configured`, for which an
explicit user retry is safe because Gateway made no upstream attempt, from
barred `not_configured` state that requires provider re-login or logout. The
stable recovery is host `cluster up` for known pre-execution companion
unavailability or a locked/unavailable status.

### Broker lock, key, and vault failure

Auth Broker starts locked after creation or restart. `cluster up` obtains the
installation root key from the fixed host backend and sends it through bounded
stdin; it is not placed in argv, environment, a Workspace mount, or the image.
On macOS the backend is Keychain service `io.tobari.auth-root.v1`, account
`tobari`; on Linux it is owner-only XDG state `auth/keys/root.key`.

A missing key beside an existing encrypted vault is a recovery fault, not a
reason to create a replacement key. An unsafe key, invalid vault, locked Broker,
known pre-dispatch socket failure, or unavailable Broker does not expose or
forward a credential. Those pre-execution Broker-path failures return local
503; an invalid handle binding returns local 403. A lost or invalid response
after an AWS companion operation or Datadog/OpenAI refresh was dispatched returns
non-retryable local 409 with a durable reconciliation barrier, never 503 replay
permission.

### Credential replacement and logout

Credential replacement and `auth logout` atomically revoke prior Broker
handles for that Context/provider. They cannot rewrite an already-running
process environment or remove a projected file that has been changed outside
Tobari's ownership check, so the public result requires Workspace re-entry.
An old handle string may remain visible to that process, but Broker rejects it.

Logout removes local Tobari credential state; it does not contact the provider
or guarantee remote revocation. It does not remove tool-native credentials in
a Workspace home or static managed-adapter files. `cluster down` and
`cluster down --purge` preserve both encrypted Context vaults and the
installation root key; purge additionally removes shared CA and active
policy-bundle volumes and is not an authentication reset.

### Shared service exhaustion

A Workspace can generate many policy decisions, TLS connections, or broker
introspection attempts through the shared services. Gateway, OPA, and Auth
Broker have fixed CPU, memory-plus-swap, PID, and Docker-log bounds, so each
service reaches its cap rather than receiving unbounded shared resources. The
ceilings do not establish fairness: a noisy Workspace can still degrade other
Workspaces within the common allocation. Per-project rate and bandwidth
controls require separate design.

### Authority mismatch

An attacker reuses an approved host, path, and method on another port, scheme,
or DNS result. Gateway supplies normalized scheme and port to OPA. The current
policy accepts only explicit, non-overlapping scheme-port pairs, while learned
rules retain Context, project, host, port, method, and raw path. A rule does not
store an independent scheme field; under the current fixed mapping, changing
scheme necessarily changes the allowed port contract and cannot reuse the
rule.

Immediately before connection, Gateway rejects non-global results for dotted
hostnames and pins the selected resolved address. A nonmatching port is denied
without becoming a permission candidate. Baseline and exact denies remain
terminal, and an exact deny wins over a learned allow for the same effect.

### Host path escape

The user supplies a root or CWD containing symlinks or `..`. The CLI resolves
the canonical host path before state or Docker operations and requires CWD
containment. It rejects filesystem root, the host home and its ancestors, XDG
management areas, Docker sockets, and Docker management paths. This does not
protect any file below an intentionally selected or overlapping root.

### Cleanup confusion

An attacker creates similarly named Docker resources. Tobari uses exact stored
names and verifies installation owner labels. Per-Workspace cleanup also
verifies the exact opaque ID and resource role; prefixes, names, and display
order are not authority. Shared cluster removal is rejected while Workspace
records remain. Purge does not remove vaults or the root key.

### Shared resource exhaustion

A process can intentionally fork, allocate memory, burn CPU, or write logs.
The work container has fixed CPU, total memory-plus-swap, PID-count, and
container-log bounds, and an old or drifted runtime spec is recreated before
reuse. The selected read-write root remains deliberately unquotaed, and network
bandwidth is not shaped; exhausting host disk through that root or consuming
authorized network capacity remains outside this control.

## Explicitly accepted risks

- A read-write Workspace can modify or delete its full mounted project root. A
  read-only Workspace cannot write through that bind but sees host or other
  read-write Context changes because the bind is live rather than a snapshot.
- Same-root and parent/child-root Workspaces share file effects in the overlap,
  even when their Contexts, homes, networks, principals, policies, and
  credential scopes differ.
- A Workspace can consume free disk below its selected root and authorized
  network throughput; current resource controls do not provide disk quotas,
  per-project fairness, or bandwidth shaping.
- A permitted destination can receive any data that Workspace can read. Tobari
  does not inspect body meaning or prevent deliberate exfiltration over an
  allowed effect.
- Tool-native credentials are intentionally readable by every process in the
  same Workspace. Managed and brokered primary secrets remain outside the
  Workspace, but Gateway and the trusted host can exercise them.
- Every process in one Workspace can read and attempt to use that Workspace's
  broker handle. Exact binding and OPA prevent cross-principal reuse, but do not
  stop a malicious same-Workspace process from exercising an already allowed
  brokered capability.
- The initialized policy is an installation-wide baseline rather than a
  per-project static allowlist. Mutually untrusted tenants require a stronger
  execution and resource boundary than this local shared cluster.
- Compromise of the host user, OS, kernel, Docker Engine or VM, Tobari CLI,
  host credential companion/driver, Gateway, OPA, Auth Broker, root-key
  backend, or activated trusted policy is outside this boundary. Encryption
  alone does not protect credentials from a compromised trusted foundation.
- On Linux, owner-only XDG key storage separates the key from vault files and
  Workspaces but does not protect against compromise of the host user or whole
  XDG state tree.
- Certificate-pinned applications may fail because Tobari terminates
  Workspace-side TLS and does not bypass pinning.
- `auth logout` does not remotely revoke a provider credential or rewrite an
  already-running process. Workspace re-entry is required to reconcile its
  projection.
- `cluster down --purge` is not credential deletion: encrypted Context vaults
  and the installation root key remain until explicit credential/state
  removal.
- HTTP/3/QUIC, SSH, raw TCP, UDP, DNS policy, covert channels, malware
  detection, provider-specific operation semantics, and same-Workspace process
  isolation are not provided.
- Unix owner and mode checks do not establish equivalent Windows ACL
  guarantees.

## Reconsideration triggers

Revisit the architecture before forwarding or semantically authorizing
non-HTTP protocols, adding recursive DNS, multiple clusters or mutually
untrusted tenants, a per-project
static baseline, remote execution, filesystem overlays or root locks,
process-level identity, stronger root-key isolation, additional executable
host drivers, a host listener or socket mount, new credential shapes, any
refresh/signing flow beyond the reviewed AWS, Datadog, OpenAI, and Anthropic plans, general TWG
authentication, multiple provider accounts, arbitrary provider operations, or
provider-specific policy semantics. A demonstrated route from one Workspace to
another, OPA, Auth Broker, the companion, or an external destination without
the Gateway's supported enforcement path is a release-blocking security defect.
