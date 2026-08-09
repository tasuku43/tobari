package tobari.http

import rego.v1

default decision := {
	"allow": false,
	"reason": "request did not match an allow rule",
	"credential_profile": null,
	"status_code": 403,
	"learnable": false,
}

decision := {
	"allow": false,
	"reason": "request did not match an allow rule",
	"credential_profile": requested_profile,
	"status_code": 403,
	"learnable": true,
} if {
	candidate_eligible
	not request_allowed
}

decision := {
	"allow": true,
	"reason": "allowed by policy",
	"credential_profile": requested_profile,
	"status_code": 403,
	"learnable": false,
} if {
	candidate_eligible
	request_allowed
}

requested_profile := input.authorization.requested_profile
broker_provider := input.authorization.broker_provider

candidate_eligible if {
	input.schema_version == 4
	data.tobari.schema_version == 2
	input.principal.cluster == "default"
	project_principal_valid
	transport_port_allowed
	candidate_scheme_allowed
	graphql_endpoints_valid
	request_protocol_valid
	authorization_shape_valid
	credential_binding_valid
	not explicitly_denied
}

request_allowed if {
	ordinary_http_request
	broker_provider == null
	authority_allowed
	method_allowed
}

request_allowed if {
	ordinary_http_request
	http_learned_rule_allowed
}

request_allowed if {
	graphql_request
	every root_field in request_graphql.root_fields {
		graphql_learned_rule_allowed(root_field)
	}
}

transport_port_allowed if {
	ports := data.tobari.boundary.ports[input.request.authority.scheme]
	input.request.authority.port in ports
}

candidate_scheme_allowed if {
	input.request.authority.scheme == "https"
}

candidate_scheme_allowed if {
	input.request.authority.scheme == "http"
	authority_allowed
}

candidate_scheme_allowed if {
	input.request.authority.scheme == "http"
	declared_graphql_endpoint
}

authority_allowed if {
	some authority in data.tobari.boundary.authorities
	authority.scheme == input.request.authority.scheme
	authority.host == input.request.authority.host
	input.request.authority.port in authority.ports
}

authorization_shape_valid if {
	object.keys(input.authorization) == {"broker_provider", "requested_profile"}
	requested_profile_shape_valid
	broker_provider_shape_valid
}

requested_profile_shape_valid if {
	requested_profile == null
}

requested_profile_shape_valid if {
	is_string(requested_profile)
	requested_profile != ""
}

broker_provider_shape_valid if {
	broker_provider == null
}

broker_provider_shape_valid if {
	is_string(broker_provider)
	count(broker_provider) <= 64
	regex.match(`^[a-z0-9]+([._-][a-z0-9]+)*$`, broker_provider)
}

method_allowed if {
	input.request.method in data.tobari.boundary.methods.read
}

method_allowed if {
	some rule in data.tobari.boundary.methods.write
	rule.method == input.request.method
	not write_path_excluded(rule)
}

write_path_excluded(rule) if {
	some prefix in rule.exclude_path_prefixes
	startswith(input.request.path.raw, prefix)
}

explicitly_denied if {
	some rule in data.tobari.rules.baseline_denies
	rule.host == input.request.authority.host
	rule.method == input.request.method
	startswith(input.request.path.raw, rule.path_prefix)
}

explicitly_denied if {
	some rule in learned_deny_rules
	ordinary_http_request
	http_rule_protocol_valid(rule)
	rule.context_id == input.principal.context_id
	rule.project_id == input.principal.project_id
	rule.host == input.request.authority.host
	rule.port == input.request.authority.port
	rule.method == input.request.method
	rule.path == input.request.path.raw
}

explicitly_denied if {
	graphql_request
	some root_field in request_graphql.root_fields
	some rule in learned_deny_rules
	graphql_rule_protocol_valid(rule)
	rule.context_id == input.principal.context_id
	rule.project_id == input.principal.project_id
	rule.host == input.request.authority.host
	rule.port == input.request.authority.port
	rule.method == input.request.method
	rule.path == input.request.path.raw
	rule.graphql_operation_type == request_graphql.operation_type
	rule.graphql_root_field == root_field
}

default learned_rules := []

learned_rules := data.tobari.rules.learned_allows

default learned_deny_rules := []

learned_deny_rules := data.tobari.rules.learned_denies

http_learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	http_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

graphql_learned_rule_allowed(root_field) if {
	some rule in learned_rules
	learned_rule_valid(rule)
	graphql_rule_protocol_valid(rule)
	rule.graphql_operation_type == request_graphql.operation_type
	rule.graphql_root_field == root_field
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_rule_matches_request(rule, project_id, request) if {
	rule.match == "exact"
	rule.context_id == input.principal.context_id
	rule.project_id == project_id
	rule.host == request.authority.host
	rule.port == request.authority.port
	rule.method == request.method
	rule.path == request.path.raw
}

learned_rule_matches_request(rule, project_id, request) if {
	rule.match == "prefix"
	rule.context_id == input.principal.context_id
	rule.project_id == project_id
	rule.host == request.authority.host
	rule.port == request.authority.port
	rule.method == request.method
	prefix_request_path_safe(request.path.raw)
	startswith(request.path.raw, rule.path)
}

prefix_request_path_safe(path) if {
	is_string(path)
	startswith(path, "/")
	not contains(path, "%")
	not contains(path, "\\")
	every segment in split(trim(path, "/"), "/") {
		segment != ""
		segment != "."
		segment != ".."
	}
}

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	http_rule_protocol_valid(rule)
	learned_rule_scope_valid(rule)
}

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	graphql_rule_protocol_valid(rule)
	rule.match == "exact"
	learned_rule_scope_valid(rule)
}

