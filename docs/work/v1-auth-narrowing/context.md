# Work Context: Narrow brokered authentication for first public V1

## Verified current facts

- The retained first-public V1 surface is GitHub trusted-host acquisition plus
  strict owner static stdin import, status, logout, and static Broker handle
  resolution.
- AWS, Datadog, OpenAI, Anthropic, Chatwork, managed injection, companion,
  refresh, signing, exact-version drivers, and their state/config selectors
  have been removed. Strict decoding rejects their pre-public state shapes.
- Auth Broker needs only its internal control attachment and Unix runtime
  socket. It performs no provider network operation, so Gateway alone retains
  the egress network used for an allowed application request.
- Integrated Docker verification exposed three retained pre-narrowing test and
  runtime assumptions: Compose still selected the former companion-sized
  70-second Broker timeout although the static Gateway accepts 1--10 seconds;
  the focused Gateway harness omitted the now-required secret-free
  `gateway.json`; and Go status consumers expected the retired provider state
  `ready` while the static Broker emits the public `configured` state.
- The integrated path now starts and restarts all shared components, imports
  two Context-bound static credentials, reports both as configured, and issues
  project-bound handles. Its next failure is after exact policy review because
  an ordinary HTTP denial audit omits the scheme needed by scheme-aware learned
  policy; that is owned by the separate policy-retirement packet.
- Auth vocabulary and retained implementation still span domain plans,
  application inputs, CLI help/fault contracts, strict provider manifests,
  Gateway static adapter code, Auth Broker vault/control paths, Docker
  lifecycle, status, embedded source snapshots, integration tests, images,
  release checks, and generated documentation.

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
