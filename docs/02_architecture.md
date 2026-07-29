# Architecture

Agentic CLI Foundry uses four layers, a task-oriented command catalog, typed operation intent, and one composition root. The purpose is to keep product decisions separate from external-system details while giving side effects a narrow, testable path.

## Dependency direction

```text
internal/cli  ------> internal/app
      |                    |
      |                    v
      +------------> internal/domain <------ internal/infra

internal/domain does not depend on app, infra, or cli.
internal/app does not depend on infra or cli.
internal/infra does not depend on app or cli.
```

`tools/archlint` enforces this direction for production code. Tests may use dedicated helpers, but they must not create a production bypass.

## Layer responsibilities

### Domain

`internal/domain/` contains pure project vocabulary, values, validation, and invariants. It performs no network, filesystem, process, terminal, clock, or credential I/O.

The base template includes:

- `operation.Effect` with `EffectUnknown`, `EffectRead`, `EffectCreate`, and `EffectWrite`;
- `operation.TargetRef`, a stable description of the object or scope affected;
- `operation.Impact`, explicit cardinality, notification, access-change, and destructive declarations;
- `operation.Intent`, the declared effect, target, and impact presented to the execution boundary;
- `fault.Error`, stable failure kind, code, retryability, and next-action metadata;
- `page` and `apicall` envelopes for opaque cursors and finite call policy;
- secret-free authentication requirement and session metadata, including a private, non-serialized ephemeral binding passed to authenticated task ports;
- `doctor` domain values used by the default task.

Unknown effects and incomplete mutation intent are invalid domain states at an external-execution boundary.

### Application

`internal/app/` contains user-task use cases. A use case:

- interprets task-specific input;
- chooses the order of operations;
- handles deterministic composition and ambiguity rules;
- defines the smallest ports it needs;
- returns task-specific results rather than infrastructure response types;
- validates that an adapter result belongs to the declared request task and
  each target, parent, or scope dimension that task actually carries before
  exposing it as success.

The application layer depends on domain values and primitive types. It does not import infrastructure, parse CLI arguments, render terminal output, or construct transport requests.

`internal/app/doctorcmd` is the default utility example. `internal/app/samplecmd` owns the synthetic discover/list and act/read use cases. Reusable application boundaries include authentication gating, complete-or-no-result pagination, and policy-neutral mutation invocation.

### Infrastructure

`internal/infra/` implements application or domain-facing contracts for external systems. Examples in a derived project may include HTTP adapters, filesystems, credential stores, clocks, subprocesses, or platform services.

Infrastructure owns protocol-specific validation and conversion. Raw OAuth tokens, refresh tokens, PATs, token sources, and authorization headers never leave this layer. Infrastructure does not decide which public command should exist, how several adapters form a user task, or how terminal output is presented.

`internal/infra/systemdoctor` is the default diagnostic adapter. `internal/infra/sampledata` is a deterministic offline repository used to prove opaque reference flow without network access.

### CLI

`internal/cli/` owns:

- `CommandSpec` and `Catalog`;
- `CommandRole` and the structured `AgentContract` for capability, inputs,
  outputs, delivery, collection coverage, paged cursor binding, prerequisites,
  failures, authentication, and mutation facts;
- argument parsing and task-level validation;
- help and public discovery;
- output and error presentation;
- the composition root that wires use cases to concrete adapters;
- the controlled handoff to side-effect execution.

The CLI represents semantic results; it does not create them. Declared task
identity, applicable request dimensions, reference kind, interpretation-
relevant absence versus explicit empty/zero/false values, ordering guarantees,
and uncertainty are domain or application facts. A scoped collection retains
scope when empty. A renderer must not derive these facts from the first
collection member, a display name, physical order, proximity, quotation, or
indentation. A relationship-rich capability keeps a presentation-independent
typed fixture and answer key so the same facts can validate every renderer.

`CommandInput` is executable invocation metadata rather than help-only prose.
It declares text/integer/boolean value kind, single/repeatable cardinality,
required state, an optional omission default, applicable integer bounds, and
explicit `requires`/`conflicts_with` relations. One catalog-owned parser applies
those facts to argv and retains explicit/defaulted/absent state; task handlers
then perform domain-specific validation such as opaque-reference shape.

`cmd/agentic-cli-foundry/main.go` is a thin executable entry point. It should not contain product logic or construct adapters independently of the CLI composition root.

