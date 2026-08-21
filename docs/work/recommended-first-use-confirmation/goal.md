# Work Goal: Confirm recommended first use on one screen

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, and `docs/04_harness.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Pre-public self-use
- Related ADRs: None; add one only if implementation reveals a durable decision not already chosen by the governing theses

## Outcome

On the first interactive `tobari` invocation for an installation with no
Context, the user sees one result-focused review of the recommended Workspace
boundary. `Start Workspace` creates a Context named `default` with the reviewed
standard settings, prepares shared services, creates or selects the current
project's Workspace, and enters Bash or runs the exact requested child.
`Customize` opens the existing complete six-stage Context wizard, and `Cancel`
performs no mutation.

## Why now

The six-stage Context wizard is precise, but the ordinary first-use path asks
every user to decide names and concepts that have one reviewed recommendation.
Repeated Enter presses are not six independent safety decisions. A single
screen can show the direct-project-write consequence, network behavior,
standard tools, absent host import, and resulting session more clearly while
retaining the existing wizard for users who deliberately customize.

## Non-goals

- Do not remove or simplify standalone `context create`, its six-stage wizard,
  its complete direct-input mode, or its JSON contract.
- Do not auto-create a Context without one explicit reviewed action.
- Do not read host AWS, Kubernetes, Git, credential, shell, or other
  configuration while preparing the recommended screen.
- Do not select an installed managed Runtime automatically. The recommendation
  remains the built-in `standard@1` revision.
- Do not import host credentials or enable experimental authentication.
- Do not add global novice, expert, quick-start, or setup modes.
- Do not change existing-Context selection, ambiguity handling, Workspace
  identity, direct-child argv, child status, or attachment cleanup.
- Do not hide that read-write access changes the real project directly.

## Acceptance criteria

- [ ] The fast path appears only for an interactive root `tobari` invocation
      when the Context collection is known empty. Existing installations and
      standalone `context create` retain their current behavior.
- [ ] Before any create, cluster, Docker, network, host-file, or Workspace
      mutation, one screen identifies the canonical project root and shows:
      project files read-write with direct changes; routine Claude Code and
      Codex traffic allowed; other requests exact review; private and unsafe
      destinations denied; `standard@1`; host configuration not imported; and
      the resulting Bash or direct-command session.
- [ ] The actions are exactly `Start Workspace`, `Customize`, and `Cancel`, with
      `Start Workspace` selected by default. The screen never implies that six
      Enter presses are a stronger confirmation.
- [ ] `Start Workspace` submits the same typed Context creation values as the
      complete existing direct contract: name `default`, Runtime `standard`,
      guided network mode, read-write source access, enabled native readiness,
      and no Workspace bootstrap.
- [ ] The displayed review and submitted typed values come from one draft
      object and one frozen presentation-independent fixture. Presentation
      cannot diverge from the mutation inputs.
- [ ] The Context create occurs before shared-service and Workspace mutations.
      A validation, review, create, or concurrent-state failure causes no later
      cluster or Workspace side effect.
- [ ] Immediately before creation, Tobari revalidates that the collection is
      still empty and `default` is still absent. A concurrent change returns a
      stable recovery result and never silently adopts or overwrites another
      process's Context.
- [ ] After confirmed creation, the existing root composition prepares shared
      services and enters the current project's Workspace. Bare `tobari` opens
      Bash; `tobari -- COMMAND...` executes the exact argv without a shell and
      returns the child's exact status to the host.
- [ ] For a direct child, the screen names the requested executable using safe
      terminal projection without repeating the remaining argv. It does not
      log, persist, or add arguments.
- [ ] `Customize` enters the existing six-stage wizard in one terminal session
      with `default`, read-write, reviewed network defaults, `standard@1`, and
      no host bootstrap preselected. The user can change every existing field,
      and only the wizard's final Create mutates state.
- [ ] Choosing `Customize` performs no host bootstrap discovery until the user
      explicitly selects the existing Configure-from-host action.
- [ ] `Cancel`, EOF, terminal failure, resize failure, or cancellation before
      Start performs zero mutation and restores the terminal.
- [ ] Redirected input or output, JSON workflows, partial direct
      `context create` input, and other machine paths never receive hidden
      recommended defaults or a prompt.
- [ ] Human help, scoped agent help, README, theses, product contract,
      architecture, capability ledger, harness, and agent-readiness evidence
      distinguish the root first-use composition from standalone Context
      creation.
- [ ] Focused domain, application, CLI, terminal, catalog, concurrency, and
      zero-side-effect tests plus `task check` and `task public:check` pass.

## Governing documents

- Thesis: North Star; Theses 0, 5, 7, and 8
- Product contract section: root `tobari`, interactive first use, Context
  creation, Workspace selection, exact child session, and output boundaries
- Architecture or security invariant: catalog-owned composition, typed
  operation boundaries, no host import without selection, and mutation ordering
- Existing ADR: current Context and root-composition ADRs; no new trust boundary
  is introduced

## Completion definition

The work is complete when the recommended root-only screen and Customize path
are product-shaped and fixture-backed, every displayed value equals the typed
creation input, cancellation and races produce zero later side effects, the
existing standalone and machine contracts remain unchanged, durable docs and
help agree, required gates pass, and this temporary packet is removed.
