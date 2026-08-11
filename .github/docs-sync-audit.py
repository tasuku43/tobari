from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
SITE = ROOT / "docs" / "architecture-site"
TARGET = "3a9acc9c264d3e4efca2cd9aafabc9a122b183b8"
SNAPSHOT = SITE / "source-snapshot.txt"


def file(path: str) -> Path:
    return ROOT / path


def replace(path: str, old: str, new: str, *, required: bool = True) -> None:
    target = file(path)
    text = target.read_text(encoding="utf-8")
    count = text.count(old)
    if required and count != 1:
        raise SystemExit(f"{path}: expected one match, found {count}: {old[:90]!r}")
    if count:
        target.write_text(text.replace(old, new), encoding="utf-8")


def replace_regex(path: str, pattern: str, replacement: str, *, required: bool = True) -> None:
    target = file(path)
    text = target.read_text(encoding="utf-8")
    updated, count = re.subn(pattern, replacement, text, count=1, flags=re.S)
    if required and count != 1:
        raise SystemExit(f"{path}: regex expected one match, found {count}: {pattern[:90]!r}")
    if count:
        target.write_text(updated, encoding="utf-8")


def write(path: str, content: str) -> None:
    file(path).write_text(content.strip() + "\n", encoding="utf-8")


SNAPSHOT.write_text(TARGET + "\n", encoding="utf-8")

# Keep every first-party evidence link on the selected product snapshot.
permalink = re.compile(
    r"(https://github\.com/tasuku43/tobari/(?:blob|tree)/)[0-9a-f]{40}(/)"
)
for root_name in ("src", "public"):
    root = SITE / root_name
    if not root.exists():
        continue
    for path in root.rglob("*"):
        if not path.is_file() or path.suffix not in {
            ".astro", ".css", ".html", ".js", ".json", ".md", ".mdx",
            ".mjs", ".svg", ".ts"
        }:
            continue
        text = path.read_text(encoding="utf-8")
        updated = permalink.sub(rf"\g<1>{TARGET}\g<2>", text)
        if updated != text:
            path.write_text(updated, encoding="utf-8")

# ---------------------------------------------------------------------------
# Installation: the current source requires development images until matching
# immutable Gateway API 4 / Auth Broker API 3 publications exist.
# ---------------------------------------------------------------------------
write(
    "docs/architecture-site/src/content/docs/start/install.mdx",
    f'''---
title: Install
description: Build the current source with the correct resolver, inspect its compatibility identity, and run dependency-aware host diagnostics before starting Tobari.
---

Install Tobari on a supported macOS or Linux host. The current source contract requires **Gateway API 4** and **Auth Broker API 3**, while the immutable published digests still identify the earlier API 3 / API 2 services. A normal source build therefore rejects those historical pins instead of starting an incompatible cluster.

For the current `main` source, use the explicit development build below. This builds repository-owned development service images and a CLI that selects them. It does not change the published digest authority.

## Prerequisites

You need:

- macOS or Linux on a Docker-supported architecture;
- Docker Engine 24 or newer and Docker Compose v2;
- the Go toolchain declared by `go.mod`;
- [Task](https://taskfile.dev/); and
- registry access for the official base runtime and other reviewed images when they are not already local.

Brokered OpenAI login additionally requires exact **Codex 0.146.0** in a reviewed host executable location. Brokered Anthropic login requires exact **Claude Code 2.1.220**. The copy of either agent installed inside a Workspace is not used as a trusted login helper.

## Build the current source path

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build:dev
bin/tobari-dev version --format json
```

The structured version result identifies the source commit, the `development` resolver, required and selected Gateway/Auth Broker APIs, and whether the pair is compatible. Check this result before any cluster mutation.

To keep the remaining examples readable, bind `tobari` to the absolute development binary in the same host shell:

```sh
TOBARI_SOURCE_ROOT=$PWD
tobari() {{ "$TOBARI_SOURCE_ROOT/bin/tobari-dev" "$@"; }}
tobari version --format json
```

A future released binary with compatible reviewed image digests does not need this shell binding. `task build`, `go install`, and `bin/tobari` remain the standard release-oriented paths, but this source revision correctly refuses to use their currently selected historical service images.

## Inspect the host without changing state

Run the diagnostic from the project directory you plan to use:

```sh
tobari doctor
tobari doctor --format json
```

`doctor` is read-only. Its checks form a dependency graph: when an earlier prerequisite is unavailable, dependent checks are reported as **blocked** rather than being misreported as independent failures. It does not start the cluster, create a Workspace, repair credentials, activate policy, or create a policy-test container.

Use `--root PATH` only when you intentionally want to inspect another project root. Read-only first-use commands do not create Tobari-owned Context, Workspace, policy, or authentication state merely to produce a report.

## What happens next

The next mutation is explicit `tobari cluster up`. It preflights the selected service identities and refuses an API-incompatible pair. The root `tobari` entry command does not silently start or repair the shared cluster.

Do not work around a diagnostic or compatibility failure by mounting the Docker socket, the host home, or broad host paths into a Workspace. Those changes would invalidate the documented boundary.

## Source references

- [Current source walkthrough](https://github.com/tasuku43/tobari/blob/{TARGET}/README.md)
- [Release and development resolver contract](https://github.com/tasuku43/tobari/blob/{TARGET}/docs/06_release.md)
- [Pinned service identities](https://github.com/tasuku43/tobari/blob/{TARGET}/internal/infra/runtimeassets/assets/versions.env)
- [Dependency-aware doctor contract](https://github.com/tasuku43/tobari/blob/{TARGET}/docs/01_product_contract.md)

## Summary

For this source snapshot, build `bin/tobari-dev`, inspect `version --format json`, and use the development resolver until compatible immutable Gateway API 4 and Auth Broker API 3 indexes are published. `doctor` then checks the host without creating or repairing product state.

**Next:** [Run the Quickstart](../quickstart/).
''',
)

