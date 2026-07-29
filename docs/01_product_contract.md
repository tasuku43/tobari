# Product Contract

This document defines what Agentic CLI Foundry promises as a product and what a derived project must replace with its own contract. It sits between the theses and implementation: broad enough to survive refactoring, specific enough to decide whether a proposed command belongs.

## Product statement

Agentic CLI Foundry is a runnable Go repository for starting a small, task-oriented, public command-line tool that humans and coding agents can extend without inventing architecture, side-effect enforcement, or release hygiene from scratch.

It is not a framework intended to support every CLI shape. It demonstrates one coherent path and makes optional integrations earn their place through a derived project's thesis.

## Primary users

- A project owner defining the first useful task for a new CLI.
- A contributor implementing or reviewing a capability.
- A coding agent that must discover constraints from repository-local evidence.
- A release owner preparing source and artifacts for a public audience.

The end users of a derived CLI are not known by this template. Naming them is the derived project's first product responsibility.

## Default supported outcomes

The runnable template supports only the outcomes needed to demonstrate its shape:

- Discover the available command surface with `agentic-cli-foundry --help`.
- Retrieve a compact versioned outcome index with `agentic-cli-foundry help --format agent`, then request one exact command or namespace for its complete machine contract.
- Inspect the local runtime with `agentic-cli-foundry doctor`.
- Discover synthetic objects with `agentic-cli-foundry sample list`.
- Read one synthetic object with `agentic-cli-foundry sample read --id <sample-id>` using the emitted ID unchanged.
- Inspect build identity with `agentic-cli-foundry version` or `agentic-cli-foundry --version`.
- Bootstrap a validated derived identity from `.harness/project.json`.
- Verify source, security, release, and public-readiness contracts through named check profiles.

The default `doctor` task is intentionally small. It proves the path from catalog and CLI input through an application use case to a concrete infrastructure adapter, then back to stable presentation.

The default public output and exit contract is:

| Surface | Contract |
|---|---|
| `doctor` | Complete delivery with exhaustive coverage of the declared checks: TSV headed `CHECK<TAB>STATUS<TAB>DETAIL`, or JSON schema version 1 under `report`; status is `pass`, `warn`, or `fail` |
| sample list | Complete delivery with exhaustive coverage of the synthetic repository: TSV headed `id<TAB>name`, or JSON schema version 1 under `items`; every emitted ID is an unchanged reusable reference |
| sample read | Complete delivery with collection coverage `not_applicable`: TSV headed `id<TAB>name<TAB>content`, or JSON schema version 1 under `item` |
| agent help | JSON schema version 6 with complete delivery and exhaustive coverage of the selected catalog scope: root `view: index` returns path/namespace/summary/capability/outcome/effect/role entries plus a machine-readable scope request; selected `view: scope` returns global I/O/error rules, executable typed input contracts, complete command contracts (including an optional fixed target), and reference-kind workflows grouped into unique producer and consumer sets |
| structured failure | JSON schema version 1 on stderr under `error`, selected by placing `--error-format json` before the command; text is the default |
| version | Complete delivery with collection coverage `not_applicable`: `agentic-cli-foundry <version> (<commit>)` when commit metadata is available |
| exit `0` | Successful command |
| exit `2` | Invalid command, option, or task input |
| exit `3` | Unexpected internal failure |
| exit `4` / `5` | Authentication required / authenticated but not permitted |
| exit `6` / `7` | Target not found / target selection ambiguous |
| exit `8` / `9` | Rate limited / temporarily unavailable |
| exit `10` / `11` | Policy or diagnostic rejection / caller cancellation |
| exit `12` / `13` | Unsupported task / violated declared contract |

Successful results are written to stdout; failures are written to stderr. A zero exit status requires a complete successful write, and a partial result is never reported as success. A failed diagnostic may emit its complete report before returning its structured nonzero failure so the caller receives the evidence needed to recover.

