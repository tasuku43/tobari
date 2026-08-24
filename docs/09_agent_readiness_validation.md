# Agent Readiness Validation

This is the executable first-public-V1 journey contract. It supplements
automated tests; it does not permit live credentials or authenticated
transcripts as repository fixtures.

## Required outcomes

| Outcome | Public route | Success evidence |
|---|---|---|
| Discover capabilities | `help --format agent`, then one namespace or exact-command selector | Root remains a compact capability index; one scoped read supplies complete typed inputs, outputs, failures, and workflow |
| Understand the current Project | `status [--format text\|json]` from the Project root or a descendant | One read selects the nearest existing Context ProjectRoot before the installation default Template, preserves desired/active/applied/observed axes without an overall status, returns one primary Next plus ordered Attention, and needs zero external joins or reconstruction; fresh no-default state performs zero Docker and owner calls |
| Choose and retire a Workspace Template | `template list`, `template show`, `template copy`, `template default set`, `template delete` | Human list identifies the default; copy consumes one exact immutable revision reference and publishes a fresh generation-1 ID with no lineage or lower-lifetime copy; changing the default does not retarget Contexts or Workspaces, and deletion rejects default or Context-bound Templates |
| Prepare a reusable Runtime source | `runtime list`, then `runtime create --copy-source-from NAME --name NAME` | Scoped help identifies `standard` or one managed current editable source; creation returns a fresh Runtime ID with empty history and no lineage, performs no build or Manifest/Workspace change, and needs zero revision decoding or source reconstruction |
| Review, reclaim, and recover Runtime material | `review runtimes`; `runtime prune dry-run`, then `runtime prune apply --plan PLAN_REF --confirm=prune`; `runtime restore --id REVISION_REF`; `runtime delete --id RUNTIME_REF --confirm=delete` | One scoped help read plus one local discovery yields every exact opaque input; dry-run is zero-write and exhaustive, apply consumes the unchanged plan, restore reconstructs exact retained content, and whole deletion preserves Manifest, Workspace, Workspace home, project, and credential authority while protected, unknown, shared, or standard targets fail closed |
| Enter bounded work | `tobari` or `tobari -- COMMAND [ARG...]`; explicit `cluster up` remains available | The root command binds canonical CWD plus the exact installation-default Template, creates only the missing final Context/Workspace authority through its reviewed path, and enters one reusable Workspace; the direct form runs exact foreground argv without a shell, returns its status to the host, and leaves the Workspace reusable |
| Understand authority lifetime | `context show --id CONTEXT_REF`, `policy rules`, and Host Loopback capability/review output | Routine guidance distinguishes Workspace Template policy, remembered Context decisions, and this-session Host Loopback access without requiring baseline, overlay, principal, epoch, or grant reconstruction; typed output retains exact destination kind and authority lifetime |
| Grow exact permission | `review permissions`, or `policy candidates` then one exact allow/deny | Terminal guardrail precedes every candidate; explicit review activates only exact Workspace Manifest/project/scheme/host/port/method/path authority |
| Resume after reviewed denial | In the attached Workspace run the exact `tobari-permission wait --id pwt_...` printed by one eligible ordinary HTTP/HTTPS denial; in a separate trusted-host terminal review and Apply; after `Allow`, deliberately retry the workload | Wait returns only `Allow`, `Deny`, or lease `Expired`; the helper has no proposal, decision, mutation, discovery, or retry authority; the fresh request receives an independent Gateway authorization |
| Open one Workspace service | In the attached Workspace run `tobari-expose PORT`; in a separate host terminal run `tobari review services --watch`; later use helper/host status, open, and stop | One action key/token confirms the complete effect card; Allow returns a generated per-exposure `.localhost` root URL plus an independent opaque lifecycle reference; Open is separate, Stop consumes the reference unchanged, and Permission review remains unrelated staged Apply |
| Inspect/reset decisions | `policy rules`, then `policy reset --id` | One current exact decision is removed through its unchanged opaque reference and returns to default deny |
| Use native Workspace auth | Run the agent CLI's native login inside the Workspace | Credential state persists in that Workspace home, receives no network grant from login, and crosses Gateway only after the ordinary exact HTTP effect is allowed |
| Exercise research Broker (repository-only) | Build with `task build:dev`, then use `bin/tobari-research` `auth` commands | No equivalent command, provider binding, projection, service, image authority, or activation switch exists in the release binary |

Routine success must require zero undeclared external-processing steps. Reading
a declared JSON/TSV field is consumption; a custom join/parser, provider-
notation decoder, source inspection, or exploratory request is reconstruction
and fails the supported-outcome claim.

