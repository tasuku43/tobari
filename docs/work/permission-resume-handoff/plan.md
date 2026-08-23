# Work Plan: Permission resume handoff

- Status: Planned
- Implementation status: Not started
- Governing packet: [goal.md](goal.md)

## Chosen approach

Add a dedicated attachment-scoped `tobari-permission` helper with one read-only
command:

```text
tobari-permission wait --id <permission-wait-id> [--format text|json]
```

For a supported generic ordinary external HTTP or HTTPS denial, Gateway emits a generated wait ID
and this exact child command in its versioned denial document. The helper sends
only the ID over a private socket owned by the current attachment process. That
trusted-host process binds the connection to the attached
WorkspaceManifestID/WorkspaceID/AttachmentID/epoch, locates the exact retained
denial scoped by those IDs plus the original normalized effect identity, and
observes canonical active policy
until it can honestly return `Allow`, `Deny`, or `Expired`.

The helper does not accept a requested decision, match, target, rule, lifetime,
reason, revision, or retry command. It has no mutation port. Host review remains
`tobari review permissions` in a separate terminal, and Apply remains the
internal fixed-target `policy apply-reviewed` action.

This is a read-only side-state observation, not Workspace reconciliation.
Learned permission, candidates, Apply receipts, and wait records remain outside
Workspace Manifest desired revisions, last-applied slices, observed Workspace
runtime, and bounded reconciliation failures. Manifest publication, Workspace
entry, `cluster up`, new-session defaults, and creation defaults never copy,
reset, approve, deny, or reconcile them implicitly.

ADR 0079 keeps `cluster up` as the only Manifest cluster-projection and
shared-runtime reconciler. Existing trusted-host `policy apply-reviewed`
remains the separate learned-permission authority mutation; it hot-activates a
reviewed policy decision without updating Manifest desired/applied revisions or
claiming Workspace/cluster runtime reconciliation.

## Alternatives considered

### Candidate-resolution wait

Reject. `policy-candidate` is a host discovery/action reference, while this
operation is a child observation. Exposing it creates the wrong authority
identity and candidate disappearance is ambiguous because discovery uses a
bounded audit window.

### Public status or poll endpoint

Reject. A public `pending` state or Workspace-visible HTTP/socket API makes
polling, enumeration, fairness, stale evidence, and compatibility part of the
public surface. The helper can own bounded internal polling while presenting
only a blocking task outcome.

### Apply callback or terminal notification

Do not use for V1. A trusted-host event fanout could later reduce polling, but
Apply must not inject child terminal text or acquire the child's PTY. The V1
read loop avoids a second mutation-time coupling and treats Apply receipts as
authoritative only through the existing boundary.

### Existing watch/notify or Operator Console

Retain as host reviewer surfaces. They solve attention and presentation, not
agent resumption. The standard helper cannot depend on the experimental
Operator Console session.

### Same-terminal overlay

Reject under ADR 0073. It would make Tobari a terminal multiplexer/emulator and
does not solve the read-only child result contract.

### Hold and replay the original request

Reject. Holding a method/body/credential stream expands retention and makes
approval trigger an effect. Tobari instead discards the denied attempt and
requires an explicit fresh request after `Allow`.

## Design

### Public contract

The helper is a separate hard-coded Program, not a root `tobari` command and
not a `tobari-expose` subcommand. Its complete public surface is:

| Field | Contract |
|---|---|
| Program/path | `tobari-permission wait` |
| Capability | Stable public capability ID `policy.permission-wait` |
| Role/effect | `RoleUtility`, `EffectRead`; one exact correlation input, no discovery, reconciliation, or mutation |
| Required input | One single `InputValueText` flag `--id <permission-wait-id>`; exact domain validation `^pwt_[0-9a-f]{32}$`, generated from 128 bits of CSPRNG output, minimum and maximum 36 ASCII bytes; no completion, producer, or `ReferenceKind` |
| Optional input | `--format text|json`; omission selects `text` |
| Delivery/coverage | Complete delivery, `not_applicable` collection coverage; the command blocks within one fixed lease and has no public cursor |
| Text success | Exactly `Allow\n`, `Deny\n`, or `Expired\n` |
| JSON success | `{"schema_version":1,"result":"allow"}`, with result limited to `allow`, `deny`, or `expired` |
| Exit | All three terminal results are successful observations and exit 0; typed operational/contract faults are nonzero and appear on stderr or the standard JSON fault envelope |
| Retry statement | `Allow` means retry-ready, never pre-authorized. The command does not execute, reconstruct, or name the original request. |

