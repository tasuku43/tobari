# Work Goal: Make routine CLI output result-first and task-specific

- Status: Accepted
- Retention: temporary
- Retention reason: None
- Governing contract: Project theses, product contract, CLI catalog, semantic presentation contracts, and harness
- Review/delete trigger: Delete after the presentation decision is promoted, fixture-verified, and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Before one-screen onboarding, Context derivation, status aggregation, and service exposure
- Related ADRs: ADR 0029, ADR 0051, ADR 0058, ADR 0060, ADR 0062, ADR 0066, ADR 0067, and ADR 0068
- Depends on: `../public-workspace-vocabulary/`,
  [ADR 0071](../../decisions/0071-define-context-as-a-stable-work-mode.md), and
  `../authority-scope-lifetime-model/`

## Outcome

Routine human output answers the user's current question with effective Access,
Tools, Workspace defaults, state, and exact next action. Internal identities,
revisions, stores, images, agent profile, native-readiness terminology, and
shared-service components recede to `--details`, JSON, or their specialist
commands. No global beginner/expert preference is added; the existing command,
`--details`, and specialist-command layers provide progressive disclosure.

## Why now

Current `context list` repeats home/tmpfs writability, image selectors, and
`agent default` for every Context. Ordinary `context show` exposes policy hash,
native readiness, profile, and image. `status` exposes diagnostic ID and home
path even when no action is required. The information is accurate but presents
implementation sources before the user-visible result. Upcoming onboarding and
status work would otherwise copy this hierarchy.

## Non-goals

- Adding or changing authority, Context mutation, Workspace lifecycle,
  authentication ownership, or shared-service behavior.
- Removing complete JSON facts required by agents or diagnostics.
- Adding a global novice/expert mode, persisted display preference, or alternate
  command registry.
- Implementing pending-permission aggregation, exposed-port aggregation, or
  `tobari expose`; those require separate task-owned results and packets.
- Hiding an action-required state merely because its underlying field is
  technical.
- Letting renderers infer routine-traffic readiness, identity, relationships,
  health, completeness, or next action from labels or field proximity.
- Redesigning the interactive Context creation wizard in this packet.

## Acceptance criteria

- [ ] Ordinary `context list` answers which reusable work modes exist and shows
      only name/current selection, effective Access summary, exact Runtime
      selection, and an action-required marker when applicable.
- [ ] Ordinary `context show` groups effective Access, Tools, Workspace
      defaults, Workspace-owned login result, Details, and exact Next without
      exposing hashes, IDs, image selectors, stores, agent profile, or native
      readiness as routine nouns.
- [ ] `context show --details` retains a complete grouped technical view, and
      JSON remains complete and schema-versioned.
- [ ] Human synthetic-default presentation says recommended defaults are not
      saved; machine output retains the explicit `synthetic_default` state and
      null identity distinctions.
- [ ] Native readiness is projected through a finite typed effective routine-
      traffic state that accounts for Context method/destination ceilings;
      presentation never equates enabled with fully allowed by inspection.
- [ ] Ordinary `status` prioritizes Workspace state, root, Context, Runtime,
      session, action-required conditions, Details, and exact Next; healthy
      diagnostic IDs, home paths, and revisions move to a detailed surface.
- [ ] Bootstrap or reconciliation facts appear in ordinary status only when the
      typed state requires user attention; omission never invents a known zero
      for not-yet-supported pending/exposure aggregates.
- [ ] Ordinary onboarding and root human help describe the backing control
      plane as Shared services, while exact `cluster` commands and specialist
      status continue to expose Gateway, OPA, and projection diagnostics.
- [ ] Agent profile is absent from routine human output and retained only where
      the accepted public-machine-field audit says it remains contractual.
- [ ] Frozen typed fixtures and answer keys prove identical task identity,
      scope, exact next argv, canonical references, state distinctions, and
      recovery before and after presentation changes.
- [ ] `task check` passes; additional profiles run only if implementation
      crosses their declared boundary.

## Governing documents

- Thesis: North Star adoption promise, Context composition, semantics-before-
  presentation, and public catalog consequences in `docs/00_theses.md`
- Product contract section: Context list/show, status, root help, human text,
  semantic tokens, and structured-output contracts
- Architecture or security invariant: task-owned validated results precede
  presentation; visible labels and omission do not create authority or meaning
- Existing ADR: ADR 0060 Context management UX, ADR 0067 Runtime selection,
  ADR 0068 structured Workspace output

## Completion definition

The work is complete when the accepted result-first/task-specific hierarchy is
promoted, every changed presentation is fixture-backed and semantically
eligible, complete detailed/machine surfaces remain available, required gates
pass, and this temporary packet is removed.
