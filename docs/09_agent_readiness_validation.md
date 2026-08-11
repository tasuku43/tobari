# Agent Readiness Validation

Tobari is agent-ready when a coding agent can discover the shared cluster,
run the root command from a project directory, enter an exact CWD-owned
Workspace without an ID, configure or inspect Context authentication without
exposing a real credential, configure narrow shell and Git identity projections
without copying host configuration, and recover from denied network requests
without source inspection. This is also a product-adoption check: the bounded path
must be easier to choose than running the agent on the host. Human entry below
ancestor roots explicitly chooses reuse or creation; that interaction is
outside the machine help contract.

## Agent integration boundary

The current CLI and its catalog already close the supported agent workflow:
read-only discovery exposes the denial and an opaque candidate, a trusted host
performs one exact allow or deny action, and the agent retries only after that
action succeeds. The same catalog owns runtime initialization and build as
explicit host-side actions. It also owns Auth Broker login/import/status/logout:
the trusted host selects one Context/provider, while a Workspace receives only
the resulting project-bound handle on re-entry. No official Codex or Claude
Code plugin, MCP server, or generic executor is required for this local
outcome.

A future integration should be a thin skill or wrapper over scoped catalog
help and existing commands. It must not copy command metadata, create a second
permission registry, accept credentials, or expose Docker/OPA authority. A
plugin becomes worthwhile only when repeated cross-repository distribution
creates a real packaging need; an MCP surface requires a separate live-service
or authenticated-action contract.

## Experience scorecard

The current implementation is safe and catalog-complete, but the following
journey is the product baseline to improve:

| Journey | Current surface | Desired evidence |
|---|---|---|
| First isolated session | Explicit `cluster up`, then `tobari`; `doctor` is recovery-only | One obvious CWD-first entry with cluster/bootstrap complexity guided or hidden while retaining an explicit recovery command |
| Agent work | Reusable root, home, and deny-by-default Gateway | The user sets the boundary first; the agent works freely inside it without per-command supervision, host credentials, or direct egress |
| First denied request | 403 plus host-side `policy review` | The agent can explain that the host must review the secret-free exact request; brokered authentication inherits no broad static allow and one fixed next command is available |
| Permission growth | Installation-wide human `policy review` Permission Inbox; machine `policy candidates`, then `policy allow --id` or `policy deny --id` | TTY users scan exact effects, stage several choices for one Context from detail, and Apply once without a second confirmation prompt; redirected review remains read-only and machine actions remain bound to one Context-scoped opaque reference |
| Advanced policy | Edit trusted-host Rego explicitly | Remains an explicit escape hatch, never a prerequisite for routine success |
| Execution setup | `context list`, `context show`, `context use --name NAME`, `tobari --context NAME` | The user can inspect stable Contexts, change only the omitted-Context default, and create same-root Tobari in different Contexts while one Gateway/OPA/Auth Broker cluster routes trusted principals and bound handles |
| Context configuration | Human `config shell` / `config git` wizard or complete direct flags, then a matching entry | A terminal user reviews current and proposed state without assembling every flag, while agents and scripts use one deterministic invocation; partial/redirected/JSON input never prompts |
| Shell presentation | `config shell`, then enter a new session | A Context inherits exported `PS1` by default or independently selects one of four allowlisted shell variables without inheriting arbitrary host environment or startup files |
| Git identity | `config git`, then enter the matching root | A Context optionally supplies only a lower-precedence `user.name`/`user.email` pair without copying Git files, authentication, signing, helpers, executable settings, or arbitrary keys |
| Runtime customization | `runtime init`, edit the current Context Dockerfile, `runtime build` | The user gets a Context-specific runtime image without naming an image or editing the Context manifest; failed builds preserve the previous image and other Contexts are unchanged |
| Context authentication | `auth status`; built-in `auth login [--provider PROVIDER]` with terminal selection on omission and explicit AWS `identity-center|console`; protected non-terminal stdin `auth import`; `auth logout` | Reviewed fixed host CLI drivers acquire GitHub/AWS/Datadog state. Omitted provider selection uses the typed installed collection and requires one explicit human choice without exposing credential state. Each current built-in displays its one supported Workspace client tool as automatically selected, preserving Provider then Tool as separate concepts without a redundant second prompt; AWS method remains a third acquisition axis. A private resident companion refreshes strict AWS state and Broker directly refreshes the fixed Datadog DCR session, each only after exact OPA allow. The user sees only secret-free state/revision/account metadata, re-enters to receive a project-bound handle, and can revoke every handle without exposing static secrets, OAuth state, temporary credentials, or signing keys |
| Context-specific work tools | Published-base `gh` and `aws`; optional locally built toolbox adds `kubectl`, `cwk`, `pup`, and `twg` | Public-base contents stay unchanged; local artifacts have pinned identity and license inventory. Datadog pup has exact US1 OAuth login and refresh authority; static examples retain explicit no-refresh limits and general TWG remains unsupported |
| Recovery and cleanup | `tobari`, `status`, `delete`, and `cluster down` | Reuse, recovery, and removal are obvious, CWD-local, reversible, and ownership-checked |

