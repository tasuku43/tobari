# ADR 0026: Route HTTP transparently through a guarded Gateway

- Status: Accepted
- Date: 2026-08-11
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, harness, public boundary, and release
- Supersedes: ADR 0005
- Superseded by: None
- Revised by: ADR 0027 resets the principal, cluster, OPA, audit, and Gateway
  API contracts to exact V1 and removes their migrations

## Context

ADR 0005 selected an explicit intercepting proxy because it avoided host
firewall mutation, persistent network capability, and platform-specific
routing. That topology fails closed for a client that ignores proxy variables,
but it does not deliver Tobari's HTTP authorization outcome for that client.
Tool-specific wrappers or registration would make coverage depend on a growing
compatibility catalog rather than the attempted HTTP effect.

An isolated Colima/Docker Engine experiment proved that a Gateway-local
nftables REDIRECT receives Workspace TCP while IPv4 forwarding remains zero
and the forward chain drops unconditionally. Synthetic DNS supplied a
non-public address without recursive lookup. Transparent HTTPS on port 6443
was denied without an upstream connection and allowed only after the validated
authority replaced the synthetic destination. Raw TCP produced no upstream
connection with lazy establishment and raw-TCP forwarding disabled.

The same experiment falsified ADR 0011's Gateway-local-address mechanism for
transparent traffic: mitmproxy reports the original redirected destination as
the accepted socket's local address. It does preserve the kernel-observed
Workspace source address.

## Decision drivers

- Ordinary HTTP/HTTPS clients receive the same policy behavior without
  tool-specific proxy support.
- A denied request performs no recursive DNS, credential resolution, or
  upstream connection.
- Workspaces and the resident Gateway retain no added capability and no direct
  egress interface.
- No host or Docker-VM-global firewall state is changed.
- One shared Gateway and OPA continue to enforce every Context and project.
- Caller-controlled headers, names, SNI, URLs, and environment remain unable
  to select a Context/project principal.

## Considered options

### Host or VM firewall capture

This gives transparent client behavior but makes Tobari a co-owner of
Docker-managed global routing and firewall state. Rollback and coexistence are
platform-dependent and can affect unrelated workloads.

### Shared Gateway/Workspace network namespace

This removes a routing hop but gives untrusted work the egress-capable
namespace. A packet-filter defect would then be the only barrier to direct
reachability.

### Guarded project next hop and synthetic DNS

The Workspace retains only its dedicated internal network. Fixed one-shot
helpers configure the existing Workspace and Gateway network namespaces, then
exit before user work begins. Gateway forwarding remains disabled; selected
traffic terminates at local HTTP and DNS listeners.

### One Gateway per Workspace

This simplifies principal demultiplexing but multiplies trusted resident
processes, CA and policy clients, health surfaces, and resource bounds without
removing the guarded-routing requirement.

## Decision

Tobari adopts the guarded project-next-hop design as its only Workspace HTTP
ingress. A non-root Gateway listener accepts TCP redirected locally from every
project interface and feeds the normalized HTTP/HTTPS authorization pipeline.
Workspace DNS is sent
to a non-recursive Gateway listener that returns one bounded synthetic IPv4
answer and never forwards the question. After OPA allows one validated HTTP
authority, Gateway replaces the synthetic destination, performs the existing
resolve/public-address/pin checks, and makes the upstream connection.

The trusted runtime invokes an immutable, fixed-entrypoint helper with only
root and `CAP_NET_ADMIN` while sharing exactly one already-verified container
network namespace. It receives no bind mounts, secrets, Docker socket, host
network, project path, or executable selector. It atomically installs and
verifies one Tobari-owned ruleset and route, then exits. The long-lived
Workspace and Gateway remain read-only, non-root where already required, with
all capabilities dropped. Gateway IPv4 and IPv6 forwarding remain disabled
and its forward path drops unconditionally. No host-global rule is installed.

The principal registry advances to schema 3. Each binding contains the stable
Context/project identity, dedicated owned project network, exact owned
Workspace endpoint, and exact Gateway endpoint. Transparent ingress derives
authority only from the kernel-observed source endpoint and the complete
owner-only binding. Registry validation rejects duplicate networks,
Workspace addresses, Gateway addresses, incomplete ownership, and stale
endpoints. SNI and HTTP authority select only the requested destination; they
never select the caller principal.

Raw TCP, non-HTTP TLS, UDP, QUIC, recursive DNS, DNS policy, certificate
pinning compatibility, and private client trust stores remain unsupported and
have no forwarding fallback. All TCP ports enter bounded HTTP/TLS detection so
an HTTPS service on a port such as 6443 is not dependent on a tool registry.

## Consequences

### Positive

- Proxy-oblivious clients using ordinary HTTP/HTTPS sockets participate in the
  same deny-review-allow-retry loop.
