# Long-02 attempt 01: stalled before scope evidence

- Status: non-acceptance attempt; parent intervention required.
- Scenario: parent-defined least-privilege journey across two project roots.
- Subject: two clean disposable `cc-bash-guard` snapshots.
- Child outcome: no final report was returned. The parent sent repeated normal
  and interrupting status requests, but the child remained in a live
  Workspace without reporting its screen, scope progress, or blocker. A
  read-only Permission Inbox query remained empty.

## Verified boundary

The parent observed a healthy Workspace container bound to project A, but its
Compose state and instance home were under the maintainer's default Tobari
state rather than the preconfigured isolated wrapper state. This is an
orchestration boundary violation, not valid two-project product evidence.

The child was closed only after status requests had gone unanswered and no
denial/scope signal had appeared. The exact child Workspace container
disappeared on shutdown; supported `list` returned an empty registry,
`delete --force` correctly reported no current-directory Tobari, and
`cluster down --purge` reported the cluster not configured. No active
Tobari-owned containers remained.

## Classification

No value signal, exact allow/deny scope, cancellation, project deletion
isolation, discovery count, or PTY evidence can be accepted from this attempt.
The next Long-02 run needs stronger parent-side entrypoint injection and must
verify the state boundary before the child begins. The child must still receive
only the desired outcome and safety constraints, not a Tobari procedure or
this finding.
