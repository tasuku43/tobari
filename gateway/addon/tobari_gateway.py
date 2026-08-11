"""Tobari's generic HTTP policy enforcement addon for mitmproxy."""

from __future__ import annotations

import ipaddress
import json
import os
import re
import socket
import time
import urllib.error
import urllib.request
import uuid
from pathlib import PurePosixPath
from typing import Any
from urllib.parse import parse_qs, unquote, urlsplit

from mitmproxy import http

from credential_adapters import (
    CONTROL_HEADERS,
    DEFAULT_SECRET_HEADERS,
    PROFILE_HEADER,
    CredentialAdapterError,
    build_credential_adapter,
)
from broker_credentials import (
    BrokerCredentialBindingError,
    BrokerCredentialOutcomeUnknown,
    BrokerCredentialUnavailable,
    BrokeredCredentialAdapter,
    redacted_audit_path,
)
from graphql_request import (
    GraphQLParseLimits,
    GraphQLRequestError,
    ParsedGraphQLRequest,
    parse_graphql_post_request,
    validate_graphql_post_headers,
)

MAX_CREDENTIAL_CONFIG_BYTES = 256 * 1024
MAX_SECRET_BYTES = 64 * 1024
MAX_PRINCIPAL_CONFIG_BYTES = 256 * 1024
PROJECT_ID_PATTERN = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)
_REQUESTED_PROFILE_UNSET = object()
_SECRET_HEADER_MARKERS = (
    "authorization",
    "api-key",
    "apikey",
    "access-token",
    "auth-token",
    "credential",
    "secret",
    "token",
)


class PolicyUnavailable(Exception):
    """OPA did not return one valid decision."""


class CredentialError(Exception):
    """A selected credential could not be injected safely."""


class CredentialBindingError(CredentialError):
    """A credential profile is not authorized for the established project."""


class PrincipalError(Exception):
    """The host-owned project principal could not be established."""


class AuthorityError(Exception):
    """Transparent ingress did not provide one safe HTTP authority."""


class UpstreamAddressError(Exception):
    """The upstream hostname cannot be bound to a safe resolved address."""


def load_project_principals(path: str) -> dict[str, dict[str, str]]:
    try:
        with open(path, "rb") as handle:
            raw = handle.read(MAX_PRINCIPAL_CONFIG_BYTES + 1)
    except OSError as error:
        raise PrincipalError("project principal registry could not be read") from error
    if len(raw) > MAX_PRINCIPAL_CONFIG_BYTES:
        raise PrincipalError("project principal registry is too large")
    try:
        document = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise PrincipalError("project principal registry is invalid") from error
    if not isinstance(document, dict) or document.get("schema_version") != 3:
        raise PrincipalError("project principal registry version is invalid")
    bindings = document.get("bindings")
    if not isinstance(bindings, list):
        raise PrincipalError("project principal bindings are invalid")
    result: dict[str, dict[str, str]] = {}
    projects: set[str] = set()
    gateway_addresses: set[str] = set()
    for binding in bindings:
        if not isinstance(binding, dict) or set(binding) != {
            "project_id", "context_id", "context", "project_root", "workspace_ip",
            "gateway_ip", "network"
        }:
            raise PrincipalError("project principal binding shape is invalid")
        project_id = binding.get("project_id")
        context_id = binding.get("context_id")
        context_name = binding.get("context")
        project_root = binding.get("project_root")
        workspace_ip = binding.get("workspace_ip")
        gateway_ip = binding.get("gateway_ip")
        network = binding.get("network")
        if (
            not isinstance(project_id, str)
            or not PROJECT_ID_PATTERN.fullmatch(project_id)
            or project_id in projects
            or not isinstance(context_id, str)
            or not PROJECT_ID_PATTERN.fullmatch(context_id)
            or not isinstance(context_name, str)
            or re.fullmatch(r"[a-z][a-z0-9-]{0,62}", context_name) is None
            or not isinstance(project_root, str)
            or not project_root.startswith("/")
            or not isinstance(workspace_ip, str)
            or not isinstance(gateway_ip, str)
            or not isinstance(network, str)
            or re.fullmatch(r"[A-Za-z0-9][A-Za-z0-9_.-]{0,127}", network) is None
        ):
            raise PrincipalError("project principal binding is invalid")
        try:
            workspace_address = ipaddress.ip_address(workspace_ip)
            gateway_address = ipaddress.ip_address(gateway_ip)
        except ValueError as error:
            raise PrincipalError("project principal address is invalid") from error
        for address in (workspace_address, gateway_address):
            if (
                address.version != 4
                or
                address.is_loopback
                or address.is_unspecified
                or address.is_multicast
                or address.is_link_local
            ):
                raise PrincipalError("project principal address is unsafe")
        canonical_workspace = str(workspace_address)
        canonical_gateway = str(gateway_address)
        if canonical_workspace == canonical_gateway:
            raise PrincipalError("project principal endpoints must be distinct")
        if (
            canonical_workspace in result
            or canonical_workspace in gateway_addresses
            or canonical_gateway in result
            or canonical_gateway in gateway_addresses
        ):
            raise PrincipalError("project principal addresses are ambiguous")
        projects.add(project_id)
        gateway_addresses.add(canonical_gateway)
        result[canonical_workspace] = {
            "project_id": project_id,
            "context_id": context_id,
            "context": context_name,
            "project_root": project_root,
        }
    return result


