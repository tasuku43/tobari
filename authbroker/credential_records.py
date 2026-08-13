"""Closed persisted credential-record contracts for the Auth Broker."""

from __future__ import annotations

import base64
import re
import secrets
from types import MappingProxyType
from typing import Any, Mapping, Protocol


CONTEXT_ID_PATTERN = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)
PROVIDER_ID_PATTERN = re.compile(r"^[a-z0-9]+(?:[._-][a-z0-9]+)*$")
HANDLE_PATTERN = re.compile(r"^tobari-h1_[A-Za-z0-9_-]{43}$")
HOST_PATTERN = re.compile(
    r"^(?=.{1,253}$)(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$"
)
HEADER_PATTERN = re.compile(r"^[!#$%&'*+.^_`|~0-9a-z-]{1,64}$")
ACCOUNT_LABEL_PATTERN = re.compile(
    r"^[A-Za-z0-9](?:[A-Za-z0-9-]{0,62}[A-Za-z0-9])?$"
)
OPENAI_ACCOUNT_LABEL_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")
AWS_ACCOUNT_LABEL_PATTERN = re.compile(r"^[0-9]{12}$")
DRIVER_REVISION_PATTERN = re.compile(r"^[0-9a-f]{64}$")
TASK_DIGEST_PATTERN = re.compile(r"^[0-9a-f]{64}$")
MAX_DRIVER_STATE_BYTES = 32 * 1024
PAYLOAD_SCHEMA_VERSION = 1
STATIC_CREDENTIAL_KIND = "static_primary_secret"
AWS_SSO_CREDENTIAL_KIND = "aws_sso_session"
DATADOG_OAUTH_CREDENTIAL_KIND = "datadog_oauth_session"
OPENAI_CODEX_OAUTH_CREDENTIAL_KIND = "openai_codex_oauth_session"
AWS_DRIVER_ID = "aws_cli_sso"
AWS_CONSOLE_DRIVER_ID = "aws_cli_console_login"
AWS_DRIVER_IDS = frozenset({AWS_DRIVER_ID, AWS_CONSOLE_DRIVER_ID})
PUP_DRIVER_ID = "datadog_pup_oauth"
OPENAI_CODEX_DRIVER_ID = "openai_codex_chatgpt_oauth"
CLAUDE_ACCOUNT_LABEL = "claude-user-inference"


class VaultError(Exception):
    """A stable, secret-free credential record or Vault failure."""

    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def validate_context_id(context_id: Any) -> str:
    if not isinstance(context_id, str) or not CONTEXT_ID_PATTERN.fullmatch(context_id):
        raise VaultError("invalid_context")
    return context_id


def validate_provider_id(provider: Any) -> str:
    if (
        not isinstance(provider, str)
        or len(provider) > 64
        or not PROVIDER_ID_PATTERN.fullmatch(provider)
    ):
        raise VaultError("invalid_provider")
    return provider


def _b64encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _b64decode(value: Any) -> bytes:
    if not isinstance(value, str) or not value or "=" in value:
        raise VaultError("vault_invalid")
    try:
        encoded = value.encode("ascii")
        decoded = base64.b64decode(
            encoded + b"=" * (-len(encoded) % 4), altchars=b"-_", validate=True
        )
    except (UnicodeEncodeError, ValueError):
        raise VaultError("vault_invalid") from None
    if _b64encode(decoded) != value:
        raise VaultError("vault_invalid")
    return decoded


def encode_secret(value: bytes) -> str:
    return _b64encode(value)


def decode_secret(value: Any) -> bytes:
    return _b64decode(value)


def empty_payload() -> dict[str, Any]:
    return {"schema_version": PAYLOAD_SCHEMA_VERSION, "providers": {}}


