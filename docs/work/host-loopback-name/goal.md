# Work Goal: Give physical-host loopback an honest private authority

- Status: Accepted
- Planning state: Fix
- Retention: temporary
- Owner: Tobari maintainers
- Target: pre-public V1
- Governing contracts: [theses](../../00_theses.md),
  [product](../../01_product_contract.md),
  [architecture](../../02_architecture.md),
  [security](../../03_security_model.md), [harness](../../04_harness.md),
  [ADR 0081](../../decisions/0081-observe-reviewed-permission-from-an-attached-workspace.md),
  [ADR 0082](../../decisions/0082-release-and-research-build-surfaces.md),
  [ADR 0083](../../decisions/0083-name-the-physical-host-loopback-authority.md), and
  [ADR 0084](../../decisions/0084-separate-workspace-templates-contexts-and-policy-memory.md)
- Review/delete trigger: delete after implementation, durable propagation, and all completion gates

## Outcome

Every interactive Workspace can reach plain HTTP on the physical host's IPv4
loopback through exact authority `host.tobari.internal:{port}`. The capability
is owned by the canonical interactive attachment session. Its authority is the
conjunction of exact hostname, non-privileged port, method/path, trusted
ContextID/WorkspaceID principal, live Attachment Epoch, exact reviewed grant,
and fresh route validation. `.internal`, DNS success, Template identity, and
Policy Memory grant nothing by themselves.

Exact retired `host.tobari.test` remains terminal and non-learnable throughout
V1. It has no alias, redirect, translation, ordinary-policy fallback, external
DNS, Broker, upstream, relay, or retry path.

## Non-goals

- HTTPS, raw TCP, UDP, privileged ports, LAN access, service discovery, or host Docker control.
- Changing Workspace `localhost`, opposite-direction service exposure, or infrastructure-only `host.docker.internal`.
- Adding a command, public resource, selector, reference kind, wildcard, suffix authority, or AuthorityRevision.
- Putting Host Loopback state in Workspace Template desired/applied state, Context Policy Memory, Workspace AppliedEntry, observation, or Runtime protection.
- Reading, translating, deleting, or migrating predecessor route/grant/cache bytes. ADR 0084's final-only clean break owns their refusal.
- Changing frozen private `context`, `context_id`, or `project_id` wire tokens; their values are final ContextID, WorkspaceID, and Context presentation.
- Changing WP07 permission ingestion, wait authority, service exposure, research authentication, or WP03 Runtime mechanisms.

## Acceptance criteria

- [ ] `host.tobari.internal` is the sole routable Host Loopback hostname in capability, route, grant, denial, review, policy, audit, relay, docs, and supported tests.
- [ ] `host.tobari.test` is rejected immediately after normalized HTTP authority and before principal, credentials, registry, OPA, permission wait, DNS/upstream, or relay work.
- [ ] Public `TOBARI_CAPABILITIES_JSON` remains schema V1; private route/grant registries are strict schema V2 and issue fresh hostname-bound opaque IDs.
- [ ] Frozen private keys carry ContextID/WorkspaceID/Context presentation only. Template identity and Policy Memory never enter attachment authority.
- [ ] Exact current Host is preserved while the authenticated relay dials physical `127.0.0.1`; sibling and wildcard names receive no Host Loopback authority.
- [ ] Current, retired, malformed, absent-SNI, ECH-unobservable, and mismatched Host Loopback TLS are terminal before leaf creation/cache insertion and before HTTP/upstream hooks. The root CA is not rotated and cache bytes never authorize.
- [ ] Clean final absence creates only schema-V2 registries. Predecessor Host Loopback presence blocks final initialization without decoding or mutation; no command performs implicit cleanup.
- [ ] Release coverage proves synthetic DNS, zero external lookup, and exact Host for standard Runtime curl/libc, Python, applicable Go pure/cgo, and Node clients.
- [ ] README, durable contracts, agent guidance, source snapshots, and release/public guards agree; old-name occurrences are only retirement/history/negative evidence.
- [ ] Focused suites, supported isolated Docker canaries, `task check`, `task security`, `task public:check`, and `task release:check` pass.

## Integration state

Implementation resumed from clean exact integrated HEAD
`583d4e1e32b74107b6347b8addd622c44e6fb48e`. It already contains WP03,
WP04, WP07, accepted WP05 durable concern `0bbd9deb424814ab92eed0b816e2c565e4b8f6d3`,
and final WP11 authority. WP05 consumes final ContextID/WorkspaceID and
Workspace Template projection; it does not replay the accepted commit or
revive predecessor Manifest migration.

## Completion definition

Promote lasting evidence to durable contracts and tests, delete this temporary
packet, leave a clean committed worktree, and notify control thread
`01a02c51-885b-7b80-a66f-05850f48ba4d` with
`WP05_IMPLEMENTATION_COMPLETE`. Report `WP05_BLOCKED` only for a genuine
supported-flow P0/P1 blocker.
