#!/usr/bin/env python3
"""Capture a bounded, real pseudo-TTY session for parent-owned evidence.

The raw capture is deliberately written only to the caller-selected artifact
directory.  The repository receives the redacted projection and metadata
schema, never the raw terminal bytes.
"""

from __future__ import annotations

import argparse
import errno
import fcntl
import hashlib
import json
import os
import pathlib
import pty
import re
import select
import signal
import struct
import subprocess
import sys
import termios
import time
from typing import Any, Iterable, Mapping, Sequence


SCHEMA_VERSION = 1
DEFAULT_ROWS = 40
DEFAULT_COLS = 120
DEFAULT_TERM = "xterm-256color"

# Keep ANSI control sequences byte-for-byte intact.  Redaction is applied only
# to printable spans so a public projection still proves redraw and cursor
# movement happened.
ANSI_SEQUENCE = re.compile(
    r"\x1b(?:\[[0-?]*[ -/]*[@-~]|\][^\x07]*(?:\x07|\x1b\\)|[()][0-2A-Z])"
)
HOME_PREFIX = re.escape(os.sep) + r"(?:Users|home|private)" + re.escape(os.sep)
ABSOLUTE_HOME = re.compile(
    r"(?:" + HOME_PREFIX + r"[^\s\x1b" + re.escape(os.sep) + r"]+)"
    r"(?:" + re.escape(os.sep) + r"[^\s\x1b]+)*"
)
CREDENTIAL_URL = re.compile(r"https?://[^/@\s:]+:[^/@\s]+@")
OPAQUE_REFERENCE = re.compile(
    r"\b(?:pcy|prj|ctx|wrk)_[A-Za-z0-9_-]{8,}\b"
    r"|\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[1-5][0-9a-fA-F]{3}-[89abAB][0-9a-fA-F]{3}-[0-9a-fA-F]{12}\b"
)


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _redact_printable(text: str, literal_values: Iterable[str]) -> str:
    for value in literal_values:
        if value:
            text = text.replace(value, "<redacted:value>")
    text = CREDENTIAL_URL.sub("<redacted:credential>@", text)
    text = ABSOLUTE_HOME.sub("<redacted:path>", text)
    text = OPAQUE_REFERENCE.sub("<redacted:opaque>", text)
    return text


def redact_text(text: str, literal_values: Iterable[str] = ()) -> str:
    """Redact sensitive printable values while preserving ANSI sequences."""

    chunks: list[str] = []
    cursor = 0
    for match in ANSI_SEQUENCE.finditer(text):
        chunks.append(_redact_printable(text[cursor : match.start()], literal_values))
        chunks.append(match.group(0))
        cursor = match.end()
    chunks.append(_redact_printable(text[cursor:], literal_values))
    return "".join(chunks)


def redact_bytes(data: bytes, literal_values: Iterable[str] = ()) -> bytes:
    return redact_text(data.decode("utf-8", errors="replace"), literal_values).encode(
        "utf-8"
    )


def _visible_tail(data: bytes, literal_values: Iterable[str], lines: int = 8) -> list[str]:
    visible = ANSI_SEQUENCE.sub("", redact_bytes(data, literal_values).decode("utf-8"))
    visible = visible.replace("\r", "")
    return visible.splitlines()[-lines:]


def _read_available(master: int, output: bytearray) -> bool:
    """Read all currently available master bytes; return whether it closed."""

    closed = False
    while True:
        try:
            data = os.read(master, 65536)
        except OSError as error:
            if error.errno in (errno.EAGAIN, errno.EWOULDBLOCK):
                break
            if error.errno == errno.EIO:
                closed = True
                break
            raise
        if not data:
            closed = True
            break
        output.extend(data)
        if len(data) < 65536:
            break
    return closed


def _wait_status(pid: int) -> tuple[int | None, bool]:
    waited, status = os.waitpid(pid, os.WNOHANG)
    if waited == 0:
        return None, False
    return status, True


def _status_fields(status: int | None) -> tuple[int | None, int | None]:
    if status is None:
        return None, None
    if os.WIFEXITED(status):
        return os.WEXITSTATUS(status), None
    if os.WIFSIGNALED(status):
        return None, os.WTERMSIG(status)
    return None, None


def _repo_root() -> pathlib.Path | None:
    try:
        result = subprocess.run(
            ["git", "rev-parse", "--show-toplevel"],
            check=True,
            capture_output=True,
            text=True,
        )
    except (OSError, subprocess.CalledProcessError):
        return None
    return pathlib.Path(result.stdout.strip()).resolve()


