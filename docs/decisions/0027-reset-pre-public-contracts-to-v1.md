# ADR 0027: Reset pre-public contracts to exact V1

- Status: Accepted
- Date: 2026-08-11
- Deciders: Tobari maintainers
- Scope: Product, CLI, architecture, security, authentication, external I/O,
  state, harness, public boundary, and release
- Supersedes: The version-increment, migration, compatibility-reader, retired
  command, and historical-image-selection clauses of ADRs 0013, 0018–0023,
  0025, and 0026
- Superseded by: None

## Context

Tobari has not been publicly released. During implementation, individual
component APIs, persisted documents, CLI envelopes, policy inputs, audits,
provider records, and vault payloads advanced independently. The repository
also accumulated readers, migrations, retired-command diagnostics, and tests
that assumed users depended on earlier development snapshots.

Those assumptions now obscure the current product and enlarge its trusted and
tested surface without protecting a published user. Previously published
Gateway/Auth Broker development indexes cannot be relabeled as V1 because
their image labels and bytes implement older source contracts.

## Decision

The current repository is the initial contract snapshot:

- every Tobari-owned schema and component API is V1;
- readers accept exactly V1 and reject all other versions;
- no state migration, compatibility reader, legacy fallback, retired command
  alias, or old-state cleanup path is provided;
- development state from another snapshot must be removed and recreated;
- behavior that is still required is expressed directly in the V1 model,
  including typed built-in credential plans and explicit AWS driver
  discriminators; and
- official Gateway and Auth Broker image authorities remain paired
  `unpublished` markers until new reviewed immutable Linux amd64/arm64 V1
  indexes exist. Development/full gates may accept only that exact paired
  marker; public and release gates reject it.

Opaque reference version markers, external provider protocol versions, pinned
third-party client versions, and versioned cryptographic/service identifiers
remain unchanged when they are not Tobari schema/API generations.

## Consequences

- The public CLI, stored state, Gateway, OPA, Auth Broker, provider projection,
  vault, companion, and harness contracts have one unambiguous V1 authority.
- Unsupported-version negative tests remain; success fixtures contain no old
  shape or implicit default.
- A future post-public V2 requires an explicit compatibility and migration
  decision based on real user obligations.
- Source builds remain usable through the explicit development image path.
  Release completion is mechanically blocked until immutable V1 component
  images are independently reviewed and pinned.

## Verification

- Contract tests require schema/API V1 across every Tobari-owned surface.
- Repository searches and focused tests reject migration and compatibility
  remnants outside historical decision text and negative canaries.
- Canonical Gateway/Auth Broker source snapshots remain byte-identical.
- `task check` is the implementation completion gate; public/release profiles
  additionally reject unpublished component image authorities.
