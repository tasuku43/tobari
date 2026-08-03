# CLI catalog audit E2E transcript

This file is the bounded transcript for the clean-build and real-argv portion
of the audit. It records command paths, exit status, selected output/fault
facts, and side-effect boundary observations without copying host paths,
secrets, or private identifiers.

## Method

- Build: `go build -o "$TMPDIR/tobari-cli-catalog-audit" ./cmd/tobari`.
- Help: run root human/agent help, namespace-scoped human/agent help, and exact
  command human/agent help from that binary.
- Representative argv: run one real invocation for every public catalog path;
  use read-only or invalid/missing-prerequisite inputs where a mutation would
  require a configured Docker Engine or policy state.
- Side-effect rule: a missing runtime, invalid input, or retired path must fail
  before Docker/policy mutation; only a configured E2E runtime may exercise the
  positive mutation path.
- Evidence format: each row records exit code and a bounded result. Full
  unbounded logs are intentionally not included.

## Catalog inventory

The inventory table is filled from `DefaultCatalog().Commands()`, not from
handwritten README text. For each row, `R` means utility/read, `D` means
discover/read, and `A` means act/create or write. Reference flow is expressed as
`produce(kind.field) -> consume(kind.input)`; `fixed(kind/id)` means a
command-bound `tool_local` target. The public set is 25 paths and exactly
matches the product-contract command table.

| Path | Role/effect | Capability | Target/reference flow | Handler | Classification |
|---|---|---|---|---|---|
| `doctor` | R/read | `system.diagnostics` | none | `runDoctor` | maintain |
| `help` | R/read | `cli.discovery` | none | `runHelp` | maintain |
| `version` | R/read | `cli.version` | none | `runVersion` | maintain |
| `context list` | R/read | `context.composition` | fixed catalog observation | `runContextList` | maintain |
| `context show` | R/read | `context.composition` | fixed/active Context observation | `runContextShow` | maintain |
| `context create` | A/create | `context.composition` | fixed(context-catalog/context-catalog) | `runContextCreate` | maintain |
| `context use` | A/write | `context.composition` | fixed(active-context/active-context) | `runContextUse` | maintain |
| `runtime init` | A/create | `runtime.customization` | fixed(active-context-runtime/active-context-runtime) | `runRuntimeInit` | maintain |
| `runtime build` | A/write | `runtime.customization` | fixed(active-context-runtime/active-context-runtime) | `runRuntimeBuild` | maintain |
| `tobari` | A/create | `tobari.lifecycle` | fixed(current-directory/current-directory) | `runProjectEnter` | maintain |
| `status` | R/read | `tobari.lifecycle` | fixed(current-directory/current-directory) | `runProjectStatus` | maintain |
| `delete` | A/write | `tobari.lifecycle` | fixed(current-directory/current-directory) | `runProjectDelete` | maintain |
| `list` | R/read | `tobari.lifecycle` | diagnostic ID only; no consumer | `runList` | maintain |
| `cluster up` | A/create | `cluster.lifecycle` | fixed(cluster/cluster-default) | `runClusterUp` | maintain |
| `cluster status` | R/read | `cluster.lifecycle` | fixed(cluster/cluster-default) | `runClusterStatus` | maintain |
| `cluster denials` | R/read | `policy.learning` | fixed cluster observation; no mutation ref | `runClusterDenials` | maintain |
| `cluster logs` | R/read | `cluster.logs` | fixed cluster observation; no mutation ref | `runClusterLogs` | maintain |
| `cluster down` | A/write | `cluster.lifecycle` | fixed(cluster/cluster-default) | `runClusterDown` | maintain |
| `policy candidates` | D/read | `policy.learning` | produce(policy-candidate.id) | `runPolicyCandidates` | maintain |
| `policy review` | D/read | `policy.learning` | produce(policy-candidate.id) -> allow/deny; TTY confirmation only | `runPolicyReview` | maintain |
| `policy tail` | D/read | `policy.learning` | produce(policy-candidate.id) -> allow/deny; text compatibility projection | `runPolicyTail` | maintain (compatibility) |
| `policy allow` | A/write | `policy.learning` | consume(policy-candidate.--id) | `runPolicyAllow` | maintain |
| `policy deny` | A/write | `policy.learning` | consume(policy-candidate.--id) | `runPolicyDeny` | maintain |
| `policy compactions` | D/read | `policy.learning` | produce(policy-compaction.id) | `runPolicyCompactions` | maintain |
| `policy compact` | A/write | `policy.learning` | consume(policy-compaction.--id) | `runPolicyCompact` | maintain |