The table is a review baseline, not a claim that the desired command count is
already met. Readiness evidence records command count, discovery rounds,
external-processing count, and the exact next command separately for machine
and human paths.

The current human-path evidence supports keeping the Permission Inbox,
CWD-owned entry, explicit cleanup, and Context runtime commands. The first
PTY replay removed `doctor` from the happy path and confirmed that TTY review
already closes the allow/deny decision; JSON review plus `policy allow --id`
remains the redirected or machine path. Context configuration evidence does
justify replacing the pre-v1.0 `context shell configure` path with the common
`config` namespace; it does not merge runtime or authentication workflows into
that boundary. The valid recovery path after an allowed
permission is `tobari` re-entry; there is no `tobari retry` command. Runtime
builds apply through the current Context, and only Workspaces bound to it pick
up the promoted image on their next matching-Context root entry while
preserving their home. Future UX
work may simplify bootstrap or polish terminal redraws, but those observations
do not change the current contract or authorize a second command surface.
Context selection has one explicit outcome: the omitted-Context default changes
without Docker or enforcement mutation. A newly created Context requires an
explicit `cluster up` to activate a new validated all-Context projection.

Authentication has one similarly explicit host/Workspace handoff. `auth
login`, `auth import`, and `auth logout` mutate only the selected Context's
encrypted broker record and revoke prior handles; they never grant policy or
rewrite a running process. Their structured result says
`workspace_reentry_required`. Credential ownership is Context-wide: every
project permanently bound to the Context is eligible, but each receives a
distinct current handle only on its next matching root entry. That entry
reconciles only the changed work container while preserving logical identity
and home. Logout revokes all handles immediately; next entry removes the
environment projection and only unchanged Tobari-owned complete files. `auth
status` is the exhaustive, secret-free Context view and never infers “not
configured” from a locked broker.

Parent-owned human-path evidence uses a real-PTY capture boundary. The
reviewable bundle records `rows`, `cols`, `TERM`, one short input event at a
time, monotonic timing, redraw-preserving output checkpoints, exit status, and
SHA-256 digests. Raw bytes stay in a task-owned directory outside Git; only a
reviewed projection and its relative metadata belong in a work packet. The
capture boundary does not teach a blind child the scenario route and does not
turn a piped replay into human success. Run its contract test with
`python3 scripts/test-pty-evidence.py`.

## Human presentation foundation evidence

The canonical synthetic corpus is
`internal/cli/testdata/human-presentation-foundation-fixture.json` (SHA-256
`c4776692325ce2f6c4d707118d9072661a1dcb43fac29b815711af4d253e9fc6`,
1,290 bytes, five cases). Its presentation-independent answer key is
`internal/cli/testdata/human-presentation-foundation-answer-key.json` (SHA-256
`223ec64482852ce774bd7555da85e2535168118e50b25e716121cc2669666301`,
1,624 bytes, five cases). Both pins are recorded in `.harness/schemas.json`.
The answer key fixes lifecycle, scoped-empty, warning, failure, and cancel facts,
exact Next argv, and negative-inference canaries. Each supported answer requires
one task invocation and zero external processing steps.

`TestPinnedHumanPresentationCorpusDrivesEveryTerminalMode` proves that colored
TTY, `NO_COLOR` TTY, and redirected text retain the same semantic document after
ANSI removal. The catalog-wide and AST checks reject undeclared or
style-dependent human structure; raw-selector tests inject repeated idle polls
and require one initial render and one terminal restoration; policy review and
rules cancellation prove exit 11 with zero action calls. This evidence changes
human text only and does not revise JSON or TSV schemas.

## Required scenario

Run against a clean Docker Engine with a synthetic root:

```sh
go run ./cmd/tobari help --format agent
go run ./cmd/tobari help cluster --format agent
go run ./cmd/tobari help tobari --format agent
go run ./cmd/tobari help status --format agent
go run ./cmd/tobari help config --format agent
go run ./cmd/tobari help config shell --format agent
go run ./cmd/tobari help config git --format agent
go run ./cmd/tobari help auth --format agent
go run ./cmd/tobari help auth login --format agent
go run ./cmd/tobari help auth import --format agent
go run ./cmd/tobari help auth status --format agent
go run ./cmd/tobari help auth logout --format agent
go build -o /tmp/tobari ./cmd/tobari
go run ./cmd/tobari context show --format json
go run ./cmd/tobari context list --format json
go run ./cmd/tobari config shell --variable PS1 --source inherit --format json
go run ./cmd/tobari config git --source literal --name "Tobari User" --email tobari@example.com --format json
go run ./cmd/tobari context use --name default --format json # changes only the current default without starting Docker
go run ./cmd/tobari cluster up
go run ./cmd/tobari auth status --format json
(cd /absolute/test/root && /tmp/tobari doctor --format json) # optional diagnostics after cluster bootstrap
(cd /absolute/test/root && /tmp/tobari)
(mkdir -p /absolute/test/root/root && cd /absolute/test/root/root && /tmp/tobari)
(cd /absolute/test/root && /tmp/tobari status --format json)
(cd /absolute/test/root && /tmp/tobari list --format json)
go run ./cmd/tobari cluster denials --tail 100 --format json
go run ./cmd/tobari context create --name restricted --format json
go run ./cmd/tobari cluster up # safely activates the new all-Context projection
(cd /absolute/test/root && /tmp/tobari --context restricted) # same root, different logical Tobari
go run ./cmd/tobari context use --name restricted --format json # changes only the omitted-Context default
go run ./cmd/tobari context use --name default --format json
go run ./cmd/tobari runtime init
# edit the current Context's runtime/Dockerfile
go run ./cmd/tobari runtime build --format json
(cd /absolute/test/root && /tmp/tobari) # reconciles an existing Workspace to the new runtime image
go run ./cmd/tobari policy review --tail 100
go run ./cmd/tobari policy candidates --tail 100 --format json
go run ./cmd/tobari policy allow --id PCY_ID
go run ./cmd/tobari policy compactions --format json
task integration:test # required reproducible synthetic Auth Broker proof
(cd /absolute/test/root && /tmp/tobari delete --context restricted)
(cd /absolute/test/root && /tmp/tobari delete --context default)
go run ./cmd/tobari cluster down --purge
```

Run the ordinary scenario with the official binary and its reviewed Gateway
and Auth Broker manifest digests. Use the `task build:dev` output
`bin/tobari-dev` only when the scenario deliberately validates unpublished
canonical source; development-image success is not official-image evidence.
The required scenario delegates synthetic credential, handle, broker, and
Gateway manipulation to `task integration:test`; the surrounding manual CLI
transcript does not reproduce those synthetic operations.

`list` emits a stable ID only as diagnostic information; lifecycle actions
resolve the same target from the current directory. `PCY_ID` denotes one exact
value emitted by `policy candidates`; the
compaction command may validly return an empty collection until enough exact
rules exist. The transcript must prove:

- Root agent help is a compact outcome/capability index.
- Exact scoped help schema 9 supplies recursive inputs/outputs, prerequisites,
  effects, references, failures, recovery commands, global flag placement, and
  directly executable success/error argv forms. One exact scoped-help call is
  sufficient for a known command; routine success requires zero source reads,
  prose parsers, or provider-notation decoders.
- Context discovery identifies the current default, stable Context identities,
  and separated agent, policy, and managed-adapter credential stores. `context
  show` reports separate secret-free broker/provider observation but no broker
  vault path/content, root key, primary secret, or handle.
- Context shell configuration reports all four allowlisted variables, preserves
  explicit empty literals, rejects arbitrary exported names before I/O, and
  applies inherited values only to a later child shell. An absent exported
  `PS1` retains the built-in prompt; no credential or startup-file state crosses.
- With the setting group wholly omitted, text terminal `config shell` and
  `config git` show the selected Context, current state, conditional input, and
  Apply/Cancel review on stderr. Direct mode returns the same report on stdout.
  Partial settings, JSON wizard attempts, redirected wizard attempts, and
  cancellation make zero mutation calls; raw-terminal failure retains the
  bounded English line-input path.
- Git configuration reports one atomic default/inherit/literal identity policy.
  The synthetic literal appears as a lower-precedence fallback after matching
  root entry; Workspace-global and repository-local identity override it. An
  inherited complete synthetic host-global pair is selected by stable root,
  while an absent/incomplete pair adds no fallback. No host config path or
  directive, repository-local host read, credential/helper/header, SSH,
  signing, hook, alias, URL rewrite, filter, proxy, or arbitrary key enters the
  Workspace, output, or logs. Identity creates no authentication or provider
  account claim.
- `context use` reports `default_updated`, never starts Docker, and does not
  mutate existing Tobari or shared enforcement. Explicit root `--context`
  selection leaves the marker unchanged.
- Cluster startup validates and atomically activates every Context policy and
  provider source while retaining exactly one Gateway, one OPA, and one locked
  Auth Broker; it unlocks the broker through the supported host root-key
  backend, and a failed Context candidate
  preserves the previous complete known-good projection.
- Cluster startup mounts no work root and Gateway receives only purpose-limited
  Context-aware principal, credential, and non-secret provider projections.
  Only Gateway mounts the broker runtime socket; OPA and Workspaces mount no
  broker socket or encrypted vault.
- Optional `doctor` emits the full read-only report, returns
  `diagnostic_failed` when any check fails, and treats warnings alone as
  healthy. It may inspect provider manifests, root-key/vault safety, broker
  state, and project bindings, but it does not initialize or repair policy,
  start/reconcile/unlock the cluster, create/replace a key, or mutate auth state.
