# Project Theses

This is the first document to consult when a design choice is ambiguous. It states why Agentic CLI Foundry exists and the principles from which its product, architecture, security, and harness decisions follow.

A derived project must make these theses concrete. Renaming `agentic-cli-foundry` is not enough. Replace the generic users, outcomes, measures, examples, non-goals, and enforcement references with facts about the new tool. Preserve a template thesis only when it is genuinely true for that product.

Each thesis follows one causal chain:

```text
North Star or thesis
  -> consequences for product and engineering choices
  -> mechanical enforcement that detects regressions
```

If a statement has no observable consequence, it is not yet useful. If an important consequence has no enforcement, it remains an aspiration and must be labeled as such.

## Thesis lifecycle

The first theses are seeds: the smallest hypotheses needed to choose and review a minimal end-to-end slice. They should be decisive, but they are not assumed to be complete.

Use this continuous loop:

```text
seed a north star and minimal theses
  -> build the smallest vertical slice that can challenge them
  -> record repeated decisions, user outcomes, agent confusion, and friction as evidence
  -> revise the thesis before adding a code workaround or exception
  -> propagate consequences into product, architecture, security, Skills, catalog, and harness
  -> repeat with the next slice
```

Early in a project, thesis revisions should be frequent because every real slice reveals missing vocabulary and false assumptions. As the project matures, revisions should become less frequent, not forbidden. User behavior, incidents, compatibility pressure, and maintenance evidence remain valid reasons to change them.

Record evidence in the active work packet. A thesis revision is complete only when:

- the new statement explains the evidence;
- consequences and non-goals are explicit;
- affected durable documents and Skills agree;
- mechanical enforcement detects the old failure or workaround;
- compatibility and migration impact are reviewed.

Do not keep a thesis unchanged merely because code already exists. Do not change a mature thesis merely because one implementation would be easier without it.

## North Star

**A contributor or coding agent can turn a well-defined CLI idea into a small, safe, public-ready vertical slice without guessing the product vocabulary, architectural boundaries, side-effect policy, or completion gate.**

The template's success is measured by whether a new maintainer can answer, from repository evidence:

- Who is the tool for, and what outcome does it own?
- Which public commands exist, and how are they discovered?
- Where may domain, application, infrastructure, and CLI code depend?
- What can each operation affect, and where is that checked?
- Which command proves a change is complete?
- What must be reviewed before source or artifacts become public?

The template does not measure success by the number of included frameworks, commands, or integrations. A small coherent vertical slice is more valuable than a broad collection of optional mechanisms.

### Consequences

- The repository is runnable before customization.
- Documentation is part of the scaffold, not an afterthought.
- The default capability crosses every architectural layer and has contract tests.
- Product-specific integrations are omitted until a derived project can state their purpose and trust boundary.
- One catalog and one gate minimize competing sources of truth.

### Mechanical enforcement

- `go run ./cmd/agentic-cli-foundry --help`, `doctor`, `sample list`, and `sample read --id` exercise the default utility and discover/act slices.
- `cli.Catalog` contract tests keep public discovery and routing aligned.
- `tools/archlint` checks layer boundaries.
- `./scripts/check.sh full` is the canonical completion path.
- `./scripts/check.sh public` checks identity, licensing, and public-boundary policy.

## Thesis 1: Define the user outcome before the mechanism

A CLI command exists to deliver a user outcome, not to mirror a package, protocol, SDK, or vendor API.

### Consequences

- Command names use the user's task vocabulary.
- A single task may compose several adapters.
- A vendor method may remain internal even when an adapter exists.
- New transport flexibility is not accepted as a substitute for a missing product decision.
- Non-goals are recorded so agents do not “complete” the tool by exposing every available method.

### Mechanical enforcement

- Every `cli.CommandSpec` must name a documented public task.
- Catalog tests reject duplicate or undiscoverable command paths.
- Application use cases own orchestration; infrastructure adapters cannot register public commands.
- Work packets require a user outcome and non-goals before implementation tasks.

### Derived-project questions

- What sentence would a user say before reaching for this tool?
- Which outcome does the tool own from start to finish?
- Which vendor concepts must remain implementation details?
- Which superficially related tasks are deliberately unsupported?

