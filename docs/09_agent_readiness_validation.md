# Agent Readiness Validation

This validation asks whether a coding agent can discover, execute, interpret, and recover from representative API-backed tasks with few CLI round trips. The scenarios are intentionally synthetic and public-safe. They model a project-collaboration CLI and a team-chat CLI without embedding a private roadmap, endpoint, account, or credential.

## What counts as a round trip

A round trip is one CLI invocation whose purpose is to learn what invocation should come next. The task invocation and authentication ceremony are counted separately, but the ceremony must also publish the human-handoff scorecard below. Parsing a declared JSON or TSV field is not an additional discovery round trip; scraping prose, guessing a URL, or probing variants is.

Track external processing separately from round trips. Extracting a declared
JSON or TSV field is direct consumption and has count zero. An undeclared
`jq`/`grep` join, custom parser, provider-notation interpretation, source
inspection, or exploratory API request is one external reconstruction step. A
supported task outcome has a routine-success external-processing budget of zero;
a deliberately raw export or low-level utility may publish a narrower contract.

The target is:

- unknown surface to one selected scoped task contract: at most two
  help-discovery invocations;
- known task path, with every required reference and other task input already
  held, to its executable contract: one scoped-help invocation;
- discover reference to read/write: no extra lookup or transformation invocation;
- classified failure to next corrective command: no prose interpretation or command guessing.
- supported outcome to semantic answer and canonical next reference: zero
  undeclared external reconstruction steps.

These are bounds on help discovery and selected-contract retrieval, not on the
whole workflow. They exclude authentication and task invocations, producer
discovery, and any later scoped-help request needed for the complete contract of
a workflow endpoint outside the initial selection. Grouped workflow endpoint
usage can form next argv without implying that the endpoint's complete contract
was selected.

For each setup/authentication candidate, record required environment variables or exports, fixed values re-entered, terminal-to-browser transfers, browser-to-terminal transfers, clipboard or OS-integration dependencies, discover-to-act trips that do not contribute to target selection, first-run commands, steady-state commands, and ceremonial inputs that add no target certainty. These values compare candidates; they are not a scalar optimization target. A handoff may be justified when it materially improves safety, explicit consent, or agent certainty.

Use this scorecard in the active work packet for every credible candidate:

| Measure | Candidate value | Safety/certainty reason |
|---|---:|---|
| Required environment variables or exports |  |  |
| Fixed values re-entered |  |  |
| Terminal-to-browser transfers |  |  |
| Browser-to-terminal transfers |  |  |
| Clipboard or OS-integration dependencies |  |  |
| Non-selecting discover-to-act trips |  |  |
| First-run commands |  |  |
| Steady-state commands |  |  |
| Ceremonial inputs with no target certainty |  |  |

## Contract-level validation method

For each derived command, verify all four stages.

| Stage | Evidence |
|---|---|
| Discover | Root `view: index` exposes path, namespace, summary, capability, outcome, effect, and role plus a machine-readable `scope_request`; selected `view: scope` declares the argv grammar, inputs, input sources, prerequisites, effect, output, authentication, errors, mutation facts, and workflow edges |
| Execute | Arguments are copied from declared fields or explicit configuration; the resolved command, exclusive reference/fixed target binding, effect, runtime target, auth requirement, and impact validate before I/O |
| Interpret | The result is bound to its declared task and every target, parent, or scope dimension that task actually carries before rendering; scoped empty collections retain scope and interpretation-relevant absent, empty, zero, false, and unresolved states stay distinct; machine output has declared fields/types/delivery/collection coverage; structural runes are visibly projected; scoped I/O metadata marks external text as untrusted data; opaque references retain exact values and their field-required kinds |
| Recover | Failure kind/code/retryability/next actions are structured; auth, permission, ambiguity, missing targets, rate limits, temporary failure, cancellation, and contract failure remain distinct |

## Semantic fixture and presentation evidence

A relationship-rich or otherwise interpretation-sensitive capability keeps one
presentation-independent typed fixture and a machine-readable answer key. The
fixture includes every request dimension the task carries, retains scope when a
scoped collection is empty, and includes interpretation-relevant
absence/empty/zero/false examples, unresolved facts, and canonical references.
Tests bind the answer key back to the typed fixture before using it to judge a
renderer.

Select negative canaries that apply to the capability, including tempting but
invalid inferences from equal display names, adjacent items, ordering, quoted
prose, raw provider notation, unknown or out-of-window parents, and indentation.
A renderer is eligible only when the semantic answer and exact next argv can be
obtained from one command with zero external reconstruction and every canonical
action reference remains complete.

