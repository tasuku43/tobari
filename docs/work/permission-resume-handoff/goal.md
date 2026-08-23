# Work Goal: Resume agent work after trusted-host permission review

- Status: Accepted
- Planning state: Ready
- Retention: temporary
- Retention reason: None
- Governing contract: [Project theses](../../00_theses.md),
  [Product contract](../../01_product_contract.md),
  [Architecture](../../02_architecture.md),
  [Security model](../../03_security_model.md), [Harness](../../04_harness.md),
  `docs/07_authentication.md` through `docs/09_agent_readiness_validation.md`,
  ADR 0024, ADR 0061, ADR 0073, accepted
  [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md),
  accepted [ADR 0081](../../decisions/0081-observe-reviewed-permission-from-an-attached-workspace.md),
  and [Runtime Retirement](../runtime-retirement/goal.md), plus the
  completed WP08 Catalog/output mechanism in [Architecture](../../02_architecture.md)
  and [Harness](../../04_harness.md), and the standard-surface boundary owned by
  [Build profile contract](../build-profile-contract/goal.md)
- Review/delete trigger: Delete after the reviewed contract is promoted, the
  implementation and readiness evidence land, and the change completes
- Successor: None
- Owner: Tobari product, domain, security, and runtime maintainers
- Target: The dedicated `codex/wp07-permission-resume` worktree at base
  `becd6c2b4fb42928152ec4b416cea198c868e875`, after re-observing integrated
  ADR 0079 identity and the completed WP08 recursive Catalog contract
- Related work: [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md),
  the completed WP08 Catalog/output contracts in [Architecture](../../02_architecture.md) and [Harness](../../04_harness.md),
  [Runtime Retirement](../runtime-retirement/goal.md),
  [Build profile contract](../build-profile-contract/goal.md),
  [First public release core](../first-public-release-core/goal.md),
  [legacy Boundary evidence](../context-capability-envelope/goal.md), and
  [policy compaction retirement](../policy-compaction-retirement/goal.md)

## Outcome

After a reviewable ordinary external HTTP or HTTPS request is denied, an agent can run one
bounded, read-only Workspace helper and wait while the operator reviews the
request in the existing separate trusted-host Permission Inbox. The helper
returns only `Allow`, `Deny`, or `Expired`. `Allow` tells the agent that a fresh
deliberate retry is now reasonable; the Gateway still authorizes that new
request independently.

The trusted host remains the only policy owner. The helper cannot propose,
stage, approve, deny, apply, reset, broaden, or inspect policy. Staging remains
inert, no authority exists before the existing final Apply succeeds, and
Tobari never retries the original request.

## Why now

The current denial response preserves the Workspace and agent session and
points the user to `tobari review permissions`, but the agent has no bounded
way to learn that Apply completed. A person or orchestrator must return to the
agent and repeat that fact. Existing Inbox watch/notify improves host attention,
not agent resumption. The missing product concept is a narrow decision
observation handoff, not another permission workflow.

## Ownership, scope, lifetime, and mutability

| Concern | Contract |
|---|---|
| Policy authority | The trusted-host Permission Inbox and canonical internal `policy apply-reviewed` action remain the only reviewed mutation path. |
| Permission scope | One learned decision is keyed by exact `workspace_manifest_id`, exact `workspace_id`, and normalized effect identity. Manifest name, generation, and revision are never substitutes. |
| Wait observation | The one canonical interactive attachment session may observe the terminal disposition of one exact denial correlation ID through its attachment-local helper socket. The record binds `WorkspaceManifestID`, `WorkspaceID`, `AttachmentID`/epoch, the frozen schema-v1 principal projection, and the exact original normalized effect. |
| Scope | V1 covers generic ordinary external HTTP and HTTPS denials whose exact original normalized effect is reviewable. It excludes Host Loopback and protocol-derived GraphQL, MCP, AWS, Kubernetes, Git, and OCI semantics initially. A reviewed conservative `path_template` Allow may still authorize the exact ordinary effect through canonical policy evaluation. |
| Lifetime | One attachment and at most 15 minutes from denial creation, whichever ends first. |
| Mutability | The denial correlation record is immutable. One terminal result consumes it; cancellation or transport loss does not. The helper has no mutation or reconciliation port. The observed result follows canonical active-policy evaluation and is not a permission lease. |
| Information | The child receives only a generated correlation ID, fixed guidance, and one terminal result. It receives no candidate/reference, rule, rationale, reviewer identity, policy bytes, scope expansion, or active revision. |

