"""Tobari's generic HTTP policy enforcement addon for mitmproxy."""

from __future__ import annotations

import hashlib
import json
import os
import time
import urllib.error
import urllib.request
import uuid
from pathlib import PurePosixPath
from typing import Any
from urllib.parse import parse_qs, unquote, urlsplit

from mitmproxy import http

DEFAULT_SECRET_HEADERS = frozenset(
    {
        "authorization",
        "proxy-authorization",
        "cookie",
        "set-cookie",
        "x-api-key",
    }
)
PROFILE_HEADER = "x-tobari-credential-profile"
MAX_CREDENTIAL_CONFIG_BYTES = 256 * 1024
MAX_SECRET_BYTES = 64 * 1024


class PolicyUnavailable(Exception):
    """OPA did not return one valid decision."""


class CredentialError(Exception):
    """A selected credential could not be injected safely."""


class Decision:
    """Validated OPA decision compatible with mitmproxy's script loader."""

    __slots__ = ("allow", "reason", "credential_profile", "status_code")

    def __init__(
        self,
        allow: bool,
        reason: str,
        credential_profile: str | None,
        status_code: int,
    ) -> None:
        self.allow = allow
        self.reason = reason
        self.credential_profile = credential_profile
        self.status_code = status_code


def _positive_int(name: str, default: int, minimum: int, maximum: int) -> int:
    raw = os.getenv(name, str(default))
    try:
        value = int(raw)
    except ValueError as error:
        raise RuntimeError(f"{name} must be an integer") from error
    if value < minimum or value > maximum:
        raise RuntimeError(f"{name} must be between {minimum} and {maximum}")
    return value


def _headers_for_policy(headers: http.Headers, secret_names: set[str]) -> dict[str, str]:
    safe: dict[str, list[str]] = {}
    for name, value in headers.fields:
        decoded_name = name.decode("latin-1").lower()
        if decoded_name in secret_names or decoded_name == PROFILE_HEADER:
            continue
        safe.setdefault(decoded_name, []).append(value.decode("latin-1"))
    return {name: ", ".join(values) for name, values in sorted(safe.items())}


def _body_metadata(raw: bytes, content_type: str, inspection_bytes: int) -> dict[str, Any]:
    metadata: dict[str, Any] = {
        "kind": "metadata",
        "size": len(raw),
        "truncated": len(raw) > inspection_bytes,
        "sha256": hashlib.sha256(raw).hexdigest(),
        "content_type": content_type,
    }
    media_type = content_type.split(";", 1)[0].strip().lower()
    if media_type == "application/json" and len(raw) <= inspection_bytes:
        try:
            metadata["value"] = json.loads(raw.decode("utf-8"))
            metadata["kind"] = "json"
        except (UnicodeDecodeError, json.JSONDecodeError):
            metadata["kind"] = "invalid_json"
    return metadata


def build_policy_input(
    flow: http.HTTPFlow,
    cluster: str,
    session: str | None,
    inspection_bytes: int,
    extra_secret_names: set[str],
) -> dict[str, Any]:
    request = flow.request
    split = urlsplit(request.url)
    host = request.host.rstrip(".").lower()
    path = split.path or "/"
    requested_profile = request.headers.get(PROFILE_HEADER)
    secret_names = set(DEFAULT_SECRET_HEADERS) | extra_secret_names
    raw = request.raw_content or b""
    return {
        "version": "v1",
        "principal": {"cluster": cluster, "session": session},
        "request": {
            "scheme": request.scheme.lower(),
            "host": host,
            "port": request.port,
            "method": request.method.upper(),
            "path": path,
            "path_segments": [unquote(segment) for segment in path.split("/") if segment],
            "query": parse_qs(split.query, keep_blank_values=True),
            "headers": _headers_for_policy(request.headers, secret_names),
            "body": _body_metadata(
                raw,
                request.headers.get("content-type", ""),
                inspection_bytes,
            ),
        },
        "credential": {"requested_profile": requested_profile},
    }


def _parse_decision(document: Any) -> Decision:
    if not isinstance(document, dict) or not isinstance(document.get("result"), dict):
        raise PolicyUnavailable("OPA response has no result object")
    result = document["result"]
    allow = result.get("allow")
    reason = result.get("reason")
    profile = result.get("credential_profile")
    if not isinstance(allow, bool) or not isinstance(reason, str) or not reason:
        raise PolicyUnavailable("OPA result has invalid allow or reason")
    if profile is not None and (not isinstance(profile, str) or not profile):
        raise PolicyUnavailable("OPA result has invalid credential_profile")
    status = result.get("status_code", 403)
    if not isinstance(status, int) or status != 403:
        raise PolicyUnavailable("OPA result has invalid status_code")
    return Decision(allow=allow, reason=reason, credential_profile=profile, status_code=status)


