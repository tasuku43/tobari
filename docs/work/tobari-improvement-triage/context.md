# Work Context: Triage Tobari product and maintenance backlog

This file records verified facts and unresolved questions. It does not turn a
desired fix or product direction into current repository behavior.

## Current behavior

- The product thesis and contract define the denial-to-retry loop as: a denied HTTP effect produces secret-free evidence and a fixed host-side review command; the host reviews one exact candidate; an explicit allow or deny action changes policy; the workload retries.
- `policy review --format json` is a read-only projection whose declared envelope is `policy_review` with schema version 2. The reported machine-readable result contains one pending exact candidate with opaque allow and deny commands.
- `internal/cli/tobari.go` routes interactive review only when the requested format is text and the runtime reports both input and output as interactive. Otherwise it renders the read-only queue.
- `internal/cli/policy_review_selector.go` first attempts raw terminal mode and falls back to line input. The raw renderer uses terminal control sequences to redraw and clear the screen; cancellation intentionally returns a successful no-op rendered as `Permission review canceled`.
- Existing unit tests cover raw selection, explicit confirmation, line fallback, EOF cancellation, opaque-ID preservation, and redirected read-only behavior. `scripts/test-integration.sh` also contains a PTY scenario that feeds an exact key sequence and verifies an interactive deny.
- The user-reported behavior was materially different from the intended contract: an interactive `policy review` invocation appeared blank and ended with `Permission review canceled`, while `policy review --format=json` showed a pending candidate. A deterministic PTY reproduction now covers early input/EOF, explicit allow/deny, cancellation, invalid input, and JSON projection; the fix is recorded in commit `7d096bb`.
- The local `codex/auth-broker` branch exists and is not an ancestor of `main`. Its historical commits contain the proposed host-side broker and declarative auth-profile work; the current `main` path does not include that branch.
- The repository's current public contract includes Context runtime customization through `runtime init`, editing the active Context Dockerfile, and `runtime build`. The current first-wave worktree has no implementation diff; only the parked auth-broker packet and this coordinator packet remain uncommitted.
- The requested base-runtime shell outcome is verified end to end: `runtimes/base/Dockerfile` contains Bash and the `tobari` user's `/bin/bash` shell, while the CLI and Docker adapter use an interactive `/bin/bash` exec. The regression coverage and transcript are committed in `912c602`. Full repository profiles remain blocked by the unrelated parked packet's machine-specific path, and the shared integration profile stopped at the existing policy-review scenario with exit 130.
- The public command catalog audit is complete and committed in `a007059`. The catalog-derived inventory found no command removal candidate that can be safely deleted from the current public surface; compatibility and recovery paths remain documented.
- The packet lifecycle audit is complete and committed in `92d742c`. It found no deletion candidate and preserves active, incomplete, uncertain, and deferred evidence.
- Several local `docs/work/` directories had no files, including paths associated with auth-broker and older exploratory topics. Empty directories are not completion evidence and are not tracked by Git; after recording that disposition, the six empty stale directories were removed on 2026-08-02. Non-empty active packets remain in place.
- The worktree is already dirty with user changes in runtime, Context, documentation, harness, and integration areas. The coordinator packet is additive and must preserve those changes.

## Issue register

The order below is a proposed execution order for review, not a permanent
priority policy.

| ID | State | Proposed order | Owner role | Dependency | Next action |
|---|---|---:|---|---|---|
| `policy-review-tty` | E2E complete; interactive review fix committed | 1 | CLI/policy maintainer | None; preserve current machine-readable contract | Carry the fixed denial-to-review-to-retry journey into the Quick Start/docs successor |
| `runtime-bash-shell` | E2E complete; scoped regression evidence committed, full shared gates have unrelated blockers | 1 (parallel) | Runtime maintainer | Existing base-image and fixed interactive exec contracts | Carry the verified Bash entry contract into the Quick Start/runtime documentation successor |
| `auth-broker-deferral` | Parked; explicitly deferred and detached from `main` | Not scheduled | Security/product owner | Explicit maintainer request to keep the experiment out of the current product path | Preserve the branch and evidence only; do not implement, commit into the current line, cherry-pick, merge, or make it a dependency until the maintainer explicitly resumes it |
| `agent-plugin-and-runtime-skill` | Discovery complete; skill-first standalone workflow selected | 2 | Product/integration maintainer | Stable policy review and runtime-build contracts | Reopen only when a concrete shared/installable or server-backed capability is proven necessary |
| `quickstart-and-architecture-docs` | Planned successor | 2 | Documentation/release maintainer | Policy-review fix, CLI audit, and runtime finalization | Create a public-boundary-ready packet for Quick Start, custom runtime build, architecture HTML, and any GitHub Pages publication |
| `cli-catalog-audit` | E2E complete; scoped packet committed | 1 | CLI/product maintainer | Current catalog and capability-ledger inventory | Carry the inventory and no-removal disposition into the coordinator handoff |
| `work-packet-retirement` | E2E complete; scoped audit packet committed | Continuous | Repository maintainer | Packet-level evidence and required gates | Re-run after the next wave; do not delete active or deferred packets based on this audit alone |

