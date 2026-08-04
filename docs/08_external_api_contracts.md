# External HTTP Contract

Tobari exposes no provider-specific API adapter. Its external contract is the
generic HTTP request crossing Gateway.

## OPA input

Gateway posts one JSON document to
`http://opa:8181/v1/data/tobari/http/decision` with schema version `2`. The
document groups each semantic responsibility instead of exposing parallel
transport fields:

```json
{
  "schema_version": 2,
  "principal": {
    "cluster": "default",
    "project_id": "01912345-6789-7abc-8def-0123456789ab"
  },
  "request": {
    "authority": {"scheme": "https", "host": "api.github.com", "port": 443},
    "method": "GET",
    "path": {"raw": "/user", "segments": ["user"]},
    "query": {"key": ["value"]},
    "headers": {"content-type": "application/json"},
    "body": {
      "state": "empty",
      "size": 0,
      "truncated": false,
      "sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
      "content_type": ""
    }
  },
  "authorization": {"requested_profile": null}
}
```

`principal.project_id` is required to be a
canonical UUIDv7 established from the Gateway local interface and the
host-owned principal registry; it is not copied from a caller header.
The default passthrough adapter emits a null requested profile; the managed
adapter may select one after its own host/project binding checks.

Header names are lowercase. Host excludes a trailing dot and uses the
normalized request authority. Query values retain occurrence order. The body
hash is over the exact bytes forwarded upstream.

## Body bounds

The default inspection maximum is 1 MiB and the supported configurable range is
1 KiB through 8 MiB. Gateway's structured inspection is limited to the
configured size. A body above the maximum is never decoded into a JSON value and
is marked truncated. Non-JSON bodies expose metadata only. An explicitly empty
captured body is `state=empty`, `size=0`, and `truncated=false`; a complete
non-empty JSON body is `state=json` and may include a bounded `value`; a
non-empty body that is not decoded is `state=metadata`, and malformed JSON is
`state=invalid_json`. If mitmproxy reports that the body was not captured,
Gateway emits `state=unavailable` with unknown size/truncation/hash and denies
before OPA. Gateway forwards the same captured bytes; policy is never evaluated
over one body while another is sent.
Mitmproxy rejects request and response bodies above the fixed 8 MiB transport
cap before the Gateway addon hook. This bounds one buffered body; a total
concurrent request-body memory limit remains a known MVP gap.

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
required and may be true only on a denial after
version, cluster, scheme, fixed request port, empty-body boundary, and captured
body pass; managed-adapter requests additionally require credential binding,
meaning an exact host/port/method/path rule can resolve that denial. The
initialized policy requires the configured port for the request scheme, and
learned rules retain the observed project, host, port, method, and path. The
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

Every validated allow/deny audit record contains `project_id` alongside the
cluster, request authority, method, path, decision, reason, adapter-dependent
credential profile name, and upstream outcome. In passthrough mode the profile
name is `null`. The project ID is metadata for host-side policy
learning and diagnostics; secret values and request bodies remain excluded.
Host-side denial, candidate, learned-rule, and compaction projections retain
the same project ID so an opaque approval cannot lose its authority scope.

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

The OPA input schema version `2`, decision fields, audit fields, default limits,
and configuration keys are public MVP compatibility boundaries. The Gateway
does not accept the former OPA input shape or incomplete v2 decisions. The
trusted
`TOBARI_CREDENTIAL_ADAPTER` setting defaults to `passthrough`; `managed` keeps
the existing credential configuration contract. The
`principal-registry/principals.json` file uses schema version 1 with `bindings`
containing `project_id`, `gateway_ip`, and `network`;
credential profile schema-v1 entries require a `projects` array.
The active policy data uses `tobari.schema_version=2`, with `boundary`,
`credentials`, and `rules` objects; mutable rules live under
`rules.learned_allows` and `rules.learned_denies`, while host-authored baseline
denies live under `rules.baseline_denies`.
Synthetic fixtures
live with Gateway tests and are generated by this repository; no upstream
provider schema is vendored, so `.harness/schemas.json` remains empty.

## Policy testing

Rego is formatted with the pinned OPA image and tested by `opa test`. The
initialized policy proves deny by default, structured authority and port
boundaries,
plain-HTTP restriction, host/port/method/path denial, explicit-empty versus
unavailable-body restriction, project-bound learned rules, passthrough
redaction/forwarding, and managed credential-profile host/project binding.
