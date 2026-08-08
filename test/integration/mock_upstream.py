"""Secret-safe integration upstream for Tobari.

The response exposes only a digest and presence bit for Authorization. Logs
contain request shape but never headers or bodies.
"""

from __future__ import annotations

import hashlib
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    server_version = "TobariMock/1"
    protocol_version = "HTTP/1.1"

    def do_GET(self) -> None:
        if self.path == "/stream-response":
            self._stream_response()
            return
        self._reply()

    def do_HEAD(self) -> None:
        self._reply(include_body=False)

    def do_POST(self) -> None:
        if self.path == "/stream-upload":
            self._stream_upload()
            return
        self._reply()

    def do_PUT(self) -> None:
        self._reply()

    def do_PATCH(self) -> None:
        self._reply()

    def _read_chunk(self) -> bytes | None:
        line = self.rfile.readline()
        if not line:
            return None
        size = int(line.split(b";", 1)[0].strip(), 16)
        if size == 0:
            while self.rfile.readline() not in (b"\r\n", b"", b"\n"):
                pass
            return None
        chunk = self.rfile.read(size)
        self.rfile.read(2)
        return chunk

    def _discard_request_body(self) -> None:
        if self.headers.get("transfer-encoding", "").lower() == "chunked":
            while self._read_chunk() is not None:
                pass
            return
        length = int(self.headers.get("content-length", "0"))
        if length:
            self.rfile.read(length)

    def _stream_upload(self) -> None:
        if self.headers.get("transfer-encoding", "").lower() != "chunked":
            self.send_error(400)
            return
        first = self._read_chunk()
        print(
            json.dumps(
                {
                    "event": "first_request_chunk",
                    "method": self.command,
                    "path": self.path,
                    "size": len(first or b""),
                },
                separators=(",", ":"),
                sort_keys=True,
            ),
            flush=True,
        )
        while self._read_chunk() is not None:
            pass
        self._reply(body_already_read=True)

    def _stream_response(self) -> None:
        self.send_response(200)
        self.send_header("content-type", "text/event-stream")
        self.send_header("transfer-encoding", "chunked")
        self.end_headers()
        for payload in (b"data: first\n\n", b"data: second\n\n"):
            self.wfile.write(f"{len(payload):x}\r\n".encode("ascii"))
            self.wfile.write(payload + b"\r\n")
            self.wfile.flush()
            if payload.startswith(b"data: first"):
                time.sleep(2)
        self.wfile.write(b"0\r\n\r\n")
        self.wfile.flush()

    def _reply(self, include_body: bool = True, body_already_read: bool = False) -> None:
        if not body_already_read:
            self._discard_request_body()
        authorization = self.headers.get("authorization")
        document = {
            "authorization_present": authorization is not None,
            "authorization_sha256": (
                hashlib.sha256(authorization.encode("utf-8")).hexdigest()
                if authorization is not None
                else None
            ),
            "method": self.command,
            "path": self.path,
        }
        body = json.dumps(document, separators=(",", ":")).encode("utf-8")
        self.send_response(200)
        self.send_header("content-type", "application/json")
        self.send_header("content-length", str(len(body)))
        self.end_headers()
        if include_body:
            self.wfile.write(body)
        print(
            json.dumps(
                {
                    "authorization_present": authorization is not None,
                    "method": self.command,
                    "path": self.path,
                },
                separators=(",", ":"),
                sort_keys=True,
            ),
            flush=True,
        )

    def log_message(self, _format: str, *_args: object) -> None:
        return


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", 8080), Handler).serve_forever()
