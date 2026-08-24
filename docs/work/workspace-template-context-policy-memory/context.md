# Work Context: WP11 — Separate Workspace Template, Context, and Policy Memory

This file separates current verified behavior from the accepted target. Dormant
final domain and adapter types may exist during implementation, but the target
is not yet an ordinary reader/writer or public contract until the atomic
cutover.

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
  OPA projection, final owner store, and bounded legacy-presence guard.
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
  authority and cannot silently become final authority or compatibility keys.
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

## Sequencing and pre-release clean break

- Control authorized WP11 as a pre-public hard cutover before further WP05
  mechanism and before WP09, WP06, and WP10. Because Tobari has no public
  release, existing development Manifest/Workspace/Policy/Broker state is not a
  compatibility or migration target. Users explicitly reset and recreate it.
- Ordinary composition selects only the final owner store. Exact final absence
  plus bounded absence of declared legacy paths is a genuinely fresh empty
  installation; legacy, unsafe, partial, or changing presence fails closed with
  zero mutation and explicit reset-and-recreate guidance.
- This decision neither deletes nor transforms predecessor data and cannot be
  generalized after the first public release. Future released-state
  compatibility remains undecided and requires an explicit release-policy
  decision.
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
  retirement, DNS/TLS, final lock order, and attachment-lifetime authority.
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
  describe only historical unsupported development state.
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
  Existing research state is neither read nor adopted; explicit final Context
  login/import creates fresh authority after the user resets legacy presence.
- **Action binding:** mutable names are read-only discovery/completion input.
  Template copy consumes one exact immutable Template revision ref; nondefault
  entry consumes a Context ref; Workspace status/delete consumes a Workspace
  ref. Name reuse can never redirect a reviewed action.
- **Clean-break identity:** final WorkspaceTemplateID, ContextID, and WorkspaceID
  are created only by final tasks. No predecessor ID, learned decision,
  candidate, Runtime edge, principal, session, home, or credential is converted
  or adopted.
- **Compatibility:** one pre-public final reader and no migration transaction.
  No command, flag, schema, state, or authority alias; bounded legacy presence
  blocks final initialization until explicit reset. This is not a post-release
  precedent.

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
  exact Runtime binding to routine copy, entry, and cluster operations using
  only the final store. Any implementation would otherwise have needed a
  forbidden predecessor read or untyped parallel authority.
- The corrected revision owns one complete typed immutable body. Validation
  recomputes Boundary, policy, entry, session, creation, and overall semantic
  digests from it. Focused clone/mutation canaries and a normal copy/entry
  derivation fixture prove final operations do not depend on predecessor bytes
  or inferred values.
- An intermediate complete-body draft represented Advanced policy as an
  arbitrary sorted relative `.rego` collection. That widened the existing
  executable-source boundary and could dead-end at the exact-file aggregate
  reader. The corrected body has a named, bounded `tobari.rego` plus
  `tobari_test.rego` pair. Its filesystem conversion rejects missing, renamed,
  duplicate, and extra sources.
- Earlier dormant pure migration drafts explored predecessor AppliedEntry and
  exact-owned Docker evidence. The product-owner clean-break decision supersedes
  that delivery scope; no final task consumes those drafts or predecessor
  observations.
- The new Workspace Template, Context, Policy Memory, Workspace binding,
  independent activation-receipt and opaque-reference types remain dormant
  domain code. No current Manifest reader, writer, Catalog path,
  schema, application port, infrastructure adapter, or public output consumes
  them in this concern.
- Dormant final application tasks now consume and re-emit exact typed refs,
  validate coherent Context/Template/Policy Memory/Workspace snapshots, and
  cross the existing mutation invoker once. Port exchange values are
  domain-owned so infrastructure can satisfy the task-owned interfaces without
  importing application code. No Catalog route or current reader is wired.
