# Product Contract

## Product statement

Tobari is a local CLI that gives a coding agent an execution boundary in
advance, then lets it act freely inside that boundary. It makes starting an
isolated coding space, understanding a denied operation, and granting the
minimum required permission extremely easy, so the safe execution path is a
more natural choice than running the agent on the host. Isolation is opt-in and
reversible; creating a space is a CWD-local action, and customizing its network
authority is an observe, review, approve, and retry loop rather than a
prerequisite policy-authoring project. Every supported outbound HTTP and HTTPS
request remains enforced through one shared Gateway and OPA policy boundary.

## Primary users and owned outcome

The primary user is a developer who wants an autonomous coding agent to edit a
bounded source tree without receiving host credentials or unrestricted network
egress. Tool-native authentication may be created inside the selected
Tobari's own persistent home; host authentication state is never copied in.
The user-facing entry point is the current project directory: a Tobari either
exists or does not exist, and the user should not need to manage container
names, network IDs, or policy internals for routine work. `cluster up` remains
the current explicit owner of shared Gateway and OPA setup; reducing that
first-use bootstrap is an adoption goal, not the reason a user adopts Tobari.

The primary operating loop is progressive policy learning: a Tobari workload is
denied by default, Gateway records the rejected host/port/method/path and reason
without secrets, the CLI presents a bounded exact proposal and a concrete
trusted-host next action, the user approves the minimum rule, and the same
workload is retried. A learnable denial also gives the agent a fixed host-side
review command, and the human path enters through `policy review`; machine
discovery remains `policy candidates` and the exact opaque reference remains
the safety boundary for `policy allow --id` and `policy deny --id`.
`policy rules` is the exhaustive current-decision view; `policy reset --id`
returns one learned Allow or exact Deny to default deny so the retained effect
can be reviewed again. Reset does not authorize or retry the request.
Trusted-host Rego editing remains the advanced path for behavior that exact
learned rules cannot express; ordinary permission growth must not require it.
Exact policy actions perform the bounded activation required for their own
mutation.
Denial evidence is a product output, not incidental debug noise.
The host-issued project principal is retained in denial, candidate, learned
rule, and compaction evidence; an approval made from one current-directory
Tobari cannot be replayed as another project's permission.
The initialized policy requires an explicitly captured empty request body for
routine learned permissions; an unavailable body is denied before policy
evaluation. Body-dependent APIs require a trusted-host body-aware Rego change
and explicit policy activation rather than an observed host-only approval.

## Public vocabulary

- **Tobari:** one long-lived logical untrusted execution environment selected
  by a canonical project root. Its work container is recoverable runtime
  implementation detail.
- **Workspace:** the human-facing name for one directory-bound Tobari in
  lifecycle and list output. It is not a second runtime resource; its identity
  remains the canonical root and its stable Tobari ID remains diagnostic.
- **cluster:** the one installation-local Gateway, OPA, policy, principal,
  optional managed-credential inputs, and CA lifecycle.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **root:** the canonical host directory selected from the current working
  directory and mounted read-write into one Tobari. A root below the host home
  is mounted at the same relative path below `/var/lib/tobari`; a root outside
  the host home uses the mirrored `/workspace` path.
- **Tobari home:** a per-Tobari persistent owner-only XDG state directory
  mounted as the work user's home.
- **Tobari image:** the minimal built-in runtime or one locally available
  compatible OCI environment image selected when a Tobari is first created.
  Its tools and bootstrap are part of the environment; its image `CMD` is not
  the Workspace lifetime command.
- **Tobari ID:** a generated stable internal identity used for state, exact
  resource labels, and host-issued project-principal bindings. It is diagnostic
  output, not a routine user action input.
- **project principal:** a host-issued binding from one stable Tobari ID to
  the exact Gateway interface on that project's dedicated network. Caller
  headers and profile names are not principals.
- **tool-owned authentication state:** files written by a tool or agent below
  one Tobari's persistent home during its own login or configuration flow.
- **credential profile:** non-secret Gateway configuration for the retained
  managed adapter; it binds a profile to exact hosts and project principals.
- **Context:** one host-selected logical execution setup. Its manifest
  references an agent profile, a compatible Tobari runtime image, a policy
  store, and managed-credential stores; those stores remain physically
  separated by trust boundary.