- One ingress removes listener, environment, nftables, status, and test parity
  surfaces that could otherwise drift independently or hide a broken
  transparent route.
- The Workspace still has no external interface, and no resident process gains
  network-administration authority.
- Pre-policy DNS exfiltration is closed instead of being accepted as the price
  of transparent routing.

### Negative

- Gateway images include nftables/iproute tooling for the one-shot helper and
  a bounded synthetic DNS listener.
- Runtime entry and cluster startup gain an exact namespace-reconciliation
  step and a new kernel/API compatibility requirement.
- Clients requiring AAAA, HTTPS/SVCB, recursive DNS semantics, certificate
  pinning, raw transport, or private trust stores fail rather than bypass.
- Unsupported protocol failure latency depends on bounded protocol detection
  and must be measured for supported clients.

### Risks and mitigations

- A routing error could create forwarding: forwarding sysctls, the forward
  drop, absence of masquerade/SNAT, routes, and exact rules are verified before
  entry and by integration tests.
- Source spoofing could select another principal: one Workspace endpoint per
  non-overlapping dedicated network, capability drops, on-link output rejects,
  registry uniqueness, and source-bind/IP_FREEBIND canaries are required.
- DNS could become a pre-policy channel: the listener is non-recursive,
  bounded, stateless for A answers, and omits full names from routine logs;
  external-DNS call-count canaries stay at zero.
- Partial or stale guard state could weaken the claim: entry always reconciles
  and verifies the exact owned revision, and failure prevents `docker exec`.

## Mechanical enforcement

- Domain tests validate complete guarded state and schema-3 endpoint
  uniqueness independently of Docker syntax.
- Runtime argv tests require exact owned-container namespace targets, the fixed
  image/entrypoint, read-only filesystem, no mounts, all capabilities dropped
  except helper `NET_ADMIN`, no-new-privileges, forwarding-off sysctls, exact
  DNS address, and guard-before-entry ordering.
- Gateway tests reject non-transparent ingress and cover authority conflicts,
  synthetic destinations, arbitrary HTTPS ports, lazy post-allow destination
  replacement, and zero DNS/upstream calls for denials and unsupported
  protocols.
- Docker integration inspects routes, sysctls, nftables state, capabilities,
  networks, source bindings, DNS behavior, cross-project denial, Gateway/OPA
  outage, restart/re-entry reconciliation, and cleanup.
- Gateway source/snapshot, API-label, dependency/license, multi-architecture,
  public artifact, and release checks cover the added image contents.

## Compatibility and migration

No command, flag, opaque reference, policy schema, or denial identity changes.
Tobari no longer injects `HTTP_PROXY`, `HTTPS_PROXY`, or `NO_PROXY` variants,
does not listen on `gateway:8080`, and does not retain port-8080 guard
exceptions. Keeping that route would make transparent interception optional in
practice and expand the trusted and tested attack surface without a supported
client need.
The owner-only registry migrates from schema 2 to schema 3 only from verified
live owned endpoints; incomplete historical bindings are omitted and fail
closed until lifecycle reconciliation. The Gateway source API advances from 4
to 5. Previously published API-3 images remain historical selections and do
not claim transparent routing.

Persisted cluster state advances from schema 3 to schema 4. Loading accepts
only the exact former `http://gateway:8080` value, removes it, and atomically
persists the remaining state; malformed or unfamiliar legacy state fails
closed. Cluster status JSON advances from schema 5 to schema 6 and removes the
`proxy` field. Neither migration recreates the retired ingress.

Rollback to an older source requires recreating Gateway and Workspace network
namespaces from Docker's closed internal-network baseline; it must not attempt
to reinterpret schema-3 source bindings as schema 2.

## Security and public-boundary impact

The new trusted effect is a bounded network-namespace mutation performed by a
short-lived helper. New public image contents are signed-distribution nftables
and iproute packages plus Tobari-owned guard and DNS source. No new credential,
host mount, external destination, listening host port, or Docker-socket access
is introduced. DNS names and HTTP authority remain untrusted and potentially
sensitive; the DNS listener must not log complete questions in routine output.

## Validation

- `task gateway:test`
- `task runtime:test`
- `task check`
- `task security`
- `task public:check`
- `task release:check`
- Manual runtime observation on the minimum supported native Linux, Colima,
  and Lima Engine/kernel combinations before publishing API 5

## Reconsideration signals

Supersede this ADR if an owned Workspace can spoof another registered endpoint,
if any pre-allow DNS or upstream call occurs, if local REDIRECT requires
forwarding or host-global state on a supported Engine, if modern clients do not
reliably fall back from unsupported DNS/QUIC behavior, or if a smaller
kernel-enforced primitive provides equivalent cross-platform guarantees.
