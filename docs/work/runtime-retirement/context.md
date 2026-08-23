# Work Context: Close the managed Runtime lifecycle

This file separates verified current-main facts, evaluation, inference, and
unknowns. Desired behavior belongs in `plan.md` and is not current behavior.

## Repository observation baseline

- **Product Owner-fixed packet state, 2026-08-23:** WP03 design is Accepted and
  fixed; implementation is Planned and has not started. The fixed implementation
  order is WP01 + WP02 completion audit -> WP08 -> WP03.
- **Verified upstream promotion, integrated HEAD `52a53bc`:** ADR 0079 and
  `docs/00_theses.md` through `docs/04_harness.md` now contain the durable
  Workspace Manifest, Workspace, Runtime-copy, migration, security, and harness
  contracts promoted by the completed upstream work. Commit `07535a9`
  supplies the model/copy implementation evidence and `428812f` supplies the
  migration/security quarantine evidence. The former packet directories have
  no Git-tracked files and are not authorities.
- **Coordination constraint:** the historical current-main observations below
  remain design evidence. Before implementation, WP03 must re-observe the actual
  post-upstream/WP08 integration HEAD and status rather than treating this
  packet-fix checkout as an implementation baseline.

- **Verified, 2026-08-23:** `git fetch origin main --prune` completed. Local
  `HEAD` and `origin/main` were both
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`
  (`chore(ci): retire obsolete runtime workflows`). The initial
  `git status --short --branch` was `## main...origin/main` with no changes.
- **Verified during research:** other delegated work packets later appeared as
  untracked directories in the shared worktree. They were treated as user or
  concurrent-agent work, read only where relevant, and not edited. This packet
  must add only `docs/work/runtime-retirement/`.
- **Verified:** repository `AGENTS.md`, `docs/00_theses.md` through
  `docs/04_harness.md`, and `docs/07_authentication.md` through
  `docs/09_agent_readiness_validation.md` were read in numeric order before
  design. README, ADRs 0067/0069/0070/0071, the Runtime Catalog, domain,
  application, infrastructure, tests, and relevant work packets were inspected.
- **Verified:** a current-source `go run ./cmd/tobari help runtime --format
  agent` used isolated temporary HOME/XDG/Go-cache directories and exited 0.
  It reported exactly five Runtime commands: `runtime list`, `runtime show`,
  `runtime create`, `runtime history`, and `runtime build`. It created no
  repository state. A checked-in binary observed earlier was stale and is not
  used as current-HEAD proof.

## Verified current behavior

### Public command and lifecycle surface

- `docs/01_product_contract.md` and `internal/cli/runtime_catalog.go` declare
  only list, show, create, history, and build for capability
  `runtime.customization`.
- Repository search and current-source agent help contain no public `runtime
  delete`, `runtime revision delete`, `runtime prune`, `runtime restore`, or
  Runtime `gc`. The first public V1 therefore lacks lifecycle closure for a
  managed Runtime and its retained build material.
- `runtime list`, `runtime show`, and `runtime history` are currently
  `RoleUtility` reads and produce no opaque references. Show/history select a
  Runtime by human name.
- `runtime create` and `runtime build` are `RoleAct` commands against the
  command-bound `tool_local` fixed target kind `runtimes` with ID
  `runtime-catalog`. Build mutates one selected managed Runtime, but its catalog
  declaration targets the complete catalog because current review permits an
  omitted name.
- Existing JSON exposes Runtime stable ID and, for each revision, ordinal,
  semantic `revision` digest, Docker image selector, inspected
  `image_digest`, creation time, and absolute snapshot path. Current human
  presentation treats `name@ordinal` as the meaningful selection syntax.

### Runtime authority and persistence

- `internal/domain/tobari/runtime_library.go` defines an installation-wide
  `RuntimeManifest` schema 1 with stable ID, unique name, kind, managed source
  path, and a non-null ordered revision collection. Managed Runtimes may have
  zero revisions; built-in standard must have exactly one.
