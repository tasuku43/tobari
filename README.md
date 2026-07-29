# Tobari

Tobari is a local execution boundary for AI coding agents. It gives shells,
Codex, Claude Code, Python, and arbitrary binaries broad freedom inside one
long-lived Docker Realm while authorizing every supported outbound HTTP and
HTTPS request with OPA.

Tobari does not guess intent from command strings. It controls the network
effect at the point where an HTTP request crosses the isolation boundary.

## The policy-learning loop

Tobari starts deny-by-default. The normal experience is:

1. Work freely inside Realm.
2. An undeclared external request receives `403`.
3. On the trusted host, inspect the secret-free denial:

   ```sh
   tobari logs --component gateway --tail 100
   ```

4. Read the `host`, `method`, `path`, and `reason` fields from the JSON audit
   record.
5. Locate the editable policy:

   ```sh
   tobari status
   ```

6. Add the smallest intended rule and reconcile it:

   ```sh
   tobari up --root /absolute/path/to/root
   ```

`up` runs every Rego test before changing the running decision point. On an
existing Realm it then reloads policy by restarting only OPA and waiting for
health; active Realm processes remain running. Tobari never turns observed
traffic into permission automatically.

## Architecture

```text
trusted host
  tobari CLI ── Docker CLI ── Docker Engine
       │
       └── selected root (read-write)
                         │
                         ▼
  internal realm network
    untrusted tobari-realm ── HTTP proxy :8080 ── trusted tobari-gateway
                                                        │
  internal control network                              ├── tobari-opa :8181
                                                        │
  egress network                                        └── external HTTPS
```

The Realm joins only `tobari-realm-net`, an internal Docker network. Gateway
joins realm, control, and egress networks. OPA joins only the internal control
network. Therefore a program that ignores the proxy has no external route, and
Realm cannot reach OPA.

For HTTPS, `HTTPS_PROXY` still points to an `http://` proxy endpoint. The client
sends `CONNECT host:443`, then establishes TLS with Gateway using the
installation CA trusted inside Realm. Gateway decrypts and normalizes the HTTP
request, asks OPA, and—only after allow—creates a separate verified TLS
connection to the upstream. Certificate-pinned applications that refuse the
Tobari CA are not supported by the MVP.

## Requirements