For trusted-host review discovery, one `help review --format agent` lookup
must return exactly `review permissions` and `review services`. A caller that
already knows either leaf needs one exact scoped-help lookup and then one task
invocation, with zero command guesses, selector interaction, or external
processing. Bare `tobari review` is the equivalent human namespace listing and
must perform no Permission or Service read.

## Reproducible local scenario

Use a temporary XDG configuration/state/data root, synthetic credentials,
local test upstreams, and the contributor development images. Do not use a
developer account or live network.

```sh
task check
task security
task integration:test

TOBARI_BIN=bin/tobari
"$TOBARI_BIN" help --format agent
"$TOBARI_BIN" help manifest --format agent
"$TOBARI_BIN" manifest create --name writable \
  --source-access read-write \
  --native-readiness enabled --format json
"$TOBARI_BIN" manifest create --name restricted \
  --source-access read-only \
  --native-readiness disabled --format json
"$TOBARI_BIN" manifest show --name writable --format json
"$TOBARI_BIN" manifest show --name restricted --format json
```

Record the invocation count, source of every input, output field consumed by the
next task, and routine-success external-processing count. Verify every emitted
opaque ID passes unchanged to its consumer.

For Workspace Manifest lifecycle classification, retrieve `help manifest --format agent`
once. Verify scoped help presents source access, complete method policy, policy
mode, and native-readiness selection as creation-time Boundary inputs; presents
`manifest runtime set` as the sole exact Runtime-binding replacement with
next-entry adoption; presents shell/Git settings as later session defaults; and
presents bootstrap as future-Workspace creation only. The journey must require
no schema inference or source inspection. After each mutable operation, verify
the stable Workspace Manifest ID and every bound Workspace identity/home remain unchanged.
Also create one Workspace Manifest with `--copy-from` and verify the new stable ID differs, the
source remains unchanged, the reviewed Boundary/Runtime/defaults match, and
neither typed output nor catalog reference flow claims ancestry or inheritance.

For Runtime source creation, retrieve `help runtime create --format agent`
once, use `standard` or one exact managed name returned by `runtime list`, and
create one new Runtime. Verify the target has a distinct stable ID, empty
history, independent editable bytes/modes, no lineage/source field or reference flow, and
that neither Docker nor any Workspace Manifest mutation ran. The known-path journey uses
one scoped-help read, one create invocation, and zero external processing.

For Runtime lifecycle closure, retrieve `help runtime --format agent` once.
Use `runtime list`, managed Runtime history, or redirected `review runtimes` to
obtain exact Runtime and revision references; use `runtime prune dry-run` to
obtain the plan reference. Pass each opaque value unchanged to its declared
consumer. Verify the read projection uses `source_digest`, keeps successful
history readiness separate from head availability, and contains no
`revision`, `image`, `image_digest`, or `snapshot_path` field. Verify dry-run
leaves a fresh XDG tree byte-identical and makes no
Docker mutation, while a confirmed prune applies only the unchanged plan.
Exercise one protected current or retained Manifest edge, Workspace applied,
pending, and observed edges, external/shared image use, an unavailable retained
revision, and an unused zero-revision Runtime. Restore must publish only the
recorded digest and leave history unchanged. Whole deletion must preserve every
Manifest, Workspace ID, Workspace home, applied receipt, project root, credential, and
shared resource. Interrupt each mutation once and resume through the same
reference or the single confirmed `review runtimes` path. Record zero external
processing and no use of Docker tags, image IDs, container IDs, names,
ordinals, paths, timestamps, or `head` as authority.

For the direct-entry route, retrieve `help tobari --format agent` once and then
invoke one known command as `tobari -- COMMAND [ARG...]`. Record one discovery
read, one task invocation, and zero external-processing steps. Replay with a
duplicate flag, a dash-prefixed value, and an explicit empty argument; verify
the child observes the exact argv and status, no shell is inserted, and the
next host command runs instead of entering Workspace Bash. Also verify bare
`--` performs no setup or Workspace mutation.

For causal failure recovery, run the same typed Docker-unavailable fixture in
human and JSON error formats. Verify identical `kind`, `code`, `phase`,
`change_state`, retryability, and exact `doctor` action; no provider name,
context name, socket path, or raw cause may appear. The failure must occur after
first-use review and before Workspace Manifest creation, with zero Workspace Manifest, cluster,
Workspace, network, and Docker mutations. Replay Engine versions 23, 24, and a
malformed value through the generic observation port. After any mutation fault
whose state is `partial`, `confirmed`, or `unknown`, execute its declared read
before choosing another mutation. A direct child's nonzero status remains the
exact child status and emits no Tobari structured error.

