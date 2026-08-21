# Work Context: Make routine CLI output result-first and task-specific

## Current behavior

- `context list --format text` renders source, home, and tmpfs access, method policy,
  Runtime status, image selector, guided/agent profile, and bootstrap for every
  Context, including values that are identical or diagnostic.
- Ordinary `context show` renders source, network default, policy revision,
  native readiness, guided/agent profile, Git identity, Runtime status, image,
  authentication, bootstrap, Details, and Next.
- `context show --details` already groups Context identity, Boundary, Workspace,
  Runtime, and stores/revisions. JSON is documented as complete regardless of
  `--details`.
- Human synthetic-default output currently prints `synthetic_default`, a
  not-persisted revision, readiness, and agent profile alongside recommended
  defaults.
- Ordinary `status` renders root, Context, Runtime diagnostic, session,
  bootstrap relationship, diagnostic ID, home path, and Next.
- Root human help lists every namespace at one level after a short Start here
  section. Exact `cluster status` intentionally exposes Gateway, OPA, aggregate
  revision, integrity, and policy path.
- Existing typed fixtures cover Context show, Context configuration, lifecycle
  status, human presentation foundations, machine output, and auth truthfulness.

## Relevant structure

- Entry point: root help, `context list`, `context show`, `status`, and guided
  first entry
- Domain rule: Context reports, Runtime status, Workspace lifecycle status,
  bootstrap relationship, method policy, native readiness, and authentication
  ownership under `internal/domain/tobari`
- Application use case: Context and Workspace read use cases under
  `internal/app/contextcmd` and `internal/app/tobaricmd`
- Infrastructure boundary: Context/runtime stores and Workspace status readers;
  no new read is planned in this packet
- CLI catalog or presentation: Context/status specs, renderers, help grouping,
  semantic tokens, text/JSON documents, and fixture tests under `internal/cli`
- Existing tests and harness checks: exact-key JSON contracts, semantic fixture
  answer keys, negative-inference canaries, generated command references,
  Playwright documentation checks, and contractlint

## Constraints

- Semantics precede presentation. A renderer cannot derive readiness or action
  from a label, hash, order, non-empty path, or nearby field.
- Absent and explicit empty/zero/false/unresolved states remain distinct where
  interpretation changes.
- JSON schemas, exact next argv, canonical references, effects, delivery, and
  collection coverage remain authoritative unless a separate accepted contract
  decision changes them.
- Root agent help remains the bounded capability index. Human help may group
  commands but cannot hide an invocable public command or create a second
  registry.
- `cluster` remains the stable exact command namespace even when routine prose
  says Shared services.
- Current status has no task-owned pending-permission or exposed-service result;
  presentation must not synthesize either as `0` or `none`.
- Shell presentation may intentionally inherit narrow values such as PS1, so a
  blanket “Host configuration not imported” statement is not currently true.

## External facts

No external source is required. This packet is based on direct read-only CLI
observations on 2026-08-21 and the accepted product-owner concept review.

## Unknowns

- [ ] Define the finite typed routine-traffic states from native-readiness,
      Context method policy, destination ceiling, and reviewed compatibility
      facts; enabled alone cannot mean fully ready.
- [ ] Decide whether shell, Git, and bootstrap fit one compact Workspace-default
      line without erasing non-default or action-required distinctions.
- [ ] Determine which bootstrap/reconciliation states demand ordinary status
      attention and which healthy states may recede safely.
- [ ] Audit the vocabulary packet's final public machine-field disposition
      before deciding where agent profile remains visible.
- [ ] Select exact before/after fixtures for persisted and synthetic Contexts,
      list diversity, Workspace absent/detached/attached, and action-required
      lifecycle states.
- [ ] Determine whether human root-help grouping is achievable from catalog
      metadata without adding presentation-only duplicate classification.

## Thesis evidence

- Repeated design decision or point of agent confusion: exact internal facts
  are exposed consistently, but the routine question and next action are less
  prominent than revisions, IDs, paths, images, profiles, and component names.
- User outcome or friction observed in the minimal slice: a user choosing a
  Context needs Access and Tools; a user running `status` needs current state and
  recovery—not store layout.
- Code workaround or exception being considered: hiding strings only in one
  renderer would make human, agent, JSON, and generated contracts diverge or
  force presentation to infer effective state.
- Current thesis that resolves it, or proposed thesis revision: keep complete
  typed semantics and add result-first task projections with explicit detailed
  surfaces.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: human presentation contract, task result variants, fixture corpus,
  root help, negative-inference tests, and generated references.

## Reproduction or observation

```sh
go run ./cmd/tobari context list --format text
go run ./cmd/tobari context show --format text
go run ./cmd/tobari context show --details --format text
go run ./cmd/tobari status --format text
go run ./cmd/tobari help
go run ./cmd/tobari cluster status --format text
```

Observed 2026-08-21: routine Context and status output leads with several
diagnostic/internal facts, while the detailed Context surface already provides
a viable complete technical destination.

## Security and public-boundary notes

- Assets and side effects involved: typed read results, human renderers, help,
  semantic fixtures, JSON contracts, and generated docs; no mutation or new
  external I/O is planned
- Credentials or confidential data involved: no secret values; authentication
  ownership remains a structural public fact
- New dependencies, destinations, files, processes, or generated content:
  deterministic fixture/answer/golden files only; no runtime dependency
- External schema provenance, publication rights, and drift evidence: all
  affected schemas and presentation fixtures are Tobari-owned
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: unchanged
- Publication and licensing concerns: synthetic fixture values only

## Glossary

- **Result-first:** present what the selected setting means for the user's task
  before the implementation source or revision that produced it.
- **Task-specific:** one command answers one primary user question and links to
  the exact detailed/specialist command for adjacent facts.
- **Routine surface:** default human text for ordinary commands.
- **Detailed surface:** explicit `--details`, JSON, or specialist command output
  retaining complete technical facts.
- **Action-required:** a typed state with a concrete exact next command; never
  inferred from visual emphasis alone.
