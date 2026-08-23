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
selected Workspace's own persistent home and is readable by processes in that
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
effect follows the ordinary deny, `review permissions`, decide, and retry loop, and
the decision is available to every process in the Workspace only until the
owning host attachment exits. Docker, Compose, host daemon state, automatic
port discovery, raw TCP, and persistent Host Loopback grants are not part of
this outcome.

The opposite direction is one explicitly reviewed Workspace service. From a
live attachment, `tobari-expose <port>` requests exact Workspace
`127.0.0.1:<port>` access. A separate trusted-host `tobari review services` may choose
`Allow once` or `Deny`; only Allow once makes the owning attachment bind a
random host IPv4-loopback port. The returned URL is exactly
`http://127.0.0.1:<random-port>`. HTTP/1.1 and WebSocket Upgrade relay without
rewriting Host, Origin, redirects, cookies, headers, or content. The helper can
list current-attachment exposures and stop one only with its unchanged opaque
reference. Attachment exit closes the listener and streams. This grants no
Workspace Manifest policy, remembered decision, Host Loopback authority, LAN access,
automatic discovery, requested host port, health probe, or browser opening.

Routine permission guidance presents three layers and no implementation-source
inventory:

1. **Workspace Manifest Access** is the immutable creation-time destination and method
   Boundary together with the routine client traffic admitted inside it.
2. **Remembered Workspace decisions** are trusted-host reviewed Allow and exact
   Deny choices bound to the Workspace Manifest and Workspace until explicit reset.
3. **This-session Host Loopback access** is an exact decision bound to the
   active attachment and removed when its owning host process exits.

Host Loopback is a separate closed policy branch rather than a temporary
widening of Workspace Manifest Access. Ordinary Workspace Manifest policy, native readiness,
remembered decisions, and Advanced Rego neither authorize nor deny it;
Attachment Grants neither authorize nor deny ordinary external traffic.

The user-facing entry point is the current project directory: a Workspace either
exists or does not exist, and the user should not need to manage container
names, network IDs, or policy internals for routine work. `cluster up` remains
the independently invocable owner of shared Gateway and OPA setup, while an
interactive first-use `tobari` composes that exact action after a newly
confirmed Workspace Manifest so a human need not remember the setup sequence.

The primary operating loop is progressive policy learning: a Workspace workload is
denied by default, Gateway records the rejected HTTP effect, including one
operation-type/root-field coordinate for a declared GraphQL endpoint or one
exact signed AWS wire-operation coordinate, and reason
without secrets, the CLI presents a bounded exact proposal and a concrete
trusted-host next action, the user approves the minimum rule, and the same
workload is retried. A learnable denial also gives the agent a fixed host-side
review command, and the human path enters through `review permissions`; machine
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
The host-issued Workspace principal and normalized scheme are retained in denial,
candidate, learned-rule, and audit evidence; an approval made from one current-directory
Workspace cannot be replayed as another Workspace's permission.
Ordinary request bodies are not a policy identity dimension. A body-bearing
POST, PUT, PATCH, or other method is authorized and learned from the same
project, scheme, host, port, method, and path dimensions as a body-free request.
Changing ordinary body content does not create another review item or rule.
For an exact trusted GraphQL endpoint, Gateway derives only the selected
operation type and canonical root fields from one bounded JSON POST body or
one bounded body-free GET parameter set; each root is a separate exact
permission. GET accepts query operations only. Gateway does not expose body content, body hashes,
GraphQL source, operation names, variables, arguments, aliases, fragments,
directives, nested selections, or literal values to OPA, retained evidence,
policy actions, audit output, or CLI output. A request may omit
`Content-Length` only without transfer/content encoding; the fixed 8 MiB
transport cap bounds receipt and Gateway rejects a complete body over 1 MiB
before parsing or policy.
Persisted-query-only and nonempty-extension requests remain unsupported and
fail locally with distinct diagnostics. Policy review derives `not_expected`
for query and `possible` for mutation as evidence; exact operation/root
identity remains the authority.
For a Workspace Manifest with a validated EKS bootstrap, the exact Kubernetes API origin
is also a trusted protocol authority. Gateway derives the API verb and one
canonical resource or non-resource coordinate from method, path, `watch`, and
`dryRun`; object bodies remain opaque. Core resources and CRDs use the same
structural path contract. Read/list/watch, possible mutation, dry-run, and
interactive connect remain visibly distinct. Impersonation headers fail
locally rather than becoming a learnable permission.
Git Smart HTTP discovery and RPC requests are reviewed as an exact repository
path plus `upload-pack` or `receive-pack` service when the standard path/query
or path/media-type contract identifies them. Upload-pack is reported as
`not_expected`; receive-pack is `possible`. Pack bodies and authentication
headers stay opaque, malformed Smart HTTP claims fail locally, and ordinary
HTTP rules cannot authorize a classified Git request.
Distinctive OCI Distribution object routes under `/v2/` are reviewed as an
exact repository, action, and object coordinate. Catalog/tag listing, pulls,
and upload-status checks are `not_expected`; manifest pushes, deletes, upload
steps, and cross-repository mounts are `possible`. A mount retains both digest
and source repository. Blob and manifest bodies, credentials, and raw query
values remain outside policy and audit. The base `/v2/` probe, token routes,
and unsupported `/v2/*` paths remain ordinary HTTP, while a classified OCI
request cannot inherit an ordinary HTTP rule.
For a commercial AWS endpoint using signed AWS Query or AWS JSON RPC, Gateway
derives only wire protocol, SigV4 service, and exact `Action` or signed
`X-Amz-Target`. The dynamic request supplies that coordinate; Tobari carries no
AWS service catalog and does not classify the operation as IAM, resource, read,
create, or write. Query parameters other than `Action` and structural
`Version` validation, and every AWS JSON body field, remain outside policy,
evidence, audit, and CLI output. Unsupported or ambiguous AWS RPC fails before
policy learning or upstream I/O rather than falling back to a broad `POST /`
candidate.
Denial audit retains only the URL path component, never the query or headers.
If that path contains a Tobari handle marker, the whole recorded path is the
literal `/[redacted-auth-handle]`. Structural URL/header handle rejections are
non-learnable and cannot become policy candidates.

Stable macOS CLI releases are distributed through
`tasuku43/homebrew-tap`. The release outcome is not closed at GitHub asset
creation: the exact audited Formula must reach that tap through its reviewed
pull-request boundary. Release preparation is a non-publishing exact-revision
task: it reuses one successful main-push CI result and creates one verified
bounded-retention Actions artifact set without repository-content authority.
Protected publication promotes only that prepared set
after exact run, revision, tag, provenance, and inventory revalidation; it does
not rebuild or reinterpret release subjects. Linux remains supported through
the published archive or source build unless a Linux Homebrew Formula contract
is added explicitly.

## Public vocabulary

- **Tobari:** the product, executable, and ownership adjective. Tobari prepares,
  enforces, and manages Workspaces and installation-local shared services.
- **Project:** the selected canonical host source directory and its contents.
- **Workspace:** one reusable isolated resource selected by a canonical project
  root plus stable Workspace Manifest ID. Its work container is recoverable runtime detail;
  the stable Workspace ID is diagnostic rather than a routine action input.
- **cluster:** the one installation-local Gateway, one OPA, aggregate policy,
  principal registry, and CA lifecycle. The experimental development profile
  adds one locked Auth Broker and provider projection.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **Host Loopback:** the constant `host.tobari.test` HTTP destination whose URL
  port selects the same physical-host IPv4 loopback port for an active attachment.
- **Attachment Epoch:** one unguessable trusted-host identity owned by the
  `tobari` process that established the active Host Loopback route.
- **Attachment Grant:** one exact reviewed Allow or Deny bound to a Workspace Manifest,
  Workspace, Attachment Epoch, target port, and exact HTTP effect. It is
  Workspace-wide for that attachment and is not a learned policy rule.
- **Auth Broker:** the experimental non-root credential-resolution daemon. It owns
  encrypted Workspace Manifest vault access, has no TCP listener, starts locked, and
  exposes separate control and Gateway-only runtime Unix sockets.
- **project root:** the canonical host directory selected from the current working
  directory and mounted directly with the bound Workspace Manifest's immutable
  `read-only` or `read-write` source access. A root below the host home
  is mounted at the same relative path below `/var/lib/tobari`; a root outside
  the host home uses the mirrored `/workspace` path.
- **Workspace home:** a per-Workspace persistent owner-only XDG state directory
  mounted as the work user's home.
- **Tobari image:** the minimal built-in runtime or one locally available
  compatible OCI environment image selected by the Workspace's bound Workspace Manifest for Workspace
  creation and later runtime-container reconciliation. Its tools and bootstrap
  are part of the environment; its image `CMD` is not the Workspace lifetime
  command.
- **Workspace ID:** a generated stable internal identity used for state, exact
  resource labels, and host-issued Workspace-principal bindings. It is diagnostic
  output, not a routine user action input.
- **Workspace principal:** a host-issued binding from one stable Workspace ID and
  stable Workspace Manifest ID to the exact owned Workspace source endpoint and Gateway
  endpoint on that Workspace's dedicated network. The internal schema-V1
  protocol retains the field name `project_id`; that compatibility name carries
  the Workspace ID and never identifies the project root. Caller headers, Workspace Manifest names,
  SNI, request authority, and profile names are not principals.