For authority lifetime, read the routine permission guidance once, then inspect
one Context, one persistent learned decision, and one Host Loopback review item.
Classify them only as Workspace Template policy, a remembered Context decision, or
this-session Host Loopback access. Verify the learned decision remains until
`policy reset`, while the Host Loopback decision disappears when its owning
attachment exits. Confirm `destination_kind` and `authority_lifetime` carry
that distinction in machine output without parsing a label or reconstructing
an Attachment Epoch. The routine-success external-processing count is zero.
The Host Loopback request uses exact `http://host.tobari.internal:{port}` and
preserves that Host authority through the reviewed relay. Verify Workspace
`localhost` remains Workspace-local, opposite-direction service exposure uses
its generated per-exposure `.localhost` authority, and `host.docker.internal` never appears as
public or policy identity. Exact retired `host.tobari.test` must be terminal,
non-learnable, and absent from the review queue.

For permission resume, trigger one synthetic reviewable ordinary HTTP denial
inside an attached Workspace and copy its exact `tobari-permission wait --id
pwt_...` command without parsing or reconstructing the ID. Start that command
inside the same attachment, complete one separate trusted-host Permission Inbox
review and Apply, and verify the helper returns only `Allow`. Then issue one
deliberate fresh request and prove Gateway evaluates it again. Repeat with an
explicit reviewed Deny and lease expiry. Verify an unknown, consumed,
cross-attachment, owner-lost, protocol-derived, Host Loopback, and malformed ID
never becomes `Allow`, `Deny`, or fabricated `Expired`; no path proposes,
approves, mutates policy, injects terminal input, or retries the request. Record
one denial-to-wait-to-readiness task, one separate trusted-host review mutation,
zero reference discovery, and zero routine external-processing steps.

For Workspace service exposure, start a synthetic HTTP/WebSocket fixture on one
non-privileged Workspace-loopback port. Run `tobari-expose PORT` in the attached
shell and obtain one bounded Service snapshot through `tobari review services
--watch` in another host terminal. Verify Back and cancellation create no
listener, Deny resolves without access, and one `a`/`allow` action returns a
URL with scheme `http`, authority
`svc-<128-bit-random-lowercase-label>.localhost:<random-port>`, root path `/`,
and an independent opaque `exp_...` reference. Verify `o` completes Allow
before separate Open and reports its bounded dispatch outcome. Use the URL
without rewriting Host, cookies, or Origin, verify WebSocket Upgrade, pass the
reference unchanged through status and stop, and confirm attachment exit
closes every listener and stream with only bounded count receipt output.
Record two declared task invocations plus review, zero identifier reconstruction,
zero automatic discovery/probing, and zero routine external-processing steps.
Before these helper journeys, prove both mounted helpers are dedicated
hardcoded Linux Programs extracted from the verified engine-native base:
checked source/module snapshot, pinned-builder amd64/arm64 construction,
per-binary source/API/digest and ELF architecture identity, owner-only host
storage, read-only standard/custom-Runtime mounting, spoofed-`argv[0]` denial,
and bounded extraction cleanup.

## Source-access matrix

Create read-only and read-write Workspace Manifests for the same canonical root. The
read-only Workspace Manifest must:

- read source bytes successfully;
- fail create, content change, delete, rename, chmod, and Git metadata writes;
- write successfully below its Workspace home and declared tmpfs;
- expose no writable source alias in mount inspection;
- include `source_access` in the runtime desired-state hash and Docker inspect
  reconciliation;
- observe later host changes and changes made through the same-root read-write
  Workspace Manifest.

The last observation proves a live direct bind, not a snapshot or filesystem-
integrity boundary. Neither Workspace Manifest is allowed to mutate the other's home,
network, or policy state. Native credentials follow the owning Workspace home.

## Workspace Manifest policy matrix

Inspect and bind the immutable Workspace Manifest policy snapshot and its complete method
default/override set. The fixed agent-ready baseline is part of the trusted
binary's default Workspace Manifest policy, not a selectable profile.

- `builtin/agent-ready`: the pinned Claude/Codex native matrix, GitHub CLI
  device-auth bootstrap, TWG CLI 1.2.5 auth/site/manifest lifecycle, and pup
  1.10.7 default-US1 DCR/token login when supplied by a custom runtime
  succeed without a permission candidate, including
  Claude capability discovery and one safe
  `/api/eval/{id}` shape plus Codex MCP initialize/list methods. Exact Deny
  overrides baseline. A Codex `tools/call` produces one exact tool-name review
  candidate, a different tool does not reuse it, and arguments/canaries never
  appear in OPA input, audit, denial, or stored policy. Downloads, acquisition,
  file transfer, self-update, unrelated paths, and third-party destinations
  remain denied or reviewable. Prove grants apply by Workspace Manifest semantic identity,
  not executable name.
