# Capability Retirement: Replace the Host Loopback presentation authority

This record applies the repository retirement checklist to the routable
`host.tobari.test` authority. The Host Loopback capability remains; only its
public/policy hostname is replaced.

## Decision and evidence

- Capability ID: existing attachment-scoped Host Loopback capability
- Previous status and public commands: implemented ambient capability at
  `http://host.tobari.test:{port}`; reviewed through existing policy review
  commands, with no hostname selector
- New status: retained with scheme `http` and authority
  `host.tobari.internal:{port}`, with unchanged effect and attachment lifetime
- Superseding capability or ADR: planned revision/supersession of ADR 0049
- User, incident, compatibility, security, or maintenance evidence: `.test`
  communicates testing; `.localhost` would point at Workspace loopback;
  `.internal` is permanently withheld from root delegation and resolver-neutral
  under current standards; a dual routable alias would widen authority
- Last version or revision that supported the old surface: current development
  revision before the planned cutover; no public release exists yet

## Public contract removal

- [ ] Current command paths, namespaces, help entries, examples, capability
      projection, and dispatch behavior name only the new authority; the old
      name remains only in bounded migration/history material.
- [ ] Produced/consumed policy-candidate and attachment-grant reference edges
      remain valid under fresh discovery; old references fail unchanged.
- [ ] Fault declarations and recovery actions never route to the retired name
      and provide the exact new URL template without automatic retry.
- [ ] Capability and schema ledgers record the name replacement, schema-2
      projection, and pre-public compatibility decision.
- [ ] Machine-schema/version and compatibility impact are explicit for
      capability, route, grant, candidate, denial, review, and audit data.
- [ ] Negative tests prove the retired name is neither an alias nor an ordinary
      external/persistent-policy fallback.

## Implementation and dependency removal

- [ ] Domain constants, application interpretation, infrastructure registries,
      Gateway/OPA policy, projection, tests, and docs have no routable old-name
      branch.
- [ ] No dependency changes are introduced solely for the rename; DNS, HTTP,
      TLS, CA, and container-runtime dependencies remain owned by existing
      supported behavior.
- [ ] No dormant CNAME, redirect, wildcard, suffix match, legacy environment
      value, learned rule, or hidden flag can reactivate old authority.
- [ ] Documentation and Skills describe the current physical-host capability,
      not `host.docker.internal` transport or the historical test name.

## Persisted state

| State | Secret? | Disposition: ignore / migrate / explicit cleanup | Recovery and evidence |
|---|---|---|---|
| Capability projection schema 1 in a live Workspace environment | No | Explicit cleanup by attachment exit | Re-enter to receive schema 2; no in-place environment mutation |
| Host Loopback route registry schema 1 | Contains relay token | Explicit bounded cleanup when no live owner | Exact owner/schema/old-host validation under lock, then replace with empty schema 2; never render token |
| Attachment grant registry schema 1 | No primary credential; authority-sensitive | Explicit bounded cleanup with route registry | Drop rather than translate; fresh denial/review produces schema-2 grant |
| Old candidate/review/grant opaque references | No | Ignore as stale and reject | Reproduce request at new URL, rediscover, and pass new reference unchanged |
| Historical denial/audit rows with old host | No | Keep as historical evidence | Never derive a learnable current candidate or rewrite identity |
| Learned external policy rules | No | No Host Loopback migration | Current invariant already excludes Host Loopback; old exact host must not become ordinary external authority |
| Gateway root CA and public trust bundle | Root private key is secret | Keep; no rotation | Digest remains stable; current Host Loopback is HTTP-only |
| Possible per-host proxy leaf cache | Private key material | Observe, then keep or exact safe cleanup | Never treat leaf deletion as authority revocation or rotate root without separate cause |

- [ ] Cleanup is attached to one explicit, bounded lifecycle action with exact
      ownership checks, fail-closed outcome, and no unrelated state deletion.
- [ ] No dependency is retained only to translate old grants, candidates,
      routes, or relay tokens.
- [ ] Relay tokens, CA keys, cached leaf keys, and old state cannot leak through
      logs, errors, fixtures, migration output, or repository history.

## Verification

- Focused negative tests: old name, all near matches, old schema, stale opaque
  references, alias/redirect absence, live-owner cleanup refusal, TLS/SNI, and
  zero side-effect call counts
- Catalog/capability/schema checks: exact Catalog output fields, capability
  schema 2, route/grant schema 2, capability ledger, schema ledger, agent help,
  generated-site source, and public-boundary scan
- Dependency and import diff: no new runtime, resolver, TLS, parser, or CLI
  dependency
- Persisted-state migration or cleanup tests: exact predecessor under lock,
  mixed/corrupt/wrong-owner refusal, empty schema-2 result, fresh epoch/review,
  historical-denial non-learning, and no secret output
- Required gate: `task check`, `task security`, `task public:check`,
  `task release:check`, and clean Docker integration
- Rollback or reintroduction policy: pre-public source revert requires all
  attachments to exit and state recreation; post-release routable reintroduction
  requires a new security and compatibility decision
