# Product Contract

## Product statement

Tobari is a local CLI that gives a coding agent an execution boundary in
advance, then lets it act freely inside that boundary. It makes starting an
isolated coding space, understanding a denied operation, and granting the
minimum required permission extremely easy, so the safe execution path is a
more natural choice than running the agent on the host. Isolation is opt-in and
reversible; creating a space is a CWD-local action, and customizing its network
authority is an observe, review, approve, and retry loop rather than a
prerequisite policy-authoring project. Every supported outbound HTTP and HTTPS
request remains enforced through one shared Gateway and OPA policy boundary.
When a Context-owned provider credential is useful, Tobari may project only a
project-bound opaque handle into the Workspace and resolve the real value in a
shared locked Auth Broker after OPA allows the ordinary HTTP effect.

## Primary users and owned outcome

The primary user is a developer who wants an autonomous coding agent to edit a
bounded source tree without receiving a real host-managed credential or
unrestricted network egress. Tool-native authentication may be created inside
the selected Tobari's own persistent home. The supported brokered path instead
acquires one credential on the trusted host for a Context, stores it in an
encrypted vault, and gives each eligible Workspace only a distinct bound
handle; host home and CLI authentication state are never copied in. A separate
narrow-projection boundary may re-encode only thesis-declared non-secret
scalars; it never copies their source file, directive, executable setting, or
authentication material.
The user-facing entry point is the current project directory: a Tobari either
exists or does not exist, and the user should not need to manage container
names, network IDs, or policy internals for routine work. `cluster up` remains
the current explicit owner of shared Gateway and OPA setup; reducing that
first-use bootstrap is an adoption goal, not the reason a user adopts Tobari.

The primary operating loop is progressive policy learning: a Tobari workload is
denied by default, Gateway records the rejected HTTP effect, including one
operation-type/root-field coordinate for a declared GraphQL endpoint, and reason
without secrets, the CLI presents a bounded exact proposal and a concrete
trusted-host next action, the user approves the minimum rule, and the same
workload is retried. A learnable denial also gives the agent a fixed host-side
review command, and the human path enters through `policy review`; machine
discovery remains `policy candidates`. Exact opaque references remain the
safety boundary for `policy allow --id` and `policy deny --id`, while the TTY
may stage several unchanged references and apply the reviewed set once.
`policy rules` is the exhaustive current-decision view; `policy reset --id`
returns one learned Allow or exact Deny to default deny so the retained effect
can be reviewed again. Reset does not authorize or retry the request.
Trusted-host Rego editing remains the advanced path for behavior that exact
learned rules cannot express; ordinary permission growth must not require it.
Exact policy actions and final reviewed-set Apply perform the bounded
activation required for their own mutation.
Denial evidence is a product output, not incidental debug noise.
The host-issued project principal is retained in denial, candidate, learned
rule, and compaction evidence; an approval made from one current-directory
Tobari cannot be replayed as another project's permission.
Ordinary request bodies are not a policy identity dimension. A body-bearing
POST, PUT, PATCH, or other method is authorized and learned from the same
project, host, port, method, and path dimensions as a body-free request.
Changing ordinary body content does not create another review item or rule.
For an exact trusted GraphQL endpoint, Gateway derives only the selected
operation type and canonical root fields from one bounded body; each root is a
separate exact permission. Gateway does not expose body content, body hashes,
GraphQL source, operation names, variables, arguments, aliases, fragments,
directives, nested selections, or literal values to OPA, retained evidence,
policy actions, audit output, or CLI output.
Denial audit retains only the URL path component, never the query or headers.
If that path contains a Tobari handle marker, the whole recorded path is the
literal `/[redacted-auth-handle]`. Structural URL/header handle rejections are
non-learnable and cannot become policy candidates.

## Public vocabulary

- **Tobari:** one long-lived logical untrusted execution environment selected
  by a canonical project root and stable Context identity. Its work container
  is recoverable runtime implementation detail.
- **Workspace:** the human-facing name for one directory-bound Tobari in
  lifecycle and list output. It is not a second runtime resource; its identity
  remains the canonical root and its stable Tobari ID remains diagnostic.
- **cluster:** the one installation-local Gateway, one OPA, one locked Auth
  Broker, one resident host credential-companion lifecycle, aggregate policy
  and provider projections, principal registry,
  Context-scoped managed-credential projection, and CA lifecycle.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **Auth Broker:** the trusted non-root credential-resolution daemon. It owns
  encrypted Context vault access, has no TCP listener, starts locked, and
  exposes separate control and Gateway-only runtime Unix sockets.
- **root:** the canonical host directory selected from the current working
  directory and mounted read-write into one Tobari. A root below the host home
  is mounted at the same relative path below `/var/lib/tobari`; a root outside
  the host home uses the mirrored `/workspace` path.
- **Tobari home:** a per-Tobari persistent owner-only XDG state directory
  mounted as the work user's home.
- **Tobari image:** the minimal built-in runtime or one locally available
  compatible OCI environment image selected by the Tobari's bound Context for Workspace
  creation and later runtime-container reconciliation. Its tools and bootstrap
  are part of the environment; its image `CMD` is not the Workspace lifetime
  command.
- **Tobari ID:** a generated stable internal identity used for state, exact
  resource labels, and host-issued project-principal bindings. It is diagnostic
  output, not a routine user action input.
- **project principal:** a host-issued binding from one stable Tobari ID and
  stable Context ID to the exact owned Workspace source endpoint and Gateway
  endpoint on that project's dedicated network. Caller headers, Context names,
  SNI, request authority, and profile names are not principals.
- **tool-owned authentication state:** files written by a tool or agent below
  one Tobari's persistent home during its own login or configuration flow.
- **credential provider:** the external service or authority whose credential
  is acquired or imported, stored, and later applied to one exact reviewed
  request binding. A provider is not the Workspace client that uses it.
- **Workspace client tool:** the CLI or other client inside a Workspace that
  receives provider-declared handle projection and emits the authenticated
  request shape. `gh`, `aws`, `pup`, `codex`, `claude`, and `cwk` are current
  client-tool examples; their names do not grant provider or network authority.
- **supported Provider-Tool pairing:** one reviewed combination of credential
  provider, Workspace client tool, acquisition mode, handle projection, and
  exact request binding. The current built-in pairings each support one client
  tool, so `auth login` asks for the provider and displays the sole tool as
  automatically selected rather than requiring ceremonial second input. AWS
  `identity-center` and `console` are acquisition methods within the AWS/`aws`
  pairing, not separate tools. A future second tool for one provider requires
  an explicit public selection and compatibility decision before it is
  exposed.
- **brokered credential:** one typed credential record owned by a stable Context
  and provider. A static provider stores one opaque primary secret acquired
  through protected non-terminal stdin or the reviewed host GitHub driver. The
  AWS provider stores encrypted opaque AWS CLI state acquired through one
  explicitly selected reviewed host flow; derived credentials are transient and are
  never persisted.
  The Datadog provider stores strict encrypted pup OAuth client/token state
  acquired through the fixed US1 host flow; only a post-policy access token is
  request-local and near-expiry state refreshes automatically.
  The OpenAI provider stores strict encrypted Codex 0.146.0 ChatGPT OAuth state;
  only a post-policy access token and account-routing header are request-local,
  and near-expiry state refreshes automatically. The Anthropic provider stores
  one Claude Code 2.1.220 inference-only setup token with no refresh state.
