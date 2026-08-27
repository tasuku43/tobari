"""Tobari's generic HTTP policy enforcement addon for mitmproxy."""

from __future__ import annotations

import ipaddress
import hashlib
import calendar
import json
import os
import re
import secrets
import socket
import stat
import threading
import time
import urllib.error
import urllib.request
import uuid
from datetime import datetime, timezone
from typing import Any
from urllib.parse import unquote, urlsplit

from mitmproxy import http

from aws_request import (
    AWSRequestError,
    ParsedAWSRequest,
    PendingAWSQueryRequest,
    classify_aws_request_headers,
    parse_aws_query_request,
)
from credential_adapters import CredentialAdapterError, PassthroughCredentialAdapter
from graphql_request import (
    GraphQLParseLimits,
    GraphQLRequestError,
    ParsedGraphQLRequest,
    parse_graphql_request,
    validate_graphql_headers,
)
from mcp_request import (
    MCPRequestError,
    ParsedMCPRequest,
    parse_mcp_post_request,
    validate_mcp_post_headers,
)
from kubernetes_request import (
    KubernetesRequestError,
    ParsedKubernetesRequest,
    parse_kubernetes_request,
)
from git_request import GitRequestError, ParsedGitRequest, classify_git_request
from oci_request import OCIRequestError, ParsedOCIRequest, parse_oci_request
from validated_file import StatIdentityCache, ValidatedFileError

MAX_GATEWAY_CONFIG_BYTES = 256 * 1024
MAX_PRINCIPAL_CONFIG_BYTES = 256 * 1024
MAX_HOST_LOOPBACK_CONFIG_BYTES = 512 * 1024
MAX_PERMISSION_SESSION_BYTES = 256 * 1024
MAX_PERMISSION_WAIT_REQUEST_BYTES = 4 * 1024
PERMISSION_SESSION_LEASE_SECONDS = 30
PERMISSION_WAIT_LEASE_SECONDS = 15 * 60
PERMISSION_INGESTION_UNIX = "unix"
PERMISSION_INGESTION_LOOPBACK_TCP = "loopback_tcp"
PERMISSION_INGESTION_GATEWAY_HOST = "host.docker.internal"
HOST_LOOPBACK_HOSTNAME = "host.tobari.internal"
RETIRED_HOST_LOOPBACK_HOSTNAME = "host.tobari.test"
HOST_LOOPBACK_REGISTRY_SCHEMA = 2
ECH_EXTENSION_TYPES = frozenset({0xFE0D, 0xFFCE})
PROJECT_ID_PATTERN = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)
class PolicyUnavailable(Exception):
    """OPA did not return one valid decision."""


class CredentialError(Exception):
    """A selected credential could not be injected safely."""


class SemanticClassificationError(ValueError):
    """Classified traffic did not select one unique semantic module."""

    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


_SEMANTIC_MODULE_PARENT = {
    "protocols.http.generic": None,
    "protocols.http.graphql": "protocols.http.generic",
    "protocols.http.mcp": "protocols.http.generic",
    "providers.aws": "protocols.http.generic",
    "providers.kubernetes": "protocols.http.generic",
    "protocols.http.git": "protocols.http.generic",
    "protocols.http.oci": "protocols.http.generic",
}


def _semantic_module_refines(candidate: str, ancestor: str) -> bool:
    current = _SEMANTIC_MODULE_PARENT.get(candidate)
    while current is not None:
        if current == ancestor:
            return True
        current = _SEMANTIC_MODULE_PARENT.get(current)
    return False


def select_semantic_module_claims(claims: list[str]) -> str | None:
    """Select one unique most-specific claim; order is never authority."""

    unique = set(claims)
    if any(claim not in _SEMANTIC_MODULE_PARENT for claim in unique):
        raise SemanticClassificationError(
            "semantic_classification_invalid", "semantic module claim is unknown"
        )
    most_specific = sorted(
        claim
        for claim in unique
        if not any(
            other != claim and _semantic_module_refines(other, claim)
            for other in unique
        )
    )
    if not most_specific:
        return None
    if len(most_specific) != 1:
        raise SemanticClassificationError(
            "semantic_classification_ambiguous",
            "request matches more than one semantic module",
        )
    return most_specific[0]


# Standard images do not contain the experimental Broker module. Distinct
# inactive exception types keep generic adapter failures out of the
# Broker-specific handlers until an experimental projection explicitly loads
# and replaces them with the reviewed Broker exceptions.
class _InactiveBrokerAuthenticationRequired(CredentialAdapterError):
    pass


class _InactiveBrokerCredentialBindingError(CredentialAdapterError):
    pass


class _InactiveBrokerCredentialOutcomeUnknown(CredentialAdapterError):
    pass


class _InactiveBrokerCredentialUnavailable(CredentialAdapterError):
    pass


BrokerAuthenticationRequired = _InactiveBrokerAuthenticationRequired
BrokerCredentialBindingError = _InactiveBrokerCredentialBindingError
BrokerCredentialOutcomeUnknown = _InactiveBrokerCredentialOutcomeUnknown
BrokerCredentialUnavailable = _InactiveBrokerCredentialUnavailable


def _experimental_broker_adapter(
    fallback: PassthroughCredentialAdapter,
    projection_path: str,
    socket_path: str,
    timeout: float,
) -> Any:
    import broker_credentials as broker

    return broker.BrokeredCredentialAdapter(
        fallback, projection_path, socket_path, timeout
    )


def _credential_error_response(
    error: CredentialAdapterError, binding_code: str
) -> tuple[int, str]:
    error_name = error.__class__.__name__.removeprefix("_Inactive")
    if error_name == "BrokerCredentialOutcomeUnknown":
        return 409, "credential_refresh_outcome_unknown"
    if error_name == "BrokerAuthenticationRequired":
        return 403, "broker_auth_required"
    if error_name == "BrokerCredentialBindingError":
        return 403, binding_code
    if error_name == "BrokerCredentialUnavailable":
        return 503, "credential_broker_unavailable"
    return 503, "credential_unavailable"


def redacted_audit_path(url: str) -> str:
    path = urlsplit(url).path or "/"
    if "tobari-h" in unquote(path):
        return "/[redacted-auth-handle]"
    return path


class PrincipalError(Exception):
    """The host-owned project principal could not be established."""


class AuthorityError(Exception):
    """Transparent ingress did not provide one safe HTTP authority."""


class UpstreamAddressError(Exception):
    """The upstream hostname cannot be bound to a safe resolved address."""


class HostLoopbackError(Exception):
    """The host-owned attachment route or grant registry is invalid."""


class PermissionSessionError(Exception):
    """The host-owned interactive attachment session is unavailable."""


def _parse_project_principals(raw: bytes) -> dict[str, dict[str, str]]:
    try:
        document = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise PrincipalError("project principal registry is invalid") from error
    if not isinstance(document, dict) or document.get("schema_version") != 1:
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
            "_frozen_principal_fingerprint": hashlib.sha256(
                "\x00".join((
                    "tobari-frozen-principal-v1", project_id, context_id,
                    context_name, project_root, canonical_workspace,
                    canonical_gateway, network,
                )).encode()
            ).hexdigest(),
        }
    return result


class PrincipalRegistrySource:
    """Return the current validated registry without stale fallback."""

    def __init__(self, path: str) -> None:
        self._cache = StatIdentityCache(
            path,
            MAX_PRINCIPAL_CONFIG_BYTES,
            _parse_project_principals,
        )

    def load(self) -> dict[str, dict[str, str]]:
        try:
            return self._cache.load()
        except PrincipalError:
            raise
        except ValidatedFileError as error:
            messages = {
                "path_invalid": "project principal registry path is invalid",
                "file_invalid": "project principal registry file is invalid",
                "size_invalid": "project principal registry is too large",
                "changed": "project principal registry changed while being read",
            }
            raise PrincipalError(
                messages.get(
                    error.code,
                    "project principal registry could not be read",
                )
            ) from error


def load_project_principals(path: str) -> dict[str, dict[str, str]]:
    """Load one registry; resident callers should retain its source."""

    return PrincipalRegistrySource(path).load()


def _permission_session_time(value: Any) -> int:
    if not isinstance(value, str):
        raise PermissionSessionError("interactive session timestamp is invalid")
    match = re.fullmatch(r"(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d{1,9}))?Z", value)
    if match is None:
        raise PermissionSessionError("interactive session timestamp is invalid")
    try:
        parsed = datetime.strptime(match.group(1), "%Y-%m-%dT%H:%M:%S")
    except ValueError as error:
        raise PermissionSessionError("interactive session timestamp is invalid") from error
    fraction = (match.group(2) or "").ljust(9, "0")
    return calendar.timegm(parsed.timetuple()) * 1_000_000_000 + int(fraction or "0")