## Non-goals

- Mutating Workspace or agent policy, or adding a Workspace-visible policy
  mutation channel.
- Adding learned permission, candidates, wait records, or attachment authority
  to Workspace Manifest desired revisions, last-applied slices, observed
  Workspace state, or bounded Manifest reconciliation failures.
- Letting a Manifest mutation, `cluster up`, Workspace entry, session-default
  activation, or creation-default activation copy, reset, approve, deny, or
  otherwise reconcile a learned permission or wait result implicitly.
- Letting `manifest create --copy-from` copy a learned permission, candidate,
  wait record/result, attachment authority, Apply receipt, or current policy
  selection into the fresh WorkspaceManifestID, or trigger reconciliation.
- Letting Runtime copy, build, delete, prune, or restore copy, reset, delete,
  rewrite, approve, or otherwise manage learned permissions, candidates, or
  wait state; Runtime lifecycle journals, prune plans, and receipts are not
  permission authority.
- Letting an agent propose a rule, decision, match mode, lifetime, scope, or
  rationale.
- Automatic approval, default approval, allow-once, temporary permission, or
  time-based policy authority.
- Granting authority before Apply, treating staged state as effective, or
  accepting an Apply receipt before the active revision is confirmed.
- Retrying, replaying, resuming, or reconstructing the original request.
- Returning `pending` as a public terminal result, listing waits, exposing a raw
  poll endpoint, or exposing `policy-candidate` or review-item references to the
  Workspace.
- Capturing query values, headers, bodies, credentials, GraphQL variables, or
  other secret-bearing request material.
- Intercepting child input, reserving a key prefix, injecting text into the
  child terminal, or changing Docker/child PTY ownership.
- Replacing the separate trusted-host Permission Inbox with the experimental
  Operator Console or a same-terminal overlay.
- Reusing or widening `tobari-expose`; permission waiting is a separate helper,
  socket, program identity, and application port.
- Treating `mattolson/agent-sandbox` host policy editing/hot reload or its
  planned interactive unblock workflow as an existing agent wait contract.
- Treating a recommended first-use Manifest draft as persisted authority or
  issuing a wait ID without exact persisted `workspace_manifest_id` and
  `workspace_id` scope.

## Acceptance criteria

- [ ] Before implementation begins, a read-only re-observation gate records the
      actual integrated HEAD and protected worktree, final domain
      types, policy/audit scope, Catalog, schema, migration, desired/applied
      state, attachment boundary, and relevant tests. Historical old-main or
      in-progress working-tree observations are not treated as the final
      implementation baseline.
- [ ] Permission-resume does not modify ADR 0079-owned production files,
      schemas, Catalog entries, migration paths, or generated contracts before
      that integration gate; any overlap is rebased and reviewed against the
      implemented contract before this slice starts.

- [ ] A dedicated catalog-derived `tobari-permission wait --id
      <permission-wait-id> [--format text|json]` command is `RoleUtility` and
      `EffectRead` under stable capability `policy.permission-wait`, consumes
      one exact correlation ID, accepts no decision or scope input, and exposes
      no discovery/list operation.
- [ ] `permission_wait_id` is one required single `InputValueText` input with
      exact domain validation `^pwt_[0-9a-f]{32}$` and minimum/maximum length
      36. It has no completion source, discovery producer, `ReferenceKind`, or
      opaque-reference semantics. If the completed post-WP08 Catalog has no
      generic maximum-length contract, WP07 may add generic `MaximumLength`
      plus derived parsing/help/contract validation; it adds no permission-
      specific Catalog value kind.
- [ ] The denial response adds a versioned, bounded, secret-free wait handoff
      only for supported generic ordinary external HTTP/HTTPS denials with an
      exact reviewable original normalized effect; unsupported denials keep the
      current trusted-host guidance without a wait ID.
