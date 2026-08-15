"""Strict per-request Auth Broker integration for the Tobari Gateway.

Provider projection is host-owned, contains no credentials, and is validated
again here before it can influence request recognition.  A broker handle is
removed from the mitmproxy flow before either the broker or OPA is called; the
only copy retained by this module lives in one request-scoped object.
"""

from __future__ import annotations

import base64
import hashlib
import json
import os
import re
import socket
from dataclasses import dataclass
from typing import Any, Callable
from urllib.parse import unquote, urlsplit

from mitmproxy import http

from credential_adapters import (
    CONTROL_HEADERS, DEFAULT_SECRET_HEADERS, PROFILE_HEADER, CredentialAdapter,
    CredentialAdapterError, PreparedCredentialRequest,
)
from reviewed_credential_profiles import (
    PRIMARY_SECRET_FIELD,
    RENEWABLE_PROVIDER_IDS,
    REVIEWED_DYNAMIC_CREDENTIAL_KINDS,
    REVIEWED_HEADER_SECRET_FIELDS,
    response_profile,
    reviewed_projection_profile,
)
from validated_file import StatIdentityCache, ValidatedFileError


PROVIDER_SCHEMA_VERSION = 1
BROKER_SCHEMA_VERSION = 1
MAX_PROVIDER_PROJECTION_BYTES = 16 * 1024 * 1024
MAX_BROKER_FRAME_BYTES = 64 * 1024
MAX_SECRET_BYTES = 32 * 1024
MAX_AWS_BODY_BYTES = 8 * 1024 * 1024
HANDLE_PATTERN = re.compile(r"^tobari-h1_[A-Za-z0-9_-]{43}$")
HANDLE_MARKER = "tobari-h"
PROVIDER_ID_PATTERN = re.compile(r"^[a-z0-9]+(?:[._-][a-z0-9]+)*$")
REVISION_PATTERN = re.compile(r"^revision_[!-~]{1,119}$")
HOST_PATTERN = re.compile(
    r"^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)"
    r"(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$"
)
HEADER_PATTERN = re.compile(r"^[!#$%&'*+.^_`|~0-9a-z-]{1,64}$")
SOURCE_FORMATS = frozenset({"raw", "bearer", "token"})
DESTINATION_FORMATS = frozenset({"preserve_scheme", "raw", "bearer", "token"})
AWS_DNS_SUFFIXES = ("amazonaws.com",)
AWS_SCOPE_COMPONENT_PATTERN = re.compile(r"^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$")
AWS_COMMERCIAL_REGION_PATTERN = re.compile(
    r"^(?:us-(?:east|west)|eu-(?:central|north|south|west)|"
    r"ap-(?:east|northeast|south|southeast)|ca-(?:central|west)|sa-east|"
    r"me-(?:central|south)|af-south|il-central|mx-central|nz-north)-[0-9]+$"
)
AWS_AUTHORIZATION_PATTERN = re.compile(
    r"^AWS4-HMAC-SHA256 Credential=(tobari-h1_[A-Za-z0-9_-]{43})/"
    r"([0-9]{8})/([a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?)/"
    r"([a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?)/aws4_request, "
    r"SignedHeaders=([a-z0-9-]+(?:;[a-z0-9-]+)*), Signature=([0-9a-f]{64})$"
)
AWS_SIGNED_AUTHORIZATION_PATTERN = re.compile(
    r"^AWS4-HMAC-SHA256 Credential=([A-Z0-9]{16,128})/"
    r"([0-9]{8})/([a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?)/"
    r"([a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?)/aws4_request, "
    r"SignedHeaders=([a-z0-9-]+(?:;[a-z0-9-]+)*), Signature=([0-9a-f]{64})$"
)
FORBIDDEN_HEADERS = frozenset(
    {"host", "content-length", "proxy-authorization", "cookie", "set-cookie"}
)


class BrokerCredentialError(CredentialAdapterError):
    """A secret-free broker failure safe to report only as a generic fault."""


class BrokerCredentialBindingError(BrokerCredentialError):
    """A Tobari-looking handle is invalid for this exact trusted principal."""


class BrokerAuthenticationRequired(BrokerCredentialError):
    """A declared provider binding received a Workspace-owned credential."""


class BrokerCredentialUnavailable(BrokerCredentialError):
    """The private broker boundary is unavailable or returned an invalid frame."""


class BrokerCredentialOutcomeUnknown(BrokerCredentialError):
    """A refresh may have executed and must not be replayed automatically."""


class _BrokerCredentialResponseInvalid(BrokerCredentialUnavailable):
    """The broker replied, but its frame cannot prove an operation outcome."""


def _strict_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    for key, item in pairs:
        if key in value:
            raise ValueError("duplicate key")
        value[key] = item
    return value


def _reject_constant(_: str) -> None:
    raise ValueError("invalid constant")


def _decode_json(payload: bytes) -> Any:
    try:
        return json.loads(
            payload.decode("utf-8"),
            object_pairs_hook=_strict_object,
            parse_constant=_reject_constant,
        )
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError) as error:
        raise BrokerCredentialUnavailable("credential broker returned invalid data") from error


def _exact_keys(value: Any, keys: set[str], label: str) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != keys:
        raise BrokerCredentialUnavailable(f"{label} is invalid")
    return value


def _valid_provider_id(value: Any) -> bool:
    return (
        isinstance(value, str)
        and len(value) <= 64
        and PROVIDER_ID_PATTERN.fullmatch(value) is not None
    )


