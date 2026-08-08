# Tobari

Tobari gives a coding agent an execution boundary in advance, then lets it act
freely inside that boundary. It makes starting an isolated coding space,
understanding a denied operation, and granting the minimum required permission
extremely easy, so the safe execution path is a more natural choice than
running the agent directly on the host.

Isolation is opt-in, project-local, reusable, and reversible. When the agent
reaches a network boundary, Tobari starts from deny, shows the useful evidence,
and lets the user teach the minimum permission through a trusted
review-and-retry loop. Every supported outbound HTTP and HTTPS request is
enforced through one installation-local Gateway and OPA cluster.
For supported Context-owned credentials, one locked Auth Broker keeps the real
value outside Workspaces and gives each project only an opaque handle that
Gateway resolves after OPA allow.
The safe path should be easier than running the agent directly on the host;
the current explicit cluster bootstrap remains separate, while permission
growth is designed as one interactive review-to-allow-or-deny flow.

Tobari does not guess intent from command strings. It controls the network
effect at the point where an HTTP request crosses an isolation boundary.

## How it feels to use

Once the shared enforcement cluster is ready, the normal loop is progressive
policy learning:

1. Run `tobari` from a project directory.
2. Work freely until an undeclared request receives `403`.
3. The agent explains that the host must review the secret-free pending queue.
4. Run `tobari policy review`, select one permission, confirm the exact allow or
   deny,
   then retry.

```sh
cd quickstart-example
tobari

# Review is the human host-side entry point; in a TTY it offers selection,
# detail, explicit confirmation, and exact allow/deny without editing OPA or Rego.
tobari policy review
```

The Gateway response gives the agent a fixed host-side review command and never
requests an automatic retry. When an interactive session ends, the host also
prints an aggregate pending-permission summary. The review queue includes
bounded `host`, `method`, `path`, and `reason` evidence plus exact allow and deny
commands. It includes only denials OPA marks resolvable by an exact learned
rule; immutable scheme, cluster, and credential-binding failures remain
diagnostics instead of becoming ineffective decisions. Baseline host-authored
denies and previously rejected exact effects are terminal and remain audit
evidence rather than queue items. `policy allow` and `policy deny` resolve the
opaque ID against retained denials, test the complete policy, atomically record
one exact rule, and activate it. Tobari never turns observed traffic into
permission automatically.

To inspect or correct an earlier decision, use the current learned-decision
inventory. It includes both Allow and exact Deny rules; resetting one returns
that exact effect to default deny and does not retry it. The next review can
then make a new explicit decision:

```sh
tobari policy rules
tobari policy reset --id <exact-policy-rule-id>
tobari policy review
```

## Cluster and Tobari topology

```text
trusted host
  Tobari CLI ── Docker CLI ── Docker Engine
       │
       ├── fixed control exec/stdin ────────────────┐
       ├── root A (rw) ── CWD-owned Tobari A ── internal network A ──┐
       └── root B (rw) ── CWD-owned Tobari B ── internal network B ──┤
                                                                 ▼
                                                       trusted Gateway
                                                          │       │       │
                                             internal control     runtime socket
                                                          │       │       │
                                                         OPA   locked Auth Broker
                                                                      │
                                                               encrypted vaults
                                                          egress: HTTPS upstream
```

Each Tobari has its own internal network and persistent XDG home directory.
Gateway alone joins that project network. OPA joins only the shared control
network. Auth Broker joins control and egress but has no TCP listener and never
joins a project network; only Gateway mounts its runtime Unix socket. A program
that ignores the proxy has no external route; one Tobari cannot directly reach
OPA, Auth Broker, or another Tobari.

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
connection. Policy remains generic HTTP and needs no provider-specific URL
rewriting. By default, tool authentication prerequisites belong to that tool
and its per-Tobari home. The supported brokered route recognizes only a strict
provider-declared handle at an exact HTTPS authority/header binding, and the
retained Gateway managed adapter remains available for the earlier static
profile-injection design.

## Requirements

