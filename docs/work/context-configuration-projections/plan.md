# Work Plan: Configure narrow Context projections

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Replace `context shell configure` with `config shell` and add `config git`.
Each command has one catalog-owned fixed-target mutation and two deterministic
input modes: no setting flags opens a terminal text-success/error wizard; a
complete setting group executes directly. Partial, redirected, or JSON wizard
invocations fail before mutation. Git identity is an atomic Context policy with
`default|inherit|literal`; inherited identity is resolved from two exact host
global Git keys for each stable Workspace root and re-encoded as a low-priority
runtime-owned fallback.

## Alternatives considered

### Prompt for missing direct-mode fields

This saves keystrokes but can turn an accidental script omission into a hidden
terminal wait and weakens the executable argv contract. It was rejected.

### Require `--interactive`

This creates the clearest syntactic mode split, but makes the ordinary human
path ceremonial. Terminal detection plus all-or-none input groups provides the
same automation safety with a cleaner default.

### Mount or copy host Git configuration

This would preserve every Git feature but crosses credentials, helpers,
signing, executable configuration, arbitrary include paths, and unrelated
policy into an untrusted Workspace. It is rejected.

## Design

### Public contract

- `config shell [--context NAME] [--format text]` opens the wizard; direct mode
  requires `--variable` and `--source`, and `literal` additionally requires
  `--value` (including explicit empty).
- `config git [--context NAME] [--format text]` opens the wizard; direct mode
  requires `--source`, and `literal` additionally requires non-empty `--name`
  and `--email`.
- `default` removes the Context projection, `inherit` late-binds host values,
  and `literal` persists Context-owned values.
- Both are `RoleAct`, `EffectWrite`, fixed `tool_local` targets in capability
  `context.composition`, complete delivery, and not-applicable coverage.
- Omitted `--context` deterministically targets the current Context without
  changing it. A wizard binds Apply to the Context name returned before review
  so a concurrent default change cannot retarget it; explicit empty is invalid,
  not omission. No opaque references are produced or consumed.
- Wizard prompts go to stderr; the confirmed Context report goes to stdout.
  Cancel and invalid/partial/TTY failures produce no mutation.
- Context report JSON schema becomes 6 and includes complete shell and Git
  policy state. Context manifest schema becomes 5; schema 4 remains a migration
  input and preserves its shell settings exactly.

### Layer changes

- Domain: Git identity source/pair/value invariants, task/target vocabulary,
  schema migration compatibility, complete Context report.
- Application: shell command rename, Git identity use case, fixed-target
  policy, task-specific runtime port, and exact task/Context/setting result
  correlation before presentation.
- Infrastructure: atomic Context policy persistence, schema-4 migration,
  bounded exact-key host Git resolver, Git-config encoder, and private
  per-Workspace projection reconciliation.
- CLI and catalog: configuration namespace, all-or-none direct input groups,
  terminal wizard selectors, dispatch, complete output schema, text/JSON
  rendering, and exact help.

### Data and control flow

```text
complete argv ------------------------------+
                                             |
terminal + wholly omitted setting flags      |
  -> show selected Context -> wizard -> review
                                             |
                                             v
typed setting -> app intent/policy -> atomic Context manifest write
                                      -> complete Context report

next Workspace root entry
  -> load Context Git policy
  -> if inherit: fixed host git reads of user.name and user.email
  -> validate a complete pair
  -> encode runtime-owned system-scope fallback atomically
  -> reconcile read-only directory mount
  -> Workspace global/repository config may override
```

### Error and cancellation behavior

- Invalid mode/source/value/partial inputs fail before application mutation.
- Wizard requires terminal stdin and stderr, text format, and a wholly omitted
  setting group. Raw-mode failure falls back to bounded English line input.
- Wizard cancellation restores terminal state and performs no mutation.
- Context writes use the standard invoker mutation outcome contract; unknown
  post-action results are non-retryable and reconcile through `context show`.
- Host Git uses at most two calls, one attempt each, a finite per-call timeout,
  bounded stdout/stderr, and no network. Command failure or unsafe output fails
  before projection replacement or Docker mutation.
- An absent or incomplete inherited pair installs no Context fallback and does
  not block Workspace entry; Workspace/repository configuration and Git's own
  identity validation remain authoritative.

### Security and public boundary

No host Git file, file path, include directive, local/worktree config,
credential, helper, signing setting, hook, alias, URL rewrite, filter, proxy,
SSH setting, or raw diagnostic crosses the boundary. The resolver invokes an
absolute executable that cannot be selected from the project root and requests
only two fixed keys with `--global --includes`. Its child environment contains
only validated `HOME` and optional `XDG_CONFIG_HOME` paths outside the root,
fixed locale/Git controls, and no ambient `PATH`, loader, or shell-startup
control. Generated and existing projection sizes are bounded before reads and
writes; values are valid UTF-8, control-free, quoted, private, and mounted
read-only. Fixtures use only `Tobari User` and `tobari@example.com`.

## Implementation slices

1. Contract, work packet, ADR, and failing product-shaped tests
2. Domain/application/store schema and mutation behavior
3. CLI namespace, direct-mode contract, and terminal wizard
4. Host Git resolver and per-Workspace fallback projection
5. Harness, README, authentication clarification, and agent-readiness evidence

## Verification

- Unit and contract tests: domain, app, catalog, argv, wizard, store migration,
  resolver, encoder, projection, report schema.
- Negative side-effect tests: partial/non-TTY/JSON/cancel zero mutation; local
  malicious include and project fake Git zero host leakage; unsafe resolver
  output zero projection/Docker calls.
- Opaque-reference and pagination tests: not applicable; fixed targets use no
  references and complete scalar output has no pagination.
- Structured output, hostile-output, and recovery tests: exact schema-6 keys,
  projected control/format characters, short writer, and read-only recovery.
- Agent-readiness: one scoped help request plus one complete direct invocation;
  zero external reconstruction steps.
- Human handoff: no environment export or clipboard step; wizard and direct
  entry are equivalent; a final review is retained for attribution certainty.
- Manual observation: raw selector and line fallback with synthetic values;
  real container Git precedence when Docker is available.
- Required profiles: focused Go tests, `task check`, `task security`,
  `task public:check`, and relevant runtime/integration tasks.

## Rollout and rollback

Before v1.0 the old `context shell configure` path is removed without an alias;
release notes/README identify `config shell`. Schema-4 manifests migrate
atomically to schema 5 while preserving ID, runtime, and shell settings. A
rollback to a schema-4 binary requires the matching older state backup because
it cannot interpret schema 5. `default` removes only Tobari's Git fallback and
never alters Workspace or repository config.

## Documentation promotion

- General narrow projection thesis and explicit excluded data.
- Configuration-first public command namespace and wizard/direct state machine.
- Layer ownership and root-scoped Git resolver/projection precedence.
- Security boundary, personal-data handling, and hostile repository canaries.
- Claims-to-checks row, README journey, Auth Broker separation, and
  agent-readiness scenario.
- ADR 0021 records the durable trust-boundary and CLI trade-offs.
