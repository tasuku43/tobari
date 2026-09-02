# ADR 0085: Make `status` the CWD home

- Status: Accepted
- Date: 2026-08-25
- Deciders: Tobari product owner and maintainers
- Scope: Product, domain, application, infrastructure, CLI, security, harness,
  and agent-readiness output
- Revises: ADR 0027 and ADR 0084 at the CWD status projection
- Related: ADR 0074, ADR 0080, ADR 0081, ADR 0082, ADR 0083, and ADR 0084
- Revised by: ADR 0092 at the Workspace-only ProjectRoot selection seam; ADR
  0093 at current-Context independence and existing-Workspace precedence
- Superseded by: None

## Context

ADR 0084 established Workspace Template, Context, Policy Memory, and Workspace
as separate authorities, but bare `status` still projected only the former
default-pair fields. A user could not tell from one routine read whether the
default Template's Context and Workspace were current, what Runtime material
was locally usable, what shared prerequisite was missing, or which one action
should come next. Calling sibling CLI handlers would duplicate selection and
permit incoherent joins; using entry or lifecycle adapters would also risk
state creation, cleanup, or reconciliation during a read.

## Decision

`status [--format text|json]` is the CWD-first status home. It remains
`RoleDiscover`, `EffectRead`, complete scalar delivery, and JSON schema 3. It
produces only the selected Workspace reference when that Workspace exists; it
consumes no reference and adds no selector. Nondefault actions continue through
the existing reference-producing discovery commands.

Selection is root-first. Tobari canonicalizes CWD, chooses the nearest existing
Workspace ProjectRoot that contains it, or treats canonical CWD as the
prospective root when none exists. That decision is independent of both the
Template default and current Context selector. A unique Workspace supplies its
permanent Context and Template binding; the Template default may disambiguate
multiple same-root Workspaces but never makes CWD select a Context directly.

One application-owned port returns one `StatusHomeSnapshot`. Presentation does
not call sibling handlers or join their JSON. The snapshot keeps these facts
independent:

- current desired Template revision and its exact Runtime binding;
- active Template-policy slice;
- current and active Context Policy Memory revisions;
- Workspace presence and last-successful AppliedEntry;
- selected Workspace runtime and attachment observation;
- Runtime revision authority, execution-material availability, and native
  compatibility evidence;
- shared-cluster runtime and receipt observation;
- pending permission and bounded Service-owner summaries;
- one structured primary `Next` plus separately ordered `Attention` items.

There is no persisted or serialized `overall_status`. Current AppliedEntry may
coexist with stopped or missing runtime material; pending entry may coexist with
running or available material. Presentation derives neither authority nor
state from names, generations, order, headings, color, Docker identifiers, or
timestamps.

`Next` contains an exact Catalog path and typed inputs, or one closed
non-command guidance value. Attention uses the same path/input shape. Status
stores no argv string, executes no action, and returns no Runtime, permission,
Service-request, or Service-exposure action reference. Service status contains
only typed counts and observation state—never a URL, port, request/exposure
reference, or owner protocol detail. Native login validity remains
`not_observed`; release status exposes no research auth, serve, Broker, or
provider path.

## Observation and mutation boundary

Status is strictly zero mutation. It never initializes a directory, creates a
lock, repairs or migrates state, cleans a journal or owner record, reconciles a
Workspace or cluster, builds/restores/prunes/retires a Runtime, or performs a
permission, Service, authentication, or browser action. Existing owner state is
read through dedicated non-creating observation seams. Invalid identity,
foreign ownership, unsafe paths, contradictory authority, ambiguous scope, or
continued anchor churn fails the complete command; expected absence and
bounded live unavailability use explicit typed facts.

The frozen Docker budget is:

| Case | Docker calls |
|---|---:|
| selected ready Workspace, normal observation | at most 6 |
| one complete bounded retry after changed evidence | at most 12 total |
| fresh or initialized authority with no default Template | 0 |

One attempt budgets four shared-cluster observations, one exact Runtime image
observation, and one exact selected-Workspace container observation. Attachment,
permission, and Service summaries add zero Docker calls. Cost is independent
of installation size and same-root sibling count. The adapter revalidates the
selected authority and root after live reads and permits only one complete
retry. On the supported research surface, OPA/Auth Broker container evidence
and selected image identities use bounded batch reads joined by exact component
labels and selected references, never output order; partial or ambiguous batch
evidence degrades typed live state without an unbudgeted fallback call.

## Human and machine presentation

Routine text leads with the canonical Project, Template, `Current`, independent
policy/Workspace/Runtime/cluster facts, review counts, sibling count, and one
`Next`. It is ANSI-independent and escapes untrusted external text through the
shared projection boundary.

JSON returns the same complete task-owned snapshot under the `status` envelope
at schema 3. It preserves null, explicit empty arrays, zero counts, false, and
`not_observed` distinctions. It contains no Workspace home, raw Docker image or
container identity, image tag, private snapshot path, inferred `last_used`,
copy lineage, predecessor Manifest vocabulary, or lossy aggregate state.

## Consequences

- One invocation answers the routine current-project question without external
  reconstruction or automatic recovery.
- Detailed Runtime, cluster, permission, Service, Context, Template, and
  Workspace investigation remains owned by their existing commands.
- A fresh read can recommend entry without inventing persisted authority.
- Adding another live section requires a typed bounded owner summary and a
  reviewed revision of the fixed call budget and trust boundary.
- The temporary `docs/work/status-home/` packet is removed after this durable
  promotion and implementation handoff.

## Required evidence

- Domain and application fixtures preserve the independent axes and one port
  call.
- Infrastructure fixtures prove 0/6/12 Docker accounting, nearest-root
  selection, one bounded retry, zero-write fresh reads, and non-creating
  attachment/Service observation.
- Recursive Catalog/output/reference tests bind schema 3 and the selected
  Workspace reference path.
- Agent-readiness fixtures prove zero external reconstruction and exclude
  predecessor vocabulary, private runtime data, inferred usage, research
  capabilities, and Service action material.
- `task check`, `task security`, `task public:check`, and
  `task release:check` decide implementation completion.
