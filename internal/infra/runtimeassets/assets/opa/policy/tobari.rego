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
	"policy_layer": "static_deny",
	"rule_refs": matching_static_deny_rule_refs,
	"semantic_effect": semantic_effect,
	"default_overridden": true,
} if {
	static_semantic_denied
}

decision_evidence := {
	"decision": decision,
	"policy_layer": "learned_deny",
	"rule_refs": matching_learned_deny_rule_refs,
	"semantic_effect": semantic_effect,
	"default_overridden": true,
} if {
	not static_semantic_denied
	count(matching_learned_deny_rule_refs) > 0
}

decision_evidence := {
	"decision": decision,
	"policy_layer": "static_allow",
	"rule_refs": matching_static_allow_rule_refs,
	"semantic_effect": semantic_effect,
	"default_overridden": true,
} if {
	candidate_eligible
	static_semantic_allowed
}

decision_evidence := {
	"decision": decision,
	"policy_layer": "learned_allow",
	"rule_refs": matching_learned_allow_rule_refs,
	"semantic_effect": semantic_effect,
	"default_overridden": true,
} if {
	candidate_eligible
	not static_semantic_allowed
	count(matching_learned_allow_rule_refs) > 0
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

matching_static_allow_rule_refs := sort(static_allow_rule_ref_set)

matching_static_deny_rule_refs := sort(static_deny_rule_ref_set)

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
	aws_dynamic_coordinates_match(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_allow_rule_matches_request(rule) if {
	kubernetes_request
	kubernetes_rule_protocol_valid(rule)
	kubernetes_dynamic_coordinates_match(rule)
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
	aws_dynamic_coordinates_match(rule)
}

learned_deny_rule_matches_request(rule) if {
	kubernetes_request
	kubernetes_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	kubernetes_dynamic_coordinates_match(rule)
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
	graphql_request
} else := "mcp" if {
	mcp_request
} else := "aws" if {
	aws_request
} else := "kubernetes" if {
	kubernetes_request
} else := "git" if {
	git_request
} else := "oci" if {
	oci_request
} else := "http" if {
	ordinary_http_request
} else := "invalid"

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
	"protocol_version": object.get(request_aws, "protocol_version", ""),
	"target_namespace": object.get(request_aws, "target_namespace", ""),
	"operation": request_aws.operation,
} if {
	request_protocol == "aws"
	is_object(request_aws)
	is_string(object.get(request_aws, "wire_protocol", null))
	is_string(object.get(request_aws, "service", null))
	is_string(object.get(request_aws, "operation", null))
}

protocol_coordinates := {
	"kind": "resource",
	"verb": request_kubernetes.verb,
	"resource": request_kubernetes.resource,
	"dry_run": request_kubernetes.dry_run,
} if {
	request_protocol == "kubernetes"
	is_object(request_kubernetes)
	request_kubernetes.kind == "resource"
}

protocol_coordinates := {
	"kind": "non_resource",
	"verb": request_kubernetes.verb,
	"path": request_kubernetes.path,
} if {
	request_protocol == "kubernetes"
	is_object(request_kubernetes)
	request_kubernetes.kind == "non_resource"
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
	input.schema_version == 2
	data.tobari.schema_version == 2
	input.principal.cluster == "default"
	project_principal_valid
	transport_port_allowed
	graphql_endpoints_valid
	mcp_endpoints_valid
	kubernetes_endpoints_valid
	request_refinement_exclusive
	request_protocol_valid
	authorization_shape_valid
	destination_ceiling_allows
	method_ceiling_allows
	not explicitly_denied
}

semantic_request_keys := object.keys(input.request) & {"graphql", "mcp", "aws", "kubernetes", "git", "oci"}

request_refinement_exclusive if {
	count(semantic_request_keys) == 0
}

request_refinement_exclusive if {
	count(semantic_request_keys) == 1
	some name in semantic_request_keys
	object.get(input.request, name, null) != null
}

destination_ceiling_allows if {
	data.tobari.policy.destination_mode == "public_https"
	input.request.authority.scheme == "https"
}

destination_ceiling_allows if {
	data.tobari.policy.destination_mode == "exact"
	some authority in data.tobari.policy.authorities
	authority.scheme == input.request.authority.scheme
	authority.host == input.request.authority.host
	authority.port == input.request.authority.port
}

method_ceiling_allows if {
	decision := method_ceiling_decision
	decision != "deny"
}

method_ceiling_decision := override.decision if {
	some override in data.tobari.policy.method_overrides
	override.method == input.request.method
} else := data.tobari.policy.method_default

request_allowed if {
	ordinary_http_request
	http_learned_rule_allowed
}

request_allowed if {
	static_semantic_allowed
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
	static_semantic_denied
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
	aws_dynamic_coordinates_match(rule)
}

explicitly_denied if {
	kubernetes_request
	some rule in learned_deny_rules
	kubernetes_rule_protocol_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
	kubernetes_dynamic_coordinates_match(rule)
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

static_semantic := object.get(data.tobari.policy, "semantic", {})
static_http := object.get(object.get(object.get(static_semantic, "protocols", {}), "http", {}), "generic", {})
static_graphql := object.get(object.get(object.get(static_semantic, "protocols", {}), "http", {}), "graphql", {})
static_mcp := object.get(object.get(object.get(static_semantic, "protocols", {}), "http", {}), "mcp", {})
static_git := object.get(object.get(object.get(static_semantic, "protocols", {}), "http", {}), "git", {})
static_oci := object.get(object.get(object.get(static_semantic, "protocols", {}), "http", {}), "oci", {})
static_aws := object.get(object.get(static_semantic, "providers", {}), "aws", {})
static_kubernetes := object.get(object.get(static_semantic, "providers", {}), "kubernetes", {})

static_rules(module, effect) := object.get(object.get(module, effect, {}), "rules", [])

static_authority_matches(rule) if {
	rule.scheme == input.request.authority.scheme
	rule.port == input.request.authority.port
	object.get(rule, "host", "") == input.request.authority.host
}

static_authority_matches(rule) if {
	rule.scheme == input.request.authority.scheme
	rule.port == input.request.authority.port
	input.request.authority.host in object.get(rule, "hosts", [])
}

static_http_path_matches(rule) if {
	not contains(rule.path, "{id}")
	rule.path == input.request.path.raw
}

static_http_path_matches(rule) if {
	contains(rule.path, "{id}")
	parts := split(rule.path, "/")
	template := array.slice(parts, 1, count(parts))
	path_template_matches(template, input.request.path.raw)
}

static_http_rule_matches(rule) if {
	ordinary_http_request
	static_authority_matches(rule)
	rule.method == input.request.method
	static_http_path_matches(rule)
}

static_graphql_rule_matches(rule, root_field) if {
	graphql_request
	input.request.method == "POST"
	static_authority_matches(rule)
	rule.path == input.request.path.raw
	rule.operation_type == request_graphql.operation_type
	rule.root_field == root_field
}

static_mcp_rule_matches(rule) if {
	mcp_request
	static_authority_matches(rule)
	rule.path == input.request.path.raw
	rule.method == request_mcp.method
	object.get(rule, "tool_name", "") == object.get(request_mcp, "tool_name", "")
}

static_aws_operation_matches(rule) if {
	not endswith(rule.operation, "*")
	rule.operation == request_aws.operation
}

static_aws_operation_matches(rule) if {
	endswith(rule.operation, "*")
	prefix := trim_suffix(rule.operation, "*")
	startswith(request_aws.operation, prefix)
}

static_aws_service_matches(rule) if {
	object.get(rule, "service", "") == request_aws.service
}

static_aws_service_matches(rule) if {
	request_aws.service in object.get(rule, "services", [])
}

static_aws_rule_matches(rule) if {
	aws_request
	static_authority_matches(rule)
	rule.wire_protocol == request_aws.wire_protocol
	object.get(rule, "protocol_version", "") == object.get(request_aws, "protocol_version", "")
	object.get(rule, "target_namespace", "") == object.get(request_aws, "target_namespace", "")
	static_aws_service_matches(rule)
	static_aws_operation_matches(rule)
}

static_kubernetes_resource_matches(rule) if {
	kubernetes_request
	request_kubernetes.kind == "resource"
	resource := rule.resource
	resource.group == request_kubernetes.resource.group
	resource.version == request_kubernetes.resource.version
	resource.resource == request_kubernetes.resource.resource
	object.get(resource, "namespace", null) == request_kubernetes.resource.namespace
	object.get(resource, "name", null) == request_kubernetes.resource.name
	object.get(resource, "subresource", null) == request_kubernetes.resource.subresource
	resource.verb == request_kubernetes.verb
	resource.dry_run == request_kubernetes.dry_run
}

static_kubernetes_non_resource_matches(rule) if {
	kubernetes_request
	request_kubernetes.kind == "non_resource"
	non_resource := rule.non_resource
	non_resource.path == request_kubernetes.path
	non_resource.verb == request_kubernetes.verb
}

static_kubernetes_rule_matches(rule) if {
	static_authority_matches(rule)
	static_kubernetes_resource_matches(rule)
}

static_kubernetes_rule_matches(rule) if {
	static_authority_matches(rule)
	static_kubernetes_non_resource_matches(rule)
}

static_git_rule_matches(rule) if {
	git_request
	static_authority_matches(rule)
	rule.service == request_git.service
	rule.repository == request_git.repository
}

static_oci_rule_matches(rule) if {
	oci_request
	static_authority_matches(rule)
	rule.action == request_oci.action
	rule.repository == request_oci.repository
	rule.object == request_oci.object
}

static_allow_rule_ref_set contains sprintf("semantic:http.generic:allow:%d", [index]) if {
	some index, rule in static_rules(static_http, "allow")
	static_http_rule_matches(rule)
}

static_allow_rule_ref_set contains sprintf("semantic:http.graphql:allow:%d", [index]) if {
	some root_field in request_graphql.root_fields
	some index, rule in static_rules(static_graphql, "allow")
	static_graphql_rule_matches(rule, root_field)
}

static_allow_rule_ref_set contains sprintf("semantic:http.mcp:allow:%d", [index]) if {
	some index, rule in static_rules(static_mcp, "allow")
	static_mcp_rule_matches(rule)
}

static_allow_rule_ref_set contains sprintf("semantic:aws:allow:%d", [index]) if {
	some index, rule in static_rules(static_aws, "allow")
	static_aws_rule_matches(rule)
}

static_allow_rule_ref_set contains sprintf("semantic:kubernetes:allow:%d", [index]) if {
	some index, rule in static_rules(static_kubernetes, "allow")
	static_kubernetes_rule_matches(rule)
}

static_allow_rule_ref_set contains sprintf("semantic:http.git:allow:%d", [index]) if {
	some index, rule in static_rules(static_git, "allow")
	static_git_rule_matches(rule)
}

static_allow_rule_ref_set contains sprintf("semantic:http.oci:allow:%d", [index]) if {
	some index, rule in static_rules(static_oci, "allow")
	static_oci_rule_matches(rule)
}

static_deny_rule_ref_set contains sprintf("semantic:http.generic:deny:%d", [index]) if {
	some index, rule in static_rules(static_http, "deny")
	static_http_rule_matches(rule)
}

static_deny_rule_ref_set contains sprintf("semantic:http.graphql:deny:%d", [index]) if {
	some root_field in request_graphql.root_fields
	some index, rule in static_rules(static_graphql, "deny")
	static_graphql_rule_matches(rule, root_field)
}

static_deny_rule_ref_set contains sprintf("semantic:http.mcp:deny:%d", [index]) if {
	some index, rule in static_rules(static_mcp, "deny")
	static_mcp_rule_matches(rule)
}

static_deny_rule_ref_set contains sprintf("semantic:aws:deny:%d", [index]) if {
	some index, rule in static_rules(static_aws, "deny")
	static_aws_rule_matches(rule)
}

static_deny_rule_ref_set contains sprintf("semantic:kubernetes:deny:%d", [index]) if {
	some index, rule in static_rules(static_kubernetes, "deny")
	static_kubernetes_rule_matches(rule)
}

static_deny_rule_ref_set contains sprintf("semantic:http.git:deny:%d", [index]) if {
	some index, rule in static_rules(static_git, "deny")
	static_git_rule_matches(rule)
}

static_deny_rule_ref_set contains sprintf("semantic:http.oci:deny:%d", [index]) if {
	some index, rule in static_rules(static_oci, "deny")
	static_oci_rule_matches(rule)
}

static_semantic_denied if {
	some rule in static_rules(static_http, "deny")
	static_http_rule_matches(rule)
}

static_semantic_denied if {
	some root_field in request_graphql.root_fields
	some rule in static_rules(static_graphql, "deny")
	static_graphql_rule_matches(rule, root_field)
}

static_semantic_denied if {
	some rule in static_rules(static_mcp, "deny")
	static_mcp_rule_matches(rule)
}

static_semantic_denied if {
	some rule in static_rules(static_aws, "deny")
	static_aws_rule_matches(rule)
}

static_semantic_denied if {
	some rule in static_rules(static_kubernetes, "deny")
	static_kubernetes_rule_matches(rule)
}

static_semantic_denied if {
	some rule in static_rules(static_git, "deny")
	static_git_rule_matches(rule)
}

static_semantic_denied if {
	some rule in static_rules(static_oci, "deny")
	static_oci_rule_matches(rule)
}

static_semantic_allowed if {
	some rule in static_rules(static_http, "allow")
	static_http_rule_matches(rule)
}

static_semantic_allowed if {
	graphql_request
	every root_field in request_graphql.root_fields {
		some rule in static_rules(static_graphql, "allow")
		static_graphql_rule_matches(rule, root_field)
	}
}

static_semantic_allowed if {
	some rule in static_rules(static_mcp, "allow")
	static_mcp_rule_matches(rule)
}

static_semantic_allowed if {
	some rule in static_rules(static_aws, "allow")
	static_aws_rule_matches(rule)
}

static_semantic_allowed if {
	some rule in static_rules(static_kubernetes, "allow")
	static_kubernetes_rule_matches(rule)
}

static_semantic_allowed if {
	some rule in static_rules(static_git, "allow")
	static_git_rule_matches(rule)
}

static_semantic_allowed if {
	some rule in static_rules(static_oci, "allow")
	static_oci_rule_matches(rule)
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
	aws_dynamic_coordinates_match(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

kubernetes_learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	kubernetes_rule_protocol_valid(rule)
	kubernetes_dynamic_coordinates_match(rule)
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
	request_refinement_exclusive
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
	request_refinement_exclusive
	declared_graphql_endpoint
	input.request.method in {"GET", "POST"}
	graphql_request_shape_valid
}

mcp_request if {
	request_refinement_exclusive
	declared_mcp_endpoint
	input.request.method == "POST"
	mcp_request_shape_valid
}

aws_request if {
	request_refinement_exclusive
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
	request_refinement_exclusive
	declared_kubernetes_endpoint
	kubernetes_request_shape_valid
	kubernetes_request_transport_valid
}

git_request if {
	request_refinement_exclusive
	not declared_graphql_endpoint
	not declared_mcp_endpoint
	not declared_kubernetes_endpoint
	git_request_shape_valid
	input.request.method == "GET"
	input.request.path.raw == sprintf("%s/info/refs", [request_git.repository])
	input.request.query == {"service": [sprintf("git-%s", [request_git.service])]}
}

oci_request if {
	request_refinement_exclusive
	not declared_graphql_endpoint
	not declared_mcp_endpoint
	not declared_kubernetes_endpoint
	request_git == null
	oci_request_shape_valid
	oci_request_transport_valid
	input.request.query == {}
}

git_request if {
	request_refinement_exclusive
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
	object.keys(request_kubernetes) == {"dry_run", "kind", "resource", "verb"}
	request_kubernetes.kind == "resource"
	request_kubernetes.verb in {"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "connect"}
	request_kubernetes.dry_run in {"none", "empty", "all"}
	is_object(request_kubernetes.resource)
	object.keys(request_kubernetes.resource) == {"group", "name", "namespace", "resource", "subresource", "version"}
	is_string(request_kubernetes.resource.group)
	is_string(request_kubernetes.resource.version)
	request_kubernetes.resource.version != ""
	is_string(request_kubernetes.resource.resource)
	request_kubernetes.resource.resource != ""
	every field in {"name", "namespace", "subresource"} {
		nullable_string(object.get(request_kubernetes.resource, field, null))
	}
}

nullable_string(value) if {
	value == null
}

nullable_string(value) if {
	is_string(value)
}

kubernetes_request_method_valid if {
	request_kubernetes.kind == "resource"
	request_kubernetes.verb in {"get", "list", "watch"}
	input.request.method == "GET"
}

kubernetes_request_method_valid if {
	request_kubernetes.kind == "resource"
	request_kubernetes.verb == "create"
	input.request.method == "POST"
}

kubernetes_request_method_valid if {
	request_kubernetes.kind == "resource"
	request_kubernetes.verb == "update"
	input.request.method == "PUT"
}

kubernetes_request_method_valid if {
	request_kubernetes.kind == "resource"
	request_kubernetes.verb == "patch"
	input.request.method == "PATCH"
}

kubernetes_request_method_valid if {
	request_kubernetes.kind == "resource"
	request_kubernetes.verb in {"delete", "deletecollection"}
	input.request.method == "DELETE"
}

kubernetes_request_method_valid if {
	request_kubernetes.kind == "resource"
	request_kubernetes.verb == "connect"
	input.request.method in {"GET", "POST"}
}

kubernetes_resource_api_prefix := sprintf("/api/%s", [request_kubernetes.resource.version]) if {
	request_kubernetes.resource.group == ""
}

kubernetes_resource_api_prefix := sprintf("/apis/%s/%s", [request_kubernetes.resource.group, request_kubernetes.resource.version]) if {
	request_kubernetes.resource.group != ""
}

kubernetes_resource_collection_path := sprintf("%s/%s", [kubernetes_resource_api_prefix, request_kubernetes.resource.resource]) if {
	request_kubernetes.resource.namespace == null
}

kubernetes_resource_collection_path := sprintf("%s/namespaces/%s/%s", [kubernetes_resource_api_prefix, request_kubernetes.resource.namespace, request_kubernetes.resource.resource]) if {
	request_kubernetes.resource.namespace != null
}

kubernetes_resource_object_path := kubernetes_resource_collection_path if {
	request_kubernetes.resource.name == null
}

kubernetes_resource_object_path := sprintf("%s/%s", [kubernetes_resource_collection_path, request_kubernetes.resource.name]) if {
	request_kubernetes.resource.name != null
}

kubernetes_resource_path := kubernetes_resource_object_path if {
	request_kubernetes.resource.subresource == null
}

kubernetes_resource_path := sprintf("%s/%s", [kubernetes_resource_object_path, request_kubernetes.resource.subresource]) if {
	request_kubernetes.resource.subresource != null
}

kubernetes_request_transport_valid if {
	request_kubernetes.kind == "resource"
	kubernetes_request_method_valid
	input.request.path.raw == kubernetes_resource_path
}

kubernetes_request_transport_valid if {
	request_kubernetes.kind == "non_resource"
	input.request.method == "GET"
	input.request.path.raw == request_kubernetes.path
}

kubernetes_request_shape_valid if {
	is_object(request_kubernetes)
	object.keys(request_kubernetes) == {"kind", "path", "verb"}
	request_kubernetes.kind == "non_resource"
	request_kubernetes.verb == "get"
	is_string(request_kubernetes.path)
	startswith(request_kubernetes.path, "/")
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
	action == "complete_upload"
	repository != ""
	startswith(object, "upload:")
	parts := split(trim_prefix(object, "upload:"), ":blob:")
	count(parts) == 2
	parts[0] != ""
	parts[1] != ""
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
	startswith(request_oci.object, "upload:")
	parts := split(trim_prefix(request_oci.object, "upload:"), ":blob:")
	count(parts) == 2
	path_prefix := sprintf("/v2/%s/blobs/uploads/", [request_oci.repository])
	startswith(input.request.path.raw, path_prefix)
	session := trim_prefix(input.request.path.raw, path_prefix)
	parts[0] == urlquery.encode(session)
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
	object.keys(request_aws) == {"operation", "protocol_version", "service", "wire_protocol"}
	request_aws.wire_protocol == "query"
	aws_service_valid(request_aws.service)
	aws_operation_valid("query", request_aws.operation)
	is_string(request_aws.protocol_version)
	regex.match(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`, request_aws.protocol_version)
}

aws_request_shape_valid if {
	is_object(request_aws)
	object.keys(request_aws) == {"operation", "service", "target_namespace", "wire_protocol"}
	request_aws.wire_protocol == "json"
	aws_service_valid(request_aws.service)
	aws_operation_valid("query", request_aws.operation)
	is_string(request_aws.target_namespace)
	regex.match(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`, request_aws.target_namespace)
	count(sprintf("%s.%s", [request_aws.target_namespace, request_aws.operation])) <= 256
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
	only_semantic_coordinate_keys(rule, set())
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

aws_dynamic_coordinates_match(rule) if {
	rule.aws_wire_protocol == request_aws.wire_protocol
	rule.aws_service == request_aws.service
	object.get(rule, "aws_protocol_version", "") == object.get(request_aws, "protocol_version", "")
	object.get(rule, "aws_target_namespace", "") == object.get(request_aws, "target_namespace", "")
	rule.aws_operation == request_aws.operation
}

kubernetes_dynamic_coordinates_match(rule) if {
	request_kubernetes.kind == "resource"
	rule.kubernetes_kind == "resource"
	rule.kubernetes_verb == request_kubernetes.verb
	rule.kubernetes_group == request_kubernetes.resource.group
	rule.kubernetes_version == request_kubernetes.resource.version
	rule.kubernetes_resource == request_kubernetes.resource.resource
	object.get(rule, "kubernetes_namespace", null) == request_kubernetes.resource.namespace
	object.get(rule, "kubernetes_name", null) == request_kubernetes.resource.name
	object.get(rule, "kubernetes_subresource", null) == request_kubernetes.resource.subresource
	rule.kubernetes_dry_run == request_kubernetes.dry_run
}

kubernetes_dynamic_coordinates_match(rule) if {
	request_kubernetes.kind == "non_resource"
	rule.kubernetes_kind == "non_resource"
	rule.kubernetes_verb == request_kubernetes.verb
	rule.kubernetes_non_resource_path == request_kubernetes.path
}

graphql_rule_protocol_valid(rule) if {
	rule.protocol == "graphql"
	only_semantic_coordinate_keys(rule, {"graphql_operation_type", "graphql_root_field"})
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
	only_semantic_coordinate_keys(rule, {"mcp_method", "mcp_tool_name"})
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
	only_semantic_coordinate_keys(rule, {"mcp_method", "mcp_tool_name"})
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
	only_semantic_coordinate_keys(rule, {"aws_wire_protocol", "aws_service", "aws_protocol_version", "aws_operation"})
	rule.aws_wire_protocol == "query"
	aws_service_valid(rule.aws_service)
	aws_operation_valid("query", rule.aws_operation)
	is_string(rule.aws_protocol_version)
	regex.match(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`, rule.aws_protocol_version)
	not object_has_key(rule, "aws_target_namespace")
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "kubernetes_kind")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

aws_rule_protocol_valid(rule) if {
	rule.protocol == "aws"
	only_semantic_coordinate_keys(rule, {"aws_wire_protocol", "aws_service", "aws_target_namespace", "aws_operation"})
	rule.aws_wire_protocol == "json"
	aws_service_valid(rule.aws_service)
	aws_operation_valid("query", rule.aws_operation)
	is_string(rule.aws_target_namespace)
	regex.match(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+)*$`, rule.aws_target_namespace)
	not object_has_key(rule, "aws_protocol_version")
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "kubernetes_kind")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

kubernetes_rule_protocol_valid(rule) if {
	rule.protocol == "kubernetes"
	only_semantic_coordinate_keys(rule, {"kubernetes_kind", "kubernetes_verb", "kubernetes_group", "kubernetes_version", "kubernetes_resource", "kubernetes_namespace", "kubernetes_name", "kubernetes_subresource", "kubernetes_dry_run"})
	rule.kubernetes_kind == "resource"
	rule.kubernetes_verb in {"get", "list", "watch", "create", "update", "patch", "delete", "deletecollection", "connect"}
	rule.kubernetes_dry_run in {"none", "empty", "all"}
	is_string(rule.kubernetes_group)
	is_string(rule.kubernetes_version)
	rule.kubernetes_version != ""
	is_string(rule.kubernetes_resource)
	count(rule.kubernetes_resource) >= 1
	count(rule.kubernetes_resource) <= 1024
	not regex.match(`[\x00-\x1f\x7f]`, rule.kubernetes_resource)
	not object_has_key(rule, "kubernetes_non_resource_path")
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

kubernetes_rule_protocol_valid(rule) if {
	rule.protocol == "kubernetes"
	only_semantic_coordinate_keys(rule, {"kubernetes_kind", "kubernetes_verb", "kubernetes_non_resource_path"})
	rule.kubernetes_kind == "non_resource"
	rule.kubernetes_verb == "get"
	is_string(rule.kubernetes_non_resource_path)
	startswith(rule.kubernetes_non_resource_path, "/")
	not object_has_key(rule, "kubernetes_group")
	not object_has_key(rule, "kubernetes_version")
	not object_has_key(rule, "kubernetes_resource")
	not object_has_key(rule, "kubernetes_namespace")
	not object_has_key(rule, "kubernetes_name")
	not object_has_key(rule, "kubernetes_subresource")
	not object_has_key(rule, "kubernetes_dry_run")
	not object_has_key(rule, "graphql_operation_type")
	not object_has_key(rule, "graphql_root_field")
	not object_has_key(rule, "mcp_method")
	not object_has_key(rule, "mcp_tool_name")
	not object_has_key(rule, "aws_wire_protocol")
	not object_has_key(rule, "aws_service")
	not object_has_key(rule, "aws_protocol_version")
	not object_has_key(rule, "aws_target_namespace")
	not object_has_key(rule, "aws_operation")
	not object_has_key(rule, "git_service")
	not object_has_key(rule, "git_repository")
	not object_has_key(rule, "oci_action")
	not object_has_key(rule, "oci_repository")
	not object_has_key(rule, "oci_object")
}

git_rule_protocol_valid(rule) if {
	rule.protocol == "git"
	only_semantic_coordinate_keys(rule, {"git_service", "git_repository"})
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
	only_semantic_coordinate_keys(rule, {"oci_action", "oci_repository", "oci_object"})
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

semantic_protocol_coordinate_keys := {
	"graphql_operation_type", "graphql_root_field",
	"mcp_method", "mcp_tool_name",
	"aws_wire_protocol", "aws_service", "aws_protocol_version", "aws_target_namespace", "aws_operation",
	"kubernetes_kind", "kubernetes_verb", "kubernetes_group", "kubernetes_version", "kubernetes_resource",
	"kubernetes_namespace", "kubernetes_name", "kubernetes_subresource", "kubernetes_dry_run", "kubernetes_non_resource_path",
	"git_service", "git_repository",
	"oci_action", "oci_repository", "oci_object",
}

only_semantic_coordinate_keys(rule, allowed) if {
	count((object.keys(rule) & semantic_protocol_coordinate_keys) - allowed) == 0
}
