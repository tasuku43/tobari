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
| `public` | `task public:check` | Bootstrap completion and public publication | Ready-profile identity, forbidden-data, required-file, license, capability/schema contracts, and public-boundary checks |

Direct invocation is supported for automation:

```sh
./scripts/check.sh fast
./scripts/check.sh full
./scripts/check.sh security
./scripts/check.sh release
./scripts/check.sh public
```

Every profile starts with a local-toolchain preflight after the gate sanitizes its Go environment. The preflight requires the exact Go version declared by `go.mod` under `GOTOOLCHAIN=local` and verifies the selected binary, its reported version, `GOVERSION`, `GOROOT`, `GOTOOLDIR`, and the compiler in that tool directory as one installation. A mismatch fails once with those values and remediation guidance before formatting, tests, downloads, or release builds begin.

All profiles require Git, Go, and `gofmt`. The `release` profile additionally requires ShellCheck 0.9.0 or newer, Ruby, `tar`, `unzip`, and either `sha256sum` or `shasum`. Pinned Go security and action-lint tools must already exist in the module cache or be downloadable over the network. The release preflight reports missing system tools together before the long gate begins; network availability is documented rather than actively probed because a network probe would be nondeterministic and provider-specific.

The canonical gate and release packager force module mode and neutralize ambient Go workspace, toolchain, experiment, FIPS, and flag settings before invoking Go. This prevents a local or CI `GOFLAGS` value from silently selecting no tests and keeps agent, developer, and workflow evidence on the same checked command set. A release fixture launches the public profile with hostile values and proves that its first Go-backed check observes only the sanitized contract.

CI is the completion authority. Pull-request CI runs `full` and the
security/public boundary profiles in parallel. The repository installs no
automatic Codex Stop hook: a per-turn gate adds latency and does not prove
completion. Optional local automation must delegate to one named profile and
must not claim equivalence to a profile it did not run.

## Harness components

### `.harness/project.json`

This schema-versioned file is the machine-readable source for template identity, bootstrap state, exact runnable defaults, and repository policy. The bootstrap tool validates it before replacement and changes its profile from `template` to `ready` only after successful application. The stored word `ready` means identity-ready only; it does not assert product, security, legal, or release readiness.

`binary_name` is a portable lowercase executable basename of at most 96 bytes, leaving room for the mandatory Windows `.exe` suffix under the 100-byte cross-format archive-entry limit. Validation rejects the case-insensitive Windows device names `CON`, `AUX`, `PRN`, `NUL`, `COM1` through `COM9`, and `LPT1` through `LPT9`; adding `.exe` does not make those names extractable on Windows. It also rejects `LICENSE` case-insensitively because every release archive reserves that entry name. These are parts of the default cross-format release-matrix contract, not naming-style preferences.

Policy that must be reviewed by both humans and tools belongs here when it is finite and structural, such as forbidden private identifiers or expected module and binary names. Product reasoning remains in documentation.

Schema 2 adds `public_guard.documentation_locale`, one explicit BCP-47-like
language tag for the intended locale of trusted repository documentation and
CLI-authored prose. The template uses `en`. A schema-1 derived repository must
first choose that locale in its thesis or product contract, add the field, and
then set `schema_version` to 2; the loader applies no default and performs no
automatic migration. Bootstrap and later profile writes preserve the selected
value.

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

### `tools/bootstrap`

Bootstrap derives its validated exact replacement set from the protected provenance values in `projectconfig.Defaults`. It maps the runnable module, repository, binary, display identity, and associated project metadata to `.harness/project.json`; it does not search-and-guess arbitrary names. The defaults declaration itself is excluded from replacement so a derived repository can prove the source values.

Always preview first:

```sh
go run ./tools/bootstrap --dry-run
go run ./tools/bootstrap
```

Bootstrap failure must leave the repository in a diagnosable state and must not claim identity readiness. Bootstrap changes identity; it cannot complete theses, threat models, or release promises. `ReadyProblems` therefore requires every runnable and user-facing derived field to change, while allowing a deliberate reuse of the GitHub owner or license.

