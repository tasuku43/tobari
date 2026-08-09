"""Fixed stdin/stdout to Broker-private Unix-socket byte pump.

This module intentionally has no frame parser, logger, provider logic, or
filesystem write path.  The authenticated protocol terminates in the Broker
and trusted host companion, not in this bridge.
"""

from __future__ import annotations

import os
import socket
import sys
import threading
from typing import BinaryIO

DEFAULT_COMPANION_SOCKET = "/run/tobari-auth/companion/bridge.sock"
COPY_BYTES = 8192


def _read_available(source: BinaryIO) -> bytes:
    """Read one available pipe chunk without waiting for COPY_BYTES or EOF."""

    try:
        descriptor = source.fileno()
    except (AttributeError, OSError, ValueError):
        # Keep the byte pump usable with in-memory streams in focused tests.
        return source.read(COPY_BYTES)
    return os.read(descriptor, COPY_BYTES)


def _stdin_to_socket(source: BinaryIO, connection: socket.socket) -> None:
    try:
        while True:
            chunk = _read_available(source)
            if not chunk:
                try:
                    connection.shutdown(socket.SHUT_WR)
                except OSError:
                    pass
                return
            connection.sendall(chunk)
    except (OSError, ValueError):
        try:
            connection.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass


def pump(
    source: BinaryIO, destination: BinaryIO, connection: socket.socket
) -> None:
    """Relay bounded chunks until the Broker side closes the session."""

    inbound = threading.Thread(
        target=_stdin_to_socket,
        args=(source, connection),
        name="companion-bridge-input",
        daemon=True,
    )
    inbound.start()
    try:
        while True:
            chunk = connection.recv(COPY_BYTES)
            if not chunk:
                return
            destination.write(chunk)
            destination.flush()
    except (OSError, ValueError):
        return
    finally:
        try:
            connection.shutdown(socket.SHUT_RDWR)
        except OSError:
            pass
        try:
            connection.close()
        except OSError:
            pass
        inbound.join(timeout=0.25)


def main(argv: list[str] | None = None) -> int:
    # The host invokes this module with fixed argv.  Do not add a path option:
    # the socket is an image-owned authority boundary.
    if argv is None:
        argv = sys.argv[1:]
    if argv:
        return 2
    connection = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    try:
        connection.connect(DEFAULT_COMPANION_SOCKET)
        pump(sys.stdin.buffer, sys.stdout.buffer, connection)
    except OSError:
        try:
            connection.close()
        except OSError:
            pass
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
