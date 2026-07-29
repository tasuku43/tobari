# Security Model

This document defines the minimum security reasoning expected from Agentic CLI Foundry and every derived project. It does not claim that the base scaffold makes project-specific integrations secure. Its purpose is to make assets, effects, trust boundaries, and enforcement visible before implementation spreads them across commands.

## Security objective

The CLI must not perform a side effect broader than the validated user task and declared `operation.Intent` and `operation.Impact`. Invalid, unknown, or inconsistent operation state fails before external I/O. Credentials, confidential identifiers, and private repository history must not enter public source or artifacts.

## Assets in the base template

- Integrity of the user's local system and files.
- Confidentiality of arguments, environment values, and future credentials.
- Integrity of command selection, effect classification, and target binding.
- Integrity of source, dependencies, generated files, and release artifacts.
- Confidentiality of information excluded by the public-repository policy.
- Availability of a deterministic local verification path.

A derived project must add every remote account, object type, local state file, credential, cache, log, subprocess, network service, and published artifact it introduces.

## Actors and assumptions

The model distinguishes:

- **User:** invokes the CLI and may provide malformed input accidentally.
- **Coding agent:** can edit files and execute commands within its granted environment; it is not proof of human authorization.
- **Contributor:** can propose source changes but cannot redefine policy without review.
- **External system:** returns untrusted data and may fail, drift, rate-limit, or be compromised.
- **Attacker:** may control input, remote content, a dependency, a copied file, or a proposed patch.
- **Release owner:** is trusted to follow the reviewed release procedure, not to bypass it manually.

The base model does not assume that a TTY, parent process, environment variable, or successful process launch proves human intent.

## Trust boundaries

```text
untrusted argv / environment / files
                |
                v
       CLI parsing and validation
                |
                v
      application task boundary
                |
                v
 Effect + Intent + TargetRef + Impact validation
                |
                v
     controlled side-effect boundary
                |
      +---------+----------+
      |         |          |
 filesystem  process    network / platform service
```

Remote responses cross the boundary in the opposite direction. They remain untrusted data after parsing, bounding, and safe structural rendering; those steps do not turn remote prose into trusted instructions.

## Effect and intent policy

| Effect | Required declaration | Base policy |
|---|---|---|
| `Read` | Stable task and bounded source | May execute without mutation authorization; still validates input, destination, and output bounds |
| `Create` | Parent or creation scope in `TargetRef`; complete base `Impact` | Fails closed without complete intent and impact; derived project decides confirmation or human authorization |
| `Write` | Existing target in `TargetRef`; complete base `Impact` | Fails closed without complete intent and impact; product-specific consequences require additional typed detail |
| `Unknown` | None | Never executable |

HTTP methods, SDK names, or command verbs do not determine the effect. A query sent with `POST` may be a read; writing a local output file is a side effect even when no network request occurs.

The base `Impact` always declares cardinality and whether the operation sends notifications, changes access, or is destructive. When those dimensions cannot explain policy, extend intent with a typed domain model before exposing the command. Examples include:

- multiple targets;
- irreversible deletion;
- access or ownership changes;
- messages or notifications sent to other people;
- execution of user-provided code;
- publication outside the user's account or machine.

Do not encode these distinctions only in a human-readable confirmation string.

## Opaque reference boundary

Discovery output may contain labels controlled by an external system, but action identity comes from the declared opaque reference field. Pass the emitted ID unchanged into the consuming argument.

- Validate the documented shape and length before adapter execution.
- Do not authorize against a display name, row position, copied URL, or reconstructed resource path.
- Do not case-fold, decode, unescape, trim, or otherwise normalize an opaque ID unless its domain contract explicitly defines that operation.
- Bind mutation `TargetRef` to the same validated ID accepted by the action command.
- Validate a returned reference against the kind required by its semantic field,
  not only against a shared byte shape. A valid reference of another kind
  remains a contract violation rather than displayable data.
- Keep remote labels out of authorization identity and sanitize them separately for display.
- Reject control/format runes and Unicode line or paragraph separators at opaque transport boundaries where they have no valid protocol role. Validation rejects; it never silently rewrites an ID, cursor, target part, or idempotency key.

The sample contract accepts only `smp_` followed by twelve lowercase hexadecimal characters. Its negative tests reject alternate forms before the sample adapter runs.

