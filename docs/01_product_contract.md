# Product Contract

## Product statement

Tobari is a local CLI that runs one shared Gateway and OPA cluster and attaches
named Docker-isolated coding spaces to user-selected roots. Every supported
outbound HTTP and HTTPS request is enforced through that shared policy boundary.

## Primary users and owned outcome

The primary user is a developer who wants an autonomous coding agent to edit a
bounded source tree without receiving host credentials or unrestricted network
egress. Tobari owns the lifecycle from environment validation through startup,
interactive or non-interactive execution, inspection, logs, and cleanup.

The primary operating loop is progressive policy learning: a Tobari workload is
denied by default, Gateway records the rejected host/method/path and reason
without secrets, the user refines and tests XDG policy on the trusted host, OPA
reloads it through watch, and the same workload is retried. Denial evidence is a
product output, not incidental debug noise.

## Public vocabulary

- **Tobari:** one named, long-lived, untrusted execution container.
- **cluster:** the one installation-local Gateway, OPA, policy, credential, and
  CA lifecycle.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **root:** the only host directory mounted read-write into one Tobari at
  `/workspace`.
- **Tobari home:** a per-Tobari persistent Docker volume mounted as the work
  user's home.
- **Tobari image:** the built-in runtime or one locally available compatible
  OCI image selected when a Tobari is first attached.
- **Tobari ID:** an opaque CLI-owned reference produced by `list` and consumed
  unchanged by individual actions.
- **credential profile:** a Gateway-only secret and exact host binding.

The public commands are:

| Command | Role | Effect | Outcome |
|---|---|---|---|
| `help [selector] [--format text|agent]` | utility | read | Discover exact command contracts |
| `version` | utility | read | Print build identity |
| `doctor [--root PATH]` | utility | read | Validate host, Docker, configuration, policy, secret permissions, ports, and residue |
| `cluster up` | act, fixed target | create | Reconcile and start shared Gateway and OPA |
| `cluster status [--format text|json]` | utility | read | Inspect shared state, health, proxy, policy, and recent errors |
| `cluster logs [--component gateway|opa|all] [--tail N]` | utility | read | Read bounded shared logs, including policy-denial evidence |
| `cluster down [--purge]` | act, fixed target | write | Remove shared transient resources after every Tobari is detached |
| `attach --name NAME --root PATH [--image IMAGE] [--devcontainer PATH]` | act, fixed cluster target | create | Attach one named Tobari with a compatible image to an existing root |
| `list [--format text|json]` | discover | read | List all configured Tobari and produce opaque IDs |
| `shell --id ID` | act, reference bound | read | Open an interactive shell in one Tobari |
| `exec --id ID [--cwd PATH] -- COMMAND...` | act, reference bound | read | Execute one command in one Tobari and preserve its exit code |
| `logs --id ID [--tail N]` | act, reference bound | read | Read one Tobari's bounded logs |
| `detach --id ID [--purge]` | act, reference bound | write | Remove one Tobari and optionally its persistent home |

`shell` and `exec` intentionally classify the CLI operation as read because the
CLI enters an existing exact referenced container. Programs inside Tobari can
mutate the explicitly mounted root; that delegated capability is a documented
security property rather than an undeclared Docker mutation by the CLI.

## Input and path contract

- Paths are expanded and canonicalized on the host before Docker calls.
- `attach --root` requires an existing directory and a name matching
  `[a-z][a-z0-9-]{0,62}`.
- `attach --image` accepts `builtin` or a portable OCI image reference and
  otherwise follows the configured default. A custom image must already exist
  locally and preserve runtime API `1`, the `tobari` image user, and the Tobari
  entrypoint. Attach never pulls an image implicitly.
- `attach --devcontainer` conflicts with `--image` and names one explicit
  regular file below the canonical root. The supported JSONC subset requires
  one literal `image` and permits only inert `$schema`, `name`, and
  `customizations` metadata. Dockerfile, Compose, Features, mounts,
  environment, user, privileges, capabilities, ports, and lifecycle properties
  are rejected rather than ignored.
- Names are unique display identities, not action selectors.
- Repeated attach is idempotent only when name, canonical root, and image
  selector all match.
- A host `--cwd` must be inside the referenced Tobari root and maps byte-for-byte by
  relative path under `/workspace`.
- The command after `exec --` is positional and repeatable; Tobari does not
  parse or infer its meaning.
- Shared cluster mutations use one command-bound `tool_local` target.
- Individual actions require exactly one `tobari_id` emitted by `list` and
  never discover, decode, reconstruct, or guess a target by name.

## Output and exit contract

Human output is concise text. Cluster status JSON is schema version 1 and
`list --format json` is schema version 2, with the selected image added to each
item. Agent help uses the catalog schema. Successful data is stdout; failures
are stderr.

| Exit | Meaning |
|---:|---|
| 0 | Success |
| 2 | Invalid command or input |
| 3 | Internal or Docker execution failure |
| 9 | Required runtime temporarily unavailable |
| 10 | Policy or diagnostic rejection |
| 11 | Caller cancellation |
| 13 | Declared contract violation |
| other from `exec` | Exact child process exit status when Docker started it |

Commands use complete delivery. `list` is exhaustive for local state at one
observation point. Status is exhaustive for the shared cluster scope. Logs are
a bounded recent window of 1 through 10,000 lines per selected component.

## Configuration contract

Configuration is resolved from
`${XDG_CONFIG_HOME:-$HOME/.config}/tobari` on both macOS and Linux:

- `config.json`: schema-v1 default Tobari image selector;
- `policy/`: Rego and data mounted read-only into OPA and watched for host edits;
- `credentials.json`: schema-v1 profile type, exact hosts, and Gateway mount path;
- `credentials/`: secret files, required to be regular owner-readable files
  with no group/other permissions.

Runtime state is stored under `${XDG_STATE_HOME:-$HOME/.local/state}/tobari`.
The state contains paths and Docker resource names, never credential contents.
Environment variables select only XDG locations and test/runtime overrides
documented in scoped help; they do not carry managed tokens.

Image selection precedence is an explicit `attach --devcontainer` image,
explicit `attach --image`, `config.json.default_image`, then `builtin` when the
configuration file has not yet been initialized. The two explicit flags cannot
be supplied together.

## Side effects

`cluster up` creates shared labeled networks, images, configuration material,
Gateway, OPA, and CA volumes. It tags the built-in runtime both by asset
version and as the local extension base `tobari-runtime:local`. `attach`
inspects the selected local image for compatibility, creates one labeled
container, one internal network, and one home volume, then joins Gateway to
that network.
`detach` removes only the exact label-owned container and network; `--purge`
also removes that exact home. `cluster down` rejects while any Tobari remains
and removes only exact shared resources; its `--purge` also removes shared CA
volumes. No command removes a mounted root or files inside it.

## Compatibility

Before v1.0, command details and configuration schema may change with release
notes. Schema-1 singleton state is not guessed or migrated automatically; users
remove it with the matching older binary before starting the schema-2 cluster.
Command names, resource labels, state schema, OPA input version, audit schema,
and Gateway decision schema remain explicit compatibility boundaries.

## Unsupported outcomes

The deliberate non-goals in [Project Theses](00_theses.md) are not hidden
commands or transport escape hatches. In particular, Tobari does not promise to
control non-proxy-aware traffic semantically; it prevents all direct egress and
supports HTTP/HTTPS through the explicit proxy only.
