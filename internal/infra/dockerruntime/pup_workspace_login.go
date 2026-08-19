package dockerruntime

import (
	"net/url"
	"regexp"
	"strings"
)

const (
	pupWorkspaceAuthorizationHost = "app.datadoghq.com"    // #nosec G101 -- public OAuth authority, not a credential.
	pupWorkspaceAuthorizationPath = "/oauth2/v1/authorize" // #nosec G101 -- public OAuth route, not a credential.
	pupWorkspaceCallbackHost      = "127.0.0.1"
	pupWorkspaceCallbackPath      = "/oauth/callback" // #nosec G101 -- public OAuth callback route, not a credential.
)

var pupWorkspaceCallbackPorts = map[int]struct{}{
	8000: {},
	8080: {},
	8888: {},
	9000: {},
}

var pupWorkspaceOrgUUIDPattern = regexp.MustCompile(
	`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`,
)

// This is the compiled default scope ceiling of pup 1.10.7. The Workspace
// contract accepts a sorted subset so --read-only and explicitly reduced
// scope sets work, while caller-added future or administrative scopes fail
// closed until the pinned client contract is reviewed again.
// #nosec G101 -- public OAuth scope names, not credentials.
const pupWorkspaceAuthorizationScopeCeiling = `
apm_generate_metrics apm_read apm_remote_configuration_read apm_remote_configuration_write
apm_service_catalog_read apm_service_ingest_read apm_service_ingest_write apm_service_renaming_write
apps_run apps_write audit_logs_read aws_configuration_read azure_configuration_read
bits_investigations_read bits_investigations_write built_in_features cases_read cases_write
ci_visibility_read cloud_cost_management_read cloud_cost_management_write code_coverage_read
connections_read dashboards_read dashboards_write data_scanner_read data_streams_monitoring_capture_messages
dbm_read debugger_capture_variables debugger_read debugger_write disaster_recovery_status_read
disaster_recovery_status_write dora_metrics_read dora_metrics_write error_tracking_read events_read
feature_flag_config_read feature_flag_config_write feature_flag_environment_config_read
feature_flag_environment_config_write gcp_configuration_read host_tags_write hosts_read incident_notification_settings_read
incident_read incident_settings_read incident_settings_write incident_write integrations_read
llm_observability_read llm_observability_write logs_generate_metrics logs_modify_indexes logs_read_archives
logs_read_config logs_read_data logs_read_index_data logs_write_archives logs_write_pipelines
manage_integrations metrics_read monitors_downtime monitors_read monitors_write notebooks_read notebooks_write
observability_pipelines_delete observability_pipelines_deploy observability_pipelines_read oci_configuration_edit
oci_configuration_read oci_configurations_manage on_call_read on_call_write org_management
reference_tables_read reference_tables_write rum_apps_read rum_apps_write rum_generate_metrics
rum_retention_filters_read rum_retention_filters_write rum_session_replay_read saved_views_write
security_monitoring_filters_read security_monitoring_filters_write security_monitoring_findings_read
security_monitoring_findings_write security_monitoring_rules_read security_monitoring_rules_write
security_monitoring_signals_read slos_read slos_write status_pages_incident_write status_pages_settings_read
status_pages_settings_write synthetics_private_location_read synthetics_read synthetics_write teams_manage
teams_read test_optimization_read test_optimization_write timeseries_query usage_read user_access_read
workflows_read workflows_run workflows_write
`

func validPupWorkspaceLoginAuthorizationURL(target string) bool {
	_, ok := parsePupWorkspaceLoginAuthorizationURL(target)
	return ok
}

func parsePupWorkspaceLoginAuthorizationURL(target string) (workspaceLoginAuthorization, bool) {
	parsed, err := url.Parse(target)
	if err != nil || parsed.Scheme != "https" || parsed.User != nil || parsed.Host != pupWorkspaceAuthorizationHost ||
		parsed.Path != pupWorkspaceAuthorizationPath || parsed.RawPath != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.RawFragment != "" {
		return workspaceLoginAuthorization{}, false
	}
	query, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return workspaceLoginAuthorization{}, false
	}
	want := map[string]string{
		"response_type":         "code",
		"code_challenge_method": "S256",
	}
	for key, value := range want {
		if len(query[key]) != 1 || query[key][0] != value {
			return workspaceLoginAuthorization{}, false
		}
	}
	// Do not restore a total-field-count check here. Pup 1.10.7 adds dd_oid
	// after it has remembered an organization; authority comes from validating
	// every mandatory field and the finite reviewed optional set independently.
	for key := range query {
		switch key {
		case "response_type", "client_id", "redirect_uri", "state", "scope", "code_challenge", "code_challenge_method", "dd_oid":
		default:
			return workspaceLoginAuthorization{}, false
		}
	}
	if values, present := query["dd_oid"]; present &&
		(len(values) != 1 || !pupWorkspaceOrgUUIDPattern.MatchString(values[0])) {
		return workspaceLoginAuthorization{}, false
	}
	if len(query["client_id"]) != 1 || !pupOAuthClientIDPattern.MatchString(query["client_id"][0]) ||
		len(query["state"]) != 1 || !pupOAuthStatePattern.MatchString(query["state"][0]) ||
		len(query["code_challenge"]) != 1 || !claudeOAuthOpaquePattern.MatchString(query["code_challenge"][0]) ||
		len(query["scope"]) != 1 || !validPupWorkspaceScopeSubset(query["scope"][0]) ||
		len(query["redirect_uri"]) != 1 {
		return workspaceLoginAuthorization{}, false
	}
	callbackPort, ok := parseWorkspaceCallbackPort(
		query["redirect_uri"][0], pupWorkspaceCallbackHost, pupWorkspaceCallbackPath,
	)
	if !ok {
		return workspaceLoginAuthorization{}, false
	}
	if _, allowed := pupWorkspaceCallbackPorts[callbackPort]; !allowed {
		return workspaceLoginAuthorization{}, false
	}
	return workspaceLoginAuthorization{callbackPort: callbackPort}, true
}

func validPupWorkspaceScopeSubset(value string) bool {
	if !validSortedPupOAuthScopes(value) {
		return false
	}
	allowed := make(map[string]struct{})
	for _, scope := range strings.Fields(pupWorkspaceAuthorizationScopeCeiling) {
		allowed[scope] = struct{}{}
	}
	actual := strings.Split(value, " ")
	if len(actual) > len(allowed) {
		return false
	}
	for _, scope := range actual {
		if _, ok := allowed[scope]; !ok {
			return false
		}
	}
	return true
}