The exact retry guidance lives in the Gateway denial document and exact helper
help. Success stdout remains the single terminal enum so automation never has
to separate guidance prose from the result.

The Catalog models `permission_wait_id` with existing `InputValueText`, required
single cardinality, and exact domain validation. It is neither a
`policy-candidate` reference nor an unconstrained string. After WP08 completes,
if the generic Catalog still cannot declare a maximum text length, WP07 may add
generic `MaximumLength` and its derived parser/help/contract validation, with
minimum and maximum both 36. It must not add a permission-specific value kind,
completion source, producer, or `ReferenceKind`.

Gateway's supported denial document makes a reviewed pre-public hard cutover to
schema 2 and gains only fixed/generated fields:

```json
{
  "tobari": {
    "schema_version": 2,
    "event": "permission_review_available",
    "run_on": "host",
    "review": {
      "available": true,
      "command": "tobari review permissions",
      "automatic_retry": false,
      "retry_after_review": true
    },
    "resume": {
      "available": true,
      "run_on": "workspace",
      "command": "tobari-permission wait --id pwt_<generated>",
      "automatic_retry": false,
      "result_values": ["allow", "deny", "expired"]
    }
  }
}
```

The actual implementation must retain supported request fields and fixed host
guidance, use structured construction rather than string interpolation, and
omit `resume` for unsupported/non-learnable denials. Gateway audit, learned
rules, candidates, review items, Apply receipts, and wait records use final
`workspace_manifest_id` and `workspace_id` fields plus normalized effect
identity. No final schema emits or accepts `Context`, `context_id`,
`project_id`, or `instance_id`. Schema 1 is not silently widened or accepted in
parallel; helper JSON remains its independent schema 1.

### Authority and presentation identity

| Concept | Authority identity | Presentation identity |
|---|---|---|
| Denied request | Gateway-evaluated WorkspaceManifestID/WorkspaceID plus normalized exact ordinary HTTP/HTTPS effect identity | Existing bounded denial fields and fixed guidance; names/revisions do not establish scope |
| Wait observation | Exact wait ID bound by the attachment server to its WorkspaceManifestID/WorkspaceID/AttachmentID/epoch and immutable retained exact normalized effect | No effect fields; only `Allow`, `Deny`, or `Expired` |
| Review candidate | Existing host-only candidate/review-item identity | Permission Inbox or read-only host JSON |
| Decision | Existing host-staged typed Allow/Deny choice | Permission Inbox staging; inert until Apply |
| Active authority | Existing learned rule under a confirmed active policy revision | Apply receipt on host; never projected to helper |
| Retry | A new Gateway request with independently validated identity | Agent-owned action after `Allow`; no automatic execution |

Workspace Manifest name, generation, complete revision, entry/session/create
slice revisions, Runtime binding, and Project root never substitute for the
permission scope. A Boundary change creates another WorkspaceManifestID, so no
learned decision is implicitly copied to the new Manifest or its Workspace.

Labels, command strings, ordering, request IDs, timing, and candidate
disappearance never create authority. The wait ID correlates information only.

### Concept count

V1 adds **zero durable public resource concepts**, preserving Domain Model
V1's budget of Workspace Manifest, Runtime, and Workspace. It adds two public
workflow contract elements and three internal concepts:

1. Ephemeral public-input `permission_wait_id`, a non-authoritative text
   correlation value, not a resource or opaque reference.
2. Public `tobari-permission wait`, a three-result blocking observation command,
   not a resource namespace.
3. Internal immutable `PermissionWaitRecord` containing wait ID, exact typed
   denial identity, WorkspaceManifestID, WorkspaceID, AttachmentID/epoch,
   creation/expiry, and
   no request body, query, headers, credentials, proposal, or decision.
