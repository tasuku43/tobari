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
    label: { en: "Trusted-host CLI", ja: "信頼ホストの CLI" },
    role: { en: "Acquisition control", ja: "取得の制御" },
    detail: {
      en: "Runs fixed gh, aws, or pup login drivers; import reads protected stdin.",
      ja: "固定された gh、aws、pup のログインドライバーを実行します。インポートは保護された標準入力だけを読みます。",
    },
    kind: "trusted",
  },
  {
    id: "workspace",
    label: { en: "Workspace", ja: "Workspace" },
    role: { en: "Untrusted process boundary", ja: "信頼しないプロセス境界" },
    detail: {
      en: "Receives only a Context/project-bound opaque handle for brokered providers.",
      ja: "ブローカー方式では、Context とプロジェクトに結び付いた不透明なハンドルだけを受け取ります。",
    },
    kind: "untrusted",
  },
  {
    id: "companion",
    label: {
      en: "Host credential companion",
      ja: "ホスト認証情報コンパニオン",
    },
    role: { en: "AWS post-policy operation", ja: "AWS のポリシー許可後処理" },
    detail: {
      en: "A private resident Tobari process. It performs only the compiled AWS credential export.",
      ja: "常駐する非公開の Tobari プロセスです。組み込み済みの AWS 認証情報エクスポートだけを実行します。",
    },
    kind: "trusted",
  },
  {
    id: "opa",
    label: { en: "OPA", ja: "OPA" },
    role: { en: "Authorization decision", ja: "通信許可の判断" },
    detail: {
      en: "Decides one normalized HTTP effect without body, handle, or credential values.",
      ja: "本文、ハンドル、認証情報の値を含まない、正規化済みの HTTP effect を判断します。",
    },
    kind: "control",
  },
  {
    id: "gateway",
    label: { en: "Gateway", ja: "Gateway" },
    role: { en: "Network enforcement", ja: "ネットワークの強制点" },
    detail: {
      en: "Derives the principal, removes handles, asks OPA, and controls every upstream attempt.",
      ja: "principal を導出し、ハンドルを除去して OPA に問い合わせ、接続先へのすべての試行を制御します。",
    },
    kind: "control",
  },
  {
    id: "broker",
    label: { en: "Auth Broker", ja: "Auth Broker" },
    role: { en: "Credential boundary", ja: "認証情報の境界" },
    detail: {
      en: "Introspects before policy; resolves, refreshes, or signs only after allow.",
      ja: "ポリシー判断前は秘密を含まない検査だけを行い、解決、更新、署名は許可後にだけ行います。",
    },
    kind: "secret",
  },
  {
    id: "vault",
    label: {
      en: "Encrypted Context vault",
      ja: "暗号化された Context vault",
    },
    role: { en: "Persistent secret state", ja: "永続する秘密情報の状態" },
    detail: {
      en: "Stores typed static secrets, opaque AWS state, Datadog OAuth state, revisions, and raw handles.",
      ja: "型付きの静的な秘密情報、AWS の不透明な状態、Datadog OAuth の状態、リビジョン、未加工のハンドルを保存します。",
    },
    kind: "secret",
  },
  {
    id: "provider-login",
    label: {
      en: "Provider login endpoints",
      ja: "プロバイダーのログイン先",
    },
    role: { en: "External acquisition", ja: "外部での認証取得" },
    detail: {
      en: "GitHub device login, AWS login, or Datadog OAuth reached by fixed trusted-host drivers.",
      ja: "信頼するホスト側の固定ドライバーが、GitHub device login、AWS login、または Datadog OAuth へ接続します。",
    },
    kind: "external",
  },
  {
    id: "upstream",
    label: { en: "Authorized upstream", ja: "許可された接続先" },
    role: { en: "Application destination", ja: "アプリケーションの宛先" },
    detail: {
      en: "Receives one separately connected request only after OPA allow and credential preparation.",
      ja: "OPA の許可と認証情報の準備が完了した後に、Gateway が別の接続で送る 1 回のリクエストだけを受け取ります。",
    },
    kind: "external",
  },
  {
    id: "datadog-token",
    label: {
      en: "Datadog token endpoint",
      ja: "Datadog トークンエンドポイント",
    },
    role: { en: "Fixed refresh destination", ja: "固定された更新先" },
    detail: {
      en: "Exact US1 HTTPS endpoint; no ambient proxy, redirect, or alternate host.",
      ja: "US1 の完全一致する HTTPS エンドポイントです。環境由来のプロキシ、リダイレクト、別ホストは使いません。",
    },
    kind: "external",
  },
];

