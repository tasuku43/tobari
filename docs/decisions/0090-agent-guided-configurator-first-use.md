# ADR 0090: Keep first use deterministic and make agent assistance task-scoped

- Status: Accepted
- Date: 2026-08-29
- Deciders: Tobari product owner and maintainers
- Scope: Product, CLI, first use, Runtime, static policy, Configurator,
  network topology, native agent authentication, desired configuration,
  security, and harness
- Revises: ADR 0084 and ADR 0088 at assisted desired-source editing; ADR 0086
  at Configurator execution topology
- Related: ADR 0046, ADR 0053, ADR 0081, ADR 0085, ADR 0089, and ADR 0091
- Revised by: ADR 0092 at task selection, execution Runtime, and Home scope
- Superseded by: None

## Context

Tobari desired configuration is deliberately ordinary typed source because a
human or coding agent may edit it before explicit host review and Apply. The
first Configurator slice therefore tested an agent-guided fresh-root journey in
which Codex or Claude Code could author Runtime, Template policy, Workspace
bootstrap, shell, and Git settings together before a Context existed.

The slice proved several useful mechanisms: a direct-egress non-Workspace
container can run from one immutable Runtime image ID; one complete managed
Home can preserve otherwise unknown native tool state; the existing strict
browser/callback bridge can support native Codex and Claude login; Docker can
retain the agent's native terminal; and a host can freeze one immutable
working-copy submission before review and Apply.

It also produced contrary product evidence. A fresh user does not yet know the
meaning or interaction of Template Boundary, static semantic policy, Context
Policy Memory, Runtime binding, Workspace bootstrap, shell, and Git defaults.
An open-ended agent interview therefore asks the user to direct and review the
same aggregate model the feature was meant to hide. A fixed prompt and generated
instructions can improve the conversation, but cannot make the exact response,
question order, abstraction, or language behavior of two independently evolving
agent clients a deterministic first-use contract.

The user does have concrete intent later. `runtime create` establishes one
specific Docker build-context source. Repeated trusted-host Permission Review
establishes exact typed evidence that may justify a reusable static Template
policy. In those states an agent can help with one bounded editing task without
inventing the task or explaining all of Tobari first.

## Decision

### Fresh first use remains Tobari-owned and deterministic

Fresh interactive `tobari` uses the existing Manual setup journey. Tobari
itself explains and reviews Project source access, effective network posture,
exact Runtime revision, optional typed Workspace bootstrap, shell and Git
defaults, then publishes the default Template/Context pair through canonical
host mutations. Fresh root does not select or start Codex or Claude Code and
does not create a Configurator draft.

Context remains the durable Project-root plus Template binding and Policy
Memory owner. It is an aggregate lifecycle identity, not a conversational
configuration target. Routine UI may explain its consequences but does not ask
an agent to “configure the Context.”

The public aggregate `configure` command is removed. It has no alias, hidden
fallback, recovery action, or retained first-use branch.

### `assist` is one common task-scoped interaction

Tobari introduces one public interaction word across eligible targets:

```text
tobari <target> assist [target binding] [--agent codex|claude]
```

`assist` means exactly: start or resume one isolated coding-agent session for
the command-owned target. It never means manual editing and has no Manual mode.
When `--agent` is omitted on a trusted interactive terminal, Tobari offers only
Codex or Claude Code. Manual editing remains the canonical source path printed
by existing discovery/create results.

The first supported tasks are:

- `runtime assist --id <runtime-ref>`: edit one managed Runtime source working
  copy, review and publish only that editable source, then offer canonical
  `runtime build` as a separate confirmed continuation;
- `policy assist --context <context-ref>`: use the explicitly referenced Context's exact Template policy and
  Policy Memory revision to edit only a static `policy.yaml` working copy,
  review semantic promotion, and delegate publication to canonical Template
  Plan/Apply. Policy Memory remains unchanged.

Runtime assistance consumes the opaque Runtime reference emitted by Runtime
discovery or creation unchanged and uses the installation standard Runtime.
Policy assistance consumes one emitted Context reference unchanged and resolves
its bound Template without ambient CWD selection.

Human success output distinguishes the two paths explicitly:

```text
Work with an isolated coding agent: tobari runtime assist --id ...
Edit manually:                     <canonical source path>
```

### Configurator remains a separate reusable execution plane

The Configurator mechanism is retained and narrowed. It is a short-lived
non-Workspace container that runs the chosen agent with ordinary external
connectivity. An assist task requires an existing Context and uses that
Context's exact current Runtime as execution material, regardless of the
authoring target. An unbuilt target Runtime never selects the container image.

The selected Context's complete managed Home is the sole mutable persistent
data bind. It preserves native Codex, Claude Code, and Runtime-tool state across
that Context's Workspace and later assistance. A shared Context/Project
attachment lease prevents a live Workspace and direct-egress Configurator from
mounting it read-write concurrently.

Configurator retains:

- lifecycle/store-fenced immutable Runtime-to-image resolution;
- disconnected create and immutable Docker container/network identities;
- dedicated ordinary-egress network and complete role inspection before attach;
- non-root, read-only-root, capability-free, resource-bounded execution;
- the selected-agent-only native browser/callback bridge;
- Docker-owned interactive terminal and fixed positional initial request;
- bounded cleanup, retained-draft classification, and crash recovery;
- host freeze followed by trusted review and canonical mutation.

It receives no Project root, host or other Context Home, Docker socket, SSH
agent, active authority, canonical desired source, writable Policy Memory,
arbitrary bind mount, host network, command selector, or generic host helper.
Runtime source, task input, or agent output cannot widen this topology.

### Each task owns a closed knowledge and writable-source contract

One task contract binds:

- exact task kind and target identity;
- exact execution Context and Runtime revision;
- exact base source and evidence revisions;
- one task working copy below the managed Home;
- a generated task instruction profile;
- a closed read-only reference/evidence bundle;
- one target-specific freeze and validator;
- one canonical host publication path.

`AGENTS.md` and `CLAUDE.md` are projections of one repository-owned canonical
task instruction source. They contain concise role, writable-target, forbidden-
effect, review, and Apply rules. Detailed schema and evaluation knowledge lives
in read-only task references rather than relying on one oversized prompt. The
agent may inspect those files, but prose is not enforcement.

Runtime assistance exposes only the Runtime working source as editable. It
does not ask about or expose Template policy, Policy Memory, Workspace
bootstrap, shell, Git, Project files, or canonical Runtime source. The host
freezes the complete bounded tree, revalidates its base, and alone publishes
the editable Runtime source. Build remains the existing exact Runtime action.

Policy assistance exposes only one `policy.yaml` working copy. It copies the
referenced Template policy, exact Template base revision, and Context
Policy Memory as read-only evidence. Its reference pack explains the terminal
destination/method Boundary, terminal exact Deny, Template static authority,
remembered reviewed Allow, unresolved review/default deny order, and the closed
generic HTTP, GraphQL, MCP, Git, OCI, AWS, and Kubernetes semantic schemas.
Method `allow` is explicitly described as admission to lower policy evaluation,
not a final network grant. The agent cannot edit Policy Memory, erase exact
Deny, broaden beyond the destination ceiling, create unknown schema fields, or
select Apply. Mechanical schema and semantic validation reject such output.

### Agent behavior is guidance; the frozen result is the contract

Tobari passes one fixed task-specific positional request asking the selected
agent to read its generated instructions and begin with the concrete target.
For Runtime it inspects the Dockerfile and asks which tools or language Runtimes
to add. For policy it groups supplied validated evidence and proposes the
smallest semantic static rules. The user may answer in any language.

The precise upstream response, question order, language following, and editing
strategy remain compatibility observations. Product correctness comes from
the command-selected task, mount contract, read-only evidence, closed schema,
target validator, immutable freeze, host semantic review, and canonical Apply.

### Review and confirmed mutations remain separate

Agent exit first stops and removes transient Configurator resources, then the
host freezes one content-addressed task submission. Review consumes that exact
value and never rereads mutable Home. The user may Apply the target source,
return to the same agent draft, or keep the draft and exit. Agent output cannot
choose an action.

Runtime source Apply and Runtime build are independent confirmed mutations.
The same inline journey may offer “Build now” after source publication, but a
build failure cannot hide or make replayable the confirmed source result.

