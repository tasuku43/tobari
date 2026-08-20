# Tobari

Tobari gives coding agents a boundary in advance, then lets them work freely
inside it. A Workspace receives one selected project root, a persistent home,
bounded runtime resources, and HTTP/HTTPS access enforced by one shared Gateway
and OPA. Direct Internet egress is unavailable; denied effects become bounded,
secret-free evidence that the trusted host can review and approve exactly.

Tobari is pre-public V1 software. The repository is preparing its first public
release; immutable Gateway/Auth Broker image indexes are not yet published.

## What V1 protects

- Each Workspace joins only its dedicated guarded internal network.
- Gateway and OPA authorize the normalized HTTP/HTTPS effect, not a command,
  process, agent brand, or request body.
- OPA, Gateway management, Docker control, other Workspaces, the host home, and
  host-managed credentials are not exposed to a Workspace.
- The selected source bind is exactly `read-only` or `read-write`; Workspace
  home and tmpfs remain writable.
- Learned permission is exact Context, project, scheme, host, port, method, and
  raw path. A declared GraphQL endpoint adds operation type and root field.
- A Context method policy gives every HTTP method one `allow`, `exact_review`,
  or `deny` decision beneath an independent immutable destination ceiling.
  Terminal destination and method Deny decisions precede baseline data,
  learned policy, and Advanced Rego.
- Every declared provider binding is Broker-required: a real Workspace
  credential is rejected before policy, and only a project-bound handle is
  accepted.
- Tool-native login inside a Workspace remains explicitly Workspace-owned only
  as compatibility for undeclared provider bindings.
- Native Workspace login routes `BROWSER`, `GH_BROWSER`, and `xdg-open` through
  one attachment-scoped Tobari opener. The host opens only a strictly validated
  authentication target; manual-code tools retain their own confirmation and
  copy window, while Codex, GitHub, AWS SSO, and pup callback variants receive
  only their reviewed one-shot loopback relay. Tobari neither observes terminal
  output nor consumes input.
- Brokered authentication keeps static, renewable, or signing state in an
  encrypted Context vault and gives each Workspace only a project-bound handle.

Tobari does not claim filesystem integrity for a read-write source or for
overlapping roots. A read-only source is a live bind, not a snapshot: host
changes and same-root read-write Context changes remain observable. Allowed
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

The development binary selects local source images and is not a release
artifact. Its canonical base contains pinned Claude Code 2.1.220 and Codex
0.147.0. `task build` compiles the standard capability profile;
`task build:dev` adds experimental capabilities such as AWS authentication.
The agent-ready base is built locally from Tobari's pinned embedded recipe and
is never published by Tobari. Published binaries use immutable Gateway and
Auth Broker identities from their release lock.

## Quick Start

```sh
# From the project directory, start the guided first-use flow.
cd /path/to/project
tobari
```

On first use, `tobari` opens the ordinary Context wizard, shows the selected
standard runtime in the final review, prepares the shared Gateway and OPA after
Context confirmation, and then offers to enter with that runtime or create a
custom Dockerfile recipe before the first Workspace. The custom path stops
before Workspace creation and gives the exact `tobari runtime build` and
subsequent `tobari` commands.

The individual operations remain available for automation and advanced use:

```sh
# Create the envelope deterministically for automation.
tobari context create --name default \
  --source-access read-write \
  --native-readiness enabled

# Reconcile shared services explicitly when automation owns the sequence.
tobari cluster up

# Optionally snapshot one secret-free AWS IAM Identity Center profile for new Workspaces.
tobari config bootstrap aws --profile engineering

# Optionally compose one reviewed EKS context using that same AWS profile.
tobari config bootstrap kubernetes eks --kube-context engineering

# Create/reuse and enter the Workspace.
tobari
```

The argument-free command requires terminal stdin/stderr and text output. It
asks for the Context name, source access, and `allow` / `exact_review` / `deny`
for the extension-method default and each standard HTTP method, then optionally
selects one typed AWS IAM Identity Center bootstrap profile and creates
once. Any explicit input selects direct mode and requires `--name`; redirected
and JSON argument-free invocations fail before mutation.

Direct `context create` owns the omission defaults: `read-write`, the fixed
Context method policy, and enabled native readiness. New Contexts persist the
normalized Context policy revision and the separate readiness choice. The
agent-ready compatibility baseline is composed by the trusted binary; it is
not a selectable profile or catalog entry.

