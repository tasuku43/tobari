# ADR 0013: Use a logical Context with physically separated stores

- Status: Accepted
- Date: 2026-08-02
- Revised: 2026-08-04
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, authentication, and harness
- Supersedes: None
- Superseded by: None

## Context

Tobari currently exposes several related but separate concepts: a shared
agent profile, an installation-wide policy directory, and reserved managed
credential metadata. Users need one clear object that describes the execution
setup they are choosing, while the implementation must not turn policy,
agent configuration, and secrets into one broad mount or one interchangeable
authority.

The product thesis is to define the agent's activity boundary before work and
let it move freely inside. The configuration surface therefore needs to make
the complete boundary understandable and adjustable without requiring users to
become Docker, OPA, or credential-adapter operators.

## Decision drivers

- One user-facing composition concept for agent, policy, and credential choices.
- Physical trust boundaries must remain explicit and enforceable.
- Tool-native authentication must remain inside a Workspace home by default.
- Existing `default` installations and learned permissions must survive rollout.
- The shared-cluster MVP must not grow a second policy-routing system implicitly.
- Future Gateway adapters and per-Workspace Context selection must remain
  possible without replacing the public model.
- A future user-selectable authentication adapter must switch with the Context;
  the adapter implementation itself remains an infrastructure plug-in and is
  never a manifest authority.

## Considered options

### Option A: One physical directory containing every Context asset

This is easy to explain but mixes Gateway-only secret material with files that
could be mounted read-only into OPA or a project. It creates pressure to pass a
single directory through the trust boundary. Rejected.

### Option B: Keep the current profile/policy/credential vocabulary

This preserves implementation paths but leaves users to assemble the setup
mentally and makes future adapter configuration another unrelated surface.
Rejected because it does not address the product friction.

### Option C: Logical Context manifest with separated stores

One trusted manifest names the agent profile, compatible Tobari runtime image,
and policy/credential stores. Policy and credentials remain in separate
owner-only paths, and the agent profile remains a read-only data store. The
active Context is host-selected.
Chosen.

## Decision

Tobari introduces a named `Context`. A Context manifest is stored at:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/tobari/contexts/<name>/context.json
```

Its non-secret fields are `name`, `agent_profile`, `image`, and `policy_mode`
(`guided` or `advanced`). `image` is the compatible Tobari runtime image used
for Workspace creation and runtime-container reconciliation. Project metadata
does not override it.
The same Context directory contains separate `policy/`,
`credentials.json`, and `credentials/` stores. The manifest references the
agent data store under the existing XDG data directory; it does not copy or
contain tool credentials. A host-owned active marker selects one Context for
the installation-local shared cluster.

The first slice keeps one active Context for the shared cluster. Cluster state
records the selected Context and its resolved stores. A Context change is an
explicit host mutation. If the shared cluster is already running,
`context use` reuses the bounded `cluster up` reconciliation path, applies the
selected policy and Gateway credential stores, waits for health, and reports
success only after the state is persisted. It never starts an unconfigured or
stopped cluster implicitly; those outcomes update only the host marker and
direct the user to `cluster up`. Entry and access-changing actions fail closed
while an interrupted reconcile journal or active-context mismatch remains.
Project runtime reconciliation uses the selected Context's agent profile and
profile digest.
Per-Workspace Context routing is deferred until Gateway/OPA routing and
project-principal semantics are designed explicitly.

The default Context is created by a compatibility migration. Existing legacy
policy and credential metadata are copied once into its separated stores;
existing Workspace homes, tool-owned authentication state, and legacy source
files are not deleted.

## Consequences

### Positive

- Users can inspect and change one named execution setup, including its runtime
  image, without editing unrelated top-level configuration.
- OPA policy, managed credentials, and agent configuration remain separate
  security assets even though they are one product concept.
- The default passthrough adapter and tool-native login flow remain unchanged.
- Future Context-aware adapter selection and per-Workspace routing have a
  stable vocabulary and manifest boundary.

### Negative

- The first implementation has one active Context per shared cluster, not one
  Context per Workspace.
- Migration temporarily leaves legacy top-level stores for compatibility and
  diagnosis.
- Switching Context while a cluster is running may recreate shared enforcement
  resources and takes as long as the bounded health wait, but no second manual
  reconcile command is required. A failed switch restores the previous marker
  and state when possible and leaves explicit recovery when Docker may have
  changed.

## Security and public-boundary impact

Context commands never accept or print secret values. Policy is mounted
read-only into OPA, managed secrets are Gateway-only, and no Context directory
is mounted wholesale into a Workspace. Names and paths are non-secret metadata,
but credential authority still comes from validated project/host bindings and
OPA decisions, never from a Context or profile display name.

## Mechanical enforcement

- Domain tests validate Context names, modes, manifest completeness, and state
  identity.
- Infrastructure tests validate owner-only regular files, atomic active
  selection, legacy migration, and separate policy/credential paths.
- Catalog tests validate four Context command contracts and mutation targets.
- Runtime tests prove active Context paths reach OPA and the selected agent
  profile mount while secret values remain outside the Workspace, and cover
  stopped, already-ready, running-switch, rollback, and interrupted-journal
  Context selection states.
- Agent-readiness validation includes Context discovery, synchronous selection,
  and explicit cluster-reconcile recovery after interruption or failure.

## Compatibility and migration

Before v1.0 the manifest schema may evolve with release notes. A missing active
marker means `default`. A state without `context_name` is interpreted as the
legacy default until the next successful cluster reconciliation writes it.
Legacy top-level policy and credential files remain untouched during the first
migration and can be used by an older binary after a rollback.

## Validation

- `go test ./internal/domain/tobari ./internal/app/contextcmd ./internal/infra/dockerruntime ./internal/cli`
- The state-matrix replay covers unconfigured, stopped, already-ready, running
  switch, failed switch, interrupted recovery, and repeat selection; the
  running switch was also replayed through the real-PTY harness.
- `task check`
- `task security`
- `task public:check`
