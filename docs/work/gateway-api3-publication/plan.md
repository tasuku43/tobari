# Work Plan: Publish and pin Gateway API 3

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Validate and merge the complete GraphQL enforcement change first. Let the
main-only Gateway workflow publish the immutable commit tag and
multi-architecture index. Inspect that exact index, update the production pin
and durable publication documentation in a second reviewed change, then run
the required gates again.

## Alternatives considered

### Publish directly from the workstation

Rejected because the release contract assigns package-write authority only to
the main-push workflow and requires the canonical CI build and verification.

### Pin a moving `main` or `latest` tag

Rejected because routine startup requires immutable, reviewed digest authority.

## Design

### Public contract

No command shape changes during publication. The existing normal `cluster up`
path becomes compatible with the API-3 implementation because its embedded
Gateway digest resolves to the reviewed API-3 manifest.

### Security and public boundary

The GitHub workflow owns registry authentication. The pinned digest is accepted
only after anonymous retrieval and metadata/platform verification. Moving tags
remain development discovery aids, not runtime authority.

## Implementation slices

1. Validate the complete API-3 source change.
2. Commit, push, review, and merge it to `main`.
3. Wait for the Gateway publication workflow and inspect the immutable manifest.
4. Update the pin and durable documentation.
5. Re-run required gates, merge the pin, and verify normal startup.

## Verification

- `task check`
- `task security`
- `task public:check`
- `task release:check`
- `task policy:test`
- `task gateway:test`
- `task authbroker:test`
- `task integration:test`
- Anonymous OCI manifest and per-platform image inspection
- Normal production-resolver `cluster up`

## Rollout and rollback

The immutable API-3 image is published before the pin changes. Rollback is an
explicit reviewed pin change to a compatible immutable digest; the API-2 image
cannot be used with this source revision.

## Documentation promotion

Replace the prior Gateway API-2 publication evidence in
`docs/05_public_repository.md` and `docs/06_release.md` with the reviewed API-3
source revision, manifest digest, and platform/metadata evidence.