def resolve_project_principal(
    flow: http.HTTPFlow, principals: dict[str, dict[str, str]]
) -> dict[str, str]:
    client = getattr(flow, "client_conn", None)
    address = getattr(client, "peername", None)
    if not isinstance(address, (tuple, list)) or not address or not isinstance(address[0], str):
        raise PrincipalError("project principal address is unavailable")
    principal = principals.get(address[0])
    if principal is None:
        raise PrincipalError("project principal is not registered")
    return principal


def resolve_upstream_address(host: str, port: int) -> tuple[str, int]:
    """Resolve and pin one upstream address before mitmproxy connects."""
    try:
        records = socket.getaddrinfo(host, port, type=socket.SOCK_STREAM)
    except OSError as error:
        raise UpstreamAddressError("upstream address could not be resolved") from error

    hostname = host.rstrip(".").lower()
    literal = False
    try:
        ipaddress.ip_address(hostname)
        literal = True
    except ValueError:
        pass

    addresses: list[tuple[str, int]] = []
    seen: set[str] = set()
    for _, _, _, _, sockaddr in records:
        if not sockaddr:
            continue
        candidate = sockaddr[0]
        if candidate in seen:
            continue
        seen.add(candidate)
        try:
            address = ipaddress.ip_address(candidate)
        except ValueError:
            continue
        non_routable = not address.is_global
        single_label_private = "." not in hostname and not literal
        if non_routable and (
            not single_label_private
            or address.is_loopback
            or address.is_link_local
            or address.is_multicast
            or address.is_unspecified
            or address.is_reserved
        ):
            raise UpstreamAddressError("upstream resolved to a non-public address")
        addresses.append((candidate, port))
    if not addresses:
        raise UpstreamAddressError("upstream address resolution returned no usable address")
    return addresses[0]


class Decision:
    """Validated OPA decision compatible with mitmproxy's script loader."""

    __slots__ = ("allow", "reason", "credential_profile", "status_code", "learnable")

    def __init__(
        self,
        allow: bool,
        reason: str,
        credential_profile: str | None,
        status_code: int,
        learnable: bool,
    ) -> None:
        self.allow = allow
        self.reason = reason
        self.credential_profile = credential_profile
        self.status_code = status_code
        self.learnable = learnable


def _positive_int(name: str, default: int, minimum: int, maximum: int) -> int:
    raw = os.getenv(name, str(default))
    try:
        value = int(raw)
    except ValueError as error:
        raise RuntimeError(f"{name} must be an integer") from error
    if value < minimum or value > maximum:
        raise RuntimeError(f"{name} must be between {minimum} and {maximum}")
    return value


def _headers_for_policy(headers: http.Headers, secret_names: set[str]) -> dict[str, str]:
    safe: dict[str, list[str]] = {}
    for name, value in headers.fields:
        decoded_name = name.decode("latin-1").lower()
        if (
            decoded_name in secret_names
            or decoded_name in CONTROL_HEADERS
            or any(marker in decoded_name for marker in _SECRET_HEADER_MARKERS)
        ):
            continue
        safe.setdefault(decoded_name, []).append(value.decode("latin-1"))
    return {name: ", ".join(values) for name, values in sorted(safe.items())}


def request_authority(flow: http.HTTPFlow) -> tuple[str, str, int]:
    """Return the exact authority used by both credential and policy checks."""
    request = flow.request
    scheme = request.scheme.lower()
    client = getattr(flow, "client_conn", None)
    if scheme == "http" and request.port == 443 and getattr(client, "tls_established", False):
        scheme = "https"
    return scheme, request.host.rstrip(".").lower(), request.port


def _canonical_authority(value: str) -> tuple[str, int | None]:
    if (
        not value
        or len(value) > 1024
        or any(ord(character) < 33 or ord(character) == 127 for character in value)
        or any(character in value for character in "/\\?#")
    ):
        raise AuthorityError("transparent HTTP authority is invalid")
    try:
        parsed = urlsplit("//" + value)
        port = parsed.port
    except ValueError as error:
        raise AuthorityError("transparent HTTP authority is invalid") from error
    if parsed.username is not None or parsed.password is not None or not parsed.hostname:
        raise AuthorityError("transparent HTTP authority is invalid")
    host = parsed.hostname.rstrip(".").lower()
    try:
        host = str(ipaddress.ip_address(host))
    except ValueError:
        if (
            len(host) > 253
            or any(
                not label
                or len(label) > 63
                or label.startswith("-")
                or label.endswith("-")
                or re.fullmatch(r"[a-z0-9-]+", label) is None
                for label in host.split(".")
            )
        ):
            raise AuthorityError("transparent HTTP authority is invalid")
    return host, port


def _is_transparent_ingress(flow: http.HTTPFlow) -> bool:
    client = getattr(flow, "client_conn", None)
    mode = getattr(client, "proxy_mode", None)
    return getattr(mode, "type_name", "") == "transparent"


