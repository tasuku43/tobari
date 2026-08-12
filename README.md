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
- A preset guardrail is an immutable ceiling above baseline data, learned
  policy, and Advanced Rego.
- Tool-native login inside a Workspace is explicitly Workspace-owned.
- Optional brokered authentication keeps a static primary secret in an
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
artifact. Routine published binaries use reviewed immutable component digests.

## Quick Start

```sh
# Create a named immutable capability envelope.
tobari context create --name default \
  --source-access read-write \
  --policy-preset builtin/reviewed-exact

# Reconcile the one installation-local Gateway/OPA/Auth Broker cluster.
tobari cluster up

# From the project directory, create/reuse and enter its Workspace.
cd /path/to/project
tobari
```

`context create` owns the omission defaults: `read-write` and
`builtin/reviewed-exact`. Persisted exact-V1 Contexts always contain both
source access and the normalized preset origin/revision. Readers do not invent
defaults for old state; recreate unpublished development state after contract
changes.

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
- one snapshotted policy-preset origin and SHA-256 revision;
- guided or Advanced policy mode;
- narrow shell/Git presentation fallbacks;
- separately stored policy and encrypted broker-vault boundaries.

The manifest contains no secret, root key, broker vault path, or handle.
`context use` changes only the default for later omitted Context selections; it
does not retarget existing Workspaces.

```sh
tobari context list
tobari context show --name default
tobari context use --name default

# The same root can have another independent Context-bound Workspace.
tobari --context restricted
```

### Source access

`read-write` permits source reads and mutation. `read-only` permits reads but
denies create/change/delete, mode changes, and Git metadata writes through the
source bind. In both cases the Workspace home and tmpfs stay writable. Tobari
adds no writable source alias, and reconciliation includes the access mode in
the runtime spec/hash and Docker inspection.

### Policy presets

- `builtin/offline`: no immediate grant, no review-eligible effect, terminally
  deny all HTTP and HTTPS.
- `builtin/reviewed-exact`: no immediate grant; only guardrail-eligible effects
  may enter exact review.
- `builtin/get-only-reviewed`: no immediate grant; only guardrail-eligible GET
  effects may enter exact review; HEAD and every non-GET are terminally denied.

GET is not described as safe or read-only. A terminal guardrail denial creates
no candidate and makes zero external DNS, Broker-resolution, or upstream calls.
There is no implicit agent/model-provider bypass.

Custom presets are strict owner-only schema-V1 non-executable local data. V1
rejects wildcard, IP/private destination, secret, shell, Rego, include,
inheritance, remote fetch, refresh, signing, symlink, unsafe-mode, and unknown-
field input. Context creation normalizes, validates, digests, and snapshots the
preset. Later source changes affect only a newly created Context.

## Exact permission workflow

When a request is eligible for exact review, Gateway retains a bounded
secret-free denial and gives the child fixed trusted-host navigation. Keep the
Workspace and agent session running, then use a separate host terminal:

```sh
tobari policy review
```

The Permission Inbox groups by validated Context/project identity. Inspect an
exact detail, stage Allow-exact or Deny-exact, and confirm one final Apply.
Staging grants nothing. Refresh preserves decisions only by opaque candidate ID;
labels, order, indentation, or similar paths never create authority. Confirmed
Apply returns the active revision and exact decision receipts. Retry the
original request in the same Workspace.

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

Advanced Contexts may add trusted-host Rego constraints, but Advanced Rego is
beneath the preset guardrail and cannot redefine exact learned identity or the
Tobari-owned router.

## Authentication

### Workspace-owned

The universal path is to run the tool's own login inside its Workspace. The
files persist below that Workspace home and are available to every process in
that Workspace. Tobari does not call them host-managed or outside the
Workspace. Their network effects still require Gateway/OPA allow.

```sh
gh auth login          # example of Workspace-owned tool login
```

### Brokered reviewed providers

The reviewed built-ins are GitHub/`gh`, AWS/`aws`, Datadog/`pup`,
OpenAI/Codex's contract-checked host login, Anthropic/Claude Code 2.1.220, and
static Chatwork/`cwk`.
Omit `--provider` on an interactive trusted-host terminal to choose an
installed reviewed login driver, or supply it for deterministic automation:

```sh
tobari auth login --context default
tobari auth login --provider github --context default
tobari auth login --provider aws --method identity-center --context default
tobari auth login --provider aws --method console --context default
tobari auth login --provider datadog --context default
tobari auth login --provider openai --context default
tobari auth login --provider anthropic --context default
tobari auth status --context default
# Re-enter the matching Workspace to receive/rotate its project-bound handle.
tobari auth logout github --context default
```

Each host driver has fixed argv, a canonical executable outside the project,
digest checks, an isolated private home, bounded visible output and state
capture, cancellation, and checked cleanup. GitHub remains API-only with no Git
credential setup. AWS supports the closed IAM Identity Center and console
cross-device methods. Datadog is fixed to default-organization US1 OAuth.
OpenAI requires the reviewed Codex host-login contract and records its stable
product version; Anthropic still requires the exact reviewed Claude version.

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
revision, refresh a fixed Datadog/OpenAI session, or sign one bounded AWS SigV4
request. A private authenticated companion performs only the compiled AWS
credential-export operation; it opens no listener. Outcome-unknown refresh or
signing is not replayed automatically. Anthropic, GitHub, Chatwork, and owner
plans remain static. Invalid, copied, stale, ambiguous, or mismatched handles
fail without fallback. Managed Gateway profiles and arbitrary executable
adapters remain unsupported.

## Runtime customization

```sh
tobari runtime init
# Edit the selected Context's runtime/Dockerfile.
tobari runtime build
```

The explicit build uses only the Context runtime directory, validates the
result against runtime API 1, and promotes the image atomically. A failure
preserves the prior selection. Existing Workspaces adopt the new bound-Context
image on their next entry while preserving home.

The public base retains Git, curl, jq, Python, SSH, GitHub CLI, and AWS CLI as
ordinary Workspace tools. Their presence grants no broker/provider/network
authority. Optional toolbox and Claude/Codex variants remain local/CI artifacts
until their own redistribution and license decisions are complete.

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
the paired component images first. Their generated immutable lock is injected
into every CLI archive without a digest-pin commit. The manual release workflow
then creates the immutable GitHub Release and, for a stable version, opens a
Formula-only pull request in `tasuku43/homebrew-tap` from the exact audited
Formula asset. Dry runs and prereleases never mutate the tap.

## Security reports

Do not open a public issue containing sensitive details. Follow
[SECURITY.md](SECURITY.md) and use GitHub private vulnerability reporting.

## License

Tobari is licensed under the MIT License. See [LICENSE](LICENSE).