def _validate_stored_header_binding(binding: Any, provider: str) -> str:
    if not isinstance(binding, dict) or set(binding) != {
        "provider_id",
        "target",
        "source",
        "destination",
        "secret_headers",
    }:
        raise VaultError("vault_invalid")
    if binding.get("provider_id") != provider:
        raise VaultError("vault_invalid")
    target = binding.get("target")
    source = binding.get("source")
    destination = binding.get("destination")
    secret_headers = binding.get("secret_headers")
    if not isinstance(target, dict) or set(target) != {"scheme", "host", "port"}:
        raise VaultError("vault_invalid")
    host = target.get("host")
    port = target.get("port")
    if (
        target.get("scheme") != "https"
        or not isinstance(host, str)
        or host != host.lower()
        or not HOST_PATTERN.fullmatch(host)
        or "." not in host
        or isinstance(port, bool)
        or not isinstance(port, int)
        or port < 1
        or port > 65535
    ):
        raise VaultError("vault_invalid")
    if not isinstance(source, dict) or set(source) != {"header", "format"}:
        raise VaultError("vault_invalid")
    if not isinstance(destination, dict) or set(destination) != {
        "header",
        "format",
        "secret_field",
    }:
        raise VaultError("vault_invalid")
    source_header = source.get("header")
    destination_header = destination.get("header")
    forbidden_headers = {
        "host",
        "content-length",
        "proxy-authorization",
        "cookie",
        "set-cookie",
    }
    if (
        not isinstance(source_header, str)
        or source_header != source_header.lower()
        or not HEADER_PATTERN.fullmatch(source_header)
        or source_header in forbidden_headers
        or source_header.startswith("x-tobari-")
        or source.get("format") not in {"raw", "bearer", "token"}
        or not isinstance(destination_header, str)
        or destination_header != destination_header.lower()
        or not HEADER_PATTERN.fullmatch(destination_header)
        or destination_header in forbidden_headers
        or destination_header.startswith("x-tobari-")
        or destination.get("format")
        not in {"preserve_scheme", "raw", "bearer", "token"}
        or destination.get("secret_field")
        not in {
            "primary_secret",
            DATADOG_OAUTH_CREDENTIAL_KIND,
            OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
        }
    ):
        raise VaultError("vault_invalid")
    if (
        not isinstance(secret_headers, list)
        or not secret_headers
        or len(secret_headers) > 32
        or any(
            not isinstance(header, str)
            or header != header.lower()
            or not HEADER_PATTERN.fullmatch(header)
            or header in forbidden_headers
            or header.startswith("x-tobari-")
            for header in secret_headers
        )
        or len(set(secret_headers)) != len(secret_headers)
        or source_header not in secret_headers
        or destination_header not in secret_headers
    ):
        raise VaultError("vault_invalid")
    return destination["secret_field"]


def _validate_stored_aws_binding(binding: Any, provider: str) -> None:
    if not isinstance(binding, dict) or set(binding) != {
        "provider_id",
        "kind",
        "aws_sigv4",
    }:
        raise VaultError("vault_invalid")
    if provider != "aws" or binding.get("provider_id") != provider:
        raise VaultError("vault_invalid")
    if binding.get("kind") != "aws_sigv4":
        raise VaultError("vault_invalid")
    sigv4 = binding.get("aws_sigv4")
    if not isinstance(sigv4, dict) or set(sigv4) != {
        "target",
        "source",
        "secret_headers",
    }:
        raise VaultError("vault_invalid")
    if sigv4.get("target") != {
        "scheme": "https",
        "port": 443,
        "dns_suffixes": ["amazonaws.com"],
    }:
        raise VaultError("vault_invalid")
    if sigv4.get("source") != {
        "authorization_header": "authorization",
        "security_token_header": "x-amz-security-token",
    }:
        raise VaultError("vault_invalid")
    if sigv4.get("secret_headers") != [
        "authorization",
        "x-amz-security-token",
    ]:
        raise VaultError("vault_invalid")


def _validate_common_record(
    record: Any,
    *,
    account_pattern: re.Pattern[str] = ACCOUNT_LABEL_PATTERN,
    max_account_label: int = 64,
) -> tuple[str | None, dict[str, Any]]:
    if not isinstance(record, dict):
        raise VaultError("vault_invalid")
    for field in ("record_id", "revision"):
        value = record.get(field)
        if (
            not isinstance(value, str)
            or not value
            or len(value) > 128
            or any(ord(character) < 0x21 or ord(character) > 0x7E for character in value)
        ):
            raise VaultError("vault_invalid")
    account_label = record.get("account_label")
    if account_label is not None and (
        not isinstance(account_label, str)
        or not account_label
        or len(account_label) > max_account_label
        or not account_pattern.fullmatch(account_label)
    ):
        raise VaultError("vault_invalid")
    handles = record.get("handles")
    if not isinstance(handles, dict) or len(handles) > 4096:
        raise VaultError("vault_invalid")
    return account_label, handles


