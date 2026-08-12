// Package tobari defines the shared enforcement cluster and CWD-owned isolation spaces.
package tobari

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	TaskClusterUp         = "cluster.up"
	TaskClusterStatus     = "cluster.status"
	TaskClusterDenials    = "cluster.denials"
	TaskClusterDown       = "cluster.down"
	TaskPolicyCandidates  = "policy.candidates"
	TaskPolicyReview      = "policy.review"
	TaskPolicyReviewApply = "policy.review.apply"
	TaskPolicyRules       = "policy.rules"
	TaskPolicyAllow       = "policy.allow"
	TaskPolicyDeny        = "policy.deny"
	TaskPolicyReset       = "policy.reset"
	ClusterTargetKind     = "cluster"
	ClusterTargetID       = "cluster-default"
	PolicyCandidateKind   = "policy-candidate"
	PolicyRuleKind        = "policy-rule"
	PolicyDecisionSetKind = "policy-decision-set"
	PolicyDecisionSetID   = "policy-decision-set-default"

	BuiltinImageSelector        = "builtin"
	RuntimeImageAPILabel        = "io.tobari.runtime-api"
	RuntimeImageAPI             = "1"
	RuntimeImageLifetimeLabel   = "io.tobari.runtime-lifetime-command"
	RuntimeImageLifetimeCommand = "sleep infinity"
)

var (
	namePattern           = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	imageReferencePattern = regexp.MustCompile(`^(?:(?:localhost|[a-z0-9]+(?:[.-][a-z0-9]+)*)(?::[0-9]{1,5})?/)?[a-z0-9]+(?:[._-][a-z0-9]+)*(?:/[a-z0-9]+(?:[._-][a-z0-9]+)*)*(?::[A-Za-z0-9_][A-Za-z0-9_.-]{0,127})?(?:@sha256:[0-9a-f]{64})?$`)
)

// ValidateName accepts a portable Docker-resource-safe display name.
func ValidateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("name must match [a-z][a-z0-9-]{0,62}")
	}
	return nil
}

// ValidateImageSelector accepts the built-in runtime or one conservative OCI image reference.
func ValidateImageSelector(image string) error {
	if image == BuiltinImageSelector {
		return nil
	}
	if len(image) == 0 || len(image) > 255 || !imageReferencePattern.MatchString(image) {
		return fmt.Errorf("image must be %q or a portable OCI image reference", BuiltinImageSelector)
	}
	return nil
}

// State is the secret-free persisted identity of the shared cluster.
type State struct {
	SchemaVersion     int    `json:"schema_version"`
	RuntimeDirectory  string `json:"runtime_directory"`
	ContextName       string `json:"context_name,omitempty"`
	AgentProfile      string `json:"agent_profile,omitempty"`
	AggregateRevision string `json:"aggregate_revision,omitempty"`
	ContextCount      int    `json:"context_count,omitempty"`
	PolicyDirectory   string `json:"policy_directory"`
	CredentialConfig  string `json:"credential_config"`
	CredentialDir     string `json:"credential_directory"`
	AssetVersion      string `json:"asset_version"`
	RecentError       string `json:"recent_error"`
}

// Validate rejects incomplete, ambiguous, or relative state.
func (s State) Validate() error {
	if s.SchemaVersion != 1 {
		return fmt.Errorf("Tobari state schema version must be 1")
	}
	for name, value := range map[string]string{
		"runtime directory": s.RuntimeDirectory,
		"policy directory":  s.PolicyDirectory, "credential config": s.CredentialConfig,
		"credential directory": s.CredentialDir,
	} {
		if !filepath.IsAbs(value) || filepath.Clean(value) != value {
			return fmt.Errorf("%s must be a canonical absolute path", name)
		}
	}
	if s.AssetVersion == "" || strings.ContainsAny(s.AssetVersion, " \t\r\n") {
		return fmt.Errorf("asset version is invalid")
	}
	if s.ContextName != "" || s.AgentProfile != "" {
		return fmt.Errorf("shared state must not contain a selected Context authority")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(s.AggregateRevision) || s.ContextCount < 1 {
		return fmt.Errorf("aggregate Context projection is invalid")
	}
	if len(s.RecentError) > 1024 || strings.IndexFunc(s.RecentError, func(r rune) bool {
		return r < ' ' || r == '\u007f' || r == '\u2028' || r == '\u2029'
	}) >= 0 {
		return fmt.Errorf("recent error is unsafe")
	}
	return nil
}

// ComponentStatus is one exact managed container observation.
type ComponentStatus struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Health string `json:"health"`
}

// ClusterStatus is a task-bound observation of shared enforcement.
type ClusterStatus struct {
	Task                     string            `json:"task"`
	Configured               bool              `json:"configured"`
	Running                  bool              `json:"running"`
	Policy                   string            `json:"policy"`
	TobariCount              int               `json:"tobari_count"`
	ContextCount             int               `json:"context_count"`
	PolicyRevision           string            `json:"policy_revision"`
	PolicyProjection         string            `json:"policy_projection"`
	PrincipalRegistry        string            `json:"principal_registry"`
	CredentialProjection     string            `json:"credential_projection"`
	AuthProviderProjection   string            `json:"auth_provider_projection"`
	AuthBrokerState          string            `json:"auth_broker_state"`
	CredentialCompanionState string            `json:"credential_companion_state"`
	RootKeyBackend           string            `json:"root_key_backend"`
	Components               []ComponentStatus `json:"components"`
	RecentError              string            `json:"recent_error"`
}