4. Internal `PermissionWaitObserver` application port that can observe current
   active disposition but cannot mutate policy.
5. Internal attachment-local helper protocol/server with bounded frames,
   concurrency, deadlines, and peer/endpoint binding.

Candidate, review item, staged decision, Apply receipt, learned rule, Inbox,
watch/notify, and Operator Console remain existing concepts. No new permission
lifetime, approval type, or pending-state entity is added.

### Layers

- Domain: define wait ID syntax, immutable wait record, closed terminal result,
  exact supported denial identity, expiry rules, and consistency invariants.
  Domain code has no clocks, sockets, logs, or policy store dependencies.
- Application: add the smallest read-only use case/ports to validate the
  intent, resolve the owning record, observe a consistent active disposition,
  wait with one propagated context/deadline, and return only the closed result.
- Infrastructure: generate/retain bounded records from Gateway audit, bind a
  private attachment socket and registry to the existing trusted-host
  attachment owner process, receive Gateway's secret-free record through an
  existing bounded control/audit seam, validate ownership, read current policy
  authority consistently, enforce limits, and package the helper. It adds no
  daemon, persistent store, Workspace file, or second policy authority. It
  never resolves authority from Manifest names/revisions, Runtime, Project root
  label, child PID, or Workspace-supplied data.
- CLI: add a separate Program catalog derived from `cli.Catalog`, typed argv,
  help, text/JSON/fault presentation, and composition. The root catalog does
  not gain a Workspace mutation command.
- Gateway: publish final denial schema-2 and audit fields with
  WorkspaceManifestID/WorkspaceID identity, generate the wait ID, retain
  source/snapshot byte equality, and never retain or replay the request.

### Data and control flow

```mermaid
sequenceDiagram
    participant A as Agent in Workspace
    participant G as Gateway
    participant P as Active OPA policy
    participant H as Attachment wait server
    participant R as Trusted-host Permission Inbox

    A->>G: Original exact ordinary HTTP/HTTPS request
    G->>P: Authorize typed effect
    P-->>G: Deny, learnable
    G-->>A: Schema-2 denial + fixed wait command
    Note over G,H: Secret-free immutable wait record is correlated to the owning attachment
    A->>H: wait(pwt_id) over private attachment socket
    loop Bounded internal observation
        H->>H: Read learned state + confirmed active revision
        H-->>A: No public pending output
    end
    R->>R: Review and stage Allow or Deny
    Note over R,A: Staging grants nothing; agent remains blocked
    R->>P: Canonical apply-reviewed mutation
    P-->>R: Confirmed active revision + receipts
    H->>H: Reuse canonical policy evaluation for exact original effect
    H-->>A: Allow, Deny, or Expired
    opt Result is Allow and agent deliberately retries
        A->>G: Fresh HTTP request
        G->>P: Authorize fresh typed effect
        P-->>G: Current decision
        G-->>A: Fresh request result
    end
```

### Wait algorithm and concurrency

1. Parse one bounded request frame and validate fixed schema/ID syntax.
2. Bind the socket endpoint to the owning attachment's exact
   WorkspaceManifestID, WorkspaceID, and AttachmentID/epoch; reject malformed,
   unknown, consumed, attempt-exhausted, unowned, cross-attachment, or legacy
   Context/project identities with the same non-enumerating typed
   `invalid_permission_wait` fault.
3. Resolve the exact retained denial. Allow at most a 2-second bounded startup
   race for Gateway audit arrival, then fault rather than invent expiry.
4. The existing attachment owner retains only the typed secret-free record in
   memory for at most 15 minutes and never beyond its attachment. Persist no
   wait state. Same-live-attachment child processes/sessions may use the same
   socket and issued ID; a different/new attachment, pending adoption, or
   Workspace recreation never rebinds it.
5. Allow at most 8 live waits per attachment, one active connection per ID, and
   3 total connection attempts per ID within the lease: the initial attempt
   plus 2 reconnects. Reject attachment capacity with retryable
   `permission_wait_busy`; reject an exhausted ID with the uniform
   `invalid_permission_wait` shape.
