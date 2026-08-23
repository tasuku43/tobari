const page = (step, path, title, japaneseTitle, goal, japaneseGoal) => ({
  step,
  path,
  title,
  japaneseTitle,
  goal,
  japaneseGoal,
});

export const learningPathStepCount = 11;

export const learningPathPages = [
  page(
    1,
    "/start/overview/",
    "Overview",
    "概要",
    "Understand what Tobari controls and which parts remain trusted.",
    "Tobari が制御する範囲と、信頼する構成要素を確認します。",
  ),
  page(
    2,
    "/start/quickstart/",
    "Quickstart",
    "クイックスタート",
    "Experience denial, host review, exact allow, and deliberate retry with curl.",
    "curl を使い、拒否、ホストでのレビュー、完全一致の許可、再実行を体験します。",
  ),
  page(
    3,
    "/start/runtime-setup/",
    "Prepare a custom runtime",
    "カスタムランタイムを用意する",
    "Add the coding agent and project tools without learning configuration switching yet.",
    "設定の切り替えを覚える前に、エージェントと開発ツールをランタイムへ追加します。",
  ),
  page(
    4,
    "/start/authentication-setup/",
    "Authenticate your tools",
    "ツールの認証を設定する",
    "Authenticate the tools you need while keeping login separate from network permission.",
    "必要なツールを認証し、ログインと外部通信の許可を別々に扱います。",
  ),
  page(
    5,
    "/how-it-works/mental-model/",
    "Mental model",
    "基本モデル",
    "Learn the core request components after the first working setup succeeds.",
    "最初の一連の導入を終えてから、通信を構成する主要コンポーネントを学びます。",
  ),
  page(
    6,
    "/how-it-works/request-journey/",
    "Request journey",
    "リクエストの流れ",
    "Follow one outbound request from the Workspace to the destination.",
    "Workspace から接続先まで、一つのリクエストを処理順に追います。",
  ),
  page(
    6,
    "/how-it-works/https-and-tls/",
    "HTTPS and TLS",
    "HTTPS と TLS",
    "Understand where the two TLS sessions exist and why Gateway can authorize HTTP effects.",
    "二つの TLS セッションがどこに存在し、Gateway が HTTP 通信を判断できる理由を確認します。",
  ),
  page(
    7,
    "/how-it-works/policy-learning/",
    "Policy learning",
    "ポリシー学習",
    "Understand how a denial becomes one reviewable exact effect.",
    "拒否された通信が、完全一致のレビュー候補になるまでを追います。",
  ),
  page(
    7,
    "/start/first-denial/",
    "First denial",
    "最初の拒否",
    "Inspect the policy review loop in more detail after experiencing it once.",
    "一度体験したポリシーレビューを、より詳しく確認します。",
  ),
  page(
    8,
    "/how-it-works/credentials/",
    "Credentials",
    "認証情報",
    "Understand the detailed credential paths after completing first-use authentication.",
    "初回の認証を完了した後で、認証情報が通る詳しい経路を理解します。",
  ),
  page(
    8,
    "/guides/authentication/",
    "Authentication details",
    "認証の詳細",
    "Compare tool-managed authentication and Tobari-managed provider credentials.",
    "ツール自身の認証と、Tobari が管理する外部サービス認証を比較します。",
  ),
  page(
    8,
    "/security/guarantees-and-limitations/",
    "Guarantees and limitations",
    "保証と制限",
    "Read each guarantee together with its preconditions and exclusions.",
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
    "/how-it-works/workspace-lifecycle/",
    "Workspace lifecycle",
    "Workspace のライフサイクル",
    "Distinguish shell exit, work-container replacement, Workspace deletion, and cluster shutdown.",
    "シェル終了、作業コンテナの置き換え、Workspace 削除、クラスター停止を区別します。",
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
    "/guides/workspace-manifests/",
    "Workspace Manifests and separate configurations",
    "設定を分ける（Workspace Manifest）",
    "Learn Workspace Manifests only when you need different runtimes, authentication sets, or policy modes.",
    "ランタイム、認証、ポリシーを用途ごとに分ける必要が出た段階で Workspace Manifest を学びます。",
  ),
  page(
    10,
    "/how-it-works/workspace-manifest-workspace-cluster/",
    "Workspace, Workspace Manifest, cluster",
    "Workspace、Workspace Manifest、クラスター",
    "Understand how a named configuration relates to Workspace identity and shared services.",
    "設定を分ける必要性を理解した後で、Workspace Manifest と Workspace、共有サービスの関係を確認します。",
  ),
  page(
    10,
    "/guides/runtime-customization/",
    "Runtime customization details",
    "ランタイムの詳細設定",
    "Inspect image selection, compatibility, and reconciliation when managing multiple setups.",
    "複数の設定を扱う段階で、イメージ選択、互換性検査、再調整の詳細を確認します。",
  ),
  page(
    10,
    "/guides/advanced-policy/",
    "Advanced policy",
    "高度なポリシー",
    "Use stricter or hand-authored policy only after the default review loop is familiar.",
    "既定のレビューループに慣れてから、より厳格なポリシーや手動管理へ進みます。",
  ),
  page(
    11,
    "/start/understanding-check/",
    "Understanding check",
    "理解度チェック",
    "Explain the boundary and the first-use workflow before opening the prepared answers.",
    "回答を開く前に、Tobari の境界と最初の一連の利用手順を説明します。",
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