- The first application result contract for direct policy decisions checked
  only the returned candidate ID, resulting rule ID, and complete current
  Policy Memory. A normal adapter mix-up could therefore report Allow while
  returning a Deny or unrelated rule, accept a no-op, or change unrelated
  authority. The corrected candidate authority contains ContextID, observing
  WorkspaceID, and one exact typed effect whose digest derives the opaque
  candidate ID. Allow/Deny validates the expected decision and reconstructs
  `previous + exact result rule`; Reset reconstructs `previous - exact target
  rule`. Both direct paths require `Changed=true` and exact generation/revision
  equality. Apply-reviewed requires one nonempty explicitly confirmed set and
  always publishes its reviewed authority change. Only same-set terminal replay
  performs zero repeated external effect, while returning the original
  confirmed `Changed=true` result.
- Historical dormant migration evidence also carried that complete typed
  effect and validated its predecessor payload digest. The clean-break target
  does not consume that path; ordinary final candidates originate only from
  final Context/Workspace observations.
- The first exhaustive application collections validated each snapshot and
  duplicate primary IDs but not aggregate binding uniqueness. The corrected
  Context collection reuses `ValidateContextBindings` and rejects duplicate
  optional Workspace IDs. The Workspace collection additionally rejects more
  than one Workspace per Context and inconsistent Project/Template pairs. A
  shared domain collection validator also makes repeated TemplateIDs require
  byte/semantic exact Template equality and rejects one Template name assigned
  to different IDs, so mixed owner-store reads cannot emit contradictory refs.
- Final authority observation uses one normalized, atomically replaceable
  envelope rather than independent Template, Context, Workspace, Policy Memory,
  activation, candidate, and default files. One installation generation and
  aggregate digest fence the complete joined observation; that aggregate
  receipt is coherence evidence, not a new domain authority identity.
- The read-only store accepts exactly one owner-only real `authority.json` in
  one owner-only real root, opens and validates that immutable file once, and
  bounds bytes plus every collection count. Unknown, duplicate, trailing,
  partial, mixed, unsafe-mode, symlinked, or domain-incoherent authority fails
  closed. A missing final root produces explicit empty lists or typed not-found
  without creating the root or a lock.
- Pending candidate state is exhaustive within the envelope. A candidate that
  already appears in its Context's current Policy Memory source-candidate set
  is consumed and cannot remain pending; historical source IDs remain valid
  without corresponding pending records.
- This store remains dormant infrastructure. It structurally satisfies only the
  accepted read ports and is not connected to the predecessor Manifest reader,
  current composition root, Catalog, migration writer, policy activation, or
  public output in this concern.
- The dormant final-authority mutation adapter serializes through the existing
  installation lifecycle authority and publishes only one complete normalized
  envelope. Pure semantic no-ops preserve generation. Workspace retirement
  and Policy Memory activation durably bind exact previous/next authority,
  operation, target, and request dimensions before the external effect. An
  active decision excludes unrelated mutation; same-ref recovery re-observes
  the exact retirement or activation receipt. One bounded latest terminal
  effect receipt survives result-delivery interruption and remains replayable
  across unrelated pure envelope mutations until the next confirmed effectful
  outcome replaces it.

## Dormant Context-entry reconciliation evidence

- The dormant entry adapter consumes one unchanged Context ref and derives the
  complete desired entry only from that Context's current final Template
  revision. Its runtime plan binds ContextID, WorkspaceID, TemplateID/revision,
  entry slice, RuntimeID/revision, resolved spec, reconciliation time, and the
  WorkspaceID-derived canonical home. Complete-envelope validation rejects one
  home shared by distinct WorkspaceIDs because standard authentication is
  Workspace-home-owned.
- Entry observes the two activation axes independently. The current exact
  Template-policy receipt and current exact Policy-Memory receipt must both be
  present and externally confirmed before runtime mutation; entry does not
  repair either axis. Missing or stale receipts are typed precondition/none,
  while uncertain pre-decision observation is observation/not-applicable.
- Runtime reconciliation uses the existing lifecycle authority and the same
  active/terminal mutation decision protocol as Workspace retirement and
  Policy activation. The decision is durable before runtime effects. A failed,
  canceled, or interrupted attempt preserves the prior last-successful
  AppliedEntry; same-Context recovery reuses the exact plan and decision ref.
  A no-op envelope transition is never mistaken for proof that the external
  effect ran and therefore still invokes receipt-idempotent reconciliation.
