from __future__ import annotations

import contextlib
import hashlib
import hmac
import io
import json
import os
import socket
import struct
import tempfile
import threading
import time
import unittest
from pathlib import Path
from unittest import mock

from cryptography.hazmat.primitives.ciphers.aead import AESGCM

import authbroker.companion_protocol as companion_module
from authbroker.broker import BrokerError, BrokerState, Dispatcher
from authbroker.companion_bridge import pump
from authbroker.companion_protocol import (
    BROKER_TO_COMPANION,
    COMPANION_PROTOCOL_VERSION,
    COMPANION_TO_BROKER,
    EPOCH_PREFIX,
    FRAME_MAGIC,
    MAX_COMPANION_FRAME_BYTES,
    MAX_COMPANION_JSON_BYTES,
    CompanionChannelManager,
    CompanionError,
    EncryptedSession,
    RefreshRequest,
    RefreshSecret,
    client_handshake,
    compute_task_digest,
    decode_refresh_secret,
    derive_epoch_key,
    encode_refresh_secret,
)
from authbroker.daemon import _CompanionUnixServer
from authbroker.vault import VaultStore

ROOT_KEY = bytes(range(32))
EPOCH_RAW = bytes(reversed(range(32)))
EPOCH_ID = EPOCH_PREFIX + "Hx4dHBsaGRgXFhUUExIREA8ODQwLCgkIBwYFBAMCAQA"
CONTEXT_ID = "018bcfe5-687b-7000-8000-000000000001"
PROJECT_ID = "018bcfe5-687b-7000-8000-000000000101"
DIGEST_A = "a" * 64
DIGEST_B = "b" * 64


def refresh_request(timeout: float = 2.0) -> RefreshRequest:
    return RefreshRequest.create(
        context_id=CONTEXT_ID,
        project_id=PROJECT_ID,
        provider="aws",
        record_id="record_synthetic",
        grant_revision="revision_synthetic",
        state_generation=7,
        driver_id="aws_cli_sso",
        driver_revision=DIGEST_A,
        binding_digest=DIGEST_B,
        request_digest="c" * 64,
        state=b'{"synthetic":"opaque-state"}',
        timeout_seconds=timeout,
    )


def secret_payload() -> bytes:
    return encode_refresh_secret(
        RefreshSecret(
            state=b'{"synthetic":"updated-state"}',
            access_key_id="ASIA1234567890ABCDEF",
            secret_access_key="abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMN",
            session_token="sessiontokenABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789+/=",
            expiration_unix_ms=int((time.time() + 3600) * 1000),
        )
    )


class _CaptureConnection:
    def __init__(self) -> None:
        self.encoded = bytearray()

    def send(self, payload: bytes | memoryview, _: int = 0) -> int:
        self.encoded.extend(payload)
        return len(payload)

    def shutdown(self, _: int) -> None:
        return

    def close(self) -> None:
        return


def encoded_frame(
    document: dict[str, object],
    *,
    payload: bytes = b"",
    key: bytes = b"A" * 32,
    session_id: bytes = b"S" * 16,
) -> bytes:
    capture = _CaptureConnection()
    sender = EncryptedSession(
        capture,  # type: ignore[arg-type]
        session_id=session_id,
        send_key=key,
        receive_key=b"B" * 32,
        send_direction=BROKER_TO_COMPANION,
        receive_direction=COMPANION_TO_BROKER,
    )
    sender.send_message(document, payload)
    return bytes(capture.encoded)


class _FakeCompanion:
    def __init__(self, manager: CompanionChannelManager, epoch_id: str, key: bytes):
        broker_socket, host_socket = socket.socketpair()
        self.manager = manager
        self.server_thread = threading.Thread(
            target=manager.serve_connection, args=(broker_socket,), daemon=True
        )
        self.server_thread.start()
        _, self.channel = client_handshake(
            host_socket, epoch_key=key, expected_epoch_id=epoch_id
        )
        ready, payload = self.channel.receive_message()
        if payload or ready["type"] != "ready":
            raise AssertionError("broker did not send readiness challenge")
        self.channel.send_message(
            {
                "protocol_version": COMPANION_PROTOCOL_VERSION,
                "type": "ready_ack",
                "session_id": ready["session_id"],
            }
        )
        deadline = time.monotonic() + 2
        while manager.status()[0] != "ready":
            if time.monotonic() >= deadline:
                raise AssertionError("companion did not become ready")
            time.sleep(0.005)
        self.loop_thread: threading.Thread | None = None
        self.seen: list[dict[str, object]] = []

    def start(self, callback=None) -> None:
        def run() -> None:
            try:
                while True:
                    document, payload = self.channel.receive_message()
                    self.seen.append(document)
                    message_type = document["type"]
                    if message_type == "ping":
                        self.channel.send_message(
                            {
                                "protocol_version": COMPANION_PROTOCOL_VERSION,
                                "type": "pong",
                                "request_id": document["request_id"],
                            }
                        )
                    elif message_type == "drain":
                        self.channel.send_message(
                            {
                                "protocol_version": COMPANION_PROTOCOL_VERSION,
                                "type": "drain_ack",
                                "request_id": document["request_id"],
                            }
                        )
                    elif callback is not None:
                        callback(self.channel, document, payload)
            except CompanionError:
                return

        self.loop_thread = threading.Thread(target=run, daemon=True)
        self.loop_thread.start()

    def close(self) -> None:
        self.channel.close()
        self.server_thread.join(timeout=2)
        if self.loop_thread is not None:
            self.loop_thread.join(timeout=2)


