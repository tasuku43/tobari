# Work Context: Permission resume handoff

## Current behavior

### Integration state on 2026-08-23

- The Workspace Manifest/Workspace model and one-time copy decisions have been
  promoted into accepted
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and the current [theses](../../00_theses.md),
  [product](../../01_product_contract.md),
  [architecture](../../02_architecture.md),
  [security](../../03_security_model.md), and [harness](../../04_harness.md)
  contracts. Their temporary planning packets were contractually deleted.
- Implementation is isolated on branch `codex/wp07-permission-resume` in
  `/Users/tasuku/work/github.com/tasuku43/tobari-wp07`, based on integrated
  HEAD `becd6c2b4fb42928152ec4b416cea198c868e875`. WP08 recursive Catalog output
  and produced-reference derivation is present and is reused without a local
  walker. The worktree was clean at implementation start.
- [Runtime Retirement](../runtime-retirement/goal.md) remains fixed without an
  implementation-start signal. It constrains permission-resume negatively but
  does not add permission authority or make Runtime retirement implementation
  a prerequisite for the independent wait-only slice.

### Verified repository facts

- Initial research was performed against local `main` and `origin/main` at
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42` after `git fetch origin main` on
  2026-08-23. The worktree was clean before this packet was created.
- Gateway schema 1 returns `permission_review_available`, the fixed host command
  `tobari review permissions`, `automatic_retry:false`, and
  `retry_after_review:true` for learnable denials. Its retained denial evidence
  excludes query values, headers, bodies, and credentials.
- `review permissions` is the host-owned Permission Inbox. Raw-TTY mode can
  stage decisions and call the internal fixed-target `policy apply-reviewed`;
  redirected and JSON modes are read-only. Staging grants nothing.
- Apply rebuilds the bounded candidate view, validates unchanged opaque IDs and
  the current-main legacy Context/Workspace scope, performs the canonical
  mutation, and returns only after its active permission-policy revision and
  receipts validate. Cancellation after confirmed mutation cannot turn success
  into replay permission.
- `--watch` refreshes the bounded Inbox, removes stale staged decisions, and
  can emit only a fixed OSC 9 or BEL cue for new typed items. It notifies the
  host reviewer, remains open after Apply, and never retries the child request.
- Candidate IDs are stable opaque hashes of typed denial identity, but
  candidates are derived from a bounded denial window rather than a persisted
  inbox. Candidate disappearance alone cannot distinguish Allow, Deny, expiry,
  compaction, reset, or evidence eviction.
- The experimental Operator Console reuses the same canonical Apply action over
  a random loopback foreground session. It is absent from standard builds and
  does not provide a child-facing completion handoff.
- `tobari-expose` proves that a dedicated, hard-coded, read-only-mounted helper
  and attachment control socket can exist without exposing the root CLI. Its
  service-grant semantics and references are unrelated and must not be widened.
- ADR 0073 rejects a same-terminal Permission Inbox for V1. Docker and the child
  own the attached terminal. The optional relay is display-only, reserves no
  key, and cannot restore an arbitrary child's alternate screen without a new
  terminal-emulation trust boundary.
- The checked-in `bin/tobari` reported commit
  `bb641387afce0cd98d772a66527f67aaa57463ad`, so its read-only help output is a
  stale observation rather than authoritative evidence for current HEAD.
- These repository facts describe the predecessor checkout used to design this
  packet. They remain historical evidence for the problem, not assertions
  about the integrated contract now governed by ADR 0079 and current durable
  documents.

### Accepted ADR 0079 model facts

- ADR 0079 is the accepted durable upstream decision. Final public vocabulary
  is Workspace Manifest / Manifest,
  `manifest` / `--manifest`, `workspace_manifest_id`, Runtime, Workspace,
  `workspace_id`, and subordinate Project root. Final V1 retains no public
  Context alias and no semantic `project_id` or `instance_id` identity.
- Workspace Manifest is a host-owned, CLI-managed, stable-ID desired
  declaration whose accepted mutations publish complete immutable revisions.
  Boundary is invariant under one WorkspaceManifestID; changing Boundary
  creates a different Workspace Manifest.
- One Workspace is durably bound to `(ProjectRoot, WorkspaceManifestID)` and
  has one stable WorkspaceID. Manifest name, generation, and revision digest do
  not replace those authority identities.
- Desired Manifest revision, last successful applied slices, observed runtime,
  and the last bounded reconciliation failure are distinct. Manifest mutation
  changes desired state only; Workspace entry and `cluster up` are the only
  relevant explicit reconciliation boundaries, and reads never converge state.
- Standard authentication state, learned permissions, and attachment authority
  are outside Workspace Manifest desired/applied state. A recommended first-use
  draft is presentation, not persisted authority.
- The selected pre-public migration direction retains legacy Context UUID bytes
  as WorkspaceManifestID and legacy ProjectInstance UUID bytes as WorkspaceID.
  Learned rules and retained bounded denial/audit evidence move to the renamed
  identity fields so candidates can be rederived under final semantics; active
  attachment authority remains a separate ephemeral concern.

### Accepted ADR 0079 one-time copy facts

- Manifest copy is `manifest create --copy-from NAME --name NAME`; Runtime
  source copy is `runtime create --copy-source-from standard|NAME --name NAME`.
  `--base` is removed without an alias, and no shared production derivation
  type or persisted lineage/provenance field is introduced.
- Manifest copy revalidates the exact reviewed current immutable source
  revision immediately before publishing a fresh WorkspaceManifestID at
  generation 1. It copies the complete desired declaration, not Workspace,
  authentication, learned permission, candidate, wait, attachment,
  AppliedEntry, failure, observed state, or current/default selection.
- Runtime copy initializes a fresh RuntimeID with empty history from current
  editable source. It neither forks an immutable revision nor copies permission
  state. Neither copy action performs reconciliation.

### Accepted Runtime Retirement facts

- Runtime lifecycle uses exact references: `runtime delete --id <runtime-ref>
  --confirm=delete`; read-only `runtime prune dry-run`; `runtime prune apply
  --plan <prune-plan-ref> --confirm=prune`; `runtime restore --id
  <runtime-revision-ref>`; read-only `review runtimes`; and `runtime build --id
  <runtime-ref>`. Name-based or omitted-name action selection and the rejected
  revision-delete/GC/global-prune/force aliases are not dependencies of
  permission-resume.
- Prune apply revalidates its complete exact plan under lock and makes no
  partial change when stale. Restore accepts execution availability only after
  an exact recorded-digest match; failure leaves history unchanged. Retirement
  and prune retain immutable Runtime revision history/snapshots until whole-
  Runtime retirement and never delete authority/history merely to reclaim an
  execution image.
- The protection graph distinguishes current and retained Manifest revisions,
  Workspace last-successful AppliedEntry, pending adoption, and observed
  running/stopped/foreign containers; unknown evidence fails closed. The
  existing Runtime manifest schema remains, while build/retirement journals and
  bounded idempotent receipts are owner-only sidecars. Those records support
  Runtime lifecycle only; they do not own or mutate permission rules.
- `last_used` remains unknown without separately approved exact evidence, and
  nested output reference producer/consumer derivation remains a Catalog-wide
  invariant. Permission-resume must neither infer policy use from Runtime
  timestamps nor add a Runtime-specific reference validator.
- Runtime delete, prune, restore, build, and copy do not copy, reset, delete,
  rewrite, or approve learned permissions, candidates, wait records, or wait
  results. RuntimeID/revision/image material is not part of permission scope.

### Product Owner-fixed WP07 decisions

- The accepted thesis is one separate hard-coded, attachment-local wait-only
  helper. Agent proposal, auto approval, pre-Apply authority, automatic retry,
  public pending/list/poll APIs, candidate-reference waiting, same-terminal
  overlay, and terminal injection are explicitly rejected.
- The public input is required single `InputValueText` with exact domain syntax
  `^pwt_[0-9a-f]{32}$` and length 36. It is a non-reference correlation value:
  no `ReferenceKind`, discovery producer, completion, or permission-specific
  Catalog value kind. Only a missing generic post-WP08 maximum-length contract
  may be filled by generic `MaximumLength` and its derived enforcement.
- Only the WP07-owned Gateway denial and retained wait/audit record change
  through one reviewed schema-2 hard cutover. Frozen schema-v1 principal,
  Gateway-to-OPA, learned-policy, and Host Loopback route/grant wires remain
  byte/schema stable. Helper JSON independently remains schema 1. The only successful results are
  `Allow`, `Deny`, and `Expired`, without policy detail or authority.
- V1 accepts generic ordinary external HTTP and HTTPS denials with one exact
  reviewable original normalized effect. Host Loopback and protocol-derived
  GraphQL/MCP/AWS/Kubernetes/Git/OCI semantics remain excluded initially.
- Canonical policy evaluation determines disposition. A final active exact or
  conservative `path_template` Allow may authorize the exact original effect;
  only an explicit final reviewed/effective explicit deny returns `Deny`.
  Baseline default-deny and every nonfinal/ambiguous state remain nonterminal.
- The canonical interactive Workspace attachment session created before the
  child owns the private socket and bounded memory registry. Exactly one owner
  epoch exists per WorkspaceManifestID/WorkspaceID; concurrent borrowers share
  it, while service-exposure controller attachments are distinct and
  ineligible. Gateway joins one authenticated frozen schema-v1 principal to one
  bounded private session record and publishes resume only after the owner ACKs
  the exact immutable record. Same-live-attachment child sessions may use the
  socket/ID; a new attachment, pending adoption, or Workspace recreation never
  inherits or rebinds it.
- Initial ceilings are 15 minutes, 8 live waits, one active connection per ID,
  3 total attempts per ID, 4 KiB request, 1 KiB response, and a 1/2/4/5-second
  observation schedule. Evidence may lower but not raise them without Product
  Owner review. Terminal result consumes the record; cancellation/transport
  loss does not.
- The ADR 0079 migration requires zero live attachments, so waits never migrate. WP08
  precedes WP07 and owns Catalog-wide output/reference traversal. The helper is
  standard release surface unless an accepted WP04 contract changes that.

### Evaluation

The missing operation is decision observation, not policy mutation or
reconciliation. A narrow
waiter can remove the human step of telling an agent that review completed
while preserving every existing authorization step. Candidate discovery,
review presentation, staging, Apply, and request retry remain separate.

Under ADR 0079, the exact learned-permission scope is
`WorkspaceManifestID + WorkspaceID + normalized effect`; a wait record adds
`AttachmentID`/epoch without changing the durable permission key. Both omit
Manifest name, generation, complete revision, activation-slice revisions,
desired/applied Workspace state, Runtime identity, Project root label, and
child PID. This keeps permission lifetime independent from Manifest publication
and entry reconciliation while active Gateway policy still enforces the
immutable Boundary and current trusted-host decision.

### Re-observed implementation baseline

- ADR 0079 production identity, the canonical Host Loopback owner/borrower
  epoch, frozen schema-v1 principal/OPA/route wires, Gateway denial, bounded
  policy audit/candidate path, helper packaging, and WP08 recursive
  `ProducedRefs` traversal were re-observed at the implementation base.
- Generic Catalog `MaximumLength` remains absent at the implementation base;
  WP07 therefore owns only that generic primitive and its derived validation,
  parsing/help display, cloning, and contract tests.

### Still unverified during implementation

- The exact supported host platforms' Unix-socket peer and mount behavior for a
  second helper must be replayed on the release matrix.
- The fixed bounds are initial safety ceilings rather than measured capacity.
  Load/cancellation evidence may select lower production values; it cannot
  raise them without Product Owner review.
- No user study yet establishes whether agents reliably follow a three-value
  wait result without treating `Allow` as authority or replaying a non-idempotent
  request. The readiness fixture must test that behavior.
- The requested comparison identity is now confirmed as
  `mattolson/agent-sandbox`, not Kubernetes SIGs Agent Sandbox.
- ADR 0079 still owns general child-session adoption behavior. WP07 fixes
  only its own boundary: a child session in the same live attachment may use
  the same socket and issued ID, while any new/different attachment may not.
- ADR 0079 leaves cross-cutting review to select current-only versus one previous Manifest
  revision body, Git fallback slice ownership, sufficient Docker migration
  evidence, or every cluster migration detail. Wait identity does not depend on
  retained Manifest bodies or Git fallback. WP07 additionally fixes that the
  ADR 0079 migration relevant to this feature requires zero live attachments and
  never moves a wait.

### Explicit inference

Because the current candidate set is bounded and reconstructed, waiting on a
candidate ID would make eviction look like a decision. A separate immutable
denial correlation record, retained only for the bounded attachment lease, is
therefore required to produce an honest `Expired` result.

## Integrated structure to revalidate before implementation

These paths explain the old-main evidence and likely overlap, but are not an
implementation file list. The pre-implementation gate must rediscover their
current successors and ownership after WP08 before any edit.

- Domain vocabulary and invariants: `internal/domain/tobari/policy.go` and
  `internal/domain/tobari/policy_template.go`
- Application queries and reviewed mutation: `internal/app/tobaricmd/policy_queries.go`
  and `internal/app/tobaricmd/policy_mutation.go`
- Gateway denial and embedded source: `gateway/addon/tobari_gateway.py` and
  `internal/infra/runtimeassets/assets/gateway/addon/tobari_gateway.py`
- Docker attachment/helper boundary: `internal/infra/dockerruntime/`, including
  `project_runtime.go` and `workspace_service_helper_asset.go`
- Host notification boundary: `internal/infra/terminal/notification.go`
- CLI catalog and Permission Inbox: `internal/cli/runtime_catalog.go`,
  `internal/cli/tobari.go`, and `internal/cli/policy_review_selector.go`
- Current helper program: `cmd/tobari-expose` and `internal/cli/catalog.go`
- Tests: `internal/cli/policy_review_pty_test.go`,
  `internal/cli/resumable_permission_review_presentation_test.go`, Gateway
  denial tests, domain policy tests, application mutation tests, runtime helper
  tests, and Docker integration scripts
- User contract: `README.md`, `docs/01_product_contract.md`, and
  `docs/09_agent_readiness_validation.md`

## Constraints

- Trusted-host-only policy ownership is absolute. Workspace and agent processes
  receive no proposal, decision, Apply, reset, policy-write, or scope-selection
  input.
- Discovery and action stay separate. The wait ID is deliberately not a
  `policy-candidate`, review-item, rule, or mutation target reference. It is a
  typed correlation value for one read-only observation.
- Catalog declares it as required single `InputValueText`, with exact domain
  syntax and length enforcement. It has no completion, producer, or
  `ReferenceKind`; WP08's reference traversal must therefore ignore it by
  construction rather than special case.
- The helper's request carries only fixed schema and the wait ID. The owning
  attachment socket, not possession of the ID, establishes Workspace identity.
- The trusted host binds that socket and record to exact WorkspaceManifestID,
  WorkspaceID, AttachmentID/epoch, and original normalized effect from the
  selected attachment. Workspace-
  supplied Manifest names, generations, revision digests, Project roots,
  environment, or request fields cannot select the scope.
- Manifest revision/name, Runtime, Project root label, and child PID are not
  wait or permission authority.
- Learned permission, candidates, and Apply receipts remain policy-owned side
  state keyed by WorkspaceManifestID/WorkspaceID/normalized effect. A wait
  record adds AttachmentID/epoch and remains attachment-owned memory. They are
  never fields of a Workspace Manifest desired revision,
  Workspace AppliedEntry, observed runtime, or reconciliation failure.
- Manifest publication, `cluster up`, Workspace entry, child-session startup,
  and creation-default activation cannot implicitly copy, reset, allow, deny,
  apply, or reconcile this permission state. The wait helper has the same
  zero-reconciliation rule as read-only status.
- Manifest copy creates a fresh authority identity and cannot copy, translate,
  or infer learned permissions, candidates, wait state, attachment authority,
  or Apply receipts from its source. The absence of persisted lineage also
  forbids later permission lookup through a `copied_from` relationship.
- Runtime copy/build/delete/prune/restore and Runtime retirement journals,
  plans, and receipts cannot read permission state for lifecycle authority or
  mutate that state as cleanup. RuntimeID and Runtime revision do not widen the
  permission scope.
- No authority exists before canonical Apply completes and the running revision
  agrees with state. An `Allow` observation is retry-readiness evidence only;
  the Gateway remains authoritative for the next request.
- Original request method, body, headers, query, credentials, and transport
  stream are never retained or replayed by this feature.
- External text is untrusted. No denial field is interpolated into commands,
  terminal control sequences, faults, or notification payloads.
- The helper must preserve one propagated cancellation/deadline chain, bounded
  frames, bounded wait duration, bounded concurrency, deterministic
  cancellation, and no general executor, filesystem, network, Docker, OPA, or
  terminal access.
- The repository remains pre-public. The added handoff deliberately hard-cuts
  Gateway denial schema 1 to schema 2 after review; readers accept no dual
  shape. Helper JSON has its own schema 1. No old Context/project field alias
  remains after the ADR 0079 cutover.

## External facts

Official primary sources were rechecked on 2026-08-23. No external code or
asset was fetched or embedded.

- NVIDIA OpenShell, [Policy Advisor](https://docs.nvidia.com/openshell/sandboxes/policy-advisor):
  a sandboxed agent may submit a narrow `policy.local` proposal, a developer may
  approve or reject it outside the sandbox, approved policy hot-reloads, and a
  wait endpoint reports proposal status. Its optional auto-approval and
  agent-retry flow are intentionally outside Tobari's constraints. The useful
  precedent is that readiness is not reported until the reloaded policy covers
  the reviewed rule.
- NVIDIA OpenShell, [Security best practices](https://docs.nvidia.com/openshell/latest/security/best-practices):
  denied endpoints are surfaced for developer review and approved changes are
  durable policy revisions. Tobari retains host ownership but does not accept an
  agent-authored proposal.
- Docker, [Local policy](https://docs.docker.com/ai/sandboxes/governance/access-controls/local/):
  local policy allow/deny changes can be immediate and scoped by sandbox, while
  organization governance remains an upper bound. This is a policy-management
  surface, not a wait-only handoff.
- Docker, [MCP policy reference](https://docs.docker.com/ai/sandboxes/governance/reference/mcp-policy/):
  `@requireApproval` uses an in-session per-authorization elicitation and denies
  when the client cannot present it. That exact-call continuation differs from
  Tobari's durable out-of-band Apply and fresh manual retry.
- Docker, [Sandboxes security](https://docs.docker.com/ai/sandboxes/security/):
  Docker describes microVM isolation, host-side proxy enforcement, and
  proxy-injected credentials. Tobari must not borrow those stronger runtime or
  credential-isolation claims for its Docker-backed Workspace.
- nono, [Runtime Supervisor](https://www.nono.sh/runtime-supervisor): supervised
  execution may intercept a denied operation, obtain a terminal/webhook/API
  human decision, expand session capability, and transparently continue the
  held operation. The bounded decision precedent is relevant, but both
  capability expansion and transparent continuation are excluded here.
- mattolson/agent-sandbox, [official repository and README](https://github.com/mattolson/agent-sandbox):
  the agent can read a sanitized live allowlist, but network policy can be
  changed only from the host with `agentbox edit policy`; active changes hot-
  reload. Its [roadmap](https://github.com/mattolson/agent-sandbox/blob/main/docs/roadmap.md)
  describes an interactive unblock workflow as planned, not as a current
  agent-side wait/result contract. Tobari similarly preserves host mutation
  ownership but adds a narrower completion observation with no agent proposal
  or project-owned policy edit.

These external products are fast-moving. Recheck their official current docs
after the integrated ADR 0079 baseline and WP08 are re-observed at design
finalization; comparison
evidence never changes Tobari authority without Product Owner review.

## Option comparison

| Option | Authority and information | TTY and lifetime | DoS, enumeration, stale, concurrency | Decision |
|---|---|---|---|---|
| Dedicated wait-only helper | No mutation; one three-value result; attachment-scoped | Child runs an ordinary command; 15-minute/attachment bound | Private socket, no list, bounded frames/waiters; explicit expiry and active-revision checks | Recommended V1 |
| Wait on `policy-candidate` | Exposes a host action reference and candidate details | Could use the same helper | Bounded-log eviction is ambiguous; invites reference probing and conflates discovery with observation | Reject |
| Apply-complete terminal injection | Host could announce completion without child API | Violates child TTY ownership and fails for full-screen/raw children | Injection races with child state and cannot bind one denial safely | Reject |
| Read-only denial guidance only | No added authority or data | Current behavior | Safe but still requires a person to tell the agent when Apply completes | Retain as fallback, insufficient alone |
| Public `status pending` command | Read-only in principle | Short calls encourage repeated polling | Adds enumeration, ambiguous disappearance, thundering herd, and a fourth public state | Reject |
| Existing watch/notify | Host-only authority and fixed cue | Separate trusted-host TTY | Bounded host refresh; does not notify the agent | Retain for reviewer attention |
| Operator Console | Canonical host Apply but experimental browser session | Separate host browser lifetime | Session bearer and server add surface; still no child completion signal | Keep optional, not V1 dependency |
| Same-terminal overlay | Could keep host authority | Requires Tobari to own terminal multiplexing/emulation | Unbounded hostile output, alternate-screen restoration, backpressure, and signal races | Reject under ADR 0073 |
| Workspace-visible poll endpoint | Easy agent polling | Independent of TTY | Widens network/API surface, allows enumeration and unbounded clients, and duplicates helper semantics | Reject |

## Remaining dependencies and implementation evidence

- [ ] After WP08 completion, confirm whether Catalog already has a generic
      maximum-length contract. If absent, add only generic `MaximumLength` and
      its derived parser/help/contract enforcement with min/max 36 here.
- [ ] From the integrated ADR 0079 implementation, identify the exact existing attachment-owner
      process and bounded control/audit seam that carries the Gateway's secret-
      free record. Ownership and the prohibition on a second store are fixed;
      only the implemented seam remains to be observed.
- [ ] Measure whether the fixed ceilings fit supported agent behavior and
      resource limits. Evidence may lower them but cannot raise them without
      Product Owner review.
- [ ] Specify component identity/version mismatch behavior after the final
      ADR 0079/WP08 component topology is observed.
- [ ] Reconcile general ADR 0079 new-child-session behavior with the fixed same-
      live-attachment permission rule without allowing cross-attachment reuse.

## Thesis evidence

- User friction: the current flow closes authorization but not the agent's
  knowledge that it may deliberately retry.
- Repeated design boundary: OpenShell, Docker MCP approval, and nono combine
  decision and request continuation more tightly than Tobari permits. A Tobari
  helper must observe only the committed decision and never carry the request.
- Downstream impact: Gateway denial schema, domain identity, application read
  port, attachment-local infrastructure, helper catalog, generated help,
  README, capability/schema ledgers, security claims, and readiness fixtures.
- Decision trigger: if implementation requires a bearer credential, public
  candidate reference, persistent inbox, same-terminal control, or policy
  mutation from the Workspace, revise this plan and the governing thesis before
  coding rather than adding an exception.
- Upstream decision consequence: any implementation that keys permission to a
  Manifest revision/generation/name, stores it in desired/applied Manifest
  state, or lets entry/Manifest mutation reconcile it contradicts Domain Model
  ADR 0079 and must stop for owner review.

## Reproduction

Read-only inspection performed for this packet:

```sh
git fetch origin main
git rev-parse HEAD
git rev-parse origin/main
git status --short --branch
git log --oneline --all --grep='permission\|policy review\|resum' -n 30
rg -n 'automatic_retry|retry_after_review|apply-reviewed|Permission Inbox' \
  README.md gateway internal docs .harness
