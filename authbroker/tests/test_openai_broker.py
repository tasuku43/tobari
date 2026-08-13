from __future__ import annotations

import base64
import tempfile
import threading
import unittest
from pathlib import Path

from authbroker.broker import BrokerError, BrokerState
from authbroker.openai_codex_oauth import CodexOAuthState, OpenAICodexOAuthError
from authbroker.renewable import ReviewedRenewableSessionDependencies
from authbroker.tests.test_openai_codex_oauth import (
    ACCOUNT_ID,
    DRIVER_REVISION,
    NOW,
    access_token,
    state_bytes,
)
from authbroker.vault import VaultStore, decode_secret


CONTEXT = "018bcfe5-687b-7000-8000-000000000001"
PROJECT = "018bcfe5-687b-7000-8000-000000000101"
KEY = bytes(range(32))
DRIVER_ID = "openai_codex_chatgpt_oauth"


def binding() -> dict[str, object]:
    return {
        "provider_id": "openai",
        "target": {"scheme": "https", "host": "chatgpt.com", "port": 443},
        "source": {"header": "authorization", "format": "bearer"},
        "destination": {
            "header": "authorization",
            "format": "bearer",
            "secret_field": "openai_codex_oauth_session",
        },
        "secret_headers": [
            "authorization",
            "chatgpt-account-id",
            "x-openai-fedramp",
        ],
    }


def decoded_secret(response: dict[str, object]) -> bytes:
    secret = response["secret"]
    assert isinstance(secret, dict)
    value = secret["value"]
    assert isinstance(value, str)
    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


class OpenAIBrokerTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.store = VaultStore(Path(self.temporary.name) / "contexts")

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def broker(self, **kwargs) -> BrokerState:
        state = BrokerState(
            self.store,
            refresh_clock=lambda: float(NOW),
            renewable_dependencies=ReviewedRenewableSessionDependencies(**kwargs),
        )
        state.unlock(KEY)
        return state

    def login(self, state: BrokerState, encoded: bytes) -> dict[str, object]:
        return state.login_openai_driver(
            CONTEXT,
            encoded,
            account_label=ACCOUNT_ID,
            driver_id=DRIVER_ID,
            driver_revision=DRIVER_REVISION,
        )

    def resolve(self, state: BrokerState, issued: dict[str, object]) -> dict[str, object]:
        return state.resolve(
            issued["handle"],
            CONTEXT,
            PROJECT,
            "openai",
            issued["revision"],
            binding()["target"],
            "authorization",
            "bearer",
        )

    def test_unexpired_session_resolves_bearer_and_broker_owned_account_header(self) -> None:
        encoded = state_bytes(access=access_token(expiration=NOW + 3600))
        state = self.broker(
            openai_refresh=lambda _state, _now: self.fail("unexpected refresh")
        )
        login = self.login(state, encoded)
        issued = state.issue_handle(CONTEXT, PROJECT, "openai", [binding()])
        metadata = state.introspect(
            issued["handle"],
            CONTEXT,
            PROJECT,
            "openai",
            binding()["target"],
            "authorization",
            "bearer",
        )
        self.assertNotIn("secret", metadata)
        self.assertNotIn("supplemental_headers", metadata)

        resolved = self.resolve(state, issued)
        self.assertEqual(resolved["revision"], login["revision"])
        self.assertEqual(decoded_secret(resolved), encoded_access(encoded))
        self.assertEqual(
            resolved["supplemental_headers"], {"chatgpt-account-id": ACCOUNT_ID}
        )
        self.assertEqual(
            resolved["secret_headers"],
            ["authorization", "chatgpt-account-id", "x-openai-fedramp"],
        )

    def test_login_binds_exact_driver_revision_and_account(self) -> None:
        encoded = state_bytes(access=access_token(expiration=NOW + 3600))
        state = self.broker()
        cases = {
            "wrong driver": {
                "account_label": ACCOUNT_ID,
                "driver_id": "openai_other_driver",
                "driver_revision": DRIVER_REVISION,
                "fault": "invalid_driver",
            },
            "wrong revision": {
                "account_label": ACCOUNT_ID,
                "driver_id": DRIVER_ID,
                "driver_revision": "b" * 64,
                "fault": "openai_oauth_state_invalid",
            },
            "wrong account": {
                "account_label": "account-other",
                "driver_id": DRIVER_ID,
                "driver_revision": DRIVER_REVISION,
                "fault": "invalid_account_label",
            },
        }
        for name, values in cases.items():
            with self.subTest(name=name), self.assertRaisesRegex(
                BrokerError, f"^{values['fault']}$"
            ):
                state.login_openai_driver(
                    CONTEXT,
                    encoded,
                    account_label=values["account_label"],
                    driver_id=values["driver_id"],
                    driver_revision=values["driver_revision"],
                )

    def test_accepts_full_go_host_account_id_syntax(self) -> None:
        account_id = "org:workspace_1.example"
        encoded = state_bytes(
            account_id=account_id,
            claim_account_id=account_id,
            access=access_token(expiration=NOW + 3600),
        )
        state = self.broker()
        login = state.login_openai_driver(
            CONTEXT,
            encoded,
            account_label=account_id,
            driver_id=DRIVER_ID,
            driver_revision=DRIVER_REVISION,
        )
        self.assertEqual(login["account_label"], account_id)
        self.assertEqual(state.status(CONTEXT, "openai")["account_label"], account_id)

    def test_due_refresh_replaces_state_without_rotating_revision(self) -> None:
        calls: list[CodexOAuthState] = []
        replacement = state_bytes(
            access="synthetic-replacement-access-token",
            last_refresh="2027-01-15T08:00:00Z",
        )

        def refresh(
            original: CodexOAuthState, _now: float
        ) -> tuple[bytes, CodexOAuthState]:
            calls.append(original)
            updated = CodexOAuthState.parse(
                replacement, driver_revision=DRIVER_REVISION
            )
            return b"synthetic-replacement-access-token", updated

        state = self.broker(openai_refresh=refresh)
        login = self.login(
            state,
            state_bytes(last_refresh="2027-01-07T07:59:59Z"),
        )
        issued = state.issue_handle(CONTEXT, PROJECT, "openai", [binding()])
        resolved = self.resolve(state, issued)

        self.assertEqual(len(calls), 1)
        self.assertEqual(decoded_secret(resolved), b"synthetic-replacement-access-token")
        self.assertEqual(
            resolved["supplemental_headers"], {"chatgpt-account-id": ACCOUNT_ID}
        )
        self.assertEqual(resolved["revision"], login["revision"])
        record = self.store.load(CONTEXT, KEY)["providers"]["openai"]
        self.assertEqual(record["revision"], login["revision"])
        self.assertEqual(record["state_generation"], 1)
        self.assertIsNone(record["refresh_task_digest"])
        self.assertEqual(decode_secret(record["state"]), replacement)

    def test_concurrent_due_resolves_share_one_refresh(self) -> None:
        entered = threading.Event()
        release = threading.Event()
        calls = 0
        replacement = state_bytes(
            access="synthetic-replacement-access-token",
            last_refresh="2027-01-15T08:00:00Z",
        )

        def refresh(
            _original: CodexOAuthState, _now: float
        ) -> tuple[bytes, CodexOAuthState]:
            nonlocal calls
            calls += 1
            entered.set()
            self.assertTrue(release.wait(2.0))
            return (
                b"synthetic-replacement-access-token",
                CodexOAuthState.parse(replacement, driver_revision=DRIVER_REVISION),
            )

        state = self.broker(openai_refresh=refresh)
        self.login(state, state_bytes(last_refresh="2027-01-07T07:59:59Z"))
        issued = state.issue_handle(CONTEXT, PROJECT, "openai", [binding()])
        results: list[dict[str, object]] = []
        errors: list[Exception] = []

        def resolve() -> None:
            try:
                results.append(self.resolve(state, issued))
            except Exception as error:  # pragma: no cover - assertion reports detail
                errors.append(error)

        first = threading.Thread(target=resolve)
        second = threading.Thread(target=resolve)
        first.start()
        self.assertTrue(entered.wait(2.0))
        second.start()
        release.set()
        first.join(2.0)
        second.join(2.0)
        self.assertFalse(first.is_alive() or second.is_alive())
        self.assertEqual(errors, [])
        self.assertEqual(len(results), 2)
        self.assertEqual(calls, 1)

    def test_unknown_refresh_outcome_leaves_durable_barrier(self) -> None:
        canary = "provider-secret-canary"

        def failed(
            _state: CodexOAuthState, _now: float
        ) -> tuple[bytes, CodexOAuthState]:
            raise OpenAICodexOAuthError(canary)

        state = self.broker(openai_refresh=failed)
        self.login(state, state_bytes(last_refresh="2027-01-07T07:59:59Z"))
        issued = state.issue_handle(CONTEXT, PROJECT, "openai", [binding()])
        with self.assertRaises(BrokerError) as captured:
            self.resolve(state, issued)
        self.assertEqual(str(captured.exception), "companion_outcome_unknown")
        self.assertNotIn(canary, str(captured.exception))
        self.assertEqual(state.status(CONTEXT, "openai")["state"], "not_configured")
        with self.assertRaisesRegex(BrokerError, "^credential_not_found$"):
            state.issue_handle(CONTEXT, PROJECT, "openai", [binding()])

        restarted = self.broker()
        self.assertEqual(restarted.status(CONTEXT, "openai")["state"], "not_configured")
        with self.assertRaisesRegex(BrokerError, "^credential_not_found$"):
            restarted.issue_handle(CONTEXT, PROJECT, "openai", [binding()])

    def test_logout_wins_over_late_refresh_result(self) -> None:
        entered = threading.Event()
        release = threading.Event()
        replacement = state_bytes(
            access="synthetic-replacement-access-token",
            last_refresh="2027-01-15T08:00:00Z",
        )

        def refresh(
            _original: CodexOAuthState, _now: float
        ) -> tuple[bytes, CodexOAuthState]:
            entered.set()
            self.assertTrue(release.wait(2.0))
            return (
                b"synthetic-replacement-access-token",
                CodexOAuthState.parse(replacement, driver_revision=DRIVER_REVISION),
            )

        state = self.broker(openai_refresh=refresh)
        self.login(state, state_bytes(last_refresh="2027-01-07T07:59:59Z"))
        issued = state.issue_handle(CONTEXT, PROJECT, "openai", [binding()])
        errors: list[Exception] = []

        def resolve() -> None:
            try:
                self.resolve(state, issued)
            except Exception as error:  # pragma: no cover - assertion reports detail
                errors.append(error)

        worker = threading.Thread(target=resolve)
        worker.start()
        self.assertTrue(entered.wait(2.0))
        state.logout(CONTEXT, "openai")
        release.set()
        worker.join(2.0)
        self.assertFalse(worker.is_alive())
        self.assertEqual([str(error) for error in errors], ["handle_revoked"])
        self.assertEqual(state.status(CONTEXT, "openai")["state"], "not_configured")


def encoded_access(encoded: bytes) -> bytes:
    parsed = CodexOAuthState.parse(encoded, driver_revision=DRIVER_REVISION)
    token = parsed.access_token(NOW)
    assert token is not None
    return token


if __name__ == "__main__":
    unittest.main()