class EpochAndControlTests(unittest.TestCase):
    def test_default_refresh_deadline_leaves_cross_clock_margin(self) -> None:
        fixed_time = 1_700_000_000.125
        with mock.patch.object(
            companion_module.time, "time", return_value=fixed_time
        ):
            default_request = RefreshRequest.create(
                context_id=CONTEXT_ID,
                project_id=PROJECT_ID,
                provider="aws",
                record_id="record_synthetic",
                grant_revision="revision_synthetic",
                state_generation=7,
                driver_id="aws_cli_sso",
                driver_revision=DIGEST_A,
                binding_digest=DIGEST_B,
                request_digest="c" * 64,
                state=b'{"synthetic":"opaque-state"}',
            )
            capped_request = RefreshRequest.create(
                context_id=CONTEXT_ID,
                project_id=PROJECT_ID,
                provider="aws",
                record_id="record_synthetic",
                grant_revision="revision_synthetic",
                state_generation=7,
                driver_id="aws_cli_sso",
                driver_revision=DIGEST_A,
                binding_digest=DIGEST_B,
                request_digest="c" * 64,
                state=b'{"synthetic":"opaque-state"}',
                timeout_seconds=600,
            )
        self.assertEqual(
            default_request.deadline_unix_ms,
            int(
                (fixed_time + companion_module.DEFAULT_REFRESH_SECONDS)
                * 1000
            ),
        )
        self.assertGreaterEqual(
            companion_module.MAX_REFRESH_SECONDS
            - companion_module.DEFAULT_REFRESH_SECONDS,
            10,
        )
        self.assertEqual(
            capped_request.deadline_unix_ms,
            int((fixed_time + companion_module.MAX_REFRESH_SECONDS) * 1000),
        )

    def test_epoch_key_vector_and_control_status_are_exact(self) -> None:
        self.assertEqual(EPOCH_RAW, bytes(reversed(range(32))))
        self.assertEqual(
            derive_epoch_key(ROOT_KEY, EPOCH_ID).hex(),
            "0d7f38e34b1bda5b2e9d9d61e3e89acc3faa736e5530b49576013bf38f062a0e",
        )
        with tempfile.TemporaryDirectory() as temporary:
            state = BrokerState(VaultStore(Path(temporary) / "contexts"))
            dispatcher = Dispatcher(state, "control")
            self.assertEqual(
                dispatcher.dispatch(
                    {"schema_version": 1, "op": "companion_status"}, b""
                ),
                {
                    "schema_version": 1,
                    "ok": True,
                    "state": "absent",
                    "epoch_id": "",
                },
            )
            with self.assertRaisesRegex(BrokerError, "locked"):
                dispatcher.dispatch(
                    {
                        "schema_version": 1,
                        "op": "companion_prepare",
                        "epoch_id": EPOCH_ID,
                    },
                    b"",
                )
            state.unlock(ROOT_KEY)
            prepared = dispatcher.dispatch(
                {
                    "schema_version": 1,
                    "op": "companion_prepare",
                    "epoch_id": EPOCH_ID,
                },
                b"",
            )
            self.assertEqual(prepared["state"], "prepared")
            self.assertEqual(prepared["epoch_id"], EPOCH_ID)
            with self.assertRaisesRegex(BrokerError, "invalid_request"):
                dispatcher.dispatch(
                    {
                        "schema_version": 1,
                        "op": "companion_status",
                        "epoch_id": EPOCH_ID,
                    },
                    b"",
                )

    def test_failed_handshake_keeps_preparation_and_emits_no_diagnostic(self) -> None:
        manager = CompanionChannelManager(broker_boot=b"B" * 16)
        manager.prepare(EPOCH_ID, derive_epoch_key(ROOT_KEY, EPOCH_ID))
        broker_socket, hostile_socket = socket.socketpair()

        def hostile() -> None:
            remaining = struct.calcsize(">8s32s16s32s")
            while remaining:
                chunk = hostile_socket.recv(remaining)
                if not chunk:
                    return
                remaining -= len(chunk)
            canary = (b"secret-handshake-canary" * 8)[: struct.calcsize(">8s16s32s32s")]
            hostile_socket.sendall(canary)
            hostile_socket.close()

        peer = threading.Thread(target=hostile)
        peer.start()
        visible_out = io.StringIO()
        visible_err = io.StringIO()
        with contextlib.redirect_stdout(visible_out), contextlib.redirect_stderr(
            visible_err
        ):
            manager.serve_connection(broker_socket)
        peer.join(timeout=1)
        self.assertEqual(manager.status(), ("prepared", EPOCH_ID))
        self.assertEqual(visible_out.getvalue(), "")
        self.assertEqual(visible_err.getvalue(), "")
        manager.invalidate()

    def test_cross_language_fixture_covers_handshake_keys_task_and_frame(self) -> None:
        fixture_path = Path(__file__).parent / "fixtures" / "companion_protocol_v1.json"
        fixture = json.loads(fixture_path.read_text(encoding="utf-8"))
        root = bytes.fromhex(fixture["root_key_hex"])
        epoch = companion_module.decode_epoch_id(fixture["epoch_id"])
        key = derive_epoch_key(root, fixture["epoch_id"])
        self.assertEqual(key.hex(), fixture["epoch_key_hex"])

        broker_boot = bytes.fromhex(fixture["broker_boot_hex"])
        broker_nonce = bytes.fromhex(fixture["broker_nonce_hex"])
        instance = bytes.fromhex(fixture["companion_instance_hex"])
        companion_nonce = bytes.fromhex(fixture["companion_nonce_hex"])
        session_id = bytes.fromhex(fixture["session_id_hex"])
        challenge = companion_module._CHALLENGE.pack(
            companion_module.CHALLENGE_MAGIC, epoch, broker_boot, broker_nonce
        )
        self.assertEqual(challenge.hex(), fixture["challenge_hex"])
        client_header = companion_module.CLIENT_MAGIC + instance + companion_nonce
        client_mac = hmac.digest(
            key,
            companion_module.CLIENT_PROOF_DOMAIN + challenge + client_header,
            "sha256",
        )
        client_proof = companion_module._CLIENT_PROOF.pack(
            companion_module.CLIENT_MAGIC, instance, companion_nonce, client_mac
        )
        self.assertEqual(client_proof.hex(), fixture["client_proof_hex"])
        server_header = companion_module.SERVER_MAGIC + session_id
        server_mac = hmac.digest(
            key,
            companion_module.SERVER_PROOF_DOMAIN
            + challenge
            + client_header
            + client_mac
            + server_header,
            "sha256",
        )
        server_proof = companion_module._SERVER_PROOF.pack(
            companion_module.SERVER_MAGIC, session_id, server_mac
        )
        self.assertEqual(server_proof.hex(), fixture["server_proof_hex"])
        broker_key, companion_key = companion_module._derive_session_keys(
            key,
            epoch,
            broker_boot,
            broker_nonce,
            instance,
            companion_nonce,
            session_id,
        )
        self.assertEqual(
            broker_key.hex(), fixture["broker_to_companion_key_hex"]
        )
        self.assertEqual(
            companion_key.hex(), fixture["companion_to_broker_key_hex"]
        )

        state = bytes.fromhex(fixture["task_state_hex"])
        task_document = {
            "request_id": fixture["request_id"],
            "context_id": fixture["context_id"],
            "project_id": fixture["project_id"],
            "provider": fixture["provider"],
            "record_id": fixture["record_id"],
            "grant_revision": fixture["grant_revision"],
            "driver_id": fixture["driver_id"],
            "driver_revision": fixture["driver_revision"],
            "binding_digest": fixture["binding_digest"],
            "request_digest": fixture["request_digest"],
            "state_sha256": hashlib.sha256(state).hexdigest(),
            "state_generation": fixture["state_generation"],
            "deadline_unix_ms": fixture["deadline_unix_ms"],
        }
        self.assertEqual(
            compute_task_digest(task_document), fixture["task_digest_hex"]
        )

        sender, receiver_socket = socket.socketpair()
        receiver = EncryptedSession(
            receiver_socket,
            session_id=session_id,
            send_key=companion_key,
            receive_key=broker_key,
            send_direction=COMPANION_TO_BROKER,
            receive_direction=BROKER_TO_COMPANION,
        )
        sender.sendall(bytes.fromhex(fixture["ready_frame_hex"]))
        ready, payload = receiver.receive_message()
        self.assertEqual(
            ready,
            {
                "protocol_version": 1,
                "type": "ready",
                "session_id": fixture["session_id_hex"],
            },
        )
        self.assertEqual(payload, b"")
        sender.close()
        receiver.close()


