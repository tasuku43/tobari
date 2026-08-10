import { diagrams, type DiagramDefinition, type DiagramEdge } from "./diagrams";

interface LocalizedNodeText {
  label: string;
  detail: string;
}

interface LocalizedDiagramText {
  title: string;
  description: string;
  nodes: Record<string, LocalizedNodeText>;
  edges: Record<string, string>;
}

const edgeKey = (edge: DiagramEdge) => `${edge.from}->${edge.to}`;

const text: Record<string, LocalizedDiagramText> = {
  "minimal-system": {
    title: "五つの要素からなるリクエスト経路",
    description:
      "Workspace から接続先の HTTP/HTTPS へ到達する経路は Gateway だけです。Gateway は OPA に判断を求め、ブローカー管理の認証情報を使う場合だけ Auth Broker と対話します。",
    nodes: {
      workspace: {
        label: "Workspace",
        detail: "外部への直接経路を持たず、プロジェクトのツールを実行します。",
      },
      gateway: {
        label: "Gateway",
        detail: "プロキシ接続を終端し、判断を執行します。",
      },
      opa: {
        label: "OPA",
        detail: "正規化された通常の HTTP effect を判断します。",
      },
      broker: {
        label: "Auth Broker",
        detail: "許可後に、ブローカー管理の認証情報を保持・処理します。",
      },
      upstream: {
        label: "Upstream（接続先）",
        detail: "Gateway からの許可済み接続だけを受け取ります。",
      },
    },
    edges: {
      "workspace->gateway": "プロキシリクエスト",
      "gateway->opa": "本文を含まない判断入力一つ",
      "opa->gateway": "許可または拒否",
      "gateway->broker": "秘密を含まない検査。認証情報処理は許可後だけ",
      "gateway->upstream": "独立した許可済み接続",
    },
  },
  "detailed-network": {
    title: "対応する Docker ネットワーク構成",
    description:
      "各 Workspace は内部のプロジェクト専用ネットワークを持ちます。Gateway はそのネットワークと外向きネットワークの両方にインターフェースを持ちます。OPA は制御経路だけを持ちます。Auth Broker は制御経路と外向きネットワークを持ちますが、プロジェクト専用ネットワークのインターフェースは持ちません。",
    nodes: {
      process: {
        label: "Workspace のプロセス",
        detail:
          "一つの内部プロジェクトネットワーク上にある runtime container 内で動きます。",
      },
      projectnet: {
        label: "プロジェクト専用の内部ネットワーク",
        detail: "Docker が提供する外部への経路はありません。",
      },
      gateway: {
        label: "Gateway",
        detail:
          "プロジェクト専用ネットワークと外向きネットワークの両方に接続する唯一のコンポーネントです。",
      },
      controlnet: {
        label: "制御ネットワーク",
        detail:
          "Gateway ↔ OPA と Gateway ↔ Auth Broker の socket／制御経路です。",
      },
      opa: {
        label: "OPA",
        detail: "ポリシー判断に外部接続は不要です。",
      },
      broker: {
        label: "Auth Broker",
        detail:
          "共有されたロック可能なサービスであり、プロジェクト専用ネットワークのインターフェースを持ちません。",
      },
      egress: {
        label: "外向きネットワーク",
        detail:
          "Gateway と Auth Broker が、宣言された役割の範囲で外部接続を持ちます。",
      },
      upstream: {
        label: "DNS と接続先",
        detail: "Workspace ではなく Gateway が到達する外部の宛先です。",
      },
    },
    edges: {
      "process->projectnet": "HTTP プロキシ通信",
      "projectnet->gateway": "インターフェースから principal を識別",
      "process->upstream": "直接経路なし",
      "gateway->opa": "判断",
      "gateway->broker": "Unix socket",
      "gateway->egress": "許可済み接続",
      "egress->upstream": "DNS/TCP/TLS",
    },
  },
  "workspace-context-cluster": {
    title: "Workspace、Context、cluster、runtime の関係",
    description:
      "project root と安定した Context ID が論理 Workspace を識別します。runtime container はそれを実現し、cluster は共有 infrastructure として動きます。",
    nodes: {
      root: {
        label: "Project root",
        detail: "現在のディレクトリから選ばれた /work/example。",
      },
      contexta: {
        label: "Context: default",
        detail: "ホスト所有の runtime、ポリシー、agent profile、認証情報設定。",
      },
      workspacea: {
        label: "Workspace A",
        detail: "論理 identity = 正規化された root + Context A。",
      },
      runtimea: {
        label: "runtime container A",
        detail:
          "reconcile される実装。論理 identity でも、寿命を決める唯一の所有者でもありません。",
      },
      contextb: {
        label: "Context: review",
        detail: "ホスト所有の別設定。",
      },
      workspaceb: {
        label: "Workspace B",
        detail: "同じ root でも Context B なら別 Workspace です。",
      },
      cluster: {
        label: "共有 cluster",
        detail: "Gateway、OPA、Auth Broker、CA、runtime のネットワーク状態。",
      },
    },
    edges: {
      "root->workspacea": "ディレクトリとの結び付き",
      "contexta->workspacea": "stable Context ID",
      "workspacea->runtimea": "整合させる",
      "root->workspaceb": "同じ root",
      "contextb->workspaceb": "異なる Context ID",
      "workspacea->cluster": "共有サービスを利用",
      "workspaceb->cluster": "共有サービスを利用",
    },
  },
  "workspace-lifecycle": {
    title: "論理 Workspace のライフサイクル",
    description:
      "exit は利用者の session を切り離します。delete は論理 Workspace と、それが所有する runtime state を削除します。container やネットワークが失われても、次の entry で整合させます。",
    nodes: {
      absent: {
        label: "存在しない (Absent)",
        detail: "root index も Workspace instance もありません。",
      },
      attached: {
        label: "接続中 (Attached)",
        detail: "entry session が Workspace を使用しています。",
      },
      detached: {
        label: "離脱済み・存在 (Detached)",
        detail:
          "identity、home、runtime state、Context との結び付き、ポリシーが残ります。",
      },
      drift: {
        label: "Runtime のずれまたは消失",
        detail:
          "論理 identity を変えずに container やネットワークを再作成できます。",
      },
    },
    edges: {
      "absent->attached": "enter / create",
      "attached->detached": "exit",
      "detached->attached": "再び enter",
      "detached->absent": "delete",
      "attached->absent": "delete --force",
      "detached->drift": "container／ネットワークの消失または recipe の変更",
      "drift->attached": "次の entry で整合させる",
    },
  },
  "tls-split": {
    title: "HTTPS は二つの検証済み TLS 接続に分かれる",
    description:
      "CONNECT は HTTP プロキシへ届きます。Gateway は Tobari CA で Workspace 側の TLS を終端し、復号した HTTP 属性を認可した後、接続先へ独立した検証済み TLS 接続を作ります。",
    nodes: {
      workspace: {
        label: "Workspace のクライアント",
        detail:
          "対応するプロキシ対応 HTTPS クライアントが Tobari CA を信頼します。",
      },
      tlsa: {
        label: "TLS 接続 A",
        detail: "Client ↔ Gateway。CONNECT の後に始まります。",
      },
      gateway: {
        label: "Gateway",
        detail: "A を終端して HTTP の判断属性を読み、OPA の判断を執行します。",
      },
      tlsb: {
        label: "TLS 接続 B",
        detail: "Gateway ↔ 接続先。通常の接続先証明書の検証を行います。",
      },
      upstream: {
        label: "HTTPS の接続先",
        detail:
          "最終 hop の平文ではなく、Gateway からの TLS 接続を受け取ります。",
      },
    },
    edges: {
      "workspace->tlsa": "CONNECT、その後に暗号化された HTTP",
      "tlsa->gateway": "Tobari が発行した leaf certificate",
      "gateway->tlsb": "許可後だけ",
      "tlsb->upstream": "検証済みの接続先 TLS",
    },
  },
  "project-principal": {
    title: "project principal の確立",
    description:
      "ホストの registry は Gateway のネットワークインターフェースを Context／project identity に結び付けます。リクエストヘッダーでこの結び付きを置き換えることはできません。",
    nodes: {
      host: {
        label: "信頼するホストのライフサイクル",
        detail: "プロジェクト専用ネットワークと Gateway の接続を作ります。",
      },
      registry: {
        label: "Principal registry",
        detail:
          "ホスト所有のインターフェース／ネットワーク → Context ID + project ID のレコード。",
      },
      network: {
        label: "Workspace 専用ネットワーク",
        detail: "対応する構成では project principal 一つに対応します。",
      },
      workspace: {
        label: "Workspace のリクエスト",
        detail: "identity に見える任意の信頼しないヘッダーを含められます。",
      },
      gateway: {
        label: "Gateway の受信インターフェース",
        detail: "受信インターフェースが選ぶ principal を検索します。",
      },
      opa: {
        label: "OPA input",
        detail: "registry から導出した Context／project field を使います。",
      },
    },
    edges: {
      "host->registry": "不可分な登録",
      "registry->gateway": "principal の検索",
      "workspace->gateway": "リクエストの byte",
      "workspace->opa": "自己申告した ID は無視",
      "gateway->opa": "信頼済み principal + 正規化済み effect",
    },
  },
  "policy-loop": {
    title: "明示的なポリシー学習ループ",
    description:
      "拒否の証拠はホストで確認できます。完全一致ルールを検証して不可分に有効化するまで、権限は増えません。再実行も利用者が明示的に行います。",
    nodes: {
      deny: {
        label: "拒否されたリクエスト",
        detail: "403。接続先への接続はありません。",
      },
      evidence: {
        label: "保持した証拠",
        detail:
          "本文や秘密情報を含まない拒否レコード。candidate ID は含みません。",
      },
      review: {
        label: "ホストでの確認",
        detail:
          "ホスト CLI が証拠を検証して不透明な reference を導出し、利用者が変更せずに選びます。",
      },
      decision: {
        label: "明示的な許可または拒否",
        detail:
          "完全一致する Context、project、destination、port、method、path の effect。",
      },
      validation: {
        label: "ポリシー全体の検証",
        detail: "集約結果が不正なら、以前の有効なポリシーを保ちます。",
      },
      activation: {
        label: "不可分な有効化",
        detail:
          "実行中の OPA が、検証済みの完全な bundle revision 一つを読みます。部分的なルールセットや自動生成 wildcard はありません。",
      },
      retry: {
        label: "明示的な再実行",
        detail: "Gateway は以前のリクエストを自動で replay しません。",
      },
    },
    edges: {
      "deny->evidence": "診断情報を保持",
      "evidence->review": "レコードを検証し candidate reference を導出",
      "review->decision": "不透明な reference で操作",
      "decision->validation": "完全一致ルールを構築",
      "validation->activation": "全 source が有効",
      "activation->retry": "操作者が時機を決める",
      "retry->deny": "新しいリクエストを再評価",
    },
  },
  "credential-boundary": {
    title: "ブローカー管理の認証情報が移動できる範囲",
    description:
      "ホストが認証情報を取得し、Auth Broker が Context vault 内で暗号化します。Workspace が受け取るのは、秘密ではない不透明なハンドルです。Gateway は許可済みで結び付きが一致するリクエストのためだけに、宣言済みの認証情報処理を行います。",
    nodes: {
      host: {
        label: "信頼するホストでの取得",
        detail: "組み込み GitHub helper または上限付きの標準入力 import。",
      },
      vault: {
        label: "暗号化された Context vault",
        detail: "installation root key の下で実物の認証情報を暗号化します。",
      },
      handle: {
        label: "プロジェクトに結び付いたハンドル",
        detail: "Workspace へ投影される不透明なレコード selector。",
      },
      workspace: {
        label: "Workspace",
        detail:
          "ハンドルは読めますが、ブローカー管理の実物の秘密情報は読めません。",
      },
      gateway: {
        label: "Gateway のポリシー許可後の経路",
        detail:
          "宣言済みの処理を一度行い、宣言された destination header だけを置き換えます。",
      },
      upstream: {
        label: "一致する HTTPS 宛先",
        detail: "許可済みリクエストの認証情報ヘッダーを受け取ります。",
      },
    },
    edges: {
      "host->vault": "保護されたホスト／Broker 入力上の秘密情報",
      "vault->handle": "秘密ではない、結び付いたレコード",
      "handle->workspace": "環境変数または完全な file projection",
      "workspace->gateway": "宣言された source header 内のハンドル",
      "vault->gateway": "許可後だけ認証情報を処理",
      "gateway->upstream": "TLS 上の宣言済み変換済みヘッダー",
      "vault->workspace": "実物の秘密情報は投影しない",
    },
  },
  "trust-boundaries": {
    title: "信頼する領域と信頼しない領域",
    description:
      "Tobari はプロジェクト、ネットワーク、ポリシー、認証情報の境界で権限を狭めます。ホスト、Docker、kernel、Gateway、OPA、Auth Broker は、信頼する強制基盤であると仮定します。",
    nodes: {
      host: {
        label: "信頼するホストの制御",
        detail:
          "CLI のライフサイクル、ポリシー確認、root key、プロバイダー認証の取得。",
      },
      services: {
        label: "信頼する強制サービス",
        detail: "Docker、Gateway、OPA、Auth Broker とその制御状態。",
      },
      workspace: {
        label: "信頼しない Workspace プロセス",
        detail:
          "選択した project root を読み書きでき、一つの Workspace 境界を共有します。",
      },
      other: {
        label: "別の Workspace／ホストファイル",
        detail: "対応する構成では mount もネットワーク到達もできません。",
      },
      upstream: {
        label: "許可済みの接続先",
        detail:
          "許可された effect で送られた、Workspace から読めるデータを受け取れます。",
      },
    },
    edges: {
      "host->services": "設定と承認",
      "services->workspace": "runtime、プロキシ、CA、不透明なハンドル",
      "workspace->other": "選択境界が通常のアクセスを遮断",
      "workspace->upstream": "許可された Gateway effect を経由する場合だけ",
      "services->upstream": "信頼する外向き接続",
    },
  },
  "state-retention": {
    title: "状態の寿命は一つの container ではなく所有関係に従う",
    description:
      "Workspace、Context、cluster、認証情報、installation state は、所有者と削除操作が異なります。",
    nodes: {
      project: {
        label: "プロジェクトのファイル",
        detail:
          "利用者所有。Workspace のライフサイクルコマンドでは削除しません。",
      },
      workspace: {
        label: "Workspace 所有の状態",
        detail: "index、instance、home、container、ネットワーク、principal。",
      },
      context: {
        label: "Context 所有の状態",
        detail:
          "manifest、runtime recipe、ポリシー source、プロバイダー設定、暗号化 vault。",
      },
      cluster: {
        label: "cluster の runtime state",
        detail:
          "共有サービス、ネットワーク、principal registry、集約 projection。",
      },
      install: {
        label: "installation state",
        detail: "root key と Gateway CA volume。",
      },
    },
    edges: {
      "workspace->workspace": "exit は保持、delete は削除",
      "cluster->cluster": "down は runtime を削除、purge は CA も削除",
      "context->context": "cluster down/purge でも保持",
      "install->install":
        "root key は down/purge 後も残り、CA は非 purge 時だけ残る",
      "project->project": "すべてのライフサイクル操作で保持",
    },
  },
  "code-layers": {
    title: "Go の四層における依存方向",
    description:
      "Domain は純粋な invariant、Application は task の解釈と最小限の port、Infrastructure は effect の実装、CLI は公開契約と依存関係の組み立てを所有します。",
    nodes: {
      cli: {
        label: "CLI composition root",
        detail: "Catalog、型付き argv、help、表示、配線。",
      },
      app: {
        label: "Application",
        detail: "use case と task 固有の port。",
      },
      domain: {
        label: "Domain",
        detail: "純粋な語彙と invariant。I/O なし。",
      },
      infra: {
        label: "Infrastructure",
        detail:
          "Docker、ファイル、プロセス、Gateway／Broker asset、外部 adapter。",
      },
    },
    edges: {
      "cli->app": "use case を呼び出す",
      "cli->infra": "具体的な adapter を組み立てる",
      "app->domain": "domain type を解釈する",
      "infra->domain": "domain／application 契約を満たす",
      "domain->infra": "外向きの依存は禁止",
    },
  },
  "image-supply": {
    title: "Runtime コンポーネントの供給経路",
    description:
      "canonical source と embedded snapshot の一致を検査します。レビュー済み workflow が digest で参照できる OCI image を build し、runtime asset が service identity を digest で固定します。",
    nodes: {
      "gateway-src": {
        label: "gateway/ の canonical source",
        detail: "Python の mitmproxy addon とテスト。",
      },
      "broker-src": {
        label: "authbroker/ の canonical source",
        detail: "Python の Broker、vault、認証取得、テスト。",
      },
      "policy-src": {
        label: "policy/ の canonical source",
        detail: "Rego source とテスト。",
      },
      snapshots: {
        label: "組み込み runtime snapshot",
        detail: "Go CLI が展開する、byte／内容を検査済みのコピー。",
      },
      images: {
        label: "GHCR OCI image",
        detail: "Gateway／Auth Broker の identity を immutable digest で固定。",
      },
      versions: {
        label: "versions.env + Compose",
        detail: "runtime で使うレビュー済みバージョン、digest、構成。",
      },
      cluster: {
        label: "検証済みの cluster 起動",
        detail: "サービスの有効化前に image を選択・検証します。",
      },
    },
    edges: {
      "gateway-src->snapshots": "source／snapshot のずれを検査",
      "broker-src->snapshots": "source／snapshot のずれを検査",
      "policy-src->snapshots": "組み込みポリシー source",
      "snapshots->images": "レビュー済み build workflow",
      "images->versions": "immutable digest による識別",
      "versions->cluster": "Compose との整合",
    },
  },
};

