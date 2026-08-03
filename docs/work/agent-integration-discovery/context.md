# Work Context: Agent integration discovery

This packet records verified facts and the evidence-backed surface decision. It
does not claim that a Codex or Claude Code plugin has been implemented.

## Current behavior

- Tobari's public entry point is a catalog-owned CLI. `policy candidates` and
  `policy review` are discover/read surfaces; `policy allow --id` and
  `policy deny --id` are explicit reference-bound host actions. The candidate
  ID is opaque and is passed unchanged to the action.
- A learnable Gateway denial returns a fixed, secret-free host-side
  `tobari policy review` navigation object. It cannot approve a request or
  trigger an automatic retry.
- `runtime init` and `runtime build` are fixed-target host actions on the
  active Context. The build context is the owner-only Context runtime
  directory; a successful compatible image is promoted into the Context, and a
  failed build leaves the previous image selected.
- Root agent help is an outcome/capability index, while scoped help supplies
  the command workflow and recovery metadata. Read-only probes on 2026-08-03
  returned:

  ```text
  help --format agent       -> schema_version=8, view=index, 25 command entries
  help policy --format agent -> view=scope, policy candidates/review/tail/allow/deny/compactions/compact
  help runtime --format agent -> view=scope, runtime init/build
  ```

- The real integration harness already exercises the equivalent agent loop in
  `scripts/test-integration.sh`: it runs `curl` inside a Tobari, checks
  secret-free denial navigation, reads host-side denial/candidate projections,
  passes opaque IDs to exact allow/deny actions, retries requests, checks
  project/path boundaries, builds a Context runtime, and cleans up.

## Relevant structure

- Entry point: `cmd/tobari` composes the CLI and application services.
- Domain rule: `internal/domain/tobari` owns policy, Context, runtime, and
  opaque-reference validation; `internal/domain/operation` owns effects and
  mutation declarations.
- Application use case: `internal/app/tobaricmd` owns the user-task
  interpretation and narrow ports for lifecycle, policy, Context, and runtime
  actions.
- Infrastructure boundary: `internal/infra/dockerruntime` owns Docker, OPA,
  Gateway, policy activation, runtime build, and host-state I/O.
- CLI catalog or presentation: `internal/cli` owns `cli.Catalog`, typed argv,
  human/agent help, and the terminal policy review presentation.
- Existing tests and harness checks: `scripts/test-integration.sh`,
  `task integration:test`, policy/Gateway tests, catalog contracts, and the
  agent-readiness scenario in `docs/09_agent_readiness_validation.md`.

## Constraints

- A skill may teach a sequence, decision point, output interpretation, and
  recovery command; it must not become a second command catalog or policy
  authority.
- Policy mutations remain host-only, explicit, exact-reference-bound, and
  one-candidate-at-a-time. A skill must ask for or surface human confirmation
  before delegating `policy allow` or `policy deny`.
- A skill must not accept or expose credentials, policy paths, Docker socket
  access, arbitrary image names, arbitrary host roots, raw OPA/Rego mutation,
  or arbitrary executor arguments.
- `runtime build` is an explicit host-side Docker build and may refresh the
  official moving base only because the user invoked that command. It is not an
  implicit agent-side network or image-selection authority.
- The repository's trusted documentation locale is English. External agent
  output remains untrusted data and must keep the existing safe projection and
  opaque-ID rules.
- The current worktree contains unrelated user/agent changes. This packet did
  not edit them and must not be used to infer their status.

## External facts

