# Work Plan: Make the Context Boundary explicit

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Extend the existing logical Context thesis rather than introduce a template or
runtime-backend abstraction. Every exact-V1 Context records immutable
`source_access` and normalized Context-owned policy identity/revision as its
creation-time Boundary. The same stable Context separately owns one exact
mutable Runtime binding, shell/Git session defaults, and a future-Workspace
bootstrap creation default. The child packets retain their unfinished
supported-platform evidence; this packet owns shared Boundary vocabulary,
precedence, public reporting, and durable ADR history.

The reconciled public mental model is “a stable reusable work mode with an
immutable creation-time Boundary.” The security model continues to describe
the actual separate enforcement boundaries:
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

The current creation grammar is:

```text
tobari context create --name NAME
  [--runtime standard|NAME@ORDINAL]
  [--source-access read-only|read-write]
  [--native-readiness enabled|disabled]
  [--bootstrap-aws-profile NAME]
  [--bootstrap-eks-context NAME]
  [--format text|json]
```

The interactive flow reviews `standard@1`, the canonical typed policy-data
defaults, `read-write`, enabled
native readiness, and no bootstrap as initial values. Complete direct creation
requires the declared group. Context creation validates every selector and the
normalized Context-owned policy snapshot before writing state; it starts no
Docker resource.

`context list` remains a complete exhaustive local collection. Each item adds
source access, policy-data identity/revision, complete method policy, native-readiness
choice, exact Runtime binding, and bootstrap state. `context show` adds the
complete shell/Git session-default inventory and separated diagnostic facts.
Synthetic default output carries explicit default display values but no stable
ID, stores, or snapshotted authority.

No command changes the Boundary after creation. `context runtime set`, `config
shell`, `config git`, and bootstrap commands mutate only their named component
with the activation timing fixed by ADR 0071. A user creates another Context to
change source/network Boundary authority. `context use` changes only the
default selector. Same-root Contexts remain separate Workspaces.

`context.composition` remains the capability ID for create/list/show/use and
the Runtime/session-default mutations. `context.workspace-bootstrap` owns the
closed future-Workspace creation-default workflow. There is no public policy
preset catalog under ADR 0066.

### Layer changes

- Domain: stable work-mode vocabulary, immutable Boundary manifest facts,
  exact Runtime/default reports, and cross-field validation.
- Application: complete create input and task-result validation; no outer-layer
  defaults or selector inference.
- Infrastructure: separate store resolution, snapshot binding, mount/policy
  projection, and exact Context persistence.
- CLI and catalog: new create inputs and report fields with stable defaults,
  enums, prerequisites, failures, and help.

### Data and control flow

```text
trusted host create inputs
        ↓ validate complete work mode and Boundary
compose Context-owned policy → normalize → validate → digest
        ↓
atomically persist Context manifest + separate policy snapshot/source
        ↓
future root entry resolves stable Context ID
        ├─ source access → exact Docker bind option
        ├─ policy ceiling → aggregate OPA projection
        ├─ Runtime binding → next-entry reconciliation
        ├─ shell/Git defaults → entry/session resolution
        ├─ bootstrap default → future Workspace creation only
        └─ credential eligibility → project-bound handle reconciliation
```

### Error and cancellation behavior

- Invalid name, source access, policy content/digest, Runtime binding, or
  existing Context fails before state creation.
- Partial Context/policy snapshot creation is recovered or rejected through one
  atomic Context-store boundary; it never falls back to implicit policy.
- Cancellation before persistence has zero mutation. A confirmed creation is
  rendered through the mutation-complete boundary and is never advertised as
  safe to replay after late cancellation/output failure.
- Missing or changed policy source after creation does not affect the Boundary.
- Unsupported development manifests fail closed under ADR 0027.

### Security and public boundary

The manifest remains secret-free and non-executable. Stable ID, source access,
policy identity/digest, Runtime identity, and projection choices are safe
authority metadata. Credential values, broker handles, root keys, executable
source, and resolved host paths do not become generic manifest inputs.

## Implementation slices

1. Accept and promote the Boundary ADR and stable-work-mode vocabulary.
2. Implement the source-access child packet.
3. Retire policy compaction and the public policy-preset contract.
4. Integrate create/list/show, project reconciliation, policy projection,
   readiness documentation, and harness claims.

## Verification

- Unit and contract tests: manifest/report cross-field invariants and defaults.
- Negative side-effect tests: invalid envelope performs no file or Docker I/O.
- Agent-readiness: one scoped Context help request exposes all creation inputs
  and mutable component timing; no source inspection or notation reconstruction.
- Manual observation: create contrasting read-only and read-write Contexts
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
