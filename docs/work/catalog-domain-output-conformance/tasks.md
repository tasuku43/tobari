# Work Tasks: Keep Catalog output declarations conformant with domain semantics

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

This packet's detailed design is accepted/fixed for future implementation.
Checked discovery and decision tasks record packet-authoring evidence only;
implementation, promotion, gate, commit, and completion work remains unchecked.

## Understand

- [x] Fetch `origin/main`, confirm local/current main identity and divergence,
      and protect a clean worktree.
- [x] Read `AGENTS.md`, `docs/00_theses.md`, governing `docs/01` through
      `docs/04`, `docs/07` through `docs/09`, the current templates, README,
      accepted ADR 0037, relevant source/tests/harness, and recent commits.
- [x] Reproduce current behavior with a bounded synthetic constructor-built
      template and remove the temporary diagnostic.
- [x] Record verified facts, evaluation, inference, external official facts,
      and unknowns separately in `context.md`.
- [x] Confirm the public outcome, non-goals, trust boundary, concept count, and
      related-packet conflicts.
- [x] Read the former WP 01/WP 02 decisions, then sync their promoted authority
      to ADR 0079 and `docs/00` through `docs/04`; commits `07535a9`/`428812f`
      are integrated evidence and the temporary packet files are removed.
- [x] Confirm the integration checkout contains no tracked production change and
      preserve unrelated untracked work packets; no production surface was
      edited during this sync.
- [x] Read accepted WP 03 Runtime Retirement and retain its recursive Runtime
      reference requirements without treating design approval as an
      implementation-start signal.

## Decide

- [x] Product Owner accepted/fixed the detailed design; implementation remains
      unstarted and all implementation/evidence/gate items remain unchecked.
- [x] Approve the minimum repository invariants and initial domain-owned scope:
      final `PolicyRule`/`PolicyRuleReport` match, protocol, decision, and
      state-change only, with exact paths inventoried before implementation.
- [x] Confirm public-contract impact: enum widening within schema 1, no command,
      field, reference, capability, state, or migration addition.
- [x] Confirm `policy rules` remains `RoleDiscover`/`EffectRead`, complete
      delivery, exhaustive coverage, and produces the existing reset reference.
- [x] Confirm zero new public concepts, zero removals/hiding/renames, and no
      trust-boundary or authentication change in the immediate repair. Record
      WP 01's separate Context retirement/Manifest rename as an upstream
      cutover, not this packet's concept addition.
- [x] Fix canonical enum order as caller-immutable deterministic presentation;
      confirm it is not semantic compatibility, identity, or authority.
- [x] Exclude CLI-only display, format, delivery, and other non-domain values;
      prohibit mechanical commonization, reflection, DSL, or generator work.
- [x] Accept schema-1 widening with no exact-only parallel schema or bump. A
      concrete incompatible consumer is `WP08_BLOCKED`, not local redesign.
- [x] Approve sequencing: WP 01 + WP 02 completion audit, then WP 08, then WP 03;
      no WP 08 implementation before explicit WP 01 `COMPLETE`.
- [x] Confirm WP 01 owns every Catalog path/field/reference-kind rename and that
      neither runtime validation nor tests accept predecessor and successor
      schemas simultaneously.
- [x] Fix affected `policy rules` `output_encoding_failed` recovery to read-only
      `version`; preserve schema 2/non-retryable/exit 13 and hand all-command
      recovery redesign to WP 10.
- [x] Approve one explicit bounded recursive `OutputField` traversal as the
      repository source for produced reference kind/path, with existing typed
      inputs remaining the consumed-reference source.
- [x] Confirm WP 02's negative no-provenance/no-lineage/no-`base` fixture
      contract and WP 03's `runtime`/`runtime-revision`/`runtime-prune-plan`
      reference fixture contract without registering nonexistent commands.

## Gate after WP 01 + WP 02 completion

- [ ] Reconcile `policy-compaction-retirement` and first-public-release packet
      statements with ADR 0037 and the audited final source. Evidence:
- [ ] Obtain implementation-readiness review from domain, CLI/catalog, harness,
      security, and release owners. Evidence:
- [ ] Reverify ADR 0079, promoted `docs/00` through `docs/04`, and integrated
      commits `07535a9`/`428812f` on the implementation HEAD. Evidence:
- [ ] Audit the promoted target-specific copy schemas, no-provenance/no-`base`
      canaries, HEAD, and status. Evidence:
- [ ] Fetch and record actual `HEAD`, upstream/origin relationship, branch, and
      `git status`; identify every remaining user-owned change and stop rather
      than overlap unresolved WP 01 files. Evidence:
- [ ] Reread `AGENTS.md`, ADR 0079, and `docs/00` through `docs/04`, then inspect
      the final policy domain, Catalog, recursive
      output declarations, renderers, schemas, tests, helper source, and
      generated data. Evidence:
- [ ] Rerun the bounded synthetic template scenario against text, JSON, and
      scoped agent help. Classify whether the bug remains, moved, or was fixed;
      update this packet's implementation coordinates if needed. Evidence:
- [ ] Enumerate every exact final Catalog path reached by domain-owned match,
      protocol, decision, and state-change and freeze the existing domain order
      as caller-immutable presentation. Reject adjacent CLI-owned enums from the
      inventory. Evidence:
- [ ] Audit known consumers for a concrete contradiction to schema-1 widening;
      if found, stop and notify `WP08_BLOCKED` without dual schema or bump.
      Evidence:
- [ ] Confirm the final public schema contains only `manifest` / `--manifest` /
      `workspace_manifest_id` / `workspace_id` and no predecessor dual schema.
      Evidence:

## Implement immediate repair

- [ ] Add a failing full-dispatch JSON regression using a valid synthetic HTTP
      path-template rule created through final post-WP01 domain constructors.
- [ ] If the gate proves the mismatch remains, add `path_template` to the exact
      final `policy rules.items[].match` declaration and correct its description
      without changing field shape or schema version. If already fixed, record
      that evidence and do not create a duplicate production patch.
- [ ] Prove the same synthetic report succeeds in text and JSON and that scoped
      agent help advertises both `exact` and `path_template`.
- [ ] Prove exact allow, exact deny, and empty exhaustive reports remain
      compatible and read-only.
- [ ] Make the fixture use only final `workspace_manifest_id`/Manifest and
      `workspace_id` semantics; retain predecessor fields only as negative
      canaries, never as an accepted fixture branch.

## Implement structural prevention

- [ ] Add caller-immutable canonical domain value sets for each approved policy
      vocabulary while preserving typed constants.
- [ ] Make domain validation consume those canonical sets without weakening
      cross-field or constructor invariants.
- [ ] Make every inventoried public and nested Catalog enum consume the same sets;
      do not move CLI-only parser vocabularies into domain for symmetry.
- [ ] Add exact field-path equality tests that fail for both missing domain values
      and extra Catalog values.
- [ ] Add a constructor-built semantic fixture and answer key for HTTP
      path-template rules with two examples and deterministic synthetic sources.
- [ ] Add exact GraphQL, MCP, AWS, Kubernetes, Git, and OCI fixtures that prove
      protocol fields are populated or explicitly empty as declared.
- [ ] Add nested reviewed-decision fixtures for exact and path-template match.
- [ ] Cover empty collections, required empty strings, optional absence,
      nullable null, zero, false, extra/missing fields, wrong types, and internal
      `segments` omission without conflating their meanings.
- [ ] Add negative tests for non-HTTP templates, unknown finite-vocabulary values,
      wrong task/scope/reference identity, and insufficient template evidence.
- [ ] Preserve the runtime recursive Catalog validator; do not add a bypass,
      mutable global Catalog, reflection registry, or large generator.
- [ ] Add generic failing fixtures proving top-level, nested object, nested
      array, and object-inside-array reference declarations are all returned by
      `ProducedRefs()` with deterministic canonical field paths.
- [ ] Implement one bounded explicit traversal over `OutputField.Fields` and
      `OutputField.Items`; preserve existing pagination cursor behavior and
      reject duplicate/conflicting reference paths.
- [ ] Make Catalog graph validation, required producer reachability, scoped
      agent help/workflows, and reference conformance fixtures consume that same
      traversal; add no handler, Runtime, or renderer-side walker.
- [ ] Prove required-reference cycles, missing producers, wrong kinds, invalid
      nested field types, and ambiguous paths still fail closed.

## Consume promoted upstream contracts and hand off to WP 03

- [ ] Expose the exact/template/deny/empty/protocol/nested corpus through a
      reusable exact-path seam over WP 01's final identity paths without
      cloning semantic fixtures or accepting two schemas.
- [ ] Consume ADR 0079 and durable-doc evidence that final Catalog, JSON, human,
      and agent surfaces
      use
      `workspace_manifest_id`/`workspace_id` and reject predecessor `context`,
      `context_id`, `project_id`, and `instance_id` fields. Consume WP 01's
      repository-wide command/flag/completion retirement checks rather than
      duplicating them.
- [ ] Record that stable ID byte preservation belongs only to ADR 0079's explicit
      predecessor migration; this packet adds no fallback reads, aliases, dual
      writes, or fixture normalization accepting both schemas.
- [ ] Register complete recursive Workspace Manifest and Workspace
      desired/applied/observed Catalog outputs as consumers of the generic
      invariants without redesigning their shapes here.
- [ ] Consume the promoted final revision-body retention, Git slice, attached
      child-session, Docker migration-evidence, and migration-precondition
      representations without reopening them.
- [ ] Audit WP 02 target-specific Manifest-copy and Runtime-source-copy fixtures:
      fresh identities, exact accepted source semantics,
      no copied state, no reconciliation, and no provenance/lineage/source-
      identity/retired-`base` fields in state, JSON, status, Catalog, or help.
- [ ] Verify WP 02 negative schema canaries for `base`, `base_id`, `base_revision`,
      `copied_from`, `copy_source`, `parent`, `inherits_from`, `derived_from`,
      `lineage`, `source_workspace_manifest_id`, `source_manifest_revision`, and
      `source_snapshot_digest`; share harness code, not a production derivation
      type.
