# Work Plan: Publish the smallest defensible Tobari V1

- Status: Proposed
- Goal: [goal.md](goal.md)
- Context: [context.md](context.md)
- Tasks: [tasks.md](tasks.md)
- Retirement record: [capability-retirement.md](capability-retirement.md)
- Context decision: [context-capability-envelope](../context-capability-envelope/plan.md)
- Source-access implementation: [context-source-access](../context-source-access/plan.md)
- Context-owned policy decision: [ADR 0066](../../decisions/0066-context-owned-policy-replaces-presets.md)

## Chosen approach

Cut the first release back to one complete proof: an untrusted coding process
runs in a bounded Docker Workspace; every supported outbound HTTP/HTTPS attempt
crosses the shared trusted Gateway; OPA authorizes its normalized route-level
effect for the exact Context/project; and an optional static broker handle is
resolved only after allow. Strengthen Context as the host-owned capability
envelope that fixes direct source access and snapshots a network guardrail;
keep runtime recipes, exact policy review, and lifecycle workflows that make
that proof usable.
Remove maintenance-heavy variants that are not necessary to establish it, then
publish the matching V1 artifacts before adding another substrate or protocol.

The release is intentionally asymmetric: Docker supplies a lightweight
execution boundary, while Tobari's differentiated contract is effect
authorization and credential mediation. The first release does not attempt to
win a container-versus-microVM isolation comparison.

## Alternatives considered

### Publish every currently implemented provider plan

This preserves maximum apparent feature count, but makes release readiness
depend on AWS CLI variants, Datadog OAuth, Codex and Claude pinned behavior,
refresh/signing logic, a companion channel, and a much larger manual matrix.
Those paths do not strengthen the generic post-policy static-handle invariant.
They are removed until repeated public usage justifies one independently
reviewed provider outcome.

### Add clone mode before release

Clone mode would reduce host source-tree integrity risk, but it creates Git,
untracked-file, submodule, LFS, worktree, editor, watcher, and apply-back
contracts. Docker Sandboxes also retains direct mode as its default. Tobari
keeps the current honest direct-mount authority for V1 and requires separate
evidence and a separate packet before adding isolated working trees.

### Replace Docker with or abstract over microVMs now

A microVM can strengthen host isolation, but it does not replace trusted
host-side effect enforcement and credential mediation. A backend abstraction
without a second proven backend would freeze guessed common semantics. V1 keeps
the concrete Docker boundary; any later substrate must first prove that the
Gateway, OPA, and Broker stay outside agent-controlled root authority.

### Keep compaction as the first permission-fatigue answer

Path-prefix compaction expands authority from observations and adds a second
opaque-reference workflow before real denial-volume evidence exists. V1 keeps
human batch review and exact learned rules, and adds explicit owner-selected
presets whose guardrail and revision are known before a Workspace exists.
Time-bounded leases or learned-rule compaction still require measured review
friction and a new policy decision after release.

## Design

### Public contract

The retained public surface is:

| Area | Retained V1 commands or behavior | Narrowing |
|---|---|---|
| Discovery and diagnostics | `help`, `version`, `doctor` | Claims and checks describe the narrower provider/runtime surface. |
| Shared enforcement | `cluster up`, `cluster status`, `cluster denials`, `cluster logs`, `cluster down` | One Gateway, one OPA, and one locked Auth Broker remain; companion state and lifecycle are removed. |
| HTTP permission workflow | `policy candidates`, `review permissions`, `policy rules`, `policy allow`, `policy deny`, `policy reset`; `policy preset list`, `policy preset show`, `policy preset init`, `policy preset validate` | Exact learned rules and explicit human batch review remain below one immutable Context guardrail. The internal `policy apply-reviewed` completion stays catalog-owned but is not a public command. Built-in and strict local custom presets replace implicit initial policy. `policy compactions` and `policy compact` are removed. No automatic retry follows allow. |
| Context and runtime | `context list`, `context show`, `context create`, `context use`, `config shell`, `config git`, `runtime init`, `runtime build` | Context creation fixes `--source-access read-only|read-write` and snapshots `--policy-preset`; guided/advanced policy and narrow host projections remain below that guardrail. Docker recipe build is still an explicit trusted-host effect. |
| Workspace lifecycle | `tobari`, `status`, `list`, `delete` | Direct source access follows the permanently bound Context; persistent per-Workspace home, fixed resources, and Docker-only runtime remain explicit. |
| Authentication | `auth login`, `auth import`, `auth status`, `auth logout` | `auth login` requires `--provider github`; `auth import` accepts strict static owner manifests. Status and help say “brokered.” Tool-native state is separately described as Workspace-owned. |

