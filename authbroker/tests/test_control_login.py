from __future__ import annotations

import unittest

import authbroker.control_login as login_module
from authbroker.control_login import (
    REVIEWED_CONTROL_LOGIN_PROVIDERS,
    DriverControlLoginPlan,
    StaticControlLoginPlan,
    is_reviewed_driver_login_provider,
    reviewed_control_login_plans,
)
from authbroker.credential_records import (
    ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND,
    AWS_SSO_CREDENTIAL_KIND,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    new_aws_sso_record,
    new_anthropic_claude_oauth_record,
    new_datadog_oauth_record,
    new_openai_codex_oauth_record,
)


class ReviewedControlLoginPlanTests(unittest.TestCase):
    def test_registry_is_the_exact_immutable_compiled_union(self) -> None:
        registry = reviewed_control_login_plans()

        self.assertEqual(tuple(registry), REVIEWED_CONTROL_LOGIN_PROVIDERS)
        self.assertIsInstance(registry["github"], StaticControlLoginPlan)
        self.assertIsInstance(registry["anthropic"], DriverControlLoginPlan)
        self.assertEqual(
            {
                provider: plan.credential_kind
                for provider, plan in registry.items()
                if isinstance(plan, DriverControlLoginPlan)
            },
            {
                "aws": AWS_SSO_CREDENTIAL_KIND,
                "datadog": DATADOG_OAUTH_CREDENTIAL_KIND,
                "openai": OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
                "anthropic": ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND,
            },
        )
        self.assertEqual(registry["github"].payload_field, "secret_length")
        for provider in ("aws", "datadog", "openai", "anthropic"):
            self.assertEqual(registry[provider].payload_field, "state_length")
            self.assertTrue(is_reviewed_driver_login_provider(provider))
        for provider in ("github", "chatwork", "arbitrary"):
            self.assertFalse(is_reviewed_driver_login_provider(provider))
        self.assertIs(registry["aws"].record_factory, new_aws_sso_record)
        self.assertIs(registry["datadog"].record_factory, new_datadog_oauth_record)
        self.assertIs(
            registry["openai"].record_factory,
            new_openai_codex_oauth_record,
        )
        self.assertIs(
            registry["anthropic"].record_factory,
            new_anthropic_claude_oauth_record,
        )
        with self.assertRaises(TypeError):
            registry["owner_selected_login"] = registry[  # type: ignore[index]
                "github"
            ]

    def test_plans_expose_no_broker_vault_or_host_execution_authority(self) -> None:
        for name in (
            "BrokerState",
            "VaultStore",
            "AESGCM",
            "CommandRunner",
            "Path",
            "os",
            "subprocess",
        ):
            self.assertFalse(hasattr(login_module, name), name)
        forbidden = {
            "broker",
            "vault",
            "root_key",
            "runner",
            "executable",
            "path",
            "locks",
        }
        for plan in reviewed_control_login_plans().values():
            with self.subTest(provider=plan.provider_id):
                names = {name.lstrip("_").lower() for name in vars(plan)}
                self.assertFalse(names & forbidden)


if __name__ == "__main__":
    unittest.main()
