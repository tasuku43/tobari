# Product Contract

## Product statement

Tobari is a local CLI that creates one Docker-isolated coding realm for a
user-selected source root and enforces all outbound HTTP and HTTPS requests
through a mitmproxy Gateway and OPA policy.

## Primary users and owned outcome

The primary user is a developer who wants an autonomous coding agent to edit a
bounded source tree without receiving host credentials or unrestricted network
egress. Tobari owns the lifecycle from environment validation through startup,
interactive or non-interactive execution, inspection, logs, and cleanup.

The primary operating loop is progressive policy learning: a Realm workload is
denied by default, Gateway records the rejected host/method/path and reason
without secrets, the user refines and tests policy on the trusted host, and the
same workload is retried. Denial evidence is a product output, not incidental
debug noise.

## Public vocabulary

- **Realm:** the single long-lived, untrusted execution container.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **root:** the only host directory mounted read-write at `/workspace`.
- **Realm home:** a persistent Docker volume mounted as the work user's home.
- **credential profile:** a Gateway-only secret and exact host binding.

The public commands are:

| Command | Role | Effect | Outcome |
|---|---|---|---|
| `help [selector] [--format text|agent]` | utility | read | Discover exact command contracts |
| `version` | utility | read | Print build identity |
| `doctor [--root PATH]` | utility | read | Validate host, Docker, configuration, policy, secret permissions, ports, and residue |
| `up --root PATH` | act, fixed target | create | Reconcile and start the one Tobari realm |
| `status [--format text|json]` | utility | read | Inspect state, containers, health, root, proxy, policy, and recent errors |
| `shell` | act, fixed target | read | Open an interactive shell in the running Realm |
| `exec [--cwd PATH] -- COMMAND...` | act, fixed target | read | Execute one command in Realm and preserve its exit code |
| `logs [--component gateway|opa|realm|all] [--tail N]` | utility | read | Read a bounded redacted log window, including policy-denial evidence |
| `down [--purge]` | act, fixed target | write | Remove owned containers and transient networks; optionally remove persistent home |

`shell` and `exec` intentionally classify the CLI operation as read because the
CLI observes/enters the existing local singleton. Programs inside Realm can
mutate the explicitly mounted root; that delegated capability is a documented
security property rather than an undeclared Docker mutation by the CLI.

## Input and path contract

- Paths are expanded and canonicalized on the host before Docker calls.
- `up --root` requires an existing directory.
- A host `--cwd` must be inside the stored root and maps byte-for-byte by
  relative path under `/workspace`.
- The command after `exec --` is positional and repeatable; Tobari does not
  parse or infer its meaning.
- A running realm is the one command-bound `tool_local` target. Commands never
  discover or guess among multiple realms.

## Output and exit contract

Human output is concise text. `status --format json` is schema version 1 under
`status`; agent help uses the catalog schema. Successful data is stdout,
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
| other from `exec` | Exact child process exit status when Docker started it |

Commands use complete delivery. Status covers the one local realm at one
observation point and is exhaustive for that scope. Logs are a bounded recent
window of 1 through 10,000 lines per selected component.

## Configuration contract

Configuration is resolved from
`${XDG_CONFIG_HOME:-$HOME/.config}/tobari` on both macOS and Linux:

- `policy/`: Rego mounted read-only into OPA;
- `credentials.json`: schema-v1 profile type, exact hosts, and Gateway mount path;
- `credentials/`: secret files, required to be regular owner-readable files
  with no group/other permissions.

Runtime state is stored under `${XDG_STATE_HOME:-$HOME/.local/state}/tobari`.
The state contains paths and Docker resource names, never credential contents.
Environment variables select only XDG locations and test/runtime overrides
documented in scoped help; they do not carry managed tokens.

## Side effects

`up` creates labeled networks, a persistent volume, images, configuration
material, and three containers. `down` removes only exact label-owned
containers and transient networks; `--purge` also removes the exact persistent
home and Gateway CA volumes. A subsequent `up` tests current Rego and restarts
only OPA to reload policy without stopping Realm processes. No command removes
the mounted root or files inside it.

## Compatibility

Before v1.0, command details and configuration schema may change with release
notes. Command names, resource labels, state schema, OPA input version, audit
schema, and Gateway decision schema are explicit compatibility boundaries from
the first MVP.

## Unsupported outcomes

The deliberate non-goals in [Project Theses](00_theses.md) are not hidden
commands or transport escape hatches. In particular, Tobari does not promise to
control non-proxy-aware traffic semantically; it prevents all direct egress and
supports HTTP/HTTPS through the explicit proxy only.
