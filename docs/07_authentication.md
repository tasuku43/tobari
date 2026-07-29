# Authentication Foundation

This document defines the reusable OAuth 2.0 and personal access token (PAT) boundary supplied by Agentic CLI Foundry. The template fixes secret-free contracts and fail-closed application behavior. A derived project chooses the actual provider flow, credential source, storage, scopes, and account policy in its thesis and security model.

The governing dependency decision is [ADR 0001](decisions/0001-oauth-library-boundary.md): do not implement OAuth protocol machinery from scratch, and do not add an unused OAuth dependency to the template core.

## What the template guarantees

The base repository supplies:

- `authn.Method` for `oauth2` and `pat`, with an invalid zero value;
- `authn.Requirement`, which declares allowed methods, authority, audience, optional account binding, and project-defined required capabilities;
- `authn.Session`, which contains only non-secret identity and authorization metadata;
- an opaque, non-serialized `authn.BindingID` that correlates a validated session with one infrastructure-owned authentication record without exposing a credential or storage handle;
- exact requirement-to-session matching without normalization or URL parsing;
- expiry and capability checks;
- an application `authn.Gate` that authenticates once, revalidates the session, rechecks cancellation, and calls the downstream action once;
- stable `fault` classifications for missing authentication, expired authentication, context mismatch, insufficient capability, cancellation, and unclassified adapter failure;
- removal of upstream causes before structured failures cross the application boundary;
- tests proving that malformed declarations, authentication failure, cancellation, session mismatch, and capability failure cause zero downstream action calls.

The zero values of `Method`, `Requirement`, `BindingID`, `Session`, and `Gate` are not usable defaults. Missing declarations fail closed.

## Runtime boundary

```text
CLI composition root
        |
        | secret-free Requirement
        v
application authn.Gate
        |
        | Authenticate(Requirement)
        v
infrastructure Authenticator ----> credential source / OAuth library
        |
        | secret-free Session metadata
        v
application revalidation
        |
        | Session.BindingID, unchanged
        v
authenticated action ------------> task port ------------> infrastructure credential manager
                                                               |
                                                               | exact binding + freshness/refresh
                                                               v
                                                        provider request
```

The raw PAT, OAuth access token, refresh token, authorization header, authenticated HTTP client, callback verifier, and credential-store handle remain inside `internal/infra`. They are not fields of a domain or application type, arguments to a public command, return values from a use case, or values rendered by the CLI.

`Session` is metadata, not a bearer capability. The action receives a copy and passes its `BindingID` unchanged to the task-specific application port. The infrastructure implementation uses that opaque value as a process-local map correlation to resolve its private authentication record immediately before credential use. The binding representation is private, omitted from JSON, redacted by generic `fmt` diagnostics, and unsuitable as a provider identifier, token, persistent credential-store key, or public command value. Architecture lint rejects production calls to `NewBindingID` outside infrastructure, including dot-import bypasses, so application or CLI input cannot be promoted directly into a binding.

This closes accidental application wiring in which authentication validates account A but a task port silently uses a default credential for account B. It does not let the base gate inspect a provider token. The infrastructure authenticator and credential manager must prove in adapter tests that the issued binding record contains the same method, authority, audience, subject, account, capabilities, and expiry represented by the returned session.

## Secret-free contracts

### Method

The core recognizes two credential families:

| Method | Meaning | Not implied |
|---|---|---|
| `oauth2` | The infrastructure adapter established an OAuth 2.0 session | Grant type, browser/device flow, refresh, storage, or provider endpoints |
| `pat` | The infrastructure adapter established a PAT-backed session | Token source, lifetime, permission model, or account selection |

Adding another method requires a domain change, tests, security-model update, and compatibility review. Treating an unknown method as PAT or OAuth is forbidden.

### Requirement

`Requirement` is created from a reviewed command/use-case contract, not inferred from a received token. It declares:

- one or more allowed methods;
- a stable authority identifier;
- a stable audience identifier;
- an optional exact account binding;
- zero or more exact project-defined capabilities.

Authority and audience are public internal identifiers, not credential endpoints. Capabilities may represent OAuth scopes, PAT permissions, or higher-level project permissions, but the template does not define their names.

### Session

`Session` reports:

- the method actually used;
- the exact authority and audience;
- a stable subject identifier;
- an optional account identifier;
- granted project-defined capabilities;
- advertised expiry, when the credential has one;
- an ephemeral opaque binding to the infrastructure-owned authentication record, excluded from JSON and ordinary output.

A zero expiry means only that the adapter did not advertise expiry. It does not mean that the credential is permanent or safe to cache. A derived security model may require expiry for its selected method.

Do not render `Session` as normal command output. If a diagnostic command needs authentication information, expose a deliberately reduced projection such as method, status, and expiry class; never include raw provider responses or secret-bearing causes.

## Gate behavior

Before calling an authenticated action, `internal/app/authn.Gate` performs this order:

1. Reject a nil context, nil action, invalid requirement, missing authenticator, or missing clock.
2. Reject cancellation before credential resolution.
3. Ask the infrastructure authenticator for secret-free session metadata exactly once.
4. Sanitize unstructured authenticator errors and strip causes from valid structured faults.
5. Reject cancellation after credential resolution.
6. Validate the session metadata, including its ephemeral binding.
7. Match method, authority, audience, optional account, expiry, and every required capability exactly.
8. Reject cancellation immediately before the action.
9. Call the action exactly once with a session metadata copy.

Steps 1–8 produce zero downstream action calls on failure. The action is responsible for passing `Session.BindingID` unchanged to its task port. The port and credential manager honor cancellation after the action begins and classify provider failures as safe structured faults.

Exact matching is intentional. The gate does not trim, case-fold, decode, parse URLs from, or otherwise transform authority, audience, account, subject, or capability values.

## Failure and recovery contract

The base gate emits stable recovery classes:

| Condition | Fault kind | Stable code | Retry assumption |
|---|---|---|---|
| Gate context missing | `contract` | `missing_authentication_context` | No; repair the application wiring |
| Authenticated action missing | `contract` | `missing_authenticated_action` | No; configure the use case action |
| Authentication requirement invalid | `contract` | `invalid_authentication_requirement` | No; repair the catalog/use-case contract |
| Authenticator not configured | `authentication` | `missing_authenticator` | No; configure the project-selected method |
| Gate clock missing | `contract` | `missing_authentication_clock` | No; repair the application wiring |
| Credential resolution failed without a safe classification | `authentication` | `authentication_failed` | No automatic retry |
| Session metadata or ephemeral binding invalid | `authentication` | `invalid_authentication_session` | No; fix the adapter contract |
| Valid metadata could not be evaluated | `contract` | `authentication_evaluation_failed` | No; repair the gate/domain contract |
| Method, authority, audience, or account mismatch | `authentication` | `authentication_context_mismatch` | No; select the correct credential/account |
| Session expired | `authentication` | `authentication_expired` | No; reacquire according to project policy |
| Capability missing | `permission` | `insufficient_authentication_capability` | No; obtain the documented permission |
| Context canceled before the action | `canceled` | `authentication_canceled` | Only when the caller chooses a new attempt |
| Action returned an unstructured error | `internal` | `unclassified_authenticated_action_error` | No; the adapter must classify it |

A non-nil catalog `AgentContract.Authentication` binds the command to this template gate. Catalog validation therefore requires every stable code above with its exact kind and `retryable: false`; an omission or mismatch fails before dispatch. Provider-specific structured faults that the gate may pass through, such as refresh rejection, rate limiting, or temporary unavailability, remain additional derived-project declarations with command-valid recovery actions.

A derived CLI should attach concrete, command-valid next actions through its command/error catalog, for example a login command or a permission-inspection command. The template cannot name such a command before the product contract chooses whether one exists.

Rate limits, temporary identity-provider failures, and unsupported flows may pass through when an infrastructure adapter returns a valid, explicitly public structured fault. The gate removes its underlying cause because provider errors may contain authorization headers, request URLs, or credential material.

After the action begins, a valid structured adapter fault remains authoritative even if the caller context is also canceled. This preserves a known authentication, permission, unavailable, or unknown-outcome classification instead of replacing it with a less precise cancellation. An unstructured action error is mapped to cancellation when the caller context is canceled; otherwise it is collapsed to the internal fallback.

## Binding, expiry, and refresh at the I/O boundary

