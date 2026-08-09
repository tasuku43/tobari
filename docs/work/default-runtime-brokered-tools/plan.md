# Work Plan: Host-driven brokered CLI authentication

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Keep tool choice in a Context-selected local runtime. Keep credential records,
project handles, grant revisions, and encrypted refresh state in Auth Broker.
Add one resident host credential companion for provider-native execution and
automatic refresh. Auth Broker calls it only after the Gateway's exact request
has passed OPA and only through one authenticated encrypted reverse
`docker exec` channel.

The first driver invokes a trusted-host AWS CLI selected by trusted host state,
using a private temporary AWS home reconstructed from encrypted broker state.
`aws configure export-credentials` performs the provider-native refresh. The
companion returns a bounded request-bound role credential lease and an updated
opaque state snapshot; Auth Broker validates the echo, rechecks the credential
record/revision, atomically persists refreshed state, signs the unchanged
request, and returns only final headers to Gateway.

The existing GitHub acquisition path moves to the same reviewed host-driver
boundary. It verifies one host GitHub CLI identity, uses fixed login/status/
token argv in a private temporary configuration directory, captures one bounded
API token, and leaves no provider CLI in the Auth Broker image. It adds no
automatic GitHub refresh or Git credential-helper claim.

## Alternatives considered

### Add every tool to public or Auth Broker images

Rejected. Tool choice is user- and Context-specific, creates recurring release
and license work, and couples refresh support to image publication.

### Mount host CLI homes into Workspaces

Rejected. It exposes reusable secrets and executable configuration to
untrusted Workspace processes and bypasses project-bound revocation.

### Bind-mount a host Unix socket into Auth Broker

Rejected as a portable contract. Docker Desktop/Colima adds a VM kernel
boundary, so filesystem visibility does not establish portable host-socket
connect semantics.

### Let provider manifests execute arbitrary host commands

Rejected. Repository or data-selected execution would turn a non-secret
manifest into host code authority. Drivers are selected only from a
trusted-host registry with fixed protocol capabilities; the initial release
enables only reviewed built-ins.

## Design

### User experience

- Existing `auth login`, `auth status`, and `auth logout` command paths remain.
- GitHub login changes implementation location from Broker image to its fixed
  host driver without changing the public command or projected-handle UX.
- AWS login gains a host-driver prerequisite and keeps the browser/device flow.
- AWS request region remains explicit Context or command configuration; it is
  not stored in the credential record.
- Steady-state Workspace commands remain unchanged: tools see only a synthetic
  project handle and normal non-secret configuration.
- When a renewable session is still valid, refresh is automatic. When the
  upstream SSO session itself expires, the stable recovery is `auth login`.
- `task toolbox:build` builds the requested local Context image; no public base
  mutation or implicit tool download occurs.

### Companion lifecycle and transport

1. Compose starts and the Broker unlocks. `cluster up` verifies the exact owned
   Broker container ID, prepares a fresh purpose-derived session in Broker,
   and starts exactly one companion through the current Tobari executable's
   private inherited bootstrap stdin.
2. The companion holds fixed `docker exec -i` argv to that container. A tiny
   image-owned bridge performs only a bounded byte pump between the exec
   stream and `/run/tobari-auth/companion/bridge.sock`, an unmounted 0700
   tmpfs directory. It parses, logs, and persists nothing.
3. A fresh challenge handshake proves the prepared session. Direction-specific
   AES-GCM keys and exact sequence numbers protect bounded frames; inner
   messages are a closed typed set with request IDs, deadlines, and task
   digests. Invalid tags, gaps, replay, or shape silently close the session.
4. No host/container TCP listener, Compose host alias, cross-kernel UDS mount,
   shell, TTY, or secret argv/environment value is introduced.
5. Cluster status JSON schema 4 includes always-present secret-free
   `credential_companion_state=ready|prepared|absent|unavailable`. `cluster down`
   drains bounded in-flight work, then Compose teardown closes the exec stream
   and companion.

### Request and refresh flow

1. Workspace AWS CLI signs with synthetic handle values and sends HTTPS through
   Gateway.
2. Gateway removes the marker, introspects the handle, and asks OPA about the
   exact Context/project/host/port/method/path before reading a bounded AWS body.
3. After allow, Auth Broker validates handle, revision, binding, request digest,
   and encrypted driver-state record, then takes a per-record single-flight
   lock without holding its global mutex during host I/O.
4. Companion validates Context/project/provider/revision/binding/request-digest
   echoes, reconstructs a private AWS home, verifies the pinned host executable,
   invokes one fixed credential-export argv with a sanitized environment, and
   returns a temporary lease plus updated opaque state.
5. Broker rechecks record identity and revision, discards stale results, saves
   refreshed state atomically, signs the unchanged request, and returns final
   headers. Gateway validates those headers and forwards once.

### Driver extensibility

The companion protocol is typed around `refresh_lease`, cancellation, ping, and
drain outcomes, not arbitrary argv. Interactive GitHub/AWS login runs directly
through context-bound host drivers, and logout remains a Broker mutation; those
operations are not companion RPCs. A driver implementation is trusted host code
registered outside Context/provider manifests and bound to an immutable
identity. This slice publishes the protocol and AWS driver. Additional dynamic
providers require their own typed export, authority, state, and failure
contract; static exact-header providers continue to need no driver.

### Cancellation and failure

- Host login uses a direct context-bound child process; cancellation terminates
  it before any broker commit. A completed cache is validated and packed before
  the broker mutation begins.
- Companion refresh is finite. Timeout or disconnect after provider execution
  begins is not blindly replayed; Broker either commits one correlated known
  result or reports an outcome-unknown refresh that requires reconciliation.
- Rotation/logout can proceed while refresh runs. The post-call record/revision
  comparison rejects stale state and headers.
- Companion, driver, session, parsing, target, signing, and response uncertainty all
  fail closed. Errors contain stable codes only.

## Implementation slices

1. Replace ADR/work packet and remove public-base changes.
2. Add local toolbox cwk/pup artifacts and checks.
3. Add host companion lifecycle, encrypted reverse-exec bridge, health, and
   bounded protocol.
4. Move GitHub/AWS login and AWS renewable credential export to host drivers;
   remove provider CLIs from Auth Broker, refactor Broker state locking, and
   keep post-policy SigV4.
5. Complete Gateway negative cases, cancellation/rotation tests, source
   snapshots, docs, readiness replay, and repository gates.

## Verification

- Go unit/contract tests for handshake/session identity, process lifecycle, executable
  selection, state packing, cancellation, Context/project/revision echo, and
  bounded output.
- Python tests for deny-before-companion, per-record single-flight, stale refresh
  discard, invalid companion response, request binding, known SigV4 vectors,
  unsupported forms, and secret-free errors.
- Synthetic end-to-end AWS CLI request through Gateway/Broker/fake companion;
  no live account or credential fixture.
- Local toolbox source/integrity/license/version checks on amd64 and arm64.
- Required profiles: `task check`, `task security`, `task public:check`, and
  `task release:check`; compatible API-v2 image publication is the explicitly
  authorized final release action for this change.

## Rollout and rollback

Schema-v1 static providers remain readable. AWS broker-native state migrates
only after exact validation into host-driver state, or fails with a stable
re-login recovery without overwriting the old record. Gateway and Auth Broker
advance together under image API v2. Existing Workspaces require normal
leave/re-entry after login or binding changes.
