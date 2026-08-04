package tobari.http

import rego.v1

base_input := {
	"schema_version": 2,
	"principal": {
		"cluster": "default",
		"project_id": "01912345-6789-7abc-8def-0123456789ab",
	},
	"request": {
		"authority": {
			"scheme": "https",
			"host": "api.github.com",
			"port": 443,
		},
		"method": "GET",
		"path": {"raw": "/user", "segments": ["user"]},
		"query": {},
		"headers": {},
		"body": {
			"state": "empty",
			"size": 0,
			"truncated": false,
			"sha256": "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			"content_type": "",
		},
	},
	"authorization": {"requested_profile": null},
}

input_with_request(request) := object.union(base_input, {"request": request})

request_with_authority(overrides) := object.union(base_input.request, {"authority": object.union(base_input.request.authority, overrides)})

request_with_path(overrides) := object.union(base_input.request, {"path": object.union(base_input.request.path, overrides)})

request_with_body(body) := object.union(base_input.request, {"body": body})

test_deny_by_default if {
	result := decision with input as input_with_request(request_with_authority({"host": "denied.example"}))
	not result.allow
	result.learnable
}

test_deny_missing_authorization_shape if {
	malformed := object.remove(base_input, ["authorization"])
	result := decision with input as malformed
	not result.allow
	not result.learnable
}

test_deny_invalid_requested_profile_shape if {
	result := decision with input as object.union(base_input, {"authorization": {"requested_profile": 17}})
	not result.allow
	not result.learnable
}

test_allow_https_get if {
	result := decision with input as base_input
	result.allow
}

test_allow_plain_http_test_host if {
	request := request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080})
	result := decision with input as input_with_request(request)
	result.allow
}

test_deny_nonempty_body_without_body_policy if {
	body := object.union(base_input.request.body, {
		"state": "metadata",
		"size": 1,
		"sha256": "2bb80d537b1da3e38bd30361aa855686bde0ba3d6190a9f3f5f4f5f5f5f5f5f5",
	})
	result := decision with input as input_with_request(request_with_body(body))
	not result.allow
	not result.learnable
}

test_deny_unavailable_body if {
	body := {
		"state": "unavailable",
		"size": null,
		"truncated": null,
		"sha256": null,
		"content_type": "",
	}
	result := decision with input as input_with_request(request_with_body(body))
	not result.allow
	not result.learnable
}

test_deny_https_non_default_port if {
	result := decision with input as input_with_request(request_with_authority({"port": 8443}))
	not result.allow
	not result.learnable
}

test_deny_plain_http_external if {
	result := decision with input as input_with_request(request_with_authority({
		"scheme": "http",
		"host": "example.com",
		"port": 8080,
	}))
	not result.allow
	not result.learnable
}

test_deny_github_write_path if {
	request := object.union(
		request_with_path({"raw": "/repos/example/repository/issues", "segments": ["repos", "example", "repository", "issues"]}),
		{"method": "POST"},
	)
	result := decision with input as input_with_request(request)
	not result.allow
	not result.learnable
}

test_learnable_denial_preserves_bound_credential_profile if {
	request := object.union(request_with_path({"raw": "/graphql", "segments": ["graphql"]}), {"method": "PUT"})
	result := decision with input as object.union(input_with_request(request), {"authorization": {"requested_profile": "github-development"}})
	not result.allow
	result.learnable
	result.credential_profile == "github-development"
}

learned_exact_fixture := {
	"id": "plr_0123456789abcdef0123456789abcdef",
	"match": "exact",
	"project_id": "01912345-6789-7abc-8def-0123456789ab",
	"host": "api.github.com",
	"port": 443,
	"method": "PUT",
	"path": "/graphql",
	"examples": ["/graphql"],
	"source_candidates": ["pcy_0123456789abcdef0123456789abcdef"],
}

learned_prefix_fixture := {
	"id": "plr_abcdef0123456789abcdef0123456789",
	"match": "prefix",
	"project_id": "01912345-6789-7abc-8def-0123456789ab",
	"host": "mock-upstream",
	"port": 8080,
	"method": "PUT",
	"path": "/review/items/",
	"examples": [
		"/review/items/one",
		"/review/items/three",
		"/review/items/two",
	],
	"source_candidates": [
		"pcy_0123456789abcdef0123456789abcdef",
		"pcy_1123456789abcdef0123456789abcdef",
		"pcy_2123456789abcdef0123456789abcdef",
	],
}

