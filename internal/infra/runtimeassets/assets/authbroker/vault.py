"""Authenticated, Context-bound encrypted credential vault storage."""

from __future__ import annotations

import json
import os
import secrets
import stat
from pathlib import Path
from typing import Any

from cryptography.exceptions import InvalidTag
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from . import SCHEMA_VERSION
from .credential_records import (
    ANTHROPIC_CLAUDE_DRIVER_ID,
    ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND,
    AWS_CONSOLE_DRIVER_ID,
    AWS_DRIVER_ID,
    AWS_DRIVER_IDS,
    AWS_SSO_CREDENTIAL_KIND,
    CLAUDE_ACCOUNT_LABEL,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    MAX_DRIVER_STATE_BYTES,
    OPENAI_CODEX_DRIVER_ID,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    PAYLOAD_SCHEMA_VERSION,
    PUP_DRIVER_ID,
    STATIC_CREDENTIAL_KIND,
    VaultError,
    _b64decode,
    _b64encode,
    decode_secret,
    empty_payload,
    encode_secret,
    new_aws_sso_record,
    new_anthropic_claude_oauth_record,
    new_datadog_oauth_record,
    new_openai_codex_oauth_record,
    new_record,
    validate_context_id,
    validate_payload,
    validate_provider_id,
)


MAX_VAULT_BYTES = 1024 * 1024


def _owned_by_service(info: os.stat_result) -> bool:
    effective_uid = os.geteuid()
    # See daemon._owned_by_service. Host-side XDG validation remains
    # authoritative for bind sources; this accepts only the UID-zero view that
    # macOS Docker virtualization gives those already validated paths.
    return info.st_uid == effective_uid or (effective_uid != 0 and info.st_uid == 0)


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
            document,
            ensure_ascii=True,
            allow_nan=False,
            separators=(",", ":"),
            sort_keys=True,
        ).encode("utf-8")
    except (TypeError, ValueError):
        raise VaultError("vault_invalid") from None


def _associated_data(context_id: str) -> bytes:
    return b"tobari-auth-vault\x00schema=1\x00context=" + context_id.encode("ascii")


def _validate_key(key: bytes | bytearray) -> bytes:
    if not isinstance(key, (bytes, bytearray)) or len(key) != 32:
        raise VaultError("invalid_key")
    return bytes(key)


class VaultStore:
    """Own envelope cryptography and safe atomic filesystem persistence."""

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
        if (
            envelope.get("context_id") != context_id
            or envelope.get("algorithm") != "AES-256-GCM"
        ):
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
                existing = os.stat(
                    "vault.enc", dir_fd=directory_fd, follow_symlinks=False
                )
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
                temporary,
                "vault.enc",
                src_dir_fd=directory_fd,
                dst_dir_fd=directory_fd,
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
