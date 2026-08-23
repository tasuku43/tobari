import copy
import io
from contextlib import redirect_stdout

import json
import os
import socket
import tempfile
import threading
import time
import unittest
from datetime import datetime, timezone
from unittest import mock

from mitmproxy import http
from mitmproxy.test import tflow

import broker_credentials as broker
import reviewed_credential_profiles as reviewed_profiles
import tobari_gateway as gateway
from reviewed_credential_profiles import (
    ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND,
    AWS_SSO_CREDENTIAL_KIND,
    DATADOG_OAUTH_CREDENTIAL_KIND,
    OPENAI_CODEX_OAUTH_CREDENTIAL_KIND,
    AWSSigV4Profile,
    AnthropicClaudeProfile,
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
BrokerAuthenticationRequired = broker.BrokerAuthenticationRequired
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
                ANTHROPIC_CLAUDE_OAUTH_CREDENTIAL_KIND: AnthropicClaudeProfile,
                AWS_SSO_CREDENTIAL_KIND: AWSSigV4Profile,
                DATADOG_OAUTH_CREDENTIAL_KIND: DatadogOAuthProfile,
                OPENAI_CODEX_OAUTH_CREDENTIAL_KIND: OpenAICodexOAuthProfile,
            },
        )
        self.assertEqual(
            {profile.provider_id for profile in registry.values()},
            {"anthropic", "aws", "datadog", "openai"},
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
        "json_merges": [],
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


def permission_session_registry(now=None):
    now = time.time() if now is None else now
    created = datetime.fromtimestamp(now - 5, timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")
    issued = datetime.fromtimestamp(now, timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")
    expires = datetime.fromtimestamp(now + 30, timezone.utc).isoformat(timespec="microseconds").replace("+00:00", "Z")
    principal = gateway._parse_project_principals(json.dumps(principal_registry()).encode())["172.29.0.3"]
    return {
        "schema_version": 2,
        "sessions": [{
            "schema_version": 2,
            "workspace_manifest_id": CONTEXT,
            "workspace_id": PROJECT,
            "attachment_id": "att_" + "1" * 32,
            "owner_kind": "interactive_workspace",
            "frozen_principal_fingerprint": principal["_frozen_principal_fingerprint"],
            "owner_pid": 42,
            "ingestion_socket": "pws_" + "2" * 32 + ".sock",
            "ingestion_nonce": "3" * 64,
            "created_at": created,
            "lease_issued_at": issued,
            "expires_at": expires,
        }],
    }


def host_loopback_registry(epoch="att_" + "1" * 32):
    material = "\x00".join(("tobari-host-loopback-route-v1", epoch, CONTEXT, PROJECT))
    route_id = "hlr_" + gateway.hashlib.sha256(material.encode()).hexdigest()[:32]
    route = {
        "id": route_id, "attachment_epoch_id": epoch, "context_id": CONTEXT,
        "context": "default", "project_id": PROJECT, "project_root": "/workspace/project",
        "hostname": "host.tobari.test",
        "relay_port": 43179, "relay_token": "3" * 64,
    }
    return {"schema_version": 1, "routes": [route]}, route


def attachment_grant(route, decision="allow", target_port=3000):
    grant = {
        "decision": decision, "lifetime": "attachment", "destination_kind": "host_loopback",
        "context_id": CONTEXT, "project_id": PROJECT,
        "attachment_epoch_id": route["attachment_epoch_id"],
        "host": route["hostname"], "target_port": target_port, "method": "GET",
        "path": "/health", "source_candidate": "pcy_" + "2" * 32,
    }
    material = "\x00".join(("tobari-attachment-grant-v2", grant["decision"], grant["context_id"], grant["project_id"], grant["attachment_epoch_id"], grant["host"], str(grant["target_port"]), grant["method"], grant["path"], grant["source_candidate"]))
    grant["id"] = "pag_" + gateway.hashlib.sha256(material.encode()).hexdigest()[:32]
    return grant


class PermissionResumeProjectionTests(unittest.TestCase):
    def test_frozen_principal_projects_to_schema2_without_widening_opa_wire(self):
        principal = gateway._parse_project_principals(
            json.dumps(principal_registry()).encode()
        )["172.29.0.3"]
        request = http.Request.make("GET", "https://api.example.com/a%2Fb")
        flow = tflow.tflow(req=request)
        with mock.patch.object(
            gateway, "request_authority", return_value=("https", "api.example.com", 443)
        ):
            policy_input = gateway.build_policy_input(flow, "default", principal, set())
        self.assertEqual(policy_input["schema_version"], 1)
        self.assertEqual(policy_input["principal"], {
            "cluster": "default", "context_id": CONTEXT, "project_id": PROJECT,
        })
        self.assertNotIn("workspace_manifest_id", json.dumps(policy_input))
        self.assertRegex(principal["_frozen_principal_fingerprint"], r"^[0-9a-f]{64}$")

    def test_session_registry_is_exact_current_and_service_owner_ineligible(self):
        now = time.time()
        document = permission_session_registry(now)
        sessions = gateway._parse_permission_sessions(json.dumps(document).encode())
        self.assertEqual(len(sessions), 1)
        principal = gateway._parse_project_principals(
            json.dumps(principal_registry()).encode()
        )["172.29.0.3"]
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "sessions.json")
            atomic_json(path, document)
            source = gateway.PermissionSessionSource(path)
            self.assertEqual(source.resolve(principal, now)["attachment_id"], "att_" + "1" * 32)
            with self.assertRaises(gateway.PermissionSessionError):
                source.resolve(principal, now + 31)
        service = copy.deepcopy(document)
        service["sessions"][0]["owner_kind"] = "service_exposure_controller"
        with self.assertRaises(gateway.PermissionSessionError):
            gateway._parse_permission_sessions(json.dumps(service).encode())

    def test_exact_effect_rejects_unrepresentable_or_divergent_paths(self):
        base = {
            "request": {
                "authority": {"scheme": "https", "host": "api.example.com", "port": 443},
                "method": "GET",
                "path": {"raw": "/a%2Fb//./../%E3%81%82", "segments": ["a/b", ".", "..", "あ"]},
            }
        }
        self.assertEqual(
            gateway._permission_wait_effect(base)["segments"],
            ["a/b", ".", "..", "あ"],
        )
        for raw, segments in (
            ("/bad%", ["bad%"]),
            ("/bad%ff", ["bad�"]),
            ("/a%2Fb", ["a", "b"]),
        ):
            invalid = copy.deepcopy(base)
            invalid["request"]["path"] = {"raw": raw, "segments": segments}
            with self.subTest(raw=raw):
                self.assertIsNone(gateway._permission_wait_effect(invalid))

    def test_resume_is_published_only_after_exact_owner_ack(self):
        principal = gateway._parse_project_principals(
            json.dumps(principal_registry()).encode()
        )["172.29.0.3"]
        session = permission_session_registry()["sessions"][0]
        policy_input = {
            "request": {
                "authority": {"scheme": "https", "host": "api.example.com", "port": 443},
                "method": "GET", "path": {"raw": "/items/a%20b", "segments": ["items", "a b"]},
            }
        }
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.permission_session_source = mock.Mock()
        addon.permission_session_source.resolve.return_value = session
        addon.permission_socket_directory = "/run/tobari/permission-ingestion"
        captured = []

        def register(_directory, _session, record):
            captured.append(record)
            return True

        with (
            mock.patch.object(gateway, "register_permission_wait", side_effect=register),
            mock.patch.object(gateway.secrets, "token_hex", return_value="4" * 32),
        ):
            resume = addon._permission_resume(principal, policy_input, "5" * 32)
        self.assertEqual(resume["command"], "tobari-permission wait --id pwt_" + "4" * 32)
        self.assertEqual(captured[0]["workspace_manifest_id"], CONTEXT)
        self.assertEqual(captured[0]["workspace_id"], PROJECT)
        self.assertEqual(captured[0]["effect"], gateway._permission_wait_effect(policy_input))
        self.assertNotIn("context_id", captured[0])
        self.assertNotIn("project_id", captured[0])
        addon.permission_session_source.confirm.assert_called_once()
        with mock.patch.object(gateway, "register_permission_wait", return_value=False):
            self.assertIsNone(addon._permission_resume(principal, policy_input, "5" * 32))

    def test_resume_is_omitted_when_session_authority_drifts_after_ack(self):
        principal = gateway._parse_project_principals(
            json.dumps(principal_registry()).encode()
        )["172.29.0.3"]
        policy_input = {
            "request": {
                "authority": {"scheme": "https", "host": "api.example.com", "port": 443},
                "method": "GET", "path": {"raw": "/items", "segments": ["items"]},
            }
        }
        with tempfile.TemporaryDirectory() as temporary:
            registry_path = os.path.join(temporary, "sessions.json")
            document = permission_session_registry()
            atomic_json(registry_path, document)
            addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
            addon.permission_session_source = gateway.PermissionSessionSource(registry_path)
            addon.permission_socket_directory = temporary

            def acknowledge_then_replace(_directory, _session, _record):
                replaced = copy.deepcopy(document)
                replaced["sessions"][0]["ingestion_nonce"] = "f" * 64
                atomic_json(registry_path, replaced)
                return True

            with mock.patch.object(
                gateway, "register_permission_wait", side_effect=acknowledge_then_replace
            ):
                self.assertIsNone(
                    addon._permission_resume(principal, policy_input, "5" * 32)
                )

    def test_owner_ack_protocol_is_bounded_and_accepts_fragmented_ack(self):
        session = permission_session_registry()["sessions"][0]
        record = {
            "schema_version": 2,
            "permission_wait_id": "pwt_" + "4" * 32,
            "denial_correlation_id": "5" * 32,
        }
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, session["ingestion_socket"])
            listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            listener.bind(path)
            os.chmod(path, 0o600)
            listener.listen(1)
            observed = []

            def serve():
                connection, _ = listener.accept()
                with connection:
                    header = b""
                    while len(header) < 69:
                        header += connection.recv(69 - len(header))
                    length = int.from_bytes(header[65:69], "big")
                    payload = b""
                    while len(payload) < length:
                        payload += connection.recv(length - len(payload))
                    observed.append((header[:65], json.loads(payload)))
                    connection.sendall(b"O")
                    connection.sendall(b"K")

            worker = threading.Thread(target=serve)
            worker.start()
            try:
                self.assertTrue(gateway.register_permission_wait(temporary, session, record))
            finally:
                worker.join(timeout=1)
                listener.close()
            self.assertEqual(observed[0][0], b"W" + session["ingestion_nonce"].encode())
            self.assertEqual(observed[0][1], record)

    def test_owner_ack_rejects_unsafe_directory_and_socket_nodes(self):
        session = permission_session_registry()["sessions"][0]
        record = {"schema_version": 2, "permission_wait_id": "pwt_" + "4" * 32}
        with tempfile.TemporaryDirectory() as temporary:
            os.chmod(temporary, 0o755)
            self.assertFalse(
                gateway.register_permission_wait(temporary, session, record)
            )
        with tempfile.TemporaryDirectory() as parent:
            target = os.path.join(parent, "actual")
            linked = os.path.join(parent, "linked")
            os.mkdir(target, 0o700)
            os.symlink(target, linked)
            self.assertFalse(gateway.register_permission_wait(linked, session, record))
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, session["ingestion_socket"])
            listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            listener.bind(path)
            try:
                os.chmod(path, 0o666)
                self.assertFalse(
                    gateway.register_permission_wait(temporary, session, record)
                )
            finally:
                listener.close()
        with tempfile.TemporaryDirectory() as temporary:
            target = os.path.join(temporary, "actual.sock")
            path = os.path.join(temporary, session["ingestion_socket"])
            listener = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
            listener.bind(target)
            os.chmod(target, 0o600)
            os.symlink(target, path)
            try:
                self.assertFalse(
                    gateway.register_permission_wait(temporary, session, record)
                )
            finally:
                listener.close()

    def test_denial_schema2_exposes_no_frozen_alias_and_protocol_has_no_resume(self):
        principal = {"context_id": CONTEXT, "project_id": PROJECT}
        flow = tflow.tflow(req=http.Request.make("GET", "https://api.example.com/items"))
        resume = {
            "available": True, "run_on": "workspace",
            "command": "tobari-permission wait --id pwt_" + "4" * 32,
            "automatic_retry": False, "result_values": ["allow", "deny", "expired"],
        }
        gateway._policy_denied(flow, 403, True, principal, resume)
        denial = json.loads(flow.response.content)
        self.assertEqual(denial["tobari"]["schema_version"], 2)
        self.assertEqual(denial["tobari"]["workspace_manifest_id"], CONTEXT)
        self.assertEqual(denial["tobari"]["workspace_id"], PROJECT)
        self.assertNotIn("context_id", denial["tobari"])
        self.assertNotIn("project_id", denial["tobari"])
        self.assertEqual(denial["tobari"]["resume"], resume)


