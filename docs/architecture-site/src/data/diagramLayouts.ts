// Centralized positions keep the release-site diagrams deterministic.
export type DiagramFlowMode = "sequence" | "relationship" | "state";

export interface DiagramNodePosition {
  x: number;
  y: number;
  width?: number;
  height?: number;
}

export type DiagramRegionKind =
  | "workspace"
  | "host"
  | "cluster"
  | "external"
  | "logical"
  | "runtime"
  | "source"
  | "pipeline"
  | "installation";

export interface DiagramRegion {
  id: string;
  label: string;
  japaneseLabel: string;
  kind: DiagramRegionKind;
  x: number;
  y: number;
  width: number;
  height: number;
}

export interface DiagramLayout {
  width: number;
  height: number;
  mode: DiagramFlowMode;
  nodes: Record<string, DiagramNodePosition>;
  regions?: DiagramRegion[];
}

const standardLayout: DiagramLayout = {
  width: 1000,
  height: 620,
  mode: "sequence",
  regions: [
    {
      id: "workspace",
      label: "Project runtime",
      japaneseLabel: "プロジェクトの実行環境",
      kind: "workspace",
      x: 15,
      y: 35,
      width: 220,
      height: 550,
    },
    {
      id: "cluster",
      label: "Tobari shared cluster · host-managed",
      japaneseLabel: "Tobari 共有クラスター · ホスト管理",
      kind: "cluster",
      x: 255,
      y: 35,
      width: 535,
      height: 550,
    },
    {
      id: "external",
      label: "External",
      japaneseLabel: "外部",
      kind: "external",
      x: 810,
      y: 35,
      width: 175,
      height: 550,
    },
  ],
  nodes: {
    workspace: { x: 110, y: 310 },
    gateway: { x: 475, y: 310, width: 210, height: 112 },
    opa: { x: 475, y: 110 },
    upstream: { x: 875, y: 310 },
  },
};

const keys = [
  "minimal-system",
  "detailed-network",
  "workspace-manifest-workspace-cluster",
  "workspace-lifecycle",
  "tls-split",
  "project-principal",
  "policy-loop",
  "credential-boundary",
  "trust-boundaries",
  "state-retention",
  "code-layers",
  "image-supply",
];

export const diagramLayouts: Record<string, DiagramLayout> = Object.fromEntries(
  keys.map((key) => [
    key,
    { ...standardLayout, nodes: { ...standardLayout.nodes } },
  ]),
);
