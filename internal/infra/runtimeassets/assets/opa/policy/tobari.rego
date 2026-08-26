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

# decision_evidence is an internal evaluator result. Gateway authorization
# continues to consume decision's exact four-field contract; this companion
# document keeps future explanations tied to the same fixed evaluation without
# retaining request bodies, headers, query values, or credentials.
decision_evidence := {
	"decision": {
		"allow": false,
		"reason": "request did not match an allow rule",
		"status_code": 403,
		"learnable": false,
	},
	"policy_layer": "default_posture",
	"rule_refs": [],
	"semantic_effect": semantic_effect,
	"default_overridden": false,
} if {
	not explicitly_denied
	not candidate_eligible
}

decision_evidence := {
	"decision": decision,
	"policy_layer": "learned_deny",
	"rule_refs": matching_learned_deny_rule_refs,
	"semantic_effect": semantic_effect,
	"default_overridden": true,
} if {
	explicitly_denied
}

decision_evidence := {
	"decision": decision,
	"policy_layer": "learned_allow",
	"rule_refs": matching_learned_allow_rule_refs,
	"semantic_effect": semantic_effect,
	"default_overridden": true,
} if {
	candidate_eligible
	request_allowed
}

decision_evidence := {
	"decision": decision,
	"policy_layer": "default_posture",
	"rule_refs": [],
	"semantic_effect": semantic_effect,
	"default_overridden": false,
} if {
	candidate_eligible
	not request_allowed
}

matching_learned_allow_rule_refs := sort({rule.id |
	some rule in learned_rules
	learned_rule_valid(rule)
	learned_allow_rule_matches_request(rule)
})

matching_learned_deny_rule_refs := sort({rule.id |
	some rule in learned_deny_rules
	is_string(object.get(rule, "id", null))
	learned_deny_rule_matches_request(rule)
})

