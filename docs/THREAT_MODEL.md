# Threat Model

This document records first-public-V1 threats and residual limits. The durable
security contract is [Security Model](03_security_model.md).

## Assets and trust

Trusted: host OS/user, Docker Engine or its Linux VM, Tobari CLI, Gateway, OPA,
Auth Broker, host root-key provider, owner policy/preset/static-provider data,
encrypted Context vaults, and the reviewed GitHub host driver.

Untrusted: every Workspace process, coding agent, selected source, Workspace
home, runtime image, downloaded package, request/response content, copied
handle, upstream service, and external text displayed by the CLI.

Protected assets include host files outside the selected source, Docker
control, other Workspaces, Gateway/OPA administration, direct Internet egress,
the installation root key, static broker primary secrets, exact Context/project
policy authority, and audit integrity.

Files exposed by a read-write source are not protected from that Workspace.
A read-only source prevents writes through the direct bind but is a live view,
not a snapshot: host and same-root read-write Context changes remain observable.
Workspace home and tmpfs remain writable. Overlapping roots intentionally share
host-file effects and provide no filesystem-integrity isolation.

## Principal threats and controls

### Direct or cross-Workspace egress

Each Workspace joins only its dedicated internal network. A verified namespace
guard routes ordinary traffic through the exact Gateway endpoint and rejects
unexpected on-link, UDP, IPv6, and direct paths. Gateway forwarding remains
disabled and its forward chain drops. Synthetic DNS is non-recursive and
performs no external lookup before allow. OPA/Gateway outage denies.

Residual risk: Docker/kernel/VM escape and covert channels are outside V1.

### Caller-selected Context or project authority

Gateway derives stable Context/project identity from the kernel-observed source
endpoint and owner-only principal registry. Headers, SNI, URLs, Context names,
profiles, and environment cannot select a principal. Missing, duplicate, stale,
or ambiguous bindings fail before OPA, Broker resolution, DNS, or upstream I/O.

### Policy widening

Learned authority is exact Context, project, scheme, host, port, method, and raw
path; trusted GraphQL endpoints add operation type and root field. Query,
headers, body, observation count, labels, ordering, path similarity, and
indentation do not widen identity. Prefix learned rules and compaction do not
exist.

The immutable preset guardrail is evaluated before baseline data, learned
rules, and Advanced Rego. `builtin/offline` terminally denies all HTTP/HTTPS;
`builtin/reviewed-exact` exposes only eligible exact candidates;
`builtin/get-only-reviewed` exposes only eligible GET candidates and terminally
denies HEAD/non-GET. None grants immediately and GET has no safe/read-only
classification. Terminal denial creates no candidate and causes zero external
DNS, Broker resolution, or upstream call.

Custom presets are strict owner-only non-executable schema-V1 data, normalized,
validated, digested, and snapshotted at Context creation. Wildcards, IP/private
destinations, secrets, shell, Rego, includes, inheritance, remote fetch,
refresh, signing, symlinks, unsafe modes, and unknown fields fail closed.

### Secret exposure or policy bypass

Workspace-owned login state is deliberately inside one Workspace home. Tobari
does not inherit host CLI homes, keychains, SSH agents, or credential
environment variables.

The optional Broker stores one closed typed credential record in an AES-256-GCM
Context vault and projects only a random handle bound to Context, project,
provider, revision, and exact HTTPS header/signing plan. Gateway removes and
introspects a recognized handle before OPA, then performs one same-revision
static resolution, Datadog/OpenAI/Anthropic token action, or bounded AWS SigV4 action
only after allow. Malformed, copied, stale, revoked, ambiguous, or mismatched
handles fail without passthrough fallback. Secrets, raw handles,
credential revisions, query, headers, and bodies do not enter OPA, audit,
denial evidence, CLI output, or logs.

