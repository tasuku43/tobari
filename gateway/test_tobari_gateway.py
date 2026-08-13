import copy
import io
from contextlib import redirect_stdout

import json
import os
import tempfile
import time
import unittest
from unittest import mock

from mitmproxy import http
from mitmproxy.test import tflow

import broker_credentials as broker
import reviewed_credential_profiles as reviewed_profiles
import tobari_gateway as gateway
from reviewed_credential_profiles import (
    AWS_SSO_CREDENTIAL_KIND,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    AWSSigV4Profile,
    DatadogOAuthProfile,
    OpenAICodexOAuthProfile,
    reviewed_gateway_credential_profiles,
)
from tobari_gateway_test_support import ReviewedDynamicCredentialGatewayTestCase

broker_module = broker
BrokerCredentialBindingError = broker.BrokerCredentialBindingError
BrokerCredentialOutcomeUnknown = broker.BrokerCredentialOutcomeUnknown
BrokerCredentialUnavailable = broker.BrokerCredentialUnavailable
BrokeredCredentialAdapter = broker.BrokeredCredentialAdapter
_broker_response = broker._broker_response
call_broker = broker.call_broker
validate_provider_projection = broker.validate_provider_projection


CONTEXT = "01912345-6789-7abc-8def-0123456789ad"
PROJECT = "01912345-6789-7abc-8def-0123456789ab"
HANDLE = "tobari-h1_" + "A" * 43
REVISION = "revision_static"


class ReviewedGatewayCredentialProfileTests(unittest.TestCase):
    def test_registry_is_the_exact_immutable_dynamic_union(self):
        registry = reviewed_gateway_credential_profiles()
        self.assertEqual(
            {kind: type(profile) for kind, profile in registry.items()},
            {
                AWS_SSO_CREDENTIAL_KIND: AWSSigV4Profile,
                DATADOG_OAUTH_CREDENTIAL_KIND: DatadogOAuthProfile,
                OPENAI_CODEX_OAUTH_CREDENTIAL_KIND: OpenAICodexOAuthProfile,
            },
        )
        self.assertEqual(
            {profile.provider_id for profile in registry.values()},
            {"aws", "datadog", "openai"},
        )
        with self.assertRaises(TypeError):
            registry["owner_selected_profile"] = registry[  # type: ignore[index]
                AWS_SSO_CREDENTIAL_KIND
            ]

    def test_profiles_expose_no_request_policy_broker_or_secret_authority(self):
        for name in (
            "BrokeredCredentialAdapter",
            "TobariGateway",
            "http",
            "socket",
            "subprocess",
            "os",
        ):
            self.assertFalse(hasattr(reviewed_profiles, name), name)
        forbidden = {
            "request",
            "policy",
            "broker",
            "caller",
            "socket",
            "secret",
            "executor",
        }
        for profile in reviewed_gateway_credential_profiles().values():
            with self.subTest(provider=profile.provider_id):
                names = {name.lstrip("_").lower() for name in vars(profile)}
                self.assertFalse(names & forbidden)


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


def atomic_json(path, document):
    temporary = path + ".next"
    with open(temporary, "w", encoding="utf-8") as handle:
        json.dump(document, handle)
    os.chmod(temporary, 0o600)
    os.replace(temporary, path)


def principal_registry(workspace_ip="172.29.0.3"):
    return {
        "schema_version": 1,
        "bindings": [
            {
                "project_id": PROJECT,
                "context_id": CONTEXT,
                "context": "default",
                "project_root": "/workspace/project",
                "workspace_ip": workspace_ip,
                "gateway_ip": "172.29.0.2",
                "network": "tobari-project-net",
            }
        ],
    }


class ValidatedRuntimeFileCacheTests(unittest.TestCase):
    def test_provider_projection_caches_one_identity_and_revalidates_replacement(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "providers.json")
            atomic_json(path, projection())
            parser = broker._parse_provider_projection
            with mock.patch.object(
                broker, "_parse_provider_projection", wraps=parser
            ) as validate:
                source = broker.ProviderProjectionSource(path)
                self.assertEqual(source.load(), projection())
                self.assertEqual(source.load(), projection())
                self.assertEqual(validate.call_count, 1)

                atomic_json(path, projection())
                self.assertEqual(source.load(), projection())
                self.assertEqual(validate.call_count, 2)

    def test_provider_projection_invalid_replacement_never_uses_cached_value(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "providers.json")
            atomic_json(path, projection())
            source = broker.ProviderProjectionSource(path)
            self.assertEqual(source.load(), projection())

            atomic_json(path, {})
            with self.assertRaises(BrokerCredentialUnavailable):
                source.load()
            with self.assertRaises(BrokerCredentialUnavailable):
                source.load()

            atomic_json(path, projection())
            self.assertEqual(source.load(), projection())

    def test_provider_projection_rejects_symlink_replacement_and_recovers(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "providers.json")
            replacement = os.path.join(temporary, "replacement.json")
            link = os.path.join(temporary, "providers.next")
            atomic_json(path, projection())
            atomic_json(replacement, projection())
            source = broker.ProviderProjectionSource(path)
            source.load()

            os.symlink(replacement, link)
            os.replace(link, path)
            with self.assertRaises(BrokerCredentialUnavailable):
                source.load()

            atomic_json(path, projection())
            self.assertEqual(source.load(), projection())

    def test_principal_registry_caches_and_observes_atomic_replacement(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "principals.json")
            atomic_json(path, principal_registry())
            parser = gateway._parse_project_principals
            with mock.patch.object(
                gateway, "_parse_project_principals", wraps=parser
            ) as validate:
                source = gateway.PrincipalRegistrySource(path)
                self.assertIn("172.29.0.3", source.load())
                self.assertIn("172.29.0.3", source.load())
                self.assertEqual(validate.call_count, 1)

                atomic_json(path, principal_registry("172.29.0.4"))
                current = source.load()
                self.assertNotIn("172.29.0.3", current)
                self.assertIn("172.29.0.4", current)
                self.assertEqual(validate.call_count, 2)

    def test_principal_registry_invalid_replacement_fails_closed_until_repaired(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "principals.json")
            atomic_json(path, principal_registry())
            source = gateway.PrincipalRegistrySource(path)
            self.assertIn("172.29.0.3", source.load())

            atomic_json(path, {"schema_version": 1, "bindings": "invalid"})
            with self.assertRaises(gateway.PrincipalError):
                source.load()
            with self.assertRaises(gateway.PrincipalError):
                source.load()

            atomic_json(path, principal_registry("172.29.0.4"))
            self.assertIn("172.29.0.4", source.load())

    def test_principal_registry_permission_change_invalidates_cache(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "principals.json")
            atomic_json(path, principal_registry())
            source = gateway.PrincipalRegistrySource(path)
            source.load()

            os.chmod(path, 0o644)
            with self.assertRaises(gateway.PrincipalError):
                source.load()

            os.chmod(path, 0o600)
            self.assertIn("172.29.0.3", source.load())


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


