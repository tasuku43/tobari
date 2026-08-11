"""Bounded non-recursive DNS for Tobari's transparent HTTP ingress."""

from __future__ import annotations

import argparse
import ipaddress
import socket
import socketserver
import struct
import threading
import time
from dataclasses import dataclass

SYNTHETIC_IPV4 = ipaddress.IPv4Address("198.18.0.10")
DEFAULT_PORT = 15053
MAX_UDP_REQUEST = 512
MAX_TCP_REQUEST = 4096
MAX_SOURCES = 1024
RATE_WINDOW_SECONDS = 1.0
RATE_REQUESTS_PER_WINDOW = 64
MAX_ACTIVE_REQUESTS = 32


class DNSMessageError(Exception):
    """A DNS message is malformed or exceeds Tobari's fixed contract."""


def _question_end(message: bytes) -> int:
    offset = 12
    wire_name_bytes = 0
    labels = 0
    while True:
        if offset >= len(message):
            raise DNSMessageError("truncated question name")
        length = message[offset]
        offset += 1
        wire_name_bytes += 1
        if length == 0:
            break
        if length & 0xC0 or length > 63 or offset + length > len(message):
            raise DNSMessageError("invalid question label")
        labels += 1
        wire_name_bytes += length
        if labels > 127 or wire_name_bytes > 255:
            raise DNSMessageError("question name is too large")
        offset += length
    if labels == 0 or offset + 4 > len(message):
        raise DNSMessageError("question shape is invalid")
    return offset + 4


def _validate_additional(message: bytes, offset: int, count: int) -> None:
    if count == 0:
        if offset != len(message):
            raise DNSMessageError("unexpected DNS message tail")
        return
    if count != 1 or len(message) - offset < 11 or message[offset] != 0:
        raise DNSMessageError("additional record shape is invalid")
    record_type, udp_size, ttl, data_length = struct.unpack(
        "!HHIH", message[offset + 1 : offset + 11]
    )
    if (
        record_type != 41
        or udp_size < 512
        or udp_size > MAX_TCP_REQUEST
        or ttl & ~0x8000
        or offset + 11 + data_length != len(message)
    ):
        raise DNSMessageError("EDNS record is invalid")


def build_response(message: bytes) -> bytes:
    """Return one bounded authoritative synthetic response without lookup."""
    if len(message) < 17 or len(message) > MAX_TCP_REQUEST:
        raise DNSMessageError("DNS message size is invalid")
    identifier, flags, questions, answers, authority, additional = struct.unpack(
        "!HHHHHH", message[:12]
    )
    if (
        flags & 0x8000
        or flags & 0x7800
        or flags & 0x0200
        or questions != 1
        or answers != 0
        or authority != 0
        or additional > 1
    ):
        raise DNSMessageError("DNS header is unsupported")
    end = _question_end(message)
    _validate_additional(message, end, additional)
    question = message[12:end]
    query_type, query_class = struct.unpack("!HH", message[end - 4 : end])
    response_flags = 0x8400 | (flags & 0x0100)  # response + authoritative + echoed RD
    if query_class != 1:
        response_flags |= 0x0005  # REFUSED
        return struct.pack("!HHHHHH", identifier, response_flags, 1, 0, 0, 0) + question
    if query_type != 1:
        # NOERROR/NODATA prompts ordinary dual-stack resolvers to retain the A
        # result without claiming support for AAAA/HTTPS/SVCB or other records.
        return struct.pack("!HHHHHH", identifier, response_flags, 1, 0, 0, 0) + question
    answer = struct.pack(
        "!HHHLH4s",
        0xC00C,
        1,
        1,
        0,
        4,
        SYNTHETIC_IPV4.packed,
    )
    return struct.pack("!HHHHHH", identifier, response_flags, 1, 1, 0, 0) + question + answer


@dataclass
class _SourceWindow:
    started: float
    count: int