### `.agents/skills/bootstrap-derived-cli`

`$bootstrap-derived-cli` is the first-run Codex workflow for a derived repository. It does not implement a second replacement engine: it resolves missing identity decisions, invokes `tools/bootstrap` in preview-then-apply order, verifies the resulting module/import/command paths and gates, then requires a project-specific thesis and security handoff before `$add-capability`. `tools/repoguard` requires both the Skill instructions and their Codex interface metadata, while the Skill's workflow delegates mechanical safety to the same bootstrap and check commands used by humans and CI.

The Skill deliberately leaves provider selection, OAuth versus PAT, credential storage, side-effect approval, user tasks, and release ownership to the derived project's theses and security model. A `ready` profile proves only that identity replacement completed; treat it as identity-ready when communicating state.

### `tools/archlint`

Architecture lint checks production dependency direction, rejects unclassified production packages, and keeps each `cmd/` entrypoint limited to argument/stream handoff, signal cancellation, the CLI composition root, and process exit. It merges Go package information for the native build and every release target on Linux, macOS, and Windows, so a platform-specific file cannot hide a forbidden dependency from the host CI platform. Each `go list -json` process is decoded from stdout only; stderr remains a separate diagnostic channel and cannot corrupt the package stream. Source checks reject detached application, infrastructure, and CLI contexts, default HTTP clients, application-layer `fmt` presentation/scanning calls, built-in `print`/`println` in domain, application, CLI, and command packages, authentication-binding issuance outside infrastructure, and command-entrypoint access outside the narrow selector allowlist. Domain and application packages cannot import `log`, `log/slog`, or Cgo. Reviewed user-facing presentation belongs in CLI and must use its injected streams; observability and native integration are explicit derived-project infrastructure policies. Any allowed exception must be narrow, named, and tested.

The template also rejects every third-party import from `cmd` and `internal/cli` by default. Vendor SDKs, authenticated transports, and other effectful clients belong in `internal/infra`, where third-party imports remain available and the dependency/security gates review them. A derived project may allow a CLI parser or renderer only by adding its exact package path to `allowedCLIThirdPartyImports` in `tools/archlint/main.go`. The same change must include an accepted ADR or thesis consequence, license and dependency review, and a regression test proving that sibling packages, module-wide prefixes, SDKs, and transports remain rejected. Wildcards and prefix allowlists are not valid exceptions.

### `tools/repoguard`

Repository guard checks public-boundary and repository-shape policy, including bootstrap state, forbidden identifiers, likely secrets, invalid or leftover identity, work-packet lifecycle consistency, required public files, and the configured documentation locale. Its English-locale check is the narrow trusted-Markdown Japanese-script canary described above, not general language detection. Its publishable path set comes from a successful Git enumeration. Tracked paths already absent from the working tree are omitted so an unstaged bootstrap rename is valid, while untracked destinations remain included. Git errors, symbolic links, special files, and other inspection errors still fail closed. A derived project extends its policy when it adds credentials, private migrations, generated content, or publication constraints.

Work-goal status is one of `Draft`, `Accepted`, `Active`, `Complete`, or
`Superseded`. `Accepted` remains a valid pre-execution state for existing
derived histories; new work may move directly from Draft to Active. Complete
requires every acceptance checkbox in every visible Acceptance section and
every task checkbox to be checked across the standard GFM unordered and ordered
list markers. Metadata is read only from the contiguous top-level
`- Key: value` block directly below the first top-level ATX H1 (`# ...`).
Fenced examples and HTML comments do not supply metadata, headings, or
checkboxes; valid top-level and list-container CommonMark fences are
recognized. A
Superseded goal names one canonical raw relative path to a non-template
repository goal, and its successor chain must terminate rather than cycle. The
guard reads each goal and successor through the same regular-file/no-symlink
repository boundary. When adopting this guard in an existing derived
repository, maintainers must review an inconsistent historical Complete packet
and either supply its evidence, return it to Active, or supersede it explicitly.
A migration must not check boxes automatically.

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
- Application tests fix task interpretation, orchestration, and ambiguity behavior.
- Each interpretation-sensitive capability adds task-owned semantic-result
  tests for its declared task identity and the target, parent, and/or scope
  dimensions it actually carries. The tests preserve scoped empty collections
  and interpretation-relevant state distinctions, reject field/reference-kind
  laundering where multiple kinds exist, and add negative-inference canaries
  where display details could be mistaken for facts. The template sample
  mechanically covers exact-ID binding, repository target mismatch, successful
  empty output, same-label identity separation, and no partial pagination
  result; it is not a universal result type.