## Relevant structure

- Product contract: `docs/00_theses.md`, `docs/01_product_contract.md`, and `docs/09_agent_readiness_validation.md` define the denial/review/retry journey and the custom runtime journey.
- CLI catalog and routing: `internal/cli/runtime_catalog.go`, `internal/cli/tobari.go`, and the catalog contract tests own public command roles, inputs, outputs, and recovery paths.
- Policy review UI: `internal/cli/policy_review_selector.go` and its tests own the TTY/line selection state machine; `internal/app/tobaricmd/service.go` owns the read-only candidate report.
- Runtime boundary: `internal/app/contextcmd`, `internal/domain/tobari/context.go`, `internal/infra/dockerruntime/context_store.go`, and the Context runtime packet own the explicit image-build flow.
- Harness: `tools/repoguard` validates packet status, checkbox completion, successor chains, regular-file boundaries, public content, and documentation shape; `scripts/check.sh` is the completion gate.
- Public documentation: `README.md`, numbered `docs/` contracts, and any future static architecture site must remain consistent and pass the public-boundary checks.

## Constraints

- Discovery and action remain separate. Interactive review may confirm one opaque candidate, but it cannot authorize from display position, recompute an identifier, or batch mutations.
- The catalog remains the only public command source of truth. Any CLI cleanup must update capability, reference-flow, recovery, help, and retirement evidence together.
- Provider-specific auth brokers and generic adapter frameworks are outside the current MVP unless their thesis, product, architecture, and security consequences are deliberately revisited.
- Runtime builds are explicit trusted-host mutations with a bounded Context directory as build context. A future agent integration must not make runtime image selection implicit or allow a project file to redefine the boundary. The Bash shell request does not change the image lifetime command or introduce a shell selector.
- External text, plugin inputs, runtime image contents, and agent output remain untrusted. Credentials and private history must never enter public docs, fixtures, work packets, or plugin interfaces.
- Repository documentation is English under the configured public-boundary policy. User-facing Japanese discussion belongs in the task conversation, not in committed repository docs, unless the governing contract changes.
- Ordinary implementation completion requires `task check`; security, public, release, runtime, and integration work adds the named profiles from the governing documents.

## External facts

