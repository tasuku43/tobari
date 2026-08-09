# Work Goal: Configure narrow Context projections

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md`, `docs/01_product_contract.md`, `docs/02_architecture.md`, `docs/03_security_model.md`, and `docs/04_harness.md`
- Review/delete trigger: Delete after durable conclusions are promoted and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: Current change
- Related ADRs: Accepted ADR 0021

## Outcome

A human can configure Context-owned shell presentation and Git commit identity
from one coherent `config` namespace, either through a terminal wizard or a
complete deterministic invocation suitable for agents and scripts. Git
inheritance projects only a validated host `user.name` and `user.email`
fallback; it never copies host Git configuration or authentication state.

## Why now

The existing `context shell configure` command introduced the first narrow host
projection. Adding Git identity exposes that shell-only naming and policy do not
generalize. The product owner selected a configuration-first namespace and an
optional wizard before a second inconsistent command path becomes public
practice.

## Non-goals

- Copying or mounting host shell startup files or Git configuration files.
- Projecting Git credentials, helpers, signing, SSH, hooks, aliases, URL
  rewrites, filters, proxy settings, or arbitrary Git keys.
- Authenticating private Git clone, fetch, or push.
- Changing Workspace-authored global or repository-local Git configuration.
- Prompting for missing direct-mode flags or permitting redirected input to
  enter a wizard.

## Acceptance criteria

- [x] `config shell` replaces `context shell configure` and `config git` adds
      atomic Git identity policy; no compatibility alias remains before v1.0.
- [x] With no setting flags, text success/error mode plus terminal stdin/stderr
      opens a reviewable wizard; complete flags execute directly; partial
      flags, non-terminal wizard attempts, and JSON wizard attempts fail before
      mutation.
- [x] `config git --source inherit` resolves only the host global
      `user.name`/`user.email` pair for the stable Workspace root at entry and
      installs it as a lower-precedence fallback than Workspace global and
      repository-local configuration.
- [x] Host files, paths, include directives, authentication, executable Git
      settings, and raw diagnostics never cross into the Workspace or result.
- [x] Context schema-4 migration preserves every existing shell setting and
      adds no implicit Git identity; Context report JSON schema 6 reports the
      complete configuration state.
- [x] Human and agent help expose both interaction modes without exploratory
      calls; routine success requires zero external reconstruction steps.
- [ ] `task check`, `task security`, `task public:check`, and the relevant
      runtime/agent-readiness scenarios pass.

## Governing documents

- Thesis: 3 (host/Workspace boundary), 5 (catalog contract), and 9 (Context composition)
- Product contract section: public commands, Context input/configuration, side effects, compatibility
- Architecture or security invariant: four-layer dependency direction, controlled external I/O, clean public boundary
- Existing ADR: ADR 0009 retains tool-native Git authentication and rejects host Git configuration inheritance

## Completion definition

The work is complete when acceptance criteria have evidence, durable decisions
have been promoted to numbered documentation and ADR 0021, required profiles
pass, temporary diagnostics are removed, and this packet is removed from the
final tree.
