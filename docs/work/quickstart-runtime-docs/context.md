# Work Context: Quick Start runtime documentation

This packet records verified facts and bounded observations for the second-wave
README journey. It does not introduce a command or change the runtime.

## Current behavior

- `cluster up` is the explicit shared Gateway/OPA startup command. The root
  `tobari` command requires that cluster to be configured and ready, resolves
  the canonical current directory, and enters a reusable Workspace.
- A learnable denied HTTP/HTTPS effect returns a secret-free `policy_denied`
  response with fixed `tobari policy review` navigation and no automatic retry.
  The host-owned review/candidate projection produces an opaque `pcy_...`
  reference; `policy allow --id ID` consumes that exact value unchanged.
- `runtime init` creates the active Context's owner-only `runtime/Dockerfile`
  and never overwrites it. `runtime build` builds only that directory, validates
  the Tobari runtime contract, and promotes the generated local image only after
  the build and digest checks succeed.
- The official runtime base is
  `ghcr.io/tasuku43/tobari/runtime:latest`. It may be refreshed only by the
  explicit `runtime build`; ordinary Workspace entry does not pull a configured
  image. A failed build leaves the previously selected Context image active.
- The current README already explains the topology and policy-learning loop,
  but the Quick Start is split across lifecycle, policy, and runtime sections
  and contains a stale retired `tobari exec` example. The second-wave edit
  makes one runnable journey and removes that stale path.

## Relevant structure

- Public documentation: `README.md`, with the Quick Start and runtime
  customization sections as the only product-facing files changed here.
- Command source of truth: `internal/cli.Catalog`, with existing paths
  `cluster up`, `tobari`, `policy review`, `policy candidates`, `policy allow`,
  `policy deny`, `runtime init`, `runtime build`, `context show`, `delete`, and
  `cluster down --purge`.
- Application/infrastructure boundary: existing lifecycle, policy, Context, and
  runtime services; no source changes are in scope.
- Existing executable E2E: `scripts/test-integration.sh`, exposed through
  `task integration:test` and included by `task runtime:test`.
- Existing contract gates: `task check`, `task public:check`, and the focused
  policy/Gateway/runtime profiles in `Taskfile.yml`.

## Constraints

- Use only existing public command paths and catalog terminology. Do not add a
  flag, image authority, arbitrary executor, or hidden pull step.
- The host owns cluster startup, policy review/allow, Context recipe editing,
  and `runtime build`. The agent owns work inside the mounted root and
  proxy-aware requests inside Tobari; it cannot mutate host policy or Context
  state.
- Policy discovery is read-only. A reference-bound action consumes one exact
  opaque candidate unchanged; display order and prose cannot identify a
  candidate.
- Runtime customization is Context-owned. `.devcontainer` metadata and project
  files do not select the runtime. `runtime build` is the only supported build
  path in this journey and is explicitly host-side.
- Public docs use `example.com`-style synthetic values and no credentials,
  private URLs, machine paths, usernames, or shell-history captures.
- No existing first-wave packet, production code, catalog, numbered contract,
  workflow, architecture HTML, GitHub Pages, or auth-broker file may change.

## External facts

None. This packet relies on repository contracts, the existing catalog, and the
repository integration/runtime harness; no external source is needed to define
the public journey.

## Unknowns

- [x] Whether the current Docker/Colima environment can complete the positive
      integration profile after the documentation edit. The cluster and
      policy/runtime setup became healthy, but the existing interactive PTY
      `policy review --tail 1000` helper blocked both aggregate profiles before
      completion; see `e2e-transcript.md`.
- [x] Whether the environment permits the explicit official-base runtime build
      without a BuildKit or VM bind-path blocker. The real build succeeded; an
      external temporary XDG path caused `policy_test_failed` at `cluster up`,
      and a repository-local synthetic XDG path completed the entry replay.
- [ ] Whether the public example endpoint returns a stable upstream status after
      an exact learned `PUT`; the README therefore treats the post-allow status
      as upstream-owned and identifies success by the absence of Tobari's
      `policy_denied` handoff.

## Thesis evidence

- Repeated design decision or point of agent confusion: the safe workflow has
  two authorities—a trusted host and an untrusted agent—and runtime image
  customization must remain attached to the active Context.
- User outcome or friction observed in the minimal slice: the existing
  commands and harness are complete, but a new user has to join separate README
  sections to find denial recovery and runtime customization.
- Code workaround or exception being considered: none. A second command
  registry, implicit retry, or direct Docker build would violate the governing
  contracts.
- Current thesis that resolves it: Theses 0, 8, and 9 already require a
  CWD-first bounded-autonomy journey, explicit policy learning, and one Context
  runtime authority.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: documentation only; no durable contract or executable enforcement
  change is justified.

## Reproduction or observation

```sh
go run ./cmd/tobari help --format agent
go run ./cmd/tobari help policy --format agent
go run ./cmd/tobari help runtime --format agent
task integration:test
task runtime:test
```

The exact outputs, environment, cleanup, and any stop point are recorded in
`e2e-transcript.md` after the replay. No output containing local paths,
credentials, or private identifiers is copied here.

Observed evidence: root and scoped agent help exited 0; policy and Gateway
focused checks exited 0; a real Context recipe edit/build/entry printed
`tree v2.1.0`; `task check` and `task public:check` exited 0 in a clean
detached verification checkout. The full integration and aggregate runtime
profiles were stopped at the existing interactive PTY review helper with task
exit 130, and that blocker is not presented as a product success.

## Security and public-boundary notes

- Assets/side effects: only public README/work-packet files are changed;
  E2E uses the existing synthetic project roots, Docker resources, Gateway/OPA,
  and Context image build boundary.
- Credentials/confidential data: none. The documented curl carries no auth
  header or body, and the transcript records no secret canary values.
- New dependencies/destinations: none. `runtime build` remains an existing
  explicit Docker side effect and is not reimplemented in docs.
- Output/recovery: denial evidence is bounded and read-only until an exact
  host action; runtime build failure is a safe retry point; cleanup is
  `tobari delete` followed by `tobari cluster down --purge`.
- Publication: all examples are synthetic and English-language, with no
  machine-specific paths or private history.

## Glossary

- **Host:** the trusted user-side CLI/Docker/OPA policy authority.
- **Agent:** an untrusted process running inside the selected Tobari.
- **Workspace:** the reusable CWD-owned Tobari session target.
- **Context:** the host-selected execution setup that owns the runtime recipe,
  policy, and related stores.
- **Opaque candidate:** one exact `pcy_...` value emitted by read-only policy
  discovery and passed unchanged to `policy allow --id` or `policy deny --id`.
