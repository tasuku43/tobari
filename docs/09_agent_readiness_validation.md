# Agent Readiness Validation

Tobari is agent-ready when a coding agent can discover the shared cluster,
run the root command from a project directory, enter an exact CWD-owned
Workspace without an ID, configure or inspect Context authentication without
exposing a real credential, and recover from denied network requests without
source inspection. This is also a product-adoption check: the bounded path
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
| First denied request | 403 plus host-side `policy review` | The agent can explain that the host must review the secret-free exact request; one fixed next command is available |
| Permission growth | Installation-wide human `policy review` Permission Inbox; machine `policy candidates`, then `policy allow --id` or `policy deny --id` | TTY users see Context/root/request at selection, detail, and confirmation; redirected review remains read-only and the action remains bound to one Context-scoped opaque reference |
| Advanced policy | Edit trusted-host Rego explicitly | Remains an explicit escape hatch, never a prerequisite for routine success |
| Execution setup | `context list`, `context show`, `context use --name NAME`, `tobari --context NAME` | The user can inspect stable Contexts, change only the omitted-Context default, and create same-root Tobari in different Contexts while one Gateway/OPA/Auth Broker cluster routes trusted principals and bound handles |
| Runtime customization | `runtime init`, edit the current Context Dockerfile, `runtime build` | The user gets a Context-specific runtime image without naming an image or editing the Context manifest; failed builds preserve the previous image and other Contexts are unchanged |
| Context authentication | `auth status`; built-in `auth login github`; protected non-terminal stdin `auth import`; `auth logout` | The host opens the fixed GitHub device page when possible, otherwise gives one exact manual URL; no Broker browser/Git setup error intervenes. The user sees only secret-free state/revision/account metadata, re-enters to receive a project-bound handle, and can revoke every handle without exposing the primary secret |
| Recovery and cleanup | `tobari`, `status`, `delete`, and `cluster down` | Reuse, recovery, and removal are obvious, CWD-local, reversible, and ownership-checked |

The table is a review baseline, not a claim that the desired command count is
already met. Readiness evidence records command count, discovery rounds,
external-processing count, and the exact next command separately for machine
and human paths.

The current human-path evidence supports keeping the Permission Inbox,
CWD-owned entry, explicit cleanup, and Context runtime commands. The first
PTY replay removed `doctor` from the happy path and confirmed that TTY review
already closes the allow/deny decision; JSON review plus `policy allow --id`
remains the redirected or machine path. No observed journey justifies deleting
or merging a public command. The valid recovery path after an allowed
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

## Required scenario

Run against a clean Docker Engine with a synthetic root:

```sh
go run ./cmd/tobari help --format agent
go run ./cmd/tobari help cluster --format agent
go run ./cmd/tobari help tobari --format agent
go run ./cmd/tobari help status --format agent
go run ./cmd/tobari help auth --format agent
go run ./cmd/tobari help auth login --format agent
go run ./cmd/tobari help auth import --format agent
go run ./cmd/tobari help auth status --format agent
go run ./cmd/tobari help auth logout --format agent
go build -o /tmp/tobari ./cmd/tobari
go run ./cmd/tobari context show --format json
go run ./cmd/tobari context list --format json
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
- Scoped help supplies inputs, outputs, prerequisites, effects, references,
  failures, and recovery commands.
- Context discovery identifies the current default, stable Context identities,
  and separated agent, policy, and managed-adapter credential stores. `context
  show` reports separate secret-free broker/provider observation but no broker
  vault path/content, root key, primary secret, or handle.
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
- Cluster status schema 3 names all three shared components and explicitly
  reports auth provider-projection integrity, broker state, and root-key
  backend. Context report schema 4 carries matching secret-free authentication
  state; agents do not infer it from labels or filesystem paths. Public backend
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
  built-in helper; logout makes no remote-revocation claim. Every success
  reports the credential revision only when configured and requires Workspace
  re-entry.
- Built-in GitHub login opens only the fixed device URL through the trusted
  host, retains that URL as the fallback on a headless host, and never asks to
  authenticate Git or configure a Git credential helper in Auth Broker.
- Each Context/provider credential makes every permanently bound project
  eligible for a different opaque handle on its next matching Workspace entry.
  Handles remain stable across ordinary root entry and broker
  restart/unlock for the same credential revision/bindings, while replacement
  and logout revoke every old handle. Logout's next entry removes its declared
  environment projection and only unchanged Tobari-owned complete files. Agents
  never decode or reconstruct a handle.
- Gateway removes a valid handle, introspects only its non-secret exact
  binding, sends schema-5 OPA input, performs zero resolutions on deny, and
  resolves the same revision exactly once after allow. Fallback occurs only
  when no Tobari handle marker exists in any inspected URL/header position;
  copied, malformed, misplaced, ambiguous, stale, revoked, or binding-mismatched
  markers fail as `credential_handle_invalid` without fallback or forwarding.
- `list` retains an explicitly exhaustive local collection, including empty,
  and reports Context with diagnostic IDs. Same-root/different-Context rows are
  distinguishable without making IDs routine action inputs.
- `status` and `delete` resolve the explicit or current Context plus nearest
  canonical ancestor; `tobari` enters an exact pair directly and explicitly
  chooses among ancestor roots. Deletion never guesses another Context.
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
- Permission Inbox selection, detail, and confirmation retain Context, root,
  and request. `PCY_ID` denotes one exact value emitted by `policy review` or `policy
  candidates`; the transcript passes it unchanged to either `policy allow` or
  `policy deny`. Allow tests the complete policy and records one exact learned
  rule; deny records one exact Context/project-bound terminal rule. Both activate
  without restarting a Tobari.
- Cleanup verifies exact owner and opaque-ID labels. Both `cluster down` forms
  preserve encrypted Context vaults and the installation root key; `--purge`
  additionally removes only shared CA volumes.

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
5. `policy allow` tests a private complete policy copy, atomically stores the
   exact learned rule, recreates only OPA, and confirms it healthy. The exact
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
- Tool-owned authentication state persists below one Tobari home and is not
  visible from another Tobari.
- The default passthrough adapter forwards a client-authenticated request only
  after allow, while its value is absent from mounts, logs, OPA input, and CLI
  output.
- The retained managed adapter is covered by exact Context/project/host binding,
  same profile-name cross-Context rejection, and post-allow injection tests.
- A configured broker provider gives same-Context projects distinct handles;
  neither Workspace can read the primary secret or use the other's handle.
  OPA denial makes zero resolve calls, allow makes one, and secret/handle
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
  only the additional shared CA volumes.
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
- empty/oversized credential stdin, provider acquisition mismatch, cancelled
  GitHub login, invalid account/status capture, and cleanup failure while the
  previous credential remains unchanged; terminal import must perform no read,
  public validation precedes a non-terminal read, and runtime prerequisites
  precede broker send;
- corrupted, wrong-Context, or unsupported-version encrypted vaults without
  revealing ciphertext or secret data;
- copied, malformed, ambiguous, stale, rotated, revoked, wrong-Context,
  wrong-project, wrong-provider, wrong-target, wrong-header, wrong-revision, and
  structurally misplaced handles before upstream I/O, with fallback allowed
  only when no Tobari marker appears anywhere inspected;
- OPA denial with zero broker resolve calls and broker timeout/invalid response
  mapped to secret-free `credential_broker_unavailable`;
- an unowned, modified, or symlinked Workspace authentication file before any
  overwrite or removal;
- malformed or legacy state;
- invalid Rego before cluster reconciliation;
- invalid Rego before policy activation and exact OPA-only recreation after a
  valid edit;
- invalid, stale, already-covered, or wrong-kind policy candidate references;
- duplicate, symlinked, group/world-accessible, concurrently changed, or
  malformed managed policy data before write;
- fewer than three, shallow, mixed-host, mixed-method, or stale compaction
  sources;
- failed learned-policy preflight before atomic replacement;
- partial startup mapped to non-retryable `cluster_start_failed`;
- partial root reconciliation mapped to non-retryable `runtime_reconcile_failed`;
- non-empty cluster removal rejection;
- down/purge preservation of encrypted Context vaults and the installation root
  key, with only shared CA volumes added by purge;
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
tobari auth login github --context default
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
the device page through the host (or shows the exact fixed manual URL), and
shows no container browser error, Git credential prompt, Git executable
failure, or persistent-plaintext warning. Status shows exactly one secret-free
account label/revision, `GH_TOKEN` has the
`tobari-h1_` handle shape, and the no-print equality test proves `gh auth token
--hostname github.com` returns that exact projected handle rather than the
primary credential. The allowed API call succeeds only after OPA allow, logout returns
`workspace_reentry_required`, and the prior handle receives
`credential_handle_invalid`. The reviewer also scans Gateway, OPA, and Auth
Broker logs for a private canary supplied only for this disposable test.

Do not save tokens, device codes, handles, root keys, vaults, `gh` config, raw
terminal capture, or authenticated API responses. Manual evidence cannot prove
future provider availability or account authorization; it covers only the
reviewed release candidate and environment.

## Evidence

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