Managed profiles remain absent. Dynamic credentials, refresh, signing,
supplemental headers, companion calls, and task barriers exist only inside the
closed reviewed AWS, Datadog, OpenAI, and Anthropic plans. Owner manifests cannot select
or extend them. No authentication path grants a policy bypass.

Residual risk: a Workspace can copy its own handle or Workspace-owned secret as
ordinary payload to an allowed destination. Payload exfiltration to explicitly
allowed effects is outside the guarantee.

### Provider helper execution

The reviewed helper set is GitHub CLI, AWS CLI, pup, a contract-checked stable
Codex CLI, and Claude Code 2.1.220. Tobari resolves canonical non-project executables for
the first four and exact `/usr/local/bin/claude` from the selected Context image, rejects unsafe
identity, hashes before and after, runs only fixed argv under private state or
isolated container boundaries, accepts only bounded browser/output contracts, and performs
checked cleanup. The Claude container receives no mount, volume, project path,
Docker socket, or Broker socket, and its removal is a commit precondition.
Codex product version is recorded rather than allowlisted;
its exact compiled login/state contract determines acceptance. Owner manifests
cannot select a helper.

### Source-access confusion

Context manifests require immutable `read-only|read-write`. Omission defaults
are applied only by creation. Runtime spec/hash and Docker inspection include
the access mode. Read-only integration proves reads work; source mutation and
Git metadata writes fail; home/tmpfs writes work; and no writable alias exists.

Residual risk: an external host or overlapping read-write Context can change
bytes observed by a read-only Workspace.

### State corruption, downgrade, or partial activation

All Tobari-owned schemas and component APIs are exactly V1. Unsupported,
missing, partial, unsafe-permission, or symlinked state fails closed without
migration/fallback. Mutation locks, durable journals, complete generation
replacement, content-addressed aggregate bundles, OPA preflight, atomic publish,
exact active-revision confirmation, and known-good rollback prevent mixed
authority.

### Resource exhaustion

Workspace and shared services have fixed CPU, memory-plus-swap, PID, and
container-log bounds. Request/response advertised body size, GraphQL body,
protocol frames, CLI output, retained log windows, subprocess output, and
timeouts are bounded.

Residual risk: V1 claims no disk quota for the selected source/home, network
bandwidth shaping, or per-project fairness within a shared Gateway/OPA/Broker.

### Destructive lifecycle mistakes

Every owned Docker resource carries exact owner/ID/role labels. Deletion uses
canonical CWD selection and exact stored resources, never a name prefix. An
attached session blocks ordinary delete; `--force` is explicit. Cluster removal
requires no Workspaces. Down/purge preserve vaults and root key; logout is the
credential-revocation operation.

### Supply-chain or release confusion

Canonical Gateway/Auth Broker source is byte-checked against embedded
snapshots. Provider CLIs and credentials are absent from Broker image layers.
Official component images require independently reviewed immutable amd64/arm64
indexes; moving tags are not runtime authority. CLI archives have exact
checksums, archive-subject SPDX metadata, and unsigned in-toto/SLSA metadata,
none of which is represented as a signature or full dependency/layer inventory.

No external publication occurs during local preparation. Explicit approval is
required before branch/tag push, OCI publication, GitHub Release creation, or
Homebrew tap update.

## Verification evidence

- unit/contract tests for domain, application, catalog, state, Gateway, OPA,
  Auth Broker, provider loading, root key, and presentation;
- Docker integration for topology, source access, guardrail zero-call ordering,
  exact learning/reset, closed Broker plans, rotation/revocation, and
  outage denial;
- dependency/image-content scans proving managed profiles, arbitrary helpers,
  compatibility readers, and Broker provider CLIs are absent;
- secret canaries and repository public-boundary scans;
- `task check`, `task security`, `task public:check`, and `task release:check`;
- manual disposable reviewed-provider acquisition recording secret-free
  pass/fail only.

Sensitive findings follow [SECURITY.md](../SECURITY.md), never a public issue.
