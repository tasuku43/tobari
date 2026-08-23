# Work Context: Release and research build boundary

This context separates verified current facts, evaluation, unknowns, and
inference. Desired future vocabulary and behavior are defined in `plan.md`, not
presented here as current reality.

## Baseline and repository state

- **Verified historical baseline:** `git fetch origin main` completed on
  2026-08-23.
  `origin/main`, local `HEAD`, and remote `refs/heads/main` all resolve to
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`.
- **Verified historical baseline:** the initial and post-observation worktree
  was clean:
  `## main...origin/main` with no short-status entries.
- **Verified:** remote annotated prerelease tags `v0.1.0-dev.1` through
  `v0.1.0-dev.11` exist. This is pre-stable software, but build-identity JSON
  has already shipped in public prerelease artifacts and its rename must be
  called out rather than treated as never published.
- **Verified:** no Docker build, cluster mutation, credential operation,
  publication, tag, or release operation was run while preparing this packet.
  Temporary Go binaries were built outside the repository. Observations were
  limited to `version`, help, negative release command dispatch, and one
  research `serve` preflight that returned unavailable before creating a
  listener; it performed no mutation.
- **Verified integrated shared-checkout state on the upstream-sync update:**
  local `HEAD` is `52a53bcc69a0f2bdf9bf2a6782ecd98bacd8b0e1` on
  `codex/workspace-manifest-v1`. The Workspace Manifest/model implementation is
  evidenced by `07535a9`; accepted ADR 0079, research-auth quarantine, and its
  security/authentication consequences are evidenced by `428812f`. The
  one-time Manifest/Runtime copy contracts are now durable in ADR 0079 and
  `docs/00_theses.md` through `docs/04_harness.md`. Both predecessor temporary
  packet directories have been removed.
- **Verified owner status:** the WP01 and WP02 contract work has been promoted
  and integrated. WP08 and WP03 remain later prerequisites, so neither the old
  clean baseline nor this intermediate integrated HEAD is the WP04
  implementation baseline.
- **Required interpretation:** all command counts, schemas, file paths, and
  current-behavior statements below are observations of commit
  `6a26a3c274d2c2ce8dc8c59321ffb7ba67594b42`, not claims about the result of
  the integrated contracts. Implementation must still follow the fixed
  dependency order and repeat bounded discovery against the actual post-WP03
  HEAD and worktree.

## Durable upstream Workspace Manifest and one-time-copy contracts

- **Verified durable authority:** [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md),
  `docs/00_theses.md` through `docs/04_harness.md`, and
  `docs/07_authentication.md` are higher-precedence context for this packet.
  Final public product language is
  **Workspace Manifest**, routine presentation is **Manifest**, CLI spelling is
  `manifest`/`--manifest`, and schema authority is
  `workspace_manifest_id`. Final V1 retains no public Context alias.
- **Verified concept budget:** the durable public resources are Workspace
  Manifest, Runtime, and Workspace. Runtime revision and Project root are
  subordinate concepts. Capability surface is executable identity and cannot
  become a fourth durable resource or a Workspace Manifest field.
- **Verified authority model:** Workspace Manifest is host-owned, CLI-managed,
  stable-ID authority with complete immutable desired revisions and an
  invariant Boundary. Runtime is independent and is bound by exact RuntimeID
  plus semantic revision digest. Workspace is the durable
  `(ProjectRoot, WorkspaceManifestID)` instance with `workspace_id` authority.
- **Verified activation model:** `cluster up` alone reconciles cluster
  projection; explicit Workspace entry alone reconciles Workspace runtime;
  session defaults activate for new child sessions; creation defaults apply to
  new Workspaces only. A desired change does not mutate a running Workspace,
  and an attached Workspace that would require recreation remains blocked
  without mutation. Reads observe desired, last successfully applied, observed,
  and last bounded failure without converging them; no resident controller
  performs implicit reconciliation.
- **Verified state separation:** standard native authentication remains in the
  Workspace home. Learned permission and attachment authority also stay
  outside Workspace Manifest desired/applied state. A fresh recommendation is
  draft presentation, not persisted authority.
- **Verified migration direction:** retain predecessor Context UUID bytes as
  WorkspaceManifestID and ProjectInstance UUID bytes as WorkspaceID where the
  exact migration is accepted; emit only final Manifest/Workspace vocabulary.
  Final public contracts remain schema V1 after the pre-public reset, with no
  dual vocabulary, ordinary fallback reader, or schema-2 compatibility layer.