- Cluster status schema 5 names all three shared components and explicitly
  reports auth provider-projection integrity, broker state, root-key backend,
  and always-present
  `credential_companion_state=ready|prepared|absent|unavailable`. The latter is
  host-process/channel readiness, not a fourth Compose service or credential
  state. Context report schema 7 carries complete shell-environment and Git
  identity policies plus matching secret-free authentication state; agents do not
  infer either from labels or filesystem paths. Public backend
  values are exactly `macos_keychain|xdg_file`, plus cluster diagnostic
  `unavailable`; `linux_xdg_file` is doctor-only diagnostic prose, not a public
  JSON enum.
- The root command binds the canonical current directory and a compatible local
  image selector without a name or root flag; an ancestor-root entry exposes all
  containing Workspaces nearest-first and requires an explicit reuse/create
  choice.
- Omitted Context selection resolves from the current default, then runtime
  image selection resolves from the Tobari's permanent Context binding; the
  legacy XDG default seeds the default Context and then `builtin` is used,
  without requiring source inspection.
- `runtime init` creates one owner-only current-Context Dockerfile and does not
  overwrite it or change the selected image.
- `runtime build` uses only that recipe directory, validates the runtime
  contract and image digest, derives the local image reference mechanically,
  and promotes it into the Context without a second image-selection command.
  Docker/BuildKit progress and concrete failure output remain available on
  host stderr in TTY and non-TTY execution, followed by a short text failure
  summary, so diagnosis needs no equivalent manual Docker command. A Docker
  build or pre-promotion validation failure leaves the previous selected image
  unchanged and identifies any candidate image or BuildKit cache that may
  remain.
  The exact official `runtime:latest` base is refreshed on this explicit build;
  explicit local or custom bases do not request a registry pull. An existing
  Workspace bound to that Context validates the promoted image on its next root entry,
  recreates only the work container when the runtime spec changed, preserves
  its home, and updates the stored project image only after success.
- Project metadata does not override the bound Context image for Workspace
  creation or existing runtime reconciliation.
- `auth status` returns one explicit Context identity, root-key storage backend,
  locked/ready broker state, a non-null exhaustive provider collection, and
  `configured`, `not_configured`, or `unavailable` per-provider state. It
  exposes no handle, vault path/content, root key, or primary secret.
- Auth mutations validate the Context, provider, fixed target, and impact before
  acquisition/vault I/O. Import rejects terminal stdin before reading. It reads
  one bounded primary secret from non-terminal stdin only after public
  Context/provider argument, intent, and mutation validation; infrastructure
  validates the selected existing Context, installed provider/acquisition mode,
  and broker readiness before broker send. Login is limited to the reviewed
  built-in host driver; logout makes no remote-revocation claim. Every success
  reports the credential revision only when configured and requires Workspace
  re-entry.
- Omitted `auth login` provider input requires interactive stdin and stderr,
  reads the selected Context's secret-free installed-provider status, and
  presents only reviewed login providers before mutation. The selected provider
  ID and the resolved Context returned by that snapshot pass into login, so a
  concurrent current/default change cannot retarget the choice. An explicit `--provider` performs no status read
  or selector interaction; `--method` without `--provider`, redirected
  omission, cancellation, and an installed collection with no reviewed login
  provider make zero login mutation calls.
- Built-in GitHub login uses one verified host GitHub CLI with fixed argv and a
  private temporary home, opens only the fixed device URL, and never asks to
  authenticate Git or configure a Git credential helper. AWS login uses one
  verified host AWS CLI device-code operation and persists only encrypted
  opaque cache state; request region is separate Context/tool configuration.
  Datadog login uses one verified host pup with fixed US1 argv and a private
  file-backed OAuth home, then persists only strict encrypted DCR client,
  token, and default-session state. Auth Broker contains neither provider CLI
  nor persistent provider home.
- Cluster readiness includes one private same-binary host companion over an
  authenticated encrypted reverse `docker exec` channel. It opens no listener
  and mounts no host socket. OPA denial causes zero companion or Datadog
  refresh calls; post-policy AWS and fixed Datadog refresh are per-record
  single-flight with a finite lock wait. Broker
  persists an encrypted task barrier before host execution, so unknown outcomes
  survive restart without replay; stale results after rotation or logout are
  rejected.
- Each Context/provider credential makes every permanently bound project
  eligible for a different opaque handle on its next matching Workspace entry.
  Handles remain stable across ordinary root entry and broker
  restart/unlock for the same credential revision/bindings, while replacement
  and logout revoke every old handle. Logout's next entry removes its declared
  environment projection and only unchanged Tobari-owned complete files. Agents
  never decode or reconstruct a handle.
- Gateway removes a valid handle, introspects only its non-secret exact
  binding, sends schema-5 OPA input, performs zero companion/secret-use/signing operations
  on deny, and resolves or signs the same revision exactly once after allow.
  Fallback occurs only
  when no Tobari handle marker exists in any inspected URL/header position;
  copied, malformed, misplaced, ambiguous, stale, revoked, or binding-mismatched
  markers fail as `credential_handle_invalid` without fallback or forwarding.
