# Work Context: Publish the smallest defensible Tobari V1

This file records verified facts and unresolved questions. Desired behavior is
kept in `goal.md` and `plan.md`.

## Current behavior

- The canonical component selection is already a paired exact V1 contract, but
  `internal/infra/runtimeassets/assets/versions.env` selects
  `GATEWAY_IMAGE=unpublished` and `AUTH_BROKER_IMAGE=unpublished`. Public and
  release gates intentionally reject that selection.
- `gh release list --limit 5` returned no release on 2026-08-12. There is no
  published source-to-artifact path for the current contract.
- `internal/infra/dockerruntime/project_runtime.go` fixes CPU to `2.0`, memory
  to `4g`, and PIDs to `512`, and directly bind-mounts the selected canonical
  root read-write into the Workspace.
- The long-lived Workspace runs as the invoking host UID/GID (normally
  non-root), is read-only outside approved mounts, is capability-dropped, and
  has no Docker socket or direct external network. A root invocation is not
  currently rejected and therefore results in container UID/GID 0.
  The shared trusted Gateway terminates transparent HTTP/HTTPS attempts and
  obtains the ordinary policy decision before DNS, upstream connection, or
  broker secret resolution.
- The Gateway policy input carries scheme, host, port, method, path, query, and
  redacted headers. Current guided denial, candidate, and learned-rule identity
  binds Context, project, host, port, method, and path but omits scheme; its
  exact learning also omits body, query values, and headers. Advanced owner
  Rego can inspect the generic query/header input. The V1 envelope decision
  must reconcile this implementation with the intended scheme-aware permission
  identity before policy preset work begins. Declared GraphQL endpoints add
  operation type and root field but not arguments or variables.
- The public policy workflow includes exact candidate discovery, interactive
  batch review, exact allow/deny/reset, and learned prefix compaction through
  `policy compactions` plus `policy compact`.
- Context already composes stable identity, runtime image/recipe, guided or
  advanced policy mode, credential eligibility, shell/Git projections, and a
  permanently bound Workspace. It does not currently persist source access or
  a policy-preset origin/revision.
- Context creation currently copies the same embedded `api.github.com`,
  `example.com`, and `mock-upstream` domain policy into every Context. This is
  implicit initial authority rather than an explicitly named user selection.
- The Docker project runtime already emits one direct source bind. Adding the
  Docker `readonly` mount option can constrain that selected bind without a
  clone, overlay, Git-only assumption, or apply-back workflow; the project home
  and tmpfs mounts remain separately writable.
- Authentication currently includes Workspace-owned tool-native passthrough,
  a retained static managed Gateway adapter, a shared locked Auth Broker,
  owner-imported static providers, built-in Chatwork and GitHub plans, dynamic
  AWS, Datadog, and OpenAI plans, a helper-acquired static Anthropic plan, and
  a resident trusted-host credential companion.
- GitHub acquisition already uses fixed `gh` argv, a private temporary home,
  an absolute canonical executable, and a digest check before and after the
  interactive command sequence. Its safety property does not require an exact
  product version string when the observed fixed-command contract holds.
- Exact Codex 0.146.0 and Claude Code 2.1.220 behavior, AWS CLI driver variants,
  Datadog OAuth refresh, AWS SigV4 signing, refresh barriers, and companion
  transport expand the release matrix beyond the static post-policy broker
  property.
- `.harness/capabilities.json` has public `policy.learning` and
  `authentication.broker` capability IDs; provider plans and compaction are
  surfaces inside those capabilities rather than independently governed
  capability IDs.
- `.harness/project.json` still describes Tobari as named Docker isolation
  spaces behind one policy-enforced Gateway, which under-emphasizes credential
  mediation and over-emphasizes the runtime substrate.
- `docs/THREAT_MODEL.md` already accepts direct source-root mutation, allowed-
  destination exfiltration, same-Workspace handle reuse, shared trusted-
  component compromise, and missing per-project fairness or quotas.