class ProtocolEndpointDeclarationTests(unittest.TestCase):

    def test_semantic_endpoint_declarations_are_exact_and_cross_protocol_collisions_fail(self):
        graphql = {"scheme": "https", "host": "api.example.com", "port": 443, "path": "/graphql"}
        mcp = {"scheme": "https", "host": "api.example.com", "port": 443, "path": "/mcp"}
        kubernetes = {"scheme": "https", "host": "cluster.example.com", "port": 443, "path": "/"}
        document = {
            "version": "v1",
            "contexts": {
                CONTEXT: {
                    "name": "default",
                    "graphql_endpoints": [graphql],
                    "mcp_endpoints": [mcp],
                    "kubernetes_endpoints": [kubernetes],
                }
            },
        }
        with tempfile.TemporaryDirectory() as temporary:
            path = os.path.join(temporary, "gateway.json")
            atomic_json(path, document)
            loaded = gateway.load_gateway_config(path)

            self.assertTrue(gateway.graphql_endpoint_declared(
                loaded, CONTEXT, "https", "api.example.com", 443, "/graphql"
            ))
            self.assertFalse(gateway.graphql_endpoint_declared(
                loaded, CONTEXT, "https", "api.example.com", 443, "/graphql/child"
            ))
            self.assertTrue(gateway.mcp_endpoint_declared(
                loaded, CONTEXT, "https", "api.example.com", 443, "/mcp"
            ))
            self.assertFalse(gateway.mcp_endpoint_declared(
                loaded, CONTEXT, "https", "other.example.com", 443, "/mcp"
            ))
            self.assertTrue(gateway.kubernetes_endpoint_declared(
                loaded, CONTEXT, "https", "cluster.example.com", 443
            ))
            self.assertFalse(gateway.kubernetes_endpoint_declared(
                loaded, CONTEXT, "https", "cluster.example.com", 8443
            ))

            colliding = copy.deepcopy(document)
            colliding["contexts"][CONTEXT]["mcp_endpoints"] = [graphql]
            atomic_json(path, colliding)
            with self.assertRaises(gateway.CredentialError):
                gateway.load_gateway_config(path)


