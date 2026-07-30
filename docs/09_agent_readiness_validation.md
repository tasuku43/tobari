# Agent Readiness Validation

Tobari is agent-ready when a coding agent can discover the shared cluster,
attach a named Tobari, pass one opaque ID unchanged into actions, and recover
from denied network requests without source inspection.

## Required scenario

Run against a clean Docker Engine with a synthetic root:

```sh
go run ./cmd/tobari help --format agent
go run ./cmd/tobari help cluster --format agent
go run ./cmd/tobari help attach --format agent
go run ./cmd/tobari doctor --root /absolute/test/root --format json
go run ./cmd/tobari cluster up
go run ./cmd/tobari attach --name test --root /absolute/test/root
go run ./cmd/tobari list --format json
go run ./cmd/tobari exec --id TBR_ID -- curl https://example.com/
go run ./cmd/tobari cluster logs --component gateway --tail 100
go run ./cmd/tobari detach --id TBR_ID --purge
go run ./cmd/tobari cluster down --purge
```

`TBR_ID` denotes the exact value emitted by `list`, not a reconstructable
literal. The transcript must prove:

- Root agent help is a compact outcome/capability index.
- Scoped help supplies inputs, outputs, prerequisites, effects, references,
  failures, and recovery commands.
- Cluster startup mounts no work root.
- Attach binds one validated display name, canonical root, and compatible local
  image selector.
- Omitted image selection resolves from the XDG default and then `builtin`
  without requiring source inspection.
- `list` retains an explicitly exhaustive local collection, including empty.
- Individual actions consume one `tobari` reference unchanged.
- `exec` passes exact argv and preserves the child status.
- A denied request produces bounded secret-free host/method/path evidence.
- Cleanup verifies exact owner and opaque-ID labels.

## Policy-learning scenario

The Docker integration test supplies the executable loop:

1. A mock-host GET is allowed.
2. A mock-host POST under `/denied` receives `403` and never reaches upstream.
3. `cluster logs --component gateway` exposes the rejected dimensions without
   a body or credential canary.
4. `cluster status` exposes the trusted XDG policy path.
5. A second named Tobari attaches only that policy directory.
6. A host-visible policy edit is watched by OPA without restart.
7. The retry succeeds; no rule is generated or accepted automatically.

Routine success and denial require zero undeclared provider parsers,
provider-notation decoders, source inspection steps, or exploratory provider
calls.

## Network and credential scenario

`task integration:test` additionally proves:

- Two named Tobari have distinct internal networks, roots, and homes.
- Neither has direct egress, OPA access, or cross-Tobari reachability.
- HTTPS is authorized after CONNECT interception and validates the Tobari CA.
- OPA and Gateway outages fail closed.
- A profile is injected only after allow and exact host binding.
- The secret is absent from Tobari files, mounts, environment, logs, OPA input,
  and CLI output.
- Concurrent processes share one selected Tobari.
- Repeated cluster and attach reconciliation does not grow owned resources.

## Interpretation rules

Agents must not infer:

- identity from a display name, container label, log order, or position;
- permission from a previous allow or similar path;
- exhaustive external history from complete CLI delivery;
- credential authority from a profile name;
- safety from an executable name;
- protection for files below a selected read-write root.

The opaque reference, declared local collection scope, cluster status, bounded
log coverage, OPA decision, and structured failures are the supported
interpretation inputs.

## Failure and recovery validation

At minimum, exercise:

- invalid name or root before Docker mutation;
- duplicate name and duplicate root;
- invalid, missing, incompatible, or conflicting image selection before Docker
  resource creation;
- malformed or legacy state;
- invalid Rego before cluster reconciliation;
- partial startup mapped to non-retryable `cluster_start_failed`;
- partial attach mapped to non-retryable `attach_failed`;
- non-empty cluster removal rejection;
- unknown or modified opaque ID before Docker calls;
- partial detach mapped to non-retryable `detach_failed`;
- explicit `--cwd` outside the selected root;
- child nonzero exit status;
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

Review the Gateway denial record, OPA watch event, two-network topology, and
Docker cleanup counts alongside command results.
