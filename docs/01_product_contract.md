# Product Contract

## Product statement

Tobari is a local CLI that runs one shared Gateway and OPA cluster and creates
or reuses Docker-isolated coding spaces selected from the current project
directory. Every supported outbound HTTP and HTTPS request is enforced through
that shared policy boundary.

## Primary users and owned outcome

The primary user is a developer who wants an autonomous coding agent to edit a
bounded source tree without receiving host credentials or unrestricted network
egress. `cd project && tobari` owns the routine lifecycle from logical state
resolution through startup, recovery, and interactive entry. The user normally
manages only the directory; a Tobari either exists or does not exist.

The primary operating loop is progressive policy learning: a Tobari workload is
denied by default, Gateway records the rejected host/method/path and reason
without secrets, `policy tail` presents a bounded exact proposal and opaque ID,
the user runs `policy allow --id`, and the same workload is retried. Trusted-host
Rego editing plus `policy apply` remains the advanced path for behavior that
exact learned rules cannot express. OPA watch may make host edits visible when
Docker-host filesystem events propagate. Denial evidence is a product output,
not incidental debug noise.

## Public vocabulary

- **Tobari:** one long-lived logical untrusted execution environment selected
  by a canonical project root. Its work container is recoverable runtime
  implementation detail.
- **cluster:** the one installation-local Gateway, OPA, policy, credential, and
  CA lifecycle.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **root:** the canonical host directory selected from the current working
  directory and mounted read-write into one Tobari at `/workspace`.
- **Tobari home:** a per-Tobari persistent owner-only XDG state directory
  mounted as the work user's home.
- **Tobari image:** the built-in runtime or one locally available compatible
  OCI image selected when a Tobari is first created.
- **Tobari ID:** a generated stable internal identity used for state and exact
  resource labels. It is diagnostic output, not a routine user action input.
- **credential profile:** a Gateway-only secret and exact host binding.

The public commands are:

| Command | Role | Effect | Outcome |
|---|---|---|---|
| `help [selector] [--format text|agent]` | utility | read | Discover exact command contracts |
| `version` | utility | read | Print build identity |
| `doctor [--root PATH]` | utility | read | Validate host, Docker, configuration, policy, secret permissions, ports, and residue |
| `tobari` | act, fixed target | create | Resolve or create the current directory's Tobari, reconcile runtime, and enter it |
| `status` | utility | read | Inspect the nearest current-directory Tobari and its diagnostic runtime state |
| `list [--format text|json]` | utility | read | List local Tobari roots with runtime diagnostics and diagnostic IDs |
| `delete [--force]` | act, fixed target | write | Delete the nearest current-directory Tobari after destructive confirmation |
| `cluster status [--format text|json]` | utility | read | Inspect shared state, health, proxy, policy, and recent errors |
| `cluster denials [--tail N] [--format text|json]` | utility | read | Read a bounded typed denial window, exact-rule learnability, policy path, and activation command |
| `cluster logs [--component gateway|opa|all] [--tail N]` | utility | read | Read bounded shared logs, including policy-denial evidence |
| `cluster down [--purge]` | act, fixed target | write | Remove shared transient resources after every logical Tobari is deleted |
| `policy candidates [--tail N] [--format text|json]` | discover | read | Discover unique pending exact-rule candidates and opaque IDs |
| `policy tail [--tail N]` | discover | read | Review the bounded pending queue with exact approval commands |
| `policy allow --id ID` | act, reference bound | write | Test, record, and activate one exact observed permission |
| `policy compactions [--format text|json]` | discover | read | Discover safe bounded prefix-compaction candidates and opaque IDs |
| `policy compact --id ID` | act, reference bound | write | Test and activate one current learned-rule compaction |
| `policy apply` | act, fixed target | write | Test host policy and activate it in the exact shared OPA component |

The root command is interactive and requires a TTY. It does not silently create
state in a non-interactive context. Programs inside Tobari can mutate the
explicitly mounted root; that delegated capability is a documented security
property rather than an undeclared Docker mutation by the CLI.

## Input and path contract

- The current working directory is expanded and canonicalized on the host
  before state or Docker calls. Lookup walks canonical parents and selects the
  nearest existing root; it never creates nested Tobari environments.
- The first creation uses the canonical current directory as root. Project
  moves and copies are not inferred or recorded in the project tree.
- The selected root is mounted at `/workspace/<canonical-root-without-leading-slash>`
  and the container workdir mirrors the host CWD below that path. Thus a root
  at `/work` and a host CWD of `/work/root` enter at `/workspace/work/root`.
