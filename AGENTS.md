# Agent Contribution Guide

This file is the only canonical operating policy for Codex and contributors in this repository. Do not create a second agent-policy copy.

## Read before changing anything

Always read [Project theses](docs/00_theses.md). Then read only the governing
documents selected by the change:

- Public outcome, command, help, output, or compatibility: [Product contract](docs/01_product_contract.md)
- Layer, dependency, catalog, or execution structure: [Architecture](docs/02_architecture.md)
- Authentication, external I/O, mutation, secrets, or untrusted data: [Security model](docs/03_security_model.md)
- Test policy, repository tooling, CI, or gates: [Harness](docs/04_harness.md)
- Publication or release: [Public Repository](docs/05_public_repository.md) and [Release](docs/06_release.md)
- External API capability: [Authentication](docs/07_authentication.md), [External API Contracts](docs/08_external_api_contracts.md), and [Agent Readiness Validation](docs/09_agent_readiness_validation.md)

Read documents in numeric order when several apply. If the scope is unclear,
the change revises a thesis, or it crosses product, architecture, security, and
harness boundaries, read `00` through `04` before acting.

## Bootstrap before capability work

When `.harness/project.json` has `profile: template`, use
[`$bootstrap-derived-cli`](.agents/skills/bootstrap-derived-cli/SKILL.md) before
adding a capability. It is the first-run Codex workflow: resolve the derived
identity, preview and apply the repository bootstrap, verify the result, and
make the first project-specific thesis and security decisions. Do not start
`$add-capability` until bootstrap reports the identity-only stored value
`profile: ready` and the generic product reasoning has been made concrete for
the derived tool.

## Decision precedence

When instructions conflict, use this order:

1. Project theses
2. Security and architecture invariants
3. Accepted architecture decision records
4. The active work packet's goal and context
5. Its plan
6. Its task checklist

Do not silently work around a higher-level rule. If a requested change requires a thesis, trust-boundary, or public-contract change, update and review that decision before implementing the mechanism.

## Thesis lifecycle

Theses are working product hypotheses, not frozen slogans. Improve them through this loop:

1. **Seed.** Before broad implementation, write the smallest north star and theses that can choose the first slice. Mark unknowns instead of inventing certainty.
2. **Test with a minimal slice.** Build one end-to-end capability that is small enough to expose whether the vocabulary, boundaries, and enforcement are useful.
3. **Capture evidence.** Record repeated decisions, agent confusion, extra discovery steps, unsafe escape hatches, review friction, and user outcomes in the active `context.md`.
4. **Revise before routing around the thesis.** When code wants an exception or workaround, first decide whether the implementation is wrong or the thesis is incomplete. Do not normalize the workaround and leave the governing idea stale.
5. **Propagate.** A thesis revision must update affected product, architecture, security, Skill, catalog, and harness contracts in the same change.
6. **Repeat.** Early projects should expect frequent thesis revisions. Mature projects revise less often, but they still change when user, incident, compatibility, or maintenance evidence justifies it.

A thesis change is not complete when only `docs/00_theses.md` changed. Its consequences and mechanical enforcement must agree across the repository.

## Non-negotiable invariants

