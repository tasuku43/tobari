# Work Plan: Keep Catalog output declarations conformant with domain semantics

- Status: Accepted
- Decision: Fixed by Product Owner; implementation not started
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Repair the immediate `policy rules.items[].match` declaration by including both
domain-valid match values, then remove the independent-list failure mode for the
affected policy output family. The domain will expose canonical finite value
sets as caller-immutable copies; domain validators and every corresponding
Catalog enum will consume those sets. Constructor-built semantic fixtures will
exercise exact, template, deny, empty, nested, optional/nullable, and protocol
branches through the runtime validator. The existing exact fail-closed JSON
guard remains the last boundary.

The logically smallest repair remains the `path_template` declaration and its
regression. The physical implementation order remains fixed, and its Workspace
Manifest/domain plus copy prerequisites are now promoted in ADR 0079,
`docs/00` through `docs/04`, and integrated commits `07535a9`/`428812f`.
Reverify those authorities and the actual new HEAD/worktree in a read-only audit,
then rerun the synthetic reproduction. Apply the smallest
repair at the final Manifest/Workspace seam only if it is still missing; if
WP 01 already fixed or relocated it, add the regression and structural checks
without duplicating predecessor-schema code. Neither validator nor tests accept
both shapes.

In the same implementation packet, extend the existing Catalog declaration
walk so `ProducedRefs()` derives opaque references recursively through objects
and arrays and records canonical field paths. All Catalog graph validation,
scoped help/workflows, fixtures, and runtime output checks consume that single
result. WP 03 supplies future Runtime schemas and reference kinds, not a second
walker. Completed WP 02 supplies no-provenance schema evidence for audit, not a
common production derivation type.

The minimum repository invariant is:

> When a finite domain-owned string vocabulary crosses an exact Catalog output
> path, domain validation and every Catalog declaration for that semantic field
> use one canonical domain-owned value set. Every task-owned semantic branch
> that can generate those values has constructor-built conformance evidence
> rendered through JSON and compared with exact-command agent help. Schema
> migrations replace the exact paths atomically and rerun the same evidence;
> they never make predecessor and successor paths alternative valid schemas.
> Runtime Catalog validation remains mandatory and fail-closed.

And for references:

> Every opaque-reference field declared in a recursive Catalog output is
> discovered by one explicit bounded traversal of `OutputField.Fields` and
> `OutputField.Items`. The traversal returns exact reference kind plus canonical
> field path and is the only source for producer reachability, workflow/help
> projection, and reference conformance. Commands may not add a local walker or
> infer references from rendered JSON, labels, names, or Go reflection.

Implementation must inventory exact final post-WP01 Catalog paths before
claiming the invariant. The fixed initial set is the domain-semantic match,
protocol, decision, and state-change vocabularies reachable from
`PolicyRule`/`PolicyRuleReport`, across public `policy rules` and nested reviewed
policy output. CLI-only display, format, delivery, and other presentation enums
stay Catalog-owned; proximity is not a reason to commonize them. No broader
repository enum-unification claim is permitted.

## Alternatives considered

### Add only `path_template` to one enum

This is the correct immediate repair and should be the first failing-test slice.
It is not sufficient completion: the domain validator, public rule declaration,
nested decision declaration, protocol enums, and tests remain independently
editable. The next valid value could recreate the same format-dependent outage.

### Reflection over domain structs or constants

Rejected. Go constants are not enumerable through reflection, struct shape does
not reveal all reachable semantic values or cross-field correlations, and tags
would move domain meaning toward presentation metadata. Reflection could compare
field names and types but cannot prove that a reviewed constructor emits every
meaningful branch. It would add opacity without eliminating a semantic fixture.

### Generate domain types, renderers, and Catalog from one large DSL

Rejected for this scope. A repository-wide generator would create a broad new
authority, risk competing with `cli.Catalog`, touch unrelated commands, and make
small semantic reviews depend on generated machinery. It may reduce literal
duplication but would not by itself prove protocol correlations, empty versus
absent behavior, or task identity. The observed defect needs a small value-set
contract and focused typed evidence.

### Generate or import JSON Schema as the canonical contract