- AppliedEntry is published only after a bare 64-lowercase-hex Docker container
  identity confirms the exact runtime/spec plan and both activation axes are
  re-observed with one finite post-effect settlement deadline. Timeout leaves
  the active decision and stage resumable and releases the lifecycle lock.
  Once the envelope is read back, later cancellation or session-start/run
  failure is typed attachment/confirmed rather than unknown or none.
- The dormant coordinator invokes a task-owned `WorkspaceSessionAuthority`
  port while the lifecycle lock still excludes Workspace deletion, and runs
  the returned child/cleanup owner only after that lock is released. Focused
  fakes prove this ordering.
- The dormant host-only `workspaceauthoritysession` bridge now maps the
  complete final `WorkspaceSessionBinding` to WP07's one canonical interactive
  attachment owner. `dockerruntime` exports only that narrow owner and does not
  import the final store, preserving the exposure-helper closure. Legacy and
  final entry wrap the same private principal path; no second attachment
  registry, epoch, nonce, heartbeat, wait registry, or permission transport is
  created.
- The final principal places exact ContextID, WorkspaceID, Template-derived
  presentation, and canonical ProjectRoot into the frozen private
  `context_id`/`project_id`/`context` projection. TemplateID, Template revision,
  Runtime/spec, session defaults, and exact 64-hex container evidence are
  validated by the complete binding and Docker observation but do not become
  wire tokens or session selectors. Immediately before child/route/channel
  effects, Run re-observes the exact principal fingerprint and same canonical
  owner on both sides of a nonce liveness handshake.
- Cross-process zero-owner observation uses a smaller persistent final session
  identity derived from one coherent Context snapshot. Missing session store
  or a valid registry without the exact ContextID+WorkspaceID is confirmed
  absent without requiring a live principal or container; malformed/unsafe
  state, stale records, failed liveness, or concurrent replacement remain
  ambiguous. No Gateway/Broker composition, resident controller, public
  recovery command, current reader, or Catalog route is changed.

## Dormant final policy projection evidence

- One pure final projection plan binds operation mode, exact input collection
  revision, hot target Context when applicable, and the complete selected
  Context content. Its separate content identity excludes those request
  preconditions, so identical live policy/principal content is confirmable
  after an unrelated default or unused-Template envelope mutation.
- Hot Policy Memory activation selects the target Context's new current memory
  while preserving every Context's active Template policy slice and every
  other Context's active memory. Cluster candidate construction separately
  selects current Template policy plus current Policy Memory for every Context;
  no combined Context revision or ambient `Template.Current` inference exists.
- Materialization binds exact Context/Template receipts, current WorkspaceID,
  AppliedEntry/spec, retained creation-defaults authority, healthy Workspace
  container identity, healthy pinned Gateway component identity, and the exact
  control/egress/project network set and endpoint addresses. Remembered rules
  are executable only for the current projected Workspace; Context-only memory
  remains durable but produces no principal-bound rows.
- One private publication receipt joins the route-independent materialized
  digest, exact aggregate revision, OPA artifact tree, Gateway artifact bytes,
  all per-Context Template/Memory receipts, and principal evidence. A live hot
  activation additionally proves that the running Gateway's exact owner-only
  aggregate mount has the same bytes; a previous immutable aggregate path is
  valid when bytes match, while changed interpretation requires cluster
  reconciliation.
- The dormant hot adapter holds the existing policy-projection lock under the
  caller's installation lifecycle authority. It fsyncs the activation-root
  parent before the first journal, writes one bounded recovery record before
  OPA effects, re-observes Docker after artifact build, publishes OPA, then
  atomically records the active receipt. Process interruption resumes the exact
  journal; confirmation performs no mutation and compares live content rather
  than the originating collection revision.
- One dormant lifecycle-owned final settlement coordinator now joins first
  entry, existing Workspace AppliedEntry/retained-creation changes, Workspace
  retirement/re-entry, Context deletion, direct Policy Allow/Deny/Reset, and
  cluster current/current candidates. It durably fixes OPA-only versus full
  settlement before effects. Full settlement keeps deny-all active while it
  proves global zero live owners, publishes the exact candidate principal
  registry, recreates and boundedly waits for the selected healthy Gateway+OPA
  closure, activates candidate OPA, publishes the global/per-axis receipt, and
  then permits the parent envelope publication. Principal CAS precedes the
  component replacement so an interruption never exposes candidate OPA with a
  stale final principal registry.
