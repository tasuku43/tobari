"""Bounded OCI Distribution request identity derived from standard routes."""

from __future__ import annotations

from dataclasses import dataclass
from urllib.parse import parse_qsl, quote, unquote


class OCIRequestError(ValueError):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


@dataclass(frozen=True)
class ParsedOCIRequest:
    action: str
    repository: str
    object: str

    def __post_init__(self) -> None:
        self.validate()

    def validate(self) -> None:
        if not isinstance(self.action, str) or self.action not in {
            "list", "pull", "push", "delete", "start_upload", "upload_status",
            "upload_chunk", "complete_upload", "mount", "cancel_upload",
        }:
            raise OCIRequestError("oci_projection_invalid", "OCI action is invalid")
        if not _valid_repository(self.repository, allow_empty=True) or not _valid_coordinate(self.object, allow_empty=False):
            raise OCIRequestError("oci_projection_invalid", "OCI coordinate is invalid")
        valid = False
        if self.action == "list":
            valid = (self.repository == "" and self.object == "catalog") or (self.repository != "" and self.object == "tags")
        elif self.action == "pull":
            valid = self.repository != "" and _valid_path_object(self.object, ("manifest:", "blob:", "referrers:"))
        elif self.action == "push":
            valid = self.repository != "" and _valid_path_object(self.object, ("manifest:",))
        elif self.action == "delete":
            valid = self.repository != "" and _valid_path_object(self.object, ("manifest:", "blob:"))
        elif self.action == "start_upload":
            valid = self.repository != "" and self.object == "upload"
        elif self.action in {"upload_status", "upload_chunk", "cancel_upload"}:
            valid = self.repository != "" and _valid_path_object(self.object, ("upload:",))
        elif self.action == "complete_upload":
            if self.object.startswith("blob:"):
                valid = self.repository != "" and _valid_quoted_token(self.object.removeprefix("blob:"))
            elif self.object.startswith("upload:") and ":blob:" in self.object:
                session, digest = self.object.removeprefix("upload:").split(":blob:", 1)
                valid = self.repository != "" and _valid_quoted_token(session) and _valid_quoted_token(digest)
        elif self.action == "mount":
            parts = self.object.removeprefix("mount:").split(":from:")
            if self.repository != "" and self.object.startswith("mount:") and len(parts) == 2:
                digest, source = parts
                valid = (
                    _valid_quoted_token(digest)
                    and _valid_quoted_token(source)
                    and _valid_repository(_decode_quoted_token(source), allow_empty=False)
                )
        if not valid or self.object.endswith(":"):
            raise OCIRequestError("oci_projection_invalid", "OCI action coordinate is invalid")


def _segment(value: str) -> str:
    if (
        not value
        or len(value.encode("utf-8")) > 512
        or value in {".", ".."}
        or "%" in value
        or "/" in value
        or "\\" in value
        or any(ord(character) < 32 or ord(character) == 127 or character in {"\u2028", "\u2029"} for character in value)
    ):
        raise OCIRequestError("oci_path_invalid", "OCI Distribution path is invalid")
    return value


def _valid_coordinate(value: object, allow_empty: bool) -> bool:
    if not isinstance(value, str) or (not allow_empty and not value):
        return False
    try:
        return len(value.encode("utf-8")) <= 1024 and not any(
            ord(character) < 32 or ord(character) == 127 or character in {"\u2028", "\u2029"}
            for character in value
        )
    except UnicodeEncodeError:
        return False


def _valid_repository(value: object, allow_empty: bool) -> bool:
    if value == "":
        return allow_empty
    if not _valid_coordinate(value, allow_empty=False):
        return False
    try:
        return all(_segment(segment) == segment for segment in value.split("/"))
    except OCIRequestError:
        return False


def _valid_quoted_token(value: str) -> bool:
    if not isinstance(value, str) or not value:
        return False
    try:
        decoded = _decode_quoted_token(value)
    except (UnicodeError, ValueError):
        return False
    return len(decoded.encode("utf-8")) <= 512 and quote(decoded, safe="") == value


def _decode_quoted_token(value: str) -> str:
    return unquote(value, encoding="utf-8", errors="strict")


def _valid_path_object(value: str, prefixes: tuple[str, ...]) -> bool:
    for prefix in prefixes:
        if value.startswith(prefix):
            suffix = value.removeprefix(prefix)
            try:
                return _segment(suffix) == suffix
            except OCIRequestError:
                return False
    return False


def _query(raw_query: str, allowed: set[str]) -> dict[str, str]:
    try:
        pairs = parse_qsl(
            raw_query, keep_blank_values=True, strict_parsing=False,
            encoding="utf-8", errors="strict", max_num_fields=8,
        )
    except (UnicodeError, ValueError) as error:
        raise OCIRequestError("oci_query_invalid", "OCI Distribution query is invalid") from error
    result: dict[str, str] = {}
    for name, value in pairs:
        if name not in allowed or name in result or len(value.encode("utf-8")) > 512:
            raise OCIRequestError("oci_query_invalid", "OCI Distribution query is invalid")
        result[name] = value
    return result


