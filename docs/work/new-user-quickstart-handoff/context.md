# Work Context: Make the first-use host handoff executable

This file records verified evidence and dependencies; it does not replace the
product or catalog contract.

## Current behavior

- `policy_denied` visibly names host-side `tobari policy review` and states that
  retry is not automatic.
- Long-01 saw a visible `tobari retry` next action that was not an executable
  public command; the child recovered through `Resume: tobari`.
- Long-01 and Medium-01 explored `tobari` inside the Workspace before
  discovering that review/recovery belongs on the host.
- Runtime preparation/build and cleanup are separate host-owned boundaries.
- `context show` required the explicit `--name=default` form during discovery.
- The first disposable `/tmp` environment hit Docker VM sharing/image setup
  failures; a host-shared disposable path worked. This is setup guidance, not
  permission to weaken sharing checks.
- The completed `quickstart-runtime-docs` packet already documents a core
  denial/runtime journey; this packet is a successor follow-up driven by blind
  E2E evidence and must preserve existing public contracts.

## Relevant structure

- Public journey: `README.md` and numbered `docs/` contracts.
- Catalog/recovery source: `internal/cli/runtime_catalog.go` and CLI help.
- Policy denial/review output: `internal/cli/` and `internal/app/tobaricmd/`.
- Runtime documentation dependency:
  [runtime-lifecycle-reconcile](../runtime-lifecycle-reconcile/goal.md).
- Evidence source:
  [new-user E2E comparison](../new-user-value-e2e/comparison.md).
- Prior docs packet: `docs/work/quickstart-runtime-docs/`.

## Constraints

- Public docs must name exact catalog paths and preserve host/Workspace trust
  ownership.
- Do not add or remove a command from documentation until catalog and recovery
  compatibility are checked.
- Use `example.com`/`example.org` synthetic values only; no private URLs,
  credentials, local paths, or shell history.
- Documentation changes require public-boundary and full repository checks.

## Unknowns

- [ ] Which valid host re-entry wording should replace the stale `tobari retry`
      message after the policy packet settles its output contract?
- [ ] Which runtime lifecycle decision should the Quick Start state?
- [ ] Should the Docker VM sharing prerequisite be a README prerequisite or a
      troubleshooting-only note?
- [ ] Which architecture/publication artifacts should remain separate from the
      first-use Quick Start?