`policy.learning` and `authentication.broker` remain public capability IDs with
narrower surfaces. `policy.presets` is one new public capability ID;
source-access selection remains part of `context.composition`. Removing compaction removes
the `policy-compaction` producer/consumer graph entirely. Policy candidates
remain opaque exact references produced by discovery and consumed unchanged by
`policy allow` or `policy deny`; the terminal review workflow retains its
command-bound fixed decision-set mutation.

Every Context records a direct source access value and one complete policy
preset snapshot. `read-write` remains the default for the ordinary coding
outcome; `read-only` makes only the selected source bind read-only while the
Workspace home and tmpfs remain writable. The value is immutable for the life
of the Context. The same host root may therefore have a read-only Workspace in
one Context and a read-write Workspace in another; the latter and ordinary
host processes can still change bytes observed by the read-only Workspace.

Preset selection is by canonical `builtin/<name>` or `custom/<name>` identity.
Context creation validates, normalizes, digests, and copies the complete preset
into Context-owned state. Editing or deleting the source preset never changes
an existing Context. `context show` reports source access, preset origin,
snapshot digest, effective guardrail summary, and learned-decision count.

V1 owns three built-ins:

- `builtin/offline` terminally denies every outbound HTTP/HTTPS effect and
  produces no permission candidate.
- `builtin/reviewed-exact` grants nothing initially and permits eligible
  effects to enter exact review under the existing public/private and protocol
  boundaries.
- `builtin/get-only-reviewed` grants nothing initially, permits only `GET` to
  enter exact review, and terminally denies `HEAD`, non-GET methods, and every
  effect outside its destination ceiling.

The guardrail is evaluated by the trusted Tobari-owned system policy before
baseline grants, learned exact rules, or Advanced Rego. None can exceed it.
Custom presets are strict, bounded, owner-only, non-executable data with an
explicit destination ceiling, method ceiling, optional exact baseline grants,
baseline denies, and declared GraphQL classification points. They contain no
secret, wildcard host, IP literal, arbitrary Rego, shell, provider refresh, or
external-fetch behavior. A baseline grant is Context-wide and must be shown as
such; learned rules remain project-bound. Deny retains terminal precedence.

The only product-owned provider is GitHub.com:

- `auth login --provider github` runs the existing fixed `gh` acquisition in a
  private trusted-host home, binds the canonical executable bytes for the
  complete flow, captures one bounded token, deletes the temporary home, and
  stores the token through the static broker plan.
- Exact `gh` product-version equality is not a security boundary. Acceptance
  depends on fixed argv, bounded strict output, stable executable digest during
  the flow, cleanup, and a recorded manual compatibility observation.
- `auth import <owner-id>` reads one primary secret only from
  protected non-terminal stdin after all public inputs and the strict
  non-executable schema-v1 manifest are validated.
- Owner manifests may declare only exact HTTPS target/header replacement and
  bounded handle projections. They cannot select executable code, refresh,
  signing, policy, arbitrary methods, or provider business operations.
- Built-in Chatwork is removed; an owner may express the same static binding as
  a local manifest without making Tobari own that provider's product contract.

Tool-native authentication remains the universal fallback and default for
unsupported providers. Its real credential is in the per-Workspace home and is
readable by that Workspace. The CLI and documentation must not call this
brokered or imply that Tobari keeps the credential outside the Workspace.

The permission claim is “authorize the attempted HTTP effect,” not “authorize
the exact business effect.” The ordinary identity contains trusted Context and
project plus scheme, host, port, method, and path. Declared GraphQL endpoints
also contain operation type and root field. Bodies, ordinary query values and
headers, GraphQL arguments and variables, upstream account authority, and
provider-side consequences remain outside the guarantee.

