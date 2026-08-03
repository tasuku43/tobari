# Work Goal: Restore a visible interactive policy review

- Status: Complete
- Retention: temporary
- Retention reason: Reproduce and repair the reported TTY presentation/interaction failure in the denial-to-review loop.
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/09_agent_readiness_validation.md
- Review/delete trigger: Delete after the PTY behavior, machine-readable behavior, opaque-reference flow, and required gates have evidence and durable conclusions are promoted.
- Successor: None
- Owner: CLI/policy maintainers
- Target: `policy review` interactive Permission Inbox
- Related ADRs: None

History note: the implementation and regression evidence were committed in
`7d096bb5749e3ad8afd6d85c88af301f5dda113f`, merged by
`966dd08841a7ccd88212dd9c8683562c99e17aa9`, and the supported-runtime PTY
completion fix is in `c957401` on current `main`.

## Outcome

When a retained learnable denial exists, a developer running `tobari policy
review` on a supported TTY can see the Permission Inbox, inspect one exact
candidate, and explicitly allow or deny it. The action delegates the unchanged
opaque candidate ID and refreshes the queue. Cancellation, redirected output,
and JSON output remain read-only and understandable, and the user is not left
with a blank screen that looks like a missing review queue.

## Why now

The reported workflow showed a pending candidate in
`policy review --format=json`, but the default interactive command appeared to
show nothing and ended with `Permission review canceled`. This is directly on
Tobari's primary denial-to-retry adoption path. Existing selector and policy
tests pass, so the first required step is to distinguish a real selector bug
from a terminal, PTY wrapper, candidate-snapshot, or screen-redraw mismatch.

## Non-goals

- Do not weaken default-deny policy or add an approval shortcut.
- Do not make a display position, command text, or decoded identifier an action authority.
- Do not change the redirected or machine-readable review contract into a mutating path.
- Do not add a new policy-management command or provider-specific adapter.
- Do not repair unrelated `cluster up`, Gateway image, or runtime-image work in this packet.
- Do not change the detached `codex/auth-broker` branch.

## Acceptance criteria

- [x] A deterministic supported-PTY reproduction uses a synthetic pending candidate and records the exact initial render, key sequence, terminal capabilities, and final visible output. Evidence: the fake-runtime PTY transcript records the initial Inbox, raw sequences, candidate, and cursor restoration.
- [x] The interactive path visibly renders the queue before input, supports selection/detail/explicit confirmation, and refreshes after one exact allow or deny action. Evidence: the allow and deny subcases render the Inbox, detail/confirmation state, and an empty refreshed queue.
- [x] The selected candidate ID is preserved byte-for-byte into the existing reference-bound action; invalid, stale, canceled, or out-of-snapshot choices perform zero policy mutation calls. Evidence: the focused PTY E2E and selector tests cover exact ID round trips and zero-call negative cases.
- [x] A user can cancel without an ambiguous blank result; empty queues, cancellation, errors, and successful mutations have distinct reviewed human outcomes. Evidence: `q`, `9q`, allow, deny, and empty-queue outputs are distinct in the focused E2E.
- [x] `policy review --format=json` and redirected text remain read-only with the existing schema, bounded collection semantics, and exact action commands. Evidence: the redirected JSON subcase returns the candidate with zero mutation calls, and focused CLI tests preserve the existing projection.
- [x] Hostile request fields, opaque IDs, stdout/stderr ownership, terminal restoration, and secret redaction remain covered. Evidence: existing hostile-output/CLI tests and the PTY transcript preserve these boundaries, including `ESC[?25h` restoration.
- [x] The policy-review integration scenario passes on the supported runtime, or an external cluster-start blocker is recorded separately and does not get misclassified as a review result. Evidence: `task integration:test` completed with `integration: OK` on current `main`; the real Gateway/OPA/mock path entered the review queue, denied the selected exact `/review-interactive` candidate, returned with `q`, and completed cleanup.
- [x] `task check` and the relevant policy, Gateway, integration, public, and agent-readiness checks pass before handoff. Evidence: focused policy/Gateway/PTY checks, `task check`, `task security`, `task public:check`, and `task integration:test` pass after `c957401`; the agent-readiness packet consumes the same denial-to-review contract.

## New-user E2E follow-up

The blind new-user journeys found a second interaction defect beyond the
original first-render/EOF fix. The implementation must keep the confirmation
state alive across a normal human pause after `a` or `d`, and must make the
next safe action and final outcome readable without a repeated redraw storm.

- [x] A human-length pause after `a` or `d` preserves the confirmation state
      and performs exactly one opaque action only after `y`, explicit cancel,
      or terminal interruption. Evidence: 120x40 staged PTY allow/deny cases
      waited 0.75 seconds before `y`; cancel/interrupt cases performed zero
      policy calls.
- [x] The raw PTY transcript has stable, readable redraw behavior without a
      repeated warning storm, and terminal state is restored on every exit.
      Evidence: delayed allow/deny each rendered exactly three frames and
      every staged case restored `ESC[?25h`.
- [x] Intentional cancellation is visually distinguishable from an
      operational failure wherever the reviewed outcome needs that distinction.
      Evidence: q/Ctrl-C finish with `Permission review canceled` and
      `No permissions changed.` while the pre-fix timeout produced the distinct
      `undeclared_fault_contract` failure.
- [x] A fresh supported-runtime PTY E2E and the policy readiness scenario pass
      after the follow-up, or the remaining external blocker is recorded
      separately from the product result. Evidence: the fresh fake-runtime
      supported PTY suite passes all four cases, and the Docker readiness
      scenario now reaches the review wrapper, denies the exact candidate,
      returns with `q`, and finishes with `integration: OK`.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially Theses 0, 7, and 8
- Product: [Product contract](../../01_product_contract.md), especially policy learning and output behavior
- Architecture/security: [Architecture](../../02_architecture.md) and [Security model](../../03_security_model.md)
- Harness/readiness: [Harness](../../04_harness.md) and [Agent readiness validation](../../09_agent_readiness_validation.md)

## Completion definition

The packet is complete because the observed TTY behavior has a deterministic
explanation and regression test, the smallest safe implementation and
integration-harness changes are verified, the existing discover/act and
fail-closed contracts remain intact, and the required evidence and gates are
recorded. The supported runtime integration now passes; the packet has no
remaining product or environment blocker.
