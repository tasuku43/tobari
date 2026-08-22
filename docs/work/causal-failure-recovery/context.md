# Work Context: Causal failure diagnosis and safe recovery

This file records verified repository facts as of 2026-08-22. Desired behavior
is kept in `plan.md` and is not presented here as current reality.

## Current behavior

### Cross-command fault boundary

- `internal/domain/fault.Error` currently carries `kind`, command-specific
  `code`, safe `message`, retry facts, and `next_actions`. Its private cause is
  deliberately absent from machine output. `PublicCopy` detaches the first
  valid structured fault from all wrapping and cause prose.
- Structured error schema 1, declared by `defaultAgentErrorFields`, has no
  operation phase or mutation/change-state field. Human rendering shows the
  same existing facts only.
- `CLI.normalizeFault` rejects an emitted fault whose kind or retryability does
  not match the selected command's Catalog declaration, then replaces runtime
  next actions with the Catalog-owned actions. Catalog validation accepts only
  an exact command path or `help` plus an exact path/namespace and rejects
  unchecked argument suffixes.
- `execution.Invoker` guarantees zero action calls for validation, policy, and
  pre-execution cancellation failures. After it calls the action, it preserves
  a valid structured adapter fault; every raw error, including raw context
  cancellation, becomes non-retryable `unclassified_mutation_outcome`.
- `emitMutationResult` does not let late cancellation turn confirmed success
  into replay permission. A failed result write becomes non-retryable
  `mutation_output_write_failed` with read-only reconciliation.
- Catalog validation already requires read-only recovery for
  `unclassified_mutation_outcome`, `mutation_output_write_failed`, and a
  non-retryable rate-limited mutation. It does not validate phase or change
  state because neither exists yet.

### Fault-path inventory

| Surface | Verified current path | What is known | Current gap |
|---|---|---|---|
| Root construction | `cli.New` falls back to `systemdoctor` when the Docker-backed runtime cannot be constructed; normal commands return `missing_runtime` and point to `doctor` | Runtime composition is unavailable; Doctor can still perform independent host checks | `missing_runtime` does not state the failed phase, and it must not be reinterpreted as Docker-provider state |
| First-use root entry | `prepareGuidedProjectEntry` reviews, creates the Context, renders `Context created`, then runs `cluster up` | The first Context is confirmed before cluster startup begins | Generic Docker unavailability is discovered after a durable Context mutation; partial success is represented by an earlier line, not one typed failure result |
| Existing root entry | guided entry observes `cluster status`; `EnterProjectSessionInContext` rechecks readiness under the lifecycle lock before Workspace mutation | Unready shared state blocks Workspace mutation | Cluster observation failures are generic wrappers and carry no phase/change facts |
| Cluster startup | `clusterUp` validates build identity before the invoker; raw action errors become `cluster_start_failed` with `cluster status`; structured action faults survive; post-start inspection can return `status_failed` saying startup succeeded | The wrapper knows whether failure was before action, during action, or after confirmed startup | `cluster_start_failed` means only that action was attempted; it cannot prove partial change, but no typed unknown state is emitted. Some structured infra faults happen after Docker changes yet carry no common state contract |
| Cluster status | `InspectCluster` first executes `docker version --format {{.Server.Version}}`; failure is wrapped as status failure at the application boundary | The selected generic Docker endpoint could not provide Engine version | The public fault does not preserve this typed causal class and must not infer which backend is stopped |
| Context creation | `contextcmd.createWithComposition` wraps raw store failures as `context_create_failed`; a returned invalid report becomes `invalid_context_report` with confirmed-create recovery | Known validation/existence/runtime-selection failures occur before mutation; an invalid returned report follows a completed adapter call | The Context store performs multiple atomic file operations and follow-up reads. Generic failure can mean zero, partial, or completed storage, while the message says only that the Context could not be created |
| Workspace select/create/reconcile | root resolution and selection precede the invoker; logical creation and `EnsureProjectRuntime` occur inside one action; raw errors become `runtime_reconcile_failed` and point to `status` | Tests prove some runtime failures retain logical Workspace state and home | The application discards the known instance on error and cannot state when a logical Workspace is confirmed but runtime reconciliation is incomplete |
| Workspace attachment | `EnterProjectRuntime` occurs after the Workspace mutation returns. A non-exit error becomes `enter_failed` and points to `status` | The Workspace mutation completed before attachment began | The fault does not state that the Workspace remains available or whether attachment-owned setup was cleaned up |
| Direct child | Docker exec receives exact argv without a shell. An `ExitCode` error becomes `(exact code, nil)`; root prints session-closed guidance and returns that code | Tobari infrastructure and attachment succeeded sufficiently to run the child; child status belongs to the child | No gap in classification; future global error work must not wrap status 2–13 as Tobari's same-numbered fault classes |
| Doctor | Application owns a finite dependency graph, result-first report, blocked checks, and exact recovery; the CLI emits the report and then `diagnostic_failed` when a check failed | Every ready independent check is observed without repair | Doctor is evidence for typed observation and recovery, not a model to run eagerly before every command. Result plus terminal error can also duplicate recovery presentation |
| Runtime build | A bounded stage vocabulary and visible-projected Docker stream are separate from stable faults; failures retain history and do not select a Context | Concrete upstream build diagnostics can be useful without becoming stable error fields | This is a positive adjacent model, not permission to expose raw Docker output for ordinary lifecycle faults |