export const credentialRoutes: CredentialRoute[] = [
  {
    id: "acquire-provider",
    from: "host-cli",
    to: "provider-login",
    label: { en: "fixed login flow", ja: "固定されたログイン処理" },
    kind: "network",
    path: "M 225 105 C 430 42, 660 42, 790 105",
    bidirectional: true,
  },
  {
    id: "commit-control",
    from: "host-cli",
    to: "broker",
    label: { en: "typed state commit", ja: "型付き状態の確定" },
    kind: "control",
    path: "M 205 145 C 245 265, 300 470, 420 515",
  },
  {
    id: "vault-state",
    from: "broker",
    to: "vault",
    label: { en: "encrypted record", ja: "暗号化されたレコード" },
    kind: "secret",
    path: "M 500 575 L 500 630",
    bidirectional: true,
  },
  {
    id: "workspace-proxy",
    from: "workspace",
    to: "gateway",
    label: { en: "opaque handle + HTTP", ja: "不透明なハンドル + HTTP" },
    kind: "handle",
    path: "M 225 355 L 410 355",
  },
  {
    id: "policy",
    from: "gateway",
    to: "opa",
    label: { en: "body-free effect", ja: "本文を含まない effect" },
    kind: "metadata",
    path: "M 500 300 L 500 185",
    bidirectional: true,
  },
  {
    id: "broker-introspect",
    from: "gateway",
    to: "broker",
    label: {
      en: "introspect, then post-allow action",
      ja: "秘密を含まない検査、許可後の処理",
    },
    kind: "control",
    path: "M 500 410 L 500 495",
    bidirectional: true,
  },
  {
    id: "upstream-request",
    from: "gateway",
    to: "upstream",
    label: { en: "one authorized request", ja: "許可済みリクエスト 1 回" },
    kind: "network",
    path: "M 590 355 L 775 355",
  },
  {
    id: "aws-companion",
    from: "broker",
    to: "companion",
    label: {
      en: "authenticated fixed operation",
      ja: "認証済みの固定処理",
    },
    kind: "secret",
    path: "M 420 535 C 340 555, 280 575, 225 605",
    bidirectional: true,
  },
  {
    id: "aws-provider",
    from: "companion",
    to: "provider-login",
    label: {
      en: "AWS CLI provider HTTPS",
      ja: "AWS CLI からプロバイダーへの HTTPS",
    },
    kind: "network",
    path: "M 205 565 C 330 430, 620 225, 790 145",
    bidirectional: true,
  },
  {
    id: "datadog-refresh",
    from: "broker",
    to: "datadog-token",
    label: { en: "exact OAuth refresh", ja: "完全一致する OAuth 更新" },
    kind: "secret",
    path: "M 580 535 C 655 555, 720 575, 790 605",
    bidirectional: true,
  },
];

