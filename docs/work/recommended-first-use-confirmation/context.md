# Work Context: Confirm recommended first use on one screen

## Current behavior

- Product documentation defines first use as the root `tobari` command
  composing Context creation, shared-service preparation, Workspace creation or
  selection, and entry.
- With no supplied creation value, `context create` uses one continuous
  six-stage terminal flow: Name, Filesystem, Network, Runtime, Workspace
  bootstrap, and Review & Create.
- The Network stage already shows routine Claude Code and Codex traffic, every
  effective method decision, and the private and unsafe destination ceiling.
  Detailed method editing opens only after an explicit Customize choice.
- The Runtime stage always shows `standard@1` and every ready managed revision,
  even when the built-in Runtime is the only choice.
- The Workspace-bootstrap stage defaults to not configured and performs no host
  read until Configure from host is selected.
- Final Context review already renders the selected filesystem, network,
  Runtime, and bootstrap values and supports section editing before one Create.
- Standalone redirected or JSON Context creation requires the complete direct
  input group and never prompts or supplies hidden defaults.
- Root direct child execution already preserves exact argv without a shell,
  leaves Workspace PID 1 fixed, closes attachment-owned routes when the child
  exits, and returns the exact child status to the host.

## Relevant structure

- Entry point: root catalog specification and root command orchestration under
  `internal/cli/tobari.go`
- Domain rule: Context creation selection, Context collection state, Runtime
  revision, Workspace session request, operation intent and impact under
  `internal/domain/tobari`
- Application use case: Context create and root Workspace preparation under
  `internal/app/contextcmd` and `internal/app/tobaricmd`
- Infrastructure boundary: owner state, Context persistence, shared services,
  Docker Workspace reconciliation, and attachment under
  `internal/infra/dockerruntime`
- CLI catalog or presentation: `cli.Catalog`,
  `internal/cli/context_create_wizard.go`, root help, Context renderers, and
  selector terminal handling
- Existing tests and harness checks: Context wizard raw and line flows,
  complete direct input, root first-use composition, direct-child status,
  catalog/help contracts, public guard, and architecture-site generation

## Constraints

- The recommended review is part of root task composition, not a second Context
  creation command or a shortcut around the canonical Context create action.
- One explicit Start is both the review of the shown draft and permission to
  continue the already-declared root composition. It cannot authorize settings
  absent from the screen.
- The project root is canonicalized before display and mutation. Display text
  is safely projected and never reconstructed into identity.
- The default Context name is a human label. The create boundary still issues
  the stable Context ID and validates uniqueness atomically.
- `standard@1` is a display revision backed by the compiled standard Runtime's
  stable identity. Presentation must not turn the label into authority.
- Routine client compatibility remains bounded by enabled native readiness and
  the Context ceiling. The screen states the result without exposing baseline,
  overlay, or projection vocabulary.
- Host configuration remains untouched on the recommended path. "Not imported"
  means no discovery as well as no persisted bootstrap.
- Repository documentation is English and fixtures use synthetic project roots
  and child commands.

## External facts

None. This decision is based on Tobari's existing implementation, tests,
product contract, and observed first-use interaction rather than an external
interface contract.

## Unknowns

- [ ] Which current root orchestration read proves that the Context collection
      is known empty before the screen without initializing unrelated durable
      state?
- [ ] Can the existing final-review renderer be factored around one typed draft,
      or should the root first-use summary use a separate result-focused
      projection with a shared semantic fixture?
- [ ] What exact stable fault and read-only next action should represent a
      Context created concurrently after the screen was rendered?
- [ ] How should the raw and bounded line-mode screens name a direct executable
      containing controls, spaces, or an explicit empty value while omitting
      all later argv?
- [ ] Does shared-service preparation currently begin before the first-use
      Context decision anywhere in root orchestration, and must ordering be
      moved to satisfy zero later side effects on cancellation?

## Thesis evidence

- Repeated design decision or point of agent confusion: ordinary users must
  currently traverse Context name, Runtime, and Workspace bootstrap concepts
  even when they intend to accept the one recommended configuration.
- User outcome or friction observed in the minimal slice: the user wants one
  `tobari` invocation to enter a useful isolated Workspace and to understand
  the direct-write and network consequences before it starts.
- Code workaround or exception being considered: auto-creating hidden defaults,
  weakening standalone direct-input requirements, or maintaining a second set
  of recommended values in root presentation.
- Current thesis that resolves it, or proposed thesis revision: root first use
  may compose one reviewed recommended Context draft, while standalone Context
  creation remains the complete customization and automation boundary.
- Downstream impact: root command help and composition, Context creation draft,
  human presentation, concurrency ordering, README, product contract,
  architecture, capability ledger, harness, and agent readiness.

## Reproduction or observation

```sh
go test ./internal/cli -run 'ContextCreateWizard|FirstUse|Root'
go run ./cmd/tobari help tobari
go run ./cmd/tobari help context create
```

Use a temporary synthetic state root and fake runtime ports to capture the
current six-stage transcript and the proposed one-screen transcript from the
same semantic settings. Do not start Docker or inspect host configuration in
the cancellation and Customize-before-selection fixtures.

## Security and public-boundary notes

- Assets and side effects involved: Context collection read and create, shared
  services, Workspace persistence, Docker reconciliation, terminal mode, and
  optional exact child execution.
- Credentials or confidential data involved: none. Host configuration and
  credentials are not read on the recommended path.
- New dependencies, destinations, files, processes, or generated content: none
  expected. A presentation dependency would require the repository's existing
  CLI dependency and ADR review.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency,
  and cancellation facts: the Context-empty observation is exhaustive at one
  local snapshot; Start is one confirmed create followed by ordered root
  composition; concurrent change fails instead of retrying or adopting state;
  there is no pagination; cancellation before create performs zero mutation;
  post-create failures retain the existing reconciliation path.
- Publication and licensing concerns: none beyond ordinary public documentation
  and synthetic fixtures.

## Glossary

- **Recommended draft:** the exact typed Context values displayed and submitted
  by the root first-use screen.
- **Fast path:** the one-screen root-only review. It is not an unreviewed or
  machine-default path.
- **Customize path:** the existing complete six-stage Context wizard entered
  deliberately from the recommended screen.
- **Later side effect:** shared-service, Docker, Workspace, or attachment work
  that must not begin when Context review or creation has not succeeded.
