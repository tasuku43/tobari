# Harness

The surface/resolver matrices and release absence gates implement [ADR 0082](decisions/0082-release-and-research-build-surfaces.md).
The Host Loopback naming, retired-name, TLS, and resolver gates implement
[ADR 0083](decisions/0083-name-the-physical-host-loopback-authority.md).
The status-home 0/6/12 observation budget, zero-write boundary, and semantic
output fixtures implement [ADR 0085](decisions/0085-make-status-the-cwd-home.md).
Current-Context persistence, zero-CWD resolution, exact override precedence,
and CWD-only Workspace routing implement
[ADR 0093](decisions/0093-select-current-context-without-cwd.md).

The harness is the executable counterpart of the theses, product contract, architecture, security model, and release policy. Its goal is not to maximize the number of tools. Its goal is to make important regressions fail through one understandable interface.

## One gate, several check modes

`./scripts/check.sh` is the canonical check implementation. Every other entry point delegates to it.

| Profile | Task alias | Intended use | Includes |
|---|---|---|---|
| `fast` | `task check:fast` | Short local feedback loop | Formatting, architecture checks, capability/schema contracts, focused unit and contract tests, generated-site drift/type/link checks, and root plus Pages-base static builds |
| `full` | `task check` | Required implementation gate | Fast profile plus browser accessibility/interaction/responsive checks, vet, race, tidy/diff checks |
| `security` | `task security` | Security and dependency changes | Repository guard, module integrity, pinned static and vulnerability analysis, and the site runtime-resource/tracking/credential source guard |
| `release` | `task release:check` | Packaging and release changes | Artifact, metadata, checksum, Formula, and workflow contracts |
| `public` | `task public:check` | Public publication | Project metadata, forbidden-data, required-file, license, capability/schema contracts, public-boundary checks, generated references, and the deployable Pages artifact |
| `policy` | `task policy:test` | Fixed-evaluator feedback | Pinned OPA format check and unit tests |
| `gateway` | `task gateway:test` | Enforcement-point feedback | Exact classifier-admission inventory/evidence gate, source-built Gateway image with hash-locked dependencies, exact runtime-input snapshot membership/bytes, and the complete addon/parser test suite |
| `authbroker` | `task authbroker:test` | Research credential-boundary feedback | Exact runtime-input snapshot membership/bytes, strict broker/provider/root-key Go tests, Python daemon/vault/protocol tests in the pinned image environment, and Auth Broker image metadata |
| `upgrade` | `task upgrade:test` | Previous-development-release continuity gate | Exact predecessor release packaging, real persisted authority and Workspace state, candidate policy/Context mutations, repeated bare re-entry, truthful doctor/status output, Workspace replacement, and Context Home byte preservation on one explicit non-default Docker context |
| `integration` | `task integration:test` | Research real runtime boundary | The `task build:dev` three-service topology, kernel/network enforcement, Broker isolation, live Gateway/OPA transport and activation, Host Loopback, and resource lifecycle canaries |
| `runtime-release` | `task runtime:release` | Standard release container gate | Policy, Gateway, and cold release-surface first-use on one explicit non-default Docker context; explicitly excludes deferred Auth Broker and `task build:dev` integration tests |
| `runtime` | `task runtime:test` | Complete research container gate | Policy, Gateway, Auth Broker image/protocol, and research integration coverage |

The integration script reports named phase start/completion and elapsed time
for preflight, fixture build, Workspace Template/cluster, credentials/Workspaces,
Gateway/Broker/transport, live policy activation, attachment-scoped Host
Loopback, runtime failure boundaries, and lifecycle. It is deliberately not a
second semantic or presentation regression suite. `check-integration-scope.sh`
fixes the phase set, bounds script and CLI-invocation growth, rejects command
families owned by fast tests, verifies runtime-only canary markers, and proves
pull-request CI reaches integration exactly once through `runtime`.
Unexpected failures name the active phase before bounded container diagnostics,
while one cleanup owner still controls the complete fixture.

The base runtime check verifies its Git, HTTP, JSON, Python, SSH, GitHub CLI,
AWS CLI, Claude Code 2.1.220, and Codex 0.147.0 baseline. The fast profile binds
both agent pins to the single checked base artifact lock. Datadog acquisition tests
instead prove selected-Workspace Template resolution, immutable image and executable
identity, structural pup login/capture conformance, and absence of host/base
fallback. The base workflow is always validation-only. Release tests reject
every Tobari-owned GHCR reference, package-write permission, registry login, or
push path. Auth Broker remains validation-only and local to `task build:dev`.

`task build` is a contributor feedback path, not a release artifact. It
builds or reuses local Tobari-managed component images whose tags contain the
exact embedded source hash, prepares the source-addressed standard Runtime
image `tobari-runtime:base-<source-id>`, and compiles `bin/tobari` with the
release surface and development resolver. The `<source-id>` is the exact
checked embedded Runtime build-input identity, not a Git commit or OCI digest.
`task build:dev` produces `bin/tobari-research` with the research surface and
the same development resolver. The release surface excludes AWS
authentication and the Operator Console; the research surface adds both
without a runtime activation flag. Paired catalog tests derive the integrated
release set and prove that research adds exactly `auth login`, `auth import`,
`auth status`, `auth logout`, and `serve`. To run the integration script against
the research binary, set
`TOBARI_INTEGRATION_BINARY=$PWD/bin/tobari-research`; the default custom base
is the canonical name printed by `go run ./tools/runtimeassetid
standard-runtime-image`. `TOBARI_INTEGRATION_CUSTOM_BASE` may override that
default only with an already-existing compatible custom image.
Both paths generate a fresh synthetic TLS authority and build a run-local
Gateway trust wrapper. The explicit-binary path wraps the already verified
source-selected research development Gateway; the self-build path wraps
its temporary research base. Before publishing that wrapper under the
exact development-resolver tag, the harness records any pre-existing image
identity. Normal exit, failure, and interruption restore that exact identity,
or remove only the run-owned tag when no predecessor existed. Tag drift fails
closed rather than overwriting concurrent contributor state. Executable checks
prove the fresh authority is both byte-embedded and trusted by the wrapper.
The cold first-use integration scenario owns no Runtime prerequisite. It builds
a release-surface binary, starts before the XDG config/state/data parent
directories themselves exist and with absent exact standard Runtime/Gateway
tags, drives the bare `tobari` entry through its deterministic Manual setup
review, proves cold authority and immutable Runtime preparation, then
requires a real Workspace shell handoff and bounded `exit`. It repeats bare
entry to prove re-entry. Focused Runtime/Gateway tests, rather than this cold
shell script, enforce Workspace direct-egress denial. It then invokes the
same release binary from a descendant and proves
that the nearest containing root is offered and reused without new authority,
stops that exact parent test container, proves that explicit create-here from a
stopped ancestor produces a distinct nested Context/Workspace,
and verifies both attachments start at the invocation's descendant directory.
The same scenario applies session, source-access, selected-Runtime, and combined
revert transitions, re-enters from a descendant after each one, then leaves a
stale AppliedEntry and proves exact Workspace deletion, new-Workspace creation,
and byte-identical Context Home continuity across fresh CLI processes. Focused
crash tests cover interrupted-decision recovery, including an exact
prepared-settlement replay that restarts matching stopped Gateway/OPA
containers and a negative identity-drift case that proves no restart occurs.
Focused application, Store, Runtime, CLI, crash-replay, and hostile-state tests
cover task assistance agent selection, authority-bound attachment, one-Home
mount, retained draft, immutable submission review, Runtime source-only CAS,
policy-only validation, and Template Stage/Plan/Apply without requiring a provider account. Native Codex and
Claude sign-in compatibility remains a bounded manual observation rather than
an unattended release gate; the harness must not claim provider continuity it
does not execute.
The scenario refuses a Docker context that already
contains either exact tag and retains successfully built images as reusable
cache; it never masks, retags, or deletes an ambient image. An explicitly
selected custom base must already exist and is never rebuilt under an arbitrary
name.
`check-integration-scope.sh` mechanically guards these cold-start claims and
requires the release runtime CI profile to execute this scenario while keeping
the research Auth Broker integration deferred. CI runs the cold scenario and
the policy/Gateway component profile as parallel jobs; the aggregate local
`runtime-release` profile retains both.

The upgrade scenario is a separate release-blocking profile because cold
first-use and persisted-state continuity are different claims. It locates the
nearest reachable annotated development tag, exports that exact committed tree,
and invokes the predecessor tree's own release packager for the current host.
The predecessor release binary creates the isolated XDG authority, Docker
resources, Workspace, a byte-exact synthetic Claude settings sentinel, and one
real default-deny policy candidate. Only then does the harness switch to the
candidate release-surface binary. It verifies pre-Allow doctor candidate
wording, applies the candidate, requires bare entry to reuse the same Context
and Workspace, creates and deletes an unrelated Context, and requires another
bare entry. Finally it deletes and recreates the selected Workspace, requiring
a new Workspace identity and byte-identical Context-owned settings. A dev.22
binary used on both sides fails at the first post-Allow bare entry with
`workspace_entry_repair_required`; the candidate must pass the complete
timeline. Script-size and semantic marker guards keep this journey bounded.

Both repository build tasks embed only `git rev-parse --verify HEAD` as the
source commit while retaining the fixed `dev` version; `-buildvcs=false` and
`-trimpath` exclude implicit VCS state and local paths. Build-identity tests
fix version JSON schema 1, require `unknown` metadata to remain incompatible,
and compile release, development, research, and invalid-tuple matrices. Release
packaging uses the embedded resolver and exposes source-selected APIs with no
repository recovery value; the development fixture exposes matching source APIs plus exactly `task build`
and `bin/tobari` for the release surface, while research recovery is exactly
`task build:dev` and `bin/tobari-research`.

The standard contributor source expects `io.tobari.gateway-api=1`, including
guarded transparent routing, synthetic DNS, and schema-1 source principals.
The research build additionally checks `io.tobari.auth-broker-api=1`.
`task build` is the matching release-surface local image path. `versions.env` contains
only source inputs and no generated Tobari-owned release output. The release
surface rejects component locks and registry publication, and verifies every
CLI archive carries the requested source revision. Local image tags are derived
from their embedded recipes. Focused component tests retain an older exact
standard binding across a newer resolver; cover Workspace entry, status,
Configurator Freeze, and Template Plan/Apply with that owner-authoritative
binding; reject incompatible or drifting immutable image evidence; and prove a
historical binding never triggers reconstruction, pull, or silent rebinding.

The focused `scripts/check-runtime-base.sh` workflow step validates the canonical
`runtimes/base` metadata and consolidated per-platform artifact lock, the Dockerfile's common
tool, integrity, redistribution/license, and runtime contracts, and byte
equality with the embedded CLI snapshot. Its pull-request and main workflow is
cache-only and has no package-write permission. The protected Release workflow
cannot publish any Tobari-owned OCI image.

`scripts/check-gateway-source.sh` validates exact membership and byte equality for
the canonical Gateway Dockerfile, `.dockerignore`, and Dockerfile-declared
image inputs in the embedded snapshot. Tests and contributor documentation
remain canonical-only. `task gateway:test` runs the
Gateway unit suite against the canonical source, while the runtime integration
continues to exercise the embedded snapshot used by the CLI. The Gateway unit
contract fixes ordinary body-free, declared GraphQL/MCP-derived, and signed AWS wire-operation OPA documents
with trusted source-bound Context principal, explicit/transparent
authority parity, synthetic-destination replacement only after allow, zero
external DNS/upstream calls on denial or unsupported protocols,
null-versus-provider authorization
metadata, strict decision fields, authorization-before-forward ordering,
exact endpoint classification, bounded parser behavior, broker
deny-before-resolution, and secret redaction. The Gateway
image workflow builds both supported architectures without publishing. Runtime
tests preflight the immutable Gateway digest, labels,
entrypoint, default user, Docker Engine platform, and selected runtime base
before shared resources; local source image feedback is covered by the explicit
contributor `task build` path.

`scripts/check-authbroker-source.sh` validates exact membership and byte equality for the canonical Auth Broker
Dockerfile, `.dockerignore`, and Dockerfile-declared image inputs in the
embedded snapshot. Tests and contributor documentation remain canonical-only.
The canonical Python unit suite
runs in the pinned image environment and covers strict schema-1 control/runtime
protocols, locked startup, the schema-1 envelope/schema-1 static payload,
Context-bound handles, exact static introspection/resolution, restart
unlock, rotation, logout, retired-operation rejection, and secret-free
failures. The image check
validates provider-CLI absence, Docker labels, fixed entrypoint, and non-root
user. Its workflow builds both supported architectures without publishing.
`scripts/check-authbroker-image.sh` is the focused image metadata/artifact check, and
`task authbroker:source:sync` is the explicit maintainer operation that
refreshes the embedded snapshot.
The source/version contract digest-pins upstream image inputs and records
agent artifact identity in the consolidated base lock. Tobari-managed Gateway,
Auth Broker, and base outputs use source-derived local tags plus inspected
local digests; release checks reject any reintroduced component lock or
registry authority. The focused Gateway, Auth Broker, and base workflows
validate Linux amd64/arm64 construction with cache-only output.