## Catalog contract matrix

This matrix is derived from each command's scoped agent help. Recovery lists are
the exact catalog paths named by the declared fault next actions; the full fault
codes and their action mapping were inspected from the same scoped JSON.

| Path | Usage inputs | Output: formats / delivery / coverage | Reference flow | Recovery command paths |
|---|---|---|---|---|
| `doctor` | `--format`, `--root` | `text|tsv|json` / `complete` / `exhaustive` | none | `doctor`, `help doctor` |
| `help` | `command`, `--format` | `text|json` / `complete` / `exhaustive` | none | `help` |
| `version` | none | `text` / `complete` / `not_applicable` | none | `help version`, `version` |
| `cluster up` | `--gateway-source` | `text` / `complete` / `not_applicable` | fixed(`cluster/cluster-default`) | `cluster down`, `cluster status`, `cluster up`, `doctor`, `help cluster up` |
| `cluster status` | `--format` | `text|json` / `complete` / `exhaustive` | fixed(`cluster/cluster-default`) | `cluster down`, `cluster status`, `cluster up`, `doctor`, `help cluster status` |
| `cluster denials` | `--tail`, `--format` | `text|json` / `complete` / `bounded_window` | fixed cluster observation | `cluster denials`, `cluster logs`, `cluster up`, `doctor`, `help cluster denials` |
| `cluster logs` | `--component`, `--tail` | `text` / `complete` / `bounded_window` | fixed cluster observation | `cluster logs`, `cluster status`, `cluster up`, `doctor`, `help cluster logs` |
| `cluster down` | `--purge` | `text` / `complete` / `not_applicable` | fixed(`cluster/cluster-default`) | `cluster down`, `cluster status`, `cluster up`, `doctor`, `list`, `help cluster down` |
| `policy candidates` | `--tail`, `--format` | `text|json` / `complete` / `bounded_window` | produce(`policy-candidate.id`) | `cluster denials`, `cluster up`, `doctor`, `help policy candidates`, `policy candidates` |
| `policy review` | `--tail`, `--format` | `text|json` / `complete` / `bounded_window` | produce(`policy-candidate.id`) -> `policy allow/deny --id` | `cluster denials`, `cluster up`, `doctor`, `help policy review`, `policy review` |
| `policy tail` | `--tail` | `text` / `complete` / `bounded_window` | produce(`policy-candidate.id`) -> `policy allow/deny --id` | `cluster denials`, `cluster up`, `doctor`, `help policy tail`, `policy tail` |
| `policy allow` | `--id` | `text` / `complete` / `not_applicable` | consume(`policy-candidate.--id`) | `cluster denials`, `cluster status`, `cluster up`, `doctor`, `help policy allow`, `policy allow`, `policy candidates` |
| `policy deny` | `--id` | `text` / `complete` / `not_applicable` | consume(`policy-candidate.--id`) | `cluster denials`, `cluster status`, `cluster up`, `doctor`, `help policy deny`, `policy deny`, `policy review` |
| `policy compactions` | `--format` | `text|json` / `complete` / `exhaustive` | produce(`policy-compaction.id`) | `cluster up`, `doctor`, `help policy compactions`, `policy compactions` |
| `policy compact` | `--id` | `text` / `complete` / `not_applicable` | consume(`policy-compaction.--id`) | `cluster status`, `cluster up`, `doctor`, `help policy compact`, `policy compact`, `policy compactions` |
| `context list` | `--format` | `text|json` / `complete` / `exhaustive` | fixed Context catalog observation | `context list`, `doctor`, `help context list` |
| `context show` | `--name`, `--format` | `text|json` / `complete` / `not_applicable` | fixed active/named Context observation | `context list`, `context show`, `doctor`, `help context show` |
| `context create` | `--name`, `--image`, `--mode`, `--format` | `text|json` / `complete` / `not_applicable` | fixed(`context-catalog/context-catalog`) | `context create`, `context list`, `context show`, `doctor`, `help context create` |
| `context use` | `--name`, `--format` | `text|json` / `complete` / `not_applicable` | fixed(`active-context/active-context`) | `context list`, `context show`, `context use`, `doctor`, `help context use` |
| `runtime init` | `--format` | `text|json` / `complete` / `not_applicable` | fixed(`active-context-runtime/active-context-runtime`) | `context show`, `doctor`, `help runtime init`, `runtime init` |
| `runtime build` | `--format` | `text|json` / `complete` / `not_applicable` | fixed(`active-context-runtime/active-context-runtime`) | `context show`, `doctor`, `help runtime build`, `runtime build`, `runtime init` |
| `tobari` | none | `none` / `complete` / `not_applicable` | fixed(`current-directory/current-directory`) | `cluster status`, `cluster up`, `delete`, `doctor`, `help tobari`, `status`, `tobari` |
| `status` | `--format` | `text|json` / `complete` / `not_applicable` | fixed(`current-directory/current-directory`) | `doctor`, `help status`, `status` |
| `list` | `--format` | `text|json` / `complete` / `exhaustive` | diagnostic ID only; no consumer | `doctor`, `help list`, `list`, `status` |
| `delete` | `--force` | `text` / `complete` / `not_applicable` | fixed(`current-directory/current-directory`) | `delete`, `doctor`, `help delete`, `status`, `tobari` |

