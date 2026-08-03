# Parent-authored new-user value scenarios

The parent agent owns these definitions. Child agents execute them as written,
record deviations, and return feedback; they must not redesign a scenario while
running it. A deviation is itself evidence.

## Common pseudo-TTY protocol

Every scenario uses a real OS pseudo-terminal for both host-side interactive
commands and the `tobari` Workspace session:

1. Allocate a pty with `pty.fork` or an equivalent tool, set `TERM=xterm-256color`
   and a 120x40 terminal size, and attach stdin/stdout/stderr to the pty.
2. Send one command or key at a time. Use human-paced delays (roughly 100–500
   ms between ordinary keystrokes and longer pauses after a redraw, build, or
   network response). Wait for a visible prompt or stable screen before the
   next action; do not feed a complete input script at once.
3. Capture raw bytes including ANSI cursor/clear sequences, a timestamped
   readable projection, every typed key including arrows/Escape/Ctrl-C, screen
   checkpoints after redraws, and process exit status. A raw capture may remain
   outside Git; commit only a redacted projection and digest.
4. Start as a new user: read the current README or exact scoped help, but do
   not inspect source or integration assertions before the first attempt. Use
   only the visible next action or one exact recovery command after a failure.
5. Do not treat JSON, redirected output, direct Docker/OPA calls, or a
   non-interactive stdin replay as a human-path success. They may be used after
   the attempted human path to explain semantics or diagnose an environment
   blocker, and must be labeled separately.
6. Record the discovery-round-trip count: each help/README lookup or exact
   recovery lookup needed to continue. Record guessed commands, backtracks,
   wrong keys, and unrecognized prompts separately.

For all scenarios use synthetic project roots and public-safe endpoints. Clean
up through the supported CLI. If an environment blocker prevents safe cleanup,
stop and report it rather than deleting broad directories or using an
unbounded Docker cleanup.

## Long scenario 1 — Safe first success: denial to customized runtime

### Value hypothesis

A new user experiences the complete Tobari value in one sitting: a project is
isolated, a denied network operation explains the trusted-host handoff, one
exact permission is approved, the same request succeeds without broadening,
and the user can explicitly customize and reuse the runtime.

### Starting state

- A fresh disposable copy of the maintainer-supplied `cc-bash-guard` project,
  no Workspace, no active Context recipe, and no pre-existing scenario-owned
  cluster state.
- The user has the current Quick Start but no source or harness knowledge.

### Human journey

1. In a host pty, run `tobari doctor --root .`; follow the visible preparation
   guidance and run `tobari cluster up`.
2. Run `tobari` in the host pty. Select/create the current directory using the
   key the screen explains (`n`, Enter, arrows, or the fallback it presents).
   Inside the Workspace, run `pwd`, inspect the shell prompt, and issue the
   synthetic request:

   ```sh
   curl -sS -w '\\nhttp=%{http_code}\\n' -X PUT https://example.com/quickstart
   ```

3. Read the denial as a human. Record whether it visibly names `403`, the
   secret-free `policy_denied` reason, `tobari policy review`, and the fact that
   retry is not automatic. Type `exit` and note the host session summary.
4. On the host, run `tobari policy review --tail 100` in a pty. Select the
   displayed `example.com`/`PUT`/`/quickstart` candidate by observing the
   screen, press Enter to inspect, press `a`, then press `y` only after the
   confirmation is visible. Record all redraws and whether the exact action is
   understandable without JSON.
5. Re-enter `tobari`, repeat the same curl, confirm that the response is an
   upstream response rather than Tobari's denial handoff, and exit.
6. On the host, run `tobari runtime init`, inspect `tobari context show`, edit
   the active Context Dockerfile to add only harmless `tree`, then run
   `tobari runtime build`. Enter `tobari` again and verify `tree --version`,
   `pwd`, and the interactive shell; exit.
7. Run `tobari delete` and `tobari cluster down --purge` in ptys. Verify through
   supported status commands that the Workspace and owned cluster state are
   gone.

### Success signal

The user can explain the host/agent boundary, intentionally approve one exact
request, observe the same request succeed without an automatic retry, enter a
customized reusable runtime, and clean up without source inspection or a
guessed command.

### Failure and observation points

Stop and record if bootstrap, selection, denial interpretation, review
selection/confirmation, retry, runtime editing/build, re-entry, or cleanup
requires an undocumented command, a non-TTY workaround, or direct Docker.

### Discovery budget

At most three lookups after the first screen: one bootstrap/recovery lookup,
one policy-review/help lookup, and one runtime/cleanup lookup. Extra lookups are
feedback, not silent recovery.

## Long scenario 2 — Least privilege across two project roots

### Value hypothesis

A new user understands that Tobari learns the smallest exact permission for one
project and does not silently authorize another project, child path, or method.
The user can deny or cancel safely and delete one project without destroying the
other project's isolation or shared profile state.

### Starting state

- Two fresh disposable copies of the maintainer-supplied `cc-bash-guard`
  project under one temporary parent, no project Workspaces, and no
  scenario-owned cluster state.
- The user knows only the public onboarding and policy-review instructions.

### Human journey

1. In a host pty, run the documented doctor/cluster preparation. Enter project A
   and project B in separate ptys, using the visible Workspace choices.
2. In each Workspace, issue the same synthetic `PUT` request to
   `https://example.com/shared` and a distinguishable path request such as
   `/shared/child`; record both denials and exit each session.
3. On the host, open the TTY Permission Inbox. Determine whether the screen
   exposes enough project/scope context to select project A's exact candidate.
   If it does, allow only A's `/shared` candidate through Enter → `a` → `y`.
   If it does not, stop the human attempt at that ambiguity and record it; do
   not use JSON to falsely count the selection as human success.