def _assert_external_output(output_dir: pathlib.Path) -> None:
    root = _repo_root()
    if root is None:
        return
    try:
        output_dir.relative_to(root)
    except ValueError:
        return
    raise ValueError(
        "PTY evidence output must be outside the repository; "
        "use a task-owned temporary directory"
    )


def _load_events(path: pathlib.Path) -> list[dict[str, Any]]:
    document = json.loads(path.read_text(encoding="utf-8"))
    if isinstance(document, Mapping):
        document = document.get("events")
    if not isinstance(document, list):
        raise ValueError("events must be a JSON array or an object with an events array")

    events: list[dict[str, Any]] = []
    for index, event in enumerate(document):
        if not isinstance(event, Mapping):
            raise ValueError(f"event {index} is not an object")
        delay = event.get("after_ms", 0)
        data = event.get("data", "")
        label = event.get("label", f"input-{index + 1}")
        if not isinstance(delay, (int, float)) or delay < 0:
            raise ValueError(f"event {index} has an invalid after_ms")
        if not isinstance(data, str) or not isinstance(label, str) or not label:
            raise ValueError(f"event {index} requires string data and label")
        events.append({"after_ms": float(delay), "data": data, "label": label})
    return events


def _checkpoint(
    output: bytes,
    label: str,
    kind: str,
    started_ns: int,
    literal_values: Iterable[str],
    input_index: int | None = None,
) -> dict[str, Any]:
    checkpoint: dict[str, Any] = {
        "label": label,
        "kind": kind,
        "elapsed_ms": round((time.monotonic_ns() - started_ns) / 1_000_000, 3),
        "output_offset": len(output),
        "output_sha256": sha256(output),
        "visible_tail": _visible_tail(output, literal_values),
    }
    if input_index is not None:
        checkpoint["input_index"] = input_index
    return checkpoint


