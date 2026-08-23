# Work Goal: Give physical-host loopback an honest private authority

- Status: Accepted
- Planning state: Fix
- Retention: temporary
- Retention reason: None
- Governing contract: [theses](../../00_theses.md),
  [product](../../01_product_contract.md),
  [architecture](../../02_architecture.md),
  [security](../../03_security_model.md), [harness](../../04_harness.md),
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md),
  and [agent readiness](../../09_agent_readiness_validation.md)
- Review/delete trigger: Delete after the authority rename is implemented, durable conclusions are promoted, and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Accepted/Fix before the first public V1 release
- Related ADRs: ADR 0049, ADR 0074, and
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
- Related work: [Catalog / Domain Output Conformance](../catalog-domain-output-conformance/goal.md),
  [Runtime Retirement](../runtime-retirement/goal.md),
  [Build Profile Contract](../build-profile-contract/goal.md),
  [First public V1 core](../first-public-release-core/goal.md), and
  [first public V1 artifacts](../first-public-release-artifacts/goal.md)
- Integration state: Product Owner accepted and fixed this packet on
  2026-08-23. The first Workspace Manifest/copy-contract stage has been
  promoted by `07535a9` and `428812f`; production implementation remains
  unauthorized until the unchanged sequence continues through WP08 -> WP03 ->
  WP04 -> WP05 and the WP05 implementation-entry observation gate passes.

## Outcome

Every interactive Workspace reaches plain HTTP on the physical host's IPv4
loopback through scheme `http` and exact authority template
`host.tobari.internal:{port}`. The name says that the destination is a
private Tobari-projected host capability, does not claim testing behavior, and
does not acquire `.localhost` loopback semantics. The existing attachment
epoch, trusted-host review, exact method/path/port grant, Gateway policy, and
authenticated relay remain the authority; the DNS suffix itself grants
nothing. Route and grant authority uses the attachment's trusted
`workspace_manifest_id` and `workspace_id`, never a public Context/project
alias or a Manifest revision.

## Why now

The current `host.tobari.test` name is globally collision-resistant and works
with Tobari's synthetic DNS, but RFC 2606 assigns `.test` to testing. Tobari
uses it for a real, reviewed development capability. ICANN has since
permanently reserved `.internal` from root delegation for private-use
applications, while `.internal` is not an IANA Special-Use name with mandated
resolver short-circuit behavior. Choosing the long-lived name before the first
public release avoids teaching agents and users a testing metaphor or carrying
a routable compatibility alias across an authority boundary.

## Non-goals

- Adding HTTPS, raw TCP, UDP, privileged ports, LAN reachability, host service
  discovery, or host Docker control.
- Changing what `localhost` means inside a Workspace; it continues to mean the
  Workspace's own loopback.
- Changing opposite-direction Workspace service exposure from its reviewed
  numeric host authority in ADR 0074.
- Exposing `host.docker.internal`, `host-gateway`, the synthetic Gateway IP,
  relay tokens, or Gateway topology as public or policy identity.
- Treating `.internal`, any `.internal` suffix, or a successful DNS answer as
  permission, private-destination eligibility, or trusted provenance.
- Adding a routable alias, redirect, CNAME, wildcard, search suffix, or learned
  rule that accepts `host.tobari.test` after the cutover.
- Rotating the shared Gateway root CA solely because the HTTP authority name
  changes.
- Adding Host Loopback or Attachment Grant state to Workspace Manifest desired,
  last-applied, observed, or failure state.
- Letting a Workspace Manifest revision publication mutate, widen, inherit, or
  reactivate an Attachment Grant.
- Deciding ADR 0079's open Manifest revision retention, Git fallback slice, child
  session default behavior, or the general Docker evidence sufficient for the
  upper migration. WP 05's own cutover precondition is fixed as cluster stopped
  plus zero live attachment.
- Preempting or duplicating the durable Workspace Manifest implementation in
  Workspace Manifest, Catalog, state, schema, migration, CA/DNS, or policy
  contracts.
- Changing ADR 0079's accepted one-time copy commands or adding copy provenance,
  lineage, `copied_from`, inheritance, reconciliation, or attachment copying.
- Changing WP 03's accepted Runtime delete/prune/restore/build commands,
  protection graph, history retention, `last_used` semantics, or Catalog-wide
  nested-reference invariant.

## Acceptance criteria

- [ ] `host.tobari.internal` is the sole routable Host Loopback presentation
      authority and the exact policy, route, grant, denial, review, and audit
      identity; all matches are exact, never suffix-based.
- [ ] One fresh interactive attachment projects scheme `http` with authority
      `host.tobari.internal:{port}`, while `localhost` still names the
      Workspace and `host.docker.internal` remains infrastructure-only.
- [ ] Throughout V1, a request to the retired `host.tobari.test` authority
      produces a fixed,
      secret-free, non-learnable retirement fault and performs zero external-
      DNS/upstream/Broker/relay/retry work or policy-grant mutation.
      Removing this terminal guard requires a separate ADR and negative safety
      evidence.
- [ ] Transient route and attachment-grant registries use schema V2, include
      only the new authority, and issue new opaque route/grant IDs; every old
      reference is stale. Route ID V2 directly binds the exact hostname and no
      independent AuthorityRevision concept exists.
- [ ] Public/helper-visible `TOBARI_CAPABILITIES_JSON` remains schema V1 and
      replaces only the hostname value. It exposes neither a dual old/new value
      nor a schema-2 compatibility surface; public capability schema and
      internal transient-registry schema are tested as separate contracts.
- [ ] Final-V1 route, grant, denial, review, audit, and principal identity uses
      routine label Manifest plus `workspace_manifest`,
      `workspace_manifest_id`, `workspace_id`, and subordinate `project_root`;
      `context`, `context_id`, `project_id`, and `instance_id` receive no public
      or internal semantic fallback.