- **Context runtime recipe:** the active Context's owner-only
  `runtime/Dockerfile`. It is the source for an explicit host-side custom
  runtime build; it is not project metadata and it is never mounted into a
  Workspace.
- **agent profile:** read-only non-secret shared agent configuration referenced
  by a Context. It is not tool-owned login state.

The stable Tobari ID is not trusted when supplied by a work container. The
host-owned principal registry derives it from the Gateway interface that
received the request. The initialized host policy remains an installation-wide
baseline; learned permissions are project-bound. Principal identity does not
select or inject tool credentials.

The public commands are:

| Command | Role | Effect | Outcome |
|---|---|---|---|
| `help [selector] [--format text|agent]` | utility | read | Discover exact command contracts |
| `version` | utility | read | Print build identity |
| `doctor [--root PATH] [--format text|tsv|json]` | utility | read | Validate host, Docker, configuration, policy, managed-secret permissions, ports, and residue |
| `tobari` | act, fixed target | create | Choose or create the current directory's Workspace, reconcile runtime, enter it, and leave it reusable after `exit` |
| `status` | utility | read | Inspect the nearest current-directory Tobari and its diagnostic runtime state |
| `list [--format text|json]` | utility | read | List local Workspaces with runtime diagnostics and diagnostic IDs |
| `delete [--force]` | act, fixed target | write | Delete the nearest current-directory Tobari when detached; use `--force` to override an attached-session guard |
| `cluster status [--format text|json]` | utility | read | Inspect shared state, health, proxy, policy, and recent errors |
| `cluster denials [--tail N] [--format text|json]` | utility | read | Read a bounded typed denial window, exact-rule learnability, policy path, and review command |
| `cluster logs [--component gateway|opa|all] [--tail N]` | utility | read | Read bounded shared logs, including policy-denial evidence |
| `cluster down [--purge]` | act, fixed target | write | Remove shared transient resources after every logical Tobari is deleted |
| `policy candidates [--tail N] [--format text|json]` | discover | read | Discover unique pending exact host/port/method/path candidates and opaque IDs |
| `policy review [--tail N] [--format text|json]` | discover | read | Review pending permissions; on a TTY, explicitly confirm one exact allow or deny |
| `policy tail [--tail N]` | discover | read | Review the bounded pending queue with exact allow and deny commands |
| `policy allow --id ID` | act, reference bound | write | Test, record, and activate one exact observed permission |
| `policy deny --id ID` | act, reference bound | write | Test, record, and activate one exact project-bound rejection |
| `policy rules [--format text|json]` | discover | read | List every current CLI-owned learned Allow and exact Deny decision; on a TTY, reset one explicitly |
| `policy reset --id ID` | act, reference bound | write | Remove one learned decision and leave its effect at default deny |
| `policy compactions [--format text|json]` | discover | read | Discover safe bounded prefix-compaction candidates and opaque IDs |
| `policy compact --id ID` | act, reference bound | write | Test and activate one current learned-rule compaction |
| `context list [--format text|json]` | utility | read | List named Contexts and identify the active execution setup |
| `context show [--name NAME] [--format text|json]` | utility | read | Inspect one Context's agent, policy, and credential-store references |
| `context create --name NAME [--image IMAGE] [--mode guided|advanced]` | act, fixed target | create | Create one named Context with a runtime image and separate owner-only stores |
| `context use --name NAME` | act, fixed target | write | Select one Context and apply it to a running shared cluster; leave a stopped or unconfigured cluster for explicit `cluster up` |
| `runtime init [--format text|json]` | act, fixed target | create | Create the active Context's runtime/Dockerfile template without changing its selected image |
| `runtime build [--format text|json]` | act, fixed target | write | Build, validate, and select the active Context's generated local runtime image |

The root command is interactive and requires a TTY on stdin, stdout, and stderr.
It does not silently create state in a non-interactive context. When the
canonical current directory is below one or more indexed Workspace roots, the
command presents an English selector ordered nearest-first. Arrow keys and
Enter choose an existing Workspace; `n` chooses explicit creation at the
current directory; `q` or Escape cancels. If raw terminal mode is unavailable,
the same choices use numbered line input without adding a terminal module or
shell subprocess. Candidate status and path text remain meaningful without
color. Programs inside Tobari can mutate the explicitly mounted root; that
delegated capability is a documented security property rather than an
undeclared Docker mutation by the CLI.