test_explicit_deny_wins_over_learned_allow if {
	deny_rule := {
		"id": "pdr_0123456789abcdef0123456789abcdef",
		"project_id": base_input.principal.project_id,
		"host": "api.github.com",
		"port": 443,
		"method": "GET",
		"path": "/user",
		"source_candidates": ["pcy_0123456789abcdef0123456789abcdef"],
	}
	allow_rule := object.union(learned_exact_fixture, {
		"method": "GET",
		"path": "/user",
		"examples": ["/user"],
	})
	result := decision with input as base_input
		with data.tobari.rules.learned_allows as [allow_rule]
		with data.tobari.rules.learned_denies as [deny_rule]
	not result.allow
	not result.learnable
}

test_exact_learned_rule_allows_exact_request if {
	request := object.union(request_with_path({"raw": learned_exact_fixture.path, "segments": ["graphql"]}), {"method": learned_exact_fixture.method})
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [learned_exact_fixture]
	result.allow
}

test_exact_learned_rule_does_not_allow_child_path if {
	request := object.union(request_with_path({"raw": "/graphql/child", "segments": ["graphql", "child"]}), {"method": learned_exact_fixture.method})
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [learned_exact_fixture]
	not result.allow
}

test_learned_rule_does_not_cross_port if {
	request := object.union(
		request_with_path({"raw": learned_exact_fixture.path, "segments": ["graphql"]}),
		{"method": learned_exact_fixture.method, "authority": object.union(base_input.request.authority, {"port": 8443})},
	)
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [learned_exact_fixture]
	not result.allow
}

test_learned_rule_does_not_cross_scheme if {
	request := object.union(
		request_with_path({"raw": learned_exact_fixture.path, "segments": ["graphql"]}),
		{"method": learned_exact_fixture.method, "authority": {"scheme": "http", "host": "api.github.com", "port": 8080}},
	)
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [learned_exact_fixture]
	not result.allow
}

test_compacted_prefix_allows_declared_directory if {
	request := object.union(
		request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080}),
		{"method": "PUT", "path": {"raw": "/review/items/four", "segments": ["review", "items", "four"]}},
	)
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [learned_prefix_fixture]
	result.allow
}

test_compacted_prefix_rejects_outside_canary if {
	request := object.union(
		request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080}),
		{"method": "PUT", "path": {"raw": "/review/items-outside-tobari-canary", "segments": ["review", "items-outside-tobari-canary"]}},
	)
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [learned_prefix_fixture]
	not result.allow
}

test_compacted_prefix_rejects_ambiguous_paths if {
	every unsafe_path in {
		"/review/items/%2Fadmin",
		"/review/items/../admin",
		"/review/items//admin",
		"/review/items\\admin",
	} {
		request := object.union(
			request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080}),
			{"method": "PUT", "path": {"raw": unsafe_path, "segments": ["review", "items", "unsafe"]}},
		)
		result := decision with input as input_with_request(request)
			with data.tobari.rules.learned_allows as [learned_prefix_fixture]
		not result.allow
	}
}

test_malformed_learned_rule_fails_closed if {
	unsafe := object.union(learned_prefix_fixture, {"host": "denied.example", "path": "/api/"})
	request := object.union(request_with_path({"raw": "/api/anything", "segments": ["api", "anything"]}), {"method": "PUT"})
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [unsafe]
	not result.allow
}

test_learned_rule_does_not_cross_project if {
	request := object.union(request_with_path({"raw": learned_exact_fixture.path, "segments": ["graphql"]}), {"method": learned_exact_fixture.method})
	principal := object.union(base_input.principal, {"project_id": "01912345-6789-7abc-8def-0123456789ac"})
	result := decision with input as object.union(input_with_request(request), {"principal": principal})
		with data.tobari.rules.learned_allows as [learned_exact_fixture]
	not result.allow
	result.learnable
}

test_allow_bound_credential if {
	result := decision with input as object.union(base_input, {"authorization": {"requested_profile": "github-development"}})
	result.allow
}

test_deny_credential_on_other_host if {
	request := request_with_authority({"host": "example.com"})
	result := decision with input as object.union(input_with_request(request), {"authorization": {"requested_profile": "github-development"}})
	not result.allow
	not result.learnable
}

test_deny_mock_write_path if {
	request := object.union(
		request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080}),
		{"method": "POST", "path": {"raw": "/denied", "segments": ["denied"]}},
	)
	result := decision with input as input_with_request(request)
	not result.allow
	not result.learnable
}
