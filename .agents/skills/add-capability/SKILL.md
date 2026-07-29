---
name: add-capability
description: >
  Use when adding or changing a user-visible CLI command, workflow, integration,
  side effect, output contract, or external adapter. Guides thesis alignment,
  layer ownership, tests, documentation, and repository gates.
---

# Add a CLI Capability

Before designing the change, run:

```sh
go run ./tools/projectmeta --field profile
```

If the profile is `template`, stop and use `$bootstrap-derived-cli`. Do not add
a capability while the repository still has the foundry identity or generic
product reasoning.

Read `docs/00_theses.md` before designing the change. A capability is complete
only when its user outcome, safety boundary, discoverability, and verification
are explicit.

For a non-trivial capability, create `docs/work/<change-name>/` from the work
packet template and keep its goal, verified context, chosen plan, tasks, and
gate evidence current. Promote durable conclusions into governing documents;
the packet is evidence, not a second product contract.

## 1. Define the user outcome

Write one sentence describing what the user can accomplish. Prefer task language
over an upstream API resource or implementation name.

Confirm:

- the capability belongs in this CLI;
- an existing command cannot express it naturally;
- input ambiguity has a deterministic resolution or is surfaced to the user;
- stdout, stderr, exit status, and machine-readable output are predictable;
- all side effects and external destinations are named.
- the command's stable capability ID is `public` in `.harness/capabilities.json`, or the upstream capability remains explicitly `internal`, `deferred`, or `excluded` with a reason.
- routine success needs zero undeclared external-processing steps: extracting a
  declared JSON/TSV field is allowed, but an extra `jq`/`grep` join, custom
  parser, provider-notation decoder, source inspection, or exploratory API call
  means the supported outcome is not operationally closed;
- any deliberately raw export or low-level utility states that narrower outcome
  explicitly instead of standing in for a composed user task.

If the thesis does not decide a design trade-off, update the thesis or an
architecture decision before implementation.

Separate discovery from action. Discovery commands may accept ambiguity and
must return stable, opaque IDs. Acting commands accept one opaque ID or another
explicitly unique selector; they must not guess among candidates. Record each
ID producer, its output field, and every consumer so contract tests can prove
that users and agents can pass the value through unchanged.

## 2. Place responsibilities in the four layers

- `internal/domain`: domain types, invariants, and pure policy.
- `internal/app`: a use case and consumer-owned ports.
- `internal/infra`: external systems and concrete port adapters.
- `internal/cli`: argument parsing, composition, help, and rendering.

Domain code imports no outer layer. Application code does not import
infrastructure or CLI code. Infrastructure code does not import application or
CLI code. The CLI is the composition root. `tools/archlint` enforces this
contract from the module path reported by `go list`.

Keep policy out of transport adapters and presentation code. Inject clocks,
filesystems, environment reads, network clients, and side-effect executors at
the narrowest useful boundary.

Validate adapter results before presentation. A task result must belong to the
declared task and every request dimension that task actually carries: target,
parent, and/or scope. A scoped collection's task-owned result retains scope even
when empty. Keep absent, explicit empty, zero, false, and unresolved states
distinct when they affect interpretation. Validate every returned opaque value
against the reference kind required by its semantic field, not merely a shared
byte shape. Presentation represents these facts and must not infer them from a
display name, order, proximity, quoting, or indentation.

## 3. Declare the operation contract

For every external action, specify:

- effect: read, create, or write; unknown is never executable;
- target, scope, and all generic impact dimensions (cardinality, notification, access change, destructive);
- choose exactly one target-binding mode for `RoleAct`: required opaque reference input(s), or one complete command-bound `tool_local` fixed target when the command path identifies a CLI-owned singleton;
- for reference-bound create, exactly one required argument/flag opaque `parent_input`, no `target_id_input`, and no other `target_inputs`;
- for reference-bound write, one required argument/flag opaque `target_id_input` whose reference kind equals `TargetKind`, plus an optional distinct opaque parent role whose input is required when present; `target_inputs` contains only those bound roles;
- for a fixed-target mutation, an explicit empty `target_inputs`, no input-role fields, and a `TargetKind` matching the fixed target kind; create uses it as scope and write as the existing target;
- validation performed before the external boundary;
- finite timeout, delivery, collection coverage, pagination, maximum attempts,
  and upstream idempotency behavior;
