# Plan: Remove inherited foundry scaffolding

## Chosen approach

1. Retire the synthetic sample capability across all four layers.
2. Replace any framework-level test use with test-local command specifications
   so generic catalog invariants remain covered without production residue.
3. Remove the one-shot bootstrap command, Skill, Task aliases, and derived-
   repository documentation.
4. Keep project metadata and intentional work/release templates used by active
   gates.
5. Rewrite remaining public template language as concrete Tobari policy.
6. Run focused tests after each slice, then full, security, and public gates.

## Alternatives rejected

- Hiding sample commands only: dormant production wiring and maintenance burden
  would remain.
- Keeping bootstrap for provenance: immutable defaults and project metadata can
  retain provenance without an unusable executable workflow.
- Removing every path containing `template`: this would delete active work-
  packet, ADR, and Formula generation inputs.

## Risks

- Removing sample fixtures could accidentally reduce catalog invariant
  coverage. Test-local fixtures must preserve those checks.
- Repoguard may encode template-profile requirements. Ready-profile behavior
  must remain fail closed while unused template-profile rules are removed only
  when their implementation owner is removed.
- Public guidance may retain dangling links after bootstrap files are deleted.
  Public-boundary checks must catch them.

## Verification

- Focused Go tests for CLI, project metadata, repoguard, and architecture
- Negative CLI test for the removed `sample` namespace
- `task check`
- `task security`
- `task public:check`
