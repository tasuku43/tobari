# Work Plan: New-user value journeys through a real pseudo-TTY

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

The parent defines four deterministic journeys before delegation. Each child
agent acts as a first-time maintainer/user in a fresh synthetic root, uses a
real pseudo-TTY with human-paced input, captures raw and readable terminal
evidence, and returns only observed behavior and bounded feedback. The parent
writes each result into this packet's `feedback/` directory, then compares the
journeys before opening any implementation packet.

The four journeys are:

1. Long: safe first success from bootstrap through denied-request recovery and
   custom runtime entry.
2. Long: least-privilege learning across two project roots, including exact
   allow/deny scope, cancellation, and reversible deletion.
3. Medium: human Permission Inbox interaction with allow, deny, cancel, and
   queue refresh through actual keystrokes.
4. Medium: first bootstrap failure, Workspace create/reuse/cancel, and cleanup,
   with command discovery and interface-simplicity review.

Runs are sequential because cluster, Gateway, OPA, Docker network, and Context
state are shared resources even when project roots differ. A scenario may use
the existing integration harness for bounded setup or cleanup, but it must
still perform the user-facing path itself through the pseudo-TTY.

## Alternatives considered

### Alternative A: Use only `task integration:test`

Rejected as the sole evidence. The existing harness is valuable contract and
integration coverage, but a streaming test helper cannot reveal whether a new
user understands the first screen, prompt ownership, redraw, key bindings, or
the next command. It remains supporting evidence, not human success.

### Alternative B: Run all four agents concurrently

Rejected because concurrent Gateway/OPA and Docker lifecycle mutations can
cross project or cluster state and make a human finding indistinguishable from
an environment race. Agents run in a controlled sequence with cleanup between
scenarios.

### Alternative C: Judge command usefulness by static names

Rejected because a command may be redundant in one journey but be the exact
recovery or machine-readable boundary in another. Every surface candidate must
include observed user friction and catalog/reference/recovery impact.

## Design

### Public contract

This investigation adds no public capability. The tested commands retain their
existing roles:

- `doctor` and `cluster up` are host preparation;
- `tobari` is fixed current-directory Workspace entry;
- `policy review` is human discovery/decision presentation on a TTY;
- `policy allow --id` and `policy deny --id` are exact opaque-reference actions;
- `runtime init` and `runtime build` are explicit host-side Context runtime
  actions; and
- `delete` and `cluster down --purge` are lifecycle cleanup.

Scenario agents may use scoped help to discover these paths. JSON review may
serve as a post-hoc semantic cross-check, but it cannot replace the human TTY
path or count as its success.

### Layer changes

- Domain: none.
- Application: none.
- Infrastructure: none beyond temporary test resources owned by the harness.
- CLI/catalog: none; command-surface observations are evidence only.
- Work packet: parent-authored scenarios and redacted feedback only.

### Data and control flow

```text
fresh project root
  -> real pty host setup and Workspace entry
  -> real pty agent work and denied/effective request
  -> real pty host review or lifecycle recovery
  -> visible value signal and exact cleanup
  -> parent feedback record
  -> cross-scenario command/product findings
```

The PTY runner must preserve terminal behavior: raw mode, cursor movement,
clear-line/screen sequences, prompt timing, echoed input, and exit status. A
normalized transcript is for reading; the raw capture/digest is the evidence
that the normalization did not hide a rendering failure.

### Error and cancellation behavior

Each scenario records the first failure without immediately improvising. The
agent may follow an explicitly surfaced next command, help path, or README
recovery once. Cancellation must be intentional and visible: `q`, Escape,
Ctrl-C, EOF, or `exit` is recorded with its owner, output, and whether state
changed. A failed cleanup is a scenario failure and remains in feedback until
resolved or explicitly handed off.

### Security and public boundary

No credentials or private data enter the packet. Temporary raw captures stay
outside Git. Feedback uses synthetic values and repository-relative references.
Direct Docker/OPA commands are not a supported user step; they are allowed only
for bounded cleanup or after the user path has been stopped and the exact
environment blocker must be diagnosed.

## Implementation slices

1. Parent-authored goal/context/plan/tasks and four scenario definitions.
2. Sequential pseudo-TTY execution by four Luna-max agents.
3. Parent recording of one redacted feedback file per result.
4. Cross-scenario comparison of value signals, blockers, and command-surface
   candidates.
5. Repository/public gate verification and scoped evidence commit.

## Verification

- Human-path E2E: all four scenario journeys are attempted with real PTY
  allocation, paced keystrokes, raw capture, screen checkpoints, and cleanup.
- Negative/recovery E2E: denied requests, cancel/back/invalid input, startup
  failure, workspace reuse, exact scope, and deletion are recorded where the
  scenario includes them.
- Interface review: every candidate names the observed command sequence,
  catalog path(s), role/effect/reference flow, and whether integration/narrowing
  would risk recovery or compatibility.
- Discovery budget: each feedback file records help/doc lookups and any
  non-routine processing; routine success must not require source inspection or
  exploratory calls.
- Repository gates: `git diff --check`, `task check`, and `task public:check`;
  run security/release/runtime/integration profiles when their boundaries are
  touched and record blockers rather than weakening them.
- Artifact check: only the work packet changes; raw host-specific transcripts
  are excluded from Git.

## Rollout and rollback

Not applicable to product behavior. This packet is evidence-only. Follow-up
implementation, docs, or command-surface changes require a new bounded packet
and their own E2E/rollback plan.

## Documentation promotion

If multiple journeys show the same trust-boundary confusion, missing recovery,
or command-integrity issue, promote the conclusion to the appropriate thesis,
product, architecture, security, harness, or ADR before implementing a local
exception. Command retirement/integration candidates require a catalog and
compatibility review packet.
