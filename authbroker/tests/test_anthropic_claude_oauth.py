from __future__ import annotations

import json
import unittest
import urllib.request
from unittest import mock

from authbroker import anthropic_refresh_transport
from authbroker.anthropic_claude_oauth import (
    CLIENT_ID,
    TOKEN_ENDPOINT,
    AnthropicClaudeOAuthError,
    ClaudeOAuthState,
    refresh,
)


DRIVER_REVISION = "a" * 64
NOW = 1_800_000_000
ACCESS_TOKEN = "dummy-access-token"
REFRESH_TOKEN = "dummy-refresh-token"
REFRESH_REQUEST_FIELD = "refresh_" + "token"
FULL_SCOPES = (
    "org:create_api_key",
    "user:profile",
    "user:inference",
    "user:sessions:claude_code",
    "user:mcp_servers",
    "user:file_upload",
)


def state_bytes(
    *,
    access: str = ACCESS_TOKEN,
    expires_at: int = (NOW + 3600) * 1000,
    scopes: tuple[str, ...] = FULL_SCOPES,
) -> bytes:
    document = {
        "schema_version": 1,
        "claude_executable": {
            "image_id": "sha256:" + "b" * 64,
            "path": "/usr/local/bin/claude",
            "sha256": DRIVER_REVISION,
            "version": "2.1.220",
        },
        "session": {
            "access_" + "token": access,
            "refresh_" + "token": REFRESH_TOKEN,
            "expires_at": expires_at,
            "scopes": sorted(scopes),
        },
    }
    return json.dumps(document, separators=(",", ":")).encode("ascii")


class AnthropicClaudeOAuthTests(unittest.TestCase):
    def test_parses_only_canonical_exact_driver_state_and_redacts_repr(self) -> None:
        state = ClaudeOAuthState.parse(
            state_bytes(), driver_revision=DRIVER_REVISION
        )
        self.assertEqual(state.access_token(NOW), ACCESS_TOKEN.encode("ascii"))
        self.assertNotIn("dummy-access", repr(state))
        for encoded, revision in (
            (state_bytes() + b"\n", DRIVER_REVISION),
            (state_bytes(), "c" * 64),
            (
                state_bytes().replace(b"user:file_upload", b"user other"),
                DRIVER_REVISION,
            ),
            (
                state_bytes().replace(b'"scopes":', b'"unknown":true,"scopes":'),
                DRIVER_REVISION,
            ),
            (
                state_bytes().replace(
                    b'"access_token":',
                    b'"access_token":"dummy-duplicate","access_token":',
                    1,
                ),
                DRIVER_REVISION,
            ),
        ):
            with self.subTest(encoded=encoded[-40:], revision=revision):
                with self.assertRaisesRegex(
                    AnthropicClaudeOAuthError, "^anthropic_oauth_state_invalid$"
                ):
                    ClaudeOAuthState.parse(encoded, driver_revision=revision)

    def test_refresh_preserves_dynamic_native_scopes_and_replaces_only_token_state(self) -> None:
        calls: list[tuple[str, str, dict[str, object], float]] = []
        granted_scopes = ("future:capability", "user:inference")

        def request(outbound: urllib.request.Request, timeout: float) -> bytes:
            assert outbound.data is not None
            calls.append(
                (
                    outbound.full_url,
                    outbound.method or "",
                    json.loads(outbound.data.decode("ascii")),
                    timeout,
                )
            )
            return json.dumps(
                {
                    "access_token": "dummy-replacement-access",
                    "refresh_token": "dummy-replacement-refresh",
                    "expires_in": 3600,
                    "refresh_token_expires_in": 7200,
                    "scope": " ".join(sorted(granted_scopes)),
                    "token_type": "Bearer",
                    "account": {
                        "uuid": "account-synthetic",
                        "email_address": "user@example.com",
                    },
                    "organization": {"uuid": "organization-synthetic"},
                },
                separators=(",", ":"),
            ).encode("ascii")

        original = ClaudeOAuthState.parse(
            state_bytes(
                expires_at=(NOW + 60) * 1000, scopes=granted_scopes
            ),
            driver_revision=DRIVER_REVISION,
        )
        token, updated = refresh(original, now=NOW, request=request)
        self.assertEqual(token, b"dummy-replacement-access")
        self.assertEqual(
            calls,
            [
                (
                    TOKEN_ENDPOINT,
                    "POST",
                    {
                        "client_id": CLIENT_ID,
                        "grant_type": "refresh_token",
                        REFRESH_REQUEST_FIELD: REFRESH_TOKEN,
                        "scope": " ".join(sorted(granted_scopes)),
                    },
                    30.0,
                )
            ],
        )
        reparsed = ClaudeOAuthState.parse(
            updated.encode(), driver_revision=DRIVER_REVISION
        )
        self.assertEqual(reparsed.access_token(NOW + 1), token)
        self.assertNotIn(b"account-synthetic", updated.encode())

    def test_refresh_rejects_malformed_response_without_exposing_it(self) -> None:
        state = ClaudeOAuthState.parse(
            state_bytes(), driver_revision=DRIVER_REVISION
        )
        for response in (
            b'{"access_token":"dummy-provider-secret-canary"}',
            b'{"access_token":"dummy-duplicate","access_token":"dummy-provider-secret-canary"}',
            json.dumps(
                {
                    "access_token": "dummy-valid-access",
                    "expires_in": 3600,
                    "scope": " ".join(sorted(FULL_SCOPES)),
                    "token_type": "MAC",
                }
            ).encode("ascii"),
        ):
            with self.assertRaises(AnthropicClaudeOAuthError) as captured:
                refresh(state, now=NOW, request=lambda _request, _timeout: response)
            self.assertEqual(str(captured.exception), "anthropic_oauth_refresh_failed")
            self.assertNotIn("dummy-provider-secret-canary", str(captured.exception))

    def test_refresh_worker_disables_proxy_redirects_and_bounds_target(self) -> None:
        class Response:
            status = 200

            def __enter__(self):
                return self

            def __exit__(self, *_args):
                return False

            def geturl(self):
                return TOKEN_ENDPOINT

            def read(self, _limit):
                return b"{}"

        body = json.dumps(
            {
                "client_id": CLIENT_ID,
                "grant_type": "refresh_token",
                REFRESH_REQUEST_FIELD: REFRESH_TOKEN,
                "scope": " ".join(sorted(FULL_SCOPES)),
            },
            separators=(",", ":"),
            sort_keys=True,
        ).encode("ascii")
        opener = mock.Mock()
        opener.open.return_value = Response()
        with mock.patch.object(
            urllib.request, "build_opener", return_value=opener
        ) as build:
            self.assertEqual(anthropic_refresh_transport.perform(body), b"{}")
        proxy = next(
            item
            for item in build.call_args.args
            if isinstance(item, urllib.request.ProxyHandler)
        )
        redirect = next(
            item
            for item in build.call_args.args
            if isinstance(item, urllib.request.HTTPRedirectHandler)
        )
        self.assertEqual(proxy.proxies, {})
        with self.assertRaises(anthropic_refresh_transport.TransportError):
            redirect.redirect_request(None, None, None, None, None, None)

        unsorted = json.loads(body.decode("ascii"))
        unsorted["scope"] = "user:inference future:capability"
        with self.assertRaises(anthropic_refresh_transport.TransportError):
            anthropic_refresh_transport.perform(
                json.dumps(
                    unsorted, separators=(",", ":"), sort_keys=True
                ).encode("ascii")
            )


if __name__ == "__main__":
    unittest.main()