- **OpenAI Codex manual, “Build plugins” and “Plugin architecture,”** [official documentation](https://developers.openai.com/plugins/), checked 2026-08-02: a plugin can package skills, an MCP server, or both; Codex and ChatGPT share a universal plugin directory; the documented guidance is to start with a skill when existing tools are enough and use a plugin when a capability should be installed/shared or needs a server-backed integration.
- **OpenAI Codex manual, “Plugins,”** [official documentation](https://learn.chatgpt.com/docs/plugins), checked 2026-08-02: Codex CLI can browse plugins from a configured marketplace, and plugin hooks/connectors/MCP capabilities inherit the host's sandbox and approval boundary. This is relevant to design, not permission to add a plugin now.
- No external Claude Code plugin or distribution fact is treated as settled in this packet. That question requires a focused official-source review before a public compatibility or endorsement claim is written.

## Unknowns

- [ ] Does the interactive failure reproduce only when raw terminal mode is entered, only in the current PTY wrapper, or when the candidate snapshot is empty or changes between calls?
- [ ] Does the screen-clearing behavior hide a correctly rendered queue from the user, or is the selector failing before its first render?
- [ ] What exact key sequence and terminal capabilities should the supported human workflow document, and what cancellation output should remain visible after redraw cleanup?
- [ ] Does the requested “official plugin” mean a Tobari-owned Codex/Claude skill, a universal plugin package, an MCP server, agent-image integration, or a combination?
- [ ] Which plugin capabilities can be implemented with existing local tools and which would require a server, authentication, hooks, or external network access?
- [ ] Which runtime-image actions should be taught by a reusable skill, and which deserve a packaged plugin only after the workflow is proven locally?
- [ ] Which README/architecture artifacts should be static repository docs, generated HTML, or GitHub Pages, and what release/publication owner will verify them?
- [ ] Which catalog commands are compatibility projections rather than redundant user outcomes, and which can be retired without breaking recovery or reference graphs?
- [ ] Which historical packet conclusions are already promoted, and which active packet evidence still depends on the current dirty worktree?

## Thesis evidence

- Repeated design decision or point of agent confusion: the central value is a safe denial-to-retry loop, but the interactive presentation can currently be hard to interpret even when the typed queue exists.
- User outcome or friction observed in the minimal slice: a machine-readable pending permission was visible while the human review path appeared canceled.
- Code workaround or exception being considered: do not add a second policy-management route or bypass opaque references to work around a TTY presentation problem.
- Current thesis that resolves it, or proposed thesis revision: Thesis 8 remains the governing rule; the issue is evidence that its human presentation and test coverage may be incomplete, not evidence for weakening the trust boundary.
- Downstream product, architecture, security, Skill, catalog, and harness impact: a fix may change only CLI presentation/state handling, but it must preserve the catalog workflow, opaque-ID round trip, pre-mutation checks, and agent-readiness denial-to-retry scenario.

## Reproduction or observation

Reported observation, redacted to the public synthetic shape:

```sh
# Inside a Tobari, a network request is denied and returns a fixed host-side review cue.
curl https://example.com | jq

# Human path appeared to cancel without exposing the pending queue.
tobari policy review

# Machine path exposed the pending exact candidate.
tobari policy review --format=json | jq
```

Expected: the TTY path renders the Permission Inbox, allows one candidate to
be inspected, and requires explicit confirmation before delegating one exact
allow or deny action. The deterministic PTY transcript in the policy-review
child packet reproduces the prior early-input/EOF failure mode and verifies the
fixed allow, deny, cancellation, invalid-input, and JSON paths.

## Security and public-boundary notes

- No new side effect is introduced by this coordinator packet. Future policy changes cross the existing host policy mutation boundary; future plugin/MCP/hooks work may add external tools or lifecycle effects and therefore needs an explicit security review.
- The runtime-image build path can download a declared base only after an explicit trusted-host build. Documentation must not turn that exception into an implicit startup pull or an unbounded Docker escape hatch.
- GitHub Pages publication, if chosen, is a public release surface. It must use synthetic examples, reviewed links/assets, repository-local source ownership, and the required public/release checks.
- Work-packet cleanup must preserve evidence, avoid deleting non-empty packets prematurely, and never copy local absolute paths, credentials, private URLs, or shell history into public docs.
- Plugin designs must minimize inputs and returned data, label writes/destructive tools accurately, and avoid credential-bearing interfaces. A plugin is not permission to expose an unrestricted executor.

## First-wave commit reconciliation

The first-wave history has been recovered onto `codex/first-wave`. The
base-runtime-only slice remains separate from this wave, and the auth-broker
experiment remains detached. Each completed first-wave child has a scoped
commit; the coordinator packet is the only current-wave packet still awaiting
its final documentation commit.

| Child | E2E / gate result | Commit state | Disposition |
|---|---|---|---|
| `policy-review-tty` | Real PTY allow/deny/cancel/invalid/JSON passed; `task check`, `task public:check`, and `task security` passed; real cluster integration stopped before review | `7d096bb` | Committed; preserve the fixed denial-to-review-to-retry journey for the docs successor |
| `agent-integration-discovery` | Workflow E2E passed; isolated `task check` and `task public:check` passed | `8e1adfa` | Committed; plugin/MCP remains deferred, skill-first conclusion retained |
| `base-runtime-only` (outside this wave) | Separate runtime-image retirement slice was committed; it removes the maintained Claude/Codex image variants and updates the base-only contract | `0ec04f8` | Preserve as a separate task; do not fold it into a first-wave child or treat it as auth-broker work |
| `runtime-bash-shell` | Base/embedded Bash, `/bin/bash` TTY entry, and Workspace reuse passed; integration stopped at the existing policy-review case with exit 130 | `912c602` | Committed; retain the gate blockers as evidence and carry the verified contract into the docs successor |
| `cli-catalog-audit` | Clean build, help, 25 representative argv paths, faults/recovery, and side-effect boundaries passed; Docker-positive flow blocked | `a007059` | Committed; no safe removal candidate identified |
| `work-packet-retirement` | Classifier and deferred-branch boundary E2E passed; no cleanup target found; isolated gates passed | `92d742c` | Committed as evidence; do not delete active or uncertain packets |
| `auth-broker-deferral` | Governance E2E passed | Intentionally none | Parked and detached; no current-wave commit, merge, cherry-pick, or dependency |

The current Git state is branch `codex/first-wave` at `92d742c`, with the
first-wave commits `7d096bb`, `8e1adfa`, `912c602`, `a007059`, and `92d742c`
in order after the recovered baseline. The worktree contains only the
uncommitted `auth-broker-deferral` and coordinator packets. The
`base-runtime-only` changes remain captured separately in `0ec04f8`, and
`codex/auth-broker` remains detached and unmerged. No rebase, reset, or cleanup
operation is pending.

## Glossary

- **Coordinator packet:** this temporary packet that routes issues; it does not implement every issue.
- **Child packet:** a bounded successor packet for one implementation or decision slice.
- **Permission Inbox:** the interactive TTY projection of retained pending exact policy candidates.
- **Opaque candidate:** a validated policy reference consumed unchanged by one exact policy action.
- **Detached branch:** a branch intentionally kept outside the current `main` product path until a resumption condition is met.
