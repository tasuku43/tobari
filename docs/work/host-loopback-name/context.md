# Work Context: Give physical-host loopback an honest private authority

This file separates verified repository facts, verified external facts,
Product Owner-fixed decisions, assessment, inference, and implementation-time
observations. Desired behavior belongs in `goal.md` and `plan.md`.

## Baseline and inspection record

### Verified repository facts

- On 2026-08-23, `git fetch origin main` succeeded. `HEAD`, local `main`, and
  `origin/main` were all
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`
  (`chore(ci): retire obsolete runtime workflows`). `git status --short
  --branch` reported a clean `main...origin/main` before this packet was
  created.
- The inspection read `AGENTS.md`, `docs/00_theses.md` through
  `docs/04_harness.md`, `docs/07_authentication.md` through
  `docs/09_agent_readiness_validation.md`, the README, CLI Catalog output
  declarations, Host Loopback domain/application/infrastructure/Gateway code,
  focused tests, ADRs 0049 and 0074, active work packets, and recent related
  commits in numeric/governing order.
- Relevant history includes `80bc6cd` (initial reviewed Host Loopback),
  `32aec8c` (route reviewed Host Loopback traffic), `1c5f8b4` (correct its
  authority model), `c9aae59` (authority scope and lifetime), and `1b2341c`
  (opposite-direction reviewed Workspace service exposure).
- No current public CLI command accepts a Host Loopback hostname. The name is
  ambient capability data and appears in `review permissions`, candidate/denial
  fields, README/agent guidance, and runtime/Gateway state. A binary invocation
  was therefore not needed to establish the name contract; the Catalog,
  executable sources, and tests are the direct sources of truth.
- The current public-boundary guard rejects a URI literal whose authority is a
  private hostname. This packet therefore writes the accepted scheme and
  authority separately. Implementation must revise the governing guard with
  one exact product-owned synthetic-authority exception and negative sibling
  tests; it must not disable the general private-hostname protection.
- **Verified upstream synchronization, 2026-08-23:** integrated `HEAD` is
  `52a53bcc69a0f2bdf9bf2a6782ecd98bacd8b0e1`. Commit `07535a9` promotes the
  Workspace Manifest and copy contracts through theses, product, architecture,
  harness, CLI, schema, migration, and implementation; commit `428812f` adds
  accepted [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and its security/migration consequences. The predecessor temporary packet
  files no longer exist and are not authority.
- **Verified shared-checkout hygiene observation, 2026-08-23:** `git status
  --short --branch` reports no tracked production modification and only
  untracked work-packet directories. All concurrent packet changes are
  user-owned and protected; this synchronization edits only this packet's
  `goal.md`, `context.md`, `plan.md`, and `tasks.md`, without staging or
  committing.
- The current-main facts below remain a **packet-creation baseline**, not an
  implementation-time contract. WP 05 must re-observe the integrated
  post-WP-04 `HEAD`, working tree, contracts, code, tests, and safe runtime
  behavior after the unchanged completed promotion -> WP08 -> WP03 -> WP04
  sequence and before any production change.

### Durable Workspace Manifest and copy contract

- [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  removes public Context vocabulary. Product language is **Workspace
  Manifest**, routine presentation is **Manifest**, CLI selection is
  `manifest`/`--manifest`, and schema authority is
  `workspace_manifest_id`, with no alias.
- The only durable resources in this area are Workspace Manifest, Runtime, and
  Workspace. Project root and Runtime revision are subordinate typed values.
- A Workspace is durably bound to `(ProjectRoot, WorkspaceManifestID)` and has
  one stable `workspace_id`. Current-main Context UUID bytes migrate to
  WorkspaceManifestID; ProjectInstance UUID bytes migrate to WorkspaceID.
- Workspace Manifest revisions are complete immutable desired declarations.
  Desired, last successfully applied, observed, and latest bounded failure are
  separate. Only explicit Workspace entry reconciles one Workspace, and only
  `cluster up` reconciles shared cluster state.
- Standard authentication, learned permission, Host Loopback routes, and
  Attachment Grants are outside Manifest desired/applied state. Attachment
  authority is keyed to the trusted current WorkspaceManifestID/WorkspaceID
  principal and Attachment Epoch.
- ADR 0079 leaves sufficient Docker migration evidence as an implementation
  gate. For WP 05's specific transient cleanup, the Product Owner has fixed the stronger
  precondition as cluster stopped plus zero live attachment; the post-WP-04
  gate must consume the actual final state and schema shapes.

### Accepted one-time copy and adjacent WP 03 decisions

- ADR 0079 fixes `manifest create --copy-from NAME --name NAME` and `runtime
  create --copy-source-from standard|NAME --name NAME`; `--base` has no alias.
  Both operations create fresh independent identities and retain no
  provenance, lineage, `copied_from`, current selection, Workspace, auth,
  learned permission, attachment, AppliedEntry, failure, or observed state.
  Neither copy action reconciles runtime or attachment state.
- A copied Manifest therefore receives a fresh WorkspaceManifestID and carries
  no Host Loopback authority. A Host Loopback route or grant can arise only
  from a separately bound Workspace and explicit entry/Attachment Epoch/review.
  Hostname or route migration cannot correlate state through a source Manifest,
  provenance, or lineage field because no such authority exists.
- WP 03 fixes whole-Runtime retirement, exact prune-plan discovery/application,
  digest-exact restore, reference-bound build, and read-only Runtime Review.
  Its protection graph distinguishes retained Manifest revisions, Workspace
  last-successful AppliedEntry and pending adoption, and observed Docker use.
  Host Loopback routes, grants, candidates, relay tokens, and attachment policy
  are not Runtime revision/history/image material and are not `last_used`
  evidence.
- WP 03's nested output producer/consumer rule is Catalog-wide. WP 05 adds no
  Runtime-only validator or reference edge. Neither ADR 0079's copy contract
  nor WP 03 changes the
  hostname candidates, DNS/TLS criteria, accepted selection, or Host Loopback
  trust-boundary decision.

### Product Owner-fixed WP 05 decision

- `host.tobari.internal` is the only public and routable authority. The old
  name has no alias, redirect, translation, suffix, or wildcard path.
- Authority is the conjunction of exact hostname, port, method/path, trusted
  WorkspaceManifestID/WorkspaceID principal, live Attachment Epoch, trusted-
  host grant, and route revalidation. `.internal`, DNS success, ordinary
  external policy, and presentation text grant nothing.
- `host.tobari.test` remains a terminal retired authority for all V1 and cannot
  fall through to external candidate creation, learned permission, external
  DNS, Broker, upstream, or relay. Removal needs a separate ADR and negative
  safety evidence.
- Existing `migrate apply` alone owns cleanup. With cluster stopped and zero
  live attachment, it accepts only exact owner/schema/old-host transient route
  and grant registries and replaces them atomically with empty schema-V2
  registries. No read, entry, `cluster up`, or new maintenance command cleans
  them implicitly.
- Transient route/grant registries are V2 and issue new opaque IDs. Route ID V2
  directly binds the exact hostname; there is no AuthorityRevision concept.
  Public/helper-visible `TOBARI_CAPABILITIES_JSON` remains V1 with only its
  hostname value replaced. These version domains must not be conflated.
- Denied or retired HTTPS is classified before any leaf certificate is
  generated. Only an actually observed exact old-host leaf entry may be
  removed by `migrate apply` after owner validation; the root CA and broad
  cache remain untouched.
- The release floor is the actual standard Runtime inventory: libc/getaddrinfo
  through curl, Python, applicable Go pure/cgo paths, and Node DNS plus HTTP.
  Java and browser remain optional unless the standard Runtime contains them.
- The public-boundary exception is exact string equality with the product-owned
  `host.tobari.internal` synthetic authority. Full-URI generalization,
  siblings, wildcard descendants, and other `.internal` names remain denied.
- The new Host header is preserved through physical-host `127.0.0.1` relay and
  is the same route/grant/policy/audit identity. Host rewriting would require a
  separate trust-boundary decision.
- The implementation order is fixed as completed Workspace Manifest/copy
  promotion -> WP08 -> WP03 -> WP04 -> WP05.
  WP 05 consumes actual final identity, Catalog, CA/DNS/policy, migration, and
  schema contracts rather than anticipating them.

### Current owner, scope, lifetime, and mutability

| Item | Owner | Scope | Lifetime | Mutability today |
|---|---|---|---|---|
| Public Host Loopback authority | Tobari product contract | Exact `host.tobari.test` plus non-privileged port | Product/release constant | Immutable within the build |
| Capability projection | Host runtime, projected into one Workspace | Workspace audience; URL template and limits | Interactive attachment environment | Constant and secret-free |
| Attachment route | Host attachment process | Legacy current-main Context ID, project ID, Attachment Epoch, exact hostname | Owning attachment | Atomically added/removed; borrower cannot extend owner lifetime |
| Attachment grant | Trusted-host reviewer and owning attachment | Legacy current-main Context ID/project ID plus exact epoch, host, port, method, and path | Attachment | Added by exact review; revoked at route teardown |
| Learned external rule | Legacy Context-keyed policy store | Ordinary external destination only | Persistent until reset | Separate from attachment and future Manifest desired/applied state; cannot authorize Host Loopback |
| Relay coordinates | Infrastructure adapter | One active route | Attachment | Random relay port and 256-bit token; never projected publicly |
| `host.docker.internal` | Docker/Gateway infrastructure | Gateway container to physical host relay listener | Runtime transport | Compose `host-gateway` mapping; not authority |
| Gateway CA | Installation-shared Gateway infrastructure | Ordinary transparent HTTPS interception | Shared cluster state; purge rotates | Not used to authorize current plain-HTTP Host Loopback |

### Current data and request path

1. `internal/domain/tobari/host_loopback.go` defines
   `HostLoopbackHostname = "host.tobari.test"`, the HTTP URL template,
   capability schema 1, route/grant registry schema 1, ports 1024-65535,
   attachment lifetime, Workspace audience, and policy-review-required access.
2. Every interactive entry creates or borrows an Attachment Epoch and projects
   the secret-free capability. Workspace `localhost` remains Workspace-local.
3. `gateway/addon/synthetic_dns.py` is non-recursive and returns the synthetic
   A address `198.18.0.10` with TTL 0 for every syntactically valid A query;
   other query types receive no address. The global DNS does not decide the
   Host Loopback destination.
4. The Gateway recognizes the exact HTTP Host authority, binds it to the
   principal's active route and epoch, and asks OPA about the separate
   `host_loopback` destination. Ordinary external policy cannot decide it.
5. Only after allow, the Gateway opens a one-shot bridge to its private
   `host.docker.internal:<relay-port>` transport. The relay authenticates the
   random token, revalidates the reviewed target port, and dials physical-host
   IPv4 `127.0.0.1:<target-port>`.
6. The original Host header is preserved to the physical-host service. The
   public authority is therefore observable to that service and is not merely
   a documentation label.

### Current identity and schema consequences

- Host Loopback policy candidates and denials carry the exact host. Policy
  candidate references consequently change when the host changes.
- Attachment grant ID derivation includes the hostname. Existing grant
  references cannot be reused under a new name.
- Route ID derivation currently binds epoch, legacy Context ID, and legacy
  project ID but omits the hostname; the route object separately validates the
  exact host. Final V1 must replace those semantic fields with
  WorkspaceManifestID and WorkspaceID and bind the canonical authority into
  the final-V1 route identity.
- Route and grant registries are owner-only attachment runtime files, not
  durable learned policy. Current readers strictly require schema 1.
- `TOBARI_CAPABILITIES_JSON` schema 1 embeds the URL template. In isolation,
  changing that value would normally require a schema bump. ADR 0079 and the
  current product contract instead define one pre-public exact-contract reset: the final public capability
  contract remains V1, while only `migrate apply` recognizes the exact
  current-main predecessor state.
- **Accepted target distinction:** the owner-only transient route/grant
  registries move from current schema 1 to schema V2, while public/helper-
  visible `TOBARI_CAPABILITIES_JSON` remains schema V1. Registry V2 is not a
  public capability schema-2 compatibility surface.
- The CLI Catalog's candidate/denial JSON schema already has `host`,
  `destination_kind`, `authority_lifetime`, and `attachment_epoch_id` fields.
  The field shapes need not change, but observed values and opaque references
  do.
- Current Host Loopback is HTTP-only. Gateway TLS authority normalization
  requires Host/SNI consistency for HTTPS generally, but Host Loopback rejects
  `https`. The shared private CA does not currently authenticate the physical
  host service or add Host Loopback authority.

## Verified external facts

Sources were checked on 2026-08-23 and are official registries,
standards, product documentation, or first-party source repositories.

### DNS reservation and resolver semantics

- The [IANA Special-Use Domain Names registry](https://www.iana.org/assignments/special-use-domain-names)
  reports `Last Updated: 2026-05-22`, lists `.test`, `.invalid`, and
  `.localhost`, and says the designation applies to their subdomains. It does
  not list `internal.`.
- [RFC 2606](https://www.rfc-editor.org/info/rfc2606/) reserves `.test` for
  testing, `.invalid` for names intended to be obviously invalid, and
  `.localhost` for traditional loopback use.
- [RFC 6761](https://www.rfc-editor.org/info/rfc6761/) permits private positive
  answers for `.test`, but says `localhost` and every name beneath it should
  resolve to IP loopback and should not be sent to configured caching DNS
  servers. It says `.invalid` names should receive immediate negative answers.
- ICANN Board [resolution 2024.07.29.06](https://www.icann.org/en/board-activities-and-meetings/materials/approved-resolutions-special-meeting-of-the-icann-board-29-07-2024-en)
  permanently reserves `.INTERNAL` from delegation in the DNS root for
  private-use applications. The rationale explicitly distinguishes that ICANN
  reservation from the IANA Special-Use registry.
- As of the inspection date,
  [draft-davies-internal-tld-06](https://datatracker.ietf.org/doc/html/draft-davies-internal-tld-06)
  is an active individual Internet-Draft with intended Informational status,
  expires 2026-11-07, and has no formal standing in the IETF standards
  process. `.internal` therefore has ICANN root non-delegation, not RFC 6761
  resolver mandates.

### Runtime and library behavior

- W3C [Secure Contexts](https://www.w3.org/TR/secure-contexts/) allows plain
  HTTP origins whose host is `localhost` or a `.localhost` descendant to be
  treated as potentially trustworthy only when the user agent enforces
  loopback resolution. Using
  `.localhost` would add both address and browser-security semantics that
  Tobari does not own.
- [Node.js DNS documentation](https://nodejs.org/api/dns.html) distinguishes
  OS-backed `dns.lookup()` from `dns.resolve*()`, which sends network DNS
  queries directly. [Go package `net`](https://pkg.go.dev/net) likewise may use
  a native `getaddrinfo` path or a pure-Go DNS resolver. Exact compatibility
  must therefore cover both OS and direct-DNS families rather than assuming
  one resolver path.
- ADR 0074 records a repository-local compatibility observation: a random
  `.localhost` name was rejected by Jupyter Server's default remote-access
  check for the opposite-direction service-exposure task. That does not prove
  Host Loopback behavior, but it proves that library/application treatment of
  local names is not uniform.

### Container-runtime precedents

- [Docker Desktop](https://docs.docker.com/desktop/features/networking/networking-how-tos/)
  documents `host.docker.internal` as the host address and
  `gateway.docker.internal` as the Docker VM gateway. Docker's `host-gateway`
  mapping is an engine transport feature, not an application authorization
  identity.
- [Podman](https://docs.podman.io/en/latest/markdown/podman-run.1.html)
  documents `host.containers.internal` and `host.docker.internal`, with
  availability dependent on its ability to determine a host-gateway address.
- Colima's first-party
  [default configuration](https://github.com/abiosoft/colima/blob/main/embedded/defaults/colima.yaml)
  maps `host.docker.internal` to `host.lima.internal` through its internal
  resolver.
- [OrbStack](https://docs.orbstack.dev/machines/network) documents
  `host.orb.internal` for a Linux machine to reach a macOS host and separately
  documents `docker.orb.internal` for forwarded Docker services. These names
  are useful mental-model precedents, not portable standards or Tobari policy
  inputs.

### TLS

- The current [CA/Browser Forum Baseline Requirements](https://cabforum.org/working-groups/server/baseline-requirements/requirements/)
  prohibit publicly trusted certificates containing internal names. They do
  not govern an enterprise/private PKI whose root is not distributed by a
  public application-software supplier.
- Tobari already owns a private Gateway CA for transparent HTTPS. That makes a
  future private TLS design technically possible, but it does not turn
  `.internal` into authenticated identity and does not justify adding HTTPS to
  this rename.

## Assessment

### Candidate comparison

ADR 0079 changes the principal vocabulary and migration boundary, not the
hostname decision criteria. The candidates remain evaluated against DNS,
resolver, TLS, mental-model, and policy properties; none is derived from the
Workspace Manifest noun.

| Candidate | Global collision | Resolver / loopback behavior | Mental model | TLS and SNI | Synthetic DNS, policy, private ceiling | Assessment |
|---|---|---|---|---|---|---|
| `host.tobari.internal` | Root delegation permanently withheld | No standardized loopback short-circuit | Physical host through a private Tobari projection | Private CA required for any future TLS; exact SNI remains possible | Exact-name branch works; `.internal` suffix grants nothing | **Accepted/Fix** |
| `host.tobari.test` | Special-Use and collision-free | Private positive answers permitted | Sounds like test data or a test-only service | Private CA required; exact SNI possible | Current design works safely | Technically sound, semantically misleading |
| `host.tobari.localhost` | Special-Use and collision-free | Required/encouraged to resolve to the caller's loopback, often without DNS | Confuses physical-host loopback with Workspace loopback | Browsers may confer secure-context semantics on plain HTTP | Can bypass synthetic DNS and address the Workspace itself | Reject |
| `host.tobari` | `.tobari` is not reserved | Search-path and future root-delegation behavior is uncontrolled | Memorable but ownership is ambiguous | Future global namespace collision affects SNI | Cannot promise collision-free private routing | Reject |
| `host.tobari.invalid` | Special-Use and collision-free | Expected to fail locally or return negative | Says the reachable capability is invalid | Some clients can fail before SNI/Gateway | Conflicts with a positive synthetic answer | Reject |
| `gateway.tobari.internal` | Root delegation permanently withheld | Resolver-neutral | Names an implementation hop, not the physical-host destination | Could make TLS termination look like destination authority | Encourages policy to follow topology | Reject |
| Projection-only / no public name | No DNS collision | An HTTP URL still needs an authority | Hides the only usable address from users and agents | Cannot express stable Host/SNI identity | IP-only routing would collapse presentation into transport | Reject |
| Other suffix such as `.local` or `.private` | `.local` is mDNS Special-Use; `.private` is not reserved | Adds mDNS behavior or future collision | Familiar but not owned | Varies | Adds more special handling or less safety | Reject |

### Accepted decision rationale

- `host.tobari.internal` preserves the useful `host.<product>.<private-use>`
  pattern seen in container runtimes without adopting any runtime's authority
  or topology.
- It is safer than `.localhost` because Tobari needs the name to resolve to the
  Gateway synthetic address, not the Workspace's `127.0.0.1`/`::1`.
- It is more honest than `.test` because the capability is used in ordinary
  agent development after explicit review, not to test DNS code.
- It is safer than an unreserved suffix because ICANN has permanently removed
  `.internal` from root delegation, while still leaving resolution under the
  local administrator's explicit control.
- The exact three-label name remains memorable and names the user outcome.
  `gateway` would expose a replaceable mechanism and would blur presentation
  identity with authorization authority.

### Public and internal concept count

The target design adds no durable public resource to ADR 0079's three-resource
budget. Within the existing operational and authority vocabulary it retains
one Host Loopback capability, one Attachment Epoch, one exact Attachment
Grant, and the existing review workflow. It renames one public constant and
adds no resource, command, role, reference kind, or policy lifetime.

Internally, there is one routable canonical authority plus one terminal
`retired_host_loopback_authority` sentinel. The sentinel is not an alias,
candidate, grant, route, or second destination kind. Splitting DNS name,
presentation host, and policy host into independently mutable concepts would
create false independence and is rejected.

### Authority and presentation identity

- The exact canonical hostname is both the HTTP presentation authority and one
  dimension of Host Loopback policy identity. In final V1 it is combined with
  WorkspaceManifestID, WorkspaceID, Attachment Epoch, target port, method, and
  path before any physical-host dial. Project root may remain bounded display
  evidence but is not a parallel ID.
- The synthetic address is routing projection only. It must never appear as
  the policy host, user URL, review identity, or certificate name.
- `host.docker.internal` is private transport identity only. It must never
  enter capability JSON, OPA input, denials, audit, or public recovery text.
- `.internal` is neither authentication nor provenance. Only exact Tobari
  binding plus the reviewed route/grant authorizes access.
- Gateway preserves `host.tobari.internal:{port}` as the Host header when the
  authenticated relay dials `127.0.0.1`; physical transport does not rewrite
  the route/grant/policy/audit identity.
- A Manifest revision digest is desired-declaration authority, not Attachment
  Grant authority. Publishing a revision does not mutate the active route or
  grant. A later entry that creates a new Attachment Epoch receives no grant
  from the prior epoch.

### Trust-boundary assessment

The rename changes no trust boundary. The Workspace still reaches only a
shared synthetic Gateway; Gateway remains fail-closed before the authenticated
relay; the relay dials only physical-host IPv4 loopback; and trusted-host
review remains required. The implementation must match the full exact hostname
and must not generalize Host Loopback to `*.tobari.internal`, `*.internal`, a
CNAME target, or any private address. This preserves the ordinary private
destination ceiling and the one explicit Host Loopback exception.

A routable old-name alias would be an authority expansion: either one new
grant would authorize two Host headers, or two parallel grants/routes would
exist. Neither is necessary before first public V1. A redirect would also make
client retry behavior part of the security path. The migration therefore uses
a terminal old-name fault, never an alias or redirect.

The durable Workspace Manifest rename changes principal field names but not the boundary:
the trusted host supplies WorkspaceManifestID/WorkspaceID to the selected
attachment. Workspace input, Manifest name, revision generation, Project root,
and presentation labels cannot reconstruct those IDs. Host Loopback remains a
separate attachment-owned branch rather than Manifest desired, applied,
observed, or failure state.

A presentation-only recommended Manifest draft has no WorkspaceManifestID and
cannot establish a principal, route, or grant. Read-only status/list/show/
doctor may report attachment observation but cannot reconcile or create the
Host Loopback branch. Only explicit Workspace entry may establish it.

## Inferences

- The container-runtime precedent supports the memorability of
  `host.tobari.internal`, but it does not establish interoperable resolver,
  policy, or TLS semantics. Tobari must retain its own exact synthetic-DNS and
  Gateway contract.
- Because the Host Loopback path is plain HTTP, changing its hostname does not
  cryptographically invalidate or require rotation of the shared root CA. The
  fixed target rejects denied/retired HTTPS before leaf generation; any
  already-existing observed leaf cache remains non-authoritative material.
- A hard pre-public cutover with fresh attachment review is less surprising
  than a compatibility alias because Host Loopback state is already explicitly
  attachment-scoped and non-persistent.

## Implementation-time observation unknowns

- [ ] After the already promoted Workspace Manifest/copy-contract stage, WP08,
      WP03, and WP04 complete in the unchanged order, record the actual
      integrated `HEAD` and clean-or-explained working tree; reread durable
      contracts; and
      inspect the final Workspace Manifest, Workspace, route/grant/principal,
      CA/DNS/policy, Catalog, JSON, migration, and test shapes. A contradiction
      is `WP05_BLOCKED`, not authority to redesign production locally.
- [ ] Re-run bounded read-only help and, where safe, fresh temporary-state
      observations from the post-WP-04 source. The current in-progress
      production tree must not be treated as final evidence or patched ahead
      of its owner.
- [ ] Inventory the actual standard Runtime after upstream implementation and
      pin exact curl/libc, Python, Go, and Node versions plus applicable Go
      pure/cgo modes. Observe Java/browser only if present; their absence is not
      a release blocker.
- [ ] Observe whether any exact old-host mitm leaf cache entry actually exists
      and, if so, its final owner/schema representation needed for bounded
      `migrate apply` deletion. Absence means no cache mutation.
- [ ] Record the read-only Docker evidence required by ADR 0079 migration. Host
      Loopback additionally needs evidence for stopped cluster, zero live
      attachment, and exact owner/schema/old-host matching before registry
      replacement; route/grant files provide no AppliedEntry evidence.
- [ ] Resolve ADR 0079's new-child-session behavior when entry adoption is
      pending. Whichever option is selected, a new session default cannot
      expand an existing exact grant, and a new Attachment Epoch cannot inherit
      one.

## Cross-packet dependencies and conflicts

- The first-public-V1 core and artifact packets must integrate the new name
  before publishing README, generated site, capability/schema ledgers, images,
  checksums, or readiness evidence. Publishing first would create a real
  compatibility obligation for `host.tobari.test`.
- The fixed order is completed Workspace Manifest/copy-contract promotion ->
  WP08 Catalog/domain output conformance -> WP03 Runtime retirement -> WP04
  build-profile contract -> WP05. Every remaining predecessor
  must complete before WP05's implementation-entry review; an in-progress
  schema, Catalog, build profile, or Runtime graph is not a frozen dependency.
- [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and the current theses/product/architecture/security contracts remain the
  identity and migration authority. WP 05 consumes their final vocabulary,
  stable-ID byte retention, exact predecessor decoder,
  principal schema, CA/DNS/policy state, and Docker evidence from the integrated
  post-WP04 tree rather than from the old main baseline.
- ADR 0079's accepted one-time copy contract does not constrain hostname
  selection. WP 05 depends only on its no-lineage/no-provenance and no-copy-of-
  attachment guarantees: migration must never consult `copied_from`, and a
  copied Manifest or Runtime receives no route or grant.
- WP 03 Runtime Retirement is accepted and does not constrain hostname
  selection. Its protection graph remains separate: Host Loopback transient
  authority is not Runtime history, snapshot, execution material, reference
  production, or usage evidence. WP 05 must not alter WP 03 command contracts.
- Context capability/source-access packets remain Boundary implementation
  evidence only; their public Context vocabulary is superseded by ADR 0079 and
  the current product contract.
  Authentication narrowing and policy compaction do not own Host Loopback
  naming, and learned permission remains separate from Manifest state.
- ADR 0074 is an explicit non-overlap: Workspace-to-host Host Loopback becomes
  `host.tobari.internal`, while host-to-Workspace service exposure stays an
  exact random numeric `127.0.0.1:<port>` authority.
- Any concurrent Gateway DNS, CA, authority normalization, policy schema, or
  generated-site change must be rebased and reviewed together. A suffix-wide
  `.internal` route or public `gateway.*` concept would conflict with this
  packet.
- WP 08 owns the Catalog-wide nested reference/output mechanism consumed by
  WP03 and WP04; WP05 creates no parallel validator. WP04's completed build
  profile fixes the actual standard Runtime inventory used by WP05's release
  compatibility floor.
- ADR 0079's current-versus-previous Manifest revision retention and Git fallback
  slice do not belong in Host Loopback identity. This packet must not create a
  dependency on either choice. Its child-session behavior remains an upstream
  observation; WP05's stopped-cluster/zero-live-attachment cleanup condition is
  fixed.

## Security and public-boundary notes

- No external content is copied into production. This packet links official
  sources and records interpretations in English, the repository's configured
  documentation locale.
- No credentials, live endpoints, real personal data, private organization
  identifiers, or external provider responses are involved.
- The state in scope is non-secret capability data plus secret relay tokens in
  existing owner-only infrastructure files. Migration must never render or log
  relay coordinates.
- The public URL and migration fault are untrusted-output-safe constants. The
  old host must not become an ordinary external candidate after retirement.