- **Integration constraint:** WP04 consumes the integrated model, copy, state,
  migration, CLI/Catalog, schema, generated-site, and helper contracts as
  upstream authority. This packet sync edits only its own four files and does
  not revise or replay that production work.

## Product Owner-fixed WP04 decision

- **Fixed vocabulary and identity:** capability authority is exactly
  `release surface|research surface`; resolver authority is the independent
  `embedded|development` axis. Final version JSON remains schema V1 with
  `capability_surface` and `resolver_channel`. Predecessor profile fields,
  values, schema 2, aliases, dual fields, and fallback readers are excluded.
- **Fixed build tuples:** protected artifacts are
  `(release, embedded, tobari)`; `task build` is
  `(release, development, bin/tobari)`; retained `task build:dev` is
  `(research, development, bin/tobari-research)`. There is no
  `build:research` alias. `tobari_research` replaces
  `tobari_experimental` without alias, requires `tobari_dev`, and
  research+embedded is invalid.
- **Fixed Catalog relation:** WP04 consumes the completed WP01/WP02/WP08/WP03
  Catalog as the common base. Research is its complete superset by exactly
  `auth login`, `auth import`, `auth status`, `auth logout`, and `serve`.
  Absolute command totals are diagnostic only.
- **Fixed implementation order:** WP01+WP02 completion audit -> WP08
  Catalog/domain output conformance -> WP03 Runtime retirement -> WP04. WP04
  starts only after a post-WP03 actual-HEAD/worktree observation gate. The first
  prerequisite's durable authorities and integration commits are now present;
  that does not authorize WP04 to begin before WP08 and WP03.
- **Fixed authentication and IA:** standard remains native, Workspace-owned,
  and host-credential-free. Research Broker remains repository-only,
  unsupported, unpublished, and absent from release Catalog/help/topology and
  standard architecture-site IA. Maintainer examples use the exact research
  binary and `--manifest`.
- **Fixed research-state disposition:** do not migrate predecessor experimental
  authentication authority in place. Reject legacy handles; require explicit
  new-surface login/import under the new Manifest identity. Leave owner-only
  predecessor material in WP01's private backup/quarantine without automatic
  deletion, decryption, or rebinding. Standard Workspace homes and native
  credentials remain untouched. Cleanup is a separate reviewed outcome.
- **Observed WP01 migration conflict:** predecessor Context UUID bytes are
  intentionally retained as WorkspaceManifestID, while the predecessor
  `config/auth/contexts/<UUID>` authority path, encrypted vault/root-key
  material, and Workspace provider-binding records would otherwise remain on
  the same reader path. The current research Broker reader also keys lookup by
  that Manifest ID. Without an explicit reachability cut, old credentials and
  handles could therefore become authority for the new Manifest through an
  implicit rebind even though no decrypt/import action ran.
- **Accepted constrained option (b):** research authentication authority is the
  replay-capable combination of credential ciphertext and every filesystem
  binding, handle, lookup, projection, and provider record that makes it
  reachable. WP01 preserves standard Workspace homes but atomically moves that
  complete filesystem research set into owner-only private backup/quarantine.
  Linux filesystem root-key material moves with the set. Normal old and new
  readers cannot discover quarantine, so preserved UUID bytes resolve neither
  old handles nor ciphertext.
- **Verified platform consequence:** after the complete filesystem set is
  quarantined, a macOS Keychain root key alone is inert recovery material rather
  than authorizing state. `migrate apply` never reads, copies, rotates, or
  deletes it. Rollback restores exact filesystem bytes and reuses the unchanged
  Keychain item; it fails closed if fresh canonical auth state now exists. No
  automatic Keychain cleanup occurs, and any future cleanup must prove no live
  or quarantined ciphertext depends on the item.
- **Evidence ownership:** WP01 owns synthetic atomic migration, journal,
  rollback, platform-specific root-key, and secret-free evidence. WP04 consumes
  it and re-verifies old/new-reader denial, release Broker absence, research
  quarantine non-discovery, and explicit new-Manifest login/import after the
  build-surface change.
- **Fixed compatibility and evidence ownership:** accept the pre-public V1
  reset because no stable release has shipped; known prerelease consumers get
  a breaking note, never dual compatibility. WP04 owns the surface/absence/
  mistaken-publication matrix and archive inspection; release packets consume
  that evidence.