Rejected. Catalog is the repository's public command and output source of truth.
JSON Schema generation can be a derived publication format, but using it as an
additional authority would duplicate routing/help/reference facts and still
would not prove domain reachability. No external schema dependency is needed.

### Hand-maintain separate lists plus exhaustive tests

Better than the current state but still leaves validators and declarations free
to drift until a fixture happens to cover the new value. Canonical value sets
make equality structural; semantic fixtures then cover correlations that a set
alone cannot express.

### Add a Runtime-only nested-reference validator

Rejected. It would let Runtime help and actions work while every other nested
reference remained invisible to the Catalog graph, and it would create a second
source of producer truth beside `CommandSpec.ProducedRefs()`. WP 03's accepted
reference fields are the forcing fixture for a Catalog-wide correction, not a
reason for a lifecycle exception.

### Derive references by reflecting over JSON or Go output structs

Rejected. Rendered JSON is already downstream of the contract, Go field names
are not stable public paths, and neither source owns `ReferenceKind`. The
existing bounded `OutputField` tree already contains fields, arrays, and kinds;
a small explicit traversal is reviewable, deterministic, and sufficient.

## Design

### Public contract

- Immediate command: existing `tobari policy rules` only. ADR 0079 and promoted
  product/architecture contracts own any
  Catalog path, field, or reference-kind rename required by the final
  Manifest/Workspace model; this packet consumes that final declaration rather
  than designing it.
- Capability: existing public policy learning/inventory capability; no ledger
  addition or maturity change.
- Role/effect: `RoleDiscover`, `EffectRead`.
- Target and prerequisites: installation-wide learned-policy state scoped by
  trusted Manifest/Workspace principals; no new input, authentication, or
  provider prerequisite.
- References: each item continues to produce one opaque `policy-rule` reference;
  `policy reset` consumes it unchanged. No reference is reconstructed from
  request, match, examples, label, or ordering.
- Recursive reference declaration: a produced reference is described by exact
  kind and canonical output path. Top-level behavior stays compatible; nested
  object/array fields become visible to the same Catalog graph. Field paths are
  contract metadata, not a new public resource or reference encoding.
- Output: schema version 1, task `policy_rules`, complete delivery, exhaustive
  coverage. `items[].match` truthfully declares `exact` and `path_template`.
  At the post-WP01 checkpoint, final field names, required status, types,
  nullability, reference kinds, protocol values, and reset command remain
  stable; predecessor identity paths are negative canaries only.
- Human and JSON: the same typed rule supplies semantic identity, decision,
  protocol, scheme, authority, request, match, examples, sources, and
  protocol-specific fields. Human prose may differ in layout but not meaning.
- Agent help: exact-command help advertises the same finite match set and field
  correlations; routine discovery budget remains one exact help call followed by
  one command call, with zero external semantic-processing calls.
- Failures: valid template output is success/exit 0. A genuine declaration drift
  remains non-retryable `output_encoding_failed`, structured error schema 2,
  presentation phase, no stdout, and exit 13. For affected `policy rules`, its
  exact read-only next action is `version`; sibling faults stay unchanged for
  WP 10.
- Compatibility: exact-only outputs remain byte/meaning compatible except for
  scoped help's corrected enum/description. A previously failing valid template
  becomes successful JSON. No alias, deprecation, hidden value, or migration is
  introduced by the immediate repair. ADR 0079's integrated pre-public atomic
  schema replacement establishes the baseline: final policy output uses
  `workspace_manifest_id`/`workspace_id` identity and rejects predecessor
  Context/instance names rather than supporting a dual schema.
- Consumer gate: schema version 1 widening is fixed. Only concrete incompatible
  consumer evidence found in the mandatory audit blocks to the control thread;
  it never authorizes WP 08 to publish a parallel exact-only schema or bump.
- Downstream compatibility: WP 02 success schemas contain no provenance or
  retired `base` family fields. WP 03 declarations use only the accepted
  `runtime`, `runtime-revision`, and `runtime-prune-plan` kinds at their exact
  recursive paths. These are fixture obligations for their owning changes, not
  extra output fields created here.
