# Agent Readiness Validation

Tobari is agent-ready when a coding agent can discover the shared cluster,
run the root command from a project directory, enter an exact CWD-owned
Workspace without an ID, and recover from denied network requests without
source inspection. This is also a product-adoption check: the bounded path
must be easier to choose than running the agent on the host. Human entry below
ancestor roots explicitly chooses reuse or creation; that interaction is
outside the machine help contract.

## Agent integration boundary

The current CLI and its catalog already close the supported agent workflow:
read-only discovery exposes the denial and an opaque candidate, a trusted host
performs one exact allow or deny action, and the agent retries only after that
action succeeds. The same catalog owns runtime initialization and build as
explicit host-side actions. No official Codex or Claude Code plugin, MCP
server, generic executor, or auth broker is required for this local outcome.

A future integration should be a thin skill or wrapper over scoped catalog
help and existing commands. It must not copy command metadata, create a second
permission registry, accept credentials, or expose Docker/OPA authority. A
plugin becomes worthwhile only when repeated cross-repository distribution
creates a real packaging need; an MCP surface requires a separate live-service
or authenticated-action contract.

## Experience scorecard

The current implementation is safe and catalog-complete, but the following
journey is the product baseline to improve:

| Journey | Current surface | Desired evidence |
|---|---|---|
| First isolated session | `doctor`, explicit `cluster up`, then `tobari` | One obvious CWD-first entry with cluster/bootstrap complexity guided or hidden while retaining an explicit recovery command |
| Agent work | Reusable root, home, and deny-by-default Gateway | The user sets the boundary first; the agent works freely inside it without per-command supervision, host credentials, or direct egress |
| First denied request | 403 plus host-side `policy review` | The agent can explain that the host must review the secret-free exact request; one fixed next command is available |
| Permission growth | Human `policy review` Permission Inbox; machine `policy candidates`, then `policy allow --id` or `policy deny --id` | TTY users select, inspect, explicitly confirm, and refresh after one exact decision without OPA/Rego editing; redirected review remains read-only and the underlying action remains exact-reference-bound and tested |
| Advanced policy | Edit trusted-host Rego explicitly | Remains an explicit escape hatch, never a prerequisite for routine success |
| Execution setup | `context list`, `context show`, `context use --name NAME` | The user can inspect and select one logical setup while policy, agent configuration, and credential stores remain physically separated |
| Runtime customization | `runtime init`, edit the active Context Dockerfile, `runtime build` | The user gets a Context-specific runtime image without naming an image or editing the Context manifest; failed builds preserve the previous image |
| Recovery and cleanup | `tobari`, `status`, `delete`, and `cluster down` | Reuse, recovery, and removal are obvious, CWD-local, reversible, and ownership-checked |

The table is a review baseline, not a claim that the desired command count is
already met. Readiness evidence records command count, discovery rounds,
external-processing count, and the exact next command separately for machine
and human paths.

The current human-path evidence supports keeping the Permission Inbox,
CWD-owned entry, explicit cleanup, and Context runtime commands. No observed
journey justifies deleting or merging a public command. The valid recovery
path after an allowed permission is `tobari` re-entry; there is no `tobari retry`
command. Runtime builds apply to new Workspaces, while existing
Workspaces keep their selected image. Future UX work may simplify bootstrap or
polish terminal redraws, but those observations do not change the current
contract or authorize a second command surface.

Parent-owned human-path evidence uses a real-PTY capture boundary. The
reviewable bundle records `rows`, `cols`, `TERM`, one short input event at a
time, monotonic timing, redraw-preserving output checkpoints, exit status, and
SHA-256 digests. Raw bytes stay in a task-owned directory outside Git; only a
reviewed projection and its relative metadata belong in a work packet. The
capture boundary does not teach a blind child the scenario route and does not
turn a piped replay into human success. Run its contract test with
`python3 scripts/test-pty-evidence.py`.

## Required scenario

Run against a clean Docker Engine with a synthetic root:

