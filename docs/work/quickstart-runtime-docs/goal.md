# Work Goal: Quick Start runtime documentation

- Status: Complete
- Retention: evidence
- Retention reason: Preserve the second-wave Quick Start/runtime journey and its bounded E2E and gate evidence until a repeatable public documentation replay replaces this packet.
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the documented journey is replaced by a repeatable harness-backed public proof and its conclusions are promoted to durable contracts if needed
- Successor: None
- Owner: Tobari documentation maintainer
- Target: Current main product line, second-wave Quick Start and runtime customization
- Related ADRs: `docs/decisions/0016-context-managed-runtime-recipes.md`

## Outcome

A new user can start in a project directory, explicitly start the shared
cluster, enter a CWD-owned Tobari, observe a denied `curl` request, hand the
exact secret-free request to the trusted host for review, allow one opaque
candidate, retry the same request, then initialize and edit the active Context
Dockerfile, run the explicit `runtime build`, and enter the resulting runtime.
The README states which steps belong to the host and which belong to the
agent, keeps recovery commands exact, and does not imply an automatic image
pull or policy retry.

## Why now

The current CLI and integration harness already close the denial/review/allow/
retry and Context runtime workflows, but the README presents them across
separate sections and includes a stale retired `tobari exec` example. A concise
second-wave journey is needed to make the safe path runnable without teaching a
new user Docker, OPA, or policy internals.

## Non-goals

- No production Go, CLI catalog, command, flag, output, image authority, or
  workflow change.
- No architecture numbered-contract, architecture HTML, GitHub Pages,
  auth-broker, credential, or external-adapter change.
- No change to existing first-wave work packets.
- No new plugin, MCP server, skill, generic executor, or authentication path.
- No implicit network pull, direct Docker command in the supported journey, or
  project-local runtime authority.
- No real credentials, private URLs, machine-specific paths, shell history, or
  non-synthetic external data in the public docs or transcript.

## Acceptance criteria

- [x] README Quick Start is concise and runnable from a project directory,
      with prerequisites and explicit cluster startup. Evidence: the new
      numbered Quick Start path begins with `doctor --root .` and `cluster up`.
- [x] README shows an in-Tobari denied curl using synthetic public values,
      host-side policy review/allow, exact opaque-reference recovery, and a
      same-request retry. Evidence: the `example.com/quickstart` PUT path and
      review/allow instructions are explicit.
- [x] README states the host/agent boundary, exact failure/recovery commands,
      and that `runtime build` is explicit. Evidence: the boundary paragraph
      and Failure and recovery section name the catalog paths.
- [x] README shows `runtime init`, editing the active Context Dockerfile to add
      a harmless tool, explicit `runtime build`, and a `tobari` entry. Evidence:
      the `tree` example and real bounded runtime replay are recorded.
- [x] `e2e-transcript.md` records an executable repository-harness or bounded
      Docker replay, including exact stop/output when an environment blocker
      prevents success; it does not claim an unrun success. Evidence: aggregate
      integration/runtime exit 130 and the successful bounded runtime replay
      are recorded separately.
- [x] Only the six allowed paths change, `git diff --check` passes, and
      `task check` plus `task public:check` pass. Evidence: clean detached
      verification checkout passed both gates; final staging is explicit.

## Governing documents

- Thesis: [Project theses](../../00_theses.md), especially the North Star and
  Theses 0, 4, 8, and 9.
- Product contract: [Product Contract](../../01_product_contract.md), public
  commands, denial recovery, Context runtime, and side effects.
- Architecture: [Architecture](../../02_architecture.md), catalog ownership,
  CWD target, and controlled Docker boundary.
- Security: [Security Model](../../03_security_model.md), trust boundary,
  deny-by-default policy, and explicit runtime-build boundary.
- Harness: [Harness](../../04_harness.md), documentation command checks and
  integration/runtime profiles.
- Public/release: [Public Repository](../../05_public_repository.md) and
  [Release Model](../../06_release.md), synthetic public-safe docs and gates.
- Agent readiness: [Agent Readiness Validation](../../09_agent_readiness_validation.md),
  policy-learning and runtime-customization scenarios.

## Completion definition

This work is complete when the README and packet describe only existing public
commands, the E2E transcript records the actual bounded replay and any
environment blocker, the required gates have evidence, the allowed-path diff is
clean, and one scoped commit is created on `main` without changing branch
history beyond that commit.