learned_rule_base_valid(rule) if {
	is_string(rule.id)
	regex.match("^plr_[0-9a-f]{32}$", rule.id)
	is_string(rule.project_id)
	regex.match("^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", rule.project_id)
	is_string(rule.context_id)
	regex.match("^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", rule.context_id)
	is_string(rule.host)
	rule.host != ""
	is_number(rule.port)
	rule.port >= 1
	rule.port <= 65535
	is_string(rule.method)
	regex.match("^[A-Z][A-Z0-9!#$%&'*+.^_`|~-]{0,31}$", rule.method)
	is_string(rule.path)
	startswith(rule.path, "/")
	is_array(rule.examples)
	is_array(rule.source_candidates)
	every source in rule.source_candidates {
		is_string(source)
		regex.match("^pcy_[0-9a-f]{32}$", source)
	}
}

project_principal_valid if {
	is_string(input.principal.project_id)
	regex.match("^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", input.principal.project_id)
}

learned_rule_scope_valid(rule) if {
	rule.match == "exact"
	count(rule.examples) >= 1
	count(rule.source_candidates) >= 1
	every example in rule.examples {
		example == rule.path
	}
}

learned_rule_scope_valid(rule) if {
	rule.match == "prefix"
	endswith(rule.path, "/")
	count(split(trim(rule.path, "/"), "/")) >= 2
	count(rule.examples) >= 3
	count(rule.source_candidates) >= 3
	prefix_request_path_safe(rule.path)
	every example in rule.examples {
		is_string(example)
		prefix_request_path_safe(example)
		startswith(example, rule.path)
	}
}

credential_binding_valid if {
	requested_profile == null
}

credential_binding_valid if {
	profile := requested_profile
	profile != null
	input.request.authority.host in data.tobari.credentials[profile].hosts
}

request_graphql := object.get(input.request, "graphql", null)

ordinary_http_request if {
	request_graphql == null
	not declared_graphql_endpoint
}

graphql_request if {
	declared_graphql_endpoint
	input.request.method == "POST"
	graphql_request_shape_valid
}

request_protocol_valid if {
	ordinary_http_request
}

request_protocol_valid if {
	graphql_request
}

default graphql_endpoints := []

graphql_endpoints := data.tobari.boundary.graphql_endpoints

declared_graphql_endpoint if {
	graphql_endpoints_valid
	some endpoint in graphql_endpoints
	endpoint.scheme == input.request.authority.scheme
	endpoint.host == input.request.authority.host
	endpoint.port == input.request.authority.port
	endpoint.path == input.request.path.raw
}

graphql_endpoints_valid if {
	is_array(graphql_endpoints)
	count(graphql_endpoints) == count({endpoint | some endpoint in graphql_endpoints})
	every endpoint in graphql_endpoints {
		graphql_endpoint_valid(endpoint)
	}
}

graphql_endpoint_valid(endpoint) if {
	is_object(endpoint)
	object.keys(endpoint) == {"host", "path", "port", "scheme"}
	endpoint.scheme in {"http", "https"}
	is_string(endpoint.host)
	endpoint.host != ""
	is_number(endpoint.port)
	endpoint.port >= 1
	endpoint.port <= 65535
	is_string(endpoint.path)
	startswith(endpoint.path, "/")
}

graphql_request_shape_valid if {
	is_object(request_graphql)
	object.keys(request_graphql) == {"operation_type", "root_fields"}
	request_graphql.operation_type in {"query", "mutation"}
	is_array(request_graphql.root_fields)
	count(request_graphql.root_fields) >= 1
	request_graphql.root_fields == sort(request_graphql.root_fields)
	count(request_graphql.root_fields) == count({root_field | some root_field in request_graphql.root_fields})
	every root_field in request_graphql.root_fields {
		graphql_name_valid(root_field)
	}
}

graphql_name_valid(value) if {
	is_string(value)
	count(value) <= 256
	regex.match(`^[_A-Za-z][_0-9A-Za-z]*$`, value)
}

http_rule_protocol_valid(rule) if {
	object.get(rule, "protocol", "http") == "http"
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
}

graphql_rule_protocol_valid(rule) if {
	rule.protocol == "graphql"
	rule.graphql_operation_type in {"query", "mutation"}
	graphql_name_valid(rule.graphql_root_field)
}

object_has_key(obj, key) if {
	key in object.keys(obj)
}