- Revision ordinals must be contiguous from one. Semantic revision digests must
  be unique. Managed revisions require an inspected image digest and canonical
  absolute snapshot path.
- `RuntimeBinding` comments and ADR 0067 define stable Runtime ID plus semantic
  revision digest as persistent authority. Name and ordinal are review
  metadata. Image selector and inspected image digest are execution material
  and evidence, not Runtime revision identity.
- `RuntimeSummary.Ready` currently means only that the manifest has at least one
  revision. Runtime reads do not inspect whether the corresponding local image
  exists, matches the stored digest, or is compatible.
- Managed source lives below the installation config Runtime directory. Source
  and every child must be owner-only, bounded to 1,024 files, 256 directories,
  32 MiB per file, and 64 MiB total, with regular files/directories only and a
  root Dockerfile.
- Build copies and hashes the complete source into a private temporary source
  snapshot, builds only from that snapshot, freezes the final successful
  snapshot, and appends the manifest after image compatibility and digest
  inspection. Snapshot files become owner-readable and directories remain
  owner-only.
- The Docker image tag is derived from Runtime name and the first twelve source
  digest hex characters. The inspected Docker `.Id` is persisted as
  `image_digest`. Current code does not add a build-attempt journal or an exact
  candidate-image ownership record.

### Failure, interruption, and external disappearance

- A source digest already present in history returns `no_change` before Docker
  image availability or digest is revalidated. A manually removed or replaced
  recorded image can therefore be hidden by successful no-change output.
- Docker build failure removes the temporary source snapshot and does not
  append history. BuildKit cache may remain. If build/load succeeds but
  compatibility inspection, image digest inspection, snapshot promotion, or
  manifest publication fails, the deterministic candidate image tag may remain
  without manifest authority.
- There is no durable build journal tying an uncommitted candidate to an exact
  Runtime ID and source digest. A name-prefix scan would therefore be an unsafe
  substitute for ownership.
- Runtime manifest publication, filesystem snapshot promotion, and Docker image
  creation are not one atomic transaction. Current code relies on ordering and
  cleanup but cannot classify or resume every process-death boundary.
- A predecessor `Context` can retain a valid Runtime binding after the local
  image disappears. A future Workspace reconciliation then fails at image
  inspection; no exact Runtime restore command exists.
- Immutable source snapshots do not guarantee bit-identical rebuilds when a
  Dockerfile uses mutable or unavailable base images or downloads. The recorded
  image digest can safely detect drift, but current state contains no exported
  image archive that guarantees restoration.

### Predecessor Context and Workspace protection on current main

- ADRs 0067 and 0071 say Contexts bind an exact Runtime ID and semantic
  revision. Building never changes a Context; an explicit Context Runtime
  mutation changes the desired binding and existing Workspaces adopt it only on
  next entry.
- `SetContextRuntime` currently resolves a name/ordinal to the exact binding
  before acquiring the Context store lock, then writes the Context under that
  narrower lock. Runtime store and Context store locks are separate.
- An installation lifecycle lock already exists and is held by Context and
  Workspace application services across check-then-mutate lifecycle sequences.
  Runtime library build/create currently use their own Runtime store lock.
- `context delete` holds the lifecycle boundary through application code,
  rejects the foundational or active Context and any Context with a durable
  Workspace, deletes Context/auth state, and leaves shared Runtime images and
  managed Runtime records unchanged.
- Current `ProjectInstance` records the selected image string and Docker runtime
  evidence, not one authoritative applied Runtime ID plus semantic revision.
  Existing Docker work containers have exact Tobari owner/component/project/
  role/spec labels and an inspected content ID. ADR 0079 and the integrated
  architecture now require a durable Workspace `last_successful_entry` with
  exact Runtime revision for truthful current/recovery reporting.
