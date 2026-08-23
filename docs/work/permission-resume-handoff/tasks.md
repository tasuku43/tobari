# Work Tasks: Permission resume handoff

## Understand

- [x] Read `AGENTS.md`, theses, product, architecture, security, harness,
      authentication, external-contract, and readiness governance in numeric
      order.
- [x] Inspect README, CLI Catalog, domain/application/infrastructure paths,
      Gateway denial, candidate derivation, Permission Inbox, watch/notify,
      Apply, attachment helper, TTY ownership, Operator Console, tests, and
      recent permission-review history.
- [x] Fetch `origin/main`, verify local/current main at
      `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`, and protect a clean starting
      worktree.
- [x] Recheck current official OpenShell, Docker Sandboxes/MCP policy, and nono
      documentation; replace the mistaken Kubernetes identity with the
      requested `mattolson/agent-sandbox` official repository/README/roadmap
      comparison in `context.md`.
- [x] Separate verified facts, evaluation, unverified implementation facts,
      and explicit inference in `context.md`.
- [x] Reconcile this packet with accepted ADR 0079 and current durable contracts:
      Workspace Manifest vocabulary, WorkspaceManifestID/WorkspaceID scope,
      desired/applied/observed separation, explicit reconciliation, and exact
      predecessor-migration direction.
- [x] Reconcile this packet with ADR 0079's promoted one-time copy contract and
      the Runtime Retirement decision, including their implementation-status
      distinction and permission-state negative boundaries.
- [x] Replace deleted upstream-packet references with ADR 0079, current durable
      contracts, and exact commit evidence `07535a9`/`428812f`.

## Decide before implementation

- [x] Accept the recommended wait-only helper and reject
      agent proposals, auto approval, pre-Apply authority, automatic retry,
      candidate wait, public pending/status, raw polling, same-terminal overlay,
      and terminal injection.
- [x] Fix `permission_wait_id` as required single `InputValueText`, exact domain
      syntax `^pwt_[0-9a-f]{32}$`, minimum/maximum length 36, and no completion,
      discovery producer, opaque-reference semantics, or `ReferenceKind`. Add
      only generic `MaximumLength` after WP08 if the completed Catalog lacks it.
- [x] Hard-cut Gateway denial to reviewed schema 2 without dual acceptance;
      keep helper JSON as its independent schema 1 and remove all legacy
      Context/project compatibility fields.
- [x] Fix stable capability ID `policy.permission-wait`, RoleUtility,
      EffectRead, and the separate hard-coded Program/grammar.
- [x] Fix generic ordinary external HTTP/HTTPS with an exact reviewable
      original normalized effect as V1 scope; exclude Host Loopback and
      protocol-derived GraphQL/MCP/AWS/Kubernetes/Git/OCI semantics initially.
- [x] Reuse canonical policy evaluation: active exact or conservative
      path-template Allow may return `Allow`; only explicit final/effective
      explicit deny returns `Deny`; default-deny and nonfinal/ambiguous states
      are nonterminal until authoritative disposition or lease expiry.
- [x] Fix the initial safety ceilings at 15 minutes, 8 live waits, one active
      connection and 3 total attempts per ID, 4 KiB/1 KiB frames, and 1/2/4/5-
      second observation. Evidence may lower but not raise them without Product
      Owner review.
- [x] Fix existing attachment-owner socket/registry ownership, existing bounded
      Gateway control/audit seam, same-live-attachment child use, no rebinding,
      cancellation without consumption, terminal consumption, and uniform
      unknown/consumed/attempt-exhausted fault shape.
- [x] Fix zero-live-attachment ADR 0079 migration for this feature; migrate no wait
      and issue any later wait freshly after re-entry.
- [x] Fix WP08 as the preceding Catalog-wide traversal owner and standard
      release composition unless an accepted WP04 decision changes it.
- [ ] Specify component identity/version mismatch behavior after observing the
      integrated ADR 0079/WP08 topology.
- [ ] Accept an ADR and propagate any thesis/security/architecture consequence
      before implementation.

## Hold and re-observe integrated upstream contracts

- [x] Record integrated ADR 0079 authority and commit evidence `07535a9` and
      `428812f`, plus current HEAD `52a53bcc69a0f2bdf9bf2a6782ecd98bacd8b0e1`;
      do not edit its production files, schema, Catalog, migration,
      desired/applied state, generated outputs, or durable docs.
