import { diagrams, type DiagramDefinition } from "./diagrams";

const titles: Record<string, [string, string]> = {
  "minimal-system": [
    "4 つの要素からなるリクエスト経路",
    "Workspace から接続先への通信は Gateway だけを通り、OPA が上限付きの判断を一つ返します。",
  ],
  "detailed-network": [
    "対応する Docker ネットワークの構成",
    "Workspace には専用の内部ネットワークが一つあり、ポリシーは制御ネットワークに留まり、Gateway だけが接続先へ到達します。",
  ],
  "workspace-manifest-workspace-cluster": [
    "Workspace Manifest と共有クラスター",
    "ホスト管理の Manifest が一つの永続 Workspace を選び、共有クラスターが完全な投影を強制します。",
  ],
  "workspace-lifecycle": [
    "Workspace のライフサイクル",
    "Workspace の識別情報とホームは、置き換え可能なランタイムより長く残ります。exit は delete ではありません。",
  ],
  "tls-split": [
    "HTTP と TLS の経路",
    "Gateway は Workspace 側 TLS を終端し、一つの判断を取得して、allow 後に別の検証済み upstream TLS 接続を作ります。",
  ],
  "project-principal": [
    "プロジェクトプリンシパル",
    "Gateway はリクエスト本文ではなく、カーネルが観測した送信元 endpoint とホスト登録簿から authority を導出します。",
  ],
  "policy-loop": [
    "ポリシー確認のループ",
    "拒否を上限付き evidence として保持し、ホストの明示的レビューで完全なポリシーを有効化し、再試行は意図的に行います。",
  ],
  "credential-boundary": [
    "Workspace 所有の認証境界",
    "agent CLI が一つの Workspace home 内のログイン状態を所有し、Tobari はホストの認証情報を継承せず、release credential service も公開しません。",
  ],
  "trust-boundaries": [
    "信頼境界",
    "信頼しない Workspace の入力は型付き Gateway とポリシー境界だけを通り、ホストと Docker は信頼する強制基盤です。",
  ],
  "state-retention": [
    "状態の保持",
    "Manifest、Workspace、home、runtime、共有 policy state には別々の owner と寿命があります。",
  ],
  "code-layers": [
    "コードのレイヤー",
    "Domain は外向き依存を持たず、Application と Infrastructure は内向きに依存し、CLI が composition root です。",
  ],
  "image-supply": [
    "イメージの供給",
    "正本 source、バイト一致 snapshot、固定 metadata、検査済み local image が一つのレビュー可能な供給経路を作ります。",
  ],
};

const labels: Record<string, string> = {
  Workspace: "Workspace",
  Gateway: "Gateway",
  OPA: "OPA",
  Upstream: "接続先",
  "Dedicated network": "専用ネットワーク",
  "Workspace Manifest": "Workspace Manifest",
  "Runtime resources": "ランタイムリソース",
  "Shared cluster": "共有クラスター",
  "Aggregate projection": "集約ポリシー投影",
  "Manifest binding": "Manifest の結び付き",
  "Workspace state": "Workspace 状態",
  "Workspace home": "Workspace home",
  "Work container": "作業コンテナ",
  "Attached session": "接続中セッション",
  "Request headers": "リクエストヘッダー",
  "Observed endpoint": "観測した endpoint",
  "Principal registry": "プリンシパル登録簿",
  "Workspace principal": "Workspace principal",
  "OPA input": "OPA 入力",
  Deny: "拒否",
  Evidence: "根拠",
  Review: "レビュー",
  Decision: "判断",
  Validation: "検証",
  Activation: "有効化",
  "Deliberate retry": "意図的な再試行",
  "Agent CLI": "agent CLI",
  "Workspace process": "Workspace のプロセス",
  "Trusted host state": "信頼するホスト状態",
  "Manifest configuration": "Manifest 設定",
  "Shared cluster state": "共有クラスター状態",
  CLI: "CLI",
  Application: "Application",
  Infrastructure: "Infrastructure",
  Domain: "Domain",
  "Canonical source": "正本 source",
  "Embedded snapshot": "埋め込み snapshot",
  "Pinned metadata": "固定 metadata",
  "Local OCI image": "ローカル OCI image",
  "Runtime component": "ランタイム component",
};

