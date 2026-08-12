# Work Context: Narrow brokered authentication for first public V1

## Verified current facts

- Current built-ins cover GitHub, Chatwork, AWS, Datadog, OpenAI, and
  helper-acquired static Anthropic credentials.
- AWS, Datadog, and OpenAI introduce dynamic state, refresh or signing;
  Anthropic adds exact-version helper acquisition; AWS also owns a resident
  companion path. A static managed Gateway adapter duplicates brokered static
  injection through a separate store and selector.
- Auth vocabulary and implementation span domain plans, application login
  inputs, CLI provider/help/fault contracts, provider manifests, host drivers,
  Gateway adapter code, Auth Broker vault/control paths, Docker lifecycle,
  doctor/status, embedded source snapshots, integration tests, images,
  dependencies, release checks, and generated documentation.
- GitHub already implements the intended fixed-command trusted-host acquisition
  and stores a static primary secret through the broker.

## Constraints

- Credential-bearing types, vaults, root keys, provider acquisition, and
  handle resolution remain infrastructure-owned.
- Gateway must authorize the ordinary HTTP effect before resolving a
  recognized handle and must never forward or fall back with an invalid one.
- Owner manifests remain strict non-secret, non-executable local data; secret
  ingress is protected non-terminal stdin only.
- Canonical `authbroker/` and `gateway/` sources own runtime behavior; embedded
  copies are byte-checked snapshots finalized only after integration.
- Pre-public retired state is rejected and recreated, not migrated.

## Evidence to capture

- Exact public/catalog/provider matrix before and after narrowing.
- Negative fixtures for every retired provider/plan/state/selector/transport.
- Retained static-flow binding, rotation, revocation, no-fallback, and
  secret-canary tests.
- Dependency, image-content, source-snapshot, and generated-data diffs.