def _permission_session_object(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result: dict[str, Any] = {}
    for key, value in pairs:
        if key in result:
            raise PermissionSessionError("interactive session registry has duplicate fields")
        result[key] = value
    return result


def _parse_permission_sessions(raw: bytes) -> list[dict[str, Any]]:
    try:
        document = json.loads(raw, object_pairs_hook=_permission_session_object)
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError) as error:
        raise PermissionSessionError("interactive session registry is invalid") from error
    if not isinstance(document, dict) or set(document) != {"schema_version", "sessions"} or document.get("schema_version") != 2:
        raise PermissionSessionError("interactive session registry version is invalid")
    sessions = document.get("sessions")
    if not isinstance(sessions, list) or len(sessions) > 128:
        raise PermissionSessionError("interactive session collection is invalid")
    pairs: set[tuple[str, str]] = set()
    epochs: set[str] = set()
    endpoints: set[tuple[str, str]] = set()
    nonces: set[str] = set()
    result: list[dict[str, Any]] = []
    expected_fields = {
        "schema_version", "workspace_manifest_id", "workspace_id",
        "attachment_id", "owner_kind", "frozen_principal_fingerprint",
        "owner_pid", "ingestion_transport", "ingestion_endpoint",
        "ingestion_nonce", "created_at",
        "lease_issued_at", "expires_at",
    }
    for session in sessions:
        if not isinstance(session, dict) or set(session) != expected_fields or session.get("schema_version") != 2:
            raise PermissionSessionError("interactive session shape is invalid")
        workspace_manifest_id = session.get("workspace_manifest_id")
        workspace_id = session.get("workspace_id")
        attachment_id = session.get("attachment_id")
        fingerprint = session.get("frozen_principal_fingerprint")
        owner_pid = session.get("owner_pid")
        transport = session.get("ingestion_transport")
        endpoint = session.get("ingestion_endpoint")
        nonce = session.get("ingestion_nonce")
        if (
            not isinstance(workspace_manifest_id, str) or PROJECT_ID_PATTERN.fullmatch(workspace_manifest_id) is None
            or not isinstance(workspace_id, str) or PROJECT_ID_PATTERN.fullmatch(workspace_id) is None
            or not isinstance(attachment_id, str) or re.fullmatch(r"att_[0-9a-f]{32}", attachment_id) is None
            or session.get("owner_kind") != "interactive_workspace"
            or not isinstance(fingerprint, str) or re.fullmatch(r"[0-9a-f]{64}", fingerprint) is None
            or not isinstance(owner_pid, int) or isinstance(owner_pid, bool) or owner_pid < 1
            or not isinstance(nonce, str) or re.fullmatch(r"[0-9a-f]{64}", nonce) is None
        ):
            raise PermissionSessionError("interactive session authority is invalid")
        if transport == PERMISSION_INGESTION_UNIX:
            if not isinstance(endpoint, str) or re.fullmatch(r"pws_[0-9a-f]{32}\.sock", endpoint) is None:
                raise PermissionSessionError("interactive session Unix endpoint is invalid")
        elif transport == PERMISSION_INGESTION_LOOPBACK_TCP:
            if not isinstance(endpoint, str):
                raise PermissionSessionError("interactive session loopback endpoint is invalid")
            match = re.fullmatch(r"127\.0\.0\.1:([1-9][0-9]{0,4})", endpoint)
            if match is None or int(match.group(1)) > 65535:
                raise PermissionSessionError("interactive session loopback endpoint is invalid")
        else:
            raise PermissionSessionError("interactive session transport is invalid")
        created = _permission_session_time(session.get("created_at"))
        issued = _permission_session_time(session.get("lease_issued_at"))
        expires = _permission_session_time(session.get("expires_at"))
        if issued < created or expires <= issued or expires - issued > PERMISSION_SESSION_LEASE_SECONDS * 1_000_000_000:
            raise PermissionSessionError("interactive session lease is invalid")
        pair = (workspace_manifest_id, workspace_id)
        transport_endpoint = (transport, endpoint)
        if pair in pairs or attachment_id in epochs or transport_endpoint in endpoints or nonce in nonces:
            raise PermissionSessionError("interactive session authority is ambiguous")
        pairs.add(pair)
        epochs.add(attachment_id)
        endpoints.add(transport_endpoint)
        nonces.add(nonce)
        result.append(dict(session))
    return result


class PermissionSessionSource:
    """Return one current canonical interactive owner without stale fallback."""

    def __init__(self, path: str, transport: str) -> None:
        if transport not in {PERMISSION_INGESTION_UNIX, PERMISSION_INGESTION_LOOPBACK_TCP}:
            raise PermissionSessionError("permission ingestion profile is invalid")
        self.transport = transport
        self._cache = StatIdentityCache(path, MAX_PERMISSION_SESSION_BYTES, _parse_permission_sessions)

    def resolve(self, principal: dict[str, str], now: float) -> dict[str, Any]:
        try:
            sessions = self._cache.load()
        except PermissionSessionError:
            raise
        except ValidatedFileError as error:
            raise PermissionSessionError("interactive session registry is unavailable") from error
        now_ns = int(now * 1_000_000_000)
        matches = [
            session for session in sessions
            if session["ingestion_transport"] == self.transport
            and session["workspace_manifest_id"] == principal["context_id"]
            and session["workspace_id"] == principal["project_id"]
            and session["frozen_principal_fingerprint"] == principal.get("_frozen_principal_fingerprint")
            and _permission_session_time(session["lease_issued_at"]) <= now_ns
            and now_ns < _permission_session_time(session["expires_at"])
        ]
        if len(matches) != 1:
            raise PermissionSessionError("interactive session join is not unique")
        return matches[0]

    def confirm(
        self, principal: dict[str, str], expected: dict[str, Any], now: float
    ) -> dict[str, Any]:
        current = self.resolve(principal, now)
        stable_fields = {
            "schema_version", "workspace_manifest_id", "workspace_id",
            "attachment_id", "owner_kind", "frozen_principal_fingerprint",
            "owner_pid", "ingestion_transport", "ingestion_endpoint",
            "ingestion_nonce", "created_at",
        }
        if any(current.get(field) != expected.get(field) for field in stable_fields):
            raise PermissionSessionError("interactive session authority changed after acknowledgement")
        current_issued = _permission_session_time(current["lease_issued_at"])
        expected_issued = _permission_session_time(expected["lease_issued_at"])
        current_expires = _permission_session_time(current["expires_at"])
        expected_expires = _permission_session_time(expected["expires_at"])
        if not (
            (current_issued == expected_issued and current_expires == expected_expires)
            or (current_issued > expected_issued and current_expires > expected_expires)
        ):
            raise PermissionSessionError("interactive session lease regressed after acknowledgement")
        return current


def _permission_ingestion_profile(
    transport: str, directory: str | None
) -> tuple[str, str | None] | None:
    if transport == PERMISSION_INGESTION_UNIX and directory == "/run/tobari/permission-ingestion":
        return transport, directory
    if transport == PERMISSION_INGESTION_LOOPBACK_TCP and directory is None:
        return transport, None
    return None


def _permission_owner_path(path: str, expected_mode: int, expected_type: int) -> bool:
    try:
        info = os.lstat(path)
    except OSError:
        return False
    return (
        stat.S_IFMT(info.st_mode) == expected_type
        and stat.S_IMODE(info.st_mode) == expected_mode
        and info.st_uid == os.getuid()
        and not stat.S_ISLNK(info.st_mode)
    )


def register_permission_wait(
    transport: str,
    socket_directory: str | None,
    session: dict[str, Any],
    record: dict[str, Any],
    timeout: float = 0.5,
) -> bool:
    try:
        payload = json.dumps(record, separators=(",", ":"), ensure_ascii=False).encode("utf-8")
    except (TypeError, UnicodeError, ValueError):
        return False
    if not payload or len(payload) > MAX_PERMISSION_WAIT_REQUEST_BYTES:
        return False
    if session.get("ingestion_transport") != transport:
        return False
    frame = b"W" + session["ingestion_nonce"].encode("ascii") + len(payload).to_bytes(4, "big") + payload
    connection: socket.socket | None = None
    accepted = False
    try:
        if transport == PERMISSION_INGESTION_UNIX:
            endpoint = session["ingestion_endpoint"]
            if (
                not isinstance(socket_directory, str)
                or os.path.basename(endpoint) != endpoint
                or not _permission_owner_path(socket_directory, 0o700, stat.S_IFDIR)
            ):
                return False
            path = os.path.join(socket_directory, endpoint)
            if not _permission_owner_path(path, 0o600, stat.S_IFSOCK):
                return False
            connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            address: str | tuple[str, int] = path
        elif transport == PERMISSION_INGESTION_LOOPBACK_TCP:
            endpoint = session["ingestion_endpoint"]
            match = re.fullmatch(r"127\.0\.0\.1:([1-9][0-9]{0,4})", endpoint)
            if match is None or int(match.group(1)) > 65535 or socket_directory is not None:
                return False
            connection = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
            address = (PERMISSION_INGESTION_GATEWAY_HOST, int(match.group(1)))
        else:
            return False
        connection.settimeout(timeout)
        connection.connect(address)
        connection.sendall(frame)
        acknowledgement = b""
        while len(acknowledgement) < 2:
            chunk = connection.recv(2 - len(acknowledgement))
            if not chunk:
                return False
            acknowledgement += chunk
        accepted = acknowledgement == b"OK"
    except (OSError, UnicodeError, ValueError):
        accepted = False
    finally:
        if connection is not None:
            try:
                connection.close()
            except OSError:
                accepted = False
    return accepted