Authentication coverage spans Go, Python, Gateway, image content, and
dependency checks. Reviewed host-driver tests fix argv/environment, canonical
executable identity/digest recheck, private state/PTY cleanup, bounded
browser/output behavior, cancellation, typed capture, and redacted errors. A
versioned synthetic Codex native-login fixture and answer key prove that the
child owns the loopback/browser flow, the dynamic authorization URL remains
visible with zero Tobari browser-open calls, and reviewed SGR regeneration,
`NO_COLOR` parity, and visible rejection of unknown controls remain bounded.
That fixture covers the research trusted-host acquisition driver. Separate
standard attached-session GitHub CLI fixtures use only synthetic GitHub.com
URLs and callback bytes and prove the strict native bridge without retaining a
token, state, code, account identity, or authenticated transcript.
The versioned synthetic Claude native-login fixture and answer key fragment the
exact OSC 8 line and no-newline paste prompt across writes. Tests prove that
the prompt is visible before flush, successful host opening omits the long URL,
failed opening retains it exactly once, the child owns non-echoing input,
later provider failure/status remains visible, and owned color is optional.
The fixture also proves Claude cursor state is not rendered as escaped text and
ends with exactly one Tobari-owned cursor-show. Native-state tests preserve
only structurally bounded subscription-type and rate-limit-tier labels, accept
other provider-owned additive metadata only at capture, prove that other
metadata is absent from the canonical Broker record, and continue to reject
duplicate keys, missing core or entitlement
values, malformed or duplicate scope tokens, grants outside the observed
request, refresh scope drift, and executable-identity drift while accepting
unknown future scope names and canonicalizing scope order. Progress tests prove fixed CRLF-framed secret-free
pre-prompt guidance and post-exit validation feedback, and prove the exact
input reader reaches the Docker runner unchanged.
Container argv tests require the fixed non-Workspace PID 1 and reject accidental
dependence on the CA-waiting Workspace entrypoint. They also distinguish default
Bash entry from positional-only direct argv, preserve order, duplicates,
dash-prefixed and explicit-empty arguments, reject a missing executable before
side effects, reject shell insertion, and preserve the direct child's exit
status. Separate
byte-level tests reject bare LF in fixed or pass-through Claude output while
the Docker TTY is raw. Fault tests fix deadline precedence across cleanup
failure and distinguish setup, authorization, output, native-state capture,
and cleanup-only failure.
Negative tests prove managed profiles, owner-selected dynamic plans, arbitrary
helpers, compatibility readers, and Broker provider CLIs are absent.

The canonical base and focused runtime checks validate the pinned client
artifacts at Claude Code 2.1.220, Codex 0.147.0, GitHub CLI 2.96.0, and AWS CLI 2.36.11. Local build fixtures
replace `/var/lib/tobari` with a temporary home mount and execute the client
commands, so an image-layer executable cannot silently depend on persistent
home state. These checks authorize neither redistribution nor publication;
the release workflow contains no base-image publication path.

Standard native-login tests execute the binary-owned opener and fixed Unix
socket agent, accept only exact schema-v1 requests, and reject duplicate keys,
unknown fields, malformed versions, oversized targets, neighboring URLs,
ownership mismatch, replay beyond the attachment budget, and opener failure.
One registry contract test fixes every reviewed driver ID and callback mode,
proves callers receive fresh compiled values, selects one fixture per entry,
and rejects empty, duplicate, missing-parser, mode-inconsistent, and ambiguous
registries.
Claude Code cases retain the exact pinned client, remote callback, PKCE shape,
and complete reviewed scope set. Codex, GitHub, AWS SSO, and pup callback cases retain their
validated non-privileged listener and opaque relay. GitHub CLI 2.96.0 keeps its
native Enter and invokes only the attachment-scoped opener for the strict device
target; TWG likewise retains its native confirmation and strict activation URL.
Pup 1.10.7 fixes the default-US1 authorization shape, seven mandatory query
fields, one optional UUID-shaped `dd_oid`, complete 110-scope ceiling, four
callback ports, bind-before-open ordering, and selected-Workspace opaque relay.
Shared closed-query-schema tests fix requiredness, singleton cardinality,
validator dispatch, and rejection of unknown fields or invalid definitions;
provider tests retain authority, scope, redirect, and callback semantics.
AWS CLI 2.36.11 fixes the commercial-region authorization-code shape, default
scope, bounded DCR/state/PKCE fields, dynamic callback port, bind-before-open
ordering, selected-Workspace opaque relay, and device-code recovery.
Runtime checks fix the exact-default-argv wrapper, pinned real executable,
fixed login/setup argv, opener mounts, and pass-through. The attached shell's
streams remain Docker-owned; the structured-color case uses only the reviewed
host-side PTY presentation relay, whose delayed-input test and distinct
polling/streaming raw-mode test enforce idle-safe keyboard forwarding and
terminal restoration; a separate canary proves literal `0x1d` pass-through,
while the direct stream path remains covered for all
other sessions.
validate each security-significant OAuth client/scope ceiling and loopback
callback schema, accept only each provider's reviewed syntax variation and a
validated dynamic non-privileged port,
and prove each callback flow's one host-loopback listener and browser open target the exact
label-verified selected Workspace. Synthetic callback canaries traverse the
fixed Docker exec relay program plus validated port without appearing in
output, logs, files, OPA, or Gateway evidence; invalid/replayed/ambiguous or
oversized output, a privileged/external callback, port collision, browser
failure, callback failure, and session exit close or avoid the listener.

Direct invocation is supported for automation:

```sh
./scripts/check.sh fast
./scripts/check.sh full
./scripts/check.sh security
./scripts/check.sh release
./scripts/check.sh public
./scripts/check.sh policy
./scripts/check.sh gateway
./scripts/check.sh authbroker
./scripts/check.sh first-use
./scripts/check.sh integration
./scripts/check.sh runtime
./scripts/check.sh runtime-release-components
./scripts/check.sh runtime-release
```

The `first-use`, `integration`, `runtime`, and `runtime-release` profiles require
`TOBARI_INTEGRATION_DOCKER_CONTEXT` (or an already exported non-default
`DOCKER_CONTEXT`). Aggregate profiles activate that exact context before any
component check so Policy, Gateway, and lifecycle evidence cannot split across
an ambient daemon and the selected integration daemon.

Every profile starts with a local-toolchain preflight after the gate sanitizes its Go environment. The preflight requires the exact Go version declared by `go.mod` under `GOTOOLCHAIN=local` and verifies the selected binary, its reported version, `GOVERSION`, `GOROOT`, `GOTOOLDIR`, and the compiler in that tool directory as one installation. A mismatch fails once with those values and remediation guidance before formatting, tests, downloads, or release builds begin.

The `fast`, `full`, `security`, and `public` profiles also require the exact
Node.js version in `.node-version` and npm version in the public site's
`packageManager` field. CI provisions both through one
repository-owned composite action; the canonical site gate alone installs the
locked dependencies and browser, and a fast workflow-ownership test rejects
workflow-local duplication or a missing Node bootstrap. Site installation uses
only the committed lockfile.
`task site:check` exposes the complete site gate directly; `task site:build`
produces and verifies the `/tobari/` Pages artifact, while `task site:dev`
serves the root-base development view. The full profile installs the pinned
Playwright Chromium build before testing representative pages with JavaScript
enabled, disabled, reduced motion, all three theme choices, keyboard controls,
and a 360 px viewport.

The site keeps canonical English routes at the root and reviewed Japanese
counterparts below `/ja/`. One source-derived parity check rejects a missing
Japanese counterpart, a Japanese-only orphan, or two files that map to the
same route. Paired content must remain substantive and preserve its heading
shape, commit-fixed evidence targets, and non-whitespace fenced machine
examples. Root and Pages-base artifact checks require both language routes, the
correct `html` language, and same-topic `en`, `ja`, and `x-default` alternate
links. Browser coverage exercises the same-topic language switch and localized
navigation without adding a runtime translation service.

Site source verification also rejects the retired universal
“question this page answers” opening in either locale. Concept, sequence,
guide, security, and reference pages begin with their own subject or task
instead of sharing one classroom-style preamble; end-of-page understanding
checks remain available where they support the learning path.

Public reference generation and claim links use the immutable product commit
recorded in `docs/architecture-site/source-snapshot.txt`; page-source
provenance separately identifies the documentation build commit. The static
gate rejects a missing or malformed snapshot, stale generated data, product
evidence that drifts to another commit, and a commit-fixed evidence path whose
blob/tree kind or existence does not match that snapshot.
Contract lint compares the product-contract schema inventory with the current
Catalog, but compares both architecture-site schema inventories with the
generated Catalog from that immutable product snapshot. An uncommitted product
change therefore cannot silently advance only the site's hand-authored claims.

The repository-shape pass skips local `node_modules/` installation trees in
the same way it skips generated `dist/` and `bin/` directories. Git still
enumerates every tracked or non-ignored repository path separately, so a
dependency directory accidentally added to publication scope does not bypass
path, link, locale, or secret checks.

All profiles require Git, Go, and `gofmt`. Container profiles additionally
require a reachable Docker Engine. The `release` profile additionally requires
ShellCheck 0.9.0 or newer, Ruby, `tar`, `unzip`, and either `sha256sum` or
`shasum`. Pinned Go security and action-lint tools must already exist in the
module cache or be downloadable over the network. The release preflight reports
missing system tools together before the long gate begins; network availability
is documented rather than actively probed because a network probe would be
nondeterministic and provider-specific.

The canonical gate and release packager force module mode and neutralize ambient Go workspace, toolchain, experiment, FIPS, and flag settings before invoking Go. This prevents a local or CI `GOFLAGS` value from silently selecting no tests and keeps agent, developer, and workflow evidence on the same checked command set. A release fixture launches the public profile with hostile values and proves that its first Go-backed check observes only the sanitized contract.

CI is the completion authority. Pull-request and main-push CI run `full`,
`security`, `public`, `runtime-release-components`, `first-use`, `upgrade`, and
`release` as seven independent parallel jobs, so the canonical full gate owns
the site build and browser tests exactly once while the two real-Docker entry
claims retain separate failure visibility.
One successful main-push CI run is the complete reusable automated source
evidence for its exact revision. Release preparation validates that workflow
path, event, branch, revision, completion, and conclusion while building the
actual five-target matrix in parallel; it does not invoke a source profile.
Its successful short-lived asset set is the only input accepted by protected
publication, which reverifies the preparation run, assembly job, artifact,
invocation identity, tag binding, and final inventory without rebuilding. The Pages workflow
runs the same canonical site gate only for a push to `main` or a manual replay;
only a successful push to `main` may upload `dist/` and enter the least-
privilege Pages deploy job. The repository installs no
automatic Codex Stop hook: a per-turn gate adds latency and does not prove
completion. Optional local automation must delegate to one named profile and
must not claim equivalence to a profile it did not run.

The repository retains seven workflow files, each with one distinct owner:

| Workflow | Trigger | Owned outcome |
|---|---|---|
| `ci.yml` | Pull request and main push | Exact-revision completion and release evidence for all seven source profiles |
| `architecture-pages.yml` | Main push and manual replay | Canonical site verification and main-only Pages deployment |
| `security.yml` | Weekly schedule and manual replay | Periodic dependency and source-security drift detection |
| `release.yml` | Protected manual dispatch | Reuse exact successful CI evidence, package CLI archives, and publish reviewed release assets |
| `gateway-image.yml` | Relevant pull request and reviewed manual revision | Cache-only multi-architecture Gateway source/image validation |
| `authbroker-image.yml` | Relevant pull request and reviewed manual revision | Cache-only research Auth Broker source/image validation |
| `runtime-base.yml` | Relevant pull request and main push | Cache-only multi-architecture validation of the one agent-ready base |

There are no per-agent Runtime workflows: Claude Code and Codex are inputs to
the base lock and build. Taskfile exposes completion profiles, focused product
profiles, contributor build/site operations, and explicit snapshot sync
operations. Implementation-only source checks and raw Go test variants remain
owned by `scripts/check.sh` rather than receiving duplicate task aliases.

## Adoption is a product check

The harness validates more than an intact security boundary. It also checks
whether the boundary is usable enough to be selected for ordinary autonomous
work. The agent-readiness scenario records the current first-use path and the
denial-to-retry path, including discovery rounds and undeclared external
processing. A passing policy test with a workflow that requires users to become
Docker or OPA operators is evidence for a thesis revision or follow-up UX
slice, not evidence that the product outcome is complete.

The desired routine journey remains CWD-first: on fresh first use complete the
deterministic Manual review and enter a reusable Workspace; create a concrete
Runtime source or accumulate reviewed Policy Memory evidence; optionally invoke
the corresponding task-scoped agent assistance; review only that source through
the trusted host; work freely within the declared root and network boundary;
receive a useful
secret-free denial, approve the minimum exact or reviewed single-segment
template permission through a trusted host
action, and retry. Human presentation may simplify this journey only while the
catalog, opaque-reference, effect, mutation, and fail-closed contracts remain
unchanged.
The first-entry evaluator retains its seven rows for deterministic fresh first use; stopped
Engine or cluster; interrupted or partial authority; explicit entry and retry;
truthful progress and Next; idempotent convergence; and final repository
integration. Separate assistance domain, application, Store, Runtime, and CLI
contract tests cover Runtime and policy selectors, pre-mutation handoff
cancellation, Context execution-Runtime binding, host-owned launch facts, draft
retention, direct-egress/mount isolation, exact Home-path agreement, shared
attachment exclusion, frozen target submission, generated task/language
guidance, the exact fixed initial positional prompt for both pinned agents, and
final-review non-authority. Recovery tests separately prove durable exact Apply
confirmation, no agent restart for a confirmed Runtime no-op, authority-fenced
Runtime/Template publication verification, policy Stage precedence, caller-
cancellation-independent settlement including cancellation before Invoker
admission, exact durable-Plan Apply recovery without agent/review/re-Plan, and
confirmed settlement-incomplete classification. Stage-lease lifetime and
Context-binding predicate tests keep
both unconfirmed and confirmed policy tasks protected from Context deletion
until Stage settlement. Context lifecycle tests prove that deleting an
unpublished draft closes its public reference flow without Docker or aggregate
mutation, and that deleting an otherwise-empty selected active Context
atomically removes it and clears the selector,
while Workspace, attachment, credential, and Stage references remain blockers.
They also prove that a fresh Docker-readiness failure precedes the durable
Context-deletion decision with no authority change, while an existing exact
partial decision bypasses fresh readiness and remains recoverable.
Fresh-root composition tests independently prove
that Manual review composes the canonical final default pair, final cluster,
Workspace entry, and child handoff without Configurator calls.
Negative fixtures prove invalid root/direct-child rejection, non-TTY zero
setup, cancellation/EOF/render failure zero later calls, concurrent authority
drift rejection, confirmed Template/Context retention after cluster failure,
missing custom Runtime-material refusal, canonical standard materialization,
and retry without a second Runtime choice.
Existing-Workspace tests additionally fix entry-specific progress labels,
zero-stage exact-current direct handoff after coherent read-only entry
classification that excludes volatile attachment status, mutation-free
current-only entry including byte-exact preservation of recovery residue,
exact-missing standard-image and stopped-protection fallback
to the canonical staged recovery flow,
declared exclusive-Home busy faults, shared same-Context Workspace leases,
exclusive Configurator and retirement exclusion, per-borrower global-session
lease lifetime when the first owner closes first, no entry-versus-settlement
self-conflict, read-only exact-current confirmation, owner-first borrower
exclusion of runtime repair before a durable decision, production-shaped root
and direct-entry busy faults, root/cluster Gateway-build faults, and Windows compilation of
the shared-lock boundary. A maintainer may additionally replay root,
descendant, current Context selection, concurrent direct-command, owner-first close,
borrower continuation, and deletion drain against an isolated Docker Context;
that observation supplements rather than replaces the repository gates.

