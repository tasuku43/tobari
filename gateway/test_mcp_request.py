import json
import unittest

from mcp_request import MCPRequestError, parse_mcp_post_request


def request(document):
    body = json.dumps(document, separators=(",", ":")).encode()
    headers = [("content-type", "application/json"), ("content-length", str(len(body)))]
    return body, headers


class MCPRequestTest(unittest.TestCase):
    def test_classifies_bootstrap_without_retaining_params(self):
        body, headers = request({"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {"clientInfo": {"name": "canary"}}})
        parsed = parse_mcp_post_request("POST", headers, body)
        self.assertEqual((parsed.method, parsed.tool_name), ("initialize", None))
        self.assertNotIn("canary", repr(parsed))

    def test_tools_call_retains_only_exact_tool_name(self):
        body, headers = request({"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": {"name": "codex_apps.search", "arguments": {"secret": "canary"}}})
        parsed = parse_mcp_post_request("POST", headers, body)
        self.assertEqual((parsed.method, parsed.tool_name), ("tools/call", "codex_apps.search"))
        self.assertNotIn("canary", repr(parsed))

    def test_rejects_batch_unknown_shape_and_encoded_body(self):
        cases = [
            ([{"jsonrpc": "2.0", "method": "ping"}], None),
            ({"jsonrpc": "2.0", "method": "ping", "secret": "canary"}, None),
            ({"jsonrpc": "2.0", "method": "tools/call", "params": {"arguments": {}}}, None),
        ]
        for document, _ in cases:
            body, headers = request(document)
            with self.assertRaises(MCPRequestError):
                parse_mcp_post_request("POST", headers, body)
        body, headers = request({"jsonrpc": "2.0", "method": "ping"})
        headers.append(("content-encoding", "gzip"))
        with self.assertRaises(MCPRequestError):
            parse_mcp_post_request("POST", headers, body)

    def test_rejects_duplicate_keys_and_nonfinite_numbers_at_any_depth(self):
        bodies = [
            b'{"jsonrpc":"2.0","method":"tools/list","method":"tools/call"}',
            b'{"jsonrpc":"2.0","method":"tools/call","params":{"name":"issues.get","arguments":{"value":1,"value":2}}}',
            b'{"jsonrpc":"2.0","method":"tools/call","params":{"name":"issues.get","arguments":{"value":NaN}}}',
            b'{"jsonrpc":"2.0","method":"tools/call","params":{"name":"issues.get","arguments":{"value":Infinity}}}',
        ]
        for body in bodies:
            with self.subTest(body=body):
                headers = [("content-type", "application/json"), ("content-length", str(len(body)))]
                with self.assertRaises(MCPRequestError):
                    parse_mcp_post_request("POST", headers, body)


if __name__ == "__main__":
    unittest.main()
