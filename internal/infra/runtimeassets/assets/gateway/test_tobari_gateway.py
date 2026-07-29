import hashlib
import io
import json
import os
import tempfile
import unittest
from contextlib import redirect_stdout
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
            "authorization": "Realm supplied secret",
            "cookie": "session=secret",
            "x-safe": "visible",
        })
        return flow

    def test_policy_input_is_generic_and_redacted(self):
        flow = self.flow()
        document = gateway.build_policy_input(flow, "default", None, 1024, set())
        request = document["request"]
        self.assertEqual(document["version"], "v1")
        self.assertEqual(request["host"], "api.example.com")
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

    def test_oversized_json_is_metadata_only(self):
        body = gateway._body_metadata(b'{"secret":"body"}', "application/json", 4)
        self.assertTrue(body["truncated"])
        self.assertEqual(body["kind"], "metadata")
        self.assertNotIn("value", body)

    def test_invalid_opa_response_fails_closed(self):
        for document in ({}, {"result": {}}, {"result": {"allow": "yes", "reason": "x"}}):
            with self.subTest(document=document):
                with self.assertRaises(gateway.PolicyUnavailable):
                    gateway._parse_decision(document)

    def test_credential_is_host_bound(self):
        request = self.flow().request
        with mock.patch("builtins.open", return_value=io.BytesIO(b"example-token\n")):
            gateway.inject_credential(request, self.config, "example", "api.example.com")
        self.assertEqual(request.headers["authorization"], "Bearer example-token")
        with self.assertRaises(gateway.CredentialError):
            gateway.inject_credential(request, self.config, "example", "other.example.com")

    def test_deny_and_audit_never_include_secrets_or_body(self):
        flow = self.flow()
        addon = gateway.TobariGateway()
        output = io.StringIO()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(False, "denied", None, 403),
            ):
                with redirect_stdout(output):
                    addon.request(flow)
        self.assertEqual(flow.response.status_code, 403)
        rendered = output.getvalue()
        self.assertNotIn("Realm supplied secret", rendered)
        self.assertNotIn("session=secret", rendered)
        self.assertNotIn("example-token", rendered)
        self.assertNotIn('{"example":true}', rendered)
        audit = json.loads(rendered)
        self.assertEqual(audit["decision"], "deny")

    def test_opa_outage_returns_503_without_forwarding(self):
        flow = self.flow()
        addon = gateway.TobariGateway()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                side_effect=gateway.PolicyUnavailable("down"),
            ):
                with redirect_stdout(io.StringIO()):
                    addon.request(flow)
        self.assertEqual(flow.response.status_code, 503)


if __name__ == "__main__":
    unittest.main()
