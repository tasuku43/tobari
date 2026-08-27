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
        monolithic = parse_oci_request("POST", "/v2/team/app/blobs/uploads/", "digest=sha256%3Aabc")
        mount = parse_oci_request(
            "POST", "/v2/team/app/blobs/uploads/", "mount=sha256%3Aabc&from=shared%2Fbase",
        )
        self.assertEqual(push.action, "push")
        self.assertEqual(start.action, "start_upload")
        self.assertEqual(chunk.action, "upload_chunk")
        self.assertEqual((complete.action, complete.object), ("complete_upload", "upload:session-1:blob:sha256%3Aabc"))
        self.assertEqual((monolithic.action, monolithic.object), ("complete_upload", "blob:sha256%3Aabc"))
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
            ("POST", "/v2/team/app/blobs/uploads/", "mount=sha256%3Aabc&from=..%2Fbad"),
            ("GET", "/v2/team/app/manifests/latest/extra", ""),
            ("GET", "/v2/team/app/blobs/sha256:abc/extra", ""),
            ("GET", "/v2/_catalog/extra", ""),
            ("GET", "/v2/manifests/latest", ""),
            ("GET", "/v2/blobs/sha256:abc", ""),
            ("GET", "/v2/referrers/sha256:abc", ""),
            ("GET", "/v2/tags/list", ""),
        ]
        for case in cases:
            with self.subTest(case=case), self.assertRaises(OCIRequestError):
                parse_oci_request(*case)

    def test_projection_rejects_invalid_direct_construction(self) -> None:
        projection_type = type(parse_oci_request("GET", "/v2/team/app/manifests/latest", ""))
        for fields in (
            {"action": [], "repository": "team/app", "object": "manifest:latest"},
            {"action": "future", "repository": "team/app", "object": "manifest:latest"},
            {"action": "push", "repository": "team/../app", "object": "manifest:latest"},
            {"action": "mount", "repository": "team/app", "object": "mount:sha256%3Aabc"},
            {"action": "pull", "repository": "team/app", "object": "manifest:\ud800"},
            {"action": "complete_upload", "repository": "team/app", "object": "upload:session%2f1:blob:sha256%3Aabc"},
            {"action": "complete_upload", "repository": "team/app", "object": "upload:%41:blob:sha256%3Aabc"},
            {"action": "complete_upload", "repository": "team/app", "object": "upload:%FF:blob:sha256%3Aabc"},
            {"action": "complete_upload", "repository": "team/app", "object": "blob:a/b"},
            {"action": "mount", "repository": "team/app", "object": "mount:%41:from:..%2Fbad"},
            {"action": "pull", "repository": "team/app", "object": "manifest:a/b"},
            {"action": "complete_upload", "repository": "team/app", "object": "blob:" + "a" * 513},
        ):
            with self.subTest(fields=fields), self.assertRaises(OCIRequestError):
                projection_type(**fields)


if __name__ == "__main__":
    unittest.main()
