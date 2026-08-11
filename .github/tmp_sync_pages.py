from pathlib import Path

ROOT = Path("docs/architecture-site")


def replace(path: str, old: str, new: str, count: int = 1) -> None:
    file = ROOT / path
    text = file.read_text()
    actual = text.count(old)
    if actual < count:
        raise SystemExit(f"{path}: expected at least {count} occurrence(s), found {actual}: {old[:80]!r}")
    text = text.replace(old, new, count)
    file.write_text(text)


# Quickstart: keep the denied Workspace session alive and review from another host terminal.
q = "src/components/QuickstartWalkthrough.astro"
replace(q, 'location: "ホスト → Workspace → ホスト",\n          action:\n            "ホストで `tobari` を実行して Workspace に入り、練習用の PUT リクエストを送ります。拒否を確認したら、`exit` でホストへ戻ります。",', 'location: "ホスト → Workspace（開いたまま）",\n          action:\n            "ホストで `tobari` を実行して Workspace に入り、練習用の PUT リクエストを送ります。拒否を確認してもセッションは終了せず、そのまま待たせます。次の手順は別の信頼するホスト端末で行います。",')
replace(q, "  -X PUT https://example.com/quickstart\nexit`,\n          expected:\n            \"HTTP 403 と `policy_denied` が表示され、ホスト側のレビューコマンドが案内されます。Gateway は接続先へ接続せず、リクエストを自動で許可・再送しません。`exit` 後も Workspace とその状態は残ります。\",", "  -X PUT https://example.com/quickstart`,\n          expected:\n            \"HTTP 403 と `policy_denied` が表示され、ホスト側のレビューコマンドが案内されます。Gateway は接続先へ接続せず、リクエストを自動で許可・再送しません。Workspace のシェルは開いたままにします。\",")
replace(q, 'location: "信頼するホスト",\n          action:\n            "レビュー画面で Context、プロジェクト、ホスト、ポート、メソッド、パスを確認し、この通信条件だけを許可します。",', 'location: "別の信頼するホスト端末",\n          action:\n            "Workspace セッションを残したまま、別のホスト端末でレビュー画面を開きます。Context、プロジェクト、ホスト、ポート、メソッド、パスを確認し、この通信条件だけを許可します。",')
replace(q, 'title: "Workspace から同じ条件のリクエストを送り直す",\n          location: "ホスト → Workspace → ホスト",\n          action:\n            "同じプロジェクトから Workspace へ入り直し、同じメソッドとパスの新しいリクエストを送ります。",', 'title: "元の Workspace セッションから同じ条件で送り直す",\n          location: "手順 2 で開いた Workspace → ホスト",\n          action:\n            "手順 2 で待たせていた Workspace セッションへ戻り、同じメソッドとパスの新しいリクエストを送ります。確認後に `exit` でホストへ戻ります。",')
replace(q, "command: `tobari\n\n# Workspace 内\ncurl -sS -w '\\\\nhttp=%{http_code}\\\\n' \\\\\n  -X PUT https://example.com/quickstart\nexit`,", "command: `# 手順 2 で開いた Workspace 内\ncurl -sS -w '\\\\nhttp=%{http_code}\\\\n' \\\\\n  -X PUT https://example.com/quickstart\nexit`,", 1)
replace(q, 'location: "Host → inside Workspace → host",\n          action:\n            "Run `tobari` on the host, issue the synthetic PUT request inside the Workspace, then use `exit` to return to the host after observing the denial.",', 'location: "Host → Workspace (keep it open)",\n          action:\n            "Run `tobari` on the host and issue the synthetic PUT request inside the Workspace. After observing the denial, keep that session open and use another trusted-host terminal for the next step.",')
replace(q, "  -X PUT https://example.com/quickstart\nexit`,\n          expected:\n            \"You receive HTTP 403 with `policy_denied` and a host-side review command. Tobari does not connect upstream, approve the request, or retry it. The Workspace and its state remain after `exit`.\",", "  -X PUT https://example.com/quickstart`,\n          expected:\n            \"You receive HTTP 403 with `policy_denied` and a host-side review command. Tobari does not connect upstream, approve the request, or retry it. Keep the Workspace shell running.\",")
replace(q, 'location: "Trusted host",\n          action:\n            "Inspect the Context, project, host, port, method, and path in the review screen, then allow only this exact communication.",', 'location: "Another trusted-host terminal",\n          action:\n            "Leave the Workspace session running. In another host terminal, inspect the Context, project, host, port, method, and path in the review screen, then allow only this exact communication.",')
replace(q, 'title: "Re-enter the Workspace and issue a new request",\n          location: "Host → inside Workspace → host",\n          action:\n            "Re-enter from the same project and deliberately send a new request with the same method and path.",', 'title: "Retry from the original Workspace session",\n          location: "Workspace opened in step 2 → host",\n          action:\n            "Return to the Workspace session that has been waiting since step 2 and deliberately send a new request with the same method and path. Exit after observing the result.",')
replace(q, "command: `tobari\n\n# Inside the Workspace\ncurl -sS -w '\\\\nhttp=%{http_code}\\\\n' \\\\\n  -X PUT https://example.com/quickstart\nexit`,", "command: `# Inside the Workspace opened in step 2\ncurl -sS -w '\\\\nhttp=%{http_code}\\\\n' \\\\\n  -X PUT https://example.com/quickstart\nexit`,", 1)

