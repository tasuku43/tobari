const page = (step, path, title, japaneseTitle, goal, japaneseGoal) => ({
  step,
  path,
  title,
  japaneseTitle,
  goal,
  japaneseGoal,
});

export const learningPathStepCount = 10;

export const learningPathPages = [
  page(
    1,
    "/start/overview/",
    "Overview",
    "概要",
    "Understand what Tobari controls and which parts remain trusted.",
    "Tobari が制御する範囲と、信頼する構成要素を説明します。",
  ),
  page(
    1,
    "/start/install/",
    "Install",
    "インストール",
    "Install the CLI and confirm the trusted-host prerequisites.",
    "CLI を導入し、信頼するホスト側の前提条件を確認します。",
  ),
  page(
    2,
    "/start/quickstart/",
    "Quickstart",
    "クイックスタート",
    "Experience denial, host review, exact allow, and deliberate retry with curl.",
    "curl を使い、拒否、ホスト側レビュー、完全一致の許可、再実行を体験します。",
  ),
  page(
    2,
    "/start/first-denial/",
    "First denial",
    "最初の拒否",
    "Understand what the first review records and what an exact permission changes.",
    "最初のレビューで確認する情報と、完全一致の許可が変える範囲を整理します。",
  ),
  page(
    3,
    "/guides/runtime-customization/",
    "Custom runtime",
    "カスタムランタイム",
    "Install the coding agent and project tools in a validated Context runtime.",
    "コーディングエージェントと開発ツールを、検証済みの Context ランタイムへ追加します。",
  ),
  page(
    4,
    "/how-it-works/mental-model/",
    "Mental model",
    "基本モデル",
    "Learn the five components after the first working setup.",
    "最初の動作確認を終えた後で、五つの主要コンポーネントを整理します。",
  ),
  page(
    5,
    "/how-it-works/workspace-lifecycle/",
    "Workspace lifecycle",
    "Workspace のライフサイクル",
    "Distinguish leaving a shell, rebuilding a container, deleting a Workspace, and stopping the cluster.",
    "シェルの終了、コンテナ再作成、Workspace の削除、クラスター停止の違いを整理します。",
  ),
  page(
    6,
    "/how-it-works/request-journey/",
    "Request journey",
    "リクエストの流れ",
    "Follow one outbound request from the Workspace to the upstream service.",
    "Workspace から接続先まで、一つのリクエストを処理順に追います。",
  ),
  page(
    6,
    "/how-it-works/https-and-tls/",
    "HTTPS and TLS",
    "HTTPS と TLS",
    "Understand CONNECT, policy evaluation, and the two TLS sessions.",
    "CONNECT、ポリシー判断、二つの TLS セッションを一つの通信として追います。",
  ),
  page(
    7,
    "/how-it-works/policy-learning/",
    "Policy learning",
    "ポリシー学習",
    "Understand how retained denial data becomes one explicit exact decision.",
    "保持された拒否情報が、明示的な完全一致の判断になるまでを追います。",
  ),
  page(
    8,
    "/how-it-works/credentials/",
    "Credentials",
    "認証情報",
    "Keep service authentication separate from network authorization.",
    "外部サービスの認証と、HTTP 通信の許可を分けて扱います。",
  ),
  page(
    8,
    "/guides/authentication/",
    "Authentication guide",
    "認証",
    "Configure supported provider credentials without exposing their primary value to the Workspace.",
    "認証情報の本体を Workspace に渡さず、対応するプロバイダーを設定します。",
  ),
  page(
    9,
    "/security/guarantees-and-limitations/",
    "Guarantees and limitations",
    "保証と制限",
    "Read each guarantee together with its preconditions and limits.",
    "各保証が成立する前提と、保護しない範囲を確認します。",
  ),
  page(
    9,
    "/security/trust-boundaries/",
    "Trust boundaries",
    "信頼境界",
    "Identify which component owns and enforces each boundary.",
    "各境界を所有し、強制するコンポーネントを特定します。",
  ),
  page(
    9,
    "/how-it-works/state-and-recovery/",
    "State and recovery",
    "状態と復旧",
    "Understand which state survives failure, exit, image replacement, and cleanup.",
    "障害、退出、イメージ交換、削除の後に残る状態を整理します。",
  ),
  page(
    9,
    "/guides/troubleshooting/",
    "Troubleshooting",
    "トラブルシューティング",
    "Choose a safe recovery action from the state you can observe.",
    "観測できる状態から、破壊的でない復旧操作を選びます。",
  ),
  page(
    10,
    "/start/understanding-check/",
    "Understanding check",
    "理解度チェック",
    "Explain the boundary before opening the prepared answers.",
    "回答を開く前に、Tobari の境界を自分の言葉で説明します。",
  ),
];

const normalizeBase = (base) => {
  const parts = String(base || "/")
    .split("/")
    .filter(Boolean);
  return parts.length ? `/${parts.join("/")}/` : "/";
};

export const normalizeDocumentationRoute = (pathname, base = "/") => {
  const normalizedBase = normalizeBase(base);
  let route = String(pathname || "/").split(/[?#]/, 1)[0] || "/";

  if (normalizedBase !== "/" && route.startsWith(normalizedBase)) {
    route = `/${route.slice(normalizedBase.length)}`;
  }

  route = `/${route.split("/").filter(Boolean).join("/")}`;
  const locale = route === "/ja" || route.startsWith("/ja/") ? "ja" : "en";

  if (locale === "ja") {
    route = route.replace(/^\/ja(?=\/|$)/, "") || "/";
  }

  if (route !== "/" && !route.endsWith("/")) route += "/";
  return { locale, route };
};

export const localizeDocumentationPath = (path, locale, base = "/") => {
  const normalizedBase = normalizeBase(base);
  const route = String(path || "/").replace(/^\/+/, "");
  const localizedRoute = locale === "ja" ? `ja/${route}` : route;
  return `${normalizedBase}${localizedRoute}`.replace(/\/{2,}/g, "/");
};

const localizeEntry = (entry, locale, base) =>
  entry
    ? {
        ...entry,
        title: locale === "ja" ? entry.japaneseTitle : entry.title,
        goal: locale === "ja" ? entry.japaneseGoal : entry.goal,
        href: localizeDocumentationPath(entry.path, locale, base),
      }
    : null;

export const getLearningPathContext = (pathname, base = "/") => {
  const { locale, route } = normalizeDocumentationRoute(pathname, base);
  const index = learningPathPages.findIndex((entry) => entry.path === route);
  if (index < 0) return null;

  return {
    locale,
    index,
    stepCount: learningPathStepCount,
    pageCount: learningPathPages.length,
    current: localizeEntry(learningPathPages[index], locale, base),
    previous: localizeEntry(learningPathPages[index - 1], locale, base),
    next: localizeEntry(learningPathPages[index + 1], locale, base),
    overviewHref: localizeDocumentationPath(
      "/start/learning-path/",
      locale,
      base,
    ),
  };
};