## Thesis 2: Close supported outcomes and make them predictable

Humans and agents should be able to discover a command, invoke it, and interpret its result without exploratory network calls or undocumented heuristics.

When a derived product declares an outcome supported, the CLI owns the
deterministic selection, joining, and interpretation needed for routine
success. The user may extract a declared JSON or TSV field, but does not need
an undeclared `jq`/`grep` pipeline, custom parser, source inspection, provider
notation knowledge, or an additional exploratory API call to reconstruct the
answer. A deliberately low-level export or transport utility may define a
narrower promise, but it must be named and classified as such instead of being
presented as a closed task outcome.

### Consequences

- Human root help is a compact command/namespace index; namespace help lists its
  relative leaves; exact command help contains usage, effect, and the complete
  executable input contract.
- Root agent help exposes only outcome-selection facts and a machine-readable scoped-help request; exact-command and namespace help expose invocation, output, authentication, failure, mutation, and workflow details.
- Help and dispatch derive from the same static catalog.
- Value kind, single/repeatable cardinality, omission default, numeric bounds,
  and input dependencies/conflicts derive from the same catalog entry as argv
  parsing. An omitted value, a catalog default, and an explicitly supplied
  empty/zero/false value remain distinguishable.
- Output shape, exit behavior, and error ownership are deliberate public contracts.
- Deterministic multi-step behavior belongs in an application use case rather than an agent prompt.
- Domain and application results preserve declared task identity, every request
  dimension the task carries, and any state distinction, reference kind, or
  bounded uncertainty that affects interpretation before presentation sees it.
- Presentation represents typed facts. It does not infer identity,
  relationships, completeness, or confidence from labels, order, proximity,
  indentation, or other display details.

### Mechanical enforcement

- Catalog-wide help, typed parsing, and routing contract tests run without
  external I/O.
- Parser contract tests exercise text, boolean, integer, repeated, defaulted,
  bounded, dependent, conflicting, absent, and explicitly empty inputs without
  a handler-owned parallel registry.
- Agent-help shape and growth tests reject detailed contracts leaking back into
  the root index and prove an unknown outcome reaches one selected scoped task
  contract in at most two help-discovery invocations. A known path needs one
  scoped-help invocation only when the caller already holds its required
  references and other task inputs; neither bound includes task execution or
  later full-contract retrieval for an out-of-scope workflow endpoint.
- Executable single-shape JSON-output contract tests compare renderer schema
  versions, envelopes, and item fields with the catalog declarations. Dedicated
  exact-key tests fix both the catalog-declared root agent index and its
  input-selected scoped variant.
- Tests cover stable command paths, effects, examples, and negative input behavior.
- Use-case tests fix orchestration order and ambiguity handling.
- Each interpretation-sensitive capability adds task-owned tests for its
  applicable request dimensions and state distinctions. Scoped collections
  retain scope when empty; semantic reference fields reject the wrong kind; and
  relationship-rich outputs include negative canaries for tempting display-only
  inferences. The template sample mechanically proves exact-ID binding,
  successful empty output, same-label identity separation, and no partial
  pagination result; richer capabilities supply their own fixtures.
- Public-contract changes are called out explicitly in pull requests.

### Reviewed evidence

- Agent-readiness records the external-processing count for routine success and
  requires zero undeclared reconstruction steps for a supported outcome. This
  is reviewed transcript evidence; the harness does not infer or mechanically
  verify the count from prose.

### Derived-project questions

- What is the cheapest reliable path from root help to a successful command?
- Which output fields and exit statuses are stable?
- Which deterministic workflow should be one command rather than several agent steps?
- How does a user obtain the unique identifier required by an action?
- What external join, parser, provider notation, or exploratory request would a
  routine caller otherwise need, and why is it not owned by the supported task?
- Which facts can be absent, empty, zero, false, unresolved, or bounded, and
  where are those states typed before rendering?

## Thesis 3: Separate discovery from action and bind one target explicitly

Discovery owns ambiguity. Action owns one uniquely identified target. External or caller-selected targets are bound by an opaque identifier emitted by discovery and accepted unchanged. A command path may instead bind one CLI-owned local singleton when no target choice exists.

