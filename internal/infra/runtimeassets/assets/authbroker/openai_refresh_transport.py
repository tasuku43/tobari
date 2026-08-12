"""Single-purpose, wall-clock-killable OpenAI refresh transport worker."""

from __future__ import annotations

import json
import ssl
import sys
import urllib.request
from typing import Any


TOKEN_ENDPOINT = "https://auth.openai.com/oauth/token"
CLIENT_ID = "app_EMoamEEZ73f0CkXaXp7hrann"
MAX_REQUEST_BYTES = 32 * 1024
MAX_RESPONSE_BYTES = 64 * 1024
NETWORK_TIMEOUT_SECONDS = 29.0
REFRESH_TOKEN_FIELD = "refresh_" + "token"


class TransportError(Exception):
    """Secret-free worker failure."""


def _request_body(encoded: bytes) -> bytes:
    if not isinstance(encoded, bytes) or not encoded or len(encoded) > MAX_REQUEST_BYTES:
        raise TransportError

    def pairs(values: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in values:
            if key in result:
                raise TransportError
            result[key] = value
        return result

    try:
        document = json.loads(encoded.decode("ascii"), object_pairs_hook=pairs)
    except (TransportError, UnicodeDecodeError, json.JSONDecodeError, ValueError):
        raise TransportError from None
    if (
        not isinstance(document, dict)
        or list(document) != ["client_id", "grant_type", REFRESH_TOKEN_FIELD]
        or document.get("client_id") != CLIENT_ID
        or document.get("grant_type") != "refresh_token"
        or not isinstance(document.get(REFRESH_TOKEN_FIELD), str)
        or not 8 <= len(document[REFRESH_TOKEN_FIELD]) <= 16 * 1024
        or any(
            ord(character) < 0x21 or ord(character) > 0x7E
            for character in document[REFRESH_TOKEN_FIELD]
        )
    ):
        raise TransportError
    canonical = json.dumps(
        document, separators=(",", ":"), sort_keys=True
    ).encode("ascii")
    if canonical != encoded:
        raise TransportError
    return encoded


def perform(encoded: bytes) -> bytes:
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, *_args: Any, **_kwargs: Any) -> None:
            raise TransportError

    body = _request_body(encoded)
    outbound = urllib.request.Request(
        TOKEN_ENDPOINT,
        data=body,
        headers={"Content-Type": "application/json", "Accept": "application/json"},
        method="POST",
    )
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}),
        urllib.request.HTTPSHandler(context=ssl.create_default_context()),
        NoRedirect(),
    )
    try:
        with opener.open(outbound, timeout=NETWORK_TIMEOUT_SECONDS) as response:  # nosec B310 -- fixed endpoint; redirects and ambient proxies are disabled.
            if (
                response.status < 200
                or response.status >= 300
                or response.geturl() != TOKEN_ENDPOINT
            ):
                raise TransportError
            content = response.read(MAX_RESPONSE_BYTES + 1)
    except TransportError:
        raise
    except Exception:
        raise TransportError from None
    if len(content) > MAX_RESPONSE_BYTES:
        raise TransportError
    return content


def main() -> int:
    try:
        if len(sys.argv) != 1:
            return 2
        encoded = sys.stdin.buffer.read(MAX_REQUEST_BYTES + 1)
        content = perform(encoded)
        if sys.stdout.buffer.write(content) != len(content):
            return 1
        sys.stdout.buffer.flush()
        return 0
    except Exception:
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