6. Read a consistent pair of learned policy state and confirmed active revision
   at 1 s, 2 s, 4 s, then at most every 5 s. Coalesce duplicate exact-identity
   observations internally so many denials do not multiply policy reads.
7. Ask the canonical policy-authority evaluator whether the exact original
   effect is authorized or has an effective explicit deny. Return `Allow` for a
   final active authorization whether its stored reviewed rule is exact or a
   conservative `path_template`; never copy matching or precedence into the
   helper. Return `Deny` only for an explicit final reviewed deny disposition or
   effective explicit deny for that exact effect.
8. Baseline default-deny, staged/applying/failed/canceled Apply, candidate
   disappearance/compaction, reset without explicit disposition, revision lag,
   and ambiguous/contradictory state remain internal nonterminal observations.
   They do not return `Deny` or a consistency result and eventually reach lease
   expiry unless an authoritative final disposition appears.
9. Caller cancellation or transport loss ends that connection with a fault and
   counts the attempt, but does not consume the record or fabricate `Expired`.
   Attachment teardown/cancellation is a fault and destroys attachment-owned
   records. Only actual lease expiry returns `Expired`.
10. Emit one bounded terminal result, consume the record, and close. Never send
    progress frames, evidence, notifications, commands, or policy details.

If active policy changes after `Allow` but before retry, the next Gateway check
may deny again. That is safe and expected: wait output is an observation, not a
lease or authorization token.

## Error and cancellation behavior

- Malformed, unknown, consumed, attempt-exhausted, unowned, or cross-attachment ID:
  `invalid_permission_wait`, non-retryable without a fresh denial.
- Helper/server unavailable or component schema mismatch:
  `permission_wait_unavailable`, retryable only through fixed read-only denial
  guidance; it never falls back to mutation or direct policy reads.
- Per-ID or attachment capacity exceeded: `permission_wait_busy`, retryable
  within the same lease with fixed backoff guidance.
- Policy revision lag, baseline default deny, or ambiguous/contradictory
  disposition remains nonterminal; diagnostics are host-only and the wait may
  still reach an authoritative result or lease expiry.
- Caller cancellation, attachment teardown, or transport loss: typed fault,
  not `Expired` and not a policy result. Cancellation/loss does not consume the
  record, though the connection counts toward its 3-attempt ceiling.
- The 15-minute lease itself ending: successful `Expired`, which consumes the
  record.
- A confirmed `Allow`/`Deny` result is emitted through the read-complete output
  boundary before later cancellation can replace it with a different meaning.

No recovery string appends unvalidated argv. Recovery is an exact catalog path
or the exact fresh denial guidance.

## Security considerations

- Authority: the child has read-only observation of one prior effect. Only the
  trusted host stages and applies policy.
- Scope: learned decisions and waits bind exact WorkspaceManifestID,
  WorkspaceID, AttachmentID/epoch, and original normalized effect. Attachment
  identity scopes the wait, not the durable learned rule. Manifest name/
  revision/generation, Runtime, Project root label, child PID, and recommended
  drafts are never permission authority.
- Information: the output leaks one ternary decision bit and nothing about rule
  shape, reviewer, scope, policy revision, other candidates, or other
  Workspaces. The ID is not sufficient outside the private owning socket.
- TTY: the child launches a normal process; no host overlay, input capture,
  OSC payload, terminal injection, or alternate-screen transition is added.
- Lifetime: records and connections end with a fixed 15-minute ceiling or the
  attachment. No daemon or persisted queue survives attachment teardown.
- DoS: fixed frames, one request/response, deadlines, 8 live waits, one active
  client and 3 total attempts per ID, coalesced reads, and 5-second maximum poll
  cadence. Evidence may lower but not raise ceilings without Product Owner
  review.
- Enumeration: no list/status endpoint, no sequential identifier, no candidate
  ID, uniform invalid-ID fault, and attachment-bound lookup.
- Stale state: active revision must agree with learned state; candidate
  disappearance is ignored; unconfirmed Apply never terminates the wait.
- Concurrency: exact identity coalescing, per-ID active-client exclusion,
  consistent snapshots, and reset/Apply race tests. A later reset can make a
  fresh retry deny, but cannot cause the waiter to execute the request.
