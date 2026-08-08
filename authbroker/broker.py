"""Locked broker state, handle isolation, and strict protocol dispatch."""

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


def _validate_project_id(project_id: Any) -> str:
    if not isinstance(project_id, str) or not PROJECT_ID_PATTERN.fullmatch(project_id):
        raise BrokerError("invalid_project")
    return project_id


def _validate_revision(revision: Any) -> str:
    if (
        not isinstance(revision, str)
        or not revision.startswith("revision_")
        or len(revision) > 128
        or any(ord(character) < 0x21 or ord(character) > 0x7E for character in revision)
    ):
        raise BrokerError("invalid_revision")
    return revision


def _validate_handle(handle: Any) -> str:
    if not isinstance(handle, str) or not HANDLE_PATTERN.fullmatch(handle):
        raise BrokerError("invalid_handle")
    encoded = handle.removeprefix("tobari-h1_").encode("ascii")
    try:
        raw = base64.b64decode(encoded + b"=", altchars=b"-_", validate=True)
    except ValueError:
        raise BrokerError("invalid_handle") from None
    if len(raw) != 32:
        raise BrokerError("invalid_handle")
    return handle


def _validate_ascii(value: Any, maximum: int, code: str) -> str:
    if (
        not isinstance(value, str)
        or len(value) > maximum
        or any(ord(character) < 0x20 or ord(character) > 0x7E for character in value)
    ):
        raise BrokerError(code)
    return value