All retained mutations preserve their declared effect, intent, fixed or opaque
target binding, impact, outcome classification, and no-automatic-replay rules.
Collection delivery and coverage remain as currently cataloged unless a
removed field makes a schema reset necessary.

Compatibility is the pre-public rule from ADR 0027: the resulting source tree
is the only V1. No prior development state, command alias, schema reader,
provider record, or image contract is migrated. Unsupported state fails
closed; the operator removes and recreates development state explicitly.

### Layer changes

- Domain: add immutable Context source-access and policy-preset snapshot
  vocabulary plus guardrail invariants; reduce provider vocabulary to static primary-secret plans and the
  reviewed GitHub helper; remove dynamic credential kinds, refresh/signing
  plans, helper-acquired Anthropic support, companion state,
  managed-adapter selection, and policy-compaction
  identities while preserving broker binding/revision/rotation invariants.
- Application: carry complete Context creation inputs and preset selection
  through task-owned ports; narrow authentication validation and results; delete dynamic
  provider inputs such as AWS method; remove compaction discover/act use cases;
  preserve policy candidate and review flows.
- Infrastructure: render direct read-only/read-write mounts, load and snapshot
  strict presets, enforce the guardrail before any allow, and delete AWS, Datadog, OpenAI, Anthropic, Chatwork built-ins;
  dynamic host drivers; refresh/signing clients; companion bridge/session;
  managed credential-file injection; and now-unowned dependencies and assets.
  Retain root key, encrypted static vault, Auth Broker Unix sockets, project
  handle issuance, GitHub acquisition, owner manifest loading, Gateway
  recognition, post-allow resolution, and exact header replacement.
- CLI and catalog: add preset commands and Context creation/report fields;
  remove retired command specs, flags, faults, output fields,
  references, provider selector rows, and recovery commands; update auth help
  and output wording; regenerate architecture-site catalog data from
  `cli.Catalog`.

### Data and control flow

```text
trusted host owns Context, policy, root key, vault, and provider manifest
       │
       ├─ GitHub fixed login or protected-stdin static import
       ▼
locked Auth Broker stores one encrypted primary secret + revision
       │
Workspace entry requests a Context/project/binding-specific opaque handle
       ▼
untrusted Workspace sends ordinary HTTP/HTTPS with handle in exact binding
       │
trusted Gateway removes handle and introspects non-secret binding metadata
       │
       ├─ OPA deny ──> audit/denial ──> human review ──> explicit manual retry
       │
       └─ OPA allow ─> resolve exactly once ─> replace exact header
                                      ───────> DNS/public-IP validation/upstream
```

The Workspace never receives the broker primary secret or root key. Gateway
does not resolve before allow, does not forward a recognized handle, and does
not fall back to Workspace-owned auth when broker validation fails.

### Error and cancellation behavior

- Invalid Context, project, provider, manifest, binding, stdin mode,
  executable, or state shape fails before credential acquisition or mutation.
- OPA, Gateway, Broker, root-key, vault, DNS, public-address validation, or
  exact binding uncertainty fails closed.
- A denied HTTP request performs no recursive DNS, broker resolution, or
  upstream attempt. A later permission change does not replay it.
- GitHub acquisition cancellation or an unclassified process result leaves no
  committed credential and returns a stable explicit recovery action. After a
  confirmed broker commit, later cancellation cannot convert success into
  replay permission.
- Import/login replacement and logout rotate or revoke every associated
  project handle atomically. Existing Workspaces require explicit re-entry.
- Removed provider names, `--method`, managed-adapter selectors, compaction
  commands, and unsupported state return cataloged invalid/unsupported faults
  where applicable; they never select a retained fallback.
- Rollback before first publication is a source revert plus development-state
  recreation. After publication, reintroduction requires a reviewed V2 or a
  compatible additive V1 decision; old unpublished snapshots are not a
  rollback authority.

### Security and public boundary