- [ ] Publish the WP 03 fixture interface for exact recursive paths and kinds,
      including
      `items[].runtime_ref`, the approved nested `revision_ref` path, and
      `plan_ref`; WP 03 later proves build/delete, restore, and prune apply
      consume their opaque values unchanged.
- [ ] Require WP 03 to use the Catalog-wide recursive traversal for list/show/
      history/prune help, graph reachability, and schema fixtures; reject a
      Runtime-only second validator.
- [ ] Do not modify audited WP 02 production or begin WP 03 production. WP 03
      becomes eligible only after the committed WP 08 completion notification.

## Verify fault and semantic behavior

- [ ] Prove valid template JSON exits 0, writes schema-1 stdout only, and does not
      emit `output_encoding_failed`.
- [ ] Prove genuine declaration drift remains schema-2
      `output_encoding_failed`, presentation phase, non-retryable, empty stdout,
      `change_state: not_applicable`, and exit 13; affected `policy rules`
      recovery is exact read-only `version`, while sibling fault recovery remains
      unchanged for WP 10.
- [ ] Prove domain-invalid learned state keeps its data/read fault rather than
      being reclassified as a presentation fault.
- [ ] Prove text, JSON, agent help, and the typed answer key agree on task, scope,
      match, protocol, request, examples, sources, and opaque identity.
- [ ] Consume the promoted upstream evidence proving the same semantic result uses
      only WorkspaceManifestID/WorkspaceID authority and no display name,
      ProjectRoot, predecessor field, or alias can substitute for it.
- [ ] Prove routine success needs one exact-help discovery plus one execution,
      zero external semantic-processing calls, and zero mutations.
- [ ] Run the common corpus in standard and
      `tobari_dev tobari_experimental` Catalog builds.
- [ ] Prove recursive produced refs include exact kind and canonical field path
      in standard and experimental Catalogs, and nested refs participate in the
      same producer/consumer reachability and workflow derivation as top-level
      refs.
- [ ] Audit WP 02 against its target-specific no-provenance interface and publish
      the WP 03 Runtime/revision/plan fixture interface. Add no disabled fixture,
      placeholder Catalog entry, or nonexistent command to WP 08.

## Synchronize documentation and generated material

- [ ] Promote the finite-vocabulary/Catalog conformance rule, recursive
      produced-reference kind/path rule, and semantic-branch evidence
      requirement to `docs/04_harness.md`.
- [ ] Clarify `docs/02_architecture.md` only if durable review finds a real gap;
      otherwise cite its existing recursive Catalog invariant.
- [ ] Update ADR 0037 mechanical enforcement evidence if approved.
- [ ] Synchronize and verify the checked runtime helper-source closure.
- [ ] Regenerate and review the architecture-site Catalog JSON.
- [ ] Update synthetic conformance fixtures, answer keys, and
      `.harness/schemas.json` digests without live data.
- [ ] Reconcile or supersede conflicting temporary work packets before release
      evidence is frozen.
- [ ] Coordinate with the durable Workspace Manifest vocabulary so the promoted
      invariant names typed semantic fields and exact Catalog paths rather than
      freezing Context-era nouns.
- [ ] Confirm the immediate repair needs no thesis, product-semantic, security,
      authentication, external API, capability-ledger, README-behavior, or
      state-migration change; keep ADR 0079's predecessor migration unchanged.
- [ ] Cross-link WP 02 and WP 03 durable/test promotion to the shared invariant;
      verify neither adds a common derivation model, lineage schema, dual
      flag/field alias, or Runtime-specific reference validator.

## Gates

- [ ] Focused domain/application/CLI tests pass. Evidence:
- [ ] Standard and experimental-tag Catalog tests pass. Evidence:
- [ ] Helper-source and generated Catalog drift checks pass. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] `task release:check` passes. Evidence:
- [ ] Manual current-source observation with isolated synthetic state passes.
      Evidence:
- [ ] Final repository diff contains no live path, host, identifier, credential,
      temporary diagnostic, or unrelated user change. Evidence:
- [ ] Final diff is based on the recorded post-WP01 integration HEAD and does
      not overwrite, duplicate, or revert WP 01 production changes. Evidence:

## Hand off

- [ ] Every acceptance criterion has linked evidence and cross-cutting review
      decisions are resolved or assigned to an explicit non-blocking successor.
- [ ] Durable conclusions are promoted from this packet.
- [ ] Goal status changes to `Complete` only after every required task and gate is
      complete.
- [ ] This temporary packet is removed from the final implementation tree.
- [ ] Handoff summarizes the repaired user outcome, invariant, compatibility,
      generated changes, checks, dependencies, and residual risks.
- [ ] Commit the completed implementation and record final commit HEAD plus clean
      or fully explained `git status`. Evidence:
- [ ] Use the thread tool to notify control thread
      `01a02c51-885b-7b80-a66f-05850f48ba4d` with exactly
      `WP08_IMPLEMENTATION_COMPLETE`, or `WP08_BLOCKED` when a gate/integration
      fact prevents completion. Include acceptance evidence, final interfaces,
      all gate results, HEAD/status, packet retention/removal, and explicit WP 03
      start eligibility. Evidence:
