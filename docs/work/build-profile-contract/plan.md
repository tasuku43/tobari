# Work Plan: Release and research build boundary

- Status: Accepted
- Decision state: Fixed by Product Owner; implementation not started
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Adopt **capability surface** as the concept and exactly two values:
**release surface** and **research surface**. Remove `profile` from this
boundary. Keep **resolver channel** as the separate existing image-authority
axis because `task build` proves that a development resolver can correctly
carry the release surface.

The release surface is the only supported/public product surface. The research
surface is a repository-only closed superset used to study the existing Auth
Broker and Operator Console. It is not a selectable tier, compatibility promise,
download, or release candidate.

The Workspace Manifest model and one-time-copy semantics are durable upstream
contracts in [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
and `docs/00_theses.md` through `docs/04_harness.md`, with authentication and
migration consequences in `docs/07_authentication.md`. Integration evidence is
recorded at `07535a9` and `428812f`. Capability surface is executable identity
outside the three-resource budget; it is not a Workspace Manifest activation
slice or persisted desired/applied fact. No build-surface production slice,
including pure types or tests, begins until WP08 and WP03 complete and the
post-WP03 implementation-start gate re-fetches and re-observes the actual
HEAD/worktree, Catalog, schemas, build tags, Taskfile, release scripts, helper
snapshot, site, and tests. Catalog/schema/site integration consumes the landed
`manifest`/Workspace contract and must not implement or alias the predecessor
Context/Project model.

The physical prerequisite order is fixed:
WP01+WP02 completion audit -> WP08 Catalog/domain output conformance -> WP03
Runtime retirement -> WP04. WP04 therefore consumes WP02, WP08, and WP03 as
completed implementation on the actual post-WP03 HEAD. Their commands and
recursive reference contracts are common non-research capabilities in both
executable surfaces, never part of the auth/serve research delta.

### Canonical build matrix

| Producer | Capability surface | Resolver channel | Canonical executable | Publication status |
|---|---|---|---|---|
| protected release packager | `release` | `embedded` | `tobari` | the only publishable executable |
| `task build` | `release` | `development` | `bin/tobari` | repository feedback only |
| `task build:dev` | `research` | `development` | `bin/tobari-research` | repository research only |

`research + embedded` is not an admitted tuple. Research build-constrained
implementations require both `tobari_dev` and the renamed `tobari_research` tag;
using `tobari_research` without the development resolver leaves no valid
compiled surface implementation and fails the build. The old
`tobari_experimental` tag is removed from active source and tooling without a
compatibility alias. An old invocation therefore cannot widen capability; at
most it produces the release surface or fails an exact gate.

`task build:dev` remains the only contributor research Task path. No
`build:research` alias is added.

This model has two internal capability values and two resolver values, but only
three admitted build tuples. The public user learns one supported surface. The
five research commands remain one indivisible research set rather than five
feature flags.

## Alternatives considered

### Keep standard/experimental capability profiles and improve prose only

Rejected. It leaves `profile` colliding with harness, AWS, and agent concepts;
keeps `experimental` looking like a supported channel; and allows executable
identity, Task output, Catalog usage, and documentation to drift again.

### Rename only experimental to research

Rejected. `standard profile` would still collide with `standard@1`, imply a
runtime-selectable profile, and fail to distinguish the supported capability
surface from the development resolver used by `task build`.

### Use release build versus development build as the only pair

Rejected. `task build` is a repository/development-resolver build with the
release command surface, so the two ideas are demonstrably orthogonal. It would
also confuse SemVer development prereleases with research capability.

### Add a feature flag or build tag for each research command

Rejected. The five commands do not have independent publication or support
lifecycles. Combinatorial surfaces would weaken the closed trust-boundary proof
and invite runtime widening without a separate user outcome.

### Remove all Broker research source immediately

Rejected for this change. Authentication behavior is explicitly out of scope,
and the repository still uses the research path for bounded experiments. This
packet makes its ownership and non-public status honest; capability retirement
would need its own decision and state/dependency evidence.

## Design

### Owner, scope, lifetime, and mutability

- **Owner:** domain owns the closed surface vocabulary and valid build tuple;
  CLI owns Catalog projection and canonical presentation identity;
  infrastructure owns resolver and topology implementations; release tooling
  owns publishable tuple enforcement; documentation owners own IA projection.
- **Scope:** one compiled executable and every Catalog/topology action reachable
  through it. Surface does not belong to a Workspace Manifest, Manifest
  revision, Runtime, Runtime revision, Workspace, provider, user, configuration
  file, Docker image, migration record, or invocation.
- **Lifetime:** creation at Go build time through reviewed build constraints;
  destruction when the executable is replaced.
- **Mutability:** immutable. No in-place transition exists. To obtain another
  surface, build or install a different explicitly named executable.

Workspace Manifest revision publication, explicit `cluster up`, explicit
Workspace entry, new-child-session defaults, and new-Workspace creation defaults
retain their distinct ADR 0079 activation rules. None is a lifecycle
event for capability surface, and neither read-only observation nor a blocked
attached-Workspace adoption can widen or reconcile that surface.

### Authority and presentation identity

- Authority is `capabilitysurface.Compiled()` plus validation of its allowed
  resolver tuple. A filename, `argv[0]`, version label, help text, environment,
  state, or Docker metadata is evidence/presentation, never authority.
- Release presentation uses canonical program `tobari`. Research presentation
  uses canonical program `tobari-research`, even if either executable is copied
  or renamed. This prevents `argv[0]` from becoming a selector.
- Root human help, agent-help `program`, invocation templates, scoped usage,
  machine invocation examples, version text, and recovery commands derive from
  the compiled surface's canonical presentation identity.
- Build identity keeps resolver channel separate and replaces public
  `capability_profile` with `capability_surface`.

### Public and internal concepts

- **Public:** `tobari` and its release surface. Users are not asked to choose a
  surface. Native Workspace authentication is part of this single product.
  Capability surface is reported executable identity, not a fourth durable
  resource beside Workspace Manifest, Runtime, and Workspace.
- **Contributor-only:** the repository research build, `task build:dev`, and
  `bin/tobari-research`. They appear in a clearly separated README contributor
  note, numbered research appendix, ADR, and harness documentation, never in
  install, Quick Start, standard troubleshooting, standard architecture IA, or
  generated public CLI reference.
- **Internal:** `CapabilitySurface`, `release|research`, resolver channel,
  build constraints, admitted tuple validation, closed research command set,
  and research topology.
- **Unrelated retained concepts:** verification profiles, AWS profiles, and
  agent profiles. Documentation and lint must distinguish their semantic use;
  a global ban on the word `profile` would be incorrect.
- **Accepted upstream vocabulary:** final Catalogs, schemas, examples, and
  research evidence use Workspace Manifest/Manifest, `manifest`, `--manifest`,
  `workspace_manifest_id`, Workspace, and `workspace_id`. This packet does not
  expose Context/Project aliases or turn a Manifest into project-owned YAML,
  generic import, or `apply -f`.

### Public contract and Catalog

No supported command is added or removed by the build-surface decision.
Existing roles, effects, opaque reference flow, authentication behavior, and
Operator Console behavior remain unchanged. Input/output/recovery spellings are
not frozen to predecessor Context fields: they consume ADR 0079's final
Manifest/Workspace Catalog contracts after that packet performs the atomic
pre-public cutover.

The executable matrix is a set relation, not an absolute command count:

```text
common Catalog   = actual integrated non-research Catalog
                   after completed WP01/WP02/WP08/WP03

release Catalog  = common Catalog
research Catalog = common Catalog
                 + auth login
                 + auth import
                 + auth status
                 + auth logout
                 + serve
```

The release Catalog has no `auth` namespace. `help auth`, bare `auth`, every
exact auth path, and `serve` fail as unknown before application or
infrastructure I/O. The research Catalog contains all five complete existing
outcomes, expressed through the final `--manifest` and
`workspace_manifest_id` contracts, and no other path delta. The site generator
continues to build a private no-research-tag binary and must additionally assert
that the generated root is release-surface before accepting its Catalog.

From completed WP02, `manifest create --copy-from` and
`runtime create --copy-source-from` are present in both surfaces, and their
one-time-copy inputs cannot select a build surface or copy authentication,
learned permission, attachment, applied/failure/observed, or selection state.
From completed WP03, `review runtimes`, reference-bound `runtime build --id`,
`runtime delete --id ... --confirm=delete`, `runtime prune dry-run`,
`runtime prune apply --plan ... --confirm=prune`, and `runtime restore --id`
are also present in both surfaces. Their Runtime,
revision, prune-plan, protection, journal, receipt, and observation data cannot
activate `auth` or `serve`. Recursive produced-reference derivation remains one
Catalog-wide invariant rather than a Runtime-specific build-surface exception.

Known-path discovery remains one scoped help invocation. Unknown outcomes stay
within root plus scoped help, at most two invocations. Surface discovery and
help require zero external processing or provider I/O.

### Authentication explanation and information architecture

Use a staged explanation rather than a side-by-side product chooser:

1. **Supported outcome:** a tool performs native login inside its Workspace and
   owns credential state in that Workspace home. Gateway redacts auth/cookies
   from policy/audit and forwards them only after the ordinary effect is allowed.
   Native state is not Workspace Manifest desired/applied state.
2. **Boundary clarification:** Tobari does not inherit host credentials, and
   the release executable has no provider binding, projection, handle, vault,
   root key, companion, Auth Broker, or `auth` command.
3. **Contributor research appendix:** only after the supported journey is
   complete, explain that `task build:dev` compiles the repository research
   surface and three-service topology. Every command is shown as
   `bin/tobari-research auth ... --manifest <name>` or
   `bin/tobari-research serve` with an adjacent unsupported/non-release warning.
   Any research credential store is described as separately Auth Broker-owned
   and scoped by `workspace_manifest_id`, never as a Manifest revision field or
   standard Manifest responsibility.

The standard architecture site contains stages 1 and 2 only. Broker sequences,
provider matrices, vault/state references, research topology, research faults,
and research commands move out of its navigation, search corpus, learning path,
generated references, glossary, diagrams, and routine troubleshooting. Their
durable authority remains the numbered repository authentication/security docs,
the ADR, code, and tests. The site must not maintain an unlinked but searchable
second product surface.

### Human, JSON, and state migration

- Human `version` changes “Capability profile” to “Capability surface” and
  prints `release` or `research`. Resolver channel remains a separate row.
- Final public version JSON remains schema V1 under ADR 0079's
  pre-public reset and replaces
  `capability_profile: standard|experimental` with
  `capability_surface: release|research` while independently reporting
  `resolver_channel: embedded|development`. It emits no predecessor field or
  value and accepts neither axis as runtime input. It introduces no dual field,
  compatibility reader, alias, or schema-2 surface.
  Existing prerelease automation must update by the final-V1 reset; release
  notes and exact predecessor evidence name the break.
- Research development recovery returns `task build:dev` and
  `bin/tobari-research`. Release-surface repository recovery remains
  `task build`/`bin/tobari`. Embedded release output keeps both fields empty.
- Build-surface state is not persisted. ADR 0079 separately owns the
  breaking Context/ProjectInstance to Workspace Manifest/Workspace schema and
  exact migration. This packet neither implements that migration nor claims
  that current Context-named research state remains byte-compatible after it.
- Standard Workspace authentication state is neither read nor migrated into
  Workspace Manifest desired/applied state. Learned permission and attachment
  authority remain separately owned even when their final identities use
  WorkspaceManifestID/WorkspaceID.
- Research Broker predecessor state is not migrated in place. The new research
  surface rejects legacy handles and requires explicit login/import under the
  new Manifest identity. UUID byte preservation makes the predecessor
  `config/auth/contexts/<UUID>` lookup collide with the new
  WorkspaceManifestID reader key, so leaving experimental authority in place is
  itself an implicit rebind and is forbidden. WP01 preserves standard Workspace
  homes while atomically moving the complete replay-capable filesystem research
  set—ciphertext, bindings, handles, lookup indexes, projections, provider
  state, and Linux filesystem root-key material—into owner-only private backup/
  quarantine. Normal old and new readers cannot discover it.
- On macOS, `migrate apply` leaves the Keychain root-key item untouched: it does
  not read, copy, rotate, or delete it. Once every dependent filesystem
  authority path is quarantined, that item alone is inert recovery material.
  Rollback restores exact filesystem bytes and reuses the unchanged item; it
  fails closed if fresh canonical auth state exists. No automatic Keychain
  cleanup occurs. Any future explicit cleanup must prove that no live or
  quarantined ciphertext depends on the item.
- WP01 owns synthetic migration, journal, rollback, platform-specific root-key,
  and secret-free evidence. WP04 consumes it and, after the build-surface
  change, re-verifies old/new-reader and legacy-handle denial, release Broker
  absence, research quarantine non-discovery, and explicit new-Manifest
  login/import. Insufficient Docker or secret evidence cannot reopen migration
  or justify an inferred binding.
- `bin/tobari-dev` is an unmanaged repository artifact. The build does not
  overwrite, adopt, or silently delete it. Documentation instructs contributors
  to rebuild and may give an explicit manual cleanup note; it never executes a
  destructive migration.
- No `tobari_experimental` or `bin/tobari-dev` compatibility alias remains.
  Because this is a prerelease-only contributor surface, fail-closed removal is
  preferred to a second vocabulary path.

### Layer changes

- **Domain:** replace `capabilityprofile` with `capabilitysurface`; define the
  closed enum, canonical program identity, valid resolver/surface tuples, and
  surface-aware build recovery. Keep the domain free of argv, environment,
  filesystem, and process inspection.
- **Application:** no auth or Operator Console use-case behavior changes.
  Under ADR 0079, those use cases consume Manifest/Workspace identity;
  this packet does not perform that rename. Application packages continue to
  know task contracts, not build selection.
- **Infrastructure:** rename build constraints and research image/topology
  presentation. Preserve the exact two-service release and three-service
  research trust boundaries, Compose inputs, socket isolation, provider plans,
  and resolver behavior.
- **CLI and Catalog:** project the closed compiled surface into one Catalog;
  derive canonical program usage and recovery from it; preserve the five
  complete research outcomes on top of the actual integrated common Catalog.
  Consume completed WP01/WP02/WP08/WP03 structured reference metadata without
  inventing a second registry or Runtime-specific validator. Do not preserve
  Context aliases or introduce a runtime constructor parameter that ordinary
  CLI input can choose.
- **Composition root and tooling:** `task build` stays release-surface with
  development resolver. `task build:dev` uses
  `tobari_dev tobari_research`, `--research`, and
  `bin/tobari-research`. The release packager remains fixed to the release
  surface and accepts no tags.

### Data and control flow

```text
reviewed Go build inputs
  -> build constraints select one immutable CapabilitySurface
  -> resolver constructor selects embedded or development authority
  -> build identity validates the admitted tuple
  -> CLI composes exactly one Catalog and canonical program identity
  -> infrastructure composes exactly one service topology
  -> version/help expose evidence of the same authority

runtime argv/env/config/state/image name/argv[0]
  -> cannot enter the surface-selection path

Workspace Manifest desired/applied/observed/failure records
Manifest/Runtime copy inputs and independent copied identities
Runtime source/revisions/retirement/prune/restore/protection/sidecar records
Workspace/migration records
  -> remain runtime data and cannot enter the surface-selection path
```

### Error and cancellation behavior

- An invalid or unimplemented surface/resolver tuple fails at build or identity
  validation before Catalog dispatch, Docker, browser, file, credential, or
  network effects.
- Release direct attempts to invoke `auth` or `serve` remain deterministic
  unknown-command failures with existing exit mapping and recovery to root help.
- Existing auth mutation outcome, cancellation, reconciliation, retryability,
  and mutation-complete behavior are unchanged in research builds.
- Existing `serve` cancellation and loopback cleanup behavior are unchanged.
- Copying or renaming a binary changes neither behavior nor canonical help
  identity; there is no basename error or activation path.

### Security and public boundary

The release trust boundary does not change: Workspace-owned native credentials,
Gateway, OPA, Docker/host infrastructure, and the reviewed native browser bridge
remain trusted as already declared. The research boundary retains Auth Broker,
vault/root-key handling, provider acquisition, and the experimental Gateway
layer exactly as today. This change reduces the presentation risk that users
mistake research code for a supported runtime mode.

Workspace Manifest remains trusted desired declaration authority, but it owns
no capability-surface selector and no native authentication, learned permission,
or attachment state. The research Auth Broker may use the retained
WorkspaceManifestID only for new-surface research state created by explicit
login/import. Retained predecessor UUID bytes do not authorize legacy-handle
acceptance, automatic vault rebinding, or secret migration and do not make
experimental vaults or handles part of the standard Manifest aggregate.

No new credential shape, destination, external API, dependency, executable
adapter, or secret fixture is introduced. Release proof concerns reachability
and topology; it does not overclaim byte-level absence of dead source.

The durable security vocabulary is:

- **Research authentication authority:** the complete replay-capable
  combination of credential ciphertext and filesystem bindings, handles,
  lookups, projections, and provider state that make it reachable.
- **Inert recovery material:** secret-bearing material with no normal
  reachability or binding path and therefore no ability to authorize or resolve
  an operation. The unchanged macOS Keychain item qualifies only after atomic
  quarantine of every dependent filesystem authority record.

This distinction is promoted to the ADR and security model. It is not a macOS
exception or permission to infer authority from possession of key material.

### Mistaken-release prevention

Use positive executable evidence rather than only source greps:

1. The release workflow has no tags/profile/surface input and invokes only the
   fixed packager.
2. The packager clears ambient Go flags and builds without `tobari_research`.
3. `go version -m` for every archive must contain no research tag.
4. A host-runnable archive must report final schema V1 with release surface,
   embedded resolver, no Broker API fields, and no contributor recovery.
5. That executable's root human and agent help must omit the auth namespace and
   serve; scoped/direct negative probes must fail before I/O.
6. The generated public Catalog must byte-correspond to the release executable's
   agent help and pass the same absence checks.
7. The archive inventory admits only canonical executable `tobari` and reviewed
   supporting files; it rejects `tobari-research`.
8. Hostile ambient `GOFLAGS=-tags=tobari_research` evidence must still produce
   release identity or fail before creating an archive.

## Implementation slices and dependency order

1. **Preserve the shared checkout:** during upstream synchronization, change
   only this packet. Do not touch integrated or concurrently owned domain,
   application, infrastructure, CLI/Catalog, schema, helper-snapshot, site,
   build-tag, Taskfile, release, generated, or test files.
2. **Complete prerequisites in fixed order:** consume the integrated WP01+WP02
   durable contracts and evidence, including atomic experimental-authority
   quarantine evidence; then wait for WP08 completion; then wait for WP03
   completion. WP04 does not overlap or partially reproduce any of those
   implementations.
3. **Post-WP03 implementation-start gate:** fetch `origin/main`; record the
   actual HEAD and worktree ownership/status; inspect the integrated commits/
   diff and reread every affected governing contract and implementation
   surface. Rebuild bounded observation binaries and derive fresh Catalog,
   recursive reference, schema, identity, help, topology, release, and site
   evidence. If a stable release or another fixed-decision contradiction is
   observed, stop and notify control thread
   `01a02c51-885b-7b80-a66f-05850f48ba4d` with `WP04_BLOCKED`.
4. **Build-surface ADR and contracts:** promote the fixed build-surface decision,
   revise capability terminology, propagate consequences through numbered
   product/architecture/security/harness/release/auth/readiness contracts, and
   resolve conflicts with Active release/auth packets without reopening Domain
   Model V1.
5. **Failing executable matrix:** add one shared path-set oracle and failing
   release/research build tests for identity, human help, agent help, scoped
   help, direct dispatch, topology, and non-activation from Manifest/Runtime/
   Workspace/migration/copy/lifecycle data. Derive the common set from the
   integrated Catalog; do not encode the historical 40/45 totals.
6. **Domain identity:** introduce capability-surface vocabulary, admitted tuple
   validation, canonical program identity, final schema-V1 field reset, and
   surface-aware recovery.
7. **Catalog and presentation:** on the completed WP01/WP02/WP08/WP03 Catalog,
   rename
   research build constraints, preserve the exact five-command path delta, and
   make all research usages consistently name `tobari-research` and
   `--manifest` without using `argv[0]` as authority. Keep all common Manifest,
   Runtime, Workspace, output, and recursive reference contracts identical
   across both surfaces.
8. **Infrastructure and build tooling:** only after the ownership gate, rename
   topology/image constraints and
   Task inputs/output; update canonical helper snapshots through their checked
   sync path; retain authentication mechanics unchanged.
9. **Fixed research-state verification:** consume WP01's synthetic atomic
   quarantine, journal, rollback, platform-root-key, and secret-free evidence.
   Prove the normal old/new research readers cannot reach predecessor material,
   legacy handles fail, Linux filesystem key material moves with the complete
   set, macOS migration performs zero Keychain operations, rollback restores
   exact bytes and fails on fresh canonical auth state, standard Workspace homes
   persist, and explicit new-Manifest login/import creates the only new research
   authority.
10. **Release and site enforcement:** add artifact-level absence checks, hostile
   ambient-tag canaries, release-Catalog site generation proof, and remove
   research concepts from standard site IA in both locales.
11. **Documentation and migration handoff:** update README and all numbered docs,
   add the final-V1 JSON/binary rename note, regenerate from one reviewed
   Manifest-era snapshot, and run all gates. Runtime examples use
   completed `--copy-source-from` and `runtime build --id` contracts, run all
   gates and archive inspection, commit the implementation, then notify the
   control thread with `WP04_IMPLEMENTATION_COMPLETE`.

Each slice must leave the default release surface buildable. A slice may not
temporarily make research runtime-selectable or publishable.

## Verification

- **Domain/unit tests:** both surface values validate; unknown values and
  research+embedded fail; canonical program and recovery matrix is exact;
  no runtime parse/constructor path accepts surface input.
- **Catalog contract tests:** release path set has no auth namespace or serve;
  research minus release is exactly five and release minus research is empty;
  the common set is derived from the integrated Catalog rather than a 40/45
  total; completed WP01/WP02/WP08/WP03 paths and nested reference contracts are
  identical across surfaces;
  every research spec retains its role/effect/behavior while using ADR 0079's
  final Manifest/Workspace input, output, and recovery vocabulary.
- **Human and agent help:** root, namespace, exact path, usage, program,
  invocation template, machine examples, and negative unknown-command behavior
  agree for both binaries.
- **Runtime/topology tests:** release selects only Gateway/OPA and one Compose
  file with zero Broker calls; research selects the existing three services and
  override. Environment/config/state/image/basename, Workspace Manifest
  revision, Runtime revision, Workspace record, migration, copy, Runtime
  lifecycle, protection, journal/receipt, and sidecar canaries cannot widen the
  surface.
- **Release tests:** every archive metadata record lacks research tags and
  research executable name; host artifact identity/Catalog negative probes;
  hostile `GOFLAGS`; workflow has no selector; exact archive inventory.
- **Site tests:** generation uses a release-surface executable; public Catalog
  has no research paths; both locales' standard IA, navigation, search data,
  diagrams, glossary, examples, and troubleshooting contain no research
  surface. Native authentication journey remains complete.
- **Compatibility tests:** final schema-V1 version fixture with the replaced
  field/value vocabulary, exact predecessor rejection, and no dual field or
  schema-2 output; WP01 synthetic quarantine/journal/rollback evidence;
  old/new predecessor reader and legacy-handle denial; Linux root-key movement;
  zero macOS Keychain operations; exact rollback and fresh-state conflict;
  explicit new-Manifest login/import; no standard credential read and no
  automatic encrypted-state, Keychain-item, or old-binary deletion.
- **Security tests:** standard zero Broker inspection/control/startup and no
  research image resolution; existing research secret-free, deny-before-
  resolution, post-allow, and vault tests remain unchanged and pass.
- **Agent readiness:** release agent help discovers no Broker capability;
  research optional journey begins with exact `bin/tobari-research` argv; known
  and unknown discovery budgets and zero external-processing count remain met.
- **Required profiles:** `task check`, `task security`, `task public:check`, and
  `task release:check`; integration/runtime research gates when build constraints
  or topology files change; release archive inspection for every produced
  target.
- **Generated evidence:** architecture-site Catalog/component references and
  product snapshot advance together from the reviewed implementation commit;
  no dirty-tree or research-Catalog leakage.

## Rollout and rollback

Ship the change in a new prerelease. Do not alter prior tags or artifacts.
Release notes name the contributor binary rename, build-tag rename, and version
JSON schema-V1 field/value reset. Standard commands retain their build-surface
behavior, while ADR 0079 owns the separate intentional public/persisted
schema reset. Predecessor experimental authority remains quarantined and
unreadable; research users explicitly login/import again under a new Manifest
identity. The old binary path and tag receive no alias.

Rollback is a new source revision that restores the prior implementation only
after checking that it does not reintroduce a supported-variant presentation or
weaken release absence evidence. Never overwrite an immutable release asset.

## Packet dependencies, conflicts, and cross-cutting review

- **Supersedes in part:**
  `capability-profiles-first-prerelease` vocabulary and profile-oriented
  evidence. Preserve its compile-time/no-runtime-flag decision and release-only
  publication boundary.
- **Fixed prerequisite order:** complete the WP01+WP02 audit, including WP01's
  experimental-authority quarantine evidence; complete WP08 Catalog/domain
  output conformance; complete WP03 Runtime retirement; then run the post-WP03
  HEAD/worktree observation gate before any WP04 production slice.
- **Fixed design integration:** ADR 0079 and `docs/00_theses.md` through
  `docs/04_harness.md` supply `--copy-from`, `--copy-source-from`, no `--base`,
  fresh independent identity, no lineage/state copying, and no reconciliation;
  integration evidence is `07535a9` and `428812f`. WP08 supplies Catalog-wide
  recursive output/reference derivation. [ADR 0080](../../decisions/0080-close-the-managed-runtime-lifecycle.md)
  supplies reference-bound build/delete/restore, plan-bound prune, read-only
  Runtime Review, protection-graph semantics, owner-only sidecars, and
  the completed lifecycle surface. WP04 consumes all of them as the common
  Catalog rather than the research delta.
- **Evidence ownership:** WP04 owns the executable surface/absence/mistaken-
  publication matrix and release archive inspection. Existing release packets
  consume this evidence and do not create a competing authority.
- **Depends on:** authentication/security owners for wording review that changes
  no auth mechanics; architecture-site owners for one-snapshot regeneration.
- **Conflicts with:** the Active `first-public-release-core` and
  `v1-auth-narrowing` goals where they still describe Broker commands or an Auth
  Broker as public V1. Those goals must be revised, superseded, or closed before
  this packet can complete.
- **Observation-only review:** verify no stable release, inventory known
  prerelease consumers and old contributor automation, inventory exact authored
  research-site targets and stale Active packets, and consume WP01's quarantine
  plus completed WP02/WP08/WP03 evidence. A contradiction blocks through the
  control thread; it does not authorize a runtime alias, dual compatibility,
  research secret migration, or fixed absolute Catalog count.

## Documentation promotion

- `docs/00_theses.md`: release/research surface vocabulary and native-first
  consequence.
- `docs/01_product_contract.md`: sole supported surface, public/internal concept
  boundary outside the three-resource budget, Catalog delta, binary identity,
  Manifest/Runtime non-activation, and compatibility.
- `docs/02_architecture.md`: capability surface versus resolver channel,
  ownership/lifetime, admitted tuples, and topology derivation.
- `docs/03_security_model.md`: immutable compile-time authority, standard native
  boundary, research Broker boundary, authentication authority versus inert
  recovery material, quarantine reachability, and no runtime widening.
- `docs/04_harness.md`: executable matrices, exact delta, artifact/site
  evidence, and profile-word disambiguation.
- `docs/05_public_repository.md` and `docs/06_release.md`: publishable inventory,
  mistaken-release prevention, final schema-V1 field reset, and no research
  artifact.
- `docs/07_authentication.md` and `docs/08_external_api_contracts.md`: native
  first, Workspace-home ownership outside Manifest desired/applied state,
  repository research appendix with Manifest-era identity, renamed build
  references, and unchanged provider mechanics.
- `docs/09_agent_readiness_validation.md`: release absence and exact research
  binary evidence without bare `tobari auth`.
- README, architecture site, capability ledger, and the new ADR: same canonical
  names and separation. Temporary conclusions do not remain solely here.
