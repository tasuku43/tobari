import hashlib
import io
import json
import os
import tempfile
import unittest
from contextlib import redirect_stdout
from types import SimpleNamespace
from unittest import mock

from mitmproxy import http
from mitmproxy.test import tflow

import tobari_gateway as gateway


class GatewayTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.secret = os.path.join(self.temp.name, "secret")
        with open(self.secret, "w", encoding="utf-8") as handle:
            handle.write("example-token\n")
        self.config = {
            "version": "v1",
            "profiles": {
                "example": {
                    "type": "bearer",
                    "hosts": ["api.example.com"],
                    "secret_file": "/run/tobari/credentials/example",
                }
            },
        }

    def flow(self, url="https://api.example.com/v1/resources?key=value", method="POST"):
        flow = tflow.tflow()
        flow.request = http.Request.make(method, url, b'{"example":true}', {
            "content-type": "application/json",
            "authorization": "Tobari supplied secret",
            "cookie": "session=secret",
            "x-safe": "visible",
        })
        return flow

    def test_policy_input_is_generic_and_redacted(self):
        flow = self.flow()
        document = gateway.build_policy_input(flow, "default", None, 1024, set())
        self.assertEqual(document["principal"]["cluster"], "default")
        request = document["request"]
        self.assertEqual(document["version"], "v1")
        self.assertEqual(request["host"], "api.example.com")
        self.assertEqual(request["port"], 443)
        self.assertEqual(request["method"], "POST")
        self.assertEqual(request["path_segments"], ["v1", "resources"])
        self.assertEqual(request["query"], {"key": ["value"]})
        self.assertNotIn("authorization", request["headers"])
        self.assertNotIn("cookie", request["headers"])
        self.assertEqual(request["body"]["kind"], "json")
        self.assertEqual(request["body"]["value"], {"example": True})
        self.assertEqual(
            request["body"]["sha256"],
            hashlib.sha256(b'{"example":true}').hexdigest(),
        )

    def test_missing_streamed_body_is_not_treated_as_empty(self):
        flow = self.flow()
        flow.request.raw_content = None
        document = gateway.build_policy_input(flow, "default", None, 1024, set())
        body = document["request"]["body"]
        self.assertEqual(body["kind"], "unavailable")
        self.assertIsNone(body["size"])
        self.assertIsNone(body["truncated"])
        self.assertIsNone(body["sha256"])

    def test_unavailable_body_is_denied_before_policy_can_allow(self):
        flow = self.flow()
        flow.request.raw_content = None
        addon = gateway.TobariGateway()
        output = io.StringIO()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(True, "allowed", None, 403, False),
            ) as query:
                with redirect_stdout(output):
                    addon.request(flow)
        query.assert_not_called()
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(json.loads(flow.response.content), {"error": "request_body_unavailable"})

    def test_resolved_private_address_is_rejected_for_dotted_host(self):
        with mock.patch.object(
            gateway.socket,
            "getaddrinfo",
            return_value=[(2, 1, 6, "", ("192.168.1.10", 443))],
        ):
            with self.assertRaises(gateway.UpstreamAddressError):
                gateway.resolve_upstream_address("api.example.com", 443)

    def test_resolved_single_label_private_address_is_pinned(self):
        with mock.patch.object(
            gateway.socket,
            "getaddrinfo",
            return_value=[(2, 1, 6, "", ("172.20.0.4", 8080))],
        ):
            self.assertEqual(
                gateway.resolve_upstream_address("mock-upstream", 8080),
                ("172.20.0.4", 8080),
            )

    def test_server_connect_replaces_hostname_with_resolved_address(self):
        server = SimpleNamespace(address=("api.example.com", 443), error=None)
        data = SimpleNamespace(server=server)
        addon = gateway.TobariGateway()
        with mock.patch.object(
            gateway,
            "resolve_upstream_address",
            return_value=("93.184.216.34", 443),
        ):
            addon.server_connect(data)
        self.assertEqual(server.address, ("93.184.216.34", 443))
        self.assertIsNone(server.error)

    def test_oversized_json_is_metadata_only(self):
        body = gateway._body_metadata(b'{"secret":"body"}', "application/json", 4)
        self.assertTrue(body["truncated"])
        self.assertEqual(body["kind"], "metadata")
        self.assertNotIn("value", body)

    def test_invalid_opa_response_fails_closed(self):
        for document in (
            {},
            {"result": {}},
            {"result": {"allow": "yes", "reason": "x"}},
            {
                "result": {
                    "allow": False,
                    "reason": "denied",
                    "credential_profile": None,
                    "status_code": 403,
                    "learnable": "yes",
                }
            },
        ):
            with self.subTest(document=document):
                with self.assertRaises(gateway.PolicyUnavailable):
                    gateway._parse_decision(document)
        decision = gateway._parse_decision(
            {
                "result": {
                    "allow": False,
                    "reason": "denied",
                    "credential_profile": None,
                    "status_code": 403,
                    "learnable": True,
                }
            }
        )
        self.assertTrue(decision.learnable)
        legacy = gateway._parse_decision(
            {
                "result": {
                    "allow": False,
                    "reason": "denied",
                    "credential_profile": None,
                    "status_code": 403,
                }
            }
        )
        self.assertFalse(legacy.learnable)

    def test_credential_is_host_bound(self):
        request = self.flow().request
        with mock.patch("builtins.open", return_value=io.BytesIO(b"example-token\n")):
            gateway.inject_credential(request, self.config, "example", "api.example.com")
        self.assertEqual(request.headers["authorization"], "Bearer example-token")
        with self.assertRaises(gateway.CredentialError):
            gateway.inject_credential(request, self.config, "example", "other.example.com")

    def test_credential_path_cannot_escape_gateway_directory(self):
        request = self.flow().request
        escaped = json.loads(json.dumps(self.config))
        escaped["profiles"]["example"]["secret_file"] = (
            "/run/tobari/credentials/../config/credentials.json"
        )
        with self.assertRaises(gateway.CredentialError):
            gateway.inject_credential(request, escaped, "example", "api.example.com")

    def test_deny_and_audit_never_include_secrets_or_body(self):
        flow = self.flow()
        addon = gateway.TobariGateway()
        output = io.StringIO()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(
                    False, "denied", "example", 403, True
                ),
            ):
                with redirect_stdout(output):
                    addon.request(flow)
        self.assertEqual(flow.response.status_code, 403)
        rendered = output.getvalue()
        self.assertNotIn("Tobari supplied secret", rendered)
        self.assertNotIn("session=secret", rendered)
        self.assertNotIn("example-token", rendered)
        self.assertNotIn('{"example":true}', rendered)
        audit = json.loads(rendered)
        self.assertEqual(audit["cluster"], "default")
        self.assertNotIn("realm", audit)
        self.assertEqual(audit["decision"], "deny")
        self.assertEqual(audit["host"], "api.example.com")
        self.assertEqual(audit["port"], 443)
        self.assertEqual(audit["method"], "POST")
        self.assertEqual(audit["path"], "/v1/resources")
        self.assertEqual(audit["reason"], "denied")
        self.assertTrue(audit["learnable"])
        self.assertEqual(audit["credential_profile"], "example")

    def test_opa_outage_returns_503_without_forwarding(self):
        flow = self.flow()
        addon = gateway.TobariGateway()
        output = io.StringIO()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                side_effect=gateway.PolicyUnavailable("OPA request failed"),
            ):
                with redirect_stdout(output):
                    addon.request(flow)
        self.assertEqual(flow.response.status_code, 503)
        audit = json.loads(output.getvalue())
        self.assertEqual(audit["reason"], "OPA request failed")
        self.assertFalse(audit["learnable"])

    def test_unexpected_gateway_error_fails_closed(self):
        flow = self.flow()
        addon = gateway.TobariGateway()
        with mock.patch.object(
            gateway,
            "load_credential_config",
            side_effect=ValueError("private unexpected failure"),
        ):
            with redirect_stdout(io.StringIO()):
                addon.request(flow)
        self.assertEqual(flow.response.status_code, 502)
        self.assertEqual(json.loads(flow.response.content), {"error": "gateway_error"})


if __name__ == "__main__":
    unittest.main()
