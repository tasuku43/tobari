"""Strict, bounded framing shared by the Auth Broker sockets and control CLI."""

from __future__ import annotations

import json
import socket
from typing import Any

from . import SCHEMA_VERSION

MAX_FRAME_BYTES = 64 * 1024
MAX_SECRET_BYTES = 32 * 1024


class ProtocolError(Exception):
    """A secret-free protocol failure safe to return to a local caller."""

    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise ProtocolError("invalid_json")
        result[key] = value
    return result


def _reject_constant(_: str) -> None:
    raise ProtocolError("invalid_json")


def decode_document(
    payload: bytes, *, maximum: int = MAX_FRAME_BYTES
) -> dict[str, Any]:
    if (
        isinstance(maximum, bool)
        or not isinstance(maximum, int)
        or maximum < 1
        or not payload
        or len(payload) > maximum
    ):
        raise ProtocolError("invalid_frame")
    try:
        text = payload.decode("utf-8")
        value = json.loads(
            text,
            object_pairs_hook=_reject_duplicate_keys,
            parse_constant=_reject_constant,
        )
    except ProtocolError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise ProtocolError("invalid_json") from None
    if not isinstance(value, dict):
        raise ProtocolError("invalid_request")
    return value


def encode_document(document: dict[str, Any]) -> bytes:
    try:
        payload = json.dumps(
            document, ensure_ascii=True, allow_nan=False, separators=(",", ":"), sort_keys=True
        ).encode("utf-8")
    except (TypeError, ValueError):
        raise ProtocolError("invalid_response") from None
    if not payload or len(payload) > MAX_FRAME_BYTES:
        raise ProtocolError("response_too_large")
    return payload + b"\n"


def require_exact_keys(document: dict[str, Any], expected: set[str]) -> None:
    if set(document) != expected:
        raise ProtocolError("invalid_request")
    if document.get("schema_version") != SCHEMA_VERSION or isinstance(
        document.get("schema_version"), bool
    ):
        raise ProtocolError("unsupported_schema")


def error_document(code: str) -> dict[str, Any]:
    return {"schema_version": SCHEMA_VERSION, "ok": False, "error": {"code": code}}


class ConnectionReader:
    """Bounded reader that preserves bytes received after the JSON newline."""

    def __init__(self, connection: socket.socket):
        self._connection = connection
        self._buffer = bytearray()

    def read_frame(self) -> dict[str, Any]:
        while True:
            newline = self._buffer.find(b"\n")
            if newline >= 0:
                if newline > MAX_FRAME_BYTES:
                    raise ProtocolError("invalid_frame")
                payload = bytes(self._buffer[:newline])
                del self._buffer[: newline + 1]
                return decode_document(payload)
            if len(self._buffer) > MAX_FRAME_BYTES:
                raise ProtocolError("invalid_frame")
            chunk = self._connection.recv(min(8192, MAX_FRAME_BYTES + 1 - len(self._buffer)))
            if not chunk:
                raise ProtocolError("incomplete_frame")
            self._buffer.extend(chunk)

    def read_exact(self, length: int) -> bytes:
        if isinstance(length, bool) or not isinstance(length, int) or length < 0:
            raise ProtocolError("invalid_length")
        while len(self._buffer) < length:
            chunk = self._connection.recv(min(8192, length - len(self._buffer)))
            if not chunk:
                raise ProtocolError("incomplete_payload")
            self._buffer.extend(chunk)
        payload = bytes(self._buffer[:length])
        del self._buffer[:length]
        return payload


def call_unix_socket(
    path: str, request: dict[str, Any], raw_payload: bytes = b""
) -> dict[str, Any]:
    """Make one bounded request; timeout policy is deliberately caller-owned."""

    encoded = encode_document(request)
    if len(raw_payload) > MAX_SECRET_BYTES:
        raise ProtocolError("payload_too_large")
    connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        connection.connect(path)
        connection.sendall(encoded)
        if raw_payload:
            connection.sendall(raw_payload)
        connection.shutdown(socket.SHUT_WR)
        reader = ConnectionReader(connection)
        response = reader.read_frame()
        if set(response).issuperset({"schema_version", "ok"}) is False:
            raise ProtocolError("invalid_response")
        if response.get("schema_version") != SCHEMA_VERSION or not isinstance(
            response.get("ok"), bool
        ):
            raise ProtocolError("invalid_response")
        return response
    finally:
        connection.close()
