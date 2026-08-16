# Architecture

## System topology

```text
host
  tobari CLI ---- Docker CLI ---- Docker Engine
      +-- root A (Context-selected ro/rw) --> Tobari A -- guarded net A --+
      +-- root B (Context-selected ro/rw) --> Tobari B -- guarded net B --+--> tobari-gateway
                                                                      |
internal control network:                                      tobari-opa :8181
egress network:                              Gateway --> policy-allowed HTTPS
```

Each Tobari joins only its dedicated internal network. OPA joins only the
shared internal control network. Gateway joins every Tobari network plus
control and egress. Standard has no Auth Broker service, provider projection,
credential mount, or host helper. The experimental development override adds a
locked Auth Broker on control/egress plus its private runtime socket and trusted
host acquisition boundary. Tobari and control networks
use Docker's `internal` property; the egress network is the only network with
an external route.

The host adapter uses a fixed one-shot helper to configure only a verified
Gateway or Workspace network namespace. The helper receives root plus
`CAP_NET_ADMIN`, no mounts or secrets, and exits before entry. The Workspace
guard installs an exact default route through Gateway and rejects unexpected
on-link, UDP, and IPv6 paths. The Gateway guard redirects project TCP and DNS
to local listeners, keeps IPv4/IPv6 forwarding disabled, and drops its forward
chain. Neither resident process retains a network capability, and no host or
Docker-VM-global firewall state is changed.

Standard reviewed login drivers are the closed GitHub, pup, Codex, and Claude
set. The experimental compile-time profile additionally activates the reviewed
AWS driver; runtime data cannot change profile membership.
The shared host resolver for GitHub, AWS, and Codex rejects the first temporary, project-local, or
home-local PATH shadow without executing it and inspects a finite PATH-ordered
candidate set for the first canonical executable under an existing trusted
installation root. Each hashes that accepted executable outside the project, runs
fixed argv with a sanitized private state boundary, accepts only its bounded
browser/PTY/output contract, rechecks executable identity, and performs checked
cleanup. Codex additionally requires a stable observed product identity and
the exact compiled host-login/state contract. The verified child owns its
native loopback callback listener, browser request, PKCE state, callback, and
token exchange; Tobari owns none of those surfaces and validates only the
resulting private file state after exit. Its stream boundary recognizes
only the reviewed reset, muted, and accent SGR vocabulary, maps those meanings
to Tobari-owned styles for an interactive terminal, and emits no style when
`NO_COLOR` is present; unknown controls use the ordinary visible projection.
Pup and Claude instead run from the selected Context image in fresh
mount-free containers. Pup binds the immutable image ID, observes bounded
semantic version syntax and executable digest without a compiled version
allowlist, then requires the fixed login, status, and native-state capture
contract. Claude runs exact 2.1.220 from the selected Context image in a fresh
mount-free, project-free login container. Tobari hashes the copied executable
bytes on the host, validates the native OSC 8 authorization URL, and maps only
the exact opening, link, browser-result, and paste-prompt events to a fixed
Tobari UI. The no-newline prompt is emitted immediately; Claude owns its
non-echoing terminal input, and Tobari preserves the next real child status
line. Browser success hides the long URL, and opener failure retains the exact
validated manual fallback. The login container replaces the ordinary
Workspace entrypoint, whose CA wait requires a deliberately absent mount, with
fixed `/usr/bin/tini -- /usr/bin/sleep infinity`; this keeps PID 1 alive for the
bounded acquisition without weakening the mount-free boundary. All fixed and control-safe
pass-through lines use explicit CRLF while the child owns the raw Docker TTY;
they do not depend on terminal newline post-processing to return to column one.
Exact cursor hide/show events are consumed and closed with one Tobari-owned
cursor-show. The prompt includes fixed guidance that authorization may take a
moment after Enter, and successful child exit produces a fixed progress
transition before credential capture. The original terminal reader reaches
Docker unchanged so the child retains TTY identity. Tobari then extracts
access token, refresh token, expiry, the dynamically granted scope set, and the
non-secret subscription-type and rate-limit-tier labels
from the Linux file. It structurally validates OAuth scope tokens, requires the
grant to be a subset of the exact set observed in the authorization URL,
normalizes that set to Tobari order, canonicalizes those values with the
observed executable identity into its own V1 record, discards all other
provider-owned optional metadata, and removes the container before commit.
Workspace receives those two bounded labels and the scope set beside its
project-bound access handle, plus a fixed non-secret refresh sentinel needed
by the exact pinned client's local login-state check; the real renewable
session remains Broker-only. Workspace
reconciliation separately merges only the reviewed non-secret top-level
`hasCompletedOnboarding` boolean into Claude's private state, preserves all
other fields, and removes that field with the Anthropic projection. The merge
is unavailable to owner manifests. Owner
manifests remain static-only and cannot select these helpers or their dynamic
plans. Managed stores remain absent.

