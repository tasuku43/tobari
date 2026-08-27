"""Bounded extraction of signed AWS RPC wire-operation identity.

This module deliberately does not load AWS service models or classify an
operation as read or mutation. It retains only the SigV4 signing service and
the operation token that the selected AWS wire protocol sends upstream.
"""

from __future__ import annotations

from dataclasses import dataclass
import re
from urllib.parse import parse_qsl


MAX_AWS_OPERATION_BODY_BYTES = 8 * 1024 * 1024
MAX_AWS_QUERY_FIELDS = 2048

_AUTHORIZATION = re.compile(
    r"AWS4-HMAC-SHA256 Credential=([^/,\s]{1,256})/([0-9]{8})/"
    r"([a-z0-9-]{1,63})/([a-z0-9-]{1,63})/aws4_request, "
    r"SignedHeaders=([a-z0-9-]+(?:;[a-z0-9-]+)*), Signature=([0-9a-f]{64})"
)
_QUERY_OPERATION = re.compile(r"[A-Za-z_][A-Za-z0-9_]{0,127}")
_JSON_TARGET = re.compile(
    r"[A-Za-z0-9_-]+(?:\.[A-Za-z0-9_-]+)*\.[A-Za-z_][A-Za-z0-9_]{0,127}"
)
_VERSION = re.compile(r"[0-9]{4}-[0-9]{2}-[0-9]{2}")
_DATE = re.compile(r"[0-9]{8}T[0-9]{6}Z")
_CONTENT_LENGTH = re.compile(r"0|[1-9][0-9]*")


class AWSRequestError(Exception):
    def __init__(self, code: str, message: str):
        super().__init__(message)
        self.code = code


@dataclass(frozen=True)
class ParsedAWSRequest:
    wire_protocol: str
    service: str
    operation: str
    protocol_version: str | None = None
    target_namespace: str | None = None


@dataclass(frozen=True)
class PendingAWSQueryRequest:
    service: str
    expected_body_bytes: int
    authority: str
    content_type: str


def _values(headers: list[tuple[str, str]], name: str) -> list[str]:
    return [value for key, value in headers if key.lower() == name]


def _media_type(value: str) -> str:
    return value.split(";", 1)[0].strip().lower()


def _aws_authority(host: str, scheme: str, port: int) -> bool:
    return (
        scheme == "https"
        and port == 443
        and host == host.lower()
        and not host.endswith(".")
        and host.endswith(".amazonaws.com")
    )


def _bounded_content_length(headers: list[tuple[str, str]]) -> int:
    values = _values(headers, "content-length")
    if len(values) != 1 or _CONTENT_LENGTH.fullmatch(values[0]) is None:
        raise AWSRequestError(
            "aws_request_invalid", "a classified AWS RPC request requires one Content-Length"
        )
    length = int(values[0])
    if length > MAX_AWS_OPERATION_BODY_BYTES:
        raise AWSRequestError("aws_request_too_large", "AWS RPC request body is too large")
    return length