- Exact selected Gateway/OPA image IDs, permission profile, managed environment,
  mount closure, Compose assets, topology, aggregate artifacts, and previous/
  next collection revisions are journal authority. Recovery replays the
  journaled environment rather than ambient process values. An interrupted
  cluster journal and a final-settlement journal exclude each other; expired
  canonical attachment rows are compacted only under the existing registry
  lock and current/live or ambiguous owners block both global fences.
- A Context with neither active receipt is intentionally inactive and omitted
  from live content while its collection revision remains a plan precondition;
  exactly one active axis is invalid. Therefore an active Context can receive a
  hot memory update beside a new inactive Context, and deleting the active
  Context yields an exact empty active projection without adopting its sibling.
  Context deletion uses the same durable parent decision, so OPA/Gateway/global
  receipts cannot retain a deleted Context after the envelope changes.
- Learned GraphQL Policy Memory remains a normal Gateway-changing input. Direct
  Allow/Deny/Reset now choose the full settlement when `gateway.json` changes;
  byte-identical HTTP/MCP changes may use the OPA-only path. The still-open
  fixed `policy apply-reviewed` application path now passes one complete,
  immutable multi-Context reviewed set into one dormant store/coordinator
  decision rather than settling Contexts sequentially or adopting
  Template.Current. ReviewItemID strict order is canonical: reversed UI
  enumeration of the same items produces the same set digest and same-action
  recovery identity, while ProposalDigest and the private reviewed-set digest
  still distinguish genuinely different complete evidence. Its domain
  transition validator proves that the supplied
  next collection is exactly the result of that set before a journal, Compose,
  principal, OPA, or envelope effect. The private active receipt also binds the
  exact reviewed-set digest, so confirmation cannot relabel another valid live
  aggregate as the requested set. Direct wrong-next fixtures preserve the
  journal, principal registry, active receipt, Compose, and OPA counters
  byte-for-byte/unchanged. No public/current reader selects these dormant seams.

## Superseded dormant migration-engine evidence

- The following records historical implementation evidence from the earlier
  predecessor-preservation scope. The pre-release clean-break decision removes
  it from WP11 acceptance: no current or final `migrate apply`, Catalog route,
  composition root, or ordinary reader may select this engine. Dormant accepted
  code may remain unreachable; dirty migration-only composition WIP is removed.
- The dormant engine consumes the earlier pure migration input/plan and
  publishes exactly one final `WorkspaceAuthorityCollection` envelope.
- One owner-only journal binds exact predecessor source digests, complete final
  collection, preserved Template/Workspace bytes, fresh stable Context
  assignments, standard-home evidence, research disposition, and the exact
  same-parent backup path for every source. Config and state roots may be on
  different filesystems; the engine never renames authority between them.
- Reader selection is monotonic. Forward recovery publishes and reads back the
  final candidate while the complete predecessor remains selected, moves the
  exact cutoff to select complete final authority, then quarantines subordinate
  sources. Rollback restores subordinate sources while final remains selected,
  restores the cutoff last to select the complete predecessor, and only then
  retires final authority. Crash fixtures cover both sides of each cutoff and
  post-rename/pre-journal recovery.
- The dormant stopped-cluster and zero-live-attachment observation is re-established
  under the exclusive engine lock at every resumed mutation and immediately
  before either reader cutoff. If quiescence changes, no subsequent authority
  path is moved. This is historical evidence, not a final cutover requirement.
- Final publication reserves a same-parent stage only after proving it absent
  before the journal. Once `prepared` is durable, the journal owns only the
  exact bounded stage layout; empty or partial owner-only stage writes can be
  reconciled after process death, while unknown entries, unsafe modes,
  symlinks, and a different complete collection fail closed. Rollback removes
  only that noncanonical journal-owned stage and can resume interrupted cleanup.