- Concept count for the immediate repair: zero new public concepts and zero
  removed, hidden, or renamed concepts. WP 01 separately owns the accepted
  Context-to-Manifest replacement within its three-resource budget. This packet
  adds two internal invariants (finite-vocabulary equality and recursive
  reference derivation) and one focused conformance corpus.

### Authority and presentation identity

`LearnedPolicyRule` validation and the task-owned `PolicyRule` are semantic
authority. After WP 01, trusted `WorkspaceManifestID`/`WorkspaceID` binding
defines its authority scope and the opaque `policy-rule` reference is action
identity. The Catalog declares what a renderer may publish; it does not
authorize a rule or infer a template. A renderer maps validated values without
decoding references or deriving `path_template` from braces. Internal
`segments` remains unavailable in public output. Tests compare task, scope,
reference kind/value, and every carried request dimension before presentation.

### Layer changes

- Domain: add small canonical value-set accessors for the inventoried policy
  vocabularies, returning fresh slices or otherwise preventing callers from
  mutating shared validity state. Use the same sets in validation. Preserve
  typed constants and constructor invariants.
- Application: no production behavior change expected. Add/extend typed fixture
  construction at the application/CLI test boundary only if needed to retain the
  real `PolicyRuleReport` path. Do not add a presentation-shaped port.
- Infrastructure: no adapter or I/O behavior change. Existing state validation
  remains fail-closed. Tests use fakes and synthetic rules, not a live file.
- CLI and catalog: after the WP 01 gate, widen the final `policy rules` match
  declaration only if still needed; derive affected public and nested policy
  enums from domain-owned sets; add one bounded explicit recursive output-field
  traversal for produced reference kind/path; make existing graph/help/workflow
  consumers use it; keep renderer and fail-closed runtime checking intact.
- Promoted Workspace Manifest interface: ADR 0079 and `docs/00` through
  `docs/04` own the replacement concepts, Catalog paths/fields/reference kinds,
  persistence, and final semantic types. This packet audits those integrated
  surfaces with no predecessor fallback.
- Promoted copy interface: the durable product/architecture contracts provide
  separate typed Manifest-copy and Runtime-source-copy results. Shared
  conformance tests assert each exact schema
  omits provenance/lineage/inherited state and retired `--base`; no production
  common derivation model is introduced here.
- WP 03 interface: its owning change declares nested Runtime/revision and prune
  plan reference fields. The shared recursive traversal supplies graph paths,
  reachability, help, and fixture checking; WP 03 adds no local validator.
- Harness/generated mirrors: add conformance fixtures/answer keys and manifest
  digests as required; synchronize helper source and regenerate the architecture
  Catalog from canonical sources.

### Canonical value-set boundary

Prefer explicit functions such as `PolicyMatchValues()` over an exported mutable
slice. Each call returns a fresh ordered list suitable for validation, Catalog
copying, tests, and deterministic agent help. Apply the same pattern only to the
inventoried domain-owned policy vocabularies. Input/parser-only CLI enums remain
Catalog-owned and should not be moved merely for symmetry.

After the WP 01 gate, Catalog tests locate final exact field paths rather than
search by label:

- `policy rules` -> envelope `items[]` -> `match`;
- nested reviewed-policy decision output -> `decisions[]` -> `match`;
- the corresponding protocol, decision, and state-change paths identified by
  the inventory.

The tests use final `workspace_manifest_id` and `workspace_id` schema identity.
A negative public-vocabulary/schema canary rejects predecessor
`context`, `context_id`, `project_id`, and `instance_id`. The enum invariant is
parameterized by typed result and exact field path, not by any one generation's
resource noun.

Each declaration must equal the canonical set. The domain-owned accessor returns
caller-immutable values in one fixed order so agent help, generated artifacts,
and tests remain deterministic. That order is a presentation contract only; it
does not change meaning, compatibility authority, or reference identity. Tests
must fail when the domain adds a value without a declaration, a caller can
mutate shared order, or a declaration advertises a value domain validation
rejects.

### Recursive reference boundary

Extend the existing bounded output declaration traversal rather than introduce
reflection or a generator. Starting from each named top-level output field:

- append `.field` for nested object fields and `[]` for array items;
- record every string field with non-empty `ReferenceKind` as
  `(kind, canonical_field_path)`;
- preserve deterministic declaration order and reject duplicate/conflicting
  paths or invalid reference declarations through Catalog validation;
- include pagination cursor references through their existing explicit contract
  without double-counting them;
- leave consumed references derived from typed command inputs, then run the
  existing producer reachability and required-chain checks over the recursive
  produced set.

`ProducedRefs()`, Catalog validation, scoped agent help, workflow discovery,
contract fixtures, and any output/reference consistency checks must call this
same traversal. A command handler or Runtime package must not repeat it. The
runtime JSON validator remains the independent fail-closed check that emitted
values match the declaration; it does not become the producer-graph source.

Required WP 03 forcing fixtures include `items[].runtime_ref`, the final
approved revision path such as `runtime.revisions[].revision_ref`, and top-level
or envelope `plan_ref`. Their exact kinds are respectively `runtime`,
`runtime-revision`, and `runtime-prune-plan`; opaque bytes round-trip unchanged
into build/delete, restore, and prune-apply inputs. If WP 03 finalizes a
different envelope, update the exact path once rather than accepting both.

### Semantic conformance corpus

Build fixtures through public domain constructors, never by assigning an
otherwise invalid struct:

1. Exact HTTP allow.
2. Single-segment HTTP path-template allow created from two compatible synthetic
   denial candidates, with `/items/123`, `/items/456`, and `/items/{id}`.
3. Exact deny with an explicit empty `examples` array.
4. Empty exhaustive report with `items: []` and stable reset metadata.
5. Exact GraphQL, MCP, AWS, Kubernetes, Git, and OCI cases sufficient to prove
   required protocol-specific strings are populated only for their protocol and
   are explicit empty strings otherwise.
6. Nested reviewed decisions containing exact and path-template values.
7. Negative non-HTTP template, unknown match/protocol/decision/state-change,
   insufficient examples, wrong scope/task/reference kind, extra field,
   missing required field, wrong type, illegal null, and undeclared enum cases.

The committed regression fixture uses only post-WP01 constructors and final
scope fields. The bounded old-main reproduction remains in `context.md` as
diagnosis evidence; it is not copied into an accepted predecessor-schema test.
Its semantic scenario is replayed at the final exact path, and predecessor
identity fields appear only in negative no-dual-schema canaries.

ADR 0079's complete recursive Workspace Manifest and Workspace
desired/applied/observed outputs are declared conformance consumers. Their
semantic variants and exact shapes remain WP 01-owned; this packet adds no
placeholder public fields and does not decide revision retention, Git slice,
child-session, or migration-evidence semantics.

The pre-implementation audit consumes WP 02's two target-specific fixture
interfaces: Manifest copy publishes a fresh WorkspaceManifestID generation 1
after exact source revision revalidation; Runtime source copy publishes a fresh
RuntimeID with empty history. Both require absence of provenance, lineage,
source identity, copied Workspace/auth/permission/attachment/applied/failure/
observed/current-selection state, and retired `base` fields. WP 08 checks the
interface but adds no placeholder command or common production result/domain
type.

Future WP 03 instantiates typed output rows for Runtime list/show/history, prune
dry-run, and their action consumers. Rows cover nested arrays/objects, optional/nullable
fields, empty Runtime/revision/plan collections, exact reference paths/kinds,
and byte-preserving round trips. `last_used` remains explicitly unknown unless
WP 03 separately approves exact usage evidence; the conformance layer never
infers it from `reconciled_at` or reference proximity.

For optional/nullable coverage, reuse or extend the generic recursive validator
fixture with absent, present, explicit null, empty string/array, zero, and false
as distinct answer-key states. Do not contort `PolicyRule` fields into optional
ones if their command contract deliberately requires explicit empty strings.

### Text, JSON, and agent semantic consistency

One template scenario must pass through the real CLI dispatcher with a fake
application port:

- text exit 0 includes the synthetic request, `Match path_template`, examples,
  and exact reset reference without leaking internal segments;
