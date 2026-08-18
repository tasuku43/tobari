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
Agent CLIs authenticate natively inside their persistent Workspace home; the
standard profile has no provider credential projection or Auth Broker.

## Primary users and owned outcome

The primary user is a developer who wants an autonomous coding agent to edit a
bounded source tree without receiving a real host-managed credential or
unrestricted network egress. Tool-native authentication is created inside the
selected Tobari's own persistent home and is readable by processes in that
Workspace; host home and CLI authentication state are never copied in. A separate
narrow-projection boundary may re-encode only thesis-declared non-secret
scalars; it never copies their source file, directive, executable setting, or
authentication material.
Inside an attached session, ChatGPT sign-in from ordinary `codex` or
`codex login`, the reviewed GitHub.com HTTPS `gh auth login` workflow, and
the pinned AWS CLI `aws sso login` authorization-code flow
additionally receive their native host-browser and localhost callback experience
through one session-scoped, pinned-client bridge. The user supplies no port,
URL, device mode, or manual callback transfer. Caller-added GitHub scopes,
GitHub Enterprise hosts, and SSH-key upload remain outside that bridge.
For GitHub and compatible custom-runtime TWG manual-code login, the native CLI
invokes the attachment-scoped opener only after its existing confirmation, so
the visible code can be copied first. Claude, Codex, GitHub, and AWS SSO callback-bearing
flows invoke it immediately. Tobari does not observe child output, consume child
input, or provide a clipboard shortcut.

Every interactive entry exposes the constant Host Loopback capability
`http://host.tobari.test:{port}` for physical-host IPv4 loopback HTTP on ports
1024 through 65535. No entry flag or service declaration is required. The
Workspace receives the URL template, bounded port range, Workspace audience,
and explicit `attachment` lifetime; `localhost` continues to mean the
Workspace. Capability discovery and routing metadata are not permission. The first exact
effect follows the ordinary deny, `policy review`, decide, and retry loop, and
the decision is available to every process in the Workspace only until the
owning host attachment exits. Docker, Compose, host daemon state, automatic
port discovery, raw TCP, and persistent Host Loopback grants are not part of
this outcome.
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
The host-issued project principal and normalized scheme are retained in denial,
candidate, learned-rule, and audit evidence; an approval made from one current-directory
Tobari cannot be replayed as another project's permission.
Ordinary request bodies are not a policy identity dimension. A body-bearing
POST, PUT, PATCH, or other method is authorized and learned from the same
project, scheme, host, port, method, and path dimensions as a body-free request.
Changing ordinary body content does not create another review item or rule.
For an exact trusted GraphQL endpoint, Gateway derives only the selected
operation type and canonical root fields from one bounded body; each root is a
separate exact permission. Gateway does not expose body content, body hashes,
GraphQL source, operation names, variables, arguments, aliases, fragments,
directives, nested selections, or literal values to OPA, retained evidence,
policy actions, audit output, or CLI output. A request may omit
`Content-Length` only without transfer/content encoding; the fixed 8 MiB
transport cap bounds receipt and Gateway rejects a complete body over 1 MiB
before parsing or policy.
Denial audit retains only the URL path component, never the query or headers.
If that path contains a Tobari handle marker, the whole recorded path is the
literal `/[redacted-auth-handle]`. Structural URL/header handle rejections are
non-learnable and cannot become policy candidates.

Stable macOS CLI releases are distributed through
`tasuku43/homebrew-tap`. The release outcome is not closed at GitHub asset
creation: the exact audited Formula must reach that tap through its reviewed
pull-request boundary. Linux remains supported through the published archive
or source build unless a Linux Homebrew Formula contract is added explicitly.

## Public vocabulary

- **Tobari:** one long-lived logical untrusted execution environment selected
  by a canonical project root and stable Context identity. Its work container
  is recoverable runtime implementation detail.
- **Workspace:** the human-facing name for one directory-bound Tobari in
  lifecycle and list output. It is not a second runtime resource; its identity
  remains the canonical root and its stable Tobari ID remains diagnostic.
- **cluster:** the one installation-local Gateway, one OPA, aggregate policy,
  principal registry, and CA lifecycle. The experimental development profile
  adds one locked Auth Broker and provider projection.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **Host Loopback:** the constant `host.tobari.test` HTTP destination whose URL
  port selects the same physical-host IPv4 loopback port for an active attachment.
- **Attachment Epoch:** one unguessable trusted-host identity owned by the
  `tobari` process that established the active Host Loopback route.
- **Attachment Grant:** one exact reviewed Allow or Deny bound to a Context,
  Workspace, Attachment Epoch, target port, and exact HTTP effect. It is
  Workspace-wide for that attachment and is not a learned policy rule.
- **Auth Broker:** the experimental non-root credential-resolution daemon. It owns
  encrypted Context vault access, has no TCP listener, starts locked, and
  exposes separate control and Gateway-only runtime Unix sockets.
- **root:** the canonical host directory selected from the current working
  directory and mounted directly with the bound Context's immutable
  `read-only` or `read-write` source access. A root below the host home
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
  one Tobari's persistent home during its own login or configuration flow. It
  is the standard credential source and is readable by every process in that
  Workspace.
- **experimental credential provider:** the external service or authority whose credential
  is acquired or imported, stored, and later applied to one exact reviewed
  request binding. A provider is not the Workspace client that uses it.