1. **The public CLI expresses and closes user tasks.** Do not expose a vendor method, arbitrary route, or raw transport escape hatch as a shortcut. A supported outcome owns the deterministic selection, joining, and interpretation needed for routine success without an undeclared parser, provider-notation decoder, source inspection, or exploratory call; a deliberately raw utility must state its narrower promise.
2. **Discovery and action stay separate.** A `RoleDiscover` command may return candidates and opaque references. A `RoleAct` uses exactly one target-binding mode: it either requires a unique opaque reference and accepts it unchanged, or declares one complete command-bound `tool_local` fixed target when the command path alone identifies a CLI-owned singleton. Fixed-target acts produce and consume no references. An action does not rediscover, normalize, decode, or reconstruct an identifier. Required-reference chains must lead back to an invocable producer rather than a closed cycle.
3. **The four-layer dependency direction holds.** Domain has no outward dependency. Application depends on domain. Infrastructure depends on domain contracts. CLI is the composition root.
4. **Every externally visible operation declares an effect.** Use `operation.EffectRead`, `operation.EffectCreate`, or `operation.EffectWrite`; unknown effects fail closed.
5. **Mutations declare intent, target binding, impact, and outcome.** A reference-bound create binds exactly one required opaque CLI input as `parent_input`; a reference-bound write binds a required opaque `target_id_input` whose kind equals `TargetKind` and may bind a distinct required opaque parent. A fixed-target mutation instead declares an explicit empty `target_inputs`, no `parent_input` or `target_id_input`, and a `TargetKind` equal to its fixed target kind; create treats the singleton as creation scope and write as the existing target. `operation.Intent`, `operation.TargetRef`, and every base `operation.Impact` dimension remain mandatory before an adapter performs the operation. After the action call, preserve valid structured outcome faults before generic cancellation; collapse every unclassified result to non-retryable `unclassified_mutation_outcome` with a read-only reconciliation action. Emit a confirmed mutation result through the mutation-complete output boundary so later cancellation cannot turn success into replay permission. Authoritative rate-window evidence is independent from logical retry permission.
6. **Side effects cross one controlled boundary.** Do not give a command or use case an unrestricted executor, filesystem, process, or network client.
7. **The catalog is the public-command and invocation source of truth.** Routing, typed argv parsing, hierarchical human help, agent help, role, reference flow, and command tests derive from `cli.Catalog`; do not create a competing registry. Every input declares value kind, cardinality, omission/default behavior, applicable bounds, and explicit dependencies or conflicts. Root agent help is an outcome/capability index only, with at most 512 encoded bytes per command entry; retrieve inputs, output, authentication, failures, mutation facts, and workflows through an exact-command or namespace selector. Recovery commands are exact catalog paths or `help <exact-path-or-namespace>`; do not append unchecked argv.
8. **Claims are executable.** When adding an invariant, add the type, lint, contract test, or release check that detects its violation.
9. **The public boundary stays clean.** Never add credentials, confidential URLs, private organization identifiers, real personal data, or copied private history.
10. **One gate decides completion.** Finish implementation work only when `task check` passes. Publication work also requires `task public:check`; release work requires `task release:check`.
11. **External calls are bounded and secret-free above infrastructure.** Propagate one context, declare pagination/call policy, and keep OAuth tokens, PATs, and credential-bearing types inside infrastructure.
12. **External text remains untrusted data.** Visible projection protects terminal and TSV/JSON structure by distinguishing backslashes, controls/formats, and Unicode line separators; it does not filter printable prompt-like meaning. Opaque references bypass display projection and retain their exact validated value.
13. **Semantics precede presentation.** Before rendering, validate the declared task identity and every request dimension that the task actually carries: target, parent, and/or scope. A scoped collection's task-owned result retains its declared scope even when empty. Preserve absent versus explicit empty/zero/false and bounded uncertainty whenever those distinctions affect interpretation, and validate returned opaque values against the reference kind required by their semantic field. Each interpretation-sensitive capability supplies task-owned conformance and negative-inference tests; presentation does not invent identity, relationships, completeness, or confidence from labels, order, proximity, quoting, or indentation.

## Layer responsibilities

```text
internal/domain/   Pure vocabulary and invariants. No I/O.
internal/app/      User-task interpretation and ports owned by each use case.
internal/infra/    Concrete adapters. No product-policy decisions.
internal/cli/      Command catalog, arguments, presentation, and dependency wiring.
```

Application packages define the smallest port needed by their task. Infrastructure satisfies that port through Go's structural typing. Application code must not import infrastructure or construct transport-specific requests.

`cmd` and `internal/cli` have no third-party imports in the template. Keep provider SDKs and transports in `internal/infra`. If a derived project needs a presentation-only CLI parser or renderer, first accept an ADR or thesis consequence that explains the need and supply-chain tradeoff; then add only the exact package path to `allowedCLIThirdPartyImports` in `tools/archlint/main.go`, review its license and dependency delta, and add a negative test proving that sibling paths and effectful packages are still denied. Never add a wildcard, module prefix, SDK, or transport to that allowlist.

The default vertical slice is:

```text
cmd/agentic-cli-foundry
  -> internal/cli
  -> internal/app/doctorcmd
  -> internal/domain/doctor and internal/domain/operation
  -> internal/infra/systemdoctor
```

The `sample list` and `sample read --id` pair follows the same layering through `internal/app/samplecmd`, `internal/domain/sample`, `internal/infra/sampledata`, and `internal/cli/sample.go`. It is the reference implementation for discover/act roles and exact opaque-ID flow.

## Working method

For a non-trivial change, create a directory under `docs/work/<change-name>/` starting from [the work-packet goal template](docs/work/_template/goal.md):

- `goal.md`: outcome, non-goals, acceptance criteria
- `context.md`: verified facts, constraints, and unknowns
- `plan.md`: chosen approach, alternatives, risks, and verification
- `tasks.md`: atomic checklist with evidence

Durable conclusions belong in theses, architecture, security, or an ADR. Do not leave lasting policy only in an implementation plan.

Work packets are active-change artifacts, not a second permanent knowledge
base. Use `Retention: temporary` by default and remove a completed packet from
the final tree after promoting its conclusions. `Retention: evidence` is a
narrow exception for raw experiments, incident/release observations, or other
facts that Git history and durable contracts cannot usefully replace; its goal
must name the reason, governing contract, and review/delete trigger.

When the same design choice, workaround, or point of confusion appears twice, treat it as thesis evidence. Record it before adding another local special case.