- **tool-owned authentication state:** files written by a tool or agent below
  one Workspace's persistent home during its own login or configuration flow. It
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
  by a stable Workspace Manifest and provider, acquired through protected non-terminal
  stdin or one purpose-limited reviewed host driver and stored in the
  encrypted Workspace Manifest vault.
- **experimental Workspace credential handle:** a versioned random opaque value bound to one
  Workspace Manifest, project, provider, credential revision, and exact HTTP binding. It
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
- **Workspace Manifest:** one stable host-owned desired Workspace definition
  with a stable opaque ID and human name. Every semantic mutation publishes one
  complete immutable revision. `WorkspaceManifestID + semantic digest` is
  authority; generation is correlation only. Its Boundary records direct
  source access, normalized policy and terminal ceilings, policy mode, and
  native-readiness participation. The same revision contains one exact Runtime
  binding and narrow Workspace defaults grouped by cluster, entry, child
  session, and creation activation boundaries. Experimental Broker state is
  separate and is never a desired/applied Manifest field.
- **default Workspace Manifest:** only the installation selector used when an
  invocation omits `--manifest`; it is not current applied state or shared
  enforcement authority.
- **synthetic default:** the display-only `default` selection returned by an
  omitted-Workspace Manifest read before any Workspace Manifest is persisted. It has no stable ID or
  store authority and cannot bind a mutation; explicitly naming an absent
  `default` Workspace Manifest returns not found.
- **Runtime:** one installation-wide reusable environment definition. A managed
  Runtime owns an editable bounded Docker build-context tree and immutable
  successful semantic revisions; the built-in standard Runtime has one
  compiled revision. Runtime source and snapshots are never Workspace mounts.
- **Workspace Manifest Runtime binding:** one exact stable Runtime ID and semantic revision
  selected by a Workspace Manifest. Its human `name@ordinal` form is review syntax, while
  the persisted ID and SHA-256 revision are authority. Only `manifest runtime
  set` replaces the binding; bound Workspaces adopt it on next entry without
  changing Workspace Manifest identity or persistent home.
- **agent profile:** read-only non-secret shared agent configuration referenced
  by a Workspace Manifest. It is not tool-owned login state.
- **narrow projection:** one fixed Workspace Manifest-owned allowlist of validated
  non-secret scalar fallbacks. The source file, path, directives, executable
  settings, credentials, and undeclared keys never enter the Workspace.

Public CLI schema V2 names Workspace/Manifest lifecycle identity explicitly: lifecycle and policy
results use `workspace_id`, project-directory facts use `project_root`, a
persistent home uses `workspace_home`, Workspace collections use `workspaces`,
and shared status uses `workspace_count`. Internal persistence, Docker labels,
Gateway/OPA input, audits, and Broker protocols retain their schema-V1
`project_id`, `root`, `instance_id`, and label keys because those exact
host-owned contracts already bind the same Workspace identity without exposing
a second public resource. Tobari is not yet public, so ADR 0027 defines these
corrected CLI names as the initial V1; no legacy aliases or compatibility
reader are introduced.

Stable Workspace and Workspace Manifest IDs are not trusted when supplied by a work
container. The host-owned principal registry derives both from the exact
kernel-observed Workspace source endpoint within its verified dedicated
network binding. The registry also retains the exact Gateway endpoint and
rejects duplicate or stale endpoints. Workspace Manifest policy is selected inside the
single OPA from that trusted principal. Learned permissions are Workspace Manifest- and
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
| `help [<command>...] [--format text|agent]` | utility | read | Discover exact command contracts |
| `completion zsh` | utility | read | Generate a thin zsh adapter that asks the current Tobari executable for typed candidates on every completion request |
| `completion candidates --current INDEX WORD...` | utility | read | Return bounded TSV command, flag, finite-value, Workspace Manifest, Runtime, or directory completion records without mutation, Docker, or network access |
| `version [--format text|json]` | utility | read | Print source version/commit, resolver channel, required and selected standard component APIs, and compatibility |
| `doctor [--root PATH] [--format text|tsv|json]` | utility | read | Report read-only host, Docker, configuration, policy, Gateway, port, and residue diagnostics without repairing state |
| `migrate apply [--format text|json]` | act, fixed target | write | Validate and migrate only the enumerated unpublished predecessor, retaining Manifest/Workspace/Runtime identities and standard Workspace homes while quarantining predecessor research authentication for explicit reauthentication |
| `cluster up` | act, fixed target | create | Validate all Workspace Manifest policy inputs and image contracts, reconcile Gateway and OPA, and confirm the exact aggregate policy is active |
| `tobari [--manifest <name>] [-- <command>...]` | act, fixed target plus TTY workflow | create | On first use, review one complete recommended Workspace Manifest draft or choose Customize, compose the exact Manifest/cluster/runtime actions under their own catalog contracts, then choose or create the current directory's Workspace for the explicit or default Manifest, reconcile runtime, and either enter Bash or run one exact foreground child argv; child exit returns to the host with its exact status while the Workspace remains reusable and any attachment-owned route and grant close |
| `status [--manifest NAME] [--format text|json]` | utility | read | Inspect the nearest current-directory Workspace for the explicit or default Manifest, including desired, last-successful applied, observed, adoption, and failure facts; human text leads with Current and Next entry |
| `list [--format text|json]` | utility | read | List local Workspaces with Workspace Manifest, runtime diagnostics, and diagnostic IDs |
| `delete [--manifest NAME] [--force]` | act, fixed target | write | Delete the nearest current-directory Workspace for the explicit or default Manifest, its owned runtime, persistent home, and tool-owned authentication state while preserving project files; `--force` overrides only the attached-session guard |
| `cluster status [--format text|json]` | utility | read | Inspect Gateway/OPA health, required live shared-network joins, registered Workspace/Gateway endpoint agreement, loaded Workspace Manifest count, aggregate revision, current-binary policy/Gateway projection integrity, and recent errors |
| `cluster denials [--tail N] [--format text|json]` | utility | read | Read a bounded typed denial window, exact-rule learnability, policy path, and review command |
| `cluster logs [--component gateway|opa|all] [--tail N]` | utility | read | Read bounded shared logs, including policy-denial evidence, without credential output |
| `cluster down [--purge]` | act, fixed target | write | Remove shared transient resources after every Workspace is deleted; `--purge` additionally removes shared CA and active policy-bundle volumes |
| `policy candidates [--tail N] [--format text|json]` | discover | read | Discover Workspace Manifest/project-scoped pending exact HTTP or GraphQL-root candidates and opaque IDs across the installation |
| `review permissions [--tail N] [--format text|json] [--watch] [--notify auto|osc9|bel|off]` | discover plus TTY fixed-target apply | read, or one confirmed write | Review the installation-wide Permission Inbox; a raw TTY can stage exact decisions from the list, inspect template scope, and apply the reviewed set; `--watch` refreshes bounded snapshots and remains open after Apply, while `--notify` selects its terminal-emulator cue and redirected or JSON output remain read-only |
| `review services` | discover plus TTY reference-bound actions | read, or one confirmed create/write | In a separate trusted-host terminal, review a fresh live Service request; Allow once or Deny is immediate and attachment-local, while redirected output is read-only |
| `service requests` | discover | read | Return one fresh exhaustive snapshot of pending service requests from live attachment owners with opaque request references |
| `service allow --id ID` | act, reference bound | create | Revalidate one pending request and create one attachment-owned random IPv4-loopback exposure |
| `service deny --id ID` | act, reference bound | write | Resolve one pending request without creating host access |
| `policy allow --id ID` | act, reference bound | write | Test, record, and activate one exact observed permission |
| `policy deny --id ID` | act, reference bound | write | Test, record, and activate one exact project-bound rejection |
| `policy rules [--format text|json]` | discover | read | List every Workspace Manifest-scoped CLI-owned learned Allow and exact Deny decision; on a TTY, reset one explicitly |
| `policy reset --id ID` | act, reference bound | write | Remove one learned decision and leave its effect at default deny |
| `manifest list [--format text|json]` | utility | read | List persisted Workspace Manifests and identify the installation default; human text summarizes effective Access, exact Runtime selection, and any action marker |
| `manifest show [--name NAME] [--details] [--format text|json]` | utility | read | Inspect one Workspace Manifest's immutable Boundary, complete desired revision, exact Runtime binding, activation slices, stores, and native Workspace-owned authentication mode |
| `config shell [--variable COLORTERM\|NO_COLOR\|PS1\|TERM] [--source default\|inherit\|literal] [--value VALUE] [--manifest NAME] [--format text\|json]` | act, fixed target | write | Publish one complete desired Manifest revision containing the updated shell-session defaults; later child sessions resolve it without rewriting Workspace home |
| `config git [--source default\|inherit\|literal] [--name NAME] [--email EMAIL] [--manifest NAME] [--format text\|json]` | act, fixed target | write | Publish one complete desired Manifest revision containing an atomic Git commit-identity session fallback; later Workspace entry resolves it without rewriting Workspace home |
| `config bootstrap aws [--profile NAME] [--refresh] [--remove] [--manifest NAME] [--format text\|json]` | act, fixed target | write | Normalize one strict secret-free host AWS IAM Identity Center profile for future Workspaces, refresh it after a semantic diff, or remove the future recipe; existing Workspace homes never change |
| `config bootstrap kubernetes eks [--kube-context NAME] [--refresh] [--remove] [--manifest NAME] [--format text\|json]` | act, fixed target | write | Compose one strict AWS CLI-generated host EKS context with the Manifest AWS profile, refresh it, or remove only EKS; no credential, arbitrary exec, network authority, or existing Workspace home changes |
| `manifest create [--copy-from NAME] [--name NAME] [--runtime RUNTIME] [--mode guided\|advanced] [--source-access read-only\|read-write] [--native-readiness enabled\|disabled] [--bootstrap-aws-profile NAME] [--bootstrap-eks-context NAME] [--format text\|json]` | act, fixed target | create | Create a fresh generation-1 Workspace Manifest from complete direct input or a reviewed exact immutable current Manifest revision; copy is one-time, independent, and records no lineage or lower-lifetime state |
| `manifest delete --name NAME [--format text\|json]` | act, fixed target | write | Delete one unused non-default Workspace Manifest and its exact owner stores while preserving Workspaces, project files, and shared runtime images |
| `manifest default set --name NAME [--format text\|json]` | act, fixed target | write | Change only the installation default Manifest used when a later invocation omits `--manifest`; do not mutate existing Workspaces or reconcile Docker |
| `manifest runtime set [--runtime RUNTIME] [--manifest NAME] [--format text\|json]` | act, fixed target | write | Publish one complete desired Manifest revision with an exact `standard` or ready `NAME@ORDINAL` Runtime binding; bound Workspaces adopt it only at their next explicit entry |
| `runtime list [--format text\|json]` | discover | read | List the exhaustive installation-wide Runtime catalog, stable Runtime references, and each ready head revision |
| `runtime show --name NAME [--format text\|json]` | discover | read | Inspect one Runtime's stable reference, managed source path, and complete successful revisions |
| `runtime history --name NAME [--format text\|json]` | discover | read | Show one Runtime's stable reference and ordered immutable successful revision history |
| `runtime create [--copy-source-from RUNTIME] --name NAME [--format text\|json]` | act, fixed target | create | Create one standalone owner-only managed Docker build-context source from the standard starter or another managed Runtime's current editable source, with a fresh Runtime ID, empty history, no lineage, and no Manifest or Workspace change |
| `review runtimes [--format text\|json]` | discover plus TTY reference-bound action | read, or one confirmed write | List the exhaustive Runtime catalog; trusted interactive text offers only managed Runtime actions and uses one confirmation for either a selected build or exact interrupted restore recovery, while redirected and JSON output remain read-only |
| `runtime build --id RUNTIME_REF [--format text\|json]` | act, reference bound | write | Re-resolve one stable managed Runtime ID under the lifecycle and store locks, then snapshot, build, validate, and append one immutable semantic revision without changing any Workspace Manifest |
| `runtime restore --id RUNTIME_REVISION_REF [--format text\|json]` | act, reference bound | write | Rebuild one exact missing or pruned managed revision from its retained immutable source, publish only an exact digest match, and preserve Runtime history, Workspace Manifests, and Workspaces; an already-available exact revision performs no durable write |
| `runtime prune dry-run [--format text\|json]` | discover | read | Produce one exact opaque prune-plan reference from a complete coherent installation observation, listing every eligible unused owned image tag, protection, blocker, preserved source/snapshot byte count, and bounded Docker observation without creating state or changing Docker |
| `runtime prune apply --plan RUNTIME_PRUNE_PLAN_REF --confirm=prune [--format text\|json]` | act, reference bound | write | Revalidate and apply one unchanged reviewed plan, removing only exact Tobari-owned unused image tags while preserving Runtime source, immutable snapshots, revision history, Workspace Manifests, Workspaces, homes, IDs, and shared image content |

