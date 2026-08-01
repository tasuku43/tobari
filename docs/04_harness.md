# Harness

The harness is the executable counterpart of the theses, product contract, architecture, security model, and release policy. Its goal is not to maximize the number of tools. Its goal is to make important regressions fail through one understandable interface.

## One gate, several profiles

`./scripts/check.sh` is the canonical check implementation. Every other entry point delegates to it.

| Profile | Task alias | Intended use | Includes |
|---|---|---|---|
| `fast` | `task check:fast` | Short local feedback loop | Formatting, architecture checks, capability/schema contracts, focused unit and contract tests |
| `full` | `task check` | Required implementation gate | Fast profile plus vet, race, tidy/diff checks |
| `security` | `task security` | Security and dependency changes | Repository guard, module integrity, pinned static and vulnerability analysis |
| `release` | `task release:check` | Packaging and release changes | Artifact, metadata, checksum, Formula, and workflow contracts |
| `public` | `task public:check` | Public publication | Project metadata, forbidden-data, required-file, license, capability/schema contracts, and public-boundary checks |
| `policy` | `task policy:test` | Rego feedback | Pinned OPA format check and unit tests |
| `gateway` | `task gateway:test` | Enforcement-point feedback | Pinned mitmproxy addon unit tests |
| `integration` | `task integration:test` | Real runtime boundary | Shared-cluster lifecycle, multiple CWD-owned Tobari, host-issued project principal separation, network separation, TLS, fail-closed, credential, typed denial, tested host-policy activation, entry, recovery, and cleanup scenarios |
| `runtime` | `task runtime:test` | Complete container gate | Policy, Gateway, and integration profiles |

The optional `task toolbox:build` workflow is not a completion profile. It
requires Docker and the locally materialized `tobari-runtime:local` base,
downloads the version-pinned specialized CLI artifacts, builds
`tobari-toolbox:local`, validates inherited runtime metadata, and executes each
named tool. The base runtime check separately verifies the common Git, HTTP,
JSON, Python, SSH, GitHub, and AWS tool contract. The fast profile statically
checks that versions, official sources, integrity checks, final user, and the
inherited entrypoint contract cannot silently disappear.

The focused `task runtime:base:check` workflow validates the canonical
`runtimes/base` metadata and digest lock, the Dockerfile's common tool and
runtime contract, and byte equality with the embedded CLI snapshot. The
main-only runtime workflow runs this check before its package-write job and
pushes only the base image; pull-request CI has no package-write permission.

The focused Claude and Codex runtime checks validate their pinned agent
artifacts and inherited contract. Their local build fixtures also replace
`/var/lib/tobari` with a temporary home mount and execute the agent commands,
so an image-layer executable cannot silently depend on the persistent home.

Direct invocation is supported for automation:

```sh
./scripts/check.sh fast
./scripts/check.sh full
./scripts/check.sh security
./scripts/check.sh release
./scripts/check.sh public
./scripts/check.sh policy
./scripts/check.sh gateway
./scripts/check.sh integration
./scripts/check.sh runtime
```

Every profile starts with a local-toolchain preflight after the gate sanitizes its Go environment. The preflight requires the exact Go version declared by `go.mod` under `GOTOOLCHAIN=local` and verifies the selected binary, its reported version, `GOVERSION`, `GOROOT`, `GOTOOLDIR`, and the compiler in that tool directory as one installation. A mismatch fails once with those values and remediation guidance before formatting, tests, downloads, or release builds begin.

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
security/public boundary profiles in parallel. The repository installs no
automatic Codex Stop hook: a per-turn gate adds latency and does not prove
completion. Optional local automation must delegate to one named profile and
must not claim equivalence to a profile it did not run.

## Harness components

### `.harness/project.json`

