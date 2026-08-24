# Product Contract

The release/research capability vocabulary, resolver axis, breaking V1 schema
cutover, and archive boundary are governed by [ADR 0082](decisions/0082-release-and-research-build-surfaces.md).
Physical-host loopback naming, retirement, and cutover are governed by
[ADR 0083](decisions/0083-name-the-physical-host-loopback-authority.md).
The CWD-first status-home selection, snapshot, output, and call-budget contract
is governed by [ADR 0085](decisions/0085-make-status-the-cwd-home.md).

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
release surface has no provider credential projection or Auth Broker.

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
`http://host.tobari.internal:{port}` for physical-host IPv4 loopback HTTP on ports
1024 through 65535. No entry flag or service declaration is required. The
Workspace receives the URL template, bounded port range, Workspace audience,
and explicit `attachment` lifetime; `localhost` continues to mean the
Workspace. Capability discovery and routing metadata are not permission. The first exact
effect follows the ordinary deny, `review permissions`, decide, and retry loop, and
the decision is available to every process in the Workspace only until the
owning host attachment exits. Docker, Compose, host daemon state, automatic
port discovery, raw TCP, and persistent Host Loopback grants are not part of
this outcome.
The retired exact `host.tobari.test` authority is terminal and non-learnable
for all V1. It has no alias, redirect, translation, external-policy fallback,
or automatic retry.

The opposite direction is one explicitly reviewed Workspace service. From a
live attachment, `tobari-expose <port>` requests exact Workspace
`127.0.0.1:<port>` access. A separate trusted-host `tobari review services`
shows one complete effect card and accepts one deliberate Allow once, Allow
once then Open, Deny, or Back action; only Allow once makes the owning Service
attachment bind `tcp4 127.0.0.1:0`. Each exposure receives a fresh URL with
scheme `http`, authority
`svc-<128-bit-random-lowercase-label>.localhost:<random-port>`, and path `/`.
HTTP/1.1 and WebSocket Upgrade relay without
rewriting Host, Origin, redirects, cookies, headers, or content. The helper can
report current-attachment pending and active state and stop one only with its
unchanged opaque reference. `service open` accepts only that reference and
derives the exact live root URL; browser dispatch is separate from Allow.
Attachment exit closes the listener and streams. This grants no
Template policy, Context Policy Memory decision, Host Loopback authority, LAN access,
automatic discovery, requested host port, health probe, or browser opening.

Routine permission guidance presents three layers and no implementation-source
inventory:

1. **Workspace Template policy** is the desired destination and method Boundary
   together with the routine client traffic admitted inside it.
2. **Remembered Context decisions** are trusted-host reviewed Allow and exact
   Deny choices retained in Context Policy Memory for one Context and Workspace
   until explicit reset.
3. **This-session Host Loopback access** is an exact decision bound to the
   active attachment and removed when its owning host process exits.

Host Loopback is a separate closed policy branch rather than a temporary
widening of Template or Context authority. Ordinary Template policy, Context
Policy Memory, native readiness, and Advanced Rego neither authorize nor deny it;
Attachment Grants neither authorize nor deny ordinary external traffic.

The user-facing entry point is the current project directory: a Workspace either
exists or does not exist, and the user should not need to manage container
names, network IDs, or policy internals for routine work. `cluster up` remains
the independently invocable owner of shared Gateway and OPA setup, while an
interactive first-use `tobari` composes that exact action after a newly
confirmed default Template/Context pair so a human need not remember the setup
sequence.

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

For a supported ordinary HTTP or HTTPS denial, Gateway schema 2 may additionally
publish one attachment-local `permission_wait_id` after the canonical
interactive attachment owner acknowledges the exact bounded record. The child
can then run `tobari-permission wait --id pwt_<32-lowercase-hex>` and receive
only `Allow`, `Deny`, or `Expired`. The helper exposes no candidate, policy
decision, scope, revision, discovery, or retry operation. `Allow` is
retry-readiness evidence; every deliberate fresh request is authorized again
by Gateway.
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
For a Workspace Template with a validated EKS bootstrap, the exact Kubernetes API origin
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
- **Workspace:** one replaceable isolated instance and home owned by one
  Context. Its work container is recoverable runtime detail; the stable
  Workspace ID and permanent Context binding are authority rather than routine
  selection inputs.
- **cluster:** the one installation-local Gateway, one OPA, aggregate policy,
  principal registry, and CA lifecycle. The research surface
  adds one locked Auth Broker and provider projection.
- **Gateway:** the trusted HTTP/HTTPS policy enforcement point.
- **OPA:** the trusted policy decision point.
- **Host Loopback:** the constant `host.tobari.internal` HTTP destination whose URL
  port selects the same physical-host IPv4 loopback port for an active attachment.
- **Attachment Epoch:** one unguessable trusted-host identity owned by the
  `tobari` process that established the active Host Loopback route.
- **Attachment Grant:** one exact reviewed Allow or Deny bound to a Context ID,
  Workspace ID, Attachment Epoch, target port, and exact HTTP effect. It is
  Workspace-wide for that attachment and is not a learned policy rule.
- **Auth Broker:** the research non-root credential-resolution daemon. It owns
  encrypted Context vault access, has no TCP listener, starts locked, and
  exposes separate control and Gateway-only runtime Unix sockets.
- **project root:** the canonical host directory selected from the current working
  directory and mounted directly with the bound Workspace Template's immutable
  `read-only` or `read-write` source access. A root below the host home
  is mounted at the same relative path below `/var/lib/tobari`; a root outside
  the host home uses the mirrored `/workspace` path.
- **Workspace home:** a per-Workspace persistent owner-only XDG state directory
  mounted as the work user's home.
- **Tobari image:** the minimal built-in runtime or one locally available
  compatible OCI environment image selected by the Workspace's Context-bound
  Template revision for creation and later runtime-container reconciliation. Its tools and bootstrap
  are part of the environment; its image `CMD` is not the Workspace lifetime
  command.
- **Workspace ID:** a generated stable internal identity used for state, exact
  resource labels, and host-issued Workspace-principal bindings. It is diagnostic
  output, not a routine user action input.
- **Workspace principal:** a host-issued binding from one stable Workspace ID and
  stable Context ID to the exact owned Workspace source endpoint and Gateway
  endpoint on that Workspace's dedicated network. The internal schema-V1
  protocol retains the field name `project_id`; that compatibility name carries
  the Workspace ID and never identifies the project root. Caller headers, Template names,
  SNI, request authority, and profile names are not principals.
- **tool-owned authentication state:** files written by a tool or agent below
  one Workspace's persistent home during its own login or configuration flow. It
  is the standard credential source and is readable by every process in that
  Workspace.
- **research credential provider:** the external service or authority whose credential
  is acquired or imported, stored, and later applied to one exact reviewed
  request binding. A provider is not the Workspace client that uses it.
