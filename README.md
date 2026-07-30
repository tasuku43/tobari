# Tobari

Tobari attaches named Docker isolation spaces to the directories where coding
agents already work. One installation-local Gateway and OPA cluster enforces
every supported outbound HTTP and HTTPS request for every attached Tobari.

Tobari does not guess intent from command strings. It controls the network
effect at the point where an HTTP request crosses an isolation boundary.

## How it feels to use

The normal loop is progressive policy learning:

1. Start the shared enforcement cluster.
2. Attach a named Tobari to a work directory.
3. Work freely until an undeclared request receives `403`.
4. Inspect the secret-free Gateway denial.
5. Edit the host-side XDG policy and retry.

```sh
tobari cluster up
tobari attach --name work --root ~/ghq
tobari list

# Copy the exact opaque ID printed by list.
tobari shell --id tbr_0123456789abcdef0123456789abcdef
tobari cluster logs --component gateway --tail 100
```

Gateway logs include bounded `host`, `method`, `path`, `decision`, and `reason`
metadata. Tobari never turns observed traffic into permission automatically.

## Cluster and Tobari topology

```text
trusted host
  Tobari CLI ── Docker CLI ── Docker Engine
       │
       ├── root A (rw) ── named Tobari A ── internal network A ──┐
       └── root B (rw) ── named Tobari B ── internal network B ──┤
                                                                 ▼
                                                       trusted Gateway
                                                          │          │
                                             internal control       egress
                                                          │          │
                                                         OPA      HTTPS
```

Each Tobari has its own internal network and persistent home volume. Gateway
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

Validate the host and intended root:

```sh
tobari doctor --root ~/ghq
```

Create shared enforcement, then attach one or more roots:

```sh
tobari cluster up
tobari attach --name work --root ~/ghq
tobari attach --name config \
  --root "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/policy"
tobari list
```

`list` is the discover step. It emits one opaque ID per Tobari. Pass the exact
ID unchanged to actions:

```sh
tobari shell --id TBR_ID
tobari exec --id TBR_ID --cwd ~/ghq/github.com/example/repository -- codex
tobari exec --id TBR_ID -- curl https://example.com/
tobari logs --id TBR_ID --tail 100
```

Replace `TBR_ID` with the value printed by `list`; it is not a literal built-in
ID. `exec` preserves the invoked process exit status.

Agent CLIs are not bundled. Install them inside a Tobari or place binaries below
its selected root. Each named home survives ordinary detach/attach cycles.

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

Build it explicitly on the trusted host, then select it on first attach:

```sh
docker build --tag my-tobari:dev .
tobari attach --name work --root ~/ghq --image my-tobari:dev
```

For the usual case, set the XDG default once:

```json
{
  "version": "v1",
  "default_image": "my-tobari:dev"
}
```

Then ordinary attach calls stay short:

```sh
tobari attach --name work --root ~/ghq
```

Image selection precedence is explicit `--image`, then
`config.json.default_image`, then `builtin` before configuration is initialized.

Tobari never pulls `--image` implicitly. The image must be available locally
and preserve runtime API `1`, the `tobari` image user, and the inherited
entrypoint. Prefer a digest selector when reproducibility matters. The
compatibility check is not a signature or trust decision: image contents remain
untrusted and run under the same fixed non-root user, read-only root filesystem,
dropped capabilities, mounts, proxy, and internal network as the built-in
image. To change an attached Tobari's image, detach it and attach it again; its
home persists unless `--purge` is used.

Detach one Tobari while retaining its home:

```sh
tobari detach --id TBR_ID
```

Remove that exact home too:

```sh
tobari detach --id TBR_ID --purge
```

The shared cluster can be removed only after every Tobari is detached:

```sh
tobari cluster down
tobari cluster down --purge # also removes shared CA volumes
```

## Commands