- **credential companion:** one resident trusted-host process entered through
  the current Tobari executable's private same-binary mode. It accepts only the
  reviewed post-policy AWS refresh operation and exchanges bounded
  authenticated frames with an unmounted Broker-private socket over a reverse
  `docker exec` stream. Interactive GitHub/AWS/Datadog/OpenAI/Anthropic login
  runs through direct context-bound host drivers, not companion RPC. The
  companion has no public
  command, host listener, provider-selected executable, or Workspace-visible
  secret surface.
- **Workspace credential handle:** a versioned random opaque value bound to one
  Context, project, provider, credential revision, and exact HTTP binding. It
  is not the real credential and is not authority without the trusted
  principal and OPA allow. Broker metadata never inherits a broad static
  host/method allow; the first exact L7 effect remains reviewable until an
  exact learned rule exists.
- **provider manifest:** strict non-secret data declaring acquisition,
  Workspace handle projections, and exact credential bindings. Owner manifests
  are V1 static-secret/header plans. Reviewed built-ins use typed closed plans
  within the same V1 authority; neither form
  declares no executable shell, arbitrary route, HTTP method/path policy, or
  provider operation semantics.
- **credential profile:** non-secret Gateway configuration for the static
  managed adapter; it binds a Context-scoped profile name to exact hosts and
  project principals.
- **Context:** one host-owned logical execution setup with a stable opaque ID
  and a human name. Its manifest
  references an agent profile, a compatible Tobari runtime image, a policy
  store, and managed-credential stores. Its stable ID determines separately
  stored Context-owned Auth Broker vault state; the manifest does not contain a
  broker vault path, key, or secret. Those stores remain physically separated
  by trust boundary.
- **current Context:** only the default Context used when an invocation omits
  an explicit Context; it is not shared enforcement authority.
- **synthetic default:** the display-only `default` selection returned by an
  omitted-Context read before any Context is persisted. It has no stable ID or
  store authority and cannot bind a mutation; explicitly naming an absent
  `default` Context returns not found.
- **Context runtime recipe:** the selected Context's owner-only
  `runtime/Dockerfile`. It is the source for an explicit host-side custom
  runtime build; it is not project metadata and it is never mounted into a
  Workspace.
- **agent profile:** read-only non-secret shared agent configuration referenced
  by a Context. It is not tool-owned login state.
- **narrow projection:** one fixed Context-owned allowlist of validated
  non-secret scalar fallbacks. The source file, path, directives, executable
  settings, credentials, and undeclared keys never enter the Workspace.

Stable Tobari and Context IDs are not trusted when supplied by a work
container. The host-owned principal registry derives both from the exact
kernel-observed Workspace source endpoint within its verified dedicated
network binding. The registry also retains the exact Gateway endpoint and
rejects duplicate or stale endpoints. Context policy is selected inside the
single OPA from that trusted principal. Learned permissions are Context- and
project-bound. Principal identity alone does not select or inject tool
credentials. Brokered resolution additionally requires an exact provider,
credential revision, target, and source-header binding; Context, project, and
the allowed decision must all match.

Every Workspace uses a guarded default route and non-recursive synthetic DNS,
so an ordinary HTTP/HTTPS client reaches Gateway transparently without proxy
environment variables. Gateway exposes no regular explicit-proxy listener.
The transparent path produces the normalized policy, credential, audit,
resolution, and upstream behavior. Gateway performs
no external DNS or upstream connection before allow, and kernel forwarding is
never a fallback. Raw TCP, non-HTTP TLS, UDP, and QUIC are rejected rather than
forwarded.

The public commands are:

| Command | Role | Effect | Outcome |
|---|---|---|---|
| `help [selector] [--format text|agent]` | utility | read | Discover exact command contracts |
| `version [--format text|json]` | utility | read | Print source version/commit, resolver channel, required and selected Gateway/Auth Broker APIs, and compatibility |
| `doctor [--root PATH] [--format text|tsv|json]` | utility | read | Report read-only host, Docker, configuration, policy, provider-manifest, root-key/vault, broker/project-handle, managed-secret, port, and residue diagnostics without repairing state |
| `cluster up` | act, fixed target | create | Validate all Context policy/provider inputs and image contracts, reconcile Gateway, OPA, and Auth Broker, unlock the broker, and start its resident host credential companion |
| `tobari [--context NAME]` | act, fixed target | create | Choose or create the current directory's Workspace in the explicit or current Context, reconcile runtime, enter it, and leave it reusable after `exit` |
| `status [--context NAME] [--format text|json]` | utility | read | Inspect the nearest current-directory Workspace in the explicit or current Context, its logical existence, runtime diagnostic, and attached/detached session observation |
| `list [--format text|json]` | utility | read | List local Workspaces with Context, runtime diagnostics, and diagnostic IDs |
| `delete [--context NAME] [--force]` | act, fixed target | write | Delete the nearest current-directory Workspace in the explicit or current Context, its owned runtime, persistent home, and tool-owned authentication state while preserving project files; `--force` overrides only the attached-session guard |
| `cluster status [--format text|json]` | utility | read | Inspect three-service and companion health, loaded Context count, aggregate revision, policy/provider projection integrity, root-key backend, and recent errors |
| `cluster denials [--tail N] [--format text|json]` | utility | read | Read a bounded typed denial window, exact-rule learnability, policy path, and review command |
| `cluster logs [--component gateway|opa|auth-broker|all] [--tail N]` | utility | read | Read bounded shared logs, including policy-denial evidence, without credential or handle output |
| `cluster down [--purge]` | act, fixed target | write | Remove shared transient resources after every logical Tobari is deleted while preserving Auth Broker vaults and the installation root key; `--purge` additionally removes shared CA and active policy-bundle volumes |
| `policy candidates [--tail N] [--format text|json]` | discover | read | Discover Context/project-scoped pending exact HTTP or GraphQL-root candidates and opaque IDs across the installation |
| `policy review [--tail N] [--format text|json]` | discover plus TTY fixed-target apply | read, or one confirmed write | Review the installation-wide Permission Inbox; on a TTY, stage exact allow or deny choices from detail and apply the reviewed set once; redirected and JSON output remain read-only |
| `policy apply-reviewed` | act, fixed target | write | Catalog-owned completion action for a non-empty typed set staged inside an interactive Permission Inbox; direct invocation is rejected |
| `policy allow --id ID` | act, reference bound | write | Test, record, and activate one exact observed permission |
| `policy deny --id ID` | act, reference bound | write | Test, record, and activate one exact project-bound rejection |
| `policy rules [--format text|json]` | discover | read | List every Context-scoped CLI-owned learned Allow and exact Deny decision; on a TTY, reset one explicitly |
| `policy reset --id ID` | act, reference bound | write | Remove one learned decision and leave its effect at default deny |
| `policy compactions [--format text|json]` | discover | read | Discover safe bounded prefix-compaction candidates and opaque IDs |
| `policy compact --id ID` | act, reference bound | write | Test and activate one current learned-rule compaction |
| `context list [--format text|json]` | utility | read | List persisted named Contexts and report the current selection as persisted or a display-only synthetic default |
| `context show [--name NAME] [--format text|json]` | utility | read | Inspect one Context's explicit persistence state, agent, policy, managed-adapter store references, and secret-free Auth Broker/provider state without returning a broker vault path/content, key, primary secret, or handle |
| `config shell [--variable COLORTERM\|NO_COLOR\|PS1\|TERM] [--source default\|inherit\|literal] [--value VALUE] [--context NAME] [--format text\|json]` | act, fixed target | write | Configure one allowlisted shell-presentation variable directly, or stage one or more rows from the complete terminal inventory and apply them atomically |
| `config git [--source default\|inherit\|literal] [--name NAME] [--email EMAIL] [--context NAME] [--format text\|json]` | act, fixed target | write | Configure one atomic Context Git commit-identity fallback directly, or stage and apply its source from one terminal screen |
| `context create --name NAME [--image IMAGE] [--mode guided|advanced]` | act, fixed target | create | Create one named Context with a runtime image and separate owner-only stores |
| `context use --name NAME` | act, fixed target | write | Change only the current/default Context; do not mutate existing Tobari or start/reconcile the cluster |
| `runtime init [--format text|json]` | act, fixed target | create | Create the current Context's runtime/Dockerfile template without changing its selected image |
| `runtime build [--format text|json]` | act, fixed target | write | Build, validate, and select the current Context's generated local runtime image |
| `auth login [--provider PROVIDER] [--method identity-center\|console] [--context NAME] [--format text\|json]` | act, fixed target | write | Acquire one supported provider credential through a reviewed interactive trusted-host CLI driver for the explicit or current Context; provider-option omission opens a terminal selector over installed reviewed login providers, each current built-in displays its sole supported Workspace client tool as automatically selected, AWS method omission preserves the fixed IAM Identity Center device flow, `console` selects fixed cross-device AWS CLI local-development login, Datadog selects fixed default-organization US1 pup OAuth, OpenAI selects pinned Codex ChatGPT device OAuth, and Anthropic selects pinned Claude inference setup-token OAuth |
| `auth import PROVIDER [--context NAME] [--format text|json]` | act, fixed target | write | Import one bounded opaque provider credential only from protected non-terminal stdin |
| `auth status [--context NAME] [--format text|json]` | utility | read | Inspect the complete installed provider collection plus bounded Context-scoped Workspace projection freshness and coverage without reading secrets |
| `auth logout PROVIDER [--context NAME] [--format text|json]` | act, fixed target | write | Remove one local Context/provider credential and revoke its Workspace handles when present, or report confirmed `no_change` when already absent, without contacting the provider |

