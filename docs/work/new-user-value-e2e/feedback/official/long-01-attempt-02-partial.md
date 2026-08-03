# Long-01 attempt 02: partial blind journey

- Status: partial; not acceptance evidence for Long-01 because the policy
  learning loop was not exercised.
- Subject: fresh disposable `cc-bash-guard` snapshot on the parent-provisioned
  Docker-VM-shared path.
- Interaction: real PTYs were used for host and Workspace sessions. The child
  did not read repository files, edit the subject repository, or commit. No raw
  digest was returned, and the child did not explicitly set the requested
  120x40 terminal size.

## Observed journey

- The child discovered `cluster_not_configured`, followed the visible `cluster
  up` cue, and reached healthy Gateway/OPA services.
- A first Workspace appeared through the interactive terminal.
- The child chose `curl -I https://example.com`, which returned `HTTP/2 200`.
  No Tobari denial, Permission Inbox, human approval, or retry occurred. The
  first network signal therefore proved connectivity, not the intended safety
  value.
- The child added a harmless marker to the isolated runtime Dockerfile,
  rebuilt the runtime, deleted the old Workspace, and re-entered. The marker
  was visible, proving the custom runtime path when the Workspace is recreated.
- The Workspace prompt visibly included `I have no name!`, which the child
  treated as a usability regression.
- One backtrack occurred because `context show default` required the
  `--name=default` form. The child used six help screens in total and none
  before the first connectivity signal.

## Command-surface feedback

- Keep: visible `Next:` guidance, `status`, `list`, `delete`, and
  `cluster up/down`.
- Integrate candidate: make the runtime-recreation requirement and the
  denial-recovery path visible in the surrounding flow.
- Narrow candidate: make `runtime build` impact and `delete --force`
  consequences clearer.
- Docs-only candidate: explain Workspace reuse, runtime lifecycle, and the
  `context show --name` grammar.
- Deprecate candidate: none evidenced.

## Cleanup and classification

Workspace deletion and cluster shutdown succeeded; the child reported no
remaining Workspace or cluster. The isolated built runtime image remains
because no supported runtime reset/remove command was exposed, and the child
did not bypass that boundary with direct Docker or filesystem deletion.

This is a self-discovery finding: an outcome-only request for a harmless
outbound request can select an already-allowed method and miss the product's
core denial-to-review value. The next Long-01 attempt may state the desired
denied-and-recovered outcome, but must still omit command names and prepared
steps.
