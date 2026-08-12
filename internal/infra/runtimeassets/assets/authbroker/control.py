"""Internal docker-exec control client; secrets are stdin-only."""

from __future__ import annotations

import argparse
import json
import sys
from typing import Any

from . import SCHEMA_VERSION
from .daemon import DEFAULT_CONTROL_SOCKET
from .protocol import MAX_SECRET_BYTES, ProtocolError, call_unix_socket


def _read_stdin(limit: int, exact: int | None = None) -> bytes:
    payload = sys.stdin.buffer.read(limit + 1)
    if len(payload) > limit or (exact is not None and len(payload) != exact) or not payload:
        raise ProtocolError("invalid_stdin")
    return payload


def _bindings(value: str) -> Any:
    try:
        parsed = json.loads(value)
    except json.JSONDecodeError:
        raise argparse.ArgumentTypeError("bindings must be JSON") from None
    if not isinstance(parsed, list):
        raise argparse.ArgumentTypeError("bindings must be a JSON array")
    return parsed


def _request(arguments: argparse.Namespace) -> tuple[dict[str, Any], bytes]:
    base: dict[str, Any] = {"schema_version": SCHEMA_VERSION, "op": arguments.operation}
    if arguments.operation == "health":
        return base, b""
    if arguments.operation == "unlock":
        key = _read_stdin(32, exact=32)
        base["key_length"] = len(key)
        return base, key
    if arguments.operation == "status":
        base.update(context_id=arguments.context_id, provider=arguments.provider)
        return base, b""
    if arguments.operation == "import":
        secret = _read_stdin(MAX_SECRET_BYTES)
        base.update(
            context_id=arguments.context_id,
            provider=arguments.provider,
            secret_length=len(secret),
        )
        return base, secret
    if arguments.operation == "login":
        if arguments.provider != "github":
            raise ProtocolError("invalid_provider")
        secret = _read_stdin(MAX_SECRET_BYTES)
        base.update(
            context_id=arguments.context_id,
            provider=arguments.provider,
            secret_length=len(secret),
            account_label=arguments.account_label,
        )
        return base, secret
    if arguments.operation == "logout":
        base.update(context_id=arguments.context_id, provider=arguments.provider)
        return base, b""
    if arguments.operation == "issue_handle":
        base.update(
            context_id=arguments.context_id,
            project_id=arguments.project_id,
            provider=arguments.provider,
            bindings=arguments.bindings,
        )
        return base, b""
    if arguments.operation == "binding_status":
        base.update(
            context_id=arguments.context_id,
            project_id=arguments.project_id,
            provider=arguments.provider,
            revision=arguments.revision,
            bindings=arguments.bindings,
        )
        return base, b""
    raise ProtocolError("unknown_operation")


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(prog="tobari-auth-control")
    parser.add_argument("--socket", default=DEFAULT_CONTROL_SOCKET)
    subparsers = parser.add_subparsers(dest="operation", required=True)
    subparsers.add_parser("health")
    subparsers.add_parser("unlock")
    for operation in ("status", "import", "logout"):
        command = subparsers.add_parser(operation)
        command.add_argument("--context-id", required=True)
        command.add_argument("--provider", required=True)
    login = subparsers.add_parser("login")
    login.add_argument(
        "--provider",
        required=True,
        choices=("github",),
    )
    login.add_argument("--context-id", required=True)
    login.add_argument("--account-label", required=True)
    issue = subparsers.add_parser("issue_handle")
    issue.add_argument("--context-id", required=True)
    issue.add_argument("--project-id", required=True)
    issue.add_argument("--provider", required=True)
    issue.add_argument("--bindings", required=True, type=_bindings)
    binding_status = subparsers.add_parser("binding_status")
    binding_status.add_argument("--context-id", required=True)
    binding_status.add_argument("--project-id", required=True)
    binding_status.add_argument("--provider", required=True)
    binding_status.add_argument("--revision", required=True)
    binding_status.add_argument("--bindings", required=True, type=_bindings)
    return parser


def main(argv: list[str] | None = None) -> int:
    try:
        arguments = _parser().parse_args(argv)
        request, raw_payload = _request(arguments)
        response = call_unix_socket(arguments.socket, request, raw_payload)
    except (ProtocolError, OSError) as error:
        code = error.code if isinstance(error, ProtocolError) else "transport_error"
        response = {
            "schema_version": SCHEMA_VERSION,
            "ok": False,
            "error": {"code": code},
        }
    encoded = json.dumps(response, separators=(",", ":"), sort_keys=True)
    sys.stdout.write(encoded + "\n")
    return 0 if response.get("ok") is True else 1


if __name__ == "__main__":
    raise SystemExit(main())
