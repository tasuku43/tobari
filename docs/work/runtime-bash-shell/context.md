# Work Context: Enter Tobari workspaces through Bash

This file records verified facts and unresolved questions. It does not treat
source presence as runtime proof.

## Main-history reconciliation

- The current checkout is `main` after `c957401`; the runtime evidence commit
  remains in the supported main history.
- The Bash regression/test evidence is in
  `912c602b4e80d055775e557e6e509b6beff26928`; it is an ancestor of merge
  `966dd08841a7ccd88212dd9c8683562c99e17aa9` and current `main`.
- The scoped runtime commit is present in the supported main history. There is
  no paused rebase, read-only Git metadata, missing commit, or integration
  blocker for this packet.

## Current behavior

- `runtimes/base/Dockerfile` currently installs `bash` and creates the `tobari`
  user with `--shell /bin/bash`.
- `internal/cli/tobari.go` currently sends
  `tobari.ExecRequest{Command: []string{"/bin/bash"}, Interactive: true,
  TTY: true}` for the root `tobari` shell path.
- `internal/infra/dockerruntime/project_runtime.go` currently enters the
  project container with `docker exec -i -t ... /bin/bash`.
- The base image keeps `/usr/local/bin/tobari-entrypoint` as its entrypoint and
  declares `sleep infinity` as the image lifetime contract. This must not be
  changed to Bash because Workspace lifetime belongs to Tobari infrastructure.
- The canonical source and embedded snapshot are synchronized by
  `scripts/sync-runtime-base.sh`; `task runtime:base:check` is the focused
  source/snapshot contract.

## Relevant structure

- Entry point: `cmd/tobari` -> `internal/cli` -> `internal/app/tobaricmd` ->
  `internal/infra/dockerruntime`
- Runtime source: `runtimes/base/Dockerfile` and `runtimes/base/entrypoint.sh`
- Embedded snapshot: `internal/infra/runtimeassets/assets/tobari/`
- Interactive shell request: `internal/cli/tobari.go`
- Docker exec adapter: `internal/infra/dockerruntime/project_runtime.go`
- Existing runtime and lifecycle checks: `scripts/check-runtime-base.sh`,
  `scripts/test-integration.sh`, and the runtime/lifecycle Go tests

## Constraints

- The CLI catalog already exposes the root `tobari` fixed-target action; this
  work must not add a command or shell-selection input.
- The image `CMD` remains an implementation detail that cannot own Workspace
  lifetime. The infrastructure appends the fixed lifetime command and enters a
  child shell through Docker exec.
- Custom images must preserve the same runtime API, user, entrypoint, and
  lifetime command. Bash is a base-runtime contract, not permission to grant
  more authority.
- Tests use synthetic local state and must not require host credentials or
  private external content.

## Unknowns

- [x] The current Docker Engine can build and run the canonical base image on
  this host. The build and runtime assertions passed on Docker/Colima.
- [x] A real `tobari` PTY entry reached the shell path and returned to a live
  Workspace. The full integration profile also completed the policy-review
  interaction and finished with `integration: OK`.
- [x] The focused fixture did not mechanically assert the exact interactive
  `/bin/bash` argv or the base-image shell contract, so those assertions were
  added without changing production behavior.

## Thesis evidence

- Repeated design decision or point of agent confusion: keep the image's
  lifetime process separate from the interactive shell so a child Bash exit
  does not delete or stop a Workspace.
- User outcome or friction observed in the minimal slice: the requested
  default interactive shell is Bash, not an image-defined long-lived command.
- Code workaround or exception being considered: none; prefer the existing
  fixed `/bin/bash` exec contract and add evidence if it is under-tested.
- Current thesis that resolves it, or proposed thesis revision: Thesis 4/5
  and the runtime image contract already resolve the boundary.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: add only regression/E2E evidence; runtime behavior did not diverge.

## Verified E2E evidence

- The canonical `runtimes/base` image built successfully and reported
  `base-bash-ok path=/usr/bin/bash shell=/bin/bash user=tobari`.
- The embedded `tobari-runtime:local` image reported the same Bash/user
  contract. `task runtime:base:check` also confirmed canonical/embedded
  source agreement.
- The actual root `tobari` PTY path reported `$BASH=/bin/bash` and
  `tobari-tty:yes`; after `exit`, the same container reported
  `running=true`, `cmd=["sleep","infinity"]`, and the fixed Tini entrypoint.
- The first-wave `task integration:test` profile was interrupted with exit 130
  after reaching its interactive policy-review case. See [the transcript](e2e/bash-runtime-transcript.md)
  for that historical replay; the repaired current-main replay is recorded
  below.
- Current-main `task check` and `task public:check` are clean repository gates
  after the deferred auth packet was committed. A clean `HEAD + allowed packet
  diff` security snapshot also passes; the current worktree `task security`
  invocation is blocked only by the out-of-scope untracked
  `docs/work/architecture-publication/context.md:57` link.
- The current-main `task integration:test` replay after `c957401` passed the
  runtime Bash entry, policy-learning review, compaction, and cleanup with
  `integration: OK`.
- The scoped runtime commit is already in `main`; final packet documentation
  is a separate scoped change and does not alter production behavior.

## Reproduction or observation

```sh
task runtime:base:check
rg -n 'bash|/bin/bash|sleep infinity|tobari-entrypoint' \
  runtimes/base internal/infra/runtimeassets/assets/tobari \
  internal/cli internal/infra/dockerruntime
```

Expected source observation is Bash in the image and `/bin/bash` in the
interactive exec request. The dedicated Docker-backed PTY replay and the
full supported integration are recorded above.

## Security and public-boundary notes

- Assets and side effects involved: runtime image contents, Docker build/run,
  and one interactive child exec; no policy or credential mutation.
- Credentials or confidential data involved: none.
- New dependencies, destinations, files, processes, or generated content:
  none expected beyond an explicit local image build and E2E transcript.
- External schema provenance, publication rights, and drift evidence: no new
  external schema; preserve existing pinned Debian/runtime sources.
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: not applicable to the shell contract;
  child exit status remains unchanged and the Workspace remains reusable.
- Publication and licensing concerns: none unless the runtime source changes;
  a changed published image must retain the existing runtime/public checks.

## Glossary

- **Lifetime command:** infrastructure-owned `sleep infinity` process that
  keeps the Workspace runtime reusable.
- **Interactive shell:** child `docker exec` session started by host `tobari`,
  currently `/bin/bash` with stdin/stdout/stderr and a TTY attached.