The same semantic fixtures drive line-oriented stderr, failure, cancellation,
status, help, narrow/NO_COLOR projections, and agent-readiness answers. They fix
the five stage names, seven stage states, checkpoint-local terminal markers,
250 ms anti-flicker, one-second elapsed, ten-second bounded wait reason, and
30-second maximum redirected heartbeat frequency. JSON-error fixtures prove
readiness and later checkpoint failures produce one parseable stderr document
with no human progress prefix or suffix. They prove external text
cannot drive progress and that no persistent progress authority, percentage,
ETA, raw log, or details-toggle interface exists.

Production-shaped cluster fixtures publish activation receipts A→B and prove
the same invocation refreshes exact collection/activation authority before
Workspace entry; post-cluster drift or read failure stops entry while retaining
a non-none mutation classification. Cancellation fixtures cover each
pre-handoff boundary and bounded classification/exit 130. Child fixtures prove
exact argv/streams/status and secondary cleanup guidance.

The readiness fixture proves that the closed generic Docker
CLI/Engine/selected-context/Compose profile runs after root review and before
the first mutation, and also before direct `cluster up` reconciliation. It
accepts Engine 24, rejects Engine 23 or malformed versions, and performs no
mutation on failure. Recording runners reject Docker mutations,
provider executable names, process managers, application openers, socket probes,
and any argv outside the fixed read set.

Auth Broker readiness is split deliberately. The reproducible synthetic
authentication proof remains available through `task integration:test` for
explicit research validation, but Auth Broker and `task build:dev` integration
are deferred and are not part of the standard release CI profile. It uses
synthetic credentials, mocked host GitHub CLI results, and local HTTP fixtures
when run. The research `auth` namespace and Broker are absent from standard
release archives, so live reviewed-provider acquisition is not a standard
release gate. Maintainers may replay the research profiles as a compatibility
observation; such a replay records only pass/fail and secret-free outcomes and
never becomes a release fixture.

## Harness components

### `.harness/project.json`

This V1 file is the machine-readable source for Tobari identity,
release metadata, and repository policy. Its schema contains no lifecycle
profile: repository readiness is established by the named verification gates,
not by a stored state label.

`binary_name` is a portable lowercase executable basename of at most 96 bytes, leaving room for the mandatory Windows `.exe` suffix under the 100-byte cross-format archive-entry limit. Validation rejects the case-insensitive Windows device names `CON`, `AUX`, `PRN`, `NUL`, `COM1` through `COM9`, and `LPT1` through `LPT9`; adding `.exe` does not make those names extractable on Windows. It also rejects `LICENSE` case-insensitively because every release archive reserves that entry name. These are parts of the default cross-format release-matrix contract, not naming-style preferences.

Policy that must be reviewed by both humans and tools belongs here when it is finite and structural, such as forbidden private identifiers or expected module and binary names. Product reasoning remains in documentation.

`public_guard.documentation_locale` is one explicit BCP-47-like language tag
for the intended locale of trusted repository documentation and CLI-authored
prose. Tobari uses `en`; the loader applies no default.

Repository guard mechanically enforces only a narrow English/Japanese canary:
when the tag is English, Japanese script is rejected in trusted Markdown prose.
Fenced code, bounded inline code spans, block quotes, parsed inline/reference
Markdown link destinations, historical `Complete` or `Superseded` work packets,
non-Markdown external fixtures, CLI-authored Go strings, other scripts, and
non-English locales are not linguistically classified. Blank lines, quotes, and
fences bound inline parsing; malformed or escaped link-like text remains prose.
Link labels and `Draft`, `Accepted`, or `Active` work-packet prose remain trusted
documentation and are checked. Stable machine identifiers and external provider
data are never translated by this setting. Both `en` and the valid three-letter
`eng` language tag activate the English canary.

### `tools/archlint`

Architecture lint checks production dependency direction, rejects unclassified
production packages, and keeps each `cmd/` entrypoint limited to
argument/stream handoff, signal cancellation, the CLI composition root, and
process exit. It merges Go package information for the native build and every
release target on Linux, macOS, and Windows, so a platform-specific file cannot
hide a forbidden dependency from the host CI platform. Each `go list -json`
process is decoded from stdout only; stderr remains a separate diagnostic
channel and cannot corrupt the package stream. Source checks reject detached
application, infrastructure, and CLI contexts, default HTTP clients,
application-layer `fmt` presentation/scanning calls, built-in `print`/`println`
in domain, application, CLI, and command packages, and command-entrypoint
access outside the narrow selector allowlist. Domain and application packages
cannot import `log`, `log/slog`, or Cgo. Reviewed user-facing presentation
belongs in CLI and must use its injected streams. Any allowed exception must be
narrow, named, and tested.

CLI source inspection also rejects ANSI SGR color or emphasis literals outside
the single shared semantic-style file. It deliberately permits non-style
terminal controls such as bounded selector cursor movement. A negative fixture
adds a direct ANSI style literal to an ordinary CLI renderer and proves the
lint reports the bypass.

Tobari rejects every third-party import from `cmd` and `internal/cli` by
default. Reviewed effectful dependencies belong in `internal/infra`. A
presentation-only exception requires an accepted ADR or thesis consequence,
license and dependency review, one exact package path in
`allowedCLIThirdPartyImports`, and a regression test proving sibling paths and
effectful packages remain rejected. Wildcards and prefix allowlists are not
valid exceptions.

### `tools/repoguard`

Repository guard checks public-boundary and repository-shape policy, including
forbidden identifiers, unresolved placeholders, likely secrets, work-packet
lifecycle consistency, required public files, and the configured documentation
locale. The production-source retirement guard also rejects user-policy Rego
paths and retired policy-mode/source identifiers outside the fixed embedded
evaluator and bounded raw legacy-marker allowlists. Its English-locale check is
the narrow trusted-Markdown Japanese-script
canary described above, not general language detection. Its publishable path
set comes from successful Git enumeration. Git errors, symbolic links, special
files, and other inspection errors fail closed.

Work-goal status is one of `Draft`, `Accepted`, `Active`, `Complete`, or
`Superseded`. `Accepted` remains a valid pre-execution state for existing
histories; new work may move directly from Draft to Active. Complete
requires every acceptance checkbox in every visible Acceptance section and
every task checkbox to be checked across the standard GFM unordered and ordered
list markers. A terminal `Complete` packet must not declare
`Retention: temporary`; it is removed after its conclusions are promoted.
`Retention: evidence` is a narrow exception and must name why the evidence
cannot be replaced and when it may be deleted. Metadata is read only from the
contiguous top-level
`- Key: value` block directly below the first top-level ATX H1 (`# ...`).
Fenced examples and HTML comments do not supply metadata, headings, or
checkboxes; valid top-level and list-container CommonMark fences are
recognized. A Superseded goal names one canonical raw relative path to a non-template
repository goal, and its successor chain must terminate rather than cycle. The
guard reads each goal and successor through the same regular-file/no-symlink
repository boundary. Maintainers must review an inconsistent historical
Complete packet and either supply its evidence, return it to Active, or
supersede it explicitly. A migration must not check boxes automatically.

### `tools/contractlint`

Contract lint validates the executable catalog before checking two repository ledgers:

- [`.harness/capabilities.json`](../.harness/capabilities.json) records supported and deliberately unsupported user capabilities without copying command paths. Each public capability ID must appear in at least one `AgentContract.CapabilityID`, every catalog capability must be public, and an `internal`, `deferred`, or `excluded` entry must remain absent from the catalog and explain why.
- [`.harness/schemas.json`](../.harness/schemas.json) pins publishable external-schema and conformance fixtures by repository-relative path and exact SHA-256 digest. Each entry also records provenance and license; a corpus may additionally pin its exact byte and case counts. An explicit empty array is valid before the project adopts an external schema or conformance corpus.

Both ledgers are strict JSON and must themselves be regular files reached without symbolic links. Unknown or duplicate object keys, duplicate IDs, malformed lowercase dot IDs, trailing values, and implicit `null` lists fail. Capability command paths remain owned only by the catalog; adding them to the ledger creates forbidden duplication rather than useful documentation.

Capability status has a narrow meaning:

| Status | Meaning |
|---|---|
| `public` | At least one catalog command exposes this supported user capability |
| `internal` | The implementation may use it, but no public command may expose it |
| `deferred` | The product may add it later, but it is unsupported now |
| `excluded` | The current product contract deliberately does not support it |

Several commands may share one public capability ID when discover and act commands form one user workflow. Conversely, one command declares exactly one primary capability; splitting a command across unrelated outcomes is a product-design signal, not a ledger shortcut. Non-public entries require a reason so an agent does not mistake absence for an implementation gap.

Schema paths must be canonical repository-relative paths below a `testdata` directory. Every path component is inspected without following symbolic links, and the target must be a regular file. A digest mismatch requires reviewing the upstream change and updating the manifest deliberately; the tool never rewrites a digest. `repoguard public` separately checks the same fixture content for public-repository policy, so a matching digest is not permission to publish a secret or unlicensed material.

The current ledger pins `auth-provider.v1` to the repository-authored synthetic
provider manifest under `internal/infra/authproviders/testdata`. Its MIT
provenance and exact digest make schema drift reviewable; parser, normalization,
Gateway, and Workspace projection tests still determine semantic compatibility.
The fixture contains no real provider account or credential.

Run the focused check with:

```sh
task contracts:check
```

The same tool runs in `fast`, therefore in `full`, and directly in `public`. There is no CI-only capability or schema interpretation.

When adding an external API, first record every considered user capability in the capability ledger, including deliberately deferred and excluded outcomes. Promote an ID to `public` only in the same change that adds a validated catalog contract. When vendoring an upstream schema or response fixture, record its source and publication license, compute the digest from the exact bytes, and add adapter contract tests. A schema digest proves identity, not compatibility: tests must still fail when a reviewed upstream change violates the domain mapping.

### Tests

The test suite has complementary levels:

- Domain tests fix pure invariants.
- Application tests fix task interpretation, orchestration, and ambiguity behavior,
  including nearest-first Workspace candidate snapshots, explicit create/use
  choices, stale-choice rejection under the lifecycle lock, and zero mutation
  calls on cancellation or invalid selection.
- Each interpretation-sensitive capability adds task-owned semantic-result
  tests for its declared task identity and the target, parent, and/or scope
  dimensions it actually carries. The tests preserve scoped empty collections
  and interpretation-relevant state distinctions, reject field/reference-kind
  laundering where multiple kinds exist, and add negative-inference canaries
  where display details could be mistaken for facts.
- Pagination and mutation-boundary tests prove rejection/cancellation before
  downstream calls, complete runtime-fault declarations, and
  complete-or-no-result behavior.
- Catalog output tests validate `complete|paged` delivery independently from
  `not_applicable|exhaustive|bounded_window|differential_window` collection
  coverage. Pagination tests require an exact optional-input/top-level-string
  opaque cursor binding, typed empty-cursor completion, and JSON-only
  presentation for paged delivery, forbid that binding for complete delivery,
  and reject paged plus `not_applicable`. Renderer fixtures reject an omitted,
  null, or non-string cursor.
- Infrastructure tests fix protocol conversion and boundary failure.
- A catalog-derived first-use canary executes every public `EffectRead` handler
  with a separate fresh XDG tree, compares the complete owned tree before and
  after, and rejects Docker mutation argv. Associated fixtures cover read-only
  XDG directories, concurrent reads, synthetic absence, unsupported-version
  and corrupt-state fail-closed behavior,
  and the sole pre-existing-journal cleanup exception without record loss.
- Completion tests derive command and flag candidates from the public catalog,
  validate partial command words such as `cont`, exercise finite values,
  conflicts, directory delegation, global and command-local Workspace Template selection,
  Runtime names and ready revision selectors, and reject malformed or
  structurally unsafe bounded requests. The generated zsh adapter is fixed as a
  live candidate client with no embedded command inventory.