For the CWD lifecycle commands `tobari`, `status`, and `delete`, one non-empty
invocation Context may appear before or after the command path: for example,
`tobari --context toolbox status` and `tobari status --context toolbox` are
equivalent. Omission resolves the current Context without changing it;
duplicate or explicit-empty placement is invalid. After name resolution, the
stable Context ID is authoritative for the remainder of the operation.

`cluster status --format json` uses output schema 1. Its `cluster` object keeps
the three shared container components and adds always-present secret-free
`credential_companion_state`, exactly `ready`, `prepared`, `absent`, or
`unavailable`. The field reports the resident host process/channel
relationship; it is not a fourth Compose
service, provider credential state, or permission fact. `absent` means no
prepared epoch or active session, `prepared` means Broker accepted the epoch
and is waiting for the authenticated channel, `ready` means that channel is
active, and `unavailable` means the Broker control observation failed.

The root command is interactive and requires a TTY on stdin, stdout, and stderr.
It does not silently create state in a non-interactive context. When the
canonical current directory is below one or more indexed Workspace roots, the
command presents an English selector ordered nearest-first. Arrow keys and
Enter choose an existing Workspace; `n` chooses explicit creation at the
current directory; `q` or Escape cancels. If raw terminal mode is unavailable,
the same choices use numbered line input without adding a terminal module or
shell subprocess. Candidate status and path text remain meaningful without
color. Programs inside Tobari can mutate the explicitly mounted root; that
delegated capability is a documented security property rather than an
undeclared Docker mutation by the CLI.

## Input and path contract

- The current working directory is expanded and canonicalized on the host
  before state or Docker calls. An exact indexed root is reused directly. When
  only containing ancestor roots exist, `tobari` lists every valid root
  nearest-first and offers explicit creation at the current directory; it never
  creates a nested Tobari implicitly.
- `doctor` validates the current directory as the prospective project root
  when `--root` is omitted. `--root PATH` exists only for diagnosing another
  host directory without changing the shell's current directory. The command
  reports its fixed complete check set in catalog order. A dependent that was
  not observed is `blocked` by one named direct prerequisite rather than
  reported as a speculative failure. Each observed failure owns a concrete
  correction and exact next Tobari command; blocked rows do not duplicate that
  recovery. The command returns `diagnostic_failed` when any check fails;
  warnings and blocked dependents alone do not add another failure. It remains
  read-only: it does not
  initialize policy, start/reconcile or unlock the shared cluster, create or
  replace the root key, or mutate provider, vault, credential, handle, or
  project-auth state.
- Each `(canonical root, stable Context ID)` identifies at most one Workspace.
  Repeated or concurrent creation for that pair is rejected as already
  existing; the same root may have independent Workspaces in different
  Contexts. An explicit Context never changes the current Context.
- Project-root selection rejects the filesystem root, the user's home and its
  ancestors, and any path overlapping XDG Tobari configuration, state, or
  shared-profile management directories, Docker sockets, or Docker management
  paths. A repository containing policy source remains allowed; only the
  trusted active policy/configuration paths are protected.
- An explicit create choice uses the canonical current directory as root.
  Project moves and copies are not inferred or recorded in the project tree.
- The selected root is the only project directory mounted read-write. When it
  is below the host home, the container target preserves the relative path
  below `HOME=/var/lib/tobari`; for example, a host root of
  `$HOME/path/to` enters at `/var/lib/tobari/path/to`. A root outside
  the host home retains `/workspace/<canonical-root-without-leading-slash>`;
  thus a root at `/work` and a host CWD of `/work/root` enter at
  `/workspace/work/root`. Tobari never mounts the host home wholesale.
- Same-root Tobari in different Contexts and parent/child roots may run
  concurrently. Runtime, home, network, policy, and managed credentials follow
  their Tobari/Context boundaries, but overlapping host-file writes are visible
  to every mount of those files. Tobari provides no overlay, checkout clone,
  root lock, session exclusion, warning gate, or filesystem integrity isolation
  for this user-selected sharing.
- The configured image accepts `builtin` or a portable OCI image reference.
  In normal builds, `builtin` resolves to the published official runtime base.
  A custom image must already exist locally and preserve runtime API `1`, the
  `tobari` image user, the `io.tobari.runtime-lifetime-command` capability, and
  the Tobari entrypoint. That capability is currently `sleep infinity`, which
  is required by Tobari's fixed Workspace lifetime command. Ordinary `tobari`
  startup never pulls a configured image implicitly. `cluster up` may obtain
  the official runtime base for an uncustomized Context; custom images still
  fail closed if missing or incompatible before project runtime network or
  container mutation.
- `cluster up` obtains the embedded immutable Gateway digest when it is not
  already available locally and does the same for the immutable Auth Broker
  digest. It validates each digest, API/role labels, non-root default user,
  entrypoint, and Docker Engine platform before running policy tests or
  creating shared networks and containers. Gateway and Auth Broker source
  development use the contributor-only `task build:dev` path and a
  `tobari_dev` binary, not a public `cluster up` option.
  Both official images are published as Linux amd64/arm64 OCI indexes and the
  checked runtime metadata contains their reviewed immutable manifest digests.
  Moving `main` and `latest` tags never become runtime authority. Contributor
  validation uses `tobari-gateway:dev` and `tobari-auth-broker:dev` through
  `task build:dev` without changing the normal binary's selectors.
