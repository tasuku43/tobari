"""Locked broker state, handle isolation, and credential lifecycle orchestration."""

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

# Keep the established broker module namespace stable while implementations
# move behind one-way internal module boundaries.

from . import SCHEMA_VERSION
from .aws_sigv4 import SigV4Request
from .companion_protocol import (
    CompanionChannelManager,
    RefreshRequest,
    RefreshResult,
    derive_epoch_key,
)
from .credential_records import (
    AWS_SSO_CREDENTIAL_KIND,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    STATIC_CREDENTIAL_KIND,
    VaultError,
    decode_secret,
    empty_payload,
    encode_secret,
    new_record,
    validate_context_id,
    validate_provider_id,
)
from .control_login import (
    DriverControlLogin,
    DriverControlLoginPlan,
    control_login_plan,
    is_reviewed_driver_login_provider,
)
from .broker_contract import (
    DEFAULT_RECORD_LOCK_TIMEOUT_SECONDS,
    DESTINATION_FORMATS,
    HANDLE_PATTERN,
    HEADER_PATTERN,
    HOST_PATTERN,
    PROJECT_ID_PATTERN,
    PROVEN_PRE_EXECUTION_REFRESH_ERRORS,
    SOURCE_FORMATS,
    AwsRefreshSnapshot,
    AwsSigV4Binding,
    Binding,
    BrokerError,
    CompanionError,
    DatadogOAuthError,
    HandleRecord,
    NormalizedBinding,
    OpenAICodexOAuthError,
    ProtocolError,
    RenewableSessionSnapshot,
    SigV4Error,
    Target,
    _document_digest,
    _parse_bindings,
    _signing_request_document,
    _translate_error,
    _validate_ascii,
    _validate_handle,
    _validate_header,
    _validate_project_id,
    _validate_revision,
)
from .dispatcher import Dispatcher
from .protocol import MAX_SECRET_BYTES, require_exact_keys
from .renewable import (
    OpenAIAccountSupplement,
    RENEWABLE_CREDENTIAL_KINDS,
    RefreshedRenewableSession,
    RenewableSessionAdapter,
    ReviewedRenewableSessionDependencies,
    ResolvedRenewableSecret,
    reviewed_renewable_session_adapters,
)
from .request_signing import (
    REQUEST_SIGNING_CREDENTIAL_KINDS,
    RequestSigningAdapter,
    ReviewedRequestSigningDependencies,
    reviewed_request_signing_adapters,
)
from .vault import VaultStore