- A deny-only Workspace Manifest produces terminal denials for every method and leaves the
  review queue empty.
- An exact-review Workspace Manifest sends eligible effects to exact review without
  granting immediate authority.
- A GET-only Workspace Manifest uses default Deny with an exact-review `GET` override;
  `HEAD` and every non-GET remain terminal denials. Do not describe GET as safe
  or read-only.
- A method-Allow Workspace Manifest still remains bounded by the destination ceiling and
  exact Deny; Method Allow is Workspace Manifest-wide, not process identity.

The native-login subset must include exactly:

- Claude: `GET platform.claude.com/v1/oauth/hello`,
  `POST platform.claude.com/v1/oauth/token`,
  `GET api.anthropic.com/api/oauth/claude_cli/roles`, and
  `GET api.anthropic.com/api/oauth/profile`;
- Codex: `POST auth.openai.com/oauth/token`,
  `POST auth.openai.com/api/accounts/deviceauth/usercode`, and
  `POST auth.openai.com/api/accounts/deviceauth/token`;
- GitHub CLI: `POST github.com/login/device/code`,
  `POST github.com/login/oauth/access_token`, and GraphQL `query` root `viewer`
  at declared exact endpoint `POST api.github.com/graphql`;
- TWG CLI: `POST auth.atlassian.com/oauth/device/code`,
  `POST auth.atlassian.com/oauth/token`, `POST
  auth.atlassian.com/oauth/revoke`, `POST
  api.atlassian.com/accessible-products`, GraphQL `query` root `me` at
  declared exact endpoint `POST api.atlassian.com/graphql`, and `GET
  teamwork-graph.atlassian.com/cli/manifest.json`;
- pup: `POST api.datadoghq.com/api/v2/oauth2/register` and
  `POST api.datadoghq.com/oauth2/v1/token`.

The compile-time review bundles are exactly `claude_ready`, `codex_ready`,
`gh_ready`, `twg_ready`, and `pup_ready`, coupled to the five reviewed client
versions. TWG and pup remain custom-runtime-only and the bundles are not
installation claims.
Prove new normalized Workspace Manifest snapshots contain no readiness rule, legacy
agent-ready snapshots retain their exact bytes, aggregate generation removes
every historical bundle form and projects only the current binary set, and no
runtime bundle or executable selector exists. Prove the dedicated family
catalog has unique IDs, explicit pinned client versions, positive unique
append-only contract revisions, and exactly one current contract per
family. Prove the aggregate revision includes its
effective expansion, an older active revision is reported invalid, and root
entry returns exact `cluster up` recovery before Workspace mutation.
Create Workspace Manifests with enabled and disabled readiness while varying the complete
Workspace Manifest method policy. Enabled readiness is independent of any profile name,
but destination ceilings and method Deny filter it, and exact Deny remains
terminal. Prove every method uses its explicit override or the Workspace Manifest default,
including an extension-method canary. Disabled readiness supplies no overlay.
An omitted readiness value resolves to the default Workspace Manifest without
rewriting the stored policy snapshot. For GitHub,
neighboring methods, paths, query variants,
GitHub API hosts, ordinary HTTP at `/graphql`, mutation, sibling or mixed roots,
Git transport, downloads, uploads, releases, and self-update receive no baseline
grant.

For TWG, prove one dedicated schema-v1 opener request carries only the strict
HTTPS Atlassian activation URL and opens once after native confirmation. Verify
the owned Workspace and make zero callback-listener calls. Duplicate keys,
unknown fields, malformed versions, host case, userinfo, explicit port, path,
encoded/empty/duplicate/extra query, fragment, neighboring targets, replay past
the attachment budget, and oversized requests open nothing. Prove only GraphQL
`query` root `me` at the declared exact endpoint
receives baseline authority. Ordinary HTTP at that route, mutation, sibling or
mixed roots, REST, telemetry, and neighboring OAuth requests receive no
baseline grant. Prove only exact `POST /accessible-products`, `POST
/oauth/revoke`, and stable `GET /cli/manifest.json` are added by TWG contract
revision 2; alternate methods, REST routes, beta manifest, installer, checksum,
artifact, update execution, and download receive no baseline grant.