- Secrets: do not retain query, headers, body, credentials, or transport. Keep
  generated IDs, fixed commands, faults, logs, and fixtures synthetic and
  secret-free.
- Manifest separation: Manifest mutation and activation boundaries do not read
  or write learned permission as desired/applied state. The helper observes
  active permission state only and performs no Workspace or cluster
  reconciliation.

## Implementation slices and dependency order

### Integrated ADR 0079 baseline and WP08 hold

1. Treat accepted
   [ADR 0079](../../decisions/0079-model-workspace-manifests-and-applied-workspaces.md),
   current durable contracts, and commit `07535a9` as the integrated model/copy
   authority. Do not change their production files, schema, Catalog, migration,
   desired/applied state, generated data, or durable contracts from this packet.
2. Fetch and record the actual HEAD/upstream relation, protect the worktree,
   then re-read the final WorkspaceManifestID, WorkspaceID, ProjectRoot,
   desired/applied/observed/failure, principal, policy/audit, attachment,
   Catalog, schema, migration, and test implementations.
3. Run safe read-only CLI/help inspection and focused verification as needed.
   Record confirmed facts and any divergence in this packet before selecting
   files or beginning permission-resume implementation.
4. Re-observe whether durable-contract or cross-cutting review has resolved the Docker
   evidence, cluster-stop/no-attachment preconditions, and new-child-session
   behavior. Preserve any still-open choices and dependencies; do not settle
   them here. If any decided scope, attachment, or migration boundary differs
   from this plan, stop for owner review instead of adding a second identity
   path, compatibility alias, or dual reader.

After this gate, wait for WP08 implementation completion and consume its actual
Catalog contract before permission-resume work begins. WP08 alone owns
Catalog-wide recursive output/reference traversal. ADR 0079's promoted copy
contract and WP03 are accepted negative constraints; Runtime retirement
production implementation is not otherwise a permission-authority prerequisite
unless integration ownership assigns an overlapping file or generated surface
to it first.

### Fixed copy/Runtime-retirement independence constraints

1. `manifest create --copy-from` publishes a fresh WorkspaceManifestID and
   never copies or remaps learned rules, candidates, wait records/results,
   attachment authority, Apply receipts, or policy selection. No provenance or
   lineage record exists to recover permission scope, and copy triggers no
   reconciliation.
2. Runtime source copy creates a fresh RuntimeID with empty history and has no
   permission effect. Permission-resume introduces no common derivation type.
3. Runtime build/delete/prune/restore, Runtime lifecycle journals, prune plans,
   receipts, retained snapshots, and availability changes never copy, reset,
   delete, rewrite, approve, or migrate permission state. Runtime identity is
   absent from the permission key.
4. Consume WP08's repository-wide Catalog producer/consumer traversal. Add no
   permission- or Runtime-local walker/validator. The V1 wait ID is fixed as
   required single `InputValueText`, not a reference; only generic
   `MaximumLength` may be added after inspecting completed WP08.

### Slice 0: contract decision

1. Re-observe integrated ADR 0079 implementation and completed WP08, including actual Catalog input bounds,
   identity/audit/attachment seams, and standard release composition.
2. Accept and promote a durable ADR covering the fixed non-reference wait ID,
   schema-2 denial cutover, private socket/registry ownership, exact disposition
   semantics, three-result contract, no retry, bounds/reconnect, and ordinary
   HTTP/HTTPS V1 scope.
3. Propagate any thesis consequence through product, architecture, security,
   harness, authentication/external-contract/readiness docs before mechanism.

### Slice 1: domain and conformance fixtures

1. Add typed wait ID/record/result and negative invariants.
2. Add a presentation-independent denial-to-result fixture and answer key for
   Allow, Deny, Expired, staging, failed Apply, reset race, stale revision,
   invalid ownership, empty scope, and hostile external text.
3. Pin the expected routine-success external-processing count to zero.

### Slice 2: Gateway and read application boundary

1. Add final schema-2 denial/audit fields for generic ordinary HTTP/HTTPS only, using
   WorkspaceManifestID/WorkspaceID and rejecting all legacy identity fields.
