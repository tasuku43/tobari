"""Strict Datadog US1 pup OAuth state and bounded token refresh."""

from __future__ import annotations

import json
import re
import ssl
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Callable


PUP_DRIVER_ID = "datadog_pup_oauth"
PUP_SITE = "datadoghq.com"
PUP_CLIENT_NAME = "datadog-pup-cli"
TOKEN_ENDPOINT = "https://api.datadoghq.com/oauth2/v1/token"
MAX_STATE_BYTES = 32 * 1024
MAX_RESPONSE_BYTES = 64 * 1024
REFRESH_WINDOW_SECONDS = 300
_DIGEST = re.compile(r"^[0-9a-f]{64}$")
_CLIENT_ID = re.compile(r"^[A-Za-z0-9._~+/=-]{8,512}$")
_SCOPE = re.compile(r"^[A-Za-z0-9:_-]+(?: [A-Za-z0-9:_-]+)*$")
_REDIRECT = re.compile(
    r"^http://127\.0\.0\.1:(?:8000|8080|8888|9000)/oauth/callback$"
)


class DatadogOAuthError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _strict_json(
    encoded: bytes, error_code: str = "datadog_oauth_state_invalid"
) -> dict[str, Any]:
    def pairs(values: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in values:
            if key in result:
                raise DatadogOAuthError(error_code)
            result[key] = value
        return result

    def reject_constant(_: str) -> None:
        raise DatadogOAuthError(error_code)

    try:
        value = json.loads(
            encoded.decode("utf-8"), object_pairs_hook=pairs, parse_constant=reject_constant
        )
    except DatadogOAuthError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise DatadogOAuthError(error_code) from None
    if not isinstance(value, dict):
        raise DatadogOAuthError(error_code)
    return value


def _canonical(document: dict[str, Any]) -> bytes:
    try:
        encoded = json.dumps(
            document, ensure_ascii=True, allow_nan=False, separators=(",", ":"), sort_keys=True
        ).encode("utf-8")
    except (TypeError, ValueError):
        raise DatadogOAuthError("datadog_oauth_state_invalid") from None
    if not encoded or len(encoded) > MAX_STATE_BYTES:
        raise DatadogOAuthError("datadog_oauth_state_invalid")
    return encoded


def _printable_secret(value: Any) -> bool:
    return (
        isinstance(value, str)
        and 8 <= len(value) <= 16 * 1024
        and all(0x21 <= ord(character) <= 0x7E for character in value)
    )


@dataclass(frozen=True, repr=False)
class PupOAuthState:
    document: dict[str, Any]

    @classmethod
    def parse(cls, encoded: bytes, *, driver_revision: str | None = None) -> "PupOAuthState":
        if not isinstance(encoded, bytes) or not encoded or len(encoded) > MAX_STATE_BYTES:
            raise DatadogOAuthError("datadog_oauth_state_invalid")
        document = _strict_json(encoded)
        if _canonical(document) != encoded or set(document) != {
            "schema_version", "site", "pup_executable", "client", "token"
        }:
            raise DatadogOAuthError("datadog_oauth_state_invalid")
        executable = document.get("pup_executable")
        client = document.get("client")
        token = document.get("token")
        if (
            document.get("schema_version") != 1
            or isinstance(document.get("schema_version"), bool)
            or document.get("site") != PUP_SITE
            or not isinstance(executable, dict)
            or set(executable) != {"path", "sha256"}
            or not isinstance(executable.get("path"), str)
            or not executable["path"].startswith("/")
            or not _DIGEST.fullmatch(executable.get("sha256", ""))
            or (driver_revision is not None and executable["sha256"] != driver_revision)
            or not isinstance(client, dict)
            or set(client) != {"client_id", "client_name", "redirect_uris", "registered_at", "site"}
            or client.get("client_name") != PUP_CLIENT_NAME
            or client.get("site") != PUP_SITE
            or not _CLIENT_ID.fullmatch(client.get("client_id", ""))
            or isinstance(client.get("registered_at"), bool)
            or not isinstance(client.get("registered_at"), int)
            or client["registered_at"] <= 0
            or not isinstance(client.get("redirect_uris"), list)
            or len(client["redirect_uris"]) != 1
            or not isinstance(client["redirect_uris"][0], str)
            or not _REDIRECT.fullmatch(client["redirect_uris"][0])
            or not isinstance(token, dict)
            or set(token) != {"access_token", "refresh_token", "token_type", "expires_in", "issued_at", "scope", "client_id"}
            or token.get("client_id") != client.get("client_id")
            or token.get("token_type") != "Bearer"
            or not _printable_secret(token.get("access_token"))
            or not _printable_secret(token.get("refresh_token"))
            or isinstance(token.get("expires_in"), bool)
            or not isinstance(token.get("expires_in"), int)
            or not REFRESH_WINDOW_SECONDS < token["expires_in"] <= 24 * 60 * 60
            or isinstance(token.get("issued_at"), bool)
            or not isinstance(token.get("issued_at"), int)
            or token["issued_at"] <= 0
            or not isinstance(token.get("scope"), str)
            or not 1 <= len(token["scope"]) <= 16 * 1024
            or not _SCOPE.fullmatch(token["scope"])
        ):
            raise DatadogOAuthError("datadog_oauth_state_invalid")
        return cls(document=document)

    def access_token(self, now: float) -> bytes | None:
        if not isinstance(now, (int, float)) or now < 0:
            raise DatadogOAuthError("datadog_oauth_state_invalid")
        token = self.document["token"]
        if now >= token["issued_at"] + token["expires_in"] - REFRESH_WINDOW_SECONDS:
            return None
        return token["access_token"].encode("ascii")

    def encode(self) -> bytes:
        return _canonical(self.document)


def _default_request(request: urllib.request.Request, timeout: float) -> bytes:
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, *_args: Any, **_kwargs: Any) -> None:
            raise DatadogOAuthError("datadog_oauth_refresh_failed")

    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}),
        urllib.request.HTTPSHandler(context=ssl.create_default_context()),
        NoRedirect(),
    )
    try:
        with opener.open(request, timeout=timeout) as response:  # nosec B310 -- request URL is the fixed TOKEN_ENDPOINT constant; redirects and ambient proxies are disabled.
            if response.status < 200 or response.status >= 300:
                raise DatadogOAuthError("datadog_oauth_refresh_failed")
            if response.geturl() != TOKEN_ENDPOINT:
                raise DatadogOAuthError("datadog_oauth_refresh_failed")
            content = response.read(MAX_RESPONSE_BYTES + 1)
    except DatadogOAuthError:
        raise
    except (OSError, urllib.error.URLError, urllib.error.HTTPError, TimeoutError):
        raise DatadogOAuthError("datadog_oauth_refresh_failed") from None
    if len(content) > MAX_RESPONSE_BYTES:
        raise DatadogOAuthError("datadog_oauth_refresh_failed")
    return content