### Generic Docker observation and the 24+ gap

- Both `systemdoctor.Inspector` and `dockerruntime.Runtime` use fixed generic
  observations for `docker` on `PATH`, `docker version --format
  {{.Server.Version}}`, `docker context show`, and `docker compose version
  --short`. They do not invoke a backend-specific executable.
- Doctor recovery currently says to install a compatible Docker CLI, restore
  the local Docker Engine, repair the selected Docker context, or enable the
  Compose v2 plugin. It does not name a provider-specific command.
- `InspectCluster` also uses the generic server-version command before its
  other read-only Docker observations.
- The public architecture-site install page says "Docker Engine 24 or newer".
  No repository code found by the audit parses the returned server version or
  rejects a version below 24. Current Doctor tests establish availability, not
  that minimum. This is a claimed-versus-enforced contract gap.

## Relevant structure

- Entry point: `internal/cli/cli.go`, `internal/cli/tobari.go`
- Domain rule: `internal/domain/fault`, `internal/domain/doctor`,
  `internal/domain/operation`, lifecycle result types in `internal/domain/tobari`
- Application use case: `internal/app/execution`, `internal/app/doctorcmd`,
  `internal/app/contextcmd`, `internal/app/tobaricmd`
- Infrastructure boundary: `internal/infra/systemdoctor`,
  `internal/infra/dockerruntime/doctor_observer.go`, `cluster_up.go`,
  `cluster_observation.go`, `context_store.go`, and project lifecycle files
- CLI catalog or presentation: `internal/cli/runtime_catalog.go`,
  `errors.go`, `output.go`, `doctor.go`, and generated agent-help contracts
- Existing tests and harness checks: invoker structured-fault/cancellation
  tests; Catalog recovery-grammar tests; guided-entry partial-success tests;
  project runtime retention and child-exit tests; Doctor dependency/argv and
  read-only tests; mutation-outcome and confirmed-output harness rows

## Constraints

- Cause classification must be based on a typed result from the layer that
  owns the exact operation, never on matching adapter prose.
- A failure result may weaken a claim from partial/none to unknown; it may not
  strengthen unknown state through presentation or Catalog prose.
- The global fault contract remains secret-free. Adapter errors may contain
  paths, socket endpoints, URLs, credentials, environment, or arbitrary
  upstream output and remain private causes only.
- Readiness observation is bounded, read-only, cancellation-aware, and selected
  by application policy. Infrastructure may not decide that every command
  requires the full Doctor graph.
- Tobari owns no Docker-provider lifecycle. Generic Docker context selection is
  host state observed through Docker itself, not a signal to invoke the
  provider behind it.
- Reads remain observational under Thesis 5. A recovery command may reconcile
  only if it is already declared as a mutation; fault presentation cannot
  silently perform that action.
- Direct child status is not a fault envelope. Root exit codes require command
  context to distinguish a child status from Tobari's stable fault exits.
- Public documentation and packet prose are English and contain no private
  paths, real project names, credentials, or copied provider output.

## External facts

No external provider documentation is required for the accepted product
boundary. Before preserving Docker Engine 24 as a technical minimum, the
implementation must record compatibility evidence from supported generic
Docker Engine/API behavior or revise the claim; provider marketing or desktop
application state is not authoritative for this decision.

## Unknowns

