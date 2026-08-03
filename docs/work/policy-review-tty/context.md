# Work Context: Restore a visible interactive policy review

This file records verified facts and unresolved questions. It does not treat a
desired TTY experience or an uncompleted integration run as current behavior.

## Main-history reconciliation

- The current checkout is `main` at `ed37f805a4e2876f93c6ad86fb70beb40b6fc073`.
- The policy-review implementation and regression evidence are in
  `7d096bb5749e3ad8afd6d85c88af301f5dda113f`; that commit is an ancestor of
  merge `966dd08841a7ccd88212dd9c8683562c99e17aa9` and therefore of current
  `main`.
- No rebase, alternate index, or read-only Git metadata condition is part of
  the current main checkout. The remaining packet state below is a product/
  runtime integration question, not a branch or staging failure.

## Current behavior

- The public contract defines `policy review` as a read-only discover command that may, on a TTY, compose selection, detail inspection, explicit confirmation, and one exact `policy allow` or `policy deny` action.
- `internal/cli/tobari.go` enters interactive mode only for text output when both input and output are reported as terminals. JSON and redirected text use the read-only renderer.
- `internal/cli/policy_review_selector.go` first attempts raw terminal mode. On Unix it requires the injected input to be an `*os.File` backed by a character device; otherwise it falls back to line input. The raw renderer writes ANSI cursor/screen-control sequences and clears the selector screen when it finishes.
- Raw input semantics are: number selects an item, Enter opens detail, `a` requests allow, `d` requests deny, `y` confirms, and `q` cancels or returns. The integration fixture sends `3dy` because raw mode does not require newline characters. Unix raw mode uses `VMIN=0/VTIME=1`; a zero-byte character-device read is now treated as a polling timeout because Go can surface that read as `io.EOF`.
- Existing focused tests pass: selector raw selection/confirmation/cancellation, line fallback, CLI TTY delegation and queue refresh, redirected read-only behavior, and application terminal gating.
- The Gateway and OPA focused tests pass, including learnable denial navigation and policy candidate behavior.
- The user-reported observation is explained by the raw terminal read path: the initial Inbox rendered, the first zero-byte `VMIN=0/VTIME=1` poll was surfaced as EOF, and the selector converted it to cancellation before a human byte arrived. The validated candidate report was present; this was not a discovery or opaque-reference failure.

## Reproduction or observation

Commands already run in the current worktree:

```sh
go test ./internal/cli ./internal/app/tobaricmd -run 'PolicyReview|PolicyReviewSelector|Interactive' -count=1
task gateway:test
task policy:test
task integration:test
```

The focused Go, Gateway, and OPA checks passed. The real integration stopped
before policy review at shared-cluster startup with `cluster_start_failed` and a
non-retryable recovery instruction. A direct diagnostic using an OS temporary
XDG root first failed because the Docker VM could not access that host path;
using a repository-local synthetic root produced an interrupted cluster journal
before Gateway/OPA became ready. These are runtime/setup observations, not
evidence that the policy-review selector itself failed.

The existing integration fixture reaches the intended review path only after
cluster setup. Its old platform-specific `script` wrapper could close the
child input around the first raw read, so it was replaced with a small
standard-library Python PTY bridge that keeps the child master open and
forwards stdin until the child exits. The policy-learning input now waits for
the first frame before sending `3dy`.

Repeated first-wave integration attempts stopped at shared-cluster startup with
`cluster_start_failed`, before the review section. Those attempts were run
with the repository-local policy path and a task-specific Go cache; the failure
was therefore an independent runtime/setup blocker, not a review assertion.
The current-main recheck is recorded under `Gate evidence` below.

```text
GOCACHE=/private/tmp/tobari-policy-review-gocache task integration:test
error: Cluster startup did not complete; inspect status before retrying
kind: unavailable
code: cluster_start_failed
retryable: false
retry_after: none
next_action: tobari cluster status — Reconcile partial Docker state.
task: Failed to run task "integration:test": exit status 9
```

The first-wave run did not execute the policy-review assertions. It must be
rerun in a healthy Docker/Colima environment before claiming the full
integration path.