- [ ] The wait ID is generated, uninterpreted, non-authoritative, non-enumerable in
      routine interfaces, and useful only over the owning attachment's private
      helper socket. It uses `pwt_` plus 32 lowercase hexadecimal characters
      from 128 bits of CSPRNG output. It is not a policy reference or bearer
      credential.
- [ ] WP07-owned Gateway denial and retained wait/audit records use exact
      `WorkspaceManifestID` + `WorkspaceID` + normalized effect identity; the
      wait record additionally binds exact `AttachmentID`/epoch. Their schema-2
      output exposes no legacy alias. Frozen schema-v1 principal registry,
      Gateway-to-OPA input, persisted learned-policy, and Host Loopback
      route/grant wires retain their exact `context_id`/`project_id`/`context`
      compatibility tokens and gain no dual reader.
- [ ] The wait handoff makes a reviewed hard cutover of the Gateway denial
      document to schema 2; no reader silently widens schema 1 and no dual
      schema is accepted. The helper JSON contract independently remains
      schema 1.
- [ ] Manifest name, generation, complete revision, and activation-slice
      revisions are presentation/correlation or desired/applied facts only;
      none can authorize, select, copy, reset, or approve a learned permission.
- [ ] `manifest create --copy-from` publishes a fresh independent
      WorkspaceManifestID without copying learned permissions, candidates,
      wait state, attachment authority, Apply receipts, or policy selection,
      and performs no permission reconciliation or lineage-based remapping.
- [ ] Runtime copy/build/delete/prune/restore and their journals, plans, and
      receipts perform no permission-rule or wait-state copy, reset, deletion,
      rewrite, approval, or scope migration. Runtime identity and revision are
      not permission-scope dimensions.
- [ ] The helper has no policy mutation, OPA, policy-file, Docker, process,
      general filesystem, general network, or host-terminal capability.
- [ ] Public success output is exactly one of `Allow`, `Deny`, or `Expired` in
      human mode and the same closed enum in schema-1 JSON. There is no public
      `pending`, policy revision, candidate, rule, or scope payload.
- [ ] `Allow` is emitted only when the canonical active policy authorizes the
      exact original normalized effect after final Apply, whether the stored
      reviewed Allow is exact or one conservative `path_template`. The helper
      reuses policy-authority precedence/evaluation and never reimplements rule
      matching.
- [ ] `Deny` is emitted only for an explicit final reviewed deny disposition or
      effective explicit deny for the exact effect. Baseline default-deny,
      staged/applying/failed/canceled state, candidate disappearance or
      compaction, reset without explicit disposition, and ambiguous or
      contradictory policy state are nonterminal and eventually produce
      `Expired` unless an authoritative final result appears first.
- [ ] Expiry is bounded to 15 minutes and attachment lifetime. Cancellation,
      unavailable infrastructure, malformed/unowned IDs, and read failures are
      typed faults rather than false `Expired`, `Deny`, or `Allow` results.
      Ambiguous/contradictory policy disposition remains nonterminal. Only an
      actual validated-record lease expiry returns `Expired`; attachment
      teardown is cancellation or transport loss.
- [ ] The initial safety ceilings are a 15-minute lease, at most 8 live waits
      per attachment, one active connection per ID, at most 3 total connection
      attempts per ID within the lease, a 4 KiB request frame, a 1 KiB response
      frame, and observation at 1 s, 2 s, 4 s, then at most every 5 s. Evidence
      may lower a ceiling; raising one requires Product Owner review.
- [ ] Client cancellation or transport loss counts as an attempt but does not
      consume the record or fabricate `Expired`; a terminal result consumes the
      record. Unknown, consumed, cross-attachment, and attempt-exhausted IDs use
      one non-enumerating fault shape.
- [ ] A terminal result closes the wait. The helper never retries the denied
      request; fixed `Allow` guidance tells the agent to deliberately retry a
      new request, which the Gateway reevaluates from scratch.
