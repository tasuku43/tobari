# External API Contracts

Tobari exposes no provider-specific business-operation API. It authorizes the
ordinary HTTP/HTTPS effect that leaves a Workspace through Gateway. The closed
reviewed brokered plans cover GitHub, AWS, Datadog, OpenAI/Codex,
Anthropic/Claude, and Chatwork; owner manifests may add only a static primary
secret when an exact HTTPS/header transformation expresses the outcome.

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

## Broker contract

Provider schema 1 is strict non-secret, non-executable data. It declares a
bounded Workspace handle projection and exact HTTPS target, source header,
source format, and destination header. Owner files cannot select helpers,
dynamic records, OAuth, refresh, signing, supplemental headers, arbitrary
methods/routes, policy, shell, executable paths, remote fetch, or provider
operations. Overlapping recognition coordinates reject the complete
projection.

Gateway follows one sequence:

1. Match the request against the host-owned declared provider bindings. Reject
   a real Workspace credential at a declared header or AWS signing binding as
   non-learnable `broker_auth_required` before OPA or external I/O.
2. Reject malformed, misplaced, ambiguous, stale, copied, or binding-mismatched
   Tobari-looking handle markers.
3. Remove one recognized handle and request non-secret Broker introspection of
   Context, project, provider, revision, target, source header, and format.
4. Only for an undeclared binding with no marker, select Workspace-owned
   compatibility passthrough.
5. Send only normalized request identity and non-secret provider identity to
   OPA.
6. On deny, stop with zero resolution, refresh, companion call, signing, or
   upstream call.
7. On static allow, resolve the same revision once and replace only the
   declared header.
8. On Datadog/OpenAI/Anthropic allow, select or refresh the same record once and apply
   only the reviewed bearer/supplemental-header result.
9. On AWS allow, retain at most 8 MiB, obtain one private companion export,
   sign that exact authorized request locally, and apply only those headers.
10. Make one upstream attempt without application replay.

Compatibility passthrough applies only when no declared binding and no
Tobari-looking marker exists. No malformed or stale handle is forwarded or
accepted by fallback. Secret values, raw handles, credential revisions,
queries, headers, and bodies are absent from OPA audit and denial output.

Managed adapters/profiles remain absent. Dynamic records, refresh, task
barriers, signing, supplemental headers, the credential companion, and exact-
version drivers exist only inside the closed reviewed built-in plans. Owner
manifests cannot select or extend them.

## GitHub acquisition

`auth login --provider github` resolves one canonical non-project GitHub CLI,
uses fixed API-only argv and sanitized environment, runs in a private temporary
home, recognizes only `https://github.com/login/device`, retains manual browser
fallback, captures bounded token/status output, rechecks the executable digest,
performs checked cleanup, and only then commits the static secret. It requests
no Git protocol or credential-helper setup and reads no ambient GitHub home.
Exact GitHub CLI product-version equality is not an authority boundary.

## Other reviewed acquisition and post-policy plans

The shared acquisition resolver ignores untrusted PATH shadows without
executing them and selects the first finite PATH candidate whose canonical
executable passes the existing conventional-root and mode contract.
AWS offers only `identity-center` and `console` host acquisition. Its opaque
state is re-entered by a private authenticated companion after allow, and
Broker emits one standard header-based SigV4 result for the exact bounded
request. Datadog uses fixed US1 pup acquisition and a proxy-free,
no-redirect, same-record refresh transport. OpenAI accepts a stable observed
host Codex version only when the exact compiled V1 native-browser/state
contract succeeds. The verified Codex child owns the loopback listener,
dynamic authorization URL, browser request, PKCE state, callback, and exchange;
Tobari never binds, parses, or opens them. Tobari regenerates only the reviewed
Codex reset, muted, and accent SGR vocabulary; `NO_COLOR` strips those styles
and unknown controls remain visibly projected. OpenAI
refreshes only the same ChatGPT account record, returning
its validated account ID for one supplemental header. Anthropic accepts only
the four required renewable-session values extracted from exact Claude Code
2.1.220 in the selected Context image, structurally validates the dynamic
requested and granted scope sets, rejects grants outside the observed request,
and canonicalizes their order without compiling provider scope names, discards other native-file metadata,
stores a strict Tobari-owned record, selects an unexpired bearer or refreshes the same record through
the fixed proxy-free, no-redirect platform endpoint without scope drift, and
exposes only a project-bound handle plus the same non-secret scope set in the
Workspace credential file.
Chatwork is a static stdin-import binding. The OpenAI Broker client ID and
refresh endpoint and the Workspace Codex handle projection remain separately
fixed; host product version does not select either contract.

Any dispatched AWS companion or Datadog refresh whose result becomes unknown
returns `credential_refresh_outcome_unknown` (`409`), records a durable task
barrier when required, and makes no application upstream attempt. Automatic
replay is forbidden; trusted-host `auth status` is the reconciliation route.

`auth import PROVIDER` reads one bounded secret from non-terminal stdin after
public validation and before one Broker send. Terminal input is rejected before
reading. Login/import rotate the record and all handles; logout removes local
state and revokes handles without claiming provider-side revocation.

## Faults and evidence

OPA or Gateway uncertainty denies. Invalid handles return
`credential_handle_invalid`; locked or unavailable Broker state returns
`credential_broker_unavailable`; a real credential at a declared binding
returns `broker_auth_required`. None permits fallback. Auth mutation
uncertainty uses `auth status` reconciliation before another mutation.

Automated tests use synthetic secrets and provider state, fake fixed-driver
results, local HTTP servers, fixed clocks, and canaries. Live reviewed-provider
acquisition is manual release evidence and records pass/fail only; no
credential, code, handle, vault, account identifier, authenticated response,
or raw transcript may become a fixture.
