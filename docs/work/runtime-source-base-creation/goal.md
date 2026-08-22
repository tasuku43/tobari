# Work Goal: Create a Runtime source from an explicit Base

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, and `docs/04_harness.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Product and implementation agent
- Target: Feedback-derived usability queue
- Related ADRs: ADR 0071 (Context work-mode separation)

## Outcome

A person creating a managed Runtime chooses one Base source before creation.
`standard` means Tobari's built-in editable starter source; another Runtime
name means that managed Runtime's current editable `source/` tree. Tobari
copies the selected source through the existing bounded owner-only source
contract into a new standalone managed Runtime without building it, changing a
Context, or retaining inheritance.

When `standard` is the only available Base, interactive creation does not show
a redundant chooser. When managed Runtime sources exist and `--base` is
omitted on interactive text streams, creation first shows a Base chooser with
`standard` initially selected. A fully explicit caller uses
`runtime create --base <runtime-name> --name <new-name>`.

## Why now

The existing `runtime create` always starts from the built-in Dockerfile
template. A person who wants a nearby tool environment must manually copy a
trusted Runtime source tree outside Tobari or reconstruct it from memory.
Runtime already separates editable source, immutable builds, and explicit
Context selection; Base-aware creation removes that source-preparation friction
without combining those lifecycle stages.

## Non-goals

- Copying an immutable `name@ordinal` revision snapshot.
- Building a Runtime revision as part of creation.
- Selecting the created Runtime for any Context.
- Copying a Runtime ID, revision history, image identity, Context binding, or
  Workspace state.
- Persisting lineage, inheritance, or a live relationship to the Base.
- Treating a Runtime name as an opaque reference, mutation parent, or target.
- Adding `runtime customize`, opening `$EDITOR`, or changing `runtime build` or
  `context runtime set` into one combined mutation.
- Changing existing non-interactive `runtime create --name NAME` omission
  semantics: omission of `--base` continues to mean the built-in standard
  starter source and never prompts on redirected or machine-readable streams.

## Acceptance criteria

- [ ] `runtime create --base standard --name frontend` creates one new
      owner-only editable source tree from the exact built-in starter source.
- [ ] `runtime create --base frontend --name mobile` copies `frontend`'s
      current managed editable source, preserving bounded relative paths,
      bytes, and owner permission bits required for later editing and build.
- [ ] The new Runtime receives a new stable ID, empty revision history, and an
      independent source path; later Base edits do not affect it.
- [ ] Creation copies no immutable revision snapshot, image identity, Runtime
      history, Context binding, Workspace state, or inheritance metadata.
- [ ] With only `standard` available, interactive omission of `--base` creates
      from standard without a chooser. With one or more managed Runtime
      sources, interactive omission shows a deterministic Base-first chooser
      with `standard` initially selected.
- [ ] Redirected, JSON, and otherwise non-interactive omission of `--base`
      retains the current standard-starter behavior with no prompt.
- [ ] An exact supplied `--base` skips Base selection; completion offers
      `standard` and managed Runtime names, never `name@ordinal` revisions.
- [ ] Source validation, source-drift detection, cancellation, invalid Base,
      target collision, and presentation/output failure before action leave no
      partial target Runtime.
- [ ] Human help, scoped agent help, completion, text/JSON results, and stable
      failures describe Base as a source initializer and preserve the separate
      next actions `runtime build` and `context runtime set`.
- [ ] The known-path agent journey needs one scoped-help lookup, one creation
      invocation, and zero undeclared external processing.
- [ ] Focused tests, `task check`, `task security`, and `task public:check`
      pass; release checks remain unnecessary unless implementation changes
      packaging or release assets.

## Governing documents

- Thesis: bounded autonomy should be easier than host execution; Runtime
  editing, immutable revision creation, and Context selection remain separate.
- Product contract section: Runtime catalog and `runtime create` / `runtime
  build` lifecycle.
- Architecture or security invariant: one owner-only bounded managed source
  tree, one controlled fixed-target catalog mutation, immutable build
  snapshots, and no Context mutation during Runtime creation.
- Existing ADR: ADR 0071 keeps Runtime preparation independent from Context
  composition and selection.

## Completion definition

The work is complete when acceptance criteria have evidence, durable decisions
have been promoted to numbered documentation or an ADR, required profiles
pass, temporary diagnostics or sensitive artifacts are removed, and this
temporary packet is removed from the final tree.