class SecureFrameTests(unittest.TestCase):
    @staticmethod
    def receiver(connection: socket.socket) -> EncryptedSession:
        return EncryptedSession(
            connection,
            session_id=b"S" * 16,
            send_key=b"B" * 32,
            receive_key=b"A" * 32,
            send_direction=COMPANION_TO_BROKER,
            receive_direction=BROKER_TO_COMPANION,
        )

    @staticmethod
    def ping() -> dict[str, object]:
        return {
            "protocol_version": COMPANION_PROTOCOL_VERSION,
            "type": "ping",
            "request_id": "1" * 32,
        }

    @staticmethod
    def sender(connection: socket.socket) -> EncryptedSession:
        return EncryptedSession(
            connection,
            session_id=b"S" * 16,
            send_key=b"A" * 32,
            receive_key=b"B" * 32,
            send_direction=BROKER_TO_COMPANION,
            receive_direction=COMPANION_TO_BROKER,
        )

    @staticmethod
    def saturate(connection: socket.socket) -> None:
        connection.setblocking(False)
        try:
            while True:
                connection.send(b"x" * 4096)
        except BlockingIOError:
            pass
        finally:
            connection.setblocking(True)

    def test_replay_gap_tag_and_oversize_close_the_frame(self) -> None:
        wire = encoded_frame(self.ping())
        cases: dict[str, tuple[bytes, str]] = {
            "gap": (wire[:4] + struct.pack(">Q", 1) + wire[12:], "sequence"),
            "tag": (wire[:-1] + bytes([wire[-1] ^ 1]), "auth_failed"),
            "oversize": (
                struct.pack(">IQ", MAX_COMPANION_FRAME_BYTES + 1, 0),
                "frame_invalid",
            ),
        }
        for name, (candidate, code) in cases.items():
            with self.subTest(name=name):
                sender, receiver_socket = socket.socketpair()
                receiver = self.receiver(receiver_socket)
                sender.sendall(candidate)
                with self.assertRaisesRegex(CompanionError, code):
                    receiver.receive_message()
                sender.close()
                receiver.close()

        sender, receiver_socket = socket.socketpair()
        receiver = self.receiver(receiver_socket)
        sender.sendall(wire)
        self.assertEqual(receiver.receive_message()[0]["type"], "ping")
        sender.sendall(wire)
        with self.assertRaisesRegex(CompanionError, "sequence"):
            receiver.receive_message()
        sender.close()
        receiver.close()

    def test_duplicate_json_and_inner_json_limit_are_rejected(self) -> None:
        duplicate = (
            b'{"protocol_version":1,"type":"ping","type":"ping",'
            b'"request_id":"' + b"1" * 32 + b'"}'
        )
        plaintext = struct.pack(">I", len(duplicate)) + duplicate
        ciphertext_length = len(plaintext) + 16
        aad = (
            FRAME_MAGIC
            + b"S" * 16
            + BROKER_TO_COMPANION
            + struct.pack(">QI", 0, ciphertext_length)
        )
        ciphertext = AESGCM(b"A" * 32).encrypt(
            BROKER_TO_COMPANION + struct.pack(">Q", 0), plaintext, aad
        )
        wire = struct.pack(">IQ", ciphertext_length, 0) + ciphertext
        sender, receiver_socket = socket.socketpair()
        receiver = self.receiver(receiver_socket)
        sender.sendall(wire)
        with self.assertRaisesRegex(CompanionError, "message_invalid"):
            receiver.receive_message()
        sender.close()
        receiver.close()

        oversized_json = b"x" * (MAX_COMPANION_JSON_BYTES + 1)
        plaintext = struct.pack(">I", len(oversized_json)) + oversized_json
        ciphertext_length = len(plaintext) + 16
        aad = (
            FRAME_MAGIC
            + b"S" * 16
            + BROKER_TO_COMPANION
            + struct.pack(">QI", 0, ciphertext_length)
        )
        ciphertext = AESGCM(b"A" * 32).encrypt(
            BROKER_TO_COMPANION + struct.pack(">Q", 0), plaintext, aad
        )
        sender, receiver_socket = socket.socketpair()
        receiver = self.receiver(receiver_socket)
        sender.sendall(struct.pack(">IQ", ciphertext_length, 0) + ciphertext)
        with self.assertRaisesRegex(CompanionError, "message_invalid"):
            receiver.receive_message()
        sender.close()
        receiver.close()

    def test_refresh_and_drain_writes_are_bounded_when_peer_stops_reading(
        self,
    ) -> None:
        request = refresh_request()
        cases = {
            "refresh": (request.document(), request.state),
            "drain": (
                {
                    "protocol_version": COMPANION_PROTOCOL_VERSION,
                    "type": "drain",
                    "request_id": "2" * 32,
                    "deadline_unix_ms": int((time.time() + 1) * 1000),
                },
                b"",
            ),
        }
        for name, (document, payload) in cases.items():
            with self.subTest(name=name):
                sender_socket, peer = socket.socketpair()
                sender_socket.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 1024)
                self.saturate(sender_socket)
                session = self.sender(sender_socket)
                started = time.monotonic()
                with mock.patch.object(
                    companion_module, "COMPANION_WRITE_TIMEOUT_SECONDS", 0.1
                ):
                    with self.assertRaisesRegex(
                        CompanionError, "^companion_disconnected$"
                    ):
                        session.send_message(document, payload)
                self.assertLess(time.monotonic() - started, 1)
                self.assertTrue(session._closed)
                session.close()
                peer.close()

    def test_close_shutdown_unblocks_a_peer_stalled_send_before_send_lock(self) -> None:
        sender_socket, peer = socket.socketpair()
        sender_socket.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 1024)
        self.saturate(sender_socket)
        session = self.sender(sender_socket)
        started = threading.Event()
        errors: list[str] = []

        def send() -> None:
            started.set()
            try:
                session.send_message(self.ping())
            except CompanionError as error:
                errors.append(error.code)

        with mock.patch.object(
            companion_module, "COMPANION_WRITE_TIMEOUT_SECONDS", 5.0
        ):
            writer = threading.Thread(target=send)
            writer.start()
            self.assertTrue(started.wait(1))
            time.sleep(0.05)
            self.assertTrue(writer.is_alive())
            close_started = time.monotonic()
            session.close()
            self.assertLess(time.monotonic() - close_started, 1)
            writer.join(timeout=1)
        self.assertFalse(writer.is_alive())
        self.assertEqual(errors, ["companion_disconnected"])
        peer.close()


