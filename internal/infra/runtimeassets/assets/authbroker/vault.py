"""Authenticated, Context-bound encrypted credential vault storage."""

from __future__ import annotations

import base64
import json
import os
import re
import secrets
import stat
from pathlib import Path
from typing import Any

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from . import SCHEMA_VERSION


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
AWS_ACCOUNT_LABEL_PATTERN = re.compile(r"^[0-9]{12}$")
DRIVER_REVISION_PATTERN = re.compile(r"^[0-9a-f]{64}$")
TASK_DIGEST_PATTERN = re.compile(r"^[0-9a-f]{64}$")
MAX_VAULT_BYTES = 1024 * 1024
MAX_DRIVER_STATE_BYTES = 32 * 1024
PAYLOAD_SCHEMA_VERSION = 2
LEGACY_PAYLOAD_SCHEMA_VERSION = 1
STATIC_CREDENTIAL_KIND = "static_primary_secret"
AWS_SSO_CREDENTIAL_KIND = "aws_sso_session"
AWS_DRIVER_ID = "aws_cli_sso"


class VaultError(Exception):
    """A stable, secret-free vault failure."""

    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _owned_by_service(info: os.stat_result) -> bool:
    effective_uid = os.geteuid()
    # See daemon._owned_by_service. Host-side XDG validation remains
    # authoritative for bind sources; this accepts only the UID-zero view that
    # macOS Docker virtualization gives those already validated paths.
    return info.st_uid == effective_uid or (effective_uid != 0 and info.st_uid == 0)


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


def _strict_json(payload: bytes) -> dict[str, Any]:
    def pairs(values: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in values:
            if key in result:
                raise VaultError("vault_invalid")
            result[key] = value
        return result

    def constant(_: str) -> None:
        raise VaultError("vault_invalid")

    try:
        value = json.loads(
            payload.decode("utf-8"), object_pairs_hook=pairs, parse_constant=constant
        )
    except VaultError:
        raise
    except (UnicodeDecodeError, json.JSONDecodeError):
        raise VaultError("vault_invalid") from None
    if not isinstance(value, dict):
        raise VaultError("vault_invalid")
    return value


def _canonical_json(document: dict[str, Any]) -> bytes:
    try:
        return json.dumps(
            document, ensure_ascii=True, allow_nan=False, separators=(",", ":"), sort_keys=True
        ).encode("utf-8")
    except (TypeError, ValueError):
        raise VaultError("vault_invalid") from None


def _associated_data(context_id: str) -> bytes:
    return b"tobari-auth-vault\x00schema=1\x00context=" + context_id.encode("ascii")


def _validate_key(key: bytes | bytearray) -> bytes:
    if not isinstance(key, (bytes, bytearray)) or len(key) != 32:
        raise VaultError("invalid_key")
    return bytes(key)


def empty_payload() -> dict[str, Any]:
    return {"schema_version": PAYLOAD_SCHEMA_VERSION, "providers": {}}


def _validate_stored_header_binding(binding: Any, provider: str) -> None:
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
        or destination.get("format") not in {
            "preserve_scheme",
            "raw",
            "bearer",
            "token",
        }
        or destination.get("secret_field") != "primary_secret"
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
    target = sigv4.get("target")
    source = sigv4.get("source")
    if target != {
        "scheme": "https",
        "port": 443,
        "dns_suffixes": ["amazonaws.com"],
    }:
        raise VaultError("vault_invalid")
    if source != {
        "authorization_header": "authorization",
        "security_token_header": "x-amz-security-token",
    }:
        raise VaultError("vault_invalid")
    if sigv4.get("secret_headers") != [
        "authorization",
        "x-amz-security-token",
    ]:
        raise VaultError("vault_invalid")


def _validate_stored_binding(binding: Any, provider: str) -> str:
    if isinstance(binding, dict) and binding.get("kind") == "aws_sigv4":
        _validate_stored_aws_binding(binding, provider)
        return AWS_SSO_CREDENTIAL_KIND
    _validate_stored_header_binding(binding, provider)
    return STATIC_CREDENTIAL_KIND


def _validate_common_record(record: Any) -> tuple[str | None, dict[str, Any]]:
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
        or len(account_label) > 64
        or not ACCOUNT_LABEL_PATTERN.fullmatch(account_label)
    ):
        raise VaultError("vault_invalid")
    handles = record.get("handles")
    if not isinstance(handles, dict) or len(handles) > 4096:
        raise VaultError("vault_invalid")
    return account_label, handles


