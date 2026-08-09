# Plan: Policy hot activation

## Chosen direction

Preserve `policy review` as the read-only discovery surface for redirected and
machine callers. Its terminal workflow stages several exact Allow or Deny
choices and delegates one command-bound policy-decision-set mutation on Apply.
The existing single-reference actions remain available and compatible.

Replace routine OPA recreation with a complete revisioned bundle activation.
Prefer a Docker-managed bundle volume written and watched inside the Docker
host because it retains the three-service topology and removes reliance on host
bind-mount notification. Accept this mechanism only after a bounded real-Docker
experiment proves exact revision observation and prior-policy retention.

An authority-increasing change may keep serving the previous narrower policy
until the candidate is active. An authority-reducing or mixed change first
activates a complete deny-all transition bundle. It keeps that fence until the
candidate or the previous known-good revision is confirmed active.

## Alternatives

### Continue recreating OPA once per reviewed decision

Rejected because it preserves the latency that motivated the work.

### Stage several choices but call the existing action once per item

Rejected because each public action promises complete activation and success;
delaying those activations would make their mutation results false.

### Add a resident control-network bundle distributor immediately

Deferred. It follows OPA's management-bundle model cleanly but introduces a
fourth trusted service, image identity, resource contract, topology boundary,
release artifact, and supply-chain surface. Use it only if the Docker-managed
volume experiment cannot meet the portability and revision guarantees.

### Rely on the existing host bind mount and OPA `--watch`

Rejected because the current portable activation boundary exists specifically
because host-to-Docker file notification is not reliable enough.

### Apply multiple opaque candidate IDs through one public batch flag

Rejected because it would create a multi-target act outside the catalog's
single-reference binding contract. The terminal workflow instead acts on one
command-bound installation decision-set target and revalidates every staged
opaque ID as typed mutation content.

## Risks

- A filesystem notification may be lost, leaving OPA on the old revision.
  Exact bounded revision polling must turn this into a failed mutation, never a
  false success.
- A direct watched-file write may expose an incomplete archive. OPA must retain
  the prior active policy, and publication must produce a final observable
  update or use an atomic helper.
- A crash after an authority-reducing fence activates could leave the cluster
  intentionally unavailable. Durable reconciliation evidence must prefer
  fail-closed recovery over guessed authority.
- Multi-Context staged changes require rollback of more than one source file.
  The first slice is therefore bounded to one Context and exposes that limit in
  the selector, application validation, runtime defense, and public contract.
- Presentation can imply that a staged decision is already active. The UI must
  distinguish `staged`, `applied`, and `discarded` states.

## Verification

- Domain tests for staged-set bounds, duplicate/conflicting IDs, and result
  identity.
- Application tests for fresh-snapshot validation, one activation, zero-call
  cancellation, concurrency, and structured mutation outcomes.
- CLI selector and real-PTY tests for several staged decisions, summary, Apply,
  discard, and unchanged redirected/JSON behavior.
- Infrastructure tests for bundle publication argv, exact revision polling,
  no OPA recreation, fence/rollback ordering, and ownership checks.
- OPA/Gateway tests for revision identity and deny-all transition behavior.
- Docker integration proving stable OPA container identity, multi-decision
  activation, invalid-bundle retention, and retry behavior.
- `task check`, `task security`, `task policy:test`, and
  `task integration:test`.
