# Work Goal: Make `status` the CWD-centric Workspace home

- Status: Accepted
- Decision state: Fixed by Product Owner; production implementation has not started
- Implementation state: Planned only after every upstream implementation
  through WP09 completes and the integrated-HEAD re-baseline gate passes
- Retention: temporary
- Retention reason: None
- Governing contract: Accepted [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and its promoted [thesis](../../00_theses.md), [product](../../01_product_contract.md), [architecture](../../02_architecture.md), [security](../../03_security_model.md), and [harness](../../04_harness.md) consequences; CWD-first lifecycle observation, result-first routine output, and zero-mutation reads
- Review/delete trigger: Delete after the accepted design is promoted, implementation evidence is complete, and the first public V1 contract includes the result
- Successor: None
- Owner: Tobari maintainers
- Target: After WP01+WP02, WP08, WP03, WP04, WP05, WP07, and WP09 production
  implementations complete and their actual integrated HEAD is re-baselined,
  before the first public V1 status schema is frozen
- Related ADRs: ADR 0027 and accepted ADR 0079, which promotes WP01+WP02 and
  supersedes incompatible Context-era decisions
- Depends on, in implementation order: promoted WP01+WP02 contracts in
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and [docs/00](../../00_theses.md) through [docs/04](../../04_harness.md), the
  completed WP08 Catalog/output contracts in [Architecture](../../02_architecture.md)
  and [Harness](../../04_harness.md), completed [ADR 0080 Runtime lifecycle](../../decisions/0080-close-the-managed-runtime-lifecycle.md), completed WP04
  [ADR 0082 build surfaces](../../decisions/0082-release-and-research-build-surfaces.md), completed WP05
  [ADR 0083 Host Loopback authority](../../decisions/0083-name-the-physical-host-loopback-authority.md), WP07
  `../first-use-progress-recovery/`, completed WP09
  [ADR 0074 Service exposure](../../decisions/0074-expose-one-reviewed-workspace-service.md), plus the
  integrated policy-compaction and release contracts

## Outcome

From any directory, one `tobari status` invocation tells the user which
canonical Project root is in scope, which explicit or installation-default
persisted Workspace Manifest—or no-authority recommended draft—is selected,
which durable Workspace is bound to
`(ProjectRoot, WorkspaceManifestID)`, what the Workspace last applied
successfully, whether the bound Runtime revision authority is ready, whether
its exact local execution image is available, what Docker/runtime state is
observed now, what the next explicit
entry would adopt, what the latest bounded reconciliation failure means, what
shared cluster projection is desired/applied/observed, which independent
permission or live-service attention exists, one exact primary Next, and every
separately ordered Attention item that remains relevant.

The central read model is:

```text
selection + desired + last successfully applied + latest attempt
  + observed runtime + Runtime authority/material + migration evidence
  + cluster/session/creation slices + permission/service summaries
  -> orthogonal closed facts + one structured primary Next + ordered Attention
```

The result is one task-owned `StatusHomeSnapshot`. It contains no serialized or
persisted lossy overall/adoption status. It is not a renderer-side join
of public commands. A successful or failed `status` invocation never publishes
a Manifest revision, records an AppliedEntry or failure, reconciles a Workspace,
reconciles the cluster, or creates/deletes Tobari state or Docker resources.

## Higher-level decisions inherited from WP 01

- Public durable resources are exactly Workspace Manifest, Runtime, and
  Workspace. Runtime revision and Project root are subordinate concepts.
- Public vocabulary is `Workspace Manifest`; routine label is `Manifest`; CLI
  namespace/selector are `manifest` and `--manifest`; schema identity is
  `workspace_manifest_id`. Final V1 has no Context command, flag, field, or
  alias.
- Workspace Manifest is a host-owned, CLI-managed, stable-ID, revisioned desired
  declaration. It is not project-owned YAML, generic import, or `apply -f`.
- Every accepted Manifest mutation publishes one complete immutable desired
  revision. Boundary is invariant within one WorkspaceManifestID; changing it
  requires another Manifest.
- Activation is item-specific: cluster projection at explicit `cluster up`,
  entry slice at explicit Workspace entry, session defaults for a new child
  session, and creation defaults only for a new Workspace.
- Workspace is the durable instance bound to `(ProjectRoot,
  WorkspaceManifestID)` and identified only by `workspace_id`.
- Only explicit Workspace entry may reconcile one Workspace; only `cluster up`
  may reconcile shared resources/projection. No resident controller exists.
- Standard authentication, learned permission, and attachment authority remain
  outside Manifest desired/applied state.
- The first-use recommended default is a no-authority draft, never a persisted
  Manifest or synthetic Manifest identity.

## Higher-level decisions inherited from WP 02 and WP 03

- Manifest and Runtime copy are one-time initializers with fresh identities.
  Status stores or displays no base, parent, source, copy provenance, ancestry,
  or lineage. Copying also copies no Workspace, authentication, learned
  permission, attachment, applied/failure/observed, or selection state and
  triggers no reconciliation.
- A copied Manifest is therefore just an independent generation-1 Manifest; a
  copied Runtime is just an independent Runtime with empty revision history.
  `--base` is removed with no alias.
- Runtime revision authority and local execution-material availability are
  independent: `ready` means a valid immutable revision exists; `available`
  means its exact local execution image is present. Desired, applied, observed,
  ready, and available are not interchangeable.
- `last_used` is `unknown` unless a separately approved exact usage receipt
  exists. AppliedEntry time, `reconciled_at`, and container activity are never
  relabelled as last use.
- Whole-Runtime retirement, prune, restore, and build remain separate explicit
  commands. Status performs none of them. It may present a safe Next command,
  but it neither performs an installation-wide prune-protection scan nor
  claims that deletion/prune is safe.
- Human build selection is `review runtimes`; mutation is
  `runtime build --id <runtime-ref>`. No `--name`, omitted-name action
  selection, or old alias is suggested by status.

## Owner, scope, lifetime, mutability, and authority

- **Owner:** the installed CLI owns correlation and interpretation. Manifest,
  Runtime, Workspace, cluster projection, learned-policy, and live attachment
  owners remain authoritative for their own facts.
- **Scope:** nearest canonical Project root from CWD; exact explicit Manifest or
  WP01 installation-level default; selected bound Workspace; all other logical
  Workspaces at exactly
  the same root; selected-Workspace policy/service attention; and the shared
  cluster projection needed for entry.
- **Lifetime:** one invocation snapshot. Desired Manifest revisions, Workspace
  AppliedEntry, observed runtime, reconciliation failure, cluster projection,
  permission evidence, and attachment facts retain their distinct lifetimes.
- **Mutability:** none. `status` observes only. Desired state changes through
  typed Manifest operations, one Workspace reconciles only at explicit entry,
  and shared projection reconciles only at `cluster up`.
- **Authority:** WorkspaceManifestID plus semantic revision digest authorizes
  desired facts; WorkspaceID plus ProjectRoot and WorkspaceManifestID identifies
  the durable instance; RuntimeID plus semantic revision digest identifies the
  selected Runtime; AppliedEntry and bounded failure receipts are trusted
  host-owned evidence; Docker labels are observations that still require exact
  ownership validation. Names, generations, ordinals, paths, image tags,
  headings, order, color, and proximity are never authority.

## Non-goals

- Implementing WP 01 domain types, migration, Manifest mutation, Workspace
  reconciliation, or cluster reconciliation in this packet.
- Adding a fourth durable public resource, a public Project resource, a
  user-managed Workspace name, or a controller/status subresource.
- Retaining Context terminology as an alias, compatibility flag, JSON field,
  command, or routine label.
- An installation dashboard or replacement for `list`, `manifest show`,
  `runtime list`, `cluster status`, `policy candidates`, `policy rules`,
  `service requests`, or `tobari-expose list`.
- Returning permission, rule, request, or exposure references from `status`.
- Inferring native client installation, login, account, or authorization state
  from Workspace-home credentials or provider output.
- Inspecting Docker state for every same-root or installation-wide Workspace.
- Adding refresh/watch, pagination, `--tail`, repair, reconciliation, migration,
  approval, or service mutation behavior to `status`.
- Persisting or presenting `base`, `copied_from`, copy source, parent,
  provenance, ancestry, or lineage for either Manifest or Runtime.
- Running Runtime delete, prune, restore, build, or an exhaustive Runtime
  protection/installation inventory as part of status.
- Exposing a Docker tag, Docker image/container ID, private Runtime snapshot
  path, or inferred `last_used` value in routine status.
- Resolving WP 01's outstanding revision retention, Git slice, attached child
  session, migration Docker evidence, or cluster-stop/no-attachment decisions.

## Acceptance criteria

- [ ] CWD selects the nearest canonical Project root independently of Manifest
      selection; a fresh CWD is an explicit prospective root; all logical
      Workspaces at exactly that root are grouped by stable
      WorkspaceManifestID.
- [ ] CWD/root resolution completes before Manifest selection. Explicit
      `--manifest` selects one exact persisted Manifest for this invocation
      without mutating the installation default, binding, or Workspace.
- [ ] Omitted `--manifest` uses only the installation-level default established
      by `manifest default set --name`. There is no project-local current
      Manifest, `manifest use`, hidden current selector, or inference from an
      existing/only Workspace binding.
- [ ] If no persisted installation default exists, status returns a typed
      no-authority recommended draft with no `workspace_manifest_id` or
      persisted authority claim while still returning every logical same-root
      Workspace as a sibling/alternative. It performs zero Docker/live-owner
      calls and writes nothing.
- [ ] One typed `tobari.status` result retains requested/root/Manifest/Workspace
      scope even when the selected Workspace or same-root sibling collection is
      empty.
- [ ] The selected Workspace read model explicitly separates desired Manifest
      and entry revisions, last successful AppliedEntry, observed runtime and
      attachment facts, and latest bounded failure/unknown evidence.
- [ ] Runtime output separately represents immutable revision authority
      (`ready` or its typed contrary) and exact local execution-material
      availability (`available`, `missing`, `mismatched`, `unknown`, or the
      final WP 03 enum). It never derives one from the other or from desired /
      applied equality.
- [ ] `last_used` is explicitly `unknown` unless an exact usage receipt approved
      outside this packet exists. AppliedEntry, `reconciled_at`, running/stopped
      observation, and Docker timestamps are not inference inputs for it.
- [ ] The snapshot exposes separate closed axes, never one overloaded aggregate:
      `workspace_presence=absent|present`;
      `entry_state=current|pending|blocked_attached|failed|unknown` when present;
      `observed_runtime_state=not_observed|absent|stopped|running|drifted|unknown`;
      `runtime_revision_authority=ready|not_ready|unknown`; and
      `execution_material_availability=available|missing|mismatched|pruned|unknown`,
      aligned to final upstream vocabulary.
- [ ] Invalid/foreign ownership is a command fault, not an observation enum.
      State integrity is emitted only if WP01 supplies a real domain value;
      otherwise contradictory/incomplete authority fails the command. Migrated-
      unverified evidence remains a bounded evidence field, not overall state.
- [ ] `latest_attempt`/failure remains a separate optional typed fact. Entry
      precedence is exact: matching-current-desired final failure → `failed`;
      desired difference plus exact recreation requirement and attachment →
      `blocked_attached`; other desired difference → `pending`; matching desired
      and AppliedEntry plus required consistency evidence → `current`;
      insufficient/changed evidence → `unknown`. An older failure cannot
      override a newer desired identity.
- [ ] The model permits truthful cross-axis combinations, including
      `entry_state=current` with stopped or unavailable execution material and
      `entry_state=pending` with running or available execution material.
- [ ] Human status leads with `Current` and `Next entry`. Desired values are
      never called active, current-applied, or successfully reconciled until
      the matching AppliedEntry and bounded observation prove that claim.
- [ ] Changes confined to cluster, entry, session, or creation activation slices
      appear in their own section and do not falsely make the whole Manifest or
      Workspace pending. Creation-only edits never request entry adoption.
- [ ] Session behavior is consumed exactly from final WP01 implementation. WP06
      introduces no child-session activation rule or inference.
- [ ] If an entry change requires runtime recreation while attached, status
      reports `blocked_attached`; status performs no mutation and never suggests
      that the running Workspace already adopted the desired revision.
- [ ] Cluster projection reports desired aggregate identity, last successful
      applied identity, current bounded observation, and latest failure/unknown
      independently from the selected Manifest revision. Only `cluster up` is a
      reconciliation action.
- [ ] Source access is presented as immutable Manifest Boundary authority;
      native login as Workspace-home owner/lifetime with validity
      `not_observed`; learned permission and attachment authority remain
      separate status sections.
- [ ] Pending permissions retain a fixed bounded window and unparsed count;
      learned rules retain stored exhaustive scope plus projection validity;
      pending services and active exposures retain fresh attachment scope.
      Status emits counts, not opaque action references.
- [ ] Status emits no copy provenance or lineage and never suggests `--base`,
      `runtime build --name`, or omitted-name build selection. A copy causes no
      special status relationship or reconciliation state.
- [ ] Runtime protection shown in status is limited to relationships already in
      the selected project snapshot—current Manifest binding, a retained
      Manifest revision if available, pending adoption, last applied, and
      selected observed container—and is explicitly non-exhaustive for prune or
      delete safety. Full eligibility remains owned by `runtime prune dry-run`
      and Runtime lifecycle actions.
- [ ] Shared-entry, permission, and service availability are independently
      typed. A broken cluster does not by itself make the authenticated live
      owner snapshot unavailable.
- [ ] Domain/Application derives one structured primary Next and a separately
      ordered Attention collection. Presentation selects neither. Multiple
      permission/service items remain in Attention even when migration,
      cluster, Runtime, or Workspace safety produces the primary Next.
- [ ] Primary Next precedence is fixed: invalid/changed authority faults;
      migration/cluster prerequisite; missing persisted Manifest/default;
      Runtime revision/material readiness; Workspace absence/entry failure/
      block/pending; permission/service attention; enter/resume.
- [ ] Next is never automatic and contains only a Catalog-known command path
      plus typed inputs, or a typed non-command action. It stores no free-form
      command/argv string. Common Catalog validation and quoting render argv.
      `wait_for_detach` and `continue_attached` are non-executable guidance.
- [ ] `status [--manifest NAME] [--details] [--format text|json]` remains
      Catalog `RoleUtility`, `EffectRead`, capability `tobari.lifecycle`,
      complete delivery, scalar top-level coverage, and no produced/consumed
      references. No `--context` alias exists.
- [ ] Commands requiring opaque Runtime/revision/plan references route to their
      owning discover/review command, such as `review runtimes`; status remains
      `RoleUtility` and produces/consumes no references.
- [ ] `--details` changes only human projection; it causes no additional local,
      Docker, policy, owner, or JSON work. JSON is byte-identical with or
      without it.
- [ ] Fresh recommended-draft status makes zero Docker and live-owner calls and
      writes nothing. After every upstream implementation through WP09
      completes, the actual final stores, ports, Catalog, and adapters are
      re-measured before freezing a configured-status
      numeric ceiling independent of installation/same-root Workspace count;
      live-owner calls have a separate finite count and deadline.
- [ ] Expected absence/unavailability yields one complete typed degraded
      section. Invalid identity, contradictory adapter results, unsafe paths,
      ambiguous scope, or anchor churn after one bounded retry fails the entire
      command without mixed or partial success.
- [ ] `status` creates, deletes, cleans, reconciles, or publishes nothing: no
      config/state/lock/policy/credential/key/vault/journal/owner-record/socket/
      container/network/volume/image/AppliedEntry/failure/principal change.
- [ ] Styled TTY, non-empty `NO_COLOR`, and redirected text preserve identical
      semantic facts/order/Next; JSON is TTY-independent; scoped agent help
      exposes the complete contract without reviving old vocabulary.
- [ ] Missing bounded owner evidence is `not_observed`, never false zero, for
      permission, service, or attachment facts. Routine stays compact; details
      adds semantic fields but no I/O and never reveals raw Docker identifiers/
      tags, private paths, secrets, unrelated action refs, or inferred last use.
- [ ] Fixtures cover no-default draft with empty/non-empty siblings, ready/
      detached, attached, multiple Manifest-bound Workspaces, pending permission,
      broken cluster, entry pending/blocked/failed/unknown, every observed
      Runtime and material combination, migrated-unverified evidence, and
      explicit zero/null/not-observed with zero external reconstruction.
- [ ] Status consumes WP 01's exact predecessor migration: legacy Context UUID
      bytes become WorkspaceManifestID and legacy ProjectInstance UUID bytes
      become WorkspaceID. Status adds no migration reader, dual vocabulary, or
      implicit fill and can represent unverified migrated AppliedEntry evidence.
- [ ] WP01 retention/slice/session/migration decisions are consumed from final
      implementation and are not re-decided or emulated locally.
- [ ] Before implementation, an explicit gate records the completed integrated
      HEAD/branch/working-tree ownership after all upstream implementations
      through WP09; re-reads their actual schema/Catalog/domain/application/
      infrastructure/tests; safely replays read-only binaries in isolated state;
      and updates JSON, wireframes, state axes, owner summaries, and call budget.
      A moving/incomplete upstream blocks WP06 implementation.
- [ ] WP06 does not pre-implement WP03 Runtime types, WP07 wait state, WP09
      exposure owner protocol, or any upstream production/schema/Catalog/
      migration surface. It consumes WP08 recursive Catalog conformance and
      remains no-refs. The WP05-retired host name and WP04 research-only auth/
      serve paths appear in no output, fixture, Next, or Attention.
- [ ] Standard authentication remains Workspace-owned native state; status
      reads no credential files/provider output and reports login validity
      `not_observed`. Native/agent-ready compatibility is Runtime metadata/
      evidence, not authentication or overall Workspace authority.
- [ ] `task check`, `task security`, `task public:check`, and
      `task release:check` pass; agent-readiness semantic fixtures prove the
      routine outcome requires zero external reconstruction.

## Governing documents

- Higher/upstream decisions: WP01 through WP09 in the dependency order declared
  above; their final implemented contracts govern WP06 consumption.
- Thesis: promoted [docs/00_theses.md](../../00_theses.md), especially CWD
  ownership, bounded lifecycle, read purity, and executable claims.
- Product, architecture, security, and harness: promoted
  [docs/01](../../01_product_contract.md), [docs/02](../../02_architecture.md),
  [docs/03](../../03_security_model.md), and [docs/04](../../04_harness.md),
  plus any later reviewed consumer changes required by status implementation.
- Authentication/API readiness: `docs/07` through `docs/09`, preserving
  Workspace-owned standard authentication and exact task/read semantics.

## Completion definition

This work is complete only after all upstream implementations through WP09 are
complete and re-baselined, status fixtures and implementation prove the fixed
contract, all four required gates pass, durable conclusions are promoted, and
this temporary packet is removed. The implementation owner then commits and
notifies control with `WP06_IMPLEMENTATION_COMPLETE` or
`WP06_IMPLEMENTATION_BLOCKED`, final interfaces, gate evidence, exact HEAD/
status, retention outcome, and WP10 readiness. Updating this accepted/fixed
packet does not authorize or begin implementation.
