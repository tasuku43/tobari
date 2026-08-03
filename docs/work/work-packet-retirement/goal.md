# Work Goal: Audit and safely retire completed work packets

- Status: Active
- Retention: evidence
- Retention reason: The packet inventory and lifecycle classifications are repository-state evidence that cannot be reconstructed from durable product and architecture documents alone.
- Governing contract: docs/00_theses.md, docs/01_product_contract.md, docs/02_architecture.md, docs/03_security_model.md, docs/04_harness.md, docs/05_public_repository.md, docs/06_release.md
- Review/delete trigger: Delete this evidence packet after the coordinator accepts the audit, the inventory is superseded by a newer audit, and no active/deferred packet disposition depends on this record.
- Successor: None
- Owner: Repository maintainer
- Target: `docs/work` packet lifecycle and safe cleanup
- Related ADRs: None

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
- Do not edit any existing active packet.
- Do not change product code, CLI behavior, branches, issues, releases, or publications.
- Do not delete a non-empty packet unless the lifecycle procedure proves every deletion condition.
- Do not treat a passing unit, build, or focused integration check as E2E completion by itself.

## Acceptance criteria

- [x] The packet reads the governing documents and the work-packet templates/README and records the applicable lifecycle rules. Evidence: `context.md` records the documents, template set, and `tools/repoguard` lifecycle rules.
- [x] Every non-empty non-template packet and every empty `docs/work` directory is inventoried, with goal/status/tasks/check/E2E/durable-conclusion/successor evidence. Evidence: `context.md` inventories nine non-empty packets and the `_template` exclusion; the final empty-directory scan is zero.
- [x] A machine-checkable lifecycle procedure is executed against the actual repository and produces at least one complete, incomplete, and deferred classification. Evidence: the final classifier reports one evidence packet as `complete`, eight active packets as `incomplete`, and the coordinator's `auth-broker-deferral` row as `deferred`.
- [x] Temporary-retention cleanup is applied conservatively; active, incomplete, uncertain, and evidence-retention packets are preserved, and no packet is removed unless every deletion predicate is proven. Evidence: `context.md` records an empty deletion-candidate set because the temporary auth-broker and CLI packets retain open commit/handoff conditions; all other packets and dirty artifacts are preserved.
- [x] Markdown/reference integrity is checked and the required repository task gates pass for this documentation-only change. Evidence: HEAD-plus-own-packet `task public:check` and `task check` pass; actual-checkout blockers from out-of-scope packet files are recorded separately.
- [x] The final handoff names all changed or cleanup-target paths, the absence or presence of deletion candidates, and the E2E result. Evidence: `context.md` and `tasks.md` name the four authorized packet paths, the empty cleanup-candidate set, remaining holds, and E2E handoff.
- [ ] The four authorized packet files are staged and committed in one intentional commit, and the final report includes the commit SHA and clean scoped status. Evidence: blocked because the environment cannot create `.git/index.lock`; no stage or commit exists.

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

The audit is complete when its state classifier, reference check, and required
task gates pass; the inventory proves which packets are not eligible for
cleanup; any clearly completed temporary packet is explicitly listed before
removal; and the four authorized files are staged and committed in one
intentional commit without changing the coordinator or active packets. If Git
metadata is read-only, keep this packet `Active` and record the blocker rather
than claiming completion.
