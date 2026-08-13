from __future__ import annotations

import unittest

from authbroker.renewable import (
    DatadogRenewableSessionAdapter,
    OpenAIRenewableSessionAdapter,
    RENEWABLE_CREDENTIAL_KINDS,
    reviewed_renewable_session_adapters,
)
from authbroker.vault import (
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
)


class ReviewedRenewableSessionRegistryTests(unittest.TestCase):
    def test_registry_is_the_exact_immutable_compiled_union(self) -> None:
        registry = reviewed_renewable_session_adapters()

        self.assertEqual(
            set(registry),
            {
                DATADOG_OAUTH_CREDENTIAL_KIND,
                OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
            },
        )
        self.assertEqual(set(registry), set(RENEWABLE_CREDENTIAL_KINDS))
        self.assertIsInstance(
            registry[DATADOG_OAUTH_CREDENTIAL_KIND],
            DatadogRenewableSessionAdapter,
        )
        self.assertIsInstance(
            registry[OPENAI_CODEX_OAUTH_CREDENTIAL_KIND],
            OpenAIRenewableSessionAdapter,
        )
        self.assertEqual(
            {
                kind: adapter.provider_id
                for kind, adapter in registry.items()
            },
            {
                DATADOG_OAUTH_CREDENTIAL_KIND: "datadog",
                OPENAI_CODEX_OAUTH_CREDENTIAL_KIND: "openai",
            },
        )
        with self.assertRaises(TypeError):
            registry["owner_selected_session"] = registry[  # type: ignore[index]
                DATADOG_OAUTH_CREDENTIAL_KIND
            ]

    def test_adapters_expose_no_broker_state_authority(self) -> None:
        for adapter in reviewed_renewable_session_adapters().values():
            with self.subTest(provider=adapter.provider_id):
                names = {name.lstrip("_") for name in vars(adapter)}
                self.assertFalse(
                    names
                    & {
                        "vaults",
                        "handles",
                        "record_locks",
                        "active_refresh_tasks",
                        "root_key",
                    }
                )


if __name__ == "__main__":
    unittest.main()
