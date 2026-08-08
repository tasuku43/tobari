# External HTTP and provider contract

Tobari's enforcement contract remains the generic HTTP request crossing
Gateway. The Auth Broker adds one bounded provider-facing acquisition helper
for GitHub.com; it does not add provider-specific policy operations or a raw
provider API surface.

## OPA input

Gateway posts one JSON document to
`http://opa:8181/v1/data/tobari/http/decision` with schema version `5`:

```json
{
  "schema_version": 5,
  "principal": {
    "cluster": "default",
    "context_id": "01912345-6789-7abc-8def-0123456789ad",
    "project_id": "01912345-6789-7abc-8def-0123456789ab"
  },
  "request": {
    "authority": {"scheme": "https", "host": "api.github.com", "port": 443},
    "method": "GET",
    "path": {"raw": "/user", "segments": ["user"]},
    "query": {"key": ["value"]},
    "headers": {"content-type": "application/json"}
  },
  "authorization": {
    "requested_profile": null,
    "broker_provider": "github"
  }
}
```

`principal.context_id` and `principal.project_id` are required canonical UUIDv7
values established together from the Gateway local interface and owner-only
principal registry. Neither value is copied from a caller header, environment,
request URL, session field, provider ID, handle, or profile name. OPA uses the
Context ID at the fixed endpoint to select policy and denies an unknown,
absent, or mismatched principal without falling back to the current/default
Context.

Header names are lowercase. Host excludes a trailing dot and uses the
normalized request authority. Query values retain occurrence order. Request
and response bodies, decoded values, and hashes are outside the policy
contract.

The authorization object is always present:

- passthrough uses null `requested_profile` and null `broker_provider`;
- managed mode may provide a non-secret requested profile only after its
  Context/project/host precheck and keeps `broker_provider` null; and
- a recognized broker handle uses null `requested_profile` and the validated
  provider ID returned by non-secret broker introspection.

These fields are metadata for policy and validation. Neither selects a real
credential. OPA may select a managed `credential_profile` only for the managed
path. A brokered allow with a non-null selected profile is rejected before
secret resolution.

## Body-independent authorization and streaming

Gateway evaluates OPA from request headers before forwarding a body byte. It
enables request streaming only after principal validation, credential
preparation, policy allow, and any post-allow credential action succeed. It
streams the corresponding upstream response after its headers arrive. Body
presence and content therefore do not alter or split the exact
Context/project/host/port/method/path candidate or learned rule.

Mitmproxy retains a fixed 8 MiB `body_size_limit`. A message with a known
`Content-Length` above that value is rejected before the ordinary addon header
hook. An allowed unknown-length body has no total-byte limit in this contract
and is forwarded incrementally rather than buffered in full.

## Brokered-request ordering

A brokered request is recognized only from the schema-1 provider projection and
an exact HTTPS authority/header/syntax match. A Tobari handle marker in a URL,
cookie, header name, unsupported header value, or ambiguous syntax is rejected
before OPA. A malformed, misplaced, ambiguous, or binding-mismatched marker
fails as `credential_handle_invalid`; it never falls back or reaches upstream.
Only complete absence of a Tobari marker from every inspected URL/header
position selects the configured passthrough or managed fallback. Gateway
removes a valid declared placeholder header from the forwardable request, then
sends one schema-1 `introspect`
request over `/run/tobari-auth/runtime/broker.sock`. The request binds the
handle, stable Context/project/provider IDs, target, source header, and source
format. The response contains only the opaque credential revision and repeated
normalized target/source/destination/redaction metadata.

If OPA denies, Gateway never sends `resolve`. If OPA allows, Gateway sends
exactly one schema-1 `resolve` request with every introspection dimension plus
the returned revision. A valid response repeats the metadata and returns one
bounded `primary_secret` as unpadded base64url. Gateway validates the entire
response, replaces only the declared destination header, and makes one upstream
attempt. The secret is request-local and is never included in policy input,
audit, denial output, logs, or retry state.

