# Work Goal: Audit and safely retire completed work packets

- Status: Complete
- Retention: evidence
- Retention reason: The packet inventory and lifecycle classifications are repository-state evidence that cannot be reconstructed from durable product and architecture documents alone.
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md
- Review/delete trigger: Delete this evidence packet after the coordinator accepts the audit, the inventory is superseded by a newer audit, and no active/deferred packet disposition depends on this record.
- Successor: None
- Owner: Repository maintainer
- Target: `docs/work` packet lifecycle and safe cleanup
- Related ADRs: None

History note: the lifecycle audit was committed in
`92d742c3397e7aea8a24ccc23fbfef41e33d7134`, refreshed in `1cc400e`, and this
refresh audits the clean `main` baseline at `2810f48`. The current packet tree
contains the runtime lifecycle, Quick Start handoff, and PTY evidence packets;
their open/retained states are recorded in `context.md`.

## Outcome

Every non-empty work packet in the current repository is classified as active,
complete, superseded, deferred, or otherwise not eligible for cleanup. The
classification records goal, checklist, gate, E2E, durable-conclusion, and
successor evidence, applies the temporary-retention deletion rule, and leaves
active or uncertain packets untouched. The audit itself is replayable against
the repository state and does not modify the coordinator packet.

## Why now

The repository contains several packets with substantial implementation
evidence but open acceptance, publication, integration, or handoff work. A
packet with passing focused tests must not be mistaken for a completed packet,
and the deferred auth-broker branch has no standalone packet. A bounded audit
is needed before any cleanup so unfinished work, deferred work, and evidence
retention are not conflated.

## Non-goals

- Do not edit `docs/work/tobari-improvement-triage/*`.
- Do not edit any active packet outside the four explicitly authorized packet directories.
- Do not change product code, CLI behavior, branches, issues, releases, or publications.
- Do not delete a non-empty packet unless the lifecycle procedure proves every deletion condition.
- Do not treat a passing unit, build, or focused integration check as E2E completion by itself.

## Acceptance criteria

- [x] The packet reads the governing documents and the work-packet templates/README and records the applicable lifecycle rules. Evidence: `context.md` records the documents, template set, and `tools/repoguard` lifecycle rules.
- [x] Every non-empty non-template packet and every empty `docs/work` directory is inventoried, with goal/status/tasks/check/E2E/durable-conclusion/successor evidence. Evidence: `context.md` inventories nine non-empty packets and the `_template` exclusion; the final empty-directory scan is zero.
- [x] A machine-checkable lifecycle procedure is executed against the actual repository and produces at least one complete, incomplete, and deferred classification. Evidence: the final classifier output in `context.md` records complete catalog/lifecycle evidence packets, active policy/runtime and other incomplete packets, and the separately deferred auth-broker item.
- [x] Temporary-retention cleanup is applied conservatively; active, incomplete, uncertain, and evidence-retention packets are preserved, and no packet is removed unless every deletion predicate is proven. Evidence: `context.md` records an empty deletion-candidate set; evidence packets, the accepted/deferred auth packet, active packets, and the out-of-scope `README.md` change are preserved.
- [x] Markdown/reference integrity is checked and the required repository task gates pass for this documentation-only change. Evidence: current-main `task public:check` and `task check` pass after the authorized packet edits; the unrelated `README.md` change is excluded from staging.
- [x] The final handoff names all changed or cleanup-target paths, the absence or presence of deletion candidates, and the E2E result. Evidence: `context.md` and `tasks.md` name the four authorized packet paths, the empty cleanup-candidate set, remaining holds, and E2E handoff.
- [x] The authorized packet files in the four scoped directories are staged and committed in one intentional commit, and the final report includes the commit SHA and clean scoped status. Evidence: the final staging audit uses only the 16 `goal/context/plan/tasks` paths under the four authorized directories; the handoff reports the resulting SHA and leaves `README.md` unstaged.

## Governing documents

- Thesis: [Project theses](../../00_theses.md)
- Product contract: [Product contract](../../01_product_contract.md)
- Architecture: [Architecture](../../02_architecture.md)
- Security: [Security model](../../03_security_model.md)
- Harness: [Harness](../../04_harness.md)
- Public repository: [Public Repository](../../05_public_repository.md)
- Release: [Release](../../06_release.md)
- Work-packet guidance: [Documentation map](../../README.md), [goal template](../_template/goal.md), [context template](../_template/context.md), [plan template](../_template/plan.md), and [tasks template](../_template/tasks.md)

## Completion definition

The audit is complete: its state classifier, reference check, and required
task gates pass; the inventory proves which packets are not eligible for
cleanup; no completed temporary packet is removed without its retention
trigger; and the four authorized files were committed in one intentional
scoped commit without changing the coordinator or active packets. The packet
remains retained as evidence until its review/delete trigger is satisfied.