- Authentication commands accept only an existing Context name and installed
  provider ID. `auth login` accepts the provider through optional
  `--provider`: when omitted, interactive terminal stdin and stderr present only
  the installed reviewed built-in `github`, `aws`, `datadog`, `openai`, and
  `anthropic` login
  providers for the selected Context and require one explicit choice. A
  supplied provider skips that status read and menu. The omitted-provider flow
  binds login to the Context returned by the status snapshot, so a concurrent
  current/default change cannot retarget the mutation. `--method` requires an
  explicitly supplied `--provider`, remains AWS-only, and omission for explicit
  AWS login means `identity-center`. GitHub shows its device code and the trusted
  host opens exactly `https://github.com/login/device` when possible. AWS
  requires explicit `identity-center` or `console` selection, with omission
  meaning `identity-center`. Identity Center asks for one validated start URL,
  SSO region, 12-digit account ID, and role name, then leaves the validated
  regional device URL and one-time code. Console mode requires AWS CLI 2.32 or
  newer, asks for one commercial AWS region, runs fixed `aws login --remote`,
  opens only the exact region-bound AWS authorization URL when possible, and
  leaves that same URL for manual cross-device completion. Datadog uses fixed
  host pup OAuth for US1, with pup-owned browser consent and loopback callback
  in an isolated file-backed home. OpenAI uses pinned Codex 0.146.0 native
  ChatGPT device login with fixed `login --device-auth` argv plus only the
  reviewed file-credential-store and no-update configuration overrides in an
  isolated file-backed home. Anthropic uses pinned Claude Code 2.1.220 by
  running exactly `claude setup-token` in an isolated private home and PTY,
  capturing one inference token without printing it. The corresponding
  canonical host executable must report exactly `codex-cli 0.146.0` or
  `2.1.220 (Claude Code)`, remain non-group/world-writable, and resolve from a
  conventional non-project trusted installation root. A Workspace binary,
  project `PATH`, ambient provider home, or newer version is not a login
  fallback. Auth Broker
  configures no Git or AWS CLI state in a Workspace and contains no provider
  CLI executable. `auth import` accepts a non-empty
  credential of at most
  32 KiB from non-terminal stdin only; terminal stdin is rejected before any
  byte is read. Non-terminal bytes are read only after public Context/provider
  argument, intent, and mutation validation; infrastructure then validates the
  selected existing Context, installed provider/acquisition mode, and broker
  readiness before broker send. The credential is never a positional/flag
  value or Tobari environment input. Every successful auth mutation requires
  existing Workspaces to be re-entered before their environment or
  complete-file handle projection can change.
- OpenAI projects one Tobari-owned complete `.codex/auth.json` whose exact
  external-host `chatgptAuthTokens` compatibility shape contains fixed sentinel
  values and the project-bound handle only as `tokens.access_token`:

  ```json
  {"auth_mode":"chatgptAuthTokens","OPENAI_API_KEY":null,"tokens":{"id_token":"e30.e30.x","access_token":"${HANDLE}","refresh_token":"","account_id":null},"last_refresh":"1970-01-01T00:00:00Z"}
  ```

  This is a deliberately unstable, version-pinned Codex 0.146.0 shim, not an
  OpenAI credential file or an upstream compatibility promise. It binds only
  HTTPS `chatgpt.com:443` with bearer input. Gateway removes caller-supplied
  `Authorization`, `ChatGPT-Account-ID`, and `X-OpenAI-FedRAMP` before OPA;
  after allow, Broker returns the same-revision access token and validated
  non-secret account ID, and Gateway alone injects `Authorization: Bearer` and
  `ChatGPT-Account-ID` for one upstream attempt. FedRAMP and alternate
  authorities are rejected. Workspace Codex never receives or refreshes the
  provider session.
- Anthropic projects only
  `CLAUDE_CODE_OAUTH_TOKEN=${HANDLE}`. It does not project
  `ANTHROPIC_API_KEY`, `ANTHROPIC_AUTH_TOKEN`, the setup token, or Claude's
  private login store. Its one exact binding is bearer HTTPS
  `api.anthropic.com:443`; Broker resolves the static inference token only after
  allow and never refreshes it. The supported outcome is first-party inference
  and local MCP. Remote Control and claude.ai connectors remain unsupported.
- `runtime init` creates the current Context's owner-only
  `runtime/Dockerfile`. The template starts from
  `ghcr.io/tasuku43/tobari/runtime:latest`; editing that file is the supported
  place to add tools and environment configuration for the Context. The
  command does not overwrite an existing recipe.
- Direct `config shell` changes one allowlisted shell-presentation policy in
  the explicit or current Context. Its terminal editor may stage several
  distinct rows and commits the complete change set with one atomic write.
  `default` removes its override;
  `inherit` reads that exported variable from the host process launching each
  future `tobari` session; `literal` requires `--value` and preserves an
  explicit empty value. `--value` is invalid for the other sources. The fixed
  inventory is `PS1`, `TERM`, `COLORTERM`, and `NO_COLOR`; it excludes
  `PATH`, `HOME`, `BASH_ENV`, `ENV`, `PROMPT_COMMAND`, credential variables,
  and arbitrary names. A new V1 Context selects `PS1=inherit`. When the
  launcher has no exported `PS1`, the built-in `\h:\w\$ `
  prompt remains.
  Running sessions are unchanged, and no host startup file is sourced or
  mounted. Literal values are ordinary owner-only configuration and must not
  contain secrets.
- `config git` changes one atomic `user.name`/`user.email` fallback in the
  explicit or current Context. `default` removes Tobari's fallback; `inherit`
  resolves only a complete pair from host-global Git configuration for each
  stable Workspace root during reconciliation; `literal` requires both
  non-empty `--name` and `--email`. New Contexts use `default`,
  and an absent or incomplete inherited pair adds no fallback. The projected
  system-scope value is lower precedence than Workspace global and
  repository/worktree configuration. No Git file/path/include directive,
  credential helper, token, HTTP header, SSH command, signing setting, hook,
  alias, URL rewrite, filter, proxy, or arbitrary key is projected.
- For both `config` commands, omitting the entire setting group in text mode
  uses terminal stdin and stderr to show the selected Context, complete current
  state, pending changes, and Apply/Cancel controls on one screen. Shell stages
  multiple rows; Git stages one atomic identity source. Text mode requires both the
  success and error formats to be text. Supplying any setting input selects
  direct mode and requires a complete valid group; partial input never prompts.
  Redirected or JSON invocations require direct mode. An explicit empty
  Context is invalid rather than equivalent to omission. After reading current
  state, the wizard binds Apply to that returned Context name, so another
  process changing the current/default marker during review cannot retarget
  the write. Cancellation and every wizard validation or terminal failure
  perform zero mutation.
  Prompts use stderr and the confirmed complete Context report uses stdout.
- `runtime build` is the explicit exception to the no-implicit-pull rule. It
  runs a host Docker build using only the Context runtime directory as build
  context; Docker may obtain a missing base image for this explicit build.
  Tobari requests plain BuildKit progress and forwards the visible-projected
  Docker stdout and stderr diagnostic stream to host stderr while the build
  runs, including in non-TTY environments. The diagnostic stream retains the
  concrete failed step and Docker/BuildKit error separately from Tobari's
  stable structured mutation fault; a user does not need to rerun an equivalent
  Docker command to obtain the upstream failure.
  Tobari validates the resulting image against the same runtime contract,
  records its immutable local image digest, and then selects the generated
  `tobari-context-<context>:<source>` image in the current Context. The previous
  selected image remains in force until promotion succeeds.
- The built-in `tobari/runtime` image is the base work runtime: it preserves the
  lifecycle contract and its existing common-tool baseline includes Git, HTTP,
  JSON, Python, SSH, GitHub CLI, and AWS CLI. `kubectl`, `cwk`, `pup`, and TWG
  belong to the optional locally built Context toolbox; none is added to the
  published base by this capability. The base is
  published on reviewed main pushes as the moving
  `latest` and `main` development channels; registry publication is not
  implied by local image selection, and Tobari never pulls the published image
  implicitly during ordinary startup.