Read results recheck cancellation immediately before their complete write. A
mutation result returned after the controlled action has confirmed success uses
the effect-aware complete-write path: later cancellation cannot reclassify that
success as a retryable failure. A writer failure still prevents exit 0, but is
the non-retryable `mutation_output_write_failed` fault with a read-only
reconciliation action rather than permission to repeat the mutation.

Delivery and collection coverage are independent public facts. Complete
delivery means the task-selected result was fully written or no success was
reported. It does not imply all provider history. Collection coverage is
`not_applicable`, `exhaustive` over the exact declared task scope and
observation, `bounded_window`, or `differential_window`. Paged delivery uses the
separate public cursor contract and states the coverage obtained when that
traversal completes.

## Public CLI vocabulary

`cli.Catalog` is the source of truth for public commands. Each `cli.CommandSpec` represents one user task and owns at least:

- a stable command path;
- a concise task summary;
- an explicit `operation.Effect`;
- a `CommandRole` of utility, discover, or act;
- structured inputs and output fields from which opaque-reference edges are derived;
- each input's executable value kind, single or repeatable cardinality,
  required/optional state, omission default, applicable numeric bounds, and
  explicit dependency/conflict relations;
- a stable capability ID, output formats/types, delivery, collection coverage,
  prerequisites, declared failures, and exact recovery commands;
- a default output format and, when JSON is supported, a stable envelope and positive schema version;
- argument and validation behavior;
- a handler or use-case binding;
- enough metadata to generate accurate help and contract tests.

No command path may also be another command's word-boundary namespace prefix: `foo` and `foo bar` cannot coexist because exact selection would hide the namespace children. Within the template's intentionally small usage grammar, brackets define optional argv inputs, non-bracketed inputs are required, a written `a|b` enumeration must match several `AllowedValues` exactly and in order, and `--flag=literal` binds one exact allowed value. Stdin, environment, and configuration inputs remain outside argv syntax matching.

Every command declares the common runtime failures that its shared execution path can emit. `operation_canceled` is always present with its stable kind/retryability. Read commands with output declare retryable `output_write_failed`; create/write commands with output instead declare non-retryable `mutation_output_write_failed` with read-only reconciliation. A non-nil authentication requirement binds a command to the template `app/authn.Gate`, so the catalog additionally requires every standard gate fault with its exact kind and retryability; provider-specific faults remain explicit additions. Mutations similarly publish the standard invoker's contract and policy-rejection failures, including non-retryable `unclassified_mutation_outcome` with a read-only reconciliation action, so runtime normalization does not turn a predictable failure into `undeclared_fault_contract` or an unsafe retry.

`next_actions[].command` uses a deliberately small executable grammar: an exact catalog command path, or `help` followed by one exact path or canonical namespace. Prefix-only matches, unknown help selectors, non-canonical whitespace, and unchecked argv suffixes fail catalog validation. A derived project that needs fixed arguments in recovery must first add a typed argument contract and parser-aware validation; it must not append plausible-looking prose to the command string.

The catalog-owned parser enforces command-line input declarations and preserves
three states independently: absent with no value, omitted with a declared
default, and explicitly supplied (including empty, zero, or false). Handlers
perform only task-specific conversion and domain validation; they do not
redeclare flag type, repetition, range, default, or input relations. Value flags
accept `--flag value` or `--flag=value`, but a dash-prefixed value requires the
unambiguous equals form. Boolean flags accept the bare true form and explicit
`=true` or `=false`. The `--` marker begins positional-only input so an opaque
dash-prefixed positional value can pass unchanged.

Human help and machine help are distinct catalog projections. Human root help
lists direct commands and each namespace once, namespace help lists relative
leaves, and exact help prints the full executable input contract plus that argv
grammar. Scoped agent help publishes the same grammar structurally. Natural
`<namespace> --help` and `<exact-command> --help` forms resolve to the same
canonical help selector. The machine root remains an exact-path outcome index.