def _validate_header(value: Any) -> str:
    if (
        not isinstance(value, str)
        or value != value.lower()
        or not HEADER_PATTERN.fullmatch(value)
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
        scheme = value.get("scheme")
        host = value.get("host")
        port = value.get("port")
        if scheme != "https":
            raise BrokerError("invalid_binding")
        if (
            not isinstance(host, str)
            or host != host.lower()
            or not HOST_PATTERN.fullmatch(host)
            or "." not in host
        ):
            raise BrokerError("invalid_binding")
        if isinstance(port, bool) or not isinstance(port, int) or port < 1 or port > 65535:
            raise BrokerError("invalid_binding")
        return cls(scheme=scheme, host=host, port=port)

    def document(self) -> dict[str, Any]:
        return {"scheme": self.scheme, "host": self.host, "port": self.port}


@dataclass(frozen=True)
class Binding:
    """Provider credential projection; HTTP L7 authorization stays in OPA."""

    provider_id: str
    target: Target
    source_header: str
    source_format: str
    destination_header: str
    destination_format: str
    secret_field: str
    secret_headers: tuple[str, ...]

    @classmethod
    def parse(cls, value: Any) -> "Binding":
        if not isinstance(value, dict) or set(value) != {
            "provider_id",
            "target",
            "source",
            "destination",
            "secret_headers",
        }:
            raise BrokerError("invalid_binding")
        provider_id = value.get("provider_id")
        target = value.get("target")
        source = value.get("source")
        destination = value.get("destination")
        secret_headers = value.get("secret_headers")
        if not isinstance(target, dict) or set(target) != {"scheme", "host", "port"}:
            raise BrokerError("invalid_binding")
        if not isinstance(source, dict) or set(source) != {"header", "format"}:
            raise BrokerError("invalid_binding")
        if not isinstance(destination, dict) or set(destination) != {
            "header",
            "format",
            "secret_field",
        }:
            raise BrokerError("invalid_binding")
        try:
            provider_id = validate_provider_id(provider_id)
        except VaultError:
            raise BrokerError("invalid_binding") from None
        normalized_target = Target.parse(target)
        source_header = source.get("header")
        source_format = source.get("format")
        destination_header = destination.get("header")
        destination_format = destination.get("format")
        secret_field = destination.get("secret_field")
        source_header = _validate_header(source_header)
        if source_format not in SOURCE_FORMATS:
            raise BrokerError("invalid_binding")
        destination_header = _validate_header(destination_header)
        if destination_format not in DESTINATION_FORMATS or secret_field != "primary_secret":
            raise BrokerError("invalid_binding")
        if (
            not isinstance(secret_headers, list)
            or not secret_headers
            or len(secret_headers) > 32
            or any(
                not isinstance(header, str)
                or header != header.lower()
                or not HEADER_PATTERN.fullmatch(header)
                for header in secret_headers
            )
            or len(set(secret_headers)) != len(secret_headers)
        ):
            raise BrokerError("invalid_binding")
        if (
            source_header not in secret_headers
            or destination_header not in secret_headers
        ):
            raise BrokerError("invalid_binding")
        for header in secret_headers:
            _validate_header(header)
        return cls(
            provider_id,
            normalized_target,
            source_header,
            source_format,
            destination_header,
            destination_format,
            secret_field,
            tuple(sorted(secret_headers)),
        )

    def document(self) -> dict[str, Any]:
        return {
            "provider_id": self.provider_id,
            "target": self.target.document(),
            "source": {
                "header": self.source_header,
                "format": self.source_format,
            },
            "destination": {
                "header": self.destination_header,
                "format": self.destination_format,
                "secret_field": self.secret_field,
            },
            "secret_headers": list(self.secret_headers),
        }

    def matches(self, target: Target, source_header: str, source_format: str) -> bool:
        return (
            self.target == target
            and self.source_header == source_header
            and self.source_format == source_format
        )


def _parse_bindings(value: Any, provider: str | None = None) -> tuple[Binding, ...]:
    if not isinstance(value, list) or not value or len(value) > 64:
        raise BrokerError("invalid_binding")
    parsed = tuple(Binding.parse(item) for item in value)
    if provider is not None and any(binding.provider_id != provider for binding in parsed):
        raise BrokerError("invalid_binding")
    canonical = [binding.document() for binding in parsed]
    keys = [repr(item) for item in canonical]
    if len(set(keys)) != len(keys):
        raise BrokerError("invalid_binding")
    grouped_formats: dict[tuple[Target, str], set[str]] = {}
    for binding in parsed:
        grouped_formats.setdefault((binding.target, binding.source_header), set()).add(
            binding.source_format
        )
    if any("raw" in formats and len(formats) > 1 for formats in grouped_formats.values()):
        raise BrokerError("invalid_binding")
    return tuple(sorted(parsed, key=lambda binding: repr(binding.document())))


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
                for index in range(len(self._key)):
                    self._key[index] = 0
            self._key = bytearray(key)
            self._handles.clear()
        return {"schema_version": SCHEMA_VERSION, "ok": True, "state": "unlocked"}

    def _require_key(self) -> bytearray:
        if self._key is None:
            raise BrokerError("locked")
        return self._key

    def _load_or_empty(self, context_id: str) -> dict[str, Any]:
        key = self._require_key()
        try:
            return self._vaults.load(context_id, key)
        except VaultError as error:
            if error.code == "vault_not_found":
                return empty_payload()
            raise

    def _revoke(self, context_id: str, provider: str) -> None:
        doomed = [
            digest
            for digest, record in self._handles.items()
            if record.context_id == context_id and record.provider == provider
        ]
        for digest in doomed:
            del self._handles[digest]

    def _revoke_project(self, context_id: str, project_id: str, provider: str) -> None:
        doomed = [
            digest
            for digest, record in self._handles.items()
            if record.context_id == context_id
            and record.project_id == project_id
            and record.provider == provider
        ]
        for digest in doomed:
            del self._handles[digest]

    def _index_handle(
        self,
        handle: str,
        context_id: str,
        project_id: str,
        provider: str,
        credential: dict[str, Any],
        bindings: tuple[Binding, ...],
    ) -> HandleRecord:
        record = HandleRecord(
            context_id=context_id,
            project_id=project_id,
            provider=provider,
            record_id=credential["record_id"],
            revision=credential["revision"],
            bindings=bindings,
        )
        digest = hashlib.sha256(handle.encode("ascii")).digest()
        self._handles[digest] = record
        return record

    def import_secret(
        self,
        context_id: Any,
        provider: Any,
        secret: bytes,
        account_label: str | None = None,
    ) -> dict[str, Any]:
        try:
            context_id = validate_context_id(context_id)
            provider = validate_provider_id(provider)
            if not isinstance(secret, bytes) or not secret or len(secret) > MAX_SECRET_BYTES:
                raise BrokerError("invalid_secret")
            with self._mutex:
                payload = self._load_or_empty(context_id)
                record = new_record(secret, account_label=account_label)
                updated = {
                    "schema_version": SCHEMA_VERSION,
                    "providers": dict(payload["providers"]),
                }
                updated["providers"][provider] = record
                self._vaults.save(context_id, self._require_key(), updated)
                self._revoke(context_id, provider)
                response = {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": provider,
                    "revision": record["revision"],
                }
                if record["account_label"] is not None:
                    response["account_label"] = record["account_label"]
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
                    updated = {
                        "schema_version": SCHEMA_VERSION,
                        "providers": dict(payload["providers"]),
                    }
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
                    return {
                        "schema_version": SCHEMA_VERSION,
                        "ok": True,
                        "state": "locked",
                        "provider": provider,
                    }
                payload = self._load_or_empty(context_id)
                record = payload["providers"].get(provider)
                if record is None:
                    return {
                        "schema_version": SCHEMA_VERSION,
                        "ok": True,
                        "state": "not_configured",
                        "provider": provider,
                    }
                response = {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "state": "ready",
                    "provider": provider,
                    "revision": record["revision"],
                }
                if record["account_label"] is not None:
                    response["account_label"] = record["account_label"]
                return response
        except Exception as error:
            raise _translate_error(error) from None

    def issue_handle(
        self,
        context_id: Any,
        project_id: Any,
        provider: Any,
        bindings: Any,
    ) -> dict[str, Any]:
        try:
            context_id = validate_context_id(context_id)
            project_id = _validate_project_id(project_id)
            provider = validate_provider_id(provider)
            normalized_bindings = _parse_bindings(bindings, provider)
            with self._mutex:
                payload = self._load_or_empty(context_id)
                credential = payload["providers"].get(provider)
                if credential is None:
                    raise BrokerError("credential_not_found")
                existing = credential["handles"].get(project_id)
                if existing is not None:
                    existing_bindings = _parse_bindings(existing["bindings"], provider)
                    if existing_bindings == normalized_bindings:
                        handle = _validate_handle(existing["handle"])
                        self._index_handle(
                            handle,
                            context_id,
                            project_id,
                            provider,
                            credential,
                            normalized_bindings,
                        )
                        return {
                            "schema_version": SCHEMA_VERSION,
                            "ok": True,
                            "handle": handle,
                            "provider": provider,
                            "revision": credential["revision"],
                        }
                raw = secrets.token_bytes(32)
                handle = "tobari-h1_" + base64.urlsafe_b64encode(raw).rstrip(b"=").decode("ascii")
                updated_credential = dict(credential)
                updated_handles = dict(credential["handles"])
                updated_handles[project_id] = {
                    "handle": handle,
                    "bindings": [binding.document() for binding in normalized_bindings],
                }
                updated_credential["handles"] = updated_handles
                updated = {
                    "schema_version": SCHEMA_VERSION,
                    "providers": dict(payload["providers"]),
                }
                updated["providers"][provider] = updated_credential
                self._vaults.save(context_id, self._require_key(), updated)
                self._revoke_project(context_id, project_id, provider)
                self._index_handle(
                    handle,
                    context_id,
                    project_id,
                    provider,
                    updated_credential,
                    normalized_bindings,
                )
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "handle": handle,
                    "provider": provider,
                    "revision": credential["revision"],
                }
        except Exception as error:
            raise _translate_error(error) from None

    def binding_status(
        self,
        context_id: Any,
        project_id: Any,
        provider: Any,
        revision: Any,
        bindings: Any,
    ) -> dict[str, Any]:
        """Report whether one host-owned project binding still exists.

        This control-only diagnostic deliberately accepts no raw handle and
        returns no handle or credential material.  The caller must supply the
        same normalized task dimensions used when the handle was issued.
        """
        try:
            context_id = validate_context_id(context_id)
            project_id = _validate_project_id(project_id)
            provider = validate_provider_id(provider)
            revision = _validate_revision(revision)
            normalized_bindings = _parse_bindings(bindings, provider)
            with self._mutex:
                payload = self._load_or_empty(context_id)
                credential = payload["providers"].get(provider)
                state = "stale"
                if credential is not None and credential["revision"] == revision:
                    persisted = credential["handles"].get(project_id)
                    if persisted is None:
                        state = "missing"
                    else:
                        _validate_handle(persisted["handle"])
                        persisted_bindings = _parse_bindings(
                            persisted["bindings"], provider
                        )
                        state = (
                            "ready"
                            if persisted_bindings == normalized_bindings
                            else "stale"
                        )
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "state": state,
                    "provider": provider,
                    "revision": revision,
                }
        except Exception as error:
            raise _translate_error(error) from None

    def _handle_record(
        self, handle: Any, context_id: Any, project_id: Any, provider: Any
    ) -> HandleRecord:
        handle = _validate_handle(handle)
        context_id = validate_context_id(context_id)
        project_id = _validate_project_id(project_id)
        provider = validate_provider_id(provider)
        digest = hashlib.sha256(handle.encode("ascii")).digest()
        record = self._handles.get(digest)
        if record is None:
            payload = self._load_or_empty(context_id)
            credential = payload["providers"].get(provider)
            if credential is None:
                raise BrokerError("handle_not_found")
            persisted = credential["handles"].get(project_id)
            if persisted is None:
                raise BrokerError("handle_not_found")
            persisted_handle = _validate_handle(persisted["handle"])
            persisted_digest = hashlib.sha256(persisted_handle.encode("ascii")).digest()
            if not secrets.compare_digest(digest, persisted_digest):
                raise BrokerError("handle_not_found")
            record = self._index_handle(
                persisted_handle,
                context_id,
                project_id,
                provider,
                credential,
                _parse_bindings(persisted["bindings"], provider),
            )
        if (
            record.context_id != context_id
            or record.project_id != project_id
            or record.provider != provider
        ):
            raise BrokerError("handle_binding_mismatch")
        return record

    @staticmethod
    def _selected_binding(
        record: HandleRecord,
        target_value: Any,
        source_header_value: Any,
        source_format_value: Any,
    ) -> tuple[Binding, Target, str, str]:
        target = Target.parse(target_value)
        if (
            not isinstance(source_header_value, str)
            or source_header_value != source_header_value.lower()
            or not HEADER_PATTERN.fullmatch(source_header_value)
            or source_header_value
            in {"host", "content-length", "proxy-authorization", "cookie", "set-cookie"}
            or source_header_value.startswith("x-tobari-")
        ):
            raise BrokerError("invalid_binding")
        if not isinstance(source_format_value, str) or source_format_value not in SOURCE_FORMATS:
            raise BrokerError("invalid_binding")
        matches = [
            binding
            for binding in record.bindings
            if binding.matches(target, source_header_value, source_format_value)
        ]
        if len(matches) != 1:
            raise BrokerError("handle_binding_mismatch")
        return matches[0], target, source_header_value, source_format_value

    def introspect(
        self,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: Any,
        target: Any,
        source_header: Any,
        source_format: Any,
    ) -> dict[str, Any]:
        try:
            provider = validate_provider_id(provider)
            with self._mutex:
                self._require_key()
                record = self._handle_record(handle, context_id, project_id, provider)
                binding, normalized_target, normalized_header, normalized_format = (
                    self._selected_binding(record, target, source_header, source_format)
                )
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": record.provider,
                    "revision": record.revision,
                    "target": normalized_target.document(),
                    "source": {
                        "header": normalized_header,
                        "format": normalized_format,
                    },
                    "destination": binding.document()["destination"],
                    "secret_headers": list(binding.secret_headers),
                }
        except Exception as error:
            raise _translate_error(error) from None

    def resolve(
        self,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: Any,
        revision: Any,
        target: Any,
        source_header: Any,
        source_format: Any,
    ) -> dict[str, Any]:
        try:
            provider = validate_provider_id(provider)
            revision = _validate_revision(revision)
            with self._mutex:
                key = self._require_key()
                record = self._handle_record(handle, context_id, project_id, provider)
                if record.provider != provider or record.revision != revision:
                    raise BrokerError("handle_binding_mismatch")
                selected_binding, normalized_target, normalized_header, normalized_format = (
                    self._selected_binding(record, target, source_header, source_format)
                )
                payload = self._vaults.load(record.context_id, key)
                credential = payload["providers"].get(provider)
                if (
                    credential is None
                    or credential["record_id"] != record.record_id
                    or credential["revision"] != record.revision
                ):
                    self._revoke(record.context_id, provider)
                    raise BrokerError("handle_revoked")
                secret = decode_secret(credential["secret"])
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": provider,
                    "revision": revision,
                    "target": normalized_target.document(),
                    "source": {
                        "header": normalized_header,
                        "format": normalized_format,
                    },
                    "destination": selected_binding.document()["destination"],
                    "secret_headers": list(selected_binding.secret_headers),
                    "secret": {
                        "field": selected_binding.secret_field,
                        "encoding": "base64url",
                        "value": base64.urlsafe_b64encode(secret).rstrip(b"=").decode("ascii"),
                    },
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
            if self._interface == "runtime":
                return self._runtime(operation, request, raw_payload)
            return self._control(operation, request, raw_payload)
        except Exception as error:
            raise _translate_error(error) from None

    def expected_raw_length(self, request: dict[str, Any]) -> int:
        operation = request.get("op")
        if self._interface != "control" or operation not in {"unlock", "import", "login"}:
            return 0
        key = "key_length" if operation == "unlock" else "secret_length"
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
            require_exact_keys(
                request,
                {
                    "schema_version",
                    "op",
                    "context_id",
                    "provider",
                    "secret_length",
                    "account_label",
                },
            )
            if len(raw_payload) != request["secret_length"]:
                raise ProtocolError("invalid_length")
            if request["provider"] != "github":
                raise ProtocolError("invalid_provider")
            return self._state.import_secret(
                request["context_id"],
                request["provider"],
                raw_payload,
                account_label=request["account_label"],
            )
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