Production Go packages stay within `cmd/` and the four `internal/` layers. The `cmd/` entrypoint imports only context, operating-system signal handling, and `internal/cli`; process execution, network, filesystem, and third-party dependencies belong behind infrastructure ports. Repository-only programs live under `tools/` and cannot be imported by production packages.

CLI, application, and infrastructure code propagate the caller context instead of creating `context.Background()` or `context.TODO()`. The command entrypoint creates the signal-aware root and calls the context-only CLI boundary; a nil context produces a context-independent contract fault before dispatch. Infrastructure network code uses an explicitly constructed client governed by a finite call policy; package-level default HTTP clients and convenience calls are rejected by architecture lint.

## Catalog as the public source of truth

`cli.Catalog` contains every public `cli.CommandSpec`. Routing, root help, command help, uniqueness checks, and catalog-wide effect tests derive from it.

Do not add a second command list for documentation or dispatch. Internal adapters and generated operations are not public merely because they exist. A capability becomes public only when a user-task use case and command specification deliberately expose it.

At minimum, catalog validation rejects:

- duplicate or empty command paths, and a command path that is also another command's word-boundary namespace prefix;
- missing summaries or handlers;
- `EffectUnknown`;
- `RoleUnknown`;
- malformed, duplicate, role-inconsistent, orphaned, or closed-cycle reference declarations;
- missing capability, input descriptions/allowed values, output field meanings,
  delivery/collection coverage, prerequisites, or recovery commands;
- argv input metadata whose `Required` value or ordered `AllowedValues` disagree
  with the small bracket, `a|b`, and exact `--flag=literal` usage grammar;
  non-argv sources are excluded from this syntax check;
- missing or invalid input value kind/cardinality, invalid defaults or numeric
  bounds, unenforceable dependency/conflict relations, duplicate scalar input,
  and a positional input after a repeatable positional input;
- missing or inconsistent common runtime failures (`operation_canceled`, read-output `output_write_failed`, mutation-output `mutation_output_write_failed`, standard authentication-gate failures, and standard mutation-invoker contract/policy/unknown-outcome failures), or an uncertain/confirmed-mutation-output recovery that points to another mutation;
- recovery commands that are only catalog prefixes, contain unchecked argv, use an unknown help selector, or otherwise fall outside the exact-path/`help <path-or-namespace>` grammar;
- read commands with mutation metadata; incomplete reference-bound create/write roles; incomplete, mixed, or mismatched fixed-target mutation binding; unbound or extra target inputs; and mutations without complete impact;
- a root agent-index entry larger than 512 encoded bytes, which would let selection prose crowd detailed scoped contracts as the catalog grows;
- command metadata that cannot produce consistent help and routing.

## Command roles and opaque reference flow

Actions use exactly one target-binding mode. `AgentContract.FixedTarget` is the command-bound declaration: stable kind, ID, description, and the only supported scope `tool_local`. Only `RoleAct` may declare it, and a fixed-target command has no produced or consumed reference edge. External, ambiguous, or caller-selected targets remain reference-bound. Scoped agent-help schema version 6 publishes the optional fixed target and the executable typed input contract while the compact root index shape remains unchanged.

`CommandRole` describes where a task sits in a user workflow:

| Role | Responsibility |
|---|---|
| `RoleUtility` | Repository or runtime operation that is neither candidate discovery nor unique-target action |
| `RoleDiscover` | Owns ambiguity and may emit opaque references with candidates |
| `RoleAct` | Operates on exactly one target-binding mode: required opaque reference(s), or one command-bound `tool_local` fixed singleton; never chooses among candidates |
| `RoleUnknown` | Invalid for a public command |

Reference kinds live on `AgentContract.Output.Fields`, `AgentContract.Inputs`, and the top-level cursor field owned by `AgentContract.Pagination`. `ProducedRef` and `ConsumedRef` are compatibility projections, not a second declaration. The catalog derives the reference graph, grouped scoped workflows, and those producer/consumer projections from the structured fields. Fault recovery `next_actions` remain explicit declarations on `CommandError`; they do not derive from reference adjacency. A shared kind is an explicit claim that every value of that kind is interchangeable at each matching input; use distinct kinds when fields or target roles are not interchangeable. Every kind needs a producer and consumer, and the required-reference dependency graph must be reachable from at least one command that can run without an unresolved required reference. Optional first-page cursors do not create a dependency. `CommandOutput.Fields` always describe values inside the declared JSON envelope; public `paged` delivery owns its separate cursor name, string type, description, and shared opaque kind in `Pagination`. Its typed `completion: "empty_cursor"` rule makes an always-present empty string the sole completion marker. Human help may stay concise; scoped agent help exposes the exact graph, pagination binding, and field meanings.

