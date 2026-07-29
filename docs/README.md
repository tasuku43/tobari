# Documentation Map

This directory contains the durable reasoning for Agentic CLI Foundry. Read the numbered documents in order when starting a derived project or making a change that crosses product, architecture, security, harness, publication, or release boundaries.

In a newly derived repository, Codex starts with [`$bootstrap-derived-cli`](../.agents/skills/bootstrap-derived-cli/SKILL.md). After identity and initial project reasoning are concrete, recurring capability work uses [`$add-capability`](../.agents/skills/add-capability/SKILL.md).

| Document | Purpose | Primary readers |
|---|---|---|
| [00_theses.md](00_theses.md) | North star, thesis-learning lifecycle, and principles used to resolve ambiguous decisions | Everyone |
| [01_product_contract.md](01_product_contract.md) | Users, supported outcomes, public vocabulary, compatibility, and non-goals | Product owners, contributors, agents |
| [02_architecture.md](02_architecture.md) | Four layers, catalog, typed effects and intent, and execution flow | Contributors, agents, reviewers |
| [03_security_model.md](03_security_model.md) | Assets, actors, trust boundaries, abuse cases, and required controls | Everyone changing side effects or data handling |
| [04_harness.md](04_harness.md) | How written claims become local and CI checks | Contributors, agents, maintainers |
| [05_public_repository.md](05_public_repository.md) | Clean-room derivation, sanitization, licensing, and public-readiness review | Maintainers and release owners |
| [06_release.md](06_release.md) | Versioning, artifact construction, provenance decisions, and release procedure | Release owners |
| [07_authentication.md](07_authentication.md) | Secret-free OAuth/PAT boundary and derived security decisions | Security owners, adapter authors, agents |
| [08_external_api_contracts.md](08_external_api_contracts.md) | Pagination, retry/idempotency, schema, capability, and API adapter contracts | Adapter authors, agents, reviewers |
| [09_agent_readiness_validation.md](09_agent_readiness_validation.md) | Scenario-based discovery, execution, interpretation, and recovery validation | Product owners, agents, reviewers |

Additional directories serve different lifetimes:

- The [decision template](decisions/0000-template.md) starts durable architecture decision records. An ADR is never edited to hide an old decision; a later ADR supersedes it.
- The [work-packet goal template](work/_template/goal.md) starts bounded work packets. Facts and plans there are temporary unless promoted into a durable document.

Root community documents have stable conventional locations:

| Document | Purpose |
|---|---|
| [`README.md`](../README.md) | User and template-adopter entry point |
| [`AGENTS.md`](../AGENTS.md) | Canonical contribution policy for humans and agents |
| [`CONTRIBUTING.md`](../CONTRIBUTING.md) | Contribution workflow and review expectations |
| [`CODE_OF_CONDUCT.md`](../CODE_OF_CONDUCT.md) | Participation standards and private conduct reporting |
| [`SUPPORT.md`](../SUPPORT.md) | Support channels, required evidence, and boundaries |
| [`SECURITY.md`](../SECURITY.md) | Supported versions and private vulnerability reporting |
| [`LICENSE`](../LICENSE) | Repository license terms |

## Decision precedence

When two documents disagree, use this order:

1. Theses
2. Security and architecture invariants
3. Accepted ADRs
4. Active work packet goal and context
5. Active plan
6. Task checklist

The root [AGENTS.md](../AGENTS.md) turns this order into contribution policy. A lower-level document cannot grant an exception to a higher-level invariant.

## Stable reasoning and task-time instructions

Durable documentation explains **why the system has its current shape**. Task-time instructions explain **how to perform a recurring change safely**. Keep long procedural checklists out of a thesis or architecture overview; place them in a focused work template, repository tool, or agent skill and link back to the governing invariant.

Conversely, do not leave a durable product or security decision only in a work plan. Promote it to a numbered document or ADR before closing the work packet.

## Derived-project documentation pass

Bootstrap changes identity, not intent. Before adding real capabilities, a derived project must:

1. Rewrite the generic north star and success measures.
2. Name its primary users, supported tasks, and explicit non-goals.
3. Document every credential, data store, subprocess, filesystem write, and network destination; complete the authentication and external-API decisions when applicable.
4. Decide compatibility and release promises.
5. Bind each important claim to a type, test, lint, or release check.
6. Run `task check` and `task public:check`.

All repository documentation is public and written in English by default. A
derived project may adopt another language only through an explicit thesis or
product-contract decision and the matching
`.harness/project.json` `public_guard.documentation_locale`. The repository
guard has only a narrow English/Japanese trusted-Markdown canary; maintainers
remain responsible for broader linguistic review and understandable
public-boundary checks.
