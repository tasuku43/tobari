# External API Contracts

This document defines the small set of cross-project contracts supplied for API-backed CLIs. It does not turn an upstream API into the public CLI and does not provide a universal HTTP client. The product remains a set of user outcomes; transport is an implementation detail behind those outcomes.

## Boundary: template decisions versus derived decisions

| Area | Fixed by this template | Decided by the derived project |
|---|---|---|
| Public surface | Commands describe user outcomes; catalog metadata is complete and machine-readable | Which outcomes, commands, aliases, and compatibility promises exist |
| Discovery and action | External or selected targets use exact opaque references without rediscovery; only a command-bound CLI-owned singleton may use a reference-free fixed target | Reference kinds, fixed-target eligibility, filters, ambiguity rules, and task-specific workflows |
| Authentication | Secret-free requirements and session metadata, a fail-closed application gate, an ephemeral binding issued only by infrastructure and passed unchanged through task ports, exact record revalidation before I/O, typed failures, and zero downstream calls on rejection | OAuth or PAT, grant, credential source/storage, scopes, expiry headroom, refresh/cache, tenant/account selection, login UX, and revocation |
| OAuth implementation | Do not implement OAuth protocol machinery in the template; keep a selected library behind infrastructure | Whether OAuth is needed and which reviewed adapter/library is justified; see [ADR 0001](decisions/0001-oauth-library-boundary.md) |
| Effects | `read`, `create`, or `write`; a create binds one opaque parent/scope input, a write binds one matching opaque existing-target input plus optional parent, generic impact is explicit, and policy is injected at one application boundary | Confirmation, approval, dry-run, OS authentication, authorization reuse, and domain-specific impact |
| Pagination and coverage | Opaque cursor envelope, explicit budgets, loop detection, cancellation, complete-or-no-result delivery, a JSON-only public-page contract with a top-level completion cursor, and an explicit collection-coverage class | Exhaustive task scope, bounded or differential window meaning, page size, ordering/snapshot semantics, limits, and user overrides |
| Calls | Finite timeout, attempt count, and upstream idempotency are explicit; unsafe mutation retry is rejected | Vendor error classification, retry/backoff budget, idempotency-key support, and endpoint-specific timeouts |
| Payload presence | Domain/application input retains omitted versus explicit empty/zero/false and declares replacement, patch, or append semantics before wire encoding | Which fields may clear, remain unchanged, append, reject empty, or map to provider `null` |
| Failures | Stable kind/code/retryability/next-action model | Product-specific codes and commands that resolve a failure |
| Schemas | Wire DTOs stay in infrastructure; drift is tested with publishable fixtures | Schema source, update cadence, unknown-field policy, compatibility window, and fixture license |
| Capabilities | Public catalog entries are finite and validated; unsupported work is recorded rather than exposed accidentally | Upstream coverage, deferred/internal capabilities, owners, rollout order, and explicit non-goals |
| Output | Declared format, fields, types, delivery, collection coverage, terminal escaping, and contract tests | Which human and machine formats are stable, the exact collection scope/window, and the bounded size/streaming policy |
| Result interpretation | Request identity/dimensions, access limitation, and optional enrichment remain typed independently of presentation | Provider-specific visibility states, enrichment sources, uncertainty, and mandatory versus optional relations |
| Release | One gate, public-boundary checks, byte-for-byte reproducible archives for identical pinned inputs, checksums, and immutable release intent | Supported platforms, signing/provenance, package managers, cadence, and long-term support |

The template side of this table fixes vocabulary, validation, and enforcement points; it does not silently choose the derived-side settings. The derived side is not a gap: it is where the product thesis and security model must become concrete before the corresponding live capability is enabled.

## Authentication and credential flow

Read [Authentication](07_authentication.md) before adding a network adapter. Application code receives only a validated, secret-free session description and passes its non-serialized ephemeral binding unchanged into the task port. Infrastructure resolves that binding and revalidates or refreshes the exact private authentication record at I/O time. Raw access tokens, refresh tokens, PATs, client secrets, token sources, authenticated transports, and authorization headers remain inside infrastructure.

Authentication is a precondition, not a transport error to discover after a write. A failed or mismatched requirement must produce zero downstream API calls. Authentication and permission are different failure kinds: reauthentication is not presented as a remedy for a valid identity that lacks authorization.

A non-nil catalog authentication requirement means the command uses the template application gate. The catalog must declare the gate's complete standard fault set with exact code, kind, retryability, and command-valid recovery actions; validation rejects omissions before dispatch. Provider-specific authentication, rate-limit, unavailable, or unsupported faults are additional derived-project declarations rather than replacements for that base set.