def query_opa(url: str, policy_input: dict[str, Any], timeout: float) -> Decision:
    payload = json.dumps({"input": policy_input}, separators=(",", ":")).encode("utf-8")
    request = urllib.request.Request(
        url,
        data=payload,
        headers={"content-type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(request, timeout=timeout) as response:
            if response.status != 200:
                raise PolicyUnavailable("OPA returned a non-success status")
            body = response.read(1024 * 1024 + 1)
    except (OSError, urllib.error.URLError, TimeoutError) as error:
        raise PolicyUnavailable("OPA request failed") from error
    if len(body) > 1024 * 1024:
        raise PolicyUnavailable("OPA response exceeded the size limit")
    try:
        return _parse_decision(json.loads(body))
    except (json.JSONDecodeError, UnicodeDecodeError) as error:
        raise PolicyUnavailable("OPA response was not valid JSON") from error


def load_credential_config(path: str) -> dict[str, Any]:
    try:
        with open(path, "rb") as handle:
            raw = handle.read(MAX_CREDENTIAL_CONFIG_BYTES + 1)
    except OSError as error:
        raise CredentialError("credential configuration could not be read") from error
    if len(raw) > MAX_CREDENTIAL_CONFIG_BYTES:
        raise CredentialError("credential configuration is too large")
    try:
        document = json.loads(raw)
    except (UnicodeDecodeError, json.JSONDecodeError) as error:
        raise CredentialError("credential configuration is invalid") from error
    if not isinstance(document, dict) or document.get("version") != "v1":
        raise CredentialError("credential configuration version is invalid")
    profiles = document.get("profiles")
    if not isinstance(profiles, dict):
        raise CredentialError("credential profiles are invalid")
    return document


def configured_secret_headers(config: dict[str, Any]) -> set[str]:
    names: set[str] = set()
    for profile in config.get("profiles", {}).values():
        if isinstance(profile, dict) and isinstance(profile.get("header"), str):
            names.add(profile["header"].lower())
    return names


def _validated_profile(config: dict[str, Any], name: str, host: str) -> dict[str, Any]:
    profile = config.get("profiles", {}).get(name)
    if not isinstance(profile, dict):
        raise CredentialError("OPA selected an unknown credential profile")
    profile_type = profile.get("type")
    hosts = profile.get("hosts")
    secret_file = profile.get("secret_file")
    if profile_type not in {"bearer", "header"}:
        raise CredentialError("credential profile type is invalid")
    if not isinstance(hosts, list) or not hosts or any(not isinstance(item, str) for item in hosts):
        raise CredentialError("credential profile hosts are invalid")
    if host not in {item.rstrip(".").lower() for item in hosts}:
        raise CredentialError("credential profile is not bound to the request host")
    if not isinstance(secret_file, str):
        raise CredentialError("credential secret path is invalid")
    secret_path = PurePosixPath(secret_file)
    if (
        secret_path.parent != PurePosixPath("/run/tobari/credentials")
        or secret_path.name in {"", ".", ".."}
    ):
        raise CredentialError("credential secret path is invalid")
    header = profile.get("header", "authorization" if profile_type == "bearer" else None)
    if not isinstance(header, str) or not header or header.lower() in {"host", "content-length"}:
        raise CredentialError("credential header is invalid")
    return {**profile, "header": header}


def inject_credential(
    request: http.Request,
    config: dict[str, Any],
    profile_name: str,
    host: str,
) -> None:
    profile = _validated_profile(config, profile_name, host)
    try:
        with open(profile["secret_file"], "rb") as handle:
            raw = handle.read(MAX_SECRET_BYTES + 1)
    except OSError as error:
        raise CredentialError("credential secret could not be read") from error
    if not raw or len(raw) > MAX_SECRET_BYTES or b"\x00" in raw:
        raise CredentialError("credential secret is invalid")
    value = raw.rstrip(b"\r\n").decode("utf-8", errors="strict")
    if not value or "\r" in value or "\n" in value:
        raise CredentialError("credential secret is invalid")
    header = profile["header"]
    request.headers[header] = f"Bearer {value}" if profile["type"] == "bearer" else value


def _deny(flow: http.HTTPFlow, status: int, code: str) -> None:
    body = json.dumps({"error": code}, separators=(",", ":")).encode("utf-8")
    flow.response = http.Response.make(status, body, {"content-type": "application/json"})


def _audit(**fields: Any) -> None:
    print(json.dumps(fields, separators=(",", ":"), sort_keys=True), flush=True)


class TobariGateway:
    def __init__(self) -> None:
        self.opa_url = os.getenv(
            "TOBARI_OPA_URL",
            "http://opa:8181/v1/data/tobari/http/decision",
        )
        self.cluster = os.getenv("TOBARI_CLUSTER", "default")
        self.inspection_bytes = _positive_int(
            "TOBARI_INSPECTION_BYTES", 1024 * 1024, 1024, 8 * 1024 * 1024
        )
        self.opa_timeout = float(
            _positive_int("TOBARI_OPA_TIMEOUT_SECONDS", 2, 1, 10)
        )
        self.credential_path = os.getenv(
            "TOBARI_CREDENTIAL_CONFIG",
            "/run/tobari/config/credentials.json",
        )

    def request(self, flow: http.HTTPFlow) -> None:
        started = time.monotonic()
        request_id = uuid.uuid4().hex
        host = flow.request.host.rstrip(".").lower()
        profile_name: str | None = None
        upstream_status: int | None = None
        decision_name = "deny"
        reason = "gateway rejected request"
        try:
            config = load_credential_config(self.credential_path)
            secret_names = configured_secret_headers(config)
            policy_input = build_policy_input(
                flow,
                self.cluster,
                flow.request.headers.get("x-tobari-session"),
                self.inspection_bytes,
                secret_names,
            )
            for name in set(DEFAULT_SECRET_HEADERS) | secret_names | {PROFILE_HEADER}:
                if name not in {"cookie", "set-cookie"}:
                    flow.request.headers.pop(name, None)
            decision = query_opa(self.opa_url, policy_input, self.opa_timeout)
            reason = decision.reason
            profile_name = decision.credential_profile
            if not decision.allow:
                _deny(flow, decision.status_code, "policy_denied")
                upstream_status = decision.status_code
                return
            if profile_name is not None:
                inject_credential(flow.request, config, profile_name, host)
            decision_name = "allow"
            flow.metadata["tobari_audit"] = {
                "timestamp": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                "request_id": request_id,
                "cluster": self.cluster,
                "host": host,
                "method": flow.request.method.upper(),
                "path": urlsplit(flow.request.url).path or "/",
                "decision": decision_name,
                "reason": reason,
                "credential_profile": profile_name,
                "started": started,
            }
        except PolicyUnavailable as error:
            reason = str(error)
            _deny(flow, 503, "policy_unavailable")
            upstream_status = 503
        except CredentialError as error:
            reason = str(error)
            _deny(flow, 503, "credential_unavailable")
            upstream_status = 503
        except (RuntimeError, UnicodeError):
            reason = "credential processing failed"
            _deny(flow, 503, "credential_unavailable")
            upstream_status = 503
        except Exception:
            reason = "gateway error"
            _deny(flow, 502, "gateway_error")
            upstream_status = 502
        finally:
            if decision_name != "allow":
                _audit(
                    timestamp=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
                    request_id=request_id,
                    cluster=self.cluster,
                    host=host,
                    method=flow.request.method.upper(),
                    path=urlsplit(flow.request.url).path or "/",
                    decision=decision_name,
                    reason=reason,
                    credential_profile=profile_name,
                    upstream_status=upstream_status,
                    duration_ms=int((time.monotonic() - started) * 1000),
                )

    def response(self, flow: http.HTTPFlow) -> None:
        event = flow.metadata.pop("tobari_audit", None)
        if not isinstance(event, dict):
            return
        started = event.pop("started")
        _audit(
            **event,
            upstream_status=flow.response.status_code,
            duration_ms=int((time.monotonic() - started) * 1000),
        )

    def error(self, flow: http.HTTPFlow) -> None:
        event = flow.metadata.pop("tobari_audit", None)
        if not isinstance(event, dict):
            return
        started = event.pop("started")
        event["decision"] = "upstream_error"
        event["reason"] = "upstream request failed"
        _audit(
            **event,
            upstream_status=None,
            duration_ms=int((time.monotonic() - started) * 1000),
        )


addons = [TobariGateway()]