For pup, prove the dedicated opener accepts only the exact default-US1
authorization route, seven mandatory query fields, an optional single
UUID-shaped `dd_oid` organization hint, bounded DCR/state/PKCE shapes, `S256`,
a sorted subset of the complete 110-scope pup 1.10.7 ceiling,
and exact `127.0.0.1:{8000,8080,8888,9000}/oauth/callback`. Verify it binds
before browser open and relays one opaque callback only to the selected owned
Workspace. Reject caller-added scopes, alternate sites, host case, userinfo,
explicit port, neighboring path, missing mandatory fields, duplicate or
malformed hints, unknown query fields, fragment, callback
host/path/port changes, replay, and oversized requests. Product APIs,
telemetry, revoke, and neighboring OAuth effects receive no baseline grant.

For each terminal denial, record zero permission candidates, external DNS
lookups, and upstream attempts. Repeat with a learned
exact allow, baseline grant, and Advanced Rego allow that would otherwise match;
none may bypass the Workspace Manifest policy ceiling.

Workspace Manifest-policy tests use strict owner-only schema-V1 data. Reject unknown fields,
wildcards, IP/private destinations, secrets, shell, Rego, include, inheritance,
remote fetch, refresh, signing, symlinks, unsafe modes, duplicate keys, and
ambiguous rules. Workspace Manifest creation normalizes, validates, digests, and snapshots
the Workspace Manifest policy. Editing policy source afterward must not change the existing
Workspace Manifest report or active policy ceiling. Updating the trusted binary must update only
the native-readiness overlay of existing enabled Workspace Manifests without rewriting
their snapshot.

For typed Workspace bootstrap, use only synthetic host homes. Prove the AWS
adapter reads one fixed shared-config file only after explicit bootstrap
editing, parses it once, resolves profiles through shared referenced SSO
sessions, exposes typed available/unavailable candidates, and rejects unknown
keys, helpers, duplicates,
symlinks, unsafe modes, oversized input, credentials, and cache material. A new
Workspace must receive exact owner-only canonical `.aws/config` bytes and an
applied revision before publication. After a semantic Workspace Manifest refresh, prove
the existing file is byte-identical and reports `older`, while a newly created
Workspace receives the new revision. For the dependent EKS adapter, prove one
explicit context in fixed `~/.kube/config` resolves only an inline CA,
commercial EKS origin, optional namespace, and exact `aws eks get-token` contract
with matching `AWS_PROFILE`. Reject tokens, keys, client certificates, auth
providers, proxy/insecure TLS, file references, aliases, arbitrary exec/env/role
arguments, unsafe paths, duplicates, and source drift. A composed new Workspace
must receive canonical private `.kube/config`; removing EKS preserves AWS and
removing AWS first is rejected. No test may perform external AWS or Kubernetes
I/O.
Whole-file malformed or unsafe input must return no partial candidates;
individual semantic incompatibility may remain visible but unselectable. Final
Create must revalidate the selected profile/session/EKS semantic bundle, return
the draft to review on selected-source drift, and ignore unrelated profile
changes. The ordinary no-bootstrap path performs no host configuration read.
Workspace receives the new revision and reports `current`. A staged refresh
whose source changes before Apply must make zero Workspace Manifest and Workspace writes.

The canonical contributor base must run `claude --version` as 2.1.220,
`codex --version` as 0.147.0, and `gh --version` as 2.96.0 after replacing
`/var/lib/tobari` with a fresh Workspace home. The client pins and agent-ready core matrix are reviewed as one
contract. Verify the base workflow is validation-only and the protected
Release workflow has no runtime registry login or publication path. Pending
redistribution review remains explicit artifact metadata; it never becomes a
dormant switch that can publish the combined base.
TWG and pup are intentionally absent from this canonical-base assertion; a
custom runtime claiming `twg_ready` compatibility must provide TWG CLI 1.2.5,
and one claiming `pup_ready` compatibility must provide pup 1.10.7.

## Reviewed policy journey

Generate one learnable denial from a running Workspace. Verify the child sees
only bounded secret-free host-review navigation and that no candidate ID or
unchecked argv is embedded in the response. Keep the Workspace terminal
attached, but use a separate trusted-host terminal for Permission Inbox;
Tobari reserves no `Ctrl+]` or other child-input shortcut. In that terminal:

1. Open `review permissions --watch`; prove an empty raw-terminal Inbox receives a
   new bounded candidate without restart and emits at most one fixed trusted
   terminal cue. Prove `--notify=off`, explicit OSC 9/BEL, conservative auto,
   identified cmux auto-selection, and hostile evidence isolation. Prove two
   unchanged timer refreshes keep one alternate-screen frame and emit no
   repaint, while a changed typed snapshot redraws. One distinct path remains exact,
   while a second
   compatible distinct HTTP path produces one typed `/path/{id}` proposal.
