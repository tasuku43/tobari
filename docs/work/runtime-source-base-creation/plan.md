# Work Plan: Create a Runtime source from an explicit Base

- Status: Accepted
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)

## Chosen approach

Extend the canonical `runtime create` action with optional
`--base <runtime-name>`. `standard` generates the same built-in starter source
used by creation today. A managed Runtime name selects its current editable
`source/` tree. The action validates and streams one stable source inventory
into a private target staging tree, preserves bytes and owner modes, creates a
fresh Runtime identity with empty history, and atomically publishes the
standalone target. No Base identity or revision is persisted in the new
Runtime.

Interactive text omission reads the exhaustive local Runtime catalog. If
`standard` is the only source Base, it proceeds exactly as today. If managed
sources exist, it shows one Base-first chooser with `standard` selected. An
explicit `--base` skips the chooser. Redirected and JSON omission retains the
existing standard-starter behavior and never prompts.

## Alternatives considered

### Copy an immutable `name@ordinal` revision snapshot

This initially appeared attractive because the revision is stable and already
validated. It is rejected because immutable snapshot files are deliberately
changed to `0400`, while the revision record retains no original per-file mode
inventory. Copying such a snapshot cannot distinguish an originally
executable installer from an ordinary file and therefore cannot reconstruct a
faithful editable source tree.

### Add a long `runtime customize` workflow

A guide could create, open an editor, build, and select a Runtime in one flow.
It adds host process execution, editor lifetime/recovery behavior, and several
separate mutations before the remaining source-starting-point problem is
understood. It is deferred. Base-aware source creation is the smaller change
that preserves the existing create/edit/build/select lifecycle.

### Copy from the selected Context's Runtime

This would make Context omission or `context use` affect a Runtime-catalog
creation task. It also selects an immutable binding rather than editable
source. Rejected because Runtime source is installation-wide and has no
current/default Runtime of its own.

## Design

### Public contract

- Command remains `runtime create`, capability `runtime.customization`,
  `RoleAct`, `EffectCreate`, and command-bound fixed target `tool_local`
  Runtime catalog.
- Grammar becomes
  `runtime create [--base <runtime-name>] --name <new-name> [--format text|json]`.
  Name remains required; this packet does not add a Runtime-name prompt.
- `--base` is optional single text input completed from `standard` plus managed
  Runtime names. It is neither a reference nor a mutation target/parent.
- Explicit `--base standard` and non-interactive omission both choose the
  built-in starter source. Explicit `--base NAME` chooses only NAME's current
  managed editable source. `NAME@ORDINAL` is invalid.
- Interactive text omission shows a chooser only when at least one managed
  source exists. The chooser is deterministic (`standard` first, managed names
  sorted) and initially selects `standard`. It identifies the new Runtime name,
  that source is copied but not built, and that no Context changes. Cancel
  performs no mutation.
- Success retains the existing Runtime report schema and `Created: true`;
  Base is not emitted as lineage. Human text identifies the resulting source
  path and exact next action `tobari runtime build --name <new-name>` through
  existing output contracts. A later Context selection stays separate.
- Discovery remains one scoped `help runtime create --format agent` read for a
  known path. Completion or `runtime list` supplies the exact Base name with
  zero external processing.
- Stable failures cover invalid/missing Base, invalid Base source, source drift,
  target collision, canceled chooser, copy/commit failure, invalid confirmed
  report, and standard mutation-output failures. Recovery remains exact
  `runtime list`, `runtime show`, or scoped help.

### Layer changes

- Domain: define a Base source selector that accepts only `standard` or a valid
  managed Runtime name, and a creation result invariant with fresh identity,
  empty history, and no lineage. Do not add Base to persisted Runtime schema.
- Application: extend the create use case and its consumer-owned port with one
  exact Base selector; retain the canonical invoker, fixed target, impact, and
  confirmed result correlation.
- Infrastructure: refactor the existing bounded source inventory/stream-copy
  logic so build snapshotting and Base copying share path, type, bound, mode,
  and drift validation. For a managed Base, preserve source bytes and original
  owner permission bits in a private target stage. For standard, generate the
  canonical starter source. Publish one fresh-ID/empty-history Runtime only
  after the complete stage validates.
- CLI and catalog: add typed `--base`, Runtime-name completion, conditional
  standard-first terminal chooser, zero-mutation cancellation, help, faults,
  and presentation fixtures. Do not create a competing command registry.

### Data and control flow