- CLI tests fix routing, help, rendering, exit behavior, the catalog-owned typed
  argv parser, and the distinction among absent, defaulted, and explicitly
  supplied values. Negative fixtures cover type/range/enumeration,
  repeatability, dependency/conflict, duplicate scalar, and syntax drift. The
  exact six semantic style tokens are tested with styling enabled and disabled;
  `NO_COLOR` and non-TTY tests reject ANSI style sequences, while marker and
  wording canaries prove status remains distinguishable without color. A
  ledger-pinned typed fixture and presentation-independent answer key drive
  lifecycle, scoped-empty, warning, failure, and pre-action-cancel renderers
  through colored TTY, `NO_COLOR` TTY, and redirected text. Their checks fix
  required facts, exact Next argv, unsupported-inference canaries, one task
  invocation, and zero external processing. Catalog
  tests additionally fix lifecycle collection counts, human-anchor-before-ref
  card ordering, explicit empty collections, command-success versus resource-
  lifecycle separation, and primary Next versus subordinate manual paths.
  validation rejects every text-producing command that omits the shared
  semantic-token presentation declaration. The
  deterministic CLI-experience evaluator additionally fixes the terminal-owned
  named palette, reviewed marker vocabulary, shared spinner ownership and
  timing, source-level ANSI ownership, color-independent semantic document,
  first-use raw interaction continuity, main public-handler reachability, and
  absence of predecessor presentation owners. Violations are sorted by
  rule/target/detail so the gate is stable and reviewable. The
  Workspace selector tests cover the dependency-free raw key state machine,
  bounded scrolling, no redraw during repeated idle VTIME polls, exactly-once
  inline column-zero cursor restoration, negative DEC-1049 canaries, a small
  screen-state model for bottom-edge scroll plus wrapping, a resize no-old-
  origin control-sequence test, unknown-size append-only fallback,
  conservative Unicode width, wrapped-row
  pre-reservation and saved-origin/below-origin repaint, one-owner fail-closed finish,
  final-frame scrollback retention, English status rendering, and ANSI-free numbered-input
  fallback. Catalog routing tests cover bare canonical namespace help plus no
  more than three deterministic, exact-selector typo suggestions. Interactive
  entry tests preserve the Bash or exact direct-command
  child exit status, assert that the Workspace remains logically existing
  after the child returns, and keep the resume/delete summary on host stderr
  rather than child stdout.
- Attached-session presentation tests use synthetic PTYs to prove that bounded
  JSON/YAML stdout receives only fixed SGR token wrappers, visible bytes remain
  identical, fragmented and pending candidates flush safely, and ordinary,
  invalid, oversized, control-bearing, tagged, anchored, aliased, or escaped-
  control data follows the pass-through rule. They separately prove stderr
  remains uncolored, `NO_COLOR` is presence-only, non-TTY output bypasses the
  relay, the child sees the initial window size, and the relay's writer and
  exit failures restore the host terminal.
- Agent-help shape, edge-equivalence, and derived-scale size tests keep root
  discovery index-only while grouped scoped workflows retain the complete
  invocation, reference, and recovery contract without producer/consumer
  Cartesian growth.
- JSON-output contract tests compare every built-in renderer's exact schema
  version, envelope, nested object/array shape, required and optional keys,
  scalar types, enums, reference declarations, and nullability with its catalog
  `CommandOutput` declaration. Runtime rendering repeats that recursive check
  before stdout, and negative tests cover missing/extra children, wrong type,
  wrong enum, invalid null, excessive depth/count, empty undeclared sentinels,
  and leaked internal commands. Paged probes additionally enforce the one
  declared always-present string cursor and its typed `empty_cursor` sentinel.
  Auth additions specifically pin cluster status schema 1 with nullable
  unconfigured resources, Workspace Template report
  schema 1, Workspace Template list schema 1, Workspace status schema 1, and auth
  result/status schema 1, including explicit Workspace Template persistence state, null
  pre-authority IDs/stores, the complete shell
  environment inventory, atomic Git identity policy, explicit empty literal
  values, explicit empty provider collections, and null account labels.
  Help's catalog fields describe root `view: index`; separate exact-key tests
  cover both that view and the input-selected `view: scope` variant. The
  ledger-backed machine-output fixture and presentation-independent answer key
  preserve empty collections, explicit null, false, zero, unavailable states,
  nested arrays, structured recovery, and a routine-success external-processing
  count of zero.
- Domain-owned finite strings that cross a Catalog output field are tested as
  one canonical ordered set at every exact field path. Constructor-built
  semantic fixtures still cover correlations that set equality cannot prove:
  exact and single-segment-template Allows, exact Denies with an explicit empty
  examples collection, empty exhaustive reports, nested reviewed receipts, and
  protocol-specific fields with explicit empty siblings. The caller receives a
  fresh value slice; ordering stabilizes help and generated output but is not
  identity or authority. CLI-only format, delivery, and presentation enums stay
  outside this rule.
- Produced-reference tests force top-level, nested-object, scalar-array-item,
  and object-inside-array shapes through the one bounded `OutputField`
  traversal. They compare deterministic canonical kind/path pairs with Catalog
  role validation, required-producer reachability, scoped agent help, and
  workflows, while duplicate paths, cursor collisions, invalid nested types,
  excessive depth/count, and closed reference cycles fail Catalog validation.
  Runtime, Runtime-revision, and prune-plan schemas use that same generic
  interface without a Runtime-specific walker; their producer/consumer graph,
  human projection, and exact opaque-byte round trips are covered directly.
- Auth truth-table tests freeze current, missing, stale, unavailable,
  unresolved, zero-Workspace, configured-with-current-projection, changed, and
  no-change states independently of presentation. Infrastructure tests prove
  exact Context/revision/binding correlation, deterministic bounded
  collections, zero Broker calls after a collection/call-policy bound is
  exceeded, and preservation of a confirmed receipt across post-success
  cancellation. CLI tests pin configured-provider rotation warning, exact
  working-directory plus argv actions only for justified rows, neutral
  selector cancellation, and absence of removal/revocation/re-entry claims for
  no-op logout.
- Adversarial output tests keep TSV/JSON records and stdout/stderr ownership intact across controls, Unicode format/line separators, existing backslashes, and printable prompt-like data while preserving opaque IDs exactly.
- Catalog tests scan every public command for completeness and unique paths.
- Catalog syntax tests reject command/namespace prefix collisions,
  bracket/`a|b`/exact-literal usage drift from `Required`/`AllowedValues`,
  fault-code signature conflicts across command and agent-help global errors,
  and missing common runtime failure declarations.
- Reference-graph tests connect discover producers to act consumers by kind and exact field/argument declarations.
- Opaque-ID round-trip tests pass discovery output unchanged into action input.
- CWD-owned lifecycle integration creates same-root/different-Template Workspaces
  for location-free Contexts and independent-root Workspaces, then proves their actual containers, networks,
  and XDG homes are distinct while Gateway, OPA, and public CA state are
  shared. It retains one partial container/network cleanup followed by exact
  public deletion, principal-registry cleanup, cluster-down refusal while
  Workspaces remain, and final owned-resource purge. Ancestor selection,
  repeated/concurrent creation, drift reconciliation, child exit semantics,
  and attachment guards are owned by deterministic domain, application,
  infrastructure, and CLI tests. Mount-guard fixtures additionally prove that
  a live read-write strict ancestor blocks a new or stopped descendant before
  start, while same-root, descendant, read-only/stopped ancestor, and an
  already-running exact target remain admissible; malformed, foreign,
  duplicate, oversized, or contradictory Docker evidence fails closed and a
  newly created blocked target is removed.
- Lifecycle target-safety tests prove root, status, and delete expose no
  Template/Context selector; bind destructive actions through exact opaque
  references where required; keep root selection CWD-first; and prove
  same-root/different-Template Contexts remain siblings. Frozen typed status,
  answer-key, and text fixtures verify schema-3 independent axes, structured
  Next/Attention, exact human Runtime selection, suppression of healthy
  diagnostic IDs/home, and the complete delete impact.
- Logical lifecycle tests inject interruptions at home, instance, root-index,
  runtime, and deletion boundaries; they prove journals recover without
  duplicate IDs and diagnose orphaned one-sided records. Runtime interruption
  coverage additionally cancels a fully matching re-entry during guard
  validation and proves its existing principal was never removed or followed
  by a Docker mutation. Shared-state tests
  prove locked atomic writes, interrupted cluster reconcile diagnosis, and
  explicit cluster network reconnects. Cluster-observation tests additionally
  detach each required Gateway/OPA shared join, detach a registered project
  endpoint, and remove a live binding; each drift is read-only detected before
  root readiness can skip reconciliation. Cleanup tests interrupt Compose down,
  retain the exact journal, and resume to absent state without manual repair.
- Custom-image tests build from the stable local Tobari base, include a fixture
  with a terminating image `CMD`, select it through bounded configuration, and
  prove the image `CMD` cannot own Workspace lifetime. They also prove
  compatibility is checked before per-Workspace resources exist.
- Runtime-spec tests assert fixed CPU, memory, PID-count, and container-log
  options and the Tobari-owned lifetime command, include that contract in drift
  hashing, and the Docker integration scenario inspects one live instance per
  relevant runtime shape.
- Workspace Template Runtime tests cover exact ID+revision selection, built-in
  initialization, ready-revision enforcement, explicit upgrade/rollback, and
  the fact that project metadata cannot override the bound Workspace Template Runtime.
- Shared Runtime tests cover non-overwriting source initialization, complete
  bounded regular-file snapshots, the 1,024-file/256-directory/32-MiB-file/
  64-MiB-total limits, fixed-buffer stream copy plus digest identity, generated
  image naming, compatibility and digest inspection, cold managed build before
  any Workspace preparation, exact canonical parent preparation before the
  managed attempt, bounded parent diagnostics, cancellation-safe pre-attempt
  journal cleanup, semantic no-op builds,
  failed-build history preservation, and zero Workspace Template writes. Application and
  CLI tests prove source validation retains a reviewed relative path and
  actual/limit or owner-only correction while stripping private causes and
  making zero Docker calls.
- Managed Runtime lifecycle tests cover complete current/retained Template and
  Workspace applied/pending/observed protection, strict authority-directory and
  immutable-snapshot validation, bounded task-scoped Docker observation,
  canonical selector and content-ID correlation, shared/foreign container and
  tag preservation, exact-reference same-name races, and zero-create reads.
  Journal fixtures interrupt build, restore, prune, and whole deletion at each
  supported filesystem/Docker boundary and prove one-confirmation recovery,
  production-composition TTY entry, failed-draft recovery without fabricated
  successful history, active-journal causal classification,
  same-plan/idempotent replay, terminal receipts, restore supersession before
  journal cleanup, prune→restore→external-disappearance `missing` semantics,
  new-plan generation after restore, and no cross-Runtime mutation.
  Catalog and presentation tests fix `runtime`, `runtime-revision`, and
  `runtime-prune-plan` reachability, exact mutation intent/target/impact/fault
  contracts, exhaustive dry-run uncertainty, and the absence of revision
  deletion, broad garbage collection, raw Docker authority, and false refs.
  Public-projection fixtures derive list/show/history/build and redirected
  review from one coherent snapshot, keep `ready=true` distinct from a pruned
  head, require `source_digest` plus nullable semantic lifecycle evidence, and
  reject `revision`, `image`, `image_digest`, `snapshot_path`, and their exact
  underlying values in JSON and human output. Confirmed post-effect
  cancellation or authority drift remains verification/confirmed in both the
  application fault and Catalog declaration.
- Runtime source, snapshot, build, history append, and explicit Workspace Template binding
  are owned by focused domain, application, infrastructure, and CLI contract
  tests. CLI tests additionally cover interactive text Review, managed-only
  build selection, complete ready-revision rollback selection, exact shown-
  Workspace Template binding, separate unchanged editing and old-to-new Review states,
  Back-to-list zero mutation, direct-mode bypass, line-mode fallback,
  cancellation, and non-TTY/JSON zero-mutation rejection. They do not repeat
  inside the general Docker integration scenario.
- Policy-learning domain/application/CLI tests own baseline-versus-learnable
  classification and the allow, deny, reset, re-review, template, and output
  matrices. Docker integration retains one opaque-reference allow followed by
  a retry through the same Workspace and stable OPA process to prove live
  watched-bundle composition.
- Policy-candidate domain and CLI tests fold repeated and concurrently emitted
  exact denials into one pending item, retain the latest evidence, count the
  required bounded observations, keep
  Context/scheme/host/port/method/path differences separate, and exclude resolved
  Allow and exact Deny decisions.
- Gateway contract tests verify that a learnable denial carries only the fixed
  host-side review navigation, while non-learnable and infrastructure failures
  do not invite approval. Session lifecycle tests verify that the aggregate
  pending-permission summary stays on host stderr and is best-effort.
- Workspace Template contract tests verify stable Template IDs, owner-only
  separated Template/Context/Policy-Memory boundaries, default selection,
  immutable Context binding, exact V1 records, and unsupported-version
  rejection. Runtime tests verify aggregate Template baseline plus Context
  Policy Memory reaches one OPA, the bound agent profile reaches the Workspace
  as read-only data, and Context vaults remain Gateway-inaccessible except
  through the Broker socket. Aggregate tests cover namespace
  reservation, secret-sensitive revisions, complete-candidate validation,
  serialization, and known-good retention.
- Auth domain and catalog tests fix exact schema-1 static providers and the
  standard GitHub/Datadog/OpenAI/Anthropic login union, exact command
  effects/inputs/outputs/failures, bounded interactive provider omission, and
  rejection of AWS before acquisition. A targeted `tobari_research`
  matrix adds AWS and its `identity-center|console` method axis while proving
  the release/default profile remains standard. They also cover Context binding, cancellation,
  redirected zero-mutation behavior, exhaustive Context-scoped status,
  explicit locked/unavailable state, non-terminal stdin-only import and terminal
  refusal before reading, read-after-public-validation/send-after-runtime-
  prerequisite ordering, complete auth fault inventory, public
  `macos_keychain|xdg_file` backend enums versus the internal
  `linux_xdg_file` doctor label, secret-free rendering, and exact Context-wide
  eligibility/next-entry/logout guidance. Provider loader tests reject unsafe
  ownership, symlinks, unknown fields, collisions, built-in overrides, and
  helper-backed user manifests, including
  `ambiguous_provider_http_binding` for overlapping exact HTTP recognition.
- Root-key tests cover the fixed macOS Keychain service/account and stdin-only
  update shape, Linux owner-only XDG file creation, unsafe path/mode/symlink
  rejection, and the rule that a missing key is never regenerated beside an
  encrypted vault.
