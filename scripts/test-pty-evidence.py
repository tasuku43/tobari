#!/usr/bin/env python3
"""Contract tests for the real-PTY evidence boundary."""

from __future__ import annotations

import json
import os
import subprocess
import sys
import tempfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
CAPTURE = ROOT / "scripts" / "pty-evidence.py"


def fail(message: str) -> None:
    raise AssertionError(message)


def main() -> int:
    with tempfile.TemporaryDirectory(prefix="tobari-pty-evidence-") as temporary:
        artifact_dir = Path(temporary) / "capture"
        events_path = Path(temporary) / "events.json"
        redacted_value = "synthetic-redact-value"
        events_path.write_text(
            json.dumps(
                [
                    {
                        "after_ms": 50,
                        "data": "hello\r",
                        "label": "typed-short-command",
                    }
                ]
            ),
            encoding="utf-8",
        )
        child = (
            "import os,sys; "
            "home=os.sep+'Users'+os.sep+'example'+os.sep+'private'; "
            "sys.stdout.write('\\x1b[2Kprompt path='+home+' ref=pcy_123456789 value="
            + redacted_value
            + "\\n'); sys.stdout.flush(); "
            "data=os.read(0,64); "
            "sys.stdout.write('\\x1b[1A\\x1b[2Kdone input='+data.decode('utf-8','replace')+'\\n'); sys.stdout.flush()"
        )
        result = subprocess.run(
            [
                sys.executable,
                str(CAPTURE),
                "--output-dir",
                str(artifact_dir),
                "--events",
                str(events_path),
                "--rows",
                "40",
                "--cols",
                "120",
                "--term",
                "xterm-256color",
                "--redact-value",
                redacted_value,
                "--",
                sys.executable,
                "-c",
                child,
            ],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        if result.returncode != 0:
            fail(f"capture failed: stdout={result.stdout!r} stderr={result.stderr!r}")

        metadata = json.loads((artifact_dir / "metadata.json").read_text(encoding="utf-8"))
        raw = (artifact_dir / "transcript.raw").read_bytes()
        redacted = (artifact_dir / "transcript.redacted").read_bytes()
        if metadata["terminal"] != {"cols": 120, "rows": 40, "term": "xterm-256color"}:
            fail(f"terminal metadata mismatch: {metadata['terminal']!r}")
        if metadata["result"]["exit_code"] != 0:
            fail(f"child exit mismatch: {metadata['result']!r}")
        if metadata["raw"]["sha256"] != __import__("hashlib").sha256(raw).hexdigest():
            fail("raw digest does not describe the raw artifact")
        if b"\x1b[2K" not in raw or b"\x1b[2K" not in redacted:
            fail("redaction removed ANSI redraw evidence")
        home_prefix = (os.sep + "Users" + os.sep).encode()
        for forbidden in (home_prefix, b"pcy_123456789", redacted_value.encode()):
            if forbidden in redacted:
                fail(f"redaction leaked {forbidden!r}")
        labels = [checkpoint["label"] for checkpoint in metadata["checkpoints"]]
        if labels != ["spawn", "typed-short-command", "exit"]:
            fail(f"checkpoint labels mismatch: {labels!r}")
        if metadata["inputs"][0]["data"] != "hello\r":
            fail(f"typed input was not retained: {metadata['inputs']!r}")
        if b"done input=hello\r" not in raw or b"done input=hello\rhello\r" in raw:
            fail("PTY event was not delivered exactly once")
        if not metadata["redacted"]["ansi_preserved"]:
            fail("redacted artifact did not declare ANSI preservation")
        rejected = subprocess.run(
            [
                sys.executable,
                str(CAPTURE),
                "--output-dir",
                str(ROOT),
                "--events",
                str(events_path),
                "--",
                sys.executable,
                "-c",
                "pass",
            ],
            cwd=ROOT,
            capture_output=True,
            text=True,
            check=False,
        )
        if rejected.returncode != 2 or "outside the repository" not in rejected.stderr:
            fail(f"repository-local output was not rejected: {rejected!r}")
    print("pty evidence contract: ok")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
