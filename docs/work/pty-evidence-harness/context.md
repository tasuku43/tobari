# Work Context: Make human PTY evidence reproducible and safe

## Current behavior

- The four child E2E runs used real PTYs and reported 120x40 plus
  `TERM=xterm-256color`.
- The child reports contain readable projections but no raw-byte digest or
  complete timestamped screen checkpoints.
- Existing focused policy-review tests contain raw PTY evidence. The blind
  new-user orchestration deliberately keeps capture parent-owned so the child
  can remain unaware of the route and still be evaluated as a first-time user.
- The parent capture boundary is now implemented in
  `scripts/pty-evidence.py`. It refuses repository-local output, sets the PTY
  size and `TERM`, records delayed input/checkpoints and exit status, and emits
  raw/redacted SHA-256 metadata. Its contract replay passes. A parent-owned
  capture around `task integration:test` completed on 2026-08-04 with exit 0,
  120x40, `TERM=xterm-256color`, `spawn`/`exit` checkpoints, and elapsed
  `74354 ms`; the external artifact and both digests are recorded in the
  successor feedback packet.
- The 2026-08-04 blind Medium-02 rerun independently completed the lifecycle
  outcome through a real 80x24 PTY (`TERM=dumb`) without being given the
  capture route. It returned functional observations but no child-owned raw
  bundle. This is expected from the parent-owned boundary and is recorded
  explicitly rather than backfilled.
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

- [x] Choose whether capture lives in the parent orchestration wrapper, a
      reusable harness helper, or both. Decision: the reusable helper is
      parent-owned; the child remains unaware of the route and raw artifact
      location.
- [x] What digest/metadata format is stable across macOS and Linux PTYs?
      Decision: schema-versioned JSON with terminal metadata, monotonic
      elapsed milliseconds, offsets, prefix digests, and SHA-256 files.
- [x] Which checkpoint projection is sufficient to prove redraw without
      storing every raw frame publicly? Decision: ANSI-preserving redacted
      output plus checkpoint offsets, prefix digests, and visible tails; raw
      bytes remain external.
- [x] How should a child report a capture location without reading the parent
      repository or being taught the scenario route? Decision: the child does
      not report a location; the parent orchestration owns the external bundle
      and records its path/digests alongside the blind functional feedback.

## Current E2E evidence

- Blind outcome-only rerun: [Medium-02 feedback](../new-user-value-e2e/feedback/official/medium-02-blind-rerun-20260804.md).
- Parent capture metadata: external
  `/private/tmp/tobari-pty-integration-20260804/capture/metadata.json`.
- Raw SHA-256: `db2b7f75ae62ab98e3ef56853267ec1dcc00ffaaaaa89bc07a86223c7ead257b`.
- Redacted SHA-256: `0dc2b769ab9092f848b638348d1de6d39f49f596930b2f312e8a4c1c1fbf8d72`.
- Repository contract: `python3 scripts/test-pty-evidence.py` passed after
  asserting that the scheduled PTY event is delivered exactly once.
