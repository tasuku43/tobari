# Agent Readiness Validation

This is the executable first-public-V1 journey contract. It supplements
automated tests; it does not permit live credentials or authenticated
transcripts as repository fixtures.

## Required outcomes

| Outcome | Public route | Success evidence |
|---|---|---|
| Discover capabilities | `help --format agent`, then one namespace or exact-command selector | Root remains a compact capability index; one scoped read supplies complete typed inputs, outputs, failures, and workflow |
| Choose and retire a Context envelope | `context list`, `context show`, `context create`, `context use`, `context delete` | Human list cards expose filesystem and complete method-policy facts; `context show` gives one concise boundary/runtime decision and an explicit `--details` diagnostic from the same read while JSON remains identical; argument-free terminal creation shows the complete effective boundary, edits one section, discovers optional typed bootstrap candidates without selector re-entry, and creates once; changing current does not retarget Workspaces, and deletion rejects protected/current/bound Contexts |
| Enter bounded work | `tobari [--context NAME]`; explicit `cluster up` remains available | On first use, one root route completes reviewed Context creation and exact shared-cluster reconciliation before one selected live source bind, writable home/tmpfs, guarded network, reusable Workspace, and no direct egress |
| Grow exact permission | `policy review`, or `policy candidates` then one exact allow/deny | Terminal guardrail precedes every candidate; explicit review activates only exact Context/project/scheme/host/port/method/path authority |
| Inspect/reset decisions | `policy rules`, then `policy reset --id` | One current exact decision is removed through its unchanged opaque reference and returns to default deny |
| Use native Workspace auth | Run the agent CLI's native login inside the Workspace | Credential state persists in that Workspace home, receives no network grant from login, and crosses Gateway only after the ordinary exact HTTP effect is allowed |
| Exercise experimental Broker research | Build with `task build:dev`, then use its `auth` commands | No equivalent command, provider binding, projection, service, image authority, or activation switch exists in the standard binary |

Routine success must require zero undeclared external-processing steps. Reading
a declared JSON/TSV field is consumption; a custom join/parser, provider-
notation decoder, source inspection, or exploratory request is reconstruction
and fails the supported-outcome claim.

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
"$TOBARI_BIN" help context --format agent
"$TOBARI_BIN" context create --name writable \
  --source-access read-write --policy-preset builtin/agent-ready \
  --native-readiness enabled --format json
"$TOBARI_BIN" context create --name restricted \
  --source-access read-only --policy-preset builtin/offline \
  --native-readiness disabled --format json
"$TOBARI_BIN" context show --name writable --format json
"$TOBARI_BIN" context show --name restricted --format json
```

Record the invocation count, source of every input, output field consumed by the
next task, and routine-success external-processing count. Verify every emitted
opaque ID passes unchanged to its consumer.

## Source-access matrix

Create read-only and read-write Contexts for the same canonical root. The
read-only Context must:

- read source bytes successfully;
- fail create, content change, delete, rename, chmod, and Git metadata writes;
- write successfully below its Workspace home and declared tmpfs;
- expose no writable source alias in mount inspection;
- include `source_access` in the runtime desired-state hash and Docker inspect
  reconciliation;
- observe later host changes and changes made through the same-root read-write
  Context.

The last observation proves a live direct bind, not a snapshot or filesystem-
integrity boundary. Neither Context is allowed to mutate the other's home,
network, or policy state. Native credentials follow the owning Workspace home.

## Policy-preset matrix

For every preset, inspect and bind its immutable immediate-grant count.

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
  remain denied or reviewable. Prove grants apply by Context semantic identity,
  not executable name.
- `builtin/offline`: every HTTP and HTTPS effect is a terminal denial; the
  review queue remains empty.
- `builtin/reviewed-exact`: only guardrail-eligible effects reach exact review.
- `builtin/get-only-reviewed`: only guardrail-eligible GET effects reach exact
  review; HEAD and every non-GET are terminal denials. Do not describe GET as
  safe or read-only.
- `builtin/public-get-reviewed`: public HTTPS GET succeeds without a candidate;
  every other public HTTPS method remains eligible only for exact review. An
  exact Deny for the same GET remains terminal.

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
Prove new normalized Context snapshots contain no readiness rule, legacy
agent-ready snapshots retain their exact bytes, aggregate generation removes
every historical bundle form and projects only the current binary set, and no
runtime bundle or executable selector exists. Prove the dedicated family
catalog has unique IDs, explicit pinned client versions, positive unique
append-only contract revisions, and exactly one current contract per
family. Prove the aggregate revision includes its
effective expansion, an older active revision is reported invalid, and root
entry returns exact `cluster up` recovery before Workspace mutation.
Create enabled and disabled readiness with every builtin preset. Enabled
readiness is independent of preset identity, but destination ceilings and
method Deny filter it, and exact Deny remains terminal. Prove every method uses
an exact override or the preset default, including an extension-method canary.
Disabled readiness supplies
no overlay. Missing legacy state preserves the former behavior (enabled only
for `builtin/agent-ready`) without rewriting the manifest. For GitHub,
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
authorization route, seven reviewed query fields, bounded DCR/state/PKCE
shapes, `S256`, a sorted subset of the complete 110-scope pup 1.10.7 ceiling,
and exact `127.0.0.1:{8000,8080,8888,9000}/oauth/callback`. Verify it binds
before browser open and relays one opaque callback only to the selected owned
Workspace. Reject caller-added scopes, alternate sites, host case, userinfo,
explicit port, neighboring path, duplicate/extra query, fragment, callback
host/path/port changes, replay, and oversized requests. Product APIs,
telemetry, revoke, and neighboring OAuth effects receive no baseline grant.

For each terminal denial, record zero permission candidates, external DNS
lookups, and upstream attempts. Repeat with a learned
exact allow, baseline grant, and Advanced Rego allow that would otherwise match;
none may bypass the guardrail.

Custom-preset tests use strict owner-only schema-V1 data. Reject unknown fields,
wildcards, IP/private destinations, secrets, shell, Rego, include, inheritance,
remote fetch, refresh, signing, symlinks, unsafe modes, duplicate keys, and
ambiguous rules. Context creation normalizes, validates, digests, and snapshots
the source. Editing the source preset afterward must not change the existing
Context report or active guardrail. Updating the trusted binary must update only
the native-readiness overlay of existing enabled Contexts without rewriting
their snapshot.

For typed Workspace bootstrap, use only synthetic host homes. Prove the AWS
adapter reads one fixed shared-config file only after explicit bootstrap
editing, parses it once, resolves profiles through shared referenced SSO
sessions, exposes typed available/unavailable candidates, and rejects unknown
keys, helpers, duplicates,
symlinks, unsafe modes, oversized input, credentials, and cache material. A new
Workspace must receive exact owner-only canonical `.aws/config` bytes and an
applied revision before publication. After a semantic Context refresh, prove
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
whose source changes before Apply must make zero Context and Workspace writes.

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
unchecked argv is embedded in the response. In a trusted-host terminal:

1. Open `policy review --watch`; prove an empty raw-terminal Inbox receives a
   new bounded candidate without restart and emits at most one fixed trusted
   terminal cue. Prove `--notify=off`, explicit OSC 9/BEL, conservative auto,
   identified cmux auto-selection, and hostile evidence isolation. Prove two
   unchanged timer refreshes keep one alternate-screen frame and emit no
   repaint, while a changed typed snapshot redraws. One distinct path remains exact,
   while a second
   compatible distinct HTTP path produces one typed `/path/{id}` proposal.
2. Stage exact Allow and Deny directly from the list, clear or overwrite one,
   and prove no mutation occurs. Inspect the Context/project/effect detail. Prove the proposal states that
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
--id`, and `policy reset --id`. The ordinary identity is exact Context,
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
session exit. The exact allow is shared by every process in the Context; the
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
synthetic responses. Prove the strict authorization URL opens once, one opaque
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

