"""Authenticated, bounded reverse channel for the trusted host companion.

The companion transport is deliberately not a general RPC facility.  It has
one prepared epoch, one active session, and a closed set of message shapes.
Every exception carries only a stable code because frames may contain
credential state.
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import re
import secrets
import select
import socket
import struct
import threading
import time
from collections import deque
from dataclasses import dataclass, field
from typing import Any, Protocol

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF

from .protocol import MAX_SECRET_BYTES, ProtocolError, decode_document

COMPANION_PROTOCOL_VERSION = 1
MAX_COMPANION_JSON_BYTES = 8 * 1024
MAX_COMPANION_FRAME_BYTES = 128 * 1024
MAX_COMPANION_PAYLOAD_BYTES = 96 * 1024
MAX_PENDING_REFRESHES = 32
MAX_IGNORED_REFRESHES = 128
MAX_REFRESH_SECONDS = 60.0
DEFAULT_REFRESH_SECONDS = 45.0
MAX_CANCEL_RESOLUTION_SECONDS = 5.0
COMPANION_WRITE_TIMEOUT_SECONDS = 2.0
HANDSHAKE_TIMEOUT_SECONDS = 5.0

EPOCH_PREFIX = "companion-e1_"
CHALLENGE_MAGIC = b"TBC2CHAL"
CLIENT_MAGIC = b"TBC2CLNT"
SERVER_MAGIC = b"TBC2SRVR"
FRAME_MAGIC = b"TBC2FRM1"
BROKER_TO_COMPANION = b"B2C1"
COMPANION_TO_BROKER = b"C2B1"

EPOCH_SALT_DOMAIN = b"tobari/credential-companion/salt/v1\x00"
EPOCH_KEY_INFO = b"tobari/credential-companion/epoch-key/v1"
CLIENT_PROOF_DOMAIN = b"tobari/credential-companion/client-proof/v1\x00"
SERVER_PROOF_DOMAIN = b"tobari/credential-companion/server-proof/v1\x00"
SESSION_SALT_DOMAIN = b"tobari/credential-companion/session-salt/v1\x00"
SESSION_KEY_INFO = b"tobari/credential-companion/session-key/v1\x00"
TASK_DIGEST_DOMAIN = b"tobari/credential-companion/refresh-task/v1\x00"

_CHALLENGE = struct.Struct(">8s32s16s32s")
_CLIENT_PROOF = struct.Struct(">8s16s32s32s")
_SERVER_PROOF = struct.Struct(">8s16s32s")
_FRAME_HEADER = struct.Struct(">IQ")
_INNER_HEADER = struct.Struct(">I")
_U64_PAIR = struct.Struct(">QQ")

_HEX_16 = re.compile(r"^[0-9a-f]{32}$")
_HEX_32 = re.compile(r"^[0-9a-f]{64}$")
_SAFE_ID = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:/@+-]{0,127}$")
_ERROR_CODE = re.compile(r"^[a-z][a-z0-9_]{0,63}$")
_ACCESS_KEY = re.compile(r"^[A-Z0-9]{16,128}$")
_SECRET_VALUE = re.compile(r"^[A-Za-z0-9/+=_-]+$")
_RESULT_ERRORS = {
    "cancelled",
    "driver_failed",
    "driver_unavailable",
    "invalid_state",
    "outcome_unknown",
    "timeout",
}


class CompanionError(Exception):
    """A secret-free companion failure."""

    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _raw_url_encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _raw_url_decode(value: str) -> bytes:
    try:
        encoded = value.encode("ascii")
        decoded = base64.b64decode(
            encoded + b"=" * (-len(encoded) % 4), altchars=b"-_", validate=True
        )
    except (UnicodeEncodeError, ValueError):
        raise CompanionError("invalid_companion_epoch") from None
    if _raw_url_encode(decoded) != value:
        raise CompanionError("invalid_companion_epoch")
    return decoded


def new_epoch_id() -> str:
    return EPOCH_PREFIX + _raw_url_encode(secrets.token_bytes(32))


def decode_epoch_id(epoch_id: Any) -> bytes:
    if not isinstance(epoch_id, str) or not epoch_id.startswith(EPOCH_PREFIX):
        raise CompanionError("invalid_companion_epoch")
    decoded = _raw_url_decode(epoch_id.removeprefix(EPOCH_PREFIX))
    if len(decoded) != 32:
        raise CompanionError("invalid_companion_epoch")
    return decoded


def derive_epoch_key(root_key: bytes | bytearray, epoch_id: Any) -> bytes:
    """Derive the only key that may cross to the host companion."""

    if not isinstance(root_key, (bytes, bytearray)) or len(root_key) != 32:
        raise CompanionError("invalid_key")
    epoch = decode_epoch_id(epoch_id)
    salt = hashlib.sha256(EPOCH_SALT_DOMAIN + epoch).digest()
    return HKDF(
        algorithm=hashes.SHA256(),
        length=32,
        salt=salt,
        info=EPOCH_KEY_INFO,
    ).derive(bytes(root_key))


def _derive_session_keys(
    epoch_key: bytes,
    epoch: bytes,
    broker_boot: bytes,
    broker_nonce: bytes,
    companion_instance: bytes,
    companion_nonce: bytes,
    session_id: bytes,
) -> tuple[bytes, bytes]:
    salt = hashlib.sha256(
        SESSION_SALT_DOMAIN + broker_nonce + companion_nonce
    ).digest()
    base_info = (
        SESSION_KEY_INFO
        + epoch
        + broker_boot
        + companion_instance
        + session_id
    )

    def derive(direction: bytes) -> bytes:
        return HKDF(
            algorithm=hashes.SHA256(),
            length=32,
            salt=salt,
            info=base_info + b"\x00" + direction,
        ).derive(epoch_key)

    return derive(b"broker-to-companion"), derive(b"companion-to-broker")


def _read_exact(connection: socket.socket, length: int) -> bytes:
    result = bytearray()
    while len(result) < length:
        try:
            chunk = connection.recv(length - len(result))
        except (OSError, TimeoutError):
            raise CompanionError("companion_disconnected") from None
        if not chunk:
            raise CompanionError("companion_disconnected")
        result.extend(chunk)
    return bytes(result)


def _send_exact_bounded(
    connection: socket.socket,
    payload: bytes,
    timeout_seconds: float,
    closed_event: threading.Event,
) -> None:
    """Write one frame without changing the socket's concurrent read mode."""

    if (
        not isinstance(payload, bytes)
        or not payload
        or timeout_seconds <= 0
        or not isinstance(closed_event, threading.Event)
        or not hasattr(socket, "MSG_DONTWAIT")
    ):
        raise CompanionError("companion_disconnected")
    deadline = time.monotonic() + timeout_seconds
    remaining_payload = memoryview(payload)
    send_flags = socket.MSG_DONTWAIT | getattr(socket, "MSG_NOSIGNAL", 0)
    while remaining_payload:
        if closed_event.is_set():
            raise CompanionError("companion_disconnected")
        try:
            written = connection.send(remaining_payload, send_flags)
        except (BlockingIOError, InterruptedError):
            written = None
        except OSError:
            raise CompanionError("companion_disconnected") from None
        if written is not None and written > 0:
            remaining_payload = remaining_payload[written:]
            continue
        if written is not None:
            raise CompanionError("companion_disconnected")
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            raise CompanionError("companion_disconnected")
        try:
            _, writable, exceptional = select.select(
                [], [connection], [connection], min(remaining, 0.05)
            )
        except (OSError, ValueError):
            raise CompanionError("companion_disconnected") from None
        if exceptional or closed_event.is_set():
            raise CompanionError("companion_disconnected")
        if not writable:
            continue


