# Work Context: CWD-owned Tobari lifecycle

## Current behavior

- The catalog exposes `attach --name --root`, `shell --id`, `exec --id`, `logs --id`, and `detach --id`; root invocation returns `missing_command`.
- Schema-2 state stores all named instances in one state file and each instance owns a Docker volume home.
- The Docker runtime already canonicalizes host directories and attaches Gateway to each owned network.

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
