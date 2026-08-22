import unittest

from addon.oci_request import OCIRequestError, parse_oci_request


class OCIRequestTests(unittest.TestCase):
    def test_read_routes_keep_repository_and_object_exact(self) -> None:
        manifest = parse_oci_request("GET", "/v2/team/app/manifests/latest", "")
        blob = parse_oci_request("HEAD", "/v2/team/app/blobs/sha256:abc", "")
        tags = parse_oci_request("GET", "/v2/team/app/tags/list", "n=20")
        self.assertEqual((manifest.action, manifest.repository, manifest.object), ("pull", "team/app", "manifest:latest"))
        self.assertEqual((blob.action, blob.repository, blob.object), ("pull", "team/app", "blob:sha256:abc"))
        self.assertEqual((tags.action, tags.repository, tags.object), ("list", "team/app", "tags"))

    def test_write_and_upload_routes_are_distinct_without_body_parsing(self) -> None:
        push = parse_oci_request("PUT", "/v2/team/app/manifests/latest", "")
        start = parse_oci_request("POST", "/v2/team/app/blobs/uploads/", "")
        chunk = parse_oci_request("PATCH", "/v2/team/app/blobs/uploads/session-1", "")
        complete = parse_oci_request("PUT", "/v2/team/app/blobs/uploads/session-1", "digest=sha256%3Aabc")
        mount = parse_oci_request(
            "POST", "/v2/team/app/blobs/uploads/", "mount=sha256%3Aabc&from=shared%2Fbase",
        )
        self.assertEqual(push.action, "push")
        self.assertEqual(start.action, "start_upload")
        self.assertEqual(chunk.action, "upload_chunk")
        self.assertEqual((complete.action, complete.object), ("complete_upload", "blob:sha256:abc"))
        self.assertEqual((mount.action, mount.object), ("mount", "mount:sha256%3Aabc:from:shared%2Fbase"))

    def test_unrelated_http_is_not_claimed(self) -> None:
        self.assertIsNone(parse_oci_request("GET", "/api/v2/items", ""))
        self.assertIsNone(parse_oci_request("GET", "/v2/me", ""))
        self.assertIsNone(parse_oci_request("GET", "/v2/", ""))

    def test_malformed_or_unsupported_distribution_routes_fail_closed(self) -> None:
        cases = [
            ("POST", "/v2/team/app/manifests/latest", ""),
            ("PUT", "/v2/team/app/blobs/uploads/session-1", ""),
            ("POST", "/v2/team/app/blobs/uploads/", "mount=&from=other"),
            ("POST", "/v2/team/app/blobs/uploads/", "mount=sha256%3Aabc"),
            ("GET", "/v2/team/%2Fapp/tags/list", ""),
        ]
        for case in cases:
            with self.subTest(case=case), self.assertRaises(OCIRequestError):
                parse_oci_request(*case)


if __name__ == "__main__":
    unittest.main()
