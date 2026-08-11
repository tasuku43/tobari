# Architecture

## System topology

```text
host
  tobari CLI ---- Docker CLI ---- Docker Engine
      +-- fixed control exec/stdin --> tobari-auth-broker (locked)
      |                                      |
      |                               encrypted Context vaults
      +-- reviewed fixed GitHub/AWS/pup login drivers --> provider HTTPS
      |       `-- AWS method: identity-center or console (explicit)
      +-- private same-binary credential companion
      |       |-- reviewed fixed AWS refresh driver --> provider HTTPS
      |       +-- encrypted reverse docker exec stream
      |                  --> broker-private bridge socket
      +-- root A (rw) --> Tobari A -- internal net A --+
      +-- root B (rw) --> Tobari B -- internal net B --+--> tobari-gateway
                                                               |      |      |
internal control network:                              tobari-opa :8181 | Unix runtime socket
                                                                      tobari-auth-broker
egress network:                                       Gateway and Auth Broker --> HTTPS
```

Each Tobari joins only its dedicated internal network. OPA joins only the
shared internal control network. Gateway joins every Tobari network plus
control and egress. Auth Broker joins control and egress but has no TCP
listener; Gateway reaches only its read-only mounted runtime Unix socket and
host commands reach only fixed control operations. The resident credential
companion opens no listener. It holds one fixed `docker exec -i` stream to the
verified Broker container, whose image-owned bridge byte-pumps to an unmounted
private Unix socket. Provider CLIs remain on the host and cannot be selected by
a Workspace, request, or owner manifest. Tobari and control networks
use Docker's `internal` property; the egress network is the only network with
an external route.

The AWS host driver registry is closed over two explicit acquisition methods.
`identity-center` retains schema-1 `aws_cli_sso` state and its fixed device
flow. `console` requires AWS CLI 2.32 or newer, uses fixed cross-device
`aws login --remote`, opens only its validated region-bound authorization URL
through the host browser boundary with terminal fallback, and stores distinct schema-2
`aws_cli_console_login` state. Broker accepts either ID as opaque encrypted
state; the companion decodes it, requires exact driver/state agreement, and
uses the shared fixed credential-export boundary after policy allow.

The Datadog registry contains one `datadog_pup_oauth` acquisition plan. It runs
fixed host pup against US1 in an isolated home and commits strict opaque state.
After policy allow, Broker resolves a valid token locally or refreshes only at
the exact US1 OAuth token endpoint; pup remains absent from the Broker image.

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

- `internal/domain`: pure cluster/Tobari/Auth Broker specifications, provider
  and result schemas, state, paths, Gateway/OPA schemas, operation effects, and
  validation.
- `internal/app`: lifecycle, status, entry, diagnostics, doctor, and
  Context-authentication use cases with consumer-owned ports.
- `internal/infra`: Docker CLI runner, local state/config filesystem, embedded
  asset materialization, provider projection, host root-key storage, broker
  control, private companion lifecycle/protocol, reviewed host credential
  drivers, platform inspection, and terminal/environment capability adapters.
- `internal/cli`: the canonical catalog, typed argv parsing, rendering, signal
  handoff, shared semantic style presentation, and composition root.

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
`policy reset`, `auth login`, `auth import`, `auth status`, and `auth logout`
remain valid
internal seams today. They are not permission to expose Docker, OPA, or opaque
resource identifiers as the routine mental model. `policy review` is the
ordinary human-facing Permission Inbox: on a TTY it composes selection, detail
inspection, explicit Allow-exact or Deny-exact staging, and one final Apply of
the complete reviewed set. Staging grants no authority; redirected and
machine-readable review remains read-only. Detail actions cannot mutate from
the list, and final Apply is a command-owned fixed-target mutation that
revalidates every unchanged opaque candidate ID. Single-reference `policy
allow` and `policy deny` remain machine and recovery actions. `policy rules` is the exhaustive current learned-decision inventory;
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
the Context policy directory, the managed-credential metadata/secret stores,
and the stable identity used to locate separately stored encrypted Auth Broker
state. The manifest contains no broker vault path, root key, or primary secret.
It may also own one fixed `runtime/Dockerfile` recipe and its last successful
managed build record plus narrow shell and Git identity policies. The manifest
is not itself a mountable authority: policy
is mounted only into OPA, managed secrets are mounted only into Gateway,
brokered secrets remain in encrypted Context vaults, and agent configuration is
mounted read-only into the work runtime. Tool-owned authentication remains in
the per-Workspace home; a configured broker provider projects only a
project-bound handle into it.

Credential ownership is Context-wide. Every project permanently bound to that
Context is eligible, but reconciliation issues a distinct project-bound handle
only on the project's next matching Workspace entry; no running process is
rewritten. Replacement revokes the previous revision. Logout removes the
Context/provider credential and all handles immediately, while the next entry
recreates the work container without the environment projection and removes
only unchanged Tobari-owned complete files.

Every Context has a stable UUIDv7 identity, and every logical Tobari permanently
binds one Context. `(canonical root, Context ID)` selects one record, so the
same root may have multiple Context-bound Tobari. `context use` changes only
the current/default Context used when an invocation omits a selector; it does
not mutate existing records or Docker. Project runtime reconciliation resolves
the stored Context ID and uses that Context's runtime image and agent profile.

Cluster state records one content-addressed projection revision and loaded
Context count, not an active enforcement Context. `cluster up` builds the
projection from all authoritative Context policy and credential sources,
validates each source and the whole candidate, publishes only a complete
owner-only directory, and starts exactly one Gateway, one OPA, and one locked
Auth Broker. Cluster reconciliation unlocks the broker through the host
root-key provider after its control endpoint is healthy, prepares a fresh
purpose-derived companion epoch, verifies the exact Broker container, and
starts exactly one private same-binary host companion. Companion health is
part of readiness; no companion is exposed through the public Catalog. Policy
mutations serialize this same all-Context activation and preserve the previous
known-good revision on any failure.
Cluster status schema 5 projects all three component states plus
`auth_provider_projection`, `auth_broker_state`, `root_key_backend`, and
always-present secret-free `credential_companion_state`
(`ready|prepared|absent|unavailable`). The companion field is host-process/
channel readiness, not a fourth Compose service
or credential state. Context report schema 7 exposes the complete Context
shell-environment and Git identity policies plus broker and installed-provider state without
returning a vault path/content, root key, primary secret, or handle. Public Linux backend values are `xdg_file`; the
infrastructure/doctor detail `linux_xdg_file` is not a public JSON enum. The
canonical schema/path/backend table is in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

Narrow host projection is a separate composition concern, not a file or secret
mount. Domain owns each fixed scalar inventory, `default|inherit|literal`
invariants, complete reports, and Git's atomic name/email pair. Context
application use cases own the fixed-target writes; the owner-only Context store
persists only non-default policy. CLI owns two input-completion modes for the
same use cases: complete typed argv, or a terminal-only staged editor when the
entire setting group is omitted. The editor first reads typed complete current
state, writes no mutation before Apply, binds the returned Context name across
the human review, sends every distinct staged Shell row through one application
call and one atomic manifest replacement,
and never completes a partial direct invocation. Explicit-empty Context input
is rejected rather than collapsed into the omitted current-Context selector.

Project runtime infrastructure resolves only declared shell `inherit` entries
from the launching process at child-exec time and passes exact values to Bash.
It always owns `PROMPT_COMMAND` so the chosen `PS1` survives image startup
behavior, but that hook is neither configurable nor inherited. Git inheritance
instead uses a purpose-built host adapter during stable-root reconciliation. It
runs at most two finite, bounded, one-attempt, fixed-key global-scope Git calls;
repository/worktree config cannot select another host file for that read. The
child receives only validated host config-directory paths and fixed controls,
never ambient `PATH`, loader, or shell-startup variables. A complete validated
pair is safely encoded after an `/etc/gitconfig` include in one size-bounded,
private per-Workspace system config. Its directory is mounted read-only and
selected by `GIT_CONFIG_SYSTEM`, leaving Workspace-global and repository-local
configuration at higher precedence. The report includes explicit `default`
state so presentation never infers policy from omitted manifest fields.

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
  authbroker/Dockerfile
  authbroker/broker.py
  authbroker/daemon.py
  authbroker/control.py
  authbroker/companion_protocol.py
  authbroker/companion_bridge.py
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
Context/project-principal registry and all-Context policy/provider projections,
and invokes Docker through the runtime port. Compose owns only Gateway, OPA,
Auth Broker, shared networks, and CA volumes. Cluster startup obtains the
verified Gateway and Auth Broker images by digest and the
official runtime base image through the runtime image resolver. The normal
resolver uses published images only; the contributor `tobari_dev` resolver
selects local development tags built by `task build:dev`. The runtime adapter
creates or reconciles each logical Tobari from its bound Context image, or an
exact configured local image on the legacy named path, and connects Gateway to
its dedicated network, then records the Gateway interface address in the
principal registry. Before project container creation, the runtime issues
configured provider handles and renders only manifest-declared environment or
complete-file projections. A public-only CA volume is mounted read-only into
each Tobari, whose entrypoint builds an ephemeral CA bundle.

The same resolver owns a pure build-identity projection used by `version` and
cluster preflight. The published implementation reads selected component APIs
from embedded `versions.env`; the `tobari_dev` implementation fixes them to the
canonical source APIs and proves contributor metadata through its build tag.
Neither implementation can consult CWD, project metadata, environment, or a
moving registry tag. Cluster preflight compares this projection before state
loading, asset materialization, journals, policy tests, or Docker calls.

Auth Broker follows the same canonical-source/snapshot pattern as Gateway. Its
editable Python package, Dockerfile, tests, and bridge/protocol source live
under `authbroker/`; the Go binary embeds the checked
snapshot at `internal/infra/runtimeassets/assets/authbroker/`.
`scripts/sync-authbroker-source.sh` refreshes the snapshot and
`scripts/check-authbroker-source.sh` rejects byte drift. The source and image
checks run the broker unit suite, prove that no provider CLI is installed, and
build the fixed non-root image. The main-only image workflow builds Linux amd64
and arm64 and publishes moving development tags plus an immutable commit tag.
Routine startup uses the reviewed multi-architecture digest in `versions.env`;
the moving tags never select runtime authority. The contributor resolver uses
`tobari-auth-broker:dev` for explicit local source validation.

The Codex/Claude broker source advances the Gateway image contract from
`io.tobari.gateway-api=3` to `4` and the Auth Broker image contract from
`io.tobari.auth-broker-api=2` to `3`. The Go preflight expects those newer
labels for this source tree, while the contributor resolver supplies the local
matching images. The already published immutable `versions.env` selections are
still Gateway API 3 and Auth Broker API 2; they remain historical runtime facts
and do not implement the new supplemental-header and dynamic OAuth protocol.
A routine published resolver cannot claim this capability until reviewed
immutable multi-architecture pins advance.

Compose mounts owner-only host state
`auth/contexts` at `/var/lib/tobari-auth/contexts` and `auth/runtime` at
`/run/tobari-auth/runtime`; control and companion sockets live on private tmpfs
mounts. The daemon listens on
`/run/tobari-auth/control/broker.sock` and
`/run/tobari-auth/runtime/broker.sock`. Control operations enter the container
through fixed `docker exec` argv and stdin. Gateway mounts only the runtime
directory read-only. The provider projection is generated atomically from the
built-in documents plus owner-only XDG user manifests and is mounted read-only
into Gateway; neither the projection nor a provider manifest contains a secret.

After unlock, the host creates a fresh companion epoch and derives a
purpose-separated session key from the installation root key; only that derived
key crosses the companion's inherited bootstrap stdin. The same executable is
started with a private process identity rather than a Catalog command. It holds
fixed `docker exec -i --user <uid:gid> <verified-container-id>` argv followed
by fixed `python3 -m authbroker.companion_bridge` container argv. The bridge
only pumps bounded bytes between
stdin/stdout and `/run/tobari-auth/companion/bridge.sock`; it parses, logs, and
persists nothing. A challenge handshake derives direction-specific AES-GCM
keys, and every frame carries exact monotonically increasing sequence state.
Authentication failure, replay, gap, oversized frame, duplicate session, or
disconnect closes the session. `cluster down` drains bounded in-flight work and
then closes the exec stream as Compose teardown removes the Broker container.

The reviewed GitHub, AWS, and pup drivers keep interactive provider-native execution on the
trusted host. Each resolves and hashes one host executable, uses fixed argv and
a sanitized environment, reconstructs only a private bounded temporary home,
and deletes it on every outcome. The GitHub driver recognizes only the fixed
device URL and requests no Git protocol. The AWS driver runs either the fixed
Identity Center device-code flow or the explicit fixed console cross-device
remote flow and later the fixed credential-export command; its cache bytes
return to Auth Broker only as opaque encrypted state. No URL, executable,
argument, environment key, or driver supplied by a provider manifest,
repository, Workspace, or request can alter that behavior. Request region is
separate Context/tool configuration and is not part of login state. Repository
`.git/config` and user-authored global Git/AWS configuration remain inside the
project/Workspace boundary. The
separate Context Git identity fallback contains only safely re-encoded
`user.name` and `user.email`; it grants no authentication or signing authority.

Custom images are supported only when they preserve runtime API label
`io.tobari.runtime-api=1`, the `tobari` image user, and the exact built-in
entrypoint, including the `io.tobari.runtime-lifetime-command=sleep infinity`
capability needed for Tobari's fixed Workspace lifetime command. The intended
construction is `FROM ghcr.io/tasuku43/tobari/runtime:latest`; a runtime-API label is a
compatibility assertion, not an image provenance or trust signature. The
selected image's `CMD` is ignored for Workspace lifetime: Docker create receives
the explicit `sleep infinity` command after the image. Tobari validates
compatibility before creating the project home, network, or container. Docker
create still supplies the invoking numeric UID/GID,
read-only root filesystem, dropped capabilities, fixed CPU/memory/PID/log
resource bounds, fixed mounts, proxy environment, internal network, and health
check.

`images/toolbox` remains an optional custom derivation for Context-specific
tools. A separate host task builds it from the official Tobari runtime base,
adds pinned `kubectl`, `cwk`, `pup`, and local-only TWG artifacts, and leaves
only the local `tobari-toolbox:local` tag. It inherits the base's existing
GitHub CLI and AWS CLI. It is not embedded runtime state, an official published
artifact, an Auth Broker layer, or an implicit cluster dependency. No inclusion
in this toolbox changes the public-base redistribution claim.

The first published family member follows the same layering: a main-branch
push runs `.github/workflows/runtime-base.yml` and publishes the reviewed
multi-architecture base to `ghcr.io/<owner>/tobari/runtime:latest` and `:main`
plus an
immutable `sha-<commit>` tag. Future agent variants use the same package with
variant-qualified tags such as
`ghcr.io/<owner>/tobari/runtime:codex.0.42.0-base.0.1.0-r1`. Pull requests and
ordinary local startup do not push images. Explicit `cluster up` may pull the
official runtime base for the selected uncustomized Context; an explicit
`runtime build` refreshes the exact official `runtime:latest` base when the
recipe starts from it, while explicit local or custom bases do not receive a
registry-pull request.

The first Claude and Codex variants are build-only children under
`runtimes/claude` and `runtimes/codex`. Each downloads a pinned official agent
release, verifies its per-architecture checksum, and inherits the base user,
entrypoint, and lifetime command. Codex uses the official standalone package,
which keeps its CLI companion binaries and Linux sandbox resources together.
Agent executables and package resources live in image-owned `/usr/local/bin`
and `/opt/tobari` paths; `/var/lib/tobari` contains only per-Tobari home state
and is safe to replace with the persistent home bind. Their workflows do not
publish agent tags until redistribution and license review is complete.

The root resolver obtains the desired image from the stored Context identity's
strict manifest on each runtime reconciliation. Before the default Context exists, the
owner-only XDG `config.json` `default_image` seeds that manifest; absence falls
back to `builtin`. The resolved selector, rather than the source of the
default, is persisted on the logical Tobari only as the last successful
runtime-container image. Project metadata is not consulted for runtime
selection.

Project runtime path mapping is owned by the Docker adapter. The selected root
is mounted read-write exactly once. If its canonical path is below the host
home, the adapter maps the host-home-relative suffix below `/var/lib/tobari`;
otherwise it uses the mirrored `/workspace` path. The per-Workspace home mount
is established before a nested project mount, and the runtime image contract
keeps executable and package assets in `/usr/local/bin` or `/opt/tobari`, not
below `/var/lib/tobari`.

The shared Gateway, OPA, and Auth Broker Compose services use fixed CPU, memory-plus-swap,
PID, and JSON-file log rotation bounds (`10m` per file, three files) so one
project cannot grow shared service resources without a cap. These are shared
service ceilings, not per-project fairness controls.

Project metadata is not a runtime adapter. Tobari does not interpret
`.devcontainer` files, invoke the Dev Container CLI, or transfer container
creation to a second orchestrator. The supported customization adapter is the
explicit current-Context `runtime init`/`runtime build` path: infrastructure
builds only the owner-only Context runtime directory, validates the resulting
image, and promotes it into the existing Context image field. Future runtime
import formats must attach to this same Context boundary rather than introduce
a second implicit image authority.

Runtime build diagnostics use two deliberately separate paths. The
application's optional build-progress port carries a bounded, validated stage
vocabulary and selection-state metadata for CLI presentation. A purpose-bound
writer carries visible-projected Docker/BuildKit stdout and stderr to host
stderr as the build runs. Upstream prose never becomes a structured fault
field or a source of promotion-state inference; infrastructure decides state
from completed build, compatibility, digest, and atomic manifest operations.

## Lifecycle model

The MVP owns one shared cluster `tool_local` target with stable ID
`cluster-default` and many CWD-owned logical Tobari records. The root index
stores a canonical root, stable Context ID, and stable internal ID at
`$XDG_STATE_HOME/tobari/roots/<hash>.json`; each instance owns
`instances/<id>/state.json` and `instances/<id>/home`. The instance record
contains the stable ID, canonical root, permanent Context binding, last reconciled image, profile, and
diagnostic container or network identifiers. Logical state, not Docker
inspection, defines whether a Tobari exists. Docker labels include:

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
Gateway image, Auth Broker image, and every required runtime image, builds and
tests the complete all-Context policy/provider projection, reconciles exactly
one OPA, one Gateway, and one Auth Broker, unlocks the broker, and
reconnects Gateway to every existing registered project network.
Image preflight fails before the policy test, cluster journal, shared network,
or service-container mutation. Local Tobari-managed image development uses
`task build:dev` and the `tobari_dev` image resolver instead of a public
cluster option.
Root invocation only verifies that configured cluster is ready and reads the
canonical CWD's indexed Workspace candidates. An exact current-root record is
selected directly; when only ancestor records exist, the CLI presents every
containing root nearest-first and the application accepts either one validated
candidate or an explicit create-at-CWD choice. The choice is revalidated under
the lifecycle lock before the selected logical record is created or reused. It
then resolves the selected record's root-scoped Context Git fallback before
Docker calls, resolves the bound Context image, requires the broker to be
ready, reconciles the Context's configured project-bound handle projection,
ensures one exact project network and
work container, connects Gateway with the `gateway` alias, binds the Gateway
interface address to the host-issued Context/project principal, waits for project
readiness, and enters the container. The
logical creation and deletion boundaries use durable journals so an interruption
between the home, instance, index, runtime, and deletion steps is recoverable
without treating a partial file set as a second project. Runtime convergence
stores a hash of the bound Context's desired image identity, mounts, security,
environment, health contract, fixed resource contract, and profile revision on
the project container; drift recreates only that container and updates the
stored project image only after success. OPA runs with `--watch --bundle`
against one read-only Docker-managed bundle volume. Source edits become authority only
after explicit whole-projection validation and activation. The principal registry is a generic host-issued
contract: Docker currently supplies the network/address observation, while a
future stronger runtime may supply the same binding through another adapter.
Only its dedicated directory is mounted read-only into Gateway; lifecycle
updates replace the registry file atomically inside that directory so a
single-file bind mount cannot strand Gateway on an old inode or expose the
neighboring credential configuration.
Logical Tobari and Context IDs are not trusted when echoed by a caller; Gateway
derives both from the local interface address. Exact allow, deny, and compaction
actions provide the deterministic portable activation path: each locks the
projection, tests the target Context's private source copy and the complete
all-Context candidate, verifies the exact OPA and bundle-volume ownership
labels, builds a revision-named archive through pinned OPA, atomically renames
it through a fixed networkless pinned publisher,
waits for the running OPA to report the exact expected revision, and rolls back
on failure. Reducing or mixed authority first activates a deny-all transition
revision.
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
Cluster removal is rejected until no instance record remains. Both `cluster
down` forms remove shared runtime resources while preserving encrypted Context
vaults and the installation root key. `--purge` additionally removes only the
shared CA and active policy-bundle volumes; cluster teardown is not credential
logout or revocation.

The root-index collection enforces one logical Workspace per `(canonical root,
Context ID)`. Its hash includes both values, and the project lock performs the
exact pair check again immediately before explicit creation. A repeated or
concurrent creation for one pair has one winner; a different Context may own a
separate stable Tobari, home, container, and network at the same root.
Parent/child roots may also overlap and run concurrently. Their host-file
effects are intentionally shared; the architecture does not add overlays,
checkout copies, root locks, or filesystem integrity isolation.

## Command catalog

`cli.Catalog` is the only registry for public paths, roles, effects, fixed
targets, inputs, outputs, failures, routing, and human/agent help. The root
operation is represented as a catalog-owned fixed current-directory target even
though it has no argv path words. Handlers receive parsed inputs and call one
application service. `tobari` and `delete` declare complete fixed-target
mutation impacts; `tobari` keeps its target fixed to the canonical CWD even
when its selected Workspace root is an ancestor. `status` resolves the same CWD
target. For those three lifecycle commands, the dispatcher normalizes the
prefix or command-local Context spelling into the command's one catalog input;
the typed parser rejects duplicates and explicit empty values. Application
resolves that selector to one validated manifest before CWD or Workspace
selection, and infrastructure receives the bound manifest rather than
rediscovering its display name. Status and force-delete preview retain that
stable Context identity through presentation and exact follow-up argv. `list`
reports IDs as diagnostic fields but no public lifecycle action consumes them.
The dependency-free terminal capability is an infrastructure
adapter used only by the CLI's human selector; a line-input fallback keeps
raw-mode availability out of the public command contract.

Catalog output fields recursively own every object child and array item,
including required/optional, nullable, enum, opaque-reference kind, and
semantic-scope facts. Object and field counts are bounded. The JSON renderer
validates its exact top-level envelope, schema version, nested keys, types,
enums, and nullability against that declaration before writing stdout; missing
and extra keys fail closed. Scoped agent-help schema 9 publishes the same
recursive declaration and exact success/error argv forms, including global
flag placement. Root help remains an index.

Interactive-only completion can remain a catalog entry with internal
visibility so the workflow uses the same typed composition authority. Internal
entries are excluded from public command copies, lookup, routing, reference
workflows, and help. Public projections also omit any interactive metadata that
would reveal an internal completion path.

The four auth commands share one catalog-declared `authentication.broker`
capability. Login, import, and logout are fixed-target writes to the one
installation credential catalog; status is a Context-scoped read. The
application resolves the explicit or current Context before the infrastructure
port, validates mutation intent and impact before acquisition/vault I/O, and
accepts import material only through the declared stdin input. Terminal import
stdin is rejected before reading. Non-terminal bytes are read after public
argument/intent/mutation validation; infrastructure then validates the selected
existing Context, installed provider/acquisition mode, and broker readiness
before broker send. Provider IDs are human selectors validated against the
installed projection; they are not opaque action references or credential
authority. Login may receive that selector directly or, when it is omitted,
the CLI reads the same typed Context status and presents only the closed set of
installed reviewed login providers on terminal stderr. The exact selected ID
and the resolved Context returned by that snapshot then enter the unchanged
login use case, preventing a concurrent default-Context change from retargeting
the reviewed choice. Redirected omission fails before the
status read or mutation, and an AWS method flag requires explicit `--provider`.

`doctor` composes bounded read-only environment, Docker, policy, provider,
root-key/vault, broker, and project-binding diagnostics. It reports all checks,
fails the command with `diagnostic_failed` when any check fails, and treats
warnings alone as healthy. It observes rather than repairs: it does not create a
root key, initialize or activate policy, start/reconcile/unlock the cluster, or
mutate provider, vault, credential, handle, or project-auth state.

Every catalog command that supports human text explicitly declares the shared
semantic-token presentation. The CLI presentation layer owns the exact
`text`, `muted`, `accent`, `success`, `warning`, and `danger` vocabulary and is
the only production location that maps those meanings to ANSI color or
emphasis. Command renderers select tokens by information meaning; they do not
own escape sequences or concrete colors. Infrastructure reports terminal
capability and the presence-only `NO_COLOR` environment preference. The CLI
combines those facts per output stream, keeping redirected and machine output
free of ANSI styling. Cursor-control sequences used by bounded interactive
selectors remain a separate terminal mechanism and do not define visual
styles.

Human text is built in three independent stages. Task-owned typed results first
select a `humanOutput` document containing headings, rows, sections, empty-state
scope/bounds, and exact recovery. The style projection may then add only the
six semantic ANSI tokens; it cannot select, remove, reorder, or rename document
content. The terminal interaction state machine independently owns cursor
movement, redraw eligibility, and restoration. Its reader absorbs idle VTIME
polls without returning a render event, while input, selection changes,
completion, and cancellation remain observable state transitions. Catalog
selection supplies bare-namespace normalization and deterministic typo
suggestions, so routing and recovery do not create a second command registry.

## Gateway request flow

```text
client request headers
  -> establish the host-issued Context/project principal at the header hook
  -> reject every malformed, misplaced, ambiguous, or binding-mismatched Tobari
     handle marker as credential_handle_invalid
  -> strictly recognize one valid broker handle from provider projection,
     remove it, and introspect its full non-secret binding
  -> only when no Tobari handle marker exists, select the trusted
     passthrough/managed adapter
  -> redact client authentication and cookie headers for OPA input
  -> classify only trusted Context-declared exact GraphQL endpoints
  -> for ordinary HTTP, normalize body-free OPA input at the header hook
  -> for a declared GraphQL endpoint, require and retain one bounded request,
     parse the selected operation and canonical root fields, and normalize only
     those derived protocol coordinates with the HTTP input
  -> POST one fixed decision endpoint with finite timeout
  -> route by trusted Context ID inside the Tobari-owned OPA package
  -> require every declared GraphQL root coordinate and any broker-provider
     request to have its exact learned L7 rule rather
     than inheriting a broad static host/method allow
  -> deny on any invalid/unavailable decision
  -> on a static brokered allow, resolve the same revision exactly once and
     replace only the declared destination header
  -> on an AWS SigV4 allow, keep the request bounded and unstreamed, then hash
     the same complete request; Broker uses one post-policy companion export,
     rechecks/persists the revision, and signs through the reviewed plan
  -> on a Datadog OAuth allow, Broker selects the same-revision access token or
     performs one same-record refresh at the exact proxy-free, no-redirect US1
     token endpoint, commits refreshed state, and returns one bearer value;
     Gateway validates the same revision, replaces only the declared
     destination header, and makes one upstream attempt
  -> otherwise strip control headers, then forward client authentication or
     apply the managed profile once after allow
  -> enable ordinary request-body streaming; forward an allowed buffered
     GraphQL body or a signed bounded AWS body once
  -> resolve and pin the upstream address; reject unsafe dotted-host results
  -> stream the authorized upstream response from its headers
  -> emit redacted audit JSON
