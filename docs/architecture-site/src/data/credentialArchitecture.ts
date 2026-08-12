export type SiteLocale = "en" | "ja";

export interface LocalizedText {
  en: string;
  ja: string;
}

export interface CredentialNode {
  id: string;
  label: LocalizedText;
  role: LocalizedText;
  detail: LocalizedText;
  kind: "trusted" | "untrusted" | "control" | "secret" | "external";
}

export interface CredentialRoute {
  id: string;
  from: string;
  to: string;
  label: LocalizedText;
  kind: "control" | "handle" | "metadata" | "secret" | "network";
  path: string;
  bidirectional?: boolean;
}

export interface CredentialScenario {
  id: string;
  label: LocalizedText;
  summary: LocalizedText;
  routes: string[];
  sent: LocalizedText;
  withheld: LocalizedText;
  result: LocalizedText;
  failure: LocalizedText;
}

export const credentialNodes: CredentialNode[] = [
  {
    id: "host-cli",
    label: { en: "Trusted-host CLI", ja: "信頼するホスト上の CLI" },
    role: { en: "Static acquisition control", ja: "静的認証情報の取得" },
    detail: {
      en: "Runs only the fixed GitHub helper; owner providers import one bounded secret from protected stdin.",
      ja: "固定 GitHub ヘルパーだけを実行します。owner provider は保護された標準入力から上限付きの秘密情報を一つインポートします。",
    },
    kind: "trusted",
  },
  {
    id: "workspace",
    label: { en: "Workspace", ja: "Workspace" },
    role: { en: "Untrusted process boundary", ja: "信頼しないプロセスの境界" },
    detail: {
      en: "Tool-native login stays in its writable home; brokered providers project only a bound opaque handle.",
      ja: "ツール自身のログイン状態は書き込み可能なホームに残ります。ブローカー方式では結び付いた不透明ハンドルだけを配置します。",
    },
    kind: "untrusted",
  },
  {
    id: "opa",
    label: { en: "OPA", ja: "OPA" },
    role: { en: "Authorization decision", ja: "通信許可の判断" },
    detail: {
      en: "Decides one normalized exact HTTP effect without body, handle, or credential values.",
      ja: "本文、ハンドル、認証情報の値を除いた正規化済みの完全一致 HTTP 通信を判断します。",
    },
    kind: "control",
  },
  {
    id: "gateway",
    label: { en: "Gateway", ja: "Gateway" },
    role: { en: "Network enforcement", ja: "通信経路の制御" },
    detail: {
      en: "Derives the principal, removes a recognized handle, asks OPA, then performs at most one exact replacement.",
      ja: "プリンシパルを導出し、認識したハンドルを除去して OPA に問い合わせ、完全一致する置換を最大一回だけ行います。",
    },
    kind: "control",
  },
  {
    id: "broker",
    label: { en: "Auth Broker", ja: "Auth Broker" },
    role: { en: "Static credential boundary", ja: "静的認証情報の境界" },
    detail: {
      en: "Introspects non-secret binding metadata before policy and resolves one same-revision static secret only after allow.",
      ja: "ポリシー判断前は秘密を含まない結び付きだけを確認し、許可後に同じリビジョンの静的な秘密情報を一度だけ解決します。",
    },
    kind: "secret",
  },
  {
    id: "vault",
    label: { en: "Encrypted Context vault", ja: "暗号化された Context 保管庫" },
    role: { en: "Persistent static secret state", ja: "永続する静的秘密情報" },
    detail: {
      en: "Stores one static primary secret per Context/provider plus revisions and raw handles.",
      ja: "Context とプロバイダーごとの静的な秘密情報一つ、リビジョン、未加工ハンドルを保存します。",
    },
    kind: "secret",
  },
  {
    id: "provider-login",
    label: { en: "GitHub device login", ja: "GitHub デバイスログイン" },
    role: { en: "Only built-in acquisition", ja: "唯一の組み込み取得処理" },
    detail: {
      en: "Reached only by the fixed trusted-host GitHub CLI helper with manual browser fallback.",
      ja: "信頼するホスト上の固定 GitHub CLI ヘルパーからだけ到達し、ブラウザーを開けない場合は手動手順を残します。",
    },
    kind: "external",
  },
  {
    id: "upstream",
    label: { en: "Authorized upstream", ja: "許可された接続先" },
    role: { en: "Application destination", ja: "アプリケーションの宛先" },
    detail: {
      en: "Receives one separately connected request only after the preset guardrail and exact policy allow.",
      ja: "プリセットのガードレールと完全一致ポリシーが許可した後に、別接続で送る一回のリクエストだけを受け取ります。",
    },
    kind: "external",
  },
];

