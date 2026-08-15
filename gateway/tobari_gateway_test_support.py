import json
import os
import tempfile
import unittest
from types import SimpleNamespace

from mitmproxy import http
from mitmproxy.test import tflow

import tobari_gateway as gateway
from broker_credentials import BrokeredCredentialAdapter


class ReviewedDynamicCredentialGatewayTestCase(unittest.TestCase):
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
            "version": "v1",
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
                    "schema_version": 1,
                    "bindings": [
                        {
                            "project_id": self.project_a,
                            "context_id": self.context_a,
                            "context": "default",
                            "project_root": "/workspace/project-a",
                            "workspace_ip": "172.29.0.3",
                            "gateway_ip": "172.29.0.2",
                            "network": "tobari-project-a-net",
                        }
                    ],
                },
                handle,
            )
        os.chmod(self.principal_path, 0o600)
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
            "json_merges": [],
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
                "helper": "claude-native-oauth",
            },
            "credential": {"kind": "anthropic_claude_oauth_session"},
            "workspace_projections": [
                {
                    "kind": "complete_file",
                    "path": ".claude/.credentials.json",
                    "template": (
                        '{"claudeAiOauth":{"accessToken":"${HANDLE}","refreshToken":"dummy-value",'
                        '"expiresAt":4102444800000,'
                        '"scopes":${OAUTH_SCOPES_JSON},'
                        '"subscriptionType":${CLAUDE_SUBSCRIPTION_TYPE_JSON},'
                        '"rateLimitTier":${CLAUDE_RATE_LIMIT_TIER_JSON}}}'
                    ),
                },
                {
                    "kind": "merge_json",
                    "path": ".claude.json",
                    "template": '{"hasCompletedOnboarding":true}',
                },
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
                        "secret_field": "anthropic_claude_oauth_session",
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
                "secret_field": "anthropic_claude_oauth_session",
            },
            "secret_headers": ["authorization"],
        }
        return {
            "schema_version": 1,
            "providers": [provider],
            "environment": [],
            "complete_files": [
                {
                    "provider_id": "anthropic",
                    "path": ".claude/.credentials.json",
                    "template": provider["workspace_projections"][0]["template"],
                }
            ],
            "json_merges": [
                {
                    "provider_id": "anthropic",
                    "path": ".claude.json",
                    "template": provider["workspace_projections"][1]["template"],
                }
            ],
            "header_bindings": [normalized],
            "secret_headers": ["authorization"],
        }

    @staticmethod
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
            "json_merges": [],
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
    @staticmethod
    def datadog_oauth_provider_projection():
        provider = {
            "schema_version": 1,
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
            "schema_version": 1,
            "providers": [provider],
            "environment": [
                {"provider_id": "datadog", "name": "DD_ACCESS_TOKEN", "template": "${HANDLE}"},
                {"provider_id": "datadog", "name": "DD_SITE", "template": "datadoghq.com"},
            ],
            "complete_files": [],
            "json_merges": [],
            "header_bindings": [normalized],
            "signing_bindings": [],
            "secret_headers": ["authorization"],
        }

    @staticmethod
    @staticmethod
    def openai_codex_oauth_provider_projection():
        auth_template = (
            '{"auth_mode":"chatgptAuthTokens","OPENAI_API_KEY":null,'
            '"tokens":{"id_token":"e30.e30.x","access_token":"${HANDLE}",'
            '"refresh_token":"","account_id":null},'
            '"last_refresh":"1970-01-01T00:00:00Z"}'
        )
        provider = {
            "schema_version": 1,
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
            "schema_version": 1,
            "providers": [provider],
            "environment": [],
            "complete_files": [
                {
                    "provider_id": "openai",
                    "path": ".codex/auth.json",
                    "template": auth_template,
                }
            ],
            "json_merges": [],
            "header_bindings": [normalized],
            "signing_bindings": [],
            "secret_headers": [
                "authorization",
                "chatgpt-account-id",
                "x-openai-fedramp",
            ],
        }

    @staticmethod
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
            "schema_version": 1,
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
            "schema_version": 1,
            "providers": [provider],
            "environment": [
                {"provider_id": "aws", "name": "AWS_ACCESS_KEY_ID", "template": "${HANDLE}"},
                {"provider_id": "aws", "name": "AWS_EC2_METADATA_DISABLED", "template": "true"},
                {"provider_id": "aws", "name": "AWS_SECRET_ACCESS_KEY", "template": "${HANDLE}"},
                {"provider_id": "aws", "name": "AWS_SESSION_TOKEN", "template": "${HANDLE}"},
            ],
            "complete_files": [],
            "json_merges": [],
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
        original_host = flow.request.host
        original_port = flow.request.port
        tls_established = flow.request.scheme == "https"
        default_port = 443 if tls_established else 80
        host_header = original_host
        if original_port != default_port:
            host_header = f"{original_host}:{original_port}"
        flow.request.headers["host"] = host_header
        flow.client_conn = SimpleNamespace(
            peername=("172.29.0.3", 51000),
            sockname=("198.18.0.10", original_port),
            tls_established=tls_established,
            sni=original_host if tls_established else None,
            proxy_mode=SimpleNamespace(type_name="transparent"),
        )
        flow.server_conn.address = ("198.18.0.10", original_port)
        return flow