The agent-help, success-output, and error-output schemas are versioned
independently from prose help. A derived project must increment or deliberately
evolve the affected schema when changing its machine-readable shape. For a JSON
command result with one shape, the catalog declaration and executable JSON must
agree on `schema_version`, envelope, and item fields; contract tests compare
them in both directions.

The help command has one deliberate input-selected variant rather than a
generic catalog output-variant registry. Its `CommandOutput.Fields` declares
the `commands[]` entries in root `view: index`. Supplying the optional selector
returns the independent `view: scope` shape under the same agent-help schema
version. Dedicated exact-key contract tests cover both root and scoped top-level
objects and their command entries, rejecting missing and extra keys. A second
command that needs input-selected result shapes is evidence to add catalog-owned
variant metadata before exposing it, not to add another undocumented exception.

In scoped agent-help schema 6, one workflow record names a reference kind and
lists its unique producers and consumers. Sharing a kind already declares every
producer value interchangeable at every listed consumer, so the document does
not repeat their Cartesian product or duplicate it as command-local next
actions. Structured error recovery remains in each declared fault's
`next_actions`.

The `scope_request` invocation counts bound only help discovery and retrieval of
one selected task contract. An unknown outcome uses the root index plus one
scoped-help request. A known path uses one scoped-help request when the caller
already holds every required reference and other task input. The counts do not
include authentication, task execution, producer discovery, or a later
scoped-help request for the complete contract of a workflow endpoint outside
the selected scope.

Command names describe outcomes. Package names, SDK methods, URL paths, database tables, and protocol verbs are not automatically public vocabulary.

## Compatibility boundary

Before version `1.0.0`, the template may refine its example surface, but every change must be intentional and tested. A derived project must decide when its own compatibility promise begins.

Once declared stable, the following are public contracts unless explicitly documented otherwise:

- command paths and required arguments;
- command roles and produced or consumed reference declarations;
- effect classification;
- machine-readable output fields and types;
- exit-status meanings;
- default side effects;
- configuration and environment variable names;
- filesystem locations and persisted formats;
- release artifact names and supported platforms.

Internal package layout is not a public Go library API. The `internal/` boundary is deliberate.

## Product rules

### Prefer outcomes over coverage

Do not add a command merely because an external system exposes an operation. Record unsupported or deferred capabilities explicitly when a derived project maintains an upstream coverage ledger.

### Separate discovery from action

Each command is a `utility`, `discover`, or `act` task. Discovery owns filters and ambiguity. An action uses exactly one target-binding mode: a required opaque reference for an external or selected target, or a catalog-declared `tool_local` fixed target when the command path uniquely identifies one CLI-owned singleton. Fixed targets publish stable kind, ID, scope, and description, produce and consume no references, and cannot be combined with a reference. Do not hide a search or candidate choice inside an action or label an externally selected object fixed.

The ID shown by discovery passes unchanged into action. Do not decode, normalize, reconstruct, or substitute a resource URL merely because an external system exposes those forms. Display labels are for people; opaque references are for stable composition.

The default `sample list` and `sample read --id` pair exists to make this flow executable. A derived project replaces its synthetic sample domain with a real task while preserving or deliberately revising the role and reference contracts.

A fixed target is appropriate for state such as this installation's one selected authentication configuration when the command path itself supplies all target certainty. It removes no mutation facts: effect, intent, runtime target, impact, policy, failures, and reconciliation remain required.

The sample reference kind is `sample`. `sample list` produces field `id`; `sample read` consumes argument `--id`. A sample ID is `smp_` followed by exactly twelve lowercase hexadecimal characters. The CLI validates that shape without changing the bytes and rejects names, partial IDs, uppercase variants, URLs, resource paths, whitespace, and control characters before the adapter runs.

### Compose deterministic workflows

If a common result requires a deterministic series of adapter calls, implement one application use case. Do not make every user or agent rediscover the sequence.

### Close supported outcomes operationally

A command that claims a supported user outcome owns the deterministic
selection, joining, and interpretation required for routine success. Reading a
declared JSON or TSV field is normal consumption. Requiring an undeclared
`jq`/`grep` pipeline, custom parser, provider notation decoder, source
inspection, or exploratory API call means the outcome is not yet closed.