- **experimental Workspace client tool:** the CLI or other client inside a Workspace that
  receives a provider-declared handle projection and emits the authenticated
  request shape. Standard pairings cover GitHub/`gh`, Datadog/`pup`,
  OpenAI/Codex, Anthropic/Claude, and Chatwork/`cwk`; the experimental
  repository profile additionally covers AWS/`aws`. Their names
  grant neither provider identity nor network authority.
- **experimental brokered credential:** one typed static or reviewed renewable record owned
  by a stable Context and provider, acquired through protected non-terminal
  stdin or one purpose-limited reviewed host driver and stored in the
  encrypted Context vault.
- **experimental Workspace credential handle:** a versioned random opaque value bound to one
  Context, project, provider, credential revision, and exact HTTP binding. It
  is not the real credential, but it is a scoped bearer capability that should
  not be published or logged. It is not authority without the trusted
  principal, exact binding, and OPA allow. Broker metadata never inherits a
  broad static host/method allow; the first L7 effect remains reviewable until
  an explicit exact or single-segment-template learned rule exists.
- **experimental provider manifest:** strict non-secret data declaring static import,
  Workspace handle projections, and exact HTTPS/header credential bindings.
  Owner manifests are V1 static-secret/header plans. Standard reviewed
  built-ins are a closed typed union for GitHub, Datadog, OpenAI, Anthropic,
  and Chatwork; the experimental profile adds AWS without making it runtime configurable.
  Owner data declares no executable shell, helper choice, refresh, signing,
  arbitrary route, HTTP method/path policy, or provider operation semantics.
- **Context:** one host-owned capability envelope with a stable
  opaque ID and a human name. Its manifest records direct source access,
  normalized policy-preset origin and snapshot revision, an immutable
  enabled/disabled native-readiness selection, and references an
  agent profile, compatible Tobari runtime image, and policy store. Its stable
  ID determines policy and runtime ownership. Enabled native readiness selects
  the installed trusted binary's current overlay without mutating the snapshot;
  preset guardrails and ceilings remain terminal. Experimental builds may maintain
  separate Context-owned Auth Broker state; the manifest contains no broker
  vault path, key, or secret.
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
project-bound. Principal identity never selects or injects tool credentials.
Standard Gateway passes one Workspace-owned credential only after the ordinary
HTTP decision; experimental Broker resolution additionally requires its exact
provider, revision, target, and header binding.

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
| `version [--format text|json]` | utility | read | Print source version/commit, resolver channel, required and selected standard component APIs, and compatibility |
| `doctor [--root PATH] [--format text|tsv|json]` | utility | read | Report read-only host, Docker, configuration, policy, Gateway, port, and residue diagnostics without repairing state |
| `cluster up` | act, fixed target | create | Validate all Context policy inputs and image contracts, reconcile Gateway and OPA, and confirm the exact aggregate policy is active |
| `tobari [--context NAME]` | act, fixed target | create | Choose or create the current directory's Workspace in the explicit or current Context, reconcile runtime, enter it with a deny-by-default attachment-owned Host Loopback capability, and leave it reusable after `exit` while closing any owned route and grant |
| `status [--context NAME] [--format text|json]` | utility | read | Inspect the nearest current-directory Workspace in the explicit or current Context, its logical existence, runtime diagnostic, and attached/detached session observation |
| `list [--format text|json]` | utility | read | List local Workspaces with Context, runtime diagnostics, and diagnostic IDs |
| `delete [--context NAME] [--force]` | act, fixed target | write | Delete the nearest current-directory Workspace in the explicit or current Context, its owned runtime, persistent home, and tool-owned authentication state while preserving project files; `--force` overrides only the attached-session guard |
| `cluster status [--format text|json]` | utility | read | Inspect Gateway/OPA health, loaded Context count, aggregate revision, current-binary policy/Gateway projection integrity, and recent errors |
| `cluster denials [--tail N] [--format text|json]` | utility | read | Read a bounded typed denial window, exact-rule learnability, policy path, and review command |
| `cluster logs [--component gateway|opa|all] [--tail N]` | utility | read | Read bounded shared logs, including policy-denial evidence, without credential output |
| `cluster down [--purge]` | act, fixed target | write | Remove shared transient resources after every logical Tobari is deleted; `--purge` additionally removes shared CA and active policy-bundle volumes |
| `policy candidates [--tail N] [--format text|json]` | discover | read | Discover Context/project-scoped pending exact HTTP or GraphQL-root candidates and opaque IDs across the installation |
| `policy review [--tail N] [--format text|json]` | discover plus TTY fixed-target apply | read, or one confirmed write | Review the installation-wide Permission Inbox; on a TTY, stage persistent exact/template decisions or exact attachment-only Host Loopback decisions without changing their typed lifetime and apply the reviewed set once; redirected and JSON output remain read-only |
| `policy allow --id ID` | act, reference bound | write | Test, record, and activate one exact observed permission |
| `policy deny --id ID` | act, reference bound | write | Test, record, and activate one exact project-bound rejection |
| `policy rules [--format text|json]` | discover | read | List every Context-scoped CLI-owned learned Allow and exact Deny decision; on a TTY, reset one explicitly |
| `policy reset --id ID` | act, reference bound | write | Remove one learned decision and leave its effect at default deny |
| `policy preset list [--format text\|json]` | utility | read | List the exhaustive installed built-in and custom policy preset catalog with current content revisions and guardrails |
| `policy preset show --name PRESET [--format text\|json]` | utility | read | Inspect one complete normalized policy preset without activating it |
| `policy preset init --name NAME [--format text\|json]` | act, fixed target | create | Create one owner-only strict custom policy preset template without overwriting |
| `policy preset validate --name PRESET [--format text\|json]` | utility | read | Strictly validate, normalize, and digest one custom preset source without changing Context or active policy |
| `context list [--format text|json]` | utility | read | List persisted named Contexts and report the current selection as persisted or a display-only synthetic default |
| `context show [--name NAME] [--format text|json]` | utility | read | Inspect one Context's explicit persistence state, immutable source access, preset origin/revision, effective method default/overrides, agent, policy, and native Workspace-owned authentication mode without returning credential values |
| `config shell [--variable COLORTERM\|NO_COLOR\|PS1\|TERM] [--source default\|inherit\|literal] [--value VALUE] [--context NAME] [--format text\|json]` | act, fixed target | write | Configure one allowlisted shell-presentation variable directly, or stage one or more rows from the complete terminal inventory and apply them atomically |
| `config git [--source default\|inherit\|literal] [--name NAME] [--email EMAIL] [--context NAME] [--format text\|json]` | act, fixed target | write | Configure one atomic Context Git commit-identity fallback directly, or stage and apply its source from one terminal screen |
| `config bootstrap aws [--profile NAME] [--refresh] [--remove] [--context NAME] [--format text\|json]` | act, fixed target | write | Normalize one strict secret-free host AWS IAM Identity Center profile for future Workspaces, refresh it after a semantic diff, or remove the future recipe; existing Workspace homes never change |
| `config bootstrap kubernetes eks [--kube-context NAME] [--refresh] [--remove] [--context NAME] [--format text\|json]` | act, fixed target | write | Compose one strict AWS CLI-generated host EKS context with the Context AWS profile, refresh it, or remove only EKS; no credential, arbitrary exec, network authority, or existing Workspace home changes |
| `context create [--name NAME] [--image IMAGE] [--mode guided|advanced] [--source-access read-only\|read-write] [--policy-preset PRESET] [--native-readiness enabled\|disabled] [--bootstrap-aws-profile NAME] [--bootstrap-eks-context NAME] [--format text\|json]` | act, fixed target | create | With no inputs, use one continuous five-step terminal frame for name, source access, a complete HTTP method policy, optional typed Workspace bootstrap, and final review, then create once; any explicit input selects deterministic direct mode and requires `--name`; EKS requires AWS and omission imports no host configuration |
| `context delete --name NAME [--format text\|json]` | act, fixed target | write | Delete one unused non-current non-default Context and its exact owner stores while preserving project files and shared runtime images |
| `context use --name NAME [--format text\|json]` | act, fixed target | write | Change only the current/default Context; do not mutate existing Tobari or start/reconcile the cluster |
| `runtime init [--format text|json]` | act, fixed target | create | Create the current Context's runtime/Dockerfile template without changing its selected image |
| `runtime build [--format text|json]` | act, fixed target | write | Build, validate, and select the current Context's generated local runtime image |