- The final store and migration publisher share one executable 64 MiB bounded
  encoder. It streams the typed collection into a capped buffer, matches the
  ordinary JSON representation byte-for-byte, and rejects an over-bound final
  envelope before the first journal write, stage creation, or cutoff move. The
  journal independently checks its 96 MiB serialized ceiling before rename.
- Process exclusion uses a safe owner-only lock file plus a kernel-released
  advisory lock. A pathname left by SIGKILL is reusable, while a live holder is
  excluded. Journal and source rename errors are classified by exact read-back
  before resume decisions.
- Every Apply/Rollback validates the transaction root and fsyncs its
  already-validated parent directory before acquiring the root-local lock or
  reaching any journal, final publication, or predecessor move. Repeating the
  parent fsync changes no existing-root contents and closes the first-run race
  where a second process could otherwise observe an uncommitted directory
  entry and advance effects before its durability was established.
- Standard Workspace homes are declared outside every mutation/backup path and
  are neither read nor transformed. Linux filesystem root-key material is one
  required exact research source; macOS Keychain recovery material has no
  engine port and is untouched. Fresh canonical research-auth state blocks
  rollback rather than being merged or overwritten.
- The dormant engine's committed second apply is read-only and returns `changed:false` with the
  same Context assignments. Rollback is idempotent but terminal; implicit
  reapply after rollback is rejected so an older rolled-final receipt cannot
  collide with a later transaction. Starting again requires a separately
  reviewed new transaction rather than mutating the existing journal.

## Dormant final-Context research authentication evidence

- The task-owned application seam accepts only one unchanged final Context ref.
  Login/import bind that ref as the credential-create parent; logout targets the
  Context credential authority; status is a zero-mutation exhaustive read that
  re-emits only `context_ref`. No Template ref becomes an auth result producer.
- One private decision digest binds task, exact Context/Template revision,
  Runtime binding, provider ID plus the complete normalized reviewed Provider
  authority, login method, and exact prior credential status. Recovery resolves
  the current owner manifest but requires its complete body to equal the
  journaled authority; a valid same-ID replacement cannot change the helper,
  credential, projection, or binding plan after interruption.
  The owner-only effect decision is durable before Broker mutation. Recovery
  distinguishes a decision written before the effect from an exact observed
  consequence, retains a secret-free terminal result through output
  uncertainty, and never repeats a confirmed effect while the decision is
  active. Completed login/import does not permanently shadow a later explicit
  rotation; status never acknowledges or mutates a receipt. Logout remains an
  idempotent same-target convergence path.
- Context deletion and credential create/logout share the installation
  lifecycle authority. Deletion consumes the final adapter's bounded exhaustive
  Context inventory, not the predecessor Workspace deletion adapter. All vault
  records count even when their provider definition was removed; locked,
  incomplete, malformed, unsafe, or over-bound Broker authority fails closed.
- Status does not acquire the creating mutation lock. It uses the existing
  non-creating lifecycle observation and requires two exact complete final
  collection plus exhaustive Broker inventory observations to agree. A fresh
  root remains tree-identical; an appearing lifecycle root or authority/
  inventory drift fails closed without creating `lifecycle.lock`.
- Container-backed acquisition revalidates the complete final Context and
  resolves stable Runtime ID plus semantic revision to a twice-observed exact
  Docker image ID with owner/component/revision/compatibility evidence. The
  persisted mutable image selector is correlation only and is never passed to
  the credential helper as execution authority. Standard providers do not
  inspect a Runtime image.
- Final Context A/B credentials remain isolated by fresh ContextID. Context
  creation/copy has no credential input, and matching predecessor bytes or
  legacy IDs are never read or adopted. The canonical Broker source and its
  embedded runtime input remain byte-identical. Release returns unsupported
  before provider, Broker, helper, or Docker effects; research Catalog/current
  composition selection remains deliberately absent until the atomic cutover.

## Security and public-boundary notes

- Assets and side effects involved: static Template revisions, mutable learned
  policy, Context bindings, Workspace state/home, principal registry, OPA
  projection, final task journals, and deletion recovery.
- Credentials or confidential data involved: none in this packet. Standard
  Workspace-native authentication and research Broker material retain their
  separately governed final ownership; predecessor material is never read or
  adopted.
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
