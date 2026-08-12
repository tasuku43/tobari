# Context

## Verified facts

- `versions.env` currently contains paired `unpublished` owned-image markers.
- The normal resolver reads owned image authorities from that embedded file.
- `task build:dev` builds mutable `:dev` tags and compiles a separate resolver.
- Gateway and Auth Broker canonical sources are already byte-checked against
  snapshots embedded in the CLI.
- Release packaging currently has no component-lock input.
- Component workflows already emit image digest/revision evidence, but the
  release workflow does not consume it.

## Constraints

- Preserve all existing uncommitted authentication and Gateway work.
- Official startup must use immutable digest references.
- Release artifacts must remain deterministic and create-only.
- No registry credentials or host-specific data may enter build identity.

## Unknowns to resolve with tests

- Exact buildx metadata behavior for cache-only versus pushed OCI indexes.
- Whether the first integrated publish run requires small workflow adjustments
  for the installed Docker/buildx version.
