# Context: Remove inherited foundry scaffolding

## Verified facts

- `.harness/project.json` has profile `ready` and concrete Tobari identity.
- The production product contract lists no `sample` command.
- Synthetic sample domain, application, infrastructure, CLI, and test packages
  remain in the repository.
- The composition root still constructs the sample service even though the
  production catalog excludes its commands.
- `tools/bootstrap`, its repository Skill, Task aliases, and first-run
  documentation remain even though bootstrap rejects the `ready` profile.
- ADR 0003 explicitly identifies manual migration as the path for an existing
  ready repository.
- `docs/work/_template`, `docs/decisions/0000-template.md`, and
  `Formula/tobari.rb.template` are active authoring or packaging inputs rather
  than inherited product identity.

## Constraints

- Preserve the four-layer dependency rule and catalog-derived routing.
- Preserve project metadata needed by public and release gates.
- Do not remove generic infrastructure that current Tobari contracts or gates
  still exercise.
- Public documentation remains English.

## Unknowns to resolve through checks

- Which catalog framework tests rely on the synthetic sample solely as a
  fixture and need a test-local replacement.
- Which projectconfig and repoguard assertions genuinely require protected
  provenance defaults after bootstrap implementation is removed.