- **research Workspace client tool:** the CLI or other client inside a Workspace that
  receives a provider-declared handle projection and emits the authenticated
  request shape. Standard pairings cover GitHub/`gh`, Datadog/`pup`,
  OpenAI/Codex, Anthropic/Claude, and Chatwork/`cwk`; the research
  surface additionally covers AWS/`aws`. Their names
  grant neither provider identity nor network authority.
- **research brokered credential:** one typed static or reviewed renewable record owned
  by a stable Context and provider, acquired through protected non-terminal
  stdin or one purpose-limited reviewed host driver and stored in the
  encrypted Context vault.
- **research Workspace credential handle:** a versioned random opaque value bound to one
  Context, Workspace, provider, credential revision, and exact HTTP binding. It
  is not the real credential, but it is a scoped bearer capability that should
  not be published or logged. It is not authority without the trusted
  principal, exact binding, and OPA allow. Broker metadata never inherits a
  broad static host/method allow; the first L7 effect remains reviewable until
  an explicit exact or single-segment-template learned rule exists.
- **research provider manifest:** strict non-secret data declaring static import,
  Workspace handle projections, and exact HTTPS/header credential bindings.
  Owner manifests are V1 static-secret/header plans. Standard reviewed
  built-ins are a closed typed union for GitHub, Datadog, OpenAI, Anthropic,
  and Chatwork; the research surface adds AWS without making it runtime configurable.
  Owner data declares no executable shell, helper choice, refresh, signing,
  arbitrary route, HTTP method/path policy, or provider operation semantics.
- **Workspace Template:** one stable host-owned desired Workspace definition
  with a stable opaque ID and human name. Every semantic mutation publishes one
  complete immutable revision. `WorkspaceTemplateID + semantic digest` is
  authority; generation is correlation only. Its Boundary records direct
  source access, normalized policy and terminal ceilings, policy mode, and
  native-readiness participation. The same revision contains one exact Runtime
  binding and narrow Workspace defaults grouped by cluster, entry, child
  session, and creation activation boundaries. Context, Policy Memory,
  Workspace, authentication, and applied state are separate authorities.
- **Context:** one durable immutable binding between a canonical Project root
  and one Workspace Template. ContextID is authority; a Context owns Policy
  Memory and has no second name, default, or mutable selector.
- **Policy Memory:** one Context-owned complete immutable history of remembered
  authorization decisions. It survives Workspace replacement and is not copied
  with a Template.
- **default Workspace Template:** only the installation selector used by bare
  root entry and bare `status`; it is not Context, applied, or shared-cluster
  authority.
- **recommended draft:** the display-only first-use proposal shown before any
  Template or Context exists. It has no ID, revision, selector, or store
  authority and cannot bind a mutation.
- **Runtime:** one installation-wide reusable environment definition. A managed
  Runtime owns an editable bounded Docker build-context tree and immutable
  successful semantic revisions; the built-in standard Runtime has one
  compiled revision. Runtime source and snapshots are never Workspace mounts.
- **Workspace Template Runtime binding:** one exact stable Runtime ID and semantic revision
  selected by a Workspace Template. Its human `name@ordinal` form is review syntax, while
  the persisted ID and SHA-256 revision are authority. Only `template runtime
  set` replaces the binding; a Context resolves its bound Template's current
  revision and its Workspace adopts that entry slice on next entry without
  changing Context or Workspace identity or persistent home.
- **agent profile:** read-only non-secret shared agent configuration referenced
  by a Workspace Template. It is not tool-owned login state.
- **narrow projection:** one fixed Workspace Template-owned allowlist of validated
  non-secret scalar fallbacks. The source file, path, directives, executable
  settings, credentials, and undeclared keys never enter the Workspace.

Public contracts use `workspace_template_id`, `context_id`, and `workspace_id`
for their distinct authorities. Template, Context, Policy Memory, and family
command results start at schema 1; Workspace state and bare status use schema
3. Frozen private Gateway/OPA/Broker/Host Loopback wire tokens remain governed
by their own versions and do not become public/domain aliases. There is no
public compatibility reader or predecessor vocabulary alias.