2. Stage exact Allow and Deny directly from the list, clear or overwrite one,
   and prove no mutation occurs. Inspect the Workspace Manifest/project/effect detail. Prove the proposal states that
   future single-segment values are included and offers Allow template, Allow
   observed exact, and Deny pending exact. Staging grants nothing.
3. Refresh and prove decisions remain bound by typed review-item ID, never by label,
   order, or indentation.
4. Confirm one final ordered Apply and observe the authoritative active
   revision, then prove watch returns to a fresh waiting snapshot.
5. Retry in the same running Workspace.
6. Inspect `policy rules`, reset the exact or template rule, and prove the request returns
   to default deny and becomes reviewable again.

Machine replay uses `policy candidates`, `policy allow --id`, `policy deny
--id`, and `policy reset --id`. The ordinary identity is exact Workspace Manifest,
project, scheme, host, port, method, and raw path; GraphQL adds operation type
and root field. Query, headers, body, and repeated identical observation count do
not widen authority. Prefix rules, compaction commands/references/state, and
dormant prefix fallbacks must all be absent.

## Standard native-login regression

Run Claude Code, Codex, GitHub CLI, and AWS CLI native login in a fresh standard
Workspace home, and TWG CLI 1.2.5 plus pup 1.10.7 native login in compatible
custom-runtime Workspaces. The test boundary does not retain credentials or authenticated
transcripts.

For Claude, prove token exchange is followed by an allowed authenticated
`GET /api/oauth/profile` and roles request. The regression fails if Gateway
returns `broker_auth_required`, removes Authorization before the allowed
upstream request, exposes it to OPA/audit, or synthesizes account metadata.
After login, Claude `/status` must obtain provider-owned `subscriptionType` and
`rateLimitTier` rather than null values and must not mislabel a subscription as
`Claude API account` because of Tobari interception. Ordinary Claude Code
2.1.220 login in an attached Workspace must invoke the shared opener and open
its exact validated authorization URL once on the host without observing child
output or creating a host callback listener. Changed client, redirect, scope
set, PKCE shape, host, path, duplicate, malformed, or oversized requests open nothing.

For Codex, prove both the browser callback flow's token exchange and the
device-code user-code/poll/token exchange use the same post-policy passthrough
boundary. The browser flow must run as ordinary `codex login` inside the
attached Workspace, open the exact validated URL on the host, and complete the
validated dynamic localhost callback without a manual URL/callback transfer or
device-mode substitution. The same outcome must hold when ChatGPT sign-in is
selected from ordinary `codex`: its exact reviewed TUI originator and an
authorization URL fragmented across ANSI synchronized-update writes and visual
rows must open once, independent of surrounding prose, cursor positions, and
terminal width. Before login, ordinary `codex` must render its native
interactive screen rather than block on terminal capability queries; prove the
observed Docker child still sees a TTY and receives the caller's initial size
and subsequent resize. Vary the surrounding client prose and callback port,
and prove the OAuth client remains exact while scopes cannot exceed the reviewed
ceiling.
Prove the host listener exists only for that login, relays one opaque callback
to the selected Workspace's same port, and closes on success, failure, or
session exit. The exact allow is shared by every process in the Workspace Manifest; the
test must not infer authority from the `codex` executable name.

For GitHub CLI 2.96.0, exercise the preferred GitHub.com HTTPS device-login path
and its web-application fallback. For the device path, prove the client retains
native Enter, then invokes the attachment-scoped opener carrying the independently
validated `https://github.com/login/device` target and opens once
on the host, the selected
Workspace is verified, and no callback listener is created. Reject neighboring
paths, queries, hosts, duplicate keys, malformed requests, replay, and oversized
targets. Exact default `gh auth login` must use the pinned real
client with fixed GitHub.com HTTPS login and Git credential setup argv, retain
native Enter, and invoke only the shared opener as its Workspace browser; other argv must
pass through unchanged. The wrapper must not force color on or off: GitHub CLI
retains native TTY detection and honors an inherited `NO_COLOR`. The one-time
code remains provider-owned visible child output and never enters Tobari state,
logs, policy, or fixtures.

