# ADR 0030: Narrow first public V1 to exact policy and static brokered authentication

- Status: Partially superseded
- Date: 2026-08-12
- Deciders: Tobari maintainers
- Scope: Product, architecture, security, harness, compatibility, and public boundary
- Supersedes: ADR 0020, `0021-add-datadog-pup-oauth.md`, ADR 0023, and ADR 0025
- Superseded by: ADR 0031 for provider removal and static-only authentication; exact-policy and managed-profile decisions remain active
- Revises: ADR 0009, ADR 0019, ADR 0024, ADR 0027, and ADR 0029

## Context

There is no public Tobari release and the canonical component selection remains
`unpublished`. The source nevertheless carries several independent credential
acquisition, refresh, signing, exact-client-version, companion, and managed
injection paths plus learned path-prefix compaction. Each adds trusted code,
state, dependencies, external drift, release checks, and failure modes.

The first public release must establish one defensible end-to-end property
before expanding provider convenience: bounded Workspace execution, exact HTTP
effect authorization, and optional post-policy static credential replacement.
There is no public denial-volume evidence that justifies widening learned
authority through compaction and no compatibility obligation for unpublished
development state.

## Decision drivers

- Minimize trusted code and external compatibility obligations for V1.
- Preserve the strongest common static broker properties end to end.
- Keep exact review explicit and prohibit observation-derived widening.
- Eliminate alternate credential stores, dormant fallbacks, and hidden readers.
- Use the pre-public exact-V1 reset rather than carrying migration code.

## Decision

First public V1 retains:

- tool-native authentication owned by one Workspace home;
- one locked shared Auth Broker, installation root key, encrypted Context
  vaults, project-bound opaque handles, rotation, revocation, and logout;
- strict owner schema-V1 static provider manifests and protected non-terminal
  stdin import;
- one reviewed built-in GitHub.com static plan and GitHub CLI acquisition helper;
- Gateway recognition, non-secret introspection, OPA authorization before
  resolution, exact same-revision replacement, and no fallback for any
  Tobari-looking invalid handle;
- exact policy candidates, explicit batch review, exact allow/deny/reset, and
  Advanced Rego beneath the immutable preset guardrail.

First public V1 removes completely:

- `policy compactions`, `policy compact`, the `policy-compaction` reference,
  path-prefix learned rules, compaction state, matching, recovery, and fallback;
- the static `managed` Gateway adapter and its profiles, secret files,
  selectors, mounts, policy input, status, and storage;
- AWS, Datadog, OpenAI, Anthropic, and Chatwork built-ins;
- AWS/Datadog/OpenAI dynamic record kinds, refresh, task barriers, signing,
  supplemental headers, and provider-native resolution;
- AWS/Codex/Claude/pup host drivers, exact-version contracts, PTY/shim flows,
  and provider-specific browser/endpoint behavior;
- the resident credential companion, bridge/socket/session protocol, private
  execution mode, lifecycle/status/doctor fields, and image content;
- compatibility readers, dormant configuration selectors, aliases, hidden
  routes, state conversion, and unowned dependencies for every retired path.

`auth login` accepts explicit `--provider github`; omission on an interactive
trusted-host terminal opens a bounded selector over the installed provider
snapshot filtered to compiled reviewed login drivers. In first public V1 that
filter still yields only GitHub. JSON error mode, redirected/non-terminal
omission, cancellation, an empty eligible set, or a selection outside the
snapshot fails before login mutation. `auth import PROVIDER`, `auth status`, and `auth logout PROVIDER`
remain. An owner manifest is strict non-secret,
non-executable local data describing one static primary-secret import, bounded
handle projection, and exact HTTPS/header replacement. It cannot select shell,
helper, refresh, signing, arbitrary methods/routes, policy, or provider business
operations.

The GitHub helper retains fixed argv, a canonical executable outside the
project, digest checks before and after the complete flow, a sanitized private
temporary home, bounded strict token capture, fixed browser target/manual
fallback, and checked cleanup. Exact GitHub CLI product-version equality is not
a security boundary; the fixed observed command contract is.

Brokered output and help say `brokered`. Tool-native authentication is
explicitly `Workspace-owned`; Tobari does not claim those credentials remain
outside the Workspace. Neither brokered acquisition nor import grants network
authority or creates a policy rule.

Policy learning is exact only. The ordinary identity is Context, project,
scheme, host, port, method, and raw path plus optional GraphQL operation/root.
There is no prefix learned-rule variant and no observation count can create
broader authority. Policy presets integrate only after this deletion.

## Consequences

### Positive

- One compact broker property and one acquisition helper receive concentrated
  security and compatibility evidence.
- V1 has no provider refresh/signing execution after allow and no resident host
  credential process.
- Guided learning never widens authority beyond an explicitly reviewed exact
  effect.

### Negative

- AWS, Datadog, OpenAI, Anthropic, Chatwork, and managed profiles use
  Workspace-owned tool login or an owner static manifest where expressible.
- Existing development vaults, Workspaces, Contexts, and cluster state must be
  removed and recreated.
- Reintroducing any removed provider or compaction requires a new reviewed
  capability and release decision.

## Mechanical enforcement

- Catalog and CLI tests reject every removed command, provider, method, output
  field, fault, recovery, and reference edge while proving omitted-provider
  selection can expose only installed compiled reviewed login drivers.
- Domain and state parsers reject prefix policy and non-static credential
  record kinds without migration or fallback.
- Dependency and image-content tests prove no managed, companion, refresh,
  signing, or retired host-driver code remains.
- Static broker tests re-prove project/Context/provider/revision/target/header
  binding, locked startup, deny-before-resolution, exact one-time replacement,
  rotation, revocation, no invalid-handle fallback, and secret-free output.
- Canonical Gateway/Auth Broker source and embedded snapshots become byte-equal
  only after the deletion and integrated policy work stabilizes.

## Compatibility and migration

ADR 0027 applies. No public version supported the retired surface. Operators
use the old development snapshot to log out where useful, delete affected
Workspaces/Contexts and old cluster state, then recreate under V1. New readers
reject old state; they do not decode it for migration or log its content.

## Security and public-boundary impact

The change removes trusted processes, external endpoints, credential shapes,
and dependencies. The retained primary secret still exists in the trusted
encrypted vault and can be exercised by Gateway only after an exact OPA allow.
No real provider response, credential, account identifier, or private URL is a
fixture.

## Validation

```sh
task check
task security
task public:check
```

Release validation additionally inspects final component images and replays one
manual GitHub acquisition observation without recording a live credential.

## Reconsideration signals

Reconsider only after public usage establishes a concrete provider outcome,
denial/review evidence, and maintenance budget. Any new dynamic credential,
signer, helper, companion, managed store, or authority-widening policy requires
an explicit thesis, trust-boundary, catalog, state, dependency, and release
decision before implementation.
