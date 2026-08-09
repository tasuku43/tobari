export type SequenceTone =
  "control" | "network" | "allowed" | "denied" | "secret" | "diagnostic";

export interface SequenceStep {
  title: string;
  from: string;
  to: string;
  sent: string;
  withheld: string;
  owner: string;
  failure: string;
  explanation: string;
  tone: SequenceTone;
}

export interface SequenceScenario {
  id: string;
  label: string;
  summary: string;
  actors: string[];
  steps: SequenceStep[];
}

export const sequenceScenarios: SequenceScenario[] = [
  {
    id: "allowed-passthrough",
    label: "Allowed passthrough request",
    summary:
      "A normal HTTP effect is identified, authorized once, and then streamed to the upstream service.",
    actors: [
      "Workspace process",
      "Gateway",
      "OPA",
      "DNS / resolver",
      "Upstream",
    ],
    steps: [
      {
        title: "Proxy request arrives",
        from: "Workspace process",
        to: "Gateway",
        sent: "HTTP method, URL, headers, and a streaming body",
        withheld: "No trusted project identifier is accepted from the request",
        owner: "Gateway",
        failure: "The request stops before policy or upstream work.",
        explanation:
          "The Workspace can reach only its dedicated internal network. Its configured HTTP proxy is the Gateway.",
        tone: "network",
      },
      {
        title: "Principal is established",
        from: "Gateway network interface",
        to: "Gateway",
        sent: "Host-owned Context ID and project ID from the principal registry",
        withheld: "Workspace-supplied identity headers",
        owner: "Gateway",
        failure: "An unknown or ambiguous interface fails closed.",
        explanation:
          "The receiving Gateway interface, not a claim in the request, selects the principal.",
        tone: "control",
      },
      {
        title: "Decision input is normalized",
        from: "Gateway",
        to: "OPA",
        sent: "Principal, scheme, host, port, method, normalized path, and non-secret headers",
        withheld: "Request body and credential headers",
        owner: "Gateway",
        failure: "Malformed input is rejected; nothing reaches upstream.",
        explanation:
          "The request body is never a policy-rule identity dimension.",
        tone: "control",
      },
      {
        title: "One policy decision",
        from: "OPA",
        to: "Gateway",
        sent: "A structured allow decision",
        withheld: "Credentials and request body",
        owner: "OPA",
        failure:
          "Deny, malformed output, timeout, or unavailability closes the path.",
        explanation:
          "OPA decides whether the ordinary HTTP effect is allowed. Gateway enforces that decision.",
        tone: "allowed",
      },
      {
        title: "Destination is resolved",
        from: "Gateway",
        to: "DNS / resolver",
        sent: "Allowed destination hostname",
        withheld: "Workspace network access and credentials",
        owner: "Gateway",
        failure:
          "Resolution or destination validation failure returns an error.",
        explanation:
          "Resolution happens only after authorization and remains Gateway-owned.",
        tone: "network",
      },
      {
        title: "Separate upstream connection",
        from: "Gateway",
        to: "Upstream",
        sent: "The authorized request; body is streamed",
        withheld: "Policy internals and trusted principal registry",
        owner: "Gateway",
        failure: "Connection failure is reported without changing policy.",
        explanation:
          "Gateway opens the egress connection. The Workspace never receives direct egress.",
        tone: "allowed",
      },
      {
        title: "Secret-free audit event",
        from: "Gateway",
        to: "Host diagnostics",
        sent: "Decision dimensions, outcome, and bounded diagnostic metadata",
        withheld: "Body and secret headers",
        owner: "Gateway",
        failure:
          "Diagnostics do not turn a denied request into an allowed one.",
        explanation:
          "Audit records explain the effect without copying payloads or credentials.",
        tone: "diagnostic",
      },
    ],
  },
  {
    id: "learnable-denial",
    label: "Learnable policy denial",
    summary:
      "A request outside the active exact rules is denied before any upstream connection.",
    actors: ["Workspace process", "Gateway", "OPA", "Review store", "Upstream"],
    steps: [
      {
        title: "Undeclared effect arrives",
        from: "Workspace process",
        to: "Gateway",
        sent: "Requested host, port, method, and path",
        withheld: "Any authority to approve itself",
        owner: "Gateway",
        failure: "Malformed proxy traffic is rejected.",
        explanation:
          "The request itself is untrusted data, not a request for automatic permission.",
        tone: "network",
      },
      {
        title: "Body-free policy input",
        from: "Gateway",
        to: "OPA",
        sent: "Trusted principal and normalized decision dimensions",
        withheld: "Body and credential headers",
        owner: "Gateway",
        failure: "Normalization failure stops processing.",
        explanation:
          "Two requests differing only in body are the same policy candidate.",
        tone: "control",
      },
      {
        title: "Default deny",
        from: "OPA",
        to: "Gateway",
        sent: "Deny plus learnable classification",
        withheld: "No policy edit or inferred wildcard",
        owner: "OPA",
        failure:
          "Unexpected policy output is treated as unavailable, never allow.",
        explanation:
          "No active exact rule matches, so default deny remains in force.",
        tone: "denied",
      },
      {
        title: "Evidence retained",
        from: "Gateway",
        to: "Review store",
        sent: "Secret-free exact effect in a bounded denial record",
        withheld:
          "Request body, secret headers, candidate ID, and authority from display order",
        owner: "Gateway",
        failure: "If evidence cannot be retained, the request is still denied.",
        explanation:
          "A trusted host CLI later validates this evidence and derives the opaque action reference.",
        tone: "diagnostic",
      },
      {
        title: "No upstream connection",
        from: "Gateway",
        to: "Workspace process",
        sent: "403 and the host-side review location",
        withheld: "No retry and no automatic approval",
        owner: "Gateway",
        failure: "The effect remains denied.",
        explanation:
          "The Workspace learns where the operator can review; it cannot approve the request.",
        tone: "denied",
      },
    ],
  },
  {
    id: "opa-unavailable",
    label: "OPA unavailable",
    summary:
      "Authorization infrastructure failure is a closed path, not permission.",
    actors: ["Workspace process", "Gateway", "OPA", "Upstream"],
    steps: [
      {
        title: "Request reaches Gateway",
        from: "Workspace process",
        to: "Gateway",
        sent: "HTTP effect",
        withheld: "Direct egress",
        owner: "Gateway",
        failure: "The request ends at Gateway if processing cannot continue.",
        explanation: "The network topology still prevents a direct route.",
        tone: "network",
      },
      {
        title: "Policy query fails",
        from: "Gateway",
        to: "OPA",
        sent: "Secret-free normalized input",
        withheld: "Body and credentials",
        owner: "OPA",
        failure:
          "Timeout, connection failure, or malformed result is policy_unavailable.",
        explanation: "Gateway requires one valid structured decision.",
        tone: "denied",
      },
      {
        title: "Gateway fails closed",
        from: "Gateway",
        to: "Workspace process",
        sent: "503 policy_unavailable",
        withheld: "No upstream DNS lookup or connection",
        owner: "Gateway",
        failure:
          "The caller may diagnose OPA, but the effect is not authorized.",
        explanation:
          "Unavailable is distinct from a reviewable default denial.",
        tone: "denied",
      },
      {
        title: "Upstream remains untouched",
        from: "Gateway",
        to: "Upstream",
        sent: "Nothing",
        withheld: "The entire request",
        owner: "Gateway",
        failure: "No side effect is initiated.",
        explanation: "A broken policy path never fails open.",
        tone: "denied",
      },
    ],
  },
  {
    id: "broker-allowed",
    label: "Brokered credential allowed",
    summary:
      "The secret is resolved only after the ordinary HTTP effect is allowed.",
    actors: ["Workspace process", "Gateway", "Auth Broker", "OPA", "Upstream"],
    steps: [
      {
        title: "Handle is presented",
        from: "Workspace process",
        to: "Gateway",
        sent: "Opaque project-bound handle in the declared source header",
        withheld: "Primary credential",
        owner: "Gateway",
        failure: "Ambiguous or malformed marker fails closed.",
        explanation:
          "The handle names a constrained broker record; it is not the credential.",
        tone: "network",
      },
      {
        title: "Handle is removed",
        from: "Gateway",
        to: "Gateway",
        sent: "Handle moves only into internal broker processing",
        withheld: "Handle is not forwarded upstream or sent to OPA",
        owner: "Gateway",
        failure:
          "A Tobari-looking invalid handle does not fall back to passthrough.",
        explanation:
          "Gateway strips the recognized source before policy evaluation.",
        tone: "control",
      },
      {
        title: "Non-secret introspection",
        from: "Gateway",
        to: "Auth Broker",
        sent: "Handle plus trusted Context/project identity",
        withheld: "Primary credential",
        owner: "Auth Broker",
        failure:
          "Wrong Context, project, provider, revision, target, or binding rejects the request.",
        explanation:
          "Introspection proves the record binding without disclosing the secret.",
        tone: "control",
      },
      {
        title: "Ordinary effect is decided",
        from: "Gateway",
        to: "OPA",
        sent: "HTTP dimensions plus non-secret authorization metadata",
        withheld: "Handle, body, and primary credential",
        owner: "OPA",
        failure: "Deny means no secret resolution.",
        explanation:
          "Login does not add an allow rule. OPA still evaluates host, port, method, and path.",
        tone: "allowed",
      },
      {
        title: "Resolve exactly once",
        from: "Gateway",
        to: "Auth Broker",
        sent: "Allowed record and exact HTTP/header binding",
        withheld: "Secret is never returned to the Workspace",
        owner: "Auth Broker",
        failure: "Locked, stale, or inconsistent state fails closed.",
        explanation:
          "Resolution is post-policy and single-use for this request.",
        tone: "secret",
      },
      {
        title: "Declared header replaced",
        from: "Gateway",
        to: "Upstream",
        sent: "Primary credential only in the manifest-declared destination header",
        withheld: "Handle and broker internals",
        owner: "Gateway",
        failure: "No alternate header or target is attempted.",
        explanation:
          "Gateway connects to the exact HTTPS target and streams the authorized request.",
        tone: "allowed",
      },
    ],
  },
  {
    id: "invalid-handle",
    label: "Invalid or stale broker handle",
    summary:
      "A copied, old, malformed, or mismatched Tobari handle cannot become passthrough traffic.",
    actors: ["Workspace process", "Gateway", "Auth Broker", "OPA", "Upstream"],
    steps: [
      {
        title: "Tobari marker detected",
        from: "Workspace process",
        to: "Gateway",
        sent: "A value with the broker-handle marker",
        withheld: "No primary credential exists in the Workspace",
        owner: "Gateway",
        failure: "Marker ambiguity is immediately invalid.",
        explanation: "The marker commits processing to the brokered path.",
        tone: "network",
      },
      {
        title: "Binding check fails",
        from: "Gateway",
        to: "Auth Broker",
        sent: "Handle and trusted principal",
        withheld: "Secret resolution",
        owner: "Auth Broker",
        failure: "Invalid, stale, copied, or mismatched handles are rejected.",
        explanation:
          "The record must match Context, project, provider, credential revision, target, and header binding.",
        tone: "denied",
      },
      {
        title: "No fallback",
        from: "Gateway",
        to: "Workspace process",
        sent: "403 credential_handle_invalid",
        withheld: "No passthrough, managed adapter, or policy request",
        owner: "Gateway",
        failure:
          "The caller must refresh its projection by re-entering after credential repair.",
        explanation:
          "A syntactically Tobari value never downgrades into ordinary authorization data.",
        tone: "denied",
      },
      {
        title: "No policy or upstream call",
        from: "Gateway",
        to: "OPA / Upstream",
        sent: "Nothing",
        withheld: "Request, credential, and egress",
        owner: "Gateway",
        failure: "No external effect occurs.",
        explanation:
          "Invalid binding is rejected before OPA because there is no valid ordinary request to authorize.",
        tone: "denied",
      },
    ],
  },
  {
    id: "policy-review",
    label: "Policy review and activation",
    summary:
      "A trusted host-side operator reviews retained evidence and explicitly activates an exact rule.",
    actors: [
      "Review store",
      "Host operator",
      "Tobari CLI",
      "Policy validator",
      "OPA runtime",
      "Workspace process",
    ],
    steps: [
      {
        title: "Read retained evidence",
        from: "Review store",
        to: "Tobari CLI",
        sent: "Strict secret-free denial records from a bounded tail",
        withheld: "Body, secrets, and any pre-existing candidate ID",
        owner: "Tobari CLI",
        failure: "Malformed records are rejected as candidates.",
        explanation:
          "Gateway records evidence; the trusted host CLI validates it before candidate discovery.",
        tone: "diagnostic",
      },
      {
        title: "Discover retained candidates",
        from: "Tobari CLI",
        to: "Host operator",
        sent: "Validated effect plus CLI-derived opaque candidate reference",
        withheld: "Body, secrets, and authority from list order",
        owner: "Tobari CLI",
        failure: "No candidate means no action target.",
        explanation: "Discovery can show candidates but cannot mutate policy.",
        tone: "diagnostic",
      },
      {
        title: "Choose an exact outcome",
        from: "Host operator",
        to: "Tobari CLI",
        sent: "Unchanged opaque reference and explicit allow or deny intent",
        withheld: "No reconstructed ID or wildcard",
        owner: "Host operator",
        failure: "Missing, stale, or mismatched reference is rejected.",
        explanation:
          "The trusted host action is separate from the denied Workspace request.",
        tone: "control",
      },
      {
        title: "Build the exact rule",
        from: "Tobari CLI",
        to: "Policy validator",
        sent: "Context, project, host, port, method, and path",
        withheld: "Body and credential",
        owner: "Tobari CLI",
        failure: "The current active policy remains unchanged.",
        explanation:
          "Rules do not automatically generalize to another Workspace or wildcard.",
        tone: "control",
      },
      {
        title: "Validate the whole policy",
        from: "Policy validator",
        to: "Tobari CLI",
        sent: "Complete syntax and semantic result",
        withheld: "No partial activation",
        owner: "OPA tooling",
        failure: "Any invalid source aborts activation.",
        explanation: "The aggregate policy is checked as a unit.",
        tone: "control",
      },
      {
        title: "Activate atomically",
        from: "Tobari CLI",
        to: "OPA runtime",
        sent: "A fully validated, content-addressed aggregate projection",
        withheld: "No transient half-written rule set",
        owner: "Tobari CLI",
        failure:
          "The candidate is not partially activated; retained source/projection state is restored and Gateway fails closed if OPA is unavailable.",
        explanation:
          "OPA hot-loads the validated complete bundle and reports its exact revision. Reset removes learned decisions and returns to baseline default deny; it is not an allow.",
        tone: "allowed",
      },
      {
        title: "Retry deliberately",
        from: "Host operator",
        to: "Workspace process",
        sent: "A conscious instruction to retry the task",
        withheld: "No automatic replay by Gateway",
        owner: "User / agent workflow",
        failure: "Without retry, no request is sent.",
        explanation:
          "A prior denial is never replayed automatically after policy changes.",
        tone: "network",
      },
    ],
  },
];