def _validate_refresh_fields(record: dict[str, Any]) -> None:
    generation = record.get("state_generation")
    barrier = record.get("refresh_task_digest")
    if (
        isinstance(generation, bool)
        or not isinstance(generation, int)
        or generation < 0
        or generation > (1 << 63) - 1
        or (
            barrier is not None
            and (
                not isinstance(barrier, str)
                or TASK_DIGEST_PATTERN.fullmatch(barrier) is None
            )
        )
    ):
        raise VaultError("vault_invalid")


def _validate_driver_revision(record: dict[str, Any]) -> None:
    revision = record.get("driver_revision")
    if not isinstance(revision, str) or DRIVER_REVISION_PATTERN.fullmatch(revision) is None:
        raise VaultError("vault_invalid")


def _validate_state(record: dict[str, Any]) -> None:
    state = decode_secret(record.get("state"))
    if not state or len(state) > MAX_DRIVER_STATE_BYTES:
        raise VaultError("vault_invalid") from None


class CredentialRecordContract(Protocol):
    """One reviewed record shape without Vault or filesystem authority."""

    credential_kind: str
    provider_id: str | None

    def validate_record(self, provider: str, record: Any) -> dict[str, Any]: ...

    def validate_binding(self, provider: str, binding: Any) -> None: ...


class StaticCredentialRecordContract:
    credential_kind = STATIC_CREDENTIAL_KIND
    provider_id = None

    def validate_record(self, provider: str, record: Any) -> dict[str, Any]:
        del provider
        if not isinstance(record, dict) or set(record) != {
            "credential_kind",
            "record_id",
            "revision",
            "account_label",
            "secret",
            "handles",
        }:
            raise VaultError("vault_invalid")
        _, handles = _validate_common_record(record)
        secret = decode_secret(record.get("secret"))
        if not secret or len(secret) > 32 * 1024:
            raise VaultError("vault_invalid")
        return handles

    def validate_binding(self, provider: str, binding: Any) -> None:
        if _validate_stored_header_binding(binding, provider) != "primary_secret":
            raise VaultError("vault_invalid")


class AWSSSORecordContract:
    credential_kind = AWS_SSO_CREDENTIAL_KIND
    provider_id = "aws"

    def validate_record(self, provider: str, record: Any) -> dict[str, Any]:
        if provider != self.provider_id or not isinstance(record, dict) or set(record) != {
            "credential_kind",
            "record_id",
            "revision",
            "account_label",
            "driver_id",
            "driver_revision",
            "state_generation",
            "refresh_task_digest",
            "state",
            "handles",
        }:
            raise VaultError("vault_invalid")
        account_label, handles = _validate_common_record(record)
        if (
            not isinstance(account_label, str)
            or AWS_ACCOUNT_LABEL_PATTERN.fullmatch(account_label) is None
            or record.get("driver_id") not in AWS_DRIVER_IDS
        ):
            raise VaultError("vault_invalid")
        _validate_driver_revision(record)
        _validate_refresh_fields(record)
        _validate_state(record)
        return handles

    def validate_binding(self, provider: str, binding: Any) -> None:
        _validate_stored_aws_binding(binding, provider)


class DatadogOAuthRecordContract:
    credential_kind = DATADOG_OAUTH_CREDENTIAL_KIND
    provider_id = "datadog"

    def validate_record(self, provider: str, record: Any) -> dict[str, Any]:
        if provider != self.provider_id or not isinstance(record, dict) or set(record) != {
            "credential_kind",
            "record_id",
            "revision",
            "account_label",
            "driver_id",
            "driver_revision",
            "state_generation",
            "refresh_task_digest",
            "state",
            "handles",
        }:
            raise VaultError("vault_invalid")
        account_label, handles = _validate_common_record(record)
        if account_label != "datadog-us1" or record.get("driver_id") != PUP_DRIVER_ID:
            raise VaultError("vault_invalid")
        _validate_driver_revision(record)
        _validate_refresh_fields(record)
        _validate_state(record)
        return handles

    def validate_binding(self, provider: str, binding: Any) -> None:
        _validate_stored_header_binding(binding, provider)
        if binding != {
            "provider_id": "datadog",
            "target": {
                "scheme": "https",
                "host": "api.datadoghq.com",
                "port": 443,
            },
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {
                "header": "authorization",
                "format": "bearer",
                "secret_field": DATADOG_OAUTH_CREDENTIAL_KIND,
            },
            "secret_headers": ["authorization"],
        }:
            raise VaultError("vault_invalid")