write(
    "docs/architecture-site/src/content/docs/ja/start/install.mdx",
    f'''---
title: インストール
description: 現在のソースに対応する実行ファイルをビルドし、互換性の識別情報とホスト側の前提条件を、状態を変更せずに確認します。
---

対応する macOS または Linux ホストへ Tobari を導入します。現在のソースは **Gateway API 4** と **Auth Broker API 3** を必要としますが、固定済みの公開ダイジェストは一つ前の API 3 / API 2 の成果物です。通常のソースビルドは、互換性のない古い成果物を起動せず、明示的に拒否します。

現在の `main` を試すときは、次の開発用ビルドを使います。リポジトリ内のソースから開発用サービスイメージと、それらを選ぶ CLI を作ります。公開ダイジェストの正本を書き換える操作ではありません。

## 前提条件

次が必要です。

- Docker が対応するアーキテクチャの macOS または Linux
- Docker Engine 24 以降と Docker Compose v2
- `go.mod` が宣言する Go ツールチェーン
- [Task](https://taskfile.dev/)
- 公式ベースランタイムなど、ローカルにないレビュー済みイメージを取得できること

OpenAI の Broker 認証を使う場合は、信頼するホスト上のレビュー対象となる場所に、正確に **Codex 0.146.0** が必要です。Anthropic では、正確に **Claude Code 2.1.220** が必要です。Workspace に入れたエージェント本体は、信頼するログイン用ヘルパーには使いません。

## 現在のソースをビルドする

```sh
git clone https://github.com/tasuku43/tobari.git
cd tobari
task build:dev
bin/tobari-dev version --format json
```

構造化されたバージョン表示には、ソースコミット、`development` リゾルバー、必要な Gateway/Auth Broker API、選択中の API、互換性が含まれます。クラスターを変更する前に確認してください。

以降の例を読みやすくするため、同じホストシェルで `tobari` を開発用バイナリへ結び付けます。

```sh
TOBARI_SOURCE_ROOT=$PWD
tobari() {{ "$TOBARI_SOURCE_ROOT/bin/tobari-dev" "$@"; }}
tobari version --format json
```

将来、互換する不変ダイジェストを含むリリース版が公開された場合、このシェル設定は不要です。`task build`、`go install`、`bin/tobari` は通常のリリース向け経路ですが、このソース時点では、そこから選ばれる過去のサービスイメージを正しく拒否します。

## 状態を変えずにホストを確認する

利用予定のプロジェクトディレクトリから実行します。

```sh
tobari doctor
tobari doctor --format json
```

`doctor` は読み取り専用です。検査には依存関係があり、前提となる検査を実行できない場合、後続は独立した失敗ではなく **blocked（前提不足）** として表示します。クラスターの起動、Workspace の作成、認証情報の修復、ポリシーの有効化、ポリシー確認用コンテナの作成は行いません。

別のディレクトリを意図して調べる場合だけ `--root PATH` を使います。最初の読み取り操作は、表示のためだけに Context、Workspace、ポリシー、認証状態を新しく作りません。

## 次に変更されるもの

次の変更操作は、明示的な `tobari cluster up` です。選択したサービスイメージを事前検査し、API が合わない組み合わせは起動しません。ルートの `tobari` 入室コマンドが、共有クラスターを暗黙に起動・修復することもありません。

診断や互換性の失敗を、Docker ソケット、ホストホーム、広いホストパスの追加マウントで回避しないでください。文書化した境界そのものが変わります。

## 参照する仕様

- [現在のソース向け手順](https://github.com/tasuku43/tobari/blob/{TARGET}/README.md)
- [リリースと開発用リゾルバーの契約](https://github.com/tasuku43/tobari/blob/{TARGET}/docs/06_release.md)
- [固定済みサービスイメージ](https://github.com/tasuku43/tobari/blob/{TARGET}/internal/infra/runtimeassets/assets/versions.env)
- [依存関係を持つ doctor の契約](https://github.com/tasuku43/tobari/blob/{TARGET}/docs/01_product_contract.md)

## まとめ

このソース時点では `bin/tobari-dev` を作り、`version --format json` で互換性を確認します。Gateway API 4 と Auth Broker API 3 に対応する不変な公開成果物が揃うまでは、開発用リゾルバーを使います。`doctor` は製品状態を作成・修復せずにホストを確認します。

**次へ:** [クイックスタート](../quickstart/)。
''',
)

# ---------------------------------------------------------------------------
# First-use authentication: add Codex/Claude plans and report re-entry only
# when auth status can prove a missing or stale Workspace projection.
# ---------------------------------------------------------------------------
write(
    "docs/architecture-site/src/content/docs/start/authentication-setup.mdx",
    '''---
title: Authenticate your tools
description: Authenticate only the tools and providers you use, while keeping account login, Workspace projection, and HTTP permission separate.
---

The previous step installed the coding agent and project tools. Authentication is separate: an executable can exist without an account session, and a successful login does not grant an HTTP/HTTPS allow rule.

Keep Tobari's default setup for this first pass. You do not need to learn Context switching yet.

## Choose where the credential should live

There are two ordinary paths.

### The tool stores its own login in the Workspace

Enter the project and use the tool's documented login flow:

```sh
cd /path/to/your/project
tobari

# Inside the Workspace
<agent-command> <login-command>
```

This state belongs to the Workspace home. Other processes in the same Workspace may be able to read or use it, and `tobari delete` removes it with the Workspace.

### Tobari stores a reviewed provider credential outside the Workspace

Run provider login on the trusted host:

```sh
tobari cluster up
tobari auth status
tobari auth login
```

You may select a provider explicitly:

```sh
tobari auth login --provider github
tobari auth login --provider aws
tobari auth login --provider datadog
tobari auth login --provider openai
tobari auth login --provider anthropic
```

The current reviewed pairings are GitHub/`gh`, AWS/`aws`, Datadog/`pup`, OpenAI/Codex 0.146.0, and Anthropic/Claude Code 2.1.220. OpenAI uses the exact trusted-host Codex device login; Anthropic uses the exact trusted-host `claude setup-token` flow. The Workspace copy of Codex or Claude is not an acquisition helper.

Tobari commits typed credential state to the encrypted Context vault and projects only a project-bound opaque handle where the provider plan requires one. Login still adds no OPA permission.

## Use auth status as the authority for Workspace activation

After a login, import, replacement, or logout, inspect the result or run:

```sh
tobari auth status --format json
```

The result distinguishes provider configuration from each eligible Workspace projection. An exact re-entry action is present only when Tobari can prove that a Workspace projection is **missing** or **stale**. A current projection needs no action. `unavailable` or `unresolved` means Tobari could not prove freshness and does not invent a command. Logging out an already absent provider reports `no_change` and makes no removal, revocation, or re-entry claim.

When a reported row includes a working directory and argv, run that exact action. Do not infer re-entry from the provider label alone.

## Review new network effects without stopping the agent session

An authenticated tool can still receive `policy_denied` for an undeclared API request. Keep the Workspace and agent session running. In a separate trusted-host terminal:

1. run `tobari policy review`;
2. inspect the exact destination, port, method, and path;
3. stage Allow or Deny and confirm one Apply; and
4. return to the same running Workspace and issue a new request.

Authentication never bypasses Gateway or OPA, and policy allow never performs provider login.

## Completion

The first-use path is complete when you have:

1. experienced deny → host review → allow → same-session retry with `curl`;
2. built a runtime containing the actual coding agent and project tools; and
3. authenticated only the accounts required by those tools.

Continue normal work with the default setup. Learn [Contexts and separate configurations](../../guides/contexts/) only when you need different runtimes, authentication sets, or policy modes.

For provider-specific details, read [Authentication details](../../guides/authentication/).
''',
)

