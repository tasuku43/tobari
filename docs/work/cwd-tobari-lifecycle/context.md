# Work Context: CWD-owned Tobari lifecycle

## Verified implementation state

- The catalog exposes the fixed-target root command `tobari`, plus CWD-scoped
  `status`, exhaustive `list`, and destructive `delete`; the named lifecycle
  commands are no longer public catalog entries.
- Root invocation resolves the nearest canonical root, persists one UUIDv7
  logical instance under XDG state, and keeps logical existence independent of
  container or network health.
- Project homes are host directories under `instances/<id>/home`; project
  networks and containers are owned by exact ID and role labels.
- Runtime inspection reports missing, degraded, or unreachable resources as
  diagnostics, while reconciliation can recreate missing resources and
  reconnect a container to its project network.
- A shared XDG profile is mounted read-only, while each project's `.claude`
  directory and home remain writable only within that project's state.

## Relevant structure

- Entry point: `cmd/tobari/main.go` and `internal/cli/cli.go`
- Domain rule: `internal/domain/tobari`
- Application use case: `internal/app/tobaricmd`
- Infrastructure boundary: `internal/infra/dockerruntime`
- CLI catalog and presentation: `internal/cli`
- Existing tests and harness checks: `internal/*/*_test.go` and `scripts/check.sh`

## Constraints

- The external enforcement topology and credential boundary remain unchanged.
- Runtime mutation remains inside the Docker/filesystem infrastructure port.
- Canonical root lookup must resolve symlinks and select the nearest ancestor.
- A legacy named state is not automatically deleted or reinterpreted.

## Unknowns

- [ ] Whether Docker Desktop permits the XDG state home bind in every supported host configuration; integration verification will record this.
- [ ] Whether a legacy migration command is needed before the first stable release; it is deliberately excluded from this replacement.

## Thesis evidence

- Repeated design decision: user-managed names and lifecycle verbs obscure the one project-directory outcome.
- User outcome: `cd project && tobari` must be sufficient for routine work.
- Proposed thesis revision: Tobari is one long-lived logical environment selected by canonical directory, while containers are recoverable runtime resources.
- Impact: public commands, catalog, state storage, runtime ownership, tests, and documentation require coordinated revision.

## Security and public-boundary notes

- Assets: selected project root, per-instance XDG state home, Docker container and network, shared read-only profile.
- Credentials remain outside profiles and are never copied from a host home.
- Runtime calls remain bounded Docker CLI invocations; no external API is added.
- Legacy state is left intact rather than silently deleting a user home volume.
