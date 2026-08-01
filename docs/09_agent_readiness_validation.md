# Agent Readiness Validation

Tobari is agent-ready when a coding agent can discover the shared cluster,
run the root command from a project directory, enter an exact CWD-owned
Workspace without an ID, and recover from denied network requests without
source inspection. Human entry below ancestor roots explicitly chooses reuse
or creation; that interaction is outside the machine help contract.

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
go run ./cmd/tobari policy candidates --tail 100 --format json
go run ./cmd/tobari policy allow --id PCY_ID
go run ./cmd/tobari policy compactions --format json
go run ./cmd/tobari delete --force
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
- Cluster startup mounts no work root.
- The root command binds the canonical current directory and a compatible local
  image selector without a name or root flag; an ancestor-root entry exposes all
  containing Workspaces nearest-first and requires an explicit reuse/create
  choice.
- Omitted image selection resolves from the XDG default and then `builtin`
  without requiring source inspection.
- An explicit image-based Dev Container file resolves within the selected root;
  unsupported runtime metadata returns one stable recovery fault.
- `list` retains an explicitly exhaustive local collection, including empty,
  while preserving diagnostic IDs without making them action inputs.
- `status` and `delete` resolve the same nearest canonical ancestor; `tobari`
  enters an exact root directly and explicitly chooses among ancestor roots.
- A child terminal exit leaves the logical Tobari existing for reuse, emits the
  host-side resume/delete guidance on stderr, and does not introduce a stopped
  state; `tobari delete --force` is the explicit external cleanup operation.
- A denied request produces bounded typed secret-free host/method/path
  evidence, the host policy path, and the exact activation command.
- Candidate discovery deduplicates pending effects and emits opaque references
  without changing authority; orthogonal scheme or credential failures remain
  diagnostics and do not become ineffective candidates.
- `policy allow` consumes one candidate reference unchanged, tests the complete
  policy, records only an exact rule, and activates it without restarting a
  Tobari.
- Cleanup verifies exact owner and opaque-ID labels.

## Policy-learning scenario

The Docker integration test supplies the executable loop:

1. A mock-host GET is allowed.
2. A mock-host POST under `/denied` receives `403` and never reaches upstream.
3. `cluster denials` exposes the rejected dimensions, trusted XDG policy path,
   and exact apply command without a body or credential canary.
4. `policy candidates` and `policy tail` expose one pending exact proposal and
   its opaque reference without mutating policy.
5. `policy allow` tests a private complete policy copy, atomically stores the
   exact learned rule, recreates only OPA, and confirms it healthy.
6. The exact retry succeeds while a child path remains denied.
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
- A profile is injected only after allow and exact host binding.
- The secret is absent from Tobari files, mounts, environment, logs, OPA input,
  and CLI output.
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
- escaping, malformed, ambiguous, oversized, or unsupported Dev Container
  configuration before Docker mutation;
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