## Classification summary

- **Delete candidate:** the five unregistered legacy definitions in the next
  table (`attach`, `shell`, `exec`, per-Tobari `logs`, and `detach`). They are
  already rejected before catalog routing. Source cleanup is not applied in
  this packet because the application/infrastructure legacy port and persisted
  state compatibility need one capability-retirement change.
- **Integrate:** none. E2E and scoped contracts show that similar-looking
  public paths own different task scopes, effects, output schemas, or human vs
  machine behavior. In particular, `policy tail` is an explicit compatibility
  projection, not an accidental duplicate.
- **Maintain:** 23 public paths, including all lifecycle, cluster, policy,
  Context, runtime, diagnostic, help, and version paths not listed as hold.
  `policy review` remains the TTY Permission Inbox and `policy candidates`
  remains machine discovery.
- **Hold:** `context list` and `context show` need a product/architecture
  decision because their read declarations initialize the default Context store
  on an empty XDG root. The positive Docker policy/lifecycle workflow also
  remains held until the integration harness can run without colliding with an
  existing cluster.

### Description follow-ups (not command retirement)

- `policy tail` should say “compatibility projection” in its human summary or
  scoped description; its contract and E2E output are currently correct.
- `policy review` should surface its TTY-only explicit allow/deny and redirected
  read-only behavior prominently in human docs; scoped agent help already
  declares it.
- `cluster logs` should make its bounded/redacted shared-component scope clear
  in the short summary; it remains distinct from typed `cluster denials`.
- README contains a stale `tobari exec --id ...` example and should be corrected
  by the documentation packet, not by this audit.

## Suspected legacy definitions (not public catalog paths)

| Source definition | Current argv | Catalog reachable? | Classification | Evidence/next action |
|---|---|---:|---|---|
| `attachSpec` + `runAttach` | `attach` | no | delete candidate | Explicit retired-command rejection; verify no production references and remove only as disjoint dead code if gates pass. |
| `shellSpec` + `runTobariShell` | `shell` | no | delete candidate | Same; current root `tobari` owns interactive entry. |
| `execSpec` + `runTobariExec` | `exec` | no | delete candidate | Same; README has one stale example that docs packet must correct separately. |
| `logsSpec` + `runTobariLogs` | `logs` | no | delete candidate | Shared `cluster logs` is the supported log surface; retired message points there. |
| `detachSpec` + `runDetach` | `detach` | no | delete candidate | `delete` owns CWD-local lifecycle removal; persisted-state compatibility requires separate review before app-layer cleanup. |

