# Work Context: WP11 — Separate Workspace Template, Context, and Policy Memory

This file separates current verified behavior from the accepted target. Pure
domain and migration-planning types may exist during implementation, but the
target is not yet an ordinary reader/writer or public contract.

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
- The dynamic learned policy is not part of that static Template. The
  owner-decision target names it `Policy Memory` and uses “remembered decisions”
  in routine presentation.
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

## Sequencing and predecessor observation

- Control authorized WP11 as a pre-public hard cutover before further WP05
  mechanism and before WP09, WP06, and WP10. The exact implementation
  predecessor is `0bbd9deb424814ab92eed0b816e2c565e4b8f6d3`, not a published
  compatibility contract.
- The independent sibling worktree uses branch
  `codex/wp11-template-context-policy-memory`. Draft packet commit
  `555736bef355ce3372dd1f944464190721292439` has exact base `0bbd9deb`.
- Final WP03 `922fa792452ce053c994f4271e6debebae1e91dc`, WP04
  `cc5d14b949276cafac0387dca3b7807d4ed34ed5`, and WP07
  `77c5607ed6867c3f5162eb0a076f5589234a8462` are ancestors of the base and
  remain accepted outcomes. WP11 changes only their affected identity,
  protection, authentication-owner, learned-policy, Catalog, and migration
  seams.
- ADR 0083 and `0bbd9deb` remain the accepted Host Loopback hostname,
  retirement, DNS/TLS, migration-lock, and attachment-lifetime authority.
  WP11 replaces its predecessor principal dimensions with ContextID and
  WorkspaceID before the paused mechanism is implemented.

## External facts

No external specification is required for this packet. Kubernetes
PodTemplate/controller and Docker Compose service-definition analogies are only
metaphors; Tobari's explicit-entry reconciliation and host-owned security state
remain authoritative.

## Owner-decision target

The packet recommends one complete V1 target rather than leaving independent
choices open:

- **Vocabulary:** public long noun `Workspace Template`, CLI noun `template`,
  durable `Context`, replaceable `Workspace`, and `Policy Memory`. Routine UI
  says “remembered decisions”; `policy` remains the command namespace.
  `Manifest` is retired from current public/domain resource vocabulary and may
  describe only a private serialized migration artifact.
- **Identity:** introduce opaque `WorkspaceTemplateID`, `ContextID`, and
  `WorkspaceID`. TemplateID plus semantic Template revision digest is static
  content authority. ContextID is the Policy Memory and Project/Template
  binding authority. WorkspaceID is the applied-instance, home, and native-auth
  authority. Names, roots, generations, images, and containers are never
  authority.
- **Uniqueness:** exactly one Context may exist for one `(canonical ProjectRoot,
  WorkspaceTemplateID)` pair. A Project may have several Contexts only by
  selecting different Templates. Context has no second human name and cannot
  rebind to another Template in V1.
- **Defaults:** preserve one installation `DefaultTemplateSelection`. Bare
  `tobari` and bare `status` own the default-Template/default-pair workflow and
  revalidate that exact selection under the action/read boundary. Explicit
  nondefault entry consumes a Context ref through `context enter --id`; no
  action accepts a Template or Context display name. There is no current/default
  Context, `context use`, active Template, or mutable Context selector.
- **Tracking:** Context always desires its Template's current immutable
  revision. It does not pin or snapshot. Workspace `AppliedEntry` records the
  exact last-successful TemplateID/digest; explicit entry is the only Workspace
  adoption writer. Reads derive desired state without mutation.
- **Boundary:** one WorkspaceTemplateID fixes one immutable source/network
  Boundary fingerprint across every revision. A Boundary change creates a fresh
  Template and therefore a fresh Context. Template revisions may change only
  baseline and typed defaults inside that fixed terminal Boundary.
- **Policy:** Policy Memory contains only confirmed remembered Allows and exact
  Denies for one Context. Pending candidates retain ContextID plus the observing
  WorkspaceID but are not authority. Template Boundary/baseline and Policy
  Memory have separate semantic revisions and activation receipts.
- **Deletion:** Workspace deletion removes that Workspace's container state,
  persistent home, native authentication, pending wait/candidate observation,
  and WorkspaceID; it preserves Context and Policy Memory. Context deletion
  requires no Workspace, attachment, or research credential, then deletes the
  Context binding, Policy Memory, and unresolved candidates while preserving
  Project files, Templates, Runtimes, and non-authorizing installation audit
  evidence. Template deletion requires no default selection and no Context.
