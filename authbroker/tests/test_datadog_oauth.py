from __future__ import annotations

import json
import unittest
import urllib.parse
import urllib.request
from unittest import mock

from authbroker import datadog_oauth
from authbroker.datadog_oauth import (
    DatadogOAuthError,
    PupOAuthState,
    TOKEN_ENDPOINT,
    refresh,
)


DRIVER_REVISION = "a" * 64


def state_bytes(*, issued_at: int = 1_800_000_000, expires_in: int = 3600) -> bytes:
    document = {
        "schema_version": 1,
        "site": "datadoghq.com",
        "pup_executable": {"path": "/opt/homebrew/bin/pup", "sha256": DRIVER_REVISION},
        "client": {
            "client_id": "client-example-123",
            "client_name": "datadog-pup-cli",
            "redirect_uris": ["http://127.0.0.1:8000/oauth/callback"],
            "registered_at": 1_799_999_000,
            "site": "datadoghq.com",
        },
        "token": {
            "access_token": "dummy-access-token",
            "refresh_token": "dummy-refresh-token",
            "token_type": "Bearer",
            "expires_in": expires_in,
            "issued_at": issued_at,
            "scope": "dashboards_read metrics_read",
            "client_id": "client-example-123",
        },
    }
    return json.dumps(document, separators=(",", ":"), sort_keys=True).encode("ascii")


class DatadogOAuthTests(unittest.TestCase):
    def test_parses_only_canonical_driver_bound_state_and_redacts_repr(self) -> None:
        state = PupOAuthState.parse(state_bytes(), driver_revision=DRIVER_REVISION)
        self.assertEqual(state.access_token(1_800_000_100), b"dummy-access-token")
        self.assertNotIn("dummy-access-token", repr(state))
        with self.assertRaises(DatadogOAuthError):
            PupOAuthState.parse(state_bytes() + b"\n", driver_revision=DRIVER_REVISION)
        with self.assertRaises(DatadogOAuthError):
            PupOAuthState.parse(state_bytes(), driver_revision="b" * 64)
        with self.assertRaises(DatadogOAuthError):
            PupOAuthState.parse(state_bytes(expires_in=300), driver_revision=DRIVER_REVISION)

    def test_refresh_uses_fixed_endpoint_and_replaces_canonical_state(self) -> None:
        calls: list[tuple[str, str, dict[str, list[str]], float]] = []

        def request(outbound: urllib.request.Request, timeout: float) -> bytes:
            calls.append(
                (
                    outbound.full_url,
                    outbound.method or "",
                    urllib.parse.parse_qs(outbound.data.decode("ascii"), strict_parsing=True),
                    timeout,
                )
            )
            return json.dumps(
                {
                    "access_token": "dummy-replacement-access-token",
                    "refresh_token": "dummy-replacement-refresh-token",
                    "token_type": "Bearer",
                    "expires_in": 3600,
                    "scope": "dashboards_read metrics_read",
                },
                separators=(",", ":"),
                sort_keys=True,
            ).encode("ascii")

        original = PupOAuthState.parse(
            state_bytes(issued_at=1_799_990_000), driver_revision=DRIVER_REVISION
        )
        token, updated = refresh(original, now=1_800_000_000, request=request)
        self.assertEqual(token, b"dummy-replacement-access-token")
        self.assertEqual(
            calls,
            [
                (
                    TOKEN_ENDPOINT,
                    "POST",
                    {
                        "grant_type": ["refresh_token"],
                        "client_id": ["client-example-123"],
                        "refresh_" + "token": ["dummy-refresh-token"],
                    },
                    30.0,
                )
            ],
        )
        reparsed = PupOAuthState.parse(updated.encode(), driver_revision=DRIVER_REVISION)
        self.assertEqual(reparsed.access_token(1_800_000_001), token)

    def test_rejects_malformed_refresh_without_exposing_provider_body(self) -> None:
        state = PupOAuthState.parse(state_bytes(), driver_revision=DRIVER_REVISION)
        with self.assertRaises(DatadogOAuthError) as captured:
            refresh(state, now=1_800_000_000, request=lambda _request, _timeout: b'{"access_token":"dummy"}')
        self.assertEqual(str(captured.exception), "datadog_oauth_refresh_failed")
        with self.assertRaises(DatadogOAuthError) as duplicate:
            refresh(
                state,
                now=1_800_000_000,
                request=lambda _request, _timeout: b'{"access_token":"dummy","access_token":"dummy-other"}',
            )
        self.assertEqual(str(duplicate.exception), "datadog_oauth_refresh_failed")

    def test_default_transport_disables_proxy_and_redirects(self) -> None:
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

        opener = mock.Mock()
        opener.open.return_value = Response()
        request = urllib.request.Request(TOKEN_ENDPOINT, data=b"grant_type=refresh_token")
        with mock.patch.object(urllib.request, "build_opener", return_value=opener) as build:
            self.assertEqual(datadog_oauth._default_request(request, 30.0), b"{}")
        handlers = build.call_args.args
        proxy = next(item for item in handlers if isinstance(item, urllib.request.ProxyHandler))
        redirect = next(
            item for item in handlers if isinstance(item, urllib.request.HTTPRedirectHandler)
        )
        self.assertEqual(proxy.proxies, {})
        with self.assertRaises(DatadogOAuthError):
            redirect.redirect_request(None, None, None, None, None, None)
        opener.open.assert_called_once_with(request, timeout=30.0)


if __name__ == "__main__":
    unittest.main()
