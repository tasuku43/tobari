package tobari.http

import rego.v1

base_input := {
	"version": "v1",
	"principal": {
		"cluster": "default",
		"project_id": "01912345-6789-7abc-8def-0123456789ab",
		"session": null,
	},
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
		with data.tobari.learned_allow_rules as []
	not result.allow
	result.learnable
}

test_allow_https_get if {
	result := decision with input as base_input
	result.allow
}

test_deny_nonempty_body_without_body_policy if {
	request := object.union(base_input.request, {"body": object.union(base_input.request.body, {
		"size": 1,
		"kind": "metadata",
		"sha256": "2bb80d537b1da3e38bd30361aa855686bde0ba3d6190a9f3f5f4f5f5f5f5f5f5",
	})})
	result := decision with input as object.union(base_input, {"request": request})
	not result.allow
	not result.learnable
}

test_deny_unavailable_body if {
	request := object.union(base_input.request, {"body": {
		"kind": "unavailable",
		"size": null,
		"truncated": null,
		"sha256": null,
	}})
	result := decision with input as object.union(base_input, {"request": request})
	not result.allow
	not result.learnable
}

test_deny_https_non_default_port if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {"port": 8443})})
	not result.allow
	not result.learnable
}

test_deny_plain_http_external if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {"scheme": "http"})})
	not result.allow
	not result.learnable
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
		with data.tobari.learned_allow_rules as []
	not result.allow
}

test_learnable_denial_preserves_bound_credential_profile if {
	request := object.union(base_input.request, {
		"method": "POST",
		"path": "/repos/example/repository/issues",
	})
	result := decision with input as object.union(base_input, {
		"request": request,
		"credential": {"requested_profile": "github-development"},
	})
		with data.tobari.learned_allow_rules as []
	not result.allow
	result.learnable
	result.credential_profile == "github-development"
}

test_deny_mock_write_path if {
	result := decision with input as object.union(base_input, {"request": object.union(base_input.request, {
		"scheme": "http",
		"host": "mock-upstream",
		"port": 8080,
		"method": "POST",
		"path": "/denied",
	})})
		with data.tobari.learned_allow_rules as []
	not result.allow
	result.learnable
}

learned_exact_fixture := {
	"id": "plr_0123456789abcdef0123456789abcdef",
	"match": "exact",
	"project_id": "01912345-6789-7abc-8def-0123456789ab",
	"host": "api.github.com",
	"port": 443,
	"method": "POST",
	"path": "/repos/example/repository/issues",
	"examples": ["/repos/example/repository/issues"],
	"source_candidates": ["pcy_0123456789abcdef0123456789abcdef"],
}

learned_prefix_fixture := {
	"id": "plr_abcdef0123456789abcdef0123456789",
	"match": "prefix",
	"project_id": "01912345-6789-7abc-8def-0123456789ab",
	"host": "mock-upstream",
	"port": 8080,
	"method": "POST",
	"path": "/api/v1/items/",
	"examples": [
		"/api/v1/items/one",
		"/api/v1/items/three",
		"/api/v1/items/two",
	],
	"source_candidates": [
		"pcy_0123456789abcdef0123456789abcdef",
		"pcy_1123456789abcdef0123456789abcdef",
		"pcy_2123456789abcdef0123456789abcdef",
	],
}

learned_scheme(port) := "https" if {
	port == 443
}

learned_scheme(port) := "http" if {
	port == 8080
}

test_exact_learned_rule_overrides_matching_legacy_deny if {
	request := object.union(base_input.request, {
		"method": learned_exact_fixture.method,
		"path": learned_exact_fixture.path,
	})
	result := decision with input as object.union(base_input, {"request": request})
		with data.tobari.learned_allow_rules as [learned_exact_fixture]
	result.allow
}

test_exact_learned_rule_does_not_allow_child_path if {
	request := object.union(base_input.request, {
		"method": learned_exact_fixture.method,
		"path": sprintf("%s/child", [learned_exact_fixture.path]),
	})
	result := decision with input as object.union(base_input, {"request": request})
		with data.tobari.learned_allow_rules as [learned_exact_fixture]
	not result.allow
}

test_learned_rule_does_not_cross_port if {
	request := object.union(base_input.request, {
		"port": 8443,
		"method": learned_exact_fixture.method,
		"path": learned_exact_fixture.path,
	})
	result := decision with input as object.union(base_input, {"request": request})
		with data.tobari.learned_allow_rules as [learned_exact_fixture]
	not result.allow
}

