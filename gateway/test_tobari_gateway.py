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
import broker_credentials as broker_module
from broker_credentials import (
    BrokerCredentialBindingError,
    BrokerCredentialOutcomeUnknown,
    BrokerCredentialUnavailable,
    BrokeredCredentialAdapter,
    _broker_response,
    call_broker,
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
            "contexts": {
                self.context_a: {"name": "default", "graphql_endpoints": [], "profiles": {
                    "example": {
                        "type": "bearer", "hosts": ["api.example.com"],
                        "projects": [self.project_a],
                        "secret_file": f"/run/tobari/credentials/{self.context_a}/example",
                    }
                }},
            },
        }
        self.credential_path = os.path.join(self.temp.name, "credentials.json")
        with open(self.credential_path, "w", encoding="utf-8") as handle:
            json.dump(self.config, handle)
        self.previous_credential_path = os.environ.get("TOBARI_CREDENTIAL_CONFIG")
        os.environ["TOBARI_CREDENTIAL_CONFIG"] = self.credential_path
        self.addCleanup(self.restore_credential_path)
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

    def restore_credential_path(self):
        if self.previous_credential_path is None:
            os.environ.pop("TOBARI_CREDENTIAL_CONFIG", None)
        else:
            os.environ["TOBARI_CREDENTIAL_CONFIG"] = self.previous_credential_path

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

    @staticmethod
    def anthropic_claude_provider_projection():
        provider = {
            "schema_version": 1,
            "id": "anthropic",
            "display_name": "Anthropic account for Claude Code",
            "acquisition": {
                "mode": "builtin_helper",
                "helper": "claude-setup-token",
            },
            "credential": {"kind": "primary_secret"},
            "workspace_projections": [
                {
                    "kind": "env",
                    "name": "CLAUDE_CODE_OAUTH_TOKEN",
                    "template": "${HANDLE}",
                }
            ],
            "header_bindings": [
                {
                    "target": {
                        "scheme": "https",
                        "host": "api.anthropic.com",
                        "port": 443,
                    },
                    "source": {
                        "header": "authorization",
                        "formats": ["bearer"],
                    },
                    "destination": {
                        "header": "authorization",
                        "format": "bearer",
                        "secret_field": "primary_secret",
                    },
                    "secret_headers": ["authorization"],
                }
            ],
        }
        normalized = {
            "provider_id": "anthropic",
            "target": {
                "scheme": "https",
                "host": "api.anthropic.com",
                "port": 443,
            },
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {
                "header": "authorization",
                "format": "bearer",
                "secret_field": "primary_secret",
            },
            "secret_headers": ["authorization"],
        }
        return {
            "schema_version": 1,
            "providers": [provider],
            "environment": [
                {
                    "provider_id": "anthropic",
                    "name": "CLAUDE_CODE_OAUTH_TOKEN",
                    "template": "${HANDLE}",
                }
            ],
            "complete_files": [],
            "header_bindings": [normalized],
            "secret_headers": ["authorization"],
        }

    @staticmethod
    def static_tool_provider_projection():
        providers = [
            {
                "schema_version": 1,
                "id": "chatwork",
                "display_name": "Chatwork API for cwk",
                "acquisition": {"mode": "stdin_import"},
                "credential": {"kind": "primary_secret"},
                "workspace_projections": [
                    {"kind": "env", "name": "CWK_API_TOKEN", "template": "${HANDLE}"},
                ],
                "header_bindings": [
                    {
                        "target": {"scheme": "https", "host": "api.chatwork.com", "port": 443},
                        "source": {"header": "x-chatworktoken", "formats": ["raw"]},
                        "destination": {
                            "header": "x-chatworktoken",
                            "format": "raw",
                            "secret_field": "primary_secret",
                        },
                        "secret_headers": ["x-chatworktoken"],
                    }
                ],
            },
            {
                "schema_version": 1,
                "id": "datadog",
                "display_name": "Datadog access token for pup",
                "acquisition": {"mode": "stdin_import"},
                "credential": {"kind": "primary_secret"},
                "workspace_projections": [
                    {"kind": "env", "name": "DD_ACCESS_TOKEN", "template": "${HANDLE}"},
                    {"kind": "env", "name": "DD_SITE", "template": "datadoghq.com"},
                ],
                "header_bindings": [
                    {
                        "target": {"scheme": "https", "host": "api.datadoghq.com", "port": 443},
                        "source": {"header": "authorization", "formats": ["bearer"]},
                        "destination": {
                            "header": "authorization",
                            "format": "bearer",
                            "secret_field": "primary_secret",
                        },
                        "secret_headers": ["authorization"],
                    }
                ],
            },
        ]
        return {
            "schema_version": 1,
            "providers": providers,
            "environment": [
                {"provider_id": "chatwork", "name": "CWK_API_TOKEN", "template": "${HANDLE}"},
                {"provider_id": "datadog", "name": "DD_ACCESS_TOKEN", "template": "${HANDLE}"},
                {"provider_id": "datadog", "name": "DD_SITE", "template": "datadoghq.com"},
            ],
            "complete_files": [],
            "header_bindings": [
                {
                    "provider_id": "chatwork",
                    "target": {"scheme": "https", "host": "api.chatwork.com", "port": 443},
                    "source": {"header": "x-chatworktoken", "format": "raw"},
                    "destination": {
                        "header": "x-chatworktoken",
                        "format": "raw",
                        "secret_field": "primary_secret",
                    },
                    "secret_headers": ["x-chatworktoken"],
                },
                {
                    "provider_id": "datadog",
                    "target": {"scheme": "https", "host": "api.datadoghq.com", "port": 443},
                    "source": {"header": "authorization", "format": "bearer"},
                    "destination": {
                        "header": "authorization",
                        "format": "bearer",
                        "secret_field": "primary_secret",
                    },
                    "secret_headers": ["authorization"],
                },
            ],
            "secret_headers": ["authorization", "x-chatworktoken"],
        }

    @staticmethod
    def datadog_oauth_provider_projection():
        provider = {
            "schema_version": 2,
            "id": "datadog",
            "display_name": "Datadog access token for pup",
            "acquisition": {"mode": "builtin_helper", "helper": "pup-oauth"},
            "credential": {"kind": "datadog_oauth_session"},
            "workspace_projections": [
                {"kind": "env", "name": "DD_ACCESS_TOKEN", "template": "${HANDLE}"},
                {"kind": "env", "name": "DD_SITE", "template": "datadoghq.com"},
            ],
            "header_bindings": [
                {
                    "target": {"scheme": "https", "host": "api.datadoghq.com", "port": 443},
                    "source": {"header": "authorization", "formats": ["bearer"]},
                    "destination": {"header": "authorization", "format": "bearer", "secret_field": "datadog_oauth_session"},
                    "secret_headers": ["authorization"],
                }
            ],
            "signing_bindings": [],
        }
        normalized = {
            "provider_id": "datadog",
            "target": {"scheme": "https", "host": "api.datadoghq.com", "port": 443},
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {"header": "authorization", "format": "bearer", "secret_field": "datadog_oauth_session"},
            "secret_headers": ["authorization"],
        }
        return {
            "schema_version": 2,
            "providers": [provider],
            "environment": [
                {"provider_id": "datadog", "name": "DD_ACCESS_TOKEN", "template": "${HANDLE}"},
                {"provider_id": "datadog", "name": "DD_SITE", "template": "datadoghq.com"},
            ],
            "complete_files": [],
            "header_bindings": [normalized],
            "signing_bindings": [],
            "secret_headers": ["authorization"],
        }

    @staticmethod
    def openai_codex_oauth_provider_projection():
        auth_template = (
            '{"auth_mode":"chatgptAuthTokens","OPENAI_API_KEY":null,'
            '"tokens":{"id_token":"e30.e30.x","access_token":"${HANDLE}",'
            '"refresh_token":"","account_id":null},'
            '"last_refresh":"1970-01-01T00:00:00Z"}'
        )
        provider = {
            "schema_version": 2,
            "id": "openai",
            "display_name": "OpenAI account for Codex",
            "acquisition": {
                "mode": "builtin_helper",
                "helper": "codex-chatgpt-oauth",
            },
            "credential": {"kind": "openai_codex_oauth_session"},
            "workspace_projections": [
                {
                    "kind": "complete_file",
                    "path": ".codex/auth.json",
                    "template": auth_template,
                }
            ],
            "header_bindings": [
                {
                    "target": {
                        "scheme": "https",
                        "host": "chatgpt.com",
                        "port": 443,
                    },
                    "source": {
                        "header": "authorization",
                        "formats": ["bearer"],
                    },
                    "destination": {
                        "header": "authorization",
                        "format": "bearer",
                        "secret_field": "openai_codex_oauth_session",
                    },
                    "secret_headers": [
                        "authorization",
                        "chatgpt-account-id",
                        "x-openai-fedramp",
                    ],
                }
            ],
        }
        normalized = {
            "provider_id": "openai",
            "target": {"scheme": "https", "host": "chatgpt.com", "port": 443},
            "source": {"header": "authorization", "format": "bearer"},
            "destination": {
                "header": "authorization",
                "format": "bearer",
                "secret_field": "openai_codex_oauth_session",
            },
            "secret_headers": [
                "authorization",
                "chatgpt-account-id",
                "x-openai-fedramp",
            ],
        }
        return {
            "schema_version": 2,
            "providers": [provider],
            "environment": [],
            "complete_files": [
                {
                    "provider_id": "openai",
                    "path": ".codex/auth.json",
                    "template": auth_template,
                }
            ],
            "header_bindings": [normalized],
            "signing_bindings": [],
            "secret_headers": [
                "authorization",
                "chatgpt-account-id",
                "x-openai-fedramp",
            ],
        }

    @staticmethod
    def aws_provider_projection():
        signing_plan = {
            "target": {
                "scheme": "https",
                "port": 443,
                "dns_suffixes": ["amazonaws.com"],
            },
            "source": {
                "authorization_header": "authorization",
                "security_token_header": "x-amz-security-token",
            },
            "secret_headers": ["authorization", "x-amz-security-token"],
        }
        provider = {
            "schema_version": 2,
            "id": "aws",
            "display_name": "AWS IAM Identity Center",
            "acquisition": {"mode": "builtin_helper", "helper": "aws-sso"},
            "credential": {"kind": "aws_sso_session"},
            "workspace_projections": [
                {"kind": "env", "name": "AWS_ACCESS_KEY_ID", "template": "${HANDLE}"},
                {"kind": "env", "name": "AWS_EC2_METADATA_DISABLED", "template": "true"},
                {"kind": "env", "name": "AWS_SECRET_ACCESS_KEY", "template": "${HANDLE}"},
                {"kind": "env", "name": "AWS_SESSION_TOKEN", "template": "${HANDLE}"},
            ],
            "header_bindings": [],
            "signing_bindings": [{"kind": "aws_sigv4", "aws_sigv4": signing_plan}],
        }
        normalized = {
            "provider_id": "aws",
            "kind": "aws_sigv4",
            "aws_sigv4": signing_plan,
        }
        return {
            "schema_version": 2,
            "providers": [provider],
            "environment": [
                {"provider_id": "aws", "name": "AWS_ACCESS_KEY_ID", "template": "${HANDLE}"},
                {"provider_id": "aws", "name": "AWS_EC2_METADATA_DISABLED", "template": "true"},
                {"provider_id": "aws", "name": "AWS_SECRET_ACCESS_KEY", "template": "${HANDLE}"},
                {"provider_id": "aws", "name": "AWS_SESSION_TOKEN", "template": "${HANDLE}"},
            ],
            "complete_files": [],
            "header_bindings": [],
            "signing_bindings": [normalized],
            "secret_headers": ["authorization", "x-amz-security-token"],
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

    def aws_signed_flow(self):
        body = b"Action=GetCallerIdentity&Version=2011-06-15"
        flow = self.flow("https://sts.us-east-1.amazonaws.com/", "POST")
        flow.request.raw_content = body
        flow.request.headers["content-type"] = "application/x-www-form-urlencoded"
        flow.request.headers["content-length"] = str(len(body))
        flow.request.headers["x-amz-date"] = "20260809T120000Z"
        flow.request.headers["x-amz-security-token"] = self.handle
        flow.request.headers["authorization"] = (
            "AWS4-HMAC-SHA256 Credential="
            + self.handle
            + "/20260809/us-east-1/sts/aws4_request, "
            "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token, "
            "Signature="
            + "0" * 64
        )
        return flow

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

    @staticmethod
    def declare_graphql_endpoint(addon):
        addon.graphql_config["contexts"][
            "01912345-6789-7abc-8def-0123456789ad"
        ]["graphql_endpoints"] = [
            {
                "scheme": "https",
                "host": "api.example.com",
                "port": 443,
                "path": "/graphql",
            }
        ]

    def test_declared_graphql_is_parsed_before_policy_and_forwards_original_bytes(self):
        body = json.dumps(
            {
                "query": "mutation Change($secret: String!) { second: closeIssue(input: {value: $secret}) { id } first: updateIssue(input: {value: $secret}) { id } }",
                "variables": {"secret": "raw-variable-canary"},
            },
            separators=(",", ":"),
        ).encode()
        flow = self.flow("https://api.example.com/graphql")
        flow.request.raw_content = body
        flow.request.headers["content-length"] = str(len(body))
        addon = gateway.TobariGateway()
        self.declare_graphql_endpoint(addon)
        captured = {}

        def allow(_url, document, _timeout):
            captured.update(document)
            return gateway.Decision(True, "allowed", None, 403, False)

        with mock.patch.object(gateway, "query_opa", side_effect=allow) as query:
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
                query.assert_not_called()
                self.assertFalse(flow.request.stream)
                addon.request(flow)
        self.assertIsNone(flow.response)
        self.assertEqual(flow.request.raw_content, body)
        self.assertEqual(
            captured["request"]["graphql"],
            {
                "operation_type": "mutation",
                "root_fields": ["closeIssue", "updateIssue"],
            },
        )
        encoded = json.dumps(captured)
        self.assertNotIn("raw-variable-canary", encoded)
        self.assertNotIn("mutation Change", encoded)

    def test_graphql_denial_audits_each_root_without_document_or_variables(self):
        body = json.dumps(
            {
                "query": "query Private($token: String!) { viewer { login } repository(name: $token) { id } }",
                "variables": {"token": "raw-audit-canary"},
            },
            separators=(",", ":"),
        ).encode()
        flow = self.flow("https://api.example.com/graphql")
        flow.request.raw_content = body
        flow.request.headers["content-length"] = str(len(body))
        addon = gateway.TobariGateway()
        self.declare_graphql_endpoint(addon)
        output = io.StringIO()
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(
                False, "request did not match an allow rule", None, 403, True
            ),
        ):
            with redirect_stdout(output):
                addon.requestheaders(flow)
                addon.request(flow)
        self.assertEqual(flow.response.status_code, 403)
        records = [json.loads(line) for line in output.getvalue().splitlines()]
        self.assertEqual(
            [record["graphql_root_field"] for record in records],
            ["repository", "viewer"],
        )
        self.assertTrue(all(record["schema_version"] == 3 for record in records))
        combined = output.getvalue() + flow.response.content.decode()
        self.assertNotIn("raw-audit-canary", combined)
        self.assertNotIn("query Private", combined)

    def test_invalid_declared_graphql_fails_locally_without_opa(self):
        body = b'{"query":"subscription Events { eventAdded { id } }"}'
        flow = self.flow("https://api.example.com/graphql")
        flow.request.raw_content = body
        flow.request.headers["content-length"] = str(len(body))
        addon = gateway.TobariGateway()
        self.declare_graphql_endpoint(addon)
        with mock.patch.object(gateway, "query_opa") as query:
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
                addon.request(flow)
        query.assert_not_called()
        self.assertEqual(flow.response.status_code, 400)
        self.assertEqual(json.loads(flow.response.content), {"error": "unsupported_operation"})

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
        mismatched_secret_kind = json.loads(json.dumps(self.provider_projection))
        mismatched_secret_kind["providers"][0]["header_bindings"][0]["destination"][
            "secret_field"
        ] = "openai_codex_oauth_session"
        for binding in mismatched_secret_kind["header_bindings"]:
            binding["destination"]["secret_field"] = "openai_codex_oauth_session"
        invalid_documents.append(mismatched_secret_kind)
        for document in invalid_documents:
            with self.subTest(document=document):
                with self.assertRaises(BrokerCredentialUnavailable):
                    validate_provider_projection(document)

    def test_anthropic_claude_projection_is_closed_and_self_consistent(self):
        projection = self.anthropic_claude_provider_projection()
        self.assertEqual(validate_provider_projection(projection), projection)

        def alternate_helper(value):
            value["providers"][0]["acquisition"]["helper"] = "claude-login"

        def alternate_provider(value):
            value["providers"][0]["id"] = "anthropic-alt"
            value["environment"][0]["provider_id"] = "anthropic-alt"
            value["header_bindings"][0]["provider_id"] = "anthropic-alt"

        def alternate_display_name(value):
            value["providers"][0]["display_name"] = "Anthropic account"

        def alternate_environment(value):
            provider = value["providers"][0]
            provider["workspace_projections"][0]["name"] = "ANTHROPIC_AUTH_TOKEN"
            value["environment"][0]["name"] = "ANTHROPIC_AUTH_TOKEN"

        def alternate_host(value):
            provider_binding = value["providers"][0]["header_bindings"][0]
            normalized_binding = value["header_bindings"][0]
            provider_binding["target"]["host"] = "api.anthropic.example.com"
            normalized_binding["target"]["host"] = "api.anthropic.example.com"

        def alternate_header(value):
            for binding in (
                value["providers"][0]["header_bindings"][0],
                value["header_bindings"][0],
            ):
                binding["source"]["header"] = "x-api-key"
                binding["destination"]["header"] = "x-api-key"
                binding["secret_headers"] = ["x-api-key"]
            value["secret_headers"] = ["x-api-key"]

        def alternate_format(value):
            provider_binding = value["providers"][0]["header_bindings"][0]
            normalized_binding = value["header_bindings"][0]
            provider_binding["source"]["formats"] = ["token"]
            provider_binding["destination"]["format"] = "token"
            normalized_binding["source"]["format"] = "token"
            normalized_binding["destination"]["format"] = "token"

        for label, mutate in (
            ("helper", alternate_helper),
            ("provider", alternate_provider),
            ("display name", alternate_display_name),
            ("environment", alternate_environment),
            ("host", alternate_host),
            ("header", alternate_header),
            ("format", alternate_format),
        ):
            with self.subTest(label=label):
                invalid = json.loads(json.dumps(projection))
                mutate(invalid)
                with self.assertRaises(BrokerCredentialUnavailable):
                    validate_provider_projection(invalid)

    def test_static_tool_provider_projection_is_exact_and_self_consistent(self):
        projection = self.static_tool_provider_projection()
        self.assertEqual(validate_provider_projection(projection), projection)
        self.assertEqual(
            [provider["id"] for provider in projection["providers"]],
            ["chatwork", "datadog"],
        )
        self.assertEqual(
            {
                item["name"]: item["template"]
                for item in projection["environment"]
            },
            {
                "CWK_API_TOKEN": "${HANDLE}",
                "DD_ACCESS_TOKEN": "${HANDLE}",
                "DD_SITE": "datadoghq.com",
            },
        )

    def test_datadog_oauth_projection_is_closed_and_self_consistent(self):
        projection = self.datadog_oauth_provider_projection()
        self.assertEqual(validate_provider_projection(projection), projection)
        serialized = json.loads(json.dumps(projection))
        serialized["providers"][0].pop("signing_bindings")
        self.assertEqual(validate_provider_projection(serialized), serialized)
        invalid_documents = []
        for mutate in (
            lambda value: value["providers"][0]["acquisition"].update(helper="command"),
            lambda value: value["providers"][0]["credential"].update(kind="primary_secret"),
            lambda value: value["providers"][0]["header_bindings"][0]["target"].update(host="api.datadoghq.eu"),
            lambda value: value["providers"][0]["header_bindings"][0]["destination"].update(secret_field="primary_secret"),
        ):
            invalid = json.loads(json.dumps(projection))
            mutate(invalid)
            invalid_documents.append(invalid)
        for document in invalid_documents:
            with self.subTest(document=document):
                with self.assertRaises(BrokerCredentialUnavailable):
                    validate_provider_projection(document)

    def test_datadog_oauth_resolves_only_after_allow_with_exact_secret_field(self):
        import base64

        self.provider_projection = self.datadog_oauth_provider_projection()
        flow = self.flow("https://api.datadoghq.com/api/v2/users", "GET")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"
        calls = []

        def broker_call(request):
            calls.append(request)
            binding = self.provider_projection["header_bindings"][0]
            response = {
                "schema_version": 1,
                "ok": True,
                "provider": "datadog",
                "revision": "revision_example",
                "target": binding["target"],
                "source": binding["source"],
                "destination": binding["destination"],
                "secret_headers": binding["secret_headers"],
            }
            if request["op"] == "resolve":
                response["secret"] = {
                    "field": "datadog_oauth_session",
                    "encoding": "base64url",
                    "value": base64.urlsafe_b64encode(
                        self.real_token.encode()
                    ).rstrip(b"=").decode(),
                }
            return response

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", None, 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual([request["op"] for request in calls], ["introspect", "resolve"])
        self.assertEqual(flow.request.headers["authorization"], f"Bearer {self.real_token}")

    def test_datadog_oauth_unknown_refresh_outcome_is_non_retryable(self):
        self.provider_projection = self.datadog_oauth_provider_projection()
        flow = self.flow("https://api.datadoghq.com/api/v2/users", "GET")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"

        def broker_call(request):
            binding = self.provider_projection["header_bindings"][0]
            if request["op"] == "introspect":
                return {
                    "schema_version": 1,
                    "ok": True,
                    "provider": "datadog",
                    "revision": "revision_example",
                    "target": binding["target"],
                    "source": binding["source"],
                    "destination": binding["destination"],
                    "secret_headers": binding["secret_headers"],
                }
            raise BrokerCredentialOutcomeUnknown("synthetic Datadog refresh outcome")

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", None, 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual(flow.response.status_code, 409)
        self.assertEqual(
            json.loads(flow.response.content),
            {"error": "credential_refresh_outcome_unknown"},
        )

    def test_openai_codex_oauth_projection_is_closed_and_self_consistent(self):
        projection = self.openai_codex_oauth_provider_projection()
        self.assertEqual(validate_provider_projection(projection), projection)
        with_explicit_empty_signing = json.loads(json.dumps(projection))
        with_explicit_empty_signing["providers"][0]["signing_bindings"] = []
        self.assertEqual(
            validate_provider_projection(with_explicit_empty_signing),
            with_explicit_empty_signing,
        )
        for label, mutate in (
            (
                "display name",
                lambda value: value["providers"][0].update(
                    display_name="OpenAI account"
                ),
            ),
            (
                "helper",
                lambda value: value["providers"][0]["acquisition"].update(
                    helper="command"
                ),
            ),
            (
                "kind",
                lambda value: value["providers"][0]["credential"].update(
                    kind="primary_secret"
                ),
            ),
            (
                "target",
                lambda value: value["providers"][0]["header_bindings"][0][
                    "target"
                ].update(host="api.openai.com"),
            ),
            (
                "projection path",
                lambda value: value["providers"][0]["workspace_projections"][0].update(
                    path=".config/openai/auth.json"
                ),
            ),
            (
                "projection template",
                lambda value: value["providers"][0]["workspace_projections"][0].update(
                    template="${HANDLE}"
                ),
            ),
            (
                "secret headers",
                lambda value: value["providers"][0]["header_bindings"][0].update(
                    secret_headers=["authorization"]
                ),
            ),
        ):
            with self.subTest(label=label):
                invalid = json.loads(json.dumps(projection))
                mutate(invalid)
                with self.assertRaises(BrokerCredentialUnavailable):
                    validate_provider_projection(invalid)

    def test_openai_codex_headers_are_broker_owned_and_injected_only_after_allow(self):
        import base64

        self.provider_projection = self.openai_codex_oauth_provider_projection()
        flow = self.flow("https://chatgpt.com/backend-api/codex/responses", "POST")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"
        flow.request.headers["chatgpt-account-id"] = "caller-account"
        flow.request.headers["x-openai-fedramp"] = "true"
        calls = []
        events = []

        def broker_call(request):
            events.append(request["op"])
            calls.append(request)
            self.assertNotIn("authorization", flow.request.headers)
            self.assertNotIn("chatgpt-account-id", flow.request.headers)
            self.assertNotIn("x-openai-fedramp", flow.request.headers)
            binding = self.provider_projection["header_bindings"][0]
            response = {
                "schema_version": 1,
                "ok": True,
                "provider": "openai",
                "revision": "revision_example",
                "target": binding["target"],
                "source": binding["source"],
                "destination": binding["destination"],
                "secret_headers": binding["secret_headers"],
            }
            if request["op"] == "resolve":
                response["secret"] = {
                    "field": "openai_codex_oauth_session",
                    "encoding": "base64url",
                    "value": base64.urlsafe_b64encode(
                        self.real_token.encode()
                    ).rstrip(b"=").decode(),
                }
                response["supplemental_headers"] = {
                    "chatgpt-account-id": "acct-0123456789"
                }
            return response

        captured = {}

        def allow(_url, policy_input, _timeout):
            events.append("policy")
            captured.update(policy_input)
            self.assertEqual([item["op"] for item in calls], ["introspect"])
            return gateway.Decision(True, "allowed", None, 403, False)

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(gateway, "query_opa", side_effect=allow):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)

        self.assertEqual(events, ["introspect", "policy", "resolve"])
        self.assertEqual(
            captured["authorization"],
            {"requested_profile": None, "broker_provider": "openai"},
        )
        for header in (
            "authorization",
            "chatgpt-account-id",
            "x-openai-fedramp",
        ):
            self.assertNotIn(header, captured["request"]["headers"])
        self.assertEqual(
            flow.request.headers["authorization"], f"Bearer {self.real_token}"
        )
        self.assertEqual(
            flow.request.headers["chatgpt-account-id"], "acct-0123456789"
        )
        self.assertNotIn("x-openai-fedramp", flow.request.headers)

    def test_openai_codex_denial_never_resolves_or_forwards_routing_headers(self):
        self.provider_projection = self.openai_codex_oauth_provider_projection()
        flow = self.flow("https://chatgpt.com/backend-api/codex/responses", "POST")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"
        flow.request.headers["chatgpt-account-id"] = "caller-account"
        flow.request.headers["x-openai-fedramp"] = "true"
        calls = []

        def broker_call(request):
            calls.append(request)
            binding = self.provider_projection["header_bindings"][0]
            return {
                "schema_version": 1,
                "ok": True,
                "provider": "openai",
                "revision": "revision_example",
                "target": binding["target"],
                "source": binding["source"],
                "destination": binding["destination"],
                "secret_headers": binding["secret_headers"],
            }

        addon = self.broker_gateway(broker_call)
        captured = {}

        def deny(_url, policy_input, _timeout):
            captured.update(policy_input)
            return gateway.Decision(False, "denied", None, 403, True)

        with mock.patch.object(gateway, "query_opa", side_effect=deny):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual([request["op"] for request in calls], ["introspect"])
        for header in (
            "authorization",
            "chatgpt-account-id",
            "x-openai-fedramp",
        ):
            self.assertNotIn(header, flow.request.headers)
            self.assertNotIn(header, captured["request"]["headers"])

    def test_openai_codex_resolve_requires_exact_broker_account_header(self):
        import base64

        invalid_supplements = (
            None,
            {},
            {"chatgpt-account-id": "acct-0123456789", "x-extra": "value"},
            {"chatgpt-account-id": "bad account"},
            {"chatgpt-account-id": "account\nvalue"},
        )
        for supplement in invalid_supplements:
            with self.subTest(supplement=supplement):
                self.provider_projection = self.openai_codex_oauth_provider_projection()
                flow = self.flow(
                    "https://chatgpt.com/backend-api/codex/responses", "POST"
                )
                flow.request.headers["authorization"] = f"Bearer {self.handle}"

                def broker_call(request):
                    binding = self.provider_projection["header_bindings"][0]
                    response = {
                        "schema_version": 1,
                        "ok": True,
                        "provider": "openai",
                        "revision": "revision_example",
                        "target": binding["target"],
                        "source": binding["source"],
                        "destination": binding["destination"],
                        "secret_headers": binding["secret_headers"],
                    }
                    if request["op"] == "resolve":
                        response["secret"] = {
                            "field": "openai_codex_oauth_session",
                            "encoding": "base64url",
                            "value": base64.urlsafe_b64encode(
                                self.real_token.encode()
                            ).rstrip(b"=").decode(),
                        }
                        if supplement is not None:
                            response["supplemental_headers"] = supplement
                    return response

                addon = self.broker_gateway(broker_call)
                with mock.patch.object(
                    gateway,
                    "query_opa",
                    return_value=gateway.Decision(
                        True, "allowed", None, 403, False
                    ),
                ):
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                self.assertEqual(flow.response.status_code, 503)
                self.assertEqual(
                    json.loads(flow.response.content),
                    {"error": "credential_broker_unavailable"},
                )
                self.assertNotIn("authorization", flow.request.headers)
                self.assertNotIn("chatgpt-account-id", flow.request.headers)

    def test_non_openai_resolve_rejects_supplemental_headers(self):
        flow = self.flow("https://api.github.com/user", "GET")
        flow.request.headers["authorization"] = f"Bearer {self.handle}"

        def broker_call(request):
            response = self.broker_response(request)
            if request["op"] == "resolve":
                response["supplemental_headers"] = {
                    "chatgpt-account-id": "acct-0123456789"
                }
            return response

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", None, 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual(flow.response.status_code, 503)
        self.assertNotIn("authorization", flow.request.headers)

    def test_aws_sigv4_handle_is_signed_only_after_policy_and_complete_body(self):
        self.provider_projection = self.aws_provider_projection()
        self.assertEqual(
            validate_provider_projection(self.provider_projection),
            self.provider_projection,
        )
        body = b"Action=GetCallerIdentity&Version=2011-06-15"
        flow = self.flow("https://sts.us-east-1.amazonaws.com/", "POST")
        flow.request.raw_content = body
        flow.request.headers["content-type"] = "application/x-www-form-urlencoded; charset=utf-8"
        flow.request.headers["content-length"] = str(len(body))
        flow.request.headers["x-amz-date"] = "20260809T120000Z"
        flow.request.headers["x-amz-security-token"] = self.handle
        flow.request.headers["authorization"] = (
            "AWS4-HMAC-SHA256 Credential="
            + self.handle
            + "/20260809/us-east-1/sts/aws4_request, "
            "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token, "
            "Signature="
            + "0" * 64
        )
        calls = []

        def broker_call(request):
            calls.append(request)
            if request["op"] == "introspect_signing":
                return {
                    "schema_version": 1,
                    "ok": True,
                    "provider": "aws",
                    "revision": "revision_example",
                    "kind": "aws_sigv4",
                    "target": request["target"],
                    "source": request["binding"]["aws_sigv4"]["source"],
                    "secret_headers": request["binding"]["aws_sigv4"]["secret_headers"],
                }
            self.assertEqual(request["op"], "sign_sigv4")
            return {
                "schema_version": 1,
                "ok": True,
                "provider": "aws",
                "revision": "revision_example",
                "headers": {
                    "authorization": (
                        "AWS4-HMAC-SHA256 Credential=ASIAEXAMPLEKEY1234/"
                        "20260809/us-east-1/sts/aws4_request, "
                        "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token, "
                        "Signature=" + "a" * 64
                    ),
                    "x_amz_date": "20260809T120001Z",
                    "x_amz_security_token": "real-session-token-canary",
                    "x_amz_content_sha256": None,
                },
            }

        captured = {}

        def allow(_url, policy_input, _timeout):
            captured.update(policy_input)
            return gateway.Decision(True, "allowed", None, 403, False)

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(gateway, "query_opa", side_effect=allow):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual([item["op"] for item in calls], ["introspect_signing"])
        self.assertFalse(flow.request.stream)
        self.assertNotIn("authorization", flow.request.headers)
        self.assertNotIn("x-amz-security-token", flow.request.headers)
        self.assertNotIn("authorization", captured["request"]["headers"])
        self.assertEqual(
            captured["authorization"],
            {"requested_profile": None, "broker_provider": "aws"},
        )

        addon.request(flow)
        self.assertEqual(
            [item["op"] for item in calls],
            ["introspect_signing", "sign_sigv4"],
        )
        signing_request = calls[1]["request"]
        import hashlib

        self.assertEqual(signing_request["payload_hash"], hashlib.sha256(body).hexdigest())
        self.assertEqual(signing_request["region"], "us-east-1")
        self.assertEqual(signing_request["service"], "sts")
        self.assertNotIn("body", signing_request)
        self.assertTrue(
            flow.request.headers["authorization"].startswith(
                "AWS4-HMAC-SHA256 Credential=ASIAEXAMPLEKEY1234/"
            )
        )
        self.assertEqual(
            flow.request.headers["x-amz-security-token"],
            "real-session-token-canary",
        )
        rendered = json.dumps(captured) + json.dumps(calls)
        self.assertNotIn("real-session-token-canary", json.dumps(captured))
        self.assertNotIn("secret_access_key", rendered)

    def test_aws_refresh_outcome_unknown_returns_non_retryable_409(self):
        self.provider_projection = self.aws_provider_projection()
        flow = self.aws_signed_flow()

        def broker_call(request):
            if request["op"] == "introspect_signing":
                return {
                    "schema_version": 1,
                    "ok": True,
                    "provider": "aws",
                    "revision": "revision_example",
                    "kind": "aws_sigv4",
                    "target": request["target"],
                    "source": request["binding"]["aws_sigv4"]["source"],
                    "secret_headers": request["binding"]["aws_sigv4"]["secret_headers"],
                }
            raise BrokerCredentialOutcomeUnknown("synthetic outcome unknown")

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", None, 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertIsNone(flow.response)
        with redirect_stdout(io.StringIO()):
            addon.request(flow)
        self.assertEqual(flow.response.status_code, 409)
        self.assertEqual(
            json.loads(flow.response.content),
            {"error": "credential_refresh_outcome_unknown"},
        )

    def test_persisted_refresh_barrier_denies_before_policy(self):
        self.provider_projection = self.aws_provider_projection()
        flow = self.aws_signed_flow()
        addon = self.broker_gateway(
            lambda _request: (_ for _ in ()).throw(
                BrokerCredentialOutcomeUnknown("synthetic durable barrier")
            )
        )
        with mock.patch.object(gateway, "query_opa") as query:
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        query.assert_not_called()
        self.assertEqual(flow.response.status_code, 409)
        self.assertEqual(
            json.loads(flow.response.content),
            {"error": "credential_refresh_outcome_unknown"},
        )

    def test_known_broker_unavailability_remains_retryable_503(self):
        self.provider_projection = self.aws_provider_projection()
        flow = self.aws_signed_flow()

        def broker_call(request):
            if request["op"] == "introspect_signing":
                return {
                    "schema_version": 1,
                    "ok": True,
                    "provider": "aws",
                    "revision": "revision_example",
                    "kind": "aws_sigv4",
                    "target": request["target"],
                    "source": request["binding"]["aws_sigv4"]["source"],
                    "secret_headers": request["binding"]["aws_sigv4"]["secret_headers"],
                }
            raise BrokerCredentialUnavailable("synthetic unavailable")

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", None, 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        with redirect_stdout(io.StringIO()):
            addon.request(flow)
        self.assertEqual(flow.response.status_code, 503)
        self.assertEqual(
            json.loads(flow.response.content),
            {"error": "credential_broker_unavailable"},
        )

    def test_aws_sigv4_denial_never_signs_or_retains_handle(self):
        self.provider_projection = self.aws_provider_projection()
        flow = self.flow("https://sts.us-east-1.amazonaws.com/", "POST")
        flow.request.headers["x-amz-date"] = "20260809T120000Z"
        flow.request.headers["x-amz-security-token"] = self.handle
        flow.request.headers["authorization"] = (
            "AWS4-HMAC-SHA256 Credential="
            + self.handle
            + "/20260809/us-east-1/sts/aws4_request, "
            "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token, "
            "Signature="
            + "0" * 64
        )
        calls = []

        def broker_call(request):
            calls.append(request)
            return {
                "schema_version": 1,
                "ok": True,
                "provider": "aws",
                "revision": "revision_example",
                "kind": "aws_sigv4",
                "target": request["target"],
                "source": request["binding"]["aws_sigv4"]["source"],
                "secret_headers": request["binding"]["aws_sigv4"]["secret_headers"],
            }

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(False, "denied", None, 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual([item["op"] for item in calls], ["introspect_signing"])
        self.assertNotIn("tobari_deferred_credential", flow.metadata)
        self.assertNotIn("authorization", flow.request.headers)
        self.assertNotIn("x-amz-security-token", flow.request.headers)
        self.assertEqual(flow.response.status_code, 403)

    def test_aws_sigv4_rejects_request_changes_after_policy_allow(self):
        self.provider_projection = self.aws_provider_projection()

        def configure_flow():
            body = b"Action=GetCallerIdentity&Version=2011-06-15"
            flow = self.flow(
                "https://sts.us-east-1.amazonaws.com/?Version=2011-06-15",
                "POST",
            )
            flow.request.raw_content = body
            flow.request.headers["content-type"] = "application/x-www-form-urlencoded"
            flow.request.headers["content-length"] = str(len(body))
            flow.request.headers["x-extra"] = "policy-visible"
            flow.request.headers["x-amz-date"] = "20260809T120000Z"
            flow.request.headers["x-amz-security-token"] = self.handle
            flow.request.headers["authorization"] = (
                "AWS4-HMAC-SHA256 Credential="
                + self.handle
                + "/20260809/us-east-1/sts/aws4_request, "
                "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token, "
                "Signature="
                + "0" * 64
            )
            return flow

        def mutate_method(flow):
            flow.request.method = "PUT"

        def mutate_path(flow):
            flow.request.path = "/changed?Version=2011-06-15"

        def mutate_query(flow):
            flow.request.path = "/?Version=changed"

        def mutate_authority(flow):
            flow.request.host = "sts.us-west-2.amazonaws.com"

        def mutate_signed_header(flow):
            flow.request.headers["content-type"] = "application/json"

        def mutate_policy_header(flow):
            flow.request.headers["x-extra"] = "changed"

        for name, mutation in {
            "method": mutate_method,
            "path": mutate_path,
            "query": mutate_query,
            "authority": mutate_authority,
            "signed header": mutate_signed_header,
            "policy-visible header": mutate_policy_header,
        }.items():
            with self.subTest(name=name):
                calls = []

                def broker_call(request):
                    calls.append(request)
                    if request["op"] != "introspect_signing":
                        raise AssertionError("changed request reached signing")
                    return {
                        "schema_version": 1,
                        "ok": True,
                        "provider": "aws",
                        "revision": "revision_example",
                        "kind": "aws_sigv4",
                        "target": request["target"],
                        "source": request["binding"]["aws_sigv4"]["source"],
                        "secret_headers": request["binding"]["aws_sigv4"]["secret_headers"],
                    }

                flow = configure_flow()
                addon = self.broker_gateway(broker_call)
                with mock.patch.object(
                    gateway,
                    "query_opa",
                    return_value=gateway.Decision(True, "allowed", None, 403, False),
                ):
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                mutation(flow)
                with redirect_stdout(io.StringIO()):
                    addon.request(flow)
                self.assertEqual([item["op"] for item in calls], ["introspect_signing"])
                self.assertEqual(flow.response.status_code, 403)
                self.assertEqual(
                    json.loads(flow.response.content),
                    {"error": "broker_signing_request_invalid"},
                )

    def test_aws_sigv4_rejects_unbounded_or_separately_authenticated_forms(self):
        self.provider_projection = self.aws_provider_projection()
        cases = (
            ("content-type", "application/vnd.amazon.eventstream"),
            (
                "x-amz-content-sha256",
                "STREAMING-AWS4-HMAC-SHA256-PAYLOAD",
            ),
            ("x-amz-content-sha256", "UNSIGNED-PAYLOAD"),
            ("x-amz-s3session-token", "synthetic-s3-express-token"),
        )
        for header, value in cases:
            with self.subTest(header=header, value=value):
                flow = self.flow("https://s3.us-east-1.amazonaws.com/", "POST")
                flow.request.headers["content-length"] = str(len(flow.request.raw_content))
                flow.request.headers["x-amz-date"] = "20260809T120000Z"
                flow.request.headers["x-amz-security-token"] = self.handle
                flow.request.headers["authorization"] = (
                    "AWS4-HMAC-SHA256 Credential="
                    + self.handle
                    + "/20260809/us-east-1/s3/aws4_request, "
                    "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token, "
                    "Signature="
                    + "0" * 64
                )
                flow.request.headers[header] = value
                addon = self.broker_gateway(
                    lambda _request: (_ for _ in ()).throw(
                        AssertionError("unsupported AWS form reached broker")
                    )
                )
                with mock.patch.object(gateway, "query_opa") as query:
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                query.assert_not_called()
                self.assertEqual(flow.response.status_code, 403)
                self.assertEqual(
                    json.loads(flow.response.content),
                    {"error": "credential_handle_invalid"},
                )

    def test_aws_sigv4_rejects_noncommercial_partition_scopes_before_policy(self):
        self.provider_projection = self.aws_provider_projection()
        for region, host in (
            ("us-gov-west-1", "sts.us-gov-west-1.amazonaws.com"),
            ("cn-north-1", "sts.cn-north-1.amazonaws.com.cn"),
        ):
            with self.subTest(region=region):
                flow = self.flow("https://" + host + "/", "GET")
                flow.request.headers["x-amz-date"] = "20260809T120000Z"
                flow.request.headers["x-amz-security-token"] = self.handle
                flow.request.headers["authorization"] = (
                    "AWS4-HMAC-SHA256 Credential="
                    + self.handle
                    + "/20260809/"
                    + region
                    + "/sts/aws4_request, "
                    "SignedHeaders=host;x-amz-date;x-amz-security-token, "
                    "Signature="
                    + "0" * 64
                )
                addon = self.broker_gateway(
                    lambda _request: (_ for _ in ()).throw(
                        AssertionError("noncommercial AWS scope reached broker")
                    )
                )
                with mock.patch.object(gateway, "query_opa") as query:
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                query.assert_not_called()
                self.assertEqual(flow.response.status_code, 403)
                self.assertEqual(
                    json.loads(flow.response.content),
                    {"error": "credential_handle_invalid"},
                )

    def test_broker_classifies_invalid_aws_request_as_binding_denial(self):
        for code in (
            "aws_target_unsupported",
            "aws_scope_target_mismatch",
            "aws_query_unsupported",
            "aws_header_invalid",
        ):
            with self.subTest(code=code), self.assertRaises(
                BrokerCredentialBindingError
            ):
                _broker_response(
                    json.dumps(
                        {
                            "schema_version": 1,
                            "ok": False,
                            "error": {"code": code},
                        },
                        separators=(",", ":"),
                    ).encode()
                )
        with self.assertRaises(BrokerCredentialUnavailable):
            _broker_response(
                b'{"schema_version":1,"ok":false,"error":{"code":"companion_unavailable"}}'
            )
        with self.assertRaises(BrokerCredentialOutcomeUnknown):
            _broker_response(
                b'{"schema_version":1,"ok":false,"error":{"code":"companion_outcome_unknown"}}'
            )

    def test_post_send_signing_transport_loss_is_outcome_unknown(self):
        class FailingSocket:
            def settimeout(self, _timeout):
                pass

            def connect(self, _path):
                pass

            def sendall(self, _payload):
                raise TimeoutError("synthetic timeout")

            def close(self):
                pass

        with mock.patch.object(
            broker_module.socket, "socket", return_value=FailingSocket()
        ):
            with self.assertRaises(BrokerCredentialOutcomeUnknown):
                call_broker(
                    "/run/tobari-auth/runtime/broker.sock",
                    {"schema_version": 1, "op": "sign_sigv4"},
                    70.0,
                )

    def test_post_send_openai_resolve_transport_loss_is_outcome_unknown(self):
        class FailingSocket:
            def settimeout(self, _timeout):
                pass

            def connect(self, _path):
                pass

            def sendall(self, _payload):
                raise TimeoutError("synthetic timeout")

            def close(self):
                pass

        with mock.patch.object(
            broker_module.socket, "socket", return_value=FailingSocket()
        ):
            with self.assertRaises(BrokerCredentialOutcomeUnknown):
                call_broker(
                    "/run/tobari-auth/runtime/broker.sock",
                    {"schema_version": 1, "op": "resolve", "provider": "openai"},
                    70.0,
                )

    def test_pre_send_broker_transport_loss_remains_unavailable(self):
        class FailingSocket:
            def settimeout(self, _timeout):
                pass

            def connect(self, _path):
                raise FileNotFoundError("synthetic missing socket")

            def close(self):
                pass

        with mock.patch.object(
            broker_module.socket, "socket", return_value=FailingSocket()
        ):
            with self.assertRaises(BrokerCredentialUnavailable):
                call_broker(
                    "/run/tobari-auth/runtime/broker.sock",
                    {"schema_version": 1, "op": "sign_sigv4"},
                    70.0,
                )

    def test_static_tool_handles_are_replaced_only_at_the_exact_binding(self):
        import base64

        self.provider_projection = self.static_tool_provider_projection()
        cases = [
            (
                "chatwork",
                "https://api.chatwork.com/v2/me",
                "https://api.chatwork.example/v2/me",
                "x-chatworktoken",
                self.handle,
                "x-chatworktoken",
                self.real_token,
            ),
            (
                "datadog",
                "https://api.datadoghq.com/api/v2/users/me",
                "https://api.datadoghq.eu/api/v2/users/me",
                "authorization",
                f"Bearer {self.handle}",
                "authorization",
                f"Bearer {self.real_token}",
            ),
        ]
        for (
            provider,
            url,
            wrong_url,
            source_header,
            source_value,
            destination_header,
            expected,
        ) in cases:
            with self.subTest(provider=provider):
                flow = self.flow(url, "GET")
                flow.request.headers.pop("authorization", None)
                flow.request.headers[source_header] = source_value
                calls = []

                def broker_call(request):
                    calls.append(request)
                    binding = next(
                        binding
                        for binding in self.provider_projection["header_bindings"]
                        if binding["provider_id"] == request["provider"]
                    )
                    response = {
                        "schema_version": 1,
                        "ok": True,
                        "provider": request["provider"],
                        "revision": "revision_example",
                        "target": binding["target"],
                        "source": binding["source"],
                        "destination": binding["destination"],
                        "secret_headers": binding["secret_headers"],
                    }
                    if request["op"] == "resolve":
                        response["secret"] = {
                            "field": "primary_secret",
                            "encoding": "base64url",
                            "value": base64.urlsafe_b64encode(
                                self.real_token.encode()
                            ).rstrip(b"=").decode(),
                        }
                    return response

                addon = self.broker_gateway(broker_call)
                with mock.patch.object(
                    gateway,
                    "query_opa",
                    return_value=gateway.Decision(True, "allowed", None, 403, False),
                ):
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                self.assertEqual(
                    [(request["op"], request["provider"]) for request in calls],
                    [("introspect", provider), ("resolve", provider)],
                )
                self.assertEqual(flow.request.headers[destination_header], expected)

                wrong_flow = self.flow(wrong_url, "GET")
                wrong_flow.request.headers.pop("authorization", None)
                wrong_flow.request.headers[source_header] = source_value
                wrong_addon = self.broker_gateway(
                    lambda _: self.fail("wrong target reached the broker")
                )
                with mock.patch.object(gateway, "query_opa") as query:
                    with redirect_stdout(io.StringIO()):
                        wrong_addon.requestheaders(wrong_flow)
                query.assert_not_called()
                self.assertEqual(wrong_flow.response.status_code, 403)
                self.assertEqual(
                    json.loads(wrong_flow.response.content),
                    {"error": "credential_handle_invalid"},
                )

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
        addon.graphql_config["contexts"][self.context_b] = {
            "name": "restricted",
            "graphql_endpoints": [],
            "profiles": {},
        }
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
        self.assertIn("Keep the current Workspace and agent session running", document["message"])
        self.assertIn("separate trusted-host terminal", document["message"])
        self.assertIn("After Apply succeeds", document["message"])
        self.assertIn("retry this request in the same Workspace", document["message"])
        self.assertIn("does not approve or retry automatically", document["message"])
        self.assertNotIn("`exit`", document["message"])
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
        self.assertIn("`tobari cluster denials`", document["message"])
        self.assertIn("read-only diagnostics", document["message"])
        self.assertNotIn("approve", document["message"].lower())
        self.assertNotIn("retry", document["message"].lower())
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
