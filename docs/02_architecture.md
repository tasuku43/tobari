# Architecture

## System topology

```text
host
  tobari CLI ---- Docker CLI ---- Docker Engine
      |
      +-- selected root (rw) --------------------+
                                                  v
internal realm network:                    tobari-realm
  tobari-realm <---- HTTP proxy :8080 ---- tobari-gateway
                                               |
internal control network:                     |---- tobari-opa :8181
                                               |
egress network:                                +---- HTTPS upstream
```

Realm joins only the internal realm network. OPA joins only the internal
control network. Gateway joins realm, control, and egress. The Realm and
control networks use Docker's `internal` property; the egress network is the
only network with an external route.

For HTTPS, Realm connects to the HTTP proxy and sends `CONNECT host:443`.
Gateway responds, terminates client TLS with the installation CA, evaluates the
decrypted HTTP request, then creates a separate TLS connection to the upstream.
This is the explicit HTTPS flow documented by mitmproxy; it is not plaintext
HTTP from Realm to the final service.

## Four-layer dependency direction

```text
internal/cli  ------> internal/app
      |                    |
      |                    v
      +------------> internal/domain <------ internal/infra
```

- `internal/domain`: pure runtime specifications, state, paths, Gateway/OPA
  schemas, operation effects, and validation.
- `internal/app`: lifecycle, status, exec, logs, and doctor use cases with
  consumer-owned ports.
- `internal/infra`: Docker CLI runner, local state/config filesystem, embedded
asset materialization, and platform inspection.
- `internal/cli`: the canonical catalog, typed argv parsing, rendering, signal
  handoff, and composition root.

Domain performs no I/O. Application imports neither infrastructure nor CLI.
Infrastructure satisfies ports structurally without importing application.
CLI is the only production composition root. `tools/archlint` enforces these
directions.

## Runtime assets

The Go binary embeds a versioned runtime tree:

```text
runtime/
  compose.yaml
  realm/Dockerfile
  realm/entrypoint.sh
  gateway/Dockerfile
  gateway/addon/tobari_gateway.py
  gateway/config.example.json
  opa/policy/tobari.rego
  opa/policy/tobari_test.rego
```

`up` materializes exact embedded bytes under the Tobari state directory, writes
generated non-secret runtime configuration, and invokes Docker through the
runtime port. No runtime asset is downloaded during startup. The Gateway CA and key
persist in a named volume; a separate volume exposes only the public
certificate to Realm, whose entrypoint builds an ephemeral CA bundle.

## Lifecycle model

The MVP owns one `tool_local` target with stable ID `realm:default`. Runtime
state records schema version, canonical host root, resource names, policy path,
proxy endpoint, and asset revision. Docker labels include:

```text
io.tobari.owner=default
io.tobari.component=realm|gateway|opa
io.tobari.version=<asset revision>
```

`up` validates configuration, tests policy, reconciles exact resources, starts
OPA and Gateway before Realm, and waits for health. A conflicting root is an
input failure. On an existing Realm, `up` restarts only OPA after successful
policy tests and waits for health so host policy edits take effect without
terminating Realm work. `down` verifies the ownership label on each exact resource
before removing it. Persistent home is preserved unless `--purge` is supplied.

## Command catalog

`cli.Catalog` is the only registry for public paths, roles, effects, fixed
targets, inputs, outputs, failures, routing, and human/agent help. Handlers
receive parsed inputs and call one application service. The catalog declares
`up` as create and `down` as write with complete mutation intent and impact.
All other commands are reads; `shell` and `exec` act on the fixed running realm.

## Gateway request flow

```text
client flow
  -> strip inbound secret headers
  -> buffer bounded body once
  -> normalize OPA input
  -> POST decision with finite timeout
  -> deny on any invalid/unavailable decision
  -> validate optional credential profile + exact host
  -> inject secret inside Gateway
  -> forward once
  -> emit redacted audit JSON
```

The same buffered bytes inspected by policy are forwarded. JSON is structured
only when complete and within the limit. The addon never retries.

Denied audit records are also the policy-development feedback interface:
`tobari logs --component gateway` exposes bounded host, method, path, decision,
and reason metadata; `status` exposes the trusted host policy directory. No
automatic rule generation or permission expansion occurs.

## Docker abstraction

Application code owns narrow ports such as `Ensure`, `Inspect`, `Exec`,
`Logs`, and `Remove`. The MVP infrastructure implementation invokes the Docker
CLI with fixed command structures and caller context. This keeps Docker Engine
API or Podman replacement possible without promising either today. Arbitrary
shell strings are never constructed; user commands are passed as argv after
Docker's `--`.

## Cancellation and errors

The command root installs signal-aware cancellation and propagates one context.
Pre-execution cancellation makes zero Docker calls. A child `exec` exit status
is preserved. Lifecycle operations return structured state after confirmed
completion; unclassified post-mutation errors are non-retryable and direct the
user to `status` for reconciliation.

## Architecture enforcement

- Go architecture lint preserves layer direction and a thin `cmd/tobari`.
- Domain and application tests prove path, state, effect, and orchestration
  invariants without Docker.
- Infrastructure tests use a recording command runner.
- Gateway and Rego tests cover policy boundaries.
- Docker integration tests prove actual network topology and lifecycle.
