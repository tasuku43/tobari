# ADR 0029: Make Context the immutable capability envelope

- Status: Accepted
- Date: 2026-08-12
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, harness, and public boundary
- Supersedes: None
- Superseded by: ADR 0079 supersedes the Context vocabulary and flat aggregate
  lifecycle while retaining the immutable Boundary security invariant
- Revises: ADR 0010, ADR 0013, ADR 0024, ADR 0026, and ADR 0028
- Revised by: ADR 0066 removes public policy presets and ADR 0071 limits the
  envelope's immutability to the creation-time Boundary while retaining a
  stable reusable Context identity; ADR 0079 retains that Boundary under a
  Workspace Manifest identity; ADR 0087 removes the Advanced/executable-policy
  seam while retaining the terminal Boundary invariant

## Context

A Context already binds one stable identity to runtime image, guided or
Advanced policy, agent profile, narrow shell/Git projections, policy source,
managed credential metadata, and separately stored broker state. Two authority
facts remain implicit: every project source bind is read-write, and Context
creation copies one unnamed embedded policy containing example domains.

Adding independent per-Workspace flags would let Workspaces with one Context
diverge. Keeping a live preset reference would let later file edits silently
change existing authority. Combining stores into one document would expose
policy or credentials to readers and mounts that do not need them.

The implementation also disagrees with its current authorization claim:
Gateway carries scheme, query, and redacted headers, but guided candidate and
learned-rule identity omits scheme. Advanced Rego can inspect query and header
input. V1 must distinguish the stable Tobari-owned permission identity from
additional owner-authored constraints before preset precedence is added.

## Decision drivers

- Make the full user-chosen boundary visible before a Workspace exists.
- Preserve stable Context/project authority and physical trust separation.
- Keep direct source binding without adding clone, overlay, or apply-back.
- Replace implicit example authority with an immutable named policy origin.
- Ensure a terminal guardrail cannot be bypassed by learned or Advanced policy.
- Make omission defaults creation-time facts, never compatibility readers.

## Decision

Context is the logical, host-owned capability envelope for every Workspace
permanently bound to its stable ID. It composes physically separate boundaries;
the manifest is not itself an executable policy, credential store, generic
mount specification, or secret container.

Every persisted exact-V1 Context manifest has these required immutable fields:

- `source_access`: exactly `read-only` or `read-write`;
- `policy_preset_origin`: one normalized `builtin/<name>` or `custom/<name>`;
- `policy_preset_revision`: lowercase `sha256:<64-hex>` over the complete
  normalized schema-V1 preset snapshot.

`context create` owns the only omission defaults: `read-write` and
`builtin/reviewed-exact`. It validates all inputs, normalizes and validates the
selected preset, computes its revision, and atomically persists the manifest
and Context-owned snapshot before reporting success. Reading an old, missing,
partial, mismatched, or unsupported manifest/snapshot fails closed. No reader
invents either default and no source-preset edit changes an existing Context.

The selected source access controls only the one direct project-root bind.
`read-only` adds Docker read-only authority to that bind; the Workspace home and
tmpfs remain writable. It is a live view, not a snapshot: host processes or a
read-write Context for the same root may change bytes that the read-only
Workspace observes. No writable alias of the selected root is allowed.

The preset snapshot owns a terminal network guardrail plus any Context-wide
exact baseline data. The Tobari-owned system evaluator applies this precedence:

```text
terminal preset guardrail
  -> preset baseline deny
  -> exact learned deny
  -> preset baseline grant
  -> exact learned allow or Advanced owner Rego
  -> default deny / exact-review eligibility
```

Any terminal denial finishes locally before permission-candidate creation,
external DNS, broker resolution, or upstream connection. Advanced Rego may
further constrain generic query/header input, but it cannot grant outside the
preset guardrail, change the Tobari-owned learned identity, or turn query or
headers into a learned permission dimension.