Use this writing discipline:

- Production code explains **how**.
- Tests state **what** must remain true.
- Commit messages explain **why** the change exists.
- Code comments explain **why not** a plausible alternative.

Observe runtime-only behavior before changing it. Add bounded diagnostics, reproduce the behavior, record the evidence in `context.md`, and then implement the smallest verified fix.

## Adding a command or capability

1. Define the user outcome and test it against the current theses. Revise a weak thesis before adding a code-level exception.
2. Classify the command with `RoleUtility`, `RoleDiscover`, or `RoleAct`; for an act, choose exactly one reference-bound or command-bound `tool_local` fixed target, and declare any reference kinds on structured inputs and output fields so produced/consumed edges are derived.
3. Prefer extending an existing task when the outcome is the same.
4. Add or refine domain vocabulary and its invariants, including declared task
   identity and every target, parent, or scope dimension the task carries;
   contextual kind validation where semantic reference fields exist; and
   explicit absent/empty/zero/false or uncertain states where they affect
   interpretation.
5. Add an application use case with task-specific input, output, and ports.
6. Implement a concrete infrastructure adapter behind those ports.
7. Register one complete `cli.CommandSpec` in `cli.Catalog` and derive routing, typed parsing, hierarchical human help, scoped agent help, capability, role, reference flow, output, prerequisites, and recovery metadata from it. Each input declares `value_kind`, `cardinality`, any omission default, numeric bounds, and explicit `requires`/`conflicts_with` relations; do not reimplement those facts in a handler switch. Keep the root agent index limited to path, namespace, summary, outcome, capability, effect, and role; verify detailed metadata and the executable argv grammar only in scoped help. Dash-prefixed flag values use the equals form and dash-prefixed positional values follow the positional-only marker. Declare delivery (`complete` or `paged`) separately from collection coverage (`not_applicable`, `exhaustive`, `bounded_window`, or `differential_window`); complete delivery does not imply exhaustive provider history. Complete delivery has no public pagination binding. Paged delivery is JSON-only, cannot use `not_applicable`, and binds one optional opaque cursor argument or flag to one same-kind, always-present top-level string cursor beside the JSON envelope, with typed `empty_cursor` completion.
8. Declare `Effect`, `Intent`, `TargetRef`, and `Impact`. Bind create scope through `MutationContract.parent_input`; bind a write's existing target through `target_id_input` and any distinct scope through optional `parent_input`. Make missing, unbound, mismatched, or inconsistent values fail before the side effect.
9. For an external API, bind the secret-free authentication requirement, declare every standard `app/authn.Gate` fault plus any provider-specific fault in the catalog, issue `BindingID` only inside infrastructure, pass the validated session's non-serialized binding unchanged through each authenticated task port, resolve and revalidate that exact infrastructure authentication record immediately before I/O, and declare pagination/call policy, provider fault mapping, and publishable schema fixtures before enabling live I/O. Each mutation endpoint also declares patch/replacement/append behavior and omission/empty/clear/default/unchanged semantics. Each result distinguishes task coverage from provider visibility restrictions and isolates optional semantic enrichment from a valid core record. Never pass credential-bearing clients or provider types into application code.
10. Add unit, contract, opaque-reference round-trip, negative-path,
    hostile-output, recovery, and public-boundary tests in proportion to risk.
    For interpretation-sensitive results, include a presentation-independent
    typed fixture, answer key, every applicable request dimension, an empty-
    scope case when the task returns a scoped collection, and applicable
    negative-inference canaries; record a reviewed routine-success external-
    processing count of zero for supported outcomes.
11. Propagate any thesis change through product, architecture, security, Skill, and harness documents.
12. Run `task check` and replay the relevant agent-readiness scenario.

## Verification commands

```sh
task check:fast
task check
task security
task release:check
task public:check
```

The underlying interface is `./scripts/check.sh fast|full|security|release|public`. Optional local automation must call that interface and must not claim equivalence to a profile it did not run.

Do not weaken a check merely to make a change pass. If a check encodes the wrong policy, update the governing document and test the new policy as part of the same reviewed change.

## Scope and safety

- Preserve unrelated user changes in a dirty worktree.
- Do not rewrite Git history, delete data, publish, or create releases unless the task explicitly requires it.
- Do not fetch or embed external content without verifying its license and integrity.
- Use synthetic fixtures such as `example.com`, deterministic timestamps, and non-secret tokens.
- Keep repository documentation in the explicit `.harness/project.json` `public_guard.documentation_locale` (English in the template). A derived project changes it only with an explicit thesis or product-contract decision. Stable command paths, flags, environment names, fault codes, JSON keys, schema values, and reference kinds remain language-neutral machine identifiers; external text remains untranslated untrusted data.
- Security reports follow [SECURITY.md](SECURITY.md), never public issues containing sensitive details.
