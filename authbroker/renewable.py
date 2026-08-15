"""Closed reviewed renewable-session mechanics for the Auth Broker."""

from __future__ import annotations

from dataclasses import dataclass
from types import MappingProxyType
from typing import Any, Callable, Mapping, Protocol

from .anthropic_claude_oauth import (
    ClaudeOAuthState,
    refresh as refresh_anthropic_oauth,
)
from .broker_contract import BrokerError
from .credential_records import (
    ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
)
from .datadog_oauth import PupOAuthState, refresh as refresh_datadog_oauth
from .openai_codex_oauth import (
    CodexOAuthState,
    refresh as refresh_openai_codex_oauth,
)


@dataclass(frozen=True)
class OpenAIAccountSupplement:
    account_id: str


@dataclass(frozen=True)
class ResolvedRenewableSecret:
    secret: bytes
    supplemental: OpenAIAccountSupplement | None = None


@dataclass(frozen=True)
class RefreshedRenewableSession:
    state: bytes
    resolved: ResolvedRenewableSecret


class RenewableSessionAdapter(Protocol):
    """Provider mechanics without Vault, handle, lock, or barrier authority."""

    provider_id: str
    credential_kind: str

    def validate_initial_state(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> None: ...

    def current_secret(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> ResolvedRenewableSecret | None: ...

    def refresh(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> RefreshedRenewableSession: ...

    def workspace_projection_values(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> dict[str, Any]: ...


@dataclass(frozen=True)
class ReviewedRenewableSessionDependencies:
    """Closed testable provider transports, never a runtime adapter registry."""

    datadog_refresh: Callable[
        [PupOAuthState, float], tuple[bytes, PupOAuthState]
    ] | None = None
    openai_refresh: Callable[
        [CodexOAuthState, float], tuple[bytes, CodexOAuthState]
    ] | None = None
    anthropic_refresh: Callable[
        [ClaudeOAuthState, float], tuple[bytes, ClaudeOAuthState]
    ] | None = None


class DatadogRenewableSessionAdapter:
    provider_id = "datadog"
    credential_kind = DATADOG_OAUTH_CREDENTIAL_KIND

    def __init__(
        self,
        refresh: Callable[
            [PupOAuthState, float], tuple[bytes, PupOAuthState]
        ] | None = None,
    ) -> None:
        self._refresh = refresh or (
            lambda state, now: refresh_datadog_oauth(state, now=now)
        )

    def validate_initial_state(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> None:
        del account_label
        PupOAuthState.parse(state, driver_revision=driver_revision)

    def current_secret(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> ResolvedRenewableSecret | None:
        del account_label
        parsed = PupOAuthState.parse(state, driver_revision=driver_revision)
        secret = parsed.access_token(now)
        if secret is None:
            return None
        return ResolvedRenewableSecret(secret=secret)

    def refresh(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> RefreshedRenewableSession:
        del account_label
        parsed = PupOAuthState.parse(state, driver_revision=driver_revision)
        secret, refreshed = self._refresh(parsed, now)
        refreshed_state = refreshed.encode()
        reparsed = PupOAuthState.parse(
            refreshed_state, driver_revision=driver_revision
        )
        if not secret or reparsed.access_token(now) != secret:
            raise BrokerError("datadog_oauth_refresh_failed")
        return RefreshedRenewableSession(
            state=refreshed_state,
            resolved=ResolvedRenewableSecret(secret=secret),
        )

    def workspace_projection_values(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> dict[str, Any]:
        self.validate_initial_state(
            state, driver_revision=driver_revision, account_label=account_label
        )
        return {}


class OpenAIRenewableSessionAdapter:
    provider_id = "openai"
    credential_kind = OPENAI_CODEX_OAUTH_CREDENTIAL_KIND

    def __init__(
        self,
        refresh: Callable[
            [CodexOAuthState, float], tuple[bytes, CodexOAuthState]
        ] | None = None,
    ) -> None:
        self._refresh = refresh or (
            lambda state, now: refresh_openai_codex_oauth(state, now=now)
        )

    @staticmethod
    def _resolved(parsed: CodexOAuthState, secret: bytes) -> ResolvedRenewableSecret:
        return ResolvedRenewableSecret(
            secret=secret,
            supplemental=OpenAIAccountSupplement(parsed.account_id),
        )

    def validate_initial_state(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> None:
        parsed = CodexOAuthState.parse(state, driver_revision=driver_revision)
        if parsed.account_id != account_label:
            raise BrokerError("invalid_account_label")

    def current_secret(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> ResolvedRenewableSecret | None:
        parsed = CodexOAuthState.parse(state, driver_revision=driver_revision)
        if parsed.account_id != account_label:
            raise BrokerError("invalid_account_label")
        secret = parsed.access_token(now)
        if secret is None:
            return None
        return self._resolved(parsed, secret)

    def refresh(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> RefreshedRenewableSession:
        parsed = CodexOAuthState.parse(state, driver_revision=driver_revision)
        if parsed.account_id != account_label:
            raise BrokerError("invalid_account_label")
        secret, refreshed = self._refresh(parsed, now)
        refreshed_state = refreshed.encode()
        reparsed = CodexOAuthState.parse(
            refreshed_state, driver_revision=driver_revision
        )
        if (
            not secret
            or reparsed.account_id != parsed.account_id
            or reparsed.access_token(now) != secret
        ):
            raise BrokerError("openai_oauth_refresh_failed")
        return RefreshedRenewableSession(
            state=refreshed_state,
            resolved=self._resolved(reparsed, secret),
        )

    def workspace_projection_values(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> dict[str, Any]:
        self.validate_initial_state(
            state, driver_revision=driver_revision, account_label=account_label
        )
        return {}


class AnthropicRenewableSessionAdapter:
    provider_id = "anthropic"
    credential_kind = ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND

    def __init__(
        self,
        refresh: Callable[
            [ClaudeOAuthState, float], tuple[bytes, ClaudeOAuthState]
        ]
        | None = None,
    ) -> None:
        self._refresh = refresh or (
            lambda state, now: refresh_anthropic_oauth(state, now=now)
        )

    def validate_initial_state(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> None:
        if account_label != "claude-user-native":
            raise BrokerError("invalid_account_label")
        ClaudeOAuthState.parse(state, driver_revision=driver_revision)

    def current_secret(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> ResolvedRenewableSecret | None:
        self.validate_initial_state(
            state, driver_revision=driver_revision, account_label=account_label
        )
        secret = ClaudeOAuthState.parse(
            state, driver_revision=driver_revision
        ).access_token(now)
        return None if secret is None else ResolvedRenewableSecret(secret=secret)

    def refresh(
        self,
        state: bytes,
        *,
        driver_revision: str,
        account_label: str,
        now: float,
    ) -> RefreshedRenewableSession:
        self.validate_initial_state(
            state, driver_revision=driver_revision, account_label=account_label
        )
        secret, refreshed = self._refresh(
            ClaudeOAuthState.parse(state, driver_revision=driver_revision), now
        )
        encoded = refreshed.encode()
        if (
            ClaudeOAuthState.parse(
                encoded, driver_revision=driver_revision
            ).access_token(now)
            != secret
        ):
            raise BrokerError("anthropic_oauth_refresh_failed")
        return RefreshedRenewableSession(
            encoded, ResolvedRenewableSecret(secret=secret)
        )

    def workspace_projection_values(
        self, state: bytes, *, driver_revision: str, account_label: str
    ) -> dict[str, Any]:
        self.validate_initial_state(
            state, driver_revision=driver_revision, account_label=account_label
        )
        parsed = ClaudeOAuthState.parse(state, driver_revision=driver_revision)
        return {"oauth_scopes": list(parsed.oauth_scopes())}


RENEWABLE_CREDENTIAL_KINDS = frozenset(
    {
        DATADOG_OAUTH_CREDENTIAL_KIND,
        OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
        ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND,
    }
)


def reviewed_renewable_session_adapters(
    dependencies: ReviewedRenewableSessionDependencies | None = None,
) -> Mapping[str, RenewableSessionAdapter]:
    """Return the immutable compiled adapter union; there is no registration API."""

    dependencies = dependencies or ReviewedRenewableSessionDependencies()
    adapters: tuple[RenewableSessionAdapter, ...] = (
        AnthropicRenewableSessionAdapter(dependencies.anthropic_refresh),
        DatadogRenewableSessionAdapter(dependencies.datadog_refresh),
        OpenAIRenewableSessionAdapter(dependencies.openai_refresh),
    )
    registry = {adapter.credential_kind: adapter for adapter in adapters}
    if set(registry) != set(RENEWABLE_CREDENTIAL_KINDS):
        raise RuntimeError("reviewed renewable-session registry is inconsistent")
    if len({adapter.provider_id for adapter in adapters}) != len(adapters):
        raise RuntimeError("reviewed renewable-session registry duplicates a provider")
    return MappingProxyType(registry)
