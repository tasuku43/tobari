import { diagrams, type DiagramDefinition } from "./diagrams";

const titles: Record<string, [string, string]> = {
  "minimal-system": [
    "4 つの要素からなるリクエスト経路",
    "Workspace から接続先への通信は Gateway を通り、OPA が一つの判断を返します。",
  ],
  "detailed-network": [
    "対応する Docker ネットワークの構成",
    "プロジェクトの通信は Gateway へ入り、ポリシー判断は制御ネットワークで行います。",
  ],
  "workspace-manifest-workspace-cluster": [
    "Workspace Manifest と共有クラスター",
    "ホストが管理する Manifest が Workspace と共有の執行投影を選びます。",
  ],
  "workspace-lifecycle": [
    "Workspace のライフサイクル",
    "Workspace の識別情報とホームは、置き換え可能なランタイムより長く残ります。",
  ],
  "tls-split": [
    "HTTP と TLS の経路",
    "Gateway が明示的プロキシと透過 HTTP/TLS の両方を制御します。",
  ],
  "project-principal": [
    "プロジェクトプリンシパル",
    "Gateway が登録済みの送信元エンドポイントから主体を導出します。",
  ],
  "policy-loop": [
    "ポリシー確認のループ",
    "拒否を不透明な候補として保持し、ホストの明示的な確認で完全なポリシーを有効化します。",
  ],
  "credential-boundary": [
    "Workspace 所有の認証境界",
    "release surface の認証は Workspace 内で行い、ホストの認証情報を継承しません。",
  ],
  "trust-boundaries": [
    "信頼境界",
    "信頼しない Workspace の入力は型付き Gateway とポリシーの境界だけを通ります。",
  ],
  "state-retention": [
    "状態の保持",
    "Manifest、Workspace、ランタイム、共有ポリシーには明示的な所有者と寿命があります。",
  ],
  "code-layers": [
    "コードのレイヤー",
    "Domain、Application、Infrastructure、CLI の依存は内向きです。",
  ],
  "image-supply": [
    "イメージの供給",
    "Release イメージとソーススナップショットを不変の build contract で選びます。",
  ],
};

const nodes = [
  [
    "workspace",
    "Workspace",
    "外部への直接経路を持たず、プロジェクトのツールを実行します。",
  ],
  ["gateway", "Gateway", "判断を執行し、外部接続を所有します。"],
  ["opa", "OPA", "正規化した通常の HTTP 通信を判断します。"],
  ["upstream", "接続先", "Gateway からの許可済み接続だけを受け取ります。"],
];

export const diagramsJa: Record<string, DiagramDefinition> = Object.fromEntries(
  Object.entries(diagrams).map(([name, diagram]) => {
    const [title, description] = titles[name] ?? [
      diagram.title,
      diagram.description,
    ];
    return [
      name,
      {
        ...diagram,
        title,
        description,
        nodes: diagram.nodes.map((node) => {
          const translation = nodes.find(([id]) => id === node.id);
          return translation
            ? { ...node, label: translation[1], detail: translation[2] }
            : node;
        }),
        edges: diagram.edges.map((edge) => ({
          ...edge,
          label:
            edge.from === "workspace"
              ? "保護された HTTP/HTTPS リクエスト"
              : edge.from === "gateway" && edge.to === "opa"
                ? "本文を含まない判断入力一つ"
                : edge.from === "opa"
                  ? "許可または拒否"
                  : "独立した許可済み接続",
        })),
      },
    ];
  }),
);
