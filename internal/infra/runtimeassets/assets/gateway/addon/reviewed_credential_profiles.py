"""Closed non-secret Gateway projection profiles for reviewed credentials."""

from __future__ import annotations

import re
from types import MappingProxyType
from typing import Any, Mapping, Protocol


PRIMARY_SECRET_FIELD = "primary_secret"
AWS_SSO_CREDENTIAL_KIND = "aws_sso_session"
DATADOG_OAUTH_CREDENTIAL_KIND = "datadog_oauth_session"
OPENAI_CODEX_OAUTH_CREDENTIAL_KIND = "openai_codex_oauth_session"
OPENAI_ACCOUNT_ID_PATTERN = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$")


class ReviewedGatewayCredentialProfile(Protocol):
    provider_id: str
    credential_kind: str
    renewable: bool
    supplemental_header_names: frozenset[str]

    def matches_projection(
        self,
        *,
        display_name: str,
        mode: str,
        helper: Any,
        normalized: list[dict[str, Any]],
        signing_bindings: list[dict[str, Any]],
        environment: list[dict[str, str]],
        complete_files: list[dict[str, str]],
    ) -> bool: ...

    def validate_supplemental_headers(
        self, value: Any
    ) -> dict[str, str] | None: ...


class _BaseProfile:
    renewable = False
    supplemental_header_names: frozenset[str] = frozenset()

    def validate_supplemental_headers(self, value: Any) -> dict[str, str] | None:
        return {} if value is None else None


class AnthropicClaudeProfile(_BaseProfile):
    provider_id = "anthropic"
    credential_kind = PRIMARY_SECRET_FIELD

    def matches_projection(
        self,
        *,
        display_name: str,
        mode: str,
        helper: Any,
        normalized: list[dict[str, Any]],
        signing_bindings: list[dict[str, Any]],
        environment: list[dict[str, str]],
        complete_files: list[dict[str, str]],
    ) -> bool:
        return (
            display_name == "Anthropic account for Claude Code"
            and mode == "builtin_helper"
            and helper == "claude-setup-token"
            and not signing_bindings
            and environment
            == [
                {
                    "provider_id": "anthropic",
                    "name": "CLAUDE_CODE_OAUTH_TOKEN",
                    "template": "${HANDLE}",
                }
            ]
            and not complete_files
            and normalized
            == [
                {
                    "provider_id": "anthropic",
                    "target": {
                        "scheme": "https",
                        "host": "api.anthropic.com",
                        "port": 443,
                    },
                    "source": {"header": "authorization", "format": "bearer"},
                    "destination": {
                        "header": "authorization",
                        "format": "bearer",
                        "secret_field": PRIMARY_SECRET_FIELD,
                    },
                    "secret_headers": ["authorization"],
                }
            ]
        )


class AWSSigV4Profile(_BaseProfile):
    provider_id = "aws"
    credential_kind = AWS_SSO_CREDENTIAL_KIND

    def matches_projection(
        self,
        *,
        display_name: str,
        mode: str,
        helper: Any,
        normalized: list[dict[str, Any]],
        signing_bindings: list[dict[str, Any]],
        environment: list[dict[str, str]],
        complete_files: list[dict[str, str]],
    ) -> bool:
        del display_name, environment, complete_files
        return (
            mode == "builtin_helper"
            and helper == "aws-sso"
            and len(signing_bindings) == 1
            and not normalized
        )


class DatadogOAuthProfile(_BaseProfile):
    provider_id = "datadog"
    credential_kind = DATADOG_OAUTH_CREDENTIAL_KIND
    renewable = True

    def matches_projection(
        self,
        *,
        display_name: str,
        mode: str,
        helper: Any,
        normalized: list[dict[str, Any]],
        signing_bindings: list[dict[str, Any]],
        environment: list[dict[str, str]],
        complete_files: list[dict[str, str]],
    ) -> bool:
        del display_name
        return (
            mode == "builtin_helper"
            and helper == "pup-oauth"
            and not signing_bindings
            and normalized
            == [
                {
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
                }
            ]
            and environment
            == [
                {
                    "provider_id": "datadog",
                    "name": "DD_ACCESS_TOKEN",
                    "template": "${HANDLE}",
                },
                {
                    "provider_id": "datadog",
                    "name": "DD_SITE",
                    "template": "datadoghq.com",
                },
            ]
            and not complete_files
        )


