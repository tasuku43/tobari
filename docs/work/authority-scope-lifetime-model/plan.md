# Work Plan: Explain every authority by scope, lifetime, owner, and precedence

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Preserve the existing evaluator and authority types. Add one routine-user
three-layer explanation and one contributor/operator inventory derived from
the actual policy branches. Map every Scope/Lifetime/Owner/Precedence claim to
existing executable evidence and add only missing negative canaries.

The routine model is:

```text
Access                    Context-owned ceiling and resulting routine behavior
Remembered decisions      persistent Context + Workspace Allow / exact Deny
This-session host access  attachment-scoped exact Host Loopback decisions
```

## Alternatives considered

### Expose every implementation source to routine users

Showing snapshot, baseline, readiness overlay, learned rules, Advanced Rego,
epoch, and grant independently is technically complete but makes users learn
internal composition before they can assess ordinary access. Rejected.

### Collapse all positive authority into Context

This would make Context appear to freeze current-binary compatibility,
Workspace decisions, and attachment authority despite their different owners
and lifetimes. Rejected because it is false and weakens reviewability.

### Treat Host Loopback as an ordinary learned permission

This would permit a temporary physical-host exception to become persistent or
template authority. Rejected by ADR 0049 and current typed lifetime contracts.

## Design

### Public contract

No command, role, input, output schema, reference kind, effect, or mutation
contract changes. Routine prose teaches three layers. Detailed technical docs
retain these sources:

| Source | Scope | Lifetime | Owner | Precedence |
|---|---|---|---|---|
| Context destination/method Boundary | Context | Context lifetime | host user at creation | terminal before all positive authority |
| trusted baseline deny | Context | snapshot/binary contract | Tobari | before learned and Advanced positive authority |
| exact remembered Deny | Context + Workspace + exact effect | until reset | trusted-host user | before baseline, learned, and Advanced Allow |
| baseline grants | Context exact reviewed effects | snapshot revision | Tobari | inside ceiling and exact Deny |
| native readiness | enabled Context | installed compatibility revision | Tobari contract plus creation-time enablement | inside ceiling and exact Deny |
| remembered Allow | Context + Workspace + exact/reviewed template | until reset | trusted-host user | cannot exceed ceiling or Deny |
| Advanced Rego | Context evaluator | host policy revision | advanced host user | cannot exceed ceiling or exact Deny |
| Attachment Grant | Context + Workspace + epoch + exact Host Loopback effect | owning attachment | trusted-host user; route owned by host process | separate closed Host Loopback branch |
| default deny | every unresolved effect | always | Tobari evaluator | final fail-closed result |

Exact wording and any order distinction are corrected from code/OPA evidence
during implementation rather than inferred from this planning table.

### Layer changes

- Domain: no new authority abstraction unless a missing typed distinction is
  discovered; preserve current lifetime/kind validation
- Application: no use-case change; preserve bounded discovery and reviewed
  mutation ownership
- Infrastructure: no evaluator or relay change except a missing negative
  canary exposing an existing contract defect, which must stop for redesign
- CLI and catalog: detailed descriptions only where current scope/lifetime
  wording is incomplete; ordinary presentation changes remain deferred

### Data and control flow

```text
ordinary external effect
  → validate principal and Context ceiling/method decision
  → trusted deny / exact remembered deny
  → baseline or native grant / remembered Allow / Advanced policy
  → default deny or exact-review eligibility

Host Loopback effect
  → validate active Workspace principal + Attachment Epoch + route
  → validate Context ceiling/method decision
  → exact attachment Allow / Deny
  → missing, stale, or mismatched authority denies without host target I/O
```

### Error and cancellation behavior

Unchanged. Denial remains fail-closed and secret-free. Staging grants no
authority. Apply revalidates opaque candidates and never retries the denied
request. Attachment cleanup closes the physical route before registry/policy
removal so stale projection data remains inert.

### Security and public boundary

This packet changes security explanation and test coverage, not authority.
Every positive source stays below Context ceilings and exact Deny. Host
Loopback remains loopback-only, HTTP-only, exact, token-protected, and
attachment-scoped. Public evidence retains typed lifetime and destination kind
without secrets or transport tokens.

## Implementation slices

1. Build an evidence matrix from evaluator code, policy, Gateway, domain, and
   integration tests.
2. Resolve the recorded ordering/Advanced-Rego unknowns against code and ADRs.
3. Promote the three-layer model and technical inventory through governing
   docs and detailed catalog descriptions.
4. Add missing canaries and claim-to-enforcement rows.
5. Replay policy/Host Loopback readiness scenarios and required gates.

## Verification

- Unit and contract tests: Context composition, learned identity and reset,
  typed lifetime/kind, attachment registry/grant validation
- Negative side-effect tests: terminal ceiling and stale/mismatched attachment
  produce zero candidate/DNS/Broker/upstream/host-target I/O as applicable
- Opaque-reference and complete-pagination tests: candidates and rule IDs remain
  unchanged; bounded complete projections retain scope/lifetime facts
- Structured output, hostile-output, and recovery tests: labels cannot create
  authority; exact next commands and redaction remain
- Agent-readiness scenario and discovery-round-trip count: routine explanation
  requires no policy-source inspection or join
- Human-handoff scorecard: not applicable; no new setup/auth transfer
- Manual observation: optional local synthetic denied/Allow/Deny/Host Loopback
  journey after deterministic evidence passes
- Required profiles: `task check` and `task security`
- Generated-diff or artifact checks: harness claim table and generated
  architecture/security references as applicable

## Rollout and rollback

No runtime state or public schema migration is planned. Documentation and
missing-test changes are source-revertible. If evidence shows the actual
evaluator conflicts with the accepted precedence, stop and revise the thesis or
ADR before changing enforcement.

## Documentation promotion

- Add the routine three-layer explanation to product guidance.
- Add one complete authority table to architecture/security.
- Explain native readiness as current-binary compatibility bounded by Context.
- Add claim-to-enforcement mapping for every precedence edge and lifetime end.
- Keep progressive-disclosure UI and service exposure in separate packets.