const details: Record<string, string> = {
  "Runs project tools without a direct external route.":
    "外部への直接経路を持たず、project tool を実行します。",
  "Derives trusted identity and enforces the decision.":
    "信頼できる identity を導出し、判断を強制します。",
  "Decides one normalized body-free HTTP effect.":
    "本文を含まない正規化 HTTP effect を一つ判断します。",
  "Receives only an authorized connection from Gateway.":
    "Gateway からの許可済み接続だけを受け取ります。",
  "No direct public route.": "公開ネットワークへの直接経路を持ちません。",
  "Carries traffic for one Workspace and one Gateway interface.":
    "一つの Workspace と一つの Gateway interface の通信だけを運びます。",
  "The only component joining project, control, and egress paths.":
    "project、control、egress の各経路へ接続する唯一の component です。",
  "Has no Workspace or egress network route.":
    "Workspace network にも egress route にも接続しません。",
  "Reached by Gateway only after allow.":
    "allow 後に Gateway からだけ到達します。",
  "Selects Runtime, policy, source access, and stable authority.":
    "Runtime、policy、source access、安定した authority を選びます。",
  "Retains one permanent Manifest binding and its own home.":
    "一つの恒久 Manifest binding と専用 home を保持します。",
  "Replaceable container and dedicated network.":
    "置き換え可能な container と専用 network です。",
  "Replaceable container and network.":
    "置き換え可能な container と network です。",
  "Gateway and OPA enforce all loaded Manifest policy.":
    "Gateway と OPA が読み込んだ全 Manifest policy を強制します。",
  "Content-addressed policy built from every Manifest.":
    "全 Manifest から構築した内容アドレス方式の policy です。",
  "Stable for the lifetime of the Workspace.":
    "Workspace の寿命を通じて安定しています。",
  "Owns the logical identity and last applied entry.":
    "論理 identity と最後に適用した entry を所有します。",
  "Persists agent-owned login and tool state.":
    "agent 所有の login と tool state を永続化します。",
  "Replaceable runtime realization.": "置き換え可能な runtime の実体です。",
  "Ends on exit without deleting logical state.":
    "exit で終了し、論理 state は削除しません。",
  "Untrusted text cannot select project authority.":
    "信頼しない text は project authority を選べません。",
  "Kernel-observed Workspace source address.":
    "カーネルが観測した Workspace source address です。",
  "Host-owned exact endpoint-to-identity mapping.":
    "ホスト所有の exact endpoint-to-identity mapping です。",
  "Exact Manifest ID and Workspace ID pair.":
    "exact Manifest ID と Workspace ID の組です。",
  "Receives the derived principal.": "導出済み principal を受け取ります。",
  "No upstream connection.": "upstream connection を作りません。",
  "Secret-free retained effect.": "秘密情報を含まない retained effect です。",
  "Trusted host inspects current authority.":
    "信頼するホストが現在の authority を確認します。",
  "Allow, deny, or no action.": "allow、deny、no action のいずれかです。",
  "Exact rule and complete aggregate.": "exact rule と完全な aggregate です。",
  "Atomic known-good publication.": "known-good state を atomic に公開します。",
  "The old request is never replayed.": "以前の request は再送しません。",
  "Persistent tool-owned login files and configuration.":
    "tool が所有する login file と設定を永続化します。",
  "Creates and reads its own login state.":
    "自身の login state を作成して読み取ります。",
  "Removes credential values from decision and audit input.":
    "credential value を decision と audit input から除外します。",
  "Decides ordinary HTTP effect without credential values.":
    "credential value を含めず通常の HTTP effect を判断します。",
  "Receives original request values only after allow.":
    "allow 後だけ元の request value を受け取ります。",
  "Untrusted code with selected-root and home access.":
    "選択した root と home へアクセスできる信頼しない code です。",
  "Issues identity and owns lifecycle and policy source.":
    "identity を発行し、lifecycle と policy source を所有します。",
  "Enforces the request path and external connection.":
    "request path と external connection を強制します。",
  "Makes the bounded decision.": "上限付きの判断を行います。",
  "External destination.": "外部の接続先です。",
  "Host-owned desired Runtime and policy source.":
    "ホスト所有の desired Runtime と policy source です。",
  "Durable logical identity and applied receipt.":
    "永続する論理 identity と applied receipt です。",
  "Writable agent-owned tool state.":
    "agent 所有の書き込み可能な tool state です。",
  "Aggregate revision and shared resource identity.":
    "aggregate revision と shared resource identity です。",
  "Catalog, rendering, and composition.":
    "Catalog、rendering、composition を担当します。",
  "Task interpretation and ports.":
    "task interpretation と port を担当します。",
  "Bounded external adapters.": "上限付き external adapter を実装します。",
  "Pure vocabulary and invariants.": "純粋な vocabulary と invariant です。",
  "The only editable Gateway and helper implementation.":
    "編集可能な Gateway と helper の実装正本です。",
  "Byte-checked runtime build input.":
    "バイト一致を検査した runtime build input です。",
  "Immutable upstream digests and component contracts.":
    "不変 upstream digest と component contract です。",
  "Built or reused only after exact validation.":
    "exact validation 後だけ build または再利用します。",
  "Starts with inspected identity and compatibility.":
    "検査済み identity と compatibility で起動します。",
};

