package tobari.http

import rego.v1

base_input := {
	"schema_version": 1,
	"principal": {
		"cluster": "default",
		"context_id": "01912345-6789-7abc-8def-0123456789ad",
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
	},
	"authorization": {"requested_profile": null, "broker_provider": null},
}

input_with_request(request) := object.union(base_input, {"request": request})

request_with_authority(overrides) := object.union(base_input.request, {"authority": object.union(base_input.request.authority, overrides)})

request_with_path(overrides) := object.union(base_input.request, {"path": object.union(base_input.request.path, overrides)})

test_deny_by_default if {
	result := decision with input as input_with_request(request_with_authority({"host": "denied.example"}))
	not result.allow
	result.learnable
}

test_readme_quickstart_put_is_learnable if {
	request := object.union(
		request_with_authority({"host": "example.com"}),
		{"method": "PUT", "path": {"raw": "/quickstart", "segments": ["quickstart"]}},
	)
	result := decision with input as input_with_request(request)
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
	result := decision with input as object.union(base_input, {"authorization": {"requested_profile": 17, "broker_provider": null}})
	not result.allow
	not result.learnable
}

test_deny_invalid_broker_provider_shape if {
	result := decision with input as object.union(base_input, {"authorization": {"requested_profile": null, "broker_provider": "GitHub.com"}})
	not result.allow
	not result.learnable
}

test_deny_unknown_authorization_field if {
	result := decision with input as object.union(base_input, {"authorization": {"requested_profile": null, "broker_provider": null, "handle": "redacted"}})
	not result.allow
	not result.learnable
}

test_broker_authorization_requires_exact_learned_permission if {
	request := object.union(request_with_path({"raw": "/broker-api", "segments": ["broker-api"]}), {"method": "POST"})
	result := decision with input as object.union(
		input_with_request(request),
		{"authorization": {"requested_profile": null, "broker_provider": "github"}},
	)
	not result.allow
	result.learnable
	result.credential_profile == null
}

test_allow_provider_neutral_broker_authorization_after_learning if {
	request := object.union(request_with_path({"raw": "/broker-api", "segments": ["broker-api"]}), {"method": "POST"})
	allow_rule := object.union(learned_exact_fixture, {"method": "POST", "path": "/broker-api", "examples": ["/broker-api"]})
	result := decision with input as object.union(
		input_with_request(request),
		{"authorization": {"requested_profile": null, "broker_provider": "github"}},
	)
		with data.tobari.rules.learned_allows as [allow_rule]
	result.allow
	result.credential_profile == null
}

test_allow_https_get if {
	result := decision with input as base_input
	result.allow
}

test_domain_methods_do_not_authorize_another_domain if {
	authorities := [
		{"scheme": "https", "host": "api.github.com", "ports": [443], "methods": {"read": ["GET"], "write": [{"method": "POST", "exclude_path_prefixes": []}]}},
		{"scheme": "https", "host": "example.com", "ports": [443], "methods": {"read": ["GET"], "write": []}},
	]
	request := object.union(
		request_with_authority({"host": "example.com"}),
		{"method": "POST", "path": {"raw": "/write", "segments": ["write"]}},
	)
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.authorities as authorities
	not result.allow
	result.learnable
}

test_allow_plain_http_test_host if {
	request := request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080})
	result := decision with input as input_with_request(request)
	result.allow
}

test_body_content_is_not_a_policy_dimension if {
	request := object.union(request_with_path({"raw": "/http-review", "segments": ["http-review"]}), {"method": "PUT"})
	first := input_with_request(object.union(request, {"body": {"value": "first"}}))
	second := input_with_request(object.union(request, {"body": {"value": "second"}}))
	first_result := decision with input as first
		with data.tobari.rules.learned_allows as [learned_exact_fixture]
	second_result := decision with input as second
		with data.tobari.rules.learned_allows as [learned_exact_fixture]
	first_result.allow
	second_result.allow
}

test_body_bearing_put_and_patch_are_learnable if {
	every method in {"PUT", "PATCH"} {
		request := object.union(
			request_with_path({"raw": "/body-review", "segments": ["body-review"]}),
			{"method": method, "body": {"value": method}},
		)
		result := decision with input as input_with_request(request)
		not result.allow
		result.learnable
	}
}

