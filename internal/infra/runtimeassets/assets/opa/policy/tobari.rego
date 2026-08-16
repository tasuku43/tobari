package tobari.http

import rego.v1

default decision := {
	"allow": false,
	"reason": "request did not match an allow rule",
	"status_code": 403,
	"learnable": false,
}

decision := {
	"allow": false,
	"reason": "request did not match an allow rule",
	"status_code": 403,
	"learnable": true,
} if {
	candidate_eligible
	not request_allowed
}

decision := {
	"allow": true,
	"reason": "allowed by policy",
	"status_code": 403,
	"learnable": false,
} if {
	candidate_eligible
	request_allowed
}

broker_provider := input.authorization.broker_provider

candidate_eligible if {
	input.schema_version == 1
	data.tobari.schema_version == 1
	input.principal.cluster == "default"
	project_principal_valid
	transport_port_allowed
	graphql_endpoints_valid
	mcp_endpoints_valid
	request_protocol_valid
	authorization_shape_valid
	not explicitly_denied
}

request_allowed if {
	ordinary_http_request
	http_learned_rule_allowed
}

request_allowed if {
	mcp_request
	mcp_learned_rule_allowed
}

request_allowed if {
	graphql_request
	every root_field in request_graphql.root_fields {
		graphql_learned_rule_allowed(root_field)
	}
}

transport_port_allowed if {
	input.request.authority.scheme in {"http", "https"}
	is_number(input.request.authority.port)
	not is_boolean(input.request.authority.port)
	input.request.authority.port >= 1
	input.request.authority.port <= 65535
}

authorization_shape_valid if {
	object.keys(input.authorization) == {"broker_provider"}
	broker_provider_shape_valid
}

broker_provider_shape_valid if {
	broker_provider == null
}

broker_provider_shape_valid if {
	is_string(broker_provider)
	count(broker_provider) <= 64
	regex.match(`^[a-z0-9]+([._-][a-z0-9]+)*$`, broker_provider)
}

explicitly_denied if {
	some rule in learned_deny_rules
	ordinary_http_request
	http_rule_protocol_valid(rule)
	rule.context_id == input.principal.context_id
	rule.project_id == input.principal.project_id
	rule.scheme == input.request.authority.scheme
	rule.host == input.request.authority.host
	rule.port == input.request.authority.port
	rule.method == input.request.method
	rule.path == input.request.path.raw
}

explicitly_denied if {
	mcp_request
	some rule in learned_deny_rules
	mcp_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.mcp_method == request_mcp.method
	object.get(rule, "mcp_tool_name", "") == object.get(request_mcp, "tool_name", "")
}

explicitly_denied if {
	graphql_request
	some root_field in request_graphql.root_fields
	some rule in learned_deny_rules
	graphql_rule_protocol_valid(rule)
	rule.context_id == input.principal.context_id
	rule.project_id == input.principal.project_id
	rule.scheme == input.request.authority.scheme
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

mcp_learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	mcp_rule_protocol_valid(rule)
	rule.mcp_method == request_mcp.method
	object.get(rule, "mcp_tool_name", "") == object.get(request_mcp, "tool_name", "")
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_rule_matches_request(rule, project_id, request) if {
	rule.match == "exact"
	rule.context_id == input.principal.context_id
	rule.project_id == project_id
	rule.scheme == request.authority.scheme
	rule.host == request.authority.host
	rule.port == request.authority.port
	rule.method == request.method
	rule.path == request.path.raw
}

learned_rule_matches_request(rule, project_id, request) if {
	rule.match == "path_template"
	rule.context_id == input.principal.context_id
	rule.project_id == project_id
	rule.scheme == request.authority.scheme
	rule.host == request.authority.host
	rule.port == request.authority.port
	rule.method == request.method
	path_template_matches(rule.segments, request.path.raw)
}

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	http_rule_protocol_valid(rule)
	learned_rule_scope_valid(rule)
}

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	mcp_rule_protocol_valid(rule)
	rule.match == "exact"
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
	rule.scheme in {"http", "https"}
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
	object.get(rule, "segments", []) == []
	count(rule.examples) >= 1
	count(rule.source_candidates) >= 1
	every example in rule.examples {
		example == rule.path
	}
}