## Delivery, collection coverage, and pagination

`CommandOutput` declares two separate facts. `delivery` is `complete` or `paged`:

- `complete` means one invocation returns the entire result selected by the task
  or returns no successful result;
- `paged` means one invocation returns one complete public page and an explicit
  continuation cursor.

`collection_coverage` is independent:

- `not_applicable` for a scalar, single object, or no output;
- `exhaustive` for every item in the exact declared task scope at the stated
  observation point;
- `bounded_window` for a completely delivered finite/latest/provider-capped
  window that is not the whole task universe;
- `differential_window` for a completely delivered change window since an
  explicit provider or task checkpoint, not historical exhaustiveness.

Therefore `delivery: complete` does not imply `collection_coverage: exhaustive`.
The concrete limit, checkpoint, ordering, snapshot, and uncertainty remain typed
domain/application result facts rather than generic fields invented by the
renderer. Existing derived commands must classify their real scope; do not
mechanically migrate every old `complete` declaration to `exhaustive`.

`domain/page` defines a one-page envelope with an opaque cursor. `app/pagination.Drain` owns complete-or-no-result traversal and requires explicit page, item, and page-size budgets.

An exhaustive task scope delivered through internal traversal follows this contract:

1. Forward each cursor byte-for-byte; never decode, trim, reconstruct, or expose a resource URL as a replacement.
2. Stop only when the adapter returns an empty next cursor.
3. Detect repeated cursors and finite-budget exhaustion.
4. Honor the same cancellation context on every page.
5. Reject a page containing more items than the requested page size; the page size is a per-response memory bound, not only an upstream hint.
6. Validate every page and domain item before presentation.
7. Return the complete result or an error with no partial result.

`delivery: complete` exposes no public pagination binding. It may pair with
`exhaustive`, `bounded_window`, or `differential_window` according to the exact
task scope. `delivery: paged` binds exactly one optional cursor argument or flag
to exactly one top-level string cursor field through
`AgentContract.Pagination`. The cursor field is always emitted beside
`schema_version` and the collection envelope; `CommandOutput.Fields` continue
to describe only items inside the envelope. Both cursor endpoints carry the
same dedicated opaque reference kind, and no other command, input, or output
may use that cursor kind. The typed `completion: "empty_cursor"` rule makes the
empty string the only completion marker; omission, JSON `null`, and a non-string
value are contract failures, not completion. A paged collection cannot declare
`not_applicable`; its coverage describes the scope obtained when the declared
cursor traversal reaches completion.

Paged commands support only JSON and use JSON as their default. This keeps every successful presentation self-describing and prevents a text or TSV page without a completion marker from looking exhaustive. Catalog validation rejects an unknown delivery or coverage, paged plus `not_applicable`, a missing paged binding, a binding on complete delivery, any other output format, a required or non-CLI cursor input, an invalid or colliding top-level cursor field, a missing or unknown completion rule, non-opaque cursors, kind mismatch, and extra cursor candidates. Renderer fixture checks require the top-level cursor to be present and string-typed. Agent help projects the binding with the input/output contracts and derives a same-command continuation workflow, so an agent passes the emitted cursor bytes back without trimming, decoding, or guessing. A declared page is not an incomplete successful output; silently truncating that page, omitting its continuation cursor, or reaching a local limit without a cursor is a contract failure.

## Payload presence and update semantics

Before an adapter builds a provider request, the task contract classifies each
mutation payload as one of these shapes:

- **replacement:** the command supplies the complete task-owned state being
  replaced; every omitted field has one declared domain meaning and the adapter
  does not reinterpret omission as “leave unchanged”;
- **patch:** each field carries explicit presence independently from its value;
  absent means no change, while present empty, zero, or false remains an
  intentional value such as clear/disable when the product permits it;
- **append/additive:** supplied values are added to existing state; the product
  declares whether an explicitly empty collection is a no-op or invalid input,
  and omission is not silently promoted into an empty append.

Use task-owned optional or presence types rather than reconstructing intent
from Go zero values, pointer allocation, JSON `omitempty`, flag order, or
provider defaults. Provider `null`, an omitted property, an empty
array/object/string, zero, and false are distinct whenever the upstream
protocol distinguishes them. The domain/application contract decides which
distinctions matter, and the infrastructure wire DTO preserves that decision
exactly.

Adapter tests assert the complete encoded body for every meaningful presence
state and prove invalid combinations make zero provider calls. Include at least
omitted, explicit empty/zero/false, clear, and ordinary non-empty cases that the
task supports. A provider SDK convenience type is not evidence that its
`omitempty` behavior matches the product contract.

## Result access and optional enrichment

Successful delivery and collection coverage do not imply that the authenticated
identity could observe every provider object. When the provider reports hidden,
forbidden, unavailable, or redacted records inside an otherwise valid task
scope, preserve that limitation as a task-owned typed state. Do not collapse it
into an empty collection, silently weaken `collection_coverage`, or infer the
missing facts from neighboring records.

Separate a valid core wire record from optional semantic enrichment. If core
identity, encoding, bounds, or request-dimension checks fail, the whole result
fails. If an explicitly optional relationship or annotation cannot be
established, the core result may remain successful only when the entire
affected enrichment is marked unknown or unavailable with a bounded reason. Do
not publish a partially guessed relation from labels, order, indentation,
cached neighbors, or provider notation. Tests cover valid enrichment, wholly
absent or unknown enrichment, and a negative-inference canary.

## Timeout, retry, and idempotency

`domain/apicall.Policy` is declared per adapter operation:

- `Timeout` is finite.
- `MaxAttempts` includes the initial call and is at least one.
- `Idempotency` is `safe`, `keyed`, or `unsafe`; the zero value is invalid.
- A keyed operation has one opaque key per logical operation and reuses it across transport attempts.
- A mutation with more than one attempt is valid only when the upstream operation is safe or keyed.

The application mutation invoker calls its action once. Any proven-safe transport retry happens inside the adapter and does not repeat policy, confirmation, or logical intent construction. An adapter may retry only typed retryable failures, must respect `Retry-After` when applicable, and must not sleep past context cancellation or its overall budget. `retry_after` is timing evidence, not replay permission: a rate-limited mutation may expose an authoritative positive window while remaining non-retryable. A missing window means timing is unknown, not immediate permission. Only `retryable` answers whether the same logical command may be repeated.

Read-only application services recheck cancellation immediately after a port returns and suppress the result when a port ignored cancellation. Because exhaustive pagination returns no partial result, every `operation_canceled` path in its drain is retryable and matches the catalog's common read cancellation contract.

Mutation semantics are phase-sensitive. Before the action call, `execution.Invoker` guarantees zero mutation attempts, so its common `operation_canceled` fault is retryable. Once the action is called, cancellation does not prove that the provider rejected or rolled back the effect. A valid structured adapter fault is authoritative even when its private cause is `context.Canceled` or `context.DeadlineExceeded`; the invoker returns a detached `fault.PublicCopy` and preserves its kind, code, and retryability. Any other action error, including a raw cancellation, becomes non-retryable `contract/unclassified_mutation_outcome` because the invoker cannot infer whether the effect occurred. That common fault must point only to an exact read-only reconciliation command. A nil action error is a confirmed success and is not overwritten by cancellation observed after confirmation.

The adapter contract must distinguish a request that was not sent, a confirmed result, and an unknown outcome when the provider makes that distinction possible. An unknown mutation outcome is non-retryable by default and points to an exact read/discover command that reconciles the target before another write. Do not translate cancellation into permission to repeat an unsafe action.

After an action has returned confirmed success, the CLI's effect-aware output
finalizer does not reclassify that success because cancellation arrived before
the stdout write. A short or failed write still prevents exit 0, but it becomes
non-retryable `mutation_output_write_failed` and points only to read-only
reconciliation. It never tells the caller to repeat the confirmed mutation.

The template does not select a backoff formula or universal numeric ceiling because vendor limits and latency budgets differ. A derived security/product contract records the maximum accepted timeout and attempt count, formula, jitter source, caps, and tests; user configuration above those bounds must fail rather than create an effectively unbounded call.

For keyed mutation retry, create one key only after the complete logical intent and payload have been validated. Reuse that key for transport attempts of the same logical operation, never reuse it for a different target or payload, and never regenerate it merely because the transport result is uncertain. Adapter tests must prove same-operation reuse and cross-operation separation; `apicall.Policy` validates the generic declaration but cannot infer provider-specific key binding.

## Side-effect and impact boundary

Every command has an `operation.Effect`. A mutation also declares:

- a canonical `TargetRef`;
- a catalog `MutationContract` that binds its structured opaque inputs to the target roles;
- impact cardinality: one, many, or unbounded;
- whether it sends notifications;
- whether it changes access;
- whether it is destructive.

