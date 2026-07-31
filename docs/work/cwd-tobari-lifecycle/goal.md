# Work Goal: CWD-owned Tobari lifecycle

- Status: Active
- Retention: temporary
- Retention reason: Implementation evidence for the lifecycle replacement
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md
- Review/delete trigger: Delete after durable lifecycle decisions and verification evidence are promoted
- Successor: None
- Owner: Codex
- Target: CWD-owned lifecycle replacement
- Related ADRs: None

## Outcome

A developer runs `tobari` from a project directory to create or reuse the one
logical Tobari covering that directory, reconcile its runtime, and enter it.
The user manages only the current directory: a Tobari either exists or does not
exist, and `tobari delete` is the only command that ends its lifecycle.

## Why now

The existing named attach/detach workflow exposes container-oriented identity
and lifecycle details that the requested project-directory workflow must own.

## Non-goals

- Automatically identify project moves or copies.
- Automatically migrate legacy named state.
- Add non-interactive execution; a future `run` command owns that outcome.
- Relax the shared Gateway, OPA, credential, or host-mount boundaries.

## Acceptance criteria

- [ ] `tobari` resolves the nearest canonical CWD-rooted logical Tobari, creates it when absent, reconciles its runtime, and enters it only on a TTY.
- [ ] `status`, `list`, and `delete` expose the CWD-owned lifecycle without a name, root argument, or user-supplied ID.
- [ ] Logical identity, root index, and per-instance home survive work-container loss and are atomically persisted under XDG state.
- [ ] `delete` is the sole lifecycle-ending command and removes only selected owned runtime and instance data.
- [ ] Shared read-only agent profiles and isolated per-Tobari mutable state are mounted without sharing credentials or histories.
- [ ] The named command surface is explicitly retired, durable contracts are updated, and the required checks pass.

## Governing documents

- Thesis: `docs/00_theses.md`
- Product contract section: public commands, input/path, configuration, and side effects
- Architecture or security invariant: four-layer runtime boundary and exact ownership labels
- Existing ADR: None

## Completion definition

The work is complete when the acceptance criteria have evidence, durable
decisions are promoted to the numbered contracts, required checks pass, and
this temporary packet is removed.
