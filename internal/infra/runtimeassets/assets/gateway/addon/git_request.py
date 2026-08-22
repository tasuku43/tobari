"""Bounded Git Smart HTTP request identity."""

from __future__ import annotations

from dataclasses import dataclass
from urllib.parse import parse_qsl


class GitRequestError(ValueError):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass(frozen=True)
class ParsedGitRequest:
    service: str
    repository: str


_SERVICE_PATHS = {
    "/git-upload-pack": ("upload-pack", "application/x-git-upload-pack-request"),
    "/git-receive-pack": ("receive-pack", "application/x-git-receive-pack-request"),
}


def _header_values(headers: list[tuple[str, str]], name: str) -> list[str]:
    return [value.strip() for key, value in headers if key.lower() == name]


def _repository(path: str, suffix: str) -> str:
    repository = path[: -len(suffix)]
    if (
        not repository.startswith("/")
        or repository == "/"
        or len(repository.encode("utf-8")) > 1024
        or "//" in repository
        or "%" in repository
        or "\\" in repository
        or any(segment in {"", ".", ".."} for segment in repository.split("/")[1:])
        or any(
            ord(character) < 32 or ord(character) == 127 or character in {"\u2028", "\u2029"}
            for character in repository
        )
    ):
        raise GitRequestError("git_repository_invalid", "Git repository path is invalid")
    return repository


def classify_git_request(
    method: str,
    path: str,
    raw_query: str,
    headers: list[tuple[str, str]],
) -> ParsedGitRequest | None:
    """Classify exact Smart HTTP discovery/RPC paths; unrelated HTTP returns None."""

    if path.endswith("/info/refs"):
        repository = _repository(path, "/info/refs")
        if method.upper() != "GET":
            raise GitRequestError("git_method_invalid", "Git reference discovery requires GET")
        try:
            query = parse_qsl(
                raw_query, keep_blank_values=True, strict_parsing=True,
                encoding="utf-8", errors="strict", max_num_fields=2,
            )
        except (UnicodeError, ValueError) as error:
            raise GitRequestError("git_query_invalid", "Git service query is invalid") from error
        if len(query) != 1 or query[0][0] != "service" or query[0][1] not in {
            "git-upload-pack", "git-receive-pack"
        }:
            raise GitRequestError("git_service_invalid", "Git reference discovery service is invalid")
        if _header_values(headers, "transfer-encoding"):
            raise GitRequestError("git_discovery_body_invalid", "Git reference discovery cannot stream a request body")
        lengths = _header_values(headers, "content-length")
        if len(lengths) > 1 or (lengths and lengths[0] != "0"):
            raise GitRequestError("git_discovery_body_invalid", "Git reference discovery request must be body-free")
        return ParsedGitRequest(query[0][1].removeprefix("git-"), repository)

    for suffix, (service, media_type) in _SERVICE_PATHS.items():
        if not path.endswith(suffix):
            continue
        repository = _repository(path, suffix)
        if method.upper() != "POST" or raw_query:
            raise GitRequestError("git_rpc_invalid", "Git RPC transport is invalid")
        content_types = _header_values(headers, "content-type")
        if content_types != [media_type]:
            raise GitRequestError("git_content_type_invalid", "Git RPC content type is invalid")
        return ParsedGitRequest(service, repository)
    return None
