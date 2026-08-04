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

## User-facing composition

The internal separation between shared cluster reconciliation, project runtime
reconciliation, and policy activation must not become a setup tax for the
ordinary user. The product front door is the current project directory and the
bounded-autonomy outcome: enter a reusable isolated space, let the agent work,
and teach the minimum network permission after a useful denial. The CLI may
orchestrate or guide these internal steps in a future slice, but every such
path remains catalog-owned, effect-declared, and failure-before-side-effect
where applicable.

`cluster up`, `cluster status`, `cluster denials`, `policy candidates`,
`policy review`, `policy tail`, `policy allow`, `policy deny`, `policy rules`,
and `policy reset` remain valid
internal seams today. They are not permission to expose Docker, OPA, or opaque
resource identifiers as the routine mental model. `policy review` is the
ordinary human-facing Permission Inbox: on a TTY it composes selection,
detail inspection, explicit confirmation, and the existing `policy allow` or
`policy deny` action for one candidate; redirected and machine-readable review remains
read-only. `policy rules` is the exhaustive current learned-decision inventory;
on a TTY it composes selection, detail inspection, explicit reset confirmation,
and `policy reset` for one current decision, while redirected and
machine-readable inventory remains read-only. `policy candidates` is the
machine discovery surface and `policy tail` is a compatibility projection. The
catalog declares this
composition while preserving discover/act separation: the act still consumes
exactly one validated opaque reference or one declared fixed target.

### Context composition

Context is the user-facing composition layer for the execution boundary. A
trusted manifest names the compatible runtime image, read-only agent profile,
the Context policy directory, and the managed-credential metadata/secret stores.
It may also own one fixed `runtime/Dockerfile` recipe and its last successful
managed build record. The manifest is not itself a mountable authority: policy
is mounted only into OPA, managed secrets are mounted only into Gateway, and
agent configuration is mounted read-only into the work runtime. Tool-owned
authentication remains in the per-Workspace home.

The first implementation has one active Context for the shared cluster. Cluster
state records the active Context and resolved paths, and project runtime
reconciliation uses its agent-profile reference and digest. `context use` is a
host mutation and the owner of the complete selection outcome: when the shared
cluster is running, it reuses the bounded `cluster up` reconciliation path and
does not succeed until the selected policy and Gateway credential mounts are
health-checked and persisted. When the cluster is stopped or unconfigured, it
updates only the host marker and reports that explicit `cluster up` is needed;
it never starts Docker implicitly. A reconcile journal is written before the
marker changes, and failure restores the previous marker/state when possible;
an unresolved journal or active-context mismatch blocks entry and policy paths.
Per-Workspace Context routing is deferred because it would require explicit OPA
routing, learned-rule scope, and project-principal decisions.

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

The canonical Tobari base-image source now lives under `runtimes/base`. Its
Dockerfile and bootstrap are copied into the embedded
`internal/infra/runtimeassets/assets/tobari` snapshot by the explicit
`scripts/sync-runtime-base.sh` maintainer operation, and
`task runtime:base:check` fails if the snapshot drifts. The base metadata,
digest lock, and family manifest are kept beside the source image. This keeps
the distributed CLI self-contained while avoiding two independently edited
base definitions.

Gateway follows the same monorepo pattern but has its own release unit. The
canonical Dockerfile, addon, entrypoint, and tests live under `gateway/`; the
Go binary embeds a checked snapshot at
`internal/infra/runtimeassets/assets/gateway/`. The explicit
`scripts/sync-gateway-source.sh` operation refreshes that snapshot and
`scripts/check-gateway-source.sh` rejects drift. Compose and OPA remain under
`internal/infra/runtimeassets/assets` because they are CLI-owned orchestration
and policy inputs, not Gateway image contents. The main-only Gateway workflow
builds the canonical source for Linux amd64 and arm64 and publishes moving
development tags plus an immutable commit tag to GHCR. The embedded
`versions.env` records one reviewed immutable Gateway digest for routine
startup; moving tags are never consumed by the CLI.

The root ensure operation materializes exact embedded bytes under the Tobari state directory,
writes generated non-secret runtime configuration, including the owner-only
project-principal registry, and invokes Docker through
the runtime port. Compose owns only Gateway, OPA, shared networks, and CA
volumes. The built-in image receives an asset-version tag and the stable local
extension tag `tobari-runtime:local`; this tag is the local base work runtime
with the common tools shared by supported agents. Cluster startup obtains the
verified Gateway image by digest, while `cluster up --gateway-source` builds
the embedded snapshot only when explicitly requested. The runtime adapter
creates each logical Tobari from the built-in image or an exact configured
local image and connects Gateway to its dedicated network, then records the
Gateway interface address in the principal registry. A public-only CA volume is mounted read-only into
each Tobari, whose entrypoint builds an ephemeral CA bundle.

