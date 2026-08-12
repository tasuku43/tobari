# Work Goal: Publish the smallest defensible Tobari V1

- Status: Active
- Retention: temporary
- Retention reason: None
- Governing contract: `docs/00_theses.md` through `docs/09_agent_readiness_validation.md`
- Review/delete trigger: Delete after the V1 scope is promoted, the release is published, and the change completes
- Successor: None
- Owner: Tobari maintainers
- Target: First public V1 release
- Related ADRs: ADRs 0009, 0010, 0016, 0019, 0020,
  `0021-add-datadog-pup-oauth.md`,
  `0021-context-owned-narrow-host-projections.md`, 0023, 0025, 0026, 0027,
  and 0028
- Related work: [Context capability envelope](../context-capability-envelope/goal.md),
  [Context source access](../context-source-access/goal.md),
  [policy compaction retirement](../policy-compaction-retirement/goal.md),
  [policy presets](../policy-presets/goal.md),
  [V1 authentication narrowing](../v1-auth-narrowing/goal.md), and
  [release artifacts](../first-public-release-artifacts/goal.md)

## Outcome

Tobari has one installable, public V1 that gives a coding agent a bounded
Docker-backed Workspace, authorizes its outbound HTTP effects at the trusted
Context/project boundary, and optionally mediates a static credential without
placing the primary secret in the Workspace. The release makes no stronger
microVM, snapshot-integrity, request-body, provider-business-operation, or
general agent-capability claim. A host-owned Context makes source access and a
snapshotted network-policy preset visible before Workspace creation.
High-maintenance provider refresh/signing plans and unproven permission
compaction are absent from the public surface.

## Why now

The source tree contains the intended V1 enforcement path, but the canonical
Gateway and Auth Broker image authorities are still `unpublished`, and the
repository has no public release. At the same time, the product contract owns
six built-in credential plans, a trusted-host companion, static managed
injection, and prefix compaction before the central HTTP authorization outcome
has been published and observed. That maintenance surface obscures the
smallest differentiating product and blocks a release that users can test.

## Non-goals

- Adding clone, overlay, snapshot, or apply-back filesystem modes.
- Adding a microVM, remote executor, private Docker daemon, or runtime-backend
  plugin abstraction.
- Adding MCP interception or tool-call authorization.
- Adding temporary, session, allow-once, or time-based permissions.
- Making resource limits configurable; V1 retains the trusted fixed defaults.
- Adding per-Workspace Gateway, OPA, or Auth Broker instances, quotas, or
  fairness controls.
- Authorizing ordinary HTTP request bodies, GraphQL arguments or variables,
  arbitrary TCP, UDP, QUIC, SSH, or private destinations.
- Platform code signing, notarization, or a reproducible-build claim. The V1
  release still requires checksums, SBOMs, and CI-generated provenance.
- Preserving or migrating unpublished development state from a prior source
  snapshot.

## Acceptance criteria

- [ ] The theses and public documentation define Tobari as bounded execution,
      HTTP effect authorization, and credential mediation, and do not position
      it as a stronger sandbox than a microVM product.
- [ ] The HTTP authorization claim names its actual dimensions: Context,
      project, scheme, host, port, method, and path; declared GraphQL endpoints
      additionally use operation type and root field. It explicitly excludes
      ordinary bodies, query values, headers, GraphQL arguments, and variables
      from permission identity.
- [ ] The retained command and security contract is the one in `plan.md`, with
      one generic static broker plan plus the GitHub trusted-host acquisition
      flow; dynamic AWS, Datadog, and OpenAI plans, the helper-acquired
      Anthropic plan, the credential companion, static managed injection, and
      policy compaction are removed.
- [ ] Context is documented and enforced as a host-owned capability envelope:
      every new Context fixes direct source access to `read-only` or
      `read-write` and snapshots one immutable built-in or owner-authored
      network-policy preset revision.
- [ ] V1 includes `builtin/offline`, `builtin/reviewed-exact`, and
      `builtin/get-only-reviewed`, plus strict local custom preset
      init/list/show/validate and Context selection. Presets distinguish
      terminal guardrails from baseline grants and learned exact rules.
- [ ] Tool-native authentication is labeled Workspace-owned, while `auth`
      commands and status are labeled brokered; no output implies that every
      credential stays outside the Workspace.
- [ ] Direct read-only/read-write source mounting, fixed resource limits, the
      Docker runtime, and the installation-shared Gateway/OPA/Auth Broker are
      retained as explicit V1 limits and trusted boundaries. Read-only is not
      described as a snapshot, a wholly read-only Workspace, or protection
      against another host process or read-write Context.
- [ ] Unsupported retired commands, provider selectors, plan variants, state
      shapes, and adapter selectors fail closed with no alias, hidden fallback,
      dormant transport, or compatibility reader.
- [ ] Immutable Linux amd64/arm64 Gateway and Auth Broker V1 image indexes are
      published and digest-pinned; supported CLI archives, checksums, SBOMs,
      and provenance are published through one ordinary GitHub Release and the
      documented Homebrew install path succeeds.
- [ ] Routine agent use discovers the retained outcome through catalog-derived
      help, follows opaque policy references unchanged, and requires zero
      undeclared parsing or provider-notation interpretation.
- [ ] `task check`, `task security`, `task public:check`, and
      `task release:check` pass, and the bounded manual release observations in
      the release and authentication contracts are recorded.

## Governing documents

- Thesis: `docs/00_theses.md`, especially product outcome, isolation boundary,
  policy learning, authentication, and release claims
- Product contract section: `docs/01_product_contract.md`, command catalog,
  Context composition, policy identity, authentication, and compatibility
- Architecture or security invariant: four-layer dependency direction,
  controlled side-effect boundaries, fail-closed HTTP enforcement, and
  post-policy project-bound broker resolution
- Existing ADR: ADR 0010 for the direct mount, ADR 0019 for the static broker
  core, ADR 0026 for transparent HTTP enforcement, ADR 0027 for the pre-public
  V1 reset, and ADR 0028 for exact-domain policy source

## Completion definition

The work is complete when acceptance criteria have evidence, the narrowed V1
decision supersedes or revises the affected ADRs, durable contracts and
generated documentation agree with code, release artifacts are published and
verified, required profiles pass, development-only state and temporary
diagnostics are removed, and this temporary packet is removed from the final
tree.
