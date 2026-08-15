"""Strict Claude native-login state and bounded first-party token refresh."""

from __future__ import annotations

import json
import math
import posixpath
import re
import subprocess
import sys
import time
import urllib.request
from dataclasses import dataclass
from typing import Any, Callable


DRIVER_ID = "anthropic_claude_native_oauth"
CLIENT_ID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
TOKEN_ENDPOINT = "https://platform.claude.com/v1/oauth/token"
MAX_STATE_BYTES = 32 * 1024
MAX_RESPONSE_BYTES = 64 * 1024
REFRESH_WINDOW_SECONDS = 5 * 60
_DIGEST = re.compile(r"^[0-9a-f]{64}$")
_IMAGE_ID = re.compile(r"^sha256:[0-9a-f]{64}$")
_ACCESS_RESPONSE_FIELD = "access_" + "token"
_REFRESH_RESPONSE_FIELD = "refresh_" + "token"
_REFRESH_EXPIRES_RESPONSE_FIELD = "refresh_" + "token_expires_in"


class AnthropicClaudeOAuthError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _strict_json(encoded: bytes, code: str) -> dict[str, Any]:
    def pairs(values: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in values:
            if key in result:
                raise AnthropicClaudeOAuthError(code)
            result[key] = value
        return result

    def reject_constant(_: str) -> None:
        raise AnthropicClaudeOAuthError(code)

    try:
        value = json.loads(
            encoded.decode("utf-8"), object_pairs_hook=pairs, parse_constant=reject_constant
        )
    except AnthropicClaudeOAuthError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError, RecursionError):
        raise AnthropicClaudeOAuthError(code) from None
    if not isinstance(value, dict):
        raise AnthropicClaudeOAuthError(code)
    return value


def _canonical(value: dict[str, Any], code: str) -> bytes:
    try:
        encoded = json.dumps(
            value, ensure_ascii=False, allow_nan=False, separators=(",", ":")
        )
        encoded = (
            encoded.replace("&", r"\u0026")
            .replace("<", r"\u003c")
            .replace(">", r"\u003e")
            .replace("\u2028", r"\u2028")
            .replace("\u2029", r"\u2029")
            .encode("utf-8")
        )
    except (TypeError, ValueError, UnicodeEncodeError, RecursionError):
        raise AnthropicClaudeOAuthError(code) from None
    if not encoded or len(encoded) > MAX_STATE_BYTES:
        raise AnthropicClaudeOAuthError(code)
    return encoded


def _secret(value: Any) -> bool:
    return (
        isinstance(value, str)
        and 8 <= len(value) <= 16 * 1024
        and all(0x21 <= ord(character) <= 0x7E for character in value)
    )


def _millis(value: Any) -> bool:
    return (
        isinstance(value, int)
        and not isinstance(value, bool)
        and 946684800000 <= value <= 7258118400000
    )


def _normalize_scopes(value: Any) -> tuple[str, ...]:
    if not isinstance(value, list) or not 1 <= len(value) <= 32:
        raise AnthropicClaudeOAuthError("anthropic_oauth_state_invalid")
    scopes: list[str] = []
    seen: set[str] = set()
    for scope in value:
        if (
            not isinstance(scope, str)
            or not 1 <= len(scope.encode("utf-8")) <= 128
            or any(
                character != 0x21
                and not 0x23 <= character <= 0x5B
                and not 0x5D <= character <= 0x7E
                for character in map(ord, scope)
            )
            or scope in seen
        ):
            raise AnthropicClaudeOAuthError("anthropic_oauth_state_invalid")
        seen.add(scope)
        scopes.append(scope)
    return tuple(sorted(scopes))


def _normalize_scope_string(value: Any) -> tuple[str, ...]:
    if not isinstance(value, str) or not value or "  " in value:
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    try:
        return _normalize_scopes(value.split(" "))
    except AnthropicClaudeOAuthError:
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed") from None


@dataclass(frozen=True, repr=False)
class ClaudeOAuthState:
    document: dict[str, Any]

    @classmethod
    def parse(cls, encoded: bytes, *, driver_revision: str) -> "ClaudeOAuthState":
        code = "anthropic_oauth_state_invalid"
        if not isinstance(encoded, bytes) or not encoded or len(encoded) > MAX_STATE_BYTES:
            raise AnthropicClaudeOAuthError(code)
        document = _strict_json(encoded, code)
        if _canonical(document, code) != encoded or list(document) != [
            "schema_version",
            "claude_executable",
            "session",
        ]:
            raise AnthropicClaudeOAuthError(code)
        executable, session = document.get("claude_executable"), document.get("session")
        if (
            document.get("schema_version") != 1
            or isinstance(document.get("schema_version"), bool)
            or not isinstance(executable, dict)
            or list(executable) != ["image_id", "path", "sha256", "version"]
            or _IMAGE_ID.fullmatch(executable.get("image_id", "")) is None
            or executable.get("path") != "/usr/local/bin/claude"
            or _DIGEST.fullmatch(executable.get("sha256", "")) is None
            or executable.get("sha256") != driver_revision
            or executable.get("version") != "2.1.220"
            or not isinstance(session, dict)
        ):
            raise AnthropicClaudeOAuthError(code)
        expected = [
            "access_token",
            "refresh_token",
            "expires_at",
            "scopes",
        ]
        if list(session) != expected:
            raise AnthropicClaudeOAuthError(code)
        try:
            normalized_scopes = _normalize_scopes(session.get("scopes"))
        except AnthropicClaudeOAuthError:
            raise AnthropicClaudeOAuthError(code) from None
        if (
            not _secret(session.get("access_token"))
            or not _secret(session.get("refresh_token"))
            or not _millis(session.get("expires_at"))
            or session.get("scopes") != list(normalized_scopes)
        ):
            raise AnthropicClaudeOAuthError(code)
        return cls(document=document)

    def access_token(self, now: float) -> bytes | None:
        if (
            not isinstance(now, (int, float))
            or isinstance(now, bool)
            or not math.isfinite(now)
            or now < 0
        ):
            raise AnthropicClaudeOAuthError("anthropic_oauth_state_invalid")
        session = self.document["session"]
        if now * 1000 >= session["expires_at"] - REFRESH_WINDOW_SECONDS * 1000:
            return None
        return session["access_token"].encode("ascii")

    def encode(self) -> bytes:
        return _canonical(self.document, "anthropic_oauth_state_invalid")

    def oauth_scopes(self) -> tuple[str, ...]:
        return tuple(self.document["session"]["scopes"])