- [ ] Immediately before implementation, fetch and record actual HEAD/upstream,
      protect the worktree, and re-read the implemented domain/application/
      infrastructure/CLI, Catalog, schema, migration, policy/audit, attachment,
      and test surfaces rather than relying on this synchronization snapshot.
- [ ] Confirm then-current WorkspaceManifestID/WorkspaceID/ProjectRoot scope,
      principal and policy/audit fields, desired/applied/observed/failure split,
      predecessor UUID retention, candidate rederivation, and exclusion of
      active attachment authority.
- [ ] Re-observe the decision status of ADR 0079's new-child-session and migration
      Docker-evidence/cluster-stop/no-attachment questions. Preserve unresolved
      choices as dependencies; after an owner decision, update this packet with
      exact helper availability and teardown consequences before the affected
      implementation slice.
- [ ] Run safe read-only CLI/help and focused checks, record the integrated
      evidence, and stop for owner review if implemented contracts diverge.
- [ ] Verify no unmerged upstream ownership or worktree conflict remains in files
      permission-resume would change. Add no alternate identity path,
      compatibility alias, or dual reader.
- [ ] Wait for `WP08_IMPLEMENTATION_COMPLETE`, inspect its actual Catalog input
      bounds and recursive output/reference traversal, and reuse it without a
      second walker or validator. Determine only whether generic
      `MaximumLength` remains absent.
- [ ] At ADR 0079/WP08 design finalization, recheck current official
      OpenShell, Docker Sandboxes, nono, and `mattolson/agent-sandbox` sources;
      update only comparison facts, not fixed Tobari authority, without Product
      Owner review.

## Implement domain and application

- [ ] Add typed wait ID, immutable secret-free wait record, exact
      WorkspaceManifestID/WorkspaceID/AttachmentID-or-epoch/original-normalized-
      effect identity, closed result enum, expiry, attempt, consumption, and
      validation invariants.
- [ ] Add a read-only application intent/use case and minimal record/policy
      observation ports with one context and deadline.
- [ ] Prove the application layer has no mutation, OPA, Docker, filesystem,
      process, general network, terminal, request-replay, Workspace
      reconciliation, or cluster-reconciliation port.
- [ ] Preserve active permission-policy revision consistency and fail closed on staged,
      in-flight, failed, canceled, stale, default-deny, or contradictory policy
      state by keeping it nonterminal; reuse the policy authority's evaluator
      for exact/path-template Allow and explicit Deny precedence.
- [ ] Prove Manifest mutation, entry, cluster up, read-only status, session
      defaults, and creation defaults perform zero implicit learned-permission
      copy/reset/allow/deny/Apply/reconciliation.
- [ ] Prove `manifest create --copy-from` creates a fresh independent
      WorkspaceManifestID without copying/remapping learned permissions,
      candidates, waits, attachment authority, Apply receipts, or policy
      selection, and without permission reconciliation or lineage lookup.
- [ ] Prove Runtime source copy and Runtime build/delete/prune/restore leave
      learned permissions, candidates, waits, and policy selection unchanged;
      no Runtime identity, journal, plan, receipt, or lifecycle timestamp enters
      permission scope or authority.

## Implement Gateway and infrastructure

- [ ] Add final schema-2 fixed/generated denial and audit fields for supported
      generic ordinary HTTP/HTTPS denials, using `workspace_manifest_id` and `workspace_id`,
      rejecting legacy identity fields, and keeping request payloads, queries,
      headers, and credentials absent.
- [ ] Keep canonical and embedded Gateway sources manifest- and byte-equal.
- [ ] Add the bounded registry to the existing trusted-host attachment owner,
      receiving secret-free records through the existing bounded control/audit
      seam without persisted state, daemon, Workspace file, or second policy
      authority.
- [ ] Add the private attachment-local socket, ownership binding, bounded
      frames, deadlines, 8-live-wait ceiling, one-active-client and 3-attempt
      rule, reconnect without consumption, terminal consumption, coalesced
      observations, uniform non-enumerating faults, cancellation, and teardown.
- [ ] Add consistent learned-state/active-revision observation behind the
      application port; do not expose policy data to the helper.
- [ ] Package and mount a dedicated read-only `tobari-permission` helper without
      widening `tobari-expose` or the root CLI.

## Implement CLI and presentation

- [ ] Add a separate Program catalog for `tobari-permission wait`, exact typed
      argv, help, stable `policy.permission-wait` capability,
      RoleUtility/EffectRead declaration, required single `InputValueText`,
      exact regex/min/max bounds, no completion/reference/producer, omission/
      default behavior, faults, and recovery metadata.