# Policy review: review is a parallel trusted-host operation; it does not require session teardown.
replace("src/content/docs/ja/guides/policy-review.mdx", "ポリシーレビューは、保持された拒否の根拠を、ホストが所有する明示的な判断へ変える操作です。Workspace から出て、信頼するホスト上の Permission Inbox で、量を制限した秘密情報を含まない記録を確認します。", "ポリシーレビューは、保持された拒否の根拠を、ホストが所有する明示的な判断へ変える操作です。Workspace セッションはそのまま待たせ、別の信頼するホスト端末で Permission Inbox を開いて、量を制限した秘密情報を含まない記録を確認します。")
replace("src/content/docs/ja/guides/policy-review.mdx", "## 1. 再試行を止め、Workspace から出る", "## 1. 再試行を止め、Workspace セッションを保つ")
replace("src/content/docs/ja/guides/policy-review.mdx", "レビュー前に Workspace を出ます。繰り返しの再試行は権限を増やさず、同じ許可対象の上限のある根拠を更新するだけです。", "レビューのために Workspace を終了する必要はありません。エージェントを待たせたまま別のホスト端末を開きます。繰り返しの再試行は権限を増やさず、同じ許可対象の上限のある根拠を更新するだけです。")
replace("src/content/docs/ja/guides/policy-review.mdx", "確定した許可後、一致する Workspace へ再入室して新しいリクエストを送ります。", "確定した許可後、待たせていた Workspace セッションへ戻って新しいリクエストを送ります。セッションを終了していた場合だけ、同じ Workspace へ再入室します。")
replace("src/content/docs/guides/policy-review.mdx", "Policy review turns retained denial evidence into an explicit host-owned decision. Leave the Workspace, inspect the bounded secret-free Permission Inbox on the trusted host,", "Policy review turns retained denial evidence into an explicit host-owned decision. Keep the Workspace session waiting, open another trusted-host terminal, and inspect the bounded secret-free Permission Inbox there,")
replace("src/content/docs/guides/policy-review.mdx", "## 1. Stop retrying and leave the Workspace", "## 1. Stop retrying and keep the Workspace session")
replace("src/content/docs/guides/policy-review.mdx", "Exit the Workspace before review. Repeated retries do not increase authority; they only refresh bounded evidence for the same effect.", "You do not need to terminate the Workspace for review. Leave the agent waiting and use another host terminal. Repeated retries do not increase authority; they only refresh bounded evidence for the same effect.")
replace("src/content/docs/guides/policy-review.mdx", "After a confirmed allow, re-enter the matching Workspace and issue a new request.", "After a confirmed allow, return to the waiting Workspace session and issue a new request. Re-enter only if that session was already closed.")

# First-denial pages: make the host/Workspace concurrency explicit.
replace("src/content/docs/ja/start/first-denial.mdx", "信頼するホストで実行します。\n\n```sh\ntobari policy review\n```", "Workspace セッションを終了する必要はありません。エージェントを待たせ、別の信頼するホスト端末で実行します。\n\n```sh\ntobari policy review\n```")
replace("src/content/docs/start/first-denial.mdx", "On the trusted host:\n\n```sh\ntobari policy review\n```", "The Workspace session can stay open. Leave the agent waiting and run review from another trusted-host terminal:\n\n```sh\ntobari policy review\n```")

