# Documentation map

This repository separates current product knowledge from work-in-progress
notes. The current contract is the numbered documentation and accepted
decision records, while [`cli.Catalog`](../internal/cli/catalog.go) is the
executable public-command authority. `docs/work` is not a second specification.

## Start here

| Need | Read | What it answers |
|---|---|---|
| Use Tobari | [`README.md`](../README.md) | What the product does and how to run the first bounded workflow |
| Understand the current contract | [`00_theses.md`](00_theses.md) through [`09_agent_readiness_validation.md`](09_agent_readiness_validation.md) | Product outcome, public CLI, architecture, security, checks, release, authentication, external contracts, and readiness |
| Review a long-lived design choice | [`decisions/`](decisions/0000-template.md) | Why an accepted architecture or security decision exists, and what it superseded |
| Review abuse cases | [`THREAT_MODEL.md`](THREAT_MODEL.md) | Assets, trust assumptions, abuse cases, accepted risks, and reconsideration triggers |
| Read the public documentation | [Published site](https://tasuku43.github.io/tobari/) ([source and maintenance](architecture-site/README.md)) | Guided explanations, diagrams, and generated reference derived from the governing documents and CLI Catalog |
| See unfinished work | [`work/_template/goal.md`](work/_template/goal.md) | Only active or explicitly deferred work with a named owner and review trigger |

Read the numbered documents in order when a change crosses several boundaries.
The short purpose of each document is:

| Document | Current role |
|---|---|
| [`00_theses.md`](00_theses.md) | North star, product hypotheses, and deliberate non-goals |
| [`01_product_contract.md`](01_product_contract.md) | Supported outcomes, public vocabulary, CLI behavior, lifecycle, and compatibility |
| [`02_architecture.md`](02_architecture.md) | Layers, catalog ownership, runtime topology, and execution flow |
| [`03_security_model.md`](03_security_model.md) | Trust boundaries, assets, side-effect controls, credentials, and fail-closed rules |
| [`04_harness.md`](04_harness.md) | Executable checks, E2E evidence, and completion gates |
| [`05_public_repository.md`](05_public_repository.md) | Public-boundary, license, and publication review |
| [`06_release.md`](06_release.md) | CLI and OCI image release boundaries and required gates |
| [`07_authentication.md`](07_authentication.md) | Tool-native authentication, retained managed mode, and deferred experiments |
| [`08_external_api_contracts.md`](08_external_api_contracts.md) | Gateway and OPA protocol contracts |
| [`09_agent_readiness_validation.md`](09_agent_readiness_validation.md) | New-user and coding-agent readiness scenarios |

## Current open work

The temporary
[`default-runtime-brokered-tools`](work/default-runtime-brokered-tools/goal.md)
packet is active. It keeps requested provider tools in an optional local
Context toolbox and adds reviewed host GitHub/AWS credential drivers plus a
private resident companion for post-policy AWS refresh. The currently pinned
Gateway/Auth Broker images are API v1; publishing and pinning compatible API-v2
indexes is a separate explicitly authorized release blocker.

The following are deliberate product boundaries, not completed features:

- Public Claude and Codex image publication is deferred until redistribution,
  licensing, support, and provenance decisions are accepted. See
  [Release Model](06_release.md).
- Provider-specific policy operations, arbitrary helper code, general TWG
  login/refresh, and multiple-account selection remain excluded. The sole
  dynamic plan is reviewed host AWS CLI credential export after exact OPA allow
  followed by Broker-owned bounded SigV4. See [Authentication
  handling](07_authentication.md).
- Simplifying explicit cluster bootstrap remains a product hypothesis under
  the bounded-autonomy thesis, not an unreviewed command change. See
  [Agent Readiness Validation](09_agent_readiness_validation.md).

## How to distinguish current and historical material

- The numbered documents and ADRs with `Status: Accepted` state the current
  contract. An ADR with `Status: Superseded` is historical reasoning; follow
  its `Superseded by` link instead of treating it as current behavior.
- `README.md` is the concise user-facing route. The published documentation is
  an explanatory and generated-reference projection; it cannot add a command,
  permission, or guarantee beyond the numbered documents and CLI Catalog.
- A work packet is task-time material. Its `goal.md` status identifies whether
  it is `Draft`, `Accepted`, or `Active`; completed temporary packets are
  removed rather than used as a history archive. Evidence packets are retained
  only when their raw experiment or release observation cannot be replaced by
  a contract or executable test, and each one must state its deletion trigger.
- Closed implementation history belongs in Git. Do not preserve a plan,
  transcript, or handoff in the normal documentation path solely because it
  records how a completed change was made.

## Decision precedence

When two documents disagree, use this order:

1. Project theses
2. Security and architecture invariants
3. Accepted ADRs
4. Active work-packet goal and context
5. Active plan
6. Task checklist

The root [`AGENTS.md`](../AGENTS.md) turns this order into contribution policy.
A lower-level document cannot grant an exception to a higher-level invariant.

## Work packet lifecycle

Create a packet from [`work/_template/goal.md`](work/_template/goal.md) for a
non-trivial change. Keep it small and delete it in the same handoff that
promotes its durable conclusions. Use `Retention: evidence` only for a narrow
raw experiment, incident, or release observation that the product contract and
tests cannot replace. Do not create a separate backlog or an index of finished
packets; the active packet links above are enough.

`task check` runs the repository guard that validates packet status, completion
checklists, successor paths, and the terminal-retention rule. A terminal packet
with temporary retention is a lifecycle error, not a reason to keep it around.

## Documentation language

Repository documentation is public and written in English. Stable command
paths, flags, output keys, and reference kinds remain language-neutral; output
received from external systems remains untrusted data.
