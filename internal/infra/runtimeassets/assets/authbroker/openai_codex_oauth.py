"""Strict Codex 0.146.0 ChatGPT OAuth state and bounded token refresh."""

from __future__ import annotations

import base64
import datetime as dt
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


DRIVER_ID = "openai_codex_chatgpt_oauth"
CODEX_VERSION = "0.146.0"
CLIENT_ID = "app_EMoamEEZ73f0CkXaXp7hrann"
TOKEN_ENDPOINT = "https://auth.openai.com/oauth/token"
MAX_STATE_BYTES = 32 * 1024
MAX_RESPONSE_BYTES = 64 * 1024
REFRESH_WINDOW_SECONDS = 5 * 60
FALLBACK_REFRESH_SECONDS = 8 * 24 * 60 * 60
REFRESH_TOKEN_FIELD = "refresh_" + "token"
_EPOCH = dt.datetime(1970, 1, 1, tzinfo=dt.timezone.utc)
_DIGEST = re.compile(r"^[0-9a-f]{64}$")
_ACCOUNT = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
_TIMESTAMP = re.compile(
    r"^(?P<date>[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2})"
    r"(?:\.(?P<fraction>[0-9]{1,9}))?Z$"
)


class OpenAICodexOAuthError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _strict_json(encoded: bytes, code: str) -> dict[str, Any]:
    def pairs(values: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in values:
            if key in result:
                raise OpenAICodexOAuthError(code)
            result[key] = value
        return result

    def reject_constant(_: str) -> None:
        raise OpenAICodexOAuthError(code)

    try:
        document = json.loads(
            encoded.decode("utf-8"), object_pairs_hook=pairs, parse_constant=reject_constant
        )
    except OpenAICodexOAuthError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError, RecursionError):
        raise OpenAICodexOAuthError(code) from None
    if not isinstance(document, dict):
        raise OpenAICodexOAuthError(code)
    return document


def _canonical(document: dict[str, Any]) -> bytes:
    try:
        encoded = json.dumps(
            document, ensure_ascii=False, allow_nan=False, separators=(",", ":")
        )
        # Match Go's encoding/json output used by the pinned host driver.
        encoded = (
            encoded.replace("&", r"\u0026")
            .replace("<", r"\u003c")
            .replace(">", r"\u003e")
            .replace("\u2028", r"\u2028")
            .replace("\u2029", r"\u2029")
        )
        encoded = encoded.encode("utf-8")
    except (TypeError, ValueError, UnicodeEncodeError, RecursionError):
        raise OpenAICodexOAuthError("openai_oauth_state_invalid") from None
    if not encoded or len(encoded) > MAX_STATE_BYTES:
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    return encoded


def _printable_secret(value: Any) -> bool:
    return (
        isinstance(value, str)
        and 8 <= len(value) <= 16 * 1024
        and all(0x21 <= ord(character) <= 0x7E for character in value)
    )


def _decode_jwt_part(value: str, code: str) -> bytes:
    if "=" in value or re.fullmatch(r"[A-Za-z0-9_-]+", value) is None:
        raise OpenAICodexOAuthError(code)
    try:
        decoded = base64.b64decode(
            value.encode("ascii") + b"=" * (-len(value) % 4),
            altchars=b"-_",
            validate=True,
        )
    except ValueError:
        raise OpenAICodexOAuthError(code) from None
    if (
        not decoded
        or base64.urlsafe_b64encode(decoded).rstrip(b"=").decode("ascii") != value
    ):
        raise OpenAICodexOAuthError(code)
    return decoded


def _jwt_payload(value: str, code: str) -> dict[str, Any]:
    if not _printable_secret(value):
        raise OpenAICodexOAuthError(code)
    pieces = value.split(".")
    if len(pieces) != 3 or any(not piece for piece in pieces):
        raise OpenAICodexOAuthError(code)
    header = _strict_json(_decode_jwt_part(pieces[0], code), code)
    if not isinstance(header, dict):
        raise OpenAICodexOAuthError(code)
    payload = _strict_json(_decode_jwt_part(pieces[1], code), code)
    _decode_jwt_part(pieces[2], code)
    return payload


def _id_token_account(value: str) -> str:
    payload = _jwt_payload(value, "openai_oauth_state_invalid")
    claims = payload.get("https://api.openai.com/auth")
    if not isinstance(claims, dict):
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    fedramp = claims.get("chatgpt_account_is_fedramp", False)
    if not isinstance(fedramp, bool) or fedramp:
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    account = claims.get("chatgpt_account_id")
    if not isinstance(account, str) or _ACCOUNT.fullmatch(account) is None:
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    return account


