# ADR 0033: Bind component images during CLI release assembly

- Status: Accepted
- Date: 2026-08-12
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, release, and harness
- Supersedes: The independent release-unit portion of ADR 0017
- Superseded by: None

## Context

Gateway and Auth Broker are internal executables required by every Tobari CLI
release. Treating their immutable digests as committed source inputs creates a
cycle: publish images, copy digests into source, commit again, rerun gates, and
only then build the CLI. Repository development separately relies on mutable
`:dev` tags and a build-tag-selected binary.

## Decision

Gateway, Auth Broker, and the CLI form one release unit. The release workflow
builds both component indexes from the exact requested source revision and
creates one strict schema-1 component lock. CLI packaging accepts that lock as
a required generated input and injects its immutable image references and API
identities at link time. The lock is published beside the CLI archives and is
not committed as source authority.

Normal installed-user startup continues to pull and verify immutable GHCR
digests. Repository development instead builds local component images named by
the embedded canonical source hash and reuses an image only after the ordinary
metadata preflight succeeds. Moving tags never select either path.

`versions.env` remains the source authority for reviewed third-party parents
and standalone runtime inputs, not for generated Tobari-owned release outputs.

## Consequences

- A source revision and release version are supplied once.
- No digest-pin follow-up commit is needed.
- Every CLI archive identifies the exact component authorities generated for
  its release.
- Installed startup retains common reviewed bytes and avoids local package
  installation/build failures.
- Release CI becomes responsible for paired component construction before CLI
  packaging and must fail closed on partial evidence.

## Mechanical enforcement

- The component-lock validator checks schema, source revision, canonical image
  repositories, immutable digests, APIs, and exact platform set.
- Release packaging has no fallback component authority.
- Published resolver tests reject absent or malformed link-injected authority.
- Dev resolver and build-script tests derive identical source-hash tags.
- Release/public gates validate workflow ordering and lock propagation.