A command-bound `tool_local` fixed target is the only no-reference action mode. It is safe only when the command path identifies exactly one CLI-owned local object and the catalog fixes its stable kind, ID, scope, and description. It cannot represent an external object, account choice, arbitrary path, or candidate set. Fixed-target mutations still construct a matching runtime `TargetRef`, complete impact, intent, policy decision, and outcome contract before I/O.

## Controlled execution boundary

All filesystem writes, subprocess execution, credential access, network calls, and platform services must have a bounded construction path. A command or use case must not receive an unrestricted client merely because it is convenient.

For a mutation, the boundary performs this order:

1. Snapshot and validate the operation input.
2. Validate `Effect`, `Intent`, `TargetRef`, `Impact`, and concrete destination consistency.
3. Apply project-specific policy such as dry-run, human authorization, or confirmation.
4. Execute the side effect once.
5. Return a bounded result and audit event if the product requires one.

A rejection at steps 1–3 must result in zero mutation attempts. Tests use fakes or counters to prove this order.

Cancellation is interpreted relative to that boundary. Before step 4, the invoker can prove zero attempts and may expose retryable `operation_canceled`. After step 4 begins, a valid structured adapter fault remains authoritative and is copied without its private cause. A raw error or cancellation cannot prove rollback; it becomes non-retryable `unclassified_mutation_outcome`, whose catalog recovery must be a read-only reconciliation command. A confirmed nil result is not replaced by a later context cancellation, including at the final CLI write: the effect-aware output finalizer preserves confirmed mutation success, while read output retains the pre-write cancellation check. If that confirmed result cannot be written completely, `mutation_output_write_failed` is non-retryable and also permits only read-only reconciliation.

Provider rate evidence and replay permission are different security facts.
`retry_after` may carry an authoritative positive wait window on a
non-retryable `rate_limited` mutation. Only `retryable` authorizes repeating the
same logical command; timing can guide reconciliation, a different safe read,
or a later independently authorized operation.

Application ports treat both a nil interface and an interface containing a typed nil pointer as missing configuration. The shared port check prevents a dependency-wiring mistake from becoming a panic after validation or policy approval.

## Credentials and secrets

The base template contains a secret-free authentication contract, an ephemeral non-serialized session binding, an application gate, and a separate provider-neutral store for bounded non-secret user authentication configuration, but no concrete credential acquisition or credential storage implementation. A derived project must document:

- credential issuer and scope;
- acquisition and refresh flow;
- storage location and operating-system protection;
- how secrets are kept out of process arguments, logs, errors, history, and generated files;
- revocation and expiration behavior;
- tests and scans that fail on unsafe handling.

Secrets must not cross from infrastructure into domain or application values and must not be accepted through command-line arguments when a safer channel is available. Public client identifiers, public redirect URIs, the explicitly selected method, and schema metadata may use the non-secret configuration boundary; token, PAT, refresh token, authorization code, PKCE verifier, and client secret never do. The boundary uses strict bounded decoding, rejects unknown schema, symlinks, and non-regular files, confines replacement through a verified opened directory, revalidates the directory, target, and staged-file identities immediately before replacement, and reports corrupt state read-only rather than repairing or falling back. On Unix it also rejects unsafe owner modes and syncs the replaced directory. Windows still enforces regular shape and confinement, but portable mode bits do not prove ACL ownership and the portable API does not justify an atomicity or durability claim, so an error after replacement begins leaves the active version explicitly uncertain. Do not persist tokens in plaintext configuration or test real credentials in CI. Read [Authentication](07_authentication.md) and [ADR 0001](decisions/0001-oauth-library-boundary.md) before implementing OAuth or PAT support.

## Filesystem, process, and network policy

### Filesystem

- Validate output paths and overwrite behavior before writing.
- Use restrictive permissions for confidential data.
- Define atomicity, symbolic-link, and partial-write behavior where relevant.
- Keep caches, state, configuration, and audit data separate.

### Processes

- Prefer in-process APIs.
- If a subprocess is required, allowlist the executable and fixed argument structure.
- Keep secrets out of argv and inherited environment unless explicitly justified.
- Bound output, time, and cancellation behavior.

### Network

- Declare allowed destinations and redirect behavior.
- Use timeouts, cancellation, and response-size limits.
- Validate protocol and host before sending credentials or user data.
- Treat remote names and content as unsafe terminal and machine context.

Machine-readable policy belongs in `.harness/project.json` or a project-specific typed manifest and is checked by `tools/repoguard` or a dedicated contract test.

## Output and terminal safety

Remote or file-derived text may contain terminal controls, format characters, Unicode line/paragraph separators, existing backslash escape-looking sequences, JSON-looking fragments, prompt-like prose, or excessive data. The template's visible projection escapes backslash first, then control/format runes and U+2028/U+2029. Therefore an actual newline remains distinguishable from the two input characters `\n`; JSON encoding applies its own structural escaping afterward. TSV delimiters and record newlines are added only by the renderer.

This projection protects terminal and TSV/JSON structure. It does **not** detect intent in printable text, remove prompt-like content, or prove that a language model will ignore it. Printable text such as `SYSTEM ...` or JSON-looking content remains semantically untrusted data. Agent consumers must keep data fields separate from instructions and apply their own trust policy; the CLI cannot claim semantic prompt-injection prevention.

Scoped agent help publishes this boundary in `io_contract`: `external_text_trust` is `untrusted_data`, `external_text_projection` is `visible_escape`, and `opaque_reference_policy` is `validated_exact_bytes`. A configured documentation locale governs trusted repository and CLI-authored prose only. It never translates stable command paths, flags, environment variables, fault codes, JSON keys, schema values, reference kinds, or provider-controlled text.

A derived project must decide:

- which output modes are stable;
- the exact collection scope, observation/window/checkpoint, and whether
  coverage is exhaustive, bounded, differential, or not applicable;
- how control characters are escaped or rejected;
- maximum stdout and stderr budgets;
- whether partial output is ever allowed;
- how secrets and confidential fields are redacted;
- which stream carries errors versus data.

Do not let presentation sanitization change the identity used for authorization. Authorization uses validated domain values; display labels/content use a visible projection. Opaque references bypass that projection and retain the exact validated value.

Presentation is also a semantic trust boundary. It must not invent or strengthen
identity, scope, relationships, completeness, or uncertainty from external
prose, item order, display names, proximity, quoting, or indentation. A result
for a scoped task retains that declared scope even when its collection is
empty; recovering scope from a member would make empty and adversarial
collections ambiguous. Task-owned tests cover only the request dimensions and
state distinctions the capability actually carries, rather than assuming one
universal result shape.

## Supply-chain boundary

- Add a dependency only with a documented purpose and license review.
- Pin CI actions and security tools according to repository policy.
- Verify module integrity and known vulnerabilities in the security profile.
- Review generated changes and prove generation is reproducible.
- Build releases from reviewed source through the documented workflow.
- Reopen each completed archive and verify its exact canonical entry set, mode,
  metadata, and bytes against reviewed executable, license, and optional notice
  inputs before publication.
- Decide signing and provenance explicitly; absence of signing is also a release-model decision.

## Required negative tests

Every new side-effect class should include tests for:

- unknown or missing effect;
- incomplete or mismatched target;
- malformed input before adapter invocation;
- alternate, transformed, or ambiguous reference forms before adapter invocation;
- policy or authorization rejection with zero side-effect attempts;
- nil and typed-nil external ports with zero downstream calls;
- cancellation and timeout;
- structured versus unclassified post-mutation outcomes, including a deadline cause that must not erase the adapter's stable classification;
- remote error without secret leakage;
- authentication or scope mismatch with zero downstream calls;
- incomplete pagination with no partial successful output;
- unsafe mutation retry or missing idempotency declaration;
- timeout or attempt configuration above the project's declared maximum;
- keyed retry that changes key within one logical operation or reuses a key for a different target or payload;
- oversized or hostile output;
- raw ESC/newline/bidi/zero-width/U+2028/U+2029 characters, existing backslashes, JSON-looking text, and prompt-like printable data across TSV, JSON, and stderr;
- actual control characters remaining distinguishable from pre-existing visible escape-looking text;
- repeated invocation when authorization must not be reused.

## Derived-project threat-model pass

Before implementing a real integration, add a table covering:

| Task | Assets | Inputs | Effect | Target or impact | External boundary | Authorization | Stored data | Enforcement |
|---|---|---|---|---|---|---|---|---|

Then document credible abuse cases and limits. If a risk is accepted rather than eliminated, record the rationale in an ADR and state what evidence would trigger reconsideration.

## Limits

The template cannot protect a fully compromised developer machine, maintainer account, CI platform, or external service. It cannot infer whether copied source is legally publishable. It narrows accidental bypasses and makes review evidence repeatable; it does not replace independent review for high-impact systems.