For a significant default presentation change, generate before and after
goldens from the same typed fixture. Record the fixture and golden hashes, exact
byte counts, and any tokenizer name/version and token counts. These are
secondary efficiency evidence after semantic eligibility, not a substitute for
correctness. Keep failed, invalidated, and inconclusive evidence, and record a
product compatibility decision separately from any benchmark result. Live-model
evaluation is explicit and optional; it is not part of `task check`.

## Scenario A: project-collaboration CLI

### Outcome

Find a project by a human filter, obtain its canonical reference, and read its current summary.

### Expected path

1. The agent reads the compact root outcome index, chooses `commands[].path` or `commands[].namespace`, and applies the published `scope_request.invocation_template` without guessing help syntax.
2. Scoped help identifies a `discover` command, its filter input, authentication/scopes, complete or paged delivery, exhaustive/bounded/differential collection coverage, and its produced `project` reference field.
3. The agent runs discovery in a machine format and selects an exact `project_id`. Multiple candidates remain data, not a hidden choice by the later action.
4. Scoped help for the read action declares `--project-id` as consuming that reference kind.
5. The agent passes the exact emitted bytes into the read action. It does not parse a browser URL, normalize case, or call discovery again.
6. The result declares its delivery and collection coverage and names every
   stable output field.

### Recovery probes

- No credential: `authentication`, not `permission`; next action names the configured login/status command.
- Valid identity without scope: `permission`; retrying login is not claimed to fix it unless the derived flow can request additional scope.
- No matches: successful empty discovery or a documented `not_found`, never a fabricated reference.
- Multiple matches: discovery returns candidates or `ambiguous`; action is not attempted.
- Stale project ID: `not_found` with discovery as a next action.
- Page cursor loop or local bound: contract failure, no partial successful output.
- Rate limit: `rate_limited`, independent retryable metadata and bounded
  retry-after evidence; timing never authorizes a duplicate logical operation.

### Acceptance

An agent that knows only the desired outcome reaches one selected task contract
with at most two help-discovery invocations, then reuses the discovered
reference without transformation. Once the read path and all required inputs
are known, its complete contract takes one scoped-help invocation. Retrieving a
complete contract for an endpoint outside the initial selection is counted
separately. Every recovery probe selects its next action from structured
metadata.

## Scenario B: team-chat CLI

### Outcome

Find a room, inspect its metadata, then send one message to the explicitly selected room.

### Expected path

1. A scoped query identifies room discovery and declares the exact output field carrying the room reference.
2. The read action consumes the same room reference and makes no hidden name search.
3. The send action declares `create` or `write` according to the derived thesis, cardinality `one`, notification `yes`, access-change/destructive declarations, authentication/scopes, idempotency behavior, and stable result fields. Creating a new message binds the selected room reference as `parent_input` and has no `target_id_input`; changing an existing message binds the message reference as `target_id_input` and may bind the room as a distinct `parent_input`.
4. The application mutation invoker validates the runtime intent and applies the project's policy. The template does not assume whether that policy is human confirmation, dry-run, OS authentication, or a role check.
5. The infrastructure adapter performs one logical send. It retries transport only if the upstream operation is safe or uses one stable idempotency key.
6. The result returns the canonical message and room references needed by later reads or updates.

### Recovery probes

- Room name supplied where an ID is required: `invalid_input`; next action is room discovery.
- Room reference maps to multiple accounts/tenants: `ambiguous`; account-selection command is explicit.
- Missing send scope: `permission`; zero send attempts.
- Policy denial or missing impact dimension: `rejected` or `contract`; zero send attempts.
- Missing, extra, non-opaque, or reference-kind-mismatched mutation binding: catalog/contract rejection; zero send attempts.
- Timeout before execution: `canceled`/`unavailable`; zero or explicitly classified transport attempts.
- Timeout after an unknown upstream result: do not claim a safe retry unless idempotency proves it; provide a read/status action when available.
- Hostile room/message text: raw controls, format runes, line/paragraph separators, and delimiters cannot alter terminal or TSV/JSON structure. Existing backslashes remain distinguishable from projected controls. Printable JSON-looking or prompt-like prose remains present as untrusted data; the CLI makes no semantic prompt-injection-prevention claim.

### Acceptance

The agent never sends to a room selected implicitly by display name, can identify the exact input supplying the create parent or existing write target, can tell that sending has a notification side effect before executing it, and does not repeat an unsafe send after an uncertain failure. It treats every external text field as data rather than as a CLI-authored instruction.

## Runnable template probes

The synthetic sample flow is the executable minimum for these scenarios:

```sh
go run ./cmd/agentic-cli-foundry help --format agent
go run ./cmd/agentic-cli-foundry help sample --format agent
go run ./cmd/agentic-cli-foundry sample list --format json
go run ./cmd/agentic-cli-foundry sample read --id smp_2f4a6c8e0b1d --format json
go run ./cmd/agentic-cli-foundry --error-format json sample read --id smp_000000000000
```