### Consequences

- Every public command has a `CommandRole`: `RoleUtility`, `RoleDiscover`, or `RoleAct`; `RoleUnknown` is invalid.
- A `discover` command may accept filters, return zero or more candidates, and emit stable opaque IDs.
- An `act` command uses exactly one target-binding mode: at least one required opaque reference, or one complete catalog-declared fixed target with scope `tool_local`.
- A fixed target has a stable kind, stable ID, description, and scope; the command path is the selection, so the command produces and consumes no references.
- Fixed targets are not a shortcut for external resources, multiple candidates, account selection, or caller-provided local paths.
- An action does not search again, choose the “best” candidate, accept a copied resource URL as an implicit alternative, or reconstruct an identifier from display fields.
- The ID is not decoded, normalized, case-folded, unescaped, or reformatted between producer and consumer unless its domain type explicitly defines that transformation.
- Display labels may change without changing the reference contract.

### Mechanical enforcement

- `cli.CommandSpec` declares `Role`; reference kinds are attached once to structured input and output fields in its `AgentContract`.
- The catalog derives `ProducedRef{Kind, Field}` and `ConsumedRef{Kind, Argument}` projections from those fields, so routing, help, and reference-flow checks cannot drift across parallel registries.
- Catalog validation rejects incomplete, mixed, or role-inconsistent reference/fixed-target declarations.
- Agent help projects role and reference flow from the same catalog used by dispatch.
- Whole-catalog tests prove every consumed reference has a visible producer, every produced reference has a consumer, and no required-reference cycle is closed off from an invocable producer.
- Round-trip tests pass the exact opaque ID bytes emitted by discovery into the action command.
- Fixed-target tests prove that scoped agent help supplies target certainty without ceremonial discovery or input.
- Negative tests reject URLs, resource paths, control characters, and undocumented alternative reference forms before adapter execution.

The runnable proof is `sample list` -> `sample read --id`. It uses reference kind `sample`, producer field `id`, and consumer argument `--id`. The synthetic ID is `smp_` followed by exactly twelve lowercase hexadecimal characters. Validation rejects uppercase, partial IDs, names, URLs, whitespace, and resource paths without rewriting them.

### Derived-project questions

- Which command owns ambiguity and returns candidates?
- What opaque reference kind connects discovery to action?
- If no selection exists, is the object truly one CLI-owned singleton whose stable identity is fixed by the command path?
- Is the action target truly unique, and where is that proven?
- Which tempting identifier conversions would couple the CLI to an external storage or URL format?
- If no in-tool producer exists, what product and catalog change is needed before exposing the action?

## Thesis 4: Declare side effects before executing them

An operation's effect, intent, and target are product facts. They must be known and validated before infrastructure performs the operation.

### Consequences

- `read`, `create`, and `write` are explicit domain values, not guesses derived from an HTTP verb or function name.
- Mutations carry an `operation.Intent` and `operation.TargetRef`.
- The public mutation contract either binds declared CLI reference inputs to target roles or binds one command-declared `tool_local` singleton. A reference-bound `create` consumes one opaque parent/scope reference; a reference-bound `write` consumes an opaque existing-target ID and may consume a distinct parent. A fixed-target mutation has no target inputs; `create` treats the singleton as creation scope and `write` treats it as the existing target.
- `target_inputs` is the complete set of role-bound target inputs, not an unclassified list that can contain extra selectors.
- Unknown or inconsistent effects fail closed.
- Authentication, confirmation, audit, dry-run, and policy decisions can attach to one execution boundary.
- Adapters receive bounded inputs rather than unrestricted clients or executors.
- A confirmed mutation result crosses a dedicated complete-write boundary; a
  cancellation observed after confirmation cannot turn success into a safe
  retry claim. Provider rate-window evidence remains independent from whether
  repeating the same logical mutation is permitted.

### Mechanical enforcement

- Domain constructors and validation reject unknown or incomplete mutation intent.
- The catalog requires a declared effect for every public command.
- Catalog validation retains every reference-bound rule and additionally rejects a fixed-target mutation unless `target_inputs` is an explicit empty list, both input-role fields are absent, and `TargetKind` equals the fixed target kind.
- Architecture lint prevents application code from importing concrete infrastructure.
- Negative tests prove that validation failure occurs before the side effect.