def _jwt_expiration(value: str) -> int | None:
    try:
        payload = _jwt_payload(value, "openai_oauth_state_invalid")
    except OpenAICodexOAuthError:
        return None
    expiration = payload.get("exp")
    if expiration is None:
        return None
    if (
        isinstance(expiration, bool)
        or not isinstance(expiration, int)
        or expiration < -(1 << 63)
        or expiration > (1 << 63) - 1
    ):
        return None
    return expiration


def _parse_timestamp(value: Any) -> dt.datetime:
    if not isinstance(value, str) or len(value) > 40:
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    matched = _TIMESTAMP.fullmatch(value)
    if matched is None:
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    fraction = matched.group("fraction")
    # time.RFC3339Nano formatting elides a zero fraction and trailing zeros.
    if fraction is not None and fraction.endswith("0"):
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    try:
        parsed = dt.datetime.strptime(matched.group("date"), "%Y-%m-%dT%H:%M:%S").replace(
            microsecond=int(((fraction or "") + "000000")[:6]), tzinfo=dt.timezone.utc
        )
    except ValueError:
        raise OpenAICodexOAuthError("openai_oauth_state_invalid") from None
    return parsed


def _format_timestamp(current: float) -> str:
    if (
        not isinstance(current, (int, float))
        or isinstance(current, bool)
        or not math.isfinite(current)
        or current < 0
    ):
        raise OpenAICodexOAuthError("openai_oauth_state_invalid")
    try:
        value = dt.datetime.fromtimestamp(current, tz=dt.timezone.utc)
    except (OverflowError, OSError, ValueError):
        raise OpenAICodexOAuthError("openai_oauth_state_invalid") from None
    base = value.strftime("%Y-%m-%dT%H:%M:%S")
    fraction = f"{value.microsecond:06d}".rstrip("0")
    return base + (f".{fraction}" if fraction else "") + "Z"


@dataclass(frozen=True, repr=False)
class CodexOAuthState:
    document: dict[str, Any]

    @classmethod
    def parse(
        cls, encoded: bytes, *, driver_revision: str
    ) -> "CodexOAuthState":
        if not isinstance(encoded, bytes) or not encoded or len(encoded) > MAX_STATE_BYTES:
            raise OpenAICodexOAuthError("openai_oauth_state_invalid")
        document = _strict_json(encoded, "openai_oauth_state_invalid")
        if (
            _canonical(document) != encoded
            or list(document) != ["schema_version", "codex_executable", "auth"]
        ):
            raise OpenAICodexOAuthError("openai_oauth_state_invalid")
        executable = document.get("codex_executable")
        auth = document.get("auth")
        if (
            document.get("schema_version") != 1
            or isinstance(document.get("schema_version"), bool)
            or not isinstance(executable, dict)
            or list(executable) != ["path", "sha256", "version"]
            or not isinstance(executable.get("path"), str)
            or not executable["path"].startswith("/")
            or executable["path"].startswith("//")
            or posixpath.normpath(executable["path"]) != executable["path"]
            or not isinstance(executable.get("sha256"), str)
            or not _DIGEST.fullmatch(executable["sha256"])
            or executable.get("version") != CODEX_VERSION
            or not isinstance(driver_revision, str)
            or _DIGEST.fullmatch(driver_revision) is None
            or executable["sha256"] != driver_revision
            or not isinstance(auth, dict)
            or list(auth) != ["auth_mode", "OPENAI_API_KEY", "tokens", "last_refresh"]
            or auth.get("auth_mode") != "chatgpt"
            or auth.get("OPENAI_API_KEY") is not None
        ):
            raise OpenAICodexOAuthError("openai_oauth_state_invalid")
        tokens = auth.get("tokens")
        if not isinstance(tokens, dict) or list(tokens) != [
            "id_token", "access_token", "refresh_token", "account_id"
        ]:
            raise OpenAICodexOAuthError("openai_oauth_state_invalid")
        account = tokens.get("account_id")
        if (
            not isinstance(account, str)
            or _ACCOUNT.fullmatch(account) is None
            or not _printable_secret(tokens.get("id_token"))
            or not _printable_secret(tokens.get("access_token"))
            or not _printable_secret(tokens.get("refresh_token"))
        ):
            raise OpenAICodexOAuthError("openai_oauth_state_invalid")
        token_account = _id_token_account(tokens["id_token"])
        if token_account != account:
            raise OpenAICodexOAuthError("openai_oauth_state_invalid")
        _parse_timestamp(auth.get("last_refresh"))
        return cls(document=document)

    @property
    def account_id(self) -> str:
        return self.document["auth"]["tokens"]["account_id"]

    def access_token(self, now: float) -> bytes | None:
        if (
            not isinstance(now, (int, float))
            or isinstance(now, bool)
            or not math.isfinite(now)
            or now < 0
        ):
            raise OpenAICodexOAuthError("openai_oauth_state_invalid")
        auth = self.document["auth"]
        access = auth["tokens"]["access_token"]
        expiration = _jwt_expiration(access)
        if expiration is not None:
            if now >= expiration - REFRESH_WINDOW_SECONDS:
                return None
        elif now > (
            _parse_timestamp(auth["last_refresh"]) - _EPOCH
        ).total_seconds() + FALLBACK_REFRESH_SECONDS:
            return None
        return access.encode("ascii")

    def encode(self) -> bytes:
        return _canonical(self.document)


