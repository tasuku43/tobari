from __future__ import annotations

import unittest

import authbroker.request_signing as signing_module
from authbroker.credential_records import AWS_SSO_CREDENTIAL_KIND
from authbroker.request_signing import (
    AWSSigV4RequestSigningAdapter,
    reviewed_request_signing_adapters,
)


class ReviewedRequestSigningRegistryTests(unittest.TestCase):
    def test_registry_is_the_exact_immutable_compiled_union(self) -> None:
        registry = reviewed_request_signing_adapters()
        self.assertEqual(
            {kind: type(adapter) for kind, adapter in registry.items()},
            {AWS_SSO_CREDENTIAL_KIND: AWSSigV4RequestSigningAdapter},
        )
        adapter = registry[AWS_SSO_CREDENTIAL_KIND]
        self.assertEqual(adapter.provider_id, "aws")
        self.assertEqual(adapter.binding_kind, "aws_sigv4")
        with self.assertRaises(TypeError):
            registry["owner_selected_signer"] = adapter  # type: ignore[index]

    def test_adapter_exposes_no_broker_state_or_effect_authority(self) -> None:
        for name in (
            "BrokerState",
            "VaultStore",
            "CompanionChannelManager",
            "Path",
            "socket",
            "subprocess",
        ):
            self.assertFalse(hasattr(signing_module, name), name)
        forbidden = {
            "broker",
            "vault",
            "companion",
            "handle",
            "lock",
            "barrier",
            "executor",
        }
        for adapter in reviewed_request_signing_adapters().values():
            names = {name.lstrip("_").lower() for name in vars(adapter)}
            self.assertFalse(names & forbidden)


if __name__ == "__main__":
    unittest.main()
