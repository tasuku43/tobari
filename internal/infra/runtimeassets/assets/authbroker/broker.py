"""Static V1 broker state, handle isolation, and strict dispatch."""

from __future__ import annotations

import base64
import hashlib
import re
import secrets
import threading
from dataclasses import dataclass
from typing import Any

from . import SCHEMA_VERSION
from .protocol import MAX_SECRET_BYTES, ProtocolError, require_exact_keys
from .vault import (
    STATIC_CREDENTIAL_KIND,
    VaultError,
    VaultStore,
    decode_secret,
    empty_payload,
    new_record,
    validate_context_id,
    validate_provider_id,
)

PROJECT_ID_PATTERN = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)
HANDLE_PATTERN = re.compile(r"^tobari-h1_[A-Za-z0-9_-]{43}$")
HOST_PATTERN = re.compile(
    r"^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$"
)
HEADER_PATTERN = re.compile(r"^[!#$%&'*+.^_`|~0-9a-z-]{1,64}$")
SOURCE_FORMATS = ("raw", "bearer", "token")
DESTINATION_FORMATS = ("preserve_scheme", "raw", "bearer", "token")


class BrokerError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _translate_error(error: Exception) -> BrokerError:
    if isinstance(error, BrokerError):
        return error
    if isinstance(error, (ProtocolError, VaultError)):
        return BrokerError(error.code)
    return BrokerError("internal_error")


def _validate_project_id(value: Any) -> str:
    if not isinstance(value, str) or PROJECT_ID_PATTERN.fullmatch(value) is None:
        raise BrokerError("invalid_project")
    return value


def _validate_revision(value: Any) -> str:
    if (
        not isinstance(value, str)
        or not value.startswith("revision_")
        or len(value) > 128
        or any(ord(character) < 0x21 or ord(character) > 0x7E for character in value)
    ):
        raise BrokerError("invalid_revision")
    return value


def _validate_handle(value: Any) -> str:
    if not isinstance(value, str) or HANDLE_PATTERN.fullmatch(value) is None:
        raise BrokerError("invalid_handle")
    try:
        raw = base64.b64decode(
            value.removeprefix("tobari-h1_").encode("ascii") + b"=",
            altchars=b"-_",
            validate=True,
        )
    except ValueError:
        raise BrokerError("invalid_handle") from None
    if len(raw) != 32:
        raise BrokerError("invalid_handle")
    return value


def _validate_header(value: Any) -> str:
    if (
        not isinstance(value, str)
        or value != value.lower()
        or HEADER_PATTERN.fullmatch(value) is None
        or value in {"host", "content-length", "proxy-authorization", "cookie", "set-cookie"}
        or value.startswith("x-tobari-")
    ):
        raise BrokerError("invalid_binding")
    return value


@dataclass(frozen=True)
class Target:
    scheme: str
    host: str
    port: int

    @classmethod
    def parse(cls, value: Any) -> "Target":
        if not isinstance(value, dict) or set(value) != {"scheme", "host", "port"}:
            raise BrokerError("invalid_binding")
        host = value.get("host")
        port = value.get("port")
        if (
            value.get("scheme") != "https"
            or not isinstance(host, str)
            or host != host.lower()
            or HOST_PATTERN.fullmatch(host) is None
            or "." not in host
            or isinstance(port, bool)
            or not isinstance(port, int)
            or port < 1
            or port > 65535
        ):
            raise BrokerError("invalid_binding")
        return cls("https", host, port)

    def document(self) -> dict[str, Any]:
        return {"scheme": self.scheme, "host": self.host, "port": self.port}