def _validate_handles(
    provider: str, handles: dict[str, Any], credential_kind: str
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
        kinds = {_validate_stored_binding(binding, provider) for binding in bindings}
        if kinds != {credential_kind}:
            raise VaultError("vault_invalid")


def _validate_v2_record(provider: str, record: Any) -> None:
    if not isinstance(record, dict):
        raise VaultError("vault_invalid")
    credential_kind = record.get("credential_kind")
    if credential_kind == STATIC_CREDENTIAL_KIND:
        if set(record) != {
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
    elif credential_kind == AWS_SSO_CREDENTIAL_KIND:
        if provider != "aws" or set(record) != {
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
        state_generation = record.get("state_generation")
        refresh_task_digest = record.get("refresh_task_digest")
        if (
            not isinstance(account_label, str)
            or AWS_ACCOUNT_LABEL_PATTERN.fullmatch(account_label) is None
            or record.get("driver_id") != AWS_DRIVER_ID
            or not isinstance(record.get("driver_revision"), str)
            or DRIVER_REVISION_PATTERN.fullmatch(record["driver_revision"]) is None
            or isinstance(state_generation, bool)
            or not isinstance(state_generation, int)
            or state_generation < 0
            or state_generation > (1 << 63) - 1
            or (
                refresh_task_digest is not None
                and (
                    not isinstance(refresh_task_digest, str)
                    or TASK_DIGEST_PATTERN.fullmatch(refresh_task_digest) is None
                )
            )
        ):
            raise VaultError("vault_invalid")
        state = decode_secret(record.get("state"))
        # The state is canonicalized and interpreted only by the reviewed
        # trusted-host driver. The Broker deliberately owns it as opaque,
        # bounded encrypted bytes and never parses provider cache content.
        if not state or len(state) > MAX_DRIVER_STATE_BYTES:
            raise VaultError("vault_invalid") from None
    else:
        raise VaultError("vault_invalid")
    _validate_handles(provider, handles, credential_kind)


def _migrate_v1_payload(document: dict[str, Any]) -> dict[str, Any]:
    providers = document.get("providers")
    if not isinstance(providers, dict) or len(providers) > 64:
        raise VaultError("vault_invalid")
    migrated: dict[str, Any] = {
        "schema_version": PAYLOAD_SCHEMA_VERSION,
        "providers": {},
    }
    for provider, record in providers.items():
        validate_provider_id(provider)
        if not isinstance(record, dict) or set(record) != {
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
        _validate_handles(provider, handles, STATIC_CREDENTIAL_KIND)
        migrated["providers"][provider] = {
            "credential_kind": STATIC_CREDENTIAL_KIND,
            **record,
        }
    return migrated


def validate_payload(document: dict[str, Any]) -> dict[str, Any]:
    if set(document) != {"schema_version", "providers"}:
        raise VaultError("vault_invalid")
    version = document.get("schema_version")
    if isinstance(version, bool):
        raise VaultError("vault_version_unsupported")
    if version == LEGACY_PAYLOAD_SCHEMA_VERSION:
        return _migrate_v1_payload(document)
    if version != PAYLOAD_SCHEMA_VERSION:
        raise VaultError("vault_version_unsupported")
    providers = document.get("providers")
    if not isinstance(providers, dict) or len(providers) > 64:
        raise VaultError("vault_invalid")
    for provider, record in providers.items():
        validate_provider_id(provider)
        _validate_v2_record(provider, record)
    return document


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
        "record_id": "record_" + secrets.token_urlsafe(18),
        "revision": "revision_" + secrets.token_urlsafe(18),
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
    if (
        not isinstance(state, bytes)
        or not state
        or len(state) > MAX_DRIVER_STATE_BYTES
    ):
        raise VaultError("invalid_secret")
    if (
        not isinstance(account_label, str)
        or AWS_ACCOUNT_LABEL_PATTERN.fullmatch(account_label) is None
    ):
        raise VaultError("invalid_account_label")
    if driver_id != AWS_DRIVER_ID:
        raise VaultError("invalid_driver")
    if (
        not isinstance(driver_revision, str)
        or DRIVER_REVISION_PATTERN.fullmatch(driver_revision) is None
    ):
        raise VaultError("invalid_driver_revision")
    return {
        "credential_kind": AWS_SSO_CREDENTIAL_KIND,
        "record_id": "record_" + secrets.token_urlsafe(18),
        "revision": "revision_" + secrets.token_urlsafe(18),
        "account_label": account_label,
        "driver_id": driver_id,
        "driver_revision": driver_revision,
        "state_generation": 0,
        # A non-null digest is a durable no-replay barrier. It is persisted
        # before host provider execution and cleared only by the same
        # correlated successful refresh (or a proven pre-execution failure).
        "refresh_task_digest": None,
        "state": encode_secret(state),
        "handles": {},
    }


class VaultStore:
    def __init__(self, root: str | os.PathLike[str]):
        self.root = Path(root)

    @staticmethod
    def _validate_directory(path: Path, create: bool) -> None:
        if create and not path.exists():
            try:
                path.mkdir(mode=0o700)
            except FileExistsError:
                pass
            except OSError:
                raise VaultError("vault_path_invalid") from None
        try:
            info = path.lstat()
        except FileNotFoundError:
            if not create:
                raise VaultError("vault_not_found") from None
            raise VaultError("vault_path_invalid") from None
        except OSError:
            raise VaultError("vault_path_invalid") from None
        if (
            not stat.S_ISDIR(info.st_mode)
            or stat.S_ISLNK(info.st_mode)
            or not _owned_by_service(info)
            or stat.S_IMODE(info.st_mode) != 0o700
        ):
            raise VaultError("vault_path_invalid")

    def _context_directory(self, context_id: str, create: bool) -> Path:
        validate_context_id(context_id)
        self._validate_directory(self.root, create=create)
        directory = self.root / context_id
        self._validate_directory(directory, create=create)
        return directory

    @staticmethod
    def _read_regular_owner_file(path: Path) -> bytes:
        flags = os.O_RDONLY
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        try:
            descriptor = os.open(path, flags)
        except FileNotFoundError:
            raise VaultError("vault_not_found") from None
        except OSError:
            raise VaultError("vault_path_invalid") from None
        try:
            info = os.fstat(descriptor)
            if (
                not stat.S_ISREG(info.st_mode)
                or not _owned_by_service(info)
                or stat.S_IMODE(info.st_mode) != 0o600
                or info.st_size <= 0
                or info.st_size > MAX_VAULT_BYTES
            ):
                raise VaultError("vault_path_invalid")
            chunks: list[bytes] = []
            remaining = info.st_size
            while remaining:
                chunk = os.read(descriptor, min(65536, remaining))
                if not chunk:
                    raise VaultError("vault_invalid")
                chunks.append(chunk)
                remaining -= len(chunk)
            return b"".join(chunks)
        finally:
            os.close(descriptor)

    def exists(self, context_id: str) -> bool:
        try:
            directory = self._context_directory(context_id, create=False)
        except VaultError as error:
            if error.code == "vault_not_found":
                return False
            raise
        path = directory / "vault.enc"
        try:
            path.lstat()
        except FileNotFoundError:
            return False
        except OSError:
            raise VaultError("vault_path_invalid") from None
        self._read_regular_owner_file(path)
        return True

    def load(self, context_id: str, key: bytes | bytearray) -> dict[str, Any]:
        context_id = validate_context_id(context_id)
        key_bytes = _validate_key(key)
        directory = self._context_directory(context_id, create=False)
        envelope = _strict_json(self._read_regular_owner_file(directory / "vault.enc"))
        if set(envelope) != {
            "schema_version",
            "context_id",
            "algorithm",
            "nonce",
            "ciphertext",
        }:
            raise VaultError("vault_invalid")
        if envelope.get("schema_version") != SCHEMA_VERSION or isinstance(
            envelope.get("schema_version"), bool
        ):
            raise VaultError("vault_version_unsupported")
        if envelope.get("context_id") != context_id or envelope.get("algorithm") != "AES-256-GCM":
            raise VaultError("vault_invalid")
        nonce = _b64decode(envelope.get("nonce"))
        ciphertext = _b64decode(envelope.get("ciphertext"))
        if len(nonce) != 12 or len(ciphertext) < 16:
            raise VaultError("vault_invalid")
        try:
            plaintext = AESGCM(key_bytes).decrypt(
                nonce, ciphertext, _associated_data(context_id)
            )
        except InvalidTag:
            raise VaultError("vault_integrity_failed") from None
        if not plaintext or len(plaintext) > MAX_VAULT_BYTES:
            raise VaultError("vault_invalid")
        return validate_payload(_strict_json(plaintext))

    def save(
        self, context_id: str, key: bytes | bytearray, payload: dict[str, Any]
    ) -> None:
        context_id = validate_context_id(context_id)
        key_bytes = _validate_key(key)
        normalized_payload = validate_payload(payload)
        plaintext = _canonical_json(normalized_payload)
        nonce = secrets.token_bytes(12)
        ciphertext = AESGCM(key_bytes).encrypt(
            nonce, plaintext, _associated_data(context_id)
        )
        envelope = _canonical_json(
            {
                "schema_version": SCHEMA_VERSION,
                "context_id": context_id,
                "algorithm": "AES-256-GCM",
                "nonce": _b64encode(nonce),
                "ciphertext": _b64encode(ciphertext),
            }
        )
        if len(envelope) > MAX_VAULT_BYTES:
            raise VaultError("vault_too_large")
        directory = self._context_directory(context_id, create=True)
        self._atomic_replace(directory, envelope)

    @staticmethod
    def _atomic_replace(directory: Path, payload: bytes) -> None:
        directory_flags = os.O_RDONLY
        if hasattr(os, "O_DIRECTORY"):
            directory_flags |= os.O_DIRECTORY
        if hasattr(os, "O_NOFOLLOW"):
            directory_flags |= os.O_NOFOLLOW
        try:
            directory_fd = os.open(directory, directory_flags)
        except OSError:
            raise VaultError("vault_write_failed") from None
        temporary = ".vault.tmp-" + secrets.token_hex(12)
        backup = ".vault.backup-" + secrets.token_hex(12)
        wrote_replacement = False
        has_backup = False
        descriptor = -1
        try:
            flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
            if hasattr(os, "O_NOFOLLOW"):
                flags |= os.O_NOFOLLOW
            descriptor = os.open(temporary, flags, 0o600, dir_fd=directory_fd)
            os.fchmod(descriptor, 0o600)
            view = memoryview(payload)
            while view:
                written = os.write(descriptor, view)
                if written <= 0:
                    raise OSError("short write")
                view = view[written:]
            os.fsync(descriptor)
            os.close(descriptor)
            descriptor = -1

            try:
                existing = os.stat("vault.enc", dir_fd=directory_fd, follow_symlinks=False)
            except FileNotFoundError:
                existing = None
            if existing is not None:
                if (
                    not stat.S_ISREG(existing.st_mode)
                    or existing.st_uid != os.geteuid()
                    or stat.S_IMODE(existing.st_mode) != 0o600
                ):
                    raise VaultError("vault_path_invalid")
                os.link(
                    "vault.enc",
                    backup,
                    src_dir_fd=directory_fd,
                    dst_dir_fd=directory_fd,
                    follow_symlinks=False,
                )
                has_backup = True
            os.replace(
                temporary, "vault.enc", src_dir_fd=directory_fd, dst_dir_fd=directory_fd
            )
            wrote_replacement = True
            os.fsync(directory_fd)
        except VaultError:
            raise
        except OSError:
            if wrote_replacement and has_backup:
                try:
                    os.replace(
                        backup,
                        "vault.enc",
                        src_dir_fd=directory_fd,
                        dst_dir_fd=directory_fd,
                    )
                    has_backup = False
                    os.fsync(directory_fd)
                except OSError:
                    pass
            raise VaultError("vault_write_failed") from None
        finally:
            if descriptor >= 0:
                os.close(descriptor)
            for name in (temporary, backup if has_backup else ""):
                if not name:
                    continue
                try:
                    os.unlink(name, dir_fd=directory_fd)
                except FileNotFoundError:
                    pass
                except OSError:
                    pass
            os.close(directory_fd)