For the web-application fallback, prove one strict fixed-client URL with the
required `repo read:org gist` scopes and optional `workflow` opens immediately
on the host through the dedicated opener, one
opaque callback reaches the exact selected Workspace's dynamic
`127.0.0.1/callback` port, and GitHub CLI retains its result presentation.
All child input remains Docker- and client-owned; no `c` or clipboard shortcut
is part of the bridge. Explicit non-default native login retains its own Enter input. Changed client,
caller-added or SSH-key-upload scope, GitHub
Enterprise host, malformed state, external/privileged callback, duplicate,
ambiguous, oversized, replayed, port-collision, opener-failure, callback-failure,
and session-exit cases must open or relay nothing beyond the declared one-shot
effect. The bridge itself grants no Workspace HTTP permission; `gh_ready`
supplies only the two exact device bootstrap/exchange POSTs and exact GraphQL
`query` / `viewer` effect. Routine
success uses one
`gh auth login` invocation, no manual URL/callback transfer, and zero external
processing.

For TWG CLI 1.2.5, replay the exact device-code, token, site inventory, stable
manifest, and revoke effects plus its synthetic current-user document without `Content-Length`. Prove the
declared GraphQL endpoint buffers it under the fixed transport cap, rejects an
actual body over 1 MiB, and emits only `query` / `me` to policy. Client
authentication is absent from OPA and denial evidence, the allowed upstream
request retains it unchanged, and the identity lookup does not produce a
route-wide HTTP grant. Routine success uses
one `twg login` invocation and zero external processing; `twg logout` can revoke
the token without review; later provider effects are classified separately.

For pup 1.10.7, replay exact US1 DCR registration and token exchange with
synthetic responses for both a first login without an organization hint and a
repeat login with a remembered UUID-shaped `dd_oid`. Prove each strict
authorization URL opens once, one opaque
callback reaches only the selected Workspace on each of the four fixed ports,
and the complete compiled scope ceiling plus reduced/read-only subsets remain
accepted while caller-added scopes and alternate sites fail closed. Routine
success uses one `pup auth login` invocation and zero external processing;
later Datadog product effects are classified separately.

For pinned AWS CLI 2.36.11, replay one synthetic default IAM Identity Center
authorization-code opener request. Prove the exact commercial-region OIDC URL
opens once, the host binds the URL-selected dynamic non-privileged port before
browser open, and one opaque callback reaches only the label-verified selected
Workspace's same `127.0.0.1/oauth/callback` listener. Reject alternate
partitions, region/host case, explicit authority ports, neighboring paths,
changed or duplicate query fields, non-default scopes, malformed DCR/state/PKCE
values, and external or privileged callbacks. Prove the bridge adds no AWS
baseline effect and retains `aws sso login --use-device-code` as provider-owned
recovery. Routine callback success uses one `aws sso login` invocation, no
manual URL/callback transfer, and zero external processing.

Automated Gateway fixtures use synthetic bearer canaries and prove the canary
is absent from OPA input and denial evidence, preserved exactly for the single
allowed upstream attempt, and never produces `broker_auth_required`. Live
manual evidence records only pinned client versions, pass/fail, and the
secret-free account-status classification.

## Research Broker synthetic journey

The research integration evidence uses a fake GitHub CLI, synthetic
static provider manifests and secrets, local Broker/Gateway/OPA/upstream
fixtures, and secret canaries. It proves:

- locked startup and exact root-key/vault ownership/integrity;
- protected stdin refusal before reading and validation before Broker send;
- omitted-provider selection is interactive, bounded to installed reviewed
  login drivers, and explicit provider selection remains deterministic; the
  research matrix accepts its two methods;
- fixed purpose-limited GitHub/AWS/pup/Codex/Claude argv, canonical executable
  digest checks, selected-Workspace Manifest image binding for pup and Claude, private
  homes/PTY where declared, bounded browser targets, and checked cleanup;
- per-project handles bound to Workspace Manifest/provider/revision/target/header;
- direct bearer/raw credentials and direct AWS signatures at declared bindings
  fail as `broker_auth_required` with zero fallback, Broker, OPA, DNS, or
  upstream calls, while one undeclared binding retains compatibility passthrough;
- non-secret introspection before OPA; zero static resolution, refresh,
  companion call, or signing on deny; one same-revision reviewed action and
  one upstream attempt on allow;
- bounded AWS SigV4 and private companion behavior, fixed
  Datadog/OpenAI/Anthropic refresh transports, OpenAI supplemental-header
  ownership, Anthropic four renewable fields plus two structurally bounded
  non-secret entitlement labels with dynamic bounded scope
  validation, granted-subset enforcement, normalization, and secret-free
  diagnostic stages into a strict Tobari-owned session, fixed-client
  resolution/refresh preserving the stored scope set and labels, exact
  Workspace handle/public-refresh-sentinel projection, and durable
  unknown-outcome barriers;
- rotation, logout, revocation, Workspace re-entry, and no invalid-handle
  passthrough fallback;
