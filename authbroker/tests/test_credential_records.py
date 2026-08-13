from __future__ import annotations

import unittest

import authbroker.credential_records as records_module
import authbroker.vault as vault_module
from authbroker.credential_records import (
    AWSSSORecordContract,
    AWS_SSO_CREDENTIAL_KIND,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    REVIEWED_CREDENTIAL_RECORD_KINDS,
    STATIC_CREDENTIAL_KIND,
    DatadogOAuthRecordContract,
    OpenAICodexOAuthRecordContract,
    StaticCredentialRecordContract,
    reviewed_credential_record_contracts,
)


class ReviewedCredentialRecordContractTests(unittest.TestCase):
    def test_registry_is_the_exact_immutable_compiled_union(self) -> None:
        registry = reviewed_credential_record_contracts()

        self.assertEqual(set(registry), set(REVIEWED_CREDENTIAL_RECORD_KINDS))
        self.assertEqual(
            {
                kind: type(contract)
                for kind, contract in registry.items()
            },
            {
                STATIC_CREDENTIAL_KIND: StaticCredentialRecordContract,
                AWS_SSO_CREDENTIAL_KIND: AWSSSORecordContract,
                DATADOG_OAUTH_CREDENTIAL_KIND: DatadogOAuthRecordContract,
                OPENAI_CODEX_OAUTH_CREDENTIAL_KIND: OpenAICodexOAuthRecordContract,
            },
        )
        self.assertEqual(
            {
                kind: contract.provider_id
                for kind, contract in registry.items()
            },
            {
                STATIC_CREDENTIAL_KIND: None,
                AWS_SSO_CREDENTIAL_KIND: "aws",
                DATADOG_OAUTH_CREDENTIAL_KIND: "datadog",
                OPENAI_CODEX_OAUTH_CREDENTIAL_KIND: "openai",
            },
        )
        with self.assertRaises(TypeError):
            registry["owner_selected_record"] = registry[  # type: ignore[index]
                STATIC_CREDENTIAL_KIND
            ]

    def test_contracts_expose_no_vault_or_filesystem_authority(self) -> None:
        for name in ("AESGCM", "Path", "VaultStore", "os", "stat"):
            self.assertFalse(hasattr(records_module, name), name)
        forbidden = {
            "root",
            "key",
            "aesgcm",
            "path",
            "directory",
            "open",
            "replace",
            "vaults",
        }
        for contract in reviewed_credential_record_contracts().values():
            with self.subTest(kind=contract.credential_kind):
                names = {name.lstrip("_").lower() for name in vars(contract)}
                self.assertFalse(names & forbidden)

    def test_vault_module_keeps_the_established_record_import_surface(self) -> None:
        for name in (
            "VaultError",
            "decode_secret",
            "empty_payload",
            "encode_secret",
            "new_aws_sso_record",
            "new_datadog_oauth_record",
            "new_openai_codex_oauth_record",
            "new_record",
            "validate_context_id",
            "validate_payload",
            "validate_provider_id",
        ):
            self.assertIs(getattr(vault_module, name), getattr(records_module, name))


if __name__ == "__main__":
    unittest.main()
