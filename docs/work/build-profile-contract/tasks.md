# Work Tasks: Release and research build boundary

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

This Accepted/Fix checklist records packet-preparation evidence, fixed Product
Owner decisions, and future dependency order. Checked design/discovery items
are not implementation. Production code, tests, durable contracts, generated
content, and release files remain unchanged by this packet.

## Understand

- [x] Fetch `origin/main`, verify local/remote main at
      `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`, and verify the historical
      observation worktree was clean.
- [x] Read `AGENTS.md`, theses, product, architecture, security, harness,
      public/release, authentication, external API, and agent-readiness
      contracts in numeric order.
- [x] Inspect README, Taskfile build paths, release workflow/packager/lint,
      Catalog registration, domain identity, application ownership,
      infrastructure resolver/topology, site generator/content, tests, and
      relevant recent commits.
- [x] Build three private temporary observation binaries and establish the
      historical 40/40/45 Catalog counts, exact five-path research delta,
      release unknown-command behavior, build metadata, and recovery mismatch.
- [x] Inspect every repository file containing `tobari_experimental` and record
      the canonical source/snapshot consequence.
- [x] Check current official Go build-constraint and `-tags` documentation and
      record only the facts used.
- [x] Inspect remote tags and record that eleven public development prereleases
      make the JSON rename a published prerelease compatibility event.
- [x] Read accepted ADR 0079 and durable `docs/00_theses.md` through
      `docs/04_harness.md` plus `docs/07_authentication.md`; record final
      Manifest/Workspace vocabulary, three-resource budget, state ownership,
      schema-V1 reset, migration direction, and one-time-copy semantics as
      upstream authority rather than local choices.
- [x] Observe integrated upstream evidence at `07535a9` and `428812f`, preserve
      all production/Catalog/schema/helper/site files, and keep old-main
      observations historical only.
- [x] Read the durable copy contracts and Product Owner-fixed WP03 design;
      record their exact copy and Runtime lifecycle command vocabulary,
      independence/protection semantics, common-Catalog consequence, and the
      remaining implementation dependency.
- [x] Identify WP08 as the Catalog/output conformance predecessor and record the fixed
      implementation order: WP01+WP02 completion audit -> WP08 -> WP03 -> WP04.
- [x] Record WP01 evidence that UUID byte preservation plus the predecessor
      `config/auth/contexts/<UUID>` reader path would implicitly rebind legacy
      experimental credential/handle authority to a new Manifest.
- [x] Accept WP01's resolution: atomically quarantine only the experimental
      replay-capable filesystem authority set, preserve standard Workspace homes,
      deny normal-reader and legacy-handle reachability, and perform no automatic
      delete/decrypt/import/rebind. Linux filesystem root-key material moves with
      the set; the macOS Keychain item remains unchanged and becomes inert
      recovery material. WP01 owns migration/journal/rollback/secret-free
      evidence; WP04 later re-verifies denial and explicit new login/import.
- [x] Record verified facts, evaluation, unknowns, and inference separately in
      `context.md`.
- [x] Replace references to the retired WP01/WP02 temporary packet paths with
      ADR 0079, numbered durable contracts, and integration-commit evidence;
      verify both retired paths are absent from all four WP04 packet files.

## Mandatory implementation-start gate

- [x] Confirm the WP01+WP02 durable contracts and integration evidence at
      `07535a9` and `428812f`. This completes only the first dependency leg and
      does not start WP04 production implementation.
- [ ] Wait for `WP08_IMPLEMENTATION_COMPLETE`, then for
      `WP03_IMPLEMENTATION_COMPLETE`; do not partially reproduce either packet.
- [ ] After WP03 handoff, fetch `origin/main`; record actual HEAD,
      branch/upstream, worktree status and ownership; inspect the integrated
      WP01/WP02/WP08/WP03 commits/diff; and reread the resulting governing
      contracts, ADRs, README, Catalog, schema/migration, Taskfile, build
      constraints, release scripts, helper snapshot, site generator/content,
      and relevant tests.
- [ ] Rebuild bounded release, repository-release, and research observation
      binaries against that actual baseline. Derive complete Catalog path sets,
      roles/effects/recursive references, version identity, help, topology,
      release archives, and recovery afresh; record absolute counts only as
      diagnostics.
- [ ] Verify that no stable release exists and inventory known prerelease
      consumers. If contradicted, stop and send `WP04_BLOCKED` to control thread
      `01a02c51-885b-7b80-a66f-05850f48ba4d`; do not invent compatibility.
- [ ] Consume WP01's synthetic quarantine/migration/journal/rollback evidence
      and observe complete filesystem-set movement, exact old/new-reader denial,
      Linux root-key movement, zero macOS Keychain operations, exact-byte
      rollback using the unchanged item, fresh-canonical-state conflict, and
      standard-home preservation.