- [ ] Child stdin, signals, resize events, alternate-screen state, and exit
      status remain Docker/child-owned. The host continues to review in a
      separate terminal under ADR 0073.
- [ ] The helper behaves like a read-only status operation: it observes one
      active permission disposition and never reconciles Workspace runtime,
      shared cluster state, Manifest desired/applied state, or permission
      authority.
- [ ] The canonical interactive Workspace attachment owner process owns both the private
      helper socket and bounded in-memory wait registry. Gateway supplies one
      secret-free correlation/audit record through a bounded authenticated
      owner ingestion seam and publishes resume only after exact owner ACK.
      Exactly one owner epoch exists per WorkspaceManifestID/WorkspaceID;
      borrowers share it, service-exposure controllers are ineligible, and
      ambiguity fails closed. No persistent store, daemon, Workspace file,
      second policy authority, or child-PID authority is added.
- [ ] Any child process or session within the same live attachment may use that
      attachment's socket and issued ID. A different or new attachment cannot;
      pending adoption and Workspace recreation never rebind a wait.
- [ ] Standard authentication and native credential state remain Workspace-
      owned and absent from Manifest desired/applied state, wait records, helper
      inputs/output, migration reads, and permission observation.
- [ ] The feature adds no persisted wait-state migration. The ADR 0079 migration runs
      with zero live attachments, so no wait is moved or rebound; re-entry
      creates a new attachment and any later wait is freshly issued. Its
      explicit predecessor migration preserves legacy Context UUID bytes as
      `workspace_manifest_id`, legacy ProjectInstance UUID bytes as
      `workspace_id`, and learned rules plus any retained bounded denial/audit
      evidence under those renamed fields. Candidates are rederived from the
      final identity; wait records and active attachment authority remain
      ephemeral and are not migrated.
- [ ] Domain, application, infrastructure, CLI/catalog, Gateway, hostile-output,
      concurrency, expiry, stale-state, and agent-readiness tests prove the
      contract, including zero external processing for routine interpretation.
- [ ] The governing theses, product, architecture, security, harness, Gateway
      denial schema, helper catalog, README, capability/schema ledgers, and
      generated catalog documentation are updated together during the future
      implementation.
- [ ] A reviewed ADR and governing consequences land before mechanism. WP08 is
      complete first and remains the sole owner of Catalog-wide recursive
      output/reference traversal; WP07 consumes it without a parallel walker.
- [ ] The helper is a standard release surface unless an accepted WP04 contract
      separately changes that classification.
- [ ] Future implementation passes focused evidence, `task check`,
      `task security`, `task public:check`, `task release:check`, and the
      relevant permission-resume agent-readiness scenario, is committed, and
      then notifies the root/control thread.

## Governing docs

- Thesis: progressive trusted-host authorization, discovery/action separation,
  controlled side effects, executable claims, and secret-free external text in
  `docs/00_theses.md`
- Product contract: Gateway denial, permission review, output, reference flow,
  compatibility, and TTY behavior in `docs/01_product_contract.md`
- Architecture: domain/app/infra/CLI ownership, Gateway audit, attachment
  helper boundary, and catalog derivation in `docs/02_architecture.md`
- Security: post-policy effects, trusted-host authority, untrusted request
  text, attachment isolation, bounded I/O, and no secret projection in
  `docs/03_security_model.md`
- Harness: catalog, schema, hostile-output, PTY, concurrency, and readiness
  claims in `docs/04_harness.md`
- Authentication and external API contracts: denial-before-resolution and
  preserved request identity in `docs/07_authentication.md` and
  `docs/08_external_api_contracts.md`
- Readiness: presentation-independent answer keys and zero undeclared external
  processing in `docs/09_agent_readiness_validation.md`
- Decisions: ADR 0024 for confirmed Apply, ADR 0061 for the experimental
  Operator Console, and ADR 0073 for separate-terminal ownership

## Completion definition

This packet becomes implementation-complete only after all acceptance criteria
and tasks have evidence, the conclusions are promoted into governing contracts,
the canonical gate passes, and this temporary packet is deleted. Creating this
Ready/Planned packet does not satisfy that definition and starts no
implementation.