## Relevant structure

- Entry point: `cmd/tobari` composed by `internal/cli`
- Domain rule: `internal/domain/operation`, `internal/domain/tobari`,
  `internal/domain/authbroker`, and policy-domain validation
- Application use case: `internal/app/tobaricmd`, `internal/app/authcmd`, and
  policy command use cases
- Infrastructure boundary: `internal/infra/dockerruntime`,
  `internal/infra/credentialhost`, `internal/infra/authproviders`, canonical
  Gateway/Auth Broker sources, OPA assets, root key, and encrypted vault
- CLI catalog or presentation: `internal/cli/runtime_catalog.go`,
  `internal/cli/auth_catalog.go`, and catalog-derived generated architecture
  data
- Existing tests and harness checks: domain/application/CLI unit tests,
  Gateway/Auth Broker Python suites, Docker integration, architecture lint,
  public guard, schema/capability ledgers, and release artifact checks

## Constraints

- The public CLI must close user tasks through `cli.Catalog`; removed command
  and reference edges cannot survive through aliases or hidden dispatch.
- Discovery and action stay separate. Retaining exact policy mutation means
  candidate IDs remain opaque and unchanged from discovery to action.
- OPA remains the only authority for the ordinary HTTP effect. Broker provider
  recognition cannot become provider-business-operation authorization.
- A Workspace may receive only a project-bound handle for brokered mode. The
  primary secret, vault, root key, and credential-bearing types remain in
  infrastructure, and resolution remains strictly after allow.
- Removing dynamic plans must not accidentally weaken malformed-handle,
  rotation, revocation, binding, or no-fallback behavior in the retained static
  plan.
- ADR 0027 allows a pre-public exact-V1 reset: no migration or compatibility
  reader is required, and prior development state must be explicitly removed
  and recreated.
- Repository documentation is English, generated surfaces derive from their
  executable source of truth, and no real credential or provider response may
  become a fixture.
- Publication work requires `task public:check` and release work requires
  `task release:check`; no local build may be described as a public release.

## External facts

- Docker, “Security model,” <https://docs.docker.com/ai/sandboxes/security/>,
  checked 2026-08-12: Docker Sandboxes uses a microVM boundary, but its default
  Workspace mode is also a direct read-write mount; `--clone` is opt-in and
  uses a read-only source plus an in-VM clone.
- Docker, “Policy concepts,”
  <https://docs.docker.com/ai/sandboxes/governance/concepts/>, checked
  2026-08-12: Docker network rules are connection resources such as hostname,
  CIDR, and port rather than Tobari's HTTP method/path identity.
- Docker, “Isolation layers,”
  <https://docs.docker.com/ai/sandboxes/security/isolation/>, checked
  2026-08-12: the agent can use sudo inside its microVM, while network and
  credential enforcement remain host-side. This supports keeping Tobari's
  enforcement outside any future agent-controlled VM.
- Docker, “Docker Sandboxes release notes,”
  <https://docs.docker.com/ai/sandboxes/release-notes/>, checked 2026-08-12:
  the latest stable release documented there is 0.37.0, not the 0.38 version
  cited in the feedback. The comparison is useful evidence, not a versioned
  Tobari dependency.

## Unknowns

- [ ] Record the exact first-release tag, supported OS/architecture matrix, and
      immutable Gateway/Auth Broker index digests after the release candidate
      is built and independently inspected.
- [ ] Confirm the CI identity and artifact-attestation mechanism that can
      generate SBOM/provenance without introducing a long-lived signing key.
- [ ] Confirm the Homebrew tap publication authority and perform a clean-host
      install/uninstall observation before publication.
- [ ] Re-run the retained GitHub trusted-host login manually with a disposable
      account and a currently supported `gh` build; record argv-contract and
      cleanup evidence without recording provider output or secrets.
