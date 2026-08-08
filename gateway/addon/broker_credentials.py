"""Strict per-request Auth Broker integration for the Tobari Gateway.

Provider projection is host-owned, contains no credentials, and is validated
again here before it can influence request recognition.  A broker handle is
removed from the mitmproxy flow before either the broker or OPA is called; the
only copy retained by this module lives in one request-scoped object.
"""

from __future__ import annotations

import base64
import json
import os
import re
import socket
import stat
from dataclasses import dataclass
from typing import Any, Callable
from urllib.parse import unquote, urlsplit

from mitmproxy import http

from credential_adapters import (
    CONTROL_HEADERS,
    DEFAULT_SECRET_HEADERS,
    CredentialAdapter,
    CredentialAdapterError,
    PreparedCredentialRequest,
)


PROVIDER_SCHEMA_VERSION = 1
BROKER_SCHEMA_VERSION = 1
MAX_PROVIDER_PROJECTION_BYTES = 16 * 1024 * 1024
MAX_BROKER_FRAME_BYTES = 64 * 1024
MAX_SECRET_BYTES = 32 * 1024
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
FORBIDDEN_HEADERS = frozenset(
    {"host", "content-length", "proxy-authorization", "cookie", "set-cookie"}
)


class BrokerCredentialError(CredentialAdapterError):
    """A secret-free broker failure safe to report only as a generic fault."""


class BrokerCredentialBindingError(BrokerCredentialError):
    """A Tobari-looking handle is invalid for this exact trusted principal."""