- OpenAI Codex Manual, [Build skills](https://learn.chatgpt.com/docs/build-skills.md), checked 2026-08-03: standalone skills package repeatable instructions and resources across Codex CLI, IDE extension, and desktop app; plugins are the distribution layer for skills and optional connectors/MCP.
- OpenAI Codex Manual, [Build an MCP server](https://developers.openai.com/plugins/build/mcp-server.md), checked 2026-08-03: MCP is appropriate for live external data, authentication, and controlled actions; tools need explicit schemas, authorization, and safety annotations.
- OpenAI Codex Manual, [Build plugins](https://developers.openai.com/plugins/build/plugins), checked 2026-08-03: plugin shape should follow a recognizable use case; skills-only is valid when existing tools are sufficient, while MCP is for a real tool/integration requirement.
- Anthropic, [Extend Claude Code](https://code.claude.com/docs/en/features-overview), checked 2026-08-03: skills are reusable/invocable workflows, MCP connects external services, and plugins package skills, subagents, hooks, and MCP servers.
- Anthropic, [Extend Claude with skills](https://code.claude.com/docs/en/slash-commands), checked 2026-08-03: standalone project skills are invoked with slash commands and use the Agent Skills format; plugins are the sharing/distribution layer.
- Anthropic, [Connect Claude Code to tools via MCP](https://code.claude.com/docs/en/mcp), checked 2026-08-03: MCP servers expose tools and may bundle with plugins, but their tools still require explicit server-side authorization and lifecycle configuration.

These facts establish the surface mapping only. They do not authorize Tobari
to call vendor APIs, claim an official vendor plugin, or move authentication
into Tobari.

## Unknowns

- [ ] Whether a future standalone skill should be one shared Agent Skills
  package or two thin vendor-specific wrappers; answer with activation and
  output tests after the first skill exists.
- [ ] Whether users need agent-readable policy status beyond the existing
  catalog JSON; answer from the first skill's E2E transcript before adding any
  new CLI or MCP output.
- [ ] Whether a future plugin distribution channel is worth its packaging and
  review cost; answer after repeated cross-repository skill use or an explicit
  user distribution requirement.

## Thesis evidence

- Repeated design decision or point of agent confusion: the existing catalog,
  fixed host-side next command, and opaque candidate reference already define
  the safe agent handoff. Recreating those facts in an MCP schema would create
  a competing registry.
- User outcome or friction observed in the minimal slice: the policy and
  runtime journeys close successfully through the existing CLI. The observed
  friction was environmental (Go cache permissions, BuildKit activity path,
  Docker VM bind visibility), not missing agent authority.
- Code workaround or exception being considered: a generic agent executor or
  auth broker was considered and rejected. The skill must remain a thin
  workflow guide over exact CLI commands.
- Current thesis that resolves it, or proposed thesis revision: Thesis 8 and
  Thesis 9 already resolve the authority and Context boundaries. No thesis
  revision is justified by this evidence.
- Downstream product, architecture, security, Skill, catalog, and harness
  impact: create a future skill packet with the existing command paths and
  recovery rules; do not add a public command, ledger entry, MCP tool, or auth
  layer. If a future surface changes a public outcome, propagate that change
  through `docs/00` through `docs/04`, the catalog, and the relevant tests.

## Reproduction or observation

### Agent discovery probes

```sh
GOCACHE=/private/tmp/tobari-agent-help-cache go run ./cmd/tobari help --format agent
GOCACHE=/private/tmp/tobari-agent-help-cache go run ./cmd/tobari help policy --format agent
GOCACHE=/private/tmp/tobari-agent-help-cache go run ./cmd/tobari help runtime --format agent
```

Observed shape: root `view=index` with 25 command entries; `policy` scoped
help contains the seven policy discovery/action paths; `runtime` scoped help
contains `runtime init` and `runtime build`.

### End-to-end workflow proof

The first attempts exposed environment-only blockers and were not counted as
Tobari product failures:

1. `task integration:test` could not write the default Go build cache.
2. A writable `GOCACHE` reached `cluster_start_failed` because Docker BuildKit
   tried to write its default activity directory.
3. The explicit Gateway source-build recovery path surfaced the same Docker
   client cache boundary as `gateway_source_build_failed`.
4. A direct probe with XDG state under an external temporary directory reached
   `policy_test_failed` because the Colima VM could not see that host bind path.
5. A temporary `DOCKER_CONFIG` fixed BuildKit but hid the installed Compose
   plugin, so it was not used as the final setup.

The final run preserved the normal Docker context and Compose plugin, redirecting
only BuildKit's configuration:

```text
$ buildx_config_dir=$(mktemp -d /private/tmp/tobari-agent-buildx-config.XXXXXX)
$ BUILDX_CONFIG="$buildx_config_dir" \
    TOBARI_INTEGRATION_GATEWAY_SOURCE=1 \
    GOCACHE=/private/tmp/tobari-agent-integration-buildx-cache \
    task integration:test
task: [integration:test] ./scripts/check.sh integration
...
integration: OK
```

Environment observed: macOS host, Colima Docker context, Docker Engine
27.4.0, Compose v2.24.6, arm64. The harness removed its synthetic roots and
Tobari-owned containers, networks, and volumes after completion.

The transcript's semantic stages were:

| Stage | Input and operation | Expected and observed result | Recovery/interpretation |
|---|---|---|---|
| 1 | In-Tobari `curl` to an allowed mock endpoint | 200 and upstream evidence | Establish the normal bounded path |
| 2 | In-Tobari denied baseline and learnable requests | 403; learnable response contains `policy_denied`, fixed `tobari policy review`, `automatic_retry:false`, and no secret | Agent reports the host handoff; it does not retry automatically |
| 3 | Host `cluster denials`, `policy candidates`, and `policy review --format json` | Bounded typed evidence and opaque `pcy_*` candidates | Read-only discovery preserves exact request dimensions |
| 4 | Explicit host `policy allow --id ID` and `policy deny --id ID` | One exact rule activates; denied effect remains terminal | The selected opaque ID is passed byte-for-byte; no display-position action |
| 5 | Retry the same learnable request and adjacent canaries | Approved exact path returns 200; denied path, child path, and other project remain 403 | Confirms no authority broadening and closes the loop |
| 6 | `runtime init`, official-base `runtime build`, local-base edit, second `runtime build`, `context show` | Context runtime becomes ready and selected image is promoted in both builds | Runtime customization stays explicit host-side; failed builds retain the prior image |
| 7 | Detach/delete/cluster cleanup | `integration: OK`, no owned resources remain | Recovery and cleanup are part of the proof |

Policy recovery therefore has six decision stages before cleanup, and runtime
customization has four build/selection stages before cleanup. The reviewed
routine-success external-processing count is zero: no provider parser, source
inspection, exploratory provider call, automatic retry, credential lookup, or
undeclared authority step was introduced. The Docker build is a declared
host-side effect of `runtime build`, not a hidden agent action.

### Commit recheck

The required workflow was replayed again before committing this packet. The
literal current-worktree `task integration:test` reached the policy TTY check
and exited 126 from the unrelated, uncommitted PTY helper change in
`scripts/test-integration.sh`; Gateway, OPA, and both Tobari runtimes were
healthy and the failure occurred after the non-TTY denial evidence. Replaying
the committed helper then stalled in the same TTY-only step and was stopped
without changing repository files. Neither helper is part of this packet and
neither was staged.

To verify the packet's required workflow without changing that helper, the same
current harness was executed in memory with only the TTY-only block replaced by
its existing JSON `policy review` plus exact `policy deny --id` and retry. The
repository file was not written. That run returned:

```text
integration: OK
```

It covered deny/review/opaque deny/retry, runtime image initialization and
promotion for official and local bases, cross-project and child-path boundary
canaries, and cleanup with no Tobari-owned resources remaining. The TTY helper
failure remains a separate integration issue; the selected agent-equivalent
workflow proof is green.

## Security and public-boundary notes

- Assets and side effects involved: synthetic project roots, per-Tobari homes,
  Gateway/OPA policy state, local runtime image tags, and test Docker resources.
  All were created and removed by the existing harness or exact probe cleanup.
- Credentials or confidential data involved: none. The integration harness uses
  synthetic authentication canaries; this packet records no values, tokens,
  policy contents, personal identifiers, or private URLs.
- New dependencies, destinations, files, processes, or generated content: no
  repository production dependency, CLI command, process authority, or MCP
  endpoint. Temporary caches and a disposable Docker build configuration were
  used only for the local E2E run and removed or moved out of the repository.
- External schema provenance, publication rights, and drift evidence: no
  external schema or fixture was added. Official Codex and Claude documentation
  was consulted as linked current reference material only.
- Output delivery, collection coverage, pagination, timeout, retry, idempotency,
  and cancellation facts: existing CLI contracts remain unchanged; policy
  discovery is bounded/read-only, actions are exact-reference-bound, the policy
  loop does not auto-retry, and runtime build failure remains a safe retry point.
- Publication and licensing concerns: do not call a future surface an
  "official OpenAI" or "official Anthropic" plugin. A later plugin must undergo
  the relevant distribution, license, privacy, and public-boundary review.

## Glossary

- **Agent surface:** the smallest supported way for Codex or Claude Code to
  discover and invoke an existing Tobari workflow.
- **Skill:** reusable instructions and decision flow; it does not own a new
  side-effect boundary.
- **Plugin:** a distribution bundle for skills and optional tools/connectors;
  it is not needed merely to wrap a local CLI.
- **MCP:** a tool/data integration protocol; it is deferred because this slice
  needs no live external service, user authentication, or remote action.
- **Opaque reference:** a candidate value produced by discovery and passed to a
  reference-bound action unchanged.
- **Authority boundary:** the host-owned catalog, policy store, Context, and
  Docker/Gateway/OPA control path that an untrusted agent cannot select or
  mutate directly.
