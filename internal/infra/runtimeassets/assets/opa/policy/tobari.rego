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

candidate_eligible if {
	input.schema_version == 3
	data.tobari.schema_version == 2
	input.principal.cluster == "default"
	project_principal_valid
	transport_port_allowed
	candidate_scheme_allowed
	authorization_shape_valid
	credential_binding_valid
	not explicitly_denied
}

request_allowed if {
	authority_allowed
	method_allowed
}

request_allowed if {
	learned_rule_allowed
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

authority_allowed if {
	some authority in data.tobari.boundary.authorities
	authority.scheme == input.request.authority.scheme
	authority.host == input.request.authority.host
	input.request.authority.port in authority.ports
}

authorization_shape_valid if {
	requested_profile == null
}

authorization_shape_valid if {
	is_string(requested_profile)
	requested_profile != ""
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
	rule.project_id == input.principal.project_id
	rule.host == input.request.authority.host
	rule.port == input.request.authority.port
	rule.method == input.request.method
	rule.path == input.request.path.raw
}

default learned_rules := []

learned_rules := data.tobari.rules.learned_allows

default learned_deny_rules := []

learned_deny_rules := data.tobari.rules.learned_denies

learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	learned_rule_matches_request(rule, input.principal.project_id, input.request)
}

learned_rule_matches_request(rule, project_id, request) if {
	rule.match == "exact"
	rule.project_id == project_id
	rule.host == request.authority.host
	rule.port == request.authority.port
	rule.method == request.method
	rule.path == request.path.raw
}

learned_rule_matches_request(rule, project_id, request) if {
	rule.match == "prefix"
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
	is_string(rule.id)
	regex.match("^plr_[0-9a-f]{32}$", rule.id)
	is_string(rule.project_id)
	regex.match("^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$", rule.project_id)
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
	learned_rule_scope_valid(rule)
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
