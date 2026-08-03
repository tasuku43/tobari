# Work Goal: Restore a visible interactive policy review

- Status: Active
- Retention: temporary
- Retention reason: Reproduce and repair the reported TTY presentation/interaction failure in the denial-to-review loop.
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/09_agent_readiness_validation.md
- Review/delete trigger: Delete after the PTY behavior, machine-readable behavior, opaque-reference flow, and required gates have evidence and durable conclusions are promoted.
- Successor: None
- Owner: CLI/policy maintainers
- Target: `policy review` interactive Permission Inbox
- Related ADRs: None

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

- [ ] A deterministic supported-PTY reproduction uses a synthetic pending candidate and records the exact initial render, key sequence, terminal capabilities, and final visible output.
- [ ] The interactive path visibly renders the queue before input, supports selection/detail/explicit confirmation, and refreshes after one exact allow or deny action.
- [ ] The selected candidate ID is preserved byte-for-byte into the existing reference-bound action; invalid, stale, canceled, or out-of-snapshot choices perform zero policy mutation calls.
- [ ] A user can cancel without an ambiguous blank result; empty queues, cancellation, errors, and successful mutations have distinct reviewed human outcomes.
- [ ] `policy review --format=json` and redirected text remain read-only with the existing schema, bounded collection semantics, and exact action commands.
- [ ] Hostile request fields, opaque IDs, stdout/stderr ownership, terminal restoration, and secret redaction remain covered.
- [ ] The policy-review integration scenario passes on the supported runtime, or an external cluster-start blocker is recorded separately and does not get misclassified as a review result.
- [ ] `task check` and the relevant policy, Gateway, integration, public, and agent-readiness checks pass before handoff.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially Theses 0, 7, and 8
- Product: [Product contract](../../01_product_contract.md), especially policy learning and output behavior
- Architecture/security: [Architecture](../../02_architecture.md) and [Security model](../../03_security_model.md)
- Harness/readiness: [Harness](../../04_harness.md) and [Agent readiness validation](../../09_agent_readiness_validation.md)

## Completion definition

The packet is complete when the observed TTY behavior has a deterministic
explanation and regression test, the smallest safe implementation change is
verified, the existing discover/act and fail-closed contracts remain intact,
and all required evidence and gates are recorded. If the runtime cannot reach
the review scenario, the packet must name that independent blocker and leave
the review behavior unclaimed rather than marking the issue complete.
