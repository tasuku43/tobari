"""Single-purpose, wall-clock-killable Anthropic refresh transport worker."""

from __future__ import annotations

import json
import ssl
import sys
import urllib.request
from typing import Any

TOKEN_ENDPOINT = "https://platform.claude.com/v1/oauth/token"
CLIENT_ID = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
MAX_REQUEST_BYTES = 32 * 1024
MAX_RESPONSE_BYTES = 64 * 1024


class TransportError(Exception):
    pass


def _valid_scope_string(value: Any) -> bool:
    if not isinstance(value, str) or not value or "  " in value:
        return False
    scopes = value.split(" ")
    if (
        not 1 <= len(scopes) <= 32
        or len(set(scopes)) != len(scopes)
        or scopes != sorted(scopes)
    ):
        return False
    return all(
        1 <= len(scope.encode("utf-8")) <= 128
        and all(
            character == 0x21
            or 0x23 <= character <= 0x5B
            or 0x5D <= character <= 0x7E
            for character in map(ord, scope)
        )
        for scope in scopes
    )


def perform(encoded: bytes) -> bytes:
    try:
        value = json.loads(encoded.decode("ascii"))
    except Exception:
        raise TransportError from None
    if (
        not isinstance(value, dict)
        or set(value) != {"client_id", "grant_type", "refresh_token", "scope"}
        or value.get("client_id") != CLIENT_ID
        or value.get("grant_type") != "refresh_token"
        or not _valid_scope_string(value.get("scope"))
        or not isinstance(value.get("refresh_token"), str)
        or not 8 <= len(value["refresh_token"]) <= 16 * 1024
        or json.dumps(value, separators=(",", ":"), sort_keys=True).encode("ascii") != encoded
    ):
        raise TransportError
    class NoRedirect(urllib.request.HTTPRedirectHandler):
        def redirect_request(self, *_args: Any, **_kwargs: Any) -> None:
            raise TransportError
    request = urllib.request.Request(
        TOKEN_ENDPOINT, data=encoded,
        headers={"Content-Type": "application/json", "Accept": "application/json"}, method="POST",
    )
    opener = urllib.request.build_opener(
        urllib.request.ProxyHandler({}), urllib.request.HTTPSHandler(context=ssl.create_default_context()), NoRedirect(),
    )
    try:
        with opener.open(request, timeout=29.0) as response:  # nosec B310 -- fixed endpoint; redirects/proxies disabled.
            if response.status < 200 or response.status >= 300 or response.geturl() != TOKEN_ENDPOINT:
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
        encoded = sys.stdin.buffer.read(MAX_REQUEST_BYTES + 1)
        content = perform(encoded)
        if sys.stdout.buffer.write(content) != len(content):
            return 1
        return 0
    except Exception:
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