class ValidatedRuntimeFileCacheTests(unittest.TestCase):

    def test_host_loopback_source_binds_workspace_epoch_port_and_attachment_grants(self):
        with tempfile.TemporaryDirectory() as temporary:
            service_path = os.path.join(temporary, "services.json")
            grant_path = os.path.join(temporary, "grants.json")
            registry, route = host_loopback_registry()
            grant = attachment_grant(route)
            atomic_json(service_path, registry)
            atomic_json(grant_path, {"schema_version": 1, "grants": [grant]})
            source = gateway.HostLoopbackRegistrySource(service_path, grant_path)
            principal = {"context_id": CONTEXT, "project_id": PROJECT}
            resolved, grants = source.resolve(principal, "http", route["hostname"], 3000)
            self.assertEqual(resolved, route)
            self.assertEqual(grants, [grant])
            with self.assertRaises(gateway.HostLoopbackError):
                source.resolve({**principal, "project_id": "01912345-6789-7abc-8def-0123456789ac"}, "http", route["hostname"], 3000)
            with self.assertRaises(gateway.HostLoopbackError):
                source.resolve(principal, "https", route["hostname"], 3000)

    def test_host_loopback_registry_rejects_identity_or_relay_rebinding(self):
        registry, _ = host_loopback_registry()
        for field, value in (("context_id", "01912345-6789-7abc-8def-0123456789ac"), ("relay_token", "short")):
            changed = copy.deepcopy(registry)
            changed["routes"][0][field] = value
            with self.subTest(field=field):
                with self.assertRaises(gateway.HostLoopbackError):
                    gateway._parse_host_loopback_routes(json.dumps(changed).encode())

    def test_attachment_grant_identity_binds_target_port(self):
        _, route = host_loopback_registry()
        grant = attachment_grant(route, target_port=3000)
        changed = copy.deepcopy(grant)
        changed["target_port"] = 3001
        with self.assertRaises(gateway.HostLoopbackError):
            gateway._parse_attachment_grants(json.dumps({"schema_version": 1, "grants": [changed]}).encode())

    def test_host_loopback_policy_precedes_port_bound_authenticated_relay_selection(self):
        _, route = host_loopback_registry()
        grant = attachment_grant(route)
        for allowed in (False, True):
            with self.subTest(allowed=allowed):
                addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
                addon.cluster, addon.opa_url, addon.opa_timeout = "default", "http://opa.invalid", 2
                addon.graphql_config = {}
                addon.principal_source = mock.Mock()
                addon.principal_source.load.return_value = {}
                addon.host_loopback_source = mock.Mock()
                addon.host_loopback_source.resolve.return_value = (route, [grant])
                addon.host_loopback_bridges = mock.Mock()
                addon.host_loopback_bridges.open.return_value = ("127.0.0.1", 45000)
                prepared = mock.Mock(secret_headers=set(), broker_provider=None, deferred=False)
                addon.credential_adapter = mock.Mock()
                addon.credential_adapter.prepare.return_value = prepared
                request = http.Request.make("GET", "http://host.tobari.test:3000/health")
                request.headers["Host"] = "host.tobari.test:3000"
                flow = tflow.tflow(req=request)
                captured = []

                def decide(_url, document, _timeout):
                    captured.append(document)
                    return gateway.Decision(allow=allowed, reason="attachment", status_code=403, learnable=not allowed)

                with (
                    mock.patch.object(gateway, "normalize_ingress_authority", return_value=("http", route["hostname"], 3000)),
                    mock.patch.object(gateway, "resolve_project_principal", return_value={"project_id": PROJECT, "context_id": CONTEXT, "context": "default", "project_root": "/workspace/project"}),
                    mock.patch.object(gateway, "graphql_endpoint_declared", return_value=False),
                    mock.patch.object(gateway, "mcp_endpoint_declared", return_value=False),
                    mock.patch.object(gateway, "query_opa", side_effect=decide),
                    mock.patch.object(gateway, "_audit"),
                ):
                    addon.requestheaders(flow)
                self.assertEqual(captured[0]["destination"]["attachment_epoch_id"], route["attachment_epoch_id"])
                self.assertEqual(captured[0]["request"]["authority"]["port"], 3000)
                self.assertNotIn(route["relay_token"], json.dumps(captured[0]))
                if allowed:
                    addon.host_loopback_bridges.open.assert_called_once_with(route["relay_port"], route["relay_token"], 3000)
                    self.assertEqual(flow.server_conn.address, ("127.0.0.1", 45000))
                    self.assertEqual((flow.request.host, flow.request.port), ("127.0.0.1", 45000))
                    self.assertEqual(flow.request.host_header, "host.tobari.test:3000")
                else:
                    addon.host_loopback_bridges.open.assert_not_called()
                    self.assertEqual(flow.response.status_code, 403)
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

    def test_gateway_wire_rejects_renamed_workspace_identity_aliases(self):
        principal = principal_registry()
        binding = principal["bindings"][0]
        binding["workspace_id"] = binding.pop("project_id")
        binding["workspace_manifest_id"] = binding.pop("context_id")
        binding["workspace_manifest"] = binding.pop("context")
        with self.assertRaises(gateway.PrincipalError):
            gateway._parse_project_principals(json.dumps(principal).encode())

        routes, route = host_loopback_registry()
        route["workspace_id"] = route.pop("project_id")
        route["workspace_manifest_id"] = route.pop("context_id")
        route["workspace_manifest"] = route.pop("context")
        with self.assertRaises(gateway.HostLoopbackError):
            gateway._parse_host_loopback_routes(json.dumps(routes).encode())

        _, canonical_route = host_loopback_registry()
        grant = attachment_grant(canonical_route)
        grant["workspace_id"] = grant.pop("project_id")
        grant["workspace_manifest_id"] = grant.pop("context_id")
        with self.assertRaises(gateway.HostLoopbackError):
            gateway._parse_attachment_grants(
                json.dumps({"schema_version": 1, "grants": [grant]}).encode()
            )

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