write(
    "docs/architecture-site/src/content/docs/ja/start/authentication-setup.mdx",
    '''---
title: ツールの認証を設定する
description: 実際に使うツールとプロバイダーだけを認証し、アカウントのログイン、Workspace への反映、HTTP 通信の許可を分けて扱います。
---

前の手順で、コーディングエージェントと開発ツールをランタイムへ追加しました。認証は別です。実行ファイルがあってもアカウントの認証状態があるとは限らず、ログインに成功しても HTTP/HTTPS の許可ルールは増えません。

最初は Tobari の初期設定をそのまま使います。ここで Context の切り替えを学ぶ必要はありません。

## 認証情報を置く場所を選ぶ

通常は、次の二つから選びます。

### ツール自身が Workspace にログイン状態を保存する

プロジェクトへ入り、そのツールの公式なログイン手順を実行します。

```sh
cd /path/to/your/project
tobari

# Workspace 内
<エージェントのコマンド> <ログインコマンド>
```

この状態は Workspace のホームに属します。同じ Workspace 内の別プロセスが読み取り・利用できる場合があり、`tobari delete` では Workspace と一緒に削除されます。

### Tobari がレビュー済みプロバイダーの認証情報を外側に保持する

信頼するホストで実行します。

```sh
tobari cluster up
tobari auth status
tobari auth login
```

プロバイダーを明示することもできます。

```sh
tobari auth login --provider github
tobari auth login --provider aws
tobari auth login --provider datadog
tobari auth login --provider openai
tobari auth login --provider anthropic
```

現在のレビュー済み組み合わせは、GitHub/`gh`、AWS/`aws`、Datadog/`pup`、OpenAI/Codex 0.146.0、Anthropic/Claude Code 2.1.220 です。OpenAI は信頼するホスト上の正確な Codex によるデバイスログイン、Anthropic は正確な `claude setup-token` を使います。Workspace 内に入れた Codex や Claude は、認証情報を取得する信頼済みヘルパーには使いません。

Tobari は型付きの認証状態を暗号化した Context 保管庫へ保存し、必要な方式では、プロジェクトに結び付いた不透明なハンドルだけを Workspace へ配置します。ログインしても OPA の許可は増えません。

## Workspace への反映は auth status で確認する

ログイン、インポート、置換、ログアウトの後は、その結果または次の出力を確認します。

```sh
tobari auth status --format json
```

結果は、プロバイダーの設定状態と、対象 Workspace ごとの投影状態を分けて表示します。Tobari が **missing** または **stale** と確認できた Workspace に限り、正確な作業ディレクトリと再入室コマンドを返します。現在の投影には操作が不要です。`unavailable` や `unresolved` は、状態を確定できないため、推測したコマンドを表示しません。すでに存在しないプロバイダーをログアウトした場合は `no_change` となり、削除・失効・再入室が起きたとは説明しません。

結果に作業ディレクトリと引数列がある場合だけ、その操作を使います。プロバイダー名だけを見て再入室が必要だと推測しないでください。

## 新しい外部通信は、エージェントを止めずにレビューする

認証済みツールでも、未許可の API 通信には `policy_denied` が返ります。Workspace とエージェントのセッションは動かしたままにします。別の信頼するホストターミナルで、次を行います。

1. `tobari policy review` を実行する
2. 宛先、ポート、メソッド、パスを確認する
3. Allow または Deny を準備し、一度の Apply を確認する
4. 元の Workspace セッションへ戻り、新しいリクエストとして再実行する

認証が Gateway や OPA を迂回することはありません。通信の許可によってプロバイダーへログインできるようになるわけでもありません。

## 完了条件

最初の一連の導入は、次を終えた時点で完了です。

1. `curl` で拒否 → ホスト側レビュー → 許可 → 同じセッションでの再試行を体験した
2. 実際に使うエージェントと開発ツールをランタイムへ追加した
3. そのツールに必要なアカウントだけを認証した

初期設定のまま通常の作業を始められます。ランタイム、認証、ポリシーを用途ごとに分けたくなった段階で、[設定を分ける（Context）](../../guides/contexts/)を読んでください。

プロバイダーごとの詳細は、[認証の詳細](../../guides/authentication/)で説明します。
''',
)

