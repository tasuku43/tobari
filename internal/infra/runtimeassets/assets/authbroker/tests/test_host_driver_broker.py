from __future__ import annotations

import contextlib
import io
import json
import logging
import tempfile
import threading
import time
import unittest
from datetime import datetime, timezone
from pathlib import Path
from unittest import mock

from authbroker.aws_sigv4 import EMPTY_SHA256
from authbroker.broker import AwsRefreshSnapshot, BrokerError, BrokerState, Dispatcher
from authbroker.companion_protocol import (
    CompanionError,
    RefreshRequest,
    RefreshResult,
    RefreshSecret,
    encode_refresh_secret,
)
from authbroker.control import _parser as control_parser, _request as control_request
from authbroker.protocol import ProtocolError
from authbroker.vault import VaultError, VaultStore, decode_secret


CONTEXT = "018bcfe5-687b-7000-8000-000000000001"
CONTEXT_TWO = "018bcfe5-687b-7000-8000-000000000002"
PROJECT = "018bcfe5-687b-7000-8000-000000000101"
PROJECT_TWO = "018bcfe5-687b-7000-8000-000000000102"
KEY = bytes(range(32))
NOW = 1_700_000_000
DRIVER_REVISION = "a" * 64
INITIAL_STATE = b'{"opaque_host_driver_state":"initial-canary"}'
UPDATED_STATE = b'{"opaque_host_driver_state":"updated-canary"}'
ACCESS_KEY = "ASIAEXAMPLE000000"
SECRET_KEY = "temporary-secret-key-value"
SESSION_TOKEN = "temporary-session-token-value"


def aws_binding() -> dict[str, object]:
    return {
        "provider_id": "aws",
        "kind": "aws_sigv4",
        "aws_sigv4": {
            "target": {
                "scheme": "https",
                "port": 443,
                "dns_suffixes": ["amazonaws.com"],
            },
            "source": {
                "authorization_header": "authorization",
                "security_token_header": "x-amz-security-token",
            },
            "secret_headers": ["authorization", "x-amz-security-token"],
        },
    }


def signing_request(
    host: str = "sts.us-east-1.amazonaws.com",
) -> dict[str, object]:
    return {
        "host": host,
        "method": "POST",
        "path": "/",
        "query": "",
        "region": "us-east-1",
        "service": "sts",
        "headers": [["content-type", "application/x-www-form-urlencoded"]],
        "payload_hash": EMPTY_SHA256,
    }


def successful_result(request: RefreshRequest, state: bytes = UPDATED_STATE) -> RefreshResult:
    payload = encode_refresh_secret(
        RefreshSecret(
            state=state,
            access_key_id=ACCESS_KEY,
            secret_access_key=SECRET_KEY,
            session_token=SESSION_TOKEN,
            expiration_unix_ms=(NOW + 3600) * 1000,
        )
    )
    return RefreshResult(
        request_id=request.request_id,
        task_digest=request.task_digest,
        state_generation=request.state_generation,
        secret_payload=payload,
    )


class FakeCompanion:
    def __init__(self, callback=None):
        self.callback = callback or successful_result
        self.calls: list[RefreshRequest] = []
        self.invalidations = 0

    def invalidate(self) -> None:
        self.invalidations += 1

    def refresh(self, request: RefreshRequest, _cancel_event=None) -> RefreshResult:
        self.calls.append(request)
        return self.callback(request)


class BinaryInput:
    def __init__(self, payload: bytes):
        self.buffer = io.BytesIO(payload)


class HostCompletedLoginTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.store = VaultStore(Path(self.temporary.name) / "contexts")
        self.state = BrokerState(self.store, companion=FakeCompanion())
        self.state.unlock(KEY)
        self.dispatcher = Dispatcher(self.state, "control")

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def test_control_cli_builds_exact_provider_specific_raw_requests(self) -> None:
        github_args = control_parser().parse_args(
            [
                "login",
                "--context-id",
                CONTEXT,
                "--provider",
                "github",
                "--account-label",
                "octo-user",
            ]
        )
        with mock.patch("authbroker.control.sys.stdin", BinaryInput(b"github-token")):
            github, payload = control_request(github_args)
        self.assertEqual(payload, b"github-token")
        self.assertEqual(
            github,
            {
                "schema_version": 1,
                "op": "login",
                "context_id": CONTEXT,
                "provider": "github",
                "account_label": "octo-user",
                "secret_length": 12,
            },
        )

        anthropic_args = control_parser().parse_args(
            [
                "login",
                "--context-id",
                CONTEXT,
                "--provider",
                "anthropic",
                "--account-label",
                "claude-user-inference",
            ]
        )
        with mock.patch(
            "authbroker.control.sys.stdin", BinaryInput(b"claude-token")
        ):
            anthropic, payload = control_request(anthropic_args)
        self.assertEqual(payload, b"claude-token")
        self.assertEqual(anthropic["provider"], "anthropic")
        self.assertEqual(anthropic["account_label"], "claude-user-inference")
        self.assertEqual(anthropic["secret_length"], len(payload))

        aws_args = control_parser().parse_args(
            [
                "login",
                "--context-id",
                CONTEXT,
                "--provider",
                "aws",
                "--account-label",
                "123456789012",
                "--driver-id",
                "aws_cli_sso",
                "--driver-revision",
                DRIVER_REVISION,
            ]
        )
        with mock.patch("authbroker.control.sys.stdin", BinaryInput(INITIAL_STATE)):
            aws, payload = control_request(aws_args)
        self.assertEqual(payload, INITIAL_STATE)
        self.assertEqual(aws["state_length"], len(INITIAL_STATE))
        self.assertNotIn("secret_length", aws)
        self.assertEqual(aws["driver_id"], "aws_cli_sso")
        self.assertEqual(aws["driver_revision"], DRIVER_REVISION)

    def test_control_cli_never_runs_a_provider_and_rejects_cross_provider_fields(self) -> None:
        arguments = control_parser().parse_args(
            [
                "login",
                "--context-id",
                CONTEXT,
                "--provider",
                "github",
                "--account-label",
                "octo-user",
                "--driver-id",
                "aws_cli_sso",
                "--driver-revision",
                DRIVER_REVISION,
            ]
        )
        with (
            mock.patch("authbroker.control.sys.stdin", BinaryInput(b"token")),
            self.assertRaisesRegex(ProtocolError, "^invalid_request$"),
        ):
            control_request(arguments)

        wrong_anthropic = control_parser().parse_args(
            [
                "login",
                "--context-id",
                CONTEXT,
                "--provider",
                "anthropic",
                "--account-label",
                "arbitrary-account",
            ]
        )
        with (
            mock.patch("authbroker.control.sys.stdin", BinaryInput(b"token")),
            self.assertRaisesRegex(ProtocolError, "^invalid_request$"),
        ):
            control_request(wrong_anthropic)

    def test_dispatcher_commits_github_token_and_opaque_aws_driver_state(self) -> None:
        github = self.dispatcher.dispatch(
            {
                "schema_version": 1,
                "op": "login",
                "context_id": CONTEXT,
                "provider": "github",
                "account_label": "octo-user",
                "secret_length": 12,
            },
            b"github-token",
        )
        self.assertEqual(github["account_label"], "octo-user")

        anthropic = self.dispatcher.dispatch(
            {
                "schema_version": 1,
                "op": "login",
                "context_id": CONTEXT,
                "provider": "anthropic",
                "account_label": "claude-user-inference",
                "secret_length": len(b"claude-token"),
            },
            b"claude-token",
        )
        self.assertEqual(anthropic["account_label"], "claude-user-inference")
        self.assertEqual(
            decode_secret(
                self.store.load(CONTEXT, KEY)["providers"]["anthropic"]["secret"]
            ),
            b"claude-token",
        )

        invalid_anthropic = {
            "schema_version": 1,
            "op": "login",
            "context_id": CONTEXT,
            "provider": "anthropic",
            "account_label": "arbitrary-account",
            "secret_length": len(b"claude-token"),
        }
        with self.assertRaisesRegex(BrokerError, "^invalid_request$"):
            self.dispatcher.dispatch(invalid_anthropic, b"claude-token")

        aws = self.dispatcher.dispatch(
            {
                "schema_version": 1,
                "op": "login",
                "context_id": CONTEXT,
                "provider": "aws",
                "account_label": "123456789012",
                "driver_id": "aws_cli_sso",
                "driver_revision": DRIVER_REVISION,
                "state_length": len(INITIAL_STATE),
            },
            INITIAL_STATE,
        )
        self.assertEqual(aws["account_label"], "123456789012")
        record = self.store.load(CONTEXT, KEY)["providers"]["aws"]
        self.assertEqual(record["driver_id"], "aws_cli_sso")
        self.assertEqual(record["driver_revision"], DRIVER_REVISION)
        self.assertEqual(record["state_generation"], 0)
        self.assertIsNone(record["refresh_task_digest"])
        self.assertEqual(decode_secret(record["state"]), INITIAL_STATE)

    def test_console_login_driver_is_distinct_and_remains_opaque(self) -> None:
        arguments = control_parser().parse_args(
            [
                "login",
                "--context-id",
                CONTEXT,
                "--provider",
                "aws",
                "--account-label",
                "123456789012",
                "--driver-id",
                "aws_cli_console_login",
                "--driver-revision",
                DRIVER_REVISION,
            ]
        )
        with mock.patch("authbroker.control.sys.stdin", BinaryInput(INITIAL_STATE)):
            request, payload = control_request(arguments)
        self.assertEqual(request["driver_id"], "aws_cli_console_login")
        self.assertEqual(payload, INITIAL_STATE)

        result = self.dispatcher.dispatch(request, payload)
        self.assertEqual(result["account_label"], "123456789012")
        record = self.store.load(CONTEXT, KEY)["providers"]["aws"]
        self.assertEqual(record["driver_id"], "aws_cli_console_login")
        self.assertEqual(decode_secret(record["state"]), INITIAL_STATE)

    def test_aws_login_shape_and_metadata_are_strict(self) -> None:
        base = {
            "schema_version": 1,
            "op": "login",
            "context_id": CONTEXT,
            "provider": "aws",
            "account_label": "123456789012",
            "driver_id": "aws_cli_sso",
            "driver_revision": DRIVER_REVISION,
            "state_length": len(INITIAL_STATE),
        }
        invalid = (
            ({**base, "secret_length": len(INITIAL_STATE)}, "invalid_request"),
            ({key: value for key, value in base.items() if key != "account_label"}, "invalid_request"),
            ({**base, "account_label": "not-an-account"}, "invalid_account_label"),
            ({**base, "driver_id": "arbitrary"}, "invalid_driver"),
            ({**base, "driver_revision": "A" * 64}, "invalid_driver_revision"),
        )
        for request, code in invalid:
            with self.subTest(code=code, request=request):
                with self.assertRaisesRegex(BrokerError, f"^{code}$"):
                    self.dispatcher.dispatch(request, INITIAL_STATE)

        self.assertEqual(self.dispatcher.expected_raw_length(base), len(INITIAL_STATE))
        with self.assertRaisesRegex(BrokerError, "^invalid_length$"):
            self.dispatcher.expected_raw_length({**base, "state_length": 0})

    def test_vault_rejects_driver_metadata_or_generation_tampering(self) -> None:
        self.dispatcher.dispatch(
            {
                "schema_version": 1,
                "op": "login",
                "context_id": CONTEXT,
                "provider": "aws",
                "account_label": "123456789012",
                "driver_id": "aws_cli_sso",
                "driver_revision": DRIVER_REVISION,
                "state_length": len(INITIAL_STATE),
            },
            INITIAL_STATE,
        )
        payload = self.store.load(CONTEXT, KEY)
        for field, value in (
            ("driver_id", "arbitrary"),
            ("driver_revision", "A" * 64),
            ("state_generation", -1),
            ("state_generation", True),
            ("refresh_task_digest", "not-a-task-digest"),
            ("refresh_task_digest", True),
        ):
            tampered = {
                "schema_version": payload["schema_version"],
                "providers": {key: dict(record) for key, record in payload["providers"].items()},
            }
            tampered["providers"]["aws"][field] = value
            with self.subTest(field=field, value=value):
                with self.assertRaisesRegex(VaultError, "^vault_invalid$"):
                    self.store.save(CONTEXT, KEY, tampered)


class HostRefreshSigningTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.store = VaultStore(Path(self.temporary.name) / "contexts")
        self.companion = FakeCompanion()
        self.state = BrokerState(
            self.store,
            companion=self.companion,
            refresh_clock=lambda: float(NOW),
            sigv4_clock=lambda: datetime(
                2023, 11, 14, 22, 13, 20, tzinfo=timezone.utc
            ),
        )
        self.state.unlock(KEY)
        self.login = self.state.login_aws_driver(
            CONTEXT,
            INITIAL_STATE,
            account_label="123456789012",
            driver_id="aws_cli_sso",
            driver_revision=DRIVER_REVISION,
        )
        self.issued = self.state.issue_handle(CONTEXT, PROJECT, "aws", [aws_binding()])

    def tearDown(self) -> None:
        self.temporary.cleanup()

    def sign(self) -> dict[str, object]:
        return self.state.sign_sigv4(
            self.issued["handle"],
            CONTEXT,
            PROJECT,
            "aws",
            self.issued["revision"],
            aws_binding(),
            signing_request(),
        )

    def test_synthetic_refresh_persists_only_opaque_state_then_signs(self) -> None:
        metadata = self.state.introspect_signing(
            self.issued["handle"],
            CONTEXT,
            PROJECT,
            "aws",
            {"scheme": "https", "host": "sts.us-east-1.amazonaws.com", "port": 443},
            aws_binding(),
        )
        self.assertEqual(metadata["revision"], self.login["revision"])
        signed = self.sign()
        self.assertEqual(signed["revision"], self.login["revision"])
        self.assertTrue(signed["headers"]["authorization"].startswith("AWS4-HMAC-SHA256 "))
        self.assertEqual(signed["headers"]["x_amz_security_token"], SESSION_TOKEN)

        self.assertEqual(len(self.companion.calls), 1)
        request = self.companion.calls[0]
        self.assertEqual(request.context_id, CONTEXT)
        self.assertEqual(request.project_id, PROJECT)
        self.assertEqual(request.provider, "aws")
        self.assertEqual(request.grant_revision, self.login["revision"])
        self.assertEqual(request.state_generation, 0)
        self.assertEqual(request.driver_id, "aws_cli_sso")
        self.assertEqual(request.driver_revision, DRIVER_REVISION)
        self.assertEqual(request.state, INITIAL_STATE)
        self.assertRegex(request.binding_digest, r"^[0-9a-f]{64}$")
        self.assertRegex(request.request_digest, r"^[0-9a-f]{64}$")
        self.assertNotIn(INITIAL_STATE.decode(), repr(request))

        record = self.store.load(CONTEXT, KEY)["providers"]["aws"]
        self.assertEqual(record["revision"], self.login["revision"])
        self.assertEqual(record["state_generation"], 1)
        self.assertEqual(decode_secret(record["state"]), UPDATED_STATE)
        serialized = json.dumps(record)
        self.assertNotIn(SECRET_KEY, serialized)
        self.assertNotIn(SESSION_TOKEN, serialized)

    def test_invalid_request_revision_binding_and_target_fail_before_companion(self) -> None:
        with self.assertRaisesRegex(BrokerError, "^aws_signing_request_invalid$"):
            self.state.sign_sigv4(
                "not-a-handle",
                CONTEXT,
                PROJECT,
                "aws",
                self.issued["revision"],
                aws_binding(),
                {**signing_request(), "extra": "forbidden"},
            )
        cases = (
            ("revision_wrong", aws_binding(), signing_request()),
            (
                self.issued["revision"],
                {**aws_binding(), "provider_id": "github"},
                signing_request(),
            ),
            (
                self.issued["revision"],
                aws_binding(),
                signing_request("sts.us-east-1.amazonaws.com.attacker.example"),
            ),
            (
                self.issued["revision"],
                aws_binding(),
                {**signing_request(), "extra": "forbidden"},
            ),
        )
        for revision, binding, request in cases:
            with self.subTest(revision=revision, request=request):
                with self.assertRaises(BrokerError):
                    self.state.sign_sigv4(
                        self.issued["handle"],
                        CONTEXT,
                        PROJECT,
                        "aws",
                        revision,
                        binding,
                        request,
                    )
        self.assertEqual(self.companion.calls, [])

    def test_runtime_sign_dispatch_is_payload_free_and_exact(self) -> None:
        dispatcher = Dispatcher(self.state, "runtime")
        request = {
            "schema_version": 1,
            "op": "sign_sigv4",
            "handle": self.issued["handle"],
            "context_id": CONTEXT,
            "project_id": PROJECT,
            "provider": "aws",
            "revision": self.issued["revision"],
            "binding": aws_binding(),
            "request": signing_request(),
        }
        with self.assertRaisesRegex(BrokerError, "^unexpected_payload$"):
            dispatcher.dispatch(request, b"raw-body-is-never-a-signing-input")
        with self.assertRaisesRegex(BrokerError, "^invalid_request$"):
            dispatcher.dispatch({**request, "body": "forbidden"}, b"")
        self.assertEqual(self.companion.calls, [])

    def test_per_record_refresh_is_single_flight_without_global_lock(self) -> None:
        second_issued = self.state.issue_handle(
            CONTEXT, PROJECT_TWO, "aws", [aws_binding()]
        )
        first_started = threading.Event()
        release_first = threading.Event()
        active_lock = threading.Lock()
        active = 0
        maximum_active = 0
        generations: list[int] = []

        def callback(request: RefreshRequest) -> RefreshResult:
            nonlocal active, maximum_active
            with active_lock:
                active += 1
                maximum_active = max(maximum_active, active)
                generations.append(request.state_generation)
            if request.state_generation == 0:
                first_started.set()
                if not release_first.wait(5):
                    raise AssertionError("first refresh was not released")
            result = successful_result(
                request,
                UPDATED_STATE + str(request.state_generation).encode("ascii"),
            )
            with active_lock:
                active -= 1
            return result

        self.companion.callback = callback
        results: list[dict[str, object]] = []
        errors: list[Exception] = []

        def worker(handle: str, project: str) -> None:
            try:
                results.append(
                    self.state.sign_sigv4(
                        handle,
                        CONTEXT,
                        project,
                        "aws",
                        self.issued["revision"],
                        aws_binding(),
                        signing_request(),
                    )
                )
            except Exception as error:  # pragma: no cover - asserted below
                errors.append(error)

        first = threading.Thread(
            target=worker, args=(self.issued["handle"], PROJECT)
        )
        second = threading.Thread(
            target=worker, args=(second_issued["handle"], PROJECT_TWO)
        )
        first.start()
        self.assertTrue(first_started.wait(5))
        second.start()

        # Status uses the global state mutex and must remain available while
        # the trusted-host refresh is blocked.
        self.assertEqual(self.state.status(CONTEXT, "aws")["state"], "ready")
        release_first.set()
        first.join(5)
        second.join(5)
        self.assertFalse(first.is_alive())
        self.assertFalse(second.is_alive())
        self.assertEqual(errors, [])
        self.assertEqual(len(results), 2)
        self.assertEqual(maximum_active, 1)
        self.assertEqual(generations, [0, 1])
        record = self.store.load(CONTEXT, KEY)["providers"]["aws"]
        self.assertEqual(record["state_generation"], 2)

    def test_same_record_wait_queue_is_bounded_before_host_execution(self) -> None:
        started = threading.Event()
        release = threading.Event()

        def callback(request: RefreshRequest) -> RefreshResult:
            started.set()
            if not release.wait(5):
                raise AssertionError("refresh was not released")
            return successful_result(request)

        companion = FakeCompanion(callback)
        state = BrokerState(
            self.store,
            companion=companion,
            refresh_clock=lambda: float(NOW),
            sigv4_clock=lambda: datetime(
                2023, 11, 14, 22, 13, 20, tzinfo=timezone.utc
            ),
            record_lock_timeout=0.05,
        )
        state.unlock(KEY)
        issued = state.issue_handle(CONTEXT, PROJECT, "aws", [aws_binding()])
        first_errors: list[Exception] = []

        def first_call() -> None:
            try:
                state.sign_sigv4(
                    issued["handle"], CONTEXT, PROJECT, "aws",
                    issued["revision"], aws_binding(), signing_request(),
                )
            except Exception as error:  # pragma: no cover - asserted below
                first_errors.append(error)

        first = threading.Thread(target=first_call)
        first.start()
        self.assertTrue(started.wait(5))
        began = time.monotonic()
        with self.assertRaisesRegex(BrokerError, "^companion_busy$"):
            state.sign_sigv4(
                issued["handle"], CONTEXT, PROJECT, "aws",
                issued["revision"], aws_binding(), signing_request(),
            )
        self.assertLess(time.monotonic() - began, 0.5)
        self.assertEqual(len(companion.calls), 1)
        release.set()
        first.join(5)
        self.assertFalse(first.is_alive())
        self.assertEqual(first_errors, [])

    def test_logout_wins_over_late_refresh_result(self) -> None:
        started = threading.Event()
        release = threading.Event()

        def callback(request: RefreshRequest) -> RefreshResult:
            started.set()
            if not release.wait(5):
                raise AssertionError("refresh was not released")
            return successful_result(request)

        self.companion.callback = callback
        errors: list[Exception] = []

        def worker() -> None:
            try:
                self.sign()
            except Exception as error:
                errors.append(error)

        thread = threading.Thread(target=worker)
        thread.start()
        self.assertTrue(started.wait(5))
        logout = self.state.logout(CONTEXT, "aws")
        self.assertTrue(logout["changed"])
        release.set()
        thread.join(5)
        self.assertFalse(thread.is_alive())
        self.assertEqual([getattr(error, "code", "") for error in errors], ["handle_revoked"])
        self.assertNotIn("aws", self.store.load(CONTEXT, KEY)["providers"])

    def test_different_records_can_refresh_concurrently(self) -> None:
        self.state.login_aws_driver(
            CONTEXT_TWO,
            b'{"opaque_host_driver_state":"second-context"}',
            account_label="210987654321",
            driver_id="aws_cli_sso",
            driver_revision="b" * 64,
        )
        second_issued = self.state.issue_handle(
            CONTEXT_TWO, PROJECT_TWO, "aws", [aws_binding()]
        )
        both_started = threading.Event()
        release = threading.Event()
        active_lock = threading.Lock()
        active = 0
        maximum_active = 0

        def callback(request: RefreshRequest) -> RefreshResult:
            nonlocal active, maximum_active
            with active_lock:
                active += 1
                maximum_active = max(maximum_active, active)
                if active == 2:
                    both_started.set()
            if not release.wait(5):
                raise AssertionError("concurrent refreshes were not released")
            result = successful_result(
                request, UPDATED_STATE + request.context_id.encode("ascii")
            )
            with active_lock:
                active -= 1
            return result

        self.companion.callback = callback
        errors: list[Exception] = []

        def worker(context_id: str, project_id: str, issued: dict[str, object]) -> None:
            try:
                self.state.sign_sigv4(
                    issued["handle"],
                    context_id,
                    project_id,
                    "aws",
                    issued["revision"],
                    aws_binding(),
                    signing_request(),
                )
            except Exception as error:  # pragma: no cover - asserted below
                errors.append(error)

        first = threading.Thread(target=worker, args=(CONTEXT, PROJECT, self.issued))
        second = threading.Thread(
            target=worker, args=(CONTEXT_TWO, PROJECT_TWO, second_issued)
        )
        first.start()
        second.start()
        self.assertTrue(both_started.wait(5))
        release.set()
        first.join(5)
        second.join(5)
        self.assertFalse(first.is_alive())
        self.assertFalse(second.is_alive())
        self.assertEqual(errors, [])
        self.assertEqual(maximum_active, 2)

    def test_relogin_rotation_wins_over_late_refresh_result(self) -> None:
        started = threading.Event()
        release = threading.Event()

        def callback(request: RefreshRequest) -> RefreshResult:
            started.set()
            if not release.wait(5):
                raise AssertionError("refresh was not released")
            return successful_result(request)

        self.companion.callback = callback
        errors: list[Exception] = []
        thread = threading.Thread(
            target=lambda: self._capture_error(self.sign, errors)
        )
        thread.start()
        self.assertTrue(started.wait(5))
        replacement_state = b'{"opaque_host_driver_state":"replacement-canary"}'
        replacement = self.state.login_aws_driver(
            CONTEXT,
            replacement_state,
            account_label="123456789012",
            driver_id="aws_cli_sso",
            driver_revision="b" * 64,
        )
        release.set()
        thread.join(5)
        self.assertFalse(thread.is_alive())
        self.assertEqual([getattr(error, "code", "") for error in errors], ["handle_revoked"])
        record = self.store.load(CONTEXT, KEY)["providers"]["aws"]
        self.assertEqual(record["revision"], replacement["revision"])
        self.assertEqual(record["state_generation"], 0)
        self.assertEqual(decode_secret(record["state"]), replacement_state)

    @staticmethod
    def _capture_error(call, errors: list[Exception]) -> None:
        try:
            call()
        except Exception as error:
            errors.append(error)

    def test_invalid_or_unknown_companion_result_sets_durable_no_replay_barrier(self) -> None:
        cases = {
            "correlation": lambda request: RefreshResult(
                request_id="0" * 32,
                task_digest=request.task_digest,
                state_generation=request.state_generation,
                secret_payload=successful_result(request).secret_payload,
            ),
            "payload": lambda request: RefreshResult(
                request_id=request.request_id,
                task_digest=request.task_digest,
                state_generation=request.state_generation,
                secret_payload=b'{"secret":"provider-canary"}',
            ),
            "outcome_unknown": lambda _request: (_ for _ in ()).throw(
                CompanionError("companion_outcome_unknown")
            ),
        }
        for name, callback in cases.items():
            with self.subTest(name=name):
                self.login = self.state.login_aws_driver(
                    CONTEXT,
                    INITIAL_STATE,
                    account_label="123456789012",
                    driver_id="aws_cli_sso",
                    driver_revision=DRIVER_REVISION,
                )
                self.issued = self.state.issue_handle(
                    CONTEXT, PROJECT, "aws", [aws_binding()]
                )
                before = self.store.load(CONTEXT, KEY)["providers"]["aws"]
                self.companion.calls.clear()
                self.companion.callback = callback
                with self.assertRaisesRegex(
                    BrokerError, "^companion_outcome_unknown$"
                ):
                    self.sign()
                after = self.store.load(CONTEXT, KEY)["providers"]["aws"]
                self.assertEqual(after["record_id"], before["record_id"])
                self.assertEqual(after["revision"], before["revision"])
                self.assertEqual(after["state_generation"], before["state_generation"])
                self.assertEqual(after["state"], before["state"])
                self.assertRegex(after["refresh_task_digest"], r"^[0-9a-f]{64}$")
                self.assertEqual(len(self.companion.calls), 1)
                self.assertEqual(
                    self.state.status(CONTEXT, "aws")["state"],
                    "not_configured",
                )
                with self.assertRaisesRegex(BrokerError, "^companion_outcome_unknown$"):
                    self.state.introspect_signing(
                        self.issued["handle"],
                        CONTEXT,
                        PROJECT,
                        "aws",
                        {
                            "scheme": "https",
                            "host": "sts.us-east-1.amazonaws.com",
                            "port": 443,
                        },
                        aws_binding(),
                    )
                # A Broker restart reloads the encrypted barrier and still
                # refuses to issue a handle or replay the host refresh.
                restarted = BrokerState(self.store, companion=FakeCompanion())
                restarted.unlock(KEY)
                with self.assertRaisesRegex(BrokerError, "^credential_not_found$"):
                    restarted.issue_handle(
                        CONTEXT, PROJECT, "aws", [aws_binding()]
                    )

    def test_proven_pre_execution_failure_clears_refresh_barrier(self) -> None:
        self.companion.callback = lambda _request: (_ for _ in ()).throw(
            CompanionError("companion_unavailable")
        )
        with self.assertRaisesRegex(BrokerError, "^companion_unavailable$"):
            self.sign()
        record = self.store.load(CONTEXT, KEY)["providers"]["aws"]
        self.assertIsNone(record["refresh_task_digest"])
        self.assertEqual(record["state_generation"], 0)
        self.assertEqual(decode_secret(record["state"]), INITIAL_STATE)

    def test_secret_bearing_values_are_redacted_from_repr_logs_and_errors(self) -> None:
        request = RefreshRequest.create(
            context_id=CONTEXT,
            project_id=PROJECT,
            provider="aws",
            record_id="record_synthetic",
            grant_revision="revision_synthetic",
            state_generation=0,
            driver_id="aws_cli_sso",
            driver_revision=DRIVER_REVISION,
            binding_digest="b" * 64,
            request_digest="c" * 64,
            state=INITIAL_STATE,
        )
        snapshot = AwsRefreshSnapshot(
            context_id=CONTEXT,
            project_id=PROJECT,
            provider="aws",
            record_id="record_synthetic",
            revision="revision_synthetic",
            state_generation=0,
            driver_id="aws_cli_sso",
            driver_revision=DRIVER_REVISION,
            binding_digest="b" * 64,
            request_digest="c" * 64,
            state_sha256="d" * 64,
            state=INITIAL_STATE,
        )
        secret = RefreshSecret(
            state=UPDATED_STATE,
            access_key_id=ACCESS_KEY,
            secret_access_key=SECRET_KEY,
            session_token=SESSION_TOKEN,
            expiration_unix_ms=(NOW + 3600) * 1000,
        )
        rendered = repr((request, snapshot, secret))
        for canary in (
            INITIAL_STATE.decode(),
            UPDATED_STATE.decode(),
            ACCESS_KEY,
            SECRET_KEY,
            SESSION_TOKEN,
        ):
            self.assertNotIn(canary, rendered)

        self.companion.callback = lambda _request: (_ for _ in ()).throw(
            CompanionError("companion_outcome_unknown")
        )
        stream = io.StringIO()
        handler = logging.StreamHandler(stream)
        logger = logging.getLogger()
        logger.addHandler(handler)
        try:
            with contextlib.redirect_stderr(stream):
                with self.assertRaisesRegex(BrokerError, "^companion_outcome_unknown$") as raised:
                    self.sign()
        finally:
            logger.removeHandler(handler)
        self.assertEqual(repr(raised.exception), "BrokerError('companion_outcome_unknown')")
        for canary in (INITIAL_STATE.decode(), SECRET_KEY, SESSION_TOKEN):
            self.assertNotIn(canary, stream.getvalue())


if __name__ == "__main__":
    unittest.main()