- Auth Broker tests cover exact 64 KiB schema-1 framing, bounded credential
  payloads, locked health, AES-256-GCM Context binding, schema-1 typed state,
  atomic vault writes,
  SHA-256-only live handle lookup, deterministic handle reuse for one revision,
  cross-Context/binding rejection, rotation, logout, restart/unlock
  rehydration, same-revision static replacement, bounded AWS signing,
  Datadog/OpenAI/Anthropic refresh through the exact immutable renewable-adapter union,
  shared single-flight/snapshot/CAS lifecycle, durable operation barriers,
  adapter incapability over Vault/handles/locks, the exact immutable persisted
  record-contract union, record-contract incapability over filesystem/ciphers,
  compatibility re-exports, exact immutable control-login plans, shared
  static/driver parsing, plan incapability over Broker/Vault/host execution,
  the exact immutable Gateway reviewed-profile union, Gateway-profile
  incapability over requests/policy/Broker/secrets, and rejection of every
  unknown record/operation. Provider network tests use local synthetic servers.
- Acquisition UX tests split trusted-host, isolated Workspace Template-runtime, and Broker
  boundaries for GitHub/AWS/pup/Codex/Claude: canonical executable/digest
  selection, fixed argv, sanitized state, bounded streams/capture, checked
  cleanup, and cancellation preserve the prior credential on failure. Claude
  tests additionally prove a fresh mount-free selected-image container, exact
  native authorization URL projection/opening, and strict renewable-state
  capture. No provider CLI
  exists in the Broker image.
- Workspace auth projection tests prove one configured Context credential
  produces Context/Workspace-bound handles, injects only declared environment
  or complete-file data, refuses to overwrite unowned/modified/symlinked files,
  removes only unchanged Tobari-owned files, and includes changed handle state
  in next-entry work-container reconciliation without deleting the Workspace
  home. Logout tests prove immediate revocation and next-entry environment/file
  removal; down and purge tests prove vault/root-key preservation.
- Brokered Gateway tests prove broker-required declared header/signing
  bindings, zero-call direct-credential rejection, undeclared-binding
  compatibility passthrough, exact target/header recognition, handle removal
  before OPA, schema-1 provider metadata, zero resolve calls on deny, one
  same-revision resolve after allow, exact destination replacement, secret
  redaction, query/header omission and whole-path handle redaction in audit,
  copied/stale/malformed/misplaced/ambiguous/mismatched handle rejection,
  non-learnable structural denials, broker-unavailable failure, and fallback
  only when no declared binding and no inspected URL/header position contains a
  Tobari marker.
  Dynamic cases prove zero refresh/companion/signing on deny, exact
  same-revision results after allow, and non-retryable unknown outcomes.
  Negative cases reject managed-profile and arbitrary-plan markers before OPA.
- Template-policy source contract tests reject numeric, unknown, or premature
  final-v1 schemas, unknown fields, duplicate semantic entries, incomplete
  closed file sets, extra files, symlinks, hardlinks, owner/mode drift, hostile
  YAML features, and same-read byte drift. They prove unchanged source comments
  and ordering survive bookkeeping while semantic canonicalization remains
  deterministic.
- Typed-policy projection tests prove a Workspace Template owns exactly
  `template.yaml` plus transitional `policy.yaml`, executable source never
  appears in config or Workspace roots, the fixed evaluator makes first
  brokered use learnable until an exact Context Memory rule is installed, and
  the complete internal bundle is layout-checked and schema-checked. Source
  transaction tests exercise
  rollback, durable commit recovery, incomplete activation, concurrent runtime
  mutation, cross-process locking, and direct external editing; a journal or
  ambiguous generation remains fail closed.
- Doctor tests prove the full report is emitted, failures produce
  `diagnostic_failed`, warnings alone remain healthy, and provider/root-key/
  vault/broker/project-binding observation does not create a key, start or
  unlock services, or mutate auth state. Domain/application fixtures pin the
  finite topological inventory, direct blockers, independent continuation,
  task-owned recovery, cancellation before blocked-row materialization, and
  schema-1 recursive output. Infrastructure fixtures allowlist observational
  Docker argv, reject policy test-container creation, and compare content-aware
  fresh and unsupported-version Workspace Template trees before and after doctor.
- Clean-break fixtures distinguish exact fresh final absence from bounded
  declared legacy presence. Legacy-only, legacy-plus-final, unsafe, symlinked,
  and changing observations return typed reset-and-recreate guidance while
  preserving the complete filesystem tree and every Docker/OPA/Gateway/Broker
  call count. No predecessor identity, policy, Runtime, Workspace home,
  principal/session, or credential becomes a final result or protection edge.
