"""Closed reviewed request-signing mechanics for the Auth Broker."""

from __future__ import annotations

import secrets
from dataclasses import dataclass, field
from types import MappingProxyType
from typing import Any, Callable, Mapping, Protocol

from .aws_sigv4 import (
    Clock as SigV4Clock,
    SigV4Headers,
    SigV4Request,
    parse_credentials,
    parse_request,
    sign,
)
from .broker_contract import AwsRefreshSnapshot, BrokerError
from .companion_protocol import (
    RefreshRequest,
    RefreshResult,
    decode_refresh_secret,
)
from .credential_records import AWS_SSO_CREDENTIAL_KIND


@dataclass(frozen=True)
class CompletedRequestSigning:
    state: bytes = field(repr=False)
    headers: SigV4Headers = field(repr=False)


class RequestSigningAdapter(Protocol):
    """Signing mechanics without companion, Vault, lock, or barrier authority."""

    provider_id: str
    credential_kind: str
    binding_kind: str

    def parse_request(self, document: Any) -> SigV4Request: ...

    def create_refresh_request(
        self, snapshot: AwsRefreshSnapshot
    ) -> RefreshRequest: ...

    def complete(
        self,
        request: SigV4Request,
        refresh_request: RefreshRequest,
        result: Any,
    ) -> CompletedRequestSigning: ...


@dataclass(frozen=True)
class ReviewedRequestSigningDependencies:
    refresh_clock: Callable[[], float] | None = None
    sigv4_clock: SigV4Clock | None = None


class AWSSigV4RequestSigningAdapter:
    provider_id = "aws"
    credential_kind = AWS_SSO_CREDENTIAL_KIND
    binding_kind = "aws_sigv4"

    def __init__(
        self, dependencies: ReviewedRequestSigningDependencies | None = None
    ) -> None:
        dependencies = dependencies or ReviewedRequestSigningDependencies()
        self._refresh_clock = dependencies.refresh_clock
        self._sigv4_clock = dependencies.sigv4_clock

    def parse_request(self, document: Any) -> SigV4Request:
        return parse_request(document)

    def create_refresh_request(
        self, snapshot: AwsRefreshSnapshot
    ) -> RefreshRequest:
        return RefreshRequest.create(
            context_id=snapshot.context_id,
            project_id=snapshot.project_id,
            provider=snapshot.provider,
            record_id=snapshot.record_id,
            grant_revision=snapshot.revision,
            state_generation=snapshot.state_generation,
            driver_id=snapshot.driver_id,
            driver_revision=snapshot.driver_revision,
            binding_digest=snapshot.binding_digest,
            request_digest=snapshot.request_digest,
            state=snapshot.state,
        )

    def complete(
        self,
        request: SigV4Request,
        refresh_request: RefreshRequest,
        result: Any,
    ) -> CompletedRequestSigning:
        if (
            not isinstance(result, RefreshResult)
            or result.request_id != refresh_request.request_id
            or not secrets.compare_digest(
                result.task_digest, refresh_request.task_digest
            )
            or result.state_generation != refresh_request.state_generation
        ):
            raise BrokerError("companion_result_invalid")
        decode_kwargs: dict[str, Any] = {}
        if self._refresh_clock is not None:
            decode_kwargs["clock"] = self._refresh_clock
        refreshed = decode_refresh_secret(result.secret_payload, **decode_kwargs)
        credentials = parse_credentials(
            {
                "access_key_id": refreshed.access_key_id,
                "secret_access_key": refreshed.secret_access_key,
                "session_token": refreshed.session_token,
            }
        )
        sign_kwargs: dict[str, Any] = {}
        if self._sigv4_clock is not None:
            sign_kwargs["clock"] = self._sigv4_clock
        return CompletedRequestSigning(
            state=refreshed.state,
            headers=sign(request, credentials, **sign_kwargs),
        )


REQUEST_SIGNING_CREDENTIAL_KINDS = frozenset({AWS_SSO_CREDENTIAL_KIND})


def reviewed_request_signing_adapters(
    dependencies: ReviewedRequestSigningDependencies | None = None,
) -> Mapping[str, RequestSigningAdapter]:
    """Return the immutable compiled signing union; there is no registration API."""

    adapters: tuple[RequestSigningAdapter, ...] = (
        AWSSigV4RequestSigningAdapter(dependencies),
    )
    registry = {adapter.credential_kind: adapter for adapter in adapters}
    if set(registry) != set(REQUEST_SIGNING_CREDENTIAL_KINDS):
        raise RuntimeError("reviewed request-signing registry is inconsistent")
    if len({adapter.provider_id for adapter in adapters}) != len(adapters):
        raise RuntimeError("reviewed request-signing registry duplicates a provider")
    return MappingProxyType(registry)
