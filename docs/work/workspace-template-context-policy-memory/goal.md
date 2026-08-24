# Work Goal: WP11 — Separate Workspace Template, Context, and Policy Memory

- Status: Draft
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, and `docs/03_security_model.md`
- Review/delete trigger: Delete after the owner schedules the work, durable conclusions are promoted, implementation gates pass, and the change completes
- Successor: None
- Owner: Tobari product owner and maintainers
- Target: Owner sequencing decision after WP01+02 and against the remaining V1 work packets
- Related ADRs: ADR 0079 (likely revision or successor required), ADR 0070

## Outcome

Tobari gives the reusable static Workspace design, the project-specific durable
working context, its learned policy, and the replaceable applied Workspace
separate names and lifecycles. The target under review is a versioned
**Workspace Template**, a durable **Context** that binds one Project to one
Template and owns mutable **Policy Memory**, and a replaceable **Workspace**
that records what was last applied and what is currently observed. Explicit
Workspace entry remains the mutation-bearing reconciliation boundary; there is
no resident controller.

## Why now

WP01+02 correctly introduced desired/applied/observed state and immutable
Workspace Manifest revisions, but implementation and product discussion exposed
a remaining ownership mismatch: a reusable static definition and policy learned
from one Project's runtime activity do not have the same scope, lifetime, or
mutability. Calling both one Manifest makes the resource look static while it
also grows dynamically. The product owner also expects deleting and recreating
a Workspace to preserve that learned state. This should be decided before
downstream V1 work cements Manifest ownership into status, permission handoff,
first-use UX, and public reference flows.

## Non-goals

- Do not implement, rename commands, migrate state, or change public schemas in
  this packet-creation handoff.
- Do not weaken the WP01 desired/applied/observed split or introduce a resident
  Kubernetes-style controller.
- Do not decide the detailed Runtime lifecycle, Host Loopback name, build
  profile, status presentation, service exposure, or first-use recovery owned by
  their respective packets.
- Do not make `Manifest` a user-authored arbitrary YAML/JSON or `apply -f`
  boundary without a separate trust-boundary decision.
- Do not require Template revisions and Policy Memory revisions to share one
  generation or transaction merely because a Context presents them together.
- Do not finalize the provisional name `Policy Memory` before vocabulary review.

## Acceptance criteria

- [ ] The owner approves or rejects the four-concept model: Workspace Template,
      immutable Template Manifest/revision, Context, and Workspace.
- [ ] Static Template fields and dynamic learned policy are assigned by explicit
      owner, scope, lifetime, mutability, and authority rather than by naming
      convenience.
- [ ] A Context can outlive Workspace deletion, and its Policy Memory is deleted
      only by an explicit Context-lifecycle decision.
- [ ] Multiple Contexts for one Project can bind different Templates without
      sharing learned policy implicitly.
- [ ] Template copy and Context creation are distinct user outcomes: static fork
      versus choosing a Template for a new project-specific Context.
- [ ] Template advancement, Context desired state, Workspace last-successful
      applied state, pending adoption, failure, retry, and read-only observation
      have an exact no-controller reconciliation contract.
- [ ] Public CLI roles, opaque reference producers/consumers, human output,
      JSON/schema versions, default selection, compatibility, and migration are
      designed before implementation.
- [ ] Policy Boundary/baseline and Policy Memory remain separate authority tiers;
      learned policy cannot widen the Template's terminal ceiling.
- [ ] Migration preserves authoritative IDs and learned rules without inferring
      ownership from names, generations, roots, images, or containers; rollback
      and mixed-version behavior fail closed.
- [ ] ADR/thesis/product/architecture/security/harness conclusions are promoted,
      and `task check`, `task security`, `task public:check`, and relevant runtime
      integration gates pass before the temporary packet is removed.

## Governing documents

- Thesis: Thesis 0, Thesis 4, Thesis 8, and especially Thesis 9
- Product contract section: Public vocabulary; public command surface; input and
  path contract; Workspace Manifest definition, selection, deletion, policy,
  output, and migration contracts
- Architecture or security invariant: four-layer dependency direction; explicit
  reconciliation boundaries; stable identity and digest authority; learned
  policy isolation; atomic all-definition policy activation
- Existing ADR: ADR 0079, which fixes the current Workspace Manifest model and
  must be deliberately revised or superseded rather than silently renamed

## Completion definition

The work is complete only after the owner has scheduled and approved the
design, acceptance criteria have evidence, durable decisions have been promoted
to numbered documentation or an ADR, required profiles pass, temporary
diagnostics are removed, and this temporary packet is removed from the final
tree.
