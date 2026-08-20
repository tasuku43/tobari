# Work Context: Define Context as a stable reusable work mode

## Current behavior

- `docs/02_architecture.md` calls Context the user-facing immutable capability
  envelope and describes one manifest containing stable identity, immutable
  source/policy facts, exact Runtime binding, agent profile, shell policy, and
  Git identity policy.
- ADR 0029 makes required `source_access` and the Context-owned policy snapshot
  immutable creation facts. That authority decision remains valid.
- ADR 0067 makes Runtime installation-wide and reusable, while each Context
  stores one exact binding that `context runtime set` may explicitly replace or
  roll back. Existing Workspaces adopt it on next entry and preserve home.
- `config shell` and `config git` explicitly mutate narrow Context-owned
  projections. Child sessions late-bind shell settings; root entry resolves the
  Git identity fallback without granting authentication or signing authority.
- Context bootstrap adapters are mutable recipes only for future Workspace
  creation. Existing Workspace homes retain their create-time revision and are
  never synchronized or rewritten.
- The Workspace key is canonical project root plus stable Context ID.
  `context use` changes only the omitted-input default and cannot retarget an
  existing Workspace.
- Enabled native readiness stores an immutable Context choice but uses the
  installed Tobari binary's current reviewed compatibility revision inside the
  Context's terminal destination and method ceilings.

## Relevant structure

- Entry point: `context create`, `context show`, `context runtime set`,
  `config shell`, `config git`, bootstrap commands, and root `tobari`
- Domain rule: Context manifest, Runtime binding, shell/Git configuration, and
  bootstrap recipes under `internal/domain/tobari`
- Application use case: `internal/app/contextcmd` and root reconciliation under
  `internal/app/tobaricmd`
- Infrastructure boundary: Context store, Runtime store, project runtime
  reconciliation, shell/Git host readers, and bootstrap projection
- CLI catalog or presentation: Context/Runtime/config specs and renderers under
  `internal/cli`
- Existing tests and harness checks: strict manifest readers, immutable
  source/policy tests, Runtime next-entry adoption, configuration projection,
  bootstrap future-only behavior, catalog contracts, and ADR/harness claims

## Constraints

- A Context name is a selector; stable ID carries authority and permanent
  Workspace binding.
- Source access, Context-owned policy snapshot, policy revision, terminal
  destination/method ceilings, and native-readiness participation choice do
  not gain a mutation path.
- The installed binary's compatibility rules remain independently revisioned
  and cannot exceed Context ceilings.
- Runtime source and history remain installation-owned; a Context stores only
  one validated ready revision binding.
- Shell/Git projections remain narrow, non-secret, and do not become arbitrary
  environment, dotfile, helper, signing, or credential ingress.
- Bootstrap remains typed, secret-free, and future-Workspace-only.
- This conceptual correction cannot introduce a compatibility reader, implicit
  default, per-Workspace flag, or second source of truth.

## External facts

No external source is required. The decision follows verified repository
behavior, accepted ADRs, and product-owner approval of the 2026-08-21 concept
review.

## Unknowns

- [ ] Inventory every durable/public statement that treats the whole Context,
      rather than only its Boundary facts, as immutable.
- [ ] Map each shell and Git setting to its exact current resolution and
      activation point in code and tests; avoid claiming home mutation or
      future-only behavior where none exists.
- [ ] Decide whether one new ADR should revise ADR 0029 and relate ADR 0067, or
      whether an explicit revision to an existing ADR is repository-conformant.
- [ ] Determine the exact disposition of the older active
      `context-capability-envelope` packet without losing unfinished runtime
      evidence owned by its child packet.
- [ ] Identify missing mechanical checks proving there is no mutation route for
      Boundary fields and no silent home rewrite for mutable settings.

## Thesis evidence

- Repeated design decision or point of agent confusion: Context is called
  immutable while several catalog-owned Context mutations are intentional and
  tested.
- User outcome or friction observed in the minimal slice: a user cannot tell
  whether changing Runtime or defaults violates the Context boundary or should
  require creating another Context.
- Code workaround or exception being considered: documenting each mutable
  command as a local exception would preserve a false top-level definition.
- Current thesis that resolves it, or proposed thesis revision: Context is a
  stable reusable work mode; its Boundary is the immutable capability envelope.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: Thesis 9, ADR 0029/0067 relation, Context command descriptions,
  activation timing, mutation-route tests, claim-to-enforcement mapping, and
  later onboarding/derivation UX.

## Reproduction or observation

```sh
rg -n 'immutable capability envelope|context runtime set|config shell|config git|future Workspace|next entry' \
  docs/00_theses.md docs/01_product_contract.md docs/02_architecture.md \
  docs/03_security_model.md internal/cli
```

Observed 2026-08-21: durable documents simultaneously assert whole-Context
immutability and declare reviewed Context mutations with distinct activation
timing.

## Security and public-boundary notes

- Assets and side effects involved: Context manifest/policy snapshot, Runtime
  binding, shell/Git projections, bootstrap recipes, Workspace reconciliation,
  catalog and durable security claims
- Credentials or confidential data involved: none added; tool-owned login
  remains in Workspace home and experimental secrets remain infrastructure-owned
- New dependencies, destinations, files, processes, or generated content: one
  durable ADR may be added; no runtime dependency or destination
- External schema provenance, publication rights, and drift evidence: all
  affected schemas and contracts are Tobari-owned
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: unchanged; existing mutation completion
  and cancellation contracts remain
- Publication and licensing concerns: none beyond normal public documentation

## Glossary

- **Context:** a stable host-owned reusable work mode bound by stable ID.
- **Boundary:** the creation-time immutable source/network authority and
  compatibility ceiling within a Context; a conceptual section, not a new
  public resource.
- **Runtime binding:** the one exact ready Runtime revision selected by a
  Context and explicitly replaceable through the catalog-owned mutation.
- **Session defaults:** narrow shell presentation and Git identity fallback
  resolved for later child sessions without rewriting Workspace home.
- **Creation defaults:** typed bootstrap recipes applied once only to future
  Workspace homes.