4. Re-enter both projects and retry `/shared`; A should be allowed while B
   remains denied. Retry A's `/shared/child`; it must remain denied. Record the
   first visible proof of project and path isolation.
5. Produce a new candidate in B, open the human review queue, inspect it, and
   deny it with `d` → `y`. Produce another candidate and cancel/back out with
   `q` or Escape. Verify that cancellation leaves the candidate and policy
   unchanged, then reopen the queue and complete the intended decision.
6. Delete project A through `tobari delete` and verify through supported status/
   list commands that B, its policy scope, and the shared profile remain. Then
   delete B and purge the cluster.

### Success signal

The user can see which project and exact request a decision affects, approve
only A's request, observe B and the child path remain blocked, distinguish
deny/cancel from allow, and reverse project lifecycle without broad cleanup.

### Failure and observation points

Pay special attention to missing project identity in the Inbox, ambiguous
candidate ordering, queue refresh after a decision, commands that duplicate
status/list/denial discovery, and cleanup commands whose ownership is unclear.

### Discovery budget

At most four lookups: bootstrap, Workspace selection, policy review, and
project cleanup. Any need to obtain project identity or exact scope from JSON
before a human decision is a finding.

## Medium scenario 1 — Permission Inbox by human keystrokes

### Value hypothesis

A new user can learn the safe permission loop directly from the TTY: inspect
one request, allow one, deny one, cancel one, and understand the queue refresh
without editing policy or memorizing a hidden key sequence.

### Starting state

- A fresh disposable `cc-bash-guard` project copy and ready cluster, entered
  through a real pty.
- Three synthetic learnable denied requests already generated by the user in
  the Workspace: `/inbox/allow`, `/inbox/deny`, and `/inbox/cancel`.

### Human journey

1. Exit to the host and start `tobari policy review` in a pty with no JSON.
2. Observe the list, use the shown arrow/number binding to select
   `/inbox/allow`, press Enter, inspect the exact host/port/method/path, press
   `a`, and confirm with `y`.
3. Verify the queue visibly refreshes. Select `/inbox/deny`, inspect, press
   `d`, and confirm with `y`. Reopen or continue until the queue shows only the
   remaining candidate.
4. Select `/inbox/cancel`, press `q` or Escape at detail and at the list, then
   reopen the queue. Confirm no mutation occurred and the candidate remains.
5. After the human path, use `policy review --format json` only as a semantic
   cross-check of queue state, then clean up the Workspace and cluster.

### Success signal

The visible screen teaches the complete decision loop; all three outcomes are
deliberate and distinguishable, the opaque ID is never guessed, and cancellation
does not mutate policy.

### Failure and observation points

Record blank screens, redraw artifacts, key bindings that are not visible,
confirmation wording that is easy to misread, queue refresh confusion, or any
need to fall back to JSON/source inspection.

### Discovery budget

At most two lookups: one policy-review help/readme lookup and one cleanup
lookup. The screen itself must provide the interaction grammar.

## Medium scenario 2 — Bootstrap, Workspace reuse, cancellation, and cleanup

### Value hypothesis

A new user can recover from an intentionally premature `tobari` invocation,
create an isolated Workspace, leave and reuse it, cancel a selection safely,
and delete only what they intend without learning several lifecycle commands by
trial and error.

### Starting state

- A fresh disposable `cc-bash-guard` project copy and no scenario-owned
  cluster state.
- The user begins with the primary `tobari` command, not with hidden setup
  knowledge.

### Human journey

1. In a host pty, run `tobari` before cluster setup. Read the failure and follow
   only the visible next action or the README recovery once.
2. Run `tobari doctor --root .` and `tobari cluster up` in ptys. Run `tobari`
   again, create the Workspace using the visible create-here key, run `pwd`
   and a harmless `printf`, then `exit`.
3. Re-run `tobari` from the same directory. Reuse the existing Workspace using
   the displayed selection, verify the same mirrored path, and exit. Record
   whether the UI explains that exit preserves the Workspace.
4. From a nested synthetic directory, invoke `tobari` and cancel the selector
   with `q` or Escape. Verify no Workspace was created and the cancellation
   output is understandable.
5. On the host, use `status`/`list` only as the UI directs, run `tobari delete`,
   verify status, and run `tobari cluster down --purge`. Record whether `delete`
   versus `cluster down` ownership is clear.

### Success signal

The user reaches an isolated shell, understands reusable versus deleted state,
can cancel safely, and completes cleanup with the documented commands only.

### Failure and observation points

Record the first-command failure guidance, selector labels, `exit` versus
`delete` versus `cluster down` distinctions, repeated `status`/`list` work,
and any command that appears to be a second name for the same lifecycle action.

### Discovery budget

At most two lookups: one bootstrap recovery lookup and one lifecycle cleanup
lookup. The Workspace selector must explain its own keys.

## Cross-scenario feedback rubric

For every observation, classify:

- `product`: the supported outcome or invariant is missing or wrong;
- `environment`: Docker/Colima/Gateway/PTY prevented a meaningful attempt;
- `docs`: the behavior exists but the user cannot discover the route;
- `presentation`: the route is present but screen/timing/wording obscures it;
- `surface`: a command/flag appears redundant, too broad, fragmented, or a
  candidate for integration/narrowing/deprecation; this requires catalog,
  compatibility, reference-flow, effect, and recovery analysis before action.

Surface feedback must state the observed user sequence, commands considered,
why a simpler integration or narrower interface might help, and what existing
workflow would break if a command were removed. Do not turn a surface
observation into a change in this packet.
