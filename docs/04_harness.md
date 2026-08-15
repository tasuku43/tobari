# Harness

The harness is the executable counterpart of the theses, product contract, architecture, security model, and release policy. Its goal is not to maximize the number of tools. Its goal is to make important regressions fail through one understandable interface.

## One gate, several profiles

`./scripts/check.sh` is the canonical check implementation. Every other entry point delegates to it.

| Profile | Task alias | Intended use | Includes |
|---|---|---|---|
| `fast` | `task check:fast` | Short local feedback loop | Formatting, architecture checks, capability/schema contracts, focused unit and contract tests, generated-site drift/type/link checks, and root plus Pages-base static builds |
| `full` | `task check` | Required implementation gate | Fast profile plus browser accessibility/interaction/responsive checks, vet, race, tidy/diff checks |
| `security` | `task security` | Security and dependency changes | Repository guard, module integrity, pinned static and vulnerability analysis, and the site runtime-resource/tracking/credential source guard |
| `release` | `task release:check` | Packaging and release changes | Artifact, metadata, checksum, Formula, and workflow contracts |
| `public` | `task public:check` | Public publication | Project metadata, forbidden-data, required-file, license, capability/schema contracts, public-boundary checks, generated references, and the deployable Pages artifact |
| `policy` | `task policy:test` | Rego feedback | Pinned OPA format check and unit tests |
| `gateway` | `task gateway:test` | Enforcement-point feedback | Source-built Gateway image with hash-locked dependencies, exact runtime-input snapshot membership/bytes, existing addon tests, and bounded GraphQL parser tests |
| `authbroker` | `task authbroker:test` | Credential-boundary feedback | Exact runtime-input snapshot membership/bytes, strict broker/provider/root-key Go tests, Python daemon/vault/protocol tests in the pinned image environment, and Auth Broker image metadata |
| `integration` | `task integration:test` | Real runtime boundary | One Gateway/OPA/Auth Broker shared across multiple Contexts, explicit/transparent HTTP parity, guarded routes and forwarding-off inspection, synthetic DNS and zero-pre-policy-upstream canaries, same-root and overlapping-root Tobari, Context/project source-principal and handle separation, separate network, home, runtime, policy, and credential boundaries, shared host-file visibility, typed/redacted denial, Context-local learning/reset, exact marker-absence fallback, broker restart/rotation/logout, down/purge authentication-state preservation, V1 restart, recovery, and cleanup scenarios |
| `runtime` | `task runtime:test` | Complete container gate | Policy, Gateway, Auth Broker image/protocol, and integration coverage |

The integration script reports named phase start/completion and elapsed time
for preflight, fixture build, Context/cluster, credentials/Workspaces,
Gateway/Broker/transport, policy review/activation, diagnostics, and lifecycle
coverage. Unexpected failures name the active phase before bounded container
diagnostics, while one cleanup owner still controls the complete fixture.

The optional `task toolbox:build` workflow is not a completion profile. It
requires Docker and the official Tobari runtime base, downloads the
version-pinned `kubectl`, `cwk`, `pup`, and local-only TWG artifacts, builds
`tobari-toolbox:local`, validates inherited runtime metadata, and executes each
named tool plus the inherited GitHub CLI and AWS CLI. The base runtime check
continues to verify its pre-change Git, HTTP, JSON, Python, SSH, GitHub CLI, and
AWS CLI baseline. The fast profile statically checks toolbox versions, official
sources, per-platform integrity, local license/notices, final user, and the
inherited entrypoint contract. It also fails if a toolbox-only tool enters the
published base through this capability.

`task build` is a contributor feedback path, not a completion profile. It
builds or reuses local Tobari-managed component images whose tags contain the
exact embedded source hash, builds `tobari-runtime:dev`, and compiles
`bin/tobari` with the development resolver. `task build:dev` retains the
compatibility-named `bin/tobari-dev` output. To run the integration script
against that compatibility binary, set `TOBARI_INTEGRATION_BINARY=$PWD/bin/tobari-dev` and
`TOBARI_INTEGRATION_CUSTOM_BASE=tobari-runtime:dev`.
The integration script owns its dev-resolver prerequisites: when
`tobari-runtime:dev` is absent, it creates a temporary alias to the selected
compatible integration base and removes only that alias during cleanup. It
never overwrites or deletes a contributor's pre-existing dev tag.

Both repository build tasks embed only `git rev-parse --verify HEAD` as the
source commit while retaining the fixed `dev` version; `-buildvcs=false` and
`-trimpath` exclude implicit VCS state and local paths. Build-identity tests
fix version JSON schema 1, require `unknown` metadata to remain incompatible,
and compile both resolver tags. Published packaging requires one validated
component lock and exposes its selected APIs with no repository recovery value;
the development fixture exposes matching source APIs plus exactly `task build`
and `bin/tobari`.

