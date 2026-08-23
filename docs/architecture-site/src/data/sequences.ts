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

const requestSteps = (denied: boolean): SequenceStep[] => [
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
    title: denied ? "Default deny" : "One policy decision",
    from: "OPA",
    to: "Gateway",
    sent: denied
      ? "Deny plus bounded review evidence"
      : "A structured allow decision",
    withheld: "No automatic policy edit or wildcard",
    owner: "OPA",
    failure: denied
      ? "The effect remains denied."
      : "Unexpected output fails closed.",
    explanation: denied
      ? "The host operator may review the exact retained evidence."
      : "Gateway enforces the complete decision.",
    tone: denied ? "denied" : "allowed",
  },
  {
    title: denied ? "No upstream connection" : "Separate upstream connection",
    from: "Gateway",
    to: "Upstream",
    sent: denied ? "Nothing" : "The authorized request; body is streamed",
    withheld: denied
      ? "The entire request and egress"
      : "Policy internals and trusted registry state",
    owner: "Gateway",
    failure: denied
      ? "No external side effect occurs."
      : "Connection failure is reported without changing policy.",
    explanation: denied
      ? "A denial never opens an upstream connection."
      : "The Workspace has no direct egress route.",
    tone: denied ? "denied" : "allowed",
  },
];

export const sequenceScenarios: SequenceScenario[] = [
  {
    id: "allowed-passthrough",
    label: "Allowed passthrough request",
    summary:
      "A normal HTTP effect is identified, authorized once, and then streamed to the upstream service.",
    actors: workspaceActors,
    steps: requestSteps(false),
  },
  {
    id: "learnable-denial",
    label: "Learnable policy denial",
    summary:
      "A request outside the active exact rules is denied before any upstream connection.",
    actors: ["Workspace process", "Gateway", "OPA", "Review store", "Upstream"],
    steps: requestSteps(true),
  },
  {
    id: "opa-unavailable",
    label: "OPA unavailable",
    summary:
      "Authorization infrastructure failure is a closed path, not permission.",
    actors: workspaceActors,
    steps: [
      requestSteps(false)[0],
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
