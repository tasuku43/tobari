"""Fail-closed caching for small host-owned files replaced atomically."""

from __future__ import annotations

import os
import stat
import threading
from typing import Callable, Generic, TypeVar, cast


T = TypeVar("T")
_MISSING = object()


class ValidatedFileError(Exception):
    """A validated file could not provide one current trusted snapshot."""

    def __init__(self, code: str):
        super().__init__(code)
        self.code = code


FileIdentity = tuple[int, int, int, int, int, int, int, int]


def _identity(info: os.stat_result) -> FileIdentity:
    return (
        info.st_dev,
        info.st_ino,
        info.st_mode,
        info.st_uid,
        info.st_gid,
        info.st_size,
        info.st_mtime_ns,
        info.st_ctime_ns,
    )


class StatIdentityCache(Generic[T]):
    """Reuse one validated value while the exact path identity is unchanged.

    Cache hits perform only a non-following stat. A changed path is opened with
    O_NOFOLLOW, bounded, validated, and checked again before the new value is
    cached. Any failure clears the cache; an invalid replacement can therefore
    never fall back to an older value.
    """

    def __init__(
        self,
        path: str,
        maximum_bytes: int,
        validator: Callable[[bytes], T],
    ) -> None:
        self._path = path
        self._maximum_bytes = maximum_bytes
        self._validator = validator
        self._cached_identity: FileIdentity | None = None
        self._cached_value: object = _MISSING
        self._lock = threading.Lock()

    @staticmethod
    def _validate_info(info: os.stat_result) -> None:
        if (
            not stat.S_ISREG(info.st_mode)
            or info.st_uid != os.geteuid()
            or stat.S_IMODE(info.st_mode) != 0o600
        ):
            raise ValidatedFileError("file_invalid")

    def _validate_path(self) -> None:
        if (
            not isinstance(self._path, str)
            or not self._path
            or not os.path.isabs(self._path)
            or os.path.normpath(self._path) != self._path
        ):
            raise ValidatedFileError("path_invalid")
        if (
            isinstance(self._maximum_bytes, bool)
            or not isinstance(self._maximum_bytes, int)
            or self._maximum_bytes <= 0
        ):
            raise ValidatedFileError("size_invalid")

    def _stat_path(self) -> os.stat_result:
        try:
            return os.stat(self._path, follow_symlinks=False)
        except OSError as error:
            raise ValidatedFileError("unavailable") from error

    def _read_changed(self, expected: FileIdentity) -> T:
        flags = os.O_RDONLY
        if hasattr(os, "O_CLOEXEC"):
            flags |= os.O_CLOEXEC
        if hasattr(os, "O_NOFOLLOW"):
            flags |= os.O_NOFOLLOW
        try:
            descriptor = os.open(self._path, flags)
        except OSError as error:
            raise ValidatedFileError("unavailable") from error
        try:
            opened = os.fstat(descriptor)
            self._validate_info(opened)
            if _identity(opened) != expected:
                raise ValidatedFileError("changed")
            if opened.st_size <= 0 or opened.st_size > self._maximum_bytes:
                raise ValidatedFileError("size_invalid")
            chunks: list[bytes] = []
            remaining = self._maximum_bytes + 1
            while remaining:
                chunk = os.read(descriptor, min(65536, remaining))
                if not chunk:
                    break
                chunks.append(chunk)
                remaining -= len(chunk)
            after_read = os.fstat(descriptor)
            if _identity(after_read) != expected:
                raise ValidatedFileError("changed")
        finally:
            os.close(descriptor)
        raw = b"".join(chunks)
        if not raw or len(raw) > self._maximum_bytes:
            raise ValidatedFileError("size_invalid")
        value = self._validator(raw)
        current = self._stat_path()
        self._validate_info(current)
        if _identity(current) != expected:
            raise ValidatedFileError("changed")
        return value

    def load(self) -> T:
        with self._lock:
            try:
                self._validate_path()
                observed = self._stat_path()
                self._validate_info(observed)
                identity = _identity(observed)
                if (
                    self._cached_identity == identity
                    and self._cached_value is not _MISSING
                ):
                    return cast(T, self._cached_value)
                # Clear before validating a changed identity. A rejected update
                # must not retain a usable last-known-good value.
                self._cached_identity = None
                self._cached_value = _MISSING
                value = self._read_changed(identity)
                self._cached_identity = identity
                self._cached_value = value
                return value
            except Exception:
                self._cached_identity = None
                self._cached_value = _MISSING
                raise
