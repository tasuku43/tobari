# Agent Readiness Validation

This is the executable first-public-V1 journey contract. It supplements
automated tests; it does not permit live credentials or authenticated
transcripts as repository fixtures.

## Required outcomes

| Outcome | Public route | Success evidence |
|---|---|---|
| Discover capabilities | `help --format agent`, then one namespace or exact-command selector | Root remains a compact capability index; one scoped read supplies complete typed inputs, outputs, failures, and workflow |
| Choose a Context envelope | `context list`, `context show`, `context create`, `context use` | Source access and policy-preset origin/revision are explicit before entry; changing current Context does not retarget existing Workspaces |
| Enter bounded work | `cluster up`, then `tobari [--context NAME]` | One selected live source bind, writable home/tmpfs, guarded network, reusable Workspace, and no direct egress |
| Grow exact permission | `policy review`, or `policy candidates` then one exact allow/deny | Terminal guardrail precedes every candidate; explicit review activates only exact Context/project/scheme/host/port/method/path authority |
| Inspect/reset decisions | `policy rules`, then `policy reset --id` | One current exact decision is removed through its unchanged opaque reference and returns to default deny |
| Use unsupported Workspace-owned auth | Tool login or environment/file injection inside the Workspace for an undeclared binding | Credential state is readable by processes in that Workspace, receives no network grant, and is explicitly a compatibility path |
| Use Broker-required auth | Standard: `auth login [--provider github|datadog|openai|anthropic]` or `auth import PROVIDER`; experimental: AWS plus `--method identity-center|console`; then `auth status`/`auth logout` | A declared binding accepts only a project-bound handle; a real Workspace credential fails before OPA; OPA deny performs no secret-bearing plan action; allow applies one same-revision reviewed result |

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
  --source-access read-write --policy-preset builtin/agent-ready --format json
"$TOBARI_BIN" context create --name restricted \
  --source-access read-only --policy-preset builtin/offline --format json
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
network, policy, or broker handle state.

## Policy-preset matrix

For every preset, inspect and bind its immutable immediate-grant count.

- `builtin/agent-ready`: the exact pinned Claude/Codex core matrix succeeds
  without a permission candidate; exact Deny overrides one core grant; plugin,
  MCP, connector, file-transfer, download, evaluation, self-update, unrelated
  paths, and third-party destinations remain denied or reviewable. Prove the
  grant applies by Context HTTP identity rather than executable name.
- `builtin/offline`: every HTTP and HTTPS effect is a terminal denial; the
  review queue remains empty.
- `builtin/reviewed-exact`: only guardrail-eligible effects reach exact review.
- `builtin/get-only-reviewed`: only guardrail-eligible GET effects reach exact
  review; HEAD and every non-GET are terminal denials. Do not describe GET as
  safe or read-only.

For each terminal denial, record zero permission candidates, external DNS
lookups, Broker resolution calls, and upstream attempts. Repeat with a learned
exact allow, baseline grant, and Advanced Rego allow that would otherwise match;
none may bypass the guardrail.

Custom-preset tests use strict owner-only schema-V1 data. Reject unknown fields,
wildcards, IP/private destinations, secrets, shell, Rego, include, inheritance,
remote fetch, refresh, signing, symlinks, unsafe modes, duplicate keys, and
ambiguous rules. Context creation normalizes, validates, digests, and snapshots
the source. Editing the source preset afterward must not change the existing
Context report or active guardrail.

The canonical contributor base must run `claude --version` as 2.1.220 and
`codex --version` as 0.147.0 after replacing `/var/lib/tobari` with a fresh
Workspace home. The client pins and agent-ready core matrix are reviewed as one
contract. Verify the base workflow is validation-only and the protected
Release workflow invokes the publishability check before registry login. While
either artifact lock records pending redistribution review, that check must
fail closed.

## Reviewed policy journey

Generate one learnable denial from a running Workspace. Verify the child sees
only bounded secret-free host-review navigation and that no candidate ID or
unchecked argv is embedded in the response. In a trusted-host terminal:

1. Open `policy review`; one distinct path remains exact, while a second
   compatible distinct HTTP path produces one typed `/path/{id}` proposal.
2. Inspect the Context/project/effect detail. Prove the proposal states that
   future single-segment values are included and offers Allow template, Allow
   observed exact, and Deny pending exact. Staging grants nothing.
3. Refresh and prove decisions remain bound by typed review-item ID, never by label,
   order, or indentation.
4. Confirm one final ordered Apply and observe the authoritative active
   revision.
5. Retry in the same running Workspace.
6. Inspect `policy rules`, reset the exact or template rule, and prove the request returns
   to default deny and becomes reviewable again.

Machine replay uses `policy candidates`, `policy allow --id`, `policy deny
--id`, and `policy reset --id`. The ordinary identity is exact Context,
project, scheme, host, port, method, and raw path; GraphQL adds operation type
and root field. Query, headers, body, and repeated identical observation count do
not widen authority. Prefix rules, compaction commands/references/state, and
dormant prefix fallbacks must all be absent.

## Broker synthetic journey

The required `task integration:test` evidence uses a fake GitHub CLI, synthetic
static provider manifests and secrets, local Broker/Gateway/OPA/upstream
fixtures, and secret canaries. It proves:

- locked startup and exact root-key/vault ownership/integrity;
- protected stdin refusal before reading and validation before Broker send;
- omitted-provider selection is interactive, bounded to installed reviewed
  login drivers, and explicit provider selection remains deterministic; the
  standard matrix rejects AWS before acquisition while the experimental matrix
  accepts its two methods;
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
Owner manifests cannot select a reviewed dynamic plan. Tools without a
supported binding remain Workspace-owned compatibility authentication; adding
an exact static owner binding makes that request shape Broker-required.

## Manual reviewed-provider acquisition

Immediately before publication, reviewers use disposable provider accounts and
an interactive trusted-host terminal. The GitHub slice is:

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
and console methods. With the standard binary, prove the same provider and
method argv are rejected before acquisition. Then replay selected-Context-runtime
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

Also inspect generated diffs, dependency/license diffs, canonical Gateway/Auth
Broker source equality, release archive checksums, archive-level SPDX SBOM,
unsigned in-toto/SLSA metadata, Formula rendering, and the clean-environment
Linux/Colima Quick Start.

Stop for explicit approval before pushing a branch or tag, publishing an OCI
image, creating a GitHub Release, or updating a Homebrew tap. After approval,
the protected workflow publishes and inspects the runtime, Gateway, and Auth
Broker indexes, generates their source-bound lock, injects it into the exact
SemVer archives, and creates the immutable GitHub Release. A prerelease such as
`v0.1.0-dev.1` is marked as a GitHub prerelease and must not mutate Homebrew. A stable run must then create the
Formula-only `tasuku43/homebrew-tap` pull request from the exact audited Formula
asset; dry runs and prereleases must not cross that boundary.