## Historical behavior at the observed pre-WP01 commit

### Build and executable matrix

| Producer | Go build selection | Resolver | Reported capability profile | Output name | Historical observed Catalog |
|---|---|---|---|---|---|
| release packager | no project build tag | `embedded` | `standard` | archive member `tobari` | 40 commands; no `auth` or `serve` |
| `task build` | `tobari_dev` | `development` | `standard` | `bin/tobari` | same 40 commands; no `auth` or `serve` |
| `task build:dev` | `tobari_dev tobari_experimental` | `development` | `experimental` | `bin/tobari-dev` | 45 commands; release set plus four `auth` commands and `serve` |

- **Verified:** `Taskfile.yml` first builds local images. `task build` invokes
  `scripts/build-dev-images.sh` and compiles `bin/tobari` with `tobari_dev`.
  `task build:dev` invokes the same script with `--experimental`, compiles with
  both tags, and writes `bin/tobari-dev`.
- **Verified:** `scripts/package-release.sh` clears ambient `GOFLAGS` and other
  Go environment authority, builds without project tags, packages one host
  executable, and on a host-runnable target compares exact version JSON against
  `capability_profile:"standard"`. Its caller supplies no tag field.
- **Verified:** `.github/workflows/release.yml` calls the fixed packager for the
  five target tuples. It has no capability-profile input.
- **Verified:** `scripts/lint-release.sh` currently greps for the two Taskfile
  tag expressions and excludes Auth Broker publication inputs from the release
  workflow. It inspects archive module/GOOS/GOARCH metadata, but the reviewed
  sections do not compare the packaged executable's root human help, agent
  help, exact Catalog path set, or negative `auth`/`serve` dispatch.
- **Verified:** the current research binary's version JSON reports
  `development_build_command:"task build"` and
  `development_binary:"bin/tobari"`. `buildidentity.DevelopmentRecovery()`
  keys only on the development resolver, so it does not identify the research
  producer or binary correctly.

### Catalog and runtime observations

- **Verified:** a current no-tag binary and a `tobari_dev` binary each returned
  40 root agent-help command entries. `help auth --format agent`, `auth`, and
  `serve` all failed as unknown commands with exit 2.
- **Verified:** a `tobari_dev tobari_experimental` binary returned 45 root
  agent-help entries. Set comparison showed the exact additional paths were
  `auth import`, `auth login`, `auth logout`, `auth status`, and `serve`, with
  no path missing from the release set.
- **Verified:** research `help auth --format agent` succeeds and bare `auth`
  renders namespace help. Its agent-help document still declares
  `program:"tobari"`, and generated usage and machine invocation strings use
  bare `tobari auth ...`; the current binary basename is therefore not reflected
  in Catalog presentation.
- **Verified:** `internal/cli/runtime_catalog.go` appends
  `experimentalRuntimeCommandSpecs()` and `authCommandSpecs()` to the same
  Catalog used for routing and help. Build-constrained standard files return
  empty slices; research files return `serve` and four auth specs.
- **Verified:** standard CLI tests check four exact auth paths are absent.
  Standard serve tests additionally check lookup and direct invocation. There
  is no one executable matrix test that compares both root path sets, human
  help, agent help, scoped namespace help, direct dispatch, site generation,
  and release archive evidence as one boundary.
- **Verified:** the standard Docker topology has only Gateway and OPA and loads
  only `compose.yaml`. The experimental topology adds Auth Broker, loads
  `compose.experimental.yaml`, and selects the experimental Gateway layer.
  Official resolver `AuthBrokerImage` fails; the development resolver selects
  Broker only when the compiled profile includes experimental capability.
- **Verified:** build-constrained source uses `tobari_experimental` in the
  capability-profile domain, auth Catalog, Operator Console registration, and
  Docker topology. The same standard-only files also exist in the mechanically
  checked runtime helper source snapshot. Harness and integration scripts, the
  Taskfile, authentication/harness docs, ADR 0044, and the prior work packet
  name the tag as well.

### Layer ownership

- **Domain:** `internal/domain/capabilityprofile` owns a closed
  `standard|experimental` enum and `Compiled()`. It rejects an invented enum
  value, but there is no runtime parser because build constraints choose the
  implementation. `internal/domain/buildidentity` combines this profile with
  independent `embedded|development` resolver identity.
