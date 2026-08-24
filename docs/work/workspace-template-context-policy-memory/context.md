# Work Context: WP11 — Separate Workspace Template, Context, and Policy Memory

This file separates current verified behavior from the proposed design. The
proposal is not implemented and is not yet a public contract.

## Current behavior

- ADR 0079 and Thesis 9 define `Workspace Manifest` as the stable named desired
  Workspace definition. Each semantic mutation publishes a complete immutable
  revision; `(WorkspaceManifestID, semantic digest)` is authority and generation
  is correlation only.
- A current Workspace permanently binds `(canonical Project root,
  WorkspaceManifestID)`, stores last-successful `AppliedEntry`, and keeps desired,
  applied, failed/unknown reconciliation, and observed runtime facts separate.
- Only explicit Workspace entry and `cluster up` reconcile. Status, list, show,
  doctor, and completion are read-only observations.
- `manifest create --copy-from NAME --name NAME` reviews and revalidates an exact
  immutable current Manifest revision, then creates a fresh independent
  WorkspaceManifestID at generation 1. It copies no Workspace, authentication,
  learned policy, AppliedEntry, failure, observation, attachment, or default
  selection state.
- `manifest default set` and omission of `--manifest` choose the default desired
  definition for later invocations; they do not retarget existing Workspaces.
- The current Manifest owns a static source/network Boundary, policy snapshot
  and ceilings, Runtime binding, session defaults, and creation defaults, even
  though those fields activate at typed boundaries.
- Learned Allows and exact Denies are currently retained by Manifest/project
  policy identity and activated through the shared cluster policy projection.
  They are mutable during the lifetime of running or recreated Workspace work,
  unlike an immutable Manifest revision body.
- Current public deletion protects a Manifest referenced by a logical Workspace.
  Ordinary Workspace deletion ends the root-bound instance; the current model
  has no separate durable Context whose policy memory survives independently.

Verified sources: `docs/00_theses.md` Thesis 9,
`docs/01_product_contract.md` public vocabulary and command table,
`docs/02_architecture.md`, `docs/03_security_model.md`, `docs/04_harness.md`,
`docs/decisions/0079-model-workspace-manifests-and-applied-workspaces.md`,
`internal/domain/tobari/context.go`, `internal/domain/tobari/project.go`, and
`internal/domain/tobari/policy.go`.

## Product-owner direction recorded for WP11

These are accepted inputs to the next design review, not current implementation:

- The static reusable concept is class-like: a design from which project work is
  instantiated. `Workspace Template` currently fits that meaning better than a
  primary public resource named `Manifest`.
- The dynamic learned policy is not part of that static Template. Its provisional
  name is `Policy Memory` until vocabulary review.
- A durable `Context` is the leading aggregate that binds one Project to one
  Workspace Template and owns the corresponding Policy Memory.
- The owner selected the follow-current model: a Context desires the Template's
  current immutable revision, while its Workspace records the last successfully
  applied revision. Explicit entry adopts a newer desired revision.
- Deleting a Workspace preserves its Context and Policy Memory. This is a firm
  lifecycle requirement, not an optional alternative.
- One Project may have multiple Contexts bound to different Templates. Their
  Policy Memories are isolated unless a future explicit operation says
  otherwise.
- The current copy operation splits into two meanings: copying static Template
  content to create a new independent Template, and creating a new Context that
  selects an existing Template. The latter is not a copy.

## Relevant structure

- Entry point: root `tobari [--manifest NAME]`, `manifest` namespace, `cluster
  up`, Workspace entry/delete, and policy review/reset commands.
- Domain rule: `WorkspaceManifest`, `WorkspaceManifestRevision`, `Workspace`,
  `AppliedEntry`, `LearnedPolicyRule`, policy candidates, and exact deny rules.
- Application use case: Manifest creation/copy/default selection, root entry,
  policy review/apply/reset, Workspace deletion, and cluster reconciliation.
- Infrastructure boundary: owner-only Manifest/revision/policy stores, Workspace
  state/home, Docker runtime observation, principal registry, atomic aggregate
  OPA projection, and migration journal.
- CLI catalog or presentation: `cli.Catalog` is the sole public command and
  reference-flow authority; existing `manifest` and `--manifest` vocabulary is a
  pre-public hard contract from WP01.
- Existing tests and harness checks: Manifest immutable revision/copy tests,
  desired/applied/observed status fixtures, Workspace lifecycle tests, policy
  isolation and atomic activation tests, migration tests, public vocabulary
  guards, agent-readiness transcripts, and Docker integration.

## Constraints

- ADR 0079 is Accepted. WP11 must revise or supersede it and propagate the result;
  a local noun replacement is not sufficient.
- Owner/scope/lifetime/mutability/authority must drive resource boundaries.
  Presentation convenience alone must not split or combine aggregates.
- `WorkspaceManifestID + semantic digest` is the current content authority.
  Generations, names, roots, image tags, and observed containers are not
  authority and cannot silently become migration keys.
- There is no resident controller. Mutation-bearing reconciliation remains
  explicit Workspace entry and `cluster up`; read operations never repair.
- Template advancement and Policy Memory mutation have independent revision and
  activation axes. A Context may present both, but must not collapse them into
  one misleading revision.
- The Template's terminal destination/method Boundary remains above learned
  positive policy. Policy Memory cannot grant beyond the Boundary or exact
  terminal Deny.
- Project files and Workspace/container observations are untrusted. Template,
  Context, Policy Memory, AppliedEntry, and principal records are trusted
  host-owned state and require exact owner, schema, digest, and atomicity checks.