- JSON exit 0 has schema 1 and validates against the Catalog before stdout;
- scoped agent help lists both match values and the same required/empty protocol
  field contract;
- the answer key compares semantic fields to the typed report, not text
  indentation or JSON field proximity;
- before migration it verifies current scope identity; after migration the same
  scenario verifies only `workspace_manifest_id`/`workspace_id` and rejects old
  public identity fields;
- the handler performs zero writes and zero external calls in both formats.

The existing key-shape helper remains useful but cannot be the acceptance proof.
Add a value/correlation assertion or semantic answer key instead of duplicating
the runtime validator wholesale.

### Data and control flow

```text
owner-only learned state (or synthetic fake)
  -> infrastructure validation
  -> policy-rules application query
  -> validated domain PolicyRuleReport
  -> CLI projection (no semantic inference)
  -> Catalog recursive validation
  -> stdout as text or schema-1 JSON

domain canonical value sets
  -> domain validation
  -> exact public/nested Catalog enum declarations
  -> scoped agent help and runtime JSON validation

WP 01 + WP 02 completion evidence
  -> audit actual new HEAD/worktree and inventory final declarations
  -> rerun synthetic reproduction at final field paths
  -> apply only still-needed repair and conformance checks
  -> reject every predecessor public alias

recursive Catalog OutputField tree
  -> one bounded explicit kind/path traversal
  -> ProducedRefs and producer reachability
  -> scoped help/workflows and conformance fixtures
  -> WP 03 Runtime/revision/plan reference consumers

WP 02 target-specific results
  -> shared schema conformance harness
  -> negative provenance/lineage/base-field canaries
  -> no shared production derivation model
```

No new side-effect boundary, process, filesystem path, network client, or secret
type is introduced.

### Error and cancellation behavior

- Domain-invalid persisted data continues to fail before projection using its
  existing read/data fault; it must not be reclassified as an encoding failure.
- Catalog/output drift continues to fail after task interpretation but before
  stdout with `output_encoding_failed`, schema 2, phase `presentation`,
  `change_state: not_applicable`, non-retryable status, no retry-after, and exit
  13. A negative test uses the validator/declaration boundary rather than adding
  a production escape hatch or mutable global Catalog.
- Preserve generic fault-mapping tests for empty stdout and exit 13. If proving
  the policy-specific mapping requires a helper seam, extract a small pure
  marshal/validate function that accepts an existing `CommandOutput`; do not add
  runtime Catalog injection or bypass validation.
- Cancellation remains non-mutating and cannot turn a successful read into
  replay permission. No mutation-complete handling applies.
- For the affected `policy rules` declaration failure, replace the self-looping
  next action with the exact read-only `version` command so the user can capture
  build identity for diagnosis. Do not alter fail-closed, non-retryable,
  schema-2, empty-stdout, or exit-13 behavior. Do not bulk-edit other Catalog
  faults; repository-wide recovery redesign is handed to WP 10.

### Security and public boundary

- Trust-boundary change: none. The same owner-only learned state crosses the same
  infrastructure/application/CLI boundaries.
- Authority change: none. Adding a declared enum value does not grant policy
  authority; only validated state and opaque references do.
- WP 01 identity rename changes schema vocabulary, not the trust boundary:
  final principals use trusted WorkspaceManifestID/WorkspaceID, while learned
  permissions remain separate from Manifest desired/applied state.
- Intent/target/impact: read only; mutation contracts are unchanged.
- Credentials/destinations/dependencies: none added. Do not use live provider or
  Workspace data.
- Fixture policy: deterministic `example.com` inputs, synthetic UUID-like values,
  fixed times, no real paths/hosts/IDs, and no confidential history.
- External text remains untrusted and passes through existing visible projection;
  opaque references bypass display projection exactly as before.
- Public generated outputs are derived, license-clean, and checked for drift.

## Implementation slices

1. **Mandatory WP01 + WP02 completion audit.** Verify ADR 0079, promoted
   `docs/00` through `docs/04`, and integrated commits `07535a9`/`428812f` on the
   actual HEAD. Fetch/read-only compare HEAD and upstream, protect the working
   tree, reread the durable contracts, and inventory domain/Catalog/schema/renderer/tests/
   helper/generated changes, and rerun the isolated synthetic text/JSON/agent
   reproduction. Amend implementation coordinates if the seam moved. A concrete
   incompatible enum consumer is `WP08_BLOCKED`; do not invent dual schema or a
   version bump. Do not write overlapping production files before this gate.