def capture(
    argv: Sequence[str],
    output_dir: pathlib.Path,
    events: Sequence[Mapping[str, Any]],
    *,
    rows: int = DEFAULT_ROWS,
    cols: int = DEFAULT_COLS,
    term: str = DEFAULT_TERM,
    timeout_seconds: float = 60.0,
    literal_values: Iterable[str] = (),
) -> dict[str, Any]:
    """Run argv under a real PTY and write the evidence bundle."""

    if not argv:
        raise ValueError("a command is required after --")
    if rows <= 0 or cols <= 0:
        raise ValueError("terminal rows and cols must be positive")
    if timeout_seconds <= 0:
        raise ValueError("timeout_seconds must be positive")

    output_dir = output_dir.expanduser().resolve()
    _assert_external_output(output_dir)
    output_dir.mkdir(parents=True, exist_ok=False)
    raw_path = output_dir / "transcript.raw"
    redacted_path = output_dir / "transcript.redacted"
    metadata_path = output_dir / "metadata.json"

    literal_values = tuple(literal_values)
    pid, master = pty.fork()
    if pid == 0:
        child_environment = os.environ.copy()
        child_environment["TERM"] = term
        os.execvpe(argv[0], list(argv), child_environment)

    output = bytearray()
    checkpoints: list[dict[str, Any]] = []
    inputs: list[dict[str, Any]] = []
    started_ns = time.monotonic_ns()
    checkpoint_output = b""
    status: int | None = None
    timed_out = False
    master_closed = False
    event_index = 0
    next_event_ns = started_ns + (
        int(float(events[0]["after_ms"]) * 1_000_000) if events else 0
    )

    fcntl.ioctl(master, termios.TIOCSWINSZ, struct.pack("HHHH", rows, cols, 0, 0))
    os.set_blocking(master, False)
    checkpoints.append(_checkpoint(bytes(output), "spawn", "process", started_ns, literal_values))

    try:
        while status is None:
            now_ns = time.monotonic_ns()
            elapsed_seconds = (now_ns - started_ns) / 1_000_000_000
            if elapsed_seconds >= timeout_seconds:
                timed_out = True
                try:
                    os.kill(pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                _, status = os.waitpid(pid, 0)
                break

            wait_seconds = 0.05
            if event_index < len(events):
                wait_seconds = min(wait_seconds, max(0.0, (next_event_ns - now_ns) / 1_000_000_000))
            readable, _, _ = select.select([master], [], [], wait_seconds)
            if master in readable:
                master_closed = _read_available(master, output) or master_closed

            now_ns = time.monotonic_ns()
            while event_index < len(events) and now_ns >= next_event_ns:
                event = events[event_index]
                data = str(event["data"]).encode("utf-8")
                delivered = False
                if not master_closed:
                    try:
                        os.write(master, data)
                        delivered = True
                    except OSError as error:
                        if error.errno not in (errno.EIO, errno.EPIPE):
                            raise
                        master_closed = True
                inputs.append(
                    {
                        "label": str(event["label"]),
                        "elapsed_ms": round((now_ns - started_ns) / 1_000_000, 3),
                        "data": redact_text(str(event["data"]), literal_values),
                        "data_sha256": sha256(data),
                        "byte_length": len(data),
                        "delivered": delivered,
                    }
                )
                checkpoints.append(
                    _checkpoint(
                        bytes(output),
                        str(event["label"]),
                        "after_input",
                        started_ns,
                        literal_values,
                        event_index,
                    )
                )
                event_index += 1
                if event_index < len(events):
                    next_event_ns += int(float(events[event_index]["after_ms"]) * 1_000_000)

            status, exited = _wait_status(pid)
            if exited:
                break

        if status is None:
            _, status = os.waitpid(pid, 0)

        while not master_closed:
            master_closed = _read_available(master, output) or master_closed
            if not master_closed:
                time.sleep(0.01)
    finally:
        os.close(master)

    exit_code, signal_number = _status_fields(status)
    raw_bytes = bytes(output)
    redacted_bytes = redact_bytes(raw_bytes, literal_values)
    checkpoints.append(
        _checkpoint(raw_bytes, "exit", "process", started_ns, literal_values)
    )
    raw_path.write_bytes(raw_bytes)
    redacted_path.write_bytes(redacted_bytes)

    metadata: dict[str, Any] = {
        "schema_version": SCHEMA_VERSION,
        "terminal": {"rows": rows, "cols": cols, "term": term},
        "command": {
            "program": pathlib.Path(argv[0]).name,
            "argv_sha256": sha256(json.dumps(list(argv), separators=(",", ":")).encode()),
        },
        "result": {
            "exit_code": exit_code,
            "signal": signal_number,
            "timed_out": timed_out,
            "elapsed_ms": round((time.monotonic_ns() - started_ns) / 1_000_000, 3),
        },
        "raw": {
            "path": raw_path.name,
            "bytes": len(raw_bytes),
            "sha256": sha256(raw_bytes),
        },
        "redacted": {
            "path": redacted_path.name,
            "bytes": len(redacted_bytes),
            "sha256": sha256(redacted_bytes),
            "ansi_preserved": True,
        },
        "inputs": inputs,
        "checkpoints": checkpoints,
    }
    metadata_path.write_text(
        json.dumps(metadata, ensure_ascii=False, indent=2, sort_keys=True) + "\n",
        encoding="utf-8",
    )
    return metadata


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        description="Capture a bounded command under a real PTY into an external evidence bundle."
    )
    parser.add_argument("--output-dir", required=True, type=pathlib.Path)
    parser.add_argument("--events", required=True, type=pathlib.Path)
    parser.add_argument("--rows", type=int, default=DEFAULT_ROWS)
    parser.add_argument("--cols", type=int, default=DEFAULT_COLS)
    parser.add_argument("--term", default=DEFAULT_TERM)
    parser.add_argument("--timeout-seconds", type=float, default=60.0)
    parser.add_argument(
        "--redact-value", action="append", default=[], help="literal value to remove from projection"
    )
    parser.add_argument("command", nargs=argparse.REMAINDER)
    return parser


def main(argv: Sequence[str] | None = None) -> int:
    args = _parser().parse_args(argv)
    command = list(args.command)
    if command and command[0] == "--":
        command = command[1:]
    try:
        metadata = capture(
            command,
            args.output_dir,
            _load_events(args.events),
            rows=args.rows,
            cols=args.cols,
            term=args.term,
            timeout_seconds=args.timeout_seconds,
            literal_values=args.redact_value,
        )
    except (OSError, ValueError, json.JSONDecodeError) as error:
        print(f"pty evidence: {error}", file=sys.stderr)
        return 2
    print(
        json.dumps(
            {
                "artifact_dir": str(args.output_dir.expanduser().resolve()),
                "raw_sha256": metadata["raw"]["sha256"],
                "redacted_sha256": metadata["redacted"]["sha256"],
                "exit_code": metadata["result"]["exit_code"],
            },
            sort_keys=True,
        )
    )
    return 0 if metadata["result"]["exit_code"] == 0 else 1


if __name__ == "__main__":
    raise SystemExit(main())