def classify_aws_request_headers(
    method: str,
    scheme: str,
    host: str,
    port: int,
    path: str,
    query: str,
    headers: list[tuple[str, str]],
) -> ParsedAWSRequest | PendingAWSQueryRequest | None:
    """Return a minimal operation identity only for a signed AWS RPC request."""

    if not _aws_authority(host, scheme, port):
        return None
    authorization_values = _values(headers, "authorization")
    looks_signed = any(value.startswith("AWS4-HMAC-SHA256 ") for value in authorization_values)
    content_types = _values(headers, "content-type")
    targets = _values(headers, "x-amz-target")
    claims_rpc = any(
        _media_type(value)
        in {
            "application/x-www-form-urlencoded",
            "application/x-amz-json-1.0",
            "application/x-amz-json-1.1",
        }
        for value in content_types
    ) or bool(targets)
    if not claims_rpc:
        return None
    if not looks_signed:
        raise AWSRequestError("aws_signature_invalid", "AWS RPC signature is invalid")
    if len(authorization_values) != 1:
        raise AWSRequestError("aws_signature_invalid", "AWS SigV4 authorization is ambiguous")
    matched = _AUTHORIZATION.fullmatch(authorization_values[0])
    if matched is None:
        raise AWSRequestError("aws_signature_invalid", "AWS SigV4 authorization is invalid")
    _, scope_date, _region, service, signed_value, _signature = matched.groups()
    signed_headers = tuple(signed_value.split(";"))
    if (
        signed_headers != tuple(sorted(set(signed_headers)))
        or "content-type" not in signed_headers
        or "host" not in signed_headers
        or "x-amz-date" not in signed_headers
    ):
        raise AWSRequestError("aws_signature_invalid", "AWS SigV4 signed headers are invalid")
    dates = _values(headers, "x-amz-date")
    if len(dates) != 1 or _DATE.fullmatch(dates[0]) is None or not dates[0].startswith(scope_date):
        raise AWSRequestError("aws_signature_invalid", "AWS SigV4 date is invalid")
    if _values(headers, "transfer-encoding"):
        raise AWSRequestError("aws_request_unsupported", "streaming AWS RPC requests are unsupported")
    hashes = _values(headers, "x-amz-content-sha256")
    if len(hashes) > 1 or any(re.fullmatch(r"[0-9a-f]{64}", value) is None for value in hashes):
        raise AWSRequestError("aws_request_unsupported", "unsigned or streaming AWS payloads are unsupported")
    if method != "POST" or path != "/" or query != "" or len(content_types) != 1:
        if claims_rpc:
            raise AWSRequestError("aws_request_invalid", "AWS RPC transport coordinates are invalid")
        return None
    media_type = _media_type(content_types[0])
    expected = _bounded_content_length(headers)
    if media_type == "application/x-www-form-urlencoded":
        if targets:
            raise AWSRequestError("aws_request_invalid", "AWS Query request cannot contain X-Amz-Target")
        return PendingAWSQueryRequest(
            service=service,
            expected_body_bytes=expected,
            authority=host,
            content_type=content_types[0],
        )
    if media_type in {"application/x-amz-json-1.0", "application/x-amz-json-1.1"}:
        if len(targets) != 1 or "x-amz-target" not in signed_headers or _JSON_TARGET.fullmatch(targets[0]) is None or len(targets[0]) > 256:
            raise AWSRequestError("aws_operation_invalid", "AWS JSON operation target is invalid or unsigned")
        namespace, operation = targets[0].rsplit(".", 1)
        return ParsedAWSRequest(
            wire_protocol="json",
            service=service,
            operation=operation,
            target_namespace=namespace,
        )
    if targets:
        raise AWSRequestError("aws_request_invalid", "X-Amz-Target is invalid for this content type")
    return None


def parse_aws_query_request(
    pending: PendingAWSQueryRequest,
    method: str,
    scheme: str,
    host: str,
    port: int,
    path: str,
    query: str,
    headers: list[tuple[str, str]],
    body: bytes,
) -> ParsedAWSRequest:
    content_types = _values(headers, "content-type")
    if (
        method != "POST"
        or scheme != "https"
        or host != pending.authority
        or port != 443
        or path != "/"
        or query != ""
        or content_types != [pending.content_type]
        or _media_type(pending.content_type) != "application/x-www-form-urlencoded"
        or _values(headers, "transfer-encoding")
        or _bounded_content_length(headers) != pending.expected_body_bytes
    ):
        raise AWSRequestError("aws_request_changed", "AWS Query request changed before authorization")
    if len(body) != pending.expected_body_bytes:
        raise AWSRequestError("aws_request_changed", "AWS Query request body length changed")
    try:
        fields = parse_qsl(
            body.decode("ascii"),
            keep_blank_values=True,
            strict_parsing=True,
            encoding="utf-8",
            errors="strict",
            max_num_fields=MAX_AWS_QUERY_FIELDS,
            separator="&",
        )
    except (UnicodeError, ValueError) as error:
        raise AWSRequestError("aws_operation_invalid", "AWS Query body is invalid") from error
    actions = [value for name, value in fields if name == "Action"]
    versions = [value for name, value in fields if name == "Version"]
    if len(actions) != 1 or _QUERY_OPERATION.fullmatch(actions[0]) is None:
        raise AWSRequestError("aws_operation_invalid", "AWS Query Action is invalid or ambiguous")
    if len(versions) != 1 or _VERSION.fullmatch(versions[0]) is None:
        raise AWSRequestError("aws_operation_invalid", "AWS Query Version is invalid or ambiguous")
    return ParsedAWSRequest(
        wire_protocol="query",
        service=pending.service,
        operation=actions[0],
        protocol_version=versions[0],
    )
