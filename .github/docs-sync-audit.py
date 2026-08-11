from pathlib import Path
import re

ROOT = Path(__file__).resolve().parents[1]
SITE = ROOT / "docs" / "architecture-site"
TARGET = "3a9acc9c264d3e4efca2cd9aafabc9a122b183b8"
SNAPSHOT = SITE / "source-snapshot.txt"

SNAPSHOT.write_text(TARGET + "\n", encoding="utf-8")

# All first-party implementation links on the public site are fixed to the
# selected product snapshot. Keep the paths, but move the commit authority as
# one operation so verify-source can validate every target.
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

patterns = {
    "policy-session-reentry": re.compile(
        r"(Workspace から出る|Workspace へ入り直|leave the Workspace|re-enter the Workspace|use `exit` to return to the host)",
        re.I,
    ),
    "old-provider-inventory": re.compile(
        r"(GitHub、AWS、Datadog|GitHub, AWS, (?:or )?Datadog|GitHub, AWS, and Datadog)",
        re.I,
    ),
    "build-without-identity": re.compile(r"\btobari version\b(?!\s+--format)", re.I),
    "source-build-standard-binary": re.compile(r"\btask build\b|\bbin/tobari\b"),
    "auth-always-reenter": re.compile(
        r"(認証設定を変更したら.*入り直|After authentication changes.*re-enter|requires Workspace re-entry|workspace_reentry_required)",
        re.I,
    ),
    "doctor-flat-failure": re.compile(
        r"(all checks|すべての検査|full diagnostic set|complete check set|diagnostic_failed)",
        re.I,
    ),
    "old-schema-number": re.compile(
        r"(?:schema|スキーマ)\s*(?:version\s*)?(?:1|2|3|4|5|6|7)\b",
        re.I,
    ),
}

lines = [
    f"target_source={TARGET}",
    "This report is temporary and lists wording that may need a manual review after generated data is refreshed.",
]
for path in sorted((SITE / "src").rglob("*")):
    if not path.is_file() or path.suffix not in {".astro", ".md", ".mdx", ".mjs", ".ts"}:
        continue
    text = path.read_text(encoding="utf-8")
    relative = path.relative_to(ROOT).as_posix()
    for name, pattern in patterns.items():
        for match in pattern.finditer(text):
            line = text.count("\n", 0, match.start()) + 1
            excerpt = " ".join(text[match.start(): match.start() + 180].split())
            lines.append(f"{name}\t{relative}:{line}\t{excerpt}")

(SITE / "docs-sync-audit.txt").write_text("\n".join(lines) + "\n", encoding="utf-8")
