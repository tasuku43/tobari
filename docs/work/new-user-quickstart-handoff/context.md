# Work Context: Make the first-use host handoff executable

This file records verified evidence and dependencies; it does not replace the
product or catalog contract.

## Current behavior

- `policy_denied` visibly names host-side `tobari policy review` and states that
  retry is not automatic.
- The policy allow success renderer had a stale `retry` recovery label even
  though no `retry` catalog path exists. The valid host handoff is `tobari`:
  re-enter the Workspace and retry the same request inside it.
- Long-01 and Medium-01 explored `tobari` inside the Workspace before
  discovering that review/recovery belongs on the host. README now labels the
  host-owned and Workspace-owned portions of the journey explicitly.
- Runtime preparation/build and cleanup are separate host-owned boundaries.
  The active Context image is preflighted before a new Workspace is registered;
  a missing or incompatible image leaves no broken Workspace and points to
  `tobari runtime build`.
- `runtime build` promotes the active Context image for new Workspaces only.
  An existing Workspace keeps its stored image and is not silently refreshed.
- The supported interactive session is Bash as user `tobari`, with
  `BASH=/bin/bash`; the host-controlled `sleep infinity` process owns the
  reusable Workspace lifetime.
- A Docker VM bind path is a prerequisite on Colima/Lima: the project and
  Tobari XDG configuration/state directories must be visible to the VM.
  An unshared path fails setup before Workspace startup; sharing checks remain
  unchanged.
- The completed `quickstart-runtime-docs` packet already documents the core
  denial/runtime journey. This successor keeps its exact synthetic request,
  host policy flow, runtime commands, and cleanup, and adds only the boundary,
  stale-recovery, preflight, new-Workspace, Bash, and shared-path handoffs.

## Relevant structure

- Public journey: `README.md` and numbered `docs/` contracts.
- Catalog/recovery source: `internal/cli/runtime_catalog.go` and CLI help.
- Policy denial/review output: `internal/cli/` and `internal/app/tobaricmd/`.
- Runtime documentation dependency:
  [runtime-lifecycle-reconcile](../runtime-lifecycle-reconcile/goal.md).
- Evidence source:
  [new-user E2E comparison](../new-user-value-e2e/comparison.md).
- Prior docs packet: `docs/work/quickstart-runtime-docs/`.

## Constraints

- Public docs must name exact catalog paths and preserve host/Workspace trust
  ownership.
- Do not add or remove a command from documentation until catalog and recovery
  compatibility are checked.
- Use `example.com`/`example.org` synthetic values only; no private URLs,
  credentials, local paths, or shell history.
- Keep recovery commands executable under the catalog: `tobari`, `tobari
  policy review`, `tobari policy allow --id ID`, `tobari policy deny --id ID`,
  `tobari runtime build`, `tobari delete`, and `tobari cluster down --purge`.
- Do not imply that re-entering an existing Workspace applies a newly built
  Context image.
- Documentation changes require public-boundary and full repository checks.

## Unknowns

- [x] The valid host re-entry wording is `tobari`, matching the existing
      Workspace `Resume: tobari` guidance; no retry command is added.
- [x] The runtime lifecycle decision is consumed from commit `6094f08` and its
      sanitized transcript: preflight before registration, new-Workspace-only
      image promotion, existing-image reuse, and Bash shell identity.
- [x] Docker VM shared-path guidance belongs in the README prerequisites and
      bind-mount troubleshooting note because it blocks setup.
- [x] Architecture/publication artifacts remain separate from this first-use
      Quick Start; no such files are changed here.

## Catalog and prior-packet audit

- The documented path was checked against `internal/cli.Catalog`: `doctor`,
  `cluster up`, `tobari`, `policy review`, `policy allow --id`, `policy deny
  --id`, `runtime init`, `context show`, `runtime build`, `delete`, and
  `cluster down --purge` are existing paths. No retry alias or new input was
  introduced.
- Root and scoped agent help remain the source of detailed input/output
  contracts. The README uses `--name=default` only for the named Context
  example and states that omission selects the active Context.
- Compared with `quickstart-runtime-docs`, this packet changes only the
  host/Workspace ownership cue, valid post-allow re-entry, runtime preflight
  and new-Workspace effect, Bash expectation, Docker shared-path prerequisite,
  and cleanup ordering. The prior packet's synthetic `example.com` request and
  exact opaque-reference rule are preserved.

## Verification evidence

- Focused public validation passed: `task public:check` exited 0 with
  `repoguard (public): OK` and `contractlint: OK`.
- The focused CLI tests for the exact allow handoff and the real 120x40 PTY
  policy-review cases passed. The PTY suite covered delayed allow, delayed
  deny, cancellation, invalid selection, and redirected JSON read-only review;
  the opaque candidate remained unchanged and cancellation made zero policy
  calls.
- Exact catalog/help audit used the root index plus scoped `policy`, `runtime`,
  `context show`, `delete`, and `cluster down` selectors. This six-invocation
  audit is recorded separately from the documented new-user journey budget;
  the public journey itself retains at most three long-path lookups and two
  medium bootstrap/lifecycle lookups.
- `task runtime:base:check` exited 0 with `runtimecheck: OK`. The runtime
  profile reached policy 27/27 and Gateway 25/25, started healthy Gateway/OPA,
  a synthetic mock upstream, and two task-owned Workspaces, then blocked at
  the checked-in Python `pty.fork` wrapper running
  `bin/tobari policy review --tail 1000`. The child and wrapper produced no
  further output or exit after the review handoff; Ctrl-C stopped the task and
  the task reported exit status 130. The cleanup trap removed the task-owned
  containers, networks, volumes, and temporary integration directory. This is
  the known external policy-review PTY input handoff blocker, not a failure of
  the focused selector, runtime preflight, or catalog wording.
- Final repository gates: `task check`, `task public:check`, and `task security`
  exited 0. `task release:check` exited 1 before release assertions because
  the existing out-of-scope `scripts/test-integration.sh:238` triggers
  ShellCheck SC2183 and SC2016. No release script is changed in this packet;
  the warning is recorded as the release-profile blocker.