Value-shape validation alone is insufficient when the same opaque encoding can
name several resource classes. The task result validates each reference against
the kind required by its semantic field. Supplying a structurally valid value
of another kind is reference-kind laundering and fails before presentation or
action composition.

Agent-help schema version 6 separates selection from invocation detail. Root `help --format agent` is an `index` view containing only each command's path, top-level namespace, summary, capability ID, outcome, effect, and role. Each encoded command entry has a 512-byte catalog budget. Its `scope_request` names `commands[].path` and `commands[].namespace` as selectors and supplies the exact invocation template. `help <selector> --format agent` is a `scope` view containing the executable value-flag, boolean, equals-only dash-value, and positional-only grammar; global I/O/error contracts; complete selected `AgentContract` values (including executable typed inputs and an optional fixed target); and reference workflows touching the selection. Each workflow groups one `reference_kind` with unique `producers[]` and `consumers[]`; the shared-kind interchangeability contract supplies the implicit producer-to-consumer edges without encoding their Cartesian product. A scoped selection returns the complete group whenever any member touches the selection, so exact next usage, field, and input facts are not lost. Redundant command-local reference next actions are omitted, while fault recovery remains in `contract.errors[].next_actions`. The I/O contract marks external text as untrusted data, declares visible structural projection, and distinguishes validated exact opaque references.

The `scope_request` counters bound only help discovery and retrieval of one
selected command or namespace contract. An unknown outcome needs the root index
plus one scoped-help request. A known path needs one scoped-help request only
when the caller already holds every required reference and other task input.
Authentication, task execution, producer discovery, and later retrieval of the
complete contract for a workflow endpoint outside the selected scope are not
part of those counts. Root size still grows with the number of outcomes, but
detailed inputs, outputs, authentication, failures, mutations, and workflows do
not multiply there.

Catalog `CommandOutput` metadata is executable compatibility data, not descriptive decoration. `Delivery` declares complete-or-no-success invocation delivery versus the public paged cursor protocol. `CollectionCoverage` independently declares `not_applicable`, exhaustive coverage of the exact task scope and observation, a bounded window, or a differential window. Complete delivery never implies all provider history, and concrete limits/checkpoints remain task-owned semantic facts. Generic CLI contract tests compare each single-shape JSON renderer's `schema_version`, declared envelope, and every item key with the corresponding catalog declaration.

Agent help is the one deliberate input-selected variant:
`CommandOutput.Fields` describes root `view: index` command entries, while an
optional selector produces an independent `view: scope` shape under the same
schema version. Dedicated shape contract tests compare the exact top-level and
command-entry keys for both views, rejecting missing and extra keys. The
template does not add generic output-variant metadata for this single case; a
second command needing variants must trigger a catalog abstraction rather than
another exception.

The default graph is:

```text
sample list
  RoleDiscover
  produces {kind: sample, field: id}
       |
       | exact opaque value
       v
sample read --id <sample-id>
  RoleAct
  consumes {kind: sample, argument: --id}
```

`sample list` renders lowercase `id<TAB>name`; `sample read` renders `id<TAB>name<TAB>content`. The ID validator accepts only `smp_` plus twelve lowercase hexadecimal characters. It validates shape but never transforms the value.

## Operation effect and intent

Effect answers **what class of action occurs**:

| Effect | Meaning |
|---|---|
| `Read` | Observes state without intentionally changing it |
| `Create` | Creates a new object within a declared parent or scope |
| `Write` | Changes or removes an existing declared target |

Intent answers **what this invocation is allowed to affect**. A mutation intent includes a `TargetRef` and an `Impact`. Cardinality and the notification, access-change, and destructive dimensions must all be explicit; their zero values fail closed. Derived projects extend this base with domain-owned detail such as recipients, visibility changes, publication destinations, or workflow triggers.

