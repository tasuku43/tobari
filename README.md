# Tobari

Tobari gives coding agents a boundary in advance, then lets them work freely
inside it. A Workspace receives one selected project root, a persistent home,
bounded runtime resources, and HTTP/HTTPS access enforced by one shared Gateway
and OPA. Direct Internet egress is unavailable; denied effects become bounded,
secret-free evidence that the trusted host can review and approve exactly.

Tobari is pre-public V1 software. The repository is preparing its first public
release. Tobari publishes no OCI images; a released release-surface binary builds its
pinned Gateway and agent-ready base locally from embedded source recipes.

## What V1 protects

- Each Workspace joins only its dedicated guarded internal network.
- Gateway and OPA authorize the normalized HTTP/HTTPS effect, not a command,
  process, agent brand, or request body.
- OPA, Gateway management, Docker control, other Workspaces, the host home, and
  host-managed credentials are not exposed to a Workspace.
- The selected source bind is exactly `read-only` or `read-write`; Workspace
  home and tmpfs remain writable.
- Learned permission is exact Context, Workspace, scheme, host, port, method, and
  raw path. Closed semantic modules add only their bounded request coordinates:
  GraphQL/MCP, AWS, Kubernetes, Git Smart HTTP, or OCI Distribution. Signed AWS
  Query/JSON RPC adds wire protocol, SigV4 service, exact version/namespace, and
  operation; Tobari does not need an AWS service catalog or infer IAM/read-write
  semantics.
- A Workspace Template V1 Method Boundary lists terminal denied HTTP methods
  beneath an independent public-HTTPS destination ceiling.
  Terminal destination and method Deny decisions precede baseline data,
  learned policy, and the fixed Tobari evaluator over canonical typed data.
- On the release surface, each tool creates and owns its authentication state
  inside one persistent Workspace home. Every process in that Workspace can
  read that state; host CLI homes and host credentials are never inherited.
- Gateway removes client authentication and cookies from OPA input and Tobari
  audit evidence, then forwards the original values only after policy allows
  the ordinary HTTP effect.
- Native Workspace login routes `BROWSER`, `GH_BROWSER`, and `xdg-open` through
  one attachment-scoped Tobari opener. The host opens only a strictly validated
  authentication target; manual-code tools retain their own confirmation and
  copy window, while Codex, GitHub, AWS SSO, and pup callback variants receive
  only their reviewed one-shot loopback relay. Tobari neither observes terminal
  output nor consumes input. This bridge neither owns credentials nor grants a
  Workspace HTTP permission.

Tobari does not claim filesystem integrity for a read-write source or for
overlapping roots. A read-only source is a live bind, not a snapshot: host
changes and same-root read-write Workspace changes remain observable. Allowed
destinations can receive payload chosen by the Workspace. Docker/kernel escape,
multi-tenant production isolation, raw TCP, UDP, QUIC, recursive DNS, Git SSH,
clone/overlay/apply-back, and process-level identity are outside V1.

## Requirements

- Linux with Docker Engine, or macOS with Colima
- Go and repository tools for source builds
- an interactive terminal for Workspace entry and guided review

Windows CLI archives are buildable, but the Workspace runtime is not supported
on Windows in V1.

## Install a stable release on macOS

After the first stable release is published through the shared tap:

```sh
brew install tasuku43/tap/tobari
tobari version
```

Linux users install a published archive or build from source.

## Build from source

```sh
task build
bin/tobari version
```

The repository `task build` binary selects local source images and is not a
release artifact. It is the release surface with the development resolver and
is named `bin/tobari`. Its canonical base contains pinned Claude Code 2.1.220
and Codex 0.147.0. `task build:dev` retains the contributor task path but builds
the separate research surface as `bin/tobari-research`; it adds unsupported
repository-only capabilities such as the Auth Broker research path, AWS broker
acquisition, and the Operator Console.
The agent-ready base is built locally from Tobari's pinned embedded recipe and
is never published by Tobari. A released release-surface binary likewise builds its
pinned Gateway locally and contains no Auth Broker service or command.