learned_rule_scope_valid(rule) if {
	rule.match == "path_template"
	is_array(rule.segments)
	count(rule.segments) >= 2
	rule.path == sprintf("/%s", [concat("/", rule.segments)])
	count([segment | some segment in rule.segments; segment == "{id}"]) == 1
	count(rule.examples) >= 2
	count(rule.source_candidates) >= 2
	every segment in rule.segments {
		path_template_rule_segment_valid(segment)
	}
	every example in rule.examples {
		path_template_matches(rule.segments, example)
	}
}

path_template_rule_segment_valid(segment) if {
	is_string(segment)
	segment == "{id}"
}

path_template_rule_segment_valid(segment) if {
	is_string(segment)
	segment != ""
	segment != "."
	segment != ".."
	segment != "{id}"
	not contains(segment, "/")
	not contains(segment, "\\")
	not contains(segment, "%")
}

path_template_request_segment_valid(segment) if {
	is_string(segment)
	segment != ""
	segment != "."
	segment != ".."
	not contains(segment, "\\")
	not contains(segment, "%")
}

path_template_segment_matches(template, actual) if {
	template == "{id}"
	path_template_request_segment_valid(actual)
}

path_template_segment_matches(template, actual) if {
	template != "{id}"
	template == actual
}

path_template_matches(template_segments, raw_path) if {
	is_string(raw_path)
	startswith(raw_path, "/")
	parts := split(raw_path, "/")
	actual_segments := array.slice(parts, 1, count(parts))
	count(actual_segments) == count(template_segments)
	every index, template in template_segments {
		path_template_segment_matches(template, actual_segments[index])
	}
}

request_graphql := object.get(input.request, "graphql", null)
request_mcp := object.get(input.request, "mcp", null)

ordinary_http_request if {
	request_graphql == null
	request_mcp == null
	not declared_graphql_endpoint
	not declared_mcp_endpoint
}

graphql_request if {
	declared_graphql_endpoint
	input.request.method == "POST"
	graphql_request_shape_valid
}

mcp_request if {
	declared_mcp_endpoint
	input.request.method == "POST"
	mcp_request_shape_valid
}

request_protocol_valid if {
	ordinary_http_request
}

request_protocol_valid if {
	graphql_request
}

request_protocol_valid if {
	mcp_request
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

default mcp_endpoints := []

mcp_endpoints := data.tobari.boundary.mcp_endpoints

declared_mcp_endpoint if {
	mcp_endpoints_valid
	some endpoint in mcp_endpoints
	endpoint.scheme == input.request.authority.scheme
	endpoint.host == input.request.authority.host
	endpoint.port == input.request.authority.port
	endpoint.path == input.request.path.raw
}

mcp_endpoints_valid if {
	is_array(mcp_endpoints)
	count(mcp_endpoints) == count({endpoint | some endpoint in mcp_endpoints})
	every endpoint in mcp_endpoints {
		graphql_endpoint_valid(endpoint)
	}
}

mcp_request_shape_valid if {
	is_object(request_mcp)
	object.keys(request_mcp) == {"method"}
	mcp_method_valid(request_mcp.method)
	request_mcp.method != "tools/call"
}

mcp_request_shape_valid if {
	is_object(request_mcp)
	object.keys(request_mcp) == {"method", "tool_name"}
	request_mcp.method == "tools/call"
	mcp_tool_name_valid(request_mcp.tool_name)
}

mcp_method_valid(value) if {
	is_string(value)
	count(value) <= 128
	regex.match(`^[A-Za-z0-9_.-]+(/[A-Za-z0-9_.-]+)*$`, value)
}

mcp_tool_name_valid(value) if {
	is_string(value)
	count(value) <= 256
	regex.match(`^[A-Za-z0-9_.:/-]+$`, value)
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
	rule.protocol == "http"
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
}

graphql_rule_protocol_valid(rule) if {
	rule.protocol == "graphql"
	rule.graphql_operation_type in {"query", "mutation"}
	graphql_name_valid(rule.graphql_root_field)
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
}

mcp_rule_protocol_valid(rule) if {
	rule.protocol == "mcp"
	mcp_method_valid(rule.mcp_method)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	rule.mcp_method != "tools/call"
	not object_has_key(rule, "mcp_tool_name")
}

mcp_rule_protocol_valid(rule) if {
	rule.protocol == "mcp"
	rule.mcp_method == "tools/call"
	mcp_tool_name_valid(rule.mcp_tool_name)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
}

object_has_key(obj, key) if {
	key in object.keys(obj)
}
