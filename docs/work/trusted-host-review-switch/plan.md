# Work Plan: Switch from an attached Workspace to trusted-host review

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Treat the attachment as a narrow terminal multiplexer with one fixed host
prefix. In child mode, it forwards every byte except the exact prefix grammar.
`Ctrl+]` followed by `r` enters Trusted Host Review; repeating `Ctrl+]` sends
one literal prefix; `?` opens host shortcut help; any other second byte is
forwarded with the prefix unchanged. Trusted Host Review initially composes
only the existing Permission Inbox and canonical reviewed-set Apply. It is a
host presentation and input boundary, not a Workspace command or authority
channel.

Implementation starts with a bounded PTY experiment. Mechanism work proceeds
only if the experiment proves honest restoration and bounded handling of child
output for ordinary, raw, continuously writing, and alternate-screen children.
Failure of that experiment is a design result, not permission to weaken the
terminal contract or silently drop output.

## Alternatives considered

### Separate trusted-host terminal

This preserves the current direct Docker terminal ownership and remains the
safe fallback, but it is the friction this work intends to remove and does not
provide the shared review transition needed by future service requests.

### Workspace-invoked handoff helper

A dedicated Workspace helper could voluntarily block and ask the host to take
over the terminal. It cannot provide review while Claude, Codex, Vim, or
another foreground program owns the TTY, and a Workspace process cannot prove
that a human requested the handoff.

### Host browser or platform-native confirmation UI

A loopback page or OS dialog would avoid terminal multiplexing, but it adds a
second presentation platform, platform-specific behavior or a browser session,
and more authority than the existing terminal Permission Inbox needs.

## Design

### Public contract

- The existing root `tobari` command remains the only public attachment entry.
  No new host command path is required for the initial slice.
- Interactive root help declares one availability-limited keyboard workflow:
  press `Ctrl+]`, then `r` to open Trusted Host Review.
- The shortcut is active only when stdin and output are a supported interactive
  TTY. Redirected and JSON workflows retain zero interception.
- `Ctrl+]`, then `Ctrl+]` sends one literal prefix. Unknown two-byte sequences
  are forwarded unchanged. The grammar is byte-based, has no timing heuristic,
  and is covered across fragmented reads.
- Trusted Host Review opens on the installation-wide Permission Inbox and may
  show current counts by Context and Workspace. It does not infer a current-
  Workspace-only policy scope.
- The surface visibly states that Workspace input is paused. Back returns to
  the child without mutation. Permission staging and Apply retain the existing
  typed policy contracts and outputs.
- No new opaque reference kind is produced or consumed. Existing policy
  candidate references remain unchanged from the existing discover result to
  the existing fixed-target reviewed-set Apply.
- The root command continues to own the child session's final host exit status.
  A review transition cannot replace a nonzero child status with success.
- The capability is expected to be public after the PTY evidence and ADR are
  accepted. Until then, the work packet records it as pre-public and unshipped.

### Layer changes

- Domain: add only pure vocabulary and invariants needed to describe one host
  prefix transition and correlated child/review state. Do not move terminal
  bytes or policy presentation into domain.
- Application: compose the existing bounded Permission Inbox read and reviewed
  decision-set Apply while retaining their current ports, target validation,
  and receipts. Own the high-level attachment/review state transitions and
  child-exit correlation.
- Infrastructure: extend the reviewed Unix PTY/interactive adapter to detect
  only the fixed input grammar, isolate review input, preserve child output and
  terminal state, forward resize/signals in the active mode, and restore or
  fail honestly. Do not add a generic executor or network channel.
- CLI and catalog: render Trusted Host Review, reuse the existing Permission
  Inbox selector, compose catalog-owned actions, publish exact shortcut help,
  and keep the root catalog entry as the workflow source of truth.

### Data and control flow

```text
host keyboard
  -> attachment prefix decoder
     -> ordinary/literal/unknown bytes -> Workspace child PTY
     -> Ctrl+] then r -> trusted-host review state
          -> bounded Permission Inbox read
          -> stage typed IDs in host memory only
          -> optional confirmed canonical policy Apply
          -> restore child terminal and resume input routing

Workspace stdout/stderr
  -> child presentation path only
  -X-> prefix decoder
  -X-> host-review activation or confirmation
```

Child output received while review is active follows one bounded, tested
strategy selected by the PTY experiment. The implementation must state its
backpressure, memory, replay/redraw, alternate-screen, and child-exit behavior
in the ADR; “usually quiet during review” is not an acceptable assumption.

