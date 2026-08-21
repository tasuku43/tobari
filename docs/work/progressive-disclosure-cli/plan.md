# Work Plan: Make routine CLI output result-first and task-specific

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Presentation evidence: [presentation-evidence.md](presentation-evidence.md)

## Chosen approach

Combine a result-first default presentation with task-specific command
boundaries. Keep grouped-complete information on existing detailed and machine
surfaces. Do not add a global display mode.

```text
context list           Which work modes exist? Name + Access + Runtime
context show           What happens if I use this Context? Effective result
context show --details What is its complete technical composition?
status                 What is true here, and what should I do next?
cluster status         What is wrong inside shared enforcement services?
policy rules           What persistent decisions were remembered?
```

## Alternatives considered

### Group everything but hide nothing

The current detailed Context grouping could become the default. It is accurate
and low risk but still asks routine users to understand agent profile, native
readiness, revisions, image identity, and stores. Retained for `--details`, not
chosen as the default.

### Global beginner/expert mode

A persisted preference could switch all output density. Rejected because it
adds configuration and hidden state, complicates support/reproduction, and is
unnecessary when commands, `--details`, and JSON already express depth.

### Conditional prose-only hiding

Renderers could omit fields ad hoc when they look default or healthy. Rejected
because labels and omission would invent semantic defaults and diverge across
commands. Every omission/action must derive from typed task-owned state.

## Design

### Public contract

Representative ordinary Context output:

```text
Context default

Access
  Project files      Read-write · changes affect this project directly
  Routine clients    Ready | Limited by Context | Not enabled
  Other requests     Exact review
  Private targets    Denied

Tools
  Runtime            sre@1

Workspace defaults
  Shell presentation Inherited | Standard | Customized
  Git identity       Not imported | Configured
  Bootstrap          None | Configured | Action required

Login                Stays in each Workspace
Details              tobari context show --details
Next                 tobari
```

The exact enum and wording for Routine clients and defaults are finalized only
after the domain-result audit. The renderer does not compute them from enabled,
image, profile, or policy labels.

Representative ordinary status:

```text
Workspace ready
  Root       /projects/app
  Context    default
  Runtime    sre@1
  Session    detached
  Next       tobari --context default
  Details    tobari status --details   # only if an accepted detailed form exists
```

Do not add the illustrated `--details` form unless the catalog already supports
it or this packet explicitly accepts a typed input/output extension after
audit. Until separate packets add task-owned pending/exposure results, ordinary
status omits those rows without claiming `none`.

Human synthetic default uses `Recommended defaults · not saved`; JSON retains
`synthetic_default` and null identity/store distinctions. Routine human output
hides agent profile, native-readiness terminology, IDs, hashes, image selectors,
and paths. Detailed/JSON disposition follows the vocabulary machine-field audit.

### Layer changes

- Domain: add only the smallest typed effective summary/state distinctions
  needed to prevent presentation inference
- Application: validate task identity and return complete task-owned summaries;
  do not join unrelated status/policy/service reads in this packet
- Infrastructure: no new adapter or external read; map existing trusted state
  to domain facts only where already owned by the task
- CLI and catalog: default renderers, exact details links, human root-help
  grouping when catalog-derived, field descriptions, fixtures, and goldens

### Data and control flow

```text
validated Context / Workspace task result
        ↓ domain-owned effective summary and action-required state
        ├─ ordinary renderer → result-first bounded text
        ├─ detailed renderer → grouped complete technical text
        └─ JSON renderer → unchanged complete schema
```

### Error and cancellation behavior

Read behavior, failures, recovery commands, retryability, cancellation, and
exit mapping remain unchanged. A new detailed input or task-result variant, if
needed, must be catalog-declared and rejected before I/O when invalid.

### Security and public boundary

No authority changes. Direct project mutation risk, unknown-request review,
private-target denial, Workspace-owned login, and every action-required
recovery remain explicit. Hiding a healthy technical fact cannot hide a warning
or turn unknown into none.

## Implementation slices

1. Freeze the semantic fixture corpus and answer keys before renderer changes.
2. Audit task result variants and add minimal typed effective summaries.
3. Implement Context list/show ordinary and detailed projections.
4. Implement status hierarchy and catalog-derived human-help grouping within
   the accepted no-aggregation scope.
5. Promote contracts, negative-inference tests, goldens, and generated docs.

## Verification

- Unit and contract tests: effective summaries and action-required state
- Negative side-effect tests: invalid presentation inputs cause no additional
  read/mutation; no new external call is introduced
- Opaque-reference and complete-pagination tests: preserve applicable current
  references and exhaustive Context list scope
- Structured output, hostile-output, and recovery tests: JSON exact keys,
  escaping, exact next argv, absent/unknown distinctions
- Agent-readiness scenario and discovery-round-trip count: root/scoped help
  budgets unchanged and complete details remain one exact command away
- Human-handoff scorecard: not applicable; no setup/auth transfer
- Manual observation: synthetic persisted/default Contexts and Workspace states
- Required profiles: `task check`; conditionally `task public:check` if the
  publishable schema changes despite the default no-schema-change plan
- Generated-diff or artifact checks: catalog/help and architecture-site output

## Rollout and rollback

Default human text changes pre-public. JSON and state remain compatible unless
an audit-driven contract change receives separate approval. Rollback is a
source revert when schemas remain unchanged.

## Documentation promotion

- Product contract: ordinary versus detailed human presentation.
- Architecture: task-owned effective summary before presentation.
- Harness: frozen fixtures, answer keys, negative-inference and exact-next-argv
  checks.
- README/site: result-first examples generated from accepted semantics.
