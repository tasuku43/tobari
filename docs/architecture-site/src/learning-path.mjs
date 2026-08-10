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
    2,
    "/how-it-works/mental-model/",
    "Mental model",
    "基本モデル",
    "Learn the five components without implementation detail.",
    "実装の詳細に入る前に、五つの主要コンポーネントを押さえます。",
  ),
  page(
    3,
    "/how-it-works/workspace-lifecycle/",
    "Workspace lifecycle",
    "Workspace のライフサイクル",
    "Distinguish leaving a shell, deleting a Workspace, and stopping the cluster.",
    "シェルの終了、Workspace の削除、クラスター停止の違いを整理します。",
  ),
  page(
    4,
    "/how-it-works/request-journey/",
    "Request journey",
    "リクエストの流れ",
    "Follow one outbound request from the Workspace to the upstream service.",
    "Workspace から接続先まで、一つのリクエストを処理順に追います。",
  ),
  page(
    5,
    "/how-it-works/https-and-tls/",
    "HTTPS and TLS",
    "HTTPS と TLS",
    "Understand the two TLS connections and the certificate limits.",
    "二つの TLS 接続と、証明書に関する制限を確認します。",
  ),
  page(
    6,
    "/how-it-works/policy-learning/",
    "Policy learning",
    "ポリシー学習",
    "Understand how a denial becomes one reviewable exact effect.",
    "拒否された通信が、完全一致のレビュー候補になるまでを追います。",
  ),
  page(
    6,
    "/start/first-denial/",
    "First denial",
    "最初の拒否",
    "Practice reviewing one denied effect on the trusted host.",
    "拒否された通信を、信頼するホストでレビューする手順を確認します。",
  ),
  page(
    7,
    "/how-it-works/credentials/",
    "Credentials",
    "認証情報",
    "Keep authentication separate from network authorization.",
    "外部サービスの認証と、HTTP 通信の許可を分けて扱います。",
  ),
  page(
    7,
    "/guides/authentication/",
    "Authentication guide",
    "認証",
    "Set up supported provider credentials without exposing them to the Workspace.",
    "認証情報の本体を Workspace に渡さず、対応するプロバイダーを設定します。",
  ),
  page(
    8,
    "/security/guarantees-and-limitations/",
    "Guarantees and limitations",
    "保証と制限",
    "Read each guarantee together with its preconditions and limits.",
    "各保証が成立する前提と、保護しない範囲を確認します。",
  ),
  page(
    8,
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
    "Understand which state survives failure, exit, and cleanup.",
    "障害、退出、削除の後に残る状態を整理します。",
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