// UnconfiguredClusterStatus preserves an explicit finite observation for every
// cluster-owned state dimension while leaving resources that do not exist
// absent. It prevents consumers from having to decode empty-string sentinels.
func UnconfiguredClusterStatus(task string) ClusterStatus {
	return ClusterStatus{
		Task: task, PolicyProjection: "unavailable", PrincipalRegistry: "unavailable",
		CredentialProjection: "unavailable", AuthProviderProjection: "unavailable",
		AuthBrokerState: "unavailable", CredentialCompanionState: "absent",
		RootKeyBackend: "unavailable", Components: []ComponentStatus{},
	}
}

// Validate binds status to the requested cluster task and scope.
func (s ClusterStatus) Validate() error {
	if s.Task != TaskClusterStatus && s.Task != TaskClusterUp && s.Task != TaskClusterDown {
		return fmt.Errorf("cluster status task identity is invalid")
	}
	if !s.Configured {
		if s.Running || s.Policy != "" || s.TobariCount != 0 || s.ContextCount != 0 || s.PolicyRevision != "" || len(s.Components) != 0 {
			return fmt.Errorf("unconfigured status contains cluster state")
		}
		if s.Components == nil || s.PolicyProjection != "unavailable" || s.PrincipalRegistry != "unavailable" ||
			s.CredentialProjection != "unavailable" || s.AuthProviderProjection != "unavailable" ||
			s.AuthBrokerState != "unavailable" || s.CredentialCompanionState != "absent" ||
			s.RootKeyBackend != "unavailable" {
			return fmt.Errorf("unconfigured cluster observation is incomplete")
		}
		return nil
	}
	if !filepath.IsAbs(s.Policy) || s.TobariCount < 0 || s.ContextCount < 1 ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(s.PolicyRevision) || s.Components == nil ||
		s.PolicyProjection == "" || s.PrincipalRegistry == "" || s.CredentialProjection == "" ||
		s.AuthProviderProjection == "" || s.AuthBrokerState == "" || s.CredentialCompanionState == "" || s.RootKeyBackend == "" {
		return fmt.Errorf("configured cluster status is incomplete")
	}
	if s.AuthBrokerState != "ready" && s.AuthBrokerState != "locked" && s.AuthBrokerState != "unavailable" {
		return fmt.Errorf("configured cluster Auth Broker state is invalid")
	}
	if s.CredentialCompanionState != "ready" && s.CredentialCompanionState != "prepared" &&
		s.CredentialCompanionState != "absent" && s.CredentialCompanionState != "unavailable" {
		return fmt.Errorf("configured cluster credential companion state is invalid")
	}
	if s.AuthProviderProjection != "valid" && s.AuthProviderProjection != "invalid" {
		return fmt.Errorf("configured cluster auth provider projection is invalid")
	}
	for name, state := range map[string]string{
		"policy projection":     s.PolicyProjection,
		"principal registry":    s.PrincipalRegistry,
		"credential projection": s.CredentialProjection,
	} {
		if state != "valid" && state != "invalid" {
			return fmt.Errorf("configured cluster %s is invalid", name)
		}
	}
	if s.RootKeyBackend != "macos_keychain" && s.RootKeyBackend != "xdg_file" && s.RootKeyBackend != "unavailable" {
		return fmt.Errorf("configured cluster root-key backend is invalid")
	}
	if len(s.Components) != 3 {
		return fmt.Errorf("configured cluster component collection is incomplete")
	}
	componentNames := map[string]struct{}{"auth-broker": {}, "gateway": {}, "opa": {}}
	states := map[string]struct{}{"absent": {}, "created": {}, "running": {}, "paused": {}, "restarting": {}, "removing": {}, "exited": {}, "dead": {}}
	healthStates := map[string]struct{}{"none": {}, "starting": {}, "healthy": {}, "unhealthy": {}}
	for _, component := range s.Components {
		if _, exists := componentNames[component.Name]; !exists {
			return fmt.Errorf("configured cluster component name is invalid")
		}
		delete(componentNames, component.Name)
		if _, exists := states[component.State]; !exists {
			return fmt.Errorf("configured cluster component state is invalid")
		}
		if _, exists := healthStates[component.Health]; !exists {
			return fmt.Errorf("configured cluster component health is invalid")
		}
	}
	return nil
}

// LogRequest is a bounded observation of shared or per-Tobari logs.
type LogRequest struct {
	Component string
	Tail      int
}

func (r LogRequest) ValidateCluster() error {
	switch r.Component {
	case "all", "auth-broker", "gateway", "opa":
	default:
		return fmt.Errorf("cluster log component is invalid")
	}
	return r.validateTail()
}

func (r LogRequest) validateTail() error {
	if r.Tail < 1 || r.Tail > 10_000 {
		return fmt.Errorf("log tail must be between 1 and 10000")
	}
	return nil
}
