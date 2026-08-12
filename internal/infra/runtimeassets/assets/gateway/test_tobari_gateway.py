import copy
import json
import os
import tempfile
import unittest
from unittest import mock

from mitmproxy import http
from mitmproxy.test import tflow

import broker_credentials as broker
import tobari_gateway as gateway


CONTEXT = "01912345-6789-7abc-8def-0123456789ad"
PROJECT = "01912345-6789-7abc-8def-0123456789ab"
HANDLE = "tobari-h1_" + "A" * 43
REVISION = "revision_static"


def projection():
    provider = {
        "schema_version": 1,
        "id": "github",
        "display_name": "GitHub.com",
        "acquisition": {"mode": "builtin_helper", "helper": "github-gh"},
        "credential": {"kind": "primary_secret"},
        "workspace_projections": [
            {"kind": "env", "name": "GH_HOST", "template": "github.com"},
            {"kind": "env", "name": "GH_TOKEN", "template": "${HANDLE}"},
        ],
        "header_bindings": [{
            "target": {"scheme": "https", "host": "api.github.com", "port": 443},
            "source": {"header": "authorization", "formats": ["bearer", "token"]},
            "destination": {"header": "authorization", "format": "preserve_scheme", "secret_field": "primary_secret"},
            "secret_headers": ["authorization"],
        }],
    }
    bindings = [{
        "provider_id": "github",
        "target": {"scheme": "https", "host": "api.github.com", "port": 443},
        "source": {"header": "authorization", "format": source_format},
        "destination": {"header": "authorization", "format": "preserve_scheme", "secret_field": "primary_secret"},
        "secret_headers": ["authorization"],
    } for source_format in ("bearer", "token")]
    return {
        "schema_version": 1,
        "providers": [provider],
        "environment": [
            {"provider_id": "github", "name": "GH_HOST", "template": "github.com"},
            {"provider_id": "github", "name": "GH_TOKEN", "template": "${HANDLE}"},
        ],
        "complete_files": [],
        "header_bindings": bindings,
        "secret_headers": ["authorization"],
    }


def metadata(binding, *, secret=None):
    result = {
        "schema_version": 1,
        "ok": True,
        "provider": "github",
        "revision": REVISION,
        "target": binding["target"],
        "source": binding["source"],
        "destination": binding["destination"],
        "secret_headers": binding["secret_headers"],
    }
    if secret is not None:
        import base64
        result["secret"] = {
            "field": "primary_secret",
            "encoding": "base64url",
            "value": base64.urlsafe_b64encode(secret).rstrip(b"=").decode("ascii"),
        }
    return result


