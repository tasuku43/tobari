# Work Tasks: Expose one Workspace HTTP service to the trusted host

- Goal: [goal.md](goal.md)
- Plan: [plan.md](plan.md)

## Understand

- [x] Read governing theses, product, architecture, security, and harness
      sections. Evidence: current `docs/00_theses.md` through
      `docs/04_harness.md` were reviewed in numeric order during this design
      sequence on 2026-08-21.
- [x] Observe the current browser channel, runtime asset, Host Loopback,
      attachment, and direct-command behavior. Evidence recorded in
      `context.md`.
- [x] Record verified facts, unresolved protocol and compatibility questions,
      and thesis evidence.
- [x] Confirm the public outcome and non-goals with the product owner.

## Decide

- [x] Compare host command, full in-Workspace CLI, Docker publishing, generic
      RPC, and dedicated helper approaches. Evidence: the product owner chose
      the dedicated in-Workspace helper and purpose-specific browser-channel
      pattern on 2026-08-21.
- [x] Choose explicit host approval rather than automatic exposure. Evidence:
      the product owner selected `Allow once` through the same Trusted Host
      Review experience as Permission Inbox.
- [x] Choose attachment ownership, random host port, exact numeric IPv4
      loopback authority, no browser auto-open, and no active health checks.
      Evidence: random `.localhost` passed Vite, Next.js, and Storybook but
      Jupyter Server 2.20.0 rejected it by default; all four accept exact
      `127.0.0.1:<random-port>` without rewriting or configuration widening.
- [x] Resolve the `trusted-host-review-switch` dependency. Evidence: ADR 0073
      rejected inline review; the product owner selected a separate
      trusted-host `tobari review` inbox on 2026-08-21.
- [x] Keep Permission and Service review semantics distinct. Permission
      requests retain staged Apply; Service requests use immediate `Allow once`
      or `Deny` with attachment-only lifetime.
- [x] Confirm representative Vite, Next.js, Storybook, and Jupyter Host and
      Origin compatibility before finalizing the data-plane ADR. Evidence:
      pinned observations recorded in `context.md` on 2026-08-22.
- [x] Resolve one canonical program-scoped catalog and helper help contract
      without exposing the host CLI inside the Workspace. Evidence: one global
      program-aware Catalog validates cross-program reference flow while
      routing and help are filtered per executable.
- [x] Resolve the `stop` target-binding mode. Evidence: the product owner chose
      `tobari-expose stop <exposure-ref>` on 2026-08-22; creation and list
      produce that compact opaque reference and exact next command.
- [ ] Declare every operation's role, effect, target, intent, impact, stable
      failures, output, and cancellation contract mechanically.
- [ ] Write and accept the service-exposure ADR and any required thesis revision
      before mechanism code.
- [x] Prototype and prove the live-owner host rendezvous, including concurrent
      attachments, stale/forged registry records, peer/nonce identity, owner
      exit during review, and safe registry cleanup. Evidence: 20 macOS runs
      and Linux amd64 cross-compilation recorded in `context.md`.
- [x] Obtain design approval. Evidence: the product owner accepted the complete
      service request, approval, relay, lifetime, state, and compatibility
      contract on 2026-08-21.

## Implement

- [ ] Add failing domain state and operation-contract tests.
- [ ] Add failing canonical catalog, helper grammar, help, output, and exit-code
      tests.
- [ ] Add application use cases and smallest owned ports for submit, withdraw,
      list, approve, deny, stop, relay creation, and attachment cleanup.
- [ ] Add the binary-owned read-only helper asset and exact schema with real
      executable protocol tests.
- [ ] Add a separate non-TTY control process and unpredictable owner-only
      attachment socket.
- [ ] Add host-loopback listener and bounded HTTP authority validation.
- [ ] Add bidirectional HTTP and WebSocket relay with explicit concurrency,
      backpressure, half-close, timeout, cancellation, and shutdown behavior.
- [ ] Add fixed 502 behavior for an unavailable Workspace service.
- [ ] Add canonical service-request discovery and opaque-reference-bound
      `Allow once` and `Deny` actions, then compose them into the separate-host
      `tobari review` source selector without changing `policy review` or its
      Apply semantics.
- [ ] Add current-attachment list, idempotent repeat, and exact stop behavior.
- [ ] Factor browser and service transport or lifecycle code only where tests
      prove the same invariant; retain separate schemas and authority.
- [ ] Add hostile-input, replay, stale, cross-attachment, wrong-authority,
      response-smuggling, and zero-data-logging tests.
- [ ] Add representative pinned development-server compatibility fixtures or
      bounded runtime evidence.
- [ ] Update capability ledger, runtime asset manifest, durable documentation,
      ADRs, architecture site, README, and agent-readiness evidence.

## Verify

- [ ] Focused domain, application, catalog, protocol, relay, runtime asset, CLI,
      and Docker tests pass. Evidence:
- [ ] Invalid, denied, stale, hostile, and cross-attachment inputs perform zero
      listener or Workspace-service side effects. Evidence:
- [ ] Exact request and exposure references survive discovery and action
      unchanged. Evidence:
- [ ] HTTP, WebSocket, 502, concurrent-stream, stop, and attachment cleanup
      integration passes. Evidence:
- [ ] Pinned Vite, Next.js, Storybook, and Jupyter observations pass without
      Host, Origin, redirect, cookie, or content rewriting. Evidence:
- [ ] `task check` passes. Evidence:
- [ ] `task security` passes. Evidence:
- [ ] `task public:check` passes. Evidence:
- [ ] The agent-readiness journey needs one helper command, one host prefix,
      one explicit approval, and zero undeclared external-processing steps.
      Evidence:
- [ ] Generated diffs, public documentation, capability ledger, and repository
      status are understood. Evidence:

## Hand off

- [ ] Acceptance criteria have evidence.
- [ ] Goal status is changed to `Complete` only after all goal and task
      checkboxes are complete.
- [ ] Durable decisions are promoted out of the work packet.
- [ ] Temporary diagnostics, application bytes, browser history, and sensitive
      runtime observations are removed.
- [ ] Follow-up HTTP versions or protocols are explicit and do not block the
      accepted first slice.
- [ ] This temporary packet is removed in the completing implementation commit.
- [ ] Handoff summary explains outcome, authority, lifecycle, checks, and
      remaining compatibility limits.
