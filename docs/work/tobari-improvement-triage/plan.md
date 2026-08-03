# Work Plan: Triage Tobari product and maintenance backlog

- Status: Active
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Use this packet as a coordinator, not as a large implementation bucket. Keep a
single issue register with explicit states and dependencies, then create one
child packet for each issue that crosses a product, architecture, security,
public, or harness boundary. Start with the policy-review TTY regression because
it blocks the central denial-to-retry story and the Quick Start evidence. Keep
the auth-broker branch deferred and detached. Explore agent integrations at the
workflow boundary first, beginning with a skill-shaped local workflow and only
adding a plugin or MCP server when a shared/installable or server-backed
capability is proven necessary. Audit the catalog before finalizing public docs,
and clean completed packets only after their evidence and durable conclusions
are accounted for.

Every child is evaluated through an observable end-to-end scenario. A packet is
not complete because its analysis, unit tests, or documentation build passes:
the child must name the user journey, run it against the resulting surface, and
record the command, result, and artifact or transcript in its `context.md` and
`tasks.md`. For governance-only work, the E2E scenario is the repository or
branch boundary that a maintainer can inspect and replay; it must still have a
machine-checkable assertion. An environment failure is recorded as a blocker,
not counted as completion.

Each delegated child owns its commit. Before reporting completion, the child
must re-check its scoped diff, rerun the required E2E and repository gates,
create one intentional commit containing only its packet or implementation
slice, and report the commit SHA. The coordinator may review and integrate the
commit, but does not manufacture a substitute commit for a child.

## Alternatives considered

### Alternative A: Put all implementation into one large packet

Rejected for now. Policy UI, provider/auth boundaries, public documentation,
catalog retirement, image publication, and packet lifecycle have different
contracts and verification gates. One large packet would make an active issue
look complete when only one slice had passed.

### Alternative B: Track only a conversation-level list

Rejected for this repository work. A conversation list cannot be checked by
the repository guard, cannot carry evidence or successor links, and makes it
too easy to lose the distinction between an active packet and a deliberately
deferred branch.

### Alternative C: Build provider-specific plugins before stabilizing Tobari

Rejected. It would move auth and agent semantics back into Tobari before the
generic boundary and denial/retry workflow are proven, conflicting with the
current MVP exclusions and increasing the public/security surface prematurely.

## Design

### Public contract

This coordinator packet adds no public command, capability, effect, target, or
external adapter. Its only output is repository-local planning evidence. Child
packets must classify their own utility/discover/act role, input and output
contracts, opaque-reference flow, delivery/coverage, faults, recovery command,
agent-readiness scenario, and required gate profile before implementation.

The initial child-packet routing is:

| Child packet | Outcome | Main contract boundary |
|---|---|---|
| Policy-review TTY | Human can see and deliberately review one pending exact permission | Product, CLI presentation, harness, agent readiness |
| Auth-broker deferral | Main remains free of the deferred auth-broker capability while its branch disposition is explicit and replayable | Architecture, security, repository history |
| Agent integration discovery | User can manage policy/runtime workflows through a supported agent surface without a new authority boundary | Product, skill/plugin, security, public docs |
| Quick Start and architecture | A clean user can run the denial-to-review-to-retry and runtime customization journeys and understand the topology | Product, public repository, release/docs publication |
| CLI catalog audit | Public command surface has no redundant or unexplained paths | Product, architecture, catalog, compatibility, harness |
| Work-packet lifecycle | Active/completed/deferred evidence is discoverable and mechanically consistent | Harness, public repository |

### Delegation waves

The first wave uses disjoint write scopes and can run in parallel:

1. Policy-review TTY implementation and PTY E2E.
2. CLI/catalog audit and command-surface E2E inventory.
3. Agent/plugin and custom-runtime-skill discovery with a bounded workflow
   proof.
4. Work-packet lifecycle audit and cleanup of only E2E-proven completed packets.

The auth-broker packet is parked outside these waves. Its branch and read-only
governance evidence remain detached from `main`; no implementation, merge,
cherry-pick, current-line commit, or dependency edge is allowed until the
maintainer explicitly resumes it. Its existing packet is retained as deferred
evidence and is not a blocker for the core product waves.

The second wave starts only after the policy-review and catalog reports are
accepted. It owns README, Quick Start, architecture HTML/GitHub Pages, and the
runtime-image customization walkthrough. Its E2E is a clean checkout/build,
render, and replay of both documented journeys. No second-wave writer edits
the first-wave packets; it consumes their reported facts.

