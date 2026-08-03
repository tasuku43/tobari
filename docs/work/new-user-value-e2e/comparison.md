# Cross-scenario comparison

- Status: functional comparison complete; product and documentation changes
  are not authorized by this evidence packet.
- Evaluation rule: a journey counts as functional E2E only when the child
  reached its declared value signal through a real human-paced pseudo-TTY and
  bounded cleanup. Parent baselines establish that the underlying path exists;
  they do not replace blind child evidence.

## Scenario matrix

| Journey | Functional result | Strongest value signal | Repeated or new friction |
|---|---|---|---|
| [Long-01](feedback/official/long-01.md) | Complete, with earlier blocked/partial attempts retained | Exact denied request approved, retried upstream, custom runtime visible after fresh Workspace | Confirmation fault; unknown `tobari retry`; runtime changes require recreation; `I have no name!` prompt |
| [Long-02](feedback/official/long-02.md) | Complete on rerun | A-only permission succeeds while identical B request remains denied; A deletion leaves B | Confirmation fault; exact project scope is understandable but raw evidence incomplete |
| [Medium-01](feedback/official/medium-01.md) | Complete with setup deviation | Inbox allow/deny/cancel leaves one candidate pending | Confirmation fault; frequent redraw; host/Workspace command-boundary confusion; first probe already allowed |
| [Medium-02](feedback/official/medium-02.md) | Complete on rerun | Recovery, same-root reuse, nested cancel without mutation, bounded cleanup | Selector cancel is styled as `Command failed`; unrelated shell warning; cleanup ownership needs explanation |

All four child runs reported real PTYs, 120x40 dimensions, and
`TERM=xterm-256color`. None returned a raw-byte digest or complete timestamped
screen-checkpoint artifact. The functional E2E predicate is satisfied, but the
packet's stronger raw-evidence acceptance criterion remains open and must not
be silently claimed complete.

## Repeated findings

### 1. Permission confirmation is not stable at human speed

The parent baseline, Long-01, Long-02, and Medium-01 independently reproduced
the same behavior: pressing `a` or `d` and pausing before `y` can terminate the
TTY review with `undeclared_fault_contract`. Immediate short input succeeds and
the candidate remains safe, but a normal human pause is treated as an
undeclared failure.

Classification: product/contract defect at the interactive output boundary.

Disposition: open a focused implementation packet for a persistent,
interruptible confirmation state, preserving explicit exact allow/deny
semantics and real cancellation. Its acceptance function must include a
human-length pause and a raw PTY regression.

### 2. Recovery guidance must name only executable next actions

Long-01 saw a visible `tobari retry` suggestion that was not a valid command;
the child recovered through the visible `Resume: tobari` path. The denial
response's `automatic_retry:false` and host review cue were otherwise clear.

Classification: product/catalog/help contract mismatch.

Disposition: align the recovery output with a catalog path that actually
exists, or change the message to the valid host re-entry action. Do not add a
generic retry alias without deciding whether it is a public capability or just
a workflow label.

### 3. The host/Workspace boundary is real but partly implicit

The denied response identifies a host-owned review action. Long-01 and
Medium-01 nevertheless explored `tobari` from inside the Workspace and had to
infer that review/recovery belongs on the host. This is a trust-boundary
teaching problem, not evidence that an unrestricted in-Workspace CLI should be
added.

Classification: documentation/presentation, with a possible bounded product
cue.

Disposition: improve the denial/exit handoff and Quick Start wording first;
consider a host-only cue only after the product contract confirms the boundary.

### 4. Runtime lifecycle semantics need one explicit decision

The parent baseline and Long-01 showed two related facts: registering before a
runtime is ready can leave an instance requiring delete/re-registration, and a
later `runtime build` does not replace the image of an existing Workspace.

Classification: product lifecycle contract plus documentation gap.

Disposition: decide whether runtime preparation is a hard precondition or
whether a safe reconcile/refresh transition should exist. Until then, document
that runtime build applies to newly created Workspaces and keep the current
separate lifecycle boundary.

