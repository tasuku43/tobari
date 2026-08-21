# Work Context: Explain every authority by scope, lifetime, owner, and precedence

## Current behavior

- For ordinary external traffic, principal/schema/Context validity precedes the
  Context destination ceiling and one complete method decision. Destination or
  method Deny is terminal before candidate, DNS, Broker, or upstream I/O.
- The ordinary evaluator then has one combined exact-deny tier containing
  trusted baseline Deny and remembered HTTP, GraphQL, or MCP exact Deny. The two
  sources have no semantic order within that tier, and both precede every
  positive source.
- Context-policy positive authority contains method Allow and baseline/native
  exact, template, GraphQL, or MCP grants. Only while still unresolved may a
  Guided remembered Allow or Advanced Rego decide the request, followed by the
  fail-closed/default-review result.
- Context snapshots retain immutable policy data and a `native_readiness`
  selection. Enabled readiness replaces historical readiness rules at aggregate
  generation with the installed binary's independently revisioned current
  compatibility set; Context ceilings still win.
- Learned Allow and exact Deny decisions bind Context plus project/Workspace
  principal and exact effect identity. They persist until an explicit reset.
- Advanced Rego is owner-authored Context policy. It may further constrain
  generic request input and participate only within Tobari-owned ceilings and
  learned-identity rules.
- Every interactive entry creates or borrows one active Attachment Epoch for
  Host Loopback. Host Loopback uses a separate evaluator branch: an active
  principal-owned route and epoch are required, then exact Attachment Deny,
  Attachment Allow, or attachment review decides the request. Ordinary Context
  destination/method ceilings, durable exact Deny, baseline/native authority,
  remembered Allow, and Advanced Rego are excluded from this branch.
- An Attachment Grant binds Context, Workspace principal, epoch, target port,
  and exact HTTP effect, applies Workspace-wide for that owning attachment, and
  disappears when its route owner exits. Gateway resolves the owner-bound route
  and grants before OPA, opens the authenticated bridge only after OPA Allow,
  and the host relay revalidates route, project, epoch, port, and Allow before
  dialing physical loopback.
- Attachment Grants are runtime-owned, absent from durable `policy rules`, and
  cannot become persistent learned rules, templates, arbitrary leases, raw TCP,
  or Docker authority.
- Missing or unresolved positive authority falls through to default deny or,
  only when all orthogonal invariants permit, an exact-review candidate.

## Relevant structure

- Entry point: Context creation/show, policy candidates/review/rules/allow/deny/reset,
  root Workspace entry, and Host Loopback discovery
- Domain rule: Context policy composition, learned-rule identity, review
  candidates, authority lifetime/kind, and Attachment Grant validation under
  `internal/domain/tobari`
- Application use case: policy discovery/mutation and project lifecycle under
  `internal/app/tobaricmd`
- Infrastructure boundary: aggregate policy generation, Gateway OPA document,
  policy addon, attachment registry/relay, and atomic activation under
  `internal/infra/dockerruntime` and embedded Gateway assets
- CLI catalog or presentation: policy and Context command specs, structured
  authority/lifetime fields, Permission Inbox, rules, and lifecycle help
- Existing tests and harness checks: context-policy composition, OPA/Gateway
  authorization-before-I/O, exact-Deny precedence, policy identity, attachment
  lease cleanup, hostile evidence projection, integration canaries, and
  security claim table

## Constraints

- Every supported HTTP/HTTPS effect crosses the one Gateway/OPA enforcement
  boundary; documentation cannot invent a second precedence implementation.
- Context destination/method ceilings and the combined exact-deny tier remain
  terminal for ordinary external traffic. They neither authorize nor deny the
  independently closed Host Loopback branch.
- Native readiness is trusted-binary data, never agent identity, observed
  candidate growth, provider metadata, or a mutable external profile.
- Learned rules are host-reviewed, Context/Workspace-bound, and exact or one
  reviewed safe path template; display labels never carry authority.
- Attachment identity and authority lifetime are typed. Matching host, port,
  method, path, prose, or list position cannot convert persistent and
  attachment-scoped decisions.
