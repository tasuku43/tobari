# ADR 0082: Separate release and research build surfaces

- Status: Accepted
- Date: 2026-08-24
- Deciders: Tobari product owner and maintainers
- Scope: Product, CLI, Catalog, build, resolver, topology, authentication,
  release, architecture site, schema, migration, and harness
- Related: ADR 0044, ADR 0079, ADR 0080
- Superseded by: None

## Context

Tobari has two repository build paths with different supported capability
sets. The protected release artifact must remain native Workspace-owned and
must not expose, resolve, or activate the repository-only Auth Broker or
Operator Console through its Catalog, topology, API, or reachable paths. A
contributor build needs the same release Catalog plus a closed research
surface, while using local source-derived component images. Earlier prose and
build tags called this distinction a standard/experimental profile, which
collided with agent profiles, provider profiles, and harness profiles and made
it possible for a renamed binary, ambient build flags, or runtime state to be
mistaken for capability authority.

## Decision

The capability boundary is named **release surface** or **research surface**.
The component resolver is a separate axis named **embedded** or
**development**. Version schema V1 contains exactly these independent fields:

```json
{
  "capability_surface": "release|research",
  "resolver_channel": "embedded|development"
}
```

The protected release tuple is `(release surface, embedded resolver,
canonical program tobari)`. Repository `task build` is `(release surface,
development resolver, bin/tobari)`. The retained contributor path
`task build:dev` is `(research surface, development resolver,
bin/tobari-research)`. The tuple `(research surface, embedded resolver)` is
invalid and fails at compile time; `tobari_research` without `tobari_dev` is
also an intentional compile failure. There is no `profile` capability field,
standard/experimental alias, dual reader, or `build:research` alias.

The release Catalog is the common base. Research is its exact superset with
only five additional paths: `auth login`, `auth import`, `auth status`, `auth
logout`, and `serve`. Manifest, Runtime, Workspace, and shared Catalog
lifecycle paths are common to both surfaces. The root Catalog path remains
`tobari` so the common set is stable; the renderer treats it as the executable
root. No-argument and delimiter-led entry route to that path, while usage,
human help, machine invocations, and recovery render the compiled program
once (`tobari` or `tobari-research`). A copied or renamed basename is never
authority.

Capability authority is compile-time build constraints and the compiled
surface value only. Runtime source or revision, Workspace Manifest revision,
Workspace state, argv, environment, configuration, migration input, and
binary rename/copy cannot select or extend the surface or resolver. Release
Catalog, release human help, root agent help, release topology, and protected
archives contain no Broker component/API, `auth` path, or `serve` path.
Research Broker remains repository-only, unsupported, and unpublished; its
examples use `bin/tobari-research ... --manifest ...` and never appear in the
release architecture-site IA.

Standard authentication is always native and Workspace-owned. Host
credentials are not inherited. Research Broker authority is distinct from
inert recovery material: WP01 migration atomically quarantines the complete
replay-capable research filesystem set (including Linux filesystem root-key
material and binding/handle/provider state), leaves the macOS Keychain item
untouched, makes old readers unable to discover or resolve it, and requires a
fresh explicit research login/import under a new Manifest identity. No
automatic Keychain cleanup, decrypt, import, or rebind is performed. WP04
consumes old-reader denial and fresh-login evidence; it does not move this
authority into Manifest desired/applied state.

## Compatibility and migration

The stable release is pre-public and accepts the breaking cutover. The old
`capability_profile` field, standard/experimental values, research-only old
tag, and public aliases are rejected rather than dual-read. Known prerelease
consumers receive a breaking note. Existing release archive names and the
canonical `tobari` program are preserved; research archives are not release
artifacts and use `tobari-research` only in the contributor path.

## Consequences

- Product language can distinguish capability authority from resolver choice
  without overloading `profile`.
- Routine release UX has no Broker or `serve` discovery, while contributors
  get an executable research identity and exact five-path delta.
- Version, help, Catalog, topology, binary metadata, and archive inspection
  can mechanically prove presence and absence.
- WP01 Manifest migration and WP03 Runtime lifecycle remain separate seams;
  WP04 consumes their final schema and Catalog without changing their wires or
  lifecycle semantics.

## Mechanical enforcement

- Build-tag files define the two valid surface/resolver tuples and reject the
  invalid research-only tuple at compile time.
- Version JSON, human output, root/scoped agent help, recovery, completion,
  and Catalog tests derive executable identity from the compiled surface while
  retaining the common root path.
- Release checks clear ambient `GOFLAGS`, reject research tags in
  `go version -m` metadata, reject research binary names and archive members,
  and replay a hostile `GOFLAGS=-tags=tobari_research` package attempt.
- Standard, release-development, and research package suites prove the exact
  Catalog set relation, release `auth`/`serve` absence, valid version tuples,
  root entry, and research resolver identity.
- The architecture site is generated from the release Catalog and release
  component/version evidence; research contracts remain repository docs.