test_https_non_default_port_is_learnable if {
	result := decision with input as input_with_request(request_with_authority({"port": 8443}))
	not result.allow
	result.learnable
}

test_invalid_transport_ports_are_terminal if {
	every port in {0, 65536, "443"} {
		result := decision with input as input_with_request(request_with_authority({"port": port}))
		not result.allow
		not result.learnable
	}
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
	request := object.union(request_with_path({"raw": "/credential-review", "segments": ["credential-review"]}), {"method": "PUT"})
	result := decision with input as object.union(input_with_request(request), {"authorization": {"requested_profile": "github-development", "broker_provider": null}})
	not result.allow
	result.learnable
	result.credential_profile == "github-development"
}

learned_exact_fixture := {
	"id": "plr_0123456789abcdef0123456789abcdef",
	"match": "exact",
	"context_id": "01912345-6789-7abc-8def-0123456789ad",
	"project_id": "01912345-6789-7abc-8def-0123456789ab",
	"host": "api.github.com",
	"port": 443,
	"protocol": "http",
	"method": "PUT",
	"path": "/http-review",
	"examples": ["/http-review"],
	"source_candidates": ["pcy_0123456789abcdef0123456789abcdef"],
}

graphql_endpoint_fixture := {
	"scheme": "https",
	"host": "api.github.com",
	"port": 443,
	"path": "/graphql",
}

graphql_request_fixture := object.union(
	object.union(request_with_path({"raw": "/graphql", "segments": ["graphql"]}), {"method": "POST"}),
	{"graphql": {"operation_type": "query", "root_fields": ["viewer"]}},
)

graphql_allow_fixture := object.union(learned_exact_fixture, {
	"method": "POST",
	"path": "/graphql",
	"examples": ["/graphql"],
	"protocol": "graphql",
	"graphql_operation_type": "query",
	"graphql_root_field": "viewer",
})

graphql_second_allow_fixture := object.union(graphql_allow_fixture, {
	"id": "plr_1123456789abcdef0123456789abcdef",
	"graphql_root_field": "repository",
	"source_candidates": ["pcy_1123456789abcdef0123456789abcdef"],
})

graphql_deny_fixture := {
	"id": "pdr_0123456789abcdef0123456789abcdef",
	"context_id": base_input.principal.context_id,
	"project_id": base_input.principal.project_id,
	"host": "api.github.com",
	"port": 443,
	"method": "POST",
	"path": "/graphql",
	"protocol": "graphql",
	"graphql_operation_type": "query",
	"graphql_root_field": "viewer",
	"source_candidates": ["pcy_0123456789abcdef0123456789abcdef"],
}

test_graphql_generic_http_allow_does_not_authorize_declared_endpoint if {
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
	not result.allow
	result.learnable
}

test_graphql_http_learned_rule_does_not_authorize_declared_endpoint if {
	http_rule := object.union(learned_exact_fixture, {"method": "POST"})
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [http_rule]
	not result.allow
	result.learnable
}

test_graphql_explicit_http_learned_rule_does_not_authorize_declared_endpoint if {
	http_rule := object.union(learned_exact_fixture, {"method": "POST", "protocol": "http"})
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [http_rule]
	not result.allow
	result.learnable
}

test_graphql_exact_root_rule_allows_declared_endpoint if {
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
	result.allow
}

test_graphql_exact_mutation_rule_allows_declared_endpoint if {
	request := object.union(graphql_request_fixture, {"graphql": {"operation_type": "mutation", "root_fields": ["updateIssue"]}})
	allow_rule := object.union(graphql_allow_fixture, {
		"graphql_operation_type": "mutation",
		"graphql_root_field": "updateIssue",
	})
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [allow_rule]
	result.allow
}

test_graphql_requires_every_distinct_root if {
	request := object.union(graphql_request_fixture, {"graphql": {"operation_type": "query", "root_fields": ["repository", "viewer"]}})
	partial := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
	complete := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture, graphql_second_allow_fixture]
	not partial.allow
	partial.learnable
	complete.allow
}

