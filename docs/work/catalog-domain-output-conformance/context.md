# Work Context: Keep Catalog output declarations conformant with domain semantics

This file separates verified facts from evaluation, unknowns, and proposed
design. All reproduction data below is synthetic.

## Current behavior

### Verified facts

- On 2026-08-23, `git fetch origin main --prune` left local `main` and
  `origin/main` at `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42` with zero commits of divergence;
  the worktree was clean before investigation.
- Later on 2026-08-23, the shared checkout was on
  `codex/workspace-manifest-v1`; `HEAD` and `origin/main` still named the same
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42` commit, while the working tree
  contained extensive unstaged WP 01 production changes across domain,
  application, infrastructure, CLI/Catalog, tests, helper source, tools, ADRs,
  and work packets. In particular, `internal/domain/tobari/policy.go`,
  `internal/cli/runtime_catalog.go`, and their helper-source copies are in the
  active change set. Those files were inspected read-only and were not used as
  implementation targets during packet design.
- The current integration HEAD is `52a53bcc69a0f2bdf9bf2a6782ecd98bacd8b0e1`.
  It contains promoted upstream commits `07535a9` and `428812f`, durable ADR
  0079, and aligned `docs/00` through `docs/04`. The former Domain Model V1 and
  Derivation / Copy temporary packet files are removed; only unrelated untracked
  work packets remain in `git status`.
- `internal/domain/tobari/policy.go` declares `PolicyMatchExact = "exact"` and
  `PolicyMatchPathTemplate = "path_template"`; `LearnedPolicyRule.Validate`
  accepts both.
- `internal/domain/tobari/policy_template.go` can construct a reviewed HTTP
  single-segment `{id}` path template from two compatible synthetic denial
  candidates. Domain validation rejects unsupported protocols and malformed or
  insufficient examples.
- `internal/app/tobaricmd/policy_queries.go` loads and validates learned and deny
  rules and returns a task-owned `PolicyRuleReport`. The application does not
  translate the match value.
- `internal/cli/tobari.go` copies `PolicyRule.Match` into both the JSON projection
  and the human renderer. Human output therefore displays `Match path_template`.
- `internal/cli/runtime_catalog.go` declares
  `policy rules` item field `match` with enum `exact` only. The nested internal
  `policy apply-reviewed` decision declaration already lists `exact` and
  `path_template`.
- The same current-main `policy rules` item declaration uses `context_id` and
  `context` for Manifest authority/name, and already uses `workspace_id` and
  `project_root` for Workspace/root identity. Those current names are verified
  predecessor facts, not the target public V1 vocabulary.
- `internal/cli/machine_output.go` marshals output and then recursively validates
  the document against the exact command output contract. It checks nested
  keys, types, required/optional/nullable status, enums, references, envelopes,
  and collection coverage before writing stdout.
- At the investigated baseline, `OutputField` already models recursive
  `Fields` and `Items`, and Catalog validation recursively checks them. However,
  `CommandSpec.ProducedRefs()` examines only top-level
  `Agent.Output.Fields`; `ConsumedRefs()` derives command inputs. Help workflow
  construction, producer reachability, and Catalog graph validation all consume
  those derivation methods. A nested field may therefore validate as a
  reference at output time but remain absent from the producer graph.
- On a valid template mismatch, `renderPolicyRulesWithCommands` maps that
  presentation error to `output_encoding_failed`; the global structured fault
  uses schema version 2 and exits 13.
- `TestPolicyRulesJSONIsReadOnlyAndMatchesCatalog` creates exact rules only and
  asserts item-field names. The generic recursive JSON tests prove that an enum
  mismatch is rejected, but do not prove that every domain-reachable policy
  match satisfies the `policy rules` declaration.
- `go test ./internal/domain/tobari ./internal/app/tobaricmd ./internal/cli`
  passed on the current commit before and after the bounded reproduction. The
  existing tests therefore do not detect this reachable mismatch.
- Current-source scoped agent help, run through `go run ./cmd/tobari help policy
  rules --format agent` with an isolated temporary XDG home, succeeds and
  advertises only `enum: ["exact"]` for `items[].match`.
- The checked-in `bin/tobari` identifies an older source commit, so it was not
  used as evidence for current `main`; current-source `go run` and tests were
  used instead.
- ADR 0079 replaces public Context with host-owned, CLI-managed,
  revisioned Workspace Manifest; final CLI/schema spellings are `manifest`,
  `--manifest`, and `workspace_manifest_id`, with no public alias. A Workspace
  binds `(ProjectRoot, WorkspaceManifestID)` and uses `workspace_id` as its only
  instance authority. Standard authentication, learned permission, and
  attachment authority remain outside Manifest desired/applied state.
- The Workspace Manifest/domain implementation and copy semantics are now
  promoted durable upstream contracts. Their retained-revision, Git slice,
  child-session, migration-evidence, and migration-precondition representations
  must be consumed from ADR 0079 and `docs/00` through `docs/04`, not
  reconstructed from deleted temporary packets.
- Promoted `docs/00` through `docs/04` fix Manifest copy and Runtime source-copy as separate
  one-time initializers with fresh identities and no persisted or presented
  provenance. It removes `--base` without an alias and explicitly rejects a
  shared production derivation type.
- Accepted WP 03 fixes Runtime discovery/action references as `runtime`,
  `runtime-revision`, and `runtime-prune-plan`. Its natural outputs include
  nested paths such as `items[].runtime_ref` and revision references, while
  prune dry-run emits an exact `plan_ref`; WP 03 explicitly delegates recursive
  output-reference derivation to this Catalog-wide packet.
- Product Owner fixed WP 08 detailed design: initial domain-owned vocabulary
  scope, deterministic non-authoritative order, schema-1 enum widening,
  policy-only `version` recovery, one recursive Catalog producer walker,
  WP 01 + WP 02 audit -> WP 08 -> WP 03 ordering, interface-only downstream
  fixtures, required gates, commit, and control-thread completion/blocking
  notification. These are accepted constraints, not remaining design choices.

### Evaluation

- The JSON renderer is correctly enforcing a fail-closed contract. Relaxing or
  bypassing runtime validation would hide the declaration error and violate the
  Catalog and semantics-before-presentation theses.
- The immediate defect is a missing `path_template` Catalog enum member. The
  structural defect is that a domain-owned finite semantic vocabulary and its
  Catalog projections have independent handwritten lists without a complete
  semantic-branch fixture.
- Key-shape equality is necessary but insufficient for output conformance. A
  test must exercise values reachable through domain constructors and validate
  them through the same Catalog declaration used at runtime.
- The failure is public-contract relevant because README documents JSON policy
  inventory and reset flow, even though the repository is still prerelease.
- The conformance defect and invariant are independent of whether the scope
  field is currently named `context_id` or finally named
  `workspace_manifest_id`. Binding a test helper to the predecessor spelling
  would turn the immediate repair into migration debt.
- Recursive output validation and recursive reference-graph derivation are two
  views of the same Catalog declaration. Keeping the former recursive and the
  latter top-level-only would make runtime JSON truthful while agent workflow
  discovery and required-reference reachability remain incomplete.
- A small explicit traversal over the existing `OutputField` tree is the
  minimum structural fix. It preserves Catalog authority and yields stable
  field paths without reflection, output structs, or a Runtime-specific
  registry.

### Inference

- Commit history indicates the drift was introduced across two independently
  valid changes: `58f811d` introduced the current exact-only policy-rule Catalog
  declaration, while `d9df267` later added the path-template domain and renderer
  path without widening that public declaration. This is evidence for a missing
  cross-layer invariant, not evidence of a renderer defect.
- Because durable product, architecture, security, and ADR text already admit
  the reviewed path-template rule, the likely correct compatibility action is
  to repair the schema-1 enum without a schema bump or state migration for that
  immediate change.
- Because ADR 0079 is the accepted upper-level pre-public reset, its rename is not
  an additive compatibility extension. The immediate repair was logically the
  smaller first change, but the promoted upstream implementation is now in the
  integration HEAD. The safe order is to reobserve that actual final seam, then
  apply only the still-needed regression repair and
  generic invariants. Old/new field names must never become alternative valid
  shapes.

## Relevant structure

- Entry point: `cmd/tobari` -> `internal/cli` dispatcher -> `policy rules`.
- Domain rule: `internal/domain/tobari/policy.go`, `policy_template.go`, and
  `policy_rule_report.go` own valid policy semantics and task identity.
- Application use case: `internal/app/tobaricmd/policy_queries.go` owns the
  exhaustive inventory read and validates loaded state before returning it.
- Infrastructure boundary: `internal/infra/dockerruntime` reads owner-only
  learned-policy state and satisfies the application port; no network call is
  involved.
- CLI catalog or presentation: `internal/cli/runtime_catalog.go` declares output
  shape and enum values; `internal/cli/tobari.go` projects human and JSON output;
  `internal/cli/machine_output.go` enforces the declaration.
- Catalog reference graph: `internal/cli/catalog.go` owns recursive
  `OutputField` declarations, `ProducedRefs()` / `ConsumedRefs()`, Catalog
  validation, and required-reference reachability; `internal/cli/help.go`
  derives scoped help and workflows from those methods.
- Generated mirrors: `internal/infra/runtimeassets/_helper-source` contains the
  checked helper-source closure, and
  `docs/architecture-site/src/generated/catalog.json` is generated from the
  Catalog. Neither is a second editable authority.
- Existing tests and harness checks: domain policy/template tests, application
  query tests, CLI policy presentation tests, recursive output-contract tests,
  `.harness/schemas.json`, standard `task build`, experimental `task build:dev`,
  and `scripts/check.sh` profiles.
- Promoted upstream authority: ADR 0079 and `docs/00` through `docs/04` own the
  final Manifest/Workspace concepts, desired/applied/observed schemas, Catalog
  renames, predecessor migration, copy commands, and no-provenance semantics;
  `07535a9`/`428812f` provide integration evidence.
- Accepted downstream design input: `docs/work/runtime-retirement/` owns Runtime
  lifecycle commands, reference kinds, and exact output envelopes. It has not
  authorized production implementation through this packet.

## Owner, scope, lifetime, mutability, and identity

- Semantic owner: the Tobari policy domain owns which match, protocol, decision,
  and state-change values are valid and which combinations are meaningful.
- Declaration owner: `cli.Catalog` owns the public field paths, types, required
  status, nullable status, enum declarations, reference kinds, delivery, and
  coverage for each command.
- Reference-graph owner: the Catalog derives produced references recursively
  from those declarations and consumed references from typed inputs. Canonical
  paths identify presentation locations; the opaque reference bytes and kind,
  not the path or label, remain action authority.
- Presentation owner: CLI renderers map a validated task result into human and
  JSON forms; they do not create policy authority.
- Scope: one installation-wide learned-policy inventory spanning Workspace
  Manifests and their bound Workspaces. Collection coverage is exhaustive and
  delivery is complete; there is no cursor or pagination input. Each rule's
  authority scope is the trusted `workspace_manifest_id`/`workspace_id`
  binding on the post-WP01 implementation baseline.
- Lifetime: learned allow and deny rules persist until reset. A projected report
  exists only for one read execution.
- Mutability: `policy rules` is `RoleDiscover` plus `EffectRead`; it must not
  mutate learned policy. `policy reset` remains the separate reference-bound
  action consuming one opaque `policy-rule` reference.
- Authority identity: the validated domain `PolicyRule`, its trusted Manifest
  and Workspace identity binding, and its opaque rule reference are
  authoritative for semantic identity. `workspace_manifest_id` and
  `workspace_id` are the final schema identities; Manifest names, project root,
  paths, match text, examples, order, indentation, and Catalog descriptions are
  presentation data and cannot independently authorize reset.

## Public and internal concepts

- Public concepts added by the immediate repair: zero. `path_template` is an
  existing accepted match value whose JSON and agent declaration becomes
  reachable and truthful.
- Public concepts removed, hidden, or renamed by the immediate repair: zero.
- ADR 0079 removes public Context and renames it to Workspace Manifest;
  that does not add a fourth durable resource here. This packet consumes the
  final three-resource budget (Workspace Manifest, Runtime, Workspace) without
  owning the rename.
- Internal design concepts proposed: one domain-owned finite-vocabulary source
  for the affected policy output family, one focused semantic conformance
  corpus, and one explicit recursive Catalog output-field walker reused for
  reference derivation. These are enforcement mechanisms, not public concepts
  or resources.
- Existing internal-only `segments` remains internal. JSON must not infer or
  expose it merely because the renderer sees a template path.

## Constraints

- The Catalog remains the only public command and invocation registry. Domain
  value ownership must not become a competing command/output registry.
- Domain cannot import CLI. CLI may consume exported, immutable-by-caller copies
  of domain-owned value sets through the existing dependency direction.
- The initial finite-vocabulary scope is exactly domain-semantic match,
  protocol, decision, and state-change reachable from final post-WP01
  `PolicyRule` / `PolicyRuleReport`. CLI-owned display, format, delivery, and
  other presentation enums are excluded; inventory must not mechanically move
  non-domain values into a shared abstraction.
- Canonical value order is fixed for deterministic help, generated artifacts,
  and tests, and callers cannot mutate the returned slice. Order does not carry
  semantic compatibility, identity, or authority.
- The JSON runtime guard remains exact and fail-closed. No special case may allow
  valid-looking output around a mismatched declaration.
- Human, JSON, and agent help must describe the same semantics, but their bytes
  need not be identical.
- Required strings that are protocol-inapplicable currently use explicit empty
  strings in `policy rules`; optional and nullable have distinct meanings and
  tests must not collapse absent, empty, zero, false, or null.
- Fixtures use `example.com`, deterministic timestamps, and synthetic opaque
  identifiers. No live learned-policy state, host name, path, user identifier,
  credential, or private history enters the packet or repository fixture.
- Standard and experimental builds share the runtime Catalog core. Common tests
  must run in both tag sets so future profile composition cannot conceal drift.
- `task check` is the implementation completion gate. Public and release gates
  are additionally required because the change repairs an advertised machine
  contract and generated publishable Catalog material.
- The match repair and WP 01 cutover are separate compatibility decisions, but
  the shared-checkout order now places implementation of this repair after the
  WP 01 gate. The repair changes only the final match declaration; it does not
  restore predecessor paths/fields/reference kinds. No test may normalize both
  schemas into success or allow either spelling at runtime.
- The generic invariant is keyed by typed semantic vocabulary plus exact
  Catalog output path, not by the words Context, Manifest, project, or instance.
  It must be reusable for ADR 0079's complete recursive Manifest and Workspace
  desired/applied/observed schemas when their exact shapes are approved.
- Canonical reference paths must distinguish object fields and array items (for
  example `items[].runtime_ref`) and be produced by the same explicit traversal
  everywhere. Do not infer them from JSON bytes, Go struct reflection, rendered
  labels, or command-specific field searches.
- WP 02 fixtures are negative schema canaries, not a reason to add lineage or a
  common derivation abstraction. WP 03 reference fixtures are consumers of the
  Catalog-wide traversal, not permission for a Runtime-only validator.
- WP 01, WP 02, and WP 03 cutovers are atomic pre-public contracts. A test that
  normalizes predecessor and successor enums, paths, flags, or JSON fields into
  one accepted form is invalid even if it makes a shared fixture convenient.
- The enum widening remains schema version 1 with no parallel exact-only
  contract. Concrete incompatible-consumer evidence at the mandatory audit is a
  control-thread `WP08_BLOCKED` condition, not authority to add a dual schema or
  version bump.
- The affected `policy rules` `output_encoding_failed` recovery points to the
  read-only exact command `version`. All-command recovery redesign is excluded
  and handed to WP 10.
- Implementation order is WP 01 + WP 02 completion audit, WP 08, then WP 03.
  Absence of the promoted ADR/docs and integrated completion commits at the
  implementation audit is an absolute stop.

## External facts

- Go `encoding/json` package documentation, https://pkg.go.dev/encoding/json,
  checked 2026-08-23: `Marshal` recursively encodes exported values and struct
  fields. It does not know Tobari's domain vocabulary or Catalog declarations;
  Tobari must perform that validation at its own boundary.
- JSON Schema, “Enumerated values,”
  https://json-schema.org/understanding-json-schema/reference/enum, checked
  2026-08-23: an enum constrains a value to a fixed set of unique elements.
  Tobari's Catalog is not delegated to JSON Schema, but the finite-set semantics
  support treating a missing valid value as a declaration error rather than an
  encoding exception.
- No provider API, external data source, or third-party schema is involved in
  the defect or proposed repair.

## Unknowns

- [ ] At the implementation audit, reverify commits `07535a9`/`428812f`, the
      upstream relationship, worktree ownership, and exact
      final PolicyRule/PolicyRuleReport and Manifest/Workspace Catalog paths.
- [ ] At that audit, determine whether `path_template` remains missing, moved,
      or already repaired, and whether any concrete consumer disproves the
      accepted schema-version-1 widening. Consumer evidence blocks and returns
      to the control thread; it does not reopen dual-schema design locally.
- [ ] Enumerate every exact final Catalog field path for the fixed match,
      protocol, decision, and state-change scope, and identify the canonical
      presentation order already owned by the final domain vocabulary. Do not
      expand scope merely because another enum is nearby.
- [ ] Determine whether Harness alone already states both durable invariants or
      Architecture needs a short clarification; this affects promotion location,
      not the accepted mechanism.
- [ ] Reconcile stale exact-only packet language and inspect the final WP 01/WP 02
      shapes needed by conformance fixtures without reopening their decisions.
- [ ] Before WP 03 begins, record its final exact nested output paths for
      `runtime`, `runtime-revision`, and `runtime-prune-plan` fixtures. WP 03 may
      instantiate the interface but may not add a second walker.

## Thesis evidence

- Repeated design decision or point of agent confusion: the domain match values,
  the public rule Catalog enum, and the nested reviewed-decision enum were
  maintained as separate literals; only two currently agree.
- User outcome or friction observed in the minimal slice: a valid read succeeds
  for humans and fails for JSON/agents, so the same state has format-dependent
  availability and misleading recovery.
- Code workaround or exception being considered: adding only one Catalog string
  repairs today's fixture but leaves the same drift path for another match,
  protocol, decision, nested field, or build profile.
- Current thesis that resolves it: Catalog is the output source of truth, claims
  are executable, and task semantics precede presentation. No thesis change is
  needed; mechanical coverage is incomplete.
- Downstream impact: domain vocabulary export, Catalog declaration reuse,
  explicit recursive reference derivation, semantic fixtures, CLI contract
  tests, helper-source synchronization, architecture-site Catalog generation,
  harness schema digests, and public and release gates. WP 01's recursive
  Manifest/Workspace declarations and WP 02/WP 03 schemas become consumers.
  Authentication and external API contracts remain unchanged.

## Reproduction or observation

The bounded reproduction temporarily added one untracked CLI test, ran it, and
removed it before packet authoring. It constructed two candidates through domain
constructors using only `api.example.com`, `/items/123`, `/items/456`, fixed
timestamps, and deterministic synthetic identifiers. It exercised the normal
dispatcher and fake application port; it did not read or mutate a user's Tobari
state.

```sh
go test ./internal/cli -run TestTemporaryPolicyRulesPathTemplateReproduction -v
go test ./internal/domain/tobari ./internal/app/tobaricmd ./internal/cli
```

Observed on macOS/arm64 at current `main`:

- Text: exit 0; stdout contained the synthetic `/items/{id}` request,
  `Match path_template`, both examples, and the reset command; stderr was empty.
- JSON: exit 13; stdout was empty; stderr was the structured schema-2
  `output_encoding_failed` contract fault with phase `presentation`,
  `change_state: "not_applicable"`, `retryable: false`, and `policy rules` as
  the current next command.
- Existing focused packages: pass.
- Worktree after cleanup: clean.

## Security and public-boundary notes

- Assets and side effects involved: owner-only non-secret learned-policy state is
  read. The command is read-only and the reproduction used an in-memory fake.
- Credentials or confidential data involved: none. Standard native
  authentication and experimental Broker behavior are out of scope.
- New dependencies, destinations, files, processes, or generated content: no new
  dependency or destination. Future implementation changes canonical Go/tests,
  durable docs, checked helper source, generated Catalog JSON, and synthetic
  harness fixtures/digests only.
- External schema provenance, publication rights, and drift evidence: no copied
  external schema. Generated public material derives from repository-owned
  Catalog and fixtures.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency,
  and cancellation facts: complete delivery, exhaustive coverage, no pagination,
  no external timeout/retry, read idempotency, and context cancellation before
  presentation. A confirmed read has no mutation-complete boundary.
- Identity cutover: predecessor `context_id`/`context` output is current-main
  evidence only. Final V1 uses `workspace_manifest_id` and Workspace Manifest
  presentation, retains `workspace_id`, and exposes no dual schema. Stable ID
  byte preservation is owned by the ADR 0079 migration contract.
- Publication and licensing concerns: synthetic content only; run public guard
  and release checks. Do not commit temporary runtime homes or live policy files.

## Related packets and conflict map

- ADR 0079 plus promoted `docs/00` through `docs/04` are the accepted upstream
  authority for the Workspace Manifest/Runtime/Workspace budget, identity,
  activation, desired/applied/observed separation, migration, copy commands,
  fresh identities, state exclusion, and no-lineage contract. The implementation
  gate audits the integrated truth recorded by `07535a9`/`428812f`; it does not
  revive removed temporary packet authority.
- `docs/work/runtime-retirement/` is design-fixed but has no implementation-start
  signal. It starts only after WP 08 completes and depends on this packet for one
  recursive Catalog reference walker
  and field-path-aware producer/consumer graph. It owns lifecycle commands,
  ref kinds, envelopes, state, journals, protection, faults, and mutations. A
  WP 03-specific validator would conflict with this packet and is forbidden.
- The fixed integration order is WP 01 + WP 02 completion audit -> WP 08
  implementation -> WP 03 implementation. This packet audits WP 02 and never
  registers absent WP 03 commands as placeholders.
- `docs/work/policy-compaction-retirement/` is active but contains exact-only V1
  assumptions that conflict with accepted ADR 0037 and current source. It must be
  reconciled or superseded; it cannot override the higher-precedence durable
  contract.
- `docs/work/first-public-release-core/` contains older exact-only policy wording
  and coordinates Catalog/schema publication. This repair must land before its
  final public contract evidence is frozen.
- `docs/work/capability-profiles-first-prerelease/` depends on standard versus
  experimental Catalog integrity. This packet supplies a shared-core
  conformance requirement, not a new profile capability.
- `docs/work/first-public-release-artifacts/` owns release artifact evidence and
  must observe regenerated Catalog material and passing release/public gates.
- No authentication or external API packet is a mechanism dependency.

## Glossary

- Domain-owned finite vocabulary: a closed set of semantic string values whose
  validity is decided by a domain type, not by a renderer.
- Workspace Manifest: ADR 0079's host-owned, CLI-managed, stable-ID revisioned
  desired declaration; routine label `Manifest`. It is not a project file or a
  container of learned permission/authentication/attachment state.
- Workspace identity: the durable instance authority `workspace_id`, permanently
  bound to `(ProjectRoot, WorkspaceManifestID)`; old `project_id` and
  `instance_id` are not final semantic aliases.
- Catalog enum: the finite set declared for one exact output field path in one
  command contract.
- Semantic conformance fixture: a constructor-built typed task result plus answer
  key used to prove meaningful values and correlations before presentation.
- Declaration conformance: every valid value emitted at a Catalog field path is
  accepted by that path's declaration, including nested and protocol-specific
  branches.
- Canonical reference field path: the unambiguous path produced by walking the
  Catalog declaration, with `[]` marking array items, such as
  `items[].runtime_ref`. It locates a produced opaque reference for contracts
  and help; it is not itself identity or action authority.
- Path template: the already accepted HTTP request path with exactly one reviewed
  `{id}` segment and retained examples; it is not a glob or regex.
