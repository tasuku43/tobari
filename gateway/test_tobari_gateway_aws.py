import io
import json
from contextlib import ExitStack, redirect_stdout
from unittest import mock

import broker_credentials as broker_module
import tobari_gateway as gateway
from broker_credentials import (
    BrokerCredentialBindingError,
    BrokerCredentialOutcomeUnknown,
    BrokerCredentialUnavailable,
    _broker_response,
    call_broker,
    validate_provider_projection,
)
from tobari_gateway_test_support import ReviewedDynamicCredentialGatewayTestCase


class ReviewedAWSSigV4GatewayTests(ReviewedDynamicCredentialGatewayTestCase):
    def test_brokered_aws_handle_wins_over_every_colliding_protocol_classifier(self):
        self.provider_projection = self.aws_provider_projection()
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

        collisions = {
            "graphql": "graphql_endpoint_declared",
            "mcp": "mcp_endpoint_declared",
            "kubernetes": "kubernetes_endpoint_declared",
            "oci": "parse_oci_request",
            "git": "classify_git_request",
        }
        for name, classifier in collisions.items():
            with self.subTest(classifier=name):
                flow = self.aws_signed_flow()
                captured = {}

                def allow(_url, policy_input, _timeout):
                    captured.update(policy_input)
                    return gateway.Decision(True, "allowed", 403, False)

                addon = self.broker_gateway(broker_call)
                with ExitStack() as stack:
                    stack.enter_context(
                        mock.patch.object(
                            gateway,
                            classifier,
                            side_effect=AssertionError(
                                f"{name} classifier ran before brokered AWS"
                            ),
                        )
                    )
                    stack.enter_context(
                        mock.patch.object(gateway, "query_opa", side_effect=allow)
                    )
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                    self.assertIn("tobari_aws_query_pending", flow.metadata)
                    self.assertNotIn("tobari_graphql_pending", flow.metadata)
                    self.assertNotIn("tobari_mcp_pending", flow.metadata)
                    addon.request(flow)

                self.assertEqual(
                    captured["request"]["aws"],
                    {
                        "wire_protocol": "query",
                        "service": "sts",
                        "operation": "GetCallerIdentity",
                    },
                )
                self.assertNotIn("graphql", captured["request"])
                self.assertNotIn("mcp", captured["request"])
                self.assertNotIn("kubernetes", captured["request"])
                self.assertNotIn("oci", captured["request"])
                self.assertNotIn("git", captured["request"])

        self.assertEqual(
            [request["op"] for request in calls],
            ["introspect_signing", "sign_sigv4"] * len(collisions),
        )

    def test_malformed_brokered_aws_handle_is_terminal_before_colliding_classifiers(self):
        self.provider_projection = self.aws_provider_projection()
        flow = self.aws_signed_flow()
        flow.request.headers["x-amz-content-sha256"] = "UNSIGNED-PAYLOAD"
        addon = self.broker_gateway(
            lambda _request: (_ for _ in ()).throw(
                AssertionError("malformed AWS handle reached broker")
            )
        )
        with ExitStack() as stack:
            for classifier in (
                "graphql_endpoint_declared",
                "mcp_endpoint_declared",
                "kubernetes_endpoint_declared",
                "parse_oci_request",
                "classify_git_request",
            ):
                stack.enter_context(
                    mock.patch.object(
                        gateway,
                        classifier,
                        side_effect=AssertionError(
                            f"{classifier} ran after malformed AWS handle"
                        ),
                    )
                )
            with mock.patch.object(gateway, "query_opa") as query:
                with redirect_stdout(io.StringIO()):
                    addon.requestheaders(flow)
        query.assert_not_called()
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(
            json.loads(flow.response.content), {"error": "credential_handle_invalid"}
        )

    def test_direct_aws_signature_at_declared_target_requires_broker(self):
        self.provider_projection = self.aws_provider_projection()
        flow = self.flow("https://sts.us-east-1.amazonaws.com/", "GET")
        flow.request.headers["x-amz-date"] = "20260809T120000Z"
        flow.request.headers["x-amz-security-token"] = "real-session-token-canary"
        flow.request.headers["authorization"] = (
            "AWS4-HMAC-SHA256 Credential=ASIAEXAMPLEKEY1234/"
            "20260809/us-east-1/sts/aws4_request, "
            "SignedHeaders=host;x-amz-date;x-amz-security-token, "
            "Signature=" + "a" * 64
        )
        broker_call = mock.Mock()
        addon = self.broker_gateway(broker_call)
        with mock.patch.object(gateway, "query_opa") as query:
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        query.assert_not_called()
        broker_call.assert_not_called()
        self.assertEqual(flow.response.status_code, 403)
        self.assertEqual(
            json.loads(flow.response.content), {"error": "broker_auth_required"}
        )
        self.assertNotIn("authorization", flow.request.headers)
        self.assertNotIn("x-amz-security-token", flow.request.headers)

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
            return gateway.Decision(True, "allowed", 403, False)

        addon = self.broker_gateway(broker_call)
        with mock.patch.object(gateway, "query_opa", side_effect=allow):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertEqual([item["op"] for item in calls], ["introspect_signing"])
        self.assertFalse(flow.request.stream)
        self.assertNotIn("authorization", flow.request.headers)
        self.assertNotIn("x-amz-security-token", flow.request.headers)
        self.assertEqual(captured, {})

        with mock.patch.object(gateway, "query_opa", side_effect=allow):
            addon.request(flow)
        self.assertNotIn("headers", captured["request"])
        self.assertEqual(captured["request"]["aws"], {
            "wire_protocol": "query",
            "service": "sts",
            "operation": "GetCallerIdentity",
        })
        self.assertEqual(captured["authorization"], {"broker_provider": "aws"})
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
            return_value=gateway.Decision(True, "allowed", 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        self.assertIsNone(flow.response)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", 403, False),
        ):
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
            return_value=gateway.Decision(True, "allowed", 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
        with mock.patch.object(
            gateway,
            "query_opa",
            return_value=gateway.Decision(True, "allowed", 403, False),
        ):
            with redirect_stdout(io.StringIO()):
                addon.request(flow)
        self.assertEqual(flow.response.status_code, 503)
        self.assertEqual(
            json.loads(flow.response.content),
            {"error": "credential_broker_unavailable"},
        )

    def test_aws_sigv4_denial_never_signs_or_retains_handle(self):
        self.provider_projection = self.aws_provider_projection()
        flow = self.aws_signed_flow()
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
        with (
            mock.patch.object(
                gateway,
                "query_opa",
                return_value=gateway.Decision(False, "denied", 403, False),
            ),
            mock.patch.object(gateway, "commit_upstream_authority") as commit,
        ):
            with redirect_stdout(io.StringIO()):
                addon.requestheaders(flow)
            with redirect_stdout(io.StringIO()):
                addon.request(flow)
        self.assertEqual([item["op"] for item in calls], ["introspect_signing"])
        commit.assert_not_called()
        self.assertNotIn("tobari_deferred_credential", flow.metadata)
        self.assertNotIn("authorization", flow.request.headers)
        self.assertNotIn("x-amz-security-token", flow.request.headers)
        self.assertEqual(flow.response.status_code, 403)

    def test_aws_query_rejects_structural_changes_before_delayed_policy(self):
        self.provider_projection = self.aws_provider_projection()

        def configure_flow():
            body = b"Action=GetCallerIdentity&Version=2011-06-15"
            flow = self.flow(
                "https://sts.us-east-1.amazonaws.com/",
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
            flow.request.path = "/changed"

        def mutate_query(flow):
            flow.request.path = "/?Action=CreateRole"

        def mutate_authority(flow):
            flow.request.host = "sts.us-west-2.amazonaws.com"

        def mutate_signed_header(flow):
            flow.request.headers["content-type"] = "application/json"

        for name, mutation, expected_status, expected_error in (
            ("method", mutate_method, 400, "aws_request_changed"),
            ("path", mutate_path, 400, "aws_request_changed"),
            ("query", mutate_query, 400, "aws_request_changed"),
            ("authority", mutate_authority, 400, "request_authority_invalid"),
            ("signed header", mutate_signed_header, 400, "aws_request_changed"),
        ):
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
                    return_value=gateway.Decision(True, "allowed", 403, False),
                ):
                    with redirect_stdout(io.StringIO()):
                        addon.requestheaders(flow)
                mutation(flow)
                with mock.patch.object(
                    gateway,
                    "query_opa",
                    return_value=gateway.Decision(True, "allowed", 403, False),
                ):
                    with redirect_stdout(io.StringIO()):
                        addon.request(flow)
                self.assertEqual([item["op"] for item in calls], ["introspect_signing"])
                self.assertEqual(flow.response.status_code, expected_status)
                self.assertEqual(
                    json.loads(flow.response.content),
                    {"error": expected_error},
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