## Quick Start

```sh
# From the project directory, start the guided first-use flow.
cd /path/to/project
tobari

# Or prepare the same Workspace and run one exact foreground command.
tobari -- claude
tobari -- codex exec "Fix the failing tests"
```

On first use, `tobari` shows one recommended review for the canonical project:
direct read-write project access, routine Claude Code and Codex traffic,
exact review for other requests, private-destination denial, `standard@1`, no
host import, and Bash or the exact requested executable. **Start Workspace**
publishes the reviewed `default` Workspace Template, selects it as the
installation default, creates this Project's Context, prepares Gateway and OPA,
reconciles the Workspace, and enters it. **Customize** edits the same complete
Template draft before those mutations. The draft has no identity or authority
until Start is confirmed, and no host configuration is imported implicitly.

Progress is written to Tobari-owned stderr before child handoff. It has five
checkpoint-local lines: Check requirements, Save setup or Use Context, Prepare
protection, Prepare Workspace, and Enter Workspace. A completed line proves
only that checkpoint. Long work shows bounded elapsed/wait information, not a
percentage or ETA. If setup fails or is interrupted, the fault preserves known
Template, Context, cluster, and Workspace facts and gives one causal Next.

The same authorities remain available through exact commands:

```sh
# Inspect reusable setup and Project-specific Contexts.
tobari template list
tobari context list

# Reconcile shared services explicitly when automation owns the sequence.
tobari cluster up

# Select the location-free current Context by its unchanged opaque reference.
tobari context use --id <context-ref>

# At a root without a Workspace, bare entry creates it from that current Context.
tobari
tobari -- claude
```

With no command, Tobari enters Bash as before. After `--`, it passes the command
and every argument directly to Docker without shell expansion or reparsing.
The command is a foreground child exec, not container PID 1. When it exits,
Tobari returns its exact status to the host shell instead of opening Bash.
On a fresh Project, a direct command still goes through the one interactive
review. After authority exists, `tobari -- claude`, `tobari -- codex exec ...`,
and `tobari -- gh auth login` are routine direct-entry forms.

Native clients sign in inside the persistent Workspace home after policy is
applied. Tobari never inspects or copies a host credential. A fresh interactive
shell shows this once; a direct command relies on the client's own prompt.

Leaving the child shell or direct command detaches only the session. The
Workspace and persistent home remain available:

```sh
tobari                    # resume the default Context
tobari status             # observe without reconciliation
tobari workspace list     # obtain the exact Workspace reference
tobari workspace delete --id <workspace-ref> --confirm=delete
```

Add `--force` to the exact `workspace delete` only when intentionally
terminating its attached session. `cluster down` requires all Workspaces to be
deleted first.

## Workspace Templates, Contexts, and Workspaces

A Workspace Template is one reusable static setup with a stable ID and complete
immutable revisions. It owns the source/network Boundary, baseline policy,
exact Runtime revision, and typed entry/session/creation defaults. It owns no
Project, learned decision, Workspace home, authentication, attachment, applied
receipt, or observation.

A Context is the durable binding between one canonical Project root and one
Template ID. Its Policy Memory retains that Project's reviewed decisions across
Workspace replacement. A Workspace is the replaceable applied instance and
persistent home for one Context. These identities and lifetimes are distinct;
names, roots, generations, images, containers, and timestamps never authorize a
mutation.

```sh
# Discover exact opaque references.
tobari template list
tobari template show --id <template-ref>
tobari context list
tobari context show --id <context-ref>
tobari workspace list

# Copy one exact immutable Template revision into a fresh independent Template.
tobari template copy --from <template-revision-ref> --name restricted

# Create a Context, select it, then let bare entry bind the CWD-owned Workspace.
tobari context create --template <template-ref>
tobari context use --id <context-ref>
tobari -- codex exec "Fix the failing tests"
```

