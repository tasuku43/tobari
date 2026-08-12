# Work Plan: Create Contexts from bounded policy presets

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Parent decision: [Context capability envelope](../context-capability-envelope/plan.md)

## Chosen approach

Add one strict local preset catalog with immutable embedded built-ins and
owner-only custom schema-V1 documents. A Context never holds a live reference:
creation resolves one selector, validates and canonicalizes the complete
document, computes its digest, and atomically snapshots it into Context-owned
policy state. The manifest/report retains origin and revision for explanation.

Add a system-enforced guardrail above all decision modes. Effective authority
is:

```text
guardrail permits effect
AND no terminal baseline/exact deny matches
AND one of:
    Context-wide exact baseline grant
    project-bound exact learned grant
    Advanced Rego allow
```

An unmatched but guardrail-eligible effect may become an exact candidate under
the existing learnability constraints. A guardrail rejection is terminal and
non-learnable.

## Alternatives considered

### Preset as initial data only

If `get-only` only seeds current policy, later `policy allow` or Advanced Rego
could grant POST. That contradicts the Context constraint users selected.
Rejected.

### Live preset references

Convenient fleet updates create cross-Context blast radius and a second
authority source. Rejected for local V1; snapshots are stable.

### Provider-oriented built-ins

GitHub/npm/pip presets would require endpoint drift, wildcard decisions, and
provider maintenance before public evidence. Deferred.

### Arbitrary Rego presets

This duplicates Advanced mode, introduces executable code into the reusable
preset boundary, and weakens validation. Rejected; custom presets are data.

## Design

### Public contract

New commands:

| Command | Role/effect | Outcome |
|---|---|---|
| `policy preset list [--format text|json]` | utility/read | Return the exhaustive installed built-in/custom preset catalog with kind, name, revision, and guardrail summary. |
| `policy preset show --name NAME [--format text|json]` | utility/read | Return one exact preset contract, including scope and limitations. |
| `policy preset init --name NAME [--format text|json]` | act/create, fixed preset-catalog target | Create one owner-only `custom/NAME` template without overwriting. |
| `policy preset validate --name custom/NAME [--format text|json]` | utility/read | Strictly validate and digest one custom source without changing a Context or active policy. |

`policy.presets` is a new public capability. These commands produce/consume no
opaque action reference: names are exact local selectors and init creates
within the one catalog-owned fixed target. Context creation accepts one
`--policy-preset` selector and snapshots it in the separate Context creation
mutation.

`init` writes a complete valid deny-all custom template, returns its canonical
selector, owner-only source path, normalized revision, and exact validation
facts, and never opens an editor or executes content. The user may use it
unchanged or edit the documented schema and run `validate`; routine setup does
not require source-code inspection or an undeclared parser. Recovery actions
remain exact catalog paths rather than embedding unchecked argument values.

Canonical selectors are:

- `builtin/offline`
- `builtin/reviewed-exact`
- `builtin/get-only-reviewed`
- `custom/<portable-name>`

Built-in semantics:

| Built-in | Immediate grants | Reviewable effects | Terminal effects |
|---|---:|---|---|
| `offline` | none | none | all HTTP/HTTPS |
| `reviewed-exact` | none | eligible exact effects inside existing public/private/protocol boundaries | effects outside those boundaries |
| `get-only-reviewed` | none | eligible exact `GET` effects inside the destination boundary | `HEAD`, every non-GET, and effects outside the destination boundary |

Custom schema V1 is a closed non-executable document with:

- canonical name and explicit schema version;
- method ceiling: `all` or a sorted unique explicit method set;
- destination ceiling: explicit `public_https` scope or a sorted unique exact
  public `scheme + host + port` set; plain HTTP must always be exact;
- zero or more exact Context-wide baseline grants, each binding scheme, host,
  port, method, and path and proven inside both ceilings;
- zero or more baseline denies using the existing exact-host/method/path-prefix
  matcher and proven inside the declared destination space;
- zero or more exact GraphQL classification endpoints; and
- no learned rules, project IDs, credential bindings, wildcard/IP/private
  destinations, executable fields, includes, inheritance, or remote source.

The exact schema is frozen only after strict/hostile tests. Canonicalization
sorts semantically unordered collections, rejects duplicates rather than
silently merging, and hashes the complete normalized bytes. Built-in revisions
use the same normalized representation and validator.

