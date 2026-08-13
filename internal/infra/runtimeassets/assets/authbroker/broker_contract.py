"""Pure Auth Broker errors, credential bindings, and immutable snapshots."""

from __future__ import annotations

import base64
import hashlib
import json
import re
from dataclasses import dataclass, field
from typing import Any

from .aws_sigv4 import SigV4Error, SigV4Request
from .companion_protocol import CompanionError
from .datadog_oauth import DatadogOAuthError
from .openai_codex_oauth import OpenAICodexOAuthError
from .protocol import ProtocolError
from .vault import (
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    VaultError,
    validate_provider_id,
)

PROJECT_ID_PATTERN = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)
HANDLE_PATTERN = re.compile(r"^tobari-h1_[A-Za-z0-9_-]{43}$")
HOST_PATTERN = re.compile(
    r"^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$"
)
HEADER_PATTERN = re.compile(r"^[!#$%&'*+.^_`|~0-9a-z-]{1,64}$")
SOURCE_FORMATS = ("raw", "bearer", "token")
DESTINATION_FORMATS = ("preserve_scheme", "raw", "bearer", "token")
PROVEN_PRE_EXECUTION_REFRESH_ERRORS = frozenset(
    {
        "companion_unavailable",
        "companion_busy",
        "companion_task_invalid",
        "companion_cancelled",
        "companion_timeout",
    }
)
DEFAULT_RECORD_LOCK_TIMEOUT_SECONDS = 1.0


class BrokerError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _translate_error(error: Exception) -> BrokerError:
    if isinstance(error, BrokerError):
        return error
    if isinstance(error, (
        ProtocolError, VaultError, SigV4Error, CompanionError,
        DatadogOAuthError, OpenAICodexOAuthError,
    )):
        return BrokerError(error.code)
    return BrokerError("internal_error")


def _validate_project_id(project_id: Any) -> str:
    if not isinstance(project_id, str) or not PROJECT_ID_PATTERN.fullmatch(project_id):
        raise BrokerError("invalid_project")
    return project_id


def _validate_revision(revision: Any) -> str:
    if (
        not isinstance(revision, str)
        or not revision.startswith("revision_")
        or len(revision) > 128
        or any(ord(character) < 0x21 or ord(character) > 0x7E for character in revision)
    ):
        raise BrokerError("invalid_revision")
    return revision


def _validate_handle(handle: Any) -> str:
    if not isinstance(handle, str) or not HANDLE_PATTERN.fullmatch(handle):
        raise BrokerError("invalid_handle")
    encoded = handle.removeprefix("tobari-h1_").encode("ascii")
    try:
        raw = base64.b64decode(encoded + b"=", altchars=b"-_", validate=True)
    except ValueError:
        raise BrokerError("invalid_handle") from None
    if len(raw) != 32:
        raise BrokerError("invalid_handle")
    return handle


def _validate_ascii(value: Any, maximum: int, code: str) -> str:
    if (
        not isinstance(value, str)
        or len(value) > maximum
        or any(ord(character) < 0x20 or ord(character) > 0x7E for character in value)
    ):
        raise BrokerError(code)
    return value


def _validate_header(value: Any) -> str:
    if (
        not isinstance(value, str)
        or value != value.lower()
        or not HEADER_PATTERN.fullmatch(value)
        or value in {"host", "content-length", "proxy-authorization", "cookie", "set-cookie"}
        or value.startswith("x-tobari-")
    ):
        raise BrokerError("invalid_binding")
    return value


@dataclass(frozen=True)
class Target:
    scheme: str
    host: str
    port: int

    @classmethod
    def parse(cls, value: Any) -> "Target":
        if not isinstance(value, dict) or set(value) != {"scheme", "host", "port"}:
            raise BrokerError("invalid_binding")
        scheme = value.get("scheme")
        host = value.get("host")
        port = value.get("port")
        if scheme != "https":
            raise BrokerError("invalid_binding")
        if (
            not isinstance(host, str)
            or host != host.lower()
            or not HOST_PATTERN.fullmatch(host)
            or "." not in host
        ):
            raise BrokerError("invalid_binding")
        if isinstance(port, bool) or not isinstance(port, int) or port < 1 or port > 65535:
            raise BrokerError("invalid_binding")
        return cls(scheme=scheme, host=host, port=port)

    def document(self) -> dict[str, Any]:
        return {"scheme": self.scheme, "host": self.host, "port": self.port}


