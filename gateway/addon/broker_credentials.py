"""Static project-bound Auth Broker integration for the Tobari Gateway."""

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
    CONTROL_HEADERS, DEFAULT_SECRET_HEADERS, CredentialAdapter,
    CredentialAdapterError, PreparedCredentialRequest,
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
HOST_PATTERN = re.compile(r"^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$")
HEADER_PATTERN = re.compile(r"^[!#$%&'*+.^_`|~0-9a-z-]{1,64}$")
SOURCE_FORMATS = frozenset({"raw", "bearer", "token"})
DESTINATION_FORMATS = frozenset({"preserve_scheme", "raw", "bearer", "token"})
FORBIDDEN_HEADERS = frozenset({"host", "content-length", "proxy-authorization", "cookie", "set-cookie"})


class BrokerCredentialError(CredentialAdapterError): pass
class BrokerCredentialBindingError(BrokerCredentialError): pass
class BrokerCredentialUnavailable(BrokerCredentialError): pass


def _strict_object(items: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in items:
        if key in result:
            raise ValueError("duplicate")
        result[key] = value
    return result


def _decode_json(payload: bytes) -> Any:
    try:
        return json.loads(payload.decode("utf-8"), object_pairs_hook=_strict_object,
                          parse_constant=lambda _: (_ for _ in ()).throw(ValueError("constant")))
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError) as error:
        raise BrokerCredentialUnavailable("invalid broker data") from error


def _exact(value: Any, keys: set[str]) -> dict[str, Any]:
    if not isinstance(value, dict) or set(value) != keys:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return value


def _header(value: Any) -> str:
    if (not isinstance(value, str) or value != value.lower() or HEADER_PATTERN.fullmatch(value) is None
            or value in FORBIDDEN_HEADERS or value.startswith("x-tobari-")):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return value


def _target(value: Any) -> dict[str, Any]:
    target = _exact(value, {"scheme", "host", "port"})
    host, port = target.get("host"), target.get("port")
    if (target.get("scheme") != "https" or not isinstance(host, str) or host != host.lower()
            or HOST_PATTERN.fullmatch(host) is None or "." not in host
            or isinstance(port, bool) or not isinstance(port, int) or not 1 <= port <= 65535):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    return target


def _binding(value: Any, provider: str) -> dict[str, Any]:
    item = _exact(value, {"provider_id", "target", "source", "destination", "secret_headers"})
    source = _exact(item.get("source"), {"header", "format"})
    destination = _exact(item.get("destination"), {"header", "format", "secret_field"})
    headers = item.get("secret_headers")
    if (item.get("provider_id") != provider or source.get("format") not in SOURCE_FORMATS
            or destination.get("format") not in DESTINATION_FORMATS
            or destination.get("secret_field") != "primary_secret"
            or not isinstance(headers, list) or not headers or headers != sorted(set(headers))):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    source_header, destination_header = _header(source.get("header")), _header(destination.get("header"))
    for name in headers: _header(name)
    if source_header not in headers or destination_header not in headers:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    _target(item.get("target"))
    return item