```

Ordinary body content is deliberately opaque to policy. Ordinary bodies are
neither retained nor hashed by Gateway. A trusted exact GraphQL endpoint is a
narrow pre-policy exception: Gateway requires a positive length no greater
than 1 MiB, parses one strict UTF-8 JSON request, and sends only operation type
and sorted canonical root fields to OPA. The original bytes are forwarded once
after allow; source text, operation name, aliases, fragment names, directives,
nested selections, arguments, variables, extensions, literal values, and body
hashes never enter policy, audit, learned state, or CLI output. The reviewed AWS signing plan temporarily
retains and hashes only an already-authorized request within the fixed body cap;
the bytes never enter policy, audit, logs, vault state, or retry state. Client authentication can be present on the forwarded
request but is absent from OPA input and audit output. No query or headers are
emitted in audit. Audit retains the path component, except that any path
containing a Tobari handle marker becomes `/[redacted-auth-handle]`. Structural
URL/header handle rejections are non-learnable and cannot become policy
candidates. Any Tobari-looking handle marker either enters the exact valid
broker route or fails as `credential_handle_invalid`; only complete marker
absence permits configured fallback. A valid candidate is removed before
broker or OPA I/O and is never forwarded on failure. Broker
introspection returns no secret; policy denial performs no resolution or
companion call. The
default passthrough adapter never loads or injects managed credentials; the
retained managed adapter performs Context/project/host validation and
injection at the same post-allow boundary. The addon never retries.

Denied audit records are also the policy-development feedback interface. A
learnable Gateway denial carries a fixed host-side `tobari policy review`
navigation hint, and session closure may summarize the pending queue on host
stderr. These are advisory only: they contain no action reference and cannot
approve or retry a request. `tobari cluster denials` parses one bounded Gateway
log window, rejects
malformed denial-shaped records, and returns typed Context and project principal, host, port,
method, path, optional GraphQL operation/root coordinate, reason, status,
exact-rule learnability, request identity, timestamp, the
trusted host policy directory, and the exact review command. OPA computes
learnability only when version, cluster, Context, scheme, fixed port, project-principal,
and (for the managed adapter) credential-binding boundaries already pass, so an exact
Context/project/host/port/method/path rule, plus the GraphQL coordinate when
present, can close the request. `policy review` and
`policy candidates` deterministically fold only that eligible retained evidence
by the structured Context/project/host/port/method/path and optional GraphQL
effect key. They emit one opaque
exact-rule reference, the latest evidence, and an observation count for each
pending effect; references remain stable across repeated denials. This pure
read projection also converges concurrent identical audit records without a
second persisted inbox or write race. They remove effects already covered by
the CLI-owned learned allow or deny data and trusted baseline deny rules.
Baseline denies remain audit-only. `policy review` is the routine human text
workflow: it stages explicit decisions over unchanged opaque candidate IDs for
one Context and applies the complete typed set once. Apply or discard precedes
switching Context, keeping source promotion to one atomic file replacement;
redirected review is read-only.
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

Each authoritative Context policy data source is schema 2. Guided Contexts
contribute only that data; aggregate generation loads the current shared Rego
evaluator and tests from Tobari's embedded runtime assets. Advanced Context
source targets input schema 4, may retain source schema 3 for compatibility,
is rewritten to Gateway runtime input schema 5, and rejects any other shape. The aggregate
projection is schema 1 and stores those sources below
`tobari_contexts[context_id]`; the Tobari-owned router is the only
`tobari.http` decision entrypoint. `boundary.authorities` and
`boundary.methods` describe the configured request boundary,
`boundary.graphql_endpoints` declares exact protocol-classification points, `boundary.ports`
describes the scheme-specific candidate transport boundary, and `rules` keeps
baseline denies, learned allows, and learned denies in separate collections.
Gateway and OPA share this structure; the CLI only owns the mutation of the
two learned collections and never rewrites the host-authored boundary.

Guided Contexts use one current Tobari-owned evaluator, projected once, with
Context-specific authorities, methods, ports, GraphQL endpoints, baseline decisions, learned
decisions, and credential metadata supplied as data. Advanced source retains
the editable `package tobari.http` source contract, but projection rewrites it
to `tobari.contexts.c<uuid>.http`. Validation rejects source that claims the
cluster router, `tobari.system`, another Context package, or
`data.tobari_contexts`. This is namespace and routing enforcement within one
OPA process, not a claim of Rego process-level confidentiality.

The cluster router sends every input carrying a GraphQL coordinate through the
current Tobari-owned system evaluator, even for an Advanced Context; the
Context's data still supplies its endpoint and rules. Only ordinary HTTP input
routes to editable Advanced Rego. This prevents older or custom policy source
from ignoring the new coordinate and accidentally authorizing it through one
coarse HTTP rule.

`policy deny` resolves the same exact candidate reference and appends one
Context/project-bound exact deny rule through the same aggregate preflight, atomic-write, and OPA
activation boundary. Exact denies are terminal and win over learned allows for
the same Context/project/host/port/method/path and optional GraphQL coordinate.

Compaction discovery is pure over current ordinary HTTP learned rules. GraphQL
rules are exact-only and never enter compaction. It groups at least
three exact rules only when Context, project, host, port, method, and a sufficiently deep
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
Auth mutations use the same structured-outcome rule. A failed or cancelled
GitHub, AWS, or pup host driver leaves the previous Context credential unchanged; an
`auth_mutation_outcome_unknown`, `unclassified_mutation_outcome`, or
`mutation_output_write_failed` result is non-retryable and directs the user to
`auth status` before another auth mutation. Confirmed login/import/logout output
is finalized before late cancellation can imply that replay is safe.

## Architecture enforcement

- Go architecture lint preserves layer direction and a thin `cmd/tobari`.
- Domain and application tests prove path, state, effect, and orchestration
  invariants without Docker.
- Infrastructure tests use a recording command runner.
- Gateway and Rego tests cover policy boundaries.
- Auth Broker, root-key, provider, and Gateway integration tests cover locked
  startup, encrypted vaults, project-bound handles, deny-before-resolution,
  rotation/revocation, and fallback adapters.
- Companion and host-driver tests cover private same-binary startup, exact
  reverse-exec argv/container identity, authenticated framing, no-listener/
  no-mount topology, fixed GitHub/AWS/pup CLI commands, post-policy refresh,
  bounded single-flight state update, encrypted no-replay barrier,
  cancellation settlement, blocked-peer teardown, and stale-result rejection.
- Docker integration tests prove actual network topology and lifecycle.