- The human permission path is exercised through the actual public
  \`review permissions\` handler. Raw-terminal tests cover list, typed detail,
  exact Allow/Deny, path-template Allow-only choices, clear, final staged
  review, explicit Apply, manual refresh with staged-set discard, and
  pre-Apply cancellation. They prove zero mutation before final confirmation,
  unchanged opaque review-item IDs, separate attachment/persistent lifetimes,
  inline raw-input/cursor restoration before mutation, and an outcome-first receipt
  with authoritative revision and ordered rule identities. Redirected and JSON
  review remain read-only. A handler-reachability canary proves the public
  Catalog reaches \`runFinalPolicyReview\`; source ownership checks reject the
  retired predecessor handler, selector, catalog declaration, and their
  presentation-only corpus. The release contract has no watch or notification
  input. TTY \`policy rules\` separately covers current-decision inventory and
  explicit reset.
- Catalog retirement canaries prove bare `review` is a generic namespace with
  exactly `permissions`, `runtimes`, and `services`, performs zero task calls, and is not a
  registered command. Exact-path tests reject the retired `policy review` and
  former selector route without an alias or handler fallback. The three leaf
  contracts retain their distinct staged versus immediate workflows, output
  schemas, reference kinds, collection coverage, and redirected read-only
  behavior.
- Research-surface Operator Console tests pin one valid typed snapshot before listen, exact
  `127.0.0.1:0` ownership, 256-bit bearer generation, loopback peer and exact
  Host/Origin/authentication gates, closed routes and methods, strict bounded
  JSON, CSP/no-store/no-cookie/no-external-asset responses, dark/light token
  parity, refresh-safe opaque-ID staging, explicit final review, one canonical
  apply call, authoritative revision receipts, zero automatic mutation retry,
  opener fallback, and bounded cancellation cleanup. Browser-level tests prove the
  fragment is removed after transfer to tab-scoped session storage and that
  unconfirmed or abandoned staging delegates no write.
- Container integration PTY helpers set `NO_COLOR` explicitly so semantic
  assertions do not depend on a developer or runner environment or split a
  plain-text canary across independently styled spans. CLI unit tests retain
  ownership of ANSI and styling behavior. Positive Docker network-membership
  assertions use a bounded convergence read after service health; negative
  topology assertions remain immediate so an unwanted attachment cannot be
  hidden by retry. The owner-only synthetic TLS key is generated and consumed
  under one explicit fixture UID/GID; the harness does not broaden its file or
  parent-directory modes to accommodate an image-default user. Linux
  integration deletion also proves that HOME-relative project bind targets are
  pre-created by Tobari as owner-only directories rather than left for Docker
  to create as the engine user; symlink, non-directory, and broad-mode
  substitutions fail before container creation.
- Parent-owned blind E2E runs use `scripts/pty-evidence.py` when raw terminal
  evidence is required. The runner allocates a real PTY, sets explicit
  rows/columns and `TERM`, applies a timestamped short-input schedule, and
  records exit status, raw bytes, SHA-256 digests, and output checkpoints.
  The raw bundle must be written to a task-owned directory outside the
  repository. `transcript.redacted` and `metadata.json` are the reviewable
  projection; redaction changes printable host paths, literal values, and
  opaque IDs but preserves ANSI control sequences and the raw digest boundary.
  `python3 scripts/test-pty-evidence.py` is the executable contract test and
  runs as part of `task check:fast`.
- Context-principal integration creates same-root and independent-root
  Tobari in different Workspace Templates, checks distinct owned Workspace and Gateway
  network endpoints and non-overlapping dedicated networks, distinct homes,
  shared same-root host-file effects, cross-Context handle rejection,
  and exact registry cleanup after Workspace Template-bound deletion. Source spoofing,
  stale registry, IP_FREEBIND, restart, and recovery permutations remain in
  focused Gateway and recording-runner tests.
  Gateway unit tests separately reject retired managed-profile selectors before
  OPA or upstream I/O.
- Auth Broker runtime integration inspects exactly one broker beside one
  Gateway and one OPA, verifies the separate control/runtime socket mounts and
  fixed resource/log bounds, proves the runtime socket crosses the exact
  owner-labeled Docker local/tmpfs volume read-write in Broker and read-only in
  Gateway (never a host bind mount), unlocks through a synthetic host root key, issues
  different handles to Workspace Template-bound projects, proves an OPA denial performs no
  resolution, rejects copied handles across real Workspaces, and confirms that
  primary-secret canaries never appear in Workspace files, environment, host
  state outside encrypted vaults, or component logs. Rotation, revocation,
  locked restart, and protocol permutations are owned by focused Broker,
  Gateway, root-key, application, and CLI tests.
- Policy-boundary tests prove the normalized request authority and exact
  1-65535 port are required by the structured boundary, HTTPS on a non-default
  port remains learnable, invalid ports are terminal, and learned rules do not
  cross ports or schemes.
- Gateway boundary tests resolve and pin an upstream address, reject unsafe
  resolved addresses for dotted hosts, and preserve the explicit single-label
  private-service exception only in its focused fixture. Broad Docker
  integration uses a reserved synthetic FQDN on an isolated test-only subnet
  whose addresses are classified as globally routable; it does not relax the
  production resolver to reach a private address. Final reconciliation must
  remove the unmanaged fixture network, after which the harness explicitly
  restores it before the next synthetic upstream assertion. During a live
  attachment, the harness removes that fixture network after recording the
  denial so Policy Allow proves the supported OPA-only settlement path rather
  than requesting the physically fenced Gateway replacement path.
- Gateway boundary tests prove ordinary body content is absent from policy
  input, a request stream is enabled only after allow, and local denial
  responses are not treated as authorized upstream responses. Declared
  GraphQL tests prove only operation type and canonical root fields enter
  policy identity, every root is required, original bytes are forwarded once,
  unsupported envelopes fail before OPA learning or upstream I/O, and source,
  variables, arguments, and aliases remain absent from policy and audit.
- Signed AWS RPC tests prove only wire protocol, SigV4 service, and exact
  `Action` or signed `X-Amz-Target` enter identity; no service catalog or
  IAM/read-write classification exists. Unsigned, ambiguous, changed,
  streaming, URL-query-mixed, and unsupported forms fail before OPA learning
  or upstream I/O, while ordinary signed non-RPC AWS traffic remains HTTP.
- Domain and Gateway tests prove body variants aggregate into one exact
  candidate and learned rule. Integration retains only the transport facts:
  allowed chunked uploads and SSE responses arrive incrementally, and the fixed
  8 MiB advertised-body cap rejects an over-limit request before upstream I/O.
- Runtime-asset and integration tests enforce fixed JSON log rotation for the
  shared Gateway, OPA, and Auth Broker services.
- Runtime-asset and integration tests inspect the fixed shared-service CPU,
  memory-plus-swap, and PID ceilings.
- Runtime-asset and integration tests require OPA's fixed Go heap limit to stay
  below its container ceiling, and the first-use/upgrade transition matrix
  exercises enough watched-bundle settlements to expose a missing headroom
  contract.
- Learned-policy domain, application, and CLI tests pass opaque denial
  candidates unchanged into exact allow/deny/reset actions, infer only one safe
  raw-segment template, and reject prefix/compaction and cross-scope shapes.
  Docker integration retains one unchanged opaque candidate-to-allow round
  trip solely to prove live activation and retry.
- Negative tests prove rejection before side effects.
- Release tests inspect actual artifacts and metadata, not only workflow text.
  Archive tests cover deterministic multi-entry order, canonical metadata,
  create-only output, regular-file identity checks, exact executable/license/
  optional-notice bytes, and independent reopen verification.
  Release lint additionally proves that parallel CI owns every source profile;
  preparation accepts exact successful main-push CI, builds and retains one
  verified asset set; publication promotes only one matching successful
  preparation run and cannot rebuild; and only stable protected publication
  may cross into `tasuku43/homebrew-tap`. The GitHub App token is scoped to that
  repository, the exact published Formula is propagated, and the mutation ends
  at a Formula-only pull request rather than a direct `main` push.
- Work-packet tests retain the Accepted compatibility state; reject unsupported
  status, unchecked GFM acceptance/tasks, malformed fence evasion, template or
  cyclic successor chains, and missing successors; and retain
  regular-file/no-symlink repository policy.

A global coverage percentage is not a substitute for these contracts. Add tests at the boundary where a future regression would otherwise pass unnoticed.

## Claims-to-checks discipline

Every strong statement should identify its enforcement path.

| Claim type | Preferred enforcement |
|---|---|
| Layer dependency | Go-aware architecture lint and import-boundary tests |
| Build and resolver identity | Pure identity validation, exact version text/JSON tests, standard/dev build-tag resolver fixtures, artifact metadata inspection, and a zero-progress/zero-Docker API-mismatch preflight test |
| Finite domain state | Types, constructors, and table-driven negative tests |
| Domain-owned Catalog vocabulary | Caller-immutable canonical ordered sets consumed by domain validation and every inventoried exact Catalog field path, plus ledger-pinned constructor/output fixtures for semantic branches and explicit-empty correlations |
| Catalog completeness | Whole-catalog contract tests |
| Public executable composition | Exact whole-public-catalog Program/path-to-production-handler mapping across the release and attachment helpers; targeted nonzero Go execution evidence for each exact release/helper handler (never a global coverage percentage); focused CLI behavior assertions for public mutation projections; and an explicit release-binary scenario for root-owned environment, terminal, or adapter-composition journeys that cannot be proved by ordinary handler dispatch |
| Semantic terminal presentation | Zero-violation deterministic CLI-experience evaluation covering terminal-owned named colors, six semantic tokens, reviewed markers, shared spinner ownership, `NO_COLOR` document parity, raw-journey continuity, public-handler reachability, and predecessor-owner absence; ledger-pinned typed fixtures across colored TTY, redirected text, scoped empty, warning, failure, and neutral cancellation; inline saved-origin redraw, final-frame retention, cursor restoration, and negative DEC-1049 canaries; bounded namespace/suggestion tests; and AST lint rejecting style-dependent structure or direct ANSI SGR outside the shared style layer |
| Output delivery versus collection coverage | Independent finite enums and catalog tests, including complete bounded/differential windows and paged exhaustive traversal |
| Operationally closed supported outcome | Reviewed agent-readiness transcript with zero undeclared external reconstruction, plus task-owned deterministic-composition tests and declared field extraction |
| Request-bound semantic result | Per-capability domain/application tests for declared task identity and every applicable request dimension, including scope, state, contextual-kind, empty-result, no-partial-result, and negative-inference fixtures where applicable |
| Action target composition | Reachable reference-graph validation and byte-preserving round trips for reference-bound acts; complete and exclusive declarations for command-bound fixed targets; reference-free fixed reads/writes; and fixed-create canaries that allow only distinct confirmed child-resource refs while rejecting consumed refs and escaped scope refs |
| Recursive produced references | One bounded `OutputField.Fields`/`Items` traversal with deterministic dot/`[]` paths; forcing-shape, duplicate/cursor-collision, reachability, scoped-help, and workflow tests; typed inputs remain the consumed-reference authority |
| Side-effect ordering | Fake adapter counters and failure-before-I/O tests |
| Ancestor Workspace choice | Typed nearest-first candidate fixtures, selector key/fallback tests, locked stale-choice checks, zero-downstream-call cancellation tests, attached-current versus unattached-current creation tests, and a release-binary real-Docker PTY canary proving descendant-CWD ancestor reuse versus explicit distinct-Context nested creation after the exact parent test container is stopped without selector mutation |
| Session-versus-Workspace lifecycle | Child exit-status preservation, host stderr guidance, stdout/stderr ownership, logical-state-after-exit, and explicit delete tests |
| Attached-session deletion guard | Docker Exec ID observation, guard-before-delete negative tests, force override, and stable structured fault/help contract |
| Context/Workspace lifecycle target safety | Exact opaque-reference round trips, duplicate/empty parser rejection, unknown/stale zero-I/O counters, immutable Context binding, explicit attachment states, typed `end_active_session`, and frozen presentation evidence |
| Docker bind-source identity | Canonical XDG-root and short-temporary-root derivation, dangling-management-symlink and delimiter/control rejection, legacy/final ProjectRoot revalidation before existing start and after create, unstarted-container cleanup on drift, bounded networkless fresh-cluster visibility/type probe inventory, store-level zero-decision/zero-named-mutation failures, and required shared-context lifecycle replay |
| One Context per canonical root/Template pair and one Workspace per Context | Domain duplicate-index validation, exact Context binding checks, repeated create rejection, same-root/different-Template Contexts, and concurrent convergence |
| Custom image isolation | Runtime-API inspection plus exact create-argv and Docker integration tests |
| Agent executable/home separation | Runtimechecker path assertions, local image builds with a home overlay, and Tobari smoke tests for each agent command |
| Shared runtime resource bounds | Fixed project create-argv, resource-aware spec hash, and Docker HostConfig integration assertions |
| Workspace Template Runtime boundary | Complete Template-body tests, exact Runtime-revision compatibility, ignored-Project-metadata regression, Workspace-entry reconciliation, home preservation, and failure-before-AppliedEntry tests |
| Portable policy activation | Pre-mutation OPA tests, exact OPA and volume owner-label checks, owner-only archive/stdin staging without a host bind, fixed networkless volume-only builder and same-volume publisher argv, preflight-created volume cleanup on success/failure/cancellation, pre-existing-volume retention and causal conflict, unchanged-ready publication skip, exact active revision plus defined decision readiness, stable OPA identity, invalid-bundle retention, and Docker integration |
| Workspace Template definition and selection | Stable TemplateID/digest tests; planned editable Boundary changes; complete revision publication; opaque Plan/reference mutation; fixed-target draft create; reference-bound draft copy/default/delete; absence of granular config/Runtime setters; no Context, Policy Memory, Workspace, auth, attachment, applied, observed, or lineage copying; atomic owner-only generation tests; zero-Docker Template actions; Configurator task-scope/catalog/Template-stage lock tests rejecting symlinks, hard links, unsafe metadata, and descriptor/path replacement before source mutation; and Catalog/human/JSON/readiness fixtures |
| Template revision authority and history | WorkspaceTemplateID+semantic-digest authority tests; generation correlation-only validation; semantic no-op leaves generation unchanged; A→B→A creates a later generation with the same A digest; retained `generation+digest` receipt accepts only exact same-ID/canonical-body idempotence and rejects other identity/body or partial artifacts |
| Desired/applied/observed reconciliation | Typed desired, AppliedEntry, observation, adoption, and failure fixtures; Current/Next human and schema-2 JSON contracts; entry and cluster-up as the only mutation boundaries; attached pending adoption zero-Docker-call rejection; success-after-verification ordering; failure/cancellation preserves last success under the same lifecycle lock; status/list/show/doctor/completion fresh and existing-state zero-write canaries |
| CWD status home | Nearest-root-before-default selection; one task-owned port/snapshot; independent Template-policy, Policy-Memory, AppliedEntry, Runtime-material, cluster, attachment, permission, and Service axes; no overall status; structured Next/Attention; schema-3 recursive Catalog/output/reference fixtures; fresh zero-write/zero-Docker evidence; exact 6-call normal and 12-call one-retry ceilings; sibling-count independence; non-creating owner reads; and release absence of predecessor, private Runtime, Service action, inferred usage, and research capability fields |
| Pre-release final-authority migration boundary | Exact fresh-final-empty classification; ordinary-read migration-required fault; strict exact typed `authority.json` decoder; Advanced/Rego/unsupported/unsafe rejection; read-only authority plus complete predecessor Runtime-tree digest Plan; unchanged opaque Apply reference; stale zero-mutation; stable-ID custom Runtime config/state conversion with exact Template-reference resolution; phase-fault recovery at each config/state/source/object/manifest/pointer rename and sync; read-back rollback to byte-identical old roots; retirement/no-coexistence proof; no credential or unsupported predecessor adoption; and a future-release-policy guard |
| Workspace Template deletion boundary | Exact Template-reference destructive intent, lifecycle-lock coverage, default-selection and Context-reference rejection, Template-only removal, Project/Context/Workspace/Policy-Memory/Runtime preservation, hostile-path and symlink canaries, and text/JSON contracts |
| Workspace Template source-access boundary | Closed enum/default tests, exact desired-spec hash and Docker bind inspection, read/change/delete/Git-write canaries, writable home/tmpfs checks, no writable alias inventory, same-root cross-Context observation, nested home-relative roots, and unsupported-state rejection |
| Workspace Template-owned policy ceiling | Normalized owner-only snapshot/digest tests, source-change immutability, complete default-plus-exact-override method resolution with canonical ordering and ambiguity rejection, method-deny and GET-only canaries, single-catalog current/history readiness validation, orthogonal enabled/disabled binary-readiness replacement without snapshot rewrite, stale active-revision invalidation and entry recovery, destination/method-Deny terminal canaries, exact-Deny precedence over broad method Allow, fixed-evaluator precedence over baseline/learned policy, scheme-aware exact learned identity, and terminal-denial zero candidate/DNS/Broker/upstream counters |
| Workspace Template session-default boundary | Strict `template.yaml` fields, exact Template-reference Apply binding, complete typed input, partial/invalid zero-write canaries, immutable complete-revision publication, later-child timing, absence of granular setters, and Workspace-home non-mutation |
| Workspace Template shell environment boundary | Fixed allowlist and closed source-schema domain tests, explicit-empty preservation, exact source Plan/fingerprint/Apply binding, zero-I/O rejection for arbitrary names and ambiguous values, mechanical absence of granular setter/publication paths, complete Workspace Template report output, exact child-exec environment assertions, missing-export fallback, Bash-quote and bounded inherited-value canaries, and host-credential non-copy assertions |
| Workspace Template Git identity boundary | Closed pair/source domain tests, planned whole-Template source publication and direct-writer absence, exact two-key global Git argv with an absolute executable and exact `HOME`/optional `XDG_CONFIG_HOME` plus fixed-control environment allowlist, project-owned config-directory and `PATH`/loader/shell-startup canaries, timeout/output/framing/unsafe-value bounds, malicious local-include exclusion, private atomic Workspace projection encoding, symlink and existing-file size checks, read-only directory mount and system-scope precedence, excluded helper/signing/auth/path keys, absent/incomplete-pair behavior, and secret-/personal-data-free faults and fixtures |
| Workspace Template Workspace bootstrap boundary | Closed AWS IAM Identity Center plus dependent EKS schemas, AWS-only revision compatibility, composed semantic digest/generation/diff tests, fixed host AWS and Kubernetes configuration-file regular-file/size bounds, shared parse/resolve paths for exact preparation and typed discovery, available/unavailable candidate invariants, shared-session and matching-profile fixtures, malformed/duplicate/unsafe whole-file zero-partial-result canaries, selected-reference and exact `aws eks get-token` validation, unknown/helper/credential/cache/proxy/TLS/file-reference/arbitrary-exec/alternate-path/symlink rejection, secret-free reports, selected-semantic drift rejection with unrelated-profile tolerance, atomic Workspace Template replacement, exact private `.aws/config` plus canonical `.kube/config` bytes, create-before-publication rollback, no credentials/cache projection, predecessor-schema rejection, existing-Workspace byte preservation, dependency-aware removal, and `not_configured`/`not_applied`/`current`/`older` status coverage |
| Template create and copy remain distinct | Fixed-target `template create --name` with closed source-access and bounded exact GraphQL endpoint inputs; reference-bound `template copy --from ... --name`; fresh unpublished ID/source with null base revision; generation-1 Plan/Apply both before and after unrelated active authority exists; exact source identity, ownership, Runtime, and revision revalidation; no lineage or lower-lifetime state; zero active authority/reconciliation before Plan/Apply; and unknown/missing/invalid/tombstoned/stale/concurrently-published/collision zero-publication canaries |
| Guided first Workspace entry | No-authority draft transcript; exact Template publication/default selection/Context creation/cluster/standard-Runtime preparation/entry checkpoints; five stages and seven states; partial-success recovery; noninteractive fresh zero-mutation; default shell; exact positional-only child argv/status; cancellation settlement; plan-bound DecisionRef stability across unrelated final-authority revisions; stripped-public-fault fallback into same-plan pre-release receipt rebinding; Context-owned Claude settings preservation across Workspace replacement; idempotent repeat convergence; exact `builtin/standard` revision parsing through the real legacy guard; and one release-CI real-Docker public-command scenario from absent Tobari config/state roots and absent exact Tobari images through confirmed Workspace entry, exact-root re-entry, descendant ancestor reuse, and explicit stopped-ancestor nested creation |
| Previous-release Workspace continuity | Nearest reachable annotated development tag and exact ancestor commit; predecessor tree's own release packager and verified release/embedded/compatible identity; isolated absent XDG roots and explicit non-default Docker context; real predecessor first entry and policy denial; candidate-side candidate count, Allow, rules, status, and bare re-entry; same Context/Workspace identity after policy and unrelated-Context authority revisions; public Workspace-delete outcome matched against actual Home preservation; new Workspace identity only after explicit deletion; exact Context-owned Claude settings bytes throughout; and a dev.22 negative replay that reaches and fails the post-Allow bare entry |
| Shared Runtime revision boundary | Fixed Runtime-catalog target contracts; typed `--copy-source-from standard|NAME`; no reference/lineage binding or `--base` alias; fresh-ID/empty-history atomic creation; bounded owner-only source copying; snapshot-before-BuildKit ordering; cold pre-Workspace canonical-parent preparation before the managed attempt; bounded parent diagnostics and cancellation-safe pre-attempt rollback; one timeout-/byte-bounded compatibility and identity contract; immutable-ID binding after ownership evidence; volume-free, oversized-metadata, and mutable-retag canaries; NoChange compatibility revalidation with precondition/none rollback; build and restore publication recovery revalidation at retained journal phases before mutation; post-build incompatibility as retained mutation/partial authority; failed/no-op history preservation; zero Template/Context/Workspace writes during create/build; RuntimeID+digest references; and Workspace-entry reconciliation with home preservation |
| Managed Runtime lifecycle closure | Exact Runtime/revision/prune-plan reference graph; strict closed `runtime.yaml` plus editable `source/` config and separate immutable state; complete coherent catalog/protection/journal/snapshot/material observation; current/retained Template plus Workspace applied/pending/observed protection; standard and cross-Runtime active-lifecycle rejection; canonical selector, content correlation, shared/foreign-use preservation, bounded Docker output/calls, and zero-create reads; immutable-source restore; non-forced prune; phase-aware two-root create/delete quarantine journals with restart settlement at every rename/sync boundary; monotonic terminal receipts, same-name and interruption replay; final-composition TTY one-confirmation review recovery, failed-draft recovery projection without invented history, and exact active-journal recovery routing; nullable reclaimable bytes and unknown last-used; exact human/JSON/fault/help contracts; no revision delete, global prune, Docker-ID authority, or Runtime-specific produced-reference walker |
| Gateway source and image boundary | Canonical-source/snapshot byte comparison, pinned mitmproxy parent, signed nftables/iproute dependency inventory, canonical-source unit tests, source API-1/role labels, transparent-only listener and fixed network-guard entrypoint, explicit rejection of non-transparent ingress, absence of proxy environment/port exceptions, content-addressed development selection, Gateway-only lock validation, bounded immutable digest/platform/entrypoint/volume metadata preflight, exact inherited-CA-only `Config.Volumes` allowlist with Compose tmpfs shadow, mutable-tag retag canary, non-root resident process, and validation/release workflow permission separation |
| Research Auth Broker source and image boundary | Canonical-source/snapshot byte comparison, canonical Python tests in the pinned image environment, provider-CLI absence including Codex/Claude, source API-1/role labels, content-addressed development selection, absence from the release lock/publication workflow, bounded volume-free metadata and immutable-ID execution canaries, non-root Dockerfile, and validation workflow permission separation |
| Context-owned encrypted research credentials | Root-key backend tests, strict owner/mode/symlink checks, AES-GCM schema/ContextID AAD canaries, atomic vault replacement, missing-key-with-vault rejection, and secret-free outputs |
| Cluster teardown mode recovery | Exact-mode active-journal rejection; retained-down-to-purge explicit upgrade; completed-purge-to-retained dominance only after stopped-runtime and purged-volume revalidation; recreated-volume negative canary; repeat-completion zero-removal assertions; and a real-Docker lifecycle sequence covering purge followed by ordinary down |
| Authentication state survives cluster teardown | Exact down/purge resource assertions, preserved vault/key canaries, and subsequent cluster-up unlock/status proof |
| Doctor remains observational | Fixed dependency-matrix, direct-blocker, complete-report, schema-1 renderer/agent-contract, fail/warn exit, cancellation, Docker-argv allowlist, host-only policy-source validation, content-aware fresh/unsupported-version snapshots, and zero-create/zero-repair canaries across root-key, vault, provider, broker, and project-auth state |
| Every declared read remains observational | Dynamic public-catalog handler coverage, per-command fresh-XDG before/after snapshots, zero Docker-mutation argv, lockless fresh lifecycle reads, read-only/concurrent fixtures, unsupported-version fail-closed state, and bounded cleanup of only a pre-existing validated journal |
| Pre-release final authority cannot implicitly inherit development state | Clean installation to exact final-empty first use; generation-only ordinary read/mutation/recovery; bounded fixed-path legacy-presence and unsafe/drift rejection; final Context source absence before first publication plus strict final-owner validation afterward; Context draft creation followed by healthy final observation and declared read-fault classification; exact supported typed predecessor accepted only by `installation migration plan/apply`; no unsupported predecessor influence on Runtime protection, policy, principal/session, or authentication; no old alias or compatibility fallback; explicit reset-and-recreate guidance; and a release-policy guard proving the exception cannot authorize a post-release clean break |
| Context-bound broker handles | Full Context/provider/revision/target/header round trips, hash-only live index assertions, copied/stale/rotated/revoked negative tests, exact Context eligibility and next-entry semantics, and Workspace projection reconciliation |
| Agent OAuth acquisition and pinned projections | Multi-version host Codex tests require one exact compiled driver-contract revision, fixed argv/environment, canonical executable digest, and strict captured state while treating stable product version as audit metadata; exact-key/byte tests independently fix the Codex 0.147.0 Workspace `.codex/auth.json` sentinel shim with only `${HANDLE}` as `tokens.access_token`; Claude tests fix exact 2.1.220 selected-image login, host-side executable hashing, no-mount container isolation, four renewable fields plus structurally bounded subscription/rate-limit labels, dynamic scope-token validation, granted-subset enforcement, canonical normalization into a Tobari-owned record, secret-free capture-stage faults, fixed progress transitions, and fixed-client refresh that preserves the stored set and labels; exact-byte Workspace tests require only `${HANDLE}` as secret-bearing material, the public `dummy-value` refresh sentinel, the captured non-secret scopes and entitlement labels, and an exact `hasCompletedOnboarding` JSON merge that preserves unrelated Claude state and rejects unsafe targets; an isolated real-client matrix proves empty refresh is reported expired while the sentinel plus captured labels restores the native account/default-model UI; direct synthetic Gateway bearer/account-header checks, API-key/auth-token absence, modified/symlinked-file refusal, and Workspace-client drift rejection remain required |
| Declared provider bindings require Broker | Direct bearer/raw and AWS SigV4 canaries assert `broker_auth_required`, secret-header removal, and zero fallback/Broker/OPA/DNS/upstream calls; one undeclared-binding canary retains post-policy compatibility passthrough |
| Broker fallback requires an undeclared binding and marker absence | URL/path/query/fragment/header-name/value marker canaries, malformed/ambiguous/binding-mismatch rejection, and compatibility passthrough tests with no declared binding or marker anywhere inspected |
| Post-policy credential action | Gateway call-order/count tests for handle removal, introspect-before-OPA, zero resolve/refresh/companion/signing on deny, one same-revision reviewed action after allow, exact header replacement/signing, and no-secret canaries |
| Closed broker plan boundary | Exact static and dynamic record schemas, immutable renewable-adapter, request-signing-mechanics, persisted-record-contract, control-login-plan, and Gateway reviewed-profile membership; one strict versioned test-only capability fixture checked from Go and Python for built-in/acquisition/manifest/login/record/runtime/Gateway parity; adapter/plan/profile incapability tests, record/Vault import compatibility, shared Broker-owned login commit and snapshot/single-flight/barrier/CAS conformance, Context/provider/revision/HTTPS-header or signing introspection, bounded AWS/Datadog/OpenAI/Anthropic behavior, rotation/revocation, durable barriers, and no invalid-handle fallback |
| Unsupported broker capabilities stay absent | Catalog, state-parser, dependency, image-content, and hostile-header tests reject managed profiles, owner-selected dynamic plans, arbitrary helpers, compatibility readers, and provider CLIs inside Broker |
| Research protected provider acquisition | Catalog selector/method/stdin contracts, terminal rules, bounded readers, canonical GitHub/AWS/pup/Codex/Claude identity/digest checks, fixed argv/environment, control-safe output, bounded browser/PTY behavior, checked cleanup, cancellation/failure preservation, and required synthetic integration; optional live observations make no standard publication claim |
| Typed denial recovery | Strict host/port audit projection, query/header absence, whole-path handle-marker redaction, non-learnable structural rejection, per-record isolation with an explicit unparsed count, fixed host-review navigation schema, host-stderr session summary, empty and partially unprojectable bounded scopes, hostile-field canaries, and end-to-end JSON assertions |
| Explicit policy learning | OPA scheme/port learnability classification, terminal deny exclusion, deterministic repeated/concurrent Context/scheme/host/port/method/path candidate aggregation with required latest/count, two-distinct-example single-raw-segment template inference with ambiguity and unsafe-path suppression, Context-scoped reference validation, single-reference exact allow/deny/reset round trips, bounded typed TTY staging with template/exact choices, same-receipt byte-exact reobservation of every selected non-durable candidate, durable-only pending removal, retained-evidence recovery after activation, zero-create lifecycle-observed journal/collection projection, private recovery serialization canary, publication-interruption plus disappeared-denial process-reconstruction recovery, explicit raw/line TTY resume, JSON/redirected zero-action tests, exact public-help projection of internal Apply faults, terminal/output read-only reconciliation, one fixed-target Apply and zero-write discard, installation-wide inventory/review, aggregate preflight ordering, and Docker retry |
| Research session-scoped Operator Console | Standard-catalog absence plus research-catalog presence; typed pre-listen snapshot conformance; TCP4 random-loopback binding; exact peer/Host/Origin/bearer/method/path/content-type and bounded-body negative tests; CSP/no-store/no-cookie/no-external-asset canaries; fragment bootstrap and dark/light browser tests; inert staging; one canonical fixed-target Apply with receipt and no retry; opener fallback and cancellation cleanup |
| Ordinary HTTP authority order | Aggregate-router and OPA tests prove principal/schema/Workspace Template validity, terminal destination/method decisions, one combined trusted-baseline plus remembered exact-Deny tier with no internal order, Template static positive authority, remembered reviewed Allow, and unresolved fail-closed/default-review completion; exact-Deny-over-baseline and terminal-ceiling zero-candidate/DNS/Broker/upstream canaries prevent lower-tier bypass |
| Attachment-scoped Host Loopback authority | Exact `host.tobari.internal` hostname/URL-template, public capability schema V1, private route/grant schema V2, hostname-bound opaque IDs, bounded port, and destination/lifetime domain tests; terminal non-learnable `host.tobari.test` plus sibling/suffix/private-ceiling canaries; host-derived epoch and Workspace-wide audience assertions; negative retired `--host-http` parsing; owner-only strict route registry and secret-free capability projection; aggregate-router cross-branch canaries prove ordinary authority cannot decide Host Loopback and attachment authority cannot decide ordinary traffic; random-port 256-bit-authenticated host relay that revalidates the reviewed IPv4-loopback target port while preserving exact Host; Gateway principal/epoch/port derivation and one-shot stream tests; OPA exact Attachment Deny/Allow/review order and lifetime/port-confusion canaries; zero external-DNS/upstream/Broker/relay/retry work for retired, malformed, unauthenticated, unreviewed, stale, mismatched, denied, unavailable, and post-detach requests; pre-leaf TLS rejection and CA/cache non-authority; final lifecycle-to-interactive-to-host-loopback lock ordering, legacy-registry zero-mutation rejection, and deterministic competing-entry fences; route-first teardown removes both route and grants, borrower non-ownership preserves both only until owner exit, and crash reconciliation removes stale final state; standard Runtime curl/libc, Python, applicable Go pure/cgo, and Node resolver/HTTP canaries; policy-review lifetime presentation; and supported Linux/macOS Colima Docker integration |
| Attachment-owned Workspace service exposure | Recursive Catalog reference closure and Program-filtered dispatch/help for pending-only review, host status/actions, and JSON-only helper create/status/stop; fixed-create child-reference and mutation-complete canaries; one-key/raw and one-token/line confirmation with redirected/JSON read-only and pre-read watch/notify rejection; generic evidence-free notifications; final Context/Workspace plus distinct Service AttachmentID authority; bounded owner anchor with complete/partial/unavailable counts, unsafe/duplicate/contradictory fail-closed state, no read cleanup, and fresh exact-action re-resolution; `tcp4 127.0.0.1:0`, independent 128-bit lowercase `.localhost` origins, exact Host/absolute-form/framing validation before Workspace I/O, no numeric fallback, unchanged accepted headers/cookies/Origin/content, fixed 502, HTTP/1.1/WebSocket/backpressure, and listener-first bounded cleanup receipts; pinned Playwright Chromium cross-origin cookie and `Domain=localhost` contamination canaries; purpose-limited reference-only browser opener with not-dispatched/requested/unknown outcomes; dedicated hardcoded helper, checked source/snapshot equality, engine architecture packaging/mount evidence, and isolated Engine/Desktop/Colima integration; public guard positive/negative exact-origin exception; capability/catalog/public-boundary coverage |
| Attachment-owned permission resume | Dedicated hardcoded `tobari-permission wait` Program with plain bounded correlation input and no reference producer, completion, discovery, or local output walker; one canonical interactive owner and shared borrower epoch with service-controller exclusion; exact frozen-v1 principal to schema-3 denial plus schema-2 wait projection and byte/schema canaries for every frozen sibling wire; owner acknowledgment plus exact post-ACK authority re-read before resume publication; CSPRNG IDs; closed host-platform selection of Linux `unix` or Darwin `loopback_tcp` with exact Gateway profile match, no Docker-provider/context parsing, and zero probe/fallback/downgrade; Colima-only Darwin release evidence and explicit unsupported/unvalidated-provider omission; loopback-only bind and exact `host.docker.internal`, nonce-first constant-time authentication, owner-only regular/no-symlink registry shape, atomic replacement and three-renewal visibility, invalid replacement with zero stale fallback, registry/mount/environment absence from OPA/Workspace/guards, and log/public-output coordinate absence; bounded private registry, ingestion frames, rate, lease, live-record count, attempts, concurrency, teardown, stale/replaced/symlink rejection, and non-enumerating faults; strictly advancing lease issue time with rollback/non-advance/expiry rejection; canonical live-OPA exact/template Allow, explicit Deny, default-deny pending, revision-lag, ambiguity, strict final-active-receipt revision binding, and legacy-state-only rejection tests; helper packaging/read-only mount and cancellation/re-entry canaries; zero proposal, policy mutation, reconciliation, TTY injection, and automatic retry; prompt-anchored PTY plus explicit fixture-network restoration integration from denial through separate trusted-host Apply, wait result, and deliberate fresh Gateway-authorized retry |
| Declared GraphQL policy identity | Exact trusted endpoint projection, hash-pinned parser and license checks, strict bounded POST-envelope and body-free GET-parameter fixtures, query-only GET with URL-source redaction, persisted-query/extension diagnostics, absent-length acceptance under fixed transport and semantic caps, positive/duplicate/mismatched length plus transfer/content encoding canaries, conservative root-fragment expansion, all-roots OPA matching, HTTP-rule non-matching canaries, per-root audit/candidate/allow/deny/reset round trips, deterministic review-only state-change projection, prefix-rule rejection, raw-body/query privacy canaries, and zero-upstream integration |
| Kubernetes API policy identity | Validated EKS-origin projection, schema-free core/CRD and non-resource path fixtures, exact get/list/watch/create/update/patch/delete/deletecollection/connect plus dry-run identity, HTTP-rule non-matching and exact-rule OPA canaries, impersonation/ambiguous-mode local rejection, opaque object bodies, deterministic state-change projection, and zero-upstream denial |
| Git Smart HTTP policy identity | Exact discovery query and RPC path/media-type fixtures, repository plus upload/receive identity, HTTP-rule non-matching and exact-rule OPA canaries, malformed/ambiguous local rejection, opaque pack bodies and credentials, deterministic state-change projection, and zero-upstream denial |
| OCI Distribution policy identity | Distinctive catalog/tag/manifest/blob/referrer/upload route fixtures, exact repository/action/object identity including mount source repository, ordinary `/v2/` and `/v2/me` collision canaries, HTTP-rule non-matching and exact-rule OPA canaries, malformed local rejection, opaque bodies/credentials/raw query values, deterministic state-change projection, and zero-upstream denial |
| Signed AWS wire-operation policy identity | Exact commercial authority/SigV4 projection, strict Query and JSON transport fixtures, positive bounded length, Action and signed-target extraction, unsigned/ambiguous/changed/streaming/query-mixed canaries, exact OPA matching with HTTP-rule non-matching, audit/candidate/rule/CLI round trips, parameter/body privacy, and explicit absence of service-model/IAM/read-write inference |
| Protocol-classifier admission completeness | Exact discovery of `gateway/addon/*_request.py`; one harness row per source; Gateway import/call binding; positive, collision, malformed-local-failure, minimal-policy-projection, privacy-exclusion, no-ordinary-HTTP-fallback, zero-downstream, and finite deterministic adversarial-corpus evidence; parser network/file/dynamic-code/executable-I/O rejection; negative validator fixtures for missing rows/dimensions/evidence, unbounded corpus, and parser I/O |
| Workspace Template source access | Required manifest/catalog field, omission-default tests, immutable runtime spec/hash, Docker inspect/reconciliation, read-only mutation/Git-metadata denial, writable home/tmpfs, no writable alias, and same-root observation |
| Workspace Template-owned policy data | Strict normalization/digest/snapshot tests, dedicated binary catalog with unique family IDs, explicit pinned client versions, positive append-only contract revisions, one current contract, core-only new agent-ready snapshots, complete historical readiness stripping, current binary projection into existing agent-ready Workspace Templates without rewrite, `claude_ready`, `codex_ready`, `gh_ready`, custom-runtime `twg_ready`, and custom-runtime `pup_ready` exact native-authentication grants plus native discovery grants, GitHub GraphQL `query` / `viewer` endpoint/baseline with mutation/sibling/mixed-root/HTTP canaries, TWG exact device-code/token/revoke, site inventory, stable manifest, and GraphQL `query` / `me` endpoint/baseline with method/REST/beta-manifest/installer/download and neighboring-Atlassian canaries, pup exact US1 DCR/token POSTs with neighboring host/path/method/product canaries, method-deny zero-overlay canaries, one safe evaluation template, exact MCP endpoint and initialize/list baseline, exact tool-name action review with payload canaries, acquisition/file-transfer/update exclusions, exact-Deny precedence, fixed-baseline schema canaries, GET-without-safe-claim contract, and terminal zero-candidate/DNS/Broker/upstream calls |
| Context/Workspace principal and credential scope | Owner-only atomic registry, exact Workspace-source/Gateway endpoint and network uniqueness, forged Context/Workspace/SNI/authority and unknown-principal denial, source-spoof canaries, live drift detection, exact stopped-principal retention bound to prior aggregate/container/spec/configured-IP/network/Gateway/registry evidence with dead/ID/IP/attachment/registry drift rejection, Context-owned Broker tests, copied-handle cross-Context canaries, fixed-evaluator canaries, and multi-Context Docker integration |
| Standard native Workspace authentication boundary | Release-surface empty authentication-projection and full runtime-reconciliation tests prove zero Auth Broker inspection/control calls and no research auth-registry creation; the attached-session suite adds a closed Claude/Codex/GitHub CLI/AWS CLI/TWG/pup browser union, one binary-owned read-only opener, exact `BROWSER`/`GH_BROWSER`/`xdg-open` projection, and a dedicated schema-v1 Unix-socket/non-TTY Docker exec protocol; a shared closed query-field schema enforces required singleton values, rejects invalid definitions and unknown fields, and dispatches provider-owned validators; exact mandatory URL fields, individually reviewed optional selectors, device targets, provider clients or bounded DCR IDs, scope ceilings, state/PKCE/callback shapes including AWS's commercial-region/default-scope dynamic callback and pup 1.10.7's optional UUID-shaped `dd_oid`, complete 110-scope ceiling, and four fixed ports, direct Docker terminal ownership, fixed login/setup argv and pass-through, zero-listener device paths, dynamic non-privileged host-loopback-only callback relay to the label-verified selected Workspace, duplicate-key/unknown-field/version/size/replay/neighbor canaries, opaque callback canaries, and cleanup; the research-surface suite separately retains project-bound handle and file projection coverage |
| Transparent attached-child terminal ownership | Direct-stream coverage plus Unix PTY literal-`0x1d`, delayed-input, resize, exact-status, output-failure, and terminal-restoration tests; root human-help rejects any `Ctrl+]` or Trusted Host Review shortcut; excluded capability/catalog absence; and independent Permission Inbox raw-terminal tests |
| Atomic multi-Template policy activation | Source and projection locks, Workspace Template namespace rejection, complete all-Context OPA validation, content-addressed atomic publication, stale-revision rejection, known-good rollback, and invalid/concurrent mutation tests |
| File-backed desired resources | Stable-ID path and directory/document identity tests; exact Template/Context/policy V1 round trip; ordinary-reader alpha rejection; explicit content-addressed alpha-to-V1 source migration with active/source/target fences, no activation, idempotent replay, and crash-boundary recovery; numeric/unknown/relabelled-alpha/unknown-module rejection; strict unknown-field/duplicate-key/alias/tag/bounds/mode/owner/symlink/hard-link/unknown-child canaries; whole-directory staged Template pair publication and rollback; Template pair fingerprint, final pre-publication fence, stale-base and idempotent settlement tests; comment-preserving base bookkeeping; Context identity immutability and final fingerprint fence; exact `in_sync`/`modified`/`invalid`/`missing` output; missing-source last-known-good behavior; tombstone admission and interrupted deletion settlement; reflection/Catalog proof that only draft/plan/apply can reach ordinary Template semantic publication and schema migration cannot activate it; narrow source Runtime ID+revision; ID-based strict Runtime layout; and no-Rego whole-tree regression |
| Closed semantic policy modules | Exact sealed registry for generic HTTP, GraphQL, MCP, AWS, Kubernetes, Git Smart HTTP, and OCI Distribution; module-owned validation/matching/state-change projection; static and Policy Memory parity; semantic-set normalization and duplicate/shadow rejection; classified-traffic generic-HTTP exclusion; strict bounded parsers and privacy canaries; request-time sibling collision rejection before OPA/upstream with no falsely selected audit module; complete static Allow/Deny OPA execution and evidence; and final V1 source compiler round trips |
| Causal failure recovery | Closed phase/change-state validation, Catalog/runtime agreement including final Template/Context/Workspace CRUD emitted-fault matrices and stale-reference dispatch canaries, text/JSON equivalence, pre-action-none and post-action-unknown cancellation, standard-Runtime preparation failure/cancellation as mutation-unknown with read-only status recovery, confirmed-output preservation, lifecycle unknown/confirmed classifications, root/cluster status legacy-state canaries that require non-retryable observation and terminating `doctor` recovery instead of a self-loop, provider-neutral root and direct-`cluster up` readiness with Engine 23/24 boundaries and zero reconciliation calls on failure, and a machine-checked release Catalog graph that rejects self-loops, closed reference cycles, nonexistent paths, unchecked required inputs, action rediscovery, output-encoding replay, and mutation retry after unknown state while permitting only causally terminating read-only classifiers |
| Confirmed mutation output | One effect-aware finalizer, late-cancellation regression, non-retryable mutation short-write fault, and read-only recovery validation |
| Pagination completeness | Cursor loop/budget/cancellation tests, retryability/catalog agreement, and no-partial-result assertion |
| Public paged continuation | Catalog validation of one exact same-kind optional input/top-level output binding, non-`not_applicable` coverage, JSON-only presentation, and agent-help/reference-workflow projection |
| Retry safety | Timeout/attempt/idempotency validation and adapter contract tests |
| Rate evidence versus replay permission | Fault validation permits positive `retry_after` on non-retryable rate limits only, plus text/JSON projection tests |
| Executable command inputs | Catalog validation, one shared typed parser, handler integration tests, and exact human/agent-help input projection |
| Catalog-derived shell completion | Typed `InputCompletion` validation, command/flag/value planner tests, validated Workspace Template/Context/Runtime application candidates, bounded hostile-TSV canaries, generated-adapter checks, and catalog-derived first-use zero-write coverage |
| Agent recovery | Catalog fault declarations, exact-path/help-selector executable grammar tests, and structured error snapshots |
| Bounded agent discovery | Fixed root-index shape, 512-byte per-command entry validation, 100-command growth/selection tests, and a derived-scale grouped-workflow whole-response budget with edge-equivalence checks |
| Bounded-autonomy adoption | Agent-readiness first-use and denial-to-retry transcripts record command count, discovery rounds, external-processing count, and the concrete next action; a reviewed human-handoff scorecard identifies setup friction as product evidence |
| Task-scoped agent assistance | Public Runtime/policy Catalog and production-handler reachability; deterministic fresh-root absence of agent selection; color/no-color inline selector and handoff; pre-mutation Back/cancel; Runtime-assist negative CWD/Workspace/Context observation with installation standard Runtime and per-agent Home; policy omission through persisted current Context, explicit `--context` override precedence without selector mutation, and exact Context Runtime/Memory round trips with negative CWD canaries; target editable-source digest in Runtime draft identity; lifecycle/store-fenced image-ID resolution; disconnected immutable-ID-only container create and complete-role inspection; direct-egress single task-owned Home mount; selected-agent native-login bridge; fixed initial positional prompts; scoped Workspace/Configurator attachment exclusion; metadata-first task reservation, same-agent retained-draft exclusion, durable frozen submission settlement, post-materialization Runtime-preparation failure classified as task-scoped retained material, and post-publication generation recovery; immutable target freeze; Runtime source-only base CAS with typed drift and zero build/revision mutation; exact deterministic V2 pre-release aggregate receipt exclusion without adoption or deletion; shared pre-mutation readiness fault parity between implementation and both assist Catalog contracts; exact policy knowledge-pack bytes, read-only Policy Memory evidence, non-policy equality rejection, one-Context Stage, Plan/fingerprint/final-confirmation Apply, cancellation discard, and crash replay; hostile output projection; selected native-state-root and environment contracts; bounded manual native-login compatibility observation; plus cold release-binary deterministic Manual setup, entry, re-entry, and descendant/nested selection |
| Work-packet lifecycle consistency | Repository validation of finite status, terminal-retention rule, all GFM completion checkboxes, CommonMark fence handling, explicit non-template acyclic supersession, and regular-file paths |
| Local Go consistency | Gate preflight comparison of required/reported/compiler versions and GOROOT/GOTOOLDIR, with a mixed-installation shell fixture |
| External text structure | Visible-projection unit/E2E tests plus scoped I/O trust metadata; printable meaning remains explicitly out of scope |
| Documentation locale | Versioned project policy, exact schema-1 validation diagnostic, locale preservation test, and narrow English/Japanese trusted-Markdown fixtures; broader linguistic conformance remains manual |
| Public resource vocabulary | Catalog/result exact-key tests, frozen lifecycle and policy presentation fixtures, and an AST/document vocabulary check that rejects duplicate lifecycle uses of Tobari while allowing product, executable, and ownership phrases |
| Public-site locale parity | Source-derived English/Japanese route equality, substantive-content/heading/evidence/machine-example parity, built `html` language and alternate-link checks under root and Pages bases, plus same-topic browser navigation |
| Public capability coverage | Exact bidirectional match between capability ledger and catalog `CapabilityID` values |
| External schema compatibility | Vendored fixture, generator, and drift test |
| Secret or private-data exclusion | Repository policy, scanner, and synthetic fixtures |

For the task-assistance row, the required browser-control negative is exercised at
Runtime preparation, before image pull/build or any later Docker call. The
post-ready loss canaries cover both the Docker control process and the host
request/response relay for ordinary Workspace and Configurator attachments.
Clean cancellation/bridge loss remains confirmed retained material through the
mutation Invoker; bounded immutable-ID cleanup exhaustion is separately fixed
as `configuration_cleanup_incomplete` with partial change state.
| Reproducible generation | Regenerate and require a clean diff |
| Artifact integrity | Deterministic multi-entry packer, independent reopen verifier, exact supporting-file extraction, build metadata inspection, checksums, and install tests |
| Documentation command | Execute or parse the canonical snippet where practical |

If no practical mechanical check exists, state the manual review step and why automation is not reliable. Do not describe a manual convention as mechanically guaranteed.

## Adding an invariant

1. State the invariant and the failure it prevents in the governing document.
2. Identify the smallest code mutation that would violate it.
3. Put validation at the narrowest shared boundary.
4. Add a test or lint fixture that fails for the mutation.
5. Give the failure an actionable message with file, rule, and next step.
6. Add the check to the appropriate `scripts/check.sh` profile.
7. Confirm local Task and CI paths exercise the same implementation.

Do not add a grep that checks only whether a function name exists when the real claim concerns behavior. Prefer types, AST analysis, runtime validation, and contract tests in that order of semantic strength.

## Generated and automated changes

Generation is allowed when it reduces hand-maintained duplication without making the public product dynamic at runtime.

- Inputs and tool versions are reviewed and pinned.
- Generated output is committed only when repository policy requires it.
- Regeneration is deterministic.
- Generated code cannot register public commands implicitly.
- Generated schema fixtures must retain reviewed provenance and license metadata and an exact manifest digest.
- Automated updates use pull requests and the same profiles as human changes.
- A passing generator does not classify a new capability or side effect on behalf of a reviewer.

## Failure handling

A failed check is a work item, not an obstacle to bypass. Fix the implementation or, when policy is wrong, update the governing decision and its enforcement together. Do not:

- delete a negative test without replacing its guarantee;
- add a broad lint exclusion;
- switch a pinned tool to `latest` to obtain a passing result;
- make CI and local checks silently diverge;
- suppress output that a contributor needs to act on the failure.

Record nondeterministic failures with inputs, platform, and logs in the active work packet before changing timeouts or retries.

## Completion rules

- Ordinary implementation: `task check`
- Security boundary or dependency: `task check` and `task security`
- Public repository change: `task check` and `task public:check`
- Release or packaging change: `task check` and `task release:check`
- First public release: all profiles, plus the manual review in [Public Repository](05_public_repository.md)
