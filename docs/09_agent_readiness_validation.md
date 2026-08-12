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
| Use Workspace-owned auth | Tool login inside the Workspace | Credential state remains inside that Workspace home and receives no network grant |
| Use static broker auth | `auth login --provider github` or `auth import PROVIDER`; `auth status`; `auth logout` | Workspace receives only a project-bound handle; OPA deny resolves no secret; allow replaces one exact header once |

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

TOBARI_BIN=bin/tobari-dev
"$TOBARI_BIN" help --format agent
"$TOBARI_BIN" help context --format agent
"$TOBARI_BIN" context create --name writable \
  --source-access read-write --policy-preset builtin/reviewed-exact --format json
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

For every preset, first prove no immediate grant exists.

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

## Exact policy journey

Generate one learnable denial from a running Workspace. Verify the child sees
only bounded secret-free host-review navigation and that no candidate ID or
unchecked argv is embedded in the response. In a trusted-host terminal:

1. Open `policy review` and inspect the exact Context/project/effect detail.
2. Stage Allow-exact or Deny-exact; staging grants nothing.
3. Refresh and prove decisions remain bound by candidate ID, never by label,
   order, or indentation.
4. Confirm one final ordered Apply and observe the authoritative active
   revision.
5. Retry in the same running Workspace.
6. Inspect `policy rules`, reset the exact rule, and prove the request returns
   to default deny and becomes reviewable again.

Machine replay uses `policy candidates`, `policy allow --id`, `policy deny
--id`, and `policy reset --id`. The ordinary identity is exact Context,
project, scheme, host, port, method, and raw path; GraphQL adds operation type
and root field. Query, headers, body, observation count, and path similarity do
not widen authority. Prefix rules, compaction commands/references/state, and
dormant prefix fallbacks must all be absent.

## Static broker synthetic journey

The required `task integration:test` evidence uses a fake GitHub CLI, synthetic
static provider manifests and secrets, local Broker/Gateway/OPA/upstream
fixtures, and secret canaries. It proves:

- locked startup and exact root-key/vault ownership/integrity;
- protected stdin refusal before reading and validation before Broker send;
- `auth login` requires exactly `--provider github`;
- fixed API-only GitHub CLI argv, canonical executable digest checks, private
  home, fixed device page/manual fallback, no Git setup, and checked cleanup;
- per-project handles bound to Context/provider/revision/target/header;
- non-secret introspection before OPA, zero resolution on deny, one exact
  same-revision replacement on allow, and one upstream attempt;
- rotation, logout, revocation, Workspace re-entry, and no invalid-handle
  passthrough fallback;
- secret-free logs/output and canonical/embedded source equality;
- absence of managed profiles/state, AWS, Datadog, OpenAI, Anthropic, Chatwork,
  dynamic records, refresh, signing, supplemental headers, task barriers,
  companion protocols/process modes, exact-version drivers, selectors,
  dependencies, and image contents.

Owner manifests are strict static data and cannot select a helper or policy.
Tools outside the GitHub pairing remain Workspace-owned authentication.

## Manual GitHub acquisition

Immediately before publication, a reviewer uses a disposable GitHub account
and an interactive trusted-host terminal:

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

No AWS, Datadog, OpenAI, Anthropic, or Chatwork live scenario is part of V1.

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
Linux/Colima Quick Start. The paired `unpublished` component-image marker is an
intentional blocker until reviewed multi-architecture V1 indexes exist.

Stop for explicit approval before pushing a branch or tag, publishing an OCI
image, creating a GitHub Release, or updating a Homebrew tap. After component
images are published and independently inspected, pin their immutable digests
in one reviewed commit, rerun every gate, and only then publish the exact
SemVer release artifacts.
