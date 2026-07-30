# Architecture

## System topology

```text
host
  tobari CLI ---- Docker CLI ---- Docker Engine
      |
      +-- root A (rw) --> named Tobari A -- internal net A --+
      +-- root B (rw) --> named Tobari B -- internal net B --+--> tobari-gateway
                                                               |         |
internal control network:                              tobari-opa :8181  |
                                                                         |
egress network:                                               HTTPS upstream
```

Each Tobari joins only its dedicated internal network. OPA joins only the
shared internal control network. Gateway joins every Tobari network plus
control and egress. Tobari and control networks use Docker's `internal`
property; the egress network is the only network with an external route.

For HTTPS, Tobari connects to the HTTP proxy and sends `CONNECT host:443`.
Gateway responds, terminates client TLS with the installation CA, evaluates the
decrypted HTTP request, then creates a separate TLS connection to the upstream.
This is the explicit HTTPS flow documented by mitmproxy; it is not plaintext
HTTP from Tobari to the final service.

## Four-layer dependency direction

```text
internal/cli  ------> internal/app
      |                    |
      |                    v
      +------------> internal/domain <------ internal/infra
```

- `internal/domain`: pure cluster/Tobari specifications, state, paths, Gateway/OPA
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
  tobari/Dockerfile
  tobari/entrypoint.sh
  gateway/Dockerfile
  gateway/addon/tobari_gateway.py
  gateway/config.example.json
  opa/policy/tobari.rego
  opa/policy/tobari_test.rego
```

`cluster up` materializes exact embedded bytes under the Tobari state directory,
writes generated non-secret runtime configuration, and invokes Docker through
the runtime port. Compose owns only Gateway, OPA, shared networks, and CA
volumes. The built-in image receives an asset-version tag and the stable local
extension tag `tobari-runtime:local`. The runtime adapter creates each named
Tobari from the built-in image or an exact user-selected local image and
connects Gateway to its dedicated network. No runtime asset is downloaded
during startup or attach. A public-only CA volume is mounted read-only into
each Tobari, whose entrypoint builds an ephemeral CA bundle.

Custom images are supported only when they preserve runtime API label
`io.tobari.runtime-api=1`, the `tobari` image user, and the exact built-in
entrypoint. The intended construction is `FROM tobari-runtime:local`; a
runtime-API label is a compatibility assertion, not an image provenance or
trust signature. Docker create still supplies the invoking numeric UID/GID,
read-only root filesystem, dropped capabilities, fixed mounts, proxy
environment, internal network, and health check.

Attach resolves an explicit image first. When omitted, infrastructure reads the
strict owner-only XDG `config.json` and uses `default_image`; absence before
first initialization falls back to `builtin`. The resolved selector, rather
than the source of the default, is persisted on the Tobari.

An explicit Dev Container path is resolved after the root and must remain
inside it after symlink evaluation. Infrastructure reads at most 256 KiB,
normalizes JSON-with-comments and trailing commas, and rejects duplicate keys.
It returns typed image metadata to application; application rejects every
top-level property outside `image`, `$schema`, `name`, and `customizations`.
The selected literal image then uses the same local compatibility inspection.
Tobari does not invoke the Dev Container CLI or transfer container creation to
a second orchestrator.

## Lifecycle model

The MVP owns one cluster `tool_local` target with stable ID `cluster-default`.
Schema-2 runtime state records shared resource names, policy path, proxy
endpoint, asset revision, and a finite list of named Tobari. Every Tobari record
contains an opaque ID, canonical root, selected image, and exact container,
network, and volume names. Older schema-2 records without the additive image
field mean `builtin`. Docker labels include:

```text
io.tobari.owner=default
io.tobari.component=tobari|gateway|opa
io.tobari.tobari-id=<opaque id when applicable>
io.tobari.version=<asset revision>
```

`cluster up` validates configuration, tests policy, reconciles OPA and Gateway,
and waits for health. OPA runs with `--watch` against a read-only XDG bind, so
host edits reload without a lifecycle mutation. `attach` requires the cluster,
validates name/root uniqueness, creates exact labeled resources, connects
Gateway with the `gateway` alias, and waits for health. `detach` verifies owner
and Tobari-ID labels before removing exact resources. Persistent home is
preserved unless `--purge` is supplied. Cluster removal is rejected until the
Tobari list is empty.

## Command catalog

`cli.Catalog` is the only registry for public paths, roles, effects, fixed
targets, inputs, outputs, failures, routing, and human/agent help. Handlers
receive parsed inputs and call one application service. The catalog declares
cluster and attach/detach mutations with complete intent and impact. `list` is
the sole `tobari_id` producer; individual actions consume one exact ID. No
action selects by display name.

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
`tobari cluster logs --component gateway` exposes bounded host, method, path,
decision, and reason metadata; `cluster status` exposes the trusted host policy
directory. No
automatic rule generation or permission expansion occurs.

## Docker abstraction

Application code owns narrow ports such as `EnsureCluster`, `Attach`, `Inspect`,
`Exec`, `Logs`, and `Detach`. The MVP infrastructure implementation invokes the Docker
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
