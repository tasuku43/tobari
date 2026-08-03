# Work Context: New-user value journeys through a real pseudo-TTY

This packet records observed human-path evidence. It does not redefine the
current CLI contract and it must not turn a blocked environment run into a
feature-completion claim.

## Current behavior

- The public onboarding path begins with `tobari doctor --root .`, explicit
  `tobari cluster up`, and a CWD-owned `tobari` Workspace entry. Ordinary
  Workspace entry requires a TTY and does not repair the shared cluster.
- A learnable denied HTTP request returns secret-free `policy_denied` evidence
  and the fixed host-side navigation `tobari policy review`; it does not return
  a candidate ID or retry automatically.
- Host `tobari policy review` uses a TTY Permission Inbox for bounded selection,
  exact request inspection, explicit `a`/`d` action, and `y` confirmation. A
  redirected or JSON review is read-only; exact `policy allow --id` and
  `policy deny --id` consume one opaque reference unchanged.
- `runtime init`, editing the active Context Dockerfile, and explicit
  `runtime build` are the supported runtime-customization path. A successful
  build promotes a compatible image; a failed build leaves the prior selection.
- `exit` leaves a reusable Workspace. `tobari delete` removes the selected
  project instance, while `tobari cluster down --purge` removes shared cluster
  state when no project remains.
- The existing integration harness has a Python `pty.fork` helper and exercises
  policy review and runtime entry, but its streaming helper is not itself a
  substitute for a first-time human transcript. This packet adds paced,
  screen-aware observation on top of the existing supported commands.
- The official subject is a clean tracked snapshot of the maintainer-supplied
  `cc-bash-guard` project at revision `e045d15`. The parent creates disposable
  copies for each run; the source project and its working tree are never used
  as a scenario mount.
- The parent baseline completed the core denial-to-review-to-retry loop and a
  runtime customization replay through a real PTY. Its redacted evidence is in
  [feedback/official/parent-baseline.md](feedback/official/parent-baseline.md);
  it is an entry criterion and does not count as one of the four blind child
  journeys. The baseline found three hypotheses for child comparison: a
  missing runtime can leave a newly registered instance unrecoverable without
  deletion/re-registration, raw review confirmation is sensitive to a human
  pause after `a`, and `runtime build` does not refresh an existing Workspace.

## Relevant structure

- Parent scenario definitions: [scenarios.md](scenarios.md).
- Per-scenario observations: [feedback/](feedback/README.md).
- Public onboarding: `README.md`, especially Quick Start, policy recovery,
  runtime customization, and failure/recovery sections.
- Catalog source of truth: `internal/cli.Catalog` and its catalog-backed help;
  command-surface findings must be checked against roles, references, effects,
  recovery, and compatibility before becoming a follow-up.
- Existing E2E harness: `scripts/test-integration.sh`, `task integration:test`,
  `task runtime:test`, and focused policy/Gateway/runtime profiles.

## Constraints

- Each official scenario uses a fresh disposable copy of the subject project
  and a single sequential cluster session unless its own goal explicitly tests
  two project roots. The subject copy is preparation, not a usage manual.
- A real pseudo-TTY is mandatory for host and Workspace interactions. Set a
  deterministic terminal size (120 columns by 40 rows unless the current
  terminal cannot support it), use `TERM=xterm-256color`, attach stdin/stdout/
  stderr to the pty, and capture raw bytes including ANSI control sequences.
- Input is human-paced: send one key or short command at a time, wait for the
  visible prompt or stable screen, and use realistic pauses. Do not pipe an
  entire transcript into a process or call a successful non-TTY replay a human
  success.
- Record both the raw transcript and a readable projection. At every redraw,
  retain a screen checkpoint with a monotonic timestamp, typed key/escape
  sequence, visible lines, and process state. Redact paths and identifiers that
  could identify the host.
- During the first attempt, agents may use README/help and the recovery command
  explicitly shown by the product. They must not inspect source, decode a
  provider-specific identifier, call exploratory endpoints, or use direct
  Docker/OPA commands to bypass a missing product transition.
