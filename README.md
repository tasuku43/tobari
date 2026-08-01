# Tobari

Tobari creates or reuses one long-lived Docker-isolated coding space for the
current project directory. One installation-local Gateway and OPA cluster
enforces every supported outbound HTTP and HTTPS request for every Tobari.

Tobari does not guess intent from command strings. It controls the network
effect at the point where an HTTP request crosses an isolation boundary.

## How it feels to use

The normal loop is progressive policy learning:

1. Run `tobari` from a project directory.
2. Work freely until an undeclared request receives `403`.
3. Review the secret-free pending queue.
4. Approve one exact host, method, and path rule by opaque ID, then retry.

```sh
cd ~/ghq/example
tobari
tobari status
tobari list

# The ID in list is diagnostic only; lifecycle actions use the current directory.
tobari policy tail --tail 100
# Copy one exact opaque ID printed by policy tail.
tobari policy allow --id pcy_0123456789abcdef0123456789abcdef
```

The queue includes bounded `host`, `method`, `path`, and `reason` evidence plus
the exact approval command. It includes only denials OPA marks resolvable by an
exact learned rule; immutable scheme, cluster, and credential-binding failures
remain diagnostics instead of becoming ineffective approvals. `policy allow`
resolves the opaque ID against retained denials, tests the complete policy,
atomically records one exact rule, and activates it. Tobari never turns
observed traffic into permission automatically.

## Cluster and Tobari topology

```text
trusted host
  Tobari CLI ── Docker CLI ── Docker Engine
       │
       ├── root A (rw) ── CWD-owned Tobari A ── internal network A ──┐
       └── root B (rw) ── CWD-owned Tobari B ── internal network B ──┤
                                                                 ▼
                                                       trusted Gateway
                                                          │          │
                                             internal control       egress
                                                          │          │
                                                         OPA      HTTPS
```

Each Tobari has its own internal network and persistent XDG home directory. Gateway
alone joins that network. OPA joins only the shared control network, and only
Gateway joins egress. A program that ignores the proxy has no external route;
one Tobari cannot directly reach OPA or another Tobari.

For HTTPS, `HTTPS_PROXY` points to `http://gateway:8080`. The client sends
`CONNECT host:443`, establishes TLS with Gateway using the Tobari CA, and sends
the decrypted HTTP request to Gateway. Gateway asks OPA and, only after allow,
creates a separate verified TLS connection to the upstream. This is HTTPS on
both sides of the policy boundary, not plaintext traffic to the destination.

Certificate-pinned clients that reject the Tobari CA fail rather than bypass
Gateway.

Proxy-aware tools such as `gh`, Git over HTTPS, and `curl` receive the same
`HTTP_PROXY` and `HTTPS_PROXY` settings. Their destination remains an HTTPS
URL: the client uses HTTP `CONNECT` to reach Gateway, Gateway authorizes the
decrypted request, and the upstream leg is a separate verified HTTPS
connection. No GitHub-specific URL rewriting or adapter is required. Tool
authentication prerequisites still belong to that tool or an explicitly
configured Gateway credential profile.

## Requirements