- **Application:** `internal/app/authcmd` owns the existing predecessor
  Context-scoped Broker tasks and the narrow runtime port.
  `internal/app/tobaricmd` composes typed read results for the Operator Console.
  ADR 0079 and its durable contracts, not this packet, cut those task
  inputs/results over to Manifest/Workspace identity. Neither application
  package should choose build membership.
- **Infrastructure:** `internal/infra/dockerruntime` owns the two- versus
  three-service topology, local versus embedded image resolution, Broker
  sockets/mounts, and research Compose override. `internal/infra/operatorconsole`
  owns the bounded loopback server. These implementations exist in source even
  when their CLI entry points are absent.
- **CLI:** `internal/cli.Catalog` remains the routing, help, role, effect,
  output, recovery, and reference-flow authority. Build-constrained catalog
  projection decides which complete specs are registered.
- **Generated documentation:** `tools/sitegen` exports a committed tree, builds
  a no-tag private temporary `tobari`, and derives site Catalog JSON from that
  executable's agent help. The generation path therefore currently selects the
  standard Catalog, which is the correct source direction.

## Documentation and vocabulary observations

- **Verified:** current maintained sources use all of `standard profile`,
  `experimental profile`, `development profile`, `research`,
  `capability_profile`, and `tobari-dev`. The word `profile` also independently
  means a harness verification profile, an AWS configuration profile, and an
  agent profile.
- **Evaluation:** `standard` is overloaded with the `standard@1` Runtime and
  agent-ready baseline; `development` already identifies the local image
  resolver and SemVer prereleases; `experimental` can read like a supported
  product tier; and `profile` conventionally implies runtime selection. These
  words obscure the fact that the boundary is one immutable executable surface.
- **Verified:** README's supported path now explains native Workspace-owned
  authentication and says release has no Broker command or service. It still
  describes a standard/experimental capability profile and names the research
  output `bin/tobari-dev`.
- **Verified:** `docs/07_authentication.md` stages native authentication first
  and later labels the Broker section experimental and unsupported. Its examples
  correctly use `bin/tobari-dev`.
- **Verified:** `docs/09_agent_readiness_validation.md` correctly separates
  standard native-login regression from an optional experimental Broker
  journey, but its optional live examples use bare `tobari auth ...` before
  later prose says to use the `task build:dev` binary.
- **Verified:** the architecture site pins product snapshot
  `381835cda72885f78f5a3c9c69868e25df9a15fd`, older than current main. Its
  generated Catalog contains no auth or serve paths, but the authored standard
  site includes Broker concepts, topologies, provider flows, research wording,
  and bare `tobari auth` examples. Of 88 English/Japanese content files, 60
  match Auth Broker, auth-command, or experimental terms. The normal learning
  path includes credential pages that mix supported native and research paths.
- **Evaluation:** Catalog generation and authored information architecture
  currently disagree. A standard user can encounter commands that the generated
  release Catalog proves do not exist.

## Recent history and related packets

- **Verified:** commit `475e201703579d4c639c37f8051cbf6d80b22a52`
  introduced the standard/experimental capability profile for the first
  prerelease.
- **Verified:** commit `a45e3ba1957f9bd3af707a1450f5d2cfdc54f88d`
  made native Workspace authentication standard and moved Broker commands and
  three-service topology behind the experimental tag.
- **Verified:** commit `9ba775f1ecda68649174fc844489be3c35dd2c33`
  moved the Operator Console `serve` command behind the same tag.
- **Verified:** `docs/work/capability-profiles-first-prerelease/` is still an
  Active temporary packet. It established the compile-time boundary but records
  the now-overloaded profile vocabulary and originally focused its negative
  acceptance on AWS rather than the complete current auth/serve delta.
- **Verified:** `first-public-release-core` and `v1-auth-narrowing` remain
  Active and describe a Broker-bearing public V1, which conflicts with the
  newer native-standard thesis and current release Catalog. The release-artifact
  packet is also Active and owns packaging evidence.
- **Evaluation:** this packet should supersede only the vocabulary,
  presentation, and boundary-evidence conclusions of the profile packet. It
  must not silently rewrite other Active packets; their owners must reconcile
  stale Broker publication assumptions before implementation completion.
- **Verified durable model contract:** ADR 0079 and `docs/00_theses.md` through
  `docs/04_harness.md` reserve authentication build surface as a separate
  packet interface. Their public vocabulary, resource count, identity,
  activation, state separation, and pre-public schema/migration decisions
  govern this packet; `07535a9` and `428812f` are integration evidence, not
  substitute authorities.
