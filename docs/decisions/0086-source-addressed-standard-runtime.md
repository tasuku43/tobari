# ADR 0086: Use one source-addressed standard Runtime identity

- Status: Accepted
- Date: 2026-08-26
- Deciders: Tobari product owner and maintainers
- Scope: Product, architecture, security, harness, public repository, release,
  and Runtime infrastructure
- Revises: ADR 0043, ADR 0045, and ADR 0082 where they describe standard Runtime
  local image selection
- Related: ADR 0076, ADR 0080, ADR 0084, and ADR 0085
- Superseded by: None

## Context

The standard Runtime has two resolver channels. The embedded channel prepares
the pinned local base during release-surface startup and the development
channel prepares repository recovery through `task build`. A mutable
`tobari-runtime:dev` name made that preparation distinction look like two
different Runtime authorities. An unversioned `tobari-runtime:base` fallback
had the opposite problem: it was a name without a source identity. Either
spelling could leak into generated Runtime recipes, bindings, helper
extraction, and integration fixtures.

The standard Runtime is also different from Gateway and Auth Broker: it is the
common execution base whose exact checked source inputs are embedded in both
resolver binaries. Equal inputs therefore provide a safe naming key, while
the resolver channel still needs to control when missing material is prepared
and which repository recovery action is valid.

## Decision

`builtin` is the stable public image selector. Resolving that selector for the
standard Runtime derives one local image name:

```text
tobari-runtime:base-<source-id>
```

`<source-id>` is the exact lowercase SHA-256 identity returned for the checked
embedded standard Runtime build-input closure: the embedded `tobari/` image
inputs, `versions.env`, and the verified helper-source closure. It is not a
Git commit, OCI image digest, registry reference, current directory, branch,
dirty state, environment value, or moving tag. The derivation is owned by
`runtimeassets`; both resolver channels and the repository image-name tool use
that same function.

Development and embedded resolvers remain distinct channels. Development
`task build` eagerly prepares the source-addressed standard Runtime and
provides repository recovery. Embedded `cluster up` builds the same name only
when it is absent and uses embedded release recovery. A channel is preparation
and recovery identity, not a second image-name authority. Equal exact source
identity may reuse the same local material name; unequal identity cannot alias.
An immutable Template may retain an older exact standard binding after the CLI
changes. Entry validates and reuses that exact local material; it neither
rebuilds the old name from new source nor silently replaces the binding.

There is no active unversioned `tobari-runtime:base` authority and no mutable
`tobari-runtime:dev` standard Runtime dependency. Existing final-authority
migration guards may recognize the old development spelling only as rejected
predecessor state; they never select, rewrite, or silently migrate it. Existing
managed Runtime source and persisted manifests are not implicitly rewritten.

Durable user configuration keeps the stable selector in `WorkspaceManifest.Image`.
When a Runtime binding exists, `RuntimeBinding.Image` and binding-backed
`ManifestRuntimeReport.Image` are the exact resolved execution material. A
new/default Workspace Manifest therefore persists `builtin` while its standard
binding carries `tobari-runtime:base-<source-id>`. The same separation applies
to resolved build progress and generated recipe base references; selectors are
never placed in material fields. `builtin` may resolve to a differing binding
material, but an explicit portable image selector must equal its binding/report
material; contradictory selector/material pairs are rejected before
presentation.

Before use, the selected standard or custom image still passes the exact
runtime API, lifetime, user, entrypoint, and volume-free filesystem
compatibility checks. Host-side helpers are Tobari-owned assets extracted only from the
current verified canonical standard image into owner-only host state and
mounted read-only into both standard and custom Workspaces. A retained
historical or custom Runtime image neither provides nor determines those
helpers.

The integration scenario derives its default custom base from the canonical
source-addressed name. It may build a missing image only when the requested
name equals that canonical default. An explicitly named custom image must
already exist and is never rebuilt under an arbitrary name; a canonical image
created for the scenario remains a reusable verified local cache.

## Consequences and enforcement

- Runtime image derivation errors propagate through resolver and Runtime
  boundaries; there is no empty, panic, or unversioned fallback.
- Release packaging needs no link-time Runtime image authority; embedded bytes
  derive the same name as development bytes.
- Domain validation rejects `builtin` in resolved Runtime revision, binding,
  build-progress, recipe-material, and official binding-report fields.
- Resolver, runtime lifecycle, helper, integration-ownership, and selector /
  material-separation tests prove the decision. `check-integration-scope.sh`
  guards the custom-image build boundary, and the repository image-identity
  guard rejects active alias dependencies outside historical ADR/evidence.
- Thesis 7 and the Product, Architecture, Security, Harness, Public
  Repository, and Release contracts describe source equality and channel-owned
  preparation consistently. Historical ADR facts remain unchanged.

## Rejected alternatives

- Keep `:dev` as a second alias: this preserves a mutable parent and a
  channel-spelling dependency in generated Runtime source.
- Use a Git SHA: unrelated repository changes and dirty-source limitations make
  it the wrong identity for embedded build inputs.
- Use an OCI image digest: Tobari does not publish an image, and the digest is
  material validation evidence rather than the checked source naming key.