def _parse_host_loopback_routes(raw: bytes) -> list[dict[str, Any]]:
    try:
        document = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise HostLoopbackError("Host Loopback registry is invalid") from error
    if not isinstance(document, dict) or set(document) != {"schema_version", "routes"} or document.get("schema_version") != HOST_LOOPBACK_REGISTRY_SCHEMA:
        raise HostLoopbackError("Host Loopback registry version is invalid")
    routes = document.get("routes")
    if not isinstance(routes, list) or len(routes) > 128:
        raise HostLoopbackError("Host Loopback route collection is invalid")
    result: list[dict[str, Any]] = []
    projects: set[str] = set()
    for route in routes:
        if not isinstance(route, dict) or set(route) != {
            "id", "attachment_epoch_id", "context_id", "context", "project_id",
            "project_root", "hostname", "relay_port", "relay_token",
        }:
            raise HostLoopbackError("Host Loopback route shape is invalid")
        identity_material = "\x00".join(("tobari-host-loopback-route-v2", str(route.get("attachment_epoch_id")), str(route.get("context_id")), str(route.get("project_id")), str(route.get("hostname"))))
        expected_id = "hlr_" + hashlib.sha256(identity_material.encode()).hexdigest()[:32]
        if (
            not isinstance(route.get("id"), str)
            or re.fullmatch(r"hlr_[0-9a-f]{32}", route["id"]) is None
            or route["id"] != expected_id
            or not isinstance(route.get("attachment_epoch_id"), str)
            or re.fullmatch(r"att_[0-9a-f]{32}", route["attachment_epoch_id"]) is None
            or not isinstance(route.get("context_id"), str)
            or PROJECT_ID_PATTERN.fullmatch(route["context_id"]) is None
            or not isinstance(route.get("project_id"), str)
            or PROJECT_ID_PATTERN.fullmatch(route["project_id"]) is None
            or route["project_id"] in projects
            or not isinstance(route.get("context"), str)
            or re.fullmatch(r"[a-z][a-z0-9-]{0,62}", route["context"]) is None
            or not isinstance(route.get("project_root"), str)
            or not route["project_root"].startswith("/")
            or route.get("hostname") != HOST_LOOPBACK_HOSTNAME
            or not isinstance(route.get("relay_port"), int)
            or isinstance(route["relay_port"], bool)
            or route["relay_port"] < 1024
            or route["relay_port"] > 65535
            or not isinstance(route.get("relay_token"), str)
            or re.fullmatch(r"[0-9a-f]{64}", route["relay_token"]) is None
        ):
            raise HostLoopbackError("Host Loopback route binding is invalid")
        projects.add(route["project_id"])
        result.append(route)
    return result


def _parse_attachment_grants(raw: bytes) -> list[dict[str, Any]]:
    try:
        document = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise HostLoopbackError("attachment grant registry is invalid") from error
    if not isinstance(document, dict) or set(document) != {"schema_version", "grants"} or document.get("schema_version") != HOST_LOOPBACK_REGISTRY_SCHEMA:
        raise HostLoopbackError("attachment grant registry version is invalid")
    grants = document.get("grants")
    if not isinstance(grants, list) or len(grants) > 512:
        raise HostLoopbackError("attachment grant collection is invalid")
    result: list[dict[str, Any]] = []
    for grant in grants:
        required = {
            "id", "decision", "lifetime", "destination_kind", "context_id", "project_id",
            "attachment_epoch_id", "host", "target_port", "method", "path", "source_candidate",
        }
        material = "\x00".join(("tobari-attachment-grant-v2", str(grant.get("decision")), str(grant.get("context_id")), str(grant.get("project_id")), str(grant.get("attachment_epoch_id")), str(grant.get("host")), str(grant.get("target_port")), str(grant.get("method")), str(grant.get("path")), str(grant.get("source_candidate"))))
        expected_id = "pag_" + hashlib.sha256(material.encode()).hexdigest()[:32]
        if (
            not isinstance(grant, dict)
            or set(grant) != required
            or grant.get("decision") not in {"allow", "deny"}
            or grant.get("lifetime") != "attachment"
            or grant.get("destination_kind") != "host_loopback"
            or grant.get("id") != expected_id
            or not isinstance(grant.get("context_id"), str)
            or PROJECT_ID_PATTERN.fullmatch(grant["context_id"]) is None
            or not isinstance(grant.get("project_id"), str)
            or PROJECT_ID_PATTERN.fullmatch(grant["project_id"]) is None
            or not isinstance(grant.get("attachment_epoch_id"), str)
            or re.fullmatch(r"att_[0-9a-f]{32}", grant["attachment_epoch_id"]) is None
            or grant.get("host") != HOST_LOOPBACK_HOSTNAME
            or not isinstance(grant.get("target_port"), int)
            or isinstance(grant["target_port"], bool)
            or grant["target_port"] < 1024
            or grant["target_port"] > 65535
            or not isinstance(grant.get("source_candidate"), str)
            or re.fullmatch(r"pcy_[0-9a-f]{32}", grant["source_candidate"]) is None
            or not isinstance(grant.get("method"), str)
            or re.fullmatch(r"[A-Z][A-Z0-9_-]{0,31}", grant["method"]) is None
            or not isinstance(grant.get("path"), str)
            or not grant["path"].startswith("/")
        ):
            raise HostLoopbackError("attachment grant is invalid")
        result.append(grant)
    return result


class HostLoopbackRegistrySource:
    def __init__(self, route_path: str, grant_path: str) -> None:
        self._routes = StatIdentityCache(route_path, MAX_HOST_LOOPBACK_CONFIG_BYTES, _parse_host_loopback_routes)
        self._grants = StatIdentityCache(grant_path, MAX_HOST_LOOPBACK_CONFIG_BYTES, _parse_attachment_grants)

    def resolve(self, principal: dict[str, str], scheme: str, host: str, port: int) -> tuple[dict[str, Any] | None, list[dict[str, Any]]]:
        if host != HOST_LOOPBACK_HOSTNAME:
            return None, []
        if scheme != "http" or port < 1024 or port > 65535:
            raise HostLoopbackError("Host Loopback supports plain HTTP on non-privileged ports")
        try:
            routes = self._routes.load()
            grants = self._grants.load()
        except (ValidatedFileError, HostLoopbackError) as error:
            raise HostLoopbackError("Host Loopback authority could not be read") from error
        matches = [route for route in routes if route["project_id"] == principal["project_id"] and route["context_id"] == principal["context_id"]]
        if len(matches) != 1:
            raise HostLoopbackError("Host Loopback is not active for this Workspace")
        route = matches[0]
        active = [grant for grant in grants if grant["project_id"] == route["project_id"] and grant["context_id"] == route["context_id"] and grant["attachment_epoch_id"] == route["attachment_epoch_id"]]
        return route, active