## E2E result table

The clean binary was built from the current checkout and invoked directly. A
command that reaches a declared missing-prerequisite fault is an E2E
reachability result when it proves the handler and recovery path without
crossing the side-effect boundary. The valid Context/runtime mutations below
used an isolated temporary XDG root. No positive Docker mutation was attempted
against the user's active resource namespace.

| argv | Exit | Observed output/fault/recovery | Side-effect boundary |
|---|---:|---|---|
| `help` | 0 | Root human help rendered the catalog hierarchy and usage. | Read-only. |
| `help --format agent` | 0 | Agent index: schema 8, `view=index`, 25 paths; root keys remained index-only. | Read-only. |
| `help cluster`, `help cluster --format agent` | 0 | Human and scoped agent namespace help both rendered all five cluster commands. | Read-only. |
| `help policy review`, `help policy review --format agent` | 0 | Exact human/agent help exposed inputs, bounded coverage, faults, candidate refs, and interactive explicit confirmation/read-only behavior. | Read-only. |
| All 25 exact `help <path>` human and agent invocations | 0 | Every public path had a successful scoped human and scoped agent projection. | Read-only. |
| `--help`, `cluster status --help`, `policy review --help`, `context create --help`, `runtime build --help` | 0 | Trailing/direct help aliases reached catalog help. | Read-only. |
| `version` | 0 | Printed `tobari dev`. | Read-only. |
| `doctor --format json` | 0 | Schema-versioned diagnostic report; Docker/Compose checks passed and root readiness remained a warning. | Observational; no Context initialization observed. |
| `cluster status --format json` | 0 | `{configured:false,running:false,...}` in the isolated XDG root. | Read-only; no cluster mutation. |
| `cluster denials --tail 1 --format json` | 9 | `cluster_not_running`; recovery `tobari cluster up`. | Failed before Docker/policy mutation. |
| `cluster logs --component opa --tail 1` | 9 | `cluster_not_running`; recovery `tobari cluster up`. | Failed before Docker/policy mutation. |
| `cluster up` with a valid argv and an invalid task-local `DOCKER_HOST` | 9 | `gateway_image_unavailable`; recovery `tobari doctor`; preflight stopped before shared startup. | No shared resource mutation observed in the isolated root. |
| `cluster down --purge` | 0 | `Cluster not configured`. | No target resource existed in the isolated root. |
| `policy candidates --tail 1 --format json` | 9 | `cluster_not_running`; recovery `tobari cluster up`. | Failed before policy mutation. |
| `policy review --tail 1 --format json` | 9 | Redirected/JSON path stayed non-interactive and returned `cluster_not_running`; recovery `tobari cluster up`. | No policy mutation. |
| `policy tail --tail 1` | 9 | `cluster_not_running`; recovery `tobari cluster up`. | No policy mutation. |
| `policy allow --id not-a-reference` | 2 | `invalid_policy_candidate_id`; recovery `tobari policy candidates`. | Rejected before runtime/policy calls. |
| `policy deny --id not-a-reference` | 2 | `invalid_policy_candidate_id`; recovery `tobari policy review`. | Rejected before runtime/policy calls. |
| `policy compactions --format json` | 9 | `cluster_not_running`; recovery `tobari cluster up`. | Failed before policy mutation. |
| `policy compact --id not-a-reference` | 2 | `invalid_policy_compaction_id`; recovery `tobari policy compactions`. | Rejected before runtime/policy calls. |
| `context list --format json` on an empty XDG root | 0 | Returned the default Context. A fresh-root before/after check observed 0 files before and 6 owner-only Context files/markers after. | **Finding:** declared read operation initializes state; hold for contract decision. |
| `context show --format json` | 0 | Returned active default Context and separated store references. | Same initialization path; hold with `context list`. |
| `context create --name catalog-audit --image builtin --mode guided --format json` | 0 | Created a synthetic named Context and separate policy/credential stores in the task-local XDG root. | Intended host mutation, isolated to the task-local root. |
| `context use --name catalog-audit --format json` | 0 | Switched the task-local active marker and returned the selected Context. | Intended host mutation, isolated to the task-local root. |
| `context show --name catalog-audit --format json` | 0 | Returned the named active Context and runtime/store status. | Read-only after the isolated setup. |
| `runtime init --format json` | 0 | Created the active Context runtime recipe without changing the selected `builtin` image. | Intended host mutation, isolated to the task-local root. |
| second `runtime init --format json` | 10 | `runtime_recipe_exists`; recovery `tobari context show`; recipe was not overwritten. | Rejected before a second write. |
| `runtime build --format json` with a valid recipe and invalid task-local `DOCKER_HOST` | 10 | `runtime_build_failed`; recovery `tobari context show`; no promoted image was observed. | Build failed before image promotion; isolated to the task-local root. |
| `tobari` without a TTY | 2 | `tty_required`; recovery `tobari help tobari`. | Failed before logical/Docker mutation. |
| `status --format json` | 0 | Explicit empty result: `exists:false`; no root/ID/home inferred. | Read-only. |
| `list --format json` | 0 | Explicit empty exhaustive collection: `tobari:[]`. | Read-only. |
| `delete --force` | 6 | `project_not_found`; recovery `tobari`; no destructive call. | Failed before deletion. |
| `attach`, `shell`, `exec`, `logs`, `detach` with representative argv | 2 | `retired_command`; named lifecycle replacement points to CWD `tobari` (logs points to `cluster logs`). | Not catalog-reachable; no application/Docker call. |