The root agent contract must be schema version 6 with `view: index`, reveal the
`sample` namespace and both exact paths, and contain no input, output,
authentication, error, mutation, fixed-target, or workflow detail. Its
`scope_request` must identify the selector fields and exact invocation template.
Its two-invocation unknown-outcome bound means root index plus one selected
scoped contract; its one-invocation known-path bound assumes every required
reference and other task input is already held. Neither includes task execution
or later complete-contract retrieval for a workflow endpoint outside the
selected scope. The scoped contract must use `view: scope`, contain only the
relevant list/read commands, and represent the `sample` workflow as one
reference-kind group with unique `producers[]` and `consumers[]`. The producer
field plus consumer input and exact usage must provide the next argv without a
command-local duplicate edge. Its `invocation_grammar` must explain value and
boolean flag forms, equals-only dash-prefixed flag values, and the
positional-only marker. The complete global and selected command contracts
remain present, including fault-local recovery actions. Its `io_contract` must
publish `external_text_trust: untrusted_data`,
`external_text_projection: visible_escape`, and
`opaque_reference_policy: validated_exact_bytes`. The help catalog's
`CommandOutput.Fields` describes root `view: index` command entries; the
input-selected `view: scope` document is an independent variant under the same
schema version, with both views covered by dedicated exact-key contract tests.
The `id` selected from the list JSON is field extraction, not identifier
transformation: pass its exact string bytes to read. The final probe must fail as
`not_found`, use the dedicated exit status, write no success data to stdout, and
name `sample list` as the structured next action on stderr.

### Scoped-help footprint evidence

On the template catalog, the schema-4 (pre-schema-5) 2026-07-18 UTF-8
measurements were 1,517 bytes for root agent help, 5,359 bytes for exact
`sample read` help, and 8,359 bytes for the `sample` namespace. The 512-byte
limit continues to bound each root selection entry.

With schema 6, measured on 2026-07-19, the current root remains 1,517 bytes,
exact `sample read` help is 5,909 bytes, and the `sample` namespace is 8,814
bytes. The scoped increase records executable input types, cardinality,
defaults, and relations while the root remains selection-only.

Schema 6 retains the fixed derived-scale regression with six selected commands, 18
producer endpoints, 18 consumer endpoints, and 324 implicit same-kind edges.
The grouped document is 26,643 UTF-8 bytes; a pair-expanded representation of
the same facts is 179,909 bytes. The fixed corpus has a 65,536-byte
whole-response budget. The test expands the groups in memory and proves exact
edge-set equality, so meeting the budget cannot delete producer fields,
consumer inputs, usage, invocation contracts, or fault recovery. This is a
regression bound for the named corpus, not a claim that an arbitrary catalog
can never exceed 64 KiB.

Validation must also cover:

- every list-emitted sample ID passed unchanged to read;
- URL, name, partial, uppercase, whitespace, and control-character variants rejected before repository access;
- catalog/output snapshots detecting field or semantic changes;
- root-versus-scoped agent-help shape snapshots and a per-command root-size growth bound;
- executable checks that each single-shape JSON result's schema version,
  envelope, and item keys equal its `CommandOutput` declaration;
- help checks that root `commands[]` keys equal the help
  `CommandOutput.Fields` declaration and that dedicated exact-key contracts fix
  both root `view: index` and input-selected `view: scope` variants;
- adversarial TSV/JSON/stderr fixtures containing ESC, actual newline, bidi and zero-width format runes, U+2028/U+2029, literal backslash escapes, JSON-looking fragments, and prompt-like printable text;
- exact opaque-ID round trips alongside hostile labels/content, proving presentation never rewrites identity;
- complete pagination or no result;
- typed not-found recovery pointing back to discovery;
- structured contract visibility for effect, prerequisites, fields, delivery,
  collection coverage, errors, and next actions.
- declared default formats, JSON envelopes/schema versions, stdout/stderr ownership, and the complete exit-code map;
- successful output emitted only after complete pagination, validation, bounding, and rendering;
- root help that never embeds complete command contracts, plus namespace/exact scoped help that does not force the agent to ingest unrelated detail.

The sample is not evidence that a real API adapter is secure. A derived CLI repeats the scenario with fake adapter fixtures, authentication failures, pagination, cancellation, policy denial, and upstream error mappings before enabling a real network integration.

## Review record

Record the invocation transcript, number of discovery round trips, routine
external-processing count, selected output/reference fields, exact next argv,
and each recovery probe in the active work packet. If an agent needs prose
interpretation, source inspection, URL parsing, hidden filtering, a custom
join/parser, provider-notation decoding, an exploratory request, or an extra
command guess, treat that as product/thesis evidence rather than teaching the
agent a workaround.