- which derived policy applies at `app/execution.Invoker`; do not make the template assume approval, confirmation, OS authentication, or dry-run;
- audit-safe fields and secret fields;
- allowed network destination.

Route all equivalent effects through one central enforcement boundary. A new
command must not create a second raw transport or bypass validation.

## 4. Update the command catalog

Add the command to the canonical catalog and derive dispatch and help from that
entry. Complete its `AgentContract`: stable capability ID, user outcome,
inputs with source, value kind, cardinality, required/default behavior,
allowed values, numeric bounds, dependencies, and conflicts; formats,
fields/types/descriptions,
delivery, collection coverage, non-auth prerequisites, optional secret-free authentication
requirement, stable faults with exact next commands, and mutation contract when
applicable. Nil collections mean unknown and are invalid; use explicit empty
collections for known none.

The shared dispatcher parses argument and flag inputs exactly once from this
contract before invoking a handler. Handlers receive `ParsedInputs`, not raw
argv, and preserve absent, explicitly supplied empty/zero/false, and defaulted
values through `Provided` and `Defaulted`. Environment, configuration, and
stdin inputs remain owned by their typed source resolvers; the argv parser must
not synthesize or require them. Keep positional declaration order identical to
usage order, place no required positional after an optional one, and keep a
repeatable positional last. Use `--flag=value` when a flag value itself begins
with a dash, and use the published `--` positional-only marker before a
dash-prefixed positional value. Verify exact human and scoped agent help expose
this grammar rather than requiring implementation knowledge. Repeat a
repeatable flag once per value; preserve occurrence order and duplicates, and
do not invent comma splitting. Boolean flags accept bare `--flag` for true or
the explicit `--flag=true` / `--flag=false` forms; a separated boolean token is
not a value.

Declare delivery independently from collection coverage. `complete` delivery
returns the entire task-selected result or no success; it does not by itself
claim exhaustive provider history. Use `not_applicable` for a scalar, single
object, or no output; `exhaustive` for the exact declared task scope and
observation; `bounded_window` for a fully delivered finite/latest/provider-capped
window; and `differential_window` for a change window since an explicit
checkpoint. Keep concrete limits, checkpoints, ordering, and uncertainty in the
task result.

For `complete` delivery, do not declare a pagination binding. For deliberately
`paged` delivery, declare `AgentContract.Pagination` with the exact optional
cursor argument/flag and exact top-level string cursor output field. The cursor
is emitted beside `schema_version` and the JSON envelope; `CommandOutput.Fields`
describe only values inside the envelope. Both cursor endpoints must use one
dedicated opaque reference kind, and no extra input or output may reuse that
cursor kind. A paged command supports only JSON and defaults to JSON. Pass the
emitted cursor back unchanged. Declare the only supported completion rule,
`completion: "empty_cursor"`, and emit the top-level cursor on every successful
page. A paged collection cannot declare `not_applicable`. An empty string is complete; omission, JSON `null`, and non-string values
are contract failures. Do not use `paged` to make silent truncation or a local
traversal limit look successful.

For a mutation, fill `MutationContract` from required argument or flag inputs rather than
maintaining an informal target description. Treat a missing or unbound role,
optional or non-CLI target, duplicate or extra target input, non-reference input, or target-kind mismatch as
a catalog error. Do not defer that ambiguity to command parsing, policy, or the
adapter.

Do not hand-maintain `ProducedRef` or `ConsumedRef`. The catalog derives
reference compatibility, the reference graph, grouped workflows, and those
producer/consumer projections from structured input/output reference kinds.
Fault recovery `next_actions` are not reference adjacency: declare them
explicitly on each stable fault and validate their executable commands.
An act command must either require at least one opaque reference or declare one complete `tool_local` fixed target, never both. A fixed-target act produces and consumes no references. Give semantically
different references different kinds; sharing a kind declares them
interchangeable across every matching field/input edge. Ensure required
reference chains lead back to a command that can run without an unresolved
required reference rather than forming a closed cycle.
Verify that root agent help adds only the command's path, namespace, summary,
capability, outcome, effect, and role. Then use an exact-command or
namespace-scoped invocation to verify the complete contract and workflows.
Root help must not regain inputs, output detail, authentication, errors,
mutation facts, or workflows as the catalog grows, and each encoded command
entry must remain within the 512-byte catalog budget.

