# ADR 0076: Promote one prepared release bundle

- Status: Accepted
- Date: 2026-08-22
- Deciders: Tobari maintainers
- Scope: Harness, security, public boundary, and release
- Supersedes: [ADR 0002: Isolate routine and specialized harness profiles](0002-isolate-harness-profiles.md)
- Superseded by: None

## Context

ADR 0002 correctly separated routine and specialized profiles, but required the
Release workflow to invoke every profile again. Main-push CI now exercises the
full, security, public, runtime, and release profiles in parallel. Recent release
observations show that repeating those profiles serially after dispatch consumes
about twelve minutes, while routine dry-run then publication repeats the same
source checks and artifact assembly for one revision.

The safety requirement is complete exact-revision evidence and immutable
publication subjects. It does not require every proof to execute inside the
privileged publication run or after the tag is created.

## Decision drivers

- Preserve every automated and human release guarantee.
- Keep publication credentials out of source validation and artifact preparation.
- Bind reused evidence and promoted bytes to one exact repository revision.
- Make tag-to-publication latency proportional to verification and mutation,
  not the entire repository test suite.

## Considered options

### Repeat every profile in Release

This keeps one workflow self-contained but serializes already successful
exact-revision CI and rebuilds a prepared candidate during publication.

### Run all Release profiles in parallel

This preserves the duplicate checks with a shorter critical path, but still
spends runner time establishing a second source-evidence set.

### Reuse exact CI and promote prepared bytes

Main/pull-request CI owns all source profiles in parallel. A preparation run
accepts only a successful main-push CI run for the exact revision, builds and
verifies one release bundle without public-distribution authority, and retains
it for a bounded period. A later protected publication revalidates the tag, preparation
run, successful assembly job, exact artifact, preparation invocation, and final
inventory before creating an immutable Release.

## Decision

Use exact-revision CI plus a two-operation manual Release workflow. `prepare`
waits for successful main-push CI while building the release matrix in parallel,
then produces one complete short-lived asset set. `publish` requires that
successful preparation run ID and promotes its unchanged, reverified bytes.
Only `publish` crosses the protected environment; stable publication alone may
obtain the repository-scoped Homebrew App token.

The standalone profiles remain canonical local interfaces. CI composition, not
publication, owns their automated execution. CI build caches may accelerate
ordinary jobs; the release profile retains two isolated Go build caches for its
reproducibility proof. Preparation builds the actual publication subjects once.

## Consequences

### Positive

- Tag-to-GitHub-Release latency no longer includes source gates or archive builds.
- Source profiles remain complete and execute in parallel on every CI revision.
- Publication uses exactly the bytes that were available for approval.
- Prerelease preparation can use Linux because it renders or audits no Formula.

### Negative

- Publication requires a preparation run ID and an unexpired prepared artifact.
- A release revision must have a successful main-push CI run.
- Cross-run validation and artifact retention become maintained workflow contracts.

### Risks and mitigations

- A run ID could select unrelated bytes. Validate repository, workflow path,
  event, main branch, revision, completion, successful assembly job, exact
  artifact name, expiry, preparation attempt, provenance, and final inventory.
- A failed CI run could race preparation. Wait boundedly for the exact run and
  require its final successful conclusion before the preparation run can pass.
- A prepared artifact can expire. Retain it for seven days and require a new
  preparation rather than weakening validation.

## Mechanical enforcement

- `.github/workflows/ci.yml` contains all five required profile invocations.
- `.github/workflows/release.yml` contains no source-profile invocation.
- Release lint fixes exact CI-run selection, prepare-only build/assembly,
  bounded artifact retention, protected publish-only mutation, cross-run
  download inputs, preparation-invocation verification, and stable-only tap
  access.
- Release artifact tools continue to reject changed tag, revision, builder,
  invocation, subjects, inventory, or overwrite attempts.

## Compatibility and migration

CLI commands, output, state, archive names, archive bytes, checksums, SPDX, and
provenance schemas do not change. Maintainer workflow changes from a boolean
`publish` input to `operation=prepare|publish`; publish additionally supplies
the successful preparation run ID. Existing prepared artifacts are not
promotable and must be regenerated once.

## Security and public-boundary impact

No new credential, external destination, dependency, or published artifact is
added. Preparation receives read-only repository and Actions API access; the
artifact actions create only the declared one-day intermediate archives and
seven-day complete bundle. The publication job retains content write only
behind `release-publication`.
Homebrew credentials remain stable-publish-only and repository-scoped. Prepared
artifacts contain the same public release subjects and expire after seven days.

## Validation

```sh
task check
task security
task public:check
task release:check
```

Observe one preparation that reuses the exact main-push CI run, then publish a
new prerelease from its run ID and confirm the published subject digests match.

## Reconsideration signals

Supersede this ADR if cross-run artifacts cannot provide reliable bounded
retention, if exact-revision CI selection is ambiguous, or if promotion
incidents show that rebuilding under the protected job is safer than reviewing
and promoting prepared bytes.