class StandardNativeLoginGatewayTests(unittest.TestCase):
    def test_unregistered_principal_denial_audits_explicit_null_scope(self):
        with (
            mock.patch.dict(
                os.environ, {"TOBARI_AUTH_PROVIDER_PROJECTION": ""}, clear=False
            ),
            mock.patch.object(gateway, "load_gateway_config", return_value={}),
        ):
            addon = gateway.TobariGateway()
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        request = http.Request.make(
            "POST", "https://api.anthropic.com/api/event_logging/v2/batch"
        )
        flow = tflow.tflow(req=request)
        with (
            mock.patch.object(
                gateway,
                "normalize_ingress_authority",
                return_value=("https", "api.anthropic.com", 443),
            ),
            mock.patch.object(
                gateway,
                "resolve_project_principal",
                side_effect=gateway.PrincipalError(
                    "project principal is not registered"
                ),
            ),
            mock.patch.object(gateway, "_audit") as audit,
        ):
            addon.requestheaders(flow)

        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(
            {
                name: audit.call_args.kwargs[name]
                for name in ("context", "context_id", "project_id", "project_root")
            },
            {
                "context": None,
                "context_id": None,
                "project_id": None,
                "project_root": None,
            },
        )
        self.assertEqual(
            audit.call_args.kwargs["reason"],
            "project principal is not registered",
        )
        self.assertFalse(audit.call_args.kwargs["learnable"])

    def test_claude_and_codex_native_auth_bypass_broker_after_policy_allow(self):
        cases = (
            ("GET", "https://platform.claude.com/v1/oauth/hello"),
            ("POST", "https://platform.claude.com/v1/oauth/token"),
            ("GET", "https://api.anthropic.com/api/oauth/claude_cli/roles"),
            ("GET", "https://api.anthropic.com/api/oauth/profile"),
            ("POST", "https://auth.openai.com/oauth/token"),
            ("POST", "https://auth.openai.com/api/accounts/deviceauth/usercode"),
            ("POST", "https://auth.openai.com/api/accounts/deviceauth/token"),
        )
        for method, url in cases:
            with self.subTest(method=method, url=url):
                with (
                    mock.patch.dict(
                        os.environ,
                        {"TOBARI_AUTH_PROVIDER_PROJECTION": ""},
                        clear=False,
                    ),
                    mock.patch.object(gateway, "load_gateway_config", return_value={}),
                ):
                    addon = gateway.TobariGateway()
                self.assertEqual(addon.credential_adapter.name, "passthrough")
                addon.principal_source = mock.Mock()
                addon.principal_source.load.return_value = {}
                request = http.Request.make(
                    method, url, headers={"authorization": "Bearer native-canary"}
                )
                flow = tflow.tflow(req=request)
                decision_inputs = []

                def allow(_url, document, _timeout):
                    decision_inputs.append(document)
                    return gateway.Decision(
                        allow=True, reason="agent-ready", status_code=200,
                        learnable=False,
                    )

                with (
                    mock.patch.object(
                        gateway, "normalize_ingress_authority",
                        return_value=("https", request.host, 443),
                    ),
                    mock.patch.object(
                        gateway, "resolve_project_principal", return_value={
                            "project_id": PROJECT, "context_id": CONTEXT,
                            "context": "default", "project_root": "/workspace/project",
                        },
                    ),
                    mock.patch.object(
                        gateway, "graphql_endpoint_declared", return_value=False
                    ),
                    mock.patch.object(
                        gateway, "mcp_endpoint_declared", return_value=False
                    ),
                    mock.patch.object(gateway, "query_opa", side_effect=allow),
                    mock.patch.object(gateway, "commit_upstream_authority") as commit,
                    mock.patch.object(gateway, "_audit"),
                ):
                    addon.requestheaders(flow)

                    if url.endswith("/api/oauth/profile"):
                        provider_profile = {
                            "subscriptionType": "max",
                            "rateLimitTier": "default_claude_max_20x",
                        }
                        flow.response = http.Response.make(
                            200,
                            json.dumps(provider_profile),
                            {"content-type": "application/json"},
                        )
                        addon.responseheaders(flow)
                        addon.response(flow)
                        self.assertEqual(
                            json.loads(flow.response.content), provider_profile
                        )

                if not url.endswith("/api/oauth/profile"):
                    self.assertIsNone(
                        flow.response,
                        flow.response.content if flow.response is not None else None,
                    )
                self.assertEqual(
                    flow.request.headers["authorization"], "Bearer native-canary"
                )
                self.assertEqual(len(decision_inputs), 1)
                self.assertNotIn("native-canary", json.dumps(decision_inputs[0]))
                commit.assert_called_once_with(flow)

    def test_mcp_tool_call_is_buffered_and_exposes_only_method_and_tool(self):
        body = json.dumps({
            "jsonrpc": "2.0", "id": 7, "method": "tools/call",
            "params": {"name": "codex_apps.search", "arguments": {"secret": "argument-canary"}},
        }, separators=(",", ":")).encode()
        with (
            mock.patch.dict(os.environ, {"TOBARI_AUTH_PROVIDER_PROJECTION": ""}, clear=False),
            mock.patch.object(gateway, "load_gateway_config", return_value={}),
        ):
            addon = gateway.TobariGateway()
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        request = http.Request.make(
            "POST", "https://chatgpt.com/backend-api/ps/mcp", content=body,
            headers={"content-type": "application/json", "content-length": str(len(body))},
        )
        flow = tflow.tflow(req=request)
        inputs = []

        def deny(_url, document, _timeout):
            inputs.append(document)
            return gateway.Decision(allow=False, reason="review", status_code=403, learnable=True)

        with (
            mock.patch.object(gateway, "normalize_ingress_authority", return_value=("https", "chatgpt.com", 443)),
            mock.patch.object(gateway, "resolve_project_principal", return_value={"project_id": PROJECT, "context_id": CONTEXT, "context": "default", "project_root": "/workspace/project"}),
            mock.patch.object(gateway, "graphql_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "mcp_endpoint_declared", return_value=True),
            mock.patch.object(gateway, "query_opa", side_effect=deny),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
            mock.patch.object(gateway, "_audit") as audit,
        ):
            addon.requestheaders(flow)
            self.assertIsNone(flow.response)
            self.assertFalse(flow.request.stream)
            addon.request(flow)
        self.assertEqual(inputs[0]["request"]["mcp"], {"method": "tools/call", "tool_name": "codex_apps.search"})
        self.assertNotIn("argument-canary", json.dumps(inputs[0]))
        self.assertNotIn("argument-canary", flow.response.content.decode())
        self.assertEqual(audit.call_args.kwargs["mcp_tool_name"], "codex_apps.search")
        self.assertNotIn("argument-canary", json.dumps(audit.call_args.kwargs))
        commit.assert_not_called()


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

    def test_declared_binding_rejects_direct_credential_without_fallback(self):
        fallback = mock.Mock()
        caller = mock.Mock()
        adapter = broker.BrokeredCredentialAdapter(
            fallback=fallback, projection_path="/projection.json",
            socket_path="/broker.sock", timeout=2,
            projection_loader=lambda _: projection(), caller=caller,
        )
        request = self.request(value="Bearer real-workspace-token")
        with self.assertRaises(BrokerAuthenticationRequired):
            adapter.prepare(
                request, "https", "api.github.com", 443, CONTEXT, PROJECT
            )
        self.assertNotIn("authorization", request.headers)
        caller.assert_not_called()
        fallback.prepare.assert_not_called()

    def test_undeclared_binding_retains_workspace_owned_fallback(self):
        prepared = mock.Mock(secret_headers={"authorization"}, broker_provider=None)
        fallback = mock.Mock()
        fallback.prepare.return_value = prepared
        caller = mock.Mock()
        adapter = broker.BrokeredCredentialAdapter(
            fallback=fallback, projection_path="/projection.json",
            socket_path="/broker.sock", timeout=2,
            projection_loader=lambda _: projection(), caller=caller,
        )
        request = self.request(
            url="https://api.example.com/", value="Bearer real-workspace-token"
        )
        selected = adapter.prepare(
            request, "https", "api.example.com", 443, CONTEXT, PROJECT
        )
        self.assertIs(selected.request, prepared)
        self.assertEqual(request.headers["authorization"], "Bearer real-workspace-token")
        caller.assert_not_called()
        fallback.prepare.assert_called_once()

    def test_broker_required_fault_is_terminal_before_policy(self):
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.cluster = "default"
        addon.opa_url = "http://opa.invalid/decision"
        addon.opa_timeout = 2
        addon.graphql_config = {}
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        addon.credential_adapter = mock.Mock()
        addon.credential_adapter.prepare.side_effect = BrokerAuthenticationRequired(
            "broker authentication is required for this provider binding"
        )
        flow = tflow.tflow(req=self.request(value="Bearer real-workspace-token"))
        with (
            mock.patch.object(
                gateway,
                "normalize_ingress_authority",
                return_value=("https", "api.github.com", 443),
            ),
            mock.patch.object(gateway, "resolve_project_principal", return_value={
                "project_id": PROJECT, "context_id": CONTEXT,
                "context": "default", "project_root": "/workspace/project",
            }),
            mock.patch.object(gateway, "query_opa") as query,
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
            mock.patch.object(gateway, "_audit") as audit,
        ):
            addon.requestheaders(flow)
        query.assert_not_called()
        commit.assert_not_called()
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(
            json.loads(flow.response.content), {"error": "broker_auth_required"}
        )
        self.assertFalse(audit.call_args.kwargs["learnable"])
        self.assertNotIn("real-workspace-token", str(audit.call_args.kwargs))

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
                mock.patch.object(gateway, "mcp_endpoint_declared", return_value=False),
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

    def test_lengthless_graphql_is_buffered_bounded_and_authorized_semantically(self):
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.cluster = "default"
        addon.opa_url = "http://opa.invalid/decision"
        addon.opa_timeout = 2
        addon.graphql_config = {}
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        prepared = mock.Mock(secret_headers={"authorization"}, broker_provider=None, deferred=False)
        addon.credential_adapter = mock.Mock()
        addon.credential_adapter.prepare.return_value = prepared
        body = b'{"query":"query TwgCLI_WhoAmIRich { me { user { name } } }"}'
        request = http.Request.make(
            "POST",
            "https://api.atlassian.com/graphql",
            content=body,
            headers={"content-type": "application/json", "authorization": "Bearer canary"},
        )
        del request.headers["content-length"]
        flow = tflow.tflow(req=request)
        principal = {
            "project_id": PROJECT,
            "context_id": CONTEXT,
            "context": "default",
            "project_root": "/workspace/project",
        }
        policy_inputs = []

        def allow(_url, document, _timeout):
            policy_inputs.append(document)
            return gateway.Decision(
                allow=True,
                reason="allowed by Context policy",
                status_code=200,
                learnable=False,
            )

        with (
            mock.patch.object(
                gateway,
                "normalize_ingress_authority",
                return_value=("https", "api.atlassian.com", 443),
            ),
            mock.patch.object(gateway, "resolve_project_principal", return_value=principal),
            mock.patch.object(gateway, "graphql_endpoint_declared", return_value=True),
            mock.patch.object(gateway, "query_opa", side_effect=allow),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
            mock.patch.object(gateway, "_audit") as audit,
        ):
            addon.requestheaders(flow)
            self.assertIsNone(flow.response)
            self.assertFalse(flow.request.stream)
            self.assertIn("tobari_graphql_pending", flow.metadata)
            addon.request(flow)

        self.assertEqual(len(policy_inputs), 1)
        self.assertEqual(
            policy_inputs[0]["request"]["graphql"],
            {"operation_type": "query", "root_fields": ["me"]},
        )
        self.assertNotIn("TwgCLI_WhoAmIRich", json.dumps(policy_inputs[0]))
        self.assertNotIn("canary", json.dumps(policy_inputs[0]))
        prepared.apply.assert_called_once_with(flow.request)
        commit.assert_called_once_with(flow)
        audit.assert_not_called()

    def test_declared_graphql_get_redacts_source_and_authorizes_query_roots(self):
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.cluster = "default"
        addon.opa_url = "http://opa.invalid/decision"
        addon.opa_timeout = 2
        addon.graphql_config = {}
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        prepared = mock.Mock(secret_headers=set(), broker_provider=None, deferred=False)
        addon.credential_adapter = mock.Mock()
        addon.credential_adapter.prepare.return_value = prepared
        request = http.Request.make(
            "GET",
            "https://api.example.com/graphql?query=query%20Read%20%7B%20viewer%20%7B%20secretCanary%20%7D%20%7D&operationName=Read",
        )
        flow = tflow.tflow(req=request)
        principal = {
            "project_id": PROJECT, "context_id": CONTEXT,
            "context": "default", "project_root": "/workspace/project",
        }
        policy_inputs = []

        def allow(_url, document, _timeout):
            policy_inputs.append(document)
            return gateway.Decision(allow=True, reason="allowed", status_code=403, learnable=False)

        with (
            mock.patch.object(gateway, "normalize_ingress_authority", return_value=("https", "api.example.com", 443)),
            mock.patch.object(gateway, "resolve_project_principal", return_value=principal),
            mock.patch.object(gateway, "graphql_endpoint_declared", return_value=True),
            mock.patch.object(gateway, "query_opa", side_effect=allow),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
        ):
            addon.requestheaders(flow)
            self.assertIn("tobari_graphql_pending", flow.metadata)
            addon.request(flow)

        self.assertEqual(policy_inputs[0]["request"]["graphql"], {"operation_type": "query", "root_fields": ["viewer"]})
        self.assertEqual(policy_inputs[0]["request"]["query"], {})
        self.assertNotIn("secretCanary", json.dumps(policy_inputs[0]))
        prepared.apply.assert_called_once_with(flow.request)
        commit.assert_called_once_with(flow)

    def test_declared_kubernetes_authority_derives_watch_without_schema(self):
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.cluster = "default"
        addon.opa_url = "http://opa.invalid/decision"
        addon.opa_timeout = 2
        addon.graphql_config = {}
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        prepared = mock.Mock(secret_headers={"authorization"}, broker_provider=None, deferred=False)
        addon.credential_adapter = mock.Mock()
        addon.credential_adapter.prepare.return_value = prepared
        request = http.Request.make(
            "GET",
            "https://cluster.us-east-1.eks.amazonaws.com/apis/acme.example/v1/namespaces/team/widgets?watch=true",
            headers={"authorization": "Bearer secret-canary"},
        )
        flow = tflow.tflow(req=request)
        principal = {
            "project_id": PROJECT, "context_id": CONTEXT,
            "context": "default", "project_root": "/workspace/project",
        }
        policy_inputs = []

        def deny(_url, document, _timeout):
            policy_inputs.append(document)
            return gateway.Decision(False, "review required", 403, True)

        with (
            mock.patch.object(gateway, "normalize_ingress_authority", return_value=("https", "cluster.us-east-1.eks.amazonaws.com", 443)),
            mock.patch.object(gateway, "resolve_project_principal", return_value=principal),
            mock.patch.object(gateway, "graphql_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "mcp_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "kubernetes_endpoint_declared", return_value=True),
            mock.patch.object(gateway, "query_opa", side_effect=deny),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
        ):
            addon.requestheaders(flow)

        self.assertEqual(flow.response.status_code, 403)
        commit.assert_not_called()
        self.assertEqual(policy_inputs[0]["request"]["kubernetes"], {
            "verb": "watch",
            "resource": "acme.example/v1/namespaces/team/widgets",
            "dry_run": "none",
        })
        denial = json.loads(flow.response.content)
        self.assertEqual(denial["tobari"]["request"]["protocol"], "kubernetes")
        self.assertEqual(denial["tobari"]["request"]["kubernetes_verb"], "watch")
        self.assertEqual(denial["tobari"]["request"]["kubernetes_resource"], "acme.example/v1/namespaces/team/widgets")
        self.assertNotIn("secret-canary", json.dumps(policy_inputs[0]))
        self.assertNotIn("secret-canary", json.dumps(denial))

    def test_git_smart_http_receive_pack_is_exact_and_body_opaque(self):
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.cluster = "default"
        addon.opa_url = "http://opa.invalid/decision"
        addon.opa_timeout = 2
        addon.graphql_config = {}
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        prepared = mock.Mock(secret_headers={"authorization"}, broker_provider=None, deferred=False)
        addon.credential_adapter = mock.Mock()
        addon.credential_adapter.prepare.return_value = prepared
        request = http.Request.make(
            "POST", "https://git.example.com/team/repo.git/git-receive-pack",
            content=b"secret-pack-payload",
            headers={
                "content-type": "application/x-git-receive-pack-request",
                "authorization": "Bearer secret-canary",
            },
        )
        flow = tflow.tflow(req=request)
        principal = {
            "project_id": PROJECT, "context_id": CONTEXT,
            "context": "default", "project_root": "/workspace/project",
        }
        policy_inputs = []

        def deny(_url, document, _timeout):
            policy_inputs.append(document)
            return gateway.Decision(False, "review required", 403, True)

        with (
            mock.patch.object(gateway, "normalize_ingress_authority", return_value=("https", "git.example.com", 443)),
            mock.patch.object(gateway, "resolve_project_principal", return_value=principal),
            mock.patch.object(gateway, "graphql_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "mcp_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "kubernetes_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "query_opa", side_effect=deny),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
        ):
            addon.requestheaders(flow)

        self.assertEqual(flow.response.status_code, 403)
        commit.assert_not_called()
        self.assertEqual(policy_inputs[0]["request"]["git"], {
            "service": "receive-pack", "repository": "/team/repo.git",
        })
        serialized = json.dumps(policy_inputs[0])
        self.assertNotIn("secret-pack-payload", serialized)
        self.assertNotIn("secret-canary", serialized)

    def test_oci_manifest_push_is_exact_and_body_opaque(self):
        addon = gateway.TobariGateway.__new__(gateway.TobariGateway)
        addon.cluster = "default"
        addon.opa_url = "http://opa.invalid/decision"
        addon.opa_timeout = 2
        addon.graphql_config = {}
        addon.principal_source = mock.Mock()
        addon.principal_source.load.return_value = {}
        prepared = mock.Mock(secret_headers={"authorization"}, broker_provider=None, deferred=False)
        addon.credential_adapter = mock.Mock()
        addon.credential_adapter.prepare.return_value = prepared
        request = http.Request.make(
            "PUT", "https://registry.example.com/v2/team/app/manifests/latest",
            content=b'{"secret":"manifest-payload"}',
            headers={"content-type": "application/vnd.oci.image.manifest.v1+json", "authorization": "Bearer secret-canary"},
        )
        flow = tflow.tflow(req=request)
        principal = {
            "project_id": PROJECT, "context_id": CONTEXT,
            "context": "default", "project_root": "/workspace/project",
        }
        policy_inputs = []

        def deny(_url, document, _timeout):
            policy_inputs.append(document)
            return gateway.Decision(False, "review required", 403, True)

        with (
            mock.patch.object(gateway, "normalize_ingress_authority", return_value=("https", "registry.example.com", 443)),
            mock.patch.object(gateway, "resolve_project_principal", return_value=principal),
            mock.patch.object(gateway, "graphql_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "mcp_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "kubernetes_endpoint_declared", return_value=False),
            mock.patch.object(gateway, "query_opa", side_effect=deny),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
        ):
            addon.requestheaders(flow)

        self.assertEqual(flow.response.status_code, 403)
        commit.assert_not_called()
        self.assertEqual(policy_inputs[0]["request"]["oci"], {
            "action": "push", "repository": "team/app", "object": "manifest:latest",
        })
        serialized = json.dumps(policy_inputs[0])
        self.assertNotIn("manifest-payload", serialized)
        self.assertNotIn("secret-canary", serialized)

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

    def test_declared_raw_binding_rejects_workspace_owned_secret(self):
        provider_projection = self.static_tool_provider_projection()
        fallback = mock.Mock()
        caller = mock.Mock()
        adapter = broker.BrokeredCredentialAdapter(
            fallback=fallback, projection_path="/projection.json",
            socket_path="/broker.sock", timeout=2,
            projection_loader=lambda _: provider_projection, caller=caller,
        )
        request = http.Request.make(
            "GET",
            "https://api.chatwork.com/v2/me",
            headers={"x-chatworktoken": "real-workspace-token"},
        )
        with self.assertRaises(BrokerAuthenticationRequired):
            adapter.prepare(
                request,
                "https",
                "api.chatwork.com",
                443,
                self.context_a,
                self.project_a,
            )
        self.assertNotIn("x-chatworktoken", request.headers)
        caller.assert_not_called()
        fallback.prepare.assert_not_called()

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
            value["complete_files"][0]["provider_id"] = "anthropic-alt"
            value["json_merges"][0]["provider_id"] = "anthropic-alt"
            value["header_bindings"][0]["provider_id"] = "anthropic-alt"

        def alternate_display_name(value):
            value["providers"][0]["display_name"] = "Anthropic account"

        def alternate_complete_file(value):
            provider = value["providers"][0]
            provider["workspace_projections"][0]["path"] = ".claude/other.json"
            value["complete_files"][0]["path"] = ".claude/other.json"

        def alternate_onboarding_state(value):
            provider = value["providers"][0]
            provider["workspace_projections"][1]["template"] = (
                '{"hasCompletedOnboarding":false}'
            )
            value["json_merges"][0]["template"] = (
                '{"hasCompletedOnboarding":false}'
            )

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
            ("complete file", alternate_complete_file),
            ("onboarding state", alternate_onboarding_state),
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
        invalid = json.loads(json.dumps(projection))
        invalid["providers"][0]["workspace_projections"].append(
            {
                "kind": "merge_json",
                "path": ".tool.json",
                "template": '{"configured":true}',
            }
        )
        invalid["json_merges"].append(
            {
                "provider_id": "chatwork",
                "path": ".tool.json",
                "template": '{"configured":true}',
            }
        )
        with self.assertRaises(BrokerCredentialUnavailable):
            validate_provider_projection(invalid)

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
