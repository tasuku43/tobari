import unittest

from aws_request import (
    AWSRequestError,
    ParsedAWSRequest,
    PendingAWSQueryRequest,
    classify_aws_request_headers,
    parse_aws_query_request,
)


class AWSRequestIdentityTests(unittest.TestCase):
    def headers(self, content_type, length, signed="content-type;host;x-amz-date"):
        return [
            (
                "authorization",
                "AWS4-HMAC-SHA256 Credential=ASIAEXAMPLE/20260821/us-east-1/sts/aws4_request, "
                f"SignedHeaders={signed}, Signature={'a' * 64}",
            ),
            ("content-type", content_type),
            ("content-length", str(length)),
            ("host", "sts.us-east-1.amazonaws.com"),
            ("x-amz-date", "20260821T010203Z"),
        ]

    def classify(self, headers):
        return classify_aws_request_headers(
            "POST", "https", "sts.us-east-1.amazonaws.com", 443, "/", "", headers
        )

    def test_query_extracts_only_action_after_complete_body(self):
        body = b"Action=GetCallerIdentity&Version=2011-06-15&Resource=secret"
        pending = self.classify(self.headers("application/x-www-form-urlencoded", len(body)))
        self.assertEqual(
            pending,
            PendingAWSQueryRequest(
                "sts", len(body), "sts.us-east-1.amazonaws.com", "application/x-www-form-urlencoded"
            ),
        )
        parsed = parse_aws_query_request(
            pending,
            "POST", "https", "sts.us-east-1.amazonaws.com", 443, "/", "",
            self.headers("application/x-www-form-urlencoded", len(body)), body,
        )
        self.assertEqual(parsed, ParsedAWSRequest("query", "sts", "GetCallerIdentity"))
        self.assertNotIn("secret", repr(parsed))

    def test_json_requires_one_signed_exact_target(self):
        headers = self.headers(
            "application/x-amz-json-1.0", 2,
            "content-type;host;x-amz-date;x-amz-target",
        ) + [("x-amz-target", "DynamoDB_20120810.ListTables")]
        parsed = self.classify(headers)
        self.assertEqual(parsed, ParsedAWSRequest("json", "sts", "DynamoDB_20120810.ListTables"))
        for changed in (
            [item for item in headers if item[0] != "x-amz-target"],
            headers + [("x-amz-target", "DynamoDB_20120810.PutItem")],
            self.headers("application/x-amz-json-1.0", 2) + [("x-amz-target", "DynamoDB_20120810.ListTables")],
        ):
            with self.assertRaises(AWSRequestError):
                self.classify(changed)

    def test_non_aws_and_non_rpc_requests_remain_unclassified(self):
        headers = self.headers("application/octet-stream", 2)
        self.assertIsNone(self.classify(headers))
        self.assertIsNone(classify_aws_request_headers("POST", "https", "example.com", 443, "/", "", headers))

    def test_ambiguous_or_changed_query_fails_closed(self):
        cases = [
            b"Action=ListRoles&Action=CreateRole&Version=2010-05-08",
            b"Action=ListRoles&Version=2010-05-08&Version=2010-05-08",
            b"Action=List-Roles&Version=2010-05-08",
            b"Action=ListRoles&Version=latest",
        ]
        for body in cases:
            pending = self.classify(self.headers("application/x-www-form-urlencoded", len(body)))
            with self.assertRaises(AWSRequestError):
                parse_aws_query_request(
                    pending, "POST", "https", "sts.us-east-1.amazonaws.com", 443, "/",
                    "",
                    self.headers("application/x-www-form-urlencoded", len(body)), body,
                )

    def test_rpc_without_signature_or_with_query_coordinates_fails_closed(self):
        body = b"Action=ListRoles&Version=2010-05-08"
        headers = self.headers("application/x-www-form-urlencoded", len(body))
        without_authorization = [item for item in headers if item[0] != "authorization"]
        with self.assertRaises(AWSRequestError):
            self.classify(without_authorization)
        with self.assertRaises(AWSRequestError):
            classify_aws_request_headers(
                "POST", "https", "sts.us-east-1.amazonaws.com", 443, "/", "Action=CreateRole", headers
            )

    def test_unsigned_streaming_and_malformed_sigv4_are_rejected(self):
        body = b"Action=ListRoles&Version=2010-05-08"
        base = self.headers("application/x-www-form-urlencoded", len(body))
        cases = [
            base + [("transfer-encoding", "chunked")],
            base + [("x-amz-content-sha256", "UNSIGNED-PAYLOAD")],
            [(name, value.replace("Signature=" + "a" * 64, "Signature=bad")) if name == "authorization" else (name, value) for name, value in base],
        ]
        for headers in cases:
            with self.assertRaises(AWSRequestError):
                self.classify(headers)


if __name__ == "__main__":
    unittest.main()