2. **Immediate regression on the actual seam.** Add the constructor-built
   failing regression at final Manifest/Workspace paths. If the bug remains,
   make the minimum declaration correction; if already repaired, add no
   duplicate patch. Prove text, JSON, scoped agent help, fault, exit, and
   no-dual-schema behavior.
3. **Finite-vocabulary invariant.** Inventory the affected policy vocabularies;
   add domain-owned immutable value sets and make validation plus every exact
   public/nested Catalog declaration consume them. Expand exact/template/deny/
   empty/protocol/nested/optional fixtures and negative canaries.
4. **Recursive reference invariant.** Add failing generic nested object/array
   producer tests, implement one explicit bounded output-field traversal, and
   route `ProducedRefs()`, graph/reachability, scoped help/workflows, and
   conformance fixtures through it. Preserve top-level and cursor behavior;
   reject duplicate/conflicting paths. Add no Runtime-specific code.
5. **Promoted final-schema consumers.** Register complete Manifest/Workspace
   desired/applied/observed declarations at their actual final paths, retaining
   ADR 0079 ownership of shapes and promoted choices. Prove predecessor names are
   rejected, never normalized.
6. **WP02/WP03 schema-fixture interfaces.** Audit completed WP 02 against the
   target-specific no-provenance/no-`base` requirements and publish exact nested
   Runtime/revision/plan reference path/kind requirements for future WP 03. WP 08
   records interfaces only and never registers nonexistent downstream commands;
   WP 03 instantiates its rows after WP 08 completes.
7. Run the common corpus under standard and experimental tags; synchronize
   helper-source snapshots, generated architecture Catalog, harness fixtures,
   answer keys, and digests from canonical inputs.
8. Promote both invariants to Harness and, if durable review confirms a gap,
   Architecture. Reconcile conflicting temporary packets.
9. Run focused, full, security, public, and release gates. Promote durable
   invariants, commit implementation completion, remove this temporary packet in
   the completion handoff, and notify the control thread before WP 03 starts.

## Verification

- Unit and contract tests: domain value-set/validation equality; constructor
  template tests; application report validation; exact field-path Catalog enum
  equality; recursive object/array reference kind/path derivation; duplicate/
  conflict/cycle/reachability negatives; CLI text/JSON/agent consistency; a
  reusable exact-path seam for WP 01/WP 02/WP 03.
- Negative side-effect tests: both formats make zero writes/network calls;
  invalid state and declaration drift fail before stdout.
- Opaque-reference and complete-pagination tests: exact reference round trip to
  `policy reset`; nested Runtime/revision/plan refs round-trip unchanged to their
  accepted action inputs; complete/exhaustive output has no cursor and preserves
  empty scope; existing paged cursor derivation is not double-counted.
- Structured output, hostile-output, and recovery tests: nested types/enums,
  extra/missing fields, nullability, controls/Unicode projection, schema-2
  `output_encoding_failed`, empty stdout, exit 13, and the affected policy-only
  read-only `version` recovery without changing sibling command faults.
- Agent-readiness scenario and discovery-round-trip count: one exact help lookup
  plus one `policy rules --format json` execution; zero external-processing
  calls; no provider-notation decoder or exploratory query.
- Human-handoff scorecard: not applicable; no setup or authentication candidate.
- Manual observation: isolated temporary XDG home and synthetic in-memory state;
  compare text, JSON, and scoped agent help from current source.
- Required profiles: focused Go tests; standard and
  `tobari_dev tobari_experimental` CLI tests; `task check`; `task security`;
  `task public:check`; `task release:check`.
- Generated-diff or artifact checks: helper-source sync/check,
  architecture-site Catalog generation/check, `.harness/schemas.json` digest
  validation, and a final diff proving no live state or unrelated generated file
  entered the change.
