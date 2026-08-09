# Work Context: Configure narrow Context projections

This file records verified facts and unresolved questions. Desired behavior is
identified separately from current behavior.

## Current behavior

- `context shell configure` is the sole public Context setting mutation. It
  accepts one of four allowlisted variables and `default|inherit|literal`.
- The command is catalog-owned, fixed-target, `EffectWrite`, and reports the
  complete selected Context.
- Shell inheritance is resolved from the exported host environment for each
  interactive `docker exec`; an absent inherited value is omitted.
- Context manifests use schema 4 and Context reports use public JSON schema 5.
- Workspace roots and private homes are mounted, but raw host CLI configuration
  and the host home are not.
- The worktree contains unrelated in-progress Auth Broker, runtime-base, site,
  and documentation changes. They are user-owned and must remain intact.

## Relevant structure

- Entry point: `internal/cli/context.go` and the catalog in `internal/cli/runtime_catalog.go`
- Domain rule: `internal/domain/tobari/context.go`
- Application use case: `internal/app/contextcmd/service.go`
- Infrastructure boundary: `internal/infra/dockerruntime/context_store.go`, `context_runtime.go`, and `project_runtime.go`
- CLI catalog or presentation: `internal/cli/catalog.go`, `help.go`, `context.go`, and selector precedents
- Existing tests and harness checks: Context domain/app/CLI/store/runtime tests and the Context shell environment claim row in `docs/04_harness.md`

## Constraints

- `cli.Catalog` remains the only public command and typed argv authority.
- A wizard is human input completion for the same fixed-target mutation, not a
  second hidden mutation path.
- Wizard prompts use terminal stdin/stderr and leave stdout for the declared
  result. JSON and redirected invocations never prompt.
- Direct mode is selected only by a complete setting-input group. Any partial
  group fails rather than prompting for the rest.
- Git identity is personal, non-secret data and remains opt-in for new and
  migrated Contexts.
- Host Git resolution is read-only, fixed-key, bounded, finite, shell-free, and
  excludes repository/worktree config from the trusted host read.
- Generated projection is private runtime-owned state, mounted read-only, and
  lower precedence than Workspace global and repository-local Git config.
- Repository documentation remains English and public fixtures remain synthetic.

## External facts

- Git `git-config` documentation, <https://git-scm.com/docs/git-config>, checked
  2026-08-09: `--global` limits reads to the user's global files; `--includes`
  explicitly follows includes when a scope flag is supplied; `gitdir`
  conditions use the discovered repository location; later values take
  precedence.

## Unknowns

- [x] Command namespace: product owner selected `config shell` and `config git`.
- [x] Interaction trigger: product owner selected a wizard only when setting
      flags are wholly omitted; partial flags remain errors.
- [x] Git identity unit: `user.name` and `user.email` form one atomic policy.
- [x] Default policy: no Git identity projection for new or migrated Contexts.
- [ ] Verify whether the repository gates require a live Docker integration
      run in this dirty worktree or whether deterministic runtime tests provide
      the completion evidence available in this environment.

## Thesis evidence

- Repeated design decision or point of agent confusion: shell and Git are now
  two instances of a narrow non-secret host projection, while Thesis 9 names
  only shell and Thesis 3 literally rejects all host CLI configuration copies.
- User outcome or friction observed in the minimal slice: `context shell
  configure` groups by target internals rather than the user's configuration
  task and provides only an option-heavy path.
- Code workaround or exception being considered: adding a sibling shell-only
  special case or mounting `~/.gitconfig` would route around the governing idea.
- Proposed thesis revision: define Context-owned narrow projection as an
  explicit, allowlisted scalar boundary; continue to prohibit source-file,
  directive, executable, secret, and authentication projection.
- Downstream impact: product/architecture/security/harness, ADR, catalog,
  Context schemas, wizard contract, runtime projection, README, authentication
  clarification, and agent-readiness validation.

## Reproduction or observation

```sh
go run ./cmd/tobari help context
go run ./cmd/tobari help context shell configure
git -C <canonical-root> config --global --includes --null --get user.name
git -C <canonical-root> config --global --includes --null --get user.email
```

Observed on macOS with synthetic temporary configuration: `--global
--includes` selected a `gitdir` conditional identity for the canonical root,
did not read repository-local configuration, and returned exit status 1 for an
absent exact key. A generated lower-scope projection was overridden by later
Workspace/repository configuration. No real identity value was retained.

## Security and public-boundary notes

- Assets and side effects involved: owner-only Context manifest and exact
  per-Workspace projection file; one atomic Context write and at most two
  read-only host Git calls per inherited root reconciliation.
- Credentials or confidential data involved: none. Email may be personal data
  and is never logged or used in fixtures.
- New dependencies, destinations, files, processes, or generated content:
  existing host Git executable launched with an exact environment allowlist;
  no network destination or new module.
- External schema provenance, publication rights, and drift evidence: Git CLI
  behavior is tested against fixed argv and synthetic files rather than copied
  upstream content.
- Output delivery and execution: complete scalar result, no pagination, one
  mutation attempt, finite Git timeout, no retry, idempotent replacement of
  the same Context policy, cancellation before apply causes zero mutation.
- Publication and licensing concerns: none beyond citing public Git documentation.

## Glossary

- Narrow projection: a thesis-declared allowlist of validated non-secret
  scalar values re-encoded into a Tobari-owned boundary without transferring
  the source file, path, directive, executable setting, or credential.
- Direct mode: a complete argv invocation that never prompts.
- Wizard mode: terminal-only collection and review of a wholly omitted setting
  input group before invoking the same application mutation.
- Context fallback: Git identity projected at system scope so Workspace-global
  and repository-local settings can override it.