const edgeLabels: Record<string, string> = {
  "guarded HTTP/HTTPS request": "保護された HTTP/HTTPS request",
  "one body-free decision input": "本文を含まない decision input 一つ",
  "allow or deny": "allow または deny",
  "separate authorized connection": "別の許可済み connection",
  "internal traffic": "内部通信",
  "guarded route": "保護された route",
  "decision input and result": "decision input と result",
  "authorized egress": "許可済み egress",
  "no direct route": "直接経路なし",
  "permanent identity binding": "恒久的な identity binding",
  "validated policy source": "検証済み policy source",
  "read-only complete policy": "read-only の完全な policy",
  "reconciles replaceable resources": "置き換え可能な resource を調整",
  "guarded request path": "保護された request path",
  "permanent binding": "恒久的な binding",
  "owns until delete": "delete まで所有",
  "reconcile on entry": "entry 時に調整",
  "bounded child process": "上限付き child process",
  "exit preserves state": "exit は state を保持",
  "TLS connection A": "TLS connection A",
  "normalized body-free effect": "本文を含まない正規化 effect",
  "TLS connection B after allow": "allow 後の TLS connection B",
  "exact lookup": "exact lookup",
  "host-issued identity": "host-issued identity",
  "trusted request scope": "信頼できる request scope",
  "cannot override": "上書き不可",
  "retain bounded facts": "上限付き facts を保持",
  "produce opaque reference": "opaque reference を生成",
  "explicit host choice": "明示的な host choice",
  "bind exact target": "exact target を binding",
  "complete projection passes": "完全な projection が合格",
  "new policy is active": "新しい policy が active",
  "a new request is evaluated": "新しい request を評価",
  "tool-owned login state": "tool-owned login state",
  "ordinary guarded request": "通常の保護された request",
  "credential-free decision input": "credential-free decision input",
  "original values after allow": "allow 後の元の value",
  "read-only principal projection": "read-only principal projection",
  "guarded untrusted request": "保護された信頼しない request",
  "typed decision input": "型付き decision input",
  "after one allow": "一つの allow 後",
  "stable binding": "安定した binding",
  reconciles: "調整",
  "contributes policy": "policy へ寄与",
  "invokes use case": "use case を呼び出す",
  "injects adapter": "adapter を注入",
  "depends on contracts": "contract に依存",
  "implements ports with domain types": "domain type で port を実装",
  "byte-equality gate": "バイト一致 gate",
  "reviewed build input": "レビュー済み build input",
  "pinned dependency identity": "固定 dependency identity",
  "inspect before activation": "activation 前に inspect",
};

const translated = (translations: Record<string, string>, value: string) => {
  const result = translations[value];
  if (!result)
    throw new Error(`Missing Japanese diagram translation: ${value}`);
  return result;
};

export const diagramsJa: Record<string, DiagramDefinition> = Object.fromEntries(
  Object.entries(diagrams).map(([name, diagram]) => {
    const title = titles[name];
    if (!title) throw new Error(`Missing Japanese diagram title: ${name}`);
    return [
      name,
      {
        ...diagram,
        title: title[0],
        description: title[1],
        nodes: diagram.nodes.map((diagramNode) => ({
          ...diagramNode,
          label: translated(labels, diagramNode.label),
          detail: translated(details, diagramNode.detail),
        })),
        edges: diagram.edges.map((diagramEdge) => ({
          ...diagramEdge,
          label: translated(edgeLabels, diagramEdge.label),
        })),
      },
    ];
  }),
);
