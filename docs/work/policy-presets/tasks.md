# Work Tasks: Create Contexts from bounded policy presets

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Inspect current domain policy source, Context initialization, guided and
      Advanced evaluation, candidate learnability, compaction, aggregate
      activation, and policy catalog.
- [x] Record implicit example-policy grants and absent preset identity.
- [ ] Observe representative retained agent traffic and denial volume without
      capturing credentials or request bodies.

## Decide

- [x] Select immutable Context snapshots instead of live references.
- [x] Separate guardrail, Context-wide baseline grants, baseline denies, and
      project-bound learned exact rules.
- [x] Select the three built-in identities and exact GET-only reviewed meaning.
- [x] Require the guardrail above guided, learned, and Advanced allows.
- [x] Freeze custom schema V1 and report field names after hostile-fixture
      review. Evidence: `PolicyPresetMethodCeiling` and
      `PolicyPresetDestinationCeiling` make all/public-HTTPS versus exact
      collection meaning explicit; task-owned outputs carry source, scope,
      limitations, ceiling modes/counts, and normalized revision.
- [x] Add and accept the durable policy-preset ADR. Evidence: accepted ADR 0029
      owns immutable snapshot and system-guardrail precedence.

## Retire compaction prerequisite

- [ ] Remove `policy compactions`, `policy compact`, `policy-compaction`
      references, prefix learned-rule variants, source fields, faults,
      recoveries, tests, and dormant fallback.
- [ ] Complete the applicable parent `capability-retirement.md` evidence.
- [ ] Prove retained exact candidate/allow/deny/reset and batch review behavior.

## Implement domain and store

- [x] Add failing selector, preset, ceiling, baseline, endpoint, normalization,
      digest, and report tests.
- [x] Implement the shared strict validator used by embedded and custom presets.
- [x] Add immutable normalized built-ins with exact revisions.
- [x] Add owner-only custom preset paths, strict reader, atomic non-overwriting
      init, symlink/mode/race/size guards, and synthetic fixtures.
- [x] Add Context snapshot identity/content and exact-V1 persistence.

## Implement application and CLI

- [ ] Add `policy.presets` to the public capability ledger.
- [x] Add list/show/init/validate use cases with task-owned minimal ports.
- [x] Register complete catalog specs, dispatch, help, presentation, fixtures,
      faults, and generated architecture data.
- [x] Add `--policy-preset` to Context creation with
      `builtin/reviewed-exact` default.
- [x] Add typed Context list/show preset and effective-policy facts.
- [x] Prove invalid inputs and read commands perform zero writes/activation.

## Implement enforcement

- [x] Generate initial Context domain policy only from the normalized snapshot;
      stop copying test/example domains implicitly.
- [x] Project the guardrail into the complete aggregate.
- [x] Enforce guardrail and deny precedence before baseline, learned, and
      Advanced allow paths.
- [x] Make guardrail rejection terminal/non-learnable with stable secret-free
      reason and no review recovery.
- [ ] Preserve exact project-bound learned rule identity and manual retry only.
- [ ] Preserve aggregate preflight, atomic promotion, revision confirmation,
      reduction fencing, and fail-closed rollback.

## Verify

- [ ] Every built-in behavior matrix passes.
- [ ] Custom grant/deny/endpoint ceiling and hostile parser tests pass.
- [ ] Learned and Advanced bypass canaries pass.
- [ ] Terminal denial has zero candidate, DNS, Broker, and upstream calls.
- [ ] Preset source update/delete does not affect existing Context snapshots.
- [ ] Same preset produces deterministic normalized bytes/revision.
- [ ] Cloud-agent offline/GET-only failures are observed and documented with no
      bypass.
- [ ] Agent preset discovery/selection needs no source inspection or external
      processing.
- [x] Focused tests pass. Evidence: `go test ./internal/domain/tobari
      ./internal/app/contextcmd ./internal/app/policypresetcmd
      ./internal/infra/dockerruntime` and `go test ./internal/cli` pass locally;
      Gateway source compiles with `python3 -m py_compile`, while its unittest
      suite requires the integration profile's mitmproxy dependency.
- [ ] `task check` passes. Evidence:
- [ ] `task check:fast` passes. Evidence: blocked before repository checks by
      host Node.js v22.14.0/npm 10.9.2; repository requires Node.js v24.18.0
      and npm 11.16.0. Focused Go suites pass.
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:

## Hand off

- [ ] Acceptance criteria and parent Context/release packets agree.
- [ ] Documentation never labels GET safe/read-only or a reviewed effect
      automatically allowed.
- [ ] Durable conclusions are promoted and this temporary packet is removed.