def normalize_ingress_authority(flow: http.HTTPFlow) -> tuple[str, str, int]:
    """Bind transparent traffic to one Host/SNI authority before policy."""
    if not _is_transparent_ingress(flow):
        raise AuthorityError("transparent ingress is required")

    client = getattr(flow, "client_conn", None)
    original = getattr(client, "sockname", None)
    if (
        not isinstance(original, (tuple, list))
        or len(original) < 2
        or not isinstance(original[0], str)
        or not isinstance(original[1], int)
        or isinstance(original[1], bool)
        or original[1] < 1
        or original[1] > 65535
    ):
        raise AuthorityError("transparent original destination is unavailable")
    try:
        original_host = str(ipaddress.ip_address(original[0]))
    except ValueError as error:
        raise AuthorityError("transparent original destination is invalid") from error

    host_header = flow.request.host_header
    if not isinstance(host_header, str):
        raise AuthorityError("transparent HTTP authority is unavailable")
    host, declared_port = _canonical_authority(host_header)
    port = original[1] if declared_port is None else declared_port
    if port != original[1]:
        raise AuthorityError("transparent HTTP authority conflicts with destination port")

    tls_established = bool(getattr(client, "tls_established", False))
    sni = getattr(client, "sni", None)
    if sni is not None:
        if not isinstance(sni, str):
            raise AuthorityError("transparent TLS authority is invalid")
        sni_host, sni_port = _canonical_authority(sni)
        if sni_port is not None or sni_host != host:
            raise AuthorityError("transparent TLS and HTTP authorities conflict")
    elif tls_established:
        try:
            if str(ipaddress.ip_address(host)) != original_host:
                raise AuthorityError("transparent TLS authority is unavailable")
        except ValueError as error:
            raise AuthorityError("transparent TLS authority is unavailable") from error

    scheme = "https" if tls_established else "http"
    flow.request.scheme = scheme
    flow.request.host = host
    flow.request.port = port
    flow.metadata["tobari_transparent_authority"] = (host, port)
    return scheme, host, port


def commit_upstream_authority(flow: http.HTTPFlow) -> None:
    """Replace a transparent synthetic destination only after authorization."""
    authority = flow.metadata.pop("tobari_transparent_authority", None)
    if authority is None:
        return
    if (
        not isinstance(authority, tuple)
        or len(authority) != 2
        or not isinstance(authority[0], str)
        or not isinstance(authority[1], int)
    ):
        raise AuthorityError("transparent upstream authority is invalid")
    server = getattr(flow, "server_conn", None)
    if server is None:
        raise AuthorityError("transparent upstream connection is unavailable")
    server.address = authority


def build_policy_input(
    flow: http.HTTPFlow,
    cluster: str,
    principal: dict[str, str],
    extra_secret_names: set[str],
    requested_profile: str | None | object = _REQUESTED_PROFILE_UNSET,
    broker_provider: str | None = None,
    graphql: ParsedGraphQLRequest | None = None,
) -> dict[str, Any]:
    request = flow.request
    split = urlsplit(request.url)
    scheme, host, port = request_authority(flow)
    path = split.path or "/"
    if requested_profile is _REQUESTED_PROFILE_UNSET:
        requested_profile = request.headers.get(PROFILE_HEADER)
    secret_names = set(DEFAULT_SECRET_HEADERS) | extra_secret_names
    policy_input = {
        "schema_version": 5,
        "principal": {
            "cluster": cluster,
            "context_id": principal["context_id"],
            "project_id": principal["project_id"],
        },
        "request": {
            "authority": {
                "scheme": scheme,
                "host": host,
                "port": port,
            },
            "method": request.method.upper(),
            "path": {
                "raw": path,
                "segments": [unquote(segment) for segment in path.split("/") if segment],
            },
            "query": parse_qs(split.query, keep_blank_values=True),
            "headers": _headers_for_policy(request.headers, secret_names),
        },
        "authorization": {
            "requested_profile": requested_profile,
            "broker_provider": broker_provider,
        },
    }
    if graphql is not None:
        policy_input["request"]["graphql"] = {
            "operation_type": graphql.operation_type,
            "root_fields": list(graphql.root_fields),
        }
    return policy_input


def _parse_decision(document: Any) -> Decision:
    if not isinstance(document, dict) or not isinstance(document.get("result"), dict):
        raise PolicyUnavailable("OPA response has no result object")
    result = document["result"]
    allow = result.get("allow")
    reason = result.get("reason")
    profile = result.get("credential_profile")
    if not isinstance(allow, bool) or not isinstance(reason, str) or not reason:
        raise PolicyUnavailable("OPA result has invalid allow or reason")
    if profile is not None and (not isinstance(profile, str) or not profile):
        raise PolicyUnavailable("OPA result has invalid credential_profile")
    status = result.get("status_code")
    if not isinstance(status, int) or status != 403:
        raise PolicyUnavailable("OPA result has invalid status_code")
    learnable = result.get("learnable")
    if not isinstance(learnable, bool):
        raise PolicyUnavailable("OPA result has invalid learnable")
    if allow and learnable:
        raise PolicyUnavailable("OPA result cannot allow and be learnable")
    return Decision(
        allow=allow,
        reason=reason,
        credential_profile=profile,
        status_code=status,
        learnable=learnable,
    )