The gate's expiry check is an admission snapshot, not a lease over the subsequent provider request. The template does not choose an expiry headroom, refresh threshold, cache lifetime, or reuse policy because those values depend on the provider and operation budget. Every derived authenticated task follows this minimum shape:

1. Infrastructure creates an ephemeral `BindingID` independently of token, secret, provider identifier, and credential-store path bytes.
2. The authenticator stores the actual credential and its exact session metadata in an infrastructure-owned record, then returns only the secret-free session.
3. The use case passes `Session.BindingID` unchanged into its task port; it never selects an account by a global default after the gate.
4. The port resolves that binding immediately before I/O, verifies that the record still represents the validated authority, audience, subject/account, and capabilities, and checks expiry using the same caller context.
5. When the derived policy permits refresh, infrastructure refreshes inside that bound record. A refreshed identity or account mismatch is an authentication failure and makes zero provider task requests.
6. Refresh rejection is classified as authentication; insufficient provider permission is permission; a retryable identity-service outage may be unavailable. Causes and provider bodies remain private.
7. A missing, unknown, stale, or mismatched binding fails before the provider task request. A typed-nil task port fails at the use-case boundary through `portcheck.IsNil`.

The binding is process-local correlation metadata. Do not persist it, cache it across sessions, render it, log it, accept it from a user, or use it as proof of possession. Provider flow, storage, refresh locking, cache policy, and approval remain derived-project decisions.

## Non-secret user configuration

`authn.UserConfiguration` is persistable setup metadata, not credential material. It records a supported schema version, one explicitly selected authentication method, and a bounded list of public name/value parameters. Suitable parameters include a public client ID or public redirect URI. Tokens, PATs, refresh tokens, authorization codes, PKCE verifiers, client secrets, authorization headers, and credential-store handles are forbidden even if a provider happens to serialize one as text.

The application resolver gives a present environment source precedence over persistent configuration. A present but invalid environment value fails closed; it does not reveal or fall back to the persistent value. Missing environment configuration permits the persistent source. Corrupt, unsafe, or unknown-schema persistent configuration is an error, not absence. Once a method is selected, authentication invokes only that method; failure never triggers probing or fallback to another credential family.

The provider-neutral file store accepts an injected absolute path and creates no
default state or parent directory. It strictly decodes one bounded JSON value,
rejects unknown fields and schema versions, validates bounded unique public
parameters, and on Unix requires owner-only directory and regular-file modes.
It rejects symbolic links and non-regular targets, opens and verifies the
parent as a confined filesystem root, stages an owner-only same-directory file,
and revalidates the requested directory identity, staged-file identity and
size, and target shape immediately before replacement. Unix then syncs the
opened directory after rename. Windows still enforces regular shape and
confined replacement, but portable mode bits do not prove ACL ownership and
the standard API makes no atomicity or directory-durability guarantee. Once
replacement begins, an error therefore does not promise which version is
active. Read-only status reports missing, valid, or invalid state and never
repairs a partial or corrupt result. Credentials require a separate
project-selected store and threat model.

## OAuth decision

OAuth protocol behavior is security-sensitive and provider-dependent. The template must not implement authorization URL construction, state or nonce validation, PKCE, callback handling, code exchange, token parsing, refresh, or authenticated transport itself.

When a derived project selects OAuth 2.0:

1. Record the grant and callback/device model in an accepted ADR.
2. Prefer `golang.org/x/oauth2` as the first candidate unless a provider requirement justifies another maintained implementation.
3. Add the dependency only to the derived project and import it only from `internal/infra`.
4. Pin the selected version and review its license, maintainers, transitive graph, release history, and vulnerability status.
5. Use the library for protocol machinery while retaining provider-specific validation and policy in the adapter.
6. Run `go mod verify`, `govulncheck`, dependency review, adapter tests, and the full one gate.

The derived design also separates four responsibilities: protocol code constructs and validates the authorization exchange; CLI presentation prints a usable manual URL; a platform/process adapter may attempt browser auto-open; and a bounded callback adapter receives the redirect. Auto-open failure returns the same manual URL rather than changing flow or method. Passing an authorization URL through a browser subprocess argv retains process-list and diagnostic-log exposure of its public query metadata and must be documented. Authorization codes, PKCE verifiers, access/refresh tokens, client secrets, and PATs never enter subprocess argv. Callback listeners bind the documented local address, validate state/PKCE through the reviewed OAuth implementation, bound time and requests, and return no provider secret to presentation.

This accepts a bounded supply-chain dependency in preference to maintaining a private OAuth implementation. The dependency is optional so PAT-only projects do not inherit that risk or update burden.

A provider SDK is not the default. Use one only when an ADR demonstrates that provider-specific behavior materially reduces total security or maintenance risk and its dependency surface is acceptable.

## PAT decision

PAT support uses the same requirement, session, gate, fault, and adapter boundaries. A derived project still decides:

- how the PAT is supplied or acquired;
- whether an operating-system credential store, environment input, standard input, or another mechanism is acceptable;
- how authority, audience, subject, account, permissions, expiry, and revocation are verified;
- whether the PAT may be cached or reused;
- how the user replaces or revokes it.

Do not accept a PAT as a normal command-line flag: argv commonly reaches shell history, process inspection, logs, and agent transcripts. Never persist it in plaintext project configuration.

## Decisions left to a derived project

The template intentionally does not fix these choices:

| Decision | Where to decide it |
|---|---|
| OAuth grant, browser/device behavior, redirect listener, callback URI, state, nonce, and PKCE policy | Product contract, security model, and OAuth ADR |
| Provider endpoints and client registration | Infrastructure configuration and security model |
| PAT input mechanism | Security model and CLI contract |
| Credential store and operating-system integration | Security model and infrastructure ADR |
| Refresh, reuse, cache lifetime, logout, and revocation | Security model and authentication ADR |
| Concrete scopes, PAT permissions, and capability names | Product contract and security model |
| Account discovery and selection | User-task workflow and product contract |
| Whether a write requires human approval, reauthentication, or dry-run | Thesis, security model, and mutation policy |
| Authentication commands and recovery command names | Command catalog |

These are not optional decisions. They are deliberately deferred until a real external API and user task make the tradeoffs concrete.

## Derived adapter requirements

An implementation of `app/authn.Authenticator` must:

- own every credential-bearing value and dependency in `internal/infra`;
- return only validated, secret-free `domain/authn.Session` metadata;
- issue a fresh non-secret binding independent of credential bytes and bind the returned metadata to that exact infrastructure record;
- respect context cancellation and project timeouts;
- map expected provider failures to stable, redaction-safe `fault.Error` values;
- avoid including provider response bodies, headers, URLs with query values, token claims, or secret values in public messages;
- avoid logging secrets at every log level;
- make account, audience, and capability mismatches observable without echoing their unsafe values.

Every authenticated task port must accept the opaque `authn.BindingID` supplied by the use case. Its infrastructure adapter resolves and revalidates that record immediately before credential use rather than consulting a process-wide default account. This is a structural application-port input, not a credential-bearing type crossing into application code.

The API adapter called after the gate must return structured faults for expected not-found, permission, rate-limit, unavailable, and contract failures. An unstructured error is intentionally collapsed to `unclassified_authenticated_action_error` so accidental provider prose is not exposed.

## Required verification in a derived project

Keep the template tests and add adapter-specific tests for:

- empty, malformed, expired, and revoked credentials;
- wrong authority, audience, subject/account, and permission set;
- two simultaneously available accounts/authorities/audiences, proving that a session for one cannot drive the other's task port record;
- missing, unknown, stale, mismatched, and cross-session binding IDs with zero provider task requests;
- callback state mismatch and PKCE failure for an applicable OAuth flow;
- cancellation during acquisition, exchange, refresh, and API execution;
- refresh failure and reuse behavior selected by the security model;
- expiry between gate admission and task-port I/O, including refresh success, refresh identity mismatch, refresh failure, and cancellation with zero unintended requests;
- credential-store unavailable and access-denied behavior;
- authentication failure with zero API calls;
- permission mismatch with zero API calls;
- typed-nil authenticator and task ports with zero acquisition or API calls;
- secret canaries absent from stdout, stderr, structured errors, logs, snapshots, fixtures, and test failure output;
- exact dependency and vulnerability checks after adding an OAuth library or credential-store module.

Use fake credentials and publishable provider fixtures. Never place a real token, client secret, private callback registration, internal endpoint, or account identifier in source, history, CI, release artifacts, or examples.