- Standard authentication remains Workspace-home owned under the current thesis.
  WP11 must not accidentally move credentials into Context or Template merely
  because Policy Memory moves.
- A public `Template` does not imply arbitrary repository-owned configuration,
  file import, inheritance, or `apply -f`.
- Pre-public V1 prefers a coherent hard cutover over aliases, but the owner must
  schedule that cutover against downstream packets and any first-release freeze.

## External facts

No external specification is required for this packet. Kubernetes
PodTemplate/controller and Docker Compose service-definition analogies are only
metaphors; Tobari's explicit-entry reconciliation and host-owned security state
remain authoritative.

## Unknowns

- [ ] Decide the final public nouns: `Workspace Template` versus `Template`, and
      whether `Manifest` remains only the immutable serialized revision/body.
- [ ] Decide the final dynamic noun: `Policy Memory`, `Learned Policy`, `Context
      Policy`, or another term that communicates reviewed retained decisions.
- [ ] Decide whether Context has an independent opaque ContextID or whether a
      typed `(ProjectID, WorkspaceTemplateID)` pair is sufficient authority.
- [ ] Decide whether a Context follows Template current forever, can pin a
      revision, or may expose an explicit upgrade policy later. The currently
      selected V1 direction is follow-current with entry-time adoption.
- [ ] Decide exact default selection semantics: default Template for new Context
      creation, default Context for a Project, invocation-local Context, or a
      deliberately smaller combination.
- [ ] Decide Template and Context deletion protection, including Context delete
      confirmation and the exact disposition of Policy Memory, pending review
      items, audit evidence, and Workspace home.
- [ ] Decide whether one Project may have multiple Contexts using the same
      Template and, if so, what human name or opaque reference disambiguates
      them.
- [ ] Decide how static Boundary/baseline changes interact with existing Policy
      Memory whose entries are no longer admissible, without silently granting
      or deleting reviewed history.
- [ ] Decide the two independent activation receipts: Template applied revision
      at Workspace entry and Policy Memory active revision at shared-cluster
      policy activation.
- [ ] Design migration from WorkspaceManifestID and Manifest/project learned
      stores to TemplateID, Context identity, and Policy Memory without inferring
      association from names or current Docker state.
- [ ] Decide public command paths, structured schema versions, reference kinds,
      recovery commands, and alias policy only after the aggregate decision.
- [ ] Have the control owner place WP11 before, between, or after downstream
      WP03–10 work and identify which accepted packets must be rebased.

## Thesis evidence

- Repeated design decision or point of agent confusion: `Work Mode`, `Type`, and
  `Manifest` each tried to name a class-like definition, while the same object
  also owned policy that grows from runtime experience. The mismatch persisted
  after the WP01 rename and indicates an ownership issue rather than a vocabulary
  issue alone.
- User outcome or friction observed in the minimal slice: users want to choose a
  reusable setup, keep project-specific learned policy across Workspace
  recreation, and understand what is static versus what changes while working.
- Code workaround or exception being considered: keeping learned rules in the
  Manifest-owned policy tree would preserve current implementation but encode
  Project-specific mutable state under a reusable static definition.
- Current thesis that resolves it, or proposed thesis revision: Thesis 9 should
  be revised from “Every Workspace applies one Workspace Manifest” to a model in
  which a Context binds one Project to one Template and a Workspace applies that
  Context's desired Template revision plus separately activated Policy Memory.
- Downstream impact: product vocabulary, ADR 0079, identity/reference graph,
  Catalog, state/migration, security principals, policy projection, status,
  permission review/resume, first-use, site/help, and agent-readiness fixtures.

## Reproduction or observation

No effectful runtime reproduction was needed. This packet is based on current
contract and source inspection plus the product-owner lifecycle decisions above.

## Security and public-boundary notes

- Assets and side effects involved: static Template revisions, mutable learned
  policy, Context bindings, Workspace state/home, principal registry, OPA
  projection, migration backup/journal, and deletion recovery.
- Credentials or confidential data involved: none in this packet. Standard
  Workspace-native authentication and research Broker material must retain their
  separately governed ownership during any future migration.
- New dependencies, destinations, files, processes, or generated content: none
  proposed. No resident controller is introduced.
- External schema provenance, publication rights, and drift evidence: not
  applicable; all proposed state is Tobari-owned.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency,
  and cancellation facts: unresolved for future commands. Reads must remain
  read-only; create/copy/delete/reconcile mutations must declare exact target,
  impact, idempotency, bounded outcome, and recovery.
- Publication and licensing concerns: no external content or dependency.

## Glossary

- **Workspace Template:** proposed stable reusable static Workspace design. It
  has independently versioned immutable revisions.
- **Template Manifest:** provisional secondary term for the canonical immutable
  body/representation of one Template revision, not necessarily a primary
  user-managed resource.
- **Context:** proposed durable Project × Workspace Template binding. It owns
  project-specific retained policy learning and outlives a replaceable Workspace.
- **Policy Memory:** provisional name for reviewed learned Allows and exact
  Denies retained for one Context. It is constrained by Template Boundary and
  is not a credential or an audit-log synonym.
- **Workspace:** replaceable applied/observed isolated instance for one Context.
  It records the last successful Template application and runtime facts.
- **Desired Template revision:** the Template's current immutable revision that
  the Context intends to use on next explicit Workspace entry.
- **Applied Template revision:** the exact Template semantic digest last
  successfully reconciled into the Workspace.