```text
runtime create argv
        |
        +-- --base standard ------> canonical built-in starter source
        +-- --base NAME ----------> exact managed editable source
        +-- non-interactive omit -> canonical built-in starter source
        +-- interactive omit -----> list source Bases
                                      | standard only: select it
                                      | otherwise: standard-first chooser
                                      v
                       validated exact Base selector
                                      |
                          canonical create invoker
                                      |
               bounded inventory + drift-checked stream copy
                                      |
              private target stage + fresh ID + empty history
                                      |
                         atomic catalog publication
                                      |
                  confirmed Runtime report; no build/select
```

### Error and cancellation behavior

- Invalid selector syntax and ordinal revision spellings fail before source or
  target I/O.
- Missing or unsafe Base, source bounds/mode/type violations, and detected
  source changes fail with no authoritative target. Stable diagnostics expose
  only bounded safe relative-path and correction facts already allowed by the
  Runtime source contract.
- Chooser cancellation, EOF, raw/line terminal failure, and rendering failure
  occur before the mutation invoker and create nothing.
- Copy uses the caller context and a fixed buffer; cancellation removes the
  private target stage. No retry is performed internally.
- A target-name collision fails without reading/copying more source than
  needed to establish the collision under the catalog lock.
- Once the action begins, structured faults survive generic cancellation;
  unclassified outcomes remain non-retryable with read-only `runtime list`
  reconciliation. Confirmed success is not converted into replay permission by
  late cancellation or output failure.

### Security and public boundary

The source is trusted-host input but may contain private binaries. The adapter
must not render source bytes, absolute host paths, or private causes. It copies
only one validated owner-local tree into another owner-local tree and grants no
Workspace, Docker, network, credential, Context, or publication authority.
Tests use synthetic small files and executable-mode canaries. No dependency,
destination, process, credential, licensed artifact, or release archive changes.

## Implementation slices

1. Public contract, Base vocabulary, and failing catalog/domain/presentation tests.
2. Application selector/result correlation and zero-mutation behavior.
3. Shared bounded source inventory/copy primitive and atomic standalone Runtime creation.
4. Conditional Base-first chooser, completion, human/agent help, and structured failures.
5. Durable docs, harness claims, agent-readiness replay, and full gates.

## Verification

- Unit and contract tests: standard and managed Base validation; fresh ID;
  empty history; no persisted lineage; exact bytes and owner modes; independent
  later edits; catalog grammar and fixed-target binding.
- Negative side-effect tests: ordinal revision Base, missing/symlink/special or
  unsafe-mode source, every size/count bound, changing source identity/size/mode
  during copy, collision, cancellation, writer failure, and atomic cleanup.
- Opaque-reference and complete-pagination tests: no new reference or
  pagination flow; catalog tests prove `--base` is not a mutation binding.
- Structured output, hostile-output, and recovery tests: unchanged schema-1
  Runtime result, visible-safe hostile Runtime names, bounded relative source
  diagnostics, exact read-only reconciliation, and no presentation-derived
  lineage.
- Agent-readiness scenario and discovery-round-trip count: one known-path
  scoped help plus create, or one `runtime list` discovery plus create; zero
  external processing and no revision-to-source reconstruction.
- Human-handoff scorecard for setup/authentication candidates: not applicable;
  no setup or authentication boundary changes.
- Manual observation: standard-only TTY, multiple-source raw and line chooser,
  explicit standard/managed Base, redirected omission, JSON omission, cancel,
  executable-mode preservation, and later Base/target independence.
- Required profiles: focused Go/catalog/contract/completion tests, `task check`,
  `task security`, and `task public:check`. `task release:check` is unnecessary
  unless implementation unexpectedly changes packaging.
- Generated-diff or artifact checks: presentation goldens, agent help, docs
  site/catalog generation as governed, and an exact repository-status audit.

## Rollout and rollback

No persisted schema migration is needed. Existing Runtime sources, histories,
Context bindings, and images retain their meaning. Existing non-interactive
`runtime create --name NAME` remains standard-based. Removing `--base` and the
conditional chooser restores the old creation path; Runtimes already created
from another Base remain ordinary standalone managed Runtimes and need no
migration.

## Documentation promotion

- Product contract: `--base` grammar, conditional interactive chooser, source
  meaning, compatibility, and separate build/select next actions.
- Architecture: Base source selection, shared bounded copy primitive, atomic
  fresh-ID/empty-history creation, and no lineage.
- Security model: owner-mode-preserving local copy, private-source
  confidentiality, zero partial target, and no new Docker/network authority.
- ADR: add or revise a Runtime lifecycle decision if review finds the
  editable-source-versus-immutable-revision distinction too durable for docs
  00–04 alone.
- Harness and agent readiness: conditional chooser, source bounds/drift/mode
  canaries, no-lineage evidence, compatibility, and zero external processing.