- `list` retains an explicitly exhaustive local collection, including empty,
  and reports Context with diagnostic IDs. Same-root/different-Context rows are
  distinguishable without making IDs routine action inputs.
- `tobari`, `status`, and `delete` accept either one prefix or command-local
  non-empty Context selector through the same catalog input. Scoped help shows
  both exact forms; duplicate, empty, unknown, or stale selectors fail before
  Workspace or Docker I/O. Resolution binds one stable Context ID before the
  nearest canonical ancestor is selected, so same-root Contexts remain
  distinct and deletion never guesses another Context.
- Status schema 3 reports selected Context ID/name even when no Workspace
  exists, explicit attachment observation when it does, and exact
  Context-preserving next argv. One scoped-help request plus one status
  invocation requires zero source inspection, joins, or external
  reconstruction.
- A child terminal exit leaves the logical Tobari existing for reuse, emits the
  host-side resume/delete guidance on stderr, and does not introduce a stopped
  state; detached `tobari delete` removes it, while `tobari delete --force`
  explicitly overrides an attached-session warning.
- A denied request produces bounded typed secret-free host/method/path
  evidence, the host policy path, and the exact review command. Baseline
  terminal denies remain audit evidence without inviting approval. Audit emits
  no query or headers, and a path containing a Tobari handle marker is replaced
  wholly by `/[redacted-auth-handle]`; URL/header handle-structure failures are
  non-learnable.
- Candidate discovery crosses every Context, keeps same-request/different-
  Context effects distinct, and emits opaque references
  without changing authority; orthogonal scheme, project-principal, or
  managed-credential failures remain diagnostics and do not become ineffective
  candidates.
- Permission Inbox selection groups only matching stable Context/project IDs,
  keeps same-label different-ID scopes separate, and exposes the exact effect,
  observation count, and latest retained observation before detail. The detail
  retains Context, root, and request; `a` or `d` is accepted only there and
  stages that unchanged opaque ID without granting authority. Several choices
  from one Context produce one final `p` Apply and one exact-revision bundle
  activation; switching Context first requires Apply or discard, and `q`
  discards with zero writes. `PCY_ID` denotes one exact value emitted by
  `policy review` or `policy candidates`; the machine transcript passes it
  unchanged to either `policy allow` or `policy deny`. All paths test the
  complete policy and activate without restarting OPA or a Tobari.
- Cleanup verifies exact owner and opaque-ID labels. Both `cluster down` forms
  preserve encrypted Context vaults and the installation root key; `--purge`
  additionally removes only shared CA and active policy-bundle volumes.

## Policy-learning scenario

The Docker integration test supplies the executable loop:

1. A mock-host GET is allowed.
2. A mock-host POST under `/denied` receives `403`, never reaches upstream, and
   is reported as a terminal baseline denial rather than a queue candidate.
3. Body-bearing PUT requests with different payloads receive `403`, expose
   fixed review navigation, and aggregate into one body-free exact candidate;
   a body-bearing PATCH is also reviewable.
4. `cluster denials` exposes the rejected dimensions, trusted XDG policy path,
   and exact review command. `policy review`, `policy candidates`, and
   `policy tail` expose the same pending exact proposals and opaque references;
   redirected review does not mutate policy.
5. Interactive review stages several same-Context decisions and activates one
   complete revisioned bundle on final Apply. The OPA container ID remains
   stable and OPA reports the exact expected revision. The machine `policy
   allow` path retains its one-reference contract. The exact
   retry succeeds with a third body value while a child path remains denied.
6. `policy deny` records and activates one exact project-bound terminal rule;
   the rejected request remains denied and its historical lines disappear from
   the actionable review queue.
7. Three separately denied and approved sibling paths produce one current
   `policy compactions` proposal.
8. `policy compact` retains the examples, activates the bounded prefix, permits
   a sibling, and keeps its adjacent outside-prefix canary denied.
9. A chunked upload reaches upstream before its delayed second chunk, and an
   SSE response exposes its first event before the delayed second event.
10. A declared synthetic GraphQL endpoint denies two previously unseen root
    fields as two independent exact candidates without retaining document or
    variable canaries. Allowing one root does not authorize the other; after
    both exact approvals, the unchanged request body reaches upstream once.
    An HTTP-only rule, malformed document, unsupported method, and exact deny
    cannot bypass or broaden this check.

Routine success and denial require zero undeclared provider parsers,
provider-notation decoders, source inspection steps, or exploratory provider
calls.

## Network and credential scenario

`task integration:test` additionally proves:

- Multiple Contexts still produce exactly one Gateway, one OPA, and one Auth
  Broker.
- Two same-root Tobari in different Contexts have distinct stable IDs, homes,
  containers, networks, principals, runtime authority, and policy authority,
  while edits to their shared mounted host files are mutually visible.
- Parent/child roots in different Contexts may run concurrently.
- Neither has direct egress, OPA access, or cross-Tobari reachability.
- HTTPS is authorized after CONNECT interception and validates the Tobari CA.
- OPA and Gateway outages fail closed.
- Declared GraphQL endpoints require every canonical query/mutation root,
  never fall back to HTTP-only rules, preserve the original body after allow,
  and keep source documents and variables out of OPA, audit, denial, and CLI.
