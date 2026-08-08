# External HTTP Contract

Tobari exposes no provider-specific API adapter. Its external contract is the
generic HTTP request crossing Gateway.

## OPA input

Gateway posts one JSON document to
`http://opa:8181/v1/data/tobari/http/decision` with schema version `4`. The
document groups each semantic responsibility instead of exposing parallel
transport fields:

```json
{
  "schema_version": 4,
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
  "authorization": {"requested_profile": null}
}
```

`principal.context_id` and `principal.project_id` are required canonical UUIDv7
values established together from the Gateway local interface and host-owned
principal registry; neither is copied from a caller header, environment,
request URL, session field, or profile name. OPA uses the Context ID at this one
fixed endpoint to select the Context policy. Unknown, absent, or mismatched IDs
deny without falling back to the current/default Context.
The default passthrough adapter emits a null requested profile; the managed
adapter may select one after its own Context/host/project binding checks.

Header names are lowercase. Host excludes a trailing dot and uses the
normalized request authority. Query values retain occurrence order. Request
and response body bytes and derived metadata, such as decoded values or hashes,
are outside this contract. Media-type and framing headers remain ordinary
redacted header fields, but are not learned-rule identity dimensions.

## Body-independent authorization and streaming

Gateway evaluates OPA from request headers before any body byte is forwarded.
It enables mitmproxy request streaming only after the Context/project principal,
credential binding, policy decision, and credential application succeed. It
streams the corresponding upstream response after its headers arrive. Body
presence and content therefore do not alter or split the exact
Context/project/host/port/method/path candidate or learned rule, and body data is never
copied into OPA input, denial evidence, policy data, or audit output.

Mitmproxy retains a fixed 8 MiB `body_size_limit`. A message with a known
`Content-Length` above that value is rejected before the ordinary addon header
hook. An allowed unknown-length body, such as a chunked upload or streaming
response, has no total-byte limit in this contract and is forwarded
incrementally rather than buffered in full.

## Decision

OPA must return exactly one object:

```json
{
  "allow": true,
  "reason": "allowed by policy",
  "credential_profile": null,
  "status_code": 403,
  "learnable": false
}
```

`status_code` is required and restricted to 403 in the MVP. `learnable` is
required and may be true only on a denial after version, cluster, scheme, and
fixed request port pass; managed-adapter requests additionally require
credential binding,
meaning an exact Context/project/host/port/method/path rule can resolve that denial. The
initialized policy requires the configured port for the request scheme, and
learned rules retain the observed Context, project, host, port, method, and path. The
scheme/port boundary is evaluated before a learned rule can match, so an
approval cannot move an effect outside the configured transport boundary.
Gateway records the value for candidate discovery but never treats it as
authorization. Missing or wrong-typed required fields deny. `allow` is the
only authorization fact; no `ask` or pending state exists. The decision has no
audit member: Gateway owns audit emission and OPA returns authorization data
only.

## Timeouts, attempts, and retry

- OPA decision timeout: 2 seconds by default, configurable up to 10 seconds.
- Upstream connection/request timeout: 30 seconds by default, configurable up
  to 120 seconds.
- Maximum attempt count: one.
- Gateway never retries because it cannot infer idempotency from an arbitrary
  HTTP request.
- Cancellation closes the proxy flow; it does not turn an unknown upstream
  mutation outcome into permission to replay.

## Gateway audit

Every validated allow/deny audit record uses `schema_version: 2` and contains `context_id`, the human
`context` name, `project_id`, and safe `project_root` alongside the cluster,
request authority, method, path, decision, reason, adapter-dependent
credential profile name, and upstream outcome. In passthrough mode the profile
name is `null`. Context and project IDs are metadata for host-side policy
learning and diagnostics; secret values and request bodies remain excluded.
Host-side denial, candidate, learned-rule, and compaction projections retain
the same Context/project pair so an opaque approval cannot lose its authority scope.

## Errors

Policy denial returns 403. When OPA marks the denial as learnable, the response
also carries a fixed, secret-free `tobari` navigation object so an agent can
tell the user to review the permission on the trusted host:

```json
{
  "error": "policy_denied",
  "message": "Tobari blocked this network request because it is outside the current execution boundary. Leave the Workspace with `exit`, then run `tobari policy review` on the trusted host.",
  "tobari": {
    "schema_version": 1,
    "event": "permission_review_available",
    "run_on": "host",
    "review": {
      "available": true,
      "command": "tobari policy review",
      "automatic_retry": false,
      "retry_after_review": true
    },
    "request": {
      "host": "api.example.com",
      "port": 443,
      "method": "POST",
      "path": "/token"
    }
  }
}
```

The response contains no query, body, headers, credentials, policy path, or
opaque action ID. The command is fixed catalog language and is advisory only;
the host-side retained denial queue remains the source of truth. The learnable
message may remind the agent to leave the Workspace before asking the host to
run the review command. A
non-learnable policy denial uses `event=permission_review_unavailable`,
`review.available=false`, and a null command so it cannot be mistaken for a
safe exact approval candidate. OPA unavailability or malformed decisions
return 503 with a generic message. Gateway internal normalization failure
returns 502. Responses contain no OPA body, upstream error body, secret value,
or raw request body. Audit records distinguish deny, policy unavailable, and
gateway error without exposing confidential content.

## Schema and compatibility

The OPA input schema version `4`, audit schema version `2`, decision fields, default limits,
and configuration keys are public MVP compatibility boundaries. The Gateway
does not accept the former OPA input shapes or incomplete decisions. The
trusted
`TOBARI_CREDENTIAL_ADAPTER` setting defaults to `passthrough`; `managed` keeps
the existing credential configuration contract. The
`principal-registry/principals.json` uses schema version 2 with `bindings`
containing `project_id`, `context_id`, `context`, `project_root`, `gateway_ip`,
and `network`. Gateway credential projection schema v2 contains `contexts`
keyed by Context ID; each projected profile still requires a `projects` array
and its secret path is below that Context ID. Context source credentials remain
schema v1.
Each Context policy source uses `tobari.schema_version=2`, with `boundary`,
`credentials`, and `rules` objects; mutable rules live under
`rules.learned_allows` and `rules.learned_denies`, while host-authored baseline
denies live under `rules.baseline_denies`. Aggregate policy projection schema 1
places Context data under `tobari_contexts[context_id]`, supplies one
Tobari-owned router, and content-addresses the complete candidate. Guided
Contexts share one system evaluator; Advanced source is projected to a
Context-ID package and cannot claim system/router namespaces.
Synthetic fixtures
live with Gateway tests and are generated by this repository; no upstream
provider schema is vendored, so `.harness/schemas.json` remains empty.

## Policy testing

Rego is formatted with the pinned OPA image and tested by `opa test`. The
initialized policy proves deny by default, structured authority and port
boundaries,
plain-HTTP restriction, host/port/method/path denial, body-independent
decisions, Context/project-bound learned rules, passthrough
redaction/forwarding, unknown-Context denial, and managed credential-profile
Context/host/project binding.