Custom images are supported only when they preserve runtime API label
`io.tobari.runtime-api=1`, the `tobari` image user, and the exact built-in
entrypoint, including the `io.tobari.runtime-lifetime-command=sleep infinity`
capability needed for Tobari's fixed Workspace lifetime command. The intended
construction is `FROM tobari-runtime:local`; a runtime-API label is a
compatibility assertion, not an image provenance or trust signature. The
selected image's `CMD` is ignored for Workspace lifetime: Docker create receives
the explicit `sleep infinity` command after the image. Tobari validates
compatibility before creating the project home, network, or container. Docker
create still supplies the invoking numeric UID/GID,
read-only root filesystem, dropped capabilities, fixed CPU/memory/PID/log
resource bounds, fixed mounts, proxy environment, internal network, and health
check.

`images/toolbox` remains a transitional optional custom derivation for tools
outside the common base set, such as kubectl and TWG. A separate host task
builds it from `tobari-runtime:local`, verifies pinned official artifacts, and
leaves only the local `tobari-toolbox:local` tag. It is not embedded runtime
state, an official published artifact, or an implicit cluster dependency.

The first published family member follows the same layering: a main-branch
push runs `.github/workflows/runtime-base.yml` and publishes the reviewed
multi-architecture base to `ghcr.io/<owner>/tobari/runtime:latest` and `:main`
plus an
immutable `sha-<commit>` tag. Future agent variants use the same package with
variant-qualified tags such as
`ghcr.io/<owner>/tobari/runtime:codex.0.42.0-base.0.1.0-r1`. Pull requests and
ordinary local startup do not push or pull images; an explicit `runtime build`
refreshes the exact official `runtime:latest` base when the recipe starts from
it, while explicit local or custom bases do not receive a registry-pull
request.

The first Claude and Codex variants are build-only children under
`runtimes/claude` and `runtimes/codex`. Each downloads a pinned official agent
release, verifies its per-architecture checksum, and inherits the base user,
entrypoint, and lifetime command. Codex uses the official standalone package,
which keeps its CLI companion binaries and Linux sandbox resources together.
Agent executables and package resources live in image-owned `/usr/local/bin`
and `/opt/tobari` paths; `/var/lib/tobari` contains only per-Tobari home state
and is safe to replace with the persistent home bind. Their workflows do not
publish agent tags until redistribution and license review is complete.

The root resolver obtains the image from the active Context's strict manifest.
Before the default Context exists, the owner-only XDG `config.json`
`default_image` seeds that manifest; absence falls back to `builtin`. The
resolved selector, rather than the source of the default, is persisted on the
logical Tobari. Project metadata is not consulted for runtime selection.

Project runtime path mapping is owned by the Docker adapter. The selected root
is mounted read-write exactly once. If its canonical path is below the host
home, the adapter maps the host-home-relative suffix below `/var/lib/tobari`;
otherwise it uses the mirrored `/workspace` path. The per-Workspace home mount
is established before a nested project mount, and the runtime image contract
keeps executable and package assets in `/usr/local/bin` or `/opt/tobari`, not
below `/var/lib/tobari`.

The shared Gateway and OPA Compose services use fixed CPU, memory-plus-swap,
PID, and JSON-file log rotation bounds (`10m` per file, three files) so one
project cannot grow shared service resources without a cap. These are shared
service ceilings, not per-project fairness controls.

Project metadata is not a runtime adapter. Tobari does not interpret
`.devcontainer` files, invoke the Dev Container CLI, or transfer container
creation to a second orchestrator. The supported customization adapter is the
explicit active-Context `runtime init`/`runtime build` path: infrastructure
builds only the owner-only Context runtime directory, validates the resulting
image, and promotes it into the existing Context image field. Future runtime
import formats must attach to this same Context boundary rather than introduce
a second implicit image authority.

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

Interactive session attachment is a separate, transient process state. The
runtime adapter starts the work container with the infrastructure-owned
`sleep infinity` lifetime process independently of the selected image `CMD`,
then enters it through one `docker exec -i -t ... /bin/bash` child session.
Exact commands use the same child-exec boundary. The state model is:

```text
Workspace absent
  -> tobari
Attached session + Workspace exists
  -> exit
Detached session + Workspace exists
  -> tobari
Attached session + Workspace exists
  -> exit
Detached session + Workspace exists
  -> tobari delete
Workspace absent
```

