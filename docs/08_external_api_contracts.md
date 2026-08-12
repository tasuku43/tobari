# External API Contracts

Tobari exposes no provider-specific business-operation API. It authorizes the
ordinary HTTP/HTTPS effect that leaves a Workspace through Gateway. The sole
reviewed brokered acquisition helper is GitHub CLI for one static GitHub.com
credential; owner manifests may import another static primary secret when the
same exact HTTPS/header transformation can express the outcome.

## Generic HTTP contract

Gateway derives trusted Context/project identity from the kernel-observed
Workspace source endpoint and owner-only principal registry. It normalizes
scheme, host, port, method, raw path, query, and redacted headers for OPA.
Query and headers can be Advanced-Rego constraints but are not guided learned-
permission identity. Ordinary bodies are payload and stream only after allow.

A declared exact GraphQL endpoint is the bounded exception: Gateway accepts one
unambiguous positive-length JSON body of at most 1 MiB, derives only operation
type and canonical root fields, asks OPA for every root coordinate, and
forwards the original bytes once after allow. Source, operation name, variables,
arguments, aliases, fragments, directives, nested selections, literals, and
body hashes never enter policy, evidence, audit, or CLI output.

Gateway performs no external DNS or upstream connection before allow. It uses
finite OPA, Broker, DNS, connect, and upstream timeouts and makes one upstream
attempt. It does not retry an arbitrary HTTP request.

## Policy-preset ceiling

The immutable preset guardrail is evaluated before baseline data, learned
exact policy, or Advanced Rego. `builtin/offline` terminally denies all
HTTP/HTTPS; `builtin/reviewed-exact` makes only eligible effects exact-review
candidates; `builtin/get-only-reviewed` makes only eligible GET effects
candidates and terminally denies HEAD and every non-GET. None grants immediate
authority and GET is not classified as safe or read-only. Terminal denial
creates no candidate and causes zero external DNS, Broker resolution, or
upstream calls.

## Static broker contract

Provider schema 1 is strict non-secret, non-executable data. It declares a
bounded Workspace handle projection and exact HTTPS target, source header,
source format, and destination header. Owner files cannot select helpers,
dynamic records, OAuth, refresh, signing, supplemental headers, arbitrary
methods/routes, policy, shell, executable paths, remote fetch, or provider
operations. Overlapping recognition coordinates reject the complete
projection.

Gateway follows one sequence:

1. Reject malformed, misplaced, ambiguous, stale, copied, or binding-mismatched
   Tobari-looking handle markers.
2. Remove one recognized handle and request non-secret Broker introspection of
   Context, project, provider, revision, target, source header, and format.
3. Send only normalized request identity and non-secret provider identity to
   OPA.
4. On deny, stop with zero resolution or upstream call.
5. On allow, resolve the same revision once, replace only the declared header,
   and make one upstream attempt.

Passthrough applies only when no Tobari-looking marker exists. No malformed or
stale handle is forwarded or accepted by fallback. Secret values, raw handles,
credential revisions, queries, headers, and bodies are absent from OPA audit
and denial output.

First public V1 contains no managed adapter/profile, AWS SigV4, Datadog OAuth,
OpenAI/Codex OAuth, Anthropic setup-token plan, Chatwork built-in, refresh,
task barrier, signer, supplemental header, credential companion, or exact-
client-version driver. Tool-native use of those services remains Workspace-
owned authentication and still requires ordinary Gateway/OPA authorization.

## GitHub acquisition

`auth login --provider github` resolves one canonical non-project GitHub CLI,
uses fixed API-only argv and sanitized environment, runs in a private temporary
home, recognizes only `https://github.com/login/device`, retains manual browser
fallback, captures bounded token/status output, rechecks the executable digest,
performs checked cleanup, and only then commits the static secret. It requests
no Git protocol or credential-helper setup and reads no ambient GitHub home.
Exact GitHub CLI product-version equality is not an authority boundary.

`auth import PROVIDER` reads one bounded secret from non-terminal stdin after
public validation and before one Broker send. Terminal input is rejected before
reading. Login/import rotate the record and all handles; logout removes local
state and revokes handles without claiming provider-side revocation.

## Faults and evidence

OPA or Gateway uncertainty denies. Invalid handles return
`credential_handle_invalid`; locked or unavailable Broker state returns
`credential_broker_unavailable`. Neither permits fallback. Auth mutation
uncertainty uses `auth status` reconciliation before another mutation.

Automated tests use synthetic secrets, fake GitHub CLI results, local HTTP
servers, fixed clocks, and canaries. Live GitHub acquisition is manual release
evidence and records pass/fail only; no credential, code, handle, vault, account
identifier, authenticated response, or raw transcript may become a fixture.