- Agent-image recipes are complete compatible variants in the same runtime
  family. The current local recipes pin Claude Code 2.1.220 and Codex 0.146.0;
  a Context-owned custom recipe may compose those reviewed local artifacts so
  both version commands work in its Workspace. They add the agent tools and
  only their agent-specific dependencies; they do not create a second
  authority boundary. No Claude or Codex agent tag is published until the
  corresponding redistribution and license review is complete.
- Project metadata does not select or alter the runtime image. Workspaces use
  their permanently bound Context image when created and again when their runtime container
  is reconciled by root entry; all selected images still pass the same
  compatibility checks before Docker mutation. If an existing Workspace's
  bound Context image changes, the next matching-Context `tobari` entry preserves the
  Workspace home and root record, recreates only the work container when the
  runtime spec changed, and records the new image only after reconciliation
  succeeds.
- Shared cluster mutations use one command-bound `tool_local` target and are
  never performed by the root `tobari` operation.
- CWD-local lifecycle operations use one command-bound `tool_local` current
  directory target plus the explicit or current Context selector; they do not
  accept an ID or root selector. Context selection is resolved to a stable ID
  before CWD/Workspace or Docker observation and never guesses a different
  same-root Workspace. A force-delete preview binds the subsequent mutation to
  the stable Context ID it displayed and fails closed if that authority changes.
- `delete` removes that nearest Context-bound target without `--force` when no session is
  attached. An attached session returns `project_session_attached` and leaves
  state untouched; `--force` is the explicit override.

## Output and exit contract

Human output is concise text. The canonical public machine-output inventory is:

<!-- public-cli-json-schemas:start -->

| Surface | Envelope | Schema |
| --- | --- | ---: |
| Structured error | `error` | 1 |
| Agent help (`view: index` and input-selected `view: scope`) | `commands` | 1 |
| Version | `build_identity` | 1 |
| Doctor report | `report` | 1 |
| Context list | `contexts` | 1 |
| Context report (show/create/use/config/runtime results) | `context` | 1 |
| Cluster status | `cluster` | 1 |
| Cluster denials | `denials` | 1 |
| Policy candidates | `policy_candidates` | 1 |
| Policy review | `policy_review` | 1 |
| Policy rules | `policy_rules` | 1 |
| Policy compactions | `policy_compactions` | 1 |
| Authentication result and status | `auth` | 1 |
| Workspace list | `tobari` | 1 |
| Workspace status | `status` | 1 |
<!-- public-cli-json-schemas:end -->

Workspace status always reports the selected Context ID/name, logical
existence, runtime diagnostic, attachment observation, and exact Context-bound
next argv. Context reports include a complete four-item shell-environment
inventory, complete Git identity policy, and explicit secret-free Auth
Broker/provider state. Every auth result uses envelope `auth`. Public
authentication backend values are
`macos_keychain` or `xdg_file`; cluster status may additionally report
`unavailable`. Unconfigured cluster resources are `null`; unavailable
observations use declared finite values and never an empty-string sentinel.
The infrastructure/doctor label `linux_xdg_file` is not a public
auth or cluster JSON enum. Their items associate Context name,
stable Context ID, Tobari ID, safe project root, HTTP effect, observation data
where applicable, and one opaque mutation reference. Agent help uses the V1
catalog schema, including recursive field declarations and executable
success/error invocation forms in scoped help; it also keeps internal
interactive completion commands outside public discovery and help.
Successful data is stdout;
failures are stderr.

`version --format json` uses schema version 1 with envelope
`build_identity`. Its fixed fields are `version`, `commit`,
`resolver_channel`, `development_source`, required and selected Gateway/Auth
Broker APIs, `compatible`, `development_build_command`, and
`development_binary`. An absent source commit is the explicit string
`unknown` and makes `compatible=false`. The two repository-command fields are
empty for a published resolver and contain exactly `task build:dev` and
`bin/tobari-dev` only when the compiled development metadata proves that path.
The Workspace selector is a human stderr interaction; it produces no JSON or
stdout selection protocol. A successful choice prints an English summary before
the child session, and cancellation or stale selection prints no success
summary. The interactive child owns stdout. When it returns, the host command
writes this lifecycle guidance to stderr:

```text
Workspace session closed.
Workspace remains available.

Resume: tobari
Remove: tobari delete
If another session is attached: tobari delete --force
  ```

`exit` therefore detaches the session without deleting the Workspace. The
Workspace remains existing until the host runs `tobari delete`, which is the
normal lifecycle-ending operation when no session is attached. If another
session is attached, ordinary delete fails with a warning and `--force` is the
explicit override. There is no public `stop` or `pause` state. The choice is
revalidated under the lifecycle lock before logical or Docker mutation, so a
changed candidate set fails closed and asks the user to run `tobari` again.
When a learnable network request is denied, the Gateway's 403 response carries
fixed secret-free host-review navigation for the agent, and an interactive
session close may summarize the pending queue on host stderr. These are
advisory only; the interactive `policy review` queue is the human entry point.
It stages unchanged opaque references only from exact detail screens and uses
one final `policy apply-reviewed` fixed-target action to revalidate and activate
one Context's set. Apply or discard is required before switching Context so the
source change remains one atomic file replacement. `policy rules` is the
current learned-decision inventory; its TTY reset flow delegates one explicit
opaque reference to `policy reset`. Redirected and machine-readable review and
inventory remain read-only. The Permission Inbox groups candidates by their
validated stable Context and project identities, renders the Context/root scope
once per group, and leads each selectable row with the exact HTTP effect and
observation count. A compact selected-effect preview exposes the latest retained
observation before detail inspection. Matching display names, paths, order, or
indentation do not merge distinct typed identities. Allow-exact and Deny-exact
keys are inactive on the list; on the exact detail screen the chosen action is
explicitly staged without a second yes/no prompt. Only the final Apply delegates
the reviewed set to the mutation boundary. Apply is advertised only for a
non-empty staged set, shows one final ordered exact review, and requires an
explicit confirmation. Refresh preserves choices by candidate ID and drops
stale IDs rather than matching labels. Confirmed output carries the active OPA
revision plus each ordered exact Context/project/effect/candidate decision and
directs the caller to retry in the current running Workspace. The public
read-only JSON review schema remains version 1 and does not expose this
internal TTY Apply receipt.
Human `text` output uses one shared presentation vocabulary across lifecycle,
policy, diagnostics, help, version, and error views: an outcome-first heading,
a small state marker, aligned detail rows, semantic style tokens, and an exact
next action when the result has a useful recovery or continuation. The complete
token vocabulary is `text`, `muted`, `accent`, `success`, `warning`, and
`danger`. Ordinary values and explanations use `text`; readable secondary
labels and auxiliary detail use `muted`; only primary headings and the next
operation use `accent`. The `success`, `warning`, and `danger` tokens are
reserved for state. A path or opaque ID is not accented merely because it is a
path or ID. Concrete colors and emphasis belong only to the shared
presentation layer, never to an individual command renderer.