Bare `tobari review` is a pure Catalog namespace listing with exactly the
public task leaves `permissions`, `runtimes`, and `services`; it performs no task read or
mutation and has no registered selector handler. `review permissions` retains
bounded, durable staged Apply. `review services` retains fresh exhaustive
discovery and immediate attachment-local Allow once or Deny. The pre-public
`policy review` path and registered root `review` selector have no alias or
fallback; persisted policy and attachment state need no migration.

Every Tobari-controlled base Runtime image builds a dedicated Linux Workspace
helper from the checked source/Catalog closure with a pinned builder. The host
extracts and verifies that engine-native helper, then mounts it read-only only
while attached. Its main hardcodes the helper Program; changing `argv[0]` or
copying it cannot expose host commands:

| Helper command | Role | Effect | Outcome |
|---|---|---|---|
| `tobari-expose <port>` | act, fixed target | create | Request trusted-host review for one exact non-privileged Workspace-loopback port; wait for the confirmed result and return its opaque exposure reference and exact stop command |
| `tobari-expose list` | discover | read | List the exhaustive current-attachment exposure inventory and unchanged opaque references |
| `tobari-expose stop <exposure-ref>` | act, reference bound | write | Close one exact current-attachment listener and its active relays without stopping the Workspace service |
| `tobari-expose help [<command>...] [--format text\|agent]` | utility | read | Discover only the helper program's exact contracts |

The unsupported experimental development profile built by `task build:dev`
additionally exposes `serve [--no-open]`. It runs one foreground IPv4-loopback
Operator Console for typed cluster, Workspace, Permission Inbox, and learned-rule
inspection; it may open the host browser, stages decisions without authority,
and delegates one confirmed reviewed set to the canonical fixed-target Apply.
The standard development binary and release archives omit this command.

For the CWD lifecycle commands `tobari`, `status`, and `delete`, one non-empty
invocation Workspace Manifest may appear before or after the command path: for example,
`tobari --manifest toolbox status` and `tobari status --manifest toolbox` are
equivalent. Omission resolves the default Workspace Manifest without changing it;
duplicate or explicit-empty placement is invalid. After name resolution, the
stable Workspace Manifest ID is authoritative for the remainder of the operation.

The root command is interactive and requires a TTY on stdin, stdout, and stderr.
It does not silently create state in a non-interactive context. With no
persisted Workspace Manifest and no explicit `--manifest`, it first validates the canonical
project root and shows one recommended draft: direct read-write project effect,
effective routine/other/private Access, `standard@1`, no host import, and Bash
or the safely projected direct executable. Start revalidates that the Workspace Manifest
collection remains empty and `default` absent under the creation lock, then
invokes canonical Workspace Manifest creation. A concurrent change fails with exact
read-only `manifest list` recovery and is never adopted or overwritten.
Customize opens the same ordinary six-stage Workspace Manifest wizard with recommended
values prefilled. Cancellation, EOF, rendering, or terminal failure before
Create changes no Workspace Manifest, host configuration, cluster, Docker, Workspace, or
network state.
After either review path chooses continuation and before Workspace Manifest creation, root
runs the closed generic Workspace-start readiness profile: Docker CLI, selected
Engine version, selected Docker Context, and Compose v2. Engine major versions
below 24 fail as unsupported. Any failed or invalid observation returns one
fixed safe fault pointing to `doctor` and performs zero Workspace Manifest, cluster,
Workspace, network, or Docker mutation. The profile neither identifies nor
manages the Docker provider. Standalone `manifest create` remains independent
from Docker readiness.
After confirmed Workspace Manifest creation it emits that durable success, performs the
exact catalog-owned `cluster up` action without another confirmation, and
retains the Workspace Manifest if cluster reconciliation fails. When shared services are
ready, root proceeds directly to Workspace selection and entry with the exact
Runtime revision reviewed during Workspace Manifest creation. Runtime customization is a
separate prepare-first flow: `runtime create`, edit the managed source tree,
`runtime build`, then select the ready revision during Workspace Manifest creation or with
`manifest runtime set`. Existing persisted Workspace Manifests never receive an automatic
upgrade prompt. If their shared projection is absent, stopped, or
invalid, the same interactive root invocation composes exact `cluster up`
before Workspace mutation. When the
canonical current directory is below one or more indexed Workspace roots, the
command presents an English selector ordered nearest-first. Arrow keys and
Enter choose an existing Workspace; `n` chooses explicit creation at the
current directory; `q` or Escape cancels. If raw terminal mode is unavailable,
the same choices use numbered line input without adding a terminal module or
shell subprocess. Candidate status and path text remain meaningful without
color. Programs inside a Workspace can mutate the explicitly mounted root; that
delegated capability is a documented security property rather than an
undeclared Docker mutation by the CLI.

An optional direct command begins only after the required positional-only
marker: `tobari [--manifest NAME] -- COMMAND [ARG...]`. A bare `--`, an empty
executable, or child argv without the marker fails before setup or Workspace
mutation. Tobari neither invokes a shell nor expands, joins, or reparses the
argv; order, duplicates, dash-prefixed values, and explicit empty arguments are
preserved. The command owns the foreground terminal and signals for that exec
session. Its exit returns to the host shell rather than entering Bash, and its
exact status is returned without stopping the fixed Workspace lifetime process.
Neither Bash nor a direct child reserves a Tobari input prefix. In particular,
`Ctrl+]` is forwarded to the child unchanged; trusted-host Permission Inbox
review runs through `tobari review permissions` in a separate host terminal.

## Input and path contract

- The current working directory is expanded and canonicalized on the host
  before state or Docker calls. An exact indexed root is reused directly. When
  only containing ancestor roots exist, `tobari` lists every valid root
  nearest-first and offers explicit creation at the current directory; it never
  creates a nested Workspace implicitly.
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
- Each `(canonical root, stable Workspace Manifest ID)` identifies at most one Workspace.
  Repeated or concurrent creation for that pair is rejected as already
  existing; the same root may have independent Workspaces in different
  Workspace Manifests. An explicit Workspace Manifest never changes the default Workspace Manifest.