## Product Owner decisions fixed

- [x] Fix `release surface|research surface` and independent
      `embedded|development` resolver vocabulary; final version JSON is schema
      V1 with `capability_surface` and `resolver_channel`, with no predecessor
      field/value, schema 2, dual reader, or alias.
- [x] Fix the three build tuples and canonical programs: protected
      release=`release+embedded+tobari`; `task build`=`release+development+
      bin/tobari`; retained `task build:dev`=`research+development+
      bin/tobari-research`.
- [x] Fix no `build:research` alias, no `tobari_experimental` alias,
      `tobari_dev && tobari_research` for research implementation, and
      fail-fast rejection of embedded+research.
- [x] Fix research Catalog as the complete integrated common Catalog plus
      exactly four auth paths and `serve`; use set differences, never absolute
      command totals.
- [x] Fix build constraints as sole capability authority; every Manifest,
      Runtime, Workspace, migration, copy, lifecycle, argv, environment,
      configuration, state, image, basename, or copied-binary input is
      non-authoritative.
- [x] Fix native Workspace-owned standard authentication and repository-only,
      unsupported, unpublished research Broker presentation; remove research
      concepts from standard IA and require exact research binary examples.
- [x] Fix predecessor research disposition to atomic private quarantine and
      explicit new-Manifest login/import, with no legacy-handle acceptance,
      automatic deletion/decryption/import/rebinding, or standard credential
      read.
- [x] Fix **research authentication authority** as the complete replay-capable
      ciphertext plus filesystem binding/handle/lookup/projection/provider set,
      and **inert recovery material** as material with no normal reachability or
      binding path. Treat the distinction as durable security vocabulary.
- [x] Fix platform behavior: Linux filesystem root-key material moves with the
      quarantined set; macOS Keychain is not read/copied/rotated/deleted,
      rollback reuses it only after exact filesystem restoration, and any
      future cleanup proves no live/quarantined ciphertext dependency.
- [x] Accept the pre-public V1 breaking reset under the no-stable-release
      premise; known prerelease consumers receive a breaking note and no dual
      compatibility.
- [x] Assign WP04 ownership of the surface/absence/mistaken-publication matrix
      and release archive inspection; release packets consume this evidence.
- [x] Fix completion gates, commit requirement, control-thread completion/
      blocked notification, and temporary-packet deletion after durable
      promotion.

## Implementation coordination still required

- [ ] Promote the fixed decision through one ADR and the durable numbered
      contracts before production mechanisms are considered complete.
- [ ] Promote authentication-authority versus inert-recovery-material vocabulary
      and its platform-neutral reachability rule into the security model and ADR.
- [ ] Resolve or explicitly supersede conflicting Broker-publication outcomes
      in Active first-release and auth-narrowing packets.

## Contract and failing evidence

- [ ] Revise `docs/00_theses.md` through `docs/04_harness.md` in order with
      release/research surface vocabulary and mechanical consequences.
- [ ] Add a build-matrix test that produces release-artifact-equivalent,
      repository release-surface, and repository research binaries in private
      temporary directories.
- [ ] Add one exact Catalog path-set oracle: research minus release is the five
      named paths, release minus research is empty, and the common set is
      derived from the actual integrated Catalog rather than historical 40/45
      totals.
- [ ] Prove completed WP02/WP08/WP03 Manifest copy, Runtime copy/build/review/
      delete/prune/restore paths and complete nested reference contracts are
      identical across release and research.
- [ ] Add root human, root agent, namespace, exact help, and direct-dispatch
      negative tests for release `auth` and `serve` absence.
- [ ] Add research presentation tests proving canonical program, usage,
      invocation template, machine examples, and recovery all name
      `tobari-research`.
- [ ] Add negative tests proving environment, config, state, image names,
      runtime argv, actual `argv[0]`, Workspace Manifest or Runtime revisions,
      Workspace or migration records, fresh-state recommendation drafts, and
      the retired tag cannot widen release.
- [ ] Add non-activation canaries for completed WP02/WP03 copy inputs/fresh
      identities and Runtime protection, prune-plan, restore, journal, receipt,
      sidecar, and observation data, consuming WP08's Catalog-wide mechanism
      without adding a Runtime-specific validator.
- [ ] Add topology/resolver tests that reject research+embedded and prove
      release has zero Broker resolution, control, inspection, Compose, or
      startup calls.

## Domain and presentation

- [ ] Replace capability-profile domain vocabulary with the closed
      `CapabilitySurface` `release|research` enum.
- [ ] Encode valid surface/resolver tuples and fail before runtime effects on an
      invalid tuple.
- [ ] Define immutable canonical program identity separately from executable
      basename and add surface-aware development recovery.