- **Verified durable copy contract:** Manifest copy is
  `manifest create --copy-from NAME --name NAME`; Runtime source copy is
  `runtime create --copy-source-from standard|NAME --name NAME`; `--base` has
  no alias. Both produce fresh independent identity, persist no lineage, copy
  no Workspace/auth/learned/attachment/applied/failure/observed/selection
  state, and perform no reconciliation. These semantics are durable in ADR
  0079 and `docs/00_theses.md` through `docs/04_harness.md`.
- **Verified Product Owner-fixed WP03 design:** Runtime lifecycle uses
  `runtime delete --id <runtime-ref> --confirm=delete`, read-only
  `runtime prune dry-run`,
  `runtime prune apply --plan <ref> --confirm=prune` with complete locked
  revalidation and zero partial apply on staleness,
  `runtime restore --id <runtime-revision-ref>`, read-only
  `review runtimes`, and `runtime build --id <runtime-ref>`; `--name` or
  omitted-name build selection has no alias.
  Retirement protection distinguishes Manifest revisions, Workspace
  applied/pending state, owned/foreign observations, and unknown evidence;
  prune preserves authority/history, restore requires exact recorded digest,
  `last_used` remains unknown without separately approved exact evidence. The
  Runtime manifest schema is retained while build/retirement journals and
  bounded idempotent receipts remain owner-only sidecars. WP03 has no
  implementation-start notice.
- **Evaluation:** the WP01+WP02 durable-contract leg is integrated, while WP04
  must still rebaseline the result through the fixed sequence. WP04 begins only
  after WP08 and WP03 have completed and the post-WP03 gate has consumed their
  actual commands, references, schemas, and documentation rather than adding
  placeholders.

## Constraints

- The Catalog remains the only public command and invocation authority.
- Capability membership must be compile-time, closed, and fail-safe. Runtime
  input cannot broaden a release executable. Workspace Manifest desired,
  applied, observed, or failure state; Runtime source/revisions; Workspace
  records; and migration evidence are all runtime data for this invariant.
- The three-resource public concept budget remains Workspace Manifest, Runtime,
  and Workspace. Capability surface may be reported by `version`, but is not a
  durable public resource, selector, or Manifest activation slice.
- Resolver channel and capability surface are independent concepts: current
  `task build` is direct evidence for development resolver plus release
  capability. They must not be collapsed into a misleading two-build model.
- The five research commands are one reviewed research set. Per-command build
  flags or runtime feature combinations would add unsupported authority and
  test combinations without an independent user outcome.
- The release and research Catalogs share every non-research capability. The
  completed WP01/WP02/WP08/WP03 Manifest, Runtime, Workspace, output, and
  reference contracts belong to that common base. Command totals may grow; only
  `research_paths - release_paths` is fixed to the four Broker auth commands
  plus `serve`, and `release_paths - research_paths` remains empty.
- Catalog expectations must be derived from the actual post-WP03 Catalog and
  structured reference metadata. WP08's nested produced-reference rule is a
  Catalog-wide invariant; this packet must not introduce a Runtime-only
  validator or flatten nested references to simplify the build matrix.
- Standard native auth and research Broker auth have different ownership and
  trust boundaries. Presentation must not imply that one is a mode selected by
  the user at runtime. Standard native authentication state, learned
  permissions, and attachment authority remain outside Workspace Manifest
  desired/applied state.
- Current `context`, `--context`, `context_id`, `ProjectInstance`, `project_id`,
  and `instance_id` are predecessor vocabulary. Final design work in this
  packet must use `manifest`, `--manifest`, `workspace_manifest_id`, Workspace,
  and `workspace_id` without an alias, while leaving the actual domain migration
  from the durable Workspace Manifest contract.
- Release artifacts and public site generation must derive from executable
  release Catalog evidence, not a hand-maintained denylist alone.
- Existing prerelease artifacts are immutable. A correction ships in a later
  prerelease; no prior archive, tag, or release is overwritten.
- Documentation remains English in repository authorities. The architecture
  site may retain its reviewed bilingual source policy, but both locales must
  describe the same supported surface.
- No credentials, private identifiers, live provider responses, or
  authenticated transcripts may enter fixtures or documentation.

## External facts