- Project-root selection rejects the filesystem root, the user's home and its
  ancestors, and any path overlapping XDG Tobari configuration, state, or
  shared-profile management directories, Docker sockets, or Docker management
  paths. A repository containing policy source remains allowed; only the
  trusted active policy/configuration paths are protected.
- An explicit create choice uses the canonical current directory as root.
  Project moves and copies are not inferred or recorded in the project tree.
- The selected root is the only project directory mounted from the host and
  uses the bound Workspace Manifest's immutable `read-only` or `read-write` access. When
  it is below the host home, the container target preserves the relative path
  below `HOME=/var/lib/tobari`; for example, a host root of
  `$HOME/path/to` enters at `/var/lib/tobari/path/to`. A root outside
  the host home retains `/workspace/<canonical-root-without-leading-slash>`;
  thus a root at `/work` and a host CWD of `/work/root` enter at
  `/workspace/work/root`. Read-only applies only to this direct live bind: it
  is not a snapshot, host or same-root read-write Workspace Manifest changes remain
  observable, no writable source alias exists, and Workspace home and tmpfs
  remain writable. Tobari never mounts the host home wholesale.
- Same-root Workspaces in different Workspace Manifests and parent/child roots may run
  concurrently. Runtime, home, network, policy, and broker handles follow
  their Workspace/Workspace Manifest boundaries, but overlapping host-file writes are visible
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
  the official runtime base for an uncustomized Workspace Manifest; custom images still
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
  accept an existing Workspace Manifest name and installed provider ID; the experimental
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
  compatible Workspace Manifest image, runs exact Claude Code 2.1.220 native account login,
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
  reports bounded Workspace Manifest-credential validation in progress. Claude's reviewed prompt owns its
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
  failures, and each preserves the previous Workspace Manifest credential. A capture
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
  byte is read. Non-terminal bytes are read only after public Workspace Manifest/provider
  argument, intent, and mutation validation; infrastructure then validates the
  selected existing Workspace Manifest, installed provider/acquisition mode, and broker
  readiness before broker send. The credential is never a positional/flag
  value or Tobari environment input. Every successful auth mutation requires
  existing Workspaces to be re-entered before their environment or
  handle projection can change. Reviewed built-ins may use bounded dynamic
  records, Datadog/OpenAI/Anthropic refresh, AWS signing and the private companion, or
  the OpenAI supplemental header. Managed profiles and owner-selected dynamic
  behavior remain absent.
- `runtime create` creates one installation-owned, owner-only `source/` tree.
  `--copy-source-from standard` uses the built-in Dockerfile starter;
  `--copy-source-from NAME` copies
  that managed Runtime's current editable source through the same bounded,
  drift-checked stream contract used by build. The copy preserves relative
  paths, bytes, and owner permission bits, receives a fresh Runtime ID and
  empty history, and retains no source identity, lineage, or inheritance. Scripts, package manifests, and configuration
  files, including host-acquired private binaries, may be added beside it under
  one regular-file contract: at most 1,024 files, 256 directories, 32 MiB per
  file, and 64 MiB total. The source root and all children have no group/other
  permission bits; files may retain owner execute. `runtime build` streams the
  complete semantic tree into a private immutable snapshot while hashing the
  copied bytes and builds only from that snapshot.
- `runtime build --id` is a direct reference-bound action. The separate
  `review runtimes` discover command requires interactive stdin and stderr
  before it can offer an action; redirected or JSON invocation returns the
  exhaustive catalog and remains read-only. Interactive Build Review selects
  only a managed Runtime and shows its source, current successful head or draft
  state, and the fact that no Workspace Manifest changes before handing its
  unchanged stable reference to `runtime build --id`. `manifest runtime set`
  retains direct and omitted-input Review modes. Workspace Manifest Runtime
  Review shows the exact persisted Workspace Manifest,
  current binding, selectable `standard@1` or successful `NAME@ORDINAL`
  revisions, and next-entry timing. Its unchanged editing state offers Runtime
  selection, unlocked Workspace Manifest selection, or cancellation without presenting an
  Apply action. Selecting a different Runtime enters a dedicated Review state
  that renders the exact old-to-new binding and offers Apply, Back to the
  Runtime list, or cancellation. When Workspace Manifest is omitted, editing starts from
  the default Workspace Manifest and may select another persisted Workspace Manifest; an explicit
  Workspace Manifest remains fixed. Final Build or Apply reaches the same application
  mutation boundary once. Back, cancellation, unchanged selection, and read,
  validation, or terminal failure perform zero mutation. Review prompts use
  stderr and the confirmed complete report uses stdout.
  Successful human text closes the prepare-first handoff without composing the
  mutations: create reports the exact named build command, build reports the
  exact ready `NAME@ORDINAL` selection command, and Workspace Manifest selection reports
  the exact next Workspace entry command. For the default Workspace Manifest that entry is
  the ordinary CWD-first `tobari`; a non-default Workspace Manifest remains explicit.
- Successful managed Runtime list, show, history, build, and review results
  expose an opaque `runtime-revision` reference for each eligible immutable
  revision. Built-in `standard` never exposes that reference because it is not
  a managed retirement target. `runtime restore --id` consumes the emitted
  reference unchanged, revalidates the complete lifecycle observation and
  retained source, and rebuilds with mutable base refresh disabled. Publication
  requires the rebuilt content digest to equal the recorded revision authority.
  Restore never appends or rewrites history, changes a Workspace Manifest, or
  changes a Workspace. An interrupted restore remains fail-closed and resumes
  through the same one-confirmation `review runtimes` recovery path.
- Direct `config shell` changes one allowlisted shell-presentation policy in
  the explicit or default Workspace Manifest. Its terminal editor may stage several
  distinct rows and commits the complete change set with one atomic write.
  `default` removes its override;
  `inherit` reads that exported variable from the host process launching each
  future `tobari` session; `literal` requires `--value` and preserves an
  explicit empty value. `--value` is invalid for the other sources. The fixed
  inventory is `PS1`, `TERM`, `COLORTERM`, and `NO_COLOR`; it excludes
  `PATH`, `HOME`, `BASH_ENV`, `ENV`, `PROMPT_COMMAND`, credential variables,
  and arbitrary names. A new V1 Workspace Manifest selects `PS1=inherit`. When the
  launcher has no exported `PS1`, the built-in `\h:\w\$ `
  prompt remains.
  Running sessions are unchanged, and no host startup file is sourced or
  mounted. Literal values are ordinary owner-only configuration and must not
  contain secrets.
- `config git` changes one atomic `user.name`/`user.email` fallback in the
  explicit or default Workspace Manifest. `default` removes Tobari's fallback; `inherit`
  resolves only a complete pair from host-global Git configuration for each
  stable Workspace root during reconciliation; `literal` requires both
  non-empty `--name` and `--email`. New Workspace Manifests use `default`,
  and an absent or incomplete inherited pair adds no fallback. The projected
  system-scope value is lower precedence than Workspace global and
  repository/worktree configuration. No Git file/path/include directive,
  credential helper, token, HTTP header, SSH command, signing setting, hook,
  alias, URL rewrite, filter, proxy, or arbitrary key is projected.
- For both `config` commands, omitting the entire setting group in text mode
  uses terminal stdin and stderr to show the selected Workspace Manifest, complete current
  state, pending changes, and Apply/Cancel controls on one screen. Shell stages
  multiple rows; Git stages one atomic identity source. Text mode requires both the
  success and error formats to be text. Supplying any setting input selects
  direct mode and requires a complete valid group; partial input never prompts.
  Redirected or JSON invocations require direct mode. An explicit empty
  Workspace Manifest is invalid rather than equivalent to omission. After reading current
  state, the wizard binds Apply to that returned Workspace Manifest name, so another
  process changing the default selector during review cannot retarget
  the write. Cancellation and every wizard validation or terminal failure
  perform zero mutation.
  Prompts use stderr and the confirmed complete Workspace Manifest report uses stdout.