class ChannelBackpressureTests(unittest.TestCase):
    @staticmethod
    def active_manager() -> tuple[
        CompanionChannelManager, EncryptedSession, socket.socket
    ]:
        connection, peer = socket.socketpair()
        connection.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, 1024)
        SecureFrameTests.saturate(connection)
        session = SecureFrameTests.sender(connection)
        manager = CompanionChannelManager(broker_boot=b"B" * 16)
        with manager._lock:
            manager._active = companion_module._ActiveSession(EPOCH_ID, 1, session)
        return manager, session, peer

    def test_invalidate_unblocks_refresh_write_without_waiting_for_send_lock(
        self,
    ) -> None:
        manager, _, peer = self.active_manager()
        errors: list[str] = []
        started = threading.Event()

        def refresh() -> None:
            started.set()
            try:
                manager.refresh(refresh_request())
            except CompanionError as error:
                errors.append(error.code)

        with mock.patch.object(
            companion_module, "COMPANION_WRITE_TIMEOUT_SECONDS", 5.0
        ):
            caller = threading.Thread(target=refresh)
            caller.start()
            self.assertTrue(started.wait(1))
            time.sleep(0.05)
            self.assertTrue(caller.is_alive())
            invalidate_started = time.monotonic()
            manager.invalidate()
            self.assertLess(time.monotonic() - invalidate_started, 1)
            caller.join(timeout=1)
        self.assertFalse(caller.is_alive())
        self.assertEqual(errors, ["companion_outcome_unknown"])
        peer.close()

    def test_drain_write_is_bounded_when_peer_stops_reading(self) -> None:
        manager, _, peer = self.active_manager()
        started = time.monotonic()
        with mock.patch.object(
            companion_module, "COMPANION_WRITE_TIMEOUT_SECONDS", 0.1
        ):
            with self.assertRaisesRegex(
                CompanionError, "^companion_disconnected$"
            ):
                manager.drain(timeout=1)
        self.assertLess(time.monotonic() - started, 1)
        peer.close()