Exercise the public help forms directly: `<binary> help --format agent` for the
root index, `<binary> help <namespace> --format agent` or
`<binary> help <exact-command> --format agent` for scoped machine contracts,
and `<binary> <exact-command> --help` for exact human help.

Treat `scope_request` invocation counts as help-discovery bounds only. An
unknown outcome needs the root index plus one selected scoped contract. A known
path needs one scoped-help invocation when the caller already holds every
required reference and other task input. These counts exclude authentication,
task execution, producer discovery, and any later scoped-help request needed to
retrieve the complete contract of a workflow endpoint outside the selected
scope.

Keep every recovery `command` executable under the template's small grammar:
use one exact catalog path, or `help` plus an exact path/canonical namespace.
Do not append flags, values, or guessed selectors. If a derived product needs a
fixed argument-bearing recovery, introduce a typed argument contract and
parser-aware validation before publishing it.

Add bidirectional contract tests so every public catalog entry has a dispatch,
help, and fixture, and no removed or internal entry remains exposed. For a JSON
result with one shape, compare the executable schema version, envelope, and
item keys with `CommandOutput`; do not accept a declaration-only or
renderer-only change.

The template help command is the one deliberate input-selected variant:
`CommandOutput.Fields` describes root `view: index` command entries, while a
selector returns an independent `view: scope` shape under the same agent-help
schema version. Exact-key contract tests cover both views and reject both
missing and extra keys. Do not add generic output-variant metadata for this
single command. If another command needs input-selected result shapes, revisit
the catalog abstraction before exposing it rather than multiplying exceptions.

For an output-contract change, audit the finite set of task-owned semantic
result variants rather than a sample of provider routes. Record which facts are
kept or omitted and why. Preserve canonical references, every applicable
request dimension, trust framing, recovery facts, interpretation-relevant
empty/zero/false values, and bounded uncertainty unless the public contract
deliberately changes them. Add a negative canary so a removed non-contract field
cannot silently return.

Keep command paths disjoint from their word-boundary namespaces. Match argv
`Required` flags to bracketed versus non-bracketed usage syntax, keep a written
`a|b` list exactly aligned with multiple `AllowedValues`, and use
`--flag=literal` for one exact allowed value; do not apply this grammar to stdin,
environment, or configuration inputs. Declare the common
cancellation/output failures and, for mutations, every standard invoker
contract or policy failure before exposing the command.

The standard non-retryable mutation set is
`invalid_mutation_contract` (`contract`), `missing_mutation_action`
(`contract`), `missing_mutation_policy` (`rejected`), `mutation_rejected`
(`rejected`), and `unclassified_mutation_outcome` (`contract`). A mutation with
output also declares non-retryable `mutation_output_write_failed` (`internal`);
every command declares the shared cancellation contract. Use catalog validation
as the executable authority for the exact set, kinds, and retryability.

Treat mutation cancellation by phase. Before the action, `operation_canceled`
is retryable because the invoker proves zero attempts. After the action begins,
return a valid structured adapter fault for a known classification and let the
invoker strip its private cause. Never return a raw cancellation as proof that
the write did not happen. Unclassified post-action errors become non-retryable
`unclassified_mutation_outcome`; declare that code in the mutation catalog and
point its next action only to an exact read/discover reconciliation command.
Render first, then pass every successful result to the effect-aware complete
write boundary. For create/write commands, declare non-retryable
`mutation_output_write_failed` with only a read-only reconciliation action;
late cancellation or a short stdout write must never advertise the confirmed
mutation as safe to repeat. A non-retryable mutation `rate_limited` recovery is
also read-only. Treat positive `retry_after` as timing evidence independent of
the `retryable` replay decision, and render an absent rate window as unknown.

## 5. Add authentication and API boundaries only when needed

Read `docs/07_authentication.md` and `docs/08_external_api_contracts.md` for an
external API capability.

- Keep raw PATs, OAuth tokens, refresh material, token sources, authorization
  headers, and credential-store handles inside `internal/infra`.