# ---------------------------------------------------------------------------
# Keep the Quickstart Workspace session running while review happens in a
# second trusted-host terminal.
# ---------------------------------------------------------------------------
quickstart = "docs/architecture-site/src/components/QuickstartWalkthrough.astro"
replace(quickstart, 'location: "ホスト → Workspace → ホスト",\n          action:\n            "ホストで `tobari` を実行して Workspace に入り、練習用の PUT リクエストを送ります。拒否を確認したら、`exit` でホストへ戻ります。",\n          command: `tobari\n\n# Workspace 内\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart\nexit`,\n          expected:\n            "HTTP 403 と `policy_denied` が表示され、ホスト側のレビューコマンドが案内されます。Gateway は接続先へ接続せず、リクエストを自動で許可・再送しません。`exit` 後も Workspace とその状態は残ります。",', 'location: "ホスト → Workspace（そのまま動かす）",\n          action:\n            "ホストで `tobari` を実行して Workspace に入り、練習用の PUT リクエストを送ります。拒否を確認しても `exit` せず、このセッションを動かしたまま別のホストターミナルを開きます。",\n          command: `tobari\n\n# Workspace 内。このセッションは開いたままにする\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart`,\n          expected:\n            "HTTP 403 と `policy_denied` が表示され、別の信頼するホストターミナルで実行するレビューコマンドが案内されます。Gateway は接続先へ接続せず、リクエストを自動で許可・再送しません。",')
replace(quickstart, 'location: "信頼するホスト",\n          action:\n            "レビュー画面で Context、プロジェクト、ホスト、ポート、メソッド、パスを確認し、この通信条件だけを許可します。",', 'location: "別の信頼するホストターミナル",\n          action:\n            "Workspace セッションを動かしたまま、別ターミナルのレビュー画面で Context、プロジェクト、ホスト、ポート、メソッド、パスを確認し、この通信条件だけを許可します。",')
replace(quickstart, 'title: "Workspace から同じ条件のリクエストを送り直す",\n          location: "ホスト → Workspace → ホスト",\n          action:\n            "同じプロジェクトから Workspace へ入り直し、同じメソッドとパスの新しいリクエストを送ります。",\n          command: `tobari\n\n# Workspace 内\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart\nexit`,', 'title: "元の Workspace セッションで同じ条件を再試行する",\n          location: "手順2から動かしている Workspace",\n          action:\n            "元の Workspace セッションへ戻り、同じメソッドとパスの新しいリクエストを送ります。確認後に `exit` します。",\n          command: `# 元の Workspace 内\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart\nexit`,')
replace(quickstart, 'location: "Host → inside Workspace → host",\n          action:\n            "Run `tobari` on the host, issue the synthetic PUT request inside the Workspace, then use `exit` to return to the host after observing the denial.",\n          command: `tobari\n\n# Inside the Workspace\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart\nexit`,\n          expected:\n            "You receive HTTP 403 with `policy_denied` and a host-side review command. Tobari does not connect upstream, approve the request, or retry it. The Workspace and its state remain after `exit`.",', 'location: "Host → Workspace (keep it running)",\n          action:\n            "Run `tobari` on the host and issue the synthetic PUT request inside the Workspace. After observing the denial, keep this session open and start a second terminal on the trusted host.",\n          command: `tobari\n\n# Inside the Workspace. Keep this session open.\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart`,\n          expected:\n            "You receive HTTP 403 with `policy_denied` and a review command for a separate trusted-host terminal. Tobari does not connect upstream, approve the request, or retry it.",')
replace(quickstart, 'location: "Trusted host",\n          action:\n            "Inspect the Context, project, host, port, method, and path in the review screen, then allow only this exact communication.",', 'location: "Separate trusted-host terminal",\n          action:\n            "Keep the Workspace session running. In a second terminal, inspect the Context, project, host, port, method, and path, then allow only this exact communication.",')
replace(quickstart, 'title: "Re-enter the Workspace and issue a new request",\n          location: "Host → inside Workspace → host",\n          action:\n            "Re-enter from the same project and deliberately send a new request with the same method and path.",\n          command: `tobari\n\n# Inside the Workspace\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart\nexit`,', 'title: "Retry in the original Workspace session",\n          location: "The Workspace kept open from step 2",\n          action:\n            "Return to the original Workspace session and deliberately issue a new request with the same method and path. Exit after the check.",\n          command: `# In the original Workspace\ncurl -sS -w \'\\\\nhttp=%{http_code}\\\\n\' \\\\\n  -X PUT https://example.com/quickstart\nexit`,')

# ---------------------------------------------------------------------------
# Policy review pages: same-session retry and resumable candidate-ID refresh.
# ---------------------------------------------------------------------------
for path in [
    "docs/architecture-site/src/content/docs/guides/policy-review.mdx",
    "docs/architecture-site/src/content/docs/ja/guides/policy-review.mdx",
]:
    text = file(path).read_text(encoding="utf-8")
    if path.endswith("/ja/guides/policy-review.mdx"):
        text = text.replace(
            "Workspace から出て、信頼するホスト上の Permission Inbox で、量を制限した秘密情報を含まない記録を確認します。",
            "Workspace とエージェントのセッションを動かしたまま、別の信頼するホストターミナルで Permission Inbox を開き、量を制限した秘密情報を含まない記録を確認します。",
        ).replace(
            "## 1. 再試行を止め、Workspace から出る",
            "## 1. 再試行を止め、Workspace は動かしたままにする",
        ).replace(
            "レビュー前に Workspace を出ます。繰り返しの再試行は権限を増やさず、同じ許可対象の上限のある根拠を更新するだけです。",
            "レビューのために Workspace を終了しません。元のセッションを保持し、別のホストターミナルを使います。確認前に同じリクエストを繰り返しても権限は増えません。",
        ).replace(
            "確定した許可後、一致する Workspace へ再入室して新しいリクエストを送ります。",
            "確定した許可後、元の実行中 Workspace セッションへ戻り、新しいリクエストを送ります。",
        )
        text = text.replace(
            "Context を切り替える前に適用または破棄するため、ソースの更新は一度のアトミックなファイル置換になります。",
            "Context を切り替える前に適用または破棄します。手動 Refresh では、準備済みの判断を表示名ではなく候補 ID で照合します。同じ ID は判断と順序を保ち、消えた ID は Apply 対象から外れ、同じ表示名の別 ID へ権限を移しません。",
        )
    else:
        text = text.replace(
            "Leave the Workspace, inspect the bounded secret-free Permission Inbox on the trusted host, stage choices from exact detail screens, and Apply one Context's reviewed set once.",
            "Keep the Workspace and agent session running. In a separate trusted-host terminal, inspect the bounded secret-free Permission Inbox, stage choices from exact detail screens, and Apply one Context's reviewed set once.",
        ).replace(
            "## 1. Stop retrying and leave the Workspace",
            "## 1. Stop retrying and keep the Workspace running",
        ).replace(
            "Leave the Workspace before review. Repeated retries do not add authority; they only refresh bounded evidence for the same effect.",
            "Do not exit for review. Keep the original session open and use a separate trusted-host terminal. Repeated retries do not add authority.",
        ).replace(
            "After a confirmed allow, re-enter the matching Workspace and issue a new request.",
            "After a confirmed allow, return to the original running Workspace and issue a new request.",
        )
        text = text.replace(
            "Apply or discard before switching Context, so source promotion remains one atomic file replacement.",
            "Apply or discard before switching Context. Manual Refresh reconciles staged choices only by candidate ID: retained IDs keep their decision and order, stale IDs lose Apply eligibility, and a matching label never transfers authority to a replacement ID.",
        )
    file(path).write_text(text, encoding="utf-8")