The unsupported experimental development profile built by `task build:dev`
additionally exposes `serve [--no-open]`. It runs one foreground IPv4-loopback
Operator Console for typed cluster, Workspace, Permission Inbox, and learned-rule
inspection; it may open the host browser, stages decisions without authority,
and delegates one confirmed reviewed set to the canonical fixed-target Apply.
The standard development binary and release archives omit this command.

For the CWD lifecycle commands `tobari`, `status`, and `delete`, one non-empty
invocation Context may appear before or after the command path: for example,
`tobari --context toolbox status` and `tobari status --context toolbox` are
equivalent. Omission resolves the current Context without changing it;
duplicate or explicit-empty placement is invalid. After name resolution, the
stable Context ID is authoritative for the remainder of the operation.

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
- The selected root is the only project directory mounted from the host and
  uses the bound Context's immutable `read-only` or `read-write` access. When
  it is below the host home, the container target preserves the relative path
  below `HOME=/var/lib/tobari`; for example, a host root of
  `$HOME/path/to` enters at `/var/lib/tobari/path/to`. A root outside
  the host home retains `/workspace/<canonical-root-without-leading-slash>`;
  thus a root at `/work` and a host CWD of `/work/root` enter at
  `/workspace/work/root`. Read-only applies only to this direct live bind: it
  is not a snapshot, host or same-root read-write Context changes remain
  observable, no writable source alias exists, and Workspace home and tmpfs
  remain writable. Tobari never mounts the host home wholesale.
- Same-root Tobari in different Contexts and parent/child roots may run
  concurrently. Runtime, home, network, policy, and broker handles follow
  their Tobari/Context boundaries, but overlapping host-file writes are visible
  to every mount of those files. Tobari provides no overlay, checkout clone,
  root lock, session exclusion, warning gate, or filesystem integrity isolation
  for this user-selected sharing.
- The configured image accepts `builtin` or a portable OCI image reference.
  In a release build, `builtin` resolves to a source-derived local image. When
  absent, `cluster up` builds it from the CLI's embedded pinned recipe before
  compatibility validation. Contributor builds resolve to the local
  development base. The combined agent-ready base is permanently local-only
  and is not a release registry artifact.
  A custom image must already exist locally and preserve runtime API `1`, the
  `tobari` image user, the `io.tobari.runtime-lifetime-command` capability, and
  the Tobari entrypoint. That capability is currently `sleep infinity`, which
  is required by Tobari's fixed Workspace lifetime command. Ordinary `tobari`
  startup never pulls a configured image implicitly. `cluster up` may obtain
  the official runtime base for an uncustomized Context; custom images still
  fail closed if missing or incompatible before project runtime network or
  container mutation.
