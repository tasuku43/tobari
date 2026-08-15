# Work Plan: Capability profiles and first prerelease

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Keep the existing source-versus-published resolver axis independent from one
new compile-time capability profile. The default profile is standard;
`tobari_experimental` adds all currently reviewed experimental registrations.
Generate one two-service lock from actual OCI build results and inject its
immutable references into release binaries. Build the pinned agent-ready base
locally from the embedded recipe when its source-derived tag is absent.

## Alternatives considered

### One build tag per feature

Rejected because combinations grow without a coherent maturity contract and
release leakage becomes difficult to test.

### Runtime feature flags

Rejected because a release binary could accidentally activate unsupported
credential and side-effect surfaces after publication.

## Design

### Public contract

`task build` produces a source resolver with the standard profile.
`task build:dev` produces a source resolver with the experimental profile.
Release archives always use the published resolver and standard profile.
`version` reports both axes. Existing command roles and effects do not change.

### Layer changes

- Domain: profile vocabulary, active provider registry, and two-service lock invariant.
- Application: no new use case; existing provider validation consumes the active projection.
- Infrastructure: filter built-in manifests and host drivers through the active profile; ensure the official runtime from embedded local source.
- CLI and catalog: derive login provider/method/help/faults from the active profile and expose profile identity.

### Data and control flow

Build tag selects profile -> domain registry selects active providers ->
infrastructure loads/projects only active manifests -> catalog and drivers use
the same active union. Release builds two OCI indexes -> records evidence ->
creates lock -> packages CLI with immutable Gateway/Auth Broker refs and a
source-derived local-base tag.

### Error and cancellation behavior

An AWS selector in a standard build fails during catalog parsing or installed
provider validation before host process, Broker, vault, OPA, DNS, or network
effects. Release publication remains create-only and refuses an existing
immutable component tag or GitHub Release.

### Security and public boundary

Profiles grant no runtime activation mechanism. The release workflow alone has
package-write permission. Package deletion is a separately authorized,
explicitly enumerated destructive operation immediately before first publish.

## Implementation slices

1. Profile contract and failing provider/build identity tests.
2. Active provider projection and catalog/driver filtering.
3. Two-service component lock and local-base official resolver.
4. Sole publication workflow and prerelease contract.
5. Governing documentation, gates, GHCR reset, and handoff.

## Verification

- Unit and contract tests: standard and `tobari_experimental` Go matrices.
- Negative side-effect tests: standard AWS login rejected before acquisition.
- Manual observation: `v0.1.0-dev.1` dry-run workflow and anonymous OCI inspection.
- Required profiles: full, security, release, public, runtime.
- Generated-diff or artifact checks: reproducible archive matrices and two-service lock.

## Rollout and rollback

This is pre-public V1. Existing local state has no compatibility guarantee.
Rollback uses a new source revision and prerelease; published immutable assets
are never overwritten.

## Documentation promotion

- Standard/experimental profile lifecycle to theses, product, architecture, security, harness, and release docs.
- Local-only base and sole protected two-service GHCR publication to public/release docs.
