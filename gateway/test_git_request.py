import unittest

from addon.git_request import GitRequestError, classify_git_request


class GitRequestTests(unittest.TestCase):
    def test_discovery_identifies_upload_and_receive_without_payload(self) -> None:
        fetch = classify_git_request("GET", "/team/repo.git/info/refs", "service=git-upload-pack", [])
        push = classify_git_request("GET", "/team/repo.git/info/refs", "service=git-receive-pack", [("Content-Length", "0")])
        self.assertEqual((fetch.service, fetch.repository), ("upload-pack", "/team/repo.git"))
        self.assertEqual((push.service, push.repository), ("receive-pack", "/team/repo.git"))

    def test_rpc_media_type_identifies_service_and_body_stays_opaque(self) -> None:
        parsed = classify_git_request(
            "POST", "/team/repo.git/git-receive-pack", "",
            [("Content-Type", "application/x-git-receive-pack-request"), ("Authorization", "secret")],
        )
        self.assertEqual((parsed.service, parsed.repository), ("receive-pack", "/team/repo.git"))

    def test_unrelated_http_is_not_claimed(self) -> None:
        self.assertIsNone(classify_git_request("GET", "/repos/team/repo", "", []))

    def test_malformed_claims_fail_closed(self) -> None:
        cases = [
            ("POST", "/team/repo.git/info/refs", "service=git-upload-pack", []),
            ("GET", "/team/repo.git/info/refs", "service=git-upload-pack&x=1", []),
            ("POST", "/team/repo.git/git-upload-pack", "", [("Content-Type", "application/json")]),
            ("POST", "/team/../repo.git/git-receive-pack", "", [("Content-Type", "application/x-git-receive-pack-request")]),
        ]
        for case in cases:
            with self.subTest(case=case), self.assertRaises(GitRequestError):
                classify_git_request(*case)

    def test_projection_rejects_invalid_direct_construction(self) -> None:
        valid_type = type(classify_git_request("GET", "/team/repo.git/info/refs", "service=git-upload-pack", []))
        for fields in (
            {"service": "future", "repository": "/team/repo.git"},
            {"service": "upload-pack", "repository": "/team/../repo.git"},
            {"service": "upload-pack", "repository": "/team/\ud800.git"},
        ):
            with self.subTest(fields=fields), self.assertRaises(GitRequestError):
                valid_type(**fields)


if __name__ == "__main__":
    unittest.main()