Leaving the child shell detaches only the session. The Workspace and persistent
home remain available:

```sh
tobari                 # resume
tobari status          # observe
tobari delete          # remove Workspace runtime/home, preserve project files
```

Use `tobari delete --force` only when intentionally terminating another
attached session. `cluster down` requires all Workspaces to be deleted first.

## Context capability envelope

A Context permanently binds every Workspace created with its stable ID to:

- one compatible runtime image and read-only agent profile;
- `source_access: read-only|read-write`;
- one Context-owned normalized policy snapshot and SHA-256 revision;
- guided or Advanced policy mode;
- narrow shell/Git presentation fallbacks;
- an optional secret-free create-only Workspace bootstrap snapshot;
- separately stored policy and encrypted broker-vault boundaries.

The manifest contains no secret, root key, broker vault path, or handle.
`context use` changes only the default for later omitted Context selections; it
does not retarget existing Workspaces.

```sh
tobari context list
tobari context show --name default
tobari context use --name default
tobari context delete --name disposable
tobari config bootstrap aws --refresh --context default
tobari config bootstrap kubernetes eks --refresh --context default

# The same root can have another independent Context-bound Workspace.
tobari --context restricted
```

`context list` renders one vertical card per Context so filesystem and network
method facts remain readable. `context show` keeps ordinary text focused on the
selected boundary, runtime state, and exact next command; add `--details` for
the complete sectioned host diagnostic. JSON is already complete and is
unchanged by that flag. `context delete` accepts only an additional
non-current Context with no bound Workspace. It preserves project files and
shared runtime images; the foundational `default` Context has no delete path.

### Source access

`read-write` permits source reads and mutation. `read-only` permits reads but
denies create/change/delete, mode changes, and Git metadata writes through the
source bind. In both cases the Workspace home and tmpfs stay writable. Tobari
adds no writable source alias, and reconciliation includes the access mode in
the runtime spec/hash and Docker inspection.

### Context policy and native readiness

Native readiness is an independent immutable Context capability and defaults to
`enabled`; use `--native-readiness disabled` for an intentionally strict
Context. Its finite exact overlay never overrides the Context-owned policy:
destination and method Deny decisions filter it, and exact Deny still wins.

Context creation collects one complete HTTP method `default` plus exact
`overrides`. Unknown and extension methods receive that default. `context list`
and `context show` expose the effective method policy and immutable
`policy_revision`; there is no user-selectable policy catalog.

The fixed agent-ready baseline is trusted-binary data composed into each new
Context snapshot. It supplies the reviewed Claude Code, Codex, GitHub CLI, TWG,
and pup compatibility routes when the corresponding clients are present, but
it is not a reusable profile and does not install any client. Context method
choices can express deny-only, exact-review, GET-only, or other bounded
postures. Method Allow is Context-wide rather than process identity, and exact
Deny still overrides it. A terminal denial creates no candidate and makes zero
external DNS, Broker-resolution, or upstream calls.

Context policy snapshots are strict owner-only schema-V1 non-executable data.
They reject wildcard, IP/private destination, secret, shell, Rego, include,
inheritance, remote fetch, refresh, signing, symlink, unsafe-mode, and unknown
fields. Creation normalizes, validates, digests, and stores the snapshot at
`contexts/<name>/policy/context.json`; later changes cannot rewrite an existing
Context. Native readiness is the exception: when enabled, it selects the
installed binary current reviewed overlay without rewriting the snapshot.
Run `tobari cluster up` after a binary upgrade to activate that overlay; until
then status marks the older projection invalid and root entry fails closed with
the same recovery command.

## Permission workflow

When a request is eligible for review, Gateway retains a bounded
secret-free denial and gives the child fixed trusted-host navigation. Keep the
Workspace and agent session running, then use a separate host terminal:

```sh
tobari policy review
# Or keep a trusted-host raw terminal waiting for new denials:
tobari policy review --watch
# Disable or explicitly choose its terminal-emulator cue:
tobari policy review --watch --notify=off
```

The Permission Inbox groups by validated Context/project identity. One distinct
path remains exact. After a second compatible distinct HTTP path, Inbox proposes
a single-segment `/path/{id}` template. Inspect its examples and explicit future
scope. Exact Allow or Deny can be staged and cleared directly from the raw list;
template Allow remains detail-only. Stage Allow template, Allow observed exact, or Deny pending exact
and confirm one final Apply. Staging grants nothing. Refresh preserves decisions
only by opaque typed review-item ID; labels, order, or indentation never create
authority. Confirmed Apply returns the active revision and stored-rule receipts. Retry the
original request in the same Workspace.