learned_allow_rule_matches_request(rule) if {
	ordinary_http_request
	http_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_allow_rule_matches_request(rule) if {
	mcp_request
	mcp_rule_protocol_valid(rule)
	rule.mcp_method == request_mcp.method
	object.get(rule, "mcp_tool_name", "") == object.get(request_mcp, "tool_name", "")
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_allow_rule_matches_request(rule) if {
	graphql_request
	some root_field in request_graphql.root_fields
	graphql_rule_protocol_valid(rule)
	rule.graphql_operation_type == request_graphql.operation_type
	rule.graphql_root_field == root_field
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_allow_rule_matches_request(rule) if {
	aws_request
	aws_rule_protocol_valid(rule)
	rule.aws_wire_protocol == request_aws.wire_protocol
	rule.aws_service == request_aws.service
	rule.aws_operation == request_aws.operation
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_allow_rule_matches_request(rule) if {
	kubernetes_request
	kubernetes_rule_protocol_valid(rule)
	rule.kubernetes_verb == request_kubernetes.verb
	rule.kubernetes_resource == request_kubernetes.resource
	rule.kubernetes_dry_run == request_kubernetes.dry_run
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_allow_rule_matches_request(rule) if {
	git_request
	git_rule_protocol_valid(rule)
	rule.git_service == request_git.service
	rule.git_repository == request_git.repository
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_allow_rule_matches_request(rule) if {
	oci_request
	oci_rule_protocol_valid(rule)
	rule.oci_action == request_oci.action
	rule.oci_repository == request_oci.repository
	rule.oci_object == request_oci.object
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_deny_rule_matches_request(rule) if {
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

learned_deny_rule_matches_request(rule) if {
	mcp_request
	mcp_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.mcp_method == request_mcp.method
	object.get(rule, "mcp_tool_name", "") == object.get(request_mcp, "tool_name", "")
}

learned_deny_rule_matches_request(rule) if {
	graphql_request
	some root_field in request_graphql.root_fields
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

learned_deny_rule_matches_request(rule) if {
	aws_request
	aws_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.aws_wire_protocol == request_aws.wire_protocol
	rule.aws_service == request_aws.service
	rule.aws_operation == request_aws.operation
}

learned_deny_rule_matches_request(rule) if {
	kubernetes_request
	kubernetes_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.kubernetes_verb == request_kubernetes.verb
	rule.kubernetes_resource == request_kubernetes.resource
	rule.kubernetes_dry_run == request_kubernetes.dry_run
}

learned_deny_rule_matches_request(rule) if {
	git_request
	git_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.git_service == request_git.service
	rule.git_repository == request_git.repository
}

learned_deny_rule_matches_request(rule) if {
	oci_request
	oci_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.oci_action == request_oci.action
	rule.oci_repository == request_oci.repository
	rule.oci_object == request_oci.object
}

request_protocol := "graphql" if {
	request_graphql != null
} else := "mcp" if {
	request_mcp != null
} else := "aws" if {
	request_aws != null
} else := "kubernetes" if {
	request_kubernetes != null
} else := "git" if {
	request_git != null
} else := "oci" if {
	request_oci != null
} else := "http"

default protocol_coordinates := {}

protocol_coordinates := {
	"operation_type": request_graphql.operation_type,
	"root_fields": sort(request_graphql.root_fields),
} if {
	request_protocol == "graphql"
	is_object(request_graphql)
	is_string(object.get(request_graphql, "operation_type", null))
	is_array(object.get(request_graphql, "root_fields", null))
}

protocol_coordinates := {
	"method": request_mcp.method,
	"tool_name": object.get(request_mcp, "tool_name", ""),
} if {
	request_protocol == "mcp"
	is_object(request_mcp)
	is_string(object.get(request_mcp, "method", null))
}

protocol_coordinates := {
	"wire_protocol": request_aws.wire_protocol,
	"service": request_aws.service,
	"operation": request_aws.operation,
} if {
	request_protocol == "aws"
	is_object(request_aws)
	is_string(object.get(request_aws, "wire_protocol", null))
	is_string(object.get(request_aws, "service", null))
	is_string(object.get(request_aws, "operation", null))
}

protocol_coordinates := {
	"verb": request_kubernetes.verb,
	"resource": request_kubernetes.resource,
	"dry_run": request_kubernetes.dry_run,
} if {
	request_protocol == "kubernetes"
	is_object(request_kubernetes)
	is_string(object.get(request_kubernetes, "verb", null))
	is_string(object.get(request_kubernetes, "resource", null))
	is_string(object.get(request_kubernetes, "dry_run", null))
}

protocol_coordinates := {
	"service": request_git.service,
	"repository": request_git.repository,
} if {
	request_protocol == "git"
	is_object(request_git)
	is_string(object.get(request_git, "service", null))
	is_string(object.get(request_git, "repository", null))
}

protocol_coordinates := {
	"action": request_oci.action,
	"repository": request_oci.repository,
	"object": request_oci.object,
} if {
	request_protocol == "oci"
	is_object(request_oci)
	is_string(object.get(request_oci, "action", null))
	is_string(object.get(request_oci, "repository", null))
	is_string(object.get(request_oci, "object", null))
}

semantic_effect := {
	"protocol": request_protocol,
	"scheme": object.get(object.get(object.get(input, "request", {}), "authority", {}), "scheme", ""),
	"host": object.get(object.get(object.get(input, "request", {}), "authority", {}), "host", ""),
	"port": object.get(object.get(object.get(input, "request", {}), "authority", {}), "port", 0),
	"method": object.get(object.get(input, "request", {}), "method", ""),
	"path": object.get(object.get(object.get(input, "request", {}), "path", {}), "raw", ""),
	"coordinates": protocol_coordinates,
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
	kubernetes_endpoints_valid
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
	aws_request
	aws_learned_rule_allowed
}

request_allowed if {
	kubernetes_request
	kubernetes_learned_rule_allowed
}

request_allowed if {
	git_request
	git_learned_rule_allowed
}

request_allowed if {
	oci_request
	oci_learned_rule_allowed
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

explicitly_denied if {
	aws_request
	some rule in learned_deny_rules
	aws_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.aws_wire_protocol == request_aws.wire_protocol
	rule.aws_service == request_aws.service
	rule.aws_operation == request_aws.operation
}

explicitly_denied if {
	kubernetes_request
	some rule in learned_deny_rules
	kubernetes_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.kubernetes_verb == request_kubernetes.verb
	rule.kubernetes_resource == request_kubernetes.resource
	rule.kubernetes_dry_run == request_kubernetes.dry_run
}

explicitly_denied if {
	git_request
	some rule in learned_deny_rules
	git_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.git_service == request_git.service
	rule.git_repository == request_git.repository
}

explicitly_denied if {
	oci_request
	some rule in learned_deny_rules
	oci_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	rule.oci_action == request_oci.action
	rule.oci_repository == request_oci.repository
	rule.oci_object == request_oci.object
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

aws_learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	aws_rule_protocol_valid(rule)
	rule.aws_wire_protocol == request_aws.wire_protocol
	rule.aws_service == request_aws.service
	rule.aws_operation == request_aws.operation
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

kubernetes_learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	kubernetes_rule_protocol_valid(rule)
	rule.kubernetes_verb == request_kubernetes.verb
	rule.kubernetes_resource == request_kubernetes.resource
	rule.kubernetes_dry_run == request_kubernetes.dry_run
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

git_learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	git_rule_protocol_valid(rule)
	rule.git_service == request_git.service
	rule.git_repository == request_git.repository
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

oci_learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	oci_rule_protocol_valid(rule)
	rule.oci_action == request_oci.action
	rule.oci_repository == request_oci.repository
	rule.oci_object == request_oci.object
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

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	aws_rule_protocol_valid(rule)
	rule.match == "exact"
	learned_rule_scope_valid(rule)
}

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	kubernetes_rule_protocol_valid(rule)
	rule.match == "exact"
	learned_rule_scope_valid(rule)
}

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	git_rule_protocol_valid(rule)
	rule.match == "exact"
	learned_rule_scope_valid(rule)
}

learned_rule_valid(rule) if {
	learned_rule_base_valid(rule)
	oci_rule_protocol_valid(rule)
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
request_aws := object.get(input.request, "aws", null)
request_kubernetes := object.get(input.request, "kubernetes", null)
request_git := object.get(input.request, "git", null)
request_oci := object.get(input.request, "oci", null)

ordinary_http_request if {
	request_graphql == null
	request_mcp == null
	request_aws == null
	request_kubernetes == null
	request_git == null
	request_oci == null
	not declared_graphql_endpoint
	not declared_mcp_endpoint
	not declared_kubernetes_endpoint
}

graphql_request if {
	declared_graphql_endpoint
	input.request.method in {"GET", "POST"}
	graphql_request_shape_valid
}

mcp_request if {
	declared_mcp_endpoint
	input.request.method == "POST"
	mcp_request_shape_valid
}

aws_request if {
	not declared_graphql_endpoint
	not declared_mcp_endpoint
	not declared_kubernetes_endpoint
	request_git == null
	request_oci == null
	input.request.method == "POST"
	input.request.path.raw == "/"
	aws_request_shape_valid
}

kubernetes_request if {
	declared_kubernetes_endpoint
	kubernetes_request_shape_valid
}

git_request if {
	not declared_graphql_endpoint
	not declared_mcp_endpoint
	not declared_kubernetes_endpoint
	git_request_shape_valid
	input.request.method == "GET"
	input.request.path.raw == sprintf("%s/info/refs", [request_git.repository])
	input.request.query == {"service": [sprintf("git-%s", [request_git.service])]}
}

oci_request if {
	not declared_graphql_endpoint
	not declared_mcp_endpoint
	not declared_kubernetes_endpoint
	request_git == null
	oci_request_shape_valid
	oci_request_transport_valid
	input.request.query == {}
}

git_request if {
	not declared_graphql_endpoint
	not declared_mcp_endpoint
	not declared_kubernetes_endpoint
	git_request_shape_valid
	input.request.method == "POST"
	input.request.path.raw == sprintf("%s/git-%s", [request_git.repository, request_git.service])
	input.request.query == {}
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

request_protocol_valid if {
	aws_request
}

request_protocol_valid if {
	kubernetes_request
}

request_protocol_valid if {
	git_request
}

request_protocol_valid if {
	oci_request
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

default kubernetes_endpoints := []

kubernetes_endpoints := data.tobari.boundary.kubernetes_endpoints

declared_kubernetes_endpoint if {
	kubernetes_endpoints_valid
	some endpoint in kubernetes_endpoints
	endpoint.scheme == input.request.authority.scheme
	endpoint.host == input.request.authority.host
	endpoint.port == input.request.authority.port
	endpoint.path == "/"
}

kubernetes_endpoints_valid if {
	is_array(kubernetes_endpoints)
	count(kubernetes_endpoints) == count({endpoint | some endpoint in kubernetes_endpoints})
	every endpoint in kubernetes_endpoints {
		graphql_endpoint_valid(endpoint)
		endpoint.scheme == "https"
		endpoint.port == 443
		endpoint.path == "/"
	}
}

kubernetes_request_shape_valid if {
	is_object(request_kubernetes)
	object.keys(request_kubernetes) == {"dry_run", "resource", "verb"}
	request_kubernetes.verb in {"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "connect"}
	request_kubernetes.dry_run in {"none", "empty", "all"}
	is_string(request_kubernetes.resource)
	count(request_kubernetes.resource) >= 1
	count(request_kubernetes.resource) <= 1024
	not regex.match(`[\x00-\x1f\x7f]`, request_kubernetes.resource)
}

git_request_shape_valid if {
	is_object(request_git)
	object.keys(request_git) == {"repository", "service"}
	request_git.service in {"upload-pack", "receive-pack"}
	git_repository_valid(request_git.repository)
}

git_repository_valid(repository) if {
	is_string(repository)
	count(repository) >= 2
	count(repository) <= 1024
	startswith(repository, "/")
	not contains(repository, "//")
	not contains(repository, "\\")
	not contains(repository, "%")
	not regex.match(`[\x00-\x1f\x7f]`, repository)
	parts := array.slice(split(repository, "/"), 1, count(split(repository, "/")))
	every part in parts {
		part != ""
		part != "."
		part != ".."
	}
}

oci_request_shape_valid if {
	is_object(request_oci)
	object.keys(request_oci) == {"action", "object", "repository"}
	request_oci.action in {"list", "pull", "push", "delete", "start_upload", "upload_status", "upload_chunk", "complete_upload", "mount", "cancel_upload"}
	protocol_coordinate_valid(request_oci.repository, true)
	protocol_coordinate_valid(request_oci.object, false)
	oci_action_coordinate_valid(request_oci.action, request_oci.repository, request_oci.object)
}

oci_action_coordinate_valid("list", "", "catalog") := true

oci_action_coordinate_valid("list", repository, "tags") if {
	repository != ""
}

oci_action_coordinate_valid(action, repository, object) if {
	action == "pull"
	repository != ""
	some prefix in {"manifest:", "blob:", "referrers:"}
	startswith(object, prefix)
	trim_prefix(object, prefix) != ""
}

oci_action_coordinate_valid(action, repository, object) if {
	action == "push"
	repository != ""
	startswith(object, "manifest:")
	trim_prefix(object, "manifest:") != ""
}

oci_action_coordinate_valid(action, repository, object) if {
	action == "delete"
	repository != ""
	some prefix in {"manifest:", "blob:"}
	startswith(object, prefix)
	trim_prefix(object, prefix) != ""
}

oci_action_coordinate_valid("start_upload", repository, "upload") if {
	repository != ""
}

oci_action_coordinate_valid(action, repository, object) if {
	action in {"upload_status", "upload_chunk", "cancel_upload"}
	repository != ""
	startswith(object, "upload:")
	trim_prefix(object, "upload:") != ""
}

oci_action_coordinate_valid(action, repository, object) if {
	action == "complete_upload"
	repository != ""
	startswith(object, "blob:")
	trim_prefix(object, "blob:") != ""
}

oci_action_coordinate_valid(action, repository, object) if {
	action == "mount"
	repository != ""
	startswith(object, "mount:")
	parts := split(trim_prefix(object, "mount:"), ":from:")
	count(parts) == 2
	parts[0] != ""
	parts[1] != ""
}

protocol_coordinate_valid(value, true) if {
	is_string(value)
	count(value) <= 1024
	not regex.match(`[\x00-\x1f\x7f]`, value)
}

protocol_coordinate_valid(value, false) if {
	is_string(value)
	count(value) >= 1
	count(value) <= 1024
	not regex.match(`[\x00-\x1f\x7f]`, value)
}

oci_action_method(action, method) if {
	[action, method] in {
		["pull", "GET"], ["pull", "HEAD"], ["push", "PUT"], ["delete", "DELETE"],
		["upload_status", "GET"], ["upload_chunk", "PATCH"], ["cancel_upload", "DELETE"],
	}
}

oci_request_transport_valid if {
	request_oci.action == "list"
	input.request.method == "GET"
	request_oci.repository == ""
	request_oci.object == "catalog"
	input.request.path.raw == "/v2/_catalog"
}

oci_request_transport_valid if {
	request_oci.action == "list"
	input.request.method == "GET"
	request_oci.repository != ""
	request_oci.object == "tags"
	input.request.path.raw == sprintf("/v2/%s/tags/list", [request_oci.repository])
}

oci_request_transport_valid if {
	request_oci.action in {"pull", "push", "delete"}
	oci_action_method(request_oci.action, input.request.method)
	startswith(request_oci.object, "manifest:")
	input.request.path.raw == sprintf("/v2/%s/manifests/%s", [request_oci.repository, trim_prefix(request_oci.object, "manifest:")])
}

oci_request_transport_valid if {
	request_oci.action in {"pull", "delete"}
	oci_action_method(request_oci.action, input.request.method)
	startswith(request_oci.object, "blob:")
	input.request.path.raw == sprintf("/v2/%s/blobs/%s", [request_oci.repository, trim_prefix(request_oci.object, "blob:")])
}

oci_request_transport_valid if {
	request_oci.action == "pull"
	input.request.method == "GET"
	startswith(request_oci.object, "referrers:")
	input.request.path.raw == sprintf("/v2/%s/referrers/%s", [request_oci.repository, trim_prefix(request_oci.object, "referrers:")])
}

oci_upload_collection_path if {
	input.request.path.raw in {
		sprintf("/v2/%s/blobs/uploads", [request_oci.repository]),
		sprintf("/v2/%s/blobs/uploads/", [request_oci.repository]),
	}
}

oci_request_transport_valid if {
	request_oci.action in {"start_upload", "mount"}
	input.request.method == "POST"
	oci_upload_collection_path
}

oci_request_transport_valid if {
	request_oci.action == "complete_upload"
	input.request.method == "POST"
	oci_upload_collection_path
	startswith(request_oci.object, "blob:")
}

oci_request_transport_valid if {
	request_oci.action in {"upload_status", "upload_chunk", "cancel_upload"}
	oci_action_method(request_oci.action, input.request.method)
	startswith(request_oci.object, "upload:")
	input.request.path.raw == sprintf("/v2/%s/blobs/uploads/%s", [request_oci.repository, trim_prefix(request_oci.object, "upload:")])
}

oci_request_transport_valid if {
	request_oci.action == "complete_upload"
	input.request.method == "PUT"
	startswith(request_oci.object, "blob:")
	startswith(input.request.path.raw, sprintf("/v2/%s/blobs/uploads/", [request_oci.repository]))
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

aws_request_shape_valid if {
	is_object(request_aws)
	object.keys(request_aws) == {"operation", "service", "wire_protocol"}
	request_aws.wire_protocol in {"query", "json"}
	aws_service_valid(request_aws.service)
	aws_operation_valid(request_aws.wire_protocol, request_aws.operation)
}

aws_service_valid(value) if {
	is_string(value)
	count(value) <= 63
	regex.match(`^[a-z0-9][a-z0-9-]{0,62}$`, value)
}

aws_operation_valid("query", value) if {
	is_string(value)
	count(value) <= 128
	regex.match(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`, value)
}

aws_operation_valid("json", value) if {
	is_string(value)
	count(value) <= 256
	regex.match(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*\.[A-Za-z_][A-Za-z0-9_]{0,127}$`, value)
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
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "kubernetes_verb")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

graphql_rule_protocol_valid(rule) if {
	rule.protocol == "graphql"
	rule.graphql_operation_type in {"query", "mutation"}
	graphql_name_valid(rule.graphql_root_field)
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "kubernetes_verb")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

mcp_rule_protocol_valid(rule) if {
	rule.protocol == "mcp"
	mcp_method_valid(rule.mcp_method)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	rule.mcp_method != "tools/call"
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "kubernetes_verb")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

mcp_rule_protocol_valid(rule) if {
	rule.protocol == "mcp"
	rule.mcp_method == "tools/call"
	mcp_tool_name_valid(rule.mcp_tool_name)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "kubernetes_verb")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

aws_rule_protocol_valid(rule) if {
	rule.protocol == "aws"
	rule.aws_wire_protocol in {"query", "json"}
	aws_service_valid(rule.aws_service)
	aws_operation_valid(rule.aws_wire_protocol, rule.aws_operation)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "kubernetes_verb")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

kubernetes_rule_protocol_valid(rule) if {
	rule.protocol == "kubernetes"
	rule.kubernetes_verb in {"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "connect"}
	rule.kubernetes_dry_run in {"none", "empty", "all"}
	is_string(rule.kubernetes_resource)
	count(rule.kubernetes_resource) >= 1
	count(rule.kubernetes_resource) <= 1024
	not regex.match(`[\x00-\x1f\x7f]`, rule.kubernetes_resource)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

git_rule_protocol_valid(rule) if {
	rule.protocol == "git"
	rule.git_service in {"upload-pack", "receive-pack"}
	git_repository_valid(rule.git_repository)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "kubernetes_verb")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

oci_rule_protocol_valid(rule) if {
	rule.protocol == "oci"
	rule.oci_action in {"list", "pull", "push", "delete", "start_upload", "upload_status", "upload_chunk", "complete_upload", "mount", "cancel_upload"}
	protocol_coordinate_valid(rule.oci_repository, true)
	protocol_coordinate_valid(rule.oci_object, false)
	oci_action_coordinate_valid(rule.oci_action, rule.oci_repository, rule.oci_object)
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "kubernetes_verb")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
}

object_has_key(obj, key) if {
	key in object.keys(obj)
}
