# Work Plan: Create a Context from an explicit Base

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Extend the existing `context create` workflow with one dedicated Base stage.
When no persisted Context exists, retain the current Tobari-recommended draft
without showing a redundant chooser. When persisted Contexts exist and
`--base` is omitted on an interactive invocation, show a complete deterministic
Base chooser before name and individual settings, with the current Context
initially selected. An exact `--base <context-name>` seeds the draft directly
and skips the chooser. After Base selection, reuse the current complete
Review/Customize/Create flow.

Base materializes one standalone draft. It is not persisted as Context lineage,
does not become a live configuration source, and supplies no mutation target or
parent authority. The final fixed-target Context-catalog create receives the
complete reviewed values and creates one new Context atomically.

## Alternatives considered

### Put Base inside Customize

This avoids a new top-level step but presents the initializer of every setting
as if it were one peer setting beside Project files, Network, and Runtime. It
also makes changing Base after partial edits ambiguous. Rejected because the
abstraction levels differ.

### Implicitly use only the current Context

This preserves the shortest flow, but choosing another Base would require
mutating the global omission default through `context use` or discovering an
otherwise hidden flag. Rejected because current-Context selection and draft
initialization are different user tasks.

### Add `context revise`, `context clone`, or `--from`

A second command duplicates the canonical create contract and suggests source
mutation or persistent lineage. `--from` frames the operation as ancestry rather
than choosing initial settings. Rejected in favor of the existing create action
and the accepted `--base` vocabulary.

## Design

### Public contract

- Command remains `context create`, `RoleAct`, `EffectCreate`, fixed target
  `tool_local` Context catalog, capability `context.composition`.
- Add optional text flag `--base <context-name>`, completed from existing
  Context names. It is a deterministic draft-source selector, not a reference,
  mutation target, parent input, or persisted output relationship.
- Interactive omission with zero Contexts uses Tobari recommendations and does
  not show a Base chooser.
- Interactive omission with persisted Contexts reads the exhaustive local
  Context collection and shows a dedicated Base step before name. The current
  Context is initially selected; other Contexts and Tobari recommended settings
  remain visible choices.
- Explicit `--base` skips interactive Base selection. Other supplied create
  inputs override the corresponding draft values; omitted values inherit the
  selected Base instead of reopening their initial collection stages.
- Changing Base after customization warns that the complete draft will be
  replaced. Confirmation resets the draft to the newly selected Base; no hidden
  override merge occurs.
- Review identifies the selected Base and presents the complete resulting
  settings. Create emits the ordinary confirmed Context result and exact next
  action. Human details may describe the draft source, but stored Context and
  stable machine output gain no lineage unless the implementation audit proves
  a task-identity field is required and the product contract explicitly accepts
  it.
- Invalid or missing explicit Base fails before mutation with recovery through
  `context list` or exact scoped help. Cancel and output/presentation failures
  before action perform zero mutation.
- `--from` is absent from parsing, help, completion, docs, and recovery text.

### Layer changes

- Domain: add a pure, validated Base/draft composition that can materialize the
  complete standalone Boundary, exact Runtime, and Workspace defaults without
  lower-lifetime state or lineage.
- Application: own Base observation and/or complete-draft creation behind the
  smallest Context ports; preserve the canonical mutation invoker and atomic
  confirmed result. The implementation audit chooses the narrowest split that
  prevents CLI-owned storage policy.
- Infrastructure: project one exact Context's copyable settings and atomically
  create the new standalone stores; never copy Workspace, authentication,
  learned-rule, or Attachment state.
- CLI and catalog: add the typed `--base` input, completion, Base chooser,
  seeded review/customization, help, output, faults, and golden evidence from
  the canonical Catalog and existing Context-create presentation machinery.

### Data and control flow

