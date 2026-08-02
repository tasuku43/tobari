"""Secret-safe integration upstream for Tobari.

The response exposes only a digest and presence bit for Authorization. Logs
contain request shape but never headers or bodies.
"""

from __future__ import annotations

import hashlib
import json
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


class Handler(BaseHTTPRequestHandler):
    server_version = "TobariMock/1"

    def do_GET(self) -> None:
        self._reply()

    def do_HEAD(self) -> None:
        self._reply(include_body=False)

    def do_POST(self) -> None:
        self._reply()

    def do_PUT(self) -> None:
        self._reply()

    def _reply(self, include_body: bool = True) -> None:
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
