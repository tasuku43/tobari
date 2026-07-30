package tobari.http

import rego.v1

default decision := {
	"allow": false,
	"reason": "request did not match an allow rule",
	"credential_profile": null,
	"status_code": 403,
	"audit": {"level": "metadata"},
}

decision := {
	"allow": true,
	"reason": "allowed by policy",
	"credential_profile": input.credential.requested_profile,
	"status_code": 403,
	"audit": {"level": "metadata"},
} if {
	input.version == "v1"
	input.principal.cluster == "default"
	allowed_host
	allowed_scheme
	allowed_method
	not explicitly_denied
	credential_binding_valid
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

credential_binding_valid if {
	input.credential.requested_profile == null
}

credential_binding_valid if {
	profile := input.credential.requested_profile
	profile != null
	input.request.host in data.tobari.credentials[profile].hosts
}