def validate_provider_projection(document: Any) -> dict[str, Any]:
    projection = _exact(document, {"schema_version", "providers", "environment", "complete_files", "header_bindings", "secret_headers"})
    providers = projection.get("providers")
    if projection.get("schema_version") != PROVIDER_SCHEMA_VERSION or not isinstance(providers, list) or not providers:
        raise BrokerCredentialUnavailable("provider projection is invalid")
    provider_ids: list[str] = []
    expected_bindings: list[dict[str, Any]] = []
    expected_environment: list[dict[str, str]] = []
    expected_files: list[dict[str, str]] = []
    for raw in providers:
        provider = _exact(raw, {"schema_version", "id", "display_name", "acquisition", "credential", "workspace_projections", "header_bindings"})
        provider_id = provider.get("id")
        if (provider.get("schema_version") != 1 or not isinstance(provider_id, str)
                or PROVIDER_ID_PATTERN.fullmatch(provider_id) is None
                or provider.get("credential") != {"kind": "primary_secret"}
                or not isinstance(provider.get("acquisition"), dict)):
            raise BrokerCredentialUnavailable("provider projection is invalid")
        acquisition = provider["acquisition"]
        if (
            provider_id == "github"
            and acquisition != {"mode": "builtin_helper", "helper": "github-gh"}
        ) or (
            provider_id != "github" and acquisition != {"mode": "stdin_import"}
        ):
            raise BrokerCredentialUnavailable("provider projection is invalid")
        provider_ids.append(provider_id)
        raw_bindings = provider.get("header_bindings")
        if not isinstance(raw_bindings, list) or not raw_bindings:
            raise BrokerCredentialUnavailable("provider projection is invalid")
        for raw_binding in raw_bindings:
            base = _exact(raw_binding, {"target", "source", "destination", "secret_headers"})
            formats = _exact(base.get("source"), {"header", "formats"}).get("formats")
            if not isinstance(formats, list) or formats != sorted(set(formats)):
                raise BrokerCredentialUnavailable("provider projection is invalid")
            for source_format in formats:
                normalized = {"provider_id": provider_id, "target": base["target"],
                    "source": {"header": base["source"]["header"], "format": source_format},
                    "destination": base["destination"], "secret_headers": base["secret_headers"]}
                expected_bindings.append(_binding(normalized, provider_id))
        projections = provider.get("workspace_projections")
        if not isinstance(projections, list):
            raise BrokerCredentialUnavailable("provider projection is invalid")
        for item in projections:
            if not isinstance(item, dict) or item.get("kind") not in {"env", "complete_file"}:
                raise BrokerCredentialUnavailable("provider projection is invalid")
            normalized = {"provider_id": provider_id, **{key: val for key, val in item.items() if key != "kind"}}
            (expected_environment if item["kind"] == "env" else expected_files).append(normalized)
    if provider_ids != sorted(set(provider_ids)):
        raise BrokerCredentialUnavailable("provider projection is invalid")
    expected_bindings.sort(key=lambda item: (item["target"]["host"], item["source"]["header"], item["source"]["format"], item["provider_id"]))
    expected_environment.sort(key=lambda item: item["name"])
    expected_files.sort(key=lambda item: item["path"])
    secrets_expected = sorted({name for item in expected_bindings for name in item["secret_headers"]})
    if (projection.get("header_bindings") != expected_bindings or projection.get("environment") != expected_environment
            or projection.get("complete_files") != expected_files or projection.get("secret_headers") != secrets_expected):
        raise BrokerCredentialUnavailable("provider projection is inconsistent")
    recognizers = [(item["target"]["scheme"], item["target"]["host"], item["target"]["port"], item["source"]["header"], item["source"]["format"]) for item in expected_bindings]
    if len(recognizers) != len(set(recognizers)):
        raise BrokerCredentialUnavailable("provider projection is ambiguous")
    return projection


def load_provider_projection(path: str) -> dict[str, Any]:
    if not isinstance(path, str) or not os.path.isabs(path) or os.path.normpath(path) != path:
        raise BrokerCredentialUnavailable("provider projection path is invalid")
    try:
        descriptor = os.open(path, os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0))
        try:
            info = os.fstat(descriptor)
            if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or stat.S_IMODE(info.st_mode) != 0o600:
                raise BrokerCredentialUnavailable("provider projection file is invalid")
            raw = os.read(descriptor, MAX_PROVIDER_PROJECTION_BYTES + 1)
        finally:
            os.close(descriptor)
    except BrokerCredentialError:
        raise
    except OSError as error:
        raise BrokerCredentialUnavailable("provider projection is unavailable") from error
    if not raw or len(raw) > MAX_PROVIDER_PROJECTION_BYTES:
        raise BrokerCredentialUnavailable("provider projection size is invalid")
    return validate_provider_projection(_decode_json(raw))


def _broker_response(payload: bytes) -> dict[str, Any]:
    response = _decode_json(payload)
    if not isinstance(response, dict) or response.get("schema_version") != BROKER_SCHEMA_VERSION:
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    if response.get("ok") is False:
        error = response.get("error")
        code = error.get("code") if isinstance(error, dict) else None
        if code in {"handle_not_found", "handle_revoked", "handle_binding_mismatch", "invalid_handle", "invalid_binding", "invalid_context", "invalid_project", "invalid_provider", "invalid_revision"}:
            raise BrokerCredentialBindingError("credential handle is invalid")
        raise BrokerCredentialUnavailable("credential broker is unavailable")
    if response.get("ok") is not True:
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    return response