## Help evidence

| Invocation | Exit | Required fact | Result |
|---|---:|---|---|
| `help` | 0 | Root human help | 25 public paths grouped by namespace. |
| `help --format agent` | 0 | Root agent index | `view=index`, schema 8, selection-only entries. |
| `help cluster`, `help policy`, `help context`, `help runtime` | 0 | Namespace human help | All four namespaces rendered. |
| `help cluster --format agent`, `help policy --format agent`, `help context --format agent`, `help runtime --format agent` | 0 | Namespace agent help | Scoped contracts and workflows rendered. |
| Every `help <public-path>` and `help <public-path> --format agent` | 0 | Exact scoped help | 25/25 human and 25/25 agent projections succeeded. |
| Direct `--help` samples | 0 | Human alias | Direct and trailing help routing remained executable. |

## Gate evidence

- `task integration:test` without a task-local cache: failed before the script
  due sandbox denial while Go wrote its default build cache.
- `GOCACHE=<task-local> task integration:test`: reached the real Docker runner,
  then failed with `cluster_start_failed` (exit 9). The positive policy,
  lifecycle, and cleanup scenario is blocked until the runner isolates resource
  names or refuses to run alongside an existing cluster. After the run, no
  Tobari containers/networks were present while the default XDG state retained
  an instance record; no recovery mutation was attempted.
- Focused CLI/catalog tests: `GOCACHE=<task-local> go test ./internal/cli
  ./internal/app/tobaricmd` passed.
- `task check:fast`: passed; repoguard, archlint, contractlint, runtimecheck,
  gateway snapshot, and Go tests all passed.
- `task check`: passed with exit 0; the same repository gates and full Go test
  suite passed. Go emitted only sandbox warnings while trying to update the
  read-only module stat cache.
- `task public:check`: passed with exit 0; public repoguard and contractlint
  passed.
- Clean build: `go build -trimpath` produced the audit binary; root and all
  scoped help projections and representative argv rows above were executed
  from that binary.
- Repository status: must show only this child packet plus pre-existing user or
  other-agent files; no coordinator or other packet edits were made here.

The Docker integration failure is a harness/environment follow-up, not a
successful positive-flow result: the catalog-level E2E is complete for every
public path and every legacy deletion candidate, while the positive shared
cluster/policy mutation flow remains held until resource isolation or a safe
preflight is available.