## Input and path contract

- The current working directory is expanded and canonicalized on the host
  before state or Docker calls. An exact indexed root is reused directly. When
  only containing ancestor roots exist, `tobari` lists every valid root
  nearest-first and offers explicit creation at the current directory; it never
  creates a nested Tobari implicitly.
- Each canonical root identifies at most one Workspace. Repeated or concurrent
  explicit creation at an already indexed root is rejected as already existing;
  it never creates a second logical record for that directory.
- Project-root selection rejects the filesystem root, the user's home and its
  ancestors, and any path overlapping XDG Tobari configuration, state, or
  shared-profile management directories, Docker sockets, or Docker management
  paths. A repository containing policy source remains allowed; only the
  trusted active policy/configuration paths are protected.
- An explicit create choice uses the canonical current directory as root.
  Project moves and copies are not inferred or recorded in the project tree.
- The selected root is the only project directory mounted read-write. When it
  is below the host home, the container target preserves the relative path
  below `HOME=/var/lib/tobari`; for example, a host root of
  `$HOME/path/to` enters at `/var/lib/tobari/path/to`. A root outside
  the host home retains `/workspace/<canonical-root-without-leading-slash>`;
  thus a root at `/work` and a host CWD of `/work/root` enter at
  `/workspace/work/root`. Tobari never mounts the host home wholesale.
- The configured image accepts `builtin` or a portable OCI image reference.
  In normal builds, `builtin` resolves to the published official runtime base.
  A custom image must already exist locally and preserve runtime API `1`, the
  `tobari` image user, the `io.tobari.runtime-lifetime-command` capability, and
  the Tobari entrypoint. That capability is currently `sleep infinity`, which
  is required by Tobari's fixed Workspace lifetime command. Ordinary `tobari`
  startup never pulls a configured image implicitly. `cluster up` may obtain
  the official runtime base for an uncustomized Context; custom images still
  fail closed if missing or incompatible before project runtime network or
  container mutation.
- `cluster up` obtains the embedded immutable Gateway digest when it is not
  already available locally, then validates its digest, API/role labels,
  non-root default user, entrypoint, and Docker Engine platform before running
  policy tests or creating shared networks and containers. Gateway source
  development uses the contributor-only `task build:dev` path and a
  `tobari_dev` binary, not a public `cluster up` option.
- `runtime init` creates the active Context's owner-only
  `runtime/Dockerfile`. The template starts from
  `ghcr.io/tasuku43/tobari/runtime:latest`; editing that file is the supported
  place to add tools and environment configuration for the Context. The
  command does not overwrite an existing recipe.
- `runtime build` is the explicit exception to the no-implicit-pull rule. It
  runs a host Docker build using only the Context runtime directory as build
  context; Docker may obtain a missing base image for this explicit build.
  Tobari validates the resulting image against the same runtime contract,
  records its immutable local image digest, and then selects the generated
  `tobari-context-<context>:<source>` image in the active Context. The previous
  selected image remains in force until promotion succeeds.
- The built-in `tobari/runtime` image is the base work runtime: it preserves the
  lifecycle contract and carries common Git, HTTP, JSON, Python, SSH, and
  command-line tools. It is published on reviewed main pushes as the moving
  `latest` and `main` development channels; registry publication is not
  implied by local image selection, and Tobari never pulls the published image
  implicitly during ordinary startup.
- Official agent images are complete compatible variants in the same runtime
  family, with tags such as
  `claude.2.1.34-base.0.1.0-r1`. They add the agent tool and only its
  agent-specific dependencies; they do not create a second authority boundary.
- Project metadata does not select or alter the runtime image. New Workspaces
  use the active Context image, or an explicitly supplied `--image` for the
  legacy named lifecycle command; all selected images still pass the same
  compatibility checks before Docker mutation.
- Shared cluster mutations use one command-bound `tool_local` target and are
  never performed by the root `tobari` operation.
- CWD-local lifecycle operations use one command-bound `tool_local` current
  directory target and do not accept an ID, name, or root selector.
- `delete` removes that nearest target without `--force` when no session is
  attached. An attached session returns `project_session_attached` and leaves
  state untouched; `--force` is the explicit override.

## Output and exit contract