- A Workspace container can continue using an older revision after its Context
  changes desired Runtime until the next successful entry. Protecting only
  current Context bindings would therefore be unsafe.

### Confirmed target model from ADR 0079 and durable contracts

- **Accepted ADR 0079 decision, 2026-08-23:** the public `Context` concept is
  retired without an alias. Product language is Workspace Manifest, routine
  display is Manifest, CLI namespace/flag are `manifest` / `--manifest`, and
  schema identity is `workspace_manifest_id`.
- The durable public resource budget is exactly Workspace Manifest, Runtime,
  and Workspace. Runtime revision and Project root are subordinate concepts.
- A Workspace Manifest is host-owned, CLI-managed, stable-ID authority. Each
  accepted mutation issues one complete immutable desired revision; Boundary
  is immutable within one Manifest identity. It is not project-owned YAML,
  generic import, or `apply -f`.
- Runtime binding authority is exactly Runtime ID plus semantic revision digest.
  `name@ordinal`, head/display status, Docker tags, image IDs, and container IDs
  are presentation or infrastructure evidence only.
- A Workspace is a durable instance bound to `(ProjectRoot,
  WorkspaceManifestID)` with public authority `workspace_id`; it is not its
  runtime container. Retirement must preserve its ID, home, and applied receipt.
- Desired, last successfully applied, observed, and last bounded failure are
  separate. Only explicit Workspace entry reconciles a Workspace Runtime;
  `cluster up` alone reconciles shared cluster projection; reads never converge
  state and no resident controller exists. A desired change does not mutate a
  running Workspace, and attached adoption requiring recreation is blocked.
- Runtime protection must distinguish (1) current Manifest bindings, (2)
  immutable Manifest revision bodies exposed by the durable retained-revision
  inventory, (3) each Workspace `last_successful_entry`, (4) pending desired
  adoption, and (5) observed owned-container use. A last bounded failure is
  diagnostic, not independent retention authority; it protects a Runtime only
  through an exact retained Manifest body or other exact edge above.
- Pre-public migration intends to preserve predecessor Context UUIDs as
  WorkspaceManifestIDs and predecessor ProjectInstance UUIDs as WorkspaceIDs,
  with no dual vocabulary. A `last_successful_entry` may be synthesized only
  when state plus read-only Docker evidence proves the exact current desired spec;
  otherwise migration records unverified/pending state and Runtime cleanup
  fails closed.

### Existing test and gate evidence

- Domain tests enforce managed/built-in identity, revision digest validity,
  contiguous ordinals, and zero-revision managed Runtime validity.
- Infrastructure tests cover bounded source traversal, symlink/special-file and
  mode rejection, streaming snapshots, immutable successful snapshots,
  no-change builds, failed-build history preservation, and Context independence.
- Context and project tests cover exact binding, explicit rollback, Workspace
  next-entry adoption, home preservation, owner labels, image identity, and
  lifecycle locking.
- Catalog tests enforce that a mutating command is `RoleAct`; a reference-bound
  act requires at least one required opaque reference; a fixed-target act
  consumes none; a write's `target_id_input` kind equals `TargetKind`; required
  reference chains have an invocable producer; and every mutation has complete
  impact facts.
- Completed WP08 implementation makes `CommandSpec.ProducedRefs()` traverse
  bounded `OutputField.Fields` and `Items` recursively, so Runtime identities
  inside paths such as `items[].runtime_ref` and
  `revisions[].revision_ref` create executable producer edges through the
  shared Catalog contract.
- Current tests have no Runtime-retirement target round trip, predecessor Context/Workspace
  reference barrier, image sharing, prune plan, last-used, disk accounting,
  build journal, delete journal, interruption, or idempotent retirement receipt.

### Related repository history

- `01f65b8 feat: create Runtime sources from a Base` introduced empty managed
  source lifecycle and one-time source copy.
- `7e47a6e feat(cli): close Runtime customization handoffs` aligned current
  Runtime help/output handoffs but did not add retirement.