- macOS or Linux on a Docker-supported architecture
- Docker Engine 24 or newer
- Docker Compose v2
- Go version declared in [`go.mod`](go.mod) for source builds
- [Task](https://taskfile.dev/) for development commands
- access to the reviewed Gateway and Auth Broker images when they are not
  already local; the
  explicit runtime build may also obtain its declared base image

Docker Desktop-specific APIs are not used. Colima, Lima-based Docker contexts,
and standard Linux Docker Engine use the same Docker CLI adapter.

Container bases are pinned by immutable digest in
[`versions.env`](internal/infra/runtimeassets/assets/versions.env).

The Auth Broker image is currently at its first-publication bootstrap boundary:
`AUTH_BROKER_IMAGE=unpublished` is intentional. A normal binary fails
`cluster up` before Docker mutation until the main workflow publishes the
Linux amd64/arm64 manifest and a reviewed digest replaces that marker.
Contributors can exercise the complete local source path with `task build:dev`
and `bin/tobari-dev`; `task public:check` reports the bootstrap state, while
`task release:check` blocks a release that still contains it.

## Install from source

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build
tobari version # works immediately when this repository's bin/ is on PATH
```

Alternatively:

```sh
install -m 0755 bin/tobari ~/.local/bin/tobari
```

Or build through Go's install path:

```sh
go install ./cmd/tobari
```

Ensure the destination is on `PATH`; for the `install` example above, that
usually means `~/.local/bin` is on `PATH`.

## Quick start

This path has one deliberate host/agent boundary. The trusted host starts the
cluster, reviews and changes policy, edits the current Context recipe, and runs
the explicit runtime build. A process inside Tobari can work below the
selected project root and make proxy-aware HTTP/HTTPS requests, but it cannot
reach OPA, Docker, host credentials, or the Internet directly. A denial is a
host handoff; it is never an automatic approval or retry.

Host-owned actions in this walkthrough are `tobari cluster up`,
`tobari policy review`, `tobari policy rules`, `tobari policy reset --id ID`,
`tobari policy deny --id ID`, `tobari context show`, `tobari runtime init`,
`tobari runtime build`, `tobari list`, `tobari delete`, and
`tobari cluster down`. Optional Context authentication also uses host-owned
`tobari auth login|import|status|logout`; those commands never run inside a
Workspace.
Workspace-owned actions are the agent's commands and file changes below the
selected project root. Leave the Workspace before running a host-owned
recovery command; the Workspace does not contain a second Tobari control path.

Prerequisites:

- macOS or Linux with Docker Engine 24 or newer and Docker Compose v2;
- an interactive terminal for the root `tobari` command;
- `tobari` installed from source or available on `PATH`; and
- when using Colima or Lima, the project directory and Tobari XDG
  configuration/state directories must be visible through the Docker VM's
  shared paths; an unshared bind path stops setup before a Workspace can start;
  and
- access to the reviewed Gateway and Auth Broker images and official runtime
  base image if they are not already local. The explicit `runtime build` step
  may obtain its declared base image.

### 1. Start from a project directory

The directory below is synthetic and can be replaced with an existing project
directory. Do not use the filesystem root, your home directory, or a Tobari
configuration/state directory as a project root.

```sh
mkdir -p quickstart-example
cd quickstart-example
tobari cluster up
```

`cluster up` is explicit shared-cluster startup. It preflights the reviewed
Gateway and Auth Broker images, obtains the official runtime base image for the
default Context, validates the all-Context policy/provider projections, starts
Gateway, OPA, Auth Broker, policy, and CA state, then unlocks the broker through
the supported host root-key backend. Ordinary `tobari` entry does not repair or
start the cluster. If a published image is unavailable, inspect
the host with `tobari doctor` and retry `tobari cluster up`; `doctor` is a
diagnostic recovery command, not a prerequisite for the normal path.
While the checked-in Auth Broker selector is the explicit `unpublished`
bootstrap marker, use `task build:dev` and repeat this walkthrough with
`bin/tobari-dev`; official startup is intentionally unavailable until the first
published manifest digest is reviewed and pinned.

### 2. Observe a denied request inside Tobari

Enter the project from the host:

```sh
tobari
```

The command opens the supported interactive Bash shell as user `tobari`
(`BASH=/bin/bash`). The shell is inside the Workspace; `exit` returns to the
host and leaves the Workspace reusable. Run this request inside that shell.
`example.com` and the path are
synthetic public values; the `PUT` is intentionally outside the initialized
allow rules while remaining eligible for exact policy learning.

```sh
curl -sS -w '\nhttp=%{http_code}\n' \
  -X PUT https://example.com/quickstart
```

The response is a secret-free `policy_denied` with `http=403` and fixed host
navigation to `tobari policy review`. It does not contain a candidate ID and
does not request an automatic retry. Leave the session before running the host
recovery commands; policy review and policy changes belong to the trusted host:

```sh
exit
```

### 3. Review and allow one exact permission on the host

On the host, use the TTY Permission Inbox and inspect the exact
`example.com` / `PUT` / `/quickstart` request before confirming **allow**:

```sh
tobari policy review --tail 100
```

For a redirected or scripted host flow, review is read-only. Copy the exact
opaque `pcy_...` value for that same request unchanged, then run the explicit
action; this is the machine path, not an additional step after the TTY flow:

```sh
tobari policy review --tail 100 --format json
tobari policy allow --id <exact-pcy-id-from-policy-review>
```

Replace the angle-bracket placeholder with the value emitted by review; do not
derive an ID from display order, host text, or a previous denial. `policy allow`
tests the complete policy, records one exact project-bound rule, and activates
it without restarting the Tobari. `policy deny --id <exact-pcy-id>` is the
corresponding recovery when the requested permission should remain blocked.
The successful allow handoff names `tobari` as the next command: re-enter from
the host and repeat the same request inside the Workspace.

### 4. Retry the same request

Re-enter the same project directory and run the same curl again:

```sh
tobari
curl -sS -w '\nhttp=%{http_code}\n' \
  -X PUT https://example.com/quickstart
exit
```

The response is now an upstream response rather than Tobari's
`policy_denied` handoff. The final HTTP status belongs to `example.com`; the
Tobari contract is that the exact learned request is allowed, while a child
path, another project, or another method is not silently broadened.

### 5. Inspect the Workspace lifecycle

Back on the host, list the local Workspaces:

```sh
tobari list
```

`list` reports Context, roots, runtime diagnostics, and stable IDs for diagnosis
only; lifecycle commands still resolve the target from the current directory
plus the explicit or current Context.
`tobari delete` is the command that ends the nearest detached Workspace:

```sh
# Stop after Stage 1:
tobari delete
```

Use that delete when you are stopping after the policy loop. The runtime
customization stage below is more useful if you keep the policy-demo Workspace
so the next `tobari` entry can show container-only image reconciliation. If
you did delete it, the next `tobari` after the build simply creates a fresh
Workspace with the same current Context image.

### 6. Customize the current Context runtime

Runtime customization is host-owned and explicit. Initialize the current
Context recipe, inspect the reported Dockerfile path, and edit that file:

The selected Context image is checked before a new Workspace is registered and
the permanently bound Context image is checked before an existing Workspace is
reconciled. A missing or incompatible
image returns `image_not_found`, points to `tobari runtime build`, and leaves
the previous Workspace state, home, project network, and work container
unchanged.

```sh
tobari runtime init --format json
tobari context show --name=default --format=json
```

Omit `--name=default` to inspect the current Context; use the equals form when
you name a Context explicitly.

Add one harmless tool between the template's existing `USER root` and
`USER tobari` lines. For example, install `tree` and keep the package lists
out of the image:

```dockerfile
RUN apt-get update \
    && apt-get install -y --no-install-recommends tree \
    && rm -rf /var/lib/apt/lists/*
```

Then build and promote the current Context runtime explicitly, and enter the
project again:

```sh
tobari runtime build --format json
tobari
```

If the policy-demo Workspace still exists, this entry validates the new image,
recreates only the work container when the runtime spec changed, and preserves
the Workspace home. If you deleted the Workspace in the lifecycle step, this
entry creates it again with the newly selected current Context image.

Inside the new session, verify the added tool and then leave the reusable
Workspace:

```sh
tree --version
exit
```

`runtime init` does not overwrite an existing recipe or change the selected
image. Editing the Dockerfile does not build anything. `runtime build` is the
one deliberate host Docker build boundary: with the template's official
`ghcr.io/tasuku43/tobari/runtime:latest` base, that explicit build may refresh
the base; an explicit local or custom base does not request a registry pull.
The build context is only the current Context runtime directory. A successful
compatible image is promoted into the Context, and a failed build leaves the
previously selected image active. Existing Workspaces observe the promoted
image on the next `tobari` entry without losing their home.

### Failure and recovery

- `tty_required`: run the root `tobari` command from an interactive terminal.
- `cluster_start_failed`, `gateway_image_unavailable`,
  `auth_broker_image_unavailable`, `runtime_image_unavailable`,
  `gateway_image_incompatible`, `auth_broker_image_incompatible`, or
  `incompatible_image`: run `tobari doctor`, inspect `tobari cluster status`,
  and retry `tobari cluster up`.
- `auth_broker_unavailable`, `auth_broker_locked`, or
  `auth_broker_request_failed`: leave the Workspace, run `tobari cluster up`,
  and continue only after `tobari auth status` reports `ready`.
- `auth_mutation_outcome_unknown`, `unclassified_mutation_outcome`, or
  `mutation_output_write_failed`: do not replay login, import, or logout; run
  `tobari auth status` and reconcile the selected Context first.
- `root_key_unavailable`, `root_key_unsafe`, `keychain_denied`, or
  `root_key_missing_with_vault`: use `tobari doctor`; restore the original key
  when encrypted state exists because Tobari will not silently replace it.
- `credential_handle_invalid` (HTTP 403): leave and re-enter the Workspace to
  receive the current project-bound handle. `credential_broker_unavailable`
  (HTTP 503) requires host-side `cluster up` before another request.
- A learnable `403`: leave the session and run `tobari policy review`. On a TTY,
  inspect the request and explicitly confirm allow or deny; the review flow
  applies the exact decision and tells you to re-enter. Redirected or machine
  review remains read-only and uses one unchanged candidate ID with
  `tobari policy allow --id ID` or `tobari policy deny --id ID`. Do not retry
  before a confirmed host action.
- To correct a previous decision, run `tobari policy rules`, copy its opaque
  `policy-rule` ID unchanged into `tobari policy reset --id ID`, then use
  `tobari policy review` to choose the next decision. Reset leaves the exact
  effect denied by default and never retries it.
- `runtime_recipe_missing`: run `tobari runtime init`, edit the current Context
  Dockerfile, then run `tobari runtime build`.
- `runtime_recipe_exists`: inspect the existing recipe with
  `tobari context show`; `runtime init` never overwrites it.
- `runtime_build_failed` or `incompatible_image`: run `tobari context show`,
  correct the Dockerfile, and retry `tobari runtime build`; the previous image
  remains selected until promotion succeeds.
- `project_session_attached`: exit the attached session, then run
  `tobari delete`; use `tobari delete --force` only when terminating that
  session is intentional.

When finished with this synthetic Workspace, clean up from the host. Delete the
project Workspace first; only then remove the shared cluster:

```sh
tobari list
tobari delete
tobari cluster down --purge
```

Cluster cleanup preserves encrypted Context vaults and the installation root
key. `--purge` additionally removes only shared CA volumes; use `auth logout`
for credential deletion and handle revocation.

The base runtime already carries common Git, HTTP, JSON, Python, SSH, and
command-line tools. Install additional tools through the current Context recipe
when they should be part of a reusable Context; tool-native authentication
state remains below each Tobari's persistent home.

### Lifecycle and image reference

Start the shared enforcement cluster explicitly from the project directory:

```sh
tobari cluster up
```

`cluster up` obtains the reviewed immutable Gateway and Auth Broker images and
official runtime base image when they are not already local, then checks their
digests, API/role labels, non-root entrypoints, and Docker Engine architecture
before starting shared resources. Tobari contributors who need to test local
Gateway, Auth Broker, or runtime-base source changes use `task build:dev` and the resulting
`bin/tobari-dev` binary; the public `cluster up` path does not build images
from source.

If startup reports Docker, image, policy, or bind-path problems, run
`tobari doctor` from the project directory and retry `tobari cluster up`.
Use `tobari doctor --root PATH` only when diagnosing a different directory.
Doctor emits the full read-only report, returns `diagnostic_failed` when any
check fails, and treats warnings alone as healthy. It may inspect provider,
root-key/vault, broker, and project-binding state, but never repairs it, starts
or unlocks the cluster, or creates/replaces a root key.

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
Help, JSON, agent help, and logs keep their respective machine or raw data
contracts and never receive terminal styling.

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
root at `/work` enters `/workspace/work/root/app`; when the selected root is
below the host home, Tobari preserves the home-relative path instead, so
`$HOME/path/to` enters at `/var/lib/tobari/path/to`. Only the selected
project root is mounted; the host home is never mounted wholesale. A shell exit
returns to the host while the Workspace remains existing. Tobari prints the
following host guidance on stderr after the child session returns:

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
in the current Context containing the current directory. Use `--context NAME`
for another same-root Context. If another terminal is attached, the command
warns and fails; use `tobari delete --force` only when terminating that session
is intentional. Each canonical root and stable Context pair has at most one
Workspace, including when explicit creation requests race. The same root may
have independent Workspaces in different Contexts:

```sh
tobari                         # uses the current Context
tobari --context restricted    # does not change the current Context
tobari list                    # shows both Context/root pairs
```

Same-root and parent/child-root Tobari have separate stable identities, homes,
containers, internal networks, policy, runtime authority, and managed
credentials. Their overlapping host project files are deliberately the same
files: edits are mutually visible across Contexts. Tobari adds no overlay,
checkout copy, root lock, session exclusion, or filesystem integrity isolation.

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

The base runtime bundles the common work tools shared by supported agents:
Git, GitHub CLI, AWS CLI, curl, jq, Python, and SSH. Official agent variants
such as Claude and Codex add only their agent-specific tool and dependencies.
These images are convenience starting points; they do not change Tobari's
isolation or lifecycle boundary. The base image is published on main pushes as
`ghcr.io/tasuku43/tobari/runtime:latest` and `:main`, plus an immutable commit tag. Tobari
still validates any selected image locally and never pulls it implicitly.

Install other agent CLIs inside a Tobari or place binaries below its selected
root. The per-Tobari home survives shell exit and runtime recovery.

### Specialized CLI toolbox

The base runtime already contains the common GitHub and AWS tools. Repeated
cluster or Atlassian exercises can use the optional local toolbox image for
specialized tools such as kubectl, TWG, rsync, and DNS utilities. Build and
validate it on the trusted host:

```sh
task toolbox:build
cd quickstart-example
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
Authenticate deliberately within the isolated environment; the resulting
tool-owned state stays in that Tobari's persistent home. AWS SigV4 and OAuth
refresh remain tool/provider behavior outside Tobari's contract. Git over HTTPS
uses the Gateway; Git over SSH and other non-HTTP transports have no direct
egress route.

### Explicit image compatibility path

The default Context starts from the published Tobari runtime base image. This
is a bootstrap runtime: it is useful for first entry, but ongoing work normally
needs a Context-specific runtime image with project tools installed. Create
that image without creating a new Context:

```sh
tobari runtime init
# edit the current Context runtime/Dockerfile
tobari runtime build
```

The generated Dockerfile starts from the official base:

```dockerfile
FROM ghcr.io/tasuku43/tobari/runtime:latest

USER root
RUN apt-get update \
    && apt-get install -y --no-install-recommends nodejs npm \
    && rm -rf /var/lib/apt/lists/*
USER tobari
```

`runtime build` builds a machine-managed local image for the current Context,
validates the Tobari runtime contract, and selects it only after a successful
build. Editing the Dockerfile alone does not change the selected image.

If you already maintain a compatible image outside `runtime build`, build it
explicitly on the trusted host, then select it in a Context:

```sh
docker build --tag my-tobari:dev .
tobari context create --name project-tools --image my-tobari:dev
tobari context use --name project-tools
tobari context show
```

Before the default Context is initialized, the owner-only XDG setting can seed
its image:

```json
{
  "version": "v1",
  "default_image": "my-tobari:dev"
}
```

The first Context initialization copies this selector into its manifest.
`context use` changes only the current/default Context and never starts or
reconciles Docker. Creating a Context while cluster state exists reports that
an explicit `cluster up` is required to validate and activate the new complete
all-Context projection.

Then ordinary root invocations stay short:

```sh
tobari
```

New Workspaces use the explicit or current Context's runtime image and then
persist that stable Context binding. Existing Workspaces always reconcile from
their bound Context, regardless of later `context use`. Before the default
Context is initialized, `config.json.default_image` seeds it; otherwise
`builtin` resolves to the published official runtime base in normal builds.
Project metadata does not override the Context image, and the per-Workspace
home is preserved when only the runtime image changes.

Tobari never pulls a configured image implicitly. The image must be available
locally and preserve runtime API `1`, the `tobari` image user, the inherited
entrypoint, and `io.tobari.runtime-lifetime-command=sleep infinity` required by
Tobari's fixed lifetime command.
Prefer a digest selector when reproducibility matters. The compatibility check
is not a signature or trust decision: image contents remain untrusted and run
under the same fixed non-root user, read-only root filesystem, dropped
capabilities, mounts, proxy, and internal network as the built-in image. If the
image is missing or incompatible, `tobari` refuses to bring up a new Workspace
or replace an existing work container.

The selected image's `CMD` does not own Workspace lifetime. Tobari starts the
work container with its own long-lived `sleep infinity` command, then runs an
interactive `/bin/bash` shell or an agent as a child session. Run an exact agent command
from inside the current `tobari` session; a child exit returns its status while
the Workspace remains reusable.

### Contributor local image path

When developing Tobari itself, use the development build task:

```sh
task build:dev
bin/tobari-dev cluster up
TOBARI_INTEGRATION_BINARY=$PWD/bin/tobari-dev \
  TOBARI_INTEGRATION_CUSTOM_BASE=tobari-runtime:dev \
  ./scripts/test-integration.sh
```

That task builds `tobari-gateway:dev`, `tobari-runtime:dev`, and a
`tobari_dev`-tagged CLI binary. Only that binary resolves Tobari-managed images
to those local tags. The normal `task build` binary keeps using the published
Gateway digest and official runtime base.

### Runtime customization

Runtime selection is intentionally owned by Context. Project-local
`.devcontainer` files are not interpreted, so they cannot silently become a
second execution-boundary configuration. The preferred path for a Context-
specific runtime is:

```sh
tobari runtime init --format json
tobari context show --format json
# edit the current Context runtime/Dockerfile path reported above
tobari runtime build --format json
tobari
```

`runtime init` creates the template and never overwrites an existing
Dockerfile. Add tools between its existing `USER root` and `USER tobari` lines;
the template includes a harmless package-install example. `runtime build` uses
only that Context runtime directory, refreshes the official
`ghcr.io/tasuku43/tobari/runtime:latest` base because the build was explicitly
requested, validates the Tobari runtime contract, and selects a machine-managed
local image. A local or custom base also works without an extra refresh pull.
No image name, Context name, or manifest edit is needed. If editing or building
fails, the previously selected image remains selected. Inspect the exact current
path and status with:

```sh
tobari context show
```

The explicit image path above remains available for importing an already-built
compatible image or for advanced workflows, but ordinary customization should
stay attached to the current Context through `runtime init` and `runtime build`.

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

Both forms preserve Auth Broker Context vaults and the installation root key.

## Commands

| Command | Outcome |
|---|---|
| `tobari cluster up` | Build and validate all-Context policy/provider projections, reconcile exactly one shared Gateway, OPA, and Auth Broker, then unlock the broker |
| `tobari cluster status [--format text\|json]` | Show three-service readiness, Context count, aggregate revision, policy/provider projection integrity, root-key backend, project count, and diagnostics |
| `tobari cluster denials [--tail N] [--format text\|json]` | Read typed denial evidence, policy path, and review command |
| `tobari cluster logs [--component auth-broker\|gateway\|opa\|all] [--tail N]` | Read bounded shared logs and denial evidence without credential contents |
| `tobari cluster down [--purge]` | Remove an empty cluster while preserving Auth Broker vaults/root key; purge additionally removes shared CA state |
| `tobari policy review [--tail N] [--format text\|json]` | Installation-wide Permission Inbox: inspect Context/root/request and explicitly allow or deny one exact permission on a TTY; read-only when redirected |
| `tobari policy candidates [--tail N] [--format text\|json]` | Discover Context/project-scoped pending exact decisions and opaque IDs |
| `tobari policy tail [--tail N]` | Compatibility view of the bounded queue with exact allow and deny commands |
| `tobari policy allow --id ID` | Test, store, and activate one exact observed permission |
| `tobari policy deny --id ID` | Test, store, and activate one exact Context/project-bound rejection |
| `tobari policy rules [--format text\|json]` | List current decisions across all Contexts; on a TTY, reset one explicitly |
| `tobari policy reset --id ID` | Remove one current learned decision and return its effect to default deny |
| `tobari policy compactions [--format text\|json]` | Discover test-backed prefix compactions and opaque IDs |
| `tobari policy compact --id ID` | Test and activate one current bounded compaction |
| `tobari context list [--format text\|json]` | List stable named Contexts and identify the current default |
| `tobari context show [--name NAME] [--format text\|json]` | Inspect runtime, agent, policy, managed-adapter store references, and secret-free broker/provider state without a broker vault path/content, key, primary secret, or handle |
| `tobari context create --name NAME [--image IMAGE] [--mode guided\|advanced]` | Create a named execution Context without secrets |
| `tobari context use --name NAME` | Change only the current/default Context without mutating existing Tobari or Docker |
| `tobari runtime init [--format text\|json]` | Create the current Context's runtime/Dockerfile template |
| `tobari runtime build [--format text\|json]` | Build, validate, and select the current Context runtime image |
| `tobari auth login PROVIDER [--context NAME] [--format text\|json]` | Acquire one supported provider credential through a trusted-host helper; currently `github` |
| `tobari auth import PROVIDER [--context NAME] [--format text\|json]` | Import one bounded opaque provider credential from protected non-terminal stdin only |
| `tobari auth status [--context NAME] [--format text\|json]` | Inspect exhaustive secret-free provider and broker state for one Context |
| `tobari auth logout PROVIDER [--context NAME] [--format text\|json]` | Remove one local Context/provider credential and revoke every issued handle without remote logout |
| `tobari [--context NAME]` | Choose or create the current-directory Workspace in the explicit or current Context, enter it, and leave it reusable after `exit` |
| `tobari status [--context NAME] [--format text\|json]` | Report Context-bound logical existence and runtime diagnostics for the current directory |
| `tobari list [--format text\|json]` | List local Workspaces with Context, runtime diagnostics, and diagnostic IDs |
| `tobari delete [--context NAME] [--force]` | Delete the nearest detached current-directory Tobari in the selected Context; `--force` overrides an attached-session guard |
| `tobari doctor [--root PATH] [--format text\|tsv\|json]` | Diagnose Docker, paths, policy, root-key/broker/provider state, managed-secret permissions, and residue |
| `tobari help [SELECTOR] [--format text\|agent]` | Read human or machine command contracts |
| `tobari version` | Print build identity |

`cluster status`, `cluster denials`, `policy candidates`, `policy tail`,
`policy compactions`, `status`, `list`, and `doctor` are observational and
never reconcile Docker or create/delete runtime resources. They may clear an
exact durable journal before selecting logical state. Runtime recovery belongs
only to the root `tobari` operation.
`auth status` is also observational; unlike cluster status it requires the
configured broker service so it can distinguish ready, locked, and unavailable
provider state without reading a secret.

## XDG configuration and live policy

On macOS and Linux, Tobari uses the same XDG paths:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/tobari/
  config.json
  auth/
    providers/*.json      # optional owner-only stdin-import provider manifests
  contexts/
    active.json           # compatibility-named current/default Context marker
    <name>/
      context.json        # stable Context ID, agent profile, runtime, policy mode
      runtime/
        Dockerfile         # optional Context runtime recipe
      policy/
        data.json
        tobari.rego
        tobari_test.rego
      credentials.json     # reserved managed-adapter metadata
      credentials/         # reserved managed-adapter secret files

${XDG_STATE_HOME:-$HOME/.local/state}/tobari/
  roots/<root-and-context-hash>.json
  instances/<tobari-id>/state.json
  instances/<tobari-id>/home
  cluster-projections/<aggregate-revision>/
  auth/
    keys/root.key         # Linux only; macOS uses Keychain
    contexts/<context-id>/vault.enc
    projection/providers.json
    projects/<tobari-id>.json
    runtime/broker.sock   # Gateway-only mounted runtime socket

${XDG_DATA_HOME:-$HOME/.local/share}/tobari/
  profiles/default/       # shared read-only agent profile
```

On first use of legacy single-active-Context state, Tobari preserves each
project ID, canonical root, and home only when the legacy current marker,
cluster paths, Context source, instance state, and root index consistently name
one valid Context. It then permanently binds that Tobari to the Context's stable
ID and rewrites the pair-derived index. Conflicting markers, interrupted
reconcile journals, missing/broken Contexts, unsafe credentials, or incomplete
project state fail closed with a recovery diagnosis. Historical Context-less
denials and learned data are not guessed into an actionable permission; repeat
the denied request to obtain correctly scoped evidence.

OPA sees one read-only, content-addressed cluster projection generated from all
Context policy sources. The fixed `tobari.http/decision` router selects a
Context only from Gateway's trusted principal. Guided Contexts share one
Tobari-owned evaluator with Context-specific data. Advanced `package
tobari.http` source is projected into a reserved Context-ID namespace and
cannot claim the router, system packages, or another Context's entrypoint.

`context use` changes only the current/default marker. `cluster up` and exact
policy mutations serialize aggregate generation, test every source and the
complete candidate, atomically publish only valid state, and retain the prior
known-good revision on activation failure. Exact allow, deny, reset, and
compaction actions derive Context and Tobari authority solely from their opaque
reference and recreate only the exact shared OPA component. Advanced
host-authored edits remain explicit and are not part of the routine queue.

Use the policy directory as a trusted-host path when editing policy; do not
mount its parent configuration directory into a Tobari:

```sh
${EDITOR:-vi} "${XDG_CONFIG_HOME:-$HOME/.config}/tobari/contexts/<name>/policy/tobari.rego"
```

Use `tobari context show` to discover the exact current Context and its policy
path. Keep the Context's policy directory separate from its credential stores;
managed and broker vault stores remain outside untrusted containers. Only
manifest-declared opaque handles may enter a Workspace. For routine exact permission
growth, use `tobari policy review` and `tobari policy allow` or `policy deny`; use
`tobari policy rules` and `policy reset` to correct a current learned decision;
direct Rego editing is the advanced path. Tool-native login state belongs in the selected instance
home instead.

The initialized policy is generic HTTP policy, not a GitHub operation adapter. It starts
deny-by-default, distinguishes HTTPS from explicitly allowed test-only HTTP,
restricts methods and paths, and validates credential profile or non-secret
broker-provider metadata without selecting a real credential. Current Context
policy source targets input schema 4; aggregate generation also accepts legacy
source schema 3 and rewrites both to Gateway runtime schema 5 before activation.

### Grow and compact learned policy

Use the human review queue during normal work:

```sh
tobari policy review
tobari policy allow --id PCY_ID  # redirected/scripted path only
tobari policy rules
tobari policy reset --id PLR_OR_PDR_ID  # return one decision to default deny
```

On a TTY, `policy review` is the installation-wide human flow: select a request,
inspect its Context, Tobari/root, and exact host/port/method/path, keep those
dimensions visible during allow/deny confirmation, and let Tobari delegate the
selected ID to `policy allow` or `policy deny`. It refreshes the queue after
each successful decision. Redirected or `--error-format json` review is read-only. `PCY_ID` is
emitted by the review or machine discovery queue and must be copied unchanged
when invoking the explicit action; `policy candidates` remains the structured
machine path.
It expires when its denial falls outside retained logs or another learned rule
already covers that exact effect. Repeating the same denied
Context/project/host/port/method/path retains one item and the same ID, increments its
retained-window observation count, and refreshes its latest evidence. Approval never accepts a host
wildcard, method wildcard, prefix, or user-supplied pattern.

`policy rules` is the exhaustive installation-wide current-decision view,
including learned Allow and exact Deny rules that no longer appear in the
pending queue. Human and JSON views retain Context and Tobari/root.
`policy reset --id` consumes its Context-scoped `policy-rule` ID unchanged,
removes exactly that CLI-owned decision through the same tested activation
boundary, and leaves the effect at default deny. Use `policy review` afterward
if the retained denial should receive a different explicit decision.

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
the XDG Rego and data files on the trusted host and add tests. The routine
review queue does not interpret or activate arbitrary host edits.

Run read-only diagnostics when you want to test policy without activating it:

```sh
tobari doctor
```

An invalid watched edit does not authorize traffic. Inspect OPA logs and correct
the policy:

```sh
tobari cluster logs --component opa --tail 100
```

## Authentication in a Tobari

Tobari has three explicit authentication paths. Tool-native passthrough remains
the universal default: enter the Workspace and run the tool's normal login
flow. The tool persists its own state below `HOME=/var/lib/tobari`; Tobari never
copies the host home, Keychain, SSH agent, or host CLI configuration into it:

```sh
cd quickstart-example
tobari
gh auth login       # example: GitHub CLI's native device/browser flow
aws sso login       # example: AWS CLI's native flow
claude login        # example: an agent CLI's native flow
```

These are examples; the boundary applies to any tool that stores its own
authentication state in the Tobari home. The state survives runtime recovery,
is isolated from other Tobaris, and is removed by exact `tobari delete`.

The Auth Broker path instead acquires one credential on the trusted host for an
explicit or current Context. The built-in provider is GitHub.com:

```sh
tobari cluster up
tobari auth status --context default
tobari auth login github --context default
tobari auth status --context default
tobari                 # re-enter this Context's Workspace
gh api user            # succeeds only when OPA allows the exact HTTP request
exit
tobari auth logout github --context default
```

`auth login github` runs the pinned GitHub CLI's ordinary web login inside the
broker with ephemeral configuration. The token is captured internally; only a
bounded account label and opaque credential revision may appear in output.
The real credential is encrypted at
`auth/contexts/<context-id>/vault.enc`. macOS stores the installation root key
in Keychain service `io.tobari.auth-root.v1`; Linux uses owner-only XDG state
`auth/keys/root.key`. The broker starts locked after restart and `cluster up`
unlocks it through stdin. A missing root key beside an existing vault fails
closed.
Public backend values are `macos_keychain` and `xdg_file`; the
`linux_xdg_file` string is doctor-only infrastructure diagnostic prose. See the
[canonical schema/path/backend table](docs/07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

Credential ownership is Context-wide. Every project permanently bound to that
Context is eligible, but each receives a distinct `tobari-h1_...` handle only
on its next matching Workspace entry. The handle is bound to its Context,
project, provider, credential revision, and exact HTTPS/header contract. For
GitHub this is projected as `GH_TOKEN`, with
`GH_HOST=github.com`; it is not the real token. Gateway removes the handle,
performs non-secret introspection, asks OPA about the body-free HTTP effect,
and resolves the primary secret exactly once only after allow. Denial performs
no resolution. A copied, stale, malformed, revoked, or mismatched handle fails
with `credential_handle_invalid` and is never forwarded.

Login, import, replacement, and logout cannot rewrite a running process, so
successful output says `workspace_reentry_required`. Leave and re-enter the
Workspace to receive the current projection. Logout removes local broker state
and revokes all handles immediately; next entry removes the environment
projection and only unchanged Tobari-owned complete files. It does not revoke
the token at the provider.

Owner-controlled schema-v1 provider manifests may be installed as owner-only
JSON below `auth/providers/`. They contain no secret or executable helper and
may acquire one opaque credential only through protected non-terminal stdin:

```sh
secret-source-command | tobari auth import example --context default
```

Replace `secret-source-command` with a trusted password-manager or equivalent
no-echo source. The value is read from non-terminal stdin only, is bounded to
32 KiB, and never appears in Tobari argv, environment, or output. If stdin is
an interactive terminal, Tobari refuses the import before reading a byte;
redirect or pipe the trusted source instead. Public Context/provider argument,
intent, and mutation validation precedes the read; infrastructure validates the
selected existing Context, installed provider/acquisition mode, and broker
readiness before broker send. Manifests declare bounded handle
environment/complete-file templates and exact HTTPS header transformations;
they cannot add policy, arbitrary routes, methods/paths, refresh, signing,
provider operations, or a built-in override.

Handles are random capabilities rather than token hashes: they do not expose
whether two Contexts use the same upstream token, do not depend on provider
token format, and can be revoked independently. Arbitrary manifest commands
would execute untrusted code inside the broker, while request-wide replacement
would make unrelated URL/body/cookie/header bytes ambiguous, so v1 rejects
both.

The retained `managed` adapter is the third path. Trusted runtime configuration
may select the earlier static `credentials.json` plus owner-only
`credentials/` profile design. Gateway checks the trusted Context, project,
and exact host before OPA and immediately before post-allow injection. The
default passthrough path never loads those files. There is no implicit fallback
from a Tobari-looking broker handle to managed or passthrough behavior.
Fallback is selected only when no Tobari marker exists in any inspected
URL/header position; malformed, misplaced, ambiguous, or binding-mismatched
markers fail as `credential_handle_invalid` without forwarding.

See [Authentication handling](docs/07_authentication.md) for the provider
manifest, vault, socket, failure, and compatibility contracts.

## Security guarantees

Under the documented topology and trusted-component assumptions:

- Each Tobari can write only its selected root and exact XDG home directory;
  overlapping roots intentionally share host-file effects and are not integrity-isolated.
- Tool-native authentication state persists in that exact home and is not shared
  with another Tobari.
- A brokered primary secret and installation root key never enter a Workspace
  or OPA. Each Workspace receives only its exact Context/project-bound opaque
  handle; copied, stale, rotated, revoked, or mismatched handles fail closed.
- Gateway never interprets a handle in a URL, cookie, unsupported header, or
  request body as authentication. URL and header occurrences are rejected
  before OPA and audit. Request bodies remain opaque Workspace-controlled data:
  Tobari does not scan every body byte and therefore does not claim to prevent
  a malicious Workspace that already knows its handle from sending it as
  ordinary payload on an otherwise allowed effect.
- Tobari have no Docker socket, SSH agent, host networking, privileged mode, or
  added Linux capabilities.
- Direct Internet egress has no route.
- Tobari cannot reach OPA, Auth Broker sockets/vaults, reserved Gateway
  credential files, or another Tobari.
- HTTP/HTTPS requests fail closed when OPA or Gateway fails.
- The default adapter forwards client authentication only after allow; managed
  credentials are injected only after allow and exact Context/host/project
  binding. Brokered credentials are introspected without a secret before OPA
  and resolved exactly once only after allow.
- Audit logs contain request metadata and decisions, not secret values or raw
  bodies. They contain no query or headers; a path containing a Tobari handle
  marker is wholly replaced by `/[redacted-auth-handle]`, and structural
  URL/header handle rejections are non-learnable.
- Cleanup verifies exact owner and opaque Tobari-ID labels.
- OPA cannot rewrite the host XDG policy.

Read [`docs/THREAT_MODEL.md`](docs/THREAT_MODEL.md) for assumptions and abuse
cases.

## What Tobari does not guarantee

- A Tobari can modify or delete every file below its selected root.
- An allowed destination can receive data from that root.
- An allowed credential can exercise all provider authority it grants.
- Any process in a Workspace can read its tool-owned credentials and broker
  handles and can exercise capabilities already permitted for that exact
  project; Tobari does not provide process-level identity inside a Workspace.
- Tobari does not protect against Docker/host compromise, container or VM
  escape, covert channels, malware, or interference among processes inside the
  same Tobari.
- Proxy variables do not support applications that ignore proxies or pin
  certificates; those applications fail rather than bypass Gateway.
- HTTP/3/QUIC, raw TCP, UDP, Git SSH, and other non-HTTP protocols are
  unsupported.
- Brokered authentication does not provide token refresh, remote provider
  logout, multiple accounts per Context, Git credential helpers, GitHub App
  tokens, arbitrary OAuth, AWS SigV4, request signing, or provider-operation
  policy semantics.

## Troubleshooting

```sh
tobari doctor
tobari cluster status
tobari list
```

Common failures:

- `policy_test_failed`: inspect the policy reported by `cluster denials` or
  `cluster status`; for exact decisions, rediscover the queue and retry the
  corresponding action. If tests pass outside Tobari, verify that the XDG
  policy directory is shared with the Docker VM.
- `gateway_image_unavailable`: inspect Docker registry access with `tobari
  doctor`, then retry `tobari cluster up`.
- `auth_broker_image_unavailable`: if `AUTH_BROKER_IMAGE` is `unpublished`, use
  the contributor `task build:dev`/`bin/tobari-dev` path or complete the
  owner-side image publication and digest-pin handoff. Otherwise inspect
  registry access and retry `cluster up`.
- `runtime_image_unavailable`: inspect Docker registry access and the selected
  Context image with `tobari doctor`, then retry `tobari cluster up`.
- `gateway_image_incompatible`: inspect the Gateway image digest, labels,
  entrypoint, and architecture with `tobari doctor` before retrying.
- `auth_broker_image_incompatible`: inspect the Auth Broker digest, API/role
  labels, non-root user, entrypoint, and architecture before retrying.
- `auth_broker_locked`, `auth_broker_unavailable`, or
  `auth_broker_request_failed`: leave the Workspace, run `tobari cluster up`,
  and confirm `tobari auth status` before retrying.
- `auth_mutation_outcome_unknown`, `unclassified_mutation_outcome`, or
  `mutation_output_write_failed`: the broker may have committed or confirmed
  completion could not be delivered. Do not repeat login, import, or logout;
  run `tobari auth status` and reconcile the selected Context first.
- `auth_login_tty_required`: run trusted-host GitHub login with interactive
  stdin and stderr. `invalid_credential_input` on terminal stdin means import
  refused before reading; pipe or redirect a trusted no-echo source.
- `root_key_missing_with_vault`: restore the original macOS Keychain item or
  Linux XDG key, or explicitly remove unrecoverable local auth state. Tobari
  never creates a replacement while an encrypted vault exists.
- `invalid_provider_manifest`: inspect owner/mode/symlink safety and the strict
  schema-v1 files below XDG `auth/providers`, then rerun `cluster up`.
- `ambiguous_provider_http_binding`: remove overlapping exact
  scheme/host/port/source-header/source-format recognition, run `tobari doctor`,
  then rerun `cluster up`; no partial provider projection is activated.
- `credential_handle_invalid` (HTTP 403): leave and re-enter the selected
  Context's Workspace. `credential_broker_unavailable` (HTTP 503) requires
  host-side broker reconciliation before another request.
- HTTPS certificate error: confirm the program honors `SSL_CERT_FILE`,
  `REQUESTS_CA_BUNDLE`, or `GIT_SSL_CAINFO`.
- `tty_required`: run the root `tobari` command from an interactive terminal.
- `workspace_selection_stale`: the Workspace list changed during selection;
  run `tobari` again and choose from the refreshed list.
- cancelled or unavailable Workspace selection: choose an available candidate,
  press `n` to create explicitly at the current directory, or run `q` to leave.
- `already_inside`: exit the current Tobari before entering another session.
- `image_not_found`: run `tobari runtime build` on the host; a new Workspace
  is not registered, and an existing Workspace runtime is not replaced, until
  its selected compatible image is available locally.
- `incompatible_image`: extend the official Tobari runtime base, or another
  compatible image, without replacing its user, lifetime-command capability, or
  entrypoint.
- `runtime_recipe_missing`: run `tobari runtime init`, edit the current Context
  Dockerfile, and run `tobari runtime build`.
- `runtime_build_failed`: inspect `tobari context show`; the previous selected
  image remains active until a validated build succeeds.
- `project_not_found`: run `tobari` from the intended project directory.
- `project_session_attached`: exit the attached session and retry `tobari
  delete`, or use `tobari delete --force` only when terminating that session is
  intentional.
- intended request returns `403`: ask the user to run `tobari policy review` on
  the host, approve one exact candidate with `policy allow --id`, and retry;
  use `cluster denials` plus a tested host edit only when the exact learning
  flow cannot express the required behavior.
- root bind-mount error under Colima/Lima: place the project and Tobari XDG
  configuration/state directories on paths shared with the Docker VM, then
  rerun `tobari doctor` from the project directory and `tobari cluster up`.

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
task authbroker:test
task authbroker:image:check
task integration:test
task runtime:test
task security
task public:check
```

The integration profile creates two CWD-owned Tobari, dedicated internal
networks, one shared Gateway, OPA, and Auth Broker, and a mock upstream. It proves network
separation, HTTP and HTTPS enforcement, default client-auth forwarding,
tool-home persistence, broker lock/unlock and project-handle separation,
deny-before-resolution, fail-closed outages, CWD resolution, runtime recovery,
typed denial recovery, tested host-policy activation, terminal exit behavior,
concurrency, idempotency, and exact cleanup. Automated auth tests use only
synthetic credentials; `task integration:test` is the required reproducible
Auth Broker proof. Live GitHub login is the separate manual release scenario,
including a no-print equality check that `gh auth token --hostname github.com`
returns the exact projected `GH_TOKEN` handle, in
[Agent Readiness Validation](docs/09_agent_readiness_validation.md).

## MVP exclusions

The MVP excludes multiple clusters, process-level identity, transparent
proxying, raw TCP/UDP/QUIC, Git SSH semantic inspection, provider-operation
adapters, arbitrary executable provider helpers, AWS SigV4, OAuth refresh,
GitHub App tokens, remote token revocation, multiple provider accounts per
Context, Git credential helpers, approval workflows, policy engines other than
OPA, Kubernetes, filesystem overlays, private clone mode, GUI, remote
execution, and production multi-tenancy. macOS Keychain is used narrowly for
the installation Auth Broker root key; it is not mounted into a Workspace or a
general provider-keychain integration.

## License

Tobari is available under the [MIT License](LICENSE).