- Use `app/authn.Gate` with a secret-free requirement/session and prove auth,
  permission, mismatch, and cancellation failures make zero downstream calls.
- Treat non-nil `AgentContract.Authentication` as a binding to that gate. Declare
  every standard gate fault with its exact code, kind, and retryability, plus
  each provider-specific pass-through fault and a command-valid recovery action;
  catalog validation rejects an incomplete standard set before dispatch.
- Make each authenticated application task port accept `authn.BindingID`.
  Pass the ID from the validated `Session` unchanged; the infrastructure
  adapter resolves that process-local binding and revalidates or refreshes the
  exact private authentication record immediately before I/O. Never pass an
  OAuth client, token source, PAT wrapper, authenticated transport, provider
  SDK type, or credential-store handle into application code.
- Call `authn.NewBindingID` only from production infrastructure. Architecture
  lint rejects issuance in domain, application, CLI, and command packages, so
  argv or configuration values cannot be promoted into authentication bindings.
- Treat gate expiry as admission metadata rather than a lease. Keep refresh
  thresholds, reuse, cache/storage, account selection, and approval policy in
  the derived security model, while making stale or mismatched bindings fail
  before a provider task request.
- Do not implement OAuth protocol machinery. Add a reviewed OAuth library only
  in a derived project whose accepted security model selects OAuth.
- Keep schema-versioned non-secret user configuration separate from credential storage. Decode it strictly within a byte bound, fail closed on an invalid higher-priority environment or persistent value, and never probe another method after a selected method fails.
- For an injected persistent-configuration path, confine staging and replacement through one verified opened parent directory, use create-exclusive temporary files, revalidate parent/staged/target identities before and after replacement, and sync the directory where the platform provides that guarantee. State Windows ACL, atomicity, and durability limitations explicitly; an error after replacement begins leaves the active version uncertain.
- If OAuth launches a browser, separate URL presentation, platform auto-open, and callback receipt; retain a manual URL fallback and never place authorization codes, PKCE verifiers, tokens, PATs, or client secrets in subprocess argv.
- Make one-page adapters return an opaque cursor envelope. Use bounded
  complete traversal, or declare a paged public result with the catalog-bound
  optional cursor input and next-cursor output.
- Keep wire DTOs in infrastructure. Add publishable fixtures under `testdata`
  and bind their path, digest, provenance, and license in
  `.harness/schemas.json`.
- Classify each mutation payload as replacement, patch, or append. Preserve
  omitted separately from explicit empty/zero/false and provider `null`, and
  assert exact encoded bodies for every meaningful presence state.
- Keep access limitation separate from delivery/coverage and core record
  validity separate from optional enrichment. Optional enrichment may fail
  wholly unknown with a bounded reason; never publish a partially inferred
  relation from labels, order, indentation, or neighboring records.
- Map provider failures once into stable `fault.Error` values. Never expose a
  raw response body or cause.

## 6. Test at each boundary

Add the smallest set that proves the capability:

- domain tests for invariants and edge cases;
- application tests with fake ports for ordering and failure behavior;
- adapter contract tests for exact requests and bounded responses;
- CLI tests from argv through stdout, stderr, exit code, and captured effects;
- argv grammar tests for equals-form dash-prefixed opaque IDs, rejection of the
  ambiguous separated form, repeatable-value order and duplicates, explicit
  false versus omitted/defaulted booleans, unsupported boolean spellings, and
  positional-only dash-prefixed values;
- rejection tests proving invalid input causes zero external calls;
- catalog tests rejecting missing, extra, duplicate, optional, non-CLI, non-opaque, and reference-kind-mismatched mutation bindings;
- catalog tests rejecting optional act references and closed required-reference cycles;
- authentication/policy/cancellation tests proving zero downstream mutation;
- mutation outcome tests proving structured deadline/cancellation causes retain their typed classification, unstructured post-action errors are non-retryable, confirmed success is not overwritten, and reconciliation cannot point to a mutation;
- confirmed-mutation output tests proving late cancellation does not replace
  success, short writes emit non-retryable `mutation_output_write_failed`, and
  every recovery remains read-only;
- rate-limit tests covering known and unknown timing independently from
  retryability, including a non-retryable mutation with positive timing;