- `b6b8f16 Harden cluster recovery after runtime disruption` is relevant
  precedent for causal, read-before-retry recovery after runtime disruption.
- `6a26a3c chore(ci): retire obsolete runtime workflows` is current main and is
  unrelated to managed Runtime object retirement.

## Relevant structure

- Entry point and composition: `cmd/tobari`, `internal/cli/runtime_catalog.go`,
  `internal/cli/runtime_library.go`, and the catalog-owned argument parser.
- Domain rule: `internal/domain/tobari/runtime_library.go`, operation effects,
  intent, target references, impact, task identity, and fault vocabulary.
- Application use case: `internal/app/tobaricmd` Runtime ports and service, plus
  predecessor Context/Workspace lifecycle services that own the lifecycle lock.
- Infrastructure boundary: `internal/infra/dockerruntime/runtime_library.go`,
  predecessor Context store, project state/reconciliation, lifecycle lock, and bounded
  Docker runner.
- CLI catalog/presentation: Runtime command specs, structured output schema,
  human Runtime rendering, Review conventions, and mutation-complete output.
- Harness: catalog/reference/mutation tests, Runtime unit/integration tests,
  hostile-output tests, capability/schema ledgers, architecture-site generated
  data, public guard, and release/public profiles.

## Constraints

### Product and compatibility

- The CLI must close the lifecycle outcome; it cannot expose raw Docker or
  filesystem cleanup and require the caller to join Workspace Manifest,
  Workspace, image,
  and snapshot state.
- Discovery and action stay separate. Every destructive act is either bound to
  one required opaque reference or a command-owned singleton. A Runtime name
  does not make `runtime delete` a fixed target.
- Target public language is fixed by ADR 0079 and `docs/00` through `docs/04`:
  Workspace Manifest / Manifest,
  `manifest`, `--manifest`, `workspace_manifest_id`, and `workspace_id`, with no
  predecessor aliases. The Runtime packet may not reopen that vocabulary.
- The integrated durable state retains authoritative Manifest revision receipts
  and exposes a complete current/retained protection inventory. Runtime prune
  consumes that inventory and may not shorten it or decide that only current
  desired state matters.
- The repository is pre-public and strict V1 readers are intentional. Ideal
  schema-1 public output may replace provisional fields without aliases, but
  current persisted Runtime manifests should not be rewritten without an
  independently necessary state change.

### Architecture and security

- Domain remains pure; application owns complete protection interpretation and
  smallest task ports; infrastructure owns filesystem/Docker observation and
  effects; CLI remains composition/presentation.
- No command receives an unrestricted filesystem, Docker, process, or network
  executor. Exact owned paths and exact Docker operations cross one reviewed
  infrastructure boundary.
- External text and Docker output are untrusted. Docker labels and tag prefixes
  are evidence only until exact ownership, target identity, and content digest
  are revalidated.
- Unknown effect, target, Manifest-retention scan, Workspace applied/pending
  scan, Docker use, image
  identity, snapshot safety, journal state, or result fails closed.
- Reads stay zero-write. Cleanup of an old journal is not hidden inside list,
  show, history, status, doctor, or prune dry-run.
- No force deletion and no daemon-global image/system/builder prune are allowed.

### Platform, dependency, and release

- Docker image layers and tags are shared. Image sizes are non-additive and
  reclaimable bytes cannot be inferred from a virtual size.
- A bounded owner-specific inspection must replace Docker daemon-wide output
  where the latter can grow with unrelated resources.
- No new third-party dependency belongs in `cmd` or `internal/cli`; existing
  runner and standard library primitives are sufficient unless a later ADR and
  dependency review prove otherwise.
- Public documentation is English. Fixtures must use synthetic IDs, paths,
  digests, timestamps, tags, and logs.

## External facts

Checked 2026-08-23 against current official Docker documentation:

- **`docker image rm`** — Docker CLI reference,
  <https://docs.docker.com/reference/cli/docker/image/rm/>. Removing by tag may
  only untag an image when other tags remain; removing the final reference may
  remove image content. Docker rejects removal of an image used by a running
  container unless forced. Design consequence: use only exact Tobari-owned tags,
  inspect container use, never force, and report preserved shared content.
- **`docker image prune`** — Docker CLI reference,
  <https://docs.docker.com/reference/cli/docker/image/prune/>. Default prune
  removes dangling images; `--all` broadens to images not referenced by any
  container; filters such as `until` are daemon-wide and negative-filter
  prediction is difficult. Design consequence: do not invoke this global
  command for Tobari ownership.
- **`docker builder prune`** — Docker CLI reference,
  <https://docs.docker.com/reference/cli/docker/builder/prune/>. It removes
  build cache and can broaden to all unused cache. Design consequence: exclude
  BuildKit cache from V1 Runtime prune because the cache may be shared and has
  no Runtime authority record.
- **`docker system df`** — Docker CLI reference,
  <https://docs.docker.com/reference/cli/docker/system/df/>. Verbose output
  distinguishes shared, unique, and virtual sizes; shared layers make image
  sizes non-additive. Design consequence: report bounded image-size evidence
  and uncertainty, never sum virtual sizes into a promised reclaimable total.
- **Storage drivers** — Docker Engine manual,
  <https://docs.docker.com/engine/storage/drivers/>. Containers share
  read-only image layers, and current Docker Engine installations may use the
  containerd image store. Design consequence: the public contract names Runtime
  image availability/material, not a storage-driver-specific deletion model.

No external API schema, fetched content, credential, or licensed artifact is
needed for implementation. Docker documentation is design evidence only.

## Evaluation

- The lifecycle gap is a V1 product defect: creation without supported
  retirement leaves ordinary users with Docker and filesystem internals as the
  only escape hatch.
- Whole-Runtime delete and unused image prune serve different outcomes.
  Combining them would either make delete incomplete or make storage cleanup
  unexpectedly destroy editable source and reproducibility evidence.
- Individual revision deletion conflicts with current append-only contiguous
  history and the accepted future-domain interface. It would add tombstones,
  ordinal semantics, reference migration, and recovery complexity without being
  required to reclaim daemon image storage.
- A single `runtime prune --dry-run`/non-dry-run command would make effect, role,
  output, and mutation impact depend on argv. Two catalog leaves keep read and
  write contracts static and make the reviewed candidate set an exact action
  target.
- A public generic `gc` hides the owned resource and encourages daemon-global
  behavior. Runtime-specific prune is narrower and explainable.
- Source and image material need different internal lifetimes, but exposing
  separate public source/image resources would permit unsupported half-Runtime
  states. Keep the separation internal and task-owned.
- Docker image digest remains essential internal verification evidence, but it
  should not be a routine public selector, reference kind, or target identity.

## Inferences to validate during implementation

- A Runtime/revision opaque reference can be derived from kind plus stable
  Runtime ID and semantic revision, then resolved by exact comparison across the
  bounded local catalog without decoding user input.
- Once Runtime references exist, current build target binding can be corrected:
  create genuinely owns the catalog creation scope, whereas build changes one
  selected existing Runtime and should consume that Runtime reference. A
  separate discover/review entry can preserve the current human selection flow.
- The existing installation lifecycle lock can serialize Runtime retirement
  against Workspace Manifest mutation/deletion, Workspace creation/entry, and
  Runtime build/restore if lock ordering is promoted and mechanically tested.
- The confirmed Workspace `last_successful_entry` is exact applied protection
  and includes `reconciled_at`. That timestamp can support a
  last-successfully-applied diagnostic, but it is not proof of runtime use. A
  Product Owner fixed no V1 usage receipt, so `last_used` remains unknown rather
  than being inferred from entry, container activity, filesystem timestamps, or
  migration.
