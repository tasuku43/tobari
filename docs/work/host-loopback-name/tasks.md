# Work Tasks: Give physical-host loopback an honest private authority

## Rebaseline and decision

- [x] Rebase the independent WP05 branch on exact final WP11 HEAD `583d4e1e32b74107b6347b8addd622c44e6fb48e` and verify clean ancestry including accepted `0bbd9deb`.
- [x] Re-read governing docs, ADR 0081/0082/0083/0084, packet, final Context/Workspace source, WP07 session owner, Gateway, DNS, OPA, tests, and recent commits.
- [x] Replace predecessor Manifest/migration vocabulary with final ContextID/WorkspaceID/Workspace Template and ADR 0084 clean-break semantics.
- [x] Preserve Product Owner-fixed hostname, retired guard, schema split, HTTP-only behavior, CA disposition, Host preservation, and dependency separation.
- [x] Fix the finite A-G evaluator in `plan.md` at 18 RED / 1 N/A before any
      resumed production edit. Every later implementation change must reduce
      only those named cells; completion requires 0 RED.

## Domain and coherent consumers

- [ ] Change exact hostname and URL while retaining public capability schema V1.
- [ ] Advance private route/grant registry to schema V2 and issue fresh exact-hostname-bound IDs; add no AuthorityRevision.
- [ ] Use final Context/Workspace Go vocabulary while preserving frozen private JSON keys `context`, `context_id`, `project_id`.
- [ ] Update final entry/attachment, policy candidate/review, Gateway reader, embedded/helper snapshots, and all mechanically necessary consumers atomically.
- [ ] Add exact current host/scheme/port enforcement to the OPA Host Loopback branch and sibling/ordinary-policy isolation tests.

## Terminal retirement and TLS

- [ ] Reject exact retired HTTP immediately after normalized authority with fixed secret-free `retired_host_loopback_authority` and zero downstream call counts.
- [ ] Exclude both current and retired Host Loopback names from permission-resume projection.
- [ ] Add pinned mitmproxy ClientHello/start-client terminal path for current, retired, malformed/absent SNI, ECH-unobservable, and relevant drift cases before leaf/cache insertion.
- [ ] Prove zero passthrough, upstream connect, HTTP hook, OPA, Broker, relay, retry, and grant mutation for terminal cases.
- [ ] Prove ordinary external and sibling `.internal` traffic cannot borrow Host Loopback grants and Host remains exact through allowed relay.

## Final-only state and separation

- [ ] Prove final clean absence creates schema-V2 stores and predecessor Host Loopback presence is rejected by existing non-decoding guard with zero mutation.
- [ ] Prove attachment startup/read-only commands/cluster operations do not translate or clean predecessor route/grant/cache state.
- [ ] Prove fresh epoch/route/review/grant and teardown; no Template mutation, Policy Memory, copied state, Runtime edge, or permission-ingestion/service-exposure field participates.
- [ ] Prove public capability V1 and private registry V2 are independent versioned contracts.

## Tests, evaluator, and handoff

- [ ] Run focused domain, infrastructure, Gateway, OPA, standard/research, race, source-snapshot, and public-guard tests.
- [ ] Run one isolated Docker evaluator for transparent HTTP, retired HTTP, TLS pre-leaf/cache, CA digest, teardown, and standard Runtime curl/libc, Python, applicable Go pure/cgo, and Node clients.
- [ ] Replay the Host Loopback agent-readiness journey with no undeclared parsing or source inspection.
- [ ] Run `task check`, `task security`, `task public:check`, and `task release:check`.
- [ ] Promote durable evidence, audit positive/retired token use, delete this temporary packet, commit a clean final tree, and notify control with `WP05_IMPLEMENTATION_COMPLETE` plus exact HEAD.
