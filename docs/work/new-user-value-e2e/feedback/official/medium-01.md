# Medium-01: Permission Inbox by human keystrokes

- Status: functional E2E complete with one scenario deviation; raw transcript
  digest and per-key timing were not returned, so packet-wide evidence
  completeness remains open.
- Subject: fresh disposable `cc-bash-guard` snapshot with an isolated Tobari
  state and a real OS pseudo-TTY.
- Delegation: the child received only the desired outcome and sandbox
  boundary. It did not read repository documentation, source, work packets, or
  harness files, and it made no repository changes or commit.

## Value journey

1. The child followed the visible startup transition, entered a Workspace, and
   issued one request at a time from the interactive shell. The first probe was
   already allowed, so it did not count as a denial value signal.
2. The child discovered the human review surface and generated three additional
   independent synthetic requests. `example.net`, `example.org`, and
   `www.iana.org` each returned `policy_denied`, HTTP 403, the visible review
   command, and `automatic_retry:false`.
3. The Permission Inbox displayed the candidates. The child inspected one,
   used `a` followed by `y`, and observed the exact permission become allowed.
4. The queue refreshed. The child inspected a second candidate, used `d`
   followed by `y`, and observed the exact permission become denied.
5. The child canceled the remaining queue with `q`, reopened/rechecked it, and
   confirmed that exactly one candidate remained pending rather than being
   silently allowed.
6. The child deleted the Workspace and stopped the shared environment. The
   visible final state was `No Tobari for this directory` and `Cluster not
   configured`; no scenario-owned containers remained.

## Deviation and discovery

The intended three-denial setup was not available to the blind user as a
pre-generated state. The child first tried an already-allowed request, then
adapted by creating a fourth request so that three denied candidates existed.
This is a useful first-time-user finding: a value scenario that requires a
particular queue shape must either seed that state outside the user journey or
make the setup outcome discoverable.

The child used the visible product/help path to discover policy review. It also
guessed `tobari` and `go` inside the Workspace; both were unavailable. Counted
discovery was at least one explicit help lookup, plus those two failed guesses.
No README, source, JSON, direct Docker/OPA, or non-interactive policy path was
used as the human success route.

## Readable checkpoints

| Checkpoint | Visible result | Interpretation |
|---|---|---|
| First denied request | `policy_denied`, `403`, review command, `automatic_retry:false` | The boundary and human owner are visible. |
| Inbox list | Three exact pending requests | Multiple decisions can be held in one queue. |
| Allow | `a` → `y`; permission allowed | One exact request is deliberately permitted. |
| Deny | `d` → `y`; permission denied | One exact request is deliberately rejected. |
| Cancel | `q`; one pending candidate remains | Cancel is distinct from deny and does not mutate policy. |
| Cleanup | `No Tobari for this directory`; `Cluster not configured` | Workspace and shared state are removed. |

The child verified a real pseudo-TTY with `TERM=xterm-256color` and a
120x40 terminal (`stty rows 40 cols 120`). It did not provide a raw-byte
capture digest or timestamped key transcript, so these are functional
checkpoints rather than complete packet-level PTY evidence.

## Friction and classification

### Confirmation timeout is a product/contract defect

Pressing `a` without immediately entering `y` caused
`undeclared_fault_contract`. The child recovered by entering `a` and `y` as a
short sequence, and no unintended policy change was observed.

Candidate disposition: **keep** the explicit confirmation and exact
allow/deny semantics, but route the timeout behavior to a focused PTY contract
fix. A normal human pause must not become an undeclared command failure.

### Inbox redraw is a presentation defect

The child reported very frequent TUI redraws that made the screen difficult to
read while selecting and confirming. The underlying list/detail/decision/queue
refresh model remained understandable after observation.

Candidate disposition: **keep** the Permission Inbox as the primary human
surface; create a presentation-focused follow-up rather than replacing it with
raw policy commands.

### Workspace boundary needs an explicit product/docs decision

The child tried `tobari` inside the Workspace and could not find it, then used
the visible host-side help/recovery path. This confirms the host/Workspace
boundary exists, but the first-time user must infer which actions belong on the
host.

Candidate disposition: **docs-only or integrate a bounded host-action cue**;
do not add an unrestricted in-Workspace control path. The existing catalog
separates the host lifecycle and policy commands from the Workspace shell, so
any new entry must preserve that boundary and be reviewed against the product
contract.

### Cleanup commands are distinct ownership boundaries

The child suggested a combined cleanup command because Workspace deletion and
shared-cluster shutdown are separate steps. This run also showed that the
separate commands are understandable once their final statuses are visible.

Candidate disposition: **keep** the separate project-owned delete and
shared-cluster down operations for now; consider a docs-only workflow or a
bounded convenience candidate after command-catalog review. No deprecation is
proven by this scenario.

## Acceptance boundary

The child independently reached the requested three deliberate Inbox outcomes
and clean shutdown through a real TTY, with one additional denied request
needed because the first probe was already allowed. The scenario is therefore
functionally complete but retains a setup/discoverability deviation and the
missing raw PTY evidence required by the parent packet.