class ReviewedBrokerGatewayTests(unittest.TestCase):
    def request(self, url="https://api.github.com/repos/openai/openai", value=None):
        request = http.Request.make("GET", url)
        if value is not None:
            request.headers["authorization"] = value
        return request

    def test_projection_accepts_static_plan_and_rejects_unreviewed_shapes(self):
        self.assertEqual(broker.validate_provider_projection(projection()), projection())
        for mutation in (
            lambda value: value["providers"][0]["credential"].update(kind="arbitrary_secret"),
            lambda value: value["providers"][0]["acquisition"].update(mode="command"),
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
                json.dump({"schema_version": 1, "bindings": []}, handle)
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
                mock.patch.object(gateway, "_audit") as audit,
            ):
                addon.requestheaders(flow)
            self.assertEqual(calls, ["introspect"], flow.response.content)
            commit.assert_not_called()
            self.assertEqual(audit.call_args.kwargs["scheme"], "https")
            self.assertEqual(flow.response.status_code, 403)
            self.assertNotIn("authorization", flow.request.headers)

    def test_graphql_policy_denial_preserves_scheme_and_never_commits_upstream(self):
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.cluster = "default"
        addon.opa_url = "http://opa.invalid/decision"
        addon.opa_timeout = 2
        request = http.Request.make(
            "POST",
            "https://graphql.example.com/graphql",
            content=b'{"query":"mutation { closeIssue updateIssue }"}',
            headers={"content-type": "application/json"},
        )
        flow = tflow.tflow(req=request)
        credential_request = mock.Mock(secret_headers=set(), broker_provider=None)
        pending = {
            "started": time.monotonic(),
            "request_id": "a" * 32,
            "scheme": "https",
            "host": "graphql.example.com",
            "port": 443,
            "audit_path": "/graphql",
            "principal": {
                "project_id": PROJECT, "context_id": CONTEXT,
                "context": "default", "project_root": "/workspace/project",
            },
            "credential_request": credential_request,
        }
        with (
            mock.patch.object(gateway, "query_opa", return_value=gateway.Decision(
                allow=False, reason="review required", status_code=403,
                learnable=True,
            )),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
            mock.patch.object(gateway, "_audit") as audit,
        ):
            addon._complete_graphql_request(flow, pending)
        credential_request.apply.assert_not_called()
        commit.assert_not_called()
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(len(audit.call_args_list), 2)
        self.assertTrue(all(call.kwargs["scheme"] == "https" for call in audit.call_args_list))

    def test_broker_error_maps_reviewed_dynamic_outcomes(self):
        with self.assertRaises(broker.BrokerCredentialBindingError):
            broker._broker_response(json.dumps({
                "schema_version": 1, "ok": False,
                "error": {"code": "handle_revoked"},
            }).encode())
        with self.assertRaises(broker.BrokerCredentialOutcomeUnknown):
            broker._broker_response(json.dumps({
                "schema_version": 1, "ok": False,
                "error": {"code": "companion_outcome_unknown"},
            }).encode())
        with self.assertRaises(broker.BrokerCredentialBindingError):
            broker._broker_response(json.dumps({
                "schema_version": 1, "ok": False,
                "error": {"code": "aws_signing_request_invalid"},
            }).encode())




class ReviewedDynamicCredentialGatewayTests(ReviewedDynamicCredentialGatewayTestCase):

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
            return_value=gateway.Decision(True, "allowed", 403, False),
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
            return_value=gateway.Decision(True, "allowed", 403, False),
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
            return gateway.Decision(True, "allowed", 403, False)

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(gateway, "query_opa", side_effect=allow):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)

        self.assertEqual(events, ["introspect", "policy", "resolve"])
        self.assertEqual(
            captured["authorization"],
            {"broker_provider": "openai"},
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
            return gateway.Decision(False, "denied", 403, True)

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
                    True, "allowed", 403, False
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
            return_value=gateway.Decision(True, "allowed", 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual(flow.response.status_code, 503)
        self.assertNotIn("authorization", flow.request.headers)


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
                    return_value=gateway.Decision(True, "allowed", 403, False),
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


if __name__ == "__main__":
    unittest.main()