export const credentialRoutes: CredentialRoute[] = [
  {
    id: "acquire-provider",
    from: "host-cli",
    to: "provider-login",
    label: { en: "fixed GitHub device flow", ja: "固定 GitHub デバイスフロー" },
    kind: "network",
    path: "M 225 105 C 430 42, 660 42, 790 105",
    bidirectional: true,
  },
  {
    id: "commit-control",
    from: "host-cli",
    to: "broker",
    label: {
      en: "bounded static-secret commit",
      ja: "上限付き静的秘密情報の確定",
    },
    kind: "control",
    path: "M 205 145 C 245 265, 300 470, 420 515",
  },
  {
    id: "vault-state",
    from: "broker",
    to: "vault",
    label: { en: "encrypted static record", ja: "暗号化された静的レコード" },
    kind: "secret",
    path: "M 500 575 L 500 630",
    bidirectional: true,
  },
  {
    id: "workspace-proxy",
    from: "workspace",
    to: "gateway",
    label: { en: "opaque handle + HTTP", ja: "不透明ハンドル + HTTP" },
    kind: "handle",
    path: "M 225 355 L 410 355",
  },
  {
    id: "policy",
    from: "gateway",
    to: "opa",
    label: { en: "exact body-free effect", ja: "本文を除いた完全一致通信" },
    kind: "metadata",
    path: "M 500 300 L 500 185",
    bidirectional: true,
  },
  {
    id: "broker-introspect",
    from: "gateway",
    to: "broker",
    label: {
      en: "introspect, then static resolve",
      ja: "結び付きを確認し、静的に解決",
    },
    kind: "control",
    path: "M 500 410 L 500 495",
    bidirectional: true,
  },
  {
    id: "upstream-request",
    from: "gateway",
    to: "upstream",
    label: { en: "one authorized request", ja: "許可済みリクエスト一回" },
    kind: "network",
    path: "M 590 355 L 775 355",
  },
];

export const credentialScenarios: CredentialScenario[] = [
  {
    id: "acquisition",
    label: { en: "Acquire on trusted host", ja: "信頼するホストで取得" },
    summary: {
      en: "The fixed GitHub helper or protected stdin import commits one static Context credential. Workspace is not part of acquisition.",
      ja: "固定 GitHub ヘルパーまたは保護された標準入力インポートが、Context の静的認証情報一つを確定します。Workspace は取得処理に参加しません。",
    },
    routes: ["acquire-provider", "commit-control", "vault-state"],
    sent: {
      en: "Fixed GitHub argv or bounded stdin bytes, then one static credential over Broker control.",
      ja: "固定 GitHub argv または上限付き標準入力、その後 Auth Broker 制御経路へ静的認証情報一つ。",
    },
    withheld: {
      en: "No host home, executable owner-manifest field, primary credential, or provider CLI enters Workspace or the Broker image.",
      ja: "ホストホーム、owner manifest の実行項目、実物の認証情報、プロバイダー CLI は Workspace や Auth Broker イメージへ入りません。",
    },
    result: {
      en: "Encrypted Context-owned static credential state; still no network allow rule.",
      ja: "Context 所有の静的認証情報が暗号化保存されますが、通信許可ルールは増えません。",
    },
    failure: {
      en: "The previous record remains unchanged; no partial state is projected.",
      ja: "以前のレコードは変わらず、不完全な状態は配置されません。",
    },
  },
  {
    id: "static",
    label: { en: "Static header provider", ja: "静的ヘッダープロバイダー" },
    summary: {
      en: "Gateway removes and introspects the handle, OPA decides the exact effect beneath the preset guardrail, then Broker resolves one static secret after allow.",
      ja: "Gateway がハンドルを除去して結び付きを確認し、プリセットのガードレール下で OPA が完全一致通信を判断します。許可後に Auth Broker が静的な秘密情報を一度だけ解決します。",
    },
    routes: [
      "workspace-proxy",
      "broker-introspect",
      "policy",
      "vault-state",
      "upstream-request",
    ],
    sent: {
      en: "Handle and trusted binding metadata before policy; one request-local static value after allow.",
      ja: "ポリシー判断前はハンドルと信頼済み結び付き、許可後はリクエスト専用の静的な値一つ。",
    },
    withheld: {
      en: "OPA never sees handle, body, revision, or secret; Workspace never receives the primary secret.",
      ja: "OPA はハンドル、本文、リビジョン、秘密情報を見ません。Workspace は実物の秘密情報を受け取りません。",
    },
    result: {
      en: "Gateway replaces only the declared destination header and makes one upstream attempt.",
      ja: "Gateway は宣言済みの宛先ヘッダーだけを置換し、接続先へ一回だけ接続します。",
    },
    failure: {
      en: "Invalid handle, terminal guardrail, or OPA deny stops before secret resolution and upstream; no fallback is attempted.",
      ja: "不正ハンドル、終端ガードレール、OPA 拒否では秘密情報の解決と接続先通信より前に停止し、別方式へフォールバックしません。",
    },
  },
];

export function textFor(value: LocalizedText, locale: SiteLocale): string {
  return value[locale];
}