- [ ] Is Docker Engine 24 a tested technical minimum for an exact Tobari
      feature, or only an unverified installation recommendation? Replay the
      supported Docker integration matrix or identify the exact required API
      before choosing enforcement versus documentation correction.
- [ ] Which existing structured infrastructure faults can prove `none`,
      `partial`, or `confirmed` without adapter changes? Complete an exhaustive
      mutation-code audit before assigning Catalog change-state declarations.
- [ ] Does every attachment setup failure prove cleanup and absence of an
      active attachment, including cleanup failure? Add deterministic injected
      failures at each setup/close boundary before promising that wording.
- [ ] Should Doctor's result-first `diagnostic_failed` retain a second full
      error frame after the report, or can it preserve the nonzero exit with a
      smaller non-duplicating terminator? Compare from one typed Doctor fixture;
      this question must not block lifecycle fault typing.

## Thesis evidence

- Repeated design decision or point of agent confusion: Docker availability,
  cluster readiness, logical Workspace retention, and attachment startup are
  repeatedly collapsed into generic lifecycle strings despite having different
  owners and lifetimes.
- User outcome or friction observed in the minimal slice: after failure, the
  user needs to know the actual proven causal class, what Tobari changed, and
  the exact next safe Tobari action without learning Docker backend operations.
- Code workaround or exception being considered: backend-specific detection or
  advice would make messages feel concrete but would move provider ownership
  into Tobari and make false diagnoses likely.
- Current thesis that resolves it, or proposed thesis revision: Thesis 0 keeps
  Docker detail advanced and requires one continuation; Thesis 5 makes reads
  observational and recovery explicit. Promote the phase/change-state model as
  their common consequence rather than a command-local exception.
- Downstream impact: product output/exit schema, architecture fault ownership,
  security cause stripping, Catalog declarations and validation, Doctor/shared
  readiness primitives, lifecycle application results, CLI presentation,
  agent-readiness transcripts, generated architecture-site schemas, and
  harness coverage.

## Reproduction or observation

```sh
# Source audit used for the current facts.
rg -n 'cluster_start_failed|context_create_failed|runtime_reconcile_failed|enter_failed|unclassified_mutation_outcome' internal
rg -n 'Server.Version|compose.*version|Docker Engine 24' internal docs

# Existing deterministic tests that expose the composed outcomes.
go test ./internal/cli -run 'GuidedEntryRetainsContext|DelimiterLedRootInvocation'
go test ./internal/infra/dockerruntime -run 'EnterProjectRuntimePreserves|EnsureProjectRuntime'
go test ./internal/app/execution
```

The packet audit inspected source and existing tests rather than running a live
Docker backend. No provider state or unbounded logs were collected.

## Security and public-boundary notes

- Assets and side effects involved: host Context and Workspace state, owned
  Docker resources, shared services, attachment-local routes/grants, terminal
  output, and process exit status.
- Credentials or confidential data involved: none are required; private cause
  chains may contain sensitive material and remain excluded.
- New dependencies, destinations, files, processes, or generated content: no
  new dependency or destination. A preflight reuses purpose-bound generic
  Docker observations and starts no provider or Docker resource.
- External schema provenance, publication rights, and drift evidence: the fault
  schema is Tobari-owned. Docker output is runtime observation, not a fixture or
  published copied schema.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency,
  and cancellation facts: one complete fault result; no collection or
  pagination; finite observation timeout/output; no automatic retry; exact
  read-only reconciliation for unknown state; one propagated context.
- Publication and licensing concerns: none beyond keeping architecture-site
  generated contracts aligned with the public fault schema.

## Glossary

- **Causal identity:** existing stable `kind` plus command-specific `code`; it
  says what Tobari established without reproducing a private cause string.
- **Phase:** the closed stage at which the command stopped: input,
  precondition, observation, mutation, verification, attachment, or
  presentation.
- **Change state:** what the command can prove about its declared effect:
  `not_applicable`, `none`, `partial`, `confirmed`, or `unknown`.
- **Provider:** the host product or VM implementation supplying Docker, such as
  Colima or Docker Desktop. Tobari does not own it.
- **Generic Docker readiness:** bounded observation through the Docker CLI of
  CLI presence, selected context, compatible Engine, and Compose availability.
- **Child outcome:** the foreground shell/direct command's exit status after
  Tobari successfully crossed the infrastructure boundary; it is not a Tobari
  fault.
