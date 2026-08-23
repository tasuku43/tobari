# Work Goal: Make the release and research build boundary unambiguous

- Status: Accepted
- Decision state: Fixed by Product Owner; production implementation not started
- Implementation readiness: Planned after the fixed WP01+WP02 audit (durable
  upstream integrated) -> WP08 -> WP03 -> WP04 dependency sequence and the
  post-WP03 observation gate
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md` through `docs/04_harness.md`, plus `docs/06_release.md` through `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the vocabulary, build identity, Catalog boundary, documentation information architecture, and executable release evidence are promoted and the change completes
- Successor: None
- Owner: Product, domain, security, documentation, and release maintainers
- Target: A prerelease before the first stable V1
- Prerequisite: audit the integrated Workspace Manifest and one-time-copy
  contracts in [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md),
  `docs/00_theses.md` through `docs/04_harness.md`, and
  `docs/07_authentication.md` (implementation evidence `07535a9` and
  `428812f`), then complete [Catalog/domain output conformance](../catalog-domain-output-conformance/goal.md)
  (WP08), then complete [Runtime Retirement](../runtime-retirement/goal.md),
  then rebaseline the actual post-WP03 HEAD/worktree before WP04 production work
- Related ADRs: ADR 0044, [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md), and the future build-surface vocabulary ADR
- Related work: the durable Workspace Manifest and one-time-copy contracts in [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md) and `docs/00_theses.md` through `docs/04_harness.md`, [Catalog/domain output conformance](../catalog-domain-output-conformance/goal.md), [Runtime Retirement](../runtime-retirement/goal.md), [Capability profiles and first prerelease](../capability-profiles-first-prerelease/goal.md), [First public release core](../first-public-release-core/goal.md), [Release artifacts](../first-public-release-artifacts/goal.md), and [V1 authentication narrowing](../v1-auth-narrowing/goal.md)

## Outcome

Tobari presents one supported **release surface** and one repository-only
**research surface** without calling either a profile. The protected release
artifact and `task build` expose the same release Catalog and two-service
Gateway/OPA topology. `task build:dev` produces the distinctly named research
binary and adds exactly four Auth Broker commands plus `serve` to the complete
post-integration common Catalog. Workspace Manifest, one-time-copy,
Catalog/reference-conformance, and Runtime lifecycle contracts completed by
WP01, WP02, WP08, and WP03 belong to that common Catalog in both surfaces; they
are not research capability. The executable's compiled surface
is immutable for its lifetime and cannot be widened by argv, environment,
configuration, persisted state, image contents, or any other runtime input. A
capability surface is executable identity, not a fourth
durable public resource and not Workspace Manifest desired, applied, observed,
or failure state. No Workspace Manifest revision, Runtime revision, Workspace
record, or migration input can expand it.

The standard user journey explains Workspace-owned native authentication first
and does not expose the Auth Broker as an installable, supported, or selectable
variant. Standard authentication state remains in the Workspace home and is
excluded from Workspace Manifest desired/applied state. Repository research
documentation remains available to maintainers, but every research command
example names the research binary and final `manifest` vocabulary explicitly.

## Why now

The current implementation has the intended compile-time safety boundary, but
uses `standard`, `experimental`, `development`, `research`, and `profile` for
overlapping ideas. `profile` already denotes verification profiles, AWS
profiles, and agent profiles. The published architecture site's standard
information architecture still contains Broker diagrams and bare `tobari auth`
examples, while the release Catalog correctly omits those commands. The current
research binary also reports the release build recovery command and binary name.
These presentation inconsistencies can make the repository experiment look
like a supported runtime-selectable variant even though the code correctly
prevents runtime activation.

## Non-goals

- Change native Workspace authentication, any Auth Broker provider, vault,
  handle, acquisition, refresh, signing, redaction, or post-policy behavior.
- Add, remove, split, or independently activate any of the five current
  research-only commands.
- Add a runtime feature flag, environment selector, configuration key, command
  input, Workspace Manifest field/revision, provider manifest field, Runtime
  field/revision, or persisted surface selector.
- Publish the research binary, Auth Broker, experimental Gateway layer, or any
  Tobari-owned OCI image.
- Claim that unreachable research source bytes are absent from every release
  binary; the contract is Catalog, topology, resolver, and executable behavior.
- Rename unrelated uses of `profile` that denote real independent concepts,
  including harness verification profiles, AWS profiles, and agent profiles.
- Independently redesign command roles, effects, fault semantics, or auth and
  Operator Console behavior. Their final inputs, outputs, recovery, and schema
  vocabulary must consume ADR 0079's `manifest`, `--manifest`,
  `workspace_manifest_id`, and `workspace_id` cutover rather than preserve the
  current Context/Project predecessor vocabulary.
- Reimplement or revise the durable Context/ProjectInstance to Workspace
  Manifest/Workspace migration, or edit any of its production/schema/Catalog/
  migration evidence as part of this packet. WP04 consumes ADR 0079 and the
  integrated implementation and must not create aliases or a second migration
  path.
