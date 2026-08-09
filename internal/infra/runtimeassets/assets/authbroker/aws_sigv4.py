"""Strict AWS Signature Version 4 signing for the reviewed broker plan.

The caller supplies an already-authorized request description and the SHA-256
digest of the complete bounded body.  This module never accepts a body, URL,
credential source, endpoint override, or signing algorithm from a provider
manifest.
"""

from __future__ import annotations

import hashlib
import hmac
import re
from dataclasses import dataclass
from datetime import datetime, timezone
from typing import Any, Callable, Mapping, Sequence
from urllib.parse import quote


ALGORITHM = "AWS4-HMAC-SHA256"
EMPTY_SHA256 = hashlib.sha256(b"").hexdigest()
AWS_DNS_SUFFIXES = ("amazonaws.com",)
HOST_PATTERN = re.compile(
    r"^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)"
    r"(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$"
)
SCOPE_COMPONENT_PATTERN = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$")
COMMERCIAL_REGION_PATTERN = re.compile(
    r"^(?:us-(?:east|west)|eu-(?:central|north|south|west)|"
    r"ap-(?:east|northeast|south|southeast)|ca-(?:central|west)|sa-east|"
    r"me-(?:central|south)|af-south|il-central|mx-central|nz-north)-[0-9]+$"
)
HEADER_NAME_PATTERN = re.compile(r"^[!#$%&'*+.^_`|~0-9a-z-]{1,64}$")
PAYLOAD_HASH_PATTERN = re.compile(r"^[0-9a-f]{64}$")
ACCESS_KEY_PATTERN = re.compile(r"^[A-Z0-9]{16,128}$")
PATH_PATTERN = re.compile(r"^/[A-Za-z0-9._~/-]*$")
QUERY_NAME_VALUE_PATTERN = re.compile(r"^[A-Za-z0-9._~!$'()*+,;:@/?%=&-]*$")
INVALID_PERCENT_ESCAPE_PATTERN = re.compile(r"%(?![0-9A-Fa-f]{2})")
FORBIDDEN_SIGNED_HEADERS = frozenset(
    {
        "authorization",
        "connection",
        "content-length",
        "expect",
        "proxy-authorization",
        "proxy-authenticate",
        "te",
        "trailer",
        "transfer-encoding",
        "upgrade",
        "user-agent",
        "x-amzn-trace-id",
    }
)
MAX_HEADER_VALUE_BYTES = 8 * 1024
MAX_HEADERS = 64
MAX_QUERY_BYTES = 16 * 1024


class SigV4Error(Exception):
    """Stable secret-free signing failure."""

    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


@dataclass(frozen=True, repr=False)
class SigV4Credentials:
    access_key_id: str
    secret_access_key: str
    session_token: str


@dataclass(frozen=True, repr=False)
class SigV4Request:
    host: str
    method: str
    path: str
    query: str
    region: str
    service: str
    headers: tuple[tuple[str, str], ...]
    payload_hash: str


@dataclass(frozen=True, repr=False)
class SigV4Headers:
    authorization: str
    amz_date: str
    security_token: str
    content_sha256: str | None


Clock = Callable[[], datetime]


def _aws_host(value: Any) -> str:
    if (
        not isinstance(value, str)
        or value != value.lower()
        or HOST_PATTERN.fullmatch(value) is None
        or not any(value.endswith("." + suffix) for suffix in AWS_DNS_SUFFIXES)
    ):
        raise SigV4Error("aws_target_unsupported")
    return value


def _scope_component(value: Any) -> str:
    if not isinstance(value, str) or SCOPE_COMPONENT_PATTERN.fullmatch(value) is None:
        raise SigV4Error("aws_scope_invalid")
    return value


def _normalize_header_value(value: str) -> str:
    if (
        not isinstance(value, str)
        or not value
        or len(value) > MAX_HEADER_VALUE_BYTES
        # Gateway receives raw HTTP header bytes and decodes them as Latin-1.
        # Restrict the signed subset to visible ASCII plus horizontal tabs so
        # Broker UTF-8 serialization can never change the bytes sent upstream.
        or any(
            character != "\t" and not 0x20 <= ord(character) <= 0x7E
            for character in value
        )
    ):
        raise SigV4Error("aws_header_invalid")
    return " ".join(value.strip().split())