def _identity(action: str, repository: str, object_value: str) -> ParsedOCIRequest:
    if len(repository.encode("utf-8")) > 1024 or len(object_value.encode("utf-8")) > 1024:
        raise OCIRequestError("oci_path_invalid", "OCI Distribution identity is too long")
    return ParsedOCIRequest(action, repository, object_value)


def parse_oci_request(method: str, path: str, raw_query: str) -> ParsedOCIRequest | None:
    """Classify standard `/v2/` routes without reading manifest or blob bodies."""

    if path != "/v2" and not path.startswith("/v2/"):
        return None
    method = method.upper()
    if path in {"/v2", "/v2/"}:
        return None

    relative = path[len("/v2/"):]
    if relative.endswith("/blobs/uploads/"):
        relative = relative[:-1]
    parts = [_segment(part) for part in relative.split("/")]
    if parts == ["_catalog"]:
        if method != "GET":
            raise OCIRequestError("oci_route_invalid", "OCI catalog request is invalid")
        _query(raw_query, {"n", "last"})
        return _identity("list", "", "catalog")
    if len(parts) >= 3 and parts[-2:] == ["tags", "list"]:
        if method != "GET":
            raise OCIRequestError("oci_route_invalid", "OCI tag-list request is invalid")
        _query(raw_query, {"n", "last"})
        return _identity("list", "/".join(parts[:-2]), "tags")
    if len(parts) >= 3 and parts[-2] in {"manifests", "referrers"}:
        repository, kind, reference = "/".join(parts[:-2]), parts[-2], parts[-1]
        if kind == "referrers":
            if method != "GET":
                raise OCIRequestError("oci_route_invalid", "OCI referrers request is invalid")
            _query(raw_query, {"artifactType"})
            return _identity("pull", repository, "referrers:" + reference)
        if _query(raw_query, set()):
            raise OCIRequestError("oci_query_invalid", "OCI manifest query is invalid")
        actions = {"GET": "pull", "HEAD": "pull", "PUT": "push", "DELETE": "delete"}
        if method not in actions:
            raise OCIRequestError("oci_route_invalid", "OCI manifest request is invalid")
        return _identity(actions[method], repository, "manifest:" + reference)
    if len(parts) >= 3 and parts[-2] == "blobs" and parts[-1] != "uploads":
        if _query(raw_query, set()):
            raise OCIRequestError("oci_query_invalid", "OCI blob query is invalid")
        actions = {"GET": "pull", "HEAD": "pull", "DELETE": "delete"}
        if method not in actions:
            raise OCIRequestError("oci_route_invalid", "OCI blob request is invalid")
        return _identity(actions[method], "/".join(parts[:-2]), "blob:" + parts[-1])
    upload_index = len(parts) - 2 if parts[-2:] == ["blobs", "uploads"] else len(parts) - 3
    if upload_index >= 1 and parts[upload_index:upload_index + 2] == ["blobs", "uploads"]:
        repository = "/".join(parts[:upload_index])
        upload_id = parts[-1] if len(parts) == upload_index + 3 else ""
        query = _query(raw_query, {"digest", "mount", "from"})
        if not upload_id and method == "POST":
            if "mount" in query:
                if set(query) != {"mount", "from"} or not query["mount"] or not query["from"]:
                    raise OCIRequestError("oci_query_invalid", "OCI blob mount query is invalid")
                if not _valid_repository(query["from"], allow_empty=False):
                    raise OCIRequestError("oci_query_invalid", "OCI blob mount source repository is invalid")
                mount_object = (
                    "mount:" + quote(query["mount"], safe="")
                    + ":from:" + quote(query["from"], safe="")
                )
                return _identity("mount", repository, mount_object)
            if "digest" in query:
                if not query["digest"] or set(query) != {"digest"}:
                    raise OCIRequestError("oci_query_invalid", "OCI monolithic upload query is invalid")
                return _identity("complete_upload", repository, "blob:" + quote(query["digest"], safe=""))
            if query:
                raise OCIRequestError("oci_query_invalid", "OCI upload-start query is invalid")
            return _identity("start_upload", repository, "upload")
        if upload_id:
            if method == "GET" and not query:
                return _identity("upload_status", repository, "upload:" + upload_id)
            if method == "PATCH" and not query:
                return _identity("upload_chunk", repository, "upload:" + upload_id)
            if method == "PUT" and set(query) == {"digest"} and query["digest"]:
                return _identity(
                    "complete_upload", repository,
                    "upload:" + quote(upload_id, safe="") + ":blob:" + quote(query["digest"], safe=""),
                )
            if method == "DELETE" and not query:
                return _identity("cancel_upload", repository, "upload:" + upload_id)
        raise OCIRequestError("oci_route_invalid", "OCI blob upload request is invalid")
    if any(marker in parts for marker in {"_catalog", "manifests", "referrers", "blobs", "tags"}):
        raise OCIRequestError("oci_route_invalid", "OCI Distribution object route is malformed")
    return None
