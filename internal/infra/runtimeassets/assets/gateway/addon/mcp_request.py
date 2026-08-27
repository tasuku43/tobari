"""Bounded semantic classifier for one MCP JSON-RPC request."""

from __future__ import annotations

import json
import re
from dataclasses import dataclass


MCP_METHOD = re.compile(r"^[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*$")
MCP_TOOL_NAME = re.compile(r"^[A-Za-z0-9_.:/-]+$")
MAX_MCP_BODY_BYTES = 1024 * 1024


class MCPRequestError(Exception):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass(frozen=True)
class ParsedMCPRequest:
    method: str
    tool_name: str | None = None


def _strict_object(pairs: list[tuple[str, object]]) -> dict[str, object]:
    result: dict[str, object] = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate JSON object key")
        result[key] = value
    return result


def _reject_nonfinite_json(value: str) -> object:
    raise ValueError(f"non-finite JSON number {value}")


def validate_mcp_post_headers(method: str, headers: list[tuple[str, str]]) -> None:
    if method != "POST":
        raise MCPRequestError("invalid_method", "MCP transport method must be POST")
    values: dict[str, list[str]] = {}
    for name, value in headers:
        values.setdefault(name.lower(), []).append(value)
    content_types = values.get("content-type", [])
    if len(content_types) != 1 or content_types[0].split(";", 1)[0].strip().lower() != "application/json":
        raise MCPRequestError("invalid_content_type", "MCP request must use application/json")
    if "transfer-encoding" in values or "content-encoding" in values:
        raise MCPRequestError("unsupported_encoding", "MCP request encoding is unsupported")
    lengths = values.get("content-length", [])
    if len(lengths) != 1 or not lengths[0].isdigit():
        raise MCPRequestError("invalid_content_length", "MCP request requires one bounded Content-Length")
    length = int(lengths[0])
    if length < 1 or length > MAX_MCP_BODY_BYTES:
        raise MCPRequestError("invalid_content_length", "MCP request body size is invalid")


def parse_mcp_post_request(method: str, headers: list[tuple[str, str]], body: bytes) -> ParsedMCPRequest:
    validate_mcp_post_headers(method, headers)
    if len(body) < 1 or len(body) > MAX_MCP_BODY_BYTES:
        raise MCPRequestError("invalid_body_size", "MCP request body size is invalid")
    declared_length = next(value for name, value in headers if name.lower() == "content-length")
    if int(declared_length) != len(body):
        raise MCPRequestError("content_length_mismatch", "MCP request body length does not match Content-Length")
    try:
        document = json.loads(
            body,
            object_pairs_hook=_strict_object,
            parse_constant=_reject_nonfinite_json,
        )
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError) as error:
        raise MCPRequestError("invalid_json", "MCP request body is invalid JSON") from error
    if not isinstance(document, dict) or not set(document).issubset({"jsonrpc", "id", "method", "params"}):
        raise MCPRequestError("invalid_shape", "MCP request must be one JSON-RPC object")
    if document.get("jsonrpc") != "2.0":
        raise MCPRequestError("invalid_version", "MCP request JSON-RPC version is invalid")
    rpc_method = document.get("method")
    if not isinstance(rpc_method, str) or len(rpc_method) > 128 or MCP_METHOD.fullmatch(rpc_method) is None:
        raise MCPRequestError("invalid_rpc_method", "MCP JSON-RPC method is invalid")
    tool_name = None
    if rpc_method == "tools/call":
        params = document.get("params")
        if not isinstance(params, dict) or not set(params).issubset({"name", "arguments", "_meta"}):
            raise MCPRequestError("invalid_tool_call", "MCP tools/call params are invalid")
        candidate = params.get("name")
        if not isinstance(candidate, str) or len(candidate) > 256 or MCP_TOOL_NAME.fullmatch(candidate) is None:
            raise MCPRequestError("invalid_tool_name", "MCP tool name is invalid")
        tool_name = candidate
    return ParsedMCPRequest(method=rpc_method, tool_name=tool_name)