def _validate_target_scope(host: str, region: str, service: str) -> None:
    # AWS has a small set of global endpoints whose hostname omits the signing
    # region. Keep those exact instead of inferring from suffix membership.
    global_targets = {
        "cloudfront.amazonaws.com": ("us-east-1", "cloudfront"),
        "iam.amazonaws.com": ("us-east-1", "iam"),
        "route53.amazonaws.com": ("us-east-1", "route53"),
        "s3.amazonaws.com": ("us-east-1", "s3"),
        "sts.amazonaws.com": ("us-east-1", "sts"),
    }
    expected = global_targets.get(host)
    if expected is not None:
        if (region, service) != expected:
            raise SigV4Error("aws_scope_target_mismatch")
        return

    suffix = next(
        (
            candidate
            for candidate in AWS_DNS_SUFFIXES
            if host.endswith("." + candidate)
        ),
        None,
    )
    if suffix is None:
        raise SigV4Error("aws_target_unsupported")
    labels = host[: -(len(suffix) + 1)].split(".")
    # The first reviewed plan accepts only endpoints that state the signing
    # service and region as complete DNS labels. Provider aliases, FIPS names,
    # and endpoint variants require an explicit future contract.
    if service not in labels or region not in labels:
        raise SigV4Error("aws_scope_target_mismatch")


def _headers(value: Any, host: str) -> tuple[tuple[str, str], ...]:
    if not isinstance(value, Sequence) or isinstance(value, (str, bytes)):
        raise SigV4Error("aws_headers_invalid")
    if len(value) > MAX_HEADERS:
        raise SigV4Error("aws_headers_invalid")
    combined: dict[str, list[str]] = {}
    for item in value:
        if (
            not isinstance(item, Sequence)
            or isinstance(item, (str, bytes))
            or len(item) != 2
        ):
            raise SigV4Error("aws_headers_invalid")
        name, raw = item
        if (
            not isinstance(name, str)
            or name != name.lower()
            or HEADER_NAME_PATTERN.fullmatch(name) is None
            or name in FORBIDDEN_SIGNED_HEADERS
            or name.startswith("x-tobari-")
        ):
            raise SigV4Error("aws_header_invalid")
        combined.setdefault(name, []).append(_normalize_header_value(raw))
    if "host" in combined:
        raise SigV4Error("aws_header_invalid")
    combined["host"] = [host]
    # Every x-amz header forwarded upstream must be signed. The caller removes
    # placeholder auth fields before constructing this list.
    normalized = tuple(
        (name, ",".join(items)) for name, items in sorted(combined.items())
    )
    return normalized


def parse_request(document: Mapping[str, Any]) -> SigV4Request:
    if not isinstance(document, Mapping) or set(document) != {
        "host",
        "method",
        "path",
        "query",
        "region",
        "service",
        "headers",
        "payload_hash",
    }:
        raise SigV4Error("aws_signing_request_invalid")
    host = _aws_host(document.get("host"))
    method = document.get("method")
    if (
        not isinstance(method, str)
        or method != method.upper()
        or re.fullmatch(r"[A-Z]{3,16}", method) is None
    ):
        raise SigV4Error("aws_method_invalid")
    path = document.get("path")
    # Reject normalization-sensitive paths in the first plan. AWS CLI service
    # APIs use canonical simple paths; S3 object transfer is deliberately out.
    if (
        not isinstance(path, str)
        or len(path.encode("utf-8")) > 8 * 1024
        or PATH_PATTERN.fullmatch(path) is None
        or "//" in path
        or any(segment in {".", ".."} for segment in path.split("/"))
    ):
        raise SigV4Error("aws_path_unsupported")
    query = document.get("query")
    if (
        not isinstance(query, str)
        or len(query.encode("ascii", "ignore")) != len(query)
        or len(query) > MAX_QUERY_BYTES
        or QUERY_NAME_VALUE_PATTERN.fullmatch(query) is None
        or INVALID_PERCENT_ESCAPE_PATTERN.search(query) is not None
        or re.search(r"(?:^|&)X-Amz-(?:Algorithm|Credential|Signature)=", query, re.I)
    ):
        raise SigV4Error("aws_query_unsupported")
    region = _scope_component(document.get("region"))
    service = _scope_component(document.get("service"))
    if COMMERCIAL_REGION_PATTERN.fullmatch(region) is None:
        raise SigV4Error("aws_scope_invalid")
    _validate_target_scope(host, region, service)
    payload_hash = document.get("payload_hash")
    if not isinstance(payload_hash, str) or PAYLOAD_HASH_PATTERN.fullmatch(payload_hash) is None:
        raise SigV4Error("aws_payload_hash_invalid")
    return SigV4Request(
        host=host,
        method=method,
        path=path,
        query=query,
        region=region,
        service=service,
        headers=_headers(document.get("headers"), host),
        payload_hash=payload_hash,
    )