Stable Workspace and Context IDs are not trusted when supplied by a work
container. The host-owned principal registry derives both from the exact
kernel-observed Workspace source endpoint within its verified dedicated
network binding. The registry also retains the exact Gateway endpoint and
rejects duplicate or stale endpoints. Template policy and Context Policy
Memory are selected inside the single OPA from that trusted principal. Learned
permissions are Context-owned and retain observed Workspace identity. Principal identity never selects
or injects tool credentials.
Standard Gateway passes one Workspace-owned credential only after the ordinary
HTTP decision; research Broker resolution additionally requires its exact
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
| `completion candidates --current <index> <word>...` | utility | read | Return bounded catalog-derived or validated local-state candidates for the current command word without mutation, Docker, or network access |
| `version [--format text|json]` | utility | read | Print source version/commit, resolver channel, required and selected standard component APIs, and compatibility |
| `doctor [--root PATH] [--format text|tsv|json]` | utility | read | Report read-only host, Docker, configuration, policy, Gateway, port, and residue diagnostics without repairing state |
| `cluster up [--format text|json]` | act | create | Activate the exact final collection's current Template-policy and Policy-Memory axes and reconcile the selected shared component closure |
| `cluster status [--format text|json]` | utility | read | Observe one bounded final collection, its active or stopped receipt consequence, and the selected shared component closure without repair |
| `cluster denials [--tail <lines>] [--format text|json]` | utility | read | Inspect one bounded Gateway denial window correlated to exact final Context, Template, and Workspace authority |
| `cluster logs [--component gateway\|opa\|all] [--tail <lines>]` | utility | read | Inspect one bounded redacted window from the surface-selected final shared components |
| `cluster down [--format text|json]` | act | write | Stop the exact final shared component closure and clear every active Context receipt while preserving Templates, Contexts, current Policy Memory, and the final envelope |
| `policy candidates [--format text|json]` | discover | read | Return every exact pending candidate from one coherent final authority envelope |
| `review permissions [--format text|json]` | discover | read | Inspect the coherent final pending set without rediscovering predecessor denial logs |
| `policy rules [--format text|json]` | discover | read | Return every current Context-owned remembered decision from one coherent final authority envelope |
| `policy allow --id <policy-candidate-ref> [--format text|json]` | act | write | Remember and activate one exact Allow |
| `policy deny --id <policy-candidate-ref> [--format text|json]` | act | write | Remember and activate one exact Deny |
| `policy reset --id <policy-rule-ref> [--format text|json]` | act | write | Remove one exact current remembered decision and activate the resulting Policy Memory |
| `template list [--format text|json]` | discover | read | Return the exhaustive final Workspace Template collection |
| `template show [--name <name>] [--format text|json]` | discover | read | Return one final Template and its exact current immutable revision |
| `template create --name <name> [--format text|json]` | act | create | Create one fresh Template from the reviewed built-in standard body |
| `template copy --from <template-revision-ref> --name <name> [--format text|json]` | act | create | Create one independent Template from one exact retained revision |
| `template default set --id <template-ref> [--format text|json]` | act | write | Select the default Workspace Template |
| `template delete --id <template-ref> --confirm=delete [--format text|json]` | act | write | Delete one unused Workspace Template |
| `config shell --id <template-ref> --variable COLORTERM\|NO_COLOR\|PS1\|TERM --source default\|inherit\|literal [--value <value>] [--format text|json]` | act | write | Update exact Template shell defaults from the current body under the lifecycle lock |
| `config git --id <template-ref> --source default\|inherit\|literal [--name <name> --email <email>] [--format text|json]` | act | write | Update exact Template Git defaults from the current body under the lifecycle lock |
| `config bootstrap aws --id <template-ref> [--profile <name>] [--refresh] [--remove] [--format text|json]` | act | write | Update exact Template AWS creation defaults from the current body under the lifecycle lock |
| `config bootstrap kubernetes eks --id <template-ref> [--kube-context <name>] [--refresh] [--remove] [--format text|json]` | act | write | Update exact Template EKS creation defaults from the current body under the lifecycle lock |
| `template runtime set --id <template-ref> --runtime <runtime-revision-ref> [--format text|json]` | act | write | Replace exact Template Runtime binding from the current body under the lifecycle lock |
| `context list [--format text|json]` | discover | read | Return every final Context with exact Project and Template scope |
| `context show --id <context-ref> [--format text|json]` | discover | read | Return one exact Context with desired and independently active authority |
| `context create --template <template-ref> [--format text|json]` | act | create | Create one empty Context from one unchanged Template reference and canonical CWD |
| `context enter --id <context-ref> [--format text|json] [-- <command>...]` | act | create | Reconcile and enter one exact Context Workspace |
| `context delete --id <context-ref> --confirm=delete [--format text|json]` | act | write | Delete one exact Context, its Policy Memory, and unresolved candidates |
| `workspace list [--format text|json]` | discover | read | Return every final Workspace and its exact owner binding |
| `workspace status --id <workspace-ref> [--format text|json]` | discover | read | Return one exact Workspace and its applied authority |
| `workspace delete --id <workspace-ref> --confirm=delete [--force] [--format text|json]` | act | write | Retire one exact Workspace, home, native authentication state, and owned runtime resources while preserving Context Policy Memory |
| `review services [--watch] [--notify auto\|osc9\|bel\|off] [--format text\|json]` | discover plus TTY reference-bound actions | read, or one confirmed create/write | Return pending request refs only; a trusted interactive text TTY uses one action key/token as confirmation, while JSON and redirected operation are read-only |
| `service status [--format text\|json]` | discover | read | Return one complete-delivery, bounded-window host snapshot of pending requests and active exposures with both opaque ref kinds and explicit complete/partial/unavailable owner observation |
| `service allow --id REQUEST_REF [--format text\|json]` | act, reference bound | create | Revalidate one pending request and create one attachment-owned random IPv4-loopback exposure without opening a browser |
| `service deny --id REQUEST_REF [--format text\|json]` | act, reference bound | write | Resolve one pending request without creating host access |
| `service open --id EXPOSURE_REF [--format text\|json]` | act, reference bound | write | Revalidate one active exposure and request purpose-limited browser opening of its owner-derived root URL |
| `service stop --id EXPOSURE_REF [--format text\|json]` | act, reference bound | write | Close one exact listener and its relays through the live owner |
| `runtime list [--format text\|json]` | discover | read | List the exhaustive installation-wide Runtime catalog, stable Runtime references, and each ready head revision |
| `runtime show --name NAME [--format text\|json]` | discover | read | Inspect one Runtime's stable reference, managed source path, and complete successful revisions |
| `runtime history --name NAME [--format text\|json]` | discover | read | Show one Runtime's stable reference and ordered immutable successful revision history |
| `runtime create [--copy-source-from RUNTIME] --name NAME [--format text\|json]` | act, fixed target | create | Create one standalone owner-only managed Docker build-context source from the standard starter or another managed Runtime's current editable source, with a fresh Runtime ID, empty history, no lineage, and no Template, Context, or Workspace change |
| `review runtimes [--format text\|json]` | discover plus TTY reference-bound action | read, or one confirmed write | List the exhaustive Runtime catalog; trusted interactive text offers only managed Runtime actions and uses one confirmation for a selected build or exact interrupted build, restore, or whole-delete recovery, prioritizing the enclosing delete journal, while redirected and JSON output remain read-only |
| `runtime build --id RUNTIME_REF [--format text\|json]` | act, reference bound | write | Re-resolve one stable managed Runtime ID under the lifecycle and store locks, then snapshot, build, validate, and append one immutable semantic revision without changing any Workspace Template |
| `runtime restore --id RUNTIME_REVISION_REF [--format text\|json]` | act, reference bound | write | Rebuild one exact missing or pruned managed revision from its retained immutable source, publish only an exact digest match, and preserve Runtime history, Workspace Templates, and Workspaces; an already-available exact revision performs no durable write |
| `runtime delete --id RUNTIME_REF --confirm=delete [--format text\|json]` | act, reference bound | write | Delete one exact unused managed Runtime as a whole—editable source, immutable snapshots, revision history, and exact owned image tags—only after complete protection and use observation, while preserving Workspace Templates, Workspaces, IDs, homes, applied receipts, Project roots, credentials, and shared resources |
| `runtime prune dry-run [--format text\|json]` | discover | read | Produce one exact opaque prune-plan reference from a complete coherent installation observation, listing every eligible unused owned image tag, protection, blocker, preserved source/snapshot byte count, and bounded Docker observation without creating state or changing Docker |
| `runtime prune apply --plan RUNTIME_PRUNE_PLAN_REF --confirm=prune [--format text\|json]` | act, reference bound | write | Revalidate and apply one unchanged reviewed plan, removing only exact Tobari-owned unused image tags while preserving Runtime source, immutable snapshots, revision history, Workspace Templates, Workspaces, homes, IDs, and shared image content |
| `tobari [-- <command>...]` | act | create | Atomically initialize a fresh default Template and Context when required, then reconcile and enter their exact Workspace |
| `status [--format text|json]` | discover | read | Return one CWD-first schema-3 home snapshot for the nearest Project root and installation-default Template, with independent desired/active/applied/observed facts, one Next, and ordered Attention |

Bare `tobari review` is a pure Catalog namespace listing with exactly the
public task leaves `permissions`, `runtimes`, and `services`; it performs no task read or
mutation and has no registered selector handler. `review permissions` retains
bounded, durable staged Apply. `review services` retains complete delivery
over bounded-window owner observation and immediate attachment-local Allow
once, Allow once then Open, or Deny. The pre-public
`policy review` path and registered root `review` selector have no alias or
fallback; persisted policy and attachment state need no migration.

