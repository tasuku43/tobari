# ADR 0020: Add a host credential companion for renewable brokered credentials

- Status: Accepted
- Date: 2026-08-09
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, external I/O, harness, public boundary, and release
- Supersedes: [ADR 0019: Add a shared locked Auth Broker for Context credentials](0019-shared-locked-auth-broker.md)
- Superseded by: None

## Context

ADR 0019 supports one Context-owned static secret transformed into one exact
HTTPS header after OPA authorization. The requested kubectl, cwk, TWG, pup, and
AWS CLI workflows expose three different families: static header credentials,
refreshable OAuth/SSO sessions, and request signing.

The first implementation put provider acquisition, AWS refresh, role exchange,
and signing code in the Auth Broker image and promoted common CLIs into the
published work base. The user clarified two requirements before that design was
released:

1. Tool choice belongs to a Context-specific work environment, not an
   ever-growing public base or Auth Broker image.
2. Provider-native command execution should be delegated to the trusted host,
   and refresh should remain automatic.

Copying host CLI homes, AWS SSO caches, or temporary role credentials into a
Workspace would violate the project-bound post-policy boundary. Returning real
AWS credentials through `credential_process` has the same problem. Conversely,
letting a provider manifest select an arbitrary host command would turn
non-secret data into host code authority.

A host process and an Auth Broker container also cannot use a bind-mounted Unix
socket as a portable contract: Docker Desktop and Colima add a VM kernel
boundary. Opening a host TCP listener would introduce firewall, LAN, rootless
engine, certificate, and host-gateway cases that the task does not otherwise
need.

## Decision drivers

- Keep provider primary secrets, refresh material, and temporary AWS
  credentials outside Workspaces, OPA, logs, audit, and public CLI output.
- Preserve one generic Gateway/OPA authorization decision before any
  credential resolution, refresh, signing, or upstream attempt.
- Keep work-tool selection in a Context image and provider-native execution on
  the trusted host without requiring those tools in Auth Broker.
- Automate renewable credentials while making cancellation, rotation, logout,
  stale results, and uncertain provider outcomes explicit.
- Keep provider manifests strict non-secret data. Repository or Workspace
  input must never select host code, executable path, argv, or environment.
- Retain one release gate for every new image and wire compatibility boundary.

## Considered options

### Add every supported CLI to published images

Rejected. Users need different tools, licenses and upstream versions vary, and
image publication is the wrong extension boundary for host authentication.

### Mount or copy host CLI state into Workspaces

Rejected. Every Workspace process would receive reusable credentials and
executable configuration outside the handle/revision boundary.

### Execute arbitrary commands named by provider manifests

Rejected. A manifest, Context, repository, or request must not become a host
command loader. Dynamic behavior requires a typed trusted-host driver.

### Connect Auth Broker to a host listener

Rejected for the first slice. Mutual TLS can protect content, but a listener
still expands platform networking, exposure, denial-of-service, and lifecycle
contracts.

### Keep a reverse Docker exec channel

Selected. The host companion owns one fixed long-lived `docker exec -i` to the
exact verified Auth Broker container. A tiny image-owned bridge relays bytes to
one unmounted Broker-private Unix socket. No port, host alias, host socket mount,
shell, TTY, or secret command argument is required.

## Decision

### Split credential authority from provider execution

Auth Broker remains the authority for encrypted Context credential records,
grant revisions, project handles, exact provider bindings, rotation, logout,
and post-policy resolution. A new resident trusted-host credential companion
owns provider-native execution. Gateway and OPA do not connect to the
companion.

Static exact-header providers continue through Auth Broker without a host
driver. A renewable credential record contains typed opaque driver state in the
authenticated Context vault. The state is decrypted only after an allow and is
sent to the companion for one typed operation. A normal refresh preserves the
grant revision; login, import, binding changes, and logout rotate or revoke all
project handles.

Provider manifests cannot name executables or arbitrary behavior. Driver IDs
and capabilities are a closed typed host registry owned by reviewed
infrastructure. The initial dynamic driver is `aws_cli_sso`; extending the
registry requires an explicit driver protocol, authority, state, failure, and
test contract. This moves binaries out of images without accepting
request-selected code execution.