This rule does not prohibit a derived product from exposing a deliberately raw
export or low-level utility. Such a surface must state its narrower outcome and
must not stand in for a task that the product claims to complete.

Application and domain results carry declared task identity and every request
dimension the task actually has. They also retain reference kinds, explicit
empty/zero/false state, and bounded uncertainty whenever those distinctions
affect interpretation before the CLI renders them. A scoped collection retains
scope even when empty. Presentation may omit only facts that its reviewed
output contract declares redundant; it must not recover scope from the first
item, strengthen an unresolved fact, or infer relationships from names,
ordering, proximity, quoting, or indentation.

Provider collection coverage and provider visibility are different facts. A
task may fully deliver its declared bounded window while the provider reports
that some objects inside that window are inaccessible or hidden. That
limitation is a task-owned typed state, not an empty collection or a weaker
coverage guess. Likewise, if a core record is valid but optional semantic
enrichment cannot be established, the core may remain successful while the
entire affected enrichment relation is typed unknown; mandatory identity,
encoding, and bound failures still fail the result.

### Keep locale policy at the trust boundary

`public_guard.documentation_locale` selects the one default language for
trusted repository and CLI-authored prose. The template uses English. Changing
it is a product/thesis decision, not a translation-only edit. Command paths,
flags, environment variables, fault codes, JSON keys, schema values, and
reference kinds remain stable language-neutral identifiers. Provider and file
text remains untranslated untrusted data; translation does not sanitize it.
Historical evidence is not rewritten merely to make its prose match a new
default locale.

### Bound raw flexibility

Arbitrary routes, opaque parameter maps, unrestricted scripts, and pass-through request bodies expand both the product and security surface. They are excluded unless a project's thesis explicitly makes raw transport the product.

### Keep provider policy in the derived product

The base template fixes fail-closed authentication, bounded non-secret user-configuration, external-call, pagination, failure, output, and mutation enforcement boundaries. It does not select a provider, concrete configuration fields, OAuth grant or library version, PAT source, credential store, account and refresh policy, browser/callback policy, retry/backoff values, or approval experience. Those choices depend on the real user outcome and trust boundary, so the derived product contract and security model must make them concrete before live I/O is enabled. See [Authentication](07_authentication.md) and [External API Contracts](08_external_api_contracts.md).

## Explicit non-goals of the base template

- Choosing a CLI parsing framework for every project.
- Choosing a concrete OAuth flow, PAT source, credential store, telemetry system, updater, or vendor API by default.
- Exposing this repository as a reusable Go library.
- Supporting every operating system or package manager without a release decision.
- Treating passing tests as a substitute for a product review.
- Turning private source into public source through automated string replacement alone.

## Derived-project completion checklist

Before expanding implementation, replace this document's generic content with answers to:

1. Who is the primary user?
2. What high-value outcome does the CLI own?
3. What is the canonical public vocabulary?
4. What is deliberately unsupported?
5. Which commands discover identifiers, and which commands act on them?
6. Which reference kinds, producer fields, and consumer arguments connect them?
7. Which output, exit, configuration, and side-effect behavior is stable?
8. Which upstream capabilities remain internal or deferred?
9. What compatibility and deprecation policy applies?
10. Which authentication method, account model, credential source/storage, refresh/reuse behavior, and recovery workflow apply?
11. Which timeout, retry/idempotency, delivery, collection coverage, pagination,
    schema-drift, and mutation-approval policies bound external I/O?
12. What is the routine-success external-processing count, and which declared
    fields provide the answer and next canonical reference without custom
    reconstruction?
13. How are result task, request scope, reference kinds, absent/empty/zero/false
    states, and uncertainty validated before presentation?
14. If a capability is removed, what happens to its catalog entries,
    dependencies, dormant fallbacks, credentials, and persisted non-secret
    state?

Update catalog contract tests when the resulting public surface changes.