test_graphql_operation_type_is_exact if {
	request := object.union(graphql_request_fixture, {"graphql": {"operation_type": "mutation", "root_fields": ["viewer"]}})
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
	not result.allow
	result.learnable
}

test_graphql_root_field_is_exact if {
	request := object.union(graphql_request_fixture, {"graphql": {"operation_type": "query", "root_fields": ["viewerLogin"]}})
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
	not result.allow
	result.learnable
}

test_graphql_prefix_rule_fails_closed if {
	prefix_rule := object.union(graphql_allow_fixture, {
		"match": "prefix",
		"path": "/graphql/",
		"examples": ["/graphql/a", "/graphql/b", "/graphql/c"],
		"source_candidates": [
			"pcy_0123456789abcdef0123456789abcdef",
			"pcy_1123456789abcdef0123456789abcdef",
			"pcy_2123456789abcdef0123456789abcdef",
		],
	})
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [prefix_rule]
	not result.allow
	result.learnable
}

test_graphql_exact_deny_wins_over_allow if {
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
		with data.tobari.rules.learned_denies as [graphql_deny_fixture]
	not result.allow
	not result.learnable
}

test_graphql_deny_for_another_root_does_not_match if {
	other_deny := object.union(graphql_deny_fixture, {"graphql_root_field": "repository"})
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
		with data.tobari.rules.learned_denies as [other_deny]
	result.allow
}

test_graphql_http_deny_does_not_match if {
	http_deny := object.remove(graphql_deny_fixture, ["protocol", "graphql_operation_type", "graphql_root_field"])
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
		with data.tobari.rules.learned_denies as [http_deny]
	result.allow
}

test_graphql_baseline_path_deny_remains_protocol_agnostic if {
	request := object.union(
		object.union(
			request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080}),
			{"method": "POST", "path": {"raw": "/denied", "segments": ["denied"]}},
		),
		{"graphql": {"operation_type": "mutation", "root_fields": ["updateThing"]}},
	)
	endpoint := {"scheme": "http", "host": "mock-upstream", "port": 8080, "path": "/denied"}
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [endpoint]
	not result.allow
	not result.learnable
}

test_graphql_identity_does_not_authorize_ordinary_http if {
	request := object.remove(graphql_request_fixture, ["graphql"])
	request_input := object.union(
		input_with_request(request),
		{"authorization": {"requested_profile": null, "broker_provider": "github"}},
	)
	result := decision with input as request_input
		with data.tobari.boundary.graphql_endpoints as []
		with data.tobari.rules.learned_allows as [graphql_allow_fixture]
	not result.allow
	result.learnable
}

test_explicit_http_protocol_preserves_http_learning if {
	http_rule := object.union(learned_exact_fixture, {"protocol": "http"})
	request := object.union(request_with_path({"raw": http_rule.path, "segments": ["graphql"]}), {"method": http_rule.method})
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [http_rule]
	result.allow
}

test_http_null_graphql_preserves_ordinary_authorization if {
	request := object.union(base_input.request, {"graphql": null})
	result := decision with input as input_with_request(request)
	result.allow
}

test_explicit_http_protocol_preserves_exact_deny if {
	deny_rule := {
		"id": "pdr_0123456789abcdef0123456789abcdef",
		"context_id": base_input.principal.context_id,
		"project_id": base_input.principal.project_id,
		"host": "api.github.com",
		"port": 443,
		"method": "GET",
		"path": "/user",
		"protocol": "http",
		"source_candidates": ["pcy_0123456789abcdef0123456789abcdef"],
	}
	result := decision with input as base_input
		with data.tobari.rules.learned_denies as [deny_rule]
	not result.allow
	not result.learnable
}

test_declared_graphql_endpoint_never_falls_back_to_http if {
	request := object.union(graphql_request_fixture, {"graphql": null})
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
	not result.allow
	not result.learnable
}

test_declared_graphql_endpoint_rejects_absent_identity if {
	request := object.remove(graphql_request_fixture, ["graphql"])
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
	not result.allow
	not result.learnable
}