The current contributor sources expect `io.tobari.gateway-api=1` and
`io.tobari.auth-broker-api=1`, including guarded transparent routing,
synthetic DNS, and schema-1 source principals. `task build` is the matching
local image path. `versions.env` contains only source inputs and no generated
Tobari-owned release output. The release profile validates strict paired lock
generation, derives both APIs from the canonical Dockerfiles, and requires the
same source revision for both component indexes and every CLI archive.

The focused `task runtime:base:check` workflow validates the canonical
`runtimes/base` metadata and per-platform artifact lock, the Dockerfile's common
tool, integrity, redistribution/license, and runtime contracts, and byte
equality with the embedded CLI snapshot. The
main-only runtime workflow runs this check before its package-write job and
pushes only the base image; pull-request CI has no package-write permission.

`task gateway:source:check` validates exact membership and byte equality for
the canonical Gateway Dockerfile, `.dockerignore`, and Dockerfile-declared
image inputs in the embedded snapshot. Tests and contributor documentation
remain canonical-only. `task gateway:test` runs the
Gateway unit suite against the canonical source, while the runtime integration
continues to exercise the embedded snapshot used by the CLI. The Gateway unit
contract fixes ordinary body-free and declared GraphQL-derived OPA documents
with trusted source-bound Context/project principal, explicit/transparent
authority parity, synthetic-destination replacement only after allow, zero
external DNS/upstream calls on denial or unsupported protocols,
null-versus-provider authorization
metadata, strict decision fields, authorization-before-forward ordering,
exact endpoint classification, bounded parser behavior, broker
deny-before-resolution, and secret redaction. The Gateway
image
workflow builds both supported architectures; only its main-push publish job
has package-write permission, and its pull-request validation job is
cache-only. Runtime tests preflight the immutable Gateway digest, labels,
entrypoint, default user, Docker Engine platform, and selected runtime base
before shared resources; local source image feedback is covered by the explicit
contributor `task build` path.

`task authbroker:source:check` and `scripts/check-authbroker-source.sh`
validate exact membership and byte equality for the canonical Auth Broker
Dockerfile, `.dockerignore`, and Dockerfile-declared image inputs in the
embedded snapshot. Tests and contributor documentation remain canonical-only.
The canonical Python unit suite
runs in the pinned image environment and covers strict schema-1 control/runtime
protocols, locked startup, the schema-1 envelope/schema-1 static payload,
Context/project-bound handles, exact static introspection/resolution, restart
unlock, rotation, logout, retired-operation rejection, and secret-free
failures. The image check
validates provider-CLI absence, Docker labels, fixed entrypoint, and non-root
user. Its workflow builds
both supported architectures; pull requests are validation/cache-only, while
only the main-push job has package-write permission.
`task authbroker:image:check` is the focused image metadata/artifact check, and
`task authbroker:source:sync` is the explicit maintainer operation that
refreshes the embedded snapshot.
The version contract requires every production image selector, including Auth
Broker, to be an immutable digest reference. Component-lock tests reject a
partial lock, moving tag, wrong repository, malformed digest, source mismatch,
or incomplete platform set. The
reviewed Linux amd64/arm64 manifest receives the same runtime preflight and
ordinary immutable-image checks as Gateway.

Authentication coverage spans Go, Python, Gateway, image content, and
dependency checks. Reviewed host-driver tests fix argv/environment, canonical
executable identity/digest recheck, private state/PTY cleanup, bounded
browser/output behavior, cancellation, typed capture, and redacted errors.
Negative tests prove managed profiles, owner-selected dynamic plans, arbitrary
helpers, compatibility readers, and Broker provider CLIs are absent.

