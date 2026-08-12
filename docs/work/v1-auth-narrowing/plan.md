# Work Plan: Narrow brokered authentication for first public V1

## Chosen approach

First promote a superseding V1 scope decision. Add negative catalog and state
canaries, narrow public auth inputs/results to GitHub plus owner static import,
then delete managed, provider-specific dynamic/helper, companion, refresh,
signing, exact-version, and unowned dependency paths vertically. Re-run the
retained static broker security suite before finalizing source snapshots.

## Layer ownership

- Domain: retain only static primary-secret plan, exact HTTP/header binding,
  opaque project/revision handle, rotation, and revocation vocabulary.
- Application: retain GitHub login, static import/status/logout use cases and
  remove provider-specific inputs/results.
- Infrastructure: retain root key, encrypted static vault, Auth Broker sockets,
  GitHub acquisition, strict owner manifests, and Gateway post-allow exact
  replacement; delete every retired implementation and asset.
- CLI: narrow catalog/help/schema/fault/recovery/provider reporting and clarify
  brokered versus Workspace-owned authentication.

## Ordering

1. Accept the envelope and V1 auth retirement decision.
2. Land contract canaries and narrow catalog/application/domain shapes.
3. Delete managed injection, retired built-ins/drivers, companion, refresh,
   signing, state readers, assets, and dependencies.
4. Re-prove retained static security invariants and source-snapshot equality.
5. Integrate independently of source/policy lanes, resolving shared files only
   in the integration lane.

## Verification

Run focused domain/application/CLI, Gateway/Auth Broker, runtime, source
snapshot, and hostile-state tests. Then run `task check`, `task security`, and
`task public:check` on the integrated branch. Live GitHub acquisition remains a
recorded manual release observation and supplies no fixture.

## Rollout and rollback

There is no state migration. Log out/delete old development Workspaces and
Contexts with the old snapshot, stop the old cluster, then recreate state.
Rollback before publication is a source revert plus the same recreation.
