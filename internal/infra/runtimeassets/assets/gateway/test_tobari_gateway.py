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
from broker_credentials import (
    BrokerCredentialBindingError,
    BrokerCredentialUnavailable,
    BrokeredCredentialAdapter,
    validate_provider_projection,
)


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
        self.handle = "tobari-h1_" + "A" * 43
        self.real_token = "real-token-canary"
        self.provider_projection = self.github_provider_projection()
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

    @staticmethod
    def github_provider_projection():
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
            "header_bindings": [
                {
                    "target": {"scheme": "https", "host": "api.github.com", "port": 443},
                    "source": {"header": "authorization", "formats": ["bearer", "token"]},
                    "destination": {
                        "header": "authorization",
                        "format": "preserve_scheme",
                        "secret_field": "primary_secret",
                    },
                    "secret_headers": ["authorization"],
                }
            ],
        }
        bindings = []
        for source_format in ("bearer", "token"):
            bindings.append(
                {
                    "provider_id": "github",
                    "target": {"scheme": "https", "host": "api.github.com", "port": 443},
                    "source": {"header": "authorization", "format": source_format},
                    "destination": {
                        "header": "authorization",
                        "format": "preserve_scheme",
                        "secret_field": "primary_secret",
                    },
                    "secret_headers": ["authorization"],
                }
            )
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

    def broker_gateway(self, broker_call):
        addon = gateway.TobariGateway()
        addon.credential_adapter = BrokeredCredentialAdapter(
            addon.credential_adapter,
            "/run/tobari/auth/providers.json",
            "/run/tobari-auth/runtime/broker.sock",
            2.0,
            projection_loader=lambda _: self.provider_projection,
            caller=lambda _path, request, _timeout: broker_call(request),
        )
        return addon

    def broker_response(self, request):
        binding = self.provider_projection["header_bindings"][0]
        response = {
            "schema_version": 1,
            "ok": True,
            "provider": "github",
            "revision": "revision_example",
            "target": binding["target"],
            "source": binding["source"],
            "destination": binding["destination"],
            "secret_headers": binding["secret_headers"],
        }
        if request["op"] == "resolve":
            import base64

            response["secret"] = {
                "field": "primary_secret",
                "encoding": "base64url",
                "value": base64.urlsafe_b64encode(self.real_token.encode()).rstrip(b"=").decode(),
            }
        return response

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
        self.assertEqual(document["schema_version"], 5)
        self.assertEqual(
            document["authorization"],
            {"requested_profile": None, "broker_provider": None},
        )
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

    def test_provider_projection_is_strict_and_self_consistent(self):
        self.assertEqual(
            validate_provider_projection(self.provider_projection),
            self.provider_projection,
        )
        invalid_documents = []
        unknown = json.loads(json.dumps(self.provider_projection))
        unknown["providers"][0]["shell"] = "printenv"
        invalid_documents.append(unknown)
        inconsistent = json.loads(json.dumps(self.provider_projection))
        inconsistent["header_bindings"][0]["target"]["host"] = "evil.example.com"
        invalid_documents.append(inconsistent)
        cookie = json.loads(json.dumps(self.provider_projection))
        cookie["providers"][0]["header_bindings"][0]["source"]["header"] = "cookie"
        invalid_documents.append(cookie)
        ambiguous = json.loads(json.dumps(self.provider_projection))
        ambiguous["header_bindings"].append(ambiguous["header_bindings"][0])
        invalid_documents.append(ambiguous)
        for document in invalid_documents:
            with self.subTest(document=document):
                with self.assertRaises(BrokerCredentialUnavailable):
                    validate_provider_projection(document)

    def test_broker_denial_introspects_but_never_resolves_or_leaks_auth(self):
        flow = self.flow("https://api.github.com/user", "GET")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"
        calls = []

        def broker_call(request):
            calls.append(request)
            return self.broker_response(request)

        addon = self.broker_gateway(broker_call)
        captured = {}

        def deny(_url, policy_input, _timeout):
            captured.update(policy_input)
            return gateway.Decision(False, "denied", None, 403, True)

        output = io.StringIO()
        with mock.patch.object(gateway, "query_opa", side_effect=deny):
            with redirect_stdout(output):
                addon.requestheaders(flow)
        self.assertEqual([request["op"] for request in calls], ["introspect"])
        self.assertEqual(
            captured["authorization"],
            {"requested_profile": None, "broker_provider": "github"},
        )
        self.assertNotIn("authorization", captured["request"]["headers"])
        self.assertNotIn("authorization", flow.request.headers)
        rendered = output.getvalue() + flow.response.content.decode()
        self.assertNotIn(self.handle, rendered)
        self.assertNotIn(self.real_token, rendered)
        self.assertEqual(flow.response.status_code, 403)

    def test_broker_allow_resolves_once_and_replaces_exact_header(self):
        flow = self.flow("https://api.github.com/graphql", "POST")
        flow.request.headers["authorization"] = f"token {self.handle}"
        flow.request.headers["x-tobari-credential-profile"] = "untrusted-selector"
        flow.request.headers["proxy-authorization"] = "proxy-secret"
        calls = []

        def broker_call(request):
            calls.append(request)
            response = self.broker_response(request)
            # The fixture helper defaults to the bearer normalized binding;
            # echo the independently selected token binding for this request.
            binding = self.provider_projection["header_bindings"][1]
            response["source"] = binding["source"]
            return response

        addon = self.broker_gateway(broker_call)
        captured = {}

        def allow(_url, policy_input, _timeout):
            captured.update(policy_input)
            self.assertNotIn(self.handle, json.dumps(policy_input))
            self.assertNotIn(self.real_token, json.dumps(policy_input))
            return gateway.Decision(True, "allowed", None, 403, False)

        with mock.patch.object(gateway, "query_opa", side_effect=allow):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual([request["op"] for request in calls], ["introspect", "resolve"])
        self.assertEqual(flow.request.headers["authorization"], f"token {self.real_token}")
        self.assertNotIn(self.handle, flow.request.headers["authorization"])
        self.assertNotIn("x-tobari-credential-profile", flow.request.headers)
        self.assertNotIn("proxy-authorization", flow.request.headers)
        self.assertEqual(calls[1]["revision"], "revision_example")
        self.assertTrue(flow.request.stream)

    def test_broker_path_rejects_opa_static_profile_before_resolution(self):
        flow = self.flow("https://api.github.com/user", "GET")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"
        calls = []

        def broker_call(request):
            calls.append(request)
            return self.broker_response(request)

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", "example", 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual([request["op"] for request in calls], ["introspect"])
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(
            json.loads(flow.response.content), {"error": "credential_handle_invalid"}
        )

    def test_tobari_handle_wrong_host_header_format_and_shape_fail_before_opa(self):
        cases = [
            ("https://uploads.github.com/user", "authorization", f"Bearer {self.handle}"),
            ("http://api.github.com/user", "authorization", f"Bearer {self.handle}"),
            ("https://api.github.com:444/user", "authorization", f"Bearer {self.handle}"),
            ("https://api.github.com/user", "x-api-key", self.handle),
            ("https://api.github.com/user", "authorization", f"Basic {self.handle}"),
            ("https://api.github.com/user", "authorization", "Bearer tobari-h1_short"),
        ]
        for url, header, value in cases:
            with self.subTest(url=url, header=header, value=value):
                flow = self.flow(url, "GET")
                flow.request.headers.pop("authorization", None)
                flow.request.headers[header] = value
                calls = []
                addon = self.broker_gateway(lambda request: calls.append(request))
                with mock.patch.object(gateway, "query_opa") as query:
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                query.assert_not_called()
                self.assertEqual(calls, [])
                self.assertEqual(flow.response.status_code, 403)
                self.assertEqual(
                    json.loads(flow.response.content),
                    {"error": "credential_handle_invalid"},
                )

    def test_handle_in_url_cookie_or_embedded_header_is_rejected_and_redacted(self):
        cases = [
            (f"https://api.github.com/{self.handle}", "x-safe", "visible"),
            (
                "https://api.github.com/%2574obari-h1_" + "A" * 43,
                "x-safe",
                "visible",
            ),
            (
                f"https://api.github.com/user?capability={self.handle}",
                "x-safe",
                "visible",
            ),
            (
                "https://api.github.com/user",
                "cookie",
                f"session={self.handle}",
            ),
            (
                "https://api.github.com/user",
                "x-safe",
                f"prefix={self.handle}:suffix",
            ),
        ]
        for url, header, value in cases:
            with self.subTest(url=url, header=header):
                flow = self.flow(url, "GET")
                flow.request.headers.pop("authorization", None)
                flow.request.headers[header] = value
                output = io.StringIO()
                addon = self.broker_gateway(
                    lambda _request: (_ for _ in ()).throw(
                        AssertionError("unsupported handle position reached broker")
                    )
                )
                with mock.patch.object(gateway, "query_opa") as query:
                    with redirect_stdout(output):
                        addon.requestheaders(flow)
                query.assert_not_called()
                self.assertEqual(flow.response.status_code, 403)
                self.assertNotIn(self.handle, output.getvalue())
                self.assertNotIn("tobari-h1_", output.getvalue())

    def test_handle_only_in_body_uses_passthrough_without_broker_or_replacement(self):
        flow = self.flow("https://api.github.com/body-only", "POST")
        flow.request.headers.pop("authorization", None)
        body = f'{{"capability":"{self.handle}"}}'.encode()
        flow.request.content = body
        broker_calls = []

        def broker_call(request):
            broker_calls.append(request)
            raise AssertionError("request body selected the Auth Broker")

        addon = self.broker_gateway(broker_call)
        captured = {}

        def allow(_url, policy_input, _timeout):
            captured.update(policy_input)
            return gateway.Decision(True, "allowed", None, 403, False)

        with mock.patch.object(gateway, "query_opa", side_effect=allow) as query:
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)

        query.assert_called_once()
        self.assertEqual(broker_calls, [])
        self.assertIsNone(flow.response)
        self.assertEqual(flow.request.content, body)
        self.assertNotIn("authorization", flow.request.headers)
        self.assertIsNone(captured["authorization"]["broker_provider"])
        self.assertNotIn("body", captured["request"])
        self.assertTrue(flow.request.stream)

        flow.response = http.Response.make(200, b"upstream response")
        addon.responseheaders(flow)
        self.assertTrue(flow.response.stream)

    def test_broker_repeats_principal_binding_and_maps_rejection_without_handle(self):
        flow = self.flow("https://api.github.com/user", "GET")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"
        requests = []

        def reject(request):
            requests.append(request)
            raise BrokerCredentialBindingError(
                "credential handle is not valid for this request"
            )

        addon = self.broker_gateway(reject)
        output = io.StringIO()
        with mock.patch.object(gateway, "query_opa") as query:
            with redirect_stdout(output):
                addon.requestheaders(flow)
        query.assert_not_called()
        self.assertEqual(requests[0]["context_id"], self.context_a)
        self.assertEqual(requests[0]["project_id"], self.project_a)
        self.assertEqual(flow.response.status_code, 403)
        self.assertNotIn(self.handle, output.getvalue() + flow.response.content.decode())

    def test_one_broker_keeps_concurrent_context_credentials_separate(self):
        second_handle = "tobari-h1_" + "B" * 43
        flows = [
            self.flow("https://api.github.com/user", "GET"),
            self.flow("https://api.github.com/user", "GET"),
        ]
        flows[0].request.headers["authorization"] = f"Bearer {self.handle}"
        flows[1].request.headers["authorization"] = f"Bearer {second_handle}"
        flows[1].client_conn = SimpleNamespace(sockname=("172.29.1.2", 8080))
        seen = []

        def broker_call(request):
            seen.append((request["op"], request["context_id"], request["project_id"]))
            response = self.broker_response(request)
            if request["op"] == "resolve":
                import base64

                token = (
                    "context-a-token"
                    if request["context_id"] == self.context_a
                    else "context-b-token"
                )
                response["secret"]["value"] = base64.urlsafe_b64encode(
                    token.encode()
                ).rstrip(b"=").decode()
            return response

        addon = self.broker_gateway(broker_call)
        principals = {
            "172.29.0.2": self.principal_a,
            "172.29.1.2": self.principal_b,
        }
        with mock.patch.object(gateway, "load_project_principals", return_value=principals):
            with mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(True, "allowed", None, 403, False),
            ):
                with redirect_stdout(io.StringIO()):
                    addon.requestheaders(flows[0])
                    addon.requestheaders(flows[1])
        self.assertEqual(flows[0].request.headers["authorization"], "Bearer context-a-token")
        self.assertEqual(flows[1].request.headers["authorization"], "Bearer context-b-token")
        self.assertEqual(
            seen,
            [
                ("introspect", self.context_a, self.project_a),
                ("resolve", self.context_a, self.project_a),
                ("introspect", self.context_b, self.project_b),
                ("resolve", self.context_b, self.project_b),
            ],
        )

    def test_broker_enabled_gateway_retains_passthrough_for_non_handle_auth(self):
        flow = self.flow("https://api.github.com/user", "GET")
        flow.request.headers["authorization"] = "Bearer workspace-owned-token"
        addon = self.broker_gateway(
            lambda _request: (_ for _ in ()).throw(
                AssertionError("ordinary auth reached broker")
            )
        )
        captured = {}

        def allow(_url, policy_input, _timeout):
            captured.update(policy_input)
            return gateway.Decision(True, "allowed", None, 403, False)

        with mock.patch.object(gateway, "query_opa", side_effect=allow):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual(flow.request.headers["authorization"], "Bearer workspace-owned-token")
        self.assertIsNone(captured["authorization"]["broker_provider"])

    def test_broker_enabled_gateway_retains_managed_profile_fallback(self):
        flow = self.flow()
        flow.request.headers["x-tobari-credential-profile"] = "example"
        addon = self.managed_gateway()
        addon.credential_adapter = BrokeredCredentialAdapter(
            addon.credential_adapter,
            "/run/tobari/auth/providers.json",
            "/run/tobari-auth/runtime/broker.sock",
            2.0,
            projection_loader=lambda _: self.provider_projection,
            caller=lambda _path, _request, _timeout: (_ for _ in ()).throw(
                AssertionError("managed profile reached broker")
            ),
        )
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

    def test_redirected_handle_is_independently_rejected_outside_binding(self):
        first = self.flow("https://api.github.com/redirect", "GET")
        first.request.headers["authorization"] = f"Bearer {self.handle}"
        calls = []

        def broker_call(request):
            calls.append(request)
            return self.broker_response(request)

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", None, 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(first)
        self.assertEqual(first.request.headers["authorization"], f"Bearer {self.real_token}")

        redirected = self.flow("https://redirect.example.com/user", "GET")
        redirected.request.headers["authorization"] = f"Bearer {self.handle}"
        with mock.patch.object(gateway, "query_opa") as query:
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(redirected)
        query.assert_not_called()
        self.assertEqual(redirected.response.status_code, 403)
        self.assertEqual([request["op"] for request in calls], ["introspect", "resolve"])

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
