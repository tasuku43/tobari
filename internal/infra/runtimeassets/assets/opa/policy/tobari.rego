package tobari.http

import rego.v1

default decision := {
	"allow": false,
	"reason": "request did not match an allow rule",
	"credential_profile": null,
	"status_code": 403,
	"learnable": false,
	"audit": {"level": "metadata"},
}

decision := {
	"allow": false,
	"reason": "request did not match an allow rule",
	"credential_profile": input.credential.requested_profile,
	"status_code": 403,
	"learnable": true,
	"audit": {"level": "metadata"},
} if {
	candidate_eligible
	not request_allowed
}

decision := {
	"allow": true,
	"reason": "allowed by policy",
	"credential_profile": input.credential.requested_profile,
	"status_code": 403,
	"learnable": false,
	"audit": {"level": "metadata"},
} if {
	candidate_eligible
	request_allowed
}

candidate_eligible if {
	input.version == "v1"
	input.principal.cluster == "default"
	allowed_scheme
	allowed_port
	body_is_empty
	credential_binding_valid
}

request_allowed if {
	allowed_host
	allowed_port
	body_is_empty
	allowed_method
	not explicitly_denied
}

request_allowed if {
	allowed_scheme
	allowed_port
	body_is_empty
	learned_rule_allowed
}

allowed_host if {
	input.request.host in data.tobari.allowed_hosts
}

allowed_scheme if {
	input.request.scheme == "https"
}

allowed_scheme if {
	input.request.scheme == "http"
	input.request.host in data.tobari.allowed_http_hosts
}

allowed_port if {
	ports := data.tobari.allowed_ports[input.request.scheme]
	input.request.port in ports
}

body_is_empty if {
	input.request.body.kind == "metadata"
	input.request.body.size == 0
	input.request.body.truncated == false
}

allowed_method if {
	input.request.method in data.tobari.read_methods
}

allowed_method if {
	input.request.method == "POST"
	not startswith(input.request.path, "/repos/")
}

explicitly_denied if {
	some rule in data.tobari.deny_rules
	rule.host == input.request.host
	rule.method == input.request.method
	startswith(input.request.path, rule.path_prefix)
}

default learned_rules := []

learned_rules := data.tobari.learned_allow_rules

learned_rule_allowed if {
	some rule in learned_rules
	learned_rule_valid(rule)
	learned_rule_matches_request(rule, input.request)
}

learned_rule_matches_request(rule, request) if {
	rule.match == "exact"
	rule.host == request.host
	rule.port == request.port
	rule.method == request.method
	rule.path == request.path
}

learned_rule_matches_request(rule, request) if {
	rule.match == "prefix"
	rule.host == request.host
	rule.port == request.port
	rule.method == request.method
	prefix_request_path_safe(request.path)
	startswith(request.path, rule.path)
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
	input.credential.requested_profile == null
}

credential_binding_valid if {
	profile := input.credential.requested_profile
	profile != null
	input.request.host in data.tobari.credentials[profile].hosts
}
