# Work Tasks: Restore a visible interactive policy review

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read the governing thesis, product, architecture, security, harness, and readiness sections.
- [x] Inspect the policy-review orchestration, terminal capability, selector, catalog, focused tests, and integration helper.
- [x] Record the user observation and distinguish it from the current verified unit behavior.
- [x] Run focused selector/CLI/application tests. Evidence: `go test ./internal/cli ./internal/app/tobaricmd -run 'PolicyReview|PolicyReviewSelector|Interactive' -count=1` passed.
- [x] Run Gateway and OPA focused tests. Evidence: `task gateway:test` passed 25 tests; `task policy:test` passed 27/27 Rego tests.
- [x] Reproduce the interactive behavior with a clean PTY and a synthetic pending candidate. Evidence: the durable fake-runtime PTY E2E reproduced the pre-fix zero-byte-read cancellation and passes after the fix with delayed input.
- [x] Record the raw control-byte transcript, rendered queue, input sequence, and final visible output. Evidence: [fake-runtime PTY transcript](e2e/fake-runtime-pty-transcript.txt) records `1ay`, `1dy`, `q`, `9q`, the exact candidate round trips, queue refresh, cursor restoration, and redirected JSON.

## Decide

- [x] Classify the current evidence as an early-input/EOF race candidate; TTY detection and selector semantics work in the delayed real-PTY probe, while the user's exact terminal wrapper remains unverified.
- [x] Decide whether the public empty/canceled human states need a contract change. No change is needed; the defect was raw polling, and cancellation remains an explicit safe no-op.
- [x] Add an ADR only if the supported terminal model or public interaction boundary changes. No ADR is needed; the supported Unix PTY boundary is repaired without changing the public command or catalog.
- [x] Confirm that no new command, provider adapter, or opaque-reference shortcut is needed. The existing `policy review` to `policy allow`/`policy deny` flow remains authoritative.

## Implement

- [x] Add the smallest failing regression test at the observed failure boundary. Evidence: `TestReadSelectorByteTreatsCharDevicePollAsTimeout` covers the Go `io.EOF` mapping, and `TestPolicyReviewRealPTYAndReadOnlyE2E` covers the real PTY boundary.
- [x] Implement the minimal CLI/terminal fix, if the reproduction requires one. `readSelectorByte` treats zero-byte EOF from a character device as polling timeout; the integration helper now owns a direct Python PTY bridge.
- [x] Preserve redirected and JSON read-only output, exact ID round trips, and zero mutation on cancellation. The E2E covers allow, deny, cancel, invalid selection, and redirected JSON.
- [x] Update agent-readiness evidence if the supported human path changes. No catalog or invocation contract changed; the existing denial-to-review-to-exact-action scenario remains valid, while the packet adds the missing real-PTY evidence.

## Verify

- [x] `task gateway:test` passes. Evidence: focused Gateway tests passed.
- [x] `task policy:test` passes. Evidence: all 27 policy tests passed.
- [x] Focused CLI/application tests pass after the reproduction/fix. Evidence: `GOCACHE=/private/tmp/tobari-policy-review-gocache go test ./internal/cli ./internal/app/tobaricmd -run 'PolicyReview|PolicyReviewSelector|Interactive|ReadSelectorByte' -count=1` passed; the real-PTY E2E ran five subcases.
- [x] `task check` passes. Evidence: `GOCACHE=/private/tmp/tobari-policy-review-gocache task check` passed.
- [x] `task public:check` passes when public docs or contract text changes. Evidence: `GOCACHE=/private/tmp/tobari-policy-review-gocache task public:check` passed; `bash -n scripts/test-integration.sh` and `git diff --check` also passed.
- [ ] `task integration:test` reaches and passes policy review. Evidence: the
      follow-up run reached `/review-interactive` and
      `scripts/test-integration.sh:744`, where the checked-in PTY wrapper and
      `tobari policy review --tail 1000` child waited without output or
      assertion progress for over four minutes. Ctrl-C produced exit 130 and
      cleanup left no integration containers. The review assertion was not
      reached; this is an external harness blocker, separate from the passing
      focused PTY suite.
- [x] PTY behavior is observed on the supported platform(s), with terminal restoration verified. Evidence: macOS Python-PTY E2E passed allow/deny/cancel/invalid cases and observed `ESC[?25h`; the helper uses the same standard-library PTY model on Linux.
- [x] Opaque-reference and hostile-output regression evidence is recorded. Evidence: E2E markers preserve the synthetic opaque candidate ID through allow/deny; existing hostile-output selector/CLI tests remain in the focused suite.

## Hand off

- [ ] Acceptance criteria have evidence. Fake-runtime real-PTY, negative, JSON,
      focused/full/public/security gates, and the delayed-confirmation follow-up
      are complete; the Docker policy-learning scenario is externally blocked
      at the checked-in PTY review wrapper before its first assertion.
- [x] The policy-review contract and any durable decision are promoted. No public contract change was needed; the packet records the mechanical terminal invariant and its regression tests.
- [x] Temporary PTY diagnostics and sensitive artifacts are removed. Only the sanitized durable E2E transcript remains in this packet.
- [ ] The coordinator packet is updated with the final status and successor disposition.
- [ ] This temporary child packet is removed after completion or explicitly superseded.

## New-user E2E follow-up

- [x] Reproduce the `a`/`d` confirmation failure after a normal human pause in
      a real PTY and record the exact state transition and side-effect count.
      Evidence: `1a` followed by a one-second pause returned
      `undeclared_fault_contract`, exit 13, with zero mutation calls; the
      follow-up packet transcript records the repeated redraws.
- [x] Implement the smallest selector/redraw fix without adding a command or
      changing the JSON/reference contract. Evidence: timeout-only reads now
      wait in place; list/detail render only on state changes; confirmation
      remains pending until an explicit byte arrives.
- [x] Add delayed-input, explicit-cancel, interrupt, redraw, and zero-call
      negative-path regressions. Evidence: the 120x40 PTY test covers delayed
      allow/deny, cancel, Ctrl-C, exact IDs, cursor restoration, and three
      stable frames; existing invalid, hostile, stale, wrong-kind, JSON, and
      zero-call tests remain green.
- [x] Replay the fresh 120x40 PTY journey and the policy-learning integration
      scenario; record any external runtime blocker separately. Evidence: the
      fresh fake-runtime PTY journey passes all four staged cases; the Docker
      integration attempt remains a separate pre-review startup/preflight
      blocker and is not counted as product success.
- [x] Run focused tests, `task check`, `task public:check`, security, and the
      relevant readiness checks. Evidence: focused CLI/application, hostile,
      opaque/invalid, Gateway, and OPA tests passed; `task check:fast`, `task
      check`, `task public:check`, and `task security` passed. `task
      integration:test` was attempted through the supported readiness path and
      is recorded as the separate wrapper/input wait blocker above.
- [ ] Commit the follow-up with an intentional message and report the SHA;
      update the triage/comparison handoff before closing this packet.
