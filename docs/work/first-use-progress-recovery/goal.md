# Work Goal: Make first Workspace entry legible and recoverable

- Status: Active
- Design status: Accepted / Fixed
- Implementation status: Started from `c421588bb5a2938dee35be629bd81ab7d76b4308`
- Retention: temporary
- Retention reason: implementation coordination only; durable conclusions must be promoted before completion
- Governing contract: ADR 0084, ADR 0085, and docs/00-04
- Review/delete trigger: delete after durable promotion and all final gates
- Successor: None
- Owner: WP10 implementer and Product Owner
- Target: final pre-public first-entry integration
- Related ADRs: 0080, 0082, 0083, 0084, 0085

## Outcome

From a canonical Project directory, one `tobari` invocation reviews a fresh
recommended Workspace Template when final authority is empty, establishes the
default Template and Project-specific Context through the existing final
authority boundary, activates shared protection through canonical `cluster
up`, reconciles the Context's Workspace through explicit entry, and hands the
terminal to Bash or exact argv. Long work is visibly bounded, cancellation is
classified at the boundary it reached, and every failure leaves one causal
Catalog action or typed non-command condition.

The journey consumes, and does not duplicate, these existing authorities:

- Workspace Template: reusable desired definition and immutable revisions;
- Context: one ProjectRoot plus Template binding;
- Policy Memory: Context-owned remembered decisions;
- Workspace: replaceable applied instance and home;
- Runtime: independent revision and execution-material authority;
- status schema 3: the zero-mutation Current/Next/Attention read model.

## Why now

The integrated baseline has the final authority model and status home, but the
bare root currently publishes a fresh Template, default selection, and Context
before proving interactive review, then attempts entry without composing the
required cluster activation. On a fresh redirected invocation this produced
durable authority and an undeclared fault. The final integration therefore
does not yet close the primary first-use outcome.

## Non-goals

- No Manifest namespace, `--manifest`, Manifest selector, migration, or alias.
- No new public resource, progress command/flag/schema, event stream, daemon,
  controller, provider manager, recovery command, or parallel status model.
- No implicit Runtime build, restore, prune, delete, or global Docker cleanup.
- No host credential inspection, host login stage, research auth/serve path,
  or persisted authentication-banner preference.
- No change to WP03 Runtime, WP04 build surfaces, WP05 Host Loopback, WP07
  permission wait, WP09 Service exposure, or WP06 status authority.
- No Git checkout/worktree/rollback behavior.

## Frozen evaluator

No row or public concept may be added without Product Owner review.

| ID | Existing accepted outcome | Passing boundary |
|---|---|---|
| E1 | Fresh first use | Empty final authority performs zero Docker before one interactive Start/Customize/Cancel review; Start establishes the exact default Template and Context, activates protection, reconciles the Workspace, and hands off once |
| E2 | Stopped local engine or cluster | A stopped/unavailable engine changes no final authority before review/preflight; an absent/stopped known cluster converges through canonical `cluster up`; provider startup remains an external condition |
| E3 | Interrupted or partial authority | Confirmed Template/Context, cluster receipt, and AppliedEntry facts are retained independently; unknown mutation outcome never grants blind replay |
| E4 | Explicit entry and retry | `context enter --id CONTEXT_REF [-- COMMAND...]` remains the nondefault exact-reference path; root retry re-observes the default pair and uses only canonical mutations |
| E5 | Truthful progress and Next | Review precedes progress; five finite stages report only their checkpoints; failure/cancel shows retained facts, causal change state, and one Catalog path or typed condition |
| E6 | Idempotent convergence | Repeating the same known-safe root/cluster/entry intent converges without duplicate Template, Context, Workspace, home, receipt, or child handoff |
| E7 | Final repository integration | Durable product/architecture/security/harness/readiness/README contracts agree; all four required task profiles and isolated explicit-context Docker/Colima evidence pass; packet is retired |

## Acceptance criteria

- [ ] Fresh redirected or noninteractive root fails before final state, lock,
      Docker, network, Workspace, or child mutation.
- [ ] Fresh interactive review exposes exactly Start Workspace, Customize, and
      Cancel for one non-authoritative recommended Template draft.
- [ ] Root composes the current atomic default Template/Context initialization,
      canonical final `cluster up`, exact Context entry, and child handoff
      without reimplementing their journals or receipts.
- [ ] Routine progress uses exactly five semantic stages:
      `check_requirements`, `resolve_context`, `prepare_protection`,
      `prepare_workspace`, and `enter_workspace`.
- [ ] Stage state is only `pending`, `running`, `succeeded`, `skipped`,
      `blocked`, `failed`, or `unknown`; a checkmark proves only its checkpoint.
- [ ] Stable line-oriented stderr works first. Current work appears after a
      250 ms anti-flicker threshold, elapsed time after one second, and one
      bounded sanitized wait reason after ten seconds; redirected heartbeat is
      no more frequent than 30 seconds. There is no percent, ETA, raw log, or
      live details key.
- [ ] Before child handoff, caller interruption preserves confirmed results,
      performs bounded settlement/classification, reports one recovery, and
      exits 130. After handoff, child streams and status are authoritative;
      bounded cleanup diagnostics cannot replace child status.
- [ ] `tobari -- COMMAND [ARG...]` and `context enter --id REF -- COMMAND...`
      preserve exact argv with no shell, parsing, persistence, or logging;
      malformed positional-only forms fail before setup.
- [ ] Missing/mismatched Runtime material performs zero implicit build or
      restore and routes to `review runtimes`; root never synthesizes an opaque
      Runtime revision reference.
- [ ] Attached pending adoption launches no new child and returns WP06's typed
      non-command detach condition rather than polling.
- [ ] Standard auth remains native and Workspace-home owned. Fresh shell entry
      may show one non-blocking login-location line; direct commands rely on the
      native client prompt. Release guidance contains no research auth/serve.
- [ ] Every root/fault/status recovery edge is checked against the integrated
      Catalog and WP06 typed Next/Attention model; no self-loop, unchecked argv,
      nonexistent path, action rediscovery, closed reference cycle, or mutation
      retry after unknown outcome remains.
- [ ] status remains schema 3, zero mutation, and within Docker budgets 0/6/12.
- [ ] No default Docker context is used by final integration evidence; an
      explicit disposable context is used and cleaned exactly.
- [ ] `task check`, `task security`, `task public:check`, and
      `task release:check` pass on the exact final clean HEAD.
- [ ] Durable conclusions are promoted and this temporary packet is deleted.

## Governing documents

- [Project theses](../../00_theses.md)
- [Product contract](../../01_product_contract.md)
- [Architecture](../../02_architecture.md)
- [Security model](../../03_security_model.md)
- [Harness](../../04_harness.md)
- [Agent readiness](../../09_agent_readiness_validation.md)
- [ADR 0084](../../decisions/0084-separate-workspace-templates-contexts-and-policy-memory.md)
- [ADR 0085](../../decisions/0085-make-status-the-cwd-home.md)

## Completion definition

All seven evaluator rows have executable evidence; current public and durable
contracts contain no predecessor first-use vocabulary; required profiles and
explicit-context runtime evidence pass; the worktree is clean; and this
temporary packet is absent from the final tree.
