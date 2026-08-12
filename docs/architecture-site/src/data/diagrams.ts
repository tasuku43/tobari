export type DiagramKind =
  | "trusted"
  | "untrusted"
  | "control"
  | "network"
  | "persistent"
  | "secret"
  | "handle"
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

export const diagrams: Record<string, DiagramDefinition> = {
  "minimal-system": {
    title: "The five-part request path",
    description:
      "A Workspace reaches upstream HTTP and HTTPS only through Gateway. Gateway asks OPA and uses Auth Broker only for brokered credentials.",
    conceptual: true,
    nodes: [
      {
        id: "workspace",
        label: "Workspace",
        detail: "Runs project tools without a direct external route.",
        kind: "untrusted",
      },
      {
        id: "gateway",
        label: "Gateway",
        detail:
          "Terminates transparently routed HTTP/TLS and enforces the decision.",
        kind: "network",
      },
      {
        id: "opa",
        label: "OPA",
        detail: "Decides the normalized ordinary HTTP effect.",
        kind: "control",
      },
      {
        id: "broker",
        label: "Auth Broker",
        detail: "Keeps and resolves static brokered credentials after allow.",
        kind: "secret",
      },
      {
        id: "upstream",
        label: "Upstream",
        detail: "Receives only an authorized connection from Gateway.",
        kind: "untrusted",
      },
    ],
    edges: [
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
        to: "broker",
        label: "introspect; resolve only after allow",
        kind: "handle",
        style: "dashed",
      },
      {
        from: "gateway",
        to: "upstream",
        label: "separate authorized connection",
        kind: "allowed",
      },
    ],
  },
  "detailed-network": {
    title: "Supported Docker network topology",
    description:
      "A request starts on one internal project network. Gateway is the only component that also joins egress; policy and credential control use a separate control network.",
    nodes: [
      {
        id: "process",
        label: "Workspace process",
        detail: "Runs inside a runtime container on one project network.",
        kind: "untrusted",
      },
      {
        id: "projectnet",
        label: "Dedicated internal network",
        detail:
          "Carries Workspace proxy traffic and has no Docker-provided external route.",
        kind: "trusted",
        shape: "boundary",
      },
      {
        id: "gateway",
        label: "Gateway",
        detail:
          "The only component attached to both the project network and egress.",
        kind: "network",
      },
      {
        id: "controlnet",
        label: "Control network",
        detail: "Carries Gateway ↔ OPA and Gateway ↔ Auth Broker traffic.",
        kind: "control",
        shape: "boundary",
      },
      {
        id: "opa",
        label: "OPA",
        detail: "Decides policy without an external connection.",
        kind: "control",
      },
      {
        id: "broker",
        label: "Auth Broker",
        detail: "Uses control and egress, but never joins a project network.",
        kind: "secret",
      },
      {
        id: "egress",
        label: "Egress network",
        detail:
          "Carries authorized external connections owned by Gateway or Broker.",
        kind: "network",
        shape: "boundary",
      },
      {
        id: "upstream",
        label: "DNS and upstream",
        detail: "Reached by Gateway, not directly by Workspace.",
        kind: "untrusted",
      },
    ],
    edges: [
      {
        from: "process",
        to: "projectnet",
        label: "HTTP/HTTPS proxy traffic enters the project network",
        kind: "network",
      },
      {
        from: "projectnet",
        to: "gateway",
        label: "Gateway receives the request on the project-specific interface",
        kind: "trusted",
      },
      {
        from: "process",
        to: "upstream",
        label: "No supported direct route exists",
        kind: "denied",
        style: "blocked",
      },
      {
        from: "gateway",
        to: "controlnet",
        label:
          "Policy and credential control traffic stays on the control network",
        kind: "control",
      },
      {
        from: "controlnet",
        to: "opa",
        label: "Body-free policy input and one allow or deny result",
        kind: "control",
      },
      {
        from: "controlnet",
        to: "broker",
        label: "Handle introspection; credential work only after allow",
        kind: "handle",
        style: "dashed",
      },
      {
        from: "gateway",
        to: "egress",
        label: "Gateway opens an external connection only after authorization",
        kind: "allowed",
      },
      {
        from: "egress",
        to: "upstream",
        label: "DNS, TCP, and TLS reach the selected destination",
        kind: "network",
      },
    ],
  },
  "workspace-context-cluster": {
    title: "Workspace, Context, cluster, and runtime",
    description:
      "The project root and stable Context ID identify a logical Workspace. The runtime container realizes it; the cluster is shared infrastructure.",
    nodes: [
      {
        id: "root",
        label: "Project root",
        detail: "/work/example selected from the current directory.",
        kind: "persistent",
        shape: "store",
      },
      {
        id: "contexta",
        label: "Context: default",
        detail:
          "Host-owned runtime, policy, agent profile, and credential configuration.",
        kind: "trusted",
        shape: "store",
      },
      {
        id: "workspacea",
        label: "Workspace A",
        detail: "Logical identity = normalized root + Context A.",
        kind: "control",
      },
      {
        id: "runtimea",
        label: "Runtime container A",
        detail:
          "Reconciled implementation; not the logical identity or sole lifetime owner.",
        kind: "untrusted",
      },
      {
        id: "contextb",
        label: "Context: review",
        detail: "A different host-owned configuration.",
        kind: "trusted",
        shape: "store",
      },
      {
        id: "workspaceb",
        label: "Workspace B",
        detail: "Same root with Context B is a different Workspace.",
        kind: "control",
      },
      {
        id: "cluster",
        label: "Shared cluster",
        detail: "Gateway, OPA, Auth Broker, CA, and runtime network state.",
        kind: "network",
        shape: "boundary",
      },
    ],
    edges: [
      {
        from: "root",
        to: "workspacea",
        label: "directory binding",
        kind: "persistent",
      },
      {
        from: "contexta",
        to: "workspacea",
        label: "stable Context ID",
        kind: "trusted",
      },
      {
        from: "workspacea",
        to: "runtimea",
        label: "reconciles",
        kind: "control",
      },
      {
        from: "root",
        to: "workspaceb",
        label: "same root",
        kind: "persistent",
      },
      {
        from: "contextb",
        to: "workspaceb",
        label: "different Context ID",
        kind: "trusted",
      },
      {
        from: "workspacea",
        to: "cluster",
        label: "uses shared services",
        kind: "network",
      },
      {
        from: "workspaceb",
        to: "cluster",
        label: "uses shared services",
        kind: "network",
      },
    ],
  },
  "workspace-lifecycle": {
    title: "Logical Workspace lifecycle",
    description:
      "Exit detaches the user session. Delete removes the logical Workspace and its owned runtime state. Missing containers or networks are reconciled on entry.",
    nodes: [
      {
        id: "absent",
        label: "Absent",
        detail: "No root index or Workspace instance exists.",
        kind: "diagnostic",
      },
      {
        id: "attached",
        label: "Attached",
        detail: "An entry session is using the Workspace.",
        kind: "allowed",
      },
      {
        id: "detached",
        label: "Detached but existing",
        detail:
          "Identity, home, runtime state, Context binding, and policy remain.",
        kind: "persistent",
        shape: "store",
      },
      {
        id: "drift",
        label: "Runtime drift or loss",
        detail:
          "Container/network can be recreated without changing logical identity.",
        kind: "diagnostic",
      },
    ],
    edges: [
      {
        from: "absent",
        to: "attached",
        label: "enter / create",
        kind: "allowed",
      },
      { from: "attached", to: "detached", label: "exit", kind: "control" },
      {
        from: "detached",
        to: "attached",
        label: "enter again",
        kind: "allowed",
      },
      { from: "detached", to: "absent", label: "delete", kind: "denied" },
      {
        from: "attached",
        to: "absent",
        label: "delete --force",
        kind: "denied",
        style: "dashed",
      },
      {
        from: "detached",
        to: "drift",
        label: "container/network loss or recipe change",
        kind: "diagnostic",
        style: "dashed",
      },
      {
        from: "drift",
        to: "attached",
        label: "next entry reconciles",
        kind: "allowed",
      },
    ],
  },
  "tls-split": {
    title: "One HTTPS request crosses two TLS sessions",
    description:
      "The client first creates a TLS session with Gateway. Gateway can then authorize the decrypted HTTP request and, only after allow, create a separate verified TLS session to the destination.",
    nodes: [
      {
        id: "workspace",
        label: "Workspace client",
        detail:
          "Uses an ordinary HTTPS URL and trusts the Tobari CA for the Gateway-side TLS session.",
        kind: "untrusted",
      },
      {
        id: "gateway",
        label: "Gateway",
        detail:
          "Terminates client-side TLS, owns the upstream connection, and enforces the decision.",
        kind: "trusted",
      },
      {
        id: "opa",
        label: "OPA",
        detail:
          "Decides the normalized HTTP effect without receiving the request body.",
        kind: "control",
      },
      {
        id: "upstream",
        label: "HTTPS destination",
        detail:
          "Receives a separately connected and certificate-verified TLS session from Gateway.",
        kind: "untrusted",
      },
    ],
    edges: [
      {
        id: "connect",
        from: "workspace",
        to: "gateway",
        label: "Guarded TCP reaches transparent Gateway ingress",
        kind: "network",
      },
      {
        id: "workspace-tls",
        from: "workspace",
        to: "gateway",
        label: "TLS session 1 starts with a Tobari-issued leaf certificate",
        kind: "trusted",
      },
      {
        id: "policy-query",
        from: "gateway",
        to: "opa",
        label:
          "Gateway sends scheme, host, port, method, and path—not the body",
        kind: "control",
      },
      {
        id: "policy-result",
        from: "opa",
        to: "gateway",
        label: "OPA returns one allow or deny decision",
        kind: "control",
      },
      {
        id: "upstream-connect",
        from: "gateway",
        to: "upstream",
        label: "After allow, Gateway resolves the destination and opens TCP",
        kind: "allowed",
      },
      {
        id: "upstream-tls",
        from: "gateway",
        to: "upstream",
        label:
          "TLS session 2 verifies the destination certificate independently",
        kind: "network",
      },
      {
        id: "https-forward",
        from: "gateway",
        to: "upstream",
        label:
          "Gateway forwards HTTP over session 2; the response returns through both sessions",
        kind: "allowed",
      },
    ],
  },
  "project-principal": {
    title: "Project principal establishment",
    description:
      "The host registry binds a Workspace source endpoint, Gateway endpoint, and dedicated network to Context and project identity. Request headers cannot replace that binding.",
    nodes: [
      {
        id: "host",
        label: "Trusted host lifecycle",
        detail: "Creates project network and Gateway attachment.",
        kind: "trusted",
      },
      {
        id: "registry",
        label: "Principal registry",
        detail:
          "Host-owned source/Gateway endpoints + network → Context ID + project ID record.",
        kind: "persistent",
        shape: "store",
      },
      {
        id: "network",
        label: "Workspace-dedicated network",
        detail: "One project principal for supported topology.",
        kind: "trusted",
        shape: "boundary",
      },
      {
        id: "workspace",
        label: "Workspace request",
        detail: "May contain arbitrary untrusted identity-looking headers.",
        kind: "untrusted",
      },
      {
        id: "gateway",
        label: "Workspace source endpoint",
        detail:
          "Looks up the principal selected by the kernel-observed peer address.",
        kind: "network",
      },
      {
        id: "opa",
        label: "OPA input",
        detail: "Uses registry-derived Context and project fields.",
        kind: "control",
      },
    ],
    edges: [
      {
        from: "host",
        to: "registry",
        label: "atomic registration",
        kind: "trusted",
      },
      {
        from: "host",
        to: "network",
        label: "creates the project-specific internal network",
        kind: "trusted",
      },
      {
        from: "network",
        to: "gateway",
        label: "carries this project's exact source endpoint",
        kind: "network",
      },
      {
        from: "registry",
        to: "gateway",
        label: "principal lookup",
        kind: "trusted",
      },
      {
        from: "workspace",
        to: "gateway",
        label: "request bytes",
        kind: "network",
      },
      {
        from: "workspace",
        to: "opa",
        label: "self-claimed IDs ignored",
        kind: "denied",
        style: "blocked",
      },
      {
        from: "gateway",
        to: "opa",
        label: "trusted principal + normalized effect",
        kind: "control",
      },
    ],
  },
  "policy-loop": {
    title: "Explicit policy-learning loop",
    description:
      "Denied evidence can be reviewed on the host. Nothing grants permission until an exact rule is validated and atomically activated; a retry remains deliberate.",
    nodes: [
      {
        id: "deny",
        label: "Denied request",
        detail: "403; no upstream connection.",
        kind: "denied",
      },
      {
        id: "evidence",
        label: "Retained evidence",
        detail:
          "Body-free and secret-free denial record; it contains no candidate ID.",
        kind: "diagnostic",
        shape: "store",
      },
      {
        id: "review",
        label: "Host review",
        detail:
          "Host CLI validates evidence, derives an opaque reference, and the user selects it unchanged.",
        kind: "trusted",
      },
      {
        id: "decision",
        label: "Explicit allow or deny",
        detail:
          "Exact Context, project, destination, port, method, and path effect.",
        kind: "control",
      },
      {
        id: "validation",
        label: "Whole-policy validation",
        detail: "Invalid aggregate leaves the previous active policy intact.",
        kind: "control",
      },
      {
        id: "activation",
        label: "Atomic activation",
        detail:
          "Running OPA loads one validated complete bundle revision; no partial rule set or invented wildcard.",
        kind: "allowed",
      },
      {
        id: "retry",
        label: "Deliberate retry",
        detail: "Gateway never replays the old request automatically.",
        kind: "network",
      },
    ],
    edges: [
      {
        from: "deny",
        to: "evidence",
        label: "retain diagnostic",
        kind: "diagnostic",
      },
      {
        from: "evidence",
        to: "review",
        label: "validate record and derive candidate reference",
        kind: "trusted",
      },
      {
        from: "review",
        to: "decision",
        label: "act by opaque reference",
        kind: "control",
      },
      {
        from: "decision",
        to: "validation",
        label: "build exact rule",
        kind: "control",
      },
      {
        from: "validation",
        to: "activation",
        label: "all sources valid",
        kind: "allowed",
      },
      {
        from: "activation",
        to: "retry",
        label: "operator decides when",
        kind: "network",
      },
      {
        from: "retry",
        to: "deny",
        label: "new request is evaluated again",
        kind: "network",
        style: "dashed",
      },
    ],
  },
  "credential-boundary": {
    title: "Where brokered credential material can move",
    description:
      "The host acquires a credential; Auth Broker encrypts it in a Context vault. Workspace receives a non-secret opaque handle. Gateway obtains the secret once only for an already allowed bound request.",
    nodes: [
      {
        id: "host",
        label: "Trusted host acquisition",
        detail: "Built-in GitHub helper or bounded stdin import.",
        kind: "trusted",
      },
      {
        id: "vault",
        label: "Encrypted Context vault",
        detail: "Primary credential encrypted under installation root key.",
        kind: "secret",
        shape: "store",
      },
      {
        id: "handle",
        label: "Project-bound handle",
        detail: "Opaque record selector projected into Workspace.",
        kind: "handle",
      },
      {
        id: "workspace",
        label: "Workspace",
        detail: "Can read handle; cannot read brokered primary secret.",
        kind: "untrusted",
      },
      {
        id: "gateway",
        label: "Gateway post-policy path",
        detail: "Resolves once and replaces only declared destination header.",
        kind: "network",
      },
      {
        id: "upstream",
        label: "Exact HTTPS target",
        detail: "Receives the credential header for the allowed request.",
        kind: "untrusted",
      },
    ],
    edges: [
      {
        from: "host",
        to: "vault",
        label: "secret over protected host/broker input",
        kind: "secret",
      },
      {
        from: "vault",
        to: "handle",
        label: "non-secret bound record",
        kind: "handle",
        style: "dashed",
      },
      {
        from: "handle",
        to: "workspace",
        label: "environment or complete file projection",
        kind: "handle",
      },
      {
        from: "workspace",
        to: "gateway",
        label: "handle in declared source header",
        kind: "handle",
      },
      {
        from: "vault",
        to: "gateway",
        label: "resolve after allow only",
        kind: "secret",
        style: "dashed",
      },
      {
        from: "gateway",
        to: "upstream",
        label: "declared transformed header over TLS",
        kind: "secret",
      },
      {
        from: "vault",
        to: "workspace",
        label: "primary secret never projected",
        kind: "denied",
        style: "blocked",
      },
    ],
  },
  "trust-boundaries": {
    title: "Trusted and untrusted regions",
    description:
      "Tobari narrows authority at the project, network, policy, and credential boundaries. It assumes the host, Docker, kernel, Gateway, OPA, and Auth Broker are trusted enforcement infrastructure.",
    nodes: [
      {
        id: "host",
        label: "Trusted host controls",
        detail: "CLI lifecycle, policy review, root key, provider acquisition.",
        kind: "trusted",
        shape: "boundary",
      },
      {
        id: "services",
        label: "Trusted enforcement services",
        detail: "Docker, Gateway, OPA, Auth Broker, and their control state.",
        kind: "trusted",
        shape: "boundary",
      },
      {
        id: "workspace",
        label: "Untrusted Workspace processes",
        detail:
          "Can read/write the selected project root and share one Workspace boundary.",
        kind: "untrusted",
        shape: "boundary",
      },
      {
        id: "other",
        label: "Other Workspace / host files",
        detail: "Not mounted or network-reachable in supported topology.",
        kind: "persistent",
        shape: "boundary",
      },
      {
        id: "upstream",
        label: "Allowed upstream",
        detail:
          "May receive any Workspace-readable data sent by an allowed effect.",
        kind: "untrusted",
        shape: "boundary",
      },
    ],
    edges: [
      {
        from: "host",
        to: "services",
        label: "configuration and approval",
        kind: "trusted",
      },
      {
        from: "services",
        to: "workspace",
        label: "runtime, proxy, CA, opaque handles",
        kind: "control",
      },
      {
        from: "workspace",
        to: "other",
        label: "selection boundary blocks ordinary access",
        kind: "denied",
        style: "blocked",
      },
      {
        from: "workspace",
        to: "upstream",
        label: "only through an allowed Gateway effect",
        kind: "allowed",
      },
      {
        from: "services",
        to: "upstream",
        label: "trusted egress connection",
        kind: "network",
      },
    ],
  },
  "state-retention": {
    title: "State lifetime follows ownership, not one container",
    description:
      "Workspace, Context, cluster, credential, and installation state have different owners and deletion operations.",
    nodes: [
      {
        id: "project",
        label: "Project files",
        detail: "User-owned; never deleted by Workspace lifecycle commands.",
        kind: "persistent",
        shape: "store",
      },
      {
        id: "workspace",
        label: "Workspace-owned state",
        detail: "Index, instance, home, container, network, principal.",
        kind: "persistent",
        shape: "store",
      },
      {
        id: "context",
        label: "Context-owned state",
        detail:
          "Manifest, runtime recipe, policy sources, provider configuration, encrypted vault.",
        kind: "persistent",
        shape: "store",
      },
      {
        id: "cluster",
        label: "Cluster runtime state",
        detail:
          "Shared services, networks, principal registry, aggregate projections.",
        kind: "network",
        shape: "store",
      },
      {
        id: "install",
        label: "Installation state",
        detail: "Root key and Gateway CA volume.",
        kind: "secret",
        shape: "store",
      },
    ],
    edges: [
      {
        from: "workspace",
        to: "workspace",
        label: "exit preserves; delete removes",
        kind: "control",
      },
      {
        from: "cluster",
        to: "cluster",
        label: "down removes runtime; purge also removes CA",
        kind: "denied",
      },
      {
        from: "context",
        to: "context",
        label: "cluster down/purge preserves",
        kind: "allowed",
      },
      {
        from: "install",
        to: "install",
        label: "root key survives down/purge; CA survives only non-purge",
        kind: "secret",
      },
      {
        from: "project",
        to: "project",
        label: "all lifecycle operations preserve",
        kind: "persistent",
      },
    ],
  },
  "code-layers": {
    title: "Four-layer Go dependency direction",
    description:
      "Domain owns pure invariants. Application owns task interpretation and minimal ports. Infrastructure implements effects. CLI derives the public contract and composes dependencies.",
    nodes: [
      {
        id: "cli",
        label: "CLI composition root",
        detail: "Catalog, typed argv, help, presentation, wiring.",
        kind: "control",
      },
      {
        id: "app",
        label: "Application",
        detail: "Use cases and task-specific ports.",
        kind: "trusted",
      },
      {
        id: "domain",
        label: "Domain",
        detail: "Pure vocabulary and invariants; no I/O.",
        kind: "persistent",
      },
      {
        id: "infra",
        label: "Infrastructure",
        detail:
          "Docker, files, processes, Gateway/Broker assets, external adapters.",
        kind: "network",
      },
    ],
    edges: [
      { from: "cli", to: "app", label: "invoke use cases", kind: "control" },
      {
        from: "cli",
        to: "infra",
        label: "construct concrete adapters",
        kind: "control",
      },
      {
        from: "app",
        to: "domain",
        label: "interpret domain types",
        kind: "trusted",
      },
      {
        from: "infra",
        to: "domain",
        label: "satisfy domain/app contracts",
        kind: "network",
      },
      {
        from: "domain",
        to: "infra",
        label: "outward dependency forbidden",
        kind: "denied",
        style: "blocked",
      },
    ],
  },
  "image-supply": {
    title: "Runtime component supply path",
    description:
      "Canonical source is checked against embedded snapshots. Reviewed workflows build digest-addressable OCI images, and runtime assets pin service identities by digest.",
    nodes: [
      {
        id: "gateway-src",
        label: "gateway/ canonical source",
        detail: "Python mitmproxy addon and tests.",
        kind: "trusted",
      },
      {
        id: "broker-src",
        label: "authbroker/ canonical source",
        detail: "Python broker, vault, acquisition, and tests.",
        kind: "trusted",
      },
      {
        id: "policy-src",
        label: "policy/ canonical source",
        detail: "Rego sources and tests.",
        kind: "trusted",
      },
      {
        id: "snapshots",
        label: "Embedded runtime snapshots",
        detail: "Byte/content checked copies materialized by the Go CLI.",
        kind: "persistent",
        shape: "store",
      },
      {
        id: "images",
        label: "GHCR OCI images",
        detail: "Gateway/Auth Broker identities pinned by immutable digest.",
        kind: "network",
      },
      {
        id: "versions",
        label: "versions.env + Compose",
        detail: "Reviewed versions, digests, and topology used at runtime.",
        kind: "control",
        shape: "store",
      },
      {
        id: "cluster",
        label: "Verified cluster startup",
        detail: "Images are selected and verified before service activation.",
        kind: "allowed",
      },
    ],
    edges: [
      {
        from: "gateway-src",
        to: "snapshots",
        label: "source/snapshot drift check",
        kind: "diagnostic",
      },
      {
        from: "broker-src",
        to: "snapshots",
        label: "source/snapshot drift check",
        kind: "diagnostic",
      },
      {
        from: "policy-src",
        to: "snapshots",
        label: "embedded policy source",
        kind: "diagnostic",
      },
      {
        from: "snapshots",
        to: "images",
        label: "reviewed build workflow",
        kind: "network",
      },
      {
        from: "images",
        to: "versions",
        label: "immutable digest identity",
        kind: "control",
      },
      {
        from: "versions",
        to: "cluster",
        label: "Compose reconciliation",
        kind: "allowed",
      },
    ],
  },
};
