from __future__ import annotations

import base64
import json
import math
import unittest
import urllib.request
from unittest import mock

from authbroker import openai_codex_oauth, openai_refresh_transport
from authbroker.openai_codex_oauth import (
    CLIENT_ID,
    FALLBACK_REFRESH_SECONDS,
    MAX_RESPONSE_BYTES,
    MAX_STATE_BYTES,
    REFRESH_WINDOW_SECONDS,
    TOKEN_ENDPOINT,
    CodexOAuthState,
    OpenAICodexOAuthError,
    refresh,
)


DRIVER_REVISION = "a" * 64
ACCOUNT_ID = "account-synthetic-123"
ACCESS_FIELD = "access_" + "token"
REFRESH_FIELD = "refresh_" + "token"
NOW = 1_800_000_000


def _b64(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def jwt(payload: dict[str, object]) -> str:
    header = json.dumps(
        {"alg": "none", "typ": "JWT"}, separators=(",", ":"), sort_keys=True
    ).encode("ascii")
    encoded_payload = json.dumps(
        payload, separators=(",", ":"), sort_keys=True
    ).encode("ascii")
    return f"{_b64(header)}.{_b64(encoded_payload)}.{_b64(b'synthetic-signature')}"


def id_token(
    account_id: str = ACCOUNT_ID,
    *,
    include_account: bool = True,
    fedramp: object = False,
) -> str:
    claims: dict[str, object] = {"chatgpt_account_is_fedramp": fedramp}
    if include_account:
        claims["chatgpt_account_id"] = account_id
    return jwt({"https://api.openai.com/auth": claims})


def access_token(*, expiration: int | None = None) -> str:
    if expiration is None:
        return "synthetic-opaque-access-token"
    return jwt({"exp": expiration})


def state_bytes(
    *,
    account_id: str = ACCOUNT_ID,
    claim_account_id: str | None = ACCOUNT_ID,
    access: str | None = None,
    last_refresh: str = "2027-01-15T08:00:00Z",
    fedramp: object = False,
    version: str = "0.146.0",
    digest: str = DRIVER_REVISION,
    path: str = "/opt/tobari/bin/codex",
) -> bytes:
    document = {
        "schema_version": 1,
        "codex_executable": {
            "path": path,
            "sha256": digest,
            "version": version,
        },
        "auth": {
            "auth_mode": "chatgpt",
            "OPENAI_API_KEY": None,
            "tokens": {
                "id_token": id_token(
                    claim_account_id or "unused",
                    include_account=claim_account_id is not None,
                    fedramp=fedramp,
                ),
                ACCESS_FIELD: access or access_token(),
                REFRESH_FIELD: "dummy-refresh-token",
                "account_id": account_id,
            },
            "last_refresh": last_refresh,
        },
    }
    encoded = json.dumps(document, ensure_ascii=False, separators=(",", ":"))
    encoded = (
        encoded.replace("&", r"\u0026")
        .replace("<", r"\u003c")
        .replace(">", r"\u003e")
        .replace("\u2028", r"\u2028")
        .replace("\u2029", r"\u2029")
    )
    return encoded.encode("utf-8")


class OpenAICodexOAuthStateTests(unittest.TestCase):
    def test_parses_exact_go_host_encoding_and_redacts_repr(self) -> None:
        encoded = state_bytes()
        state = CodexOAuthState.parse(encoded, driver_revision=DRIVER_REVISION)
        self.assertEqual(state.encode(), encoded)
        self.assertEqual(state.account_id, ACCOUNT_ID)
        self.assertEqual(state.access_token(NOW), b"synthetic-opaque-access-token")
        rendered = repr(state)
        self.assertNotIn("synthetic-opaque-access-token", rendered)
        self.assertNotIn("dummy-refresh-token", rendered)

        unicode_path = "/opt/コーデックス/codex&review"
        unicode_encoded = state_bytes(path=unicode_path)
        unicode_state = CodexOAuthState.parse(
            unicode_encoded, driver_revision=DRIVER_REVISION
        )
        self.assertEqual(unicode_state.document["codex_executable"]["path"], unicode_path)
        self.assertIn(b"\\u0026", unicode_state.encode())
        self.assertIn("コーデックス".encode("utf-8"), unicode_state.encode())

    def test_rejects_noncanonical_state_and_wrong_executable_binding(self) -> None:
        valid = state_bytes()
        invalid = {
            "trailing whitespace": valid + b"\n",
            "oversized": valid + b"x" * MAX_STATE_BYTES,
            "sorted object keys": json.dumps(
                json.loads(valid), separators=(",", ":"), sort_keys=True
            ).encode("ascii"),
            "unclean path": state_bytes(path="/opt/tobari/../bin/codex"),
            "relative path": state_bytes(path="opt/tobari/bin/codex"),
            "double-root path": state_bytes(path="//opt/tobari/bin/codex"),
            "wrong version": state_bytes(version="0.147.0"),
            "uppercase digest": state_bytes(digest="A" * 64),
        }
        for name, encoded in invalid.items():
            with self.subTest(name=name), self.assertRaisesRegex(
                OpenAICodexOAuthError, "^openai_oauth_state_invalid$"
            ):
                CodexOAuthState.parse(encoded, driver_revision=DRIVER_REVISION)
        with self.assertRaisesRegex(
            OpenAICodexOAuthError, "^openai_oauth_state_invalid$"
        ):
            CodexOAuthState.parse(valid, driver_revision="b" * 64)

    def test_requires_matching_namespaced_account_and_rejects_fedramp(self) -> None:
        invalid = {
            "missing account claim": state_bytes(claim_account_id=None),
            "mismatched account": state_bytes(claim_account_id="account-other"),
            "fedramp": state_bytes(fedramp=True),
            "invalid fedramp type": state_bytes(fedramp="false"),
            "invalid account syntax": state_bytes(account_id="account/invalid"),
        }
        for name, encoded in invalid.items():
            with self.subTest(name=name), self.assertRaisesRegex(
                OpenAICodexOAuthError, "^openai_oauth_state_invalid$"
            ):
                CodexOAuthState.parse(encoded, driver_revision=DRIVER_REVISION)

    def test_accepts_zero_through_nine_canonical_fraction_digits(self) -> None:
        fractions = ["", "1", "12", "123", "1234", "12345", "123456", "1234567", "12345678", "123456789"]
        for fraction in fractions:
            timestamp = "2026-08-10T01:02:03" + (f".{fraction}" if fraction else "") + "Z"
            with self.subTest(timestamp=timestamp):
                state = CodexOAuthState.parse(
                    state_bytes(last_refresh=timestamp), driver_revision=DRIVER_REVISION
                )
                self.assertEqual(state.document["auth"]["last_refresh"], timestamp)

    def test_rejects_noncanonical_or_invalid_timestamps(self) -> None:
        for timestamp in (
            "2026-08-10T01:02:03.0Z",
            "2026-08-10T01:02:03.120Z",
            "2026-08-10T01:02:03.1234567890Z",
            "2026-08-10T10:02:03+09:00",
            "2026-02-30T01:02:03Z",
            "2026-08-10T01:02:60Z",
        ):
            with self.subTest(timestamp=timestamp), self.assertRaisesRegex(
                OpenAICodexOAuthError, "^openai_oauth_state_invalid$"
            ):
                CodexOAuthState.parse(
                    state_bytes(last_refresh=timestamp), driver_revision=DRIVER_REVISION
                )

    def test_access_refresh_window_and_opaque_fallback_are_exact(self) -> None:
        jwt_state = CodexOAuthState.parse(
            state_bytes(access=access_token(expiration=NOW + REFRESH_WINDOW_SECONDS)),
            driver_revision=DRIVER_REVISION,
        )
        self.assertIsNone(jwt_state.access_token(NOW))
        fresh_jwt = CodexOAuthState.parse(
            state_bytes(access=access_token(expiration=NOW + REFRESH_WINDOW_SECONDS + 1)),
            driver_revision=DRIVER_REVISION,
        )
        self.assertIsNotNone(fresh_jwt.access_token(NOW))
        expired_jwt = CodexOAuthState.parse(
            state_bytes(access=access_token(expiration=-1)),
            driver_revision=DRIVER_REVISION,
        )
        self.assertIsNone(expired_jwt.access_token(NOW))

        refreshed_at = NOW - FALLBACK_REFRESH_SECONDS
        opaque = CodexOAuthState.parse(
            state_bytes(last_refresh=openai_codex_oauth._format_timestamp(refreshed_at)),
            driver_revision=DRIVER_REVISION,
        )
        self.assertIsNotNone(opaque.access_token(NOW))
        self.assertIsNone(opaque.access_token(NOW + 1))
        for invalid_now in (True, -1, math.nan, math.inf):
            with self.subTest(now=invalid_now), self.assertRaisesRegex(
                OpenAICodexOAuthError, "^openai_oauth_state_invalid$"
            ):
                opaque.access_token(invalid_now)


class OpenAICodexOAuthRefreshTests(unittest.TestCase):
    def state(self, *, access: str | None = None) -> CodexOAuthState:
        return CodexOAuthState.parse(
            state_bytes(access=access), driver_revision=DRIVER_REVISION
        )

    def test_refresh_posts_only_fixed_json_contract_and_preserves_account(self) -> None:
        calls: list[tuple[str, str, dict[str, str], bytes, float]] = []

        def request(outbound: urllib.request.Request, timeout: float) -> bytes:
            calls.append(
                (
                    outbound.full_url,
                    outbound.method or "",
                    {key.lower(): value for key, value in outbound.header_items()},
                    outbound.data,
                    timeout,
                )
            )
            return json.dumps(
                {
                    "id_token": None,
                    ACCESS_FIELD: "dummy-replacement-access-token",
                    REFRESH_FIELD: "dummy-replacement-refresh-token",
                },
                separators=(",", ":"),
            ).encode("ascii")

        token, updated = refresh(self.state(), now=NOW + 0.125, request=request)
        self.assertEqual(token, b"dummy-replacement-access-token")
        self.assertEqual(
            calls,
            [
                (
                    TOKEN_ENDPOINT,
                    "POST",
                    {"content-type": "application/json", "accept": "application/json"},
                    json.dumps(
                        {
                            "client_id": CLIENT_ID,
                            "grant_type": "refresh_token",
                            REFRESH_FIELD: "dummy-refresh-token",
                        },
                        separators=(",", ":"),
                        sort_keys=True,
                    ).encode("ascii"),
                    30.0,
                )
            ],
        )
        self.assertEqual(updated.account_id, ACCOUNT_ID)
        self.assertEqual(updated.document["auth"]["last_refresh"], "2027-01-15T08:00:00.125Z")
        self.assertEqual(
            updated.document["auth"]["tokens"]["refresh_token"],
            "dummy-replacement-refresh-token",
        )
        self.assertEqual(
            list(json.loads(updated.encode())),
            ["schema_version", "codex_executable", "auth"],
        )
        reparsed = CodexOAuthState.parse(
            updated.encode(), driver_revision=DRIVER_REVISION
        )
        self.assertEqual(reparsed.access_token(NOW + 1), token)

    def test_refresh_rejects_account_change_and_unusable_access(self) -> None:
        changed_id = id_token("account-other")
        responses = {
            "changed account": json.dumps(
                {"id_token": changed_id, ACCESS_FIELD: "dummy-replacement-token"},
                separators=(",", ":"),
            ).encode("ascii"),
            "still due": json.dumps(
                {ACCESS_FIELD: access_token(expiration=NOW + REFRESH_WINDOW_SECONDS)},
                separators=(",", ":"),
            ).encode("ascii"),
            "only null": b'{"access_token":null}',
        }
        for name, response in responses.items():
            with self.subTest(name=name), self.assertRaisesRegex(
                OpenAICodexOAuthError, "^openai_oauth_refresh_failed$"
            ):
                refresh(self.state(), now=NOW, request=lambda _request, _timeout: response)

    def test_refresh_rejects_malformed_bounded_responses_without_leaking(self) -> None:
        canary = "provider-secret-canary"
        responses = {
            "empty": b"",
            "not bytes": "not-bytes",
            "oversized": b"x" * (MAX_RESPONSE_BYTES + 1),
            "malformed": ('{"' + ACCESS_FIELD + '":"' + canary).encode("ascii"),
            "duplicate": (
                b'{"access_' b'token":"dummy-one","access_' b'token":"dummy-two"}'
            ),
            "unknown key": (
                b'{"access_' b'token":"dummy-token","scope":"openid"}'
            ),
            "short token": b'{"access_' b'token":"test"}',
        }
        for name, response in responses.items():
            with self.subTest(name=name), self.assertRaises(
                OpenAICodexOAuthError
            ) as captured:
                refresh(self.state(), now=NOW, request=lambda _request, _timeout: response)  # type: ignore[arg-type]
            self.assertEqual(str(captured.exception), "openai_oauth_refresh_failed")
            self.assertNotIn(canary, str(captured.exception))

        with self.assertRaises(OpenAICodexOAuthError) as captured:
            refresh(
                self.state(),
                now=NOW,
                request=lambda _request, _timeout: (_ for _ in ()).throw(RuntimeError(canary)),
            )
        self.assertEqual(str(captured.exception), "openai_oauth_refresh_failed")
        self.assertNotIn(canary, str(captured.exception))

    def test_invalid_time_fails_before_network(self) -> None:
        called = False

        def request(_outbound: urllib.request.Request, _timeout: float) -> bytes:
            nonlocal called
            called = True
            return b'{}'

        with self.assertRaisesRegex(
            OpenAICodexOAuthError, "^openai_oauth_refresh_failed$"
        ):
            refresh(self.state(), now=math.nan, request=request)
        self.assertFalse(called)

    def test_default_transport_uses_fixed_isolated_wall_clock_worker(self) -> None:
        body = json.dumps(
            {
                "client_id": CLIENT_ID,
                "grant_type": "refresh_token",
                REFRESH_FIELD: "dummy-refresh-token",
            },
            separators=(",", ":"),
            sort_keys=True,
        ).encode("ascii")
        request = urllib.request.Request(
            TOKEN_ENDPOINT,
            data=body,
            headers={"Content-Type": "application/json", "Accept": "application/json"},
            method="POST",
        )
        process = mock.Mock()
        process.args = ["fixed-worker"]
        process.returncode = 0
        process.communicate.return_value = (b"{}", None)
        with mock.patch.object(
            openai_codex_oauth.subprocess, "Popen", return_value=process
        ) as spawn:
            self.assertEqual(openai_codex_oauth._default_request(request, 30.0), b"{}")
        argv = spawn.call_args.args[0]
        self.assertEqual(argv[:2], [openai_codex_oauth.sys.executable, "-I"])
        self.assertTrue(argv[2].endswith("/openai_refresh_transport.py"))
        self.assertEqual(
            spawn.call_args.kwargs,
            {
                "stdin": openai_codex_oauth.subprocess.PIPE,
                "stdout": openai_codex_oauth.subprocess.PIPE,
                "stderr": openai_codex_oauth.subprocess.DEVNULL,
                "env": {},
                "close_fds": True,
                "start_new_session": True,
            },
        )
        self.assertEqual(process.communicate.call_args.kwargs["input"], body)
        remaining = process.communicate.call_args.kwargs["timeout"]
        self.assertGreater(remaining, 0)
        self.assertLessEqual(remaining, 30.0)

        timed_out = mock.Mock()
        timed_out.args = ["fixed-worker"]
        timed_out.poll.return_value = None
        timed_out.communicate.side_effect = [
            openai_codex_oauth.subprocess.TimeoutExpired("fixed-worker", 30.0),
            (b"", None),
        ]
        with mock.patch.object(
            openai_codex_oauth.subprocess, "Popen", return_value=timed_out
        ), self.assertRaisesRegex(
            OpenAICodexOAuthError, "^openai_oauth_refresh_failed$"
        ):
            openai_codex_oauth._default_request(request, 30.0)
        timed_out.kill.assert_called_once_with()
        self.assertEqual(timed_out.communicate.call_count, 2)

        wrong = urllib.request.Request(
            "https://example.com/oauth/token",
            data=body,
            headers={"Content-Type": "application/json", "Accept": "application/json"},
            method="POST",
        )
        with self.assertRaisesRegex(
            OpenAICodexOAuthError, "^openai_oauth_refresh_failed$"
        ):
            openai_codex_oauth._default_request(wrong, 30.0)

    def test_refresh_worker_disables_proxy_redirects_and_bounds_response(self) -> None:
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
        body = json.dumps(
            {
                "client_id": CLIENT_ID,
                "grant_type": "refresh_token",
                REFRESH_FIELD: "dummy-refresh-token",
            },
            separators=(",", ":"),
            sort_keys=True,
        ).encode("ascii")
        with mock.patch.object(urllib.request, "build_opener", return_value=opener) as build:
            self.assertEqual(openai_refresh_transport.perform(body), b"{}")
        handlers = build.call_args.args
        proxy = next(item for item in handlers if isinstance(item, urllib.request.ProxyHandler))
        redirect = next(
            item for item in handlers if isinstance(item, urllib.request.HTTPRedirectHandler)
        )
        self.assertEqual(proxy.proxies, {})
        with self.assertRaisesRegex(
            openai_refresh_transport.TransportError, "^$"
        ):
            redirect.redirect_request(None, None, None, None, None, None)
        outbound = opener.open.call_args.args[0]
        self.assertEqual(outbound.full_url, TOKEN_ENDPOINT)
        self.assertEqual(outbound.method, "POST")
        self.assertEqual(outbound.data, body)
        opener.open.assert_called_once_with(
            outbound, timeout=openai_refresh_transport.NETWORK_TIMEOUT_SECONDS
        )

        for malformed in (b"{}", body + b"\n", b"x" * (32 * 1024 + 1)):
            with self.subTest(length=len(malformed)), self.assertRaises(
                openai_refresh_transport.TransportError
            ):
                openai_refresh_transport.perform(malformed)


if __name__ == "__main__":
    unittest.main()
