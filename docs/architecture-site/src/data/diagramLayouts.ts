// Centralized positions keep release-site diagrams deterministic and make
// node/edge drift fail during the static build.
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

const requestPath: DiagramLayout = {
  width: 1000,
  height: 580,
  mode: "sequence",
  regions: [
    {
      id: "workspace-region",
      label: "Workspace",
      japaneseLabel: "Workspace",
      kind: "workspace",
      x: 20,
      y: 35,
      width: 200,
      height: 510,
    },
    {
      id: "cluster-region",
      label: "Host-managed shared cluster",
      japaneseLabel: "ホスト管理の共有クラスター",
      kind: "cluster",
      x: 245,
      y: 35,
      width: 500,
      height: 510,
    },
    {
      id: "external-region",
      label: "External",
      japaneseLabel: "外部",
      kind: "external",
      x: 770,
      y: 35,
      width: 210,
      height: 510,
    },
  ],
  nodes: {
    workspace: { x: 120, y: 290 },
    gateway: { x: 430, y: 290, width: 200 },
    opa: { x: 610, y: 115 },
    upstream: { x: 875, y: 290 },
  },
};

const tlsPath: DiagramLayout = {
  ...requestPath,
  regions: requestPath.regions?.map((region) =>
    region.id === "external-region"
      ? {
          ...region,
          label: "External HTTPS destination",
          japaneseLabel: "外部の HTTPS 接続先",
        }
      : { ...region },
  ),
  nodes: { ...requestPath.nodes },
};

export const diagramLayouts: Record<string, DiagramLayout> = {
  "minimal-system": requestPath,
  "detailed-network": {
    width: 1100,
    height: 620,
    mode: "relationship",
    nodes: {
      workspace: { x: 100, y: 310 },
      "project-network": { x: 315, y: 310, width: 210 },
      gateway: { x: 560, y: 310 },
      opa: { x: 560, y: 105 },
      upstream: { x: 970, y: 310 },
    },
  },
  "workspace-template-context-workspace-cluster": {
    width: 1100,
    height: 650,
    mode: "relationship",
    nodes: {
      manifest: { x: 130, y: 160, width: 220 },
      workspace: { x: 400, y: 340, width: 210 },
      runtime: { x: 675, y: 470, width: 210 },
      projection: { x: 675, y: 150, width: 220 },
      cluster: { x: 960, y: 310, width: 210 },
    },
  },
  "workspace-lifecycle": {
    width: 1100,
    height: 650,
    mode: "state",
    nodes: {
      manifest: { x: 120, y: 150 },
      state: { x: 380, y: 300, width: 210 },
      home: { x: 660, y: 130 },
      container: { x: 660, y: 450 },
      session: { x: 970, y: 450 },
    },
  },
  "tls-split": tlsPath,
  "project-principal": {
    width: 1100,
    height: 620,
    mode: "sequence",
    nodes: {
      request: { x: 120, y: 480 },
      endpoint: { x: 120, y: 160 },
      registry: { x: 390, y: 160, width: 220 },
      principal: { x: 680, y: 300, width: 220 },
      opa: { x: 970, y: 300 },
    },
  },
  "policy-loop": {
    width: 1000,
    height: 620,
    mode: "state",
    nodes: {
      deny: { x: 120, y: 180 },
      evidence: { x: 380, y: 100 },
      review: { x: 690, y: 130 },
      decision: { x: 900, y: 310 },
      validation: { x: 700, y: 520 },
      activation: { x: 380, y: 530 },
      retry: { x: 110, y: 400 },
    },
  },
  "credential-boundary": {
    width: 1100,
    height: 620,
    mode: "sequence",
    nodes: {
      home: { x: 120, y: 120 },
      agent: { x: 120, y: 420 },
      gateway: { x: 500, y: 320, width: 210 },
      opa: { x: 700, y: 120 },
      upstream: { x: 970, y: 320 },
    },
  },
  "trust-boundaries": {
    width: 1040,
    height: 680,
    mode: "relationship",
    regions: [
      {
        id: "trusted-infrastructure",
        label: "Trusted enforcement infrastructure",
        japaneseLabel: "信頼する強制基盤",
        kind: "host",
        x: 15,
        y: 25,
        width: 710,
        height: 245,
      },
      {
        id: "untrusted-workspace",
        label: "Untrusted project execution",
        japaneseLabel: "信頼しないプロジェクト実行環境",
        kind: "workspace",
        x: 245,
        y: 300,
        width: 500,
        height: 350,
      },
      {
        id: "external",
        label: "External destination",
        japaneseLabel: "外部の接続先",
        kind: "external",
        x: 775,
        y: 300,
        width: 250,
        height: 350,
      },
    ],
    nodes: {
      workspace: { x: 515, y: 390, width: 235, height: 118 },
      host: { x: 150, y: 105, width: 220 },
      gateway: { x: 405, y: 105, width: 210 },
      opa: { x: 625, y: 105 },
      upstream: { x: 890, y: 390, width: 220, height: 118 },
    },
  },
  "state-retention": {
    width: 1100,
    height: 660,
    mode: "relationship",
    nodes: {
      manifest: { x: 120, y: 130, width: 220 },
      state: { x: 390, y: 300, width: 210 },
      home: { x: 650, y: 110 },
      runtime: { x: 650, y: 490 },
      cluster: { x: 970, y: 300, width: 210 },
    },
  },
  "code-layers": {
    width: 920,
    height: 670,
    mode: "relationship",
    nodes: {
      cli: { x: 455, y: 85, width: 220 },
      app: { x: 270, y: 310, width: 210 },
      infra: { x: 650, y: 310, width: 220 },
      domain: { x: 455, y: 565, width: 220 },
    },
  },
  "image-supply": {
    width: 1100,
    height: 620,
    mode: "sequence",
    nodes: {
      source: { x: 120, y: 190 },
      snapshot: { x: 390, y: 190, width: 210 },
      metadata: { x: 390, y: 470, width: 210 },
      image: { x: 700, y: 300, width: 210 },
      runtime: { x: 990, y: 300 },
    },
  },
};
