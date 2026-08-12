"""No-TCP Auth Broker daemon serving private Unix-domain sockets."""

from __future__ import annotations

import argparse
import errno
import os
import signal
import socketserver
import stat
import threading
from pathlib import Path
from typing import Any

from .broker import BrokerError, BrokerState, Dispatcher
from .protocol import ConnectionReader, ProtocolError, encode_document, error_document
from .vault import VaultStore

DEFAULT_RUNTIME_SOCKET = "/run/tobari-auth/runtime/broker.sock"
DEFAULT_CONTROL_SOCKET = "/run/tobari-auth/control/broker.sock"
DEFAULT_VAULT_ROOT = "/var/lib/tobari-auth/contexts"


def _owned_by_service(info: os.stat_result) -> bool:
    effective_uid = os.geteuid()
    # Docker Desktop/Colima can project an owner-only macOS bind mount as UID
    # zero inside its VM while still enforcing the host user's access. The
    # trusted host validates the source owner and mode before Compose starts.
    # Accept that projection only for a non-root broker; the entrypoint rejects
    # actual root execution.
    return info.st_uid == effective_uid or (effective_uid != 0 and info.st_uid == 0)


def _prepare_socket_directory(socket_path: str) -> bool:
    directory = Path(socket_path).parent
    try:
        directory.mkdir(mode=0o700, parents=True, exist_ok=True)
        info = directory.lstat()
    except OSError:
        raise RuntimeError("socket directory is unavailable") from None
    if (
        not stat.S_ISDIR(info.st_mode)
        or stat.S_ISLNK(info.st_mode)
        or not _owned_by_service(info)
        or stat.S_IMODE(info.st_mode) != 0o700
    ):
        raise RuntimeError("socket directory is unsafe")
    virtualized_root = info.st_uid == 0 and os.geteuid() != 0
    try:
        existing = os.lstat(socket_path)
    except FileNotFoundError:
        return virtualized_root
    except OSError:
        raise RuntimeError("socket path is unavailable") from None
    if not stat.S_ISSOCK(existing.st_mode) or not _owned_by_service(existing):
        raise RuntimeError("socket path is unsafe")
    try:
        os.unlink(socket_path)
    except OSError:
        raise RuntimeError("stale socket could not be removed") from None
    return virtualized_root


def _protect_socket(path: str, virtualized_root: bool) -> None:
    try:
        os.chmod(path, 0o600)
        return
    except OSError as error:
        # macOS VirtioFS permits creating and sharing a Unix socket in the
        # owner-only bind source but rejects chmod(2) with EINVAL and reports
        # the socket as 0755. The enclosing host-validated 0700 directory is
        # still the access boundary. No other chmod failure is accepted.
        if error.errno != errno.EINVAL or not virtualized_root:
            raise RuntimeError("runtime socket could not be protected") from None
    try:
        info = os.lstat(path)
    except OSError:
        raise RuntimeError("runtime socket could not be verified") from None
    if (
        not stat.S_ISSOCK(info.st_mode)
        or not _owned_by_service(info)
        or stat.S_IMODE(info.st_mode) != 0o755
    ):
        raise RuntimeError("runtime socket is unsafe")


class _BrokerRequestHandler(socketserver.BaseRequestHandler):
    def handle(self) -> None:
        reader = ConnectionReader(self.request)
        response: dict[str, Any]
        try:
            request = reader.read_frame()
            dispatcher: Dispatcher = self.server.dispatcher  # type: ignore[attr-defined]
            raw_length = dispatcher.expected_raw_length(request)
            raw_payload = reader.read_exact(raw_length) if raw_length else b""
            response = dispatcher.dispatch(request, raw_payload)
        except (BrokerError, ProtocolError) as error:
            response = error_document(error.code)
        except Exception:
            # Never include exception text because dependency diagnostics may
            # contain secret-bearing values.
            response = error_document("internal_error")
        try:
            self.request.sendall(encode_document(response))
        except (OSError, ProtocolError):
            return


class _UnixServer(socketserver.ThreadingUnixStreamServer):
    daemon_threads = True
    allow_reuse_address = False

    def __init__(self, path: str, dispatcher: Dispatcher):
        self.dispatcher = dispatcher
        virtualized_root = _prepare_socket_directory(path)
        old_umask = os.umask(0o077)
        try:
            super().__init__(path, _BrokerRequestHandler)
        finally:
            os.umask(old_umask)
        try:
            _protect_socket(path, virtualized_root)
        except Exception:
            self.server_close()
            try:
                os.unlink(path)
            except OSError:
                pass
            raise


def serve(
    runtime_socket: str,
    control_socket: str,
    vault_root: str,
) -> int:
    state = BrokerState(VaultStore(vault_root))
    runtime_server = _UnixServer(runtime_socket, Dispatcher(state, "runtime"))
    try:
        control_server = _UnixServer(control_socket, Dispatcher(state, "control"))
    except Exception:
        runtime_server.server_close()
        try:
            os.unlink(runtime_socket)
        except OSError:
            pass
        raise
    stopping = threading.Event()

    def stop(_: int, __: Any) -> None:
        stopping.set()

    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    threads = [
        threading.Thread(target=runtime_server.serve_forever, name="runtime-socket"),
        threading.Thread(target=control_server.serve_forever, name="control-socket"),
    ]
    for thread in threads:
        thread.start()
    try:
        while not stopping.wait(0.25):
            pass
    finally:
        runtime_server.shutdown()
        control_server.shutdown()
        runtime_server.server_close()
        control_server.server_close()
        for thread in threads:
            thread.join()
        for path in (runtime_socket, control_socket):
            try:
                os.unlink(path)
            except FileNotFoundError:
                pass
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--runtime-socket", default=DEFAULT_RUNTIME_SOCKET)
    parser.add_argument("--control-socket", default=DEFAULT_CONTROL_SOCKET)
    parser.add_argument("--vault-root", default=DEFAULT_VAULT_ROOT)
    arguments = parser.parse_args(argv)
    return serve(
        arguments.runtime_socket,
        arguments.control_socket,
        arguments.vault_root,
    )


if __name__ == "__main__":
    raise SystemExit(main())
