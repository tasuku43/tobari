# Runtime lifecycle E2E evidence

This is sanitized evidence from a task-owned disposable Docker state. It
contains no host paths, credentials, image digests, or raw terminal bytes.

## Precondition failure: no broken Workspace

The active Context selected the synthetic image
`localhost/missing-runtime:latest`. A real OS PTY was configured as
`TERM=xterm-256color`, 120 columns by 40 rows, and started from a fresh
`project-c` directory.

Observed result:

```text
Message       selected Tobari image is not available locally; build or pull it explicitly
Kind          unavailable
Code          image_not_found
Retryable     no
Next          tobari runtime build — Build or make the selected compatible runtime image available to Docker.
```

The follow-up `list --format=json` contained the pre-existing task-owned
records only; no `project-c` record or runtime resource was created.

## Normal entry and re-entry

The active Context was then switched to a built compatible image. A fresh
`project-d` was entered through the same 120x40 PTY. The human-visible shell
and marker output were:

```text
tobari@...:~/project-d$ printf ...
e2e_runtime_uid=501 e2e_runtime_name=tobari
e2e_runtime_home=/var/lib/tobari e2e_runtime_cwd=/var/lib/tobari/project-d
e2e_runtime_shell=/bin/bash
Workspace session closed.
Workspace remains available.
```

A second real PTY entry printed `e2e_reentry=ok e2e_runtime_shell=/bin/bash`,
then exited cleanly. The Workspace remained `runtime: ready` in `list` after
both sessions.

## Identity disposition

The clean supported image used for this replay maps the host execution UID to
the `tobari` account and produces a normal `tobari@...` prompt. The earlier
`I have no name!` observation therefore belongs to an image whose passwd entry
did not match the host UID; no product change is justified by the bounded
replay. The image contract remains host-owned and should be rechecked if a
future supported runtime build reproduces the mismatch.