@dataclass(frozen=True)
class Binding:
    """Provider credential projection; HTTP L7 authorization stays in OPA."""

    provider_id: str
    target: Target
    source_header: str
    source_format: str
    destination_header: str
    destination_format: str
    secret_field: str
    secret_headers: tuple[str, ...]

    @classmethod
    def parse(cls, value: Any) -> "Binding":
        if not isinstance(value, dict) or set(value) != {
            "provider_id",
            "target",
            "source",
            "destination",
            "secret_headers",
        }:
            raise BrokerError("invalid_binding")
        provider_id = value.get("provider_id")
        target = value.get("target")
        source = value.get("source")
        destination = value.get("destination")
        secret_headers = value.get("secret_headers")
        if not isinstance(target, dict) or set(target) != {"scheme", "host", "port"}:
            raise BrokerError("invalid_binding")
        if not isinstance(source, dict) or set(source) != {"header", "format"}:
            raise BrokerError("invalid_binding")
        if not isinstance(destination, dict) or set(destination) != {
            "header",
            "format",
            "secret_field",
        }:
            raise BrokerError("invalid_binding")
        try:
            provider_id = validate_provider_id(provider_id)
        except VaultError:
            raise BrokerError("invalid_binding") from None
        normalized_target = Target.parse(target)
        source_header = source.get("header")
        source_format = source.get("format")
        destination_header = destination.get("header")
        destination_format = destination.get("format")
        secret_field = destination.get("secret_field")
        source_header = _validate_header(source_header)
        if source_format not in SOURCE_FORMATS:
            raise BrokerError("invalid_binding")
        destination_header = _validate_header(destination_header)
        if destination_format not in DESTINATION_FORMATS or secret_field not in {
            "primary_secret", DATADOG_OAUTH_CREDENTIAL_KIND,
            OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
        }:
            raise BrokerError("invalid_binding")
        if (
            not isinstance(secret_headers, list)
            or not secret_headers
            or len(secret_headers) > 32
            or any(
                not isinstance(header, str)
                or header != header.lower()
                or not HEADER_PATTERN.fullmatch(header)
                for header in secret_headers
            )
            or len(set(secret_headers)) != len(secret_headers)
        ):
            raise BrokerError("invalid_binding")
        if (
            source_header not in secret_headers
            or destination_header not in secret_headers
        ):
            raise BrokerError("invalid_binding")
        for header in secret_headers:
            _validate_header(header)
        result = cls(
            provider_id,
            normalized_target,
            source_header,
            source_format,
            destination_header,
            destination_format,
            secret_field,
            tuple(sorted(secret_headers)),
        )
        if secret_field == DATADOG_OAUTH_CREDENTIAL_KIND and result.document() != {
            "provider_id": "datadog",
            "target": {"scheme": "https", "host": "api.datadoghq.com", "port": 443},
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {"header": "authorization", "format": "bearer", "secret_field": DATADOG_OAUTH_CREDENTIAL_KIND},
            "secret_headers": ["authorization"],
        }:
            raise BrokerError("invalid_binding")
        if secret_field == OPENAI_CODEX_OAUTH_CREDENTIAL_KIND and result.document() != {
            "provider_id": "openai",
            "target": {"scheme": "https", "host": "chatgpt.com", "port": 443},
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {
                "header": "authorization", "format": "bearer",
                "secret_field": OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
            },
            "secret_headers": [
                "authorization", "chatgpt-account-id", "x-openai-fedramp"
            ],
        }:
            raise BrokerError("invalid_binding")
        return result

    def document(self) -> dict[str, Any]:
        return {
            "provider_id": self.provider_id,
            "target": self.target.document(),
            "source": {
                "header": self.source_header,
                "format": self.source_format,
            },
            "destination": {
                "header": self.destination_header,
                "format": self.destination_format,
                "secret_field": self.secret_field,
            },
            "secret_headers": list(self.secret_headers),
        }

    def matches(self, target: Target, source_header: str, source_format: str) -> bool:
        return (
            self.target == target
            and self.source_header == source_header
            and self.source_format == source_format
        )