test_learned_rule_does_not_cross_scheme if {
	request := object.union(base_input.request, {
		"scheme": "http",
		"port": 8080,
		"method": learned_exact_fixture.method,
		"path": learned_exact_fixture.path,
	})
	result := decision with input as object.union(base_input, {"request": request})
		with data.tobari.learned_allow_rules as [learned_exact_fixture]
	not result.allow
}

test_compacted_prefix_allows_declared_directory if {
	request := object.union(base_input.request, {
		"scheme": "http",
		"host": learned_prefix_fixture.host,
		"port": 8080,
		"method": learned_prefix_fixture.method,
		"path": "/api/v1/items/four",
	})
	result := decision with input as object.union(base_input, {"request": request})
		with data.tobari.learned_allow_rules as [learned_prefix_fixture]
	result.allow
}

test_compacted_prefix_rejects_outside_canary if {
	request := object.union(base_input.request, {
		"scheme": "http",
		"host": learned_prefix_fixture.host,
		"port": 8080,
		"method": learned_prefix_fixture.method,
		"path": "/api/v1/items-outside-tobari-canary",
	})
	result := decision with input as object.union(base_input, {"request": request})
		with data.tobari.learned_allow_rules as [learned_prefix_fixture]
	not result.allow
}

test_compacted_prefix_rejects_ambiguous_paths if {
	every unsafe_path in {
		"/api/v1/items/%2Fadmin",
		"/api/v1/items/../admin",
		"/api/v1/items//admin",
		"/api/v1/items\\admin",
	} {
		request := object.union(base_input.request, {
			"scheme": "http",
			"host": learned_prefix_fixture.host,
			"port": 8080,
			"method": learned_prefix_fixture.method,
			"path": unsafe_path,
		})
		result := decision with input as object.union(base_input, {"request": request})
			with data.tobari.learned_allow_rules as [learned_prefix_fixture]
		not result.allow
	}
}

test_malformed_learned_rule_fails_closed if {
	unsafe := object.union(learned_prefix_fixture, {
		"host": "denied.example",
		"path": "/api/",
	})
	request := object.union(base_input.request, {
		"host": unsafe.host,
		"method": unsafe.method,
		"path": "/api/anything",
	})
	result := decision with input as object.union(base_input, {"request": request})
		with data.tobari.learned_allow_rules as [unsafe]
	not result.allow
}

test_all_learned_rules_are_valid if {
	every rule in learned_rules {
		learned_rule_valid(rule)
	}
}

test_every_learned_example_matches_and_is_allowed if {
	every rule in learned_rules {
		every example in rule.examples {
			request := object.union(base_input.request, {
				"scheme": learned_scheme(rule.port),
				"host": rule.host,
				"port": rule.port,
				"method": rule.method,
				"path": example,
			})
			learned_rule_matches_request(rule, base_input.principal.project_id, request)
			result := decision with input as object.union(base_input, {"request": request})
			result.allow
		}
	}
}

test_every_exact_rule_rejects_a_child_path if {
	every rule in learned_rules {
		exact_rule_rejects_child(rule)
	}
}

exact_rule_rejects_child(rule) if {
	rule.match != "exact"
}

exact_rule_rejects_child(rule) if {
	rule.match == "exact"
	not learned_rule_matches_request(
		rule,
		base_input.principal.project_id,
		object.union(base_input.request, {
			"host": rule.host,
			"method": rule.method,
			"path": sprintf("%s/child", [rule.path]),
		}),
	)
}

test_every_prefix_rule_rejects_an_outside_canary if {
	every rule in learned_rules {
		prefix_rule_rejects_outside_canary(rule)
	}
}

prefix_rule_rejects_outside_canary(rule) if {
	rule.match != "prefix"
}

prefix_rule_rejects_outside_canary(rule) if {
	rule.match == "prefix"
	not learned_rule_matches_request(
		rule,
		base_input.principal.project_id,
		object.union(base_input.request, {
			"host": rule.host,
			"method": rule.method,
			"path": sprintf("%s-outside-tobari-canary", [trim_suffix(rule.path, "/")]),
		}),
	)
}

test_learned_rule_does_not_cross_project if {
	request := object.union(base_input.request, {
		"method": learned_exact_fixture.method,
		"path": learned_exact_fixture.path,
	})
	principal := object.union(base_input.principal, {"project_id": "01912345-6789-7abc-8def-0123456789ac"})
	result := decision with input as object.union(base_input, {
		"principal": principal,
		"request": request,
	})
		with data.tobari.learned_allow_rules as [learned_exact_fixture]
	not result.allow
	result.learnable
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
	not result.learnable
}
