# Work Plan: Make Context the explicit capability envelope

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Extend the existing logical Context thesis rather than introduce a template or
runtime-backend abstraction. Every exact-V1 Context records immutable
`source_access` and a normalized policy-preset snapshot identity/revision in
addition to its current composition. The two child packets implement those
axes; this packet owns their shared vocabulary, precedence, public reporting,
and durable ADR.

The public mental model is “a host-owned capability envelope.” The security
model continues to describe the actual separate enforcement boundaries:
Docker mount construction, OPA/Gateway policy projection, Auth Broker vault and
handles, runtime image, and read-only profile/config projections.

## Alternatives considered

### Live reusable Context template

A Context could reference a mutable template containing filesystem, policy,
runtime, and credentials. This would make one edit silently affect multiple
Context authorities and create a second source of truth. Rejected for V1.

### Independent runtime and policy flags

Source access could live on each Workspace and presets could be applied
directly to policy. That permits same-Context Workspaces to diverge and weakens
the stable Context/project authority model. Rejected.

### Copy every store into one Context document

This makes the bundle literal but exposes secrets and executable policy to
unnecessary readers and mounts. Rejected; logical composition and physical
separation remain distinct.

## Design

### Public contract

The intended creation grammar is:

```text
tobari context create --name NAME
  [--image IMAGE]
  [--mode guided|advanced]
  [--source-access read-only|read-write]
  [--policy-preset builtin/offline|builtin/reviewed-exact|builtin/get-only-reviewed|custom/NAME]
  [--format text|json]
```

Omission defaults are `builtin` image, `guided` mode, `read-write` source, and
`builtin/reviewed-exact` preset. Every preset guardrail applies before either
guided or Advanced allows. Creation validates every selector and snapshot
before writing the Context; it starts no Docker resource.

`context list` remains a complete exhaustive local collection. Each item adds
source access, policy mode, preset name, and preset revision. `context show`
adds the complete effective guardrail summary and learned-decision count.
Synthetic default output carries explicit default display values but no stable
ID, stores, or snapshotted authority.

No command changes source access or preset revision after creation. A user
creates another Context and chooses it explicitly. `context use` changes only
the default selector. Same-root Contexts remain separate Workspaces.

`context.composition` remains the capability ID for create/list/show/use and
source access. `policy.presets` owns preset discovery/creation/validation.

### Layer changes

- Domain: capability-envelope vocabulary, immutable source/preset manifest
  facts, reports, defaults, and cross-field validation.
- Application: complete create input and task-result validation; no outer-layer
  defaults or selector inference.
- Infrastructure: separate store resolution, snapshot binding, mount/policy
  projection, and exact Context persistence.
- CLI and catalog: new create inputs and report fields with stable defaults,
  enums, prerequisites, failures, and help.

### Data and control flow

```text
trusted host create inputs
        ↓ validate complete envelope
load built-in/custom preset → normalize → validate → digest
        ↓
atomically persist Context manifest + separate preset snapshot/policy source
        ↓
future root entry resolves stable Context ID
        ├─ source access → exact Docker bind option
        ├─ preset guardrail → aggregate OPA projection
        ├─ runtime/profile/projections
        └─ credential eligibility → project-bound handle reconciliation
```

### Error and cancellation behavior

- Invalid name, source access, mode, preset selector, preset content, digest,
  image, or existing Context fails before state creation.
- Partial Context/preset snapshot creation is recovered or rejected through one
  atomic Context-store boundary; it never falls back to implicit policy.
- Cancellation before persistence has zero mutation. A confirmed creation is
  rendered through the mutation-complete boundary and is never advertised as
  safe to replay after late cancellation/output failure.
- Missing or changed source preset after creation does not affect the Context.
- Unsupported development manifests fail closed under ADR 0027.

### Security and public boundary

The manifest remains secret-free and non-executable. Stable ID, source access,
preset identity/digest, runtime identity, and projection choices are safe
authority metadata. Credential values, broker handles, root keys, arbitrary
Rego, and resolved host paths do not become generic manifest inputs.

## Implementation slices

1. Accept and promote the capability-envelope ADR and vocabulary.
2. Implement the source-access child packet.
3. Retire policy compaction and implement the policy-presets child packet.
4. Integrate create/list/show, project reconciliation, policy projection,
   readiness documentation, and harness claims.

## Verification

- Unit and contract tests: manifest/report cross-field invariants and defaults.
- Negative side-effect tests: invalid envelope performs no file or Docker I/O.
- Agent-readiness: one scoped Context help request exposes all creation inputs;
  no source inspection or preset-notation reconstruction.
- Manual observation: create read-only/offline and read-write/reviewed Contexts
  for one root and inspect their independent reports and runtime effects.
- Required profiles: `task check`, `task security`, and `task public:check`.

## Rollout and rollback

The resulting repository is the sole pre-public V1. Existing development
Contexts lack required envelope fields and are removed/recreated; no defaults
are injected while reading old state. Before publication, rollback requires a
source revert and development-state recreation.

## Documentation promotion

- Revise Context theses and ADR 0013 consequences through a new ADR.
- Update product command/report and Workspace-key contracts.
- Update architecture and security precedence diagrams.
- Add executable claim rows for immutable source access, preset snapshots, and
  guardrail precedence.
- Update readiness scenarios for choosing and inspecting Context constraints.