Watch refreshes the same bounded Inbox snapshot, preserves staging and focus by
opaque typed ID, backs off after refresh failures, and stays open after Apply.
It keeps one alternate-screen frame between Apply operations and does not
repaint an unchanged successful timer refresh.
It never retries the denied request or gives the Workspace a policy-mutation
channel. Redirected and JSON review remain one read-only snapshot.
By default, a successful refresh that adds at least one previously unseen typed
review item emits one fixed terminal-emulator cue. `--notify=osc9`, `bel`, or
`off` selects it explicitly; `auto` uses OSC 9 in an identified iTerm2 or cmux
terminal and conservatively falls back to BEL elsewhere. Tobari
never puts denial evidence in the control payload or configures OS, tmux, or SSH
notification passthrough.

The experimental development profile also offers the same trusted-host workflow
in a foreground browser Operator Console. Build and invoke that profile
explicitly:

```sh
task build:dev
./bin/tobari-dev serve
# or print the URL without opening the host browser
./bin/tobari-dev serve --no-open
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
curl http://host.tobari.test:3000/health
```

No entry flag or service declaration is needed. The first exact host, port,
method, and path remains denied until it is approved through interactive
`tobari policy review`. That decision is Workspace-wide only for the current
route-owning attachment; exit revokes it, and it never appears in persistent
`policy rules`. `TOBARI_CAPABILITIES_JSON` describes the URL template, port
range, lifetime, and audience without exposing relay credentials. This outcome
does not provide host Docker, Compose control, raw TCP, privileged ports, or
private-LAN access.

Advanced Contexts may add trusted-host Rego constraints, but Advanced Rego is
beneath the Context policy ceiling and cannot redefine exact learned identity or the
Tobari-owned router.

## Authentication

### Broker-first routing

For every exact provider binding Tobari declares, the Workspace must use its
project-bound handle. A real token, session credential, or direct AWS signature
at that binding is rejected as `broker_auth_required` before OPA or upstream
I/O. Broker login still grants no network permission; Gateway/OPA authorize the
ordinary HTTP effect separately.

Workspace-owned login or environment/file injection remains a compatibility
path only for a request that matches no declared provider binding. Its files
persist below that Workspace home and are available to every process in the
Workspace. This path is useful for an unsupported provider, but it does not
have the Broker's primary-secret isolation, rotation, or refresh/signing
boundary.

For an unsupported provider, use that tool's own login or credential injection
only with the understanding that the credential is Workspace-readable and the
compatibility route disappears if Tobari later declares the exact binding.

### Brokered reviewed providers

The standard reviewed built-ins are GitHub/`gh`, Datadog/`pup` from the
selected Context runtime,
OpenAI/Codex's contract-checked host login, Anthropic/Claude Code 2.1.220 from
the selected Context runtime, and
static Chatwork/`cwk`. The experimental `task build:dev` profile additionally
enables AWS/`aws`.
Omit `--provider` on an interactive trusted-host terminal to choose an
installed reviewed login driver, or supply it for deterministic automation:

```sh
tobari auth login --context default
tobari auth login --provider github --context default
tobari auth login --provider datadog --context default
tobari auth login --provider openai --context default
tobari auth login --provider anthropic --context default
tobari auth status --context default
# Re-enter the matching Workspace to receive/rotate its project-bound handle.
tobari auth logout github --context default
```

Experimental AWS login uses `bin/tobari-dev auth login --provider aws` with
`--method identity-center` or `--method console`.

Each acquisition driver has fixed argv, digest checks, an isolated private
state boundary, bounded visible output and state capture, cancellation, and
checked cleanup. GitHub remains API-only with no Git
credential setup. AWS supports the closed IAM Identity Center and console
cross-device methods. Datadog runs structurally compatible pup from a fresh
mount-free selected-Context-runtime container, bridges the validated localhost
callback only through pup stdin, and is fixed to default-organization US1 OAuth.
OpenAI requires the reviewed Codex host-login contract and records its stable
product version. Anthropic starts a fresh mount-free container from the selected
compatible Context image, requires exact `/usr/local/bin/claude` 2.1.220,
opens only its validated native authorization URL, captures the resulting
renewable session, and removes the container before commit. Workspace entry
projects only the scoped handle plus the non-secret Claude onboarding-complete
field, so authenticated `claude` starts without repeating account selection.

