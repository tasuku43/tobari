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
    project_a = "01912345-6789-7abc-8def-0123456789ab"
    project_b = "01912345-6789-7abc-8def-0123456789ac"
    context_a = "01912345-6789-7abc-8def-0123456789ad"
    context_b = "01912345-6789-7abc-8def-0123456789ae"

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.secret = os.path.join(self.temp.name, "secret")
        with open(self.secret, "w", encoding="utf-8") as handle:
            handle.write("example-token\n")
        self.principal_a = {"project_id": self.project_a, "context_id": self.context_a, "context": "default", "project_root": "/workspace/project-a"}
        self.principal_b = {"project_id": self.project_b, "context_id": self.context_b, "context": "restricted", "project_root": "/workspace/project-b"}
        self.config = {
            "version": "v2",
            "contexts": {self.context_a: {"name": "default", "profiles": {
                "example": {
                    "type": "bearer", "hosts": ["api.example.com"],
                    "projects": [self.project_a],
                    "secret_file": f"/run/tobari/credentials/{self.context_a}/example",
                }
            }}},
        }
        self.principal_path = os.path.join(self.temp.name, "principals.json")
        with open(self.principal_path, "w", encoding="utf-8") as handle:
            json.dump(
                {
                    "schema_version": 2,
                    "bindings": [
                        {
                            "project_id": self.project_a,
                            "context_id": self.context_a,
                            "context": "default",
                            "project_root": "/workspace/project-a",
                            "gateway_ip": "172.29.0.2",
                            "network": "tobari-project-a-net",
                        }
                    ],
                },
                handle,
            )
        self.previous_principal_path = os.environ.get("TOBARI_PRINCIPAL_REGISTRY")
        os.environ["TOBARI_PRINCIPAL_REGISTRY"] = self.principal_path
        self.addCleanup(self.restore_principal_path)

    def restore_principal_path(self):
        if self.previous_principal_path is None:
            os.environ.pop("TOBARI_PRINCIPAL_REGISTRY", None)
        else:
            os.environ["TOBARI_PRINCIPAL_REGISTRY"] = self.previous_principal_path

    def managed_gateway(self):
        previous = os.environ.get("TOBARI_CREDENTIAL_ADAPTER")
        os.environ["TOBARI_CREDENTIAL_ADAPTER"] = "managed"

        def restore():
            if previous is None:
                os.environ.pop("TOBARI_CREDENTIAL_ADAPTER", None)
            else:
                os.environ["TOBARI_CREDENTIAL_ADAPTER"] = previous

        self.addCleanup(restore)
        return gateway.TobariGateway()

    def flow(self, url="https://api.example.com/v1/resources?key=value", method="POST"):
        flow = tflow.tflow()
        flow.request = http.Request.make(method, url, b'{"example":true}', {
            "content-type": "application/json",
            "authorization": "Tobari supplied secret",
            "cookie": "session=secret",
            "x-safe": "visible",
        })
        flow.client_conn = SimpleNamespace(sockname=("172.29.0.2", 8080))
        return flow

    def test_project_principal_comes_from_gateway_local_network_address(self):
        flow = self.flow()
        flow.client_conn = SimpleNamespace(sockname=("172.29.0.2", 8080))
        principals = {"172.29.0.2": self.principal_a}
        self.assertEqual(
            gateway.resolve_project_principal(flow, principals), self.principal_a
        )

    def test_project_principal_does_not_follow_forged_session_header(self):
        flow = self.flow()
        flow.request.headers["x-tobari-session"] = self.project_a
        flow.request.headers["x-tobari-context"] = self.context_a
        flow.request.headers["x-tobari-project-id"] = self.project_a
        flow.client_conn = SimpleNamespace(sockname=("172.29.1.2", 8080))
        principals = {"172.29.1.2": self.principal_b}
        self.assertEqual(
            gateway.resolve_project_principal(flow, principals), self.principal_b
        )
        policy_input = gateway.build_policy_input(flow, "default", self.principal_b, set())
        self.assertEqual(policy_input["principal"]["context_id"], self.context_b)
        self.assertEqual(policy_input["principal"]["project_id"], self.project_b)

    def test_unknown_project_principal_is_denied_before_opa(self):
        flow = self.flow()
        flow.client_conn = SimpleNamespace(sockname=("172.29.9.2", 8080))
        addon = gateway.TobariGateway()
        output = io.StringIO()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(gateway, "load_project_principals", return_value={}):
                with mock.patch.object(gateway, "query_opa") as query:
                    with redirect_stdout(output):
                        addon.requestheaders(flow)
        query.assert_not_called()
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(
            json.loads(flow.response.content), {"error": "project_principal_unavailable"}
        )

    def test_policy_input_is_generic_and_redacted(self):
        flow = self.flow()
        document = gateway.build_policy_input(flow, "default", self.principal_a, set())
        self.assertEqual(document["principal"]["cluster"], "default")
        self.assertEqual(document["principal"]["project_id"], self.project_a)
        self.assertNotIn("session", document["principal"])
        request = document["request"]
        self.assertEqual(document["schema_version"], 4)
        self.assertEqual(document["principal"]["context_id"], self.context_a)
        self.assertEqual(request["authority"]["scheme"], "https")
        self.assertEqual(request["authority"]["host"], "api.example.com")
        self.assertEqual(request["authority"]["port"], 443)
        self.assertEqual(request["method"], "POST")
        self.assertEqual(request["path"]["raw"], "/v1/resources")
        self.assertEqual(request["path"]["segments"], ["v1", "resources"])
        self.assertEqual(request["query"], {"key": ["value"]})
        self.assertNotIn("authorization", request["headers"])
        self.assertNotIn("cookie", request["headers"])
        self.assertNotIn("body", request)

    def test_passthrough_forwards_client_auth_only_after_allow(self):
        flow = self.flow()
        flow.request.headers["x-api-key"] = "api-secret"
        flow.request.headers["x-auth-token"] = "token-secret"
        flow.request.headers["proxy-authorization"] = "proxy-secret"
        flow.request.headers["x-tobari-session"] = self.project_a
        addon = gateway.TobariGateway()
        with mock.patch.object(
            gateway,
            "load_credential_config",
            side_effect=AssertionError("passthrough loaded managed credentials"),
        ):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(True, "allowed", None, 403, False),
            ):
                with redirect_stdout(io.StringIO()):
                    addon.requestheaders(flow)
        self.assertEqual(addon.credential_adapter_name, "passthrough")
        self.assertEqual(flow.request.headers["authorization"], "Tobari supplied secret")
        self.assertEqual(flow.request.headers["x-api-key"], "api-secret")
        self.assertEqual(flow.request.headers["x-auth-token"], "token-secret")
        self.assertEqual(flow.request.headers["cookie"], "session=secret")
        self.assertNotIn("proxy-authorization", flow.request.headers)
        self.assertNotIn("x-tobari-session", flow.request.headers)
        self.assertTrue(flow.request.stream)

    def test_passthrough_does_not_interpret_profile_selector(self):
        flow = self.flow()
        flow.request.headers["x-tobari-credential-profile"] = "example"
        document = gateway.build_policy_input(
            flow, "default", self.principal_a, set(), None
        )
        self.assertIsNone(document["authorization"]["requested_profile"])
        self.assertNotIn("x-tobari-credential-profile", document["request"]["headers"])

    def test_intercepted_connect_request_uses_https_scheme_for_policy(self):
        flow = self.flow("http://example.com:443/quickstart", "PUT")
        flow.request.raw_content = b""
        flow.client_conn = SimpleNamespace(
            sockname=("172.29.0.2", 8080),
            tls_established=True,
        )
        document = gateway.build_policy_input(
            flow, "default", self.principal_a, set(), None
        )
        self.assertEqual(document["request"]["authority"]["scheme"], "https")
        self.assertEqual(document["request"]["authority"]["port"], 443)
        self.assertNotIn("body", document["request"])

    def test_plain_http_on_port_443_keeps_http_scheme_for_policy(self):
        flow = self.flow("http://example.com:443/quickstart", "PUT")
        flow.request.raw_content = b""
        flow.client_conn = SimpleNamespace(
            sockname=("172.29.0.2", 8080),
            tls_established=False,
        )
        document = gateway.build_policy_input(
            flow, "default", self.principal_a, set(), None
        )
        self.assertEqual(document["request"]["authority"]["scheme"], "http")

    def test_unknown_credential_adapter_fails_closed_at_construction(self):
        with mock.patch.dict(
            os.environ, {"TOBARI_CREDENTIAL_ADAPTER": "unknown"}, clear=False
        ):
            with self.assertRaises(gateway.CredentialAdapterError):
                gateway.TobariGateway()

    def test_policy_input_redacts_generic_secret_shaped_headers(self):
        flow = self.flow()
        flow.request.headers["x-auth-token"] = "token-secret"
        flow.request.headers["x-safe"] = "visible"
        document = gateway.build_policy_input(
            flow, "default", self.principal_a, set(), None
        )
        self.assertNotIn("x-auth-token", document["request"]["headers"])
        self.assertEqual(document["request"]["headers"]["x-safe"], "visible")

    def test_policy_input_is_identical_for_different_body_content(self):
        flow = self.flow()
        first = gateway.build_policy_input(flow, "default", self.principal_a, set())
        flow.request.raw_content = None
        second = gateway.build_policy_input(flow, "default", self.principal_a, set())
        self.assertEqual(first, second)
        self.assertNotIn("body", first["request"])

    def test_allowed_request_streams_body_after_policy_allow(self):
        flow = self.flow()
        flow.request.raw_content = None
        addon = gateway.TobariGateway()

        def allow_without_body(policy_url, policy_input, timeout):
            self.assertNotIn("body", policy_input["request"])
            self.assertFalse(flow.request.stream)
            return gateway.Decision(True, "allowed", None, 403, False)

        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                side_effect=allow_without_body,
            ):
                with redirect_stdout(io.StringIO()):
                    addon.requestheaders(flow)
        self.assertIsNone(flow.response)
        self.assertTrue(flow.request.stream)

    def test_denied_request_does_not_stream_body(self):
        flow = self.flow()
        addon = gateway.TobariGateway()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(False, "denied", None, 403, True),
            ):
                with redirect_stdout(io.StringIO()):
                    addon.requestheaders(flow)
        self.assertEqual(flow.response.status_code, 403)
        self.assertFalse(flow.request.stream)

    def test_authorized_upstream_response_is_streamed(self):
        flow = self.flow()
        flow.response = http.Response.make(200, b"response")
        flow.metadata["tobari_audit"] = {"started": 0.0}
        addon = gateway.TobariGateway()
        addon.responseheaders(flow)
        self.assertTrue(flow.response.stream)

    def test_local_response_is_not_streamed(self):
        flow = self.flow()
        flow.response = http.Response.make(403, b"denied")
        addon = gateway.TobariGateway()
        addon.responseheaders(flow)
        self.assertFalse(flow.response.stream)

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
        with self.assertRaises(gateway.PolicyUnavailable):
            gateway._parse_decision(
                {
                    "result": {
                        "allow": False,
                        "reason": "denied",
                        "credential_profile": None,
                        "status_code": 403,
                    }
                }
            )

    def test_credential_is_host_bound(self):
        request = self.flow().request
        with mock.patch("builtins.open", return_value=io.BytesIO(b"example-token\n")):
            gateway.inject_credential(
                request, self.config, "example", "api.example.com", self.context_a, self.project_a
            )
        self.assertEqual(request.headers["authorization"], "Bearer example-token")
        with self.assertRaises(gateway.CredentialError):
            gateway.inject_credential(
                request, self.config, "example", "other.example.com", self.context_a, self.project_a
            )

    def test_credential_path_cannot_escape_gateway_directory(self):
        request = self.flow().request
        escaped = json.loads(json.dumps(self.config))
        escaped["contexts"][self.context_a]["profiles"]["example"]["secret_file"] = (
            "/run/tobari/credentials/../config/credentials.json"
        )
        with self.assertRaises(gateway.CredentialError):
            gateway.inject_credential(
                request, escaped, "example", "api.example.com", self.context_a, self.project_a
            )

    def test_credential_profile_does_not_cross_project(self):
        flow = self.flow()
        flow.client_conn = SimpleNamespace(sockname=("172.29.1.2", 8080))
        flow.request.headers["x-tobari-credential-profile"] = "example"
        addon = self.managed_gateway()
        output = io.StringIO()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway, "load_project_principals", return_value={"172.29.1.2": self.principal_b}
            ):
                with mock.patch.object(gateway, "query_opa") as query:
                    with redirect_stdout(output):
                        addon.requestheaders(flow)
        query.assert_not_called()
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(
            json.loads(flow.response.content), {"error": "credential_profile_not_bound"}
        )

    def test_same_profile_name_is_strictly_context_scoped(self):
        config = json.loads(json.dumps(self.config))
        config["contexts"][self.context_b] = {
            "name": "restricted",
            "profiles": {
                "example": {
                    "type": "bearer",
                    "hosts": ["api.example.com"],
                    "projects": [self.project_b],
                    "secret_file": f"/run/tobari/credentials/{self.context_b}/example",
                }
            },
        }
        with self.assertRaises(gateway.CredentialBindingError):
            gateway._profile_project_binding(
                config, "example", self.context_a, self.project_b
            )
        with self.assertRaises(gateway.CredentialBindingError):
            gateway._profile_project_binding(
                config, "example", self.context_b, self.project_a
            )
        selected = gateway._profile_project_binding(
            config, "example", self.context_b, self.project_b
        )
        self.assertEqual(
            selected["secret_file"],
            f"/run/tobari/credentials/{self.context_b}/example",
        )

    def test_managed_adapter_injects_only_after_allow(self):
        flow = self.flow()
        flow.request.headers["x-tobari-credential-profile"] = "example"
        flow.request.headers["x-tobari-session"] = self.project_a
        flow.request.headers["proxy-authorization"] = "proxy-secret"
        addon = self.managed_gateway()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "load_project_principals",
                return_value={"172.29.0.2": self.principal_a},
            ):
                with mock.patch.object(
                    gateway,
                    "query_opa",
                    return_value=gateway.Decision(True, "allowed", "example", 403, False),
                ):
                    with mock.patch(
                        "builtins.open", return_value=io.BytesIO(b"example-token\n")
                    ):
                        with redirect_stdout(io.StringIO()):
                            addon.requestheaders(flow)
        self.assertEqual(flow.request.headers["authorization"], "Bearer example-token")
        self.assertNotIn("x-tobari-credential-profile", flow.request.headers)
        self.assertNotIn("x-tobari-session", flow.request.headers)
        self.assertNotIn("proxy-authorization", flow.request.headers)

    def test_credential_config_requires_project_bindings(self):
        config = json.loads(json.dumps(self.config))
        del config["contexts"][self.context_a]["profiles"]["example"]["projects"]
        with self.assertRaises(gateway.CredentialError):
            with mock.patch("builtins.open", return_value=io.BytesIO(json.dumps(config).encode())):
                gateway.load_credential_config("ignored")

    def test_deny_and_audit_never_include_secrets_or_body(self):
        flow = self.flow()
        addon = self.managed_gateway()
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
                    addon.requestheaders(flow)
        self.assertEqual(flow.response.status_code, 403)
        rendered = output.getvalue()
        self.assertNotIn("Tobari supplied secret", rendered)
        self.assertNotIn("session=secret", rendered)
        self.assertNotIn("example-token", rendered)
        self.assertNotIn('{"example":true}', rendered)
        audit = json.loads(rendered)
        self.assertEqual(audit["schema_version"], 2)
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

    def test_learnable_policy_denial_guides_agent_to_host_review_without_secrets(self):
        flow = self.flow()
        addon = self.managed_gateway()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(False, "denied", "example", 403, True),
            ):
                with redirect_stdout(io.StringIO()):
                    addon.requestheaders(flow)

        document = json.loads(flow.response.content)
        self.assertEqual(document["error"], "policy_denied")
        self.assertIn("Leave the Workspace with `exit`", document["message"])
        self.assertEqual(document["tobari"]["schema_version"], 1)
        self.assertEqual(document["tobari"]["event"], "permission_review_available")
        self.assertEqual(document["tobari"]["run_on"], "host")
        self.assertEqual(
            document["tobari"]["review"],
            {
                "available": True,
                "command": "tobari policy review",
                "automatic_retry": False,
                "retry_after_review": True,
            },
        )
        self.assertEqual(
            document["tobari"]["request"],
            {
                "host": "api.example.com",
                "port": 443,
                "method": "POST",
                "path": "/v1/resources",
            },
        )
        rendered = flow.response.content.decode("utf-8")
        self.assertNotIn("Tobari supplied secret", rendered)
        self.assertNotIn("session=secret", rendered)
        self.assertNotIn("example-token", rendered)
        self.assertNotIn('{"example":true}', rendered)
        self.assertNotIn("key=value", rendered)

    def test_non_learnable_policy_denial_does_not_invite_approval(self):
        flow = self.flow()
        addon = self.managed_gateway()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(False, "denied", "example", 403, False),
            ):
                with redirect_stdout(io.StringIO()):
                    addon.requestheaders(flow)

        document = json.loads(flow.response.content)
        self.assertEqual(document["tobari"]["event"], "permission_review_unavailable")
        self.assertEqual(document["tobari"]["review"]["available"], False)
        self.assertIsNone(document["tobari"]["review"]["command"])
        self.assertFalse(document["tobari"]["review"]["retry_after_review"])

    def test_opa_outage_returns_503_without_forwarding(self):
        flow = self.flow()
        addon = self.managed_gateway()
        output = io.StringIO()
        with mock.patch.object(gateway, "load_credential_config", return_value=self.config):
            with mock.patch.object(
                gateway,
                "query_opa",
                side_effect=gateway.PolicyUnavailable("OPA request failed"),
            ):
                with redirect_stdout(output):
                    addon.requestheaders(flow)
        self.assertEqual(flow.response.status_code, 503)
        audit = json.loads(output.getvalue())
        self.assertEqual(audit["reason"], "OPA request failed")
        self.assertFalse(audit["learnable"])

    def test_unexpected_gateway_error_fails_closed(self):
        flow = self.flow()
        addon = self.managed_gateway()
        with mock.patch.object(
            gateway,
            "load_credential_config",
            side_effect=ValueError("private unexpected failure"),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual(flow.response.status_code, 502)
        self.assertEqual(json.loads(flow.response.content), {"error": "gateway_error"})


if __name__ == "__main__":
    unittest.main()
