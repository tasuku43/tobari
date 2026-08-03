# Medium-02 attempt 01: partial Workspace reuse and cleanup

- Status: non-acceptance partial attempt; reuse and cleanup succeeded, but the
  planned explicit cancellation interaction was not reached.
- Subject: fresh disposable `cc-bash-guard` snapshot with an isolated
  Docker-VM-shared state.
- Delegation: the child used a real 120x40 PTY, did not read or edit repository
  files, and did not commit.

## Observed journey

- The child recovered from an initially unconfigured state and entered a
  Workspace.
- The same project was successfully re-entered and reused.
- No visible selection/recovery cancellation occurred; entry was fast, and
  delete required no confirmation. The intended cancel/no-mutation branch was
  untested.
- Workspace deletion completed. The final visible screens were `No Tobari for
  this directory` and `Cluster not configured`, with no Workspace remaining.
- Parent observation saw an isolated state registration briefly report ready
  while product-visible state was already absent. The child did not bypass this
  with Docker inspection; the discrepancy remains lifecycle evidence.

## Command-surface candidates

- Keep: visible entry/re-entry, `status`, `list`, `delete`, and cluster status
  boundaries.
- Narrow/integrate candidate: expose an intentional cancel/back path before
  creating or deleting a Workspace.
- Docs-only: explain the difference between registered Workspace, running
  runtime, and final `No Tobari`/`Cluster not configured` state.
- Deprecate candidate: none evidenced.

## Acceptance boundary

This attempt proved reuse and cleanup but did not satisfy the full scenario
because no explicit cancellation was observed. It returned no raw digest or
discovery/help count.