The focused Claude and Codex runtime checks validate their pinned agent
artifacts at Claude Code 2.1.220 and Codex 0.146.0 and their inherited contract.
Their local build fixtures also replace `/var/lib/tobari` with a temporary home
mount and execute the agent commands, so an image-layer executable cannot
silently depend on the persistent home. A Context custom-runtime smoke test may
compose both local artifacts and run both version commands. None of these
checks authorizes publication: agent tags stay local while redistribution and
license review is pending.

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
./scripts/check.sh integration
./scripts/check.sh runtime
```

Every profile starts with a local-toolchain preflight after the gate sanitizes its Go environment. The preflight requires the exact Go version declared by `go.mod` under `GOTOOLCHAIN=local` and verifies the selected binary, its reported version, `GOVERSION`, `GOROOT`, `GOTOOLDIR`, and the compiler in that tool directory as one installation. A mismatch fails once with those values and remediation guidance before formatting, tests, downloads, or release builds begin.

The `fast`, `full`, `security`, and `public` profiles also require the exact
Node.js version in `.node-version` and npm version in the public site's
`packageManager` field. CI and release preflight provision both through one
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

CI is the completion authority. Pull-request CI runs `full`, `runtime`, and the
security/public boundary profiles in parallel, so the canonical full gate owns
the pull-request site build and browser tests exactly once. The Pages workflow
runs the same canonical site gate only for a push to `main` or a manual replay;
only a successful push to `main` may upload `dist/` and enter the least-
privilege Pages deploy job. The repository installs no
automatic Codex Stop hook: a per-turn gate adds latency and does not prove
completion. Optional local automation must delegate to one named profile and
must not claim equivalence to a profile it did not run.

## Adoption is a product check

The harness validates more than an intact security boundary. It also checks
whether the boundary is usable enough to be selected for ordinary autonomous
work. The agent-readiness scenario records the current first-use path and the
denial-to-retry path, including discovery rounds and undeclared external
processing. A passing policy test with a workflow that requires users to become
Docker or OPA operators is evidence for a thesis revision or follow-up UX
slice, not evidence that the product outcome is complete.

The desired routine journey remains CWD-first: enter a reusable Tobari, work
freely within the declared root and network boundary, receive a useful
secret-free denial, approve the minimum exact permission through a trusted host
action, and retry. Human presentation may simplify this journey only while the
catalog, opaque-reference, effect, mutation, and fail-closed contracts remain
unchanged.

Auth Broker readiness is split deliberately. The required agent-readiness
scenario delegates its reproducible synthetic authentication proof to `task
integration:test`; that command is required evidence, not an optional adjacent
check. It uses synthetic credentials, mocked host GitHub CLI results, and local
HTTP fixtures and makes no live provider call. The manual transcript does not
duplicate synthetic broker manipulation.

A release candidate also receives manual trusted-host GitHub validation. Run
`auth login --provider github` with a test account, inspect secret-free `auth status`, and
re-enter a Context-bound Workspace. Inside it, perform the following no-print
shape and equality checks before the allowed API request:

```sh
case "${GH_TOKEN-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$(gh auth token --hostname github.com)" = "$GH_TOKEN"
gh api user --jq .login >/dev/null
```

The successful equality check proves `gh auth token` returns the exact
projected handle, not the primary credential. Then run `auth logout github` on
the host and prove the old handle fails. The reviewer records only pass/fail
and secret-free outcomes outside the repository. Tokens, device codes, vaults,
handles, and raw authenticated transcripts are prohibited evidence.

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
locale. Its English-locale check is the narrow trusted-Markdown Japanese-script
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
  validation rejects every text-producing command that omits the shared
  semantic-token presentation declaration. The
  Workspace selector tests cover the dependency-free raw key state machine,
  bounded scrolling, no redraw during repeated idle VTIME polls, exactly-once
  terminal restoration, English status rendering, and ANSI-free numbered-input
  fallback. Catalog routing tests cover bare canonical namespace help plus no
  more than three deterministic, exact-selector typo suggestions. Interactive
  entry tests preserve the
  child exit status, assert that the Workspace remains logically existing
  after the child returns, and keep the resume/delete summary on host stderr
  rather than child stdout.
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
  unconfigured resources, Context report
  schema 1, Context list schema 1, Workspace status schema 1, and auth
  result/status schema 1, including explicit Context persistence state, null
  pre-authority IDs/stores, the complete shell
  environment inventory, atomic Git identity policy, explicit empty literal
  values, explicit empty provider collections, and null account labels.
  Help's catalog fields describe root `view: index`; separate exact-key tests
  cover both that view and the input-selected `view: scope` variant. The
  ledger-backed machine-output fixture and presentation-independent answer key
  preserve empty collections, explicit null, false, zero, unavailable states,
  nested arrays, structured recovery, and a routine-success external-processing
  count of zero.
- Auth truth-table tests freeze current, missing, stale, unavailable,
  unresolved, zero-Workspace, configured-with-current-projection, changed, and
  no-change states independently of presentation. Infrastructure tests prove
  exact Context/project/revision/binding correlation, deterministic bounded
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
- CWD-owned lifecycle integration tests prove that two roots receive distinct
  containers, networks, and XDG homes while sharing only Gateway, OPA, and
  public CA state. They also prove canonical ancestor lookup, container/network
  recovery, explicit ancestor Workspace selection and current-CWD creation,
  one-Workspace-per-canonical-root behavior under repeated and concurrent
  creation, profile/spec drift recreation, concurrent entry convergence, and
  exact selected deletion after partial runtime cleanup. A child `exit` is
  verified as session detachment, not Workspace deletion; detached `delete` is
  the normal external cleanup path and `delete --force` is the explicit
  attached-session override.
- Lifecycle target-safety tests exercise both Context flag placements for root
  entry, status, and delete; reject duplicate, explicit-empty, unknown, and
  stale selectors before handler, Workspace, or Docker I/O; bind a force
  preview to its displayed stable Context ID; retain Context scope for an empty
  status; and prove same-root/different-Context selection through a bound
  manifest. Frozen typed status, answer-key, and text fixtures verify schema-1
  Context/attachment/next-argv semantics and the complete delete impact.
- Logical lifecycle tests inject interruptions at home, instance, root-index,
  runtime, and deletion boundaries; they prove journals recover without
  duplicate IDs and diagnose orphaned one-sided records. Shared-state tests
  prove locked atomic writes, interrupted cluster reconcile diagnosis, and
  explicit cluster network reconnects.
- Custom-image tests build from the stable local Tobari base, include a fixture
  with a terminating image `CMD`, select it through bounded configuration, and
  prove the image `CMD` cannot own Workspace lifetime. They also prove
  compatibility is checked before per-Tobari resources exist.
- Runtime-spec tests assert fixed CPU, memory, PID-count, and container-log
  options and the Tobari-owned lifetime command, include that contract in drift
  hashing, and the Docker integration scenario inspects those limits after
  creation and recovery.
- Context image tests cover manifest selection, built-in initialization, invalid image
  rejection, and the fact that project metadata cannot override the bound Context
  runtime image.
- Context runtime tests cover non-overwriting recipe initialization, the
  owner-only Docker build context, generated image naming, compatibility and
  digest inspection, source-digest drift, and unchanged image selection after
  build or promotion failure. They also assert that an exact official base
  requests `--pull` while an explicit local base does not.
- The Docker integration scenario creates the current Context recipe, runs a
  real managed build from the public official GHCR base, verifies ready status
  and automatic Context image promotion, then repeats the flow with the local
  base before cleanup.
- Policy-learning integration projects baseline and learnable denials, proves
  baseline denies stay out of the actionable queue, and exercises exact allow,
  deny, reset, and re-review activation through reference-bound commands
  without restarting any Tobari.
- Policy-candidate domain and CLI tests fold repeated and concurrently emitted
  exact denials into one pending item, retain the latest evidence, count the
  required bounded observations, keep
  Context/project/scheme/host/port/method/path differences separate, and exclude resolved
  Allow and exact Deny decisions.
- Gateway contract tests verify that a learnable denial carries only the fixed
  host-side review navigation, while non-learnable and infrastructure failures
  do not invite approval. Session lifecycle tests verify that the aggregate
  pending-permission summary stays on host stderr and is best-effort.
- Context contract tests verify stable manifest IDs, owner-only separated policy/vault boundaries,
  current-marker-only selection, exact V1 binding, and unsupported-version
  rejection. Runtime tests verify aggregate Context policy reaches one OPA, the
  bound agent profile reaches the project as read-only data, and broker vaults
  remain Gateway-inaccessible except through the Broker socket. Aggregate tests cover namespace
  reservation, secret-sensitive revisions, complete-candidate validation,
  serialization, and known-good retention.
- Auth domain and catalog tests fix exact schema-1 static providers and the
  closed reviewed GitHub/AWS/Datadog/OpenAI/Anthropic login union, exact command
  effects/inputs/outputs/failures, bounded interactive provider omission,
  explicit-provider requirements outside that selector, the AWS-only
  `identity-center|console` method axis, and rejection of unsupported providers
  or inapplicable methods. They also cover Context binding, cancellation,
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
  cross-Context/project/binding rejection, rotation, logout, restart/unlock
  rehydration, same-revision static replacement, bounded AWS signing,
  Datadog/OpenAI refresh through the exact immutable renewable-adapter union,
  shared single-flight/snapshot/CAS lifecycle, durable operation barriers,
  adapter incapability over Vault/handles/locks, the exact immutable persisted
  record-contract union, record-contract incapability over filesystem/ciphers,
  compatibility re-exports, exact immutable control-login plans, shared
  static/driver parsing, plan incapability over Broker/Vault/host execution,
  the exact immutable Gateway reviewed-profile union, Gateway-profile
  incapability over requests/policy/Broker/secrets, and rejection of every
  unknown record/operation. Provider network tests use local synthetic servers.
- Acquisition UX tests split trusted-host and Broker boundaries for
  GitHub/AWS/pup/Codex/Claude: canonical executable/digest selection, fixed
  argv, sanitized environment, bounded streams/state, checked cleanup, and
  cancellation preserve the prior credential on failure. No provider CLI
  exists in the Broker image.
- Workspace auth projection tests prove one configured Context credential
  produces distinct project-bound handles, injects only declared environment
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
- Domain-policy contract tests reject unknown fields, duplicate JSON keys,
  missing fields, duplicate rule IDs, directory/embedded-host mismatches,
  non-canonical/wildcard/IP hosts, incomplete pairs, extra files, symlinks, and
  unsafe modes. They prove new domains appear with both JSON files, unchanged
  files retain exact bytes, method authority is isolated by host, deny wins,
  and composed projection bytes are deterministic.
- Guided-policy projection tests prove a Context owns only the exact
  `policy/domains/<host>/{allow,deny}.json` tree, stale Context-local Rego fails
  closed, the shared evaluator makes first brokered use learnable until an
  exact rule is installed, and Advanced Context Rego remains isolated,
  layout-checked, and schema-checked. Source transaction tests exercise
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
  fresh and unsupported-version Context trees before and after doctor.
- The human permission path is exercised through `policy review`; its TTY
  Permission Inbox covers bounded selection, detail inspection, staged exact
  allow/deny choices, manual refresh, one final exact review and confirmed
  Apply, and pre-Apply discard. PTY tests prove
  action keys are ineffective on the list, several same-Context detail choices
  produce exactly one activation without a second yes/no prompt, a Context
  switch requires Apply or discard, and cancellation delegates nothing. Its
  presentation tests group by stable Context/project IDs, keep same-label
  different-ID scopes separate, preserve selection and staged order by
  candidate ID across refresh/reorder, remove stale choices without transfer,
  lead rows with the exact HTTP effect, and keep
  selected observation evidence visible before inspection. The
  TTY `policy rules` path separately covers exhaustive current-decision
  inventory, explicit reset confirmation, refresh, and re-review of the
  retained denial. Neither path requires hand-editing OPA or Rego. Redirected
  review and inventory stay read-only; exact reference-bound allow, deny, and
  reset actions are the routine policy mutations.
- The resumable-permission corpus pins a typed inbox, scoped empty queue, zero
  staged set, mixed HTTP/GraphQL final review, stale refresh, activation fault,
  authoritative revision receipt, and stable Workspace/OPA identities. Its
  answer key records zero routine external processing. Container integration
  keeps both container IDs stable, confirms the receipt revision against
  `cluster status`, and retries through the same running Workspace.
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
- Context/project-principal integration creates same-root and overlapping-root
  Tobari in different Contexts, checks distinct owned Workspace and Gateway
  network endpoints and non-overlapping dedicated networks,
  stable IDs and homes, shared host-file effects, and proves an atomically updated
  directory-mounted registry is visible without manually recreating Gateway,
  denies learned permission when requested by another Context/project, and
  checks registry cleanup after restart, recovery, and exact Context-bound
  deletion. It also rejects unprivileged source binding/IP_FREEBIND attempts
  and proves transparent connections select the exact source-bound principal.
  Gateway unit tests separately reject retired managed-profile selectors before
  OPA or upstream I/O.
- Auth Broker runtime integration inspects exactly one locked broker beside one
  Gateway and one OPA, verifies the separate control/runtime socket mounts and
  fixed resource/log bounds, unlocks through a synthetic host root key, issues
  different handles to Context-bound projects, proves an OPA denial performs no
  resolution, rotates and revokes handles, restarts locked, re-unlocks, and
  confirms that primary-secret canaries never appear in Workspace files,
  environment, OPA input, audit, CLI output, or component logs.
- Policy-boundary tests prove the normalized request authority and exact
  1-65535 port are required by the structured boundary, HTTPS on a non-default
  port remains learnable, invalid ports are terminal, and learned rules do not
  cross ports or schemes.
- Gateway boundary tests resolve and pin an upstream address, reject unsafe
  resolved addresses for dotted hosts, and preserve the explicit single-label
  private-service exception used by the local integration shape.
- Gateway boundary tests prove ordinary body content is absent from policy
  input, a request stream is enabled only after allow, and local denial
  responses are not treated as authorized upstream responses. Declared
  GraphQL tests prove only operation type and canonical root fields enter
  policy identity, every root is required, original bytes are forwarded once,
  unsupported envelopes fail before OPA learning or upstream I/O, and source,
  variables, arguments, and aliases remain absent from policy and audit.
- Integration tests prove body variants aggregate into one exact candidate and
  learned rule, allowed chunked uploads and SSE responses arrive incrementally,
  and the fixed 8 MiB advertised-body cap still rejects an over-limit request.
- Runtime-asset and integration tests enforce fixed JSON log rotation for the
  shared Gateway, OPA, and Auth Broker services.
- Runtime-asset and integration tests inspect the fixed shared-service CPU,
  memory-plus-swap, and PID ceilings.
- Learned-policy integration passes opaque denial candidates unchanged into
  exact allow and deny actions, verifies a neighboring request remains denied,
  resets one exact decision, and rejects every prefix-rule/compaction shape.
- Negative tests prove rejection before side effects.
- Release tests inspect actual artifacts and metadata, not only workflow text.
  Archive tests cover deterministic multi-entry order, canonical metadata,
  create-only output, regular-file identity checks, exact executable/license/
  optional-notice bytes, and independent reopen verification.
  Release lint additionally proves that only stable protected publication may
  cross into `tasuku43/homebrew-tap`, that its GitHub App token is scoped to
  that repository, that the exact published Formula is propagated, and that
  the mutation ends at a Formula-only pull request rather than a direct
  `main` push.
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
| Catalog completeness | Whole-catalog contract tests |
| Semantic terminal presentation | Ledger-pinned typed fixture and answer key across colored TTY, `NO_COLOR` TTY, redirected text, scoped empty, warning, failure, and neutral pre-action cancellation; exact six-token rendering tests; catalog-wide text-presentation declarations; idle-selector redraw/restoration tests; bounded catalog-derived namespace/suggestion tests; and AST lint that rejects style-dependent structure or direct ANSI SGR outside the shared style layer |
| Output delivery versus collection coverage | Independent finite enums and catalog tests, including complete bounded/differential windows and paged exhaustive traversal |
| Operationally closed supported outcome | Reviewed agent-readiness transcript with zero undeclared external reconstruction, plus task-owned deterministic-composition tests and declared field extraction |
| Request-bound semantic result | Per-capability domain/application tests for declared task identity and every applicable request dimension, including scope, state, contextual-kind, empty-result, no-partial-result, and negative-inference fixtures where applicable |
| Action target composition | Reachable reference-graph validation and byte-preserving round trips for reference-bound acts; complete, exclusive, reference-free declarations for command-bound fixed targets |
| Side-effect ordering | Fake adapter counters and failure-before-I/O tests |
| Ancestor Workspace choice | Typed nearest-first candidate fixtures, selector key/fallback tests, locked stale-choice checks, and zero-downstream-call cancellation tests |
| Session-versus-Workspace lifecycle | Child exit-status preservation, host stderr guidance, stdout/stderr ownership, logical-state-after-exit, and explicit delete tests |
| Attached-session deletion guard | Docker Exec ID observation, guard-before-delete negative tests, force override, and stable structured fault/help contract |
| Context-bound lifecycle target safety | Prefix/post-command equivalence, duplicate/empty parser rejection, unknown/stale zero-I/O counters, stable-manifest same-root selection, preview-ID mismatch rejection, explicit attachment states, exact Context-preserving recovery argv, and frozen presentation evidence |
| One Workspace per canonical root/Context pair | Domain duplicate-index validation, pair-hash/index checks, repeated explicit-create rejection, same-root/different-Context creation, and concurrent explicit-create convergence |
| Custom image isolation | Runtime-API inspection plus exact create-argv and Docker integration tests |
| Agent executable/home separation | Runtimechecker path assertions, local image builds with a home overlay, and Tobari smoke tests for each agent command |
| Shared runtime resource bounds | Fixed project create-argv, resource-aware spec hash, and Docker HostConfig integration assertions |
| Context runtime boundary | Context manifest tests, compatibility validation, ignored-project-metadata regression, existing-Workspace image reconciliation tests, and failure-before-state-update tests |
| Portable policy activation | Pre-mutation OPA tests, exact OPA and volume owner-label checks, owner-only archive/stdin staging without a host bind, fixed networkless volume-only builder and same-volume publisher argv, unchanged-ready publication skip, exact active revision plus defined decision readiness, stable OPA identity, invalid-bundle retention, and Docker integration |
| Context composition and selection | Stable-ID manifest/domain tests, required immutable source-access and preset origin/revision, creation-only defaults, snapshot digest binding, catalog effect/target contracts, owner-only atomic store tests, current-default-only selection, explicit synthetic/persisted observation state, first-use zero-write and concurrent/read-only XDG canaries, permanent Tobari binding, unsupported-version fixtures, and agent-readiness transcript |
| Context source-access boundary | Closed enum/default tests, exact desired-spec hash and Docker bind inspection, read/change/delete/Git-write canaries, writable home/tmpfs checks, no writable alias inventory, same-root cross-Context observation, nested home-relative roots, and unsupported-state rejection |
| Context policy-preset guardrail | Normalized owner-only snapshot/digest tests, source-change immutability, system-evaluator precedence over baseline/learned/Advanced policy, scheme-aware exact learned identity, and terminal-denial zero candidate/DNS/Broker/upstream counters |
| Context configuration interaction boundary | Catalog-derived `config shell`/`config git` help and argv dependencies, complete-direct versus wholly-omitted staged-editor mode tests, explicit-empty Context plus partial direct input and redirected/non-TTY/JSON editor rejection before mutation, raw-terminal and English line fallback, complete current/pending rendering, multi-row Shell staging with one atomic write, exact task/selected-Context correlation for reads, shown-Context binding across concurrent default changes, exact task/Context/applied-setting/cluster correlation for mutation results, pre-Apply discard, terminal restoration, stdout/stderr separation, fixed-target invoker coverage, and exact schema-1 Context result keys |
| Context shell environment boundary | Fixed allowlist and source-enum domain tests, explicit-empty preservation, exact V1 persistence, zero-I/O rejection for arbitrary names and ambiguous values, owner-only atomic update tests, complete Context report output, exact child-exec environment assertions, missing-export fallback, Bash-quote and bounded inherited-value canaries, and host-credential non-copy assertions |
| Context Git identity boundary | Atomic pair/source domain tests, exact V1 shell-setting preservation and opt-in default initialization, exact two-key global Git argv with an absolute executable and exact `HOME`/optional `XDG_CONFIG_HOME` plus fixed-control environment allowlist, project-owned config-directory and `PATH`/loader/shell-startup canaries, timeout/output/framing/unsafe-value bounds, malicious local-include exclusion, private atomic config encoding, symlink and existing-file size checks, read-only directory mount and system-scope precedence, excluded helper/signing/auth/path keys, absent/incomplete-pair behavior, and secret-/personal-data-free faults and fixtures |
| Context runtime build boundary | Fixed current-Context target contracts, owner-only recipe checks, bounded BuildKit plain-progress/load argv including official-base refresh versus local-base behavior, live visible-projected stdout/stderr diagnostics, syntax/RUN/base/daemon failure canaries, nonzero/zero exit assertions, compatibility/digest validation, source-digest status, previous-image preservation, atomic promotion tests, and bound-Context next-entry reconciliation coverage |
| Gateway source and image boundary | Canonical-source/snapshot byte comparison, pinned mitmproxy parent, signed nftables/iproute dependency inventory, canonical-source unit tests, source API-1/role labels, transparent-only listener and fixed network-guard entrypoint, explicit rejection of non-transparent ingress, absence of proxy environment/port exceptions, content-addressed development selection, paired component-lock validation, immutable digest/platform/entrypoint release preflight, non-root resident process, and validation/release workflow permission separation |
| Auth Broker source and image boundary | Canonical-source/snapshot byte comparison, canonical Python tests in the pinned image environment, provider-CLI absence including Codex/Claude, source API-1/role labels, content-addressed development selection, paired component-lock validation, immutable digest/platform/entrypoint release preflight, non-root Dockerfile, and validation/release workflow permission separation |
| Context-owned encrypted credentials | Root-key backend tests, strict owner/mode/symlink checks, AES-GCM schema/Context AAD canaries, atomic vault replacement, missing-key-with-vault rejection, and secret-free outputs |
| Authentication state survives cluster teardown | Exact down/purge resource assertions, preserved vault/key canaries, and subsequent cluster-up unlock/status proof |
| Doctor remains observational | Fixed dependency-matrix, direct-blocker, complete-report, schema-1 renderer/agent-contract, fail/warn exit, cancellation, Docker-argv allowlist, host-only policy-source validation, content-aware fresh/unsupported-version snapshots, and zero-create/zero-repair canaries across root-key, vault, provider, broker, and project-auth state |
| Every declared read remains observational | Dynamic public-catalog handler coverage, per-command fresh-XDG before/after snapshots, zero Docker-mutation argv, lockless fresh lifecycle reads, read-only/concurrent fixtures, unsupported-version fail-closed state, and bounded cleanup of only a pre-existing validated journal |
| Project-bound broker handles | Full Context/project/provider/revision/target/header round trips, hash-only live index assertions, copied/stale/rotated/revoked negative tests, exact Context-wide eligibility and next-entry semantics, and Workspace projection reconciliation |
| Agent OAuth acquisition and pinned projections | Multi-version host Codex tests require one exact compiled driver-contract revision, fixed argv/environment, canonical executable digest, and strict captured state while treating stable product version as audit metadata; exact-key/byte tests independently fix the Codex 0.146.0 Workspace `.codex/auth.json` sentinel shim with only `${HANDLE}` as `tokens.access_token`, direct synthetic Gateway bearer/account-header checks, Claude `CLAUDE_CODE_OAUTH_TOKEN=${HANDLE}`-only environment checks, API-key/auth-token absence, modified/symlinked-file refusal, and Workspace-client drift rejection; an isolated network-disabled client observation records login-status and verbatim-handle behavior but remains a required manual release replay rather than an automated artifact claim |
| Declared provider bindings require Broker | Direct bearer/raw and AWS SigV4 canaries assert `broker_auth_required`, secret-header removal, and zero fallback/Broker/OPA/DNS/upstream calls; one undeclared-binding canary retains post-policy compatibility passthrough |
| Broker fallback requires an undeclared binding and marker absence | URL/path/query/fragment/header-name/value marker canaries, malformed/ambiguous/binding-mismatch rejection, and compatibility passthrough tests with no declared binding or marker anywhere inspected |
| Post-policy credential action | Gateway call-order/count tests for handle removal, introspect-before-OPA, zero resolve/refresh/companion/signing on deny, one same-revision reviewed action after allow, exact header replacement/signing, and no-secret canaries |
| Closed broker plan boundary | Exact static and dynamic record schemas, immutable renewable-adapter, request-signing-mechanics, persisted-record-contract, control-login-plan, and Gateway reviewed-profile membership; one strict versioned test-only capability fixture checked from Go and Python for built-in/acquisition/manifest/login/record/runtime/Gateway parity; adapter/plan/profile incapability tests, record/Vault import compatibility, shared Broker-owned login commit and snapshot/single-flight/barrier/CAS conformance, Context/project/provider/revision/HTTPS-header or signing introspection, bounded AWS/Datadog/OpenAI behavior, rotation/revocation, durable barriers, and no invalid-handle fallback |
| Unsupported broker capabilities stay absent | Catalog, state-parser, dependency, image-content, and hostile-header tests reject managed profiles, owner-selected dynamic plans, arbitrary helpers, compatibility readers, and provider CLIs inside Broker |
| Protected provider acquisition | Catalog selector/method/stdin contracts, terminal rules, bounded readers, canonical GitHub/AWS/pup/Codex/Claude identity/digest checks, fixed argv/environment, control-safe output, bounded browser/PTY behavior, checked cleanup, cancellation/failure preservation, synthetic integration, and manual live validation |
| Typed denial recovery | Strict host/port audit projection, query/header absence, whole-path handle-marker redaction, non-learnable structural rejection, fixed host-review navigation schema, host-stderr session summary, empty bounded scope, hostile-field canaries, and end-to-end JSON assertions |
| Explicit policy learning | OPA scheme/port learnability classification, terminal deny exclusion, deterministic repeated/concurrent Context/project/scheme/host/port/method/path candidate aggregation with required latest/count, Context-scoped reference validation, single-reference allow/deny/reset round trips, bounded typed TTY staging with one fixed-target Apply and zero-write discard, installation-wide inventory/review, aggregate preflight ordering, and Docker retry |
| Declared GraphQL policy identity | Exact trusted endpoint projection, hash-pinned parser and license checks, strict bounded envelope fixtures, conservative root-fragment expansion, all-roots OPA matching, HTTP-rule non-matching canaries, per-root audit/candidate/allow/deny/reset round trips, prefix-rule rejection, raw-body privacy canaries, and zero-upstream integration |
| Context source access | Required manifest/catalog field, omission-default tests, immutable runtime spec/hash, Docker inspect/reconciliation, read-only mutation/Git-metadata denial, writable home/tmpfs, no writable alias, and same-root observation |
| Context policy preset | Strict normalization/digest/snapshot tests, built-in and custom schema canaries, guardrail precedence, GET-without-safe-claim contract, and terminal zero-candidate/DNS/Broker/upstream calls |
| Context/project principal and credential scope | Owner-only atomic registry schema 1, exact Workspace-source/Gateway endpoint and network uniqueness, regular/transparent source derivation, forged-Context/project/SNI/authority and unknown-principal denial, source-spoof canaries, passthrough/static-broker tests, copied-handle cross-Context canaries, Rego canaries, and multi-Context Docker integration |
| Atomic multi-Context policy activation | Source and projection locks, Context namespace rejection, complete all-Context OPA validation, content-addressed atomic publication, stale-revision rejection, known-good rollback, and invalid/concurrent mutation tests |
| Mutation outcome classification | Structured-fault-first/cause-stripping tests, non-retryable unclassified outcome fallback, and read-only recovery validation |
| Confirmed mutation output | One effect-aware finalizer, late-cancellation regression, non-retryable mutation short-write fault, and read-only recovery validation |
| Pagination completeness | Cursor loop/budget/cancellation tests, retryability/catalog agreement, and no-partial-result assertion |
| Public paged continuation | Catalog validation of one exact same-kind optional input/top-level output binding, non-`not_applicable` coverage, JSON-only presentation, and agent-help/reference-workflow projection |
| Retry safety | Timeout/attempt/idempotency validation and adapter contract tests |
| Rate evidence versus replay permission | Fault validation permits positive `retry_after` on non-retryable rate limits only, plus text/JSON projection tests |
| Executable command inputs | Catalog validation, one shared typed parser, handler integration tests, and exact human/agent-help input projection |
| Agent recovery | Catalog fault declarations, exact-path/help-selector executable grammar tests, and structured error snapshots |
| Bounded agent discovery | Fixed root-index shape, 512-byte per-command entry validation, 100-command growth/selection tests, and a derived-scale grouped-workflow whole-response budget with edge-equivalence checks |
| Bounded-autonomy adoption | Agent-readiness first-use and denial-to-retry transcripts record command count, discovery rounds, external-processing count, and the concrete next action; a reviewed human-handoff scorecard identifies setup friction as product evidence |
| Work-packet lifecycle consistency | Repository validation of finite status, terminal-retention rule, all GFM completion checkboxes, CommonMark fence handling, explicit non-template acyclic supersession, and regular-file paths |
| Local Go consistency | Gate preflight comparison of required/reported/compiler versions and GOROOT/GOTOOLDIR, with a mixed-installation shell fixture |
| External text structure | Visible-projection unit/E2E tests plus scoped I/O trust metadata; printable meaning remains explicitly out of scope |
| Documentation locale | Versioned project policy, exact schema-1 validation diagnostic, locale preservation test, and narrow English/Japanese trusted-Markdown fixtures; broader linguistic conformance remains manual |
| Public-site locale parity | Source-derived English/Japanese route equality, substantive-content/heading/evidence/machine-example parity, built `html` language and alternate-link checks under root and Pages bases, plus same-topic browser navigation |
| Public capability coverage | Exact bidirectional match between capability ledger and catalog `CapabilityID` values |
| External schema compatibility | Vendored fixture, generator, and drift test |
| Secret or private-data exclusion | Repository policy, scanner, and synthetic fixtures |
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
