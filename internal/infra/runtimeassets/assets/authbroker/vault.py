"""Encrypted static-credential vault for the V1 Auth Broker."""

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

MAX_VAULT_BYTES = 1024 * 1024
CONTEXT_ID_PATTERN = re.compile(
    r"^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$"
)
PROVIDER_ID_PATTERN = re.compile(r"^[a-z0-9]+(?:[._-][a-z0-9]+)*$")
REVISION_PATTERN = re.compile(r"^revision_[!-~]{1,119}$")
HANDLE_PATTERN = re.compile(r"^tobari-h1_[A-Za-z0-9_-]{43}$")
STATIC_CREDENTIAL_KIND = "primary_secret"


class VaultError(Exception):
    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


def _owned_by_service(info: os.stat_result) -> bool:
    owner = os.geteuid()
    return info.st_uid == owner or (owner != 0 and info.st_uid == 0)


def validate_context_id(value: Any) -> str:
    if not isinstance(value, str) or CONTEXT_ID_PATTERN.fullmatch(value) is None:
        raise VaultError("invalid_context")
    return value


def validate_provider_id(value: Any) -> str:
    if (
        not isinstance(value, str)
        or len(value) > 64
        or PROVIDER_ID_PATTERN.fullmatch(value) is None
    ):
        raise VaultError("invalid_provider")
    return value


def _b64encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).rstrip(b"=").decode("ascii")


def _b64decode(value: Any) -> bytes:
    if not isinstance(value, str) or not value:
        raise VaultError("vault_invalid")
    try:
        return base64.b64decode(
            value.encode("ascii") + b"=" * (-len(value) % 4),
            altchars=b"-_",
            validate=True,
        )
    except (UnicodeEncodeError, ValueError):
        raise VaultError("vault_invalid") from None


def encode_secret(value: bytes) -> str:
    if not isinstance(value, bytes) or not value:
        raise VaultError("vault_invalid")
    return _b64encode(value)


def decode_secret(value: Any) -> bytes:
    return _b64decode(value)


