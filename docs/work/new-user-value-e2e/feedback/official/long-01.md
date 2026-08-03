# Long-01: safe first success through policy learning and runtime reuse

- Status: functional E2E complete; evidence-completeness follow-up remains for
  the raw PTY digest and verified terminal dimensions.
- Subject: fresh disposable `cc-bash-guard` snapshot on the parent-provisioned
  Docker-VM-shared path.
- Delegation: the child received only the desired outcome, safety boundary, and
  a preconfigured isolated host entrypoint. It did not read repository files,
  edit the subject repository, or commit.

## Value journey

1. The child saw `cluster_not_configured`, followed the visible startup cue,
   and reached healthy Gateway/OPA services through a real PTY.
2. The first Workspace opened interactively. An earlier `example.com` probe
   was already allowed and was explicitly not counted as the value signal.
3. The child then made a synthetic request to `example.org`. It visibly
   returned `policy_denied`; the host-side Permission Inbox showed the exact
   `example.org:443 GET /` candidate with `Current Tobari only` scope.
4. After human review and explicit confirmation, the exact permission was
   applied. The same request was retried and returned the Example Domain HTML,
   proving that the request crossed the Tobari boundary after the minimum
   permission decision.
5. The child rebuilt a harmless runtime marker and entered a newly created
   Workspace. `echo $TOBARI_TRIAL_MARKER` printed
   `synthetic-runtime-customization`, proving that the custom runtime was
   observable after recreation.
6. `delete` succeeded; `list` showed `No Workspaces`, `status` showed `No
   Tobari for this directory`, and cluster status showed `Cluster not
   configured`.

## Human-path observations

- The first denied request supplied enough information to discover the human
  review boundary without a candidate ID or source inspection.
- Two review attempts surfaced `undeclared_fault_contract` before the child
  completed the confirmation. The candidate remained safe and the final
  confirmation applied only the exact displayed permission. This independently
  corroborates the parent baseline's raw-confirmation timing finding.
- The product offered `tobari retry`, but that command was unknown. The child
  recovered through the visible `Resume: tobari` flow instead. This is a
  contract/navigation defect, not a reason to count the retry as routine.
- The child invoked product help eight times, plus one Bash `help` and one
  failed in-Workspace `tobari --help`. No help was needed before the first
  allowed probe, but discovery became exploratory around review, runtime, and
  recovery.
- All interactions used PTYs. The child did not expose terminal dimensions,
  and no raw PTY digest was returned, so the packet keeps evidence-completeness
  open even though the functional journey succeeded.

## Command-surface candidates

- Keep: visible `Next:` guidance, `cluster up/down`, `status`, `list`, and
  `delete` as distinct lifecycle/recovery boundaries.
- Integrate: connect the exact permission completion to a valid retry/re-entry
  path, or emit only a recovery command that exists in the catalog.
- Narrow: clarify the impact of `runtime build` and the destructive scope of
  `delete --force` before a first-time user chooses them.
- Docs-only: explain that runtime changes require a later/new Workspace entry,
  and document the `context show --name` grammar discovered in the partial
  attempt.
- Deprecate candidate: none evidenced by this journey.

## Acceptance boundary

This file satisfies the functional Long-01 outcome: denied request, exact
human approval, same-request retry, visible runtime customization, and clean
shutdown. It does not close the packet-wide raw-PTY evidence predicate because
the child did not return a capture digest or terminal-size checkpoint.