for path in [
    "docs/architecture-site/src/content/docs/start/first-denial.mdx",
    "docs/architecture-site/src/content/docs/ja/start/first-denial.mdx",
]:
    text = file(path).read_text(encoding="utf-8")
    if "/ja/" in path:
        text = text.replace(
            "ここでは、拒否の発生からホスト側のレビュー、新しいリクエストとしての再試行までを順に追います。先に[クイックスタート](../quickstart/)を完了するか、ホストと Workspace のコマンドを別々の場所で実行することを確認してください。",
            "ここでは、拒否の発生からホスト側のレビュー、元の実行中 Workspace での再試行までを順に追います。Workspace とエージェントは動かしたままにし、ポリシーレビューには別の信頼するホストターミナルを使います。",
        )
    else:
        text = text.replace(
            "This page follows the denial, trusted-host review, and deliberate retry as a new request. Complete the Quickstart first or keep host and Workspace commands in separate locations.",
            "This page follows the denial, trusted-host review, and deliberate retry in the original running Workspace. Keep the Workspace and agent session open, and use a separate trusted-host terminal for policy review.",
        )
    file(path).write_text(text, encoding="utf-8")

# ---------------------------------------------------------------------------
# Provider support projection and credential diagrams.
# ---------------------------------------------------------------------------
provider_support = "docs/architecture-site/src/data/providerToolSupport.ts"
replace(
    provider_support,
    '''  {
    providerId: "chatwork",
    providerName: { en: "Chatwork", ja: "Chatwork" },''',
    '''  {
    providerId: "openai",
    providerName: { en: "OpenAI", ja: "OpenAI" },
    toolCommand: "codex",
    toolName: { en: "Codex CLI 0.146.0", ja: "Codex CLI 0.146.0" },
    acquisition: {
      en: "Reviewed ChatGPT device OAuth through exact trusted-host Codex",
      ja: "信頼するホスト上の正確な Codex を使う、レビュー済み ChatGPT デバイス OAuth",
    },
    mode: "login",
  },
  {
    providerId: "anthropic",
    providerName: { en: "Anthropic", ja: "Anthropic" },
    toolCommand: "claude",
    toolName: { en: "Claude Code 2.1.220", ja: "Claude Code 2.1.220" },
    acquisition: {
      en: "Reviewed setup-token flow through exact trusted-host Claude Code",
      ja: "信頼するホスト上の正確な Claude Code を使う、レビュー済み setup-token フロー",
    },
    mode: "login",
  },
  {
    providerId: "chatwork",
    providerName: { en: "Chatwork", ja: "Chatwork" },''',
)

credential_arch = "docs/architecture-site/src/data/credentialArchitecture.ts"
for old, new in [
    ("Runs fixed gh, aws, or pup login drivers; import reads protected stdin.", "Runs fixed gh, aws, pup, Codex 0.146.0, or Claude Code 2.1.220 login drivers; import reads protected stdin."),
    ("あらかじめ決められた gh、aws、pup のログイン処理を実行します。", "あらかじめ決められた gh、aws、pup、Codex 0.146.0、Claude Code 2.1.220 のログイン処理を実行します。"),
    ("Stores typed static secrets, opaque AWS state, Datadog OAuth state, revisions, and raw handles.", "Stores typed static secrets, opaque AWS state, Datadog OAuth state, OpenAI Codex OAuth state, revisions, and raw handles."),
    ("型付きの静的な秘密情報、AWS の不透明な状態、Datadog OAuth の状態、リビジョン、未加工のハンドルを保存します。", "型付きの静的な秘密情報、AWS の不透明な状態、Datadog OAuth の状態、OpenAI Codex OAuth の状態、リビジョン、未加工のハンドルを保存します。"),
    ("GitHub device login, AWS login, or Datadog OAuth reached by fixed trusted-host drivers.", "GitHub device login, AWS login, Datadog OAuth, OpenAI device OAuth, or Anthropic setup-token reached by fixed trusted-host drivers."),
    ("GitHub のデバイスログイン、AWS のログイン、または Datadog OAuth", "GitHub のデバイスログイン、AWS のログイン、Datadog OAuth、OpenAI デバイス OAuth、または Anthropic setup-token"),
]:
    replace(credential_arch, old, new, required=False)

replace(
    credential_arch,
    '''  {
    id: "datadog-token",
    label: {
      en: "Datadog token endpoint",''',
    '''  {
    id: "openai-token",
    label: {
      en: "OpenAI token endpoint",
      ja: "OpenAI トークンエンドポイント",
    },
    role: { en: "Fixed OAuth refresh destination", ja: "固定された OAuth 更新先" },
    detail: {
      en: "The compiled Codex OAuth plan refreshes only at its reviewed exact HTTPS authority after OPA allow.",
      ja: "組み込みの Codex OAuth プランが、OPA の許可後に、レビュー済みの完全一致 HTTPS 接続先だけで更新します。",
    },
    kind: "external",
  },
  {
    id: "datadog-token",
    label: {
      en: "Datadog token endpoint",''',
)
replace(
    credential_arch,
    '''  {
    id: "datadog-refresh",
    from: "broker",
    to: "datadog-token",''',
    '''  {
    id: "openai-refresh",
    from: "broker",
    to: "openai-token",
    label: { en: "exact Codex OAuth refresh", ja: "完全一致する Codex OAuth 更新" },
    kind: "secret",
    path: "M 580 515 C 655 515, 720 505, 790 505",
    bidirectional: true,
  },
  {
    id: "datadog-refresh",
    from: "broker",
    to: "datadog-token",''',
)
replace(
    credential_arch,
    '''  {
    id: "datadog",
    label: { en: "Datadog OAuth", ja: "Datadog OAuth" },''',
    '''  {
    id: "openai",
    label: { en: "OpenAI Codex OAuth", ja: "OpenAI Codex OAuth" },
    summary: {
      en: "Only after OPA allow, Broker selects the stored Codex access token or performs one exact reviewed OAuth refresh before returning a request-local bearer value.",
      ja: "OPA の許可後にだけ Auth Broker が保存済みの Codex アクセストークンを選ぶか、レビュー済みの完全一致 OAuth 更新を 1 回実行し、リクエスト専用の Bearer 値を返します。",
    },
    routes: [
      "workspace-proxy",
      "broker-introspect",
      "policy",
      "vault-state",
      "openai-refresh",
      "upstream-request",
    ],
    sent: {
      en: "One strict OAuth refresh when required; one request-local bearer value returns to Gateway.",
      ja: "必要な場合だけ厳密な OAuth 更新を 1 回行い、そのリクエストだけで使う Bearer 値を Gateway へ返します。",
    },
    withheld: {
      en: "No Codex executable in Broker, no Workspace OAuth session, no ambient proxy or redirect, and no token in OPA.",
      ja: "Auth Broker 内の Codex 実行ファイル、Workspace 内の OAuth セッション、環境由来のプロキシやリダイレクト、OPA 内のトークンはありません。",
    },
    result: {
      en: "Refreshed state commits before Gateway replaces the exact bearer header and makes one upstream attempt.",
      ja: "更新後の状態を確定してから、Gateway が対象の Bearer ヘッダーだけを置き換え、接続先へ 1 回接続します。",
    },
    failure: {
      en: "Known pre-send failure is 503; post-send ambiguity is non-retryable 409 with a durable barrier; no application-upstream attempt occurs.",
      ja: "送信前の失敗と確認できる場合は 503、送信後に結果を確定できない場合は永続バリアを残して再試行不可の 409 となり、アプリケーション接続先へは送信しません。",
    },
  },
  {
    id: "datadog",
    label: { en: "Datadog OAuth", ja: "Datadog OAuth" },''',
)