- Implement the fixed WP02 one-time copy semantics or WP03 Runtime retirement
  lifecycle. Their contracts constrain the shared Catalog and documentation
  integration but are not implementation authorization for this packet.
- Implement WP08 Catalog/domain output conformance. WP04 consumes its completed
  Catalog-wide nested reference derivation and adds no competing validator.
- Freeze an absolute total command count such as the historical 40/45. The
  acceptance authority is the Catalog set relation after completed
  WP01/WP02/WP08/WP03 work, not a stale count captured from the old main
  baseline.
- Migrate, decrypt, rebind, or automatically delete predecessor experimental
  Broker authority into the new surface. WP01 may atomically quarantine its
  complete filesystem set; WP04 does not treat quarantine or a macOS Keychain
  item as login state. Research secret continuity is deliberately not attempted.
- Publish a release, create a tag, migrate user state, or implement any part of
  this packet during packet preparation.

## Acceptance criteria

- [ ] Numbered contracts and one accepted ADR use **release surface** and
      **research surface** as the sole capability-boundary vocabulary;
      `standard profile`, `experimental profile`, `development profile`, and
      `capability profile` no longer name that boundary.
- [ ] Before implementation, the WP01+WP02 audit, WP08, and WP03 are complete,
      and a fresh read-only gate records fetched `origin/main`, actual
      post-WP03 HEAD, worktree status, relevant integration commits/diff,
      Catalog path sets and recursive reference contracts, schemas, build
      matrix, build tags, Taskfile, release scripts, and generated/site state.
      Historical 40/40/45 evidence is not reused as current authority, and no
      upstream or concurrently owned shared-checkout file is overwritten.
- [ ] Capability surface remains executable build identity outside the durable
      public resource budget of Workspace Manifest, Runtime, and Workspace. It
      is absent from Manifest desired/applied/observed/failure state and cannot
      be selected by a Manifest revision, Runtime revision, Workspace record,
      migration record, or fresh-state recommended draft.
- [ ] The protected release artifact has the release surface, the embedded
      resolver, canonical executable name `tobari`, and a Catalog with no
      `auth` namespace, no `auth login|import|status|logout`, and no `serve`.
- [ ] `task build` produces `bin/tobari` with the development resolver and the
      exact release Catalog; being source-built does not grant research
      capability.
- [ ] `task build:dev` produces `bin/tobari-research` with the development
      resolver and research surface. Its Catalog is exactly the release Catalog
      plus five commands: `auth login`, `auth import`, `auth status`,
      `auth logout`, and `serve`; it adds no sixth command and removes none.
- [ ] `task build:dev` remains the sole contributor research Task path. No
      `build:research` alias is added. The active tag cuts over without alias
      from `tobari_experimental` to `tobari_research`; research implementations
      require `tobari_dev && tobari_research`, and embedded+research fails fast.
- [ ] The expected common Catalog is derived from the actual integrated Catalog,
      never from one absolute command count. Completed WP01/WP02/WP08/WP03
      Manifest, Runtime, Workspace, output, and reference contracts appear
      identically in release and research; only the four Broker auth commands
      and `serve` form the surface delta.
- [ ] All maintained documentation, help, version recovery, Task output, tests,
      and research evidence use `bin/tobari-research` consistently. No active
      example invokes Broker research through bare or installed `tobari auth`.
- [ ] Release root human help, root agent help, scoped help, direct dispatch,
      generated Catalog, archive inspection, and host-runnable release evidence
      mechanically prove the absence of `auth` and `serve`.
- [ ] Research root human help, root agent help, scoped help, and direct
      dispatch mechanically prove the exact five-command Catalog delta and use
      `tobari-research` as canonical presentation identity.
- [ ] Release topology contains only Gateway and OPA and cannot resolve or
      start Auth Broker or the research Gateway layer. Research topology remains
      the existing Gateway/OPA/Auth Broker topology and existing trust model.
- [ ] Standard authentication remains native and Workspace-owned: the agent or
      provider CLI creates its own state only in the persistent Workspace home;
      host credentials are not inherited, and Gateway forwards client auth and
      cookies only after ordinary policy allow. Native authentication state,
      learned permissions, and attachment authority are not Workspace Manifest
      desired/applied state.
- [ ] Research documentation and contracts no longer describe Broker material
      as a standard Manifest responsibility. New research credential state is
      created only by explicit login/import under a new Manifest identity and
      remains a separate Auth Broker store, never a Manifest revision field,
      native credential, or supported public outcome. Retained predecessor UUID
      bytes do not authorize old vault or handle reuse.
- [ ] Research authentication remains explicitly repository-only, unsupported,
      and non-release; no wording, navigation, badge, download, install command,
      version label, or side-by-side product chooser presents it as a supported
      variant.
- [ ] Build constraints are the sole surface authority. Runtime argv,
      environment, configuration, state, Workspace Manifest desired/applied
      revisions, Runtime data/revisions, image metadata, executable basename,
      or copied/renamed binaries cannot widen release to research.