@dataclass(frozen=True)
class Binding:
    provider_id: str
    target: Target
    source_header: str
    source_format: str
    destination_header: str
    destination_format: str
    secret_headers: tuple[str, ...]

    @classmethod
    def parse(cls, value: Any) -> "Binding":
        if not isinstance(value, dict) or set(value) != {
            "provider_id", "target", "source", "destination", "secret_headers"
        }:
            raise BrokerError("invalid_binding")
        try:
            provider = validate_provider_id(value.get("provider_id"))
        except VaultError:
            raise BrokerError("invalid_binding") from None
        source = value.get("source")
        destination = value.get("destination")
        headers = value.get("secret_headers")
        if (
            not isinstance(source, dict)
            or set(source) != {"header", "format"}
            or not isinstance(destination, dict)
            or set(destination) != {"header", "format", "secret_field"}
            or destination.get("secret_field") != STATIC_CREDENTIAL_KIND
            or not isinstance(headers, list)
            or not headers
            or len(headers) > 32
            or len(headers) != len(set(headers))
        ):
            raise BrokerError("invalid_binding")
        source_header = _validate_header(source.get("header"))
        destination_header = _validate_header(destination.get("header"))
        source_format = source.get("format")
        destination_format = destination.get("format")
        if source_format not in SOURCE_FORMATS or destination_format not in DESTINATION_FORMATS:
            raise BrokerError("invalid_binding")
        normalized_headers = tuple(sorted(_validate_header(item) for item in headers))
        if source_header not in normalized_headers or destination_header not in normalized_headers:
            raise BrokerError("invalid_binding")
        return cls(
            provider,
            Target.parse(value.get("target")),
            source_header,
            source_format,
            destination_header,
            destination_format,
            normalized_headers,
        )

    def document(self) -> dict[str, Any]:
        return {
            "provider_id": self.provider_id,
            "target": self.target.document(),
            "source": {"header": self.source_header, "format": self.source_format},
            "destination": {
                "header": self.destination_header,
                "format": self.destination_format,
                "secret_field": STATIC_CREDENTIAL_KIND,
            },
            "secret_headers": list(self.secret_headers),
        }

    def matches(self, target: Target, header: str, source_format: str) -> bool:
        return self.target == target and self.source_header == header and self.source_format == source_format


def _parse_bindings(value: Any, provider: str | None = None) -> tuple[Binding, ...]:
    if not isinstance(value, list) or not value or len(value) > 64:
        raise BrokerError("invalid_binding")
    parsed = tuple(Binding.parse(item) for item in value)
    if provider is not None and any(item.provider_id != provider for item in parsed):
        raise BrokerError("invalid_binding")
    canonical = [repr(item.document()) for item in parsed]
    if len(canonical) != len(set(canonical)):
        raise BrokerError("invalid_binding")
    grouped: dict[tuple[Target, str], set[str]] = {}
    for item in parsed:
        grouped.setdefault((item.target, item.source_header), set()).add(item.source_format)
    if any("raw" in formats and len(formats) > 1 for formats in grouped.values()):
        raise BrokerError("invalid_binding")
    return tuple(sorted(parsed, key=lambda item: repr(item.document())))


@dataclass(frozen=True)
class HandleRecord:
    context_id: str
    project_id: str
    provider: str
    record_id: str
    revision: str
    bindings: tuple[Binding, ...]