class BrokerCredentialUnavailable(BrokerCredentialError):
    """The private broker boundary is unavailable or returned an invalid frame."""


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
        or destination.get("secret_field") != "primary_secret"
    ):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return {
        "header": header,
        "format": output_format,
        "secret_field": "primary_secret",
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


def _validate_provider(provider: Any) -> tuple[list[dict[str, Any]], list[dict[str, str]], list[dict[str, str]]]:
    provider = _exact_keys(
        provider,
        {
            "schema_version",
            "id",
            "display_name",
            "acquisition",
            "credential",
            "workspace_projections",
            "header_bindings",
        },
        "provider",
    )
    provider_id = provider.get("id")
    display_name = provider.get("display_name")
    if (
        provider.get("schema_version") != PROVIDER_SCHEMA_VERSION
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
    if credential.get("kind") != "primary_secret":
        raise BrokerCredentialUnavailable("provider projection is invalid")

    workspace = provider.get("workspace_projections")
    if not isinstance(workspace, list) or not 1 <= len(workspace) <= 32:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    environment: list[dict[str, str]] = []
    complete_files: list[dict[str, str]] = []
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
        if set(item) != expected or kind not in {"env", "complete_file"}:
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
            complete_files.append(
                {"provider_id": provider_id, "path": path, "template": template}
            )
            workspace_keys.append((kind, path))
    if handle_count < 1 or handle_count > len(workspace) or workspace_keys != sorted(workspace_keys):
        raise BrokerCredentialUnavailable("provider projection is invalid")

    raw_bindings = provider.get("header_bindings")
    if not isinstance(raw_bindings, list) or not 1 <= len(raw_bindings) <= 64:
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
    return normalized, environment, complete_files


def validate_provider_projection(document: Any) -> dict[str, Any]:
    projection = _exact_keys(
        document,
        {
            "schema_version",
            "providers",
            "environment",
            "complete_files",
            "header_bindings",
            "secret_headers",
        },
        "provider projection",
    )
    if projection.get("schema_version") != PROVIDER_SCHEMA_VERSION or isinstance(
        projection.get("schema_version"), bool
    ):
        raise BrokerCredentialUnavailable("provider projection version is invalid")
    providers = projection.get("providers")
    if not isinstance(providers, list) or not 1 <= len(providers) <= 64:
        raise BrokerCredentialUnavailable("provider projection is invalid")

    expected_bindings: list[dict[str, Any]] = []
    expected_environment: list[dict[str, str]] = []
    expected_files: list[dict[str, str]] = []
    provider_ids: list[str] = []
    for provider in providers:
        bindings, environment, complete_files = _validate_provider(provider)
        provider_ids.append(provider["id"])
        expected_bindings.extend(bindings)
        expected_environment.extend(environment)
        expected_files.extend(complete_files)
    if provider_ids != sorted(provider_ids) or len(provider_ids) != len(set(provider_ids)):
        raise BrokerCredentialUnavailable("provider projection is invalid")

    expected_bindings.sort(key=_binding_sort_key)
    expected_environment.sort(key=lambda item: item["name"])
    expected_files.sort(key=lambda item: item["path"])
    expected_secrets = sorted(
        {
            header
            for binding in expected_bindings
            for header in binding["secret_headers"]
        }
    )
    if (
        projection.get("header_bindings") != expected_bindings
        or projection.get("environment") != expected_environment
        or projection.get("complete_files") != expected_files
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
    return projection


def load_provider_projection(path: str) -> dict[str, Any]:
    if not isinstance(path, str) or not path or not os.path.isabs(path) or os.path.normpath(path) != path:
        raise BrokerCredentialUnavailable("provider projection path is invalid")
    flags = os.O_RDONLY
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    try:
        descriptor = os.open(path, flags)
        try:
            info = os.fstat(descriptor)
            if (
                not stat.S_ISREG(info.st_mode)
                or info.st_uid != os.geteuid()
                or stat.S_IMODE(info.st_mode) != 0o600
            ):
                raise BrokerCredentialUnavailable("provider projection file is invalid")
            chunks: list[bytes] = []
            remaining = MAX_PROVIDER_PROJECTION_BYTES + 1
            while remaining:
                chunk = os.read(descriptor, min(65536, remaining))
                if not chunk:
                    break
                chunks.append(chunk)
                remaining -= len(chunk)
        finally:
            os.close(descriptor)
    except BrokerCredentialError:
        raise
    except OSError as error:
        raise BrokerCredentialUnavailable("provider projection is unavailable") from error
    raw = b"".join(chunks)
    if not raw or len(raw) > MAX_PROVIDER_PROJECTION_BYTES:
        raise BrokerCredentialUnavailable("provider projection size is invalid")
    return validate_provider_projection(_decode_json(raw))


def _broker_response(payload: bytes) -> dict[str, Any]:
    response = _decode_json(payload)
    if (
        not isinstance(response, dict)
        or response.get("schema_version") != BROKER_SCHEMA_VERSION
        or isinstance(response.get("schema_version"), bool)
    ):
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    if response.get("ok") is False:
        if set(response) != {"schema_version", "ok", "error"}:
            raise BrokerCredentialUnavailable("credential broker returned invalid data")
        error = response.get("error")
        if not isinstance(error, dict) or set(error) != {"code"} or not isinstance(error.get("code"), str):
            raise BrokerCredentialUnavailable("credential broker returned invalid data")
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
        }:
            raise BrokerCredentialBindingError("credential handle is not valid for this request")
        raise BrokerCredentialUnavailable("credential broker is unavailable")
    if response.get("ok") is not True:
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
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

    connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    connection.settimeout(timeout)
    try:
        connection.connect(path)
        connection.sendall(encoded)
        connection.shutdown(socket.SHUT_WR)
        response = bytearray()
        while True:
            if len(response) > MAX_BROKER_FRAME_BYTES:
                raise BrokerCredentialUnavailable("credential broker response is too large")
            chunk = connection.recv(min(8192, MAX_BROKER_FRAME_BYTES + 2 - len(response)))
            if not chunk:
                break
            response.extend(chunk)
    except BrokerCredentialError:
        raise
    except (OSError, TimeoutError) as error:
        raise BrokerCredentialUnavailable("credential broker is unavailable") from error
    finally:
        connection.close()
    if not response or len(response) > MAX_BROKER_FRAME_BYTES + 1 or response.count(b"\n") != 1 or not response.endswith(b"\n"):
        raise BrokerCredentialUnavailable("credential broker response is invalid")
    return _broker_response(bytes(response[:-1]))


@dataclass(frozen=True)
class _HandleCandidate:
    header: str
    source_format: str
    source_scheme: str | None
    handle: str


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
) -> tuple[str, bytes | None]:
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
        return revision, None
    secret_document = _exact_keys(
        response.get("secret"), {"field", "encoding", "value"}, "credential broker secret"
    )
    encoded = secret_document.get("value")
    if (
        secret_document.get("field") != "primary_secret"
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
    return revision, secret


@dataclass
class _FallbackRequest:
    request: PreparedCredentialRequest
    projection_secret_headers: set[str]
    broker_provider: str | None = None

    @property
    def requested_profile(self) -> str | None:
        return self.request.requested_profile

    @property
    def secret_headers(self) -> set[str]:
        return set(self.request.secret_headers) | self.projection_secret_headers

    def apply(self, request: http.Request, selected_profile: str | None) -> str | None:
        return self.request.apply(request, selected_profile)


@dataclass
class _BrokerRequest:
    binding: dict[str, Any]
    candidate: _HandleCandidate
    context_id: str
    project_id: str
    revision: str
    broker_call: Callable[[dict[str, Any]], dict[str, Any]]
    requested_profile: str | None = None

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

    def apply(self, request: http.Request, selected_profile: str | None) -> str | None:
        if selected_profile is not None:
            raise BrokerCredentialBindingError(
                "policy selected an incompatible static credential profile"
            )
        response = self.broker_call(self._request("resolve"))
        revision, secret = _validate_broker_metadata(
            response, self.binding, include_secret=True
        )
        if revision != self.revision or secret is None:
            raise BrokerCredentialUnavailable("credential broker returned inconsistent data")
        source = self.binding["source"]["header"]
        destination = self.binding["destination"]["header"]
        request.headers.pop(source, None)
        request.headers.pop(destination, None)
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
        return None


class BrokeredCredentialAdapter:
    """Recognize broker handles per request, otherwise retain the old adapter."""

    name = "brokered"

    def __init__(
        self,
        fallback: CredentialAdapter,
        projection_path: str,
        socket_path: str,
        timeout: float,
        *,
        projection_loader: Callable[[str], dict[str, Any]] = load_provider_projection,
        caller: Callable[[str, dict[str, Any], float], dict[str, Any]] = call_broker,
    ) -> None:
        self.fallback = fallback
        self.projection_path = projection_path
        self.socket_path = socket_path
        self.timeout = timeout
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
        projection = self.projection_loader(self.projection_path)
        selected = _find_candidate(request, projection, scheme, host, port)
        if selected is None:
            fallback = self.fallback.prepare(
                request, scheme, host, port, context_id, project_id
            )
            return _FallbackRequest(fallback, set(projection["secret_headers"]))
        candidate, binding = selected
        # Remove every copy of the recognized source before crossing either the
        # broker or OPA boundary.  It cannot subsequently enter mitmproxy logs,
        # an audit event, a denial body, or an upstream request on failure.
        request.headers.pop(candidate.header, None)
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
        revision, _ = _validate_broker_metadata(
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