```sh
go run ./cmd/tobari help --format agent
go run ./cmd/tobari help cluster --format agent
go run ./cmd/tobari help tobari --format agent
go run ./cmd/tobari help status --format agent
go run ./cmd/tobari doctor --root /absolute/test/root --format json
cd /absolute/test/root
go build -o /tmp/tobari ./cmd/tobari
(cd /absolute/test/root && /tmp/tobari)
(mkdir -p /absolute/test/root/root && cd /absolute/test/root/root && /tmp/tobari)
(cd /absolute/test/root && /tmp/tobari status --format json)
(cd /absolute/test/root && /tmp/tobari list --format json)
go run ./cmd/tobari cluster denials --tail 100 --format json
go run ./cmd/tobari context show --format json
go run ./cmd/tobari context list --format json
go run ./cmd/tobari context use --name default
go run ./cmd/tobari runtime init
# edit the active Context's runtime/Dockerfile
go run ./cmd/tobari runtime build --format json
go run ./cmd/tobari policy review --tail 100
go run ./cmd/tobari policy candidates --tail 100 --format json
go run ./cmd/tobari policy allow --id PCY_ID
go run ./cmd/tobari policy compactions --format json
go run ./cmd/tobari delete
go run ./cmd/tobari cluster down --purge
```

`list` emits a stable ID only as diagnostic information; lifecycle actions
resolve the same target from the current directory. `PCY_ID` denotes one exact
value emitted by `policy candidates`; the
compaction command may validly return an empty collection until enough exact
rules exist. The transcript must prove:

- Root agent help is a compact outcome/capability index.
- Scoped help supplies inputs, outputs, prerequisites, effects, references,
  failures, and recovery commands.
- Context discovery identifies the active Context and its separated agent,
  policy, and credential stores without exposing secret values.
- Cluster startup mounts no work root.
- The root command binds the canonical current directory and a compatible local
  image selector without a name or root flag; an ancestor-root entry exposes all
  containing Workspaces nearest-first and requires an explicit reuse/create
  choice.
- Omitted image selection resolves from the active Context's runtime image; the
  legacy XDG default seeds the default Context and then `builtin` is used,
  without requiring source inspection.
- `runtime init` creates one owner-only active-Context Dockerfile and does not
  overwrite it or change the selected image.
- `runtime build` uses only that recipe directory, validates the runtime
  contract and image digest, derives the local image reference mechanically,
  and promotes it into the Context without a second image-selection command.
  A build or validation failure leaves the previous selected image unchanged.
  The exact official `runtime:latest` base is refreshed on this explicit build;
  explicit local or custom bases do not request a registry pull.
- Project metadata does not override the active Context image.
- `list` retains an explicitly exhaustive local collection, including empty,
  while preserving diagnostic IDs without making them action inputs.
- `status` and `delete` resolve the same nearest canonical ancestor; `tobari`
  enters an exact root directly and explicitly chooses among ancestor roots.
- A child terminal exit leaves the logical Tobari existing for reuse, emits the
  host-side resume/delete guidance on stderr, and does not introduce a stopped
  state; detached `tobari delete` removes it, while `tobari delete --force`
  explicitly overrides an attached-session warning.
- A denied request produces bounded typed secret-free host/method/path
  evidence, the host policy path, and the exact review command. Baseline
  terminal denies remain audit evidence without inviting approval.
- Candidate discovery deduplicates pending effects and emits opaque references
  without changing authority; orthogonal scheme, project-principal, or
  managed-credential failures remain diagnostics and do not become ineffective
  candidates.
- `PCY_ID` denotes one exact value emitted by `policy review` or `policy
  candidates`; the transcript passes it unchanged to either `policy allow` or
  `policy deny`. Allow tests the complete policy and records one exact learned
  rule; deny records one exact project-bound terminal rule. Both activate
  without restarting a Tobari.
- Cleanup verifies exact owner and opaque-ID labels.

## Policy-learning scenario

The Docker integration test supplies the executable loop:

1. A mock-host GET is allowed.
2. A mock-host POST under `/denied` receives `403`, never reaches upstream, and
   is reported as a terminal baseline denial rather than a queue candidate.
3. Two learnable mock-host PUT denials receive `403` and expose fixed review
   navigation without a body or credential canary.
4. `cluster denials` exposes the rejected dimensions, trusted XDG policy path,
   and exact review command. `policy review`, `policy candidates`, and
   `policy tail` expose the same pending exact proposals and opaque references;
   redirected review does not mutate policy.
5. `policy allow` tests a private complete policy copy, atomically stores the
   exact learned rule, recreates only OPA, and confirms it healthy. The exact
   retry succeeds while a child path remains denied.
6. `policy deny` records and activates one exact project-bound terminal rule;
   the rejected request remains denied and its historical lines disappear from
   the actionable review queue.
7. Three separately denied and approved sibling paths produce one current
   `policy compactions` proposal.
8. `policy compact` retains the examples, activates the bounded prefix, permits
   a sibling, and keeps its adjacent outside-prefix canary denied.

Routine success and denial require zero undeclared provider parsers,
provider-notation decoders, source inspection steps, or exploratory provider
calls.

## Network and credential scenario

`task integration:test` additionally proves:

- Two CWD-owned Tobari have distinct internal networks, roots, and homes.
- Neither has direct egress, OPA access, or cross-Tobari reachability.
- HTTPS is authorized after CONNECT interception and validates the Tobari CA.
- OPA and Gateway outages fail closed.
- Tool-owned authentication state persists below one Tobari home and is not
  visible from another Tobari.
- The default passthrough adapter forwards a client-authenticated request only
  after allow, while its value is absent from mounts, logs, OPA input, and CLI
  output.
- The retained managed adapter is covered separately by exact project/host
  binding and post-allow injection tests.
- Concurrent processes share one selected Tobari.
- Repeated root reconciliation does not grow owned resources.

## Interpretation rules

Agents must not infer:

- identity from a display name, container label, log order, or position;
- permission from a previous allow or similar path;
- exhaustive external history from complete CLI delivery;
- credential authority from a profile name;
- safety from an executable name;
- protection for files below a selected read-write root.

The opaque reference, declared local collection scope, cluster status, bounded
denial/log coverage, OPA decision, and structured failures are the supported
interpretation inputs.

## Failure and recovery validation

At minimum, exercise:

- invalid or inaccessible root before Docker mutation;
- nested-root entry lists all containing ancestors, reuses a chosen one, or
  creates a new root only after an explicit create choice;
- cancelled, unavailable, or stale Workspace selection performs no logical or
  Docker mutation and directs the user to retry or choose again;
- invalid, missing, incompatible, or conflicting image selection before Docker
  resource creation;
- invalid or incompatible Context image configuration before Docker mutation;
- malformed or legacy state;
- invalid Rego before cluster reconciliation;
- invalid Rego before policy activation and exact OPA-only recreation after a
  valid edit;
- invalid, stale, already-covered, or wrong-kind policy candidate references;
- duplicate, symlinked, group/world-accessible, concurrently changed, or
  malformed managed policy data before write;
- fewer than three, shallow, mixed-host, mixed-method, or stale compaction
  sources;
- failed learned-policy preflight before atomic replacement;
- partial startup mapped to non-retryable `cluster_start_failed`;
- partial root reconciliation mapped to non-retryable `runtime_reconcile_failed`;
- non-empty cluster removal rejection;
- unknown or modified opaque ID before Docker calls;
- partial delete remains retryable through `delete` and never removes logical
  state before exact resource cleanup;
- non-TTY root invocation fails before logical state creation;
- OPA unavailable and malformed decision;
- output write failure without partial structured data.

A post-mutation raw adapter error must never become replay permission. Valid
structured outcome faults are preserved; unknown outcomes collapse to a
non-retryable contract fault.

## Evidence

```sh
task check
task runtime:test
task security
task public:check
```

Review the typed Gateway denial, tested OPA activation, two-network topology,
and Docker cleanup counts alongside command results.
