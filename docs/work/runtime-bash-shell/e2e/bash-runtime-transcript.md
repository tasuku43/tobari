# Bash runtime E2E transcript

This transcript records the runtime evidence for `runtime-bash-shell`. The
image tags and temporary roots below are local test fixtures only.

## Canonical and embedded image contracts

```text
$ docker build --tag tobari-runtime-bash-shell:local --file runtimes/base/Dockerfile runtimes/base
... DONE

$ docker run --rm --entrypoint /bin/bash tobari-runtime-bash-shell:local -lc '...'
base-bash-ok path=/usr/bin/bash shell=/bin/bash user=tobari

$ docker run --rm --entrypoint /bin/bash tobari-runtime:local -lc '...'
embedded-bash-ok path=/usr/bin/bash shell=/bin/bash user=tobari

$ docker image inspect --format 'cmd={{json .Config.Cmd}} entrypoint={{json .Config.Entrypoint}} user={{.Config.User}}' tobari-runtime-bash-shell:local
cmd=["sleep","infinity"] entrypoint=["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"] user=tobari

$ task runtime:base:check
runtimecheck: OK
```

The image checks used `test -x /bin/bash`, verified the `tobari` passwd entry
was `/bin/bash`, and verified the container user was `tobari`.

## Actual `tobari` PTY entry

The following was run against a real Docker/Colima cluster and the actual
root `tobari` command, with a PTY attached. The child commands printed the
shell path and checked all three standard streams with `test -t`.

```text
terminal: tobari@...:~/workspace$ printf 'tobari-shell:%s\n' "$BASH"
tobari-shell:/bin/bash
terminal: ...$ if test -t 0 && test -t 1 && test -t 2; then ...
tobari-tty:yes
terminal: ...$ exit
Workspace session closed.
Workspace remains available.
Resume: tobari
```

Immediately after the child exited:

```text
$ docker inspect --format 'running={{.State.Running}} cmd={{json .Config.Cmd}} entrypoint={{json .Config.Entrypoint}}' <workspace-container>
running=true cmd=["sleep","infinity"] entrypoint=["/usr/bin/tini","--","/usr/local/bin/tobari-entrypoint"]
```

This proves the interactive child is Bash and that its exit does not stop the
Workspace lifetime process.

## Historical broader integration profile

Docker/Colima was available. The profile was run with a writable BuildKit
configuration because the host Docker configuration directory is read-only to
the test sandbox:

```text
buildx_config="$(mktemp -d)"
DOCKER_CONFIG="$HOME/.docker" \
BUILDX_CONFIG="$buildx_config" \
DOCKER_CONTEXT=colima \
GOCACHE=/tmp/tobari-runtime-bash-gocache \
task integration:test
```

Cluster startup, custom-image build, runtime setup, and policy fixtures ran.
The existing policy-review PTY case then stopped making progress while waiting
for its interactive review process. It was interrupted to clean up the live
cluster:

```text
^Ctask: Signal received: "interrupt"
task: Failed to run task "integration:test": exit status 130
```

This historical run is recorded as the reason the shared PTY bridge needed a
bounded repair, not as a Bash E2E failure. The dedicated real-runtime replay
above completed successfully.

## Historical recovery rerun

After Git recovery onto `codex/first-wave`, the focused checks were replayed:

```text
task runtime:base:check
runtimecheck: OK

go test ./internal/infra/dockerruntime
ok  github.com/tasuku43/tobari/internal/infra/dockerruntime

task integration:test
task: Failed to run task "integration:test": exit status 130
```

The fresh Docker/Colima PTY replay again produced
`tobari-shell:/bin/bash`, `tobari-tty:yes`, `Workspace remains available.`,
and `running=true cmd=["sleep","infinity"]`. The integration process again
stopped at the existing `/review-interactive` policy-review case before it was
interrupted.

`task check`, `task security`, and `task public:check` each stopped before
their normal checks on `docs/work/auth-broker-deferral/context.md:106`:
`machine-specific home directory path`. That packet is outside this child
packet's allowed write scope and was not edited.

## Current supported integration replay

After the bounded PTY bridge repair in `c957401`:

```text
$ task integration:test
delete_target: ... runtime=missing
delete_target: ... runtime=ready
integration: OK
```

This same run covered the canonical and embedded Bash image contract, real
PTY `tobari` entry/reentry, reusable `sleep infinity` Workspace lifetime,
Gateway/OPA policy learning, interactive review, compaction, and cleanup. The
review input was sent through a 40x120 PTY as `3`, `d`, `y`, `q`; the selected
candidate was denied and the remaining review queue was exited explicitly.
