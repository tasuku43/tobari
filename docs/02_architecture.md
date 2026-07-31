# Architecture

## System topology

```text
host
  tobari CLI ---- Docker CLI ---- Docker Engine
      |
      +-- root A (rw) --> Tobari A -- internal net A --+
      +-- root B (rw) --> Tobari B -- internal net B --+--> tobari-gateway
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
- `internal/app`: lifecycle, status, entry, diagnostics, and doctor use cases with
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

The root ensure operation materializes exact embedded bytes under the Tobari state directory,
writes generated non-secret runtime configuration, and invokes Docker through
the runtime port. Compose owns only Gateway, OPA, shared networks, and CA
volumes. The built-in image receives an asset-version tag and the stable local
extension tag `tobari-runtime:local`. The runtime adapter creates each logical
Tobari from the built-in image or an exact configured local image and
connects Gateway to its dedicated network. No runtime asset is downloaded
during startup. A public-only CA volume is mounted read-only into
each Tobari, whose entrypoint builds an ephemeral CA bundle.

Custom images are supported only when they preserve runtime API label
`io.tobari.runtime-api=1`, the `tobari` image user, and the exact built-in
entrypoint. The intended construction is `FROM tobari-runtime:local`; a
runtime-API label is a compatibility assertion, not an image provenance or
trust signature. Docker create still supplies the invoking numeric UID/GID,
read-only root filesystem, dropped capabilities, fixed mounts, proxy
environment, internal network, and health check.

`images/toolbox` is the optional reference derivation for repeated
network-facing CLI exercises. A separate host task builds it from
`tobari-runtime:local`, verifies pinned official artifacts, checks its runtime
metadata and tool executability, and leaves only the local
`tobari-toolbox:local` tag. It is not embedded runtime state, a published
artifact, or an implicit cluster dependency.

The root resolver obtains an image from bounded project metadata or the strict
owner-only XDG `config.json` `default_image`; absence before first initialization
falls back to `builtin`. The resolved selector, rather than the source of the
default, is persisted on the logical Tobari.

An explicit Dev Container path is resolved after the root and must remain
inside it after symlink evaluation. Infrastructure reads at most 256 KiB,
normalizes JSON-with-comments and trailing commas, and rejects duplicate keys.
It returns typed image metadata to application; application rejects every
top-level property outside `image`, `$schema`, `name`, and `customizations`.
The selected literal image then uses the same local compatibility inspection.
Tobari does not invoke the Dev Container CLI or transfer container creation to
a second orchestrator.

## Lifecycle model

The MVP owns one shared cluster `tool_local` target with stable ID
`cluster-default` and many CWD-owned logical Tobari records. The root index
stores a canonical root and stable internal ID at
`$XDG_STATE_HOME/tobari/roots/<hash>.json`; each instance owns
`instances/<id>/state.json` and `instances/<id>/home`. The instance record
contains the stable ID, canonical root, selected image, profile, and diagnostic
container or network identifiers. Logical state, not Docker inspection,
defines whether a Tobari exists. Docker labels include:

```text
io.tobari.owner=default
io.tobari.component=tobari|gateway|opa
io.tobari.id=<stable ID when applicable>
io.tobari.role=work|network
io.tobari.version=<asset revision>
```

Root invocation resolves the canonical CWD and nearest indexed ancestor. When
there is no record it atomically creates an ID, root index, state record, and
home; when a record exists it retains those values. It then validates
configuration, tests policy, reconciles OPA and Gateway, ensures one exact
network and work container, connects Gateway with the `gateway` alias, and
enters the container. OPA runs with `--watch` against a read-only XDG bind, so
host edits reload when Docker-host filesystem events propagate. `policy apply`
provides the deterministic portable path: it tests the current bind, verifies
the exact OPA ownership label, recreates only OPA, and waits for health.
`delete` verifies owner, ID, and role labels before removing the selected
container and network, then removes only its XDG home and records. Container
or network loss is reconciled by the root operation; it never deletes logical
state. Cluster removal is rejected until no instance record remains.

## Command catalog

`cli.Catalog` is the only registry for public paths, roles, effects, fixed
targets, inputs, outputs, failures, routing, and human/agent help. The root
operation is represented as a catalog-owned fixed current-directory target even
though it has no argv path words. Handlers receive parsed inputs and call one
application service. `tobari` and `delete` declare complete fixed-target
mutation impacts; `status` resolves the same CWD target. `list` reports IDs as
diagnostic fields but no public lifecycle action consumes them.

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

Denied audit records are also the policy-development feedback interface.
`tobari cluster denials` parses one bounded Gateway log window, rejects
malformed denial-shaped records, and returns typed host, method, path, reason,
status, exact-rule learnability, request identity, timestamp, the trusted host
policy directory, and the exact apply command. OPA computes learnability only
when version, cluster, scheme, and credential binding already pass, so an exact
host/method/path rule can close the request. `policy candidates`
deterministically maps only that eligible retained evidence to opaque
exact-rule references that remain stable across repeated denials of the same
host/method/path, and removes effects already covered by the CLI-owned
learned-rule data. `policy tail` is a human text projection over the same
bounded task result. Raw `cluster logs` remains the component-debugging
interface.

`policy allow` resolves one exact candidate reference against retained
validated audit state without decoding it. Infrastructure reads the bounded,
owner-only `data.json`, preserves non-owned members, appends one deterministic
exact learned rule, tests a private complete policy copy, atomically replaces
the data file, and calls the existing OPA activation boundary.

Compaction discovery is pure over current learned rules. It groups at least
three exact rules only when host, method, and a sufficiently deep directory
prefix agree. The opaque proposal binds the exact source-rule set.
`policy compact` resolves that current proposal, replaces only those sources
with one prefix rule retaining the positive examples, runs rule-match boundary
canaries and the full OPA suite, then uses the same atomic write and activation
path. Compaction discovery and prefix evaluation reject encoded separators,
backslashes, empty segments, and dot segments rather than generalizing across
ambiguous upstream normalization. A changed source set makes the proposal stale
rather than silently recomputing its meaning.

## Docker abstraction

Application code owns narrow ports such as `ResolveOrCreate`, `EnsureRuntime`,
`EnterRuntime`, `Inspect`, and `Delete`. The MVP infrastructure implementation invokes the Docker
CLI with fixed command structures and caller context. This keeps Docker Engine
API or Podman replacement possible without promising either today. Arbitrary
shell strings are never constructed; user commands are passed as argv after
Docker's `--`.

## Cancellation and errors

The command root installs signal-aware cancellation and propagates one context.
Pre-execution cancellation makes zero Docker calls. A child interactive session
exit status is preserved. Lifecycle operations return structured state after
confirmed completion; unclassified post-mutation errors are non-retryable and
direct the user to `status` for reconciliation.

## Architecture enforcement

- Go architecture lint preserves layer direction and a thin `cmd/tobari`.
- Domain and application tests prove path, state, effect, and orchestration
  invariants without Docker.
- Infrastructure tests use a recording command runner.
- Gateway and Rego tests cover policy boundaries.
- Docker integration tests prove actual network topology and lifecycle.