- If setup fails because of Docker/Colima/Gateway state, record the exact
  failure and stop at the boundary; cleanup may use the supported CLI and the
  bounded harness cleanup.

## Unknowns

- [ ] Can a new user recognize the host/agent boundary and the next owner of an
      action from the first denial without consulting source or JSON?
- [ ] Does the raw TTY Permission Inbox make selection, detail inspection,
      allow/deny, confirmation, cancel, and refresh understandable at human
      typing speed?
- [ ] Does a multi-project user have enough visible scope to select the right
      exact permission without confusing project-local state?
- [ ] Which startup, entry, status, policy, runtime, and cleanup commands feel
      duplicated or fragmented when followed as a first-time user?
- [ ] Which observed friction is a documentation/presentation problem versus a
      missing capability or a command that should be integrated or narrowed?
- [ ] Can the current environment complete the four journeys without an
      unrelated existing PTY/integration blocker?

## Observation protocol

Delegation mode is part of the evidence. The first three attempts in this
packet were protocol-v1 pilots: one guided run and two blind runs, all of which
were incomplete or lacked a final handoff. They are retained as pilot evidence
and do not satisfy the official acceptance criteria. The official re-run starts
all four journeys from fresh project copies. Each child receives only a user
goal, a fresh environment boundary, safety constraints, progress-reporting
expectations, and the value signal. It must not receive the scenario's command
sequence, README/thesis/work-packet text, source, or harness usage
instructions. The child does not edit the repository; the parent writes and
commits the returned feedback. Official findings therefore measure self-
discovery rather than the ability to follow a prepared route.

At the start, after bootstrap, after the first value signal, at any meaningful
blocker, and before cleanup, the child sends a short status update to the
parent while continuing the run. A status request is answered before the
parent decides whether to stop or continue; elapsed time alone is not a
failure criterion.

Each official feedback file must include:

1. Environment and scenario ID, with synthetic/redacted project names.
2. The first-attempt command/key sequence and discovery-round-trip count.
3. Raw-PTY capture location or digest plus readable screen checkpoints.
4. Time to first meaningful value and the exact visible success signal.
5. Every pause, backtrack, guessed command, confusing label, and recovery.
6. Product/environment/docs/presentation classification for each blocker.
7. Candidate command-surface disposition: keep, integrate, narrow, deprecate
   candidate, or docs-only, with catalog/recovery compatibility evidence.
8. Cleanup and final process/cluster state.

Pilot files in the feedback root are historical protocol-v1 records. Official
files live under `feedback/official/` and are the only files counted toward
the four-scenario acceptance criteria.

## Security and public-boundary notes

- Use `example.com` or the repository's synthetic `mock-upstream` only. Do not
  include real credentials, private endpoints, authorization headers, host
  paths, usernames, or shell-history captures.
- The only intended side effects are temporary project/Context state, Docker
  resources owned by the scenario, Gateway/OPA policy learning, and cleanup.
  No production repository file is edited by a scenario runner.
- Opaque policy IDs are treated as secret-free but still copied exactly within
  the transcript; feedback may replace them with stable placeholders after
  recording their kind and round-trip behavior.
- No external dependency or publication artifact is introduced by this
  investigation. Any raw PTY capture containing machine-specific data is kept
  outside the repository and represented by a digest plus redacted projection.

## Glossary

- **Human success:** a new user reaches the declared value signal through the
  visible TTY path using only documented/helped commands and exact inputs.
- **Pseudo-TTY transcript:** raw terminal bytes plus time-ordered screen
  checkpoints and typed input, not merely stdout from a piped process.
- **Value signal:** the first observable proof that Tobari made the user's work
  safer or easier, such as entering the isolated project, recovering one exact
  denied request, or reusing a customized runtime.
- **Command-surface candidate:** an observation about keeping, integrating,
  narrowing, deprecating, or documenting a command; it is not an approved
  catalog change.