def _validate_header(value: Any) -> str:
    if (
        not isinstance(value, str)
        or HEADER_PATTERN.fullmatch(value) is None
        or value in FORBIDDEN_HEADERS
        or value.startswith("x-tobari-")
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return value


def _validate_target(value: Any) -> dict[str, Any]:
    target = _exact_keys(value, {"scheme", "host", "port"}, "provider target")
    host = target.get("host")
    port = target.get("port")
    if (
        target.get("scheme") != "https"
        or not isinstance(host, str)
        or HOST_PATTERN.fullmatch(host) is None
        or "." not in host
        or isinstance(port, bool)
        or not isinstance(port, int)
        or port < 1
        or port > 65535
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return {"scheme": "https", "host": host, "port": port}


def _validate_destination(value: Any) -> dict[str, Any]:
    destination = _exact_keys(
        value, {"header", "format", "secret_field"}, "provider destination"
    )
    header = _validate_header(destination.get("header"))
    output_format = destination.get("format")
    if (
        output_format not in DESTINATION_FORMATS
        or destination.get("secret_field")
        not in {PRIMARY_SECRET_FIELD} | set(REVIEWED_HEADER_SECRET_FIELDS)
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return {
        "header": header,
        "format": output_format,
        "secret_field": destination["secret_field"],
    }


def _validate_secret_headers(value: Any, source: str, destination: str) -> list[str]:
    if (
        not isinstance(value, list)
        or not value
        or len(value) > 32
        or len(value) != len(set(value))
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    headers = [_validate_header(item) for item in value]
    if headers != sorted(headers) or source not in headers or destination not in headers:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return headers


def _binding_sort_key(binding: dict[str, Any]) -> tuple[Any, ...]:
    return (
        binding["provider_id"],
        binding["target"]["host"],
        binding["target"]["port"],
        binding["source"]["header"],
        binding["source"]["format"],
        binding["destination"]["header"],
        binding["destination"]["format"],
    )


def _raw_binding_sort_key(binding: dict[str, Any]) -> tuple[Any, ...]:
    return (
        binding["target"]["scheme"],
        binding["target"]["host"],
        binding["target"]["port"],
        binding["source"]["header"],
        ",".join(binding["source"]["formats"]),
        binding["destination"]["header"],
        binding["destination"]["format"],
    )


def _validate_aws_signing_binding(
    value: Any, provider_id: str
) -> dict[str, Any]:
    binding = _exact_keys(value, {"kind", "aws_sigv4"}, "provider signing binding")
    if binding.get("kind") != "aws_sigv4":
        raise BrokerCredentialUnavailable("provider projection is invalid")
    plan = _exact_keys(
        binding.get("aws_sigv4"),
        {"target", "source", "secret_headers"},
        "provider AWS signing plan",
    )
    target = _exact_keys(
        plan.get("target"), {"scheme", "port", "dns_suffixes"}, "provider AWS target"
    )
    if (
        target.get("scheme") != "https"
        or target.get("port") != 443
        or target.get("dns_suffixes") != list(AWS_DNS_SUFFIXES)
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    source = _exact_keys(
        plan.get("source"),
        {"authorization_header", "security_token_header"},
        "provider AWS source",
    )
    if source != {
        "authorization_header": "authorization",
        "security_token_header": "x-amz-security-token",
    }:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    secret_headers = plan.get("secret_headers")
    if secret_headers != ["authorization", "x-amz-security-token"]:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return {
        "provider_id": provider_id,
        "kind": "aws_sigv4",
        "aws_sigv4": {
            "target": {
                "scheme": "https",
                "port": 443,
                "dns_suffixes": list(AWS_DNS_SUFFIXES),
            },
            "source": dict(source),
            "secret_headers": list(secret_headers),
        },
    }


def _validate_provider(
    provider: Any,
) -> tuple[
    list[dict[str, Any]],
    list[dict[str, Any]],
    list[dict[str, str]],
    list[dict[str, str]],
]:
    if not isinstance(provider, dict):
        raise BrokerCredentialUnavailable("provider is invalid")
    schema_version = provider.get("schema_version")
    provider_keys = {
        "schema_version",
        "id",
        "display_name",
        "acquisition",
        "credential",
        "workspace_projections",
        "header_bindings",
    }
    if "signing_bindings" in provider:
        provider_keys.add("signing_bindings")
    provider = _exact_keys(
        provider,
        provider_keys,
        "provider",
    )
    provider_id = provider.get("id")
    display_name = provider.get("display_name")
    if (
        schema_version != PROVIDER_SCHEMA_VERSION
        or isinstance(provider.get("schema_version"), bool)
        or not _valid_provider_id(provider_id)
        or not isinstance(display_name, str)
        or not display_name
        or display_name.strip() != display_name
        or len(display_name.encode("utf-8")) > 96
        or any(ord(character) < 0x20 or character in "\u2028\u2029" for character in display_name)
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")

    acquisition = provider.get("acquisition")
    if not isinstance(acquisition, dict):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    mode = acquisition.get("mode")
    expected_acquisition = {"mode", "helper"} if mode == "builtin_helper" else {"mode"}
    if set(acquisition) != expected_acquisition or mode not in {
        "builtin_helper",
        "stdin_import",
    }:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    if mode == "builtin_helper" and not _valid_provider_id(acquisition.get("helper")):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    credential = _exact_keys(provider.get("credential"), {"kind"}, "provider credential")
    credential_kind = credential.get("kind")
    valid_credential = credential_kind in (
        {PRIMARY_SECRET_FIELD} | set(REVIEWED_DYNAMIC_CREDENTIAL_KINDS)
    )
    if not valid_credential:
        raise BrokerCredentialUnavailable("provider projection is invalid")

    workspace = provider.get("workspace_projections")
    if not isinstance(workspace, list) or not 1 <= len(workspace) <= 32:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    environment: list[dict[str, str]] = []
    complete_files: list[dict[str, str]] = []
    json_merges: list[dict[str, str]] = []
    workspace_keys: list[tuple[str, str]] = []
    handle_count = 0
    for item in workspace:
        if not isinstance(item, dict):
            raise BrokerCredentialUnavailable("provider projection is invalid")
        kind = item.get("kind")
        expected = {"kind", "name", "template"} if kind == "env" else {
            "kind",
            "path",
            "template",
        }
        if set(item) != expected or kind not in {"env", "complete_file", "merge_json"}:
            raise BrokerCredentialUnavailable("provider projection is invalid")
        template = item.get("template")
        if not isinstance(template, str) or not template or "\x00" in template:
            raise BrokerCredentialUnavailable("provider projection is invalid")
        handle_count += template.count("${HANDLE}")
        if kind == "env":
            name = item.get("name")
            if not isinstance(name, str) or re.fullmatch(r"[A-Z_][A-Z0-9_]{0,63}", name) is None:
                raise BrokerCredentialUnavailable("provider projection is invalid")
            environment.append({"provider_id": provider_id, "name": name, "template": template})
            workspace_keys.append((kind, name))
        else:
            path = item.get("path")
            if (
                not isinstance(path, str)
                or not path
                or path.startswith("/")
                or "\\" in path
                or any(part in {"", ".", ".."} for part in path.split("/"))
                or len(path.encode("utf-8")) > 240
            ):
                raise BrokerCredentialUnavailable("provider projection is invalid")
            target = {"provider_id": provider_id, "path": path, "template": template}
            if kind == "complete_file":
                complete_files.append(target)
            else:
                json_merges.append(target)
            workspace_keys.append((kind, path))
    if handle_count < 1 or handle_count > len(workspace) or workspace_keys != sorted(workspace_keys):
        raise BrokerCredentialUnavailable("provider projection is invalid")

    raw_bindings = provider.get("header_bindings")
    if not isinstance(raw_bindings, list) or len(raw_bindings) > 64:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    normalized: list[dict[str, Any]] = []
    checked_raw: list[dict[str, Any]] = []
    for value in raw_bindings:
        binding = _exact_keys(
            value,
            {"target", "source", "destination", "secret_headers"},
            "provider binding",
        )
        target = _validate_target(binding.get("target"))
        source = _exact_keys(binding.get("source"), {"header", "formats"}, "provider source")
        source_header = _validate_header(source.get("header"))
        formats = source.get("formats")
        if (
            not isinstance(formats, list)
            or not 1 <= len(formats) <= 3
            or len(formats) != len(set(formats))
            or formats != sorted(formats)
            or any(item not in SOURCE_FORMATS for item in formats)
            or ("raw" in formats and len(formats) > 1)
        ):
            raise BrokerCredentialUnavailable("provider projection is invalid")
        destination = _validate_destination(binding.get("destination"))
        secret_headers = _validate_secret_headers(
            binding.get("secret_headers"), source_header, destination["header"]
        )
        checked = {
            "target": target,
            "source": {"header": source_header, "formats": formats},
            "destination": destination,
            "secret_headers": secret_headers,
        }
        checked_raw.append(checked)
        for source_format in formats:
            normalized.append(
                {
                    "provider_id": provider_id,
                    "target": target,
                    "source": {"header": source_header, "format": source_format},
                    "destination": destination,
                    "secret_headers": secret_headers,
                }
            )
    if checked_raw != raw_bindings or checked_raw != sorted(checked_raw, key=_raw_binding_sort_key):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    if credential_kind == PRIMARY_SECRET_FIELD and any(
        binding["destination"]["secret_field"] != PRIMARY_SECRET_FIELD
        for binding in normalized
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    raw_signing = provider.get("signing_bindings", [])
    if not isinstance(raw_signing, list) or len(raw_signing) > 8:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    signing_bindings = [
        _validate_aws_signing_binding(item, provider_id) for item in raw_signing
    ]
    profile = reviewed_projection_profile(
        provider_id, credential_kind, acquisition.get("helper")
    )
    if json_merges and (
        provider_id != "anthropic"
        or credential_kind != "anthropic_claude_oauth_session"
        or mode != "builtin_helper"
        or acquisition.get("helper") != "claude-native-oauth"
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    if profile is not None:
        if (
            profile.provider_id != provider_id
            or profile.credential_kind != credential_kind
            or not profile.matches_projection(
                display_name=display_name,
                mode=mode,
                helper=acquisition.get("helper"),
                normalized=normalized,
                signing_bindings=signing_bindings,
                environment=environment,
                complete_files=complete_files,
                json_merges=json_merges,
            )
        ):
            raise BrokerCredentialUnavailable("provider projection is invalid")
    elif credential_kind != PRIMARY_SECRET_FIELD or signing_bindings:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    if not normalized and not signing_bindings:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return normalized, signing_bindings, environment, complete_files, json_merges


def validate_provider_projection(document: Any) -> dict[str, Any]:
    if not isinstance(document, dict):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    projection_version = document.get("schema_version")
    projection_keys = {
        "schema_version",
        "providers",
        "environment",
        "complete_files",
        "json_merges",
        "header_bindings",
        "secret_headers",
    }
    if "signing_bindings" in document:
        projection_keys.add("signing_bindings")
    projection = _exact_keys(
        document,
        projection_keys,
        "provider projection",
    )
    if projection_version != PROVIDER_SCHEMA_VERSION or isinstance(projection_version, bool):
        raise BrokerCredentialUnavailable("provider projection version is invalid")
    providers = projection.get("providers")
    if not isinstance(providers, list) or not 1 <= len(providers) <= 64:
        raise BrokerCredentialUnavailable("provider projection is invalid")

    expected_bindings: list[dict[str, Any]] = []
    expected_signing_bindings: list[dict[str, Any]] = []
    expected_environment: list[dict[str, str]] = []
    expected_files: list[dict[str, str]] = []
    expected_json_merges: list[dict[str, str]] = []
    provider_ids: list[str] = []
    for provider in providers:
        bindings, signing_bindings, environment, complete_files, json_merges = _validate_provider(provider)
        provider_ids.append(provider["id"])
        expected_bindings.extend(bindings)
        expected_signing_bindings.extend(signing_bindings)
        expected_environment.extend(environment)
        expected_files.extend(complete_files)
        expected_json_merges.extend(json_merges)
    if provider_ids != sorted(provider_ids) or len(provider_ids) != len(set(provider_ids)):
        raise BrokerCredentialUnavailable("provider projection is invalid")

    expected_bindings.sort(key=_binding_sort_key)
    expected_signing_bindings.sort(key=lambda item: item["provider_id"])
    expected_environment.sort(key=lambda item: item["name"])
    expected_files.sort(key=lambda item: item["path"])
    expected_json_merges.sort(key=lambda item: item["path"])
    expected_secrets = sorted(
        {
            header
            for binding in expected_bindings
            for header in binding["secret_headers"]
        } | {
            header
            for binding in expected_signing_bindings
            for header in binding["aws_sigv4"]["secret_headers"]
        }
    )
    if (
        projection.get("header_bindings") != expected_bindings
        or projection.get("signing_bindings", []) != expected_signing_bindings
        or projection.get("environment") != expected_environment
        or projection.get("complete_files") != expected_files
        or projection.get("json_merges") != expected_json_merges
        or projection.get("secret_headers") != expected_secrets
    ):
        raise BrokerCredentialUnavailable("provider projection is inconsistent")

    recognition: set[tuple[str, str, int, str, str]] = set()
    environment_names: set[str] = set()
    file_names: set[str] = set()
    for item in expected_environment:
        if item["name"] in environment_names:
            raise BrokerCredentialUnavailable("provider projection is ambiguous")
        environment_names.add(item["name"])
    for item in expected_files:
        if item["path"] in file_names:
            raise BrokerCredentialUnavailable("provider projection is ambiguous")
        file_names.add(item["path"])
    for item in expected_json_merges:
        if item["path"] in file_names:
            raise BrokerCredentialUnavailable("provider projection is ambiguous")
        file_names.add(item["path"])
    for binding in expected_bindings:
        key = (
            binding["target"]["host"],
            binding["target"]["scheme"],
            binding["target"]["port"],
            binding["source"]["header"],
            binding["source"]["format"],
        )
        if key in recognition:
            raise BrokerCredentialUnavailable("provider projection is ambiguous")
        recognition.add(key)
    if len(expected_signing_bindings) > 1:
        raise BrokerCredentialUnavailable("provider projection is ambiguous")
    return projection


def _parse_provider_projection(raw: bytes) -> dict[str, Any]:
    return validate_provider_projection(_decode_json(raw))


class ProviderProjectionSource:
    """Return the current validated projection without stale fallback."""

    def __init__(self, path: str) -> None:
        self._cache = StatIdentityCache(
            path,
            MAX_PROVIDER_PROJECTION_BYTES,
            _parse_provider_projection,
        )

    def load(self) -> dict[str, Any]:
        try:
            return self._cache.load()
        except BrokerCredentialError:
            raise
        except ValidatedFileError as error:
            messages = {
                "path_invalid": "provider projection path is invalid",
                "file_invalid": "provider projection file is invalid",
                "size_invalid": "provider projection size is invalid",
                "changed": "provider projection changed while being read",
            }
            raise BrokerCredentialUnavailable(
                messages.get(error.code, "provider projection is unavailable")
            ) from error


def load_provider_projection(path: str) -> dict[str, Any]:
    """Load one projection; resident callers should retain its source."""

    return ProviderProjectionSource(path).load()


def _broker_response(payload: bytes) -> dict[str, Any]:
    try:
        response = _decode_json(payload)
    except BrokerCredentialUnavailable as error:
        raise _BrokerCredentialResponseInvalid(
            "credential broker returned invalid data"
        ) from error
    if (
        not isinstance(response, dict)
        or response.get("schema_version") != BROKER_SCHEMA_VERSION
        or isinstance(response.get("schema_version"), bool)
    ):
        raise _BrokerCredentialResponseInvalid(
            "credential broker returned invalid data"
        )
    if response.get("ok") is False:
        if set(response) != {"schema_version", "ok", "error"}:
            raise _BrokerCredentialResponseInvalid(
                "credential broker returned invalid data"
            )
        error = response.get("error")
        if not isinstance(error, dict) or set(error) != {"code"} or not isinstance(error.get("code"), str):
            raise _BrokerCredentialResponseInvalid(
                "credential broker returned invalid data"
            )
        if error["code"] == "companion_outcome_unknown":
            raise BrokerCredentialOutcomeUnknown(
                "credential refresh outcome is unknown; host re-login is required"
            )
        if error["code"] in {
            "handle_not_found",
            "handle_revoked",
            "handle_binding_mismatch",
            "invalid_handle",
            "invalid_binding",
            "invalid_context",
            "invalid_project",
            "invalid_provider",
            "invalid_revision",
            "aws_signing_request_invalid",
            "aws_target_unsupported",
            "aws_scope_invalid",
            "aws_scope_target_mismatch",
            "aws_method_invalid",
            "aws_path_unsupported",
            "aws_query_unsupported",
            "aws_payload_hash_invalid",
            "aws_headers_invalid",
            "aws_header_invalid",
        }:
            raise BrokerCredentialBindingError("credential handle is not valid for this request")
        raise BrokerCredentialUnavailable("credential broker is unavailable")
    if response.get("ok") is not True:
        raise _BrokerCredentialResponseInvalid(
            "credential broker returned invalid data"
        )
    return response


def call_broker(path: str, request: dict[str, Any], timeout: float) -> dict[str, Any]:
    if (
        not isinstance(path, str)
        or not path.startswith("/")
        or os.path.normpath(path) != path
        or "\x00" in path
        or len(path.encode("utf-8")) > 103
    ):
        raise BrokerCredentialUnavailable("credential broker socket path is invalid")
    try:
        encoded = json.dumps(
            request, ensure_ascii=True, allow_nan=False, separators=(",", ":"), sort_keys=True
        ).encode("utf-8") + b"\n"
    except (TypeError, ValueError) as error:
        raise BrokerCredentialUnavailable("credential broker request is invalid") from error
    if len(encoded) > MAX_BROKER_FRAME_BYTES:
        raise BrokerCredentialUnavailable("credential broker request is too large")

    refresh_capable_request = request.get("op") == "sign_sigv4" or (
        request.get("op") == "resolve"
        and request.get("provider") in RENEWABLE_PROVIDER_IDS
    )
    send_started = False
    connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    connection.settimeout(timeout)
    try:
        connection.connect(path)
        # Mark before sendall: a partial write can be accepted even when the
        # local call raises, so every later transport loss is outcome-unknown.
        send_started = True
        connection.sendall(encoded)
        connection.shutdown(socket.SHUT_WR)
        response = bytearray()
        while True:
            if len(response) > MAX_BROKER_FRAME_BYTES:
                raise _BrokerCredentialResponseInvalid(
                    "credential broker response is too large"
                )
            chunk = connection.recv(min(8192, MAX_BROKER_FRAME_BYTES + 2 - len(response)))
            if not chunk:
                break
            response.extend(chunk)
    except _BrokerCredentialResponseInvalid as error:
        if refresh_capable_request and send_started:
            raise BrokerCredentialOutcomeUnknown(
                "credential refresh outcome is unknown; host re-login is required"
            ) from error
        raise
    except BrokerCredentialError:
        raise
    except (OSError, TimeoutError) as error:
        if refresh_capable_request and send_started:
            raise BrokerCredentialOutcomeUnknown(
                "credential refresh outcome is unknown; host re-login is required"
            ) from error
        raise BrokerCredentialUnavailable("credential broker is unavailable") from error
    finally:
        connection.close()
    if not response or len(response) > MAX_BROKER_FRAME_BYTES + 1 or response.count(b"\n") != 1 or not response.endswith(b"\n"):
        if refresh_capable_request and send_started:
            raise BrokerCredentialOutcomeUnknown(
                "credential refresh outcome is unknown; host re-login is required"
            )
        raise BrokerCredentialUnavailable("credential broker response is invalid")
    try:
        return _broker_response(bytes(response[:-1]))
    except _BrokerCredentialResponseInvalid as error:
        if refresh_capable_request and send_started:
            raise BrokerCredentialOutcomeUnknown(
                "credential refresh outcome is unknown; host re-login is required"
            ) from error
        raise


@dataclass(frozen=True)
class _HandleCandidate:
    header: str
    source_format: str
    source_scheme: str | None
    handle: str


@dataclass(frozen=True)
class _AWSHandleCandidate:
    handle: str
    region: str
    service: str
    signed_headers: tuple[str, ...]
    expected_body_bytes: int


@dataclass(frozen=True)
class _AWSRequestSnapshot:
    scheme: str
    host: str
    port: int
    method: str
    path: str
    query: str
    headers: tuple[tuple[str, str], ...]
    signed_headers: tuple[tuple[str, str], ...]


def _aws_target_matches(
    binding: dict[str, Any], scheme: str, host: str, port: int
) -> bool:
    plan = binding["aws_sigv4"]
    target = plan["target"]
    return (
        scheme == target["scheme"]
        and port == target["port"]
        and any(host.endswith("." + suffix) for suffix in target["dns_suffixes"])
    )


def _header_values(request: http.Request, name: str) -> list[str]:
    return [
        raw_value.decode("latin-1")
        for raw_name, raw_value in request.headers.fields
        if raw_name.decode("latin-1").lower() == name
    ]


def _find_aws_candidate(
    request: http.Request,
    projection: dict[str, Any],
    scheme: str,
    host: str,
    port: int,
) -> tuple[_AWSHandleCandidate, dict[str, Any]] | None:
    authorization_values = _header_values(request, "authorization")
    token_values = _header_values(request, "x-amz-security-token")
    looks_like_aws = any(
        value.startswith("AWS4-HMAC-SHA256 ") for value in authorization_values
    )
    if not looks_like_aws:
        return None
    if len(authorization_values) != 1 or len(token_values) != 1:
        raise BrokerCredentialBindingError("AWS credential handle position is ambiguous")
    matched = AWS_AUTHORIZATION_PATTERN.fullmatch(authorization_values[0])
    if matched is None:
        if _contains_handle_marker(authorization_values[0]):
            raise BrokerCredentialBindingError("AWS credential handle is malformed")
        return None
    handle, scope_date, region, service, signed_value, _ = matched.groups()
    if HANDLE_PATTERN.fullmatch(handle) is None or token_values[0].strip(" \t") != handle:
        raise BrokerCredentialBindingError("AWS credential handles do not match")
    if (
        AWS_SCOPE_COMPONENT_PATTERN.fullmatch(region) is None
        or AWS_SCOPE_COMPONENT_PATTERN.fullmatch(service) is None
        or AWS_COMMERCIAL_REGION_PATTERN.fullmatch(region) is None
    ):
        raise BrokerCredentialBindingError("AWS signing scope is invalid")
    signed_headers = tuple(signed_value.split(";"))
    if (
        signed_headers != tuple(sorted(set(signed_headers)))
        or not {"host", "x-amz-date", "x-amz-security-token"}.issubset(signed_headers)
    ):
        raise BrokerCredentialBindingError("AWS signed headers are invalid")
    date_values = _header_values(request, "x-amz-date")
    if (
        len(date_values) != 1
        or re.fullmatch(r"[0-9]{8}T[0-9]{6}Z", date_values[0]) is None
        or not date_values[0].startswith(scope_date)
    ):
        raise BrokerCredentialBindingError("AWS signing date is invalid")
    for raw_name, raw_value in request.headers.fields:
        name = raw_name.decode("latin-1").lower()
        value = raw_value.decode("latin-1")
        if _contains_handle_marker(value):
            if name == "authorization":
                if value.count(HANDLE_MARKER) != 1:
                    raise BrokerCredentialBindingError("AWS handle position is ambiguous")
            elif name == "x-amz-security-token":
                if value.strip(" \t") != handle:
                    raise BrokerCredentialBindingError("AWS handle position is invalid")
            else:
                raise BrokerCredentialBindingError("AWS handle position is invalid")
    if _header_values(request, "transfer-encoding"):
        raise BrokerCredentialBindingError("streaming AWS requests are unsupported")
    content_encodings = _header_values(request, "content-encoding")
    content_types = _header_values(request, "content-type")
    content_hashes = _header_values(request, "x-amz-content-sha256")
    if any("aws-chunked" in value.lower() for value in content_encodings) or any(
        _header_values(request, name)
        for name in ("x-amz-decoded-content-length", "x-amz-trailer")
    ):
        raise BrokerCredentialBindingError("streaming AWS requests are unsupported")
    if any(
        value.split(";", 1)[0].strip().lower()
        == "application/vnd.amazon.eventstream"
        for value in content_types
    ):
        raise BrokerCredentialBindingError("AWS event-stream requests are unsupported")
    if len(content_hashes) > 1 or any(
        re.fullmatch(r"[0-9a-f]{64}", value) is None for value in content_hashes
    ):
        # This rejects every streaming/unsigned sentinel, including current and
        # future STREAMING-AWS4-HMAC-SHA256-* variants.
        raise BrokerCredentialBindingError("streaming AWS requests are unsupported")
    if _header_values(request, "x-amz-s3session-token"):
        raise BrokerCredentialBindingError("S3 Express session authentication is unsupported")
    lengths = _header_values(request, "content-length")
    if len(lengths) > 1:
        raise BrokerCredentialBindingError("AWS request length is ambiguous")
    if lengths:
        if re.fullmatch(r"0|[1-9][0-9]*", lengths[0]) is None:
            raise BrokerCredentialBindingError("AWS request length is invalid")
        if int(lengths[0]) > MAX_AWS_BODY_BYTES:
            raise BrokerCredentialBindingError("AWS request body is too large")
        expected_body_bytes = int(lengths[0])
    elif request.method.upper() in {"GET", "HEAD"}:
        expected_body_bytes = 0
    else:
        raise BrokerCredentialBindingError(
            "a bounded AWS request requires Content-Length"
        )
    matches = [
        binding
        for binding in projection["signing_bindings"]
        if binding["kind"] == "aws_sigv4"
        and _aws_target_matches(binding, scheme, host, port)
    ]
    if len(matches) != 1:
        raise BrokerCredentialBindingError("AWS credential handle is not valid for this target")
    return (
        _AWSHandleCandidate(
            handle, region, service, signed_headers, expected_body_bytes
        ),
        matches[0],
    )


def _candidate(header: str, value: str) -> _HandleCandidate | None:
    stripped = value.strip(" \t")
    if stripped.startswith("tobari-h"):
        return _HandleCandidate(header, "raw", None, stripped)
    pieces = re.split(r"[ \t]+", stripped)
    handle_positions = [index for index, piece in enumerate(pieces) if piece.startswith("tobari-h")]
    if not handle_positions:
        return None
    if len(pieces) == 2 and handle_positions == [1]:
        return _HandleCandidate(header, pieces[0].lower(), pieces[0], pieces[1])
    return _HandleCandidate(header, "invalid", None, "")


def _direct_source_format(value: str) -> str | None:
    """Classify a non-handle value by the same exact source syntax as a binding."""

    stripped = value.strip(" \t")
    if not stripped:
        return None
    pieces = re.split(r"[ \t]+", stripped)
    if len(pieces) == 2 and pieces[0].lower() in {"bearer", "token"} and pieces[1]:
        return pieces[0].lower()
    return "raw"


def _direct_header_binding(
    request: http.Request,
    projection: dict[str, Any],
    scheme: str,
    host: str,
    port: int,
) -> dict[str, Any] | None:
    target = {"scheme": scheme, "host": host, "port": port}
    for raw_name, raw_value in request.headers.fields:
        header = raw_name.decode("latin-1").lower()
        source_format = _direct_source_format(raw_value.decode("latin-1"))
        if source_format is None:
            continue
        matches = [
            binding
            for binding in projection["header_bindings"]
            if binding["target"] == target
            and binding["source"]
            == {"header": header, "format": source_format}
        ]
        if len(matches) > 1:
            raise BrokerCredentialUnavailable("provider projection is ambiguous")
        if matches:
            return matches[0]
    return None


def _direct_aws_binding(
    request: http.Request,
    projection: dict[str, Any],
    scheme: str,
    host: str,
    port: int,
) -> dict[str, Any] | None:
    authorization_values = _header_values(request, "authorization")
    if not any(
        value.startswith("AWS4-HMAC-SHA256 ") for value in authorization_values
    ):
        return None
    matches = [
        binding
        for binding in projection["signing_bindings"]
        if binding["kind"] == "aws_sigv4"
        and _aws_target_matches(binding, scheme, host, port)
    ]
    if len(matches) > 1:
        raise BrokerCredentialUnavailable("provider projection is ambiguous")
    return matches[0] if matches else None


def _remove_projected_secret_headers(
    request: http.Request, projection: dict[str, Any]
) -> None:
    for header in projection["secret_headers"]:
        request.headers.pop(header, None)


def _contains_handle_marker(value: str) -> bool:
    current = value
    for _ in range(3):
        if HANDLE_MARKER in current:
            return True
        decoded = unquote(current)
        if decoded == current:
            return False
        current = decoded
    return HANDLE_MARKER in current


def _remove_handle_headers(request: http.Request) -> None:
    names = {
        raw_name.decode("latin-1").lower()
        for raw_name, raw_value in request.headers.fields
        if _contains_handle_marker(raw_name.decode("latin-1"))
        or _contains_handle_marker(raw_value.decode("latin-1"))
    }
    for name in names:
        request.headers.pop(name, None)


def redacted_audit_path(url: str) -> str:
    path = urlsplit(url).path or "/"
    if _contains_handle_marker(path):
        return "/[redacted-auth-handle]"
    return path


def _reject_non_header_handle_positions(request: http.Request) -> None:
    split = urlsplit(request.url)
    # This is bounded structural inspection of URL components, not generic
    # request-byte replacement. A handle is never a supported URL credential
    # position and must not reach OPA, audit, or upstream.
    for component in (split.path, split.query, split.fragment):
        if _contains_handle_marker(component):
            raise BrokerCredentialBindingError(
                "credential handle is not valid in the request target"
            )


def _find_candidate(
    request: http.Request,
    projection: dict[str, Any],
    scheme: str,
    host: str,
    port: int,
) -> tuple[_HandleCandidate, dict[str, Any]] | None:
    _reject_non_header_handle_positions(request)
    candidates: list[_HandleCandidate] = []
    for raw_name, raw_value in request.headers.fields:
        name = raw_name.decode("latin-1").lower()
        value = raw_value.decode("latin-1")
        if HANDLE_MARKER in name:
            raise BrokerCredentialBindingError(
                "credential handle is not valid in a header name"
            )
        candidate = _candidate(name, value)
        if _contains_handle_marker(value) and candidate is None:
            raise BrokerCredentialBindingError(
                "credential handle is not valid in this header position"
            )
        if candidate is not None:
            candidates.append(candidate)
    if not candidates:
        return None
    if len(candidates) != 1:
        raise BrokerCredentialBindingError("credential handle position is ambiguous")
    candidate = candidates[0]
    if HANDLE_PATTERN.fullmatch(candidate.handle) is None:
        raise BrokerCredentialBindingError("credential handle is malformed")
    matches = [
        binding
        for binding in projection["header_bindings"]
        if binding["target"] == {"scheme": scheme, "host": host, "port": port}
        and binding["source"]
        == {"header": candidate.header, "format": candidate.source_format}
    ]
    if len(matches) != 1:
        raise BrokerCredentialBindingError("credential handle is not valid for this target")
    return candidate, matches[0]


def _validate_broker_metadata(
    response: dict[str, Any],
    binding: dict[str, Any],
    *,
    include_secret: bool,
) -> tuple[str, bytes | None, dict[str, str]]:
    profile = response_profile(
        binding["provider_id"], binding["destination"]["secret_field"]
    )
    supplemental_names = (
        profile.supplemental_header_names if profile is not None else frozenset()
    )
    expected = {
        "schema_version",
        "ok",
        "provider",
        "revision",
        "target",
        "source",
        "destination",
        "secret_headers",
    }
    if include_secret:
        expected.add("secret")
        if supplemental_names:
            expected.add("supplemental_headers")
    if set(response) != expected:
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    revision = response.get("revision")
    if (
        response.get("provider") != binding["provider_id"]
        or not isinstance(revision, str)
        or REVISION_PATTERN.fullmatch(revision) is None
        or response.get("target") != binding["target"]
        or response.get("source") != binding["source"]
        or response.get("destination") != binding["destination"]
        or response.get("secret_headers") != binding["secret_headers"]
    ):
        raise BrokerCredentialUnavailable("credential broker returned inconsistent data")
    if not include_secret:
        return revision, None, {}
    secret_document = _exact_keys(
        response.get("secret"), {"field", "encoding", "value"}, "credential broker secret"
    )
    encoded = secret_document.get("value")
    if (
        secret_document.get("field") != binding["destination"]["secret_field"]
        or secret_document.get("encoding") != "base64url"
        or not isinstance(encoded, str)
        or not encoded
        or "=" in encoded
    ):
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    try:
        secret = base64.b64decode(
            encoded.encode("ascii") + b"=" * (-len(encoded) % 4),
            altchars=b"-_",
            validate=True,
        )
    except (UnicodeEncodeError, ValueError) as error:
        raise BrokerCredentialUnavailable("credential broker returned invalid data") from error
    if (
        not secret
        or len(secret) > MAX_SECRET_BYTES
        or base64.urlsafe_b64encode(secret).rstrip(b"=").decode("ascii") != encoded
        or any(byte < 0x20 or byte == 0x7F for byte in secret)
    ):
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    supplemental_headers: dict[str, str] = {}
    if supplemental_names:
        supplemental_headers = profile.validate_supplemental_headers(  # type: ignore[union-attr]
            response.get("supplemental_headers")
        )
        if supplemental_headers is None:
            raise BrokerCredentialUnavailable("credential broker returned invalid data")
    return revision, secret, supplemental_headers


def _validate_signing_introspection(
    response: dict[str, Any], binding: dict[str, Any], target: dict[str, Any]
) -> str:
    expected = {
        "schema_version",
        "ok",
        "provider",
        "revision",
        "kind",
        "target",
        "source",
        "secret_headers",
    }
    revision = response.get("revision")
    plan = binding["aws_sigv4"]
    if (
        set(response) != expected
        or response.get("provider") != binding["provider_id"]
        or not isinstance(revision, str)
        or REVISION_PATTERN.fullmatch(revision) is None
        or response.get("kind") != "aws_sigv4"
        or response.get("target") != target
        or response.get("source") != plan["source"]
        or response.get("secret_headers") != plan["secret_headers"]
    ):
        raise BrokerCredentialUnavailable("credential broker returned inconsistent data")
    return revision


def _validated_signing_headers(
    response: dict[str, Any],
    provider: str,
    revision: str,
    candidate: _AWSHandleCandidate,
    payload_hash: str,
) -> dict[str, str | None]:
    if set(response) != {"schema_version", "ok", "provider", "revision", "headers"}:
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    if response.get("provider") != provider or response.get("revision") != revision:
        raise BrokerCredentialUnavailable("credential broker returned inconsistent data")
    headers = _exact_keys(
        response.get("headers"),
        {
            "authorization",
            "x_amz_date",
            "x_amz_security_token",
            "x_amz_content_sha256",
        },
        "credential broker AWS headers",
    )
    authorization = headers.get("authorization")
    amz_date = headers.get("x_amz_date")
    security_token = headers.get("x_amz_security_token")
    content_hash = headers.get("x_amz_content_sha256")
    matched = (
        AWS_SIGNED_AUTHORIZATION_PATTERN.fullmatch(authorization)
        if isinstance(authorization, str)
        else None
    )
    signed_headers: tuple[str, ...] = ()
    if matched is not None:
        _, scope_date, region, service, signed_value, _ = matched.groups()
        signed_headers = tuple(signed_value.split(";"))
    expected_signed_headers = {
        name
        for name in candidate.signed_headers
        if name
        not in {
            "host",
            "authorization",
            "x-amz-date",
            "x-amz-security-token",
            "x-amz-content-sha256",
        }
    } | {"host", "x-amz-date", "x-amz-security-token"}
    if content_hash is not None:
        expected_signed_headers.add("x-amz-content-sha256")
    if (
        not isinstance(authorization, str)
        or matched is None
        or len(authorization) > 4096
        or any(character in "\x00\r\n" for character in authorization)
        or _contains_handle_marker(authorization)
        or not isinstance(amz_date, str)
        or re.fullmatch(r"[0-9]{8}T[0-9]{6}Z", amz_date) is None
        or scope_date != amz_date[:8]
        or region != candidate.region
        or service != candidate.service
        or signed_headers != tuple(sorted(set(signed_headers)))
        or set(signed_headers) != expected_signed_headers
        or not isinstance(security_token, str)
        or not 16 <= len(security_token) <= 16 * 1024
        or any(ord(character) < 0x21 or ord(character) > 0x7E for character in security_token)
        or _contains_handle_marker(security_token)
        or (
            content_hash is not None
            and (
                not isinstance(content_hash, str)
                or re.fullmatch(r"[0-9a-f]{64}", content_hash) is None
                or content_hash != payload_hash
            )
        )
    ):
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    return {
        "authorization": authorization,
        "x_amz_date": amz_date,
        "x_amz_security_token": security_token,
        "x_amz_content_sha256": content_hash,
    }


@dataclass
class _FallbackRequest:
    request: PreparedCredentialRequest
    projection_secret_headers: set[str]
    broker_provider: str | None = None

    @property
    def secret_headers(self) -> set[str]:
        return set(self.request.secret_headers) | self.projection_secret_headers

    def apply(self, request: http.Request) -> None:
        self.request.apply(request)


@dataclass
class _BrokerRequest:
    binding: dict[str, Any]
    candidate: _HandleCandidate
    context_id: str
    project_id: str
    revision: str
    broker_call: Callable[[dict[str, Any]], dict[str, Any]]
    @property
    def broker_provider(self) -> str:
        return self.binding["provider_id"]

    @property
    def secret_headers(self) -> set[str]:
        return set(DEFAULT_SECRET_HEADERS) | set(self.binding["secret_headers"])

    def _request(self, operation: str) -> dict[str, Any]:
        request = {
            "schema_version": BROKER_SCHEMA_VERSION,
            "op": operation,
            "handle": self.candidate.handle,
            "context_id": self.context_id,
            "project_id": self.project_id,
            "provider": self.binding["provider_id"],
            "target": self.binding["target"],
            "source_header": self.binding["source"]["header"],
            "source_format": self.binding["source"]["format"],
        }
        if operation == "resolve":
            request["revision"] = self.revision
        return request

    def apply(self, request: http.Request) -> None:
        response = self.broker_call(self._request("resolve"))
        revision, secret, supplemental_headers = _validate_broker_metadata(
            response, self.binding, include_secret=True
        )
        if revision != self.revision or secret is None:
            raise BrokerCredentialUnavailable("credential broker returned inconsistent data")
        destination = self.binding["destination"]["header"]
        for header in self.binding["secret_headers"]:
            request.headers.pop(header, None)
        for header in {"proxy-authorization"} | set(CONTROL_HEADERS):
            request.headers.pop(header, None)
        value = secret.decode("latin-1")
        output_format = self.binding["destination"]["format"]
        if output_format == "preserve_scheme":
            if self.binding["source"]["format"] == "raw":
                rendered = value
            elif self.candidate.source_scheme is not None:
                rendered = f"{self.candidate.source_scheme} {value}"
            else:
                raise BrokerCredentialUnavailable("credential broker binding is invalid")
        elif output_format == "raw":
            rendered = value
        elif output_format == "bearer":
            rendered = f"Bearer {value}"
        elif output_format == "token":
            rendered = f"token {value}"
        else:
            raise BrokerCredentialUnavailable("credential broker binding is invalid")
        request.headers[destination] = rendered
        for header, header_value in supplemental_headers.items():
            request.headers[header] = header_value
        return None


@dataclass
class _AWSSigV4Request:
    binding: dict[str, Any]
    candidate: _AWSHandleCandidate
    target: dict[str, Any]
    context_id: str
    project_id: str
    revision: str
    broker_call: Callable[[dict[str, Any]], dict[str, Any]]
    deferred: bool = True
    snapshot: _AWSRequestSnapshot | None = None

    @property
    def broker_provider(self) -> str:
        return self.binding["provider_id"]

    @property
    def secret_headers(self) -> set[str]:
        return set(DEFAULT_SECRET_HEADERS) | set(
            self.binding["aws_sigv4"]["secret_headers"]
        )

    def apply(self, request: http.Request) -> None:
        for header in set(CONTROL_HEADERS) | {"proxy-authorization"}:
            request.headers.pop(header, None)
        if self.snapshot is not None:
            raise BrokerCredentialBindingError("AWS request was already authorized")
        self.snapshot = self._snapshot_request(request)
        return None

    def _snapshot_request(self, request: http.Request) -> _AWSRequestSnapshot:
        split = urlsplit(request.url)
        headers = tuple(
            (
                raw_name.decode("latin-1").lower(),
                raw_value.decode("latin-1"),
            )
            for raw_name, raw_value in request.headers.fields
        )
        signed_headers = tuple(
            (name, value)
            for name, value in self._signed_request_headers(request)
        )
        return _AWSRequestSnapshot(
            scheme=request.scheme,
            host=request.host,
            port=request.port,
            method=request.method.upper(),
            path=split.path or "/",
            query=split.query,
            headers=headers,
            signed_headers=signed_headers,
        )

    def _signed_request_headers(self, request: http.Request) -> list[list[str]]:
        broker_owned = {
            "host",
            "authorization",
            "x-amz-date",
            "x-amz-security-token",
            "x-amz-content-sha256",
        }
        selected: list[list[str]] = []
        signed = set(self.candidate.signed_headers)
        for name in self.candidate.signed_headers:
            if name in broker_owned:
                continue
            values = _header_values(request, name)
            if not values:
                raise BrokerCredentialBindingError("an AWS signed header is missing")
            selected.extend([[name, value] for value in values])
        for raw_name, _ in request.headers.fields:
            name = raw_name.decode("latin-1").lower()
            if name.startswith("x-amz-") and name not in signed and name not in broker_owned:
                raise BrokerCredentialBindingError("an unsigned AWS header is unsupported")
        return selected

    def apply_body(self, request: http.Request) -> None:
        snapshot = self.snapshot
        if snapshot is None or self._snapshot_request(request) != snapshot:
            raise BrokerCredentialBindingError(
                "AWS request changed after policy authorization"
            )
        body = request.raw_content
        if body is None:
            body = b""
        if not isinstance(body, bytes) or len(body) > MAX_AWS_BODY_BYTES:
            raise BrokerCredentialBindingError("AWS request body is too large")
        lengths = _header_values(request, "content-length")
        if len(body) != self.candidate.expected_body_bytes:
            raise BrokerCredentialBindingError("AWS request body length changed")
        if lengths:
            if (
                len(lengths) != 1
                or re.fullmatch(r"0|[1-9][0-9]*", lengths[0]) is None
                or int(lengths[0]) != self.candidate.expected_body_bytes
            ):
                raise BrokerCredentialBindingError("AWS request body length changed")
        response = self.broker_call(
            {
                "schema_version": BROKER_SCHEMA_VERSION,
                "op": "sign_sigv4",
                "handle": self.candidate.handle,
                "context_id": self.context_id,
                "project_id": self.project_id,
                "provider": self.binding["provider_id"],
                "revision": self.revision,
                "binding": self.binding,
                "request": {
                    "host": self.target["host"],
                    "method": snapshot.method,
                    "path": snapshot.path,
                    "query": snapshot.query,
                    "region": self.candidate.region,
                    "service": self.candidate.service,
                    "headers": [list(item) for item in snapshot.signed_headers],
                    "payload_hash": hashlib.sha256(body).hexdigest(),
                },
            }
        )
        rendered = _validated_signing_headers(
            response,
            self.binding["provider_id"],
            self.revision,
            self.candidate,
            hashlib.sha256(body).hexdigest(),
        )
        for name in {
            "authorization",
            "x-amz-date",
            "x-amz-security-token",
            "x-amz-content-sha256",
        }:
            request.headers.pop(name, None)
        request.headers["authorization"] = rendered["authorization"]  # type: ignore[assignment]
        request.headers["x-amz-date"] = rendered["x_amz_date"]  # type: ignore[assignment]
        request.headers["x-amz-security-token"] = rendered["x_amz_security_token"]  # type: ignore[assignment]
        if rendered["x_amz_content_sha256"] is not None:
            request.headers["x-amz-content-sha256"] = rendered[
                "x_amz_content_sha256"
            ]  # type: ignore[assignment]


class BrokeredCredentialAdapter:
    """Require handles at declared bindings; retain fallback only elsewhere."""

    name = "brokered"

    def __init__(
        self,
        fallback: CredentialAdapter,
        projection_path: str,
        socket_path: str,
        timeout: float,
        *,
        projection_loader: Callable[[str], dict[str, Any]] | None = None,
        caller: Callable[[str, dict[str, Any], float], dict[str, Any]] = call_broker,
    ) -> None:
        self.fallback = fallback
        self.projection_path = projection_path
        self.socket_path = socket_path
        self.timeout = timeout
        if projection_loader is None:
            projection_source = ProviderProjectionSource(projection_path)
            self.projection_loader = lambda _path: projection_source.load()
        else:
            self.projection_loader = projection_loader
        self.caller = caller

    def _call(self, request: dict[str, Any]) -> dict[str, Any]:
        return self.caller(self.socket_path, request, self.timeout)

    def prepare(
        self,
        request: http.Request,
        scheme: str,
        host: str,
        port: int,
        context_id: str,
        project_id: str,
    ) -> PreparedCredentialRequest:
        if PROFILE_HEADER in request.headers:
            request.headers.pop(PROFILE_HEADER, None)
            raise BrokerCredentialBindingError(
                "retired credential profile selector is unsupported"
            )
        projection = self.projection_loader(self.projection_path)
        _reject_non_header_handle_positions(request)
        try:
            aws_selected = _find_aws_candidate(
                request, projection, scheme, host, port
            )
        except BrokerCredentialBindingError:
            _remove_handle_headers(request)
            raise
        if aws_selected is not None:
            candidate, binding = aws_selected
            for name in {
                "authorization",
                "x-amz-date",
                "x-amz-security-token",
                "x-amz-content-sha256",
            }:
                request.headers.pop(name, None)
            target = {"scheme": scheme, "host": host, "port": port}
            introspection = self._call(
                {
                    "schema_version": BROKER_SCHEMA_VERSION,
                    "op": "introspect_signing",
                    "handle": candidate.handle,
                    "context_id": context_id,
                    "project_id": project_id,
                    "provider": binding["provider_id"],
                    "target": target,
                    "binding": binding,
                }
            )
            revision = _validate_signing_introspection(
                introspection, binding, target
            )
            return _AWSSigV4Request(
                binding=binding,
                candidate=candidate,
                target=target,
                context_id=context_id,
                project_id=project_id,
                revision=revision,
                broker_call=self._call,
            )
        if _direct_aws_binding(request, projection, scheme, host, port) is not None:
            _remove_projected_secret_headers(request, projection)
            raise BrokerAuthenticationRequired(
                "broker authentication is required for this provider binding"
            )
        try:
            selected = _find_candidate(request, projection, scheme, host, port)
        except BrokerCredentialBindingError:
            _remove_handle_headers(request)
            raise
        if selected is None:
            if _direct_header_binding(
                request, projection, scheme, host, port
            ) is not None:
                _remove_projected_secret_headers(request, projection)
                raise BrokerAuthenticationRequired(
                    "broker authentication is required for this provider binding"
                )
            fallback = self.fallback.prepare(
                request, scheme, host, port, context_id, project_id
            )
            return _FallbackRequest(fallback, set(projection["secret_headers"]))
        candidate, binding = selected
        # Remove every secret-sensitive header in the exact binding before
        # crossing either the broker or OPA boundary. This includes caller-
        # supplied routing headers that only the broker may restore after allow.
        for header in binding["secret_headers"]:
            request.headers.pop(header, None)
        introspection = self._call(
            {
                "schema_version": BROKER_SCHEMA_VERSION,
                "op": "introspect",
                "handle": candidate.handle,
                "context_id": context_id,
                "project_id": project_id,
                "provider": binding["provider_id"],
                "target": binding["target"],
                "source_header": binding["source"]["header"],
                "source_format": binding["source"]["format"],
            }
        )
        revision, _, _ = _validate_broker_metadata(
            introspection, binding, include_secret=False
        )
        return _BrokerRequest(
            binding=binding,
            candidate=candidate,
            context_id=context_id,
            project_id=project_id,
            revision=revision,
            broker_call=self._call,
        )