- **Source:** [Go command: Build constraints](https://pkg.go.dev/cmd/go#hdr-Build_constraints), official Go documentation, checked 2026-08-23.
  A `//go:build` expression determines whether a file is included in a
  particular build, and tags passed with `-tags` are additional constraints
  considered satisfied during that build.
- **Source:** [Go command: build flags](https://pkg.go.dev/cmd/go#hdr-Compile_packages_and_dependencies), official Go documentation, checked 2026-08-23.
  `-tags` is a build input. It is not an executable runtime input. This supports
  compile-time surface selection but does not by itself prove that Tobari has no
  separate runtime selector; repository tests must prove that product fact.

## Observation-only unknowns and required checkpoints

No product or architecture choice remains open in this packet. Implementation
may proceed only after observing the following against the post-WP03 baseline:

- [ ] Fetch `origin/main`; record actual HEAD, branch/upstream, clean or
      explicitly owned worktree status, and the completed WP01/WP02/WP08/WP03
      commits/diff. Re-read the resulting governing contracts, ADRs, README,
      Catalog, schemas/migration, Taskfile, build constraints, release scripts,
      helper snapshot, site generator/content, and relevant tests.
- [ ] Rebuild the three observation binaries and derive complete Catalog path,
      role/effect, recursive reference, help, identity, topology, and recovery
      sets. Record absolute totals only as diagnostics.
- [ ] Verify the factual precondition that no stable release exists and
      inventory known prerelease version-JSON/binary consumers for the breaking
      note. Contradictory evidence is `WP04_BLOCKED`, not authority to design a
      compatibility reader or alias.
- [ ] Inventory predecessor research owner-only encrypted paths and WP01's
      private backup/quarantine result only to re-verify atomic isolation,
      normal-reader unreachability, legacy-handle denial, and non-delete/
      non-decrypt/non-import/non-rebind behavior. Consume WP01's synthetic
      migration/journal/rollback/secret-free evidence, including Linux root-key
      movement, zero macOS Keychain operations, exact-byte rollback through the
      unchanged item, and fail-closed conflict with fresh canonical auth state.
      Insufficient Docker or secret evidence cannot reopen secret migration.
- [ ] Inventory authored architecture-site research material and stale Active
      packets by semantic owner so the fixed IA removal and supersede/reconcile
      work has exact targets. Unrelated AWS, agent, or harness uses of `profile`
      remain outside the build boundary.
- [ ] Confirm the exact release-archive platforms and host-runnable target used
      for artifact inspection, plus any external automation still invoking the
      old contributor binary or tag. Findings affect the breaking note and
      cleanup guidance, not the no-alias decision.

## Inferences and hypotheses

- **Inference from verified Catalog/site facts:** the public-site contradiction
  is authored-information-architecture drift, not evidence that `tools/sitegen`
  currently generates the research Catalog. The generator's no-tag executable
  and committed generated Catalog are both release-surface.
- **Inference from the exact current path-set delta:** the four auth commands and
  `serve` form the current research capability group. Treating them as one
  surface is a design recommendation, not proof that they can never receive
  independent lifecycles; any future split requires a new outcome and decision.
- **Hypothesis to test during implementation:** deriving program identity,
  recovery, documentation Catalog, and release evidence from one compiled
  capability-surface authority will prevent the separate drift seen after the
  native-auth and Operator Console commits.
- **Hypothesis to test with contributor review:** renaming the local executable
  to `tobari-research` and removing it from standard site IA will reduce the
  impression of a supported product variant without making the research code
  undiscoverable to maintainers.
- **Fixed consequence from the accepted decisions:** retaining predecessor
  Context UUID bytes for standard WorkspaceManifestID migration does not permit
  reuse of a research Broker vault or handle. The new research surface rejects
  legacy handles and starts only from explicit new login/import; encrypted
  predecessor material remains quarantined private evidence until a separately
  reviewed cleanup outcome.

## Security vocabulary fixed by migration evidence

- **Research authentication authority:** the complete replay-capable set of
  ciphertext plus filesystem bindings, handles, lookup indexes, projections,
  and provider state. Quarantining less than this set can leave an implicit
  authority path even if no explicit import occurs.
- **Inert recovery material:** secret-bearing material that cannot authorize or
  resolve an operation because every normal reachability/binding path is
  absent. A preserved macOS Keychain root key has this status only after the
  complete dependent filesystem research set is atomically quarantined.
- This distinction is a durable security contract for the future ADR and
  security model, not a platform workaround or implementation exception.

## Thesis evidence

- Repeated design decision or point of agent confusion: standard-only auth and
  serve exclusions were added in separate commits and separate tests, while
  docs, version identity, and research binary presentation retained older
  vocabulary.
- User outcome or friction observed in the minimal slice: the executable safely
  excludes research commands, yet standard documentation can still instruct a
  user to run them through `tobari`.
- Code workaround or exception being considered: additional negative greps and
  command-specific tags would preserve the ambiguity rather than define one
  build-surface contract.
- Current thesis that resolves it, or proposed thesis revision: preserve Thesis
  3's native-standard/Broker-research boundary but rename its capability concept
  to a closed release/research surface and make Catalog, topology, identity,
  release, and IA consequences executable together.
- Downstream impact: theses, product, architecture, security, harness, release,
  authentication, agent-readiness, ADR, Taskfile, build identity, Catalog,
  topology constraints, site generation/content, release lint, fixtures, and
  documentation tests.

## Reproduction or observation

The following bounded historical observation was run at the pre-WP01 baseline
commit with outputs under a private temporary directory. It performs no Docker
or credential I/O and must be rerun against the integrated post-WP03 HEAD before
implementation:

```sh
probe_dir=$(mktemp -d)
go build -trimpath -o "$probe_dir/tobari-release" ./cmd/tobari
go build -trimpath -tags=tobari_dev \
  -o "$probe_dir/tobari-standard-dev" ./cmd/tobari
go build -trimpath -tags='tobari_dev tobari_experimental' \
  -o "$probe_dir/tobari-dev" ./cmd/tobari

"$probe_dir/tobari-release" help --format agent
"$probe_dir/tobari-standard-dev" help --format agent
"$probe_dir/tobari-dev" help --format agent
go version -m "$probe_dir/tobari-dev"
```

Historical root path counts were 40, 40, and 45. The third path set minus the
first was exactly the five research commands; the reverse difference was empty.
The totals are not future goldens. Temporary paths and binary bytes are not
release evidence.

## Security and public-boundary notes

- Assets and side effects involved: future local builds, release archives,
  generated documentation, and the existing optional research cluster. This
  packet itself adds documentation only.
- Credentials or confidential data involved: none. Future research tests use
  existing synthetic fixtures; live provider replay remains optional and
  secret-free.
- New dependencies, destinations, files, processes, or generated content: no
  new third-party dependency or network destination. Future work renames build
  constraints and local output, updates generated site data from a reviewed
  commit, and runs existing Go/Task/Docker boundaries.
- Trust-boundary fact: release stays Gateway plus OPA with Workspace-owned
  native credentials. Research stays Gateway plus OPA plus Auth Broker with
  existing control/egress separation. The plan changes selection proof and
  presentation, not authentication mechanics.
- ADR 0079 fact: capability surface is system/build authority. Workspace
  Manifest revisions and Runtime revisions cannot select it. Standard native
  credential bytes remain in Workspace home and must not be read into Manifest
  migration; learned permission and attachment authority remain separately
  owned even when keyed by WorkspaceManifestID/WorkspaceID.
- Output contract: build identity is complete deterministic output; Catalog
  help is complete delivery with no provider call. No pagination, retry,
  mutation idempotency, or external schema drift applies to surface discovery.
- Publication and licensing: research binaries and Auth Broker remain
  unpublished. Standard architecture-site content must not imply otherwise.

## Glossary of current terms

- **Capability profile:** current `standard|experimental` compile-time enum;
  proposed replacement is in `plan.md`.
- **Resolver channel:** current `embedded|development` image-authority enum;
  independent from command membership.
- **Standard profile:** current normal/release capability set.
- **Experimental profile:** current repository-only superset selected by
  `tobari_experimental`.
- **Development build:** ambiguous current phrase used for both repository
  resolver binaries and the experimental/research binary.
- **Profile (unrelated):** a harness gate configuration, AWS configuration
  record, or agent configuration; these meanings are not build authority.
- **Context:** historical predecessor public/domain term. ADR 0079
  replaces it with Workspace Manifest/Manifest and retains no final public
  alias; current-behavior evidence above deliberately keeps the historical
  spelling.
- **Workspace Manifest:** accepted final host-owned, CLI-managed, revisioned
  desired declaration. Capability surface and authentication state are not its
  fields.
- **Capability surface:** immutable executable capability identity. It is not a
  durable resource, Manifest revision, Runtime revision, or runtime selector.