2. Add the read-only application use case and minimal ports.
3. Prove no request payload/credential retention and embedded Gateway source
   equality.

### Slice 3: attachment server and helper

1. Add bounded in-memory record retention and consistent active-policy
   observation to infrastructure.
2. Add the private attachment socket, limits, ownership binding, teardown, and
   concurrency control.
3. Add the catalog-derived `tobari-permission` helper and read-only runtime
   mount without changing `tobari-expose`.

### Slice 4: integration and contract promotion

1. Wire the attachment lifecycle and helper availability into fixed denial
   guidance; unsupported configurations fail closed to current host guidance.
2. Update README, generated catalog/architecture data, capability/schema/claim
   ledgers, and governing docs.
3. Run focused, fast, security, full, public, release, and
   agent-readiness gates; review generated and dependency diffs.

## Verification strategy

- Domain: syntax/length, immutability, exact identity, closed result enum,
  expiry precedence, invalid dual-decision tests, and exact
  WorkspaceManifestID/WorkspaceID/normalized-effect scope without Manifest
  revision/name/generation substitution.
- Application: intent/effect/target validation, no mutation or reconciliation
  port, active permission-policy revision consistency, staged/unconfirmed
  behavior, cancellation, and faults.
- Gateway: final schema-2 hard cutover, exact supported classifier, fixed command,
  secret-free fields, unsupported omission, source/snapshot equality, and no
  retry/retention canaries.
- Infrastructure: attachment-owner registry/socket, same-attachment child use,
  cross/new-attachment denial, frame limits, lease/teardown, 8-wait ceiling,
  one-active/3-attempt behavior, cancellation without consumption, uniform
  unknown/consumed/exhausted faults, coalescing, poll cadence, Apply/reset
  races, log rotation, and race-detector coverage.
- CLI/catalog: exact grammar, value kind/cardinality/default,
  required single `InputValueText`, min/max 36, no completion/reference/
  producer, RoleUtility/EffectRead, stable `policy.permission-wait` capability,
  text/JSON/fault contracts, output budgets, no list/status/mutation/recovery
  argv, helper/root isolation, and generated help.
- TTY: raw child bytes, signals, resize, alternate screen, and child exit status
  remain unchanged while the helper runs as an ordinary child command.
- Security: hostile output, Unicode/control projection, no secrets, no policy
  files/OPA/Docker/general network in helper dependencies, enumeration probes,
  resource exhaustion, and fail-closed component mismatch.
- ADR 0079 integration: no legacy Context/project schema alias; learned
  rule UUID bytes survive the explicit predecessor migration under
  WorkspaceManifestID/WorkspaceID; wait/attachment state does not migrate;
  Manifest mutation, entry, cluster up, status, session, and creation-default
  paths perform zero permission mutation or implicit reconciliation.
- Policy disposition: canonical evaluator reuse, exact original-effect binding,
  exact or path-template active Allow, explicit final Deny only, baseline
  default-deny/nonfinal/ambiguous state nonterminal, and no helper-side matching.
- Derivation integration: a copied Manifest with a fresh WorkspaceManifestID
  has no copied learned rules/candidates/waits/attachments/receipts, no lineage
  fallback, and no permission reconciliation; Runtime source copy likewise has
  zero permission effects.
- Runtime retirement integration: build/delete/prune/restore and their
  journals/plans/receipts leave permission state byte-for-byte unchanged;
  RuntimeID/revision is never added to the permission scope, and no lifecycle
  timestamp is treated as learned-permission use evidence.
- Readiness: one denial, separate host Apply, one wait, deliberate fresh retry;
  answer key distinguishes Allow from authority and records zero undeclared
  external processing.

Required gates during future implementation:

```sh
task check:fast
task security
task check
task public:check
task release:check
```

## Migration and compatibility

- Persisted wait state: none. Wait records are attachment-memory-only and
  expire; this feature adds no wait-state migration command or background
  conversion.
- Upstream predecessor state: ADR 0079's explicit migration retains
  legacy Context UUID bytes as `workspace_manifest_id`, ProjectInstance UUID
  bytes as `workspace_id`, and learned rules plus retained bounded denial/audit
  evidence under the renamed scope fields. Candidates are rederived rather than
  persisted as a second inbox. It does not migrate wait records or active
  attachment authority.