def query_opa(url: str, policy_input: dict[str, Any], timeout: float) -> Decision:
    payload = json.dumps({"input": policy_input}, separators=(",", ":")).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=payload,
        headers={"content-type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            if response.status != 200:
                raise PolicyUnavailable("OPA returned a non-success status")
            body = response.read(1024 * 1024 + 1)
    except (OSError, urllib.error.URLError, TimeoutError) as error:
        raise PolicyUnavailable("OPA request failed") from error
    if len(body) > 1024 * 1024:
        raise PolicyUnavailable("OPA response exceeded the size limit")
    try:
        return _parse_decision(json.loads(body))
    except (json.JSONDecodeError, UnicodeDecodeError) as error:
        raise PolicyUnavailable("OPA response was not valid JSON") from error


def load_credential_config(path: str) -> dict[str, Any]:
    try:
        with open(path, "rb") as handle:
            raw = handle.read(MAX_CREDENTIAL_CONFIG_BYTES + 1)
    except OSError as error:
        raise CredentialError("credential configuration could not be read") from error
    if len(raw) > MAX_CREDENTIAL_CONFIG_BYTES:
        raise CredentialError("credential configuration is too large")
    try:
        document = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise CredentialError("credential configuration is invalid") from error
    if not isinstance(document, dict) or document.get("version") != "v2":
        raise CredentialError("credential configuration version is invalid")
    contexts = document.get("contexts")
    if not isinstance(contexts, dict):
        raise CredentialError("credential Contexts are invalid")
    for context_id, context in contexts.items():
        if not isinstance(context_id, str) or not PROJECT_ID_PATTERN.fullmatch(context_id):
            raise CredentialError("credential Context identity is invalid")
        if not isinstance(context, dict) or not isinstance(context.get("profiles"), dict):
            raise CredentialError("credential Context is invalid")
        endpoints = context.get("graphql_endpoints", [])
        if not isinstance(endpoints, list):
            raise CredentialError("GraphQL endpoint configuration is invalid")
        seen_endpoints: set[tuple[str, str, int, str]] = set()
        for endpoint in endpoints:
            if not isinstance(endpoint, dict) or set(endpoint) != {
                "scheme", "host", "port", "path"
            }:
                raise CredentialError("GraphQL endpoint configuration is invalid")
            scheme = endpoint.get("scheme")
            host = endpoint.get("host")
            port = endpoint.get("port")
            path = endpoint.get("path")
            if (
                scheme not in {"http", "https"}
                or not isinstance(host, str)
                or not host
                or host != host.lower()
                or host.endswith(".")
                or len(host) > 253
                or any(
                    not label
                    or len(label) > 63
                    or label.startswith("-")
                    or label.endswith("-")
                    or re.fullmatch(r"[a-z0-9-]+", label) is None
                    for label in host.split(".")
                )
                or not isinstance(port, int)
                or isinstance(port, bool)
                or port < 1
                or port > 65535
                or not isinstance(path, str)
                or not path.startswith("/")
                or len(path) > 4096
                or any(
                    ord(character) < 32
                    or ord(character) == 127
                    or character in {"\u2028", "\u2029"}
                    for character in path
                )
                or any(character in path for character in "%\\?#")
                or "//" in path
                or (
                    path != "/"
                    and any(
                        segment in {"", ".", ".."}
                        for segment in path.strip("/").split("/")
                    )
                )
            ):
                raise CredentialError("GraphQL endpoint configuration is invalid")
            identity = (scheme, host, port, path)
            if identity in seen_endpoints:
                raise CredentialError("GraphQL endpoint configuration is ambiguous")
            seen_endpoints.add(identity)
        for name, profile in context["profiles"].items():
            if not isinstance(name, str) or not name or not isinstance(profile, dict):
                raise CredentialError("credential profiles are invalid")
            projects = profile.get("projects")
            if (
                not isinstance(projects, list)
                or any(
                    not isinstance(project, str)
                    or PROJECT_ID_PATTERN.fullmatch(project) is None
                    for project in projects
                )
                or len(projects) != len(set(projects))
            ):
                raise CredentialError("credential profile project bindings are invalid")
    return document


def graphql_endpoint_declared(
    config: dict[str, Any], context_id: str, scheme: str, host: str, port: int, path: str
) -> bool:
    """Match only host-owned exact endpoint declarations for this Context."""

    context = config.get("contexts", {}).get(context_id)
    if not isinstance(context, dict):
        raise CredentialBindingError("credential Context is not established")
    return any(
        endpoint == {"scheme": scheme, "host": host, "port": port, "path": path}
        for endpoint in context.get("graphql_endpoints", [])
    )


def configured_secret_headers(config: dict[str, Any]) -> set[str]:
    names: set[str] = set()
    for context in config.get("contexts", {}).values():
        for profile in context.get("profiles", {}).values():
            if isinstance(profile, dict) and isinstance(profile.get("header"), str):
                names.add(profile["header"].lower())
    return names


def _profile_project_binding(
    config: dict[str, Any], name: str, context_id: str, project_id: str
) -> dict[str, Any]:
    context = config.get("contexts", {}).get(context_id)
    if not isinstance(context, dict):
        raise CredentialBindingError("credential Context is not established")
    profile = context.get("profiles", {}).get(name)
    if not isinstance(profile, dict):
        raise CredentialError("OPA selected an unknown credential profile")
    projects = profile.get("projects")
    if (
        not isinstance(project_id, str)
        or PROJECT_ID_PATTERN.fullmatch(project_id) is None
        or not isinstance(projects, list)
        or project_id not in projects
    ):
        raise CredentialBindingError("credential profile is not bound to the project")
    return profile


def _validated_profile(
    config: dict[str, Any], name: str, host: str, context_id: str, project_id: str
) -> dict[str, Any]:
    profile = _profile_project_binding(config, name, context_id, project_id)
    profile_type = profile.get("type")
    hosts = profile.get("hosts")
    secret_file = profile.get("secret_file")
    if profile_type not in {"bearer", "header"}:
        raise CredentialError("credential profile type is invalid")
    if not isinstance(hosts, list) or not hosts or any(not isinstance(item, str) for item in hosts):
        raise CredentialError("credential profile hosts are invalid")
    if host not in {item.rstrip(".").lower() for item in hosts}:
        raise CredentialError("credential profile is not bound to the request host")
    if not isinstance(secret_file, str):
        raise CredentialError("credential secret path is invalid")
    secret_path = PurePosixPath(secret_file)
    if (
        secret_path.parent != PurePosixPath("/run/tobari/credentials") / context_id
        or secret_path.name in {"", ".", ".."}
    ):
        raise CredentialError("credential secret path is invalid")
    header = profile.get("header", "authorization" if profile_type == "bearer" else None)
    if not isinstance(header, str) or not header or header.lower() in {"host", "content-length"}:
        raise CredentialError("credential header is invalid")
    return {**profile, "header": header}


def inject_credential(
    request: http.Request,
    config: dict[str, Any],
    profile_name: str,
    host: str,
    context_id: str,
    project_id: str,
) -> None:
    profile = _validated_profile(config, profile_name, host, context_id, project_id)
    try:
        with open(profile["secret_file"], "rb") as handle:
            raw = handle.read(MAX_SECRET_BYTES + 1)
    except OSError as error:
        raise CredentialError("credential secret could not be read") from error
    if not raw or len(raw) > MAX_SECRET_BYTES or b"\x00" in raw:
        raise CredentialError("credential secret is invalid")
    value = raw.rstrip(b"\r\n").decode("utf-8", errors="strict")
    if not value or "\r" in value or "\n" in value:
        raise CredentialError("credential secret is invalid")
    header = profile["header"]
    request.headers[header] = f"Bearer {value}" if profile["type"] == "bearer" else value


def _deny(flow: http.HTTPFlow, status: int, code: str) -> None:
    body = json.dumps({"error": code}, separators=(",", ":")).encode("utf-8")
    flow.response = http.Response.make(status, body, {"content-type": "application/json"})


def _policy_denied(
    flow: http.HTTPFlow,
    status: int,
    learnable: bool,
    graphql: ParsedGraphQLRequest | None = None,
) -> None:
    path = urlsplit(flow.request.url).path or "/"
    review_available = bool(learnable)
    review = {
        "available": review_available,
        "command": "tobari policy review" if review_available else None,
        "automatic_retry": False,
        "retry_after_review": review_available,
    }
    message = (
        "Tobari blocked this network request because it is outside the current execution boundary. "
        "Run `tobari cluster denials` on the trusted host for read-only diagnostics."
    )
    if review_available:
        message = (
            "Tobari blocked this network request because it is outside the current execution boundary. "
            "Keep the current Workspace and agent session running. In a separate trusted-host terminal, "
            "run `tobari policy review`. After Apply succeeds, retry this request in the same Workspace. "
            "Tobari does not approve or retry automatically."
        )
    document = {
        "error": "policy_denied",
        "message": message,
        "tobari": {
            "schema_version": 1,
            "event": "permission_review_available"
            if review_available
            else "permission_review_unavailable",
            "run_on": "host",
            "review": review,
            "request": {
                "host": flow.request.host.rstrip(".").lower(),
                "port": flow.request.port,
                "method": flow.request.method.upper(),
                "path": path,
            },
        },
    }
    if graphql is not None:
        document["tobari"]["schema_version"] = 2
        document["tobari"]["request"]["protocol"] = "graphql"
        document["tobari"]["request"]["graphql_operation_type"] = graphql.operation_type
        document["tobari"]["request"]["graphql_root_fields"] = list(graphql.root_fields)
    body = json.dumps(document, separators=(",", ":")).encode("utf-8")
    flow.response = http.Response.make(status, body, {"content-type": "application/json"})


def _audit(**fields: Any) -> None:
    fields["schema_version"] = 3 if fields.get("protocol") == "graphql" else 2
    print(json.dumps(fields, separators=(",", ":"), sort_keys=True), flush=True)


def _request_header_pairs(request: http.Request) -> list[tuple[str, str]]:
    return [
        (name.decode("latin-1"), value.decode("latin-1"))
        for name, value in request.headers.fields
    ]


def _graphql_audit_events(base: dict[str, Any], parsed: ParsedGraphQLRequest) -> list[dict[str, Any]]:
    return [
        {
            **base,
            "protocol": "graphql",
            "graphql_operation_type": parsed.operation_type,
            "graphql_root_field": root_field,
        }
        for root_field in parsed.root_fields
    ]


class TobariGateway:
    def __init__(self) -> None:
        self.opa_url = os.getenv(
            "TOBARI_OPA_URL",
            "http://opa:8181/v1/data/tobari/http/decision",
        )
        self.cluster = os.getenv("TOBARI_CLUSTER", "default")
        self.opa_timeout = float(
            _positive_int("TOBARI_OPA_TIMEOUT_SECONDS", 2, 1, 10)
        )
        self.credential_path = os.getenv(
            "TOBARI_CREDENTIAL_CONFIG",
            "/run/tobari/config/credentials.json",
        )
        # The aggregate projection is immutable for the Gateway container's
        # lifetime. Load trusted endpoint declarations once, never from caller
        # headers and never after body bytes have arrived.
        self.graphql_config = (
            load_credential_config(self.credential_path)
            if os.path.exists(self.credential_path)
            else None
        )
        self.credential_adapter_name = os.getenv(
            "TOBARI_CREDENTIAL_ADAPTER", "passthrough"
        )
        base_credential_adapter = build_credential_adapter(
            self.credential_adapter_name,
            credential_path=self.credential_path,
            # Resolve these callbacks when the request runs so the adapter
            # boundary remains observable/testable without duplicating the
            # managed implementation in the Gateway lifecycle.
            load_config=lambda path: load_credential_config(path),
            configured_secret_headers=lambda config: configured_secret_headers(config),
            profile_binding=lambda config, name, context, project: _profile_project_binding(
                config, name, context, project
            ),
            injector=lambda request, config, name, host, context, project: inject_credential(
                request, config, name, host, context, project
            ),
        )
        self.auth_provider_projection_path = os.getenv(
            "TOBARI_AUTH_PROVIDER_PROJECTION", ""
        )
        self.auth_broker_socket = os.getenv(
            "TOBARI_AUTH_BROKER_SOCKET",
            "/run/tobari-auth/runtime/broker.sock",
        )
        self.auth_broker_timeout = float(
            # The broker may wait for the companion's bounded 60-second
            # terminal refresh result. Keep this outer socket deadline larger
            # so the Gateway never manufactures an earlier unknown outcome.
            _positive_int("TOBARI_AUTH_BROKER_TIMEOUT_SECONDS", 70, 70, 90)
        )
        if self.auth_provider_projection_path:
            self.credential_adapter = BrokeredCredentialAdapter(
                base_credential_adapter,
                self.auth_provider_projection_path,
                self.auth_broker_socket,
                self.auth_broker_timeout,
            )
        else:
            self.credential_adapter = base_credential_adapter
        self.principal_path = os.getenv(
            "TOBARI_PRINCIPAL_REGISTRY",
            "/run/tobari/principal-registry/principals.json",
        )

    def server_connect(self, data: Any) -> None:
        address = data.server.address
        if not address:
            data.server.error = "upstream address is missing"
            return
        try:
            data.server.address = resolve_upstream_address(address[0], address[1])
        except UpstreamAddressError as error:
            data.server.error = str(error)

    def requestheaders(self, flow: http.HTTPFlow) -> None:
        started = time.monotonic()
        request_id = uuid.uuid4().hex
        scheme, host, port = "", "", 0
        profile_name: str | None = None
        project_id: str | None = None
        context_id: str | None = None
        context_name: str | None = None
        project_root: str | None = None
        upstream_status: int | None = None
        decision_name = "deny"
        reason = "gateway rejected request"
        learnable = False
        audit_deferred = False
        audit_valid = False
        request_path = "/"
        audit_path = "/"
        try:
            scheme, host, port = normalize_ingress_authority(flow)
            request_path = urlsplit(flow.request.url).path or "/"
            audit_path = redacted_audit_path(flow.request.url)
            audit_valid = True
            principal = resolve_project_principal(
                flow, load_project_principals(self.principal_path)
            )
            project_id = principal["project_id"]
            context_id = principal["context_id"]
            context_name = principal["context"]
            project_root = principal["project_root"]
            credential_request = self.credential_adapter.prepare(
                flow.request, scheme, host, port, context_id, project_id
            )
            profile_name = credential_request.requested_profile
            if graphql_endpoint_declared(
                self.graphql_config
                if self.graphql_config is not None
                else load_credential_config(self.credential_path),
                context_id,
                scheme,
                host,
                port,
                request_path,
            ):
                validate_graphql_post_headers(
                    flow.request.method.upper(),
                    _request_header_pairs(flow.request),
                    limits=GraphQLParseLimits(),
                )
                flow.request.stream = False
                audit_deferred = True
                flow.metadata["tobari_graphql_pending"] = {
                    "started": started,
                    "request_id": request_id,
                    "scheme": scheme,
                    "host": host,
                    "port": port,
                    "audit_path": audit_path,
                    "principal": principal,
                    "credential_request": credential_request,
                }
                return
            policy_input = build_policy_input(
                flow,
                self.cluster,
                principal,
                credential_request.secret_headers,
                profile_name,
                credential_request.broker_provider,
            )
            decision = query_opa(self.opa_url, policy_input, self.opa_timeout)
            reason = decision.reason
            profile_name = decision.credential_profile
            learnable = decision.learnable
            if not decision.allow:
                _policy_denied(flow, decision.status_code, learnable)
                upstream_status = decision.status_code
                return
            profile_name = credential_request.apply(
                flow.request, decision.credential_profile
            )
            decision_name = "allow"
            flow.metadata["tobari_audit"] = {
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "request_id": request_id,
                "cluster": self.cluster,
                "project_id": project_id,
                "context_id": context_id,
                "context": context_name,
                "project_root": project_root,
                "host": host,
                "port": port,
                "method": flow.request.method.upper(),
                "path": audit_path,
                "decision": decision_name,
                "reason": reason,
                "credential_profile": profile_name,
                "learnable": learnable,
                "started": started,
            }
            if bool(getattr(credential_request, "deferred", False)):
                # AWS SigV4 needs a hash of the complete, bounded request body.
                # Policy has already allowed the ordinary HTTP effect, but the
                # request remains buffered and cannot reach upstream until the
                # broker returns final signed headers from request().
                flow.metadata["tobari_deferred_credential"] = credential_request
                flow.request.stream = False
            else:
                commit_upstream_authority(flow)
                # Authorization is complete before mitmproxy forwards any body bytes.
                # The body is deliberately opaque to policy and is never retained here.
                flow.request.stream = True
        except GraphQLRequestError as error:
            reason = str(error)
            _deny(flow, 400, error.code)
            upstream_status = 400
        except PolicyUnavailable as error:
            reason = str(error)
            _deny(flow, 503, "policy_unavailable")
            upstream_status = 503
        except CredentialBindingError as error:
            reason = str(error)
            _deny(flow, 403, "credential_profile_not_bound")
            upstream_status = 403
        except BrokerCredentialOutcomeUnknown as error:
            reason = str(error)
            _deny(flow, 409, "credential_refresh_outcome_unknown")
            upstream_status = 409
        except BrokerCredentialBindingError as error:
            reason = str(error)
            _deny(flow, 403, "credential_handle_invalid")
            upstream_status = 403
        except BrokerCredentialUnavailable as error:
            reason = str(error)
            _deny(flow, 503, "credential_broker_unavailable")
            upstream_status = 503
        except (CredentialAdapterError, CredentialError) as error:
            reason = str(error)
            _deny(flow, 503, "credential_unavailable")
            upstream_status = 503
        except PrincipalError as error:
            reason = str(error)
            _deny(flow, 403, "project_principal_unavailable")
            upstream_status = 403
        except AuthorityError as error:
            reason = str(error)
            _deny(flow, 400, "request_authority_invalid")
            upstream_status = 400
        except (RuntimeError, UnicodeError):
            reason = "credential processing failed"
            _deny(flow, 503, "credential_unavailable")
            upstream_status = 503
        except Exception:
            reason = "gateway error"
            _deny(flow, 502, "gateway_error")
            upstream_status = 502
        finally:
            if decision_name != "allow" and not audit_deferred and audit_valid:
                _audit(
                    timestamp=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                    request_id=request_id,
                    cluster=self.cluster,
                    project_id=project_id,
                    context_id=context_id,
                    context=context_name,
                    project_root=project_root,
                    host=host,
                    port=port,
                    method=flow.request.method.upper(),
                    path=audit_path,
                    decision=decision_name,
                    reason=reason,
                    credential_profile=profile_name,
                    learnable=learnable,
                    upstream_status=upstream_status,
                    duration_ms=int((time.monotonic() - started) * 1000),
                )

    def _complete_graphql_request(
        self, flow: http.HTTPFlow, pending: dict[str, Any]
    ) -> None:
        started = pending["started"]
        request_id = pending["request_id"]
        principal = pending["principal"]
        credential_request = pending["credential_request"]
        host = pending["host"]
        port = pending["port"]
        audit_path = pending["audit_path"]
        parsed: ParsedGraphQLRequest | None = None
        profile_name = credential_request.requested_profile

        def audit_failure(
            status: int, code: str, reason: str, learnable: bool = False
        ) -> None:
            base = {
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "request_id": request_id,
                "cluster": self.cluster,
                "project_id": principal["project_id"],
                "context_id": principal["context_id"],
                "context": principal["context"],
                "project_root": principal["project_root"],
                "host": host,
                "port": port,
                "method": flow.request.method.upper(),
                "path": audit_path,
                "decision": "deny",
                "reason": reason,
                "credential_profile": profile_name,
                "learnable": learnable,
                "upstream_status": status,
                "duration_ms": int((time.monotonic() - started) * 1000),
            }
            events = _graphql_audit_events(base, parsed) if parsed is not None else [base]
            for event in events:
                _audit(**event)
            if code == "policy_denied" and parsed is not None:
                _policy_denied(flow, status, learnable, parsed)
            else:
                _deny(flow, status, code)

        try:
            body = flow.request.raw_content
            if not isinstance(body, bytes):
                raise GraphQLRequestError(
                    "invalid_body", "GraphQL request body must be bytes"
                )
            parsed = parse_graphql_post_request(
                method=flow.request.method.upper(),
                headers=_request_header_pairs(flow.request),
                body=body,
            )
            policy_input = build_policy_input(
                flow,
                self.cluster,
                principal,
                credential_request.secret_headers,
                profile_name,
                credential_request.broker_provider,
                parsed,
            )
            decision = query_opa(self.opa_url, policy_input, self.opa_timeout)
            profile_name = decision.credential_profile
            if not decision.allow:
                audit_failure(
                    decision.status_code,
                    "policy_denied",
                    decision.reason,
                    decision.learnable,
                )
                return
            profile_name = credential_request.apply(
                flow.request, decision.credential_profile
            )
            if bool(getattr(credential_request, "deferred", False)):
                apply_body = getattr(credential_request, "apply_body", None)
                if not callable(apply_body):
                    raise BrokerCredentialUnavailable(
                        "deferred credential contract is unavailable"
                    )
                apply_body(flow.request)
            commit_upstream_authority(flow)
            base = {
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "request_id": request_id,
                "cluster": self.cluster,
                "project_id": principal["project_id"],
                "context_id": principal["context_id"],
                "context": principal["context"],
                "project_root": principal["project_root"],
                "host": host,
                "port": port,
                "method": flow.request.method.upper(),
                "path": audit_path,
                "decision": "allow",
                "reason": decision.reason,
                "credential_profile": profile_name,
                "learnable": False,
                "started": started,
            }
            flow.metadata["tobari_graphql_audits"] = _graphql_audit_events(base, parsed)
        except GraphQLRequestError as error:
            audit_failure(400, error.code, str(error))
        except PolicyUnavailable as error:
            audit_failure(503, "policy_unavailable", str(error))
        except CredentialBindingError as error:
            audit_failure(403, "credential_profile_not_bound", str(error))
        except BrokerCredentialOutcomeUnknown as error:
            audit_failure(409, "credential_refresh_outcome_unknown", str(error))
        except BrokerCredentialBindingError as error:
            audit_failure(403, "credential_handle_invalid", str(error))
        except BrokerCredentialUnavailable as error:
            audit_failure(503, "credential_broker_unavailable", str(error))
        except (CredentialAdapterError, CredentialError) as error:
            audit_failure(503, "credential_unavailable", str(error))
        except AuthorityError as error:
            audit_failure(400, "request_authority_invalid", str(error))
        except (RuntimeError, UnicodeError, ValueError):
            audit_failure(503, "credential_unavailable", "credential processing failed")
        except Exception:
            audit_failure(502, "gateway_error", "gateway error")

    def request(self, flow: http.HTTPFlow) -> None:
        graphql_pending = flow.metadata.pop("tobari_graphql_pending", None)
        if isinstance(graphql_pending, dict):
            self._complete_graphql_request(flow, graphql_pending)
            return
        pending = flow.metadata.pop("tobari_deferred_credential", None)
        if pending is None:
            return
        event = flow.metadata.get("tobari_audit")

        def deny(status: int, code: str, reason: str) -> None:
            if isinstance(event, dict):
                event["decision"] = "deny"
                event["reason"] = reason
            _deny(flow, status, code)

        try:
            apply_body = getattr(pending, "apply_body", None)
            if not callable(apply_body):
                raise BrokerCredentialUnavailable(
                    "deferred credential contract is unavailable"
                )
            apply_body(flow.request)
            commit_upstream_authority(flow)
        except BrokerCredentialOutcomeUnknown as error:
            deny(409, "credential_refresh_outcome_unknown", str(error))
        except BrokerCredentialBindingError as error:
            deny(403, "broker_signing_request_invalid", str(error))
        except BrokerCredentialUnavailable as error:
            deny(503, "credential_broker_unavailable", str(error))
        except CredentialAdapterError as error:
            deny(503, "credential_unavailable", str(error))
        except AuthorityError as error:
            deny(400, "request_authority_invalid", str(error))
        except (RuntimeError, UnicodeError, ValueError):
            deny(503, "credential_unavailable", "credential processing failed")
        except Exception:
            deny(502, "gateway_error", "gateway error")

    def responseheaders(self, flow: http.HTTPFlow) -> None:
        if not isinstance(flow.metadata.get("tobari_audit"), dict) and not isinstance(
            flow.metadata.get("tobari_graphql_audits"), list
        ):
            return
        # Stream only responses belonging to an authorized upstream request.
        flow.response.stream = True

    def response(self, flow: http.HTTPFlow) -> None:
        graphql_events = flow.metadata.pop("tobari_graphql_audits", None)
        if isinstance(graphql_events, list):
            for event in graphql_events:
                started = event.pop("started")
                _audit(
                    **event,
                    upstream_status=flow.response.status_code,
                    duration_ms=int((time.monotonic() - started) * 1000),
                )
            return
        event = flow.metadata.pop("tobari_audit", None)
        if not isinstance(event, dict):
            return
        started = event.pop("started")
        _audit(
            **event,
            upstream_status=flow.response.status_code,
            duration_ms=int((time.monotonic() - started) * 1000),
        )

    def error(self, flow: http.HTTPFlow) -> None:
        graphql_events = flow.metadata.pop("tobari_graphql_audits", None)
        if isinstance(graphql_events, list):
            for event in graphql_events:
                started = event.pop("started")
                event["decision"] = "upstream_error"
                event["reason"] = "upstream request failed"
                _audit(
                    **event,
                    upstream_status=None,
                    duration_ms=int((time.monotonic() - started) * 1000),
                )
            return
        event = flow.metadata.pop("tobari_audit", None)
        if not isinstance(event, dict):
            return
        started = event.pop("started")
        event["decision"] = "upstream_error"
        event["reason"] = "upstream request failed"
        _audit(
            **event,
            upstream_status=None,
            duration_ms=int((time.monotonic() - started) * 1000),
        )


addons = [TobariGateway()]
