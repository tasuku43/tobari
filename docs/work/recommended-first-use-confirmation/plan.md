# Work Plan: Confirm recommended first use on one screen

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Add a root-only recommended Context draft before the existing first-use
composition. When an interactive root invocation observes a known-empty Context
collection, it renders one full-screen review with `Start Workspace`,
`Customize`, and `Cancel`. The draft owns all displayed semantic values and the
exact inputs passed to the existing canonical Context create use case.

`Start Workspace` revalidates the empty collection, creates the reviewed
`default` Context, and only after confirmed creation continues through the
existing shared-service, Workspace, attachment, Bash, or exact-child flow.
`Customize` passes the recommended draft into the existing six-stage wizard as
initial selections without performing host discovery. Standalone Context
creation and every machine path remain unchanged.

## Alternatives considered

### Keep the six-stage wizard as the ordinary path

This exposes every concept and remains useful for deliberate customization,
but it makes the recommended case feel like six decisions and obscures the
combined outcome. It remains available behind Customize.

### Create defaults automatically and show only progress

This minimizes keystrokes but removes the meaningful review of direct project
writes and network behavior. Tobari should reduce ceremony, not confirmation.

### Ask only for Context name, then create the rest implicitly

This keeps one prompt but spends the only first-use decision on a label rather
than the access consequence. `default` is sufficient until a user deliberately
creates another work mode.

## Design

### Public contract

The screen is:

```text
Tobari will create an isolated Workspace for:
  <canonical project root>

Project files
  Read-write · changes are made directly

Network
  Claude Code and Codex routine traffic   allowed
  Other requests                          exact review
  Private and unsafe destinations         denied

Tools
  standard@1

Host configuration
  Not imported

Session
  Open Bash

❯ Start Workspace
  Customize
  Cancel
```

For `tobari -- COMMAND...`, Session becomes `Run <executable> directly`. The
executable is safely projected; later argv is not repeated. The exact argv
already accepted by the root parser remains unchanged.

- Root `tobari` remains the one catalog command and RoleAct fixed-target
  composition. No new command path or reference kind is added.
- Start composes the existing canonical `context create` action with one exact
  draft: `default`, `standard`, guided policy, read-write source, enabled native
  readiness, and absent bootstrap.
- The screen is human interactive text only. Agent help describes the first-use
  outcome and branch but does not invent an agent-selectable prompt input.
- Customize starts at Name with `default` prefilled, then retains the existing
  Filesystem, Network, Runtime, Workspace bootstrap, and Review & Create stages.
  Existing Back, edit, terminal restoration, and one final mutation remain.
- An existing Context collection bypasses the screen and uses current root
  selection behavior.
- Stable first-use race recovery points to the root `tobari` task or a read-only
  Context inspection path; it never appends unchecked argv.
- Success output remains the existing Context, shared-service, Workspace, and
  session progress contract. The review writes prompts to the existing
  interactive presentation stream and does not create a second JSON shape.

### Layer changes

- Domain: add or reuse one validated recommended Context draft and task identity
  whose fields distinguish absence from explicit values. Pure validation proves
  the draft is exactly the supported recommendation.
- Application: own the pre-create observation and revalidation, call the
  existing Context creation port once, and allow later root composition only
  after a valid confirmed result. Do not expose an unrestricted state store or
  executor.
- Infrastructure: retain atomic Context uniqueness and existing shared-service
  and Workspace adapters. Add no recommended-setting policy in infrastructure.
- CLI and catalog: select the root-only branch, render the draft, collect one
  action, seed the existing wizard for Customize, and update catalog-derived
  human and agent help.

### Data and control flow

```text
root parsed inputs and canonical project root
  -> read exact Context collection state
  -> known non-empty -> existing root behavior
  -> known empty -> build validated recommended draft
       -> render one screen
       -> Cancel -> restore terminal; stop
       -> Customize -> existing six-stage wizard -> canonical create
       -> Start -> revalidate empty and default absent
            -> canonical Context create with exact displayed draft
            -> validate confirmed result
            -> prepare shared services
            -> select or create Workspace
            -> enter Bash or exact child argv
```

The draft, presentation fixture, create input, and confirmation-result tests
bind the same values. Presentation labels, menu order, or a displayed project
path never become mutation identity.