class SourceLimiter:
    """Fixed-memory per-source request limiter."""

    def __init__(self) -> None:
        self._windows: dict[str, _SourceWindow] = {}
        self._lock = threading.Lock()

    def allow(self, source: str, now: float | None = None) -> bool:
        observed = time.monotonic() if now is None else now
        with self._lock:
            current = self._windows.get(source)
            if current is not None and observed - current.started < RATE_WINDOW_SECONDS:
                if current.count >= RATE_REQUESTS_PER_WINDOW:
                    return False
                current.count += 1
                return True
            if current is None and len(self._windows) >= MAX_SOURCES:
                expired = [
                    key
                    for key, window in self._windows.items()
                    if observed - window.started >= RATE_WINDOW_SECONDS
                ]
                for key in expired:
                    del self._windows[key]
                if len(self._windows) >= MAX_SOURCES:
                    return False
            self._windows[source] = _SourceWindow(observed, 1)
            return True


class _BoundedMixIn(socketserver.ThreadingMixIn):
    daemon_threads = True
    block_on_close = False
    request_queue_size = MAX_ACTIVE_REQUESTS

    def __init__(self, *args, limiter: SourceLimiter, **kwargs):
        self._slots = threading.BoundedSemaphore(MAX_ACTIVE_REQUESTS)
        self.limiter = limiter
        super().__init__(*args, **kwargs)

    def process_request(self, request, client_address):  # type: ignore[no-untyped-def]
        if not self._slots.acquire(blocking=False):
            self.shutdown_request(request)
            return
        super().process_request(request, client_address)

    def process_request_thread(self, request, client_address):  # type: ignore[no-untyped-def]
        try:
            super().process_request_thread(request, client_address)
        finally:
            self._slots.release()


def _allowed(server: object, source: str) -> bool:
    limiter = getattr(server, "limiter", None)
    return isinstance(limiter, SourceLimiter) and limiter.allow(source)


class UDPHandler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        message, transport = self.request
        if not _allowed(self.server, self.client_address[0]) or len(message) > MAX_UDP_REQUEST:
            return
        try:
            response = build_response(message)
        except DNSMessageError:
            return
        transport.sendto(response, self.client_address)


class TCPHandler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        if not _allowed(self.server, self.client_address[0]):
            return
        self.request.settimeout(1.0)
        length_bytes = self._read_exact(2)
        if length_bytes is None:
            return
        length = struct.unpack("!H", length_bytes)[0]
        if length < 17 or length > MAX_TCP_REQUEST:
            return
        message = self._read_exact(length)
        if message is None:
            return
        try:
            response = build_response(message)
        except DNSMessageError:
            return
        self.request.sendall(struct.pack("!H", len(response)) + response)

    def _read_exact(self, length: int) -> bytes | None:
        chunks = bytearray()
        while len(chunks) < length:
            try:
                chunk = self.request.recv(length - len(chunks))
            except (OSError, TimeoutError):
                return None
            if not chunk:
                return None
            chunks.extend(chunk)
        return bytes(chunks)


class UDPServer(_BoundedMixIn, socketserver.UDPServer):
    allow_reuse_address = True


class TCPServer(_BoundedMixIn, socketserver.TCPServer):
    allow_reuse_address = True


def _query(name: str = "health.invalid") -> bytes:
    labels = name.split(".")
    question = b"".join(bytes([len(label)]) + label.encode("ascii") for label in labels)
    return struct.pack("!HHHHHH", 0x5442, 0x0100, 1, 0, 0, 0) + question + b"\x00" + struct.pack("!HH", 1, 1)


def healthcheck(host: str, port: int) -> bool:
    query = _query()
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as client:
        client.settimeout(1.0)
        client.sendto(query, (host, port))
        response, _ = client.recvfrom(MAX_UDP_REQUEST)
    return response == build_response(query) and SYNTHETIC_IPV4.packed in response


def serve(host: str, port: int) -> None:
    limiter = SourceLimiter()
    udp = UDPServer((host, port), UDPHandler, limiter=limiter)
    tcp = TCPServer((host, port), TCPHandler, limiter=limiter)
    tcp_thread = threading.Thread(target=tcp.serve_forever, daemon=True)
    tcp_thread.start()
    try:
        udp.serve_forever()
    finally:
        udp.server_close()
        tcp.shutdown()
        tcp.server_close()
        tcp_thread.join(timeout=2.0)


def main() -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--check", action="store_true")
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=DEFAULT_PORT)
    arguments = parser.parse_args()
    if arguments.port < 1024 or arguments.port > 65535:
        return 2
    if arguments.check:
        return 0 if healthcheck(arguments.host, arguments.port) else 1
    serve(arguments.host, arguments.port)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