- [ ] Apply nested-output reference producer/consumer rules through the
      completed WP08 Catalog-wide invariant only; add no Runtime- or permission-
      local walker/validator and keep `permission_wait_id` non-reference.
- [ ] Emit exactly `Allow`, `Deny`, or `Expired` in text and the closed schema-1
      enum in JSON; emit no public pending/progress, evidence, revision, rule,
      scope, candidate, or reviewer data.
- [ ] Preserve child-owned stdin, signals, resize, alternate screen, and exit
      status; add no reserved key, injected output, or same-terminal UI.
- [ ] Keep original-request execution entirely absent from helper inputs,
      domain records, ports, implementation, output, and recovery.

## Test and migrate

- [ ] Add domain, application, Gateway, helper catalog, output, fault,
      hostile-text, PTY, infrastructure, Docker integration, concurrency,
      expiry, 3-attempt/reconnect/consumption, stale-state, reset/Apply race,
      exact/path-template canonical evaluation, explicit-Deny-only, and
      component-mismatch tests.
- [ ] Add negative canaries for policy writes, candidate/reference exposure,
      list/status/pending/poll APIs, secrets, request retention/replay, raw
      terminal control, legacy Context/project identities,
      Manifest-name/revision/generation authority, recommended-draft authority,
      and cross-Workspace/cross-attachment lookup.
- [ ] Prove baseline default-deny, candidate disappearance/compaction, reset
      without explicit disposition, staged/applying/failed/canceled Apply, and
      ambiguous/contradictory observation never fabricate `Deny` or `Allow`.
- [ ] Add copy/Runtime-retirement negative canaries: Manifest copy does not copy/remap any
      permission or wait side state, and Runtime copy/build/delete/prune/restore
      does not copy/reset/delete/rewrite/approve it or infer use from lifecycle
      evidence.
- [ ] Add a typed presentation-independent fixture and answer key covering
      Allow, Deny, Expired, staged, failed Apply, stale revision, cancellation,
      empty scope, hostile evidence, and deliberate fresh retry.
- [ ] Record routine-success external processing as zero and prove the agent
      does not treat `Allow` as authority or auto-retry permission.
- [ ] Prove ADR 0079 migration refuses live attachments, no persisted wait-state
      migration exists, and post-migration/re-entry waits are freshly issued;
      verify ADR 0079's
      explicit predecessor migration retains learned-rule and retained-denial
      stable ID bytes under WorkspaceManifestID/WorkspaceID, rederives
      candidates, excludes wait/attachment state, and makes mixed component
      versions fail closed.

## Document and integrate

- [ ] Update governing theses if necessary, plus product, architecture,
      security, harness, authentication, external API, readiness, README, and
      the accepted ADR in the same implementation change.
- [ ] Update catalog-derived help/architecture data and capability, schema, and
      claim ledgers; review generated and dependency diffs.
- [ ] Reconcile identity/migration order with ADR 0079 first, then exact-
      rule and generated-file overlap with first-public-release-core,
      policy-compaction-retirement, legacy context-capability-envelope evidence,
      and release/profile work before merging.
- [ ] Coordinate, without making them implementation prerequisites, any
      Catalog/schema-ledger/generated-file overlap with the design-fixed but
      not-yet-started Runtime Retirement packet; treat ADR 0079 copy semantics
      as already integrated durable authority.
- [ ] Keep `tobari-permission` in the standard release Catalog/assets unless an
      accepted WP04 contract has separately changed that composition.
- [ ] Keep repository documentation in English and fixtures synthetic,
      deterministic, licensed, and secret-free.

## Verify and hand off

- [ ] Focused Go, Gateway, policy, helper, PTY, Docker, and race tests pass.
- [ ] `task check:fast` passes.
- [ ] `task security` passes.
- [ ] `task check` passes as the one completion gate.
- [ ] `task public:check` and `task release:check` pass when consumed by public
      or release work.
- [ ] Review `git diff`, generated/dependency changes, and public-boundary
      canaries; preserve unrelated worktree changes.
- [ ] Promote durable conclusions, attach evidence to the governing contracts,
      remove this temporary packet, and only then mark the implementation work
      complete.
- [ ] Commit the eventual implementation only after all required evidence and
      gates pass, then notify the root/control thread with completion evidence.