./bin/tobari help review permissions --format agent
```

The binary command was help-only and cancel-free; its embedded commit was stale
and was not used to infer current implementation behavior.

Before future implementation, repeat observation after WP08 lands against the
then-current integrated ADR 0079 baseline:

```sh
git fetch origin main
git rev-parse HEAD
git rev-parse origin/main
git status --short --branch
rg -n 'WorkspaceManifestID|WorkspaceID|workspace_manifest_id|workspace_id' \
  internal gateway docs README.md .harness
rg -n 'permission|candidate|apply-reviewed|Catalog|migration' \
  internal gateway docs README.md .harness
```

Record the exact integrated revision, preserve unrelated changes, inspect the
final domain/application/infrastructure/CLI and tests, and run only safe
read-only help or focused verification needed to establish the new baseline.
If the checkout has new overlapping in-progress work, stop this gate rather
than treating partial state as accepted implementation evidence.

## Security and public-boundary notes

- Assets and side effects: one generated read-only helper, one attachment-local
  Unix socket, bounded in-memory correlation records, the existing secret-free
  Gateway audit, and read-only policy/revision observation on the trusted host.
- Credentials and confidential data: none. The wait ID is generated correlation
  data, not authorization; the socket endpoint supplies attachment identity.
  Helper output contains only the closed result enum.
- External I/O: no upstream network from the helper. Its only I/O is one bounded
  local socket exchange and stdout/stderr.
- Failure semantics: a known lease ending yields `Expired`; infrastructure,
  ownership, parsing, consistency, and cancellation failures remain typed
  faults. No fault grants retry permission.
- Idempotency: waiting is read-only and repeatable within its lease. A returned
  result does not make a subsequent effect idempotent and does not authorize
  replay.
- Compatibility: no released state exists. The helper itself adds no persisted
  wait state, but it depends on ADR 0079's explicit predecessor
  migration to rename learned-policy and retained denial/audit scope fields
  while preserving legacy UUID bytes. Candidates are rederived; wait and
  attachment records are ephemeral and are not migrated. Older Gateway,
  runtime, helper, and CLI components must fail closed on identity or contract
  mismatch.

## Cross-packet interactions

- `first-public-release-core` deliberately excludes temporary/time-based
  permissions and per-Workspace Gateway instances. This packet adds neither: a
  bounded observation lease is not policy authority, and the helper uses the
  existing attachment process and shared Gateway.
- `first-public-release-core` and `policy-compaction-retirement` own learned
  policy identity and canonical exact/path-template evaluation. WP07 always
  waits on one exact original effect, but a final reviewed conservative
  `path_template` Allow may authorize that effect through the canonical
  evaluator. WP07 must not copy the matching algorithm.
- [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
  and current durable contracts own the overlapping public vocabulary, stable
  principal identity, migration, desired/applied/observed state, schema, and
  Catalog cutover. Permission-resume must re-observe their actual integrated
  implementation after WP08 and before coding; it adds no alternate model.
- The completed WP08 contracts in [Architecture](../../02_architecture.md) and
  [Harness](../../04_harness.md) precede WP07 and own the one Catalog-wide
  recursive output/reference traversal. WP07 adds no walker,
  `ReferenceKind`, discovery producer, or permission-specific value kind; it
  consumes the completed generic Catalog contract and may add only generic
  `MaximumLength` if still absent.
- ADR 0079's promoted one-time copy contract has only a negative permission
  dependency: Manifest copy excludes learned permissions, candidates, waits,
  attachments, and receipts and causes no reconciliation; Runtime source copy
  has no permission effect. Commit `07535a9` is exact upstream implementation
  evidence. Future concurrent Catalog or generated-file edits still require
  merge-order coordination.
- `runtime-retirement` is design-fixed but not implementation-started. Delete,
  prune, restore, build, lifecycle journals, prune-plan references, and
  receipts add no permission responsibility. WP03 does not need to land first,
  but shared Catalog/reference-flow/generated files must not be independently
  regenerated over one another.
- `build-profile-contract` (WP04) owns any later change to standard versus
  experimental/release composition. Until such an accepted change, the
  permission helper is standard release surface.
- `context-capability-envelope` is legacy implementation evidence for the
  immutable Boundary only. ADR 0079 supersedes its Context vocabulary
  and flat-manifest lifecycle. A wait result cannot exceed the Boundary or
  mutate Workspace Manifest desired/applied state.
- Release/profile packets must include the helper in standard runtime assets
  and keep source/snapshot identity, provenance, and capability ledgers aligned,
  unless an accepted WP04 contract separately changes the surface.

## Glossary

- **Permission wait ID:** generated, opaque, non-authoritative correlation
  value for one retained denial; it is not a policy reference or credential.
- **Wait helper:** the dedicated `tobari-permission` Workspace program with one
  read-only `wait` command.
- **Retry-ready:** active policy was observed to allow the prior exact identity;
  a new request is still independently authorized by Gateway.
- **Expired:** the known denial's time lease ended while the connection remained
  deliverable without a terminal active decision being observed. Attachment
  transport loss is a fault, not a guaranteed `Expired` result.
- **Terminal result:** one successful outcome from the closed enum `Allow`,
  `Deny`, or `Expired`; unrelated system faults are not terminal results.
- **Permission scope:** exact WorkspaceManifestID, WorkspaceID, and normalized
  effect identity. Manifest name/revision/generation and Project root are not
  substitutes.
- **Manifest state separation:** learned permission and wait observation are
  outside desired, last-applied, observed-runtime, and bounded-failure records.
