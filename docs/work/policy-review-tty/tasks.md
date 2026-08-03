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
- [ ] `task integration:test` reaches and passes policy review. Evidence: a clean repository-local run stops before review at non-retryable `cluster_start_failed` (exit 9); the latest rerun stopped even earlier because an existing healthy `tobari-gateway` container failed the integration preflight (exit 1). Neither is success evidence; the active container was left untouched.
- [x] PTY behavior is observed on the supported platform(s), with terminal restoration verified. Evidence: macOS Python-PTY E2E passed allow/deny/cancel/invalid cases and observed `ESC[?25h`; the helper uses the same standard-library PTY model on Linux.
- [x] Opaque-reference and hostile-output regression evidence is recorded. Evidence: E2E markers preserve the synthetic opaque candidate ID through allow/deny; existing hostile-output selector/CLI tests remain in the focused suite.

## Hand off

- [ ] Acceptance criteria have evidence. Fake-runtime real-PTY, negative, JSON, and repository gates are complete; the Docker policy-learning scenario remains externally blocked before review.
- [x] The policy-review contract and any durable decision are promoted. No public contract change was needed; the packet records the mechanical terminal invariant and its regression tests.
- [x] Temporary PTY diagnostics and sensitive artifacts are removed. Only the sanitized durable E2E transcript remains in this packet.
- [ ] The coordinator packet is updated with the final status and successor disposition.
- [ ] This temporary child packet is removed after completion or explicitly superseded.