class ChannelManagerTests(unittest.TestCase):
    def setUp(self) -> None:
        self.temporary = tempfile.TemporaryDirectory()
        self.manager = CompanionChannelManager(broker_boot=b"B" * 16)
        self.state = BrokerState(
            VaultStore(Path(self.temporary.name) / "contexts"),
            companion=self.manager,
        )
        self.state.unlock(ROOT_KEY)
        self.key = derive_epoch_key(ROOT_KEY, EPOCH_ID)
        self.state.prepare_companion(EPOCH_ID)
        self.companion = _FakeCompanion(self.manager, EPOCH_ID, self.key)

    def tearDown(self) -> None:
        self.companion.close()
        self.manager.invalidate()
        self.temporary.cleanup()

    def test_one_active_session_cannot_be_replaced(self) -> None:
        self.companion.start()
        second_broker, second_host = socket.socketpair()
        attempt = threading.Thread(
            target=self.manager.serve_connection, args=(second_broker,), daemon=True
        )
        attempt.start()
        second_host.settimeout(1)
        self.assertEqual(second_host.recv(1), b"")
        attempt.join(timeout=1)
        second_host.close()
        self.assertEqual(self.manager.status(), ("ready", EPOCH_ID))
        self.manager.ping(timeout=1)

    def test_refresh_round_trip_and_disconnect_fail_pending(self) -> None:
        def success(channel, document, _: bytes) -> None:
            if document["type"] != "refresh":
                return
            channel.send_message(
                {
                    "protocol_version": COMPANION_PROTOCOL_VERSION,
                    "type": "refresh_accepted",
                    "request_id": document["request_id"],
                    "task_digest": document["task_digest"],
                }
            )
            payload = secret_payload()
            channel.send_message(
                {
                    "protocol_version": COMPANION_PROTOCOL_VERSION,
                    "type": "refresh_result",
                    "request_id": document["request_id"],
                    "task_digest": document["task_digest"],
                    "state_generation": document["state_generation"],
                    "ok": True,
                    "error": None,
                    "payload_length": len(payload),
                },
                payload,
            )

        self.companion.start(success)
        request = refresh_request()
        result = self.state.refresh_with_companion(request)
        self.assertEqual(result.request_id, request.request_id)
        self.assertEqual(
            decode_refresh_secret(result.secret_payload).state,
            b'{"synthetic":"updated-state"}',
        )

        self.companion.close()
        deadline = time.monotonic() + 1
        while self.manager.status()[0] != "absent" and time.monotonic() < deadline:
            time.sleep(0.005)
        with self.assertRaisesRegex(CompanionError, "unavailable"):
            self.manager.refresh(refresh_request())

    def test_disconnect_after_accept_fails_one_pending_call(self) -> None:
        def disconnect(channel, document, _: bytes) -> None:
            if document["type"] == "refresh":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_accepted",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                channel.close()

        self.companion.start(disconnect)
        with self.assertRaisesRegex(CompanionError, "outcome_unknown"):
            self.manager.refresh(refresh_request())

    def test_pre_send_cancellation_is_known_and_sends_nothing(self) -> None:
        self.companion.start()
        cancelled = threading.Event()
        cancelled.set()
        with self.assertRaisesRegex(CompanionError, "^companion_cancelled$"):
            self.manager.refresh(refresh_request(), cancelled)
        self.assertEqual(self.companion.seen, [])

    def test_cancel_frame_waits_for_refresh_accepted(self) -> None:
        refresh_seen = threading.Event()
        release_accept = threading.Event()
        cancelled = threading.Event()
        request = refresh_request()

        def callback(channel, document, _: bytes) -> None:
            if document["type"] == "refresh":
                refresh_seen.set()
                if not release_accept.wait(1):
                    return
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_accepted",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
            elif document["type"] == "cancel":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "cancel_ack",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_result",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                        "state_generation": request.state_generation,
                        "ok": False,
                        "error": "cancelled",
                        "payload_length": 0,
                    }
                )

        self.companion.start(callback)
        errors: list[str] = []

        def call() -> None:
            try:
                self.manager.refresh(request, cancelled)
            except CompanionError as error:
                errors.append(error.code)

        caller = threading.Thread(target=call)
        caller.start()
        self.assertTrue(refresh_seen.wait(1))
        cancelled.set()
        time.sleep(0.1)
        with self.manager._lock:
            self.assertFalse(self.manager._pending[request.request_id].cancel_sent)
        release_accept.set()
        caller.join(timeout=1)
        self.assertEqual(errors, ["companion_cancelled"])

    def test_cancel_ack_is_receipt_only_until_correlated_terminal_result(self) -> None:
        accepted = threading.Event()
        cancel_seen = threading.Event()
        ack_sent = threading.Event()
        release_result = threading.Event()

        def callback(channel, document, _: bytes) -> None:
            if document["type"] == "refresh":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_accepted",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                accepted.set()
            elif document["type"] == "cancel":
                cancel_seen.set()
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "cancel_ack",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                ack_sent.set()
                if not release_result.wait(1):
                    return
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_result",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                        "state_generation": 7,
                        "ok": False,
                        "error": "cancelled",
                        "payload_length": 0,
                    }
                )

        self.companion.start(callback)
        cancelled = threading.Event()
        result: list[str] = []

        def call() -> None:
            try:
                self.manager.refresh(refresh_request(), cancelled)
            except CompanionError as error:
                result.append(error.code)

        caller = threading.Thread(target=call)
        caller.start()
        self.assertTrue(accepted.wait(1))
        cancelled.set()
        self.assertTrue(cancel_seen.wait(1))
        self.assertTrue(ack_sent.wait(1))
        self.assertTrue(caller.is_alive())
        release_result.set()
        caller.join(timeout=1)
        self.assertEqual(result, ["companion_cancelled"])

    def test_cancellation_returns_correlated_terminal_result(self) -> None:
        accepted = threading.Event()

        def callback(channel, document, _: bytes) -> None:
            if document["type"] == "refresh":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_accepted",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                accepted.set()
            elif document["type"] == "cancel":
                payload = secret_payload()
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_result",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                        "state_generation": 7,
                        "ok": True,
                        "error": None,
                        "payload_length": len(payload),
                    },
                    payload,
                )
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "cancel_ack",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )

        self.companion.start(callback)
        cancelled = threading.Event()
        results = []

        def call() -> None:
            results.append(self.manager.refresh(refresh_request(), cancelled))

        caller = threading.Thread(target=call)
        caller.start()
        self.assertTrue(accepted.wait(1))
        cancelled.set()
        caller.join(timeout=1)
        self.assertEqual(len(results), 1)
        self.assertEqual(
            decode_refresh_secret(results[0].secret_payload).state,
            b'{"synthetic":"updated-state"}',
        )
        self.manager.ping(timeout=1)

    def test_unresolved_cancellation_is_outcome_unknown_and_late_result_is_isolated(
        self,
    ) -> None:
        accepted = threading.Event()
        cancel_seen = threading.Event()
        first_request: list[dict[str, object]] = []
        request_count = 0

        def callback(channel, document, _: bytes) -> None:
            nonlocal request_count
            if document["type"] == "refresh":
                request_count += 1
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_accepted",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                if request_count == 1:
                    first_request.append(document)
                    accepted.set()
                    return
                payload = secret_payload()
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_result",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                        "state_generation": document["state_generation"],
                        "ok": True,
                        "error": None,
                        "payload_length": len(payload),
                    },
                    payload,
                )
            elif document["type"] == "cancel":
                cancel_seen.set()
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "cancel_ack",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )

        self.companion.start(callback)
        cancelled = threading.Event()
        errors: list[str] = []

        def call() -> None:
            try:
                self.manager.refresh(refresh_request(), cancelled)
            except CompanionError as error:
                errors.append(error.code)

        with mock.patch.object(
            companion_module, "MAX_CANCEL_RESOLUTION_SECONDS", 0.1
        ):
            caller = threading.Thread(target=call)
            caller.start()
            self.assertTrue(accepted.wait(1))
            cancelled.set()
            self.assertTrue(cancel_seen.wait(1))
            caller.join(timeout=1)
        self.assertEqual(errors, ["companion_outcome_unknown"])

        late = first_request[0]
        payload = secret_payload()
        self.companion.channel.send_message(
            {
                "protocol_version": COMPANION_PROTOCOL_VERSION,
                "type": "refresh_result",
                "request_id": late["request_id"],
                "task_digest": late["task_digest"],
                "state_generation": late["state_generation"],
                "ok": True,
                "error": None,
                "payload_length": len(payload),
            },
            payload,
        )
        result = self.manager.refresh(refresh_request())
        self.assertEqual(
            decode_refresh_secret(result.secret_payload).state,
            b'{"synthetic":"updated-state"}',
        )

    def test_deadline_waits_for_ack_and_preserves_timeout_code(self) -> None:
        def callback(channel, document, _: bytes) -> None:
            if document["type"] == "refresh":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_accepted",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
            elif document["type"] == "cancel":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "cancel_ack",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_result",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                        "state_generation": 7,
                        "ok": False,
                        "error": "timeout",
                        "payload_length": 0,
                    }
                )

        self.companion.start(callback)
        with self.assertRaisesRegex(CompanionError, "^companion_timeout$"):
            self.manager.refresh(refresh_request(timeout=0.1))

    def test_mismatched_late_result_closes_the_channel(self) -> None:
        accepted = threading.Event()

        def callback(channel, document, _: bytes) -> None:
            if document["type"] == "refresh":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "refresh_accepted",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )
                accepted.set()
            elif document["type"] == "cancel":
                channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "cancel_ack",
                        "request_id": document["request_id"],
                        "task_digest": document["task_digest"],
                    }
                )

        self.companion.start(callback)
        cancelled = threading.Event()
        errors: list[str] = []

        def call() -> None:
            try:
                self.manager.refresh(refresh_request(), cancelled)
            except CompanionError as error:
                errors.append(error.code)

        with mock.patch.object(
            companion_module, "MAX_CANCEL_RESOLUTION_SECONDS", 0.1
        ):
            caller = threading.Thread(target=call)
            caller.start()
            self.assertTrue(accepted.wait(1))
            cancelled.set()
            caller.join(timeout=1)
        self.assertEqual(errors, ["companion_outcome_unknown"])
        request_document = next(
            document
            for document in self.companion.seen
            if document["type"] == "refresh"
        )
        self.companion.channel.send_message(
            {
                "protocol_version": COMPANION_PROTOCOL_VERSION,
                "type": "refresh_result",
                "request_id": request_document["request_id"],
                "task_digest": "f" * 64,
                "state_generation": request_document["state_generation"],
                "ok": False,
                "error": "cancelled",
                "payload_length": 0,
            }
        )
        deadline = time.monotonic() + 1
        while self.manager.status()[0] != "absent" and time.monotonic() < deadline:
            time.sleep(0.005)
        self.assertEqual(self.manager.status()[0], "absent")