- [ ] WP02 copy inputs and WP03 Runtime lifecycle inputs/references cannot
      select capability surface. Copy creates independent identities without
      reconciliation or copied auth/authority state; Runtime retirement,
      prune, restore, protection state, receipts, and sidecars remain common
      runtime data and never activate `auth` or `serve`.
- [ ] Release packaging accepts no build-tag input, neutralizes ambient
      `GOFLAGS`, contains no research build tag, packages only `tobari`, and
      fails if executable identity or Catalog evidence is not release-surface.
- [ ] `version` human and JSON output distinguish capability surface from
      resolver channel. JSON schema V1 contains
      `capability_surface: release|research` and
      `resolver_channel: embedded|development`; it contains no predecessor
      field/value, schema 2, dual reader, or alias. Research development
      recovery names `task build:dev` and `bin/tobari-research`; release
      artifacts expose no contributor recovery command.
- [ ] The published standard architecture site is generated from the release
      Catalog and its start, guide, how-it-works, reference, search, and
      learning-path surfaces contain no Broker research command, topology,
      provider map, state path, or recovery journey. Native authentication
      remains in the standard journey.
- [ ] Existing prerelease consumers receive an explicit breaking-schema note:
      final public version JSON remains schema V1 while the pre-public reset
      replaces `capability_profile` with
      `capability_surface: release|research`. No dual field, alias, ordinary
      compatibility reader, or schema-2 public contract is introduced.
- [ ] Research Broker predecessor state is not migrated in place. Legacy
      research handles are rejected by the new surface; research users
      explicitly login/import again under a new Manifest identity. WP01's
      private migration backup/quarantine retains predecessor owner-only
      encrypted material without automatic deletion, decryption, or rebinding;
      cleanup requires separate review. Standard Workspace homes and native
      credentials are retained and never read by this disposition.
- [ ] Research migration security vocabulary distinguishes **authentication
      authority**—credential ciphertext plus every filesystem binding, handle,
      lookup, projection, and provider record needed to replay it—from **inert
      recovery material**. WP01 atomically quarantines that complete filesystem
      authority set. Linux filesystem root-key material moves with it; a macOS
      Keychain root key remains unchanged and becomes inert once no normal
      reader can reach dependent ciphertext or bindings.
- [ ] `migrate apply` never reads, copies, rotates, or deletes the macOS
      Keychain item. Rollback restores exact quarantined filesystem bytes and
      reuses the unchanged item, failing closed if fresh canonical auth state
      now exists. No automatic Keychain cleanup occurs; any future explicit
      cleanup must prove no live or quarantined ciphertext depends on the item.
- [ ] Old and new normal readers cannot discover quarantine. Preserved UUID
      bytes resolve neither old handles nor ciphertext. Release remains unable
      to activate Broker paths; research auto-discovers no quarantine and gains
      authority only through explicit fresh login/import.
- [ ] Every final research `auth` Catalog/help/evidence example uses the
      durable Workspace Manifest spelling `--manifest` and schema identity
      `workspace_manifest_id`; this packet does not implement or alias the
      former `context` namespace/flag/schema.
- [ ] Runtime examples consumed from completed WP02/WP03 integration use
      `runtime create --copy-source-from ...` and `runtime build --id ...`, with
      no `--base`, `--name` build-action alias, or omitted-name action
      selection.
- [ ] Catalog routine discovery remains within the existing one-invocation
      known-path and two-invocation unknown-outcome budgets, with zero
      undeclared external-processing steps.
- [ ] Focused matrices, `task check`, `task security`, `task public:check`, and
      `task release:check` plus release archive inspection pass before
      implementation is considered complete.

## Governing documents

- Thesis: `docs/00_theses.md`, especially native Workspace authentication,
  compiled research brokering, one shared cluster, and release claims
- Product contract section: `docs/01_product_contract.md`, public concepts,
  command Catalog, authentication, build identity, and compatibility
- Architecture or security invariant: compile-time closed capability surface,
  four-layer ownership, native credential ownership, Broker post-policy
  resolution, and no runtime widening
- Harness and release: `docs/04_harness.md` and `docs/06_release.md`
- Authentication and agent readiness: `docs/07_authentication.md` and
  `docs/09_agent_readiness_validation.md`
- Existing ADR: `docs/decisions/0044-make-native-workspace-auth-standard.md`
- Accepted upstream decision: [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md), with durable consequences in `docs/00_theses.md` through `docs/04_harness.md` and `docs/07_authentication.md`; integrated evidence commits `07535a9` and `428812f`

## Completion definition

The design is Accepted and fixed; implementation has not started. The future
work is complete only after the ADR and numbered contracts own the durable
vocabulary, all acceptance criteria have evidence, the required gates and
archive inspection pass, the implementation is committed, the control thread
receives `WP04_IMPLEMENTATION_COMPLETE`, and this temporary packet is removed.
A post-WP03 gate contradiction instead receives `WP04_BLOCKED`; neither outcome
authorizes publication.