- `cluster up` builds the pinned agent-ready base and Gateway locally when
  absent from the CLI's embedded source-derived image identities. It validates
  API/role labels, non-root default user,
  entrypoint, and Docker Engine platform before running policy tests or
  creating shared networks and containers. Released and contributor binaries
  use distinct content-addressed local tags derived from their embedded source.
  Tobari publishes no Gateway, runtime, or Auth Broker OCI image, and moving
  registry tags never become runtime authority.
- Standard authentication is owned by each agent CLI in the Workspace and the
  standard catalog contains no `auth` namespace. The following legacy driver
  contract is compiled only by `task build:dev`. Its authentication commands
  accept an existing Context name and installed provider ID; the experimental
  profile accepts GitHub, Datadog, OpenAI, Anthropic, Chatwork import, and AWS.
  AWS alone adds
  `--method identity-center|console`. The GitHub driver shows the
  GitHub device code and the trusted host opens exactly
  `https://github.com/login/device` when possible, with the same URL retained
  for manual fallback. The OpenAI driver runs Codex's native browser login;
  the verified Codex child owns its loopback callback listener, dynamic OAuth
  state and URL, browser request, callback, and token exchange. Tobari binds no
  callback port and never parses or opens that dynamic URL, but preserves the
  bounded manual guidance Codex prints. Its visible terminal stream recognizes only Codex's
  reviewed reset, muted, and accent SGR sequences and regenerates them as
  Tobari-owned presentation; `NO_COLOR` removes those styles without changing
  the instructions, while every other control remains visibly projected. The
  Anthropic driver starts a fresh project-free container from the selected
  compatible Context image, runs exact Claude Code 2.1.220 native account login,
  validates the four required renewable-session values plus the non-secret
  subscription-type and rate-limit-tier labels from its Linux state, discards
  every other provider-owned optional field, canonicalizes a Tobari-owned
  record, and requires checked
  cleanup before Broker commit. Tobari opens only the exact reviewed Claude
  authorization URL. Its fixed terminal UI reports browser-open success
  without repeating the long URL, shows that exact URL only when host opening
  fails, and emits the paste-code prompt as soon as Claude requests input even
  though the upstream prompt has no newline. Before that prompt it states that
  authorization may take a moment after Enter; after successful child exit it
  reports bounded Context-credential validation in progress. Claude's reviewed prompt owns its
  non-echoing terminal input; Tobari emits no entered code and does not hide the
  next child status line by guessing that it is an echo. Tobari passes the
  original terminal input stream unchanged to Docker and Claude so their TTY
  detection remains authoritative. The isolated login
  container overrides the Workspace CA-waiting entrypoint with fixed
  `/usr/bin/tini -- /usr/bin/sleep infinity`, because the acquisition container
  deliberately has no Workspace CA mount. While Docker owns the raw interactive terminal, every Tobari-owned
  and control-safe pass-through Claude line uses explicit CRLF framing so each
  row begins at column one. Exact Claude cursor hide/show events are consumed
  as presentation state and Tobari emits one owned cursor-show at completion;
  they never appear as escaped instructions. Setup, authorization, output, timeout, native
  credential capture, and login-container cleanup remain distinct secret-free
  failures, and each preserves the previous Context credential. A capture
  failure includes exactly one fixed diagnostic stage covering export,
  archive, permissions, document, OAuth core, token, expiry, scope set,
  entitlement, or
  canonical record; it includes no provider value or raw child cause. OAuth
  scope names are not fixed by Tobari: it bounds and canonicalizes the observed
  requested and granted sets, rejects a grant outside the request, and carries
  the granted set unchanged through refresh and Workspace projection. The
  Workspace credential projection gives Claude Code the project-bound handle
  as `accessToken`, the fixed non-secret `dummy-value` refresh sentinel, the
  dynamic scope set, and the captured subscription/rate-limit labels. The
  sentinel has no Broker-handle shape and grants no refresh authority; the
  primary refresh token remains Broker-only. The Workspace projection also
  merges only the reviewed non-secret
  `hasCompletedOnboarding: true` field into Claude's private top-level state so
  an already authenticated interactive client does not repeat account login;
  unrelated Claude state is preserved and the field is removed with the
  Anthropic projection. The
  shared Tobari browser behavior never opens a URL derived from arbitrary provider text. The
  GitHub driver runs fixed API-authentication-only GitHub CLI argv
  from one canonical non-project executable in a private temporary home and
  configures no Git protocol or credential helper. Auth Broker contains no
  provider CLI executable. `auth import` accepts a non-empty credential of at most
  32 KiB from non-terminal stdin only; terminal stdin is rejected before any
  byte is read. Non-terminal bytes are read only after public Context/provider
  argument, intent, and mutation validation; infrastructure then validates the
  selected existing Context, installed provider/acquisition mode, and broker
  readiness before broker send. The credential is never a positional/flag
  value or Tobari environment input. Every successful auth mutation requires
  existing Workspaces to be re-entered before their environment or
  handle projection can change. Reviewed built-ins may use bounded dynamic
  records, Datadog/OpenAI/Anthropic refresh, AWS signing and the private companion, or
  the OpenAI supplemental header. Managed profiles and owner-selected dynamic
  behavior remain absent.
