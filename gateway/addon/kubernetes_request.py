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
    kind: str
    verb: str
    dry_run: str | None = None
    group: str | None = None
    version: str | None = None
    resource: str | None = None
    namespace: str | None = None
    name: str | None = None
    subresource: str | None = None
    non_resource_path: str | None = None

    def __post_init__(self) -> None:
        self.validate()

    def validate(self) -> None:
        resource_values = (
            self.group,
            self.version,
            self.resource,
            self.namespace,
            self.name,
            self.subresource,
        )
        if self.kind == "resource":
            if (
                self.verb not in {
                    "get", "list", "watch", "create", "update", "patch",
                    "delete", "deletecollection", "connect",
                }
                or self.dry_run not in {"none", "empty", "all"}
                or self.group is None
                or self.version is None
                or self.resource is None
                or self.non_resource_path is not None
                or any(value is not None and not isinstance(value, str) for value in resource_values)
            ):
                raise KubernetesRequestError("kubernetes_projection_invalid", "Kubernetes resource projection is invalid")
            return
        if self.kind == "non_resource":
            if (
                self.verb != "get"
                or self.dry_run is not None
                or any(value is not None for value in resource_values)
                or self.non_resource_path is None
                or not _valid_non_resource_path(self.non_resource_path)
            ):
                raise KubernetesRequestError("kubernetes_projection_invalid", "Kubernetes non-resource projection is invalid")
            return
        raise KubernetesRequestError("kubernetes_projection_invalid", "Kubernetes request kind is invalid")


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
        or any(
            ord(character) < 32
            or ord(character) == 127
            or character in {"\u2028", "\u2029"}
            for character in decoded
        )
    ):
        raise KubernetesRequestError("kubernetes_path_invalid", f"Kubernetes {label} is invalid")
    return decoded


def _coordinate(value: str) -> str:
    if len(value.encode("utf-8")) > 1024:
        raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes resource coordinate is too long")
    return value


def _canonical_path_segments(path: str) -> list[str] | None:
    if (
        not path.startswith("/")
        or path == "/"
        or path.endswith("/")
        or "//" in path
        or any(character in path for character in "%\\?#")
        or len(path.encode("utf-8")) > 1024
        or any(
            ord(character) < 32
            or ord(character) == 127
            or character in {"\u2028", "\u2029"}
            for character in path
        )
    ):
        return None
    segments = path[1:].split("/")
    try:
        return [_segment(segment, "non-resource path segment") for segment in segments]
    except KubernetesRequestError:
        return None


def _valid_non_resource_path(path: str) -> bool:
    segments = _canonical_path_segments(path)
    if not segments:
        return False
    if segments[0] == "api":
        return len(segments) in {1, 2}
    if segments[0] == "apis":
        return len(segments) in {1, 3}
    return segments[0] in {"healthz", "livez", "openapi", "readyz", "version"}


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


def _resource_projection(path: str) -> dict[str, str | None]:
    if "%" in path or not path.startswith("/") or path == "/" or path.endswith("/") or "//" in path:
        raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes resource path must be canonical")
    raw = path.strip("/").split("/")
    if len(raw) < 3 or raw[0] not in {"api", "apis"}:
        raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes resource path is invalid")
    if raw[0] == "api":
        version = _segment(raw[1], "version")
        parts = raw[2:]
        group = ""
    else:
        if len(raw) < 4:
            raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes grouped resource path is invalid")
        group = _segment(raw[1], "group")
        version = _segment(raw[2], "version")
        parts = raw[3:]
    namespace = None
    # Without discovery, only the unambiguous three-or-more-segment form can
    # select namespace scope. One and two segments identify the real
    # cluster-scoped `namespaces` resource and an optional object name.
    if len(parts) >= 3 and parts[0] == "namespaces":
        namespace = _segment(parts[1], "namespace")
        parts = parts[2:]
    if not 1 <= len(parts) <= 3:
        raise KubernetesRequestError("kubernetes_path_invalid", "Kubernetes resource path depth is unsupported")
    resource = _segment(parts[0], "resource")
    name = _segment(parts[1], "name") if len(parts) >= 2 else None
    subresource = _segment(parts[2], "subresource") if len(parts) == 3 else None
    return {
        "group": group,
        "version": version,
        "resource": resource,
        "namespace": namespace,
        "name": name,
        "subresource": subresource,
    }


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
    claims_non_resource = any(path == prefix or path.startswith(prefix + "/") for prefix in _NON_RESOURCE_PREFIXES)
    if claims_non_resource and _valid_non_resource_path(path):
        if method != "GET" or watch or dry_run != "none":
            raise KubernetesRequestError("kubernetes_non_resource_invalid", "Kubernetes non-resource request is unsupported")
        return ParsedKubernetesRequest(kind="non_resource", verb="get", non_resource_path=_coordinate(path))
    if claims_non_resource and not (path.startswith("/api/") or path.startswith("/apis/")):
        raise KubernetesRequestError("kubernetes_non_resource_invalid", "Kubernetes non-resource path is invalid")

    projection = _resource_projection(path)
    named = projection["name"] is not None
    subresource = projection["subresource"]
    if subresource in _INTERACTIVE_SUBRESOURCES:
        if method not in {"GET", "POST"} or watch or dry_run != "none":
            raise KubernetesRequestError("kubernetes_connect_invalid", "Kubernetes interactive subresource request is invalid")
        return ParsedKubernetesRequest(kind="resource", verb="connect", dry_run="none", **projection)
    if method == "GET":
        if dry_run != "none":
            raise KubernetesRequestError("kubernetes_dry_run_invalid", "Kubernetes read cannot declare dry-run")
        if watch and named:
            raise KubernetesRequestError("kubernetes_watch_invalid", "Kubernetes watch requires a collection resource")
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
    return ParsedKubernetesRequest(kind="resource", verb=verb, dry_run=dry_run, **projection)
