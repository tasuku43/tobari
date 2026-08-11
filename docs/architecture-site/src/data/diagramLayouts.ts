// Centralized positions keep component locations stable while numbered edges carry the explanation.
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

export const diagramLayouts: Record<string, DiagramLayout> = {
  "minimal-system": {
    width: 1000,
    height: 620,
    mode: "sequence",
    regions: [
      {
        id: "project-runtime",
        label: "Project runtime",
        japaneseLabel: "プロジェクトの実行環境",
        kind: "workspace",
        x: 15,
        y: 35,
        width: 220,
        height: 550,
      },
      {
        id: "shared-cluster",
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
      opa: { x: 475, y: 95 },
      broker: { x: 475, y: 525 },
      upstream: { x: 875, y: 310 },
    },
  },
  "detailed-network": {
    width: 1200,
    height: 680,
    mode: "sequence",
    regions: [
      {
        id: "project-runtime",
        label: "Project runtime and dedicated internal network",
        japaneseLabel: "プロジェクトの実行環境と専用内部ネットワーク",
        kind: "workspace",
        x: 15,
        y: 35,
        width: 390,
        height: 610,
      },
      {
        id: "shared-cluster",
        label: "Tobari shared cluster",
        japaneseLabel: "Tobari 共有クラスター",
        kind: "cluster",
        x: 425,
        y: 35,
        width: 565,
        height: 610,
      },
      {
        id: "external",
        label: "External DNS and destination",
        japaneseLabel: "外部の DNS と接続先",
        kind: "external",
        x: 1010,
        y: 35,
        width: 175,
        height: 610,
      },
    ],
    nodes: {
      process: { x: 95, y: 340 },
      projectnet: { x: 305, y: 340, width: 190 },
      gateway: { x: 555, y: 340, width: 205, height: 112 },
      controlnet: { x: 555, y: 115, width: 190 },
      opa: { x: 825, y: 95 },
      broker: { x: 825, y: 245 },
      egress: { x: 825, y: 470, width: 190 },
      upstream: { x: 1085, y: 470 },
    },
  },
  "workspace-context-cluster": {
    width: 1180,
    height: 700,
    mode: "relationship",
    regions: [
      {
        id: "host-owned-inputs",
        label: "Host-owned project and configuration",
        japaneseLabel: "ホストが管理するプロジェクトと設定",
        kind: "host",
        x: 15,
        y: 35,
        width: 230,
        height: 630,
      },
      {
        id: "logical-workspaces",
        label: "Logical Workspace identities",
        japaneseLabel: "論理 Workspace の識別",
        kind: "logical",
        x: 270,
        y: 35,
        width: 315,
        height: 630,
      },
      {
        id: "runtime-and-shared",
        label: "Recreated runtime and shared services",
        japaneseLabel: "作り直せる実行環境と共有サービス",
        kind: "runtime",
        x: 610,
        y: 35,
        width: 550,
        height: 630,
      },
    ],
    nodes: {
      root: { x: 125, y: 130 },
      contexta: { x: 125, y: 360 },
      contextb: { x: 125, y: 565 },
      workspacea: { x: 430, y: 245 },
      workspaceb: { x: 430, y: 530 },
      runtimea: { x: 705, y: 245 },
      cluster: { x: 1010, y: 390, width: 220, height: 120 },
    },
  },
  "workspace-lifecycle": {
    width: 920,
    height: 650,
    mode: "state",
    nodes: {
      absent: { x: 120, y: 320 },
      attached: { x: 455, y: 105 },
      detached: { x: 790, y: 320, width: 210, height: 112 },
      drift: { x: 455, y: 545, width: 210, height: 112 },
    },
  },
  "tls-split": {
    width: 1040,
    height: 650,
    mode: "sequence",
    regions: [
      {
        id: "workspace-client",
        label: "Workspace client",
        japaneseLabel: "Workspace のクライアント",
        kind: "workspace",
        x: 15,
        y: 35,
        width: 220,
        height: 580,
      },
      {
        id: "shared-cluster",
        label: "Gateway and OPA · host-managed",
        japaneseLabel: "Gateway と OPA · ホスト管理",
        kind: "cluster",
        x: 260,
        y: 35,
        width: 500,
        height: 580,
      },
      {
        id: "external",
        label: "External HTTPS destination",
        japaneseLabel: "外部の HTTPS 接続先",
        kind: "external",
        x: 785,
        y: 35,
        width: 240,
        height: 580,
      },
    ],
    nodes: {
      workspace: { x: 105, y: 315, width: 200, height: 108 },
      gateway: { x: 500, y: 315, width: 215, height: 118 },
      opa: { x: 500, y: 555, width: 195, height: 102 },
      upstream: { x: 925, y: 315, width: 200, height: 108 },
    },
  },
  "project-principal": {
    width: 1120,
    height: 650,
    mode: "sequence",
    regions: [
      {
        id: "trusted-host",
        label: "Trusted host lifecycle and registry",
        japaneseLabel: "信頼するホストのライフサイクルと登録情報",
        kind: "host",
        x: 15,
        y: 25,
        width: 485,
        height: 255,
      },
      {
        id: "project-network",
        label: "Workspace and project-dedicated network",
        japaneseLabel: "Workspace とプロジェクト専用ネットワーク",
        kind: "workspace",
        x: 15,
        y: 305,
        width: 485,
        height: 320,
      },
      {
        id: "shared-cluster",
        label: "Gateway and policy decision",
        japaneseLabel: "Gateway とポリシー判断",
        kind: "cluster",
        x: 530,
        y: 25,
        width: 570,
        height: 600,
      },
    ],
    nodes: {
      host: { x: 105, y: 105 },
      registry: { x: 380, y: 105, width: 210 },
      network: { x: 380, y: 485, width: 210 },
      workspace: { x: 105, y: 485 },
      gateway: { x: 690, y: 325, width: 220, height: 118 },
      opa: { x: 1010, y: 325 },
    },
  },
  "policy-loop": {
    width: 1080,
    height: 700,
    mode: "state",
    regions: [
      {
        id: "runtime-attempt",
        label: "Workspace and Gateway",
        japaneseLabel: "Workspace と Gateway",
        kind: "workspace",
        x: 15,
        y: 35,
        width: 220,
        height: 630,
      },
      {
        id: "trusted-review",
        label: "Trusted host review",
        japaneseLabel: "信頼するホストでのレビュー",
        kind: "host",
        x: 260,
        y: 35,
        width: 520,
        height: 630,
      },
      {
        id: "active-policy",
        label: "Validated active policy",
        japaneseLabel: "検証済みの有効ポリシー",
        kind: "cluster",
        x: 805,
        y: 35,
        width: 260,
        height: 630,
      },
    ],
    nodes: {
      deny: { x: 120, y: 165 },
      evidence: { x: 355, y: 115 },
      review: { x: 610, y: 115 },
      decision: { x: 610, y: 355 },
      validation: { x: 930, y: 245 },
      activation: { x: 930, y: 515 },
      retry: { x: 355, y: 555 },
    },
  },
  "credential-boundary": {
    width: 1120,
    height: 680,
    mode: "sequence",
    regions: [
      {
        id: "trusted-host",
        label: "Trusted host acquisition",
        japaneseLabel: "信頼するホストでの取得",
        kind: "host",
        x: 15,
        y: 25,
        width: 495,
        height: 255,
      },
      {
        id: "workspace",
        label: "Workspace receives only a handle",
        japaneseLabel: "Workspace にはハンドルだけを渡す",
        kind: "workspace",
        x: 15,
        y: 305,
        width: 495,
        height: 345,
      },
      {
        id: "shared-cluster",
        label: "Auth Broker and Gateway",
        japaneseLabel: "Auth Broker と Gateway",
        kind: "cluster",
        x: 540,
        y: 25,
        width: 350,
        height: 625,
      },
      {
        id: "external",
        label: "Exact external target",
        japaneseLabel: "完全一致する外部の接続先",
        kind: "external",
        x: 920,
        y: 305,
        width: 185,
        height: 345,
      },
    ],
    nodes: {
      host: { x: 105, y: 105 },
      vault: { x: 405, y: 105, width: 210 },
      handle: { x: 405, y: 350, width: 205 },
      workspace: { x: 105, y: 350 },
      gateway: { x: 705, y: 350, width: 220, height: 118 },
      upstream: { x: 1015, y: 350 },
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
      host: { x: 170, y: 105, width: 220 },
      services: { x: 515, y: 105, width: 235 },
      workspace: { x: 515, y: 390, width: 235, height: 118 },
      other: { x: 170, y: 555, width: 220 },
      upstream: { x: 890, y: 390, width: 220, height: 118 },
    },
  },
  "state-retention": {
    width: 1200,
    height: 620,
    mode: "relationship",
    nodes: {
      project: { x: 115, y: 310, width: 195 },
      workspace: { x: 355, y: 310, width: 195 },
      context: { x: 595, y: 310, width: 195 },
      cluster: { x: 835, y: 310, width: 195 },
      install: { x: 1075, y: 310, width: 195 },
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
    width: 1260,
    height: 720,
    mode: "sequence",
    regions: [
      {
        id: "canonical-source",
        label: "Canonical repository source",
        japaneseLabel: "リポジトリの正本ソース",
        kind: "source",
        x: 15,
        y: 35,
        width: 210,
        height: 650,
      },
      {
        id: "reviewed-pipeline",
        label: "Embedded snapshot and reviewed build pipeline",
        japaneseLabel: "埋め込みスナップショットとレビュー済みビルド",
        kind: "pipeline",
        x: 250,
        y: 35,
        width: 710,
        height: 650,
      },
      {
        id: "installed-runtime",
        label: "Installed host and runtime cluster",
        japaneseLabel: "インストール先ホストと実行クラスター",
        kind: "installation",
        x: 985,
        y: 35,
        width: 260,
        height: 650,
      },
    ],
    nodes: {
      "gateway-src": { x: 105, y: 120 },
      "broker-src": { x: 105, y: 350 },
      "policy-src": { x: 105, y: 580 },
      snapshots: { x: 390, y: 350, width: 220, height: 118 },
      images: { x: 680, y: 350, width: 210 },
      versions: { x: 900, y: 350, width: 210 },
      cluster: { x: 1135, y: 350, width: 190 },
    },
  },
};
