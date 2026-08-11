from pathlib import Path
import re

repo = Path.cwd()


def replace_exact(path: str, old: str, new: str) -> None:
    target = repo / path
    text = target.read_text()
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected one exact match, found {count}")
    target.write_text(text.replace(old, new))


def replace_regex(path: str, pattern: str, replacement: str) -> None:
    target = repo / path
    text = target.read_text()
    updated, count = re.subn(pattern, replacement, text, count=1, flags=re.S)
    if count != 1:
        raise SystemExit(f"{path}: expected one regex match, found {count}")
    target.write_text(updated)


home_tasks = '''const tasks = [
  {
    href: "/start/quickstart/",
    title: t("I want to try it", "まず動かす"),
    detail: t(
      "Experience denial, review, allow, and deliberate retry in a synthetic project.",
      "拒否、レビュー、許可、再実行までを、練習用ディレクトリで確認します。",
    ),
  },
  {
    href: "/start/first-denial/",
    title: t("A request was denied", "通信が拒否された"),
    detail: t(
      "Find why it stopped, where to review it, and what can be allowed.",
      "拒否理由と通信条件を確認し、何を許可するか判断します。",
    ),
  },
  {
    href: "/guides/runtime-customization/",
    title: t(
      "I need the coding agent and project tools",
      "カスタムランタイムを作る",
    ),
    detail: t(
      "Build and validate the Context runtime before using a real repository.",
      "実際のリポジトリで使う前に、エージェントと開発ツールを追加します。",
    ),
  },
  {
    href: "/guides/authentication/",
    title: t(
      "I need GitHub or another supported login",
      "GitHub などの認証を設定する",
    ),
    detail: t(
      "Configure supported credentials without placing their primary value in the Workspace.",
      "認証情報の本体を Workspace に渡さず、対応するプロバイダーを設定します。",
    ),
  },
  {
    href: "/security/guarantees-and-limitations/",
    title: t("I need the security boundary", "セキュリティ境界を確認する"),
    detail: t(
      "Read each guarantee with its prerequisites, exclusions, and trusted components.",
      "保証が成立する前提と、Tobari が保護しない範囲を確認します。",
    ),
  },
];'''

replace_regex(
    "docs/architecture-site/src/components/HomeExperience.astro",
    r"const tasks = \[\n.*?\n\];(?=\n\nconst protectedItems)",
    home_tasks,
)

old_ja_step = '''        {
          title: "状態を確認して、練習用リソースを削除する",
          location: "信頼するホスト",
          action:
            "Workspace の状態を確認し、練習用 Workspace を削除してから、空になった共有クラスターを停止します。",
          command: `tobari status
tobari list
tobari delete
tobari cluster down --purge`,
          expected:
            "`delete` は Workspace の永続ホームと、Workspace が所有するランタイムリソースを削除します。練習用ディレクトリのファイルは削除しません。`cluster down --purge` は、共有 CA と有効なポリシーバンドルのボリュームも削除します。",
          blocked:
            "`cluster_not_empty` の場合は `tobari list` で残っている Workspace を確認し、それぞれを削除してから再実行します。接続中のセッションを意図的に終了する場合だけ `tobari delete --force` を使います。",
        },'''
new_ja_step = '''        {
          title: "状態を確認し、次の作業を選ぶ",
          location: "信頼するホスト",
          action:
            "Workspace と共有クラスターの状態を確認します。次にカスタムランタイムを作る場合は、そのまま残します。ここで終了する場合だけ、練習用 Workspace と共有状態を削除します。",
          command: `tobari status
tobari list

# ここで終了する場合だけ
tobari delete
tobari cluster down --purge`,
          expected:
            "`status` と `list` で、Workspace と Context の状態を確認できます。カスタムランタイムへ進む場合は削除せず、次の `runtime build` と入室で作業コンテナだけを調整します。ここで終了する場合は、`delete` が永続ホームと Workspace 所有のランタイム状態を削除し、`cluster down --purge` が共有 CA とポリシーバンドルも削除します。",
          blocked:
            "`cluster_not_empty` の場合は `tobari list` でほかの Workspace を確認します。接続中のセッションを意図的に終了する場合だけ `tobari delete --force` を使います。カスタムランタイムへ続けるだけなら、削除操作は不要です。",
        },'''
replace_exact(
    "docs/architecture-site/src/components/QuickstartWalkthrough.astro",
    old_ja_step,
    new_ja_step,
)

old_ja_next = '''      nextTitle: "必要に応じて詳細を確認する",
      nextLinks: [
        ["拒否された通信のレビュー", "/start/first-denial/"],
        ["リクエストを処理する順序", "/how-it-works/request-journey/"],
        ["保証が成立する前提と制限", "/security/guarantees-and-limitations/"],
        ["終了・削除・復旧後に残る状態", "/how-it-works/state-and-recovery/"],
      ],'''
