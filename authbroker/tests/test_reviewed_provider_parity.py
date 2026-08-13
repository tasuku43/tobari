from __future__ import annotations

import importlib.util
import json
from pathlib import Path
from types import ModuleType
from typing import Any
import unittest
from unittest import mock

from authbroker.control_login import (
    DriverControlLoginPlan,
    StaticControlLoginPlan,
    reviewed_control_login_plans,
)
from authbroker.credential_records import (
    STATIC_CREDENTIAL_KIND,
    reviewed_credential_record_contracts,
)
from authbroker.renewable import reviewed_renewable_session_adapters
from authbroker.request_signing import reviewed_request_signing_adapters


ROOT = Path(__file__).resolve().parents[2]
FIXTURE = Path(__file__).parent / "fixtures" / "reviewed_provider_capabilities_v1.json"
CONTROL_LOGIN_SHAPES = frozenset({"none", "static_secret", "driver_state"})
RUNTIME_CAPABILITIES = frozenset(
    {
        "static_secret_resolution",
        "renewable_bearer_session",
        "request_signing",
        "supplemental_header_application",
    }
)


def _load_gateway_profiles() -> ModuleType:
    path = ROOT / "gateway" / "addon" / "reviewed_credential_profiles.py"
    spec = importlib.util.spec_from_file_location(
        "reviewed_credential_profiles_for_parity", path
    )
    if spec is None or spec.loader is None:
        raise RuntimeError("cannot load reviewed Gateway credential profiles")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def _load_fixture() -> dict[str, Any]:
    document = json.loads(FIXTURE.read_text(encoding="utf-8"))
    if set(document) != {"schema_version", "reviewed_login_order", "providers"}:
        raise AssertionError("provider capability fixture has unexpected top-level keys")
    if document["schema_version"] != 1:
        raise AssertionError("provider capability fixture has unsupported schema")
    if not isinstance(document["reviewed_login_order"], list):
        raise AssertionError("reviewed_login_order must be a list")
    providers = document["providers"]
    if not isinstance(providers, list) or not providers:
        raise AssertionError("providers must be a non-empty list")
    expected_keys = {
        "provider_id",
        "host_acquisition",
        "manifest_credential_kind",
        "broker_control_login",
        "broker_record_kind",
        "broker_runtime_capabilities",
        "gateway_reviewed_profile",
    }
    provider_ids: list[str] = []
    for provider in providers:
        if not isinstance(provider, dict) or set(provider) != expected_keys:
            raise AssertionError("provider capability fixture entry has unexpected keys")
        provider_id = provider["provider_id"]
        if not isinstance(provider_id, str) or not provider_id:
            raise AssertionError("provider_id must be a non-empty string")
        provider_ids.append(provider_id)
        acquisition = provider["host_acquisition"]
        if not isinstance(acquisition, dict):
            raise AssertionError("host_acquisition must be an object")
        mode = acquisition.get("mode")
        expected_acquisition_keys = (
            {"mode", "helper"} if mode == "builtin_helper" else {"mode"}
        )
        if set(acquisition) != expected_acquisition_keys or mode not in {
            "builtin_helper",
            "stdin_import",
        }:
            raise AssertionError("host_acquisition is not a closed shape")
        if mode == "builtin_helper" and not isinstance(acquisition["helper"], str):
            raise AssertionError("builtin_helper requires one helper identifier")
        if provider["broker_control_login"] not in CONTROL_LOGIN_SHAPES:
            raise AssertionError("unknown Broker control-login shape")
        capabilities = provider["broker_runtime_capabilities"]
        if (
            not isinstance(capabilities, list)
            or not capabilities
            or len(capabilities) != len(set(capabilities))
            or not set(capabilities) <= RUNTIME_CAPABILITIES
        ):
            raise AssertionError("Broker runtime capabilities are not a closed set")
        if not isinstance(provider["gateway_reviewed_profile"], bool):
            raise AssertionError("gateway_reviewed_profile must be boolean")
    if provider_ids != sorted(provider_ids) or len(provider_ids) != len(set(provider_ids)):
        raise AssertionError("providers must be unique and ordered by ID")
    if set(document["reviewed_login_order"]) != {
        provider["provider_id"]
        for provider in providers
        if provider["broker_control_login"] != "none"
    }:
        raise AssertionError("reviewed login order does not match fixture membership")
    return document


