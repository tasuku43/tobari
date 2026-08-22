# Capability Retirement: Replace competing review entry points

## Decision and evidence

- Capability ID: `policy.review` path and `trusted-host.review` selector surface
- Previous status and public commands: public `policy review`; public root
  `review` selector
- New status: replaced by `review permissions` and `review services`
- Superseding capability or ADR: accepted review-first CLI packet; ADRs 0073
  and 0074 as revised during implementation
- User, incident, compatibility, security, or maintenance evidence: the two
  trusted-host review tasks appeared at different command depths, while the
  desired root path cannot safely be both a registered command and namespace
- Last version or revision that supported the old surface: commit
  `1b2341c79bf0edf33812cb5ac5359b38d36210a3`

## Public contract removal

- [ ] Old command paths, exact help entries, examples, and dispatch bindings are
      removed.
- [ ] Existing policy-candidate and service-request reference graphs remain
      valid through the replacement leaves and lower-level operations.
- [ ] Fault declarations, denial guidance, and recovery actions no longer name
      retired paths.
- [ ] Capability and schema ledgers record the replacement without inventing a
      compatibility alias.
- [ ] Existing leaf machine fields retain their schema; command identity and
      recovery strings explicitly move.
- [ ] Negative tests prove the retired paths are not accepted through a hidden
      handler, argv rewrite, or legacy selector.

## Implementation and dependency removal

- [ ] Selector-only presentation and routing are removed; shared Permission and
      Service use cases, ports, adapters, and policies remain owned by the new
      leaves.
- [ ] No dependency, transport, build tag, generated source, environment value,
      or CI/release step exists solely for the selector.
- [ ] No dormant selector or legacy path can reactivate old behavior.
- [ ] Documentation and applicable Skill guidance describe the current paths.

## Persisted state

| State | Secret? | Disposition: ignore / migrate / explicit cleanup | Recovery and evidence |
|---|---|---|---|
| Learned permission rules and retained denial evidence | No | ignore | The same application and policy stores are used by `review permissions`; no format or identity changes |
| Live service requests, exposures, and attachment records | No | ignore | The same service use cases and opaque references are used by `review services`; no format or identity changes |
| Workspace credentials | Yes | ignore | Command navigation neither reads nor changes Workspace-owned credentials |

- [ ] No cleanup action is introduced because no state format is retired.
- [ ] No dependency is retained solely for legacy cleanup.
- [ ] Existing secret-free output and fixture rules remain unchanged.

## Verification

- Focused negative tests: retired path lookup, dispatch, help, recovery, and
  no-side-effect canaries
- Catalog/capability/schema checks: global catalog validation, scoped agent
  help, exact leaf schema assertions, and reference-flow closure
- Dependency and import diff: no new dependency; selector-only code removed
- Persisted-state migration or cleanup tests: not applicable because state is
  preserved unchanged
- Required gate: `task check` and `task public:check`
- Rollback or reintroduction policy: restore only through a new reviewed public
  contract; do not keep dormant aliases