- The configured image accepts `builtin` or a portable OCI image reference. A
  custom image must already exist locally and preserve runtime API `1`, the
  `tobari` image user, and the Tobari entrypoint. Tobari never pulls an image
  implicitly.
- The repository's optional `tobari-toolbox:local` recipe is a reviewed custom
  image workflow, not another built-in selector. Its explicit build verifies
  pinned vendor artifacts and runtime compatibility; root runtime startup
  neither build nor pull it.
- An explicitly configured Dev Container file is one regular file below the
  canonical root. The supported JSONC subset requires
  one literal `image` and permits only inert `$schema`, `name`, and
  `customizations` metadata. Dockerfile, Compose, Features, mounts,
  environment, user, privileges, capabilities, ports, and lifecycle properties
  are rejected rather than ignored.
- Shared cluster mutations use one command-bound `tool_local` target.
- CWD-local lifecycle operations use one command-bound `tool_local` current
  directory target and do not accept an ID, name, or root selector.

## Output and exit contract

Human output is concise text. Cluster status and cluster-denials JSON are
schema version 1. `list --format json` reports root, runtime diagnostic, and
stable ID. Agent help uses the catalog schema. Successful data is stdout;
failures are stderr.

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
Policy candidates and tail are bounded by the same retained Gateway-line
window and omit effects already covered by learned rules. Compactions are
exhaustive for the current validated learned-rule file at one observation.

## Configuration contract

Configuration is resolved from
`${XDG_CONFIG_HOME:-$HOME/.config}/tobari` on both macOS and Linux:

- `config.json`: schema-v1 default Tobari image selector;
- `policy/`: Rego and data mounted read-only into OPA and watched for host edits;
- `credentials.json`: schema-v1 profile type, exact hosts, and Gateway mount path;
- `credentials/`: secret files, required to be regular owner-readable files
  with no group/other permissions.

Runtime state is stored under `${XDG_STATE_HOME:-$HOME/.local/state}/tobari`:
`roots/<hash>.json` indexes canonical roots and
`instances/<id>/state.json` contains one logical instance and diagnostic runtime
identifiers. `instances/<id>/home` is the writable home. Shared read-only agent
profiles are under `${XDG_DATA_HOME:-$HOME/.local/share}/tobari/profiles`.
State contains paths and Docker resource names or identifiers, never credential
contents.
Environment variables select only XDG locations and test/runtime overrides
documented in scoped help; they do not carry managed tokens.

Image selection uses configured bounded image metadata, then
`config.json.default_image`, then `builtin` when configuration has not yet been
initialized. Project metadata can select only a literal compatible image.

OPA reads the policy bind with `--watch`. Because Docker-host file-event
delivery varies by runtime, `policy apply` is the portable completion step: it
tests the current host directory before recreating only OPA and waiting for
health.

## Side effects

The root command creates shared labeled networks, images, configuration
material, Gateway, OPA, and CA volumes as needed. It tags the built-in runtime
both by asset version and as the local extension base `tobari-runtime:local`.
It creates or reuses the current root's labeled container and internal network,
binds its XDG home, joins Gateway to that network, and enters the resulting
terminal session. `delete` removes only that exact label-owned container,
network, root index, instance state, and home. `cluster down` rejects while any
Tobari remains
and removes only exact shared resources; its `--purge` also removes shared CA
volumes. No command removes a mounted root or files inside it.
`policy apply` runs pinned OPA tests against the read-only host bind, then
recreates only the exact owner-labeled OPA container and waits for health.
Gateway remains up and fails closed during that bounded activation interval.
`policy allow` and `policy compact` first build and test the complete candidate
policy in a private host temporary directory. After successful tests they
atomically replace only `policy/data.json` and invoke the same activation
boundary. They never write Rego source or credential files.
OPA marks a denial learnable only when its version, cluster, scheme, and
credential binding already satisfy the orthogonal boundary. Candidate
discovery excludes other denials, preventing a successful no-op approval.

## Compatibility

Before v1.0, command details and configuration schema may change with release
notes. Legacy named state is not guessed or migrated automatically; users clean
it with the matching older binary before adopting the CWD-owned lifecycle.
Command names, resource labels, state schema, OPA input version, audit schema,
and Gateway decision schema remain explicit compatibility boundaries.

## Unsupported outcomes

The deliberate non-goals in [Project Theses](00_theses.md) are not hidden
commands or transport escape hatches. In particular, Tobari does not promise to
control non-proxy-aware traffic semantically; it prevents all direct egress and
supports HTTP/HTTPS through the explicit proxy only.
