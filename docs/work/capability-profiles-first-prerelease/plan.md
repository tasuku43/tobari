# Work Plan: Capability profiles and first prerelease

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Keep the existing development-versus-embedded resolver axis independent from one
new compile-time capability profile. The default profile is standard;
`tobari_experimental` adds all currently reviewed experimental registrations.
Build the pinned Gateway and agent-ready base locally from embedded recipes
when their source-derived tags are absent. Publish no Tobari-owned OCI image.

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
Release archives always use the embedded resolver and standard profile.
`version` reports both axes. Existing command roles and effects do not change.

### Layer changes

- Domain: profile vocabulary and active provider registry.
- Application: no new use case; existing provider validation consumes the active projection.
- Infrastructure: filter built-in manifests and host drivers through the active profile; ensure Gateway and the official runtime from embedded local source.
- CLI and catalog: derive login provider/method/help/faults from the active profile and expose profile identity.

### Data and control flow

Build tag selects profile -> domain registry selects active providers ->
infrastructure loads/projects only active manifests -> catalog and drivers use
the same active union. Release packages CLI archives directly with the
embedded resolver and source-derived local image identities.

### Error and cancellation behavior

An AWS selector in a standard build fails during catalog parsing or installed
provider validation before host process, Broker, vault, OPA, DNS, or network
effects. Release publication remains create-only and refuses an existing
GitHub Release.

### Security and public boundary

Profiles grant no runtime activation mechanism. Release workflows have no
package-write permission, registry login, or image push path.

## Implementation slices

1. Profile contract and failing provider/build identity tests.
2. Active provider projection and catalog/driver filtering.
3. Embedded local Gateway/base resolver and removal of the component lock.
4. GitHub-Release-only publication workflow and prerelease contract.
5. Governing documentation, gates, GHCR reset, and handoff.

## Verification

- Unit and contract tests: standard and `tobari_experimental` Go matrices.
- Negative side-effect tests: standard AWS login rejected before acquisition.
- Manual observation: `v0.1.0-dev.1` dry-run workflow and anonymous OCI inspection.
- Required profiles: full, security, release, public, runtime.
- Generated-diff or artifact checks: reproducible five-archive matrices and metadata.

## Rollout and rollback

This is pre-public V1. Existing local state has no compatibility guarantee.
Rollback uses a new source revision and prerelease; published immutable assets
are never overwritten.

## Documentation promotion

- Standard/experimental profile lifecycle to theses, product, architecture, security, harness, and release docs.
- Local-only Tobari images and GitHub-Release-only publication to public/release docs.