def call_broker(path: str, request: dict[str, Any], timeout: float) -> dict[str, Any]:
    if not isinstance(path, str) or not path.startswith("/") or os.path.normpath(path) != path or "\x00" in path:
        raise BrokerCredentialUnavailable("credential broker socket path is invalid")
    encoded = json.dumps(request, ensure_ascii=True, allow_nan=False, separators=(",", ":"), sort_keys=True).encode() + b"\n"
    if len(encoded) > MAX_BROKER_FRAME_BYTES:
        raise BrokerCredentialUnavailable("credential broker request is too large")
    connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    connection.settimeout(timeout)
    try:
        connection.connect(path)
        connection.sendall(encoded)
        connection.shutdown(socket.SHUT_WR)
        response = bytearray()
        while len(response) <= MAX_BROKER_FRAME_BYTES:
            chunk = connection.recv(8192)
            if not chunk: break
            response.extend(chunk)
    except (OSError, TimeoutError) as error:
        raise BrokerCredentialUnavailable("credential broker is unavailable") from error
    finally:
        connection.close()
    if not response.endswith(b"\n") or response.count(b"\n") != 1 or len(response) > MAX_BROKER_FRAME_BYTES + 1:
        raise BrokerCredentialUnavailable("credential broker response is invalid")
    return _broker_response(bytes(response[:-1]))


@dataclass(frozen=True)
class _HandleCandidate:
    header: str
    source_format: str
    source_scheme: str | None
    handle: str


def _contains_handle_marker(value: str) -> bool:
    for _ in range(3):
        if HANDLE_MARKER in value: return True
        decoded = unquote(value)
        if decoded == value: return False
        value = decoded
    return HANDLE_MARKER in value


def redacted_audit_path(url: str) -> str:
    path = urlsplit(url).path or "/"
    return "/[redacted-auth-handle]" if _contains_handle_marker(path) else path


def _candidate(header: str, value: str) -> _HandleCandidate | None:
    stripped = value.strip(" \t")
    if stripped.startswith(HANDLE_MARKER): return _HandleCandidate(header, "raw", None, stripped)
    pieces = re.split(r"[ \t]+", stripped)
    positions = [index for index, piece in enumerate(pieces) if piece.startswith(HANDLE_MARKER)]
    if not positions: return None
    if len(pieces) == 2 and positions == [1]: return _HandleCandidate(header, pieces[0].lower(), pieces[0], pieces[1])
    return _HandleCandidate(header, "invalid", None, "")


def _find_candidate(request: http.Request, projection: dict[str, Any], scheme: str, host: str, port: int) -> tuple[_HandleCandidate, dict[str, Any]] | None:
    split = urlsplit(request.url)
    if any(_contains_handle_marker(value) for value in (split.path, split.query, split.fragment)):
        raise BrokerCredentialBindingError("credential handle is invalid in request target")
    candidates: list[_HandleCandidate] = []
    for raw_name, raw_value in request.headers.fields:
        name, value = raw_name.decode("latin-1").lower(), raw_value.decode("latin-1")
        if HANDLE_MARKER in name: raise BrokerCredentialBindingError("credential handle is invalid in header name")
        candidate = _candidate(name, value)
        if _contains_handle_marker(value) and candidate is None: raise BrokerCredentialBindingError("credential handle is invalid in header")
        if candidate is not None: candidates.append(candidate)
    if not candidates: return None
    for candidate in candidates:
        request.headers.pop(candidate.header, None)
    if len(candidates) != 1 or HANDLE_PATTERN.fullmatch(candidates[0].handle) is None:
        raise BrokerCredentialBindingError("credential handle is malformed or ambiguous")
    candidate = candidates[0]
    matches = [item for item in projection["header_bindings"] if item["target"] == {"scheme": scheme, "host": host, "port": port} and item["source"] == {"header": candidate.header, "format": candidate.source_format}]
    if len(matches) != 1: raise BrokerCredentialBindingError("credential handle is invalid for target")
    return candidate, matches[0]