The ordinary Tobari-owned HTTP permission identity is stable Context, project,
scheme, host, port, method, and raw path. Query, headers, and body are excluded.
Declared GraphQL endpoints add operation type and root field but exclude source,
operation name, arguments, variables, and other payload values. Guided denial,
candidate, rule IDs, matching, and audit must therefore add scheme before the
policy-preset child is integrated.

`context list` and `context show` report `source_access`,
`policy_preset_origin`, and `policy_preset_revision`. `context show` also
reports a task-owned `policy_guardrail` summary and learned-decision count;
details remain available through policy/preset commands rather than inferred
from labels. Synthetic default output shows explicit creation defaults but no
stable ID, revision, snapshot, or enforcement authority.

No command mutates `source_access`, preset origin, or preset revision.
`context use` changes only the default used when a later invocation omits a
Context. Existing and running Workspaces remain bound to their stored stable
Context ID. A different envelope requires a new Context.

## Consequences

### Positive

- Users can compare source and network ceilings before entering a Workspace.
- Same-root read-only and read-write Workspaces can coexist under different
  Context IDs without a second source-binding mechanism.
- Preset source changes are reviewable future input, not ambient live authority.
- Learned exact rules and Advanced Rego share one unavoidable upper guardrail.

### Negative

- Pre-public Contexts must be deleted and recreated.
- Context report and manifest schemas gain required fields.
- Source mount reconciliation and policy activation must include the immutable
  facts in their exact desired-state hashes.
- Scheme must be added to guided learned-policy state and identifiers before V1.

### Risks and mitigations

- A writable alias could invalidate source access. Enumerate exact mounts,
  inspect Docker HostConfig, and exercise nested home-relative roots.
- A stale or missing preset snapshot could cause implicit authority. Bind the
  manifest to normalized bytes by digest and fail before activation.
- Advanced Rego could bypass a guardrail if routed directly. Keep guardrail
  evaluation in the Tobari-owned system package and add terminal zero-call
  canaries for candidates, DNS, Broker, and upstream.
- Reports could imply snapshot integrity. Name direct live access explicitly
  and test same-root cross-Context observation.

## Mechanical enforcement

- Domain constructors require the closed source-access enum and normalized
  preset origin/revision in every persisted manifest, summary, and report.
- Catalog tests derive create defaults, enums, scoped help, and recursive output
  fields from `cli.Catalog`; invalid inputs cause zero state/Docker calls.
- Context-store tests prove atomic snapshot/digest binding, owner-only regular
  files, immutable update behavior, and unsupported-state rejection.
- Runtime spec/hash, create argv, inspect, and integration tests distinguish the
  source bind while preserving writable home/tmpfs and no writable alias.
- OPA/Gateway tests place the guardrail above baseline, exact learned, and
  Advanced paths and prove terminal denial has zero candidate/DNS/Broker/
  upstream calls.
- Agent-readiness tests create and inspect contrasting envelopes through scoped
  help with zero source reconstruction.

## Compatibility and migration

ADR 0027 applies. This repository becomes the only V1 contract; unpublished
development Contexts are explicitly removed and recreated. No manifest or
policy reader fills absent fields, migrates old state, or preserves the prior
implicit example policy.

## Security and public-boundary impact

The manifest and preset identity are non-secret authority metadata. The preset
snapshot is strict owner-only non-executable data and remains physically
separate from runtime, Rego, credential vaults, root keys, and Workspace home.
No new destination, credential, process, dependency, or provider is introduced.

## Validation

```sh
task check
task security
task public:check
```

Docker integration additionally proves live read-only/read-write same-root
behavior and guardrail terminal zero-call ordering.

## Reconsideration signals

Reconsider if V1 requires clone/overlay/apply-back, mutable organizational
templates, remote presets, Context inheritance, resource selection, temporary
permissions, or a policy identity that includes query/header/body values. Each
would require a new product and trust-boundary decision rather than a manifest
escape hatch.
