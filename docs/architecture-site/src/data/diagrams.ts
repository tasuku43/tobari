export type DiagramKind =
  | "trusted"
  | "untrusted"
  | "control"
  | "network"
  | "persistent"
  | "secret"
  | "allowed"
  | "denied"
  | "diagnostic";

export interface DiagramNode {
  id: string;
  label: string;
  detail: string;
  kind: DiagramKind;
  shape?: "box" | "store" | "boundary";
}

export interface DiagramEdge {
  id?: string;
  from: string;
  to: string;
  label: string;
  kind: DiagramKind;
  style?: "solid" | "dashed" | "blocked";
}

export interface DiagramDefinition {
  title: string;
  description: string;
  conceptual?: boolean;
  nodes: DiagramNode[];
  edges: DiagramEdge[];
}

const node = (
  id: string,
  label: string,
  detail: string,
  kind: DiagramKind,
  shape?: DiagramNode["shape"],
): DiagramNode => ({ id, label, detail, kind, shape });

const edge = (
  from: string,
  to: string,
  label: string,
  kind: DiagramKind,
  style?: DiagramEdge["style"],
): DiagramEdge => ({ from, to, label, kind, style });

const requestPathNodes = [
  node(
    "workspace",
    "Workspace",
    "Runs project tools without a direct external route.",
    "untrusted",
  ),
  node(
    "gateway",
    "Gateway",
    "Derives trusted identity and enforces the decision.",
    "network",
  ),
  node(
    "opa",
    "OPA",
    "Decides one normalized body-free HTTP effect.",
    "control",
  ),
  node(
    "upstream",
    "Upstream",
    "Receives only an authorized connection from Gateway.",
    "allowed",
  ),
];

const requestPathEdges = [
  edge("workspace", "gateway", "guarded HTTP/HTTPS request", "network"),
  edge("gateway", "opa", "one body-free decision input", "control"),
  edge("opa", "gateway", "allow or deny", "control"),
  edge("gateway", "upstream", "separate authorized connection", "allowed"),
];

