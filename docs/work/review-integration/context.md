# Work Context: Align CWD-owned lifecycle boundaries

## Current behavior

- `internal/app/tobaricmd/service.go` calls `ClusterUp` before resolving a project in `EnterProject`.
- Project state is stored below XDG state and project runtime reconciliation is implemented in `internal/infra/dockerruntime/project_runtime.go`.
- Shared cluster state still has the legacy `State.Tobari` collection and named lifecycle methods in `RuntimePort`.
- Existing integration tests cover CWD reuse, network separation, runtime recovery, policy enforcement, profile isolation, and cleanup.
- The review specification requires explicit cluster readiness, protected-root checks, mutation recovery, current-state concurrency, runtime readiness, spec drift, and retirement of the legacy authority.

## Relevant structure

- Entry point: `cmd/tobari/main.go`
- Domain: `internal/domain/tobari`
- Application use case: `internal/app/tobaricmd.Service`
- Infrastructure boundary: `internal/infra/dockerruntime.Runtime`
- CLI catalog: `internal/cli/runtime_catalog.go`
- Harness: `scripts/check.sh`, `scripts/test-integration.sh`

## Constraints

- Preserve the four-layer dependency direction and catalog-as-source-of-truth rule.
- Keep credentials and active policy host-side and keep Docker effects behind infrastructure ports.
- Preserve existing public cluster/policy outcomes unless the review explicitly changes their data source.
- Do not auto-migrate legacy named state or delete user data outside exact validated targets.
- Use synthetic fixtures and commit by concern.

## Unknowns

- [ ] Exact legacy runtime methods that can be removed without losing currently supported cluster/policy behavior.
- [ ] Whether Docker inspect exposes enough stable fields for a portable project spec hash across supported engines.
- [ ] Which cluster state fields are required by policy and logs after removing the legacy project collection.

## Thesis evidence

- Repeated design decision or point of agent confusion: shared cluster lifecycle and CWD project lifecycle are currently interleaved in one root operation.
- User outcome or friction observed in the minimal slice: a project command can create shared infrastructure before the user explicitly asks for it.
- Code workaround or exception being considered: keep legacy named methods hidden while adding new project behavior.
- Current thesis that resolves it: shared cluster and project runtime are separate explicit boundaries; hidden legacy authority must be removed or isolated.
- Downstream impact: product contract, security model, architecture, cluster state model, catalog errors, and integration harness must agree.

## Reproduction or observation

```sh
rg -n "ClusterUp|State\.Tobari|Attach\(|Detach\(|Exec\(|ensureProjectContainer|ClusterDown" internal
```

## Security and public-boundary notes

- Assets: XDG state/config/data, active policy, credentials, Docker networks/containers, Gateway, OPA.
- Credentials: no new credentials; protected-root validation prevents project bind mounts from reaching them.
- New side effects: cluster explicit reconcile may reconnect Gateway to registered project networks; bare `tobari` must not mutate shared state.
- Output: stable faults must distinguish unconfigured, unhealthy, protected-root, drift, and readiness failures.
- Publication: temporary packet contains no secrets or private identifiers.