def _validate_metadata(response: dict[str, Any], binding: dict[str, Any], include_secret: bool) -> tuple[str, bytes | None]:
    expected = {"schema_version", "ok", "provider", "revision", "target", "source", "destination", "secret_headers"}
    if include_secret: expected.add("secret")
    revision = response.get("revision")
    if (set(response) != expected or response.get("provider") != binding["provider_id"]
            or not isinstance(revision, str) or REVISION_PATTERN.fullmatch(revision) is None
            or response.get("target") != binding["target"] or response.get("source") != binding["source"]
            or response.get("destination") != binding["destination"] or response.get("secret_headers") != binding["secret_headers"]):
        raise BrokerCredentialUnavailable("credential broker returned inconsistent data")
    if not include_secret: return revision, None
    secret_document = _exact(response.get("secret"), {"field", "encoding", "value"})
    encoded = secret_document.get("value")
    if secret_document.get("field") != "primary_secret" or secret_document.get("encoding") != "base64url" or not isinstance(encoded, str) or not encoded or "=" in encoded:
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    try: secret = base64.b64decode(encoded.encode("ascii") + b"=" * (-len(encoded) % 4), altchars=b"-_", validate=True)
    except (UnicodeEncodeError, ValueError) as error: raise BrokerCredentialUnavailable("credential broker returned invalid data") from error
    if not secret or len(secret) > MAX_SECRET_BYTES or any(byte < 0x20 or byte == 0x7f for byte in secret):
        raise BrokerCredentialUnavailable("credential broker returned invalid data")
    return revision, secret


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
    def broker_provider(self) -> str: return self.binding["provider_id"]
    @property
    def secret_headers(self) -> set[str]: return set(DEFAULT_SECRET_HEADERS) | set(self.binding["secret_headers"])

    def apply(self, request: http.Request, selected_profile: str | None) -> str | None:
        if selected_profile is not None: raise BrokerCredentialBindingError("managed credential profiles are retired")
        response = self.broker_call({"schema_version": BROKER_SCHEMA_VERSION, "op": "resolve", "handle": self.candidate.handle,
            "context_id": self.context_id, "project_id": self.project_id, "provider": self.binding["provider_id"],
            "revision": self.revision, "target": self.binding["target"], "source_header": self.binding["source"]["header"],
            "source_format": self.binding["source"]["format"]})
        revision, secret = _validate_metadata(response, self.binding, True)
        if revision != self.revision or secret is None: raise BrokerCredentialUnavailable("credential broker returned inconsistent data")
        for header in set(self.binding["secret_headers"]) | set(CONTROL_HEADERS) | {"proxy-authorization"}: request.headers.pop(header, None)
        value = secret.decode("latin-1")
        output_format = self.binding["destination"]["format"]
        if output_format == "preserve_scheme":
            rendered = value if self.binding["source"]["format"] == "raw" else f"{self.candidate.source_scheme} {value}"
        elif output_format == "raw": rendered = value
        elif output_format == "bearer": rendered = f"Bearer {value}"
        elif output_format == "token": rendered = f"token {value}"
        else: raise BrokerCredentialUnavailable("credential broker binding is invalid")
        request.headers[self.binding["destination"]["header"]] = rendered
        return None


class BrokeredCredentialAdapter:
    name = "brokered"
    def __init__(self, fallback: CredentialAdapter, projection_path: str, socket_path: str, timeout: float,
                 *, projection_loader: Callable[[str], dict[str, Any]] = load_provider_projection,
                 caller: Callable[[str, dict[str, Any], float], dict[str, Any]] = call_broker) -> None:
        self.fallback, self.projection_path, self.socket_path, self.timeout = fallback, projection_path, socket_path, timeout
        self.projection_loader, self.caller = projection_loader, caller

    def prepare(self, request: http.Request, scheme: str, host: str, port: int, context_id: str, project_id: str) -> PreparedCredentialRequest:
        projection = self.projection_loader(self.projection_path)
        selected = _find_candidate(request, projection, scheme, host, port)
        if selected is None:
            return self.fallback.prepare(request, scheme, host, port, context_id, project_id)
        candidate, binding = selected
        for header in binding["secret_headers"]: request.headers.pop(header, None)
        response = self.caller(self.socket_path, {"schema_version": BROKER_SCHEMA_VERSION, "op": "introspect",
            "handle": candidate.handle, "context_id": context_id, "project_id": project_id, "provider": binding["provider_id"],
            "target": binding["target"], "source_header": binding["source"]["header"], "source_format": binding["source"]["format"]}, self.timeout)
        revision, _ = _validate_metadata(response, binding, False)
        return _BrokerRequest(binding, candidate, context_id, project_id, revision,
                              lambda item: self.caller(self.socket_path, item, self.timeout))
