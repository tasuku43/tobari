# Work Tasks: Give physical-host loopback an honest private authority

Product design tasks marked complete below were accepted and fixed by the
Product Owner on 2026-08-23. Every implementation, test, durable-documentation,
generated, release, gate, and handoff task remains incomplete unless checked.
Implementation was authorized on 2026-08-24 from exact integrated baseline
`97dd314bf00f152d1b1a127089354afd63eacd0c`; Accepted/Fix is not completion.

## Mandatory implementation-entry re-observation gate

- [x] Confirm Workspace Manifest and one-time-copy contracts were promoted to
      [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md)
      and current durable contracts by `07535a9` and `428812f`; deleted
      temporary packet files are not authority.
- [x] Continue the unchanged dependency order through WP08 -> WP03 -> WP04,
      then identify the integrated revision. Do not modify any predecessor's
      overlapping production, schema, Catalog, migration, CA/DNS, policy,
      build-profile, Runtime, or test files in advance.
- [x] Record the exact post-WP-04 `HEAD` and a clean or fully explained `git
      status`; preserve all unrelated user/concurrent changes.
- [x] Reread `AGENTS.md`, `docs/00_theses.md` through `docs/04_harness.md`,
      relevant `docs/07` through `docs/09`, ADR 0079, current durable contracts,
      and WP08/03/04 completion evidence in governing order.
- [x] Reinspect README, Catalog plus human/agent help, Workspace Manifest and
      Workspace domain/application/infrastructure code, Host Loopback route/
      grant/principal, Gateway DNS/TLS, OPA policy/data, CA ownership,
      migration/schema fixtures, tests, and recent related commits.
- [x] Run only bounded read-only help and safe fresh temporary-state
      observations from the integrated source; record binary source identity
      and reject stale-binary evidence.
- [x] Compare every final identity, lifetime, schema, migration, CA/DNS/policy,
      copy-isolation, and Runtime-separation fact to this packet. Update only
      the packet or obtain the required owner decision for discrepancies before
      starting WP 05 production code. Evidence: control accepted
      `WP05_IMPLEMENTATION_ENTRY_OBSERVED`; canonical APIs are
      `Runtime.withInteractiveAttachmentLock` and
      `Runtime.permissionSessionActive`, and a lock-held zero-owner predicate
      is the only missing WP07-owned migration seam.

## Decision and contracts

- [x] Product Owner accepted and fixed `host.tobari.internal` as the sole exact
      public/routable authority with hard cutover, no alias/redirect/
      translation, no suffix/wildcard authority, and an all-V1 terminal old-name
      guard whose later removal needs a separate ADR and negative evidence.
- [x] Product Owner fixed authority as the conjunction of exact hostname,
      port, method/path, WorkspaceManifestID/WorkspaceID principal, live
      Attachment Epoch, trusted-host grant, and route revalidation; ordinary
      external and Host Loopback policy remain mutually unusable.
- [x] Product Owner fixed `migrate apply` as sole cleanup owner after
      revalidating cluster stopped, zero live attachment, exact owner/schema,
      and exact old host. No maintenance command or implicit cleanup exists.
- [x] Product Owner fixed transient route/grant registries at schema V2 with
      new opaque IDs and direct hostname binding in route ID V2, while public
      `TOBARI_CAPABILITIES_JSON` remains schema V1 with only the hostname
      replaced. No AuthorityRevision or dual compatibility surface exists.
- [x] Product Owner fixed HTTPS pre-leaf terminal rejection, conditional exact
      old-leaf cleanup only through `migrate apply`, no broad cache deletion,
      and no root CA rotation.
- [x] Product Owner fixed the standard Runtime release floor to curl/libc,
      Python, applicable Go pure/cgo, and Node DNS+HTTP; Java/browser are
      optional unless actually included.
- [x] Product Owner fixed the public-boundary exception to exact
      `host.tobari.internal`, exact Host preservation through the relay, and no
      compatibility Host rewrite.
- [x] Product Owner fixed implementation order as completed Workspace
      Manifest/copy-contract promotion -> WP08 -> WP03 -> WP04 -> WP05. WP05
      consumes the completed actual HEAD and never
      anticipates predecessor production contracts.
- [x] Confirm ADR 0079's accepted copy operations carry no attachment, route,
      grant, applied/failure/observed/auth/learned-permission/current-selection
      state and expose no lineage/`copied_from`; confirm WP 03 leaves hostname
      selection independent and Host Loopback outside Runtime lifecycle
      authority. Do not implement either packet here.
- [x] Revise ADR 0049 through accepted ADR 0083 with the exact
      `host.tobari.internal` decision, source dates, rejected alternatives,
      non-alias migration, CA disposition, and reconsideration triggers.