class OpenAICodexOAuthRecordContract:
    credential_kind = OPENAI_CODEX_OAUTH_CREDENTIAL_KIND
    provider_id = "openai"

    def validate_record(self, provider: str, record: Any) -> dict[str, Any]:
        if provider != self.provider_id or not isinstance(record, dict) or set(record) != {
            "credential_kind",
            "record_id",
            "revision",
            "account_label",
            "driver_id",
            "driver_revision",
            "state_generation",
            "refresh_task_digest",
            "state",
            "handles",
        }:
            raise VaultError("vault_invalid")
        account_label, handles = _validate_common_record(
            record,
            account_pattern=OPENAI_ACCOUNT_LABEL_PATTERN,
            max_account_label=128,
        )
        if (
            not isinstance(account_label, str)
            or OPENAI_ACCOUNT_LABEL_PATTERN.fullmatch(account_label) is None
            or record.get("driver_id") != OPENAI_CODEX_DRIVER_ID
        ):
            raise VaultError("vault_invalid")
        _validate_driver_revision(record)
        _validate_refresh_fields(record)
        _validate_state(record)
        return handles

    def validate_binding(self, provider: str, binding: Any) -> None:
        _validate_stored_header_binding(binding, provider)
        if binding != {
            "provider_id": "openai",
            "target": {"scheme": "https", "host": "chatgpt.com", "port": 443},
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {
                "header": "authorization",
                "format": "bearer",
                "secret_field": OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
            },
            "secret_headers": [
                "authorization",
                "chatgpt-account-id",
                "x-openai-fedramp",
            ],
        }:
            raise VaultError("vault_invalid")


def reviewed_credential_record_contracts() -> Mapping[str, CredentialRecordContract]:
    """Return the immutable compiled record union; there is no registration API."""

    contracts: tuple[CredentialRecordContract, ...] = (
        StaticCredentialRecordContract(),
        AWSSSORecordContract(),
        DatadogOAuthRecordContract(),
        OpenAICodexOAuthRecordContract(),
    )
    registry = {contract.credential_kind: contract for contract in contracts}
    if len(registry) != len(contracts):
        raise RuntimeError("reviewed credential-record registry duplicates a kind")
    return MappingProxyType(registry)


REVIEWED_CREDENTIAL_RECORD_KINDS = frozenset(
    {
        STATIC_CREDENTIAL_KIND,
        AWS_SSO_CREDENTIAL_KIND,
        DATADOG_OAUTH_CREDENTIAL_KIND,
        OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    }
)
_REVIEWED_CREDENTIAL_RECORD_CONTRACTS = reviewed_credential_record_contracts()
if set(_REVIEWED_CREDENTIAL_RECORD_CONTRACTS) != set(REVIEWED_CREDENTIAL_RECORD_KINDS):
    raise RuntimeError("reviewed credential-record registry is inconsistent")


def _validate_handles(
    provider: str,
    handles: dict[str, Any],
    contract: CredentialRecordContract,
) -> None:
    for project_id, handle_record in handles.items():
        validate_context_id(project_id)
        if not isinstance(handle_record, dict) or set(handle_record) != {
            "handle",
            "bindings",
        }:
            raise VaultError("vault_invalid")
        handle = handle_record.get("handle")
        if not isinstance(handle, str) or not HANDLE_PATTERN.fullmatch(handle):
            raise VaultError("vault_invalid")
        encoded_handle = handle.removeprefix("tobari-h1_")
        try:
            if len(_b64decode(encoded_handle)) != 32:
                raise VaultError("vault_invalid")
        except VaultError:
            raise VaultError("vault_invalid") from None
        bindings = handle_record.get("bindings")
        if not isinstance(bindings, list) or not bindings or len(bindings) > 64:
            raise VaultError("vault_invalid")
        for binding in bindings:
            contract.validate_binding(provider, binding)


def validate_payload(document: dict[str, Any]) -> dict[str, Any]:
    if set(document) != {"schema_version", "providers"}:
        raise VaultError("vault_invalid")
    version = document.get("schema_version")
    if isinstance(version, bool) or version != PAYLOAD_SCHEMA_VERSION:
        raise VaultError("vault_version_unsupported")
    providers = document.get("providers")
    if not isinstance(providers, dict) or len(providers) > 64:
        raise VaultError("vault_invalid")
    for provider, record in providers.items():
        validate_provider_id(provider)
        credential_kind = record.get("credential_kind") if isinstance(record, dict) else None
        contract = _REVIEWED_CREDENTIAL_RECORD_CONTRACTS.get(credential_kind)
        if contract is None:
            raise VaultError("vault_invalid")
        handles = contract.validate_record(provider, record)
        _validate_handles(provider, handles, contract)
    return document