- Migration preconditions: ADR 0079 migration requires zero live attachments for
  this feature. It migrates no wait record; re-entry creates a new attachment
  and any later wait receives a freshly issued ID.
- Denial wire schema: reviewed pre-public schema 2 hard cutover with only
  `workspace_manifest_id`/`workspace_id` semantic identity where identity is
  carried. Mixed old/new Gateway, helper, runtime, or CLI identity fails closed
  and keeps fixed host review guidance; schema 1 is not accepted as a parallel
  denial shape and no Context alias exists. Helper JSON remains schema 1.
- CLI: this packet adds one separate Program and denial fields and introduces
  no additional rename or alias. ADR 0079 separately replaces the
  legacy Context namespace/flags without aliases. `policy candidates`,
  `review permissions`, watch/notify, `policy apply-reviewed`, and
  `tobari-expose` retain their permission-workflow contracts under the final
  identity fields.
- Behavior: old agents ignore the new guidance and continue manual retry. New
  agents against old runtime receive no wait availability and use current host
  guidance. No automatic compatibility shim may infer a candidate or poll
  policy directly.
- Rollback: remove helper availability and schema-2 resume fields while keeping
  trusted-host review. Do not reinterpret the schema-2 document as schema 1.
  Ephemeral records disappear with attachment teardown; ADR 0079 owns
  rollback of the principal/policy identity migration.

## Documentation promotion

Promote accepted conclusions into theses only if they change the product
hypothesis; otherwise update product, architecture, security, harness,
authentication/external API/readiness contracts, README, catalog-derived help,
capability/schema ledgers, and one ADR. Remove this temporary packet only after
implementation evidence and the canonical gate complete.

## Cross-packet integration and unresolved review

- ADR 0079 and its implementation are integrated. Land only after the explicit
  pre-implementation re-observation gate confirms the no-Context vocabulary,
  final WorkspaceManifestID/WorkspaceID types, principal/policy/audit field
  cutover, Catalog/schema ownership, and predecessor migration in then-current
  code. This packet adds no dual reader or alternate migration and does not
  preempt the durable model in shared files.
- WP08 must complete before WP07 and remains the sole owner of Catalog-wide
  recursive output/reference traversal. Re-observe its actual input contract;
  add only generic `MaximumLength` if still absent, and never duplicate its
  walker or turn `permission_wait_id` into a reference.
- ADR 0079's one-time copy semantics are promoted and integrated. Runtime
  Retirement is design-fixed but not implementation-started. Neither adds
  permission authority responsibility or blocks this independent helper, but
  concurrent work must serialize overlapping Catalog, schema-ledger, generated,
  and migration-test changes. Manifest copy and every Runtime lifecycle action
  retain the negative boundaries specified above.
- WP04 owns any separate change to profile/release composition. In the absence
  of such an accepted change, `tobari-permission` is standard release surface.
- Land after exact original-effect identity and canonical exact/path-template
  policy precedence are stable; the helper never implements either matcher.
- Coordinate schema/capability/generated changes with first-public-release-core
  and capability-profile/release work; do not merge conflicting generated
  snapshots independently.
- Preserve the immutable Workspace Manifest Boundary from ADR 0079 and
  the current attachment-owned Host Loopback branch. Treat
  context-capability-envelope only as legacy Boundary evidence.
- Do not resolve ADR 0079's retained previous-revision-body or Git
  fallback slice questions here; permission identity depends on neither.
  Reconcile its general new-child-session behavior with WP07's fixed same-live-
  attachment rule. ADR 0079 migration has zero live attachments for this feature;
  it never migrates or rebinds waits.
- Remaining pre-implementation dependencies are factual, not design choices:
  the integrated ADR 0079 attachment/control-audit seam, completed WP08 Catalog
  maximum-length support, exact component-version mismatch behavior, and WP04
  standard-surface status if WP04 changes it. The input model, schema identities,
  classifier, disposition semantics, ownership, attachment scope, bounds,
  reconnect behavior, and no-retry/no-mutation contract are fixed.