new_ja_next = '''      nextTitle: "次に、実際の作業環境を作る",
      nextLinks: [
        [
          "カスタムランタイムにエージェントを追加する",
          "/guides/runtime-customization/",
        ],
        ["最初の拒否を詳しく確認する", "/start/first-denial/"],
        ["リクエストを処理する順序", "/how-it-works/request-journey/"],
        ["保証が成立する前提と制限", "/security/guarantees-and-limitations/"],
      ],'''
replace_exact(
    "docs/architecture-site/src/components/QuickstartWalkthrough.astro",
    old_ja_next,
    new_ja_next,
)

old_en_step = '''        {
          title: "Inspect state and remove the synthetic resources",
          location: "Trusted host",
          action:
            "Inspect the Workspace, delete the synthetic Workspace, and then stop the now-empty shared cluster.",
          command: `tobari status
tobari list
tobari delete
tobari cluster down --purge`,
          expected:
            "`delete` removes the Workspace persistent home and owned runtime resources, but not the synthetic project files. `cluster down --purge` also removes shared CA and active policy-bundle volumes.",
          blocked:
            "For `cluster_not_empty`, use `tobari list`, delete every remaining Workspace, and retry. Use `tobari delete --force` only when you intentionally want to terminate an attached session.",
        },'''
new_en_step = '''        {
          title: "Inspect state and choose the next step",
          location: "Trusted host",
          action:
            "Inspect the Workspace and shared cluster. Keep them when you will set up the custom runtime next. Delete the synthetic Workspace and shared state only when you are stopping here.",
          command: `tobari status
tobari list

# Optional cleanup when stopping here
tobari delete
tobari cluster down --purge`,
          expected:
            "`status` and `list` show the Workspace and Context state. When continuing to runtime setup, leave them in place; the next `runtime build` and entry reconcile only the work container. When stopping here, `delete` removes the persistent home and Workspace-owned runtime state, and `cluster down --purge` also removes the shared CA and policy-bundle volumes.",
          blocked:
            "For `cluster_not_empty`, inspect other Workspaces with `tobari list`. Use `tobari delete --force` only to terminate an attached session intentionally. No deletion is required when continuing to custom runtime setup.",
        },'''
replace_exact(
    "docs/architecture-site/src/components/QuickstartWalkthrough.astro",
    old_en_step,
    new_en_step,
)

old_en_next = '''      nextTitle: "Read only the detail you need next",
      nextLinks: [
        ["What to inspect after a denial", "/start/first-denial/"],
        ["The complete request sequence", "/how-it-works/request-journey/"],
        ["Guarantees and exclusions", "/security/guarantees-and-limitations/"],
        [
          "State that survives exit, deletion, and recovery",
          "/how-it-works/state-and-recovery/",
        ],
      ],'''
new_en_next = '''      nextTitle: "Next, prepare the real work environment",
      nextLinks: [
        ["Set up the coding-agent runtime", "/guides/runtime-customization/"],
        ["Inspect the first denial in detail", "/start/first-denial/"],
        ["The complete request sequence", "/how-it-works/request-journey/"],
        ["Guarantees and exclusions", "/security/guarantees-and-limitations/"],
      ],'''
replace_exact(
    "docs/architecture-site/src/components/QuickstartWalkthrough.astro",
    old_en_next,
    new_en_next,
)

replace_exact(
    "docs/architecture-site/src/content/docs/ja/start/first-denial.mdx",
    "**次へ:** [学習ガイドに沿って読む](../learning-path/)。",
    "**次へ:** [カスタムランタイムを準備する](../../guides/runtime-customization/)。",
)
replace_exact(
    "docs/architecture-site/src/content/docs/start/first-denial.mdx",
    "**Next:** [Follow the recommended learning path](../learning-path/).",
    "**Next:** [Set up the custom runtime](../../guides/runtime-customization/).",
)

replace_exact(
    "docs/architecture-site/tests/site.spec.ts",
    '''  "start/quickstart/",
  "how-it-works/system-overview/",''',
    '''  "start/quickstart/",
  "guides/runtime-customization/",
  "how-it-works/system-overview/",''',
)
replace_exact(
    "docs/architecture-site/tests/site.spec.ts",
    '''  "ja/",
  "ja/how-it-works/credentials/",''',
    '''  "ja/",
  "ja/guides/runtime-customization/",
  "ja/how-it-works/credentials/",''',
)

checks = {
    "docs/architecture-site/src/components/HomeExperience.astro": [
        'href: "/guides/runtime-customization/"',
        '"カスタムランタイムを作る"',
    ],
    "docs/architecture-site/src/components/QuickstartWalkthrough.astro": [
        '"Set up the coding-agent runtime"',
        '"カスタムランタイムにエージェントを追加する"',
    ],
    "docs/architecture-site/src/content/docs/ja/start/first-denial.mdx": [
        "カスタムランタイムを準備する",
    ],
}
for path, required in checks.items():
    text = (repo / path).read_text()
    for value in required:
        if value not in text:
            raise SystemExit(f"{path}: missing required text {value!r}")