This schema-versioned file is the machine-readable source for Tobari identity,
release metadata, and repository policy. Schema 3 contains no lifecycle
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
list markers. Metadata is read only from the contiguous top-level
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
- [`.harness/schemas.json`](../.harness/schemas.json) pins publishable external-schema fixtures by repository-relative path and exact SHA-256 digest. Each entry also records provenance and license. An explicit empty array is valid before the project adopts an external schema.

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
- CLI tests fix routing, help, rendering, exit behavior, the catalog-owned typed
  argv parser, and the distinction among absent, defaulted, and explicitly
  supplied values. Negative fixtures cover type/range/enumeration,
  repeatability, dependency/conflict, duplicate scalar, and syntax drift. The
  Workspace selector tests cover the dependency-free raw key state machine,
  bounded scrolling, terminal restoration, English status rendering, and
  ANSI-free numbered-input fallback. Interactive entry tests preserve the
  child exit status, assert that the Workspace remains logically existing
  after the child returns, and keep the resume/delete summary on host stderr
  rather than child stdout.
- Agent-help shape, edge-equivalence, and derived-scale size tests keep root
  discovery index-only while grouped scoped workflows retain the complete
  invocation, reference, and recovery contract without producer/consumer
  Cartesian growth.
- JSON-output contract tests compare each single-shape built-in renderer's
  schema version, envelope, and item keys with its catalog `CommandOutput`
  declaration and enforce the always-present string cursor for any paged probe.
  Help's catalog fields describe root `view: index`; separate exact-key tests
  cover both that view and the input-selected `view: scope` variant.
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
- Dev Container tests cover comments, trailing commas, duplicate keys, size and
  symlink escape, unsupported runtime properties, input conflicts, and one
  image-based Docker integration path.
- Policy-learning integration projects a denial, edits the host XDG policy and
  its test, activates it through the fixed-target command, and observes the
  changed decision without restarting any Tobari.
- Project-principal integration creates two current-directory Tobari, checks
  distinct Gateway network addresses, denies a credential profile and learned
  permission when requested by the other project, and checks registry cleanup
  after network recovery and deletion.
- Policy-boundary tests prove the normalized request port is required by the
  initialized scheme-port allowlist, rejected non-default ports are not
  learnable, and learned rules do not cross ports.
- Gateway boundary tests resolve and pin an upstream address, reject unsafe
  resolved addresses for dotted hosts, and preserve the explicit single-label
  private-service exception used by the local integration shape.
- Gateway boundary tests distinguish an explicitly empty body from an
  unavailable streamed body and deny the latter before OPA can allow it.
- Runtime-asset and integration tests enforce the fixed 8 MiB mitmproxy body
  cap before request/response forwarding.
- Runtime-asset and integration tests enforce fixed JSON log rotation for both
  shared Gateway and OPA services.
- Runtime-asset and integration tests inspect the fixed shared-service CPU,
  memory-plus-swap, and PID ceilings.
- Learned-policy integration also passes an opaque denial candidate unchanged
  into exact approval, verifies a neighboring request remains denied, creates
  three exact siblings, passes one opaque compaction candidate unchanged into
  compaction, and rechecks both retained positives and the outside-prefix
  canary.