For HTTPS, ordinary DNS receives a bounded
synthetic non-public IPv4 answer and the direct TCP connection is redirected
to Gateway's local transparent listener. Gateway requires one consistent SNI
and HTTP authority, terminates client TLS with the installation CA, evaluates
the same decrypted HTTP request, and only after allow replaces the synthetic
destination, resolves and pins it, and creates a separate certificate-verified
TLS connection to the upstream. The path does not send plaintext HTTP over the
final Internet hop, and synthetic DNS never performs an external lookup.

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
  control, reviewed host credential drivers and companion, platform inspection, and
  terminal/environment capability adapters.
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
`policy review`, `policy allow`, `policy deny`, `policy rules`,
`policy reset` remains a valid standard seam. `auth login`, `auth import`,
`auth status`, and `auth logout` remain experimental-only seams
internal seams today. They are not permission to expose Docker, OPA, or opaque
resource identifiers as the routine mental model. `policy review` is the
ordinary human-facing Permission Inbox: on a TTY it composes selection, typed
exact-or-template detail inspection, explicit template-Allow or exact staging,
and one final Apply of
the complete reviewed set. Staging grants no authority; redirected and
machine-readable review remains read-only. Detail actions cannot mutate from
the list, and final Apply is a command-owned fixed-target mutation that
revalidates every unchanged opaque review-item ID and reconstructs every
template proposal from fresh pending evidence and current exact rules.
Single-reference `policy
allow` and `policy deny` remain machine and recovery actions. `policy rules` is the exhaustive current learned-decision inventory;
on a TTY it composes selection, detail inspection, explicit reset confirmation,
and `policy reset` for one current decision, while redirected and
machine-readable inventory remains read-only. `policy candidates` is the
machine discovery surface. The catalog declares this
composition while preserving discover/act separation: the act still consumes
exactly one validated opaque reference or one declared fixed target.

### Context composition

Context is the user-facing immutable capability envelope for the execution
boundary. A trusted manifest fixes direct source access and a normalized
policy-preset origin/revision, and names the compatible runtime image, read-only agent profile,
the Context policy directory, and its stable identity. The standard manifest
locates no Auth Broker state. Experimental state remains separately keyed and
the manifest contains no broker vault path, root key, or primary secret.
It may also own one fixed `runtime/Dockerfile` recipe and its last successful
managed build record plus narrow shell and Git identity policies. The manifest
is not itself a mountable authority: policy
is mounted only into OPA and agent configuration is mounted read-only into the
work runtime. Tool-owned authentication remains in the per-Workspace home.
Only the experimental profile has encrypted Context vaults and projects a
project-bound handle.

Experimental Broker credential ownership is Context-wide. Every project permanently bound to that
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

Context and project stores expose separate observation and ensure/mutation
paths. Observation never initializes directories, manifests, active markers,
vault state, policy, or lock files. A missing omitted Context becomes a typed
display-only synthetic default. Persisted state must match the exact V1
contract; only a validated create/write path may re-read under its mutation
lock and atomically initialize it. Project observation opens an existing
lock when present and otherwise remains lockless. A pre-existing validated
project journal is the narrow exception: observation acquires the recovery
lock, completes bounded cleanup, and then reads the remaining logical state.