- [ ] Host Loopback and Attachment Grant remain attachment-owned authority
      outside Workspace Manifest desired/applied state. Publishing a new
      Manifest revision neither changes an active exact grant nor carries it
      into a new Attachment Epoch.
- [ ] A presentation-only recommended Manifest draft, an absent Manifest
      catalog, and every read-only status/list/show/doctor path create no route,
      grant, principal, or persisted Host Loopback authority. Only explicit
      Workspace entry may establish the attachment branch.
- [ ] Existing `migrate apply` is the sole cleanup/cutover owner. After
      revalidating cluster stopped, zero live attachment, exact owner, exact
      predecessor schema, and exact old hostname, it atomically replaces only
      matching transient route/grant registries with empty schema-V2
      registries. No new maintenance command, translation, implicit reader
      cleanup, or cleanup by attachment startup, status/doctor, or `cluster up`
      exists.
- [ ] No WP 05 production implementation starts before the already promoted
      Workspace Manifest/copy-contract stage, WP08, WP03, and WP04 complete in
      that unchanged order and an implementation-entry review
      records their actual integrated `HEAD` and working tree, rereads the
      promoted contracts, and re-observes final WorkspaceManifestID/
      WorkspaceID, CA/DNS/policy, Catalog, migration, and route/grant shapes.
      Any mismatch blocks WP05 or updates this packet before code.
- [ ] WP 05 migration never consults nonexistent provenance or `copied_from`
      state, copies attachment authority, or treats an ADR 0079 copy action as
      reconciliation. Verification of copy behavior remains governed by ADR
      0079 and its durable contract tests.
- [ ] WP 05 keeps Runtime retirement as a separate authority graph. It treats
      Host Loopback route/grant state as neither Runtime revision/history/
      material nor `last_used` evidence, and it neither modifies WP 03 commands
      nor creates a Runtime-specific reference validator. Verification of the
      Runtime lifecycle remains owned by WP 03.
- [ ] Human help, README examples, agent guidance, structured JSON output,
      capability projection, generated documentation sources, and recovery
      text name only the new authority, except an explicit migration note and
      negative retirement tests.
- [ ] HTTP-only behavior, Host/SNI consistency, the exact private-destination
      ceiling exception, and the existing post-policy authenticated relay are
      unchanged. Denied/retired HTTPS is terminally classified before leaf
      certificate generation. No new trust boundary or public concept is
      introduced.
- [ ] Gateway forwards the exact new Host authority unchanged to physical-host
      `127.0.0.1`; route, grant, policy, and audit use the same identity. No
      compatibility Host rewrite is introduced.
- [ ] The release compatibility matrix covers the standard Runtime's actual
      libc/getaddrinfo curl path, Python, applicable Go pure/cgo paths, and Node
      DNS plus HTTP. Java/browser are optional observations unless present in
      the standard Runtime. Every gated path proves Tobari synthetic DNS, zero
      external lookup, and exact Host preservation.
- [ ] The existing shared Gateway CA remains byte-identical across the rename
      scenario; TLS to both new and retired names remains unavailable with no
      host relay or leaf generation. If an old-host leaf cache entry is
      observed, only `migrate apply` may remove its exact owner-verified entry;
      broad cache deletion and root CA rotation remain forbidden.
- [ ] `task check`, `task security`, `task public:check`,
      `task release:check`, and the relevant clean Docker integration journey
      pass on the integrated implementation.

## Governing documents

- Thesis: [docs/00_theses.md](../../00_theses.md), especially narrow Host Loopback authority,
  synthetic DNS, attachment lifetime, and public mental-model claims
- Product contract section: [docs/01_product_contract.md](../../01_product_contract.md), Host Loopback,
  authority lifetimes, Workspace capability projection, and compatibility
- Architecture or security invariant: [docs/02_architecture.md](../../02_architecture.md) Host Loopback
  order and relay; [docs/03_security_model.md](../../03_security_model.md) exact separate policy branch,
  no pre-policy external I/O, private ceiling, and Gateway CA boundary
- Harness and readiness: [docs/04_harness.md](../../04_harness.md) Host Loopback claim and
  [docs/09_agent_readiness_validation.md](../../09_agent_readiness_validation.md) authority-lifetime scenario
- Existing ADR: ADR 0049 defines the current name and HTTP attachment lease;
  ADR 0074 proves the opposite-direction loopback authority must stay separate
- Accepted upper-level decision: [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  defines Workspace Manifest/Workspace identity, desired/applied separation,
  attachment exclusion, the pre-public migration boundary, and target-specific
  one-time copy with no lineage or attachment copying. Commits `07535a9` and
  `428812f` are the upstream promotion evidence; their final integrated
  contracts must be re-observed before this packet enters implementation.
- Accepted adjacent decision: WP 03 fixes Runtime retirement and its
  protection graph without owning Host Loopback naming
- Accepted integration order: completed Workspace Manifest/copy-contract
  promotion (`07535a9`, `428812f`), then WP08 Catalog/domain output conformance,
  then WP03 Runtime retirement, then WP04 build-profile contract, then WP05

## Completion definition

The work is complete when acceptance criteria have evidence, ADR 0049 and all
affected durable contracts agree on one exact authority, schema and retirement
behavior are mechanically enforced, required profiles pass, temporary
implementation diagnostics are removed, and this temporary packet is removed
from the final tree. On implementation completion, notify control thread
`01a02c51-885b-7b80-a66f-05850f48ba4d` with
`WP05_IMPLEMENTATION_COMPLETE`; an actual implementation blocker is reported
as `WP05_BLOCKED`. This packet is Accepted/Fix only and does not authorize
implementation.
