# Work Plan: Give physical-host loopback an honest private authority

- Status: Fix
- Decision: Accepted by the Product Owner on 2026-08-23; implementation is not
  authorized by this status

## Decision

Replace the sole routable Host Loopback authority
`host.tobari.test` with `host.tobari.internal` before the first public V1.
Keep the capability plain HTTP, exact, attachment-scoped, Workspace-wide, and
trusted-host-reviewed. Do not support a routable old-name alias or redirect.
Reserve the retired name throughout V1 as a terminal, non-learnable diagnostic
route that performs no policy mutation or external/relay work. Removing that
guard requires a separate ADR and negative safety evidence.

This decision changes one public constant, not the capability's owner, scope,
lifetime, mutability, effect dimensions, or trust boundary.

[ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
and the current theses/product/architecture/security contracts are the durable
upstream identity and migration authority, promoted by commits `07535a9` and
`428812f`. Final V1 uses Workspace Manifest/Manifest,
`workspace_manifest_id`, and `workspace_id`; no Context, `context_id`,
`project_id`, or `instance_id` alias is accepted. WP 05 enters implementation
only after the unchanged completed promotion -> WP08 -> WP03 -> WP04 sequence,
the post-WP04 contract is re-observed, and this packet is reconciled. Hostname
selection remains this packet's responsibility and is not derived from the
Manifest noun.

## Domain model and invariants

### Canonical authority

- Define scheme `http`, one canonical exact hostname
  `host.tobari.internal`, and authority template
  `host.tobari.internal:{port}`.
- The hostname remains a required value in route, denial, candidate, review,
  grant, audit, and capability projection. It is not independently selectable.
- Bind the final-V1 route identity to WorkspaceManifestID, WorkspaceID,
  Attachment Epoch, and canonical hostname. Continue binding candidate and
  grant identity to those trusted stable IDs and the exact host/effect.
- Authority exists only when the exact hostname, port, method/path, trusted
  WorkspaceManifestID/WorkspaceID principal, live Attachment Epoch, trusted-
  host grant, and freshly revalidated route all agree. No subset grants access.
- Match ASCII-lowercase canonical host after the existing bounded HTTP
  authority parser. Do not accept a trailing-dot variant, Unicode lookalike,
  alternate case as a separate policy identity, wildcard, suffix, CNAME, IP
  literal, or search-domain expansion. Any normalization retained by the HTTP
  parser must produce exactly one documented canonical value before policy.
- Keep the exact port range 1024-65535, physical-host target `127.0.0.1`, HTTP
  method/path decision, attachment lifetime, and Workspace audience.
- Preserve the new exact Host header through the relay to `127.0.0.1`; route,
  grant, policy, and audit retain that same identity. Do not add a compatibility
  rewrite to a loopback IP or another host.

### Workspace Manifest boundary and attachment lifetime

- Host Loopback route and Attachment Grant are attachment-owned authority, not
  fields or activation slices of Workspace Manifest desired, last-applied,
  observed, or failure state.
- Publishing an immutable Manifest revision changes desired state only. It
  neither mutates, widens, revokes, nor republishes an active exact grant.
- A grant is usable only with its original WorkspaceManifestID, WorkspaceID,
  Attachment Epoch, host, port, method, and path. Any new Attachment Epoch
  starts with no inherited Host Loopback grant.
- Project root remains a trusted canonical value in the Workspace record and
  may be safe display evidence, but it is not `project_id` or an independent
  Host Loopback authority selector.
- ADR 0079 still leaves open whether a new child session may start with new
  session defaults while an entry adoption is blocked. This packet does not
  decide that behavior. Under either choice, session-default activation cannot
  change Host Loopback route/grant identity or authority.
- A recommended fresh-state Manifest draft has no stable authority ID and
  cannot create a Host Loopback route or grant. Status, list, show, and doctor
  remain read-only observations and cannot reconcile this attachment branch.
  Only explicit Workspace entry may establish it.

### Retired authority

- Recognize exact legacy `host.tobari.test` only as
  `retired_host_loopback_authority` before ordinary external classification.
- Return one fixed secret-free denial that tells the user to use scheme `http`
  with authority `host.tobari.internal:{port}` from a fresh interactive
  attachment and review the new request.
- Mark it non-learnable and exclude it from policy candidates and review
  items. It must create no route/grant and perform zero relay, upstream,
  external DNS, Broker, redirect, or retry work.
- Do not translate an old candidate, review item, grant, route, audit ID, or
  denial into the new authority. The user makes a new request and reviews the
  new exact effect.
- Keep the retired sentinel for the entire V1 lifetime. Removing it requires a
  separate ADR and negative safety evidence; making it routable requires a new
  security and compatibility decision.

### Private ceiling

- Preserve `host_loopback` as the sole closed exception to ordinary private
  destination rejection.
- A `.internal` suffix, synthetic DNS answer, private IP, or
  `host.docker.internal` transport never selects the exception. Selection
  requires exact canonical Host plus the live principal-bound route.
- Add negative canaries for sibling names, including
  `gateway.tobari.internal`, `foo.tobari.internal`, `host.internal`, and
  `host.tobari.internal.example.com`.
- The public-boundary guard exception is exact string equality with the one
  product-owned synthetic hostname. It is not a full-URI exception and cannot
  admit `*.tobari.internal`, another `.internal` name, or a sibling host.

## Mandatory implementation-entry gate after WP 04

No WP 05 production, test, durable-documentation, Catalog, schema, generated,
or release change may begin until all of the following evidence exists:

1. Confirm the durable Workspace Manifest/copy-contract promotion evidence
   (`07535a9`, `428812f`), then completion evidence in the unchanged order:
   WP08 Catalog/domain output conformance, WP03 Runtime retirement, and WP04
   build-profile contract.
   Record the resulting exact integrated `HEAD` and a clean or fully explained
   `git status`; protect every unrelated change.
2. Reread `AGENTS.md`, `docs/00_theses.md` through `docs/04_harness.md`, the
   relevant `docs/07` through `docs/09`, ADR 0079, and the promoted durable
   contracts plus WP08/03/04 completion evidence in governing order.
3. Reinspect README, the actual Catalog and human/agent help, Domain Model and
   Host Loopback domain/application/infrastructure slices, Gateway synthetic
   DNS/TLS routing, OPA data/policy, CA ownership, principal and route/grant
   stores, migration code, schema fixtures, tests, and recent commits.
4. Run only bounded read-only help and fresh temporary-state observations that
   cannot mutate shared installation or repository state. Record exact binary
   source identity; discard stale-binary output as proof.
5. Compare the final implementation with every identity, lifetime, migration,
   schema, CA/DNS/policy, copy-isolation, and Runtime-separation assumption in
   this packet. Classify facts, evaluations, unknowns, and inferences. Update
   the packet or obtain an owner decision for every mismatch before code.

The gate fails closed while any predecessor is incomplete, overlapping files
remain contractually unstable, the integrated revision cannot be identified,
the worktree overlap is unexplained, or relevant baseline gates fail. WP 05
does not patch predecessor production files in advance to make this gate pass.

## Layer ownership and implementation dependency order

1. **Complete the fixed upstream sequence.** The Workspace Manifest/copy
   contract promotion is complete; WP08 consumes its Catalog/domain contracts,
   WP03 consumes WP08, WP04 consumes
   the completed Runtime/output/build-profile contracts, and only then may WP05
   pass the mandatory entry gate. Freeze actual WorkspaceManifestID/WorkspaceID,
   Catalog, CA/DNS/policy, standard Runtime, migration, and schema shapes from
   evidence. Do not implement an intermediate identity or schema.
2. **Reconcile accepted adjacent decisions.** Confirm ADR 0079 copies no
   attachment or lineage state, WP03 treats Host Loopback as outside Runtime lifecycle
   authority, WP08 remains the Catalog-wide reference owner, and WP04 supplies
   the actual standard Runtime compatibility inventory.
3. **Durable Host Loopback decision.** Revise or supersede ADR 0049 and
   propagate the hostname reason through theses, product, architecture,
   security, harness, and readiness contracts. Record that ICANN private-use
   reservation is not IETF Special-Use resolver semantics.
4. **Final-V1 domain vocabulary.** Replace the canonical hostname, bind route
   and grant identity to WorkspaceManifestID/WorkspaceID/Attachment Epoch,
   keep attachment state outside Manifest revisions, define the terminal
   retired-authority classification, and prove all exact invariants without
   I/O.
5. **Application interpretation.** Ensure candidate/review/report use cases
   surface only new Host Loopback items; old items are non-learnable recovery
   diagnostics and never become persistent external rules.
6. **Atomic predecessor migration.** Existing `migrate apply` revalidates
   cluster stopped, zero live attachment, exact owner, exact predecessor schema,
   and exact old hostname; it retains legacy UUID bytes under ADR 0079, migrates
   durable principal/policy/audit identity, and atomically replaces only exact
   transient route/grant registries with empty schema-V2 registries. It
   translates no attachment state and adds no maintenance command.
7. **Infrastructure, Gateway, and OPA.** Emit only the final schema-V2
   route/grant shape, switch the exact request branch,
   add the retired terminal guard before ordinary routing, preserve Host/SNI
   checks, policy order, owner-only atomic files, relay secrecy, route-first
   teardown, and private `host.docker.internal` bridging.
8. **CLI and presentation.** Update Catalog-derived descriptions, human text,
   JSON fixtures, capability environment, README, agent recovery, and
   documentation generation sources. Remove old Context/project semantic
   fields and add no hostname flag or command.
9. **Reconciliation and integration.** Require post-migration `cluster up`,
   then fresh Workspace entry/attachment/review. Exercise retirement denial,
   CA stability, resolver/client compatibility, and clean Docker runtime
   behavior before release artifacts are prepared.

Domain remains dependency-free; application owns task interpretation;
infrastructure owns files, sockets, DNS transport, CA material, and Docker
names; CLI remains the composition and presentation root.

## Public, human, JSON, and state compatibility

### Public CLI and human presentation

- Host Loopback adds no command or hostname selector. Its existing review
  workflow keeps its role, effect, and reference flow, while ADR 0079 and the
  current product contract replace global `context`/`--context` vocabulary with
  `manifest`/`--manifest` and no alias.
- `review permissions` displays the new exact host for fresh requests. An old-host
  attempt is absent from the review queue and appears only in bounded denial or
  diagnostic output with the fixed recovery action.
- README, shell examples, Skills/agent guidance, `help` descriptions, recovery
  text, architecture site source, and readiness scenarios use the new URL.
- Historical ADR text may retain the old name only where explicitly labeled as
  superseded history. Current guidance must never show it as executable.

### JSON and capability schemas

- Join ADR 0079's pre-public contract reset. Public/helper-visible
  `TOBARI_CAPABILITIES_JSON` stays schema V1 and changes only the hostname value.
  It has no schema-2 reader, dual old/new value, or alias field.
- Owner-only transient Host Loopback route and attachment-grant registries use
  schema V2. Their new route/grant opaque IDs make every predecessor reference
  stale. This internal registry version is not the public capability version.
- The exact current-main predecessor may be recognized only inside the one
  approved `migrate apply` path. Ordinary final-V1 readers accept only the new
  hostname and WorkspaceManifestID/WorkspaceID fields.
- Route ID V2 directly binds WorkspaceManifestID, WorkspaceID, Attachment
  Epoch, and canonical hostname. Attachment Grant and policy candidate IDs bind
  the same trusted identity plus their exact effect. Do not add an alias array,
  nullable legacy field, Manifest revision, Project-root-derived ID, or an
  independent AuthorityRevision concept.
- Candidate/denial/report envelope semantic fields replace legacy Context and
  project identity with display `workspace_manifest`, authority
  `workspace_manifest_id`/`workspace_id`, and subordinate diagnostic
  `project_root`; exact `host` values and opaque IDs also change. No `context`,
  `context_id`, `project_id`, or `instance_id` output fallback remains.
- Fresh-state draft and absent-Manifest variants contain no authority ID and
  cannot be accepted by Host Loopback action or route construction. Attachment
  observation in status remains distinct from Manifest desired/applied state.
- The retired diagnostic uses an existing bounded denial envelope if it can
  represent `learnable: false` and the recovery exactly. If it cannot, revise
  the governing output contract before adding a new field or fault variant;
  do not overload `external` or `host_loopback` identity.

### Runtime-state cutover

- Do not support a rolling hostname migration with a live attachment or mixed
  old/new principal schemas. `migrate apply` must revalidate cluster stopped
  and zero live attachment immediately before cleanup/publication.
- Order the one pre-public cutover as: stop the cluster and prove zero live
  attachment;
  run ADR 0079's exact predecessor migration; retain Context UUID bytes as
  WorkspaceManifestID and ProjectInstance UUID bytes as WorkspaceID; validate
  exact owner/schema/old-host transient registries; atomically replace them
  with empty schema-V2 registries without translation; optionally remove only
  an observed exact owner-verified old-host leaf cache entry; publish final-V1
  state; run `cluster up`; then enter a Workspace and create a fresh Attachment
  Epoch.
- Host Loopback route/grant state is not predecessor migration input,
  AppliedEntry evidence, Manifest desired/applied state, or learned permission.
  Do not translate routes, grants, relay tokens, or candidates. A migrated
  denial/audit record may retain historical old host text, but candidate
  derivation must classify it as retired and non-learnable.
- Exact predecessor inspection and transient-state replacement belong only to
  `migrate apply`, not to status, list, show, doctor, attachment startup,
  `cluster up`, or an independent Host Loopback compatibility reader.
- If old Host Loopback state is live, malformed, unknown, symlinked,
  wrong-owner, or mixed-version, fail before migration publication and leave
  durable predecessor state untouched for exact recovery.
- Re-entry creates a fresh Attachment Epoch, final-V1 route and capability
  projection, and new candidate/grant references. Manifest revision retention
  and Git fallback slice choices do not participate in this identity.

### One-time copy and WP 03 Runtime lifecycle separation

- `manifest create --copy-from` publishes a fresh WorkspaceManifestID and
  copies no Workspace, attachment, route, grant, learned permission,
  authentication, AppliedEntry, failure, observed, or current-selection state.
  It performs no reconciliation. A copied declaration therefore has no Host
  Loopback migration relationship until a separately bound Workspace enters
  and creates a fresh Attachment Epoch.
- `runtime create --copy-source-from` copies only editable Runtime source into
  a fresh RuntimeID with empty history. It copies no Manifest binding,
  Workspace state, attachment, route, or grant and performs no reconciliation.
- Persisted provenance, lineage, and `copied_from` do not exist. Hostname,
  principal, route, grant, and predecessor migration must never infer an
  authority relationship from source-name equality or historical copy input.
- WP 03 owns Runtime deletion, prune, restore, build selection, retained
  history/snapshots, protection graph, and usage evidence. Host Loopback
  transient state is neither protected Runtime material nor `last_used`
  evidence. WP 05 adds no command, Runtime reference, protection edge, or
  Runtime-specific nested-reference validation.

### Rollback

- Before publication, rollback follows ADR 0079: stop the new cluster, restore its
  exact owner-only predecessor backup with the old binary, and recreate all
  interactive attachments. No final-V1 route/grant state is translated back.
- After a release containing the new authority, rollback must not make the old
  name routable without an explicit compatibility/security decision. A
  terminal diagnostic is safe to retain.

## DNS and runtime compatibility plan

Test the canonical base and supported host/runtime matrix using only synthetic
services and local fixtures:

- the standard Runtime's actual libc/getaddrinfo path through curl, Python,
  applicable Go pure/cgo paths, and Node `dns.lookup`/`dns.resolve` plus HTTP;
- Java and a representative browser only as optional observations unless the
  completed WP04 standard Runtime actually contains them;
- exact A resolution of `host.tobari.internal` to the synthetic Gateway
  address, zero external DNS lookup, TTL 0 behavior, no AAAA-based bypass, and
  exact `host.tobari.internal:{port}` Host preservation at the host service;
- negative canaries showing `host.tobari.localhost` resolves or is treated as
  Workspace loopback rather than the Gateway, and `host.tobari.invalid` may
  fail before DNS;
- `host.tobari.test` reaches only the terminal retired guard when the request
  traverses Tobari, never ordinary external routing;
- Docker Desktop and supported Colima behavior. Podman and OrbStack remain
  research comparisons unless the product separately adds those runtimes.

The acceptance target is stable HTTP-client behavior, not identical raw DNS
API output. A direct resolver that intentionally bypasses the system resolver
must still be contained by Tobari's network boundary and must not acquire an
unreviewed path.

## TLS, SNI, and CA plan

- Keep Host Loopback `http` only. Scheme `https` with authority
  `host.tobari.internal:*`, and TLS with either new or retired SNI, must fail
  at the terminal Host Loopback classification before leaf generation and
  without opening the relay.
- Preserve existing authority normalization: when TLS is in scope elsewhere,
  Host and SNI must agree before policy. Do not add Host Loopback-specific SNI
  aliases.
- Do not rotate the shared Gateway root CA for this rename. Record its public
  certificate digest before and after the migration journey and require
  equality.
- Inspect per-host leaf caching. If and only if an exact old-host leaf exists,
  treat it as non-authoritative cache material and let `migrate apply` remove
  only its owner-verified exact entry. Never perform broad cache deletion or
  rotate the root/trust bundle.
- A future HTTPS Host Loopback capability needs a separate packet and ADR
  covering private-CA issuance, SNI, termination versus re-encryption, physical
  service identity, client trust, secure-context claims, rotation, revocation,
  and HTTP/2. This rename provides no implied HTTPS compatibility.

## Alternatives

### Keep `host.tobari.test`

Lowest code churn and already collision-free. Rejected because `.test` is a
testing namespace and the public outcome is routine reviewed host access. The
cost of correcting the mental model rises sharply after first publication.

### Use `host.tobari.localhost`

Memorable and collision-free. Rejected because standards and browsers may
force all `.localhost` descendants to the caller's loopback and confer
potentially trustworthy-origin behavior. That points at the Workspace, not the
physical host, and can bypass synthetic DNS.

### Use `host.tobari.invalid`

Collision-free and unmistakably non-global. Rejected because standards invite
clients/resolvers to fail before Tobari can project a positive route. A
reachable capability should not claim invalidity.

### Use `host.tobari`

Shorter. Rejected because `.tobari` has no root reservation or Special-Use
status, and search paths/future delegation could collide with the private
meaning.

### Use `gateway.tobari.internal`

Technically routable through the same synthetic DNS. Rejected because it names
the policy/transport mechanism instead of the physical-host destination and
would encourage users to treat Gateway identity as service authority.

### Hide the name in projection only

Rejected because users and HTTP clients need a concrete URL authority, while
policy and Host forwarding need a stable exact host. Hiding it from docs would
force agents to inspect ambient JSON or transport internals for routine use.

### Support both old and new names

Rejected because a normalized alias lets one grant authorize two authorities,
while parallel routes/grants duplicate attachment authority. A redirect adds
automatic client retry to the security path. A terminal diagnostic supplies
migration guidance without expanding access.

## Test strategy

### Domain and application

- Final-V1 URL and public schema-V1 projection, transient registry schema V2,
  exact host validation,
  WorkspaceManifestID/WorkspaceID/Attachment-Epoch route and grant identity,
  exact-predecessor isolation, and hostile/near-match hostname cases.
- Candidate, denial, review, grant, policy-rule, and opaque-reference tests
  prove new-host identity, stale-old-reference rejection, and no translation.
- Desired Manifest mutation tests prove a new revision does not touch, widen,
  inherit, or republish an active grant; a new Attachment Epoch begins with no
  prior grant.
- The old authority is non-learnable and never enters persistent exact allow or
  deny state. Ordinary external and Host Loopback authority remain mutually
  unusable.
- Empty-scope and absent-versus-empty report semantics remain unchanged.

### Infrastructure, Gateway, and OPA

- stopped-cluster/zero-live-attachment enforcement, exact owner/schema/old-host
  matching, predecessor identity-byte retention, atomic empty registry-V2
  replacement, transient route/grant non-translation,
  mixed/corrupt state rejection, post-`cluster up` fresh attachment,
  concurrent attachments, borrower lifetime, crash reconciliation, and
  route-first teardown.
- New exact authority reaches OPA before relay; allow opens one authenticated
  relay to the reviewed `127.0.0.1` port; original Host remains exact.
- Old, sibling, wildcard, trailing-dot, malformed, HTTPS/SNI-mismatch, stale,
  unreviewed, and post-detach requests produce zero external-DNS/upstream/
  Broker/relay/retry work. Internal synthetic DNS remains available to reach
  the terminal classifier.
- Synthetic DNS remains non-recursive and secret-free. Global/private DNS
  answers, search suffixes, or CNAMEs cannot select Host Loopback.
- CA digest stability, no pre-rejection leaf generation, exact conditional
  old-leaf deletion, cache non-authority, and no TLS relay are explicit canaries.

### CLI, docs, and readiness

- Catalog/schema/help tests prove no new command or selector and keep exact
  recovery argv/path metadata valid.
- Human and JSON golden fixtures show routine label Manifest,
  `workspace_manifest`, `workspace_manifest_id`, `workspace_id`, subordinate
  `project_root`, the new host, and attachment lifetime, with no `context`,
  `context_id`, `project_id`, or `instance_id` alias.
- Root agent help remains within its 512-byte per-entry budget; exact command
  help owns detailed policy fields.
- Public-boundary scans permit the old name only in the superseded ADR,
  migration note, retirement guard, and negative fixtures.
- Revise the public-boundary governing contract and scanner narrowly enough to
  permit a URI only when its host is the exact product-owned synthetic
  `host.tobari.internal`; keep other private hostnames and sibling `.internal`
  names denied. Add positive and negative mechanical tests before placing the
  complete URI literal in current public documentation.
- Agent-readiness replay distinguishes Workspace `localhost`, physical-host
  `host.tobari.internal`, and opposite-direction numeric exposure with zero
  source inspection or provider-notation decoding.
- Migration readiness reuses ADR 0079's synthetic Docker evidence and selected
  cluster/attachment-precondition journey. This packet additionally requires
  no live Host Loopback route owner and adds fresh epoch/review assertions
  rather than defining a second migration.

## Documentation and release propagation

- Revise `docs/00_theses.md` through `docs/04_harness.md`, ADR 0049, README,
  `docs/09_agent_readiness_validation.md`, applicable security/threat-model
  material, capability/schema ledgers, and Catalog-derived generated-site
  sources.
- Apply ADR 0079's Workspace Manifest/Manifest, `manifest`/`--manifest`,
  `workspace_manifest_id`, and `workspace_id` vocabulary atomically; do not
  publish Host Loopback output with either old public nouns or dual fields.
- `docs/07_authentication.md` and `docs/08_external_api_contracts.md` should
  change only if their no-pre-policy-resolution or external-I/O wording names
  the old authority; do not manufacture an auth/API consequence.
- Record official-source URLs and check dates in the durable ADR, but do not
  make a mutable Internet-Draft the governing security guarantee. The ICANN
  permanent root reservation and Tobari's exact enforcement are sufficient.
- Coordinate with first-public-release packets before generating release
  files. This packet itself changes no generated or release artifact.

## Gates

Focused suites may run during implementation. Final completion requires:

```sh
task check
task security
task public:check
task release:check
```

The synthetic resolver/client matrix and clean Docker integration journey are
additional required evidence. The canonical completion gate remains `task
check`; public and release gates are additionally required because this name is
a user-visible V1 contract.

## Implementation-time observation gates

1. Record the exact completed post-WP04 revision and final
   WorkspaceManifestID/WorkspaceID, Catalog, CA/DNS/policy, migration,
   route/grant, standard Runtime, and cache schemas. Any incompatible fact is
   `WP05_BLOCKED`, not permission to invent a compatibility layer.
2. Inventory exact standard Runtime curl/libc, Python, Go, and Node versions and
   determine which Go pure/cgo modes are applicable. Java/browser remain
   optional unless that inventory includes them.
3. Observe whether an old-host leaf cache entry exists and its exact owner/
   schema representation. Absence means no cache mutation; presence permits
   only the fixed exact `migrate apply` removal.
4. Record sufficient Docker evidence for stopped cluster, zero live attachment,
   exact transient owner/schema/old-host matching, atomic registry-V2
   replacement, and fresh post-`cluster up` attachment.
5. Consume ADR 0079's final child-session behavior without changing the fixed rule: no
   session or Manifest revision may expand or inherit an Attachment Grant.