# Installation: current source needs the development resolver until reviewed API-4/API-3 images are published.
ja_install = "src/content/docs/ja/start/install.mdx"
old = '''## CLI をビルドして配置する

公開リポジトリをクローンし、宣言済みのビルドタスクを使います。

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build
```

生成物は `bin/tobari` です。このリポジトリの `bin` ディレクトリを `PATH` に追加するか、すでに `PATH` に含まれるディレクトリへバイナリをインストールします。

```sh
install -m 0755 bin/tobari ~/.local/bin/tobari
```

Go の標準的なインストール方法も利用できます。

```sh
go install ./cmd/tobari
```

リポジトリが管理するランタイム権限の代わりに、未検証で更新されるイメージタグを取得しないでください。共有サービスの識別情報は、CLI に埋め込まれたレビュー済みの不変なダイジェストから選ばれます。
'''
new = '''## CLI をビルドし、実行する成果物を確認する

公開リポジトリをクローンし、まず通常の CLI をビルドします。

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build
bin/tobari version --format json
```

`version` は、ソースコミット、ランタイム解決経路、必要な Gateway / Auth Broker API と、選択中の成果物が互換かどうかを表示します。共有サービスを起動する前に、この結果を確認してください。

**現在のソーススナップショットでは、ソースが要求する Gateway API 4 / Auth Broker API 3 に対して、通常ビルドが参照するレビュー済み公開ダイジェストは一世代前です。** 通常の `bin/tobari` は、この不一致を検出するとクラスター起動前に停止します。互換する公開ダイジェストが発行されるまで、現在のチェックアウト自体を試す場合は、リポジトリ所有の開発用成果物を明示的に使います。

```sh
task build:dev
TOBARI_SOURCE_ROOT=$PWD
tobari() { "$TOBARI_SOURCE_ROOT/bin/tobari-dev" "$@"; }
tobari version --format json
```

この関数は現在のホストシェルだけで `tobari` を `bin/tobari-dev` に結び付けます。以降のクイックスタートも同じシェルで実行してください。互換するレビュー済み成果物を選べる公開版では、この開発用の切り替えは不要です。

通常の公開バイナリを配置する場合、生成物は `bin/tobari` です。`PATH` 上のディレクトリへ配置できます。

```sh
install -m 0755 bin/tobari ~/.local/bin/tobari
```

Go の標準的なインストール方法も利用できます。

```sh
go install ./cmd/tobari
```

未検証で更新されるイメージタグへ置き換えて互換性検査を回避しないでください。共有サービスの識別情報は、レビュー済みの不変なダイジェストまたは明示的なリポジトリ開発経路から選ばれます。
'''
replace(ja_install, old, new)
replace(ja_install, "command -v tobari\ntobari version", "command -v tobari\ntobari version --format json")

en_install = "src/content/docs/start/install.mdx"
old = '''## Build and place the CLI

Clone the public repository and use its declared build task:

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build
```

The result is `bin/tobari`. Either add this repository's `bin` directory to `PATH`, or install the binary into a directory already on `PATH`:

```sh
install -m 0755 bin/tobari ~/.local/bin/tobari
```

The repository also supports Go's install path:

```sh
go install ./cmd/tobari
```

Do not download an unverified moving image tag as a substitute for the repository's runtime authority. Shared service identities are selected from reviewed immutable digests embedded with the CLI.
'''
new = '''## Build the CLI and verify the resolver you will use

Clone the public repository, build the ordinary CLI, and inspect its runtime identity before starting shared services:

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build
bin/tobari version --format json
```

`version` reports the source commit, runtime resolver channel, required Gateway / Auth Broker APIs, and whether the selected component identities are compatible.

**At this source snapshot, source requires Gateway API 4 and Auth Broker API 3 while the reviewed published digests selected by the ordinary build are one generation older.** `bin/tobari` deliberately stops before cluster mutation when it detects that mismatch. Until compatible reviewed published digests exist, use the repository-owned development artifacts explicitly when exercising this checkout:

```sh
task build:dev
TOBARI_SOURCE_ROOT=$PWD
tobari() { "$TOBARI_SOURCE_ROOT/bin/tobari-dev" "$@"; }
tobari version --format json
```

That function binds `tobari` to `bin/tobari-dev` only in the current host shell. Run the following Quickstart from the same shell. A compatible published build does not need this development override.

For an ordinary published binary, the build result remains `bin/tobari`; install it into a directory on `PATH` if desired:

```sh
install -m 0755 bin/tobari ~/.local/bin/tobari
```

The repository also supports Go's install path:

```sh
go install ./cmd/tobari
```

Do not replace an incompatible reviewed digest with an unverified moving tag. Shared service identities come from reviewed immutable digests or the explicit repository development resolver.
'''
replace(en_install, old, new)
replace(en_install, "command -v tobari\ntobari version", "command -v tobari\ntobari version --format json")