def parse_credentials(value: Any) -> SigV4Credentials:
    if not isinstance(value, Mapping) or set(value) != {
        "access_key_id",
        "secret_access_key",
        "session_token",
    }:
        raise SigV4Error("aws_credentials_invalid")
    access_key = value.get("access_key_id")
    secret = value.get("secret_access_key")
    token = value.get("session_token")
    if (
        not isinstance(access_key, str)
        or ACCESS_KEY_PATTERN.fullmatch(access_key) is None
        or not isinstance(secret, str)
        or not 16 <= len(secret) <= 256
        or any(ord(character) < 0x21 or ord(character) > 0x7E for character in secret)
        or not isinstance(token, str)
        or not 16 <= len(token) <= 16 * 1024
        or any(ord(character) < 0x21 or ord(character) > 0x7E for character in token)
    ):
        raise SigV4Error("aws_credentials_invalid")
    return SigV4Credentials(access_key, secret, token)


def _canonical_query(raw: str) -> str:
    if raw == "":
        return ""
    encoded: list[tuple[str, str]] = []
    for field in raw.split("&"):
        name, separator, value = field.partition("=")
        if not separator:
            value = ""
        # Decode only valid percent triplets, then re-encode using AWS's RFC3986
        # unreserved set. Invalid or ambiguous encodings fail closed.
        try:
            from urllib.parse import unquote_to_bytes

            name_bytes = unquote_to_bytes(name)
            value_bytes = unquote_to_bytes(value)
            encoded.append(
                (quote(name_bytes, safe="-_.~"), quote(value_bytes, safe="-_.~"))
            )
        except (UnicodeError, ValueError):
            raise SigV4Error("aws_query_unsupported") from None
    encoded.sort()
    return "&".join(f"{name}={value}" for name, value in encoded)


def _hmac(key: bytes, value: str) -> bytes:
    return hmac.new(key, value.encode("utf-8"), hashlib.sha256).digest()


def sign(
    request: SigV4Request,
    credentials: SigV4Credentials,
    *,
    clock: Clock | None = None,
) -> SigV4Headers:
    if not isinstance(request, SigV4Request) or not isinstance(credentials, SigV4Credentials):
        raise SigV4Error("aws_signing_request_invalid")
    instant = (clock or (lambda: datetime.now(timezone.utc)))()
    if not isinstance(instant, datetime) or instant.tzinfo is None:
        raise SigV4Error("aws_clock_invalid")
    instant = instant.astimezone(timezone.utc)
    amz_date = instant.strftime("%Y%m%dT%H%M%SZ")
    date = amz_date[:8]

    header_values: dict[str, str] = dict(request.headers)
    header_values["x-amz-date"] = amz_date
    header_values["x-amz-security-token"] = credentials.session_token
    content_sha256: str | None = None
    if request.service == "s3":
        content_sha256 = request.payload_hash
        header_values["x-amz-content-sha256"] = content_sha256
    canonical_names = sorted(header_values)
    canonical_headers = "".join(
        f"{name}:{_normalize_header_value(header_values[name])}\n"
        for name in canonical_names
    )
    signed_headers = ";".join(canonical_names)
    canonical_request = "\n".join(
        (
            request.method,
            quote(request.path, safe="/-_.~"),
            _canonical_query(request.query),
            canonical_headers,
            signed_headers,
            request.payload_hash,
        )
    )
    scope = f"{date}/{request.region}/{request.service}/aws4_request"
    string_to_sign = "\n".join(
        (
            ALGORITHM,
            amz_date,
            scope,
            hashlib.sha256(canonical_request.encode("utf-8")).hexdigest(),
        )
    )
    date_key = _hmac(("AWS4" + credentials.secret_access_key).encode("utf-8"), date)
    region_key = _hmac(date_key, request.region)
    service_key = _hmac(region_key, request.service)
    signing_key = _hmac(service_key, "aws4_request")
    signature = hmac.new(
        signing_key, string_to_sign.encode("utf-8"), hashlib.sha256
    ).hexdigest()
    authorization = (
        f"{ALGORITHM} Credential={credentials.access_key_id}/{scope}, "
        f"SignedHeaders={signed_headers}, Signature={signature}"
    )
    return SigV4Headers(
        authorization=authorization,
        amz_date=amz_date,
        security_token=credentials.session_token,
        content_sha256=content_sha256,
    )
