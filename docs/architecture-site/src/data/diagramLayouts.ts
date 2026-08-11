export type DiagramFlowMode = "sequence" | "relationship" | "state";

export interface DiagramNodePosition {
  x: number;
  y: number;
  width?: number;
  height?: number;
}

export interface DiagramLayout {
  width: number;
  height: number;
  mode: DiagramFlowMode;
  nodes: Record<string, DiagramNodePosition>;
}

export const diagramLayouts: Record<string, DiagramLayout> = {
  "minimal-system": {
    width: 1000,
    height: 620,
    mode: "sequence",
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
    mode: "relationship",
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
    nodes: {
      root: { x: 105, y: 125 },
      contexta: { x: 105, y: 465 },
      workspacea: { x: 365, y: 250 },
      runtimea: { x: 650, y: 250 },
      contextb: { x: 365, y: 555 },
      workspaceb: { x: 650, y: 555 },
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
    width: 1000,
    height: 700,
    mode: "state",
    nodes: {
      deny: { x: 110, y: 150 },
      evidence: { x: 500, y: 90 },
      review: { x: 890, y: 150 },
      decision: { x: 890, y: 430 },
      validation: { x: 700, y: 610 },
      activation: { x: 300, y: 610 },
      retry: { x: 110, y: 430 },
    },
  },
  "credential-boundary": {
    width: 1120,
    height: 680,
    mode: "sequence",
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
    nodes: {
      host: { x: 170, y: 105, width: 220 },
      services: { x: 515, y: 105, width: 235 },
      workspace: { x: 515, y: 360, width: 235, height: 118 },
      other: { x: 170, y: 565, width: 220 },
      upstream: { x: 890, y: 360, width: 220, height: 118 },
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
    nodes: {
      "gateway-src": { x: 105, y: 120 },
      "broker-src": { x: 105, y: 350 },
      "policy-src": { x: 105, y: 580 },
      snapshots: { x: 390, y: 350, width: 220, height: 118 },
      images: { x: 680, y: 350, width: 210 },
      versions: { x: 960, y: 350, width: 210 },
      cluster: { x: 1175, y: 350, width: 190 },
    },
  },
};