class HostLoopbackBridges:
    """Create one short-lived loopback listener for one authorized host relay."""

    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._pending: set[tuple[str, int]] = set()

    def open(self, relay_port: int, relay_token: str, target_port: int) -> tuple[str, int]:
        listener = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        listener.settimeout(30.0)
        address = listener.getsockname()
        with self._lock:
            self._pending.add(address)
        threading.Thread(target=self._serve, args=(listener, address, relay_port, relay_token, target_port), daemon=True).start()
        return address

    def authorize(self, address: tuple[str, int]) -> bool:
        with self._lock:
            if address not in self._pending:
                return False
            self._pending.remove(address)
            return True

    def _serve(self, listener: socket.socket, address: tuple[str, int], relay_port: int, relay_token: str, target_port: int) -> None:
        inbound: socket.socket | None = None
        outbound: socket.socket | None = None
        try:
            inbound, _ = listener.accept()
            outbound = socket.create_connection(("host.docker.internal", relay_port), 10.0)
            outbound.sendall(b"C" + relay_token.encode("ascii") + target_port.to_bytes(2, "big"))
            acknowledgement = b""
            while len(acknowledgement) < 2:
                chunk = outbound.recv(2 - len(acknowledgement))
                if not chunk:
                    break
                acknowledgement += chunk
            if acknowledgement != b"OK":
                raise OSError("Host Loopback relay rejected the bridge")
            outbound.settimeout(None)
            threads = [
                threading.Thread(target=self._copy, args=(inbound, outbound), daemon=True),
                threading.Thread(target=self._copy, args=(outbound, inbound), daemon=True),
            ]
            for thread in threads:
                thread.start()
            for thread in threads:
                thread.join()
        except OSError:
            pass
        finally:
            with self._lock:
                self._pending.discard(address)
            listener.close()
            if inbound is not None:
                inbound.close()
            if outbound is not None:
                outbound.close()

    @staticmethod
    def _copy(source: socket.socket, destination: socket.socket) -> None:
        try:
            while chunk := source.recv(65536):
                destination.sendall(chunk)
        except OSError:
            pass
        try:
            destination.shutdown(socket.SHUT_WR)
        except OSError:
            pass
        try:
            destination.shutdown(socket.SHUT_WR)
        except OSError:
            pass


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

    __slots__ = ("allow", "reason", "status_code", "learnable")

    def __init__(
        self,
        allow: bool,
        reason: str,
        status_code: int,
        learnable: bool,
    ) -> None:
        self.allow = allow
        self.reason = reason
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
    broker_provider: str | None = None,
    graphql: ParsedGraphQLRequest | None = None,
    mcp: ParsedMCPRequest | None = None,
    aws: ParsedAWSRequest | None = None,
    kubernetes: ParsedKubernetesRequest | None = None,
    git: ParsedGitRequest | None = None,
    oci: ParsedOCIRequest | None = None,
    host_loopback: dict[str, Any] | None = None,
    attachment_grants: list[dict[str, Any]] | None = None,
) -> dict[str, Any]:
    request = flow.request
    split = urlsplit(request.url)
    scheme, host, port = request_authority(flow)
    path = split.path or "/"
    policy_input = {
        "schema_version": 2,
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
            # The fixed evaluator consumes no request headers. Query values are
            # excluded by default and projected only from a validated protocol
            # identity below.
            "query": {},
        },
        "authorization": {
            "broker_provider": broker_provider,
        },
    }
    if graphql is not None:
        # GET transports carry the source document and variables in the URL.
        # They are payload, never policy input, just like the POST body.
        policy_input["request"]["query"] = {}
        policy_input["request"]["graphql"] = {
            "operation_type": graphql.operation_type,
            "root_fields": list(graphql.root_fields),
        }
    if mcp is not None:
        policy_input["request"]["mcp"] = {"method": mcp.method}
        if mcp.tool_name is not None:
            policy_input["request"]["mcp"]["tool_name"] = mcp.tool_name
    if aws is not None:
        policy_input["request"]["aws"] = {
            "wire_protocol": aws.wire_protocol,
            "service": aws.service,
            "operation": aws.operation,
        }
        if aws.protocol_version is not None:
            policy_input["request"]["aws"]["protocol_version"] = aws.protocol_version
        if aws.target_namespace is not None:
            policy_input["request"]["aws"]["target_namespace"] = aws.target_namespace
    if kubernetes is not None:
        kubernetes.validate()
        if kubernetes.kind == "resource":
            policy_input["request"]["kubernetes"] = {
                "kind": "resource",
                "verb": kubernetes.verb,
                "dry_run": kubernetes.dry_run,
                "resource": {
                    "group": kubernetes.group,
                    "version": kubernetes.version,
                    "resource": kubernetes.resource,
                    "namespace": kubernetes.namespace,
                    "name": kubernetes.name,
                    "subresource": kubernetes.subresource,
                },
            }
        else:
            policy_input["request"]["kubernetes"] = {
                "kind": "non_resource",
                "verb": kubernetes.verb,
                "path": kubernetes.non_resource_path,
            }
    if git is not None:
        git.validate()
        policy_input["request"]["git"] = {
            "service": git.service,
            "repository": git.repository,
        }
        if request.method.upper() == "GET":
            policy_input["request"]["query"] = {
                "service": [f"git-{git.service}"],
            }
    if oci is not None:
        oci.validate()
        policy_input["request"]["query"] = {}
        policy_input["request"]["oci"] = {
            "action": oci.action,
            "repository": oci.repository,
            "object": oci.object,
        }
    if host_loopback is not None:
        policy_input["destination"] = {
            "kind": "host_loopback",
            "attachment_epoch_id": host_loopback["attachment_epoch_id"],
        }
        policy_input["authorization"]["attachment_grants"] = attachment_grants or []
    return policy_input


def _parse_decision(document: Any) -> Decision:
    if not isinstance(document, dict) or not isinstance(document.get("result"), dict):
        raise PolicyUnavailable("OPA response has no result object")
    result = document["result"]
    allow = result.get("allow")
    reason = result.get("reason")
    if set(result) != {"allow", "reason", "status_code", "learnable"}:
        raise PolicyUnavailable("OPA result has an invalid shape")
    if not isinstance(allow, bool) or not isinstance(reason, str) or not reason:
        raise PolicyUnavailable("OPA result has invalid allow or reason")
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


def load_gateway_config(path: str) -> dict[str, Any]:
    try:
        with open(path, "rb") as handle:
            raw = handle.read(MAX_GATEWAY_CONFIG_BYTES + 1)
    except OSError as error:
        raise CredentialError("Gateway configuration could not be read") from error
    if len(raw) > MAX_GATEWAY_CONFIG_BYTES:
        raise CredentialError("Gateway configuration is too large")
    try:
        document = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise CredentialError("Gateway configuration is invalid") from error
    if (
        not isinstance(document, dict)
        or set(document) != {"version", "contexts"}
        or document.get("version") != "v2"
    ):
        raise CredentialError("Gateway configuration version is invalid")
    contexts = document.get("contexts")
    if not isinstance(contexts, dict):
        raise CredentialError("Gateway Contexts are invalid")
    for context_id, context in contexts.items():
        if not isinstance(context_id, str) or not PROJECT_ID_PATTERN.fullmatch(context_id):
            raise CredentialError("Gateway Context identity is invalid")
        if not isinstance(context, dict) or set(context) != {
            "name", "graphql_endpoints", "mcp_endpoints", "kubernetes_endpoints"
        }:
            raise CredentialError("Gateway Context is invalid")
        name = context.get("name")
        if not isinstance(name, str) or re.fullmatch(r"[a-z][a-z0-9-]{0,62}", name) is None:
            raise CredentialError("Gateway Context name is invalid")
        seen_protocol_endpoints: set[tuple[str, str, int, str]] = set()
        for protocol_key in ("graphql_endpoints", "mcp_endpoints", "kubernetes_endpoints"):
            endpoints = context.get(protocol_key)
            if not isinstance(endpoints, list):
                raise CredentialError("Gateway semantic endpoint configuration is invalid")
            seen_endpoints: set[tuple[str, str, int, str]] = set()
            for endpoint in endpoints:
                _validate_gateway_endpoint(endpoint)
                if protocol_key == "kubernetes_endpoints" and (
                    endpoint["scheme"] != "https"
                    or endpoint["port"] != 443
                    or endpoint["path"] != "/"
                    or not endpoint["host"].endswith(".eks.amazonaws.com")
                ):
                    raise CredentialError("Gateway Kubernetes endpoint is outside the validated EKS boundary")
                identity = (endpoint["scheme"], endpoint["host"], endpoint["port"], endpoint["path"])
                if identity in seen_endpoints or identity in seen_protocol_endpoints:
                    raise CredentialError("Gateway semantic endpoint configuration is ambiguous")
                seen_endpoints.add(identity)
                seen_protocol_endpoints.add(identity)
    return document


def _validate_gateway_endpoint(endpoint: Any) -> None:
    if not isinstance(endpoint, dict) or set(endpoint) != {
        "scheme", "host", "port", "path"
    }:
        raise CredentialError("Gateway semantic endpoint configuration is invalid")
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
        raise CredentialError("Gateway semantic endpoint configuration is invalid")


def graphql_endpoint_declared(
    config: dict[str, Any], context_id: str, scheme: str, host: str, port: int, path: str
) -> bool:
    """Match only host-owned exact endpoint declarations for this Context."""

    context = config.get("contexts", {}).get(context_id)
    if not isinstance(context, dict):
        raise CredentialError("Gateway Context is not established")
    return any(
        endpoint == {"scheme": scheme, "host": host, "port": port, "path": path}
        for endpoint in context.get("graphql_endpoints", [])
    )