`template default set --id <template-ref>` changes only later bare root/status
initialization. `context use --id <context-ref>` changes only the installation
current Context; it reads no CWD and never retargets an existing Workspace.
Context-aware options such as `policy assist --context` use that selection when
omitted and override it for one invocation when present. Template copy
records no lineage and copies no Context, Policy Memory, Workspace, home,
authentication, applied, failure, observed, or default state. Reads are
observational: Template mutation does not activate cluster policy, and Context
mutation does not reconcile a Workspace.

`read-write` source access permits source mutation; `read-only` denies source
create/change/delete, mode, and Git-metadata writes while leaving Workspace
home and tmpfs writable. The immutable Template Boundary also fixes terminal
destination and method ceilings. Baseline policy and native client readiness
cannot widen those ceilings, and exact Deny remains terminal.

## Permission workflow

Tobari presents network authority in three user-facing layers:

| Layer | What it means |
|---|---|
| Workspace Template Access | The immutable destination and method Boundary plus reviewed baseline policy |
| Remembered Context decisions | Host-reviewed Allow and exact Deny choices in one Context's Policy Memory; they remain until `policy reset` |
| This-session Host Loopback access | Exact access to physical-host loopback for the active attachment only; it disappears when the owning attachment exits |

The third layer is a separate closed branch, not a temporary widening of
ordinary Internet access. Ordinary Template and Context authority cannot
authorize Host Loopback, and Host Loopback decisions cannot authorize an
ordinary destination.

When a request is eligible for review, Gateway retains a bounded secret-free
denial and gives the child fixed trusted-host navigation. A supported ordinary
HTTP or HTTPS denial also prints one attachment-local wait command. The agent
may run it inside the same attached Workspace while the user keeps the
Workspace running and reviews from a separate trusted-host terminal:

```sh
# Inside the attached Workspace; use the exact ID printed by Gateway.
tobari-permission wait --id pwt_0123456789abcdef0123456789abcdef
```

On the host:

```sh
tobari review permissions
```

The attached Workspace reserves no Tobari key prefix. `Ctrl+]` and all other
input remain child-owned, so Permission Inbox stays in this separate trusted-host
terminal rather than replacing a shell or full-screen child's presentation.

The Permission Inbox presents one validated final snapshot as a list, typed
detail, and final staged review. Exact requests can stage Allow or Deny; a
typed single-segment `/path/{id}` proposal can stage only its matching-path
Allow. Choices can be cleared before one explicit Apply. Staging grants
nothing. Manual refresh discards the staged set and reads fresh authority;
labels, order, color, or indentation never transfer a decision. Confirmed Apply
returns the active revision and stored-rule receipts.
The waiting helper then returns only `Allow`, `Deny`, or `Expired`. `Allow`
means the exact effect is retry-ready; the agent must deliberately issue a
fresh request, which Gateway authorizes independently. The helper never
proposes, approves, mutates policy, or retries the original request. Unsupported
denials omit the wait handoff and retain the same host-review workflow.

The Permission Inbox never retries the denied request or gives the Workspace a
policy-mutation channel. Redirected and JSON review remain one read-only
snapshot. The release command has no watch or notification option.

The research surface also offers the same trusted-host workflow
in a foreground browser Operator Console. Build and invoke that profile
explicitly:

```sh
task build:dev
./bin/tobari-research serve
# or print the URL without opening the host browser
./bin/tobari-research serve --no-open
```

The console combines cluster health, local Workspaces, the Permission Inbox,
and current learned rules. Its dense Operator Console theme supports dark and
light modes. Decisions are inert until the final review and explicit Apply,
which reuses the same typed `policy apply-reviewed` mutation and returns the
authoritative active revision. The server binds only a random IPv4-loopback
port, keeps its session bearer in the URL fragment and browser tab, loads no
external assets, and stops with the foreground process. It has no daemon,
remote-bind, or caller-selected-port mode.
The standard `task build` binary and release archives do not expose `serve`.