- Tool-owned authentication state persists below one Tobari home and is not
  visible from another Tobari.
- The default passthrough adapter forwards a client-authenticated request only
  after allow, while its value is absent from mounts, logs, OPA input, and CLI
  output.
- The retained managed adapter is covered by exact Context/project/host binding,
  same profile-name cross-Context rejection, and post-allow injection tests.
- A configured broker provider gives same-Context projects distinct handles;
  neither Workspace can read the primary secret or use the other's handle.
  Its first exact L7 effect is a reviewable denial rather than inheriting a
  broad static host/method allow. OPA denial makes zero resolve calls, the
  exact learned allow makes one, and secret/handle
  canaries remain absent from OPA, audit, denial, CLI, and component-log output.
  Audit omits query/headers, replaces a handle-bearing path wholly with
  `/[redacted-auth-handle]`, and keeps structural handle rejections
  non-learnable. Passthrough/managed fallback occurs only when no Tobari marker
  appears anywhere inspected.
- Broker restart reports locked until cluster reconciliation supplies the root
  key; unlock retains valid same-revision handles. Credential replacement and
  logout revoke all previous handles, and re-entry supplies or removes only the
  current Tobari-owned projection.
- Cluster down and purge preserve the encrypted Context vaults and installation
  root key, and a later cluster up unlocks the preserved state; purge removes
  only the additional shared CA and active policy-bundle volumes.
- Identical effects may be allowed in one Context and denied in another;
  learning, exact deny, reset, and compaction never cross that boundary.
- Changing current Context does not migrate existing Tobari, and restart
  restores their durable Context bindings.
- Concurrent processes share one selected Tobari.
- Repeated root reconciliation does not grow owned resources.

## Interpretation rules

Agents must not infer:

- identity from a Context display name, container label, log order, or position;
- permission from a previous allow or similar path;
- exhaustive external history from complete CLI delivery;
- credential authority from a profile name;
- real credential value, provider account authority, policy permission, or
  cross-project usability from a provider ID, account label, credential
  revision, or opaque broker handle;
- absence of a Context credential from a locked or unavailable broker;
- safety from an executable name;
- protection or separation for files below overlapping selected read-write roots.

The opaque reference, declared local collection scope, cluster status, bounded
denial/log coverage, OPA decision, and structured failures are the supported
interpretation inputs.

## Failure and recovery validation

At minimum, exercise:

- invalid or inaccessible root before Docker mutation;
- nested-root entry lists all containing ancestors, reuses a chosen one, or
  creates a new root only after an explicit create choice;
- cancelled, unavailable, or stale Workspace selection performs no logical or
  Docker mutation and directs the user to retry or choose again;
- invalid, missing, incompatible, or conflicting image selection before Docker
  resource creation;
- invalid or incompatible Context image configuration before Docker mutation;
- unavailable or locked Auth Broker before login/import/logout and Workspace
  handle reconciliation;
- unsafe, missing, denied, or wrong-sized root-key state, including mandatory
  rejection when encrypted vault state exists without the original key;
- duplicate-key, unknown-field, oversized, collision-prone, symlinked,
  non-owner-only, built-in-overriding, or helper-backed user provider manifests;
- overlapping exact provider scheme/host/port/source-header/source-format
  recognition mapped to `ambiguous_provider_http_binding`, with no partial
  activation;
- empty/oversized credential stdin, provider acquisition mismatch, project- or
  temporary-selected, missing, writable, or changed host executable, hostile
  control-bearing provider output, cancelled GitHub/AWS/Datadog login, invalid
  account/status or credential-process capture, and setup/success cleanup
  failure while the
  previous credential remains unchanged; terminal import must perform no read,
  public validation precedes a non-terminal read, and runtime prerequisites
  precede broker send;
- invalid AWS start URL/SSO region/account/role, cancellation, device-login
  failure, malformed/oversized cache, changed executable digest, invalid or
  expired process credentials, and expired SSO session while the previous
  credential remains unchanged; request region is not login state;
- missing, changed, or writable host pup; non-US1, multi-session, missing
  default-organization, malformed, oversized, duplicate-key, short-lived, or
  digest-mismatched pup state; refresh redirects, proxy use, malformed or
  oversized responses, stale completion, and unknown network outcomes while
  the previous credential remains unchanged or is disabled for explicit
  reconciliation;
- corrupted, wrong-Context, or unsupported-version encrypted vaults without
  revealing ciphertext or secret data;
- copied, malformed, ambiguous, stale, rotated, revoked, wrong-Context,
  wrong-project, wrong-provider, wrong-target, wrong-header, wrong-revision, and
  structurally misplaced handles before upstream I/O, with fallback allowed
  only when no Tobari marker appears anywhere inspected;