### Error and cancellation behavior

- Context-state read failure returns the existing bounded failure and does not
  show a draft based on unknown state.
- Invalid project root or invalid direct command fails before the screen and
  before mutation.
- Cancel, EOF, unsupported terminal, and review-render failure create no Context
  and perform no later root side effect. A supported line-mode equivalent may
  be used where the current wizard already permits it.
- If the collection changes after review, revalidation returns a stable
  retryable observation fault before create. It does not silently select the
  new Context, overwrite `default`, or continue shared-service preparation.
- A Context-create failure preserves its existing typed classification and
  prevents all later root actions.
- Once Context creation is confirmed, later cluster or Workspace failure uses
  existing reconciliation and may leave the created Context. Error copy must
  say what exists and give an exact safe next command.
- Customize cancellation returns from the wizard without creating a Context;
  it does not fall through to Start.
- Direct-child exit after successful entry remains the child's exact host exit
  status and cannot be replaced by earlier setup success.

### Security and public boundary

- The screen does not weaken source or network policy. It makes the effective
  boundary visible in one place.
- Host configuration is neither discovered nor imported on Start. Customize
  reaches the existing host discovery boundary only after the explicit choice.
- Recommended values are compiled product policy above infrastructure, not
  mutable project, Runtime, environment, or host input.
- Project paths and executable names use the existing visible projection.
  Remaining argv, project content, host files, credentials, IDs, hashes, and
  store paths are absent from the screen.
- Tests use temporary state, fake runtime ports, fixed clocks, and synthetic
  paths. Cancellation tests assert zero Docker, network, host-file, and durable
  write calls.

## Implementation slices

1. Promote the root-only recommended-first-use contract through theses and
   product documentation; add one frozen typed fixture and failing catalog,
   presentation, branch, and zero-side-effect tests.
2. Add the validated recommended draft and application ordering, including
   empty-state revalidation and concurrent-change faults.
3. Add the one-screen raw and line presentation and seed adapter into the
   existing wizard without duplicating settings or render authority.
4. Compose confirmed creation into the existing root service, Workspace, Bash,
   and direct-child paths; add complete failure and exact-status coverage.
5. Update README, architecture, capability ledger, harness, public help,
   architecture site, and agent-readiness evidence.

## Verification

- Unit and contract tests: exact recommended draft, Context-empty observation,
  presentation equality, Start, Customize, Cancel, existing-Context bypass,
  standalone wizard unchanged, and complete direct mode unchanged.
- Negative side-effect tests: cancellation, output failure, invalid project,
  invalid direct argv, unknown Context state, concurrent creation, and Context
  create failure cause zero later cluster, Docker, Workspace, host-file, or
  network calls.
- Presentation tests: frozen semantic fixture and answer key, raw and line
  goldens, direct executable projection, absent later argv, no internal names,
  and same-fixture before and after comparison.
- Catalog and help tests: root human and scoped agent help explain the branch;
  no new routable command, hidden default in machine input, or recovery argv.
- Integration tests: first-use Bash and exact child, child nonzero status,
  Workspace reuse on the next invocation, and terminal restoration.
- Agent-readiness scenario: unknown-path discovery remains root index plus one
  root scope; the human first-use journey needs one Start and zero command
  guesses or external processing.
- Required profiles: focused Go tests, `task check`, and
  `task public:check`. Run `task security` if implementation changes the trust
  boundary rather than only presenting the existing one.
- Generated checks: help, capability ledger, architecture site, public guard,
  and `git diff --check`.

## Rollout and rollback

This is pre-public behavior with no persisted schema migration. Contexts already
created by the old wizard remain unchanged. Safe rollback removes only the
root-only screen and restores the existing six-stage first-use route; it does
not delete the `default` Context or any Workspace created through the accepted
flow.

## Documentation promotion

- State in the North Star and product contract that ordinary first use reviews
  one recommended Workspace boundary; detailed Context creation is progressive
  disclosure.
- Document root-only composition and operation ordering in architecture.
- Add the typed fixture, zero-side-effect matrix, concurrency check, help
  contract, and first-use agent journey to the harness.
- Update README examples and capability ledger without presenting the fast path
  as a machine-default or a second Context-create command.