Machine workflows use unchanged opaque references:

```sh
tobari policy candidates --format json
tobari policy allow --id "$CANDIDATE_ID"
tobari policy deny --id "$CANDIDATE_ID"
tobari policy rules --format json
tobari policy reset --id "$RULE_ID"
```

Reset removes one CLI-owned learned Allow or exact Deny and returns the effect
to default deny; it does not grant or retry. V1 has no prefix learned rule,
compaction command/reference/state, wildcard learned authority, or observation-
derived widening.

### Physical-host loopback HTTP

Inside a Workspace, `localhost` means that Workspace. To reach an HTTP server
on the physical host, use the constant Host Loopback URL and the server's
non-privileged port:

```sh
curl http://host.tobari.internal:3000/health
```

No entry flag or service declaration is needed. The first exact host, port,
method, and path remains denied until it is approved through interactive
`tobari review permissions`. That decision is Workspace-wide only for the current
route-owning attachment; exit revokes it, and it never appears in persistent
`policy rules`. `TOBARI_CAPABILITIES_JSON` describes the URL template, port
range, lifetime, and audience without exposing relay credentials. This outcome
does not provide host Docker, Compose control, raw TCP, privileged ports, or
private-LAN access.
The pre-public `host.tobari.test` spelling is retired, not an alias. Requests
using it fail locally with fresh-attachment guidance and cannot become ordinary
external permissions.

### Open a Workspace development service on the host

Keep the attached Workspace running. Inside it, request one exact loopback
service:

```sh
tobari-expose 3000
```

In a separate trusted-host terminal, run `tobari review services --watch`,
inspect the complete effect card, then press `a` for Allow once or `o` for
Allow once then Open. The waiting helper emits one final JSON document. Its
confirmed exposure URL has scheme `http`, a fresh
`svc-<128-bit-random-lowercase-label>.localhost:<random-port>` authority, and
root path `/`; the unrelated `exp_...` reference controls lifecycle changes.

```sh
tobari-expose status
tobari-expose stop exp_0123456789abcdef0123456789abcdef
tobari service status
tobari service open --id exp_0123456789abcdef0123456789abcdef
```

The socket still binds only IPv4 `127.0.0.1` on an OS-selected port. Only the
exact generated Host authority admits HTTP/1.1 and WebSocket Upgrade. Tobari
does not rewrite Host, Origin, redirects, cookies, headers, or content. The
attachment owns the listener: exit closes it and active connections. Approval
is not saved as Workspace Template policy or a remembered Context decision, and the
helper cannot choose a host port, publish to the LAN, or expose raw TCP.
The Workspace helpers are dedicated engine-native Linux programs built into
Tobari's verified base Runtime and mounted read-only even when the Workspace
Template selects a custom Runtime; they are not host release executables or
archive members.

The evaluator is embedded and Tobari-owned. Templates and Contexts contribute
only canonical typed policy data; ordinary users never create, edit, copy, or
manage executable policy source.

## Authentication

### Standard native Workspace authentication

Authentication belongs to each tool inside one Workspace. Run an installed
client through the ordinary entry path and use that client's normal login:

```sh
tobari -- claude
tobari -- codex
tobari -- gh auth login
```

Claude Code, Codex, GitHub CLI, AWS CLI, and other installed tools create and
persist their own login state below that Workspace's `HOME=/var/lib/tobari`.
The state is available to every process in the same Workspace, is not shared
with another Workspace, and is deleted with the Workspace. Tobari never mounts
or copies host CLI homes, host token caches, credential helpers, keychains, or
credential environment variables. A reviewed secret-free Workspace Template bootstrap is
configuration only; it imports no authentication state.