- OPA denial with zero companion, broker resolve, Datadog refresh, AWS refresh,
  role-credential, or signing
  calls; unsupported SigV4a, presign, streaming/chunked/event, custom-endpoint,
  ambiguous-header, changed-length, and over-limit requests; any
  authority/method/path/query/signed- or policy-visible-header change between
  allow and signing produces zero refresh/sign calls; known pre-send Broker
  unavailability maps to secret-free `credential_broker_unavailable`, while an
  explicit unknown result or post-send timeout/invalid response maps to
  non-retryable `credential_refresh_outcome_unknown`;
- companion bootstrap/authentication/replay/gap/oversize/disconnect/timeout,
  peer-not-reading bounded write/close, receipt-only cancel acknowledgment, and
  outcome-unknown refresh paths; no host listener, host socket mount, child key/
  channel inheritance, blind replay, stale vault update, or secret-bearing log;
- an unowned, modified, or symlinked Workspace authentication file before any
  overwrite or removal;
- malformed or legacy state;
- invalid Rego before cluster reconciliation;
- invalid Rego before policy activation and stable OPA container identity after
  a valid hot bundle activation;
- invalid or stale policy bundles, exact revision timeout, authority-reducing
  transition order, and cross-Context reviewed sets before source write;
- invalid, stale, already-covered, or wrong-kind policy candidate references;
- duplicate, symlinked, group/world-accessible, concurrently changed, or
  malformed managed policy data before write;
- fewer than three, shallow, mixed-host, mixed-method, or stale compaction
  sources;
- failed learned-policy preflight before atomic replacement;
- partial startup mapped to non-retryable `cluster_start_failed`;
- partial root reconciliation mapped to non-retryable `runtime_reconcile_failed`;
- inherited Git identity lookup failure mapped to non-retryable
  `git_identity_resolution_failed` before any Docker inspection or mutation,
  with the prior projection preserved and no raw diagnostic or identity value;
- non-empty cluster removal rejection;
- down/purge preservation of encrypted Context vaults and the installation root
  key, with only shared CA and active policy-bundle volumes added by purge;
- unknown or modified opaque ID before Docker calls;
- partial delete remains retryable through `delete` and never removes logical
  state before exact resource cleanup;
- non-TTY root invocation fails before logical state creation;
- OPA unavailable and malformed decision;
- output write failure without partial structured data.
- `auth_mutation_outcome_unknown`, `unclassified_mutation_outcome`, and
  `mutation_output_write_failed` as non-retryable auth outcomes requiring
  `auth status` before another mutation.

A post-mutation raw adapter error must never become replay permission. Valid
structured outcome faults are preserved; unknown outcomes collapse to a
non-retryable contract fault.

## Manual GitHub validation

Automated tests make no live provider call. Before publishing an Auth Broker
release candidate, a reviewer uses a disposable GitHub test account on a
trusted host and records only secret-free outcomes outside the repository:

```sh
tobari cluster up
tobari auth status --context default --format json
tobari auth login --provider github --context default
tobari auth status --context default --format json
(cd /absolute/test/root && tobari) # re-enter to receive the project handle
# Inside that Workspace, after an exact policy allow:
case "${GH_TOKEN-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$(gh auth token --hostname github.com)" = "$GH_TOKEN"
gh api user --jq .login >/dev/null
exit
tobari auth logout github --context default --format json
(cd /absolute/test/root && tobari) # reconcile the revoked projection
```

The reviewer verifies the login uses the ordinary GitHub.com web flow, opens
the device page through the reviewed host driver (or shows the exact fixed
manual URL), and shows no Git credential prompt or Git configuration. The
canonical host executable/digest remains stable and its private temporary home
is removed. Status shows exactly one secret-free
account label/revision, `GH_TOKEN` has the
`tobari-h1_` handle shape, and the no-print equality test proves `gh auth token
--hostname github.com` returns that exact projected handle rather than the
primary credential. The allowed API call succeeds only after OPA allow, logout returns
`workspace_reentry_required`, and the prior handle receives
`credential_handle_invalid`. The reviewer also scans Gateway, OPA, and Auth
Broker logs for a private canary supplied only for this disposable test.

Do not save tokens, device codes, handles, root keys, vaults, host `gh` config, raw
terminal capture, or authenticated API responses. Manual evidence cannot prove
future provider availability or account authorization; it covers only the
reviewed release candidate and environment.

## Manual Datadog pup OAuth validation

Before publishing a Datadog-capable Auth Broker release candidate, a reviewer
uses a disposable default organization on Datadog US1 and records only
secret-free pass/fail outcomes outside the repository:

```sh
tobari cluster up
tobari auth login --provider datadog --context default
tobari auth status --context default --format json
(cd /absolute/test/root && tobari) # re-enter to receive the project handle
# Inside that Workspace, after one exact policy allow:
case "${DD_ACCESS_TOKEN-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$DD_SITE" = datadoghq.com
pup users --no-agent --read-only list >/dev/null
# Repeat after the original access token enters its five-minute refresh window.
pup users --no-agent --read-only list >/dev/null
exit
tobari auth logout datadog --context default --format json
(cd /absolute/test/root && tobari) # reconcile the revoked projection
```

