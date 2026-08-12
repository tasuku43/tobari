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

- [x] Remove `policy compactions`, `policy compact`, `policy-compaction`
      references, prefix learned-rule variants, source fields, faults,
      recoveries, tests, and dormant fallback. Evidence: the prerequisite
      retirement commits and hostile old-state tests were integrated before
      preset feature commits; the integration journey rejects both command
      paths and proves three exact rules do not grant an unseen sibling.
- [x] Complete the applicable parent `capability-retirement.md` evidence.
      Evidence: the retirement packet inventories command, reference, fault,
      recovery, state, OPA, production seeds, and dormant fallback and records
      integration verification.
- [x] Prove retained exact candidate/allow/deny/reset and batch review behavior.
      Evidence: focused policy/CLI tests and the integration journey retain
      candidate-ID mutation, interactive staged review, same-session manual
      retry, exact denial, rule inventory, and reset.

## Implement domain and store

- [x] Add failing selector, preset, ceiling, baseline, endpoint, normalization,
      digest, and report tests.
- [x] Implement the shared strict validator used by embedded and custom presets.
- [x] Add immutable normalized built-ins with exact revisions.
- [x] Add owner-only custom preset paths, strict reader, atomic non-overwriting
      init, symlink/mode/race/size guards, and synthetic fixtures.
- [x] Add Context snapshot identity/content and exact-V1 persistence.

## Implement application and CLI

- [x] Add `policy.presets` to the public capability ledger. Evidence:
      `.harness/capabilities.json` declares the exact catalog capability and
      the product plus EN/JA schema tables declare `policy_presets` version 1.
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
- [x] Preserve exact project-bound learned rule identity and manual retry only.
      Evidence: scheme-aware candidate identity tests, project-bound mutation
      tests, catalog reference flow, and integration review steps pass opaque
      candidate IDs unchanged and never auto-retry the denied request.
- [x] Preserve aggregate preflight, atomic promotion, revision confirmation,
      reduction fencing, and fail-closed rollback. Evidence:
      `policydata_test.go` covers complete aggregate preflight, concurrent
      changes, activation receipts, deterministic projection, transaction
      recovery, and rollback after invalid or externally changed source.

## Verify

- [x] Every built-in behavior matrix passes. Evidence: domain tests prove the
      three stable zero-grant built-ins; aggregate/pinned OPA tests prove
      offline, reviewed-exact HTTPS, and GET-only terminal precedence.
- [x] Custom grant/deny/endpoint ceiling and hostile parser tests pass.
      Evidence: domain/store tests reject out-of-ceiling rules, duplicate and
      unknown fields, executable-shaped data, IP/reserved destinations,
      unsafe modes, symlinks, and ambiguous sources.
- [x] Learned and Advanced bypass canaries pass. Evidence: aggregate-router
      tests prove `builtin/reviewed-exact` and `builtin/get-only-reviewed`
      reject plain HTTP at the system guardrail before an Advanced policy can
      run; pinned OPA tests prove a learned exact rule with no explicit scheme
      cannot validate or authorize a matching request.
- [x] Terminal denial has zero candidate, DNS, Broker, and upstream calls.
      Evidence: pinned OPA terminal decisions are non-learnable; Gateway tests
      prove terminal denial performs only non-secret handle introspection, no
      resolution or upstream commit, while guardrail projection precedes all
      policy evaluators. The integration journey additionally checks absence
      of candidates/upstream when Docker is available.
- [x] Preset source update/delete does not affect existing Context snapshots.
      Evidence: `TestContextSnapshotsPresetAndIgnoresLaterSourceEdit` passes;
      the integration journey repeats the revision observation against public
      commands.
- [x] Same preset produces deterministic normalized bytes/revision. Evidence:
      domain normalization and stable built-in revision tests plus deterministic
      aggregate projection tests pass.
- [ ] Cloud-agent offline/GET-only failures are observed and documented with no
      bypass.
- [x] Agent preset discovery/selection needs no source inspection or external
      processing. Evidence: all four commands and Context selection derive
      complete input/output/fault contracts from the catalog; regenerated
      scoped help and the readiness journey consume declared fields directly.
- [x] Focused tests pass. Evidence: `go test ./internal/domain/tobari
      ./internal/app/contextcmd ./internal/app/policypresetcmd
      ./internal/infra/dockerruntime` and `go test ./internal/cli` pass locally;
      `go test ./internal/infra/runtimeassets` also passes. Pinned OPA format
      validation passes and the policy modules plus `data.json` pass 53/53
      tests, including the missing-scheme canary. The `task policy:test`
      wrapper cannot bind this `/tmp` lane into the local Docker daemon; its
      whole-directory form also encounters the pre-existing domain allow/deny
      JSON merge errors. Gateway source compiles with `python3 -m py_compile`,
      while its unittest suite requires the integration profile's mitmproxy
      dependency.
- [x] `task check` passes. Evidence: `mise exec -- task check` passed on
      integrated V1 HEAD on 2026-08-12, including all Go tests with race,
      generated/catalog/source checks, both site builds, and Playwright 40/40.
- [x] `task check:fast` passes. Evidence: `mise exec -- task check:fast`
      passed after preset/auth/policy/site integration on 2026-08-12.
- [x] `task security` passes. Evidence: `mise exec -- task security` passed on
      the same integration branch.
- [ ] `task public:check` passes. Evidence: repoguard and contractlint passed;
      the gate stopped only at the deliberate unpublished Gateway digest
      checkpoint.

## Hand off

- [ ] Acceptance criteria and parent Context/release packets agree.
- [ ] Documentation never labels GET safe/read-only or a reviewed effect
      automatically allowed.
- [ ] Durable conclusions are promoted and this temporary packet is removed.