Every Tobari-controlled base Runtime image builds dedicated Linux Workspace
helpers from the checked source/Catalog closure with a pinned builder. The host
extracts and verifies those engine-native helpers, then mounts them read-only
only while attached. Each main hardcodes its helper Program; changing `argv[0]`
or copying a helper cannot expose host commands:

| Helper command | Role | Effect | Outcome |
|---|---|---|---|
| `tobari-expose <port>` | act, fixed target | create | Request trusted-host review, block, keep pending guidance on stderr, and emit one final schema-1 JSON exposure only after confirmed Allow |
| `tobari-expose status` | discover | read | Emit schema-1 JSON for complete current-attachment pending and active state; pending rows have no host mutation ref and active rows carry exact stop refs |
| `tobari-expose stop <exposure-ref>` | act, reference bound | write | Emit schema-1 JSON after closing one exact current-attachment listener and its active relays |
| `tobari-expose help [<command>...] [--format text\|agent]` | utility | read | Discover only the helper program's exact contracts |
| `tobari-permission wait --id <permission-wait-id> [--format text\|json]` | utility | read | Observe one attachment-owned reviewed disposition as `Allow`, `Deny`, or lease `Expired`, without mutating policy or retrying the denied request |

The unsupported research surface built by `task build:dev`
additionally exposes `serve [--no-open]`. It runs one foreground IPv4-loopback
Operator Console for typed cluster, Workspace, Permission Inbox, and learned-rule
inspection; it may open the host browser, stages decisions without authority,
and delegates one confirmed reviewed set to the canonical fixed-target Apply.
The release-surface development binary and release archives omit this command.

The CWD entry/read commands `tobari` and `status` have no Template or Context
selector. Bare `status` resolves the nearest canonical ProjectRoot
from CWD before applying the installation default Template; same-root Contexts
for other Templates remain siblings and never redirect selection. Nondefault
work uses an opaque Context or Workspace reference obtained from its owning
discovery command.

Fresh root review and default-shell entry require the existing interactive
terminal contract. An existing-authority direct child may use its declared
noninteractive stream contract. Root does not silently create state in a
noninteractive fresh context. With no
persisted final Template/Context authority, it first validates the canonical
Project root and shows one recommended draft: direct read-write project effect,
effective routine/other/private Access, `standard@1`, no host import, and Bash
or the safely projected direct executable. The draft has no TemplateID,
ContextID, default selection, revision, Workspace, or persistence. Start
revalidates empty authority under the canonical final lock, publishes the
reviewed `default` Workspace Template, selects it as the installation default,
and creates this Project's Context through the existing final default-pair
boundary. Customize edits the same complete Template body before publication;
it is not Template copy and records no provenance. Cancellation, EOF,
rendering, terminal failure, or a noninteractive fresh invocation changes no
Template, Context, host configuration, cluster, Docker, Workspace, or network
state.

After review and before the first mutation, root runs the closed generic
Workspace-start readiness profile: Docker CLI, selected Engine version,
selected Docker Context, and Compose v2. Engine major versions below 24 fail as
unsupported. Any failed or invalid observation returns one fixed safe fault
pointing to `doctor` and performs zero Template, Context, cluster, Workspace,
network, or Docker mutation. The profile neither identifies nor manages the
Docker provider. Standalone Template/Context actions remain independent from
this root readiness composition.

After final default-pair publication, root performs the exact canonical
`cluster up` reconciliation without another confirmation, re-observes the
returned final collection receipt, and requires the exact reviewed
Project/default-Template/Context authority before entry. A later failure never
rolls back a confirmed earlier receipt. When protection is ready, root invokes
the canonical Context-entry boundary, which alone reconciles the Workspace
AppliedEntry and hands off the child. Runtime customization is an independent
prepare-first flow; root never implicitly builds, restores, prunes, deletes, or
selects another Runtime. If authoritative Runtime execution material is absent
or mismatched, immediate human recovery is `review runtimes`, which owns the
opaque revision reference and exact build/restore choice.

An existing default pair skips fresh review and is re-observed before each
canonical mutation. If its shared projection is absent, stopped, or invalid,
the same root invocation composes canonical `cluster up` before Workspace
entry. When the
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
marker: `tobari -- COMMAND [ARG...]`. A bare `--`, an empty
executable, or child argv without the marker fails before setup or Workspace
mutation. Tobari neither invokes a shell nor expands, joins, or reparses the
argv; order, duplicates, dash-prefixed values, and explicit empty arguments are
preserved. Fresh direct entry still requires the one recommended interactive
review; after authority exists it is a routine path for Claude, Codex, GitHub
CLI login, or another exact executable. The child owns the foreground terminal
and signals after handoff. Its exit returns to the host shell rather than
entering Bash, and its exact status is returned without stopping the fixed
Workspace lifetime process. Tobari may then emit one clearly host-owned bounded
cleanup diagnostic, but cleanup failure cannot replace child status or permit
automatic retry.
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
- Each `(canonical root, stable Workspace Template ID)` identifies at most one
  Context, and each Context owns at most one replaceable Workspace. Repeated or
  concurrent entry converges on that Workspace; the same root may have
  independent Contexts for different Templates. Explicit Context actions never
  change the default Workspace Template.
- Project-root selection rejects the filesystem root, the user's home and its
  ancestors, and any path overlapping XDG Tobari configuration, state, or
  shared-profile management directories, Docker sockets, or Docker management
  paths. A repository containing policy source remains allowed; only the
  trusted active policy/configuration paths are protected.
- An explicit create choice uses the canonical current directory as root.
  Project moves and copies are not inferred or recorded in the project tree.
- The selected root is the only project directory mounted from the host and
  uses the bound Workspace Template's immutable `read-only` or `read-write` access. When
  it is below the host home, the container target preserves the relative path
  below `HOME=/var/lib/tobari`; for example, a host root of
  `$HOME/path/to` enters at `/var/lib/tobari/path/to`. A root outside
  the host home retains `/workspace/<canonical-root-without-leading-slash>`;
  thus a root at `/work` and a host CWD of `/work/root` enter at
  `/workspace/work/root`. Read-only applies only to this direct live bind: it
  is not a snapshot, host or same-root read-write Workspace Template changes remain
  observable, no writable source alias exists, and Workspace home and tmpfs
  remain writable. Tobari never mounts the host home wholesale.