def _new_identity(prefix: str) -> str:
    return prefix + secrets.token_urlsafe(18)


def new_record(secret: bytes, account_label: str | None = None) -> dict[str, Any]:
    if not isinstance(secret, bytes) or not secret or len(secret) > 32 * 1024:
        raise VaultError("invalid_secret")
    if account_label is not None and (
        not isinstance(account_label, str)
        or not account_label
        or len(account_label) > 64
        or not ACCOUNT_LABEL_PATTERN.fullmatch(account_label)
    ):
        raise VaultError("invalid_account_label")
    return {
        "credential_kind": STATIC_CREDENTIAL_KIND,
        "record_id": _new_identity("record_"),
        "revision": _new_identity("revision_"),
        "account_label": account_label,
        "secret": encode_secret(secret),
        "handles": {},
    }


def new_aws_sso_record(
    state: bytes,
    *,
    account_label: str,
    driver_id: str,
    driver_revision: str,
) -> dict[str, Any]:
    if not isinstance(state, bytes) or not state or len(state) > MAX_DRIVER_STATE_BYTES:
        raise VaultError("invalid_secret")
    if (
        not isinstance(account_label, str)
        or AWS_ACCOUNT_LABEL_PATTERN.fullmatch(account_label) is None
    ):
        raise VaultError("invalid_account_label")
    if driver_id not in AWS_DRIVER_IDS:
        raise VaultError("invalid_driver")
    if (
        not isinstance(driver_revision, str)
        or DRIVER_REVISION_PATTERN.fullmatch(driver_revision) is None
    ):
        raise VaultError("invalid_driver_revision")
    return {
        "credential_kind": AWS_SSO_CREDENTIAL_KIND,
        "record_id": _new_identity("record_"),
        "revision": _new_identity("revision_"),
        "account_label": account_label,
        "driver_id": driver_id,
        "driver_revision": driver_revision,
        "state_generation": 0,
        # Persisted before provider execution; cleared only by the same task.
        "refresh_task_digest": None,
        "state": encode_secret(state),
        "handles": {},
    }


def new_datadog_oauth_record(
    state: bytes, *, account_label: str, driver_id: str, driver_revision: str
) -> dict[str, Any]:
    if not isinstance(state, bytes) or not state or len(state) > MAX_DRIVER_STATE_BYTES:
        raise VaultError("invalid_secret")
    if account_label != "datadog-us1" or driver_id != PUP_DRIVER_ID:
        raise VaultError("invalid_driver")
    if (
        not isinstance(driver_revision, str)
        or DRIVER_REVISION_PATTERN.fullmatch(driver_revision) is None
    ):
        raise VaultError("invalid_driver_revision")
    return {
        "credential_kind": DATADOG_OAUTH_CREDENTIAL_KIND,
        "record_id": _new_identity("record_"),
        "revision": _new_identity("revision_"),
        "account_label": account_label,
        "driver_id": driver_id,
        "driver_revision": driver_revision,
        "state_generation": 0,
        "refresh_task_digest": None,
        "state": encode_secret(state),
        "handles": {},
    }


def new_openai_codex_oauth_record(
    state: bytes, *, account_label: str, driver_id: str, driver_revision: str
) -> dict[str, Any]:
    if not isinstance(state, bytes) or not state or len(state) > MAX_DRIVER_STATE_BYTES:
        raise VaultError("invalid_secret")
    if (
        not isinstance(account_label, str)
        or OPENAI_ACCOUNT_LABEL_PATTERN.fullmatch(account_label) is None
        or driver_id != OPENAI_CODEX_DRIVER_ID
    ):
        raise VaultError("invalid_driver")
    if (
        not isinstance(driver_revision, str)
        or DRIVER_REVISION_PATTERN.fullmatch(driver_revision) is None
    ):
        raise VaultError("invalid_driver_revision")
    return {
        "credential_kind": OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
        "record_id": _new_identity("record_"),
        "revision": _new_identity("revision_"),
        "account_label": account_label,
        "driver_id": driver_id,
        "driver_revision": driver_revision,
        "state_generation": 0,
        "refresh_task_digest": None,
        "state": encode_secret(state),
        "handles": {},
    }