credential_map = "docs/architecture-site/src/components/CredentialArchitectureMap.astro"
replace(credential_map, '"datadog-token": "external",', '"datadog-token": "external",\n  "openai-token": "external",')
replace(credential_map, 'grid-template-columns: repeat(4, minmax(0, 1fr));', 'grid-template-columns: repeat(auto-fit, minmax(9rem, 1fr));')

# ---------------------------------------------------------------------------
# Broad provider inventory wording. These are descriptive lists, not schema
# claims; add the two reviewed provider plans wherever the old trio was named.
# ---------------------------------------------------------------------------
for path in (SITE / "src").rglob("*"):
    if not path.is_file() or path.suffix not in {".astro", ".md", ".mdx", ".mjs", ".ts"}:
        continue
    text = path.read_text(encoding="utf-8")
    replacements = {
        "GitHub, AWS, and Datadog": "GitHub, AWS, Datadog, OpenAI, and Anthropic",
        "GitHub, AWS, or Datadog": "GitHub, AWS, Datadog, OpenAI, or Anthropic",
        "GitHub、AWS、Datadog": "GitHub、AWS、Datadog、OpenAI、Anthropic",
        "GitHub/AWS/pup": "GitHub/AWS/pup/Codex/Claude",
        "gh, aws, or pup": "gh, aws, pup, Codex, or Claude",
        "gh、aws、pup": "gh、aws、pup、Codex、Claude",
        "AWS/Datadog plan vocabulary": "AWS/Datadog/OpenAI plan vocabulary",
        "AWS/Datadog 計画の語彙": "AWS/Datadog/OpenAI 計画の語彙",
        "AWS and Datadog plans only": "AWS, Datadog, OpenAI, and Anthropic reviewed plans only",
        "AWS と Datadog 処理だけ": "AWS、Datadog、OpenAI、Anthropic のレビュー済み処理だけ",
    }
    updated = text
    for old, new in replacements.items():
        updated = updated.replace(old, new)
    if updated != text:
        path.write_text(updated, encoding="utf-8")

# Authentication guide: add the two provider-specific flows and truthful
# activation guidance close to the existing built-in login instructions.
for path in [
    "docs/architecture-site/src/content/docs/guides/authentication.mdx",
    "docs/architecture-site/src/content/docs/ja/guides/authentication.mdx",
]:
    text = file(path).read_text(encoding="utf-8")
    if "/ja/" in path:
        text = text.replace(
            "## GitHub、AWS、Datadog、OpenAI、Anthropic の組み込み認証",
            "## GitHub、AWS、Datadog、OpenAI、Anthropic の組み込み認証",
        )
        marker = "<CredentialArchitectureMap headingLevel={2} initial=\"acquisition\" />"
        addition = '''<h3 id="openai">OpenAI / Codex</h3>

信頼するホストに正確な Codex 0.146.0 を用意し、次を実行します。

```sh
tobari auth login --provider openai --context default
```

Tobari は所有者専用の一時 `HOME` と `CODEX_HOME` で、固定した `codex login --device-auth` の経路を実行します。利用者は通常の ChatGPT ブラウザ操作を完了します。ファイル保存された OAuth セッションだけを厳密に取得し、一時ホームを削除してから暗号化した Context 保管庫へ確定します。Workspace 内の Codex はこの取得処理には使いません。アクセストークンの更新が必要な場合は、OPA の許可後に Auth Broker が組み込みの完全一致更新を行います。

<h3 id="anthropic">Anthropic / Claude Code</h3>

信頼するホストに正確な Claude Code 2.1.220 を用意し、次を実行します。

```sh
tobari auth login --provider anthropic --context default
```

Tobari は所有者専用の一時ホームと上限付きの非公開 PTY で、固定した `claude setup-token` を実行します。秘密ではない案内だけを表示し、取得した setup token はターミナルへ再表示せず、後片付けと実行ファイルの再検査に成功した後で確定します。Anthropic のこのプランは静的なトークン解決であり、Broker 内で更新処理を行いません。

'''
        if addition not in text:
            text = text.replace(marker, addition + marker)
        text = text.replace(
            "ログイン後、その Context に結び付いた Workspace へ入り直します。\n\n```sh\ntobari --context default\n```",
            "ログイン後は、結果または `tobari auth status --format json` の `workspace_activation` を確認します。Tobari が投影を `missing` または `stale` と確定できた Workspace に限り、正確な作業ディレクトリと再入室引数を返します。`ready` には操作が不要で、`unavailable`／`unresolved` では推測した再入室を案内しません。すでに存在しない認証情報のログアウトは `no_change` です。",
        )
    else:
        marker = '<CredentialArchitectureMap headingLevel={2} initial="acquisition" />'
        addition = '''<h3 id="openai">OpenAI / Codex</h3>

Install exact Codex 0.146.0 in a reviewed trusted-host executable root, then run:

```sh
tobari auth login --provider openai --context default
```

Tobari runs the fixed Codex device-login path in owner-only temporary `HOME` and `CODEX_HOME` directories. The user completes the ordinary ChatGPT browser step. Tobari accepts only the strict file-backed OAuth session, removes the temporary home, and then commits canonical encrypted Context state. The Workspace copy of Codex is not used for acquisition. When refresh is needed, Auth Broker performs the compiled exact OAuth refresh only after OPA allow.

<h3 id="anthropic">Anthropic / Claude Code</h3>

Install exact Claude Code 2.1.220 in a reviewed trusted-host executable root, then run:

```sh
tobari auth login --provider anthropic --context default
```

Tobari runs fixed `claude setup-token` in an owner-only temporary home through a bounded private PTY. It forwards only recognized non-secret instructions, never reprints the captured token, and commits one inference token only after cleanup and executable revalidation. This plan is static token resolution; Broker does not refresh it.

'''
        if addition not in text:
            text = text.replace(marker, addition + marker)
        text = text.replace(
            "After login, re-enter a Workspace bound to that Context:\n\n```sh\ntobari --context default\n```",
            "After login, inspect the result or `tobari auth status --format json`. Only a Workspace whose projection is authoritatively `missing` or `stale` receives an exact working-directory and argv re-entry action. `ready` needs no action; `unavailable` and `unresolved` do not invent one. Logging out an already absent provider reports `no_change`.",
        )
    file(path).write_text(text, encoding="utf-8")