def _default_request(request: urllib.request.Request, timeout: float) -> bytes:
    if (
        not isinstance(request, urllib.request.Request)
        or request.full_url != TOKEN_ENDPOINT
        or request.method != "POST"
        or timeout != 30.0
        or not isinstance(request.data, bytes)
        or not request.data
        or len(request.data) > MAX_STATE_BYTES
    ):
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    worker = posixpath.join(posixpath.dirname(posixpath.realpath(__file__)), "anthropic_refresh_transport.py")
    started = time.monotonic()
    process: subprocess.Popen[bytes] | None = None
    try:
        process = subprocess.Popen(  # nosec B603 -- executable and worker are fixed image-owned paths.
            [sys.executable, "-I", worker], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, env={}, close_fds=True, start_new_session=True,
        )
        content, _ = process.communicate(input=request.data, timeout=timeout - (time.monotonic() - started))
    except Exception:
        if process is not None:
            try:
                process.kill()
                process.communicate()
            except Exception:
                pass
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed") from None
    if process.returncode != 0 or len(content) > MAX_RESPONSE_BYTES:
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    return content


def refresh(
    state: ClaudeOAuthState,
    *,
    now: float | None = None,
    request: Callable[[urllib.request.Request, float], bytes] = _default_request,
) -> tuple[bytes, ClaudeOAuthState]:
    current = time.time() if now is None else now
    if not isinstance(state, ClaudeOAuthState):
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    session = state.document["session"]
    body = json.dumps(
        {
            "client_id": CLIENT_ID,
            "grant_type": "refresh_token",
            _REFRESH_RESPONSE_FIELD: session["refresh_token"],
            "scope": " ".join(session["scopes"]),
        },
        separators=(",", ":"),
        sort_keys=True,
    ).encode("ascii")
    outbound = urllib.request.Request(
        TOKEN_ENDPOINT, data=body,
        headers={"Content-Type": "application/json", "Accept": "application/json"}, method="POST",
    )
    try:
        response = _strict_json(request(outbound, 30.0), "anthropic_oauth_refresh_failed")
    except Exception:
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed") from None
    allowed_response_fields = {
        _ACCESS_RESPONSE_FIELD,
        _REFRESH_RESPONSE_FIELD,
        "expires_in",
        _REFRESH_EXPIRES_RESPONSE_FIELD,
        "scope",
        "token_type",
        "account",
        "organization",
    }
    if not set(response).issubset(allowed_response_fields):
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    access = response.get(_ACCESS_RESPONSE_FIELD)
    renewal = response.get(_REFRESH_RESPONSE_FIELD, session["refresh_token"])
    expires_in = response.get("expires_in")
    try:
        scopes = _normalize_scope_string(response.get("scope"))
    except AnthropicClaudeOAuthError:
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed") from None
    token_type = response.get("token_type")
    account = response.get("account")
    organization = response.get("organization")
    if (
        not _secret(access)
        or not _secret(renewal)
        or isinstance(expires_in, bool)
        or not isinstance(expires_in, int)
        or not 1 <= expires_in <= 86400
        or scopes != tuple(session["scopes"])
        or (token_type is not None and token_type != "Bearer")
        or not _valid_optional_identity(account, {"uuid", "email_address"})
        or not _valid_optional_identity(organization, {"uuid"})
    ):
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    updated = json.loads(json.dumps(state.document))
    target = updated["session"]
    target["access_token"] = access
    target["refresh_token"] = renewal
    target["expires_at"] = int(current * 1000) + expires_in * 1000
    refresh_expires = response.get(_REFRESH_EXPIRES_RESPONSE_FIELD)
    if refresh_expires is not None:
        if isinstance(refresh_expires, bool) or not isinstance(refresh_expires, int) or refresh_expires < expires_in:
            raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    parsed = ClaudeOAuthState.parse(
        _canonical(updated, "anthropic_oauth_refresh_failed"),
        driver_revision=updated["claude_executable"]["sha256"],
    )
    secret = parsed.access_token(current)
    if secret is None:
        raise AnthropicClaudeOAuthError("anthropic_oauth_refresh_failed")
    return secret, parsed


def _valid_optional_identity(value: Any, fields: set[str]) -> bool:
    if value is None:
        return True
    return (
        isinstance(value, dict)
        and set(value) == fields
        and all(
            isinstance(item, str)
            and 1 <= len(item) <= 320
            and all(0x20 <= ord(character) <= 0x7E for character in item)
            for item in value.values()
        )
    )