export const credentialScenarios: CredentialScenario[] = [
  {
    id: "acquisition",
    label: { en: "Acquire on trusted host", ja: "信頼するホストで取得" },
    summary: {
      en: "Interactive provider login stays on the trusted host. The CLI commits typed state through Broker control; Workspace is not part of acquisition.",
      ja: "対話型のプロバイダーログインは信頼するホスト内で完結します。CLI は Auth Broker の制御経路で型付き状態を確定し、Workspace は取得処理に参加しません。",
    },
    routes: ["acquire-provider", "commit-control", "vault-state"],
    sent: {
      en: "Fixed provider argv, bounded projected output, then typed credential state over Broker control.",
      ja: "固定されたプロバイダー argv、上限付きの表示出力、その後 Auth Broker の制御経路へ渡す型付き認証情報状態。",
    },
    withheld: {
      en: "No host home, provider CLI, primary credential, or login callback service enters Workspace or the Broker image.",
      ja: "ホストの home、プロバイダー CLI、実物の認証情報、ログイン callback service は、Workspace や Auth Broker image に入りません。",
    },
    result: {
      en: "Encrypted Context-owned credential state; still no network allow rule.",
      ja: "Context 所有の認証情報状態が暗号化されて保存されますが、通信の許可ルールは増えません。",
    },
    failure: {
      en: "The previous record remains unchanged; no partial state is projected.",
      ja: "以前のレコードは変わらず、不完全な状態は投影されません。",
    },
  },
  {
    id: "static",
    label: { en: "Static header provider", ja: "静的ヘッダープロバイダー" },
    summary: {
      en: "Gateway strips and introspects the handle, OPA decides the ordinary effect, then Broker resolves one static secret after allow.",
      ja: "Gateway がハンドルを除去して秘密を含まない検査を行い、OPA が通常の effect を判断します。許可された場合にだけ、Auth Broker が静的な秘密情報を 1 回解決します。",
    },
    routes: [
      "workspace-proxy",
      "broker-introspect",
      "policy",
      "vault-state",
      "upstream-request",
    ],
    sent: {
      en: "Handle and trusted binding metadata before policy; one request-local credential value after allow.",
      ja: "ポリシー判断前はハンドルと信頼済みの結び付きメタデータ、許可後はそのリクエストだけで使う認証情報の値 1 つ。",
    },
    withheld: {
      en: "OPA never sees handle, body, revision, or secret; Workspace never receives the primary secret.",
      ja: "OPA はハンドル、本文、リビジョン、秘密情報を見ません。Workspace は実物の秘密情報を受け取りません。",
    },
    result: {
      en: "Gateway replaces only the declared destination header and makes one upstream attempt.",
      ja: "Gateway は宣言済みの宛先ヘッダーだけを置き換え、接続先へ 1 回だけ接続します。",
    },
    failure: {
      en: "Invalid handle or OPA deny stops before secret resolution and upstream.",
      ja: "不正なハンドルまたは OPA の拒否では、秘密情報の解決と接続先への接続より前に停止します。",
    },
  },
  {
    id: "aws",
    label: { en: "AWS SigV4", ja: "AWS SigV4" },
    summary: {
      en: "Only after OPA allow, Gateway captures the bounded request and Broker uses the private companion for one fixed AWS credential export before signing locally.",
      ja: "OPA の許可後にだけ Gateway が上限内のリクエストを保持し、Auth Broker が非公開コンパニオンへ固定された AWS 認証情報エクスポートを 1 回依頼してからローカルで署名します。",
    },
    routes: [
      "workspace-proxy",
      "broker-introspect",
      "policy",
      "vault-state",
      "aws-companion",
      "aws-provider",
      "upstream-request",
    ],
    sent: {
      en: "Opaque AWS state to the authenticated companion operation; temporary role credentials return only to Broker; signed headers return to Gateway.",
      ja: "不透明な AWS 状態を認証済みコンパニオン処理へ送ります。一時的な role 認証情報は Auth Broker だけへ戻り、署名済みヘッダーだけが Gateway へ戻ります。",
    },
    withheld: {
      en: "No Workspace-selected executable, argv, profile, host socket, temporary credential, body, or body hash enters policy.",
      ja: "Workspace が選んだ実行ファイル、argv、profile、ホスト socket、一時認証情報、本文、body hash はポリシーへ入りません。",
    },
    result: {
      en: "One bounded, standard header-based SigV4 request to the reviewed authority.",
      ja: "レビュー済みの authority へ、上限内の標準的なヘッダー方式の SigV4 リクエストを 1 回送ります。",
    },
    failure: {
      en: "Known pre-execution failure is 503; explicit or post-dispatch uncertainty is non-retryable 409; no upstream attempt occurs.",
      ja: "実行前と確定できる失敗は 503、dispatch 後など結果不明の失敗は再試行不可の 409 となり、接続先へは送信しません。",
    },
  },
  {
    id: "datadog",
    label: { en: "Datadog OAuth", ja: "Datadog OAuth" },
    summary: {
      en: "Only after OPA allow, Broker selects a valid token or performs one same-record refresh at the exact Datadog US1 token endpoint.",
      ja: "OPA の許可後にだけ Auth Broker が有効なトークンを選ぶか、完全一致する Datadog US1 トークンエンドポイントで同じレコードの更新を 1 回実行します。",
    },
    routes: [
      "workspace-proxy",
      "broker-introspect",
      "policy",
      "vault-state",
      "datadog-refresh",
      "upstream-request",
    ],
    sent: {
      en: "One strict OAuth refresh form when needed; one request-local bearer token returns to Gateway.",
      ja: "必要な場合だけ厳密な OAuth 更新フォームを 1 回送り、そのリクエストだけで使う bearer token 1 つを Gateway へ返します。",
    },
    withheld: {
      en: "No pup process in Broker, ambient proxy, redirect, alternate token host, OAuth state in Workspace, or token in OPA.",
      ja: "Auth Broker 内の pup プロセス、環境由来のプロキシ、リダイレクト、別のトークンホスト、Workspace 内の OAuth 状態、OPA 内のトークンはありません。",
    },
    result: {
      en: "Refreshed state commits before Gateway replaces the exact bearer header and makes one upstream attempt.",
      ja: "更新後の状態を確定してから、Gateway が完全一致する bearer header を置き換え、接続先へ 1 回接続します。",
    },
    failure: {
      en: "Known pre-send failure is 503; post-send ambiguity is non-retryable 409 with a durable barrier; no upstream attempt occurs.",
      ja: "送信前と確定できる失敗は 503、送信後の結果不明は永続 barrier 付きの再試行不可 409 となり、接続先へは送信しません。",
    },
  },
];

export function textFor(value: LocalizedText, locale: SiteLocale): string {
  return value[locale];
}