- `runtime init` creates the current Context's owner-only
  `runtime/Dockerfile`. The template starts from the resolver-selected local
  source-derived base or the contributor-local `tobari-runtime:dev`. Editing
  that file is the supported
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
- `context create` has two complete modes. With no command input, text success
  and error formats plus terminal stdin/stderr are required; the wizard reads a
  valid name, chooses direct source access, stages `allow`, `exact_review`, or
  `deny` for the extension-method default and each standard HTTP method, and
  optionally selects a typed Workspace bootstrap. On a raw-capable terminal,
  name, filesystem, network, bootstrap, and final review share one alternate-
  screen session; step transitions do not return to a normal line prompt and
  Back preserves staged values. A terminal without the reviewed raw-mode
  support uses the bounded line-mode equivalent. Final Create performs one
  mutation and cancellation from any step performs none. Any
  explicit input, including `--format`, selects direct mode and requires
  `--name`; defaults complete omitted direct-mode values without prompting.
  Method Deny removes selected-preset positive baseline entries for that method
  from the new immutable snapshot rather than leaving an invalid or misleading
  unreachable grant. Redirected or JSON argument-free creation fails before
  mutation.
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
  lifecycle contract and its common-tool baseline includes Git, HTTP, JSON,
  Python, SSH, GitHub CLI, AWS CLI, Claude Code 2.1.220, and Codex 0.147.0.
  Both agent executables live outside the mutable Workspace home; Claude
  self-update is disabled and Codex uses its pinned standalone package.
  `kubectl`, `cwk`, `pup`, and TWG are not added to the base. `twg_ready`
  supplies only TWG CLI 1.2.5 network/browser lifecycle readiness, including
  exact site inventory, stable CLI manifest, token revoke, and GraphQL `query`
  / `me` current-user lookup; `pup_ready` supplies only
  pup 1.10.7 default-US1 DCR, token, browser, and callback readiness. Neither
  bundle installs its client. A selected custom Context runtime must provide
  the exact compatible client before its login can run. The protected release workflow publishes the reviewed combined
  base as one immutable Linux amd64/arm64 index alongside Gateway and Auth
  Broker. A local image or standalone validation workflow is not publication authority.
- The pinned client versions and `builtin/agent-ready` exact and semantic effect catalog are
  one compatibility contract. Its compile-time `claude_ready`, `codex_ready`,
  `gh_ready`, custom-runtime `twg_ready`, and custom-runtime `pup_ready`
  bundles are projected by the
  installed binary into exact native-authentication effects for every existing
  and future agent-ready Context;
  GitHub CLI additionally receives only GraphQL `query` root `viewer`, and TWG
  CLI receives only exact site inventory, stable CLI manifest, token revoke,
  and GraphQL `query` root `me`, at their declared exact API endpoints;
  pup receives only exact US1 DCR registration and token exchange/refresh.
  The bundles are not runtime selectors or
  executable identity. Updating a pinned client or its independent readiness
  contract revision requires reviewing its artifact lock where changed, exact
  effects, host interactions, and core control-plane effects. Separate agent-image recipes
  remain build-only validation inputs and create no second authority boundary.
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
| Context deletion | `context_deletion` | 1 |
| Cluster status | `cluster` | 1 |
| Cluster denials | `denials` | 1 |
| Policy candidates | `policy_candidates` | 1 |
| Policy review | `policy_review` | 1 |
| Policy rules | `policy_rules` | 1 |
| Policy presets | `policy_presets` | 1 |
| Workspace list | `tobari` | 1 |
| Workspace status | `status` | 1 |
<!-- public-cli-json-schemas:end -->