### Error and cancellation behavior

- Prefix parsing performs no side effect. EOF with a held prefix has an exact
  tested disposition and cannot invent a review transition.
- `b`, `q`, or review cancellation discards un-applied staging and returns to
  the child. Review-mode `Ctrl+C` is host review cancellation and is not sent
  to the child; child-mode signals remain ordinary child signals.
- A failed Permission Inbox read keeps policy unchanged and offers Back. A
  valid structured policy Apply fault remains authoritative.
- Once Apply starts, cancellation follows the existing mutation outcome
  classification. Unknown outcomes direct only to read-only reconciliation.
- If the child exits during review, no pending decision is auto-applied. The
  selected durable contract must preserve the child exit status, restore the
  host terminal exactly once, and close attachment-owned routes in the normal
  owner order.
- Terminal restoration failure is reported without claiming that the child or
  policy action did not occur.

### Security and public boundary

- Host input is the only prefix source. Workspace output and Workspace control
  channels cannot reach the decoder.
- The host-review header is explanatory presentation, not authority; authority
  remains the host-owned process, typed evidence, fresh validation, and
  canonical mutation boundary.
- The review transition grants no additional Workspace capability and exposes
  no host socket, credential, filesystem path, process executor, or Docker
  control to the Workspace.
- The browser channel remains a separate closed authorization-URL protocol.
  No generic attachment RPC or shared operation string is introduced.
- Tests use synthetic paths, effects, terminal bytes, and child processes and
  retain no real terminal transcript or credential.

## Implementation slices

1. Write the terminal-ownership ADR and revise the theses/product contract
   before mechanism code; add failing catalog and prefix-state tests.
2. Build the bounded PTY experiment and record go/no-go evidence for ordinary,
   raw, continuous-output, alternate-screen, resize, signal, and child-exit
   cases.
3. If the experiment passes, implement the infrastructure prefix and
   child/review state boundary with exact cleanup and restoration.
4. Compose the existing Permission Inbox and reviewed-set Apply inside the
   host surface; add human help and trusted presentation.
5. Promote the final terminal contract through architecture, security,
   harness, capability ledger, README, architecture site, and agent-readiness
   evidence.

## Verification

- Unit and contract tests: prefix DFA including fragmented input, literal and
  unknown sequences; catalog/help; existing policy review conformance; child
  result correlation.
- Negative side-effect tests: Workspace output cannot switch modes; review keys
  never reach the child; cancellation and failed reads perform zero mutation;
  unconfirmed staging grants nothing.
- Opaque-reference tests: existing policy candidate IDs survive review and
  fresh Apply unchanged; same-label replacement IDs remain undecided.
- Structured output, hostile-output, and recovery tests: hostile child and
  denial text cannot inject host controls; Apply faults retain existing exact
  recovery; no JSON variant is invented for the keyboard surface.
- Agent-readiness scenario and discovery-round-trip count: attached human
  denial-to-review needs the one documented prefix and no separate terminal or
  command discovery.
- Manual observation: supported macOS and Linux terminals, US and JIS layouts,
  Bash, a pinned agent CLI, and a full-screen synthetic or established TUI.
- Required profiles: focused Go/PTY tests, relevant Docker integration,
  `task check`, `task security`, and `task public:check`.
- Generated-diff or artifact checks: catalog-derived help, architecture site,
  capability ledger, and public documentation remain synchronized.

## Rollout and rollback

This is a pre-public contract change with no persisted-state migration. Safe
rollback removes the host prefix and review composition and restores direct
Docker terminal ownership. It must not leave a hidden input interceptor,
terminal configuration, socket, background process, or compatibility reader.
If the PTY experiment fails, the existing separate-terminal `policy review`
workflow remains the supported fallback and no partial shortcut ships.

## Documentation promotion

- Revise the North Star and Thesis 8 from “separate trusted-host terminal” to
  one trusted-host review surface reachable through the attachment prefix.
- Narrow the thesis statement that Tobari never intercepts child input to the
  exact fixed, literal-escapable host prefix exception.
- Add an ADR defining terminal ownership, input grammar, hidden-output handling,
  cleanup, restoration, and why browser/service channels do not control it.
- Update product command/help and output/exit contracts for the interactive
  attachment workflow.
- Update architecture and security models for the host review state and
  terminal authority.
- Add the executable PTY/security matrix to the harness and capability ledger.