- Ordinary learned rules, baseline/native authority, Advanced Rego, and Host
  Loopback decisions cannot be converted or copied across their policy
  branches.
- Host Loopback has no direct Workspace-to-host route; Gateway and OPA decide
  before the authenticated host relay connects to physical IPv4 loopback.
- The standard profile retains Workspace-owned tool authentication and no
  host-managed credential projection.

## External facts

No external source is required. This packet consolidates existing Tobari-owned
contracts and the accepted product-owner decision from 2026-08-21.

## Unknowns

- [x] Trace the exact current ordinary and Host Loopback evaluator branches
      from typed input through OPA result and record each precedence edge.
      Evidence: implementation audit on 2026-08-21 found the separate aggregate
      clauses and Gateway/relay revalidation described above.
- [ ] Inventory the existing unit, policy, Gateway, integration, and security
      test that enforces each row; identify untested edges without assuming
      prose is evidence.
- [ ] Confirm whether Advanced Rego can produce positive authority in every
      documented case or should be described only as an additional constraint
      on generic input.
- [x] Confirm baseline deny versus exact learned deny ordering is semantically
      observable or only that both precede every positive decision. Evidence:
      both are members of one terminal exact-deny tier and have no semantic
      ordering relative to each other.
- [ ] Identify every public output that exposes `destination_kind`,
      `authority_lifetime`, epoch, or principal scope and verify the typed facts
      cannot be inferred from labels.

## Thesis evidence

- Repeated design decision or point of agent confusion: baseline, overlay,
  snapshot, learned rule, Attachment Grant, and Advanced policy appear as
  similarly weighted public nouns despite different owners and lifetimes.
- User outcome or friction observed in the minimal slice: users need to know
  what can never happen, what they remembered for this Workspace, and what ends
  with the current connection—not the implementation source that produced it.
- Code workaround or exception being considered: local UI simplification could
  hide or accidentally merge lifetime/precedence facts without a canonical
  model.
- Current thesis that resolves it, or proposed thesis revision: keep existing
  enforcement and explain it as three user layers plus one complete technical
  authority inventory. Host Loopback is an independently closed attachment
  exception, not a positive source underneath ordinary Context network policy.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: glossary, policy flow, authority/lifetime output descriptions,
  claim-to-enforcement map, and missing negative canaries.

## Reproduction or observation

```sh
rg -n 'destination ceiling|method Deny|exact Deny|native readiness|Attachment Grant|default deny' \
  docs/00_theses.md docs/01_product_contract.md docs/02_architecture.md \
  docs/03_security_model.md internal/domain/tobari internal/infra/dockerruntime
```

Observed 2026-08-21: all authority sources and core precedence claims exist,
but their scope/lifetime/owner facts are distributed and the routine user path
does not reduce them to one stable three-layer model.

## Security and public-boundary notes

- Assets and side effects involved: Context policy snapshot, aggregate policy,
  learned-rule state, Advanced policy, attachment route/grant registries,
  Gateway audit/candidate evidence, and public authority projections
- Credentials or confidential data involved: no new credential; existing
  authentication/cookie redaction and broker deny-before-resolution remain
- New dependencies, destinations, files, processes, or generated content: no
  dependency, runtime destination, or process; durable tables and deterministic
  test fixtures may be added
- External schema provenance, publication rights, and drift evidence: all
  policy and output schemas are Tobari-owned
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: unchanged; policy snapshots remain
  bounded reads and confirmed mutations retain current non-retryable contracts
- Publication and licensing concerns: synthetic local fixtures only

## Glossary

- **Context Access:** routine-user explanation of the immutable destination,
  method, source, and compatibility ceilings plus resulting routine traffic.
- **Remembered decision:** a host-reviewed persistent learned Allow or exact
  Deny bound to one Context and Workspace effect.
- **This-session host access:** routine-user explanation of exact Host Loopback
  decisions that end with the owning attachment.
- **Authority source:** one typed input capable of denying or allowing an
  otherwise valid effect at a declared precedence.
- **Precedence:** the deterministic ordering that ensures a lower authority
  cannot override a terminal ceiling or deny.