def mcp_endpoint_declared(
    config: dict[str, Any], context_id: str, scheme: str, host: str, port: int, path: str
) -> bool:
    context = config.get("contexts", {}).get(context_id)
    if not isinstance(context, dict):
        raise CredentialError("Gateway Context is not established")
    return any(
        endpoint == {"scheme": scheme, "host": host, "port": port, "path": path}
        for endpoint in context.get("mcp_endpoints", [])
    )


def kubernetes_endpoint_declared(
    config: dict[str, Any], context_id: str, scheme: str, host: str, port: int
) -> bool:
    context = config.get("contexts", {}).get(context_id)
    if not isinstance(context, dict):
        return False
    return any(
        endpoint == {"scheme": scheme, "host": host, "port": port, "path": "/"}
        for endpoint in context.get("kubernetes_endpoints", [])
    )


def _deny(flow: http.HTTPFlow, status: int, code: str) -> None:
    body = json.dumps({"error": code}, separators=(",", ":")).encode("utf-8")
    flow.response = http.Response.make(status, body, {"content-type": "application/json"})


def _permission_wait_effect(policy_input: dict[str, Any]) -> dict[str, Any] | None:
    try:
        authority = policy_input["request"]["authority"]
        request = policy_input["request"]
        raw = request["path"]["raw"]
        segments = request["path"]["segments"]
        scheme, host, port = authority["scheme"], authority["host"], authority["port"]
        method = request["method"]
    except (KeyError, TypeError):
        return None
    try:
        raw_bytes = raw.encode("utf-8")
    except (AttributeError, UnicodeError):
        return None
    if (
        scheme not in {"http", "https"}
        or not isinstance(host, str) or host in {HOST_LOOPBACK_HOSTNAME, RETIRED_HOST_LOOPBACK_HOSTNAME}
        or not 1 <= len(host) <= 253 or host != host.lower() or host.endswith(".")
        or any(
            not 1 <= len(label) <= 63 or label.startswith("-") or label.endswith("-")
            or re.fullmatch(r"[a-z0-9-]+", label) is None
            for label in host.split(".")
        )
        or not isinstance(port, int) or isinstance(port, bool) or not 1 <= port <= 65535
        or not isinstance(method, str) or re.fullmatch(r"[A-Z][A-Z0-9!#$%&'*+.^_`|~-]{0,31}", method) is None
        or not isinstance(raw, str) or not raw.startswith("/") or len(raw_bytes) > 4096
        or not isinstance(segments, list) or not all(isinstance(segment, str) for segment in segments)
    ):
        return None
    normalized: list[str] = []
    for raw_segment in raw.split("/"):
        if not raw_segment:
            continue
        for index, character in enumerate(raw_segment):
            if character == "%" and (
                index + 2 >= len(raw_segment)
                or re.fullmatch(r"[0-9A-Fa-f]{2}", raw_segment[index + 1:index + 3]) is None
            ):
                return None
        try:
            decoded = unquote(raw_segment, errors="strict")
        except UnicodeDecodeError:
            return None
        if any(ord(character) < 32 or ord(character) == 127 or character in {"\u2028", "\u2029"} for character in decoded):
            return None
        normalized.append(decoded)
    if normalized != segments:
        return None
    return {"scheme": scheme, "host": host, "port": port, "method": method, "path": raw, "segments": list(segments)}


def _terminal_tls_reason(client_hello: Any) -> str | None:
    """Classify authority that must close before mitmproxy creates a leaf."""
    try:
        extensions = client_hello.extensions
        if not isinstance(extensions, list) or any(
            not isinstance(item, tuple) or len(item) != 2 or not isinstance(item[0], int)
            for item in extensions
        ):
            return "tls_authority_malformed"
        if any(extension_type in ECH_EXTENSION_TYPES for extension_type, _ in extensions):
            return "tls_authority_unobservable"
        sni = client_hello.sni
    except (AttributeError, TypeError, UnicodeError, ValueError):
        return "tls_authority_malformed"
    if sni is None:
        return "tls_authority_unobservable"
    if not isinstance(sni, str):
        return "tls_authority_malformed"
    normalized = sni.rstrip(".").lower()
    if normalized in {HOST_LOOPBACK_HOSTNAME, RETIRED_HOST_LOOPBACK_HOSTNAME}:
        return "host_loopback_tls_unsupported"
    return None


def _policy_denied(
    flow: http.HTTPFlow,
    status: int,
    learnable: bool,
    principal: dict[str, str],
    resume: dict[str, Any] | None = None,
    graphql: ParsedGraphQLRequest | None = None,
    mcp: ParsedMCPRequest | None = None,
    aws: ParsedAWSRequest | None = None,
    kubernetes: ParsedKubernetesRequest | None = None,
    git: ParsedGitRequest | None = None,
    oci: ParsedOCIRequest | None = None,
) -> None:
    path = urlsplit(flow.request.url).path or "/"
    review_available = bool(learnable)
    review = {
        "available": review_available,
        "command": "tobari review permissions" if review_available else None,
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
            "run `tobari review permissions`. After Apply succeeds, retry this request in the same Workspace. "
            "Tobari does not approve or retry automatically."
        )
    document = {
        "error": "policy_denied",
        "message": message,
        "tobari": {
            "schema_version": 3,
            "workspace_manifest_id": principal["context_id"],
            "workspace_id": principal["project_id"],
            "event": "permission_review_available"
            if review_available
            else "permission_review_unavailable",
            "run_on": "host",
            "review": review,
            "request": {
                "scheme": flow.request.scheme.lower(),
                "host": flow.request.host.rstrip(".").lower(),
                "port": flow.request.port,
                "method": flow.request.method.upper(),
                "path": path,
            },
        },
    }
    if resume is not None:
        document["tobari"]["resume"] = resume
    if graphql is not None:
        document["tobari"]["request"]["protocol"] = "graphql"
        document["tobari"]["request"]["graphql_operation_type"] = graphql.operation_type
        document["tobari"]["request"]["graphql_root_fields"] = list(graphql.root_fields)
    if mcp is not None:
        document["tobari"]["request"]["protocol"] = "mcp"
        document["tobari"]["request"]["mcp_method"] = mcp.method
        if mcp.tool_name is not None:
            document["tobari"]["request"]["mcp_tool_name"] = mcp.tool_name
    if aws is not None:
        document["tobari"]["request"]["protocol"] = "aws"
        document["tobari"]["request"]["aws_wire_protocol"] = aws.wire_protocol
        document["tobari"]["request"]["aws_service"] = aws.service
        if aws.protocol_version is not None:
            document["tobari"]["request"]["aws_protocol_version"] = aws.protocol_version
        if aws.target_namespace is not None:
            document["tobari"]["request"]["aws_target_namespace"] = aws.target_namespace
        document["tobari"]["request"]["aws_operation"] = aws.operation
    if kubernetes is not None:
        kubernetes.validate()
        document["tobari"]["request"]["protocol"] = "kubernetes"
        document["tobari"]["request"]["kubernetes_kind"] = kubernetes.kind
        document["tobari"]["request"]["kubernetes_verb"] = kubernetes.verb
        if kubernetes.kind == "resource":
            document["tobari"]["request"]["kubernetes_group"] = kubernetes.group
            document["tobari"]["request"]["kubernetes_version"] = kubernetes.version
            document["tobari"]["request"]["kubernetes_resource"] = kubernetes.resource
            document["tobari"]["request"]["kubernetes_namespace"] = kubernetes.namespace
            document["tobari"]["request"]["kubernetes_name"] = kubernetes.name
            document["tobari"]["request"]["kubernetes_subresource"] = kubernetes.subresource
            document["tobari"]["request"]["kubernetes_dry_run"] = kubernetes.dry_run
        else:
            document["tobari"]["request"]["kubernetes_non_resource_path"] = kubernetes.non_resource_path
    if git is not None:
        git.validate()
        document["tobari"]["request"]["protocol"] = "git"
        document["tobari"]["request"]["git_service"] = git.service
        document["tobari"]["request"]["git_repository"] = git.repository
    if oci is not None:
        oci.validate()
        document["tobari"]["request"]["protocol"] = "oci"
        document["tobari"]["request"]["oci_action"] = oci.action
        document["tobari"]["request"]["oci_repository"] = oci.repository
        document["tobari"]["request"]["oci_object"] = oci.object
    body = json.dumps(document, separators=(",", ":")).encode("utf-8")
    flow.response = http.Response.make(status, body, {"content-type": "application/json"})


def _audit(**fields: Any) -> None:
    fields["schema_version"] = 2
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


def _mcp_audit_event(base: dict[str, Any], parsed: ParsedMCPRequest) -> dict[str, Any]:
    event = {**base, "protocol": "mcp", "mcp_method": parsed.method}
    if parsed.tool_name is not None:
        event["mcp_tool_name"] = parsed.tool_name
    return event


