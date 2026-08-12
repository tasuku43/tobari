# Work Context: Select direct source access per Context

## Current behavior

- `project_runtime.go` emits
  `--mount type=bind,src=<root>,dst=<workspace-root>` without `readonly` for
  every Workspace.
- The selected root is the only project source mounted read-write. The separate
  read-only `/run/tobari/git` bind is a Tobari-generated system Git config
  projection, not an external worktree Git directory or metadata mount.
- Workspaces use the invoking host UID/GID (normally non-root), a read-only
  image root filesystem, dropped capabilities, no Docker socket, a persistent
  `/var/lib/tobari` home bind, and writable `/tmp` and `/run` tmpfs mounts.
  Root invocation is not currently rejected.
- A root below the host home may map beneath the container's
  `/var/lib/tobari` path while the more specific project bind overlays that
  path. Exact Docker integration must verify the nested read-only bind wins.
- Project runtime identity already includes a spec hash over the desired
  runtime contract, and runtime drift recreates only the work container while
  preserving its home.
- A canonical root plus Context ID is already unique, so the same source can be
  read-only in one Context and read-write in another without a new Workspace
  selector.

## Relevant structure

- Entry point: `context create`, `tobari --context`, `status`, `list`
- Domain rule: `ContextManifest`, Context summaries/reports, project runtime
  spec/result types
- Application use case: `internal/app/contextcmd` and `internal/app/tobaricmd`
- Infrastructure boundary: `context_store.go`, `project_runtime.go`, project
  state/spec hashing, Docker inspect/reconciliation
- CLI catalog or presentation: Context create input and Context/Workspace
  observation fields
- Existing tests and harness checks: mount argv tests, runtime inspection,
  forbidden-mount canaries, same-root Context integration, threat-model claim
  table

## Constraints

- Direct binding remains the only V1 source-binding mechanism.
- The access value is host-owned Context authority; repository content and
  runtime image cannot change it.
- There must be no writable alias of the selected source through the Workspace
  home, generated Git projection, profile, or other bind.
- Read-only failure is an ordinary filesystem error from the kernel/Docker
  boundary, not a permission-review candidate or an approval workflow.
- Existing development Contexts are recreated under ADR 0027; missing access
  does not default while reading persisted V1 state.
- Context creation starts no Docker resources; mount behavior begins on root
  entry/reconciliation.

## External facts

- Docker, “Bind mounts,” <https://docs.docker.com/engine/storage/bind-mounts/>,
  checked 2026-08-12: bind mounts are writable by default and `readonly`/`ro`
  prevents container writes through that mount. Product completion relies on
  integration with the supported Engine/kernel matrix, not prose alone.

## Unknowns

- [ ] Observe nested bind behavior for roots below the host home on every
      supported native Linux, Colima, and Lima path.
- [ ] Record representative build/test tools that fail because they write
      caches or generated files into the source tree; documentation must not
      promise those workflows in read-only Contexts.

## Thesis evidence

- Repeated decision: direct read-write was accepted only to avoid clone/overlay
  complexity, not because every Context needs write authority.
- User outcome: create an investigation/review Context that can read the same
  source without modifying it.
- Avoided workaround: advising users to change host permissions or remember a
  Docker flag outside Context authority.
- Proposed revision: direct is the binding mode; read-only/read-write is the
  immutable Context-owned access dimension.
- Downstream impact: ADR 0010, manifest/report schema, runtime spec hash,
  mount tests, security limitations, CLI help, integration matrix.

## Reproduction or observation

```sh
nl -ba internal/infra/dockerruntime/project_runtime.go | sed -n '642,662p'
go run ./cmd/tobari context create --help
rg -n 'projectSpec|spec.hash|readonly' internal/infra/dockerruntime
```

Observed 2026-08-12: the root bind is unconditional and writable; create has no
source-access input; profile and generated Git projection mounts already use
read-only options.

## Security and public-boundary notes

- Assets: host project root integrity, Git metadata, Workspace home, tmpfs.
- Credentials: Workspace-owned credentials remain in the writable home and are
  not protected by read-only source access.
- New dependencies/destinations/processes: none.
- Side effects: Context creation persists one authority fact; runtime entry may
  create/recreate a container with the exact mount access.
- Retry/cancellation: source-access validation is pre-I/O; runtime creation uses
  existing typed reconciliation and cleanup contracts.
- Publication: examples must use synthetic paths and must not imply source
  confidentiality or recovery.

## Glossary

- **Source access:** write authority through the one selected direct project
  bind, not access to every Workspace filesystem.
- **Direct read-only:** the live host directory mounted read-only; not a copy or
  point-in-time snapshot.
- **Writable alias:** any second path in the Workspace through which the same
  host source bytes could be changed.
