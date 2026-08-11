from __future__ import annotations

import base64
import io
import json
import tempfile
import threading
import unittest
from pathlib import Path
from unittest import mock

from authbroker.broker import BrokerError, BrokerState, Dispatcher
from authbroker.control import _parser as control_parser, _request as control_request
from authbroker.datadog_oauth import PupOAuthState
from authbroker.vault import VaultStore, decode_secret, new_record


CONTEXT = "018bcfe5-687b-7000-8000-000000000001"
PROJECT = "018bcfe5-687b-7000-8000-000000000101"
KEY = bytes(range(32))
NOW = 1_800_000_000
DRIVER_REVISION = "a" * 64


class BinaryInput:
    def __init__(self, payload: bytes):
        self.buffer = io.BytesIO(payload)


def state_bytes(*, issued_at: int, access: str = "dummy-access-token") -> bytes:
    return json.dumps(
        {
            "schema_version": 1,
            "site": "datadoghq.com",
            "pup_executable": {"path": "/opt/homebrew/bin/pup", "sha256": DRIVER_REVISION},
            "client": {
                "client_id": "client-example-123",
                "client_name": "datadog-pup-cli",
                "redirect_uris": ["http://127.0.0.1:8000/oauth/callback"],
                "registered_at": NOW - 1000,
                "site": "datadoghq.com",
            },
            "token": {
                "access_" + "token": access,
                "refresh_token": "dummy-refresh-token",
                "token_type": "Bearer",
                "expires_in": 3600,
                "issued_at": issued_at,
                "scope": "dashboards_read metrics_read",
                "client_id": "client-example-123",
            },
        },
        separators=(",", ":"),
        sort_keys=True,
    ).encode("ascii")


def binding() -> dict[str, object]:
    return {
        "provider_id": "datadog",
        "target": {"scheme": "https", "host": "api.datadoghq.com", "port": 443},
        "source": {"header": "authorization", "format": "bearer"},
        "destination": {
            "header": "authorization",
            "format": "bearer",
            "secret_field": "datadog_oauth_session",
        },
        "secret_headers": ["authorization"],
    }


class DatadogBrokerTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.store = VaultStore(Path(self.temporary.name) / "contexts")

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def state(self, **kwargs) -> BrokerState:
        state = BrokerState(self.store, refresh_clock=lambda: float(NOW), **kwargs)
        state.unlock(KEY)
        return state

    def test_control_commits_strict_pup_state_and_unexpired_resolve_skips_refresh(self) -> None:
        encoded = state_bytes(issued_at=NOW - 60)
        args = control_parser().parse_args(
            [
                "login", "--context-id", CONTEXT, "--provider", "datadog",
                "--account-label", "datadog-us1", "--driver-id", "datadog_pup_oauth",
                "--driver-revision", DRIVER_REVISION,
            ]
        )
        with mock.patch("authbroker.control.sys.stdin", BinaryInput(encoded)):
            request, payload = control_request(args)
        state = self.state(datadog_refresh=lambda _state, _now: self.fail("unexpected refresh"))
        result = Dispatcher(state, "control").dispatch(request, payload)
        self.assertEqual(result["provider"], "datadog")
        issued = state.issue_handle(CONTEXT, PROJECT, "datadog", [binding()])
        metadata = state.introspect(
            issued["handle"], CONTEXT, PROJECT, "datadog",
            binding()["target"], "authorization", "bearer",
        )
        self.assertNotIn("secret", metadata)
        resolved = state.resolve(
            issued["handle"], CONTEXT, PROJECT, "datadog", issued["revision"],
            binding()["target"], "authorization", "bearer",
        )
        self.assertEqual(
            base64.urlsafe_b64decode(resolved["secret"]["value"] + "=="),
            b"dummy-access-token",
        )

    def test_due_refresh_is_single_record_state_replacement_without_revision_rotation(self) -> None:
        calls: list[PupOAuthState] = []

        def refresh(original: PupOAuthState, _now: float) -> tuple[bytes, PupOAuthState]:
            calls.append(original)
            updated = PupOAuthState.parse(
                state_bytes(issued_at=NOW, access="dummy-replacement-access-token"),
                driver_revision=DRIVER_REVISION,
            )
            return b"dummy-replacement-access-token", updated

        state = self.state(datadog_refresh=refresh)
        login = state.login_datadog_driver(
            CONTEXT, state_bytes(issued_at=NOW - 4000), account_label="datadog-us1",
            driver_id="datadog_pup_oauth", driver_revision=DRIVER_REVISION,
        )
        issued = state.issue_handle(CONTEXT, PROJECT, "datadog", [binding()])
        resolved = state.resolve(
            issued["handle"], CONTEXT, PROJECT, "datadog", issued["revision"],
            binding()["target"], "authorization", "bearer",
        )
        self.assertEqual(resolved["revision"], login["revision"])
        self.assertEqual(len(calls), 1)
        record = self.store.load(CONTEXT, KEY)["providers"]["datadog"]
        self.assertEqual(record["state_generation"], 1)
        self.assertIsNone(record["refresh_task_digest"])
        self.assertIn(b"dummy-replacement-access-token", decode_secret(record["state"]))

    def test_concurrent_due_resolves_share_one_refresh(self) -> None:
        entered = threading.Event()
        release = threading.Event()
        calls = 0

        def refresh(_original: PupOAuthState, _now: float) -> tuple[bytes, PupOAuthState]:
            nonlocal calls
            calls += 1
            entered.set()
            self.assertTrue(release.wait(2.0))
            updated = PupOAuthState.parse(
                state_bytes(issued_at=NOW, access="dummy-replacement-access-token"),
                driver_revision=DRIVER_REVISION,
            )
            return b"dummy-replacement-access-token", updated

        state = self.state(datadog_refresh=refresh)
        state.login_datadog_driver(
            CONTEXT, state_bytes(issued_at=NOW - 4000), account_label="datadog-us1",
            driver_id="datadog_pup_oauth", driver_revision=DRIVER_REVISION,
        )
        issued = state.issue_handle(CONTEXT, PROJECT, "datadog", [binding()])
        results: list[dict[str, object]] = []
        errors: list[Exception] = []

        def resolve() -> None:
            try:
                results.append(state.resolve(
                    issued["handle"], CONTEXT, PROJECT, "datadog", issued["revision"],
                    binding()["target"], "authorization", "bearer",
                ))
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

    def test_unknown_refresh_outcome_leaves_durable_barrier_and_requires_relogin(self) -> None:
        def failed(_state: PupOAuthState, _now: float) -> tuple[bytes, PupOAuthState]:
            raise RuntimeError("provider response lost")

        state = self.state(datadog_refresh=failed)
        state.login_datadog_driver(
            CONTEXT, state_bytes(issued_at=NOW - 4000), account_label="datadog-us1",
            driver_id="datadog_pup_oauth", driver_revision=DRIVER_REVISION,
        )
        issued = state.issue_handle(CONTEXT, PROJECT, "datadog", [binding()])
        with self.assertRaisesRegex(BrokerError, "^companion_outcome_unknown$"):
            state.resolve(
                issued["handle"], CONTEXT, PROJECT, "datadog", issued["revision"],
                binding()["target"], "authorization", "bearer",
            )
        self.assertEqual(state.status(CONTEXT, "datadog")["state"], "not_configured")
        with self.assertRaisesRegex(BrokerError, "^credential_not_found$"):
            state.issue_handle(CONTEXT, PROJECT, "datadog", [binding()])
        restarted = self.state()
        self.assertEqual(restarted.status(CONTEXT, "datadog")["state"], "not_configured")
        with self.assertRaisesRegex(BrokerError, "^credential_not_found$"):
            restarted.issue_handle(CONTEXT, PROJECT, "datadog", [binding()])

    def test_logout_wins_over_late_refresh_result(self) -> None:
        entered = threading.Event()
        release = threading.Event()

        def refresh(_original: PupOAuthState, _now: float) -> tuple[bytes, PupOAuthState]:
            entered.set()
            self.assertTrue(release.wait(2.0))
            updated = PupOAuthState.parse(
                state_bytes(issued_at=NOW, access="dummy-replacement-access-token"),
                driver_revision=DRIVER_REVISION,
            )
            return b"dummy-replacement-access-token", updated

        state = self.state(datadog_refresh=refresh)
        state.login_datadog_driver(
            CONTEXT, state_bytes(issued_at=NOW - 4000), account_label="datadog-us1",
            driver_id="datadog_pup_oauth", driver_revision=DRIVER_REVISION,
        )
        issued = state.issue_handle(CONTEXT, PROJECT, "datadog", [binding()])
        errors: list[Exception] = []

        def resolve() -> None:
            try:
                state.resolve(
                    issued["handle"], CONTEXT, PROJECT, "datadog", issued["revision"],
                    binding()["target"], "authorization", "bearer",
                )
            except Exception as error:  # pragma: no cover - assertion reports detail
                errors.append(error)

        worker = threading.Thread(target=resolve)
        worker.start()
        self.assertTrue(entered.wait(2.0))
        state.logout(CONTEXT, "datadog")
        release.set()
        worker.join(2.0)
        self.assertFalse(worker.is_alive())
        self.assertEqual([str(error) for error in errors], ["handle_revoked"])
        self.assertEqual(state.status(CONTEXT, "datadog")["state"], "not_configured")


if __name__ == "__main__":
    unittest.main()
