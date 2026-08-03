# Work Tasks: Enter Tobari workspaces through Bash

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, and harness
  sections. Evidence: docs/00 through docs/04 reviewed before packet creation.
- [x] Observe current source behavior. Evidence: canonical and embedded
  Dockerfiles install Bash; CLI and Docker adapter request `/bin/bash`.
- [x] Record verified facts and unknowns in `context.md`.
- [x] Confirm the public outcome and non-goals in `goal.md`.

## Decide

- [x] Decide whether a production change is needed after runtime E2E. Evidence:
  the canonical and embedded images already contain Bash, the user shell is
  `/bin/bash`, and the existing `tobari` entry path already requests Bash.
  Production code was left unchanged.
- [x] Keep the fixed-target `tobari` action and existing `/bin/bash` request;
  no new catalog input or capability.
- [x] Preserve the infrastructure-owned `sleep infinity` lifetime boundary.
- [x] Record any durable contract correction in the governing document before
  closing. Evidence: no correction was needed; the governing contract already
  matches the verified behavior.

## Implement

- [x] Add a focused regression assertion if the current Bash contract is not
  mechanically covered. Evidence: `TestEnterProjectRuntimeMirrorsHostCWDPath`
  now asserts the complete `docker exec -i -t ... /bin/bash` argv, and the
  integration fixture asserts the image/user/TTY/lifetime contract.
- [x] Synchronize `runtimes/base` into the embedded snapshot if the source
  changes. Evidence: no runtime source changed; `task runtime:base:check`
  passed and the canonical/embedded E2E checks both passed.
- [x] Do not modify unrelated runtime, policy, auth, or catalog work.

## Verify

- [x] `task runtime:base:check` passes. Evidence: `runtimecheck: OK`.
- [x] Base-image E2E proves executable Bash and `tobari`'s `/bin/bash` shell.
  Evidence: canonical output `base-bash-ok path=/usr/bin/bash shell=/bin/bash
  user=tobari`; embedded output `embedded-bash-ok path=/usr/bin/bash
  shell=/bin/bash user=tobari`.
- [x] Interactive `tobari` E2E proves `/bin/bash` with a TTY and a reusable
  Workspace after child exit. Evidence: the real PTY transcript reports
  `tobari-shell:/bin/bash`, `tobari-tty:yes`, followed by
  `Workspace remains available.` and Docker inspection
  `running=true cmd=["sleep","infinity"]`.
- [x] `task check` passes. Evidence: current-main `task check` passes with
  repository hygiene, architecture, catalog, runtime, vet, race, tidy, and Go
  test checks green.
- [x] `task security` passes. Evidence: the clean `HEAD + allowed packet diff`
  security snapshot passes with repository guard, dependency verification, and
  vulnerability checks green; the current worktree invocation is blocked only
  by the out-of-scope untracked architecture-publication packet link.
- [x] `task public:check` passes. Evidence: current-main `task public:check`
  passes with public repository guard and contract checks green.
- [x] `task integration:test` passes. Evidence: current `main` completes the
  canonical/embedded Bash contract, real PTY Workspace entry/reentry,
  runtime reuse, policy learning, interactive review, and cleanup with
  `integration: OK`.
- [x] Canonical/embedded runtime diff is understood and temporary artifacts
  are removed. Evidence: no runtime source changed; `task runtime:base:check`
  passed. Integration containers and the repository-local temporary roots were
  cleaned; temporary diagnostic images and directories were removed.
- [x] The delegated sub-agent creates an intentional scoped commit and reports
  its SHA. Evidence: `912c602b4e80d055775e557e6e509b6beff26928` contains the
  scoped runtime test/transcript changes and is merged into current `main` by
  `966dd08841a7ccd88212dd9c8683562c99e17aa9`.

## Hand off

- [x] Acceptance criteria have E2E evidence; the dedicated Bash E2E, full
  repository gates, and broader runtime integration all pass.
- [x] Goal status is changed to `Complete` after required gates and the
  delegated commit are complete.
- [x] Durable decisions were promoted or explicitly confirmed unchanged.
- [x] Temporary diagnostics and sensitive artifacts were removed from the
  repository.
- [x] Handoff summary explains whether the request required code, what E2E ran,
  the commit SHA, and the remaining environment blocker. Evidence: the
  dedicated runtime transcript, `912c602b`, and the unresolved integration
  result are recorded in this packet.
