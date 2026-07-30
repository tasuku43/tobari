# Context: Low-friction policy learning and compaction

## Verified facts

- Gateway denial audit records contain validated timestamp, request identity,
  host, method, path, reason, and status without credentials or bodies.
- `cluster denials` returns a bounded window and `policy apply` tests the host
  policy before deterministic OPA-only activation.
- The current initialized data model has broad allowed hosts/methods and
  explicit deny rules but no learned exact-rule collection.
- A reference-bound action may resolve one exact opaque ID in current owned
  state; it must not decode the ID or guess by labels.
- Docker-host file watching is not portable, so every confirmed learning
  mutation must complete through the existing activation boundary.

## User state machine

1. A request is denied and becomes bounded audit evidence.
2. A pending exact candidate is visible through machine or human discovery.
3. One candidate is explicitly approved and becomes an active exact learned rule.
4. Repeated active exact rules may produce a reviewable compaction candidate.
5. One compaction is explicitly approved and replaces only its exact source rules.
6. Historical denial evidence remains visible while active candidates recede.

## Constraints

- Candidate and compaction IDs are opaque, deterministic, kind-specific, and
  become stale when their owned source state disappears or changes.
- Exact learned rules may override a legacy explicit deny only because the user
  approved the exact denied effect.
- Compaction supports HTTPS path prefixes only; it never broadens host or method.
- Policy JSON is bounded, owner-only, regular, duplicate-key-free, and updated atomically.
- Tests exercise rule matching itself so unrelated broad legacy permissions
  cannot make a boundary canary pass accidentally.

## Unknowns

- A later retention policy may need a durable audit ledger beyond Docker's log window.
- Rule expiry, revocation, and manual annotations remain later capabilities.