`context show` distinguishes preset facts from learned state. Required facts
include origin selector, revision, method ceiling, destination scope/count,
Context-wide grant count, baseline deny count, GraphQL endpoint count, and
project-bound learned allow/deny counts. It does not label GET or any grant
safe/read-only.

### Layer changes

- Domain: preset selector/kind, strict document, normalized snapshot, digest,
  guardrail match, baseline grant/deny invariants, reports and counts.
- Application: list/show/init/validate use cases with minimal preset-store ports;
  Context create consumes one complete normalized snapshot.
- Infrastructure: embedded built-ins, owner-only custom store, safe parser,
  canonical encoding/digest, atomic init, Context snapshot/domain generation,
  aggregate projection and guardrail enforcement.
- CLI and catalog: four command specs, new capability ID, create input, Context
  report fields, complete outputs/faults/help, dispatch and presentation.

### Data and control flow

```text
policy preset init → owner edits strict local data
          ↓
policy preset validate/list/show (read-only, no activation)
          ↓
context create --policy-preset exact-selector
          ↓ resolve → strict parse → normalize → digest
          ↓ atomic Context snapshot + manifest
cluster up / policy mutation
          ↓ complete all-Context aggregate
Gateway request → system guardrail
          ├─ terminal deny: audit only, zero candidate/DNS/Broker/upstream
          └─ eligible: deny precedence → baseline/learned/Advanced decision
```

### Error and cancellation behavior

- Unknown selector, duplicate/unknown field, unsafe file, invalid ceiling,
  out-of-ceiling grant/deny/endpoint, oversize, or digest mismatch fails before
  Context mutation or policy activation.
- `init` never overwrites. Existing-file and cancellation outcomes are typed;
  a confirmed create is not replayable after output failure.
- `validate`, `list`, and `show` never write or activate policy.
- Missing/deleted origin after Context creation does not affect the snapshot.
- Guardrail denial is non-learnable and cannot be recovered through `policy
  allow`; the user chooses another Context. It causes zero downstream I/O.
- Policy activation keeps current atomic revision confirmation and fencing.

### Security and public boundary

Preset content is authority-bearing but secret-free, so files are owner-only
and outputs may expose normalized policy facts after visible projection. No
third-party content, network fetch, provider account, token, executable, or
new dependency is introduced. Advanced Rego is always subordinate to the
Tobari-owned guardrail.

## Implementation slices

1. Retire learned prefix/compaction code and contracts so presets target the
   exact-rule-only V1 model.
2. Add strict domain schema, embedded built-ins, and hostile parser fixtures.
3. Add custom preset store plus list/show/init/validate application/CLI slices.
4. Add Context selector/snapshot/report and atomic creation behavior.
5. Add aggregate guardrail/preference semantics and zero-call tests.
6. Add integration/readiness/manual observations and promote documentation.

## Verification

- Domain/schema tests: all fields, canonicalization, digest, ordering,
  duplicates, ceilings, grants, denies, endpoints, selectors and size bounds.
- Store tests: owner modes, symlink/race rejection, no overwrite, atomic write,
  source change after snapshot, built-in immutability.
- Catalog tests: command roles/effects/outputs/faults, fixed init target, input
  grammar, capability ledger, dispatch/help parity.
- Policy tests: each built-in matrix; grant inside/outside ceiling; deny
  precedence; learned and Advanced bypass canaries; GraphQL classification;
  terminal non-learnability.
- Zero-call tests: terminal denial performs no candidate persistence, DNS,
  Broker resolution, refresh, or upstream call.
- Agent readiness: discover/select/inspect presets without source inspection;
  explicit cloud-agent incompatibility under offline/GET-only.
- Required profiles: `task check`, `task security`, `task public:check`.

## Rollout and rollback

No old Context is inferred to use a preset. Development Contexts are removed
and recreated under ADR 0027. Built-ins are immutable within V1; changing bytes
requires a new built-in identity or explicit compatible contract. Custom source
changes affect only future Context creation. Before publication, rollback is a
source revert plus development-state recreation.

## Documentation promotion

- Context/policy theses: guardrail, baseline grant, learned exact permission,
  and snapshot lifecycle.
- Product contract: commands, selectors, outputs, defaults, errors and
  non-learnable recovery.
- Architecture/security/threat model: system precedence, atomic projection,
  strict store, blast radius, GET limitations, cloud-agent effects.
- Harness/capability/schema ledgers and readiness scenarios.
- New durable ADR linked from ADRs 0013, 0026, and 0028.
