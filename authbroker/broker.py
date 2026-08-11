"""Locked broker state, handle isolation, and strict protocol dispatch."""

from __future__ import annotations

import base64
import hashlib
import json
import re
import secrets
import threading
import time
from dataclasses import dataclass, field
from typing import Any, Callable

from . import SCHEMA_VERSION
from .aws_sigv4 import (
    SigV4Error,
    SigV4Request,
    parse_credentials,
    parse_request,
    sign,
)
from .datadog_oauth import (
    DatadogOAuthError,
    PupOAuthState,
    refresh as refresh_datadog_oauth,
)
from .openai_codex_oauth import (
    CodexOAuthState,
    OpenAICodexOAuthError,
    refresh as refresh_openai_codex_oauth,
)
from .companion_protocol import (
    CompanionChannelManager,
    CompanionError,
    RefreshRequest,
    RefreshResult,
    decode_refresh_secret,
    derive_epoch_key,
)
from .protocol import MAX_SECRET_BYTES, ProtocolError, require_exact_keys
from .vault import (
    AWS_SSO_CREDENTIAL_KIND,
    CLAUDE_ACCOUNT_LABEL,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    STATIC_CREDENTIAL_KIND,
    VaultError,
    VaultStore,
    decode_secret,
    empty_payload,
    encode_secret,
    new_aws_sso_record,
    new_datadog_oauth_record,
    new_openai_codex_oauth_record,
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
PROVEN_PRE_EXECUTION_REFRESH_ERRORS = frozenset(
    {
        "companion_unavailable",
        "companion_busy",
        "companion_task_invalid",
        "companion_cancelled",
        "companion_timeout",
    }
)
DEFAULT_RECORD_LOCK_TIMEOUT_SECONDS = 1.0


class BrokerError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _translate_error(error: Exception) -> BrokerError:
    if isinstance(error, BrokerError):
        return error
    if isinstance(error, (
        ProtocolError, VaultError, SigV4Error, CompanionError,
        DatadogOAuthError, OpenAICodexOAuthError,
    )):
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
        if destination_format not in DESTINATION_FORMATS or secret_field not in {
            "primary_secret", DATADOG_OAUTH_CREDENTIAL_KIND,
            OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
        }:
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
        result = cls(
            provider_id,
            normalized_target,
            source_header,
            source_format,
            destination_header,
            destination_format,
            secret_field,
            tuple(sorted(secret_headers)),
        )
        if secret_field == DATADOG_OAUTH_CREDENTIAL_KIND and result.document() != {
            "provider_id": "datadog",
            "target": {"scheme": "https", "host": "api.datadoghq.com", "port": 443},
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {"header": "authorization", "format": "bearer", "secret_field": DATADOG_OAUTH_CREDENTIAL_KIND},
            "secret_headers": ["authorization"],
        }:
            raise BrokerError("invalid_binding")
        if secret_field == OPENAI_CODEX_OAUTH_CREDENTIAL_KIND and result.document() != {
            "provider_id": "openai",
            "target": {"scheme": "https", "host": "chatgpt.com", "port": 443},
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {
                "header": "authorization", "format": "bearer",
                "secret_field": OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
            },
            "secret_headers": [
                "authorization", "chatgpt-account-id", "x-openai-fedramp"
            ],
        }:
            raise BrokerError("invalid_binding")
        return result

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


@dataclass(frozen=True)
class AwsSigV4Binding:
    """One reviewed built-in AWS signing plan; no executable manifest fields."""

    provider_id: str
    dns_suffixes: tuple[str, ...]
    authorization_header: str
    security_token_header: str
    secret_headers: tuple[str, ...]

    @classmethod
    def parse(cls, value: Any) -> "AwsSigV4Binding":
        if not isinstance(value, dict) or set(value) != {
            "provider_id",
            "kind",
            "aws_sigv4",
        }:
            raise BrokerError("invalid_binding")
        if value.get("provider_id") != "aws" or value.get("kind") != "aws_sigv4":
            raise BrokerError("invalid_binding")
        plan = value.get("aws_sigv4")
        if not isinstance(plan, dict) or set(plan) != {
            "target",
            "source",
            "secret_headers",
        }:
            raise BrokerError("invalid_binding")
        target = plan.get("target")
        source = plan.get("source")
        if target != {
            "scheme": "https",
            "port": 443,
            "dns_suffixes": ["amazonaws.com"],
        }:
            raise BrokerError("invalid_binding")
        if source != {
            "authorization_header": "authorization",
            "security_token_header": "x-amz-security-token",
        }:
            raise BrokerError("invalid_binding")
        if plan.get("secret_headers") != [
            "authorization",
            "x-amz-security-token",
        ]:
            raise BrokerError("invalid_binding")
        return cls(
            provider_id="aws",
            dns_suffixes=("amazonaws.com",),
            authorization_header="authorization",
            security_token_header="x-amz-security-token",
            secret_headers=("authorization", "x-amz-security-token"),
        )

    def document(self) -> dict[str, Any]:
        return {
            "provider_id": self.provider_id,
            "kind": "aws_sigv4",
            "aws_sigv4": {
                "target": {
                    "scheme": "https",
                    "port": 443,
                    "dns_suffixes": list(self.dns_suffixes),
                },
                "source": {
                    "authorization_header": self.authorization_header,
                    "security_token_header": self.security_token_header,
                },
                "secret_headers": list(self.secret_headers),
            },
        }

    def matches_target(self, target: Target) -> bool:
        return (
            target.scheme == "https"
            and target.port == 443
            and any(target.host.endswith("." + suffix) for suffix in self.dns_suffixes)
        )


NormalizedBinding = Binding | AwsSigV4Binding


def _parse_bindings(
    value: Any, provider: str | None = None
) -> tuple[NormalizedBinding, ...]:
    if not isinstance(value, list) or not value or len(value) > 64:
        raise BrokerError("invalid_binding")
    parsed = tuple(
        AwsSigV4Binding.parse(item)
        if isinstance(item, dict) and item.get("kind") == "aws_sigv4"
        else Binding.parse(item)
        for item in value
    )
    if provider is not None and any(binding.provider_id != provider for binding in parsed):
        raise BrokerError("invalid_binding")
    canonical = [binding.document() for binding in parsed]
    keys = [repr(item) for item in canonical]
    if len(set(keys)) != len(keys):
        raise BrokerError("invalid_binding")
    grouped_formats: dict[tuple[Target, str], set[str]] = {}
    for binding in parsed:
        if not isinstance(binding, Binding):
            continue
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
    bindings: tuple[NormalizedBinding, ...]


@dataclass(frozen=True)
class AwsRefreshSnapshot:
    context_id: str
    project_id: str
    provider: str
    record_id: str
    revision: str
    state_generation: int
    driver_id: str
    driver_revision: str
    binding_digest: str
    request_digest: str
    state_sha256: str
    state: bytes = field(repr=False)

    @property
    def lock_key(self) -> tuple[str, str, str, str]:
        return (self.context_id, self.provider, self.record_id, self.revision)


@dataclass(frozen=True)
class DatadogRefreshSnapshot:
    context_id: str
    project_id: str
    provider: str
    record_id: str
    revision: str
    state_generation: int
    driver_id: str
    driver_revision: str
    binding_digest: str
    state_sha256: str
    state: bytes = field(repr=False)

    @property
    def lock_key(self) -> tuple[str, str, str, str]:
        return (self.context_id, self.provider, self.record_id, self.revision)


@dataclass(frozen=True)
class OpenAIRefreshSnapshot:
    context_id: str
    project_id: str
    provider: str
    record_id: str
    revision: str
    state_generation: int
    driver_id: str
    driver_revision: str
    binding_digest: str
    state_sha256: str
    state: bytes = field(repr=False)

    @property
    def lock_key(self) -> tuple[str, str, str, str]:
        return (self.context_id, self.provider, self.record_id, self.revision)


def _document_digest(document: dict[str, Any]) -> str:
    try:
        encoded = json.dumps(
            document,
            ensure_ascii=True,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("ascii")
    except (TypeError, ValueError, UnicodeEncodeError):
        raise BrokerError("aws_signing_request_invalid") from None
    return hashlib.sha256(encoded).hexdigest()


def _signing_request_document(request: SigV4Request) -> dict[str, Any]:
    return {
        "host": request.host,
        "method": request.method,
        "path": request.path,
        "query": request.query,
        "region": request.region,
        "service": request.service,
        "headers": [list(item) for item in request.headers],
        "payload_hash": request.payload_hash,
    }


class BrokerState:
    def __init__(
        self,
        vaults: VaultStore,
        *,
        sigv4_clock: Callable[[], Any] | None = None,
        refresh_clock: Callable[[], float] | None = None,
        companion: CompanionChannelManager | None = None,
        datadog_refresh: Callable[[PupOAuthState, float], tuple[bytes, PupOAuthState]] | None = None,
        openai_refresh: Callable[[CodexOAuthState, float], tuple[bytes, CodexOAuthState]] | None = None,
        record_lock_timeout: float = DEFAULT_RECORD_LOCK_TIMEOUT_SECONDS,
    ):
        self._vaults = vaults
        self._sigv4_clock = sigv4_clock
        self._refresh_clock = refresh_clock
        self._companion = companion or CompanionChannelManager()
        self._datadog_refresh = datadog_refresh or (
            lambda state, now: refresh_datadog_oauth(state, now=now)
        )
        self._openai_refresh = openai_refresh or (
            lambda state, now: refresh_openai_codex_oauth(state, now=now)
        )
        if (
            isinstance(record_lock_timeout, bool)
            or not isinstance(record_lock_timeout, (int, float))
            or record_lock_timeout <= 0
            or record_lock_timeout > DEFAULT_RECORD_LOCK_TIMEOUT_SECONDS
        ):
            raise ValueError("record lock timeout is invalid")
        self._record_lock_timeout = float(record_lock_timeout)
        self._key: bytearray | None = None
        self._handles: dict[bytes, HandleRecord] = {}
        self._record_locks: dict[
            tuple[str, str, str, str], threading.Lock
        ] = {}
        self._active_refresh_tasks: dict[
            tuple[str, str, str, str], str
        ] = {}
        self._mutex = threading.RLock()

    @property
    def locked(self) -> bool:
        with self._mutex:
            return self._key is None

    @property
    def companion_channel(self) -> CompanionChannelManager:
        return self._companion

    def unlock(self, key: bytes) -> dict[str, Any]:
        if not isinstance(key, bytes) or len(key) != 32:
            raise BrokerError("invalid_key")
        with self._mutex:
            self._companion.invalidate()
            if self._key is not None:
                for index in range(len(self._key)):
                    self._key[index] = 0
            self._key = bytearray(key)
            self._handles.clear()
            self._record_locks.clear()
            self._active_refresh_tasks.clear()
        return {"schema_version": SCHEMA_VERSION, "ok": True, "state": "unlocked"}

    def prepare_companion(self, epoch_id: Any) -> dict[str, Any]:
        """Bind one non-secret host epoch to the in-memory installation key."""

        try:
            with self._mutex:
                key = self._require_key()
                epoch_key = derive_epoch_key(key, epoch_id)
                self._companion.prepare(epoch_id, epoch_key)
            state, current_epoch = self._companion.status()
            return {
                "schema_version": SCHEMA_VERSION,
                "ok": True,
                "state": state,
                "epoch_id": current_epoch,
            }
        except Exception as error:
            raise _translate_error(error) from None

    def companion_status(self) -> dict[str, Any]:
        state, epoch_id = self._companion.status()
        return {
            "schema_version": SCHEMA_VERSION,
            "ok": True,
            "state": state,
            "epoch_id": epoch_id,
        }

    def refresh_with_companion(
        self,
        request: RefreshRequest,
        cancel_event: threading.Event | None = None,
    ) -> RefreshResult:
        """Typed host refresh boundary; credential CAS remains caller-owned."""

        try:
            with self._mutex:
                self._require_key()
            return self._companion.refresh(request, cancel_event)
        except Exception as error:
            raise _translate_error(error) from None

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
        lock_keys = [
            key
            for key in self._record_locks
            if key[0] == context_id and key[1] == provider
        ]
        for key in lock_keys:
            del self._record_locks[key]
        active_keys = [
            key
            for key in self._active_refresh_tasks
            if key[0] == context_id and key[1] == provider
        ]
        for key in active_keys:
            del self._active_refresh_tasks[key]

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

    def _refresh_barrier_is_active(
        self, context_id: str, provider: str, credential: Any
    ) -> bool:
        if not isinstance(credential, dict):
            return False
        task_digest = credential.get("refresh_task_digest")
        if not isinstance(task_digest, str):
            return False
        key = (
            context_id,
            provider,
            credential.get("record_id"),
            credential.get("revision"),
        )
        active = self._active_refresh_tasks.get(key)
        return isinstance(active, str) and secrets.compare_digest(
            active, task_digest
        )

    def _index_handle(
        self,
        handle: str,
        context_id: str,
        project_id: str,
        provider: str,
        credential: dict[str, Any],
        bindings: tuple[NormalizedBinding, ...],
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
            if provider in {"aws", "datadog", "openai"}:
                raise BrokerError("invalid_provider")
            if not isinstance(secret, bytes) or not secret or len(secret) > MAX_SECRET_BYTES:
                raise BrokerError("invalid_secret")
            with self._mutex:
                payload = self._load_or_empty(context_id)
                record = new_record(secret, account_label=account_label)
                updated = {
                    "schema_version": payload["schema_version"],
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

    def login_aws_driver(
        self,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        """Commit one host-completed, executable-bound opaque AWS state."""

        try:
            context_id = validate_context_id(context_id)
            if not isinstance(state, bytes) or not state or len(state) > MAX_SECRET_BYTES:
                raise BrokerError("invalid_secret")
            record = new_aws_sso_record(
                state,
                account_label=account_label,
                driver_id=driver_id,
                driver_revision=driver_revision,
            )
            with self._mutex:
                payload = self._load_or_empty(context_id)
                updated = {
                    "schema_version": payload["schema_version"],
                    "providers": dict(payload["providers"]),
                }
                updated["providers"]["aws"] = record
                self._vaults.save(context_id, self._require_key(), updated)
                self._revoke(context_id, "aws")
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": "aws",
                    "revision": record["revision"],
                    "account_label": record["account_label"],
                }
        except Exception as error:
            raise _translate_error(error) from None

    def login_datadog_driver(
        self,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        """Commit one host-completed, executable-bound pup OAuth session."""

        try:
            context_id = validate_context_id(context_id)
            PupOAuthState.parse(state, driver_revision=driver_revision)
            record = new_datadog_oauth_record(
                state,
                account_label=account_label,
                driver_id=driver_id,
                driver_revision=driver_revision,
            )
            with self._mutex:
                payload = self._load_or_empty(context_id)
                updated = {
                    "schema_version": payload["schema_version"],
                    "providers": dict(payload["providers"]),
                }
                updated["providers"]["datadog"] = record
                self._vaults.save(context_id, self._require_key(), updated)
                self._revoke(context_id, "datadog")
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": "datadog",
                    "revision": record["revision"],
                    "account_label": record["account_label"],
                }
        except Exception as error:
            raise _translate_error(error) from None

    def login_openai_driver(
        self,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        """Commit one host-completed, executable-bound Codex OAuth session."""

        try:
            context_id = validate_context_id(context_id)
            parsed = CodexOAuthState.parse(state, driver_revision=driver_revision)
            if parsed.account_id != account_label:
                raise BrokerError("invalid_account_label")
            record = new_openai_codex_oauth_record(
                state,
                account_label=account_label,
                driver_id=driver_id,
                driver_revision=driver_revision,
            )
            with self._mutex:
                payload = self._load_or_empty(context_id)
                updated = {
                    "schema_version": payload["schema_version"],
                    "providers": dict(payload["providers"]),
                }
                updated["providers"]["openai"] = record
                self._vaults.save(context_id, self._require_key(), updated)
                self._revoke(context_id, "openai")
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": "openai",
                    "revision": record["revision"],
                    "account_label": record["account_label"],
                }
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
                        "schema_version": payload["schema_version"],
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
                if (
                    record is not None
                    and record.get("credential_kind") in {
                        AWS_SSO_CREDENTIAL_KIND, DATADOG_OAUTH_CREDENTIAL_KIND,
                        OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
                    }
                    and record.get("refresh_task_digest") is not None
                    and not self._refresh_barrier_is_active(
                        context_id, provider, record
                    )
                ):
                    # A prior host refresh crossed its durable execution
                    # barrier without a correlated commit. Treat it as absent
                    # until explicit host re-login replaces the record.
                    record = None
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
                if (
                    credential.get("credential_kind") in {
                        AWS_SSO_CREDENTIAL_KIND, DATADOG_OAUTH_CREDENTIAL_KIND,
                        OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
                    }
                    and credential.get("refresh_task_digest") is not None
                ):
                    raise BrokerError("credential_not_found")
                self._validate_credential_bindings(credential, normalized_bindings)
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
                    "schema_version": payload["schema_version"],
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

    @staticmethod
    def _validate_credential_bindings(
        credential: dict[str, Any], bindings: tuple[NormalizedBinding, ...]
    ) -> None:
        kind = credential.get("credential_kind")
        if kind == STATIC_CREDENTIAL_KIND and all(
            isinstance(binding, Binding) for binding in bindings
        ):
            return
        if kind == AWS_SSO_CREDENTIAL_KIND and all(
            isinstance(binding, AwsSigV4Binding) for binding in bindings
        ):
            return
        if kind == DATADOG_OAUTH_CREDENTIAL_KIND and all(
            isinstance(binding, Binding)
            and binding.secret_field == DATADOG_OAUTH_CREDENTIAL_KIND
            for binding in bindings
        ):
            return
        if kind == OPENAI_CODEX_OAUTH_CREDENTIAL_KIND and all(
            isinstance(binding, Binding)
            and binding.secret_field == OPENAI_CODEX_OAUTH_CREDENTIAL_KIND
            for binding in bindings
        ):
            return
        raise BrokerError("credential_binding_mismatch")

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
                if (
                    credential is not None
                    and credential["revision"] == revision
                    and not (
                        credential.get("credential_kind") in {
                            AWS_SSO_CREDENTIAL_KIND, DATADOG_OAUTH_CREDENTIAL_KIND,
                            OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
                        }
                        and credential.get("refresh_task_digest") is not None
                        and not self._refresh_barrier_is_active(
                            context_id, provider, credential
                        )
                    )
                ):
                    self._validate_credential_bindings(
                        credential, normalized_bindings
                    )
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
            if isinstance(binding, Binding)
            if binding.matches(target, source_header_value, source_format_value)
        ]
        if len(matches) != 1:
            raise BrokerError("handle_binding_mismatch")
        return matches[0], target, source_header_value, source_format_value

    @staticmethod
    def _selected_signing_binding(
        record: HandleRecord,
        target_value: Any,
        binding_value: Any,
    ) -> tuple[AwsSigV4Binding, Target]:
        target = Target.parse(target_value)
        binding = AwsSigV4Binding.parse(binding_value)
        matches = [
            persisted
            for persisted in record.bindings
            if isinstance(persisted, AwsSigV4Binding)
            and persisted == binding
            and persisted.matches_target(target)
        ]
        if len(matches) != 1:
            raise BrokerError("handle_binding_mismatch")
        return matches[0], target

    def _aws_refresh_snapshot(
        self,
        *,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: str,
        revision: str,
        binding: Any,
        request: SigV4Request,
    ) -> AwsRefreshSnapshot:
        """Validate and snapshot every mutable grant dimension under _mutex."""

        key = self._require_key()
        record = self._handle_record(handle, context_id, project_id, provider)
        if record.revision != revision:
            raise BrokerError("handle_binding_mismatch")
        selected, _ = self._selected_signing_binding(
            record,
            {"scheme": "https", "host": request.host, "port": 443},
            binding,
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
        if credential.get("credential_kind") != AWS_SSO_CREDENTIAL_KIND:
            raise BrokerError("credential_not_signable")
        if (
            credential.get("refresh_task_digest") is not None
            and not self._refresh_barrier_is_active(
                record.context_id, provider, credential
            )
        ):
            raise BrokerError("companion_outcome_unknown")
        state = decode_secret(credential["state"])
        return AwsRefreshSnapshot(
            context_id=record.context_id,
            project_id=record.project_id,
            provider=record.provider,
            record_id=record.record_id,
            revision=record.revision,
            state_generation=credential["state_generation"],
            driver_id=credential["driver_id"],
            driver_revision=credential["driver_revision"],
            binding_digest=_document_digest(selected.document()),
            request_digest=_document_digest(_signing_request_document(request)),
            state_sha256=hashlib.sha256(state).hexdigest(),
            state=state,
        )

    def _datadog_refresh_snapshot(
        self,
        *,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: str,
        revision: str,
        target: Any,
        source_header: Any,
        source_format: Any,
    ) -> tuple[DatadogRefreshSnapshot, Binding, Target, str, str]:
        key = self._require_key()
        record = self._handle_record(handle, context_id, project_id, provider)
        if record.revision != revision:
            raise BrokerError("handle_binding_mismatch")
        selected, normalized_target, normalized_header, normalized_format = self._selected_binding(
            record, target, source_header, source_format
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
        if credential.get("credential_kind") != DATADOG_OAUTH_CREDENTIAL_KIND:
            raise BrokerError("credential_not_resolvable")
        if (
            credential.get("refresh_task_digest") is not None
            and not self._refresh_barrier_is_active(record.context_id, provider, credential)
        ):
            raise BrokerError("companion_outcome_unknown")
        state = decode_secret(credential["state"])
        snapshot = DatadogRefreshSnapshot(
            context_id=record.context_id,
            project_id=record.project_id,
            provider=record.provider,
            record_id=record.record_id,
            revision=record.revision,
            state_generation=credential["state_generation"],
            driver_id=credential["driver_id"],
            driver_revision=credential["driver_revision"],
            binding_digest=_document_digest(selected.document()),
            state_sha256=hashlib.sha256(state).hexdigest(),
            state=state,
        )
        return snapshot, selected, normalized_target, normalized_header, normalized_format

    @staticmethod
    def _datadog_credential_matches_snapshot(
        credential: Any, snapshot: DatadogRefreshSnapshot
    ) -> bool:
        if not isinstance(credential, dict):
            return False
        try:
            state_sha256 = hashlib.sha256(decode_secret(credential["state"])).hexdigest()
        except (KeyError, VaultError):
            return False
        return (
            credential.get("credential_kind") == DATADOG_OAUTH_CREDENTIAL_KIND
            and credential.get("record_id") == snapshot.record_id
            and credential.get("revision") == snapshot.revision
            and credential.get("state_generation") == snapshot.state_generation
            and credential.get("driver_id") == snapshot.driver_id
            and credential.get("driver_revision") == snapshot.driver_revision
            and secrets.compare_digest(state_sha256, snapshot.state_sha256)
        )

    def _persist_datadog_refresh_barrier(
        self, snapshot: DatadogRefreshSnapshot, task_digest: str
    ) -> None:
        key = self._require_key()
        payload = self._vaults.load(snapshot.context_id, key)
        credential = payload["providers"].get(snapshot.provider)
        if (
            not self._datadog_credential_matches_snapshot(credential, snapshot)
            or credential.get("refresh_task_digest") is not None
        ):
            raise BrokerError("handle_revoked")
        updated_credential = dict(credential)
        updated_credential["refresh_task_digest"] = task_digest
        updated = {"schema_version": payload["schema_version"], "providers": dict(payload["providers"])}
        updated["providers"][snapshot.provider] = updated_credential
        self._vaults.save(snapshot.context_id, key, updated)

    def _openai_refresh_snapshot(
        self,
        *,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: str,
        revision: str,
        target: Any,
        source_header: Any,
        source_format: Any,
    ) -> tuple[OpenAIRefreshSnapshot, Binding, Target, str, str]:
        key = self._require_key()
        record = self._handle_record(handle, context_id, project_id, provider)
        if record.revision != revision:
            raise BrokerError("handle_binding_mismatch")
        selected, normalized_target, normalized_header, normalized_format = self._selected_binding(
            record, target, source_header, source_format
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
        if credential.get("credential_kind") != OPENAI_CODEX_OAUTH_CREDENTIAL_KIND:
            raise BrokerError("credential_not_resolvable")
        if (
            credential.get("refresh_task_digest") is not None
            and not self._refresh_barrier_is_active(record.context_id, provider, credential)
        ):
            raise BrokerError("companion_outcome_unknown")
        state = decode_secret(credential["state"])
        snapshot = OpenAIRefreshSnapshot(
            context_id=record.context_id,
            project_id=record.project_id,
            provider=record.provider,
            record_id=record.record_id,
            revision=record.revision,
            state_generation=credential["state_generation"],
            driver_id=credential["driver_id"],
            driver_revision=credential["driver_revision"],
            binding_digest=_document_digest(selected.document()),
            state_sha256=hashlib.sha256(state).hexdigest(),
            state=state,
        )
        return snapshot, selected, normalized_target, normalized_header, normalized_format

    @staticmethod
    def _openai_credential_matches_snapshot(
        credential: Any, snapshot: OpenAIRefreshSnapshot
    ) -> bool:
        if not isinstance(credential, dict):
            return False
        try:
            state_sha256 = hashlib.sha256(decode_secret(credential["state"])).hexdigest()
        except (KeyError, VaultError):
            return False
        return (
            credential.get("credential_kind") == OPENAI_CODEX_OAUTH_CREDENTIAL_KIND
            and credential.get("record_id") == snapshot.record_id
            and credential.get("revision") == snapshot.revision
            and credential.get("state_generation") == snapshot.state_generation
            and credential.get("driver_id") == snapshot.driver_id
            and credential.get("driver_revision") == snapshot.driver_revision
            and secrets.compare_digest(state_sha256, snapshot.state_sha256)
        )

    def _persist_openai_refresh_barrier(
        self, snapshot: OpenAIRefreshSnapshot, task_digest: str
    ) -> None:
        key = self._require_key()
        payload = self._vaults.load(snapshot.context_id, key)
        credential = payload["providers"].get(snapshot.provider)
        if (
            not self._openai_credential_matches_snapshot(credential, snapshot)
            or credential.get("refresh_task_digest") is not None
        ):
            raise BrokerError("handle_revoked")
        updated_credential = dict(credential)
        updated_credential["refresh_task_digest"] = task_digest
        updated = {
            "schema_version": payload["schema_version"],
            "providers": dict(payload["providers"]),
        }
        updated["providers"][snapshot.provider] = updated_credential
        self._vaults.save(snapshot.context_id, key, updated)

    @staticmethod
    def _aws_credential_matches_snapshot(
        credential: Any, snapshot: AwsRefreshSnapshot
    ) -> bool:
        if not isinstance(credential, dict):
            return False
        try:
            state_sha256 = hashlib.sha256(
                decode_secret(credential["state"])
            ).hexdigest()
        except (KeyError, VaultError):
            return False
        return (
            credential.get("credential_kind") == AWS_SSO_CREDENTIAL_KIND
            and credential.get("record_id") == snapshot.record_id
            and credential.get("revision") == snapshot.revision
            and credential.get("state_generation") == snapshot.state_generation
            and credential.get("driver_id") == snapshot.driver_id
            and credential.get("driver_revision") == snapshot.driver_revision
            and secrets.compare_digest(state_sha256, snapshot.state_sha256)
        )

    def _persist_refresh_barrier(
        self, snapshot: AwsRefreshSnapshot, task_digest: str
    ) -> None:
        """Persist no-replay intent before any host provider execution."""

        key = self._require_key()
        payload = self._vaults.load(snapshot.context_id, key)
        credential = payload["providers"].get(snapshot.provider)
        if (
            not self._aws_credential_matches_snapshot(credential, snapshot)
            or credential.get("refresh_task_digest") is not None
        ):
            raise BrokerError("handle_revoked")
        updated_credential = dict(credential)
        updated_credential["refresh_task_digest"] = task_digest
        updated = {
            "schema_version": payload["schema_version"],
            "providers": dict(payload["providers"]),
        }
        updated["providers"][snapshot.provider] = updated_credential
        self._vaults.save(snapshot.context_id, key, updated)

    def _clear_refresh_barrier(
        self, snapshot: AwsRefreshSnapshot, task_digest: str
    ) -> bool:
        """Clear only a proven pre-execution failure for this exact task."""

        key = self._require_key()
        payload = self._vaults.load(snapshot.context_id, key)
        credential = payload["providers"].get(snapshot.provider)
        if (
            not self._aws_credential_matches_snapshot(credential, snapshot)
            or credential.get("refresh_task_digest") != task_digest
        ):
            return False
        updated_credential = dict(credential)
        updated_credential["refresh_task_digest"] = None
        updated = {
            "schema_version": payload["schema_version"],
            "providers": dict(payload["providers"]),
        }
        updated["providers"][snapshot.provider] = updated_credential
        self._vaults.save(snapshot.context_id, key, updated)
        return True

    def _finish_active_refresh(
        self, snapshot: AwsRefreshSnapshot | DatadogRefreshSnapshot | OpenAIRefreshSnapshot,
        task_digest: str,
    ) -> None:
        with self._mutex:
            active = self._active_refresh_tasks.get(snapshot.lock_key)
            if isinstance(active, str) and secrets.compare_digest(
                active, task_digest
            ):
                del self._active_refresh_tasks[snapshot.lock_key]

    def _record_lock(
        self, snapshot: AwsRefreshSnapshot | DatadogRefreshSnapshot | OpenAIRefreshSnapshot
    ) -> threading.Lock:
        existing = self._record_locks.get(snapshot.lock_key)
        if existing is not None:
            return existing
        created = threading.Lock()
        self._record_locks[snapshot.lock_key] = created
        return created

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
                if any(
                    isinstance(binding, AwsSigV4Binding)
                    for binding in record.bindings
                ):
                    raise BrokerError("credential_not_resolvable")
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

    def introspect_signing(
        self,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: Any,
        target: Any,
        binding: Any,
    ) -> dict[str, Any]:
        try:
            provider = validate_provider_id(provider)
            with self._mutex:
                key = self._require_key()
                record = self._handle_record(handle, context_id, project_id, provider)
                selected, normalized_target = self._selected_signing_binding(
                    record, target, binding
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
                if credential.get("credential_kind") != AWS_SSO_CREDENTIAL_KIND:
                    raise BrokerError("credential_not_signable")
                if (
                    credential.get("refresh_task_digest") is not None
                    and not self._refresh_barrier_is_active(
                        record.context_id, provider, credential
                    )
                ):
                    raise BrokerError("companion_outcome_unknown")
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": record.provider,
                    "revision": record.revision,
                    "kind": "aws_sigv4",
                    "target": normalized_target.document(),
                    "source": {
                        "authorization_header": selected.authorization_header,
                        "security_token_header": selected.security_token_header,
                    },
                    "secret_headers": list(selected.secret_headers),
                }
        except Exception as error:
            raise _translate_error(error) from None

    def _resolve_openai_after_lock(
        self,
        *,
        initial: OpenAIRefreshSnapshot,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: str,
        revision: str,
        target: Any,
        source_header: Any,
        source_format: Any,
    ) -> dict[str, Any]:
        with self._mutex:
            snapshot, binding, normalized_target, normalized_header, normalized_format = (
                self._openai_refresh_snapshot(
                    handle=handle,
                    context_id=context_id,
                    project_id=project_id,
                    provider=provider,
                    revision=revision,
                    target=target,
                    source_header=source_header,
                    source_format=source_format,
                )
            )
            if snapshot.lock_key != initial.lock_key:
                raise BrokerError("handle_revoked")
            parsed = CodexOAuthState.parse(
                snapshot.state, driver_revision=snapshot.driver_revision
            )
            now = self._refresh_clock() if self._refresh_clock is not None else time.time()
            secret = parsed.access_token(now)
            if secret is None:
                if snapshot.state_generation >= (1 << 63) - 1:
                    raise BrokerError("state_generation_exhausted")
                task_digest = secrets.token_hex(32)
                self._persist_openai_refresh_barrier(snapshot, task_digest)
                self._active_refresh_tasks[snapshot.lock_key] = task_digest
            else:
                task_digest = ""

        if secret is None:
            try:
                secret, refreshed = self._openai_refresh(parsed, now)
                refreshed_state = refreshed.encode()
                reparsed = CodexOAuthState.parse(
                    refreshed_state, driver_revision=snapshot.driver_revision
                )
                if (
                    not secret
                    or reparsed.account_id != parsed.account_id
                    or reparsed.access_token(now) != secret
                ):
                    raise BrokerError("openai_oauth_refresh_failed")
            except Exception:
                self._finish_active_refresh(snapshot, task_digest)
                raise BrokerError("companion_outcome_unknown") from None
            try:
                with self._mutex:
                    key = self._require_key()
                    payload = self._vaults.load(snapshot.context_id, key)
                    current = payload["providers"].get(provider)
                    if (
                        not self._openai_credential_matches_snapshot(current, snapshot)
                        or current.get("refresh_task_digest") != task_digest
                    ):
                        raise BrokerError("handle_revoked")
                    updated_credential = dict(current)
                    updated_credential["state"] = encode_secret(refreshed_state)
                    updated_credential["state_generation"] = snapshot.state_generation + 1
                    updated_credential["refresh_task_digest"] = None
                    updated = {
                        "schema_version": payload["schema_version"],
                        "providers": dict(payload["providers"]),
                    }
                    updated["providers"][provider] = updated_credential
                    self._vaults.save(snapshot.context_id, key, updated)
                    parsed = reparsed
            except BrokerError:
                self._finish_active_refresh(snapshot, task_digest)
                raise
            except Exception:
                self._finish_active_refresh(snapshot, task_digest)
                raise BrokerError("companion_outcome_unknown") from None
            self._finish_active_refresh(snapshot, task_digest)

        return self._resolved_secret_response(
            provider,
            revision,
            normalized_target,
            normalized_header,
            normalized_format,
            binding,
            secret,
            supplemental_headers={"chatgpt-account-id": parsed.account_id},
        )

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
                if any(
                    isinstance(binding, AwsSigV4Binding)
                    for binding in record.bindings
                ):
                    raise BrokerError("credential_not_resolvable")
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
                kind = credential.get("credential_kind")
                if kind == STATIC_CREDENTIAL_KIND:
                    secret = decode_secret(credential["secret"])
                    return self._resolved_secret_response(
                        provider, revision, normalized_target, normalized_header,
                        normalized_format, selected_binding, secret
                    )
                if kind == OPENAI_CODEX_OAUTH_CREDENTIAL_KIND:
                    initial, _, _, _, _ = self._openai_refresh_snapshot(
                        handle=handle, context_id=context_id, project_id=project_id,
                        provider=provider, revision=revision, target=target,
                        source_header=source_header, source_format=source_format,
                    )
                elif kind == DATADOG_OAUTH_CREDENTIAL_KIND:
                    initial, _, _, _, _ = self._datadog_refresh_snapshot(
                        handle=handle, context_id=context_id, project_id=project_id,
                        provider=provider, revision=revision, target=target,
                        source_header=source_header, source_format=source_format,
                    )
                else:
                    raise BrokerError("credential_not_resolvable")
                record_lock = self._record_lock(initial)

            if not record_lock.acquire(timeout=self._record_lock_timeout):
                raise BrokerError("companion_busy")
            try:
                if kind == OPENAI_CODEX_OAUTH_CREDENTIAL_KIND:
                    return self._resolve_openai_after_lock(
                        initial=initial,
                        handle=handle,
                        context_id=context_id,
                        project_id=project_id,
                        provider=provider,
                        revision=revision,
                        target=target,
                        source_header=source_header,
                        source_format=source_format,
                    )
                with self._mutex:
                    snapshot, selected_binding, normalized_target, normalized_header, normalized_format = self._datadog_refresh_snapshot(
                        handle=handle, context_id=context_id, project_id=project_id,
                        provider=provider, revision=revision, target=target,
                        source_header=source_header, source_format=source_format,
                    )
                    if snapshot.lock_key != initial.lock_key:
                        raise BrokerError("handle_revoked")
                    parsed = PupOAuthState.parse(
                        snapshot.state, driver_revision=snapshot.driver_revision
                    )
                    now = self._refresh_clock() if self._refresh_clock is not None else time.time()
                    secret = parsed.access_token(now)
                    if secret is None:
                        if snapshot.state_generation >= (1 << 63) - 1:
                            raise BrokerError("state_generation_exhausted")
                        task_digest = secrets.token_hex(32)
                        self._persist_datadog_refresh_barrier(snapshot, task_digest)
                        self._active_refresh_tasks[snapshot.lock_key] = task_digest
                    else:
                        task_digest = ""

                if secret is None:
                    try:
                        secret, refreshed = self._datadog_refresh(parsed, now)
                        refreshed_state = refreshed.encode()
                        PupOAuthState.parse(
                            refreshed_state, driver_revision=snapshot.driver_revision
                        )
                        if not secret or refreshed.access_token(now) != secret:
                            raise BrokerError("datadog_oauth_refresh_failed")
                    except Exception:
                        self._finish_active_refresh(snapshot, task_digest)
                        raise BrokerError("companion_outcome_unknown") from None
                    try:
                        with self._mutex:
                            key = self._require_key()
                            payload = self._vaults.load(snapshot.context_id, key)
                            current = payload["providers"].get(provider)
                            if (
                                not self._datadog_credential_matches_snapshot(current, snapshot)
                                or current.get("refresh_task_digest") != task_digest
                            ):
                                raise BrokerError("handle_revoked")
                            updated_credential = dict(current)
                            updated_credential["state"] = encode_secret(refreshed_state)
                            updated_credential["state_generation"] = snapshot.state_generation + 1
                            updated_credential["refresh_task_digest"] = None
                            updated = {
                                "schema_version": payload["schema_version"],
                                "providers": dict(payload["providers"]),
                            }
                            updated["providers"][provider] = updated_credential
                            self._vaults.save(snapshot.context_id, key, updated)
                    except BrokerError:
                        self._finish_active_refresh(snapshot, task_digest)
                        raise
                    except Exception:
                        self._finish_active_refresh(snapshot, task_digest)
                        raise BrokerError("companion_outcome_unknown") from None
                    self._finish_active_refresh(snapshot, task_digest)

                return self._resolved_secret_response(
                    provider, revision, normalized_target, normalized_header,
                    normalized_format, selected_binding, secret
                )
            finally:
                record_lock.release()
        except Exception as error:
            raise _translate_error(error) from None

    @staticmethod
    def _resolved_secret_response(
        provider: str,
        revision: str,
        normalized_target: Target,
        normalized_header: str,
        normalized_format: str,
        selected_binding: Binding,
        secret: bytes,
        supplemental_headers: dict[str, str] | None = None,
    ) -> dict[str, Any]:
        response = {
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
        if supplemental_headers is not None:
            response["supplemental_headers"] = supplemental_headers
        return response

    def sign_sigv4(
        self,
        handle: Any,
        context_id: Any,
        project_id: Any,
        provider: Any,
        revision: Any,
        binding: Any,
        request: Any,
    ) -> dict[str, Any]:
        try:
            # The complete signing request is validated before any credential
            # lookup, lock acquisition, companion call, or vault mutation.
            provider = validate_provider_id(provider)
            revision = _validate_revision(revision)
            normalized_request = parse_request(request)
            with self._mutex:
                initial = self._aws_refresh_snapshot(
                    handle=handle,
                    context_id=context_id,
                    project_id=project_id,
                    provider=provider,
                    revision=revision,
                    binding=binding,
                    request=normalized_request,
                )
                record_lock = self._record_lock(initial)

            # The lock is specific to one immutable record/revision. Waiting
            # and host/provider I/O never hold the installation-wide mutex.
            if not record_lock.acquire(timeout=self._record_lock_timeout):
                raise BrokerError("companion_busy")
            try:
                with self._mutex:
                    snapshot = self._aws_refresh_snapshot(
                        handle=handle,
                        context_id=context_id,
                        project_id=project_id,
                        provider=provider,
                        revision=revision,
                        binding=binding,
                        request=normalized_request,
                    )
                    if snapshot.lock_key != initial.lock_key:
                        raise BrokerError("handle_revoked")
                    if snapshot.state_generation >= (1 << 63) - 1:
                        raise BrokerError("state_generation_exhausted")
                    refresh_request = RefreshRequest.create(
                        context_id=snapshot.context_id,
                        project_id=snapshot.project_id,
                        provider=snapshot.provider,
                        record_id=snapshot.record_id,
                        grant_revision=snapshot.revision,
                        state_generation=snapshot.state_generation,
                        driver_id=snapshot.driver_id,
                        driver_revision=snapshot.driver_revision,
                        binding_digest=snapshot.binding_digest,
                        request_digest=snapshot.request_digest,
                        state=snapshot.state,
                    )
                    # This encrypted marker makes a Broker crash after host
                    # execution fail closed across restart. Only the exact
                    # correlated task may replace it with refreshed state.
                    self._persist_refresh_barrier(
                        snapshot, refresh_request.task_digest
                    )
                    self._active_refresh_tasks[
                        snapshot.lock_key
                    ] = refresh_request.task_digest

                try:
                    result = self.refresh_with_companion(refresh_request)
                    if (
                        not isinstance(result, RefreshResult)
                        or result.request_id != refresh_request.request_id
                        or not secrets.compare_digest(
                            result.task_digest, refresh_request.task_digest
                        )
                        or result.state_generation != snapshot.state_generation
                    ):
                        raise BrokerError("companion_result_invalid")
                    decode_kwargs: dict[str, Any] = {}
                    if self._refresh_clock is not None:
                        decode_kwargs["clock"] = self._refresh_clock
                    refreshed = decode_refresh_secret(
                        result.secret_payload, **decode_kwargs
                    )
                    # Treat malformed temporary credentials as an unknown
                    # refresh outcome before committing even opaque state.
                    credentials = parse_credentials(
                        {
                            "access_key_id": refreshed.access_key_id,
                            "secret_access_key": refreshed.secret_access_key,
                            "session_token": refreshed.session_token,
                        }
                    )
                    sign_kwargs: dict[str, Any] = {}
                    if self._sigv4_clock is not None:
                        sign_kwargs["clock"] = self._sigv4_clock
                    signed = sign(normalized_request, credentials, **sign_kwargs)
                except Exception as error:
                    self._finish_active_refresh(
                        snapshot, refresh_request.task_digest
                    )
                    translated = _translate_error(error)
                    if translated.code in PROVEN_PRE_EXECUTION_REFRESH_ERRORS:
                        try:
                            with self._mutex:
                                cleared = self._clear_refresh_barrier(
                                    snapshot, refresh_request.task_digest
                                )
                        except Exception:
                            raise BrokerError("companion_outcome_unknown") from None
                        if not cleared:
                            raise BrokerError("handle_revoked") from None
                        raise translated
                    # Once execution might have begun, malformed, missing, or
                    # failed results leave the durable barrier in place. A host
                    # re-login is the only operation that replaces it.
                    raise BrokerError("companion_outcome_unknown") from None

                try:
                    with self._mutex:
                        key = self._require_key()
                        payload = self._vaults.load(snapshot.context_id, key)
                        credential = payload["providers"].get(provider)
                        if (
                            not self._aws_credential_matches_snapshot(
                                credential, snapshot
                            )
                            or credential.get("refresh_task_digest")
                            != refresh_request.task_digest
                        ):
                            raise BrokerError("handle_revoked")
                        updated_credential = dict(credential)
                        updated_credential["state"] = encode_secret(refreshed.state)
                        updated_credential["state_generation"] = (
                            snapshot.state_generation + 1
                        )
                        updated_credential["refresh_task_digest"] = None
                        updated = {
                            "schema_version": payload["schema_version"],
                            "providers": dict(payload["providers"]),
                        }
                        updated["providers"][provider] = updated_credential
                        self._vaults.save(snapshot.context_id, key, updated)
                except BrokerError:
                    self._finish_active_refresh(
                        snapshot, refresh_request.task_digest
                    )
                    raise
                except Exception:
                    self._finish_active_refresh(
                        snapshot, refresh_request.task_digest
                    )
                    raise BrokerError("companion_outcome_unknown") from None

                self._finish_active_refresh(snapshot, refresh_request.task_digest)

                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": provider,
                    "revision": revision,
                    "headers": {
                        "authorization": signed.authorization,
                        "x_amz_date": signed.amz_date,
                        "x_amz_security_token": signed.security_token,
                        "x_amz_content_sha256": signed.content_sha256,
                    },
                }
            finally:
                record_lock.release()
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
        if operation == "unlock":
            key = "key_length"
        elif operation == "login" and request.get("provider") in {"aws", "datadog", "openai"}:
            key = "state_length"
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
            provider = request.get("provider")
            if provider in {"github", "anthropic"}:
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
                if (
                    provider == "anthropic"
                    and request["account_label"] != CLAUDE_ACCOUNT_LABEL
                ):
                    raise ProtocolError("invalid_request")
                return self._state.import_secret(
                    request["context_id"],
                    provider,
                    raw_payload,
                    account_label=request["account_label"],
                )
            if provider == "aws":
                require_exact_keys(
                    request,
                    {
                        "schema_version",
                        "op",
                        "context_id",
                        "provider",
                        "account_label",
                        "driver_id",
                        "driver_revision",
                        "state_length",
                    },
                )
                if len(raw_payload) != request["state_length"]:
                    raise ProtocolError("invalid_length")
                return self._state.login_aws_driver(
                    request["context_id"],
                    raw_payload,
                    account_label=request["account_label"],
                    driver_id=request["driver_id"],
                    driver_revision=request["driver_revision"],
                )
            if provider == "datadog":
                require_exact_keys(
                    request,
                    {
                        "schema_version", "op", "context_id", "provider",
                        "account_label", "driver_id", "driver_revision", "state_length",
                    },
                )
                if len(raw_payload) != request["state_length"]:
                    raise ProtocolError("invalid_length")
                return self._state.login_datadog_driver(
                    request["context_id"], raw_payload,
                    account_label=request["account_label"],
                    driver_id=request["driver_id"],
                    driver_revision=request["driver_revision"],
                )
            if provider == "openai":
                require_exact_keys(
                    request,
                    {
                        "schema_version", "op", "context_id", "provider",
                        "account_label", "driver_id", "driver_revision", "state_length",
                    },
                )
                if len(raw_payload) != request["state_length"]:
                    raise ProtocolError("invalid_length")
                return self._state.login_openai_driver(
                    request["context_id"], raw_payload,
                    account_label=request["account_label"],
                    driver_id=request["driver_id"],
                    driver_revision=request["driver_revision"],
                )
            raise ProtocolError("invalid_provider")
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