- [ ] Keep final public version JSON at schema V1, replace
      `capability_profile: standard|experimental` with
      `capability_surface: release|research`, add the independent
      `resolver_channel: embedded|development`, update human labels, and reject
      every dual-field, predecessor-value, alias, or schema-2 proposal.
- [ ] Update fixtures and negative schema tests for exact absent/empty Broker
      and contributor fields on the release surface.
- [ ] Preserve application-owned auth and Operator Console behavior unchanged.
- [ ] Keep capability surface outside Workspace Manifest desired, last-applied,
      observed, and failure state; keep standard native credentials, learned
      permissions, and attachment authority outside Manifest desired/applied
      state.

## Catalog, build, and topology

- [ ] Rename active build constraints from `tobari_experimental` to
      `tobari_research`; require `tobari_dev && tobari_research` for research
      implementations and leave no embedded-research implementation.
- [ ] Reject `build:research`, old-tag aliases, research without `tobari_dev`,
      and embedded+research through exact Task/build/tuple negative tests.
- [ ] Update CLI composition so release remains the base Catalog and research
      appends exactly the existing four auth specs and `serve`.
- [ ] Derive that base from the post-WP03 integrated Catalog and preserve every
      common path. Completed WP01/WP02/WP08/WP03 Manifest, Runtime, Workspace,
      output, and reference paths must not move into the research-only
      projection.
- [ ] Apply the research Catalog delta only after the shared ADR 0079
      cutover, using final `manifest`/`--manifest`,
      `workspace_manifest_id`, and `workspace_id` contracts with no public
      Context, `project_id`, or `instance_id` alias.
- [ ] Make Catalog-derived research help consistently render
      `tobari-research` without reading `argv[0]` as authority.
- [ ] Update `scripts/build-dev-images.sh` to use research vocabulary while
      preserving the exact existing images, Dockerfiles, and topology.
- [ ] Update `task build` to remain development-resolver/release-surface and
      `task build:dev` to build `tobari_dev tobari_research` as
      `bin/tobari-research`.
- [ ] Update the checked runtime helper source snapshot only through its
      canonical synchronization mechanism and prove source/snapshot equality.
- [ ] Remove active `tobari_experimental`, capability-profile, and
      `bin/tobari-dev` build-boundary references without changing unrelated
      profile concepts.

## Release and mistaken-publication prevention

- [ ] Keep the release packager fixed, selector-free, and ambient-Go-setting
      neutral; do not accept surface or tag input.
- [ ] For every archive, inspect `go version -m` and reject the research tag or
      research executable name.
- [ ] On the host-runnable release target, verify exact release version identity,
      empty Broker/contributor fields, root human/agent Catalog absence, scoped
      help failure, and direct `auth`/`serve` unknown-command failure.
- [ ] Add a hostile `GOFLAGS=-tags=tobari_research` canary and prove no research
      archive can be created.
- [ ] Make archive inventory reject `tobari-research` and continue accepting
      only canonical `tobari` plus reviewed supporting files.
- [ ] Ensure release workflow inputs, prepared evidence, provenance, SBOM,
      Formula, and publication jobs cannot name or select the research surface.

## Documentation and information architecture

- [ ] Update README supported build/auth journey first, then add one clearly
      separated repository research note using `bin/tobari-research` only.
- [ ] Update `docs/05_public_repository.md` through
      `docs/09_agent_readiness_validation.md` in numeric order, including the
      final schema-V1 field/binary migration and unchanged auth mechanics.
- [ ] Consume completed WP02/WP03 Runtime examples:
      `runtime create --copy-source-from` with no `--base`, and
      `runtime build --id` with no `--name` or omitted-name action alias.
- [ ] Rewrite every active research example to use the exact research binary;
      auth examples also use explicit `--manifest`; reject bare `tobari auth`
      and `tobari serve` in active guidance/evidence.
- [ ] Remove Broker research topology, commands, providers, state, faults,
      recovery, and chooser language from both locales of standard architecture
      site IA, navigation, search, learning path, diagrams, and glossary.
- [ ] Preserve a complete native Workspace authentication setup, explanation,
      and troubleshooting journey in the standard site.
- [ ] Make site generation assert release-surface executable identity before
      accepting agent help; generated Catalog must contain no research paths.
- [ ] Advance site source snapshot and generated Catalog/component data from the
      same reviewed implementation commit; do not generate from a dirty tree.
- [ ] Add semantic documentation tests that distinguish capability-surface
      violations from legitimate verification/AWS/agent profile terminology.
- [ ] Add release notes for `bin/tobari-dev` to `bin/tobari-research`,
      `tobari_experimental` to `tobari_research`, and the final schema-V1
      version field/value reset.

