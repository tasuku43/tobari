# Work Context: Host-driven brokered CLI authentication

This file records verified facts and unresolved questions. Desired design is
recorded in `plan.md`, not as current behavior here.

## Current behavior

- The published base already contains GitHub CLI and AWS CLI. The requested
  kubectl, cwk, TWG, and pup additions were removed after the user clarified
  that the shared base must not become a universal tool catalog.
- `images/toolbox` is an explicit local derivative containing the base's GitHub
  CLI and AWS CLI plus pinned kubectl, cwk, pup, and local-only TWG. It is
  suitable for a selected Context and is not a public runtime release unit.
- Auth Broker schema v1 stores one static primary secret and resolves it only
  after Gateway receives an OPA allow for the same Context, project, target,
  and request.
- The host-driver slice now places IAM Identity Center acquisition and
  credential export on the trusted host; Auth Broker retains encrypted state,
  exact request binding, CAS, and local SigV4 signing without provider CLIs.
- Cluster services are containers. A macOS-created Unix socket cannot be
  assumed to retain connect semantics through the Docker VM filesystem. A
  host listener would also add firewall, rootless-engine, and LAN exposure
  cases. The selected bridge is therefore a persistent reverse `docker exec`
  byte stream into an unmounted Broker-private Unix socket.

## Relevant structure

- Entry point and composition: `cmd/tobari` and `internal/cli`.
- Domain provider and projection contracts: `internal/domain/authbroker`.
- Auth use case: `internal/app/authcmd`.
- Host/Docker lifecycle: `internal/infra/dockerruntime`.
- Canonical Auth Broker and Gateway sources: `authbroker/` and `gateway/`;
  embedded copies under `internal/infra/runtimeassets/assets/` are snapshots.
- Optional Context tool image: `images/toolbox`.

## Constraints

- Workspace, OPA, logs, audit, and CLI output never receive provider primary
  secrets, refresh material, or AWS role credentials.
- OPA authorization for the exact ordinary HTTP effect precedes credential
  resolution, refresh, signing, or an upstream attempt.
- Host executable selection is trusted-host state. A provider manifest,
  repository file, Workspace input, or network request cannot supply a path,
  argv, environment key, or driver implementation.
- Companion calls have a root-key-derived, direction-separated authenticated
  session, exact schemas, monotonic sequence numbers, size/time/call bounds,
  request and credential-generation binding, and secret-free errors.
- Refresh network I/O cannot hold the Broker's installation-wide state mutex.
  Rotation/logout wins through a post-call record/revision comparison. The
  per-record queue wait is finite, and Broker persists an encrypted task-digest
  barrier before host execution so outcome-unknown cannot replay after restart.
- Downloads in the local toolbox remain pinned and checked. Local-only TWG
  inclusion does not make a public redistribution claim.
- Auth Broker contains no provider CLI or persistent provider home. GitHub and
  AWS login execute only through reviewed fixed host drivers.

## Verified external facts

- Docker documents `host.docker.internal` for container-to-host services, but
  using it would require a host listener plus Linux/rootless/Desktop network
  handling. A fixed long-lived `docker exec -i` can instead carry an encrypted
  reverse channel without ports, host aliases, or host socket mounts.
- AWS CLI `configure export-credentials --format process` resolves temporary
  credentials through the CLI's normal provider chain.
- AWS CLI IAM Identity Center token-provider profiles automatically refresh an
  access token with the refresh token until the SSO session expires. Expired
  overall sessions still require an explicit login.
- The first driver is deliberately classic-portal commercial partition only.
  New portal URL forms and China/GovCloud/ISO/sovereign partition binding need
  a typed partition model rather than suffix inference.
- `twg auth refresh` refreshes stored OAuth credentials without prompting, but
  TWG currently exposes the active secret through a shell-sourceable export,
  not a bounded typed credential response. General TWG also spans authorities
  beyond the current exact `api.atlassian.com` example.
- cwk uses a static `x-chatworktoken`; pup supports a static bearer access
  token for the bounded Datadog path; a complete CA kubeconfig can use one
  static bearer handle for one public Kubernetes API authority.

## Thesis evidence

- User correction: tool binaries belong to a Context-selected runtime, not the
  public work base or an ever-growing Auth Broker image.
- Repeated design pressure: GitHub, AWS, and TWG each own login state and
  refresh behavior that cannot be represented by a static header manifest.
- Unsafe workaround rejected: mounting host CLI homes or returning real AWS
  credentials through `credential_process` would give reusable credentials to
  every process in a Workspace.
- Proposed thesis revision: separate encrypted handle/revision authority in
  Auth Broker from provider-native execution in one resident trusted-host
  companion, while retaining post-policy use and strict driver selection.

## Security and compatibility notes

- The companion is trusted host infrastructure. Compromise has the authority
  of registered credential drivers, but not arbitrary Workspace-selected code.
- A companion session key is purpose-derived from the installation root key
  and a fresh session identifier. Only the derived key reaches the companion
  over its inherited bootstrap stdin; the root key never leaves its existing
  host/Broker boundary. Gateway and OPA receive neither value nor driver state.
- AWS CLI cache bytes are treated as opaque secret state, encrypted by the
  existing Context vault between calls, and materialized only in a private
  bounded temporary driver home while the host CLI executes.
- Known pre-execution companion unavailability is retryable. Once host
  execution might have begun, a missing/invalid result or post-send transport
  loss is HTTP 409 and is never automatically replayed. After settlement,
  `auth status` with `broker_state=ready` and AWS `configured` identifies a
  committed/no-upstream-attempt path that an operator may explicitly retry;
  AWS `not_configured` identifies a record barred until re-login/logout. A
  stale result after login rotation or logout is discarded.
- Gateway/Auth Broker image API v2 remains unreleased until compatible images
  are published and immutable digests are updated; old API-v1 images must not
  be accepted by the new binary.

## Verification evidence

- Auth Broker tests pass 71/71 and Gateway tests pass 55/55. Targeted Go unit,
  race, vet, Darwin PTY, and Linux arm64 PTY cancellation suites also pass; the
  Linux suite completed 20 repetitions in a network-disabled disposable
  container.
- The local toolbox image builds and version-smokes Git 2.39.5, GitHub CLI
  2.96.0, AWS CLI 2.36.11, kubectl 1.36.3, TWG 1.1.1, cwk 0.2.4, and pup
  1.10.5.
- `task check` passed twice after the final hardening. `task security`,
  `task public:check`, and `task release:check` also pass; govulncheck reports
  no vulnerabilities.
- With explicit authorization, the prior cluster containers and CA volumes
  were removed while logical Workspace state, home, vault, and root key were
  retained. The real Docker integration profile then passed API v2 startup,
  deny-before-host-execution, one post-allow AWS host refresh, SigV4 signing,
  one upstream attempt, lifecycle recovery, and owned-resource cleanup.
- The first real run exposed two cross-runtime transport facts. Python buffered
  `read(8192)` waited for EOF instead of forwarding an 88-byte client proof;
  the bridge now uses `os.read` and a live-pipe regression test. After that,
  Colima's container clock was observed about 140 ms ahead of the host, so a
  Broker deadline at the exact 60-second receiver maximum was rejected before
  provider execution. Broker-created refreshes now default to 45 seconds while
  the receiver retains the 60-second hard maximum; Python and Go clock-margin
  tests cover both acceptance and rejection.
- Compatible API-v2 Auth Broker and Gateway image publication and immutable
  digest pinning are explicitly authorized and remain the final active release
  step before this temporary packet is removed.