Workspace status always reports the selected Context ID/name, logical
existence, runtime diagnostic, attachment observation, and exact Context-bound
next argv. Context reports include a complete four-item shell-environment
inventory, complete Git identity policy, and authentication mode
`native_workspace`. Native agent credentials are created and persisted by the
agent CLI inside the Workspace home and never appear in CLI output.
Unconfigured cluster resources are `null`; unavailable
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
`resolver_channel`, `development_source`, `capability_profile`, required and selected Gateway
APIs, `compatible`, `development_build_command`, and
`development_binary`. An absent source commit is the explicit string
`unknown` and makes `compatible=false`. The two repository-command fields are
empty for an embedded resolver and contain exactly `task build` and
`bin/tobari` only when the compiled development metadata proves that path.
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
It stages unchanged opaque review-item references only from typed detail screens and uses
one final `policy apply-reviewed` fixed-target action to revalidate and activate
one Context's set. Apply or discard is required before switching Context so the
source change remains one atomic domain-generation replacement. `policy rules` is the
current learned-decision inventory; its TTY reset flow delegates one explicit
opaque reference to `policy reset`. Redirected and machine-readable review and
inventory remain read-only. The Permission Inbox groups candidates by their
validated stable Context and project identities, renders the Context/root scope
once per group, and leads each selectable row with the exact HTTP effect or
typed `{id}` template and its evidence count. A compact selected-effect preview exposes the latest retained
observation before detail inspection. Matching display names, paths, order, or
indentation do not merge distinct typed identities. Action keys are inactive
on the list. An exact detail offers Allow exact and Deny exact; a template
detail states that unseen values are included and offers Allow template, Allow
observed exact, and Deny pending exact. The chosen action is staged without a
second yes/no prompt. Only the final Apply delegates
the reviewed set to the mutation boundary. Apply is advertised only for a
non-empty staged set, shows one final ordered typed review, and requires an
explicit confirmation. Refresh preserves choices by candidate ID and drops
stale IDs rather than matching labels. Confirmed output carries the active OPA
revision plus each ordered Context/project/effect/stored-rule decision and
directs the caller to retry in the current running Workspace. The public
read-only JSON review schema remains version 1 and does not expose this
internal TTY Apply receipt.
Experimental `tobari serve` exposes the same human task in one foreground
Operator Console and is absent from the standard and release catalogs.
It binds only a random IPv4 loopback port, issues one process-memory 256-bit
session bearer through the initial URL fragment, and stores no cookie or
persistent browser credential. The page removes that fragment after moving the
bearer to tab-scoped session storage. Every API call must present the bearer;
writes additionally require the exact loopback Origin. The console stages
exact/template Allow or exact Deny decisions locally, shows the complete typed
review, and requires explicit Apply before delegating to the same internal
`policy apply-reviewed` action. Closing the page or process discards staging;
the server never retries a mutation automatically. Success shows the
authoritative active revision and ordered stored-rule receipts. The command is
not a daemon, has no remote bind or caller-selected port, loads no external
assets, and `--no-open` only suppresses the purpose-limited host-browser open
while retaining the printed session URL.
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
change, not for an idle terminal-read poll. They use a bounded alternate screen
and repaint from terminal home instead of moving by logical row counts, so a
long row that wraps cannot duplicate headings or drift later redraws. Finishing
restores the main screen and visible cursor exactly once.
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
type/root field. Reason, status, request ID, timestamp, and broker-handle
evidence do not create a second permission identity. The
candidate retains the latest matching evidence and reports the required number
of matching retained observations. Concurrent identical audit records therefore project to one pending
item without a separate mutable inbox write. A current learned Allow or exact
Deny remains the resolved history and is never updated by discovery. After an
explicit reset, retained matching evidence may produce the same stable pending
candidate ID again.
For interactive review only, typed domain logic also considers current exact
Allows as evidence. Two distinct compatible HTTP paths with the same Context,
project, scheme, host, port, method, and segment count may produce one inert
single-segment `{id}` proposal when exactly one safe raw segment differs and at
least one source is still pending. Repeated identical observations do not meet
that threshold. Ambiguous, shallow, percent-encoded, empty/dot-segment,
backslash, multi-segment, and GraphQL proposals are suppressed. Proposal
identity binds the proposed authority, while final Apply rebuilds its current
evidence before any mutation.

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
  image selector, guided/advanced policy mode, required immutable
  `source_access`, required normalized `policy_preset_origin` and
  `policy_preset_revision`, explicit `native_readiness`, allowlisted shell-environment
  overrides, and an optional non-default Git identity policy;
- `contexts/<name>/runtime/Dockerfile`: optional owner-only Context runtime
  recipe created by `runtime init`; its source digest and last successful
  managed image build are recorded additively in `context.json`;
- `contexts/<name>/policy/preset.json`: owner-only normalized schema-v1
  non-executable snapshot whose SHA-256 digest equals the manifest preset
  revision; source preset changes never rewrite it. Enabled native readiness
  grants the reviewed Claude Code 2.1.220 and Codex 0.147.0 native model,
  account, bootstrap, first-party capability-discovery, bounded evaluation,
  and telemetry effects plus the pinned GitHub CLI 2.96.0 exact device-login
  bootstrap and GraphQL `query` / `viewer` current-user effects, plus TWG CLI
  1.2.5 exact device-code/token/revoke, site inventory, stable CLI manifest,
  and GraphQL `query` / `me` current-user effects,
  plus pup 1.10.7 exact US1 DCR registration and token exchange/refresh when
  supplied by a custom runtime, to
  every process in the Context, subject to the preset's terminal destination
  ceiling and method decisions. Those native readiness effects are not stored
  in new snapshots. Compile-time `claude_ready`, `codex_ready`, `gh_ready`, and
  `twg_ready`, and `pup_ready` names are review provenance; aggregate generation strips their
  complete historical snapshot forms and projects the installed binary's
  current set. The five families, their pinned client version, current
  readiness contract revision, and append-only historical contracts are
  declared in one dedicated compile-time catalog. If the active
  aggregate revision differs from the revision desired by that catalog and
  the current Context sources, status reports an invalid policy projection and
  root entry requires explicit `cluster up`. Strict schema V1 carries optional
  GraphQL protocol/operation/root identity on a `baseline_grants` item and keeps
  the existing explicit `mcp_baseline_grants` collection; every semantic grant
  requires its matching declared exact endpoint. HTTP-only normalized snapshots
  omit those optional GraphQL keys and retain their bytes. MCP initialization
  and enumeration are baseline methods at one exact endpoint; `tools/call`
  requires exact tool-name review. Dynamic evaluation uses one safe identifier
  segment without persisting its value. It is Context authority, not executable
  identity; exact Deny remains terminal, while payload arguments, downloads,
  file transfer, acquisition, self-update, and unmatched effects receive no
  baseline grant;
  schema V1 requires `method_policy.default` and sorted unique exact
  `method_policy.overrides`, each using `allow`, `exact_review`, or `deny`.
  `builtin/offline` defaults to Deny; `builtin/reviewed-exact` defaults to
  Exact Review; `builtin/get-only-reviewed` defaults to Deny with GET Exact
  Review; `builtin/public-get-reviewed` defaults to Exact Review with GET
  Allow. Exact Deny wins over method Allow. GET is not described as safe or
  read-only. Custom presets are strict
  owner-only non-executable data with no wildcard, IP/private destination,
  secret, shell, Rego, include, inheritance, remote fetch, refresh, or signing;
- `contexts/<name>/policy/domains/<canonical-host>/allow.json`: strict
  schema-v1 authority, per-domain method, exact GraphQL endpoint, and
  learned-Allow source for one canonical lower-case
  host;