# ---------------------------------------------------------------------------
# Current implementation/release state shown on the overview and versions page.
# ---------------------------------------------------------------------------
replace_regex(
    "docs/architecture-site/src/content/docs/ja/start/overview.mdx",
    r"\*\*実装済み:\*\*.*?この文書では、",
    '''**実装済み:** 再利用できる Workspace、プロジェクト専用ネットワーク、Gateway と OPA による通信制御、同じ Workspace セッションを保ったポリシーレビュー、Context ごとのランタイム、ツール自身による認証、保持型アダプター、静的 Broker プロバイダー、レビュー済みの GitHub・AWS・Datadog・OpenAI・Anthropic 認証情報取得処理です。組み込みの動的プランは、AWS SigV4、Datadog US1 OAuth 更新、OpenAI Codex OAuth 更新です。Anthropic setup-token は静的に解決し、更新しません。

現在のソース契約は Gateway API 4 と Auth Broker API 3 です。`versions.env` に固定された公開イメージは過去の API 3 / API 2 であり、このソースとは互換ではありません。標準起動は古い組み合わせを拒否します。対応する不変な公開成果物が揃うまでは、[インストール](../install/)にある `task build:dev` と `bin/tobari-dev` を使ってください。[コンポーネントのバージョン](../../reference/component-versions/)では、要求 API と選択中のイメージ識別情報を別々に確認できます。

この文書では、''',
)
replace_regex(
    "docs/architecture-site/src/content/docs/start/overview.mdx",
    r"\*\*Implemented:\*\*.*?This documentation does not present",
    '''**Implemented:** reusable Workspaces, dedicated project networks, Gateway/OPA enforcement, same-session policy review, Context-owned runtimes, tool-native authentication, the retained managed adapter, static Broker providers, and reviewed GitHub, AWS, Datadog, OpenAI, and Anthropic acquisition. Closed dynamic plans cover AWS SigV4, Datadog US1 OAuth refresh, and OpenAI Codex OAuth refresh. Anthropic setup-token is static and does not refresh.

The current source contract requires Gateway API 4 and Auth Broker API 3. The immutable public pins in `versions.env` are the historical API 3 / API 2 publications and are incompatible with this source. Standard startup rejects them. Until compatible immutable indexes are published, use `task build:dev` and `bin/tobari-dev` from [Install](../install/). [Component versions](../../reference/component-versions/) keeps required APIs separate from selected image identities.

This documentation does not present''',
)

for path in [
    "docs/architecture-site/src/content/docs/reference/component-versions.mdx",
    "docs/architecture-site/src/content/docs/ja/reference/component-versions.mdx",
]:
    text = file(path).read_text(encoding="utf-8")
    if "/ja/" in path:
        text = text.replace("```console\ntobari version\n```", "```console\ntobari version --format json\n```")
        insert = '''
## 現在のサービス API 互換性

生成表が示す Gateway API 4 と Auth Broker API 3 は、現在のソースが要求するインターフェースです。一方、表の Gateway/Auth Broker ダイジェストは、`versions.env` が選ぶ過去の API 3 / API 2 公開成果物です。これは同じ種類の値ではありません。

通常のバイナリは、この不一致を起動前に拒否します。現在のソースを動かす場合は `task build:dev` で開発用イメージを作り、`bin/tobari-dev version --format json` で `development` リゾルバーと互換性を確認してください。開発用イメージは、公開リリースの正本ではありません。
'''
        marker = "## CLI のバージョン表示"
    else:
        text = text.replace("```console\ntobari version\n```", "```console\ntobari version --format json\n```")
        insert = '''
## Current service API compatibility

The generated Gateway API 4 and Auth Broker API 3 rows describe the interfaces required by the current source. The Gateway/Auth Broker digests in the component table are the historical API 3 / API 2 publications selected by `versions.env`. These are different kinds of identity.

A standard binary rejects that mismatch before startup. To exercise this source, run `task build:dev` and inspect `bin/tobari-dev version --format json` for the `development` resolver and compatible selected APIs. Development images are not publication authority.
'''
        marker = "## CLI version output"
    if insert.strip() not in text:
        text = text.replace(marker, insert + "\n" + marker)
    file(path).write_text(text, encoding="utf-8")

# ---------------------------------------------------------------------------
# Public schema/state pages that changed in the selected source.
# ---------------------------------------------------------------------------
for path in [
    "docs/architecture-site/src/content/docs/reference/configuration-and-state.mdx",
    "docs/architecture-site/src/content/docs/ja/reference/configuration-and-state.mdx",
    "docs/architecture-site/src/content/docs/reference/json-schemas.mdx",
    "docs/architecture-site/src/content/docs/ja/reference/json-schemas.mdx",
    "docs/architecture-site/src/content/docs/reference/glossary.mdx",
    "docs/architecture-site/src/content/docs/ja/reference/glossary.mdx",
]:
    text = file(path).read_text(encoding="utf-8")
    changes = {
        "Context manifest schema 4": "Context manifest schema 5",
        "Context report schema 6": "Context report schema 8",
        "public Context report schema 6": "public Context report schema 8",
        "Schema 4 is persisted host configuration. Schema 6": "Schema 5 is persisted host configuration. Schema 8",
        "schema 4; public Context report is schema 6": "schema 5; public Context report is schema 8",
        "スキーマ 4 と Context レポートのスキーマ 6": "スキーマ 5 と Context レポートのスキーマ 8",
        "スキーマ 4 は永続化するホスト設定です。スキーマ 6": "スキーマ 5 は永続化するホスト設定です。スキーマ 8",
        "スキーマ 4、公開 Context レポートはスキーマ 6": "スキーマ 5、公開 Context レポートはスキーマ 8",
        '"schema_version": 6, "context"': '"schema_version": 8, "context"',
        "schema 4 CLI presentation": "schema 5 CLI presentation",
        "スキーマ 4 CLI presentation": "スキーマ 5 CLI presentation",
    }
    for old, new in changes.items():
        text = text.replace(old, new)
    text = text.replace(
        "Datadog OAuth state, revisions, durable barriers, and raw handles",
        "Datadog OAuth state, OpenAI Codex OAuth state, Anthropic static token state, revisions, durable barriers, and raw handles",
    ).replace(
        "Datadog OAuth 状態、リビジョン、永続する再実行防止状態、元のハンドル",
        "Datadog OAuth 状態、OpenAI Codex OAuth 状態、Anthropic の静的トークン状態、リビジョン、永続する再実行防止状態、元のハンドル",
    )
    file(path).write_text(text, encoding="utf-8")