Cluster state records one content-addressed projection revision and loaded
Context count, not an active enforcement Context. Standard `cluster up` builds
the projection from all authoritative Context policy sources,
validates each source and the whole candidate, publishes only a complete
owner-only directory, and starts exactly one Gateway and one OPA. Experimental
composition adds the provider projection and one locked Auth Broker.
Reconciliation is not ready on process health alone: OPA must
serve both the exact content-addressed aggregate revision and its decision
document. An already-ready identical revision is not republished. Cluster
reconciliation unlocks the broker through the host
root-key provider after its control endpoint is healthy and verifies the exact
Broker container. Policy
mutations serialize this same all-Context activation and preserve the previous
known-good revision on any failure.
Standard cluster status schema 1 projects Gateway and OPA. Experimental status
additionally projects Auth Broker, `auth_provider_projection`,
`auth_broker_state`, and `root_key_backend`.
Context report schema 1 exposes explicit Context
persistence state, nullable pre-authority ID/stores, the complete Context
shell-environment and Git identity policies plus `native_workspace`
authentication mode. Experimental output adds broker and installed-provider state without
returning a vault path/content, root key, primary secret, or handle. Public Linux backend values are `xdg_file`; the
infrastructure/doctor detail `linux_xdg_file` is not a public JSON enum. The
canonical schema/path/backend table is in
[Authentication handling](07_authentication.md#experimental-canonical-schemas-paths-and-backend-identifiers).

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

Context creation is the sole owner of envelope defaults. It resolves omitted
source access to `read-write` and omitted preset to
`builtin/agent-ready`, normalizes and validates the complete preset, binds
its owner-only snapshot by SHA-256 revision, and atomically persists both
manifest and snapshot before returning. Observation and old-state readers never
invent either field. Root entry carries `source_access` into the exact project
runtime spec/hash; policy activation carries the preset snapshot into the
Tobari-owned system evaluator. A read-only source is the same live direct bind
with Docker read-only authority: no writable alias is added, home and tmpfs
remain writable, and host or same-root read-write Context changes remain
observable. Neither path rediscovers the source preset.

The system evaluator owns terminal guardrail precedence before baseline deny,
exact learned deny, baseline grant, exact or reviewed single-segment-template
learned allow, or Advanced Rego.
Terminal denial ends before candidate projection, external DNS, broker
resolution, and upstream I/O. Advanced modules may further constrain generic
input but cannot bypass the guardrail or redefine the scheme-aware exact
learned identity.
`builtin/agent-ready` uses the reviewed-exact guardrail plus a finite exact
baseline coupled to Claude Code 2.1.220 and Codex 0.147.0. The baseline grants
model execution, bootstrap/catalog, account-state, and fixed first-party
telemetry effects to the Context principal; it does not identify a process.
Exact Deny precedes that grant. Optional plugins, MCP, connectors, file
transfer, downloads, evaluation routes, self-update, and unmatched effects
continue into terminal denial or the ordinary review evaluator.
`builtin/offline` terminally denies all HTTP/HTTPS and exposes no review
candidate. `builtin/reviewed-exact` exposes only eligible effects to exact
review. `builtin/get-only-reviewed` exposes only eligible GET effects to exact
review and terminally denies HEAD and all non-GET methods; GET receives no safe
or read-only classification. Those three strict presets grant nothing
immediately.

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

Gateway follows the same monorepo pattern and is part of the CLI release unit. The
canonical Dockerfile, addon, entrypoint, and tests live under `gateway/`; the
Go binary embeds a checked snapshot of only the Dockerfile, `.dockerignore`,
and Dockerfile-declared image inputs at
`internal/infra/runtimeassets/assets/gateway/`. The explicit
`scripts/sync-gateway-source.sh` operation refreshes that snapshot and
`scripts/check-gateway-source.sh` rejects byte, membership, or Docker `COPY`
drift. Tests and contributor documentation stay canonical-only rather than
inflating the distributed CLI or its development image identity. Compose and OPA remain under
`internal/infra/runtimeassets/assets` because they are CLI-owned orchestration
and policy inputs, not Gateway image contents. Pull-request workflows validate
the canonical Linux amd64/arm64 build with cache-only output. No workflow
publishes a Tobari-owned image.

The root ensure operation materializes exact embedded bytes under the Tobari state directory,
writes generated non-secret runtime configuration, including the owner-only
Context/project-principal registry and all-Context policy projection,
and invokes Docker through the runtime port. Standard Compose owns only Gateway,
OPA, shared networks, and CA volumes. Cluster startup ensures the verified
Gateway and agent-ready runtime images from embedded pinned recipes under
source-derived local tags, building each only when absent. The release resolver
is `embedded`; the development resolver selects its own source-hash local tags.
The runtime adapter
creates or reconciles each logical Tobari from its bound Context image and connects Gateway to
its dedicated network. After it has reconciled the Workspace guard, it records
the exact owned Workspace and Gateway endpoints in the schema-1 principal
registry. Before project container creation, the runtime issues
configured provider handles and renders only manifest-declared environment or
complete-file projections. A public-only CA volume is mounted read-only into
each Tobari, whose entrypoint builds an ephemeral CA bundle.

The same resolver owns a pure build-identity projection used by `version` and
cluster preflight. Both implementations fix APIs to canonical source and derive
image tags from embedded source bytes; release packaging injects no image
authority.
Neither implementation can consult CWD, project metadata, environment, or a
moving registry tag. Cluster preflight compares this projection before state
loading, asset materialization, journals, policy tests, or Docker calls.

Experimental Auth Broker follows the same canonical-source/runtime-input pattern as Gateway. Its
editable Python package, Dockerfile, tests, and bridge/protocol source live
under `authbroker/`; the Go binary embeds the checked Docker build inputs, not
the tests or contributor documentation, at
`internal/infra/runtimeassets/assets/authbroker/`.
`scripts/sync-authbroker-source.sh` refreshes the snapshot and
`scripts/check-authbroker-source.sh` rejects byte, membership, or Docker
`COPY` drift. The source and image
checks run the broker unit suite, prove that no provider CLI is installed, and
build the fixed non-root image. It is not published. The experimental
contributor resolver uses a source-hash local tag.

Both canonical sources declare component API V1. Source records only reviewed
parent inputs; generated owned-image outputs never enter `versions.env` or
release packaging.

Compose mounts owner-only host state
`auth/contexts` at `/var/lib/tobari-auth/contexts` and `auth/runtime` at
`/run/tobari-auth/runtime`; the control socket lives on private tmpfs. The
daemon listens on
`/run/tobari-auth/control/broker.sock` and
`/run/tobari-auth/runtime/broker.sock`. Control operations enter the container
through fixed `docker exec` argv and stdin. Gateway mounts only the runtime
directory read-only. The provider projection is generated atomically from the
built-in documents plus owner-only XDG user manifests. Its dedicated parent
directory is mounted read-only into Gateway so atomic replacement selects a new
inode without recreating the service; neither the projection nor a provider
manifest contains a secret. Gateway caches a validated projection only while
its complete stat identity remains unchanged and fails closed, without a
last-known-good fallback, when a replacement is invalid.

The reviewed host-driver implementation union keeps GitHub, AWS, and Codex provider-native execution
on the trusted host. Each resolves and hashes one canonical executable from
conventional non-project trusted installation roots, uses only its fixed argv
and sanitized private state, and deletes temporary state on every outcome.
The Auth Broker domain owns the complete implementation provider-ID vocabulary,
the immutable standard or experimental active subset, and the
presentation-ordered reviewed-login subset with its exact helper binding.
The application service, CLI input enum, embedded-manifest loader, and fixed
infrastructure driver table derive from or prove parity with that closed
vocabulary. These immutable projections do not provide runtime registration;
the infrastructure table continues to own executable dispatch and payload
validation for each compiled driver.

Inside the Auth Broker, reviewed renewable bearer sessions use a second closed
compile-time registry keyed by the persisted credential kind. Its provider-local
adapters own only state parsing, freshness, the fixed refresh transport,
refreshed-state validation, and typed supplemental values. Broker core retains
handle and binding validation, the immutable record snapshot, per-record
single-flight lock, durable outcome-unknown barrier, provider-call ordering,
compare-and-swap, Vault commit, rotation, and revocation. Adapters receive no
Vault, root-key, handle-index, lock, barrier, executable, or registration
capability. AWS request signing and its trusted-host companion remain a
separate reviewed capability rather than optional methods on a common Provider
interface. Its closed mechanics registry owns only SigV4 request parsing,
companion request/result correlation, bounded temporary-credential decoding,
and request signing. Broker core retains the companion call, per-record lock,
durable no-replay barrier, snapshot comparison, Vault CAS, and response
framing; the mechanics object receives none of those authorities. Host
acquisition likewise remains a distinct trusted-host boundary
and shares only the existing strict credential-state envelope with Broker
runtime resolution.

The Anthropic envelope is deliberately not a copy of Claude's private file
schema. The acquisition boundary depends on four exact renewable fields and
two bounded non-secret entitlement labels from the pinned executable, while
Broker persists only a strict Tobari-owned record and supplies its reviewed
fixed client ID during refresh. All other additive or account-local Claude
metadata is discarded. Subscription and rate-limit values are structurally
bounded provider output, not compiled value lists, and cannot widen Broker
behavior. Scope names are likewise absent from the compiled contract: Broker refresh uses
the persisted canonical set and rejects a changed refresh response, while the
same non-secret set and the two native-client entitlement labels are projected
beside the Workspace handle. A fixed non-secret refresh sentinel prevents the
pinned client from misreporting that projection as expired but is never
recognized or resolved as a Broker handle.

The persisted credential-record union is also separated from Vault storage
authority. Closed record contracts own credential-kind membership, exact V1
record and binding shapes, bounded secret/state encoding, and record
construction. `VaultStore` alone owns root-key use, AES-GCM envelopes,
Context-associated data, owner/mode validation, bounded file reads, and atomic
replacement. Record contracts receive no key, path, file descriptor, cipher,
or persistence callback. Static records deliberately accept any validated
provider ID because strict owner manifests remain the supported declarative
extension boundary; AWS, Datadog, OpenAI, and Anthropic records bind one exact reviewed
provider and binding shape.

The control-socket login vocabulary is a third closed projection with only two
capability shapes: bounded static-secret login and bounded reviewed driver-state
login. Its immutable plans own exact request keys, payload-length field,
reviewed driver IDs, credential kind, and fixed record constructor. Dispatcher
parses through that union and Broker core performs one driver-state commit
lifecycle: Context validation, optional renewable-state validation, record
construction, Vault save, and prior-handle revocation. A plan receives neither
`BrokerState` nor `VaultStore` and cannot execute the host helper. The Go
trusted-host acquisition drivers remain on the other side of the control
socket and are not implementations of this protocol plan.

Gateway keeps its own closed, non-secret credential-profile registry for the
exact Anthropic, AWS, Datadog, and OpenAI projection constraints. Profiles see
only already parsed projection values and may validate fixed normalized
bindings, Workspace projections, helper identity, renewable classification,
and typed supplemental response metadata. They receive no HTTP request,
credential value, Broker caller, OPA client, socket, or mutation callback.
Generic schema validation continues to accept strict owner-authored static
providers. Gateway core continues to own candidate recognition, removal of all
secret-sensitive headers before introspection and policy, deny-before-resolution,
one same-revision post-allow action, and exact final header/signature application.

These Go and Python projections deliberately do not share a production
registry or generated adapter interface. A versioned test-only reviewed
provider capability fixture records only the translations that must agree
across component boundaries. Go tests bind provider IDs, acquisition mode and
helper, public manifest credential kind, and login membership to the domain
registry and embedded built-ins. Python tests bind control-login shape,
persisted record kind, renewable/signing/supplemental capabilities, and Gateway
profile membership to the compiled Broker and Gateway registries. In
particular, public `primary_secret` intentionally maps to persisted
`static_primary_secret`; identical spelling is not required where the trust
boundary owns a different representation. No production component reads this
fixture, and updating it cannot register a provider or grant adapter authority.

GitHub recognizes only its fixed device URL, requests one host-browser open at
most once, retains the exact manual fallback, and requests no Git protocol.
OpenAI delegates browser opening and the dynamic authorization URL entirely to
the verified Codex child; Tobari visibly projects but never parses, reconstructs,
or opens that URL. Claude recognizes only its exact pinned OSC 8 authorization
event, opens that exact URL at most once, and otherwise treats changed provider
text as control-safe untrusted data. Its fixed UI never projects the pasted
authorization code and never guesses that an actual provider failure is input
echo. No URL,
executable, argument, environment key, or driver supplied by a provider
manifest, repository, Workspace, request, or project `PATH` can alter that
behavior. A Workspace copy is never an acquisition fallback. Request region is
separate Context/tool configuration and is not part of login state. Repository
`.git/config` and Workspace-authored tool configuration remain
inside the project/Workspace boundary; ambient host provider configuration is
never read by these drivers. The
separate Context Git identity fallback contains only safely re-encoded
`user.name` and `user.email`; it grants no authentication or signing authority.

Custom images are supported only when they preserve runtime API label
`io.tobari.runtime-api=1`, the `tobari` image user, and the exact built-in
entrypoint, including the `io.tobari.runtime-lifetime-command=sleep infinity`
capability needed for Tobari's fixed Workspace lifetime command. The intended
construction uses the resolver-selected immutable release digest (or the
contributor-local base) as `FROM`; a runtime-API label is a
compatibility assertion, not an image provenance or trust signature. The
selected image's `CMD` is ignored for Workspace lifetime: Docker create receives
the explicit `sleep infinity` command after the image. Tobari validates
compatibility before creating the project home, network, or container. Docker
create still supplies the invoking numeric UID/GID,
read-only root filesystem, dropped capabilities, fixed CPU/memory/PID/log
resource bounds, fixed mounts, guarded internal network, and health
check.

The canonical base under `runtimes/base` now includes Claude Code 2.1.220 and
Codex 0.147.0 beside the common tools. It downloads the pinned official agent
releases, verifies their per-architecture checksums, preserves the base user,
entrypoint, and lifetime command, and smoke-tests both version commands.
Claude may become the provider-only acquisition executable in
the separate mount-free login container; it never makes an ordinary Workspace
process or image executable a general host helper. Codex uses the official standalone package, which keeps its
CLI companion binaries and Linux sandbox resources together. Agent executables
and package resources live in image-owned `/usr/local/bin` and `/opt/tobari`
paths; `/var/lib/tobari` contains only per-Tobari home state and is safe to
replace with the persistent home bind. The retained child recipes remain
build-only integrity fixtures for each upstream artifact.

The combined base declares `NOASSERTION` and is permanently local-build-only.
`.github/workflows/runtime-base.yml` has read-only repository permission and
builds the multi-architecture source with cache-only output; it has no registry
permission, login, or push step. The released CLI materializes the same recipe
and builds it on the user's Docker host when its source-derived tag is absent.
Contributor development resolves `builtin` to its local combined base.

The root resolver obtains the desired image from the stored Context identity's
strict manifest on each runtime reconciliation. A new Context selects
`builtin`. The resolved selector, rather than the source of the
default, is persisted on the logical Tobari only as the last successful
runtime-container image. Project metadata is not consulted for runtime
selection.

Project runtime path mapping is owned by the Docker adapter. The selected root
is mounted exactly once with the bound Context's immutable source access. If
its canonical path is below the host
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

Explicit standard `cluster up` validates configuration, obtains and preflights the
Gateway image, locally ensures every required runtime image, builds and tests
the complete all-Context policy projection, reconciles exactly one OPA and one
Gateway, and
reconnects Gateway to every existing registered project network. It completes
only after OPA serves the exact aggregate revision and a defined decision
document.
Image preflight fails before the policy test, cluster journal, shared network,
or service-container mutation. Local Tobari-managed image development uses
`task build` and the source-hash development resolver instead of a public
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
work container, connects Gateway with the `gateway` alias, reconciles and
verifies both network-namespace guards, binds the exact owned Workspace source
endpoint plus Gateway endpoint to the host-issued Context/project principal,
waits for project readiness, and enters the container. The
logical creation and deletion boundaries use durable journals so an interruption
between the home, instance, index, runtime, and deletion steps is recoverable
without treating a partial file set as a second project. Runtime convergence
stores a hash of the bound Context's desired image identity, mounts, security,
environment, health contract, fixed resource contract, and profile revision on
the project container; drift recreates only that container and updates the
stored project image only after success. OPA runs with `--watch --bundle`
against one read-only Docker-managed bundle volume. Source edits become authority only
after explicit whole-projection validation and activation. The principal registry is a generic host-issued
contract: Docker currently supplies the owned network/endpoints observation, while a
future stronger runtime may supply the same binding through another adapter.
Only its dedicated directory is mounted read-only into Gateway; lifecycle
updates replace the registry file atomically inside that directory so a
single-file bind mount cannot strand Gateway on an old inode or expose the
neighboring credential configuration. Gateway caches a validated registry only
while its complete stat identity remains unchanged and fails closed, without a
last-known-good fallback, when a replacement is invalid.
Logical Tobari and Context IDs are not trusted when echoed by a caller; Gateway
derives both from the kernel-observed Workspace source endpoint and the exact
host registry binding. Exact allow, deny, and reset
actions provide the deterministic portable activation path: each locks the
projection, tests the target Context's private source copy and the complete
all-Context candidate, verifies the exact OPA and bundle-volume ownership
labels, builds a revision-named archive through pinned OPA, atomically renames
it through a fixed networkless pinned publisher, and transfers the tested
owner-only host projection through a bounded owner-only archive and container
stdin into the bundle volume rather than giving container root a host bind,
waits for the running OPA to report the exact expected revision, and rolls back
on failure. Reducing or mixed authority first activates a deny-all transition
revision.
The target-copy test yields a one-shot in-memory receipt bound to the exact
preflight bytes, candidate source digest, Context policy directory, and source
transaction. Aggregate construction may consume it only for that same
candidate; a missing or mismatched receipt reruns the per-Context OPA test. The
complete aggregate candidate test, existing aggregate revalidation, activation
test, and authority-reduction fence are never skipped.
`delete` observes active Docker exec IDs before ordinary removal, then verifies
owner, ID, and role labels before removing the selected container and network;
an attached exec rejects ordinary deletion and `--force` skips only that guard.
It then removes only its XDG home and records. Container
or network loss is reconciled by the root operation; it never deletes logical
state. Cluster status and reconcile derive project counts and network joins
from the indexed CWD-owned records.
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
and extra keys fail closed. Scoped agent-help schema 1 publishes the same
recursive declaration and exact success/error argv forms, including global
flag placement. Root help remains an index.

Interactive-only completion can remain a catalog entry with internal
visibility so the workflow uses the same typed composition authority. Internal
entries are excluded from public command copies, lookup, routing, reference
workflows, and help. Public projections also omit any interactive metadata that
would reveal an internal completion path.

In the experimental build, the four auth commands share one catalog-declared `authentication.broker`
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
authority. The compiled union includes GitHub, Datadog, OpenAI, Anthropic, and
AWS; AWS alone adds its method axis. Interactive omission opens the bounded
selector. Import, status, and logout remain
available for strict owner static manifests and Chatwork.

Standard `doctor` composes bounded read-only environment, Docker, policy, and
project-binding diagnostics. Experimental doctor adds provider, root-key/vault,
and broker checks. The application-owned
finite DAG schedules each infrastructure observation once its declared direct
prerequisites pass, continues independent branches, and creates typed blocked
rows for unready dependents. Infrastructure returns only pass, warning, or
failure observations; application code owns recovery and validates the fixed
complete order. Doctor JSON schema 1 and the text/TSV renderers project those
same facts. It fails the command with `diagnostic_failed` when any check fails,
and treats warnings alone as healthy. It observes rather than repairs: policy
checks validate bounded host source structure and do not start an OPA test
container, and doctor does not create a
root key, initialize or activate policy, start/reconcile/unlock the cluster, or
mutate provider, vault, credential, handle, or project-auth state.

Every catalog command that supports human text explicitly declares the shared
semantic-token presentation. The CLI presentation layer owns the exact
`text`, `muted`, `accent`, `success`, `warning`, and `danger` vocabulary and is
the only production location that maps those meanings to ANSI color or
emphasis for catalog-rendered output. Command renderers select tokens by
information meaning; they do not own escape sequences or concrete colors.
The separate trusted-host login stream boundary may regenerate only its closed
reviewed upstream SGR vocabulary as the same semantic styles; it visibly
projects every unknown control and has dedicated conformance tests.
Infrastructure reports terminal
capability and the presence-only `NO_COLOR` environment preference. The CLI
combines those facts per output stream, keeping redirected and machine output
free of ANSI styling. Cursor-control sequences used by bounded interactive
selectors remain a separate terminal mechanism and do not define visual
styles.

Human text is built in three independent stages. Task-owned typed results first
select a `humanOutput` document containing headings, rows, sections, empty-state
scope/bounds, and exact recovery. The style projection may then add only the
six semantic ANSI tokens; it cannot select, remove, reorder, or rename document
content. The terminal interaction state machine independently owns alternate-
screen entry, home-and-clear repaint, redraw eligibility, and main-screen/
cursor restoration. It never locates a prior frame by logical line count
because one line may occupy multiple physical rows after terminal wrapping.
Its reader absorbs idle VTIME polls without returning a render event, while
input, selection changes, completion, and cancellation remain observable state transitions. Catalog
selection supplies bare-namespace normalization and deterministic typo
suggestions, so routing and recovery do not create a second command registry.

## Gateway request flow

Standard Gateway establishes the source-bound Context/project principal,
normalizes the transparent authority, redacts authentication and cookies from
OPA/audit, asks OPA about the ordinary exact HTTP effect, and only after allow
forwards the original Workspace-owned credential in one upstream attempt. It
has no provider projection or Broker adapter. The Claude/Codex native-login
regression exercises this sequence, including Claude's authenticated profile
metadata request after token exchange.

### Experimental Broker augmentation

The experimental Gateway layer extends the same sequence as follows:

```text
client request headers
  -> establish the host-issued Context/project principal from the source
     endpoint at the header hook
  -> for transparent ingress, require one consistent SNI/HTTP authority and
     bind it as the requested server address instead of the synthetic target
  -> reject every malformed, misplaced, ambiguous, or binding-mismatched Tobari
     handle marker as credential_handle_invalid
  -> strictly recognize one valid broker handle from provider projection,
     remove it, and introspect its full non-secret binding
  -> at a declared header or AWS signing binding, reject any real Workspace
     credential as broker_auth_required before OPA or external I/O
  -> only when no declared binding and no Tobari handle marker exists, select
     Workspace-owned compatibility passthrough
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
  -> on a Datadog/OpenAI/Anthropic allow, select or refresh the same record once and
     apply only its reviewed bearer/supplemental-header result
  -> on an AWS allow, retain the authorized request within 8 MiB, obtain one
     private companion export, sign locally, and apply only those headers
  -> for the undeclared compatibility route, strip control headers and forward
     client authentication only after allow
  -> enable ordinary request-body streaming; forward an allowed buffered
     GraphQL or signed AWS body once
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
hashes never enter policy, audit, learned state, or CLI output. Client authentication can be present on the forwarded
request but is absent from OPA input and audit output. No query or headers are
emitted in audit. Audit retains the path component, except that any path
containing a Tobari handle marker becomes `/[redacted-auth-handle]`. Structural
URL/header handle rejections are non-learnable and cannot become policy
candidates. Any Tobari-looking handle marker either enters the exact valid
broker route or fails as `credential_handle_invalid`; only complete marker
absence permits configured fallback. A valid candidate is removed before
broker or OPA I/O and is never forwarded on failure. Broker introspection
returns no secret; policy denial performs no resolution, refresh, companion
call, or signing. The addon has no managed or arbitrary dynamic fallback and
never retries.

Denied audit records are also the policy-development feedback interface. A
learnable Gateway denial carries a fixed host-side `tobari policy review`
navigation hint, and session closure may summarize the pending queue on host
stderr. These are advisory only: they contain no action reference and cannot
approve or retry a request. The learnable response tells the caller to keep the
current Workspace and agent session running, use a separate trusted-host
terminal, and retry in that same Workspace only after confirmed Apply. A
non-learnable response instead names the read-only `tobari cluster denials`
diagnostic and exposes no review command. `tobari cluster denials` parses one bounded Gateway
log window, rejects
malformed denial-shaped records, and returns typed Context and project principal, host, port,
method, path, optional GraphQL operation/root coordinate, reason, status,
exact-rule learnability, request identity, timestamp, the
trusted host policy directory, and the exact review command. OPA computes
learnability only when version, cluster, Context, scheme, fixed port,
project-principal, and preset guardrail boundaries already pass, so an exact
Context/project/scheme/host/port/method/path rule, plus the GraphQL coordinate when
present, can close the request. `policy review` and
`policy candidates` deterministically fold only that eligible retained evidence
by the structured Context/project/scheme/host/port/method/path and optional GraphQL
effect key. They emit one opaque
exact-rule reference, the latest evidence, and an observation count for each
pending effect; references remain stable across repeated denials. This pure
read projection also converges concurrent identical audit records without a
second persisted inbox or write race. They remove effects already covered by
the CLI-owned learned allow or deny data and trusted baseline deny rules.
Baseline denies remain audit-only. `policy review` is the routine human text
workflow: it stages explicit decisions over unchanged opaque candidate IDs for
one Context and applies the complete typed set once. Apply or discard precedes
switching Context, keeping source promotion to one atomic domain generation;
redirected review is read-only. Manual refresh intersects the staged ordered
set with the fresh queue by opaque ID. Final review repeats every exact scope,
effect, decision, and candidate ID before one explicit confirmation. The
infrastructure returns the revision only after the running OPA confirms it;
the typed application receipt preserves that revision and ordered decisions.
`policy rules` is the exhaustive current inventory of CLI-owned learned Allows
and exact Denies. `policy reset --id` removes exactly one such decision through
the same preflight, atomic-write, and OPA activation boundary, leaving the
matching effect at default deny so the retained denial can enter `policy review`
again. It never edits baseline policy, grants permission, or retries a request.
Raw `cluster logs` remains the component-debugging interface.

`policy allow` resolves one exact candidate reference against retained
validated audit state without decoding it. Infrastructure reads the bounded,
owner-only `policy/domains/<canonical-host>/{allow,deny}.json` tree, preserves
every unchanged file byte-for-byte, appends one deterministic exact learned
rule to its host's `allow.json`, tests a private complete policy copy, swaps one
complete domain-source generation, and calls the existing OPA activation
boundary. Deny mutations use the matching `deny.json`; an unknown exact host is
created with both files in the staged generation.

Each authoritative domain source is strict schema 1. The directory name and
every embedded authority, endpoint, credential binding, and rule host must be
the same canonical lower-case host. Methods belong to the authority records
composed from that domain and cannot authorize another host. Wildcards and IP
literals are not source syntax. Guided Contexts contribute only the domain
tree; aggregate generation loads the current shared Rego evaluator and tests
from Tobari's embedded runtime assets. Advanced Contexts add exactly
`tobari.rego` and `tobari_test.rego`. Gateway runtime input uses exact schema 1
and rejects any other shape. The generated aggregate projection may contain a
single internal `data.json`; it is immutable execution input, never a Context
source. The aggregate projection is schema 1 and stores the composed sources below
`tobari_contexts[context_id]`; the Tobari-owned router is the only
`tobari.http` decision entrypoint. Each entry in `boundary.authorities` owns its
scheme, host, ports, and host-local methods;
`boundary.graphql_endpoints` declares exact protocol-classification points, and `rules` keeps
baseline denies, learned allows, and learned denies in separate collections.
Gateway and OPA share this structure; the CLI only owns the mutation of the
two learned collections and never rewrites the host-authored boundary.

Routine mutations serialize through an in-process mutex and a cross-process
file lock. They prepare and fsync a complete sibling generation, record a
strict durable journal with original/candidate digests, move the prior
`domains/` generation to a recovery name, and rename the complete candidate
into place. A lockless reader therefore observes the old generation, a
fail-closed gap/journal, or the new generation, never a valid mixed pair.
Aggregate state records the candidate immutable projection revision before the
journal is finalized; recovery commits only when that revision is durable and
otherwise restores the validated original. Unexpected external edits make the
transaction ambiguous and fail closed instead of being overwritten.

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
the same Context/project/scheme/host/port/method/path and optional GraphQL coordinate.

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
GitHub host driver leaves the previous Context credential unchanged; a
host-login availability rejection carries one closed
infrastructure-owned diagnostic stage through the existing public fault code.
The stage vocabulary distinguishes driver dependency, executable lookup,
symlink/canonical-path/trusted-root checks, identity, a recognized ChatGPT app
bundle selection, and Codex executable/version observation without retaining a
raw cause, local path, digest, or provider output. An
`auth_mutation_outcome_unknown`, `unclassified_mutation_outcome`, or
`mutation_output_write_failed` result is non-retryable and directs the user to
`auth status` before another auth mutation. Confirmed login/import/logout output
is finalized before late cancellation can imply that replay is safe.
After a confirmed Auth Broker mutation, projection observation is advisory:
bounded registry, project, or binding-status failures become explicit
`unavailable` activation coverage and cannot turn confirmed `changed` or
`no_change` into replay permission. Infrastructure returns bounded, secret-free
observation facts only. `authcmd` and the Auth Broker domain validate request
Context, logical project, and credential-revision correlation, then derive
activation state, exact re-entry actions, and mutation-change semantics.
Presentation never derives freshness from provider labels, process existence,
or row order.

## Architecture enforcement

- Go architecture lint preserves layer direction and a thin `cmd/tobari`.
- Domain and application tests prove path, state, effect, and orchestration
  invariants without Docker.
- Infrastructure tests use a recording command runner.
- Gateway and Rego tests cover policy boundaries.
- Auth Broker, root-key, provider, companion, and Gateway integration tests
  cover locked startup, every closed vault record, project-bound handles,
  deny-before-action, bounded static/refresh/signing results,
  rotation/revocation, durable unknown-outcome barriers, and no fallback.
- Host-driver tests cover each fixed argv, canonical executable identity and
  digest recheck, private temporary home or PTY, bounded browser/output
  contract, checked cleanup, and cancellation settlement. Negative dependency
  and image tests prove managed profiles, arbitrary helpers, and provider CLIs
  inside Broker remain absent.
- Docker integration tests prove actual network topology and lifecycle.