Terminal styling is applied only when the corresponding output stream is an
interactive terminal and `NO_COLOR` is absent. Redirected text and any
invocation with `NO_COLOR` present contain no ANSI style sequences and preserve
the exact same headings, markers, field order, scoped empty state, bounds, and
Next guidance. `NO_COLOR` selects no alternate terse or tabular renderer.
Markers, words, and layout carry the same status meaning without color.
`doctor` defaults to this human text
view; `doctor --format tsv` remains the tab-separated projection for scripts,
and JSON/agent help remain schema contracts. Doctor JSON schema 1 declares
`check`, `status`, `detail`, nullable `blocked_by`, and nullable `recovery`;
recovery contains required `action` and `next_command`. TSV flattens those same
facts without inferring recovery from labels or row order.
Empty collections are explicit rather than silent. Opaque IDs remain byte-for-
byte exact, while external evidence remains subject to the existing safe text
projection before it is displayed. Root, namespace, and exact human help use
the same hierarchy; `--format agent` is machine-readable JSON and never receives
terminal styling.

A bare canonical namespace such as `policy` is shorthand for its existing
catalog-derived namespace help. Unknown commands may show at most three stable
edit-distance suggestions derived only from exact catalog paths and canonical
namespaces; recovery remains one exact catalog help selector and never appends
unchecked argv. A deliberate pre-action cancellation is a neutral `Canceled`
outcome on stderr with exit 11 and no success or danger treatment. It performs
no mutation; a confirmed mutation keeps its existing reconciliation contract.
Raw selectors render once initially and only after an input or selection-state
change, not for an idle terminal-read poll, and restore terminal state exactly
once when they finish.
When `cluster up` runs with an interactive stderr terminal, it may also render
bounded fixed-step startup progress on stderr. The progress uses terminal
control sequences and color only for that terminal presentation; it carries no
runtime diagnostics and is absent for non-interactive or machine-readable
callers. The completed checklist remains visible, and the final human summary
is the same summary rendered by `cluster status`; it remains the only
successful data written to stdout. JSON output is unchanged and contains no
terminal control sequences. The checklist presents the internal startup work
as three user-facing phases: `prepare environment`, `start services`, and
`verify readiness`. In a terminal, semantic colors distinguish active,
healthy, warning, failed, and secondary information; labels and values remain
otherwise plain. The ready summary prioritizes outcome, component health,
attached Tobari count, and policy path; configured/running booleans and the
full recent diagnostic remain available in JSON or failure detail. A
successful `cluster up` additionally points to the next `tobari`
command.

`context use` reports `default_updated`; it changes only the omitted-Context
default regardless of cluster state. `context create` reports
`requires_reconcile` when a configured cluster does not yet contain the new
Context projection; an explicit `cluster up` is required. Neither command
starts Docker.

Project runtime diagnostics may report `incomplete` when a durable root index
survives without its instance state. This preserves logical existence for safe
cleanup, prevents runtime recreation, and directs the user to delete the exact
current-directory Tobari before creating it again.

The lifecycle state model has two dimensions:

```text
Workspace absent
  -> tobari
Attached session + Workspace exists
  -> exit
Detached session + Workspace exists
  -> tobari
Attached session + Workspace exists
  -> exit
Detached session + Workspace exists
  -> tobari delete (from the host)
Workspace absent
```

`status` and `delete` resolve one stable Context before the nearest canonical
Workspace containing the host current directory. Status distinguishes logical
absence from an existing Workspace whose runtime is missing, and reports
`attached`, `detached`, or `not_applicable` directly rather than inferring it
from labels or presentation order. When several ancestor Workspaces exist,
run the destructive command from a directory whose nearest Workspace is the
one intended for removal. If that Workspace has an attached session, add
`--force` only when terminating that session and removing its persistent home
and tool-owned authentication state is intentional; the mounted project root
and its files remain outside deletion.

| Exit | Meaning |
|---:|---|
| 0 | Success |
| 2 | Invalid command or input |
| 3 | Internal or Docker execution failure |
| 9 | Required runtime temporarily unavailable |
| 10 | Policy or diagnostic rejection |
| 11 | Caller cancellation |
| 13 | Declared contract violation |
| other from root entry | Exact child process exit status when Docker started the interactive work process |

Commands use complete delivery. `list` is exhaustive for local logical state at
one observation point. `status` is a CWD-local scalar observation; cluster
status is exhaustive for the shared cluster scope. Logs are
a bounded recent window of 1 through 10,000 lines per selected component.
Denials are a fully delivered typed projection from the requested bounded
Gateway-line window; an empty `items` collection means no valid denial occurred
in that window, not exhaustive history.
Policy candidates, review, and tail are bounded by the same retained
Gateway-line window and omit effects already covered by learned allow rules,
baseline deny rules, or exact learned deny rules. Baseline and exact denies
remain available as audit evidence but never become pending queue items.
Within that window, candidates aggregate by exact Context identity, project
principal, host, port, method, normalized path, and optional GraphQL operation
type/root field. Reason, status, request ID, timestamp, and
credential-profile evidence do not create a second permission identity. The
candidate retains the latest matching evidence and reports the required number
of matching retained observations. Concurrent identical audit records therefore project to one pending
item without a separate mutable inbox write. A current learned Allow or exact
Deny remains the resolved history and is never updated by discovery. After an
explicit reset, retained matching evidence may produce the same stable pending
candidate ID again.
Compactions are exhaustive for the current validated learned-rule file at one
observation.

## Configuration contract

Configuration is resolved from
`${XDG_CONFIG_HOME:-$HOME/.config}/tobari` on both macOS and Linux:

A declared read observes these paths without creating them. On a fresh
installation, omitted Context selection may return `synthetic_default` with
JSON `null` for Context ID and stores; it does not create `contexts/active.json`
or a manifest. Persisted state must use the exact current V1 contract. Missing,
malformed, or unsupported-version state fails closed instead of becoming
synthetic state.

- `contexts/<name>/context.json`: host-owned schema-v1 Context manifest with a
  stable UUIDv7 Context ID, the named agent profile, compatible Tobari runtime
  image selector, guided/advanced policy mode, allowlisted shell-environment
  overrides, and an optional non-default Git identity policy;
- `contexts/<name>/runtime/Dockerfile`: optional owner-only Context runtime
  recipe created by `runtime init`; its source digest and last successful
  managed image build are recorded additively in `context.json`;
- `contexts/<name>/policy/data.json`: authoritative HTTP and exact GraphQL
  endpoint boundary, baseline, and
  learned allow/deny data for that Context; Guided Contexts own no Rego files,
  while Advanced Contexts additionally own `tobari.rego` and
  `tobari_test.rego`; the directory is never mounted directly as the shared
  OPA's complete policy;
- `contexts/<name>/credentials.json`: reserved schema-v1 profile metadata for
  the explicitly selected managed Gateway adapter;
- `contexts/<name>/credentials/`: reserved managed-adapter secret files for
  that Context, never mounted into a Workspace;
- `auth/providers/*.json`: optional owner-only schema-v1 provider manifests;
  user manifests cannot replace built-ins and may use protected non-terminal stdin import
  only;
- `contexts/active.json`: owner-only current/default Context
  selection; missing means `default` and the marker has no enforcement authority;
- `principal-registry/principals.json`: owner-only host-issued schema-v1
  Context/project-to-owned-Workspace-and-Gateway endpoint bindings, maintained by lifecycle reconciliation
  and directory-mounted read-only into Gateway so atomic host updates remain
  visible without exposing credential files;
The default passthrough adapter does not load managed credential files.

Tool authentication state is not cluster configuration. It belongs below the
selected instance's persistent home and is created by the tool's own login or
configuration flow. Brokered authentication is separate installation state:
the normalized schema-v1 provider projection is generated below
`auth/projection/providers.json`; schema-1-envelope/schema-1-payload Context
vaults are below
`auth/contexts/<context-id>/vault.enc`; the Linux root key is the owner-only
`auth/keys/root.key`; runtime sockets are below `auth/runtime`; and schema-v1
Workspace authentication file registries are below `auth/projects`. On macOS,
the root key is instead stored in Keychain under service
`io.tobari.auth-root.v1` and account `tobari`.
The complete canonical schema/path/backend table is in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