- [x] Propagate the accepted consequence through theses, product, architecture,
      security, harness, readiness, README, threat model, and applicable
      authentication/external-I/O contracts. No threat-model, authentication,
      or external-API text named the old authority or required a new boundary.
- [ ] Coordinate the integration point with first-public-V1 core and artifact
      packets so no published artifact teaches `host.tobari.test`.

## Domain and application

- [ ] Replace the canonical hostname and URL template; keep exact HTTP,
      non-privileged-port, Workspace-audience, and attachment-lifetime
      invariants.
- [ ] Emit public capability schema V1 and transient route/grant registry schema
      V2. Bind new route/grant identity to exact WorkspaceManifestID,
      WorkspaceID, Attachment Epoch, canonical hostname, and effect; issue new
      opaque IDs and add no AuthorityRevision, Context/project alias, or
      Manifest revision dimension.
- [ ] Define the bounded retired-authority classification without adding a
      routable alias, public destination kind, command, or reference kind; keep
      it terminal for all V1.
- [ ] Make policy candidate/review/report interpretation surface only the new
      Host Loopback authority and reject all stale old references unchanged.
- [ ] Prove ordinary external rules cannot decide Host Loopback and attachment
      grants cannot decide ordinary or sibling `.internal` traffic.
- [ ] Prove Host Loopback and Attachment Grant stay outside Manifest
      desired/applied/observed/failure state; publishing a Manifest revision
      cannot touch or widen an active grant, and a new epoch inherits none.
- [ ] Reject presentation-only recommended Manifest drafts and absent
      Manifest state as route/grant authority; prove status/list/show/doctor
      create or reconcile nothing and explicit Workspace entry is the only
      attachment establishment boundary.

## Infrastructure, Gateway, DNS, and policy

- [ ] Integrate with the existing exact `migrate apply`: retain legacy UUID
      bytes as WorkspaceManifestID/WorkspaceID, revalidate cluster stopped and
      zero live attachment, and refuse mixed, malformed, wrong-owner,
      wrong-schema, wrong-host, symlinked, or unknown predecessor state before
      publication.
- [ ] Add the smallest canonical lock-held zero-live-owner predicate beside
      WP07's `Runtime.withInteractiveAttachmentLock` and
      `Runtime.permissionSessionActive`; Host Loopback code must not read or
      migrate permission-ingestion endpoint, nonce, lease, ACK, wait registry,
      or Gateway-only transport/profile fields.
- [ ] Hold locks in exact order `lifecycle -> interactive-attachment ->
      host-loopback`, retaining the interactive lock from zero-owner proof
      through schema-V1 route/grant replacement. Add a deterministic competing-
      entry canary proving normal entry cannot publish a session in that gap.
- [ ] Atomically replace only exact owner/schema/`host.tobari.test` transient
      route/grant registries with empty schema-V2 registries. Never translate
      route, grant, candidate, relay, Manifest, AppliedEntry, learned
      permission, or attachment authority, and add no maintenance command.
- [ ] Prove attachment startup, status/list/show/doctor, and `cluster up` do not
      clean or rewrite predecessor Host Loopback state.
- [ ] Require fresh attachment epoch, route, denial, review, and grant after
      cutover; never translate an old route, grant, relay token, or candidate.
- [ ] Switch Gateway route/grant parsing and exact request classification to
      the new host while preserving policy-before-relay order and original Host
      forwarding unchanged to physical-host `127.0.0.1`.
- [ ] Add the old-name terminal guard before ordinary external classification;
      emit fixed recovery and prove zero external-DNS/upstream/Broker/relay/
      retry work; the bounded internal synthetic-DNS response may still route
      the request to that guard.
- [ ] Preserve non-recursive synthetic DNS, exact private-ceiling exception,
      and infrastructure-only `host.docker.internal`/`host-gateway` transport.
- [ ] Preserve HTTP-only behavior and Host/SNI consistency; prove TLS to new,
      old, and near-match names is terminal before leaf generation and never
      reaches the host relay.
- [ ] Record Gateway CA digest before and after the cutover journey and prove no
      root/trust-bundle rotation. If and only if an exact old-host leaf entry is
      observed, have `migrate apply` remove only its owner-verified entry; prove
      cache material grants no authority and broad deletion never occurs.
- [ ] During integration review, keep Runtime delete/prune/restore/build
      protection and usage evaluation independent: Host Loopback transient
      files and attachment activity are not Runtime history, snapshot, image
      material, `last_used`, or a new protection/reference edge. Leave WP 03
      implementation and its verification in WP 03.

## CLI, presentation, schema, and migration

- [ ] Update Catalog-derived human/JSON descriptions and fixtures with ADR 0079's
      Manifest and Workspace identity. Host Loopback adds no command or host
      selector and does not alter the existing review role/effect/reference
      flow.