class SecretEnvelopeAndBridgeTests(unittest.TestCase):
    def test_secret_envelope_is_canonical_strict_and_redacted_by_errors(self) -> None:
        payload = secret_payload()
        decoded = decode_refresh_secret(payload)
        self.assertEqual(encode_refresh_secret(decoded), payload)
        document = json.loads(payload)
        document["unknown"] = True
        with self.assertRaisesRegex(CompanionError, "result_invalid"):
            decode_refresh_secret(
                json.dumps(document, separators=(",", ":"), sort_keys=True).encode()
            )
        duplicate = payload[:-1] + b',"schema_version":1}'
        with self.assertRaisesRegex(CompanionError, "result_invalid"):
            decode_refresh_secret(duplicate)

    def test_bridge_preserves_binary_bytes_without_output(self) -> None:
        bridge_socket, server_socket = socket.socketpair()
        source_bytes = b"\x00secret\xffframe\n"
        response_bytes = b"\x01reply\x00\xfe"
        destination = io.BytesIO()
        received = bytearray()

        def peer() -> None:
            while True:
                chunk = server_socket.recv(8192)
                if not chunk:
                    break
                received.extend(chunk)
            server_socket.sendall(response_bytes)
            server_socket.shutdown(socket.SHUT_WR)

        peer_thread = threading.Thread(target=peer)
        peer_thread.start()
        visible_out = io.StringIO()
        visible_err = io.StringIO()
        with contextlib.redirect_stdout(visible_out), contextlib.redirect_stderr(
            visible_err
        ):
            pump(io.BytesIO(source_bytes), destination, bridge_socket)
        peer_thread.join(timeout=1)
        server_socket.close()
        self.assertEqual(bytes(received), source_bytes)
        self.assertEqual(destination.getvalue(), response_bytes)
        self.assertEqual(visible_out.getvalue(), "")
        self.assertEqual(visible_err.getvalue(), "")

    def test_bridge_forwards_short_pipe_read_before_writer_eof(self) -> None:
        bridge_socket, server_socket = socket.socketpair()
        read_descriptor, write_descriptor = os.pipe()
        source = os.fdopen(read_descriptor, "rb")
        source_bytes = b"TBC2CLNT" + (b"x" * 80)
        response_bytes = b"TBC2SRVR" + (b"y" * 72)
        destination = io.BytesIO()
        pump_thread = threading.Thread(
            target=pump,
            args=(source, destination, bridge_socket),
        )
        pump_thread.start()
        try:
            os.write(write_descriptor, source_bytes)
            server_socket.settimeout(1)
            self.assertEqual(server_socket.recv(8192), source_bytes)

            os.close(write_descriptor)
            write_descriptor = -1
            self.assertEqual(server_socket.recv(8192), b"")
            server_socket.sendall(response_bytes)
            server_socket.shutdown(socket.SHUT_WR)

            pump_thread.join(timeout=1)
            self.assertFalse(pump_thread.is_alive())
            self.assertEqual(destination.getvalue(), response_bytes)
        finally:
            if write_descriptor >= 0:
                os.close(write_descriptor)
            server_socket.close()
            bridge_socket.close()
            source.close()
            pump_thread.join(timeout=1)

    def test_companion_socket_directory_and_socket_are_owner_only(self) -> None:
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "companion" / "bridge.sock"
            manager = CompanionChannelManager()
            server = _CompanionUnixServer(str(path), manager)
            try:
                self.assertEqual(path.parent.stat().st_mode & 0o777, 0o700)
                self.assertEqual(path.stat().st_mode & 0o777, 0o600)
            finally:
                server.server_close()


if __name__ == "__main__":
    unittest.main()
