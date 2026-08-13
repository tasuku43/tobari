"""Closed control-socket login request plans for reviewed providers."""

from __future__ import annotations

from dataclasses import dataclass
from types import MappingProxyType
from typing import Any, Callable, Mapping, Protocol

from .credential_records import (
    AWS_DRIVER_IDS,
    AWS_SSO_CREDENTIAL_KIND,
    CLAUDE_ACCOUNT_LABEL,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_DRIVER_ID,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    PUP_DRIVER_ID,
    new_aws_sso_record,
    new_datadog_oauth_record,
    new_openai_codex_oauth_record,
)
from .protocol import ProtocolError, require_exact_keys


REVIEWED_CONTROL_LOGIN_PROVIDERS = (
    "github",
    "aws",
    "datadog",
    "openai",
    "anthropic",
)


@dataclass(frozen=True)
class StaticControlLogin:
    provider_id: str
    context_id: Any
    account_label: Any
    secret: bytes


@dataclass(frozen=True)
class DriverControlLogin:
    plan: DriverControlLoginPlan
    context_id: Any
    account_label: Any
    driver_id: Any
    driver_revision: Any
    state: bytes


ControlLogin = StaticControlLogin | DriverControlLogin
RecordFactory = Callable[..., dict[str, Any]]


class ControlLoginPlan(Protocol):
    """Protocol mechanics without Broker, Vault, helper, or process authority."""

    provider_id: str
    payload_field: str

    def validate_client_metadata(
        self, account_label: Any, driver_id: Any, driver_revision: Any
    ) -> None: ...

    def build_request(
        self,
        *,
        context_id: Any,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
        payload_length: int,
    ) -> dict[str, Any]: ...

    def parse(self, request: dict[str, Any], raw_payload: bytes) -> ControlLogin: ...


@dataclass(frozen=True)
class StaticControlLoginPlan:
    provider_id: str
    required_account_label: str | None = None
    payload_field: str = "secret_length"

    def _validate_account(self, account_label: Any) -> None:
        if (
            self.required_account_label is not None
            and account_label != self.required_account_label
        ):
            raise ProtocolError("invalid_request")

    def validate_client_metadata(
        self, account_label: Any, driver_id: Any, driver_revision: Any
    ) -> None:
        if driver_id is not None or driver_revision is not None:
            raise ProtocolError("invalid_request")
        self._validate_account(account_label)

    def build_request(
        self,
        *,
        context_id: Any,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
        payload_length: int,
    ) -> dict[str, Any]:
        self.validate_client_metadata(account_label, driver_id, driver_revision)
        return {
            "context_id": context_id,
            "provider": self.provider_id,
            "secret_length": payload_length,
            "account_label": account_label,
        }

    def parse(
        self, request: dict[str, Any], raw_payload: bytes
    ) -> StaticControlLogin:
        require_exact_keys(
            request,
            {
                "schema_version",
                "op",
                "context_id",
                "provider",
                "secret_length",
                "account_label",
            },
        )
        if len(raw_payload) != request["secret_length"]:
            raise ProtocolError("invalid_length")
        self._validate_account(request["account_label"])
        return StaticControlLogin(
            provider_id=self.provider_id,
            context_id=request["context_id"],
            account_label=request["account_label"],
            secret=raw_payload,
        )


@dataclass(frozen=True)
class DriverControlLoginPlan:
    provider_id: str
    credential_kind: str
    driver_ids: frozenset[str]
    record_factory: RecordFactory
    payload_field: str = "state_length"

    def validate_client_metadata(
        self, account_label: Any, driver_id: Any, driver_revision: Any
    ) -> None:
        del account_label
        if driver_id not in self.driver_ids or driver_revision is None:
            raise ProtocolError("invalid_request")

    def build_request(
        self,
        *,
        context_id: Any,
        account_label: Any,
        driver_id: Any,
        driver_revision: Any,
        payload_length: int,
    ) -> dict[str, Any]:
        self.validate_client_metadata(account_label, driver_id, driver_revision)
        return {
            "context_id": context_id,
            "provider": self.provider_id,
            "account_label": account_label,
            "driver_id": driver_id,
            "driver_revision": driver_revision,
            "state_length": payload_length,
        }

    def parse(
        self, request: dict[str, Any], raw_payload: bytes
    ) -> DriverControlLogin:
        require_exact_keys(
            request,
            {
                "schema_version",
                "op",
                "context_id",
                "provider",
                "account_label",
                "driver_id",
                "driver_revision",
                "state_length",
            },
        )
        if len(raw_payload) != request["state_length"]:
            raise ProtocolError("invalid_length")
        return DriverControlLogin(
            plan=self,
            context_id=request["context_id"],
            account_label=request["account_label"],
            driver_id=request["driver_id"],
            driver_revision=request["driver_revision"],
            state=raw_payload,
        )

    def new_record(self, login: DriverControlLogin) -> dict[str, Any]:
        if login.plan is not self:
            raise ProtocolError("invalid_request")
        return self.record_factory(
            login.state,
            account_label=login.account_label,
            driver_id=login.driver_id,
            driver_revision=login.driver_revision,
        )


def _build_reviewed_control_login_plans() -> Mapping[str, ControlLoginPlan]:
    plans: tuple[ControlLoginPlan, ...] = (
        StaticControlLoginPlan("github"),
        DriverControlLoginPlan(
            "aws",
            AWS_SSO_CREDENTIAL_KIND,
            AWS_DRIVER_IDS,
            new_aws_sso_record,
        ),
        DriverControlLoginPlan(
            "datadog",
            DATADOG_OAUTH_CREDENTIAL_KIND,
            frozenset({PUP_DRIVER_ID}),
            new_datadog_oauth_record,
        ),
        DriverControlLoginPlan(
            "openai",
            OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
            frozenset({OPENAI_CODEX_DRIVER_ID}),
            new_openai_codex_oauth_record,
        ),
        StaticControlLoginPlan("anthropic", CLAUDE_ACCOUNT_LABEL),
    )
    registry = {plan.provider_id: plan for plan in plans}
    if tuple(registry) != REVIEWED_CONTROL_LOGIN_PROVIDERS:
        raise RuntimeError("reviewed control-login registry is inconsistent")
    return MappingProxyType(registry)


_REVIEWED_CONTROL_LOGIN_PLANS = _build_reviewed_control_login_plans()


def reviewed_control_login_plans() -> Mapping[str, ControlLoginPlan]:
    """Return the exact immutable compiled login union."""

    return _REVIEWED_CONTROL_LOGIN_PLANS


def control_login_plan(provider: Any) -> ControlLoginPlan:
    plan = (
        _REVIEWED_CONTROL_LOGIN_PLANS.get(provider)
        if isinstance(provider, str)
        else None
    )
    if plan is None:
        raise ProtocolError("invalid_provider")
    return plan


def control_login_payload_field(provider: Any) -> str:
    """Preserve the pre-dispatch unknown-provider length classification."""

    plan = (
        _REVIEWED_CONTROL_LOGIN_PLANS.get(provider)
        if isinstance(provider, str)
        else None
    )
    return plan.payload_field if plan is not None else "secret_length"


def is_reviewed_driver_login_provider(provider: Any) -> bool:
    plan = (
        _REVIEWED_CONTROL_LOGIN_PLANS.get(provider)
        if isinstance(provider, str)
        else None
    )
    return isinstance(plan, DriverControlLoginPlan)


def parse_control_login(
    request: dict[str, Any], raw_payload: bytes
) -> ControlLogin:
    return control_login_plan(request.get("provider")).parse(request, raw_payload)
