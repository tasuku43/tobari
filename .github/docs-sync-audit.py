from __future__ import annotations

import re
from pathlib import Path

HERE = Path(__file__).resolve().parent
CORE = HERE / "docs-sync-audit-core.py"

old_overview_pattern = (
    r"\*\*" + r"Implemented:" + r"\*\*.*?This documentation does not present"
)
new_overview_pattern = (
    r"\*\*" + r"Implemented(?: now)?:" + r"\*\*.*?This documentation does not present"
)
core_source = CORE.read_text(encoding="utf-8").replace(
    old_overview_pattern,
    new_overview_pattern,
)
exec(
    compile(core_source, str(CORE), "exec"),
    {"__name__": "__main__", "__file__": str(CORE)},
)

root = Path.cwd() / "docs" / "architecture-site" / "src" / "content" / "docs"

# The canonical page used a different sentence than the temporary core patch
# expected. Remove the unconditional re-entry example and replace it with the
# same status-driven activation rule used by the Japanese page.
english_auth = root / "guides" / "authentication.mdx"
text = english_auth.read_text(encoding="utf-8")
text = text.replace(
    "Leave and re-enter a Workspace permanently bound to that Context:\n\n"
    "```sh\n"
    "tobari --context default\n"
    "```",
    "After login, inspect the result or `tobari auth status --format json`. "
    "Only a Workspace whose projection is authoritatively `missing` or `stale` "
    "receives an exact working-directory and argv re-entry action. `ready` needs "
    "no action; `unavailable` and `unresolved` do not invent one. Logging out an "
    "already absent provider reports `no_change`.",
)
english_auth.write_text(text, encoding="utf-8")

# Complete the schema updates that appear in prose tables rather than the
# generated version projection.
residual_replacements = {
    root / "ja" / "reference" / "configuration-and-state.mdx": {
        "保存用 Context マニフェストのスキーマ 4": "保存用 Context マニフェストのスキーマ 5",
        "公開 Context レポートのスキーマ 6": "公開 Context レポートのスキーマ 8",
    },
    root / "reference" / "json-schemas.mdx": {
        "| Context manifest                       | Schema 4": "| Context manifest                       | Schema 5",
        "Context schema 4 and public Context report schema 8": "Context schema 5 and public Context report schema 8",
    },
    root / "ja" / "reference" / "json-schemas.mdx": {
        "| Context マニフェスト                           | スキーマ 4": "| Context マニフェスト                           | スキーマ 5",
    },
}
for path, replacements in residual_replacements.items():
    value = path.read_text(encoding="utf-8")
    for old, new in replacements.items():
        value = value.replace(old, new)
    path.write_text(value, encoding="utf-8")

# Prefer the structured identity form in the explanatory check at the bottom
# of the versions page; it includes resolver/API compatibility, not only dev.
for path, replacements in {
    root / "reference" / "component-versions.mdx": {
        "What does <code>tobari version</code> normally report?": "What does <code>tobari version --format json</code> identify?",
        "`dev`, with an optional source commit only when the build supplied it. Release builds inject the validated release identity.": "The CLI version and source identity, the selected resolver, required and selected service APIs, and whether the pair is compatible. Release builds also carry their validated release identity.",
    },
    root / "ja" / "reference" / "component-versions.mdx": {
        "<code>tobari version</code> は通常何を表示しますか？": "<code>tobari version --format json</code> では何を確認できますか？",
        "`dev` です。ビルド時に値を渡した場合だけ、ソースコミットも表示します。リリースビルドでは、検証済みのリリース識別情報を埋め込みます。": "CLI とソースの識別情報、選択したリゾルバー、必要 API と選択中 API、組み合わせの互換性を確認できます。リリースビルドには検証済みのリリース識別情報も入ります。",
    },
}.items():
    value = path.read_text(encoding="utf-8")
    for old, new in replacements.items():
        value = value.replace(old, new)
    path.write_text(value, encoding="utf-8")


def synchronize_fences(english_path: Path, japanese_path: Path) -> None:
    pattern = re.compile(r"^```[^\n]*\n.*?^```[ \t]*$", re.M | re.S)
    english = english_path.read_text(encoding="utf-8")
    japanese = japanese_path.read_text(encoding="utf-8")
    english_blocks = pattern.findall(english)
    japanese_blocks = pattern.findall(japanese)
    if len(english_blocks) != len(japanese_blocks):
        raise SystemExit(
            f"fenced example count differs: {english_path}={len(english_blocks)} "
            f"{japanese_path}={len(japanese_blocks)}"
        )
    iterator = iter(english_blocks)
    japanese_path.write_text(
        pattern.sub(lambda _: next(iterator), japanese),
        encoding="utf-8",
    )


synchronize_fences(
    root / "start" / "authentication-setup.mdx",
    root / "ja" / "start" / "authentication-setup.mdx",
)
synchronize_fences(
    root / "guides" / "authentication.mdx",
    root / "ja" / "guides" / "authentication.mdx",
)

english_versions = root / "reference" / "component-versions.mdx"
text = english_versions.read_text(encoding="utf-8")
section = """## Current service API compatibility

The generated Gateway API 4 and Auth Broker API 3 rows describe the interfaces required by the current source. The Gateway/Auth Broker digests in the component table are the historical API 3 / API 2 publications selected by `versions.env`. These are different kinds of identity.

A standard binary rejects that mismatch before startup. To exercise this source, run `task build:dev` and inspect `bin/tobari-dev version --format json` for the `development` resolver and compatible selected APIs. Development images are not publication authority.

"""
if section.strip() not in text:
    text = text.replace("## CLI version behavior", section + "## CLI version behavior")
text = text.replace(
    "```console\ntobari version\n```",
    "```console\ntobari version --format json\n```",
)
english_versions.write_text(text, encoding="utf-8")