def _aws_audit_event(base: dict[str, Any], parsed: ParsedAWSRequest) -> dict[str, Any]:
    event = dict(base)
    event["protocol"] = "aws"
    event["aws_wire_protocol"] = parsed.wire_protocol
    event["aws_service"] = parsed.service
    if parsed.protocol_version is not None:
        event["aws_protocol_version"] = parsed.protocol_version
    if parsed.target_namespace is not None:
        event["aws_target_namespace"] = parsed.target_namespace
    event["aws_operation"] = parsed.operation
    return event


def _kubernetes_audit_event(base: dict[str, Any], parsed: ParsedKubernetesRequest) -> dict[str, Any]:
    parsed.validate()
    event = {**base, "protocol": "kubernetes", "kubernetes_kind": parsed.kind, "kubernetes_verb": parsed.verb}
    if parsed.kind == "resource":
        event.update({
            "kubernetes_group": parsed.group,
            "kubernetes_version": parsed.version,
            "kubernetes_resource": parsed.resource,
            "kubernetes_namespace": parsed.namespace,
            "kubernetes_name": parsed.name,
            "kubernetes_subresource": parsed.subresource,
            "kubernetes_dry_run": parsed.dry_run,
        })
    else:
        event["kubernetes_non_resource_path"] = parsed.non_resource_path
    return event


def _git_audit_event(base: dict[str, Any], parsed: ParsedGitRequest) -> dict[str, Any]:
    parsed.validate()
    return {
        **base,
        "protocol": "git",
        "git_service": parsed.service,
        "git_repository": parsed.repository,
    }