class BrokerState:
    def __init__(self, vaults: VaultStore):
        self._vaults = vaults
        self._key: bytearray | None = None
        self._handles: dict[bytes, HandleRecord] = {}
        self._mutex = threading.RLock()

    @property
    def locked(self) -> bool:
        with self._mutex:
            return self._key is None

    def unlock(self, key: bytes) -> dict[str, Any]:
        if not isinstance(key, bytes) or len(key) != 32:
            raise BrokerError("invalid_key")
        with self._mutex:
            if self._key is not None:
                self._key[:] = b"\x00" * len(self._key)
            self._key = bytearray(key)
            self._handles.clear()
        return {"schema_version": SCHEMA_VERSION, "ok": True, "state": "unlocked"}

    def _require_key(self) -> bytearray:
        if self._key is None:
            raise BrokerError("locked")
        return self._key

    def _load_or_empty(self, context_id: str) -> dict[str, Any]:
        try:
            return self._vaults.load(context_id, self._require_key())
        except VaultError as error:
            if error.code == "vault_not_found":
                return empty_payload()
            raise

    def _revoke(self, context_id: str, provider: str) -> None:
        for digest in [
            digest for digest, record in self._handles.items()
            if record.context_id == context_id and record.provider == provider
        ]:
            del self._handles[digest]

    def import_secret(
        self, context_id: Any, provider: Any, secret: bytes, account_label: str | None = None
    ) -> dict[str, Any]:
        try:
            context_id = validate_context_id(context_id)
            provider = validate_provider_id(provider)
            if not isinstance(secret, bytes) or not secret or len(secret) > MAX_SECRET_BYTES:
                raise BrokerError("invalid_secret")
            with self._mutex:
                payload = self._load_or_empty(context_id)
                record = new_record(secret, account_label)
                updated = {"schema_version": SCHEMA_VERSION, "providers": dict(payload["providers"])}
                updated["providers"][provider] = record
                self._vaults.save(context_id, self._require_key(), updated)
                self._revoke(context_id, provider)
            response = {
                "schema_version": SCHEMA_VERSION,
                "ok": True,
                "provider": provider,
                "revision": record["revision"],
            }
            if account_label is not None:
                response["account_label"] = account_label
            return response
        except Exception as error:
            raise _translate_error(error) from None

    def logout(self, context_id: Any, provider: Any) -> dict[str, Any]:
        try:
            context_id = validate_context_id(context_id)
            provider = validate_provider_id(provider)
            with self._mutex:
                payload = self._load_or_empty(context_id)
                changed = provider in payload["providers"]
                if changed:
                    updated = {"schema_version": SCHEMA_VERSION, "providers": dict(payload["providers"])}
                    del updated["providers"][provider]
                    self._vaults.save(context_id, self._require_key(), updated)
                self._revoke(context_id, provider)
            return {
                "schema_version": SCHEMA_VERSION,
                "ok": True,
                "provider": provider,
                "state": "logged_out",
                "changed": changed,
            }
        except Exception as error:
            raise _translate_error(error) from None

    def status(self, context_id: Any, provider: Any) -> dict[str, Any]:
        try:
            context_id = validate_context_id(context_id)
            provider = validate_provider_id(provider)
            with self._mutex:
                if self._key is None:
                    return {"schema_version": SCHEMA_VERSION, "ok": True, "state": "locked", "provider": provider}
                record = self._load_or_empty(context_id)["providers"].get(provider)
                if record is None:
                    return {"schema_version": SCHEMA_VERSION, "ok": True, "state": "not_configured", "provider": provider}
                response = {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "state": "configured",
                    "provider": provider,
                    "revision": record["revision"],
                }
                if record["account_label"] is not None:
                    response["account_label"] = record["account_label"]
                return response
        except Exception as error:
            raise _translate_error(error) from None

    def issue_handle(self, context_id: Any, project_id: Any, provider: Any, bindings: Any) -> dict[str, Any]:
        try:
            context_id = validate_context_id(context_id)
            project_id = _validate_project_id(project_id)
            provider = validate_provider_id(provider)
            parsed = _parse_bindings(bindings, provider)
            with self._mutex:
                payload = self._load_or_empty(context_id)
                credential = payload["providers"].get(provider)
                if credential is None:
                    raise BrokerError("credential_not_found")
                if credential.get("credential_kind") != STATIC_CREDENTIAL_KIND:
                    raise BrokerError("credential_binding_mismatch")
                persisted = credential["handles"].get(project_id)
                if persisted is not None and _parse_bindings(persisted["bindings"], provider) == parsed:
                    handle = _validate_handle(persisted["handle"])
                else:
                    handle = "tobari-h1_" + base64.urlsafe_b64encode(secrets.token_bytes(32)).rstrip(b"=").decode("ascii")
                    changed = dict(credential)
                    changed["handles"] = dict(credential["handles"])
                    changed["handles"][project_id] = {"handle": handle, "bindings": [item.document() for item in parsed]}
                    updated = {"schema_version": SCHEMA_VERSION, "providers": dict(payload["providers"])}
                    updated["providers"][provider] = changed
                    self._vaults.save(context_id, self._require_key(), updated)
                record = HandleRecord(context_id, project_id, provider, credential["record_id"], credential["revision"], parsed)
                self._handles[hashlib.sha256(handle.encode("ascii")).digest()] = record
            return {"schema_version": SCHEMA_VERSION, "ok": True, "provider": provider, "revision": credential["revision"], "handle": handle}
        except Exception as error:
            raise _translate_error(error) from None

    def binding_status(self, context_id: Any, project_id: Any, provider: Any, revision: Any, bindings: Any) -> dict[str, Any]:
        try:
            context_id = validate_context_id(context_id)
            project_id = _validate_project_id(project_id)
            provider = validate_provider_id(provider)
            revision = _validate_revision(revision)
            parsed = _parse_bindings(bindings, provider)
            with self._mutex:
                credential = self._load_or_empty(context_id)["providers"].get(provider)
                state = "stale"
                if credential is not None and credential["revision"] == revision:
                    persisted = credential["handles"].get(project_id)
                    if persisted is None:
                        state = "missing"
                    elif _parse_bindings(persisted["bindings"], provider) == parsed:
                        _validate_handle(persisted["handle"])
                        state = "ready"
            return {"schema_version": SCHEMA_VERSION, "ok": True, "state": state, "provider": provider, "revision": revision}
        except Exception as error:
            raise _translate_error(error) from None

    def _handle_record(self, handle: Any, context_id: Any, project_id: Any, provider: Any) -> HandleRecord:
        handle = _validate_handle(handle)
        context_id = validate_context_id(context_id)
        project_id = _validate_project_id(project_id)
        provider = validate_provider_id(provider)
        digest = hashlib.sha256(handle.encode("ascii")).digest()
        record = self._handles.get(digest)
        if record is None:
            credential = self._load_or_empty(context_id)["providers"].get(provider)
            if credential is None:
                raise BrokerError("handle_not_found")
            persisted = credential["handles"].get(project_id)
            if persisted is None or not secrets.compare_digest(handle, _validate_handle(persisted["handle"])):
                raise BrokerError("handle_not_found")
            record = HandleRecord(context_id, project_id, provider, credential["record_id"], credential["revision"], _parse_bindings(persisted["bindings"], provider))
            self._handles[digest] = record
        if (record.context_id, record.project_id, record.provider) != (context_id, project_id, provider):
            raise BrokerError("handle_binding_mismatch")
        return record

    @staticmethod
    def _selected_binding(record: HandleRecord, target: Any, header: Any, source_format: Any) -> tuple[Binding, Target]:
        normalized_target = Target.parse(target)
        header = _validate_header(header)
        if source_format not in SOURCE_FORMATS:
            raise BrokerError("invalid_binding")
        matches = [item for item in record.bindings if item.matches(normalized_target, header, source_format)]
        if len(matches) != 1:
            raise BrokerError("handle_binding_mismatch")
        return matches[0], normalized_target

    def introspect(self, handle: Any, context_id: Any, project_id: Any, provider: Any, target: Any, source_header: Any, source_format: Any) -> dict[str, Any]:
        try:
            provider = validate_provider_id(provider)
            with self._mutex:
                self._require_key()
                record = self._handle_record(handle, context_id, project_id, provider)
                binding, normalized_target = self._selected_binding(record, target, source_header, source_format)
            return {
                "schema_version": SCHEMA_VERSION, "ok": True, "provider": provider,
                "revision": record.revision, "target": normalized_target.document(),
                "source": {"header": binding.source_header, "format": binding.source_format},
                "destination": binding.document()["destination"], "secret_headers": list(binding.secret_headers),
            }
        except Exception as error:
            raise _translate_error(error) from None

    def resolve(self, handle: Any, context_id: Any, project_id: Any, provider: Any, revision: Any, target: Any, source_header: Any, source_format: Any) -> dict[str, Any]:
        try:
            provider = validate_provider_id(provider)
            revision = _validate_revision(revision)
            with self._mutex:
                record = self._handle_record(handle, context_id, project_id, provider)
                if record.revision != revision:
                    raise BrokerError("handle_binding_mismatch")
                binding, normalized_target = self._selected_binding(record, target, source_header, source_format)
                credential = self._vaults.load(record.context_id, self._require_key())["providers"].get(provider)
                if credential is None or credential["record_id"] != record.record_id or credential["revision"] != record.revision:
                    self._revoke(record.context_id, provider)
                    raise BrokerError("handle_revoked")
                if credential.get("credential_kind") != STATIC_CREDENTIAL_KIND:
                    raise BrokerError("credential_not_resolvable")
                secret = decode_secret(credential["secret"])
            return {
                "schema_version": SCHEMA_VERSION, "ok": True, "provider": provider, "revision": revision,
                "target": normalized_target.document(),
                "source": {"header": binding.source_header, "format": binding.source_format},
                "destination": binding.document()["destination"], "secret_headers": list(binding.secret_headers),
                "secret": {"field": STATIC_CREDENTIAL_KIND, "encoding": "base64url", "value": base64.urlsafe_b64encode(secret).rstrip(b"=").decode("ascii")},
            }
        except Exception as error:
            raise _translate_error(error) from None


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
            return self._runtime(operation, request, raw_payload) if self._interface == "runtime" else self._control(operation, request, raw_payload)
        except Exception as error:
            raise _translate_error(error) from None

    def expected_raw_length(self, request: dict[str, Any]) -> int:
        operation = request.get("op")
        if self._interface != "control" or operation not in {"unlock", "import", "login"}:
            return 0
        key = "key_length" if operation == "unlock" else "secret_length"
        value = request.get(key)
        maximum = 32 if operation == "unlock" else MAX_SECRET_BYTES
        if isinstance(value, bool) or not isinstance(value, int) or value < 1 or value > maximum or (operation == "unlock" and value != 32):
            raise BrokerError("invalid_length")
        return value

    def _runtime(self, operation: str, request: dict[str, Any], raw: bytes) -> dict[str, Any]:
        if raw:
            raise ProtocolError("unexpected_payload")
        if operation == "health":
            require_exact_keys(request, {"schema_version", "op"})
            return {"schema_version": SCHEMA_VERSION, "ok": True, "state": "locked" if self._state.locked else "unlocked"}
        fields = {"schema_version", "op", "handle", "context_id", "project_id", "provider", "target", "source_header", "source_format"}
        if operation == "introspect":
            require_exact_keys(request, fields)
            return self._state.introspect(request["handle"], request["context_id"], request["project_id"], request["provider"], request["target"], request["source_header"], request["source_format"])
        if operation == "resolve":
            require_exact_keys(request, fields | {"revision"})
            return self._state.resolve(request["handle"], request["context_id"], request["project_id"], request["provider"], request["revision"], request["target"], request["source_header"], request["source_format"])
        raise ProtocolError("unknown_operation")

    def _control(self, operation: str, request: dict[str, Any], raw: bytes) -> dict[str, Any]:
        if operation == "health":
            require_exact_keys(request, {"schema_version", "op"})
            if raw:
                raise ProtocolError("unexpected_payload")
            return {"schema_version": SCHEMA_VERSION, "ok": True, "state": "locked" if self._state.locked else "unlocked"}
        if operation == "unlock":
            require_exact_keys(request, {"schema_version", "op", "key_length"})
            if len(raw) != request["key_length"]:
                raise ProtocolError("invalid_length")
            return self._state.unlock(raw)
        if operation in {"import", "login"}:
            fields = {"schema_version", "op", "context_id", "provider", "secret_length"}
            if operation == "login":
                fields.add("account_label")
                if request.get("provider") != "github":
                    raise ProtocolError("invalid_provider")
            require_exact_keys(request, fields)
            if len(raw) != request["secret_length"]:
                raise ProtocolError("invalid_length")
            return self._state.import_secret(request["context_id"], request["provider"], raw, request.get("account_label"))
        if operation in {"status", "logout"}:
            require_exact_keys(request, {"schema_version", "op", "context_id", "provider"})
            if raw:
                raise ProtocolError("unexpected_payload")
            return getattr(self._state, operation)(request["context_id"], request["provider"])
        if operation == "issue_handle":
            require_exact_keys(request, {"schema_version", "op", "context_id", "project_id", "provider", "bindings"})
            if raw:
                raise ProtocolError("unexpected_payload")
            return self._state.issue_handle(request["context_id"], request["project_id"], request["provider"], request["bindings"])
        if operation == "binding_status":
            require_exact_keys(request, {"schema_version", "op", "context_id", "project_id", "provider", "revision", "bindings"})
            if raw:
                raise ProtocolError("unexpected_payload")
            return self._state.binding_status(request["context_id"], request["project_id"], request["provider"], request["revision"], request["bindings"])
        raise ProtocolError("unknown_operation")
