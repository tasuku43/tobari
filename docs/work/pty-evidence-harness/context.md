# Work Context: Make human PTY evidence reproducible and safe

## Current behavior

- The four child E2E runs used real PTYs and reported 120x40 plus
  `TERM=xterm-256color`.
- The child reports contain readable projections but no raw-byte digest or
  complete timestamped screen checkpoints.
- Existing focused policy-review tests contain raw PTY evidence, but the blind
  new-user orchestration does not yet force the child-to-parent handoff to
  return it.
- Raw captures may contain ANSI controls, host paths, usernames, opaque IDs,
  and shell prompts; they must stay outside the public repository.

## Relevant structure

- Existing PTY helper: `scripts/test-integration.sh` and policy-review test
  helpers.
- Agent feedback intake: `docs/work/new-user-value-e2e/feedback/`.
- Harness contract: `docs/04_harness.md` and
  `docs/09_agent_readiness_validation.md`.
- Evidence source: `docs/work/new-user-value-e2e/comparison.md`.

## Constraints

- Preserve the distinction between human success and harness-only replay.
- Capture one key/short command at a time and retain redraw timing.
- Redaction must not alter opaque reference values in the evidence before the
  parent records their kind/round-trip role.
- No public artifact may include local absolute paths, credentials, private
  URLs, shell history, or copied private output.

## Unknowns

- [ ] Should capture live in the parent orchestration wrapper, a reusable
      harness helper, or both?
- [ ] What digest/metadata format is stable across macOS and Linux PTYs?
- [ ] Which checkpoint projection is sufficient to prove redraw without
      storing every raw frame publicly?
- [ ] How should a child report a capture location without reading the parent
      repository or being taught the scenario route?
