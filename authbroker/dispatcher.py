"""Strict Auth Broker runtime/control protocol routing."""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

from . import SCHEMA_VERSION
from .broker_contract import BrokerError, _translate_error
from .control_login import (
    DriverControlLogin,
    StaticControlLogin,
    control_login_payload_field,
    parse_control_login,
)
from .protocol import MAX_SECRET_BYTES, ProtocolError, require_exact_keys

if TYPE_CHECKING:
    from .broker import BrokerState


class Dispatcher:
    def __init__(self, state: BrokerState, interface: str):
        if interface not in {"runtime", "control"}:
            raise ValueError("invalid interface")
        self._state = state
        self._interface = interface

    def dispatch(self, request: dict[str, Any], raw_payload: bytes) -> dict[str, Any]:
        try:
            operation = request.get("op")
            if not isinstance(operation, str):
                raise ProtocolError("invalid_request")
            if self._interface == "runtime":
                return self._runtime(operation, request, raw_payload)
            return self._control(operation, request, raw_payload)
        except Exception as error:
            raise _translate_error(error) from None

    def expected_raw_length(self, request: dict[str, Any]) -> int:
        operation = request.get("op")
        if self._interface != "control" or operation not in {"unlock", "import", "login"}:
            return 0
        if operation == "unlock":
            key = "key_length"
        elif operation == "login":
            key = control_login_payload_field(request.get("provider"))
        else:
            key = "secret_length"
        value = request.get(key)
        if isinstance(value, bool) or not isinstance(value, int):
            raise BrokerError("invalid_length")
        maximum = 32 if operation == "unlock" else MAX_SECRET_BYTES
        if value < 1 or value > maximum or (operation == "unlock" and value != 32):
            raise BrokerError("invalid_length")
        return value

    def _runtime(
        self, operation: str, request: dict[str, Any], raw_payload: bytes
    ) -> dict[str, Any]:
        if raw_payload:
            raise ProtocolError("unexpected_payload")
        if operation == "health":
            require_exact_keys(request, {"schema_version", "op"})
            return {
                "schema_version": SCHEMA_VERSION,
                "ok": True,
                "state": "locked" if self._state.locked else "unlocked",
            }
        if operation == "introspect":
            require_exact_keys(
                request,
                {
                    "schema_version",
                    "op",
                    "handle",
                    "context_id",
                    "project_id",
                    "provider",
                    "target",
                    "source_header",
                    "source_format",
                },
            )
            return self._state.introspect(
                request["handle"],
                request["context_id"],
                request["project_id"],
                request["provider"],
                request["target"],
                request["source_header"],
                request["source_format"],
            )
        if operation == "resolve":
            require_exact_keys(
                request,
                {
                    "schema_version",
                    "op",
                    "handle",
                    "context_id",
                    "project_id",
                    "provider",
                    "revision",
                    "target",
                    "source_header",
                    "source_format",
                },
            )
            return self._state.resolve(
                request["handle"],
                request["context_id"],
                request["project_id"],
                request["provider"],
                request["revision"],
                request["target"],
                request["source_header"],
                request["source_format"],
            )
        if operation == "introspect_signing":
            require_exact_keys(
                request,
                {
                    "schema_version",
                    "op",
                    "handle",
                    "context_id",
                    "project_id",
                    "provider",
                    "target",
                    "binding",
                },
            )
            return self._state.introspect_signing(
                request["handle"],
                request["context_id"],
                request["project_id"],
                request["provider"],
                request["target"],
                request["binding"],
            )
        if operation == "sign_sigv4":
            require_exact_keys(
                request,
                {
                    "schema_version",
                    "op",
                    "handle",
                    "context_id",
                    "project_id",
                    "provider",
                    "revision",
                    "binding",
                    "request",
                },
            )
            return self._state.sign_sigv4(
                request["handle"],
                request["context_id"],
                request["project_id"],
                request["provider"],
                request["revision"],
                request["binding"],
                request["request"],
            )
        raise ProtocolError("unknown_operation")

    def _control(
        self, operation: str, request: dict[str, Any], raw_payload: bytes
    ) -> dict[str, Any]:
        if operation == "health":
            require_exact_keys(request, {"schema_version", "op"})
            if raw_payload:
                raise ProtocolError("unexpected_payload")
            return {
                "schema_version": SCHEMA_VERSION,
                "ok": True,
                "state": "locked" if self._state.locked else "unlocked",
            }
        if operation == "companion_prepare":
            require_exact_keys(request, {"schema_version", "op", "epoch_id"})
            if raw_payload:
                raise ProtocolError("unexpected_payload")
            return self._state.prepare_companion(request["epoch_id"])
        if operation == "companion_status":
            require_exact_keys(request, {"schema_version", "op"})
            if raw_payload:
                raise ProtocolError("unexpected_payload")
            return self._state.companion_status()
        if operation == "status":
            require_exact_keys(request, {"schema_version", "op", "context_id", "provider"})
            if raw_payload:
                raise ProtocolError("unexpected_payload")
            return self._state.status(request["context_id"], request["provider"])
        if operation == "unlock":
            require_exact_keys(request, {"schema_version", "op", "key_length"})
            if len(raw_payload) != 32:
                raise ProtocolError("invalid_length")
            return self._state.unlock(raw_payload)
        if operation == "import":
            require_exact_keys(
                request,
                {"schema_version", "op", "context_id", "provider", "secret_length"},
            )
            if len(raw_payload) != request["secret_length"]:
                raise ProtocolError("invalid_length")
            return self._state.import_secret(
                request["context_id"], request["provider"], raw_payload
            )
        if operation == "login":
            login = parse_control_login(request, raw_payload)
            if isinstance(login, StaticControlLogin):
                return self._state.import_secret(
                    login.context_id,
                    login.provider_id,
                    login.secret,
                    account_label=login.account_label,
                )
            if isinstance(login, DriverControlLogin):
                return self._state.login_driver(login)
            raise ProtocolError("invalid_request")
        if operation == "logout":
            require_exact_keys(request, {"schema_version", "op", "context_id", "provider"})
            if raw_payload:
                raise ProtocolError("unexpected_payload")
            return self._state.logout(request["context_id"], request["provider"])
        if operation == "issue_handle":
            require_exact_keys(
                request,
                {
                    "schema_version",
                    "op",
                    "context_id",
                    "project_id",
                    "provider",
                    "bindings",
                },
            )
            if raw_payload:
                raise ProtocolError("unexpected_payload")
            return self._state.issue_handle(
                request["context_id"],
                request["project_id"],
                request["provider"],
                request["bindings"],
            )
        if operation == "binding_status":
            require_exact_keys(
                request,
                {
                    "schema_version",
                    "op",
                    "context_id",
                    "project_id",
                    "provider",
                    "revision",
                    "bindings",
                },
            )
            if raw_payload:
                raise ProtocolError("unexpected_payload")
            return self._state.binding_status(
                request["context_id"],
                request["project_id"],
                request["provider"],
                request["revision"],
                request["bindings"],
            )
        raise ProtocolError("unknown_operation")