The design deliberately retains these accepted risks: any process in a
read-write Workspace can change or delete the selected host root; a read-only
Workspace can read and exfiltrate it and observes changes made by the host or
another read-write Context; any process can read
Workspace-owned credentials and broker handles projected into that Workspace;
an allowed destination can receive allowed request data; container escape
reaches the Docker Engine/kernel trust boundary; and compromise of a shared
Gateway, OPA, Auth Broker, root key, or host Docker authority has installation-
wide impact.

The design removes a resident trusted-host companion, provider refresh
endpoints, post-policy AWS execution/signing, multiple credential-state
parsers, a second static managed-secret store, and learned authority widening.
It introduces no new runtime destination or protocol. Release generation may
add only reviewed, pinned tooling for checksums, SBOMs, OCI signing/attestation,
and provenance. No live credential, account identifier, provider response, or
private URL is committed.

## Implementation slices

1. **Context capability-envelope decision.** Complete the
   [context-capability-envelope](../context-capability-envelope/plan.md)
   design gate and durable ADR before implementing either new axis.
2. **Parallel Context capabilities.** Implement
   [context-source-access](../context-source-access/plan.md) independently;
   complete [policy-compaction-retirement](../policy-compaction-retirement/plan.md),
   then apply [ADR 0066](../../decisions/0066-context-owned-policy-replaces-presets.md) against the
   exact-rule-only source model. Run
   [v1-auth-narrowing](../v1-auth-narrowing/plan.md) independently after the
   common Context decision.
3. **Durable V1 scope and claim.** Add one ADR that supersedes the dynamic
   portions of ADRs 0020, 0021, 0023, and 0025; revises ADRs 0009 and 0019;
   retires policy compaction; and records the exact retained surface. Propagate
   the product thesis and security-language change before mechanism deletion.
4. **Catalog and contract narrowing.** Add negative catalog/schema/capability
   tests, remove compaction and dynamic auth inputs/outputs/references/faults,
   make GitHub explicit, and clarify Workspace-owned versus brokered auth.
5. **Credential implementation retirement.** Remove managed injection,
   Chatwork and dynamic built-ins, refresh/signing code, host drivers,
   companion processes/protocols/state, dependencies, fixtures, and image
   contents. Preserve and re-prove the static broker invariants end to end.
6. **Integrated policy and runtime reconciliation.** Keep exact learned rules
   and batch review below preset guardrails, update cluster/doctor/status
   reports, and prove both direct source-access modes and shared services.
7. **Executable documentation and release artifacts.** Complete
   [first-public-release-artifacts](../first-public-release-artifacts/plan.md):
   regenerate catalog, schemas, capability claims, architecture site, and
   release docs; prepare image, CLI archive, checksum, SBOM, provenance, and
   Homebrew automation locally; then stop at the explicit publication approval
   checkpoint before any push, tag, OCI publication, GitHub Release, or tap
   update.

Each slice must keep the repository buildable. Contract-deletion tests land
with the corresponding removal rather than leaving a hidden fallback between
slices.

## Dependency and critical path

```text
Context capability-envelope ADR
        ├───────────────┬──────────────────────────────┐
        ▼               ▼                              ▼
source-access       compaction retirement          auth narrowing
implementation          │                         and companion removal
        │               ▼                              │
        │          policy presets                      │
        └───────────────┴──────────────┬───────────────┘
                                       ▼
                    integrated Context/policy/security verification
                                       │
                                       ▼
                 Gateway/Auth Broker canonical source finalization
                                       │
                                       ▼
               release artifact preparation and synthetic dry run
                                       │
                                       ▼
                         publication approval checkpoint
```

The envelope ADR is the first decision gate. Source access can then proceed in
parallel with credential narrowing. Policy presets follow compaction retirement
because the preset schema and guardrail should target the exact-rule-only V1
model rather than retain a dormant prefix authority form. Final component
images cannot be built or published until all three branches join and the
canonical Gateway/Auth Broker sources stop changing.

The likely longest branch is now the larger of credential narrowing and custom
preset/guardrail enforcement. Release automation, SBOM, provenance, and
Homebrew preparation may proceed with synthetic artifacts, but immutable V1
publication remains downstream of the integration join.