- Exact source/snapshot logical byte totals can be computed within existing
  source bounds. Docker virtual size can be observed per exact image, while
  unique/shared/reclaimable size should remain unknown unless a bounded
  authoritative primitive is proven.
- A journal outside the retiring Runtime directory can preserve stable target
  identity and resume after the active directory is atomically quarantined.

## Upstream audit inputs and implementation evidence

These items do not reopen the Product Owner-fixed WP03 design. The first three
are now supplied by ADR 0079 and integrated `docs/00` through `docs/04`; the
remainder prove that the fixed contract is implementable on the integrated
checkout and supported Docker platforms.

- [x] The durable Runtime-protection seam exposes every current and retained
      Manifest Runtime reference. Runtime protection and GC consume that
      inventory and do not shorten its retention window.
- [x] The durable seam exposes pending desired adoption separately from each
      Workspace's `last_successful_entry` and observed runtime/container edge.
- [x] ADR 0079 and durable security/harness contracts require sufficient
      read-only Docker evidence for exact `last_successful_entry`, otherwise
      unverified/pending state, plus cluster-stopped/no-attachment migration.
- [ ] Prove the complete lock graph and ordering across lifecycle, Runtime
      store, Workspace Manifest store, Workspace state, and Docker. Add a
      race fixture before choosing journal step ordering.
- [ ] Observe supported Docker platforms for exact behavior when untagging an
      image with foreign tags, stopped/running foreign containers, and shared
      content. V1 must block or preserve rather than force.
- [x] Product Owner fixed no V1 image archive. Restore uses snapshot rebuild plus
      digest equality and explicitly permits `unrestorable`; a future archive is
      outside WP03.
- [ ] Establish a bounded, owner-only build-attempt identity and prove CLI build
      labels cannot be overridden by an untrusted Dockerfile.
- [x] Product Owner fixed bounded receipt retention sufficient for idempotent
      exact retry without a generic collector. Implementation must promote and
      test the exact mechanical bound for one retired Runtime or removed
      candidate set, co-located with lifecycle evidence.
- [ ] Confirm whether any current Docker inspect primitive supplies bounded
      unique/shared byte evidence per exact image on every supported platform.
      Until proven, those fields remain explicit unknowns.
- [x] Product Owner fixed no additional optional opaque-reference read input in
      WP03 V1. Human-name reads remain discovery only; exact actions remain
      reference-bound and idempotent action receipts reconcile repeated targets.
- [x] Product Owner fixed one Catalog-wide recursive produced-reference
      field-path contract such as `items[].runtime_ref` and
      `runtime.revisions[].revision_ref`. WP08 owns its implementation; WP03
      consumes it and adds no task-specific alternative.

## Thesis evidence

- Repeated decision: predecessor Context and policy mutations already require stable
  identity, lifecycle locking, read-before-retry, and exact target binding.
  Runtime retirement needs the same invariant rather than a Docker exception.
- Repeated friction: Runtime build, predecessor Context selection, Workspace applied state,
  Docker image availability, and disk cleanup currently answer related facts in
  different layers. A caller would have to inspect source or Docker to close
  the task.
- Code workaround being rejected: scan `tobari-runtime-*` tags and call Docker
  prune/remove. It would infer authority from presentation names, miss Manifest
  and Workspace semantics, and cross an uncontrolled global boundary.
- Current thesis resolution: the public CLI closes user tasks, actions consume
  opaque references unchanged, mutations declare complete impact, and semantic
  identity is validated before presentation or side effects.
- Durable propagation required: ADR 0067 and the Runtime sections of theses,
  product, architecture, security, harness, catalog capability/schema fixtures,
  agent-readiness scenarios, and release/public generated data.

## Security and public-boundary notes

- Assets and side effects: private Runtime source/snapshots/manifests, private
  journals/receipts, Manifest/Workspace binding evidence, exact Docker image
  tags/content, and bounded Docker build/inspect/remove calls.