### Use an encrypted reverse exec session

After Compose starts and Auth Broker unlocks, `cluster up` verifies the exact
owned Broker container ID and prepares a fresh companion session. A
purpose-separated session key is derived from the installation root key and a
fresh random session identifier. The root key never reaches the companion; only
the derived session material crosses its private inherited bootstrap stdin.

The companion runs fixed argv equivalent to:

```text
docker exec -i --user <installation-uid:gid> <verified-container-id> \
  python3 -m authbroker.companion_bridge
```

The bridge opens only `/run/tobari-auth/companion/bridge.sock` inside an
unmounted 0700 tmpfs directory and performs a bounded bidirectional byte pump.
It does not parse provider data, log frames, execute another command, or write
state.

A fresh challenge handshake binds the prepared installation, Broker boot,
companion instance, and session. Direction-specific AES-256-GCM keys protect
length-bounded frames. Each direction accepts only the exact next sequence
number; replay, gaps, invalid tags, oversized frames, malformed inner payloads,
or an unexpected message type close the session without diagnostic content.
Typed inner messages carry random request IDs, deadlines, state generations,
and a digest over Context, project, provider, credential record, grant revision,
driver identity, binding, triggering request, and encrypted-state hash.

The message set is closed to readiness, refresh lease, cancellation, ping,
drain, and their acknowledgements/results. It is not a generic RPC, shell, or
network tunnel. The Broker admits a bounded number of pending calls. The host
admits a bounded number of driver processes. Disconnect fails every unresolved
call closed.

`cluster status` and doctor report companion readiness separately from
container liveness. Docker health does not require the companion before the
unlock/attach sequence, avoiding a startup deadlock. `cluster down` stops new
requests, drains bounded work, tears down Compose, and lets exec EOF terminate
the companion. A crash leaves dynamic operations unavailable until `cluster
up`; static Broker operations remain independently diagnosable.

### Run AWS IAM Identity Center through host AWS CLI

Trusted-host `auth login aws` validates the access-portal start URL, SSO region,
twelve-digit account ID, and role name. Request region remains ordinary
Context/command configuration rather than credential state. Login
resolves one host AWS CLI executable from trusted host state, records its
canonical identity and SHA-256 digest, creates a private temporary AWS home,
writes one fixed token-provider profile, and invokes one fixed device-code
login argv. Browser opening remains purpose-limited to the validated AWS device
URL with a manual fallback.

Cancellation terminates the host child directly. No Broker mutation begins
until the child succeeds and its bounded SSO cache is validated and packed.
Failure therefore preserves the previous Context credential. The cache bytes
are immediately imported as typed opaque driver state into the encrypted
Context vault and removed with the temporary home. Host global AWS
configuration is neither read nor written.

For an allowed request, Auth Broker takes a per-record single-flight lock,
waits at most one second for the same record, and returns known pre-execution
unavailability without a barrier or companion call when that bound expires.
It then
snapshots record/revision/state generation, and releases its installation-wide
mutex before calling the companion. The companion validates the task digest,
reconstructs a private temporary AWS home, revalidates the pinned executable,
and invokes fixed `aws configure export-credentials --format process` argv with
a sanitized environment and finite timeout/output limits. AWS CLI performs any
due IAM Identity Center refresh. The companion returns a request-bound
short-lived credential lease plus updated opaque cache state.

Broker reacquires its state lock, reloads the record, and commits updated state
only when record ID, grant revision, state generation, binding, and request
digest still match. Rotation or logout wins; a late result is discarded.
Temporary role credentials are never persisted. Broker uses them only to sign
the unchanged bounded request and clears them afterward. Gateway receives only
the final request-local SigV4 headers.

