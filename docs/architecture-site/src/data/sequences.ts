export type SequenceTone =
  "control" | "network" | "allowed" | "denied" | "diagnostic";

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

const workspaceActors = ["Workspace process", "Gateway", "OPA", "Upstream"];

const deniedRequestSteps = (): SequenceStep[] => [
  {
    title: "Guarded HTTP request arrives",
    from: "Workspace process",
    to: "Gateway",
    sent: "HTTP method, URL, headers, and a streaming body",
    withheld: "No trusted identity is accepted from request text",
    owner: "Gateway",
    failure: "Malformed input stops before external work.",
    explanation:
      "The Workspace reaches the destination only through the host Gateway.",
    tone: "network",
  },
  {
    title: "Decision input is normalized",
    from: "Gateway",
    to: "OPA",
    sent: "Workspace identity and normalized non-secret HTTP dimensions",
    withheld: "Request body and credential values",
    owner: "Gateway",
    failure: "Normalization failure closes the request.",
    explanation: "The body is not a policy-rule identity dimension.",
    tone: "control",
  },
  {
    title: "Default deny",
    from: "OPA",
    to: "Gateway",
    sent: "Deny plus bounded review evidence",
    withheld: "No automatic policy edit or wildcard",
    owner: "OPA",
    failure: "The effect remains denied.",
    explanation: "The host operator may review the exact retained evidence.",
    tone: "denied",
  },
  {
    title: "No upstream connection",
    from: "Gateway",
    to: "Upstream",
    sent: "Nothing",
    withheld: "The entire request and egress",
    owner: "Gateway",
    failure: "No external side effect occurs.",
    explanation: "A denial never opens an upstream connection.",
    tone: "denied",
  },
];

const allowedRequestSteps = (): SequenceStep[] => [
  {
    title: "Guarded HTTP request arrives",
    from: "Workspace process",
    to: "Gateway",
    sent: "HTTP method, URL, headers, and a streaming body",
    withheld: "No trusted identity is accepted from request text",
    owner: "Gateway",
    failure: "The request stops before policy or upstream work.",
    explanation:
      "Explicit proxy compatibility and the transparent route converge at Gateway.",
    tone: "network",
  },
  {
    title: "Principal is established",
    from: "Workspace source endpoint",
    to: "Gateway",
    sent: "Host-owned Manifest ID and Workspace ID from the principal registry",
    withheld: "Workspace-supplied identity headers",
    owner: "Gateway",
    failure: "An unknown or ambiguous source endpoint fails closed.",
    explanation:
      "The kernel-observed source endpoint, not request text, selects authority.",
    tone: "control",
  },
  {
    title: "Decision input is normalized",
    from: "Gateway",
    to: "OPA",
    sent: "Principal, scheme, host, port, method, path, and non-secret headers",
    withheld: "Request body and credential values",
    owner: "Gateway",
    failure: "Malformed input is rejected before upstream work.",
    explanation: "The body is never a policy-rule identity dimension.",
    tone: "control",
  },
  {
    title: "One policy decision",
    from: "OPA",
    to: "Gateway",
    sent: "A structured allow decision",
    withheld: "Credential values and request body",
    owner: "OPA",
    failure: "Deny, malformed output, timeout, or outage closes the path.",
    explanation: "OPA decides the effect and Gateway enforces the result.",
    tone: "allowed",
  },
  {
    title: "Destination is resolved",
    from: "Gateway",
    to: "DNS / resolver",
    sent: "Allowed destination hostname",
    withheld: "Workspace network access and credential values",
    owner: "Gateway",
    failure: "Resolution or destination validation failure returns an error.",
    explanation:
      "Resolution happens only after allow and remains Gateway-owned.",
    tone: "network",
  },
  {
    title: "Separate upstream connection",
    from: "Gateway",
    to: "Upstream",
    sent: "The authorized request; body is streamed",
    withheld: "Policy internals and trusted registry state",
    owner: "Gateway",
    failure: "Connection failure is reported without changing policy.",
    explanation:
      "Gateway opens egress; the Workspace never receives a direct route.",
    tone: "allowed",
  },
  {
    title: "Secret-free audit event",
    from: "Gateway",
    to: "Host diagnostics",
    sent: "Decision dimensions, outcome, and bounded diagnostics",
    withheld: "Body and credential values",
    owner: "Gateway",
    failure: "Diagnostics never turn a denied request into allow.",
    explanation:
      "Audit explains the effect without copying payloads or credential values.",
    tone: "diagnostic",
  },
];

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
    steps: allowedRequestSteps(),
  },
  {
    id: "learnable-denial",
    label: "Learnable policy denial",
    summary:
      "A request outside the active exact rules is denied before any upstream connection.",
    actors: ["Workspace process", "Gateway", "OPA", "Review store", "Upstream"],
    steps: deniedRequestSteps(),
  },
  {
    id: "opa-unavailable",
    label: "OPA unavailable",
    summary:
      "Authorization infrastructure failure is a closed path, not permission.",
    actors: workspaceActors,
    steps: [
      allowedRequestSteps()[0],
      {
        title: "Policy query fails",
        from: "Gateway",
        to: "OPA",
        sent: "Secret-free normalized input",
        withheld: "Body and direct egress",
        owner: "Gateway",
        failure: "The request ends at Gateway with policy_unavailable.",
        explanation: "A missing authorization decision never becomes allow.",
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
        sent: "Secret-free exact effect and an opaque candidate reference",
        withheld: "Body, secrets, and authority from display order",
        owner: "Tobari CLI",
        failure: "Invalid evidence is not a candidate.",
        explanation: "Discovery shows candidates but does not mutate policy.",
        tone: "diagnostic",
      },
      {
        title: "Activate atomically",
        from: "Tobari CLI",
        to: "OPA runtime",
        sent: "A fully validated aggregate policy projection",
        withheld: "No transient half-written rule set",
        owner: "Tobari CLI",
        failure: "The active policy remains unchanged on validation failure.",
        explanation: "The operator chooses one exact rule before activation.",
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
        explanation: "A prior denial is never replayed automatically.",
        tone: "network",
      },
    ],
  },
];