Human output is concise text. Cluster status JSON is schema version 1; cluster
denials, policy candidates, and policy compactions JSON are schema version 2
because their items retain the project principal. `list --format json` reports
root, runtime diagnostic, and stable ID. Agent help uses the catalog schema.
Successful data is stdout;
failures are stderr.
The Workspace selector is a human stderr interaction; it produces no JSON or
stdout selection protocol. A successful choice prints an English summary before
the child session, and cancellation or stale selection prints no success
summary. The interactive child owns stdout. When it returns, the host command
writes this lifecycle guidance to stderr:

```text
Workspace session closed.
Workspace remains available.

Resume: tobari
Remove: tobari delete
If another session is attached: tobari delete --force
  ```

`exit` therefore detaches the session without deleting the Workspace. The
Workspace remains existing until the host runs `tobari delete`, which is the
normal lifecycle-ending operation when no session is attached. If another
session is attached, ordinary delete fails with a warning and `--force` is the
explicit override. There is no public `stop` or `pause` state. The choice is
revalidated under the lifecycle lock before logical or Docker mutation, so a
changed candidate set fails closed and asks the user to run `tobari` again.
When a learnable network request is denied, the Gateway's 403 response carries
fixed secret-free host-review navigation for the agent, and an interactive
session close may summarize the pending queue on host stderr. These are
advisory only; the interactive `policy review` queue is the human entry point
and delegates one explicitly confirmed opaque reference to the separate exact
reference-bound `policy allow` or `policy deny` action. `policy rules` is the
current learned-decision inventory; its TTY reset flow delegates one explicit
opaque reference to `policy reset`. Redirected and machine-readable review and
inventory remain read-only.
Human `text` output uses one shared presentation vocabulary across lifecycle,
policy, diagnostics, help, version, and error views: an outcome-first heading,
a small state marker, aligned detail rows, semantic color tokens, and an exact
next action when the result has a useful recovery or continuation. Success is
green, warnings are yellow, failures are red, active or navigational emphasis
is cyan, and secondary labels/details are muted. Color is applied only when
the corresponding output stream is an interactive terminal; redirected text
has the same semantic order without ANSI control sequences. `doctor` defaults
to this human text view; `doctor --format tsv` remains the tab-separated
projection for scripts, and JSON/agent help remain schema contracts.
Empty collections are explicit rather than silent. Opaque IDs remain byte-for-
byte exact, while external evidence remains subject to the existing safe text
projection before it is displayed. Root, namespace, and exact human help use
the same hierarchy; `--format agent` is machine-readable JSON and never receives
terminal styling.
When `cluster up` runs with an interactive stderr terminal, it may also render
bounded fixed-step startup progress on stderr. The progress uses terminal
control sequences and color only for that terminal presentation; it carries no
runtime diagnostics and is absent for non-interactive or machine-readable
callers. The completed checklist remains visible, and the final human summary
is the same summary rendered by `cluster status`; it remains the only
successful data written to stdout. JSON output is unchanged and contains no
terminal control sequences. The checklist presents the internal startup work
as three user-facing phases: `prepare environment`, `start services`, and
`verify readiness`. In a terminal, semantic colors distinguish active,
healthy, warning, failed, and secondary information; labels and values remain
otherwise plain. The ready summary prioritizes outcome, component health,
attached Tobari count, and policy path; configured/running booleans, the proxy
endpoint, and the full recent diagnostic remain available in JSON or failure
detail. A successful `cluster up` additionally points to the next `tobari`
command.

`context use` reports a `cluster` result in its Context output. `reconciled`
means the selected policy and Gateway credential mounts were applied and
health-checked; `already_ready` means the running cluster already used that
Context; `not_running` and `not_configured` mean the host selection was saved
but an explicit `cluster up` is still required. A failed switch is not a
successful selection result.

Project runtime diagnostics may report `incomplete` when a durable root index
survives without its instance state. This preserves logical existence for safe
cleanup, prevents runtime recreation, and directs the user to delete the exact
current-directory Tobari before creating it again.

The lifecycle state model has two dimensions:

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
  -> tobari delete (from the host)