The request body is never a credential source or replacement surface and is
not scanned for handle-shaped bytes. A body containing a Workspace-readable
handle has no broker meaning and follows the ordinary body-streaming contract.

The broker protocol uses strict exact-key newline-delimited JSON frames of at
most 64 KiB and a finite Gateway timeout of 2 seconds by default, configurable
only from 1 through 10 seconds. A protocol, timeout, socket, lock, or integrity
failure is unavailable, not permission to fall back with the same handle.

## OPA decision

OPA must return exactly one result object:

```json
{
  "allow": true,
  "reason": "allowed by policy",
  "credential_profile": null,
  "status_code": 403,
  "learnable": false
}
```

`status_code` is required and restricted to 403. `learnable` is required and
may be true only on a denial after version, cluster, scheme, fixed request port,
and trusted principal checks pass; managed requests additionally require their
profile binding. A broker provider is not itself permission and does not make a
denial learnable outside those same HTTP boundaries. Missing or wrong-typed
fields deny. `allow` is the only authorization fact; there is no `ask` or
pending state. Gateway owns audit emission.

## Timeouts, attempts, redirect, and retry

- OPA decision timeout: 2 seconds by default, configurable up to 10 seconds.
- Auth Broker runtime timeout: 2 seconds by default, configurable from 1 to 10
  seconds.
- Upstream connection/request timeout: 30 seconds by default, configurable up
  to 120 seconds.
- Maximum upstream attempt count: one.
- Gateway never retries because it cannot infer idempotency from an arbitrary
  HTTP request.
- Each proxy redirect request is independently normalized and authorized; a
  prior allow does not authorize the redirected authority or path.
- Cancellation closes the proxy flow and does not convert an unknown upstream
  mutation outcome into replay permission.

## GitHub.com acquisition helper

`auth login github` is the only supported provider-facing acquisition flow. A
trusted interactive host command runs these fixed GitHub CLI operations in an
ephemeral configuration directory:

```text
gh auth login --hostname github.com --web
gh auth status --active --hostname github.com --json hosts
gh auth token --hostname github.com
```

`GH_PROMPT_DISABLED=1`, `GH_BROWSER=/bin/true`, and `NO_COLOR=1` are fixed for
the ephemeral helper environment. Ambient browser selectors are removed. The
trusted host output boundary recognizes only the exact
`https://github.com/login/device` constant and invokes the platform opener once;
failure retains a manual fixed-URL instruction and does not fail acquisition.
Omitting `--git-protocol` is intentional: Auth Broker is acquiring GitHub API
authentication and must not ask about, configure, or require Git credential
handling. The temporary GitHub CLI plaintext-storage warning is expected for
the private tmpfs and is the only fixed helper line withheld from public output.

Ambient `GH_TOKEN`, `GITHUB_TOKEN`, enterprise-token, host, and repository
variables are removed. The login call owns the ordinary GitHub CLI web/device
interaction. Tobari neither interprets OAuth endpoints nor follows provider
responses itself. It validates exactly one active successful GitHub.com account
from bounded status JSON, captures one token of at most 32 KiB internally, and
removes the temporary directory even on failure. The sequence has no
pagination or automatic retry. It is bounded by the user's interactive
completion and command cancellation; cancellation or any failed capture leaves
the previous Context credential unchanged.

This helper does not refresh tokens, select among multiple Context accounts,
revoke a token remotely on logout, inspect scopes, call the GitHub REST API as
an application adapter, or interpret GitHub business operations. The GitHub CLI
is pinned to version 2.96.0 in the Auth Broker image and its Linux amd64/arm64
archives are checksum-verified.

Owner-controlled providers use `auth import` and make no external call during
acquisition. Their schema-1 manifests may declare an exact HTTPS header
transformation, but they cannot declare executable helpers, methods, paths,
pagination, retries, signing, or refresh behavior. Terminal stdin is refused
before reading. Non-terminal input is read after public Context/provider
argument, intent, and mutation validation; infrastructure validates the
selected existing Context, installed provider/acquisition mode, and broker
readiness before broker send. A provider collection with overlapping exact
scheme/host/port/source-header/source-format recognition fails completely as
`ambiguous_provider_http_binding` rather than partially activating.

## Gateway audit

Every validated allow/deny audit record uses `schema_version: 2` and contains
Context name and stable ID, project ID and safe root, cluster, request
authority, method, redacted path, decision, reason, adapter-dependent credential
profile name, upstream outcome, and timing. It contains no query or headers.
The ordinary value is only the URL path component; when that path contains a
Tobari handle marker, the whole value is `/[redacted-auth-handle]`. Structural
URL/header handle rejections are non-learnable and cannot become policy
candidates. A broker provider may be present only as secret-free authorization
metadata; a handle, credential revision, primary secret, request body, and raw
authorization value are excluded. Host-side
denial, candidate, learned-rule, and compaction projections retain the same
Context/project pair so an opaque approval cannot lose its authority scope.

## Errors

Policy denial returns 403. A learnable response contains only the existing
fixed secret-free `tobari policy review` navigation; it has no candidate ID,
query, body, header, provider handle, credential, policy path, or dynamic
command argument. Non-learnable policy denial advertises no review command.

A copied, malformed, stale, revoked, ambiguous, or Context/project/provider/
target/header-mismatched or structurally misplaced handle returns HTTP 403 with
`credential_handle_invalid`. A locked or unavailable broker, socket timeout,
invalid response, inconsistent revision, or malformed secret returns HTTP 503
with `credential_broker_unavailable`. Every Tobari-looking marker follows one of
those fail-closed paths unless it is a valid exact broker candidate; fallback
requires that no marker exists anywhere inspected. Gateway removes a valid
candidate before broker/OPA processing, so a failed candidate cannot reach OPA
logs, denial output, or upstream.
OPA unavailability or malformed decisions return 503 `policy_unavailable`.
Other Gateway normalization failures remain secret-free 4xx/5xx failures.

## Schema and compatibility

The OPA input schema version `5`, audit schema version `2`, decision fields,
timeouts, attempt count, provider manifest/projection schema `1`, broker
control/runtime schema `1`, encrypted vault schema `1`, handle prefix
`tobari-h1_`, and Auth Broker image API label `1` are explicit pre-v1
compatibility boundaries. Gateway does not accept former OPA input shapes,
incomplete decisions, or unknown broker frames.
Public auth backend values are exactly `macos_keychain|xdg_file`, and cluster
status may additionally report `unavailable`; the infrastructure/doctor label
`linux_xdg_file` is not a public JSON enum. The canonical schema, state-path,
socket, handle, and backend inventory is in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

The principal registry remains schema version 2. Each Context policy data
source uses `tobari.schema_version=2`; current Rego source targets OPA input
schema 4. Aggregate projection schema 1 accepts legacy source input schema 3
only for compatibility, rewrites both source versions to runtime schema 5,
stores Context data below `tobari_contexts[context_id]`, and rejects other
shapes. Guided Contexts share one system evaluator, while Advanced source is
projected into a Context-ID package and cannot claim router or system
namespaces.

No upstream provider response fixture is vendored. The repository-authored
synthetic provider manifest is pinned as `auth-provider.v1` in
`.harness/schemas.json` with MIT provenance and an exact digest. GitHub status
tests use synthetic JSON and mock subprocess results. A live GitHub login is
manual release evidence and may not be recorded with tokens, device codes,
vaults, or raw authenticated output.

## Policy testing

Pinned OPA tests prove schema-5 rejection of older or incomplete input,
deny-by-default behavior, structured authority and port boundaries,
body-independent decisions, Context/project-bound learned rules, null versus
broker-provider authorization metadata, and managed profile binding. Gateway
tests independently prove handle removal, deny-before-resolve, exact
post-allow replacement, and fallback compatibility.
Fallback tests require marker absence in every inspected URL/header position;
audit tests require query/header omission, whole-path marker redaction, and
non-learnable structural handle rejection.