The child shell's `exit`, or a nonzero exit from an exact agent command, ends
only the exec process; it does not stop or delete the work container, logical
instance, root index, or per-Workspace home. There is no persisted stopped or
paused state. Detached `delete` removes the logical
Workspace; an attached exec makes ordinary deletion fail, and `delete --force`
is the explicit host-side override.

Explicit `cluster up` validates configuration, obtains and preflights the
Gateway image, tests policy, reconciles OPA and Gateway, and reconnects Gateway
to every existing registered project network. Image preflight fails before the
policy test, cluster journal, shared network, or service-container mutation;
the source-build flag is the only deliberate local-build path.
Root invocation only verifies that configured cluster is ready and reads the
canonical CWD's indexed Workspace candidates. An exact current-root record is
selected directly; when only ancestor records exist, the CLI presents every
containing root nearest-first and the application accepts either one validated
candidate or an explicit create-at-CWD choice. The choice is revalidated under
the lifecycle lock before the selected logical record is created or reused. It
then ensures one exact project network and work container, connects Gateway
with the `gateway` alias, binds the Gateway interface address to the host-issued
project principal, waits for project readiness, and enters the container. The
logical creation and deletion boundaries use durable journals so an interruption
between the home, instance, index, runtime, and deletion steps is recoverable
without treating a partial file set as a second project. Runtime convergence
stores a hash of the desired image identity, mounts, security, environment,
health contract, fixed resource contract, and profile revision on the project
container; drift recreates only that container. OPA runs with
`--watch` against a read-only XDG bind, so host edits reload when Docker-host
filesystem events propagate. The principal registry is a generic host-issued
contract: Docker currently supplies the network/address observation, while a
future stronger runtime may supply the same binding through another adapter.
Only its dedicated directory is mounted read-only into Gateway; lifecycle
updates replace the registry file atomically inside that directory so a
single-file bind mount cannot strand Gateway on an old inode or expose the
neighboring credential configuration.
The logical Tobari ID is not trusted when echoed by a caller; Gateway derives
it from the local interface address. Exact allow, deny, and compaction actions
provide the deterministic portable activation path: each tests its complete
private policy copy, verifies the exact OPA ownership label, recreates only OPA,
and waits for health.
`delete` observes active Docker exec IDs before ordinary removal, then verifies
owner, ID, and role labels before removing the selected container and network;
an attached exec rejects ordinary deletion and `--force` skips only that guard.
It then removes only its XDG home and records. Container
or network loss is reconciled by the root operation; it never deletes logical
state. Cluster status and reconcile derive project counts and network joins
from the indexed CWD-owned records rather than the legacy named collection.
Status and list expose an `incomplete` runtime diagnostic when an indexed root
has lost its instance record; root entry refuses to rebuild that record and
delete remains the exact cleanup path.
Cluster removal is rejected until no instance record remains.

The root-index collection enforces one logical Workspace per canonical root.
The root hash names the index file, and the project lock performs the exact-root
check again immediately before explicit creation. A repeated or concurrent
creation therefore has one winner and returns `already exists` to the other
callers rather than creating duplicate state.

## Command catalog

`cli.Catalog` is the only registry for public paths, roles, effects, fixed
targets, inputs, outputs, failures, routing, and human/agent help. The root
operation is represented as a catalog-owned fixed current-directory target even
though it has no argv path words. Handlers receive parsed inputs and call one
application service. `tobari` and `delete` declare complete fixed-target
mutation impacts; `tobari` keeps its target fixed to the canonical CWD even
when its selected Workspace root is an ancestor. `status` resolves the same CWD
target. `list` reports IDs as diagnostic fields but no public lifecycle action
consumes them. The dependency-free terminal capability is an infrastructure
adapter used only by the CLI's human selector; a line-input fallback keeps
raw-mode availability out of the public command contract.

## Gateway request flow

```text
client flow
  -> select trusted credential adapter (passthrough by default)
  -> redact client authentication and cookie headers for OPA input
  -> buffer bounded body once
  -> normalize the schema-2 OPA input
  -> reject an unavailable body as ambiguous
  -> POST decision with finite timeout
  -> deny on any invalid/unavailable decision
  -> require the initialized empty-body boundary
  -> validate the host-issued project principal and adapter request context
  -> strip only proxy and Tobari control headers after allow
  -> resolve and pin the upstream address; reject unsafe dotted-host results
  -> adapter forwards client authentication or applies the managed profile
     once after allow
  -> emit redacted audit JSON
```

