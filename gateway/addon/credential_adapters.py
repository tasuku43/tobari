"""Pluggable Gateway handling for tool/client authentication."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable, Protocol

from mitmproxy import http


DEFAULT_SECRET_HEADERS = frozenset(
    {
        "authorization",
        "proxy-authorization",
        "cookie",
        "set-cookie",
        "x-api-key",
    }
)
PROFILE_HEADER = "x-tobari-credential-profile"
CONTROL_HEADERS = frozenset({"x-tobari-session", PROFILE_HEADER})


class CredentialAdapterError(Exception):
    """A credential adapter could not prepare or forward a request."""


class PreparedCredentialRequest(Protocol):
    """The request-scoped behavior selected before policy evaluation."""

    requested_profile: str | None
    secret_headers: set[str]

    def apply(self, request: http.Request, selected_profile: str | None) -> str | None:
        """Apply post-allow handling and return the effective profile."""


class CredentialAdapter(Protocol):
    """Small boundary between policy enforcement and auth handling."""

    name: str

    def prepare(
        self, request: http.Request, host: str, project_id: str
    ) -> PreparedCredentialRequest:
        """Prepare request-local auth behavior before policy evaluation."""


@dataclass
class _PassthroughRequest:
    requested_profile: str | None = None
    secret_headers: set[str] | None = None

    def __post_init__(self) -> None:
        if self.secret_headers is None:
            self.secret_headers = set(DEFAULT_SECRET_HEADERS)

    def apply(self, request: http.Request, selected_profile: str | None) -> str | None:
        if selected_profile is not None:
            raise CredentialAdapterError(
                "OPA selected a credential profile for the passthrough adapter"
            )
        for name in {"proxy-authorization"} | set(CONTROL_HEADERS):
            request.headers.pop(name, None)
        return None


class PassthroughCredentialAdapter:
    """Forward tool/client auth after allow without Gateway injection."""

    name = "passthrough"

    def prepare(
        self, request: http.Request, host: str, project_id: str
    ) -> PreparedCredentialRequest:
        del host, project_id
        # A profile selector is a managed-adapter control input. It is redacted
        # and removed rather than being forwarded or interpreted in passthrough.
        return _PassthroughRequest()


@dataclass
class _ManagedRequest:
    config: dict[str, Any]
    requested_profile: str | None
    secret_headers: set[str]
    profile_binding: Callable[[dict[str, Any], str, str], dict[str, Any]]
    injector: Callable[[http.Request, dict[str, Any], str, str, str], None]
    host: str
    project_id: str

    def apply(self, request: http.Request, selected_profile: str | None) -> str | None:
        names = set(DEFAULT_SECRET_HEADERS) | self.secret_headers | set(CONTROL_HEADERS)
        for name in names:
            if name not in {"cookie", "set-cookie"}:
                request.headers.pop(name, None)
        if selected_profile is not None:
            # Validate the OPA-selected value again at the side-effect boundary
            # before opening the secret file or changing the upstream request.
            self.profile_binding(self.config, selected_profile, self.project_id)
            self.injector(
                request,
                self.config,
                selected_profile,
                self.host,
                self.project_id,
            )
        return selected_profile


class ManagedCredentialAdapter:
    """The retained Gateway-managed profile binding/injection implementation."""

    name = "managed"

    def __init__(
        self,
        path: str,
        load_config: Callable[[str], dict[str, Any]],
        configured_secret_headers: Callable[[dict[str, Any]], set[str]],
        profile_binding: Callable[[dict[str, Any], str, str], dict[str, Any]],
        injector: Callable[[http.Request, dict[str, Any], str, str, str], None],
    ) -> None:
        self.path = path
        self.load_config = load_config
        self.configured_secret_headers = configured_secret_headers
        self.profile_binding = profile_binding
        self.injector = injector

    def prepare(
        self, request: http.Request, host: str, project_id: str
    ) -> PreparedCredentialRequest:
        config = self.load_config(self.path)
        requested_profile = request.headers.get(PROFILE_HEADER)
        if requested_profile is not None:
            self.profile_binding(config, requested_profile, project_id)
        return _ManagedRequest(
            config=config,
            requested_profile=requested_profile,
            secret_headers=self.configured_secret_headers(config),
            profile_binding=self.profile_binding,
            injector=self.injector,
            host=host,
            project_id=project_id,
        )


def build_credential_adapter(
    name: str,
    *,
    credential_path: str,
    load_config: Callable[[str], dict[str, Any]],
    configured_secret_headers: Callable[[dict[str, Any]], set[str]],
    profile_binding: Callable[[dict[str, Any], str, str], dict[str, Any]],
    injector: Callable[[http.Request, dict[str, Any], str, str, str], None],
) -> CredentialAdapter:
    if name == PassthroughCredentialAdapter.name:
        return PassthroughCredentialAdapter()
    if name == ManagedCredentialAdapter.name:
        return ManagedCredentialAdapter(
            credential_path,
            load_config,
            configured_secret_headers,
            profile_binding,
            injector,
        )
    raise CredentialAdapterError(f"unknown credential adapter: {name}")
