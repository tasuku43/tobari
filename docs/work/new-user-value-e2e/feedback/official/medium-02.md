# Medium-02: Workspace recovery and reuse

- Status: partial functional journey; reuse and cleanup succeeded, but the
  planned explicit cancellation interaction was not reached.
- Subject: fresh disposable `cc-bash-guard` snapshot with the parent-provided
  exact wrapper and isolated Docker-VM-shared state.
- Delegation: the child used a real 120x40 PTY, did not read or edit repository
  files, and did not commit.

## Observed journey

- The child recovered from an initially unconfigured state and entered a
  Workspace.
- The same project was successfully re-entered and reused.
- No visible selection/recovery cancellation occurred; entry was fast, and
  delete required no confirmation. This leaves the intended cancel/no-mutation
  branch untested.
- Workspace deletion completed. The final visible screens were `No Tobari for
  this directory` and `Cluster not configured`, with no Workspace remaining.
- During parent observation, an isolated state file briefly reported a ready
  registration while the product-visible state was already absent. The child
  did not bypass this with Docker inspection; the discrepancy is recorded as
  lifecycle evidence rather than resolved by inference.

## Command-surface candidates

- Keep: visible entry/re-entry, `status`, `list`, `delete`, and cluster status
  boundaries; they made the successful cleanup result observable.
- Narrow/integrate candidate: make the reuse and deletion consequence visible
  at the same lifecycle boundary, and expose an intentional cancel/back path
  where a user can stop before creating or deleting a Workspace.
- Docs-only: explain the difference between a registered Workspace, a running
  runtime, and the final `No Tobari`/`Cluster not configured` state.
- Deprecate candidate: none evidenced.

## Acceptance boundary

The run proves functional Workspace reuse and cleanup but does not satisfy the
full Medium-02 scenario because no explicit cancellation was observed. It also
returned no raw digest or discovery/help count, so packet-wide evidence
completion remains open.