The same buffered bytes inspected by policy are forwarded. JSON is structured
only when complete and within the limit. Client authentication can be present
on the forwarded request but is absent from OPA input and audit output. The
default passthrough adapter never loads or injects managed credentials; the
retained managed adapter performs the existing project/host validation and
injection at the same post-allow boundary. The addon never retries.

Denied audit records are also the policy-development feedback interface. A
learnable Gateway denial carries a fixed host-side `tobari policy review`
navigation hint, and session closure may summarize the pending queue on host
stderr. These are advisory only: they contain no action reference and cannot
approve or retry a request. `tobari cluster denials` parses one bounded Gateway
log window, rejects
malformed denial-shaped records, and returns typed project principal, host, port,
method, path, reason, status, exact-rule learnability, request identity, timestamp, the
trusted host policy directory, and the exact review command. OPA computes
learnability only when version, cluster, scheme, fixed port, project-principal,
and (for the managed adapter) credential-binding boundaries already pass, so an exact
project/host/port/method/path rule can close the request. `policy review` and
`policy candidates` deterministically map only that eligible retained evidence
to opaque exact-rule references that remain stable across repeated denials of
the same project/host/port/method/path, and remove effects already covered by
the CLI-owned learned allow or deny data and trusted baseline deny rules.
Baseline denies remain audit-only. `policy review` is the routine human text
projection and, after explicit TTY confirmation, delegates one unchanged
candidate ID to `policy allow` or `policy deny`; redirected review is read-only.
`policy rules` is the exhaustive current inventory of CLI-owned learned Allows
and exact Denies. `policy reset --id` removes exactly one such decision through
the same preflight, atomic-write, and OPA activation boundary, leaving the
matching effect at default deny so the retained denial can enter `policy review`
again. It never edits baseline policy, grants permission, or retries a request.
`policy tail`
is a compatibility projection over the same bounded task result. Raw `cluster
logs` remains the component-debugging interface.

`policy allow` resolves one exact candidate reference against retained
validated audit state without decoding it. Infrastructure reads the bounded,
owner-only `data.json`, preserves non-owned members, appends one deterministic
exact learned rule, tests a private complete policy copy, atomically replaces
the data file, and calls the existing OPA activation boundary.

The active policy data is schema 2. `boundary.authorities` and
`boundary.methods` describe the configured request boundary, `boundary.ports`
describes the scheme-specific candidate transport boundary, and `rules` keeps
baseline denies, learned allows, and learned denies in separate collections.
Gateway and OPA share this structure; the CLI only owns the mutation of the
two learned collections and never rewrites the host-authored boundary.

`policy deny` resolves the same exact candidate reference and appends one
project-bound exact deny rule through the same preflight, atomic-write, and OPA
activation boundary. Exact denies are terminal and win over learned allows for
the same project/host/port/method/path.

Compaction discovery is pure over current learned rules. It groups at least
three exact rules only when project, host, port, method, and a sufficiently deep
directory prefix agree. The opaque proposal binds the exact source-rule set.
`policy compact` resolves that current proposal, replaces only those sources
with one prefix rule retaining the positive examples, runs rule-match boundary
canaries and the full OPA suite, then uses the same atomic write and activation
path. Compaction discovery and prefix evaluation reject encoded separators,
backslashes, empty segments, and dot segments rather than generalizing across
ambiguous upstream normalization. A changed source set makes the proposal stale
rather than silently recomputing its meaning.

## Docker abstraction

Application code owns narrow ports such as `ResolveOrCreate`, `EnsureRuntime`,
`EnterRuntime`, `Inspect`, and `Delete`. The MVP infrastructure implementation
invokes the Docker CLI with fixed command structures and caller context. This
keeps Docker Engine API or Podman replacement possible without promising either
today. Arbitrary shell strings are never constructed; user commands are passed
as argv after Docker's `--`.

## Cancellation and errors

The command root installs signal-aware cancellation and propagates one context.
Pre-execution cancellation makes zero Docker calls. A child interactive session
exit status is preserved, and the CLI emits the session-closed/resume/delete
guidance on host stderr after the child returns. Child stdout remains owned by
the interactive process. Lifecycle operations return structured state after
confirmed completion; unclassified post-mutation errors are non-retryable and
direct the user to `status` for reconciliation.

## Architecture enforcement

- Go architecture lint preserves layer direction and a thin `cmd/tobari`.
- Domain and application tests prove path, state, effect, and orchestration
  invariants without Docker.
- Infrastructure tests use a recording command runner.
- Gateway and Rego tests cover policy boundaries.
- Docker integration tests prove actual network topology and lifecycle.