- `manifest create` has one staged human flow and one complete direct mode. On
  interactive text success/error streams, supplied values prefill their
  corresponding stages and those stages are skipped on the initial pass;
  omitted stages remain reviewed. When persisted Workspace Manifests exist and `--copy-from`
  is omitted, a dedicated Copy source step precedes Name, initially selects the
  default Workspace Manifest, and also offers Tobari recommended settings. Copy initializes the
  Boundary, exact Runtime binding, shell/Git defaults, and future-Workspace
  bootstrap into one standalone draft; it copies no Workspace, authentication,
  remembered permission, Attachment, applied/failure/observed state, default
  selection, or lineage. Changing the source after editing requires an explicit whole-draft reset. With no persisted
  Workspace Manifest, the chooser is skipped. The ordinary six-stage path is name, filesystem, network, Runtime, Workspace
  bootstrap, and Review & Create. `--name sre3` therefore starts at Filesystem
  with `sre3` already bound rather than creating immediately.
  Network first renders reviewed routine Claude Code/Codex traffic, every
  standard and extension-method effective decision, and the private/unsafe
  destination ceiling. Customization alone exposes default, inherited, and
  override sources plus inherit/reset controls. Runtime always presents
  `standard@1` and every ready managed revision after Network, even when
  standard is the only choice. Workspace bootstrap always follows Runtime and
  defaults to not configured. Entering that step performs no host read; only
  an explicit Configure from host choice discovers compatible AWS IAM Identity
  Center and optional Amazon EKS settings. Review mirrors the chosen
  filesystem, network, Runtime, and future-Workspace bootstrap boundary; it can
  scroll and edit one section without replaying later steps. On a raw-capable terminal,
  all stages share one alternate-screen session; Back preserves staged values.
  A terminal without the reviewed raw-mode support uses the bounded line-mode
  equivalent. Explicit Create performs one
  mutation and cancellation from any step performs none. Redirected or JSON
  creation never prompts and, without `--copy-from`, requires explicit `--name`, `--runtime`, `--mode`,
  `--source-access`, and `--native-readiness`; Workspace bootstrap is an
  optional explicit addition whose omission means not configured. An exact
  `--copy-from NAME --name NAME` is complete because omitted work-mode values come
  from the reviewed exact immutable source revision; supplied values replace the corresponding
  draft values. Partial
  machine input fails before mutation instead of applying hidden defaults.
  Method Deny removes Workspace Manifest-policy positive baseline entries for that method
  from the new immutable snapshot rather than leaving an invalid or misleading
  unreachable grant. Final review also identifies the exact ready Runtime
  selected in the manifest. Redirected or JSON incomplete creation fails
  before mutation. Standalone success points to the root entry while preserving
  the created Workspace Manifest explicitly when it is not current; root entry owns the
  remaining human setup sequence.
- `runtime build` is the explicit exception to the no-implicit-pull rule. It
  runs a host Docker build using only the immutable snapshot of the selected
  installation Runtime source tree as build context; Docker may obtain a
  missing base image for this explicit build.
  Tobari requests plain BuildKit progress and forwards the visible-projected
  Docker stdout and stderr diagnostic stream to host stderr while the build
  runs, including in non-TTY environments. The diagnostic stream retains the
  concrete failed step and Docker/BuildKit error separately from Tobari's
  stable structured mutation fault; a user does not need to rerun an equivalent
  Docker command to obtain the upstream failure.
  Tobari validates the resulting image against the same runtime contract,
  records its immutable local image digest, and appends that successful
  immutable revision without selecting it in any Workspace Manifest. Existing Workspace Manifest
  bindings remain in force until a separate `manifest runtime set` succeeds.
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
  bundle installs its client. A selected custom Workspace Manifest runtime must provide
  the exact compatible client before its login can run. The released CLI
  carries the pinned combined-base recipe and builds it locally when absent.
  The base workflow validates Linux amd64 and arm64 with cache-only output;
  neither it nor the protected release workflow publishes an OCI image.
- The pinned client versions and `builtin/agent-ready` exact and semantic effect catalog are
  one compatibility contract. Its compile-time `claude_ready`, `codex_ready`,
  `gh_ready`, custom-runtime `twg_ready`, and custom-runtime `pup_ready`
  bundles are projected by the
  installed binary into exact native-authentication effects for every existing
  and future agent-ready Workspace Manifest;
  GitHub CLI additionally receives only GraphQL `query` root `viewer`, and TWG
  CLI receives only exact site inventory, stable CLI manifest, token revoke,
  and GraphQL `query` root `me`, at their declared exact API endpoints;
  pup receives only exact US1 DCR registration and token exchange/refresh.
  The bundles are not runtime selectors or
  executable identity. Updating a pinned client or its independent readiness
  contract revision requires reviewing its artifact lock where changed, exact
  effects, host interactions, and core control-plane effects. The Claude and
  Codex artifact locks belong to the combined base and create no second image
  or authority boundary.
- Project metadata does not select or alter the runtime image. Workspaces use
  their permanently bound Workspace Manifest image when created and again when their runtime container
  is reconciled by root entry; all selected images still pass the same
  compatibility checks before Docker mutation. If an existing Workspace's
  bound Workspace Manifest image changes, the next matching-Workspace Manifest `tobari` entry preserves the
  Workspace home and root record, recreates only the work container when the
  runtime spec changed, and records the new image only after reconciliation
  succeeds.
- Shared cluster mutations use one command-bound `tool_local` target and are
  never performed by the root `tobari` operation.
- CWD-local lifecycle operations use one command-bound `tool_local` current
  directory target plus the explicit or default Workspace Manifest selector; they do not
  accept an ID or root selector. Workspace Manifest selection is resolved to a stable ID
  before CWD/Workspace or Docker observation and never guesses a different
  same-root Workspace. A force-delete preview binds the subsequent mutation to
  the stable Workspace Manifest ID it displayed and fails closed if that authority changes.
- `delete` removes that nearest Workspace Manifest-bound target without `--force` when no session is
  attached. An attached session returns `project_session_attached` and leaves
  state untouched; `--force` is the explicit override.

## Output and exit contract

Human output is concise text. The canonical public machine-output inventory is:

<!-- public-cli-json-schemas:start -->

| Surface | Envelope | Schema |
| --- | --- | ---: |
| Structured error | `error` | 2 |
| Agent help (`view: index` and input-selected `view: scope`) | `commands` | 1 |
| Version | `build_identity` | 1 |
| Doctor report | `report` | 1 |
| Installation migration | `migration` | 2 |
| Workspace Manifest list | `workspace_manifests` | 2 |
| Workspace Manifest report (show/create/default/config/runtime results) | `workspace_manifest` | 2 |
| Runtime list | `runtimes` | 1 |
| Runtime report (show/create/history/build results) | `runtime` | 1 |
| Runtime restore result | `runtime_restore` | 1 |
| Runtime prune plan | `runtime_prune_plan` | 1 |
| Runtime prune result | `runtime_prune_result` | 1 |
| Workspace Manifest deletion | `workspace_manifest_deletion` | 2 |
| Cluster status | `cluster` | 1 |
| Cluster denials | `denials` | 1 |
| Policy candidates | `policy_candidates` | 1 |
| Policy review | `policy_review` | 1 |
| Policy rules | `policy_rules` | 1 |
| Workspace list | `workspaces` | 2 |
| Workspace status | `status` | 2 |
<!-- public-cli-json-schemas:end -->

Workspace status JSON always reports the selected Workspace Manifest ID/name, logical
existence, runtime diagnostic, attachment observation, and exact Workspace Manifest-bound
next argv. Routine human status leads with the Workspace result, root, Workspace Manifest,
exact Runtime selection plus health, session state, any required action, and
the exact next command; healthy IDs, home paths, revisions, and bootstrap state
remain in JSON rather than the default human view. Workspace Manifest reports include a
complete four-item shell-environment
inventory, complete Git identity policy, and authentication mode
`native_workspace`. Native agent credentials are created and persisted by the
agent CLI inside the Workspace home and never appear in CLI output.
Human `manifest list` text renders one result-first card per Manifest definition: name and
default marker, effective source/routine/other/private Access, exact Runtime
selection, and an action marker only when required. The synthetic default says
that recommended defaults are not saved. Human `manifest show` text defaults to
an outcome-first lifecycle summary: fixed Boundary and effective request
posture; the exact Runtime binding adopted on next entry; Workspace defaults
split between later entries/sessions and new Workspace homes only; separate
Workspace-tool login ownership; an exact detailed-inspection command; and the
Workspace Manifest-preserving next action. `--details` renders the same single typed result
with complete values under those lifecycle headings plus Workspace Manifest identity and
stores/revisions. Human new-Workspace setup names AWS and Kubernetes EKS rather
than presenting bootstrap as current Workspace state. It performs no second read. Schema-2 JSON
is already complete, is byte-identical with or without `--details`, and remains
the automation contract.
Successful text `manifest create` uses that same Workspace Manifest-summary row structure
for the confirmed typed result. Its heading states creation, its explicit
cluster row preserves whether reconciliation is required, its details command
opens the newly created Workspace Manifest by name when needed, and its root continuation
selects that same Workspace Manifest when it is not the default. It does not infer
authentication state that the create task did not observe. Schema-2 JSON
remains unchanged.
Unconfigured cluster resources are `null`; unavailable
observations use declared finite values and never an empty-string sentinel.
The infrastructure/doctor label `linux_xdg_file` is not a public
auth or cluster JSON enum. Their items associate Workspace Manifest name,
stable Workspace Manifest ID, Workspace ID, safe project root, HTTP effect, observation data
where applicable, and one opaque mutation reference. Agent help uses the V1
catalog schema, including recursive field declarations, typed completion
sources, and executable success/error invocation forms in scoped help.
`completion zsh` and `completion candidates` are ordinary public read-only
utilities because the shell invokes them directly. Candidate output is a
bounded two-column TSV protocol with `candidate` or `directive` in column one;
it is not a parser for human output or another command's JSON.
Successful data is stdout;
failures are stderr.