# Detailed authentication guide: current provider set, source compatibility, and activation-state semantics.
ja_auth = "src/content/docs/ja/guides/authentication.mdx"
replace(ja_auth, "## GitHub、AWS、Datadog の組み込み認証", "## Codex、Claude Code、GitHub、AWS、Datadog の組み込み認証")
replace(ja_auth, '''<aside class="warning">
  <strong>標準クラスターが使う成果物:</strong> ソースとテストには、AWS
  コンソールログインと Datadog
  の認証情報取得・更新経路があります。ただし、現在固定されている Gateway と Auth
  Broker のイメージは、それらより前の AWS Identity Center 成果物です。Datadog
  または AWS コンソールログインを使う前に、
  <a href="../../reference/component-versions/">コンポーネントのバージョン</a>
  で、標準クラスターが選ぶダイジェストを確認してください。
</aside>''', '''<aside class="warning">
  <strong>現在のソースと公開ダイジェスト:</strong> このソースは Gateway API 4 / Auth
  Broker API 3 を要求し、OpenAI/Codex と Anthropic/Claude Code を含む現在の認証計画を実装しています。
  一方、通常ビルドが参照するレビュー済み公開ダイジェストは一世代前です。互換性がない場合、
  Tobari はクラスター起動前に停止します。現在のチェックアウトを試す場合は
  <a href="../../start/install/">インストール</a>に記載した明示的な開発用経路を使い、
  任意のイメージタグで検査を回避しないでください。
</aside>''')
replace(ja_auth, "tobari auth login --provider github --context default\ntobari auth status --context default", "tobari auth login --provider openai --context default\ntobari auth login --provider anthropic --context default\n# または: github / aws / datadog\ntobari auth status --context default")
replace(ja_auth, "ログインは、信頼するホストの対話型ターミナルで行います。`--provider` を省略した場合、インストール済みでレビュー済みのプロバイダーだけを表示し、各行に対応ツールも示します。`--provider` を指定すると選択画面を省略します。\n\nGitHub では、", "ログインは、信頼するホストの対話型ターミナルで行います。`--provider` を省略した場合、インストール済みでレビュー済みのプロバイダーだけを表示し、各行に対応ツールも示します。`--provider` を指定すると選択画面を省略します。\n\nOpenAI では、信頼するホストにある完全一致の Codex 0.146.0 を固定引数のデバイスログインで実行し、隔離した一時 HOME から ChatGPT OAuth セッションだけを取得します。Workspace の `.codex/auth.json` に入る access token は実トークンではなく、プロジェクトに結び付いたハンドルです。実際のアクセストークンと account ID は、OPA の許可後に Auth Broker が選択または必要に応じて更新します。\n\nAnthropic では、信頼するホストにある完全一致の Claude Code 2.1.220 で `claude setup-token` を実行し、取得した inference token を Workspace の外へ保存します。Workspace の `CLAUDE_CODE_OAUTH_TOKEN` には実トークンではなくハンドルが入り、`api.anthropic.com` への許可済み通信でだけ解決されます。この経路には自動更新はありません。\n\nGitHub では、")
replace(ja_auth, '''ログイン後、その Context に結び付いた Workspace へ入り直します。

```sh
tobari --context default
```

Workspace は、''', '''ログイン、置換、ログアウトは、すでに動いているプロセスの環境やファイルをその場で書き換えません。`tobari auth status --context default` は既存 Workspace の投影状態を観測し、欠落または古い投影に再入室が必要な場合だけ、対象と操作を表示します。すべての Workspace を無条件に再起動するのではなく、その案内に従ってください。

Workspace は、''')
replace(ja_auth, "8. Datadog の許可では、十分な有効期間が残るトークンを選びます。必要な場合は、プロキシとリダイレクトを使わず、完全一致する US1 トークンエンドポイントで同じレコードを一度だけ更新します。Bearer 値を返す前に状態を保存します。\n9. Gateway が完全一致する HTTPS 接続先へ一度接続します。", "8. Datadog の許可では、十分な有効期間が残るトークンを選びます。必要な場合は、プロキシとリダイレクトを使わず、完全一致する US1 トークンエンドポイントで同じレコードを一度だけ更新します。Bearer 値を返す前に状態を保存します。\n9. OpenAI の許可では、有効期限に余裕のあるアクセストークンを選び、必要なら固定された OpenAI OAuth エンドポイントで一度だけ更新します。更新状態を先に保存し、Gateway が検証済みのアカウント ID と認証ヘッダーを最終的に付与します。Anthropic は静的な解決経路を使い、自動更新しません。\n10. Gateway が完全一致する HTTPS 接続先へ一度接続します。")
replace(ja_auth, "認証情報を置き換えると、以前に発行したハンドルはすべて無効になります。実行中のプロセスには古い環境変数が残りますが、その値は使えません。新しいハンドルを受け取るには、Workspace から退出して入り直します。", "認証情報を置き換えると、以前に発行したハンドルはすべて無効になります。実行中のプロセスには古い投影が残る場合がありますが、その値は使えません。`auth status` で Workspace ごとの投影状態を確認し、欠落または古い行に表示された再入室操作だけを実行します。")
replace(ja_auth, "ログアウトは、ローカルのプロバイダーレコードを削除し、ハンドルを無効化します。", "ログアウトは、認証情報が存在する場合はローカルのプロバイダーレコードを削除してハンドルを無効化し、すでに存在しない場合は `no_change` として報告します。")
replace(ja_auth, "許可された Gateway の経路だけが、宣言済みの静的認証情報の解決、AWS 署名、Datadog トークン操作を開始できます。", "許可された Gateway の経路だけが、宣言済みの静的認証情報の解決、AWS 署名、Datadog または OpenAI のトークン操作を開始できます。")