Chatwork and owner providers use strict schema-V1 static plans and protected
stdin:

```sh
printf '%s' "$STATIC_SECRET" | tobari auth import example --context default
```

Do not put secrets in argv, environment-based Tobari configuration, manifests,
logs, or fixtures. Terminal stdin is rejected before reading. An owner manifest
is non-secret, non-executable local data describing bounded handle projection
and exact HTTPS/header replacement. It cannot select a helper, dynamic record,
refresh, signer, shell, executable, arbitrary route/method, policy, remote
fetch, or provider business operation.

Gateway removes one recognized handle, performs full non-secret
Context/project/provider/revision/target/header introspection, and asks OPA
about the ordinary HTTP effect. Only after allow may Broker resolve the same
revision, refresh a fixed Datadog/OpenAI/Anthropic session, or sign one bounded AWS SigV4
request. A private authenticated companion performs only the compiled AWS
credential-export operation; it opens no listener. Outcome-unknown refresh or
signing is not replayed automatically. GitHub, Chatwork, and owner
plans remain static. Invalid, copied, stale, ambiguous, or mismatched handles
fail without fallback. Managed Gateway profiles and arbitrary executable
adapters remain unsupported.

Acquisition and runtime use are separate boundaries. Reviewed host drivers may
run `gh`, `aws`, or Codex on the host; pup and Claude run in isolated containers
from the selected Context image to acquire Context-owned state;
Gateway later accepts only a handle at that provider's declared request
binding. Denying a few token endpoints alone would not enforce this boundary,
because an already acquired token could still be injected into a Workspace.
Tobari therefore enforces the exact runtime binding. It does not infer login
intent from a command or process name.

## Runtime customization

```sh
tobari runtime create --name frontend
# Edit every required file in the reported Runtime source directory.
tobari runtime build --name frontend
tobari context runtime set --runtime frontend@1
```

The explicit build snapshots the complete bounded Runtime source tree,
validates the result against runtime API 1, and appends an immutable successful
revision without changing any Context. Selection is a separate Context action,
so the same revision can be reused by several Contexts. Existing Workspaces
adopt a changed binding on their next entry while preserving home.

The public base retains Git, curl, jq, Python, SSH, GitHub CLI, AWS CLI, Claude
Code, and Codex as ordinary Workspace tools. Their presence grants no
broker/provider/network authority. A custom Context runtime must add pup before
it can acquire Datadog credentials.

## Command discovery

```sh
tobari help
tobari help --format agent
tobari help context --format agent
tobari help policy review --format agent
```

The root agent form is a bounded capability index. Select one namespace or
exact command to retrieve complete typed inputs, output, failures, mutation
facts, and workflows. `cli.Catalog` is the single public-command source of
truth.

## Diagnostics

```sh
tobari doctor
tobari cluster status
tobari cluster denials
tobari cluster logs --component gateway --tail 200
```

Reads are observational: they do not initialize Context, policy, key, vault,
Broker, project, or Docker state. External text is untrusted and visibly
projected; printable prompt-like meaning is not filtered. Opaque references are
validated and passed byte-for-byte unchanged.

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

Local preparation creates the exact five CLI archives plus `checksums.txt`, an
archive-subject SPDX 2.3 SBOM (`filesAnalyzed: false`), and unsigned in-toto/
SLSA provenance metadata. These are integrity and auditable metadata, not a
signature, dependency/layer inventory, vulnerability report, or independent
builder proof.

Preparation stops for explicit approval before any external mutation. Do not
push a branch or tag, publish OCI images, create a GitHub Release, or update a
Homebrew tap as part of local preparation. After approval, publish and inspect
only the Gateway and Auth Broker images first. Their generated immutable lock
is injected into every CLI archive without a digest-pin commit. The
agent-ready base is built locally from the released CLI's pinned recipe and is
never published by Tobari. The manual release workflow
then creates the immutable GitHub Release and, for a stable version, opens a
Formula-only pull request in `tasuku43/homebrew-tap` from the exact audited
Formula asset. Dry runs and prereleases never mutate the tap.

## Security reports

Do not open a public issue containing sensitive details. Follow
[SECURITY.md](SECURITY.md) and use GitHub private vulnerability reporting.

## License

Tobari is licensed under the MIT License. See [LICENSE](LICENSE).