```text
context create argv
        |
        +-- no Contexts ----------> recommended Base
        +-- --base NAME ----------> exact Context Base read
        +-- interactive omission -> Base chooser (current preselected)
                                      |
                                      v
                           complete standalone draft
                                      |
                           name / review / Customize
                                      |
                            confirmed complete values
                                      |
                  canonical fixed Context-catalog mutation
                                      |
                        new Context report and next action
```

### Error and cancellation behavior

- Invalid name, unknown Base, invalid copied settings, or incomplete draft fails
  before the mutation invoker performs creation.
- Base discovery/read is bounded to the exhaustive local Context catalog and
  uses the caller context. No network or Docker read is needed merely to choose
  a Base; exact Runtime readiness validation remains part of creation.
- Ctrl+C, q, Cancel, rejected Base reset, and terminal rendering failure create
  no Context and do not change the current Context.
- Once action begins, preserve structured faults before generic cancellation;
  unclassified outcomes remain non-retryable and recover through `context list`.
- Confirmed mutation output remains authoritative; a late cancellation or short
  writer cannot make the same create appear safely retryable.

### Security and public boundary

The change adds no credential, external I/O, network destination, dependency,
or new trust principal. Copy only Context-owned work-mode settings whose
lifetimes permit standalone creation. Explicitly exclude Workspace homes,
native tool authentication, remembered permissions, active Attachment routes
or grants, and current/default selection. Synthetic public-safe Context names
and settings back every fixture.

## Implementation slices

1. Contract, Base vocabulary, and failing domain/catalog/presentation tests.
2. Complete Base-to-draft composition and application ordering/zero-mutation
   behavior.
3. Atomic infrastructure projection/creation for every copyable Context part.
4. Catalog `--base`, completion, chooser, review/customization, and output.
5. Durable ADR/product/architecture/security/harness/readiness promotion and
   full gates.

## Verification

- Unit and contract tests: Base validation, complete standalone draft, exact
  copy/override behavior, new ID, unchanged Base, and catalog grammar.
- Negative side-effect tests: unknown Base, invalid copied data, reset decline,
  cancel, stale/removed Runtime, terminal failure, and concurrent collection
  change all prove the declared zero-mutation or structured outcome.
- Opaque-reference and complete-pagination tests: no new opaque-reference or
  pagination flow; catalog tests prove `--base` is not bound as target/parent.
- Structured output, hostile-output, and recovery tests: visible projection for
  hostile Context names, exact executable recovery paths, unchanged machine
  keys unless deliberately versioned, and no display-derived lineage.
- Agent-readiness scenario and discovery-round-trip count: known command uses
  one scoped-help lookup; Context-name completion or `context list` supplies the
  exact Base without external processing.
- Human-handoff scorecard for setup/authentication candidates: not applicable;
  no setup or authentication capability changes.
- Manual observation: zero-, one-, and multiple-Context raw/line terminal flows;
  explicit `--base`; Base reset after edits; Cancel.
- Required profiles: focused Go/catalog/contract tests and `task check`;
  `task security` for changed authority-copying claims; `task public:check` if
  public documentation changes. Release is not expected.
- Generated-diff or artifact checks: site/catalog generation and clean diff;
  preserve unrelated review-command edits.

## Rollout and rollback

No persisted Context schema migration or lineage state is intended. Existing
direct complete `context create` invocations retain their meanings. Removing
the new Base selection code restores the previous creation flow without
rewriting existing Contexts; Contexts created from a Base are ordinary
standalone Contexts and remain valid.

## Documentation promotion

- Product contract: Base-first Context creation and `--base` grammar.
- Architecture: Base as a read-only draft initializer, complete standalone
  composition, and atomic fixed-target creation.
- Security model: exact copyable settings and explicit exclusion of lower-
  lifetime Workspace/authentication/permission/Attachment state.
- ADR 0071 revision or successor: Context creation may initialize from another
  work-mode snapshot without creating inheritance.
- Harness and agent readiness: Base chooser, no-lineage, zero-mutation, and
  zero-external-processing evidence.