- Same-root Workspaces in different Workspace Templates and parent/child roots may run
  concurrently. Runtime, home, network, policy, and broker handles follow
  their Workspace/Workspace Template boundaries, but overlapping host-file writes are visible
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
  the official runtime base for an uncustomized Workspace Template; custom images still
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
  release-surface catalog contains no `auth` namespace. The following legacy driver
  contract is compiled only by `task build:dev`. Its authentication commands
  accept an existing Workspace Template name and installed provider ID; the research
  surface accepts GitHub, Datadog, OpenAI, Anthropic, Chatwork import, and AWS.
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
  compatible Workspace Template image, runs exact Claude Code 2.1.220 native account login,
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
  byte is read. Non-terminal bytes are read only after public Workspace Template/provider
  argument, intent, and mutation validation; infrastructure then validates the
  selected existing Workspace Template, installed provider/acquisition mode, and broker
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
- `runtime build --id <runtime-ref>` is a direct reference-bound action. The
  separate `review runtimes` discover command returns the exhaustive catalog;
  redirected and JSON invocation remain read-only. Interactive review hands
  the unchanged Runtime or Runtime-revision reference to the exact build,
  restore, or interrupted-lifecycle action only after confirmation.
  `template runtime set --id <template-ref> --runtime
  <runtime-revision-ref>` is the separate exact binding mutation. Runtime
  preparation and Template selection never infer each other's target.
- Successful managed Runtime list, show, history, build, and review results
  expose an opaque `runtime-revision` reference for each eligible immutable
  revision. Built-in `standard` never exposes that reference because it is not
  a managed retirement target. `runtime restore --id` consumes the emitted
  reference unchanged, revalidates the complete lifecycle observation and
  retained source, and rebuilds with mutable base refresh disabled. Publication
  requires the rebuilt content digest to equal the recorded revision authority.
  Restore never appends or rewrites history, changes a Workspace Template, or
  changes a Workspace. An interrupted restore remains fail-closed and resumes
  through the same one-confirmation `review runtimes` recovery path.
- Runtime schema 1 publishes revision identity as `source_digest`, never the
  provisional `revision` alias. Each managed revision separately reports typed
  `availability`, `storage`, `last_used`, and `snapshot` evidence; `storage` is
  null for built-in `standard`. `ready` continues to mean that successful
  history exists and does not imply that head execution material is locally
  available. Docker image selectors, image digests, and private snapshot paths
  are absent from list, show, history, build, and redirected/JSON review output.
- Direct `config shell --id <template-ref>` changes one allowlisted
  shell-presentation policy in the exact referenced Workspace Template.
  `default` removes its override;
  `inherit` reads that exported variable from the host process launching each
  future `tobari` session; `literal` requires `--value` and preserves an
  explicit empty value. `--value` is invalid for the other sources. The fixed
  inventory is `PS1`, `TERM`, `COLORTERM`, and `NO_COLOR`; it excludes
  `PATH`, `HOME`, `BASH_ENV`, `ENV`, `PROMPT_COMMAND`, credential variables,
  and arbitrary names. A new V1 Workspace Template selects `PS1=inherit`. When the
  launcher has no exported `PS1`, the built-in `\h:\w\$ `
  prompt remains.
  Running sessions are unchanged, and no host startup file is sourced or
  mounted. Literal values are ordinary owner-only configuration and must not
  contain secrets.
- `config git --id <template-ref>` changes one atomic
  `user.name`/`user.email` fallback in the exact referenced Workspace Template.
  `default` removes Tobari's fallback; `inherit`
  resolves only a complete pair from host-global Git configuration for each
  stable Workspace root during reconciliation; `literal` requires both
  non-empty `--name` and `--email`. New Workspace Templates use `default`,
  and an absent or incomplete inherited pair adds no fallback. The projected
  system-scope value is lower precedence than Workspace global and
  repository/worktree configuration. No Git file/path/include directive,
  credential helper, token, HTTP header, SSH command, signing setting, hook,
  alias, URL rewrite, filter, proxy, or arbitrary key is projected.
- Both `config` commands require an exact opaque Template reference and one
  complete valid setting group. Partial input fails before mutation. The action
  revalidates the referenced Template under the lifecycle lock, so mutable name
  or default-selection changes cannot retarget the write.
- `template create --name NAME` creates one fresh Template from the reviewed
  built-in standard body. `template copy --from <template-revision-ref> --name
  NAME` is the distinct one-time copy initializer. Copy revalidates the exact
  retained revision and copies no Context, Policy Memory, Workspace, home,
  authentication, attachment, applied/failure/observed state, default
  selection, or lineage. Neither action reconciles Docker or the cluster.
- Bare root owns the only recommended first-use review. Its draft has no
  authority. Start crosses Template publication, default selection, Context
  creation, cluster activation, Workspace reconciliation, and child handoff as
  distinct canonical checkpoints. A later failure retains every earlier
  confirmed result and emits one causal Next action.
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
  immutable revision without selecting it in any Workspace Template. Existing Workspace Template
  bindings remain in force until a separate `template runtime set` succeeds.
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
  bundle installs its client. A selected custom Workspace Template runtime must provide
  the exact compatible client before its login can run. The released CLI
  carries the pinned combined-base recipe and builds it locally when absent.
  The base workflow validates Linux amd64 and arm64 with cache-only output;
  neither it nor the protected release workflow publishes an OCI image.
- The pinned client versions and `builtin/agent-ready` exact and semantic effect catalog are
  one compatibility contract. Its compile-time `claude_ready`, `codex_ready`,
  `gh_ready`, custom-runtime `twg_ready`, and custom-runtime `pup_ready`
  bundles are projected by the
  installed binary into exact native-authentication effects for every existing
  and future agent-ready Workspace Template;
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
- Workspace observation does not select or alter the Runtime. A Context
  resolves its bound Workspace Template's exact revision; its Workspace uses
  that revision when created and again during entry reconciliation. All
  selected images still pass the same compatibility checks before Docker
  mutation. If the bound Template's Runtime changes, the next Context entry
  preserves the Workspace home and authority record, recreates only the work container when the
  runtime spec changed, and records the new image only after reconciliation
  succeeds.
- Shared cluster mutations use one command-bound `tool_local` target and are
  owned by canonical `cluster up`; bare root may invoke that same application
  boundary as one separately reported checkpoint before Workspace entry.
- Bare root and bare `status` accept no Template, Context, Workspace, or root
  selector. They combine the installation default Template with canonical CWD
  and never guess from same-root siblings. Nondefault entry and every deletion
  consume opaque references from `context list/show` or
  `workspace list/status`.

## Output and exit contract

Human output is concise text. The canonical public machine-output inventory is:

<!-- public-cli-json-schemas:start -->