`MutationContract` connects catalog target binding to runtime intent. The reference-bound branch is unchanged: for `Create`, `parent_input` names the single opaque parent or scope reference; for `Write`, `target_id_input` names an opaque reference whose kind equals `TargetKind` and an optional distinct `parent_input` can bind scope. The fixed-target branch requires an explicitly non-nil empty `target_inputs`, no input-role fields, and a `TargetKind` equal to `FixedTarget.Kind`; create treats the singleton as creation scope and write as the existing target. Missing, extra, duplicate, non-reference, mixed-mode, or mismatched binding fails before policy or I/O.

The catalog binding and runtime `TargetRef` are complementary checks: the former proves that an agent can supply an unambiguous target from the command contract, while the latter proves that the concrete invocation presented to policy and infrastructure has the declared target and impact. Neither replaces the other.

Do not infer effect from an HTTP method, command name, or adapter function. A `POST` may be a read-like query; a local file write is still a side effect without HTTP. The product declaration is authoritative and must agree with the concrete operation at runtime.

## Execution flow

```text
argv
  -> CLI selects one CommandSpec from Catalog
  -> CLI validates role/reference/input declarations and parses argv through the catalog-owned parser
  -> handler receives validated values with explicit/defaulted/absent state and chooses presentation
  -> application use case interprets the user outcome
  -> domain validates Effect, Intent, TargetRef, Impact, auth, and API envelopes
  -> controlled execution boundary snapshots intent and applies derived policy
  -> infrastructure adapter performs one bounded logical operation
  -> application validates the result against the declared task and its applicable request dimensions
  -> CLI renders stable output and exit behavior
```

For mutations, validation failure must occur before the external side effect. `app/execution.Invoker` provides the common ordering and has no permissive default policy. Dry-run, human approval, OS authentication, authorization reuse, confirmation, and audit behavior remain derived policy rather than command-local conventions.

## Error ownership

- Domain errors explain invalid values or invariants.
- Application errors explain task failure or ambiguity.
- Infrastructure errors map unstable upstream details into a stable `fault.Error` without leaking secrets.
- CLI maps fault kind, code, retryability, retry-after, and next actions to stable human and machine presentation and exit statuses.

`retryable` answers whether repeating the same logical command is permitted.
`retry_after` is independent provider evidence about a wait window. A
rate-limited mutation may therefore be non-retryable while still publishing a
positive wait duration for reconciliation or a different operation; the timing
must never be interpreted as replay authorization. Text output renders an
unknown rate-limit window as `unknown`, while JSON retains the stable nullable
field.

The stable exit mapping is `0` success; `2` invalid input; `3` internal; `4` authentication; `5` permission; `6` not found; `7` ambiguous; `8` rate limited; `9` unavailable; `10` rejected; `11` canceled; `12` unsupported; and `13` contract violation. Success output is written to stdout, while text or schema-versioned JSON failures are written to stderr. A zero status requires a complete successful write. One effect-aware finalizer checks cancellation immediately before a read write, but preserves confirmed create/write success across later cancellation. A short or failed mutation-result write returns non-retryable `mutation_output_write_failed` with read-only reconciliation rather than suggesting the mutation itself is safe to repeat. A failing `doctor` report may be rendered in full before its structured `rejected` failure so callers receive evidence without treating an incomplete result as success.

Do not render inside domain, application, or infrastructure packages. Do not make a use case parse human-facing error strings from an adapter.

## Adding a vertical slice

Add capabilities in this order:

1. User outcome and product-contract decision.
2. Utility, discover, or act role and its exclusive reference-bound or command-bound target flow.
3. Domain vocabulary, effect, intent, reference validation, and invariants.
4. Application input, result, use case, and owned ports.
5. Infrastructure adapter satisfying those ports.
6. CLI arguments, rendering, and one `CommandSpec`.
7. Unit tests at each layer and catalog/reference/execution contract tests.
8. Harness and documentation updates for any new invariant.

This order makes product intent reviewable before transport code creates momentum toward the wrong public API.

## Optional capabilities

Concrete schema generators, update checks, credential stores, OAuth libraries, telemetry, package-manager formulas, code signing, and platform-specific authorization are optional modules, not base-layer responsibilities. A derived project adds one only after documenting:

- the user outcome it enables;
- its trust and compatibility boundaries;
- dependency and maintenance cost;
- failure behavior;
- mechanical checks and release impact.