## Experimental Broker synthetic journey

The experimental integration evidence uses a fake GitHub CLI, synthetic
static provider manifests and secrets, local Broker/Gateway/OPA/upstream
fixtures, and secret canaries. It proves:

- locked startup and exact root-key/vault ownership/integrity;
- protected stdin refusal before reading and validation before Broker send;
- omitted-provider selection is interactive, bounded to installed reviewed
  login drivers, and explicit provider selection remains deterministic; the
  experimental matrix accepts its two methods;
- fixed purpose-limited GitHub/AWS/pup/Codex/Claude argv, canonical executable
  digest checks, selected-Context image binding for pup and Claude, private
  homes/PTY where declared, bounded browser targets, and checked cleanup;
- per-project handles bound to Context/provider/revision/target/header;
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
the dev-only research profile and is not a standard release-readiness outcome.

## Optional experimental reviewed-provider acquisition

When reviewing the experimental profile, maintainers may use disposable
provider accounts and an interactive trusted-host terminal. This observation
is not a standard release-readiness requirement because the `auth` namespace
and Broker runtime are absent from standard archives. The optional GitHub slice
is:

```sh
tobari auth login --provider github --context default
tobari auth status --context default --format json
# Re-enter the default Context's Workspace.
case "${GH_TOKEN-}" in tobari-h1_*) ;; *) exit 1 ;; esac
test "$(gh auth token --hostname github.com)" = "$GH_TOKEN"
gh api user --jq .login >/dev/null
tobari auth logout github --context default --format json
```

The equality assertion proves `gh auth token` returns the projected handle, not
the primary credential. Prove the old handle fails after logout. Record only
pass/fail, the exact source commit/image digests, and secret-free status. Never
record the token, device code, handle, account identifier, vault, authenticated
response, or raw transcript.

Using the `task build:dev` experimental binary, replay the AWS Identity Center
and console methods. With the standard binary, prove the `auth` namespace is
absent. Then replay selected-Context-runtime
Datadog pup flow and localhost stdin relay, the
contract-checked host Codex native browser/loopback flow, the separately pinned Workspace Codex
handle projection, isolated Context-runtime Claude Code 2.1.220 native login
and handle-only credential-file projection, and Chatwork stdin
import separately. Record only command/observed-version, pass/fail, and secret-free
state/revision metadata; never store provider responses or credential state.
When the host Codex version has advanced, also verify its official source still
matches the compiled refresh client identity and replay one near-expiry refresh
without recording tokens, account identifiers, or raw transcripts.

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