- Negative tests prove rejection before side effects.
- Release tests inspect actual artifacts and metadata, not only workflow text.
  Archive tests cover deterministic multi-entry order, canonical metadata,
  create-only output, regular-file identity checks, exact executable/license/
  optional-notice bytes, and independent reopen verification.
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
| Finite domain state | Types, constructors, and table-driven negative tests |
| Catalog completeness | Whole-catalog contract tests |
| Output delivery versus collection coverage | Independent finite enums and catalog tests, including complete bounded/differential windows and paged exhaustive traversal |
| Operationally closed supported outcome | Reviewed agent-readiness transcript with zero undeclared external reconstruction, plus task-owned deterministic-composition tests and declared field extraction |
| Request-bound semantic result | Per-capability domain/application tests for declared task identity and every applicable request dimension, including scope, state, contextual-kind, empty-result, no-partial-result, and negative-inference fixtures where applicable |
| Action target composition | Reachable reference-graph validation and byte-preserving round trips for reference-bound acts; complete, exclusive, reference-free declarations for command-bound fixed targets |
| Side-effect ordering | Fake adapter counters and failure-before-I/O tests |
| Ancestor Workspace choice | Typed nearest-first candidate fixtures, selector key/fallback tests, locked stale-choice checks, and zero-downstream-call cancellation tests |
| Session-versus-Workspace lifecycle | Child exit-status preservation, host stderr guidance, stdout/stderr ownership, logical-state-after-exit, and explicit delete tests |
| Attached-session deletion guard | Docker Exec ID observation, guard-before-delete negative tests, force override, and stable structured fault/help contract |
| One Workspace per canonical root | Domain duplicate-index validation, root-hash/index checks, repeated explicit-create rejection, and concurrent explicit-create convergence |
| Custom image isolation | Runtime-API inspection plus exact create-argv and Docker integration tests |
| Agent executable/home separation | Runtimechecker path assertions, local image builds with a home overlay, and Tobari smoke tests for each agent command |
| Shared runtime resource bounds | Fixed project create-argv, resource-aware spec hash, and Docker HostConfig integration assertions |
| Dev Container boundary | Bounded JSONC/path tests, application rejection, and catalog input conflicts |
| Portable policy activation | Pre-mutation OPA tests, exact owner-label check, OPA-only recreation argv, and Docker integration |
| Typed denial recovery | Strict host/port audit projection, empty bounded scope, hostile-field canaries, and end-to-end JSON assertions |
| Explicit policy learning | OPA scheme/port learnability classification, project/host/port/method/path candidate/reference domain validation, discover-act graph and round trip, strict atomic XDG writer, preflight ordering, and Docker retry |
| Bounded policy compaction | Pure deterministic same-project/host/port/method grouping, minimum evidence and path-depth invariants, positive/boundary OPA tests, stale-reference rejection, and Docker canary |
| Project principal and credential scope | Owner-only atomic registry schema, local-interface derivation, forged-session and unknown-principal denial, profile project binding, cross-project Rego canary, and two-project Docker integration |
| Mutation outcome classification | Structured-fault-first/cause-stripping tests, non-retryable unclassified outcome fallback, and read-only recovery validation |
| Confirmed mutation output | One effect-aware finalizer, late-cancellation regression, non-retryable mutation short-write fault, and read-only recovery validation |
| Pagination completeness | Cursor loop/budget/cancellation tests, retryability/catalog agreement, and no-partial-result assertion |
| Public paged continuation | Catalog validation of one exact same-kind optional input/top-level output binding, non-`not_applicable` coverage, JSON-only presentation, and agent-help/reference-workflow projection |
| Retry safety | Timeout/attempt/idempotency validation and adapter contract tests |
| Rate evidence versus replay permission | Fault validation permits positive `retry_after` on non-retryable rate limits only, plus text/JSON projection tests |
| Executable command inputs | Catalog validation, one shared typed parser, handler integration tests, and exact human/agent-help input projection |
| Agent recovery | Catalog fault declarations, exact-path/help-selector executable grammar tests, and structured error snapshots |
| Bounded agent discovery | Fixed root-index shape, 512-byte per-command entry validation, 100-command growth/selection tests, and a derived-scale grouped-workflow whole-response budget with edge-equivalence checks |
| Work-packet lifecycle consistency | Repository validation of finite status, all GFM completion checkboxes, CommonMark fence handling, explicit non-template acyclic supersession, and regular-file paths |
| Local Go consistency | Gate preflight comparison of required/reported/compiler versions and GOROOT/GOTOOLDIR, with a mixed-installation shell fixture |
| External text structure | Visible-projection unit/E2E tests plus scoped I/O trust metadata; printable meaning remains explicitly out of scope |
| Documentation locale | Versioned project policy, explicit schema-1 migration diagnostic, locale preservation test, and narrow English/Japanese trusted-Markdown fixtures; broader linguistic conformance remains manual |
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
