# Work Goal: Keep Catalog output declarations conformant with domain semantics

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md` invariants 7, 8, and 13; `docs/02_architecture.md` Catalog and machine-output boundaries
- Review/delete trigger: Delete after the conformance invariant and evidence are promoted, the implementation completes, and all required gates pass
- Successor: None
- Owner: CLI/catalog and Tobari policy domain maintainers
- Target: Future implementation; design accepted/fixed, implementation not started
- Related ADRs: `docs/decisions/0037-review-single-segment-path-templates.md`
- Related upstream contracts: `docs/decisions/0079-model-workspace-manifests-and-applied-workspaces.md`
  and promoted `docs/00_theses.md` through `docs/04_harness.md`; commits
  `07535a9` and `428812f` are the integration evidence. The accepted downstream
  design consumer remains `docs/work/runtime-retirement/`.

## Outcome

A learned single-segment HTTP path-template rule remains a valid policy-domain
result and is presented consistently by `tobari policy rules` in human, JSON,
and exact-command agent help. JSON succeeds instead of failing at presentation,
and a focused repository invariant prevents a domain-owned finite vocabulary
from drifting away from the Catalog enum that declares its public projection.
That invariant is expressed in terms of typed semantic fields and exact Catalog
paths, so it survives the accepted Workspace Manifest/Workspace schema reset
without preserving old Context vocabulary.

The same packet closes one structural gap required by Runtime lifecycle work:
opaque references declared anywhere in a recursive Catalog output are derived
once, with canonical field paths, and the producer/consumer graph is reused by
Catalog validation, scoped agent help, workflow discovery, fixtures, and output
conformance. Runtime-specific reference walkers or validators are not added.

## Why now

The investigated `main` baseline accepts and persists a domain-valid
`path_template` rule. Human
output renders it successfully, but `policy rules --format json` exits 13 with
`output_encoding_failed` because `policyRuleOutputFields().match` declares only
`exact`. Existing focused tests pass because their successful JSON fixture
contains only exact rules and their field assertion checks shape rather than
reachable enum values. The current fail-closed renderer is working as designed;
the declaration and its coverage are incomplete.

Since that reproduction, the Workspace Manifest/domain and copy contracts were
promoted into ADR 0079 and `docs/00` through `docs/04`, integrated through
`07535a9`/`428812f`, and their temporary packets were removed. The reproduction
remains diagnosis evidence, not a patch target. No implementation may begin
until a read-only WP 01 + WP 02 completion audit rechecks the actual new HEAD
and working tree against those durable authorities.
The fixed dependency order is WP 01 + WP 02 completion audit, WP 08
implementation, then WP 03 implementation.

## Non-goals

- Do not change the learned-policy algorithm, the reviewed one-segment `{id}`
  template rule, protocol eligibility, or policy authority.
- Do not add a command, capability, reference kind, match mode, template syntax,
  public `segments` field, arbitrary glob/regex support, or provider notation.
- Do not move policy interpretation into the renderer or let the Catalog create
  domain meaning.
- Do not replace `cli.Catalog` with JSON Schema, reflection, a second registry,
  or a repository-wide generated command DSL.
- Do not design or perform WP 01's `context` -> `manifest`, `context_id` ->
  `workspace_manifest_id`, or internal instance-identity migration here. Do not
  admit old and new names simultaneously as a compatibility schema.
- Do not design the complete recursive Workspace Manifest or Workspace
  desired/applied/observed schemas here; ADR 0079 and promoted `docs/00` through
  `docs/04` own their exact shapes and this packet supplies only the generic
  conformance interface they must consume.
- Do not implement WP 02 copy commands or semantics, create a shared production
  derivation type, or add provenance/lineage fields. Do not implement WP 03
  lifecycle commands, Runtime state, protection joins, or mutations.
- Do not add a Runtime-only nested-reference validator or a second producer /
  consumer registry. Do not use Go reflection or a large generator to discover
  reference fields.
- Do not migrate persisted policy state, change credentials or authentication,
  perform external calls, or modify the standard/experimental profile boundary.
- Do not treat a one-line `path_template` enum addition as sufficient completion;
  it is the immediate repair, not the recurrence prevention.
- Do not begin implementation in this work packet.

## Acceptance criteria

- [ ] A valid synthetic path-template learned rule makes `tobari policy rules`
      exit 0 in text and JSON; JSON uses schema version 1, emits
      `match: "path_template"`, and preserves the same semantic rule identity,
      request, protocol fields, examples, source candidates, and opaque rule
      reference as text and the typed domain result.
- [ ] The diagnosed `path_template` regression is repaired and guarded before
      release. A mandatory post-promotion read-only gate records the actual
      HEAD/worktree, re-inventories changed domain/Catalog/
      schema/renderer/test/helper-source surfaces, and reruns the synthetic
      reproduction. If WP 01 already repaired or relocated the declaration, the
      implementation adds the regression and invariant at the new seam instead
      of applying a duplicate predecessor-schema patch.
- [ ] Exact allow, exact deny, path-template allow, and an empty exhaustive rule
      report all satisfy the public Catalog declaration; invalid or undeclared
      enum values still fail closed before stdout.
- [ ] The domain owns one canonical finite value set for each policy vocabulary
      reachable from the final post-WP01 `PolicyRule` / `PolicyRuleReport`
      family: domain-semantic match, protocol, decision, and state-change only.
      Domain validation and all corresponding exact Catalog field declarations
      consume it, and focused tests prove equality at every public or nested
      path. CLI-only display, format, delivery, and other presentation values
      remain excluded.
- [ ] Canonical enum order is fixed and caller-immutable for deterministic agent
      help, generated artifacts, and tests. Order is presentation stability, not
      semantic compatibility, identity, or authority.
- [ ] One explicit recursive walk over `OutputField.Fields` and
      `OutputField.Items` derives every produced opaque-reference kind together
      with a canonical field path. Catalog validation, producer reachability,
      scoped help/workflows, fixtures, and runtime output validation consume
      that result; nested references are not silently omitted or rediscovered by
      a command-specific validator.
- [ ] A constructor-built synthetic template fixture covers nested, optional,
      nullable, explicit-empty, and protocol-specific fields without recording a
      real host, path, identifier, credential, or learned-policy file.
- [ ] Scoped agent help declares both `exact` and `path_template`; human, JSON,
      and agent surfaces agree semantically without presentation inference and
      routine success requires zero external processing calls and no exploratory
      discovery round trip.
- [ ] A valid template no longer produces `output_encoding_failed`; genuine
      output-contract drift retains structured error schema 2, presentation
      phase, non-retryable classification, empty stdout, and exit 13. The
      affected `policy rules` fault points to read-only `version` so users can
      report build identity instead of rerunning the same failing projection;
      repository-wide recovery redesign is deferred to WP 10.
- [ ] Common conformance tests run against standard and
      `tobari_dev tobari_experimental` Catalog builds; helper-source snapshots,
      architecture-site Catalog output, and pinned synthetic fixture digests are
      regenerated from their canonical inputs.
- [ ] Exact-rule consumers and persisted state remain compatible; schema version
      1 and existing command/reference/fault identifiers remain stable, and no
      state migration is required for the immediate repair. The WP 01 + WP 02
      completion audit requires the implemented baseline to expose only
      `workspace_manifest_id`/`workspace_id` identity and reject old public
      Context or instance aliases.
- [ ] `path_template` is accepted as a schema-version-1 enum widening of an
      existing domain contract. No exact-only parallel schema or version bump is
      added. If the mandatory audit finds a concrete incompatible consumer, the
      implementation stops and reports `WP08_BLOCKED` to the control thread
      rather than choosing a dual schema or bump locally.
- [ ] ADR 0079's complete recursive Workspace Manifest and Workspace
      desired/applied/observed Catalog schemas are registered as conformance
      consumers of the same generic invariant, without adding them to this
      packet's implementation scope or deciding their unresolved field shapes.
- [ ] The WP 02 schema-fixture interface requires Manifest and Runtime copy
      success schemas to contain no
      provenance, lineage, source-identity, inherited-state, or retired `base`
      fields and do not accept predecessor/new dual shapes. Target-specific
      fixtures may share test machinery but not a common production derivation
      type.
- [ ] The WP 03 future-fixture interface requires recursive reference discovery
      and exact byte round trip for paths such as `items[].runtime_ref`, the
      approved nested revision
      path (for example `runtime.revisions[].revision_ref`), and `plan_ref`, with
      kinds `runtime`, `runtime-revision`, and `runtime-prune-plan`. WP 03 uses
      this Catalog-wide mechanism and adds no second validator. WP 08 does not
      pre-register commands absent from the audited checkout.
- [ ] Durable harness/architecture wording records the minimum domain-to-Catalog
      enum invariant and its mechanical enforcement; stale active work-packet
      claims are reconciled rather than used to weaken ADR 0037 or current code.
- [ ] Focused tests, `task check`, `task security`, `task public:check`, and
      `task release:check` pass with understood generated diffs.
- [ ] Implementation completion is committed, then the control thread
      `01a02c51-885b-7b80-a66f-05850f48ba4d` receives
      `WP08_IMPLEMENTATION_COMPLETE`; a blocking consumer or integration fact
      instead receives `WP08_BLOCKED`. The notification includes acceptance
      evidence, final interfaces, gates, HEAD/status, packet retention, and
      whether WP 03 may start.

## Governing documents

- Thesis: `docs/00_theses.md`, especially semantics before presentation,
  Catalog ownership, executable claims, and one completion gate.
- Product contract section: `docs/01_product_contract.md` machine-readable
  output, learned policy rules, structured faults, and exit status 13.
- Architecture or security invariant: `docs/02_architecture.md` Catalog and JSON
  renderer contracts; `docs/03_security_model.md` learned-policy integrity,
  untrusted projection, and fail-closed presentation.
- Harness contract: `docs/04_harness.md` command/catalog contract coverage,
  semantic fixtures, negative inference, standard/dev builds, and public/release
  profiles.
- Existing ADR: `docs/decisions/0037-review-single-segment-path-templates.md`.
- External API readiness: `docs/07_authentication.md`,
  `docs/08_external_api_contracts.md`, and
  `docs/09_agent_readiness_validation.md`; no external API or authentication
  change is planned.
- Upstream durable decision: ADR 0079 plus `docs/00_theses.md`,
  `docs/01_product_contract.md`, `docs/02_architecture.md`,
  `docs/03_security_model.md`, and `docs/04_harness.md`; their accepted
  vocabulary, resource budget, activation/identity/migration rules, copy
  semantics, and no-lineage contract are binding. Commits `07535a9` and
  `428812f` record the promoted integration evidence.
- Accepted downstream design: `docs/work/runtime-retirement/` consumes the
  completed invariant afterward and does not authorize capability
  implementation in this packet.

## Completion definition

The work is complete only after every acceptance criterion has evidence, the
domain/Catalog invariants are promoted to durable documentation and mechanical
checks, required profiles pass, generated outputs are synchronized, conflicts
with other active packets are resolved, promoted upstream completion evidence has
passed the pre-implementation audit, WP 02/WP 03 have explicit reusable
conformance interfaces and no-dual-schema evidence requirements, the
implementation is committed, the control thread is notified, and this temporary
packet is removed.