- macOS or Linux on a Docker-supported architecture
- Docker Engine 24 or newer
- Docker Compose v2
- Go version declared in [`go.mod`](go.mod) for source builds
- [Task](https://taskfile.dev/) for the documented development commands
- outbound image-registry access on first startup

Docker Desktop-specific APIs are not used. Colima, Lima-based Docker contexts,
and standard Linux Docker Engine are supported by the same Docker CLI adapter.

Container bases are pinned by immutable digest in
[`versions.env`](internal/infra/runtimeassets/assets/versions.env). The Gateway
currently uses mitmproxy 12.1.2 because the tested official 12.2.3 arm64 image
terminates with `SIGILL`; update the pin only after the arm64 runtime test
passes.

## Install from source

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build
install -m 0755 bin/tobari ~/.local/bin/tobari
```

Alternatively, from a checkout:

```sh
go install ./cmd/tobari
```

Ensure the destination is on `PATH`.

## Colima

Start a VM with enough capacity and confirm its context:

```sh
colima start --cpu 4 --memory 8 --disk 60
docker context show
docker version
docker compose version
tobari doctor --root ~/ghq
```

Colima normally shares the macOS user directory. If the selected root is not
below a shared directory, configure the mount in Colima before `up`.

## Linux Docker Engine

Confirm that the invoking user can access the intended Engine without passing a
Docker socket into Realm:

```sh
docker version
docker compose version
tobari doctor --root "$HOME/ghq"
```

Rootless Docker is compatible in principle, subject to its ordinary bind-mount
and UID/GID mapping behavior.

## Quick Start

```sh
tobari doctor --root ~/ghq
tobari up --root ~/ghq
tobari status
tobari shell
```

Run one exact argv at a host directory below the configured root:

```sh
tobari exec --cwd ~/ghq/github.com/example/repository -- claude
tobari exec --cwd ~/ghq/github.com/example/repository -- codex
tobari exec -- curl https://example.com/
```

Tobari preserves the invoked process exit status:

```sh
tobari exec -- sh -c 'exit 37'
echo $? # 37
```

Stop transient resources while retaining the Realm home and Gateway CA:

```sh
tobari down
```

Remove the three exact Tobari persistent volumes as well:

```sh
tobari down --purge
```

`--purge` must be used while Realm state still exists; a preceding ordinary
`down` deliberately forgets the lifecycle state and preserves volumes.

## Commands

| Command | Outcome |
|---|---|
| `tobari up --root PATH` | Test policy and create or reconcile one Realm |
| `tobari status [--format text\|json]` | Show root, proxy, policy, containers, and health |
| `tobari shell` | Open Bash in the running Realm |
| `tobari exec [--cwd PATH] -- COMMAND...` | Execute exact argv and preserve its exit status |
| `tobari logs [--component gateway\|opa\|realm\|all] [--tail N]` | Read a bounded, visibly escaped log window |
| `tobari down [--purge]` | Remove exact owned runtime resources |
| `tobari doctor [--root PATH] [--format tsv\|json]` | Diagnose Docker, paths, policy, credentials, and residue |
| `tobari help [COMMAND] [--format text\|agent]` | Read human or machine command contracts |
| `tobari version` | Print build identity |

`status`, `logs`, and `doctor` are observational and never repair state.

## Policy

On first `up`, Tobari initializes editable policy files under:

- macOS: `~/Library/Application Support/tobari/policy`
- Linux: `${XDG_CONFIG_HOME:-$HOME/.config}/tobari/policy`

The sample policy is generic HTTP policy, not a GitHub adapter. It allows only
listed hosts, rejects ordinary plain HTTP, restricts methods, supports explicit
host/method/path denials, and validates credential profile bindings.

```rego
package tobari.http

import rego.v1

default decision := {
    "allow": false,
    "reason": "request did not match an allow rule",
    "credential_profile": null,
    "status_code": 403,
    "audit": {"level": "metadata"},
}

decision := {
    "allow": true,
    "reason": "allowed by policy",
    "credential_profile": input.credential.requested_profile,
    "status_code": 403,
    "audit": {"level": "metadata"},
} if {
    input.request.host in data.tobari.allowed_hosts
    input.request.method in data.tobari.read_methods
    input.request.scheme == "https"
}
```

The actual initialized policy includes tested plain-HTTP mock rules,
method/path denial, and credential binding. Edit `data.json`, `tobari.rego`,
and `tobari_test.rego` together. `tobari up` refuses to reload when `opa test`
fails.

Gateway sends OPA a versioned generic input containing realm/session, scheme,
normalized host and port, method, path segments, multi-valued query, safe
headers, bounded body metadata, and an optional requested credential profile.
Authorization, proxy authorization, cookies, API keys, configured secret
headers, and raw bodies are excluded.

## Credential injection

Tobari supports static bearer and fixed-header injection. Create an owner-only
secret file on the host:

```sh
config_dir="${XDG_CONFIG_HOME:-$HOME/.config}/tobari"
install -d -m 0700 "$config_dir/credentials"
install -m 0600 /path/to/token "$config_dir/credentials/github-development"
```

On macOS, use
`$HOME/Library/Application Support/tobari` as `config_dir`.

Configure only metadata in `credentials.json`:

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

Add the same profile-to-host binding to policy data. A Realm client requests
the profile as non-secret metadata:

```sh
tobari exec -- curl \
  -H 'X-Tobari-Credential-Profile: github-development' \
  https://api.github.com/user
```

OPA must allow the request and return that profile. Gateway then independently
checks the exact host binding, reads the secret, removes Realm-supplied
authorization, and injects the managed header. The secret file is mounted only
in Gateway and is absent from Realm, CLI argv, OPA input, and audit logs.

## Security guarantees

Under the supported topology and trusted-component assumptions:

- Realm can write only its named home volume and the selected read-write root.
- Realm has no Docker socket, SSH agent, host networking, privileged mode, or
  added Linux capabilities.
- Direct Realm Internet egress has no route.
- Realm cannot reach OPA or Gateway credential files.
- HTTP/HTTPS proxy requests fail closed when OPA or Gateway fails.
- OPA sees the same buffered request bytes Gateway forwards.
- A managed credential is injected only after allow and an exact host-binding
  check.
- Audit logs contain request metadata and decisions, not secret values or raw
  bodies.
- Cleanup verifies exact ownership labels before removal.

Read [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md) for assumptions and abuse
cases.

## What Tobari does not guarantee

- Realm can modify or delete every file below the selected root.
- An allowed destination can receive data from that root.
- An allowed credential can exercise all authority granted by its provider.
- Tobari does not protect against Docker/host compromise, container or VM
  escape, covert channels, malware, or interference among processes in the
  shared Realm.
- Proxy environment variables do not transparently support applications that
  ignore proxies or pin certificates; those applications fail rather than
  bypass Gateway.
- HTTP/3/QUIC, raw TCP, UDP, Git SSH, and other non-HTTP protocols are
  unsupported.

## Troubleshooting

`doctor` reports Docker CLI/Engine/context/Compose, root validity, state, policy
tests, secret-file modes, and residual owned containers:

```sh
tobari doctor --root /absolute/root
```

Common failures:

- `policy_test_failed`: run `tobari status`, edit the reported policy directory,
  and correct the Rego test failure.
- HTTPS certificate error: confirm the program honors `SSL_CERT_FILE`,
  `REQUESTS_CA_BUNDLE`, or `GIT_SSL_CAINFO`; certificate-pinned programs are
  unsupported.
- `Could not resolve proxy`: inspect `tobari status` and
  `tobari logs --component gateway`; all three containers must be healthy.
- root bind-mount error under Colima/Lima: move the root under a shared host
  directory or configure that VM's mounts.
- an intended request returns `403`: inspect the Gateway denial record and
  refine the minimum host/method/path rule on the host.
- partial lifecycle failure: use `tobari status` before retrying `up` or `down`.

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

`task integration:test` requires an unused set of Tobari container names. It
creates real Realm, Gateway, OPA, and mock-upstream containers and proves:
HTTP allow/deny, HTTPS interception and CA trust, direct-egress denial,
control-plane isolation, OPA/Gateway fail closed, credential injection and
non-disclosure, exit-code preservation, concurrent exec, idempotent startup,
secret-free denial evidence, and exact cleanup.

CI runs the Go implementation gate, security/public gates, and complete
container runtime gate as separate jobs.

## MVP exclusions

The MVP deliberately excludes multiple or per-repository Realms, process-level
identity, transparent proxying, raw TCP/UDP/QUIC, Git SSH semantic inspection,
provider-specific adapters, AWS SigV4, OAuth refresh, GitHub App token refresh,
Keychain integration, human approval, policy engines other than OPA,
Kubernetes, filesystem overlays, private clone mode, GUI, remote execution,
and production multi-tenancy.

## License

Tobari is available under the [MIT License](LICENSE).
