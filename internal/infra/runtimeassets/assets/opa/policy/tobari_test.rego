package tobari.http

import rego.v1

base_input := {
	"version": "v1",
	"principal": {"cluster": "default", "session": null},
	"request": {
		"scheme": "https",
		"host": "api.github.com",
		"port": 443,
		"method": "GET",
		"path": "/user",
		"path_segments": ["user"],
		"query": {},
		"headers": {},
		"body": {
			"kind": "metadata",
			"size": 0,
			"truncated": false,
			"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
	},
	"credential": {"requested_profile": null},
}

test_deny_by_default if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {"host": "denied.example"})})
	not result.allow
}

test_allow_https_get if {
	result := decision with input as base_input
	result.allow
}

test_deny_plain_http_external if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {"scheme": "http"})})
	not result.allow
}

test_allow_plain_http_test_host if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {
		"scheme": "http",
		"host": "mock-upstream",
		"port": 8080,
	})})
	result.allow
}

test_deny_github_write_path if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {
		"method": "POST",
		"path": "/repos/example/repository/issues",
	})})
	not result.allow
}

test_deny_mock_write_path if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {
		"scheme": "http",
		"host": "mock-upstream",
		"port": 8080,
		"method": "POST",
		"path": "/denied",
	})})
	not result.allow
}

test_allow_bound_credential if {
	result := decision with input as object.union(base_input, {"credential": {"requested_profile": "github-development"}})
	result.allow
}

test_deny_credential_on_other_host if {
	result := decision with input as object.union(base_input, {
		"request": object.union(base_input.request, {"host": "example.com"}),
		"credential": {"requested_profile": "github-development"},
	})
	not result.allow
}