- secret-free logs/output and canonical/embedded source equality;
- absence of managed profiles/state, manifest-selected executable helpers,
  arbitrary provider routes, compatibility readers, and provider CLIs inside
  the Broker image.

Owner manifests are strict static data and cannot select a helper or policy.
Owner manifests cannot select a reviewed dynamic plan. This section describes
the dev-only research surface and is not a release-surface readiness outcome.

## Optional research reviewed-provider acquisition

When reviewing the research surface, maintainers may use disposable
provider accounts and an interactive trusted-host terminal. This observation
is not a standard release-readiness requirement because the `auth` namespace
and Broker runtime are absent from standard archives. The optional GitHub slice
is:

```sh
bin/tobari-research auth login --provider github --manifest default
bin/tobari-research auth status --manifest default --format json
# Re-enter the default Workspace Manifest's Workspace.
case "${GH_TOKEN-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$(gh auth token --hostname github.com)" = "$GH_TOKEN"
gh api user --jq .login >/dev/null
bin/tobari-research auth logout github --manifest default --format json
```

The equality assertion proves `gh auth token` returns the projected handle, not
the primary credential. Prove the old handle fails after logout. Record only
pass/fail, the exact source commit/image digests, and secret-free status. Never
record the token, device code, handle, account identifier, vault, authenticated
response, or raw transcript.

Using `task build:dev` (`bin/tobari-research`), replay the AWS Identity Center
and console methods. With the release-surface binary, prove the `auth` namespace is
absent. Then replay selected-Workspace Manifest-runtime
Datadog pup flow and localhost stdin relay, the
contract-checked host Codex native browser/loopback flow, the separately pinned Workspace Codex
handle projection, isolated Workspace Manifest-runtime Claude Code 2.1.220 native login
and handle-only credential-file projection, and Chatwork stdin
import separately. Record only command/observed-version, pass/fail, and secret-free
state/revision metadata; never store provider responses or credential state.
When the host Codex version has advanced, also verify its official source still
matches the compiled refresh client identity and replay one near-expiry refresh
without recording tokens, account identifiers, or raw transcripts.

## Enumerated predecessor migration

Using the synthetic predecessor fixture, run `tobari doctor --format json` and
verify the failed Workspace Manifest row names exact recovery `migrate apply`; dependent
rows must be blocked by Workspace Manifest. Then run:

```sh
tobari help migrate apply --format agent
tobari migrate apply --format json
tobari doctor --format json
tobari migrate apply --format json
```

The first mutation must report the complete Workspace Manifest collection, a
secret-free recovery ID, retained Workspace Manifest IDs and default selection, `standard` for the
standard predecessor, and an exact `legacy-NAME@ORDINAL` binding for a custom
Dockerfile predecessor, plus only the bounded
`research_auth_disposition: reauthentication_required` when predecessor
research state exists. The second invocation must report `changed: false` and
no recovery ID. Compare standard Workspace-home/native-auth, learned-rule,
Workspace state, and Runtime canaries byte-for-byte. Verify research filesystem
authority is quarantined and old-reader handle resolution fails without reading
macOS Keychain; on Linux verify root-key bytes move and restore with the set.
Exercise every transaction phase to prove full predecessor resolution before
the central state move and zero resolution afterwards, plus resume and exact
rollback/fresh-state refusal. Replay unknown/mixed/partial/corrupt,
duplicate-key, unsafe-mode, symlink, digest-drift, and Runtime-conflict
fixtures and require zero final-state publication. Do not record a real
Workspace home, credential, Keychain fact, quarantine path, or private Runtime
source as evidence.

## Publication checkpoint

The local release-ready handoff requires:

```sh
task check
task security
task public:check
task release:check
```

Also inspect generated diffs, dependency/license diffs, canonical Gateway
source equality, release archive checksums, archive-level SPDX SBOM,
unsigned in-toto/SLSA metadata, Formula rendering, and the clean-environment
Linux/Colima Quick Start.

Stop for explicit approval before pushing a branch or tag, creating a GitHub
Release, or updating a Homebrew tap. Tobari has no OCI publication step. After
approval, the protected workflow builds the exact SemVer archives and creates
the immutable GitHub Release. The released CLI builds the pinned Gateway and
agent-ready base locally and never obtains either from GHCR. A
prerelease such as `v0.1.0-dev.1` is marked as a GitHub prerelease and must not
mutate Homebrew. A stable run must then create the
Formula-only `tasuku43/homebrew-tap` pull request from the exact audited Formula
asset; dry runs and prereleases must not cross that boundary.