- [ ] Keep public/helper-visible capability, candidate, denial, review,
      principal, and audit contracts at their final V1 shapes while transient
      route/grant registries alone use V2; present Manifest and expose
      `workspace_manifest`, `workspace_manifest_id`, `workspace_id`, and
      subordinate `project_root`. Preserve frozen private compatibility wire
      spellings `context`, `context_id`, and `project_id` with their existing
      Workspace Manifest/Workspace meaning; do not expose them as public aliases.
- [ ] Add no `copied_from`, provenance, lineage, source Manifest/Runtime, or
      copy-derived Host Loopback field to hostname/route migration or its human,
      JSON, state, status, or audit output. Leave copy-output verification with
      ADR 0079's durable contract tests.
- [ ] Add one exact, secret-free, non-learnable old-name recovery message; do
      not emit a URL redirect or automatically replay the request, and keep the
      terminal classification for all V1.
- [x] Update README, agent guidance, examples, generated-site sources, and
      readiness scenarios to distinguish Workspace `localhost`, physical-host
      `host.tobari.internal`, and opposite-direction numeric exposure.
- [x] Keep current documentation uses of the old name confined to historical
      decision text, migration guidance, retirement-contract prose, and
      negative tests; the executable retired guard remains a later concern.
- [x] Update the governing public-boundary contract and scanner for exactly one
      product-owned synthetic URI host, `host.tobari.internal`; prove sibling
      and unrelated private hosts, TLS, malformed ports, casing, and userinfo
      remain rejected before publishing a complete URI literal.
- [ ] Document the ordered cutover: ADR 0079 migration precondition, stable-ID byte
      retention and durable principal rename, cluster stopped, zero live
      attachment, exact registry-V2 replacement, no attachment-state migration,
      post-migration `cluster up`, fresh Workspace entry/Attachment Epoch,
      fresh review, rollback, conditional exact leaf cleanup, and no CA
      rotation.

## Tests and observations

- [ ] Add domain unit tests for public capability schema V1, transient registry
      schema V2, exact hostname,
      WorkspaceManifestID/WorkspaceID/epoch route/grant identity,
      trailing-dot/case/lookalike/sibling rejection, and strict current-main
      predecessor isolation.
- [ ] Add application/CLI contract tests for new-host candidates, old-host
      non-learnability, stale opaque references, exact recovery, human/JSON
      output, and unchanged empty/absent semantics.
- [ ] Add infrastructure tests for exact predecessor cleanup, live-owner
      refusal, stopped-cluster/zero-live-attachment gates, owner/schema/old-host
      matching, stable UUID-byte migration, atomic empty registry-V2
      replacement, zero attachment-state translation, no implicit cleanup,
      required `cluster up`, fresh epoch/review, concurrent/borrowed
      attachments, crash reconciliation, token secrecy, exact reviewed target
      dial, and teardown.
- [ ] Add Gateway/OPA call-count canaries for new allow/deny and old, sibling,
      wildcard, malformed, TLS, SNI-mismatch, stale, and post-detach cases;
      retired/denied cases perform zero external-DNS/upstream/Broker/relay/
      retry work while internal synthetic DNS remains available.
- [ ] Run and record the standard Runtime resolver/client matrix for
      curl/libc-getaddrinfo, Python, applicable Go pure/cgo, and Node DNS+HTTP.
      Prove synthetic DNS traversal, zero external lookup, and exact Host
      preservation for every gated path. Observe Java/browser only if present.
- [ ] Run supported Docker Desktop and Colima journeys. Record Podman and
      OrbStack only as non-supported comparative observations unless runtime
      scope changes through a separate decision.
- [ ] Replay agent readiness with zero undeclared parsing, source inspection,
      exploratory calls, or name-notation decoding.
- [ ] Reuse ADR 0079's Docker migration evidence and add the fixed stopped-cluster/
      zero-live-attachment and exact owner/schema/old-host evidence without
      resolving its previous-revision retention or Git fallback questions.

## Gates and handoff

- [ ] Run focused Go/Gateway/OPA tests during implementation without treating
      them as the final completion decision.
- [ ] Run the clean `task integration:test` Host Loopback journey with only
      synthetic local fixtures and verify owner cleanup.
- [ ] Run `task check`, `task security`, `task public:check`, and
      `task release:check` on the integrated release candidate.
- [ ] Record exact commands, revision, client/runtime versions, CA digest, and
      results in `context.md`; record no secrets or private endpoints.
- [ ] Promote durable conclusions, remove superseded temporary material, mark
      the implementation complete only after every acceptance criterion has
      evidence, and delete this temporary packet in the closing handoff.
- [ ] After an actual implementation completion commit, notify control thread
      `01a02c51-885b-7b80-a66f-05850f48ba4d` with
      `WP05_IMPLEMENTATION_COMPLETE`; if the implementation-entry or execution
      gate is genuinely blocked, notify it with `WP05_BLOCKED`. Do not send
      either signal for this design-only Fix update.
