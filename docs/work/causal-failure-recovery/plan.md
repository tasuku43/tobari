# Work Plan: Causal failure diagnosis and safe recovery

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Presentation evidence: [presentation-evidence.md](presentation-evidence.md)

## Chosen approach

Extend Tobari's existing structured-fault and Catalog contract instead of
creating a diagnosis subsystem beside it. Existing `kind` and command-specific
`code` remain the causal identity. Every public structured fault gains two
closed typed facts: the command `phase` and its `change_state`. The Catalog
declares and validates those facts with kind, retryability, and exact recovery;
the emitting layer must agree or the CLI fails closed.

Add one shared domain vocabulary for generic Docker readiness and two bounded
application-selected profiles: Doctor's complete graph continues unchanged,
while first-use Start-Workspace and `cluster up` may request only the exact
generic prerequisites they need before mutation. Infrastructure implements the
observations with fixed read-only Docker CLI argv. No layer detects or controls
the provider behind Docker.

Carry explicit phase receipts through the composed lifecycle. An owning layer
may say `partial` or `confirmed` only with typed evidence. Otherwise an error
after the action boundary is `unknown` and routes to read-only reconciliation.
Direct child nonzero exit remains outside the fault contract.

## Alternatives considered

### Tell users how to start a detected Docker backend

Rejected. Context/socket/process-name heuristics cannot prove which host product
owns the Engine or whether starting it is safe. Backend management would add
provider dependencies and side effects to a tool whose outcome is isolation,
not Docker administration.

### Wrap every failure with `run doctor`

Rejected. Doctor deliberately observes a complete independent graph and emits
a result-first report. Eagerly running it before every command adds latency,
duplicates observations, can obscure the command phase, and does not establish
mutation state. The design reuses typed observation primitives, not Doctor's
presentation or entire scheduler.

### Improve only human messages

Rejected. Prose-only phase and mutation claims drift between commands and leave
agents to infer safety. Human and JSON output must project the same typed facts,
and Catalog validation must make the claims executable.

### Add a second generic cause taxonomy

Rejected. `fault.Kind` and command-specific `code` already form the stable
cross-command and exact causal identity. A parallel `cause_class` enum would
overlap them. The missing orthogonal facts are phase and change state.

## Design

### Public contract

- No new public command is added and existing command roles/effects remain.
- Structured error JSON advances as one reviewed schema change. In addition to
  existing fields, every error includes:
  - `phase`: `input`, `precondition`, `observation`, `mutation`,
    `verification`, `attachment`, or `presentation`;
  - `change_state`: `not_applicable`, `none`, `partial`, `confirmed`, or
    `unknown`.
- Reads use `not_applicable`. Input/precondition failures of mutations use
  `none`. `partial` requires proof that a proper subset of the command effect
  is durable. `confirmed` requires proof that the declared mutation completed,
  even when verification or output later fails. `unknown` means an action was
  attempted and its effect cannot be classified safely.
- `kind` and `code` remain causal identity. Stable code changes are made only
  where the current code collapses two states that require different recovery;
  they are not derived by parsing Docker or adapter text.
- Text shows `Phase` and `Changes` beside the existing cause/retry facts. It
  uses short state-owned wording and the same exact Catalog next action as JSON.
- Recovery from `partial`, `confirmed`, or `unknown` is read-only. A read may
  then recommend one exact mutation, but the failure path performs neither.
- Root direct-child status remains the existing special exit contract: exact
  child status, no structured Tobari error. Help and agent contract must keep
  that distinction explicit because numeric statuses can overlap Tobari exits.
- There are no opaque reference changes, pagination, or external API schemas.
  Routine failure interpretation uses zero undeclared discovery or processing.

### Failure-state assignment rules

| Condition | Phase | Change state | Recovery rule |
|---|---|---|---|
| Parser, review, intent, policy, or cancellation before action | `input` or `precondition` | `none` for a mutation; `not_applicable` for a read | Retry/help only when Catalog permits |
| Read-only Docker/cluster/status observation fails | `observation` | `not_applicable` | Exact read or Doctor; never backend control |
| Mutation action returns a typed zero-change fault | `mutation` | `none` | Command-specific correction; retry only if explicitly safe |
| Owning layer confirms a proper subset persisted | `mutation` or `verification` | `partial` | Exact read-only target reconciliation |
| Owning layer confirms the declared mutation, then verification/output fails | `verification` or `presentation` | `confirmed` | Exact read-only reconciliation; never replay |
| Action ran but valid structured outcome evidence is absent | `mutation` | `unknown` | Existing non-retryable unclassified fault and exact read-only reconciliation |
| Workspace is confirmed and attachment setup fails before child start | `attachment` | `confirmed` for the Workspace create effect | `status`; claim no live attachment only after cleanup tests prove it |
| Foreground child exits nonzero | no fault | no fault | Return exact child status and normal closure guidance |