| Command | Outcome |
|---|---|
| `tobari cluster up` | Test policy and reconcile shared Gateway and OPA |
| `tobari cluster status [--format text\|json]` | Show shared health, proxy, XDG policy, and attached count |
| `tobari cluster logs [--component gateway\|opa\|all] [--tail N]` | Read bounded shared logs and denial evidence |
| `tobari cluster down [--purge]` | Remove an empty cluster and optionally shared CA state |
| `tobari attach --name NAME --root PATH [--image IMAGE]` | Attach one named Tobari with a compatible local image |
| `tobari list [--format text\|json]` | Discover configured Tobari and opaque action IDs |
| `tobari shell --id ID` | Open Bash in one exact Tobari |
| `tobari exec --id ID [--cwd PATH] -- COMMAND...` | Execute exact argv and preserve its exit status |
| `tobari logs --id ID [--tail N]` | Read one Tobari's bounded logs |
| `tobari detach --id ID [--purge]` | Remove one Tobari and optionally its home |
| `tobari doctor [--root PATH] [--format tsv\|json]` | Diagnose Docker, paths, policy, credentials, and residue |
| `tobari help [SELECTOR] [--format text\|agent]` | Read human or machine command contracts |
| `tobari version` | Print build identity |

`cluster status`, `list`, `logs`, and `doctor` are observational and never
repair state.

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
  state.json
  runtime/
```

OPA sees `policy/` through a read-only bind and runs with file watch enabled.
A read-only bind still reflects host changes; OPA does not need write authority
over trusted policy. Valid host edits therefore take effect without restarting
OPA, Gateway, or an active Tobari.

Use a named Tobari rooted at the policy subdirectory when you want an isolated
policy-editing environment:

```sh
tobari attach --name policy \
  --root "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/policy"
```

Attach the `policy/` subdirectory, not the parent Tobari configuration
directory. The parent also contains credential metadata and secret files that
must remain outside untrusted containers.

The initialized policy is generic HTTP policy, not a GitHub adapter. It starts
deny-by-default, distinguishes HTTPS from explicitly allowed test-only HTTP,
restricts methods and paths, and validates credential profile bindings.

Run read-only diagnostics after an edit to execute the Rego tests:

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

Add the same profile-to-host binding to policy data. A Tobari client requests
the non-secret profile:

```sh
tobari exec --id TBR_ID -- curl \
  -H 'X-Tobari-Credential-Profile: github-development' \
  https://api.github.com/user
```

OPA must allow the request and select that profile. Gateway independently
checks the exact host binding, reads the secret, removes client-supplied
authorization, and injects the managed header. The secret is absent from
Tobari mounts, environment, CLI argv, OPA input, and audit logs.

## Security guarantees

Under the documented topology and trusted-component assumptions:

- Each Tobari can write only its selected root and exact home volume.
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

- `policy_test_failed`: edit the policy reported by `cluster status`, then run
  `doctor`.
- HTTPS certificate error: confirm the program honors `SSL_CERT_FILE`,
  `REQUESTS_CA_BUNDLE`, or `GIT_SSL_CAINFO`.
- `cluster_not_running`: run `cluster up` before `attach`.
- `image_not_found`: build or pull the selected image explicitly on the host.
- `incompatible_image`: extend `tobari-runtime:local` without replacing its
  user or entrypoint.
- `image_conflict`: detach before changing an existing Tobari's image.
- `tobari_not_found`: pass an opaque ID from `list` unchanged.
- intended request returns `403`: inspect `cluster logs --component gateway`
  and refine the minimum host/method/path rule.
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

The integration profile creates two named Tobari, dedicated internal networks,
one shared Gateway and OPA, and a mock upstream. It proves network separation,
HTTP and HTTPS enforcement, credential injection, fail-closed outages, opaque
reference flow, live XDG policy watch, exit-code preservation, concurrency,
idempotency, and exact cleanup.

## MVP exclusions

The MVP excludes multiple clusters, process-level identity, transparent
proxying, raw TCP/UDP/QUIC, Git SSH semantic inspection, provider-specific
adapters, AWS SigV4, OAuth refresh, GitHub App token refresh, Keychain
integration, approval workflows, policy engines other than OPA, Kubernetes,
filesystem overlays, private clone mode, GUI, remote execution, and production
multi-tenancy.

## License

Tobari is available under the [MIT License](LICENSE).