class StaticBrokerGatewayTests(unittest.TestCase):
    def request(self, url="https://api.github.com/repos/openai/openai", value=None):
        request = http.Request.make("GET", url)
        if value is not None:
            request.headers["authorization"] = value
        return request

    def test_projection_accepts_static_plan_and_rejects_retired_shapes(self):
        self.assertEqual(broker.validate_provider_projection(projection()), projection())
        for mutation in (
            lambda value: value.update(signing_bindings=[]),
            lambda value: value["providers"][0]["credential"].update(kind="openai_codex_oauth_session"),
            lambda value: value["providers"][0]["acquisition"].update(helper="aws-sso"),
            lambda value: value["providers"][0].update(refresh={"kind": "oauth"}),
        ):
            value = copy.deepcopy(projection())
            mutation(value)
            with self.assertRaises(broker.BrokerCredentialUnavailable):
                broker.validate_provider_projection(value)

    def test_handle_is_removed_before_introspection_and_resolved_once_after_allow(self):
        calls = []
        binding = projection()["header_bindings"][0]

        def caller(_path, request, _timeout):
            calls.append(copy.deepcopy(request))
            self.assertNotIn("authorization", outbound.headers)
            if request["op"] == "introspect":
                return metadata(binding)
            if request["op"] == "resolve":
                return metadata(binding, secret=b"real-token")
            self.fail(request)

        outbound = self.request(value=f"Bearer {HANDLE}")
        adapter = broker.BrokeredCredentialAdapter(
            fallback=mock.Mock(), projection_path="/projection.json",
            socket_path="/broker.sock", timeout=2,
            projection_loader=lambda _: projection(), caller=caller,
        )
        prepared = adapter.prepare(outbound, "https", "api.github.com", 443, CONTEXT, PROJECT)
        self.assertEqual([item["op"] for item in calls], ["introspect"])
        self.assertNotIn("authorization", outbound.headers)
        prepared.apply(outbound)
        self.assertEqual([item["op"] for item in calls], ["introspect", "resolve"])
        self.assertEqual(outbound.headers["authorization"], "Bearer real-token")

    def test_invalid_handle_never_falls_back(self):
        fallback = mock.Mock()
        adapter = broker.BrokeredCredentialAdapter(
            fallback=fallback, projection_path="/projection.json",
            socket_path="/broker.sock", timeout=2,
            projection_loader=lambda _: projection(), caller=mock.Mock(),
        )
        request = self.request("https://example.com/", f"Bearer {HANDLE}")
        with self.assertRaises(broker.BrokerCredentialBindingError):
            adapter.prepare(request, "https", "example.com", 443, CONTEXT, PROJECT)
        fallback.prepare.assert_not_called()
        self.assertNotIn("authorization", request.headers)

    def test_retired_profile_selector_fails_closed_before_broker_or_fallback(self):
        fallback = mock.Mock()
        caller = mock.Mock()
        adapter = broker.BrokeredCredentialAdapter(
            fallback=fallback, projection_path="/projection.json",
            socket_path="/broker.sock", timeout=2,
            projection_loader=lambda _: projection(), caller=caller,
        )
        request = self.request(value=f"Bearer {HANDLE}")
        request.headers["x-tobari-credential-profile"] = "legacy"
        with self.assertRaises(broker.BrokerCredentialBindingError):
            adapter.prepare(request, "https", "api.github.com", 443, CONTEXT, PROJECT)
        self.assertNotIn("x-tobari-credential-profile", request.headers)
        caller.assert_not_called()
        fallback.prepare.assert_not_called()

    def test_terminal_policy_denial_never_resolves_or_commits_upstream(self):
        with tempfile.TemporaryDirectory() as temporary:
            provider_path = os.path.join(temporary, "providers.json")
            principal_path = os.path.join(temporary, "principals.json")
            with open(provider_path, "w", encoding="utf-8") as handle:
                json.dump(projection(), handle)
            with open(principal_path, "w", encoding="utf-8") as handle:
                json.dump({"schema_version": 1, "projects": []}, handle)
            os.chmod(provider_path, 0o600)
            os.chmod(principal_path, 0o600)
            with mock.patch.dict(os.environ, {
                "TOBARI_AUTH_PROVIDER_PROJECTION": provider_path,
                "TOBARI_PRINCIPAL_REGISTRY": principal_path,
            }, clear=False):
                addon = gateway.TobariGateway()
            addon.graphql_config = {}
            flow = tflow.tflow(req=self.request(value=f"Bearer {HANDLE}"))
            binding = projection()["header_bindings"][0]
            calls = []

            def broker_call(_path, request, _timeout):
                calls.append(request["op"])
                return metadata(binding)

            addon.credential_adapter.caller = broker_call
            with (
                mock.patch.object(gateway, "normalize_ingress_authority", return_value=("https", "api.github.com", 443)),
                mock.patch.object(gateway, "load_project_principals", return_value={}),
                mock.patch.object(gateway, "graphql_endpoint_declared", return_value=False),
                mock.patch.object(gateway, "resolve_project_principal", return_value={
                    "project_id": PROJECT, "context_id": CONTEXT,
                    "context": "default", "project_root": "/workspace/project",
                }),
                mock.patch.object(gateway, "query_opa", return_value=gateway.Decision(
                    allow=False, reason="terminal", status_code=403,
                    learnable=False,
                )),
                mock.patch.object(gateway, "commit_upstream_authority") as commit,
            ):
                addon.requestheaders(flow)
            self.assertEqual(calls, ["introspect"], flow.response.content)
            commit.assert_not_called()
            self.assertEqual(flow.response.status_code, 403)
            self.assertNotIn("authorization", flow.request.headers)

    def test_broker_error_maps_only_static_binding_failures(self):
        with self.assertRaises(broker.BrokerCredentialBindingError):
            broker._broker_response(json.dumps({
                "schema_version": 1, "ok": False,
                "error": {"code": "handle_revoked"},
            }).encode())
        for retired in ("companion_outcome_unknown", "aws_signing_request_invalid"):
            with self.subTest(retired=retired):
                with self.assertRaises(broker.BrokerCredentialUnavailable):
                    broker._broker_response(json.dumps({
                        "schema_version": 1, "ok": False, "error": {"code": retired},
                    }).encode())


if __name__ == "__main__":
    unittest.main()
