# Quick Start runtime documentation E2E transcript

This is the bounded replay record for the README journey. It uses only
synthetic values and records the actual command, exit, result, and cleanup
boundary. It must not claim a positive Docker result when the environment stops
before that result.

## Documented journey

| Stage | Host/agent | Command or action | Required observation |
|---|---|---|---|
| 1 | Host | `tobari doctor --root .`; `tobari cluster up` | Host checks pass and the shared Gateway/OPA cluster is ready. |
| 2 | Agent | Enter with `tobari`; run `curl -sS -w '\nhttp=%{http_code}\n' -X PUT https://example.com/quickstart` | Secret-free `policy_denied`, `http=403`, fixed `tobari policy review`, no automatic retry. |
| 3 | Host | `tobari policy review --tail 100`; read-only JSON review if needed | Exact `example.com` / `PUT` / `/quickstart` candidate is inspected. |
| 4 | Host | `tobari policy allow --id <exact-pcy-id>` | The exact opaque value is copied unchanged; one exact rule is tested and activated. |
| 5 | Agent | Re-enter with `tobari`; repeat the same curl | The response is upstream-owned rather than Tobari's policy-denied handoff. |
| 6 | Host | `tobari runtime init`; edit active Context `runtime/Dockerfile` to add `tree`; `tobari runtime build --format json` | The explicit build validates and promotes the generated Context runtime. |
| 7 | Agent/host | `tobari`; `tree --version`; `exit`; cleanup | The selected runtime contains the harmless tool; `delete` and `cluster down --purge` clean up exact owned state. |

The public README uses `example.com` because it is synthetic and public-safe.
The deterministic repository integration fixture uses its own named mock
upstream for the same policy dimensions; that fixture hostname is not a public
service and is not presented as a user prerequisite.

## Replay method

Run from a clean repository checkout with a reachable Docker Engine and the
repository's declared Go toolchain:

```sh
go run ./cmd/tobari help --format agent
go run ./cmd/tobari help policy --format agent
go run ./cmd/tobari help runtime --format agent
task integration:test
task runtime:test
```

The integration profile is the preferred deterministic replay because it
creates synthetic project roots, Gateway/OPA, a mock upstream, policy denials,
opaque candidates, exact allow/deny actions, runtime image promotion, and exact
cleanup. Its mock request path is the harness equivalent of the README's
`example.com/quickstart` request. The runtime profile includes the same
integration boundary plus policy and Gateway checks.

## Results

Replay environment: macOS host, Colima Docker context, Docker Engine
27.4.0, linux/arm64. No Tobari-owned containers, networks, or volumes existed
before the replay.

| Replay | Exit | Observed result | Blocker/recovery |
|---|---:|---|---|
| Agent/root and scoped help | 0 | Root agent help returned schema 8, `view=index`, and 25 command entries. Scoped `policy` and `runtime` help returned `view=scope` and the existing review/allow/runtime paths. | No blocker. |
| `task integration:test` | 130 | The real cluster, Gateway, OPA, two Workspaces, and custom-image build became healthy. The run then blocked at the existing PTY-only `bin/tobari policy review --tail 1000` step. It was stopped with Ctrl-C; task output was `task: Failed to run task "integration:test": exit status 130`. | Full TTY integration is not claimed green. The first interrupt left four exact test containers, four exact networks, and two exact CA volumes; those names were removed and verified absent. |
| `task runtime:test` | 130 | Policy passed 27/27 and Gateway passed 25/25 before the same real integration PTY block. It was stopped with Ctrl-C; task output was `task: Failed to run task "runtime:test": exit status 130`. | Aggregate runtime profile is not claimed green because it contains the same TTY blocker. |
| `task policy:test` / `task gateway:test` / `task runtime:base:check` | 0 | Rego passed 27/27, Gateway unit tests passed 25/25, and `runtimecheck: OK` was observed. | No blocker. |
| Manual README runtime edit (`tree`) | 0 | A real `runtime init` created the active recipe; an owner-only Dockerfile edit added `tree`; explicit `runtime build --format json` returned `runtime.status=ready` and promoted `tobari-context-default:<source>`; a repository-local synthetic XDG retry reached `cluster up`, entered with `tobari`, and printed `tree v2.1.0` inside the Workspace. Exact `delete --force` and `cluster down --purge` cleanup completed with no owned Docker resources remaining. | The first external-temp `cluster up` stopped with exit 10, `policy_test_failed`, because the Colima VM could not see the external XDG bind path. The repository-local retry is the successful bounded runtime replay. |

## Gate results

The gate replay used a clean detached checkout at the current `main` commit
with only this README and packet copied into it. The shared worktree's
unrelated dirty paths were not included.

| Command | Exit | Result |
|---|---:|---|
| `git diff --check` | 0 | No whitespace errors. |
| `task check` | 0 | `repoguard (hygiene): OK`, `archlint: OK (24 packages)`, `contractlint: OK`, runtime checks, Gateway snapshot, and full/race Go tests passed. |
| `task public:check` | 0 | `repoguard (public): OK`; `contractlint: OK`. |
| `task policy:test` | 0 | Rego `PASS: 27/27`. |
| `task gateway:test` | 0 | Gateway unit tests `Ran 25 tests`; `OK`. |
| `task runtime:base:check` | 0 | `runtimecheck: OK`. |

The aggregate `task integration:test` and `task runtime:test` outcomes remain
exit 130 because the existing interactive PTY review helper did not complete;
their policy/Gateway portions and the separate bounded runtime replay are
recorded above. No full integration success is claimed.

## Boundary checks

- No credentials, private URLs, machine paths, or shell history are copied into
  this transcript.
- The policy action is host-only and consumes one unchanged opaque reference;
  the denial itself never authorizes or retries the request.
- The runtime Dockerfile is host-owned and its build context is only the active
  Context runtime directory. `runtime build` is the explicit Docker/build-base
  boundary; ordinary `tobari` entry does not pull the selected image.
- A failed build leaves the previous Context image selected. A failed or
  unavailable cluster startup is recorded as a blocker, not a success.
- The reviewed routine-success external-processing count is zero: no provider
  parser, source inspection, exploratory provider call, credential lookup, or
  automatic retry is added by the documentation.

## Cleanup

The positive replay must end with the existing exact cleanup sequence:

```sh
tobari delete
tobari cluster down --purge
```

If the environment stops before cleanup, record the exact stop and whether the
harness performed its bounded cleanup trap. Do not remove unrelated user-owned
Docker resources.
