from __future__ import annotations

import hashlib
import unittest
from datetime import datetime, timezone

from authbroker.aws_sigv4 import (
    SigV4Error,
    parse_credentials,
    parse_request,
    sign,
)


class AWSSigV4Tests(unittest.TestCase):
    def request(self, **updates):
        document = {
            "host": "iam.amazonaws.com",
            "method": "GET",
            "path": "/",
            "query": "Action=ListUsers&Version=2010-05-08",
            "region": "us-east-1",
            "service": "iam",
            "headers": [("content-type", "application/x-www-form-urlencoded; charset=utf-8")],
            "payload_hash": hashlib.sha256(b"").hexdigest(),
        }
        document.update(updates)
        return parse_request(document)

    def credentials(self):
        return parse_credentials(
            {
                "access_key_id": "AKIDEXAMPLE000000",
                "secret_access_key": "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
                "session_token": "synthetic-session-token-value",
            }
        )

    def test_sign_is_deterministic_and_scoped(self):
        result = sign(
            self.request(),
            self.credentials(),
            clock=lambda: datetime(2015, 8, 30, 12, 36, 0, tzinfo=timezone.utc),
        )
        self.assertEqual(result.amz_date, "20150830T123600Z")
        self.assertEqual(result.security_token, "synthetic-session-token-value")
        self.assertIsNone(result.content_sha256)
        self.assertIn(
            "Credential=AKIDEXAMPLE000000/20150830/us-east-1/iam/aws4_request",
            result.authorization,
        )
        self.assertIn(
            "SignedHeaders=content-type;host;x-amz-date;x-amz-security-token",
            result.authorization,
        )
        self.assertRegex(result.authorization, r"Signature=[0-9a-f]{64}$")
        self.assertEqual(
            result.authorization.rsplit("Signature=", 1)[1],
            "7067d23bf34a8bb68cc1b3f71988a2f13e6f207657eda319690206f0d62d1561",
        )

    def test_s3_adds_payload_hash_header(self):
        result = sign(
            self.request(
                host="s3.us-west-2.amazonaws.com",
                service="s3",
                region="us-west-2",
                query="",
            ),
            self.credentials(),
            clock=lambda: datetime(2026, 8, 9, tzinfo=timezone.utc),
        )
        self.assertEqual(result.content_sha256, hashlib.sha256(b"").hexdigest())
        self.assertIn("x-amz-content-sha256", result.authorization)

    def test_rejects_non_aws_custom_and_presigned_targets(self):
        for updates in (
            {"host": "aws.example.com"},
            {"host": "amazonaws.com"},
            {"query": "X-Amz-Credential=secret"},
            {"path": "/a//b"},
            {"path": "/a/../b"},
            {"payload_hash": "UNSIGNED-PAYLOAD"},
            {"query": "Action=ListUsers&broken=%GG"},
            {"host": "sts.us-west-2.amazonaws.com", "region": "us-east-1", "service": "sts"},
            {"host": "sts.us-east-1.amazonaws.com", "region": "us-east-1", "service": "iam"},
            {"host": "iam.amazonaws.com", "region": "us-west-2", "service": "iam"},
            {
                "host": "sts.us-gov-west-1.amazonaws.com",
                "region": "us-gov-west-1",
                "service": "sts",
            },
            {
                "host": "sts.cn-north-1.amazonaws.com.cn",
                "region": "cn-north-1",
                "service": "sts",
            },
        ):
            with self.subTest(updates=updates), self.assertRaises(SigV4Error):
                self.request(**updates)

    def test_rejects_secret_or_transport_headers(self):
        for name, value in (
            ("authorization", "value"),
            ("proxy-authorization", "value"),
            ("transfer-encoding", "value"),
            ("x-tobari-session", "value"),
            ("x-example", "caf\N{LATIN SMALL LETTER E WITH ACUTE}"),
            ("x-example", "value\x7f"),
        ):
            with self.subTest(name=name, value=value), self.assertRaises(SigV4Error):
                self.request(headers=[(name, value)])

    def test_secret_bearing_values_are_redacted_from_repr(self):
        credentials = self.credentials()
        result = sign(
            self.request(),
            credentials,
            clock=lambda: datetime(2015, 8, 30, 12, 36, 0, tzinfo=timezone.utc),
        )
        for rendered in (repr(credentials), repr(self.request()), repr(result)):
            self.assertNotIn("synthetic-session-token-value", rendered)
            self.assertNotIn("EXAMPLEKEY", rendered)
            self.assertNotIn("Signature=", rendered)

    def test_rejects_malformed_credentials_without_echo(self):
        candidate = {
            "access_key_id": "short",
            "secret_access_key": "secret-canary",
            "session_token": "token-canary",
        }
        with self.assertRaises(SigV4Error) as raised:
            parse_credentials(candidate)
        self.assertEqual(raised.exception.code, "aws_credentials_invalid")
        self.assertNotIn("canary", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
