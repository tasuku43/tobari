# Work Goal: Retire learned policy compaction before V1

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Exact HTTP effect authorization and capability retirement
- Review/delete trigger: Delete after retirement evidence is promoted and the parent release closes
- Successor: None
- Owner: Tobari maintainers
- Target: Before policy preset integration
- Related ADRs: ADR 0024, ADR 0027, and the Context capability-envelope decision
- Parent: [First public release core](../first-public-release-core/goal.md)
- Prerequisite: [Context capability envelope](../context-capability-envelope/goal.md)
- Child decision: [ADR 0066](../../decisions/0066-context-owned-policy-replaces-presets.md)

## Outcome

V1 learns and reviews only exact HTTP or GraphQL permissions. Observed exact
rules cannot be grouped into a wider path-prefix authority through any public,
internal, persisted, generated, or dormant execution path.

## Non-goals

- Replacing compaction with leases, wildcard rules, automatic approval, or an
  alternate broadening algorithm.
- Replacing the fixed Tobari-owned evaluator or canonical typed policy-data
  authority model.
- Changing policy preset behavior in this packet.

## Acceptance criteria

- [ ] `policy compactions` and `policy compact` are absent from catalog, help,
      dispatch, output schemas, faults, recovery, examples, and generated data.
- [ ] `policy-compaction` references and every compaction application port,
      domain type, state field, activation path, and fallback are absent.
- [ ] Learned rules admit only exact matches; prefix authority cannot be
      loaded from Context state or interpreted by OPA.
- [ ] Existing retained reference chains remain complete and exact rule/reset
      behavior is unchanged.
- [ ] Negative tests cover retired commands, state, selectors, and dormant
      fallback; dependency and generated diffs are reviewed.
- [ ] `task check`, `task security`, and `task public:check` pass on the
      integrated branch.

## Completion definition

The packet completes when exact-rule-only policy is mechanically enforced,
retirement evidence is recorded, the packet is one explainable local commit,
and policy preset integration can no longer inherit prefix authority.
