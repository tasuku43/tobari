# Work Goal: Create a Context from an explicit Base

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, and `docs/04_harness.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Product and implementation agent
- Target: Feedback-derived usability queue
- Related ADRs: ADR 0071

## Outcome

A person creating another Context first chooses one Base, reviews the complete
resulting work-mode draft, optionally customizes it, and creates a new Context
without changing the Base. With no persisted Context, Tobari continues directly
from its recommended settings. With persisted Contexts, interactive creation
presents a dedicated Base step with the current Context initially selected;
`--base <context-name>` selects one exact existing Context and skips that step.

## Why now

Context Boundary immutability is useful only if a mistaken or nearby work mode
can be corrected without rebuilding every setting from memory. The existing
creation flow already reviews one complete draft, but its initial values come
only from Tobari recommendations or individually supplied flags. The accepted
product direction is to preserve that review/customize experience and make the
draft's Base an explicit higher-level decision before individual settings.

## Non-goals

- Mutating, revising, renaming, or deleting the Base Context.
- Persisting inheritance, lineage, or a live relationship between Contexts.
- Copying Workspaces, Workspace homes, tool-owned login state, remembered
  Workspace permissions, Attachment permissions, or current/default selection.
- Requiring `context use` as an indirect way to choose a Base.
- Adding `context revise`, `context clone`, or a second creation command.
- Treating Base as another Project files, Network, Runtime, or Workspace-default
  setting inside the customization sections.

## Acceptance criteria

- [ ] With zero persisted Contexts, interactive `context create` retains the
      recommended-settings path without a redundant Base chooser.
- [ ] With persisted Contexts and no `--base`, interactive creation first shows
      one complete deterministic Base chooser, initially selecting the current
      Context and also offering Tobari recommended settings.
- [ ] `context create --base <context-name>` validates one exact existing
      Context, skips Base selection, and initializes a complete reviewable draft.
- [ ] Base selection precedes name and individual-setting review; the existing
      Create, Customize, and Cancel outcome remains recognizable.
- [ ] The Base supplies the current Boundary, exact Runtime binding, and
      Workspace defaults; explicit creation inputs and interactive customization
      change only the reviewed draft.
- [ ] Creation produces a new Context ID and no persisted parent relationship;
      it does not change the Base, current Context, or any existing Workspace.
- [ ] Base-derived creation copies no Workspace, login, learned-permission, or
      Attachment state.
- [ ] Changing Base after draft edits requires an explicit reset confirmation
      and replaces the complete draft instead of merging hidden overrides.
- [ ] Cancel, invalid Base, stale or invalid draft, and presentation failure
      perform zero Context mutation.
- [ ] Human help, scoped agent help, completion, structured failures, and
      deterministic text/JSON behavior describe `--base` without `--from`.
- [ ] The relevant agent-readiness replay completes with zero undeclared
      external processing.
- [ ] `task check` passes; security/public profiles run if the final contract
      audit determines they are required.

## Governing documents

- Thesis: bounded autonomy must be easier than host execution; first-use owns a
  complete recommended Context draft; Context is a stable reusable work mode.
- Product contract section: Context creation and Context lifecycle.
- Architecture or security invariant: immutable Boundary, independently mutable
  Runtime binding and Workspace defaults, fixed-target Context-catalog creation,
  controlled mutation boundary, and no cross-Workspace authority inheritance.
- Existing ADR: ADR 0071, stable Context work-mode lifecycle.

## Completion definition

The work is complete when acceptance criteria have evidence, durable decisions
have been promoted to numbered documentation or an ADR, required profiles pass,
temporary diagnostics or sensitive artifacts are removed, and this temporary
packet is removed from the final tree.