### 5. The Permission Inbox model is valuable; its presentation needs polish

The parent baseline and Medium-01 show that list → detail → explicit action →
confirmation → refresh → cancel is a coherent human task. Medium-01 also
reported frequent redraws that made the screen hard to read. The selector in
Medium-02, by contrast, clearly separated an existing nearest ancestor from
creating a Workspace here.

Classification: presentation friction, not command-surface redundancy.

Disposition: keep the Inbox and nested selector as distinct surfaces. Improve
redraw stability and distinguish intentional cancellation from operational
failure before considering command consolidation.

## Command-surface disposition

| Surface | Evidence | Disposition |
|---|---|---|
| `doctor`, `cluster up`, `cluster status` | Bootstrap recovery was visible and independently discoverable | Keep; Quick Start should show ownership/order |
| `tobari` fixed current-directory entry | Root reuse is simple; nested selector exposes scope | Keep; preserve fixed-target semantics |
| `policy review` | Coherent human Inbox; exact scope is visible | Keep as primary human decision surface |
| `policy allow/deny --id` | Exact opaque-reference actions remain useful after discovery | Keep as explicit narrow/machine path |
| `tobari retry` wording | Long-01 saw an unavailable command | Integrate/align recovery text; do not yet add a command |
| `status`, `list`, `delete` | Lifecycle state and cleanup were observable | Keep; clarify ownership and destructive effect |
| `cluster down --purge` | Shared-state cleanup is distinct and idempotent | Keep; document safe no-op after last deletion |
| `runtime init`, `runtime build`, Context inspection | Runtime customization is a separate host concern | Keep; document prerequisite and new-Workspace effect |
| `context show` grammar | `--name=default` was needed during discovery | Docs-only clarification candidate |

No journey provides evidence to deprecate a command. The repeated request to
combine cleanup is a convenience hypothesis only; merging project deletion and
shared-cluster shutdown could obscure ownership and should not be done from
these observations alone.

## Design directions for successor work

These are inspectable directions, not an implementation choice for this
packet. A successor packet should choose one after product-contract review.

### Direction A — Persistent exact confirmation

Keep the current Inbox screens and actions, but hold the confirmation state
until `y`, explicit cancel, or terminal interruption. This minimizes catalog
change and directly addresses the repeated defect.

### Direction B — Guided decision loop

Keep exact decisions but make the next state a stable inline panel with an
explicit default/cancel affordance and a visible queue count. This improves
readability and redraw behavior at the cost of a larger presentation contract.

### Direction C — Host handoff-first workflow

Keep review on the host, but make every denial and Workspace exit render one
valid host recovery action and a short explanation of why the action cannot be
performed inside the Workspace. This addresses the trust-boundary confusion
without adding an in-Workspace control path.

## Successor packet candidates

1. [`policy-review-tty`](../policy-review-tty/goal.md): confirmation timing,
   cancellation, redraw, raw-PTY regression, and valid recovery output.
2. [`runtime-lifecycle-reconcile`](../runtime-lifecycle-reconcile/goal.md):
   missing-runtime recovery and whether build affects existing Workspaces;
   update product/architecture/docs together.
3. [`new-user-quickstart-handoff`](../new-user-quickstart-handoff/goal.md): Quick
   Start, host/Workspace boundary, valid retry/re-entry wording, runtime
   prerequisites, and cleanup ownership.
4. [`cli-catalog-audit`](../cli-catalog-audit/goal.md): consume the keep/integrate/
   narrow classifications; no command retirement is justified by these
   journeys yet.
5. [`pty-evidence-harness`](../pty-evidence-harness/goal.md): add a parent-owned
   capture/digest/checkpoint path so future child feedback can satisfy the raw
   transcript predicate without exposing host-specific data.

These candidates are intentionally not created or implemented here. They are
the explicit handoff from evidence to reviewed product work.
