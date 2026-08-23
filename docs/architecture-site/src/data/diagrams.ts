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

const standardNodes: DiagramNode[] = [
  {
    id: "workspace",
    label: "Workspace",
    detail: "Runs project tools without a direct external route.",
    kind: "untrusted",
  },
  {
    id: "gateway",
    label: "Gateway",
    detail: "Enforces the decision and owns the external connection.",
    kind: "network",
  },
  {
    id: "opa",
    label: "OPA",
    detail: "Decides the normalized ordinary HTTP effect.",
    kind: "control",
  },
  {
    id: "upstream",
    label: "Upstream",
    detail: "Receives only an authorized connection from Gateway.",
    kind: "allowed",
  },
];

const standardEdges: DiagramEdge[] = [
  {
    from: "workspace",
    to: "gateway",
    label: "guarded HTTP/HTTPS request",
    kind: "network",
  },
  {
    from: "gateway",
    to: "opa",
    label: "one body-free decision input",
    kind: "control",
  },
  { from: "opa", to: "gateway", label: "allow or deny", kind: "control" },
  {
    from: "gateway",
    to: "upstream",
    label: "separate authorized connection",
    kind: "allowed",
  },
];

const definitions: Record<string, [string, string]> = {
  "minimal-system": [
    "The four-part request path",
    "Workspace reaches upstream traffic only through Gateway, which asks OPA for one bounded decision.",
  ],
  "detailed-network": [
    "Supported Docker network topology",
    "Project traffic enters Gateway, policy stays on the control network, and only Gateway reaches the destination.",
  ],
  "workspace-manifest-workspace-cluster": [
    "Workspace Manifest and shared cluster",
    "Host-owned Manifest state selects a Workspace and the shared enforcement projection.",
  ],
  "workspace-lifecycle": [
    "Workspace lifecycle",
    "A Workspace identity and home outlive replaceable runtime resources.",
  ],
  "tls-split": [
    "HTTP and TLS paths",
    "Gateway controls both explicit proxy traffic and transparent HTTP/TLS traffic.",
  ],
  "project-principal": [
    "Project principal",
    "Gateway derives the project principal from the registered source endpoint.",
  ],
  "policy-loop": [
    "Policy review loop",
    "A denial becomes an opaque candidate; explicit host review activates a complete policy.",
  ],
  "credential-boundary": [
    "Native Workspace authentication boundary",
    "The release surface keeps authentication native to the Workspace and outside host inheritance.",
  ],
  "trust-boundaries": [
    "Trust boundaries",
    "Untrusted Workspace input crosses only the typed Gateway and policy boundaries.",
  ],
  "state-retention": [
    "State retention",
    "Manifest, Workspace, runtime, and shared policy state have explicit owners and lifetimes.",
  ],
  "code-layers": [
    "Code layers",
    "Domain, application, infrastructure, and CLI dependencies point inward.",
  ],
  "image-supply": [
    "Image supply",
    "Release images and source snapshots are selected by immutable build contracts.",
  ],
};

export const diagrams: Record<string, DiagramDefinition> = Object.fromEntries(
  Object.entries(definitions).map(([name, [title, description]]) => [
    name,
    {
      title,
      description,
      conceptual: true,
      nodes: standardNodes.map((node) => ({ ...node })),
      edges: standardEdges.map((edge) => ({ ...edge })),
    },
  ]),
);