const localizeDiagram = (
  name: string,
  diagram: DiagramDefinition,
): DiagramDefinition => {
  const localized = text[name];
  if (!localized) throw new Error(`Missing Japanese diagram text: ${name}`);

  if (Object.keys(localized.nodes).length !== diagram.nodes.length) {
    throw new Error(`Japanese diagram node drift: ${name}`);
  }
  if (Object.keys(localized.edges).length !== diagram.edges.length) {
    throw new Error(`Japanese diagram edge drift: ${name}`);
  }

  return {
    ...diagram,
    title: localized.title,
    description: localized.description,
    nodes: diagram.nodes.map((node) => {
      const nodeText = localized.nodes[node.id];
      if (!nodeText) {
        throw new Error(
          `Missing Japanese diagram node text: ${name}/${node.id}`,
        );
      }
      return { ...node, ...nodeText };
    }),
    edges: diagram.edges.map((edge) => {
      const label = localized.edges[edgeKey(edge)];
      if (!label) {
        throw new Error(
          `Missing Japanese diagram edge text: ${name}/${edgeKey(edge)}`,
        );
      }
      return { ...edge, label };
    }),
  };
};

export const diagramsJa = Object.fromEntries(
  Object.entries(diagrams).map(([name, diagram]) => [
    name,
    localizeDiagram(name, diagram),
  ]),
) as Record<string, DiagramDefinition>;