class ReviewedProviderParityTests(unittest.TestCase):
    def test_fixture_matches_compiled_broker_and_gateway_unions(self) -> None:
        fixture = _load_fixture()
        providers = {entry["provider_id"]: entry for entry in fixture["providers"]}
        login_plans = reviewed_control_login_plans()
        record_contracts = reviewed_credential_record_contracts()
        renewable_adapters = reviewed_renewable_session_adapters()
        signing_adapters = reviewed_request_signing_adapters()
        gateway = _load_gateway_profiles()
        gateway_profiles = gateway.reviewed_gateway_credential_profiles()

        self.assertEqual(tuple(login_plans), tuple(fixture["reviewed_login_order"]))
        self.assertEqual(
            set(record_contracts),
            {entry["broker_record_kind"] for entry in providers.values()},
        )

        renewable_by_provider = {
            adapter.provider_id: adapter.credential_kind
            for adapter in renewable_adapters.values()
        }
        expected_renewable = {
            provider_id: entry["broker_record_kind"]
            for provider_id, entry in providers.items()
            if "renewable_bearer_session" in entry["broker_runtime_capabilities"]
        }
        self.assertEqual(renewable_by_provider, expected_renewable)

        signing_by_provider = {
            adapter.provider_id: adapter.credential_kind
            for adapter in signing_adapters.values()
        }
        expected_signing = {
            provider_id: entry["broker_record_kind"]
            for provider_id, entry in providers.items()
            if "request_signing" in entry["broker_runtime_capabilities"]
        }
        supplemental_providers = {
            provider_id
            for provider_id, entry in providers.items()
            if "supplemental_header_application"
            in entry["broker_runtime_capabilities"]
        }
        self.assertEqual(signing_by_provider, expected_signing)
        self.assertEqual(supplemental_providers, {"openai"})

        for provider_id, entry in providers.items():
            with self.subTest(provider=provider_id):
                manifest_kind = entry["manifest_credential_kind"]
                expected_record_kind = (
                    STATIC_CREDENTIAL_KIND
                    if manifest_kind == gateway.PRIMARY_SECRET_FIELD
                    else manifest_kind
                )
                self.assertEqual(entry["broker_record_kind"], expected_record_kind)

                login_shape = entry["broker_control_login"]
                plan = login_plans.get(provider_id)
                if login_shape == "none":
                    self.assertIsNone(plan)
                elif login_shape == "static_secret":
                    self.assertIsInstance(plan, StaticControlLoginPlan)
                    self.assertEqual(entry["broker_record_kind"], STATIC_CREDENTIAL_KIND)
                else:
                    self.assertIsInstance(plan, DriverControlLoginPlan)
                    self.assertEqual(plan.credential_kind, entry["broker_record_kind"])

                acquisition = entry["host_acquisition"]
                profile = gateway.reviewed_projection_profile(
                    provider_id,
                    manifest_kind,
                    acquisition.get("helper"),
                )
                if entry["gateway_reviewed_profile"]:
                    self.assertIsNotNone(profile)
                    self.assertEqual(profile.provider_id, provider_id)
                    self.assertEqual(profile.credential_kind, manifest_kind)
                else:
                    self.assertIsNone(profile)

                capabilities = set(entry["broker_runtime_capabilities"])
                self.assertEqual(
                    provider_id in renewable_by_provider,
                    "renewable_bearer_session" in capabilities,
                )
                self.assertEqual(
                    bool(profile and profile.supplemental_header_names),
                    "supplemental_header_application" in capabilities,
                )

        self.assertEqual(
            {profile.provider_id for profile in gateway_profiles.values()},
            {
                provider_id
                for provider_id, entry in providers.items()
                if entry["broker_record_kind"] != STATIC_CREDENTIAL_KIND
            },
        )

    def test_fixture_parser_rejects_unregistered_capability(self) -> None:
        document = json.loads(FIXTURE.read_text(encoding="utf-8"))
        document["providers"][0]["broker_runtime_capabilities"].append(
            "owner_selected_runtime_adapter"
        )
        with mock.patch.object(
            Path,
            "read_text",
            return_value=json.dumps(document),
        ):
            with self.assertRaisesRegex(AssertionError, "closed set"):
                _load_fixture()


if __name__ == "__main__":
    unittest.main()