class OpenAICodexOAuthProfile(_BaseProfile):
    provider_id = "openai"
    credential_kind = OPENAI_CODEX_OAUTH_CREDENTIAL_KIND
    renewable = True
    supplemental_header_names = frozenset({"chatgpt-account-id"})

    def matches_projection(
        self,
        *,
        display_name: str,
        mode: str,
        helper: Any,
        normalized: list[dict[str, Any]],
        signing_bindings: list[dict[str, Any]],
        environment: list[dict[str, str]],
        complete_files: list[dict[str, str]],
    ) -> bool:
        return (
            display_name == "OpenAI account for Codex"
            and mode == "builtin_helper"
            and helper == "codex-chatgpt-oauth"
            and not signing_bindings
            and normalized
            == [
                {
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
                }
            ]
            and not environment
            and complete_files
            == [
                {
                    "provider_id": "openai",
                    "path": ".codex/auth.json",
                    "template": (
                        '{"auth_mode":"chatgptAuthTokens","OPENAI_API_KEY":null,'
                        '"tokens":{"id_token":"e30.e30.x","access_token":"${HANDLE}",'
                        '"refresh_token":"","account_id":null},'
                        '"last_refresh":"1970-01-01T00:00:00Z"}'
                    ),
                }
            ]
        )

    def validate_supplemental_headers(self, value: Any) -> dict[str, str] | None:
        if not isinstance(value, dict) or set(value) != self.supplemental_header_names:
            return None
        account_id = value.get("chatgpt-account-id")
        if (
            not isinstance(account_id, str)
            or OPENAI_ACCOUNT_ID_PATTERN.fullmatch(account_id) is None
        ):
            return None
        return {"chatgpt-account-id": account_id}


def _build_profiles() -> Mapping[str, ReviewedGatewayCredentialProfile]:
    profiles: tuple[ReviewedGatewayCredentialProfile, ...] = (
        AWSSigV4Profile(),
        DatadogOAuthProfile(),
        OpenAICodexOAuthProfile(),
    )
    registry = {profile.credential_kind: profile for profile in profiles}
    if len(registry) != len(profiles):
        raise RuntimeError("reviewed Gateway credential profiles duplicate a kind")
    return MappingProxyType(registry)


_DYNAMIC_PROFILES = _build_profiles()
_ANTHROPIC_PROFILE = AnthropicClaudeProfile()
REVIEWED_DYNAMIC_CREDENTIAL_KINDS = frozenset(_DYNAMIC_PROFILES)
REVIEWED_HEADER_SECRET_FIELDS = frozenset(
    {DATADOG_OAUTH_CREDENTIAL_KIND, OPENAI_CODEX_OAUTH_CREDENTIAL_KIND}
)
RENEWABLE_PROVIDER_IDS = frozenset(
    profile.provider_id for profile in _DYNAMIC_PROFILES.values() if profile.renewable
)


def reviewed_gateway_credential_profiles() -> Mapping[
    str, ReviewedGatewayCredentialProfile
]:
    return _DYNAMIC_PROFILES


def reviewed_projection_profile(
    provider_id: Any, credential_kind: Any, helper: Any
) -> ReviewedGatewayCredentialProfile | None:
    if provider_id == _ANTHROPIC_PROFILE.provider_id or helper == "claude-setup-token":
        return _ANTHROPIC_PROFILE
    return _DYNAMIC_PROFILES.get(credential_kind)


def response_profile(
    provider_id: Any, secret_field: Any
) -> ReviewedGatewayCredentialProfile | None:
    profile = _DYNAMIC_PROFILES.get(secret_field)
    if profile is None or profile.provider_id != provider_id:
        return None
    return profile