export const diagrams: Record<string, DiagramDefinition> = {
  "minimal-system": {
    title: "The four-part request path",
    description:
      "Workspace reaches upstream traffic only through Gateway, which asks OPA for one bounded decision.",
    nodes: requestPathNodes,
    edges: requestPathEdges,
  },
  "detailed-network": {
    title: "Supported Docker network topology",
    description:
      "The Workspace has one internal project network; policy stays on the control network and only Gateway reaches the destination.",
    nodes: [
      node("workspace", "Workspace", "No direct public route.", "untrusted"),
      node(
        "project-network",
        "Dedicated network",
        "Carries traffic for one Workspace and one Gateway interface.",
        "network",
        "boundary",
      ),
      node(
        "gateway",
        "Gateway",
        "The only component joining project, control, and egress paths.",
        "network",
      ),
      node(
        "opa",
        "OPA",
        "Has no Workspace or egress network route.",
        "control",
      ),
      node(
        "upstream",
        "Upstream",
        "Reached by Gateway only after allow.",
        "allowed",
      ),
    ],
    edges: [
      edge("workspace", "project-network", "internal traffic", "network"),
      edge("project-network", "gateway", "guarded route", "network"),
      edge("gateway", "opa", "decision input and result", "control"),
      edge("gateway", "upstream", "authorized egress", "allowed"),
      edge("workspace", "upstream", "no direct route", "denied", "blocked"),
    ],
  },
  "workspace-template-context-workspace-cluster": {
    title: "Workspace Template and shared cluster",
    description:
      "A host-owned Context binds one Project to a Workspace Template while the shared cluster enforces the complete projection.",
    nodes: [
      node(
        "manifest",
        "Workspace Template",
        "Selects Runtime, baseline policy, source access, and stable authority.",
        "trusted",
        "store",
      ),
      node(
        "workspace",
        "Workspace",
        "Retains one permanent Context binding and its own home.",
        "persistent",
        "store",
      ),
      node(
        "runtime",
        "Runtime resources",
        "Replaceable container and dedicated network.",
        "network",
      ),
      node(
        "cluster",
        "Shared cluster",
        "Gateway and OPA enforce all active Context policy.",
        "control",
      ),
      node(
        "projection",
        "Aggregate projection",
        "Content-addressed policy built from every Context.",
        "trusted",
        "store",
      ),
    ],
    edges: [
      edge("manifest", "workspace", "permanent identity binding", "trusted"),
      edge("manifest", "projection", "validated policy source", "control"),
      edge("projection", "cluster", "read-only complete policy", "control"),
      edge(
        "workspace",
        "runtime",
        "reconciles replaceable resources",
        "network",
      ),
      edge("runtime", "cluster", "guarded request path", "network"),
    ],
  },
  "workspace-lifecycle": {
    title: "Workspace lifecycle",
    description:
      "Workspace identity and home outlive replaceable runtime resources; exit is not delete.",
    nodes: [
      node(
        "manifest",
        "Context binding",
        "Stable for the lifetime of the Workspace.",
        "trusted",
        "store",
      ),
      node(
        "state",
        "Workspace state",
        "Owns the logical identity and last applied entry.",
        "persistent",
        "store",
      ),
      node(
        "home",
        "Workspace home",
        "Persists agent-owned login and tool state.",
        "persistent",
        "store",
      ),
      node(
        "container",
        "Work container",
        "Replaceable runtime realization.",
        "network",
      ),
      node(
        "session",
        "Attached session",
        "Ends on exit without deleting logical state.",
        "diagnostic",
      ),
    ],
    edges: [
      edge("manifest", "state", "permanent binding", "trusted"),
      edge("state", "home", "owns until delete", "persistent"),
      edge("state", "container", "reconcile on entry", "network"),
      edge("container", "session", "bounded child process", "diagnostic"),
      edge("session", "state", "exit preserves state", "persistent", "dashed"),
    ],
  },
  "tls-split": {
    title: "One HTTPS request crosses two TLS sessions",
    description:
      "Gateway terminates Workspace-side TLS, authorizes the decrypted HTTP request, and creates a separate verified upstream TLS connection only after allow.",
    nodes: [
      node(
        "workspace",
        "Workspace client",
        "Uses an ordinary HTTPS URL and trusts the Tobari CA for Gateway-side TLS.",
        "untrusted",
      ),
      node(
        "gateway",
        "Gateway",
        "Terminates client TLS, owns the upstream connection, and enforces the decision.",
        "trusted",
      ),
      node(
        "opa",
        "OPA",
        "Decides the normalized HTTP effect without receiving the body.",
        "control",
      ),
      node(
        "upstream",
        "HTTPS destination",
        "Receives a separately connected and certificate-verified TLS session from Gateway.",
        "untrusted",
      ),
    ],
    edges: [
      edge(
        "workspace",
        "gateway",
        "Guarded TCP reaches transparent Gateway ingress",
        "network",
      ),
      edge(
        "workspace",
        "gateway",
        "TLS session 1 starts with a Tobari-issued leaf certificate",
        "trusted",
      ),
      edge(
        "gateway",
        "opa",
        "Gateway sends scheme, host, port, method, and path—not the body",
        "control",
      ),
      edge(
        "opa",
        "gateway",
        "OPA returns one allow or deny decision",
        "control",
      ),
      edge(
        "gateway",
        "upstream",
        "After allow, Gateway resolves the destination and opens TCP",
        "allowed",
      ),
      edge(
        "gateway",
        "upstream",
        "TLS session 2 verifies the destination certificate independently",
        "network",
      ),
      edge(
        "gateway",
        "upstream",
        "Gateway forwards HTTP over session 2; the response returns through both sessions",
        "allowed",
      ),
    ],
  },
  "project-principal": {
    title: "Context/Workspace principal",
    description:
      "Gateway derives authority from the kernel-observed source endpoint and host registry, never from request text.",
    nodes: [
      node(
        "request",
        "Request headers",
        "Untrusted text cannot select Context or Workspace authority.",
        "untrusted",
      ),
      node(
        "endpoint",
        "Observed endpoint",
        "Kernel-observed Workspace source address.",
        "network",
      ),
      node(
        "registry",
        "Principal registry",
        "Host-owned exact endpoint-to-identity mapping.",
        "trusted",
        "store",
      ),
      node(
        "principal",
        "Workspace principal",
        "Exact Context ID and Workspace ID pair.",
        "control",
      ),
      node("opa", "OPA input", "Receives the derived principal.", "control"),
    ],
    edges: [
      edge("endpoint", "registry", "exact lookup", "trusted"),
      edge("registry", "principal", "host-issued identity", "trusted"),
      edge("principal", "opa", "trusted request scope", "control"),
      edge("request", "principal", "cannot override", "denied", "blocked"),
    ],
  },
  "policy-loop": {
    title: "Policy review loop",
    description:
      "A denial becomes bounded evidence; explicit host review activates one complete policy and retry remains deliberate.",
    nodes: [
      node("deny", "Deny", "No upstream connection.", "denied"),
      node(
        "evidence",
        "Evidence",
        "Secret-free retained effect.",
        "diagnostic",
      ),
      node(
        "review",
        "Review",
        "Trusted host inspects current authority.",
        "trusted",
      ),
      node("decision", "Decision", "Allow, deny, or no action.", "control"),
      node(
        "validation",
        "Validation",
        "Exact rule and complete aggregate.",
        "control",
      ),
      node(
        "activation",
        "Activation",
        "Atomic known-good publication.",
        "allowed",
      ),
      node(
        "retry",
        "Deliberate retry",
        "The old request is never replayed.",
        "network",
      ),
    ],
    edges: [
      edge("deny", "evidence", "retain bounded facts", "diagnostic"),
      edge("evidence", "review", "produce opaque reference", "diagnostic"),
      edge("review", "decision", "explicit host choice", "trusted"),
      edge("decision", "validation", "bind exact target", "control"),
      edge("validation", "activation", "complete projection passes", "allowed"),
      edge("activation", "retry", "new policy is active", "allowed"),
      edge("retry", "deny", "a new request is evaluated", "network"),
    ],
  },
  "credential-boundary": {
    title: "Native Workspace authentication boundary",
    description:
      "The agent CLI owns login state inside one Workspace home; Tobari neither inherits host credentials nor exposes a release credential service.",
    nodes: [
      node(
        "home",
        "Workspace home",
        "Persistent tool-owned login files and configuration.",
        "persistent",
        "store",
      ),
      node(
        "agent",
        "Agent CLI",
        "Creates and reads its own login state.",
        "untrusted",
      ),
      node(
        "gateway",
        "Gateway",
        "Removes credential values from decision and audit input.",
        "network",
      ),
      node(
        "opa",
        "OPA",
        "Decides ordinary HTTP effect without credential values.",
        "control",
      ),
      node(
        "upstream",
        "Upstream",
        "Receives original request values only after allow.",
        "allowed",
      ),
    ],
    edges: [
      edge("home", "agent", "tool-owned login state", "persistent"),
      edge("agent", "gateway", "ordinary guarded request", "network"),
      edge("gateway", "opa", "credential-free decision input", "control"),
      edge("gateway", "upstream", "original values after allow", "allowed"),
    ],
  },
  "trust-boundaries": {
    title: "Trust boundaries",
    description:
      "Untrusted Workspace input crosses only typed Gateway and policy boundaries; host and Docker remain the trusted enforcement base.",
    nodes: [
      node(
        "workspace",
        "Workspace process",
        "Untrusted code with selected-root and home access.",
        "untrusted",
      ),
      node(
        "host",
        "Trusted host state",
        "Issues identity and owns lifecycle and policy source.",
        "trusted",
        "store",
      ),
      node(
        "gateway",
        "Gateway",
        "Enforces the request path and external connection.",
        "network",
      ),
      node("opa", "OPA", "Makes the bounded decision.", "control"),
      node("upstream", "Upstream", "External destination.", "allowed"),
    ],
    edges: [
      edge("host", "gateway", "read-only principal projection", "trusted"),
      edge("workspace", "gateway", "guarded untrusted request", "network"),
      edge("gateway", "opa", "typed decision input", "control"),
      edge("gateway", "upstream", "after one allow", "allowed"),
      edge("workspace", "upstream", "no direct route", "denied", "blocked"),
    ],
  },
  "state-retention": {
    title: "State retention",
    description:
      "Workspace Template, Context, Policy Memory, Workspace, home, runtime, and shared policy state have separate owners and lifetimes.",
    nodes: [
      node(
        "manifest",
        "Workspace Template configuration",
        "Host-owned reusable Runtime, defaults, and baseline policy.",
        "trusted",
        "store",
      ),
      node(
        "state",
        "Workspace state",
        "Durable logical identity and applied receipt.",
        "persistent",
        "store",
      ),
      node(
        "home",
        "Workspace home",
        "Writable agent-owned tool state.",
        "persistent",
        "store",
      ),
      node(
        "runtime",
        "Runtime resources",
        "Replaceable container and network.",
        "network",
      ),
      node(
        "cluster",
        "Shared cluster state",
        "Aggregate revision and shared resource identity.",
        "control",
        "store",
      ),
    ],
    edges: [
      edge("manifest", "state", "stable binding", "trusted"),
      edge("state", "home", "owns until delete", "persistent"),
      edge("state", "runtime", "reconciles", "network"),
      edge("manifest", "cluster", "contributes policy", "control"),
      edge("runtime", "cluster", "guarded request path", "network"),
    ],
  },
  "code-layers": {
    title: "Code layers",
    description:
      "Domain has no outward dependency; application and infrastructure depend inward, and CLI is the composition root.",
    nodes: [
      node("cli", "CLI", "Catalog, rendering, and composition.", "trusted"),
      node("app", "Application", "Task interpretation and ports.", "control"),
      node("infra", "Infrastructure", "Bounded external adapters.", "network"),
      node("domain", "Domain", "Pure vocabulary and invariants.", "persistent"),
    ],
    edges: [
      edge("cli", "app", "invokes use case", "control"),
      edge("cli", "infra", "injects adapter", "network"),
      edge("app", "domain", "depends on contracts", "control"),
      edge("infra", "domain", "implements ports with domain types", "control"),
      edge(
        "domain",
        "infra",
        "outward dependency forbidden",
        "denied",
        "blocked",
      ),
    ],
  },
  "image-supply": {
    title: "Image supply",
    description:
      "Canonical sources, byte-checked snapshots, pinned metadata, and inspected local images form one reviewed supply path.",
    nodes: [
      node(
        "source",
        "Canonical source",
        "The only editable Gateway and helper implementation.",
        "trusted",
        "store",
      ),
      node(
        "snapshot",
        "Embedded snapshot",
        "Byte-checked runtime build input.",
        "persistent",
        "store",
      ),
      node(
        "metadata",
        "Pinned metadata",
        "Immutable upstream digests and component contracts.",
        "control",
        "store",
      ),
      node(
        "image",
        "Local OCI image",
        "Built or reused only after exact validation.",
        "network",
      ),
      node(
        "runtime",
        "Runtime component",
        "Starts with inspected identity and compatibility.",
        "allowed",
      ),
    ],
    edges: [
      edge("source", "snapshot", "byte-equality gate", "trusted"),
      edge("snapshot", "image", "reviewed build input", "network"),
      edge("metadata", "image", "pinned dependency identity", "control"),
      edge("image", "runtime", "inspect before activation", "allowed"),
    ],
  },
};
