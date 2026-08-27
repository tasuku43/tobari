import unittest

from addon.kubernetes_request import KubernetesRequestError, parse_kubernetes_request


class KubernetesRequestTests(unittest.TestCase):
    def test_core_and_crd_resources_need_no_schema(self) -> None:
        core = parse_kubernetes_request("GET", "/api/v1/namespaces/team/pods/demo/log", "", [])
        crd = parse_kubernetes_request("PATCH", "/apis/acme.example/v1/widgets/blue", "dryRun=All", [])
        self.assertEqual(
            core,
            type(core)(kind="resource", verb="get", dry_run="none", group="", version="v1", resource="pods", namespace="team", name="demo", subresource="log"),
        )
        self.assertEqual(
            crd,
            type(crd)(kind="resource", verb="patch", dry_run="all", group="acme.example", version="v1", resource="widgets", name="blue"),
        )

    def test_list_watch_mutation_and_interactive_are_distinct(self) -> None:
        self.assertEqual(parse_kubernetes_request("GET", "/api/v1/pods", "", []).verb, "list")
        self.assertEqual(parse_kubernetes_request("GET", "/api/v1/pods", "watch=true", []).verb, "watch")
        self.assertEqual(parse_kubernetes_request("POST", "/api/v1/namespaces/team/pods", "", []).verb, "create")
        self.assertEqual(parse_kubernetes_request("POST", "/api/v1/namespaces/team/pods/demo/exec", "", []).verb, "connect")

    def test_namespaces_plural_is_resource_until_scope_is_unambiguous(self) -> None:
        listed = parse_kubernetes_request("GET", "/api/v1/namespaces", "", [])
        named = parse_kubernetes_request("GET", "/api/v1/namespaces/team", "", [])
        scoped = parse_kubernetes_request("GET", "/api/v1/namespaces/team/pods", "", [])
        ambiguous = parse_kubernetes_request("GET", "/api/v1/namespaces/team/status", "", [])
        self.assertEqual((listed.resource, listed.name, listed.namespace), ("namespaces", None, None))
        self.assertEqual((named.resource, named.name, named.namespace), ("namespaces", "team", None))
        self.assertEqual((scoped.resource, scoped.name, scoped.namespace), ("pods", None, "team"))
        self.assertEqual((ambiguous.resource, ambiguous.name, ambiguous.namespace), ("status", None, "team"))

    def test_discovery_is_exact_non_resource_read(self) -> None:
        for path in ("/apis", "/api/v1", "/apis/apps/v1", "/openapi/v3"):
            parsed = parse_kubernetes_request("GET", path, "", [])
            self.assertEqual((parsed.kind, parsed.verb, parsed.non_resource_path), ("non_resource", "get", path))

    def test_non_resource_path_set_is_canonical_and_closed(self) -> None:
        for path in ("/api", "/apis", "/api/v1", "/apis/apps/v1", "/healthz", "/openapi/v3"):
            self.assertEqual(parse_kubernetes_request("GET", path, "", []).non_resource_path, path)
        for path in ("/apis/apps", "//healthz", "/api//", "/healthz/", "/version\u2028"):
            with self.subTest(path=path), self.assertRaises(KubernetesRequestError):
                parse_kubernetes_request("GET", path, "", [])

    def test_projection_variants_are_exclusive(self) -> None:
        invalid = (
            dict(kind="future", verb="get"),
            dict(kind="resource", verb="get", dry_run="none", group="", version="v1", resource="pods", non_resource_path="/api"),
            dict(kind="non_resource", verb="get", non_resource_path="/api", group=""),
        )
        for fields in invalid:
            with self.subTest(fields=fields), self.assertRaises(KubernetesRequestError):
                type(parse_kubernetes_request("GET", "/api", "", []))(**fields)

    def test_impersonation_and_ambiguous_modes_fail_closed(self) -> None:
        cases = [
            ("GET", "/api/v1/pods", "watch=true&watch=false", []),
            ("GET", "/api/v1/pods", "", [("Impersonate-User", "admin")]),
            ("POST", "/api/v1/pods/demo", "", []),
            ("GET", "/api/v1/pods", "dryRun=All", []),
            ("GET", "/api/v1/pods/demo%2Fother", "", []),
            ("GET", "//api/v1/pods", "", []),
            ("GET", "/api/v1/pods/", "", []),
            ("GET", "/api/v1/pods/demo", "watch=true", []),
            ("GET", "/apis/acme.example/v1/widgets\u2028", "", []),
        ]
        for case in cases:
            with self.subTest(case=case), self.assertRaises(KubernetesRequestError):
                parse_kubernetes_request(*case)


if __name__ == "__main__":
    unittest.main()