## Compatibility, state, and trust-boundary evidence

- [ ] Consume ADR 0079 migration evidence that predecessor Context UUID
      bytes become WorkspaceManifestID and ProjectInstance UUID bytes become
      WorkspaceID while public outputs expose only final vocabulary; do not
      duplicate that migration in build-surface code.
- [ ] Consume WP01's synthetic atomic quarantine, migration-journal, rollback,
      and secret-free evidence. Prove predecessor
      `config/auth/contexts/<UUID>` and legacy handles are unreachable through
      the normal research reader after UUID preservation, with no automatic
      delete/decrypt/import/rebind.
- [ ] Prove the quarantined filesystem set includes every ciphertext, binding,
      handle, lookup, projection, provider, and Linux filesystem-root-key record
      needed for replay; omission of any member fails closed before publication.
- [ ] Prove macOS `migrate apply` performs zero Keychain read/copy/rotate/delete
      operations; rollback restores exact filesystem bytes, reuses the unchanged
      item, and fails closed when fresh canonical auth state exists.
- [ ] Prove old and new normal readers cannot discover quarantine and that
      neither release nor research automatically discovers or resolves legacy
      ciphertext/handles after the build-surface cutover.
- [ ] Prove explicit login/import under a new Manifest identity is the only
      path to new-surface Broker authority and does not recover a legacy handle.
- [ ] Prove standard Workspace credentials are neither inspected nor migrated,
      and that research vault/handle state is a separately Auth Broker-owned
      store rather than Workspace Manifest desired/applied state.
- [ ] Prove standard Workspace homes survive WP01 migration/quarantine and WP04
      build-surface cutover unchanged.
- [ ] Prove learned permission and attachment authority remain outside Manifest
      desired/applied state and cannot activate research capability.
- [ ] Prove completed Manifest/Runtime copy creates fresh independent identity without
      copying authority state or reconciling, and Runtime lifecycle/protection
      state remains common to both surfaces and cannot activate research.
- [ ] Prove neither build task deletes or adopts an old unmanaged
      `bin/tobari-dev`; document explicit optional cleanup only.
- [ ] Prove release remains Gateway/OPA with Workspace-owned native auth and
      research remains the existing Gateway/OPA/Auth Broker topology.
- [ ] Prove release archives contain no reachable Broker reader, image,
      topology, command, or quarantine-discovery path; research reaches Broker
      authority only after explicit fresh login/import.
- [ ] Re-run the standard native-login regression and existing research Broker
      synthetic journey without changing provider/auth outcome assertions.
- [ ] Record routine-success external-processing count zero and existing agent
      help discovery budgets for both supported release discovery and the
      optional research evidence entry point.

## Verify

- [ ] Implementation-start evidence records the post-WP03 fetched HEAD,
      worktree ownership/status, governing-contract snapshot, and newly derived
      Catalog sets; no historical absolute count is used as a golden. Evidence:
- [ ] Focused domain, Catalog, build-identity, topology, sitegen, and release
      tests pass. Evidence:
- [ ] Default `go test ./...` and the `tobari_dev tobari_research` targeted
      matrix pass. Evidence:
- [ ] Research integration/runtime gates pass when build/tag/topology source
      changes. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes. Evidence:
- [ ] Generated diff and one-source-snapshot evidence are reviewed. Evidence:
- [ ] Release archive inspection passes for every produced target. Evidence:
- [ ] Final `git status` contains only the intended implementation and promoted
      contract changes. Evidence:

## Hand off

- [ ] Acceptance criteria have concrete evidence and the goal is changed to
      `Complete` only then.
- [ ] Conflicting Active packets name their disposition and no competing public
      Broker outcome remains.
- [ ] The WP01+WP02 audit, WP08 completion, WP03 completion, and post-WP03 fresh
      rebaseline precede and are referenced by this packet's integrated evidence.
- [ ] Completed WP01/WP02/WP08/WP03 capabilities and reference contracts are
      present identically in both surfaces.
- [ ] Durable decisions are promoted to the ADR and numbered contracts.
- [ ] No credential, live-provider transcript, temporary observation binary, or
      stale generated artifact remains.
- [ ] The next prerelease handoff explains outcome, vocabulary, compatibility,
      checks, and remaining research risk without publishing automatically.
- [ ] Commit the complete implementation and evidence only after every required
      gate and archive inspection passes.
- [ ] After the commit, notify control thread
      `01a02c51-885b-7b80-a66f-05850f48ba4d` with
      `WP04_IMPLEMENTATION_COMPLETE`. If the implementation-start gate or later
      integration contradicts a fixed decision, stop and notify
      `WP04_BLOCKED` instead of changing the design independently.
- [ ] This temporary packet is removed in the completion handoff.