- WP 01/WP 02 evidence consumed at the pre-implementation audit: final-path
  success, predecessor-path rejection, target-specific no-provenance copy
  schemas, and the same policy conformance corpus against final
  Manifest/Workspace identities.
- Future WP 03 evidence, owned by its implementation change: recursive
  Runtime/reference fixtures use this packet's shared traversal with no second
  validator and no WP08-created placeholder commands.

## Rollout and rollback

No persisted-state migration is required for the immediate match repair.
Existing exact and template state is already valid under domain and
infrastructure validation. That repair keeps the current schema version 1 and
changes only the enum/description plus conformance enforcement.

ADR 0079 and the promoted docs define the integrated pre-public predecessor
migration: old Context UUID bytes become WorkspaceManifestID and old
ProjectInstance UUID bytes become WorkspaceID, while final public readers expose
only Manifest/Workspace vocabulary. This packet neither reimplements that
migration nor adds a dual reader; it reruns conformance only against final
Catalog paths and treats the old-main result as diagnosis evidence.

WP 02 completion evidence is audited before WP 08 starts. WP 03 has an accepted
design contract but starts only after WP 08 implementation completes. This
packet does not reserve or pre-edit future WP 03 command/schema files, and it
does not make old/new fields or flags coexist to stage either cutover.

Roll forward is preferred: reverting only the enum repair would restore the
valid-state JSON outage. If a later conformance harness causes unrelated
failures, narrow or stage the harness only after documenting the un-inventoried
vocabulary; do not revert runtime validation or ADR 0037. No secret or non-secret
state cleanup is needed.

## Documentation promotion

- Add the domain-owned finite-vocabulary/Catalog equality rule, recursive
  reference kind/path derivation rule, and required semantic-branch evidence to
  `docs/04_harness.md`.
- Clarify `docs/02_architecture.md` only if review finds its current recursive
  Catalog contract does not already imply the ownership/equality rule.
- Update ADR 0037's mechanical enforcement evidence if the conformance corpus is
  part of its durable validation story; do not change its accepted semantics.
- No thesis, product-semantic, security-boundary, authentication, or external
  API change is expected from the immediate repair. ADR 0079 and `docs/00`
  through `docs/04` already contain the promoted domain-model revisions. If this packet reveals another
  such change, stop and revise the governing decision before mechanism work.
- Reconcile or supersede exact-only statements in related temporary work packets
  before public-release evidence is finalized.
- Coordinate promotion against the durable Workspace Manifest vocabulary. The
  generic invariant must be written without freezing predecessor nouns, and
  ADR 0079's recursive Manifest/Workspace schemas must cite it as an
  interface requirement rather than being designed in this packet.

## Fixed cross-cutting decisions

- Initial domain-owned vocabulary scope is match, protocol, decision, and
  state-change reachable from final `PolicyRule`/`PolicyRuleReport`; exact field
  paths are inventoried after WP 01/WP 02 completion without global enum claims.
- `path_template` is a schema-1 enum widening. Concrete incompatible-consumer
  evidence blocks to the control thread; WP 08 never adds dual schema or a bump.
- Canonical order is deterministic caller-immutable presentation, not semantic
  compatibility or authority.
- The mandatory order remains WP 01 + WP 02 completion audit -> WP 08 -> WP 03.
  ADR 0079, promoted `docs/00` through `docs/04`, and commits
  `07535a9`/`428812f` are the upstream completion evidence to reverify before any
  WP 08 implementation write.
- The affected policy recovery becomes read-only `version`; WP 10 owns any
  all-command recovery redesign.
- One explicit recursive `OutputField.Fields`/`Items` traversal is the
  Catalog-wide producer-reference derivation. Consumed refs remain input-derived
  and WP 03 cannot add a local walker.
- Former WP 01/WP 02 choices are consumed only through ADR 0079 and the promoted
  final schema/contracts. WP 02
  no-provenance/no-`base` and WP 03 nested-reference fixtures are interfaces;
  WP 08 does not register nonexistent commands or common derivation types.
- The implementation change owns helper-source/generated synchronization,
  stale exact-only packet reconciliation, all named gates, final commit, packet
  removal, and control-thread notification.