If provider execution began and the result becomes unobservable, Tobari does
not blindly replay it. The operation reports an outcome-unknown refresh and
requires status reconciliation. Before sending the refresh, Broker
atomically stores its task digest in the encrypted AWS record. Only the same
correlated successful CAS clears it while committing refreshed state; a crash
therefore leaves a durable barrier that handle issue and signing cannot cross.
Explicit AWS re-login or logout replaces or removes the barred record. Gateway
maps this condition and post-send Broker transport loss to HTTP 409
`credential_refresh_outcome_unknown`, not an AWS-generically-retryable 5xx.
A lost Broker response can occur after the correlated commit has already
cleared the barrier, so recovery waits for the request to settle and branches
on `auth status`: `broker_state=ready` with AWS `configured` permits an
explicit retry because Gateway made no upstream attempt; AWS `not_configured`
requires re-login or logout and Workspace re-entry. Automatic SDK replay
remains forbidden.
A known correlated refresh result may
be committed even if the original HTTP caller canceled, but that canceled HTTP
request is never forwarded.

`cancel_ack` is receipt-only. A correlated `refresh_result` is the sole terminal
completion. After an accepted cancellation, absence of that result for five
seconds becomes outcome-unknown.

Every complete encrypted-frame write has a two-second deadline. A partial,
timed-out, or failed write invalidates the entire session before a later
sequence number can be used; close and invalidation interrupt a blocked write.

### Retain post-policy AWS request constraints

A Workspace receives the project handle as synthetic AWS access key, secret,
and session token plus the metadata-disable setting. It
receives no AWS SSO cache or real role credential. Gateway recognizes only a
standard `AWS4-HMAC-SHA256` header whose access key and security token are the
same valid handle. It removes them before OPA.

After allow, Gateway accepts the complete request only within the existing 8
MiB cap, computes the actual payload hash, and asks Broker to sign exactly that
request. Supported targets are standard commercial-partition HTTPS port 443
`amazonaws.com` authorities with a reviewed commercial region and a scope
consistent with the authority. China, GovCloud, ISO, and sovereign partitions,
new IAM Identity Center portal URL forms, query presigning,
SigV4a, custom/private endpoints, redirects that change authority,
aws-chunked/trailers, event streams, S3 Express session auth, WebSockets, and
unrepresentable canonical forms fail closed. Ordinary non-AWS traffic retains
streaming behavior.

OPA still authorizes Context/project/host/port/method/path, not an inferred AWS
operation. Least-privilege IAM permissions remain required.

### Keep requested tools Context-local

The published base runtime remains unchanged. `images/toolbox` is the explicit
local derivative for a Context that wants GitHub CLI, AWS CLI, kubectl, TWG,
cwk, pup, and diagnostic tools. Versions, per-architecture integrity, and
license inventory are pinned and checked, but building it is a trusted-host
local action and does not make TWG a public redistribution claim.

Schema-v1 built-ins/examples cover exact routine paths:

- Chatwork `CWK_API_TOKEN` to `x-chatworktoken` at `api.chatwork.com`.
- Datadog `DD_ACCESS_TOKEN` to bearer auth at fixed US1
  `api.datadoghq.com`; commands needing API plus application keys are excluded.
- One public Kubernetes API authority with complete CA data and a static bearer
  handle; exec plugins, client certificates, private endpoints, multiple
  clusters, and port forwarding are excluded.
- Delegated TWG bearer use at `api.atlassian.com:443` only. General TWG login,
  refresh, Rovo connectors, Bitbucket, media, telemetry, and other authorities
  are not claimed. Although `twg auth refresh` exists, its current secret export
  is shell-sourceable rather than a bounded typed credential response.

## Consequences

### Positive

- Provider CLIs and refresh behavior no longer grow either published image.
- Workspaces keep their existing handle-only experience while renewable AWS
  sessions refresh automatically on the trusted host.
- No host listener, host socket mount, or host CLI home crosses the boundary.
- Refresh concurrency is per credential rather than installation-wide, and
  rotation/logout mechanically defeat stale results.

### Negative

- The released Tobari host process must remain resident while dynamic
  credentials are usable.
- Host AWS CLI is a prerequisite for AWS login/refresh even when a Workspace
  image also contains AWS CLI.
- AWS commands must obtain their request region from Context configuration or
  an explicit normal AWS CLI option; login does not silently create a second
  region configuration source.
- Reverse-channel framing, lifecycle, driver state, cancellation, and recovery
  add contracts across Go and Python.
- A provider refresh whose result is lost after execution may require re-login;
  it cannot be safely replayed from inference.
- General TWG OAuth remains unsupported until its typed export and authority
  contracts are sufficient.