- [ ] Record representative cloud-agent startup failures under
      `builtin/offline` and `builtin/get-only-reviewed`; no implicit model-
      provider bypass may be added to make those presets appear compatible.

## Thesis evidence

- Repeated design decision or point of agent confusion: product discussion
  repeatedly compares Tobari's container to a microVM sandbox even though the
  distinctive enforcement and credential boundaries live outside the
  Workspace.
- User outcome or friction observed in the minimal slice: source is extensive
  but cannot be installed as a matching release; auth maintenance grows with
  every provider CLI release before there is public usage evidence.
- Code workaround or exception being considered: maintaining companion,
  refresh, signing, and exact-version client contracts to preserve broad
  provider coverage would route around the initial generic-effect thesis.
- Current thesis that resolves it, or proposed thesis revision: make bounded
  Docker execution the substrate, route-level HTTP authorization the core
  effect boundary, and the static project-bound broker the credential proof.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: revise theses and affected ADRs; narrow auth and policy catalogs;
  remove dynamic infrastructure and state; update capability/schema ledgers,
  security claims, generated architecture data, release gates, and the
  repository-local `add-capability` guidance if its examples become stale.

## Reproduction or observation

```sh
nl -ba internal/infra/runtimeassets/assets/versions.env | sed -n '1,12p'
nl -ba internal/infra/dockerruntime/project_runtime.go | sed -n '24,38p;642,662p'
gh release list --limit 5
rg -n 'policy compactions|policy compact|credential_companion_state' internal/cli
rg -n 'Codex 0.146.0|Claude Code 2.1.220|managed adapter' docs internal
```

Expected and observed on 2026-08-12: exact V1 component APIs with unpublished
image authorities; fixed resource limits; one direct root bind; no GitHub
release rows; public compaction, companion, managed-adapter, and pinned-client
contracts present. No secret-bearing command or output was used.

## Security and public-boundary notes

- Assets and side effects involved: host source tree, Workspace home, Context
  policy, Docker network namespaces, Gateway CA, OPA decision set, root key,
  encrypted credential vault, public binaries, and OCI images.
- Credentials or confidential data involved: Workspace-owned tool state,
  broker primary secrets, project handles, root key, and provider login state.
  All tests use synthetic tokens and no live provider artifact is committed.
- New dependencies, destinations, files, processes, or generated content: the
  target design removes processes and provider dependencies; release tooling
  may add only reviewed SBOM/provenance generation owned by the release gate.
- External schema provenance, publication rights, and drift evidence: no
  provider response schema is retained except the bounded GitHub CLI command
  output consumed by the fixed acquisition flow; live behavior remains a
  manual release observation.
- Output delivery, collection coverage, pagination, timeout, retry,
  idempotency, and cancellation facts: retained reads preserve their catalog
  contracts; auth and policy mutations remain non-retryable unless their
  structured outcome proves no mutation; no automatic denied-request retry is
  introduced.
- Publication and licensing concerns: every binary, OCI layer, third-party
  module, SBOM, and provenance statement must pass public and release checks.

## Glossary

- **Bounded execution:** a Docker-backed Workspace with explicit mounts,
  identity, resources, and network attachment; not a claim of a separate
  kernel or recoverable source tree.
- **HTTP effect:** the normalized HTTP authorization tuple plus the trusted
  Context/project principal; body semantics are outside the ordinary tuple.
- **Workspace-owned credential:** a real credential stored or obtained inside
  one Workspace home and readable by processes in that Workspace.
- **Brokered credential:** a host-owned primary secret represented in a
  Workspace by a project-bound opaque handle and resolved only after policy
  allow.
- **Static plan:** an exact HTTPS/header replacement with no refresh, signing,
  provider-native post-policy execution, or provider-business semantics.
- **Dynamic plan:** any credential plan that refreshes, signs, or invokes a
  provider-native driver after the initial acquisition.