- `contexts/<name>/policy/domains/<canonical-host>/deny.json`: strict
  schema-v1 baseline-Deny and learned exact-Deny source for the same host;
  every directory, embedded host, authority, endpoint, binding, and rule host
  must match exactly. Guided Contexts own no Rego files, while Advanced
  Contexts additionally own `tobari.rego` and `tobari_test.rego`. Context
  policy has no `data.json`; the generated immutable OPA projection may use
  that filename. Wildcards, IP literals, non-canonical hosts, unknown fields,
  duplicate keys or rule IDs, incomplete pairs, symlinks, unsafe permissions,
  and extra files fail closed;
- `auth/providers/*.json`: experimental-only owner provider manifests, ignored
  by standard builds;
- `contexts/active.json`: owner-only current/default Context
  selection; missing means `default` and the marker has no enforcement authority;
- `principal-registry/principals.json`: owner-only host-issued schema-v1
  Context/project-to-owned-Workspace-and-Gateway endpoint bindings, maintained by lifecycle reconciliation
  and directory-mounted read-only into Gateway so atomic host updates remain
  visible without exposing credential files;

Tool authentication state is not cluster configuration. In standard it belongs
below the selected instance's persistent home, is created by the tool's own
login, and follows the ordinary post-policy passthrough route.
The attached host process may transiently validate one exact Claude Code, Codex,
GitHub CLI, AWS CLI, custom-runtime TWG, or custom-runtime pup authorization URL and open
it. Codex, GitHub web-application, AWS SSO, and pup callback variants may relay one
opaque host-loopback callback to that same selected Workspace; Claude's remote
callback, GitHub's device target, and TWG's device target create no listener.
AWS SSO is limited to the pinned CLI's exact commercial-region authorization-code
shape, default `sso:account:access` scope, bounded DCR/state/PKCE fields, and
dynamic non-privileged `127.0.0.1/oauth/callback` port. Its documented
`--use-device-code` option remains the cross-device recovery.
Pup is limited to the exact US1 authorization route, a bounded DCR client ID,
the sorted pup 1.10.7 default-scope ceiling, and exact
`127.0.0.1:{8000,8080,8888,9000}/oauth/callback`. Claude Code 2.1.220 must use its fixed client, redirect, PKCE shape,
and complete reviewed scope set. For exact default `gh auth login`, the canonical
runtime's pinned compatibility wrapper selects the reviewed GitHub.com HTTPS
device workflow and inherits the attachment-scoped `GH_BROWSER` opener.
The client displays its code and waits for native Enter; only then does it invoke
that opener. After success the wrapper delegates fixed
Git credential setup to the same pinned client. Other GitHub CLI argv remains
native. TWG likewise invokes the shared opener after its native browser
confirmation. The dedicated schema-v1 request is not URL authority; the host
independently validates the target and Workspace. The bridge stores no callback, code, credential, or
durable authentication state and creates no cluster service. The
attached shell retains the real Docker terminal boundary without an observation path.
Experimental Broker state is separate installation state:
the normalized schema-v1 provider projection is generated below
`auth/projection/providers.json`; schema-1-envelope/schema-1-payload Context
vaults are below
`auth/contexts/<context-id>/vault.enc`; the Linux root key is the owner-only
`auth/keys/root.key`; runtime sockets are below `auth/runtime`; and schema-v1
Workspace authentication file registries are below `auth/projects`. On macOS,
the root key is instead stored in Keychain under service
`io.tobari.auth-root.v1` and account `tobari`.
The complete canonical schema/path/backend table is in
[Authentication handling](07_authentication.md#experimental-canonical-schemas-paths-and-backend-identifiers).

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
loader accepts only exact V1 state. The owner-only projection contains the
Context-aware Gateway routing document. The per-Tobari home may contain native
tool credentials by design. Standard has no provider projection or declared
credential binding.
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

When the recipe's first base is the resolver-selected official local runtime,
an explicit `runtime build` verifies or builds that pinned base. An explicit
local or custom base is not given an implicit registry-pull request.

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
The official immutable base is pulled only for an explicit build whose recipe
starts from the exact resolver-selected release digest; custom and local bases
retain their local/cache-first behavior.

`context use` validates the target Context and atomically changes only the
current/default marker. It never starts Docker, changes the aggregate, or
modifies an existing Tobari's Context, runtime, home, policy, or principal.
Creating a Context also never starts Docker; when shared state exists, the
result directs the user to explicit `cluster up` so the all-Context projection
can be validated and activated.

`context delete` validates one exact name under the installation lifecycle
lock. It rejects `default`, the current marker, and any stable Context identity
still referenced by a logical Workspace. A confirmed delete removes the exact
owner-only Context directory and Context-ID authentication state, preserves
project roots and shared runtime images, and reports `requires_reconcile` when
shared state exists. V1 has no force mode and never chooses a replacement
current Context implicitly.

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

`config bootstrap aws` atomically updates a separate secret-free recipe in the
selected Context. It reads only host `~/.aws/config`, accepts one strict IAM
Identity Center profile/session subset, and reports only adapter, profile,
generation, revision, and changed-field metadata. Configure and refresh affect
future Workspace creation only; remove stops future projection. No variant
reads or outputs AWS credentials or SSO cache values, invokes AWS, performs
login, changes network policy, or rewrites an existing Workspace home. A new
Workspace receives one canonical private `.aws/config` and records its applied
semantic revision before logical publication.

`config bootstrap kubernetes eks` composes one additional closed adapter with
that AWS recipe. It reads only fixed host `~/.kube/config`, selects one explicit
context and its exact cluster/user references, and accepts only an inline CA,
commercial EKS HTTPS origin, and the reviewed `aws eks get-token` contract with
`AWS_PROFILE` equal to the Context AWS profile. It rejects credentials, proxy or
insecure TLS options, file references, arbitrary exec fields, role arguments,
unknown fields, alternate paths, and unsafe files. Projection emits canonical
private `.kube/config` JSON in the fresh Workspace home. Removing EKS preserves
AWS; AWS cannot be removed or changed to another profile until its dependent EKS adapter is removed. No
configure, refresh, or create operation calls AWS or Kubernetes or grants a
network effect.

`cluster up` obtains and preflights the immutable Gateway image and official
runtime bases required by all Contexts, generates and validates the complete
aggregate policy/routing projection, then creates shared labeled networks,
configuration material, exactly one Gateway, exactly one OPA, and CA volumes
as needed. It reconnects Gateway to the shared
networks and existing registered project networks without creating project
state or project resources and waits for both services to be healthy. The
root command only verifies the shared cluster is
configured and ready, reads the indexed Workspace candidates, and waits for an
explicit choice when the canonical current directory is below an ancestor.
After the choice is revalidated under the lifecycle lock, it creates or reuses
the selected Context-bound logical record, resolves the bound Context's narrow
Git identity fallback for that stable root, resolves and validates its bound
Context image before project runtime mutation, reconciles its labeled container and internal
network, binds its XDG home, joins Gateway to that network, waits for the
project healthcheck and enters the resulting terminal session. Docker create
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
`policy allow`, `policy deny`, `policy reset`, and final TTY review Apply first
build and test the complete candidate
policy in a private host temporary directory. After successful tests they
preserve unchanged source bytes, build a complete replacement `domains/`
generation, and invoke the same activation boundary. The whole source
generation is swapped under an in-process mutex and a cross-process lock with
a durable recovery journal; an interrupted or externally edited transaction
cannot expose a mixed valid generation. They never write Rego source, broker
vaults, or tool-owned home files.
OPA marks a denial learnable only when its version, cluster, scheme, fixed
request port, project-principal boundary, trusted GraphQL endpoint and parsed
coordinate when applicable, and preset guardrail already satisfy the
orthogonal boundary.
Candidate
discovery excludes other denials, preventing a successful no-op approval.

In the experimental build only, `auth login`, `auth import`, and `auth logout`
validate the fixed installation
credential-catalog target and mutation impact before acquisition or vault I/O.
Login selects only the active profile's closed provider union through an
interactive trusted-host terminal. It includes GitHub, Datadog, OpenAI,
Anthropic, Chatwork import, and AWS. Anthropic alone
executes in a fresh selected-Context-image container; each driver owns
fixed argv, canonical executable identity, private state, bounded browser/PTY
behavior, strict typed capture, and checked cleanup. PATH resolution inspects
at most 256 distinct absolute candidates in order, never executes a rejected
temporary/project/home-local shadow, and selects only the first candidate whose
canonical executable satisfies the existing trusted-root and mode checks. It
prints only bounded,
control-safe guidance and commits the captured record only into the encrypted
Context vault. A host-login availability failure retains its stable fault
code and names one fixed secret-free diagnostic stage; it never publishes the
selected local path, digest, raw process error, or captured output. Drivers
read no ambient provider home and write no project or Workspace CLI
configuration; Auth Broker contains no provider CLI. Import
rejects terminal stdin before reading and reads bounded non-terminal input only
after public argument/intent/mutation validation; infrastructure validates the
selected Context, installed provider/acquisition mode, and broker readiness
before broker send. Login/import atomically replace one typed
Context/provider grant and revoke every prior handle. Gateway performs
non-secret introspection before OPA and applies exactly one same-revision
static resolution, Datadog/OpenAI/Anthropic token result, or bounded AWS SigV4 result only
after allow. Gateway makes one upstream attempt. Managed profiles and arbitrary
manifest-selected dynamic execution do not exist. Logout atomically removes
the record and its handles without contacting the provider.
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

The canonical Gateway source label is API V1. Source does not record any owned
image release output. The release workflow builds its multi-architecture index
from one requested revision and injects one Gateway-only lock into every CLI
archive. The base recipe remains
embedded and local-build-only.
Contributor builds use `task build` and
content-addressed local images. `cluster up` compares required and selected
identities before state loading or any Docker call.

## Unsupported outcomes

The deliberate non-goals in [Project Theses](00_theses.md) are not hidden
commands or transport escape hatches. Tobari supports ordinary HTTP/HTTPS
sockets through its guarded transparent path;
it does not forward raw TCP, non-HTTP TLS, UDP, QUIC, recursive DNS, Git SSH, or
certificate-pinned traffic. A client that cannot use the Tobari CA or expose an
unambiguous HTTP authority fails closed.
The experimental Broker slice supports GitHub, Datadog, OpenAI, Anthropic,
Chatwork, and AWS and retains one credential per Context/provider. Owner
manifests may express another single static primary secret only through the
exact HTTPS/header replacement contract and protected stdin import. V1 has no
managed adapter, multiple provider accounts, provider-specific policy
semantics, Git credential helper, manifest-selected helper, or general
provider SDK/plugin executor. Standard tools authenticate natively inside their
Workspace-owned home; that state is neither brokered nor a network grant.
