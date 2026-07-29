# Agent Readiness Validation

Tobari is agent-ready when a coding agent can discover and execute the Realm
workflow without source inspection, infer no authority from display text, and
recover from a denied network request using only declared outputs.

## Required scenario

Run the following against a clean Docker Engine with a synthetic root:

```sh
go run ./cmd/tobari help --format agent
go run ./cmd/tobari help up --format agent
go run ./cmd/tobari doctor --root /absolute/test/root --format json
go run ./cmd/tobari up --root /absolute/test/root
go run ./cmd/tobari status --format json
go run ./cmd/tobari exec -- curl https://example.com/
go run ./cmd/tobari logs --component gateway --tail 100
go run ./cmd/tobari down --purge
```

The transcript must prove:

- Root agent help is a compact outcome/capability index.
- Exact-command help supplies complete inputs, outputs, prerequisites, effects,
  fixed-target facts, failures, and recovery commands.
- `up` binds one command-owned Realm singleton and one canonical root.
- Status retains task identity and explicit configured/running state.
- `exec` passes exact argv and preserves the child status.
- A denied request produces a bounded secret-free audit record containing the
  host, method, path, decision, and reason needed for policy refinement.
- Recovery points to an exact catalog command or scoped help selector.
- Cleanup selects only exact ownership-labeled resources.

## Policy-learning scenario

The Docker integration test supplies the executable policy-development loop:

1. A mock-host GET is allowed.
2. A mock-host POST under `/denied` receives `403` and never reaches upstream.
3. `logs --component gateway` exposes the rejected host, method, path, and
   reason without the body or credential canary.
4. The editable policy path comes from `status`.
5. Rego format and unit tests run before `up` reloads OPA.
6. The user retries the workload; no rule is generated or accepted
   automatically.

Routine success and routine denial require zero undeclared parsers,
provider-notation decoders, source inspection steps, or exploratory provider
calls.

## Network and credential scenario

`task integration:test` additionally proves:

- Realm has no direct egress and cannot reach OPA.
- HTTPS is authorized after CONNECT interception and validates the Tobari CA
  without an insecure client flag.
- OPA and Gateway outages fail closed.
- A profile is injected only after allow and exact host binding.
- The managed secret is absent from Realm files, mounts, environment, Gateway
  logs, OPA input, and CLI output.
- Concurrent processes share the same Realm.
- Repeated `up` does not grow owned resources.

## Interpretation rules

Agents must not infer:

- identity from a container label, log order, or display position;
- permission from a previous allow or from a similar path;
- exhaustive provider history from complete CLI delivery;
- credential authority from a profile name;
- safety of a command from its executable name;
- protection for files below the selected read-write root.

The catalog's exact fixed target, task-owned status fields, declared bounded log
coverage, OPA decision, and structured failures are the only supported
interpretation inputs.

## Failure and recovery validation

At minimum, exercise:

- missing/invalid root before Docker mutation;
- conflicting root;
- invalid Rego before OPA reload;
- partial startup mapped to non-retryable `realm_start_failed` with `status`
  reconciliation;
- partial cleanup mapped to non-retryable `realm_stop_failed` with `status`
  reconciliation;
- Realm not configured for `shell`, `exec`, and `logs`;
- explicit `--cwd` outside root;
- child nonzero exit status;
- OPA unavailable and malformed decision;
- output write failure without partial structured data.

A post-mutation raw adapter error must never become replay permission. Valid
structured outcome faults are preserved without private causes; unknown
outcomes collapse to a non-retryable contract fault.

## Evidence

Completion evidence is:

```sh
task check
task runtime:test
task security
task public:check
```

Review the emitted Gateway denial record and Docker cleanup counts alongside
the command results. A passing unit suite without real container traffic is not
agent-readiness evidence for Tobari.
