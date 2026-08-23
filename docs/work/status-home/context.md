# Work Context: Make `status` the CWD-centric Workspace home

This accepted/fixed design distinguishes historical researched-main facts, the
currently moving shared checkout, Product Owner decisions through WP09, design
evaluation, inferences, and implementation-time evidence still to collect.
Legacy names appear only when
describing historical code or migration input; they are not proposed aliases.

## Repository baseline

- **Verified:** `origin/main` and local `main` were
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42` when this packet was first
  researched.
- **Verified:** the initial tree was clean. Other untracked work packets now
  coexist in the shared workspace and are not owned or modified here.
- **Verified:** current `bin/tobari` was built from `bb64138...`, one product-code
  equivalent CI-only commit behind that baseline.
- **Higher durable decision:** accepted
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and its promoted [thesis](../../00_theses.md),
  [product](../../01_product_contract.md),
  [architecture](../../02_architecture.md),
  [security](../../03_security_model.md), and [harness](../../04_harness.md)
  consequences now govern the former WP01+WP02 scope. Their vocabulary,
  concept count, activation and copy models, desired/applied/observed/failure
  separation, reconciliation boundaries, and migration direction supersede
  this packet's earlier assumptions.
- **Verified 2026-08-23 upstream promotion:** commits `07535a9` and `428812f`
  contain the integrated implementation/durable-contract evidence. The former
  WP01 and WP02 temporary packet directories are deleted; they are no
  longer authorities or implementation dependencies.
- **Verified 2026-08-23 integration baseline:** the shared checkout is on
  `codex/workspace-manifest-v1` at
  `52a53bcc69a0f2bdf9bf2a6782ecd98bacd8b0e1`. At inspection, the only unrelated
  working-tree item was untracked `scripts/__pycache__/`; this packet does not
  own or modify it.
- **Classification:** the historical main observations below are evidence about
  the predecessor, not a fixed implementation baseline. No WP06 production
  implementation may begin until every upstream implementation through WP09
  announces completion and the exact integrated HEAD plus working tree are
  inspected again.
- **Completed upstream decision:**
  [ADR 0080 Runtime lifecycle](../../decisions/0080-close-the-managed-runtime-lifecycle.md)
  and its implemented public/semantic contracts constrain status; this packet
  must not duplicate its planner or lifecycle authority.

## Verified historical-main behavior

### Legacy model being replaced by WP 01

- Current code stores a legacy Context aggregate with a stable UUID, Boundary,
  Runtime binding, session defaults, and creation defaults. Runtime/default
  changes replace fields in one flat record rather than publishing an immutable
  complete desired revision.
- Current durable instance code uses `ProjectInstance`, `project_id`, and
  `instance_id` alongside public `workspace_id`. It stores last image and
  bootstrap information but no complete AppliedEntry or bounded latest
  reconciliation failure.
- Current uniqueness is `(canonical root, legacy Context UUID)`. WP 01 retains
  those UUID bytes as `(ProjectRoot, WorkspaceManifestID)` and retains the
  legacy ProjectInstance UUID bytes as WorkspaceID during exact predecessor
  migration.
- Current status can distinguish logical existence, runtime diagnostic,
  attachment, and bootstrap desired/applied relation. It cannot authoritatively
  say which complete Manifest entry slice was last applied, what exact desired
  revision failed, or whether post-action state is unknown.
- `EnsureProjectRuntime` is the explicit entry-side reconciliation path. The
  stored image is updated only after successful reconciliation. `cluster up`
  separately reconciles Gateway/OPA and aggregate projections.
- `status`, `list`, cluster status, and `doctor` are intended as read-only and
  must not become reconciliation paths.

### Current read surfaces

| Surface | Current result | Constraint for the V1 status consumer |
|---|---|---|
| `status` | Selected legacy-Context Workspace existence, runtime, attachment, bootstrap, `next_argv` | Must be replaced by Manifest desired / Workspace applied / runtime observed / failure read model |
| `list` | Installation-wide logical Workspaces with root, legacy Context identity, per-item runtime diagnostic | Final vocabulary uses WorkspaceManifestID/WorkspaceID; do not import its per-item Docker scaling into routine status |
| legacy Context list/show | Current/default selection, Boundary, Runtime binding, defaults, login ownership | Final commands are `manifest`; empty state is a draft recommendation, not synthetic authority |
| `runtime list` | Independent Runtime catalog and ready revisions | Status binds exact RuntimeID plus semantic revision digest, not name/ordinal/image tag |
| `cluster status` | Installation-wide components and policy/principal/Gateway projection validity | Status needs a bounded shared projection summary, not a nested exhaustive cluster command |
| `policy candidates` / `review permissions` | Bounded denial window plus typed review choices | Status may expose bounded counts/window/unparsed only, with no refs |
| `policy rules` | Stored learned decisions | Learned state remains outside Manifest desired/applied; distinguish stored count from active projection validity |
| `service requests` / `review services` | Fresh pending requests from live attachment owners | Host owner snapshot currently exposes pending requests only |
| `tobari-expose list` | Exhaustive active exposures for one current attachment | Active exposure count needs an authenticated host summary extension or remains unavailable |

- **Verified:** current host service rendezvous validates owner record mode,
  attachment nonce, peer PID/UID, bounded messages, and a Unix socket.
- **Verified:** its `snapshot` response does not carry an owner identity summary
  when no request exists and does not expose active exposures.
- **Verified:** current live-owner discovery removes malformed/unreachable
  records during a read. Status cannot reuse that cleanup behavior.
- **Verified:** current cluster observation scales across installation
  Workspaces. Calling it plus selected Workspace status would violate a fixed
  routine budget.
- **Verified:** Docker CLI supports multiple object and network targets in one
  inspect invocation, making a fixed selected-target batch technically
  possible. Exact missing-target behavior still needs platform evidence.
- **Verified:** fresh temporary-XDG runs of current `status`, legacy Context
  list, Runtime list, and cluster status created no files. Current fresh status
  reports a synthetic default; WP 01 rejects that as persisted Manifest
  authority and replaces it with a typed recommended draft.

## Accepted WP 01 model consumed by status

These are settled design inputs, not current implementation facts.

### Public concepts and identities

| Public concept | Presentation selector | Authority |
|---|---|---|
| Workspace Manifest | unique host-managed name, routine `Manifest` | `workspace_manifest_id` plus semantic revision digest where revision matters |
| Runtime | unique host-managed name | `runtime_id`; a revision is RuntimeID plus semantic digest |
| Workspace | canonical CWD/root in routine output | `workspace_id` permanently bound to ProjectRoot and WorkspaceManifestID |
| Runtime revision | subordinate `name@ordinal` presentation | RuntimeID plus semantic digest; ordinal is not authority |
| Project root | subordinate canonical path value | validated value in the trusted Workspace record; no Project ID |

Workspace Manifest is not a project file or serialization API. Final V1 has no
public Context alias and no parallel `project_id`/`instance_id` authority.

### Item-specific activation

| Manifest-owned value | Becomes applied when | Status implication |
|---|---|---|
| Boundary | invariant for WorkspaceManifestID | Never describe a Boundary edit as pending under one ID; another Manifest is required |
| Cluster projection input | explicit `cluster up` | Compare aggregate desired/applied/observed/failure separately from Workspace entry |
| Entry definition | explicit Workspace entry | Drives the independent entry-state axis and Current versus Next entry |
| Session defaults | new child session | Do not fold into container/entry adoption; attached-child behavior remains an owner decision |
| Creation defaults | new Workspace creation | Existing Workspace never becomes pending because this slice changed |

### Four status facts

- **Desired:** current immutable Manifest revision, entry slice revision, exact
  Runtime binding, and other activation slice revisions.
- **Last successfully applied:** Workspace AppliedEntry and create-once receipt;
  prior success remains authoritative after later failure.
- **Observed:** bounded validation of owned container/network/spec/principal,
  runtime health, and attachment. Labels do not establish authority alone.
- **Last bounded failure:** exact attempted desired revision, phase, code,
  change-state classification, and time/bounds; a failure does not erase the
  prior AppliedEntry.

The Product Owner correction for WP06 forbids compressing these facts into one
status/adoption enum. Workspace presence, entry state, observed Runtime state,
Runtime revision authority, execution-material availability, migration
evidence, and latest attempt are independent typed facts.

## Gap analysis against the earlier status-home packet

| Earlier assumption | WP 01 correction | Required packet change |
|---|---|---|
| Context naming might remain | Workspace Manifest/Manifest/`manifest`/`--manifest`/`workspace_manifest_id` are fixed | Remove future Context vocabulary and alias ambiguity |
| One mutable selected Runtime plus applied image comparison | Complete immutable Manifest desired revision and entry slice versus AppliedEntry | Make desired/applied/observed/failure the central read model |
| One adoption/overall enum | Presence, entry, observation, Runtime authority/material, migration evidence, and latest attempt are orthogonal | Replace aggregate enum in schema, wireframes, Next, fixtures, and tests |
| Entire Manifest timing could be described together | Activation is item-specific | Separate cluster, entry, session, and creation status sections |
| Attached pending entry could be reconciled on entry | Recreation-required adoption is blocked while attached | Report `blocked_attached`; zero mutation |
| No persisted migration needed for status | WP 01 introduces exact predecessor migration and AppliedEntry/failure evidence | Make status a post-migration consumer and test unverified migration states |
| Cluster readiness is a scalar adjunct | Cluster has its own desired/applied/observed/failure aggregate | Do not call Manifest desired cluster input active before `cluster up` |
| Fresh synthetic default | Recommended draft without ID or authority | Remove synthetic Manifest state and null-ID authority framing |
| Broken cluster made permission and service summaries unavailable together | Service owner observation is independently scoped | Represent section availability independently |

## Accepted WP 02 copy consequences consumed by status

- Manifest creation uses `manifest create --copy-from NAME --name NAME`; Runtime
  creation uses `runtime create --copy-source-from standard|NAME --name NAME`.
  `--base` is removed with no alias.
- Both operations create fresh, fully independent identities. No state or JSON
  retains `copied_from`, source identity, parent, base, provenance, ancestry, or
  lineage. Status therefore has no copy section, relationship edge, grouping,
  badge, filter, or protection rule.
- A copied Manifest is a complete immutable generation-1 desired declaration
  after publication. A copied Runtime is a fresh Runtime with current editable
  source and empty immutable revision history. Neither condition authorizes
  status to call it applied, ready, or available.
- Copy transfers no Workspace/home, authentication, learned permission,
  Attachment authority, AppliedEntry, failure, observation, pending adoption,
  or installation-default/invocation selection, and it performs no
  reconciliation. Any later
  Workspace state is created only by the ordinary explicit entry lifecycle.
- There is no shared production derivation type. Status consumes only ordinary
  Manifest and Runtime results and must not reconstruct a common copy model.

## Accepted WP 03 Runtime-lifecycle consequences consumed by status

- Runtime revision `ready` is authority: a valid successful immutable revision
  exists. Image `available` is local execution material: the exact compatible
  execution image exists. Either can change independently of desired/applied
  Workspace relation; status must expose both separately.
- Availability states include `available`, `missing`, `mismatched`, `unknown`,
  and `pruned` in the accepted design. Final status spelling must be reconciled
  with the actual implemented WP 03/domain schema rather than inventing a
  parallel enum.
- Runtime manifests remain schema-stable; build/retirement journals and bounded
  idempotent receipts are owner-only sidecars. Status may read only a reviewed
  semantic projection and must not expose raw Docker tags/IDs or private
  snapshot paths.
- `last_used` stays `unknown` without an independently approved exact usage
  receipt. AppliedEntry `reconciled_at`, pending adoption, and running/stopped
  container observation are separate facts, not last-use evidence.
- Destructive protection distinguishes current and retained Manifest revisions,
  Workspace pending adoption and last-successful AppliedEntry, and observed
  running/stopped/foreign containers; unknown fails closed. Routine status is
  not that complete installation-wide protection planner. It can report only
  selected-snapshot relationships and cannot assert prune/delete eligibility.
- `runtime delete --id ... --confirm=delete`, `runtime prune dry-run`,
  `runtime prune apply --plan ... --confirm=prune`, and
  `runtime restore --id ...` remain explicit lifecycle commands. Status never
  calls them. `review runtimes` performs human build selection and
  `runtime build --id ...` performs the action; old build-by-name/omission and
  `--base` are absent, not aliases.
- Status is fixed as `RoleUtility` with no produced/consumed references. A
  reference-bound Runtime action routes through the WP08-conformant owning
  discover/review command; WP06 never adds a Runtime-only validator or direct
  reference-bearing Next.

## Relevant future structure

- Domain: WP 01 WorkspaceManifest, WorkspaceManifestRevision, activation
  revisions, Runtime/RuntimeRevision, Workspace, AppliedEntry,
  ReconciliationAttempt, ObservedWorkspaceRuntime, ProjectRoot, and pure status
  derivation.
- Application: one StatusHome use case that owns CWD/Manifest/Workspace
  correlation, orthogonal-fact snapshot consistency, optional section
  degradation, structured primary Next, and ordered Attention.
- Infrastructure: final-V1 stores/migration readers supplied by WP 01; pure
  local snapshot; fixed-target batched Docker observation; bounded denial read;
  non-cleaning authenticated owner summary.
- CLI/Catalog: final `status [--manifest NAME] [--details] [--format
  text|json]`, Manifest-only vocabulary, complete recursive JSON, semantic
  human routine/details, and exact faults/recovery.

## Mandatory implementation re-baseline after all upstream work through WP09

This is a blocking preflight gate. WP06 runs after WP01+WP02, WP08, WP03, WP04,
WP05, WP07, and WP09 production implementations:

1. Obtain every upstream completion handoff through WP09 and record the exact
   integrated commit, branch, merge relationship to `origin/main`, and
   `git status` ownership.
2. Refuse to start status production work while any upstream files are still
   moving or owned diff is ambiguous. Do not fold, repair, or overwrite it.
3. Re-read the final ADR and governing durable documents, schema versions,
   Catalog entries and generated reference data, domain types, application
   ports, infrastructure stores/migration/observation adapters, and tests.
4. Run the integrated binary only with read-only commands and isolated
   temporary XDG
   state (plus explicit cleanup) to recheck first-use zero writes, Manifest
   selection, desired/applied/failure states, Runtime readiness/availability,
   and cluster behavior.
5. Compare the implemented enums, absent/empty rules, opaque-reference fields,
   consistency anchors, and port batching with this packet. Update only this
   packet first if assumptions differ.
6. Re-measure Docker and live-owner calls from the final adapters, including the
   WP09 authenticated owner summary. Freeze a numeric
   budget only after that evidence; do not carry forward the predecessor's
   four-call decomposition as fact.
7. Produce an explicit non-overlap map before editing production: upstream
   packets own their types/schema/Catalog/migration/owner protocols; status adds
   only consumer-specific snapshot, ports, rendering, and tests after those
   surfaces settle.

## Ownership, scope, lifetime, and mutability

| Fact | Authoritative owner | Lifetime | Status mutability |
|---|---|---|---|
| CWD/root selection | invocation plus validated root index | invocation | none |
| installation default Manifest selection | trusted Manifest catalog | until explicit `manifest default set --name` | none |
| Manifest desired revision/slices | WorkspaceManifestID and semantic digests | until next accepted immutable revision; Boundary for ID lifetime | none |
| Runtime binding | Manifest entry slice, exact RuntimeID/digest | Manifest revision | none |
| Runtime revision readiness | Runtime immutable revision authority | Runtime lifetime | none |
| Runtime image availability | exact local execution observation | bounded invocation / replaceable material | none; lifecycle commands mutate |
| Runtime last use | separately approved exact receipt, if any | receipt-defined | none; otherwise `unknown` |
| Workspace | WorkspaceID + ProjectRoot + WorkspaceManifestID | Workspace lifetime | none |
| AppliedEntry | Workspace trusted host record | until later confirmed entry | none |
| last failure/unknown | Workspace trusted bounded attempt record | latest bounded evidence | none; status never clears it |
| observed runtime/attachment | exact owned Docker resources/attachment epoch | bounded invocation | none |
| cluster projection | installation singleton desired/applied/attempt/observation | installation and binary/Manifest revisions | none; only `cluster up` writes |
| native auth | native client in Workspace home | Workspace lifetime | not read; validity `not_observed` |
| learned permission | policy owner keyed by WorkspaceManifestID/WorkspaceID | until explicit reset/delete | none |
| request/exposure | authenticated attachment owner | attachment lifetime | none |

## Evaluation

- **Evaluation:** Project-root-first remains the correct status scope. WP 01
  strengthens it because `(ProjectRoot, WorkspaceManifestID)` is now the exact
  durable key and multiple Manifest-bound Workspaces at one root are explicit.
- **Evaluation:** live observation should remain selected-Workspace-only;
  same-root siblings carry logical/receipt summaries without per-sibling Docker
  calls.
- **Fixed:** Manifest selection never comes from a project-local current value
  or existing Workspace. After root resolution, explicit `--manifest` wins;
  otherwise only the WP01 installation default applies. Without it, the
  recommended draft has no authority and same-root Workspaces remain siblings.
- **Fixed:** entry state is derived from the entry slice, not the whole Manifest
  generation, and is only one axis. Presence, observation, Runtime authority/
  material, latest attempt, and migration evidence remain separate.
- **Evaluation:** the routine contract should lead with `Current` and `Next
  entry`, then bounded observation/failure. A desired revision is never called
  current until AppliedEntry and observation validate it.
- **Evaluation:** optional-source unavailability may produce a complete degraded
  snapshot, but malformed authority/receipt/observation and mixed anchors fail
  the whole task.
- **Evaluation:** `--details` remains presentation-only so call count and JSON
  facts do not vary by audience.
- **Evaluation:** Runtime protection in routine status must be a selected-scope
  relationship summary, never a safety decision. Pulling the whole installation
  protection graph into status would scale with installation size, duplicate
  prune policy, and turn unknown evidence into a dangerous apparent answer.
- **Evaluation:** status should remain reference-free unless a cross-cutting
  Catalog review explicitly changes its role/output. Consequently, the safe
  default Next for a reference-bound Runtime action is a producer such as
  `review runtimes`, not an argv reconstructed from Runtime ID/digest text.

## Fixed status axes and precedence

- `workspace_presence`: `absent|present`.
- `entry_state` when present: `current|pending|blocked_attached|failed|unknown`.
- `observed_runtime_state`:
  `not_observed|absent|stopped|running|drifted|unknown`, aligned to final WP01.
- `runtime_revision_authority`: `ready|not_ready|unknown`.
- `execution_material_availability`:
  `available|missing|mismatched|pruned|unknown`.
- `latest_attempt` remains optional and typed. A final failure counts as
  `entry_state=failed` only when it targets the currently desired entry. An
  older failure remains history.
- Invalid/foreign ownership, contradictory authority, unsafe paths, ambiguous
  scope, and anchor churn after one bounded retry are whole-command faults.
  Expected absence/unavailability may instead be a complete degraded section.
- WP01 state-integrity/migration values are consumed only if actually exposed.
  Migrated-unverified evidence is explicit bounded evidence, not a synthetic
  overall status.

Presentation may derive a non-authoritative headline. No persisted or public
JSON `overall_status`, `adoption_state`, or equivalent lossy authority exists.

## Upstream implementation evidence WP06 must consume, not decide

- WP01 final retained-revision body, Git slice ownership, attached-child
  behavior, migration Docker evidence, and migration preconditions determine
  which final fields are available. WP06 consumes the implemented result and
  invents none of them.
- WP03 supplies Runtime readiness/material types; WP07 supplies wait-state
  semantics; WP08 supplies recursive Catalog conformance; WP09 supplies the
  bounded authenticated exposure-owner summary. WP06 creates no substitutes.
- WP04 research-only auth/serve paths are absent from release status. WP05's
  retired host name is a negative fixture and never presentation vocabulary.

## Remaining implementation-time unknowns

- [ ] Map the fixed semantic axes to the exact final upstream field/type names
      without changing their meanings or adding an overall enum.
- [ ] Measure whether owner churn resolves within the one bounded snapshot retry;
      otherwise fail the command rather than infer exhaustive zero.
- [ ] Record WP09's implemented finite live-owner count and global deadline.
- [ ] After every upstream implementation through WP09, measure the actual
      adapters on Engine 23/24 and
      supported Desktop/Colima paths, including a missing member in batched
      inspect, and freeze the smallest justified numeric budget. The predecessor
      maximum-four plan is a provisional candidate only.
- [ ] Bind each fixed primary Next variant to an actual integrated Catalog path
      and typed inputs; if no safe Catalog task exists, block implementation
      rather than invent recovery text.
- [ ] Freeze JSON field names after the final integrated schemas, not from
      legacy structs or packet examples.
- [ ] Consume WP05's implemented negative-enforcement seam so WP06 output and
      fixtures never embed the retired host name, including as a canary value.

## External facts

- Docker, “docker inspect,” <https://docs.docker.com/reference/cli/docker/inspect/>,
  checked 2026-08-23: one invocation accepts multiple object names/IDs and
  returns an array; result identity and absence still require strict validation.
- Docker, “docker network inspect,”
  <https://docs.docker.com/reference/cli/docker/network/inspect/>, checked
  2026-08-23: one invocation accepts one or more networks.
- NO_COLOR, <https://no-color.org/>, checked 2026-08-23: a present non-empty
  value requests suppression of added ANSI color. Tobari also requires
  redirected text to keep identical semantics.

## Security and public-boundary notes

- Assets: Manifest desired identities/slices, Workspace AppliedEntry/failure,
  ProjectRoot/WorkspaceID, Runtime identities, bounded Docker observation,
  cluster projection identities, policy metadata, and attachment counts.
- Credentials: none are read or returned. Login validity is `not_observed`.
- Trust changes: upstream work adds trusted host-owned AppliedEntry/attempt and
  WP09 authenticated owner-summary evidence. Status only reads and validates
  their approved projections and adds no owner-protocol extension, action
  authority, URL, port, ref, payload, or credential.
- Read purity: no store initialization, migration, journal publication,
  failure clearing, AppliedEntry refresh, principal rewrite, Docker/network
  convergence, live-record cleanup, or socket creation.
- Consistency: capture desired/applied/failure anchors before external reads and
  revalidate them afterward. A changed anchor fails without mixed output.
- External text remains untrusted and visibly projected. Names/generations/
  ordinals/image tags/container IDs never substitute for semantic digests and
  host-issued identities.
- Runtime availability is observation, never authority. Raw Docker identities,
  tags, private snapshot paths, and inferred usage timestamps do not cross the
  public status boundary.

## Packet dependencies and conflicts

- Promoted WP01+WP02 contracts: [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and [docs/00](../../00_theses.md) through [docs/04](../../04_harness.md) are
  the durable authority. Their deleted temporary packets are not dependencies;
  status cannot define competing Manifest/Workspace, copy, migration,
  activation, or selection types.
- Completed WP08 contracts in [Architecture](../../02_architecture.md) and
  [Harness](../../04_harness.md): govern Runtime/status consumption; WP06 uses
  recursive Catalog conformance and remains no-refs.
- [ADR 0080](../../decisions/0080-close-the-managed-runtime-lifecycle.md): supplies readiness/availability/unknown-usage and
  owning discovery paths. WP06 adds no Runtime lifecycle types or planner.
- [ADR 0082](../../decisions/0082-release-and-research-build-surfaces.md): supplies release/research surface exclusion;
  research auth/serve never appears in release status.
- `host-loopback-name` (WP05): supplies final private host authority vocabulary;
  the retired name is excluded from output and fixtures.
- `first-use-progress-recovery` (WP07): supplies wait/progress state; WP06 does
  not pre-implement it.
- `service-exposure-ux` (WP09): supplies the bounded authenticated exposure
  owner protocol and `not_observed` behavior; WP06 does not extend it first.
- `first-public-release-core`: must not freeze the legacy status schema or old
  vocabulary; status schema replacement lands after/with WP 01 cutover.
- `policy-compaction-retirement`: owns learned-rule retention/count semantics.
- `v1-auth-narrowing`: owns Workspace-home native authentication claims.
- Service exposure: owns attachment snapshot protocol and active exposure
  lifetime; status may consume only a reviewed count summary.
- Legacy Context capability/source-access packets remain security evidence for
  Boundary behavior but are superseded for public vocabulary/lifecycle by WP 01.

## Glossary

- **Workspace Manifest:** host-owned revisioned desired declaration shared by
  multiple root-bound Workspaces; routine label `Manifest`.
- **AppliedEntry:** last successfully verified entry reconciliation receipt for
  one Workspace. It remains authoritative after a later failure.
- **Observed runtime:** bounded present-time evidence about exact owned Docker
  resources; it does not replace AppliedEntry or desired authority.
- **Entry state:** one closed axis derived from desired entry, AppliedEntry,
  exact attachment/recreation evidence, and latest matching attempt; it is not
  overall Workspace status.
- **Primary Next:** one structured Catalog task plus typed inputs, or typed
  non-command guidance, selected by Domain/Application.
- **Attention:** separately ordered non-primary permission/service facts that
  presentation preserves even when a higher safety prerequisite wins.
- **Recommended draft:** first-use presentation without a saved Manifest ID or
  authority.
- **Degraded snapshot:** a complete valid status result with one or more typed
  unavailable observations, not partial success from invalid data.
