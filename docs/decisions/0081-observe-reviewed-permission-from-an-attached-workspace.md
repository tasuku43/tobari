# ADR 0081: Observe reviewed permission from an attached Workspace

- Status: Accepted
- Date: 2026-08-23
- Deciders: Tobari product owner and maintainers
- Scope: Product, CLI, architecture, security, Gateway denial output, attachment runtime, and harness
- Related: ADR 0024, ADR 0061, ADR 0073, ADR 0074, and ADR 0079
- Superseded by: None

## Context

A learnable Gateway denial already preserves the Workspace session and directs
the operator to the trusted-host Permission Inbox. Final Apply is the only
reviewed authority mutation, but the attached agent has no bounded way to
observe that Apply made a fresh retry reasonable. Returning to the agent to
repeat that fact is routine coordination friction.

The missing capability is observation, not another approval path. Exposing a
candidate, public pending collection, agent proposal, auto approval, held
request, or terminal injection would either move policy authority into the
Workspace or violate the child-owned TTY boundary.

ADR 0079 fixes two identity layers that this change must not collapse. Public
Workspace and Manifest surfaces use `workspace_id` and
`workspace_manifest_id`, while the principal registry, Gateway-to-OPA input,
OPA and learned-policy wire, and Host Loopback route/grant registries retain
their frozen schema-v1 `project_id`, `context_id`, and `context` tokens.

## Decision

Add one standard attachment-local Program:

```text
tobari-permission wait --id pwt_<32-lowercase-hex> [--format text|json]
```

It is a Catalog-owned `RoleUtility`, `EffectRead` capability with stable ID
`policy.permission-wait`. The required ID is a single text correlation value,
not a durable resource, opaque reference, completion subject, or discoverable
producer. It is exactly 36 bytes and matches `^pwt_[0-9a-f]{32}$`.

The command returns only `Allow`, `Deny`, or `Expired` (JSON schema 1 uses the
lower-case equivalents). `Allow` is retry-readiness evidence only. The helper
never reconstructs or retries the denied request, and Gateway independently
authorizes every fresh request.

### Canonical owner and join

The canonical owner is the interactive Tobari Workspace attachment session
created before the attached child. Its AttachmentID/epoch is session authority;
Host Loopback is one capability bound to that session. Exactly one owning epoch
may be active for `(WorkspaceManifestID, WorkspaceID)`. Concurrent borrower
entries share that owner and epoch without extending them. Ambiguous, duplicate,
stale, malformed, drifted, or concurrently replaced owner state fails closed.
Workspace-service exposure controller attachments remain distinct and are
ineligible for permission-wait ownership.

The host-owned attachment lifecycle gains a bounded private canonical
interactive-session registry. Its current record binds canonical
WorkspaceManifestID and WorkspaceID, AttachmentID/epoch, owner process,
unpredictable process-instance nonce, owner-only Unix ingestion socket, and a
renewable bounded lease. The PID is diagnostic correlation, never sufficient
join authority. A renewal advances an explicit lease issue time and cannot
revive an expired lease.
Gateway may join its already-authenticated frozen schema-v1 principal only to
one exact canonical session record. A Host Loopback route is an optional
subordinate capability on that session and is never permission-wait join
authority. Gateway never
joins by log order, display name, project root, container name, child PID,
request timing, or caller data.

For an eligible ordinary external HTTP or HTTPS denial, Gateway creates 128
bits of CSPRNG correlation data and submits one immutable, secret-free schema-2
wait record to the matched attachment owner. The record binds the denial
correlation, frozen principal identity, canonical Manifest and Workspace IDs,
AttachmentID/epoch, exact normalized ordinary HTTP effect, creation and expiry,
and wait ID. Gateway publishes resume guidance only after the owner accepts and
acknowledges that exact record over the bounded authenticated ingestion channel.
Failure or timeout omits resume fields. Possession of the wait ID alone grants
nothing; the child can use it only through the owning attachment's private
read-only helper socket.

An ingestion-listener, heartbeat, or lease-renewal failure uses the same
fail-closed shutdown: close transport, invalidate active and future waits, then
remove only the exact unchanged owner record with a bounded context independent
of child cancellation. Drifted authority is retained for reconciliation and
reported as cleanup evidence; it is never deleted by epoch alone.

### Observation semantics