| Surface | Envelope | Schema |
| --- | --- | ---: |
| Structured error | `error` | 2 |
| Agent help (`view: index` and input-selected `view: scope`) | `commands` | 1 |
| Version | `build_identity` | 1 |
| Doctor report | `report` | 1 |
| Cluster activation result | `cluster_up` | 2 |
| Cluster status | `cluster` | 2 |
| Cluster denial window | `denials` | 3 |
| Cluster stop result | `cluster_down` | 2 |
| Policy candidates | `policy_candidates` | 2 |
| Policy review | `policy_review` | 2 |
| Policy rules | `policy_rules` | 2 |
| Policy mutation result | `result` | 2 |
| Workspace Template list | `templates` | 1 |
| Workspace Template report | `template` | 1 |
| Template, Context, or Workspace selection/deletion result | `result` | 1 |
| Context list | `contexts` | 1 |
| Context report | `context` | 1 |
| Context entry result | `entry` | 1 |
| Workspace list | `workspaces` | 1 |
| Workspace report | `workspace` | 1 |
| Default-pair status | `status` | 3 |
| Runtime list | `runtimes` | 1 |
| Runtime report (show/create/history/build results) | `runtime` | 1 |
| Runtime restore result | `runtime_restore` | 1 |
| Runtime delete result | `runtime_delete` | 1 |
| Runtime prune plan | `runtime_prune_plan` | 2 |
| Runtime prune result | `runtime_prune_result` | 2 |
| Service review | `service_review` | 1 |
| Service status | `service_status` | 1 |
| Confirmed Service exposure | `exposure` | 1 |
| Service browser-open request | `open` | 1 |
<!-- public-cli-json-schemas:end -->

Bare `status` is the CWD-first schema-3 home. It reports the selected
default Template, exact Context, independent desired/active Policy Memory and
Workspace AppliedEntry facts, Runtime and cluster observation, bounded
permission and Service summaries, one structured primary Next, and ordered
Attention. It performs zero mutation and returns no aggregate overall state.

`template list` returns the exhaustive Template collection and opaque Template
references. `template show` returns one exact current immutable revision and
its revision reference. Context and Workspace discovery return their own
distinct references. Human output leads with ordinary names and Current/Next;
JSON retains exact IDs, digests, nulls, empty collections, false, and bounded
unknowns. Presentation never reconstructs an action target from a name,
generation, image, container, label, order, or timestamp.

Template, Context, Workspace, Runtime, Policy, and Service results use the
schemas declared in the command table above. Native authentication validity is
not inferred: release output says `not_observed` and never exposes research
Broker or provider state.
Unconfigured cluster resources are `null`; unavailable
observations use declared finite values and never an empty-string sentinel.
The infrastructure/doctor label `linux_xdg_file` is not a public
auth or cluster JSON enum. Their items associate Context, bound Workspace
Template, Workspace ID, safe project root, HTTP effect, observation data
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
The release Catalog forms one executable causal recovery graph. Every command
Next names an existing Catalog task or a declared typed non-command condition;
it never appends unchecked required argv. Nonretryable self-loops, closed
reference cycles, action rediscovery, and mutation replay after unknown state
are invalid. A read-only classifier may be intermediate only when each of its
declared outcomes terminates at a causal action or condition. An
`output_encoding_failed` edge never recommends the same encoding-failing task;
`version` is the build-identity diagnostic and help remains Catalog-derived.

`version --format json` uses schema version 1 with envelope
`build_identity`. Its fixed fields are `version`, `commit`,
`resolver_channel`, `development_source`, `capability_surface`, required and selected Gateway
APIs, `compatible`, `development_build_command`, and
`development_binary`. An absent source commit is the explicit string
`unknown` and makes `compatible=false`. The two repository-command fields are
empty for an embedded resolver and contain exactly `task build` and
`bin/tobari` only when the compiled development metadata proves that path.
The Workspace selector is a human stderr interaction; it produces no JSON or
stdout selection protocol. A successful choice prints an English summary before
the child session, and cancellation or stale selection prints no success
summary. The interactive child owns all streams. After it exits, Tobari may
emit one clearly host-owned bounded cleanup diagnostic; cleanup failure never
replaces the child's exact exit status. `exit` therefore detaches the session
without deleting the Workspace. The Workspace remains existing until the host
discovers its exact reference with `workspace list` and runs `workspace delete
--id WORKSPACE_REF --confirm=delete`. If another session is attached, ordinary
deletion fails and `--force` overrides only that guard. There is no public
`stop` or `pause` state. The choice is
revalidated under the lifecycle lock before logical or Docker mutation, so a
changed candidate set fails closed and asks the user to run `tobari` again.
When a learnable network request is denied, the Gateway's 403 response carries
fixed secret-free host-review navigation for the agent, and an interactive
session close may summarize the pending queue on host stderr. These are
advisory only; the interactive `review permissions` queue is the human entry point.
It stages unchanged opaque review-item references only from typed detail screens and uses
one final `policy apply-reviewed` fixed-target action to revalidate and activate
one Workspace Template's set. Apply or discard is required before switching Workspace Template so the
source change remains one atomic domain-generation replacement. `policy rules` is the
current learned-decision inventory; its TTY reset flow delegates one explicit
opaque reference to `policy reset`. Redirected and machine-readable review and
inventory remain read-only. The Permission Inbox groups candidates by their
validated stable Workspace Template and Workspace identities, renders the Context-root scope
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
and no Workspace Template, host, path, reason, or other denial evidence enters the control
payload. A failed cue leaves watch active. Confirmed
output carries the active OPA
revision plus each ordered Context/effect/stored-rule decision and
directs the caller to retry in the current running Workspace. The public
read-only JSON review schema remains version 1 and does not expose this
internal TTY Apply receipt.
Research `bin/tobari-research serve` exposes the same human task in one foreground
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

The root journey's recommended review precedes progress and is not a progress
stage. Before child handoff, Tobari writes stable line-oriented progress to
stderr using exactly these semantic stages and routine labels:

| Stage | Routine label | Checkpoint proved by success |
|---|---|---|
| `check_requirements` | Check requirements | closed read-only root readiness |
| `resolve_context` | Save setup / Use Context | final desired Template/default/Context receipt |
| `prepare_protection` | Prepare protection | canonical cluster and active-policy receipts |
| `prepare_workspace` | Prepare Workspace | exact last-successful Workspace AppliedEntry |
| `enter_workspace` | Enter Workspace | successful child handoff |

Stage state is closed to `pending`, `running`, `succeeded`, `skipped`,
`blocked`, `failed`, and `unknown`. No checkmark implies a later checkpoint or
collapses desired, active, applied, and observed facts. A stage finishing within
250 ms need not show a running line; elapsed time appears after one second; one
bounded sanitized wait reason appears after ten seconds; redirected stderr may
emit a bounded heartbeat no more often than every 30 seconds. Progress exposes
no percent, ETA, raw external log, public flag, preference, schema, event
resource, or live details control.

Before handoff, caller interruption cancels the current canonical operation,
waits for its bounded settlement/classification, preserves any confirmed
mutation-complete result, reports retained facts plus one causal Next, and root
exits 130. An unknown mutation outcome never grants replay. After handoff, the
child owns stdin/stdout/stderr/signals and its exact exit status, including a
signal-derived status. On creation of a fresh Workspace only, shell entry may
emit one non-blocking stderr line that credentials stay in that Workspace;
direct children rely on their native prompts and no dismissal state is saved.

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