The assignment is relative to the Catalog-declared command effect, not every
incidental cache, Docker layer, or temporary file. If BuildKit may retain cache
but a Runtime build appended no revision, the command mutation can still be
`none`; separately documented engine-owned residue remains presentation fact.

### Generic readiness profiles

One typed observation vocabulary covers:

1. Docker CLI available by exact executable lookup;
2. selected Docker context readable through Docker;
3. Engine reachable and, if the 24+ claim is retained, generically compatible;
4. Compose v2 available.

Application code selects a profile and ordering. Infrastructure returns only a
closed observation ID/status and bounded safe detail; it does not select
recovery or inspect a provider.

- First-use root: after the user confirms Start/Customize and before Context
  creation, run the minimal full-outcome prerequisite profile. On failure,
  change state is `none` and next action is `doctor`.
- Standalone `context create`: no Docker preflight because its public outcome is
  host-only Context creation.
- `cluster up`: observe its generic Docker prerequisites before entering the
  mutation invoker. Build/image/policy compatibility that needs the mutation
  use case stays in its existing bounded phase.
- Existing Workspace entry: do not add a duplicate eager profile when
  `cluster status`/ready-cluster observation already supplies the prerequisite.
- Doctor: keep its complete graph and result-first semantics; share observation
  types/adapters where ownership permits, not rendered output.

The Engine-version task first settles the claim. If 24 is retained, parse only
the exact generic server-version field with a closed numeric rule, report
unsupported/unparseable state without provider inference, and test boundary
versions. If no required behavior justifies 24, revise public prerequisites and
the install page instead of inventing enforcement.

### Layer changes

- Domain: add closed phase/change-state validation to structured faults; add a
  small provider-neutral readiness observation vocabulary; add lifecycle phase
  receipts only where they express proven domain state.
- Application: assign phase/change state before and after the mutation invoker;
  select generic readiness profiles; preserve typed adapter outcomes; keep
  read-only reconciliation and child outcome separate.
- Infrastructure: implement bounded fixed-argv generic Docker observation;
  classify operation results at exact call sites without parsing prose; return
  explicit receipts for Context/Workspace/cluster phases that can be proven.
- CLI and Catalog: declare phase/change state for every fault, validate runtime
  agreement and recovery effect, advance the error schema, render equivalent
  human/JSON facts, and keep direct-child exit unwrapped.

### Data and control flow

```text
Catalog command + parsed intent
        |
        +-- application-selected read-only prerequisites
        |       -> typed generic observations
        |       -> precondition/none fault on failure
        |
        +-- execution.Invoker
        |       -> zero-call failures: precondition/none
        |       -> typed action result: none/partial/confirmed
        |       -> raw post-action error: mutation/unknown
        |
        +-- optional verification or attachment
                -> typed confirmed/partial fact only from owned receipt
                -> direct child exit bypasses fault presentation

fault.Error -> Catalog declaration agreement -> safe human/JSON projection
                                      -> exact read/help recovery validation
```

### Error and cancellation behavior

- Pre-action cancellation is retryable only under the existing Catalog rule and
  carries `none` for mutations.
- Cancellation or deadline returned raw after an action remains non-retryable
  `unknown`; it is not normalized to a safe retry.
- A valid structured action fault must state phase/change state and pass Catalog
  agreement. Private cause chains are stripped before the CLI boundary.
- Cluster startup possible-partial state remains `unknown` until infrastructure
  proves a proper subset. `cluster status` is the next action before another
  startup.
- Context creation raw storage failure remains `unknown` and uses `context list`.
  Validation/existence failures remain `none`; an invalid report after a valid
  created receipt is `confirmed`.
- Workspace runtime failure becomes `partial` only when the returned receipt
  proves logical Workspace/home creation. Otherwise it is `unknown`. Attachment
  failure after confirmed ensure is `confirmed` relative to Workspace creation.