The owner observes the canonical live OPA evaluator for the exact original
effect. It returns `Allow` when active policy authorizes that effect, including
through a reviewed conservative path template. It returns `Deny` only for an
effective explicit deny. Default deny, staged/applying/failed/canceled Apply,
candidate disappearance, reset without an explicit disposition, revision lag,
or ambiguous state is nonterminal and may end only in a later authoritative
result or lease expiry. The helper does not implement rule matching or policy
precedence.

V1 admits only reviewable ordinary external HTTP and HTTPS effects. Host
Loopback and protocol-derived GraphQL, MCP, AWS, Kubernetes, Git, and OCI
identities are excluded.

### Bounds and failure contract

One record lives for at most 15 minutes and never beyond its attachment. Each
attachment admits at most eight live waits. Each ID admits one active
connection and at most three connection attempts in its lease. Request and
response frames are bounded to 4 KiB and 1 KiB. Observation uses the bounded
1, 2, 4, then at-most-5-second schedule. Evidence may lower these ceilings;
raising one requires Product Owner review.

Cancellation or transport loss counts as an attempt but does not consume the
record or fabricate a terminal result. A terminal result consumes it. Unknown,
consumed, cross-attachment, and attempt-exhausted IDs use one non-enumerating
fault shape. Attachment teardown is a fault; only validated lease expiry
returns `Expired`.

### Schema boundary

Gateway denial projection and the new retained wait/audit record hard-cut to
schema 2 with `workspace_manifest_id`, `workspace_id`, and the optional resume
handoff. They accept no schema-1 alias or dual shape. Helper JSON remains its
independent schema 1.

The frozen schema-v1 principal registry, Gateway-to-OPA input, OPA and
persisted learned-policy wire, and Host Loopback route/grant records remain
byte- and schema-stable with `context_id`, `project_id`, and `context`. A
single reviewed projection converts the already-authenticated frozen principal
into the schema-2 denial/wait record; it does not widen sibling readers.

## Consequences

- Trusted-host Permission Inbox review and final Apply remain the only policy
  mutation authority. The Workspace receives no proposal, decision, scope,
  rule, revision, rationale, or retry capability.
- Wait records are bounded attachment memory. There is no daemon, persistent
  store, Workspace file, Manifest desired/applied field, migration, or second
  policy authority.
- Manifest copy, Runtime lifecycle, Workspace reconciliation, cluster
  reconciliation, and session defaults neither copy nor mutate permission or
  wait state.
- The child TTY remains Docker/child-owned. Permission review stays in a
  separate trusted-host terminal and no same-terminal overlay or injection is
  added.
- The Catalog's global recursive output/reference derivation remains the sole
  traversal. `permission_wait_id` deliberately creates no reference edge.
- WP09 may consume the canonical interactive-session identity and owner
  lifecycle, but its service-exposure attachment IDs and authority remain
  separate.

## Mechanical enforcement

- Domain and application tests fix ID syntax, immutable record identity,
  terminal results, expiry, attempts, consumption, and zero mutation ports.
- Registry and transport tests fix one owner, borrower sharing, exact principal
  join, nonce/endpoint binding, acknowledgment-before-publication, bounded
  frames/concurrency, non-enumerating faults, and teardown.
- Gateway tests cover positive and negative frozen-v1-to-schema-2 projection,
  unsupported denial omission, CSPRNG IDs, secret exclusion, and canonical /
  embedded source equality. Byte/schema canaries hold every frozen sibling wire
  unchanged.
- OPA-observer tests use the canonical evaluator and cover exact/template
  Allow, explicit Deny, default deny, stale revision, and ambiguous state.
- Program/Catalog tests cover the exact grammar, generic length bounds, no
  ReferenceKind/producer/completion, closed output schema, hardcoded Program,
  helper packaging, read-only mount, cancellation, and hostile output.
- Integration proves deny, separate trusted-host Apply, wait result, deliberate
  fresh retry, zero automatic retry, zero policy mutation from the Workspace,
  and default-cluster non-interference outside an explicitly owned disposable
  test environment.

## Compatibility and migration

This pre-public surface has no alias. Unsupported or mismatched components omit
resume or return a typed unavailable fault; they do not infer a candidate or
read policy from the Workspace. No wait state is migrated. ADR 0079 migration
requires zero live attachments, so post-migration waits are issued only after a
fresh attachment. Frozen schema-v1 sibling wires remain unchanged.

## Validation

- focused domain, application, infrastructure, Gateway, OPA, CLI, packaging,
  race, and integration tests
- `task check:fast`
- `task security`
- `task check`
- `task public:check`
- `task release:check`
- the permission-resume agent-readiness journey with zero undeclared external
  processing