`template default set --id` changes only the installation default for later
bare root/status resolution. Template and Context mutations do not start Docker
or reconcile cluster, active policy, Workspace AppliedEntry, or observation;
those boundaries remain explicit.

Partial, mixed, unsafe, or contradictory final authority fails closed before
Docker mutation. Tobari does not invent a Context or Workspace, and it does not
suggest a deletion unless discovery produced the exact typed reference.

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
  -> workspace list
  -> workspace delete --id WORKSPACE_REF --confirm=delete
Workspace absent
```

`status` resolves the nearest canonical Context ProjectRoot before applying the
installation default Template. It distinguishes logical absence from an
existing Workspace whose runtime is missing, and reports attachment as a
separate typed fact rather than inferring it from labels or presentation
order. When several ancestor Workspaces exist,
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

Commands use complete delivery. `workspace list` is exhaustive for local
Workspace state at one observation point. `status` is a CWD-local scalar observation; cluster
status is exhaustive for the shared cluster scope. Logs are
a bounded recent window of 1 through 10,000 lines per selected component.
Denials are a fully delivered typed projection from the requested bounded
Gateway-line window. A denial-shaped record that cannot satisfy the strict
Context-bound typed contract is isolated rather than failing the whole
window, and `unparsed_lines` reports how many such records were skipped without
reflecting their untrusted contents. An empty `items` collection means no valid
denial occurred in that window, not exhaustive history; a nonzero unparsed
count preserves the distinction between a fully interpretable empty window and
one containing unprojectable evidence.
Policy candidates, review, and tail are bounded by the same retained
Gateway-line window and omit effects already covered by learned allow rules,
baseline deny rules, or exact learned deny rules. Baseline and exact denies
remain available as audit evidence but never become pending queue items.
Within that window, candidates aggregate by exact Workspace Template identity, project
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
Allows as evidence. Two distinct compatible HTTP paths with the same Context,
Workspace, scheme, host, port, method, and segment count may produce one inert
single-segment `{id}` proposal when exactly one safe raw segment differs and at
least one source is still pending. Repeated identical observations do not meet
that threshold. Ambiguous, shallow, percent-encoded, empty/dot-segment,
backslash, multi-segment, and GraphQL proposals are suppressed. Proposal
identity binds the proposed authority, while final Apply rebuilds its current
evidence before any mutation.

## Configuration contract

Configuration is resolved from
`${XDG_CONFIG_HOME:-$HOME/.config}/tobari` on macOS and Linux. Ordinary reads
are non-creating. Final Template/Context/Workspace authority is one owner-only
atomic envelope; it is not a directory of user-authored YAML and is never
selected from Project metadata.

One complete validated collection contains:

- Workspace Templates with stable `WorkspaceTemplateID`, immutable current and
  retained revision bodies, and semantic digests;
- one optional `DefaultTemplateSelection`;
- Contexts with stable `ContextID`, canonical ProjectRoot, immutable Template
  binding, current Policy Memory, and independently active receipts;
- Workspaces with stable `WorkspaceID`, Context binding, create-once defaults,
  last-successful AppliedEntry, and one bounded latest failure/unknown outcome.

Template revision bodies contain the immutable source/network Boundary,
baseline policy, exact Runtime revision binding, session defaults, and
creation defaults. Context Policy Memory is separate. Native credentials stay
inside one Workspace home. Research credentials are Context-owned and remain
outside Template desired/applied state.

A genuinely fresh read returns absent authority with no synthetic persisted
resource. Root may present one recommended draft, but only canonical Template
create, default-selection, and Context-create mutations establish authority.
Malformed, unsupported, unsafe, predecessor-only, partial, or changing state
fails closed before mutation; the final binary does not decode or adopt the
predecessor serialization.

Managed Runtime source and immutable snapshots remain installation-owned
separate resources. Runtime build changes no Template. Template Runtime binding
changes only through `template runtime set`; the next explicit Workspace entry
reconciles the entry slice while preserving the Workspace home.

OPA receives one complete validated projection per Context: the current
Template-policy slice and the independent current Policy Memory. The shared
projection is content-addressed and atomically activated by `cluster up`.
Exact permission mutations serialize with the same projection owner and retain
the prior known-good aggregate on failure. Status, list, show, doctor, and
completion never publish or repair it.

Host shell and Git projection is a closed, non-secret set. Shell accepts only
`PS1`, `TERM`, `COLORTERM`, and `NO_COLOR`; Git accepts only an atomic
`user.name`/`user.email` fallback. Typed AWS/EKS bootstrap stores only the
reviewed secret-free creation recipe for future Workspace homes. No host
credential, token cache, executable helper, startup file, arbitrary environment
name, arbitrary Git key, or generic kubeconfig crosses these boundaries.

Internal private Gateway/OPA/Broker/Host Loopback wire formats retain only
their explicitly versioned compatibility tokens. Those tokens do not become
public Template, Context, or Workspace schema aliases.
## Side effects

Catalog-declared reads create no Tobari-owned XDG files or Docker resources,
including on first use and under concurrent observation. They may remove only
a pre-existing, validated mutation journal while completing its bounded
recovery under an already existing or recovery-required lock; they never
create state merely to observe it. Workspace Template, policy, credential, auth, project,
and lock initialization belongs to declared create/write outcomes.

`runtime create` creates an owner-only installation Runtime source tree without
changing a Template, Context, or Workspace. Optional `--copy-source-from`
selects `standard` or one managed current editable source. Source validation,
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
history, and leaves every Workspace Template binding unchanged. BuildKit may retain
engine-owned cache layers, and a post-build validation failure may retain the
unselected candidate image; Tobari states this instead of deleting either one.
No build promotes into a Workspace Template. `template runtime set` is the only selection
mutation and revalidates one ready `standard` or `name@ordinal` revision before
atomically replacing the Workspace Template binding. Existing Workspaces are not mutated
by either operation; their next root entry reconciles the work container to the
newly selected image while preserving the Workspace home.
The official immutable base is pulled only for an explicit build whose recipe
starts from the exact resolver-selected release digest; custom and local bases
retain their local/cache-first behavior.

`template default set` validates the target Workspace Template and atomically
changes only the default selector. It never starts Docker, changes the aggregate, or
modifies an existing Workspace's Workspace Template, runtime, home, policy, or principal.
Creating a Workspace Template also never starts Docker; when shared state exists, the
result directs the user to explicit `cluster up` so the all-Context projection
can be validated and activated.

`template delete --id <template-ref> --confirm=delete` validates one exact
opaque reference under the installation lifecycle lock. It rejects the default
selection and every Template referenced by a Context. A confirmed delete
removes only that Template's identity and revision state, preserves Projects,
Contexts, Workspaces, Policy Memory, Runtimes, and shared resources, and never
chooses a replacement default implicitly.

`config shell` and `config git` atomically publish one complete revision of the
exact referenced Template after typed input, intent, target, and impact
validation. Git inheritance performs no Git read during the
configuration mutation; the next matching root reconciliation runs at most two
fixed, one-attempt, finite-time host Git queries, validates a complete pair,
and atomically refreshes one private per-Workspace fallback before Docker
mutation. Failure preserves the prior file and returns no raw Git diagnostic or
identity. The file's exact directory is mounted read-only as system scope and
includes the image system config before the Workspace Template fallback, preserving
normal Workspace-global and repository/worktree precedence.

`config bootstrap aws` atomically updates a separate secret-free recipe in the
selected Workspace Template. It reads only host `~/.aws/config`, accepts one strict IAM
Identity Center profile/session subset, and reports only adapter, profile,
generation, revision, and changed-field metadata. Configure and refresh affect
future Workspace creation only; remove stops future projection. No variant
reads or outputs AWS credentials or SSO cache values, invokes AWS, performs
login, changes network policy, or rewrites an existing Workspace home. A new
Workspace receives one canonical private `.aws/config` and records its applied
semantic revision before logical publication.

The argument-free Workspace Template wizard reuses this resolver through a read-only
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
`AWS_PROFILE` equal to the Workspace Template AWS profile. It rejects credentials, proxy or
insecure TLS options, file references, arbitrary exec fields, role arguments,
unknown fields, alternate paths, and unsafe files. Projection emits canonical
private `.kube/config` JSON in the fresh Workspace home. Removing EKS preserves
AWS; AWS cannot be removed or changed to another profile until its dependent EKS adapter is removed. No
configure, refresh, or create operation calls AWS or Kubernetes or grants a
network effect.

After an AWS candidate is selected, the Workspace Template wizard discovers only
kubeconfig contexts compatible with that exact AWS profile and semantic
revision. `Do not configure Amazon EKS` is the first explicit choice;
incompatible contexts remain visible but unselectable. Discovery performs no
network or subprocess call and reads no credential or cache state.

`cluster up` obtains and preflights the immutable Gateway image and official
runtime bases required by all Workspace Templates, generates and validates the complete
aggregate policy/routing projection, then creates shared labeled networks,
configuration material, exactly one Gateway, exactly one OPA, and CA volumes
as needed. It reconnects Gateway to the shared
networks and existing registered project networks without creating project
state or project resources and waits for both services to be healthy. The
root command only verifies the shared cluster is
configured and ready, reads the indexed Workspace candidates, and waits for an
explicit choice when the canonical current directory is below an ancestor.
After the choice is revalidated under the lifecycle lock, it creates or reuses
the selected Workspace Template-bound logical record, resolves the bound Workspace Template's narrow
Git identity fallback for that stable root, resolves and validates its bound
Workspace Template image before project runtime mutation, reconciles its labeled container and internal
network, binds its XDG home, joins Gateway to that network, waits for the
project healthcheck and enters the resulting terminal session. Docker create
appends Tobari's fixed `sleep infinity` lifetime command after the image; the
image `CMD` is not used to own Workspace lifetime. Bash and direct commands
run through child exec sessions. A direct command is passed as exact argv after
Docker's `--`, without a shell, and never becomes PID 1. Each child exec late-binds only the
bound Workspace Template's declared shell-environment inheritance and applies its fixed
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
Workspace deletion: it returns the exact child exit status. A failed attachment
cleanup is one additional bounded host-owned stderr diagnostic and never
replaces that child status. `workspace delete --id WORKSPACE_REF
--confirm=delete` is the separate lifecycle-ending operation and removes only
that exact label-owned container, network, Workspace record, and home after
confirming that no session is attached; `--force` overrides that one guard.
`cluster down` rejects while any
Workspace remains
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
coordinate when applicable, and the Workspace Template policy ceiling already satisfies the
orthogonal boundary.
Candidate
discovery excludes other denials, preventing a successful no-op approval.

In the research build only, `auth login`, `auth import`, and `auth logout`
validate the fixed installation
credential-catalog target and mutation impact before acquisition or vault I/O.
Login selects only the active profile's closed provider union through an
interactive trusted-host terminal. It includes GitHub, Datadog, OpenAI,
Anthropic, Chatwork import, and AWS. Anthropic alone
executes in a fresh selected-Context Template-image container; each driver owns
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
before broker send. Login/import atomically replace one typed Context/provider
grant and revoke every prior handle. Gateway performs
non-secret introspection before OPA and applies exactly one same-revision
static resolution, Datadog/OpenAI/Anthropic token result, or bounded AWS SigV4 result only
after allow. Gateway makes one upstream attempt. Managed profiles and arbitrary
manifest-selected dynamic execution do not exist. Logout atomically removes
the record and its handles without contacting the provider.
One credential is Context/provider-owned, and its Workspace is eligible for a
distinct handle only on its next matching entry;
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

Each final Workspace Template, Context, Workspace, Policy, and authentication
reader accepts only its exact declared schema. Unchanged public and internal
boundaries keep their independently owned versions; a shared numeral is not
cross-surface compatibility. Tobari provides no retired command alias, implicit
old-state interpretation, migration fallback, or general compatibility shim.

Before the first public release, a genuinely fresh installation is one where
the final owner store is absent and a bounded fixed-path presence guard proves
that no predecessor desired-definition, Workspace, Policy, Broker, principal, or private
session authority is present. That state is exact empty final authority and may
create its first Template and Context. A complete final store is the only
ordinary authority source. Predecessor bytes never contribute identity,
policy, Runtime protection, principal/session state, or credentials.

Any declared legacy presence, unsafe path, or ambiguous observation fails
closed before final initialization or mutation and returns explicit
reset-and-recreate guidance. The guard observes only bounded path/type/owner
facts needed to establish presence; it does not decode, decrypt, transform,
quarantine, rename, delete, or adopt predecessor content. Destructive reset is
an explicit user action outside this cutover. Research use creates fresh
Context-owned credentials only through explicit login/import; matching legacy
IDs or bytes are never rebound.

This clean-break rule is valid only because Tobari has no public release. It is
not precedent for changing released persistent state. Compatibility and
migration requirements after the first public release remain undecided and
must be fixed by an explicit future release-policy decision before an
incompatible change.

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
The research Broker slice supports GitHub, Datadog, OpenAI, Anthropic,
Chatwork, and AWS and retains one credential per Context/provider. Owner
manifests may express another single static primary secret only through the
exact HTTPS/header replacement contract and protected stdin import. V1 has no
managed adapter, multiple provider accounts, provider-specific policy
semantics, Git credential helper, manifest-selected helper, or general
provider SDK/plugin executor. Standard tools authenticate natively inside their
Workspace-owned home; that state is neither brokered nor a network grant.
