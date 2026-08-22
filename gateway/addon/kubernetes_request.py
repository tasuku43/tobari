"""Bounded Kubernetes API request identity derived without schema discovery."""

from __future__ import annotations

from dataclasses import dataclass
from urllib.parse import parse_qsl, unquote


class KubernetesRequestError(ValueError):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass(frozen=True)
class ParsedKubernetesRequest:
    verb: str
    resource: str
    dry_run: str


_INTERACTIVE_SUBRESOURCES = {"attach", "exec", "portforward", "proxy"}
_NON_RESOURCE_PREFIXES = (
    "/api",
    "/apis",
    "/healthz",
    "/livez",
    "/openapi",
    "/readyz",
    "/version",
)


def _segment(value: str, label: str) -> str:
    decoded = unquote(value)
    if (
        not decoded
        or len(decoded.encode("utf-8")) > 253
        or decoded in {".", ".."}
        or "/" in decoded
        or "\\" in decoded
        or any(ord(character) < 32 or ord(character) == 127 for character in decoded)
    ):
        raise KubernetesRequestError("kubernetes_path_invalid", f"Kubernetes {label} is invalid")
    return decoded


def _coordinate(value: str) -> str:
    if len(value.encode("utf-8")) > 1024:
        raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes resource coordinate is too long")
    return value


def _query_modes(raw_query: str) -> tuple[bool, str]:
    try:
        pairs = parse_qsl(
            raw_query,
            keep_blank_values=True,
            strict_parsing=False,
            encoding="utf-8",
            errors="strict",
            max_num_fields=64,
        )
    except (UnicodeError, ValueError) as error:
        raise KubernetesRequestError("kubernetes_query_invalid", "Kubernetes query is invalid") from error
    watch_values = [value for name, value in pairs if name == "watch"]
    dry_run_values = [value for name, value in pairs if name == "dryRun"]
    if len(watch_values) > 1 or len(dry_run_values) > 1:
        raise KubernetesRequestError("kubernetes_query_ambiguous", "Kubernetes query mode is ambiguous")
    watch = bool(watch_values and watch_values[0].lower() in {"1", "true"})
    if watch_values and not watch:
        if watch_values[0].lower() not in {"0", "false", ""}:
            raise KubernetesRequestError("kubernetes_watch_invalid", "Kubernetes watch mode is invalid")
    if not dry_run_values:
        dry_run = "none"
    elif dry_run_values[0] == "All":
        dry_run = "all"
    elif dry_run_values[0] == "":
        dry_run = "empty"
    else:
        raise KubernetesRequestError("kubernetes_dry_run_invalid", "Kubernetes dry-run mode is invalid")
    return watch, dry_run


def _resource_coordinate(path: str) -> tuple[str, bool, str | None]:
    raw = path.strip("/").split("/")
    if len(raw) < 3 or raw[0] not in {"api", "apis"}:
        raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes resource path is invalid")
    if raw[0] == "api":
        version = _segment(raw[1], "version")
        parts = raw[2:]
        prefix = f"core/{version}"
    else:
        if len(raw) < 4:
            raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes grouped resource path is invalid")
        group = _segment(raw[1], "group")
        version = _segment(raw[2], "version")
        parts = raw[3:]
        prefix = f"{group}/{version}"
    namespace = None
    if parts and parts[0] == "namespaces":
        if len(parts) < 3:
            raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes namespace path is incomplete")
        namespace = _segment(parts[1], "namespace")
        parts = parts[2:]
    if not 1 <= len(parts) <= 3:
        raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes resource path depth is unsupported")
    resource = _segment(parts[0], "resource")
    name = _segment(parts[1], "name") if len(parts) >= 2 else None
    subresource = _segment(parts[2], "subresource") if len(parts) == 3 else None
    coordinate = prefix
    if namespace is not None:
        coordinate += f"/namespaces/{namespace}"
    coordinate += f"/{resource}"
    if name is not None:
        coordinate += f"/{name}"
    if subresource is not None:
        coordinate += f"/{subresource}"
    return _coordinate(coordinate), name is not None, subresource


def parse_kubernetes_request(
    method: str,
    path: str,
    raw_query: str,
    headers: list[tuple[str, str]],
) -> ParsedKubernetesRequest:
    """Return only an exact API verb/resource coordinate; object bodies stay opaque."""

    if any(name.lower().startswith("impersonate-") for name, _ in headers):
        raise KubernetesRequestError(
            "kubernetes_impersonation_unsupported",
            "Kubernetes impersonation is not reviewable by this contract",
        )
    method = method.upper()
    watch, dry_run = _query_modes(raw_query)
    discovery_parts = path.strip("/").split("/")
    api_discovery = (
        len(discovery_parts) == 2 and discovery_parts[0] == "api"
    ) or (
        len(discovery_parts) == 3 and discovery_parts[0] == "apis"
    )
    if any(path == prefix or path.startswith(prefix + "/") for prefix in _NON_RESOURCE_PREFIXES) and not (
        (path.startswith("/api/") or path.startswith("/apis/")) and not api_discovery
    ):
        if method != "GET" or watch or dry_run != "none":
            raise KubernetesRequestError("kubernetes_non_resource_invalid", "Kubernetes non-resource request is unsupported")
        return ParsedKubernetesRequest("get", _coordinate(f"non-resource:{path}"), "none")

    coordinate, named, subresource = _resource_coordinate(path)
    if subresource in _INTERACTIVE_SUBRESOURCES:
        if method not in {"GET", "POST"} or watch or dry_run != "none":
            raise KubernetesRequestError("kubernetes_connect_invalid", "Kubernetes interactive subresource request is invalid")
        return ParsedKubernetesRequest("connect", coordinate, "none")
    if method == "GET":
        if dry_run != "none":
            raise KubernetesRequestError("kubernetes_dry_run_invalid", "Kubernetes read cannot declare dry-run")
        verb = "watch" if watch else ("get" if named else "list")
    elif method == "POST" and not named:
        verb = "create"
    elif method == "PUT" and named:
        verb = "update"
    elif method == "PATCH" and named:
        verb = "patch"
    elif method == "DELETE":
        verb = "delete" if named else "deletecollection"
    else:
        raise KubernetesRequestError("kubernetes_method_invalid", "Kubernetes method/path combination is unsupported")
    if watch and method != "GET":
        raise KubernetesRequestError("kubernetes_watch_invalid", "Kubernetes watch requires GET")
    return ParsedKubernetesRequest(verb, coordinate, dry_run)
