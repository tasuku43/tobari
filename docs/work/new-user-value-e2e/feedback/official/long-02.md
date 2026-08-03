# Long-02: least-privilege learning across two project roots

- Status: functional E2E complete; raw capture digest remains an evidence-
  completeness follow-up.
- Subject: two fresh disposable `cc-bash-guard` snapshots.
- Delegation: the child used the exact parent-provisioned wrapper for every
  Tobari invocation, used real PTYs at 120x40, did not read repository files,
  did not edit the subject repositories, and did not commit.

## Value journey

1. The child established independent Workspaces for project A and project B.
2. The same synthetic `example.org:443 GET /` operation was denied in both
   roots. The child explicitly approved A's candidate with `Current Tobari
   only` scope.
3. The identical operation in B remained denied, demonstrating that A's exact
   permission did not widen B's policy.
4. After A was deleted, B reported ready and was successfully re-entered.
   B's independent lifecycle and policy state survived A deletion.
5. A second B candidate, `example.net:443 GET /`, was intentionally denied.
   The remaining review was canceled and visibly reported `No permissions
   changed`, proving cancellation did not become an approval.
6. Both projects were deleted, both reported `No Tobari`, the Workspace list
   was empty, and the cluster reported `Cluster not configured`.

The already-allowed `example.com` request was not counted as a value signal.

## Human-path observations

- The exact scope label `Current Tobari only` was understandable enough to
  validate the cross-root boundary.
- The child used eight visible help calls while discovering lifecycle,
  `status`, `list`, `delete`, cluster controls, and policy review surfaces.
- A single-key `a`/`d` action followed by a pause triggered
  `undeclared_fault_contract`; an explicit `y` confirmation then completed
  safely. The candidate remained controlled, but the normal human timing is
  not robust.
- PTY allocation and terminal size were reported, but no raw capture digest or
  full checkpoint transcript was returned. The functional scope result is
  accepted; the packet-wide raw-evidence predicate remains open.

## Command-surface candidates

- Keep: lifecycle `status`, `list`, `delete`, and `cluster up/status/down`, plus
  the exact policy actions needed for reference-bound allow/deny.
- Integrate/narrow candidate: present `review`, `candidates`, and `tail` as
  one clearly explained permission-queue family while preserving the
  read-only/machine-readable and opaque-reference boundaries. Do not delete a
  surface solely because the human path used another one.
- Docs-only: explain the `Current Tobari only` scope, cross-root behavior,
  deletion/re-entry lifecycle, and why cancellation reports no mutation.
- Deprecate candidate: none evidenced.

## Acceptance boundary

This file satisfies the functional Long-02 outcome: exact project-A scope,
project-B denial, intentional deny/cancel, deletion isolation, and cleanup.
Terminal-dimension evidence is present, but raw PTY digest/checkpoint evidence
is still incomplete for packet-wide acceptance.