def _strict_json(payload: bytes) -> dict[str, Any]:
    def pairs(items: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate")
            result[key] = value
        return result

    try:
        value = json.loads(
            payload.decode("utf-8"),
            object_pairs_hook=pairs,
            parse_constant=lambda _: (_ for _ in ()).throw(ValueError("constant")),
        )
    except (UnicodeDecodeError, json.JSONDecodeError, ValueError):
        raise VaultError("vault_invalid") from None
    if not isinstance(value, dict):
        raise VaultError("vault_invalid")
    return value


def _canonical_json(document: dict[str, Any]) -> bytes:
    try:
        return json.dumps(
            document, ensure_ascii=True, allow_nan=False, separators=(",", ":"), sort_keys=True
        ).encode("ascii")
    except (TypeError, ValueError, UnicodeEncodeError):
        raise VaultError("vault_invalid") from None


def _associated_data(context_id: str) -> bytes:
    return f"tobari-auth-vault-v1:{context_id}".encode("ascii")


def _validate_key(key: bytes | bytearray) -> bytes:
    if not isinstance(key, (bytes, bytearray)) or len(key) != 32:
        raise VaultError("invalid_key")
    return bytes(key)


def empty_payload() -> dict[str, Any]:
    return {"schema_version": SCHEMA_VERSION, "providers": {}}


def _validate_binding(value: Any, provider: str) -> None:
    if not isinstance(value, dict) or set(value) != {
        "provider_id", "target", "source", "destination", "secret_headers"
    }:
        raise VaultError("vault_invalid")
    if value.get("provider_id") != provider:
        raise VaultError("vault_invalid")
    target = value.get("target")
    source = value.get("source")
    destination = value.get("destination")
    headers = value.get("secret_headers")
    if (
        not isinstance(target, dict)
        or set(target) != {"scheme", "host", "port"}
        or not isinstance(source, dict)
        or set(source) != {"header", "format"}
        or not isinstance(destination, dict)
        or set(destination) != {"header", "format", "secret_field"}
        or destination.get("secret_field") != STATIC_CREDENTIAL_KIND
        or not isinstance(headers, list)
        or not headers
        or len(headers) != len(set(headers))
    ):
        raise VaultError("vault_invalid")


def _validate_record(provider: str, value: Any) -> None:
    if not isinstance(value, dict) or set(value) != {
        "record_id", "revision", "credential_kind", "secret", "account_label", "handles"
    }:
        raise VaultError("vault_invalid")
    if value.get("credential_kind") != STATIC_CREDENTIAL_KIND:
        raise VaultError("vault_invalid")
    record_id = value.get("record_id")
    revision = value.get("revision")
    label = value.get("account_label")
    handles = value.get("handles")
    if (
        not isinstance(record_id, str)
        or not record_id.startswith("record_")
        or not isinstance(revision, str)
        or REVISION_PATTERN.fullmatch(revision) is None
        or (label is not None and (not isinstance(label, str) or not label or len(label) > 128))
        or not isinstance(handles, dict)
    ):
        raise VaultError("vault_invalid")
    decode_secret(value.get("secret"))
    for project, item in handles.items():
        if CONTEXT_ID_PATTERN.fullmatch(project) is None or not isinstance(item, dict) or set(item) != {"handle", "bindings"}:
            raise VaultError("vault_invalid")
        if not isinstance(item.get("handle"), str) or HANDLE_PATTERN.fullmatch(item["handle"]) is None:
            raise VaultError("vault_invalid")
        bindings = item.get("bindings")
        if not isinstance(bindings, list) or not bindings:
            raise VaultError("vault_invalid")
        for binding in bindings:
            _validate_binding(binding, provider)


def validate_payload(document: dict[str, Any]) -> dict[str, Any]:
    if set(document) != {"schema_version", "providers"}:
        raise VaultError("vault_invalid")
    if document.get("schema_version") != SCHEMA_VERSION:
        raise VaultError("vault_version_unsupported")
    providers = document.get("providers")
    if not isinstance(providers, dict):
        raise VaultError("vault_invalid")
    for provider, record in providers.items():
        validate_provider_id(provider)
        _validate_record(provider, record)
    return document


def new_record(secret: bytes, account_label: str | None = None) -> dict[str, Any]:
    if not isinstance(secret, bytes) or not secret:
        raise VaultError("invalid_secret")
    if account_label is not None and (
        not isinstance(account_label, str) or not account_label or len(account_label) > 128
    ):
        raise VaultError("invalid_account_label")
    return {
        "record_id": "record_" + secrets.token_urlsafe(24),
        "revision": "revision_" + secrets.token_urlsafe(24),
        "credential_kind": STATIC_CREDENTIAL_KIND,
        "secret": encode_secret(secret),
        "account_label": account_label,
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
            except (FileExistsError, OSError):
                if not path.exists():
                    raise VaultError("vault_path_invalid") from None
        try:
            info = path.lstat()
        except FileNotFoundError:
            raise VaultError("vault_not_found" if not create else "vault_path_invalid") from None
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
        self._validate_directory(self.root, create)
        directory = self.root / context_id
        self._validate_directory(directory, create)
        return directory

    @staticmethod
    def _read_regular_owner_file(path: Path) -> bytes:
        flags = os.O_RDONLY | getattr(os, "O_NOFOLLOW", 0)
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
            data = b""
            while len(data) < info.st_size:
                chunk = os.read(descriptor, min(65536, info.st_size - len(data)))
                if not chunk:
                    raise VaultError("vault_invalid")
                data += chunk
            return data
        finally:
            os.close(descriptor)

    def load(self, context_id: str, key: bytes | bytearray) -> dict[str, Any]:
        context_id = validate_context_id(context_id)
        envelope = _strict_json(self._read_regular_owner_file(self._context_directory(context_id, False) / "vault.enc"))
        if set(envelope) != {"schema_version", "context_id", "algorithm", "nonce", "ciphertext"}:
            raise VaultError("vault_invalid")
        if envelope.get("schema_version") != SCHEMA_VERSION:
            raise VaultError("vault_version_unsupported")
        if envelope.get("context_id") != context_id or envelope.get("algorithm") != "AES-256-GCM":
            raise VaultError("vault_invalid")
        nonce = _b64decode(envelope.get("nonce"))
        ciphertext = _b64decode(envelope.get("ciphertext"))
        if len(nonce) != 12 or len(ciphertext) < 16:
            raise VaultError("vault_invalid")
        try:
            plaintext = AESGCM(_validate_key(key)).decrypt(nonce, ciphertext, _associated_data(context_id))
        except InvalidTag:
            raise VaultError("vault_integrity_failed") from None
        return validate_payload(_strict_json(plaintext))

    def save(self, context_id: str, key: bytes | bytearray, payload: dict[str, Any]) -> None:
        context_id = validate_context_id(context_id)
        plaintext = _canonical_json(validate_payload(payload))
        nonce = secrets.token_bytes(12)
        envelope = _canonical_json({
            "schema_version": SCHEMA_VERSION,
            "context_id": context_id,
            "algorithm": "AES-256-GCM",
            "nonce": _b64encode(nonce),
            "ciphertext": _b64encode(AESGCM(_validate_key(key)).encrypt(nonce, plaintext, _associated_data(context_id))),
        })
        if len(envelope) > MAX_VAULT_BYTES:
            raise VaultError("vault_too_large")
        self._atomic_replace(self._context_directory(context_id, True), envelope)

    @staticmethod
    def _atomic_replace(directory: Path, payload: bytes) -> None:
        directory_fd = os.open(directory, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0) | getattr(os, "O_NOFOLLOW", 0))
        temporary = ".vault.tmp-" + secrets.token_hex(12)
        backup = ".vault.backup-" + secrets.token_hex(12)
        descriptor = -1
        has_backup = False
        replaced = False
        try:
            descriptor = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0), 0o600, dir_fd=directory_fd)
            os.write(descriptor, payload)
            os.fsync(descriptor)
            os.close(descriptor)
            descriptor = -1
            try:
                existing = os.stat("vault.enc", dir_fd=directory_fd, follow_symlinks=False)
            except FileNotFoundError:
                existing = None
            if existing is not None:
                if not stat.S_ISREG(existing.st_mode) or not _owned_by_service(existing) or stat.S_IMODE(existing.st_mode) != 0o600:
                    raise VaultError("vault_path_invalid")
                os.link("vault.enc", backup, src_dir_fd=directory_fd, dst_dir_fd=directory_fd, follow_symlinks=False)
                has_backup = True
            os.replace(temporary, "vault.enc", src_dir_fd=directory_fd, dst_dir_fd=directory_fd)
            replaced = True
            os.fsync(directory_fd)
        except VaultError:
            raise
        except OSError:
            if replaced and has_backup:
                try:
                    os.replace(backup, "vault.enc", src_dir_fd=directory_fd, dst_dir_fd=directory_fd)
                    has_backup = False
                    os.fsync(directory_fd)
                except OSError:
                    pass
            raise VaultError("vault_write_failed") from None
        finally:
            if descriptor >= 0:
                os.close(descriptor)
            for name in (temporary, backup if has_backup else ""):
                if name:
                    try:
                        os.unlink(name, dir_fd=directory_fd)
                    except OSError:
                        pass
            os.close(directory_fd)
