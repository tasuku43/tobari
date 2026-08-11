from __future__ import annotations

import atexit
import re
from pathlib import Path


@atexit.register
def normalize_documentation_sync_output() -> None:
    root = Path.cwd() / "docs" / "architecture-site" / "src" / "content" / "docs"
    if not root.exists():
        return

    def synchronize_fences(english_path: Path, japanese_path: Path) -> None:
        pattern = re.compile(r"^```[^\n]*\n.*?^```[ \t]*$", re.M | re.S)
        english = english_path.read_text(encoding="utf-8")
        japanese = japanese_path.read_text(encoding="utf-8")
        english_blocks = pattern.findall(english)
        japanese_blocks = pattern.findall(japanese)
        if len(english_blocks) != len(japanese_blocks):
            raise RuntimeError(
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
        text = text.replace(
            "## CLI version behavior",
            section + "## CLI version behavior",
        )
    text = text.replace(
        "```console\ntobari version\n```",
        "```console\ntobari version --format json\n```",
    )
    english_versions.write_text(text, encoding="utf-8")