Runtime state is stored under `${XDG_STATE_HOME:-$HOME/.local/state}/tobari`:
`roots/<hash>.json` indexes `(canonical root, stable Context ID)` and
`instances/<id>/state.json` schema 1 contains one logical instance, its
permanent Context binding, and diagnostic runtime identifiers.
`instances/<id>/home` is the independent writable home for tool-owned state;
homes are never shared merely because Contexts or roots match.
Shared read-only agent profiles referenced by Contexts are under
`${XDG_DATA_HOME:-$HOME/.local/share}/tobari/profiles`.
Persisted cluster state schema 1 contains the content-addressed aggregate policy revision,
loaded Context count, aggregate projection paths, and Docker resource names or
identifiers, never one active Context authority or credential contents. The
loader accepts only exact V1 state. The owner-only projection contains a
schema-v1 Context-aware Gateway credential document and Context-ID secret
subdirectories plus the non-secret schema-v1
provider projection. The per-Tobari home may contain tool credentials and
broker handles by design, but never a brokered primary secret.
Project and cluster mutation journals are durable recovery markers. An
interrupted cluster marker, aggregate revision mismatch, or failed projection
activation makes entry and policy operations fail closed until the exact shared
cluster operation completes.
Environment variables select XDG locations and documented test/runtime
overrides. Context narrow projections are the user-facing exception: at shell
entry Tobari reads only the selected subset of `PS1`, `TERM`, `COLORTERM`, and
`NO_COLOR`; at Workspace reconciliation Git inheritance makes at most two
bounded fixed-key host-global reads for `user.name` and `user.email`. Neither
path enumerates its source, and neither copies host credential values or source
configuration into the runtime.

Image selection uses the selected Tobari's bound Context image selector. A
new Context uses `builtin` until its manifest selects another image. Project metadata does not override the
Context image; the stored project image is updated only after a successful
runtime-container reconciliation from the bound Context image.

The current Context recipe is a host-side customization source, not a second
image authority. `runtime build` derives the image reference mechanically from
the Context name and Dockerfile source digest, validates it, and atomically
promotes it into the existing Context image field. Editing the recipe or a
failed build leaves the previously selected image unchanged. Existing
Workspaces bound to that Context pick up the promoted image on their next root
entry; failed image
validation or Docker reconciliation leaves their previous logical state and
home in place.

When the recipe's first base is the exact official
`ghcr.io/tasuku43/tobari/runtime:latest` reference, an explicit `runtime build`
refreshes that moving base. An explicit local or custom base is not given a
registry-pull request, so local-only base images remain usable.

OPA reads one cluster-owned revisioned aggregate bundle with `--watch`. The
projection has one fixed `tobari.http/decision` router, Context-ID data
namespaces, one shared guided evaluator, and isolated Advanced package names.
Guided Contexts own policy data but no executable Rego source; projection uses
the current Tobari-owned shared evaluator and tests. Advanced Context source
and Gateway runtime input both use exact schema 1 before testing and activation.
Exact policy mutations lock aggregate generation, test the changed Context
source privately, generate and test the complete all-Context candidate, publish
it by building a revision-named archive and atomically renaming it inside one
exact owner-labeled Docker-managed volume, and retain the prior known-good
aggregate if activation fails. Success requires the stable running
OPA to report the exact expected revision. Authority-reducing or mixed changes
first activate a complete deny-all transition revision. Host-authored edits remain an
advanced, explicit workflow.

## Side effects

Catalog-declared reads create no Tobari-owned XDG files or Docker resources,
including on first use and under concurrent observation. They may remove only
a pre-existing, validated mutation journal while completing its bounded
recovery under an already existing or recovery-required lock; they never
create state merely to observe it. Context, policy, credential, auth, project,
and lock initialization belongs to declared create/write outcomes.

`runtime init` creates the current Context's owner-only runtime directory and
template without changing image selection. `runtime build` executes the
explicit host Docker build, validates the generated image, and atomically
updates only the current Context manifest after the image digest is confirmed.
Docker build failure exits nonzero, ends text presentation with a short summary
of the failed stage, Dockerfile, recovery command, and retained state, and
leaves the previous selected image authoritative. BuildKit may retain
engine-owned cache layers, and a post-build validation failure may retain the
unselected candidate image; Tobari states this instead of deleting either one.
An uncertain promotion or post-promotion reporting failure directs the user to
inspect the Context before retrying rather than claiming the old selection is
unchanged.
Existing Workspaces are not mutated by `runtime build` itself; the next root
entry reconciles their work container to the promoted image while preserving
the Workspace home.
The official moving base is refreshed only for an explicit build whose recipe
starts from the exact official `runtime:latest`; custom and local bases retain
their local/cache-first behavior.

`context use` validates the target Context and atomically changes only the
current/default marker. It never starts Docker, changes the aggregate, or
modifies an existing Tobari's Context, runtime, home, policy, or principal.
Creating a Context also never starts Docker; when shared state exists, the
result directs the user to explicit `cluster up` so the all-Context projection
can be validated and activated.

`config shell` and `config git` atomically update only the selected Context
manifest after typed input, intent, target, and impact validation. The terminal
editor performs no write before Apply, and one Shell Apply writes its entire
validated distinct-variable batch or nothing. Git inheritance performs no Git read during the
configuration mutation; the next matching root reconciliation runs at most two
fixed, one-attempt, finite-time host Git queries, validates a complete pair,
and atomically refreshes one private per-Workspace fallback before Docker
mutation. Failure preserves the prior file and returns no raw Git diagnostic or
identity. The file's exact directory is mounted read-only as system scope and
includes the image system config before the Context fallback, preserving
normal Workspace-global and repository/worktree precedence.

