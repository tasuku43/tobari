# Work Goal: WP11 — Separate Workspace Template, Context, and Policy Memory

- Status: Accepted
- Planning state: Accepted for implementation
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, and `docs/03_security_model.md`
- Review/delete trigger: Delete after the owner schedules the work, durable conclusions are promoted, implementation gates pass, and the change completes
- Successor: None
- Owner: Tobari product owner and maintainers
- Target: Pre-release final-authority-only hard cutover with explicit
  reset/recreation of incompatible development state, before further WP05
  mechanism and before WP09/WP06/WP10
- Related ADRs: ADR 0084 (accepted final authority), superseded ADR 0079 and ADR
  0070, ADR 0080, ADR 0081, ADR 0082, and ADR 0083

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
- Do not make a Workspace Template a user-authored arbitrary YAML/JSON or `apply -f`
  boundary without a separate trust-boundary decision.
- Do not require Template revisions and Policy Memory revisions to share one
  generation or transaction merely because a Context presents them together.
- Do not change the accepted WP03 Runtime lifecycle, WP04 release/research
  surfaces, WP07 permission-wait capability, or ADR 0083 Host Loopback hostname
  and retirement authority while rebinding their affected identity seams.

## Acceptance criteria

- [ ] The owner approves or rejects the four-concept model: Workspace Template,
      immutable Template Revision, Context with Policy Memory, and Workspace.
- [ ] Static Template fields and dynamic learned policy are assigned by explicit
      owner, scope, lifetime, mutability, and authority rather than by naming
      convenience.
- [ ] A Context can outlive Workspace deletion, and its Policy Memory is deleted
      only by an explicit Context-lifecycle decision.
- [ ] Multiple Contexts for one Project can bind different Templates without
      sharing learned policy implicitly.
- [ ] Template copy and Context creation are distinct user outcomes: static fork
      versus choosing a Template for a new project-specific Context.
- [ ] Every explicit action consumes one unchanged opaque reference or owns one
      complete command-local default target; mutable Template names are
      read-only discovery/completion input and cannot redirect entry, copy, or
      deletion after name reuse.
- [ ] One TemplateID fixes one immutable source/network Boundary fingerprint;
      Boundary change creates a fresh Template and fresh Context, so remembered
      authority cannot reactivate through same-Template widening.
- [ ] Template advancement, Context desired state, Workspace last-successful
      applied state, pending adoption, failure, retry, and read-only observation
      have an exact no-controller reconciliation contract.
- [ ] Public CLI roles, opaque reference producers/consumers, human output,
      JSON/schema versions, default selection, and pre-release clean-break
      compatibility are designed before implementation.
- [ ] Policy Boundary/baseline and Policy Memory remain separate authority tiers;
      learned policy cannot widen the Template's terminal ceiling.
- [ ] Research keeps WP04's exact five-path delta; login/import/status/logout
      consume one Context ref, release exposes none, and Context deletion has an
      exact logout-first supported workflow.
- [ ] A genuinely fresh installation is exact empty final authority; declared
      legacy presence fails closed with zero mutation and explicit
      reset-and-recreate guidance. No predecessor identity, learned rule,
      credential, principal, or Runtime-protection fact is migrated or adopted.
- [ ] ADR/thesis/product/architecture/security/harness conclusions are promoted,
      and `task check`, `task security`, `task public:check`, and relevant runtime
      integration gates pass before the temporary packet is removed.

## Governing documents

- Thesis: Thesis 0, Thesis 4, Thesis 8, and especially Thesis 9
- Product contract section: Public vocabulary; public command surface; input and
  path contract; Workspace Manifest definition, selection, deletion, policy,
  output, and pre-release clean-break contracts
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