### Derived-project questions

- What assets can each command read, create, change, delete, notify, or publish?
- Which exact opaque input supplies a create's parent or a write's existing target, and does its reference kind match the declared role?
- Is one target reference sufficient, or does the product need a typed multi-target impact model?
- Which effects require human authorization or a dry-run preview?
- What evidence proves that rejection happens before external I/O?

## Thesis 5: Turn important claims into executable contracts

Documentation explains a claim, but a repeatable check preserves it across contributors and agents.

### Consequences

- Architecture, security, compatibility, generated-code, and release promises identify their checks.
- A new invariant is incomplete until its failure mode is tested.
- CI invokes the repository's scripts instead of recreating policy in workflow YAML.
- Generated updates fail visibly when they introduce an unclassified change.
- Exceptions include a reason and a regression test.

### Mechanical enforcement

- `./scripts/check.sh` owns the `fast`, `full`, `security`, `release`, and `public` profiles.
- Task aliases, optional local automation, and CI delegate to that script.
- Tool versions and third-party actions are pinned according to repository policy.
- `task check` is the pre-merge gate; higher-risk operations add their named profile.

### Derived-project questions

- Which current claims rely only on reviewer memory?
- What is the smallest mutation that would violate each invariant?
- Can the check produce an actionable failure message?
- Is the same implementation exercised locally and in CI?

## Thesis 6: Treat public safety as a design boundary

Once source or history reaches a public remote, confidentiality cannot be restored by deleting a later commit. Public readiness begins at repository creation.

### Consequences

- Derived repositories start with clean history rather than copying a private `.git` directory.
- Runnable public defaults replace organization-specific placeholders.
- Fixtures use synthetic identities and data.
- One explicit documentation locale governs trusted repository prose. Stable
  command paths, flags, environment names, fault codes, JSON keys, schema
  values, and reference kinds remain language-neutral machine identifiers;
  external text remains untranslated untrusted data.
- License, disclosure channel, dependency rights, and release behavior are decided before publication.
- Private URLs, organization names, credentials, and internal operating procedures are prohibited in all tracked and generated content.

### Mechanical enforcement

- `.harness/project.json` records identity, documentation locale, and
  public-boundary policy.
- `tools/repoguard` checks forbidden identifiers, secrets, placeholders, required community files, and repository readiness.
- `task security` scans source and configuration.
- `task public:check` is required before the first public push and public release.

### Derived-project questions

- Was any file or Git object copied from a private source?
- Who owns the code and documentation, and which license applies?
- Which identifiers or domains must never appear publicly?
- What private vulnerability-reporting channel exists?

## Thesis 7: Keep one maintainable path through the repository

The project should not depend on one maintainer remembering parallel registries, duplicated policy, or undocumented release steps.

### Consequences

- `AGENTS.md` is the only agent-policy source of truth.
- `cli.Catalog` is canonical; typed parsing, help, and dispatch do not maintain
  separate input or command lists.
- `scripts/check.sh` is canonical; Task, optional local automation, and CI do not duplicate commands.
- Durable decisions live in theses, numbered docs, or ADRs; active implementation state lives in work packets.
- Dependencies are added only when their safety and maintenance value exceeds their ongoing cost.

### Mechanical enforcement

- Contract tests compare every derived view with its source of truth.
- Repository guard checks required documentation and bootstrap state.
- Documentation links and command snippets are checked where practical.
- Completion requires the same full gate regardless of whether a human or agent made the change.

### Derived-project questions

- Where are the current duplicate sources of truth?
- Which recurring judgment needs a Skill, generator, or lint?
- What can a new maintainer safely remove?
- Which maintenance task still requires private knowledge?

## Mature thesis changes

A mature thesis is allowed to change, but not as an incidental implementation edit. Propose the new statement, present user, incident, compatibility, or maintenance evidence, identify the consequences, update the enforcement path, and record migration impact in an ADR. The burden is evidence and repository consistency, not age alone.
