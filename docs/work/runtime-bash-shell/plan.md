# Work Plan: Enter Tobari workspaces through Bash

- Status: Complete
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Verify the existing Bash contract end to end before changing production code.
If a boundary is missing, add the smallest regression test or runtime-source
change in the existing base-image and interactive-exec paths, synchronize the
embedded snapshot, and preserve the infrastructure-owned lifetime command.

The verification found the production contract already complete. The chosen
implementation is therefore regression coverage only: assert the exact
interactive Docker argv in the adapter test and exercise the canonical,
embedded, and real `tobari` paths in the integration fixture/transcript.

## Alternatives considered

### Alternative A: Make the image `CMD` Bash

Rejected. The image `CMD` cannot own Workspace lifetime; a child shell exit
must leave the logical Workspace and `sleep infinity` runtime reusable.

### Alternative B: Keep `/bin/sh` and document Bash as optional

Rejected. The requested user outcome is a Bash interactive entry, and the
current product contract already treats `/bin/bash` as the fixed shell path.

## Design

### Public contract

No new command, input, capability, effect, target, opaque reference, or
recovery path. The existing fixed-target root `tobari` action enters an
interactive `/bin/bash` child through a TTY. The base image provides Bash and
the `tobari` user shell. Redirected or machine-readable command behavior is
unchanged.

### Layer changes

- Domain: none.
- Application: none unless an existing lifecycle result needs a regression
  assertion; no new port.
- Infrastructure: base image/snapshot and Docker exec contract only.
- CLI and catalog: retain the existing fixed `/bin/bash` request; no catalog
  change.

### Data and control flow

```text
host `tobari`
  -> catalog-owned fixed current-directory action
  -> application Attach/Exec task
  -> infrastructure `docker exec -i -t ... /bin/bash`
  -> Bash child in the selected Workspace
  -> child exit status returns to host
  -> lifetime `sleep infinity` keeps Workspace reusable
```

### Error and cancellation behavior

Missing or incompatible images fail before project resource mutation. A
missing Bash executable or wrong interactive command is a runtime contract
failure, not a fallback to an undeclared shell. Child exit status is preserved;
normal shell exit detaches the session and does not delete the Workspace.

### Security and public boundary

The image remains an untrusted runtime with the existing user, read-only root,
capability, mount, network, resource, and health constraints. Installing Bash
adds no host authority, credential access, or network destination. Explicit
Docker build/run is the only image side effect in the E2E.

## Implementation slices

1. Reproduce the canonical and embedded image contracts. **Complete.**
2. Add or refine focused Bash/runtime regression assertions if a gap exists.
   **Complete; no production source change was needed.**
3. Replay the interactive entry and reusable-Workspace E2E. **Complete for
   the dedicated real-runtime replay and the full supported integration.**
4. Run required repository gates and commit only the scoped packet/change.
   **The required repository gates are recorded in the task handoff, and the
   scoped implementation/evidence commit is
   `912c602b4e80d055775e557e6e509b6beff26928`, already merged into `main`.**

## Verification

- Unit and contract tests: runtime metadata, Docker exec argv, and child exit
  status tests.
- Negative side-effect tests: incompatible image or missing Bash fails before
  project resource mutation; shell cancellation does not delete a Workspace.
- Opaque-reference and complete-pagination tests: not applicable; this is a
  fixed-target action with no user-selected reference.
- Structured output, hostile-output, and recovery tests: preserve existing
  lifecycle summary and exact `Resume: tobari` recovery behavior.
- Agent-readiness scenario and discovery-round-trip count: one known root
  entry path; no new discovery round trip or external parser.
- Human-handoff scorecard for setup/authentication candidates: not applicable.
- Manual observation: a real TTY or equivalent PTY transcript showing Bash and
  reusable Workspace after exit.
- Required profiles: `task runtime:base:check`, `task check`, `task security`,
  and `task public:check`; `task integration:test` where Docker is available.
- Generated-diff or artifact checks: canonical/embedded runtime equality and
  no unreviewed generated files.

The current-main `task check`, `task security`, and `task public:check` profiles
pass after the deferred auth packet was committed. `task integration:test` now
also passes the Bash runtime and policy-learning path with `integration: OK`;
the bounded PTY bridge and its completion are recorded in the policy-review
packet.

## Rollout and rollback

No state migration. If a source change is required, the embedded snapshot must
be synchronized and the previous image contract remains the rollback point.
The user can keep using the existing selected image until an explicit runtime
build or release promotion succeeds.

## Documentation promotion

If verification confirms the existing contract, keep the durable product and
architecture documents unchanged and preserve only the regression evidence in
tests. If behavior differs, update the existing runtime contract and harness
claim in the same reviewed change before closing this packet.
