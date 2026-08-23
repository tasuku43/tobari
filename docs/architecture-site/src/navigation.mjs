const item = (label, japanese, link) => ({
  label,
  translations: { ja: japanese },
  link,
});

const group = (label, japanese, items) => ({
  label,
  translations: { ja: japanese },
  items,
});

export const navigationGroups = [
  group("Start", "はじめに", [
    item("Overview", "概要", "/start/overview/"),
    item("Install", "インストール", "/start/install/"),
    item("Quickstart", "クイックスタート", "/start/quickstart/"),
    item(
      "Prepare a custom runtime",
      "カスタムランタイムを用意する",
      "/start/runtime-setup/",
    ),
    item(
      "Authenticate your tools",
      "ツールの認証を設定する",
      "/start/authentication-setup/",
    ),
    item("First denial", "最初の拒否", "/start/first-denial/"),
    item("Learning path", "学習ガイド", "/start/learning-path/"),
    item(
      "Understanding check",
      "理解度チェック",
      "/start/understanding-check/",
    ),
  ]),
  group("How it works", "仕組み", [
    item("Mental model", "基本モデル", "/how-it-works/mental-model/"),
    item("System overview", "システム全体像", "/how-it-works/system-overview/"),
    item(
      "Workspace, Workspace Manifest, cluster",
      "Workspace、Workspace Manifest、クラスター",
      "/how-it-works/workspace-manifest-workspace-cluster/",
    ),
    item(
      "Workspace lifecycle",
      "Workspace のライフサイクル",
      "/how-it-works/workspace-lifecycle/",
    ),
    item(
      "Request journey",
      "リクエストの流れ",
      "/how-it-works/request-journey/",
    ),
    item("HTTPS and TLS", "HTTPS と TLS", "/how-it-works/https-and-tls/"),
    item(
      "Project identity",
      "プロジェクトの識別",
      "/how-it-works/project-identity/",
    ),
    item("Policy learning", "ポリシー学習", "/how-it-works/policy-learning/"),
    item("Credentials", "認証情報", "/how-it-works/credentials/"),
    item(
      "State and recovery",
      "状態と復旧",
      "/how-it-works/state-and-recovery/",
    ),
    item(
      "Implementation stack",
      "実装技術",
      "/how-it-works/implementation-stack/",
    ),
    item("Code architecture", "コード構造", "/how-it-works/code-architecture/"),
  ]),
  group("Security", "セキュリティ", [
    item("Overview", "概要", "/security/overview/"),
    item(
      "Guarantees and limitations",
      "保証と制限",
      "/security/guarantees-and-limitations/",
    ),
    item("Trust boundaries", "信頼境界", "/security/trust-boundaries/"),
    item("Threat model", "脅威モデル", "/security/threat-model/"),
    item(
      "Credential security",
      "認証情報の安全性",
      "/security/credential-security/",
    ),
    item(
      "Audit and privacy",
      "監査とプライバシー",
      "/security/audit-and-privacy/",
    ),
    item("Supply chain", "サプライチェーン", "/security/supply-chain/"),
    item(
      "Implementation and tests",
      "実装とテスト",
      "/security/verification-evidence/",
    ),
  ]),
  group("Guides", "ガイド", [
    item(
      "Workspace Manifests and separate configurations",
      "設定を分ける（Workspace Manifest）",
      "/guides/workspace-manifests/",
    ),
    item(
      "Runtime customization details",
      "ランタイムの詳細設定",
      "/guides/runtime-customization/",
    ),
    item("Authentication details", "認証の詳細", "/guides/authentication/"),
    item("Policy review", "ポリシーレビュー", "/guides/policy-review/"),
    item("Advanced policy", "高度なポリシー", "/guides/advanced-policy/"),
    item("Colima and Lima", "Colima と Lima", "/guides/colima-and-lima/"),
    item(
      "Troubleshooting",
      "トラブルシューティング",
      "/guides/troubleshooting/",
    ),
  ]),
  group("Reference", "リファレンス", [
    item("CLI", "CLI", "/reference/cli/"),
    item(
      "Configuration and state",
      "設定と状態",
      "/reference/configuration-and-state/",
    ),
    item(
      "Provider manifest",
      "プロバイダーマニフェスト",
      "/reference/provider-manifest/",
    ),
    item("JSON schemas", "JSON Schema", "/reference/json-schemas/"),
    item(
      "Faults and recovery",
      "障害と復旧",
      "/reference/faults-and-recovery/",
    ),
    item(
      "Runtime image contract",
      "ランタイムイメージ契約",
      "/reference/runtime-image-contract/",
    ),
    item(
      "Component versions",
      "コンポーネントのバージョン",
      "/reference/component-versions/",
    ),
    item("Glossary", "用語集", "/reference/glossary/"),
  ]),
];

export const primarySections = navigationGroups.map(
  ({ label, translations, items }) => ({
    label,
    translations,
    link: items[0].link,
  }),
);

export const navigationLabel = (entry, locale) =>
  entry.translations?.[locale] || entry.label;

export const navigationPath = (path, locale) =>
  locale === "ja" ? `/ja${path}` : path;