Structured error schema 2 retains `kind` and command-specific `code` as the
causal identity and requires `phase` plus `change_state`. Phase is one of
`precondition`, `observation`, `mutation`, `verification`, `attachment`, or
`presentation`. Change state is one of `not_applicable`, `none`, `partial`,
`confirmed`, or `unknown`. The owning layer may publish only what it proves:
pre-action failure is `none`, reads are `not_applicable`, unclassified
post-action results are `unknown`, and a confirmed mutation remains
`confirmed` if final output fails. Catalog declarations own these facts and
the exact next actions. A mutation marked `partial`, `confirmed`, or `unknown`
must first recover through a declared read-only reconciliation command.

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
advisory only; the interactive `review permissions` queue is the human entry point.
It stages unchanged opaque review-item references only from typed detail screens and uses
one final `policy apply-reviewed` fixed-target action to revalidate and activate
one Workspace Manifest's set. Apply or discard is required before switching Workspace Manifest so the
source change remains one atomic domain-generation replacement. `policy rules` is the
current learned-decision inventory; its TTY reset flow delegates one explicit
opaque reference to `policy reset`. Redirected and machine-readable review and
inventory remain read-only. The Permission Inbox groups candidates by their
validated stable Workspace Manifest and Workspace identities, renders the Workspace Manifest/project-root scope
once per group, and leads each selectable row with the exact HTTP effect or
typed `{id}` template and its evidence count. A compact selected-effect preview exposes the latest retained
observation and denial reason before detail inspection. Matching display names,
paths, order, or indentation do not merge distinct typed identities. The raw
list stages exact Allow or Deny by unchanged typed ID, clears one staged row,
and advances only to a later undecided row without wrapping. Its selection
marker has a fixed column, and its visible decision-state column uses only the
width of `Allow exact` so every HTTP effect starts at the same column and never
moves as staging or refresh changes the visible states. Template rows use the
compact list-only labels `Review {id}` and `Allow {id}`; detail and final Apply
retain the full template explanation. An exact detail
offers the same exact decisions; a template
detail states that unseen values are included and offers Allow template, Allow
observed exact, and Deny pending exact. The chosen action is staged without a
second yes/no prompt. Only the final Apply delegates
the reviewed set to the mutation boundary. Apply is advertised only for a
non-empty staged set, shows one final ordered typed review, and requires an
explicit confirmation. Refresh preserves choices by candidate ID and drops
stale IDs rather than matching labels. `--watch` requires human text on an
interactive raw terminal, automatically repeats the same bounded read with a
one-second interval and exponential backoff capped at eight seconds, preserves
the last valid snapshot on refresh failure, and continues with a fresh snapshot
after confirmed Apply. Between Apply operations it owns one alternate-screen
frame; an unchanged successful timer refresh performs the read but emits no
repaint. Stopping watch is a successful monitor stop. It never
retries an HTTP request or creates an agent-side authority channel. The optional
`--notify` value defaults to `auto`, requires `--watch`, and selects a fixed
trusted ASCII cue: explicit `osc9`, `bel`, or `off`, while `auto` uses OSC 9
for exact iTerm2 identity or for the conjunction of non-empty protected cmux
workspace and surface identities, and otherwise falls back to BEL. The
initial snapshot, refresh failures, stale-only changes, and previously seen ID
reappearance do not notify; one successful refresh coalesces every new ID into
at most one cue. Tobari configures no OS, tmux, SSH, or terminal passthrough,
and no Workspace Manifest, host, path, reason, or other denial evidence enters the control
payload. A failed cue leaves watch active. Confirmed
output carries the active OPA
revision plus each ordered Workspace Manifest/project/effect/stored-rule decision and
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
The attached Workspace shell is a separate terminal presentation surface:
when its stdout is an interactive terminal and `NO_COLOR` is absent, Tobari
may add fixed syntax colors to one bounded, complete JSON object/array or a
conservative YAML mapping/sequence. This projection is color-only: it keeps
the child's bytes, whitespace, ordering, and visible stream content intact,
does not pretty-print or reindent, and adds no styling to stderr. Incomplete,
invalid, oversized, control-bearing, ambiguous, or ordinary output passes
through unchanged. Redirected or machine-readable output never enters this
projection, and escaped controls inside a structured string remain data. The
presentation relay never parses or reserves input and is not a terminal
multiplexer; every child-input byte remains pass-through.
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
Workspace count, and policy path; configured/running booleans and the
full recent diagnostic remain available in JSON or failure detail. A
successful `cluster up` additionally points to the next `tobari`
command.

`manifest default set` reports `default_updated`; it changes only the omitted-Workspace Manifest
  default regardless of cluster state. `manifest create` reports
`requires_reconcile` when a configured cluster does not yet contain the new
Workspace Manifest projection; an explicit `cluster up` is required. Neither command
starts Docker.

Project runtime diagnostics may report `incomplete` when a durable root index
survives without its instance state. This preserves logical existence for safe
cleanup, prevents runtime recreation, and directs the user to delete the exact
current-directory Workspace before creating it again.

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

`status` and `delete` resolve one stable Workspace Manifest before the nearest canonical
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
| other from root entry | Exact Bash or direct-command child process exit status when Docker started the interactive work process |

Commands use complete delivery. `list` is exhaustive for local logical state at
one observation point. `status` is a CWD-local scalar observation; cluster
status is exhaustive for the shared cluster scope. Logs are
a bounded recent window of 1 through 10,000 lines per selected component.
Denials are a fully delivered typed projection from the requested bounded
Gateway-line window. A denial-shaped record that cannot satisfy the strict
Workspace Manifest/project-bound typed contract is isolated rather than failing the whole
window, and `unparsed_lines` reports how many such records were skipped without
reflecting their untrusted contents. An empty `items` collection means no valid
denial occurred in that window, not exhaustive history; a nonzero unparsed
count preserves the distinction between a fully interpretable empty window and
one containing unprojectable evidence.
Policy candidates, review, and tail are bounded by the same retained
Gateway-line window and omit effects already covered by learned allow rules,
baseline deny rules, or exact learned deny rules. Baseline and exact denies
remain available as audit evidence but never become pending queue items.
Within that window, candidates aggregate by exact Workspace Manifest identity, project
principal, host, port, method, normalized path, and optional GraphQL operation
type/root field or AWS wire protocol/service/operation. Reason, status, request ID, timestamp, and broker-handle
evidence do not create a second permission identity. The
candidate retains the latest matching evidence and reports the required number
of matching retained observations. Concurrent identical audit records therefore project to one pending
item without a separate mutable inbox write. A current learned Allow or exact
Deny remains the resolved history and is never updated by discovery. After an
explicit reset, retained matching evidence may produce the same stable pending
candidate ID again.
For interactive review only, typed domain logic also considers current exact
Allows as evidence. Two distinct compatible HTTP paths with the same Workspace Manifest,
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
installation, omitted Workspace Manifest selection may return `synthetic_default` with
JSON `null` for Workspace Manifest ID and stores; it does not create
`contexts/default.json` or a manifest. Persisted state must use the exact
Domain Model V1 contract. Missing,
malformed, or unsupported-version state fails closed instead of becoming
synthetic state.

- `contexts/<name>/context.json`: host-owned schema-2 Workspace Manifest with a
  stable UUIDv7 Workspace Manifest ID, the named agent profile, exact Runtime ID and
  revision plus its compatible execution-image material, guided/advanced policy mode, required immutable
  `source_access`, required normalized `policy_revision`, explicit
  `native_readiness`, allowlisted shell-environment
  overrides, an optional non-default Git identity policy, and the complete
  immutable desired revision identities;
- `contexts/<name>/revisions/<generation>-<semantic-digest>.json`: retained
  immutable desired receipts. The path distinguishes history only; authority
  remains WorkspaceManifestID plus semantic digest. Exact same-ID/body replay
  is idempotent, while another ID/body or a partial artifact fails closed;
- `runtimes/<name>/source/`: owner-only editable managed Runtime build context;
- `runtimes/<name>/revisions/<digest>/source/`: immutable successful source
  snapshot built for that semantic revision;
- `runtimes/<name>/runtime.json`: stable Runtime identity and ordered successful
  revision inventory; drafts and failed builds are not history;
- `contexts/<name>/policy/context.json`: owner-only normalized schema-v1
  non-executable snapshot whose SHA-256 digest equals the manifest policy
  revision; Workspace Manifest policy changes never rewrite an existing snapshot. Enabled native readiness
  grants the reviewed Claude Code 2.1.220 and Codex 0.147.0 native model,
  account, bootstrap, first-party capability-discovery, bounded evaluation,
  and telemetry effects plus the pinned GitHub CLI 2.96.0 exact device-login
  bootstrap and GraphQL `query` / `viewer` current-user effects, plus TWG CLI
  1.2.5 exact device-code/token/revoke, site inventory, stable CLI manifest,
  and GraphQL `query` / `me` current-user effects,
  plus pup 1.10.7 exact US1 DCR registration and token exchange/refresh when
  supplied by a custom runtime, to
  every process in the Workspace Manifest, subject to the Workspace Manifest policy's terminal destination
  ceiling and method decisions. Those native readiness effects are not stored
  in new snapshots. Compile-time `claude_ready`, `codex_ready`, `gh_ready`, and
  `twg_ready`, and `pup_ready` names are review provenance; aggregate generation strips their
  complete historical snapshot forms and projects the installed binary's
  current set. The five families, their pinned client version, current
  readiness contract revision, and append-only historical contracts are
  declared in one dedicated compile-time catalog. If the active
  aggregate revision differs from the revision desired by that catalog and
  the current Workspace Manifest sources, status reports an invalid policy projection and
  root entry requires explicit `cluster up`. Strict schema V1 carries optional
  GraphQL protocol/operation/root identity on a `baseline_grants` item and keeps
  the existing explicit `mcp_baseline_grants` collection; every semantic grant
  requires its matching declared exact endpoint. HTTP-only normalized snapshots
  omit those optional GraphQL keys and retain their bytes. MCP initialization
  and enumeration are baseline methods at one exact endpoint; `tools/call`
  requires exact tool-name review. Dynamic evaluation uses one safe identifier
  segment without persisting its value. It is Workspace Manifest authority, not executable
  identity; exact Deny remains terminal, while payload arguments, downloads,
  file transfer, acquisition, self-update, and unmatched effects receive no
  baseline grant;
  schema V1 requires `method_policy.default` and sorted unique exact
  `method_policy.overrides`, each using `allow`, `exact_review`, or `deny`.
  Workspace Manifest creation can choose Deny, Exact Review, or Allow as the complete
  default and add exact method overrides. Exact Deny wins over method Allow.
  GET is not described as safe or read-only. The fixed agent-ready baseline is
  trusted-binary data, not a selectable profile. Workspace Manifest-owned snapshots are strict
  owner-only non-executable data with no wildcard, IP/private destination,
  secret, shell, Rego, include, inheritance, remote fetch, refresh, or signing;