- macOS or Linux on a Docker-supported architecture
- Docker Engine 24 or newer
- Docker Compose v2
- Go version declared in [`go.mod`](go.mod) for source builds
- [Task](https://taskfile.dev/) for development commands
- outbound image-registry access on first startup

Docker Desktop-specific APIs are not used. Colima, Lima-based Docker contexts,
and standard Linux Docker Engine use the same Docker CLI adapter.

Container bases are pinned by immutable digest in
[`versions.env`](internal/infra/runtimeassets/assets/versions.env).

## Install from source

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build
install -m 0755 bin/tobari ~/.local/bin/tobari
```

Alternatively:

```sh
go install ./cmd/tobari
```

Ensure the destination is on `PATH`.

## Quick start

Validate the host and intended project directory:

```sh
tobari doctor --root ~/ghq/example
```

Start the shared enforcement cluster explicitly:

```sh
tobari cluster up
```

In an interactive terminal, `cluster up` shows a compact colored three-phase
checklist (`prepare environment`, `start services`, and `verify readiness`) on
stderr while it prepares, starts, and verifies the shared services. Its
completed checklist remains visible, followed by the same clean cluster
summary shown by `cluster status` on stdout and a next-step hint for running
`tobari`. Non-interactive or machine-readable callers receive no progress or
color control sequences.

The same human output language is used by the other commands: an outcome
heading, aligned detail rows, semantic colors, explicit empty states, and an
exact next action where one is useful. `doctor` defaults to this view;
`tobari doctor --format tsv` remains available for tab-separated consumers.
Help, JSON, agent help, logs, and `exec` keep their respective machine or raw
data contracts and never receive terminal styling.

Then run the primary operation from the project directory. It requires the
cluster to be configured and ready, and creates or reuses only the project
Workspace:

```sh
tobari
```

The root command does not create or repair the shared cluster. It requires a
TTY and enters the container at the mirrored host working directory. If the
current directory is below existing Workspace roots, `tobari` shows an English
selector with the containing roots nearest-first plus `Create a new Workspace
here`. Use the arrow keys and Enter to reuse a Workspace, `n` to create at the
current directory, or `q`/Escape to cancel. When raw terminal mode is
unavailable, the same experience falls back to numbered line input. For
example, from `/work/root/app`, existing roots at `/work/root` and `/work`
are shown as two choices before the create-here option. A selected Workspace
root at `/work` enters `/workspace/work/root/app`; a shell exit returns to the
host while the Workspace remains existing. Tobari prints the following host
guidance on stderr after the child session returns:

```text
Workspace session closed.
Workspace remains available.

Resume: tobari
Remove: tobari delete
If another session is attached: tobari delete --force
```

`exit` therefore leaves the session but does not stop or delete the Workspace.
There is no `stop` command or stopped state. To remove a detached Workspace,
run `tobari delete` from the host; it deletes the nearest canonical Workspace
containing the current directory. If another terminal is attached, the command
warns and fails; use `tobari delete --force` only when terminating that session
is intentional. Each canonical root can have only one Workspace, including
when explicit creation requests race.

The lifecycle model is:

```text
Workspace absent -> tobari -> Attached session + Workspace exists
Attached session + Workspace exists -> exit -> Detached session + Workspace exists
Detached session + Workspace exists -> tobari -> Attached session + Workspace exists
Detached session + Workspace exists -> tobari delete -> Workspace absent
```

`list` shows the stable ID only as diagnostic information, not as a routine
action input.

The former named lifecycle commands (`attach`, `lower`, `enter`, `lift`, and
their named shell/exec forms) are rejected with a replacement message; they do
not create a second lifecycle model. Legacy named state is not guessed or
automatically migrated.

Agent CLIs are not bundled. Install them inside a Tobari or place binaries below
its selected root. The per-Tobari home survives shell exit and runtime recovery.

### Common CLI toolbox

Repeated policy-learning exercises can use the optional local toolbox image.
It contains Git, GitHub CLI, AWS CLI v2, kubectl, TWG, curl, jq, SSH, rsync,
and basic DNS tools. Build and validate the optional toolbox on the trusted
host:

```sh
task toolbox:build
cd ~/ghq/example
tobari
```

Set it once as the usual image by changing the owner-only XDG
`config.json`:

```json
{
  "version": "v1",
  "default_image": "tobari-toolbox:local"
}
```

The versions are pinned in `images/toolbox/versions.env`. Vendor downloads are
verified during the build, and the build finishes by checking every named CLI
plus the inherited Tobari runtime label, user, and entrypoint. The image is
local and optional: Tobari does not pull it implicitly or rebuild it during
ordinary root invocation.

The toolbox contains no credentials and does not mount host CLI configuration.
Authenticate deliberately within the isolated environment or use a supported
Gateway credential profile. AWS SigV4 and OAuth refresh remain outside the MVP
credential-injection contract. Git over HTTPS uses the Gateway; Git over SSH
and other non-HTTP transports have no direct egress route.

### Custom work images

`cluster up` also builds `tobari-runtime:local`, a stable local extension base.
Add tools without replacing its user or entrypoint:

```dockerfile
FROM tobari-runtime:local

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs npm \
    && rm -rf /var/lib/apt/lists/*
USER tobari
```

Build it explicitly on the trusted host, then select it in the owner-only XDG
`config.json` before the first root invocation:

```sh
docker build --tag my-tobari:dev .
```

For the usual case, set the XDG default once:

```json
{
  "version": "v1",
  "default_image": "my-tobari:dev"
}
```

Then ordinary root invocations stay short:

```sh
tobari
```

Image selection uses `config.json.default_image`, then `builtin` before
configuration is initialized.

Tobari never pulls a configured image implicitly. The image must be available locally
and preserve runtime API `1`, the `tobari` image user, and the inherited
entrypoint. Prefer a digest selector when reproducibility matters. The
compatibility check is not a signature or trust decision: image contents remain
untrusted and run under the same fixed non-root user, read-only root filesystem,
dropped capabilities, mounts, proxy, and internal network as the built-in
image. To change an existing Tobari's image, delete it and run `tobari` again;
the new logical environment receives a new home.

### Dev Container image definitions

Tobari can read an explicit image-based Dev Container definition inside the
selected root:

```jsonc
{
  "name": "work",
  "image": "my-tobari:dev",
  "customizations": {}
}
```

```sh
cd ~/ghq/example
tobari
```

When `.devcontainer/devcontainer.json` exists below the selected root, Tobari
uses its one literal locally available compatible `image`. JSON comments and
trailing commas are accepted, and only inert `$schema`, `name`, and
`customizations` are allowed alongside it.
Effectful Dev Container properties—including `build`, Compose, Features,
mounts, environment, users, privileges, capabilities, ports, and lifecycle
commands—fail with `unsupported_devcontainer`. Tobari does not invoke the Dev
Container CLI or let the definition replace its isolation boundary. See the
[Dev Container specification](https://github.com/devcontainers/spec/blob/main/docs/specs/devcontainer-reference.md)
for the broader format that Tobari deliberately does not claim to implement.

Delete the selected detached Tobari and its per-Tobari home:

```sh
tobari delete
```

If another terminal is still attached to the Workspace, deletion warns and
fails. Add `--force` only to explicitly override that guard:

```sh
tobari delete --force
```

The shared cluster can be removed only after every Tobari is deleted:

```sh
tobari cluster down
tobari cluster down --purge # also removes shared CA volumes
```

## Commands

| Command | Outcome |
|---|---|
| `tobari cluster up` | Test policy and reconcile shared Gateway and OPA |
| `tobari cluster status [--format text\|json]` | Show shared readiness, component health, policy, project count, and diagnostics |
| `tobari cluster denials [--tail N] [--format text\|json]` | Read typed denial evidence, policy path, and activation command |
| `tobari cluster logs [--component gateway\|opa\|all] [--tail N]` | Read bounded shared logs and denial evidence |
| `tobari cluster down [--purge]` | Remove an empty cluster and optionally shared CA state |
| `tobari policy candidates [--tail N] [--format text\|json]` | Discover pending exact approvals and opaque IDs |
| `tobari policy tail [--tail N]` | Review the bounded queue with exact approval commands |
| `tobari policy allow --id ID` | Test, store, and activate one exact observed permission |
| `tobari policy compactions [--format text\|json]` | Discover test-backed prefix compactions and opaque IDs |
| `tobari policy compact --id ID` | Test and activate one current bounded compaction |
| `tobari policy apply` | Test host policy, recreate only OPA, and wait for health |
| `tobari` | Choose or create the current-directory Workspace, enter it, and leave it reusable after `exit` |
| `tobari status [--format text\|json]` | Report logical existence and runtime diagnostics for the current directory |
| `tobari list [--format text\|json]` | List local Workspaces, runtime diagnostics, and diagnostic IDs |
| `tobari delete [--force]` | Delete the nearest detached current-directory Tobari; `--force` overrides an attached-session guard |
| `tobari doctor [--root PATH] [--format text\|tsv\|json]` | Diagnose Docker, paths, policy, credentials, and residue |
| `tobari help [SELECTOR] [--format text\|agent]` | Read human or machine command contracts |
| `tobari version` | Print build identity |

`cluster status`, `cluster denials`, `policy candidates`, `policy tail`,
`policy compactions`, `status`, `list`, and `doctor` are observational and
never reconcile Docker or create/delete runtime resources. They may clear an
exact durable journal before selecting logical state. Runtime recovery belongs
only to the root `tobari` operation.

## XDG configuration and live policy

On macOS and Linux, Tobari uses the same XDG paths:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/tobari/
  config.json
  policy/
    data.json
    tobari.rego
    tobari_test.rego
  credentials.json
  credentials/

${XDG_STATE_HOME:-$HOME/.local/state}/tobari/
  roots/<root-hash>.json
  instances/<tobari-id>/state.json
  instances/<tobari-id>/home

${XDG_DATA_HOME:-$HOME/.local/share}/tobari/
  profiles/default/   # shared read-only agent profile
```

OPA sees `policy/` through a read-only bind and runs with file watch enabled.
A read-only bind still reflects host changes; OPA does not need write authority
over trusted policy. Some Docker hosts do not propagate host filesystem events
to OPA watch reliably, so finish every deliberate edit with:

```sh
tobari policy apply
```

This tests the current host policy, recreates only the exact owned OPA
container, and waits for health. Gateway remains up and fails closed during the
brief activation interval; active Tobari are not restarted. Where file events
do propagate, watch may make the change visible before this command completes,
but `policy apply` remains the portable confirmation.

Use the policy directory as a trusted-host path when editing policy; do not
mount its parent configuration directory into a Tobari:

```sh
${EDITOR:-vi} "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/policy/tobari.rego"
```

Keep the `policy/` subdirectory separate from the parent Tobari configuration
directory. The parent also contains credential metadata and secret files that
must remain outside untrusted containers.

The initialized policy is generic HTTP policy, not a GitHub adapter. It starts
deny-by-default, distinguishes HTTPS from explicitly allowed test-only HTTP,
restricts methods and paths, and validates credential profile bindings.

### Grow and compact learned policy

Use the human review queue during normal work:

```sh
tobari policy tail --tail 200
tobari policy allow --id PCY_ID
```

`PCY_ID` must be copied unchanged from `policy tail` or `policy candidates`.
It expires when its denial falls outside retained logs or another learned rule
already covers that exact effect. Repeating the same denied host/method/path
retains the same ID and refreshes its evidence. Approval never accepts a host
wildcard, method wildcard, prefix, or user-supplied pattern.

After at least three exact rules accumulate under the same sufficiently deep
directory for one host and method, review an optional compaction:

```sh
tobari policy compactions
tobari policy compact --id PCX_ID
```

Compaction is never automatic. Its opaque ID binds the current source-rule set.
The replacement keeps all positive examples, tests an adjacent outside-prefix
canary, and is rejected if the source set changed. These finite tests catch the
declared boundary regression; they do not prove every unknown future path is
safe.

For advanced policy behavior that the exact learning flow cannot express, edit
the XDG Rego and data files on the trusted host, add tests, then run
`tobari policy apply`.

Run read-only diagnostics when you want to test policy without activating it:

```sh
tobari doctor
```

An invalid watched edit does not authorize traffic. Inspect OPA logs and correct
the policy:

```sh
tobari cluster logs --component opa --tail 100
```

## Credential injection

Tobari supports static bearer and fixed-header injection. Create an owner-only
secret file on the trusted host:

```sh
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/tobari"
install -d -m 0700 "$config_dir/credentials"
install -m 0600 /path/to/token "$config_dir/credentials/github-development"
```

Configure metadata only in `credentials.json`:

```json
{
  "version": "v1",
  "profiles": {
    "github-development": {
      "type": "bearer",
      "hosts": ["api.github.com"],
      "secret_file": "/run/tobari/credentials/github-development"
    }
  }
}
```

Add the same profile-to-host binding to policy data. A process inside a Tobari
requests the non-secret profile:

```sh
curl \
  -H 'X-Tobari-Credential-Profile: github-development' \
  https://api.github.com/user
```

OPA must allow the request and select that profile. Gateway independently
checks the exact host binding, reads the secret, removes client-supplied
authorization, and injects the managed header. The secret is absent from
Tobari mounts, environment, CLI argv, OPA input, and audit logs.

## Security guarantees

Under the documented topology and trusted-component assumptions:

- Each Tobari can write only its selected root and exact XDG home directory.
- Tobari have no Docker socket, SSH agent, host networking, privileged mode, or
  added Linux capabilities.
- Direct Internet egress has no route.
- Tobari cannot reach OPA, Gateway credential files, or another Tobari.
- HTTP/HTTPS requests fail closed when OPA or Gateway fails.
- Managed credentials are injected only after allow and exact host binding.
- Audit logs contain request metadata and decisions, not secret values or raw
  bodies.
- Cleanup verifies exact owner and opaque Tobari-ID labels.
- OPA cannot rewrite the host XDG policy.

Read [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md) for assumptions and abuse
cases.

## What Tobari does not guarantee

- A Tobari can modify or delete every file below its selected root.
- An allowed destination can receive data from that root.
- An allowed credential can exercise all provider authority it grants.
- Tobari does not protect against Docker/host compromise, container or VM
  escape, covert channels, malware, or interference among processes inside the
  same Tobari.
- Proxy variables do not support applications that ignore proxies or pin
  certificates; those applications fail rather than bypass Gateway.
- HTTP/3/QUIC, raw TCP, UDP, Git SSH, and other non-HTTP protocols are
  unsupported.

## Troubleshooting

```sh
tobari doctor --root /absolute/root
tobari cluster status
tobari list
```

Common failures:

- `policy_test_failed`: edit the policy reported by `cluster denials` or
  `cluster status`, then run `doctor` or `policy apply`; if tests pass outside
  Tobari, verify that the XDG policy directory is shared with the Docker VM.
- HTTPS certificate error: confirm the program honors `SSL_CERT_FILE`,
  `REQUESTS_CA_BUNDLE`, or `GIT_SSL_CAINFO`.
- `tty_required`: run the root `tobari` command from an interactive terminal.
- `workspace_selection_stale`: the Workspace list changed during selection;
  run `tobari` again and choose from the refreshed list.
- cancelled or unavailable Workspace selection: choose an available candidate,
  press `n` to create explicitly at the current directory, or run `q` to leave.
- `already_inside`: exit the current Tobari before entering another session.
- `image_not_found`: build or pull the selected image explicitly on the host.
- `incompatible_image`: extend `tobari-runtime:local` without replacing its
  user or entrypoint.
- `project_not_found`: run `tobari` from the intended project directory.
- `project_session_attached`: exit the attached session and retry `tobari
  delete`, or use `tobari delete --force` only when terminating that session is
  intentional.
- intended request returns `403`: run `policy tail`, approve one exact candidate
  with `policy allow --id`, and retry; use `cluster denials` plus a tested host
  edit only when the exact learning flow cannot express the required behavior.
- root bind-mount error under Colima/Lima: use a directory shared with the VM.

Schema-1 singleton state from older pre-v1 builds is intentionally not guessed
or migrated. Remove it with the matching older binary before starting a
schema-2 cluster.

## Development and tests

Use the exact Go toolchain from `go.mod`:

```sh
task check:fast
task check
task policy:test
task gateway:test
task integration:test
task runtime:test
task security
task public:check
```

The integration profile creates two CWD-owned Tobari, dedicated internal
networks, one shared Gateway and OPA, and a mock upstream. It proves network
separation, HTTP and HTTPS enforcement, credential injection, fail-closed
outages, CWD resolution, runtime recovery, typed denial recovery, tested
host-policy activation, terminal exit behavior, concurrency, idempotency, and
exact cleanup.

## MVP exclusions

The MVP excludes multiple clusters, process-level identity, transparent
proxying, raw TCP/UDP/QUIC, Git SSH semantic inspection, provider-specific
adapters, AWS SigV4, OAuth refresh, GitHub App token refresh, Keychain
integration, approval workflows, policy engines other than OPA, Kubernetes,
filesystem overlays, private clone mode, GUI, remote execution, and production
multi-tenancy.

## License

Tobari is available under the [MIT License](LICENSE).
