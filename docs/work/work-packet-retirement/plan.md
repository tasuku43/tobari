# Work Plan: Audit and safely retire completed work packets

- Status: Complete
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Audit the filesystem and packet metadata first, then apply one conservative
decision procedure to every non-template packet. Treat an unchecked acceptance
criterion, open task, missing E2E journey, missing durable conclusion or
successor, broken reference, or dirty-work dependency as a preservation reason.
Use the coordinator's explicit auth-broker register row as the deferred case;
do not manufacture a standalone packet for it. Run the repository's reference
and completion gates after the audit packet is written. Delete nothing unless
the procedure identifies an exact completed temporary target.

## Alternatives considered

### Alternative A: Delete packets with mostly checked task lists

Rejected. Gateway and image-distribution packets demonstrate why focused
implementation and integration evidence can coexist with open publication,
license, or handoff obligations.

### Alternative B: Mark all packets complete after the audit

Rejected. The audit records lifecycle state; it does not satisfy another
packet's user outcome or promote its durable conclusions.

### Alternative C: Keep an informal conversation-only inventory

Rejected. A conversation list cannot be checked for broken references,
checkbox completion, or successor validity and can silently lose deferred work.

## Design

### Public contract

This packet adds no command, capability, effect, target, adapter, or external
API. Its E2E scenario is repository-boundary governance: the classifier reads
the actual packet tree, produces complete/incomplete/deferred classifications,
checks the detached branch disposition, and the repository gates verify all
references and packet syntax.

### Layer changes

- Domain: none.
- Application: none.
- Infrastructure: none.
- CLI and catalog: none.
- Repository documentation: update only the four user-authorized packet
  directories; do not edit the coordinator or any other packet.

### Data and control flow

```text
docs/work filesystem + packet metadata
  -> mechanical status/checklist/reference scan
  -> E2E classification against repository boundary
  -> conservative deletion eligibility
  -> reference/public/hygiene/task gates
  -> evidence handoff
```

### Error and cancellation behavior

Any missing packet file, malformed status, broken link, unchecked completion
condition, uncertain durable conclusion, or dirty-work dependency fails closed
to `incomplete` or `deferred` and preserves the target. A gate failure stops
cleanup; it does not justify deleting or rewriting evidence.

### Security and public boundary

The work is read-only apart from the four files in this packet. It must not
inspect or copy credentials, private URLs, or local paths. The public guard
remains the authority for packet links, regular-file boundaries, and public
documentation safety.

## Implementation slices

1. Read governing documents, templates, and the complete packet tree.
2. Execute the baseline classifier and record the inventory/disposition.
3. Create this packet without touching the coordinator or active packets.
4. Rerun classification, reference checks, detached-branch check, and task
   gates.
5. Stage only the 16 `goal/context/plan/tasks` files in the four authorized
   packet directories and create one intentional commit. Retain the lifecycle
   packet as evidence until its coordinator review/delete trigger is satisfied.

## Verification

- Unit and contract tests: not applicable; `tools/repoguard` is the packet and
  Markdown contract checker.
- Negative side-effect tests: confirm no existing packet or coordinator file
  changes and no deletion candidate is acted on when any condition is open.
- Opaque-reference and complete-pagination tests: not applicable; no CLI task.
- Structured output, hostile-output, and recovery tests: the classifier output
  is bounded and public-safe; malformed/uncertain packet metadata preserves the
  packet rather than inferring completion.
- Agent-readiness scenario and discovery-round-trip count: not applicable to a
  product capability; the repository-boundary E2E has one classifier run plus
  reference and gate checks.
- Human-handoff scorecard: the table provides exact packet paths, status,
  blocker, successor, and cleanup disposition for the maintainer.
- Manual observation: inspect `git status`, packet paths, and the detached
  auth-broker ancestry result after the checks.
- Required profiles: `task check` and `task public:check`; run the focused
  `go run ./tools/repoguard --scope hygiene` and `--scope public` checks as
  part of the reference evidence.
- Generated-diff or artifact checks: `git diff --check`; no generated artifact
  is produced.
- Commit gate: explicitly stage the 16 authorized packet paths, verify the
  cached path set contains no `README.md` or other path, run `git diff --check`,
  then create one commit and report its SHA. The unrelated `README.md` change
  remains unstaged and preserved.

## Rollout and rollback

No runtime or public command migration applies. The packet is additive and can
be reverted as one evidence change before coordinator acceptance. No packet is
deleted in this run, so no data recovery action is needed.

## Documentation promotion

No product, architecture, security, release, or thesis conclusion is changed.
The lifecycle rule is already governed by `AGENTS.md` and the work templates;
this packet records repository-state evidence and the exact review/delete
trigger for its own retention.