Workspace absent
```

`status` and `delete` continue to resolve the nearest canonical Workspace
containing the host current directory. When several ancestor Workspaces exist,
run the destructive command from a directory whose nearest Workspace is the
one intended for removal. If that Workspace has an attached session, add
`--force` only when terminating that session is intentional.

| Exit | Meaning |
|---:|---|
| 0 | Success |
| 2 | Invalid command or input |
| 3 | Internal or Docker execution failure |
| 9 | Required runtime temporarily unavailable |
| 10 | Policy or diagnostic rejection |
| 11 | Caller cancellation |
| 13 | Declared contract violation |
| other from root entry | Exact child process exit status when Docker started the interactive work process |

Commands use complete delivery. `list` is exhaustive for local logical state at
one observation point. `status` is a CWD-local scalar observation; cluster
status is exhaustive for the shared cluster scope. Logs are
a bounded recent window of 1 through 10,000 lines per selected component.
Denials are a fully delivered typed projection from the requested bounded
Gateway-line window; an empty `items` collection means no valid denial occurred
in that window, not exhaustive history.
Policy candidates, review, and tail are bounded by the same retained
Gateway-line window and omit effects already covered by learned allow rules,
baseline deny rules, or exact learned deny rules. Baseline and exact denies
remain available as audit evidence but never become pending queue items.
Compactions are exhaustive for the current validated learned-rule file at one
observation.

## Configuration contract

Configuration is resolved from
`${XDG_CONFIG_HOME:-$HOME/.config}/tobari` on both macOS and Linux:

- `config.json`: schema-v1 default Tobari image selector;
- `contexts/<name>/context.json`: host-owned Context manifest with the named
  agent profile, compatible Tobari runtime image selector, and guided/advanced
  policy mode;
- `contexts/<name>/runtime/Dockerfile`: optional owner-only Context runtime
  recipe created by `runtime init`; its source digest and last successful
  managed image build are recorded additively in `context.json`;
- `contexts/<name>/policy/`: Rego and data for that Context, mounted read-only
  into OPA and watched for host edits;
- `contexts/<name>/credentials.json`: reserved schema-v1 profile metadata for
  the explicitly selected managed Gateway adapter;
- `contexts/<name>/credentials/`: reserved managed-adapter secret files for
  that Context, never mounted into a Workspace;
- `contexts/active.json`: owner-only active Context selection; missing means
  `default`;
- `principal-registry/principals.json`: owner-only host-issued schema-v1
  project-to-Gateway-network bindings, maintained by lifecycle reconciliation
  and directory-mounted read-only into Gateway so atomic host updates remain
  visible without exposing credential files;
- Legacy top-level `policy/`, `credentials.json`, and `credentials/` are read
  only during the one-time default Context compatibility migration and remain
  untouched for rollback/diagnosis. The default passthrough adapter does not
  load managed credential files;

Tool authentication state is not cluster configuration. It belongs below the
selected instance's persistent home and is created by the tool's own login or
configuration flow.

Runtime state is stored under `${XDG_STATE_HOME:-$HOME/.local/state}/tobari`:
`roots/<hash>.json` indexes canonical roots and
`instances/<id>/state.json` contains one logical instance and diagnostic runtime
identifiers. `instances/<id>/home` is the writable home for tool-owned state.
Shared read-only agent profiles referenced by Contexts are under
`${XDG_DATA_HOME:-$HOME/.local/share}/tobari/profiles`.
Cluster state contains the active Context name, resolved store paths, and
Docker resource names or identifiers, never credential contents; managed
credential paths are reserved for the managed adapter, while the per-Tobari
home may contain tool credentials by design.
Project and cluster mutation journals are durable recovery markers. An
interrupted cluster marker, active-context mismatch, or failed Context
reconciliation makes entry and policy operations fail closed until the exact
shared cluster operation completes.
Environment variables select only XDG locations and test/runtime overrides
documented in scoped help; they do not carry managed token values and Tobari
does not copy host credential values into the runtime environment.

Image selection uses the active Context's bounded runtime image selector. The
legacy `config.json.default_image` seeds the default Context once, and `builtin`
is used when no legacy setting exists. Project metadata does not override the
Context image.

The active Context recipe is a host-side customization source, not a second
image authority. `runtime build` derives the image reference mechanically from
the Context name and Dockerfile source digest, validates it, and atomically
promotes it into the existing Context image field. Editing the recipe or a
failed build leaves the previously selected image unchanged.

When the recipe's first base is the exact official
`ghcr.io/tasuku43/tobari/runtime:latest` reference, an explicit `runtime build`
refreshes that moving base. An explicit local or custom base is not given a
registry-pull request, so local-only base images remain usable.

OPA reads the policy bind with `--watch`. Exact policy mutations test a private
complete policy copy and activate only the exact owned OPA component; host
authored edits remain an advanced, explicit workflow.

## Side effects

`runtime init` creates the active Context's owner-only runtime directory and
template without changing image selection. `runtime build` executes the
explicit host Docker build, validates the generated image, and atomically
updates only the active Context manifest after the image digest is confirmed.
Build, validation, or promotion failure leaves the previous selected image
authoritative and directs the user to inspect the Context before retrying.
The official moving base is refreshed only for an explicit build whose recipe
starts from the exact official `runtime:latest`; custom and local bases retain
their local/cache-first behavior.

`context use` validates the target Context before changing the active marker. If
the shared cluster is running, it writes the existing reconcile journal, applies
the selected Context's policy and Gateway credential paths through the same
reconciliation path as `cluster up`, waits for Gateway and OPA health, and only
then reports `reconciled`. If the cluster is stopped or unconfigured, it records
the selection without starting Docker and reports `not_running` or
`not_configured`; the next action is `cluster up`. A failed switch restores the
previous marker and persisted state when possible. If Docker may have changed,
the recovery journal remains and entry/policy commands direct the user to an
explicit `cluster up`.

`cluster up` obtains and preflights the immutable Gateway image and official
runtime base, then creates shared labeled networks, configuration material,
Gateway, OPA, and CA volumes as needed. It reconnects Gateway to the shared
networks and existing registered project networks without creating project
state or project resources, then waits for Gateway and OPA health. The root command only verifies the shared cluster is
configured and ready, reads the indexed Workspace candidates, and waits for an
explicit choice when the canonical current directory is below an ancestor.
After the choice is revalidated under the lifecycle lock, it creates or reuses
the selected logical record, validates the selected image before project
runtime mutation, reconciles its labeled container and internal network, binds
its XDG home, joins Gateway to that network, waits for the project healthcheck,
and enters the resulting terminal session. Docker create appends Tobari's
fixed `sleep infinity` lifetime command after the image; the image `CMD` is
not used to own Workspace lifetime. Shells and exact agent commands run through
child exec sessions, so a child command's nonzero exit is returned without
stopping the reusable Workspace. A changed image identity, runtime contract,
mount/security/environment/health specification, or shared profile revision
recreates only the project container and preserves its logical state and home.
Returning from that child session, including a normal shell `exit`, performs no
Workspace deletion: it only returns the child exit status and emits the host
stderr guidance described above. `delete` is the separate lifecycle-ending
operation. It removes only that exact label-owned container, network, root
index, instance state, and home after confirming that no session is attached;
`--force` overrides that one guard.
`cluster down` rejects while any
Tobari remains
and removes only exact shared resources; its `--purge` also removes shared CA
volumes. No command removes a mounted root or files inside it. Each project
work container is created with fixed CPU, memory, PID-count, and container-log
bounds; a resource-contract change is treated as runtime drift and recreates
only that work container. These limits do not claim a disk quota for the
explicitly mounted root or network bandwidth shaping.
`policy allow`, `policy deny`, `policy reset`, and `policy compact` first build and test the complete candidate
policy in a private host temporary directory. After successful tests they
atomically replace only `policy/data.json` and invoke the same activation
boundary. They never write Rego source, managed credential files, or tool-owned
home files.
OPA marks a denial learnable only when its version, cluster, scheme, fixed
request port, project-principal boundary, and (when selected) managed
credential binding already satisfy the orthogonal boundary.
Candidate
discovery excludes other denials, preventing a successful no-op approval.

## Compatibility

Before v1.0, command details and configuration schema may change with release
notes. Legacy named state is not guessed or migrated automatically; users clean
it with the matching older binary before adopting the CWD-owned lifecycle.
Legacy named lifecycle invocations are rejected explicitly and direct users to
run `tobari` from the project directory. No compatibility alias recreates the
old name/root lifecycle.
Command names, resource labels, state schema, OPA input version, audit schema,
and Gateway decision schema remain explicit compatibility boundaries.

## Unsupported outcomes

The deliberate non-goals in [Project Theses](00_theses.md) are not hidden
commands or transport escape hatches. In particular, Tobari does not promise to
control non-proxy-aware traffic semantically; it prevents all direct egress and
supports HTTP/HTTPS through the explicit proxy only.