The reviewer confirms the trusted-host driver invokes fixed
`pup --no-agent auth login --site datadoghq.com`, pup opens its ordinary browser
consent and loopback callback, the standard pup scope set is visible on the
provider consent page, and only one default US1 organization is captured. The
private host pup configuration is removed after acquisition and `pup` is absent
from the Broker image. OPA denial causes zero token selection or refresh. The
first read succeeds with a project-bound handle; the later read proves one
post-policy refresh without re-login; logout revokes the prior handle. Do not
save authorization codes, client/token/session state, access or refresh tokens,
handles, raw terminal capture, or authenticated responses.

## Manual AWS login-method validation

Before publishing an AWS-capable Auth Broker release candidate, a reviewer
uses a disposable test role on a trusted host and records only secret-free
pass/fail outcomes outside the repository:

```sh
tobari cluster up
tobari auth login --provider aws --method identity-center --context default
tobari auth status --context default --format json
(cd /absolute/test/root && tobari) # re-enter to receive the project handle
# Inside that Workspace, after one exact policy allow:
case "${AWS_ACCESS_KEY_ID-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$AWS_ACCESS_KEY_ID" = "$AWS_SECRET_ACCESS_KEY"
test "$AWS_ACCESS_KEY_ID" = "$AWS_SESSION_TOKEN"
aws sts get-caller-identity --region us-east-1 >/dev/null
# After the temporary role lease expires but while SSO remains renewable:
aws sts get-caller-identity --region us-east-1 >/dev/null
exit
tobari auth logout aws --context default --format json
(cd /absolute/test/root && tobari) # reconcile the revoked projection
```

The reviewer verifies Identity Center asks only for the supported start URL, SSO region,
account ID, and role; the reviewed host AWS CLI runs its fixed device-code flow
in a private temporary home; and output contains no SSO cache, role credential,
or signed header. Request region comes from an explicit CLI option or reviewed
Context/tool configuration and is not login state. The
three no-print comparisons prove AWS CLI receives one handle rather than real
credentials. OPA denial must cause zero companion, refresh, role acquisition,
and signing calls. The first bounded STS request succeeds once, and the second
proves automatic post-policy host-CLI refresh without re-login while the SSO
session remains renewable; logout revokes the prior handle.

Repeat the scenario with a disposable AWS console identity:

```sh
aws --version # trusted-host version is 2.32.0 or newer
tobari auth login --provider aws --method console --context default
tobari auth status --context default --format json
(cd /absolute/test/root && tobari)
case "${AWS_ACCESS_KEY_ID-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$AWS_ACCESS_KEY_ID" = "$AWS_SECRET_ACCESS_KEY"
test "$AWS_ACCESS_KEY_ID" = "$AWS_SESSION_TOKEN"
aws sts get-caller-identity --region us-east-1 >/dev/null
# Repeat after the temporary lease expires while the login refresh token is valid.
aws sts get-caller-identity --region us-east-1 >/dev/null
exit
tobari auth logout aws --context default --format json
```

The reviewer confirms fixed `aws login --remote` is used, no callback listener
or ambient profile is used, the exact region-bound AWS sign-in URL opens once
when possible and remains a usable terminal fallback, and temporary credentials
refresh through the post-policy companion path. A separate synthetic driver
test proves malformed, duplicated, and cross-region URLs cause no browser
action and AWS CLI versions below
2.32 fail before provider login; no live old-version transcript is retained.

Do not save the questionnaire values beyond the non-secret account/session review,
device or authorization code, SSO/login cache state, temporary credentials, handles, signed request,
raw terminal capture, or authenticated response. The flow validates only the
standard bounded header-based SigV4 subset, not SigV4a, presigning, streaming,
custom endpoints, or every AWS service.

## Evidence

The image prerequisite is satisfied by an anonymously retrievable Gateway
API-3 index built from source revision
`328196221c5be2861b67ec51339d0184b04c6b31` and Auth Broker API-2 index built
from source revision `a3fedb66ad5a72c19d6721f3f8da49852882ced8`, each for Linux
amd64/arm64 with reviewed API/role labels, non-root `1000:1000` users, and
entrypoints. Routine startup pins Gateway
`sha256:44a84576266617c78eae433ea53d60e199226dc7bc275b2aaa6c728875c91878`
and Auth Broker
`sha256:a2df8169fd1b28ab67d42c83c5181714ce5373ab74fe9931e84ab4542dc97fb1`
in `versions.env`. This image evidence does not replace the manual trusted-host
scenarios or release gates below.

```sh
task check
task runtime:test
task integration:test
task security
task public:check
```

Review the typed Gateway denial, tested OPA activation, three-service shared
topology, broker lock/unlock and handle-resolution counts, two project
networks, and Docker cleanup counts alongside command results.
`task integration:test` is the required synthetic Auth Broker evidence even
though `task runtime:test` also includes it; listing it explicitly preserves the
agent-readiness delegation and review boundary.
