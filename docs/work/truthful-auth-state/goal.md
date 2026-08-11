# Work Goal: Report authentication state and change truthfully

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: Project Theses 0, 3, 6, 7, and 9
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Codex
- Target: Explicit and Resumable UX program, authentication lane
- Related ADRs: ADR 0025

## Outcome

Auth status distinguishes provider configuration from each Workspace's actual
projection freshness, gives an exact action only when re-entry is genuinely
required, marks configured providers before a user can rotate them, and reports
no-op logout as no change rather than claiming credentials and handles were removed.

## Why now

Hands-on review found that every configured provider causes
`workspace_reentry_required` regardless of whether Workspace projections are
already current. The login selector does not mark configured providers, so a
human can accidentally rotate one without warning. Logging out an already
unconfigured provider left status unchanged but still claimed credential
removal, handle invalidation, and required re-entry. Cancellation also appeared
as a red retryable command failure.

## Non-goals

- Do not expose a primary secret, handle, vault path, OAuth state, or provider transcript.
- Do not add provider API calls, remote revocation, multiple accounts, or generic OAuth.
- Do not rewrite a running Workspace or grant policy permission.
- Do not infer projection freshness from labels, configured state, or process order.
- Do not change provider acquisition drivers in this packet.

## Acceptance criteria

- [x] Auth status owns a typed provider state and a separate Workspace
      activation state derived from authoritative stored/runtime revision facts.
- [x] `ready`, `workspace_reentry_required`, `not_applicable`, `unavailable`,
      and `unresolved` states have explicit invariants and fixtures.
- [x] A current Workspace projection can report Ready; configured provider
      state alone never forces re-entry.
- [x] Status gives the bound Context and exact next action only when the
      activation state justifies it.
- [x] The login selector visibly marks configured providers and explains that
      selecting one rotates the grant and revokes previous handles before login.
- [x] Logout of an absent provider is an explicit no-change result and does not
      claim revocation, file removal, or re-entry.
- [x] Real replacement/logout receipts describe only confirmed changed state;
      uncertain mutations remain non-retryable and reconcile through `auth status`.
- [x] Deliberate selector cancellation uses the shared neutral exit-11 presentation.
- [x] Unknown provider recovery leads to installed-provider discovery or exact
      help, not an unsupported import dead end.
- [x] JSON schema, human output, scoped agent help, selector, and no-secret
      canaries agree.
- [ ] `task check`, `task security`, `task authbroker:test`, and relevant
      integration tests pass. Security, Auth Broker, and integration pass; the
      full profile is blocked only by stale generated Pages schema tables in the
      explicitly excluded `docs/architecture-site/**` subtree.

## Governing documents

- Thesis: `docs/00_theses.md`, Theses 0, 3, 6, 7, and 9
- Product contract section: Auth commands, Context authentication, Output and
  exit contract, Side effects
- Architecture/security: Context-wide credential ownership, project-bound
  projection/reconciliation, mutation outcome contract
- Existing ADR: ADR 0025 for current provider plans; no new provider plan

## Completion definition

The work is complete when status and mutation receipts are evidence-backed,
no-op and current-projection cases are tested, secret boundaries remain intact,
required profiles pass, and this temporary packet is removed.