## Verification

- Unit and contract tests: all affected domain, application, CLI, auth
  provider, Gateway, Auth Broker, runtime, and documentation generators.
- Context/preset tests: immutable source access; exact Docker mount mode;
  writable home under read-only source; preset digest snapshots; no live
  propagation; guardrail precedence over baseline, learned, and Advanced
  allows; terminal non-learnability; and no model-provider bypass.
- Negative side-effect tests: zero DNS/upstream/secret resolution on deny;
  unsupported provider/plan/adapter/compaction inputs have zero provider,
  Broker, OPA mutation, or Docker side effects.
- Opaque-reference and complete-pagination tests: policy candidate round-trip
  remains exact; the retired compaction reference kind has no producer or
  consumer; retained exhaustive auth status remains complete.
- Structured output, hostile-output, and recovery tests: secret-free auth and
  audit output, hostile GitHub CLI output, malformed handles/manifests/states,
  rotation/logout, uncertain outcome, and explicit Workspace re-entry.
- Agent-readiness scenario and discovery-round-trip count: root help to exact
  retained command needs at most one scoped-help request; routine supported
  outcomes require zero external parsing or provider notation decoding.
- Human-handoff scorecard for setup/authentication candidates: GitHub login and
  static import each record certainty, safe input channel, manual step,
  cancellation, and next action.
- Manual observation: clean install, cluster start, exact HTTP deny/review/
  allow/manual-retry, static broker deny-before-resolution and allowed
  replacement, GitHub login with a disposable account, logout/re-entry, release
  archive verification, and Homebrew install/uninstall.
- Required profiles: `task check`, `task security`, `task public:check`, and
  `task release:check`.
- Generated-diff or artifact checks: catalog, capability/schema ledgers,
  Gateway/Auth Broker source snapshots, multi-architecture OCI indexes,
  checksums, SBOMs, provenance, release manifest, architecture site, and clean
  repository status.

## Rollout and rollback

There is no public-state migration because Tobari has no public release and
ADR 0027 makes the chosen source tree the sole V1. Before testing the narrowed
build, a developer must use the old snapshot to log out retired credentials,
delete development Workspaces, stop the old cluster, and explicitly remove and
recreate incompatible Context/installation state. The new binary does not
silently inspect, migrate, or delete secrets from unsupported state.

Release exposure is all-or-nothing: public and release checks reject
`unpublished` component authorities, mutable tags, missing platform images,
missing checksums/SBOM/provenance, source/snapshot drift, or an install path that
does not select the reviewed digests. No feature flag retains the old providers,
managed adapter, companion, or compaction.

Before publication, rollback is a reviewed source revert followed by the same
explicit development-state recreation. After publication, changes must honor
the released V1 compatibility contract; retired authority is not reactivated
by selecting an older hidden adapter or image.

## Documentation promotion

- `docs/00_theses.md`: product center, bounded execution wording, actual HTTP
  dimensions, Context capability envelope, two authentication exposure modes,
  static broker scope, direct mount/shared-service limits, and deferred work.
- `docs/01_product_contract.md`: retained command table, roles/references,
  output schemas, auth provider contract, permission workflow, compatibility,
  and installation outcome.
- `docs/02_architecture.md`: Docker as current substrate, shared control plane,
  static broker flow, and deleted companion/managed/dynamic paths.
- `docs/03_security_model.md` and `docs/THREAT_MODEL.md`: precise guarantees,
  accepted filesystem/container/shared-service risks, and retired attack
  surfaces.
- `docs/04_harness.md` and `.harness/*`: narrowed executable claims, schemas,
  capability surfaces, negative checks, and release evidence.
- `docs/05_public_repository.md` and `docs/06_release.md`: artifact, SBOM,
  provenance, OCI identity, GitHub Release, and Homebrew contract.
- `docs/07_authentication.md` through `docs/09_agent_readiness_validation.md`:
  static manifest/GitHub acquisition only, exact external contract, manual
  validation, and updated readiness scenarios.
- Durable ADRs: one Context capability-envelope decision and one V1
  supersession/retirement decision, not historical implementation detail.
