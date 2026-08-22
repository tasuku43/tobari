import unittest

from addon.kubernetes_request import KubernetesRequestError, parse_kubernetes_request


class KubernetesRequestTests(unittest.TestCase):
    def test_core_and_crd_resources_need_no_schema(self) -> None:
        core = parse_kubernetes_request("GET", "/api/v1/namespaces/team/pods/demo/log", "", [])
        crd = parse_kubernetes_request("PATCH", "/apis/acme.example/v1/widgets/blue", "dryRun=All", [])
        self.assertEqual((core.verb, core.resource, core.dry_run), ("get", "core/v1/namespaces/team/pods/demo/log", "none"))
        self.assertEqual((crd.verb, crd.resource, crd.dry_run), ("patch", "acme.example/v1/widgets/blue", "all"))

    def test_list_watch_mutation_and_interactive_are_distinct(self) -> None:
        self.assertEqual(parse_kubernetes_request("GET", "/api/v1/pods", "", []).verb, "list")
        self.assertEqual(parse_kubernetes_request("GET", "/api/v1/pods", "watch=true", []).verb, "watch")
        self.assertEqual(parse_kubernetes_request("POST", "/api/v1/namespaces/team/pods", "", []).verb, "create")
        self.assertEqual(parse_kubernetes_request("POST", "/api/v1/namespaces/team/pods/demo/exec", "", []).verb, "connect")

    def test_discovery_is_exact_non_resource_read(self) -> None:
        for path in ("/apis", "/api/v1", "/apis/apps/v1", "/openapi/v3"):
            parsed = parse_kubernetes_request("GET", path, "", [])
            self.assertEqual((parsed.verb, parsed.resource), ("get", "non-resource:" + path))

    def test_impersonation_and_ambiguous_modes_fail_closed(self) -> None:
        cases = [
            ("GET", "/api/v1/pods", "watch=true&watch=false", []),
            ("GET", "/api/v1/pods", "", [("Impersonate-User", "admin")]),
            ("POST", "/api/v1/pods/demo", "", []),
            ("GET", "/api/v1/pods", "dryRun=All", []),
        ]
        for case in cases:
            with self.subTest(case=case), self.assertRaises(KubernetesRequestError):
                parse_kubernetes_request(*case)


if __name__ == "__main__":
    unittest.main()