For pinned native clients, the attachment-scoped bridge may open one strictly
validated login target in the host browser and may relay one opaque loopback
callback for the closed reviewed callback flows. The client still owns OAuth
state, exchange, refresh, logout, account presentation, and credential storage.
The bridge cannot read the resulting credential and grants no HTTP permission.
Separately, Gateway redacts client authentication and cookies from OPA input
and Tobari audit evidence, asks policy about the ordinary HTTP effect, and
forwards the original values only after allow.

The standard and release binaries have no `auth` namespace. Use the client's
own status and logout commands inside the same Workspace, then leave and
re-enter the Workspace normally when needed.

| Authentication handoff | Standard path |
|---|---|
| Tobari credential command | none |
| Host credential transfer | none |
| Workspace client login | one normal client-owned flow |
| Browser transfer | host opens only a reviewed native target |
| Fixed-value manual re-entry | only when the provider's native flow requires it |
| Login-state owner | the client in one Workspace home |
| Steady-state Tobari command | none; re-enter the Workspace normally |

### Research Broker (repository-only)

The repository retains an unsupported, unpublished Auth Broker research
surface for development only. It is absent from the standard and release
surfaces and
cannot be enabled by a runtime flag, Runtime input, Template revision,
Workspace state, or renamed executable. Contributors must build the research
surface explicitly:

```sh
task build:dev
bin/tobari-research context list
bin/tobari-research context use --id <context-ref>
bin/tobari-research auth login --provider github
bin/tobari-research auth status
bin/tobari-research auth logout github
```

