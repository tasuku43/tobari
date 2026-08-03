# Work Context: CLI catalog audit

This file records verified facts and unresolved questions for the isolated CLI
catalog audit. It does not change the coordinator packet or another packet.

## Current behavior

- `internal/cli/catalog.go` defines `cli.Catalog`, catalog validation, root
  `doctor`/`help`/`version`, and appends `runtimeCommandSpecs()`.
- `internal/cli/runtime_catalog.go` registers 22 runtime command specs. Together
  with the three base specs, the public catalog currently contains 25 command
  paths. The complete command list and evidence table are recorded in
  `e2e-transcript.md`.
- `attachSpec`, `shellSpec`, `execSpec`, `logsSpec`, and `detachSpec` remain as
  unreferenced functions in `runtime_catalog.go`; their handlers remain in
  `internal/cli/tobari.go`, but `runtimeCommandSpecs()` does not register them.
  `cli.retiredCommandMessage` rejects their historical argv names before
  catalog routing. This is a dead-definition cleanup candidate, not a public
  command currently reachable through the catalog.
- The product contract intentionally keeps `policy candidates` as machine
  discovery, `policy review` as the human Permission Inbox, and `policy tail` as
  a compatibility projection. Their similar output is deliberate but requires
  clearer classification in the audit.
- `context list` and `context show` are cataloged as utility/read commands, but
  their infrastructure adapter calls `ensureContextStore()`. On an empty XDG
  root, a real `context list --format json` invocation returned successfully and
  created the default Context manifest, policy files, credentials metadata, and
  active marker. This is a read/effect boundary mismatch that needs a follow-up
  decision; this packet does not silently relabel or redesign it.
- `cluster denials` is the typed denial evidence projection; `cluster logs` is the
  bounded raw shared-component diagnostic surface. They are not interchangeable
  because their output schemas and recovery meaning differ.
- The repository README still contains one historical `tobari exec --id ...`
  example even though `exec` is rejected and absent from the catalog. This is a
  documentation consistency finding for a later docs packet; this packet will
  not edit README.
- `task integration:test` was retried with a task-specific writable Go cache.
  It reached the Docker integration runner and returned `cluster_start_failed`
  (exit 9). After the run, no Tobari containers/networks were present while
  the default XDG state still contained a project record. No restoration or
  further host mutation was attempted here. This is evidence that the current
  integration harness is unsafe to run alongside an existing Tobari cluster;
  the full positive Docker flow remains blocked until an isolated resource
  namespace or a safe preflight is provided.

## Relevant structure

- Entry point: `cmd/tobari/main.go` -> `internal/cli.New(...).RunContext`.
- Domain rule: `internal/domain/operation` validates effects/targets; opaque
  policy and Tobari references are defined in `internal/domain/tobari`.
- Application use cases: `internal/app/tobaricmd` and `internal/app/contextcmd`.
- Infrastructure boundary: `internal/infra/dockerruntime`, `systemdoctor`, and
  `terminal`; CLI does not construct Docker or network clients.
- CLI catalog/presentation: `internal/cli/catalog.go`,
  `internal/cli/runtime_catalog.go`, `help.go`, `arguments.go`, and handlers in
  `tobari.go`, `context.go`, `doctor.go`, and `version.go`.
- Existing tests and harness checks: `internal/cli/*_test.go`, catalog/help
  contract tests, `scripts/test-integration.sh`, `task check`, and
  `task public:check`.

## Constraints

- The catalog is the only public command source of truth. Do not infer public
  reachability from an unregistered spec or a dormant application method.
- Discovery and action remain separate. Policy candidates/review/tail produce
  the same opaque candidate kind; allow/deny consume it unchanged. Compactions
  and compact use a distinct opaque kind.
- Every command must retain a declared role, effect, output delivery/coverage,
  recovery, and controlled side-effect boundary.
- Existing user changes and all other work packets are out of scope. Only this
  child packet may be edited unless a disjoint dead-code cleanup is proven safe.
- Public documentation is English and must not contain local paths, secrets, or
  private organization identifiers.

## Unknowns

- [ ] Whether the unregistered legacy spec/handler/application code can be
      removed as one disjoint, buildable change without losing retained state
      compatibility; answer with targeted code reachability and full gates.
- [ ] Whether any policy command can be integrated without violating the
      product contract's explicit human/machine separation; answer with
      catalog/help and user-flow E2E, not output similarity alone.
- [ ] Whether the Docker integration environment is available for the full
      `scripts/test-integration.sh` scenario; if not, record the exact bounded
      failure and retain local E2E evidence for every command. Current result:
      Docker is reachable, but an existing-resource collision/cleanup path
      ended in `cluster_start_failed` and left the active runtime state
      incomplete; no restoration was attempted by this audit.

## Thesis evidence

- Repeated design decision or point of agent confusion: old named lifecycle
  implementation remains in source while the CWD-owned catalog rejects it.
- User outcome or friction observed in the minimal slice: command names and
  help must agree with the CWD-first lifecycle; stale `exec` examples can send
  users to an intentionally retired path.
- Code workaround or exception being considered: none; first classify the dead
  definitions and stale documentation before removing anything.
- Current thesis that resolves it: Thesis 0 makes bounded CWD-first autonomy
  the front door; Thesis 5 keeps exact ownership and explicit cleanup; Thesis 7
  makes catalog/help/tests the executable claims.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: possible capability-retirement evidence and README correction; no
  durable contract is changed by this audit alone.

## Reproduction or observation

```sh
go build -o "$TMPDIR/tobari-cli-catalog-audit" ./cmd/tobari
"$TMPDIR/tobari-cli-catalog-audit" help --format agent
"$TMPDIR/tobari-cli-catalog-audit" help cluster --format agent
"$TMPDIR/tobari-cli-catalog-audit" help policy review --format agent
"$TMPDIR/tobari-cli-catalog-audit" help --format text
```

The complete command transcripts, representative argv, exit codes, and side-
effect observations are recorded in `e2e-transcript.md`.

## Security and public-boundary notes

- Assets and side effects involved: catalog routing/help; Docker and policy
  mutations are exercised only through bounded synthetic or absent-state argv.
- Credentials or confidential data involved: none; use synthetic names and
  `example.com`-style values only.
- New dependencies, destinations, files, processes, or generated content: no
  production dependencies; the packet may create a temporary clean binary and
  task-specific cache outside the repository.
- External schema provenance, publication rights, and drift evidence: none.
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: preserve each catalog declaration;
  representative E2E records read-only failures before side effects and
  mutation recovery commands where the local runtime permits.
- Publication and licensing concerns: public packet prose contains no private
  data and any retirement conclusion must use the capability-retirement
  checklist before implementation.

## Glossary

- **Public command:** a path returned by `DefaultCatalog().Commands()` and
  accepted by catalog routing.
- **Dead definition:** a spec/handler that is present in source but not
  reachable from `cli.Catalog` or another supported production call path.
- **Compatibility projection:** a deliberately retained command that overlaps
  another output but preserves an existing user or agent workflow.
- **Representative argv:** a real executable invocation that reaches the
  catalog handler or produces the declared structured fault before any unsafe
  side effect.
