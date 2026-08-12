# Capability Retirement: V1 brokered authentication narrowing

## Decision

`authentication.broker` remains public only for strict static owner import and
one GitHub acquisition helper over the common post-policy project-bound broker
core. Managed injection, Chatwork, AWS, Datadog, OpenAI, Anthropic, companion,
refresh, signing, and exact-version driver capability are excluded from V1.

## Required negative evidence

- [ ] Retired provider names, methods, plan kinds, state discriminators,
      selectors, faults, recoveries, helpers, endpoints, and sockets fail
      closed and are absent from public/help/generated contracts.
- [ ] No managed or companion lifecycle, mount, environment, protocol,
      fallback, reader, or image content remains.
- [ ] No refresh/signing/exact-version dependency remains unowned.
- [ ] Retained static handles cannot be reused across project, Context,
      provider, revision, target, or HTTP binding.
- [ ] Denied, malformed, expired, rotated, or revoked handles cause zero
      upstream credential forwarding and no fallback.

## State handling

Old encrypted vault records and Workspaces are development-only and are not
migrated. Operators use the old snapshot to log out where possible, delete the
affected development state, stop the old cluster, and recreate it under V1.