This research surface studies Context-owned vaults, project-bound handles, and a closed set of
reviewed provider acquisition plans; it is not a supported user authentication
path or release artifact. See [Authentication handling](docs/07_authentication.md#research-broker-surface)
for its detailed research contract.

## Runtime customization

```sh
tobari runtime create --name frontend
# Edit every required file in the reported Runtime source directory.
tobari runtime list
# Copy the opaque Runtime reference shown for frontend.
tobari runtime build --id '<runtime-ref>'
tobari review runtimes
# Edit the Runtime id+revision in the reported Template template.yaml.
tobari template plan --id <template-ref>
tobari template apply --plan <template-change-plan-ref>
tobari

# Or initialize another standalone source from frontend's current editable source.
tobari runtime create --copy-source-from frontend --name frontend-node22
```

The explicit build snapshots the complete bounded Runtime source tree,
validates the result against runtime API 1, and appends an immutable successful
revision without changing any Workspace Template. Selection is a complete
Template source edit plus planned Apply, so the same revision can be reused by several Templates. Existing Workspaces
adopt a changed binding on their next entry while preserving home. On an
interactive terminal, the review flows present the exact Runtime, Workspace
Template, current binding, and impact before Build or Apply. Scripts remain
deterministic by supplying opaque Runtime and Runtime-revision references
unchanged.

Managed Runtime cleanup keeps immutable history separate from replaceable
local image availability:

```sh
tobari review runtimes
tobari runtime restore --id '<runtime-revision-ref>'

tobari runtime prune dry-run
tobari runtime prune apply --plan '<runtime-prune-plan-ref>' --confirm=prune

tobari runtime delete --id '<runtime-ref>' --confirm=delete
```

Copy the opaque references from Runtime discovery or prune dry-run without
decoding them. Prune removes only exact unused execution material; restore
reconstructs one retained immutable revision; delete retires one complete
unused managed Runtime. Current or retained Template revision references, Workspace
applied/pending/observed use, external containers, unknown evidence, and the
built-in standard Runtime block destructive work. `review runtimes` provides
the trusted-host interactive build and interruption-recovery flow; redirected
and JSON review remain read-only.

Both copy operations are one-time initializers. Runtime copy reads current
editable source, not an immutable successful revision; revisions, history, and
lineage are not copied. Template copy reads one exact retained immutable
revision and retains no inheritance or lineage. It copies no Workspace, login,
learned permission, attachment authority, applied state, failure, observation,
or default selection.

The public base retains Git, curl, jq, Python, SSH, GitHub CLI, AWS CLI, Claude
Code, and Codex as ordinary Workspace tools. Their presence grants no
credential or network authority. A custom Workspace Template Runtime must add pup before
it can acquire Datadog credentials.

## Command discovery

```sh
tobari help
tobari help --format agent
tobari help template --format agent
tobari help context --format agent
tobari help review --format agent
tobari help review permissions --format agent
```

The root agent form is a bounded capability index. Select one namespace or
exact command to retrieve complete typed inputs, output, failures, mutation
facts, and workflows. `cli.Catalog` is the single public-command source of
truth.

## zsh completion

Initialize zsh completion, then source Tobari's generated adapter from
`.zshrc`:

```zsh
autoload -Uz compinit && compinit
source <(tobari completion zsh)
```

The adapter asks the current `tobari` executable for candidates on every Tab,
so command, flag, Workspace Template, Context, and Runtime additions do not require regenerating a
checked-in shell script. The read is local and creates no Tobari state. For
For example, `tobari temp<Tab>` completes to `tobari template`.

## Diagnostics

```sh
tobari doctor
tobari cluster status
tobari cluster denials
tobari cluster logs --component gateway --tail 200
```

Reads are observational: they do not initialize Template, Context, Policy Memory, key, vault,
Broker, Workspace, or Docker state. Standard reads do not inspect or create
tool-owned authentication state; the key, vault, and Broker observations apply
only to the research surface. External text is untrusted and
visibly projected; printable prompt-like meaning is not filtered. Opaque
references are validated and passed byte-for-byte unchanged.

The exact supported typed predecessor authority migrates only through
`installation migration plan` followed by
`installation migration apply --plan <migration-plan-ref>`. Ordinary reads,
startup, and `cluster up` never migrate implicitly. Unsupported, Advanced-Rego,
corrupt, or ambiguous predecessor state fails closed.

## Development and verification

```sh
task check:fast
task check
task security
task public:check
task release:check
```

`task check` decides implementation completion. Security/public/release work
also runs the named profiles. Integration requires a working supported Docker
environment.

Repository policy and architecture are documented in:

- [Project theses](docs/00_theses.md)
- [Product contract](docs/01_product_contract.md)
- [Architecture](docs/02_architecture.md)
- [Security model](docs/03_security_model.md)
- [Harness](docs/04_harness.md)
- [Authentication](docs/07_authentication.md)
- [External API contracts](docs/08_external_api_contracts.md)
- [Agent readiness](docs/09_agent_readiness_validation.md)

## Release checkpoint

Release preparation creates the exact five CLI archives plus `checksums.txt`, an
archive-subject SPDX 2.3 SBOM (`filesAnalyzed: false`), and unsigned in-toto/
SLSA provenance metadata. These are integrity and auditable metadata, not a
signature, dependency/layer inventory, vulnerability report, or independent
builder proof.

Preparation reuses the exact successful main-push CI run, retains one verified
asset set for seven days, and stops for explicit approval before publication.
Protected publication takes that preparation run ID and promotes only its
reverified bytes; it does not rebuild. Tobari publishes no OCI images. The
released standard CLI builds the pinned Gateway and agent-ready base locally
from its embedded recipes and contains no Auth Broker capability. The manual release workflow
creates the immutable GitHub Release and, for a stable version, opens a
Formula-only pull request in `tasuku43/homebrew-tap` from the exact audited
Formula asset. Preparation and prereleases never mutate the tap.

## Security reports

Do not open a public issue containing sensitive details. Follow
[SECURITY.md](SECURITY.md) and use GitHub private vulnerability reporting.

## License

Tobari is licensed under the MIT License. See [LICENSE](LICENSE).