- authentication-binding tests with simultaneous accounts/authorities,
  missing, stale, wrong-account, and cross-session IDs, typed-nil task ports,
  expiry races, refresh identity mismatch/failure, and zero unintended provider
  task requests;
- pagination tests for empty, one, many, repeated-cursor, budget, and mid-page failure paths;
- catalog tests rejecting missing, extra, required, non-string, non-opaque, and
  reference-kind-mismatched public cursor bindings;
- hostile-output tests for ESC/newline, bidi and zero-width format characters, U+2028/U+2029, pre-existing backslashes, JSON-looking and prompt-like printable data, oversized content, and writer failure;
- tests proving structural escaping does not claim to filter semantic instructions and does not change an opaque reference;
- regression fixtures for stable TSV/JSON output and structured error output.
- for each interpretation-sensitive capability, task-result tests covering its
  declared task identity and every request dimension it actually carries,
  empty-collection scope when scoped, interpretation-relevant state
  distinctions, contextual reference kinds where semantic reference fields
  exist, and no partial success from an invalid adapter result;
- for capabilities whose output could invite display-only inference,
  negative-inference canaries proving names, prose, ordering, proximity,
  quotation, and indentation cannot create identity or relationships that are
  absent from typed semantic facts;
- for setup or authentication UX, a work-packet scorecard comparing environment exports, fixed-value re-entry, terminal/browser transfers, clipboard/OS dependencies, non-selecting discover/act trips, first-run and steady-state commands, and ceremonial inputs; retain steps justified by safety or certainty.

Tests must use temporary directories, fixed clocks, fake credentials, and local
test servers. They must not require a developer account or live network.

For a significant default text or agent-presentation change, use one frozen,
presentation-independent typed fixture and a machine-readable answer key. The
evidence packet records exact next argv, negative-inference canaries, before and
after goldens generated from the same fixture, fixture hashes, byte counts, and
any pinned tokenizer/version. Semantic correctness and canonical-reference
reuse are eligibility gates before size or token evidence. Keep invalidated,
failed, and inconclusive runs, and separate a benchmark result from the product
owner's compatibility decision. Do not put live-model evaluation in the
canonical completion gate.

## 7. Keep claims enforceable

Update the claim-to-enforcement table in `docs/04_harness.md` when a safety claim
changes. Update architecture and operating documentation when a boundary
changes. Do not rely on prose alone when a lint, type, test, or workflow can
enforce the rule.

Run:

```sh
./scripts/check.sh fast
./scripts/check.sh full
./scripts/check.sh security
```

`task check` is the single completion decision and must pass after the focused
profiles above. Publication work also requires `task public:check`; release
work requires `task release:check`.

Before public release, also confirm `./scripts/check.sh public` passes with the
stored identity-ready value `profile: ready` in `.harness/project.json`.

## 8. Validate the agent journey

Replay the relevant scenario from `docs/09_agent_readiness_validation.md`.
Record how many invocations were needed to discover the task, where each input
came from, whether output passed unchanged into the next task, and whether each
failure selected a next command without prose interpretation. Also record the
routine-success external-processing count; declared field extraction counts as
consumption, while a custom join/parser, provider-notation interpretation,
source inspection, or exploratory request counts as external reconstruction.
Extra command guesses or reconstruction are thesis/product evidence, not an
agent workaround to document.

## 9. Retire a capability deliberately

Removal is a product and security change, not dead-code cleanup. Update the
capability ledger and prove that removed commands, faults, recovery actions,
configuration selectors, dependencies, and dormant fallbacks are no longer
reachable. Decide explicitly whether persisted secret and non-secret state is
ignored, migrated, or removed by a dedicated cleanup action; unrelated commands
must not silently delete legacy state. Preserve the evidence that justified the
retirement and mark superseded work packets or ADRs with their successor.

## 10. Feed implementation learning back into the thesis

Implementation is an iterative design probe. When code or tests reveal a new
constraint, do not leave the decision only in a local comment. Revisit the
thesis, refine it when the lesson is general, then propagate that decision into
architecture documentation, the command catalog, typed contracts, tests,
linters, and this skill. The repository should become less ambiguous after
each capability is added.
