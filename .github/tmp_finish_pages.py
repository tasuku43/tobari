from pathlib import Path

root = Path("docs/architecture-site/src/content/docs/ja")

replacements = {
    root / "guides/authentication.mdx": [
        ("# または: github / aws / datadog", "# or: github / aws / datadog"),
    ],
    root / "start/authentication-setup.mdx": [
        ("# または: github / aws / datadog", "# or: github / aws / datadog"),
        ("# Workspace 内\n<agent-command> <its-login-command>", "# Inside the Workspace\n<agent-command> <its-login-command>"),
    ],
}

for path, pairs in replacements.items():
    text = path.read_text()
    for old, new in pairs:
        if old not in text:
            raise SystemExit(f"missing expected text in {path}: {old!r}")
        text = text.replace(old, new, 1)
    path.write_text(text)

print("Aligned bilingual fenced machine examples.")