test_graphql_identity_requires_exact_declared_endpoint if {
	every endpoint in {
		object.union(graphql_endpoint_fixture, {"scheme": "http", "port": 8080}),
		object.union(graphql_endpoint_fixture, {"host": "example.com"}),
		object.union(graphql_endpoint_fixture, {"port": 8443}),
		object.union(graphql_endpoint_fixture, {"path": "/graphql/v2"}),
	} {
		result := decision with input as input_with_request(graphql_request_fixture)
			with data.tobari.boundary.graphql_endpoints as [endpoint]
		not result.allow
		not result.learnable
	}
}

test_graphql_rejects_non_post_method if {
	request := object.union(graphql_request_fixture, {"method": "GET"})
	result := decision with input as input_with_request(request)
		with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
	not result.allow
	not result.learnable
}

test_graphql_rejects_malformed_identity_shapes if {
	every malformed in {
		{},
		{"operation_type": "query"},
		{"operation_type": "query", "root_fields": ["viewer"], "variables": {}},
		{"operation_type": "subscription", "root_fields": ["viewer"]},
		{"operation_type": "query", "root_fields": "viewer"},
		{"operation_type": "query", "root_fields": []},
		{"operation_type": "query", "root_fields": ["viewer", "repository"]},
		{"operation_type": "query", "root_fields": ["viewer", "viewer"]},
		{"operation_type": "query", "root_fields": [""]},
		{"operation_type": "query", "root_fields": ["invalid-name"]},
		{"operation_type": "query", "root_fields": [17]},
	} {
		request := object.union(object.remove(graphql_request_fixture, ["graphql"]), {"graphql": malformed})
		result := decision with input as input_with_request(request)
			with data.tobari.boundary.graphql_endpoints as [graphql_endpoint_fixture]
		not result.allow
		not result.learnable
	}
}

test_graphql_rejects_malformed_endpoint_declaration if {
	malformed_endpoint := object.union(graphql_endpoint_fixture, {"provider": "github"})
	result := decision with input as input_with_request(graphql_request_fixture)
		with data.tobari.boundary.graphql_endpoints as [malformed_endpoint]
	not result.allow
	not result.learnable
}

test_malformed_graphql_endpoint_collection_fails_closed_for_http if {
	malformed_endpoint := object.union(graphql_endpoint_fixture, {"provider": "github"})
	result := decision with input as base_input
		with data.tobari.boundary.graphql_endpoints as [malformed_endpoint]
	not result.allow
	not result.learnable
}

test_explicit_deny_wins_over_learned_allow if {
	deny_rule := {
		"id": "pdr_0123456789abcdef0123456789abcdef",
		"context_id": base_input.principal.context_id,
		"project_id": base_input.principal.project_id,
		"host": "api.github.com",
		"port": 443,
		"protocol": "http",
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

test_retired_learned_prefix_rule_fails_closed if {
	retired_prefix_rule := object.union(learned_exact_fixture, {
		"match": "prefix",
		"host": "mock-upstream",
		"port": 8080,
		"method": "PUT",
		"path": "/review/items/",
		"examples": ["/review/items/one", "/review/items/three", "/review/items/two"],
		"source_candidates": [
			"pcy_0123456789abcdef0123456789abcdef",
			"pcy_1123456789abcdef0123456789abcdef",
			"pcy_2123456789abcdef0123456789abcdef",
		],
	})
	request := object.union(
		request_with_authority({"scheme": "http", "host": "mock-upstream", "port": 8080}),
		{"method": "PUT", "path": {"raw": "/review/items/four", "segments": ["review", "items", "four"]}},
	)
	result := decision with input as input_with_request(request)
		with data.tobari.rules.learned_allows as [retired_prefix_rule]
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
	result := decision with input as object.union(base_input, {"authorization": {"requested_profile": "github-development", "broker_provider": null}})
	result.allow
}

test_deny_credential_on_other_host if {
	request := request_with_authority({"host": "example.com"})
	result := decision with input as object.union(input_with_request(request), {"authorization": {"requested_profile": "github-development", "broker_provider": null}})
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