- Credentials/confidential data: none are read or emitted. Workspace homes,
  native credentials, broker state, project roots, private Dockerfile content,
  and raw Docker output are protected non-targets.
- New state: owner-only schema-1 build and retirement journals/receipts; no
  external destination, process class, dependency, or generated asset.
- Delivery: reads and writes are complete. Prune dry-run has exhaustive coverage
  of the exact local Runtime-owned retirement scope at its observation point.
  No pagination is proposed for V1.
- Timeout/cancellation: honor one propagated context before each pre-commit
  step; after a destructive step, preserve journal/structured outcome and do
  not translate cancellation into safe replay. Confirmed result outranks later
  cancellation or output failure.
- Retry/idempotency: stale plans make no change; started exact actions resume;
  confirmed exact actions return receipts. Unknown results require read-only
  reconciliation before retry.
- Publication/licensing: no external content is embedded. Restore repeats the
  existing user-authored Docker build boundary and does not add publication
  rights or a remote registry promise.

## Cross-packet dependencies and conflicts

- ADR 0079 and integrated `docs/00` through `docs/04` are the confirmed upper
  decision and hard implementation dependency. This packet consumes Workspace
  Manifest / Workspace identity,
  exact desired/applied/observed/failure separation, and independently
  immutable subordinate Runtime revisions. Runtime lifecycle implementation and
  migration must follow the integrated durable implementation rather than ship
  a parallel Context reader or dual vocabulary.
- ADR 0079 and the durable product/security/harness contracts own
  `runtime create --copy-source-from`, removal of `--base`, and no-lineage copy
  semantics. Runtime retirement must not retain copy lineage or create a parent
  deletion constraint.
- `status-home` owns routine desired/applied Runtime presentation. It must share
  lifecycle state names and last-used/applied evidence without making status
  perform cleanup or installation-wide Docker scans.
- `first-use-progress-recovery` owns Workspace-entry progress/cancellation. It
  must surface missing/unrestorable selected Runtime material through the same
  causal recovery vocabulary, not build or prune implicitly.
- The completed WP08 contracts in [Architecture](../../02_architecture.md) and
  [Harness](../../04_harness.md) govern nested Runtime lifecycle output.
  Runtime schemas and Catalog declarations must consume that enforcement and
  must not add a parallel validator.
- `first-public-release-core` and `first-public-release-artifacts` currently
  describe the retained Runtime surface without retirement. This lifecycle is a
  V1 scope dependency; their catalog/schema/capability/generated locks must be
  regenerated only after integration.
- No direct conflict exists with authentication narrowing or capability
  profiles. Runtime deletion must preserve Workspace-owned credentials and the
  built-in standard Runtime/image regardless of compile-time profile.

## Glossary

- **Managed Runtime:** installation-owned aggregate containing one mutable
  source tree and ordered immutable successful revisions.
- **Runtime revision:** stable Runtime ID plus semantic source digest; ordinal is
  human ordering, not authority.
- **Source snapshot:** immutable owner-only copy of the exact source bytes/modes
  used for one build. It is not an independently selectable public resource.
- **Image material:** local Docker execution content and exact Tobari-owned tag
  associated with a successful revision or confirmed build candidate.
- **Build candidate:** a loaded image produced by a journaled build attempt that
  did not become an authoritative successful revision.
- **Protection:** an exact current Manifest binding, retained immutable Manifest
  revision binding, pending desired adoption, durable last-successful applied
  need, or exact container observation that prevents image pruning or Runtime
  deletion.
- **Prune plan:** deterministic, opaque, read-only review identity for one exact
  candidate set and protection snapshot.
- **Retirement receipt:** owner-only durable evidence that an exact destructive
  target completed, used only for idempotent recovery.
- **Logical bytes:** exact file lengths under one validated private tree; not
  filesystem allocation or Docker reclaimable space.