- Final output failure remains `presentation/confirmed` for mutations.
- No automatic retry, provider startup, repair, or rollback occurs.

### Security and public boundary

- The readiness runner has a fixed executable and argv allowlist, one context,
  finite timeout, bounded output, and a controlled environment. No shell or
  provider executable is used.
- Docker version/context/Compose output is external text. Only closed parsed
  values or visibly projected bounded detail may be shown; raw stderr remains a
  private cause.
- Tests inject hostile URLs, control characters, credential-like values,
  oversized output, context names, socket paths, and provider names and prove
  none becomes structured cause or recovery authority.
- No new credential, network destination, Docker socket mount, dependency, or
  durable state is introduced.

## Implementation slices

This is one coherent work packet because the fault schema, Catalog declaration,
application classification, and presentation must agree atomically. It is too
cross-cutting for one undifferentiated implementation commit. Deliver it as the
following staged, buildable commits inside the same packet; do not expose the
new schema until all required declarations and renderers compile together.

1. Durable decision, typed fault phase/change state, Catalog declarations and
   validation, global cancellation/unclassified/output rules, and schema/golden
   tests.
2. Provider-neutral readiness observation vocabulary/adapters, Docker 24+
   claim resolution, Doctor sharing, and zero-side-effect/provider canaries.
3. First-use and `cluster up` preflight, plus cluster action/verification
   receipts and presentation fixtures.
4. Context and Workspace create/reconcile/attach receipts, causal faults, and
   retained-state/cleanup tests.
5. Direct-child nonzero regression, representative non-lifecycle commands,
   human/JSON same-fact evidence, agent-readiness scenarios, durable docs, and
   repository gates.

If the exhaustive Catalog assignment in slice 1 cannot remain reviewable, it
may be split into mechanically generated command-family commits, but not into a
second competing packet or a period where undeclared faults are accepted.

## Verification

- Unit and contract tests: fault enum/validation/copy; Catalog declaration and
  mismatch rejection; phase transitions; lifecycle receipts; exact child exit.
- Negative side-effect tests: first-use preflight before Context creation;
  zero Docker mutation; zero provider command/process/application control;
  standalone Context creation remains Docker-free.
- Opaque-reference and complete-pagination tests: not applicable; verify no
  reference/pagination contract changes.
- Structured output, hostile-output, and recovery tests: error schema/goldens;
  human/JSON answer-key equality; bounded hostile Docker output; exact recovery
  grammar; no false state strengthening.
- Agent-readiness scenario and discovery-round-trip count: new-project Docker
  unavailable; cluster partial/unknown; Workspace retained; child status. Each
  failure provides its first exact safe action in the same invocation, so the
  recovery discovery count is zero.
- Human-handoff scorecard: not an authentication/setup acquisition capability;
  record only whether the one fault frame explains cause, changes, and next
  Tobari action without backend knowledge.
- Manual observation: optional replay against one supported generic Docker
  Engine for available/unavailable/version-boundary behavior; never manage the
  provider during the observation.
- Required profiles: focused Go tests, `task check`, `task security`, and
  `task public:check` when generated/public docs change.
- Generated-diff or artifact checks: Catalog/agent-help generated fixtures,
  architecture-site generated catalog/schema, and packet removal.

## Rollout and rollback

No persisted Context, Runtime, Workspace, or Docker state schema changes. The
structured error machine schema changes and must be versioned/documented in one
release boundary. Human output adds facts but retains stable codes and exact
commands. Rollback restores the prior error schema/rendering and preflight; it
does not require state migration. Generic readiness observations create no
state to remove. Any newly differentiated fault code can be rolled back only
with its Catalog/help snapshots and agent compatibility note.

## Documentation promotion

- Add an ADR defining causal identity, phase, change state, proof ownership,
  provider agnosticism, and direct-child exclusion.
- Update Thesis 0/5 consequences only if the existing one-continuation and
  explicit-recovery language is insufficient; otherwise record this model as
  their mechanical enforcement.
- Update product structured-error schema, lifecycle failure/partial-success,
  first-use preflight, Doctor relationship, and exit contracts.
- Update architecture layer ownership, Docker abstraction, and
  cancellation/error flow.
- Update security cause-stripping, hostile-output, provider non-authority, and
  mutation-state rules.
- Update harness rows and agent-readiness scenarios, plus public installation
  requirements after resolving Docker 24+.