`cluster up` obtains and preflights the immutable Gateway and Auth Broker images and official
runtime bases required by all Contexts, generates and validates the complete
aggregate policy/credential/provider projection, then creates shared labeled
networks, configuration material, exactly one Gateway, exactly one OPA,
exactly one Auth Broker, and CA volumes as needed. It unlocks the broker through
the supported host root-key backend and reconnects Gateway to the shared
networks and existing registered project networks without creating project
state or project resources. It verifies the exact Broker container identity,
prepares a fresh root-key-derived companion epoch, and starts one resident
same-binary host companion over a fixed reverse `docker exec -i` channel before
reporting ready. No host listener, host socket mount, shell, or public companion
command is created. It then waits for all three services and the companion to
be healthy and the broker to be ready. The root command only verifies the shared cluster is
configured and ready, reads the indexed Workspace candidates, and waits for an
explicit choice when the canonical current directory is below an ancestor.
After the choice is revalidated under the lifecycle lock, it creates or reuses
the selected Context-bound logical record, resolves the bound Context's narrow
Git identity fallback for that stable root, resolves and validates its bound
Context image before project runtime mutation, reconciles its labeled container and internal
network, binds its XDG home, joins Gateway to that network, waits for the
project healthcheck, reconciles configured provider handles and Tobari-owned
complete files, and enters the resulting terminal session. A changed handle
environment recreates only the work container; the Workspace identity, root,
and home remain. Docker create
appends Tobari's fixed `sleep infinity` lifetime command after the image; the
image `CMD` is not used to own Workspace lifetime. Shells and exact agent
commands run through child exec sessions. Each shell exec late-binds only the
bound Context's declared shell-environment inheritance and applies its fixed
fallbacks without changing container identity; ANSI color sequences in `PS1`
remain interpreted by the attached terminal. A child command's nonzero exit is
returned without stopping the reusable Workspace. A changed image identity,
runtime contract, mount/security/environment/health specification, or shared
profile revision recreates only the project container and preserves its
logical state and home.
Returning from that child session, including a normal shell `exit`, performs no
Workspace deletion: it only returns the child exit status and emits the host
stderr guidance described above. `delete` is the separate lifecycle-ending
operation. It removes only that exact label-owned container, network, root
index, instance state, and home after confirming that no session is attached;
`--force` overrides that one guard.
`cluster down` rejects while any
Tobari remains
and removes only exact shared resources; its `--purge` also removes shared CA
and active policy-bundle volumes. Both forms preserve every encrypted Context vault and the installation
root key; cluster cleanup is not credential logout or revocation. No command
removes a mounted root or files inside it. Each project
work container is created with fixed CPU, memory, PID-count, and container-log
bounds; a resource-contract change is treated as runtime drift and recreates
only that work container. These limits do not claim a disk quota for the
explicitly mounted root or network bandwidth shaping.
`policy allow`, `policy deny`, `policy reset`, `policy compact`, and final TTY
review Apply first build and test the complete candidate
policy in a private host temporary directory. After successful tests they
atomically replace only the affected `policy/data.json` sources and invoke the same activation
boundary. They never write Rego source, managed credential files, or tool-owned
home files.
OPA marks a denial learnable only when its version, cluster, scheme, fixed
request port, project-principal boundary, trusted GraphQL endpoint and parsed
coordinate when applicable, and (when selected) managed
credential binding already satisfy the orthogonal boundary.
Candidate
discovery excludes other denials, preventing a successful no-op approval.

`auth login`, `auth import`, and `auth logout` validate the fixed installation
credential-catalog target and mutation impact before acquisition or vault I/O.
Login runs the selected reviewed built-in host driver through an interactive
trusted-host terminal. GitHub retains its purpose-limited fixed device-page
open and no-Git behavior. AWS uses either a fixed validated IAM Identity Center
profile/device flow or a fixed AWS CLI 2.32-or-newer console-based remote flow.
Datadog uses fixed `pup --no-agent auth login --site datadoghq.com`, accepts
only strict default-session state, and removes the isolated home afterward.
OpenAI uses fixed pinned Codex device login and accepts only strict managed
ChatGPT state. Anthropic uses fixed pinned Claude `setup-token`, captures one
bounded inference-only token from the reviewed terminal frame, and exposes no
refresh state. All print only bounded, control-safe guidance; their opaque
state or selected token commits only into the encrypted Context
vault. Neither driver reads an ambient provider home or writes project or
Workspace CLI configuration; Auth Broker contains no provider CLI. Import
rejects terminal stdin before reading and reads bounded non-terminal input only
after public argument/intent/mutation validation; infrastructure validates the
selected Context, installed provider/acquisition mode, and broker readiness
before broker send. Login/import atomically replace one Context/provider grant
and revoke every prior handle. A successful built-in refresh preserves the
same grant revision; configuration change or re-login rotates it. AWS refresh
is invoked over the authenticated companion channel only after OPA allow. It
uses one per-record single-flight operation with finite lock wait and without
holding the Broker's global state lock over host/provider I/O. A same-record
waiter may wait at most one second; expiry is a known pre-execution availability
failure with no barrier or companion call. After acquiring the lock and before
host execution, Broker atomically persists an encrypted task barrier; only the same
correlated successful result clears it with refreshed state. A post-call
revision comparison discards a result made stale by replacement or logout, and
an unknown result requires AWS re-login rather than replay. Datadog resolve
uses the same per-record lock and encrypted barrier after OPA allow; it returns
a token with more than five minutes remaining or calls only the exact fixed
US1 OAuth token endpoint without ambient proxies or redirects. An uncertain
refresh requires Datadog re-login rather than replay. OpenAI resolve uses the
same lock and barrier after OPA allow; it returns a token outside the
five-minute refresh window or makes one bounded 30-second JSON POST to exactly
`https://auth.openai.com/oauth/token`, using the fixed Codex 0.146.0 public
client ID and refresh-token grant without ambient proxies or redirects. It
preserves omitted token fields, requires the refreshed account identity to
match, and atomically commits same-revision state before returning. When access
token expiry cannot be read, eight days after `last_refresh` is the conservative
refresh boundary. An uncertain refresh leaves the durable barrier and requires
OpenAI re-login or logout rather than replay. Anthropic performs no refresh and
requires explicit re-login before expiry or after provider rejection. Logout
atomically removes that record and its handles without contacting the provider.
One credential is Context/provider-owned, every permanently bound project is
eligible for a distinct handle only on its next matching Workspace entry, and
no mutation rewrites a running session. Confirmed results are secret-free and
distinguish `changed` from no-op logout. They list an exact Context-bound
working directory and argv only for a Workspace whose current projection is
authoritatively missing or stale; current, unavailable, unresolved,
zero-Workspace, and no-change results do not invent a re-entry action. Logout
revokes all old handles immediately when its receipt is `changed`; next entry
removes its declared environment projection
by recreation and removes only unchanged Tobari-owned complete files. `auth
status` is read-only and reports locked or unavailable Broker state as provider
availability uncertainty rather than inferring absence from an unreadable vault.

## Pre-public V1 boundary

All Tobari-owned command outputs, persisted state, OPA input and decisions,
audits, provider/projection/vault records, private protocols, and component APIs
use schema/API V1. Readers accept exactly V1 and fail closed on every other
version. Tobari has not been published, so it provides no migration, retired
command alias, old state interpretation, or compatibility shim for earlier
development snapshots. Development state must be removed and recreated when
the V1 contract changes.

The canonical Gateway and Auth Broker source labels are both API V1. Their
official immutable V1 image indexes have not yet been published and reviewed,
so `versions.env` records the paired `unpublished` marker. Contributor builds
use `task build:dev`; public and release gates reject the marker until reviewed
multi-architecture V1 digests replace it atomically. `cluster up` compares
required and selected identities before state loading or any Docker call.

## Unsupported outcomes

The deliberate non-goals in [Project Theses](00_theses.md) are not hidden
commands or transport escape hatches. Tobari supports ordinary HTTP/HTTPS
sockets through its guarded transparent path;
it does not forward raw TCP, non-HTTP TLS, UDP, QUIC, recursive DNS, Git SSH, or
certificate-pinned traffic. A client that cannot use the Tobari CA or expose an
unambiguous HTTP authority fails closed.
The built-in slice supports one GitHub.com credential, one configured
refreshable AWS CLI session, one default-organization Datadog US1 pup OAuth
session, one pinned Codex ChatGPT OAuth session, and one pinned Claude
inference-only setup token per Context. It does not add provider-specific policy
semantics, multiple accounts per provider, remote revocation, Git credential
helpers, GitHub App tokens, arbitrary or manifest-selected OAuth, manifest-selected refresh/signing,
SigV4a, query presigning, AWS streaming signatures, custom AWS endpoints, or a
general provider SDK/plugin executor. Standard AWS SigV4 is supported only for
bounded requests to reviewed AWS HTTPS suffixes and is signed after OPA allow.
The Datadog plan does not support arbitrary scopes, sites, custom gateways, or
named organizations. The static TWG example covers only one delegated token at exact
`api.atlassian.com:443`; general TWG login, refresh, and its other authorities
remain unsupported.
