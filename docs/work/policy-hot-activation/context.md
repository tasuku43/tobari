# Context: Policy hot activation

## Verified facts

- Interactive `policy review` loops after each detail action. It delegates the
  selected opaque candidate to `policy allow` or `policy deny`, emits that
  mutation result, rereads the queue, and presents the next candidate.
- Each delegated action currently owns a complete activation before it returns.
- Aggregate policy mutation tests the changed Context privately, tests every
  Context while rebuilding the aggregate, tests the complete aggregate, then
  tests the published aggregate again in `ApplyPolicy`.
- `ApplyPolicy` executes `docker compose up -d --no-deps --force-recreate
  --wait opa`.
- A local observation on 2026-08-09 showed a newer OPA start time than the
  Gateway and Workspace containers after exact policy review; the Workspace
  container was not recreated.
- The OPA image is pinned by digest and currently reports OPA 1.17.0.
- OPA documents bundle hot loading, retained prior policy on failed bundle
  activation, and active bundle revision reporting. OPA also documents
  filesystem watch for a bundle file.
- The current security contract explicitly forbids batching candidates and
  requires the portable owner-labeled OPA recreation boundary. Those decisions
  must be revised before implementation.
- The current three-service topology and pinned published images make a new
  resident bundle-distributor service a larger release and supply-chain change.
- The reviewed first slice is bounded to one Context per Apply. This still
  permits several projects and exact effects while keeping durable source
  promotion to one owner-only fsync-and-rename operation. CLI, application,
  and runtime checks reject a cross-Context set before source or Docker I/O.
- Bundle construction writes only a revision-named candidate archive. A second
  fixed, networkless, capability-dropped invocation of the already-pinned
  Debian image renames it to the watched path in the same Docker volume. A
  failed build therefore cannot truncate or replace the active bundle.

## Constraints

- Discovery and action remain separate for non-interactive and machine use.
- Each staged decision must retain the candidate's opaque ID unchanged until
  fresh application validation.
- A batch is a command-bound mutation of the one installation policy decision
  set, not permission to accept multiple unbound public action references.
- The complete candidate must be validated before it can become authority.
- A successful mutation is not reported until OPA proves the expected revision
  is active.
- Authority reduction cannot continue serving the previous Allow set while a
  replacement is being activated.
- Workspace and Gateway traffic must fail closed when active and expected
  policy revisions cannot be reconciled.
- Tests may use synthetic policy and temporary Docker resources only.

## Remaining environment evidence

- Linux CI coverage remains required for the Docker-managed watched-bundle
  behavior proven locally on the supported macOS/Colima development host.
- The repository Docker integration scenario is updated to assert the
  read-only bundle volume, stable OPA container identity, and staged Apply.
  Its local run was refused by the clean-engine guard because the user's
  `tobari-auth-broker`, `tobari-gateway`, and `tobari-opa` were active. Those
  containers were deliberately not stopped or recreated.

## Bounded experiment: Docker-managed watched bundle

On 2026-08-09, the pinned OPA 1.17.0 image built revisioned bundles A and B
from the current validated aggregate into one temporary Docker-managed volume.
One isolated, read-only, capability-dropped OPA container loaded bundle A with
`opa run --server --watch --bundle /bundle/bundle.tar.gz`. A fixed in-container
OPA query observed aggregate revision A. A second isolated builder replaced the
volume bundle with revision B. The same OPA container ID then returned exact
revision B through its Data API. No host bind notification, OPA restart, host
port, Workspace network, or external network was involved. The exact temporary
container, volume, and copied policy inputs were removed after the experiment.

This evidence selects the Docker-managed volume transport for ADR 0024. It
does not by itself prove the Linux CI host, invalid-publication retention, or
authority-reducing fence; those remain required integration cases.

## Verification evidence

- `task check` passed on 2026-08-09 after the final code and documentation
  changes, including repository guards, contract lint, all Go tests, race
  tests, site generation/type/build checks, and 19 Playwright tests.
- `task security` passed on 2026-08-09 with no vulnerabilities found and the
  public documentation source check clean.
- Focused application, CLI Permission Inbox, and Docker-runtime tests passed.
- `bash -n scripts/test-integration.sh` passed.
- `task integration:test` was not run to completion for the active-cluster
  reason above; the clean-engine scenario remains CI/manual evidence.

## Existing unrelated worktree changes

The architecture-site component and content changes plus
`docs/work/site-curtain-grid/` predate this work and must remain untouched.
