# Work Goal: Low-friction policy learning and compaction

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: docs/00_theses.md and docs/01_product_contract.md
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Current change
- Related ADRs: None

## Outcome

A developer can turn a recent denied HTTP effect into one exact tested
host/method/path permission through an opaque-reference workflow, review the
same queue through a concise human tail, and explicitly replace repeated exact
rules with a tested path-prefix rule when Tobari can present a bounded
compaction candidate.

## Why now

Real GitHub CLI and TWG journeys proved that denial evidence and portable
activation work, but users still have to understand and edit Rego data by hand.
Repeated exact permissions will also accumulate without a safe maintenance
path.

## Non-goals

- Automatic permission changes without an explicit act command
- Host wildcards, method wildcards, arbitrary user-entered patterns, or inferred credentials
- Claiming that finite tests prove safety for every unknown future request
- A mutating interactive command that combines discovery and action
- Provider-specific policy adapters

## Acceptance criteria

- [ ] `policy candidates` emits unique pending exact-rule candidates with opaque references and complete machine-readable fields.
- [ ] `policy allow --id` consumes one reference unchanged, preflights the candidate policy, atomically writes the XDG data, activates OPA, and confirms the exact rule.
- [ ] `policy tail` gives humans a bounded read-only queue with exact approval commands.
- [ ] `policy compactions` emits only deterministic candidates backed by at least three same-host/method exact rules below a sufficiently specific shared directory.
- [ ] `policy compact --id` consumes one current candidate unchanged, preserves its positive examples, verifies exact/prefix boundary canaries, atomically replaces the source rules, and activates OPA.
- [ ] Invalid, stale, ambiguous, unsafe, or test-failing candidates cause no policy write.
- [ ] The end-to-end Docker scenario performs denial, candidate discovery, exact approval, retry, repeated-rule compaction, and negative-boundary verification.
- [ ] `task check`, `task runtime:test`, `task security`, and `task public:check` pass.

## Governing documents

- Thesis: Thesis 8, denial as a safe policy-development interface
- Product contract section: Primary operating loop and public commands
- Architecture or security invariant: discovery/action separation and trusted-host-only policy writes
- Existing ADR: None

## Completion definition

The work is complete when acceptance criteria have durable tests and
documentation, the agent journey needs no undeclared parsing or source
inspection, required gates pass, temporary fixtures are removed, and this
packet is removed from the final tree.
