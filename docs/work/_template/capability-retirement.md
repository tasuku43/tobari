# Capability Retirement: Short outcome

Use this optional record when removing, replacing, or narrowing a public or
previously implemented capability. Retirement is a product, compatibility, and
security decision; passing tests after deleting a handler is not sufficient.

## Decision and evidence

- Capability ID:
- Previous status and public commands:
- New status:
- Superseding capability or ADR:
- User, incident, compatibility, security, or maintenance evidence:
- Last version or revision that supported the old surface:

## Public contract removal

- [ ] Command paths, namespaces, help entries, examples, and dispatch bindings
      are removed or explicitly deprecated.
- [ ] Produced/consumed reference edges and required chains remain valid.
- [ ] Fault declarations and recovery actions no longer name the retired path.
- [ ] Capability and schema ledgers record the new state and reason.
- [ ] Machine-schema/version and compatibility impact are explicit.
- [ ] Negative tests prove the retired path and configuration selector are not
      accepted through an undocumented fallback.

## Implementation and dependency removal

- [ ] Application ports, domain variants, adapters, composition wiring, and
      feature-specific policy are removed or justified as shared behavior.
- [ ] Provider SDKs, protocol libraries, transitive modules, imports, build
      tags, generated files, environment variables, and CI/release steps are
      removed when no supported capability owns them.
- [ ] No dormant transport, raw route, hidden flag, or legacy environment value
      can reactivate the retired behavior.
- [ ] Documentation and Skills describe the current product rather than the
      historical implementation.

## Persisted state

Choose one reviewed disposition for each secret and non-secret state format.
Unrelated commands must not silently delete legacy state.

| State | Secret? | Disposition: ignore / migrate / explicit cleanup | Recovery and evidence |
|---|---|---|---|
|  |  |  |  |

- [ ] Cleanup, if supported, is an explicit bounded action with its own effect,
      target, impact, policy, outcome, and rollback limits.
- [ ] Keeping a dependency only to read or delete legacy state is justified.
- [ ] Removed credentials cannot leak through logs, errors, fixtures, or public
      repository history.

## Verification

- Focused negative tests:
- Catalog/capability/schema checks:
- Dependency and import diff:
- Persisted-state migration or cleanup tests:
- Required gate:
- Rollback or reintroduction policy:
