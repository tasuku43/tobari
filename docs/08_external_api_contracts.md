# External HTTP and provider contract

Tobari's enforcement contract remains the generic L7 request crossing
Gateway: ordinary HTTP identity, plus GraphQL operation type and canonical
root field at an exact trusted-host-declared endpoint. Reviewed trusted-host drivers add two bounded provider-facing
acquisition flows: fixed GitHub CLI device login and fixed AWS CLI IAM Identity
Center device login. Auth Broker owns encrypted state, handles, revisions, and
signing; a resident private companion performs only post-policy AWS CLI
credential export. This still adds no provider-specific policy operation or raw
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
contract. At a declared GraphQL endpoint only, `request.graphql` is also
present as exactly `{"operation_type":"query|mutation","root_fields":[...]}`.
The sorted unique root names are derived from the selected executable document;
the source document, operation name, variables, arguments, directives,
fragments, aliases, literals, extensions, and nested fields are excluded.

The authorization object is always present:

- passthrough uses null `requested_profile` and null `broker_provider`;
- managed mode may provide a non-secret requested profile only after its
  Context/project/host precheck and keeps `broker_provider` null; and
- a recognized broker handle uses null `requested_profile` and the validated
  provider ID returned by non-secret broker introspection.

These fields are metadata for policy and validation. Neither selects a real
credential. OPA may select a managed `credential_profile` only for the managed
path. A brokered allow with a non-null selected profile is rejected before
secret resolution. A non-null `broker_provider` also prevents the request from
inheriting a broad static host/method allow: one exact learned
Context/project/host/port/method/path rule is required before broker resolution.

## Ordinary streaming and declared GraphQL capture

For ordinary HTTP, Gateway evaluates OPA from request headers before forwarding
a body byte. Ordinary requests enable streaming only after principal validation,
credential preparation, policy allow, and any post-allow credential action
succeed. AWS signing requests remain unstreamed after allow until one complete
bounded body has been hashed and signed; the body itself is not policy input.
Responses stream after their headers arrive. Body presence and content do not
alter or split an ordinary exact Context/project/host/port/method/path
candidate or learned rule.

An exact GraphQL endpoint declaration changes that endpoint's protocol
contract. It never falls back to ordinary HTTP authorization. Gateway requires
one positive unambiguous `Content-Length` no greater than 1 MiB, buffers the
strict UTF-8 `application/json` POST object, selects one query or mutation,
and asks OPA about its sorted canonical root fields. Every root requires its
own exact learned rule. The original bytes are forwarded unchanged only after
the complete root set is allowed. GET, subscription, batch, multipart,
persisted-query-only, compressed, transfer-encoded, ambiguous, malformed, or
over-limit forms fail locally and are not learnable.

Mitmproxy retains a fixed 8 MiB `body_size_limit`. A message with a known
`Content-Length` above that value is rejected before the ordinary addon header
hook. An allowed unknown-length ordinary body has no total-byte limit in this
contract and is forwarded incrementally. AWS requires one unambiguous
`Content-Length` for non-GET/HEAD requests and rejects unknown-length,
transfer-encoded, `aws-chunked`, trailer, and over-limit forms before signing.

## Brokered-request ordering

A brokered request is recognized only from the validated provider projection and
an exact HTTPS authority/header/syntax match. A Tobari handle marker in a URL,
cookie, header name, unsupported header value, or ambiguous syntax is rejected
before OPA. A malformed, misplaced, ambiguous, or binding-mismatched marker
fails as `credential_handle_invalid`; it never falls back or reaches upstream.
Only complete absence of a Tobari marker from every inspected URL/header
position selects the configured passthrough or managed fallback. Gateway
removes valid placeholder credential fields before broker, policy, audit, or
upstream processing.

For a static schema-1 provider, Gateway sends one schema-1 `introspect` request
over `/run/tobari-auth/runtime/broker.sock`. It binds the handle, stable
Context/project/provider IDs, target, source header, and source format. The
response contains only the opaque revision and repeated normalized
target/source/destination/redaction metadata. Deny sends no `resolve`; allow
sends exactly one same-revision `resolve`, validates the full response, replaces
only the declared destination header, and makes one upstream attempt.

For the schema-2 AWS plan, Gateway accepts only an AWS4-HMAC-SHA256 header whose
credential and security-token placeholders contain the same project handle,
whose scope/date/signed-header structure is unambiguous, and whose target is
HTTPS 443 below commercial-partition `amazonaws.com` with a reviewed commercial
region. China, GovCloud, ISO, and sovereign partitions are excluded. It sends one
`introspect_signing` request containing the complete fixed plan and concrete
authority. Deny triggers no further broker call. After allow and complete body
capture, Gateway sends one `sign_sigv4` request containing the same handle,
revision, binding, host, method, path, query, region, service, selected
non-secret signed headers, and payload SHA-256. It sends neither body nor role
credential. Broker repeats binding/revision checks and, before host execution,
atomically persists the task digest in the encrypted AWS record as a durable
no-replay barrier. It then requests one typed post-policy companion export. The fixed
host AWS driver materializes opaque encrypted state
in a private temporary home and runs `aws configure export-credentials
--profile tobari --format process`. AWS CLI performs provider-native refresh
when its session remains renewable and returns updated opaque state with the
temporary tuple. Broker rechecks the record/revision, rejects a stale result,
signs locally, and atomically persists the refreshed state while clearing the
same task barrier. A crash, disconnect, malformed result, or post-execution
failure leaves the encrypted barrier in place across restart; handle issue and
signing perform no companion call until explicit AWS re-login or logout.
Gateway receives only the final authorization/date/session-token fields and
applies them atomically before one upstream attempt.

Static secrets and AWS role credentials are request-local and never enter
policy input, audit, denial output, logs, retry state, Workspace mounts, or the
provider projection. AWS role credentials are never persisted.

The request body is never a credential source or replacement surface and is
not scanned for handle-shaped bytes. A body containing a Workspace-readable
handle has no broker meaning and follows the ordinary body-streaming contract.

The broker protocol uses strict exact-key newline-delimited JSON frames of at
most 64 KiB and a finite Gateway timeout of 70 seconds by default, configurable
only from 70 through 90 seconds. The outer deadline deliberately exceeds the
companion's 60-second refresh hard bound plus its 5-second
cancellation-resolution window. Broker-created refreshes use a 45-second
default deadline so transport latency and bounded host/container clock offset
cannot cross that hard maximum. Known pre-execution unavailability is HTTP 503
`credential_broker_unavailable`. An explicit outcome-unknown result, a durable
barrier, or loss/invalidity of the Broker response after `sign_sigv4` send begins
is non-retryable HTTP 409 `credential_refresh_outcome_unknown`. Neither class
permits fallback with the same handle. The SDK must not automatically replay
that response. After the original request has settled, the user runs
`auth status`: `broker_state=ready` with AWS provider state `configured`
means no upstream attempt was made and an explicit retry of the user task is
safe; AWS provider state `not_configured` means the durable barrier is present
and requires AWS re-login or logout, followed by Workspace re-entry. An
unavailable or locked status must be reconciled before making either decision.

The companion opens no host/container listener. One fixed reverse
`docker exec -i` stream reaches an image-owned byte pump and unmounted
`/run/tobari-auth/companion/bridge.sock`. A fresh root-key-derived epoch,
challenge, direction-specific AES-GCM keys, exact monotonically increasing
sequence numbers, bounded frames/deadlines, and closed message schemas protect
the stream. Each complete encrypted-frame write has a two-second deadline. A
partial, timed-out, or failed write closes the whole session before any later
sequence number can be used. `refresh_lease`, cancellation, ping, and drain are outcomes, not
arbitrary argv. A replay, gap, invalid tag, unknown shape, duplicate session,
oversize, or disconnect closes the channel; an outcome that becomes unknown
after provider execution is not blindly replayed. `cancel_ack` acknowledges
receipt only; exactly one correlated `refresh_result` is terminal. If no such
result arrives within five seconds after cancellation, the outcome is unknown.

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
- Same-record AWS refresh lock wait: at most 1 second. Expiry is known
  pre-execution `credential_broker_unavailable`; no task barrier or companion
  call has occurred.
- Auth Broker runtime timeout: 70 seconds by default, configurable from 70 to
  90 seconds so it remains outside the companion's 60-second terminal refresh
  bound and 5-second cancellation-resolution window.
- Upstream connection/request timeout: 30 seconds by default, configurable up
  to 120 seconds.
- Maximum upstream attempt count: one.
- Gateway never retries because it cannot infer idempotency from an arbitrary
  HTTP request.
- Each proxy redirect request is independently normalized and authorized; a
  prior allow does not authorize the redirected authority or path.
- Cancellation closes the proxy flow and does not convert an unknown upstream
  mutation outcome into replay permission.

## Reviewed host credential drivers

`auth login github` is one supported provider-facing acquisition flow. A
trusted interactive host driver resolves a canonical GitHub CLI executable,
binds its SHA-256 identity, and runs these fixed operations in an ephemeral
configuration directory:

```text
gh auth login --hostname github.com --web --insecure-storage
gh auth status --active --hostname github.com --json hosts
gh auth token --hostname github.com
```

`GH_PROMPT_DISABLED=1`, `GH_BROWSER=/bin/true`, and `NO_COLOR=1` are fixed for
the ephemeral driver environment. Ambient browser selectors are removed. The
trusted host output boundary recognizes only the exact
`https://github.com/login/device` constant and invokes the platform opener once;
failure retains a manual fixed-URL instruction and does not fail acquisition.
Omitting `--git-protocol` is intentional: Auth Broker is acquiring GitHub API
authentication and must not ask about, configure, or require Git credential
handling. Visible output is bounded and projected without exposing captured
token or temporary configuration contents.

Ambient `GH_TOKEN`, `GITHUB_TOKEN`, enterprise-token, host, and repository
variables are removed. The login call owns the ordinary GitHub CLI web/device
interaction. Tobari neither interprets OAuth endpoints nor follows provider
responses itself. It validates exactly one active successful GitHub.com account
from bounded status JSON, captures one token of at most 32 KiB internally, and
removes the temporary directory even on failure. The sequence has no
pagination or automatic retry. It is bounded by the user's interactive
completion and command cancellation; cancellation or any failed capture leaves
the previous Context credential unchanged.

This driver does not refresh tokens, select among multiple Context accounts,
revoke a token remotely on logout, inspect scopes, call the GitHub REST API as
an application adapter, or interpret GitHub business operations. GitHub CLI is
a trusted-host prerequisite, not an Auth Broker image artifact. Its canonical
path and content digest are validated before and after the operation; the child
inherits no companion key or channel descriptor.

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

`auth login aws` is the second supported provider flow and has two explicit
closed methods. Omission means `identity-center`; Tobari never infers a method
from ambient state. Both resolve a canonical AWS CLI executable, bind its
SHA-256 identity, use one private `0700` temporary home and sanitized
environment, and delete it on every outcome.

Identity Center accepts only the validated start URL, SSO region, 12-digit
account ID, and role described in [Authentication handling](07_authentication.md),
renders one fixed `tobari` profile/session, and runs:

```text
aws sso login --profile tobari --use-device-code --no-browser --no-cli-pager
```

The fixed profile names, output, registration scope, and configuration keys are
driver-owned. The child inherits no ambient AWS configuration, credential,
proxy, loader, browser, companion key, or channel descriptor. Login output and
cache file count/name/size/JSON shape are bounded. After success the executable
digest is rechecked, cache bytes are canonically packed, and only opaque driver
state under `aws_cli_sso` is committed through Auth Broker. Request region is
deliberately absent; it comes from non-secret Context/tool configuration or an
explicit AWS CLI request option.

Console mode first runs fixed `aws --version` and requires 2.32 or newer. It
accepts one validated commercial region, pre-renders an otherwise empty
`tobari` profile, sets `AWS_LOGIN_CACHE_DIRECTORY` to the private home, passes
the trusted terminal as stdin, and runs:

```text
aws login --remote --profile tobari --region <validated-region> \
  --no-cli-pager --no-cli-auto-prompt
```

No callback listener or automatic browser opener is used. After success Tobari
accepts only a single validated `login_session` ARN whose 12-digit account
matches the stored secret-free label, the same region/output, and bounded
canonical SHA-256-named JSON cache files. That strict schema-2 state is bound
to `aws_cli_console_login`; companion execution rejects either driver ID with
the other state shape.

For post-policy refresh the resident companion reconstructs the same private
home and fixed executable identity from encrypted driver state and runs:

```text
aws configure export-credentials --profile tobari --format process \
  --no-cli-pager --cli-connect-timeout 10 --cli-read-timeout 30
```

The operation has a 45-second process bound and bounded stdout/stderr. Only
exact process-credential JSON with a future expiration is accepted. AWS CLI
owns Identity Center or console refresh and provider calls; Broker neither
reimplements AWS authentication endpoints nor stores temporary credentials. If
the overall renewable session has expired, export fails closed and recovery is
an explicit `auth login aws` using the intended method.

AWS signing implements only standard header-based SigV4. Canonicalization is
local to the broker, uses the complete body SHA-256 supplied by Gateway, adds
the session token and current UTC timestamp, and returns no secret key. SigV4a,
query presigning, streaming/chunked/EventStream signing, redirects, custom or
private endpoints, normalization-sensitive paths, and unsigned extra
`x-amz-*` headers fail closed. The supported algorithm follows the AWS
[Signature Version 4 process](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_sigv-create-signed-request.html).

## Gateway audit

Every validated ordinary allow/deny audit record uses `schema_version: 2` and contains
Context name and stable ID, project ID and safe root, cluster, request
authority, method, redacted path, decision, reason, adapter-dependent credential
profile name, upstream outcome, and timing. It contains no query or headers.
The ordinary value is only the URL path component; when that path contains a
Tobari handle marker, the whole value is `/[redacted-auth-handle]`. Structural
URL/header handle rejections are non-learnable and cannot become policy
candidates. A broker provider may be present only as secret-free authorization
metadata; a handle, credential revision, primary secret, request body, and raw
authorization value are excluded. Host-side
For GraphQL, audit schema 3 emits one record per canonical root with only
`protocol: graphql`, `graphql_operation_type`, and `graphql_root_field` added;
it never retains the document or variables. Host-side denial, candidate,
learned-rule, and compaction projections retain the same
Context/project pair so an opaque approval cannot lose its authority scope.

## Errors

Policy denial returns 403. A learnable response contains only the existing
fixed secret-free `tobari policy review` navigation; it has no candidate ID,
query, body, header, provider handle, credential, policy path, or dynamic
command argument. Non-learnable policy denial advertises no review command.

A copied, malformed, stale, revoked, ambiguous, or Context/project/provider/
target/header-mismatched or structurally misplaced handle returns HTTP 403 with
`credential_handle_invalid`. A locked or unavailable broker, socket timeout,
invalid response, inconsistent revision, companion disconnect/outcome
uncertainty, refresh/role/signing failure, or
malformed secret returns HTTP 503
with `credential_broker_unavailable`. Every Tobari-looking marker follows one of
those fail-closed paths unless it is a valid exact broker candidate; fallback
requires that no marker exists anywhere inspected. Gateway removes a valid
candidate before broker/OPA processing, so a failed candidate cannot reach OPA
logs, denial output, or upstream.
An unsupported or ambiguous AWS signing form returns HTTP 403
`broker_signing_request_invalid`; the placeholder authorization and session
token are removed before the response.
OPA unavailability or malformed decisions return 503 `policy_unavailable`.
An unsupported or malformed request at a declared GraphQL endpoint returns a
secret-free local 400 parser code and never reaches OPA or upstream.
Other Gateway normalization failures remain secret-free 4xx/5xx failures.

## Schema and compatibility

The OPA input schema version `5`, ordinary audit schema version `2`, GraphQL
audit schema version `3`, decision fields,
timeouts, attempt count, owner provider schema `1`, built-in/projection schema
`2`, broker control/runtime schema `1`, private companion epoch/frame schema
`1`, encrypted vault envelope schema `1` and payload schema `2`, handle prefix
`tobari-h1_`, and Gateway/Auth Broker image
API labels `3` for Gateway and `2` for Auth Broker are explicit pre-v1 compatibility boundaries. Valid schema-1 static
provider projections and vault payloads remain readable through their strict
compatibility/migration paths. Gateway does not accept former OPA input shapes,
incomplete decisions, or unknown broker frames.
Public auth backend values are exactly `macos_keychain|xdg_file`, and cluster
status may additionally report `unavailable`; the infrastructure/doctor label
`linux_xdg_file` is not a public JSON enum. The canonical schema, state-path,
socket, handle, and backend inventory is in
[Authentication handling](07_authentication.md#canonical-schemas-paths-and-backend-identifiers).

The principal registry remains schema version 2. Each Context policy data
source uses `tobari.schema_version=2`; the optional exact
`boundary.graphql_endpoints` array is additive, and absence retains legacy
ordinary HTTP behavior. Aggregate projection schema 1 loads one
current shared evaluator for every data-only Guided Context. Advanced Rego
source targets OPA input schema 4, accepts source schema 3 only for
compatibility, and rewrites either source version to runtime schema 5. It
stores Context data below `tobari_contexts[context_id]` and rejects other
shapes. Guided Contexts share one system evaluator. GraphQL input always routes
through that system evaluator, including for an Advanced Context, so older or
custom Rego cannot ignore the coordinate and authorize it as coarse HTTP.
Ordinary Advanced input is projected into a Context-ID package and cannot
claim router or system namespaces.

No live upstream provider response fixture is vendored. The repository-authored
synthetic provider manifest is pinned as `auth-provider.v1` in
`.harness/schemas.json` with MIT provenance and an exact digest. GitHub status
tests use synthetic JSON and mock host subprocess results. AWS tests use
synthetic credential-process JSON, opaque cache fixtures, fake companion
frames, and signing canaries. Live GitHub and AWS logins are
manual release evidence and may not be recorded with tokens, SSO state, role
credentials, signed headers, device codes, vaults, or raw authenticated output.

## Policy testing

Pinned OPA tests prove schema-5 rejection of older or incomplete input,
deny-by-default behavior, structured authority and port boundaries,
ordinary body-independent decisions, declared-endpoint no-fallback behavior,
all-root GraphQL authorization, Context/project-bound learned rules, null versus
broker-provider authorization metadata, and managed profile binding. Gateway
tests independently prove handle removal, deny-before-resolve/sign, exact
static replacement, two-stage same-revision AWS signing after allow and
complete-body hashing, zero companion calls on deny, one bounded host export on
allow, stale refresh rejection, and fallback compatibility. Companion tests
prove authenticated direction/sequence/replay/frame contracts and no listener
or host socket mount.
Fallback tests require marker absence in every inspected URL/header position;
audit tests require query/header omission, whole-path marker redaction, and
non-learnable structural handle rejection.