def _canonical_json(document: dict[str, Any]) -> bytes:
    try:
        encoded = json.dumps(
            document,
            ensure_ascii=True,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
    except (TypeError, ValueError):
        raise CompanionError("companion_message_invalid") from None
    if not encoded or len(encoded) > MAX_COMPANION_JSON_BYTES:
        raise CompanionError("companion_message_invalid")
    return encoded


def _safe_id(value: Any) -> bool:
    return isinstance(value, str) and _SAFE_ID.fullmatch(value) is not None


def _uint63(value: Any) -> bool:
    return (
        isinstance(value, int)
        and not isinstance(value, bool)
        and 0 <= value <= (1 << 63) - 1
    )


def _require_keys(document: dict[str, Any], expected: set[str]) -> None:
    if set(document) != expected:
        raise CompanionError("companion_message_invalid")
    if document.get("protocol_version") != COMPANION_PROTOCOL_VERSION or isinstance(
        document.get("protocol_version"), bool
    ):
        raise CompanionError("companion_message_invalid")


def _validate_message(document: dict[str, Any], payload: bytes) -> None:
    message_type = document.get("type")
    if not isinstance(message_type, str):
        raise CompanionError("companion_message_invalid")
    common = {"protocol_version", "type"}
    if message_type in {"ready", "ready_ack"}:
        _require_keys(document, common | {"session_id"})
        if not isinstance(document.get("session_id"), str) or not _HEX_16.fullmatch(
            document["session_id"]
        ):
            raise CompanionError("companion_message_invalid")
    elif message_type in {"ping", "pong", "drain_ack"}:
        _require_keys(document, common | {"request_id"})
        if not isinstance(document.get("request_id"), str) or not _HEX_16.fullmatch(
            document["request_id"]
        ):
            raise CompanionError("companion_message_invalid")
    elif message_type == "drain":
        _require_keys(document, common | {"request_id", "deadline_unix_ms"})
        if (
            not isinstance(document.get("request_id"), str)
            or not _HEX_16.fullmatch(document["request_id"])
            or not _uint63(document.get("deadline_unix_ms"))
        ):
            raise CompanionError("companion_message_invalid")
    elif message_type in {"cancel", "cancel_ack", "refresh_accepted"}:
        _require_keys(document, common | {"request_id", "task_digest"})
        if (
            not isinstance(document.get("request_id"), str)
            or not _HEX_16.fullmatch(document["request_id"])
            or not isinstance(document.get("task_digest"), str)
            or not _HEX_32.fullmatch(document["task_digest"])
        ):
            raise CompanionError("companion_message_invalid")
    elif message_type == "refresh":
        _require_keys(
            document,
            common
            | {
                "request_id",
                "deadline_unix_ms",
                "task_digest",
                "context_id",
                "project_id",
                "provider",
                "record_id",
                "grant_revision",
                "state_generation",
                "driver_id",
                "driver_revision",
                "binding_digest",
                "request_digest",
                "state_sha256",
                "payload_length",
            },
        )
        if (
            not isinstance(document.get("request_id"), str)
            or not _HEX_16.fullmatch(document["request_id"])
            or not _uint63(document.get("deadline_unix_ms"))
            or not isinstance(document.get("task_digest"), str)
            or not _HEX_32.fullmatch(document["task_digest"])
            or any(
                not _safe_id(document.get(field))
                for field in (
                    "context_id",
                    "project_id",
                    "provider",
                    "record_id",
                    "grant_revision",
                    "driver_id",
                )
            )
            or not isinstance(document.get("driver_revision"), str)
            or not _HEX_32.fullmatch(document["driver_revision"])
            or not isinstance(document.get("binding_digest"), str)
            or not _HEX_32.fullmatch(document["binding_digest"])
            or not isinstance(document.get("request_digest"), str)
            or not _HEX_32.fullmatch(document["request_digest"])
            or not isinstance(document.get("state_sha256"), str)
            or not _HEX_32.fullmatch(document["state_sha256"])
            or not _uint63(document.get("state_generation"))
        ):
            raise CompanionError("companion_message_invalid")
    elif message_type == "refresh_result":
        _require_keys(
            document,
            common
            | {
                "request_id",
                "task_digest",
                "state_generation",
                "ok",
                "error",
                "payload_length",
            },
        )
        ok = document.get("ok")
        error = document.get("error")
        if (
            not isinstance(document.get("request_id"), str)
            or not _HEX_16.fullmatch(document["request_id"])
            or not isinstance(document.get("task_digest"), str)
            or not _HEX_32.fullmatch(document["task_digest"])
            or not _uint63(document.get("state_generation"))
            or not isinstance(ok, bool)
            or (ok and error is not None)
            or (
                not ok
                and (
                    not isinstance(error, str)
                    or not _ERROR_CODE.fullmatch(error)
                    or error not in _RESULT_ERRORS
                )
            )
            or (ok and not payload)
            or (not ok and bool(payload))
        ):
            raise CompanionError("companion_message_invalid")
    else:
        raise CompanionError("companion_message_invalid")
    if message_type in {"refresh", "refresh_result"}:
        length = document.get("payload_length")
        if (
            isinstance(length, bool)
            or not isinstance(length, int)
            or length != len(payload)
            or length < 0
            or length
            > (
                MAX_SECRET_BYTES
                if message_type == "refresh"
                else MAX_COMPANION_PAYLOAD_BYTES
            )
        ):
            raise CompanionError("companion_message_invalid")
        if message_type == "refresh" and (
            not hmac.compare_digest(
                document["state_sha256"], hashlib.sha256(payload).hexdigest()
            )
            or not hmac.compare_digest(
                document["task_digest"], compute_task_digest(document)
            )
        ):
            raise CompanionError("companion_task_invalid")
    elif payload:
        raise CompanionError("companion_message_invalid")


def encode_inner(document: dict[str, Any], payload: bytes = b"") -> bytes:
    if not isinstance(payload, bytes) or len(payload) > MAX_COMPANION_PAYLOAD_BYTES:
        raise CompanionError("companion_message_invalid")
    _validate_message(document, payload)
    encoded = _canonical_json(document)
    plaintext = _INNER_HEADER.pack(len(encoded)) + encoded + payload
    if len(plaintext) + 16 > MAX_COMPANION_FRAME_BYTES:
        raise CompanionError("companion_frame_too_large")
    return plaintext


def decode_inner(plaintext: bytes) -> tuple[dict[str, Any], bytes]:
    if not isinstance(plaintext, bytes) or len(plaintext) < _INNER_HEADER.size:
        raise CompanionError("companion_message_invalid")
    (json_length,) = _INNER_HEADER.unpack(plaintext[: _INNER_HEADER.size])
    if json_length < 1 or json_length > MAX_COMPANION_JSON_BYTES:
        raise CompanionError("companion_message_invalid")
    boundary = _INNER_HEADER.size + json_length
    if boundary > len(plaintext):
        raise CompanionError("companion_message_invalid")
    try:
        document = decode_document(
            plaintext[_INNER_HEADER.size : boundary],
            maximum=MAX_COMPANION_JSON_BYTES,
        )
    except ProtocolError:
        raise CompanionError("companion_message_invalid") from None
    payload = plaintext[boundary:]
    _validate_message(document, payload)
    return document, payload


class EncryptedSession:
    """Direction-bound AES-GCM frames over one connected stream socket."""

    def __init__(
        self,
        connection: socket.socket,
        *,
        session_id: bytes,
        send_key: bytes,
        receive_key: bytes,
        send_direction: bytes,
        receive_direction: bytes,
    ):
        if (
            len(session_id) != 16
            or len(send_key) != 32
            or len(receive_key) != 32
            or send_direction not in {BROKER_TO_COMPANION, COMPANION_TO_BROKER}
            or receive_direction not in {BROKER_TO_COMPANION, COMPANION_TO_BROKER}
            or send_direction == receive_direction
        ):
            raise CompanionError("companion_session_invalid")
        self.connection = connection
        self.session_id = session_id
        self._send_cipher = AESGCM(send_key)
        self._receive_cipher = AESGCM(receive_key)
        self._send_direction = send_direction
        self._receive_direction = receive_direction
        self._send_sequence = 0
        self._receive_sequence = 0
        self._send_lock = threading.Lock()
        self._state_lock = threading.Lock()
        self._closed_event = threading.Event()
        self._closed = False

    @staticmethod
    def _nonce(direction: bytes, sequence: int) -> bytes:
        return direction + struct.pack(">Q", sequence)

    def _associated_data(
        self, direction: bytes, sequence: int, ciphertext_length: int
    ) -> bytes:
        return (
            FRAME_MAGIC
            + self.session_id
            + direction
            + struct.pack(">QI", sequence, ciphertext_length)
        )

    def send_message(self, document: dict[str, Any], payload: bytes = b"") -> None:
        plaintext = encode_inner(document, payload)
        with self._send_lock:
            with self._state_lock:
                if self._closed or self._send_sequence > (1 << 64) - 1:
                    raise CompanionError("companion_disconnected")
            sequence = self._send_sequence
            ciphertext_length = len(plaintext) + 16
            if ciphertext_length > MAX_COMPANION_FRAME_BYTES:
                raise CompanionError("companion_frame_too_large")
            ciphertext = self._send_cipher.encrypt(
                self._nonce(self._send_direction, sequence),
                plaintext,
                self._associated_data(
                    self._send_direction, sequence, ciphertext_length
                ),
            )
            try:
                _send_exact_bounded(
                    self.connection,
                    _FRAME_HEADER.pack(ciphertext_length, sequence) + ciphertext,
                    COMPANION_WRITE_TIMEOUT_SECONDS,
                    self._closed_event,
                )
            except CompanionError:
                # A partial frame cannot be retried with the same sequence/nonce.
                # Fail the whole session before any caller can attempt another send.
                self._shutdown_transport()
                try:
                    self.connection.close()
                except OSError:
                    pass
                raise
            self._send_sequence += 1

    def receive_message(self) -> tuple[dict[str, Any], bytes]:
        header = _read_exact(self.connection, _FRAME_HEADER.size)
        ciphertext_length, sequence = _FRAME_HEADER.unpack(header)
        if (
            ciphertext_length < 16
            or ciphertext_length > MAX_COMPANION_FRAME_BYTES
        ):
            raise CompanionError("companion_frame_invalid")
        if sequence != self._receive_sequence:
            raise CompanionError("companion_sequence_invalid")
        ciphertext = _read_exact(self.connection, ciphertext_length)
        try:
            plaintext = self._receive_cipher.decrypt(
                self._nonce(self._receive_direction, sequence),
                ciphertext,
                self._associated_data(
                    self._receive_direction, sequence, ciphertext_length
                ),
            )
        except InvalidTag:
            raise CompanionError("companion_auth_failed") from None
        self._receive_sequence += 1
        return decode_inner(plaintext)

    def _shutdown_transport(self) -> None:
        with self._state_lock:
            self._closed = True
            self._closed_event.set()
        try:
            self.connection.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass

    def close(self) -> None:
        # shutdown must precede the send lock: a peer that stopped reading may
        # otherwise keep send_message in the kernel while close waits behind it.
        self._shutdown_transport()
        with self._send_lock:
            try:
                self.connection.close()
            except OSError:
                pass


def server_handshake(
    connection: socket.socket,
    *,
    epoch: bytes,
    epoch_key: bytes,
    broker_boot: bytes,
) -> EncryptedSession:
    if len(epoch) != 32 or len(epoch_key) != 32 or len(broker_boot) != 16:
        raise CompanionError("companion_session_invalid")
    broker_nonce = secrets.token_bytes(32)
    challenge = _CHALLENGE.pack(
        CHALLENGE_MAGIC, epoch, broker_boot, broker_nonce
    )
    try:
        connection.sendall(challenge)
    except OSError:
        raise CompanionError("companion_disconnected") from None
    encoded_client = _read_exact(connection, _CLIENT_PROOF.size)
    magic, companion_instance, companion_nonce, client_mac = _CLIENT_PROOF.unpack(
        encoded_client
    )
    client_header = CLIENT_MAGIC + companion_instance + companion_nonce
    expected_client_mac = hmac.digest(
        epoch_key, CLIENT_PROOF_DOMAIN + challenge + client_header, "sha256"
    )
    if magic != CLIENT_MAGIC or not hmac.compare_digest(
        client_mac, expected_client_mac
    ):
        raise CompanionError("companion_auth_failed")
    session_id = secrets.token_bytes(16)
    server_header = SERVER_MAGIC + session_id
    server_mac = hmac.digest(
        epoch_key,
        SERVER_PROOF_DOMAIN
        + challenge
        + client_header
        + client_mac
        + server_header,
        "sha256",
    )
    try:
        connection.sendall(_SERVER_PROOF.pack(SERVER_MAGIC, session_id, server_mac))
    except OSError:
        raise CompanionError("companion_disconnected") from None
    broker_key, companion_key = _derive_session_keys(
        epoch_key,
        epoch,
        broker_boot,
        broker_nonce,
        companion_instance,
        companion_nonce,
        session_id,
    )
    return EncryptedSession(
        connection,
        session_id=session_id,
        send_key=broker_key,
        receive_key=companion_key,
        send_direction=BROKER_TO_COMPANION,
        receive_direction=COMPANION_TO_BROKER,
    )


def client_handshake(
    connection: socket.socket,
    *,
    epoch_key: bytes,
    expected_epoch_id: str | None = None,
    companion_instance: bytes | None = None,
    companion_nonce: bytes | None = None,
) -> tuple[str, EncryptedSession]:
    """Reference client handshake used by interoperability tests."""

    if len(epoch_key) != 32:
        raise CompanionError("companion_session_invalid")
    challenge = _read_exact(connection, _CHALLENGE.size)
    magic, epoch, broker_boot, broker_nonce = _CHALLENGE.unpack(challenge)
    if magic != CHALLENGE_MAGIC:
        raise CompanionError("companion_auth_failed")
    epoch_id = EPOCH_PREFIX + _raw_url_encode(epoch)
    if expected_epoch_id is not None and epoch_id != expected_epoch_id:
        raise CompanionError("companion_auth_failed")
    companion_instance = companion_instance or secrets.token_bytes(16)
    companion_nonce = companion_nonce or secrets.token_bytes(32)
    if len(companion_instance) != 16 or len(companion_nonce) != 32:
        raise CompanionError("companion_session_invalid")
    client_header = CLIENT_MAGIC + companion_instance + companion_nonce
    client_mac = hmac.digest(
        epoch_key, CLIENT_PROOF_DOMAIN + challenge + client_header, "sha256"
    )
    try:
        connection.sendall(
            _CLIENT_PROOF.pack(
                CLIENT_MAGIC, companion_instance, companion_nonce, client_mac
            )
        )
    except OSError:
        raise CompanionError("companion_disconnected") from None
    encoded_server = _read_exact(connection, _SERVER_PROOF.size)
    server_magic, session_id, server_mac = _SERVER_PROOF.unpack(encoded_server)
    server_header = SERVER_MAGIC + session_id
    expected_server_mac = hmac.digest(
        epoch_key,
        SERVER_PROOF_DOMAIN
        + challenge
        + client_header
        + client_mac
        + server_header,
        "sha256",
    )
    if server_magic != SERVER_MAGIC or not hmac.compare_digest(
        server_mac, expected_server_mac
    ):
        raise CompanionError("companion_auth_failed")
    broker_key, companion_key = _derive_session_keys(
        epoch_key,
        epoch,
        broker_boot,
        broker_nonce,
        companion_instance,
        companion_nonce,
        session_id,
    )
    return epoch_id, EncryptedSession(
        connection,
        session_id=session_id,
        send_key=companion_key,
        receive_key=broker_key,
        send_direction=COMPANION_TO_BROKER,
        receive_direction=BROKER_TO_COMPANION,
    )


def _length_prefixed(value: str) -> bytes:
    encoded = value.encode("ascii")
    return struct.pack(">I", len(encoded)) + encoded


def compute_task_digest(document: dict[str, Any]) -> str:
    fields = (
        "request_id",
        "context_id",
        "project_id",
        "provider",
        "record_id",
        "grant_revision",
        "driver_id",
        "driver_revision",
        "binding_digest",
        "request_digest",
        "state_sha256",
    )
    try:
        material = TASK_DIGEST_DOMAIN + b"".join(
            _length_prefixed(document[field]) for field in fields
        )
        material += _U64_PAIR.pack(
            document["state_generation"], document["deadline_unix_ms"]
        )
    except (KeyError, AttributeError, UnicodeEncodeError, struct.error, TypeError):
        raise CompanionError("companion_task_invalid") from None
    return hashlib.sha256(material).hexdigest()


@dataclass(frozen=True)
class RefreshRequest:
    request_id: str
    deadline_unix_ms: int
    context_id: str
    project_id: str
    provider: str
    record_id: str
    grant_revision: str
    state_generation: int
    driver_id: str
    driver_revision: str
    binding_digest: str
    request_digest: str
    state: bytes = field(repr=False)
    task_digest: str = ""

    @classmethod
    def create(
        cls,
        *,
        context_id: str,
        project_id: str,
        provider: str,
        record_id: str,
        grant_revision: str,
        state_generation: int,
        driver_id: str,
        driver_revision: str,
        binding_digest: str,
        request_digest: str,
        state: bytes,
        timeout_seconds: float = DEFAULT_REFRESH_SECONDS,
    ) -> "RefreshRequest":
        timeout_seconds = max(0.0, min(float(timeout_seconds), MAX_REFRESH_SECONDS))
        request = cls(
            request_id=secrets.token_hex(16),
            deadline_unix_ms=int((time.time() + timeout_seconds) * 1000),
            context_id=context_id,
            project_id=project_id,
            provider=provider,
            record_id=record_id,
            grant_revision=grant_revision,
            state_generation=state_generation,
            driver_id=driver_id,
            driver_revision=driver_revision,
            binding_digest=binding_digest,
            request_digest=request_digest,
            state=state,
        )
        document = request._document("")
        return cls(**{**request.__dict__, "task_digest": compute_task_digest(document)})

    def _document(self, digest: str) -> dict[str, Any]:
        if not isinstance(self.state, bytes) or not self.state or len(self.state) > MAX_SECRET_BYTES:
            raise CompanionError("companion_task_invalid")
        return {
            "protocol_version": COMPANION_PROTOCOL_VERSION,
            "type": "refresh",
            "request_id": self.request_id,
            "deadline_unix_ms": self.deadline_unix_ms,
            "task_digest": digest,
            "context_id": self.context_id,
            "project_id": self.project_id,
            "provider": self.provider,
            "record_id": self.record_id,
            "grant_revision": self.grant_revision,
            "state_generation": self.state_generation,
            "driver_id": self.driver_id,
            "driver_revision": self.driver_revision,
            "binding_digest": self.binding_digest,
            "request_digest": self.request_digest,
            "state_sha256": hashlib.sha256(self.state).hexdigest(),
            "payload_length": len(self.state),
        }

    def document(self) -> dict[str, Any]:
        document = self._document(self.task_digest)
        expected = compute_task_digest(document)
        if not self.task_digest or not hmac.compare_digest(self.task_digest, expected):
            raise CompanionError("companion_task_invalid")
        _validate_message(document, self.state)
        return document


@dataclass(frozen=True)
class RefreshResult:
    request_id: str
    task_digest: str
    state_generation: int
    secret_payload: bytes = field(repr=False)


@dataclass(frozen=True)
class RefreshSecret:
    """Strict request-local lease plus the updated opaque driver state."""

    state: bytes = field(repr=False)
    access_key_id: str = field(repr=False)
    secret_access_key: str = field(repr=False)
    session_token: str = field(repr=False)
    expiration_unix_ms: int


def encode_refresh_secret(secret: RefreshSecret) -> bytes:
    if (
        not isinstance(secret.state, bytes)
        or not secret.state
        or len(secret.state) > MAX_SECRET_BYTES
        or not isinstance(secret.access_key_id, str)
        or not _ACCESS_KEY.fullmatch(secret.access_key_id)
        or not isinstance(secret.secret_access_key, str)
        or not 20 <= len(secret.secret_access_key) <= 128
        or not _SECRET_VALUE.fullmatch(secret.secret_access_key)
        or not isinstance(secret.session_token, str)
        or not 16 <= len(secret.session_token) <= 16384
        or not _SECRET_VALUE.fullmatch(secret.session_token)
        or not _uint63(secret.expiration_unix_ms)
    ):
        raise CompanionError("companion_result_invalid")
    document = {
        "schema_version": 1,
        "state_base64url": _raw_url_encode(secret.state),
        "credentials": {
            "version": 1,
            "access_key_id": secret.access_key_id,
            "secret_access_key": secret.secret_access_key,
            "session_token": secret.session_token,
            "expiration_unix_ms": secret.expiration_unix_ms,
        },
    }
    try:
        encoded = json.dumps(
            document,
            ensure_ascii=True,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("ascii")
    except (TypeError, ValueError, UnicodeEncodeError):
        raise CompanionError("companion_result_invalid") from None
    if not encoded or len(encoded) > MAX_COMPANION_PAYLOAD_BYTES:
        raise CompanionError("companion_result_invalid")
    return encoded


def decode_refresh_secret(
    payload: bytes, *, clock: Any = time.time
) -> RefreshSecret:
    if (
        not isinstance(payload, bytes)
        or not payload
        or len(payload) > MAX_COMPANION_PAYLOAD_BYTES
    ):
        raise CompanionError("companion_result_invalid")
    try:
        document = decode_document(payload, maximum=MAX_COMPANION_PAYLOAD_BYTES)
    except ProtocolError:
        raise CompanionError("companion_result_invalid") from None
    if set(document) != {"schema_version", "state_base64url", "credentials"}:
        raise CompanionError("companion_result_invalid")
    credentials = document.get("credentials")
    if (
        document.get("schema_version") != 1
        or isinstance(document.get("schema_version"), bool)
        or not isinstance(credentials, dict)
        or set(credentials)
        != {
            "version",
            "access_key_id",
            "secret_access_key",
            "session_token",
            "expiration_unix_ms",
        }
    ):
        raise CompanionError("companion_result_invalid")
    state_value = document.get("state_base64url")
    if not isinstance(state_value, str):
        raise CompanionError("companion_result_invalid")
    try:
        state_encoded = state_value.encode("ascii")
        state = base64.b64decode(
            state_encoded + b"=" * (-len(state_encoded) % 4),
            altchars=b"-_",
            validate=True,
        )
    except (UnicodeEncodeError, ValueError):
        raise CompanionError("companion_result_invalid") from None
    secret = RefreshSecret(
        state=state,
        access_key_id=credentials.get("access_key_id"),
        secret_access_key=credentials.get("secret_access_key"),
        session_token=credentials.get("session_token"),
        expiration_unix_ms=credentials.get("expiration_unix_ms"),
    )
    now_millis = int(clock() * 1000)
    if (
        _raw_url_encode(state) != state_value
        or not _uint63(secret.expiration_unix_ms)
        or secret.expiration_unix_ms < now_millis + 30_000
        or secret.expiration_unix_ms > now_millis + 12 * 60 * 60 * 1000
    ):
        raise CompanionError("companion_result_invalid")
    if encode_refresh_secret(secret) != payload:
        raise CompanionError("companion_result_invalid")
    return secret


class CompanionRefreshClient(Protocol):
    def refresh(
        self,
        request: RefreshRequest,
        cancel_event: threading.Event | None = None,
    ) -> RefreshResult: ...


@dataclass
class _PreparedEpoch:
    epoch_id: str
    raw: bytes
    key: bytearray = field(repr=False)
    generation: int


@dataclass
class _PendingRefresh:
    request: RefreshRequest
    event: threading.Event = field(default_factory=threading.Event)
    result: RefreshResult | None = None
    error: str | None = None
    accepted: bool = False
    sent: bool = False
    cancel_sent: bool = False
    cancel_acknowledged: bool = False


@dataclass
class _IgnoredRefresh:
    task_digest: str
    accepted: bool
    cancel_sent: bool
    cancel_acknowledged: bool
    terminal_seen: bool


@dataclass
class _PendingControl:
    expected_type: str
    event: threading.Event = field(default_factory=threading.Event)
    error: str | None = None


@dataclass
class _ActiveSession:
    epoch_id: str
    generation: int
    channel: EncryptedSession


class CompanionChannelManager:
    """Own one prepared epoch, one active session, and bounded refresh RPCs."""

    def __init__(self, *, broker_boot: bytes | None = None):
        self._broker_boot = broker_boot or secrets.token_bytes(16)
        if len(self._broker_boot) != 16:
            raise ValueError("invalid broker boot identifier")
        self._lock = threading.RLock()
        self._prepared: _PreparedEpoch | None = None
        self._generation = 0
        self._accepting: int | None = None
        self._active: _ActiveSession | None = None
        self._pending: dict[str, _PendingRefresh] = {}
        self._controls: dict[str, _PendingControl] = {}
        self._ignored_queue: deque[str] = deque()
        self._ignored: dict[str, _IgnoredRefresh] = {}
        self._draining = False

    @staticmethod
    def _wipe(value: bytearray | None) -> None:
        if value is None:
            return
        for index in range(len(value)):
            value[index] = 0

    def status(self) -> tuple[str, str]:
        with self._lock:
            if self._active is not None:
                return "ready", self._active.epoch_id
            if self._prepared is not None:
                return "prepared", self._prepared.epoch_id
            return "absent", ""

    def prepare(self, epoch_id: Any, epoch_key: bytes) -> None:
        raw = decode_epoch_id(epoch_id)
        if not isinstance(epoch_key, bytes) or len(epoch_key) != 32:
            raise CompanionError("invalid_key")
        with self._lock:
            prior_prepared = self._prepared
            prior_active = self._active
            self._generation += 1
            self._prepared = _PreparedEpoch(
                epoch_id, raw, bytearray(epoch_key), self._generation
            )
            self._active = None
            self._accepting = None
            self._draining = False
            self._fail_all_locked("companion_disconnected")
            self._ignored_queue.clear()
            self._ignored.clear()
        if prior_prepared is not None:
            self._wipe(prior_prepared.key)
        if prior_active is not None:
            prior_active.channel.close()

    def invalidate(self) -> None:
        with self._lock:
            prepared = self._prepared
            active = self._active
            self._generation += 1
            self._prepared = None
            self._active = None
            self._accepting = None
            self._draining = False
            self._fail_all_locked("companion_disconnected")
            self._ignored_queue.clear()
            self._ignored.clear()
        if prepared is not None:
            self._wipe(prepared.key)
        if active is not None:
            active.channel.close()

    def serve_connection(self, connection: socket.socket) -> None:
        """Authenticate and serve one bridge connection without logging it."""

        channel: EncryptedSession | None = None
        active: _ActiveSession | None = None
        with self._lock:
            prepared = self._prepared
            if (
                prepared is None
                or self._active is not None
                or self._accepting is not None
            ):
                try:
                    connection.close()
                except OSError:
                    pass
                return
            self._accepting = prepared.generation
            epoch_id = prepared.epoch_id
            epoch = prepared.raw
            epoch_key = bytes(prepared.key)
            generation = prepared.generation
        try:
            connection.settimeout(HANDSHAKE_TIMEOUT_SECONDS)
            channel = server_handshake(
                connection,
                epoch=epoch,
                epoch_key=epoch_key,
                broker_boot=self._broker_boot,
            )
            session_hex = channel.session_id.hex()
            channel.send_message(
                {
                    "protocol_version": COMPANION_PROTOCOL_VERSION,
                    "type": "ready",
                    "session_id": session_hex,
                }
            )
            acknowledgment, payload = channel.receive_message()
            if payload or acknowledgment != {
                "protocol_version": COMPANION_PROTOCOL_VERSION,
                "type": "ready_ack",
                "session_id": session_hex,
            }:
                raise CompanionError("companion_message_invalid")
            connection.settimeout(None)
            with self._lock:
                if (
                    self._generation != generation
                    or self._prepared is None
                    or self._prepared.generation != generation
                    or self._active is not None
                ):
                    raise CompanionError("companion_session_stale")
                active = _ActiveSession(epoch_id, generation, channel)
                self._active = active
                self._wipe(self._prepared.key)
                self._prepared = None
                self._draining = False
            while True:
                document, response_payload = channel.receive_message()
                self._receive(active, document, response_payload)
        except Exception:
            # The bridge carries secrets.  Never render exception text.
            pass
        finally:
            with self._lock:
                if self._accepting == generation:
                    self._accepting = None
            if active is not None:
                self._disconnect(active, "companion_disconnected")
            elif channel is not None:
                channel.close()
            else:
                try:
                    connection.close()
                except OSError:
                    pass

    def _remember_ignored_locked(
        self, pending: _PendingRefresh, *, terminal_seen: bool
    ) -> None:
        request_id = pending.request.request_id
        if request_id in self._ignored:
            return
        if len(self._ignored_queue) >= MAX_IGNORED_REFRESHES:
            expired = self._ignored_queue.popleft()
            self._ignored.pop(expired, None)
        self._ignored_queue.append(request_id)
        self._ignored[request_id] = _IgnoredRefresh(
            task_digest=pending.request.task_digest,
            accepted=pending.accepted,
            cancel_sent=pending.cancel_sent,
            cancel_acknowledged=pending.cancel_acknowledged,
            terminal_seen=terminal_seen,
        )

    def _fail_all_locked(self, code: str) -> None:
        for pending in self._pending.values():
            pending.error = "companion_outcome_unknown" if pending.sent else code
            pending.event.set()
        self._pending.clear()
        for pending in self._controls.values():
            pending.error = code
            pending.event.set()
        self._controls.clear()

    def _disconnect(self, active: _ActiveSession, code: str) -> None:
        with self._lock:
            if self._active is active:
                self._active = None
                self._draining = False
                self._fail_all_locked(code)
        active.channel.close()

    def _receive(
        self,
        active: _ActiveSession,
        document: dict[str, Any],
        payload: bytes,
    ) -> None:
        message_type = document["type"]
        if message_type == "ping":
            active.channel.send_message(
                {
                    "protocol_version": COMPANION_PROTOCOL_VERSION,
                    "type": "pong",
                    "request_id": document["request_id"],
                }
            )
            return
        if message_type in {"pong", "drain_ack"}:
            request_id = document["request_id"]
            with self._lock:
                pending_control = self._controls.pop(request_id, None)
                if (
                    pending_control is None
                    or pending_control.expected_type != message_type
                ):
                    raise CompanionError("companion_message_invalid")
                pending_control.event.set()
            return
        if message_type in {
            "refresh_accepted",
            "refresh_result",
            "cancel_ack",
        }:
            request_id = document["request_id"]
            with self._lock:
                ignored = self._ignored.get(request_id)
                if ignored is not None:
                    if not hmac.compare_digest(
                        ignored.task_digest, document["task_digest"]
                    ):
                        raise CompanionError("companion_message_invalid")
                    if message_type == "refresh_accepted":
                        if ignored.accepted or ignored.terminal_seen:
                            raise CompanionError("companion_message_invalid")
                        ignored.accepted = True
                    elif message_type == "cancel_ack":
                        if (
                            not ignored.accepted
                            or not ignored.cancel_sent
                            or ignored.cancel_acknowledged
                        ):
                            raise CompanionError("companion_message_invalid")
                        ignored.cancel_acknowledged = True
                    else:
                        if not ignored.accepted or ignored.terminal_seen:
                            raise CompanionError("companion_message_invalid")
                        ignored.terminal_seen = True
                    return
                pending = self._pending.get(request_id)
                if pending is None or not hmac.compare_digest(
                    pending.request.task_digest, document["task_digest"]
                ):
                    raise CompanionError("companion_message_invalid")
                if message_type == "refresh_accepted":
                    if pending.accepted:
                        raise CompanionError("companion_message_invalid")
                    pending.accepted = True
                    return
                if message_type == "cancel_ack":
                    if (
                        not pending.accepted
                        or not pending.cancel_sent
                        or pending.cancel_acknowledged
                    ):
                        raise CompanionError("companion_message_invalid")
                    pending.cancel_acknowledged = True
                    return
                if not pending.accepted:
                    raise CompanionError("companion_message_invalid")
                if document["state_generation"] != pending.request.state_generation:
                    raise CompanionError("companion_message_invalid")
                if document["ok"]:
                    pending.result = RefreshResult(
                        request_id=request_id,
                        task_digest=document["task_digest"],
                        state_generation=document["state_generation"],
                        secret_payload=payload,
                    )
                else:
                    pending.error = "companion_" + document["error"]
                del self._pending[request_id]
                if pending.cancel_sent:
                    self._remember_ignored_locked(pending, terminal_seen=True)
                pending.event.set()
            return
        raise CompanionError("companion_message_invalid")

    def _request_cancel(
        self, active: _ActiveSession, pending: _PendingRefresh
    ) -> bool:
        should_send = False
        with self._lock:
            current = self._pending.get(pending.request.request_id)
            if current is pending:
                if not pending.accepted:
                    return False
                if not pending.cancel_sent:
                    pending.cancel_sent = True
                    should_send = True
            else:
                return False
        if should_send:
            try:
                active.channel.send_message(
                    {
                        "protocol_version": COMPANION_PROTOCOL_VERSION,
                        "type": "cancel",
                        "request_id": pending.request.request_id,
                        "task_digest": pending.request.task_digest,
                    }
                )
            except CompanionError:
                self._disconnect(active, "companion_disconnected")
                raise CompanionError("companion_outcome_unknown") from None
        return True

    def _outcome_unknown(self, pending: _PendingRefresh) -> None:
        with self._lock:
            current = self._pending.get(pending.request.request_id)
            if current is pending:
                del self._pending[pending.request.request_id]
                self._remember_ignored_locked(pending, terminal_seen=False)
        raise CompanionError("companion_outcome_unknown")

    def refresh(
        self,
        request: RefreshRequest,
        cancel_event: threading.Event | None = None,
    ) -> RefreshResult:
        document = request.document()
        if cancel_event is not None and cancel_event.is_set():
            raise CompanionError("companion_cancelled")
        remaining = min(
            MAX_REFRESH_SECONDS,
            (request.deadline_unix_ms - int(time.time() * 1000)) / 1000.0,
        )
        if remaining <= 0:
            raise CompanionError("companion_timeout")
        pending = _PendingRefresh(request)
        with self._lock:
            active = self._active
            if active is None or self._draining:
                raise CompanionError("companion_unavailable")
            if len(self._pending) >= MAX_PENDING_REFRESHES:
                raise CompanionError("companion_busy")
            if request.request_id in self._pending or request.request_id in self._ignored:
                raise CompanionError("companion_task_invalid")
            self._pending[request.request_id] = pending
            # Once send begins, a partial write or lost acknowledgment cannot
            # prove that provider execution did not start.
            pending.sent = True
        try:
            active.channel.send_message(document, request.state)
        except CompanionError:
            self._disconnect(active, "companion_disconnected")
            raise CompanionError("companion_outcome_unknown") from None
        deadline = time.monotonic() + remaining
        resolution_deadline: float | None = None
        cancel_requested = False
        while True:
            wait_deadline = (
                resolution_deadline
                if resolution_deadline is not None
                else deadline
            )
            wait_for = min(0.05, max(0.0, wait_deadline - time.monotonic()))
            if pending.event.wait(wait_for):
                if pending.error is not None:
                    raise CompanionError(pending.error)
                if pending.result is None:
                    raise CompanionError("companion_response_invalid")
                return pending.result
            if pending.event.is_set():
                continue
            now = time.monotonic()
            if not cancel_requested:
                if cancel_event is not None and cancel_event.is_set():
                    cancel_requested = True
                elif now >= deadline:
                    cancel_requested = True
                if cancel_requested:
                    resolution_deadline = now + MAX_CANCEL_RESOLUTION_SECONDS
            if cancel_requested:
                self._request_cancel(active, pending)
            if resolution_deadline is not None and now >= resolution_deadline:
                if pending.event.is_set():
                    continue
                self._outcome_unknown(pending)

    def _control_call(self, request_type: str, response_type: str, timeout: float) -> None:
        request_id = secrets.token_hex(16)
        pending = _PendingControl(response_type)
        with self._lock:
            active = self._active
            if active is None:
                raise CompanionError("companion_unavailable")
            self._controls[request_id] = pending
        document: dict[str, Any] = {
            "protocol_version": COMPANION_PROTOCOL_VERSION,
            "type": request_type,
            "request_id": request_id,
        }
        if request_type == "drain":
            document["deadline_unix_ms"] = int((time.time() + timeout) * 1000)
        try:
            active.channel.send_message(document)
        except CompanionError:
            self._disconnect(active, "companion_disconnected")
            raise CompanionError("companion_disconnected") from None
        if not pending.event.wait(max(0.0, min(timeout, MAX_REFRESH_SECONDS))):
            with self._lock:
                self._controls.pop(request_id, None)
            self._disconnect(active, "companion_disconnected")
            raise CompanionError("companion_timeout")
        if pending.error is not None:
            raise CompanionError(pending.error)

    def ping(self, timeout: float = 2.0) -> None:
        self._control_call("ping", "pong", timeout)

    def drain(self, timeout: float = 2.0) -> None:
        with self._lock:
            if self._active is None:
                return
            self._draining = True
        self._control_call("drain", "drain_ack", timeout)

    def close(self, timeout: float = 2.0) -> None:
        try:
            self.drain(timeout)
        except CompanionError:
            pass
        self.invalidate()