- Authentication, pagination, and mutation-boundary tests prove rejection/cancellation before downstream calls, exact secret-free authentication binding, complete standard runtime-fault declarations, and complete-or-no-result behavior.
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
  repeatability, dependency/conflict, duplicate scalar, and syntax drift.
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
| Request-bound semantic result | Per-capability domain/application tests for declared task identity and every applicable request dimension; the sample proves exact-ID/mismatch/empty/no-partial behavior, while scoped or relationship-rich capabilities add their own scope, state, contextual-kind, and negative-inference fixtures |
| Action target composition | Reachable reference-graph validation and byte-preserving round trips for reference-bound acts; complete, exclusive, reference-free declarations for command-bound fixed targets |
| Side-effect ordering | Fake adapter counters and failure-before-I/O tests |
| Mutation outcome classification | Structured-fault-first/cause-stripping tests, non-retryable unclassified outcome fallback, and read-only recovery validation |
| Confirmed mutation output | One effect-aware finalizer, late-cancellation regression, non-retryable mutation short-write fault, and read-only recovery validation |
| Authentication precondition | Secret-free session contract, zero-downstream-call tests, and catalog validation of every standard gate fault's code/kind/retryability |
| Authentication binding | Opaque JSON-excluded/fmt-redacted binding type, infrastructure-only issuance lint, exact pass-through test, and derived two-account/stale-binding/refresh-race adapter fixtures |
| Pagination completeness | Cursor loop/budget/cancellation tests, retryability/catalog agreement, and no-partial-result assertion |
| Public paged continuation | Catalog validation of one exact same-kind optional input/top-level output binding, non-`not_applicable` coverage, JSON-only presentation, and agent-help/reference-workflow projection |
| Non-secret authentication configuration | Bounded strict codec, unknown-schema and unsafe-file rejection, opened-directory confinement, immediate identity revalidation, Unix directory sync, explicit Windows limitation, fail-closed source precedence, and read-only status tests |
| Human authentication handoff | Agent-readiness records environment exports, re-entry, browser/terminal transfers, OS integration, ceremonial inputs, and first-run/steady-state invocations |
| Retry safety | Timeout/attempt/idempotency validation and adapter contract tests |
| Rate evidence versus replay permission | Fault validation permits positive `retry_after` on non-retryable rate limits only, plus text/JSON projection tests |
| Executable command inputs | Catalog validation, one shared typed parser, handler integration tests, and exact human/agent-help input projection |
| Agent recovery | Catalog fault declarations, exact-path/help-selector executable grammar tests, and structured error snapshots |
| Bounded agent discovery | Fixed root-index shape, 512-byte per-command entry validation, 100-command growth/selection tests, and a derived-scale grouped-workflow whole-response budget with edge-equivalence checks |
| Meaningful derived identity | Field-level `ReadyProblems` tests that reject unchanged runnable/user-facing identity while allowing owner/license reuse, plus protected-defaults bootstrap tests |
| Work-packet lifecycle consistency | Repository validation of finite status, all GFM completion checkboxes, CommonMark fence handling, explicit non-template acyclic supersession, and regular-file paths |
| Bootstrap working-tree paths | Temporary-Git deletion/untracked-destination regression, successful Git enumeration requirement, selected-path no-link/regular-file validation, and full shape scan |
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