class BrokerState:
    def __init__(
        self,
        vaults: VaultStore,
        *,
        sigv4_clock: Callable[[], Any] | None = None,
        refresh_clock: Callable[[], float] | None = None,
        companion: CompanionChannelManager | None = None,
        renewable_dependencies: ReviewedRenewableSessionDependencies | None = None,
        record_lock_timeout: float = DEFAULT_RECORD_LOCK_TIMEOUT_SECONDS,
    ):
        self._vaults = vaults
        self._refresh_clock = refresh_clock
        self._companion = companion or CompanionChannelManager()
        self._renewable_adapters = reviewed_renewable_session_adapters(
            renewable_dependencies
        )
        self._request_signing_adapters = reviewed_request_signing_adapters(
            ReviewedRequestSigningDependencies(
                refresh_clock=refresh_clock,
                sigv4_clock=sigv4_clock,
            )
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
            if is_reviewed_driver_login_provider(provider):
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

    def login_driver(self, login: DriverControlLogin) -> dict[str, Any]:
        """Commit one reviewed host-completed driver state."""

        try:
            if not isinstance(login, DriverControlLogin):
                raise BrokerError("invalid_request")
            reviewed = control_login_plan(login.plan.provider_id)
            if not isinstance(reviewed, DriverControlLoginPlan) or reviewed is not login.plan:
                raise BrokerError("invalid_provider")
            context_id = validate_context_id(login.context_id)
            if (
                not isinstance(login.state, bytes)
                or not login.state
                or len(login.state) > MAX_SECRET_BYTES
            ):
                raise BrokerError("invalid_secret")
            adapter = self._renewable_adapters.get(reviewed.credential_kind)
            signing_adapter = self._request_signing_adapters.get(
                reviewed.credential_kind
            )
            if adapter is None and signing_adapter is None:
                raise BrokerError("invalid_provider")
            if adapter is not None:
                if adapter.provider_id != reviewed.provider_id:
                    raise BrokerError("invalid_provider")
                adapter.validate_initial_state(
                    login.state,
                    driver_revision=login.driver_revision,
                    account_label=login.account_label,
                )
            elif signing_adapter.provider_id != reviewed.provider_id:
                raise BrokerError("invalid_provider")
            record = reviewed.new_record(login)
            provider = reviewed.provider_id
            with self._mutex:
                payload = self._load_or_empty(context_id)
                updated = {
                    "schema_version": payload["schema_version"],
                    "providers": dict(payload["providers"]),
                }
                updated["providers"][provider] = record
                self._vaults.save(context_id, self._require_key(), updated)
                self._revoke(context_id, provider)
                return {
                    "schema_version": SCHEMA_VERSION,
                    "ok": True,
                    "provider": provider,
                    "revision": record["revision"],
                    "account_label": record["account_label"],
                }
        except Exception as error:
            raise _translate_error(error) from None

    def _login_driver_compatibility(
        self,
        provider: str,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        plan = control_login_plan(provider)
        if not isinstance(plan, DriverControlLoginPlan):
            raise BrokerError("invalid_provider")
        return self.login_driver(
            DriverControlLogin(
                plan=plan,
                context_id=context_id,
                account_label=account_label,
                driver_id=driver_id,
                driver_revision=driver_revision,
                state=state,
            )
        )

    def login_aws_driver(
        self,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        return self._login_driver_compatibility(
            "aws",
            context_id,
            state,
            account_label=account_label,
            driver_id=driver_id,
            driver_revision=driver_revision,
        )

    def login_datadog_driver(
        self,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        return self._login_driver_compatibility(
            "datadog",
            context_id,
            state,
            account_label=account_label,
            driver_id=driver_id,
            driver_revision=driver_revision,
        )

    def login_openai_driver(
        self,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        return self._login_driver_compatibility(
            "openai",
            context_id,
            state,
            account_label=account_label,
            driver_id=driver_id,
            driver_revision=driver_revision,
        )

    def login_anthropic_driver(
        self,
        context_id: Any,
        state: bytes,
        *,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
    ) -> dict[str, Any]:
        return self._login_driver_compatibility(
            "anthropic",
            context_id,
            state,
            account_label=account_label,
            driver_id=driver_id,
            driver_revision=driver_revision,
        )

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
                    and self._uses_refresh_barrier(record.get("credential_kind"))
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
                    self._uses_refresh_barrier(credential.get("credential_kind"))
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
    def _uses_refresh_barrier(credential_kind: Any) -> bool:
        return (
            credential_kind in REQUEST_SIGNING_CREDENTIAL_KINDS
            or credential_kind in RENEWABLE_CREDENTIAL_KINDS
        )

    def _renewable_adapter_for(
        self, credential_kind: Any, provider: str
    ) -> RenewableSessionAdapter:
        adapter = self._renewable_adapters.get(credential_kind)
        if adapter is None or adapter.provider_id != provider:
            raise BrokerError("credential_not_resolvable")
        return adapter

    def _request_signing_adapter_for(
        self, credential_kind: Any, provider: str
    ) -> RequestSigningAdapter:
        adapter = self._request_signing_adapters.get(credential_kind)
        if adapter is None or adapter.provider_id != provider:
            raise BrokerError("credential_not_signable")
        return adapter

    @staticmethod
    def _validate_credential_bindings(
        credential: dict[str, Any], bindings: tuple[NormalizedBinding, ...]
    ) -> None:
        kind = credential.get("credential_kind")
        if kind == STATIC_CREDENTIAL_KIND and all(
            isinstance(binding, Binding) for binding in bindings
        ):
            return
        if kind in REQUEST_SIGNING_CREDENTIAL_KINDS and all(
            isinstance(binding, AwsSigV4Binding) for binding in bindings
        ):
            return
        if kind in RENEWABLE_CREDENTIAL_KINDS and all(
            isinstance(binding, Binding)
            and binding.secret_field == kind
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
                        self._uses_refresh_barrier(
                            credential.get("credential_kind")
                        )
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
        self._request_signing_adapter_for(
            credential.get("credential_kind"), provider
        )
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

    def _renewable_session_snapshot(
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
    ) -> tuple[RenewableSessionSnapshot, Binding, Target, str, str]:
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
        credential_kind = credential.get("credential_kind")
        self._renewable_adapter_for(credential_kind, provider)
        if (
            credential.get("refresh_task_digest") is not None
            and not self._refresh_barrier_is_active(record.context_id, provider, credential)
        ):
            raise BrokerError("companion_outcome_unknown")
        state = decode_secret(credential["state"])
        account_label = credential.get("account_label")
        if not isinstance(account_label, str) or not account_label:
            raise BrokerError("credential_not_resolvable")
        snapshot = RenewableSessionSnapshot(
            context_id=record.context_id,
            project_id=record.project_id,
            provider=record.provider,
            credential_kind=credential_kind,
            record_id=record.record_id,
            revision=record.revision,
            account_label=account_label,
            state_generation=credential["state_generation"],
            driver_id=credential["driver_id"],
            driver_revision=credential["driver_revision"],
            binding_digest=_document_digest(selected.document()),
            state_sha256=hashlib.sha256(state).hexdigest(),
            state=state,
        )
        return snapshot, selected, normalized_target, normalized_header, normalized_format

    @staticmethod
    def _renewable_credential_matches_snapshot(
        credential: Any, snapshot: RenewableSessionSnapshot
    ) -> bool:
        if not isinstance(credential, dict):
            return False
        try:
            state_sha256 = hashlib.sha256(decode_secret(credential["state"])).hexdigest()
        except (KeyError, VaultError):
            return False
        return (
            credential.get("credential_kind") == snapshot.credential_kind
            and credential.get("record_id") == snapshot.record_id
            and credential.get("revision") == snapshot.revision
            and credential.get("account_label") == snapshot.account_label
            and credential.get("state_generation") == snapshot.state_generation
            and credential.get("driver_id") == snapshot.driver_id
            and credential.get("driver_revision") == snapshot.driver_revision
            and secrets.compare_digest(state_sha256, snapshot.state_sha256)
        )

    def _persist_renewable_refresh_barrier(
        self, snapshot: RenewableSessionSnapshot, task_digest: str
    ) -> None:
        key = self._require_key()
        payload = self._vaults.load(snapshot.context_id, key)
        credential = payload["providers"].get(snapshot.provider)
        if (
            not self._renewable_credential_matches_snapshot(credential, snapshot)
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
        self, snapshot: AwsRefreshSnapshot | RenewableSessionSnapshot,
        task_digest: str,
    ) -> None:
        with self._mutex:
            active = self._active_refresh_tasks.get(snapshot.lock_key)
            if isinstance(active, str) and secrets.compare_digest(
                active, task_digest
            ):
                del self._active_refresh_tasks[snapshot.lock_key]

    def _record_lock(
        self, snapshot: AwsRefreshSnapshot | RenewableSessionSnapshot
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
                signing_adapter = self._request_signing_adapter_for(
                    credential.get("credential_kind"), provider
                )
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
                    "kind": signing_adapter.binding_kind,
                    "target": normalized_target.document(),
                    "source": {
                        "authorization_header": selected.authorization_header,
                        "security_token_header": selected.security_token_header,
                    },
                    "secret_headers": list(selected.secret_headers),
                }
        except Exception as error:
            raise _translate_error(error) from None

    @staticmethod
    def _renewable_supplemental_headers(
        resolved: ResolvedRenewableSecret,
    ) -> dict[str, str] | None:
        supplemental = resolved.supplemental
        if supplemental is None:
            return None
        if isinstance(supplemental, OpenAIAccountSupplement):
            return {"chatgpt-account-id": supplemental.account_id}
        raise BrokerError("credential_not_resolvable")

    @staticmethod
    def _validate_renewable_resolution(
        resolved: Any,
    ) -> ResolvedRenewableSecret:
        if (
            not isinstance(resolved, ResolvedRenewableSecret)
            or not isinstance(resolved.secret, bytes)
            or not resolved.secret
            or len(resolved.secret) > MAX_SECRET_BYTES
        ):
            raise BrokerError("credential_not_resolvable")
        BrokerState._renewable_supplemental_headers(resolved)
        return resolved

    def _resolve_renewable_after_lock(
        self,
        *,
        initial: RenewableSessionSnapshot,
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
                self._renewable_session_snapshot(
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
            if (
                snapshot.lock_key != initial.lock_key
                or snapshot.credential_kind != initial.credential_kind
            ):
                raise BrokerError("handle_revoked")
            adapter = self._renewable_adapter_for(
                snapshot.credential_kind, snapshot.provider
            )
            now = self._refresh_clock() if self._refresh_clock is not None else time.time()
            resolved = adapter.current_secret(
                snapshot.state,
                driver_revision=snapshot.driver_revision,
                account_label=snapshot.account_label,
                now=now,
            )
            if resolved is None:
                if snapshot.state_generation >= (1 << 63) - 1:
                    raise BrokerError("state_generation_exhausted")
                task_digest = secrets.token_hex(32)
                self._persist_renewable_refresh_barrier(snapshot, task_digest)
                self._active_refresh_tasks[snapshot.lock_key] = task_digest
            else:
                resolved = self._validate_renewable_resolution(resolved)
                task_digest = ""

        if resolved is None:
            try:
                refreshed = adapter.refresh(
                    snapshot.state,
                    driver_revision=snapshot.driver_revision,
                    account_label=snapshot.account_label,
                    now=now,
                )
                if (
                    not isinstance(refreshed, RefreshedRenewableSession)
                    or not isinstance(refreshed.state, bytes)
                    or not refreshed.state
                    or len(refreshed.state) > MAX_SECRET_BYTES
                ):
                    raise BrokerError("credential_not_resolvable")
                resolved = self._validate_renewable_resolution(refreshed.resolved)
            except Exception:
                self._finish_active_refresh(snapshot, task_digest)
                raise BrokerError("companion_outcome_unknown") from None
            try:
                with self._mutex:
                    key = self._require_key()
                    payload = self._vaults.load(snapshot.context_id, key)
                    current = payload["providers"].get(provider)
                    if (
                        not self._renewable_credential_matches_snapshot(
                            current, snapshot
                        )
                        or current.get("refresh_task_digest") != task_digest
                    ):
                        raise BrokerError("handle_revoked")
                    updated_credential = dict(current)
                    updated_credential["state"] = encode_secret(refreshed.state)
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

        assert resolved is not None
        return self._resolved_secret_response(
            provider,
            revision,
            normalized_target,
            normalized_header,
            normalized_format,
            binding,
            resolved.secret,
            supplemental_headers=self._renewable_supplemental_headers(resolved),
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
                self._renewable_adapter_for(kind, provider)
                initial, _, _, _, _ = self._renewable_session_snapshot(
                    handle=handle,
                    context_id=context_id,
                    project_id=project_id,
                    provider=provider,
                    revision=revision,
                    target=target,
                    source_header=source_header,
                    source_format=source_format,
                )
                record_lock = self._record_lock(initial)

            if not record_lock.acquire(timeout=self._record_lock_timeout):
                raise BrokerError("companion_busy")
            try:
                return self._resolve_renewable_after_lock(
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
            signing_adapter = self._request_signing_adapters[
                AWS_SSO_CREDENTIAL_KIND
            ]
            normalized_request = signing_adapter.parse_request(request)
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
                    signing_adapter = self._request_signing_adapter_for(
                        AWS_SSO_CREDENTIAL_KIND, snapshot.provider
                    )
                    refresh_request = signing_adapter.create_refresh_request(snapshot)
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
                    completed = signing_adapter.complete(
                        normalized_request, refresh_request, result
                    )
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
                        updated_credential["state"] = encode_secret(completed.state)
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
                        "authorization": completed.headers.authorization,
                        "x_amz_date": completed.headers.amz_date,
                        "x_amz_security_token": completed.headers.security_token,
                        "x_amz_content_sha256": completed.headers.content_sha256,
                    },
                }
            finally:
                record_lock.release()
        except Exception as error:
            raise _translate_error(error) from None
