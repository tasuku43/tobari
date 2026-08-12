"""Static Gateway authentication boundary for public V1."""

from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol

from mitmproxy import http

DEFAULT_SECRET_HEADERS = frozenset({
    "authorization", "proxy-authorization", "cookie", "set-cookie", "x-api-key"
})
PROFILE_HEADER = "x-tobari-credential-profile"
CONTROL_HEADERS = frozenset({"x-tobari-session", PROFILE_HEADER})


class CredentialAdapterError(Exception):
    """A request cannot cross the authentication boundary safely."""


class PreparedCredentialRequest(Protocol):
    broker_provider: str | None
    secret_headers: set[str]

    def apply(self, request: http.Request) -> None: ...


class CredentialAdapter(Protocol):
    name: str

    def prepare(
        self, request: http.Request, scheme: str, host: str, port: int,
        context_id: str, project_id: str,
    ) -> PreparedCredentialRequest: ...


@dataclass
class _PassthroughRequest:
    broker_provider: str | None = None
    secret_headers: set[str] | None = None

    def __post_init__(self) -> None:
        if self.secret_headers is None:
            self.secret_headers = set(DEFAULT_SECRET_HEADERS)

    def apply(self, request: http.Request) -> None:
        for name in {"proxy-authorization"} | set(CONTROL_HEADERS):
            request.headers.pop(name, None)


class PassthroughCredentialAdapter:
    name = "passthrough"

    def prepare(
        self, request: http.Request, scheme: str, host: str, port: int,
        context_id: str, project_id: str,
    ) -> PreparedCredentialRequest:
        if PROFILE_HEADER in request.headers:
            request.headers.pop(PROFILE_HEADER, None)
            raise CredentialAdapterError("retired credential profile selector is unsupported")
        del request, scheme, host, port, context_id, project_id
        return _PassthroughRequest()