Each impact dimension uses an explicit declaration; omitted values fail closed. Product-specific effects such as message recipients, visibility transitions, file sharing, or workflow triggers belong to a derived domain type and may make the policy stricter.

The binding rules distinguish an object that does not exist yet from an existing object being changed. In the reference-bound mode, a `create` declares exactly one opaque `parent_input`; a `write` declares an opaque `target_id_input` whose kind equals `TargetKind` and may declare a distinct opaque parent. In the command-bound mode, a complete `tool_local` fixed target is the create scope or existing write target, `target_inputs` is explicitly empty, both input roles are absent, and `TargetKind` matches the fixed kind. Missing, mixed, extra, duplicate, non-reference, and mismatched declarations fail before mutation policy or adapter calls. External API resources normally remain reference-bound; fixed targets do not bypass account or resource selection.

`app/execution.Invoker` snapshots and validates command, effect, target, and impact; applies an injected policy; checks cancellation; then calls one logical mutation action. It deliberately does not decide whether policy means human approval, dry-run, OS authentication, role authorization, or another mechanism.

## Failure and recovery contract

`domain/fault.Error` provides stable recovery metadata:

- `kind`: broad recovery class;
- `code`: stable project-specific identifier;
- `retryable`: whether repeating the same logical command can be correct;
- optional `retry_after`, which is authoritative rate-window evidence when
  positive and otherwise unknown; it never overrides `retryable`;
- `next_actions`: exact commands that can resolve or investigate the failure;
- a human message that is useful but not required for machine classification.

The common kinds cover invalid input, authentication, permission, not found, ambiguity, policy rejection, rate limiting, temporary unavailability, cancellation, unsupported capability, contract failure, and internal failure. An upstream error is mapped once at the infrastructure/application boundary. `fault.PublicCopy` extracts and validates a structured fault while discarding outer wrappers and its private cause; public boundaries use that helper before testing generic cancellation, so a deadline cause cannot erase a more precise valid classification. A malformed typed fault remains a contract failure. Raw upstream bodies and credential-bearing errors are never public output.

## Wire schemas and drift

An API adapter owns wire DTOs and maps them into domain values. Do not reuse SDK or generated wire types as public output or application input.

For every remotely decoded shape, commit the smallest legally publishable fixtures that exercise:

- a minimal valid response;
- additional unknown fields;
- required-field absence or null;
- unknown enum values;
- malformed or oversized content;
- a representative error envelope.

Record fixture provenance, schema/version, checksum, license, and whether it was synthesized. A generator is pinned, deterministic, and unable to register a public command or relax an effect automatically. Schema drift fails a contract test and becomes a reviewed product/security decision when it changes capability, output, or impact.

## Capability and coverage discipline

The command catalog is the only registry of public commands. Do not create a second dispatcher from an OpenAPI document, SDK, or capability ledger.

A derived project may maintain a coverage ledger for planning. Each entry has a stable capability ID and one status:

- `public`: linked to exactly one catalog task or one documented composed workflow;
- `internal`: required by an implementation but not a user task;
- `deferred`: deliberately unsupported, with a reason or prerequisite;
- `excluded`: outside the thesis or security boundary.

Generation may update evidence about upstream operations, but it cannot promote a capability to `public`, select an effect, or invent an impact declaration. Those are reviewed product decisions.

## Adapter completion checklist

Before an external adapter is complete, prove:

1. Required authentication is declared and no secret crosses into application/domain/output.
2. The same context reaches every call; canceled reads emit no result, and mutation outcome uncertainty never enables an unsafe retry.
3. Timeout, response-size, pagination, and traversal budgets are finite.
4. Retryability and idempotency are explicit; unsafe mutation retry fails validation, and rate timing is not treated as replay permission.
5. Replacement, patch, or append semantics preserve every meaningful omitted, empty, zero, false, and provider-null state in exact request-body fixtures.
6. Core result validity, access limitations, and optional enrichment are typed separately; optional enrichment fails wholly unknown rather than partially guessed.
7. Wire fixtures cover drift and hostile data; terminal projection escapes control characters.
8. Every failure maps to a stable kind/code and useful next action.
9. Discovery returns canonical opaque references and action forwards them unchanged.
10. Missing or mismatched mutation bindings, malformed runtime targets, policy denial, auth failure, and cancellation each make zero mutation attempts.
11. Success output matches its declared schema and is emitted only when complete; confirmed mutation output survives late cancellation and a write failure cannot authorize replay.
12. `task check`, `task security`, and any adapter-specific contract test pass.
