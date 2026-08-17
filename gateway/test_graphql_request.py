import json
import unittest

from graphql_request import (
    GraphQLParseLimits,
    GraphQLRequestError,
    parse_graphql_post_request,
    validate_graphql_post_headers,
)


class GraphQLRequestParserTests(unittest.TestCase):
    @staticmethod
    def body(document, **members):
        return json.dumps(
            {"query": document, **members}, separators=(",", ":")
        ).encode("utf-8")

    @staticmethod
    def headers(body, content_type="application/json"):
        return [
            ("Content-Type", content_type),
            ("Content-Length", str(len(body))),
        ]

    def parse(self, document, *, limits=None, **members):
        body = self.body(document, **members)
        arguments = {
            "method": "POST",
            "headers": self.headers(body),
            "body": body,
        }
        if limits is not None:
            arguments["limits"] = limits
        return parse_graphql_post_request(**arguments)

    def assert_rejected(self, code, document, *, limits=None, **members):
        with self.assertRaises(GraphQLRequestError) as caught:
            self.parse(document, limits=limits, **members)
        self.assertEqual(caught.exception.code, code)

    def test_extracts_shorthand_query_and_preserves_original_bytes(self):
        body = b'{"query":"{ viewer { login } }","variables":null}'
        result = parse_graphql_post_request(
            method="POST", headers=self.headers(body), body=body
        )
        self.assertEqual(result.operation_type, "query")
        self.assertEqual(result.root_fields, ("viewer",))
        self.assertIs(result.original_body, body)
        self.assertNotIn("login", repr(result))
        self.assertNotIn("{ viewer", repr(result))

    def test_extracts_mutation_root_fields_sorted_and_unique(self):
        result = self.parse(
            """
            mutation Change($id: ID!, $skip: Boolean!) {
              second: updateIssue(id: $id) @skip(if: $skip) { id title }
              createIssue(input: {title: "secret argument"}) { id }
              first: updateIssue(id: "other") { body }
            }
            """,
            variables={"id": "secret variable", "skip": True},
        )
        self.assertEqual(result.operation_type, "mutation")
        self.assertEqual(result.root_fields, ("createIssue", "updateIssue"))

    def test_root_fragments_are_expanded_but_nested_fields_are_ignored(self):
        result = self.parse(
            """
            query Selected {
              ...RootFields
              ... on Query { directAlias: direct { nestedRootName } }
            }
            fragment RootFields on Query {
              ...MoreRootFields
              viewer { ...NestedFields }
            }
            fragment MoreRootFields on Query { repository { id } }
            fragment NestedFields on Viewer { nestedField anotherNestedField }
            """
        )
        self.assertEqual(result.root_fields, ("direct", "repository", "viewer"))

    def test_directives_and_variable_values_do_not_remove_root_fields(self):
        document = """
            query Conditional($include: Boolean!, $skip: Boolean!) {
              always
              possible @include(if: $include)
              skipped @skip(if: $skip)
            }
        """
        first = self.parse(document, variables={"include": False, "skip": True})
        second = self.parse(document, variables={"include": True, "skip": False})
        self.assertEqual(first.root_fields, ("always", "possible", "skipped"))
        self.assertEqual(first.root_fields, second.root_fields)

    def test_operation_name_selects_execution_but_is_not_returned(self):
        result = self.parse(
            "query Read { viewer } mutation Write { updateIssue { id } }",
            operationName="Write",
        )
        renamed = self.parse(
            "query OtherRead { viewer } mutation OtherWrite { updateIssue { id } }",
            operationName="OtherWrite",
        )
        self.assertEqual(result.operation_type, "mutation")
        self.assertEqual(result.root_fields, ("updateIssue",))
        self.assertEqual(result.operation_type, renamed.operation_type)
        self.assertEqual(result.root_fields, renamed.root_fields)

    def test_extracts_twg_current_user_identity_without_retaining_details(self):
        result = self.parse(
            """
            query TwgCLI_WhoAmIRich {
              me {
                user {
                  ... on AtlassianAccountUser {
                    accountId
                    email
                    name
                  }
                }
              }
            }
            """
        )
        self.assertEqual(result.operation_type, "query")
        self.assertEqual(result.root_fields, ("me",))
        self.assertNotIn("TwgCLI_WhoAmIRich", repr(result))
        self.assertNotIn("accountId", repr(result))
        self.assertNotIn("email", repr(result))

    def test_accepts_only_absent_null_or_empty_extensions(self):
        self.parse("{ viewer }", extensions=None)
        self.parse("{ viewer }", extensions={})
        self.assert_rejected(
            "invalid_envelope",
            "{ viewer }",
            extensions={"persistedQuery": {"sha256Hash": "example"}},
        )

    def test_accepts_utf8_charset_case_insensitively(self):
        body = self.body("{ viewer }")
        result = parse_graphql_post_request(
            method="POST",
            headers=self.headers(body, "Application/JSON;Charset=UTF-8"),
            body=body,
        )
        self.assertEqual(result.root_fields, ("viewer",))

    def test_accepts_absent_content_length_after_bounded_buffering(self):
        body = self.body("{ viewer }")
        result = parse_graphql_post_request(
            method="POST",
            headers=[("Content-Type", "application/json")],
            body=body,
        )
        self.assertEqual(result.operation_type, "query")
        self.assertEqual(result.root_fields, ("viewer",))

    def test_rejects_transport_outside_the_bounded_post_subset(self):
        body = self.body("{ viewer }")
        cases = {
            "method": (
                "unsupported_method",
                {"method": "GET", "headers": self.headers(body), "body": body},
            ),
            "missing content type": (
                "invalid_content_type",
                {
                    "method": "POST",
                    "headers": [("Content-Length", str(len(body)))],
                    "body": body,
                },
            ),
            "duplicate content type": (
                "invalid_content_type",
                {
                    "method": "POST",
                    "headers": self.headers(body)
                    + [("content-type", "application/json")],
                    "body": body,
                },
            ),
            "non-json content type": (
                "invalid_content_type",
                {
                    "method": "POST",
                    "headers": self.headers(body, "application/graphql"),
                    "body": body,
                },
            ),
            "charset": (
                "invalid_content_type",
                {
                    "method": "POST",
                    "headers": self.headers(body, "application/json; charset=latin-1"),
                    "body": body,
                },
            ),
            "zero content length": (
                "invalid_content_length",
                {
                    "method": "POST",
                    "headers": [("Content-Type", "application/json"), ("Content-Length", "0")],
                    "body": body,
                },
            ),
            "duplicate content length": (
                "invalid_content_length",
                {
                    "method": "POST",
                    "headers": self.headers(body)
                    + [("content-length", str(len(body)))],
                    "body": body,
                },
            ),
            "length mismatch": (
                "content_length_mismatch",
                {
                    "method": "POST",
                    "headers": [("Content-Type", "application/json"), ("Content-Length", "1")],
                    "body": body,
                },
            ),
            "transfer encoding": (
                "unsupported_transfer_encoding",
                {
                    "method": "POST",
                    "headers": self.headers(body) + [("Transfer-Encoding", "chunked")],
                    "body": body,
                },
            ),
            "lengthless transfer encoding": (
                "unsupported_transfer_encoding",
                {
                    "method": "POST",
                    "headers": [
                        ("Content-Type", "application/json"),
                        ("Transfer-Encoding", "chunked"),
                    ],
                    "body": body,
                },
            ),
            "content encoding": (
                "unsupported_content_encoding",
                {
                    "method": "POST",
                    "headers": self.headers(body) + [("Content-Encoding", "gzip")],
                    "body": body,
                },
            ),
        }
        for name, (code, arguments) in cases.items():
            with self.subTest(name=name):
                with self.assertRaises(GraphQLRequestError) as caught:
                    parse_graphql_post_request(**arguments)
                self.assertEqual(caught.exception.code, code)

    def test_rejects_oversized_declared_body_before_parsing(self):
        body = self.body("{ viewer }")
        with self.assertRaises(GraphQLRequestError) as caught:
            parse_graphql_post_request(
                method="POST",
                headers=[
                    ("Content-Type", "application/json"),
                    ("Content-Length", str(1024 * 1024 + 1)),
                ],
                body=body,
            )
        self.assertEqual(caught.exception.code, "body_too_large")

    def test_rejects_oversized_lengthless_body_before_parsing(self):
        body = b"{" + b"x" * (1024 * 1024) + b"}"
        with self.assertRaises(GraphQLRequestError) as caught:
            parse_graphql_post_request(
                method="POST",
                headers=[("Content-Type", "application/json")],
                body=body,
            )
        self.assertEqual(caught.exception.code, "body_too_large")

    def test_rejects_non_strict_json_and_envelope_shapes(self):
        cases = {
            "invalid utf8": ("invalid_json", b'{"query":"\xff"}'),
            "duplicate query": ("invalid_json", b'{"query":"{a}","query":"{b}"}'),
            "trailing json": ("invalid_json", b'{"query":"{a}"}{"query":"{b}"}'),
            "batch": ("invalid_envelope", b'[{"query":"{a}"}]'),
            "missing query": ("invalid_envelope", b'{"variables":{}}'),
            "null query": ("invalid_envelope", b'{"query":null}'),
            "operation name type": ("invalid_envelope", b'{"query":"{a}","operationName":1}'),
            "variables type": ("invalid_envelope", b'{"query":"{a}","variables":[]}'),
            "unknown member": ("invalid_envelope", b'{"query":"{a}","other":true}'),
            "non-finite variable": ("invalid_json", b'{"query":"{a}","variables":{"n":NaN}}'),
        }
        for name, (code, body) in cases.items():
            with self.subTest(name=name):
                with self.assertRaises(GraphQLRequestError) as caught:
                    parse_graphql_post_request(
                        method="POST", headers=self.headers(body), body=body
                    )
                self.assertEqual(caught.exception.code, code)

    def test_rejects_invalid_or_unsupported_documents(self):
        cases = {
            "syntax": ("invalid_document", "query Broken {") ,
            "type-system-only": ("invalid_document", "type Query { viewer: String }"),
            "subscription": ("unsupported_operation", "subscription Events { event }") ,
            "undefined fragment": ("undefined_fragment", "query Q { ...Missing }") ,
            "cyclic fragment": (
                "cyclic_fragment",
                "query Q { ...A } fragment A on Query { ...B } fragment B on Query { ...A }",
            ),
            "duplicate fragment": (
                "invalid_document",
                "query Q { ...A } fragment A on Query { a } fragment A on Query { b }",
            ),
            "duplicate operation": (
                "invalid_document",
                "query Q { a } query Q { b }",
            ),
            "anonymous among many": (
                "ambiguous_operation",
                "{ a } query Q { b }",
            ),
        }
        for name, (code, document) in cases.items():
            with self.subTest(name=name):
                self.assert_rejected(code, document, operationName="Q" if name == "duplicate operation" else None)

    def test_rejects_ambiguous_or_missing_selected_operation(self):
        document = "query A { a } query B { b }"
        self.assert_rejected("ambiguous_operation", document)
        self.assert_rejected("missing_operation", document, operationName="Missing")
        self.assert_rejected("missing_operation", "query A { a }", operationName="Missing")

    def test_applies_token_root_fragment_and_depth_limits(self):
        self.assert_rejected(
            "invalid_document",
            "query Q { first second third }",
            limits=GraphQLParseLimits(tokens=5),
        )
        self.assert_rejected(
            "too_many_root_fields",
            "query Q { first second }",
            limits=GraphQLParseLimits(root_field_occurrences=1),
        )
        self.assert_rejected(
            "too_many_fragments",
            "query Q { a } fragment A on Query { a } fragment B on Query { b }",
            limits=GraphQLParseLimits(fragments=1),
        )
        self.assert_rejected(
            "document_too_deep",
            "query Q { first { second { third } } }",
            limits=GraphQLParseLimits(selection_depth=2),
        )
        self.assert_rejected(
            "fragment_too_deep",
            "query Q { ...A } fragment A on Query { ...B } fragment B on Query { field }",
            limits=GraphQLParseLimits(fragment_depth=2),
        )

    def test_limit_configuration_itself_must_be_positive(self):
        body = self.body("{ viewer }")
        with self.assertRaises(ValueError):
            parse_graphql_post_request(
                method="POST",
                headers=self.headers(body),
                body=body,
                limits=GraphQLParseLimits(tokens=0),
            )


if __name__ == "__main__":
    unittest.main()