def refresh(
    state: PupOAuthState,
    *,
    now: float | None = None,
    request: Callable[[urllib.request.Request, float], bytes] = _default_request,
) -> tuple[bytes, PupOAuthState]:
    current = time.time() if now is None else now
    token = state.document["token"]
    form = urllib.parse.urlencode(
        {
            "grant_type": "refresh_token",
            "client_id": state.document["client"]["client_id"],
            "refresh_" + "token": token["refresh_token"],
        }
    ).encode("ascii")
    outbound = urllib.request.Request(
        TOKEN_ENDPOINT,
        data=form,
        headers={"Content-Type": "application/x-www-form-urlencoded", "Accept": "application/json"},
        method="POST",
    )
    response = _strict_json(
        request(outbound, 30.0), error_code="datadog_oauth_refresh_failed"
    )
    if set(response) != {"access_token", "token_type", "expires_in", "refresh_token", "scope"}:
        raise DatadogOAuthError("datadog_oauth_refresh_failed")
    replacement = {
        "access_" + "token": response.get("access_token"),
        "refresh_" + "token": response.get("refresh_token"),
        "token_type": response.get("token_type"),
        "expires_in": response.get("expires_in"),
        "issued_at": int(current),
        "scope": response.get("scope"),
        "client_id": state.document["client"]["client_id"],
    }
    updated_document = dict(state.document)
    updated_document["token"] = replacement
    updated = PupOAuthState.parse(_canonical(updated_document))
    return updated.document["token"]["access_token"].encode("ascii"), updated
