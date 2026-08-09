# Work Goal: Apply reviewed policy decisions once without restarting OPA

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Project Theses 0, 2, 6, 7, and 8; ADR 0024
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Current policy-learning latency slice
- Related ADRs: ADR 0018 and ADR 0024

## Outcome

A human can explicitly review several exact pending permissions in one terminal
session and apply the complete reviewed decision set through one tested policy
activation. Routine exact policy activation keeps the running OPA container,
confirms the exact active aggregate revision, and retains the existing
fail-closed, all-Context, rollback, and opaque-reference guarantees. One Apply
is bounded to one Context so its durable source promotion remains atomic.

## Why now

The current interactive Permission Inbox applies and activates every exact
decision immediately. One activation starts several short-lived OPA test
containers, recreates the shared OPA container, and waits for health before the
next candidate can be reviewed. A recent local review demonstrated that this
pause is visible in the ordinary denial-to-retry loop even though no Workspace
container is recreated.

## Non-goals

- Do not weaken exact Context/project/HTTP or GraphQL-root binding.
- Do not let redirected or machine-readable review mutate policy.
- Do not replace the existing single-reference `policy allow`, `policy deny`,
  `policy reset`, or `policy compact` machine workflows.
- Do not expose an OPA management port to the host or a Workspace.
- Do not add provider-specific policy meaning, automatic approval, wildcards,
  or a persisted mutable inbox.
- Do not publish a new runtime image or release as part of this implementation.

## Acceptance criteria

- [x] Every staged Allow or Deny comes from an exact validated candidate and is
      visibly retained in the review summary before Apply.
- [x] Cancel before Apply performs zero policy-source writes and zero policy
      activation calls.
- [x] One Apply validates the complete staged set against a fresh retained
      candidate snapshot and performs one aggregate activation.
- [x] Redirected and JSON `policy review` remain read-only; single-reference
      public acts remain compatible.
- [x] Routine successful activation leaves the OPA container identity
      unchanged and confirms the exact expected aggregate revision.
- [x] Authority-reducing activation has a tested deny-all transition or an
      equivalent revision fence so stale Allow authority is not served during
      the switch.
- [x] Invalid candidates, invalid bundles, timeouts, cancellation, concurrent
      source changes, and interrupted activation preserve or restore the prior
      known-good authority and return read-only reconciliation guidance.
- [x] OPA bundle publication is bounded, owner-labeled, secret-free, and
      unreachable from every Workspace.
- [x] The ordinary human denial-to-retry workflow gains no extra external
      processing or command round trip.
- [x] `task check` and `task security` pass.

## Governing documents

- Thesis: `docs/00_theses.md`, especially Theses 0, 2, 6, 7, and 8
- Product contract section: Primary operating loop, Output and exit contract,
  Configuration contract, and Side effects
- Architecture or security invariant: User-facing composition, Gateway request
  flow, policy mutation policy, and atomic all-Context activation
- Existing ADR: ADR 0018

## Completion definition

The work is complete when the acceptance criteria have evidence, ADR 0024 and
the numbered governing documents describe the final behavior, the required
profiles pass, temporary experiments are removed, and this temporary packet is
deleted.