# Update Context manifest/report wording in the component-independent tables,
# where the old numbers appeared without the full label.
for path in [
    "docs/architecture-site/src/content/docs/reference/configuration-and-state.mdx",
    "docs/architecture-site/src/content/docs/ja/reference/configuration-and-state.mdx",
]:
    text = file(path).read_text(encoding="utf-8")
    text = text.replace("**Schema 4.** It holds stable Context ID", "**Schema 5.** It holds stable Context ID")
    text = text.replace("**schema 6**", "**schema 8**")
    text = text.replace("**スキーマ 4**。安定した Context ID", "**スキーマ 5**。安定した Context ID")
    text = text.replace("**スキーマ 6**", "**スキーマ 8**")
    file(path).write_text(text, encoding="utf-8")

# ---------------------------------------------------------------------------
# Troubleshooting: dependency-aware doctor, same-session policy review, and
# truthful auth activation guidance.
# ---------------------------------------------------------------------------
for path in [
    "docs/architecture-site/src/content/docs/guides/troubleshooting.mdx",
    "docs/architecture-site/src/content/docs/ja/guides/troubleshooting.mdx",
]:
    text = file(path).read_text(encoding="utf-8")
    if "/ja/" in path:
        text = text.replace(
            "Workspace から出て `tobari policy review` を実行します。",
            "Workspace とエージェントを動かしたまま、別の信頼するホストターミナルで `tobari policy review` を実行します。",
        ).replace(
            "Workspace から出て、ホストで完全一致の通信をレビューして判断し、意図的に再試行します。",
            "Workspace は動かしたまま、別のホストターミナルで完全一致の通信をレビューして判断し、元の Workspace で意図的に再試行します。",
        )
        doctor_note = '''
`doctor` の各行は独立しているとは限りません。前提となる Docker、パス、イメージ、またはクラスター観測を実行できない場合、後続の検査は `blocked` になります。blocked を別の障害と数えず、最初に失敗した前提を直してください。`doctor` 自体は読み取り専用で、修復や確認用コンテナの起動を行いません。
'''
        marker = "## よくある復旧パターン"
    else:
        text = text.replace(
            "leave the Workspace and run `tobari policy review`",
            "keep the Workspace running and use a separate trusted-host terminal for `tobari policy review`",
        ).replace(
            "Leave the Workspace, review the exact effect on the host, decide, then retry deliberately.",
            "Keep the Workspace running, review the exact effect in a separate trusted-host terminal, then retry deliberately in the original session.",
        )
        doctor_note = '''
Doctor rows are dependency-aware. When Docker, a required path, an image, or a cluster observation is unavailable, dependent checks are `blocked`; do not count them as separate failures. Repair the first failed prerequisite. `doctor` is read-only and starts neither repairs nor a policy-test container.
'''
        marker = "## Common recovery patterns"
    if doctor_note.strip() not in text:
        text = text.replace(marker, doctor_note + "\n" + marker)
    file(path).write_text(text, encoding="utf-8")

# ---------------------------------------------------------------------------
# Browser assertions for the expanded provider map and new same-session flow.
# ---------------------------------------------------------------------------
test_path = "docs/architecture-site/tests/site.spec.ts"
text = file(test_path).read_text(encoding="utf-8")
text = text.replace('await expect(map.locator(".pairing-row")).toHaveCount(4);', 'await expect(map.locator(".pairing-row")).toHaveCount(6);')
text = text.replace(
    '["datadog", "pup", "pup"],\n      ["chatwork", "cwk", "cwk"],',
    '["datadog", "pup", "pup"],\n      ["openai", "Codex CLI 0.146.0", "codex"],\n      ["anthropic", "Claude Code 2.1.220", "claude"],\n      ["chatwork", "cwk", "cwk"],',
)
file(test_path).write_text(text, encoding="utf-8")

# Regenerate a smaller residual audit after these corrections. Schema numbers
# are no longer treated as stale merely because they are small integers.
patterns = {
    "policy-session-exit": re.compile(
        r"(Workspace から出る.*policy review|leave the Workspace.*policy review|re-enter the Workspace and issue a new request)",
        re.I,
    ),
    "old-provider-inventory": re.compile(
        r"(GitHub、AWS、Datadog(?!、OpenAI)|GitHub, AWS, and Datadog|GitHub, AWS, or Datadog)",
        re.I,
    ),
    "old-context-schema": re.compile(
        r"(Context (?:manifest )?schema 4|Context report schema 6|Context マニフェスト.*スキーマ 4|Context レポート.*スキーマ 6)",
        re.I,
    ),
    "old-version-command": re.compile(r"\btobari version\b(?!\s+--format)", re.I),
}
lines = [
    f"target_source={TARGET}",
    "Residual audit after manual synchronization; every entry requires review before finalization.",
]
for path in sorted((SITE / "src").rglob("*")):
    if not path.is_file() or path.suffix not in {".astro", ".md", ".mdx", ".mjs", ".ts"}:
        continue
    source = path.read_text(encoding="utf-8")
    relative = path.relative_to(ROOT).as_posix()
    for name, pattern in patterns.items():
        for match in pattern.finditer(source):
            line = source.count("\n", 0, match.start()) + 1
            excerpt = " ".join(source[match.start(): match.start() + 180].split())
            lines.append(f"{name}\t{relative}:{line}\t{excerpt}")
(SITE / "docs-sync-audit.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")