A later rerun was blocked even earlier by the integration preflight because an
existing healthy `tobari-gateway` container was still active:

```text
GOCACHE=/private/tmp/tobari-policy-review-gocache task integration:test
integration: container tobari-gateway already exists; stop the active Tobari cluster before integration tests
task: Failed to run task "integration:test": exit status 1
```

The active container was not stopped or removed because it was outside this
packet's authority. The historical `cluster_start_failed` observation remains
separate from the selector result; the current-main result is recorded below.

The durable `TestPolicyReviewRealPTYAndReadOnlyE2E` now exercises the real
selector with the existing fake runtime and a synthetic candidate. Before the
fix, a real PTY with no initial byte reproduced the cancellation path. After
the fix, the test produces these outcomes:

- A Python-allocated PTY that waits one second before sending `1ay` renders the
  Inbox, opens detail, confirms allow, refreshes to an empty queue, and
  completes with `apply_calls=1`. The opaque candidate is preserved unchanged
  in the learned rule.
- The same PTY flow with `1dy` records one exact deny candidate and refreshes
  to an empty queue with `deny_calls=1`.
- `q` and `9q` both finish with `Permission review canceled` and zero allow or
  deny calls. Redirected `--format json` returns the candidate and records zero
  calls.

The raw transcript contains the Inbox, detail, confirmation, exact candidate
round trip, queue refresh, and cursor restoration. See
`e2e/fake-runtime-pty-transcript.txt`. The full Docker integration remains
blocked independently at cluster startup.

## Relevant structure

- Interactive decision orchestration: `internal/cli/tobari.go`, `runPolicyReview`, and `policyReviewInteractiveAllowed`.
- TTY state machine and presentation: `internal/cli/policy_review_selector.go`.
- Terminal capability: `internal/infra/terminal/` and `internal/infra/dockerruntime/runtime.go`.
- Shared raw-byte polling: `internal/cli/workspace_selector.go`.
- Application terminal gate and policy report: `internal/app/tobaricmd/service.go`.
- Focused tests: `internal/cli/policy_review_selector_test.go`, `internal/cli/tobari_test.go`, `internal/cli/policy_review_pty_test.go`, and `internal/app/tobaricmd/service_test.go`.
- Runtime scenario: `scripts/test-integration.sh`, especially the policy-learning section and `run_tobari_pty_at`.
- Public metadata: `internal/cli/runtime_catalog.go` and agent-readiness policy-learning evidence.

## Constraints

- The candidate report is semantic input; the selector cannot infer authority from position or display text.
- Every action consumes exactly one unchanged opaque candidate ID. The selected ID must be checked against the validated snapshot before the side effect.
- Cancellation and invalid input must be safe no-ops; a successful cancellation must not become a retry or approval signal.
- Terminal restoration must run after raw mode, including selection errors and cancellation. Child or redirected output must not receive terminal styling or control sequences.
- External request fields remain untrusted and visibly projected; opaque IDs remain exact.
- The packet must preserve the current policy mutation boundary and the zero-downstream-call guarantees.

## Unknowns

- [x] Does the report contain the candidate at the first interactive call in the reported environment? The synthetic report is present before each fake-runtime PTY case.
- [x] Does the supported PTY wrapper deliver an actual character-device input/output pair to the child process on both macOS and Linux? The Python bridge uses `pty.fork`; the durable fake-runtime test passed on macOS, and the bridge is standard-library Unix code for Linux as well.
- [x] Does raw redraw output hide the first screen in the user's terminal, or does the command receive EOF/cancel before the first visible frame? The supported reproduction identified the zero-byte character-device poll being surfaced as `io.EOF` and converted to cancellation.
- [x] Does the output stream share a terminal with an enclosing shell or host UI that interprets the ANSI cleanup differently? The focused E2E records cursor restoration and the final visible outcomes independently of the enclosing shell.
- [x] Should the final interactive state retain a short readable summary after screen cleanup, or is the existing cancellation/success renderer sufficient once the first frame is visible? The reviewed fake-runtime transcript shows the existing distinct cancellation, mutation, and empty-queue outcomes are sufficient for this fix.
- [ ] Is the cluster-start blocker caused by current uncommitted Gateway image changes, Docker state, or a broader environment prerequisite? The policy-review packet does not claim this is resolved; repeated runs stop at `cluster_start_failed` before review.

