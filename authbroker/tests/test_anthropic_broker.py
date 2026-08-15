from __future__ import annotations

import base64
import tempfile
import threading
import unittest
from pathlib import Path

from authbroker.anthropic_claude_oauth import (
    AnthropicClaudeOAuthError,
    ClaudeOAuthState,
)
from authbroker.broker import BrokerError, BrokerState
from authbroker.renewable import ReviewedRenewableSessionDependencies
from authbroker.tests.test_anthropic_claude_oauth import (
    DRIVER_REVISION,
    FULL_SCOPES,
    NOW,
    state_bytes,
)
from authbroker.vault import VaultStore, decode_secret


CONTEXT = "018bcfe5-687b-7000-8000-000000000001"
PROJECT = "018bcfe5-687b-7000-8000-000000000101"
KEY = bytes(range(32))
DRIVER_ID = "anthropic_claude_native_oauth"
ACCOUNT_LABEL = "claude-user-native"


def binding() -> dict[str, object]:
    return {
        "provider_id": "anthropic",
        "target": {"scheme": "https", "host": "api.anthropic.com", "port": 443},
        "source": {"header": "authorization", "format": "bearer"},
        "destination": {
            "header": "authorization",
            "format": "bearer",
            "secret_field": "anthropic_claude_oauth_session",
        },
        "secret_headers": ["authorization"],
    }


def decoded_secret(response: dict[str, object]) -> bytes:
    secret = response["secret"]
    assert isinstance(secret, dict)
    value = secret["value"]
    assert isinstance(value, str)
    return base64.urlsafe_b64decode(value + "=" * (-len(value) % 4))


class AnthropicBrokerTests(unittest.TestCase):
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
        return state.login_anthropic_driver(
            CONTEXT,
            encoded,
            account_label=ACCOUNT_LABEL,
            driver_id=DRIVER_ID,
            driver_revision=DRIVER_REVISION,
        )

    def resolve(self, state: BrokerState, issued: dict[str, object]) -> dict[str, object]:
        return state.resolve(
            issued["handle"],
            CONTEXT,
            PROJECT,
            "anthropic",
            issued["revision"],
            binding()["target"],
            "authorization",
            "bearer",
        )

    def test_unexpired_session_resolves_bearer_without_exposing_native_state(self) -> None:
        encoded = state_bytes(expires_at=(NOW + 3600) * 1000)
        state = self.broker(
            anthropic_refresh=lambda _state, _now: self.fail("unexpected refresh")
        )
        login = self.login(state, encoded)
        issued = state.issue_handle(CONTEXT, PROJECT, "anthropic", [binding()])
        self.assertEqual(issued["oauth_scopes"], sorted(FULL_SCOPES))
        self.assertEqual(issued["claude_subscription_type"], "max")
        self.assertEqual(issued["claude_rate_limit_tier"], "example_claude_tier")
        self.assertNotIn("dummy-access", repr(issued))
        metadata = state.introspect(
            issued["handle"],
            CONTEXT,
            PROJECT,
            "anthropic",
            binding()["target"],
            "authorization",
            "bearer",
        )
        self.assertNotIn("secret", metadata)
        resolved = self.resolve(state, issued)
        self.assertEqual(decoded_secret(resolved), b"dummy-access-token")
        self.assertEqual(resolved["revision"], login["revision"])
        self.assertEqual(resolved["secret_headers"], ["authorization"])

    def test_login_binds_exact_driver_revision_and_account(self) -> None:
        encoded = state_bytes()
        state = self.broker()
        cases = {
            "wrong driver": (ACCOUNT_LABEL, "other-driver", DRIVER_REVISION, "invalid_driver"),
            "wrong revision": (ACCOUNT_LABEL, DRIVER_ID, "c" * 64, "anthropic_oauth_state_invalid"),
            "wrong account": ("other-account", DRIVER_ID, DRIVER_REVISION, "invalid_account_label"),
        }
        for name, (account, driver, revision, fault) in cases.items():
            with self.subTest(name=name), self.assertRaisesRegex(
                BrokerError, f"^{fault}$"
            ):
                state.login_anthropic_driver(
                    CONTEXT,
                    encoded,
                    account_label=account,
                    driver_id=driver,
                    driver_revision=revision,
                )

    def test_concurrent_due_resolves_share_one_refresh_and_persist_state(self) -> None:
        entered = threading.Event()
        release = threading.Event()
        calls = 0
        replacement = state_bytes(
            access="dummy-replacement-access",
            expires_at=(NOW + 3600) * 1000,
        )

        def refresh(
            _original: ClaudeOAuthState, _now: float
        ) -> tuple[bytes, ClaudeOAuthState]:
            nonlocal calls
            calls += 1
            entered.set()
            self.assertTrue(release.wait(2.0))
            return (
                b"dummy-replacement-access",
                ClaudeOAuthState.parse(
                    replacement, driver_revision=DRIVER_REVISION
                ),
            )

        state = self.broker(anthropic_refresh=refresh)
        login = self.login(state, state_bytes(expires_at=(NOW + 60) * 1000))
        issued = state.issue_handle(CONTEXT, PROJECT, "anthropic", [binding()])
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
        record = self.store.load(CONTEXT, KEY)["providers"]["anthropic"]
        self.assertEqual(record["revision"], login["revision"])
        self.assertEqual(record["state_generation"], 1)
        self.assertIsNone(record["refresh_task_digest"])
        self.assertEqual(decode_secret(record["state"]), replacement)

    def test_unknown_refresh_outcome_sets_durable_no_replay_barrier(self) -> None:
        def failed(
            _state: ClaudeOAuthState, _now: float
        ) -> tuple[bytes, ClaudeOAuthState]:
            raise AnthropicClaudeOAuthError("provider-secret-canary")

        state = self.broker(anthropic_refresh=failed)
        self.login(state, state_bytes(expires_at=(NOW + 60) * 1000))
        issued = state.issue_handle(CONTEXT, PROJECT, "anthropic", [binding()])
        with self.assertRaises(BrokerError) as captured:
            self.resolve(state, issued)
        self.assertEqual(str(captured.exception), "companion_outcome_unknown")
        self.assertNotIn("provider-secret-canary", str(captured.exception))
        self.assertEqual(
            state.status(CONTEXT, "anthropic")["state"], "not_configured"
        )


if __name__ == "__main__":
    unittest.main()
