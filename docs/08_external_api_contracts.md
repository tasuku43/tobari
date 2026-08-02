# External HTTP Contract

Tobari exposes no provider-specific API adapter. Its external contract is the
generic HTTP request crossing Gateway.

## OPA input

Gateway posts one JSON document to
`http://opa:8181/v1/data/tobari/http/decision` with schema version `v1`.
The request carries normalized scheme, host, port, method, path, path segments,
multi-valued query, redacted headers, bounded body metadata, the host-issued
project principal, cluster, optional caller session metadata, and an
adapter-dependent optional requested credential profile. In the default
passthrough adapter it is always `null`; the managed adapter may use it.
`principal.project_id` is required to be a
canonical UUIDv7 established from the Gateway local interface and the
host-owned principal registry; it is not copied from a caller header.

Header names are lowercase. Host excludes a trailing dot and uses the
normalized request authority. Query values retain occurrence order. The body
hash is over the exact bytes forwarded upstream.

## Body bounds

The default inspection maximum is 1 MiB and the supported configurable range is
1 KiB through 8 MiB. Gateway's structured inspection is limited to the
configured size. A body above the maximum is never decoded into a JSON value and
is marked truncated. Non-JSON bodies expose metadata only. An explicitly empty
captured body is `kind=metadata`, `size=0`, and `truncated=false`; if mitmproxy
reports that the body was not captured, Gateway emits `kind=unavailable` with
unknown size/truncation/hash and denies before OPA. Gateway forwards the same
captured bytes; policy is never evaluated over one body while another is sent.
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
  "learnable": false,
  "audit": {"level": "metadata"}
}
```

Denied decisions may include `status_code`, restricted to 403 in the MVP.
`learnable` is an optional boolean for compatibility and defaults to false;
the initialized policy always emits it. It may be true only on a denial after
version, cluster, scheme, fixed request port, empty-body boundary, and captured
body pass; managed-adapter requests additionally require credential binding,
meaning an exact host/port/method/path rule can resolve that denial. The
initialized policy requires the configured port for the request scheme, and
learned rules retain the observed port and scheme so an approval cannot move an
effect to a different authority. Gateway records the value for
candidate discovery but never treats it as authorization. Unknown keys are
tolerated for forward compatibility, but missing or wrong-typed required
fields deny. `allow` is the only authorization fact; no `ask` or pending state
exists.

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
  "message": "Tobari blocked this network request because it is outside the current execution boundary.",
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
the host-side retained denial queue remains the source of truth. A
non-learnable policy denial uses `event=permission_review_unavailable`,
`review.available=false`, and a null command so it cannot be mistaken for a
safe exact approval candidate. OPA unavailability or malformed decisions
return 503 with a generic message. Gateway internal normalization failure
returns 502. Responses contain no OPA body, upstream error body, secret value,
or raw request body. Audit records distinguish deny, policy unavailable, and
gateway error without exposing confidential content.

## Schema and compatibility

The OPA input version, decision fields, audit fields, default limits, and
configuration keys are public MVP compatibility boundaries. The trusted
`TOBARI_CREDENTIAL_ADAPTER` setting defaults to `passthrough`; `managed` keeps
the existing credential configuration contract. The
`principal-registry/principals.json` file uses schema version 1 with `bindings`
containing `project_id`, `gateway_ip`, and `network`;
credential profile schema-v1 entries require a `projects` array.
Synthetic fixtures
live with Gateway tests and are generated by this repository; no upstream
provider schema is vendored, so `.harness/schemas.json` remains empty.

## Policy testing

Rego is formatted with the pinned OPA image and tested by `opa test`. The
initialized policy proves deny by default, allowed hosts and ports,
plain-HTTP restriction, host/port/method/path denial, explicit-empty versus
unavailable-body restriction, project-bound learned rules, passthrough
redaction/forwarding, and managed credential-profile host/project binding.
