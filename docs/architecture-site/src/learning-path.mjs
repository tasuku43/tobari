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
    "Tobari が制御する範囲と、引き続き信頼する部分を理解します。",
  ),
  page(
    2,
    "/how-it-works/mental-model/",
    "Mental model",
    "メンタルモデル",
    "Learn the five components without implementation detail.",
    "実装の細部に入る前に、五つの主要な部品を理解します。",
  ),
  page(
    3,
    "/how-it-works/workspace-lifecycle/",
    "Workspace lifecycle",
    "Workspace のライフサイクル",
    "Distinguish leaving a shell, deleting a Workspace, and stopping the cluster.",
    "シェルを出ること、Workspace を削除すること、クラスターを止めることの違いを理解します。",
  ),
  page(
    4,
    "/how-it-works/request-journey/",
    "Request journey",
    "リクエストの流れ",
    "Follow one outbound request from the Workspace to the upstream service.",
    "一つの外向きリクエストが Workspace から接続先へ進む順序を追います。",
  ),
  page(
    5,
    "/how-it-works/https-and-tls/",
    "HTTPS and TLS",
    "HTTPS と TLS",
    "Understand the two TLS connections and the certificate limits.",
    "二つの TLS 接続と、証明書に関する制限を理解します。",
  ),
  page(
    6,
    "/how-it-works/policy-learning/",
    "Policy learning",
    "ポリシー学習",
    "Understand how a denial becomes one reviewable exact effect.",
    "拒否された通信が、確認できる完全一致の候補になる仕組みを理解します。",
  ),
  page(
    6,
    "/start/first-denial/",
    "First denial",
    "最初の拒否",
    "Practice reviewing one denied effect on the trusted host.",
    "拒否された通信を、信頼するホストで確認する流れを学びます。",
  ),
  page(
    7,
    "/how-it-works/credentials/",
    "Credentials",
    "認証情報",
    "Keep authentication separate from network authorization.",
    "外部サービスへの認証と、ネットワーク通信の許可を分けて理解します。",
  ),
  page(
    7,
    "/guides/authentication/",
    "Authentication guide",
    "認証ガイド",
    "Set up supported provider credentials without exposing them to the Workspace.",
    "対応する認証情報を Workspace に渡さず設定する方法を確認します。",
  ),
  page(
    8,
    "/security/guarantees-and-limitations/",
    "Guarantees and limitations",
    "保証と制限",
    "Read each guarantee together with its preconditions and limits.",
    "各保証を、その前提条件と制限を含めて確認します。",
  ),
  page(
    8,
    "/security/trust-boundaries/",
    "Trust boundaries",
    "信頼境界",
    "Identify which component owns and enforces each boundary.",
    "それぞれの境界を、どの部品が管理して実施するか確認します。",
  ),
  page(
    9,
    "/how-it-works/state-and-recovery/",
    "State and recovery",
    "状態と復旧",
    "Understand which state survives failure, exit, and cleanup.",
    "障害、退出、後片付けの後に残る状態を理解します。",
  ),
  page(
    9,
    "/guides/troubleshooting/",
    "Troubleshooting",
    "トラブルシューティング",
    "Choose a safe recovery action from the state you can observe.",
    "確認できる状態から、安全な復旧操作を選びます。",
  ),
  page(
    10,
    "/start/understanding-check/",
    "Understanding check",
    "理解度チェック",
    "Explain the boundary before opening the prepared answers.",
    "用意された答えを開く前に、自分の言葉で境界を説明します。",
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