def _oci_audit_event(base: dict[str, Any], parsed: ParsedOCIRequest) -> dict[str, Any]:
    parsed.validate()
    return {
        **base,
        "protocol": "oci",
        "oci_action": parsed.action,
        "oci_repository": parsed.repository,
        "oci_object": parsed.object,
    }


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
        self.gateway_config_path = os.getenv(
            "TOBARI_GATEWAY_CONFIG",
            "/run/tobari/config/gateway.json",
        )
        # The aggregate projection is immutable for the Gateway container's
        # lifetime. Load trusted endpoint declarations once, never from caller
        # headers and never after body bytes have arrived.
        self.graphql_config = load_gateway_config(self.gateway_config_path)
        base_credential_adapter = PassthroughCredentialAdapter()
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
            self.credential_adapter = _experimental_broker_adapter(
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
        self.principal_source = PrincipalRegistrySource(self.principal_path)
        self.host_loopback_source = HostLoopbackRegistrySource(
            os.getenv("TOBARI_HOST_LOOPBACK_REGISTRY", "/run/tobari/host-loopback/routes.json"),
            os.getenv("TOBARI_ATTACHMENT_GRANT_REGISTRY", "/run/tobari/host-loopback/grants.json"),
        )
        self.host_loopback_bridges = HostLoopbackBridges()
        self.permission_ingestion_transport = os.getenv(
            "TOBARI_PERMISSION_INGESTION_TRANSPORT", ""
        )
        self.permission_socket_directory: str | None = None
        configured_directory = os.getenv("TOBARI_PERMISSION_INGESTION_DIRECTORY")
        profile = _permission_ingestion_profile(
            self.permission_ingestion_transport, configured_directory
        )
        if profile is not None:
            self.permission_ingestion_transport, self.permission_socket_directory = profile
            self.permission_session_source: PermissionSessionSource | None = PermissionSessionSource(
                os.getenv(
                    "TOBARI_INTERACTIVE_ATTACHMENT_REGISTRY",
                    "/run/tobari/interactive-attachments/sessions.json",
                ),
                self.permission_ingestion_transport,
            )
        else:
            self.permission_session_source = None

    def _permission_resume(
        self, principal: dict[str, str], policy_input: dict[str, Any], request_id: str
    ) -> dict[str, Any] | None:
        effect = _permission_wait_effect(policy_input)
        source = getattr(self, "permission_session_source", None)
        transport = getattr(self, "permission_ingestion_transport", None)
        socket_directory = getattr(self, "permission_socket_directory", None)
        if (
            effect is None
            or source is None
            or transport not in {PERMISSION_INGESTION_UNIX, PERMISSION_INGESTION_LOOPBACK_TCP}
            or re.fullmatch(r"[0-9a-f]{32}", request_id) is None
        ):
            return None
        try:
            now = time.time()
            session = source.resolve(principal, now)
            wait_id = "pwt_" + secrets.token_hex(16)
            created = datetime.fromtimestamp(now, timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")
            expires = datetime.fromtimestamp(now + PERMISSION_WAIT_LEASE_SECONDS, timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")
            record = {
                "schema_version": 2,
                "permission_wait_id": wait_id,
                "denial_correlation_id": request_id,
                "frozen_principal_fingerprint": principal["_frozen_principal_fingerprint"],
                "workspace_manifest_id": principal["context_id"],
                "workspace_id": principal["project_id"],
                "attachment_id": session["attachment_id"],
                "effect": effect,
                "created_at": created,
                "expires_at": expires,
            }
            if not register_permission_wait(transport, socket_directory, session, record):
                return None
            # ACK proves the selected transport accepted the exact record. A
            # second trusted-source read proves that authority did not drift
            # while the request crossed that transport boundary.
            source.confirm(principal, session, time.time())
        except (KeyError, PermissionSessionError, UnicodeError, ValueError):
            return None
        return {
            "available": True,
            "run_on": "workspace",
            "command": "tobari-permission wait --id " + wait_id,
            "automatic_retry": False,
            "result_values": ["allow", "deny", "expired"],
        }

    def server_connect(self, data: Any) -> None:
        address = data.server.address
        if not address:
            data.server.error = "upstream address is missing"
            return
        if self.host_loopback_bridges.authorize(address):
            return
        try:
            data.server.address = resolve_upstream_address(address[0], address[1])
        except UpstreamAddressError as error:
            data.server.error = str(error)

    def tls_clienthello(self, data: Any) -> None:
        reason = _terminal_tls_reason(data.client_hello)
        if reason is None:
            return
        # Context is per connection and shared with tls_start_client. Keeping
        # the marker there avoids a global attacker-sized connection map.
        data.context.tobari_terminal_tls_reason = reason
        data.establish_server_tls_first = False

    def tls_start_client(self, data: Any) -> None:
        if getattr(data.context, "tobari_terminal_tls_reason", None) is not None:
            # False is intentionally non-None: TLSConfig must not generate a
            # leaf, and mitmproxy's TLS layer closes on the false context.
            data.ssl_conn = False

    def requestheaders(self, flow: http.HTTPFlow) -> None:
        started = time.monotonic()
        request_id = uuid.uuid4().hex
        scheme, host, port = "", "", 0
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
        aws_identity: ParsedAWSRequest | None = None
        kubernetes_identity: ParsedKubernetesRequest | None = None
        git_identity: ParsedGitRequest | None = None
        oci_identity: ParsedOCIRequest | None = None
        selected_module: str | None = None
        try:
            scheme, host, port = normalize_ingress_authority(flow)
            request_path = urlsplit(flow.request.url).path or "/"
            audit_path = redacted_audit_path(flow.request.url)
            audit_valid = True
            if host == RETIRED_HOST_LOOPBACK_HOSTNAME:
                reason = "retired Host Loopback authority"
                upstream_status = 410
                _deny(flow, upstream_status, "retired_host_loopback_authority")
                return
            principal = resolve_project_principal(
                flow, self.principal_source.load()
            )
            project_id = principal["project_id"]
            context_id = principal["context_id"]
            context_name = principal["context"]
            project_root = principal["project_root"]
            host_loopback, attachment_grants = None, []
            if host == HOST_LOOPBACK_HOSTNAME:
                host_loopback, attachment_grants = self.host_loopback_source.resolve(
                    principal, scheme, host, port
                )
            request_headers = _request_header_pairs(flow.request)
            credential_request = self.credential_adapter.prepare(
                flow.request, scheme, host, port, context_id, project_id
            )
            aws_classification = None
            brokered_aws_classification = getattr(
                credential_request, "aws_classification", None
            )
            if not isinstance(
                brokered_aws_classification, (ParsedAWSRequest, PendingAWSQueryRequest)
            ):
                brokered_aws_classification = None
            if brokered_aws_classification is not None:
                aws_classification = brokered_aws_classification
            graphql_claimed = brokered_aws_classification is None and graphql_endpoint_declared(
                self.graphql_config, context_id, scheme, host, port, request_path
            )
            mcp_claimed = brokered_aws_classification is None and mcp_endpoint_declared(
                self.graphql_config, context_id, scheme, host, port, request_path
            )
            kubernetes_claimed = brokered_aws_classification is None and kubernetes_endpoint_declared(
                self.graphql_config, context_id, scheme, host, port
            )
            if kubernetes_claimed:
                kubernetes_identity = parse_kubernetes_request(
                    flow.request.method.upper(), request_path,
                    urlsplit(flow.request.url).query, request_headers,
                )
            if brokered_aws_classification is None:
                oci_identity = parse_oci_request(
                    flow.request.method.upper(), request_path,
                    urlsplit(flow.request.url).query,
                )
                git_identity = classify_git_request(
                    flow.request.method.upper(), request_path,
                    urlsplit(flow.request.url).query, request_headers,
                )
            if brokered_aws_classification is None:
                aws_classification = classify_aws_request_headers(
                    flow.request.method.upper(), scheme, host, port, request_path,
                    urlsplit(flow.request.url).query,
                    request_headers,
                )
            claims = []
            if graphql_claimed:
                claims.append("protocols.http.graphql")
            if mcp_claimed:
                claims.append("protocols.http.mcp")
            if aws_classification is not None:
                claims.append("providers.aws")
            if kubernetes_identity is not None:
                claims.append("providers.kubernetes")
            if git_identity is not None:
                claims.append("protocols.http.git")
            if oci_identity is not None:
                claims.append("protocols.http.oci")
            selected_module = select_semantic_module_claims(claims)
            if selected_module == "protocols.http.graphql":
                validate_graphql_headers(
                    flow.request.method.upper(), request_headers,
                    limits=GraphQLParseLimits(),
                )
                flow.request.stream = False
                audit_deferred = True
                flow.metadata["tobari_graphql_pending"] = {
                    "started": started, "request_id": request_id,
                    "scheme": scheme, "host": host, "port": port,
                    "audit_path": audit_path,
                    "url_query": urlsplit(flow.request.url).query,
                    "principal": principal,
                    "credential_request": credential_request,
                }
                return
            if selected_module == "protocols.http.mcp":
                validate_mcp_post_headers(flow.request.method.upper(), request_headers)
                flow.request.stream = False
                audit_deferred = True
                flow.metadata["tobari_mcp_pending"] = {
                    "started": started, "request_id": request_id,
                    "scheme": scheme, "host": host, "port": port,
                    "audit_path": audit_path, "principal": principal,
                    "credential_request": credential_request,
                }
                return
            if isinstance(aws_classification, PendingAWSQueryRequest):
                flow.request.stream = False
                audit_deferred = True
                flow.metadata["tobari_aws_query_pending"] = {
                    "started": started,
                    "request_id": request_id,
                    "scheme": scheme,
                    "host": host,
                    "port": port,
                    "audit_path": audit_path,
                    "principal": principal,
                    "credential_request": credential_request,
                    "classification": aws_classification,
                }
                return
            if isinstance(aws_classification, ParsedAWSRequest):
                aws_identity = aws_classification
            policy_input = build_policy_input(
                flow,
                self.cluster,
                principal,
                credential_request.secret_headers,
                credential_request.broker_provider,
                aws=aws_identity,
                kubernetes=kubernetes_identity,
                git=git_identity,
                oci=oci_identity,
                host_loopback=host_loopback,
                attachment_grants=attachment_grants,
            )
            decision = query_opa(self.opa_url, policy_input, self.opa_timeout)
            reason = decision.reason
            learnable = decision.learnable
            if not decision.allow:
                resume = None
                if (
                    learnable
                    and aws_identity is None and kubernetes_identity is None
                    and git_identity is None and oci_identity is None
                    and host_loopback is None
                    and _permission_wait_effect(policy_input) is not None
                ):
                    resume = self._permission_resume(principal, policy_input, request_id)
                _policy_denied(
                    flow, decision.status_code, learnable, principal, resume,
                    aws=aws_identity, kubernetes=kubernetes_identity,
                    git=git_identity, oci=oci_identity,
                )
                upstream_status = decision.status_code
                return
            credential_request.apply(flow.request)
            decision_name = "allow"
            audit_event = {
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "request_id": request_id,
                "cluster": self.cluster,
                "project_id": project_id,
                "context_id": context_id,
                "context": context_name,
                "project_root": project_root,
                "scheme": scheme,
                "host": host,
                "port": port,
                "method": flow.request.method.upper(),
                "path": audit_path,
                "protocol": "http",
                "decision": decision_name,
                "reason": reason,
                "learnable": learnable,
                "started": started,
            }
            if selected_module == "protocols.http.oci" and oci_identity is not None:
                flow.metadata["tobari_audit"] = _oci_audit_event(audit_event, oci_identity)
            elif selected_module == "protocols.http.git" and git_identity is not None:
                flow.metadata["tobari_audit"] = _git_audit_event(audit_event, git_identity)
            elif selected_module == "providers.kubernetes" and kubernetes_identity is not None:
                flow.metadata["tobari_audit"] = _kubernetes_audit_event(
                    audit_event, kubernetes_identity
                )
            else:
                flow.metadata["tobari_audit"] = (
                    _aws_audit_event(audit_event, aws_identity)
                    if aws_identity is not None else audit_event
                )
            if bool(getattr(credential_request, "deferred", False)):
                # AWS SigV4 needs a hash of the complete, bounded request body.
                # Policy has already allowed the ordinary HTTP effect, but the
                # request remains buffered and cannot reach upstream until the
                # broker returns final signed headers from request().
                flow.metadata["tobari_deferred_credential"] = credential_request
                flow.request.stream = False
            else:
                if host_loopback is not None:
                    flow.metadata.pop("tobari_transparent_authority", None)
                    original_host_header = flow.request.host_header
                    bridge_address = self.host_loopback_bridges.open(
                        host_loopback["relay_port"], host_loopback["relay_token"], port
                    )
                    flow.request.host = bridge_address[0]
                    flow.request.port = bridge_address[1]
                    flow.request.host_header = original_host_header
                    flow.server_conn.address = bridge_address
                else:
                    commit_upstream_authority(flow)
                # Authorization is complete before mitmproxy forwards any body bytes.
                # The body is deliberately opaque to policy and is never retained here.
                flow.request.stream = True
        except (SemanticClassificationError, GraphQLRequestError, MCPRequestError, AWSRequestError, KubernetesRequestError, GitRequestError, OCIRequestError) as error:
            reason = str(error)
            _deny(flow, 400, error.code)
            upstream_status = 400
        except PolicyUnavailable as error:
            reason = str(error)
            _deny(flow, 503, "policy_unavailable")
            upstream_status = 503
        except CredentialAdapterError as error:
            reason = str(error)
            upstream_status, code = _credential_error_response(
                error, "credential_handle_invalid"
            )
            _deny(flow, upstream_status, code)
        except CredentialError as error:
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
        except HostLoopbackError as error:
            reason = str(error)
            _deny(flow, 403, "host_loopback_unavailable")
            upstream_status = 403
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
                event = {
                    "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                    "request_id": request_id, "cluster": self.cluster,
                    "project_id": project_id, "context_id": context_id,
                    "context": context_name, "project_root": project_root,
                    "scheme": scheme, "host": host, "port": port,
                    "method": flow.request.method.upper(), "path": audit_path,
                    "protocol": "http", "decision": decision_name,
                    "reason": reason, "learnable": learnable,
                    "upstream_status": upstream_status,
                    "duration_ms": int((time.monotonic() - started) * 1000),
                }
                if selected_module == "protocols.http.oci" and oci_identity is not None:
                    event = _oci_audit_event(event, oci_identity)
                elif selected_module == "protocols.http.git" and git_identity is not None:
                    event = _git_audit_event(event, git_identity)
                elif selected_module == "providers.kubernetes" and kubernetes_identity is not None:
                    event = _kubernetes_audit_event(event, kubernetes_identity)
                elif selected_module == "providers.aws" and aws_identity is not None:
                    event = _aws_audit_event(event, aws_identity)
                _audit(**event)

    def _complete_graphql_request(
        self, flow: http.HTTPFlow, pending: dict[str, Any]
    ) -> None:
        started = pending["started"]
        request_id = pending["request_id"]
        principal = pending["principal"]
        credential_request = pending["credential_request"]
        scheme = pending["scheme"]
        host = pending["host"]
        port = pending["port"]
        audit_path = pending["audit_path"]
        parsed: ParsedGraphQLRequest | None = None

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
                "scheme": scheme,
                "host": host,
                "port": port,
                "method": flow.request.method.upper(),
                "path": audit_path,
                "protocol": "http",
                "decision": "deny",
                "reason": reason,
                "learnable": learnable,
                "upstream_status": status,
                "duration_ms": int((time.monotonic() - started) * 1000),
            }
            events = _graphql_audit_events(base, parsed) if parsed is not None else [base]
            for event in events:
                _audit(**event)
            if code == "policy_denied" and parsed is not None:
                _policy_denied(flow, status, learnable, principal, graphql=parsed)
            else:
                _deny(flow, status, code)

        try:
            body = flow.request.raw_content
            if not isinstance(body, bytes):
                raise GraphQLRequestError(
                    "invalid_body", "GraphQL request body must be bytes"
                )
            parsed = parse_graphql_request(
                method=flow.request.method.upper(),
                headers=_request_header_pairs(flow.request),
                body=body,
                url_query=pending.get("url_query", ""),
            )
            policy_input = build_policy_input(
                flow,
                self.cluster,
                principal,
                credential_request.secret_headers,
                credential_request.broker_provider,
                parsed,
            )
            decision = query_opa(self.opa_url, policy_input, self.opa_timeout)
            if not decision.allow:
                audit_failure(
                    decision.status_code,
                    "policy_denied",
                    decision.reason,
                    decision.learnable,
                )
                return
            credential_request.apply(flow.request)
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
                "scheme": scheme,
                "host": host,
                "port": port,
                "method": flow.request.method.upper(),
                "path": audit_path,
                "protocol": "http",
                "decision": "allow",
                "reason": decision.reason,
                "learnable": False,
                "started": started,
            }
            flow.metadata["tobari_graphql_audits"] = _graphql_audit_events(base, parsed)
        except GraphQLRequestError as error:
            audit_failure(400, error.code, str(error))
        except PolicyUnavailable as error:
            audit_failure(503, "policy_unavailable", str(error))
        except CredentialAdapterError as error:
            status, code = _credential_error_response(
                error, "credential_handle_invalid"
            )
            audit_failure(status, code, str(error))
        except CredentialError as error:
            audit_failure(503, "credential_unavailable", str(error))
        except AuthorityError as error:
            audit_failure(400, "request_authority_invalid", str(error))
        except (RuntimeError, UnicodeError, ValueError):
            audit_failure(503, "credential_unavailable", "credential processing failed")
        except Exception:
            audit_failure(502, "gateway_error", "gateway error")

    def _complete_mcp_request(
        self, flow: http.HTTPFlow, pending: dict[str, Any]
    ) -> None:
        started = pending["started"]
        principal = pending["principal"]
        credential_request = pending["credential_request"]
        parsed: ParsedMCPRequest | None = None

        def base_event(decision: str, reason: str, learnable: bool) -> dict[str, Any]:
            return {
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "request_id": pending["request_id"], "cluster": self.cluster,
                "project_id": principal["project_id"], "context_id": principal["context_id"],
                "context": principal["context"], "project_root": principal["project_root"],
                "scheme": pending["scheme"], "host": pending["host"], "port": pending["port"],
                "method": flow.request.method.upper(), "path": pending["audit_path"],
                "protocol": "http", "decision": decision, "reason": reason,
                "learnable": learnable, "started": started,
            }

        def fail(status: int, code: str, reason: str, learnable: bool = False) -> None:
            event = base_event("deny", reason, learnable)
            event.pop("started")
            event["upstream_status"] = status
            event["duration_ms"] = int((time.monotonic() - started) * 1000)
            _audit(**(_mcp_audit_event(event, parsed) if parsed is not None else event))
            if code == "policy_denied" and parsed is not None:
                _policy_denied(flow, status, learnable, principal, mcp=parsed)
            else:
                _deny(flow, status, code)

        try:
            body = flow.request.raw_content
            if not isinstance(body, bytes):
                raise MCPRequestError("invalid_body", "MCP request body must be bytes")
            parsed = parse_mcp_post_request(
                flow.request.method.upper(), _request_header_pairs(flow.request), body
            )
            policy_input = build_policy_input(
                flow, self.cluster, principal, credential_request.secret_headers,
                credential_request.broker_provider, mcp=parsed,
            )
            decision = query_opa(self.opa_url, policy_input, self.opa_timeout)
            if not decision.allow:
                fail(decision.status_code, "policy_denied", decision.reason, decision.learnable)
                return
            credential_request.apply(flow.request)
            if bool(getattr(credential_request, "deferred", False)):
                apply_body = getattr(credential_request, "apply_body", None)
                if not callable(apply_body):
                    raise BrokerCredentialUnavailable("deferred credential contract is unavailable")
                apply_body(flow.request)
            commit_upstream_authority(flow)
            flow.metadata["tobari_audit"] = _mcp_audit_event(
                base_event("allow", decision.reason, False), parsed
            )
        except MCPRequestError as error:
            fail(400, error.code, str(error))
        except PolicyUnavailable as error:
            fail(503, "policy_unavailable", str(error))
        except CredentialAdapterError as error:
            status, code = _credential_error_response(error, "credential_handle_invalid")
            fail(status, code, str(error))
        except (CredentialError, RuntimeError, UnicodeError, ValueError) as error:
            fail(503, "credential_unavailable", str(error))
        except AuthorityError as error:
            fail(400, "request_authority_invalid", str(error))
        except Exception:
            fail(502, "gateway_error", "gateway error")

    def _complete_aws_query_request(
        self, flow: http.HTTPFlow, pending: dict[str, Any]
    ) -> None:
        started = pending["started"]
        principal = pending["principal"]
        credential_request = pending["credential_request"]
        parsed: ParsedAWSRequest | None = None

        def base_event(decision: str, reason: str, learnable: bool) -> dict[str, Any]:
            return {
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "request_id": pending["request_id"], "cluster": self.cluster,
                "project_id": principal["project_id"], "context_id": principal["context_id"],
                "context": principal["context"], "project_root": principal["project_root"],
                "scheme": pending["scheme"], "host": pending["host"], "port": pending["port"],
                "method": flow.request.method.upper(), "path": pending["audit_path"],
                "protocol": "http", "decision": decision, "reason": reason,
                "learnable": learnable, "started": started,
            }

        def fail(status: int, code: str, reason: str, learnable: bool = False) -> None:
            event = base_event("deny", reason, learnable)
            event.pop("started")
            event["upstream_status"] = status
            event["duration_ms"] = int((time.monotonic() - started) * 1000)
            _audit(**(_aws_audit_event(event, parsed) if parsed is not None else event))
            if code == "policy_denied" and parsed is not None:
                _policy_denied(flow, status, learnable, principal, aws=parsed)
            else:
                _deny(flow, status, code)

        try:
            body = flow.request.raw_content
            if not isinstance(body, bytes):
                raise AWSRequestError("aws_operation_invalid", "AWS Query request body must be bytes")
            current_scheme, current_host, current_port = normalize_ingress_authority(flow)
            parsed = parse_aws_query_request(
                pending["classification"], flow.request.method.upper(),
                current_scheme, current_host, current_port,
                urlsplit(flow.request.url).path or "/", urlsplit(flow.request.url).query,
                _request_header_pairs(flow.request), body,
            )
            policy_input = build_policy_input(
                flow, self.cluster, principal, credential_request.secret_headers,
                credential_request.broker_provider, aws=parsed,
            )
            decision = query_opa(self.opa_url, policy_input, self.opa_timeout)
            if not decision.allow:
                fail(decision.status_code, "policy_denied", decision.reason, decision.learnable)
                return
            credential_request.apply(flow.request)
            if bool(getattr(credential_request, "deferred", False)):
                apply_body = getattr(credential_request, "apply_body", None)
                if not callable(apply_body):
                    raise BrokerCredentialUnavailable("deferred credential contract is unavailable")
                apply_body(flow.request)
            commit_upstream_authority(flow)
            flow.metadata["tobari_audit"] = _aws_audit_event(
                base_event("allow", decision.reason, False), parsed
            )
        except AWSRequestError as error:
            fail(400, error.code, str(error))
        except PolicyUnavailable as error:
            fail(503, "policy_unavailable", str(error))
        except CredentialAdapterError as error:
            status, code = _credential_error_response(error, "credential_handle_invalid")
            fail(status, code, str(error))
        except (CredentialError, RuntimeError, UnicodeError, ValueError) as error:
            fail(503, "credential_unavailable", str(error))
        except AuthorityError as error:
            fail(400, "request_authority_invalid", str(error))
        except Exception:
            fail(502, "gateway_error", "gateway error")

    def request(self, flow: http.HTTPFlow) -> None:
        aws_pending = flow.metadata.pop("tobari_aws_query_pending", None)
        if isinstance(aws_pending, dict):
            self._complete_aws_query_request(flow, aws_pending)
            return
        mcp_pending = flow.metadata.pop("tobari_mcp_pending", None)
        if isinstance(mcp_pending, dict):
            self._complete_mcp_request(flow, mcp_pending)
            return
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
        except CredentialAdapterError as error:
            status, code = _credential_error_response(
                error, "broker_signing_request_invalid"
            )
            deny(status, code, str(error))
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
