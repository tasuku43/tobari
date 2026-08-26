# ADR 0066: Make Context-owned policy the only user-facing network policy

- Status: Accepted
- Date: 2026-08-19
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, policy, runtime, harness, and public boundary
- Revises: ADR 0029, ADR 0039, and ADR 0059
- Related: ADR 0060
- Revised by: ADR 0070 adds one explicit migration for the enumerated
  unpublished built-in preset snapshot; ADR 0087 removes the Advanced-policy
  seam and retains one canonical typed policy authority
- Superseded by: None

## Context

The public `policy preset` capability was introduced to give Context creation
reusable method and destination postures. The Context creation wizard now
collects a complete HTTP method default and exact overrides itself. Creation
then loads a preset and composes that user-owned method policy back into the
preset snapshot. The selector and custom catalog therefore add a second policy
source without owning a distinct user outcome.

The built-in `agent-ready` data is different: it is a fixed, reviewed set of
native-client compatibility routes maintained with the trusted binary. It is
not a reusable user profile and must remain available to make the default
Context usable.

## Decision

Retire `policy.presets` from the public catalog. Remove the `policy preset`
commands, the custom preset store, and `context create --policy-preset`. Do not
retain hidden aliases or an undocumented compatibility fallback.

Context creation owns the network method policy. The guided wizard and direct
mode produce one complete default plus exact overrides, and infrastructure
composes that value against one fixed compile-time agent-ready baseline. The
result is normalized, revisioned, and written to the Context-owned policy
store before the create mutation reports success.

The public Context manifest and report expose `policy_revision` and the
effective `method_policy`. They do not expose a preset origin or a guardrail
label that implies another selectable source. Native readiness remains a
separate Context setting and continues to select the bounded trusted-binary
overlay. The aggregate system router continues to enforce destination, method,
baseline-deny, exact-policy, and Advanced-policy ordering; this decision does
not make any HTTP method intrinsically safe.

The fixed baseline remains data-only and immutable at the binary revision. A
Context snapshot is still the authority boundary: changing the binary or
native-readiness bundle does not mutate the stored Context method policy or
baseline bytes. The current native readiness overlay may be applied at
aggregate generation according to the existing reviewed contract.

This is a pre-public V1 contract replacement. Current readers ignore existing
unpublished local Context manifests and custom preset files. ADR 0070 permits
one explicit mutation to decode only the enumerated built-in predecessor;
arbitrary custom presets remain unsupported and no ordinary read path gains a
fallback.

## Consequences

- Context creation has one visible policy decision: method behavior, with
  native readiness shown separately.
- `agent-ready` compatibility remains available without a profile namespace or
  user-maintained catalog.
- Context reports and machine contracts become smaller and no longer invite
  origin/name inference about the active policy.
- Existing development tests and fixtures must use Context policy snapshots;
  old custom catalog journeys are removed.
- A future reusable profile, remote policy catalog, or custom policy editor
  would be a new capability and would require a new ADR.

## Mechanical enforcement

- `cli.Catalog` contains no `policy preset` command and `context create` has no
  `--policy-preset` input.
- The capability ledger contains no public `policy.presets` entry, and catalog
  validation rejects retired paths through the normal unknown-command flow.
- Context manifest validation requires one `policy_revision`; snapshot bytes
  are owner-only, normalized, and digest-bound to that revision.
- Context creation tests prove the supplied method policy is present in the
  persisted snapshot and that a later snapshot mismatch fails closed.
- Aggregate tests prove the fixed baseline and native overlay retain terminal
  method/destination and exact-deny precedence.
- Site generation and public-boundary checks prove the retired selector and
  catalog do not reappear in generated documentation.

## Compatibility and migration

No public V1 release has shipped this capability. The old public commands,
selector, arbitrary custom files, and unknown manifest fields are not
compatibility inputs. ADR 0070 is the explicit bounded exception for one
enumerated built-in predecessor; unrelated Context reads still cannot delete
or reinterpret old state.

## Security and public-boundary impact

The public authority surface narrows: users cannot activate an arbitrary
custom policy document or select a named method profile. The fixed baseline
remains inside the trusted binary and contains no credentials, executable code,
wildcards, or remote source. No new destination, process identity, provider
binding, or external dependency is introduced.
