# Work Goal: Restore the regular Gateway-image integration path

- Status: Active
- Retention: temporary
- Retention reason: Track a reproducible integration prerequisite until the pinned Gateway image is refreshed or the supported test precondition is repaired.
- Governing contract: docs/01_product_contract.md Gateway image boundary and docs/06_release.md
- Review/delete trigger: Delete after the regular integration path passes with the verified pinned image and the conclusion is promoted to release or harness documentation.
- Successor: None
- Owner: Tobari maintainers
- Target: Pinned Gateway image used by ordinary `cluster up` and integration tests
- Related ADRs: docs/decisions/0017-gateway-source-and-image-boundary.md

## Outcome

The default integration path using the verified pinned Gateway image reaches
the same policy-learning assertions as the contributor local Gateway image
path. The test must continue to reject a stale or incompatible image rather
than silently switching to local development images.

## Current boundary

The previous explicit source-Gateway integration comparison passed the full
current lifecycle, policy, reset, and re-review scenario. The default
`./scripts/test-integration.sh` fails at the initial allowed GET with a Gateway
403 before policy-learning assertions. The same failure reproduces at commit
`4f087fd`, while the Gateway source snapshot check passes. This is a separate
image/release freshness issue, not a regression from `b6327e1`.

## Non-goals

- Do not weaken the default policy or change `/allowed` into an implicit grant.
- Do not make integration tests silently select a local dev image path.
- Do not publish an image or change the pinned digest without the release
  contract and appropriate publication authority.

## Acceptance criteria

- [ ] Identify whether the pinned digest, locally available image, or release
      metadata is older than the current Gateway contract.
- [ ] Repair the regular image path or document and mechanically enforce its
      explicit prerequisite without weakening the test.
- [ ] Run the full default integration scenario, including policy reset and
      re-review, successfully.
- [ ] Run `task check` and the relevant release/public checks.
- [ ] Promote the durable conclusion and remove this temporary packet.