## Thesis evidence

- Repeated design decision or point of agent confusion: the typed pending queue can be correct while the human Permission Inbox appears absent.
- User outcome or friction observed in the minimal slice: the human path did not make the next safe action obvious even though machine output contained the candidate.
- Code workaround or exception being considered: avoid adding a second read-only or mutating command; fix the supported presentation/PTY boundary instead.
- Current thesis that resolves it, or proposed thesis revision: Thesis 8 remains authoritative. This packet supplies evidence for improving its mechanical enforcement and human presentation, not for weakening explicit approval.
- Downstream product, architecture, security, Skill, catalog, and harness impact: preserve the existing reference graph and add a clean-PTY regression scenario plus agent-readiness evidence if the observable contract changes.

## Security and public-boundary notes

- The review UI is a host-side presentation over secret-free denial evidence. It must not reveal headers, bodies, credentials, private URLs, or policy internals beyond the existing reviewed fields.
- No new external destination or credential is needed for the focused selector work. The full integration requires Docker and the existing Gateway/OPA test images.
- The cluster-start failure must not be “fixed” by weakening path access, policy validation, or cleanup ownership just to reach the review test.
- Any final CLI presentation change is public contract work and requires `task check` plus `task public:check`; runtime/integration evidence remains separately required.

## New-user E2E follow-up

The parent-owned comparison and the blind official journeys confirm that the
original first-render/EOF fix is not the whole human-path contract. After a
candidate is selected with `a` or `d`, a normal human pause can produce an
`undeclared_fault_contract` before the user reaches `y`. The same pattern was
observed in the parent baseline and in Long-01, Long-02, and Medium-01, so it
is a product defect rather than an isolated child-agent discovery issue. See
`../new-user-value-e2e/comparison.md` and the corresponding redacted feedback
files.

The journeys also reported frequent redraws while the selector was waiting
for input. The follow-up must review the state machine and presentation as one
bounded change: preserve the exact opaque candidate and one-action mutation
boundary, keep cancellation a zero-call no-op, and make the visible final
state distinguish cancellation from an operational fault. It must not add a
retry alias, an in-Workspace policy command, or a second catalog route.

## Gate evidence

- `GOCACHE=/private/tmp/tobari-policy-review-final-gocache go test ./internal/cli -run '^TestPolicyReviewRealPTYAndReadOnlyE2E$' -count=1 -v`: passed all five real-PTY/read-only subcases in 4.13s.
- `GOCACHE=/private/tmp/tobari-policy-review-gocache task check:fast`: passed; `archlint`, `contractlint`, runtime checks, and all Go tests passed.
- `GOCACHE=/private/tmp/tobari-check-final2-gocache task check`: passed; hygiene, architecture, contract, runtime, vet, race, tidy, and Go tests were green.
- `GOCACHE=/private/tmp/tobari-public-final2-gocache task public:check`: passed (`repoguard (public)`, `contractlint`).
- The clean `HEAD + allowed packet diff` security snapshot passed `task security` with `repoguard (security): OK`, all modules verified, and `No vulnerabilities found.` The current worktree security invocation is separately blocked by the out-of-scope untracked `docs/work/architecture-publication/context.md:57` link; this packet does not edit it.
- Current-main `GOCACHE=/private/tmp/tobari-integration-final-gocache task integration:test` stopped at the preflight because `tobari-gateway` already exists and is running; it exited 1 before a clean review assertion. The active cluster was not stopped by this packet.

## Glossary

- **Raw mode:** byte-at-a-time terminal input with echo and canonical processing disabled.
- **Line fallback:** newline-delimited review input used when raw terminal mode is unavailable.
- **Visible final state:** the human-readable output remaining after selector cleanup, distinct from the raw ANSI transcript.
- **Review snapshot:** the validated candidate report used to check the opaque ID before an action.
