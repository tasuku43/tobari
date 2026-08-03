# Work Plan: Agent integration discovery

- Status: Complete
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Use the existing Tobari CLI as the only executable capability surface and add
agent guidance as a small standalone skill in a follow-up implementation
packet. Keep the first skill focused on policy recovery: discover the exact
denial, explain the host handoff, require explicit human confirmation, pass one
opaque candidate unchanged to `policy allow` or `policy deny`, and retry only
after the host action reports success. Treat Context runtime customization as a
second focused skill because it has a different trigger, side-effect profile,
and recovery contract.

The same Agent Skills workflow can be made available to Codex and Claude Code
through their native local skill locations. It should call the existing CLI,
not reimplement catalog metadata or create a new tool protocol. A plugin is a
later packaging decision, not the first implementation slice.

## Alternatives considered

### Alternative A: Build a plugin now

Rejected for the current slice. The local CLI already provides the tools and
authority needed for both journeys. Packaging a plugin now would add manifest,
distribution, license, privacy, and update obligations before repeated skill
use demonstrates a distribution problem. It also risks presenting a second
surface whose command and recovery metadata can drift from `cli.Catalog`.

### Alternative B: Add an MCP server or auth broker now

Rejected. This workflow has no need for live external data, user OAuth, remote
service actions, or a server-owned credential. An MCP bridge would be an
unnecessary second catalog and could accidentally turn policy, Docker, or
credential authority into model-callable tools. The previously deferred
auth-broker direction remains detached from main.

### Alternative C: Give the skill a generic executor

Rejected by the product and security contracts. A generic executor would make
the skill an undeclared side-effect boundary and weaken the distinction between
agent work inside a Tobari and trusted host policy/runtime actions.

## Design

### Public contract

No public CLI contract changes in this discovery packet. The follow-up policy
skill must use these existing paths:

- `help --format agent` for the compact capability index, then exact or
  namespace-scoped help for inputs, outputs, failures, and recovery.
- `policy candidates --tail N --format json` or `policy review --tail N
  --format json` for read-only discovery.
- TTY `policy review` only as the human confirmation UI; redirected and
  machine-readable review remains read-only.
- `policy allow --id ID` or `policy deny --id ID` only after explicit user
  confirmation, with the exact opaque ID copied unchanged.
- `runtime init --format json`, user editing of the active Context Dockerfile,
  and `runtime build --format json` for the separate host-side runtime skill.

`policy candidates` and `policy review` remain `RoleDiscover`; `policy allow`
and `policy deny` remain `RoleAct` reference-bound actions. Runtime commands
remain fixed-target host actions. The skills add no capability-ledger entry and
no new authentication contract.

### Layer changes

- Domain: none.
- Application: none.
- Infrastructure: none.
- CLI and catalog: none.
- Follow-up skill files: instructions only, invoking the existing executable;
  no Go-layer dependency and no unrestricted subprocess abstraction.

### Data and control flow

```text
agent
  -> skill instructions and current scoped help
  -> existing Tobari CLI/catalog
  -> policy discovery (read-only)
  -> explicit human confirmation
  -> exact opaque reference-bound action
  -> OPA/Gateway activation through existing infrastructure
  -> agent retry of the same request
```

For runtime customization, the flow is:

```text
host/user
  -> runtime init
  -> edit active Context Dockerfile
  -> explicit runtime build
  -> compatibility/digest validation
  -> Context image promotion
  -> tobari uses the active Context image
```

The agent cannot choose the policy directory, Context, image authority, host
root, credentials, or Docker resource identifier through either flow.

### Error and cancellation behavior

- A non-learnable denial, unavailable policy, redirected review, stale/missing
  candidate, or absent human confirmation stops the skill and reports the
  exact catalog recovery path. It does not synthesize an allow rule or retry.
- A learnable denial is advisory navigation only. The skill must not treat a
  displayed position, command text, or previous approval as permission.
- A failed policy action remains a structured non-success and must not be
  replayed blindly. The existing action output and read-only status/review
  commands are the recovery boundary.
- A failed runtime build leaves the prior Context image selected. The skill
  directs the user to inspect `context show` and retry the explicit build.
- No new authentication, timeout, retry, idempotency, or cancellation contract
  is introduced. Existing CLI exit codes and structured faults remain the
  source of truth.

### Security and public boundary

The skills receive only task-specific request evidence and command output. They
must not request raw credentials, tokens, cookies, policy source, private
configuration, Docker socket access, or arbitrary executor arguments. They
must preserve safe display projection and exact opaque IDs. Any later MCP
server would need a separate use-case inventory, authorization contract,
read/write annotations, and security review; this packet authorizes none of
that work.

## Implementation slices

1. Create a local `tobari-policy-recovery` skill using the existing catalog
   paths. Include direct, indirect, incomplete, and out-of-scope activation
   tests plus the policy E2E proof.
2. Create a separate `tobari-runtime-build` skill only after the first skill's
   workflow is stable. Include explicit confirmation for the Docker build and
   failure-preservation checks.
3. Replay both skills from Codex and Claude Code native skill entry points. Keep
   the CLI transcript, command count, discovery rounds, and recovery result
   identical across surfaces.
4. Reconsider plugin packaging only if two or more skills need versioned
   cross-repository distribution. Reconsider MCP only if a concrete live
   external service or authenticated controlled action is required.

## Verification

- Unit and contract tests: existing catalog/domain/application tests; no new
  production tests in this discovery packet.
- Negative side-effect tests: existing policy and runtime integration checks
  prove no automatic retry, no cross-project allow, no child-path broadening,
  and safe build promotion.
- Opaque-reference and complete-pagination tests: existing candidate/action
  round-trip and bounded policy projections in the harness.
- Structured output, hostile-output, and recovery tests: existing Gateway
  denial response, policy JSON, exact action faults, and cleanup assertions.
- Agent-readiness scenario and discovery-round-trip count: root agent help
  followed by scoped policy/runtime help; the integration transcript performs
  read-only discovery before each exact action and completes the retry loop.
- Human-handoff scorecard for setup/authentication candidates: no setup/auth
  surface is proposed now; the human owns the one policy confirmation and the
  explicit runtime build invocation, while no credentials are collected.
- Manual observation: the integration harness reached `integration: OK` on
  Docker 27.4.0 / Compose v2.24.6 / Colima arm64 after redirecting only
  BuildKit configuration to a task-specific temporary directory.
- Required verification: the real integration workflow proof reaches
  `integration: OK`; `task check` and `task public:check` are rerun before
  commit. The literal current-worktree integration helper has a separate
  uncommitted TTY-only failure and is not part of this packet's staged files.
- Generated-diff or artifact checks: no generated repository artifact or
  external schema was added; only this child packet is expected in the diff.

## Rollout and rollback

Not applicable to this discovery packet: no public contract, executable, image,
configuration, or external state is changed. A future skill can be removed or
disabled without changing Tobari state. A future plugin must be versioned and
reversible independently from the CLI.

## Documentation promotion

No durable contract change is justified by this evidence. Before a future skill
is implemented, promote only the stable workflow boundary and its tests into
the skill packet and, if the skill changes public expectations, update the
affected product/security/harness documents in the same reviewed change. Do
not promote a plugin or MCP design from this discovery packet without a new
use-case, authorization, and E2E proof.