- **Authentication:** standard auth remains Workspace-home owned. Research
  Broker credentials become Context-owned research state, never Template state;
  the four auth operations each require one unchanged Context ref from the
  Context discovery chain, and Context deletion requires exact research logout.
  Existing research state is quarantined and requires reauthentication rather
  than rebound by migration.
- **Action binding:** mutable names are read-only discovery/completion input.
  Template copy consumes one exact immutable Template revision ref; nondefault
  entry consumes a Context ref; Workspace status/delete consumes a Workspace
  ref. Name reuse can never redirect a reviewed action.
- **Migration identity:** preserve predecessor WorkspaceManifestID bytes as
  WorkspaceTemplateID and predecessor WorkspaceID bytes as WorkspaceID. Generate
  one fresh ContextID for each exact predecessor `(ProjectRoot,
  WorkspaceManifestID)` pair and journal that mapping. Rewrite learned decisions
  to Context Policy Memory only from exact validated predecessor identity.
- **Compatibility:** one pre-public transaction and one final reader. No command,
  flag, schema, state, or authority alias; no post-release fallback.

The Product Owner must accept or replace this target as a whole before
production implementation. Any replacement must restate the affected identity,
deletion, activation, reference, schema, and migration consequences rather than
changing one noun in isolation.

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

## Implementation observations

- The first pure Template revision draft stored only independently activated
  digests and Runtime identity. That could prove receipts but could not supply
  the source/network Boundary, baseline policy, session/creation defaults, or
  exact Runtime binding to routine copy, entry, and cluster operations after
  the predecessor store is retired. Any implementation would have needed a
  forbidden predecessor read or untyped parallel authority.
- The corrected revision owns one complete typed immutable body. Validation
  recomputes Boundary, policy, entry, session, creation, and overall semantic
  digests from it. Migration carries and transforms exact predecessor bodies;
  focused clone/mutation canaries and a normal copy/entry derivation fixture
  prove final operations do not depend on predecessor bytes or inferred values.
- An intermediate complete-body draft represented Advanced policy as an
  arbitrary sorted relative `.rego` collection. That widened the existing
  executable-source boundary and could dead-end at the exact-file aggregate
  reader. The corrected body has a named, bounded `tobari.rego` plus
  `tobari_test.rego` pair. Its filesystem conversion rejects missing, renamed,
  duplicate, and extra sources; migration accepts only the same exact pair.
- The first pure migration draft retained an exact predecessor AppliedEntry
  when its stored Template and Runtime fields matched, but it had no bounded
  owned-Docker evidence. A normal stopped-cluster migration after Docker cleanup
  could therefore have claimed applied authority that was missing, mismatched,
  or unknown at cutover.
- The corrected pure input carries one closed observation per Workspace, bound
  to exact WorkspaceID. Only `exact_owned` evidence agreeing with Template
  generation/digest, RuntimeID/revision, and resolved spec can retain the
  AppliedEntry. `missing`, `mismatched`, and `unknown` publish no AppliedEntry
  and become explicit unverified state. Focused fixtures cover all four states.
- The new Workspace Template, Context, Policy Memory, Workspace binding,
  independent activation-receipt, opaque-reference, and migration-plan types
  remain dormant domain code. No current Manifest reader, writer, Catalog path,
  schema, application port, infrastructure adapter, or public output consumes
  them in this concern.

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
  and cancellation facts: Template/Context reads use complete delivery and
  exhaustive or not-applicable coverage with no pagination. Mutations use one
  bounded attempt through the existing invoker contract; creates are exact
  receipt-idempotent only, deletes require literal confirmation, cancellation
  before action proves zero change, and every unknown/confirmed-late-output
  outcome recovers through a read-only list/show/status command.
- Publication and licensing concerns: no external content or dependency.

## Glossary

- **Workspace Template:** recommended public stable reusable static Workspace design. It
  has independently versioned immutable revisions.
- **Template Revision:** one canonical immutable semantic body under a Workspace
  Template. `Manifest` is not its current public/domain resource name.
- **Context:** recommended durable Project × Workspace Template binding. It owns
  project-specific retained policy learning and outlives a replaceable Workspace.
- **Policy Memory:** recommended domain name for reviewed learned Allows and exact
  Denies retained for one Context. It is constrained by Template Boundary and
  is not a credential or an audit-log synonym.
- **Workspace:** replaceable applied/observed isolated instance for one Context.
  It records the last successful Template application and runtime facts.
- **Desired Template revision:** the Template's current immutable revision that
  the Context intends to use on next explicit Workspace entry.
- **Applied Template revision:** the exact Template semantic digest last
  successfully reconciled into the Workspace.