def _default_request(request: urllib.request.Request, timeout: float) -> bytes:
    if (
        not isinstance(request, urllib.request.Request)
        or request.full_url != TOKEN_ENDPOINT
        or request.method != "POST"
        or timeout != 30.0
        or not isinstance(request.data, bytes)
        or not request.data
        or len(request.data) > MAX_STATE_BYTES
        or {key.lower(): value for key, value in request.header_items()}
        != {"content-type": "application/json", "accept": "application/json"}
    ):
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    worker = posixpath.join(
        posixpath.dirname(posixpath.realpath(__file__)),
        "openai_refresh_transport.py",
    )
    if (
        not posixpath.isabs(worker)
        or posixpath.basename(worker) != "openai_refresh_transport.py"
        or not posixpath.isabs(sys.executable)
    ):
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    started = time.monotonic()
    process: subprocess.Popen[bytes] | None = None
    try:
        process = subprocess.Popen(  # nosec B603 -- executable and sibling worker path are fixed image-owned values.
            [sys.executable, "-I", worker],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            env={},
            close_fds=True,
            start_new_session=True,
        )
        remaining = timeout - (time.monotonic() - started)
        if remaining <= 0:
            raise subprocess.TimeoutExpired(process.args, timeout)
        content, _ = process.communicate(input=request.data, timeout=remaining)
    except subprocess.TimeoutExpired:
        if process is not None:
            try:
                process.kill()
            except Exception:
                pass
            try:
                process.communicate()
            except Exception:
                pass
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed") from None
    except Exception:
        if process is not None:
            try:
                if process.poll() is None:
                    process.kill()
            except Exception:
                pass
            try:
                process.communicate()
            except Exception:
                pass
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed") from None
    if (
        process.returncode != 0
        or time.monotonic() - started > timeout
        or len(content) > MAX_RESPONSE_BYTES
    ):
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    return content


def refresh(
    state: CodexOAuthState,
    *,
    now: float | None = None,
    request: Callable[[urllib.request.Request, float], bytes] = _default_request,
) -> tuple[bytes, CodexOAuthState]:
    current = time.time() if now is None else now
    if not isinstance(state, CodexOAuthState):
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    try:
        _format_timestamp(current)
        state = CodexOAuthState.parse(
            state.encode(),
            driver_revision=state.document["codex_executable"]["sha256"],
        )
    except Exception:
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed") from None
    auth = state.document["auth"]
    tokens = auth["tokens"]
    body = json.dumps(
        {
            "client_id": CLIENT_ID,
            "grant_type": "refresh_token",
            REFRESH_TOKEN_FIELD: tokens[REFRESH_TOKEN_FIELD],
        },
        separators=(",", ":"),
        sort_keys=True,
    ).encode("ascii")
    outbound = urllib.request.Request(
        TOKEN_ENDPOINT,
        data=body,
        headers={"Content-Type": "application/json", "Accept": "application/json"},
        method="POST",
    )
    try:
        response_bytes = request(outbound, 30.0)
    except OpenAICodexOAuthError:
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed") from None
    except Exception:
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed") from None
    if (
        not isinstance(response_bytes, bytes)
        or not response_bytes
        or len(response_bytes) > MAX_RESPONSE_BYTES
    ):
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    response = _strict_json(response_bytes, "openai_oauth_refresh_failed")
    allowed = {"id_token", "access_token", "refresh_token"}
    if (
        not response
        or not set(response).issubset(allowed)
        or not any(response.get(field) is not None for field in allowed)
    ):
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    replacement = dict(tokens)
    for field in allowed:
        if field in response and response[field] is not None:
            if not _printable_secret(response[field]):
                raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
            replacement[field] = response[field]
    try:
        token_account = _id_token_account(replacement["id_token"])
    except OpenAICodexOAuthError:
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed") from None
    if token_account != state.account_id:
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    replacement["account_id"] = state.account_id
    try:
        updated_document = dict(state.document)
        updated_auth = dict(auth)
        updated_auth["tokens"] = replacement
        updated_auth["last_refresh"] = _format_timestamp(current)
        updated_document["auth"] = updated_auth
        updated = CodexOAuthState.parse(
            _canonical(updated_document),
            driver_revision=state.document["codex_executable"]["sha256"],
        )
        access = updated.access_token(current)
    except Exception:
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed") from None
    if access is None:
        raise OpenAICodexOAuthError("openai_oauth_refresh_failed")
    return access, updated