## Mechanical enforcement

- Domain tests cover schema-v1 compatibility, typed dynamic plans, built-in
  driver selection, collision rejection, handle projections, and deterministic
  binding/task digests.
- Companion protocol tests cover challenge/proof, direction key separation,
  exact sequence, replay/gap/tag/length rejection, bounded concurrency,
  cancellation, drain, disconnect, and secret-free errors.
- Host-driver tests cover canonical executable identity, digest mismatch,
  sanitized environment, exact argv, private temporary state, cache traversal,
  symlink/non-regular/oversize rejection, Darwin/Linux real-PTY canonical
  profile-prompt cancellation, readiness-flush recovery through an
  identity-checked private nonblocking description, inherited-flag isolation,
  and noncanonical VMIN/VTIME rejection before provider/Broker execution,
  output bounds, strict
  process-credential JSON, and refreshed-state packing.
- Broker tests cover per-record single-flight, no global lock across I/O,
  generation CAS, rotation/logout races, known commit after caller cancel,
  outcome-unknown recovery, and zero companion calls before allow.
- Gateway tests cover placeholder removal, deny-before-use, body bounds, exact
  known SigV4 vectors, authority/scope consistency, malformed companion/Broker
  responses, and every excluded signing form.
- Runtime/integration tests cover exact container identity, fixed no-TTY exec
  argv, hidden bootstrap stdin, startup ordering, process crash, reconciliation,
  status, drain/down, and absence of host ports/mounts.
- Toolbox checks cover pinned versions, architecture checksums, local-only
  notices/licenses, inherited runtime contract, and CLI version smoke.

## Compatibility and migration

Valid schema-v1 provider manifests and static vault records remain readable.
The dynamic vault payload advances to schema 2. Any valid uncommitted
broker-native AWS state can be converted only after exact validation; otherwise
the record remains untouched and `auth login aws` is the recovery. Older
binaries fail closed on schema 2 and never overwrite it.

Gateway and Auth Broker image API labels advance together from 1 to 2. New host
binaries reject API-v1 image digests. The transition was completed by
publishing and anonymously inspecting the Linux amd64/arm64 API-v2 indexes from
source revision `a3fedb66ad5a72c19d6721f3f8da49852882ced8`, then recording the
reviewed immutable Gateway
`sha256:9b4dbfaf587f22a1a036dec85df8637cc323d4377142b0463781b25e3ef15049`
and Auth Broker
`sha256:a2df8169fd1b28ab67d42c83c5181714ce5373ab74fe9931e84ab4542dc97fb1`
digests in `versions.env`. Existing Workspaces require normal leave/re-entry
after login or binding changes.

## Security and public-boundary impact

New secret material is the purpose-derived companion session key, encrypted AWS
CLI cache state, and request-local AWS role credentials. The session key exists
only in Broker/companion memory; root key, refresh state, and role credentials
do not enter argv, environment, logs, audit, fixtures, Gateway policy input, or
Workspace files. Tests use synthetic tokens, accounts, roles, paths, clocks,
and local fake processes. Live login is a manual release check, never a fixture.

The toolbox is local-only. The public base, its embedded snapshot, and its
publication inventory do not gain kubectl, cwk, TWG, or pup.

## Validation

- `task check`
- `task security`
- `task public:check`
- `task gateway:test`
- `task authbroker:test`
- `task integration:test`
- `task toolbox:build`
- `task release:check`
- Manual trusted-host AWS device login followed by a re-entered Workspace AWS
  read, deny-before-refresh observation, forced automatic refresh, companion
  restart/reconciliation, logout, and stale-handle rejection. Retain only
  secret-free pass/fail observations.

## Reconsideration signals

- A driver needs arbitrary command or network tunneling rather than one typed
  credential outcome.
- Companion availability or outcome-unknown refresh recovery causes routine
  user friction.
- A required AWS path needs SigV4a, streaming/event payloads, presigning,
  private endpoints, or operation-aware policy.
- TWG publishes a bounded typed credential export and an exhaustive authority
  contract for the desired command family.
- Multiple accounts, roles, clusters, sites, or credentials per Context become
  a routine outcome.