The coordinator integrates child results and is the only writer of the issue
register. Sub-agents must write their own child packet or implementation slice,
not `tobari-improvement-triage/context.md` or `tasks.md` directly.

### Layer changes

- Domain: none in the coordinator packet. Child packets add only task-owned vocabulary and invariants.
- Application: none in the coordinator packet. Policy-review behavior remains behind the existing application port.
- Infrastructure: none in the coordinator packet. Runtime, Docker, network, and plugin side effects remain outside this packet.
- CLI and catalog: no new command. Any review fix or retirement must be made through the existing catalog and its contracts.
- Documentation and repository: add this temporary packet; promote only durable conclusions to numbered docs or ADRs.

### Data and control flow

```text
reported issue or repeated friction
  -> verified repository fact and governing contract
  -> issue register state + dependency + owner role
  -> bounded child packet
  -> contract/tests/implementation or explicit deferral
  -> required gates and agent-readiness evidence
  -> durable documentation promotion
  -> child packet cleanup and coordinator closure
```

The policy-review child must keep the typed candidate report as the semantic
source of truth, preserve the exact candidate ID unchanged into the selected
action, and prove zero policy mutation on cancellation, redirected output, and
invalid selection. The documentation child must use the final tested command
path rather than documenting a workaround discovered during debugging.

### Error and cancellation behavior

The coordinator has no runtime side effect and is safe to cancel. A child
packet is not complete when a test, gate, publication, or evidence step is
unknown. Canceled policy review remains a successful no-op only if the queue
was visibly presented or the non-interactive contract clearly explained the
read-only behavior; the child packet must decide the exact presentation without
turning cancellation into permission.

### Security and public boundary

The coordinator contains no secrets and uses synthetic examples. Any future
plugin/MCP server, GitHub Pages build, runtime image publication, or policy
mutation must be reviewed against the relevant security/public/release
contracts. No child may expose an unrestricted command executor, pass host
credentials into a Tobari, or treat an agent/vendor name as an authorization
boundary.

## Implementation slices

1. Record the issue register and verified evidence in this packet.
2. Reproduce the policy-review TTY behavior and create the smallest child packet.
3. Record the auth-broker deferral and branch disposition without changing it;
   do not schedule follow-up work until explicit resumption.
4. Create discovery/decision packets for agent integrations and runtime-image skill packaging.
5. Audit the catalog and capability ledger before editing the Quick Start or command table.
6. Create the public documentation/architecture successor with runnable, synthetic examples.
7. Audit and close work packets only after evidence promotion and required gates.

## Verification

- Unit and contract tests: each child packet must name focused tests; the policy-review child must run the selector, CLI, catalog, and application tests.
- Negative side-effect tests: preserve zero calls before invalid/canceled policy actions and no hidden external mutation from plugins or docs tooling.
- Opaque-reference and complete-pagination tests: preserve the existing policy candidate graph and queue scope; do not infer history from a complete bounded window.
- Structured output, hostile-output, and recovery tests: retain JSON/TTY separation, redaction, hostile external text projection, and exact recovery commands.
- Agent-readiness scenario and discovery-round-trip count: rerun the denial-to-retry journey and record the human and machine discovery budgets; runtime customization must retain its explicit build path.
- Human-handoff scorecard: record whether a user can understand the first next action without becoming a Docker, OPA, or plugin operator.
- End-to-end completion gate: each child records a successful user-journey
  replay; analysis-only, unit-only, or build-only evidence cannot close it.
- Manual observation: use a clean PTY for policy review and a clean public-doc render for the Quick Start/architecture successor.
- Required profiles: `task check` for coordinator completion; add `task security`, `task public:check`, `task release:check`, `task runtime:test`, or `task integration:test` in the applicable child packet.
- Generated-diff or artifact checks: any GitHub Pages build or generated architecture artifact must be deterministic and leave no unreviewed files.

## Rollout and rollback

Not applicable to the coordinator packet: it adds only temporary planning
files. Each child packet owns its own rollout and rollback. Closing a packet is
recoverable through Git history; deleting a temporary packet is allowed only
after its durable conclusions and successor/deferred disposition are recorded.

## Documentation promotion

- Promote any revised thesis or durable product/architecture/security decision before closing the relevant child packet.
- Promote the final policy-review contract and agent-readiness evidence into the existing numbered docs if behavior or expectations change.
- Promote plugin/runtime integration boundaries into a skill, plugin manifest, ADR, or governing doc only after the discovery decision is accepted.
- Promote catalog retirements through `capability-retirement.md`, the catalog, ledgers, help, recovery, and tests together.
- Keep this coordinator packet temporary; it is not a permanent roadmap or second command registry.
