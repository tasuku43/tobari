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

const edgeKey = (edge: DiagramEdge) => edge.id ?? `${edge.from}->${edge.to}`;

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
        detail: "正規化された HTTP 通信を許可するか判断します。",
      },
      broker: {
        label: "Auth Broker",
        detail:
          "認証情報を保管し、解決・更新・署名は通信が許可された後だけ行います。",
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
    title: "対応する Docker ネットワーク上での通信経路",
    description:
      "リクエストはプロジェクト専用の内部ネットワークから始まります。外向きネットワークにも接続するのは Gateway だけで、ポリシー判断と認証情報の制御には別の制御ネットワークを使います。",
    nodes: {
      process: {
        label: "Workspace のプロセス",
        detail:
          "一つのプロジェクト専用ネットワークに接続したランタイム内で動きます。",
      },
      projectnet: {
        label: "プロジェクト専用の内部ネットワーク",
        detail:
          "Workspace のプロキシ通信を運びます。Docker が提供する外部への経路はありません。",
      },
      gateway: {
        label: "Gateway",
        detail:
          "プロジェクト専用ネットワークと外向きネットワークの両方に接続する唯一のコンポーネントです。",
      },
      controlnet: {
        label: "制御ネットワーク",
        detail:
          "Gateway と OPA、Gateway と Auth Broker の間の通信だけを運びます。",
      },
      opa: {
        label: "OPA",
        detail: "外部接続を使わずにポリシーを判断します。",
      },
      broker: {
        label: "Auth Broker",
        detail:
          "制御ネットワークと外向きネットワークを使いますが、プロジェクト専用ネットワークには接続しません。",
      },
      egress: {
        label: "外向きネットワーク",
        detail:
          "Gateway または Auth Broker が所有する、許可済みの外部接続を運びます。",
      },
      upstream: {
        label: "DNS と接続先",
        detail: "Workspace ではなく Gateway が到達する外部の宛先です。",
      },
    },
    edges: {
      "process->projectnet":
        "HTTP/HTTPS のプロキシ通信がプロジェクト専用ネットワークへ入る",
      "projectnet->gateway":
        "Gateway がプロジェクト固有のインターフェースでリクエストを受け取る",
      "process->upstream": "サポート対象の直接経路は存在しない",
      "gateway->controlnet":
        "ポリシー判断と認証情報の制御通信は制御ネットワーク内に留まる",
      "controlnet->opa":
        "本文を含まないポリシー入力と、許可または拒否の判断一つ",
      "controlnet->broker":
        "秘密を含まないハンドル検査。認証情報処理は許可後だけ",
      "gateway->egress": "Gateway は許可後にだけ外部接続を作る",
      "egress->upstream": "DNS、TCP、TLS で選択した接続先へ到達する",
    },
  },
  "workspace-context-cluster": {
    title: "Workspace、Context、クラスター、ランタイムの関係",
    description:
      "プロジェクトルートと安定した Context ID の組み合わせが、論理 Workspace を識別します。ランタイムコンテナはその実行環境であり、クラスターは複数の Workspace から共有される基盤です。",
    nodes: {
      root: {
        label: "プロジェクトルート",
        detail: "現在のディレクトリから選ばれた /work/example。",
      },
      contexta: {
        label: "Context: default",
        detail:
          "ホストが管理するランタイム、ポリシー、エージェントプロファイル、認証情報の設定。",
      },
      workspacea: {
        label: "Workspace A",
        detail:
          "正規化されたプロジェクトルートと Context A が、論理的な識別情報になります。",
      },
      runtimea: {
        label: "ランタイムコンテナ A",
        detail:
          "必要に応じて作り直される実行環境です。Workspace の識別情報でも、寿命を決める主体でもありません。",
      },
      contextb: {
        label: "Context: review",
        detail: "ホスト所有の別設定。",
      },
      workspaceb: {
        label: "Workspace B",
        detail:
          "同じプロジェクトルートでも、Context B を使えば別の Workspace になります。",
      },
      cluster: {
        label: "共有クラスター",
        detail:
          "Gateway、OPA、Auth Broker、CA、ランタイム用ネットワークを共有します。",
      },
    },
    edges: {
      "root->workspacea": "ディレクトリに結び付ける",
      "contexta->workspacea": "安定した Context ID",
      "workspacea->runtimea": "実行環境を整合させる",
      "root->workspaceb": "同じプロジェクトルート",
      "contextb->workspaceb": "異なる Context ID",
      "workspacea->cluster": "共有サービスを利用",
      "workspaceb->cluster": "共有サービスを利用",
    },
  },
  "workspace-lifecycle": {
    title: "論理 Workspace のライフサイクル",
    description:
      "exit で終了するのは利用者のセッションだけです。delete は論理 Workspace と、その Workspace が所有するランタイム状態を削除します。コンテナやネットワークが失われても、次に入るときに作り直せます。",
    nodes: {
      absent: {
        label: "存在しない (Absent)",
        detail:
          "ルートインデックスも Workspace のインスタンス状態もありません。",
      },
      attached: {
        label: "接続中 (Attached)",
        detail: "Workspace に入ったセッションが動いています。",
      },
      detached: {
        label: "離脱済み・存在 (Detached)",
        detail:
          "識別情報、ホーム、ランタイム状態、Context との結び付き、ポリシーが残ります。",
      },
      drift: {
        label: "ランタイムのずれまたは消失",
        detail:
          "論理 Workspace の識別情報を変えずに、コンテナやネットワークを再作成できます。",
      },
    },
    edges: {
      "absent->attached": "enter / create",
      "attached->detached": "exit",
      "detached->attached": "再び enter",
      "detached->absent": "delete",
      "attached->absent": "delete --force",
      "detached->drift": "コンテナ／ネットワークの消失またはレシピの変更",
      "drift->attached": "次に入るときに整合させる",
    },
  },
  "tls-split": {
    title: "一つの HTTPS リクエストが二つの TLS セッションを通る",
    description:
      "クライアントは最初に Gateway との TLS セッションを作ります。Gateway は復号した HTTP リクエストを認可し、許可された後にだけ、接続先へ別の検証済み TLS セッションを作ります。",
    nodes: {
      workspace: {
        label: "Workspace のクライアント",
        detail:
          "HTTP プロキシを使い、Gateway 側の TLS セッションでは Tobari CA を信頼します。",
      },
      gateway: {
        label: "Gateway",
        detail:
          "クライアント側の TLS を終端し、接続先への通信を所有して、ポリシー判断を執行します。",
      },
      opa: {
        label: "OPA",
        detail:
          "本文を受け取らず、正規化した HTTP 通信を許可するか判断します。",
      },
      upstream: {
        label: "HTTPS の接続先",
        detail:
          "Gateway が別に作り、証明書を検証した TLS セッションを受け取ります。",
      },
    },
    edges: {
      connect: "CONNECT example.com:443 が HTTP プロキシへ届く",
      "workspace-tls":
        "Tobari が発行したサーバー証明書で TLS セッション 1 を開始する",
      "policy-query":
        "Gateway がスキーム、ホスト、ポート、メソッド、パスを送り、本文は送らない",
      "policy-result": "OPA が許可または拒否の判断を一つ返す",
      "upstream-connect":
        "許可後に Gateway が接続先を名前解決し、TCP 接続を作る",
      "upstream-tls": "TLS セッション 2 で接続先の証明書を独立して検証する",
      "https-forward":
        "セッション 2 上で HTTP を転送し、レスポンスは二つのセッションを通って戻る",
    },
  },
  "project-principal": {
    title: "プロジェクトプリンシパルの確立",
    description:
      "ホストが管理する登録情報は、Gateway のネットワークインターフェースを Context ID とプロジェクト ID に結び付けます。リクエストヘッダーを書き換えても、この結び付きは変わりません。",
    nodes: {
      host: {
        label: "信頼するホストのライフサイクル",
        detail: "プロジェクト専用ネットワークと Gateway の接続を作ります。",
      },
      registry: {
        label: "プリンシパル登録情報",
        detail:
          "ホストが管理する、インターフェース／ネットワークと Context ID／プロジェクト ID の対応記録。",
      },
      network: {
        label: "Workspace 専用ネットワーク",
        detail:
          "サポート対象の構成では、一つのプロジェクトプリンシパルに対応します。",
      },
      workspace: {
        label: "Workspace のリクエスト",
        detail:
          "識別情報のように見えるヘッダーを自由に送れますが、その値は信頼されません。",
      },
      gateway: {
        label: "Gateway の受信インターフェース",
        detail: "受信インターフェースに対応するプリンシパルを検索します。",
      },
      opa: {
        label: "OPA への入力",
        detail:
          "登録情報から導出した Context ID とプロジェクト ID を使います。",
      },
    },
    edges: {
      "host->registry": "不可分な登録",
      "host->network": "プロジェクト専用の内部ネットワークを作成",
      "network->gateway": "このプロジェクトを示す受信インターフェースを選択",
      "registry->gateway": "プリンシパルを検索",
      "workspace->gateway": "リクエストデータ",
      "workspace->opa": "自己申告した ID は無視",
      "gateway->opa": "信頼済みプリンシパル + 正規化済みの通信情報",
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
        detail: "本文や秘密情報を含まない拒否記録です。候補 ID は含みません。",
      },
      review: {
        label: "ホストでの確認",
        detail:
          "ホスト側の CLI が証拠を検証して不透明な参照を生成し、利用者はその値を変更せずに選びます。",
      },
      decision: {
        label: "明示的な許可または拒否",
        detail:
          "Context、プロジェクト、宛先、ポート、HTTP メソッド、パスが完全に一致する通信だけが対象です。",
      },
      validation: {
        label: "ポリシー全体の検証",
        detail: "集約結果が不正なら、以前の有効なポリシーを保ちます。",
      },
      activation: {
        label: "不可分な有効化",
        detail:
          "実行中の OPA は、検証済みの完全なポリシーバンドルを一つのリビジョンとして読み込みます。ルールの一部だけを反映したり、ワイルドカードを自動生成したりはしません。",
      },
      retry: {
        label: "明示的な再実行",
        detail: "Gateway は以前のリクエストを自動では再送しません。",
      },
    },
    edges: {
      "deny->evidence": "診断情報を保持",
      "evidence->review": "記録を検証し、候補を指す参照を生成",
      "review->decision": "不透明な参照を指定して操作",
      "decision->validation": "完全一致ルールを構築",
      "validation->activation": "すべてのポリシーソースが有効",
      "activation->retry": "操作者が時機を決める",
      "retry->deny": "新しいリクエストを再評価",
    },
  },
  "credential-boundary": {
    title: "ブローカー管理の認証情報が移動できる範囲",
    description:
      "ホストが認証情報を取得し、Auth Broker が Context の保管庫内で暗号化します。Workspace が受け取るのは、秘密ではない不透明なハンドルです。Gateway は許可済みで結び付きが一致するリクエストのためだけに、宣言済みの認証情報処理を行います。",
    nodes: {
      host: {
        label: "信頼するホストでの取得",
        detail:
          "組み込みの GitHub ヘルパー、または長さを制限した標準入力から取得します。",
      },
      vault: {
        label: "暗号化された Context 保管庫",
        detail:
          "インストール単位のルートキーを使って、実物の認証情報を暗号化します。",
      },
      handle: {
        label: "プロジェクトに結び付いたハンドル",
        detail: "Workspace へ渡す、認証情報レコードを指す不透明な値です。",
      },
      workspace: {
        label: "Workspace",
        detail:
          "ハンドルは読めますが、ブローカー管理の実物の秘密情報は読めません。",
      },
      gateway: {
        label: "Gateway のポリシー許可後の経路",
        detail:
          "宣言済みの認証情報処理を一度だけ行い、指定された宛先ヘッダーだけを置き換えます。",
      },
      upstream: {
        label: "一致する HTTPS 宛先",
        detail: "許可済みリクエストの認証情報ヘッダーを受け取ります。",
      },
    },
    edges: {
      "host->vault": "保護されたホスト／Auth Broker の入力",
      "vault->handle": "秘密ではない、結び付いたレコード",
      "handle->workspace": "環境変数またはファイル全体への投影",
      "workspace->gateway": "宣言された送信元ヘッダー内のハンドル",
      "vault->gateway": "許可後だけ認証情報を処理",
      "gateway->upstream": "TLS 上の宣言済み変換済みヘッダー",
      "vault->workspace": "実物の秘密情報は投影しない",
    },
  },
  "trust-boundaries": {
    title: "信頼する領域と信頼しない領域",
    description:
      "Tobari は、プロジェクト、ネットワーク、ポリシー、認証情報の境界で権限を絞ります。ホスト、Docker、カーネル、Gateway、OPA、Auth Broker は、境界を実施する信頼済みの基盤として扱います。",
    nodes: {
      host: {
        label: "信頼するホストの制御",
        detail:
          "CLI のライフサイクル操作、ポリシーの確認、ルートキー、プロバイダー認証情報の取得。",
      },
      services: {
        label: "信頼する強制サービス",
        detail: "Docker、Gateway、OPA、Auth Broker とその制御状態。",
      },
      workspace: {
        label: "信頼しない Workspace プロセス",
        detail:
          "選択したプロジェクトルートを読み書きでき、同じ Workspace の境界を共有します。",
      },
      other: {
        label: "別の Workspace／ホストファイル",
        detail:
          "サポート対象の構成では、マウントもネットワーク経由の到達もできません。",
      },
      upstream: {
        label: "許可済みの接続先",
        detail:
          "許可された通信を通じて、Workspace から読み取れるデータを受け取れます。",
      },
    },
    edges: {
      "host->services": "設定と承認",
      "services->workspace": "ランタイム、プロキシ、CA、不透明なハンドル",
      "workspace->other": "選択境界が通常のアクセスを遮断",
      "workspace->upstream": "Gateway で許可された通信を経由する場合だけ",
      "services->upstream": "信頼する外向き接続",
    },
  },
  "state-retention": {
    title: "状態の寿命は、一つのコンテナではなく所有関係で決まる",
    description:
      "Workspace、Context、クラスター、認証情報、インストール全体の状態は、それぞれ所有者と削除する操作が異なります。",
    nodes: {
      project: {
        label: "プロジェクトのファイル",
        detail:
          "利用者所有。Workspace のライフサイクルコマンドでは削除しません。",
      },
      workspace: {
        label: "Workspace 所有の状態",
        detail:
          "インデックス、インスタンス状態、ホーム、コンテナ、ネットワーク、プリンシパル。",
      },
      context: {
        label: "Context 所有の状態",
        detail:
          "マニフェスト、ランタイムレシピ、ポリシーソース、プロバイダー設定、暗号化された保管庫。",
      },
      cluster: {
        label: "クラスターのランタイム状態",
        detail:
          "共有サービス、ネットワーク、プリンシパル登録情報、集約済みの投影。",
      },
      install: {
        label: "インストール全体の状態",
        detail: "ルートキーと Gateway CA ボリューム。",
      },
    },
    edges: {
      "workspace->workspace": "exit は保持、delete は削除",
      "cluster->cluster": "down はランタイムを削除、purge は CA も削除",
      "context->context": "cluster down／purge の後も保持",
      "install->install":
        "ルートキーは down／purge 後も残り、CA は purge しない場合だけ残る",
      "project->project": "すべてのライフサイクル操作で保持",
    },
  },
  "code-layers": {
    title: "Go の四層における依存方向",
    description:
      "Domain は純粋な不変条件、Application はタスクの解釈と最小限のポート、Infrastructure は外部作用の実装、CLI は公開契約と依存関係の組み立てを受け持ちます。",
    nodes: {
      cli: {
        label: "CLI（構成の起点）",
        detail: "Catalog、型付き argv、ヘルプ、表示、依存関係の配線。",
      },
      app: {
        label: "Application",
        detail: "ユースケースと、タスク固有の最小限のポート。",
      },
      domain: {
        label: "Domain",
        detail: "純粋な語彙と不変条件。I/O は行いません。",
      },
      infra: {
        label: "Infrastructure",
        detail:
          "Docker、ファイル、プロセス、Gateway／Auth Broker の資材、外部アダプター。",
      },
    },
    edges: {
      "cli->app": "ユースケースを呼び出す",
      "cli->infra": "具体的なアダプターを組み立てる",
      "app->domain": "ドメインの型を解釈する",
      "infra->domain": "Domain／Application の契約を満たす",
      "domain->infra": "外向きの依存は禁止",
    },
  },
  "image-supply": {
    title: "ランタイムコンポーネントの供給経路",
    description:
      "正本のソースと組み込みスナップショットが一致することを検査します。レビュー済みのワークフローで OCI イメージをビルドし、ランタイム資材はサービスの実体を不変のダイジェストで固定します。",
    nodes: {
      "gateway-src": {
        label: "gateway/ の正本ソース",
        detail: "Python で書かれた mitmproxy アドオンとテスト。",
      },
      "broker-src": {
        label: "authbroker/ の正本ソース",
        detail: "Python で書かれた Auth Broker、保管庫、認証取得処理、テスト。",
      },
      "policy-src": {
        label: "policy/ の正本ソース",
        detail: "Rego のソースとテスト。",
      },
      snapshots: {
        label: "組み込みランタイムスナップショット",
        detail: "Go CLI が展開する、バイト単位で内容を検査済みのコピー。",
      },
      images: {
        label: "GHCR の OCI イメージ",
        detail: "Gateway／Auth Broker の実体を不変のダイジェストで固定。",
      },
      versions: {
        label: "versions.env + Compose",
        detail:
          "ランタイムで使うレビュー済みのバージョン、ダイジェスト、構成。",
      },
      cluster: {
        label: "検証済みのクラスター起動",
        detail: "サービスを有効にする前にイメージを選び、検証します。",
      },
    },
    edges: {
      "gateway-src->snapshots": "ソース／スナップショットのずれを検査",
      "broker-src->snapshots": "ソース／スナップショットのずれを検査",
      "policy-src->snapshots": "組み込みポリシーソース",
      "snapshots->images": "レビュー済みのビルドワークフロー",
      "images->versions": "不変のダイジェストで識別",
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
