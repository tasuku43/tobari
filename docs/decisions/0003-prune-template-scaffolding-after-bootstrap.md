# ADR 0003: Prune template-only scaffolding during bootstrap

- Status: Proposed
- Date: 2026-07-20
- Deciders: Agentic CLI Foundry maintainers
- Scope: Bootstrap, repository shape, and derived-project maintenance
- Supersedes: None
- Superseded by: None

## Context

A production derived repository kept the one-shot bootstrap command, its tests,
the bootstrap Skill, Task aliases, and first-run documentation after its profile
became `ready`. The command could no longer run and no runtime, catalog, release,
or public task consumed it. Removing only the executable files would leave
broken commands and links, while adding a later `--prune` phase would introduce
an intermediate identity-ready-but-not-pruned state.

## Decision drivers

- A ready derived repository should not own template-maintenance code it cannot use.
- Identity replacement, documentation consistency, and rollback must remain fail-closed.
- Template provenance and ready-profile public guard still need project metadata.

## Proposed decision

Bootstrap apply should transactionally:

1. perform validated identity content updates and renames;
2. remove exact marker-delimited template-only sections from AGENTS, Taskfile,
   Skills, and first-run documentation;
3. prune an explicit allowlist containing the bootstrap command/tests, bootstrap
   Skill/interface metadata, and superseded integrated template design;
4. switch the profile to `ready`; and
5. roll back updates, renames, prunes, and profile together on any failure.

The template profile must require the scaffold and every marker exactly once.
The ready profile must require the reusable capability Skill while rejecting
every pruned path and leftover marker. Project config, project metadata, and the
protected template defaults remain because ready public/release checks consume
them as identity and provenance.

## Consequences

Derived repositories start capability work with less irrelevant code and fewer
instructions. Bootstrap becomes moderately more complex because deletion joins
its transaction. The template remains the sole owner of bootstrap tests.

## Mechanical enforcement required before acceptance

- One shared manifest for exact prune paths and marker pairs.
- Dry-run output and zero-mutation tests for updates, renames, and prunes.
- Apply tests proving exact deletion, surrounding-content preservation, and no
  dangling commands or links.
- Failure-injection tests proving reverse-order rollback and no residue.
- Profile-aware repository-guard tests: template requires, ready forbids.
- A simulated derived-tree `fast` and `public` pass.

## Compatibility and migration

No public CLI contract changes. Newly bootstrapped repositories receive the
pruned shape automatically. Existing ready repositories need one reviewed
manual migration or a separate narrowly scoped migration tool; bootstrap must
not become rerunnable merely to retrofit cleanup.

## Security and public-boundary impact

No credentials, destinations, dependencies, or external effects are added.
The prune allowlist and markers are trusted template inputs and must fail closed
when missing, duplicated, unclosed, changed in type, or replaced by symlinks.

## Validation

Acceptance requires focused bootstrap/projectconfig/repoguard tests plus
`task check` and `task public:check` on both template and simulated ready trees.

## Reconsideration signals

Reconsider if transactional deletion proves less reliable than generating a
fresh allowlisted derived tree, or if a reusable bootstrap capability becomes a
real post-ready user task rather than template scaffolding.