- `contexts/<name>/policy/domains/<canonical-host>/allow.json`: strict
  schema-v1 authority, per-domain method, exact GraphQL endpoint, and
  learned-Allow source for one canonical lower-case
  host;
- `contexts/<name>/policy/domains/<canonical-host>/deny.json`: strict
  schema-v1 baseline-Deny and learned exact-Deny source for the same host;
  every directory, embedded host, authority, endpoint, binding, and rule host
  must match exactly. Guided Workspace Manifests own no Rego files, while Advanced
  Workspace Manifests additionally own `tobari.rego` and `tobari_test.rego`. Workspace Manifest
  policy has no `data.json`; the generated immutable OPA projection may use
  that filename. Wildcards, IP literals, non-canonical hosts, unknown fields,
  duplicate keys or rule IDs, incomplete pairs, symlinks, unsafe permissions,
  and extra files fail closed;
- `auth/providers/*.json`: experimental-only owner provider manifests, ignored
  by standard builds;
- `contexts/default.json`: owner-only exact default Workspace Manifest
  selection; missing means `default` and the marker has no enforcement authority;
- `principal-registry/principals.json`: owner-only host-issued schema-v1
  Workspace Manifest/project-to-owned-Workspace-and-Gateway endpoint bindings, maintained by lifecycle reconciliation
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
Pup is limited to the exact US1 authorization route, seven mandatory query
fields, at most one UUID-shaped `dd_oid` organization hint, a bounded DCR
client ID, the sorted pup 1.10.7 default-scope ceiling, and exact
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
`auth/projection/providers.json`; schema-1-envelope/schema-1-payload Workspace Manifest
vaults are below
`auth/contexts/<context-id>/vault.enc`; the Linux root key is the owner-only
`auth/keys/root.key`; runtime sockets are below `auth/runtime`; and schema-v1
Workspace authentication file registries are below `auth/projects`. On macOS,
the root key is instead stored in Keychain under service
`io.tobari.auth-root.v1` and account `tobari`.
The complete canonical schema/path/backend table is in
[Authentication handling](07_authentication.md#experimental-canonical-schemas-paths-and-backend-identifiers).

Runtime state is stored under `${XDG_STATE_HOME:-$HOME/.local/state}/tobari`:
`roots/<hash>.json` indexes `(canonical root, stable Workspace Manifest ID)` and
`instances/<id>/state.json` schema 1 contains one logical instance, its
permanent Workspace Manifest binding, and diagnostic runtime identifiers.
`instances/<id>/home` is the independent writable home for tool-owned state;
homes are never shared merely because Workspace Manifests or roots match.
Shared read-only agent profiles referenced by Workspace Manifests are under
`${XDG_DATA_HOME:-$HOME/.local/share}/tobari/profiles`.
Persisted cluster state schema 1 contains the content-addressed aggregate policy revision,
loaded Workspace Manifest count, aggregate projection paths, and Docker resource names or
identifiers, never one active Workspace Manifest authority or credential contents. The
loader accepts only exact V1 state. The owner-only projection contains the
Workspace Manifest-aware Gateway routing document. The per-Workspace home may contain native
tool credentials by design. Standard has no provider projection or declared
credential binding.
Project and cluster mutation journals are durable recovery markers. An
interrupted cluster marker, aggregate revision mismatch, or failed projection
activation makes entry and policy operations fail closed until the exact shared
cluster operation completes.
Environment variables select XDG locations and documented test/runtime
overrides. Workspace Manifest narrow projections are the user-facing exception: at shell
on Workspace entry Tobari reads only the selected subset of `PS1`, `TERM`, `COLORTERM`, and
`NO_COLOR`; at Workspace reconciliation Git inheritance makes at most two
bounded fixed-key host-global reads for `user.name` and `user.email`. Neither
path enumerates its source, and neither copies host credential values or source
configuration into the runtime.

Image selection comes only from the exact Runtime revision binding stored by
the selected Workspace Manifest. A new Workspace Manifest binds `standard@1`; project metadata never
overrides that authority. `runtime build` derives a managed image selector from
the Runtime name and semantic source digest, validates it, and appends a
revision without changing a Workspace Manifest. Only `manifest runtime set` replaces a
Workspace Manifest binding. The stored project image is updated after successful
runtime-container reconciliation on the next root entry, preserving the
Workspace home; failed image validation or Docker reconciliation preserves the
previous logical state and container.

When a managed source Dockerfile starts from the exact resolver-selected
official base, the explicit build verifies or builds that pinned base. An
explicit local or custom base is not given an implicit registry-pull request.

OPA reads one cluster-owned revisioned aggregate bundle with `--watch`. The
projection has one fixed `tobari.http/decision` router, Workspace Manifest-ID data
namespaces, one shared guided evaluator, and isolated Advanced package names.
Guided Workspace Manifests own policy data but no executable Rego source; projection uses
the current Tobari-owned shared evaluator and tests. Advanced Workspace Manifest source
and Gateway runtime input both use exact schema 1 before testing and activation.
Exact policy mutations lock aggregate generation, test the changed Workspace Manifest
source privately, generate and test the complete all-Manifest candidate, publish
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
create state merely to observe it. Workspace Manifest, policy, credential, auth, project,
and lock initialization belongs to declared create/write outcomes.

`runtime create` creates an owner-only installation Runtime source tree without
changing a Workspace Manifest. On interactive text streams, omitted `--copy-from` lists source
Bases only when at least one managed Runtime exists, with `standard` selected
first; explicit Base, redirected, and JSON calls never prompt, and omission on
those streams retains the standard starter behavior. Managed Base validation,
drift, cancellation, or target collision publishes no partial Runtime.
`runtime build` snapshots that complete bounded tree,
executes the explicit host Docker build, validates the generated image, and
atomically appends a successful immutable revision after its image digest is
confirmed. A failed or semantically unchanged build appends no revision.
Source validation rejects links, special files, more than 1,024 regular files
or 256 directories, a regular file over 32 MiB, a total over 64 MiB, and any
group/other permission bit before Docker. It returns
`runtime_source_invalid` with the safe relative path, observed size/count/mode,
applicable limit or owner-only correction, and no private cause or absolute
host path. The snapshot streams through a fixed buffer; the total ceiling is
not retained as one whole-source heap allocation.
Docker build failure exits nonzero, ends text presentation with a short summary
of the failed stage, source path, recovery command, and retained Runtime
history, and leaves every Workspace Manifest binding unchanged. BuildKit may retain
engine-owned cache layers, and a post-build validation failure may retain the
unselected candidate image; Tobari states this instead of deleting either one.
No build promotes into a Workspace Manifest. `manifest runtime set` is the only selection
mutation and revalidates one ready `standard` or `name@ordinal` revision before
atomically replacing the Workspace Manifest binding. Existing Workspaces are not mutated
by either operation; their next root entry reconciles the work container to the
newly selected image while preserving the Workspace home.
The official immutable base is pulled only for an explicit build whose recipe
starts from the exact resolver-selected release digest; custom and local bases
retain their local/cache-first behavior.

`manifest default set` validates the target Workspace Manifest and atomically
changes only the default selector. It never starts Docker, changes the aggregate, or
modifies an existing Workspace's Workspace Manifest, runtime, home, policy, or principal.
Creating a Workspace Manifest also never starts Docker; when shared state exists, the
result directs the user to explicit `cluster up` so the all-Manifest projection
can be validated and activated.

`manifest delete` validates one exact name under the installation lifecycle
lock. It rejects `default`, the default selector, and any stable Workspace Manifest identity
still referenced by a logical Workspace. A confirmed delete removes the exact
owner-only Workspace Manifest directory and Workspace Manifest-ID authentication state, preserves
project roots and shared runtime images, and reports `requires_reconcile` when
shared state exists. V1 has no force mode and never chooses a replacement
default Workspace Manifest implicitly.

`config shell` and `config git` atomically update only the selected Workspace Manifest
manifest after typed input, intent, target, and impact validation. The terminal
editor performs no write before Apply, and one Shell Apply writes its entire
validated distinct-variable batch or nothing. Git inheritance performs no Git read during the
configuration mutation; the next matching root reconciliation runs at most two
fixed, one-attempt, finite-time host Git queries, validates a complete pair,
and atomically refreshes one private per-Workspace fallback before Docker
mutation. Failure preserves the prior file and returns no raw Git diagnostic or
identity. The file's exact directory is mounted read-only as system scope and
includes the image system config before the Workspace Manifest fallback, preserving
normal Workspace-global and repository/worktree precedence.

`config bootstrap aws` atomically updates a separate secret-free recipe in the
selected Workspace Manifest. It reads only host `~/.aws/config`, accepts one strict IAM
Identity Center profile/session subset, and reports only adapter, profile,
generation, revision, and changed-field metadata. Configure and refresh affect
future Workspace creation only; remove stops future projection. No variant
reads or outputs AWS credentials or SSO cache values, invokes AWS, performs
login, changes network policy, or rewrites an existing Workspace home. A new
Workspace receives one canonical private `.aws/config` and records its applied
semantic revision before logical publication.

The argument-free Workspace Manifest wizard reuses this resolver through a read-only
candidate boundary. It reads fixed `~/.aws/config` only after explicit
Workspace-bootstrap editing, parses the file once, and resolves every profile
with its referenced `sso-session`. Available candidates expose only the
secret-free semantic fields needed to choose; individually incompatible
profiles remain visible with a reason, while malformed, duplicate, or unsafe
whole-file input yields no partial candidates. Final Create revalidates the
reviewed profile/session semantic revision. A changed selected bundle returns
to review with zero mutation; unrelated profile changes do not block.

`config bootstrap kubernetes eks` composes one additional closed adapter with
that AWS recipe. It reads only fixed host `~/.kube/config`, selects one explicit
context and its exact cluster/user references, and accepts only an inline CA,
commercial EKS HTTPS origin, and the reviewed `aws eks get-token` contract with
`AWS_PROFILE` equal to the Workspace Manifest AWS profile. It rejects credentials, proxy or
insecure TLS options, file references, arbitrary exec fields, role arguments,
unknown fields, alternate paths, and unsafe files. Projection emits canonical
private `.kube/config` JSON in the fresh Workspace home. Removing EKS preserves
AWS; AWS cannot be removed or changed to another profile until its dependent EKS adapter is removed. No
configure, refresh, or create operation calls AWS or Kubernetes or grants a
network effect.

After an AWS candidate is selected, the Workspace Manifest wizard discovers only
kubeconfig contexts compatible with that exact AWS profile and semantic
revision. `Do not configure Amazon EKS` is the first explicit choice;
incompatible contexts remain visible but unselectable. Discovery performs no
network or subprocess call and reads no credential or cache state.

`cluster up` obtains and preflights the immutable Gateway image and official
runtime bases required by all Workspace Manifests, generates and validates the complete
aggregate policy/routing projection, then creates shared labeled networks,
configuration material, exactly one Gateway, exactly one OPA, and CA volumes
as needed. It reconnects Gateway to the shared
networks and existing registered project networks without creating project
state or project resources and waits for both services to be healthy. The
root command only verifies the shared cluster is
configured and ready, reads the indexed Workspace candidates, and waits for an
explicit choice when the canonical current directory is below an ancestor.
After the choice is revalidated under the lifecycle lock, it creates or reuses
the selected Workspace Manifest-bound logical record, resolves the bound Workspace Manifest's narrow
Git identity fallback for that stable root, resolves and validates its bound
Workspace Manifest image before project runtime mutation, reconciles its labeled container and internal
network, binds its XDG home, joins Gateway to that network, waits for the
project healthcheck and enters the resulting terminal session. Docker create
appends Tobari's fixed `sleep infinity` lifetime command after the image; the
image `CMD` is not used to own Workspace lifetime. Bash and direct commands
run through child exec sessions. A direct command is passed as exact argv after
Docker's `--`, without a shell, and never becomes PID 1. Each child exec late-binds only the
bound Workspace Manifest's declared shell-environment inheritance and applies its fixed
fallbacks without changing container identity; ANSI color sequences in `PS1`
remain interpreted by the attached terminal. A child command's nonzero exit is
returned without stopping the reusable Workspace. A changed image identity,
runtime contract, mount/security/environment/health specification, or shared
profile revision recreates only the project container and preserves its
logical state and home.
Before ordinary re-entry closes a project principal, Tobari observes whether
the exact owned network, endpoint pair, runtime specification, source access,
health, and connectivity already match. A matching Workspace keeps its binding
while both network guards are revalidated and the current endpoints are
atomically refreshed, so cancellation cannot create a principal gap when no
Docker mutation was needed. Drift still closes authority before repair and
remains fail closed if interrupted; cancellation never retries entry or the
child request.
Returning from that child session, including a normal shell `exit`, performs no
Workspace deletion: it only returns the child exit status and emits the host
stderr guidance described above. `delete` is the separate lifecycle-ending
operation. It removes only that exact label-owned container, network, root
index, instance state, and home after confirming that no session is attached;
`--force` overrides that one guard.
`cluster down` rejects while any
Workspace remains
and removes only exact shared resources; its `--purge` also removes shared CA
and active policy-bundle volumes. Both forms preserve every encrypted Workspace Manifest vault and the installation
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
coordinate when applicable, and the Workspace Manifest policy ceiling already satisfies the
orthogonal boundary.
Candidate
discovery excludes other denials, preventing a successful no-op approval.

In the experimental build only, `auth login`, `auth import`, and `auth logout`
validate the fixed installation
credential-catalog target and mutation impact before acquisition or vault I/O.
Login selects only the active profile's closed provider union through an
interactive trusted-host terminal. It includes GitHub, Datadog, OpenAI,
Anthropic, Chatwork import, and AWS. Anthropic alone
executes in a fresh selected-Workspace Manifest-image container; each driver owns
fixed argv, canonical executable identity, private state, bounded browser/PTY
behavior, strict typed capture, and checked cleanup. PATH resolution inspects
at most 256 distinct absolute candidates in order, never executes a rejected
temporary/project/home-local shadow, and selects only the first candidate whose
canonical executable satisfies the existing trusted-root and mode checks. It
prints only bounded,
control-safe guidance and commits the captured record only into the encrypted
Workspace Manifest vault. A host-login availability failure retains its stable fault
code and names one fixed secret-free diagnostic stage; it never publishes the
selected local path, digest, raw process error, or captured output. Drivers
read no ambient provider home and write no project or Workspace CLI
configuration; Auth Broker contains no provider CLI. Import
rejects terminal stdin before reading and reads bounded non-terminal input only
after public argument/intent/mutation validation; infrastructure validates the
selected Workspace Manifest, installed provider/acquisition mode, and broker readiness
before broker send. Login/import atomically replace one typed
Workspace Manifest/provider grant and revoke every prior handle. Gateway performs
non-secret introspection before OPA and applies exactly one same-revision
static resolution, Datadog/OpenAI/Anthropic token result, or bounded AWS SigV4 result only
after allow. Gateway makes one upstream attempt. Managed profiles and arbitrary
manifest-selected dynamic execution do not exist. Logout atomically removes
the record and its handles without contacting the provider.
One credential is Workspace Manifest/provider-owned, every permanently bound project is
eligible for a distinct handle only on its next matching Workspace entry, and
no mutation rewrites a running session. Confirmed results are secret-free and
distinguish `changed` from no-op logout. They list an exact Workspace Manifest-bound
working directory and argv only for a Workspace whose current projection is
authoritatively missing or stale; current, unavailable, unresolved,
zero-Workspace, and no-change results do not invent a re-entry action. Logout
revokes all old handles immediately when its receipt is `changed`; next entry
removes its declared environment projection
by recreation and removes only unchanged Tobari-owned complete files. `auth
status` is read-only and reports locked or unavailable Broker state as provider
availability uncertainty rather than inferring absence from an unreadable vault.

## Pre-public V1 boundary

Workspace Manifest reports, Workspace status/list, and migration output use
their Catalog-declared schema 2. Unchanged public and internal boundaries keep
their independently owned versions; a shared numeral is not cross-surface
compatibility. Ordinary readers accept only their exact declared version and
fail closed on every other version. Tobari provides no retired command alias,
implicit old-state interpretation, or general compatibility shim. The sole exception is
`migrate apply`: an explicit installation-local write that accepts only the
strict unpublished predecessor named by ADR 0070. It creates an owner-only
content-addressed backup, retains predecessor Manifest and Workspace UUID
bytes, converts desired/applied state, and promotes an exact legacy custom
Dockerfile through the managed Runtime build boundary. Standard
Workspace-native home/auth bytes are preserved without reading or converting
them.

Predecessor experimental Broker authority is not rebound. Migration enumerates
the complete filesystem-side ciphertext, bindings, handles, lookups,
projections, registries, and provider/config records and atomically moves them
to owner-only private quarantine. Unknown, mixed, partial, corrupt, unsafe,
symlinked, or drifted state fails before mutation. macOS Keychain recovery
material is neither read nor changed; Linux filesystem root-key material moves
with the set. Ordinary old/new readers cannot discover quarantined content,
and public JSON reveals no secret path or Keychain fact. Rollback restores the
byte-identical set only when no fresh canonical auth state exists. The only
public disposition is the bounded non-secret
`research_auth_disposition: reauthentication_required`; later research use
requires explicit fresh login/import.
Every other development snapshot must be removed and recreated.

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
Chatwork, and AWS and retains one credential per Workspace Manifest/provider. Owner
manifests may express another single static primary secret only through the
exact HTTPS/header replacement contract and protected stdin import. V1 has no
managed adapter, multiple provider accounts, provider-specific policy
semantics, Git credential helper, manifest-selected helper, or general
provider SDK/plugin executor. Standard tools authenticate natively inside their
Workspace-owned home; that state is neither brokered nor a network grant.
