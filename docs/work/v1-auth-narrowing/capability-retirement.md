# Capability Retirement: V1 brokered authentication narrowing

## Decision

`authentication.broker` remains public only for strict static owner import and
one GitHub acquisition helper over the common post-policy project-bound broker
core. Managed injection, Chatwork, AWS, Datadog, OpenAI, Anthropic, companion,
refresh, signing, and exact-version driver capability are excluded from V1.

## Required negative evidence

- [x] Retired provider names, methods, plan kinds, state discriminators,
      selectors, faults, recoveries, helpers, endpoints, and sockets fail
      closed and are absent from public/help/generated contracts. Evidence:
      catalog/provider/broker/runtime negative tests reject the retired names,
      `--method`, driver metadata, companion operations, and old strict state
      shapes; generated catalog and public-boundary checks contain only the
      retained GitHub/static surface.
- [x] No managed or companion lifecycle, mount, environment, protocol,
      fallback, reader, or image content remains. Evidence: Compose and image
      contracts contain no companion or managed-credential mount, socket,
      environment, module, selector, or fallback; strict runtime decoding
      rejects the removed state.
- [x] No refresh/signing/exact-version dependency remains unowned. Evidence:
      runtime source, dependency, toolbox, image-content, and generated-data
      audits removed the refresh/signing drivers and their only dependencies;
      canonical/embedded source equality checks pass.
- [x] Retained static handles cannot be reused across project, Context,
      provider, revision, target, or HTTP binding. Evidence: Auth Broker and
      Gateway suites cover every binding dimension, rotation, logout, and
      Workspace re-entry; the live Docker journey verifies project-bound
      issuance and exact post-policy replacement.
- [x] Denied, malformed, expired, rotated, or revoked handles cause zero
      upstream credential forwarding and no fallback. Evidence: Gateway
      negative tests assert denial before resolution and zero upstream commit;
      invalid, expired, rotated, and revoked handle cases never forward the
      handle or primary secret and never fall back.

## State handling

Old encrypted vault records and Workspaces are development-only and are not
migrated. Operators use the old snapshot to log out where possible, delete the
affected development state, stop the old cluster, and recreate it under V1.
