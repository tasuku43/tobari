# Work Context: Make doctor diagnose prerequisites without false blame

## Before-change evidence

- `internal/cli/doctor.go` turns any failed report into `diagnostic_failed` with
  next command `doctor` and a generic instruction to correct the prerequisite.
- In an isolated XDG environment with a fake Docker CLI and unavailable Engine,
  doctor reported Engine fail, Compose fail, and policy fail with Rego/XDG
  blame. The policy test depended on Docker and was not independently observed.
- When Docker CLI was absent, doctor emitted only the Docker CLI failure and
  omitted independently inspectable root/configuration/state checks.
- A prior read can initialize default Context policy, changing a later doctor
  branch and making the false policy diagnosis more likely.
- The product and security contracts explicitly require the complete check set
  and forbid doctor from initializing or repairing state.
- Existing tests claim full-report emission but do not cover the observed
  dependency matrix and first-read interaction.

## Relevant structure

- Entry point/presentation: `internal/cli/doctor.go`
- Domain rule: doctor result/check vocabulary under `internal/domain/doctor`
- Application use case: `internal/app/doctorcmd`
- Infrastructure boundary: `internal/infra/systemdoctor` and runtime diagnostic ports
- Catalog: doctor entry and `diagnostic_failed` declaration
- Harness: complete-report, fail/warn exit, auth no-repair, and recording-runner tests

## Constraints

- Doctor performs bounded observation only.
- A blocked dependent check is known unavailable, not pass, warning, or fail.
- Independent checks continue without speculative initialization.
- External diagnostics remain visible-projected and bounded.
- Recovery commands follow the catalog's exact-command grammar.
- `observational-read-purity` must establish no-init read behavior first.

## External facts

None. Docker availability is represented with fakes and local recording runners.

## Verified implementation facts

- The public inventory is a closed 22-check topological order. A dependent is
  ready only when every declared direct prerequisite passed.
- Infrastructure returns only pass, warning, or failure observations.
  Application code constructs blocked rows and task-owned recovery.
- Doctor JSON schema 2 publishes nullable `blocked_by` and nullable structured
  `recovery`; TSV flattens the same facts and text selects failure-first or
  actionable-warning recovery.
- Policy observation validates bounded owner-only Context source structure on
  the host. Executable OPA tests remain a `cluster up` responsibility; doctor
  issues no policy-test `docker run`.
- Runtime-construction failure keeps host root and Docker observations while
  reporting Context/XDG resolution as the root failure. It never guesses a
  pass for XDG-dependent checks.
- Content-aware fresh-tree and schema-1/schema-2 legacy Context snapshots prove
  doctor does not initialize or migrate owned state.

## Resolved unknowns

- [x] Build the exact check dependency DAG and identify independently ready checks.
- [x] Advance the public doctor report to JSON schema 2.
- [x] Determine which platform recovery facts can be safely stated without
      invoking a shell or guessing a Docker product.

## Thesis evidence

- Repeated confusion: infrastructure unavailability is converted into failures
  at every dependent semantic layer.
- User friction: the report blamed Rego and advised rerunning itself while the
  actual problem was an unavailable Engine.
- Workaround to reject: suppress policy checks only in CLI presentation.
- Thesis resolution: semantics must distinguish unobserved from failed before rendering.
- Downstream impact: domain result, application orchestration, infrastructure
  call bounds, catalog schema, output, and harness dependency fixtures.

## Reproduction or observation

```sh
PATH=/path/to/fake-docker:$PATH tobari doctor --format text
PATH=/usr/bin:/bin tobari doctor --format json
```

Use an isolated XDG tree and fake absolute Docker executable. Do not depend on
or mutate the developer's Engine.

## Security and public-boundary notes

- Assets/effects: host configuration, Docker availability, policy/provider,
  key/vault/broker/project-binding observations; no repair.
- Credentials: diagnostic results remain secret-free.
- New dependencies/destinations: none.
- Delivery: complete finite check inventory, no pagination/retry.
- Publication: synthetic paths and diagnostics only.

## Glossary

- Failed: the check ran and observed a contract violation.
- Blocked: the check could not run because a declared prerequisite was unavailable.
- Independent check: a check whose inputs remain observable despite another failure.
