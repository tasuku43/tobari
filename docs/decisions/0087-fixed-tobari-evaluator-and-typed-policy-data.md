# ADR 0087: Keep the Tobari evaluator fixed and policy data canonical

- Status: Accepted
- Date: 2026-08-26
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, architecture, security, state, migration, harness,
  and public boundary
- Revises: ADR 0028, ADR 0029, ADR 0030, ADR 0059, ADR 0060, ADR 0066,
  ADR 0071, and ADR 0084 at their executable-policy and policy-mode seams
- Related: ADR 0024, ADR 0028, ADR 0075, and ADR 0085
- Superseded by: None

## Context

The pre-public policy model exposed two sources of authority: canonical typed
policy data and a Template-selected Advanced source pair. That split made the
evaluator part of user-owned state, allowed policy-mode vocabulary to leak into
the public contract, and made a persisted Advanced final envelope ambiguous at
the clean-break boundary. It also combined the identity of the code that
interprets policy with the identity of the data it interprets.

The product thesis is that ordinary users review declarative permission data;
they do not maintain an executable policy program. Tobari must therefore own
the evaluator, its tests, and its bundle material, while retaining a complete,
auditable, and independently drift-detectable policy lifecycle.

## Decision

### One fixed evaluator and one canonical data authority

The final Template and Context authorities contain only canonical typed policy
data. Policy-mode selection and `advanced_policy` are not final authority
fields. Context policy storage is the typed domain tree and its canonical JSON
data; no user-owned Template, Context, or configuration path may contain a
`.rego` file. The embedded evaluator and its tests may exist in the Tobari
repository and in Docker-managed internal bundle material, but those paths are
implementation details and are never a user-editable source, copy target,
pointer, help path, or remediation path.

`cluster up` and reconciliation always materialize the complete Tobari-owned
evaluator together with the complete canonical policy data. Candidate
validation, content-addressed publication, atomic activation, known-good
rollback, and the deny-all transition for reducing or mixed changes remain the
activation boundary. A failed candidate cannot replace the last known-good
aggregate.

### Independent identities within the existing lifecycle

Every aggregate records both of these typed identities:

- `PolicyEvaluatorIdentity` contains evaluator schema, version, and the digest
  of the embedded evaluator bytes.
- `PolicyDataIdentity` contains the policy-data schema and the digest of the
  complete sorted canonical Context data.

Both identities are carried through desired aggregate projection, materialized
`data.json`, aggregate revision, active/publication receipt, and applied shared
state. They are validated as a pair and must agree across desired and applied
receipts. A remembered decision changes only `PolicyDataIdentity`; changing
evaluator bytes or version changes only `PolicyEvaluatorIdentity`; either
change changes aggregate readiness and the combined aggregate revision.

Decision data remains structured around the existing layers: terminal Boundary,
the combined exact-Deny tier, Template static positive authority, remembered
reviewed Allow, and unresolved review/default deny. Tests may expose layer,
rule, effect, and default facts where the existing typed decision contract
already carries them; this decision does not introduce a second policy
language or a new public explanation feature.

### Exact authority order and protocol boundary

For ordinary external traffic, precedence is exactly:

1. terminal destination/method Boundary;
2. one terminal exact-Deny tier containing trusted baseline Deny and remembered
   exact Deny, with no ordering between those members;
3. Template static positive authority;
4. remembered reviewed Allow;
5. unresolved review/default deny.

Host Loopback is a separate attachment-owned branch and is neither ordinary
Template/Context authority nor a fallback from it. Gateway retains its
protocol-derived classification for GraphQL, MCP, AWS, Kubernetes, Git, OCI,
and other classified traffic. A classified request cannot fall through to a
coarse HTTP rule.

### Persisted Advanced clean break

The final authority reader performs a bounded, non-authorizing marker check
before strict decoding. It recognizes the persisted V1 final-envelope forms
`policy.mode=advanced` and `policy.advanced_policy`, without decoding,
normalizing, translating, or executing their source. The result is the stable
`ErrLegacyExecutablePolicy` failure with deterministic reset/recreate guidance;
the read and all subsequent reconciliation remain zero-mutation. Context and
predecessor migration reject the same legacy authority explicitly.

This is a deliberate reconciliation with ADR 0084's clean-break rule and ADR
0084's no-predecessor-decoder rule: the bounded detector is a rejection guard,
not a predecessor decoder. It does not adopt UUIDs, revisions, policy rules,
homes, credentials, or executable source. A legacy state cannot be silently
normalized to the fixed evaluator.

## Consequences and verification

The public Catalog, human help, schemas, fixtures, and architecture-site
guidance describe typed policy data and the fixed evaluator only. Internal
repository evaluator files remain checked source inputs and are not user state.
Contract tests cover legacy final-envelope rejection without mutation, absence
of `.rego` in user policy layouts and copy paths, identity independence and
receipt agreement, exact precedence, classified-traffic no-fallback behavior,
atomic activation, rollback, and deny-all transitions. The pinned
`docs/architecture-site/source-snapshot.txt` publication evidence is not
regenerated by this decision.
