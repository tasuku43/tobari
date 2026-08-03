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
- [ ] `task check` passes. Evidence: blocked before tests by unrelated active
  packet lint finding in `docs/work/auth-broker-deferral/context.md:106`; that
  path is outside this packet's allowed scope.
- [ ] `task security` passes. Evidence: the same unrelated packet lint
  findings stopped the profile before security checks.
- [ ] `task public:check` passes. Evidence: the same unrelated packet lint
  findings stopped the profile before public-boundary checks.
- [ ] `task integration:test` passes. Evidence: Docker/Colima was available
  and the profile reached the existing interactive policy-review scenario,
  but it remained in that scenario until interrupted with exit 130. The exact
  replay command, stop point, and dedicated real-runtime success are in
  `e2e/bash-runtime-transcript.md`.
- [x] Canonical/embedded runtime diff is understood and temporary artifacts
  are removed. Evidence: no runtime source changed; `task runtime:base:check`
  passed. Integration containers and the repository-local temporary roots were
  cleaned; temporary diagnostic images and directories were removed.
- [ ] The delegated sub-agent creates an intentional scoped commit and reports
  its SHA. Evidence: blocked in this execution boundary. `git add --
  internal/infra/dockerruntime/project_runtime_test.go
  scripts/test-integration.sh docs/work/runtime-bash-shell/` cannot create
  `.git/index.lock`; a writable alternate index also cannot insert changed
  blobs into the read-only Git object database. No commit is claimed.

## Hand off

- [ ] Acceptance criteria have E2E evidence; shared-worktree gate blockers
  remain open and are not claimed as passed.
- [ ] Goal status is changed to `Complete` only after required gates and the
  delegated commit are complete.
- [x] Durable decisions were promoted or explicitly confirmed unchanged.
- [x] Temporary diagnostics and sensitive artifacts were removed from the
  repository.
- [ ] Handoff summary explains whether the request required code, what E2E ran,
  the commit SHA, and any environment blocker. The SHA remains absent until a
  Git-writable execution context is available.