Policy source publication delegates to canonical Template Plan/Apply with
exact source, active Template, Context, Memory, impact, and concurrent-change
fences. Context Policy Memory is neither rewritten nor automatically reset by
static promotion. Shared policy reconciliation remains a separate canonical
receipt; a later failure never rolls back Template publication.

### Recovery remains exact and target-scoped

Chooser Back or cancellation before materialization changes no task or Docker
state. After materialization, cancellation may retain one exact non-authoritative
task draft and complete task-owned Home. Its recovery is the same exact assist
command and agent, not aggregate `configure`.

A Runtime task identity includes the target editable-source digest and the
installation standard execution Runtime. A changed target/base revision,
concurrent attachment, different selected agent, or ambiguous retained
draft fails closed. The frozen submission and settlement state remain durable;
Apply confirmation is a distinct durable receipt written before canonical
mutation. After confirmation, crash replay resumes the exact submission without
rerunning the agent. Runtime authority is verified under its lifecycle/store
fence; policy Stage/Plan remains authoritative until canonical Apply settles,
including semantic no-op. A surviving exact Template Apply settlement resumes
its unchanged opaque Plan directly, without another agent run, review, or Plan.
The retained Stage prevents deletion of its exact Context until settlement, so
the reviewed Context/Home boundary cannot disappear between Plan recovery and Apply.
Only then may an exact current-authority match settle
the previous generation before a new one begins. Tobari neither silently
merges, retargets, reruns the agent, nor adopts a different Home. Clean transient
cleanup retains only confirmed working material; bounded cleanup exhaustion is
the distinct partial outcome already owned by the Configurator core.

Pre-public aggregate Configurator metadata may remain on disk but is ignored,
never adopted, migrated, inferred, or deleted by ordinary commands. Context
Homes already adopted by local development state remain valid.

## Consequences

- The safe first successful Workspace no longer depends on agent conversation
  quality or provider availability.
- Assistance begins from concrete user intent and can ask lower-abstraction
  questions without teaching the whole Tobari model.
- The direct-egress trade-off becomes narrower: exactly one task-owned Home is
  exposed, but writable configuration input is exactly one task target.
- Runtime and Policy assistance share one visible interaction and execution
  core without sharing schemas, evidence, validators, or Apply authority.
- Deterministic AWS/EKS host import remains outside agent conversation.
- Existing isolation, native-login, Home continuity, terminal, cleanup, and
  crash-safety implementation is retained rather than replaced.
- Public `configure`, bootstrap/evolve modes, draft-to-Context Home adoption,
  and aggregate agent submission are retired before first release.

## Mechanical enforcement

- Catalog and composition tests prove `configure` and fresh-root agent paths are
  absent and both exact assist commands reach their production handlers.
- Fresh first-use fixtures and cold integration select the one deterministic
  Manual journey and retain inline TUI/negative alternate-screen canaries.
- Domain/application tests bind task kind, target, Context, execution Runtime,
  base/evidence revisions, writable roots, immutable submission, intent,
  impact, cancellation, and no-op behavior.
- Runtime tests reuse immutable-image, disconnected-create, actual-image,
  dedicated-network, resource, ID-bound exec/cleanup, selected-browser subset,
  callback, and native-terminal canaries for both task kinds.
- Store tests prove one complete task-owned Home, exact target working tree,
  canonical generated instructions, read-only knowledge/evidence, target-only
  freeze, unsafe-tree rejection, retention, and crash replay.
- Runtime assist tests prove opaque create/list/show reference round trip,
  execution/target Runtime separation, exact base CAS, manual-path alternative,
  source-only Apply, separately confirmed build, and confirmed-result retention.
- Policy assist tests prove exact explicit Context/Template/Memory binding,
  closed schema/reference material, terminal-Deny and ceiling preservation,
  out-of-scope edit rejection, Memory immutability, semantic review, stale-plan
  rejection, and canonical Template Apply/reconciliation.
- Presentation fixtures prove `assist` always means isolated coding-agent
  session, selected-agent handoff is host-owned, direct Internet remains
  visible, manual editing is separate, and child response prose has no
  authority.
- Capability, reference-graph, help, recovery, helper-source, docs, security,
  `task check`, and cold first-use gates enforce the complete replacement.