@dataclass(frozen=True)
class AwsSigV4Binding:
    """One reviewed built-in AWS signing plan; no executable manifest fields."""

    provider_id: str
    dns_suffixes: tuple[str, ...]
    authorization_header: str
    security_token_header: str
    secret_headers: tuple[str, ...]

    @classmethod
    def parse(cls, value: Any) -> "AwsSigV4Binding":
        if not isinstance(value, dict) or set(value) != {
            "provider_id",
            "kind",
            "aws_sigv4",
        }:
            raise BrokerError("invalid_binding")
        if value.get("provider_id") != "aws" or value.get("kind") != "aws_sigv4":
            raise BrokerError("invalid_binding")
        plan = value.get("aws_sigv4")
        if not isinstance(plan, dict) or set(plan) != {
            "target",
            "source",
            "secret_headers",
        }:
            raise BrokerError("invalid_binding")
        target = plan.get("target")
        source = plan.get("source")
        if target != {
            "scheme": "https",
            "port": 443,
            "dns_suffixes": ["amazonaws.com"],
        }:
            raise BrokerError("invalid_binding")
        if source != {
            "authorization_header": "authorization",
            "security_token_header": "x-amz-security-token",
        }:
            raise BrokerError("invalid_binding")
        if plan.get("secret_headers") != [
            "authorization",
            "x-amz-security-token",
        ]:
            raise BrokerError("invalid_binding")
        return cls(
            provider_id="aws",
            dns_suffixes=("amazonaws.com",),
            authorization_header="authorization",
            security_token_header="x-amz-security-token",
            secret_headers=("authorization", "x-amz-security-token"),
        )

    def document(self) -> dict[str, Any]:
        return {
            "provider_id": self.provider_id,
            "kind": "aws_sigv4",
            "aws_sigv4": {
                "target": {
                    "scheme": "https",
                    "port": 443,
                    "dns_suffixes": list(self.dns_suffixes),
                },
                "source": {
                    "authorization_header": self.authorization_header,
                    "security_token_header": self.security_token_header,
                },
                "secret_headers": list(self.secret_headers),
            },
        }

    def matches_target(self, target: Target) -> bool:
        return (
            target.scheme == "https"
            and target.port == 443
            and any(target.host.endswith("." + suffix) for suffix in self.dns_suffixes)
        )


NormalizedBinding = Binding | AwsSigV4Binding


def _parse_bindings(
    value: Any, provider: str | None = None
) -> tuple[NormalizedBinding, ...]:
    if not isinstance(value, list) or not value or len(value) > 64:
        raise BrokerError("invalid_binding")
    parsed = tuple(
        AwsSigV4Binding.parse(item)
        if isinstance(item, dict) and item.get("kind") == "aws_sigv4"
        else Binding.parse(item)
        for item in value
    )
    if provider is not None and any(binding.provider_id != provider for binding in parsed):
        raise BrokerError("invalid_binding")
    canonical = [binding.document() for binding in parsed]
    keys = [repr(item) for item in canonical]
    if len(set(keys)) != len(keys):
        raise BrokerError("invalid_binding")
    grouped_formats: dict[tuple[Target, str], set[str]] = {}
    for binding in parsed:
        if not isinstance(binding, Binding):
            continue
        grouped_formats.setdefault((binding.target, binding.source_header), set()).add(
            binding.source_format
        )
    if any("raw" in formats and len(formats) > 1 for formats in grouped_formats.values()):
        raise BrokerError("invalid_binding")
    return tuple(sorted(parsed, key=lambda binding: repr(binding.document())))


@dataclass(frozen=True)
class HandleRecord:
    context_id: str
    project_id: str
    provider: str
    record_id: str
    revision: str
    bindings: tuple[NormalizedBinding, ...]


@dataclass(frozen=True)
class AwsRefreshSnapshot:
    context_id: str
    project_id: str
    provider: str
    record_id: str
    revision: str
    state_generation: int
    driver_id: str
    driver_revision: str
    binding_digest: str
    request_digest: str
    state_sha256: str
    state: bytes = field(repr=False)

    @property
    def lock_key(self) -> tuple[str, str, str, str]:
        return (self.context_id, self.provider, self.record_id, self.revision)


@dataclass(frozen=True)
class RenewableSessionSnapshot:
    context_id: str
    project_id: str
    provider: str
    credential_kind: str
    record_id: str
    revision: str
    account_label: str
    state_generation: int
    driver_id: str
    driver_revision: str
    binding_digest: str
    state_sha256: str
    state: bytes = field(repr=False)

    @property
    def lock_key(self) -> tuple[str, str, str, str]:
        return (self.context_id, self.provider, self.record_id, self.revision)


def _document_digest(document: dict[str, Any]) -> str:
    try:
        encoded = json.dumps(
            document,
            ensure_ascii=True,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("ascii")
    except (TypeError, ValueError, UnicodeEncodeError):
        raise BrokerError("aws_signing_request_invalid") from None
    return hashlib.sha256(encoded).hexdigest()


def _signing_request_document(request: SigV4Request) -> dict[str, Any]:
    return {
        "host": request.host,
        "method": request.method,
        "path": request.path,
        "query": request.query,
        "region": request.region,
        "service": request.service,
        "headers": [list(item) for item in request.headers],
        "payload_hash": request.payload_hash,
    }