# Credential architecture labels must include the currently reviewed native account helpers and OpenAI vault state.
cred = "src/data/credentialArchitecture.ts"
replace(cred, 'en: "Runs fixed gh, aws, or pup login drivers; import reads protected stdin.",\n      ja: "あらかじめ決められた gh、aws、pup のログイン処理を実行します。インポートで読み取るのは、保護された標準入力だけです。",', 'en: "Runs fixed gh, aws, pup, Codex, or Claude login drivers; import reads protected stdin.",\n      ja: "固定された gh、aws、pup、Codex、Claude のログイン処理を実行します。インポートで読み取るのは、保護された標準入力だけです。",')
replace(cred, 'en: "Stores typed static secrets, opaque AWS state, Datadog OAuth state, revisions, and raw handles.",\n      ja: "型付きの静的な秘密情報、AWS の不透明な状態、Datadog OAuth の状態、リビジョン、未加工のハンドルを保存します。",', 'en: "Stores typed static secrets, AWS state, Datadog and OpenAI OAuth state, revisions, and raw handles.",\n      ja: "型付きの静的な秘密情報、AWS の状態、Datadog と OpenAI の OAuth 状態、リビジョン、未加工のハンドルを保存します。",')
replace(cred, 'en: "GitHub device login, AWS login, or Datadog OAuth reached by fixed trusted-host drivers.",\n      ja: "信頼できるホスト上の決められた処理から、GitHub のデバイスログイン、AWS のログイン、または Datadog OAuth へ接続します。",', 'en: "GitHub, AWS, Datadog, OpenAI, or Anthropic login reached only by fixed trusted-host drivers.",\n      ja: "固定された信頼するホスト側ドライバーだけが、GitHub、AWS、Datadog、OpenAI、Anthropic のログイン先へ接続します。",')

print("Synchronized hand-written Pages content with the current product contracts.")
